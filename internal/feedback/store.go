package feedback

import (
	"crypto/sha256"
	"fmt"
	"os"
	"health-monitor/internal/config"
)

// GetMachineID returns a stable, anonymized identifier for the system
func GetMachineID() string {
	// For simplicity, we use the hostname + user
	hostname, _ := os.Hostname()
	currentUser, _ := config.GetCurrentUserName()
	rawID := fmt.Sprintf("%s-%s", hostname, currentUser)
	
	// Anonymize using SHA-256
	hash := sha256.Sum256([]byte(rawID))
	return fmt.Sprintf("%x", hash)[:16] // Return first 16 chars for a clean ID
}
