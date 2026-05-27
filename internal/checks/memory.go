package checks

import (
	"fmt"

	"health-monitor/internal/analyse/memory"
	"health-monitor/internal/rules"
	"health-monitor/pkg/model"
)

func Memory(report *model.Report) {
	data, err := memory.Collect()
	if err != nil {
		report.Summary = append(report.Summary, model.SummaryItem{
			Name:   "MEMORY",
			Status: model.UNKNOWN,
			Value:  "N/A",
		})
		return
	}

	status := rules.MemoryStatus(data.AvailMB)

	report.Summary = append(report.Summary, model.SummaryItem{
		Name:   "MEMORY",
		Status: status,
		Value:  fmt.Sprintf("%dMi/%dMi", data.AvailMB, data.TotalMB),
	})

	report.Metrics = append(report.Metrics, model.Metric{
		Name:  "Memory Usage",
		Value: fmt.Sprintf("Available: %d MB / %d MB", data.AvailMB, data.TotalMB),
	})
}
