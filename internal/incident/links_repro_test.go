package incident

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"

	"health-monitor/internal/config"
)

func TestTempoQuerySyntax(t *testing.T) {
	incident := Incident{
		ID:        "INC-20231201-120000",
		Service:   "orders_service",
		Severity:  P1,
		Title:     "High error rate in orders service",
		State:     StateStarted,
		CreatedAt: time.Date(2023, 12, 1, 12, 0, 0, 0, time.UTC),
	}

	cfg := config.Config{
		GrafanaURL:             "http://apac.monitor.local:3001",
		GrafanaTraceDataSource: "Tempo",
	}

	links := GenerateObservabilityLinks(incident, cfg)

	// Parse the Tempo link
	u, err := url.Parse(links.Traces)
	if err != nil {
		t.Fatalf("Failed to parse Traces URL: %v", err)
	}

	leftParam := u.Query().Get("left")
	if leftParam == "" {
		t.Fatal("Missing 'left' parameter in Traces URL")
	}

	var left struct {
		Queries []struct {
			Query string `json:"query"`
		} `json:"queries"`
	}
	if err := json.Unmarshal([]byte(leftParam), &left); err != nil {
		t.Fatalf("Failed to unmarshal 'left' JSON: %v", err)
	}

	if len(left.Queries) == 0 {
		t.Fatal("No queries found in 'left' parameter")
	}

	query := left.Queries[0].Query
	t.Logf("Generated Tempo query: %s", query)

	// Current (buggy) behavior: {service.name="orders_service"}
	// Desired behavior: {resource.service.name="orders_service"}
	expectedPrefix := "{resource.service.name="
	if !contains(query, expectedPrefix) {
		t.Errorf("Expected Tempo query to contain %q, but got %q", expectedPrefix, query)
	}
}

func TestPrometheusQueryLabels(t *testing.T) {
	incident := Incident{
		ID:        "INC-20231201-120000",
		Service:   "orders_service",
		Severity:  P1,
		Title:     "High error rate",
		State:     StateStarted,
		CreatedAt: time.Date(2023, 12, 1, 12, 0, 0, 0, time.UTC),
	}

	// Case 1: Default label
	cfg1 := config.Config{
		PrometheusURL:          "http://prometheus.local",
		PrometheusServiceLabel: "service",
	}
	links1 := GenerateObservabilityLinks(incident, cfg1)
	expected1 := "{service=\"orders_service\"} or {job=\"orders_service\"} or {app=\"orders_service\"}"
	if !contains(links1.Prometheus, url.QueryEscape(expected1)) {
		t.Errorf("Expected Prometheus link to contain escaped multi-label query, link: %s", links1.Prometheus)
	}

	// Case 2: Configured label (non-standard)
	cfg2 := config.Config{
		PrometheusURL:          "http://prometheus.local",
		PrometheusServiceLabel: "service_name",
	}
	links2 := GenerateObservabilityLinks(incident, cfg2)
	expected2 := "{service_name=\"orders_service\"} or {job=\"orders_service\"} or {app=\"orders_service\"}"
	if !contains(links2.Prometheus, url.QueryEscape(expected2)) {
		t.Errorf("Expected configured Prometheus link to contain escaped multi-label query, link: %s", links2.Prometheus)
	}
}

func TestMetadataLabelOverrides(t *testing.T) {
	incident := Incident{
		ID:       "INC-20231201-120000",
		Service:  "orders_service",
		Metadata: map[string]string{
			"prom_service_label": "app_name",
			"loki_service_label": "container_name",
		},
	}

	cfg := config.Config{
		PrometheusURL:          "http://prometheus.local",
		PrometheusServiceLabel: "service", // Should be overridden
		GrafanaURL:             "http://grafana.local",
		GrafanaLokiDataSource:  "Loki",
	}

	links := GenerateObservabilityLinks(incident, cfg)

	// Check Prometheus override
	expectedProm := "{app_name=\"orders_service\"} or {job=\"orders_service\"} or {app=\"orders_service\"}"
	if !contains(links.Prometheus, url.QueryEscape(expectedProm)) {
		t.Errorf("Expected Prometheus link to use multi-label query with 'app_name' from metadata, link: %s", links.Prometheus)
	}

	// Check Loki override
	if !contains(links.Logs, "container_name%3D%5C%22orders_service%5C%22") {
		t.Errorf("Expected Loki link to use 'container_name' from metadata, link: %s", links.Logs)
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
