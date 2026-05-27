package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"health-monitor/internal/config"
	"health-monitor/internal/output"
)

// Action represents a single user action in a collaborative session
type Action struct {
	Timestamp  time.Time `json:"timestamp"`
	IncidentID string    `json:"incident_id"`
	User       string    `json:"user"`
	Activity   string    `json:"activity"`
}

var (
	mu sync.Mutex
)

// LogAction records a user action to the persistent audit log for a specific incident
func LogAction(incidentID, user, activity string) error {
	mu.Lock()
	defer mu.Unlock()

	pm := config.GetProfileManager()
	// Use the established incident directory for the audit log
	incDir := filepath.Join(pm.GetStatePath(), "incidents", incidentID)
	// Create directory if it doesn't exist (e.g. for "REMOTE" global logs)
	if incidentID == "REMOTE" || incidentID == "SYSTEM" {
		incDir = pm.GetStatePath()
	}
	
	if _, err := os.Stat(incDir); os.IsNotExist(err) {
		_ = os.MkdirAll(incDir, 0755)
		_ = os.Chmod(incDir, 0777) // Ensure directory is accessible
	}

	logFile := filepath.Join(incDir, "audit.json")
	
	action := Action{
		Timestamp:  time.Now(),
		IncidentID: incidentID,
		User:       user,
		Activity:   activity,
	}

	// Load existing
	var actions []Action
	if data, err := os.ReadFile(logFile); err == nil {
		_ = json.Unmarshal(data, &actions)
	}

	actions = append(actions, action)
	
	data, err := json.MarshalIndent(actions, "", "  ")
	if err != nil {
		return err
	}

	err = os.WriteFile(logFile, data, 0666)
	if err == nil {
		// Try to ensure permissions are wide enough for the user to read
		_ = os.Chmod(logFile, 0666)
	}
	
	output.Debugf("[Audit] %s: %s - %s", user, incidentID, activity)
	return err
}

// GetHistory returns the last 10 minutes of history for an incident
func GetHistory(incidentID string) ([]Action, error) {
	mu.Lock()
	defer mu.Unlock()

	pm := config.GetProfileManager()
	incDir := filepath.Join(pm.GetStatePath(), "incidents", incidentID)
	if incidentID == "REMOTE" || incidentID == "SYSTEM" {
		incDir = pm.GetStatePath()
	}
	logFile := filepath.Join(incDir, "audit.json")

	var actions []Action
	data, err := os.ReadFile(logFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, &actions); err != nil {
		return nil, err
	}

	var history []Action
	tenMinAgo := time.Now().Add(-10 * time.Minute)

	for _, a := range actions {
		if a.Timestamp.After(tenMinAgo) {
			history = append(history, a)
		}
	}
	return history, nil
}

