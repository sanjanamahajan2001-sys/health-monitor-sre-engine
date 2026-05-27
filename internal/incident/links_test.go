package incident

import (
	"testing"
	"time"

	"health-monitor/internal/config"
)

func TestGenerateObservabilityLinks(t *testing.T) {
	// Create a test incident
	incident := Incident{
		ID:        "INC-20231201-120000",
		Service:   "billing-api",
		Severity:  P1,
		Title:     "High error rate in billing API",
		State:     StateStarted,
		CreatedAt: time.Date(2023, 12, 1, 12, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2023, 12, 1, 12, 30, 0, 0, time.UTC),
	}

	// Test with full config
	cfg := config.Config{
		GrafanaURL:               "https://grafana.example.com",
		GrafanaLokiDataSource:     "Loki",
		GrafanaTraceDataSource:   "Tempo",
		PrometheusURL:            "https://prometheus.example.com",
		LokiURL:                  "https://loki.example.com",
		TraceURL:                 "https://tempo.example.com",
		KibanaURL:                "https://kibana.example.com",
		ElasticServiceField:      "service",
	}

	links := GenerateObservabilityLinks(incident, cfg)

	// Verify Grafana link
	if links.Grafana == "" {
		t.Error("Expected Grafana link to be generated")
	}
	if !contains(links.Grafana, "grafana.example.com") {
		t.Error("Grafana link should contain Grafana URL")
	}
	if !contains(links.Grafana, "billing-api") {
		t.Error("Grafana link should contain service name")
	}

	// Verify Prometheus link
	if links.Prometheus == "" {
		t.Error("Expected Prometheus link to be generated")
	}
	if !contains(links.Prometheus, "prometheus.example.com") {
		t.Error("Prometheus link should contain Prometheus URL")
	}
	if !contains(links.Prometheus, "billing-api") {
		t.Error("Prometheus link should contain service name")
	}

	// Verify Logs link (should prefer Grafana+Loki over Kibana)
	if links.Logs == "" {
		t.Error("Expected Logs link to be generated")
	}
	if !contains(links.Logs, "grafana.example.com") {
		t.Error("Logs link should prefer Grafana when Loki datasource is available")
	}

	// Verify Traces link (should prefer Grafana+Tempo over direct Tempo)
	if links.Traces == "" {
		t.Error("Expected Traces link to be generated")
	}
	if !contains(links.Traces, "grafana.example.com") {
		t.Error("Traces link should prefer Grafana when Tempo datasource is available")
	}
}

func TestGenerateObservabilityLinksWithoutGrafana(t *testing.T) {
	incident := Incident{
		ID:        "INC-20231201-120000",
		Service:   "billing-api",
		Severity:  P1,
		Title:     "High error rate in billing API",
		State:     StateStarted,
		CreatedAt: time.Date(2023, 12, 1, 12, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2023, 12, 1, 12, 30, 0, 0, time.UTC),
	}

	// Test with only direct URLs (no Grafana)
	cfg := config.Config{
		PrometheusURL:       "https://prometheus.example.com",
		LokiURL:             "https://loki.example.com",
		TraceURL:            "https://tempo.example.com",
		KibanaURL:           "https://kibana.example.com",
		ElasticServiceField: "service",
	}

	links := GenerateObservabilityLinks(incident, cfg)

	// Grafana should be empty
	if links.Grafana != "" {
		t.Error("Expected Grafana link to be empty when Grafana URL is not configured")
	}

	// Prometheus should still work
	if links.Prometheus == "" {
		t.Error("Expected Prometheus link to be generated")
	}

	// Logs should use Kibana when Grafana is not available
	if links.Logs == "" {
		t.Error("Expected Logs link to be generated using Kibana")
	}
	if !contains(links.Logs, "kibana.example.com") {
		t.Error("Logs link should use Kibana when Grafana is not available")
	}

	// Traces should use direct Tempo URL when Grafana is not available
	if links.Traces == "" {
		t.Error("Expected Traces link to be generated using direct Tempo URL")
	}
	if !contains(links.Traces, "tempo.example.com") {
		t.Error("Traces link should use direct Tempo URL when Grafana is not available")
	}
}

func TestGenerateObservabilityLinksEmptyConfig(t *testing.T) {
	incident := Incident{
		ID:        "INC-20231201-120000",
		Service:   "billing-api",
		Severity:  P1,
		Title:     "High error rate in billing API",
		State:     StateStarted,
		CreatedAt: time.Date(2023, 12, 1, 12, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2023, 12, 1, 12, 30, 0, 0, time.UTC),
	}

	// Test with empty config
	cfg := config.Config{}

	links := GenerateObservabilityLinks(incident, cfg)

	// All links should be empty
	if links.Grafana != "" || links.Prometheus != "" || links.Logs != "" || links.Traces != "" {
		t.Error("Expected all links to be empty when config is empty")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && 
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || 
		 indexOf(s, substr) >= 0))
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
