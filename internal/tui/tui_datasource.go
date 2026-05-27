package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"os"

	"health-monitor/internal/analyse/loki"
	"health-monitor/internal/config"
	"health-monitor/internal/logs"
	"health-monitor/internal/output"
	"github.com/charmbracelet/bubbletea"
)

func (m *tuiModel) resolveGrafanaDatasource(cfg config.Config, name string) (string, string) {
	name = strings.TrimSpace(name)
	if strings.TrimSpace(cfg.GrafanaURL) == "" || name == "" {
		return "", ""
	}
	if m.grafanaDSCache == nil {
		m.grafanaDSCache = map[string]grafanaDSCacheEntry{}
	}
	if entry, ok := m.grafanaDSCache[name]; ok {
		if time.Since(entry.fetchedAt) < 10*time.Minute {
			return entry.uid, entry.dsType
		}
	}
	uid, dsType, err := output.FetchGrafanaDatasource(cfg, name)
	if err != nil {
		m.grafanaDSCache[name] = grafanaDSCacheEntry{uid: "", dsType: "", fetchedAt: time.Now()}
		return "", ""
	}
	m.grafanaDSCache[name] = grafanaDSCacheEntry{uid: uid, dsType: dsType, fetchedAt: time.Now()}
	return uid, dsType
}

func (m tuiModel) fetchLokiLogsCmd() tea.Cmd {
	return func() tea.Msg {
		entries, query, note, msg := m.fetchLokiLogs()
		return logsMsg{entries: entries, query: query, note: note, message: msg}
	}
}

func (m tuiModel) fetchLokiLogs() ([]loki.LogEntry, string, string, string) {
	cfg, err := config.Load()
	if err != nil {
		return nil, "", "", "Failed to load config for Loki."
	}
	if os.Getenv("HEALTH_MONITOR_DEMO_LOGS") == "true" {
		return fetchMockLogs(), "{service=\"*\"} | demo_mode", "Mock logs generated for demo mode", ""
	}

	if cfg.LokiURL == "" {
		return nil, "", "", "Loki URL not set."
	}

	service := cfg.APIService
	route := cfg.APIRoute
	if m.report.APILatency != nil {
		if strings.TrimSpace(m.report.APILatency.Service) != "" {
			service = m.report.APILatency.Service
		}
		if strings.TrimSpace(m.report.APILatency.Route) != "" {
			route = m.report.APILatency.Route
		}
	}
	service = normalizeSelection(service)
	route = normalizeSelection(route)

	client := loki.Client{
		BaseURL: cfg.LokiURL,
		Token:   cfg.LokiToken,
		User:    cfg.LokiUser,
		Pass:    cfg.LokiPass,
		Timeout: 6 * time.Second,
		QPS:     cfg.LokiQPS,
	}

	serviceLabel, routeLabel, note := detectLokiLabels(&client, cfg, service, route)
	selectorParts := []string{}
	if serviceLabel != "" && service != "" {
		selectorParts = append(selectorParts, fmt.Sprintf(`%s="%s"`, serviceLabel, service))
	}
	if routeLabel != "" && route != "" {
		selectorParts = append(selectorParts, fmt.Sprintf(`%s="%s"`, routeLabel, route))
	}
	if len(selectorParts) == 0 {
		if serviceLabel != "" {
			selectorParts = append(selectorParts, fmt.Sprintf(`%s=~".+"`, serviceLabel))
			note = joinNotesInline(note, []string{"using label presence filter: " + serviceLabel})
		} else if routeLabel != "" {
			selectorParts = append(selectorParts, fmt.Sprintf(`%s=~".+"`, routeLabel))
			note = joinNotesInline(note, []string{"using label presence filter: " + routeLabel})
		}
	}
	selector := "{}"
	if len(selectorParts) > 0 {
		selector = "{" + strings.Join(selectorParts, ",") + "}"
	}
	query := selector
	if strings.TrimSpace(cfg.LokiErrorRegex) != "" {
		regex := output.NormalizeLokiRegex(cfg.LokiErrorRegex)
		safeRegex, normalized := output.SanitizeLokiRegex(regex)
		if normalized {
			note = joinNotesInline(note, []string{`regex normalized (\d -> [0-9])`})
		}
		query = query + ` |~ "` + output.EscapeLogQLString(safeRegex) + `"`
	}

	window := cfg.LokiWindow
	if window == "" {
		window = "10m"
	}
	dur, err := time.ParseDuration(window)
	if err != nil || dur <= 0 {
		dur = 10 * time.Minute
		window = "10m"
	}
	end := time.Now()
	start := end.Add(-dur)

	entries, err := client.QueryRange(query, start, end, 200)
	if err != nil {
		var httpErr loki.HTTPError
		shouldRetryWithoutRegex := strings.TrimSpace(cfg.LokiErrorRegex) != ""
		if errors.As(err, &httpErr) && httpErr.StatusCode == 400 {
			shouldRetryWithoutRegex = true
		}
		errText := strings.ToLower(err.Error())
		if strings.Contains(errText, "parse error") || strings.Contains(errText, "malformed") || strings.Contains(errText, "regexp") {
			shouldRetryWithoutRegex = true
		}
		if shouldRetryWithoutRegex {
			query = selector
			entries, err = client.QueryRange(query, start, end, 200)
			if err == nil {
				return entries, query, joinNotesInline(note, []string{"regex filter removed (query parse error)"}), ""
			}
		}
		if errors.As(err, &httpErr) && strings.TrimSpace(httpErr.Body) != "" {
			return nil, query, note, "Loki query failed: " + err.Error() + " body: " + output.SanitizeLogLine(httpErr.Body)
		}
		return nil, query, note, "Loki query failed: " + err.Error() + ". Try adjusting labels or error regex."
	}
	return entries, query, note, ""
}

func (m tuiModel) fetchElasticLogsCmd() tea.Cmd {
	return func() tea.Msg {
		entries, query, note, msg := m.fetchElasticLogs()
		return logsMsg{entries: entries, query: query, note: note, message: msg}
	}
}

func (m tuiModel) fetchElasticLogs() ([]loki.LogEntry, string, string, string) {
	cfg, err := config.Load()
	if err != nil {
		return nil, "", "", "Failed to load config for Elastic."
	}
	if cfg.ElasticURL == "" {
		return nil, "", "", "Elasticsearch URL not set."
	}

	service := cfg.APIService
	route := cfg.APIRoute
	if m.report.APILatency != nil {
		if strings.TrimSpace(m.report.APILatency.Service) != "" {
			service = m.report.APILatency.Service
		}
		if strings.TrimSpace(m.report.APILatency.Route) != "" {
			route = m.report.APILatency.Route
		}
	}
	service = normalizeSelection(service)
	route = normalizeSelection(route)

	client, err := logs.NewElasticsearchClient(
		cfg.ElasticURL,
		cfg.ElasticUser,
		cfg.ElasticPass,
		cfg.ElasticToken,
		cfg.ElasticIndex,
		cfg.ElasticServiceField,
		cfg.ElasticErrorField,
		cfg.ElasticTimeField,
		cfg.ElasticErrorRegex,
	)
	if err != nil {
		return nil, "", "", "Failed to initialize Elasticsearch client: " + err.Error()
	}

	window := cfg.LokiWindow // Reuse window setting for consistency, or use a default
	if window == "" {
		window = "10m"
	}
	dur, _ := time.ParseDuration(window)
	if dur <= 0 {
		dur = 10 * time.Minute
	}
	end := time.Now()
	start := end.Add(-dur)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	entries, err := client.QueryLogs(ctx, service, route, start, end, 200)
	if err != nil {
		return nil, "", "", "Elasticsearch query failed: " + err.Error()
	}

	query := fmt.Sprintf("index: %s, service: %s, route: %s", cfg.ElasticIndex, service, route)
	return entries, query, "", ""
}

func detectLokiLabels(client *loki.Client, cfg config.Config, service string, route string) (string, string, string) {
	if client == nil {
		return cfg.LokiServiceLabel, cfg.LokiRouteLabel, ""
	}
	labels, err := client.Labels()
	if err != nil {
		return cfg.LokiServiceLabel, cfg.LokiRouteLabel, ""
	}
	labelSet := map[string]struct{}{}
	for _, l := range labels {
		labelSet[l] = struct{}{}
	}

	serviceCandidates := []string{"service", "service_name", "app", "container_name", "container"}
	routeCandidates := []string{"route", "url", "path", "method"}

	serviceLabel := ""
	routeLabel := ""
	noteParts := []string{}

	if cfg.LokiServiceLabel != "" {
		if _, ok := labelSet[cfg.LokiServiceLabel]; ok {
			serviceLabel = cfg.LokiServiceLabel
			noteParts = append(noteParts, "service label: "+serviceLabel)
		}
	}
	if cfg.LokiRouteLabel != "" {
		if _, ok := labelSet[cfg.LokiRouteLabel]; ok {
			routeLabel = cfg.LokiRouteLabel
			noteParts = append(noteParts, "route label: "+routeLabel)
		}
	}

	if serviceLabel == "" && strings.TrimSpace(service) != "" {
		if matched := findLabelWithValue(client, labelSet, serviceCandidates, service); matched != "" {
			serviceLabel = matched
			noteParts = append(noteParts, "service label: "+serviceLabel)
		}
	}
	for _, candidate := range serviceCandidates {
		if serviceLabel != "" {
			break
		}
		if _, ok := labelSet[candidate]; !ok {
			continue
		}
		values, err := client.LabelValues(candidate)
		if err != nil || len(values) == 0 {
			continue
		}
		serviceLabel = candidate
		noteParts = append(noteParts, "service label: "+serviceLabel)
		break
	}

	if routeLabel == "" && strings.TrimSpace(route) != "" {
		if matched := findLabelWithValue(client, labelSet, routeCandidates, route); matched != "" {
			routeLabel = matched
			noteParts = append(noteParts, "route label: "+routeLabel)
		}
	}
	for _, candidate := range routeCandidates {
		if routeLabel != "" {
			break
		}
		if _, ok := labelSet[candidate]; !ok {
			continue
		}
		values, err := client.LabelValues(candidate)
		if err != nil || len(values) == 0 {
			continue
		}
		for _, v := range values {
			if strings.HasPrefix(v, "/") {
				routeLabel = candidate
				break
			}
		}
		if routeLabel == "" {
			routeLabel = candidate
		}
		noteParts = append(noteParts, "route label: "+routeLabel)
		break
	}

	if len(noteParts) > 0 {
		return serviceLabel, routeLabel, "Auto-detect: " + strings.Join(noteParts, ", ")
	}
	return serviceLabel, routeLabel, "Auto-detect: no Loki service/route labels found"
}

func findLabelWithValue(client *loki.Client, labelSet map[string]struct{}, candidates []string, target string) string {
	if client == nil || strings.TrimSpace(target) == "" {
		return ""
	}
	for _, candidate := range candidates {
		if _, ok := labelSet[candidate]; !ok {
			continue
		}
		values, err := client.LabelValues(candidate)
		if err != nil || len(values) == 0 {
			continue
		}
		for _, v := range values {
			if v == target {
				return candidate
			}
		}
	}
	return ""
}

func fetchMockLogs() []loki.LogEntry {
	now := time.Now()
	return []loki.LogEntry{
		{Timestamp: now.Add(-1 * time.Minute), Line: `{"level":"error","service":"checkout","message":"Payment gateway timeout: stripe_api took 5234ms","trace_id":"tr-ch-101"}`},
		{Timestamp: now.Add(-2 * time.Minute), Line: `{"level":"warn","service":"billing","message":"Retrying invoice generation for order #8821","trace_id":"tr-bl-202"}`},
		{Timestamp: now.Add(-3 * time.Minute), Line: `{"level":"info","service":"auth","message":"User login successful: user_id=u7712","trace_id":"tr-au-303"}`},
		{Timestamp: now.Add(-5 * time.Minute), Line: `{"level":"error","service":"checkout","message":"Upstream connection reset by peer","trace_id":"tr-ch-104"}`},
		{Timestamp: now.Add(-7 * time.Minute), Line: `{"level":"info","service":"billing","message":"Invoice generated successfully for order #8820"}`},
		{Timestamp: now.Add(-10 * time.Minute), Line: `{"level":"error","service":"checkout","message":"Failed to process payment for session_id=s_9x22","error":"context deadline exceeded"}`},
		{Timestamp: now.Add(-12 * time.Minute), Line: `{"level":"info","service":"auth","message":"Session validated for token ending in ...55a2"}`},
		{Timestamp: now.Add(-15 * time.Minute), Line: `{"level":"error","service":"postgres_db","message":"Slow query detected: SELECT * FROM orders WHERE user_id = ? (850ms)"}`},
		{Timestamp: now.Add(-20 * time.Minute), Line: `{"level":"warn","service":"checkout","message":"Circuit breaker tripped for 'billing_service'"}`},
		{Timestamp: now.Add(-25 * time.Minute), Line: `{"level":"info","service":"system","message":"Service 'checkout' health check passed"}`},
	}
}
