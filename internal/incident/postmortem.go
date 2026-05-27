package incident

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// GeneratePostmortem generates a blameless postmortem report for an incident
func (s *Service) GeneratePostmortem(id string) (string, error) {
	incident, err := s.store.Load(id)
	if err != nil {
		return "", fmt.Errorf("failed to load incident %s: %w", id, err)
	}

	// 🚀 NEW: Refresh K8s metadata for postmortem
	if s.discoveryEngine != nil {
		s.EnrichWithK8sMetadata(&incident)
	}

	// Enrich analysis with similarity data for context
	opts := DefaultSimilarityOptions()
	similar, _ := s.FindSimilarIncidents(&incident, opts)

	content := renderPostmortemMarkdown(incident, similar)
	
	// Save to a specialized postmortem directory or export path
	exportPath := os.Getenv("HEALTH_MONITOR_POSTMORTEM_PATH")
	if exportPath == "" {
		exportPath = os.Getenv("HEALTH_MONITOR_EXPORT_PATH")
	}
	
	path := resolveExportPath(exportPath, incident.ID, "postmortem.md")
	
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", fmt.Errorf("failed to create export directory: %w", err)
	}

	if err := os.WriteFile(path, []byte(content+"\n"), 0644); err != nil {
		return "", fmt.Errorf("failed to save postmortem: %w", err)
	}

	// Add an audit note to the incident
	s.NoteByID(id, fmt.Sprintf("Generated postmortem report: %s", path), "system")

	return path, nil
}

func renderPostmortemMarkdown(incident Incident, similar []SimilarIncident) string {
	var sb strings.Builder

	// YAML Frontmatter for programmatic tools
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("id: %s\n", incident.ID))
	sb.WriteString(fmt.Sprintf("title: %s\n", incident.Title))
	sb.WriteString(fmt.Sprintf("date: %s\n", incident.CreatedAt.Format("2006-01-02")))
	sb.WriteString(fmt.Sprintf("service: %s\n", incident.Service))
	sb.WriteString(fmt.Sprintf("severity: %s\n", incident.Severity))
	if incident.Cluster != "" {
		sb.WriteString(fmt.Sprintf("cluster: %s\n", incident.Cluster))
	}
	if incident.Namespace != "" {
		sb.WriteString(fmt.Sprintf("namespace: %s\n", incident.Namespace))
	}
	if incident.Deployment != "" {
		sb.WriteString(fmt.Sprintf("deployment: %s\n", incident.Deployment))
	}
	if incident.K8sContext != "" {
		sb.WriteString(fmt.Sprintf("k8s_context: %s\n", incident.K8sContext))
	}
	sb.WriteString("---\n\n")

	sb.WriteString(fmt.Sprintf("# Postmortem: %s (%s)\n\n", incident.Title, incident.ID))
	
	// ... (TOC remains same) ...
	
	sb.WriteString("## <a name=\"overview\"></a>📝 Overview\n")
	sb.WriteString(fmt.Sprintf("**Date:** %s\n", incident.CreatedAt.Format("2006-01-02")))
	sb.WriteString(fmt.Sprintf("**Status:** %s\n", incident.State))
	sb.WriteString(fmt.Sprintf("**Severity:** %s\n", incident.Severity))
	sb.WriteString(fmt.Sprintf("**Service:** `%s`\n", incident.Service))
	sb.WriteString(fmt.Sprintf("**Profile:** `%s`\n", incident.Profile))
	
	if incident.Cluster != "" || incident.Namespace != "" || incident.Deployment != "" {
		sb.WriteString("\n### Kubernetes Context\n")
		if incident.Cluster != "" {
			sb.WriteString(fmt.Sprintf("- **Cluster:** %s\n", incident.Cluster))
		}
		if incident.Namespace != "" {
			sb.WriteString(fmt.Sprintf("- **Namespace:** %s\n", incident.Namespace))
		}
		if incident.Deployment != "" {
			sb.WriteString(fmt.Sprintf("- **Deployment:** %s\n", incident.Deployment))
		}
		if incident.K8sContext != "" {
			sb.WriteString(fmt.Sprintf("- **Context:** %s\n", incident.K8sContext))
		}
	}
	
	// MTTR Calculation
	if incident.State == StateResolved {
		duration := incident.UpdatedAt.Sub(incident.CreatedAt)
		sb.WriteString(fmt.Sprintf("**Time to Resolve:** %v\n", duration.Round(time.Minute)))
	}
	
	// Detected By
	detectedBy := "Manual"
	for _, evt := range incident.Events {
		if evt.Type == EventSuggest {
			detectedBy = "Automated (Health-Monitor Suggestion)"
			break
		}
	}
	sb.WriteString(fmt.Sprintf("**Detected By:** %s\n\n", detectedBy))

	if incident.Summary != "" {
		sb.WriteString("### Summary\n")
		sb.WriteString(incident.Summary + "\n\n")
	}

	// Impact Section
	sb.WriteString("## <a name=\"impact\"></a>💥 Impact\n")
	if incident.Impact != nil {
		if incident.Impact.EstimatedDowntimeMinutes > 0 {
			sb.WriteString(fmt.Sprintf("- **Estimated Downtime:** %d minutes\n", incident.Impact.EstimatedDowntimeMinutes))
		}
		if len(incident.Impact.ImpactedFlows) > 0 {
			sb.WriteString(fmt.Sprintf("- **Impacted User Flows:** %s\n", strings.Join(incident.Impact.ImpactedFlows, ", ")))
		}
		if incident.Impact.CustomMetrics != "" {
			sb.WriteString(fmt.Sprintf("- **Metrics:** %s\n", incident.Impact.CustomMetrics))
		}
	} else {
		// Fallback to flow resolution
		impacted, _ := resolveImpactedFlows(incident.Service)
		if len(impacted) > 0 {
			sb.WriteString(fmt.Sprintf("- **Potentially Impacted Flows:** %s\n", strings.Join(impacted, ", ")))
		} else {
			sb.WriteString("- No quantitative impact data recorded.\n")
		}
	}
	sb.WriteString("\n")

	// RCA Section
	sb.WriteString("## <a name=\"root-cause-analysis\"></a>🔍 Root Cause Analysis\n")
	if incident.Analysis != nil {
		if incident.Analysis.Category != "" {
			sb.WriteString(fmt.Sprintf("**Category:** %s\n", incident.Analysis.Category))
		}
		if incident.Analysis.FailureType != "" {
			sb.WriteString(fmt.Sprintf("**Failure Type:** %s\n", incident.Analysis.FailureType))
		}
		if incident.Analysis.Component != "" {
			sb.WriteString(fmt.Sprintf("**Component:** %s\n", incident.Analysis.Component))
		}
		if incident.Analysis.Dependency != "" {
			sb.WriteString(fmt.Sprintf("**Dependency:** %s\n", incident.Analysis.Dependency))
		}
		if incident.Analysis.RootCause != "" {
			sb.WriteString(fmt.Sprintf("**Root Cause:** %s\n", incident.Analysis.RootCause))
		}
		if incident.Analysis.FixSummary != "" {
			sb.WriteString(fmt.Sprintf("**Resolution:** %s\n", incident.Analysis.FixSummary))
		}
		if incident.Analysis.Prevention != "" {
			sb.WriteString(fmt.Sprintf("**Prevention:** %s\n", incident.Analysis.Prevention))
		}
		if incident.Analysis.Pattern != "" {
			sb.WriteString(fmt.Sprintf("**Pattern:** %s\n", incident.Analysis.Pattern))
		}
		if incident.Analysis.ErrorSignature != "" {
			sb.WriteString(fmt.Sprintf("**Error Signature:** %s\n", incident.Analysis.ErrorSignature))
		}
	} else {
		sb.WriteString("- Analysis documentation pending.\n")
	}
	sb.WriteString("\n")

	// Technical Details (Embedded)
	if (incident.Logs != nil && len(incident.Logs.TopErrors) > 0) || (incident.Traces != nil && incident.Traces.TraceCount > 0) {
		sb.WriteString("## <a name=\"technical-evidence\"></a>🛠️ Technical Evidence\n")
		
		if incident.Logs != nil && len(incident.Logs.TopErrors) > 0 {
			sb.WriteString("### Log Snapshots (Top Errors)\n")
			sb.WriteString("```log\n")
			for _, err := range incident.Logs.TopErrors {
				sb.WriteString(fmt.Sprintf("[%dx] %s\n", err.Count, err.Message))
			}
			sb.WriteString("```\n\n")
		}
		
		if incident.Traces != nil && incident.Traces.TraceCount > 0 {
			sb.WriteString("### Trace Statistics\n")
			sb.WriteString(fmt.Sprintf("- **Sample Size:** %d traces\n", incident.Traces.TraceCount))
			if incident.Traces.P95LatencyMs > 0 {
				sb.WriteString(fmt.Sprintf("- **P95 Latency:** %.0fms\n", incident.Traces.P95LatencyMs))
			}
			if incident.Traces.SlowestRoute != "" {
				sb.WriteString(fmt.Sprintf("- **Slowest Endpoint:** `%s`\n", incident.Traces.SlowestRoute))
			}
			sb.WriteString("\n")
		}
	}

	// Blameless Lessons Learned
	sb.WriteString("## <a name=\"lessons-learned-blameless\"></a>💡 Lessons Learned (Blameless)\n")
	if incident.Analysis != nil && incident.Analysis.LessonsLearned != nil {
		ll := incident.Analysis.LessonsLearned
		if ll.WhatWentWell != "" {
			sb.WriteString("### What Went Well\n")
			sb.WriteString(ll.WhatWentWell + "\n\n")
		}
		if ll.WhatCouldBeBetter != "" {
			sb.WriteString("### What Could Be Better\n")
			sb.WriteString(ll.WhatCouldBeBetter + "\n\n")
		}
		if ll.WhereWeGotLucky != "" {
			sb.WriteString("### Where We Got Lucky\n")
			sb.WriteString(ll.WhereWeGotLucky + "\n\n")
		}
	} else {
		sb.WriteString("- **What Went Well:** (Fill in positive outcomes)\n")
		sb.WriteString("- **What Could Be Better:** (Fill in process/technical gaps)\n")
		sb.WriteString("- **Where We Got Lucky:** (Fill in near-misses)\n\n")
	}

	// Action Items
	if len(incident.ActionItems) > 0 {
		sb.WriteString("## <a name=\"action-items\"></a>📅 Action Items\n")
		sb.WriteString("| ID | Priority | Status | Owner | Description | Due Date |\n")
		sb.WriteString("|----|----------|--------|-------|-------------|----------|\n")
		for _, item := range incident.ActionItems {
			dueDateStr := "—"
			if !item.DueDate.IsZero() {
				dueDateStr = item.DueDate.Format("2006-01-02")
			}
			sb.WriteString(fmt.Sprintf("| `%s` | %s | %s | %s | %s | %s |\n", 
				item.ID, item.Priority, item.Status, item.Owner, item.Description, dueDateStr))
		}
		sb.WriteString("\n")
	}

	// Timeline
	sb.WriteString("## <a name=\"timeline\"></a>⏳ Timeline\n")
	for _, evt := range incident.Events {
		icon := "🔹"
		switch evt.Type {
		case EventStart, EventSuggest: icon = "🚨"
		case EventAck: icon = "👀"
		case EventResolve: icon = "✅"
		case EventNote: icon = "📝"
		}
		sb.WriteString(fmt.Sprintf("- **%s** %s %s (%s)\n", 
			evt.Timestamp.Format("15:04"), icon, eventDescription(evt), evt.User))
	}
	sb.WriteString("\n")

	// Historical Context (Similarity)
	if len(similar) > 0 {
		sb.WriteString("## <a name=\"historical-context\"></a>📚 Historical Context\n")
		sb.WriteString("The following similar incidents were identified for context:\n\n")
		for _, sim := range similar {
			matchStr := strings.Join(sim.MatchedOn, ", ")
			sb.WriteString(fmt.Sprintf("- **%s:** %s (Match: %s, Confidence: %d%%)\n", 
				sim.Incident.ID, sim.Incident.Title, matchStr, int(sim.Confidence*100)))
		}
		sb.WriteString("\n")
	}

	// Observability Links
	if hasObservabilityLinks(incident.Links) {
		sb.WriteString("## <a name=\"observability-artifacts\"></a>🔗 Observability Artifacts\n")
		if incident.Links.Grafana != "" { sb.WriteString(fmt.Sprintf("- [Grafana Dashboard](%s)\n", incident.Links.Grafana)) }
		if incident.Links.Logs != "" { sb.WriteString(fmt.Sprintf("- [Log Explorer](%s)\n", incident.Links.Logs)) }
		if incident.Links.Traces != "" { sb.WriteString(fmt.Sprintf("- [Distributed Traces](%s)\n", incident.Links.Traces)) }
		sb.WriteString("\n")
	}

	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("*Generated by Health-Monitor Agent on %s*\n", time.Now().Format("2006-01-02 15:04 MST")))

	return sb.String()
}

