package alert

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"health-monitor/internal/config"
)

type HistoryEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Alert     map[string]interface{} `json:"alert"`
}

type HistoryWriter struct {
	path string
}

func historyPath() string {
	pm := config.GetProfileManager()
	profileStateDir := pm.GetStatePath()
	return filepath.Join(profileStateDir, "alert-history.log")
}

// NewHistoryWriter creates a new history writer with profile-aware path
func NewHistoryWriter() *HistoryWriter {
	return &HistoryWriter{
		path: historyPath(),
	}
}

func (hw *HistoryWriter) Write(alert map[string]interface{}) error {
	entry := HistoryEntry{
		Timestamp: time.Now().UTC(),
		Alert:     alert,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal history entry: %w", err)
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(hw.path), 0755); err != nil {
		return fmt.Errorf("failed to create history directory: %w", err)
	}

	file, err := os.OpenFile(hw.path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("failed to open history file: %w", err)
	}
	defer file.Close()

	if _, err := file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("failed to write history entry: %w", err)
	}

	return nil
}
