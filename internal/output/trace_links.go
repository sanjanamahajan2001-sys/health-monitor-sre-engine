package output

import (
	"encoding/json"
	"net/url"
	"strings"
	"sync"
	"time"

	"health-monitor/internal/config"
	"health-monitor/pkg/model"
)

type grafanaTraceCacheEntry struct {
	uid       string
	dsType    string
	fetchedAt time.Time
}

var (
	traceCacheMu sync.Mutex
	traceCache   = map[string]grafanaTraceCacheEntry{}
)

func ResolveTraceLinks(report *model.TraceSummary) []model.TraceLink {
	if report == nil || len(report.TopSlowTraces) == 0 {
		return nil
	}
	cfg, err := config.Load()
	if err != nil {
		return nil
	}
	traceIDs := make([]string, 0, len(report.TopSlowTraces))
	for _, trace := range report.TopSlowTraces {
		if strings.TrimSpace(trace.TraceID) != "" {
			traceIDs = append(traceIDs, trace.TraceID)
		}
	}
	if len(traceIDs) == 0 {
		return nil
	}
	return buildTraceLinksFromConfig(cfg, traceIDs, report.Window, report.Backend)
}

func buildTraceLinksFromConfig(cfg config.Config, traceIDs []string, window string, backend string) []model.TraceLink {
	if len(traceIDs) == 0 {
		return nil
	}
	links := []model.TraceLink{}
	baseURL := strings.TrimSuffix(cfg.TraceURL, "/")
	label := traceBackendLabel(backend, cfg.TraceBackend)
	backendName := strings.ToLower(strings.TrimSpace(label))
	grafanaUID, grafanaType := resolveGrafanaDatasource(cfg)
	for _, traceID := range traceIDs {
		if baseURL != "" {
			path := "/trace/"
			linkLabel := label + " trace " + shortTraceID(traceID)
			if backendName == "tempo" {
				path = "/api/traces/"
				linkLabel = label + " API trace " + shortTraceID(traceID)
			}
			links = append(links, model.TraceLink{
				Label: linkLabel,
				URL:   baseURL + path + url.PathEscape(traceID),
			})
		}
		if strings.TrimSpace(cfg.GrafanaURL) != "" && strings.TrimSpace(cfg.GrafanaTraceDataSource) != "" && strings.TrimSpace(grafanaUID) != "" && strings.TrimSpace(grafanaType) != "" {
			grafana := grafanaTraceExploreLink(cfg.GrafanaURL, cfg.GrafanaTraceDataSource, grafanaUID, grafanaType, traceID, window)
			if grafana != "" {
				links = append(links, model.TraceLink{
					Label: "Grafana trace " + shortTraceID(traceID),
					URL:   grafana,
				})
			}
		}
	}
	return links
}

func traceBackendLabel(reportBackend string, cfgBackend string) string {
	value := strings.TrimSpace(reportBackend)
	if value == "" {
		value = strings.TrimSpace(cfgBackend)
	}
	switch strings.ToLower(value) {
	case "jaeger":
		return "Jaeger"
	case "tempo":
		return "Tempo"
	default:
		return "Trace"
	}
}

func grafanaTraceExploreLink(baseURL string, datasource string, datasourceUID string, datasourceType string, traceID string, window string) string {
	if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(traceID) == "" {
		return ""
	}
	from := "now-4m"
	if parsed, err := time.ParseDuration(strings.TrimSpace(window)); err == nil && parsed > 0 {
		from = "now-" + parsed.String()
	}
	ds := any(datasource)
	if strings.TrimSpace(datasourceUID) != "" && strings.TrimSpace(datasourceType) != "" {
		ds = map[string]string{"uid": datasourceUID, "type": datasourceType}
	} else if strings.TrimSpace(datasource) == "" {
		return ""
	}
	panes := map[string]any{
		"pane-1": map[string]any{
			"datasource": ds,
			"queries": []map[string]string{
				{
					"refId":     "A",
					"query":     traceID,
					"queryType": "traceId",
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

func resolveGrafanaDatasource(cfg config.Config) (string, string) {
	baseURL := strings.TrimSuffix(cfg.GrafanaURL, "/")
	dsName := strings.TrimSpace(cfg.GrafanaTraceDataSource)
	if baseURL == "" || dsName == "" {
		return "", ""
	}
	cacheKey := baseURL + "|" + dsName
	traceCacheMu.Lock()
	entry, ok := traceCache[cacheKey]
	if ok && time.Since(entry.fetchedAt) < 10*time.Minute {
		traceCacheMu.Unlock()
		return entry.uid, entry.dsType
	}
	traceCacheMu.Unlock()

	uid, dsType, err := FetchGrafanaDatasource(cfg, dsName)
	if err != nil {
		traceCacheMu.Lock()
		traceCache[cacheKey] = grafanaTraceCacheEntry{uid: "", dsType: "", fetchedAt: time.Now()}
		traceCacheMu.Unlock()
		return "", ""
	}
	traceCacheMu.Lock()
	traceCache[cacheKey] = grafanaTraceCacheEntry{uid: uid, dsType: dsType, fetchedAt: time.Now()}
	traceCacheMu.Unlock()
	return uid, dsType
}


func shortTraceID(traceID string) string {
	trimmed := strings.TrimSpace(traceID)
	if len(trimmed) <= 8 {
		return trimmed
	}
	return trimmed[:8]
}
