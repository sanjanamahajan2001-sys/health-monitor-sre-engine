package config

import (
	"fmt"
	"os"
	"sync"
)

var (
	// Key: profile + warning -> value: shown
	warningsShown = make(map[string]bool)
	warningsMutex sync.RWMutex
)

// ValidateConfigOnce checks configuration and prints warnings only once per session per profile
func ValidateConfigOnce(cfg Config) {
	pm := GetProfileManager()
	activeProfile := pm.GetActiveProfile()
	
	warningsMutex.Lock()
	defer warningsMutex.Unlock()
	
	warnings := ValidateConfigUnsafe(cfg)
	for _, warning := range warnings {
		key := fmt.Sprintf("%s:%s", activeProfile, warning)
		if !warningsShown[key] {
			fmt.Fprintf(os.Stderr, "Warning: %s\n", warning)
			warningsShown[key] = true
		}
	}
}

// ResetWarningsForTest resets warnings for testing purposes
func ResetWarningsForTest() {
	warningsMutex.Lock()
	defer warningsMutex.Unlock()
	warningsShown = make(map[string]bool)
}

// ShouldShowWarnings determines if warnings should be shown based on command context
func ShouldShowWarnings(args []string) bool {
	if len(args) == 0 {
		return true // First CLI invocation
	}
	
	// Show warnings for alert listen (production critical)
	if len(args) >= 1 && args[0] == "alert" {
		if len(args) >= 2 && args[1] == "listen" {
			return true
		}
	}
	
	return false
}
