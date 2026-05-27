package incident

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"health-monitor/internal/config"
)

// ObservabilityLinks holds all the observability deep links for an incident
type ObservabilityLinks struct {
	Grafana    string `json:"grafana,omitempty"`
	Prometheus string `json:"prometheus,omitempty"`
	Logs       string `json:"logs,omitempty"`
	Traces     string `json:"traces,omitempty"`
}

// GenerateObservabilityLinks creates deep links for an incident based on config
func GenerateObservabilityLinks(incident Incident, cfg config.Config) ObservabilityLinks {
	links := ObservabilityLinks{}

	// Time window: configurable lookback, to = resolved_at OR now
	lookbackMinutes := cfg.ObservabilityLookbackMinutes
	if lookbackMinutes <= 0 {
		lookbackMinutes = 15 // Default increased from 5 to 15 for better ingestion coverage
	}
	from := incident.CreatedAt.Add(-time.Duration(lookbackMinutes) * time.Minute)
	to := time.Now()
	if incident.State == StateResolved {
		// Find the resolve event to get the resolution time
		for _, event := range incident.Events {
			if event.Type == EventResolve {
				to = event.Timestamp
				break
			}
		}
	}

	fromMs := from.UnixMilli()
	toMs := to.UnixMilli()

	// Generate Grafana dashboard link (requires Prometheus datasource)
	if cfg.GrafanaURL != "" && cfg.GrafanaPromDataSource != "" {
		links.Grafana = generateGrafanaLink(cfg, incident, fromMs, toMs)
	}

	// Generate Prometheus graph link
	if cfg.PrometheusURL != "" {
		links.Prometheus = generatePrometheusLink(cfg, incident, fromMs, toMs)
	}

	// Generate Logs link (Loki via Grafana or Kibana)
	if cfg.GrafanaURL != "" && cfg.GrafanaLokiDataSource != "" {
		links.Logs = generateLokiLink(cfg, incident, fromMs, toMs)
	} else if cfg.KibanaURL != "" {
		links.Logs = generateKibanaLink(cfg, incident, from, to)
	}

	// Generate Traces link (Tempo via Grafana or direct Tempo)
	if cfg.GrafanaURL != "" && cfg.GrafanaTraceDataSource != "" {
		links.Traces = generateTempoLink(cfg, incident, fromMs, toMs)
	} else if cfg.TraceURL != "" {
		links.Traces = generateTraceLink(cfg, incident, fromMs, toMs)
	}

	return links
}

// generateGrafanaLink creates a Grafana Explore deep link (dashboard-independent)
func generateGrafanaLink(cfg config.Config, incident Incident, fromMs, toMs int64) string {
	baseURL := strings.TrimSuffix(cfg.GrafanaURL, "/")
	service := incident.Service
	
	// Use metadata override if available, otherwise config default
	serviceLabel := incident.Metadata["prom_service_label"]
	if serviceLabel == "" {
		serviceLabel = cfg.PrometheusServiceLabel
	}
	if serviceLabel == "" {
		serviceLabel = "service" // Final fallback
	}
	
	// Handle potential comma-separated labels - pick the first one
	if idx := strings.Index(serviceLabel, ","); idx != -1 {
		serviceLabel = serviceLabel[:idx]
	}
	
	// Build query
	var query string
	if incident.Namespace != "" {
		// Robust fallback narrowed by namespace
		query = fmt.Sprintf("{%s=\"%s\", namespace=\"%s\"}", serviceLabel, service, incident.Namespace)
		if serviceLabel != "job" && serviceLabel != "app" {
			query = fmt.Sprintf("{%s=\"%[2]s\", namespace=\"%[3]s\"} or {job=\"%[2]s\", namespace=\"%[3]s\"} or {app=\"%[2]s\", namespace=\"%[3]s\"}", serviceLabel, service, incident.Namespace)
		}
	} else {
		// Original robust approach for bare-metal
		query = fmt.Sprintf("{%s=\"%s\"}", serviceLabel, service)
		if serviceLabel != "job" && serviceLabel != "app" {
			query = fmt.Sprintf("{%s=\"%[2]s\"} or {job=\"%[2]s\"} or {app=\"%[2]s\"}", serviceLabel, service)
		}
	}
	
	// Build the left panel JSON for Grafana explore
	type grafanaQuery struct {
		RefId string `json:"refId"`
		Expr  string `json:"expr"`
	}
	type grafanaRange struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	type grafanaLeft struct {
		Datasource string         `json:"datasource"`
		Queries    []grafanaQuery `json:"queries"`
		Range      grafanaRange   `json:"range"`
	}

	left := grafanaLeft{
		Datasource: cfg.GrafanaPromDataSource,
		Queries:    []grafanaQuery{{RefId: "A", Expr: query}},
		Range:      grafanaRange{From: fmt.Sprintf("%d", fromMs), To: fmt.Sprintf("%d", toMs)},
	}

	leftJSON, _ := json.Marshal(left)
	
	// Get orgId
	orgId := cfg.GrafanaOrgId
	if orgId <= 0 {
		orgId = 1 // Default
	}
	
	return fmt.Sprintf("%s/explore?orgId=%d&left=%s", baseURL, orgId, url.QueryEscape(string(leftJSON)))
}

// generatePrometheusLink creates a Prometheus graph deep link
func generatePrometheusLink(cfg config.Config, incident Incident, fromMs, toMs int64) string {
	baseURL := strings.TrimSuffix(cfg.PrometheusURL, "/")
	service := incident.Service
	
	// Use metadata override if available, otherwise config default
	serviceLabel := incident.Metadata["prom_service_label"]
	if serviceLabel == "" {
		serviceLabel = cfg.PrometheusServiceLabel
	}
	if serviceLabel == "" {
		serviceLabel = "service" // Final fallback
	}
	
	// Handle potential comma-separated labels - pick the first one
	if idx := strings.Index(serviceLabel, ","); idx != -1 {
		serviceLabel = serviceLabel[:idx]
	}
	
	// Build query
	var query string
	if incident.Namespace != "" {
		// Robust fallback narrowed by namespace
		query = fmt.Sprintf("{%s=\"%s\", namespace=\"%s\"}", serviceLabel, service, incident.Namespace)
		if serviceLabel != "job" && serviceLabel != "app" {
			query = fmt.Sprintf("{%s=\"%[2]s\", namespace=\"%[3]s\"} or {job=\"%[2]s\", namespace=\"%[3]s\"} or {app=\"%[2]s\", namespace=\"%[3]s\"}", serviceLabel, service, incident.Namespace)
		}
	} else {
		// Original robust approach for bare-metal
		query = fmt.Sprintf("{%s=\"%s\"}", serviceLabel, service)
		if serviceLabel != "job" && serviceLabel != "app" {
			query = fmt.Sprintf("{%s=\"%[2]s\"} or {job=\"%[2]s\"} or {app=\"%[2]s\"}", serviceLabel, service)
		}
	}
	
	// Calculate time range in seconds for Prometheus
	fromSec := fromMs / 1000
	toSec := toMs / 1000
	duration := toSec - fromSec
	
	// Build URL with time range
	return fmt.Sprintf("%s/graph?g0.expr=%s&g0.tab=graph&g0.range_input=%ds",
		baseURL, url.QueryEscape(query), duration)
}

// generateLokiLink creates a Loki logs deep link via Grafana
func generateLokiLink(cfg config.Config, incident Incident, fromMs, toMs int64) string {
	baseURL := strings.TrimSuffix(cfg.GrafanaURL, "/")
	service := incident.Service
	
	// Use metadata override if available, otherwise config default
	serviceLabel := incident.Metadata["loki_service_label"]
	if serviceLabel == "" {
		serviceLabel = cfg.LokiServiceLabel
	}
	if serviceLabel == "" {
		serviceLabel = "service" // Final fallback
	}
	
	// Create Loki query
	var query string
	if incident.Namespace != "" {
		query = fmt.Sprintf("{%s=\"%s\", namespace=\"%s\"}", serviceLabel, service, incident.Namespace)
	} else {
		// Handle potential comma-separated labels (like "service,app") - pick the first one
		if idx := strings.Index(serviceLabel, ","); idx != -1 {
			serviceLabel = serviceLabel[:idx]
		}
		query = fmt.Sprintf("{%s=\"%s\"}", serviceLabel, service)
	}
	
	// Build the left panel JSON for Grafana explore
	type lokiQuery struct {
		RefId string `json:"refId"`
		Expr  string `json:"expr"`
	}
	type lokiRange struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	type lokiLeft struct {
		Datasource string      `json:"datasource"`
		Queries    []lokiQuery `json:"queries"`
		Range      lokiRange   `json:"range"`
	}

	left := lokiLeft{
		Datasource: cfg.GrafanaLokiDataSource,
		Queries:    []lokiQuery{{RefId: "A", Expr: query}},
		Range:      lokiRange{From: fmt.Sprintf("%d", fromMs), To: fmt.Sprintf("%d", toMs)},
	}

	leftJSON, _ := json.Marshal(left)
	
	// Get orgId
	orgId := cfg.GrafanaOrgId
	if orgId <= 0 {
		orgId = 1 // Default
	}
	
	return fmt.Sprintf("%s/explore?orgId=%d&left=%s",
		baseURL, orgId, url.QueryEscape(string(leftJSON)))
}

// generateKibanaLink creates a Kibana logs deep link
func generateKibanaLink(cfg config.Config, incident Incident, from, to time.Time) string {
	baseURL := strings.TrimSuffix(cfg.KibanaURL, "/")
	service := incident.Service
	
	// Format time correctly for Kibana (ISO 8601)
	fromStr := from.Format("2006-01-02T15:04:05.000Z")
	toStr := to.Format("2006-01-02T15:04:05.000Z")
	
	// Build query for service field
	serviceField := incident.Metadata["elastic_service_field"]
	if serviceField == "" {
		serviceField = cfg.ElasticServiceField
	}
	if serviceField == "" {
		serviceField = "service.name" // Modern ECS fallback
	}
	
	// Get index pattern
	index := incident.Metadata["elastic_index"]
	if index == "" {
		index = cfg.KibanaIndex
	}
	if index == "" {
		index = cfg.ElasticIndex
	}
	if index == "" {
		index = "logs-*"
	}
	
	// Build query components
	var queryParts []string
	queryParts = append(queryParts, fmt.Sprintf("%s:\"%s\"", serviceField, service))
	
	if incident.Namespace != "" {
		queryParts = append(queryParts, fmt.Sprintf("kubernetes.namespace:\"%s\"", incident.Namespace))
	}
	
	query := strings.Join(queryParts, " AND ")
	
	// Build modern Kibana Discover URL
	// We use the 'rison' style encoding if possible, but standard query params are safer for compatibility
	return fmt.Sprintf("%s/app/discover#/?_g=(time:(from:'%s',to:'%s'))&_a=(index:'%s',query:(language:kuery,query:'%s'))",
		baseURL, fromStr, toStr, index, url.QueryEscape(query))
}

// generateTempoLink creates a Tempo traces deep link via Grafana
func generateTempoLink(cfg config.Config, incident Incident, fromMs, toMs int64) string {
	baseURL := strings.TrimSuffix(cfg.GrafanaURL, "/")
	service := incident.Service
	
	serviceTag := cfg.TraceServiceTag
	if serviceTag == "" || serviceTag == "service.name" {
		serviceTag = "resource.service.name" // Default for Tempo (OTel resource attribute)
	}
	
	// Build TraceQL query: MUST have {} braces
	var query string
	if incident.Namespace != "" {
		// Mix service tag with namespace filter
		query = fmt.Sprintf("{%s=\"%s\" && resource.k8s.namespace.name=\"%s\"}", serviceTag, service, incident.Namespace)
	} else {
		query = fmt.Sprintf("{%s=\"%s\"}", serviceTag, service)
	}
	
	// Build the left panel JSON for Grafana explore with Tempo
	type tempoQuery struct {
		RefId string `json:"refId"`
		Query string `json:"query"` // Tempo uses "query" instead of "expr" for TraceQL
	}
	type tempoRange struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	type tempoLeft struct {
		Datasource string       `json:"datasource"`
		Queries    []tempoQuery `json:"queries"`
		Range      tempoRange   `json:"range"`
	}

	left := tempoLeft{
		Datasource: cfg.GrafanaTraceDataSource,
		Queries:    []tempoQuery{{RefId: "A", Query: query}},
		Range:      tempoRange{From: fmt.Sprintf("%d", fromMs), To: fmt.Sprintf("%d", toMs)},
	}

	leftJSON, _ := json.Marshal(left)
	
	// Get orgId
	orgId := cfg.GrafanaOrgId
	if orgId <= 0 {
		orgId = 1 // Default
	}
	
	return fmt.Sprintf("%s/explore?orgId=%d&left=%s",
		baseURL, orgId, url.QueryEscape(string(leftJSON)))
}

// generateTraceLink creates a direct trace backend link (Jaeger/Tempo)
func generateTraceLink(cfg config.Config, incident Incident, fromMs, toMs int64) string {
	baseURL := strings.TrimSuffix(cfg.TraceURL, "/")
	service := incident.Service
	
	// Convert time based on configured unit
	timeUnit := cfg.TraceTimeUnit
	if timeUnit == "" {
		timeUnit = "microseconds" // Default
	}
	
	var fromTime, toTime int64
	switch timeUnit {
	case "microseconds":
		fromTime = fromMs * 1000
		toTime = toMs * 1000
	case "nanoseconds":
		fromTime = fromMs * 1000000
		toTime = toMs * 1000000
	case "milliseconds":
		fromTime = fromMs
		toTime = toMs
	default:
		// Default to microseconds
		fromTime = fromMs * 1000
		toTime = toMs * 1000
	}
	
	// Create trace search URL for service
	return fmt.Sprintf("%s/search?service=%s&start=%d&end=%d&maxResults=20",
		baseURL, url.QueryEscape(service), fromTime, toTime)
}
