package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetAvailablePresets(t *testing.T) {
	presets := GetAvailablePresets()
	
	// Should have at least the basic presets
	assert.Greater(t, len(presets), 0, "Should have at least one preset")
	
	// Check for required presets
	presetNames := make(map[string]bool)
	for _, preset := range presets {
		presetNames[preset.Name] = true
	}
	
	requiredPresets := []string{
		"small-team",
		"medium-team", 
		"large-team",
		"enterprise-team",
		"devops-team",
		"sre-team",
	}
	
	for _, required := range requiredPresets {
		assert.True(t, presetNames[required], "Should have preset: %s", required)
	}
}

func TestLoadPreset(t *testing.T) {
	tests := []struct {
		name        string
		presetName  string
		expectError bool
	}{
		{
			name:        "Valid small team preset",
			presetName:  "small-team",
			expectError: false,
		},
		{
			name:        "Valid medium team preset",
			presetName:  "medium-team",
			expectError: false,
		},
		{
			name:        "Valid SRE team preset",
			presetName:  "sre-team",
			expectError: false,
		},
		{
			name:        "Invalid preset name",
			presetName:  "nonexistent-preset",
			expectError: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			preset, err := LoadPreset(tt.presetName)
			
			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, TeamPreset{}, preset)
			} else {
				assert.NoError(t, err)
				assert.NotEqual(t, TeamPreset{}, preset)
				assert.Equal(t, tt.presetName, preset.Name)
				assert.NotEmpty(t, preset.Description)
				assert.NotEmpty(t, preset.TeamSize)
				assert.NotEmpty(t, preset.Complexity)
			}
		})
	}
}

func TestPresetStructure(t *testing.T) {
	presets := GetAvailablePresets()
	
	for _, preset := range presets {
		t.Run(preset.Name, func(t *testing.T) {
			// Check basic structure
			assert.NotEmpty(t, preset.Name, "Preset should have a name")
			assert.NotEmpty(t, preset.Description, "Preset should have a description")
			assert.NotEmpty(t, preset.TeamSize, "Preset should have team size")
			assert.NotEmpty(t, preset.Complexity, "Preset should have complexity level")
			
			// Check profile structure
			assert.NotEmpty(t, preset.Profile.Name, "Profile should have a name")
			assert.NotEmpty(t, preset.Profile.Config.PrometheusServiceLabel, "Should have service label")
			assert.True(t, preset.Profile.Config.AutoDiscover, "Should have auto discovery enabled")
			
			// Check flows structure
			for flowName, flow := range preset.Flows {
				assert.NotEmpty(t, flowName, "Flow should have a name")
				assert.NotEmpty(t, flow.Name, "Flow should have a display name")
				assert.NotEmpty(t, flow.Services, "Flow should have services")
				assert.NotEmpty(t, flow.SLOs, "Flow should have SLOs")
				
				for _, slo := range flow.SLOs {
					assert.NotEmpty(t, slo.ID, "SLO should have an ID")
					assert.Greater(t, slo.Objective, 0.0, "SLO should have positive objective")
					assert.LessOrEqual(t, slo.Objective, 100.0, "SLO objective should be <= 100")
					assert.NotEmpty(t, slo.Window, "SLO should have a time window")
					assert.NotEmpty(t, slo.Type, "SLO should have a type")
					assert.NotEmpty(t, slo.Description, "SLO should have a description")
					
					if slo.Type == "ratio" {
						assert.NotEmpty(t, slo.ErrorQuery, "Ratio SLO should have error query")
						assert.NotEmpty(t, slo.TotalQuery, "Ratio SLO should have total query")
					} else if slo.Type == "latency" {
						assert.NotEmpty(t, slo.LatencyQuery, "Latency SLO should have latency query")
						assert.Greater(t, slo.Threshold, 0.0, "Latency SLO should have threshold")
					}
				}
			}
			
			// Check alerts structure
			for _, alert := range preset.Alerts {
				assert.NotEmpty(t, alert.Match, "Alert should have match conditions")
				assert.NotEmpty(t, alert.Severity, "Alert should have severity")
				assert.NotEmpty(t, alert.Mode, "Alert should have mode")
				assert.NotEmpty(t, alert.Title, "Alert should have title")
			}
			
			// Check quick start structure
			assert.NotEmpty(t, preset.QuickStart.SetupSteps, "Should have setup steps")
			assert.NotEmpty(t, preset.QuickStart.BestPractices, "Should have best practices")
			
			for _, step := range preset.QuickStart.SetupSteps {
				assert.Greater(t, step.Step, 0, "Setup step should have positive step number")
				assert.NotEmpty(t, step.Title, "Setup step should have title")
				assert.NotEmpty(t, step.Description, "Setup step should have description")
			}
		})
	}
}

func TestCreateProfileFromPreset(t *testing.T) {
	// Create temporary directory for testing
	tempDir, err := os.MkdirTemp("", "health-monitor-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)
	
	// Create required subdirectories
	os.MkdirAll(filepath.Join(tempDir, "profiles"), 0755)
	os.MkdirAll(filepath.Join(tempDir, "state"), 0755)
	os.MkdirAll(filepath.Join(tempDir, "flows.d"), 0755)
	os.MkdirAll(filepath.Join(tempDir, "alerts.d"), 0755)
	
	// Override the config base path for testing
	originalPath := TestGetConfigBasePath()
	TestSetConfigBasePath(tempDir)
	defer TestSetConfigBasePath(originalPath)
	
	tests := []struct {
		name        string
		presetName  string
		profileName string
		expectError bool
	}{
		{
			name:        "Create small team profile",
			presetName:  "small-team",
			profileName: "test-small",
			expectError: false,
		},
		{
			name:        "Create SRE team profile",
			presetName:  "sre-team",
			profileName: "test-sre",
			expectError: false,
		},
		{
			name:        "Invalid preset name",
			presetName:  "nonexistent",
			profileName: "test-invalid",
			expectError: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CreateProfileFromPreset(tt.presetName, tt.profileName)
			
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				
				// Verify profile was created
				pm := GetProfileManager()
				profile, err := pm.GetProfile(tt.profileName)
				assert.NoError(t, err)
				assert.Equal(t, tt.profileName, profile.Name)
				
				// Verify profile file exists
				profilePath := filepath.Join(pm.GetProfilesPath(), tt.profileName+".yaml")
				_, err = os.Stat(profilePath)
				assert.NoError(t, err)
				
				// Verify state directory was created
				statePath := pm.GetStatePathForProfile(tt.profileName)
				_, err = os.Stat(statePath)
				assert.NoError(t, err)
				
				// Verify flows file exists if preset has flows
				preset, _ := LoadPreset(tt.presetName)
				if len(preset.Flows) > 0 {
					flowsPath := filepath.Join(pm.GetFlowsPath(), tt.profileName+".yaml")
					_, err = os.Stat(flowsPath)
					assert.NoError(t, err)
				}
				
				// Verify alerts file exists if preset has alerts
				if len(preset.Alerts) > 0 {
					alertsPath := filepath.Join(pm.GetAlertsPath(), tt.profileName+".yaml")
					_, err = os.Stat(alertsPath)
					assert.NoError(t, err)
				}
			}
		})
	}
}

func TestPresetProgression(t *testing.T) {
	// Test that presets properly progress from simple to complex
	
	// Check small team preset
	smallPreset, err := LoadPreset("small-team")
	require.NoError(t, err)
	assert.Equal(t, "small", smallPreset.TeamSize)
	assert.Equal(t, "simple", smallPreset.Complexity)
	assert.LessOrEqual(t, len(smallPreset.Flows), 2, "Small team should have few flows")
	assert.LessOrEqual(t, len(smallPreset.Alerts), 2, "Small team should have few alerts")
	
	// Check medium team preset
	mediumPreset, err := LoadPreset("medium-team")
	require.NoError(t, err)
	assert.Equal(t, "medium", mediumPreset.TeamSize)
	assert.Equal(t, "moderate", mediumPreset.Complexity)
	assert.Greater(t, len(mediumPreset.Flows), len(smallPreset.Flows), "Medium team should have more flows than small")
	assert.GreaterOrEqual(t, len(mediumPreset.Alerts), len(smallPreset.Alerts), "Medium team should have >= alerts than small")
	
	// Check large team preset
	largePreset, err := LoadPreset("large-team")
	require.NoError(t, err)
	assert.Equal(t, "large", largePreset.TeamSize)
	assert.Equal(t, "complex", largePreset.Complexity)
	assert.Greater(t, len(largePreset.Flows), len(mediumPreset.Flows), "Large team should have more flows than medium")
	assert.GreaterOrEqual(t, len(largePreset.Alerts), len(mediumPreset.Alerts), "Large team should have >= alerts than medium")
	
	// Check enterprise preset
	enterprisePreset, err := LoadPreset("enterprise-team")
	require.NoError(t, err)
	assert.Equal(t, "enterprise", enterprisePreset.TeamSize)
	assert.Equal(t, "complex", enterprisePreset.Complexity)
	assert.True(t, enterprisePreset.Profile.Config.Security.Webhook.Auth.Enabled, "Enterprise should have security enabled")
	assert.True(t, enterprisePreset.Profile.Config.Security.Audit.Enabled, "Enterprise should have audit enabled")
}

func TestSpecializedPresets(t *testing.T) {
	// Test DevOps preset
	devopsPreset, err := LoadPreset("devops-team")
	require.NoError(t, err)
	
	// Should have CI/CD specific flows
	assert.Contains(t, devopsPreset.Flows, "cicd", "DevOps preset should have CI/CD flow")
	assert.Contains(t, devopsPreset.Flows, "infrastructure", "DevOps preset should have infrastructure flow")
	
	// Should have DevOps specific alerts
	hasDevOpsAlert := false
	for _, alert := range devopsPreset.Alerts {
		if alert.Title == "CI/CD Pipeline Alert" || alert.Title == "Kubernetes Cluster Alert" {
			hasDevOpsAlert = true
			break
		}
	}
	assert.True(t, hasDevOpsAlert, "DevOps preset should have DevOps specific alerts")
	
	// Test SRE preset
	srePreset, err := LoadPreset("sre-team")
	require.NoError(t, err)
	
	// Should have SRE specific configuration
	assert.True(t, srePreset.Profile.Config.CorrelationBestEffort, "SRE preset should enable correlation")
	assert.Greater(t, srePreset.Profile.Config.CorrelationMinSamples, 50, "SRE preset should use higher correlation samples")
	assert.Less(t, srePreset.Profile.Config.LatencyThresholdSeconds, 1.0, "SRE preset should have stricter latency threshold")
	
	// Should have SRE specific flows
	assert.Contains(t, srePreset.Flows, "user_experience", "SRE preset should have user experience flow")
	assert.Contains(t, srePreset.Flows, "service_dependencies", "SRE preset should have service dependencies flow")
	
	// Should have SRE specific alerts
	hasSLOAlert := false
	for _, alert := range srePreset.Alerts {
		if alert.Title == "SLO Burn Rate Alert" {
			hasSLOAlert = true
			break
		}
	}
	assert.True(t, hasSLOAlert, "SRE preset should have SLO burn rate alert")
}

func TestPresetQuickStartGuides(t *testing.T) {
	presets := GetAvailablePresets()
	
	for _, preset := range presets {
		t.Run(preset.Name+"-quickstart", func(t *testing.T) {
			quickStart := preset.QuickStart
			
			// Should have prerequisites
			assert.NotEmpty(t, quickStart.Prerequisites, "Should have prerequisites")
			
			// Should have setup steps
			assert.NotEmpty(t, quickStart.SetupSteps, "Should have setup steps")
			
			// Setup steps should be sequential
			for i, step := range quickStart.SetupSteps {
				assert.Equal(t, i+1, step.Step, "Setup steps should be sequential starting from 1")
				assert.NotEmpty(t, step.Title, "Setup step should have title")
				assert.NotEmpty(t, step.Description, "Setup step should have description")
			}
			
			// Should have common workflows
			assert.NotEmpty(t, quickStart.CommonWorkflows, "Should have common workflows")
			
			for _, workflow := range quickStart.CommonWorkflows {
				assert.NotEmpty(t, workflow.Name, "Workflow should have name")
				assert.NotEmpty(t, workflow.Description, "Workflow should have description")
				assert.NotEmpty(t, workflow.Steps, "Workflow should have steps")
			}
			
			// Should have troubleshooting tips
			assert.NotEmpty(t, quickStart.Troubleshooting, "Should have troubleshooting tips")
			
			for _, tip := range quickStart.Troubleshooting {
				assert.NotEmpty(t, tip.Problem, "Troubleshooting tip should have problem")
				assert.NotEmpty(t, tip.Symptoms, "Troubleshooting tip should have symptoms")
				assert.NotEmpty(t, tip.Solutions, "Troubleshooting tip should have solutions")
			}
			
			// Should have best practices
			assert.NotEmpty(t, quickStart.BestPractices, "Should have best practices")
		})
	}
}

func TestPresetConfigurationValues(t *testing.T) {
	presets := GetAvailablePresets()
	
	for _, preset := range presets {
		t.Run(preset.Name+"-config", func(t *testing.T) {
			config := preset.Profile.Config
			
			// Check required configuration
			assert.NotEmpty(t, config.PrometheusServiceLabel, "Should have Prometheus service label")
			assert.NotEmpty(t, config.ServiceLabel, "Should have service label")
			assert.NotEmpty(t, config.RouteLabel, "Should have route label")
			assert.NotEmpty(t, config.LatencyMetric, "Should have latency metric")
			assert.NotEmpty(t, config.RequestCountMetric, "Should have request count metric")
			
			// Check reasonable defaults
			assert.Greater(t, config.LatencyThresholdSeconds, 0.0, "Latency threshold should be positive")
			assert.Greater(t, config.TopEndpoints, 0, "Top endpoints should be positive")
			assert.Greater(t, config.Window, "", "Window should not be empty")
			
			// Check cardinality limits increase with complexity
			if preset.Complexity == "complex" {
				assert.GreaterOrEqual(t, config.RouteCardinalityLimit, 100, "Complex preset should have higher route cardinality limit")
				assert.GreaterOrEqual(t, config.ServiceCardinalityLimit, 100, "Complex preset should have higher service cardinality limit")
			}
		})
	}
}

