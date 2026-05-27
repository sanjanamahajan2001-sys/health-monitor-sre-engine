package test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestProfileCLIIntegration(t *testing.T) {
	tempDir := t.TempDir()
	
	// Set up test environment
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")
	
	// Create test profiles
	createTestProfile(t, tempDir, "prod", map[string]interface{}{
		"prometheus_url": "http://prod-prometheus:9090",
		"api_service":    "prod-service",
	})
	
	createTestProfile(t, tempDir, "dev", map[string]interface{}{
		"prometheus_url": "http://dev-prometheus:9090", 
		"api_service":    "dev-service",
	})
	
	// Test profile list
	cmd := exec.Command("go", "run", "cmd/health-monitor/main.go", "profile", "list")
	cmd.Dir = ".."
	cmd.Env = append(cmd.Env, "HOME="+tempDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Profile list failed: %v\n%s", err, output)
	}
	
	if !contains(string(output), "prod") || !contains(string(output), "dev") {
		t.Error("Expected profiles not found in list output")
	}
	
	// Test profile-specific config loading
	cmd = exec.Command("go", "run", "cmd/health-monitor/main.go", "--profile", "prod", "profile", "show")
	cmd.Env = append(cmd.Env, "HOME="+tempDir)
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Prod config show failed: %v\n%s", err, output)
	}
	
	if !contains(string(output), "prod-prometheus") {
		t.Error("Prod profile config not loaded correctly")
	}
}

func TestProfileStateIsolation(t *testing.T) {
	tempDir := t.TempDir()
	
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")
	
	// Create test profile
	createTestProfile(t, tempDir, "test", map[string]interface{}{
		"prometheus_url": "http://test-prometheus:9090",
		"api_service":    "test-service",
	})
	
	// Create incident in test profile
	cmd := exec.Command("go", "run", "cmd/health-monitor/main.go", "--profile", "test", "incident", "start", 
		"--service", "test-service", "--severity", "P1", "--title", "Test incident")
	cmd.Dir = ".."
	cmd.Env = append(cmd.Env, "HOME="+tempDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Incident start failed: %v\n%s", err, output)
	}
	
	// Verify incident exists in test profile
	cmd = exec.Command("go", "run", "cmd/health-monitor/main.go", "--profile", "test", "incident", "list")
	cmd.Env = append(cmd.Env, "HOME="+tempDir)
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Incident list failed: %v\n%s", err, output)
	}
	
	if !contains(string(output), "Test incident") {
		t.Error("Incident not found in test profile")
	}
	
	// Verify incident doesn't exist in default profile
	cmd = exec.Command("go", "run", "cmd/health-monitor/main.go", "incident", "list")
	cmd.Env = append(cmd.Env, "HOME="+tempDir)
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Default incident list failed: %v\n%s", err, output)
	}
	
	if contains(string(output), "Test incident") {
		t.Error("Incident leaked to default profile")
	}
}

func TestProfileEnvironmentOverrides(t *testing.T) {
	tempDir := t.TempDir()
	
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")
	
	// Create base profile
	createTestProfile(t, tempDir, "test", map[string]interface{}{
		"prometheus_url": "http://base-prometheus:9090",
		"api_service":    "base-service",
	})
	
	// Set profile-specific environment variable
	os.Setenv("HEALTH_MONITOR_PROFILES__TEST__PROMETHEUS_URL", "http://env-prometheus:9090")
	defer os.Unsetenv("HEALTH_MONITOR_PROFILES__TEST__PROMETHEUS_URL")
	
	// Test that env override is applied
	cmd := exec.Command("go", "run", "cmd/health-monitor/main.go", "--profile", "test", "profile", "show")
	cmd.Dir = ".."
	cmd.Env = append(cmd.Env, "HOME="+tempDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Profile show failed: %v\n%s", err, output)
	}
	
	if !contains(string(output), "env-prometheus") {
		t.Error("Environment override not applied")
	}
}

func TestProfileBackwardCompatibility(t *testing.T) {
	tempDir := t.TempDir()
	
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")
	
	// Create legacy config
	legacyConfig := `{
		"prometheus_url": "http://legacy-prometheus:9090",
		"api_service": "legacy-service"
	}`
	
	legacyPath := filepath.Join(tempDir, ".health-monitor", "config.json")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0755); err != nil {
		t.Fatalf("Failed to create legacy config directory: %v", err)
	}
	
	if err := os.WriteFile(legacyPath, []byte(legacyConfig), 0600); err != nil {
		t.Fatalf("Failed to write legacy config: %v", err)
	}
	
	// Test that legacy config still works
	cmd := exec.Command("go", "run", "cmd/health-monitor/main.go", "profile", "show")
	cmd.Dir = ".."
	cmd.Env = append(cmd.Env, "HOME="+tempDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Legacy config test failed: %v\n%s", err, output)
	}
	
	if !contains(string(output), "legacy-prometheus") {
		t.Error("Legacy config not loaded correctly")
	}
	
	// Verify migration happened
	profilePath := filepath.Join(tempDir, ".health-monitor", "default.yaml")
	if _, err := os.Stat(profilePath); os.IsNotExist(err) {
		t.Error("Profile not created during migration")
	}
	
	// Verify backup exists
	backupPath := legacyPath + ".backup"
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Error("Legacy config not backed up")
	}
}

func TestProfilePerformance(t *testing.T) {
	tempDir := t.TempDir()
	
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")
	
	// Create multiple profiles
	for i := 0; i < 10; i++ {
		profileName := fmt.Sprintf("profile%d", i)
		createTestProfile(t, tempDir, profileName, map[string]interface{}{
			"prometheus_url": fmt.Sprintf("http://prometheus%d:9090", i),
			"api_service":    fmt.Sprintf("service%d", i),
		})
	}
	
	// Test profile switching performance
	start := time.Now()
	for i := 0; i < 10; i++ {
		profileName := fmt.Sprintf("profile%d", i)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, "go", "run", "cmd/health-monitor/main.go", "--profile", profileName, "profile", "show")
		cmd.Dir = ".."
		cmd.Env = append(cmd.Env, "HOME="+tempDir)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Profile %s show failed: %v\n%s", profileName, err, output)
		}
		
		expectedURL := fmt.Sprintf("prometheus%d", i)
		if !contains(string(output), expectedURL) {
			t.Errorf("Profile %s not loaded correctly", profileName)
		}
	}
	duration := time.Since(start)
	
	// Should complete within reasonable time (adjust threshold as needed)
	if duration > 30*time.Second {
		t.Errorf("Profile switching took too long: %v", duration)
	}
	
	t.Logf("Profile switching performance: %v for 10 profiles", duration)
}

func TestProfileConcurrentAccess(t *testing.T) {
	tempDir := t.TempDir()
	
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")
	
	// Create test profile
	createTestProfile(t, tempDir, "concurrent", map[string]interface{}{
		"prometheus_url": "http://concurrent-prometheus:9090",
		"api_service":    "concurrent-service",
	})
	
	// Test concurrent access to same profile
	done := make(chan bool, 5)
	
	for i := 0; i < 5; i++ {
		go func(id int) {
			cmd := exec.Command("go", "run", "cmd/health-monitor/main.go", "--profile", "concurrent", "profile", "show")
			cmd.Dir = ".."
			cmd.Env = append(cmd.Env, "HOME="+tempDir)
			
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Errorf("Concurrent access %d failed: %v\n%s", id, err, output)
				done <- false
				return
			}
			
			if !contains(string(output), "concurrent-prometheus") {
				t.Errorf("Concurrent access %d got wrong config", id)
				done <- false
				return
			}
			
			done <- true
		}(i)
	}
	
	// Wait for all goroutines
	for i := 0; i < 5; i++ {
		if !<-done {
			t.Error("Concurrent access test failed")
			break
		}
	}
}

func createTestProfile(t *testing.T, baseDir, profileName string, config map[string]interface{}) {
	profileDir := filepath.Join(baseDir, ".health-monitor", "profiles")
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		t.Fatalf("Failed to create profile directory: %v", err)
	}
	
	profilePath := filepath.Join(profileDir, profileName+".yaml")
	
	profileData := map[string]interface{}{
		"name":   profileName,
		"config": config,
	}
	
	data, err := yaml.Marshal(profileData)
	if err != nil {
		t.Fatalf("Failed to marshal profile: %v", err)
	}
	
	if err := os.WriteFile(profilePath, data, 0600); err != nil {
		t.Fatalf("Failed to write profile: %v", err)
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
