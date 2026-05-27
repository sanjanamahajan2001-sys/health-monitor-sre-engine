package ml

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"health-monitor/internal/config"
)

func TestLifecycleRecorder_CircularBuffer(t *testing.T) {
	// Setup temp state dir
	tmpDir, err := os.MkdirTemp("", "ml-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Mock profile manager to use tmpDir
	profile := "test-profile"
	
	r := &LifecycleRecorder{
		bufferSize:    3,
		profile:       profile,
		rollingBuffer: make([]LifecycleSnapshot, 0, 3),
	}

	// 1. Test Pushing
	r.PushSnapshot(LifecycleSnapshot{Phase: "steady", Timestamp: time.Now()})
	r.PushSnapshot(LifecycleSnapshot{Phase: "steady", Timestamp: time.Now()})
	r.PushSnapshot(LifecycleSnapshot{Phase: "steady", Timestamp: time.Now()})
	r.PushSnapshot(LifecycleSnapshot{Phase: "steady", Timestamp: time.Now()}) // Overflow

	if len(r.rollingBuffer) != 3 {
		t.Errorf("expected buffer size 3, got %d", len(r.rollingBuffer))
	}

	// 2. Test GetPreIncidentBuffer
	buf := r.GetPreIncidentBuffer()
	if len(buf) != 3 {
		t.Errorf("expected 3 snapshots in buffer, got %d", len(buf))
	}
}

func TestLifecycleRecorder_Persistence(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "ml-persist-test-*")
	defer os.RemoveAll(tmpDir)

	profile := "persist-test"
	
	// We need to ensure config uses our tmpDir for state
	os.Setenv("HEALTH_MONITOR_CONFIG_DIR", tmpDir)
	defer os.Unsetenv("HEALTH_MONITOR_CONFIG_DIR")

	r := NewLifecycleRecorder(profile)
	r.bufferSize = 5
	
	r.PushSnapshot(LifecycleSnapshot{Phase: "steady", Metrics: map[string]float64{"test": 1.0}})
	
	err := r.PersistBuffer()
	if err != nil {
		t.Fatalf("failed to persist buffer: %v", err)
	}

	// Verify file exists
	pm := config.GetProfileManager()
	path := filepath.Join(pm.GetStatePathForProfile(profile), "telemetry_window.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("telemetry_window.json was not created at %s", path)
	}

	// 3. Test Loading on New Record Initialization
	r2 := NewLifecycleRecorder(profile)
	if len(r2.rollingBuffer) != 1 {
		t.Errorf("expected 1 snapshot loaded from disk, got %d", len(r2.rollingBuffer))
	}
	if r2.rollingBuffer[0].Metrics["test"] != 1.0 {
		t.Errorf("expected metric 'test' to be 1.0, got %f", r2.rollingBuffer[0].Metrics["test"])
	}
}
