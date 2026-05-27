package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GetProfileFromFlags returns the profile name from parsed flags
func GetProfileFromFlags() (string, string) {
	// These will be set by the main package
	profileName := strings.TrimSpace(os.Getenv("HEALTH_MONITOR_PROFILE"))
	profilePath := strings.TrimSpace(os.Getenv("HEALTH_MONITOR_PROFILE_PATH"))
	
	return profileName, profilePath
}

// InitializeFromCLI initializes the profile context from command line arguments
func InitializeFromCLI() error {
	pm := GetProfileManager()

	// Get profile info from environment (set by main package)
	profileName, profilePath := GetProfileFromFlags()

	// Determine active profile
	var usePersisted bool
	switch {
	case profilePath != "":
		// Load profile from explicit path
		if err := pm.loadProfileFromPath(profilePath); err != nil {
			return fmt.Errorf("failed to load profile from path %q: %w", profilePath, err)
		}

		profileName = filepath.Base(profilePath)
		profileName = strings.TrimSuffix(profileName, filepath.Ext(profileName))
		// Don't persist explicit path profiles

	case profileName != "":
		// Use the specified profile name (from --profile flag)
		// Don't persist flag-based profile changes
		break

	case os.Getenv("HEALTH_MONITOR_PROFILE") != "":
		profileName = os.Getenv("HEALTH_MONITOR_PROFILE")
		// Don't persist environment-based profile changes

	default:
		// Use the persisted active profile (if any)
		persistedProfile := pm.GetActiveProfile()
		if persistedProfile != "" && persistedProfile != "default" {
			profileName = persistedProfile
			usePersisted = true
		} else {
			profileName = "default"
			usePersisted = true
		}
	}

	// Only set active profile without persisting if using flags/env
	if usePersisted {
		err := pm.SetActiveProfile(profileName)
		if err != nil {
			// If persisted profile is missing/invalid, fallback to default silently
			// This prevents hard-crashing when a profile is manually deleted
			if profileName != "default" {
				_ = pm.SetActiveProfile("default")
			} else {
				return err
			}
		}
	} else {
		// Set active profile without persisting
		pm.mutex.Lock()
		pm.activeProfile = profileName
		pm.mutex.Unlock()
		
		// Load the profile data.
		if err := pm.loadProfile(profileName); err != nil {
			if os.IsPermission(err) || strings.Contains(err.Error(), "permission denied") {
				fmt.Fprintf(os.Stderr, "⚠️  Warning: Permission denied loading profile %q. Falling back to default config.\n", profileName)
				fmt.Fprintf(os.Stderr, "   Hint: This profile may have been created with 'sudo'. Try running with 'sudo'.\n\n")
				pm.profiles[profileName] = &Profile{
					Name:   profileName,
					Config: Default(),
				}
			} else if os.IsNotExist(err) || strings.Contains(err.Error(), "no such file") {
				pm.profiles[profileName] = &Profile{
					Name:   profileName,
					Config: Default(),
				}
			} else {
				return fmt.Errorf("failed to load profile %q: %w", profileName, err)
			}
		}
	}
	
	return nil
}

// getConfigBasePath returns the base path for configuration files
func getConfigBasePath() string {
	if testConfigBasePath != "" {
		return testConfigBasePath
	}

	// 1️⃣ Highest priority → explicit override (container / k8s / CI safe)
	if base := os.Getenv("HEALTH_MONITOR_CONFIG_DIR"); base != "" {
		return base
	}

	// 2️⃣ If running as root, always use system config
	if os.Geteuid() == 0 {
		return "/etc/health-monitor"
	}

	// 3️⃣ Normal user execution
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		return filepath.Join(home, ".health-monitor")
	}

	// 4️⃣ Final fallback (rare edge case)
	return "/tmp/health-monitor"
}
