package rules

import "health-monitor/pkg/model"

func SSHStatus(hasAcceptedPwd bool) model.Status {
	if hasAcceptedPwd {
		return model.RISK
	}
	return model.SAFE
}
