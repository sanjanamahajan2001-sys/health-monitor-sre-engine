package checks

import (
	"health-monitor/internal/analyse/cleanup"
	"health-monitor/internal/rules"
	"health-monitor/pkg/model"
)

func Cleanup(report *model.Report) {
	data, err := cleanup.Collect()
	if err != nil {
		report.Cleanup = append(report.Cleanup,
			"No immediate cleanup actions required",
		)
		return
	}

	msgs := rules.CleanupMessages(data)

	if len(msgs) == 0 {
		report.Cleanup = append(report.Cleanup,
			"No immediate cleanup actions required",
		)
		return
	}

	for _, m := range msgs {
		report.Cleanup = append(report.Cleanup, m)
	}
}
