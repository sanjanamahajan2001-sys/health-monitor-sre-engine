package incident

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
	"os"

	"health-monitor/internal/audit"
	"health-monitor/internal/config"
	"health-monitor/internal/output"
	"health-monitor/internal/demo"
	
	"github.com/aymanbagabas/go-osc52/v2"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type incidentTUIModel struct {
	incident    Incident
	width       int
	height      int
	quitting         bool
	copySuccess      string
	copyError        string
	statusMsg        string
	postmortemPath   string
	postmortemError  string
	service          *Service
	broadcaster      Broadcaster
	viewport         viewport.Model
	ready            bool

	// Collaborative Session Banner
	connectionInfo string

	input            textinput.Model
	activeInput      inputType
	wizardData       map[string]string
	wizardSteps      []inputType
	currentStep      int

	// Demo Simulation
	viewMode         demoViewMode
}

type demoViewMode int

const (
	viewDetails demoViewMode = iota
	viewRunbook
	viewPostmortem
)

var (
	tuiTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("6")).
			MarginBottom(1)

	tuiLabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("7")).
			Bold(true)

	tuiDimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))

	tuiBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("8")).
			Padding(1, 2)

	tuiLinkStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("4")).
			Underline(true)

	tuiKeyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("3")).
			Bold(true).
			PaddingRight(1)

	tuiSuccessStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("2")).
			Bold(true)

	tuiErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("1")).
			Bold(true)
)

type inputType int

const (
	inputNone inputType = iota
	inputNote
	inputResolveSummary
	inputResolveRootCause
	inputResolveFix
	inputResolveComponent
	inputResolveCategory
	inputResolveDependency
	inputResolveFailureType
	inputResolvePrevention
	inputResolveWell
	inputResolveBetter
	inputResolveLucky
	inputResolveDowntime
	inputResolveToilMin
	inputResolveToilCat
	inputActionDesc
	inputActionOwner
	inputActionPriority
	inputActionDueDate
	inputActionStatus
)

func ShowIncidentTUI(service *Service, incident Incident, bc Broadcaster) error {
	ti := textinput.New()
	ti.Placeholder = "Enter message..."
	ti.CharLimit = 200
	ti.Width = 60

	model := incidentTUIModel{
		service:     service,
		incident:    incident,
		broadcaster: bc,
		input:       ti,
	}

	if bc != nil {
		model.connectionInfo = bc.GetConnectionInfo()
	}

	p := tea.NewProgram(model, tea.WithAltScreen())
	
	// Silence standard output/error logs to prevent "garbage" during TUI
	output.Silence()
	defer output.Unsilence()

	// Handle real-time updates from collaborative session - use goroutine to prevent deadlock
	if bc != nil {
		bc.OnUpdate(func(msg string) {
			go p.Send(updateContent{msg: msg})
		})
	}

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("failed to start incident TUI: %w", err)
	}
	return nil
}

func (m incidentTUIModel) Init() tea.Cmd {
	// Explicitly disable mouse tracking to prevent garbage if terminal was left in a weird state
	disableMouse := func() tea.Msg {
		fmt.Print("\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l")
		return nil
	}
	return tea.Batch(textinput.Blink, disableMouse)
}

func (m incidentTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	if m.activeInput != inputNone {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "enter":
				val := m.input.Value()
				m.wizardData[m.input.Placeholder] = val
				m.input.SetValue("")
				
				m.currentStep++
				if m.currentStep < len(m.wizardSteps) {
					m.activeInput = m.wizardSteps[m.currentStep]
					m.input.Placeholder = m.getPlaceholder(m.activeInput)
					return m, nil
				}
				
				// Wizard Finished
				var cmd tea.Cmd
				switch m.wizardSteps[0] {
				case inputResolveSummary:
					analysis := &audit.IncidentAnalysis{
						RootCause:  m.wizardData[m.getPlaceholder(inputResolveRootCause)],
						FixSummary: m.wizardData[m.getPlaceholder(inputResolveFix)],
						Component:  m.wizardData[m.getPlaceholder(inputResolveComponent)],
						Category:   m.wizardData[m.getPlaceholder(inputResolveCategory)],
						Dependency: m.wizardData[m.getPlaceholder(inputResolveDependency)],
						FailureType: m.wizardData[m.getPlaceholder(inputResolveFailureType)],
						Prevention: m.wizardData[m.getPlaceholder(inputResolvePrevention)],
						LessonsLearned: &audit.LessonsLearned{
							WhatWentWell:      m.wizardData[m.getPlaceholder(inputResolveWell)],
							WhatCouldBeBetter: m.wizardData[m.getPlaceholder(inputResolveBetter)],
							WhereWeGotLucky:   m.wizardData[m.getPlaceholder(inputResolveLucky)],
						},
					}
					summary := m.wizardData[m.getPlaceholder(inputResolveSummary)]
					if summary == "" {
						summary = "Resolved via TUI"
					}
					
					// Handle toil and downtime if needed (omitted for brevity in this step, can add later)
					
					cmd = m.doResolve(summary, analysis)
				case inputActionDesc:
					priorityStr := m.wizardData["Priority (P1/P2/P3/P4) [P2]"]
					priority, err := audit.ParseActionItemPriority(priorityStr)
					if err != nil {
						m.statusMsg = "✗ " + err.Error()
						m.activeInput = inputNone
						m.input.Blur()
						return m, nil
					}
					
					dueDateStr := m.wizardData["Due date (YYYY-MM-DD) [2026-03-26]"]
					if dueDateStr == "" { dueDateStr = "2026-03-26" }
					dueDate, _ := time.Parse("2006-01-02", dueDateStr)
					
					statusStr := m.wizardData["Status (TODO/IN_PROGRESS/DONE/WONT_FIX) [TODO]"]
					status, err := audit.ParseActionItemStatus(statusStr)
					if err != nil {
						m.statusMsg = "✗ " + err.Error()
						m.activeInput = inputNone
						m.input.Blur()
						return m, nil
					}
					
					cmd = m.doAddAction(
						m.wizardData["Description (e.g., Fix database retry logic)"],
						m.wizardData["Owner (e.g., sanjana)"],
						priority,
						status,
						dueDate,
					)
				case inputNote:
					cmd = m.doAddNote(val)
				}
				
				m.activeInput = inputNone
				m.input.Blur()
				return m, cmd
			case "esc", "ctrl+c":
				m.activeInput = inputNone
				m.input.Blur()
				return m, nil
			}
		}
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit

		case "g":
			if m.incident.Links.Grafana != "" {
				return m, m.copyLink(m.incident.Links.Grafana, "Grafana")
			}

		case "p":
			if m.incident.Links.Prometheus != "" {
				return m, m.copyLink(m.incident.Links.Prometheus, "Prometheus")
			}

		case "l":
			if m.incident.Links.Logs != "" {
				return m, m.copyLink(m.incident.Links.Logs, "Logs")
			}

		case "t":
			if m.incident.Links.Traces != "" {
				return m, m.copyLink(m.incident.Links.Traces, "Traces")
			}

		case "r":
			if os.Getenv("HEALTH_MONITOR_DEMO_MODE") == "true" {
				if m.viewMode == viewRunbook {
					m.viewMode = viewDetails
				} else {
					m.viewMode = viewRunbook
				}
				if m.ready {
					m.viewport.SetContent(m.renderFullContent())
					m.viewport.GotoTop()
				}
				return m, nil
			}
			return m, m.refreshIncident()

		case "n":
			m.activeInput = inputNote
			m.wizardSteps = []inputType{inputNote}
			m.wizardData = make(map[string]string)
			m.currentStep = 0
			m.input.Placeholder = m.getPlaceholder(m.activeInput)
			m.input.Focus()
			return m, nil

		case "a":
			return m, m.doAcknowledge()

		case "s":
			m.activeInput = inputResolveSummary
			m.wizardSteps = []inputType{
				inputResolveSummary, inputResolveRootCause, inputResolveFix, 
				inputResolveComponent, inputResolveCategory, inputResolveDependency, 
				inputResolveFailureType, inputResolvePrevention, inputResolveWell, 
				inputResolveBetter, inputResolveLucky, inputResolveDowntime, 
				inputResolveToilMin, inputResolveToilCat,
			}
			m.wizardData = make(map[string]string)
			m.currentStep = 0
			m.input.Placeholder = m.getPlaceholder(m.activeInput)
			m.input.Focus()
			return m, nil

		case "i":
			m.activeInput = inputActionDesc
			m.wizardSteps = []inputType{inputActionDesc, inputActionOwner, inputActionPriority, inputActionDueDate, inputActionStatus}
			m.wizardData = make(map[string]string)
			m.currentStep = 0
			m.input.Placeholder = m.getPlaceholder(m.activeInput)
			m.input.Focus()
			return m, nil

		case "m":
			if m.service != nil {
				path, err := m.service.GeneratePostmortem(m.incident.ID)
				if err != nil {
					m.postmortemError = err.Error()
					m.postmortemPath = ""
				} else {
					m.postmortemPath = path
					m.postmortemError = ""
				}
				// Clear status after 5s
				return m, tea.Batch(
					tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
						return clearStatus{}
					}),
					func() tea.Msg { return updateContent{} },
				)
			}
		}

	case actionResult:
		if msg.err != nil {
			m.statusMsg = "✗ Error: " + msg.err.Error()
		} else {
			m.statusMsg = msg.successMsg
			if m.broadcaster != nil && msg.broadcastMsg != "" {
				m.broadcaster.Broadcast(msg.broadcastMsg)
			}
		}
		return m, tea.Batch(
			m.refreshIncident(),
			tea.Tick(3*time.Second, func(t time.Time) tea.Msg { return clearStatus{} }),
		)

	case incidentUpdated:
		m.incident = msg.incident
		if m.ready {
			m.viewport.SetContent(m.renderFullContent())
		}
		return m, nil

	case updateContent:
		if msg.msg != "" {
			m.statusMsg = "🔔 " + msg.msg
		}
		if m.ready {
			m.viewport.SetContent(m.renderFullContent())
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Vertical: Title(2) + Box Border(2) + Padding(2) + Status(2) = ~8
		// Horizontal: Box Border(2) + Padding(4) = 6
		vWidth := msg.Width - 6
		vHeight := msg.Height - 8

		if !m.ready {
			m.viewport = viewport.New(vWidth, vHeight)
			m.viewport.HighPerformanceRendering = false
			m.ready = true
		} else {
			m.viewport.Width = vWidth
			m.viewport.Height = vHeight
		}
		m.viewport.SetContent(m.renderFullContent())

	case linkOpened, linksCopied, linkError, clearStatus:
		var cmd tea.Cmd
		m, cmd = m.handleLinkMsg(msg)
		if m.ready {
			m.viewport.SetContent(m.renderFullContent())
		}
		return m, cmd
	}

	if m.ready {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}

	return m, nil
}

type updateContent struct {
	msg string
}

func (m incidentTUIModel) renderFullContent() string {
	var content strings.Builder

	// Handle Simulated Views in Demo Mode
	if os.Getenv("HEALTH_MONITOR_DEMO_MODE") == "true" {
		switch m.viewMode {
		case viewRunbook:
			rb := demo.GetRunbook(m.incident.ID)
			if rb != nil {
				return rb.Content + "\n\n" + tuiDimStyle.Render("(Press 'r' to return to details)")
			}
		case viewPostmortem:
			pm := demo.GetPostmortem(m.incident.ID)
			if pm != nil {
				return pm.Content + "\n\n" + tuiDimStyle.Render("(Press 'm' to return to details)")
			}
		}
	}

	details := GenerateIncidentDetails(m.incident, m.width)
	content.WriteString(details)
	content.WriteString("\n\n")
	content.WriteString(tuiLabelStyle.Render("Controls"))
	content.WriteString("\n")
	content.WriteString(tuiDimStyle.Render(strings.Repeat("─", 8)))
	content.WriteString("\n")

	observabilityKeys := []string{}
	if m.incident.Links.Grafana != "" {
		observabilityKeys = append(observabilityKeys, tuiKeyStyle.Render("g")+"Grafana")
	}
	if m.incident.Links.Prometheus != "" {
		observabilityKeys = append(observabilityKeys, tuiKeyStyle.Render("p")+"Prometheus")
	}
	if m.incident.Links.Logs != "" {
		observabilityKeys = append(observabilityKeys, tuiKeyStyle.Render("l")+"Logs")
	}
	if m.incident.Links.Traces != "" {
		observabilityKeys = append(observabilityKeys, tuiKeyStyle.Render("t")+"Traces")
	}
	if runbookURL := getRunbookURLTUI(m.incident.Service); runbookURL != "" {
		observabilityKeys = append(observabilityKeys, tuiKeyStyle.Render("b")+"Runbook")
	}
	if hasObservabilityLinks(m.incident.Links) {
		observabilityKeys = append(observabilityKeys, tuiKeyStyle.Render("c")+"Copy all")
	}

	actionKeys := []string{
		tuiKeyStyle.Render("r")+"Refresh",
		tuiKeyStyle.Render("n")+"Note",
		tuiKeyStyle.Render("a")+"Ack",
		tuiKeyStyle.Render("v")+"View",
		tuiKeyStyle.Render("s")+"Resolve",
		tuiKeyStyle.Render("i")+"Action",
		tuiKeyStyle.Render("m")+"Postmortem",
		tuiKeyStyle.Render("q")+"Quit",
	}

	content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("Observability: ") + strings.Join(observabilityKeys, "  "))
	content.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("Actions:       ") + strings.Join(actionKeys, "  "))

	// Status messages
	if m.postmortemPath != "" {
		content.WriteString("\n\n" + tuiSuccessStyle.Render("✓ Postmortem generated: "+m.postmortemPath))
	}
	if m.postmortemError != "" {
		content.WriteString("\n\n" + tuiErrorStyle.Render("✗ Postmortem failed: "+m.postmortemError))
	}
	if m.copySuccess != "" {
		content.WriteString("\n\n" + tuiSuccessStyle.Render(m.copySuccess))
	}
	if m.copyError != "" {
		content.WriteString("\n\n" + tuiErrorStyle.Render("✗ " + m.copyError))
	}

	return content.String()
}

func (m incidentTUIModel) View() string {
	if m.quitting {
		return ""
	}

	if !m.ready {
		return "\n  Initializing..."
	}

	if m.activeInput != inputNone {
		header := fmt.Sprintf("%s (Step %d/%d)", m.input.Placeholder, m.currentStep+1, len(m.wizardSteps))
		inputBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("6")).
			Padding(1, 2).
			Render(fmt.Sprintf("%s\n\n%s\n\n(Enter to continue, Esc to cancel)", 
				lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("5")).Render(header),
				m.input.View()))
		
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, inputBox)
	}

	banner := ""
	if m.connectionInfo != "" {
		bannerStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("255")).
			Background(lipgloss.Color("27")).
			Padding(0, 2).
			MarginBottom(1).
			Width(m.width)
		banner = bannerStyle.Render(m.connectionInfo) + "\n"
	}

	return banner + tuiBoxStyle.Render(m.viewport.View())
}

type incidentUpdated struct {
	incident Incident
}

func (m incidentTUIModel) refreshIncident() tea.Cmd {
	return func() tea.Msg {
		inc, err := m.service.store.Load(m.incident.ID)
		if err != nil {
			return nil
		}
		return incidentUpdated{incident: inc}
	}
}

func (m incidentTUIModel) openLink(url string) tea.Cmd {
	return tea.ExecProcess(exec.Command("xdg-open", url), func(err error) tea.Msg {
		if err != nil {
			return linkError{err: err}
		}
		return linkOpened{}
	})
}

func (m incidentTUIModel) copyLink(url, name string) tea.Cmd {
	return func() tea.Msg {
		// Use OSC 52 to copy to clipboard (zero-dependency)
		fmt.Print(osc52.New(url).String())
		return linksCopied{count: 1}
	}
}

func (m incidentTUIModel) copyAllLinks() tea.Cmd {
	var urls []string
	if m.incident.Links.Grafana != "" {
		urls = append(urls, m.incident.Links.Grafana)
	}
	if m.incident.Links.Prometheus != "" {
		urls = append(urls, m.incident.Links.Prometheus)
	}
	if m.incident.Links.Logs != "" {
		urls = append(urls, m.incident.Links.Logs)
	}
	if m.incident.Links.Traces != "" {
		urls = append(urls, m.incident.Links.Traces)
	}

	if len(urls) == 0 {
		return nil
	}

	return func() tea.Msg {
		// Use OSC 52 to copy all links
		fmt.Print(osc52.New(strings.Join(urls, "\n")).String())
		return linksCopied{count: len(urls)}
	}
}

func truncateURL(url string, maxLen int) string {
	if len(url) <= maxLen {
		return url
	}
	return url[:maxLen-3] + "..."
}

// Message types for link operations
type linkOpened struct{}
type linksCopied struct{ count int }
type linkError struct{ err error }

func (m incidentTUIModel) handleLinkMsg(msg tea.Msg) (incidentTUIModel, tea.Cmd) {
	switch msg := msg.(type) {
	case linkOpened:
		// Link opened successfully - no action needed
		return m, nil
	case linksCopied:
		m.copySuccess = fmt.Sprintf("Copied %d links to clipboard", msg.count)
		m.copyError = ""
		// Clear success message after 3 seconds
		return m, tea.Batch(
			tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
				return clearStatus{}
			}),
			func() tea.Msg { return updateContent{} },
		)
	case linkError:
		m.copyError = fmt.Sprintf("Failed to open/copy link: %v", msg.err)
		m.copySuccess = ""
		// Clear error message after 3 seconds
		return m, tea.Batch(
			tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
				return clearStatus{}
			}),
			func() tea.Msg { return updateContent{} },
		)
	case clearStatus:
		m.copySuccess = ""
		m.copyError = ""
		m.postmortemPath = ""
		m.postmortemError = ""
		return m, nil
	}
	return m, nil
}

func getRunbookURLTUI(service string) string {
	cfg, err := config.Load()
	if err != nil {
		return ""
	}
	
	if cfg.RunbookURLs == nil {
		return ""
	}
	
	return cfg.RunbookURLs[service]
}

type clearStatus struct{}
func (m incidentTUIModel) getPlaceholder(it inputType) string {
	switch it {
	case inputNote:
		return "Note message"
	case inputResolveSummary: return "Summary (One-line description of fix)"
	case inputResolveRootCause: return "Root Cause (What happened?)"
	case inputResolveFix: return "Fix (How was it resolved?)"
	case inputResolveComponent: return "Component (e.g., api, database)"
	case inputResolveCategory: return "Category (capacity, latency, dependency, config, infra, deployment, security)"
	case inputResolveDependency: return "Dependency (e.g., api->db)"
	case inputResolveFailureType: return "Failure Type (service, dependency, infra)"
	case inputResolvePrevention: return "Prevention Measures"
	case inputResolveWell: return "What went well"
	case inputResolveBetter: return "What could be better"
	case inputResolveLucky: return "Where we got lucky"
	case inputResolveDowntime: return "Downtime (minutes, e.g. 5)"
	case inputResolveToilMin: return "Toil/Manual Effort (minutes, e.g. 10)"
	case inputResolveToilCat: return "Toil Category (manual_restart, config_change, etc.)"
	case inputActionDesc: return "Description (e.g., Fix database retry logic)"
	case inputActionOwner: return "Owner (e.g., sanjana)"
	case inputActionPriority: return "Priority (P1/P2/P3/P4) [P2]"
	case inputActionDueDate: return "Due date (YYYY-MM-DD) [2026-03-26]"
	case inputActionStatus: return "Status (TODO/IN_PROGRESS/DONE/WONT_FIX) [TODO]"
	}
	return "Enter value..."
}
func (m incidentTUIModel) doAcknowledge() tea.Cmd {
	return func() tea.Msg {
		_, _, err := m.service.AcknowledgeByID(m.incident.ID, "sanjana")
		return actionResult{
			err:          err,
			successMsg:   "✓ Incident acknowledged",
			broadcastMsg: "sanjana acknowledged the incident",
		}
	}
}

func (m incidentTUIModel) doResolve(summary string, analysis *audit.IncidentAnalysis) tea.Cmd {
	return func() tea.Msg {
		_, _, err := m.service.ResolveByIDWithAnalysis(m.incident.ID, summary, "sanjana", analysis, nil)
		return actionResult{
			err:          err,
			successMsg:   "✓ Incident resolved",
			broadcastMsg: "sanjana resolved the incident",
		}
	}
}

func (m incidentTUIModel) doAddNote(msg string) tea.Cmd {
	return func() tea.Msg {
		_, _, err := m.service.NoteByID(m.incident.ID, msg, "sanjana")
		return actionResult{
			err:          err,
			successMsg:   "✓ Note added",
			broadcastMsg: "sanjana added a note: " + msg,
		}
	}
}

func (m incidentTUIModel) doAddAction(desc, owner string, priority ActionItemPriority, status ActionItemStatus, dueDate time.Time) tea.Cmd {
	return func() tea.Msg {
		if owner == "" { owner = "sanjana" }
		if dueDate.IsZero() { dueDate = time.Now().AddDate(0, 0, 7) }
		
		_, err := m.service.AddActionItem(m.incident.ID, desc, owner, priority, status, dueDate)
		return actionResult{
			err:          err,
			successMsg:   "✓ Action item added",
			broadcastMsg: fmt.Sprintf("sanjana added action: %s", desc),
		}
	}
}

type actionResult struct {
	err          error
	successMsg   string
	broadcastMsg string
}
