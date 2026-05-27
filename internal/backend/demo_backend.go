package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"health-monitor/internal/flow"
	"health-monitor/internal/slo"
)

// DemoBackend provides fake metrics, logs, and traces for demo mode
type DemoBackend struct {
	mu     sync.RWMutex
	data   *DemoData
	started time.Time
}

type DemoData struct {
	Services []DemoService
	Flows    []flow.Flow
	SLOs     []slo.SLOResult
	Logs     []DemoLog
	Traces   []DemoTrace
}

type DemoService struct {
	Name        string
	Status      string
	Metrics     map[string]float64
	LastUpdated time.Time
}

type DemoLog struct {
	Timestamp time.Time
	Service   string
	Level     string
	Message   string
	TraceID   string
}

type DemoTrace struct {
	TraceID   string
	Service   string
	Operation string
	Duration  time.Duration
	Status    string
	Spans     []DemoSpan
}

type DemoSpan struct {
	SpanID    string
	ParentID  string
	Operation string
	Service   string
	Duration  time.Duration
	Status    string
}

// NewDemoBackend creates a new demo backend with seeded data
func NewDemoBackend() *DemoBackend {
	return &DemoBackend{
		started: time.Now(),
		data:     generateDemoData(),
	}
}

func generateDemoData() *DemoData {
	now := time.Now()
	data := &DemoData{
		Services: []DemoService{
			{
				Name:   "checkout",
				Status: "degraded",
				Metrics: map[string]float64{
					"availability":           98.5,
					"latency_p99":           850.0,
					"error_rate":            2.1,
					"request_rate":          450.0,
					"connection_pool_usage": 85.0,
				},
				LastUpdated: now.Add(-5 * time.Minute),
			},
			{
				Name:   "auth",
				Status: "healthy",
				Metrics: map[string]float64{
					"availability":    99.9,
					"latency_p99":     120.0,
					"error_rate":      0.1,
					"request_rate":    200.0,
					"success_rate":    99.9,
				},
				LastUpdated: now.Add(-2 * time.Minute),
			},
			{
				Name:   "billing",
				Status: "unhealthy",
				Metrics: map[string]float64{
					"availability":          95.2,
					"latency_p99":          2100.0,
					"error_rate":           8.5,
					"request_rate":         150.0,
					"timeout_rate":         12.0,
				},
				LastUpdated: now.Add(-1 * time.Minute),
			},
		},
		Flows: []flow.Flow{
			{
				ID:   "checkout-flow",
				Name: "Checkout Process",
				Services: []string{"checkout", "auth", "billing"},
				SLOs: []flow.SLO{
					{
						ID:               "checkout-availability",
						Service:          "checkout",
						Objective:        99.9,
						Window:           "30d",
						Type:             "availability",
						ErrorQuery:       "sum(rate(http_requests_total{service=\"checkout\",code!~\"2..\"}[5m]))",
						TotalQuery:       "sum(rate(http_requests_total{service=\"checkout\"}[5m]))",
						ThresholdSeconds: 0,
					},
				},
				Metadata: map[string]string{
					"owner":       "checkout-team",
					"criticality": "high",
				},
			},
			{
				ID:   "auth-flow",
				Name: "Authentication Process",
				Services: []string{"auth"},
				SLOs: []flow.SLO{
					{
						ID:               "auth-latency",
						Service:          "auth",
						Objective:        95.0,
						Window:           "7d",
						Type:             "latency",
						ErrorQuery:       "histogram_quantile(0.99, sum(rate(http_request_duration_seconds_bucket{service=\"auth\"}[5m])) by (le))",
						TotalQuery:       "sum(rate(http_request_duration_seconds_count{service=\"auth\"}[5m]))",
						ThresholdSeconds: 0.5,
					},
				},
				Metadata: map[string]string{
					"owner":       "auth-team",
					"criticality": "critical",
				},
			},
			{
				ID:   "order-fulfillment",
				Name: "Order Fulfillment Flow",
				Services: []string{"warehouse", "shipping"},
				SLOs: []flow.SLO{
					{
						ID:               "shipping-success",
						Service:          "shipping",
						Objective:        99.9,
						Window:           "7d",
						Type:             "availability",
						ErrorQuery:       "sum(rate(shipping_errors_total[5m]))",
						TotalQuery:       "sum(rate(shipping_requests_total[5m]))",
						ThresholdSeconds: 0,
					},
				},
				Metadata: map[string]string{
					"owner":       "fulfillment-team",
					"criticality": "high",
				},
			},
		},
		SLOs: []slo.SLOResult{
			{
				FlowID:          "checkout-flow",
				SLOID:           "checkout-availability",
				Service:         "checkout",
				Objective:       99.9,
				Window:          "30d",
				Type:            "availability",
				Current:         98.5,
				BurnRate:        2.5,
				BurnRateSlow:    1.2,
				BudgetRemaining: 15.2,
				Trend:           "⬇️",
				Status:          slo.StatusBreaching,
				Compliance:      "BREACHING",
				Risk:            "CRITICAL",
			},
			{
				FlowID:          "auth-flow",
				SLOID:           "auth-latency",
				Service:         "auth",
				Objective:       95.0,
				Window:          "7d",
				Type:            "latency",
				Current:         97.8,
				BurnRate:        0.3,
				BurnRateSlow:    0.2,
				BudgetRemaining: 85.5,
				Trend:           "➡️",
				Status:          slo.StatusHealthy,
				Compliance:      "COMPLIANT",
				Risk:            "LOW",
			},
			{
				FlowID:          "order-fulfillment",
				SLOID:           "shipping-success",
				Service:         "shipping",
				Objective:       99.9,
				Window:          "7d",
				Type:            "availability",
				Current:         99.99,
				BurnRate:        0.05,
				BurnRateSlow:    0.02,
				BudgetRemaining: 99.5,
				Trend:           "🟢",
				Status:          slo.StatusHealthy,
				Compliance:      "COMPLIANT",
				Risk:            "LOW",
			},
		},
		Logs: generateDemoLogs(now),
		Traces: generateDemoTraces(now),
	}
	
	return data
}

func generateDemoLogs(now time.Time) []DemoLog {
	var logs []DemoLog
	
	// Checkout error logs
	for i := 0; i < 15; i++ {
		logs = append(logs, DemoLog{
			Timestamp: now.Add(-time.Duration(i*5) * time.Minute),
			Service:   "checkout",
			Level:     "ERROR",
			Message:   fmt.Sprintf("Payment gateway timeout: stripe_api took %dms", 5000+rand.Intn(3000)),
			TraceID:   fmt.Sprintf("trace-checkout-%d", i),
		})
	}
	
	// Auth logs
	for i := 0; i < 8; i++ {
		logs = append(logs, DemoLog{
			Timestamp: now.Add(-time.Duration(i*10) * time.Minute),
			Service:   "auth",
			Level:     "INFO",
			Message:   "User authentication successful",
			TraceID:   fmt.Sprintf("trace-auth-%d", i),
		})
	}
	
	// Billing error logs
	for i := 0; i < 12; i++ {
		logs = append(logs, DemoLog{
			Timestamp: now.Add(-time.Duration(i*3) * time.Minute),
			Service:   "billing",
			Level:     "ERROR",
			Message:   fmt.Sprintf("Invoice generation failed: database connection timeout after %dms", 30000+rand.Intn(10000)),
			TraceID:   fmt.Sprintf("trace-billing-%d", i),
		})
	}
	
	return logs
}

func generateDemoTraces(now time.Time) []DemoTrace {
	traces := []DemoTrace{
		{
			TraceID:   "trace-checkout-failed",
			Service:   "checkout",
			Operation: "POST /checkout/complete",
			Duration:  850 * time.Millisecond,
			Status:    "error",
			Spans: []DemoSpan{
				{
					SpanID:    "span-1",
					ParentID:  "",
					Operation: "auth.validate_token",
					Service:   "auth",
					Duration:  50 * time.Millisecond,
					Status:    "ok",
				},
				{
					SpanID:    "span-2",
					ParentID:  "span-1",
					Operation: "billing.process_payment",
					Service:   "billing",
					Duration:  750 * time.Millisecond,
					Status:    "error",
				},
				{
					SpanID:    "span-3",
					ParentID:  "span-2",
					Operation: "checkout.create_order",
					Service:   "checkout",
					Duration:  50 * time.Millisecond,
					Status:    "ok",
				},
			},
		},
		{
			TraceID:   "trace-auth-success",
			Service:   "auth",
			Operation: "POST /auth/login",
			Duration:  120 * time.Millisecond,
			Status:    "ok",
			Spans: []DemoSpan{
				{
					SpanID:    "span-4",
					ParentID:  "",
					Operation: "auth.validate_credentials",
					Service:   "auth",
					Duration:  100 * time.Millisecond,
					Status:    "ok",
				},
				{
					SpanID:    "span-5",
					ParentID:  "span-4",
					Operation: "auth.generate_token",
					Service:   "auth",
					Duration:  20 * time.Millisecond,
					Status:    "ok",
				},
			},
		},
	}
	
	return traces
}

// Demo metrics interface
func (db *DemoBackend) GetMetrics(ctx context.Context, service string) (map[string]float64, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	
	for _, s := range db.data.Services {
		if s.Name == service {
			// Add some realistic variation
			result := make(map[string]float64)
			for k, v := range s.Metrics {
				// Add small random variations to simulate live data
				variation := 1.0 + (rand.Float64()-0.5)*0.1 // ±5% variation
				result[k] = v * variation
			}
			return result, nil
		}
	}
	
	return nil, fmt.Errorf("service %s not found", service)
}

// Demo logs interface
func (db *DemoBackend) GetLogs(ctx context.Context, service string, limit int) ([]DemoLog, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	
	var filtered []DemoLog
	for _, log := range db.data.Logs {
		if log.Service == service {
			filtered = append(filtered, log)
			if len(filtered) >= limit {
				break
			}
		}
	}
	
	return filtered, nil
}

// Demo traces interface
func (db *DemoBackend) GetTraces(ctx context.Context, service string, limit int) ([]DemoTrace, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	
	var filtered []DemoTrace
	for _, trace := range db.data.Traces {
		if trace.Service == service {
			filtered = append(filtered, trace)
			if len(filtered) >= limit {
				break
			}
		}
	}
	
	return filtered, nil
}

// Demo flows interface
func (db *DemoBackend) GetFlows(ctx context.Context) ([]flow.Flow, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	
	return db.data.Flows, nil
}

// Demo SLOs interface
func (db *DemoBackend) GetSLOs(ctx context.Context) ([]slo.SLOResult, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	
	// Add some time-based variation to simulate live monitoring
	results := make([]slo.SLOResult, len(db.data.SLOs))
	for i, slo := range db.data.SLOs {
		results[i] = slo
		// Simulate small changes in current values
		results[i].Current = slo.Current + (rand.Float64()-0.5)*0.5
		results[i].BudgetRemaining = slo.BudgetRemaining - rand.Float64()*0.1
	}
	
	return results, nil
}

// ExportDemoData exports all demo data as JSON for external consumption
func (db *DemoBackend) ExportDemoData() ([]byte, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	
	return json.MarshalIndent(db.data, "", "  ")
}
