package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"health-monitor/internal/storage"

	"gopkg.in/yaml.v3"
)

type Profile struct {
	Name        string            `yaml:"name" json:"name"`
	Environment string            `yaml:"environment,omitempty" json:"environment,omitempty"`
	Cluster     string            `yaml:"cluster,omitempty" json:"cluster,omitempty"`
	Provider    string            `yaml:"provider,omitempty" json:"provider,omitempty"` // e.g., "aws", "gcp", "on-prem"
	Region      string            `yaml:"region,omitempty" json:"region,omitempty"`
	Namespaces  []string          `yaml:"namespaces,omitempty" json:"namespaces,omitempty"`
	Zone        string            `yaml:"zone,omitempty" json:"zone,omitempty"`
	Config      Config            `yaml:"config" json:"config"`
	Metadata    map[string]string `yaml:"metadata,omitempty" json:"metadata,omitempty"`
}

type ProfileManager struct {
	activeProfile string
	profiles      map[string]*Profile
	mutex         sync.RWMutex
	basePath      string
}

var globalProfileManager *ProfileManager
var profileManagerMu sync.Mutex

func GetProfileManager() *ProfileManager {
	profileManagerMu.Lock()
	defer profileManagerMu.Unlock()
	
	if globalProfileManager == nil {
		globalProfileManager = &ProfileManager{
			profiles: make(map[string]*Profile),
			basePath: getConfigBasePath(),
		}
		// Load the persisted active profile
		globalProfileManager.loadActiveProfile()
}
	return globalProfileManager
}

// Reset clears the global profile manager to force re-initialization (used for demo sandbox)
func Reset() {
	profileManagerMu.Lock()
	defer profileManagerMu.Unlock()
	globalProfileManager = nil
}

func (pm *ProfileManager) SetActiveProfile(name string) error {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	if name == "" {
		name = "default"
	}

	// Check if profile already exists and has valid data
	if profile, exists := pm.profiles[name]; exists {
		// Check if the profile has actual config data (not just empty defaults)
		if profile.Config.PrometheusURL != "" || name != "default" {
			pm.activeProfile = name
			// Persist the active profile
			if err := pm.saveActiveProfile(name); err != nil {
				return fmt.Errorf("failed to persist active profile: %w", err)
			}
			return nil
		}
	}
	
	// Try to load the profile
	err := pm.loadProfile(name)
	if err != nil {
		return err
	}

	pm.activeProfile = name
	// Persist the active profile
	if err := pm.saveActiveProfile(name); err != nil {
		return fmt.Errorf("failed to persist active profile: %w", err)
	}
	return nil
}

func (pm *ProfileManager) GetActiveProfile() string {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()
	return pm.activeProfile
}

func (pm *ProfileManager) GetConfigBasePath() string {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()
	return pm.basePath
}

// saveActiveProfile persists the active profile to a file
func (pm *ProfileManager) saveActiveProfile(profileName string) error {
	activeProfilePath := filepath.Join(pm.basePath, ".active-profile")
	err := storage.AtomicWriteFile(activeProfilePath, []byte(profileName), 0600)
	if err != nil && os.IsPermission(err) {
		return fmt.Errorf("permission denied writing to %s. Try running with 'sudo' or check folder permissions", activeProfilePath)
	}
	return err
}

// loadActiveProfile loads the active profile from file
func (pm *ProfileManager) loadActiveProfile() error {
	activeProfilePath := filepath.Join(pm.basePath, ".active-profile")
	if data, err := os.ReadFile(activeProfilePath); err == nil {
		profileName := strings.TrimSpace(string(data))
		if profileName != "" {
			pm.activeProfile = profileName
			return nil
		}
	}
	// Default to "default" if no active profile file exists
	pm.activeProfile = "default"
	return nil
}

func (pm *ProfileManager) GetProfile(name string) (*Profile, error) {
	pm.mutex.RLock()
	profile, exists := pm.profiles[name]
	pm.mutex.RUnlock()

	if exists {
		return profile, nil
	}

	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	if profile, exists := pm.profiles[name]; exists {
		return profile, nil
	}

	if err := pm.loadProfileByName(name); err != nil {
		return nil, err
	}

	return pm.profiles[name], nil
}

// TransactionalReload loads a new version of the specified profile into a temporary location,
// validates it, and then atomically swaps it into the active profiles map.
func (pm *ProfileManager) TransactionalReload(name string) error {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	// 1. Create a "staging" copy of the profile (defaults + current overrides)
	staging := Profile{Config: Default()}
	profilePath := pm.getProfilePath(name)

	// 2. Load and validate YAML from disk
	data, err := os.ReadFile(profilePath)
	if err != nil {
		return fmt.Errorf("failed to read profile for reload: %w", err)
	}
	if err := yaml.Unmarshal(data, &staging); err != nil {
		return fmt.Errorf("invalid profile YAML (discarding reload): %w", err)
	}
	staging.Name = name

	// 3. Basic Validation (Can be expanded)
	if staging.Config.PrometheusURL == "" && name != "default" {
		return fmt.Errorf("invalid config: prometheus_url is missing")
	}

	// 4. Atomic Swap
	pm.profiles[name] = &staging
	return nil
}

func (pm *ProfileManager) loadProfile(name string) error {
	// Try to load profile by name
	profilePath := pm.getProfilePath(name)

	if data, err := os.ReadFile(profilePath); err == nil {
		profile := Profile{Config: Default()}
		if err := yaml.Unmarshal(data, &profile); err != nil {
			return fmt.Errorf("invalid profile YAML: %w", err)
		}
		profile.Name = name
		pm.profiles[name] = &profile
		return nil
	}
	
	// If it's the default profile, try the special loading logic
	if name == "default" {
		return pm.loadDefaultProfile()
	}

	return fmt.Errorf("profile %q not found", name)
}

func (pm *ProfileManager) loadProfileByName(name string) error {
	profilePath := pm.getProfilePath(name)

	if data, err := os.ReadFile(profilePath); err == nil {
		profile := Profile{Config: Default()}
		if err := yaml.Unmarshal(data, &profile); err != nil {
			return fmt.Errorf("invalid profile YAML: %w", err)
		}
		profile.Name = name
		pm.profiles[name] = &profile
		return nil
	}

	if name == "default" {
		return pm.loadDefaultProfile()
	}

	return fmt.Errorf("profile %q not found", name)
}

func (pm *ProfileManager) loadDefaultProfile() error {
	// Check if default profile exists
	defaultPath := pm.getProfilePath("default")
	
	if _, err := os.Stat(defaultPath); err == nil {
		// Load existing default profile
		return pm.loadProfileFromPath(defaultPath)
	}
	
	// Check for legacy config and migrate
	legacyPath := filepath.Join(pm.basePath, "config.json")
	if _, err := os.Stat(legacyPath); err == nil {
		return pm.migrateLegacyConfig()
	}
	
	// Create default profile
	defaultConfig := Default()
	profile := &Profile{
		Name:   "default",
		Config: defaultConfig,
	}
	
	pm.profiles["default"] = profile
	return pm.saveProfile("default")
}

func (pm *ProfileManager) saveProfile(name string) error {
	profile, exists := pm.profiles[name]
	if !exists {
		return fmt.Errorf("profile %q not found", name)
	}

	profilePath := pm.getProfilePath(name)

	data, err := yaml.Marshal(profile)
	if err != nil {
		return err
	}

	err = storage.AtomicWriteFile(profilePath, data, 0600)
	if err != nil && os.IsPermission(err) {
		return fmt.Errorf("permission denied writing to %s. Try running with 'sudo' or check folder permissions", profilePath)
	}
	return err
}

func (pm *ProfileManager) getProfilePath(name string) string {
	if name == "default" {
		return filepath.Join(pm.basePath, "default.yaml")
	}
	return filepath.Join(pm.basePath, "profiles", name+".yaml")
}

func (pm *ProfileManager) getStatePath(name string) string {
	return filepath.Join(pm.basePath, "state", name)
}

func (pm *ProfileManager) GetStatePath() string {
	return pm.getStatePath(pm.GetActiveProfile())
}

func (pm *ProfileManager) GetStatePathForProfile(name string) string {
	return pm.getStatePath(name)
}

func (pm *ProfileManager) GetProfilePath(name string) string {
	return pm.getProfilePath(name)
}

func (pm *ProfileManager) GetProfilesPath() string {
	return filepath.Join(pm.basePath, "profiles")
}

func (pm *ProfileManager) GetFlowsPath() string {
	return filepath.Join(pm.basePath, "flows.d")
}

func (pm *ProfileManager) GetAlertsPath() string {
	return filepath.Join(pm.basePath, "alerts.d")
}

// GetActiveProfileName returns the current active profile name (utility function)
func GetActiveProfileName() string {
	pm := GetProfileManager()
	profile := pm.GetActiveProfile()
	if profile == "" {
		return "default"
	}
	return profile
}

// GetAlertsPath returns the alerts path for current active profile
func GetAlertsPath() string {
	pm := GetProfileManager()
	return pm.GetAlertsPathForProfile(pm.GetActiveProfile())
}

// GetAlertsPathForProfile returns the alerts path for specific profile
func (pm *ProfileManager) GetAlertsPathForProfile(name string) string {
	// Check profile-specific in profiles directory first
	profileAlertsPath := filepath.Join(pm.basePath, "profiles", name+"-alerts.yaml")
	if _, err := os.Stat(profileAlertsPath); err == nil {
		return profileAlertsPath
	}
	
	// Check profile-specific in alerts.d directory (existing pattern)
	profileAlertsDPath := filepath.Join(pm.basePath, "alerts.d", name+".yaml")
	if _, err := os.Stat(profileAlertsDPath); err == nil {
		return profileAlertsDPath
	}
	
	// Fallback to base alerts.yaml
	baseAlertsPath := filepath.Join(pm.basePath, "alerts.yaml")
	if _, err := os.Stat(baseAlertsPath); err == nil {
		return baseAlertsPath
	}
	
	return ""
}

// GetFlowsPath returns the flows path for current active profile
func GetFlowsPath() string {
	pm := GetProfileManager()
	return pm.GetFlowsPathForProfile(pm.GetActiveProfile())
}

// GetFlowsPathForProfile returns the flows path for specific profile
func (pm *ProfileManager) GetFlowsPathForProfile(name string) string {
	// Check profile-specific in profiles directory first
	profileFlowsPath := filepath.Join(pm.basePath, "profiles", name+"-flows.yaml")
	if _, err := os.Stat(profileFlowsPath); err == nil {
		return profileFlowsPath
	}
	
	// Check profile-specific in flows.d directory (existing pattern)
	profileFlowsDPath := filepath.Join(pm.basePath, "flows.d", name+".yaml")
	if _, err := os.Stat(profileFlowsDPath); err == nil {
		return profileFlowsDPath
	}
	
	// Fallback to base flows.yaml
	baseFlowsPath := filepath.Join(pm.basePath, "flows.yaml")
	if _, err := os.Stat(baseFlowsPath); err == nil {
		return baseFlowsPath
	}
	
	return ""
}

func (pm *ProfileManager) loadProfileFromPath(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	
	profile := Profile{Config: Default()}
	if err := yaml.Unmarshal(data, &profile); err != nil {
		return fmt.Errorf("invalid profile YAML: %w", err)
	}
	
	// Extract profile name from filename if not set in YAML
	if profile.Name == "" {
		profile.Name = filepath.Base(strings.TrimSuffix(path, filepath.Ext(path)))
	}
	
	pm.profiles[profile.Name] = &profile
	return nil
}

func (pm *ProfileManager) migrateLegacyConfig() error {
	legacyPath := filepath.Join(pm.basePath, "config.json")

	data, err := os.ReadFile(legacyPath)
	if err != nil {
		return err
	}

	var legacyConfig Config
	if err := json.Unmarshal(data, &legacyConfig); err != nil {
		return err
	}

	profile := &Profile{
		Name:   "default",
		Config: legacyConfig,
	}

	pm.profiles["default"] = profile

	if err := pm.saveProfile("default"); err != nil {
		return err
	}

	backupPath := legacyPath + ".backup"
	return os.Rename(legacyPath, backupPath)
}

func (pm *ProfileManager) ListProfiles() []string {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	var names []string
	for name := range pm.profiles {
		names = append(names, name)
	}
	return names
}

// DiscoverProfiles discovers all available profile files on disk
func (pm *ProfileManager) DiscoverProfiles() []string {
	var profiles []string
	seen := make(map[string]bool)
	
	searchPaths := []string{pm.basePath}
	// If we are NOT already looking at /etc, add it as a secondary discovery path
	// This allows non-sudo runs to "see" system-wide profiles
	if pm.basePath != "/etc/health-monitor" {
		searchPaths = append(searchPaths, "/etc/health-monitor")
	}

	for _, basePath := range searchPaths {
		// Check for default profile
		if _, err := os.Stat(filepath.Join(basePath, "default.yaml")); err == nil {
			if !seen["default"] {
				profiles = append(profiles, "default")
				seen["default"] = true
			}
		}
		
		// Check profiles directory
		profilesDir := filepath.Join(basePath, "profiles")
		if entries, err := os.ReadDir(profilesDir); err == nil {
			for _, entry := range entries {
				if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".yaml") {
					name := entry.Name()
					// Filter out security/internal config files
					if strings.HasSuffix(name, "-security.yaml") || 
					   strings.HasSuffix(name, "-config.yaml") || 
					   name == "security-config.yaml" {
						continue
					}
					profileName := strings.TrimSuffix(name, ".yaml")
					if !seen[profileName] {
						profiles = append(profiles, profileName)
						seen[profileName] = true
					}
				}
			}
		}
	}
	
	return profiles
}

// LoadAllProfiles loads all discovered profiles into memory
func (pm *ProfileManager) LoadAllProfiles() error {
	profiles := pm.DiscoverProfiles()
	
	for _, profileName := range profiles {
		if _, exists := pm.profiles[profileName]; !exists {
			if err := pm.loadProfile(profileName); err != nil {
				fmt.Printf("Warning: Failed to load profile %s: %v\n", profileName, err)
			}
		}
	}
	
	return nil
}
