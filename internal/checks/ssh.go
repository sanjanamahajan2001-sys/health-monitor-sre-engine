package checks

import (
	"health-monitor/internal/analyse/ssh"
	"health-monitor/internal/rules"
	"health-monitor/pkg/model"
)

func SSH(report *model.Report) {
	hasAccepted, err := ssh.HasAcceptedRootLogin()
	if err != nil {
		report.Summary = append(report.Summary, model.SummaryItem{
			Name:   "SSH",
			Status: model.UNKNOWN,
			Value:  "N/A",
		})
		return
	}

	status := rules.SSHStatus(hasAccepted)

	report.Summary = append(report.Summary, model.SummaryItem{
		Name:   "SSH",
		Status: status,
		Value:  "OK",
	})
}
