package ssh

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func HasAcceptedRootLogin() (bool, error) {
	logPath, err := findAuthLogPath()
	if err != nil {
		return false, err
	}
	file, err := os.Open(logPath)
	if err != nil {
		return false, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	found := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "Accepted password") &&
			(strings.Contains(line, " ubuntu ") || strings.Contains(line, " root ") || strings.HasSuffix(line, " root") || strings.HasSuffix(line, " ubuntu")) {
			found = true
		}
	}
	if err := scanner.Err(); err != nil {
		return false, err
	}
	return found, nil
}

func findAuthLogPath() (string, error) {
	candidates := []string{
		"/var/log/auth.log",
		"/var/log/secure",
		"/var/log/messages",
	}
	for _, path := range candidates {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, nil
		}
	}
	return "", fmt.Errorf("no SSH auth log found in default locations")
}
