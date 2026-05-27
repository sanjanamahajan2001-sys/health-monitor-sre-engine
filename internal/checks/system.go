package checks

import (
	"fmt"

	"health-monitor/internal/analyse/system"
	"health-monitor/pkg/model"
)

func System(report *model.Report) {
	data, err := system.Collect()
	if err != nil {
		// Silently fail - system metrics are optional
		return
	}

	// Add CPU usage
	if data.CPUUsage > 0 {
		report.Metrics = append(report.Metrics,
			model.Metric{"CPU Usage", fmt.Sprintf("%.1f%%", data.CPUUsage)},
		)
	}

	// Add uptime
	if data.UptimeDays > 0 {
		uptimeStr := system.FormatUptime(data.UptimeDays)
		report.Metrics = append(report.Metrics,
			model.Metric{"System Uptime", uptimeStr},
		)
	}

	// Add load average
	if data.LoadAvg1 > 0 || data.LoadAvg5 > 0 || data.LoadAvg15 > 0 {
		loadStr := fmt.Sprintf("%.2f, %.2f, %.2f", data.LoadAvg1, data.LoadAvg5, data.LoadAvg15)
		report.Metrics = append(report.Metrics,
			model.Metric{"Load Average (1m, 5m, 15m)", loadStr},
		)
	}
}
