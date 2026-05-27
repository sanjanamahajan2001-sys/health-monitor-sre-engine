package incident

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"health-monitor/internal/flow"
)

func TestExportMarkdown(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HEALTH_MONITOR_EXPORT_PATH", dir)
	flowPath := filepath.Join(dir, "flows.yaml")
	t.Setenv("HEALTH_MONITOR_FLOW_PATHS", flowPath)
	flow.ResetCacheForTest()
	if err := os.WriteFile(flowPath, []byte(`
flows:
  order_flow:
    name: "Order flow"
    services:
      - order_api
`), 0600); err != nil {
		t.Fatalf("write flows: %v", err)
	}
	flow.ResetCacheForTest()
	now := time.Date(2026, 2, 4, 17, 23, 0, 0, time.UTC)
	incident := Incident{
		ID:        "INC-20260204-172300",
		Service:   "order_api",
		Severity:  P1,
		Title:     "Checkout timeouts",
		State:     StateResolved,
		CreatedAt: now,
		UpdatedAt: now,
		Summary:   "DB pool exhaustion",
		Events: []Event{
			{Timestamp: now, User: "sanjana", Type: EventStart},
			{Timestamp: now.Add(2 * time.Minute), User: "sanjana", Type: EventResolve, Message: "DB pool exhaustion"},
		},
	}
	path, err := exportIncident(incident, "markdown")
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}
	if !strings.HasPrefix(path, dir) {
		t.Fatalf("expected export in %s, got %s", dir, path)
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("read export failed: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "# Incident Report") || !strings.Contains(content, "## Impacted Flows") || !strings.Contains(content, "## Timeline") || !strings.Contains(content, "## Root Cause") {
		t.Fatalf("export content missing sections")
	}
	if !strings.Contains(content, "- Order flow") {
		t.Fatalf("expected impacted flow in markdown export")
	}
}

func TestExportJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HEALTH_MONITOR_EXPORT_PATH", dir)
	flowPath := filepath.Join(dir, "flows.yaml")
	t.Setenv("HEALTH_MONITOR_FLOW_PATHS", flowPath)
	flow.ResetCacheForTest()
	if err := os.WriteFile(flowPath, []byte(`
flows:
  order_flow:
    name: "Order flow"
    services:
      - order_api
`), 0600); err != nil {
		t.Fatalf("write flows: %v", err)
	}
	flow.ResetCacheForTest()
	now := time.Date(2026, 2, 4, 17, 23, 0, 0, time.UTC)
	incident := Incident{
		ID:        "INC-20260204-172300",
		Service:   "order_api",
		Severity:  P1,
		Title:     "Checkout timeouts",
		State:     StateResolved,
		CreatedAt: now,
		UpdatedAt: now,
		Summary:   "DB pool exhaustion",
		Events: []Event{
			{Timestamp: now, User: "sanjana", Type: EventStart},
			{Timestamp: now.Add(2 * time.Minute), User: "sanjana", Type: EventResolve, Message: "DB pool exhaustion"},
		},
	}
	path, err := exportIncident(incident, "json")
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("read export failed: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, `"incident"`) || !strings.Contains(content, `"timeline"`) || !strings.Contains(content, `"impacted_flows"`) {
		t.Fatalf("export content missing expected fields")
	}
	if !strings.Contains(content, `"Order flow"`) {
		t.Fatalf("expected impacted flow in json export")
	}
}
