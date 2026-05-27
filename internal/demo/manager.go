package demo

// RunbookSimulation represents a simulated runbook view
type RunbookSimulation struct {
	IncidentID string
	Pattern    string
	Service    string
	Content    string
}

// PostmortemSimulation represents a simulated postmortem view
type PostmortemSimulation struct {
	IncidentID string
	Content    string
}

// SimilarIncident represents a simulated historical match
type SimilarIncident struct {
	ID         string
	Title      string
	Service    string
	Confidence float64
	Age        string
	State      string
}

// GetRunbook returns a simulated runbook for the given incident
func GetRunbook(incidentID string) *RunbookSimulation {
	// Special case for the golden incident
	if incidentID == "INC-20260324-052436" || incidentID == "INC-DEMO-HIST" {
		return &RunbookSimulation{
			IncidentID: incidentID,
			Pattern:    "database connection pool exhaustion",
			Service:    "postgres_db",
			Content: `╭──────────────────────────────────────────────────────────────────────────╮
│  📗 SIMULATED RUNBOOK EXAMPLE                                            │
╰──────────────────────────────────────────────────────────────────────────╯

🔍 Pattern Analysis for ` + incidentID + `

Pattern: database connection pool exhaustion
Service: postgres_db
Component: connection_manager
Confidence: 98.0%

📚 Suggested Runbook: rb-20260325-084210-postgres-conn-exhaustion
Title: Postgres - Connection Pool Troubleshooting & Remediation
Severity: P2
Steps: 6

---
## 📖 Runbook Content

### 📋 Overview
High connection counts detected. Usually caused by application connection leaks or sudden traffic spikes.

### 🛠️ Quick Remediation
1. **Identify Leaking Service**: Check 'user-profile' or 'auth-svc' for unclosed sessions.
2. **Kill Idle Connections**: Run 'health-monitor config exec --cmd "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE state = 'idle';"'
3. **Increase Pool Size**: If load is legitimate, increase 'max_connections' in RDS/Postgres config.

### 💡 Long-term Fix
- Implement PgBouncer or similar connection pooling layer.
- Ensure all repository methods use 'defer rows.Close()'.
`,
		}
	}
	
	// Default demo runbook for other IDs
	return &RunbookSimulation{
		IncidentID: incidentID,
		Content: `╭──────────────────────────────────────────────────────────────────────────╮
│  📗 SIMULATED RUNBOOK EXAMPLE                                            │
╰──────────────────────────────────────────────────────────────────────────╯

No specific pattern match for ` + incidentID + `.

💡 General Remediation Steps:
1. Check logs for service errors.
2. Verify resource usage (CPU/Memory).
3. Review recent deployments.
`,
	}
}

// GetPostmortem returns a simulated postmortem for the given incident
func GetPostmortem(incidentID string) *PostmortemSimulation {
	// Provide a postmortem for any demo incident
	return &PostmortemSimulation{
		IncidentID: incidentID,
		Content: `╭──────────────────────────────────────────────────────────────────────────╮
│  📝 SIMULATED POSTMORTEM EXAMPLE                                         │
╰──────────────────────────────────────────────────────────────────────────╯

# Blameless Postmortem: ` + incidentID + `

**Date**: 2026-03-25
**Severity**: P2
**Status**: DRAFT

## 📝 Summary
This is a high-fidelity demonstration of the Health Monitor Postmortem feature.
In a real scenario, this would be generated from logs, traces, and incident events.

## 📉 Impact
- Service availability dropped to 92%.
- Customer impact was localized to the apac-region.

## 🔍 Root Cause
Simulated root cause analysis suggests a misconfigured health check threshold.

## ✅ Lessons Learned
- Automated rollbacks saved 15 minutes of downtime.
- Monitoring gaps identified in the outbound proxy layer.
`,
	}
}

// GetSimilarIncidents returns simulated historical matches
func GetSimilarIncidents(incidentID string) []SimilarIncident {
	if incidentID == "INC-20260324-052436" {
		return []SimilarIncident{
			{
				ID:         "INC-20260212-091522",
				Title:      "Payment Gateway Latency Spike",
				Service:    "orders_service",
				Confidence: 0.89,
				Age:        "6 weeks ago",
				State:      "Resolved",
			},
			{
				ID:         "INC-20260105-144031",
				Title:      "Egress NAT Gateway Failure",
				Service:    "infrastructure",
				Confidence: 0.76,
				Age:        "3 months ago",
				State:      "Resolved",
			},
		}
	}
	return nil
}

// IsDemoMode helper
func IsDemoMode() bool {
	// Check for environment variable
	return true // For actual implementation, this would check os.Getenv("HEALTH_MONITOR_DEMO_MODE") == "true"
}
