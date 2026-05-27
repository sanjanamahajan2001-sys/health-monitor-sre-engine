package tracing

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"health-monitor/pkg/model"
)

func TestTempoSearchAndFetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/search":
			writeJSON(w, map[string]any{
				"traces": []map[string]any{
					{"traceID": "abc123", "durationMs": 2100},
				},
			})
		case "/api/traces/abc123":
			writeJSON(w, map[string]any{
				"data": []map[string]any{
					{
						"traceID": "abc123",
						"spans": []map[string]any{
							{
								"traceID":       "abc123",
								"spanID":        "root",
								"operationName": "POST /orders",
								"startTime":     int64(1000),
								"duration":      int64(2100000),
								"processID":     "p1",
							},
							{
								"traceID":       "abc123",
								"spanID":        "db",
								"operationName": "SELECT orders",
								"startTime":     int64(1200),
								"duration":      int64(1900000),
								"processID":     "p2",
								"references": []map[string]any{
									{"refType": "CHILD_OF", "spanID": "root"},
								},
							},
						},
						"processes": map[string]any{
							"p1": map[string]any{"serviceName": "order-api"},
							"p2": map[string]any{"serviceName": "postgres"},
						},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := Client{
		BaseURL: server.URL,
		Timeout: 2 * time.Second,
		QPS:     5,
	}
	traces, err := client.TempoSearch("order-api", 4*time.Minute, 3)
	if err != nil || len(traces) == 0 {
		t.Fatalf("TempoSearch failed: %v", err)
	}
	trace, err := client.TempoFetchTrace("abc123")
	if err != nil {
		t.Fatalf("TempoFetchTrace failed: %v", err)
	}
	if strings.TrimSpace(trace.RootSpan.Name) == "" || strings.TrimSpace(trace.SlowestSpan.Name) == "" {
		t.Fatalf("expected root and slowest spans")
	}
}

func TestJaegerSearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/traces") {
			writeJSON(w, map[string]any{
				"data": []map[string]any{
					{
						"traceID": "t1",
						"spans": []map[string]any{
							{"traceID": "t1", "spanID": "s1", "operationName": "GET /health", "startTime": int64(1), "duration": int64(1000), "processID": "p1"},
						},
						"processes": map[string]any{
							"p1": map[string]any{"serviceName": "api"},
						},
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := Client{
		BaseURL: server.URL,
		Timeout: 2 * time.Second,
		QPS:     5,
	}
	traces, err := client.JaegerSearch("api", 4*time.Minute, 3)
	if err != nil || len(traces) == 0 {
		t.Fatalf("JaegerSearch failed: %v", err)
	}
	if traces[0].RootSpan.Service == "" {
		t.Fatalf("expected service on root span")
	}
}

func TestResolveTraceServiceMapping(t *testing.T) {
	cfg := TraceConfig{
		TraceServiceMap: "order_API=order-api",
	}
	latency := &model.APILatency{Service: "order_API"}
	selection := ResolveTraceService(cfg, latency, nil)
	if selection.Service != "order-api" {
		t.Fatalf("expected mapped service, got %q", selection.Service)
	}
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}
