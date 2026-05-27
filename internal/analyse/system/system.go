package system

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type SystemData struct {
	CPUUsage   float64
	UptimeDays float64
	LoadAvg1   float64
	LoadAvg5   float64
	LoadAvg15  float64
}

func Collect() (SystemData, error) {
	data := SystemData{}

	cpu, err := readCPUUsage()
	if err == nil {
		data.CPUUsage = cpu
	}

	uptimeDays, err := readUptimeDays()
	if err == nil {
		data.UptimeDays = uptimeDays
	}

	load1, load5, load15, err := readLoadAvg()
	if err == nil {
		data.LoadAvg1 = load1
		data.LoadAvg5 = load5
		data.LoadAvg15 = load15
	}

	return data, nil
}

func readUptimeDays() (float64, error) {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		return 0, fmt.Errorf("invalid /proc/uptime format")
	}
	seconds, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, err
	}
	return seconds / 86400, nil
}

func readLoadAvg() (float64, float64, float64, error) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, 0, 0, err
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return 0, 0, 0, fmt.Errorf("invalid /proc/loadavg format")
	}
	load1, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, 0, 0, err
	}
	load5, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return 0, 0, 0, err
	}
	load15, err := strconv.ParseFloat(fields[2], 64)
	if err != nil {
		return 0, 0, 0, err
	}
	return load1, load5, load15, nil
}

func readCPUUsage() (float64, error) {
	idle1, total1, err := readCPUSnapshot()
	if err != nil {
		return 0, err
	}
	time.Sleep(200 * time.Millisecond)
	idle2, total2, err := readCPUSnapshot()
	if err != nil {
		return 0, err
	}
	deltaTotal := total2 - total1
	deltaIdle := idle2 - idle1
	if deltaTotal <= 0 {
		return 0, fmt.Errorf("invalid cpu delta")
	}
	usage := (1.0 - (deltaIdle / deltaTotal)) * 100
	if usage < 0 {
		usage = 0
	}
	if usage > 100 {
		usage = 100
	}
	return usage, nil
}

func readCPUSnapshot() (float64, float64, error) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			return 0, 0, fmt.Errorf("invalid cpu stat line: %q", line)
		}
		var values []float64
		for _, v := range fields[1:] {
			val, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return 0, 0, err
			}
			values = append(values, val)
		}
		idle := values[3]
		if len(values) > 4 {
			idle += values[4]
		}
		var total float64
		for _, v := range values {
			total += v
		}
		return idle, total, nil
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, err
	}
	return 0, 0, fmt.Errorf("cpu stat not found")
}

// FormatUptime formats uptime in a human-readable way
func FormatUptime(days float64) string {
	if days < 1 {
		hours := days * 24
		if hours < 1 {
			minutes := hours * 60
			return fmt.Sprintf("%.0f minutes", minutes)
		}
		return fmt.Sprintf("%.1f hours", hours)
	}
	if days < 7 {
		return fmt.Sprintf("%.1f days", days)
	}
	if days < 30 {
		weeks := days / 7
		return fmt.Sprintf("%.1f weeks", weeks)
	}
	if days < 365 {
		months := days / 30
		return fmt.Sprintf("%.1f months", months)
	}
	years := days / 365
	return fmt.Sprintf("%.1f years", years)
}
