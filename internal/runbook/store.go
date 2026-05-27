package runbook

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"health-monitor/internal/config"
)

// Store interface for runbook persistence
type Store interface {
	Save(runbook *Runbook) error
	Get(id string) (*Runbook, error)
	List(service, pattern string, limit int) ([]*Runbook, error)
	Delete(id string) error
	Update(runbook *Runbook) error
	ListPatterns(service string) ([]string, error)
	GetSuggestions(incidentID string) ([]*RunbookSuggestion, error)
	SaveSuggestion(suggestion *RunbookSuggestion) error
	GetStats() (*StoreStats, error)
}

// FileStore implements file-based runbook storage
type FileStore struct {
	basePath string
	config   config.RunbookConfig
}

// NewFileStore creates a new file-based runbook store
func NewFileStore(basePath string, config config.RunbookConfig) *FileStore {
	return &FileStore{
		basePath: basePath,
		config:   config,
	}
}

// Save saves a runbook to file storage
func (fs *FileStore) Save(runbook *Runbook) error {
	// Create output directory with proper permissions
	if err := os.MkdirAll(fs.basePath, 0755); err != nil {
		return fmt.Errorf("failed to create base directory: %w", err)
	}
	
	// Ensure directory has correct ownership (try to fix permissions)
	if err := os.Chmod(fs.basePath, 0755); err != nil {
		// Log but don't fail - directory creation is more important
		fmt.Printf("Warning: could not set permissions on %s: %v\n", fs.basePath, err)
	}

	// Determine file extension based on format
	ext := "json" // Default to JSON for structured data
	if runbook.Format == "markdown" {
		ext = "md"
	}
	
	filename := fmt.Sprintf("%s.%s", runbook.ID, ext)
	filepath := filepath.Join(fs.basePath, filename)

	var data []byte
	var err error
	
	if runbook.Format == "markdown" {
		// Save as markdown file with content
		data = []byte(runbook.Content)
	} else {
		// Save as JSON file with full structure
		data, err = json.MarshalIndent(runbook, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal runbook: %w", err)
		}
	}

	if err := os.WriteFile(filepath, data, 0644); err != nil {
		return fmt.Errorf("failed to write runbook file: %w", err)
	}

	return nil
}

// Get retrieves a runbook by ID
func (fs *FileStore) Get(id string) (*Runbook, error) {
	filename := fmt.Sprintf("%s.json", id)
	filepath := filepath.Join(fs.basePath, filename)

	data, err := os.ReadFile(filepath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("runbook %s not found", id)
		}
		return nil, fmt.Errorf("failed to read runbook file: %w", err)
	}

	var runbook Runbook
	if err := json.Unmarshal(data, &runbook); err != nil {
		return nil, fmt.Errorf("failed to unmarshal runbook: %w", err)
	}

	return &runbook, nil
}

// List lists runbooks with optional filtering
func (fs *FileStore) List(service, pattern string, limit int) ([]*Runbook, error) {
	files, err := os.ReadDir(fs.basePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []*Runbook{}, nil
		}
		return nil, fmt.Errorf("failed to read runbook directory: %w", err)
	}

	var runbooks []*Runbook
	for _, file := range files {
		if !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		runbook, err := fs.Get(strings.TrimSuffix(file.Name(), ".json"))
		if err != nil {
			continue // Skip files that can't be read
		}

		// Apply filters
		if service != "" && runbook.Service != service {
			continue
		}
		if pattern != "" && runbook.Pattern != pattern {
			continue
		}

		runbooks = append(runbooks, runbook)
	}

	// Sort by creation date (newest first)
	sort.Slice(runbooks, func(i, j int) bool {
		return runbooks[i].CreatedAt.After(runbooks[j].CreatedAt)
	})

	// Apply limit
	if limit > 0 && len(runbooks) > limit {
		runbooks = runbooks[:limit]
	}

	return runbooks, nil
}

// Delete removes a runbook
func (fs *FileStore) Delete(id string) error {
	filename := fmt.Sprintf("%s.json", id)
	filepath := filepath.Join(fs.basePath, filename)

	if err := os.Remove(filepath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("runbook %s not found", id)
		}
		return fmt.Errorf("failed to delete runbook file: %w", err)
	}

	return nil
}

// Update updates an existing runbook
func (fs *FileStore) Update(runbook *Runbook) error {
	// Check if runbook exists
	existing, err := fs.Get(runbook.ID)
	if err != nil {
		return fmt.Errorf("runbook %s not found: %w", runbook.ID, err)
	}

	// Preserve creation timestamp
	runbook.CreatedAt = existing.CreatedAt
	runbook.UpdatedAt = time.Now()

	return fs.Save(runbook)
}

// ListPatterns lists all unique patterns for a service
func (fs *FileStore) ListPatterns(service string) ([]string, error) {
	runbooks, err := fs.List(service, "", 0)
	if err != nil {
		return nil, err
	}

	patternSet := make(map[string]bool)
	for _, runbook := range runbooks {
		if runbook.Pattern != "" {
			patternSet[runbook.Pattern] = true
		}
	}

	var patterns []string
	for pattern := range patternSet {
		patterns = append(patterns, pattern)
	}

	sort.Strings(patterns)
	return patterns, nil
}

// GetSuggestions gets runbook suggestions for an incident
func (fs *FileStore) GetSuggestions(incidentID string) ([]*RunbookSuggestion, error) {
	suggestionsFile := filepath.Join(fs.basePath, "suggestions", fmt.Sprintf("%s.json", incidentID))
	
	data, err := os.ReadFile(suggestionsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return []*RunbookSuggestion{}, nil
		}
		return nil, fmt.Errorf("failed to read suggestions file: %w", err)
	}

	var suggestions []*RunbookSuggestion
	if err := json.Unmarshal(data, &suggestions); err != nil {
		return nil, fmt.Errorf("failed to unmarshal suggestions: %w", err)
	}

	return suggestions, nil
}

// SaveSuggestion saves a runbook suggestion
func (fs *FileStore) SaveSuggestion(suggestion *RunbookSuggestion) error {
	suggestionsDir := filepath.Join(fs.basePath, "suggestions")
	if err := os.MkdirAll(suggestionsDir, 0755); err != nil {
		return fmt.Errorf("failed to create suggestions directory: %w", err)
	}

	// Load existing suggestions
	suggestions, err := fs.GetSuggestions(suggestion.IncidentID)
	if err != nil {
		return err
	}

	// Add or update suggestion
	found := false
	for i, existing := range suggestions {
		if existing.RunbookID == suggestion.RunbookID {
			suggestions[i] = suggestion
			found = true
			break
		}
	}

	if !found {
		suggestions = append(suggestions, suggestion)
	}

	// Save suggestions
	suggestionsFile := filepath.Join(suggestionsDir, fmt.Sprintf("%s.json", suggestion.IncidentID))
	data, err := json.MarshalIndent(suggestions, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal suggestions: %w", err)
	}

	if err := os.WriteFile(suggestionsFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write suggestions file: %w", err)
	}

	return nil
}

// CleanupOldRunbooks removes runbooks older than retention period
func (fs *FileStore) CleanupOldRunbooks() error {
	if fs.config.RetentionDays <= 0 {
		return nil // No cleanup if retention is disabled
	}

	cutoff := time.Now().AddDate(0, 0, -fs.config.RetentionDays)
	runbooks, err := fs.List("", "", 0)
	if err != nil {
		return err
	}

	for _, runbook := range runbooks {
		if runbook.CreatedAt.Before(cutoff) {
			if err := fs.Delete(runbook.ID); err != nil {
				// Log error but continue with cleanup
				fmt.Printf("Warning: failed to delete old runbook %s: %v\n", runbook.ID, err)
			}
		}
	}

	return nil
}

// GetStats returns statistics about the runbook store
func (fs *FileStore) GetStats() (*StoreStats, error) {
	runbooks, err := fs.List("", "", 0)
	if err != nil {
		return nil, err
	}

	stats := &StoreStats{
		TotalRunbooks:    len(runbooks),
		PublishedRunbooks: 0,
		Patterns:          make(map[string]int),
		Services:          make(map[string]int),
		Categories:        make(map[string]int),
	}

	for _, runbook := range runbooks {
		if runbook.Published {
			stats.PublishedRunbooks++
		}
		if runbook.Pattern != "" {
			stats.Patterns[runbook.Pattern]++
		}
		if runbook.Service != "" {
			stats.Services[runbook.Service]++
		}
		if runbook.Category != "" {
			stats.Categories[runbook.Category]++
		}
	}

	return stats, nil
}

// StoreStats represents statistics about the runbook store
type StoreStats struct {
	TotalRunbooks    int            `json:"total_runbooks"`
	PublishedRunbooks int            `json:"published_runbooks"`
	Patterns         map[string]int `json:"patterns"`
	Services         map[string]int `json:"services"`
	Categories       map[string]int `json:"categories"`
}
