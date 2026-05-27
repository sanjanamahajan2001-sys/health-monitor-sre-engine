package memory

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Result struct {
	TotalMB int
	AvailMB int
}

func Collect() (Result, error) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return Result{}, err
	}
	defer file.Close()

	var totalKB int
	var availKB int
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			totalKB, err = parseMeminfoKB(line)
			if err != nil {
				return Result{}, err
			}
		}
		if strings.HasPrefix(line, "MemAvailable:") {
			availKB, err = parseMeminfoKB(line)
			if err != nil {
				return Result{}, err
			}
		}
		if totalKB > 0 && availKB > 0 {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return Result{}, err
	}
	if totalKB == 0 || availKB == 0 {
		return Result{}, fmt.Errorf("missing MemTotal or MemAvailable in /proc/meminfo")
	}

	return Result{
		TotalMB: totalKB / 1024,
		AvailMB: availKB / 1024,
	}, nil
}

func parseMeminfoKB(line string) (int, error) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0, fmt.Errorf("unexpected meminfo line: %q", line)
	}
	value, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, fmt.Errorf("invalid meminfo value %q: %w", fields[1], err)
	}
	return value, nil
}
