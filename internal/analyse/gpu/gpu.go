package gpu

import (
	"fmt"
	"strconv"
	"strings"

	"health-monitor/internal/runner"
)

type Result struct {
	UsedMB  int
	TotalMB int
	UtilPct int
	TempC   int
	Present bool
}

func Collect() (Result, error) {
	if !runner.HasCommand("nvidia-smi") {
		return Result{Present: false}, nil
	}
	out, err := runner.Exec(
		"nvidia-smi --query-gpu=memory.used,memory.total,utilization.gpu,temperature.gpu --format=csv,noheader",
	)
	if err != nil {
		return Result{Present: false}, err
	}
	if strings.TrimSpace(out) == "" {
		return Result{Present: false}, fmt.Errorf("unexpected empty nvidia-smi output")
	}

	parts := strings.Split(out, ",")
	if len(parts) < 4 {
		return Result{Present: false}, fmt.Errorf("unexpected nvidia-smi output: %q", out)
	}
	used, err := strconv.Atoi(strings.TrimSpace(strings.ReplaceAll(parts[0], "MiB", "")))
	if err != nil {
		return Result{Present: false}, fmt.Errorf("invalid GPU used memory %q: %w", parts[0], err)
	}
	total, err := strconv.Atoi(strings.TrimSpace(strings.ReplaceAll(parts[1], "MiB", "")))
	if err != nil {
		return Result{Present: false}, fmt.Errorf("invalid GPU total memory %q: %w", parts[1], err)
	}
	util, err := strconv.Atoi(strings.TrimSpace(strings.ReplaceAll(parts[2], "%", "")))
	if err != nil {
		return Result{Present: false}, fmt.Errorf("invalid GPU utilization %q: %w", parts[2], err)
	}
	temp, err := strconv.Atoi(strings.TrimSpace(parts[3]))
	if err != nil {
		return Result{Present: false}, fmt.Errorf("invalid GPU temperature %q: %w", parts[3], err)
	}

	return Result{
		UsedMB:  used,
		TotalMB: total,
		UtilPct: util,
		TempC:   temp,
		Present: true,
	}, nil
}
