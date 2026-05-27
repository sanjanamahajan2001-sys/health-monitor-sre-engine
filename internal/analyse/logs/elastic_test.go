package logs

import (
	"testing"
	"time"

	"health-monitor/internal/config"
)

func TestBuildElasticQueryMissingFields(t *testing.T) {
	cfg := config.Default()
	_, err := buildElasticQuery(cfg, Scope{}, "", time.Now().Add(-time.Minute), time.Now(), 10, 5)
	if err == nil {
		t.Fatal("expected error for missing elastic fields")
	}
}

func TestBuildElasticQueryIncludesFilters(t *testing.T) {
	cfg := config.Default()
	cfg.ElasticTimeField = "@timestamp"
	cfg.ElasticErrorField = "message"
	cfg.ElasticServiceField = "service"
	cfg.ElasticRouteField = "route"
	query, err := buildElasticQuery(cfg, Scope{Service: "api", Route: "/v1"}, "error", time.Now().Add(-time.Minute), time.Now(), 10, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	filters, ok := query["query"].(map[string]interface{})["bool"].(map[string]interface{})["filter"].([]map[string]interface{})
	if !ok || len(filters) == 0 {
		t.Fatal("expected filters in elastic query")
	}
}
