package rules

import "health-monitor/pkg/model"

func ZombieStatus(count int) model.Status {
	if count == 0 {
		return model.SAFE
	}
	return model.CHECK
}
