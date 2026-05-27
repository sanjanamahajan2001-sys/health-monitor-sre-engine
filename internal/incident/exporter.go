package incident

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"health-monitor/internal/flow"
)

func exportIncident(incident Incident, format string) (string, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	switch format {
	case "md", "markdown":
		content := renderIncidentMarkdown(incident)
		path := resolveExportPath(os.Getenv("HEALTH_MONITOR_EXPORT_PATH"), incident.ID, "md")
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return "", err
		}
		if err := os.WriteFile(path, []byte(content+"\n"), 0644); err != nil {
			return "", err
		}
		return path, nil
	case "json":
		content, err := renderIncidentJSON(incident)
		if err != nil {
			return "", err
		}
		path := resolveExportPath(os.Getenv("HEALTH_MONITOR_EXPORT_PATH"), incident.ID, "json")
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return "", err
		}
		if err := os.WriteFile(path, []byte(content+"\n"), 0644); err != nil {
			return "", err
		}
		return path, nil
	default:
		return "", fmt.Errorf("unsupported format %q (use markdown or json)", format)
	}
}

func renderIncidentJSON(incident Incident) (string, error) {
	impacted, flowStatus := resolveImpactedFlows(incident.Service)
	payload, err := json.MarshalIndent(incidentExport{
		Incident:          incident,
		Timeline:          TimelineLines(incident.Events),
		ImpactedFlows:     impacted,
		FlowMappingStatus: flowStatus,
	}, "", "  ")
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

type incidentExport struct {
	Incident          Incident `json:"incident"`
	Timeline          []string `json:"timeline"`
	ImpactedFlows     []string `json:"impacted_flows"`
	FlowMappingStatus string   `json:"flow_mapping_status,omitempty"`
}

func TimelineLines(events []Event) []string {
	lines := make([]string, 0, len(events))
	for _, event := range events {
		lines = append(lines, EventDescription(event))
	}
	return lines
}

func exportMarkdown(incident Incident) (string, error) {
	content := renderIncidentMarkdown(incident)
	path := resolveExportPath(os.Getenv("HEALTH_MONITOR_EXPORT_PATH"), incident.ID, "md")
	if err := os.WriteFile(path, []byte(content+"\n"), 0600); err != nil {
		return "", err
	}
	return path, nil
}

func renderIncidentMarkdown(incident Incident) string {
	var sb strings.Builder
	sb.WriteString("# Incident Report\n\n")
	sb.WriteString(fmt.Sprintf("**ID:** %s\n\n", incident.ID))
	sb.WriteString(fmt.Sprintf("**Service:** %s\n\n", incident.Service))
	sb.WriteString(fmt.Sprintf("**Profile:** `%s`\n\n", incident.Profile))
	
	if incident.Cluster != "" || incident.Namespace != "" || incident.Deployment != "" {
		sb.WriteString("## Kubernetes Context\n\n")
		if incident.Cluster != "" {
			sb.WriteString(fmt.Sprintf("**Cluster:** %s\n", incident.Cluster))
		}
		if incident.Namespace != "" {
			sb.WriteString(fmt.Sprintf("**Namespace:** %s\n", incident.Namespace))
		}
		if incident.Deployment != "" {
			sb.WriteString(fmt.Sprintf("**Deployment:** %s\n", incident.Deployment))
		}
		if incident.K8sContext != "" {
			sb.WriteString(fmt.Sprintf("**Context:** %s\n", incident.K8sContext))
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("**Severity:** `%s`\n\n", incident.Severity))
	sb.WriteString(fmt.Sprintf("**Title:** %s\n\n", incident.Title))
	sb.WriteString(fmt.Sprintf("**State:** %s\n\n", incident.State))
	sb.WriteString(fmt.Sprintf("**Created:** %s\n\n", incident.CreatedAt.Format("2006-01-02 15:04:05 MST")))
	sb.WriteString(fmt.Sprintf("**Updated:** %s\n\n", incident.UpdatedAt.Format("2006-01-02 15:04:05 MST")))

	if strings.TrimSpace(incident.Summary) != "" {
		sb.WriteString("## Summary\n\n")
		sb.WriteString(strings.TrimSpace(incident.Summary) + "\n\n")
	}

	sb.WriteString("## Impacted Flows\n\n")
	impacted, flowStatus := resolveImpactedFlows(incident.Service)
	if strings.TrimSpace(flowStatus) != "" {
		sb.WriteString(flowStatus + "\n\n")
	} else if len(impacted) == 0 {
		sb.WriteString("None\n\n")
	} else {
		for _, name := range impacted {
			sb.WriteString("- " + name + "\n")
		}
		sb.WriteString("\n")
	}

	// Observability Links
	if hasObservabilityLinks(incident.Links) {
		sb.WriteString("## Observability\n\n")
		if incident.Links.Grafana != "" {
			sb.WriteString(fmt.Sprintf("**Grafana:** %s\n\n", incident.Links.Grafana))
		}
		if incident.Links.Prometheus != "" {
			sb.WriteString(fmt.Sprintf("**Prometheus:** %s\n\n", incident.Links.Prometheus))
		}
		if incident.Links.Logs != "" {
			sb.WriteString(fmt.Sprintf("**Logs:** %s\n\n", incident.Links.Logs))
		}
		if incident.Links.Traces != "" {
			sb.WriteString(fmt.Sprintf("**Traces:** %s\n\n", incident.Links.Traces))
		}
	}

	// Logs Section
	if incident.Logs != nil && len(incident.Logs.TopErrors) > 0 {
		sb.WriteString("## Logs\n\n")
		sb.WriteString(fmt.Sprintf("**Backend:** %s\n\n", incident.Logs.Backend))
		sb.WriteString(fmt.Sprintf("**Window:** %s\n\n", incident.Logs.Window))
		sb.WriteString("**Top Errors:**\n\n")
		for _, err := range incident.Logs.TopErrors {
			sb.WriteString(fmt.Sprintf("- [%dx] %s\n", err.Count, err.Message))
		}
		sb.WriteString("\n")
	}

	// Traces Section
	if incident.Traces != nil && incident.Traces.TraceCount > 0 {
		sb.WriteString("## Traces\n\n")
		sb.WriteString(fmt.Sprintf("**Backend:** %s\n\n", incident.Traces.Backend))
		sb.WriteString(fmt.Sprintf("**Window:** %s\n\n", incident.Traces.Window))
		if incident.Traces.SlowestRoute != "" {
			sb.WriteString(fmt.Sprintf("**Slowest Route:** %s\n\n", incident.Traces.SlowestRoute))
		}
		if incident.Traces.P95LatencyMs > 0 {
			sb.WriteString(fmt.Sprintf("**P95 Latency:** %.0fms\n\n", incident.Traces.P95LatencyMs))
		}
		if incident.Traces.ErrorCount > 0 {
			sb.WriteString(fmt.Sprintf("**Errors:** %d\n\n", incident.Traces.ErrorCount))
		}
		if len(incident.Traces.TopSpans) > 0 {
			sb.WriteString("**Top Spans:**\n\n")
			for _, span := range incident.Traces.TopSpans {
				sb.WriteString(fmt.Sprintf("- %s (%.2fms)\n", span.Name, span.DurationMs))
			}
			sb.WriteString("\n")
		}
	}

	// Analysis section
	if incident.Analysis != nil {
		sb.WriteString("## Analysis\n\n")
		
		if incident.Analysis.Component != "" {
			sb.WriteString(fmt.Sprintf("**Component:** %s\n\n", incident.Analysis.Component))
		}
		if incident.Analysis.Dependency != "" {
			sb.WriteString(fmt.Sprintf("**Dependency:** %s\n\n", incident.Analysis.Dependency))
		}
		if incident.Analysis.Category != "" {
			sb.WriteString(fmt.Sprintf("**Category:** %s\n\n", incident.Analysis.Category))
		}
		if incident.Analysis.RootCause != "" {
			sb.WriteString(fmt.Sprintf("**Root Cause:** %s\n\n", incident.Analysis.RootCause))
		}
		if incident.Analysis.FixSummary != "" {
			sb.WriteString(fmt.Sprintf("**Fix:** %s\n\n", incident.Analysis.FixSummary))
		}
		if incident.Analysis.FailureType != "" {
			sb.WriteString(fmt.Sprintf("**Failure Type:** %s\n\n", incident.Analysis.FailureType))
		}
		if incident.Analysis.Pattern != "" {
			sb.WriteString(fmt.Sprintf("**Pattern:** %s\n\n", incident.Analysis.Pattern))
		}
		if incident.Analysis.ErrorSignature != "" {
			sb.WriteString(fmt.Sprintf("**Error Signature:** %s\n\n", incident.Analysis.ErrorSignature))
		}
		if incident.Analysis.Prevention != "" {
			sb.WriteString(fmt.Sprintf("**Prevention:** %s\n\n", incident.Analysis.Prevention))
		}
	}

	// Lessons Learned section
	if incident.Analysis != nil && incident.Analysis.LessonsLearned != nil {
		ll := incident.Analysis.LessonsLearned
		sb.WriteString("## Lessons Learned\n\n")
		if ll.WhatWentWell != "" {
			sb.WriteString(fmt.Sprintf("**What Went Well:** %s\n\n", ll.WhatWentWell))
		}
		if ll.WhatCouldBeBetter != "" {
			sb.WriteString(fmt.Sprintf("**What Could Be Better:** %s\n\n", ll.WhatCouldBeBetter))
		}
		if ll.WhereWeGotLucky != "" {
			sb.WriteString(fmt.Sprintf("**Where We Got Lucky:** %s\n\n", ll.WhereWeGotLucky))
		}
	}

	// Action Items section
	if len(incident.ActionItems) > 0 {
		sb.WriteString("## Action Items\n\n")
		sb.WriteString("| ID | Priority | Status | Owner | Description | Due Date |\n")
		sb.WriteString("|----|----------|--------|-------|-------------|----------|\n")
		
		for _, item := range incident.ActionItems {
			dueDateStr := "—"
			if !item.DueDate.IsZero() {
				dueDateStr = item.DueDate.Format("2006-01-02")
			}
			
			sb.WriteString(fmt.Sprintf("| `%s` | %s | %s | %s | %s | %s |\n",
				item.ID,
				item.Priority,
				item.Status,
				item.Owner,
				item.Description,
				dueDateStr,
			))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Timeline\n\n")
	for _, event := range incident.Events {
		sb.WriteString(fmt.Sprintf("[%s] %s - %s\n", 
			event.Timestamp.Format("15:04"), 
			event.User, 
			EventDescription(event)))
	}

	return sb.String()
}


// formatTimelineLine is a package-local helper for other files in the incident package
func formatTimelineLine(event Event) string {
	msg := EventDescription(event)
	timeLabel := event.Timestamp.Format("15:04")
	user := strings.TrimSpace(event.User)
	if user == "" {
		user = "unknown"
	}
	return fmt.Sprintf("[%s] %s - %s\n", timeLabel, user, msg)
}

// eventDescription is a package-local alias for EventDescription to satisfy legacy references
func eventDescription(event Event) string {
	return EventDescription(event)
}

func resolveExportPath(exportPath string, id string, ext string) string {
	filename := "incident-" + id + "." + ext
	clean := strings.TrimSpace(exportPath)
	if clean == "" {
		return filepath.Join(os.TempDir(), "health-monitor", filename)
	}
	clean = filepath.Clean(clean)
	if clean == "." || hasPathTraversal(clean) {
		return filepath.Join(os.TempDir(), filename)
	}
	if strings.HasSuffix(clean, "/") || strings.HasSuffix(clean, string(os.PathSeparator)) {
		return filepath.Join(clean, filename)
	}
	if info, err := os.Stat(clean); err == nil && info.IsDir() {
		return filepath.Join(clean, filename)
	}
	if strings.HasSuffix(clean, "."+ext) {
		return clean
	}
	if strings.HasSuffix(clean, ".md") || strings.HasSuffix(clean, ".json") {
		return filepath.Join(filepath.Dir(clean), filename)
	}
	return filepath.Join(clean, filename)
}

func EventDescription(event Event) string {
	switch event.Type {
	case EventStart:
		return "Started incident"
	case EventAck:
		return "Acknowledged"
	case EventResolve:
		if strings.TrimSpace(event.Message) != "" {
			return "Resolved - " + strings.TrimSpace(event.Message)
		}
		return "Resolved"
	case EventNote:
		if strings.TrimSpace(event.Message) != "" {
			return strings.TrimSpace(event.Message)
		}
		return "Note added"
	case EventSuggest:
		return "Suggested incident"
	case EventSeverity:
		if strings.TrimSpace(event.Message) != "" {
			return strings.TrimSpace(event.Message)
		}
		return "Severity updated"
	default:
		if strings.TrimSpace(event.Message) != "" {
			return strings.TrimSpace(event.Message)
		}
		return "Event recorded"
	}
}

func resolveImpactedFlows(serviceName string) ([]string, string) {
	result, err := flow.LoadOnce()
	if err != nil {
		switch {
		case flow.IsNoConfig(err):
			return nil, "Flow mapping unavailable (no flow config)"
		case flow.IsInvalidConfig(err):
			return nil, "Flow mapping unavailable (invalid flow config)"
		default:
			return nil, "Flow mapping unavailable (failed to load flows)"
		}
	}
	if len(result.Flows) == 0 {
		return nil, ""
	}
	catalog := flow.NewCatalog(result.Flows)
	impacted := catalog.ImpactedFlows(serviceName)
	if len(impacted) == 0 {
		return nil, ""
	}
	names := make([]string, 0, len(impacted))
	for _, item := range impacted {
		names = append(names, item.DisplayName())
	}
	return names, ""
}
