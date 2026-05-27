package tracing

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"health-monitor/pkg/model"
)

// TraceConfig defines configuration needed for trace collection without depending on internal/config
type TraceConfig struct {
	TraceURL                 string
	TraceToken               string
	TraceUser                string
	TracePass                string
	TraceBackend             string
	TraceWindow              string
	TraceMinDurationMs       int
	TraceExcludeSystemRoutes bool
	TraceServiceMap          string
	TraceDefaultService      string
	GrafanaURL               string
	GrafanaTraceDataSource   string
	GrafanaToken             string
	GrafanaUser              string
	GrafanaPass              string
}

const (
	defaultTraceDisplayLimit = 3
	defaultTraceSearchLimit  = 10
)

func Collect(cfg TraceConfig, service string) (*model.TraceSummary, error) {
	if strings.TrimSpace(cfg.TraceURL) == "" {
		return nil, nil
	}
	window, windowNote := parseWindowWithNote(cfg.TraceWindow, 4*time.Minute)
	client := Client{
		BaseURL: cfg.TraceURL,
		Token:   cfg.TraceToken,
		User:    cfg.TraceUser,
		Pass:    cfg.TracePass,
		Timeout: 6 * time.Second,
		QPS:     5,
		Backend: normalizeBackend(cfg.TraceBackend),
	}

	selectionNote := ""
	if strings.TrimSpace(service) == "" {
		if client.Backend == BackendJaeger {
			selectionNote = "jaeger backend requires a service name; set trace_default_service or trace_service_map"
		}
		detected, note := detectTraceService(client, window)
		if detected != "" {
			service = detected
			selectionNote = note
		}
	}

	healthNote := ""
	if client.Backend == BackendTempo || client.Backend == "" {
		if note, err := client.TempoHealth(); err == nil && strings.TrimSpace(note) != "" {
			healthNote = note
		}
	}
	if client.Backend == BackendJaeger {
		if note, err := client.JaegerHealth(); err == nil && strings.TrimSpace(note) != "" {
			healthNote = note
		}
	}

	traces, backend, fetchNote, err := fetchTraces(client, service, window, defaultTraceSearchLimit)
	if err != nil {
		return &model.TraceSummary{
			Service:    service,
			Window:     window.String(),
			Backend:    string(backend),
			BackendURL: cfg.TraceURL,
			Note:       joinTraceNotes(windowNote, selectionNote, healthNote, fetchNote, "Trace data unavailable: "+err.Error()),
		}, nil
	}

	filtered, note := filterTraceCandidates(traces, cfg.TraceMinDurationMs, cfg.TraceExcludeSystemRoutes)
	if len(filtered) == 0 {
		if note == "" {
			note = "No usable traces found after filtering."
		}
		return &model.TraceSummary{
			Service:       service,
			Purpose:       "Trace diagnostics",
			Window:        window.String(),
			TopSlowTraces: nil,
			Backend:       string(backend),
			BackendURL:    cfg.TraceURL,
			Note:          joinTraceNotes(windowNote, selectionNote, healthNote, fetchNote, note),
		}, nil
	}

	filtered = limitTraces(filtered, defaultTraceDisplayLimit)
	items := make([]model.TraceItem, 0, len(filtered))
	dbNotSlowest := 0
	truncatedCount := 0
	maxSpansSeen := 0
	for _, t := range filtered {
		slowestService := t.SlowestSpan.Service
		if slowestService == "" || strings.EqualFold(slowestService, t.RootSpan.Service) {
			if strings.TrimSpace(t.SlowestSpan.PeerService) != "" {
				slowestService = t.SlowestSpan.PeerService
			}
		}
		if t.HasDBSpan && !t.SlowestSpan.IsDB {
			dbNotSlowest++
		}
		if t.Truncated {
			truncatedCount++
			if t.SpanCount > maxSpansSeen {
				maxSpansSeen = t.SpanCount
			}
		}
		item := model.TraceItem{
			TraceID:       t.TraceID,
			TotalDuration: formatDuration(t.Duration),
			RootOperation: t.RootSpan.Name,
			SlowestSpan: model.TraceSpanSummary{
				Service:   slowestService,
				Operation: t.SlowestSpan.Name,
				Duration:  formatDuration(t.SlowestSpan.Duration),
			},
		}
		items = append(items, item)
	}

	grafanaNote := grafanaTraceNote(cfg)
	return &model.TraceSummary{
		Service:       service,
		Purpose:       "Trace diagnostics",
		Window:        window.String(),
		TopSlowTraces: items,
		Backend:       string(backend),
		BackendURL:    cfg.TraceURL,
		Links:         buildTraceLinks(cfg, backend, filtered, window),
		Note:          joinTraceNotes(windowNote, selectionNote, healthNote, fetchNote, note, grafanaNote, truncatedNote(truncatedCount, maxSpansSeen), dbNotSlowestNote(dbNotSlowest)),
	}, nil
}

func fetchTraces(client Client, service string, window time.Duration, limit int) ([]Trace, Backend, string, error) {
	if limit <= 0 {
		limit = defaultTraceSearchLimit
	}
	switch client.Backend {
	case BackendJaeger:
		traces, err := client.JaegerSearch(service, window, limit)
		return normalizeTraces(traces, limit), BackendJaeger, "", err
	case BackendTempo:
		traces, note, err := fetchTempoTraces(client, service, window, limit)
		return normalizeTraces(traces, limit), BackendTempo, note, err
	default:
		traces, note, err := fetchTempoTraces(client, service, window, limit)
		if err == nil && len(traces) > 0 {
			return normalizeTraces(traces, limit), BackendTempo, note, nil
		}
		jtraces, jerr := client.JaegerSearch(service, window, limit)
		if jerr == nil && len(jtraces) > 0 {
			return normalizeTraces(jtraces, limit), BackendJaeger, "", nil
		}
		if err != nil {
			return nil, "", note, err
		}
		if jerr != nil {
			return nil, "", "", jerr
		}
		return nil, "", "", errors.New("no traces found")
	}
}

func fetchTempoTraces(client Client, service string, window time.Duration, limit int) ([]Trace, string, error) {
	traces, err := client.TempoSearch(service, window, limit)
	if err != nil {
		return nil, "", err
	}
	noteParts := []string{}
	if len(traces) == 0 && window > 0 {
		widened := window * 3
		if widened > 30*time.Minute {
			widened = 30 * time.Minute
		}
		if widened > window {
			traces, err = client.TempoSearch(service, widened, limit)
			if err != nil {
				return nil, "", err
			}
			if len(traces) > 0 {
				noteParts = append(noteParts, fmt.Sprintf("widened trace window to %s", widened))
			}
		}
	}
	detailed := make([]Trace, 0, len(traces))
	fetchFailures := 0
	for _, t := range traces {
		trace, err := client.TempoFetchTrace(t.TraceID)
		if err != nil {
			fetchFailures++
			continue
		}
		if trace.TraceID == "" {
			trace.TraceID = t.TraceID
		}
		if trace.Duration == 0 {
			trace.Duration = t.Duration
		}
		detailed = append(detailed, trace)
	}
	if fetchFailures > 0 {
		noteParts = append(noteParts, fmt.Sprintf("some trace details unavailable (%d); possible sampling or retention", fetchFailures))
	}
	if len(detailed) > 0 {
		return detailed, strings.Join(noteParts, " • "), nil
	}
	return traces, strings.Join(noteParts, " • "), nil
}

func normalizeTraces(traces []Trace, limit int) []Trace {
	sort.Slice(traces, func(i, j int) bool {
		return traces[i].Duration > traces[j].Duration
	})
	if limit > 0 && len(traces) > limit {
		return traces[:limit]
	}
	return traces
}

func limitTraces(traces []Trace, limit int) []Trace {
	if limit <= 0 || len(traces) <= limit {
		return traces
	}
	return traces[:limit]
}

func filterTraceCandidates(traces []Trace, minDurationMs int, excludeSystem bool) ([]Trace, string) {
	if len(traces) == 0 {
		return nil, ""
	}
	var filtered []Trace
	removedSystem := 0
	removedShort := 0
	minDuration := time.Duration(minDurationMs) * time.Millisecond
	for _, t := range traces {
		root := strings.ToLower(strings.TrimSpace(t.RootSpan.Name))
		if excludeSystem && isSystemTraceRoot(root) {
			removedSystem++
			continue
		}
		if minDurationMs > 0 && t.Duration > 0 && t.Duration < minDuration {
			removedShort++
			continue
		}
		filtered = append(filtered, t)
	}
	if len(filtered) == 0 {
		note := "Only system or tiny traces found; increase traffic window or exclude internal endpoints."
		return nil, note
	}
	noteParts := []string{}
	if removedSystem > 0 && excludeSystem {
		noteParts = append(noteParts, fmt.Sprintf("filtered %d system trace(s)", removedSystem))
	}
	if removedShort > 0 && minDurationMs > 0 {
		noteParts = append(noteParts, fmt.Sprintf("filtered %d very short trace(s)", removedShort))
	}
	return filtered, strings.Join(noteParts, " • ")
}

func joinTraceNotes(notes ...string) string {
	parts := []string{}
	for _, note := range notes {
		if strings.TrimSpace(note) != "" {
			parts = append(parts, note)
		}
	}
	return strings.Join(parts, " • ")
}

func dbNotSlowestNote(count int) string {
	if count <= 0 {
		return ""
	}
	return fmt.Sprintf("db spans present but not slowest in %d trace(s)", count)
}

func truncatedNote(count int, maxSpans int) string {
	if count <= 0 {
		return ""
	}
	if maxSpans > 0 {
		return fmt.Sprintf("truncated %d trace(s) with >%d spans", count, MaxSpansPerTrace)
	}
	return fmt.Sprintf("truncated %d trace(s) with large span counts", count)
}

func parseWindowWithNote(value string, fallback time.Duration) (time.Duration, string) {
	if strings.TrimSpace(value) == "" {
		return fallback, ""
	}
	if d, err := time.ParseDuration(value); err == nil && d > 0 {
		return d, ""
	}
	return fallback, "invalid trace window; using default"
}

func detectTraceService(client Client, window time.Duration) (string, string) {
	if client.Backend == BackendJaeger {
		return "", ""
	}
	services, err := client.TempoSearchServices(window, defaultTraceSearchLimit)
	if err != nil || len(services) == 0 {
		return "", ""
	}
	return services[0], "auto-detected trace service: " + services[0]
}

func buildTraceLinks(cfg TraceConfig, backend Backend, traces []Trace, window time.Duration) []model.TraceLink {
	links := []model.TraceLink{}
	baseURL := strings.TrimSuffix(cfg.TraceURL, "/")
	grafanaUID, grafanaType, _ := fetchGrafanaDatasource(cfg)
	for _, t := range traces {
		if baseURL != "" {
			switch backend {
			case BackendJaeger:
				links = append(links, model.TraceLink{
					Label: "Jaeger trace " + shortTraceID(t.TraceID),
					URL:   baseURL + "/trace/" + url.PathEscape(t.TraceID),
				})
			default:
				links = append(links, model.TraceLink{
					Label: "Tempo API trace " + shortTraceID(t.TraceID),
					URL:   baseURL + "/api/traces/" + url.PathEscape(t.TraceID),
				})
			}
		}
		if strings.TrimSpace(cfg.GrafanaURL) != "" && strings.TrimSpace(cfg.GrafanaTraceDataSource) != "" && strings.TrimSpace(grafanaUID) != "" && strings.TrimSpace(grafanaType) != "" {
			grafana := buildGrafanaTraceLink(cfg.GrafanaURL, cfg.GrafanaTraceDataSource, grafanaUID, grafanaType, t.TraceID, window)
			if grafana != "" {
				links = append(links, model.TraceLink{
					Label: "Grafana trace " + shortTraceID(t.TraceID),
					URL:   grafana,
				})
			}
		}
	}
	return links
}

func buildGrafanaTraceLink(baseURL string, datasource string, datasourceUID string, datasourceType string, traceID string, window time.Duration) string {
	if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(traceID) == "" {
		return ""
	}
	from := "now-4m"
	if window > 0 {
		from = "now-" + window.String()
	}
	ds := any(datasource)
	if strings.TrimSpace(datasourceUID) != "" && strings.TrimSpace(datasourceType) != "" {
		ds = map[string]string{"uid": datasourceUID, "type": datasourceType}
	} else if strings.TrimSpace(datasource) == "" {
		return ""
	}
	queryType := "traceId"
	if strings.EqualFold(strings.TrimSpace(datasourceType), "jaeger") {
		queryType = "traceId"
	}
	panes := map[string]any{
		"pane-1": map[string]any{
			"datasource": ds,
			"queries": []map[string]string{
				{
					"refId":     "A",
					"query":     traceID,
					"queryType": queryType,
				},
			},
			"range": map[string]string{
				"from": from,
				"to":   "now",
			},
		},
	}
	raw, _ := json.Marshal(panes)
	encoded := url.QueryEscape(string(raw))
	return strings.TrimSuffix(baseURL, "/") + "/explore?schemaVersion=1&panes=" + encoded
}

func grafanaTraceNote(cfg TraceConfig) string {
	if strings.TrimSpace(cfg.GrafanaURL) == "" || strings.TrimSpace(cfg.GrafanaTraceDataSource) == "" {
		return ""
	}
	uid, dsType, err := fetchGrafanaDatasource(cfg)
	if err != nil || uid == "" || dsType == "" {
		return "grafana trace links unavailable; set grafana_user/grafana_pass or grafana_token to resolve datasource UID"
	}
	return ""
}

func fetchGrafanaDatasource(cfg TraceConfig) (string, string, error) {
	baseURL := strings.TrimSuffix(cfg.GrafanaURL, "/")
	if baseURL == "" || strings.TrimSpace(cfg.GrafanaTraceDataSource) == "" {
		return "", "", errors.New("grafana url or datasource name missing")
	}
	req, err := http.NewRequest(http.MethodGet, baseURL+"/api/datasources/name/"+url.PathEscape(cfg.GrafanaTraceDataSource), nil)
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(cfg.GrafanaToken) != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.GrafanaToken)
	}
	if strings.TrimSpace(cfg.GrafanaUser) != "" || strings.TrimSpace(cfg.GrafanaPass) != "" {
		req.SetBasicAuth(cfg.GrafanaUser, cfg.GrafanaPass)
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("grafana datasource lookup failed: %s", resp.Status)
	}
	var payload struct {
		UID  string `json:"uid"`
		Type string `json:"type"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", "", err
	}
	if payload.UID == "" || payload.Type == "" {
		return "", "", errors.New("grafana datasource response missing uid/type")
	}
	return payload.UID, payload.Type, nil
}

func shortTraceID(traceID string) string {
	trimmed := strings.TrimSpace(traceID)
	if len(trimmed) <= 8 {
		return trimmed
	}
	return trimmed[:8]
}

func isSystemTraceRoot(name string) bool {
	if name == "" {
		return false
	}
	systemMarkers := []string{
		"/metrics", "/health", "/ready", "/live", "/healthz", "/readyz", "/livez",
		"/favicon.ico", "/status", "/debug", "/-/ready", "/-/healthy",
	}
	for _, marker := range systemMarkers {
		if strings.Contains(name, marker) {
			return true
		}
	}
	return false
}

func normalizeBackend(value string) Backend {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "tempo":
		return BackendTempo
	case "jaeger":
		return BackendJaeger
	default:
		return ""
	}
}

func parseWindow(value string, fallback time.Duration) time.Duration {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	if d, err := time.ParseDuration(value); err == nil && d > 0 {
		return d
	}
	return fallback
}

func formatDuration(value time.Duration) string {
	if value <= 0 {
		return "N/A"
	}
	if value < time.Second {
		return value.Truncate(time.Millisecond).String()
	}
	if value < time.Minute {
		return value.Truncate(10 * time.Millisecond).String()
	}
	return value.Truncate(time.Second).String()
}

func buildTraceItems(traces []Trace) []model.TraceItem {
	items := make([]model.TraceItem, 0, len(traces))
	for _, t := range traces {
		item := model.TraceItem{
			TraceID:       t.TraceID,
			TotalDuration: formatDuration(t.Duration),
			RootOperation: t.RootSpan.Name,
			SlowestSpan: model.TraceSpanSummary{
				Service:   t.SlowestSpan.Service,
				Operation: t.SlowestSpan.Name,
				Duration:  formatDuration(t.SlowestSpan.Duration),
			},
		}
		items = append(items, item)
	}
	return items
}
