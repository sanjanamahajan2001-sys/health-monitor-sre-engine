package rules

import "health-monitor/pkg/model"

func MemoryStatus(availMB int) model.Status {
	if availMB > 500 {
		return model.SAFE
	}
	return model.RISK
}
