package alert

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"health-monitor/internal/config"
)

func defaultPIDFile() string {
	pm := config.GetProfileManager()
	activeProfile := pm.GetActiveProfile()
	
	if activeProfile == "default" {
		// Maintain backward compatibility for default profile
		return filepath.Join(os.TempDir(), "health-monitor-alert.pid")
	}
	
	// Use profile-specific PID file
	return filepath.Join(os.TempDir(), fmt.Sprintf("health-monitor-alert-%s.pid", activeProfile))
}

func WritePIDFile(pidFile string) error {
	if pidFile == "" {
		pidFile = defaultPIDFile()
	}
	
	if err := os.MkdirAll(filepath.Dir(pidFile), 0755); err != nil {
		return fmt.Errorf("failed to create PID directory: %w", err)
	}
	
	return os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", os.Getpid())), 0600)
}

func WritePIDFileWithPID(pidFile string, pid int) error {
	if pidFile == "" {
		pidFile = defaultPIDFile()
	}
	
	if err := os.MkdirAll(filepath.Dir(pidFile), 0755); err != nil {
		return fmt.Errorf("failed to create PID directory: %w", err)
	}
	
	return os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", pid)), 0600)
}

func ReadPIDFile(pidFile string) (int, error) {
	if strings.TrimSpace(pidFile) == "" {
		pidFile = defaultPIDFile()
	}
	
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return 0, err
	}
	
	var pid int
	if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil {
		return 0, fmt.Errorf("invalid PID file content: %w", err)
	}
	
	return pid, nil
}

func IsProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	
	// Send signal 0 to check if process exists
	err := syscall.Kill(pid, 0)
	return err == nil
}
