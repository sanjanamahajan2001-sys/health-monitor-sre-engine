package checks

import (
	"fmt"

	"health-monitor/internal/analyse/disk"
	"health-monitor/internal/rules"
	"health-monitor/pkg/model"
)

func Disk(report *model.Report) {
	data, err := disk.Collect()
	if err != nil {
		report.Summary = append(report.Summary, model.SummaryItem{
			Name:   "DISK",
			Status: model.UNKNOWN,
			Value:  "N/A",
		})
		return
	}

	status := rules.DiskStatus(data.UsedPc)

	report.Summary = append(report.Summary, model.SummaryItem{
		Name:   "DISK",
		Status: status,
		Value:  fmt.Sprintf("%s/%s", data.Used, data.Total),
	})

	report.Metrics = append(report.Metrics,
		model.Metric{"Disk Usage", fmt.Sprintf("%s / %s (%d%%)", data.Used, data.Total, data.UsedPc)},
	)
}
