package tui

import (
	"fmt"
	"strings"

	"health-monitor/internal/demo"
	"health-monitor/internal/flow"
	"health-monitor/internal/incident"

	"github.com/charmbracelet/lipgloss"
)

// applyIncidentFilters applies the current filters to the incident list
func (m *tuiModel) applyIncidentFilters() {
	if m.incidentList == nil {
		m.filteredIncidentList = nil
		return
	}

	filtered := make([]incident.Incident, 0)
	for _, inc := range m.incidentList {
		// Apply service filter
		if m.incidentFilterService != "" {
			serviceLower := strings.ToLower(strings.TrimSpace(inc.Service))
			filterLower := strings.ToLower(strings.TrimSpace(m.incidentFilterService))
			if !strings.Contains(serviceLower, filterLower) {
				continue
			}
		}

		// Apply severity filter
		if m.incidentFilterSeverity != "" && m.incidentFilterSeverity != "all" {
			if string(inc.Severity) != m.incidentFilterSeverity {
				continue
			}
		}

		// Apply state filter
		if m.incidentFilterState != "" && m.incidentFilterState != "all" {
			stateLower := strings.ToLower(strings.TrimSpace(string(inc.State)))
			filterLower := strings.ToLower(strings.TrimSpace(m.incidentFilterState))
			// For "active" filter, show Started and Acknowledged (not Resolved)
			if filterLower == "active" {
				if stateLower == "resolved" || stateLower == "suggested" {
					continue
				}
			} else if stateLower != filterLower {
				continue
			}
		}

		filtered = append(filtered, inc)
	}

	m.filteredIncidentList = filtered
	
	// Reset page if current page is out of bounds
	if m.incidentPageSize > 0 {
		maxPage := (len(m.filteredIncidentList) - 1) / m.incidentPageSize
		if m.incidentCurrentPage > maxPage {
			m.incidentCurrentPage = 0
		}
	}
	
	// Reset selection if out of bounds
	if m.incidentSelectedIndex >= len(m.filteredIncidentList) {
		m.incidentSelectedIndex = 0
	}
}

// renderIncidentBrowser renders the incident browser (list or detail view)
func (m *tuiModel) renderIncidentBrowser() string {
	if m.incidentBrowserMode {
		return m.renderIncidentListView()
	}
	return m.renderIncidentDetailView()
}

// renderIncidentListView renders the paginated list of incidents
func (m *tuiModel) renderIncidentListView() string {
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(titleStyle.Render("Incidents Browser"))
	sb.WriteString("\n")
	
	// Show filter status
	if m.incidentFilterService != "" || m.incidentFilterSeverity != "all" || m.incidentFilterState != "all" {
		filters := []string{}
		if m.incidentFilterService != "" {
			filters = append(filters, "Service="+m.incidentFilterService)
		}
		if m.incidentFilterSeverity != "all" {
			filters = append(filters, "Severity="+m.incidentFilterSeverity)
		}
		if m.incidentFilterState != "all" {
			filters = append(filters, "State="+m.incidentFilterState)
		}
		sb.WriteString(dimStyle.Render("Filters: " + strings.Join(filters, ", ")))
		sb.WriteString("\n")
	}
	
	sb.WriteString(dimStyle.Render(strings.Repeat("─", 60)))
	sb.WriteString("\n\n")
	
	// Show error message if any
	if strings.TrimSpace(m.incidentMessage) != "" {
		sb.WriteString(dimStyle.Render(m.incidentMessage))
		sb.WriteString("\n\n")
		return sb.String()
	}
	
	// Check if we have incidents
	if len(m.filteredIncidentList) == 0 {
		if len(m.incidentList) == 0 {
			sb.WriteString(dimStyle.Render("No incidents found."))
			sb.WriteString("\n\n")
			sb.WriteString(dimStyle.Render("Incidents are created automatically by Alertmanager webhooks"))
			sb.WriteString("\n")
			sb.WriteString(dimStyle.Render("or manually via: health-monitor incident start"))
		} else {
			sb.WriteString(dimStyle.Render("No incidents match the current filters."))
			sb.WriteString("\n\n")
			sb.WriteString(dimStyle.Render("Press 'x' to clear filters"))
		}
		sb.WriteString("\n\n")
		return sb.String()
	}
	
	// Calculate pagination
	totalIncidents := len(m.filteredIncidentList)
	pageSize := m.incidentPageSize
	if pageSize <= 0 {
		pageSize = 10
	}
	startIdx := m.incidentCurrentPage * pageSize
	endIdx := startIdx + pageSize
	if endIdx > totalIncidents {
		endIdx = totalIncidents
	}
	
	// Render incidents for current page
	for i := startIdx; i < endIdx; i++ {
		inc := m.filteredIncidentList[i]
		prefix := "  "
		if i == m.incidentSelectedIndex {
			prefix = "> "
		}
		
		// Format: [P1] INC-20260213-163000 | billing_api | High Error Rate
		sevStyle := getSeverityStyle(string(inc.Severity))
		stateStyle := getStateStyle(string(inc.State))
		
		line := fmt.Sprintf("%s[%s] %s | %s | %s | %s",
			prefix,
			sevStyle.Render(string(inc.Severity)),
			inc.ID,
			inc.Service,
			stateStyle.Render(string(inc.State)),
			truncateString(inc.Title, 30),
		)
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	
	// Show pagination info
	sb.WriteString("\n")
	totalPages := (totalIncidents + pageSize - 1) / pageSize
	currentPage := m.incidentCurrentPage + 1
	sb.WriteString(dimStyle.Render(fmt.Sprintf("Showing %d-%d of %d incidents (Page %d/%d)",
		startIdx+1, endIdx, totalIncidents, currentPage, totalPages)))
	sb.WriteString("\n\n")
	
	// Show controls
	sb.WriteString(markdownBlock("## Controls"))
	sb.WriteString("↑/↓  Navigate  Enter View Details  1-4 Filter P1-P4\n")
	sb.WriteString("a    Active    r     Resolved      x   Clear Filters\n")
	sb.WriteString("q    Exit\n")
	
	return sb.String()
}

// renderIncidentDetailView renders the full details of the selected incident
func (m *tuiModel) renderIncidentDetailView() string {
	// Build full content first
	fullContent := m.buildIncidentDetailContent()
	
	if m.selectedIncident == nil {
		// Return early if no incident selected
		var sb strings.Builder
		sb.WriteString("\n")
		sb.WriteString(titleStyle.Render("Incident Details"))
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render(strings.Repeat("─", 60)))
		sb.WriteString("\n\n")
		sb.WriteString(dimStyle.Render("No incident selected."))
		sb.WriteString("\n\n")
		return sb.String()
	}
	
	// Calculate viewport for scrolling
	viewportHeight := m.height - 12 // Reserve space for header, controls, and margins
	if viewportHeight < 10 {
		viewportHeight = 10
	}
	
	// Split into lines for scrolling
	lines := strings.Split(fullContent, "\n")
	totalLines := len(lines)
	
	// Adjust scroll position
	if m.incidentScrollY < 0 {
		m.incidentScrollY = 0
	}
	maxScroll := totalLines - viewportHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.incidentScrollY > maxScroll {
		m.incidentScrollY = maxScroll
	}
	
	// Extract visible lines
	startLine := m.incidentScrollY
	endLine := startLine + viewportHeight
	if endLine > totalLines {
		endLine = totalLines
	}
	
	// Build final output with fixed header
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(titleStyle.Render("Incident Details"))
	sb.WriteString(" ")
	sb.WriteString(dimStyle.Render(fmt.Sprintf("(Line %d-%d of %d)", startLine+1, endLine, totalLines)))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render(strings.Repeat("─", 60)))
	sb.WriteString("\n")
	
	// Show scroll indicator at top if scrolled
	if m.incidentScrollY > 0 {
		sb.WriteString(dimStyle.Render("↑ More content above"))
		sb.WriteString("\n")
	}
	
	// Show visible portion
	for i := startLine; i < endLine; i++ {
		sb.WriteString(lines[i])
		sb.WriteString("\n")
	}
	
	// Scroll indicator at bottom
	if m.incidentScrollY < maxScroll {
		sb.WriteString(dimStyle.Render("↓ More content below"))
		sb.WriteString("\n")
	}
	
	sb.WriteString("\n")
	
	// Controls
	sb.WriteString(markdownBlock("## Controls"))
	if m.report.IsDemo {
		sb.WriteString("1    Details   2     Runbook       3   Postmortem\n")
		sb.WriteString("↑/↓  Scroll    PgUp/Dn Page         Esc Back\n")
	} else {
		sb.WriteString("↑/↓  Scroll    PgUp/Dn Page    Home/End Top/Bottom\n")
		sb.WriteString("Esc  Back      q       Exit\n")
	}
	
	return sb.String()
}

// buildIncidentDetailContent builds the full incident detail content
func (m *tuiModel) buildIncidentDetailContent() string {
	if m.selectedIncident == nil {
		return dimStyle.Render("No incident selected.")
	}

	// Handle Simulated Views in Demo Mode
	if m.report.IsDemo {
		switch m.viewMode {
		case viewRunbook:
			rb := demo.GetRunbook(m.selectedIncident.ID)
			if rb != nil {
				return rb.Content + "\n\n" + dimStyle.Render("(Press '1' to return to details)")
			}
		case viewPostmortem:
			pm := demo.GetPostmortem(m.selectedIncident.ID)
			if pm != nil {
				return pm.Content + "\n\n" + dimStyle.Render("(Press '1' to return to details)")
			}
		}
	}

	var sb strings.Builder
	
	inc := *m.selectedIncident
	
	// Basic info
	sb.WriteString(keyStyle.Render("ID: "))
	sb.WriteString(valueStyle.Render(inc.ID))
	sb.WriteString("\n")
	
	sb.WriteString(keyStyle.Render("Service: "))
	sb.WriteString(valueStyle.Render(inc.Service))
	sb.WriteString("\n")
	
	sb.WriteString(keyStyle.Render("Profile: "))
	sb.WriteString(valueStyle.Render(inc.Profile))
	sb.WriteString("\n")
	
	sb.WriteString(keyStyle.Render("Severity: "))
	sb.WriteString(getSeverityStyle(string(inc.Severity)).Render(string(inc.Severity)))
	sb.WriteString("\n")
	
	sb.WriteString(keyStyle.Render("Title: "))
	sb.WriteString(valueStyle.Render(inc.Title))
	sb.WriteString("\n")
	
	sb.WriteString(keyStyle.Render("State: "))
	sb.WriteString(getStateStyle(string(inc.State)).Render(string(inc.State)))
	sb.WriteString("\n")
	
	sb.WriteString(keyStyle.Render("Created: "))
	sb.WriteString(valueStyle.Render(inc.CreatedAt.Format("2006-01-02 15:04:05 MST")))
	sb.WriteString("\n")
	
	sb.WriteString(keyStyle.Render("Updated: "))
	sb.WriteString(valueStyle.Render(inc.UpdatedAt.Format("2006-01-02 15:04:05 MST")))
	sb.WriteString("\n")
	
	// Kubernetes Metadata
	if inc.Cluster != "" || inc.Namespace != "" || inc.Deployment != "" {
		sb.WriteString("\n")
		sb.WriteString(markdownBlock("## Kubernetes"))
		if inc.Cluster != "" {
			sb.WriteString(keyStyle.Render("Cluster:    "))
			sb.WriteString(valueStyle.Render(inc.Cluster))
			sb.WriteString("\n")
		}
		if inc.Namespace != "" {
			sb.WriteString(keyStyle.Render("Namespace:  "))
			sb.WriteString(valueStyle.Render(inc.Namespace))
			sb.WriteString("\n")
		}
		if inc.Deployment != "" {
			sb.WriteString(keyStyle.Render("Deployment: "))
			sb.WriteString(valueStyle.Render(inc.Deployment))
			sb.WriteString("\n")
		}
		if inc.K8sContext != "" {
			sb.WriteString(keyStyle.Render("Context:    "))
			sb.WriteString(valueStyle.Render(inc.K8sContext))
			sb.WriteString("\n")
		}
	}
	
	if strings.TrimSpace(inc.Summary) != "" {
		sb.WriteString(keyStyle.Render("Summary: "))
		sb.WriteString(valueStyle.Render(strings.TrimSpace(inc.Summary)))
		sb.WriteString("\n")
	}
	
	// Impacted Flows
	sb.WriteString("\n")
	sb.WriteString(markdownBlock("## Impacted Flows"))
	if inc.Impact != nil {
		if len(inc.Impact.ImpactedFlows) > 0 {
			for _, flowName := range inc.Impact.ImpactedFlows {
				sb.WriteString("• " + flowName + "\n")
			}
		}
		if inc.Impact.CustomMetrics != "" {
			sb.WriteString(dimStyle.Render("Metrics: " + inc.Impact.CustomMetrics))
			sb.WriteString("\n")
		}
		if inc.Impact.EstimatedDowntimeMinutes > 0 {
			sb.WriteString(dimStyle.Render(fmt.Sprintf("Est. Downtime: %d min", inc.Impact.EstimatedDowntimeMinutes)))
			sb.WriteString("\n")
		}
	} else {
		// Fallback to dynamic lookup if no stored impact
		impactedFlows := getImpactedFlows(inc.Service)
		if len(impactedFlows) > 0 {
			for _, flowName := range impactedFlows {
				sb.WriteString("• " + flowName + "\n")
			}
		} else {
			sb.WriteString(dimStyle.Render("None"))
			sb.WriteString("\n")
		}
	}
	
	// Observability Links
	if hasObservabilityLinks(inc.Links) {
		sb.WriteString("\n")
		sb.WriteString(markdownBlock("## Observability"))
		if inc.Links.Grafana != "" {
			sb.WriteString(keyStyle.Render("Grafana:    "))
			sb.WriteString(dimStyle.Render(truncateString(inc.Links.Grafana, 50)))
			sb.WriteString("\n")
		}
		if inc.Links.Prometheus != "" {
			sb.WriteString(keyStyle.Render("Prometheus: "))
			sb.WriteString(dimStyle.Render(truncateString(inc.Links.Prometheus, 50)))
			sb.WriteString("\n")
		}
		if inc.Links.Logs != "" {
			sb.WriteString(keyStyle.Render("Logs:       "))
			sb.WriteString(dimStyle.Render(truncateString(inc.Links.Logs, 50)))
			sb.WriteString("\n")
		}
		if inc.Links.Traces != "" {
			sb.WriteString(keyStyle.Render("Traces:     "))
			sb.WriteString(dimStyle.Render(truncateString(inc.Links.Traces, 50)))
			sb.WriteString("\n")
		}
	}
	
	// Logs Section
	if inc.Logs != nil && len(inc.Logs.TopErrors) > 0 {
		sb.WriteString("\n")
		sb.WriteString(markdownBlock("## Logs"))
		sb.WriteString(dimStyle.Render(fmt.Sprintf("Backend: %s", inc.Logs.Backend)))
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render(fmt.Sprintf("Window: %s", inc.Logs.Window)))
		sb.WriteString("\n")
		sb.WriteString(keyStyle.Render("Top errors:"))
		sb.WriteString("\n")
		for _, err := range inc.Logs.TopErrors {
			sb.WriteString(fmt.Sprintf("  [%dx] %s\n", err.Count, err.Message))
		}
	}
	
	// Traces Section
	if inc.Traces != nil && inc.Traces.TraceCount > 0 {
		sb.WriteString("\n")
		sb.WriteString(markdownBlock("## Traces"))
		sb.WriteString(dimStyle.Render(fmt.Sprintf("Backend: %s", inc.Traces.Backend)))
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render(fmt.Sprintf("Window: %s", inc.Traces.Window)))
		sb.WriteString("\n")
		
		if inc.Traces.SlowestRoute != "" {
			sb.WriteString(keyStyle.Render("Slowest route: "))
			sb.WriteString(valueStyle.Render(inc.Traces.SlowestRoute))
			sb.WriteString("\n")
		}
		
		if inc.Traces.P95LatencyMs > 0 {
			sb.WriteString(keyStyle.Render("P95 latency: "))
			sb.WriteString(valueStyle.Render(fmt.Sprintf("%.0fms", inc.Traces.P95LatencyMs)))
			sb.WriteString("\n")
		}
		
		if inc.Traces.ErrorCount > 0 {
			sb.WriteString(keyStyle.Render("Errors: "))
			sb.WriteString(valueStyle.Render(fmt.Sprintf("%d", inc.Traces.ErrorCount)))
			sb.WriteString("\n")
		}
		
		if len(inc.Traces.TopSpans) > 0 {
			sb.WriteString(keyStyle.Render("Top spans:"))
			sb.WriteString("\n")
			for _, span := range inc.Traces.TopSpans {
				sb.WriteString(fmt.Sprintf("  • %s\n", span.Name))
			}
		}
	}

	// Runbook Section
	runbookURL := "https://example.com/runbook/" + inc.Service
	sb.WriteString("\n")
	sb.WriteString(markdownBlock("## Runbook"))
	sb.WriteString("  " + runbookURL + "\n")

	// Runbook Suggestions Section
	if suggestion, ok := inc.Metadata["runbook_suggestion"]; ok {
		sb.WriteString("\n")
		sb.WriteString(markdownBlock("## Runbook Suggestions"))
		if pattern, ok := inc.Metadata["runbook_pattern"]; ok {
			sb.WriteString(keyStyle.Render("Pattern:    "))
			sb.WriteString(valueStyle.Render(pattern))
			sb.WriteString("\n")
		}
		sb.WriteString(keyStyle.Render("Suggested:  "))
		sb.WriteString(valueStyle.Render(suggestion))
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render(fmt.Sprintf("\nTip: Use 'health-monitor runbook suggest %s' for details", inc.ID)))
		sb.WriteString("\n")
	}

	// Timeline
	sb.WriteString("\n")
	sb.WriteString(markdownBlock("## Timeline"))
	for _, event := range inc.Events {
		sb.WriteString(formatTimelineEvent(event))
	}

	// Analysis (RCA) Section
	if inc.Analysis != nil {
		sb.WriteString("\n")
		sb.WriteString(markdownBlock("## Root Cause Analysis (RCA)"))
		if inc.Analysis.Component != "" {
			sb.WriteString(keyStyle.Render("Component: "))
			sb.WriteString(inc.Analysis.Component)
			sb.WriteString("\n")
		}
		if inc.Analysis.Category != "" {
			sb.WriteString(keyStyle.Render("Category: "))
			sb.WriteString(inc.Analysis.Category)
			sb.WriteString("\n")
		}
		if inc.Analysis.RootCause != "" {
			sb.WriteString(keyStyle.Render("Root Cause: "))
			sb.WriteString(inc.Analysis.RootCause)
			sb.WriteString("\n")
		}
		if inc.Analysis.FixSummary != "" {
			sb.WriteString(keyStyle.Render("Fix Summary: "))
			sb.WriteString(inc.Analysis.FixSummary)
			sb.WriteString("\n")
		}
		if inc.Analysis.Prevention != "" {
			sb.WriteString(keyStyle.Render("Prevention: "))
			sb.WriteString(inc.Analysis.Prevention)
			sb.WriteString("\n")
		}
	}

	// Similar Incidents Section
	demoSimilar := incident.GetDemoSimilarIncidents(inc)
	if len(demoSimilar) > 0 {
		sb.WriteString("\n")
		sb.WriteString(markdownBlock("## Similar Incidents"))
		for i, sim := range demoSimilar {
			sb.WriteString(fmt.Sprintf("%d. %s - %s (%.0f%% similar)\n", 
				i+1, sim.Incident.ID, sim.Incident.Title, sim.Confidence*100))
			sb.WriteString(dimStyle.Render(fmt.Sprintf("   Service: %s | State: %s | %s\n", 
				sim.Incident.Service, sim.Incident.State, sim.Age)))
			if i < len(demoSimilar)-1 {
				sb.WriteString("\n")
			}
		}
	} else if m.incidentList != nil {
		similar := m.findSimilarIncidents(inc)
		if len(similar) > 0 {
			sb.WriteString("\n")
			sb.WriteString(markdownBlock("## Similar Incidents"))
			for _, sim := range similar {
				sb.WriteString(fmt.Sprintf("• %s: %s [%s]\n", sim.ID, truncateString(sim.Title, 40), string(sim.State)))
			}
		}
	}

	// Action Items Section
	if len(inc.ActionItems) > 0 {
		sb.WriteString("\n")
		sb.WriteString(markdownBlock("## Action Items"))
		for i, item := range inc.ActionItems {
			// Numered list matching CLI
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, item.Description))
			sb.WriteString(dimStyle.Render(fmt.Sprintf("   Owner:    %s", item.Owner)) + "\n")
			sb.WriteString(dimStyle.Render(fmt.Sprintf("   Status:   %s", item.Status)) + "\n")
			sb.WriteString(dimStyle.Render(fmt.Sprintf("   Priority: %s", item.Priority)) + "\n")
			if i < len(inc.ActionItems)-1 {
				sb.WriteString("\n")
			}
		}
	}

	// Lessons Learned (moved to end for parity)
	if inc.Analysis != nil && inc.Analysis.LessonsLearned != nil {
		sb.WriteString("\n")
		sb.WriteString(markdownBlock("## Lessons Learned"))
		ll := inc.Analysis.LessonsLearned
		if ll.WhatWentWell != "" {
			sb.WriteString(keyStyle.Render("What Went Well:       "))
			sb.WriteString(ll.WhatWentWell)
			sb.WriteString("\n")
		}
		if ll.WhatCouldBeBetter != "" {
			sb.WriteString(keyStyle.Render("What Could Be Better:  "))
			sb.WriteString(ll.WhatCouldBeBetter)
			sb.WriteString("\n")
		}
		if ll.WhereWeGotLucky != "" {
			sb.WriteString(keyStyle.Render("Where We Got Lucky:    "))
			sb.WriteString(ll.WhereWeGotLucky)
			sb.WriteString("\n")
		}
	}
	
	return sb.String()
}

// Helper functions

func getSeverityStyle(severity string) lipgloss.Style {
	switch strings.TrimSpace(severity) {
	case "P1":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	case "P2":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true)
	case "P3":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("4")).Bold(true)
	case "P4":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)
	default:
		return lipgloss.NewStyle()
	}
}

func getStateStyle(state string) lipgloss.Style {
	switch strings.TrimSpace(state) {
	case "Started":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	case "Acknowledged":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true)
	case "Resolved":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)
	case "Suggested":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)
	default:
		return lipgloss.NewStyle()
	}
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func hasObservabilityLinks(links incident.ObservabilityLinks) bool {
	return links.Grafana != "" || links.Prometheus != "" || links.Logs != "" || links.Traces != ""
}

func getImpactedFlows(serviceName string) []string {
	result, err := flow.LoadOnce()
	if err != nil {
		return nil
	}
	if len(result.Flows) == 0 {
		return nil
	}
	catalog := flow.NewCatalog(result.Flows)
	impacted := catalog.ImpactedFlows(serviceName)
	names := make([]string, 0, len(impacted))
	for _, f := range impacted {
		names = append(names, f.DisplayName())
	}
	return names
}

func formatTimelineEvent(event incident.Event) string {
	timeStr := event.Timestamp.Format("15:04")
	desc := incident.EventDescription(event)
	
	user := strings.TrimSpace(event.User)
	if user == "" {
		user = "unknown"
	}
	
	return fmt.Sprintf("[%s] %s - %s\n", timeStr, user, desc)
}

// handleIncidentBrowserKeys handles keyboard input for the incident browser
func (m *tuiModel) handleIncidentBrowserKeys(key string) bool {
	// If in list view
	if m.incidentBrowserMode {
		switch key {
		case "up", "k":
			if m.incidentSelectedIndex > 0 {
				m.incidentSelectedIndex--
				// Adjust page if needed
				pageSize := m.incidentPageSize
				if pageSize <= 0 {
					pageSize = 10
				}
				if m.incidentSelectedIndex < m.incidentCurrentPage*pageSize {
					m.incidentCurrentPage--
				}
			}
			return true
		case "down", "j":
			if m.incidentSelectedIndex < len(m.filteredIncidentList)-1 {
				m.incidentSelectedIndex++
				// Adjust page if needed
				pageSize := m.incidentPageSize
				if pageSize <= 0 {
					pageSize = 10
				}
				if m.incidentSelectedIndex >= (m.incidentCurrentPage+1)*pageSize {
					m.incidentCurrentPage++
				}
			}
			return true
		case "pgup", "pageup":
			if m.incidentCurrentPage > 0 {
				m.incidentCurrentPage--
				pageSize := m.incidentPageSize
				if pageSize <= 0 {
					pageSize = 10
				}
				m.incidentSelectedIndex = m.incidentCurrentPage * pageSize
			}
			return true
		case "pgdown", "pagedown":
			pageSize := m.incidentPageSize
			if pageSize <= 0 {
				pageSize = 10
			}
			maxPage := (len(m.filteredIncidentList) - 1) / pageSize
			if m.incidentCurrentPage < maxPage {
				m.incidentCurrentPage++
				m.incidentSelectedIndex = m.incidentCurrentPage * pageSize
				if m.incidentSelectedIndex >= len(m.filteredIncidentList) {
					m.incidentSelectedIndex = len(m.filteredIncidentList) - 1
				}
			}
			return true
		case "home", "g":
			m.incidentSelectedIndex = 0
			m.incidentCurrentPage = 0
			return true
		case "end", "G":
			if len(m.filteredIncidentList) > 0 {
				m.incidentSelectedIndex = len(m.filteredIncidentList) - 1
				pageSize := m.incidentPageSize
				if pageSize <= 0 {
					pageSize = 10
				}
				m.incidentCurrentPage = m.incidentSelectedIndex / pageSize
			}
			return true
		case "enter":
			// Enter detail view
			if m.incidentSelectedIndex >= 0 && m.incidentSelectedIndex < len(m.filteredIncidentList) {
				m.selectedIncident = &m.filteredIncidentList[m.incidentSelectedIndex]
				m.incidentBrowserMode = false
				m.incidentScrollY = 0
			}
			return true
		case "1":
			m.incidentFilterSeverity = "P1"
			m.applyIncidentFilters()
			return true
		case "2":
			m.incidentFilterSeverity = "P2"
			m.applyIncidentFilters()
			return true
		case "3":
			m.incidentFilterSeverity = "P3"
			m.applyIncidentFilters()
			return true
		case "4":
			m.incidentFilterSeverity = "P4"
			m.applyIncidentFilters()
			return true
		case "a", "A":
			m.incidentFilterState = "active"
			m.applyIncidentFilters()
			return true
		case "r", "R":
			m.incidentFilterState = "resolved"
			m.applyIncidentFilters()
			return true
		case "x", "X":
			// Clear all filters
			m.incidentFilterService = ""
			m.incidentFilterSeverity = "all"
			m.incidentFilterState = "all"
			m.applyIncidentFilters()
			return true
		}
	} else {
		// Detail view navigation
		switch key {
		case "up", "k":
			if m.incidentScrollY > 0 {
				m.incidentScrollY--
			}
			return true
		case "down", "j":
			m.incidentScrollY++
			return true
		case "pgup", "pageup":
			m.incidentScrollY -= 10
			if m.incidentScrollY < 0 {
				m.incidentScrollY = 0
			}
			return true
		case "pgdown", "pagedown":
			m.incidentScrollY += 10
			return true
		case "home", "g":
			m.incidentScrollY = 0
			return true
		case "end", "G":
			m.incidentScrollY = 1000 // Large number to scroll to bottom
			return true
		case "e", "E":
			if m.selectedIncident != nil {
				service, err := incident.NewService(nil)
				if err == nil {
					path, err := service.GeneratePostmortem(m.selectedIncident.ID)
					if err != nil {
						m.incidentMessage = "Export failed: " + err.Error()
					} else {
						m.incidentMessage = "Postmortem exported to: " + path
					}
				} else {
					m.incidentMessage = "Export failed: " + err.Error()
				}
			}
			return true
		case "esc":
			// Return to list view
			m.viewMode = viewDetails
			m.incidentBrowserMode = true
			return true
		case "1":
			if m.report.IsDemo {
				m.viewMode = viewDetails
				m.incidentScrollY = 0
				return true
			}
		case "2":
			if m.report.IsDemo {
				m.viewMode = viewRunbook
				m.incidentScrollY = 0
				return true
			}
		case "3":
			if m.report.IsDemo {
				m.viewMode = viewPostmortem
				m.incidentScrollY = 0
				return true
			}
		}
	}
	return false
}

// findSimilarIncidents finds historical incidents similar to the current one
func (m *tuiModel) findSimilarIncidents(current incident.Incident) []incident.Incident {
	var similar []incident.Incident
	for _, inc := range m.incidentList {
		if inc.ID == current.ID {
			continue // Skip self
		}
		// Match on same service, or same component if Analysis exists
		isSimilar := false
		if inc.Service != "" && inc.Service == current.Service {
			isSimilar = true
		} else if current.Analysis != nil && inc.Analysis != nil && current.Analysis.Component == inc.Analysis.Component {
			isSimilar = true
		}
		
		if isSimilar {
			similar = append(similar, inc)
		}
	}
	// Cap to latest 3
	if len(similar) > 3 {
		similar = similar[:3]
	}
	return similar
}
