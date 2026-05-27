package commands

import (
	"os"
	"path/filepath"
	"time"

	"health-monitor/internal/flow"
	"health-monitor/internal/incident"
	"health-monitor/internal/traces"
)

// SeedDemoData writes mock incidents to demonstrate system features
func SeedDemoData(store *incident.Store, sandboxDir string) error {
	now := time.Now()

	// Seed flows first in the sandbox
	flowsDir := filepath.Join(sandboxDir, "config", "flows.d")
	os.MkdirAll(flowsDir, 0755)
	if err := seedDemoFlows(flowsDir); err != nil {
		return err
	}

	// Seed SLOs
	if err := seedDemoSLOs(); err != nil {
		return err
	}

	// 1. Auto Creation (Suggested State) - SLO Breach Simulation
	sloInc := incident.Incident{
		ID:        "INC-DEMO-SLO",
		Title:     "SLO Breach: Checkout availability dropped to 98.5%",
		State:     incident.StateSuggested,
		Severity:  incident.P2,
		Service:   "checkout",
		Profile:   "demo",
		CreatedAt: now.Add(-15 * time.Minute),
		UpdatedAt: now.Add(-15 * time.Minute),
		Summary:   "Automatic detection of checkout availability drop. Metrics show increased 5xx errors from checkout-service.",
		Events: []incident.Event{
			{
				Type:      incident.EventSuggest,
				User:      "SLO Monitor",
				Timestamp: now.Add(-15 * time.Minute),
				Message:   "Auto-detected SLO availability breach: 98.5% (Threshold: 99.9%)",
			},
		},
		// Add fake logs data for SLO breach
		Logs: &incident.LogSummary{
			Backend: "demo",
			Window:  "15m",
			TopErrors: []incident.ErrorStat{
				{
					Message: "checkout service unavailable",
					Count:   234,
					Sample:  "connection refused to checkout service",
				},
				{
					Message: "payment gateway timeout",
					Count:   156,
					Sample:  "timeout waiting for payment gateway response",
				},
			},
		},
		// Add fake traces data
		Traces: &traces.TraceSummary{
			TraceCount:     5678,
			ErrorCount:     390,
			P95LatencyMs:   4560,
			SlowestRoute:   "checkout_service -> payment_gateway",
			GrafanaURL:     "http://demo-grafana.local/d/checkout",
			TopSpans: []traces.SpanInfo{
				{
					Name:        "checkout.process",
					DurationMs:  4200,
					Service:     "checkout_service",
					ErrorCount:  234,
				},
				{
					Name:        "payment.gateway.call",
					DurationMs:  3800,
					Service:     "checkout_service",
					ErrorCount:  156,
				},
			},
		},
		// Add observability links
		Links: incident.ObservabilityLinks{
			Grafana:    "http://demo-grafana.local/d/checkout-slo",
			Prometheus: "http://demo-prometheus.local/graph?g0.expr=checkout_availability",
			Logs:       "http://demo-loki.local/explore?query=checkout",
			Traces:     "http://demo-jaeger.local/trace?service=checkout_service",
		},
		// Add metadata for runbook suggestions and RCA
		Metadata: map[string]string{
			"runbook_suggestion":    "Checkout Service Outage Runbook",
			"runbook_pattern":       "service_availability_breach",
			"suspected_cause":       "Upstream dependency 'payment_gateway' is timing out",
			"trend":                 "Increasing error rate over last 10m",
		},
	}
	store.Save(sloInc)

	// 2. Billing Service Payment Failures (Started State) with RCA and Action Items
	billingInc := incident.Incident{
		ID:        "INC-DEMO-BILLING",
		Title:     "Billing service payment processing failures",
		State:     incident.StateStarted,
		Severity:  incident.P1,
		Service:   "billing_service",
		Profile:   "demo",
		CreatedAt: now.Add(-3 * time.Hour),
		UpdatedAt: now.Add(-30 * time.Minute),
		Summary:   "Payment processing failures due to Stripe API rate limiting and timeout issues. Customer checkout flow is heavily impacted.",
		Events: []incident.Event{
			{
				Type:      incident.EventStart,
				User:      "AlertManager",
				Timestamp: now.Add(-3 * time.Hour),
			},
			{
				Type:      incident.EventNote,
				User:      "billing_team",
				Timestamp: now.Add(-2 * time.Hour),
				Message:   "Investigating Stripe API response times - seeing 429 Too Many Requests",
			},
			{
				Type:      incident.EventNote,
				User:      "oncall_engineer",
				Timestamp: now.Add(-30 * time.Minute),
				Message:   "Implemented temporary circuit breaker; investigating secondary provider",
			},
		},
		// Add fake logs data
		Logs: &incident.LogSummary{
			Backend: "demo",
			Window:  "30m",
			TopErrors: []incident.ErrorStat{
				{
					Message: "stripe api rate limit exceeded",
					Count:   456,
					Sample:  "Stripe API error: rate_limit_exceeded (429)",
				},
				{
					Message: "payment processing timeout",
					Count:   234,
					Sample:  "context deadline exceeded while processing payment",
				},
			},
		},
		// Add fake traces data
		Traces: &traces.TraceSummary{
			TraceCount:     8934,
			ErrorCount:     879,
			P95LatencyMs:   3450,
			SlowestRoute:   "billing_service -> stripe_api",
			GrafanaURL:     "http://demo-grafana.local/d/billing",
			TopSpans: []traces.SpanInfo{
				{
					Name:        "stripe.charge.create",
					DurationMs:  3200,
					Service:     "billing_service",
					ErrorCount:  456,
				},
			},
		},
		// Add observability links
		Links: incident.ObservabilityLinks{
			Grafana:    "http://demo-grafana.local/d/billing-dashboard",
			Prometheus: "http://demo-prometheus.local/graph?g0.expr=billing_errors",
			Logs:       "http://demo-loki.local/explore?query=billing",
			Traces:     "http://demo-jaeger.local/trace?service=billing_service",
		},
		// Add impact data
		Impact: &incident.ImpactMetrics{
			EstimatedDowntimeMinutes: 45,
			ImpactedFlows:            []string{"checkout-flow", "payment-flow"},
			CustomMetrics:            "Revenue lost: ~$15,000/hour, Users affected: 1,250",
		},
		ActionItems: []incident.ActionItem{
			{
				ID:          "AI-BILLING-1",
				Description: "Implement intelligent backoff and retry for Stripe requests",
				Owner:       "Billing Team",
				Status:      incident.ActionItemInProgress,
				Priority:    incident.ActionPriorityP1,
			},
			{
				ID:          "AI-BILLING-2",
				Description: "Review Stripe API usage quota and upgrade if necessary",
				Owner:       "Infrastructure",
				Status:      incident.ActionItemTODO,
				Priority:    incident.ActionPriorityP2,
			},
		},
		Metadata: map[string]string{
			"suspected_cause":    "Stripe API rate limits exceeded due to unexpected traffic spike",
			"remediation_hint":   "Check traffic patterns and consider side-channel payment provider",
		},
	}
	store.Save(billingInc)

	// 3. Manual Creation (Started State) - Auth service
	authInc := incident.Incident{
		ID:        "INC-DEMO-AUTH",
		Title:     "Authentication service latency degradation",
		State:     incident.StateStarted,
		Severity:  incident.P3,
		Service:   "auth",
		Profile:   "demo",
		CreatedAt: now.Add(-2 * time.Hour),
		UpdatedAt: now.Add(-1 * time.Hour),
		Summary:   "Customer support escalated multiple tickets regarding slow login times. Metrics confirm p99 latency spike to 4s.",
		Events: []incident.Event{
			{
				Type:      incident.EventStart,
				User:      "jane_support",
				Timestamp: now.Add(-2 * time.Hour),
				Message:   "Manually created incident from customer complaints about slow logins",
			},
			{
				Type:      incident.EventNote,
				User:      "auth_team",
				Timestamp: now.Add(-90 * time.Minute),
				Message:   "Investigating auth_db connection pool saturation",
			},
		},
		ActionItems: []incident.ActionItem{
			{
				ID:          "AI-AUTH-1",
				Description: "Optimize 'validate_session' query indexes",
				Owner:       "DBA Team",
				Status:      incident.ActionItemTODO,
				Priority:    incident.ActionPriorityP2,
			},
		},
		Metadata: map[string]string{
			"suspected_cause": "Database connection pool exhaustion on auth-master",
			"confidence":      "Medium",
		},
	}
	store.Save(authInc)

	// 4. Deduplication Scenario
	checkoutInc := incident.Incident{
		ID:        "INC-DEMO-CHECKOUT",
		Title:     "Checkout service p95 regression",
		State:     incident.StateAcknowledged,
		Severity:  incident.P1,
		Service:   "checkout",
		Profile:   "demo",
		CreatedAt: now.Add(-3 * time.Hour),
		UpdatedAt: now.Add(-1 * time.Hour),
		Summary:   "Checkout latency increased by 300% following canary deployment of v2.4.1. Rollback initiated.",
		Events: []incident.Event{
			{
				Type:      incident.EventSuggest,
				User:      "SLO Monitor",
				Timestamp: now.Add(-3 * time.Hour),
				Message:   "SLO breach: checkout latency regression detected",
			},
			{
				Type:      incident.EventAck,
				User:      "oncall_engineer",
				Timestamp: now.Add(-1 * time.Hour),
				Message:   "Acknowledged - canary deployment rollback in progress",
			},
		},
		Impact: &incident.ImpactMetrics{
			EstimatedDowntimeMinutes: 30,
			ImpactedFlows:            []string{"checkout-flow"},
			CustomMetrics:            "Conversion rate dropped by 12%",
		},
		Metadata: map[string]string{
			"suspected_cause": "Inefficient JSON parsing in new canary build v2.4.1",
			"regression":      "3.2s vs baseline 0.8s",
		},
	}
	store.Save(checkoutInc)

	// 5. Historical (Resolved State) with RCA and Action Items
	resolvedInc := incident.Incident{
		ID:        "INC-DEMO-HIST",
		Title:     "Database connection pool exhaustion",
		State:     incident.StateResolved,
		Severity:  incident.P1,
		Service:   "postgres_db",
		Profile:   "demo",
		CreatedAt: now.Add(-48 * time.Hour),
		UpdatedAt: now.Add(-24 * time.Hour),
		Summary:   "A bad deployment in 'user-profile' service caused connection leaks, eventually exhausting the main RDS cluster pool.",
		Events: []incident.Event{
			{
				Type:      incident.EventStart,
				User:      "AlertManager",
				Timestamp: now.Add(-48 * time.Hour),
			},
			{
				Type:      incident.EventResolve,
				User:      "oncall_engineer",
				Timestamp: now.Add(-24 * time.Hour),
				Message:   "Rolled back user-profile service and cycled DB connections",
			},
		},
		Analysis: &incident.IncidentAnalysis{
			RootCause:  "A missing 'defer rows.Close()' in the new profile lookup logic caused unreleased connections to accumulate under load.",
			FixSummary: "Rolled back to v1.2.3. Added static analysis check to CI/CD to catch unclosed SQL resources.",
			Prevention: "Implemented connection leak detection alerts and forced timeouts on idle connections.",
			LessonsLearned: &incident.LessonsLearned{
				WhatWentWell:      "Automated detection caught the trend before full service collapse.",
				WhatCouldBeBetter: "Rollback procedure for this specific service was slow (15m).",
			},
		},
		ActionItems: []incident.ActionItem{
			{
				ID:          "AI-HIST-1",
				Description: "Enable 'sql-rows-check' in linter",
				Owner:       "Backend Team",
				Status:      incident.ActionItemDone,
				Priority:    incident.ActionPriorityP1,
			},
		},
	}
	store.Save(resolvedInc)

	// 6. Active DB Latency for Similarity triggering
	activeDbInc := incident.Incident{
		ID:        "INC-DEMO-DB-SPIKE",
		Title:     "Latency spike in Postgres DB queries",
		State:     incident.StateStarted,
		Severity:  incident.P2,
		Service:   "postgres_db",
		Profile:   "demo",
		CreatedAt: now.Add(-10 * time.Minute),
		UpdatedAt: now.Add(-10 * time.Minute),
		Summary:   "DB query latency trending up on 'auth' service nodes.",
		Logs: &incident.LogSummary{
			Backend: "demo",
			Window:  "15m",
			TopErrors: []incident.ErrorStat{
				{
					Message: "postgres connection pool exhaustion",
					Count:   89,
					Sample:  "fatal: remaining connection slots are reserved for non-replication superuser connections",
				},
			},
		},
		Metadata: map[string]string{
			"runbook_suggestion":    "Postgres Connection Pool Runbook",
			"runbook_pattern":       "db_connection_leak",
			"suspected_cause":       "Potential connection pool regression (similar to INC-DEMO-HIST)",
		},
	}
	store.Save(activeDbInc)

	return nil
}

// seedDemoFlows creates demo flow configurations in the sandbox
func seedDemoFlows(flowsDir string) error {
	// 1. Checkout Service Flow
	checkoutFlow := `flows:
  checkout-service:
    name: "Checkout Processing Flow"
    services:
      - checkout
    slos:
      - id: checkout-availability
        name: Checkout Availability
        objective: 99.9
        type: availability
`
	os.WriteFile(filepath.Join(flowsDir, "checkout.yaml"), []byte(checkoutFlow), 0644)

	// 2. Authentication Flow
	authFlow := `flows:
  auth-flow:
    name: "User Authentication Flow"
    services:
      - auth
    slos:
      - id: auth-latency
        name: Auth Latency P99
        objective: 95.0
        type: latency
        threshold_seconds: 0.5
`
	os.WriteFile(filepath.Join(flowsDir, "auth.yaml"), []byte(authFlow), 0644)

	// 3. Billing & Payments
	billingFlow := `flows:
  billing-flow:
    name: "Billing & Payments"
    services:
      - billing_service
    slos:
      - id: billing-success-rate
        name: Billing Success Rate
        objective: 99.5
        type: availability
`
	os.WriteFile(filepath.Join(flowsDir, "billing.yaml"), []byte(billingFlow), 0644)

	// 4. Postgres Database
	dbFlow := `flows:
  postgres-db:
    name: "Postgres Database Health"
    services:
      - postgres_db
    slos:
      - id: db-latency
        name: DB Query Latency
        objective: 99.0
        type: latency
        threshold_seconds: 0.1
`
	os.WriteFile(filepath.Join(flowsDir, "database.yaml"), []byte(dbFlow), 0644)

	return nil
}

// saveFlowToFile saves a flow configuration to YAML file
func saveFlowToFile(flowsDir string, f flow.Flow) error {
	// This would need YAML marshaling - for now just create placeholder
	filename := filepath.Join(flowsDir, f.ID+".yaml")
	return os.WriteFile(filename, []byte("# Demo flow: "+f.Name+"\n"), 0644)
}

// seedDemoSLOs creates demo SLO data
func seedDemoSLOs() error {
	// This would integrate with the SLO service to seed SLO results
	// For now, SLOs are handled through the fake backend
	return nil
}
