package incident

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"health-monitor/internal/flow"
)

func TestFormatIncidentViewImpactedFlows(t *testing.T) {
	flow.ResetCacheForTest()
	home := t.TempDir()
	t.Setenv("HOME", home)
	flowsPath := home + "/.health-monitor/flows.yaml"
	t.Setenv("HEALTH_MONITOR_FLOW_PATHS", flowsPath)
	if err := writeFlowConfig(flowsPath, `
flows:
  order_flow:
    name: "Order flow"
    services:
      - order_api
`); err != nil {
		t.Fatalf("write flow config: %v", err)
	}
	flow.ResetCacheForTest()
	incident := Incident{
		ID:        "INC-20260205-120000",
		Service:   "order_api",
		Severity:  P1,
		Title:     "Checkout timeouts",
		State:     StateStarted,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	output := FormatIncidentView(incident)
	if !strings.Contains(output, "Impacted Flows:\n• Order flow") {
		t.Fatalf("expected impacted flow in output, got:\n%s", output)
	}
}

func TestFormatIncidentViewInvalidFlowConfig(t *testing.T) {
	flow.ResetCacheForTest()
	home := t.TempDir()
	t.Setenv("HOME", home)
	flowsPath := home + "/.health-monitor/flows.yaml"
	t.Setenv("HEALTH_MONITOR_FLOW_PATHS", flowsPath)
	if err := writeFlowConfig(flowsPath, "flows: ["); err != nil {
		t.Fatalf("write flow config: %v", err)
	}
	flow.ResetCacheForTest()
	incident := Incident{
		ID:        "INC-20260205-120000",
		Service:   "order_api",
		Severity:  P1,
		Title:     "Checkout timeouts",
		State:     StateStarted,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	output := FormatIncidentView(incident)
	if !strings.Contains(output, "Impacted Flows: unavailable (invalid flow config)") {
		t.Fatalf("expected invalid flows message, got:\n%s", output)
	}
}

func writeFlowConfig(path string, content string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0600)
}
