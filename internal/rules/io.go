package rules

import "health-monitor/pkg/model"

func IOStatus(wait float64) model.Status {
	if wait < 5 {
		return model.SAFE
	}
	if wait < 10 {
		return model.CHECK
	}
	return model.RISK
}
