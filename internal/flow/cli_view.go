package flow

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"
)

var (
	cliTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	cliLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("7")).Bold(true)
	cliDimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	cliBoxStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2)
)

func FormatFlowListCLI(flows []Flow, degraded map[string]bool, counts map[string]int) string {
	if !isTerminal() {
		return FormatFlowListPlain(flows, degraded, counts)
	}
	if len(flows) == 0 {
		return cliBoxStyle.Render("No flows configured.") + "\n"
	}
	idWidth, nameWidth := flowColumnWidths(flows)
	var sb strings.Builder
	sb.WriteString(cliTitleStyle.Render("Flows"))
	sb.WriteString("\n\n")
	sb.WriteString(fmt.Sprintf("%-*s  %-*s  %s\n", idWidth, "ID", nameWidth, "NAME", "STATUS"))
	sb.WriteString(cliDimStyle.Render(strings.Repeat("─", idWidth+2+nameWidth+2+len("STATUS")+12)))
	sb.WriteString("\n")
	for _, flow := range flows {
		status := "🟢 healthy"
		if degraded[flow.ID] {
			status = "🔴 degraded"
		}
		count := counts[flow.ID]
		suffix := fmt.Sprintf("(%d incident", count)
		if count != 1 {
			suffix += "s"
		}
		suffix += ")"
		sb.WriteString(fmt.Sprintf("%-*s  %-*s  %s %s\n", idWidth, flow.ID, nameWidth, flow.DisplayName(), status, suffix))
	}
	return cliBoxStyle.Render(sb.String()) + "\n"
}

func FormatFlowViewCLI(flow Flow, incidents []IncidentSummary, degraded bool) string {
	if !isTerminal() {
		return FormatFlowViewPlain(flow, incidents, degraded)
	}
	status := "🟢 healthy"
	if degraded {
		status = "🔴 degraded"
	}
	var sb strings.Builder
	sb.WriteString(cliTitleStyle.Render("Flow Details"))
	sb.WriteString("\n\n")
	sb.WriteString(lineKV("ID", flow.ID))
	sb.WriteString(lineKV("Name", flow.DisplayName()))
	sb.WriteString(lineKV("Status", status))
	sb.WriteString(cliLabelStyle.Render("Services:"))
	sb.WriteString("\n")
	for _, service := range flow.Services {
		sb.WriteString("• " + service + "\n")
	}
	sb.WriteString("\n")
	sb.WriteString(cliLabelStyle.Render("Active Incidents"))
	sb.WriteString("\n")
	if len(incidents) == 0 {
		sb.WriteString("None\n")
		return cliBoxStyle.Render(sb.String()) + "\n"
	}
	for _, incident := range incidents {
		sb.WriteString(fmt.Sprintf("• %s  %s  %s (%s)\n",
			incident.ID,
			incident.Severity,
			incident.Title,
			incident.Service,
		))
	}
	return cliBoxStyle.Render(sb.String()) + "\n"
}

func FormatFlowViewJSON(flow Flow, incidents []IncidentSummary, degraded bool) (string, error) {
	if incidents == nil {
		incidents = []IncidentSummary{}
	}
	payload := struct {
		ID        string            `json:"id"`
		Name      string            `json:"name"`
		Services  []string          `json:"services"`
		Status    string            `json:"status"`
		Incidents []IncidentSummary `json:"incidents"`
	}{
		ID:        flow.ID,
		Name:      flow.DisplayName(),
		Services:  flow.Services,
		Status:    flowStatusLabel(degraded),
		Incidents: incidents,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func FormatFlowListJSON(flows []Flow, degraded map[string]bool, counts map[string]int) (string, error) {
	if flows == nil {
		flows = []Flow{}
	}
	payload := struct {
		Flows []struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			Status    string `json:"status"`
			Incidents int    `json:"incident_count"`
		} `json:"flows"`
	}{
		Flows: make([]struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			Status    string `json:"status"`
			Incidents int    `json:"incident_count"`
		}, 0, len(flows)),
	}
	for _, flow := range flows {
		payload.Flows = append(payload.Flows, struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			Status    string `json:"status"`
			Incidents int    `json:"incident_count"`
		}{
			ID:        flow.ID,
			Name:      flow.DisplayName(),
			Status:    flowStatusLabel(degraded[flow.ID]),
			Incidents: counts[flow.ID],
		})
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func FormatFlowListPlain(flows []Flow, degraded map[string]bool, counts map[string]int) string {
	if len(flows) == 0 {
		return "No flows configured.\n"
	}
	idWidth, nameWidth := flowColumnWidths(flows)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%-*s  %-*s  %s\n", idWidth, "ID", nameWidth, "NAME", "STATUS"))
	sb.WriteString(strings.Repeat("-", idWidth+2+nameWidth+2+len("STATUS")))
	sb.WriteString("\n")
	for _, flow := range flows {
		status := "🟢 healthy"
		if degraded[flow.ID] {
			status = "🔴 degraded"
		}
		count := counts[flow.ID]
		suffix := fmt.Sprintf("(%d incident", count)
		if count != 1 {
			suffix += "s"
		}
		suffix += ")"
		sb.WriteString(fmt.Sprintf("%-*s  %-*s  %s %s\n", idWidth, flow.ID, nameWidth, flow.DisplayName(), status, suffix))
	}
	return sb.String()
}

func FormatFlowViewPlain(flow Flow, incidents []IncidentSummary, degraded bool) string {
	var sb strings.Builder
	status := "🟢 healthy"
	if degraded {
		status = "🔴 degraded"
	}
	sb.WriteString(fmt.Sprintf("ID: %s\n", flow.ID))
	sb.WriteString(fmt.Sprintf("Name: %s\n", flow.DisplayName()))
	sb.WriteString(fmt.Sprintf("Status: %s\n", status))
	sb.WriteString("Services:\n")
	for _, service := range flow.Services {
		sb.WriteString("- " + service + "\n")
	}
	sb.WriteString("\nActive Incidents\n")
	if len(incidents) == 0 {
		sb.WriteString("None\n")
		return sb.String()
	}
	for _, incident := range incidents {
		sb.WriteString(fmt.Sprintf("- %s  %s  %s (%s)\n",
			incident.ID,
			incident.Severity,
			incident.Title,
			incident.Service,
		))
	}
	return sb.String()
}

func lineKV(label string, value string) string {
	value = strings.TrimSpace(value)
	if strings.Contains(value, "\n") {
		value = strings.ReplaceAll(value, "\n", "\n  ")
	}
	return fmt.Sprintf("%s %s\n", cliLabelStyle.Render(label+":"), value)
}

func flowStatusLabel(degraded bool) string {
	if degraded {
		return "degraded"
	}
	return "healthy"
}

func flowColumnWidths(flows []Flow) (int, int) {
	idWidth := len("ID")
	nameWidth := len("NAME")
	for _, flow := range flows {
		if len(flow.ID) > idWidth {
			idWidth = len(flow.ID)
		}
		name := flow.DisplayName()
		if len(name) > nameWidth {
			nameWidth = len(name)
		}
	}
	return idWidth, nameWidth
}

func isTerminal() bool {
	fd := os.Stdout.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}
