package checks

import (
	varfs "health-monitor/internal/analyse/var"
	"health-monitor/pkg/model"
)

func Var(report *model.Report) {
	items, err := varfs.CollectTop()
	if err != nil {
		return
	}

	report.Vars = items
}
