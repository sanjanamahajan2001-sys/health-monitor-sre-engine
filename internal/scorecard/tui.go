package scorecard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"health-monitor/internal/config"
	"health-monitor/internal/incident"
	"health-monitor/internal/slo"
	"health-monitor/internal/flow"
	"health-monitor/internal/metrics"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/glamour"
)

type state int

const (
	stateSelectProfile state = iota
	stateSelectService
	stateViewReport
	stateOrgReport
)

type scorecardModel struct {
	state       state
	list        list.Model
	viewport    viewport.Model
	provider    *Provider
	report      *MonthlyReport
	width       int
	height      int
	ready       bool
	quitting    bool
	statusMsg   string
	
	// Navigation context
	selectedProfile string
	availableServices []string
	orgReport       *OrgReport
	envFilter       string
	viewYear        int
	viewMonth       time.Month
}

type item struct {
	title, desc string
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string { return i.title }

var (
	titleStyle = lipgloss.NewStyle().MarginLeft(2).Bold(true).Foreground(lipgloss.Color("6"))
	docStyle   = lipgloss.NewStyle().Margin(1, 2)
	statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)
)

func RunTUI(initialProfile string, orgMode bool, envFilter string) error {
	pm := config.GetProfileManager()
	allProfiles := pm.DiscoverProfiles()
	
	var profiles []string
	for _, p := range allProfiles {
		pLower := strings.ToLower(p)
		if strings.Contains(pLower, "security") || strings.Contains(pLower, "config") {
			continue
		}
		profiles = append(profiles, p)
	}

	items := make([]list.Item, len(profiles))
	for i, p := range profiles {
		items[i] = item{title: p, desc: "Configuration profile"}
	}

	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Select Profile for Reliability Scorecard"

	// Init Scorecard Provider
	is, _ := incident.NewStore()
	// TUI will handle re-initialization per profile, use nil/empty for initial state
	ss, _ := slo.NewService(nil) 
	provider := NewProvider(is, ss)

	m := scorecardModel{
		state:     stateSelectProfile,
		list:      l,
		provider:  provider,
		envFilter: envFilter,
		viewYear:  time.Now().Year(),
		viewMonth: time.Now().Month(),
	}

	// 🚀 NEW: Handle initial profile selection
	if initialProfile != "" {
		m.selectedProfile = initialProfile
		pm.SetActiveProfile(initialProfile)
		
		// Discover services for this profile
		loadRes, _ := flow.LoadProfileAware()
		svcMap := make(map[string]bool)
		for _, f := range loadRes.Flows {
			for _, s := range f.Services {
				svcMap[s] = true
			}
		}
		
		m.availableServices = []string{"[Complete Profile View]"}
		for s := range svcMap {
			m.availableServices = append(m.availableServices, s)
		}
		
		// Update list for service selection
		nextItems := make([]list.Item, len(m.availableServices))
		for idx, s := range m.availableServices {
			desc := "Service Scoreboard"
			if s == "[Complete Profile View]" { desc = "Aggregated team-level view" }
			nextItems[idx] = item{title: s, desc: desc}
		}
		m.list.SetItems(nextItems)
		m.list.Title = "Select Service for " + initialProfile
		m.state = stateSelectService
	}

	if orgMode {
		report, err := m.provider.GenerateOrgReport(m.viewYear, m.viewMonth, envFilter)
		if err == nil {
			m.orgReport = report
			m.state = stateOrgReport
			// We'll set the viewport content in m.Update when m.ready is true
		}
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("failed to run scorecard TUI: %w", err)
	}
	return nil
}

func (m scorecardModel) Init() tea.Cmd {
	return nil
}

func (m scorecardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		
		case "b":
			if m.state == stateViewReport {
				m.state = stateSelectService
				return m, nil
			} else if m.state == stateSelectService {
				m.state = stateSelectProfile
				m.list.Title = "Select Profile for Reliability Scorecard"
				return m, nil
			} else if m.state == stateOrgReport {
				m.state = stateSelectProfile
				return m, nil
			}

		case "p": // Previous Month
			if m.state == stateViewReport || m.state == stateOrgReport {
				target := time.Date(m.viewYear, m.viewMonth, 1, 0, 0, 0, 0, time.UTC).AddDate(0, -1, 0)
				m.viewYear = target.Year()
				m.viewMonth = target.Month()
				return m.reloadData()
			}

		case "n": // Next/Current Month
			if m.state == stateViewReport || m.state == stateOrgReport {
				target := time.Date(m.viewYear, m.viewMonth, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0)
				// Limit to current month
				now := time.Now()
				if target.After(now) {
					return m, nil
				}
				m.viewYear = target.Year()
				m.viewMonth = target.Month()
				return m.reloadData()
			}

		case "d":
			if (m.state == stateViewReport && m.report != nil) || (m.state == stateOrgReport && m.orgReport != nil) {
				filename, err := m.downloadReport()
				if err != nil {
					m.statusMsg = "Error saving report: " + err.Error()
				} else {
					m.statusMsg = "Report saved to " + filename
				}
				return m, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
					return clearMessage{}
				})
			}

		case "enter":
			if m.state == stateSelectProfile {
				i, ok := m.list.SelectedItem().(item)
				if ok {
					m.selectedProfile = i.title
					pm := config.GetProfileManager()
					pm.SetActiveProfile(i.title)
					
					// Discover services for this profile
					loadRes, _ := flow.LoadProfileAware()
					svcMap := make(map[string]bool)
					for _, f := range loadRes.Flows {
						for _, s := range f.Services {
							svcMap[s] = true
						}
					}
					
					m.availableServices = []string{"[Complete Profile View]"}
					for s := range svcMap {
						m.availableServices = append(m.availableServices, s)
					}
					
					// Update list for service selection
					items := make([]list.Item, len(m.availableServices))
					for idx, s := range m.availableServices {
						desc := "Service Scoreboard"
						if s == "[Complete Profile View]" { desc = "Aggregated team-level view" }
						items[idx] = item{title: s, desc: desc}
					}
					m.list.SetItems(items)
					m.list.Title = "Select Service for " + i.title
					m.state = stateSelectService
					return m, nil
				}
			} else if m.state == stateSelectService {
				i, ok := m.list.SelectedItem().(item)
				if ok {
					targetSvc := ""
					if i.title != "[Complete Profile View]" {
						targetSvc = i.title
					}
					
					// Load profile data
					is, _ := incident.NewStore()
					cfg, _ := config.LoadForProfile(m.selectedProfile)
					
					// Create metrics provider
					mp, _ := metrics.NewMetricProvider(&cfg)
					ss, _ := slo.NewService(mp)
					
					p := NewProvider(is, ss)
					p.TargetService = targetSvc
					m.provider = p

					report, err := m.provider.GenerateReport(m.selectedProfile, m.viewYear, m.viewMonth)
					if err == nil {
						m.report = report
						m.state = stateViewReport
						m.viewport.SetContent(m.renderReport())
					}
				}
			} else if m.state == stateOrgReport {
				// Drill down if profile selected? 
				// For now, let's just allow q/b to navigate
			}
		}

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.list.SetSize(msg.Width, msg.Height)
		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-4)
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - 4
		}
		
		if m.state == stateOrgReport && m.orgReport != nil {
			m.viewport.SetContent(m.renderOrgReport())
		} else if m.state == stateViewReport && m.report != nil {
			m.viewport.SetContent(m.renderReport())
		}

	case clearMessage:
		m.statusMsg = ""
	}

	if m.state == stateSelectProfile || m.state == stateSelectService {
		m.list, cmd = m.list.Update(msg)
		cmds = append(cmds, cmd)
	} else {
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m scorecardModel) View() string {
	if m.quitting {
		return ""
	}

	if m.state == stateSelectProfile || m.state == stateSelectService {
		return docStyle.Render(m.list.View())
	}

	title := ""
	isHistorical := m.viewYear != time.Now().Year() || m.viewMonth != time.Now().Month()
	histTag := ""
	if isHistorical {
		histTag = " (HISTORICAL VIEW)"
	}

	if m.state == stateViewReport && m.report != nil {
		title = fmt.Sprintf("Monthly Scoreboard: %s%s", m.report.Month, histTag)
	} else if m.state == stateOrgReport && m.orgReport != nil {
		title = fmt.Sprintf("Organization Scoreboard: %s%s", m.orgReport.Month, histTag)
	}

	header := titleStyle.Render("HEALTH-MONITORING AGENT\n" + title)
	
	footerText := "[q] Quit | [b] Back | [d] Download Report"
	if m.state == stateViewReport || m.state == stateOrgReport {
		footerText += " | [p] Prev Month | [n] Next Month"
	}
	footer := fmt.Sprintf("\n%s\n%s", m.statusMsg, footerText)
	
	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		m.viewport.View(),
		footer,
	)
}

func (m scorecardModel) renderReport() string {
	md := m.generateMarkdown()
	// Use viewport width for word wrap, leaving small margin
	width := m.width
	if width > 120 {
		width = 120 // Max reading width
	}
	r, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width-4),
	)
	out, _ := r.Render(md)
	return out
}

func (m scorecardModel) generateMarkdown() string {
	var sb strings.Builder
	sb.WriteString("# HEALTH-MONITORING AGENT\n")
	sb.WriteString(fmt.Sprintf("## Monthly Scoreboard: %s\n\n", m.report.Month))
	sb.WriteString(fmt.Sprintf("**Profile/Team**: %s\n", m.report.Profile))

	if m.report.NotAvailableReason != "" {
		sb.WriteString("\n> [!NOTE]\n")
		sb.WriteString(fmt.Sprintf("> **Information**: %s\n\n", m.report.NotAvailableReason))
		sb.WriteString("Historical data collection for this entity started recently. Dashboard metrics will populate as more data is gathered.\n")
		return sb.String()
	}
	
	scoreTrend := "→"
	if m.report.Trends != nil {
		if m.report.Trends.ScoreDelta > 0 { scoreTrend = fmt.Sprintf(" (↑ %.1f)", m.report.Trends.ScoreDelta) }
		if m.report.Trends.ScoreDelta < 0 { scoreTrend = fmt.Sprintf(" (↓ %.1f)", m.report.Trends.ScoreDelta) }
	}
	sb.WriteString(fmt.Sprintf("**Overall Health Score**: %.1f%%%s (%s)\n", m.report.OverallScore, scoreTrend, m.report.HealthStatus))
	
	if m.report.Ranking != nil {
		rankTrend := "→"
		if m.report.Ranking.Trend == "up" { rankTrend = "↑" }
		if m.report.Ranking.Trend == "down" { rankTrend = "↓" }
		sb.WriteString(fmt.Sprintf("**Team Ranking**: %d/%d (%s)\n", m.report.Ranking.Position, m.report.Ranking.TotalTeams, rankTrend))
	}
	sb.WriteString("\n")

	if m.report.Toil != nil {
		sb.WriteString("## 0. Toil & Capacity Metrics ✨ NEW\n\n")
		
		trendIcon := "→"
		if m.report.Toil.Trend == "Improving" { trendIcon = "↓ (Improving)" }
		if m.report.Toil.Trend == "Degrading" { trendIcon = "↑ (Degrading)" }
		
		sb.WriteString(fmt.Sprintf("- **Total Toil**: %.1f hours (Est. Cost: $%.2f) %s\n", m.report.Toil.TotalHours, m.report.Toil.CostUSD, trendIcon))
		if m.report.Toil.MonthlySavingsUSD > 0 {
			sb.WriteString(fmt.Sprintf("- **Potential Monthly Savings**: $%.2f\n", m.report.Toil.MonthlySavingsUSD))
		}
		
		status := "✅ HEALTHY"
		if m.report.Toil.Status == "Over Target" {
			status = "🚨 OVER TARGET"
		}
		sb.WriteString(fmt.Sprintf("- **Toil Percentage**: %.2f%% (target <%.0f%%) - %s\n\n", 
			m.report.Toil.PercentageOfTime, m.report.Toil.TargetPercentage, status))
	}
	
	sb.WriteString("## 1. Service Reliability Summary\n\n")
	sb.WriteString("| Service | Flow | Avail% | P1/P2 | MTTR | MTBF | Trend | %Done |\n")
	sb.WriteString("| :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- |\n")
	for _, s := range m.report.ServiceSummaries {
		trend := "→"
		if s.Trend == "Improving" { trend = "↑" }
		if s.Trend == "Degrading" { trend = "↓" }
		
		sb.WriteString(fmt.Sprintf("| %s | %s | %.2f%% | %d/%d | %s | %s | %s | %s |\n",
			s.Service, s.Flow, s.Availability, s.P1Incidents, s.P2Incidents, s.MTTR, s.MTBF, trend, s.ActionItemPerf))
	}
	sb.WriteString("\n")

	sb.WriteString("## 2. Error Budget Summary\n\n")
	sb.WriteString("| Service | Goal | Actual | Rem (%) | Status |\n")
	sb.WriteString("| :--- | :--- | :--- | :--- | :--- |\n")
	for _, b := range m.report.ErrorBudgets {
		status := b.Status
		if status == "COMPLIANT" || status == "SAFE" {
			status = "✅ OK"
		} else if status == "BREACHING" {
			status = "❌ BREACH"
		} else if b.Actual == 0 && (status == "ERROR" || status == "") {
			status = "🚨 QUERY FAIL"
		}
		sb.WriteString(fmt.Sprintf("| %s | %.1f%% | %.1f%% | %.1f%% | %s |\n",
			b.Service, b.TargetSLO, b.Actual, b.BudgetRemaining, status))
	}
	sb.WriteString("\n")

	sb.WriteString("## 3. Incident Distribution by Category\n\n")
	sb.WriteString("| Category | Count | % of Total |\n")
	sb.WriteString("| :--- | :--- | :--- |\n")
	for _, c := range m.report.CategoryStats {
		sb.WriteString(fmt.Sprintf("| %s | %d | %.1f%% |\n", c.Category, c.Count, c.Percent))
	}
	sb.WriteString("\n")

	sb.WriteString("## 4. ML Agent Insights\n\n")
	sb.WriteString(fmt.Sprintf("- **Top Feature**: %s\n", m.report.MLInsights.TopSemanticFeature))
	sb.WriteString(fmt.Sprintf("- **Top Pattern**: %s\n", m.report.MLInsights.MostFrequentPattern))
	sb.WriteString(fmt.Sprintf("- **Avg Conf**: %.1f%%\n", m.report.MLInsights.AvgConfidence))
	sb.WriteString(fmt.Sprintf("- **Actionable Hint Rate**: %.1f%%\n", m.report.MLInsights.ActionableHintRate))
	sb.WriteString("\n")

	sb.WriteString("## 5. Team-level Reliability Scorecard with Trends\n\n")
	
	// Aggregated Metrics for Summary
	sloCompliance := m.report.OverallScore // Using OverallScore as proxy for compliance if not specified
	sloDeltaStr := "stable"
	if m.report.Trends != nil {
		delta := m.report.Trends.SLOComplianceDelta * 100
		if delta > 0 { sloDeltaStr = fmt.Sprintf("up %.1f%% MoM", delta) }
		if delta < 0 { sloDeltaStr = fmt.Sprintf("down %.1f%% MoM", -delta) }
	}
	sb.WriteString(fmt.Sprintf("- **SLO Compliance**: %.1f%% (%s)\n", sloCompliance, sloDeltaStr))

	p1Count := 0
	p2Count := 0
	
	// Global MTTR calculation for summary
	var totalDur time.Duration
	var resolvedCount int
	for _, s := range m.report.ServiceSummaries {
		p1Count += s.P1Incidents
		p2Count += s.P2Incidents
		
		// Parse MTTR string (e.g. "15m") back to duration if possible
		if s.MTTR != "" && s.MTTR != "-" {
			d, err := time.ParseDuration(s.MTTR)
			if err == nil {
				totalDur += d
				resolvedCount++
			}
		}
	}
	
	avgMTTR := "-"
	if resolvedCount > 0 {
		avg := totalDur / time.Duration(resolvedCount)
		avgMTTR = avg.Round(time.Minute).String()
	}
	sb.WriteString(fmt.Sprintf("- **Incidents**: %d P1s, %d P2s (MTTR %s)\n", p1Count, p2Count, avgMTTR))

	if m.report.Toil != nil {
		toilDelta := "stable"
		if m.report.Toil.Trend == "Improving" { toilDelta = "improving" }
		if m.report.Toil.Trend == "Degrading" { toilDelta = "degrading" }
		
		sb.WriteString(fmt.Sprintf("- **Toil**: %.1f%% of time (%s) - Target <%.0f%%\n", 
			m.report.Toil.PercentageOfTime, toilDelta, m.report.Toil.TargetPercentage))
		if m.report.Toil.MonthlySavingsUSD > 0 {
			sb.WriteString(fmt.Sprintf("- **Recoverable Capacity**: $%.2f/mo\n", m.report.Toil.MonthlySavingsUSD))
		}
	}

	openActions := 0
	overdueActions := 0
	for _, ai := range m.report.NextActionItems {
		if ai.Status == "Pending" || ai.Status == "Overdue" {
			openActions++
		}
		if ai.Status == "Overdue" {
			overdueActions++
		}
	}
	sb.WriteString(fmt.Sprintf("- **Action Items**: %d open (%d overdue)\n", openActions, overdueActions))

	scoreIcon := "→"
	if m.report.Trends != nil {
		// Use a small epsilon for float comparison to avoid jitter at 100%
		if m.report.Trends.ScoreDelta > 0.05 { scoreIcon = "↑" }
		if m.report.Trends.ScoreDelta < -0.05 { scoreIcon = "↓" }
	}
	rankStr := ""
	if m.report.Ranking != nil {
		rankStr = fmt.Sprintf(" ← %d/%d teams", m.report.Ranking.Position, m.report.Ranking.TotalTeams)
	}
	
	healthLabel := ""
	if m.report.HealthStatus != "" {
		healthLabel = fmt.Sprintf(" (%s)", m.report.HealthStatus)
	}
	
	sb.WriteString(fmt.Sprintf("- **Score**: %.0f/100 (%s)%s%s\n\n", m.report.OverallScore, scoreIcon, rankStr, healthLabel))

	sb.WriteString("## 6. Executive Summary\n\n")
	sb.WriteString(fmt.Sprintf("- **Reliability Trend**: %s\n", m.report.ExecutiveSummary.ReliabilityTrend))
	sb.WriteString(fmt.Sprintf("- **Primary Risk**: %s\n", m.report.ExecutiveSummary.PrimaryRisk))
	sb.WriteString(fmt.Sprintf("- **Frequent Pattern**: %s\n", m.report.ExecutiveSummary.FrequentPattern))
	sb.WriteString(fmt.Sprintf("- **Focus Area**: %s\n", m.report.ExecutiveSummary.FocusArea))
	
	// Add recommended runbook if service with primary risk is known
	for _, s := range m.report.ServiceSummaries {
		if s.Service == m.report.ExecutiveSummary.PrimaryRisk {
			sb.WriteString(fmt.Sprintf("- **Recommended Runbook**: %s\n", s.RecommendedRunbook))
			break
		}
	}
	sb.WriteString("\n")
	sb.WriteString("> [!IMPORTANT]\n")
	sb.WriteString("> **Health Note**: 'At Risk' is triggered if Overall Score < 95% OR any P1 incidents exist. 'Critical' is triggered if Score < 80%.\n")
	sb.WriteString("\n")

	// Service-Specific Deep Dive
	if m.report.ServiceSpecific != nil {
		s := m.report.ServiceSpecific
		sb.WriteString(fmt.Sprintf("## 7. Service Deep Dive: %s\n\n", s.ServiceName))
		
		sb.WriteString("### Service Reliability Highlights\n")
		sb.WriteString("| Metric | Value | Status |\n")
		sb.WriteString("| :--- | :--- | :--- |\n")
		
		availStatus := "✅ OK"
		if s.Availability < 99.9 { availStatus = "⚠️ WARNING" }
		if s.Availability < 99.0 { availStatus = "🔴 BREACH" }
		
		sb.WriteString(fmt.Sprintf("| Availability | %.2f%% | %s |\n", s.Availability, availStatus))
		sb.WriteString(fmt.Sprintf("| MTTR | %s | %s |\n", s.MTTR, "→"))
		sb.WriteString(fmt.Sprintf("| MTBF | %s | %s |\n", s.MTBF, "→"))
		
		sloStatus := "✅ COMPLIANT"
		if len(s.UpcomingSLORisk) > 0 { sloStatus = "🔴 AT RISK" }
		sb.WriteString(fmt.Sprintf("| SLO Status | %s | %s |\n", sloStatus, "→"))
		
		// Find recommended runbook for this service
		rec := "standard_ops"
		for _, svc := range m.report.ServiceSummaries {
			if svc.Service == s.ServiceName {
				rec = svc.RecommendedRunbook
				break
			}
		}
		sb.WriteString(fmt.Sprintf("| Runbook | %s | %s |\n", rec, "→"))
		
		if len(s.UpstreamDeps) > 0 {
			sb.WriteString(fmt.Sprintf("| Upstream | %s | %s |\n", strings.Join(s.UpstreamDeps, ", "), "→"))
		}
		if len(s.DownstreamDeps) > 0 {
			sb.WriteString(fmt.Sprintf("| Downstream | %s | %s |\n", strings.Join(s.DownstreamDeps, ", "), "→"))
		}
		sb.WriteString("\n")

		sb.WriteString("### Recent Incidents (Past 7 Days)\n")
		if len(s.RecentIncidents) > 0 {
			sb.WriteString("| ID | Sev | Svc | Created | Summary |\n")
			sb.WriteString("| :--- | :--- | :--- | :--- | :--- |\n")
			for _, inc := range s.RecentIncidents {
				sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n",
					inc.ID, inc.Severity, inc.Service, inc.CreatedAt.Format("Jan 02 15:04"), inc.Title))
			}
		} else {
			sb.WriteString("_No incidents in the last 7 days._\n")
		}
		sb.WriteString("\n")

		sb.WriteString("### Service Action Items\n")
		if len(s.RecentActions) > 0 {
			sb.WriteString("| ID | Pr | Svc | Details | Owner | Status | Due |\n")
			sb.WriteString("| :--- | :--- | :--- | :--- | :--- | :--- | :--- |\n")
			for _, ai := range s.RecentActions {
				due := ai.DueDate.Format("Jan 02")
				if ai.DueDate.IsZero() { due = "-" }
				
				status := ai.Status
				if status == "Completed" { status = "✅ Done" }
				if status == "Overdue" { status = "🔴 Overdue" }
				if status == "Pending" { status = "⏳ Pending" }

				details := ai.Description
				if len(details) > 30 { details = details[:27] + "..." }

				sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %s | %s |\n",
					ai.IncidentID, ai.Priority, ai.Service, details, ai.Owner, status, due))
			}
		} else {
			sb.WriteString("_No pending action items for this service._\n")
		}
		sb.WriteString("\n")
	}

	// 7. Detailed Action Items (Only for complete profile view, or if items exist across services)
	if m.report.ServiceSpecific == nil {
		if len(m.report.NextActionItems) > 0 {
			sb.WriteString("## 7. Detailed Action Items\n\n")
			sb.WriteString("| ID | Pr | Service | Details | Owner | Status | Due |\n")
			sb.WriteString("| :--- | :--- | :--- | :--- | :--- | :--- | :--- |\n")
			for _, ai := range m.report.NextActionItems {
				due := ai.DueDate.Format("Jan 02")
				if ai.DueDate.IsZero() { due = "-" }
				
				status := ai.Status
				if status == "Completed" { status = "✅ Done" }
				if status == "Overdue" { status = "🔴 Overdue" }
				if status == "Pending" { status = "⏳ Pending" }

				details := ai.Description
				if len(details) > 25 { details = details[:22] + "..." }
				
				sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %s | %s |\n",
					ai.IncidentID, ai.Priority, ai.Service, details, ai.Owner, status, due))
			}
			sb.WriteString("\n")
		} else {
			sb.WriteString("## 7. Detailed Action Items\n\n")
			sb.WriteString("_No pending action items found for this period._\n")
		}
	}
	
	return sb.String()
}

func (m scorecardModel) downloadReport() (string, error) {
	pm := config.GetProfileManager()
	dir := filepath.Join(pm.GetStatePath(), "reports")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	
	var filename string
	var content string
	
	if m.state == stateOrgReport && m.orgReport != nil {
		filename = fmt.Sprintf("%s-Organization.md", strings.ReplaceAll(m.orgReport.Month, " ", "-"))
		content = m.generateOrgMarkdown()
	} else if m.report != nil {
		filename = fmt.Sprintf("%s-%s.md", strings.ReplaceAll(m.report.Month, " ", "-"), m.report.Profile)
		if m.provider.TargetService != "" {
			filename = fmt.Sprintf("%s-%s-%s.md", strings.ReplaceAll(m.report.Month, " ", "-"), m.report.Profile, m.provider.TargetService)
		}
		content = m.generateMarkdown()
	} else {
		return "", fmt.Errorf("no report data to download")
	}
	
	path := filepath.Join(dir, filename)
	absPath, _ := filepath.Abs(path)
	
	err := os.WriteFile(path, []byte(content), 0644)
	return absPath, err
}

func (m scorecardModel) renderOrgReport() string {
	md := m.generateOrgMarkdown()
	width := m.width
	if width > 120 { width = 120 }
	r, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width-4),
	)
	out, _ := r.Render(md)
	return out
}

func (m scorecardModel) generateOrgMarkdown() string {
	var sb strings.Builder
	sb.WriteString("# HEALTH-MONITORING AGENT\n")
	sb.WriteString(fmt.Sprintf("## Organization Scoreboard: %s\n\n", m.orgReport.Month))

	if m.orgReport.NotAvailableReason != "" {
		sb.WriteString("\n> [!NOTE]\n")
		sb.WriteString(fmt.Sprintf("> **Information**: %s\n\n", m.orgReport.NotAvailableReason))
		sb.WriteString("Overall organization monitoring is active, but no participating profiles existed or had data for this specific month.\n")
		return sb.String()
	}
	
	// NEW: Organization Data Summary
	sb.WriteString("### 📊 Organization Data Summary\n")
	sb.WriteString("| KPI | Total | Status |\n")
	sb.WriteString("| :--- | :--- | :--- |\n")
	sb.WriteString(fmt.Sprintf("| Managed Profiles | %d | Active |\n", m.orgReport.TotalProfiles))
	sb.WriteString(fmt.Sprintf("| Monitored Services | %d | Healthy |\n", m.orgReport.TotalServices))
	sb.WriteString(fmt.Sprintf("| Active SLOs | %d | Tracked |\n", m.orgReport.TotalSLOs))
	
	actionStatus := "✅ OK"
	if m.orgReport.OverdueActions > 0 { actionStatus = fmt.Sprintf("🚨 %d OVERDUE", m.orgReport.OverdueActions) }
	sb.WriteString(fmt.Sprintf("| Open Action Items | %d | %s |\n", m.orgReport.TotalActionItems, actionStatus))
	sb.WriteString("\n")

	// 🌏 Executive Summary & Reliability Scorecard
	sb.WriteString("### 🌏 Executive Summary & Reliability Scorecard\n")
	
	healthColor := "✅ HEALTHY"
	if m.orgReport.HealthStatus == "At Risk" { healthColor = "⚠️ AT RISK" }
	if m.orgReport.HealthStatus == "Critical" { healthColor = "🔴 CRITICAL" }
	
	// SLO Compliance with Trend
	sloTrendStr := "stable"
	if m.orgReport.Trends != nil {
		delta := m.orgReport.Trends.SLOComplianceDelta * 100
		if delta > 0.05 { sloTrendStr = fmt.Sprintf("up %.1f%% MoM", delta) }
		if delta < -0.05 { sloTrendStr = fmt.Sprintf("down %.1f%% MoM", -delta) }
	}
	sb.WriteString(fmt.Sprintf("- **SLO Compliance**: %.1f%% (%s)\n", m.orgReport.GlobalSLORate, sloTrendStr))
	
	burnStatus := "✅ STABLE"
	if m.orgReport.GlobalBurnRate > 1.0 { burnStatus = "⚠️ ELEVATED" }
	if m.orgReport.GlobalBurnRate > 2.0 { burnStatus = "🚨 CRITICAL" }
	sb.WriteString(fmt.Sprintf("- **Global Burn Rate**: %.2fx (%s)\n", m.orgReport.GlobalBurnRate, burnStatus))

	// Incidents with MTTR
	sb.WriteString(fmt.Sprintf("- **Incidents**: %d P1s, %d P2s (MTTR %s)\n", 
		m.orgReport.P1Incidents, m.orgReport.P2Incidents, m.orgReport.AvgMTTR))

	// Toil
	toilStatus := "✅ HEALTHY"
	if m.orgReport.GlobalToilPercent > m.orgReport.GlobalToilTarget { toilStatus = "🚨 OVER TARGET" }
	
	toilTrendStr := "stable"
	if m.orgReport.GlobalToilTrend == "Improving" { toilTrendStr = "improving" }
	if m.orgReport.GlobalToilTrend == "Degrading" { toilTrendStr = "degrading" }

	sb.WriteString(fmt.Sprintf("- **Global Toil**: %.2f%% (target <%.0f%%) - %s (%s)\n", 
		m.orgReport.GlobalToilPercent, m.orgReport.GlobalToilTarget, toilStatus, toilTrendStr))
	if m.orgReport.GlobalPotentialSavings > 0 {
		sb.WriteString(fmt.Sprintf("- **Org-Wide ROI Opportunity**: $%.2f/mo\n", m.orgReport.GlobalPotentialSavings))
	}

	// Action Items
	sb.WriteString(fmt.Sprintf("- **Action Items**: %d open (%d overdue)\n", 
		m.orgReport.TotalActionItems, m.orgReport.OverdueActions))

	// Score & Status
	scoreTrendIcon := "→"
	if m.orgReport.Trends != nil {
		if m.orgReport.Trends.ScoreDelta > 0.05 { scoreTrendIcon = "↑" }
		if m.orgReport.Trends.ScoreDelta < -0.05 { scoreTrendIcon = "↓" }
	}
	sb.WriteString(fmt.Sprintf("- **Score**: %.0f/100 (%s) %s - (Aggregated from %d profiles)\n", 
		m.orgReport.OverallScore, scoreTrendIcon, healthColor, m.orgReport.TotalProfiles))
	sb.WriteString("\n")

	// Operational Metrics Block
	sb.WriteString("### 🛠️ Operational Health (Global)\n")
	sb.WriteString("| Metric | Value | Status |\n")
	sb.WriteString("| :--- | :--- | :--- |\n")
	
	availStatus := "✅ OK"
	if m.orgReport.GlobalAvailability < 99.9 { availStatus = "⚠️ WARNING" }
	if m.orgReport.GlobalAvailability < 99.0 { availStatus = "🔴 BREACH" }
	
	toilStatus = "✅ HEALTHY"
	if m.orgReport.ToilStatus == "Over Target" { toilStatus = "🚨 OVER BUDGET" }

	sb.WriteString(fmt.Sprintf("| Availability | %.2f%% | %s |\n", m.orgReport.GlobalAvailability, availStatus))
	sb.WriteString(fmt.Sprintf("| MTTA (Avg) | %s | %s |\n", m.orgReport.AvgMTTA, "→"))
	sb.WriteString(fmt.Sprintf("| MTTR (Avg) | %s | %s |\n", m.orgReport.AvgMTTR, "→"))
	sb.WriteString(fmt.Sprintf("| MTBF (Org) | %s | %s |\n", m.orgReport.GlobalMTBF, "→"))
	sb.WriteString(fmt.Sprintf("| Engineering Toil | %.1f hrs | %s |\n", m.orgReport.GlobalToilHours, toilStatus))
	sb.WriteString(fmt.Sprintf("| Toil Cost (Est.) | $%.2f | %s |\n", m.orgReport.GlobalToilCost, "→"))
	if m.orgReport.GlobalPotentialSavings > 0 {
		sb.WriteString(fmt.Sprintf("| Potential ROI | $%.2f/mo | %s |\n", m.orgReport.GlobalPotentialSavings, "✅"))
	}
	sb.WriteString("\n")

	// Global Hotspots section
	sb.WriteString("## 🔥 Section 1: Organizational Hotspots (Top At-Risk Services)\n\n")
	if len(m.orgReport.Hotspots) > 0 {
		sb.WriteString("| Service | Profile/Team | P1 Count | Risk Status |\n")
		sb.WriteString("| :--- | :--- | :--- | :--- |\n")
		for _, h := range m.orgReport.Hotspots {
			sb.WriteString(fmt.Sprintf("| %s | %s | %d | %s |\n", h.Service, h.Profile, h.P1Incidents, h.Status))
		}
	} else {
		sb.WriteString("_No critical hotspots detected across participating profiles._\n")
	}
	sb.WriteString("\n")

	// Profile Health Index
	sb.WriteString("## 📚 Section 2: Profile Health Index (Team Performance)\n\n")
	sb.WriteString("| Profile | Score | Health | P1s | Avail% | Trend |\n")
	sb.WriteString("| :--- | :--- | :--- | :--- | :--- | :--- |\n")
	for _, p := range m.orgReport.ProfileSummaries {
		trend := "→"
		if p.Trend == "Up" { trend = "↑" }
		if p.Trend == "Down" { trend = "↓" }
		
		status := p.HealthStatus
		if status == "Healthy" { status = "✅ OK" }
		if status == "At Risk" { status = "⚠️ RISK" }
		if status == "Critical" { status = "🔴 CRIT" }

		sb.WriteString(fmt.Sprintf("| %s | %.1f%% | %s | %d | %.2f%% | %s |\n",
			p.Name, p.Score, status, p.P1Incidents, p.Availability, trend))
	}
	sb.WriteString("\n")

	// Incident Categorization
	sb.WriteString("## 📊 Section 3: Incident Distribution (Cross-Organization)\n\n")
	sb.WriteString("| Category | Incident Count | % of Global Load |\n")
	sb.WriteString("| :--- | :--- | :--- |\n")
	for _, c := range m.orgReport.CategoryStats {
		sb.WriteString(fmt.Sprintf("| %s | %d | %.1f%% |\n", c.Category, c.Count, c.Percent))
	}
	sb.WriteString("\n")

	sb.WriteString("> [!IMPORTANT]\n")
	sb.WriteString("> **Analysis Note**: Global Score is calculated as the **weighted average** of participating profile scores based on service density. Availability is aggregated from P1/P2 downtime across the entire fleet.\n")
	sb.WriteString("> This view aggregates data across all active profiles. Use `scorecard --profile <name>` for deep-dive analysis of a specific team.\n")
	
	return sb.String()
}

func (m scorecardModel) reloadData() (scorecardModel, tea.Cmd) {
	if m.state == stateViewReport && m.report != nil {
		report, err := m.provider.GenerateReport(m.selectedProfile, m.viewYear, m.viewMonth)
		if err == nil {
			m.report = report
			m.viewport.SetContent(m.renderReport())
		}
	} else if m.state == stateOrgReport {
		report, err := m.provider.GenerateOrgReport(m.viewYear, m.viewMonth, m.envFilter)
		if err == nil {
			m.orgReport = report
			m.viewport.SetContent(m.renderOrgReport())
		}
	}
	return m, nil
}

type clearMessage struct{}
