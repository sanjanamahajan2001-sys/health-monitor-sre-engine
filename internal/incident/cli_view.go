package incident

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"health-monitor/internal/flow"
	"health-monitor/internal/ml"
	"health-monitor/internal/output"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"
	"golang.org/x/term"
)

var (
	cliTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	cliLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("7")).Bold(true)
	cliDimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	cliBoxStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2)
	
	// Color styles for severity and state
	p1Style = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)   // Red
	p2Style = lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true) // Orange  
	p3Style = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)  // Yellow
	p4Style = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)  // Green
	yellowStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true) // Yellow
	dimStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))            // Dim
	resolvedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true) // Green

	// Lifecycle styles
	steadyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10")) // Green
	preStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("11")) // Yellow
	failStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("208")) // Orange
	duringStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9")) // Red
)

// stripANSI is no longer used but kept for backward compatibility if needed by other internal functions
func stripANSI(s string) string {
	// Regex to match ANSI escape sequences
	ansiRegex := regexp.MustCompile(`\x1b\[[0-9;]*[mGKHJABCD]`)
	return ansiRegex.ReplaceAllString(s, "")
}

func FormatIncidentViewCLI(incident Incident) string {
	output.Debugf("FormatIncidentViewCLI called for incident %s", incident.ID)
	
	content := GenerateIncidentDetails(incident, 0)
	
	// Better terminal width detection
	width := 80 // Default
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		width = w
	}
	
	// Create a styled box that respects terminal width
	// We subtract padding and borders (approx 4-6 chars)
	maxBoxWidth := width
	if maxBoxWidth > 100 {
		maxBoxWidth = 100 // Cap for readability
	}
	
	style := cliBoxStyle.Copy().Width(maxBoxWidth - 4)
	styled := style.Render(content)
	
	return styled + "\n"
}

func minLen(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func isTerminal() bool {
	// Check various environment variables and terminal detection
	forceCLI := os.Getenv("HEALTH_MONITOR_FORCE_CLI")
	forceColor := os.Getenv("FORCE_COLOR")
	clicolorForce := os.Getenv("CLICOLOR_FORCE")
	
	if forceCLI == "1" || forceColor == "1" || clicolorForce == "1" {
		return true
	}
	
	return isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())
}

func GenerateIncidentDetails(incident Incident, width int) string {
	if width <= 0 {
		width = 80 // Default
		if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
			width = w
		}
	}
	
	var sb strings.Builder
	
	// Title
	if isTerminal() {
		title := cliTitleStyle.Render("Incident Details")
		if !strings.Contains(title, "Incident Details") {
			title = "Incident Details"
		}
		sb.WriteString(title)
	} else {
		sb.WriteString("Incident Details")
	}
	sb.WriteString("\n\n")
	
	// Basic incident information
	sb.WriteString(fmt.Sprintf("  ID: %s\n", incident.ID))
	sb.WriteString(fmt.Sprintf("  Service: %s\n", incident.Service))
	sb.WriteString(fmt.Sprintf("  Severity: %s\n", incident.Severity))
	sb.WriteString(fmt.Sprintf("  Title: %s\n", incident.Title))
	sb.WriteString(fmt.Sprintf("  State: %s\n", incident.State))
	sb.WriteString(fmt.Sprintf("  Profile: %s\n", incident.Profile))
	sb.WriteString(fmt.Sprintf("  Created: %s UTC\n", incident.CreatedAt.Format("2006-01-02 15:04:05")))
	sb.WriteString(fmt.Sprintf("  Updated: %s UTC\n", incident.UpdatedAt.Format("2006-01-02 15:04:05")))
	
	// Kubernetes Metadata Section
	if incident.Cluster != "" || incident.Namespace != "" || incident.Deployment != "" {
		sb.WriteString("\n")
		if isTerminal() {
			sb.WriteString(cliLabelStyle.Render("Kubernetes"))
		} else {
			sb.WriteString("Kubernetes")
		}
		sb.WriteString("\n")
		if isTerminal() {
			sb.WriteString(cliDimStyle.Render(strings.Repeat("─", 10)))
		} else {
			sb.WriteString(strings.Repeat("─", 10))
		}
		sb.WriteString("\n")
		
		if incident.Cluster != "" {
			sb.WriteString(lineKV("Cluster", incident.Cluster))
		}
		if incident.Namespace != "" {
			sb.WriteString(lineKV("Namespace", incident.Namespace))
		}
		if incident.Deployment != "" {
			sb.WriteString(lineKV("Deployment", incident.Deployment))
		}
		if incident.K8sContext != "" {
			sb.WriteString(lineKV("Context", incident.K8sContext))
		}
	}
	
	// Only show summary for non-resolved incidents or if it contains meaningful information
	if incident.Summary != "" && incident.State != StateResolved {
		sb.WriteString("\n")
		if isTerminal() {
			sb.WriteString(cliDimStyle.Render(strings.Repeat(" ", 2)))
			sb.WriteString(cliDimStyle.Render(incident.Summary))
		} else {
			sb.WriteString("  " + incident.Summary)
		}
		sb.WriteString("\n")
	}
	
	// Impact section
	if incident.Impact != nil {
		sb.WriteString("\n")
		if isTerminal() {
			sb.WriteString(cliLabelStyle.Render("Impact"))
		} else {
			sb.WriteString("Impact")
		}
		sb.WriteString("\n")
		if isTerminal() {
			sb.WriteString(cliDimStyle.Render(strings.Repeat("─", 6)))
		} else {
			sb.WriteString(strings.Repeat("─", 6))
		}
		sb.WriteString("\n")
		
		if incident.Impact.EstimatedDowntimeMinutes > 0 {
			sb.WriteString(lineKV("Estimated Downtime", fmt.Sprintf("%d minutes", incident.Impact.EstimatedDowntimeMinutes)))
		}
		
		// Show impacted flows - use existing flows or detect from profile
		flows := incident.Impact.ImpactedFlows
		if flows == nil || len(flows) == 0 {
			flows = detectFlowsFromProfile(incident.Profile, incident.Service)
		}
		
		if len(flows) > 0 {
			sb.WriteString(lineKV("Impacted Flows", strings.Join(flows, ", ")))
		} else {
			sb.WriteString(lineKV("Impacted Flows", "No specific flows identified"))
		}
		
		if incident.Impact.CustomMetrics != "" {
			sb.WriteString(lineKV("Custom Metrics", incident.Impact.CustomMetrics))
		}
	}
	
	// Observability links section (CLI shows "available", TUI shows URLs)
	if hasObservabilityLinks(incident.Links) {
		sb.WriteString("\n")
		if isTerminal() {
			sb.WriteString(cliLabelStyle.Render("Observability"))
		} else {
			sb.WriteString("Observability")
		}
		sb.WriteString("\n")
		if isTerminal() {
			sb.WriteString(cliDimStyle.Render(strings.Repeat("─", 13)))
		} else {
			sb.WriteString(strings.Repeat("─", 13))
		}
		sb.WriteString("\n")
		
		if incident.Links.Grafana != "" {
			sb.WriteString(lineKV("Grafana", "available"))
		}
		if incident.Links.Prometheus != "" {
			sb.WriteString(lineKV("Prometheus", "available"))
		}
		if incident.Links.Logs != "" {
			sb.WriteString(lineKV("Logs", "available"))
		}
		if incident.Links.Traces != "" {
			sb.WriteString(lineKV("Traces", "available"))
		}
	}
	
	// Logs section - show correlated errors (matching TUI exactly)
	if incident.Logs != nil && len(incident.Logs.TopErrors) > 0 {
		sb.WriteString("\n")
		if isTerminal() {
			sb.WriteString(cliLabelStyle.Render("Logs"))
		} else {
			sb.WriteString("Logs")
		}
		sb.WriteString("\n")
		if isTerminal() {
			sb.WriteString(cliDimStyle.Render(strings.Repeat("─", 4)))
		} else {
			sb.WriteString(strings.Repeat("─", 4))
		}
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf("Top errors (last %s)\n", incident.Logs.Window))
		
		for _, error := range incident.Logs.TopErrors {
			// Format: [152x] timeout contacting payment gateway
			errorLine := fmt.Sprintf("[%dx] %s", error.Count, error.Message)
			sb.WriteString("  " + errorLine + "\n")
		}
	}

	// 🚀 NEW: Pre-incident Signals Section
	preSignalsFound := false
	var preSignalsBuilder strings.Builder
	for _, snap := range incident.LifecycleSnapshots {
		if (snap.Phase == "pre-incident" || snap.Phase == "pre-failure") && snap.FullLogs != nil && len(snap.FullLogs.TopErrors) > 0 {
			if !preSignalsFound {
				preSignalsBuilder.WriteString("\n")
				if isTerminal() {
					preSignalsBuilder.WriteString(cliLabelStyle.Foreground(lipgloss.Color("214")).Render("Pre-incident Signals"))
				} else {
					preSignalsBuilder.WriteString("Pre-incident Signals")
				}
				preSignalsBuilder.WriteString("\n")
				if isTerminal() {
					preSignalsBuilder.WriteString(cliDimStyle.Render(strings.Repeat("─", 20)))
				} else {
					preSignalsBuilder.WriteString(strings.Repeat("─", 20))
				}
				preSignalsBuilder.WriteString("\n")
				preSignalsFound = true
			}
			
			ts := snap.Timestamp.Format("15:04:05")
			phaseLabel := snap.Phase
			if isTerminal() {
				if snap.Phase == "pre-failure" {
					phaseLabel = failStyle.Render(snap.Phase)
				} else {
					phaseLabel = preStyle.Render(snap.Phase)
				}
			}
			
			for _, err := range snap.FullLogs.TopErrors {
				preSignalsBuilder.WriteString(fmt.Sprintf("  [%s] %s: %s\n", ts, phaseLabel, err.Message))
			}
		}
	}
	if preSignalsFound {
		sb.WriteString(preSignalsBuilder.String())
	}
	
	// Traces section - show trace correlation (matching TUI exactly)
	if incident.Traces != nil && incident.Traces.TraceCount > 0 {
		sb.WriteString("\n")
		if isTerminal() {
			sb.WriteString(cliLabelStyle.Render("Traces"))
		} else {
			sb.WriteString("Traces")
		}
		sb.WriteString("\n")
		if isTerminal() {
			sb.WriteString(cliDimStyle.Render(strings.Repeat("─", 6)))
		} else {
			sb.WriteString(strings.Repeat("─", 6))
		}
		sb.WriteString("\n")
		
		if incident.Traces.SlowestRoute != "" {
			sb.WriteString(lineKV("Slowest route", incident.Traces.SlowestRoute))
		}
		if incident.Traces.P95LatencyMs > 0 {
			sb.WriteString(lineKV("P95 latency", fmt.Sprintf("%.0fms", incident.Traces.P95LatencyMs)))
		}
		if incident.Traces.ErrorCount > 0 {
			sb.WriteString(lineKV("Errors", fmt.Sprintf("%d", incident.Traces.ErrorCount)))
		}
		
		if len(incident.Traces.TopSpans) > 0 {
			sb.WriteString(lineKV("Top spans", ""))
			for _, span := range incident.Traces.TopSpans {
				// Format: postgres.query (2300ms)
				spanLine := fmt.Sprintf("  %s (%.0fms)", span.Name, span.DurationMs)
				sb.WriteString(spanLine + "\n")
			}
		}
		
		if incident.Traces.GrafanaURL != "" {
			sb.WriteString(lineKV("Grafana", "available"))
		}
	}
	
	// Runbook section - show runbook link if available (matching TUI exactly)
	if runbookURL := getRunbookURL(incident.Service); runbookURL != "" {
		sb.WriteString("\n")
		if isTerminal() {
			sb.WriteString(cliLabelStyle.Render("Runbook"))
		} else {
			sb.WriteString("Runbook")
		}
		sb.WriteString("\n")
		if isTerminal() {
			sb.WriteString(cliDimStyle.Render(strings.Repeat("─", 7)))
		} else {
			sb.WriteString(strings.Repeat("─", 7))
		}
		sb.WriteString("\n")
		sb.WriteString("  " + truncateURL(runbookURL, 60) + "\n")
	}
	
	// Runbook Suggestions section - show AI-powered suggestions (matching TUI exactly)
	if incident.Metadata != nil {
		if suggestion, hasSuggestion := incident.Metadata["runbook_suggestion"]; hasSuggestion {
			sb.WriteString("\n")
			if isTerminal() {
				sb.WriteString(cliLabelStyle.Render("Runbook Suggestions"))
			} else {
				sb.WriteString("Runbook Suggestions")
			}
			sb.WriteString("\n")
			if isTerminal() {
				sb.WriteString(cliDimStyle.Render(strings.Repeat("─", 18)))
			} else {
				sb.WriteString(strings.Repeat("─", 18))
			}
			sb.WriteString("\n")
			
			// Show pattern detected
			if pattern, hasPattern := incident.Metadata["runbook_pattern"]; hasPattern {
				sb.WriteString(lineKV("Pattern detected", pattern))
			}
			
			// Show confidence
			if confidence, hasConfidence := incident.Metadata["runbook_confidence"]; hasConfidence {
				sb.WriteString(lineKV("Confidence", confidence))
			}
			
			// Show suggested runbook
			sb.WriteString(lineKV("Suggested runbook", suggestion))
			
			// Show generation time if available
			if generatedAt, hasGeneratedAt := incident.Metadata["runbook_generated_at"]; hasGeneratedAt {
				if t, err := time.Parse(time.RFC3339, generatedAt); err == nil {
					sb.WriteString(lineKV("Generated", t.Format("2006-01-02 15:04:05 UTC")))
				}
			}
			
			sb.WriteString("\n")
			sb.WriteString(fmt.Sprintf("💡 Tip: Use 'health-monitor runbook suggest %s' for detailed steps\n", incident.ID))
		}
	}
	
	// Lifecycle section (Traffic Light)
	if len(incident.LifecycleSnapshots) > 0 {
		sb.WriteString("\n")
		if isTerminal() {
			sb.WriteString(cliLabelStyle.Render("Lifecycle (SRE Traffic Light)"))
		} else {
			sb.WriteString("Lifecycle (SRE Traffic Light)")
		}
		sb.WriteString("\n")
		if isTerminal() {
			sb.WriteString(cliDimStyle.Render(strings.Repeat("─", 28)))
		} else {
			sb.WriteString(strings.Repeat("─", 28))
		}
		sb.WriteString("\n")
		
		for _, snap := range incident.LifecycleSnapshots {
			sb.WriteString(renderLifecycleSnapshot(snap))
		}
	}

	// Timeline section (matching TUI exactly)
	sb.WriteString("\n")
	if isTerminal() {
		sb.WriteString(cliLabelStyle.Render("Timeline"))
	} else {
		sb.WriteString("Timeline")
	}
	sb.WriteString("\n")
	if isTerminal() {
		sb.WriteString(cliDimStyle.Render(strings.Repeat("─", 8)))
	} else {
		sb.WriteString(strings.Repeat("─", 8))
	}
	sb.WriteString("\n")
	
	for _, event := range incident.Events {
		sb.WriteString(formatTimelineLine(event))
	}
	
	// RCA Analysis section - show root cause analysis
	if incident.Analysis != nil {
		sb.WriteString("\n")
		if isTerminal() {
			sb.WriteString(cliLabelStyle.Render("Root Cause Analysis"))
		} else {
			sb.WriteString("Root Cause Analysis")
		}
		sb.WriteString("\n")
		if isTerminal() {
			sb.WriteString(cliDimStyle.Render(strings.Repeat("─", 19)))
		} else {
			sb.WriteString(strings.Repeat("─", 19))
		}
		sb.WriteString("\n")
		
		if incident.Analysis.Component != "" {
			sb.WriteString(lineKV("Component", incident.Analysis.Component))
		}
		if incident.Analysis.Category != "" {
			sb.WriteString(lineKV("Category", incident.Analysis.Category))
		}
		if incident.Analysis.RootCause != "" {
			sb.WriteString(lineKV("Root Cause", incident.Analysis.RootCause))
		}
		if incident.Analysis.FixSummary != "" {
			sb.WriteString(lineKV("Fix Summary", incident.Analysis.FixSummary))
		}
		if incident.Analysis.Prevention != "" {
			sb.WriteString(lineKV("Prevention", incident.Analysis.Prevention))
		}
	}
	
	// Similar Incidents section
	if incident.State == StateResolved || incident.Analysis != nil {
		sb.WriteString("\n")
		if isTerminal() {
			sb.WriteString(cliLabelStyle.Render("Similar Incidents"))
		} else {
			sb.WriteString("Similar Incidents")
		}
		sb.WriteString("\n")
		if isTerminal() {
			sb.WriteString(cliDimStyle.Render(strings.Repeat("─", 18)))
		} else {
			sb.WriteString(strings.Repeat("─", 18))
		}
		sb.WriteString("\n")
		
		// Generate similar incidents based on service and patterns
		similarIncidents := GetDemoSimilarIncidents(incident)
		if len(similarIncidents) > 0 {
			for i, similar := range similarIncidents {
				sb.WriteString(fmt.Sprintf("   %d. %s - %s (%.1f%% similar)\n", 
					i+1, similar.Incident.ID, similar.Incident.Title, similar.Confidence*100))
				sb.WriteString(fmt.Sprintf("      Service: %s | State: %s | %s\n", 
					similar.Incident.Service, similar.Incident.State, similar.Age))
				if i < len(similarIncidents)-1 {
					sb.WriteString("\n")
				}
			}
		} else {
			sb.WriteString("   No similar incidents found in the last 90 days.\n")
		}
	}
	
	// Action Items section - show action items
	if len(incident.ActionItems) > 0 {
		sb.WriteString("\n")
		if isTerminal() {
			sb.WriteString(cliLabelStyle.Render("Action Items"))
		} else {
			sb.WriteString("Action Items")
		}
		sb.WriteString("\n")
		if isTerminal() {
			sb.WriteString(cliDimStyle.Render(strings.Repeat("─", 12)))
		} else {
			sb.WriteString(strings.Repeat("─", 12))
		}
		sb.WriteString("\n")
		
		for i, action := range incident.ActionItems {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, action.Description))
			sb.WriteString(fmt.Sprintf("   Owner: %s\n", action.Owner))
			sb.WriteString(fmt.Sprintf("   Status: %s\n", action.Status))
			sb.WriteString(fmt.Sprintf("   Priority: %s\n", action.Priority))
			if i < len(incident.ActionItems)-1 {
				sb.WriteString("\n")
			}
		}
	}
	
	// Lessons Learned section
	if incident.Analysis != nil && incident.Analysis.LessonsLearned != nil {
		sb.WriteString("\n")
		if isTerminal() {
			sb.WriteString(cliLabelStyle.Render("Lessons Learned"))
		} else {
			sb.WriteString("Lessons Learned")
		}
		sb.WriteString("\n")
		if isTerminal() {
			sb.WriteString(cliDimStyle.Render(strings.Repeat("─", 15)))
		} else {
			sb.WriteString(strings.Repeat("─", 15))
		}
		sb.WriteString("\n")
		
		if incident.Analysis.LessonsLearned.WhatWentWell != "" {
			sb.WriteString(lineKV("What Went Well", incident.Analysis.LessonsLearned.WhatWentWell))
		}
		if incident.Analysis.LessonsLearned.WhatCouldBeBetter != "" {
			sb.WriteString(lineKV("What Could Be Better", incident.Analysis.LessonsLearned.WhatCouldBeBetter))
		}
		if incident.Analysis.LessonsLearned.WhereWeGotLucky != "" {
			sb.WriteString(lineKV("Where We Got Lucky", incident.Analysis.LessonsLearned.WhereWeGotLucky))
		}
	}
	
	return sb.String()
}

func renderLifecycleSnapshot(snap ml.LifecycleSnapshot) string {
	timestamp := snap.Timestamp.Format("15:04")
	icon := "⚪"
	phase := snap.Phase
	style := cliDimStyle

	switch phase {
	case "steady":
		icon = "🟢"
		style = steadyStyle
	case "pre-incident":
		icon = "🟡"
		style = preStyle
	case "pre-failure":
		icon = "🟠"
		style = failStyle
	case "during":
		icon = "🔴"
		style = duringStyle
	case "post":
		icon = "🔵"
		style = resolvedStyle
	}

	label := phase
	if isTerminal() {
		label = style.Render(phase)
	}

	// Extract key metrics if available
	metricSummary := ""
	if snap.Metrics != nil {
		if lat, ok := snap.Metrics["service_p95_latency"]; ok && lat > 0 {
			metricSummary += fmt.Sprintf(" p95=%.0fms", lat)
		} else if lat, ok := snap.Metrics["service_avg_latency"]; ok && lat > 0 {
			metricSummary += fmt.Sprintf(" avg=%.0fms", lat)
		}
		if errs, ok := snap.Metrics["service_error_count"]; ok && errs > 0 {
			metricSummary += fmt.Sprintf(" errs=%.0f", errs)
		}
	}

	return fmt.Sprintf("   [%s] %s %s%s\n", timestamp, icon, padRight(label, 12), metricSummary)
}


func lineKV(label string, value string) string {
	value = strings.TrimSpace(value)
	if strings.Contains(value, "\n") {
		value = strings.ReplaceAll(value, "\n", "\n  ")
	}
	return fmt.Sprintf("   %s: %s\n", label, value)
}

// detectFlowsFromProfile reads flows from the actual profile configuration file
func detectFlowsFromProfile(profile string, service string) []string {
	var flows []string
	
	// Construct the profile file path
	profilePath := fmt.Sprintf("/etc/health-monitor/flows.d/%s.yaml", profile)
	
	// Read the profile file
	data, err := os.ReadFile(profilePath)
	if err != nil {
		// If profile file doesn't exist, try to construct flow name from service
		return []string{fmt.Sprintf("%s-flow", service)}
	}
	
	// Parse YAML content (simple string-based parsing for now)
	content := string(data)
	lines := strings.Split(content, "\n")
	
	inFlowsSection := false
	currentService := ""
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		
		// Check if we're in the flows section
		if strings.HasPrefix(line, "flows:") {
			inFlowsSection = true
			continue
		}
		
		// Check if we found the service section
		if inFlowsSection && strings.HasSuffix(line, ":") && !strings.HasPrefix(line, " ") {
			serviceName := strings.TrimSuffix(line, ":")
			if serviceName == service {
				currentService = service
				continue
			} else {
				currentService = ""
				continue
			}
		}
		
		// If we're in the right service section, look for SLOs
		if currentService == service && strings.Contains(line, "id:") {
			// Extract flow ID from the line
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				flowID := strings.TrimSpace(parts[1])
				flowID = strings.Trim(flowID, "\"") // Remove quotes if present
				if flowID != "" {
					flows = append(flows, flowID)
				}
			}
		}
	}
	
	// If no flows found, construct a default flow name
	if len(flows) == 0 {
		flows = append(flows, fmt.Sprintf("%s-flow", service))
	}
	
	return flows
}


func formatWithASCIIBox(content string) string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return content
	}
	
	maxLength := 0
	for _, line := range lines {
		if len(line) > maxLength {
			maxLength = len(line)
		}
	}
	
	maxLength += 4
	
	var result strings.Builder
	result.WriteString("+" + strings.Repeat("-", maxLength) + "+\n")
	
	for _, line := range lines {
		result.WriteString("|  " + line + strings.Repeat(" ", maxLength-len(line)-4) + "  |\n")
	}
	
	result.WriteString("+" + strings.Repeat("-", maxLength) + "+\n")
	
	cleanContent := stripANSI(content)
	
	// Detect terminal width
	width := 80
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		width = w
	}
	
	maxBoxWidth := width
	if maxBoxWidth > 120 { // List can be wider
		maxBoxWidth = 120
	}

	style := cliBoxStyle.Copy().Width(maxBoxWidth - 4)
	styled := style.Render(cleanContent)

	return styled + "\n"
}

func hasObservabilityLinks(links ObservabilityLinks) bool {
	return links.Grafana != "" || links.Prometheus != "" || links.Logs != "" || links.Traces != ""
}

func impactedFlowsValue(serviceName string) string {
	result, err := flow.LoadOnce()
	if err != nil {
		return "unavailable"
	}
	if len(result.Flows) == 0 {
		return "none"
	}
	catalog := flow.NewCatalog(result.Flows)
	impacted := catalog.ImpactedFlows(serviceName)
	if len(impacted) == 0 {
		return "none"
	}
	var sb strings.Builder
	for i, item := range impacted {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("• " + item.DisplayName())
	}
	return sb.String()
}

// FormatIncidentListCLI formats a list of incidents for terminal output
func FormatIncidentListCLI(incidents []Incident) string {
	if len(incidents) == 0 {
		return cliBoxStyle.Render("No incidents found.") + "\n"
	}

	// Define headers
	headers := []string{"ID", "TITLE", "PROFILE", "SEVERITY", "STATE", "CREATED"}

	// Calculate column widths
	widths := make(map[string]int)
	for _, h := range headers {
		widths[h] = len(h)
	}

	for _, inc := range incidents {
		if len(inc.ID) > widths["ID"] {
			widths["ID"] = len(inc.ID)
		}
		if len(inc.Title) > widths["TITLE"] {
			widths["TITLE"] = len(inc.Title)
		}
		if len(inc.Profile) > widths["PROFILE"] {
			widths["PROFILE"] = len(inc.Profile)
		}
		if len(string(inc.Severity)) > widths["SEVERITY"] {
			widths["SEVERITY"] = len(string(inc.Severity))
		}
		if len(string(inc.State)) > widths["STATE"] {
			widths["STATE"] = len(string(inc.State))
		}
	}

	// Cap TITLE width for readability
	if widths["TITLE"] > 50 {
		widths["TITLE"] = 50
	}

	var sb strings.Builder
	if isTerminal() {
		sb.WriteString(cliTitleStyle.Render("Incidents"))
	} else {
		sb.WriteString("Incidents")
	}
	sb.WriteString("\n\n")

	// Header row
	headerRow := fmt.Sprintf("%s  %s  %s  %s  %s  %s\n",
		padRight("ID", widths["ID"]),
		padRight("TITLE", widths["TITLE"]),
		padRight("PROFILE", widths["PROFILE"]),
		padRight("SEVERITY", widths["SEVERITY"]),
		padRight("STATE", widths["STATE"]),
		"CREATED")
	sb.WriteString(headerRow)

	// Separator
	totalWidth := widths["ID"] + widths["TITLE"] + widths["PROFILE"] + widths["SEVERITY"] + widths["STATE"] + 10 + 10
	if isTerminal() {
		sb.WriteString(cliDimStyle.Render(strings.Repeat("─", totalWidth)))
	} else {
		sb.WriteString(strings.Repeat("─", totalWidth))
	}
	sb.WriteString("\n")

	for _, inc := range incidents {
		id := inc.ID
		title := inc.Title
		if len(title) > widths["TITLE"] {
			title = title[:widths["TITLE"]-3] + "..."
		}
		profile := inc.Profile
		if profile == "" {
			profile = "-"
		}
		severity := colorSeverity(string(inc.Severity))
		state := colorState(string(inc.State))
		created := inc.CreatedAt.Format("2006-01-02")

		sb.WriteString(fmt.Sprintf("%s  %s  %s  %s  %s  %s\n",
			padRight(id, widths["ID"]),
			padRight(title, widths["TITLE"]),
			padRight(profile, widths["PROFILE"]),
			padRight(severity, widths["SEVERITY"]),
			padRight(state, widths["STATE"]),
			created))
	}

	content := sb.String()

	// Detect terminal width
	width := 80
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		width = w
	}

	maxBoxWidth := width
	if maxBoxWidth > 130 {
		maxBoxWidth = 130
	}

	style := cliBoxStyle.Copy().Width(maxBoxWidth - 4)
	styled := style.Render(content)

	return styled + "\n"
}

// padRight pads a string to the right, respecting ANSI color codes using lipgloss.Width()
func padRight(s string, width int) string {
	sWidth := lipgloss.Width(s)
	if sWidth >= width {
		return s
	}
	return s + strings.Repeat(" ", width-sWidth)
}

func getRunbookURL(service string) string {
	return "https://example.com/runbook/" + service
}

func colorSeverity(sev string) string {
	if !isTerminal() {
		return sev // No colors if not a terminal
	}
	
	switch sev {
	case "P1":
		return p1Style.Render("P1")
	case "P2":
		return p2Style.Render("P2")
	case "P3":
		return p3Style.Render("P3")
	case "P4":
		return p4Style.Render("P4")
	default:
		return sev
	}
}

func colorState(state string) string {
	if !isTerminal() {
		return state
	}

	switch strings.ToLower(state) {
	case "active", "started":
		return p1Style.Render(state)
	case "resolved":
		return resolvedStyle.Render(state)
	case "mitigated", "acknowledged":
		return yellowStyle.Render(state)
	case "suggested":
		return dimStyle.Render(state)
	default:
		return state
	}
}
