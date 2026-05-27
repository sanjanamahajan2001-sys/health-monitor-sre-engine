package runner

import (
	"os/exec"
	"strings"
)

// Exec runs a shell command and returns trimmed output
func Exec(cmd string) (string, error) {
	out, err := exec.Command("bash", "-c", cmd).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// HasCommand returns true if the command exists in PATH.
func HasCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// MissingCommands returns a list of commands not found in PATH.
func MissingCommands(names ...string) []string {
	var missing []string
	for _, name := range names {
		if strings.TrimSpace(name) == "" {
			continue
		}
		if !HasCommand(name) {
			missing = append(missing, name)
		}
	}
	return missing
}

// Lines splits output into non-empty lines
func Lines(out string) []string {
	raw := strings.Split(out, "\n")
	var lines []string
	for _, l := range raw {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, strings.TrimSpace(l))
		}
	}
	return lines
}

// Fields splits a single line into space-separated fields
func Fields(line string) []string {
	return strings.Fields(line)
}
