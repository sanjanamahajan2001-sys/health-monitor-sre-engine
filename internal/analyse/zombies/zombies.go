package zombies

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func Count() (int, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0, err
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		state, err := readProcState(entry.Name())
		if err != nil {
			continue
		}
		if state == "Z" {
			count++
		}
	}
	return count, nil
}

func readProcState(pid string) (string, error) {
	path := filepath.Join("/proc", pid, "stat")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(data))
	if line == "" {
		return "", fmt.Errorf("empty stat for pid %s", pid)
	}
	end := strings.LastIndex(line, ")")
	if end == -1 || end+2 >= len(line) {
		return "", fmt.Errorf("invalid stat format for pid %s", pid)
	}
	rest := strings.Fields(line[end+1:])
	if len(rest) < 2 {
		return "", fmt.Errorf("invalid stat fields for pid %s", pid)
	}
	return rest[1], nil
}
