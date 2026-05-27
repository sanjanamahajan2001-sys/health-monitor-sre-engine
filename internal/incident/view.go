package incident

import (
	"fmt"
	"strings"
	"time"

	"health-monitor/internal/config"
	"health-monitor/internal/flow"
)

func FormatIncidentView(incident Incident) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("ID: %s\n", incident.ID))
	sb.WriteString(fmt.Sprintf("Service: %s\n", incident.Service))
	sb.WriteString(fmt.Sprintf("Profile: %s\n", incident.Profile))

	// Display Kubernetes metadata if available
	if incident.Cluster != "" {
		sb.WriteString(fmt.Sprintf("Cluster: %s\n", incident.Cluster))
	}
	if incident.Namespace != "" {
		sb.WriteString(fmt.Sprintf("Namespace: %s\n", incident.Namespace))
	}
	if incident.Deployment != "" {
		sb.WriteString(fmt.Sprintf("Deployment: %s\n", incident.Deployment))
	}
	if incident.K8sContext != "" {
		sb.WriteString(fmt.Sprintf("Context: %s\n", incident.K8sContext))
	}

	sb.WriteString(formatImpactedFlowsLine(incident.Service))
	sb.WriteString(fmt.Sprintf("Severity: %s\n", incident.Severity))
	sb.WriteString(fmt.Sprintf("Title: %s\n", incident.Title))
	sb.WriteString(fmt.Sprintf("State: %s\n", incident.State))
	sb.WriteString(fmt.Sprintf("Created: %s\n", incident.CreatedAt.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("Updated: %s\n", incident.UpdatedAt.Format(time.RFC3339)))
	if strings.TrimSpace(incident.Summary) != "" {
		sb.WriteString(fmt.Sprintf("Summary: %s\n", strings.TrimSpace(incident.Summary)))
	}
	sb.WriteString("\nTimeline\n")
	sb.WriteString(strings.Repeat("-", 8))
	sb.WriteString("\n")
	for _, event := range incident.Events {
		sb.WriteString(formatTimelineLine(event))
	}
	return sb.String()
}

func formatImpactedFlowsLine(serviceName string) string {
	result, err := flow.LoadOnce()
	if err != nil {
		switch {
		case flow.IsNoConfig(err):
			return "Impacted Flows: unavailable (no flow config)\n"
		case flow.IsInvalidConfig(err):
			return "Impacted Flows: unavailable (invalid flow config)\n"
		default:
			return "Impacted Flows: unavailable (failed to load flows)\n"
		}
	}
	if len(result.Flows) == 0 {
		return "Impacted Flows: none\n"
	}
	catalog := flow.NewCatalog(result.Flows)
	impacted := catalog.ImpactedFlows(serviceName)
	if len(impacted) == 0 {
		return "Impacted Flows: none\n"
	}
	var sb strings.Builder
	sb.WriteString("Impacted Flows:\n")
	for _, flow := range impacted {
		sb.WriteString("• " + flow.DisplayName() + "\n")
	}
	return sb.String()
}

func FormatIncidentList(incidents []Incident) string {
	if len(incidents) == 0 {
		return "No incidents found.\n"
	}
	var sb strings.Builder
	sb.WriteString("ID  SEV  STATE  TITLE  PROFILE\n")
	sb.WriteString(strings.Repeat("-", 50))
	sb.WriteString("\n")
	for _, incident := range incidents {
		sb.WriteString(fmt.Sprintf("%s  %s  %s  %s  %s\n",
			incident.ID,
			incident.Severity,
			incident.State,
			incident.Title,
			config.GetActiveProfileName(),
		))
	}
	return sb.String()
}
