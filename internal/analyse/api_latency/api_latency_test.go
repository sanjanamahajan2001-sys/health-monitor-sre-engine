package api_latency

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"health-monitor/internal/analyse/prometheus"
	"health-monitor/internal/config"
)

func TestAutoDetectChoosesSharedLabels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/label/__name__/values":
			writeJSON(w, map[string]any{
				"status": "success",
				"data":   []string{"http_request_duration_seconds_bucket", "http_requests_total"},
			})
		case "/api/v1/series":
			match := strings.Join(r.URL.Query()["match[]"], ",")
			var series []map[string]string
			switch {
			case strings.Contains(match, "http_request_duration_seconds_bucket"):
				series = []map[string]string{
					{"__name__": "http_request_duration_seconds_bucket", "service": "api", "path": "/health", "le": "0.5"},
				}
			case strings.Contains(match, "http_requests_total"):
				series = []map[string]string{
					{"__name__": "http_requests_total", "service": "api", "path": "/health", "status": "200"},
				}
			default:
				series = []map[string]string{
					{"__name__": "http_requests_total", "service": "api", "path": "/health", "status": "200"},
				}
			}
			writeJSON(w, map[string]any{
				"status": "success",
				"data":   series,
			})
		case "/api/v1/query":
			query := r.URL.Query().Get("query")
			result := []map[string]any{
				{
					"metric": map[string]string{"path": "/health"},
					"value":  []any{float64(time.Now().Unix()), "1"},
				},
			}
			if strings.Contains(query, "ALERTS") {
				result = []map[string]any{}
			}
			writeJSON(w, map[string]any{
				"status": "success",
				"data": map[string]any{
					"resultType": "vector",
					"result":     result,
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := config.Default()
	cfg.PrometheusURL = server.URL
	cfg.AutoDiscover = true
	cfg.LatencyMetric = ""
	cfg.RequestCountMetric = ""
	cfg.ServiceLabel = ""
	cfg.RouteLabel = ""

	client := prometheus.Client{
		BaseURL: server.URL,
		Timeout: 2 * time.Second,
		QPS:     10,
	}

	result, err := Collect(&client, cfg)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}
	if result.LatencyMetric == "" || result.RequestMetric == "" {
		t.Fatalf("expected metrics to be auto-detected, got latency=%q request=%q", result.LatencyMetric, result.RequestMetric)
	}
	if result.ServiceLabel == "" || result.RouteLabel == "" {
		t.Fatalf("expected shared labels, got service=%q route=%q", result.ServiceLabel, result.RouteLabel)
	}
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}
