package flow

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromLocationsNoConfig(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadFromLocations([]string{filepath.Join(dir, "flows.yaml")})
	if !IsNoConfig(err) {
		t.Fatalf("expected no config error, got %v", err)
	}
}

func TestLoadFromLocationsInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := writeFlowFile(t, dir, "flows.yaml", "flows: [")
	_, err := LoadFromLocations([]string{path})
	if !IsInvalidConfig(err) {
		t.Fatalf("expected invalid config error, got %v", err)
	}
}

func TestLoadFromLocationsMissingFlowsSection(t *testing.T) {
	dir := t.TempDir()
	path := writeFlowFile(t, dir, "flows.yaml", "bad:\n")
	result, err := LoadFromLocations([]string{path})
	if err == nil {
		t.Fatalf("expected invalid config error for missing flows section")
	}
	if !IsInvalidConfig(err) {
		t.Fatalf("expected invalid config error, got %v", err)
	}
	if len(result.Warnings) == 0 {
		t.Fatalf("expected warnings for missing flows section")
	}
}

func TestLoadFromLocationsDuplicateIDs(t *testing.T) {
	dir := t.TempDir()
	first := writeFlowFile(t, dir, "flows-a.yaml", `
flows:
  order_flow:
    name: "Order flow"
    services:
      - frontend
`)
	second := writeFlowFile(t, dir, "flows-b.yaml", `
flows:
  order_flow:
    name: "Duplicate"
    services:
      - order_api
`)
	result, err := LoadFromLocations([]string{first, second})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Flows) != 1 {
		t.Fatalf("expected 1 flow, got %d", len(result.Flows))
	}
	if len(result.Warnings) == 0 {
		t.Fatalf("expected warnings for duplicate ids")
	}
}

func TestLoadFromLocationsEmptyServices(t *testing.T) {
	dir := t.TempDir()
	path := writeFlowFile(t, dir, "flows.yaml", `
flows:
  order_flow:
    name: "Order flow"
    services: []
`)
	_, err := LoadFromLocations([]string{path})
	if !IsInvalidConfig(err) {
		t.Fatalf("expected invalid config error, got %v", err)
	}
}

func TestLoadFromLocationsMultipleFiles(t *testing.T) {
	dir := t.TempDir()
	first := writeFlowFile(t, dir, "flows-a.yaml", `
flows:
  order_flow:
    name: "Order flow"
    services:
      - frontend
      - order_api
`)
	second := writeFlowFile(t, dir, "flows-b.yaml", `
flows:
  auth_flow:
    name: "Auth flow"
    services:
      - auth_api
`)
	result, err := LoadFromLocations([]string{first, second})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Flows) != 2 {
		t.Fatalf("expected 2 flows, got %d", len(result.Flows))
	}
}

func TestLoadOverrideDirectory(t *testing.T) {
	ResetCacheForTest()
	dir := t.TempDir()
	path := filepath.Join(dir, "flows.yaml")
	if err := os.WriteFile(path, []byte(`
flows:
  auth_flow:
    name: "Auth flow"
    services:
      - auth_api
`), 0600); err != nil {
		t.Fatalf("write flows: %v", err)
	}
	t.Setenv("HEALTH_MONITOR_FLOW_PATHS", dir)
	result, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Flows) != 1 || result.Flows[0].ID != "auth_flow" {
		t.Fatalf("expected auth_flow, got %+v", result.Flows)
	}
}

func writeFlowFile(t *testing.T, dir string, name string, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	return path
}
