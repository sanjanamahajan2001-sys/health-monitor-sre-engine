package slack

import (
	"fmt"
	"log"
	"strings"

	"health-monitor/internal/config"
)

// formatMessage creates the Slack message according to the strict format requirements
func (s *SlackNotifier) formatMessage(event NotificationEvent) string {
	log.Printf("DEBUG: formatMessage started for incident %s", event.Incident.ID)
	
	// Build base message
	emoji := getSeverityEmoji(event.Incident.Severity)
	state := formatStateForTitle(event.Type)
	
	message := fmt.Sprintf("%s Incident %s | %s | %s\nTitle: %s\nService: %s\nSeverity: %s\nIncident ID: %s", 
		emoji, state, event.Incident.Severity, event.Incident.Service, event.Incident.Title, event.Incident.Service, 
		event.Incident.Severity, event.Incident.ID)
	
	if event.Incident.Cluster != "" || event.Incident.Namespace != "" {
		message += "\n\n☸️ Kubernetes Context:"
		if event.Incident.Cluster != "" {
			message += fmt.Sprintf("\n• Cluster: %s", event.Incident.Cluster)
		}
		if event.Incident.Namespace != "" {
			message += fmt.Sprintf("\n• Namespace: %s", event.Incident.Namespace)
		}
		if event.Incident.Deployment != "" {
			message += fmt.Sprintf("\n• Deployment: %s", event.Incident.Deployment)
		}
	}

	message += fmt.Sprintf("\nTriggered at: %s UTC", event.Timestamp.UTC().Format("2006-01-02 15:04"))
	
	// Add Summary if available
	if event.Incident.Summary != "" {
		message += fmt.Sprintf("\n\n📝 Summary: %s", event.Incident.Summary)
	}
	
	// Add event-specific details
	switch event.Type {
	case "acknowledged":
		// Show who acknowledged
		ackedBy := getLastUserByEventType(event.Incident.Events, "ack")
		if ackedBy != "" {
			message += fmt.Sprintf("\n👤 Acknowledged by: %s", ackedBy)
		}
		
		// Show last 3 notes
		notes := getRecentNotes(event.Incident.Events, 3)
		if len(notes) > 0 {
			message += "\n\n💬 Recent Notes:"
			for _, note := range notes {
				message += fmt.Sprintf("\n• [%s] %s: %s", 
					note.Timestamp.UTC().Format("15:04"), note.User, note.Message)
			}
		}
		
	case "resolved":
		// Show resolution details
		if event.Incident.Analysis != nil && event.Incident.Analysis.FixSummary != "" {
			message += fmt.Sprintf("\n✅ Resolution: %s", event.Incident.Analysis.FixSummary)
		}
		
		notes := getRecentNotes(event.Incident.Events, 5)
		if len(notes) > 0 {
			message += "\n\n💬 Notes:"
			for _, note := range notes {
				message += fmt.Sprintf("\n• [%s] %s: %s", 
					note.Timestamp.UTC().Format("15:04"), note.User, truncateString(note.Message, 100))
			}
		}
	case "scorecard":
		// The summary contains the whole pre-formatted scorecard text
		return event.Incident.Summary
	}

	// Add Impact metrics if available
	if event.Incident.Impact != nil {
		message += "\n\n💥 Impact:"
		if event.Incident.Impact.EstimatedDowntimeMinutes > 0 {
			message += fmt.Sprintf("\n• Downtime: %d min", event.Incident.Impact.EstimatedDowntimeMinutes)
		}
		if len(event.Incident.Impact.ImpactedFlows) > 0 {
			message += fmt.Sprintf("\n• Flow(s): %s", strings.Join(event.Incident.Impact.ImpactedFlows, ", "))
		}
		if event.Incident.Impact.CustomMetrics != "" {
			message += fmt.Sprintf("\n• Metrics: %s", event.Incident.Impact.CustomMetrics)
		}
	}
	
	// Add RCA Analysis / Predictive Intelligence
	if event.Incident.Analysis != nil {
		a := event.Incident.Analysis
		
		if event.Type == "suggested" {
			message += "\n\n🧠 Predictive Intelligence:"
			if a.MatchConfidence > 0 {
				emoji := "🔴"
				if a.MatchConfidence > 0.8 {
					emoji = "🟢"
				} else if a.MatchConfidence > 0.4 {
					emoji = "🟡"
				}
				message += fmt.Sprintf("\n• Confidence: %s %.2f", emoji, a.MatchConfidence)
			}
			if a.MatchSource != "" {
				message += fmt.Sprintf("\n• Logic: %s", a.MatchSource)
			}
			if a.RootCause != "" {
				message += fmt.Sprintf("\n• Potential Cause: %s", truncateString(a.RootCause, 150))
			}
			if a.FixSummary != "" {
				message += fmt.Sprintf("\n• Recommended Fix: %s", truncateString(a.FixSummary, 150))
			}
			
			if a.LessonsLearned != nil {
				l := a.LessonsLearned
				if l.WhatWentWell != "" || l.WhereWeGotLucky != "" {
					message += "\n\n💡 Historical Metadata:"
					if l.WhatWentWell != "" {
						message += fmt.Sprintf("\n• Past success: %s", truncateString(l.WhatWentWell, 100))
					}
					if l.WhereWeGotLucky != "" {
						message += fmt.Sprintf("\n• Luck factor: %s", truncateString(l.WhereWeGotLucky, 100))
					}
				}
			}
		} else {
			message += "\n\n🔍 Analysis:"
			if a.Category != "" {
				message += fmt.Sprintf("\n• Category: %s", a.Category)
			}
			if a.Component != "" {
				message += fmt.Sprintf("\n• Component: %s", a.Component)
			}
			if a.RootCause != "" {
				message += fmt.Sprintf("\n• Root Cause: %s", truncateString(a.RootCause, 150))
			}
			if a.Prevention != "" {
				message += fmt.Sprintf("\n• Prevention: %s", truncateString(a.Prevention, 100))
			}
		}
	}

	// Add Action Items if available
	if len(event.Incident.ActionItems) > 0 {
		message += "\n\n📅 Action Items:"
		for _, item := range event.Incident.ActionItems {
			status := "⏳"
			if item.Status == "DONE" {
				status = "✅"
			}
			message += fmt.Sprintf("\n• %s [%s] %s (%s)", status, item.Priority, item.Description, item.Owner)
		}
	}
	
	// Add log correlation data if available
	if event.Incident.Logs != nil && len(event.Incident.Logs.TopErrors) > 0 {
		message += "\n\n📊 Top Errors:"
		maxErrors := 3
		if event.Type == "resolved" {
			maxErrors = 5
		}
		for i, error := range event.Incident.Logs.TopErrors {
			if i >= maxErrors {
				break
			}
			message += fmt.Sprintf("\n• %dx %s", error.Count, truncateString(error.Message, 80))
		}
	}
	
	// Add trace correlation data if available
	if event.Incident.Traces != nil && (event.Incident.Traces.TraceCount > 0 || event.Incident.Traces.ErrorCount > 0) {
		message += "\n\n🔎 Traces:"
		if event.Incident.Traces.P95LatencyMs > 0 {
			message += fmt.Sprintf("\nP95 Latency: %.2fs", event.Incident.Traces.P95LatencyMs/1000)
		}
		if event.Incident.Traces.ErrorCount > 0 {
			message += fmt.Sprintf("\nErrors: %d", event.Incident.Traces.ErrorCount)
		}
		if event.Incident.Traces.SlowestRoute != "" {
			message += fmt.Sprintf("\nSlowest: %s (P95: %.2fs)", 
				event.Incident.Traces.SlowestRoute, 
				event.Incident.Traces.P95LatencyMs/1000)
		}
	}
	
	// Add observability links if available
	if hasObservabilityLinks(event.Incident.Links) {
		message += "\n\n🔗 Observability:"
		if event.Incident.Links.Grafana != "" {
			message += fmt.Sprintf("\n• Grafana: %s", event.Incident.Links.Grafana)
		}
		if event.Incident.Links.Prometheus != "" {
			message += fmt.Sprintf("\n• Prometheus: %s", event.Incident.Links.Prometheus)
		}
		if event.Incident.Links.Logs != "" {
			message += fmt.Sprintf("\n• Logs: %s", event.Incident.Links.Logs)
		}
		if event.Incident.Links.Traces != "" {
			message += fmt.Sprintf("\n• Traces: %s", event.Incident.Links.Traces)
		}
	}
	
	// Add runbook link if available
	if runbookURL := getRunbookURL(event.Incident.Service); runbookURL != "" {
		message += fmt.Sprintf("\n\n📘 Runbook: %s", runbookURL)
	}
	
	log.Printf("DEBUG: formatMessage completed for incident %s", event.Incident.ID)
	return message
}

func getRunbookURL(service string) string {
	cfg, err := config.Load()
	if err != nil {
		return ""
	}
	
	if cfg.RunbookURLs == nil {
		return ""
	}
	
	return cfg.RunbookURLs[service]
}

// getSeverityEmoji returns the appropriate emoji for severity level
func getSeverityEmoji(severity Severity) string {
	switch severity {
	case SeverityP1:
		return "🚨"
	case SeverityP2:
		return "⚠️"
	case SeverityP3:
		return "ℹ️"
	default:
		return "ℹ️"
	}
}

// formatStateForTitle formats the state for the title line
func formatStateForTitle(eventType string) string {
	switch eventType {
	case "started":
		return "Started"
	case "suggested":
		return "Suggested"
	case "acknowledged":
		return "Acknowledged"
	case "resolved":
		return "Resolved"
	default:
		return "Unknown"
	}
}

// formatStateDisplay formats the state for the body
func formatStateDisplay(eventType string) string {
	switch eventType {
	case "started":
		return "Started"
	case "suggested":
		return "Suggested"
	case "acknowledged":
		return "Acknowledged"
	case "resolved":
		return "Resolved"
	default:
		return "Unknown"
	}
}

// hasObservabilityLinks checks if incident has any observability links
func hasObservabilityLinks(links ObservabilityLinks) bool {
	return links.Grafana != "" || links.Prometheus != "" || links.Logs != "" || links.Traces != ""
}
