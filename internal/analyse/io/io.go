package io

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func CollectIOWait() (float64, error) {
	iowait1, total1, err := readIOWaitSnapshot()
	if err != nil {
		return 0, err
	}
	time.Sleep(200 * time.Millisecond)
	iowait2, total2, err := readIOWaitSnapshot()
	if err != nil {
		return 0, err
	}
	deltaTotal := total2 - total1
	deltaWait := iowait2 - iowait1
	if deltaTotal <= 0 {
		return 0, fmt.Errorf("invalid iowait delta")
	}
	waitPct := (deltaWait / deltaTotal) * 100
	if waitPct < 0 {
		waitPct = 0
	}
	if waitPct > 100 {
		waitPct = 100
	}
	return waitPct, nil
}

func readIOWaitSnapshot() (float64, float64, error) {
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
		if len(fields) < 6 {
			return 0, 0, fmt.Errorf("invalid /proc/stat cpu line")
		}
		var total float64
		for _, v := range fields[1:] {
			val, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return 0, 0, err
			}
			total += val
		}
		iowait, err := strconv.ParseFloat(fields[5], 64)
		if err != nil {
			return 0, 0, err
		}
		return iowait, total, nil
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, err
	}
	return 0, 0, fmt.Errorf("cpu stats not found")
}
