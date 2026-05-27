package incident

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"health-monitor/internal/config"
	"health-monitor/internal/storage"
)

var storeMu sync.Mutex

var ErrIncidentExists = errors.New("incident already exists")

type Store struct {
	dir string
	mu  *sync.Mutex
}

func NewStore() (*Store, error) {
	pm := config.GetProfileManager()
	
	// Use profile-scoped state directory
	profileStateDir := pm.GetStatePath()
	incidentDir := filepath.Join(profileStateDir, "incidents")
	
	canonicalPath, err := resolveCanonicalDataPath(incidentDir)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(canonicalPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create incident directory: %w", err)
	}

	return &Store{
		dir: canonicalPath,
		mu:  &storeMu,
	}, nil
}

func NewStoreForProfile(profile string) (*Store, error) {
	pm := config.GetProfileManager()
	profileStateDir := pm.GetStatePathForProfile(profile)
	incidentDir := filepath.Join(profileStateDir, "incidents")
	
	if err := os.MkdirAll(incidentDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create incident directory for profile %s: %w", profile, err)
	}

	return &Store{
		dir: incidentDir,
		mu:  &storeMu,
	}, nil
}

func NewStoreWithDir(dir string) *Store {
	return &Store{dir: dir, mu: &storeMu}
}

func (s *Store) Save(incident Incident) error {
	if strings.TrimSpace(incident.ID) == "" {
		return errors.New("incident ID is empty")
	}
	path := s.getPath(incident.ID)
	
	s.mu.Lock()
	defer s.mu.Unlock()
	
	payload, err := json.MarshalIndent(incident, "", "  ")
	if err != nil {
		return err
	}
	
	if err := storage.AtomicWriteFile(path, payload, 0600); err != nil {
		return err
	}
	
	// Cleanup legacy root file if we just saved to a partitioned path
	if filepath.Dir(path) != s.dir {
		rootPath := filepath.Join(s.dir, incident.ID+".json")
		if _, err := os.Stat(rootPath); err == nil {
			_ = os.Remove(rootPath)
		}
	}
	
	return applySudoOwnership(path)
}

func (s *Store) SaveMetadata(id string, metadata map[string]string) error {
	inc, err := s.Load(id)
	if err != nil {
		return err
	}
	inc.Metadata = metadata
	return s.Save(inc)
}

// SaveRunbookSuggestion saves a runbook suggestion to incident metadata
func (s *Store) SaveRunbookSuggestion(id string, pattern string, runbookURL string) error {
	inc, err := s.Load(id)
	if err != nil {
		return err
	}
	
	if inc.Metadata == nil {
		inc.Metadata = make(map[string]string)
	}
	inc.Metadata["runbook_pattern"] = pattern
	inc.Metadata["runbook_url"] = runbookURL
	inc.Metadata["runbook_suggested_at"] = time.Now().Format(time.RFC3339)
	
	return s.Save(inc)
}

func (s *Store) SaveNew(incident Incident) error {
	if strings.TrimSpace(incident.ID) == "" {
		return errors.New("incident ID is empty")
	}
	path := s.getPath(incident.ID)
	
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// Check if already exists in either location
	if _, err := os.Stat(path); err == nil {
		return ErrIncidentExists
	}
	rootPath := filepath.Join(s.dir, incident.ID+".json")
	if _, err := os.Stat(rootPath); err == nil {
		return ErrIncidentExists
	}

	payload, err := json.MarshalIndent(incident, "", "  ")
	if err != nil {
		return err
	}
	
	// For "SaveNew" with O_EXCL behavior, we still want atomicity but also the EXCL check.
	// We'll use os.OpenFile with O_EXCL on the target path first, then write.
	// However, AtomicWriteFile renames over.
	// So we check existence first (already done above with Stat), then atomicity.
	
	if err := storage.AtomicWriteFile(path, payload, 0600); err != nil {
		return err
	}
	
	return applySudoOwnership(path)
}

func (s *Store) Load(id string) (Incident, error) {
	var incident Incident
	if strings.TrimSpace(id) == "" {
		return incident, errors.New("incident ID is empty")
	}
	
	path := s.getPath(id)
	
	s.mu.Lock()
	defer s.mu.Unlock()
	
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Try root directory for legacy compatibility
			rootPath := filepath.Join(s.dir, id+".json")
			data, err = os.ReadFile(rootPath)
			if err != nil {
				return incident, err
			}
		} else {
			return incident, err
		}
	}
	
	if err := json.Unmarshal(data, &incident); err != nil {
		return incident, fmt.Errorf("invalid incident file: %w", err)
	}
	return incident, nil
}

func (s *Store) List() ([]Incident, []string, error) {
	return s.ListForMonth(0, 0)
}

// ListForMonth returns incidents for a specific month using filename-based filtering (INC-YYYYMMDD)
func (s *Store) ListForMonth(month time.Month, year int) ([]Incident, []string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	var incidents []Incident
	var warnings []string
	
	datePrefix := ""
	if year > 0 && month > 0 {
		datePrefix = fmt.Sprintf("INC-%04d%02d", year, month)
	}

	// Use a map to deduplicate by ID
	idMap := make(map[string]bool)

	// 1. Scan partitioned directories (prefer these)
	// Walk the directory recursively to find all .json files
	err := filepath.Walk(s.dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if info.IsDir() {
			// Skip specialized subdirectories that don't contains actual incidents
			if info.Name() == "lifecycle" || info.Name() == "predictions" || info.Name() == "seeds" {
				return filepath.SkipDir
			}
			return nil
		}
		
		if !strings.HasSuffix(info.Name(), ".json") {
			return nil
		}
		
		// Skip root entries in this walk, we'll process them next
		if filepath.Dir(path) == s.dir {
			return nil
		}

		if info.Name() == "audit.json" {
			return nil
		}

		if datePrefix != "" && !strings.HasPrefix(info.Name(), datePrefix) {
			return nil
		}

		inc, warn := s.loadFromFile(path)
		if warn != "" {
			warnings = append(warnings, warn)
		} else {
			if !idMap[inc.ID] {
				incidents = append(incidents, inc)
				idMap[inc.ID] = true
			}
		}
		return nil
	})

	if err != nil {
		warnings = append(warnings, fmt.Sprintf("error walking incident directory: %v", err))
	}

	// 2. Scan root directory (legacy fallback)
	rootEntries, _ := os.ReadDir(s.dir)
	for _, entry := range rootEntries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		if datePrefix != "" && !strings.HasPrefix(entry.Name(), datePrefix) {
			continue
		}
		
		path := filepath.Join(s.dir, entry.Name())
		inc, warn := s.loadFromFile(path)
		if warn != "" {
			warnings = append(warnings, warn)
		} else {
			if !idMap[inc.ID] {
				incidents = append(incidents, inc)
				idMap[inc.ID] = true
			}
		}
	}

	if err != nil {
		warnings = append(warnings, fmt.Sprintf("error walking incident directory: %v", err))
	}

	sort.Slice(incidents, func(i, j int) bool {
		return incidents[i].CreatedAt.After(incidents[j].CreatedAt)
	})
	return incidents, warnings, nil
}

// GetActiveIncident returns the most recent non-resolved incident, if any.
func (s *Store) GetActiveIncident() (*Incident, error) {
	// Look for active incidents in the last 10 incidents (heuristic for speed)
	incidents, _, err := s.ListForMonth(time.Now().Month(), time.Now().Year())
	if err != nil {
		return nil, err
	}

	for _, inc := range incidents {
		if !inc.IsResolved() {
			return &inc, nil
		}
	}
	return nil, nil
}

// GetActiveIncidentID returns the ID of the current active incident, if any.
func (s *Store) GetActiveIncidentID() (string, bool) {
	inc, err := s.GetActiveIncident()
	if err != nil || inc == nil {
		return "", false
	}
	return inc.ID, true
}

// GetActiveIncidentInfo returns the ID and service of the current active incident, if any.
func (s *Store) GetActiveIncidentInfo() (string, string, bool) {
	inc, err := s.GetActiveIncident()
	if err != nil || inc == nil {
		return "", "", false
	}
	return inc.ID, inc.Service, true
}

func (s *Store) loadFromFile(path string) (Incident, string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Incident{}, fmt.Sprintf("failed to read %s: %v", filepath.Base(path), err)
	}
	var incident Incident
	if err := json.Unmarshal(data, &incident); err != nil {
		return Incident{}, fmt.Sprintf("invalid incident file %s: %v", filepath.Base(path), err)
	}
	return incident, ""
}

func (s *Store) getPath(id string) string {
	if len(id) >= 10 && strings.HasPrefix(id, "INC-") {
		// INC-YYYYMMDD - ensure year is numeric to avoid "DEMO" partitioning
		year := id[4:8]
		if _, err := strconv.Atoi(year); err == nil {
			month := id[8:10]
			return filepath.Join(s.dir, year, month, id+".json")
		}
	}
	return filepath.Join(s.dir, id+".json")
}

func incidentDir() (string, error) {
	pm := config.GetProfileManager()
	
	// Use profile-scoped state directory
	profileStateDir := pm.GetStatePath()
	incidentDir := filepath.Join(profileStateDir, "incidents")
	
	return resolveCanonicalDataPath(incidentDir)
}

func resolveCanonicalDataPath(proposed string) (string, error) {
	clean := filepath.Clean(proposed)
	if clean == "." || hasPathTraversal(clean) {
		return "", errors.New("data dir contains traversal")
	}

	if filepath.IsAbs(clean) {
		return clean, nil
	}

	// Resolve relative to profile state directory
	pm := config.GetProfileManager()
	profileStateDir := pm.GetStatePath()
	return filepath.Join(profileStateDir, clean), nil
}

func applySudoOwnership(path string) error {
	if os.Geteuid() != 0 {
		return nil
	}
	sudoUser := strings.TrimSpace(os.Getenv("SUDO_USER"))
	if sudoUser == "" {
		return nil
	}
	u, err := user.Lookup(sudoUser)
	if err != nil {
		return fmt.Errorf("unable to resolve sudo user %q: %w", sudoUser, err)
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return fmt.Errorf("invalid sudo uid for %q: %w", sudoUser, err)
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return fmt.Errorf("invalid sudo gid for %q: %w", sudoUser, err)
	}
	if err := os.Chown(path, uid, gid); err != nil {
		return nil
	}
	return nil
}

func sanitizeIncidentPath(path string) (string, error) {
	clean := filepath.Clean(path)
	if clean == "." {
		return "", errors.New("incident path must not be '.'")
	}
	if hasPathTraversal(clean) {
		return "", errors.New("incident path contains traversal")
	}
	if info, err := os.Stat(clean); err == nil && !info.IsDir() {
		return "", fmt.Errorf("incident path is not a directory: %s", clean)
	}
	return clean, nil
}

// migrateOldHealthMonitorDir checks for and migrates old ~/.health-monitor directory
func migrateOldHealthMonitorDir() {
	home, err := os.UserHomeDir()
	if err != nil {
		return // Can't determine home directory, skip migration
	}
	
	oldDir := filepath.Join(home, ".health-monitor")
	if info, err := os.Stat(oldDir); err != nil {
		return // Old directory doesn't exist, no migration needed
	} else if !info.IsDir() {
		return // Old path exists but is not a directory, skip
	}
	
	// Check if old directory has incidents
	oldIncidentsDir := filepath.Join(oldDir, "incidents")
	if oldInfo, err := os.Stat(oldIncidentsDir); err != nil || !oldInfo.IsDir() {
		return // No old incidents directory, skip migration
	}
	
	// Check if there are files to migrate
	entries, err := os.ReadDir(oldIncidentsDir)
	if err != nil {
		return // Can't read old directory, skip migration
	} else if len(entries) == 0 {
		return // Empty old directory, skip migration
	}
	
	// Create canonical directory if it doesn't exist
	canonicalDir := "/var/lib/health-monitor/incidents"
	if err := os.MkdirAll(canonicalDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "WARN: Cannot create canonical directory for migration: %v\n", err)
		return
	}
	
	// Perform migration
	migratedCount := 0
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue // Skip non-JSON files
		}
		
		oldPath := filepath.Join(oldIncidentsDir, entry.Name())
		newPath := filepath.Join(canonicalDir, entry.Name())
		
		// Check if file already exists in canonical location
		if _, err := os.Stat(newPath); err == nil {
			continue // File already exists, skip
		}
		
		// Copy file
		if err := copyFile(oldPath, newPath); err != nil {
			fmt.Fprintf(os.Stderr, "WARN: Failed to migrate %s: %v\n", entry.Name(), err)
			continue
		}
		
		migratedCount++
	}
	
	if migratedCount > 0 {
		fmt.Fprintf(os.Stderr, "INFO: Migrated %d incident files from ~/.health-monitor to /var/lib/health-monitor/incidents\n", migratedCount)
		fmt.Fprintf(os.Stderr, "INFO: You can now safely remove: %s\n", oldIncidentsDir)
	}
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()
	
	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()
	
	_, err = io.Copy(destination, source)
	return err
}
