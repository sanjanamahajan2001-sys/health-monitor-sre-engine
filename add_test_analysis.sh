#!/bin/bash

# Script to add analysis data to an incident for testing runbook suggestions

INCIDENT_ID="INC-20260220-051233"
DATA_DIR="/var/lib/health-monitor/alpha-us/incidents"

# Create analysis data with pattern information
cat > "${DATA_DIR}/${INCIDENT_ID}.json" << 'EOF'
{
  "id": "INC-20260220-042659",
  "title": "PostgreSQL Query Timeout Issue",
  "service": "billing_api",
  "severity": "P2",
  "state": "Started",
  "created_at": "2026-02-20T04:26:59Z",
  "updated_at": "2026-02-20T04:26:59Z",
  "description": "Database queries timing out during checkout process",
  "profile": "alpha-us",
  "analysis": {
    "component": "postgres_db",
    "dependency": "postgresql",
    "pattern": "query_timeout",
    "category": "dependency",
    "root_cause": "Database performance degradation",
    "fix_summary": "Optimize slow queries and add indexes",
    "failure_type": "performance",
    "error_signature": "query timeout"
  },
  "events": [
    {
      "timestamp": "2026-02-20T04:26:59Z",
      "type": "started",
      "message": "Incident started due to database query timeouts"
    }
  ],
  "logs": {
    "summary": {
      "top_errors": [
        {
          "message": "ERROR: canceling statement due to statement timeout",
          "count": 15,
          "last_seen": "2026-02-20T04:26:45Z"
        },
        {
          "message": "Context: Post checkout process",
          "count": 8,
          "last_seen": "2026-02-20T04:26:30Z"
        }
      ]
    }
  },
  "metrics": {
    "summary": {
      "anomaly_count": 3,
      "top_anomalies": [
        {
          "metric": "http_request_duration_seconds",
          "value": 5.2,
          "threshold": 1.0
        }
      ]
    }
  }
}
EOF

echo "✅ Added analysis data to incident ${INCIDENT_ID}"
echo "📊 Pattern: query_timeout"
echo "🔧 Component: postgres_db"
echo "📂 Category: dependency"
echo ""
echo "Now test the runbook suggestion:"
echo "sudo ./health-monitor runbook suggest ${INCIDENT_ID}"
