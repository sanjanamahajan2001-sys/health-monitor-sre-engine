package tui

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"

	"health-monitor/internal/analyse/api_latency"
	"health-monitor/internal/analyse/loki"
	"health-monitor/internal/analyse/prometheus"
	"health-monitor/internal/checks"
	"health-monitor/internal/config"
	"health-monitor/internal/flow"
	"health-monitor/internal/incident"
	"health-monitor/internal/output"
	"health-monitor/pkg/model"
)

// Save current configuration using profile-aware saving
func saveCurrentConfig(cfg config.Config) error {
	pm := config.GetProfileManager()
	activeProfile := pm.GetActiveProfile()
	return config.SaveForProfile(activeProfile, cfg)
}

/* ------------------ Styles Moved to tui_styles.go ------------------ */

const refreshInterval = 30 * time.Second

type tuiModel struct {
	report                model.Report
	content               string
	scrollY               int
	height                int
	width                 int
	ready                 bool
	showHelp              bool
	showConfig            bool
	showPromConfig        bool
	promIndex             int
	promInputs            []textinput.Model
	editActive            bool
	editIndex             int
	editInputs            []textinput.Model
	editFields            []editField
	editError             string
	editSaved             bool
	editConfig            config.Config
	showServices          bool
	serviceIndex          int
	serviceList           []string
	serviceMessage        string
	showEndpoints         bool
	endpointIndex         int
	endpointList          []string
	endpointMessage       string
	baseTopEndpoints      []model.EndpointLatency
	baseTopEndpointsRPS   []model.EndpointRate
	baseTopServicesRPS    []model.ServiceRate
	authEditActive        bool
	authIndex             int
	authInputs            []textinput.Model
	authMessage           string
	authTestResult        string
	showLogs              bool
	logEntries            []loki.LogEntry
	logMessage            string
	logQuery              string
	logAutoNote           string
	logLinkPath           string
	logLink               string
	promLink              string
	promLinkPath          string
	promMessage           string
	correlationLink       string
	correlationLinkPath   string
	correlationMessage    string
	promConfigMessage     string
	promConfigTest        string
	traceLinks            []model.TraceLink
	traceLinkPath         string
	logLoading            bool
	showLokiConfig        bool
	showDebug             bool
	debugOutput           string
	lastRefresh           time.Time
	loadingRefresh        bool
	lokiIndex             int
	lokiInputs            []textinput.Model
	lokiMessage           string
	lokiTestResult        string
	showGrafanaConfig     bool
	grafanaIndex          int
	grafanaInputs         []textinput.Model
	grafanaMessage        string
	showTempoConfig       bool
	tempoIndex            int
	tempoInputs           []textinput.Model
	tempoMessage          string
	showJaegerConfig      bool
	jaegerIndex           int
	jaegerInputs          []textinput.Model
	jaegerMessage         string
	showCorrelationConfig bool
	correlationIndex      int
	correlationInputs     []textinput.Model
	correlationConfigMsg  string
	showBackendSelector   bool
	backendIndex          int
	backendOptions        []string
	backendMessage        string
	showElasticConfig     bool
	elasticIndex          int
	elasticInputs         []textinput.Model
	elasticMessage        string
	exportMessage         string
	showIncidents         bool
	activeIncident        *incident.Incident
	incidentList          []incident.Incident
	incidentMessage       string
	incidentWarnings      []string
	// Incident browser state
	incidentBrowserMode    bool                // true = list view, false = detail view
	incidentSelectedIndex  int                 // Currently selected incident in list
	incidentScrollY        int                 // Scroll position in detail view
	incidentPageSize       int                 // Incidents per page (default 10)
	incidentCurrentPage    int                 // Current page number (0-indexed)
	incidentFilterService  string              // Filter by service name
	incidentFilterSeverity string              // Filter by severity (P1-P4)
	incidentFilterState    string              // Filter by state (active/resolved/all)
	filteredIncidentList   []incident.Incident // Filtered incident list
	selectedIncident       *incident.Incident  // Currently selected incident for detail view
	showIncidentFilters    bool                // Show filter input panel
	incidentFilterInputs   []textinput.Model   // Filter input fields
	incidentFilterIndex    int                 // Current filter field index
	showFlows             bool
	flowList              []flow.Flow
	flowStatuses          map[string]bool
	flowIncidents         map[string]int
	flowReasons           map[string][]string
	flowMessage           string
	grafanaDSCache        map[string]grafanaDSCacheEntry
	spinner               spinner.Model
	loadingServices       bool
	loadingEndpoints      bool
	showClusterHealth          bool
	showServiceDrilldown       bool
	showServiceDrilldownDetail  bool
	serviceDrilldownIndex      int
	selectedDrilldownService   string

	// Demo Simulation
	viewMode         demoViewMode
}

type demoViewMode int

const (
	viewDetails demoViewMode = iota
	viewRunbook
	viewPostmortem
)

type serviceListMsg struct {
	list    []string
	message string
}

type endpointListMsg struct {
	list    []string
	message string
}

type refreshTickMsg struct{}

type apiRefreshMsg struct {
	report model.Report
}

type logsMsg struct {
	entries []loki.LogEntry
	query   string
	note    string
	message string
}

type filterResultMsg struct {
	report  model.Report
	message string
	err     error
}

type incidentListMsg struct {
	active   *incident.Incident
	list     []incident.Incident
	message  string
	warnings []string
}

type editField struct {
	key         string
	label       string
	placeholder string
	sensitive   bool
}

type grafanaDSCacheEntry struct {
	uid       string
	dsType    string
	fetchedAt time.Time
}

func initialModel(report model.Report) tuiModel {
	
	var baseEndpoints []model.EndpointLatency
	var baseEndpointsRPS []model.EndpointRate
	var baseServicesRPS []model.ServiceRate
	lastRefresh := time.Time{}
	if report.APILatency != nil {
		baseEndpoints = report.APILatency.TopEndpoints
		baseEndpointsRPS = report.APILatency.TopEndpointsRPS
		baseServicesRPS = report.APILatency.TopServicesRPS
		lastRefresh = time.Now()
	}
	spin := spinner.New()
	spin.Spinner = spinner.Spinner{
		Frames: []string{"-", "\\", "|", "/"},
		FPS:    120 * time.Millisecond,
	}
	
	// Create model with ready=true to show content immediately
	m := tuiModel{
		report:                report,
		scrollY:               0,
		height:                24,  // Set default height
		width:                 80,  // Set default width
		ready:                 true, // Set ready to true immediately
		showHelp:              false,
		showConfig:            false,
		editActive:            false,
		showServices:          false,
		serviceIndex:          0,
		serviceList:           nil,
		serviceMessage:        "",
		showEndpoints:         false,
		endpointIndex:         0,
		endpointList:          nil,
		endpointMessage:       "",
		showLogs:              false,
		logEntries:            nil,
		logMessage:            "",
		logQuery:              "",
		logAutoNote:           "",
		logLinkPath:           "",
		logLink:               "",
		promLink:              "",
		promLinkPath:          "",
		promMessage:           "",
		correlationLink:       "",
		correlationLinkPath:   "",
		correlationMessage:    "",
		showIncidents:         os.Getenv("HEALTH_MONITOR_SHOW_INCIDENTS") == "true",
		baseTopEndpoints:      baseEndpoints,
		baseTopEndpointsRPS:   baseEndpointsRPS,
		baseTopServicesRPS:    baseServicesRPS,
		authEditActive:        false,
		authIndex:             0,
		authInputs:            nil,
		authMessage:           "",
		authTestResult:        "",
		showFlows:             false,
		flowList:              nil,
		flowStatuses:          nil,
		flowIncidents:         nil,
		flowReasons:           nil,
		flowMessage:           "",
		grafanaDSCache:        map[string]grafanaDSCacheEntry{},
		spinner:               spin,
		loadingServices:       false,
		loadingEndpoints:      false,
		loadingRefresh:        true,
		incidentBrowserMode:   true,
		lastRefresh:           lastRefresh,
	}
	
	m.content = m.renderContent()
	
	return m
}

// loadIncidents loads incidents from the service
func loadIncidents() tea.Cmd {
	return func() tea.Msg {
		service, err := incident.NewService(nil)
		if err != nil {
			return incidentListMsg{message: "Failed to initialize incident service: " + err.Error()}
		}
		active, warnings, _ := service.View("")
		list, listWarnings, _ := service.List()
		return incidentListMsg{
			active:   active,
			list:     list,
			warnings: append(warnings, listWarnings...),
		}
	}
}

func (m tuiModel) Init() tea.Cmd {
	// Request window size to initialize properly and start loading data immediately
	return tea.Batch(
		tea.EnterAltScreen, 
		func() tea.Msg {
			m.loadingRefresh = true 
			return m.refreshAPILatencyCmd()()
		},
		refreshTick(),
	)
}

func refreshTick() tea.Cmd {
	return tea.Tick(refreshInterval, func(time.Time) tea.Msg {
		return refreshTickMsg{}
	})
}

func (m tuiModel) refreshAPILatencyCmd() tea.Cmd {
	return func() tea.Msg {
		defer func() {
			if r := recover(); r != nil {
				config.DebugLog("CRITICAL: Background refresh crashed: %v", r)
			}
		}()

		if m.report.IsDemo {
			// In demo mode, don't perform real checks, just return existing report
			return apiRefreshMsg{report: m.report}
		}
		var updated model.Report
		config.DebugLog("TUI: background refresh started")
		checks.APILatency(&updated)
		checks.Infra(&updated)
		checks.ServiceDrilldown(&updated, m.selectedDrilldownService)
		config.DebugLog("TUI: background refresh finished")
		return apiRefreshMsg{report: updated}
	}
}

func (m tuiModel) refreshIncidentsCmd() tea.Cmd {
	return func() tea.Msg {
		if m.report.IsDemo {
			// In demo mode, we don't need to refresh from the real store as it's static
			// But we should return the current list to avoid clearing it
			return incidentListMsg{
				active: m.selectedIncident,
				list:   m.incidentList,
			}
		}
		service, err := incident.NewService(nil)
		if err != nil {
			return incidentListMsg{message: "Failed to initialize incident service: " + err.Error()}
		}
		active, warnings, _ := service.View("")
		list, listWarnings, _ := service.List()
		return incidentListMsg{
			active:   active,
			list:     list,
			warnings: append(warnings, listWarnings...),
		}
	}
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if !m.ready {
			m.ready = true
		}
		if m.showHelp {
			m.content = m.renderHelp()
		} else if m.showConfig {
			m.content = m.renderConfig()
		} else if m.showPromConfig {
			m.content = m.renderPromEditor()
		} else if m.showCorrelationConfig {
			m.content = m.renderCorrelationEditor()
		} else if m.showBackendSelector {
			m.content = m.renderBackendSelector()
		} else if m.showElasticConfig {
			m.content = m.renderElasticEditor()
		} else if m.showServices {
			m.content = m.renderServices()
		} else if m.showEndpoints {
			m.content = m.renderEndpoints()
		} else if m.showLogs {
			m.content = m.renderLogs()
		} else if m.showLokiConfig {
			m.content = m.renderLokiEditor()
		} else if m.showGrafanaConfig {
			m.content = m.renderGrafanaEditor()
		} else if m.showTempoConfig {
			m.content = m.renderTempoEditor()
		} else if m.showJaegerConfig {
			m.content = m.renderJaegerEditor()
		} else if m.showCorrelationConfig {
			m.content = m.renderCorrelationEditor()
		} else if m.showBackendSelector {
			m.content = m.renderBackendSelector()
		} else if m.showElasticConfig {
			m.content = m.renderElasticEditor()
		} else if m.showIncidents {
			m.content = m.renderIncidents()
		} else if m.showFlows {
			m.content = m.renderFlows()
		} else if m.authEditActive {
			m.content = m.renderAuthEditor()
		} else if m.editActive {
			m.content = m.renderEdit()
		} else if m.showClusterHealth {
			m.content = m.renderClusterHealth()
		} else if m.showServiceDrilldown {
			m.content = m.renderServiceDrilldown()
		} else {
			m.content = m.renderContent()
		}
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		if m.loadingServices || m.loadingEndpoints || m.loadingRefresh {
			return m, cmd
		}
		return m, nil

	case refreshTickMsg:
		if m.loadingRefresh || m.showLogs || m.showLokiConfig || m.showGrafanaConfig || m.showPromConfig || m.showTempoConfig || m.showJaegerConfig || m.showCorrelationConfig || m.showBackendSelector || m.showElasticConfig || m.showIncidents || m.showFlows || m.authEditActive || m.editActive {
			return m, refreshTick()
		}
		m.loadingRefresh = true
		return m, tea.Batch(m.refreshAPILatencyCmd(), refreshTick(), m.spinner.Tick)

	case serviceListMsg:
		if m.showServices {
			if len(msg.list) > 0 {
				m.serviceList = msg.list
			}
			if strings.TrimSpace(msg.message) != "" {
				m.serviceMessage = msg.message
			}
			m.loadingServices = false
			m.content = m.renderServices()
			return m, nil
		}
	case endpointListMsg:
		if m.showEndpoints {
			if len(msg.list) > 0 {
				m.endpointList = msg.list
			}
			if strings.TrimSpace(msg.message) != "" {
				m.endpointMessage = msg.message
			}
			m.loadingEndpoints = false
			m.content = m.renderEndpoints()
			return m, nil
		}
	case apiRefreshMsg:
		m.loadingRefresh = false
		infraStatus := m.report.InfraStatus // preserve
		if msg.report.APILatency != nil {
			m.report.APILatency = msg.report.APILatency
			m.baseTopEndpoints = msg.report.APILatency.TopEndpoints
			m.baseTopEndpointsRPS = msg.report.APILatency.TopEndpointsRPS
			m.baseTopServicesRPS = msg.report.APILatency.TopServicesRPS
			m.lastRefresh = time.Now()
		}
		if msg.report.InfraStatus != nil {
			m.report.InfraStatus = msg.report.InfraStatus
		} else {
			m.report.InfraStatus = infraStatus
		}
		
		if msg.report.APILatency == nil && msg.report.APINote != "" {
			m.report.APINote = msg.report.APINote
			m.report.APILatency = nil
		}
		if msg.report.APINote != "" {
			m.report.APINote = msg.report.APINote
		}
		if msg.report.APIConfig != nil {
			m.report.APIConfig = msg.report.APIConfig
		}
		if len(msg.report.AllServices) > 0 {
			m.report.AllServices = msg.report.AllServices
		}
		if msg.report.ServiceDrilldown != nil {
			m.report.ServiceDrilldown = msg.report.ServiceDrilldown
			config.DebugLog("TUI: Updated ServiceDrilldown for %s", msg.report.ServiceDrilldown.Service)
		} else {
			config.DebugLog("TUI: Received apiRefreshMsg WITHOUT ServiceDrilldown")
		}
		m.promMessage = ""
		m.exportMessage = ""
		m.updatePromLink()
		if msg.report.APM != nil {
			m.report.APM = msg.report.APM
		}
		if msg.report.Tracing != nil {
			m.report.Tracing = msg.report.Tracing
		}
		m.promMessage = ""
		m.exportMessage = ""
		m.updatePromLink()
		m.correlationMessage = ""
		m.updateCorrelationLink()
		m.traceLinks = nil
		m.traceLinkPath = ""
		if m.report.Tracing != nil {
			links := output.ResolveTraceLinks(m.report.Tracing)
			if len(links) > 0 {
				m.traceLinks = links
				if path, err := writeTraceLinksFile(links); err == nil {
					m.traceLinkPath = path
				}
			}
		}
		// Always update the active view's content after a refresh
		m.content = m.renderCurrentView()
		return m, nil
	
	case filterResultMsg:
		m.loadingServices = false
		m.loadingEndpoints = false
		if msg.err != nil {
			if m.showServices {
				m.serviceMessage = "Filter failed: " + msg.err.Error()
				m.content = m.renderServices()
			} else if m.showEndpoints {
				m.endpointMessage = "Filter failed: " + msg.err.Error()
				m.content = m.renderEndpoints()
			}
			return m, nil
		}
		
		if msg.report.APILatency != nil {
			m.applyFilteredLatency(msg.report.APILatency)
		}

		if m.showServices {
			m.serviceMessage = msg.message
			m.content = m.renderServices()
		} else if m.showEndpoints {
			m.endpointMessage = msg.message
			m.content = m.renderEndpoints()
		}
		return m, nil

	case logsMsg:
		m.logLoading = false
		if strings.TrimSpace(msg.message) != "" {
			m.logMessage = msg.message
		} else {
			m.logMessage = ""
		}
		m.logEntries = msg.entries
		m.logQuery = msg.query
		m.logAutoNote = msg.note
		m.logLinkPath = ""
		m.logLink = ""
		if m.logQuery != "" {
			cfg, _ := config.Load()
			lokiUID, lokiType := m.resolveGrafanaDatasource(cfg, cfg.GrafanaLokiDataSource)
			if strings.TrimSpace(lokiType) != "" && !strings.EqualFold(lokiType, "loki") {
				m.logAutoNote = joinNotesInline(m.logAutoNote, []string{"Grafana Loki datasource not configured; set GRAFANA_LOKI_DS"})
			}
			link := output.BuildGrafanaExploreURL(cfg.GrafanaURL, cfg.GrafanaLokiDataSource, lokiUID, lokiType, m.logQuery, "now-"+cfg.LokiWindow, "now")
			if link != "" {
				m.logLink = link
				if path, err := writeGrafanaLinkFile("loki", link); err == nil {
					m.logLinkPath = path
				}
			}
		}
		if m.showLogs {
			m.content = m.renderLogs()
		}
		return m, nil

	case incidentListMsg:
		if msg.message != "" {
			m.incidentMessage = msg.message
		} else {
			m.incidentMessage = ""
		}
		m.activeIncident = msg.active
		m.incidentList = msg.list
		m.incidentWarnings = msg.warnings
		m.applyIncidentFilters()
		if m.showIncidents {
			m.content = m.renderIncidents()
		}
		return m, nil

	case tea.KeyMsg:
		if m.editActive {
			return m.updateEdit(msg)
		}
		if m.authEditActive {
			return m.updateAuthEditor(msg)
		}
		if m.showPromConfig {
			return m.updatePromEditor(msg)
		}
		if m.showCorrelationConfig {
			return m.updateCorrelationEditor(msg)
		}
		if m.showBackendSelector {
			return m.updateBackendSelector(msg)
		}
		if m.showElasticConfig {
			return m.updateElasticEditor(msg)
		}
		if m.showLokiConfig {
			return m.updateLokiEditor(msg)
		}
		if m.showGrafanaConfig {
			return m.updateGrafanaEditor(msg)
		}
		if m.showTempoConfig {
			return m.updateTempoEditor(msg)
		}
		if m.showJaegerConfig {
			return m.updateJaegerEditor(msg)
		}
		if m.showServices {
			return m.updateServiceSelection(msg)
		}
		if m.showEndpoints {
			return m.updateEndpointSelection(msg)
		}
		// Handle incident browser navigation
		if m.showIncidents {
			handled := m.handleIncidentBrowserKeys(msg.String())
			if handled {
				m.content = m.renderIncidents()
				return m, nil
			}
		}
		switch msg.String() {
		case "q", "Q", "ctrl+c":
			return m, tea.Quit
		case "esc":
			if m.showHelp || m.showIncidents || m.showFlows || m.showConfig || m.showServices || m.showEndpoints || m.showLogs || m.showClusterHealth || m.showServiceDrilldown || m.showPromConfig || m.showLokiConfig || m.showGrafanaConfig || m.showTempoConfig || m.showJaegerConfig || m.showCorrelationConfig || m.showBackendSelector || m.showElasticConfig {
				if m.showServiceDrilldown && m.showServiceDrilldownDetail {
					m.showServiceDrilldownDetail = false
					// Do NOT reset scrollY; keep the user at their list position
					m.content = m.renderServiceDrilldown()
					return m, nil
				} else {
					m.showHelp = false
					m.showIncidents = false
					m.showFlows = false
					m.showConfig = false
					m.showServices = false
					m.showEndpoints = false
					m.showLogs = false
					m.showClusterHealth = false
					m.showServiceDrilldown = false
					m.showPromConfig = false
					m.showLokiConfig = false
					m.showGrafanaConfig = false
					m.showTempoConfig = false
					m.showJaegerConfig = false
					m.showCorrelationConfig = false
					m.showBackendSelector = false
					m.showElasticConfig = false
					m.scrollY = 0
					m.content = m.renderContent()
					return m, nil
				}
			}
			return m, tea.Quit
		case "E":
			mdPath, jsonPath, err := exportReportSnapshot(m.report)
			if err != nil {
				m.exportMessage = "Export failed: " + err.Error()
			} else {
				m.exportMessage = fmt.Sprintf("Report exported: %s, %s", mdPath, jsonPath)
			}
			m.content = m.renderContent()
			return m, nil
		case "M":
			path, err := exportReportSingle(m.report, "md")
			if err != nil {
				m.exportMessage = "Export failed: " + err.Error()
			} else {
				m.exportMessage = "Report exported: " + path
			}
			m.content = m.renderContent()
			return m, nil
		case "D":
			// Debug visibility is disabled/removed as per user request
			m.content = m.renderContent()
			return m, nil
		case "J":
			path, err := exportReportSingle(m.report, "json")
			if err != nil {
				m.exportMessage = "Export failed: " + err.Error()
			} else {
				m.exportMessage = "Report exported: " + path
			}
			m.content = m.renderContent()
			return m, nil
		case "y", "Y":
			m.updateCorrelationLink()
			if strings.TrimSpace(m.correlationLink) != "" {
				if err := copyToClipboard(m.correlationLink); err != nil {
					if m.correlationLinkPath != "" {
						m.correlationMessage = "Clipboard copy failed. Link saved: " + m.correlationLinkPath
					} else {
						m.correlationMessage = "Clipboard copy failed: " + err.Error()
					}
				} else {
					m.correlationMessage = "Correlation link copied to clipboard."
				}
				m.content = m.renderContent()
				return m, nil
			}
			m.updatePromLink()
			if strings.TrimSpace(m.promLink) == "" {
				m.promMessage = "Grafana link not ready yet."
				m.content = m.renderContent()
				return m, nil
			}
			if err := copyToClipboard(m.promLink); err != nil {
				if m.promLinkPath != "" {
					m.promMessage = "Clipboard copy failed. Link saved: " + m.promLinkPath
				} else {
					m.promMessage = "Clipboard copy failed: " + err.Error()
				}
			} else {
				m.promMessage = "Grafana link copied to clipboard."
			}
			m.content = m.renderContent()
			return m, nil
		case "u", "U":
			if m.report.InfraStatus == nil {
				return m, nil
			}
			m.showClusterHealth = !m.showClusterHealth
			m.showServiceDrilldown = false
			m.showHelp = false
			m.showIncidents = false
			m.showFlows = false
			m.showConfig = false
			m.editActive = false
			m.showServices = false
			m.showEndpoints = false
			m.showLogs = false
			m.showLokiConfig = false
			m.showGrafanaConfig = false
			m.authEditActive = false
			m.scrollY = 0
			if m.showClusterHealth {
				m.content = m.renderClusterHealth()
			} else {
				m.content = m.renderContent()
			}
			return m, nil
		case "p", "P":
			m.showServiceDrilldown = !m.showServiceDrilldown
			m.showClusterHealth = false
			m.showHelp = false
			m.showIncidents = false
			m.showFlows = false
			m.showConfig = false
			m.editActive = false
			m.showServices = false
			m.showEndpoints = false
			m.showLogs = false
			m.showLokiConfig = false
			m.showGrafanaConfig = false
			m.authEditActive = false
			m.scrollY = 0
			if m.showServiceDrilldown {
				m.content = m.renderServiceDrilldown()
			} else {
				m.content = m.renderContent()
			}
			return m, nil
		case "h", "H":
			m.showHelp = !m.showHelp
			m.showIncidents = false
			m.showFlows = false
			m.showConfig = false
			m.editActive = false
			m.showServices = false
			m.showEndpoints = false
			m.showLogs = false
			m.showLokiConfig = false
			m.showGrafanaConfig = false
			m.authEditActive = false
			m.scrollY = 0
			if m.showHelp {
				m.content = m.renderHelp()
			} else if m.showConfig {
				m.content = m.renderConfig()
			} else if m.showLogs {
				m.content = m.renderLogs()
			} else if m.showIncidents {
				m.content = m.renderIncidents()
			} else if m.showFlows {
				m.content = m.renderFlows()
			} else {
				m.content = m.renderContent()
			}
			return m, nil
		case "i", "I":
			m.showIncidents = !m.showIncidents
			m.showHelp = false
			m.showFlows = false
			m.showConfig = false
			m.editActive = false
			m.showServices = false
			m.showEndpoints = false
			m.showLogs = false
			m.showLokiConfig = false
			m.showGrafanaConfig = false
			m.showPromConfig = false
			m.showTempoConfig = false
			m.showJaegerConfig = false
			m.showCorrelationConfig = false
			m.showBackendSelector = false
			m.showElasticConfig = false
			m.authEditActive = false
			m.scrollY = 0
			if m.showIncidents {
				m.loadIncidents()
				m.content = m.renderIncidents()
			} else {
				m.content = m.renderContent()
			}
			return m, nil
		case "f", "F":
			m.showFlows = !m.showFlows
			m.showHelp = false
			m.showIncidents = false
			m.showConfig = false
			m.editActive = false
			m.showServices = false
			m.showEndpoints = false
			m.showLogs = false
			m.showLokiConfig = false
			m.showGrafanaConfig = false
			m.showPromConfig = false
			m.showTempoConfig = false
			m.showJaegerConfig = false
			m.showCorrelationConfig = false
			m.showBackendSelector = false
			m.showElasticConfig = false
			m.authEditActive = false
			m.scrollY = 0
			if m.showFlows {
				m.loadFlows()
				m.content = m.renderFlows()
			} else {
				m.content = m.renderContent()
			}
			return m, nil
		case "s", "S":
			m.showServices = !m.showServices
			m.showHelp = false
			m.showIncidents = false
			m.showFlows = false
			m.showConfig = false
			m.editActive = false
			m.showEndpoints = false
			m.showLogs = false
			m.showLokiConfig = false
			m.showGrafanaConfig = false
			m.showPromConfig = false
			m.showTempoConfig = false
			m.showJaegerConfig = false
			m.showCorrelationConfig = false
			m.showBackendSelector = false
			m.showElasticConfig = false
			m.authEditActive = false
			m.scrollY = 0
			if m.showServices {
				m.serviceIndex = 0
				m.serviceList = extractServices(m.baseTopServicesRPS, m.report.APILatency)
				m.serviceMessage = ""
				if len(m.serviceList) <= 1 {
					m.loadingServices = true
					m.serviceMessage = "Loading services from Prometheus..."
					m.content = m.renderServices()
					return m, tea.Batch(func() tea.Msg {
						list, listMsg := m.fetchServiceListFromProm()
						return serviceListMsg{list: list, message: listMsg}
					}, m.spinner.Tick)
				}
				m.content = m.renderServices()
			} else {
				m.content = m.renderContent()
			}
			return m, nil
		case "r", "R":
			m.showEndpoints = !m.showEndpoints
			m.showHelp = false
			m.showIncidents = false
			m.showFlows = false
			m.showConfig = false
			m.editActive = false
			m.showServices = false
			m.showLogs = false
			m.showLokiConfig = false
			m.showGrafanaConfig = false
			m.showPromConfig = false
			m.showTempoConfig = false
			m.showJaegerConfig = false
			m.showCorrelationConfig = false
			m.showBackendSelector = false
			m.showElasticConfig = false
			m.authEditActive = false
			m.scrollY = 0
			if m.showEndpoints {
				m.endpointIndex = 0
				m.endpointList = extractEndpoints(m.baseTopEndpointsRPS, m.baseTopEndpoints, m.report.APILatency)
				if len(m.endpointList) <= 1 && m.report.APILatency != nil {
					if len(m.report.APILatency.TopEndpointsRPS) > 0 {
						m.baseTopEndpointsRPS = m.report.APILatency.TopEndpointsRPS
					}
					if len(m.report.APILatency.TopEndpoints) > 0 {
						m.baseTopEndpoints = m.report.APILatency.TopEndpoints
					}
					m.endpointList = extractEndpoints(m.baseTopEndpointsRPS, m.baseTopEndpoints, m.report.APILatency)
				}
				m.endpointMessage = ""
				if len(m.endpointList) <= 1 {
					m.loadingEndpoints = true
					m.endpointMessage = "Loading endpoints from Prometheus..."
					m.content = m.renderEndpoints()
					return m, tea.Batch(func() tea.Msg {
						list, listMsg := m.fetchEndpointListFromProm()
						return endpointListMsg{list: list, message: listMsg}
					}, m.spinner.Tick)
				}
				m.content = m.renderEndpoints()
			} else {
				m.content = m.renderContent()
			}
			return m, nil
		case "c", "C":
			m.showHelp = false
			m.showIncidents = false
			m.showFlows = false
			m.showConfig = false
			m.editActive = false
			m.showServices = false
			m.showEndpoints = false
			m.showLogs = false
			m.showLokiConfig = false
			m.showGrafanaConfig = false
			m.showTempoConfig = false
			m.showJaegerConfig = false
			m.showCorrelationConfig = false
			m.showBackendSelector = false
			m.showElasticConfig = false
			m.loadingRefresh = false
			m.loadingServices = false
			m.loadingEndpoints = false
			m.scrollY = 0
			m.authEditActive = false
			if !m.showPromConfig {
				m.startPromEditor()
			}
			m.content = m.renderPromEditor()
			return m, nil
		case "z", "Z":
			m.showHelp = false
			m.showIncidents = false
			m.showFlows = false
			m.showConfig = false
			m.editActive = false
			m.showServices = false
			m.showEndpoints = false
			m.showLogs = false
			m.showLokiConfig = false
			m.showGrafanaConfig = false
			m.showPromConfig = false
			m.showTempoConfig = false
			m.showJaegerConfig = false
			m.showBackendSelector = false
			m.showElasticConfig = false
			m.authEditActive = false
			m.scrollY = 0
			if !m.showCorrelationConfig {
				m.startCorrelationEditor()
			}
			m.content = m.renderCorrelationEditor()
			return m, nil
		case "b", "B":
			m.showHelp = false
			m.showIncidents = false
			m.showFlows = false
			m.showConfig = false
			m.editActive = false
			m.showServices = false
			m.showEndpoints = false
			m.showLogs = false
			m.showLokiConfig = false
			m.showGrafanaConfig = false
			m.showPromConfig = false
			m.showTempoConfig = false
			m.showJaegerConfig = false
			m.showCorrelationConfig = false
			m.showElasticConfig = false
			m.authEditActive = false
			m.scrollY = 0
			if !m.showBackendSelector {
				m.startBackendSelector()
			}
			m.content = m.renderBackendSelector()
			return m, nil
		case "e":
			m.showHelp = false
			m.showIncidents = false
			m.showFlows = false
			m.showConfig = false
			m.editActive = false
			m.showServices = false
			m.showEndpoints = false
			m.showLogs = false
			m.showLokiConfig = false
			m.showGrafanaConfig = false
			m.showPromConfig = false
			m.showTempoConfig = false
			m.showJaegerConfig = false
			m.showCorrelationConfig = false
			m.showBackendSelector = false
			m.authEditActive = false
			m.scrollY = 0
			if !m.showElasticConfig {
				m.startElasticEditor()
			}
			m.content = m.renderElasticEditor()
			return m, nil
		case "t", "T":
			m.showHelp = false
			m.showIncidents = false
			m.showFlows = false
			m.showConfig = false
			m.editActive = false
			m.showServices = false
			m.showEndpoints = false
			m.showLogs = false
			m.showLokiConfig = false
			m.showGrafanaConfig = false
			m.showPromConfig = false
			m.showJaegerConfig = false
			m.showCorrelationConfig = false
			m.showBackendSelector = false
			m.showElasticConfig = false
			m.authEditActive = false
			m.scrollY = 0
			if !m.showTempoConfig {
				m.startTempoEditor()
			}
			m.content = m.renderTempoEditor()
			return m, nil
		case "ctrl+j":
			m.showHelp = false
			m.showIncidents = false
			m.showFlows = false
			m.showConfig = false
			m.editActive = false
			m.showServices = false
			m.showEndpoints = false
			m.showLogs = false
			m.showLokiConfig = false
			m.showGrafanaConfig = false
			m.showPromConfig = false
			m.showTempoConfig = false
			m.showCorrelationConfig = false
			m.showBackendSelector = false
			m.showElasticConfig = false
			m.authEditActive = false
			m.scrollY = 0
			if !m.showJaegerConfig {
				m.startJaegerEditor()
			}
			m.content = m.renderJaegerEditor()
			return m, nil
		case "x", "X":
			if err := resetInvalidPromOverrides(); err != nil {
				m.promMessage = "Reset overrides failed: " + err.Error()
			} else {
				m.promMessage = "Reset invalid overrides. Auto-discover enabled."
				m.updatePromLink()
			}
			m.content = m.renderContent()
			return m, nil
		case "o", "O":
			m.showHelp = false
			m.showIncidents = false
			m.showFlows = false
			m.showConfig = false
			m.editActive = false
			m.showServices = false
			m.showEndpoints = false
			m.showLogs = false
			m.authEditActive = false
			m.showGrafanaConfig = false
			m.showPromConfig = false
			m.showTempoConfig = false
			m.showJaegerConfig = false
			m.showCorrelationConfig = false
			m.showBackendSelector = false
			m.showElasticConfig = false
			m.loadingRefresh = false
			m.loadingServices = false
			m.loadingEndpoints = false
			m.scrollY = 0
			if !m.showLokiConfig {
				m.startLokiEditor()
			}
			m.content = m.renderLokiEditor()
			return m, nil
		case "v", "V":
			m.showHelp = false
			m.showIncidents = false
			m.showFlows = false
			m.showConfig = false
			m.editActive = false
			m.showServices = false
			m.showEndpoints = false
			m.showLogs = false
			m.showLokiConfig = false
			m.authEditActive = false
			m.showPromConfig = false
			m.showTempoConfig = false
			m.showJaegerConfig = false
			m.showCorrelationConfig = false
			m.showBackendSelector = false
			m.showElasticConfig = false
			m.loadingRefresh = false
			m.loadingServices = false
			m.loadingEndpoints = false
			m.scrollY = 0
			if !m.showGrafanaConfig {
				m.startGrafanaEditor()
			}
			m.content = m.renderGrafanaEditor()
			return m, nil
		case "l", "L":
			return m.startLogsView()
		case "enter":
			if m.showServiceDrilldown && !m.showServiceDrilldownDetail {
				if len(m.report.AllServices) > 0 {
					svc := m.report.AllServices[m.serviceDrilldownIndex]
					m.selectedDrilldownService = svc.Name
					m.showServiceDrilldownDetail = true
					m.report.ServiceDrilldown = nil
					m.scrollY = 0 // Start detail view at the top
					m.content = m.renderServiceDrilldown()
					// Trigger immediate data refresh for the selected service
					return m, m.refreshAPILatencyCmd()
				}
				return m, nil
			}
		case "up", "k":
			if m.showServiceDrilldown && !m.showServiceDrilldownDetail {
				if m.serviceDrilldownIndex > 0 {
					m.serviceDrilldownIndex--
				}
				// Auto-scroll: keep selection visible (top)
				// Visible window is roughly m.height - 8 lines for the list
				if m.serviceDrilldownIndex < m.scrollY {
					m.scrollY = m.serviceDrilldownIndex
				}
				m.content = m.renderServiceDrilldown()
				return m, nil
			}
			// General scrolling up
			if m.scrollY > 0 {
				m.scrollY--
			}
			m.content = m.renderCurrentView()
			return m, nil
		case "down", "j":
			if m.showServiceDrilldown && !m.showServiceDrilldownDetail {
				if m.serviceDrilldownIndex < len(m.report.AllServices)-1 {
					m.serviceDrilldownIndex++
				}
				// Auto-scroll: keep selection visible (bottom)
				windowSize := m.height - 10
				if windowSize < 5 { windowSize = 5 }
				if m.serviceDrilldownIndex >= m.scrollY + windowSize {
					m.scrollY = m.serviceDrilldownIndex - windowSize + 1
				}
				m.content = m.renderServiceDrilldown()
				return m, nil
			}
			// General scrolling down
			m.scrollY++
			m.content = m.renderCurrentView()
			return m, nil
		case "pgup":
			step := m.height / 2
			if step < 1 {
				step = 1
			}
			m.scrollY -= step
			if m.scrollY < 0 {
				m.scrollY = 0
			}
			return m, nil
		case "pgdown":
			step := m.height / 2
			if step < 1 {
				step = 1
			}
			contentLines := strings.Count(m.content, "\n")
			maxScroll := contentLines - m.height + 1
			if maxScroll > 0 {
				m.scrollY += step
				if m.scrollY > maxScroll {
					m.scrollY = maxScroll
				}
			}
			return m, nil
		case "home", "g":
			m.scrollY = 0
			return m, nil
		case "end", "G":
			contentLines := strings.Count(m.content, "\n")
			maxScroll := contentLines - m.height + 1
			if maxScroll > 0 {
				m.scrollY = maxScroll
			}
			return m, nil
		}
	}

	return m, nil
}

func (m tuiModel) View() string {
	if !m.ready {
		// Fall back to rendering content even if no window size is received.
		content := m.renderContent()
		if strings.TrimSpace(content) == "" {
			return "Initializing..."
		}
		return content + "\n" + footerStyle.Render(m.footerText())
	}

	// Always render the live auth editor to avoid stale content.
	if m.authEditActive {
		m.content = m.renderAuthEditor()
	}
	if m.editActive {
		m.content = m.renderEdit()
	}

	// Calculate footer height dynamically to reserve space
	footerStr := footerStyle.Render(m.footerText())
	footerHeight := lipgloss.Height(footerStr)

	// Get visible portion of content
	lines := strings.Split(m.content, "\n")

	// Reserve space for footer (blank line + actual footer height)
	visibleLines := m.height - footerHeight - 1
	if visibleLines <= 0 {
		visibleLines = 1
	}

	start := m.scrollY
	end := start + visibleLines
	if end > len(lines) {
		end = len(lines)
	}
	if start < 0 {
		start = 0
	}

	visibleContent := strings.Join(lines[start:end], "\n")

	// Add footer with proper spacing
	return visibleContent + "\n" + footerStr
}

func (m tuiModel) footerText() string {
	var footer string
	if m.report.IsDemo {
		if m.showIncidents && !m.incidentBrowserMode {
			footer = "Press '1' details • '2' runbook • '3' postmortem • 'esc' back • 'q' exit"
		} else {
			footer = "Press 'h' help • Press 'i' incidents • Press 'f' flows • Press 'l' logs • Press 'q' exit"
		}
	} else {
		// Rich footer for regular mode showing available navigation
		infraPart := ""
		if m.report.InfraStatus != nil {
			infraPart = " 'u' cluster •"
		}
		footer = fmt.Sprintf("Press 'h' help •%s 'p' perf • 'j/k' scroll • 'i' incidents • 'f' flows • 's' services • 'r' routes • 'b' backends • 'l' logs • 't' traces • 'c' config • 'v' grafana • 'D' debug • 'ctrl+j' jaeger • 'q' exit", infraPart)
	}

	extras := []string{}
	if m.loadingServices || m.loadingEndpoints {
		extras = append(extras, "Loading "+m.spinner.View())
	} else if m.loadingRefresh {
		extras = append(extras, "Refreshing "+m.spinner.View())
	} else if !m.lastRefresh.IsZero() {
		extras = append(extras, "Last refresh "+m.lastRefresh.Format("15:04:05"))
	}
	if len(extras) > 0 {
		footer += " • " + strings.Join(extras, " • ")
	}
	return footer
}

func (m *tuiModel) renderCurrentView() string {
	if m.showHelp {
		return m.renderHelp()
	} else if m.showIncidents {
		return m.renderIncidents()
	} else if m.showFlows {
		return m.renderFlows()
	} else if m.showClusterHealth {
		return m.renderClusterHealth()
	} else if m.showServiceDrilldown {
		return m.renderServiceDrilldown()
	} else if m.showServices {
		return m.renderServices()
	} else if m.showEndpoints {
		return m.renderEndpoints()
	} else if m.showLogs {
		return m.renderLogs()
	} else if m.showConfig {
		return m.renderEdit()
	} else if m.showLokiConfig {
		return m.renderLokiEditor()
	} else if m.showPromConfig {
		return m.renderPromEditor()
	} else if m.showGrafanaConfig {
		return m.renderGrafanaEditor()
	} else if m.showTempoConfig {
		return m.renderTempoEditor()
	} else if m.showJaegerConfig {
		return m.renderJaegerEditor()
	} else if m.showCorrelationConfig {
		return m.renderCorrelationEditor()
	} else if m.showBackendSelector {
		return m.renderBackendSelector()
	} else if m.showElasticConfig {
		return m.renderElasticEditor()
	}
	return m.renderContent()
}

func (m *tuiModel) renderContent() string {
	var sb strings.Builder

	// Title
	sb.WriteString("\n")
	sb.WriteString(titleStyle.Render("Server Health Check"))
	sb.WriteString("\n")
	if m.report.IsDemo {
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true).Align(lipgloss.Center).Width(80).Render("[ DEMO MODE ACTIVE ]"))
		sb.WriteString("\n")
	}
	sb.WriteString(disclaimerStyle.Render(fmt.Sprintf("Active Profile: %s", config.GetActiveProfileName())))
	sb.WriteString("\n")
	sb.WriteString(disclaimerStyle.Render("These suggestions are for informational purposes only. Please verify all details before proceeding."))
	sb.WriteString("\n\n")

	// Legend
	sb.WriteString(legendStyle.Render(
		green.Render("SAFE") + " = OK   " +
			yellow.Render("CHECK") + " = Needs review   " +
			red.Render("RISK") + " = Action required",
	))
	sb.WriteString("\n\n")

	// Summary Cards
	var cards []string
	for _, s := range m.report.Summary {
		card := lipgloss.NewStyle().
			Width(18).
			Align(lipgloss.Center).
			Render(
				lipgloss.NewStyle().Bold(true).Render(s.Name) + "\n" +
					statusStyle(s.Status).Render(string(s.Status)) + "\n" +
					s.Value,
			)
		cards = append(cards, panelStyle.Render(card))
	}
	sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, cards...))
	sb.WriteString("\n\n")

	// Metrics
	sb.WriteString(markdownBlock("## 📊 Metrics"))
	for _, metric := range m.report.Metrics {
		dots := strings.Repeat(".", 30-len(metric.Name))
		sb.WriteString(fmt.Sprintf(
			"%s %s %s\n",
			keyStyle.Render(metric.Name),
			dimStyle.Render(dots),
			valueStyle.Render(metric.Value),
		))
	}

	// Infrastructure Health (K8s/EKS)
	if m.report.InfraStatus != nil {
		sb.WriteString("\n")
		sb.WriteString(markdownBlock("## 🛡️ Infrastructure Health"))
		
		infra := m.report.InfraStatus
		if len(infra.NoDataReasons) > 0 {
			for _, r := range infra.NoDataReasons {
				sb.WriteString(red.Render("  ❌ " + r) + "\n")
			}
		} else {
			// Summary Cards
			clusterStatus := model.SAFE
			if infra.NodesReady < infra.NodesTotal || infra.PodsReady < infra.PodsTotal {
				clusterStatus = model.RISK
			}

			cards := []string{
				infoCard("CLUSTER", valueStyle.Render(infra.ClusterName)),
				infoCard("NODES", valueStyle.Render(fmt.Sprintf("%d/%d", infra.NodesReady, infra.NodesTotal))),
				infoCard("PODS", statusStyle(clusterStatus).Render(fmt.Sprintf("%d/%d", infra.PodsReady, infra.PodsTotal))),
				infoCard("RESTARTS", valueStyle.Render(fmt.Sprintf("%d", infra.Restarts))),
			}
			sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, cards...))
			sb.WriteString("\n\n")

			// Workload Health Table
			if len(infra.NamespaceHealth) > 0 {
				sb.WriteString(markdownBlock("### Namespace Health"))
				headers := []string{"Namespace", "Ready", "Restarts", "Status"}
				rows := [][]string{}
				for _, ns := range infra.NamespaceHealth {
					rows = append(rows, []string{
						ns.Name,
						fmt.Sprintf("%d/%d", ns.PodsReady, ns.PodsTotal),
						fmt.Sprintf("%d", ns.Restarts),
						string(ns.Status),
					})
				}
				sb.WriteString(renderTable(headers, rows))
				sb.WriteString("\n")
			}

			if len(infra.WorkloadHealth) > 0 {
				sb.WriteString(markdownBlock("### Top Workloads"))
				headers := []string{"Workload", "Namespace", "Ready", "Status"}
				rows := [][]string{}
				// Limit to top 5 or 10 workloads
				limit := 10
				if len(infra.WorkloadHealth) < limit {
					limit = len(infra.WorkloadHealth)
				}
				for i := 0; i < limit; i++ {
					w := infra.WorkloadHealth[i]
					rows = append(rows, []string{
						w.Name,
						w.Namespace,
						fmt.Sprintf("%d/%d", w.Ready, w.Desired),
						string(w.Status),
					})
				}
				sb.WriteString(renderTable(headers, rows))
				sb.WriteString("\n")
			}
		}
	}

	// API Latency (Prometheus)
	sb.WriteString("\n")
	sb.WriteString(markdownBlock("## ⏱️ API Latency (Prometheus)"))
	if m.report.APIConfig != nil {
		url := output.EmptyOr(m.report.APIConfig.PrometheusURL, "not set")
		sb.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("  Prometheus URL"), valueStyle.Render(url)))
	}
	if m.report.APINote != "" {
		sb.WriteString(dimStyle.Render("  Note: " + m.report.APINote))
		sb.WriteString("\n")
	} else if m.report.APILatency == nil {
		if m.loadingRefresh {
			sb.WriteString(dimStyle.Render("  🔍 Scanning and collecting observability data (this may take a moment)..."))
		} else {
			sb.WriteString(dimStyle.Render("  Data not available (no service/route configured or Prometheus unreachable)"))
		}
		sb.WriteString("\n")
	} else {
			if stale := findStaleDataReason(m.report.APILatency.NoDataReasons); stale != "" {
				sb.WriteString(dimStyle.Render("Last sample age: " + stale))
				sb.WriteString("\n")
			}
			route := m.report.APILatency.Route
			if route == "" {
				route = "all routes"
			}
			service := m.report.APILatency.Service
			if strings.TrimSpace(service) == "" {
				service = "all services"
			}
			cfg, _ := config.Load()
			cfg = mergeConfigFromSummary(cfg, m.report.APIConfig)
			if cfg.AutoDiscover {
				sb.WriteString(dimStyle.Render("Auto-detect: enabled"))
				sb.WriteString("\n")
			}
			statusValue := statusStyle(m.report.APILatency.Status).Render(string(m.report.APILatency.Status))
			rpsValue := valueStyle.Render(fmt.Sprintf("%.2f", m.report.APILatency.RPS))
			cardsRow1 := []string{
				infoCard("SERVICE", valueStyle.Render(service)),
				infoCard("ROUTE", valueStyle.Render(route)),
				infoCard("STATUS", statusValue),
				infoCard("RPS", rpsValue),
			}
			sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, cardsRow1...))
			sb.WriteString("\n")

			cardsRow2 := []string{
				infoCard("P90", valueStyle.Render(output.FormatLatency(m.report.APILatency.P90))),
				infoCard("P95", valueStyle.Render(output.FormatLatency(m.report.APILatency.P95))),
				infoCard("P99", valueStyle.Render(output.FormatLatency(m.report.APILatency.P99))),
				infoCard("ERROR", valueStyle.Render(output.FormatPercent(m.report.APILatency.ErrorRate))),
			}
			sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, cardsRow2...))
			sb.WriteString("\n")

			confidence := m.report.APILatency.Confidence
			if strings.TrimSpace(confidence) == "" {
				confidence = "unknown"
			}
			cardsRow3 := []string{
				infoCard("4XX", valueStyle.Render(output.FormatPercent(m.report.APILatency.ClientErrorRate))),
				infoCard("5XX", valueStyle.Render(output.FormatPercent(m.report.APILatency.ServerErrorRate))),
				infoCard("WINDOW", valueStyle.Render(m.report.APILatency.Window)),
				infoCard("CONF", valueStyle.Render(confidence)),
			}
			sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, cardsRow3...))
			sb.WriteString("\n")

			sb.WriteString(dimStyle.Render("P90/P95/P99 = latency percentiles (90%, 95%, 99% of requests at or below)"))
			sb.WriteString("\n")
			sb.WriteString(dimStyle.Render("Error Rate = overall errors • 4xx = client errors • 5xx = server errors • RPS = requests/sec"))
			sb.WriteString("\n")
			if strings.TrimSpace(m.report.APILatency.ErrorLabel) != "" {
				sb.WriteString(dimStyle.Render("Error label: " + m.report.APILatency.ErrorLabel))
				sb.WriteString("\n")
			}
			if m.report.APILatency.Note != "" {
				sb.WriteString(markdownBlock("### Auto-Detect Reasoning"))
				for _, line := range wrapNoteLines(m.report.APILatency.Note) {
					sb.WriteString(dimStyle.Render("  " + line))
					sb.WriteString("\n")
				}
			}
			if len(m.report.APILatency.NoDataReasons) > 0 {
				sb.WriteString("\n")
				sb.WriteString(markdownBlock("### Data Gaps"))
				for _, reason := range m.report.APILatency.NoDataReasons {
					line := fmt.Sprintf("%s: %s", reason.Area, reason.Reason)
					if strings.TrimSpace(reason.SuggestedFix) != "" {
						line += " (Fix: " + reason.SuggestedFix + ")"
					}
					sb.WriteString("- " + line + "\n")
				}
			}
			if warnings := extractCardinalityWarnings(m.report.APILatency.Note); len(warnings) > 0 {
				sb.WriteString("\n")
				sb.WriteString(markdownBlock("### Warnings"))
				for _, warning := range warnings {
					sb.WriteString("- " + warning + "\n")
				}
			}
			if !m.report.IsDemo {
				if issues := buildConfigHealth(cfg); len(issues) > 0 {
					sb.WriteString("\n")
					sb.WriteString(markdownBlock("### Config Health"))
					for _, issue := range issues {
						sb.WriteString("- " + issue + "\n")
					}
				}
			}
			if !m.report.IsDemo {
				if len(m.report.APILatency.TopServicesRPS) > 0 {
					sb.WriteString(dimStyle.Render("Tip: press 's' to view services and set API_SERVICE"))
					sb.WriteString("\n")
				}
				if len(m.report.APILatency.TopEndpointsRPS) > 0 || len(m.report.APILatency.TopEndpoints) > 0 {
					sb.WriteString(dimStyle.Render("Tip: press 'r' to view endpoints and set API_ROUTE"))
					sb.WriteString("\n")
				}
			}
			sb.WriteString("\n")
			sb.WriteString(markdownBlock("### Trend vs Baseline"))
			scopeService := service
			scopeRoute := route
			sb.WriteString(dimStyle.Render(fmt.Sprintf("Scope: service=%s, route=%s", scopeService, scopeRoute)))
			sb.WriteString("\n")
			sb.WriteString(dimStyle.Render("Baseline compares current window vs historical window (window × 6, clamped 30m–6h)."))
			sb.WriteString("\n")
			baselineLabel := "Baseline"
			if strings.TrimSpace(m.report.APILatency.BaselineWindow) != "" {
				baselineLabel = "Baseline " + m.report.APILatency.BaselineWindow
			}
			rows := [][]string{
				{"P95", output.FormatLatency(m.report.APILatency.P95), output.FormatLatency(m.report.APILatency.BaselineP95), output.FormatDeltaLatency(m.report.APILatency.DeltaP95)},
				{"Error Rate", output.FormatPercent(m.report.APILatency.ErrorRate), output.FormatPercent(m.report.APILatency.BaselineErrorRate), output.FormatDeltaPercent(m.report.APILatency.DeltaErrorRate)},
				{"RPS", fmt.Sprintf("%.2f", m.report.APILatency.RPS), fmt.Sprintf("%.2f", m.report.APILatency.BaselineRPS), output.FormatDeltaNumber(m.report.APILatency.DeltaRPS)},
			}
			sb.WriteString(renderTable([]string{"Metric", "Now", baselineLabel, "Delta"}, rows))
			sb.WriteString("\n")
			promLink := m.promLink
			if strings.TrimSpace(promLink) == "" {
				promUID, promType := m.resolveGrafanaDatasource(cfg, cfg.GrafanaPromDataSource)
				promLink = output.BuildGrafanaExploreURL(cfg.GrafanaURL, cfg.GrafanaPromDataSource, promUID, promType, output.PromP95Query(m.report.APILatency), "now-"+m.report.APILatency.Window, "now")
			}
			if promLink != "" {
				sb.WriteString(dimStyle.Render("Grafana link (Prometheus Explore):"))
				sb.WriteString("\n")
				sb.WriteString(renderWrappedLink(promLink, m.width))
				sb.WriteString("\n")
				if strings.TrimSpace(m.promLinkPath) != "" {
					sb.WriteString(dimStyle.Render("Saved Grafana link: " + m.promLinkPath))
					sb.WriteString("\n")
				}
				sb.WriteString(dimStyle.Render("Press 'y' to copy Grafana link."))
				sb.WriteString("\n")
			} else {
				sb.WriteString(dimStyle.Render("Grafana link-out: set Grafana URL/datasource (press 'v')"))
				sb.WriteString("\n")
			}
			if strings.TrimSpace(m.promMessage) != "" {
				sb.WriteString(dimStyle.Render(m.promMessage))
				sb.WriteString("\n")
			}
			if strings.TrimSpace(m.exportMessage) != "" {
				sb.WriteString(dimStyle.Render(m.exportMessage))
				sb.WriteString("\n")
			}
			if strings.TrimSpace(m.report.APILatency.SpikeNote) != "" {
				sb.WriteString(dimStyle.Render("Spike: " + m.report.APILatency.SpikeNote))
				sb.WriteString("\n")
			}
			if len(m.report.APILatency.TopEndpointRegressions) > 0 {
				sb.WriteString("\n")
				sb.WriteString(markdownBlock("### Top Endpoint Regressions (P95)"))
				if strings.TrimSpace(m.report.APILatency.RegressionNote) != "" {
					sb.WriteString(dimStyle.Render(m.report.APILatency.RegressionNote))
					sb.WriteString("\n")
				}
				rows := make([][]string, 0, len(m.report.APILatency.TopEndpointRegressions))
				for _, item := range m.report.APILatency.TopEndpointRegressions {
					rows = append(rows, []string{
						item.Route,
						output.FormatLatency(item.NowP95),
						output.FormatLatency(item.BaselineP95),
						output.FormatDeltaLatency(item.DeltaP95),
					})
				}
				sb.WriteString(renderTable([]string{"Endpoint", "Now P95", "Baseline", "Delta"}, rows))
				sb.WriteString("\n")
			}
			if len(m.report.APILatency.ActiveAlerts) > 0 {
				sb.WriteString("\n")
				sb.WriteString(markdownBlock("### Active Alerts"))
				for _, alert := range m.report.APILatency.ActiveAlerts {
					sb.WriteString("- " + alert + "\n")
				}
			}
			diagnosticHints := buildDiagnosticHints(m.report.APILatency)
			if len(diagnosticHints) > 0 {
				sb.WriteString("\n")
				sb.WriteString(markdownBlock("### Diagnostic Clues"))
				for _, hint := range diagnosticHints {
					sb.WriteString("- " + hint + "\n")
				}
			}
			if len(m.report.APILatency.RemediationHints) > 0 {
				sb.WriteString("\n")
				sb.WriteString(markdownBlock("### Remediation Checklist"))
				wrapWidth := m.width - 6
				if wrapWidth < 40 {
					wrapWidth = 80
				}
				for _, hint := range m.report.APILatency.RemediationHints {
					lines := wrapText(hint, wrapWidth)
					for i, line := range lines {
						if i == 0 {
							sb.WriteString("- " + line + "\n")
						} else {
							sb.WriteString("  " + line + "\n")
						}
					}
				}
			}
			if len(m.report.APILatency.LokiCorrelationHints) > 0 {
				sb.WriteString("\n")
				sb.WriteString(markdownBlock("### Prometheus ↔ Loki"))
				for _, hint := range m.report.APILatency.LokiCorrelationHints {
					sb.WriteString("- " + hint + "\n")
				}
			}
			if m.report.CorrelationSummary != nil {
				sb.WriteString("\n")
				sb.WriteString(markdownBlock("### Correlated Logs"))
				scopeParts := []string{}
				if strings.TrimSpace(m.report.CorrelationSummary.Scope.Service) != "" {
					scopeParts = append(scopeParts, "service="+m.report.CorrelationSummary.Scope.Service)
				}
				if strings.TrimSpace(m.report.CorrelationSummary.Scope.Route) != "" {
					scopeParts = append(scopeParts, "route="+m.report.CorrelationSummary.Scope.Route)
				}
				if strings.TrimSpace(m.report.CorrelationSummary.Scope.Dependency) != "" {
					scopeParts = append(scopeParts, "dependency="+m.report.CorrelationSummary.Scope.Dependency)
				}
				if len(scopeParts) > 0 {
					sb.WriteString(dimStyle.Render("Scope: " + strings.Join(scopeParts, " • ")))
					sb.WriteString("\n")
				}
				if strings.TrimSpace(m.report.CorrelationSummary.Window) != "" {
					sb.WriteString(dimStyle.Render("Window: " + m.report.CorrelationSummary.Window))
					sb.WriteString("\n")
				}
				if strings.TrimSpace(m.report.CorrelationSummary.Backend) != "" {
					sb.WriteString(dimStyle.Render("Backend: " + m.report.CorrelationSummary.Backend))
					sb.WriteString("\n")
				}
				if m.report.CorrelationSummary.Samples > 0 {
					sb.WriteString(dimStyle.Render(fmt.Sprintf("Samples: %d", m.report.CorrelationSummary.Samples)))
					sb.WriteString("\n")
				}
				if m.report.CorrelationSummary.MinSamples > 0 {
					sb.WriteString(dimStyle.Render(fmt.Sprintf("Min samples: %d", m.report.CorrelationSummary.MinSamples)))
					sb.WriteString("\n")
				}
				if strings.TrimSpace(m.report.CorrelationSummary.Confidence) != "" {
					sb.WriteString(dimStyle.Render("Confidence: " + strings.Title(m.report.CorrelationSummary.Confidence)))
					sb.WriteString("\n")
				}
				if len(m.report.CorrelationSummary.Signatures) > 0 {
					sb.WriteString("\n")
					sb.WriteString(dimStyle.Render("Top Log Patterns:") + "\n")
					for _, sig := range m.report.CorrelationSummary.Signatures {
						line := fmt.Sprintf("[%d%%] %s", int(sig.Percent), sig.Signature)
						sb.WriteString(line + "\n")
					}
				}
				if len(m.report.CorrelationSummary.NoDataReasons) > 0 {
					for _, r := range m.report.CorrelationSummary.NoDataReasons {
						sb.WriteString(red.Render("  ❌ "+r) + "\n")
					}
				}
				if len(m.report.CorrelationSummary.Notes) > 0 {
					for _, n := range m.report.CorrelationSummary.Notes {
						sb.WriteString(dimStyle.Render("  • "+n) + "\n")
					}
				}
				if strings.TrimSpace(m.report.CorrelationSummary.Query) != "" && strings.EqualFold(m.report.CorrelationSummary.Backend, "loki") {
					cfg, _ := config.Load()
					lokiUID, lokiType := m.resolveGrafanaDatasource(cfg, cfg.GrafanaLokiDataSource)
					link := m.correlationLink
					if strings.TrimSpace(link) == "" {
						link = output.BuildGrafanaExploreURL(cfg.GrafanaURL, cfg.GrafanaLokiDataSource, lokiUID, lokiType, m.report.CorrelationSummary.Query, "now-"+m.report.CorrelationSummary.Window, "now")
					}
					if strings.TrimSpace(link) != "" {
						sb.WriteString(dimStyle.Render("Grafana link (Loki Explore):"))
						sb.WriteString("\n")
						sb.WriteString(renderWrappedLink(link, m.width))
						sb.WriteString("\n")
						if strings.TrimSpace(m.correlationLinkPath) != "" {
							sb.WriteString(dimStyle.Render("Saved Grafana link: " + m.correlationLinkPath))
							sb.WriteString("\n")
						}
						sb.WriteString(dimStyle.Render("Press 'y' to copy correlation link."))
						sb.WriteString("\n")
					} else {
						switch {
						case strings.TrimSpace(cfg.GrafanaURL) == "":
							sb.WriteString(dimStyle.Render("Grafana URL not set; set GRAFANA_URL to enable Explore links."))
							sb.WriteString("\n")
						case strings.TrimSpace(cfg.GrafanaLokiDataSource) == "":
							sb.WriteString(dimStyle.Render("Grafana Loki datasource not set; set GRAFANA_LOKI_DS to enable Explore links."))
							sb.WriteString("\n")
						case strings.TrimSpace(lokiUID) == "" || strings.TrimSpace(lokiType) == "":
							sb.WriteString(dimStyle.Render("Grafana Loki datasource lookup failed; set GRAFANA_TOKEN or Grafana user/pass."))
							sb.WriteString("\n")
						case !strings.EqualFold(lokiType, "loki"):
							sb.WriteString(dimStyle.Render("Grafana Loki datasource not configured; set GRAFANA_LOKI_DS."))
							sb.WriteString("\n")
						}
					}
				} else if strings.TrimSpace(m.report.CorrelationSummary.Query) != "" && (strings.EqualFold(m.report.CorrelationSummary.Backend, "elastic") || strings.EqualFold(m.report.CorrelationSummary.Backend, "opensearch")) {
					cfg, _ := config.Load()
					kibanaLink := buildKibanaDiscoverURL(cfg.KibanaURL, cfg.KibanaIndex, m.report.CorrelationSummary.Query, m.report.CorrelationSummary.Window)
					if strings.TrimSpace(kibanaLink) != "" {
						sb.WriteString(dimStyle.Render("Kibana Discover link:"))
						sb.WriteString("\n")
						sb.WriteString(renderWrappedLink(kibanaLink, m.width))
						sb.WriteString("\n")
					} else {
						sb.WriteString(dimStyle.Render("Elastic correlation link not configured; set KIBANA_URL and KIBANA_INDEX."))
						sb.WriteString("\n")
					}
				}
				if strings.TrimSpace(m.correlationMessage) != "" {
					sb.WriteString(dimStyle.Render(m.correlationMessage))
					sb.WriteString("\n")
				}
				if len(m.report.CorrelationSummary.Notes) > 0 {
					for _, note := range m.report.CorrelationSummary.Notes {
						sb.WriteString(dimStyle.Render(note))
						sb.WriteString("\n")
					}
				}
			}
			if len(m.report.CorrelationAlternates) > 0 {
				sb.WriteString("\n")
				sb.WriteString(markdownBlock("### Alternate Correlated Logs (Low Confidence)"))
				for _, alt := range m.report.CorrelationAlternates {
					scopeParts := []string{}
					if strings.TrimSpace(alt.Scope.Service) != "" {
						scopeParts = append(scopeParts, "service="+alt.Scope.Service)
					}
					if strings.TrimSpace(alt.Scope.Route) != "" {
						scopeParts = append(scopeParts, "route="+alt.Scope.Route)
					}
					if strings.TrimSpace(alt.Scope.Dependency) != "" {
						scopeParts = append(scopeParts, "dependency="+alt.Scope.Dependency)
					}
					if len(scopeParts) > 0 {
						sb.WriteString(dimStyle.Render("Scope: " + strings.Join(scopeParts, " • ")))
						sb.WriteString("\n")
					}
					if strings.TrimSpace(alt.Window) != "" {
						sb.WriteString(dimStyle.Render("Window: " + alt.Window))
						sb.WriteString("\n")
					}
					if alt.Samples > 0 {
						sb.WriteString(dimStyle.Render(fmt.Sprintf("Samples: %d", alt.Samples)))
						sb.WriteString("\n")
					}
					if strings.TrimSpace(alt.Confidence) != "" {
						sb.WriteString(dimStyle.Render("Confidence: " + strings.Title(alt.Confidence)))
						sb.WriteString("\n")
					}
					if len(alt.NoDataReasons) > 0 {
						for _, reason := range alt.NoDataReasons {
							sb.WriteString(dimStyle.Render(reason))
							sb.WriteString("\n")
						}
					} else if len(alt.Signatures) > 0 {
						top := alt.Signatures[0]
						sb.WriteString(dimStyle.Render(fmt.Sprintf("Root cause: %s (%s, %d)", top.Signature, output.FormatPercent(top.Percent), top.Count)))
						sb.WriteString("\n")
						for _, entry := range alt.Signatures {
							sb.WriteString(fmt.Sprintf("- %s (%s, %d)\n", entry.Signature, output.FormatPercent(entry.Percent), entry.Count))
						}
					}
					if len(alt.Notes) > 0 {
						for _, note := range alt.Notes {
							sb.WriteString(dimStyle.Render(note))
							sb.WriteString("\n")
						}
					}
					sb.WriteString("\n")
				}
			}
			if m.report.APM != nil {
				sb.WriteString("\n")
				sb.WriteString(markdownBlock("### Suspected Upstream Cause"))
				if strings.TrimSpace(m.report.APM.SuspectedUpstream) == "" {
					if len(m.report.APM.NoDataReasons) > 0 {
						sb.WriteString(dimStyle.Render("No root-cause inference available; see APM Data Gaps."))
					} else {
						sb.WriteString(dimStyle.Render("No root-cause inference available."))
					}
					sb.WriteString("\n")
				}
				if strings.TrimSpace(m.report.APM.SuspectedUpstream) != "" {
					sb.WriteString(dimStyle.Render("Cause: " + m.report.APM.SuspectedUpstream))
					sb.WriteString("\n")
				}
				if strings.TrimSpace(m.report.APM.CorrelationConfidence) != "" {
					sb.WriteString(dimStyle.Render("Confidence: " + strings.Title(m.report.APM.CorrelationConfidence)))
					sb.WriteString("\n")
				}
				if len(m.report.APM.Evidence) > 0 {
					for _, item := range m.report.APM.Evidence {
						sb.WriteString("- " + item + "\n")
					}
				}
				if len(m.report.APM.RankedCauses) > 0 {
					sb.WriteString("\n")
					sb.WriteString(markdownBlock("### APM Root Cause Ranking"))
					rows := make([][]string, 0, len(m.report.APM.RankedCauses))
					for _, cause := range m.report.APM.RankedCauses {
						rows = append(rows, []string{
							cause.Path,
							fmt.Sprintf("%.2f", cause.Score),
							strings.Title(cause.Confidence),
						})
					}
					sb.WriteString(renderTable([]string{"Path", "Score", "Confidence"}, rows))
				}
				if len(m.report.APM.ReasoningSteps) > 0 {
					sb.WriteString("\n")
					sb.WriteString(markdownBlock("### Why We Think This Is The Cause"))
					for _, item := range m.report.APM.ReasoningSteps {
						sb.WriteString("- " + item + "\n")
					}
				}
				if len(m.report.APM.NextChecks) > 0 {
					sb.WriteString("\n")
					sb.WriteString(markdownBlock("### What To Check Next"))
					for _, item := range m.report.APM.NextChecks {
						sb.WriteString("- " + item + "\n")
					}
				}
				if len(m.report.APM.PossibleFixes) > 0 {
					sb.WriteString("\n")
					sb.WriteString(markdownBlock("### Possible Fixes"))
					for _, item := range m.report.APM.PossibleFixes {
						sb.WriteString("- " + item + "\n")
					}
				}
				if len(m.report.APM.NoDataReasons) > 0 {
					sb.WriteString("\n")
					sb.WriteString(markdownBlock("### APM Data Gaps"))
					for _, reason := range m.report.APM.NoDataReasons {
						line := fmt.Sprintf("%s: %s", reason.Area, reason.Reason)
						if strings.TrimSpace(reason.SuggestedFix) != "" {
							line += " (Fix: " + reason.SuggestedFix + ")"
						}
						sb.WriteString("- " + line + "\n")
					}
				}
				if strings.TrimSpace(m.report.APM.Note) != "" {
					sb.WriteString("\n")
					sb.WriteString(markdownBlock("### APM Auto-Detect Reasoning"))
					for _, line := range wrapNoteLines(m.report.APM.Note) {
						sb.WriteString(dimStyle.Render("  " + line))
						sb.WriteString("\n")
					}
				}
				if len(m.report.APM.Edges) > 0 {
					sb.WriteString("\n")
					sb.WriteString(markdownBlock("### Top Dependency Edges (P95)"))
					rows := make([][]string, 0, len(m.report.APM.Edges))
					for _, edge := range m.report.APM.Edges {
						label := edge.Source + " -> " + edge.Destination
						if strings.TrimSpace(edge.Route) != "" {
							label = label + " (" + edge.Route + ")"
						}
						rows = append(rows, []string{
							label,
							output.FormatLatency(edge.P95),
							output.FormatDeltaLatency(edge.DeltaP95),
						})
					}
					sb.WriteString(renderTable([]string{"Edge", "P95", "Delta"}, rows))
				}
			}
			if m.report.Tracing != nil {
				sb.WriteString("\n")
				sb.WriteString(markdownBlock("### Trace Diagnostics"))
				traceSvc := m.report.Tracing.Service
				if strings.TrimSpace(traceSvc) == "" {
					traceSvc = "auto"
				}
				sb.WriteString(dimStyle.Render("Service: " + traceSvc))
				sb.WriteString("\n")
				if strings.TrimSpace(m.report.Tracing.Window) != "" {
					sb.WriteString(dimStyle.Render("Window: " + m.report.Tracing.Window))
					sb.WriteString("\n")
				}
				if strings.TrimSpace(m.report.Tracing.Note) != "" {
					sb.WriteString(dimStyle.Render(m.report.Tracing.Note))
					sb.WriteString("\n")
				}
				if len(m.report.Tracing.TopSlowTraces) > 0 {
					sb.WriteString("\n")
					sb.WriteString(markdownBlock("### Top Slow Traces"))
					for i, trace := range m.report.Tracing.TopSlowTraces {
						sb.WriteString(fmt.Sprintf("%d. Trace ID: %s\n", i+1, trace.TraceID))
						sb.WriteString(dimStyle.Render("   Total Duration: " + trace.TotalDuration))
						sb.WriteString("\n")
						if strings.TrimSpace(trace.RootOperation) != "" {
							sb.WriteString(dimStyle.Render("   Root Operation: " + trace.RootOperation))
							sb.WriteString("\n")
						}
						if strings.TrimSpace(trace.SlowestSpan.Service) != "" || strings.TrimSpace(trace.SlowestSpan.Operation) != "" {
							sb.WriteString(dimStyle.Render("   Slowest Downstream Span:"))
							sb.WriteString("\n")
							if strings.TrimSpace(trace.SlowestSpan.Service) != "" {
								sb.WriteString(dimStyle.Render("     Service: " + trace.SlowestSpan.Service))
								sb.WriteString("\n")
							}
							if strings.TrimSpace(trace.SlowestSpan.Operation) != "" {
								sb.WriteString(dimStyle.Render("     Operation: " + trace.SlowestSpan.Operation))
								sb.WriteString("\n")
							}
							if strings.TrimSpace(trace.SlowestSpan.Duration) != "" {
								sb.WriteString(dimStyle.Render("     Duration: " + trace.SlowestSpan.Duration))
								sb.WriteString("\n")
							}
						}
					}
				}
				links := m.traceLinks
				if len(links) == 0 {
					links = m.report.Tracing.Links
				}
				if len(links) == 0 {
					links = output.ResolveTraceLinks(m.report.Tracing)
				}
				if len(links) > 0 {
					sb.WriteString("\n")
					sb.WriteString(markdownBlock("### Trace Links"))
					for _, link := range links {
						label := strings.TrimSpace(link.Label)
						if label == "" {
							label = "Trace link"
						}
						sb.WriteString(dimStyle.Render(label + ":"))
						sb.WriteString("\n")
						sb.WriteString(renderWrappedLink(link.URL, m.width))
						sb.WriteString("\n")
					}
					if strings.TrimSpace(m.traceLinkPath) != "" {
						sb.WriteString(dimStyle.Render("Saved trace links: " + m.traceLinkPath))
						sb.WriteString("\n")
					}
				}
			}
			if len(m.report.APILatency.TopEndpoints) > 0 {
				sb.WriteString("\n")
				title := "### Top Endpoints (P95)"
				if m.report.APILatency.TopEndpointsLimit > 0 {
					title = fmt.Sprintf("### Top Endpoints (P95) — top %d", m.report.APILatency.TopEndpointsLimit)
				}
				sb.WriteString(markdownBlock(title))
				rows := make([][]string, 0, len(m.report.APILatency.TopEndpoints))
				for _, endpoint := range m.report.APILatency.TopEndpoints {
					rows = append(rows, []string{endpoint.Route, output.FormatLatency(endpoint.P95)})
				}
				sb.WriteString(renderTable([]string{"Endpoint", "P95"}, rows))
				sb.WriteString("\n")
			}
			sb.WriteString("\n")
			title := "### Top Endpoints (RPS)"
			if m.report.APILatency.TopEndpointsLimit > 0 {
				title = fmt.Sprintf("### Top Endpoints (RPS) — top %d", m.report.APILatency.TopEndpointsLimit)
			}
			sb.WriteString(markdownBlock(title))
			if strings.TrimSpace(m.report.APILatency.TopEndpointsRPSNote) != "" {
				sb.WriteString(dimStyle.Render(m.report.APILatency.TopEndpointsRPSNote))
				sb.WriteString("\n")
			}
			if len(m.report.APILatency.TopEndpointsRPS) > 0 {
				rows := make([][]string, 0, len(m.report.APILatency.TopEndpointsRPS))
				for _, endpoint := range m.report.APILatency.TopEndpointsRPS {
					rows = append(rows, []string{endpoint.Route, fmt.Sprintf("%.2f", endpoint.RPS)})
				}
				sb.WriteString(renderTable([]string{"Endpoint", "RPS"}, rows))
			} else {
				sb.WriteString(dimStyle.Render("No per-endpoint data found in request metrics."))
				sb.WriteString("\n")
			}
			if len(m.report.APILatency.TopServicesRPS) > 0 {
				sb.WriteString("\n")
				sb.WriteString(markdownBlock("### Top Services (RPS)"))
				if strings.TrimSpace(m.report.APILatency.TopServicesRPSNote) != "" {
					sb.WriteString(dimStyle.Render(m.report.APILatency.TopServicesRPSNote))
					sb.WriteString("\n")
				}
				rows := make([][]string, 0, len(m.report.APILatency.TopServicesRPS))
				for _, svc := range m.report.APILatency.TopServicesRPS {
					rows = append(rows, []string{svc.Service, fmt.Sprintf("%.2f", svc.RPS)})
				}
				sb.WriteString(renderTable([]string{"Service", "RPS"}, rows))
			}
		}

	// VAR
	if len(m.report.Vars) > 0 {
		sb.WriteString("\n")
		sb.WriteString(markdownBlock("## 📁 /var – Top Consumers"))
		for _, v := range m.report.Vars {
			sb.WriteString(fmt.Sprintf(
				"%s  %s\n",
				valueStyle.Render(fmt.Sprintf("%dMB", v.Size)),
				v.Path,
			))
		}
	}

	// Suggestions
	sb.WriteString("\n")
	sb.WriteString(markdownBlock("## 🧹 Optimization / Cleanup Suggestions"))
	if len(m.report.Cleanup) == 0 {
		sb.WriteString(cleanupMessageStyle.Render("✓ No cleanup actions required. System looks healthy."))
		sb.WriteString("\n")
	} else {
		for _, c := range m.report.Cleanup {
			// Check if it's the default "no cleanup" message
			if strings.Contains(c, "No immediate cleanup actions required") ||
				strings.Contains(c, "No cleanup actions required") ||
				strings.Contains(c, "System looks healthy") {
				sb.WriteString(cleanupMessageStyle.Render("✓ " + c))
			} else {
				sb.WriteString("• " + c)
			}
			sb.WriteString("\n")
		}
	}

	// Debug Mode Overlay
	if m.showDebug {
		sb.WriteString("\n")
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true).Render("## 🔍 Background Debug Log (Last 10 lines)"))
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render(m.debugOutput))
		sb.WriteString("\n")
	}

	// Banner (footer)
	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("─", 80))
	sb.WriteString("\n")
	displayVersion := model.GetLatestVersion()
	bannerText := fmt.Sprintf("Sofueled Health-Monitor %s | communication@sofueled.com", displayVersion)
	sb.WriteString(bannerStyle.Render(bannerText))
	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("─", 80))
	sb.WriteString("\n\n")

	return sb.String()
}

func infoCard(title string, value string) string {
	content := lipgloss.NewStyle().
		Width(18).
		Align(lipgloss.Center).
		Render(
			lipgloss.NewStyle().Bold(true).Render(title) + "\n" +
				value,
		)
	return panelStyle.Render(content)
}

func renderTable(headers []string, rows [][]string) string {
	if len(headers) == 0 {
		return ""
	}
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i >= len(widths) {
				continue
			}
			if l := lipgloss.Width(cell); l > widths[i] {
				widths[i] = l
			}
		}
	}

	var sb strings.Builder
	sb.WriteString(tableRow(headers, widths, true))
	sb.WriteString("\n")
	for _, row := range rows {
		sb.WriteString(tableRow(row, widths, false))
		sb.WriteString("\n")
	}
	return sb.String()
}

func tableRow(values []string, widths []int, header bool) string {
	var sb strings.Builder
	for i := 0; i < len(widths); i++ {
		cell := ""
		if i < len(values) {
			cell = values[i]
		}
		padded := padRight(cell, widths[i])
		if header {
			padded = lipgloss.NewStyle().Bold(true).Render(padded)
		}
		sb.WriteString(padded)
		if i < len(widths)-1 {
			sb.WriteString("  ")
		}
	}
	return sb.String()
}

/* Utility functions moved to tui_utils.go */

/* Datasource methods moved to tui_datasource.go */

/* Loki log fetching methods moved to tui_datasource.go */

/* detectLokiLabels moved to tui_datasource.go */

/* findLabelWithValue moved to tui_datasource.go */

/* sanitizeLogLine moved to tui_utils.go */

func formatWrappedLog(ts string, line string, width int) string {
	if width <= 10 {
		return ts + " " + line
	}
	parts := wrapText(line, width)
	if len(parts) == 0 {
		return ts + " " + line
	}
	prefix := ts + " "
	padding := strings.Repeat(" ", len(prefix))
	var sb strings.Builder
	sb.WriteString(prefix + parts[0])
	for i := 1; i < len(parts); i++ {
		sb.WriteString("\n")
		sb.WriteString(padding + parts[i])
	}
	return sb.String()
}

func wrapText(value string, width int) []string {
	if width <= 0 {
		return []string{value}
	}
	words := strings.Fields(value)
	if len(words) == 0 {
		return []string{value}
	}
	lines := []string{}
	var current strings.Builder
	for _, word := range words {
		if current.Len() == 0 {
			current.WriteString(word)
			continue
		}
		if current.Len()+1+len(word) <= width {
			current.WriteString(" ")
			current.WriteString(word)
			continue
		}
		lines = append(lines, current.String())
		current.Reset()
		if len(word) > width {
			// Hard wrap long words.
			for len(word) > width {
				lines = append(lines, word[:width])
				word = word[width:]
			}
			if len(word) > 0 {
				current.WriteString(word)
			}
		} else {
			current.WriteString(word)
		}
	}
	if current.Len() > 0 {
		lines = append(lines, current.String())
	}
	return lines
}

func renderWrappedLink(link string, screenWidth int) string {
	width := screenWidth - 6
	if width < 40 {
		width = 80
	}
	parts := wrapText(link, width)
	return strings.Join(parts, "\n")
}

func buildAPMLogHint(apmInfo *model.DependencyAnalysis) string {
	if apmInfo == nil || strings.TrimSpace(apmInfo.SuspectedUpstream) == "" {
		return ""
	}
	parts := strings.Split(apmInfo.SuspectedUpstream, "->")
	if len(parts) < 2 {
		return ""
	}
	dest := strings.TrimSpace(parts[len(parts)-1])
	if idx := strings.Index(dest, "("); idx != -1 {
		dest = strings.TrimSpace(dest[:idx])
	}
	if dest == "" {
		return ""
	}
	return fmt.Sprintf("APM hint: filter logs for destination %q (LogQL example: {service=~%q} |~ \"error|exception|5[0-9][0-9]\")", dest, dest)
}

func extractCardinalityWarnings(note string) []string {
	parts := strings.Split(note, " • ")
	var warnings []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if strings.Contains(trimmed, "high route cardinality") || strings.Contains(trimmed, "high service cardinality") {
			warnings = append(warnings, trimmed)
		}
	}
	return warnings
}

func mergeConfigFromSummary(cfg config.Config, summary *model.APIConfigSummary) config.Config {
	if summary == nil {
		return cfg
	}
	if strings.TrimSpace(cfg.PrometheusURL) == "" {
		cfg.PrometheusURL = summary.PrometheusURL
	}
	if strings.TrimSpace(cfg.APIService) == "" {
		cfg.APIService = summary.APIService
	}
	if strings.TrimSpace(cfg.APIRoute) == "" {
		cfg.APIRoute = summary.APIRoute
	}
	if strings.TrimSpace(cfg.ServiceLabel) == "" {
		cfg.ServiceLabel = summary.ServiceLabel
	}
	if strings.TrimSpace(cfg.RouteLabel) == "" {
		cfg.RouteLabel = summary.RouteLabel
	}
	if strings.TrimSpace(cfg.LatencyMetric) == "" {
		cfg.LatencyMetric = summary.LatencyMetric
	}
	if strings.TrimSpace(cfg.RequestCountMetric) == "" {
		cfg.RequestCountMetric = summary.RequestMetric
	}
	if strings.TrimSpace(cfg.ErrorLabel) == "" {
		cfg.ErrorLabel = summary.ErrorLabel
	}
	if strings.TrimSpace(cfg.ErrorRegex) == "" {
		cfg.ErrorRegex = summary.ErrorRegex
	}
	if strings.TrimSpace(cfg.Window) == "" {
		cfg.Window = summary.Window
	}
	if !cfg.AutoDiscover {
		cfg.AutoDiscover = summary.AutoDiscover
	}
	return cfg
}

func isPromMetricName(name string) bool {
	if strings.TrimSpace(name) == "" {
		return false
	}
	return regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`).MatchString(name)
}

func isPromLabelName(name string) bool {
	if strings.TrimSpace(name) == "" {
		return false
	}
	return regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`).MatchString(name)
}

func buildConfigHealth(cfg config.Config) []string {
	var issues []string
	if strings.TrimSpace(cfg.PrometheusURL) == "" {
		issues = append(issues, "Prometheus URL not set.")
	} else if !strings.HasPrefix(cfg.PrometheusURL, "http://") && !strings.HasPrefix(cfg.PrometheusURL, "https://") {
		issues = append(issues, "Prometheus URL must start with http:// or https://.")
	}
	if !cfg.AutoDiscover && strings.TrimSpace(cfg.ServiceLabel) != "" && strings.TrimSpace(cfg.APIService) == "" {
		issues = append(issues, "API_SERVICE required when auto-discover is off.")
	}
	if strings.TrimSpace(cfg.LatencyMetric) != "" && !isPromMetricName(cfg.LatencyMetric) {
		issues = append(issues, fmt.Sprintf("Latency metric name invalid: %s", cfg.LatencyMetric))
	}
	if strings.TrimSpace(cfg.RequestCountMetric) != "" && !isPromMetricName(cfg.RequestCountMetric) {
		issues = append(issues, fmt.Sprintf("Request metric name invalid: %s", cfg.RequestCountMetric))
	}
	if strings.TrimSpace(cfg.ServiceLabel) != "" && !isPromLabelName(cfg.ServiceLabel) {
		issues = append(issues, fmt.Sprintf("Service label name invalid: %s", cfg.ServiceLabel))
	}
	if strings.TrimSpace(cfg.RouteLabel) != "" && !isPromLabelName(cfg.RouteLabel) {
		issues = append(issues, fmt.Sprintf("Route label name invalid: %s", cfg.RouteLabel))
	}
	if cfg.PrometheusQPS == 0 {
		issues = append(issues, "Prometheus QPS is 0; rate limiting disabled.")
	}
	if cfg.RouteCardinalityLimit == 0 {
		issues = append(issues, "Route cardinality limit is 0; per-route queries may be heavy.")
	}
	if cfg.ServiceCardinalityLimit == 0 {
		issues = append(issues, "Service cardinality limit is 0; per-service queries may be heavy.")
	}
	return issues
}

func findStaleDataReason(reasons []model.NoDataReason) string {
	for _, reason := range reasons {
		if reason.Area == "Stale data" {
			return reason.Reason
		}
	}
	return ""
}

func resetInvalidPromOverrides() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.AutoDiscover = true
	cfg.LatencyMetric = ""
	cfg.RequestCountMetric = ""
	cfg.ServiceLabel = ""
	cfg.RouteLabel = ""
	cfg.APIService = ""
	cfg.APIRoute = ""
	if err := saveCurrentConfig(cfg); err != nil {
		return err
	}
	return nil
}

func writeGrafanaLinkFile(kind string, link string) (string, error) {
	if strings.TrimSpace(link) == "" {
		return "", errors.New("link is empty")
	}
	filename := fmt.Sprintf("health-monitor-grafana-%s.url", kind)
	path := filepath.Join(os.TempDir(), filename)
	if err := os.WriteFile(path, []byte(link+"\n"), 0600); err != nil {
		return "", err
	}
	return path, nil
}

func writeTraceLinksFile(links []model.TraceLink) (string, error) {
	if len(links) == 0 {
		return "", errors.New("links are empty")
	}
	filename := "health-monitor-trace-links.txt"
	path := filepath.Join(os.TempDir(), filename)
	var b strings.Builder
	for _, link := range links {
		label := strings.TrimSpace(link.Label)
		if label == "" {
			label = "Trace link"
		}
		if strings.TrimSpace(link.URL) == "" {
			continue
		}
		b.WriteString(label)
		b.WriteString(": ")
		b.WriteString(link.URL)
		b.WriteString("\n")
	}
	if b.Len() == 0 {
		return "", errors.New("links are empty")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0600); err != nil {
		return "", err
	}
	return path, nil
}

func exportReportSnapshot(report model.Report) (string, string, error) {
	mdContent, err := output.ExportReport(report, "md")
	if err != nil {
		return "", "", err
	}
	jsonContent, err := output.ExportReport(report, "json")
	if err != nil {
		return "", "", err
	}
	cfg, _ := config.Load()
	timestamp := time.Now().Format("20060102-150405")
	mdPath := resolveExportPath(cfg.ExportPath, "md", timestamp, true)
	jsonPath := resolveExportPath(cfg.ExportPath, "json", timestamp, true)
	if err := os.WriteFile(mdPath, []byte(mdContent+"\n"), 0600); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(jsonPath, []byte(jsonContent+"\n"), 0600); err != nil {
		return "", "", err
	}
	return mdPath, jsonPath, nil
}

func exportReportSingle(report model.Report, format string) (string, error) {
	content, err := output.ExportReport(report, format)
	if err != nil {
		return "", err
	}
	ext := "md"
	if strings.EqualFold(strings.TrimSpace(format), "json") {
		ext = "json"
	}
	cfg, _ := config.Load()
	timestamp := time.Now().Format("20060102-150405")
	path := resolveExportPath(cfg.ExportPath, ext, timestamp, false)
	if err := os.WriteFile(path, []byte(content+"\n"), 0600); err != nil {
		return "", err
	}
	return path, nil
}

func resolveExportPath(exportPath string, ext string, timestamp string, multi bool) string {
	filename := "health-monitor-report-" + timestamp + "." + ext
	clean := strings.TrimSpace(exportPath)
	if clean == "" {
		return filepath.Join(os.TempDir(), filename)
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
	if multi {
		return filepath.Join(filepath.Dir(clean), filename)
	}
	if strings.HasSuffix(clean, "."+ext) {
		return clean
	}
	if strings.HasSuffix(clean, ".md") || strings.HasSuffix(clean, ".json") {
		return filepath.Join(filepath.Dir(clean), filename)
	}
	return filepath.Join(clean, filename)
}

func hasPathTraversal(path string) bool {
	sep := string(os.PathSeparator)
	if path == ".." || strings.HasPrefix(path, ".."+sep) {
		return true
	}
	if strings.Contains(path, sep+".."+sep) || strings.HasSuffix(path, sep+"..") {
		return true
	}
	return false
}

func (m *tuiModel) updatePromLink() {
	m.promLink = ""
	m.promLinkPath = ""
	if m.report.APILatency == nil {
		return
	}
	cfg, _ := config.Load()
	promUID, promType := m.resolveGrafanaDatasource(cfg, cfg.GrafanaPromDataSource)
	link := output.BuildGrafanaExploreURL(cfg.GrafanaURL, cfg.GrafanaPromDataSource, promUID, promType, output.PromP95Query(m.report.APILatency), "now-"+m.report.APILatency.Window, "now")
	if strings.TrimSpace(link) == "" {
		return
	}
	m.promLink = link
	if path, err := writeGrafanaLinkFile("prom", link); err == nil {
		m.promLinkPath = path
	}
}

func (m *tuiModel) updateCorrelationLink() {
	m.correlationLink = ""
	m.correlationLinkPath = ""
	if m.report.CorrelationSummary == nil || strings.TrimSpace(m.report.CorrelationSummary.Query) == "" {
		return
	}
	if !strings.EqualFold(m.report.CorrelationSummary.Backend, "loki") {
		return
	}
	cfg, _ := config.Load()
	lokiUID, lokiType := m.resolveGrafanaDatasource(cfg, cfg.GrafanaLokiDataSource)
	link := output.BuildGrafanaExploreURL(cfg.GrafanaURL, cfg.GrafanaLokiDataSource, lokiUID, lokiType, m.report.CorrelationSummary.Query, "now-"+m.report.CorrelationSummary.Window, "now")
	if strings.TrimSpace(link) == "" {
		return
	}
	m.correlationLink = link
	if path, err := writeGrafanaLinkFile("correlation", link); err == nil {
		m.correlationLinkPath = path
	}
}

func buildKibanaDiscoverURL(baseURL string, indexPattern string, query string, window string) string {
	if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(indexPattern) == "" || strings.TrimSpace(query) == "" {
		return ""
	}
	from := "now-30m"
	if strings.TrimSpace(window) != "" {
		from = "now-" + window
	}
	kuery := escapeKibanaQuery(query)
	_app := fmt.Sprintf("(index:'%s',query:(language:kuery,query:'%s'))", escapeKibanaQuery(indexPattern), kuery)
	_g := fmt.Sprintf("(time:(from:'%s',to:'now'))", from)
	return strings.TrimSuffix(baseURL, "/") + "/app/discover#/?_g=" + url.QueryEscape(_g) + "&_a=" + url.QueryEscape(_app)
}

func escapeKibanaQuery(value string) string {
	out := strings.ReplaceAll(value, "\\", "\\\\")
	out = strings.ReplaceAll(out, "'", "\\'")
	return out
}
func copyToClipboard(value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("empty clipboard value")
	}
	candidates := [][]string{
		{"wl-copy"},
		{"xclip", "-selection", "clipboard"},
		{"xsel", "--clipboard", "--input"},
		{"pbcopy"},
		{"powershell.exe", "-NoProfile", "-Command", "Set-Clipboard"},
		{"clip.exe"},
	}
	var lastErr error
	for _, cmd := range candidates {
		if _, err := exec.LookPath(cmd[0]); err != nil {
			continue
		}
		c := exec.Command(cmd[0], cmd[1:]...)
		c.Stdin = strings.NewReader(value)
		if err := c.Run(); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	hint := clipboardInstallHint()
	if hint != "" {
		return errors.New("no clipboard tool found. " + hint)
	}
	return errors.New("no clipboard tool found (try installing wl-copy or xclip)")
}

func clipboardInstallHint() string {
	if _, err := exec.LookPath("apt-get"); err == nil {
		return "Install: sudo apt-get install -y wl-clipboard xclip"
	}
	return ""
}

/* More utility functions moved to tui_utils.go */

func (m *tuiModel) loadIncidents() {
	service, err := incident.NewService(nil)
	if err != nil {
		m.incidentMessage = "Failed to initialize incident service: " + err.Error()
		m.incidentWarnings = nil
		m.activeIncident = nil
		m.incidentList = nil
		m.filteredIncidentList = nil
		return
	}
	active, warnings, err := service.View("")
	if err != nil {
		m.incidentMessage = "Incident load failed: " + err.Error()
		for _, warning := range warnings {
			fmt.Fprintln(os.Stderr, "Warning:", warning)
		}
		m.incidentWarnings = nil
		m.activeIncident = nil
		m.incidentList = nil
		m.filteredIncidentList = nil
		return
	}
	list, listWarnings, err := service.List()
	m.activeIncident = active
	m.incidentList = list
	for _, warning := range append(warnings, listWarnings...) {
		fmt.Fprintln(os.Stderr, "Warning:", warning)
	}
	m.incidentWarnings = nil
	if err != nil {
		m.incidentMessage = "Incident list failed: " + err.Error()
		m.filteredIncidentList = nil
	} else {
		m.incidentMessage = ""
		// Apply filters to populate filtered list
		m.applyIncidentFilters()
	}
}

func (m *tuiModel) loadFlows() {
	m.flowMessage = ""
	result, err := flow.LoadOnce()
	if err != nil {
		switch {
		case flow.IsNoConfig(err):
			m.flowMessage = "Flows disabled (no config found)"
		case flow.IsInvalidConfig(err):
			m.flowMessage = "Invalid flows config — skipping"
		default:
			m.flowMessage = "Failed to load flows: " + err.Error()
		}
		m.flowList = nil
		m.flowStatuses = nil
		m.flowIncidents = nil
		m.flowReasons = nil
		return
	}
	if len(result.Flows) == 0 {
		m.flowMessage = "No flows configured"
		m.flowList = nil
		m.flowStatuses = nil
		m.flowIncidents = nil
		m.flowReasons = nil
		return
	}
	active, _, err := incident.ListActiveCached()
	if err != nil {
		m.flowMessage = "Failed to load incidents: " + err.Error()
		m.flowList = nil
		m.flowStatuses = nil
		m.flowIncidents = nil
		m.flowReasons = nil
		return
	}
	activeServices := make(map[string]struct{})
	flowCounts := make(map[string]int)
	flowReasons := make(map[string]map[string]struct{})
	for _, inc := range active {
		serviceName := strings.TrimSpace(inc.Service)
		if serviceName == "" {
			continue
		}
		activeServices[serviceName] = struct{}{}
		for _, flowItem := range result.Flows {
			if flowItem.ID == "" {
				continue
			}
			if flow.IsDegraded(flowItem, map[string]struct{}{serviceName: {}}) {
				flowCounts[flowItem.ID]++
				if flowReasons[flowItem.ID] == nil {
					flowReasons[flowItem.ID] = make(map[string]struct{})
				}
				flowReasons[flowItem.ID][serviceName] = struct{}{}
			}
		}
	}
	statuses := make(map[string]bool, len(result.Flows))
	reasons := make(map[string][]string, len(result.Flows))
	counts := make(map[string]int, len(result.Flows))
	for _, flowItem := range result.Flows {
		statuses[flowItem.ID] = flow.IsDegraded(flowItem, activeServices)
		counts[flowItem.ID] = flowCounts[flowItem.ID]
		if flowReasons[flowItem.ID] != nil {
			for reason := range flowReasons[flowItem.ID] {
				reasons[flowItem.ID] = append(reasons[flowItem.ID], reason)
			}
			sort.Strings(reasons[flowItem.ID])
		}
	}
	m.flowList = result.Flows
	m.flowStatuses = statuses
	m.flowIncidents = counts
	m.flowReasons = reasons
}

func (m *tuiModel) renderIncidents() string {
	// Use the new interactive incident browser
	return m.renderIncidentBrowser()
}

func (m *tuiModel) renderFlows() string {
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(titleStyle.Render("Flows"))
	sb.WriteString("\n\n")
	if strings.TrimSpace(m.flowMessage) != "" {
		sb.WriteString(dimStyle.Render(m.flowMessage))
		sb.WriteString("\n\n")
	}
	if len(m.flowList) == 0 {
		return sb.String()
	}
	for _, flowItem := range m.flowList {
		statusLabel := green.Render("🟢 healthy")
		incidentCount := 0
		reasons := ""
		if m.flowStatuses != nil && m.flowStatuses[flowItem.ID] {
			statusLabel = red.Render("🔴 degraded")
		}
		if m.flowIncidents != nil {
			incidentCount = m.flowIncidents[flowItem.ID]
		}
		if m.flowReasons != nil && len(m.flowReasons[flowItem.ID]) > 0 {
			reasons = " (" + strings.Join(m.flowReasons[flowItem.ID], ", ") + ")"
		}
		countLabel := fmt.Sprintf("(%d incident", incidentCount)
		if incidentCount != 1 {
			countLabel += "s"
		}
		countLabel += ")"
		sb.WriteString(fmt.Sprintf("%s  %s %s%s\n", flowItem.DisplayName(), statusLabel, countLabel, reasons))
	}
	return sb.String()
}

func (m tuiModel) renderHelp() string {
	var sb strings.Builder

	sb.WriteString(titleStyle.Render("Health-Monitor Help"))
	sb.WriteString("\n")
	if m.report.IsDemo {
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true).Render("[ DEMO MODE ON ]\n"))
		sb.WriteString(markdownBlock("## 🎯 Demo Mode Features"))
		sb.WriteString("1. Notice the 'Suggested' incident from the SLO Breach.\n")
		sb.WriteString("2. Notice the 'redis_cache' incident with grouped deduplicated alerts.\n")
		sb.WriteString("3. Use the Incident Browser 'i' to view RCA, Action Items, and Similar incidents.\n\n")
	}
	sb.WriteString(disclaimerStyle.Render("Setup guidance for API latency checks (Prometheus)."))
	sb.WriteString("\n\n")

	sb.WriteString(markdownBlock("## ✅ What You Need"))
	sb.WriteString("- Prometheus URL reachable from this server\n")
	sb.WriteString("- API service name (e.g., payment)\n")
	sb.WriteString("- Optional: API route/path if you want per-endpoint metrics\n")
	sb.WriteString("- If Prometheus requires auth: a read-only token or username/password\n\n")

	sb.WriteString(markdownBlock("## 🔧 How To Configure"))
	sb.WriteString("URL-only mode: set PROMETHEUS_URL and run (auto-detect enabled)\n")
	sb.WriteString("Optional: edit `/etc/health-monitor/config.json` for overrides\n")
	sb.WriteString("\nOr set environment variables:\n")
	sb.WriteString("PROMETHEUS_URL, PROMETHEUS_TOKEN\n")
	sb.WriteString("PROMETHEUS_USER, PROMETHEUS_PASS\n")
	sb.WriteString("API_SERVICE, API_ROUTE\n")
	sb.WriteString("API_SERVICE_LABEL, API_ROUTE_LABEL\n")
	sb.WriteString("API_LATENCY_METRIC, API_REQUESTS_METRIC\n")
	sb.WriteString("API_AUTO_DISCOVER (true/false)\n")
	sb.WriteString("API_ERROR_LABEL, API_ERROR_REGEX\n")
	sb.WriteString("API_LATENCY_THRESHOLD, PROMETHEUS_WINDOW\n")
	sb.WriteString("PROMETHEUS_EXTRA_LABELS\n\n")
	sb.WriteString("API_ROUTE_CARDINALITY_LIMIT, API_SERVICE_CARDINALITY_LIMIT\n")
	sb.WriteString("  (0 disables cardinality guardrails)\n\n")
	sb.WriteString("HEALTH_MONITOR_EXPORT_PATH\n\n")
	sb.WriteString("PROMETHEUS_QPS, LOKI_QPS\n")
	sb.WriteString("  (0 disables rate limiting)\n\n")

	sb.WriteString(markdownBlock("## ❓ Why No Data?"))
	sb.WriteString("- Prometheus scrape target down or stale data\n")
	sb.WriteString("- Metric/label names don’t match your setup\n")
	sb.WriteString("- Window too small for low-traffic services\n")
	sb.WriteString("Use Auto-Detect Reasoning and Data Gaps sections for fixes.\n\n")
	sb.WriteString("HEALTH_MONITOR_DISABLE_UPDATES\n\n")

	sb.WriteString(markdownBlock("## 📜 Loki Logs (Optional)"))
	sb.WriteString("LOKI_URL, LOKI_TOKEN\n")
	sb.WriteString("LOKI_USER, LOKI_PASS\n")
	sb.WriteString("LOKI_SERVICE_LABEL, LOKI_ROUTE_LABEL\n")
	sb.WriteString("LOKI_ERROR_REGEX, LOKI_WINDOW\n\n")

	sb.WriteString(markdownBlock("## 📓 Incidents"))
	sb.WriteString("Press 'i' to view the incident timeline and history.\n\n")

	sb.WriteString(markdownBlock("## 📊 Opinionated Views"))
	sb.WriteString("Press 'U' (Shift+U) for Cluster Health Overview.\n")
	sb.WriteString("Press 'P' (Shift+P) for Service Drilldown (Performance).\n\n")

	sb.WriteString(markdownBlock("## 🔁 Flows"))
	sb.WriteString("Press 'f' to view flows and incident impact.\n\n")

	sb.WriteString(markdownBlock("## 🔗 Grafana (Optional)"))
	sb.WriteString("GRAFANA_URL\n")
	sb.WriteString("GRAFANA_PROM_DS, GRAFANA_LOKI_DS\n\n")
	sb.WriteString(markdownBlock("## 🧭 Tracing (Optional)"))
	sb.WriteString("Press 't' for Tempo config, 'j' for Jaeger config\n")
	sb.WriteString("TRACE_BACKEND, TRACE_URL\n")
	sb.WriteString("TRACE_TOKEN, TRACE_USER, TRACE_PASS\n")
	sb.WriteString("TRACE_SERVICE_MAP, TRACE_DEFAULT_SERVICE, TRACE_WINDOW\n")
	sb.WriteString("TRACE_MIN_DURATION_MS, TRACE_EXCLUDE_SYSTEM_ROUTES\n\n")
	sb.WriteString(markdownBlock("## 🔍 Correlation Settings"))
	sb.WriteString("Press 'z' for correlation settings\n")
	sb.WriteString(markdownBlock("## 🧰 Logs Backend"))
	sb.WriteString("Press 'b' for backend selector\n")
	sb.WriteString("Press 'e' for Elasticsearch settings\n")
	sb.WriteString("CORRELATION_MIN_SAMPLES, CORRELATION_MAX_LOGS\n")
	sb.WriteString("CORRELATION_MAX_RESULTS, CORRELATION_WINDOW\n")
	sb.WriteString("CORRELATION_BEST_EFFORT\n\n")

	sb.WriteString(markdownBlock("## 📤 Export Report"))
	sb.WriteString("From the main screen:\n")
	sb.WriteString("- Press 'E' to export Markdown + JSON to /tmp\n")
	sb.WriteString("- Press 'M' to export Markdown only\n")
	sb.WriteString("- Press 'J' to export JSON only\n\n")

	sb.WriteString(markdownBlock("## 🧭 Where To Get These Values"))
	sb.WriteString("- Prometheus URL: your platform/observability team or internal docs\n")
	sb.WriteString("- Token/credentials: request a read-only Prometheus token\n")
	sb.WriteString("- Service/route labels: check your Prometheus metrics naming\n\n")

	sb.WriteString(markdownBlock("## 🔐 Token File (Recommended)"))
	sb.WriteString("Create a token file with read-only access:\n")
	sb.WriteString("- System (root): /etc/health-monitor/prometheus.token\n")
	userTokenPath := ""
	if userConfigPath, err := config.UserConfigPath(); err == nil && userConfigPath != "" {
		userTokenPath = filepath.Join(filepath.Dir(userConfigPath), "prometheus.token")
	}
	if userTokenPath != "" {
		sb.WriteString("- User: " + userTokenPath + "\n")
	}
	sb.WriteString("Set permissions to 0600.\n")

	sb.WriteString(markdownBlock("## 🧪 Common Defaults"))
	sb.WriteString("- Service label: `service`\n")
	sb.WriteString("- Route label: `route`\n")
	sb.WriteString("- Latency metric: `http_request_duration_seconds`\n")
	sb.WriteString("- Request count: `http_requests_total`\n")
	sb.WriteString("- Error label/regex: `status=~\"5..\"`\n")
	sb.WriteString("- Auto-discover: set `API_AUTO_DISCOVER=true` or use URL-only mode\n")

	return sb.String()
}

func (m tuiModel) renderServices() string {
	var sb strings.Builder

	sb.WriteString(titleStyle.Render("API Service Selector"))
	sb.WriteString("\n")
	sb.WriteString(disclaimerStyle.Render("These are detected services. Set API_SERVICE and rerun to focus a single service."))
	sb.WriteString("\n\n")

	if len(m.serviceList) == 0 {
		sb.WriteString(dimStyle.Render("No services detected. Ensure your request metric has a service label."))
		sb.WriteString("\n")
		if strings.TrimSpace(m.serviceMessage) != "" {
			sb.WriteString("\n")
			sb.WriteString(dimStyle.Render(m.serviceMessage))
			sb.WriteString("\n")
		}
		return sb.String()
	}

	sb.WriteString(markdownBlock("## Top Services (RPS)"))
	for i, svc := range m.serviceList {
		prefix := "  "
		if i == m.serviceIndex {
			prefix = "> "
		}
		sb.WriteString(prefix + valueStyle.Render(svc))
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	sb.WriteString(markdownBlock("## How To Filter"))
	sb.WriteString("Press Enter to apply filter live, Esc to return to main screen,\n")
	sb.WriteString("or set API_SERVICE and rerun:\n")
	sb.WriteString("API_SERVICE=<service-name> health-monitor\n")
	if strings.TrimSpace(m.serviceMessage) != "" {
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render(m.serviceMessage))
		sb.WriteString("\n")
	}

	return sb.String()
}

func (m tuiModel) renderEndpoints() string {
	var sb strings.Builder

	sb.WriteString(titleStyle.Render("API Endpoint Selector"))
	sb.WriteString("\n")
	sb.WriteString(disclaimerStyle.Render("These are detected endpoints. Set API_ROUTE and rerun to focus a single endpoint."))
	sb.WriteString("\n\n")

	if len(m.endpointList) == 0 {
		sb.WriteString(dimStyle.Render("No endpoints detected. Ensure your request metric has an endpoint/route label."))
		sb.WriteString("\n")
		if strings.TrimSpace(m.endpointMessage) != "" {
			sb.WriteString("\n")
			sb.WriteString(dimStyle.Render(m.endpointMessage))
			sb.WriteString("\n")
		}
		return sb.String()
	}

	sb.WriteString(markdownBlock("## Top Endpoints"))
	for i, ep := range m.endpointList {
		prefix := "  "
		if i == m.endpointIndex {
			prefix = "> "
		}
		sb.WriteString(prefix + valueStyle.Render(ep))
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	sb.WriteString(markdownBlock("## How To Filter"))
	sb.WriteString("Press Enter to apply filter live, Esc to return to main screen,\n")
	sb.WriteString("or set API_ROUTE and rerun:\n")
	sb.WriteString("API_ROUTE=<endpoint> health-monitor\n")
	if strings.TrimSpace(m.endpointMessage) != "" {
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render(m.endpointMessage))
		sb.WriteString("\n")
	}

	return sb.String()
}

func (m tuiModel) startLogsView() (tuiModel, tea.Cmd) {
	m.showLogs = true
	m.showHelp = false
	m.showIncidents = false
	m.showFlows = false
	m.showConfig = false
	m.editActive = false
	m.showServices = false
	m.showEndpoints = false
	m.showLokiConfig = false
	m.showGrafanaConfig = false
	m.showPromConfig = false
	m.showTempoConfig = false
	m.showJaegerConfig = false
	m.showCorrelationConfig = false
	m.showBackendSelector = false
	m.showElasticConfig = false
	m.authEditActive = false
	m.scrollY = 0
	m.logLoading = true
	m.logEntries = nil
	m.logMessage = ""

	cfg, _ := config.Load()
	if cfg.LogBackend == "elastic" {
		return m, tea.Batch(m.spinner.Tick, m.fetchElasticLogsCmd())
	}
	return m, tea.Batch(m.spinner.Tick, m.fetchLokiLogsCmd())
}

func (m tuiModel) updateLogs(msg tea.Msg) (tuiModel, tea.Cmd) {
	switch key := msg.(type) {
	case tea.KeyMsg:
		if key.Type == tea.KeyRunes && looksLikeTerminalGarbage(key.String()) {
			if len(m.authInputs) > 0 {
				m.sanitizeAuthInputs()
			}
			m.content = m.renderLogs()
			return m, nil
		}
		switch key.String() {
		case "esc":
			m.showLogs = false
			m.logLoading = false
			m.scrollY = 0
			m.content = m.renderContent()
			return m, nil
		case "q":
			m.showLogs = false
			m.logLoading = false
			m.scrollY = 0
			m.content = m.renderContent()
			return m, nil
		case "enter":
			if m.showLogs {
				m.logEntries = nil
				m.logLoading = true
				m.logMessage = "Refreshing logs..."
				cfg, _ := config.Load()
				if cfg.LogBackend == "elastic" {
					return m, m.fetchElasticLogsCmd()
				}
				return m, m.fetchLokiLogsCmd()
			}
		case "y", "Y":
			if strings.TrimSpace(m.logLink) == "" {
				m.logMessage = "Grafana link not ready yet."
				m.content = m.renderLogs()
				return m, nil
			}
			if err := copyToClipboard(m.logLink); err != nil {
				if m.logLinkPath != "" {
					m.logMessage = "Clipboard copy failed. Link saved: " + m.logLinkPath
				} else {
					m.logMessage = "Clipboard copy failed: " + err.Error()
				}
			} else {
				m.logMessage = "Grafana link copied to clipboard."
			}
			m.content = m.renderLogs()
			return m, nil
		}
	}
	return m, nil
}

func (m *tuiModel) renderLogs() string {
	var sb strings.Builder
	cfg, _ := config.Load()
	title := "Latest Loki Logs"
	if cfg.LogBackend == "elastic" {
		title = "Latest Elasticsearch Logs"
	}
	sb.WriteString(titleStyle.Render(title))
	sb.WriteString("\n")
	sb.WriteString(disclaimerStyle.Render("Auto-detected labels; override in Loki config if needed."))
	sb.WriteString("\n\n")

	cfg, _ = config.Load()
	if cfg.LokiURL == "" && !m.report.IsDemo {
		sb.WriteString(dimStyle.Render("Loki URL not set. Press 'o' to configure Loki connection."))
		sb.WriteString("\n")
		return sb.String()
	}
	if cfg.LokiURL != "" {
		sb.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Loki URL"), valueStyle.Render(cfg.LokiURL)))
	} else if m.report.IsDemo {
		sb.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Loki URL"), valueStyle.Render("demo-simulated-loki")))
	}

	service := ""
	route := ""
	if m.report.APILatency != nil {
		service = m.report.APILatency.Service
		route = m.report.APILatency.Route
	}
	if strings.TrimSpace(service) == "" {
		service = "all services"
	}
	if strings.TrimSpace(route) == "" {
		route = "all routes"
	}
	sb.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Service"), valueStyle.Render(service)))
	sb.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Route"), valueStyle.Render(route)))

	window := cfg.LokiWindow
	if window == "" {
		window = "10m (default)"
	}
	sb.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Window"), valueStyle.Render(window)))
	sb.WriteString("\n")

	if strings.TrimSpace(m.logQuery) != "" {
		sb.WriteString(markdownBlock("### Query"))
		sb.WriteString(m.logQuery + "\n")
		if hint := buildAPMLogHint(m.report.APM); hint != "" {
			sb.WriteString(dimStyle.Render(hint))
			sb.WriteString("\n")
		}
		cfg, _ := config.Load()
		lokiUID, lokiType := m.resolveGrafanaDatasource(cfg, cfg.GrafanaLokiDataSource)
		link := output.BuildGrafanaExploreURL(cfg.GrafanaURL, cfg.GrafanaLokiDataSource, lokiUID, lokiType, m.logQuery, "now-"+cfg.LokiWindow, "now")
		if link != "" {
			sb.WriteString(dimStyle.Render("Grafana link (Loki Explore):"))
			sb.WriteString("\n")
			sb.WriteString(renderWrappedLink(link, m.width))
			sb.WriteString("\n")
			if strings.TrimSpace(m.logLinkPath) != "" {
				sb.WriteString(dimStyle.Render("Saved Grafana link: " + m.logLinkPath))
				sb.WriteString("\n")
			}
			sb.WriteString(dimStyle.Render("Press 'y' to copy Grafana link."))
			sb.WriteString("\n")
		} else {
			switch {
			case strings.TrimSpace(cfg.GrafanaURL) == "":
				sb.WriteString(dimStyle.Render("Grafana URL not set; set GRAFANA_URL to enable Explore links."))
				sb.WriteString("\n")
			case strings.TrimSpace(cfg.GrafanaLokiDataSource) == "":
				sb.WriteString(dimStyle.Render("Grafana Loki datasource not set; set GRAFANA_LOKI_DS to enable Explore links."))
				sb.WriteString("\n")
			case strings.TrimSpace(lokiUID) == "" || strings.TrimSpace(lokiType) == "":
				sb.WriteString(dimStyle.Render("Grafana Loki datasource lookup failed; set GRAFANA_TOKEN or Grafana user/pass."))
				sb.WriteString("\n")
			case !strings.EqualFold(lokiType, "loki"):
				sb.WriteString(dimStyle.Render("Grafana Loki datasource not configured; set GRAFANA_LOKI_DS."))
				sb.WriteString("\n")
			}
		}
	}
	if strings.TrimSpace(m.logAutoNote) != "" {
		sb.WriteString(dimStyle.Render(m.logAutoNote))
		sb.WriteString("\n")
	}
	if strings.TrimSpace(m.logMessage) != "" {
		sb.WriteString(dimStyle.Render(m.logMessage))
		sb.WriteString("\n")
	}

	if m.logLoading {
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render("Loading logs " + m.spinner.View()))
		sb.WriteString("\n")
		return sb.String()
	}

	if len(m.logEntries) == 0 {
		if strings.TrimSpace(m.logMessage) != "" {
			return sb.String()
		}
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render("No logs found in the selected window."))
		sb.WriteString("\n")
		return sb.String()
	}

	sb.WriteString("\n")
	sb.WriteString(markdownBlock("### Recent Logs"))
	sb.WriteString(dimStyle.Render("Tip: use ↑/↓ or PgUp/PgDn to scroll."))
	sb.WriteString("\n")
	wrapWidth := m.width - 6
	if wrapWidth < 40 {
		wrapWidth = 80
	}
	for _, entry := range m.logEntries {
		line := output.SanitizeLogLine(entry.Line)
		ts := entry.Timestamp.Format("15:04:05")
		sb.WriteString(formatWrappedLog(ts, line, wrapWidth))
		sb.WriteString("\n")
	}
	return sb.String()
}

func (m *tuiModel) startGrafanaEditor() {
	cfg, _ := config.Load()
	m.grafanaInputs = make([]textinput.Model, 6)

	urlInput := textinput.New()
	urlInput.Prompt = ""
	urlInput.Placeholder = "https://grafana.example.com"
	urlInput.CharLimit = 256
	urlInput.Width = 60
	urlInput.SetValue(sanitizeInput(cfg.GrafanaURL))

	tokenInput := textinput.New()
	tokenInput.Prompt = ""
	tokenInput.Placeholder = "Grafana token (stored in " + config.GrafanaTokenFilePath() + ")"
	tokenInput.CharLimit = 256
	tokenInput.Width = 60
	tokenInput.EchoMode = textinput.EchoPassword
	tokenInput.EchoCharacter = '*'
	tokenInput.SetValue("")
	tokenInput.SetCursor(0)
	tokenInput.Blur()

	userInput := textinput.New()
	userInput.Prompt = ""
	userInput.Placeholder = "Grafana username (optional)"
	userInput.CharLimit = 128
	userInput.Width = 60
	userInput.SetValue(sanitizeInputLoose(cfg.GrafanaUser))

	passInput := textinput.New()
	passInput.Prompt = ""
	passInput.Placeholder = "Grafana password (optional)"
	passInput.CharLimit = 128
	passInput.Width = 60
	passInput.EchoMode = textinput.EchoPassword
	passInput.EchoCharacter = '*'
	passInput.SetValue("")
	passInput.SetCursor(0)
	passInput.Blur()

	promInput := textinput.New()
	promInput.Prompt = ""
	promInput.Placeholder = "Prometheus datasource name (optional)"
	promInput.CharLimit = 64
	promInput.Width = 60
	promInput.SetValue(sanitizeInputLoose(cfg.GrafanaPromDataSource))

	lokiInput := textinput.New()
	lokiInput.Prompt = ""
	lokiInput.Placeholder = "Loki datasource name (optional)"
	lokiInput.CharLimit = 64
	lokiInput.Width = 60
	lokiInput.SetValue(sanitizeInputLoose(cfg.GrafanaLokiDataSource))

	m.grafanaInputs[0] = urlInput
	m.grafanaInputs[1] = tokenInput
	m.grafanaInputs[2] = userInput
	m.grafanaInputs[3] = passInput
	m.grafanaInputs[4] = promInput
	m.grafanaInputs[5] = lokiInput
	m.grafanaIndex = 0
	m.grafanaInputs[0].Focus()
	m.showGrafanaConfig = true
	m.grafanaMessage = ""
}

func (m tuiModel) renderGrafanaEditor() string {
	var sb strings.Builder
	sb.WriteString(titleStyle.Render("Grafana Link-Outs"))
	sb.WriteString("\n")
	sb.WriteString(disclaimerStyle.Render("Configure Grafana base URL, auth, and datasource names."))
	sb.WriteString("\n\n")

	sb.WriteString(keyStyle.Render("Grafana URL"))
	sb.WriteString("\n  " + m.grafanaInputs[0].View() + "\n\n")

	sb.WriteString(keyStyle.Render("Grafana Token (Optional)"))
	sb.WriteString("\n  " + m.grafanaInputs[1].View() + "\n")
	sb.WriteString(dimStyle.Render("  Stored in " + config.GrafanaTokenFilePath()))
	sb.WriteString("\n\n")

	sb.WriteString(keyStyle.Render("Grafana Username (Optional)"))
	sb.WriteString("\n  " + m.grafanaInputs[2].View() + "\n\n")

	sb.WriteString(keyStyle.Render("Grafana Password (Optional)"))
	sb.WriteString("\n  " + m.grafanaInputs[3].View() + "\n")
	sb.WriteString(dimStyle.Render("  Stored in " + config.GrafanaBasicAuthFilePath()))
	sb.WriteString("\n\n")

	sb.WriteString(keyStyle.Render("Prometheus Datasource (Optional)"))
	sb.WriteString("\n  " + m.grafanaInputs[4].View() + "\n\n")

	sb.WriteString(keyStyle.Render("Loki Datasource (Optional)"))
	sb.WriteString("\n  " + m.grafanaInputs[5].View() + "\n\n")

	sb.WriteString(keyStyle.Render("Actions"))
	sb.WriteString("\nEnter to save, Esc to return.\n")
	sb.WriteString("Ctrl+T clears Grafana token file • Ctrl+B clears Grafana basic auth file\n")

	if strings.TrimSpace(m.grafanaMessage) != "" {
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render(m.grafanaMessage))
		sb.WriteString("\n")
	}
	return sb.String()
}

func (m tuiModel) updateGrafanaEditor(msg tea.Msg) (tuiModel, tea.Cmd) {
	switch key := msg.(type) {
	case tea.KeyMsg:
		switch key.String() {
		case "esc", "q":
			m.showGrafanaConfig = false
			m.scrollY = 0
			m.content = m.renderContent()
			return m, nil
		case "ctrl+t":
			if err := config.ClearGrafanaTokenFile(); err != nil {
				m.grafanaMessage = "Failed to clear Grafana token file: " + err.Error()
			} else {
				m.grafanaMessage = "Grafana token cleared."
			}
			m.content = m.renderGrafanaEditor()
			return m, nil
		case "ctrl+b":
			if err := config.ClearGrafanaBasicAuthFile(); err != nil {
				m.grafanaMessage = "Failed to clear Grafana basic auth file: " + err.Error()
			} else {
				m.grafanaMessage = "Grafana basic auth cleared."
			}
			m.content = m.renderGrafanaEditor()
			return m, nil
		case "enter":
			if m.grafanaIndex == len(m.grafanaInputs)-1 {
				m.applyGrafanaUpdates()
				m.content = m.renderGrafanaEditor()
				return m, nil
			}
			m.grafanaInputs[m.grafanaIndex].Blur()
			m.grafanaIndex++
			m.grafanaInputs[m.grafanaIndex].Focus()
			m.content = m.renderGrafanaEditor()
			return m, nil
		case "tab", "down":
			m.grafanaInputs[m.grafanaIndex].Blur()
			m.grafanaIndex = (m.grafanaIndex + 1) % len(m.grafanaInputs)
			m.grafanaInputs[m.grafanaIndex].Focus()
			m.content = m.renderGrafanaEditor()
			return m, nil
		case "shift+tab", "up":
			m.grafanaInputs[m.grafanaIndex].Blur()
			m.grafanaIndex--
			if m.grafanaIndex < 0 {
				m.grafanaIndex = len(m.grafanaInputs) - 1
			}
			m.grafanaInputs[m.grafanaIndex].Focus()
			m.content = m.renderGrafanaEditor()
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.grafanaInputs[m.grafanaIndex], cmd = m.grafanaInputs[m.grafanaIndex].Update(msg)
	m.content = m.renderGrafanaEditor()
	return m, cmd
}

func (m *tuiModel) applyGrafanaUpdates() {
	url := sanitizeInput(m.grafanaInputs[0].Value())
	token := sanitizeTokenInput(m.grafanaInputs[1].Value())
	user := sanitizeTokenInput(m.grafanaInputs[2].Value())
	pass := sanitizeTokenInput(m.grafanaInputs[3].Value())
	promDS := strings.TrimSpace(sanitizeInputLoose(m.grafanaInputs[4].Value()))
	lokiDS := strings.TrimSpace(sanitizeInputLoose(m.grafanaInputs[5].Value()))

	cfg, err := config.Load()
	if err != nil {
		m.grafanaMessage = "Failed to load config: " + err.Error()
		return
	}
	cfg.GrafanaURL = url
	cfg.GrafanaUser = strings.TrimSpace(user)
	cfg.GrafanaPass = strings.TrimSpace(pass)
	cfg.GrafanaPromDataSource = promDS
	cfg.GrafanaLokiDataSource = lokiDS
	if cfg.GrafanaUser != "" || cfg.GrafanaPass != "" {
		cfg.GrafanaToken = ""
	}
	if err := saveCurrentConfig(cfg); err != nil {
		m.grafanaMessage = "Failed to save config: " + err.Error()
		return
	}
	if token != "" {
		if err := config.WriteGrafanaTokenFile(token); err != nil {
			m.grafanaMessage = "Failed to write Grafana token: " + err.Error()
			return
		}
		cfg.GrafanaToken = token
	}
	if user != "" || pass != "" {
		if err := config.WriteGrafanaBasicAuthFile(user, pass); err != nil {
			m.grafanaMessage = "Failed to write Grafana basic auth: " + err.Error()
			return
		}
	}
	m.grafanaMessage = "Saved Grafana configuration."
	m.updatePromLink()
}

func (m *tuiModel) startAuthEditor() {
	cfg, _ := config.Load()
	m.authInputs = make([]textinput.Model, 16)
	m.scrollY = 0

	urlInput := textinput.New()
	urlInput.Prompt = ""
	urlInput.Placeholder = "https://prom.example.com"
	urlInput.CharLimit = 256
	urlInput.Width = 60
	cleanURL := sanitizeInput(cfg.PrometheusURL)
	if cleanURL != cfg.PrometheusURL && cleanURL == "" && strings.TrimSpace(cfg.PrometheusURL) != "" {
		m.authMessage = "URL contained invalid characters; please re-enter."
	} else {
		m.authMessage = ""
	}
	urlInput.SetValue(cleanURL)

	tokenInput := textinput.New()
	tokenInput.Prompt = ""
	tokenInput.Placeholder = "Bearer token (stored in " + config.TokenFilePath() + ")"
	tokenInput.CharLimit = 256
	tokenInput.Width = 60
	tokenInput.EchoMode = textinput.EchoPassword
	tokenInput.EchoCharacter = '*'
	tokenInput.SetValue("")
	tokenInput.SetCursor(0)
	tokenInput.Blur()
	tokenInput.Prompt = ""

	userInput := textinput.New()
	userInput.Prompt = ""
	userInput.Placeholder = "Basic auth username (optional)"
	userInput.CharLimit = 128
	userInput.Width = 60
	userInput.SetValue("")

	passInput := textinput.New()
	passInput.Prompt = ""
	passInput.Placeholder = "Basic auth password (optional)"
	passInput.CharLimit = 128
	passInput.Width = 60
	passInput.EchoMode = textinput.EchoPassword
	passInput.EchoCharacter = '*'
	passInput.SetValue("")
	passInput.SetCursor(0)
	passInput.Blur()

	disableInput := textinput.New()
	disableInput.Prompt = ""
	disableInput.Placeholder = "true/false"
	disableInput.CharLimit = 5
	disableInput.Width = 10
	if cfg.DisableUpdates {
		disableInput.SetValue("true")
	} else {
		disableInput.SetValue("false")
	}
	disableInput.Blur()

	qpsInput := textinput.New()
	qpsInput.Prompt = ""
	qpsInput.Placeholder = "Prometheus QPS (0=unlimited)"
	qpsInput.CharLimit = 16
	qpsInput.Width = 20
	qpsInput.SetValue(strconv.FormatFloat(cfg.PrometheusQPS, 'f', -1, 64))
	qpsInput.Blur()

	routeLimitInput := textinput.New()
	routeLimitInput.Prompt = ""
	routeLimitInput.Placeholder = "Route cardinality limit (e.g., 500)"
	routeLimitInput.CharLimit = 8
	routeLimitInput.Width = 20
	routeLimitInput.SetValue(strconv.Itoa(cfg.RouteCardinalityLimit))
	routeLimitInput.Blur()

	serviceLimitInput := textinput.New()
	serviceLimitInput.Prompt = ""
	serviceLimitInput.Placeholder = "Service cardinality limit (e.g., 200)"
	serviceLimitInput.CharLimit = 8
	serviceLimitInput.Width = 20
	serviceLimitInput.SetValue(strconv.Itoa(cfg.ServiceCardinalityLimit))
	serviceLimitInput.Blur()

	traceBackendInput := textinput.New()
	traceBackendInput.Prompt = ""
	traceBackendInput.Placeholder = "Trace backend (tempo/jaeger)"
	traceBackendInput.CharLimit = 16
	traceBackendInput.Width = 20
	traceBackendInput.SetValue(cfg.TraceBackend)
	traceBackendInput.Blur()

	traceURLInput := textinput.New()
	traceURLInput.Prompt = ""
	traceURLInput.Placeholder = "Trace URL (e.g., https://tempo.example.com)"
	traceURLInput.CharLimit = 256
	traceURLInput.Width = 60
	traceURLInput.SetValue(sanitizeInput(cfg.TraceURL))
	traceURLInput.Blur()

	traceTokenInput := textinput.New()
	traceTokenInput.Prompt = ""
	traceTokenInput.Placeholder = "Trace token (stored in " + config.TraceTokenFilePath() + ")"
	traceTokenInput.CharLimit = 256
	traceTokenInput.Width = 60
	traceTokenInput.EchoMode = textinput.EchoPassword
	traceTokenInput.EchoCharacter = '*'
	traceTokenInput.SetValue("")
	traceTokenInput.SetCursor(0)
	traceTokenInput.Blur()

	traceUserInput := textinput.New()
	traceUserInput.Prompt = ""
	traceUserInput.Placeholder = "Trace basic auth username (optional)"
	traceUserInput.CharLimit = 128
	traceUserInput.Width = 60
	traceUserInput.SetValue("")
	traceUserInput.Blur()

	tracePassInput := textinput.New()
	tracePassInput.Prompt = ""
	tracePassInput.Placeholder = "Trace basic auth password (optional)"
	tracePassInput.CharLimit = 128
	tracePassInput.Width = 60
	tracePassInput.EchoMode = textinput.EchoPassword
	tracePassInput.EchoCharacter = '*'
	tracePassInput.SetValue("")
	tracePassInput.SetCursor(0)
	tracePassInput.Blur()

	traceWindowInput := textinput.New()
	traceWindowInput.Prompt = ""
	traceWindowInput.Placeholder = "Trace window (e.g., 4m)"
	traceWindowInput.CharLimit = 16
	traceWindowInput.Width = 20
	traceWindowInput.SetValue(cfg.TraceWindow)
	traceWindowInput.Blur()

	traceMapInput := textinput.New()
	traceMapInput.Prompt = ""
	traceMapInput.Placeholder = "Trace service map (metrics=trace,...)"
	traceMapInput.CharLimit = 256
	traceMapInput.Width = 60
	traceMapInput.SetValue(cfg.TraceServiceMap)
	traceMapInput.Blur()

	traceDefaultInput := textinput.New()
	traceDefaultInput.Prompt = ""
	traceDefaultInput.Placeholder = "Trace default service (optional)"
	traceDefaultInput.CharLimit = 128
	traceDefaultInput.Width = 60
	traceDefaultInput.SetValue(cfg.TraceDefaultService)
	traceDefaultInput.Blur()

	m.authInputs[0] = urlInput
	m.authInputs[1] = tokenInput
	m.authInputs[2] = userInput
	m.authInputs[3] = passInput
	m.authInputs[4] = disableInput
	m.authInputs[5] = qpsInput
	m.authInputs[6] = routeLimitInput
	m.authInputs[7] = serviceLimitInput
	m.authInputs[8] = traceBackendInput
	m.authInputs[9] = traceURLInput
	m.authInputs[10] = traceTokenInput
	m.authInputs[11] = traceUserInput
	m.authInputs[12] = tracePassInput
	m.authInputs[13] = traceWindowInput
	m.authInputs[14] = traceMapInput
	m.authInputs[15] = traceDefaultInput
	m.authIndex = 0
	m.authInputs[0].Focus()
	m.authEditActive = true
	m.authTestResult = ""
	m.sanitizeAuthInputs()
	m.ensureAuthVisible()
}

func sanitizeInputModels(inputs []textinput.Model) {
	for i := range inputs {
		value := inputs[i].Value()
		cleaned := stripTerminalArtifacts(value)
		if cleaned != value {
			inputs[i].SetValue(cleaned)
			value = cleaned
		}
		if looksLikeTerminalGarbage(value) {
			inputs[i].SetValue("")
		}
	}
}

func (m *tuiModel) startPromEditor() {
	cfg, _ := config.Load()
	m.promInputs = make([]textinput.Model, 8)
	m.scrollY = 0

	urlInput := textinput.New()
	urlInput.Prompt = ""
	urlInput.Placeholder = "https://prom.example.com"
	urlInput.CharLimit = 256
	urlInput.Width = 60
	cleanURL := sanitizeInput(cfg.PrometheusURL)
	urlInput.SetValue(cleanURL)

	tokenInput := textinput.New()
	tokenInput.Prompt = ""
	tokenInput.Placeholder = "Bearer token (stored in " + config.TokenFilePath() + ")"
	tokenInput.CharLimit = 256
	tokenInput.Width = 60
	tokenInput.EchoMode = textinput.EchoPassword
	tokenInput.EchoCharacter = '*'
	tokenInput.SetValue("")
	tokenInput.SetCursor(0)
	tokenInput.Blur()

	userInput := textinput.New()
	userInput.Prompt = ""
	userInput.Placeholder = "Basic auth username (optional)"
	userInput.CharLimit = 128
	userInput.Width = 60
	userInput.SetValue("")

	passInput := textinput.New()
	passInput.Prompt = ""
	passInput.Placeholder = "Basic auth password (optional)"
	passInput.CharLimit = 128
	passInput.Width = 60
	passInput.EchoMode = textinput.EchoPassword
	passInput.EchoCharacter = '*'
	passInput.SetValue("")
	passInput.SetCursor(0)
	passInput.Blur()

	disableInput := textinput.New()
	disableInput.Prompt = ""
	disableInput.Placeholder = "true/false"
	disableInput.CharLimit = 5
	disableInput.Width = 10
	if cfg.DisableUpdates {
		disableInput.SetValue("true")
	} else {
		disableInput.SetValue("false")
	}
	disableInput.Blur()

	qpsInput := textinput.New()
	qpsInput.Prompt = ""
	qpsInput.Placeholder = "Prometheus QPS (0=unlimited)"
	qpsInput.CharLimit = 16
	qpsInput.Width = 20
	qpsInput.SetValue(strconv.FormatFloat(cfg.PrometheusQPS, 'f', -1, 64))
	qpsInput.Blur()

	routeLimitInput := textinput.New()
	routeLimitInput.Prompt = ""
	routeLimitInput.Placeholder = "Route cardinality limit (e.g., 500)"
	routeLimitInput.CharLimit = 8
	routeLimitInput.Width = 20
	routeLimitInput.SetValue(strconv.Itoa(cfg.RouteCardinalityLimit))
	routeLimitInput.Blur()

	serviceLimitInput := textinput.New()
	serviceLimitInput.Prompt = ""
	serviceLimitInput.Placeholder = "Service cardinality limit (e.g., 200)"
	serviceLimitInput.CharLimit = 8
	serviceLimitInput.Width = 20
	serviceLimitInput.SetValue(strconv.Itoa(cfg.ServiceCardinalityLimit))
	serviceLimitInput.Blur()

	m.promInputs[0] = urlInput
	m.promInputs[1] = tokenInput
	m.promInputs[2] = userInput
	m.promInputs[3] = passInput
	m.promInputs[4] = disableInput
	m.promInputs[5] = qpsInput
	m.promInputs[6] = routeLimitInput
	m.promInputs[7] = serviceLimitInput
	m.promIndex = 0
	m.promInputs[0].Focus()
	m.showPromConfig = true
	m.promConfigMessage = ""
	m.promConfigTest = ""
	sanitizeInputModels(m.promInputs)
}

func (m *tuiModel) startCorrelationEditor() {
	cfg, _ := config.Load()
	m.correlationInputs = make([]textinput.Model, 5)
	m.scrollY = 0

	minSamplesInput := textinput.New()
	minSamplesInput.Prompt = ""
	minSamplesInput.Placeholder = "Min samples (e.g., 10)"
	minSamplesInput.CharLimit = 6
	minSamplesInput.Width = 12
	minSamplesInput.SetValue(strconv.Itoa(cfg.CorrelationMinSamples))
	minSamplesInput.Blur()

	maxLogsInput := textinput.New()
	maxLogsInput.Prompt = ""
	maxLogsInput.Placeholder = "Max logs (e.g., 200)"
	maxLogsInput.CharLimit = 6
	maxLogsInput.Width = 12
	maxLogsInput.SetValue(strconv.Itoa(cfg.CorrelationMaxLogs))
	maxLogsInput.Blur()

	maxResultsInput := textinput.New()
	maxResultsInput.Prompt = ""
	maxResultsInput.Placeholder = "Max results (e.g., 5)"
	maxResultsInput.CharLimit = 4
	maxResultsInput.Width = 8
	maxResultsInput.SetValue(strconv.Itoa(cfg.CorrelationMaxResults))
	maxResultsInput.Blur()

	windowInput := textinput.New()
	windowInput.Prompt = ""
	windowInput.Placeholder = "Window override (e.g., 30m)"
	windowInput.CharLimit = 16
	windowInput.Width = 20
	windowInput.SetValue(strings.TrimSpace(cfg.CorrelationWindow))
	windowInput.Blur()

	bestEffortInput := textinput.New()
	bestEffortInput.Prompt = ""
	bestEffortInput.Placeholder = "Best effort (true/false)"
	bestEffortInput.CharLimit = 5
	bestEffortInput.Width = 10
	if cfg.CorrelationBestEffort {
		bestEffortInput.SetValue("true")
	} else {
		bestEffortInput.SetValue("false")
	}
	bestEffortInput.Blur()

	m.correlationInputs[0] = minSamplesInput
	m.correlationInputs[1] = maxLogsInput
	m.correlationInputs[2] = maxResultsInput
	m.correlationInputs[3] = windowInput
	m.correlationInputs[4] = bestEffortInput
	m.correlationIndex = 0
	m.correlationInputs[0].Focus()
	m.showCorrelationConfig = true
	m.correlationConfigMsg = ""
	sanitizeInputModels(m.correlationInputs)
}

func (m *tuiModel) startBackendSelector() {
	cfg, _ := config.Load()
	current := strings.TrimSpace(strings.ToLower(cfg.LogBackend))
	if current == "" {
		if strings.TrimSpace(cfg.LokiURL) != "" {
			current = "loki"
		}
	}
	m.backendOptions = []string{"loki", "elastic"}
	m.backendIndex = 0
	for i, option := range m.backendOptions {
		if option == current {
			m.backendIndex = i
			break
		}
	}
	m.showBackendSelector = true
	m.backendMessage = ""
}

func (m tuiModel) renderBackendSelector() string {
	var sb strings.Builder
	sb.WriteString(titleStyle.Render("Logs Backend"))
	sb.WriteString("\n")
	sb.WriteString(disclaimerStyle.Render("Select the backend used for correlation logs."))
	sb.WriteString("\n\n")

	cfg, _ := config.Load()
	current := strings.TrimSpace(strings.ToLower(cfg.LogBackend))
	if current == "" {
		if strings.TrimSpace(cfg.LokiURL) != "" {
			current = "loki"
		}
	}
	sb.WriteString(dimStyle.Render("Current: " + output.EmptyOr(current, "auto")))
	sb.WriteString("\n\n")

	for i, option := range m.backendOptions {
		prefix := "  "
		if i == m.backendIndex {
			prefix = "> "
		}
		sb.WriteString(prefix + option + "\n")
	}

	sb.WriteString("\n")
	sb.WriteString(keyStyle.Render("Actions"))
	sb.WriteString("\nEnter to select, Esc to return.\n")
	if strings.EqualFold(m.backendOptions[m.backendIndex], "elastic") && strings.TrimSpace(cfg.ElasticURL) == "" {
		sb.WriteString(dimStyle.Render("Elastic URL not set; press 'e' to configure."))
		sb.WriteString("\n")
	}

	if strings.TrimSpace(m.backendMessage) != "" {
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render(m.backendMessage))
		sb.WriteString("\n")
	}
	return sb.String()
}

func (m tuiModel) updateBackendSelector(msg tea.Msg) (tuiModel, tea.Cmd) {
	switch key := msg.(type) {
	case tea.KeyMsg:
		switch key.String() {
		case "esc":
			m.showBackendSelector = false
			m.scrollY = 0
			m.content = m.renderContent()
			return m, nil
		case "q":
			m.showBackendSelector = false
			m.scrollY = 0
			m.content = m.renderContent()
			return m, nil
		case "enter":
			if m.backendIndex < 0 || m.backendIndex >= len(m.backendOptions) {
				return m, nil
			}
			cfg, err := config.Load()
			if err != nil {
				m.backendMessage = "Failed to load config."
				m.content = m.renderBackendSelector()
				return m, nil
			}
			cfg.LogBackend = m.backendOptions[m.backendIndex]
			if err := saveCurrentConfig(cfg); err != nil {
				m.backendMessage = "Failed to save config: " + err.Error()
			} else {
				m.backendMessage = "Saved backend: " + cfg.LogBackend
			}
			m.content = m.renderBackendSelector()
			return m, nil
		case "down", "tab", "j":
			m.backendIndex = (m.backendIndex + 1) % len(m.backendOptions)
			m.content = m.renderBackendSelector()
			return m, nil
		case "up", "k", "shift+tab":
			m.backendIndex--
			if m.backendIndex < 0 {
				m.backendIndex = len(m.backendOptions) - 1
			}
			m.content = m.renderBackendSelector()
			return m, nil
		}
	}
	return m, nil
}

func (m *tuiModel) startElasticEditor() {
	cfg, _ := config.Load()
	m.elasticInputs = make([]textinput.Model, 12)
	m.scrollY = 0

	urlInput := textinput.New()
	urlInput.Prompt = ""
	urlInput.Placeholder = "https://elastic.example.com"
	urlInput.CharLimit = 256
	urlInput.Width = 60
	urlInput.SetValue(sanitizeInput(cfg.ElasticURL))

	indexInput := textinput.New()
	indexInput.Prompt = ""
	indexInput.Placeholder = "Index pattern (e.g., logs-*)"
	indexInput.CharLimit = 128
	indexInput.Width = 40
	indexInput.SetValue(strings.TrimSpace(cfg.ElasticIndex))

	tokenInput := textinput.New()
	tokenInput.Prompt = ""
	tokenInput.Placeholder = "Elastic token (optional)"
	tokenInput.CharLimit = 256
	tokenInput.Width = 60
	tokenInput.EchoMode = textinput.EchoPassword
	tokenInput.EchoCharacter = '*'
	tokenInput.SetValue("")
	tokenInput.SetCursor(0)
	tokenInput.Blur()

	userInput := textinput.New()
	userInput.Prompt = ""
	userInput.Placeholder = "Basic auth username (optional)"
	userInput.CharLimit = 128
	userInput.Width = 60
	userInput.SetValue(strings.TrimSpace(cfg.ElasticUser))

	passInput := textinput.New()
	passInput.Prompt = ""
	passInput.Placeholder = "Basic auth password (optional)"
	passInput.CharLimit = 128
	passInput.Width = 60
	passInput.EchoMode = textinput.EchoPassword
	passInput.EchoCharacter = '*'
	passInput.SetValue("")
	passInput.SetCursor(0)
	passInput.Blur()

	serviceFieldInput := textinput.New()
	serviceFieldInput.Prompt = ""
	serviceFieldInput.Placeholder = "Service field (e.g., service)"
	serviceFieldInput.CharLimit = 64
	serviceFieldInput.Width = 40
	serviceFieldInput.SetValue(strings.TrimSpace(cfg.ElasticServiceField))

	routeFieldInput := textinput.New()
	routeFieldInput.Prompt = ""
	routeFieldInput.Placeholder = "Route field (optional)"
	routeFieldInput.CharLimit = 64
	routeFieldInput.Width = 40
	routeFieldInput.SetValue(strings.TrimSpace(cfg.ElasticRouteField))

	errorFieldInput := textinput.New()
	errorFieldInput.Prompt = ""
	errorFieldInput.Placeholder = "Error/message field (e.g., message)"
	errorFieldInput.CharLimit = 64
	errorFieldInput.Width = 40
	errorFieldInput.SetValue(strings.TrimSpace(cfg.ElasticErrorField))

	timeFieldInput := textinput.New()
	timeFieldInput.Prompt = ""
	timeFieldInput.Placeholder = "Time field (e.g., @timestamp)"
	timeFieldInput.CharLimit = 64
	timeFieldInput.Width = 40
	timeFieldInput.SetValue(strings.TrimSpace(cfg.ElasticTimeField))

	errorRegexInput := textinput.New()
	errorRegexInput.Prompt = ""
	errorRegexInput.Placeholder = "Error regex (optional)"
	errorRegexInput.CharLimit = 128
	errorRegexInput.Width = 40
	errorRegexInput.SetValue(strings.TrimSpace(cfg.ElasticErrorRegex))

	kibanaURLInput := textinput.New()
	kibanaURLInput.Prompt = ""
	kibanaURLInput.Placeholder = "Kibana URL (optional)"
	kibanaURLInput.CharLimit = 256
	kibanaURLInput.Width = 60
	kibanaURLInput.SetValue(sanitizeInput(cfg.KibanaURL))

	kibanaIndexInput := textinput.New()
	kibanaIndexInput.Prompt = ""
	kibanaIndexInput.Placeholder = "Kibana index pattern (optional)"
	kibanaIndexInput.CharLimit = 128
	kibanaIndexInput.Width = 40
	kibanaIndexInput.SetValue(strings.TrimSpace(cfg.KibanaIndex))

	m.elasticInputs[0] = urlInput
	m.elasticInputs[1] = indexInput
	m.elasticInputs[2] = tokenInput
	m.elasticInputs[3] = userInput
	m.elasticInputs[4] = passInput
	m.elasticInputs[5] = serviceFieldInput
	m.elasticInputs[6] = routeFieldInput
	m.elasticInputs[7] = errorFieldInput
	m.elasticInputs[8] = timeFieldInput
	m.elasticInputs[9] = errorRegexInput
	m.elasticInputs[10] = kibanaURLInput
	m.elasticInputs[11] = kibanaIndexInput
	m.elasticIndex = 0
	m.elasticInputs[0].Focus()
	m.showElasticConfig = true
	m.elasticMessage = ""
	sanitizeInputModels(m.elasticInputs)
}

func (m tuiModel) renderElasticEditor() string {
	var sb strings.Builder
	sb.WriteString(titleStyle.Render("Elasticsearch Connection"))
	sb.WriteString("\n")
	sb.WriteString(disclaimerStyle.Render("Configure Elasticsearch/OpenSearch logs backend. Leave fields empty to auto-detect."))
	sb.WriteString("\n\n")

	sb.WriteString(keyStyle.Render("URL"))
	sb.WriteString("\n  " + m.elasticInputs[0].View() + "\n\n")

	sb.WriteString(keyStyle.Render("Index Pattern"))
	sb.WriteString("\n  " + m.elasticInputs[1].View() + "\n\n")

	sb.WriteString(keyStyle.Render("Bearer Token (Optional)"))
	sb.WriteString("\n  " + m.elasticInputs[2].View() + "\n\n")

	sb.WriteString(keyStyle.Render("Basic Auth Username (Optional)"))
	sb.WriteString("\n  " + m.elasticInputs[3].View() + "\n\n")

	sb.WriteString(keyStyle.Render("Basic Auth Password (Optional)"))
	sb.WriteString("\n  " + m.elasticInputs[4].View() + "\n\n")

	sb.WriteString(keyStyle.Render("Service Field"))
	sb.WriteString("\n  " + m.elasticInputs[5].View() + "\n\n")

	sb.WriteString(keyStyle.Render("Route Field (Optional)"))
	sb.WriteString("\n  " + m.elasticInputs[6].View() + "\n\n")

	sb.WriteString(keyStyle.Render("Error/Message Field"))
	sb.WriteString("\n  " + m.elasticInputs[7].View() + "\n\n")

	sb.WriteString(keyStyle.Render("Time Field"))
	sb.WriteString("\n  " + m.elasticInputs[8].View() + "\n\n")

	sb.WriteString(keyStyle.Render("Error Regex (Optional)"))
	sb.WriteString("\n  " + m.elasticInputs[9].View() + "\n\n")

	sb.WriteString(keyStyle.Render("Kibana URL (Optional)"))
	sb.WriteString("\n  " + m.elasticInputs[10].View() + "\n\n")

	sb.WriteString(keyStyle.Render("Kibana Index Pattern (Optional)"))
	sb.WriteString("\n  " + m.elasticInputs[11].View() + "\n\n")

	sb.WriteString(dimStyle.Render("Note: basic auth takes precedence over bearer token."))
	sb.WriteString("\n\n")

	sb.WriteString(keyStyle.Render("Actions"))
	sb.WriteString("\nEnter to save, Esc to return.\n")

	if strings.TrimSpace(m.elasticMessage) != "" {
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render(m.elasticMessage))
		sb.WriteString("\n")
	}
	return sb.String()
}

func (m tuiModel) updateElasticEditor(msg tea.Msg) (tuiModel, tea.Cmd) {
	switch key := msg.(type) {
	case tea.KeyMsg:
		if key.Type == tea.KeyRunes && looksLikeTerminalGarbage(key.String()) {
			sanitizeInputModels(m.elasticInputs)
			m.content = m.renderElasticEditor()
			return m, nil
		}
		switch key.String() {
		case "esc":
			m.showElasticConfig = false
			m.scrollY = 0
			m.content = m.renderContent()
			return m, nil
		case "q":
			m.showElasticConfig = false
			m.scrollY = 0
			m.content = m.renderContent()
			return m, nil
		case "enter":
			if m.elasticIndex == len(m.elasticInputs)-1 {
				m.applyElasticUpdates()
				m.content = m.renderElasticEditor()
				return m, nil
			}
			m.elasticInputs[m.elasticIndex].Blur()
			m.elasticIndex++
			m.elasticInputs[m.elasticIndex].Focus()
			m.content = m.renderElasticEditor()
			return m, nil
		case "tab", "down":
			m.elasticInputs[m.elasticIndex].Blur()
			m.elasticIndex = (m.elasticIndex + 1) % len(m.elasticInputs)
			m.elasticInputs[m.elasticIndex].Focus()
			m.content = m.renderElasticEditor()
			return m, nil
		case "shift+tab", "up":
			m.elasticInputs[m.elasticIndex].Blur()
			m.elasticIndex--
			if m.elasticIndex < 0 {
				m.elasticIndex = len(m.elasticInputs) - 1
			}
			m.elasticInputs[m.elasticIndex].Focus()
			m.content = m.renderElasticEditor()
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.elasticInputs[m.elasticIndex], cmd = m.elasticInputs[m.elasticIndex].Update(msg)
	sanitizeInputModels(m.elasticInputs)
	m.content = m.renderElasticEditor()
	return m, cmd
}

func (m *tuiModel) applyElasticUpdates() {
	url := sanitizeInput(m.elasticInputs[0].Value())
	index := strings.TrimSpace(m.elasticInputs[1].Value())
	token := sanitizeTokenInput(m.elasticInputs[2].Value())
	user := sanitizeTokenInput(m.elasticInputs[3].Value())
	pass := sanitizeTokenInput(m.elasticInputs[4].Value())
	serviceField := strings.TrimSpace(m.elasticInputs[5].Value())
	routeField := strings.TrimSpace(m.elasticInputs[6].Value())
	errorField := strings.TrimSpace(m.elasticInputs[7].Value())
	timeField := strings.TrimSpace(m.elasticInputs[8].Value())
	errorRegex := strings.TrimSpace(m.elasticInputs[9].Value())
	kibanaURL := sanitizeInput(m.elasticInputs[10].Value())
	kibanaIndex := strings.TrimSpace(m.elasticInputs[11].Value())

	if url == "" || !(strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")) {
		m.elasticMessage = "Invalid URL. Please re-enter (must start with http:// or https://)."
		return
	}
	if index == "" && serviceField == "" && errorField == "" && timeField == "" {
		m.elasticMessage = "Saved with auto-detect enabled (fields empty)."
	}

	cfg, err := config.Load()
	if err != nil {
		m.elasticMessage = "Failed to load config."
		return
	}
	cfg.ElasticURL = url
	cfg.ElasticIndex = index
	cfg.ElasticServiceField = serviceField
	cfg.ElasticRouteField = routeField
	cfg.ElasticErrorField = errorField
	cfg.ElasticTimeField = timeField
	cfg.ElasticErrorRegex = errorRegex
	cfg.KibanaURL = kibanaURL
	cfg.KibanaIndex = kibanaIndex
	cfg.ElasticUser = strings.TrimSpace(user)
	cfg.ElasticPass = strings.TrimSpace(pass)
	if cfg.ElasticUser != "" || cfg.ElasticPass != "" {
		cfg.ElasticToken = ""
	}
	if token != "" {
		cfg.ElasticToken = token
	}
	if err := saveCurrentConfig(cfg); err != nil {
		m.elasticMessage = "Failed to save config: " + err.Error()
		return
	}
	if m.elasticMessage == "" {
		m.elasticMessage = "Saved Elasticsearch configuration."
	}
}

func (m tuiModel) renderCorrelationEditor() string {
	var sb strings.Builder
	sb.WriteString(titleStyle.Render("Correlation Settings"))
	sb.WriteString("\n")
	sb.WriteString(disclaimerStyle.Render("Tune correlation thresholds and output limits."))
	sb.WriteString("\n\n")

	sb.WriteString(keyStyle.Render("Min Samples"))
	sb.WriteString("\n  " + m.correlationInputs[0].View() + "\n\n")

	sb.WriteString(keyStyle.Render("Max Logs"))
	sb.WriteString("\n  " + m.correlationInputs[1].View() + "\n\n")

	sb.WriteString(keyStyle.Render("Max Results"))
	sb.WriteString("\n  " + m.correlationInputs[2].View() + "\n\n")

	sb.WriteString(keyStyle.Render("Correlation Window (Optional)"))
	sb.WriteString("\n  " + m.correlationInputs[3].View() + "\n")
	sb.WriteString(dimStyle.Render("  Overrides Loki window when set"))
	sb.WriteString("\n\n")

	sb.WriteString(keyStyle.Render("Best Effort Mode"))
	sb.WriteString("\n  " + m.correlationInputs[4].View() + "\n")
	sb.WriteString(dimStyle.Render("  true/false (show low sample results)"))
	sb.WriteString("\n\n")

	sb.WriteString(keyStyle.Render("Actions"))
	sb.WriteString("\nEnter to save, Esc to return.\n")

	if strings.TrimSpace(m.correlationConfigMsg) != "" {
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render(m.correlationConfigMsg))
		sb.WriteString("\n")
	}
	return sb.String()
}

func (m tuiModel) updateCorrelationEditor(msg tea.Msg) (tuiModel, tea.Cmd) {
	switch key := msg.(type) {
	case tea.KeyMsg:
		if key.Type == tea.KeyRunes && looksLikeTerminalGarbage(key.String()) {
			sanitizeInputModels(m.correlationInputs)
			m.content = m.renderCorrelationEditor()
			return m, nil
		}
		switch key.String() {
		case "esc":
			m.showCorrelationConfig = false
			m.scrollY = 0
			m.content = m.renderContent()
			return m, nil
		case "q":
			m.showCorrelationConfig = false
			m.scrollY = 0
			m.content = m.renderContent()
			return m, nil
		case "enter":
			if m.correlationIndex == len(m.correlationInputs)-1 {
				m.applyCorrelationUpdates()
				m.content = m.renderCorrelationEditor()
				return m, nil
			}
			m.correlationInputs[m.correlationIndex].Blur()
			m.correlationIndex++
			m.correlationInputs[m.correlationIndex].Focus()
			m.content = m.renderCorrelationEditor()
			return m, nil
		case "tab", "down":
			m.correlationInputs[m.correlationIndex].Blur()
			m.correlationIndex = (m.correlationIndex + 1) % len(m.correlationInputs)
			m.correlationInputs[m.correlationIndex].Focus()
			m.content = m.renderCorrelationEditor()
			return m, nil
		case "shift+tab", "up":
			m.correlationInputs[m.correlationIndex].Blur()
			m.correlationIndex--
			if m.correlationIndex < 0 {
				m.correlationIndex = len(m.correlationInputs) - 1
			}
			m.correlationInputs[m.correlationIndex].Focus()
			m.content = m.renderCorrelationEditor()
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.correlationInputs[m.correlationIndex], cmd = m.correlationInputs[m.correlationIndex].Update(msg)
	sanitizeInputModels(m.correlationInputs)
	m.content = m.renderCorrelationEditor()
	return m, cmd
}

func (m *tuiModel) applyCorrelationUpdates() {
	minSamplesValue := strings.TrimSpace(m.correlationInputs[0].Value())
	maxLogsValue := strings.TrimSpace(m.correlationInputs[1].Value())
	maxResultsValue := strings.TrimSpace(m.correlationInputs[2].Value())
	windowValue := strings.TrimSpace(m.correlationInputs[3].Value())
	bestEffortValue := strings.TrimSpace(strings.ToLower(m.correlationInputs[4].Value()))

	cfg, err := config.Load()
	if err != nil {
		m.correlationConfigMsg = "Failed to load config."
		return
	}
	if minSamplesValue != "" {
		minSamples, err := strconv.Atoi(minSamplesValue)
		if err != nil || minSamples <= 0 {
			m.correlationConfigMsg = "Min samples must be a positive integer."
			return
		}
		cfg.CorrelationMinSamples = minSamples
	}
	if maxLogsValue != "" {
		maxLogs, err := strconv.Atoi(maxLogsValue)
		if err != nil || maxLogs <= 0 {
			m.correlationConfigMsg = "Max logs must be a positive integer."
			return
		}
		cfg.CorrelationMaxLogs = maxLogs
	}
	if maxResultsValue != "" {
		maxResults, err := strconv.Atoi(maxResultsValue)
		if err != nil || maxResults <= 0 {
			m.correlationConfigMsg = "Max results must be a positive integer."
			return
		}
		cfg.CorrelationMaxResults = maxResults
	}
	if windowValue != "" {
		if _, err := time.ParseDuration(windowValue); err != nil {
			m.correlationConfigMsg = "Window must be a valid duration (e.g., 10m)."
			return
		}
	}
	cfg.CorrelationWindow = windowValue
	switch bestEffortValue {
	case "1", "true", "yes", "y", "on":
		cfg.CorrelationBestEffort = true
	case "0", "false", "no", "n", "off":
		cfg.CorrelationBestEffort = false
	default:
		m.correlationConfigMsg = "Best effort must be true/false."
		return
	}

	if err := saveCurrentConfig(cfg); err != nil {
		m.correlationConfigMsg = "Failed to save config: " + err.Error()
		return
	}
	m.correlationConfigMsg = "Saved correlation settings."
}

func (m tuiModel) renderPromEditor() string {
	var sb strings.Builder

	sb.WriteString(titleStyle.Render("Prometheus Connection"))
	sb.WriteString("\n")
	sb.WriteString(disclaimerStyle.Render("Configure Prometheus URL and auth. Rate limits are optional."))
	sb.WriteString("\n\n")

	sb.WriteString(keyStyle.Render("Prometheus URL"))
	sb.WriteString("\n  " + m.promInputs[0].View() + "\n\n")

	sb.WriteString(keyStyle.Render("Bearer Token (Optional)"))
	sb.WriteString("\n  " + m.promInputs[1].View() + "\n")
	sb.WriteString(dimStyle.Render("  Stored in " + config.TokenFilePath()))
	sb.WriteString("\n\n")

	sb.WriteString(keyStyle.Render("Basic Auth Username (Optional)"))
	sb.WriteString("\n  " + m.promInputs[2].View() + "\n\n")

	sb.WriteString(keyStyle.Render("Basic Auth Password (Optional)"))
	sb.WriteString("\n  " + m.promInputs[3].View() + "\n")
	sb.WriteString(dimStyle.Render("  Stored in " + config.BasicAuthFilePath()))
	sb.WriteString("\n\n")

	sb.WriteString(keyStyle.Render("Disable Update Checks"))
	sb.WriteString("\n  " + m.promInputs[4].View() + "\n")
	sb.WriteString(dimStyle.Render("  true/false or set HEALTH_MONITOR_DISABLE_UPDATES=1"))
	sb.WriteString("\n\n")

	sb.WriteString(keyStyle.Render("Prometheus QPS"))
	sb.WriteString("\n  " + m.promInputs[5].View() + "\n")
	sb.WriteString(dimStyle.Render("  0 disables rate limiting"))
	sb.WriteString("\n\n")

	sb.WriteString(keyStyle.Render("Route Cardinality Limit"))
	sb.WriteString("\n  " + m.promInputs[6].View() + "\n\n")

	sb.WriteString(keyStyle.Render("Service Cardinality Limit"))
	sb.WriteString("\n  " + m.promInputs[7].View() + "\n\n")

	sb.WriteString(dimStyle.Render("Note: basic auth takes precedence over bearer token."))
	sb.WriteString("\n\n")

	sb.WriteString(keyStyle.Render("Actions"))
	sb.WriteString("\nEnter to save/test, Esc to return.\n")
	sb.WriteString("Ctrl+T clears Prometheus token file • Ctrl+B clears basic auth file\n")

	if strings.TrimSpace(m.promConfigMessage) != "" {
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render(m.promConfigMessage))
		sb.WriteString("\n")
	}
	if strings.TrimSpace(m.promConfigTest) != "" {
		sb.WriteString(dimStyle.Render(m.promConfigTest))
		sb.WriteString("\n")
	}

	return sb.String()
}

func (m tuiModel) updatePromEditor(msg tea.Msg) (tuiModel, tea.Cmd) {
	switch key := msg.(type) {
	case tea.KeyMsg:
		if key.Type == tea.KeyRunes && looksLikeTerminalGarbage(key.String()) {
			sanitizeInputModels(m.promInputs)
			m.content = m.renderPromEditor()
			return m, nil
		}
		switch key.String() {
		case "esc":
			m.showPromConfig = false
			m.scrollY = 0
			m.content = m.renderContent()
			return m, nil
		case "q":
			m.showPromConfig = false
			m.scrollY = 0
			m.content = m.renderContent()
			return m, nil
		case "ctrl+t":
			if err := config.ClearTokenFile(); err != nil {
				m.promConfigMessage = "Failed to clear token file: " + err.Error()
			} else {
				m.promConfigMessage = "Prometheus token cleared."
			}
			m.content = m.renderPromEditor()
			return m, nil
		case "ctrl+b":
			if err := config.ClearBasicAuthFile(); err != nil {
				m.promConfigMessage = "Failed to clear basic auth file: " + err.Error()
			} else {
				m.promConfigMessage = "Prometheus basic auth cleared."
			}
			m.content = m.renderPromEditor()
			return m, nil
		case "enter":
			if m.promIndex == len(m.promInputs)-1 {
				m.applyPromUpdates()
				m.content = m.renderPromEditor()
				return m, nil
			}
			m.promInputs[m.promIndex].Blur()
			m.promIndex++
			m.promInputs[m.promIndex].Focus()
			m.content = m.renderPromEditor()
			return m, nil
		case "tab", "down":
			m.promInputs[m.promIndex].Blur()
			m.promIndex = (m.promIndex + 1) % len(m.promInputs)
			m.promInputs[m.promIndex].Focus()
			m.content = m.renderPromEditor()
			return m, nil
		case "shift+tab", "up":
			m.promInputs[m.promIndex].Blur()
			m.promIndex--
			if m.promIndex < 0 {
				m.promIndex = len(m.promInputs) - 1
			}
			m.promInputs[m.promIndex].Focus()
			m.content = m.renderPromEditor()
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.promInputs[m.promIndex], cmd = m.promInputs[m.promIndex].Update(msg)
	sanitizeInputModels(m.promInputs)
	m.content = m.renderPromEditor()
	return m, cmd
}

func (m *tuiModel) applyPromUpdates() {
	url := sanitizeInput(m.promInputs[0].Value())
	token := sanitizeTokenInput(m.promInputs[1].Value())
	user := sanitizeTokenInput(m.promInputs[2].Value())
	pass := sanitizeTokenInput(m.promInputs[3].Value())
	disableUpdatesValue := strings.TrimSpace(strings.ToLower(m.promInputs[4].Value()))
	promQPSValue := strings.TrimSpace(m.promInputs[5].Value())
	routeLimitValue := strings.TrimSpace(m.promInputs[6].Value())
	serviceLimitValue := strings.TrimSpace(m.promInputs[7].Value())
	disableUpdates := false
	if disableUpdatesValue != "" {
		switch disableUpdatesValue {
		case "1", "true", "yes", "y", "on":
			disableUpdates = true
		case "0", "false", "no", "n", "off":
			disableUpdates = false
		default:
			m.promConfigMessage = "Disable updates must be true/false."
			return
		}
	}

	cfg, err := config.Load()
	if err != nil {
		m.promConfigMessage = "Failed to load config."
		return
	}
	if url == "" || !(strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")) {
		m.promConfigMessage = "Invalid URL. Please re-enter (must start with http:// or https://)."
		return
	}
	cfg.PrometheusURL = url
	cfg.PrometheusUser = strings.TrimSpace(user)
	cfg.PrometheusPass = strings.TrimSpace(pass)
	if cfg.PrometheusUser != "" || cfg.PrometheusPass != "" {
		cfg.PrometheusToken = ""
	}
	cfg.DisableUpdates = disableUpdates
	cfg.AutoDiscover = true
	if promQPSValue != "" {
		qps, err := strconv.ParseFloat(promQPSValue, 64)
		if err != nil || qps < 0 {
			m.promConfigMessage = "Prometheus QPS must be a non-negative number."
			return
		}
		cfg.PrometheusQPS = qps
	}
	if routeLimitValue != "" {
		limit, err := strconv.Atoi(routeLimitValue)
		if err != nil || limit < 0 {
			m.promConfigMessage = "Route cardinality limit must be a non-negative integer."
			return
		}
		cfg.RouteCardinalityLimit = limit
	}
	if serviceLimitValue != "" {
		limit, err := strconv.Atoi(serviceLimitValue)
		if err != nil || limit < 0 {
			m.promConfigMessage = "Service cardinality limit must be a non-negative integer."
			return
		}
		cfg.ServiceCardinalityLimit = limit
	}
	if err := saveCurrentConfig(cfg); err != nil {
		m.promConfigMessage = "Failed to save config: " + err.Error()
		return
	}
	if token != "" {
		if err := config.WriteTokenFile(token); err != nil {
			m.promConfigMessage = "Failed to write token: " + err.Error()
			return
		}
		cfg.PrometheusToken = token
	}
	if user != "" || pass != "" {
		if err := config.WriteBasicAuthFile(user, pass); err != nil {
			m.promConfigMessage = "Failed to write basic auth: " + err.Error()
			return
		}
	}

	client := prometheus.Client{
		BaseURL: cfg.PrometheusURL,
		Token:   cfg.PrometheusToken,
		User:    strings.TrimSpace(user),
		Pass:    strings.TrimSpace(pass),
		Timeout: 5 * time.Second,
		QPS:     cfg.PrometheusQPS,
	}
	msg, err := client.Check()
	if err != nil {
		m.promConfigTest = "Prometheus check failed: " + err.Error()
	} else {
		m.promConfigTest = "Prometheus OK: " + msg
	}

	result, err := api_latency.Collect(&client, cfg)
	if err == nil {
		status := model.SAFE
		if cfg.LatencyThresholdSeconds <= 0 {
			cfg.LatencyThresholdSeconds = 10
		}
		if result.P95 > cfg.LatencyThresholdSeconds {
			status = model.RISK
		} else if result.P90 > cfg.LatencyThresholdSeconds {
			status = model.CHECK
		}
		updated := &model.APILatency{
			Service:             cfg.APIService,
			Route:               cfg.APIRoute,
			Window:              result.Window,
			P90:                 result.P90,
			P95:                 result.P95,
			P99:                 result.P99,
			ErrorRate:           result.ErrorRate,
			ClientErrorRate:     result.ClientErrorRate,
			ServerErrorRate:     result.ServerErrorRate,
			ErrorLabel:          result.ErrorLabel,
			RPS:                 result.RPS,
			Status:              status,
			Note:                result.Note,
			Confidence:          result.Confidence,
			TopEndpointsLimit:   cfg.TopEndpoints,
			TopEndpoints:        mapLatencyEndpoints(result.TopEndpoints),
			TopEndpointsRPS:     mapRateEndpoints(result.TopEndpointsRPS),
			TopEndpointsRPSNote: result.TopEndpointsRPSNote,
			TopServicesRPS:      mapServiceRates(result.TopServicesRPS),
			TopServicesRPSNote:  result.TopServicesRPSNote,
		}
		m.applyFilteredLatency(updated)
		m.report.APINote = ""
		if m.report.APIConfig != nil {
			m.report.APIConfig.PrometheusURL = cfg.PrometheusURL
		}
	}

	m.updatePromLink()
	m.promConfigMessage = "Saved Prometheus configuration."
}

func buildTraceInputs(cfg config.Config) []textinput.Model {
	inputs := make([]textinput.Model, 10)

	urlInput := textinput.New()
	urlInput.Prompt = ""
	urlInput.Placeholder = "https://tempo.example.com"
	urlInput.CharLimit = 256
	urlInput.Width = 60
	urlInput.SetValue(sanitizeInput(cfg.TraceURL))
	urlInput.Blur()

	tokenInput := textinput.New()
	tokenInput.Prompt = ""
	tokenInput.Placeholder = "Trace token (stored in " + config.TraceTokenFilePath() + ")"
	tokenInput.CharLimit = 256
	tokenInput.Width = 60
	tokenInput.EchoMode = textinput.EchoPassword
	tokenInput.EchoCharacter = '*'
	tokenInput.SetValue("")
	tokenInput.SetCursor(0)
	tokenInput.Blur()

	userInput := textinput.New()
	userInput.Prompt = ""
	userInput.Placeholder = "Trace basic auth username (optional)"
	userInput.CharLimit = 128
	userInput.Width = 60
	userInput.SetValue("")
	userInput.Blur()

	passInput := textinput.New()
	passInput.Prompt = ""
	passInput.Placeholder = "Trace basic auth password (optional)"
	passInput.CharLimit = 128
	passInput.Width = 60
	passInput.EchoMode = textinput.EchoPassword
	passInput.EchoCharacter = '*'
	passInput.SetValue("")
	passInput.SetCursor(0)
	passInput.Blur()

	windowInput := textinput.New()
	windowInput.Prompt = ""
	windowInput.Placeholder = "Trace window (e.g., 10m)"
	windowInput.CharLimit = 16
	windowInput.Width = 20
	windowInput.SetValue(cfg.TraceWindow)
	windowInput.Blur()

	dsInput := textinput.New()
	dsInput.Prompt = ""
	dsInput.Placeholder = "Grafana trace datasource name (e.g., Tempo)"
	dsInput.CharLimit = 128
	dsInput.Width = 60
	dsInput.SetValue(cfg.GrafanaTraceDataSource)
	dsInput.Blur()

	mapInput := textinput.New()
	mapInput.Prompt = ""
	mapInput.Placeholder = "Trace service map (metrics=trace,...)"
	mapInput.CharLimit = 256
	mapInput.Width = 60
	mapInput.SetValue(cfg.TraceServiceMap)
	mapInput.Blur()

	defaultInput := textinput.New()
	defaultInput.Prompt = ""
	defaultInput.Placeholder = "Trace default service (optional)"
	defaultInput.CharLimit = 128
	defaultInput.Width = 60
	defaultInput.SetValue(cfg.TraceDefaultService)
	defaultInput.Blur()

	minDurationInput := textinput.New()
	minDurationInput.Prompt = ""
	minDurationInput.Placeholder = "Min trace duration ms (e.g., 10)"
	minDurationInput.CharLimit = 8
	minDurationInput.Width = 20
	minDurationInput.SetValue(strconv.Itoa(cfg.TraceMinDurationMs))
	minDurationInput.Blur()

	excludeInput := textinput.New()
	excludeInput.Prompt = ""
	excludeInput.Placeholder = "Exclude system routes (true/false)"
	excludeInput.CharLimit = 5
	excludeInput.Width = 10
	if cfg.TraceExcludeSystemRoutes {
		excludeInput.SetValue("true")
	} else {
		excludeInput.SetValue("false")
	}
	excludeInput.Blur()

	inputs[0] = urlInput
	inputs[1] = tokenInput
	inputs[2] = userInput
	inputs[3] = passInput
	inputs[4] = windowInput
	inputs[5] = dsInput
	inputs[6] = mapInput
	inputs[7] = defaultInput
	inputs[8] = minDurationInput
	inputs[9] = excludeInput

	sanitizeInputModels(inputs)
	return inputs
}

func (m *tuiModel) startTempoEditor() {
	cfg, _ := config.Load()
	m.tempoInputs = buildTraceInputs(cfg)
	m.tempoIndex = 0
	m.tempoInputs[0].Focus()
	m.showTempoConfig = true
	m.tempoMessage = ""
}

func (m *tuiModel) startJaegerEditor() {
	cfg, _ := config.Load()
	m.jaegerInputs = buildTraceInputs(cfg)
	m.jaegerIndex = 0
	m.jaegerInputs[0].Focus()
	m.showJaegerConfig = true
	m.jaegerMessage = ""
}

func (m tuiModel) renderTempoEditor() string {
	var sb strings.Builder
	sb.WriteString(titleStyle.Render("Tempo Connection"))
	sb.WriteString("\n")
	sb.WriteString(disclaimerStyle.Render("Configure Tempo trace backend and auth settings."))
	sb.WriteString("\n\n")
	if strings.TrimSpace(m.tempoMessage) != "" {
		sb.WriteString(dimStyle.Render("Status: " + m.tempoMessage))
		sb.WriteString("\n\n")
	}
	sb.WriteString(renderTraceEditorInputs(m.tempoInputs))
	return sb.String()
}

func (m tuiModel) renderJaegerEditor() string {
	var sb strings.Builder
	sb.WriteString(titleStyle.Render("Jaeger Connection"))
	sb.WriteString("\n")
	sb.WriteString(disclaimerStyle.Render("Configure Jaeger trace backend and auth settings."))
	sb.WriteString("\n\n")
	if strings.TrimSpace(m.jaegerMessage) != "" {
		sb.WriteString(dimStyle.Render("Status: " + m.jaegerMessage))
		sb.WriteString("\n\n")
	}
	sb.WriteString(renderTraceEditorInputs(m.jaegerInputs))
	return sb.String()
}

func renderTraceEditorInputs(inputs []textinput.Model) string {
	var sb strings.Builder
	sb.WriteString(keyStyle.Render("Trace URL"))
	sb.WriteString("\n  " + inputs[0].View() + "\n\n")

	sb.WriteString(keyStyle.Render("Trace Token (Optional)"))
	sb.WriteString("\n  " + inputs[1].View() + "\n")
	sb.WriteString(dimStyle.Render("  Stored in " + config.TraceTokenFilePath()))
	sb.WriteString("\n\n")

	sb.WriteString(keyStyle.Render("Trace Basic Auth Username (Optional)"))
	sb.WriteString("\n  " + inputs[2].View() + "\n\n")

	sb.WriteString(keyStyle.Render("Trace Basic Auth Password (Optional)"))
	sb.WriteString("\n  " + inputs[3].View() + "\n")
	sb.WriteString(dimStyle.Render("  Stored in " + config.TraceBasicAuthFilePath()))
	sb.WriteString("\n\n")

	sb.WriteString(keyStyle.Render("Trace Window"))
	sb.WriteString("\n  " + inputs[4].View() + "\n")
	sb.WriteString(dimStyle.Render("  e.g., 4m, 10m"))
	sb.WriteString("\n\n")

	sb.WriteString(keyStyle.Render("Grafana Trace Datasource"))
	sb.WriteString("\n  " + inputs[5].View() + "\n")
	sb.WriteString(dimStyle.Render("  Needed for Grafana trace links"))
	sb.WriteString("\n\n")

	sb.WriteString(keyStyle.Render("Trace Service Map"))
	sb.WriteString("\n  " + inputs[6].View() + "\n")
	sb.WriteString(dimStyle.Render("  metrics=trace (e.g., order_API=order-api)"))
	sb.WriteString("\n\n")

	sb.WriteString(keyStyle.Render("Trace Default Service"))
	sb.WriteString("\n  " + inputs[7].View() + "\n\n")

	sb.WriteString(keyStyle.Render("Min Trace Duration (ms)"))
	sb.WriteString("\n  " + inputs[8].View() + "\n\n")

	sb.WriteString(keyStyle.Render("Exclude System Routes"))
	sb.WriteString("\n  " + inputs[9].View() + "\n")
	sb.WriteString(dimStyle.Render("  true/false"))
	sb.WriteString("\n\n")

	sb.WriteString(dimStyle.Render("Note: basic auth takes precedence over bearer token."))
	sb.WriteString("\n\n")

	sb.WriteString(keyStyle.Render("Actions"))
	sb.WriteString("\nEnter on last field to save, Esc to return.\n")
	sb.WriteString("Ctrl+T clears trace token file • Ctrl+B clears trace basic auth file\n")

	return sb.String()
}

func (m tuiModel) updateTempoEditor(msg tea.Msg) (tuiModel, tea.Cmd) {
	switch key := msg.(type) {
	case tea.KeyMsg:
		if key.Type == tea.KeyRunes && looksLikeTerminalGarbage(key.String()) {
			sanitizeInputModels(m.tempoInputs)
			m.content = m.renderTempoEditor()
			return m, nil
		}
		switch key.String() {
		case "esc":
			m.showTempoConfig = false
			m.scrollY = 0
			m.content = m.renderContent()
			return m, nil
		case "q":
			m.showTempoConfig = false
			m.scrollY = 0
			m.content = m.renderContent()
			return m, nil
		case "ctrl+t":
			if err := config.ClearTraceTokenFile(); err != nil {
				m.tempoMessage = "Failed to clear trace token file: " + err.Error()
			} else {
				m.tempoMessage = "Trace token cleared."
			}
			m.content = m.renderTempoEditor()
			return m, nil
		case "ctrl+b":
			if err := config.ClearTraceBasicAuthFile(); err != nil {
				m.tempoMessage = "Failed to clear trace basic auth file: " + err.Error()
			} else {
				m.tempoMessage = "Trace basic auth cleared."
			}
			m.content = m.renderTempoEditor()
			return m, nil
		case "enter":
			if m.tempoIndex == len(m.tempoInputs)-1 {
				m.tempoMessage = m.applyTraceUpdates("tempo", m.tempoInputs)
				m.content = m.renderTempoEditor()
				return m, nil
			}
			m.tempoInputs[m.tempoIndex].Blur()
			m.tempoIndex++
			m.tempoInputs[m.tempoIndex].Focus()
			m.content = m.renderTempoEditor()
			return m, nil
		case "tab", "down":
			m.tempoInputs[m.tempoIndex].Blur()
			m.tempoIndex = (m.tempoIndex + 1) % len(m.tempoInputs)
			m.tempoInputs[m.tempoIndex].Focus()
			m.content = m.renderTempoEditor()
			return m, nil
		case "shift+tab", "up":
			m.tempoInputs[m.tempoIndex].Blur()
			m.tempoIndex--
			if m.tempoIndex < 0 {
				m.tempoIndex = len(m.tempoInputs) - 1
			}
			m.tempoInputs[m.tempoIndex].Focus()
			m.content = m.renderTempoEditor()
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.tempoInputs[m.tempoIndex], cmd = m.tempoInputs[m.tempoIndex].Update(msg)
	sanitizeInputModels(m.tempoInputs)
	m.content = m.renderTempoEditor()
	return m, cmd
}

func (m tuiModel) updateJaegerEditor(msg tea.Msg) (tuiModel, tea.Cmd) {
	switch key := msg.(type) {
	case tea.KeyMsg:
		if key.Type == tea.KeyRunes && looksLikeTerminalGarbage(key.String()) {
			sanitizeInputModels(m.jaegerInputs)
			m.content = m.renderJaegerEditor()
			return m, nil
		}
		switch key.String() {
		case "esc":
			m.showJaegerConfig = false
			m.scrollY = 0
			m.content = m.renderContent()
			return m, nil
		case "q":
			m.showJaegerConfig = false
			m.scrollY = 0
			m.content = m.renderContent()
			return m, nil
		case "ctrl+t":
			if err := config.ClearTraceTokenFile(); err != nil {
				m.jaegerMessage = "Failed to clear trace token file: " + err.Error()
			} else {
				m.jaegerMessage = "Trace token cleared."
			}
			m.content = m.renderJaegerEditor()
			return m, nil
		case "ctrl+b":
			if err := config.ClearTraceBasicAuthFile(); err != nil {
				m.jaegerMessage = "Failed to clear trace basic auth file: " + err.Error()
			} else {
				m.jaegerMessage = "Trace basic auth cleared."
			}
			m.content = m.renderJaegerEditor()
			return m, nil
		case "enter":
			if m.jaegerIndex == len(m.jaegerInputs)-1 {
				m.jaegerMessage = m.applyTraceUpdates("jaeger", m.jaegerInputs)
				m.content = m.renderJaegerEditor()
				return m, nil
			}
			m.jaegerInputs[m.jaegerIndex].Blur()
			m.jaegerIndex++
			m.jaegerInputs[m.jaegerIndex].Focus()
			m.content = m.renderJaegerEditor()
			return m, nil
		case "tab", "down":
			m.jaegerInputs[m.jaegerIndex].Blur()
			m.jaegerIndex = (m.jaegerIndex + 1) % len(m.jaegerInputs)
			m.jaegerInputs[m.jaegerIndex].Focus()
			m.content = m.renderJaegerEditor()
			return m, nil
		case "shift+tab", "up":
			m.jaegerInputs[m.jaegerIndex].Blur()
			m.jaegerIndex--
			if m.jaegerIndex < 0 {
				m.jaegerIndex = len(m.jaegerInputs) - 1
			}
			m.jaegerInputs[m.jaegerIndex].Focus()
			m.content = m.renderJaegerEditor()
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.jaegerInputs[m.jaegerIndex], cmd = m.jaegerInputs[m.jaegerIndex].Update(msg)
	sanitizeInputModels(m.jaegerInputs)
	m.content = m.renderJaegerEditor()
	return m, cmd
}

func (m *tuiModel) applyTraceUpdates(backend string, inputs []textinput.Model) string {
	traceURL := sanitizeInput(inputs[0].Value())
	traceToken := sanitizeTokenInput(inputs[1].Value())
	traceUser := sanitizeTokenInput(inputs[2].Value())
	tracePass := sanitizeTokenInput(inputs[3].Value())
	traceWindow := strings.TrimSpace(inputs[4].Value())
	traceGrafanaDS := strings.TrimSpace(inputs[5].Value())
	traceServiceMap := strings.TrimSpace(inputs[6].Value())
	traceDefaultService := strings.TrimSpace(inputs[7].Value())
	minDurationValue := strings.TrimSpace(inputs[8].Value())
	excludeValue := strings.TrimSpace(strings.ToLower(inputs[9].Value()))

	cfg, err := config.Load()
	if err != nil {
		return "Failed to load config."
	}
	if traceURL != "" && !(strings.HasPrefix(traceURL, "http://") || strings.HasPrefix(traceURL, "https://")) {
		return "Trace URL must start with http:// or https://"
	}
	if minDurationValue != "" {
		minDuration, err := strconv.Atoi(minDurationValue)
		if err != nil || minDuration < 0 {
			return "Min duration must be a non-negative integer."
		}
		cfg.TraceMinDurationMs = minDuration
	}
	if excludeValue != "" {
		switch excludeValue {
		case "1", "true", "yes", "y", "on":
			cfg.TraceExcludeSystemRoutes = true
		case "0", "false", "no", "n", "off":
			cfg.TraceExcludeSystemRoutes = false
		default:
			return "Exclude system routes must be true/false."
		}
	}

	cfg.TraceBackend = backend
	cfg.TraceURL = traceURL
	cfg.TraceWindow = traceWindow
	cfg.TraceServiceMap = traceServiceMap
	cfg.TraceDefaultService = traceDefaultService
	cfg.GrafanaTraceDataSource = traceGrafanaDS
	if err := saveCurrentConfig(cfg); err != nil {
		return "Failed to save config: " + err.Error()
	}
	if traceToken != "" {
		if err := config.WriteTraceTokenFile(traceToken); err != nil {
			return "Failed to write trace token: " + err.Error()
		}
		cfg.TraceToken = traceToken
	}
	if traceUser != "" || tracePass != "" {
		if err := config.WriteTraceBasicAuthFile(traceUser, tracePass); err != nil {
			return "Failed to write trace basic auth: " + err.Error()
		}
	}
	return "Saved trace configuration."
}

func (m *tuiModel) startLokiEditor() {
	cfg, _ := config.Load()
	m.lokiInputs = make([]textinput.Model, 9)

	urlInput := textinput.New()
	urlInput.Prompt = ""
	urlInput.Placeholder = "https://loki.example.com"
	urlInput.CharLimit = 256
	urlInput.Width = 60
	cleanURL := sanitizeInput(cfg.LokiURL)
	urlInput.SetValue(cleanURL)

	tokenInput := textinput.New()
	tokenInput.Prompt = ""
	tokenInput.Placeholder = "Loki token (stored in " + config.LokiTokenFilePath() + ")"
	tokenInput.CharLimit = 256
	tokenInput.Width = 60
	tokenInput.EchoMode = textinput.EchoPassword
	tokenInput.EchoCharacter = '*'
	tokenInput.SetValue("")
	tokenInput.SetCursor(0)
	tokenInput.Blur()

	userInput := textinput.New()
	userInput.Prompt = ""
	userInput.Placeholder = "Basic auth username (optional)"
	userInput.CharLimit = 128
	userInput.Width = 60
	userInput.SetValue(cfg.LokiUser)

	passInput := textinput.New()
	passInput.Prompt = ""
	passInput.Placeholder = "Basic auth password (optional)"
	passInput.CharLimit = 128
	passInput.Width = 60
	passInput.EchoMode = textinput.EchoPassword
	passInput.EchoCharacter = '*'
	passInput.SetValue("")
	passInput.SetCursor(0)
	passInput.Blur()

	serviceLabelInput := textinput.New()
	serviceLabelInput.Prompt = ""
	serviceLabelInput.Placeholder = "Service label (auto-detect if empty)"
	serviceLabelInput.CharLimit = 64
	serviceLabelInput.Width = 60
	serviceLabelInput.SetValue(sanitizeInputLoose(cfg.LokiServiceLabel))

	routeLabelInput := textinput.New()
	routeLabelInput.Prompt = ""
	routeLabelInput.Placeholder = "Route label (auto-detect if empty)"
	routeLabelInput.CharLimit = 64
	routeLabelInput.Width = 60
	routeLabelInput.SetValue(sanitizeInputLoose(cfg.LokiRouteLabel))

	errorRegexInput := textinput.New()
	errorRegexInput.Prompt = ""
	errorRegexInput.Placeholder = "Error regex (optional)"
	errorRegexInput.CharLimit = 128
	errorRegexInput.Width = 60
	errorRegexInput.SetValue(sanitizeInputLoose(cfg.LokiErrorRegex))

	windowInput := textinput.New()
	windowInput.Prompt = ""
	windowInput.Placeholder = "Window (e.g., 10m)"
	windowInput.CharLimit = 16
	windowInput.Width = 20
	windowInput.SetValue(sanitizeInputLoose(cfg.LokiWindow))

	qpsInput := textinput.New()
	qpsInput.Prompt = ""
	qpsInput.Placeholder = "Loki QPS (0=unlimited)"
	qpsInput.CharLimit = 16
	qpsInput.Width = 20
	qpsInput.SetValue(strconv.FormatFloat(cfg.LokiQPS, 'f', -1, 64))
	qpsInput.Blur()

	m.lokiInputs[0] = urlInput
	m.lokiInputs[1] = tokenInput
	m.lokiInputs[2] = userInput
	m.lokiInputs[3] = passInput
	m.lokiInputs[4] = serviceLabelInput
	m.lokiInputs[5] = routeLabelInput
	m.lokiInputs[6] = errorRegexInput
	m.lokiInputs[7] = windowInput
	m.lokiInputs[8] = qpsInput
	m.lokiIndex = 0
	m.lokiInputs[0].Focus()
	m.showLokiConfig = true
	m.lokiMessage = ""
	m.lokiTestResult = ""
}

func (m tuiModel) renderLokiEditor() string {
	var sb strings.Builder
	sb.WriteString(titleStyle.Render("Loki Connection"))
	sb.WriteString("\n")
	sb.WriteString(disclaimerStyle.Render("Configure Loki URL and auth. Labels auto-detect if empty."))
	sb.WriteString("\n\n")

	sb.WriteString(keyStyle.Render("URL"))
	sb.WriteString("\n  " + m.lokiInputs[0].View() + "\n\n")

	sb.WriteString(keyStyle.Render("Bearer Token (Optional)"))
	sb.WriteString("\n  " + m.lokiInputs[1].View() + "\n")
	sb.WriteString(dimStyle.Render("  Stored in " + config.LokiTokenFilePath()))
	sb.WriteString("\n\n")

	sb.WriteString(keyStyle.Render("Basic Auth Username (Optional)"))
	sb.WriteString("\n  " + m.lokiInputs[2].View() + "\n\n")

	sb.WriteString(keyStyle.Render("Basic Auth Password (Optional)"))
	sb.WriteString("\n  " + m.lokiInputs[3].View() + "\n")
	sb.WriteString(dimStyle.Render("  Stored in " + config.LokiBasicAuthFilePath()))
	sb.WriteString("\n\n")

	sb.WriteString(keyStyle.Render("Service Label (Optional)"))
	sb.WriteString("\n  " + m.lokiInputs[4].View() + "\n\n")

	sb.WriteString(keyStyle.Render("Route Label (Optional)"))
	sb.WriteString("\n  " + m.lokiInputs[5].View() + "\n\n")

	sb.WriteString(keyStyle.Render("Error Regex (Optional)"))
	sb.WriteString("\n  " + m.lokiInputs[6].View() + "\n\n")

	sb.WriteString(keyStyle.Render("Window"))
	sb.WriteString("\n  " + m.lokiInputs[7].View() + "\n\n")

	sb.WriteString(keyStyle.Render("Loki QPS"))
	sb.WriteString("\n  " + m.lokiInputs[8].View() + "\n")
	sb.WriteString(dimStyle.Render("  0 disables rate limiting"))
	sb.WriteString("\n\n")

	sb.WriteString(dimStyle.Render("Note: basic auth takes precedence over bearer token."))
	sb.WriteString("\n\n")

	sb.WriteString(keyStyle.Render("Actions"))
	sb.WriteString("\nEnter to save/test, Esc to return.\n")
	sb.WriteString("Ctrl+T clears Loki token file • Ctrl+B clears Loki basic auth file\n")

	if strings.TrimSpace(m.lokiMessage) != "" {
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render(m.lokiMessage))
		sb.WriteString("\n")
	}
	if strings.TrimSpace(m.lokiTestResult) != "" {
		sb.WriteString(dimStyle.Render(m.lokiTestResult))
		sb.WriteString("\n")
	}
	return sb.String()
}

func (m tuiModel) updateLokiEditor(msg tea.Msg) (tuiModel, tea.Cmd) {
	switch key := msg.(type) {
	case tea.KeyMsg:
		switch key.String() {
		case "esc":
			m.showLokiConfig = false
			m.scrollY = 0
			m.content = m.renderContent()
			return m, nil
		case "q":
			m.showLokiConfig = false
			m.scrollY = 0
			m.content = m.renderContent()
			return m, nil
		case "ctrl+t":
			if err := config.ClearLokiTokenFile(); err != nil {
				m.lokiMessage = "Failed to clear Loki token file: " + err.Error()
			} else {
				m.lokiMessage = "Loki token cleared."
			}
			m.content = m.renderLokiEditor()
			return m, nil
		case "ctrl+b":
			if err := config.ClearLokiBasicAuthFile(); err != nil {
				m.lokiMessage = "Failed to clear Loki basic auth file: " + err.Error()
			} else {
				m.lokiMessage = "Loki basic auth cleared."
			}
			m.content = m.renderLokiEditor()
			return m, nil
		case "enter":
			if m.lokiIndex == len(m.lokiInputs)-1 {
				m.applyLokiUpdates()
				m.content = m.renderLokiEditor()
				return m, nil
			}
			m.lokiInputs[m.lokiIndex].Blur()
			m.lokiIndex++
			m.lokiInputs[m.lokiIndex].Focus()
			m.content = m.renderLokiEditor()
			return m, nil
		case "tab", "down":
			m.lokiInputs[m.lokiIndex].Blur()
			m.lokiIndex = (m.lokiIndex + 1) % len(m.lokiInputs)
			m.lokiInputs[m.lokiIndex].Focus()
			m.content = m.renderLokiEditor()
			return m, nil
		case "shift+tab", "up":
			m.lokiInputs[m.lokiIndex].Blur()
			m.lokiIndex--
			if m.lokiIndex < 0 {
				m.lokiIndex = len(m.lokiInputs) - 1
			}
			m.lokiInputs[m.lokiIndex].Focus()
			m.content = m.renderLokiEditor()
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.lokiInputs[m.lokiIndex], cmd = m.lokiInputs[m.lokiIndex].Update(msg)
	m.content = m.renderLokiEditor()
	return m, cmd
}

func (m *tuiModel) applyLokiUpdates() {
	url := sanitizeInput(m.lokiInputs[0].Value())
	token := sanitizeTokenInput(m.lokiInputs[1].Value())
	user := sanitizeTokenInput(m.lokiInputs[2].Value())
	pass := sanitizeTokenInput(m.lokiInputs[3].Value())
	serviceLabel := strings.TrimSpace(sanitizeInputLoose(m.lokiInputs[4].Value()))
	routeLabel := strings.TrimSpace(sanitizeInputLoose(m.lokiInputs[5].Value()))
	errorRegex := strings.TrimSpace(sanitizeInputLoose(m.lokiInputs[6].Value()))
	window := strings.TrimSpace(sanitizeInputLoose(m.lokiInputs[7].Value()))
	qpsValue := strings.TrimSpace(m.lokiInputs[8].Value())

	if url == "" {
		m.lokiMessage = "Loki URL is required."
		return
	}

	cfg, err := config.Load()
	if err != nil {
		m.lokiMessage = "Failed to load config: " + err.Error()
		return
	}
	cfg.LokiURL = url
	cfg.LokiUser = strings.TrimSpace(user)
	cfg.LokiPass = strings.TrimSpace(pass)
	cfg.LokiServiceLabel = serviceLabel
	cfg.LokiRouteLabel = routeLabel
	if errorRegex == "" {
		cfg.LokiErrorRegex = config.Default().LokiErrorRegex
	} else {
		if normalized, ok := output.SanitizeLokiRegex(errorRegex); ok {
			m.lokiMessage = joinNotesInline(m.lokiMessage, []string{"regex normalized (\\d -> [0-9])"})
			errorRegex = normalized
		}
		validated, reason := output.ValidateLokiRegex(errorRegex)
		if reason != "" {
			m.lokiMessage = joinNotesInline(m.lokiMessage, []string{reason})
			return
		}
		cfg.LokiErrorRegex = validated
	}
	if window == "" {
		cfg.LokiWindow = config.Default().LokiWindow
	} else {
		if _, err := time.ParseDuration(window); err != nil {
			m.lokiMessage = "Invalid window. Use values like 10m, 1h."
			return
		}
		cfg.LokiWindow = window
	}
	if qpsValue != "" {
		qps, err := strconv.ParseFloat(qpsValue, 64)
		if err != nil || qps < 0 {
			m.lokiMessage = "Loki QPS must be a non-negative number."
			return
		}
		cfg.LokiQPS = qps
	}
	if cfg.LokiUser != "" || cfg.LokiPass != "" {
		cfg.LokiToken = ""
	}
	if err := saveCurrentConfig(cfg); err != nil {
		m.lokiMessage = "Failed to save config: " + err.Error()
		return
	}
	if token != "" {
		if err := config.WriteLokiTokenFile(token); err != nil {
			m.lokiMessage = "Failed to write Loki token: " + err.Error()
			return
		}
		cfg.LokiToken = token
	}
	if user != "" || pass != "" {
		if err := config.WriteLokiBasicAuthFile(user, pass); err != nil {
			m.lokiMessage = "Failed to write Loki basic auth: " + err.Error()
			return
		}
	}

	client := loki.Client{
		BaseURL: cfg.LokiURL,
		Token:   cfg.LokiToken,
		User:    cfg.LokiUser,
		Pass:    cfg.LokiPass,
		Timeout: 5 * time.Second,
		QPS:     cfg.LokiQPS,
	}
	msg, err := client.Check()
	if err != nil {
		m.lokiTestResult = "Loki check failed: " + err.Error()
	} else {
		m.lokiTestResult = "Loki OK: " + msg
	}
	m.lokiMessage = "Saved Loki configuration."
}

func (m tuiModel) renderAuthEditor() string {
	var sb strings.Builder

	sb.WriteString(titleStyle.Render("Connections"))
	sb.WriteString("\n")
	sb.WriteString(disclaimerStyle.Render("Update URL and credentials."))
	sb.WriteString("\n\n")
	if strings.TrimSpace(m.authMessage) != "" {
		sb.WriteString(dimStyle.Render("Status: " + m.authMessage))
		sb.WriteString("\n\n")
	}
	if strings.TrimSpace(m.authTestResult) != "" {
		sb.WriteString(dimStyle.Render("Test: " + m.authTestResult))
		sb.WriteString("\n\n")
	}

	sb.WriteString(keyStyle.Render("URL"))
	sb.WriteString("\n  " + m.authInputs[0].View() + "\n\n")

	sb.WriteString(keyStyle.Render("Bearer Token (Optional)"))
	sb.WriteString("\n  " + m.authInputs[1].View() + "\n")
	sb.WriteString(dimStyle.Render("  Stored in " + config.TokenFilePath()))
	sb.WriteString("\n\n")

	sb.WriteString(keyStyle.Render("Basic Auth Username (Optional)"))
	sb.WriteString("\n  " + m.authInputs[2].View() + "\n\n")

	sb.WriteString(keyStyle.Render("Basic Auth Password (Optional)"))
	sb.WriteString("\n  " + m.authInputs[3].View() + "\n")
	sb.WriteString(dimStyle.Render("  Stored in " + config.BasicAuthFilePath()))
	sb.WriteString("\n\n")

	sb.WriteString(keyStyle.Render("Disable Update Checks"))
	sb.WriteString("\n  " + m.authInputs[4].View() + "\n")
	sb.WriteString(dimStyle.Render("  true/false or set HEALTH_MONITOR_DISABLE_UPDATES=1"))
	sb.WriteString("\n\n")

	sb.WriteString(keyStyle.Render("Prometheus QPS"))
	sb.WriteString("\n  " + m.authInputs[5].View() + "\n")
	sb.WriteString(dimStyle.Render("  0 disables rate limiting"))
	sb.WriteString("\n\n")

	sb.WriteString(keyStyle.Render("Route Cardinality Limit"))
	sb.WriteString("\n  " + m.authInputs[6].View() + "\n\n")

	sb.WriteString(keyStyle.Render("Service Cardinality Limit"))
	sb.WriteString("\n  " + m.authInputs[7].View() + "\n\n")

	sb.WriteString(markdownBlock("## Tracing Connection"))
	sb.WriteString(keyStyle.Render("Trace Backend (tempo/jaeger)"))
	sb.WriteString("\n  " + m.authInputs[8].View() + "\n\n")

	sb.WriteString(keyStyle.Render("Trace URL"))
	sb.WriteString("\n  " + m.authInputs[9].View() + "\n\n")

	sb.WriteString(keyStyle.Render("Trace Token (Optional)"))
	sb.WriteString("\n  " + m.authInputs[10].View() + "\n")
	sb.WriteString(dimStyle.Render("  Stored in " + config.TraceTokenFilePath()))
	sb.WriteString("\n\n")

	sb.WriteString(keyStyle.Render("Trace Basic Auth Username (Optional)"))
	sb.WriteString("\n  " + m.authInputs[11].View() + "\n\n")

	sb.WriteString(keyStyle.Render("Trace Basic Auth Password (Optional)"))
	sb.WriteString("\n  " + m.authInputs[12].View() + "\n")
	sb.WriteString(dimStyle.Render("  Stored in " + config.TraceBasicAuthFilePath()))
	sb.WriteString("\n\n")

	sb.WriteString(keyStyle.Render("Trace Window"))
	sb.WriteString("\n  " + m.authInputs[13].View() + "\n")
	sb.WriteString(dimStyle.Render("  e.g., 4m, 10m"))
	sb.WriteString("\n\n")

	sb.WriteString(keyStyle.Render("Trace Service Map"))
	sb.WriteString("\n  " + m.authInputs[14].View() + "\n")
	sb.WriteString(dimStyle.Render("  metrics=trace (e.g., order_API=order-api)"))
	sb.WriteString("\n\n")

	sb.WriteString(keyStyle.Render("Trace Default Service"))
	sb.WriteString("\n  " + m.authInputs[15].View() + "\n\n")

	sb.WriteString(dimStyle.Render("Note: basic auth takes precedence over bearer token."))
	sb.WriteString("\n\n")

	sb.WriteString(keyStyle.Render("Actions"))
	sb.WriteString("\nEnter to save/test, Esc to return.\n")
	sb.WriteString("Ctrl+T clears bearer token file • Ctrl+B clears basic auth file\n")

	// Status and test results are shown at the top for visibility.

	return sb.String()
}

func (m tuiModel) updateAuthEditor(msg tea.Msg) (tuiModel, tea.Cmd) {
	switch key := msg.(type) {
	case tea.KeyMsg:
		if key.Type == tea.KeyRunes && looksLikeTerminalGarbage(key.String()) {
			return m, nil
		}
		switch key.String() {
		case "esc":
			m.authEditActive = false
			m.content = m.renderContent()
			return m, nil
		case "q":
			m.authEditActive = false
			m.content = m.renderContent()
			return m, nil
		case "ctrl+c":
			m.authEditActive = false
			return m, tea.Quit
		case "pgup", "pageup", "prior", "ctrl+u", "alt+up":
			step := m.height / 2
			if step < 1 {
				step = 1
			}
			m.scrollY -= step
			if m.scrollY < 0 {
				m.scrollY = 0
			}
			m.content = m.renderAuthEditor()
			return m, nil
		case "pgdown", "pagedown", "next", "ctrl+d", "alt+down":
			step := m.height / 2
			if step < 1 {
				step = 1
			}
			contentLines := strings.Count(m.renderAuthEditor(), "\n")
			maxScroll := contentLines - m.height + 1
			if maxScroll > 0 {
				m.scrollY += step
				if m.scrollY > maxScroll {
					m.scrollY = maxScroll
				}
			}
			m.content = m.renderAuthEditor()
			return m, nil
		case "home", "g":
			m.scrollY = 0
			m.content = m.renderAuthEditor()
			return m, nil
		case "end", "G":
			contentLines := strings.Count(m.renderAuthEditor(), "\n")
			maxScroll := contentLines - m.height + 1
			if maxScroll > 0 {
				m.scrollY = maxScroll
			}
			m.content = m.renderAuthEditor()
			return m, nil
		case "ctrl+t":
			if err := config.ClearTokenFile(); err != nil {
				m.authMessage = "Failed to clear token file: " + err.Error()
			} else {
				m.authMessage = "Bearer token cleared."
			}
			m.content = m.renderAuthEditor()
			return m, nil
		case "ctrl+b":
			if err := config.ClearBasicAuthFile(); err != nil {
				m.authMessage = "Failed to clear basic auth file: " + err.Error()
			} else {
				m.authMessage = "Basic auth cleared."
			}
			m.content = m.renderAuthEditor()
			return m, nil
		case "enter":
			if m.authIndex == len(m.authInputs)-1 {
				m.applyAuthUpdates()
				m.content = m.renderAuthEditor()
				return m, nil
			}
			m.authInputs[m.authIndex].Blur()
			m.authIndex++
			m.authInputs[m.authIndex].Focus()
			m.ensureAuthVisible()
			m.content = m.renderAuthEditor()
			return m, nil
		case "tab", "down":
			m.authInputs[m.authIndex].Blur()
			m.authIndex = (m.authIndex + 1) % len(m.authInputs)
			m.authInputs[m.authIndex].Focus()
			m.ensureAuthVisible()
			m.content = m.renderAuthEditor()
			return m, nil
		case "shift+tab", "up":
			m.authInputs[m.authIndex].Blur()
			m.authIndex--
			if m.authIndex < 0 {
				m.authIndex = len(m.authInputs) - 1
			}
			m.authInputs[m.authIndex].Focus()
			m.ensureAuthVisible()
			m.content = m.renderAuthEditor()
			return m, nil
		}
	case tea.MouseMsg:
		switch key.Type {
		case tea.MouseWheelUp:
			if m.scrollY > 0 {
				m.scrollY--
			}
			m.content = m.renderAuthEditor()
			return m, nil
		case tea.MouseWheelDown:
			contentLines := strings.Count(m.renderAuthEditor(), "\n")
			maxScroll := contentLines - m.height + 1
			if maxScroll > 0 && m.scrollY < maxScroll {
				m.scrollY++
			}
			m.content = m.renderAuthEditor()
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.authInputs[m.authIndex], cmd = m.authInputs[m.authIndex].Update(msg)
	m.sanitizeAuthInputs()
	m.content = m.renderAuthEditor()
	return m, cmd
}

func (m *tuiModel) sanitizeAuthInputs() {
	for i := range m.authInputs {
		value := m.authInputs[i].Value()
		cleaned := stripTerminalArtifacts(value)
		if cleaned != value {
			m.authInputs[i].SetValue(cleaned)
			value = cleaned
		}
		if looksLikeTerminalGarbage(value) {
			m.authInputs[i].SetValue("")
		}
	}
}

func looksLikeTerminalGarbage(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	if strings.Contains(trimmed, "rgb") || strings.Contains(trimmed, "]11;") {
		return true
	}
	if strings.Contains(trimmed, "c/0c") || strings.Contains(trimmed, "0c/") || strings.Contains(trimmed, "0c0c") {
		return true
	}
	if strings.Contains(trimmed, "gb:") || strings.Contains(trimmed, "cb:") {
		return true
	}
	if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "R") {
		return true
	}
	return trimmed == "\\"
}

func stripTerminalArtifacts(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return value
	}
	cleaned := trimmed
	escapeSeq := regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)
	cleaned = escapeSeq.ReplaceAllString(cleaned, "")
	bracketSeq := regexp.MustCompile(`[\[\]][0-9;]*[A-Za-z]`)
	cleaned = bracketSeq.ReplaceAllString(cleaned, "")
	cleaned = strings.ReplaceAll(cleaned, `\`, "")
	if cleaned == trimmed {
		return value
	}
	return cleaned
}

func (m *tuiModel) ensureAuthVisible() {
	if m.height <= 0 {
		return
	}
	lineHeight := 4
	offset := 6
	estimatedLine := offset + (m.authIndex * lineHeight)
	visibleLines := m.height - 2
	if visibleLines < 1 {
		visibleLines = 1
	}
	if estimatedLine < m.scrollY {
		m.scrollY = estimatedLine
	} else if estimatedLine >= m.scrollY+visibleLines {
		m.scrollY = estimatedLine - visibleLines + 1
	}
	if m.scrollY < 0 {
		m.scrollY = 0
	}
}

func (m *tuiModel) applyAuthUpdates() {
	url := sanitizeInput(m.authInputs[0].Value())
	token := sanitizeTokenInput(m.authInputs[1].Value())
	user := sanitizeTokenInput(m.authInputs[2].Value())
	pass := sanitizeTokenInput(m.authInputs[3].Value())
	disableUpdatesValue := strings.TrimSpace(strings.ToLower(m.authInputs[4].Value()))
	promQPSValue := strings.TrimSpace(m.authInputs[5].Value())
	routeLimitValue := strings.TrimSpace(m.authInputs[6].Value())
	serviceLimitValue := strings.TrimSpace(m.authInputs[7].Value())
	traceBackend := strings.TrimSpace(m.authInputs[8].Value())
	traceURL := sanitizeInput(m.authInputs[9].Value())
	traceToken := sanitizeTokenInput(m.authInputs[10].Value())
	traceUser := sanitizeTokenInput(m.authInputs[11].Value())
	tracePass := sanitizeTokenInput(m.authInputs[12].Value())
	traceWindow := strings.TrimSpace(m.authInputs[13].Value())
	traceServiceMap := strings.TrimSpace(m.authInputs[14].Value())
	traceDefaultService := strings.TrimSpace(m.authInputs[15].Value())
	disableUpdates := false
	if disableUpdatesValue != "" {
		switch disableUpdatesValue {
		case "1", "true", "yes", "y", "on":
			disableUpdates = true
		case "0", "false", "no", "n", "off":
			disableUpdates = false
		default:
			m.authMessage = "Disable updates must be true/false."
			return
		}
	}

	cfg, err := config.Load()
	if err != nil {
		m.authMessage = "Failed to load config."
		return
	}
	if url == "" {
		m.authMessage = "Invalid URL. Please re-enter (must start with http:// or https://)."
		return
	}
	cfg.PrometheusURL = url
	cfg.PrometheusUser = strings.TrimSpace(user)
	cfg.PrometheusPass = strings.TrimSpace(pass)
	if cfg.PrometheusUser != "" || cfg.PrometheusPass != "" {
		// Basic auth takes precedence over token if both are present.
		cfg.PrometheusToken = ""
	}
	cfg.DisableUpdates = disableUpdates
	cfg.AutoDiscover = true
	if promQPSValue != "" {
		qps, err := strconv.ParseFloat(promQPSValue, 64)
		if err != nil || qps < 0 {
			m.authMessage = "Prometheus QPS must be a non-negative number."
			return
		}
		cfg.PrometheusQPS = qps
	}
	if routeLimitValue != "" {
		limit, err := strconv.Atoi(routeLimitValue)
		if err != nil || limit < 0 {
			m.authMessage = "Route cardinality limit must be a non-negative integer."
			return
		}
		cfg.RouteCardinalityLimit = limit
	}
	if serviceLimitValue != "" {
		limit, err := strconv.Atoi(serviceLimitValue)
		if err != nil || limit < 0 {
			m.authMessage = "Service cardinality limit must be a non-negative integer."
			return
		}
		cfg.ServiceCardinalityLimit = limit
	}
	if traceURL != "" && !(strings.HasPrefix(traceURL, "http://") || strings.HasPrefix(traceURL, "https://")) {
		m.authMessage = "Trace URL must start with http:// or https://"
		return
	}
	cfg.TraceBackend = traceBackend
	cfg.TraceURL = traceURL
	cfg.TraceWindow = traceWindow
	cfg.TraceServiceMap = traceServiceMap
	cfg.TraceDefaultService = traceDefaultService
	if err := saveCurrentConfig(cfg); err != nil {
		m.authMessage = "Failed to save config: " + err.Error()
		return
	}
	if token != "" {
		if err := config.WriteTokenFile(token); err != nil {
			m.authMessage = "Failed to write token: " + err.Error()
			return
		}
		cfg.PrometheusToken = token
	}
	if user != "" || pass != "" {
		if err := config.WriteBasicAuthFile(user, pass); err != nil {
			m.authMessage = "Failed to write basic auth: " + err.Error()
			return
		}
	}
	if traceToken != "" {
		if err := config.WriteTraceTokenFile(traceToken); err != nil {
			m.authMessage = "Failed to write trace token: " + err.Error()
			return
		}
		cfg.TraceToken = traceToken
	}
	if traceUser != "" || tracePass != "" {
		if err := config.WriteTraceBasicAuthFile(traceUser, tracePass); err != nil {
			m.authMessage = "Failed to write trace basic auth: " + err.Error()
			return
		}
	}

	client := prometheus.Client{
		BaseURL: cfg.PrometheusURL,
		Token:   cfg.PrometheusToken,
		User:    strings.TrimSpace(user),
		Pass:    strings.TrimSpace(pass),
		Timeout: 5 * time.Second,
		QPS:     cfg.PrometheusQPS,
	}
	msg, err := client.Check()
	if err != nil {
		m.authTestResult = "Prometheus check failed: " + err.Error()
	} else {
		m.authTestResult = "Prometheus OK: " + msg
	}

	result, err := api_latency.Collect(&client, cfg)
	if err == nil {
		status := model.SAFE
		if cfg.LatencyThresholdSeconds <= 0 {
			cfg.LatencyThresholdSeconds = 10
		}
		if result.P95 > cfg.LatencyThresholdSeconds {
			status = model.RISK
		} else if result.P90 > cfg.LatencyThresholdSeconds {
			status = model.CHECK
		}
		updated := &model.APILatency{
			Service:             cfg.APIService,
			Route:               cfg.APIRoute,
			Window:              result.Window,
			P90:                 result.P90,
			P95:                 result.P95,
			P99:                 result.P99,
			ErrorRate:           result.ErrorRate,
			ClientErrorRate:     result.ClientErrorRate,
			ServerErrorRate:     result.ServerErrorRate,
			ErrorLabel:          result.ErrorLabel,
			RPS:                 result.RPS,
			Status:              status,
			Note:                result.Note,
			Confidence:          result.Confidence,
			TopEndpointsLimit:   cfg.TopEndpoints,
			TopEndpoints:        mapLatencyEndpoints(result.TopEndpoints),
			TopEndpointsRPS:     mapRateEndpoints(result.TopEndpointsRPS),
			TopEndpointsRPSNote: result.TopEndpointsRPSNote,
			TopServicesRPS:      mapServiceRates(result.TopServicesRPS),
			TopServicesRPSNote:  result.TopServicesRPSNote,
		}
		m.applyFilteredLatency(updated)
		m.report.APINote = ""
		if m.report.APIConfig != nil {
			m.report.APIConfig.PrometheusURL = cfg.PrometheusURL
		}
	}

	m.authMessage = "Saved configuration."
}

func (m tuiModel) renderConfig() string {
	var sb strings.Builder

	sb.WriteString(titleStyle.Render("API Latency Configuration"))
	sb.WriteString("\n")
	sb.WriteString(disclaimerStyle.Render("Edit the config file or set env vars. Secrets are not edited in-app."))
	sb.WriteString("\n\n")

	cfg, cfgErr := config.Load()
	path, _ := config.ConfigPath()
	if m.report.APIConfig != nil && m.report.APIConfig.ActiveConfigPath != "" {
		path = m.report.APIConfig.ActiveConfigPath
	}
	if path == "" {
		path = "/etc/health-monitor/config.json"
	}

	sb.WriteString(markdownBlock("## 📁 Config Files"))
	sb.WriteString(fmt.Sprintf("Active: %s\n", valueStyle.Render(path)))
	if m.report.APIConfig != nil {
		sb.WriteString(fmt.Sprintf("System: %s\n", valueStyle.Render(output.EmptyOr(m.report.APIConfig.SystemConfigPath, "/etc/health-monitor/config.json"))))
		sb.WriteString(fmt.Sprintf("User:   %s\n", valueStyle.Render(output.EmptyOr(m.report.APIConfig.UserConfigPath, "~/.health-monitor/config.json"))))
	}
	sb.WriteString("\nOpen this file in any editor or use 'c' for Prometheus settings.\n")
	sb.WriteString("For auth, use token/basic auth files or env vars:\n")
	sb.WriteString(fmt.Sprintf("Token: %s\n", config.TokenFilePath()))
	sb.WriteString(fmt.Sprintf("Basic: %s\n\n", config.BasicAuthFilePath()))

	if _, err := os.Stat(path); err != nil {
		sb.WriteString(dimStyle.Render("No config file found. URL-only mode is active if PROMETHEUS_URL is set."))
		sb.WriteString("\n\n")
	}

	if cfgErr != nil {
		sb.WriteString(dimStyle.Render("Config could not be loaded. Set PROMETHEUS_URL and API_SERVICE to enable API latency checks."))
		sb.WriteString("\n")
		return sb.String()
	}

	sb.WriteString(markdownBlock("## ✅ Current Values"))
	sb.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Prometheus URL"), valueStyle.Render(output.EmptyOr(cfg.PrometheusURL, "not set"))))
	sb.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("API Service"), valueStyle.Render(output.EmptyOr(cfg.APIService, "not set"))))
	sb.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("API Route"), valueStyle.Render(output.EmptyOr(cfg.APIRoute, "all routes"))))
	sb.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Auth Mode"), valueStyle.Render(authMode(cfg))))
	sb.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Has Token"), valueStyle.Render(boolText(cfg.PrometheusToken != ""))))
	sb.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Has User/Pass"), valueStyle.Render(boolText(cfg.PrometheusUser != "" || cfg.PrometheusPass != ""))))
	sb.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Loki URL"), valueStyle.Render(output.EmptyOr(cfg.LokiURL, "not set"))))
	sb.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Loki Auth"), valueStyle.Render(boolText(cfg.LokiToken != "" || cfg.LokiUser != "" || cfg.LokiPass != ""))))
	sb.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Loki Service Label"), valueStyle.Render(output.EmptyOr(cfg.LokiServiceLabel, "auto"))))
	sb.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Loki Route Label"), valueStyle.Render(output.EmptyOr(cfg.LokiRouteLabel, "auto"))))
	sb.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Grafana URL"), valueStyle.Render(output.EmptyOr(cfg.GrafanaURL, "not set"))))
	sb.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Grafana Prom DS"), valueStyle.Render(output.EmptyOr(cfg.GrafanaPromDataSource, "not set"))))
	sb.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Grafana Loki DS"), valueStyle.Render(output.EmptyOr(cfg.GrafanaLokiDataSource, "not set"))))
	sb.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Grafana Trace DS"), valueStyle.Render(output.EmptyOr(cfg.GrafanaTraceDataSource, "not set"))))
	sb.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Trace Backend"), valueStyle.Render(output.EmptyOr(cfg.TraceBackend, "auto"))))
	sb.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Trace URL"), valueStyle.Render(output.EmptyOr(cfg.TraceURL, "not set"))))
	sb.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Trace Window"), valueStyle.Render(output.EmptyOr(cfg.TraceWindow, "4m"))))
	sb.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Trace Service Map"), valueStyle.Render(output.EmptyOr(cfg.TraceServiceMap, "none"))))
	sb.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Trace Default Service"), valueStyle.Render(output.EmptyOr(cfg.TraceDefaultService, "auto"))))
	if m.report.APIConfig != nil {
		sb.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Token Source"), valueStyle.Render(output.EmptyOr(m.report.APIConfig.TokenSource, "none"))))
		sb.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Token File"), valueStyle.Render(output.EmptyOr(m.report.APIConfig.TokenFilePath, "/etc/health-monitor/prometheus.token"))))
		if m.report.APIConfig.DisableUpdates {
			sb.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Updates"), valueStyle.Render("disabled")))
		} else {
			sb.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Updates"), valueStyle.Render("enabled")))
		}
	}
	sb.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Auto Discover"), valueStyle.Render(boolText(cfg.AutoDiscover))))
	sb.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Window"), valueStyle.Render(output.EmptyOr(cfg.Window, "5m"))))
	sb.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Latency Metric"), valueStyle.Render(output.EmptyOr(cfg.LatencyMetric, "http_request_duration_seconds"))))
	sb.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Request Metric"), valueStyle.Render(output.EmptyOr(cfg.RequestCountMetric, "http_requests_total"))))
	sb.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Service Label"), valueStyle.Render(output.EmptyOr(cfg.ServiceLabel, "service"))))
	sb.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Route Label"), valueStyle.Render(output.EmptyOr(cfg.RouteLabel, "route"))))
	sb.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Error Label"), valueStyle.Render(output.EmptyOr(cfg.ErrorLabel, "status"))))
	sb.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Error Regex"), valueStyle.Render(output.EmptyOr(cfg.ErrorRegex, "5.."))))
	sb.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Extra Selectors"), valueStyle.Render(output.EmptyOr(cfg.ExtraLabelSelectors, "none"))))

	sb.WriteString("\n")
	sb.WriteString(markdownBlock("## 🧰 Env Vars (Alternative)"))
	sb.WriteString("PROMETHEUS_URL, PROMETHEUS_TOKEN\n")
	sb.WriteString("PROMETHEUS_TOKEN_FILE\n")
	sb.WriteString("PROMETHEUS_USER, PROMETHEUS_PASS\n")
	sb.WriteString("API_SERVICE, API_ROUTE\n")
	sb.WriteString("API_SERVICE_LABEL, API_ROUTE_LABEL\n")
	sb.WriteString("API_LATENCY_METRIC, API_REQUESTS_METRIC\n")
	sb.WriteString("API_AUTO_DISCOVER\n")
	sb.WriteString("API_ERROR_LABEL, API_ERROR_REGEX\n")
	sb.WriteString("API_LATENCY_THRESHOLD, PROMETHEUS_WINDOW\n")
	sb.WriteString("PROMETHEUS_EXTRA_LABELS\n")
	sb.WriteString("TRACE_BACKEND, TRACE_URL\n")
	sb.WriteString("TRACE_TOKEN, TRACE_USER, TRACE_PASS\n")
	sb.WriteString("TRACE_SERVICE_MAP, TRACE_DEFAULT_SERVICE, TRACE_WINDOW\n")
	sb.WriteString("GRAFANA_TRACE_DS\n")

	return sb.String()
}

func (m tuiModel) startEdit() tuiModel {
	cfg, err := config.Load()
	if err != nil {
		cfg = config.Default()
	}
	m.editConfig = cfg
	m.editFields = []editField{
		{key: "PROMETHEUS_URL", label: "Prometheus URL", placeholder: "https://prom.example.com"},
		{key: "API_SERVICE", label: "API Service (optional if no service label)", placeholder: "payment"},
		{key: "API_ROUTE", label: "API Route (optional)", placeholder: "leave blank for all routes"},
		{key: "API_SERVICE_LABEL", label: "Service Label", placeholder: "service"},
		{key: "API_ROUTE_LABEL", label: "Route Label", placeholder: "route"},
		{key: "API_LATENCY_METRIC", label: "Latency Metric", placeholder: "http_request_duration_seconds"},
		{key: "API_REQUESTS_METRIC", label: "Requests Metric", placeholder: "http_requests_total"},
		{key: "API_AUTO_DISCOVER", label: "Auto Discover (true/false)", placeholder: "false"},
		{key: "API_ERROR_LABEL", label: "Error Label", placeholder: "status"},
		{key: "API_ERROR_REGEX", label: "Error Regex", placeholder: "5.."},
		{key: "PROMETHEUS_WINDOW", label: "Window", placeholder: "5m"},
		{key: "PROMETHEUS_EXTRA_LABELS", label: "Extra Label Selectors", placeholder: `env="prod",cluster="prod-1"`},
		{key: "API_LATENCY_THRESHOLD", label: "Latency Threshold (seconds)", placeholder: "10"},
		{key: "EXPORT_PATH", label: "Export Path (optional)", placeholder: "/tmp"},
		{key: "TRACE_BACKEND", label: "Trace Backend (tempo/jaeger)", placeholder: "tempo"},
		{key: "TRACE_URL", label: "Trace Backend URL", placeholder: "https://tempo.example.com"},
		{key: "TRACE_WINDOW", label: "Trace Window", placeholder: "4m"},
		{key: "TRACE_SERVICE_MAP", label: "Trace Service Map (metrics=trace,...)", placeholder: "order_API=order-api,postgres_db=postgres"},
		{key: "TRACE_DEFAULT_SERVICE", label: "Trace Default Service (optional)", placeholder: "order-api"},
	}

	m.editInputs = make([]textinput.Model, len(m.editFields))
	for i, field := range m.editFields {
		input := textinput.New()
		input.Prompt = ""
		input.Placeholder = field.placeholder
		input.CharLimit = 256
		input.Width = 60
		input.SetValue(m.getConfigValue(field.key, cfg))
		m.editInputs[i] = input
	}
	m.editIndex = 0
	m.editInputs[0].Focus()
	m.editActive = true
	m.scrollY = 0
	m.editError = ""
	m.editSaved = false
	return m
}

func (m tuiModel) updateEdit(msg tea.Msg) (tuiModel, tea.Cmd) {
	switch key := msg.(type) {
	case tea.KeyMsg:
		if key.Type == tea.KeyRunes && looksLikeTerminalGarbage(key.String()) {
			return m, nil
		}
		switch key.String() {
		case "esc":
			m.editActive = false
			m.editError = ""
			m.editSaved = false
			m.showConfig = true
			m.content = m.renderConfig()
			m.content = m.renderEdit()
			return m, nil
		case "q", "Q", "ctrl+c":
			m.editActive = false
			m.editError = ""
			m.editSaved = false
			m.showConfig = true
			m.content = m.renderConfig()
			m.content = m.renderEdit()
			return m, nil
		case "pgup", "pageup", "prior", "ctrl+u", "alt+up":
			step := m.height / 2
			if step < 1 {
				step = 1
			}
			m.scrollY -= step
			if m.scrollY < 0 {
				m.scrollY = 0
			}
			m.content = m.renderEdit()
			return m, nil
		case "pgdown", "pagedown", "next", "ctrl+d", "alt+down":
			step := m.height / 2
			if step < 1 {
				step = 1
			}
			contentLines := strings.Count(m.renderEdit(), "\n")
			maxScroll := contentLines - m.height + 1
			if maxScroll > 0 {
				m.scrollY += step
				if m.scrollY > maxScroll {
					m.scrollY = maxScroll
				}
			}
			m.content = m.renderEdit()
			return m, nil
		case "home", "g":
			m.scrollY = 0
			m.content = m.renderEdit()
			return m, nil
		case "end", "G":
			contentLines := strings.Count(m.renderEdit(), "\n")
			maxScroll := contentLines - m.height + 1
			if maxScroll > 0 {
				m.scrollY = maxScroll
			}
			m.content = m.renderEdit()
			return m, nil
		case "enter":
			if m.editIndex == len(m.editInputs)-1 {
				updated, err := m.applyInputs()
				if err != nil {
					m.editError = err.Error()
					m.content = m.renderEdit()
					return m, nil
				}
				if err := config.Save(updated); err != nil {
					m.editError = "Failed to save config: " + err.Error()
					m.content = m.renderEdit()
					return m, nil
				}
				m.editActive = false
				m.editSaved = true
				m.showConfig = true
				m.content = m.renderConfig()
				return m, nil
			}
			m.editInputs[m.editIndex].Blur()
			m.editIndex++
			m.editInputs[m.editIndex].Focus()
			m.ensureEditVisible()
			m.content = m.renderEdit()
			return m, nil
		case "tab", "down":
			m.editInputs[m.editIndex].Blur()
			m.editIndex = (m.editIndex + 1) % len(m.editInputs)
			m.editInputs[m.editIndex].Focus()
			m.ensureEditVisible()
			m.content = m.renderEdit()
			return m, nil
		case "shift+tab", "up":
			m.editInputs[m.editIndex].Blur()
			m.editIndex--
			if m.editIndex < 0 {
				m.editIndex = len(m.editInputs) - 1
			}
			m.editInputs[m.editIndex].Focus()
			m.ensureEditVisible()
			m.content = m.renderEdit()
			return m, nil
		}
	case tea.MouseMsg:
		switch key.Type {
		case tea.MouseWheelUp:
			if m.scrollY > 0 {
				m.scrollY--
			}
			m.content = m.renderEdit()
			return m, nil
		case tea.MouseWheelDown:
			contentLines := strings.Count(m.renderEdit(), "\n")
			maxScroll := contentLines - m.height + 1
			if maxScroll > 0 && m.scrollY < maxScroll {
				m.scrollY++
			}
			m.content = m.renderEdit()
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.editInputs[m.editIndex], cmd = m.editInputs[m.editIndex].Update(msg)
	m.sanitizeEditInputs()
	m.content = m.renderEdit()
	return m, cmd
}

func (m tuiModel) renderEdit() string {
	var sb strings.Builder

	sb.WriteString(titleStyle.Render("Edit API Latency Configuration"))
	sb.WriteString("\n")
	sb.WriteString(disclaimerStyle.Render("Press Enter to move, Esc to cancel, Enter on last field to save. Secrets are not edited here."))
	sb.WriteString("\n\n")

	for i, field := range m.editFields {
		prefix := "  "
		if i == m.editIndex {
			prefix = "> "
		}
		sb.WriteString(prefix + keyStyle.Render(field.label) + "\n")
		sb.WriteString("    " + m.editInputs[i].View() + "\n\n")
	}

	if m.editError != "" {
		sb.WriteString(red.Render("Error: " + m.editError))
		sb.WriteString("\n")
	}

	if m.editSaved {
		sb.WriteString(green.Render("Saved. Press 'c' to review config or 'q' to exit."))
		sb.WriteString("\n")
	}

	return sb.String()
}

func (m *tuiModel) sanitizeEditInputs() {
	for i := range m.editInputs {
		value := m.editInputs[i].Value()
		cleaned := stripTerminalArtifacts(value)
		if cleaned != value {
			m.editInputs[i].SetValue(cleaned)
			value = cleaned
		}
		if looksLikeTerminalGarbage(value) {
			m.editInputs[i].SetValue("")
		}
	}
}

func (m *tuiModel) ensureEditVisible() {
	if m.height <= 0 {
		return
	}
	lineHeight := 3
	offset := 4
	estimatedLine := offset + (m.editIndex * lineHeight)
	visibleLines := m.height - 2
	if visibleLines < 1 {
		visibleLines = 1
	}
	if estimatedLine < m.scrollY {
		m.scrollY = estimatedLine
	} else if estimatedLine >= m.scrollY+visibleLines {
		m.scrollY = estimatedLine - visibleLines + 1
	}
	if m.scrollY < 0 {
		m.scrollY = 0
	}
}


func authMode(cfg config.Config) string {
	if cfg.PrometheusToken != "" {
		return "token"
	}
	if cfg.PrometheusUser != "" || cfg.PrometheusPass != "" {
		return "user/pass"
	}
	return "none"
}

func boolText(val bool) string {
	if val {
		return "yes"
	}
	return "no"
}

func (m tuiModel) updateServiceSelection(msg tea.Msg) (tuiModel, tea.Cmd) {
	switch key := msg.(type) {
	case tea.KeyMsg:
		switch key.String() {
		case "q", "Q", "esc":
			m.showServices = false
			m.loadingServices = false
			m.content = m.renderContent()
			return m, nil
		case "up", "k":
			if m.serviceIndex > 0 {
				m.serviceIndex--
			}
			m.content = m.renderServices()
			return m, nil
		case "down", "j":
			if m.serviceIndex < len(m.serviceList)-1 {
				m.serviceIndex++
			}
			m.content = m.renderServices()
			return m, nil
		case "enter":
			if len(m.serviceList) == 0 {
				return m, nil
			}
			service := m.serviceList[m.serviceIndex]
			m.loadingServices = true
			m.serviceMessage = "Applying filter..."
			m.content = m.renderServices()
			return m, m.applyServiceFilterCmd(service)
		}
	}
	return m, nil
}

func (m *tuiModel) applyServiceFilterCmd(service string) tea.Cmd {
	return func() tea.Msg {
		cfg, err := config.Load()
		if err != nil {
			return filterResultMsg{err: fmt.Errorf("failed to load config: %v", err)}
		}
		if cfg.PrometheusURL == "" && m.report.APIConfig != nil {
			cfg.PrometheusURL = m.report.APIConfig.PrometheusURL
		}
		if cfg.PrometheusURL == "" {
			return filterResultMsg{err: fmt.Errorf("missing Prometheus URL")}
		}

		cfg.APIService = normalizeSelection(service)
		cfg.AutoDiscover = true
		if cfg.APIService == "" {
			resetAutoDiscoverOverrides(&cfg)
		}

		client := prometheus.Client{
			BaseURL: cfg.PrometheusURL,
			Token:   cfg.PrometheusToken,
			User:    cfg.PrometheusUser,
			Pass:    cfg.PrometheusPass,
			Timeout: 10 * time.Second,
			QPS:     cfg.PrometheusQPS,
		}

		result, err := api_latency.Collect(&client, cfg)
		if err != nil {
			return filterResultMsg{err: err}
		}

		status := model.SAFE
		if cfg.LatencyThresholdSeconds <= 0 {
			cfg.LatencyThresholdSeconds = 2.0
		}
		if result.P95 > cfg.LatencyThresholdSeconds {
			status = model.RISK
		} else if result.P90 > cfg.LatencyThresholdSeconds {
			status = model.CHECK
		}

		filtered := &model.APILatency{
			Service:                cfg.APIService,
			Route:                  cfg.APIRoute,
			Window:                 result.Window,
			P90:                    result.P90,
			P95:                    result.P95,
			P99:                    result.P99,
			BaselineWindow:         result.BaselineWindow,
			BaselineP95:            result.BaselineP95,
			BaselineErrorRate:      result.BaselineErrorRate,
			BaselineRPS:            result.BaselineRPS,
			DeltaP95:               result.DeltaP95,
			DeltaErrorRate:         result.DeltaErrorRate,
			DeltaRPS:               result.DeltaRPS,
			ErrorRate:              result.ErrorRate,
			ClientErrorRate:        result.ClientErrorRate,
			ServerErrorRate:        result.ServerErrorRate,
			RPS:                    result.RPS,
			TopEndpoints:           mapLatencyEndpoints(result.TopEndpoints),
			TopEndpointsRPS:        mapRateEndpoints(result.TopEndpointsRPS),
			TopServicesRPS:         mapServiceRates(result.TopServicesRPS),
			Status:                 status,
		}
		
		newReport := m.report
		newReport.APILatency = filtered

		return filterResultMsg{
			report:  newReport,
			message: fmt.Sprintf("Filter applied for service: %s (P95: %.2fs)", service, result.P95),
		}
	}
}



func extractServices(base []model.ServiceRate, current *model.APILatency) []string {
	source := base
	if len(source) == 0 && current != nil {
		source = current.TopServicesRPS
	}
	services := []string{"All services"}
	for _, svc := range source {
		if strings.TrimSpace(svc.Service) == "" {
			continue
		}
		services = append(services, svc.Service)
	}
	return services
}

func selectorLookback(window string) time.Duration {
	clean := strings.TrimSpace(window)
	if clean == "" {
		return time.Hour
	}
	if d, err := time.ParseDuration(clean); err == nil && d > 0 {
		if d < 5*time.Minute {
			return 5 * time.Minute
		}
		if d > 6*time.Hour {
			return 6 * time.Hour
		}
		return d
	}
	return time.Hour
}

func (m tuiModel) selectorConfig() (config.Config, string) {
	cfg, err := config.Load()
	if err != nil {
		return cfg, "Failed to load config for selector."
	}
	if cfg.PrometheusURL == "" && m.report.APIConfig != nil {
		cfg.PrometheusURL = m.report.APIConfig.PrometheusURL
	}
	if cfg.RequestCountMetric == "" && m.report.APIConfig != nil {
		cfg.RequestCountMetric = m.report.APIConfig.RequestMetric
	}
	if cfg.LatencyMetric == "" && m.report.APIConfig != nil {
		cfg.LatencyMetric = m.report.APIConfig.LatencyMetric
	}
	if cfg.ServiceLabel == "" && m.report.APIConfig != nil {
		cfg.ServiceLabel = m.report.APIConfig.ServiceLabel
	}
	if cfg.RouteLabel == "" && m.report.APIConfig != nil {
		cfg.RouteLabel = m.report.APIConfig.RouteLabel
	}
	if cfg.Window == "" && m.report.APIConfig != nil {
		cfg.Window = m.report.APIConfig.Window
	}
	return cfg, ""
}

func pickSelectorMetric(cfg config.Config) string {
	if strings.TrimSpace(cfg.RequestCountMetric) != "" {
		return cfg.RequestCountMetric
	}
	if strings.TrimSpace(cfg.LatencyMetric) != "" {
		return cfg.LatencyMetric
	}
	return ""
}

func (m tuiModel) fetchServiceListFromProm() ([]string, string) {
	cfg, msg := m.selectorConfig()
	if msg != "" {
		return nil, msg
	}
	if cfg.PrometheusURL == "" {
		return nil, "Missing Prometheus URL; cannot load services."
	}
	metric := pickSelectorMetric(cfg)
	if metric == "" || strings.TrimSpace(cfg.ServiceLabel) == "" {
		// We'll try common labels if the configured label is missing.
	}
	client := prometheus.Client{
		BaseURL: cfg.PrometheusURL,
		Token:   cfg.PrometheusToken,
		User:    cfg.PrometheusUser,
		Pass:    cfg.PrometheusPass,
		Timeout: 6 * time.Second,
		QPS:     cfg.PrometheusQPS,
	}
	label := strings.TrimSpace(cfg.ServiceLabel)
	if label != "" {
		if values, err := api_latency.FetchLabelValues(&client, metric, label, selectorLookback(cfg.Window), 200); err == nil && len(values) > 0 {
			list := append([]string{"All services"}, values...)
			return list, ""
		}
	}
	candidates := []string{"job", "service", "app", "application", "svc", "service_name"}
	for _, candidate := range candidates {
		if candidate == label {
			continue
		}
		values, err := api_latency.FetchLabelValues(&client, metric, candidate, selectorLookback(cfg.Window), 200)
		if err == nil && len(values) > 0 {
			list := append([]string{"All services"}, values...)
			return list, "Using detected service label: " + candidate
		}
	}
	return nil, "No services detected from Prometheus."
}

func (m tuiModel) fetchEndpointListFromProm() ([]string, string) {
	cfg, msg := m.selectorConfig()
	if msg != "" {
		return nil, msg
	}
	if cfg.PrometheusURL == "" {
		return nil, "Missing Prometheus URL; cannot load endpoints."
	}
	metric := pickSelectorMetric(cfg)
	if metric == "" || strings.TrimSpace(cfg.RouteLabel) == "" {
		return nil, "Missing request metric or route label."
	}
	client := prometheus.Client{
		BaseURL: cfg.PrometheusURL,
		Token:   cfg.PrometheusToken,
		User:    cfg.PrometheusUser,
		Pass:    cfg.PrometheusPass,
		Timeout: 6 * time.Second,
		QPS:     cfg.PrometheusQPS,
	}
	values, err := api_latency.FetchLabelValues(&client, metric, cfg.RouteLabel, selectorLookback(cfg.Window), 200)
	if err != nil {
		return nil, "No endpoints detected from Prometheus."
	}
	list := append([]string{"All routes"}, values...)
	return list, ""
}

func (m tuiModel) updateEndpointSelection(msg tea.Msg) (tuiModel, tea.Cmd) {
	switch key := msg.(type) {
	case tea.KeyMsg:
		switch key.String() {
		case "q", "Q", "esc":
			m.showEndpoints = false
			m.loadingEndpoints = false
			m.content = m.renderContent()
			return m, nil
		case "up", "k":
			if m.endpointIndex > 0 {
				m.endpointIndex--
			}
			m.content = m.renderEndpoints()
			return m, nil
		case "down", "j":
			if m.endpointIndex < len(m.endpointList)-1 {
				m.endpointIndex++
			}
			m.content = m.renderEndpoints()
			return m, nil
		case "enter":
			if len(m.endpointList) == 0 {
				return m, nil
			}
			endpoint := m.endpointList[m.endpointIndex]
			m.loadingEndpoints = true
			m.endpointMessage = "Applying filter..."
			m.content = m.renderEndpoints()
			return m, m.applyEndpointFilterCmd(endpoint)
		}
	}
	return m, nil
}

func (m *tuiModel) applyEndpointFilterCmd(endpoint string) tea.Cmd {
	return func() tea.Msg {
		cfg, err := config.Load()
		if err != nil {
			return filterResultMsg{err: fmt.Errorf("failed to load config: %v", err)}
		}
		if cfg.PrometheusURL == "" && m.report.APIConfig != nil {
			cfg.PrometheusURL = m.report.APIConfig.PrometheusURL
		}
		if cfg.PrometheusURL == "" {
			return filterResultMsg{err: fmt.Errorf("missing Prometheus URL")}
		}

		cfg.APIRoute = normalizeSelection(endpoint)
		cfg.AutoDiscover = true
		if cfg.APIRoute == "" {
			resetAutoDiscoverOverrides(&cfg)
		}

		client := prometheus.Client{
			BaseURL: cfg.PrometheusURL,
			Token:   cfg.PrometheusToken,
			User:    cfg.PrometheusUser,
			Pass:    cfg.PrometheusPass,
			Timeout: 10 * time.Second,
			QPS:     cfg.PrometheusQPS,
		}

		result, err := api_latency.Collect(&client, cfg)
		if err != nil {
			return filterResultMsg{err: err}
		}

		status := model.SAFE
		if cfg.LatencyThresholdSeconds <= 0 {
			cfg.LatencyThresholdSeconds = 2.0
		}
		if result.P95 > cfg.LatencyThresholdSeconds {
			status = model.RISK
		} else if result.P90 > cfg.LatencyThresholdSeconds {
			status = model.CHECK
		}

		filtered := &model.APILatency{
			Service:                cfg.APIService,
			Route:                  cfg.APIRoute,
			Window:                 result.Window,
			P90:                    result.P90,
			P95:                    result.P95,
			P99:                    result.P99,
			BaselineWindow:         result.BaselineWindow,
			BaselineP95:            result.BaselineP95,
			BaselineErrorRate:      result.BaselineErrorRate,
			BaselineRPS:            result.BaselineRPS,
			DeltaP95:               result.DeltaP95,
			DeltaErrorRate:         result.DeltaErrorRate,
			DeltaRPS:               result.DeltaRPS,
			ErrorRate:              result.ErrorRate,
			ClientErrorRate:        result.ClientErrorRate,
			ServerErrorRate:        result.ServerErrorRate,
			RPS:                    result.RPS,
			TopEndpoints:           mapLatencyEndpoints(result.TopEndpoints),
			TopEndpointsRPS:        mapRateEndpoints(result.TopEndpointsRPS),
			TopServicesRPS:         mapServiceRates(result.TopServicesRPS),
			Status:                 status,
		}
		
		newReport := m.report
		newReport.APILatency = filtered

		return filterResultMsg{
			report:  newReport,
			message: fmt.Sprintf("Filter applied for endpoint: %s (P95: %.2fs)", endpoint, result.P95),
		}
	}
}



func extractEndpoints(baseRPS []model.EndpointRate, baseP95 []model.EndpointLatency, current *model.APILatency) []string {
	seen := map[string]struct{}{}
	endpoints := []string{"All routes"}

	sourceRPS := baseRPS
	sourceP95 := baseP95
	if len(sourceRPS) == 0 && current != nil {
		sourceRPS = current.TopEndpointsRPS
	}
	if len(sourceP95) == 0 && current != nil {
		sourceP95 = current.TopEndpoints
	}

	for _, item := range sourceRPS {
		if strings.TrimSpace(item.Route) == "" {
			continue
		}
		if _, ok := seen[item.Route]; !ok {
			seen[item.Route] = struct{}{}
			endpoints = append(endpoints, item.Route)
		}
	}
	for _, item := range sourceP95 {
		if strings.TrimSpace(item.Route) == "" {
			continue
		}
		if _, ok := seen[item.Route]; !ok {
			seen[item.Route] = struct{}{}
			endpoints = append(endpoints, item.Route)
		}
	}
	return endpoints
}

func (m *tuiModel) applyFilteredLatency(filtered *model.APILatency) {
	if m.baseTopEndpoints == nil && m.report.APILatency != nil {
		m.baseTopEndpoints = m.report.APILatency.TopEndpoints
	}
	if m.baseTopEndpointsRPS == nil && m.report.APILatency != nil {
		m.baseTopEndpointsRPS = m.report.APILatency.TopEndpointsRPS
	}
	if m.baseTopServicesRPS == nil && m.report.APILatency != nil {
		m.baseTopServicesRPS = m.report.APILatency.TopServicesRPS
	}
	if len(filtered.TopEndpoints) == 0 && len(m.baseTopEndpoints) > 0 {
		filtered.TopEndpoints = m.baseTopEndpoints
	}
	if len(filtered.TopEndpointsRPS) == 0 && len(m.baseTopEndpointsRPS) > 0 {
		filtered.TopEndpointsRPS = m.baseTopEndpointsRPS
	}
	if len(filtered.TopServicesRPS) == 0 && len(m.baseTopServicesRPS) > 0 {
		filtered.TopServicesRPS = m.baseTopServicesRPS
	}
	m.report.APILatency = filtered
}

func normalizeSelection(value string) string {
	trimmed := strings.TrimSpace(value)
	switch strings.ToLower(trimmed) {
	case "all services", "all endpoints", "all routes", "all":
		return ""
	default:
		return value
	}
}

func resetAutoDiscoverOverrides(cfg *config.Config) {
	cfg.AutoDiscover = true
	cfg.LatencyMetric = ""
	cfg.RequestCountMetric = ""
	cfg.ServiceLabel = ""
	cfg.RouteLabel = ""
	cfg.APIService = ""
	cfg.APIRoute = ""
}

func selectionLabel(value string, label string) string {
	if strings.TrimSpace(value) == "" || strings.HasPrefix(strings.ToLower(value), "all ") {
		return "All " + label
	}
	return value
}

func mapLatencyEndpoints(items []api_latency.EndpointLatency) []model.EndpointLatency {
	if len(items) == 0 {
		return nil
	}
	out := make([]model.EndpointLatency, 0, len(items))
	for _, item := range items {
		out = append(out, model.EndpointLatency{Route: item.Route, P95: item.P95})
	}
	return out
}

func mapEndpointRegressions(items []api_latency.EndpointRegression) []model.EndpointRegression {
	if len(items) == 0 {
		return nil
	}
	out := make([]model.EndpointRegression, 0, len(items))
	for _, item := range items {
		out = append(out, model.EndpointRegression{
			Route:       item.Route,
			NowP95:      item.NowP95,
			BaselineP95: item.BaselineP95,
			DeltaP95:    item.DeltaP95,
		})
	}
	return out
}

func mapRateEndpoints(items []api_latency.EndpointRate) []model.EndpointRate {
	if len(items) == 0 {
		return nil
	}
	out := make([]model.EndpointRate, 0, len(items))
	for _, item := range items {
		out = append(out, model.EndpointRate{Route: item.Route, RPS: item.RPS})
	}
	return out
}

func mapServiceRates(items []api_latency.ServiceRate) []model.ServiceRate {
	if len(items) == 0 {
		return nil
	}
	out := make([]model.ServiceRate, 0, len(items))
	for _, item := range items {
		out = append(out, model.ServiceRate{Service: item.Service, RPS: item.RPS})
	}
	return out
}

/* formatLatency moved to tui_utils.go */

/* formatPercent moved to tui_utils.go */

func sanitizeInput(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if strings.Contains(trimmed, "rgb:") || strings.Contains(trimmed, "c/0c") || strings.Contains(trimmed, "0c/") {
		return ""
	}
	var b strings.Builder
	b.Grow(len(trimmed))
	for i := 0; i < len(trimmed); i++ {
		ch := trimmed[i]
		if ch == 0x1b { // ESC
			for i+1 < len(trimmed) && trimmed[i+1] != 'm' && trimmed[i+1] != '\\' && trimmed[i+1] != 'G' && trimmed[i+1] != ']' {
				i++
			}
			continue
		}
		if ch < 0x20 || ch == 0x7f {
			continue
		}
		b.WriteByte(ch)
	}
	cleaned := b.String()
	if !strings.Contains(cleaned, "http") {
		return ""
	}
	return cleaned
}

func sanitizeInputLoose(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if strings.Contains(trimmed, "rgb:") || strings.Contains(trimmed, "c/0c") || strings.Contains(trimmed, "0c/") {
		return ""
	}
	var b strings.Builder
	b.Grow(len(trimmed))
	for i := 0; i < len(trimmed); i++ {
		ch := trimmed[i]
		if ch == 0x1b || ch < 0x20 || ch == 0x7f {
			continue
		}
		b.WriteByte(ch)
	}
	return b.String()
}

func sanitizeTokenInput(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if strings.Contains(trimmed, "rgb:") || strings.Contains(trimmed, "]11;") || strings.Contains(trimmed, "c/0c") || strings.Contains(trimmed, "0c/") {
		return ""
	}
	var b strings.Builder
	b.Grow(len(trimmed))
	for i := 0; i < len(trimmed); i++ {
		ch := trimmed[i]
		if ch == 0x1b || ch < 0x20 || ch == 0x7f {
			continue
		}
		b.WriteByte(ch)
	}
	return b.String()
}

func wrapNoteLines(note string) []string {
	parts := strings.Split(note, " • ")
	var lines []string
	line := ""
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if line == "" {
			line = part
			continue
		}
		if len(line)+len(part)+3 <= 72 {
			line = line + " • " + part
		} else {
			lines = append(lines, line)
			line = part
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

func (m tuiModel) getConfigValue(key string, cfg config.Config) string {
	switch key {
	case "PROMETHEUS_URL":
		return cfg.PrometheusURL
	case "API_SERVICE":
		return cfg.APIService
	case "API_ROUTE":
		return cfg.APIRoute
	case "API_SERVICE_LABEL":
		return cfg.ServiceLabel
	case "API_ROUTE_LABEL":
		return cfg.RouteLabel
	case "API_LATENCY_METRIC":
		return cfg.LatencyMetric
	case "API_REQUESTS_METRIC":
		return cfg.RequestCountMetric
	case "API_ERROR_LABEL":
		return cfg.ErrorLabel
	case "API_ERROR_REGEX":
		return cfg.ErrorRegex
	case "API_AUTO_DISCOVER":
		if cfg.AutoDiscover {
			return "true"
		}
		return "false"
	case "PROMETHEUS_WINDOW":
		return cfg.Window
	case "PROMETHEUS_EXTRA_LABELS":
		return cfg.ExtraLabelSelectors
	case "API_LATENCY_THRESHOLD":
		if cfg.LatencyThresholdSeconds > 0 {
			return fmt.Sprintf("%.2f", cfg.LatencyThresholdSeconds)
		}
		return ""
	case "EXPORT_PATH":
		return cfg.ExportPath
	case "TRACE_BACKEND":
		return cfg.TraceBackend
	case "TRACE_URL":
		return cfg.TraceURL
	case "TRACE_WINDOW":
		return cfg.TraceWindow
	case "TRACE_SERVICE_MAP":
		return cfg.TraceServiceMap
	case "TRACE_DEFAULT_SERVICE":
		return cfg.TraceDefaultService
	default:
		return ""
	}
}

func (m tuiModel) applyInputs() (config.Config, error) {
	cfg := m.editConfig

	for i, field := range m.editFields {
		val := strings.TrimSpace(m.editInputs[i].Value())
		switch field.key {
		case "PROMETHEUS_URL":
			cfg.PrometheusURL = val
		case "API_SERVICE":
			cfg.APIService = val
		case "API_ROUTE":
			cfg.APIRoute = val
		case "API_SERVICE_LABEL":
			cfg.ServiceLabel = val
		case "API_ROUTE_LABEL":
			cfg.RouteLabel = val
		case "API_LATENCY_METRIC":
			cfg.LatencyMetric = val
		case "API_REQUESTS_METRIC":
			cfg.RequestCountMetric = val
		case "API_ERROR_LABEL":
			cfg.ErrorLabel = val
		case "API_ERROR_REGEX":
			cfg.ErrorRegex = val
		case "API_AUTO_DISCOVER":
			cfg.AutoDiscover = val == "1" || strings.EqualFold(val, "true") || strings.EqualFold(val, "yes")
		case "PROMETHEUS_WINDOW":
			cfg.Window = val
		case "PROMETHEUS_EXTRA_LABELS":
			cfg.ExtraLabelSelectors = val
		case "API_LATENCY_THRESHOLD":
			if val == "" {
				cfg.LatencyThresholdSeconds = 0
				continue
			}
			parsed, err := strconv.ParseFloat(val, 64)
			if err != nil {
				return cfg, fmt.Errorf("invalid latency threshold: %s", val)
			}
			cfg.LatencyThresholdSeconds = parsed
		case "EXPORT_PATH":
			cfg.ExportPath = val
		case "TRACE_BACKEND":
			cfg.TraceBackend = val
		case "TRACE_URL":
			cfg.TraceURL = val
		case "TRACE_WINDOW":
			cfg.TraceWindow = val
		case "TRACE_SERVICE_MAP":
			cfg.TraceServiceMap = val
		case "TRACE_DEFAULT_SERVICE":
			cfg.TraceDefaultService = val
		}
	}

	return cfg, nil
}

func statusStyle(s model.Status) lipgloss.Style {
	switch s {
	case model.SAFE:
		return green
	case model.CHECK:
		return yellow
	case model.RISK:
		return red
	default:
		return lipgloss.NewStyle()
	}
}

var (
	glamourRenderer *glamour.TermRenderer
	glamourOnce     sync.Once
)

func markdownBlock(md string) string {
	glamourOnce.Do(func() {
		r, _ := glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(80),
		)
		glamourRenderer = r
	})

	if glamourRenderer == nil {
		return md
	}

	out, _ := glamourRenderer.Render(md)
	return out
}

// PrintInteractiveTUI runs the interactive scrollable TUI
func PrintInteractiveTUI(report model.Report) error {
	defer func() {
		if r := recover(); r != nil {
			os.WriteFile("panic_tui.log", []byte(fmt.Sprintf("PANIC: %v\n", r)), 0644)
			panic(r)
		}
	}()
	// Suppress log output to prevent debug logs from bleeding into TUI
	log.SetOutput(io.Discard)
	
	p := tea.NewProgram(
		initialModel(report),
		tea.WithAltScreen(),
	)
	
	_, err := p.Run()
	if err != nil {
		os.WriteFile("tui_error.log", []byte(fmt.Sprintf("ERROR: %v\n", err)), 0644)
	}
	return err
}

func (m tuiModel) renderClusterHealth() string {
	var sb strings.Builder
	sb.WriteString(titleStyle.Render("Cluster Health Overview"))
	sb.WriteString("\n")
	sb.WriteString(disclaimerStyle.Render("Usage and saturation metrics for the active cluster/infrastructure."))
	sb.WriteString("\n\n")

	if m.report.InfraStatus == nil || m.report.InfraStatus.ClusterMetrics == nil {
		sb.WriteString(dimStyle.Render("  No cluster metrics available. Ensure Prometheus is configured and reachable."))
		sb.WriteString("\n")
		return sb.String()
	}

	metrics := m.report.InfraStatus.ClusterMetrics

	// Resource Gauges
	sb.WriteString(markdownBlock("## 🔋 Resource Consumption"))
	sb.WriteString(renderGauge("CPU Usage", metrics.CPUUsage))
	sb.WriteString(renderGauge("Memory Usage", metrics.MemoryUsage))
	sb.WriteString(renderGauge("Disk Usage", metrics.DiskUsage))
	sb.WriteString("\n")

	// Network Stats
	sb.WriteString(markdownBlock("## 🌐 Network Throughput"))
	sb.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("  Network In "), valueStyle.Render(fmt.Sprintf("%.2f KB/s", metrics.NetworkInRate))))
	sb.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("  Network Out"), valueStyle.Render(fmt.Sprintf("%.2f KB/s", metrics.NetworkOutRate))))
	sb.WriteString("\n")

	// Cluster & Node Status
	infra := m.report.InfraStatus
	nodeStatus := model.SAFE
	if metrics.NodesReady < metrics.NodesTotal {
		nodeStatus = model.CHECK
	}
	sb.WriteString(markdownBlock("## 🛡️ Infrastructure Status"))
	cards := []string{
		infoCard("NODES", statusStyle(nodeStatus).Render(fmt.Sprintf("%d/%d", metrics.NodesReady, metrics.NodesTotal))),
		infoCard("PODS", statusStyle(model.SAFE).Render(fmt.Sprintf("%d/%d", infra.PodsReady, infra.PodsTotal))),
		infoCard("RESTARTS", valueStyle.Render(fmt.Sprintf("%d", infra.Restarts))),
	}
	if infra.PodsReady < infra.PodsTotal {
		cards[1] = infoCard("PODS", statusStyle(model.RISK).Render(fmt.Sprintf("%d/%d", infra.PodsReady, infra.PodsTotal)))
	}
	sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, cards...))
	sb.WriteString("\n\n")

	// Top Namespaces
	if len(metrics.TopNamespaces) > 0 {
		sb.WriteString(markdownBlock("## 📊 Top Heavy Namespaces"))
		headers := []string{"Namespace", "CPU (%)", "Memory (MB)"}
		rows := [][]string{}
		for _, ns := range metrics.TopNamespaces {
			rows = append(rows, []string{
				ns.Name,
				fmt.Sprintf("%.2f", ns.CPUUsage),
				fmt.Sprintf("%.0f", ns.MemoryUsage),
			})
		}
		sb.WriteString(renderTable(headers, rows))
		sb.WriteString("\n")
	}

	return sb.String()
}

func (m tuiModel) renderServiceDrilldown() string {
	var sb strings.Builder

	if !m.showServiceDrilldownDetail {
		// List View: Overview of all services
		sb.WriteString(titleStyle.Render("Service Performance Overview"))
		sb.WriteString("\n")
		sb.WriteString(disclaimerStyle.Render("Status and Golden Signals for all discovered services."))
		sb.WriteString("\n\n")

		if len(m.report.AllServices) == 0 {
			sb.WriteString(dimStyle.Render("  No services detected. Ensure Prometheus is reachable and metrics have service labels."))
			sb.WriteString("\n")
			return sb.String()
		}

		headers := []string{"  Service", "Status", "RPS", "Error %", "P95 (ms)"}
		rows := [][]string{}
		
		// Render ALL services; m.scrollY in View() handles the viewport slicing
		for i, svc := range m.report.AllServices {
			prefix := "  "
			name := svc.Name
			if i == m.serviceDrilldownIndex {
				prefix = "> "
				name = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render(svc.Name)
			}
			rows = append(rows, []string{
				prefix + name,
				statusStyle(svc.Status).Render(string(svc.Status)),
				fmt.Sprintf("%.2f", svc.RPS),
				fmt.Sprintf("%.2f%%", svc.ErrorRate),
				fmt.Sprintf("%.0f", svc.P95*1000), // Convert to ms
			})
		}
		sb.WriteString(renderTable(headers, rows))
		sb.WriteString("\n\n")
		sb.WriteString(dimStyle.Render("Press ↑/↓ to navigate, Enter to drill down, Esc to return."))
		return sb.String()
	}

	// Detail View: Specific service drilldown
	serviceName := m.selectedDrilldownService
	if serviceName == "" && m.report.ServiceDrilldown != nil {
		serviceName = m.report.ServiceDrilldown.Service
	}

	sb.WriteString(titleStyle.Render(fmt.Sprintf("Service Drilldown: %s", serviceName)))
	sb.WriteString("\n")
	sb.WriteString(disclaimerStyle.Render("Golden signals, SLOs, and recent deployment activity."))
	sb.WriteString("\n\n")

	wantSvc := strings.TrimSpace(serviceName)
	gotSvc := ""
	if m.report.ServiceDrilldown != nil {
		gotSvc = strings.TrimSpace(m.report.ServiceDrilldown.Service)
	}

	if m.report.ServiceDrilldown == nil || !strings.EqualFold(gotSvc, wantSvc) {
		// If we don't have the data yet, show a loading/refresh message
		config.DebugLog("renderServiceDrilldown: MISMATCH - want=%q, got=%q, isNil=%v", 
			wantSvc, gotSvc, m.report.ServiceDrilldown == nil)
		sb.WriteString(dimStyle.Render(fmt.Sprintf("  Collecting detailed metrics for %s...", serviceName)))
		sb.WriteString("\n")
		return sb.String()
	}
	config.DebugLog("renderServiceDrilldown: MATCH - rendering for %s", wantSvc)

	d := m.report.ServiceDrilldown

	// Golden Signals
	sb.WriteString(markdownBlock("## ✨ Golden Signals"))
	rows := [][]string{
		{"Throughput", fmt.Sprintf("%.2f req/sec", d.GoldenSignals.RequestsPerSecond)},
		{"Error Rate", fmt.Sprintf("%.2f%%", d.GoldenSignals.ErrorRate)},
		{"Latency P95", fmt.Sprintf("%.2f ms", d.GoldenSignals.LatencyP95*1000)},
		{"Baseline P95", fmt.Sprintf("%.2f ms", d.GoldenSignals.LatencyBaseline*1000)},
	}
	sb.WriteString(renderTable([]string{"Signal", "Value"}, rows))
	sb.WriteString("\n")

	// SLOs
	if len(d.SLOs) > 0 {
		sb.WriteString(markdownBlock("## 🎯 Service Level Objectives (SLOs)"))
		sloHeaders := []string{"SLO Name", "Target", "Current", "Status"}
		sloRows := [][]string{}
		for _, s := range d.SLOs {
			sloRows = append(sloRows, []string{
				s.Name,
				fmt.Sprintf("%.2f%%", s.Target),
				fmt.Sprintf("%.2f%%", s.Current),
				string(s.Status),
			})
		}
		sb.WriteString(renderTable(sloHeaders, sloRows))
		sb.WriteString("\n")
	}

	// Traces (Top Spans)
	sb.WriteString(markdownBlock("## 🔍 Traces"))
	if len(d.TraceLinks) > 0 {
		sb.WriteString(dimStyle.Render("  Window: 48h\n"))
		for _, trace := range d.TraceLinks {
			sb.WriteString("  " + trace + "\n")
		}
	} else {
		sb.WriteString(dimStyle.Render("  No trace span analysis available for this period.\n"))
	}
	sb.WriteString("\n")

	// Recent Deployments
	if len(d.RecentDeployments) > 0 {
		sb.WriteString(markdownBlock("## 🚀 Recent Deployments"))
		for _, dep := range d.RecentDeployments {
			sb.WriteString(fmt.Sprintf("  • %s: %s (%s)\n", 
				dep.Time.Format("2006-01-02 15:04"), 
				keyStyle.Render(dep.Version), 
				dep.Status))
		}
		sb.WriteString("\n")
	}

	// Recent Errors (Loki)
	sb.WriteString(markdownBlock("## 📜 Recent Errors (Loki)"))
	if len(d.CorrelatedLogs) > 0 {
		for _, log := range d.CorrelatedLogs {
			sb.WriteString(dimStyle.Render("  • "+log) + "\n")
		}
	} else {
		sb.WriteString(dimStyle.Render("  No critical errors found in the last 48 hours.\n"))
	}
	sb.WriteString("\n")

	return sb.String()
}

func renderGauge(label string, percent float64) string {
	width := 30
	filled := int(float64(width) * (percent / 100.0))
	if filled > width { filled = width }
	if filled < 0 { filled = 0 }

	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	
	color := green
	if percent > 85 {
		color = red
	} else if percent > 70 {
		color = yellow
	}

	return fmt.Sprintf("  %-15s [%s] %.1f%%\n", label, color.Render(bar), percent)
}

func formatBytes(b float64) string {
	if b < 1024 {
		return fmt.Sprintf("%.0f B", b)
	}
	if b < 1024*1024 {
		return fmt.Sprintf("%.2f KB", b/1024)
	}
	if b < 1024*1024*1024 {
		return fmt.Sprintf("%.2f MB", b/(1024*1024))
	}
	return fmt.Sprintf("%.2f GB", b/(1024*1024*1024))
}

/* Demo functions moved to tui_demo_view.go */
