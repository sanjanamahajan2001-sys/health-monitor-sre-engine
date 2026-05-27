package incident

import "testing"

func TestHandleCLIStartAndNote(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("HEALTH_MONITOR_FLOW_PATHS", "")
	exit := HandleCLI([]string{
		"start",
		"--service", "order_api",
		"--severity", "P1",
		"--title", "Checkout timeouts",
	})
	if exit != 0 {
		t.Fatalf("expected start exit 0, got %d", exit)
	}
	exit = HandleCLI([]string{
		"note",
		"Scaled postgres pool 50->100",
	})
	if exit != 0 {
		t.Fatalf("expected note exit 0, got %d", exit)
	}
	exit = HandleCLI([]string{
		"export",
		"--format", "json",
	})
	if exit != 0 {
		t.Fatalf("expected export exit 0, got %d", exit)
	}
}

func TestHandleCLIInvalidSeverity(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("HEALTH_MONITOR_FLOW_PATHS", "")
	exit := HandleCLI([]string{
		"start",
		"--service", "order_api",
		"--severity", "P9",
		"--title", "Checkout timeouts",
	})
	if exit == 0 {
		t.Fatalf("expected non-zero exit for invalid severity")
	}
}
