package checks

import (
	"fmt"

	"health-monitor/internal/analyse/zombies"
	"health-monitor/internal/rules"
	"health-monitor/pkg/model"
)

func Zombies(report *model.Report) {
	count, err := zombies.Count()
	if err != nil {
		report.Metrics = append(report.Metrics,
			model.Metric{"Zombie Processes", "Unavailable"},
		)
		return
	}

	status := rules.ZombieStatus(count)

	report.Metrics = append(report.Metrics,
		model.Metric{
			"Zombie Processes",
			fmt.Sprintf("%d (%s)", count, status),
		},
	)
}
