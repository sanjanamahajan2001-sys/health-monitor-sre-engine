package checks

import (
	"fmt"

	"health-monitor/internal/analyse/gpu"
	"health-monitor/internal/rules"
	"health-monitor/pkg/model"
)

func GPU(report *model.Report) {
	data, err := gpu.Collect()
	if err != nil {
		report.Summary = append(report.Summary, model.SummaryItem{
			Name:   "GPU",
			Status: model.UNKNOWN,
			Value:  "N/A",
		})

		report.Metrics = append(report.Metrics,
			model.Metric{Name: "GPU Note", Value: err.Error()},
		)
		return
	}

	status, note := rules.GPUStatus(data)

	value := "N/A"
	temp := "N/A"

	if data.Present {
		value = fmt.Sprintf("%dMi/%dMi", data.UsedMB, data.TotalMB)
		temp = fmt.Sprintf("%d°C", data.TempC)
	}

	report.Summary = append(report.Summary, model.SummaryItem{
		Name:   "GPU",
		Status: status,
		Value:  value,
	})

	report.Metrics = append(report.Metrics,
		model.Metric{Name: "GPU Temp", Value: temp},
		model.Metric{Name: "GPU Note", Value: note},
	)
}
