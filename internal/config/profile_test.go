package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func setupTestEnvironment(t *testing.T) (*ProfileManager, string) {
	originalPath := TestGetConfigBasePath()
	tempDir := t.TempDir()
	TestSetConfigBasePath(tempDir)
	t.Cleanup(func() {
		TestSetConfigBasePath(originalPath)
	})
	return GetProfileManager(), tempDir
}

func TestProfileManager(t *testing.T) {
	pm, _ := setupTestEnvironment(t)
	
	// Test profile creation and loading
	
	// Test default profile creation
	err := pm.loadDefaultProfile()
	if err != nil {
		t.Fatalf("Failed to load default profile: %v", err)
	}
	
	if pm.GetActiveProfile() != "default" {
		t.Errorf("Expected 'default' as active profile, got %s", pm.GetActiveProfile())
	}
	
	// Test setting active profile
	err = pm.SetActiveProfile("default")
	if err != nil {
		t.Fatalf("Failed to set active profile: %v", err)
	}
	
	if pm.GetActiveProfile() != "default" {
		t.Errorf("Expected 'default' active profile, got %s", pm.GetActiveProfile())
	}
}

func TestProfileIsolation(t *testing.T) {
	pm, _ := setupTestEnvironment(t)
	
	// Create two profiles with different configs
	profile1 := &Profile{
		Name: "prod",
		Config: Config{
			PrometheusURL: "http://prod-prometheus:9090",
			APIService:    "prod-service",
		},
	}
	
	profile2 := &Profile{
		Name: "dev",
		Config: Config{
			PrometheusURL: "http://dev-prometheus:9090",
			APIService:    "dev-service",
		},
	}
	
	// Save profiles
	pm.profiles["prod"] = profile1
	pm.profiles["dev"] = profile2
	
	// Test isolation
	pm.SetActiveProfile("prod")
	cfg1, err := LoadForProfile("prod")
	if err != nil {
		t.Fatalf("Failed to load prod profile: %v", err)
	}
	
	pm.SetActiveProfile("dev")
	cfg2, err := LoadForProfile("dev")
	if err != nil {
		t.Fatalf("Failed to load dev profile: %v", err)
	}
	
	if cfg1.PrometheusURL == cfg2.PrometheusURL {
		t.Error("Profile configs should be isolated")
	}
}

func TestLegacyMigration(t *testing.T) {
	_, tempDir := setupTestEnvironment(t)
	
	// Create legacy config
	legacyConfig := Config{
		PrometheusURL: "http://legacy-prometheus:9090",
		APIService:    "legacy-service",
	}
	
	legacyPath := filepath.Join(tempDir, "config.json")
	legacyData, err := json.Marshal(legacyConfig)
	if err != nil {
		t.Fatalf("Failed to marshal legacy config: %v", err)
	}
	
	if err := os.WriteFile(legacyPath, legacyData, 0600); err != nil {
		t.Fatalf("Failed to write legacy config: %v", err)
	}
	
	// Test migration
	pm := GetProfileManager()
	
	err = pm.migrateLegacyConfig()
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}
	
	// Verify migration
	profilePath := filepath.Join(tempDir, "default.yaml")
	if _, err := os.Stat(profilePath); os.IsNotExist(err) {
		t.Error("Profile file not created after migration")
	}
	
	// Verify backup
	backupPath := legacyPath + ".backup"
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Error("Legacy config not backed up")
	}
	
	// Verify loaded config
	cfg, err := LoadForProfile("default")
	if err != nil {
		t.Fatalf("Failed to load migrated profile: %v", err)
	}
	
	if cfg.PrometheusURL != legacyConfig.PrometheusURL {
		t.Error("Migrated config doesn't match original")
	}
}

func TestEnvironmentOverrides(t *testing.T) {
	pm, _ := setupTestEnvironment(t)
	
	// Create profile
	profile := &Profile{
		Name: "test",
		Config: Config{
			PrometheusURL: "http://default-prometheus:9090",
		},
	}
	
	pm.profiles["test"] = profile
	
	// Set profile-scoped environment variable
	os.Setenv("HEALTH_MONITOR_PROFILES__TEST__PROMETHEUS_URL", "http://env-prometheus:9090")
	defer os.Unsetenv("HEALTH_MONITOR_PROFILES__TEST__PROMETHEUS_URL")
	
	// Load config with env override
	cfg, err := LoadForProfile("test")
	if err != nil {
		t.Fatalf("Failed to load profile: %v", err)
	}
	
	if cfg.PrometheusURL != "http://env-prometheus:9090" {
		t.Errorf("Expected env override, got %s", cfg.PrometheusURL)
	}
}

func TestProfilePaths(t *testing.T) {
	pm, tempDir := setupTestEnvironment(t)
	
	// Test default profile path
	defaultPath := pm.getProfilePath("default")
	expectedDefault := filepath.Join(tempDir, "default.yaml")
	if defaultPath != expectedDefault {
		t.Errorf("Expected default path %s, got %s", expectedDefault, defaultPath)
	}
	
	// Test custom profile path
	customPath := pm.getProfilePath("prod")
	expectedCustom := filepath.Join(tempDir, "profiles", "prod.yaml")
	if customPath != expectedCustom {
		t.Errorf("Expected custom path %s, got %s", expectedCustom, customPath)
	}
	
	// Test state path
	statePath := pm.getStatePath("prod")
	expectedState := filepath.Join(tempDir, "state", "prod")
	if statePath != expectedState {
		t.Errorf("Expected state path %s, got %s", expectedState, statePath)
	}
}

func TestProfileSaveAndLoad(t *testing.T) {
	pm, _ := setupTestEnvironment(t)
	
	// Create and save a profile
	originalConfig := Default()
	originalConfig.PrometheusURL = "http://test-prometheus:9090"
	originalConfig.APIService = "test-service"
	
	err := SaveForProfile("test", originalConfig)
	if err != nil {
		t.Fatalf("Failed to save profile: %v", err)
	}
	
	// Load the profile
	loadedConfig, err := LoadForProfile("test")
	if err != nil {
		t.Fatalf("Failed to load profile: %v", err)
	}
	
	// Verify config (excluding secrets)
	if loadedConfig.PrometheusURL != originalConfig.PrometheusURL {
		t.Error("Prometheus URL not preserved")
	}
	if loadedConfig.APIService != originalConfig.APIService {
		t.Error("API service not preserved")
	}
	
	// Verify secrets are not saved
	profilePath := pm.getProfilePath("test")
	data, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatalf("Failed to read profile file: %v", err)
	}
	
	var savedProfile Profile
	if err := yaml.Unmarshal(data, &savedProfile); err != nil {
		t.Fatalf("Failed to unmarshal profile: %v", err)
	}
	
	if savedProfile.Config.PrometheusToken != "" {
		t.Error("Secret token should not be saved")
	}
}

func TestProfileConcurrentAccess(t *testing.T) {
	// Test concurrent access
	setupTestEnvironment(t)
	done := make(chan bool, 10)
	
	for i := 0; i < 10; i++ {
		go func(id int) {
			profileName := fmt.Sprintf("profile%d", id)
			
			// Create profile
			config := Default()
			config.PrometheusURL = fmt.Sprintf("http://prometheus%d:9090", id)
			
			err := SaveForProfile(profileName, config)
			if err != nil {
				t.Errorf("Failed to save profile %d: %v", id, err)
				done <- false
				return
			}
			
			// Load profile
			loaded, err := LoadForProfile(profileName)
			if err != nil {
				t.Errorf("Failed to load profile %d: %v", id, err)
				done <- false
				return
			}
			
			if loaded.PrometheusURL != config.PrometheusURL {
				t.Errorf("Profile %d config mismatch", id)
				done <- false
				return
			}
			
			done <- true
		}(i)
	}
	
	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		if !<-done {
			t.Error("Concurrent access test failed")
			break
		}
	}
}

func TestProfileValidation(t *testing.T) {
	setupTestEnvironment(t)
	// Test valid profile
	validConfig := Default()
	validConfig.PrometheusURL = "http://prometheus:9090"
	
	err := SaveForProfile("valid", validConfig)
	if err != nil {
		t.Fatalf("Failed to save valid profile: %v", err)
	}
	
	cfg, err := LoadForProfile("valid")
	if err != nil {
		t.Fatalf("Failed to load valid profile: %v", err)
	}
	
	missing := MissingRequired(cfg)
	if len(missing) > 0 {
		t.Errorf("Valid profile should have no missing fields, got: %v", missing)
	}
	
	// Test invalid profile
	invalidConfig := Default()
	// PrometheusURL is empty by default
	
	err = SaveForProfile("invalid", invalidConfig)
	if err != nil {
		t.Fatalf("Failed to save invalid profile: %v", err)
	}
	
	cfg, err = LoadForProfile("invalid")
	if err != nil {
		t.Fatalf("Failed to load invalid profile: %v", err)
	}
	
	missing = MissingRequired(cfg)
	if len(missing) == 0 {
		t.Error("Invalid profile should have missing fields")
	}
}

func TestProfileBackwardCompatibility(t *testing.T) {
	_, tempDir := setupTestEnvironment(t)
	
	// Simulate existing installation with legacy config
	legacyConfig := Config{
		PrometheusURL: "http://old-prometheus:9090",
		APIService:    "old-service",
		LokiURL:       "http://old-loki:3100",
	}
	
	legacyPath := filepath.Join(tempDir, "config.json")
	legacyData, err := json.Marshal(legacyConfig)
	if err != nil {
		t.Fatalf("Failed to marshal legacy config: %v", err)
	}
	
	if err := os.WriteFile(legacyPath, legacyData, 0600); err != nil {
		t.Fatalf("Failed to write legacy config: %v", err)
	}
	
	// Initialize profile manager with legacy config
	pm := GetProfileManager()
	
	// Load should trigger migration
	err = pm.SetActiveProfile("default")
	if err != nil {
		t.Fatalf("Failed to set active profile: %v", err)
	}
	
	// Verify migration happened
	profilePath := filepath.Join(tempDir, "default.yaml")
	if _, err := os.Stat(profilePath); os.IsNotExist(err) {
		t.Error("Profile not created during migration")
	}
	
	// Verify config is accessible
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Failed to load migrated config: %v", err)
	}
	
	if cfg.PrometheusURL != legacyConfig.PrometheusURL {
		t.Error("Prometheus URL not migrated correctly")
	}
	if cfg.LokiURL != legacyConfig.LokiURL {
		t.Error("Loki URL not migrated correctly")
	}
}
