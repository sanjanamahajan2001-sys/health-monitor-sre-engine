package flow

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleViewJSONInterspersedFlag(t *testing.T) {
	ResetCacheForTest()
	dir := t.TempDir()
	flowsPath := filepath.Join(dir, "flows.yaml")
	t.Setenv("HEALTH_MONITOR_FLOW_PATHS", flowsPath)
	if err := os.WriteFile(flowsPath, []byte(`
flows:
  auth_flow:
    name: "Auth / Login"
    services:
      - identity_service
`), 0600); err != nil {
		t.Fatalf("write flows: %v", err)
	}
	SetActiveIncidentProvider(func() ([]IncidentSummary, error) {
		return []IncidentSummary{
			{ID: "INC-1", Severity: "P1", Title: "Test", Service: "identity_service", State: "Started"},
		}, nil
	})
	defer SetActiveIncidentProvider(nil)

	var out bytes.Buffer
	withStdout(&out, func() {
		exit := HandleCLI([]string{"auth_flow", "--json"})
		if exit != 0 {
			t.Fatalf("expected exit 0, got %d", exit)
		}
	})
	if !strings.Contains(out.String(), `"id": "auth_flow"`) {
		t.Fatalf("expected json output, got: %s", out.String())
	}
}

func TestHandleListJSON(t *testing.T) {
	ResetCacheForTest()
	dir := t.TempDir()
	flowsPath := filepath.Join(dir, "flows.yaml")
	t.Setenv("HEALTH_MONITOR_FLOW_PATHS", flowsPath)
	if err := os.WriteFile(flowsPath, []byte(`
flows:
  order_flow:
    name: "Order flow"
    services:
      - order_api
`), 0600); err != nil {
		t.Fatalf("write flows: %v", err)
	}
	SetActiveIncidentProvider(func() ([]IncidentSummary, error) {
		return nil, nil
	})
	defer SetActiveIncidentProvider(nil)

	var out bytes.Buffer
	withStdout(&out, func() {
		exit := HandleCLI([]string{"list", "--json"})
		if exit != 0 {
			t.Fatalf("expected exit 0, got %d", exit)
		}
	})
	if !strings.Contains(out.String(), `"id": "order_flow"`) {
		t.Fatalf("expected json output, got: %s", out.String())
	}
}

func withStdout(buf *bytes.Buffer, fn func()) {
	orig := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = orig
	_, _ = buf.ReadFrom(r)
	_ = r.Close()
}
