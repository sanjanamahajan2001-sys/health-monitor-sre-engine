package checks

import (
	"fmt"

	"health-monitor/internal/analyse/io"
	"health-monitor/internal/rules"
	"health-monitor/pkg/model"
)

func IO(report *model.Report) {
	wait, err := io.CollectIOWait()
	if err != nil {
		report.Metrics = append(report.Metrics,
			model.Metric{"Disk I/O Wait", "Unavailable"},
		)
		return
	}

	status := rules.IOStatus(wait)

	report.Metrics = append(report.Metrics,
		model.Metric{
			"Disk I/O Wait",
			fmt.Sprintf("%.2f%% (%s)", wait, status),
		},
	)
}
