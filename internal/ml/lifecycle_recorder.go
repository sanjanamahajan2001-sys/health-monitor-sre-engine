package ml

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"health-monitor/internal/config"
	"health-monitor/internal/storage"
)

// LifecycleRecorder manages the capture of system state windows
type LifecycleRecorder struct {
	mu           sync.RWMutex
	rollingBuffer []LifecycleSnapshot
	bufferSize   int
	profile      string
}

func NewLifecycleRecorder(profile string) *LifecycleRecorder {
	r := &LifecycleRecorder{
		bufferSize:   60, // 60 snapshots (e.g., 1 per minute for 1 hour)
		profile:      profile,
		rollingBuffer: make([]LifecycleSnapshot, 0, 60),
	}
	
	// Load existing buffer from disk to survive restarts
	pm := config.GetProfileManager()
	stateDir := pm.GetStatePathForProfile(profile)
	path := filepath.Join(stateDir, "telemetry_window.json")
	if data, err := os.ReadFile(path); err == nil {
		var snapshots []LifecycleSnapshot
		if err := json.Unmarshal(data, &snapshots); err == nil {
			r.rollingBuffer = snapshots
		}
	}
	
	return r
}

// PushSnapshot adds a new snapshot to the rolling buffer
func (r *LifecycleRecorder) PushSnapshot(s LifecycleSnapshot) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.rollingBuffer) >= r.bufferSize {
		r.rollingBuffer = r.rollingBuffer[1:]
	}
	r.rollingBuffer = append(r.rollingBuffer, s)
}

// PersistBuffer saves the current rolling buffer to a special telemetry_window.json file
func (r *LifecycleRecorder) PersistBuffer() error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	pm := config.GetProfileManager()
	stateDir := pm.GetStatePathForProfile(r.profile)
	path := filepath.Join(stateDir, "telemetry_window.json")

	data, err := json.MarshalIndent(r.rollingBuffer, "", "  ")
	if err != nil {
		return err
	}

	return storage.AtomicWriteFile(path, data, 0600)
}

// GetPreIncidentBuffer returns the current rolling buffer for pre-incident analysis.
// If the in-memory buffer is empty, it attempts to load from the telemetry_window.json on disk.
func (r *LifecycleRecorder) GetPreIncidentBuffer() []LifecycleSnapshot {
	r.mu.RLock()
	if len(r.rollingBuffer) > 0 {
		out := make([]LifecycleSnapshot, len(r.rollingBuffer))
		copy(out, r.rollingBuffer)
		r.mu.RUnlock()
		return out
	}
	r.mu.RUnlock()

	// Try to load from disk
	pm := config.GetProfileManager()
	stateDir := pm.GetStatePathForProfile(r.profile)
	path := filepath.Join(stateDir, "telemetry_window.json")

	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var snapshots []LifecycleSnapshot
	if err := json.Unmarshal(data, &snapshots); err != nil {
		return nil
	}

	return snapshots
}

// PersistSeed saves a set of reconstructed snapshots to an incident-specific file.
// This prevents the background daemon from overwriting them in the shared buffer.
func (r *LifecycleRecorder) PersistSeed(incidentID string, snapshots []LifecycleSnapshot) error {
	if incidentID == "" || len(snapshots) == 0 {
		return nil
	}

	pm := config.GetProfileManager()
	stateDir := pm.GetStatePathForProfile(r.profile)
	seedDir := filepath.Join(stateDir, "incidents", "seeds")

	if err := os.MkdirAll(seedDir, 0755); err != nil {
		return err
	}

	path := filepath.Join(seedDir, incidentID+"_seed.json")
	data, err := json.MarshalIndent(snapshots, "", "  ")
	if err != nil {
		return err
	}

	return storage.AtomicWriteFile(path, data, 0600)
}

// LoadSeed retrieves reconstructed snapshots for a specific incident.
func (r *LifecycleRecorder) LoadSeed(incidentID string) ([]LifecycleSnapshot, error) {
	pm := config.GetProfileManager()
	stateDir := pm.GetStatePathForProfile(r.profile)
	path := filepath.Join(stateDir, "incidents", "seeds", incidentID+"_seed.json")

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var snapshots []LifecycleSnapshot
	if err := json.Unmarshal(data, &snapshots); err != nil {
		return nil, err
	}

	return snapshots, nil
}

// PersistLifecycle saves the full lifecycle data to disk
func (r *LifecycleRecorder) PersistLifecycle(lifecycle FullLifecycle) error {
	pm := config.GetProfileManager()
	stateDir := pm.GetStatePathForProfile(r.profile)
	lifecycleDir := filepath.Join(stateDir, "incidents", "lifecycle")

	if err := os.MkdirAll(lifecycleDir, 0755); err != nil {
		return fmt.Errorf("failed to create lifecycle directory: %w", err)
	}

	filename := fmt.Sprintf("%s_lifecycle.json", lifecycle.IncidentID)
	path := filepath.Join(lifecycleDir, filename)

	data, err := json.MarshalIndent(lifecycle, "", "  ")
	if err != nil {
		return err
	}

	// After persisting lifecycle, we can cleanup the seed file
	seedPath := filepath.Join(stateDir, "incidents", "seeds", lifecycle.IncidentID+"_seed.json")
	_ = os.Remove(seedPath)

	return storage.AtomicWriteFile(path, data, 0600)
}

// LoadLifecycle retrieves persisted lifecycle data
func (r *LifecycleRecorder) LoadLifecycle(incidentID string) (*FullLifecycle, error) {
	pm := config.GetProfileManager()
	stateDir := pm.GetStatePathForProfile(r.profile)
	path := filepath.Join(stateDir, "incidents", "lifecycle", incidentID+"_lifecycle.json")

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var lifecycle FullLifecycle
	if err := json.Unmarshal(data, &lifecycle); err != nil {
		return nil, err
	}

	return &lifecycle, nil
}
