package rules

import "health-monitor/pkg/model"

func DiskStatus(pct int) model.Status {
	if pct < 85 {
		return model.SAFE
	}
	return model.RISK
}
