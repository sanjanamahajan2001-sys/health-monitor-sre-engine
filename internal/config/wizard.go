package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"health-monitor/internal/analyse/prometheus"
	"health-monitor/internal/analyse/loki"
	"health-monitor/internal/analyse/tracing"
	"health-monitor/internal/discovery"
	"health-monitor/pkg/model"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"math"
	"sort"
)

// WizardState represents the current step in the wizard
type WizardState int

const (
	StateProfileName WizardState = iota
	StateBackends
	StatePrometheusURL
	StatePrometheusAuth
	StateLokiURL
	StateLokiAuth
	StateGrafanaURL
	StateGrafanaAuth
	StateGrafanaDS   // Prometheus DS
	StateGrafanaLokiDS // Loki DS
	StateGrafanaTraceDS // Trace DS
	StateTempoURL
	StateTempoAuth
	StateDiscovery
	StateFlowGeneration
	StateAlertGeneration // NEW
	StateNotifySlackAsk     // Ask for Slack
	StateNotifySlackConfig
	StateNotifyPagerDutyAsk // Ask for PagerDuty
	StateNotifyPagerDutyConfig
	StateStdKubeConfig
	StateStdKubeContext
	StateStdAWSConfigPath
	StateStdAWSProfile
	StateStdAWSRegion
	StateElasticURL
	StateElasticAuth
	StateElasticIndex
	StateElasticFields
	StateConfigTeamSize       WizardState = 100 // Avoid conflict with wizard_teams.go
	StateConfigOpsHours       WizardState = 101
	StateConfigTargetToil     WizardState = 102
	StateConfigHourlyCost     WizardState = 103 // NEW
	StateInstantTest          WizardState = 106 // NEW
	StateSummary              WizardState = 104
	StateDone                 WizardState = 105
)


type WizardModel struct {
	state          WizardState
	profileName    string
	backends       map[string]bool
	config         Config
	textInput      textinput.Model
	err            error
	discovering    bool
	testingConn    bool
	testResult     string
	services       []string
	discoverySummary discoverySummaryMsg
	discoveryStep    string
	program          *tea.Program
	serviceMetadata map[string]model.ServiceMetadata
	generateFlows  bool
	generateAlerts bool // NEW
	awsConfigPath  string
	awsProfile     string
	slackWebhook   string
	pagerDutyKey   string
	instantTestResults []model.ServiceOverview
	quitting       bool
}

func NewWizardModel() *WizardModel {
	ti := textinput.New()
	ti.Placeholder = "default"
	ti.Focus()
	ti.CharLimit = 156
	ti.Width = 40

	return &WizardModel{
		state:       StateProfileName,
		backends:    make(map[string]bool),
		config:      Default(),
		textInput:   ti,
		profileName: "default",
		serviceMetadata: make(map[string]model.ServiceMetadata),
	}
}

func (m *WizardModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m *WizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			m.quitting = true
			return m, tea.Quit
		case tea.KeyEnter:
			if m.state == StateInstantTest {
				m.state = StateFlowGeneration
				return m, nil
			}
			return m.nextStep()
		case 't', 'T':
			if m.state == StateFlowGeneration && !m.discovering {
				m.state = StateInstantTest
				m.testingConn = true
				m.testResult = "🧪 Running instant metrics probe..."
				return m, m.runInstantTest()
			}
		}

	case discoverySummaryMsg:
		if m.config.EKS.Enabled {
			m.testResult = fmt.Sprintf("🔍 Found %d K8s services, %d distinct apps identified. (%d skipped system services)", msg.TotalK8s, msg.TotalIdentified, msg.Skipped)
		} else {
			m.testResult = fmt.Sprintf("🔍 Identified %d application apps from metrics.", msg.TotalIdentified)
		}
		return m, nil

	case discoveryStepMsg:
		m.discoveryStep = string(msg)
		return m, nil

	case textResultMsg:
		m.testingConn = false
		m.testResult = string(msg)
		return m, nil

	case servicesResultMsg:
		m.discovering = false
		m.services = msg.Services
		m.serviceMetadata = msg.Metadata
		m.config = msg.Config 
		
		summary := msg.Summary
		if len(m.services) == 0 {
			if msg.Error != "" {
				m.testResult = fmt.Sprintf("❌ Discovery error: %s", msg.Error)
			} else {
				m.testResult = "⚠️  No services discovered. Check your Kubernetes context, permissions, or Prometheus labels."
			}
		} else {
			if m.config.EKS.Enabled {
				m.testResult = fmt.Sprintf("🔍 Found %d K8s services, identified %d application apps. (%d system services hidden)", 
					summary.TotalK8s, summary.TotalIdentified, summary.Skipped)
			} else {
				m.testResult = fmt.Sprintf("🔍 Identified %d application apps from metrics.", summary.TotalIdentified)
			}
		}
		
		m.state = StateFlowGeneration
		m.discovering = true // Keep displaying "please wait" until deep discovery is done
		m.textInput.Reset()
		m.textInput.Placeholder = "y/n"
		return m, discoverDeepMetadata(m.config, m.services)

	case deepDiscoveryResultMsg:
		m.serviceMetadata = msg
		m.discovering = false
		m.state = StateFlowGeneration
		m.textInput.Placeholder = "y/n"
		m.textInput.Reset()
		return m, nil

	case globalMetadataResultMsg:
		m.config = Config(msg)
		// After global metadata, trigger service discovery
		return m, discoverServices(m.program, m.config, m.awsConfigPath, m.awsProfile)

	case errMsg:
		m.err = msg
		m.testingConn = false
		return m, nil
	case instantTestMsg:
		m.testingConn = false
		m.instantTestResults = msg
		if len(msg) == 0 {
			m.testResult = "⚠️ No metrics found for any service during instant test."
		} else {
			m.testResult = fmt.Sprintf("✅ Instant test complete. Probed %d services successfully.", len(msg))
		}
		return m, nil
	}

	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

type textResultMsg string
type instantTestMsg []model.ServiceOverview
type errMsg error

func (m *WizardModel) nextStep() (tea.Model, tea.Cmd) {
	switch m.state {
	case StateProfileName:
		name := strings.TrimSpace(m.textInput.Value())
		if name != "" {
			m.profileName = name
		}
		m.state = StateBackends
		m.textInput.Reset()
		m.textInput.Placeholder = "p, l, g, t, j (comma separated)"
		return m, nil

	case StateBackends:
		val := strings.ToLower(m.textInput.Value())
		if strings.Contains(val, "p") { m.backends["prometheus"] = true }
		if strings.Contains(val, "l") { m.backends["loki"] = true }
		if strings.Contains(val, "g") { m.backends["grafana"] = true }
		if strings.Contains(val, "t") { 
			m.backends["tempo"] = true
			m.config.TraceBackend = "tempo"
		}
		if strings.Contains(val, "j") {
			m.backends["jaeger"] = true
			m.config.TraceBackend = "jaeger"
		}
		
		return m.goToNextBackendStep()

	case StateStdKubeConfig:
		val := m.textInput.Value()
		if val == "" {
			val = "~/.kube/config"
		}
		m.config.EKS.KubeConfig = ExpandTilde(val)
		m.state = StateStdKubeContext
		m.textInput.Reset()
		m.textInput.Placeholder = "e.g., dev-cluster or arn:aws:eks:region:account:cluster/name"
		return m, nil

	case StateStdKubeContext:
		m.config.EKS.KubeContext = m.textInput.Value()
		m.textInput.Reset()
		// Always ask for AWS profile if EKS is enabled, regardless of Provider being "eks" or "kubernetes"
		if m.config.EKS.Enabled {
			m.state = StateStdAWSConfigPath
			m.textInput.Placeholder = "e.g., ~/.aws/config"
		} else {
			return m.goToNextBackendStep()
		}
		return m, nil

	case StateStdAWSConfigPath:
		val := m.textInput.Value()
		if val == "" {
			val = "~/.aws/config"
		}
		m.awsConfigPath = ExpandTilde(val)
		m.state = StateStdAWSProfile
		m.textInput.Reset()
		m.textInput.Placeholder = "e.g., default or production"
		return m, nil

	case StateStdAWSProfile:
		val := m.textInput.Value()
		if val == "" {
			val = "default"
		}
		m.awsProfile = val
		m.config.EKS.AWSProfile = val
		m.state = StateStdAWSRegion
		m.textInput.Placeholder = "e.g., us-east-1"
		m.textInput.Reset()
		return m, nil

	case StateStdAWSRegion:
		m.config.EKS.Region = m.textInput.Value()
		return m.goToNextBackendStep()

	case StatePrometheusURL:
		m.config.PrometheusURL = strings.TrimSpace(m.textInput.Value())
		m.state = StatePrometheusAuth
		m.textInput.Reset()
		m.textInput.Placeholder = "token or user:pass"
		return m, nil

	case StatePrometheusAuth:
		auth := m.textInput.Value()
		if strings.TrimSpace(auth) != "" || m.config.PrometheusToken == "" {
			if strings.Contains(auth, ":") {
				parts := strings.SplitN(auth, ":", 2)
				m.config.PrometheusUser = parts[0]
				m.config.PrometheusPass = parts[1]
			} else {
				m.config.PrometheusToken = auth
			}
		}

		// If we haven't tested with these creds yet, do it now
		if !m.testingConn && !strings.Contains(m.testResult, "Prometheus:") {
			m.testingConn = true
			m.testResult = "Testing Prometheus with credentials..."
			m.textInput.Reset()
			m.textInput.Placeholder = "Enter to proceed"
			return m, testPrometheus(m.config)
		}

		return m.goToNextBackendStep()

	case StateLokiURL:
		m.config.LokiURL = strings.TrimSpace(m.textInput.Value())
		m.state = StateLokiAuth
		m.textInput.Reset()
		m.textInput.Placeholder = "token or user:pass"
		return m, nil

	case StateLokiAuth:
		val := m.textInput.Value()
		if strings.Contains(val, ":") {
			parts := strings.SplitN(val, ":", 2)
			m.config.LokiUser = parts[0]
			m.config.LokiPass = parts[1]
			m.config.LokiToken = ""
		} else {
			m.config.LokiToken = val
			m.config.LokiUser = ""
			m.config.LokiPass = ""
		}
		// Test Loki with creds
		if !m.testingConn && !strings.Contains(m.testResult, "Loki:") {
			m.testingConn = true
			m.testResult = "Testing Loki with credentials..."
			m.textInput.Reset()
			m.textInput.Placeholder = "Enter to proceed"
			return m, testLoki(m.config)
		}
		return m.goToNextBackendStep()

	case StateElasticURL:
		m.config.ElasticURL = m.textInput.Value()
		if m.config.ElasticURL == "" {
			return m.goToNextBackendStep()
		}
		m.state = StateElasticAuth
		m.textInput.Reset()
		m.textInput.Placeholder = "Auth (token or user:pass, leave empty if none)"
		return m, nil

	case StateElasticAuth:
		val := m.textInput.Value()
		if strings.Contains(val, ":") {
			parts := strings.SplitN(val, ":", 2)
			m.config.ElasticUser = parts[0]
			m.config.ElasticPass = parts[1]
			m.config.ElasticToken = ""
		} else {
			m.config.ElasticToken = val
			m.config.ElasticUser = ""
			m.config.ElasticPass = ""
		}
		m.state = StateElasticIndex
		m.textInput.Reset()
		m.textInput.Placeholder = "Index name (e.g., logs-*, leave empty for *)"
		return m, nil

	case StateElasticIndex:
		m.config.ElasticIndex = m.textInput.Value()
		m.state = StateElasticFields
		m.textInput.Reset()
		m.textInput.Placeholder = "Format: serviceField:errorField:timeField (optional)"
		return m, nil

	case StateElasticFields:
		val := m.textInput.Value()
		if val != "" {
			parts := strings.Split(val, ":")
			if len(parts) >= 1 && parts[0] != "" {
				m.config.ElasticServiceField = parts[0]
			}
			if len(parts) >= 2 && parts[1] != "" {
				m.config.ElasticErrorField = parts[1]
			}
			if len(parts) >= 3 && parts[2] != "" {
				m.config.ElasticTimeField = parts[2]
			}
		}
		return m.goToNextBackendStep()

	case StateGrafanaURL:
		m.config.GrafanaURL = m.textInput.Value()
		m.state = StateGrafanaAuth
		m.textInput.Reset()
		return m, nil

	case StateGrafanaAuth:
		auth := m.textInput.Value()
		if strings.Contains(auth, ":") {
			parts := strings.SplitN(auth, ":", 2)
			m.config.GrafanaUser = parts[0]
			m.config.GrafanaPass = parts[1]
		} else {
			m.config.GrafanaToken = auth
		}
		m.state = StateGrafanaDS
		m.textInput.Reset()
		m.textInput.Placeholder = "Prometheus"
		return m, nil

	case StateGrafanaDS:
		m.config.GrafanaPromDataSource = m.textInput.Value()
		if m.config.GrafanaPromDataSource == "" {
			m.config.GrafanaPromDataSource = "Prometheus"
		}
		m.state = StateGrafanaLokiDS
		m.textInput.Reset()
		m.textInput.Placeholder = "Loki"
		return m, nil

	case StateGrafanaLokiDS:
		m.config.GrafanaLokiDataSource = m.textInput.Value()
		if m.config.GrafanaLokiDataSource == "" {
			m.config.GrafanaLokiDataSource = "Loki"
		}
		m.state = StateGrafanaTraceDS
		m.textInput.Reset()
		m.textInput.Placeholder = "Tempo"
		return m, nil

	case StateGrafanaTraceDS:
		m.config.GrafanaTraceDataSource = m.textInput.Value()
		if m.config.GrafanaTraceDataSource == "" {
			m.config.GrafanaTraceDataSource = "Tempo"
		}
		return m.goToNextBackendStep()

	case StateTempoURL:
		m.config.TraceURL = strings.TrimSpace(m.textInput.Value())
		m.state = StateTempoAuth
		m.textInput.Reset()
		m.textInput.Placeholder = "token or user:pass"
		return m, nil

	case StateTempoAuth:
		auth := m.textInput.Value()
		if strings.TrimSpace(auth) != "" || m.config.TraceToken == "" {
			if strings.Contains(auth, ":") {
				parts := strings.SplitN(auth, ":", 2)
				m.config.TraceUser = parts[0]
				m.config.TracePass = parts[1]
			} else {
				m.config.TraceToken = auth
			}
		}

		// Test Trace with creds
		backendName := "Tempo"
		if m.config.TraceBackend == "jaeger" {
			backendName = "Jaeger"
		}
		if !m.testingConn && !strings.Contains(m.testResult, backendName+":") {
			m.testingConn = true
			m.testResult = fmt.Sprintf("Testing %s with credentials...", backendName)
			m.textInput.Reset()
			m.textInput.Placeholder = "Enter to proceed"
			return m, testTrace(m.config)
		}

		return m.goToNextBackendStep()

	case StateDiscovery:
		// PROGRESS STATE: handled in the message listeners
		return m, nil

	case StateFlowGeneration:
		val := strings.ToLower(m.textInput.Value())
		if val == "n" || val == "no" {
			m.generateFlows = false
		} else {
			m.generateFlows = true
		}
		m.testResult = "" // Clear discovery status when moving forward
		m.state = StateAlertGeneration
		m.textInput.Reset()
		m.textInput.Placeholder = "y/n"
		return m, nil

	case StateAlertGeneration:
		val := strings.ToLower(m.textInput.Value())
		if val == "n" || val == "no" {
			m.generateAlerts = false
		} else {
			m.generateAlerts = true
		}
		m.testResult = "" // Clear discovery status when moving forward
		m.state = StateNotifySlackAsk
		m.textInput.Reset()
		m.textInput.Placeholder = "y/n"
		return m, nil

	case StateNotifySlackAsk:
		val := strings.ToLower(m.textInput.Value())
		if val == "y" || val == "yes" {
			m.state = StateNotifySlackConfig
			m.textInput.Placeholder = "https://hooks.slack.com/..."
		} else {
			m.state = StateNotifyPagerDutyAsk
			m.textInput.Placeholder = "y/n"
		}
		m.textInput.Reset()
		return m, nil

	case StateNotifySlackConfig:
		m.slackWebhook = strings.TrimSpace(m.textInput.Value())
		if m.slackWebhook != "" {
			m.config.Notifications.Enabled = true
			m.config.Notifications.Slack.Enabled = true
		}
		m.state = StateNotifyPagerDutyAsk
		m.textInput.Placeholder = "y/n"
		m.textInput.Reset()
		return m, nil

	case StateNotifyPagerDutyAsk:
		val := strings.ToLower(m.textInput.Value())
		if val == "y" || val == "yes" {
			m.state = StateNotifyPagerDutyConfig
			m.textInput.Placeholder = "Routing Key"
		} else {
			m.state = StateConfigTeamSize
		}
		m.textInput.Reset()
		return m, nil

	case StateNotifyPagerDutyConfig:
		m.pagerDutyKey = strings.TrimSpace(m.textInput.Value())
		if m.pagerDutyKey != "" {
			m.config.Notifications.Enabled = true
			m.config.Notifications.PagerDuty.Enabled = true
		}
		m.state = StateConfigTeamSize
		m.textInput.Placeholder = "e.g., 5"
		m.textInput.Reset()
		return m, nil

	case StateConfigTeamSize:
		val := m.textInput.Value()
		if val == "" {
			val = "5"
		}
		var size int
		fmt.Sscanf(val, "%d", &size)
		if size <= 0 { size = 5 }
		m.config.TeamCapacity.TeamSize = size
		m.state = StateConfigOpsHours
		m.textInput.Placeholder = "e.g., 40"
		m.textInput.Reset()
		return m, nil

	case StateConfigOpsHours:
		val := m.textInput.Value()
		if val == "" {
			val = "40"
		}
		var hours int
		fmt.Sscanf(val, "%d", &hours)
		if hours <= 0 { hours = 40 }
		m.config.TeamCapacity.OpsHoursPerWeekPerPerson = hours
		m.state = StateConfigTargetToil
		m.textInput.Placeholder = "e.g., 50"
		m.textInput.Reset()
		return m, nil

	case StateConfigTargetToil:
		val := m.textInput.Value()
		if val == "" {
			val = "50"
		}
		var target float64
		fmt.Sscanf(val, "%f", &target)
		if target <= 0 { target = 50.0 }
		m.config.TeamCapacity.TargetToilPercentage = target
		m.state = StateConfigHourlyCost
		m.textInput.Placeholder = "e.g., 100"
		m.textInput.Reset()
		return m, nil

	case StateConfigHourlyCost:
		val := m.textInput.Value()
		if val == "" {
			val = "100"
		}
		var cost float64
		fmt.Sscanf(val, "%f", &cost)
		if cost <= 0 { cost = 100.0 }
		m.config.TeamCapacity.AverageHourlyCost = cost
		m.state = StateSummary
		m.textInput.Reset()
		return m, nil

	case StateSummary:
		m.state = StateDone
		return m, tea.Quit
	}

	return m, nil
}

type globalMetadataResultMsg Config

func discoverGlobalMetadata(cfg Config) tea.Cmd {
	return func() tea.Msg {
		// 1. Discover Prometheus Service Label
		if cfg.PrometheusURL != "" {
			client := prometheus.Client{
				BaseURL: cfg.PrometheusURL,
				Token:   cfg.PrometheusToken,
				User:    cfg.PrometheusUser,
				Pass:    cfg.PrometheusPass,
				Timeout: 10 * time.Second,
			}
			if label, err := client.DiscoverServiceLabel(); err == nil {
				cfg.ServiceLabel = label
				cfg.PrometheusServiceLabel = label
			}
		}

		// 2. Discover Loki Service Label
		if cfg.LokiURL != "" {
			client := loki.Client{
				BaseURL: cfg.LokiURL,
				Token:   cfg.LokiToken,
				User:    cfg.LokiUser,
				Pass:    cfg.LokiPass,
				Timeout: 10 * time.Second,
			}
			if label, err := client.DiscoverServiceLabel(); err == nil {
				// Append rather than overwrite to preserve defaults like 'service'
				existing := strings.Split(cfg.LokiServiceLabel, ",")
				discovered := strings.Split(label, ",")
				
				labelMap := make(map[string]bool)
				for _, l := range existing {
					l = strings.TrimSpace(l)
					if l != "" {
						labelMap[l] = true
					}
				}
				for _, l := range discovered {
					l = strings.TrimSpace(l)
					if l != "" {
						labelMap[l] = true
					}
				}
				
				var merged []string
				for l := range labelMap {
					merged = append(merged, l)
				}
				cfg.LokiServiceLabel = strings.Join(merged, ",")
			}
			cfg.LokiWindow = "1h" // Data-driven default
		}

		// 3. Tempo Service Map
		if cfg.TraceURL != "" {
			// Usually service:service_name or similar
			// We'll set a standard default that works for most OTel setups
			if cfg.ServiceLabel != "" {
				cfg.TraceServiceMap = fmt.Sprintf("service:%s", cfg.ServiceLabel)
			} else {
				cfg.TraceServiceMap = "service:service_name"
			}
			cfg.TraceWindow = "15m"
		}

		return globalMetadataResultMsg(cfg)
	}
}

func (m *WizardModel) goToNextBackendStep() (tea.Model, tea.Cmd) {
	m.textInput.Reset()
	m.testResult = "" // CLEAR FOR NEXT BACKEND OR STEP
	
	// Step 0: Infrastructure configuration (if enabled)
	if m.config.EKS.Enabled {
		if m.config.EKS.KubeConfig == "" {
			m.state = StateStdKubeConfig
			m.textInput.Placeholder = "e.g., ~/.kube/config"
			return m, nil
		}
		if m.config.EKS.KubeContext == "" {
			m.state = StateStdKubeContext
			m.textInput.Placeholder = "e.g., arn:aws:eks:..."
			return m, nil
		}
		if m.config.Provider == "eks" || m.config.EKS.Enabled {
			if m.awsConfigPath == "" {
				m.state = StateStdAWSConfigPath
				m.textInput.Placeholder = "e.g., ~/.aws/config"
				return m, nil
			}
			if m.config.EKS.AWSProfile == "" {
				m.state = StateStdAWSProfile
				m.textInput.Placeholder = "e.g., default or production-account"
				return m, nil
			}
			if m.config.EKS.Region == "" {
				m.state = StateStdAWSRegion
				m.textInput.Placeholder = "e.g., us-east-1"
				return m, nil
			}
		}
	}

	if m.backends["prometheus"] && m.config.PrometheusURL == "" {
		m.state = StatePrometheusURL
		m.textInput.Placeholder = "http://localhost:9090"
		return m, nil
	}
	if m.backends["loki"] && (m.config.LokiURL == "" || m.state == StateLokiAuth) {
		if m.state != StateLokiAuth && m.state != StateLokiURL {
			m.state = StateLokiURL
			m.textInput.Reset()
			m.textInput.Placeholder = "http://loki:3100"
			return m, nil
		}
	}

	if m.backends["elastic"] && (m.config.ElasticURL == "" || m.state == StateElasticFields) {
		if m.state != StateElasticURL && m.state != StateElasticAuth && m.state != StateElasticIndex && m.state != StateElasticFields {
			m.state = StateElasticURL
			m.textInput.Reset()
			m.textInput.Placeholder = "http://elasticsearch:9200"
			return m, nil
		}
	}

	if m.backends["grafana"] && (m.config.GrafanaURL == "" || m.state == StateGrafanaAuth) {
		m.state = StateGrafanaURL
		m.textInput.Placeholder = "http://localhost:3000"
		return m, nil
	}
	if (m.backends["tempo"] || m.backends["jaeger"]) && m.config.TraceURL == "" {
		m.state = StateTempoURL
		placeholder := "http://localhost:3200"
		if m.backends["jaeger"] {
			placeholder = "http://localhost:16686"
		}
		m.textInput.Placeholder = placeholder
		return m, nil
	}
	
	// After all backends, start global discovery
	m.state = StateDiscovery
	m.discovering = true
	m.testResult = ""
	return m, discoverGlobalMetadata(m.config)
}

func (m *WizardModel) View() string {
	if m.quitting {
		return "Setup cancelled.\n"
	}

	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
	promptStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("86"))

	var s string
	s += headerStyle.Render("🚀 Health Monitor Setup Wizard") + "\n\n"

	switch m.state {
	case StateProfileName:
		s += promptStyle.Render("Enter profile name:") + "\n"
	case StateBackends:
		s += promptStyle.Render("Which backends do you use? (p)rometheus, (l)oki, (g)rafana, (t)empo, (j)aeger, (e)lastic:") + "\n"
	case StatePrometheusURL:
		s += promptStyle.Render("Prometheus URL:") + "\n"
	case StatePrometheusAuth:
		s += promptStyle.Render("Prometheus Auth (token or user:pass):") + "\n"
	case StateLokiURL:
		s += promptStyle.Render("Loki URL:") + "\n"
	case StateLokiAuth:
		s += promptStyle.Render("Loki Auth (token or user:pass):") + "\n"
	case StateElasticURL:
		s += promptStyle.Render("Elasticsearch URL:") + "\n"
	case StateElasticAuth:
		s += promptStyle.Render("Elasticsearch Auth (token or user:pass):") + "\n"
	case StateElasticIndex:
		s += promptStyle.Render("Elasticsearch Index (e.g., logs-*, leave empty for *):") + "\n"
	case StateElasticFields:
		s += promptStyle.Render("Elasticsearch Fields (Format: serviceField:errorField:timeField, optional):") + "\n"
	case StateGrafanaURL:
		s += promptStyle.Render("Grafana URL:") + "\n"
	case StateGrafanaAuth:
		s += promptStyle.Render("Grafana Auth (token or user:pass):") + "\n"
	case StateGrafanaDS:
		s += promptStyle.Render("Prometheus Data Source Name in Grafana (default: Prometheus):") + "\n"
	case StateGrafanaLokiDS:
		s += promptStyle.Render("Loki Data Source Name in Grafana (default: Loki):") + "\n"
	case StateGrafanaTraceDS:
		s += promptStyle.Render("Tempo/Trace Data Source Name in Grafana (default: Tempo):") + "\n"
	case StateTempoURL:
		backend := "Trace Backend"
		if m.backends["jaeger"] {
			backend = "Jaeger"
		} else if m.backends["tempo"] {
			backend = "Tempo"
		}
		s += promptStyle.Render(fmt.Sprintf("%s URL:", backend)) + "\n"
	case StateTempoAuth:
		backend := "Trace"
		if m.backends["jaeger"] {
			backend = "Jaeger"
		} else if m.backends["tempo"] {
			backend = "Tempo"
		}
		s += promptStyle.Render(fmt.Sprintf("%s Auth (token or user:pass):", backend)) + "\n"
	case StateDiscovery:
		s += promptStyle.Render("🔍 Deeply discovering metadata and services...") + "\n"
		if m.discoveryStep != "" {
			s += lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Italic(true).Render(m.discoveryStep) + "\n"
		}
		if len(m.services) > 0 {
			s += fmt.Sprintf("\nDiscovered %d services: %s\n", len(m.services), strings.Join(m.services, ", "))
		}
	case StateStdKubeConfig:
		s += promptStyle.Render("Kubeconfig path (default: ~/.kube/config):") + "\n"
	case StateStdKubeContext:
		s += promptStyle.Render("Kubernetes Context name:") + "\n"
	case StateStdAWSConfigPath:
		s += promptStyle.Render("AWS Config file path (default: ~/.aws/config):") + "\n"
	case StateStdAWSProfile:
		s += promptStyle.Render("AWS Profile name (default: default):") + "\n"
	case StateStdAWSRegion:
		s += promptStyle.Render("AWS Region (e.g., us-east-1):") + "\n"
	case StateConfigTeamSize:
		s += promptStyle.Render("Team size (number of engineers, default: 5):") + "\n"
	case StateConfigOpsHours:
		s += promptStyle.Render("Ops/On-call hours per week per person (default: 40):") + "\n"
	case StateConfigTargetToil:
		s += promptStyle.Render("Target toil threshold percentage (default: 50%):") + "\n"
	case StateConfigHourlyCost:
		s += promptStyle.Render("Average hourly cost per engineer in USD (default: 100):") + "\n"
	case StateFlowGeneration:
		if m.discovering {
			s += promptStyle.Render("🔍 Deeply discovering metadata and services...") + "\n"
		} else {
			s += promptStyle.Render("Generate profile-specific flows for detected services? (y/n, default: y):") + "\n"
			if len(m.services) > 0 {
				s += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("86")).Bold(true).Render("Detected services:") + "\n"
				s += m.formatServiceList(m.services) + "\n"
			}
			s += lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("Saved in: /etc/health-monitor/flows.d/") + "\n"
		}
	case StateAlertGeneration:
		s += promptStyle.Render("Generate default alert rules for detected services? (y/n, default: y):") + "\n"
		s += lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("Saved in: /etc/health-monitor/alerts.d/") + "\n"
	case StateNotifySlackAsk:
		s += promptStyle.Render("Do you want to enable Slack notifications? (y/n, default: n):") + "\n"
	case StateNotifySlackConfig:
		s += promptStyle.Render("Enter Slack Webhook URL:") + "\n"
	case StateNotifyPagerDutyAsk:
		s += promptStyle.Render("Do you want to enable PagerDuty notifications? (y/n, default: n):") + "\n"
	case StateNotifyPagerDutyConfig:
		s += promptStyle.Render("Enter PagerDuty Routing Key:") + "\n"
	case StateInstantTest:
		s += promptStyle.Render("🧪 Instant Metrics Probe Results:") + "\n\n"
		if len(m.instantTestResults) == 0 {
			if m.testingConn {
				s += "Probing services, please wait...\n"
			} else {
				s += "No metrics found. This could mean the service labels or Prometheus URL are incorrect.\n"
			}
		} else {
			header := lipgloss.NewStyle().Foreground(lipgloss.Color("86")).Bold(true)
			s += fmt.Sprintf("%-25s %-10s %-10s %-10s\n", header.Render("Service"), header.Render("RPS"), header.Render("Error %"), header.Render("P95 (s)"))
			s += strings.Repeat("-", 60) + "\n"
			for i, res := range m.instantTestResults {
				if i >= 10 {
					s += fmt.Sprintf("... and %d more\n", len(m.instantTestResults)-10)
					break
				}
				statusColor := "42" // Green
				if res.Status == model.RISK { statusColor = "196" }
				if res.Status == model.CHECK { statusColor = "208" }
				
				style := lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor))
				s += style.Render(fmt.Sprintf("%-25s %-10.2f %-10.2f %-10.2f\n", res.Name, res.RPS, res.ErrorRate, res.P95))
			}
		}
		s += "\nPress Enter to continue setup."
		return s

	case StateSummary:
		s += headerStyle.Render("Configuration Summary:") + "\n"
		s += fmt.Sprintf("Profile: %s\n", m.profileName)
		s += fmt.Sprintf("Prometheus: %s\n", m.config.PrometheusURL)
		if m.config.LokiURL != "" { 
			s += fmt.Sprintf("Loki: %s\n", m.config.LokiURL)
			s += fmt.Sprintf("  ├─ Regex: %s\n", m.config.LokiErrorRegex)
			s += fmt.Sprintf("  ├─ Label: %s\n", m.config.LokiServiceLabel)
			s += fmt.Sprintf("  └─ Window: %s\n", m.config.LokiWindow)
		}
		if m.config.GrafanaURL != "" { 
			s += fmt.Sprintf("Grafana: %s\n", m.config.GrafanaURL)
			s += fmt.Sprintf("  ├─ Prom DS: %s\n", m.config.GrafanaPromDataSource)
			s += fmt.Sprintf("  ├─ Loki DS: %s\n", m.config.GrafanaLokiDataSource)
			s += fmt.Sprintf("  └─ Trace DS: %s\n", m.config.GrafanaTraceDataSource)
		}
		if m.config.TraceURL != "" { 
			s += fmt.Sprintf("Trace (%s): %s\n", m.config.TraceBackend, m.config.TraceURL)
			s += fmt.Sprintf("  ├─ Map: %s\n", m.config.TraceServiceMap)
			s += fmt.Sprintf("  └─ Window: %s\n", m.config.TraceWindow)
		}
		s += headerStyle.Render("\nTeam Capacity:") + "\n"
		s += fmt.Sprintf("  ├─ Team Size: %d\n", m.config.TeamCapacity.TeamSize)
		s += fmt.Sprintf("  ├─ Ops Hours/Week: %d\n", m.config.TeamCapacity.OpsHoursPerWeekPerPerson)
		s += fmt.Sprintf("  ├─ Target Toil: %.0f%%\n", m.config.TeamCapacity.TargetToilPercentage)
		s += fmt.Sprintf("  └─ Avg Hourly Cost: $%.0f\n", m.config.TeamCapacity.AverageHourlyCost)
		
		s += fmt.Sprintf("\nLatency Threshold: %.1fs\n", m.config.LatencyThresholdSeconds)
		if m.generateFlows {
			s += lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("✔ Will generate profile-specific flows in flows.d/") + "\n"
		}
		if m.generateAlerts {
			s += lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("✔ Will generate default alert rules in alerts.d/") + "\n"
		}
		if m.slackWebhook != "" {
			s += lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("✔ Slack notifications enabled") + "\n"
		}
		if m.pagerDutyKey != "" {
			s += lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("✔ PagerDuty notifications enabled") + "\n"
		}
		s += "\nPress Enter to save and exit."
	}

	if m.state != StateSummary && m.state != StateDone {
		if m.testResult != "" {
			statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Italic(true)
			if strings.Contains(m.testResult, "✅") {
				statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
			} else if strings.Contains(m.testResult, "❌") {
				statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
			}
			s += statusStyle.Render(m.testResult) + "\n"
		}
		
		if m.discovering {
			// Center the discovery message to avoid overlap
			s += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Render("Running background discovery engine, please wait...") + "\n"
		} else {
			s += "\n" + m.textInput.View() + "\n"
		}
	}

	s += "\n(esc to quit)\n"
	return s
}

func RunWizard() (string, Config, bool, bool, string, string, []string, map[string]model.ServiceMetadata, error) {
	return RunWizardWithPrefs(WizardPrefs{})
}

// WizardPrefs contains initial preferences for the wizard
type WizardPrefs struct {
	InfraType  string
	KubeConfig string
	Region     string
}

// RunWizardWithPrefs starts the wizard with initial preferences
type discoverySummaryMsg struct {
	TotalK8s      int
	TotalMetric   int
	TotalIdentified int
	Skipped       int
}

type deepDiscoveryResultMsg map[string]model.ServiceMetadata

func discoverDeepMetadata(cfg Config, services []string) tea.Cmd {
	return func() tea.Msg {
		if cfg.PrometheusURL == "" || len(services) == 0 {
			return deepDiscoveryResultMsg(make(map[string]model.ServiceMetadata))
		}

		DebugLog("Starting deep discovery for %d services against %s", len(services), cfg.PrometheusURL)

		client := prometheus.Client{
			BaseURL: cfg.PrometheusURL,
			Token:   cfg.PrometheusToken,
			User:    cfg.PrometheusUser,
			Pass:    cfg.PrometheusPass,
			Timeout: 10 * time.Second,
		}

		metadata := make(map[string]model.ServiceMetadata)
		results := make(chan struct {
			svc  string
			meta model.ServiceMetadata
		}, len(services))

		for _, svc := range services {
			go func(service string) {
				// Use the new adaptive discovery with explicit mapping support
				meta, err := client.DiscoverServiceMetrics(service, cfg.PrometheusServiceLabel, cfg.PrometheusMetricMap, cfg.PrometheusLabelMap)
				if err != nil {
					DebugLog("  Discovery failed for %s: %v", service, err)
				} else {
					DebugLog("  Discovered for %s: Req=%s, Lat=%s, ErrLbl=%s", service, meta.RequestMetric, meta.LatencyMetric, meta.ErrorLabel)
				}
				
				// Populate baseline (e.g. 1 hour for setup discovery speed)
				_ = client.PopulateBaseline(&meta, service, cfg.PrometheusServiceLabel, "1h")
				results <- struct {
					svc  string
					meta model.ServiceMetadata
				}{service, meta}
			}(svc)
		}

		for i := 0; i < len(services); i++ {
			res := <-results
			metadata[res.svc] = res.meta
		}

		DebugLog("Deep discovery completed")
		return deepDiscoveryResultMsg(metadata)
	}
}

func RunWizardWithPrefs(prefs WizardPrefs) (string, Config, bool, bool, string, string, []string, map[string]model.ServiceMetadata, error) {
	model := NewWizardModel()
	
	// Apply infrastructure preferences
	if prefs.InfraType != "" {
		model.config.Provider = prefs.InfraType
		if prefs.InfraType == "eks" || prefs.InfraType == "kubernetes" {
			model.config.EKS.Enabled = true
		}
	}
	if prefs.KubeConfig != "" {
		model.config.EKS.KubeConfig = prefs.KubeConfig
	}
	if prefs.Region != "" {
		model.config.EKS.Region = prefs.Region
	}
	
	p := tea.NewProgram(model)
	model.program = p
	tm, err := p.Run()
	if err != nil {
		return "", Config{}, false, false, "", "", nil, nil, err
	}

	finalModel := tm.(*WizardModel)
	if finalModel.quitting {
		return "", Config{}, false, false, "", "", nil, nil, nil
	}

	return finalModel.profileName, finalModel.config, finalModel.generateFlows, finalModel.generateAlerts, finalModel.slackWebhook, finalModel.pagerDutyKey, finalModel.services, finalModel.serviceMetadata, nil
}

type servicesResultMsg struct {
	Services []string
	Metadata map[string]model.ServiceMetadata
	Summary  discoverySummaryMsg
	Config   Config
	Error    string
}

type discoveryStepMsg string

func discoverServices(p *tea.Program, cfg Config, awsConfigPath, awsProfile string) tea.Cmd {
	return func() tea.Msg {
		if cfg.EKS.Enabled {
			if awsConfigPath != "" {
				os.Setenv("AWS_CONFIG_FILE", awsConfigPath)
				os.Setenv("AWS_SHARED_CREDENTIALS_FILE", strings.ReplaceAll(awsConfigPath, "config", "credentials"))
			}
			if awsProfile != "" {
				os.Setenv("AWS_PROFILE", awsProfile)
			}
		}

		client := prometheus.Client{
			BaseURL: cfg.PrometheusURL,
			Token:   cfg.PrometheusToken,
			User:    cfg.PrometheusUser,
			Pass:    cfg.PrometheusPass,
			Timeout: 10 * time.Second,
		}
		
		// Create universal discovery engine
		discovery := NewUniversalDiscovery(client, cfg)
		discovery.Progress = func(step string) {
			if p != nil {
				p.Send(discoveryStepMsg(step))
			}
		}
		
		// Execute discovery
		services, metadata, summary, updatedCfg, err := discovery.DiscoverAll()
		
		errMsg := ""
		if err != nil {
			errMsg = err.Error()
		}
		
		return servicesResultMsg{
			Services: services,
			Metadata: metadata,
			Summary:  summary,
			Config:   updatedCfg,
			Error:    errMsg,
		}
	}
}

// UniversalDiscovery handles production-ready service discovery
type UniversalDiscovery struct {
	client           prometheus.Client
	cfg              Config
	patterns         []MetricPattern
	discovered       map[string]ServiceMetrics
	testCache        map[string]bool
	Progress         func(string) // Callback for UI progress
	labelValuesCache map[string][]string
}

type ServiceMetrics struct {
	Service          string
	AvailableMetrics []string
	WorkingQueries   map[string]string
	InferredLabels   map[string]string // NEW: e.g., "container" -> "frontend", "namespace" -> "otel-demo"
	DetectedLabel    string            // NEW: The specific label that worked during probing (e.g. "container", "service_name")
	Labels           []string
	SkipReason       string
}

type MetricPattern struct {
	Name        string
	Prefix      string
	TestQuery   string
	Extractor   func(service string, metrics []string) []string
}

type DiscoveryReport struct {
	TotalServices     int
	GeneratedFlows    int
	SuccessfulServices []string
	SkippedServices   []SkippedService
}

type SkippedService struct {
	Service string
	Reason  string
}

func NewUniversalDiscovery(client prometheus.Client, cfg Config) *UniversalDiscovery {
	return &UniversalDiscovery{
		client:           client,
		cfg:              cfg,
		discovered:       make(map[string]ServiceMetrics),
		testCache:        make(map[string]bool),
		labelValuesCache: make(map[string][]string),
	}
}

func (ud *UniversalDiscovery) findMetricsWithPrefixes(metrics []string, prefixes []string) []string {
	var matches []string
	for _, m := range metrics {
		for _, p := range prefixes {
			if strings.HasPrefix(m, p) {
				matches = append(matches, m)
				break
			}
		}
	}
	return matches
}

func (ud *UniversalDiscovery) testQuery(query string) bool {
	if res, ok := ud.testCache[query]; ok {
		return res
	}
	result, err := ud.client.QueryVector(query)
	ok := err == nil && len(result) > 0
	ud.testCache[query] = ok
	return ok
}

func (ud *UniversalDiscovery) discoverActualLabel(service string) string {
	// Common labels that usually identify a service
	labelsToTry := []string{"service_name", "app", "service", "container", "job", ud.cfg.ServiceLabel}
	
	for _, label := range labelsToTry {
		if label == "" { continue }
		
		values, ok := ud.labelValuesCache[label]
		if !ok {
			// Fetch from Prometheus and cache for the duration of this discovery run
			var err error
			values, err = ud.client.LabelValues(label, 30*time.Minute)
			if err != nil {
				ud.labelValuesCache[label] = nil // Mark as empty to avoid retrying
				continue
			}
			ud.labelValuesCache[label] = values
		}
		
		for _, v := range values {
			if v == service {
				return label
			}
		}
	}
	return ""
}

func (ud *UniversalDiscovery) reportProgress(step string) {
	DebugLog("[DISCOVERY] %s", step)
	if ud.Progress != nil {
		ud.Progress(step)
	}
}

func (ud *UniversalDiscovery) DiscoverAll() ([]string, map[string]model.ServiceMetadata, discoverySummaryMsg, Config, error) {
	// Step 1: Discover Kubernetes services
	ud.reportProgress("🔍 Fetching Kubernetes services...")
	k8sServices, totalRaw, skipped, k8sErr := ud.discoverKubernetesServicesWithStats()
	
	var allServices []string
	summary := discoverySummaryMsg{
		TotalK8s:      totalRaw,
		Skipped:       skipped,
	}

	// NEW: Fetch live K8s metadata from Prometheus to build high-fidelity mapping
	ud.reportProgress("🔍 Resolving cluster metadata mapping...")
	metadataMap := ud.fetchKubernetesMetadata()

	if len(k8sServices) > 0 {
		allServices = k8sServices
		summary.TotalIdentified = len(allServices)
		ud.reportProgress(fmt.Sprintf("📊 Identified %d Kubernetes services. Starting deep metadata mapping...", len(allServices)))
	}
	
	// Fallback (Phase B/C Carryover): Always try metric-based discovery if K8s found nothing
	if len(allServices) == 0 {
		ud.reportProgress("🔍 Probing Prometheus for metrics (fallback)...")
		client := prometheus.Client{
			BaseURL: ud.cfg.PrometheusURL,
			Token:   ud.cfg.PrometheusToken,
			User:    ud.cfg.PrometheusUser,
			Pass:    ud.cfg.PrometheusPass,
			Timeout: 10 * time.Second,
		}
		
		metricServices := discoverByMetricNames(client, ud.cfg)
		if len(metricServices) == 0 {
			metricServices = discoverByServiceLabel(client, ud.cfg)
		}
		
		if len(metricServices) > 0 {
			allServices = metricServices
			summary.TotalIdentified = len(allServices)
			ud.reportProgress(fmt.Sprintf("📊 Identified %d services from Prometheus metrics. Starting probing...", len(allServices)))
		}
	}
	
	// Only return error if BOTH discovery methods failed to find anything AND we have an error
	if len(allServices) == 0 && k8sErr != nil {
		return nil, nil, summary, ud.cfg, k8sErr
	}
	
	// Step 2: Detect metric patterns
	ud.reportProgress("🔍 Analyzing metric patterns...")
	ud.patterns = ud.detectPatterns()
	
	// Step 3: Discover metrics for each service
	metadata := make(map[string]model.ServiceMetadata)
	for i, service := range allServices {
		ud.reportProgress(fmt.Sprintf("🔍 Probing service %d/%d: %s", i+1, len(allServices), service))
		// Attempt to get metadata mapping for this service (Phase A)
		svcMeta := ud.getMetadataForService(metadataMap, service)
		metrics := ud.discoverServiceMetricsWithMetadata(service, svcMeta)
		
		ud.discovered[service] = metrics
		metadata[service] = ud.generateServiceMetadata(metrics)
	}
	
	ud.reportProgress("✅ Discovery finalized.")
	
	// Final schema check (Phase C): if any service found OTel, mark the whole config as OTel
	hasOTel := false
	for _, m := range metadata {
		if m.RequestMetric == "spanmetrics_calls_total" {
			hasOTel = true
			break
		}
	}
	if hasOTel {
		ud.cfg.MetricSchema = "otel"
		// Default OTel labels for logs and traces if not already set
		if ud.cfg.TraceServiceTag == "" || ud.cfg.TraceServiceTag == "service" {
			ud.cfg.TraceServiceTag = "service.name"
		}
	}

	return allServices, metadata, summary, ud.cfg, nil
}

func (ud *UniversalDiscovery) fetchKubernetesMetadata() map[string]map[string]string {
	metadata := make(map[string]map[string]string)
	
	// Query kube_pod_container_info to get mapping of pod/container/namespace
	result, err := ud.client.QueryVector("kube_pod_container_info")
	if err != nil {
		return metadata
	}
	
	for _, sample := range result {
		container := string(sample.Metric["container"])
		pod := string(sample.Metric["pod"])
		namespace := string(sample.Metric["namespace"])
		
		meta := map[string]string{
			"namespace": namespace,
			"pod":       pod,
			"container": container,
		}

		if container != "" {
			metadata[container] = meta
		}
		if pod != "" {
			metadata[pod] = meta
		}
	}
	return metadata
}

func (ud *UniversalDiscovery) getMetadataForService(mapping map[string]map[string]string, service string) map[string]string {
	// Look for direct match
	if meta, ok := mapping[service]; ok {
		return meta
	}
	
	// Look for partial match (pod/container often has prefix or suffix)
	for key, meta := range mapping {
		if strings.Contains(key, service) || strings.Contains(service, key) {
			return meta
		}
	}
	return nil
}

func (ud *UniversalDiscovery) discoverBySpecificLabel(service string, label string) []string {
	query := fmt.Sprintf("group by (__name__) ({%s=%q})", label, service)
	result, err := ud.client.QueryVector(query)
	if err != nil {
		return nil
	}
	
	var metrics []string
	for _, sample := range result {
		if name, ok := sample.Metric["__name__"]; ok {
			nameStr := string(name)
			if isSystemMetric(nameStr) {
				continue
			}
			metrics = append(metrics, nameStr)
		}
	}
	return metrics
}

func (ud *UniversalDiscovery) discoverServiceMetricsWithMetadata(service string, metadata map[string]string) ServiceMetrics {
	// Strategy 1: Data-First Strategy (Highest Fidelity)
	// Search for service name in common labels first
	if foundLabel := ud.discoverActualLabel(service); foundLabel != "" {
		metricsList := ud.discoverBySpecificLabel(service, foundLabel)
		if len(metricsList) > 0 {
			workingLabels := ud.testMetricsWork(metricsList)
			if len(workingLabels) > 0 {
				return ServiceMetrics{
					Service:          service,
					AvailableMetrics: workingLabels,
					DetectedLabel:    foundLabel,
					WorkingQueries:   ud.generateWorkingQueries(service, workingLabels, foundLabel),
					InferredLabels:   metadata,
				}
			}
		}
	}

	// Strategy 2: Targeted Metadata Strategy (Phase A)
	if metadata != nil {
		metrics := ServiceMetrics{
			Service:        service,
			WorkingQueries: make(map[string]string),
			InferredLabels: metadata,
		}
		
		allMetrics := ud.getAllMetricNames()
		var serviceMetrics []string
		
		targetLabel := ""
		targetValue := ""
		
		if v, ok := metadata["container"]; ok && v != "" {
			targetLabel = "container"
			targetValue = v
		} else if v, ok := metadata["pod"]; ok && v != "" {
			targetLabel = "pod"
			targetValue = v
		} else if v, ok := metadata["namespace"]; ok && v != "" {
			targetLabel = "namespace"
			targetValue = v
		}
		
		if targetLabel != "" {
			matches := ud.findMetricsWithPrefixes(allMetrics, []string{"http_", "app_", "grpc_", "spanmetrics_"})
			for _, m := range matches {
				probe := fmt.Sprintf("count(%s{%s=%q})", m, targetLabel, targetValue)
				if ud.testQuery(probe) {
					serviceMetrics = append(serviceMetrics, m)
				}
			}
		}

		if len(serviceMetrics) > 0 {
			metrics.DetectedLabel = targetLabel
			metrics.AvailableMetrics = serviceMetrics
			metrics.WorkingQueries = ud.generateWorkingQueries(service, serviceMetrics, targetLabel)
			if len(metrics.WorkingQueries) > 0 {
				return metrics
			}
		}
	}
	
	// Strategy 3: Pattern-Based Strategy (Phase B/C Fallback)
	return ud.discoverServiceMetrics(service)
}

func (ud *UniversalDiscovery) discoverKubernetesServicesWithStats() ([]string, int, int, error) {
	var rawServices []string
	
	if ud.cfg.EKS.Enabled {
		DebugLog("[DISCOVERY] Kubernetes/EKS enabled, kubeconfig=%s context=%s", ud.cfg.EKS.KubeConfig, ud.cfg.EKS.KubeContext)
		if ud.cfg.EKS.KubeConfig != "" {
			k8sServices := discoverByKubernetes(ud.cfg)
			rawServices = append(rawServices, k8sServices...)
			DebugLog("[DISCOVERY] Found %d services via standard Kubernetes API", len(k8sServices))
		}
		if ud.cfg.EKS.Region != "" {
			eksServices, err := discoverByEKS(ud.cfg)
			if err != nil {
				DebugLog("[DISCOVERY] EKS API error: %v", err)
			}
			rawServices = append(rawServices, eksServices...)
			DebugLog("[DISCOVERY] Found %d services via EKS API", len(eksServices))
		}
	} else {
		// Return empty without error if EKS not enabled
		return nil, 0, 0, nil
	}
	
	if len(rawServices) == 0 {
		return nil, 0, 0, fmt.Errorf("no raw services found from Kubernetes (context: %q)", ud.cfg.EKS.KubeContext)
	}
	
	serviceSet := make(map[string]bool)
	skippedCount := 0
	totalRaw := len(rawServices)
	
	for _, svc := range rawServices {
		parts := strings.Split(svc, "/")
		if len(parts) == 2 {
			namespace := parts[0]
			serviceName := parts[1]
			
			if isSystemNamespace(namespace) || isSystemService(serviceName) {
				skippedCount++
				continue
			}
			
			baseServiceName := extractBaseServiceName(serviceName)
			serviceSet[baseServiceName] = true
		}
	}
	
	var result []string
	for service := range serviceSet {
		result = append(result, service)
	}
	
	return result, totalRaw, skippedCount, nil
}

func (ud *UniversalDiscovery) discoverKubernetesServices() []string {
	// Use existing Kubernetes discovery logic (without debug)
	if ud.cfg.EKS.Enabled {
		services := discoverByKubernetes(ud.cfg)
		if len(services) > 0 {
			return services
		}
	}
	
	// Fallback to metric discovery
	client := prometheus.Client{
		BaseURL: ud.cfg.PrometheusURL,
		Token:   ud.cfg.PrometheusToken,
		User:    ud.cfg.PrometheusUser,
		Pass:    ud.cfg.PrometheusPass,
		Timeout: 10 * time.Second,
	}
	
	services := discoverByMetricNames(client, ud.cfg)
	if len(services) > 0 {
		return services
	}
	
	return discoverByServiceLabel(client, ud.cfg)
}

func (ud *UniversalDiscovery) detectPatterns() []MetricPattern {
	patterns := []MetricPattern{}
	
	// Pattern 0: OTel spanmetrics (Highest fidelity)
	// We check for the specific metric directly instead of generic prefix
	if ud.testQuery("count(spanmetrics_calls_total)") {
		patterns = append(patterns, MetricPattern{
			Name:      "otel_spanmetrics_pattern",
			Prefix:    "spanmetrics_",
			TestQuery: "count(spanmetrics_calls_total)",
			Extractor: ud.extractSpanmetricsPatternMetrics,
		})
	}

	// Pattern 1: app_service_operation_type
	if ud.testPattern("app_*") {
		patterns = append(patterns, MetricPattern{
			Name:      "app_pattern",
			Prefix:    "app_",
			TestQuery: "count(app_*)",
			Extractor: ud.extractAppPatternMetrics,
		})
	}
	
	// Pattern 2: service_operation_type
	if ud.testPattern("*_requests_total") {
		patterns = append(patterns, MetricPattern{
			Name:      "service_pattern", 
			Prefix:    "",
			TestQuery: "count(*_requests_total)",
			Extractor: ud.extractServicePatternMetrics,
		})
	}
	
	// Pattern 4: generic http_* (Lower priority)
	if ud.testPattern("http_*") {
		patterns = append(patterns, MetricPattern{
			Name:      "http_pattern",
			Prefix:    "http_",
			TestQuery: "count(http_*)",
			Extractor: ud.extractHTTPPatternMetrics,
		})
	}
	
	return patterns
}

func (ud *UniversalDiscovery) testPattern(pattern string) bool {
	// Test pattern by getting all metric names and checking if any match
	allMetrics := ud.getAllMetricNames()
	
	for _, metric := range allMetrics {
		if ud.matchesPattern(metric, pattern) {
			return true
		}
	}
	
	return false
}

func (ud *UniversalDiscovery) matchesPattern(metric, pattern string) bool {
	switch pattern {
	case "spanmetrics_*":
		return strings.HasPrefix(metric, "spanmetrics_")
	case "app_*":
		return strings.HasPrefix(metric, "app_")
	case "*_requests_total":
		return strings.HasSuffix(metric, "_requests_total")
	case "http_server_*":
		return strings.HasPrefix(metric, "http_server_")
	case "http_*":
		return strings.HasPrefix(metric, "http_")
	default:
		return false
	}
}

func (ud *UniversalDiscovery) discoverServiceMetrics(service string) ServiceMetrics {
	metrics := ServiceMetrics{Service: service}
	
	// Try each detected pattern for this service
	for _, pattern := range ud.patterns {
		patternMetrics := ud.getMetricsForPattern(service, pattern)
		
		if len(patternMetrics) > 0 {
			// Test which metrics actually work
			workingMetrics := ud.testMetricsWork(patternMetrics)
			
			if len(workingMetrics) > 0 {
				metrics.AvailableMetrics = workingMetrics
				metrics.WorkingQueries = ud.generateWorkingQueries(service, workingMetrics, "")
				return metrics // Success! Found working metrics
			}
		}
	}

	// FALLBACK: Brute-force scan if patterns failed
	// Look for ANY metric that contains the service name or its underscore version
	// fmt.Printf("Patterns failed for %s, trying brute-force scan\n", service)
	bruteMetrics := ud.bruteForceScan(service)
	if len(bruteMetrics) > 0 {
		workingMetrics := ud.testMetricsWork(bruteMetrics)
		if len(workingMetrics) > 0 {
			metrics.AvailableMetrics = workingMetrics
			metrics.WorkingQueries = ud.generateWorkingQueries(service, workingMetrics, "")
			return metrics
		}
	}
	
	// No working metrics found
	patternNames := make([]string, len(ud.patterns))
	for i, p := range ud.patterns {
		patternNames[i] = p.Name
	}
	metrics.SkipReason = fmt.Sprintf("No working metrics found. Tested patterns: %v", patternNames)
	
	// FINAL FALLBACK: Try to find by service label directly in Prometheus
	metricsList, foundLabel := ud.discoverByLabel(service)
	if len(metricsList) > 0 {
		workingLabels := ud.testMetricsWork(metricsList)
		if len(workingLabels) > 0 {
			metrics.AvailableMetrics = workingLabels
			metrics.DetectedLabel = foundLabel
			metrics.WorkingQueries = ud.generateWorkingQueries(service, workingLabels, foundLabel)
			metrics.SkipReason = "" // Clear skip reason as we found some
			return metrics
		}
	}
	
	return metrics
}

func (ud *UniversalDiscovery) discoverByLabel(service string) ([]string, string) {
	// Query Prometheus for any metric that has a service-matching label
	labelsToTry := []string{ud.cfg.ServiceLabel, "service", "app", "service_name", "container", "job", "k8s_app"}
	
	for _, label := range labelsToTry {
		if label == "" { continue }
		
		query := fmt.Sprintf("group by (__name__) ({%s=%q})", label, service)
		result, err := ud.client.QueryVector(query)
		if err != nil || len(result) == 0 {
			continue
		}
		
		var metrics []string
		for _, sample := range result {
			if name, ok := sample.Metric["__name__"]; ok {
				nameStr := string(name)
				if isSystemMetric(nameStr) {
					continue
				}
				metrics = append(metrics, nameStr)
			}
		}
		
		if len(metrics) > 0 {
			return metrics, label
		}
	}
	
	return nil, ""
}

func (ud *UniversalDiscovery) bruteForceScan(service string) []string {
	allMetrics := ud.getAllMetricNames()
	var matches []string
	svcVariants := []string{service, strings.ReplaceAll(service, "-", "_")}
	
	for _, metric := range allMetrics {
		if isSystemMetric(metric) {
			continue
		}
		for _, variant := range svcVariants {
			if strings.Contains(metric, variant) {
				matches = append(matches, metric)
				break
			}
		}
	}
	return matches
}

func (ud *UniversalDiscovery) getMetricsForPattern(service string, pattern MetricPattern) []string {
	// Get all metric names
	allMetrics := ud.getAllMetricNames()
	
	// Filter by pattern and extract for this service
	var serviceMetrics []string
	
	// Some patterns are generic and don't contain the service name in the metric name
	isGeneric := pattern.Prefix == "spanmetrics_" || pattern.Prefix == "http_server_" || pattern.Prefix == "http_"

	for _, metric := range allMetrics {
		if isSystemMetric(metric) {
			continue
		}
		
		// If generic, we include it based on prefix alone; extractor will refine based on labels
		if strings.HasPrefix(metric, pattern.Prefix) && (isGeneric || ud.isServiceMetric(metric, service)) {
			serviceMetrics = append(serviceMetrics, metric)
		}
	}
	
	return pattern.Extractor(service, serviceMetrics)
}

func (ud *UniversalDiscovery) getAllMetricNames() []string {
	// Get all metric names from Prometheus
	query := "group by (__name__) ({__name__!=\"\"})"
	result, err := ud.client.QueryVector(query)
	if err != nil {
		return []string{}
	}
	
	var metrics []string
	for _, sample := range result {
		if name, ok := sample.Metric["__name__"]; ok {
			if isSystemMetric(name) {
				continue
			}
			metrics = append(metrics, name)
		}
	}
	
	return metrics
}

func (ud *UniversalDiscovery) isServiceMetric(metric, service string) bool {
	// Check if metric belongs to this service
	return strings.Contains(metric, service) || strings.Contains(metric, strings.Replace(service, "-", "_", -1))
}

func (ud *UniversalDiscovery) extractSpanmetricsPatternMetrics(service string, metrics []string) []string {
	// Spanmetrics rely on the 'service_name' label, not the metric name
	return metrics
}

func (ud *UniversalDiscovery) extractAppPatternMetrics(service string, metrics []string) []string {
	// Extract metrics matching app_service_* pattern
	var result []string
	servicePattern := fmt.Sprintf("app_%s_", service)
	
	for _, metric := range metrics {
		if strings.HasPrefix(metric, servicePattern) {
			result = append(result, metric)
		}
	}
	
	return result
}

func (ud *UniversalDiscovery) extractServicePatternMetrics(service string, metrics []string) []string {
	// Extract metrics matching service_* pattern
	var result []string
	
	for _, metric := range metrics {
		if strings.HasPrefix(metric, service+"_") {
			result = append(result, metric)
		}
	}
	
	return result
}

func (ud *UniversalDiscovery) extractHTTPServerPatternMetrics(service string, metrics []string) []string {
	// HTTP server metrics are generic, return all
	return metrics
}

func (ud *UniversalDiscovery) extractHTTPPatternMetrics(service string, metrics []string) []string {
	// Generic HTTP metrics, return all
	return metrics
}

func (ud *UniversalDiscovery) testMetricsWork(metrics []string) []string {
	var working []string
	
	for _, metric := range metrics {
		// Test basic existence
		query := fmt.Sprintf("count(%s)", metric)
		if result, err := ud.client.QueryVector(query); err == nil && len(result) > 0 {
			working = append(working, metric)
		}
	}
	
	return working
}

func (ud *UniversalDiscovery) generateWorkingQueries(service string, metrics []string, detectedLabel string) map[string]string {
	queries := make(map[string]string)
	
	// Strategy 1: Find request and error metrics using refined prioritization
	totalMetric := ud.generateTotalQuery(service, metrics, detectedLabel)
	errorMetric := ud.generateErrorQuery(service, metrics, detectedLabel)

	// SAFETY: If total and error metrics are the same, it leads to 0% or 100% inaccuracy.
	// We only set them if they are distinct or if we have no other choice (better no SLO than a broken one).
	if totalMetric != "" {
		queries["total"] = totalMetric
	}

	if errorMetric != "" && errorMetric != totalMetric {
		queries["error"] = errorMetric
	}
	
	// Strategy 2: Find latency metrics
	latencyMetric := ud.findLatencyMetric(metrics)
	if latencyMetric != "" {
		queries["latency"] = ud.generateLatencyQuery(latencyMetric, service, detectedLabel)
	}
	
	// Strategy 3: Test queries actually work
	return ud.validateQueries(queries)
}

func (ud *UniversalDiscovery) generateTotalQuery(service string, metrics []string, detectedLabel string) string {
	label := detectedLabel
	if label == "" {
		label = ud.cfg.ServiceLabel
	}

	// Look for OTel spanmetrics first
	for _, m := range metrics {
		if m == "spanmetrics_calls_total" {
			// standard OTel spanmetrics use service_name
			return fmt.Sprintf("spanmetrics_calls_total{service_name=%q}", service)
		}
	}

	for _, m := range metrics {
		if m == "calls_total" || m == "http_requests_total" || m == "rpc_server_calls_total" {
			// Generic metrics NEED a label
			if label == "" { label = "service" }
			return fmt.Sprintf("%s{%s=%q}", m, label, service)
		}
	}

	if len(metrics) > 0 {
		m := metrics[0]
		if label != "" {
			// Test if it works with label
			q := fmt.Sprintf("%s{%s=%q}", m, label, service)
			if ud.testQuery(fmt.Sprintf("count(%s)", q)) {
				return q
			}
		}
		// Fallback to bare metric if name implies service
		return m
	}
	return ""
}

func (ud *UniversalDiscovery) generateErrorQuery(service string, metrics []string, detectedLabel string) string {
	label := detectedLabel
	if label == "" {
		label = ud.cfg.ServiceLabel
	}

	// Look for OTel spanmetrics first
	for _, m := range metrics {
		if m == "spanmetrics_calls_total" {
			return fmt.Sprintf("spanmetrics_calls_total{service_name=%q, status_code!~\"0|OK\"}", service)
		}
	}

	errorLabel := "status" // Default
	for _, m := range metrics {
		if m == "calls_total" || m == "http_requests_total" {
			if eld := ud.client.DiscoverErrorLabel(m); eld != "" {
				errorLabel = eld
			}
			if label == "" { label = "service" }
			return fmt.Sprintf("%s{%s=%q, %s!~\"2..|0|OK\"}", m, label, service, errorLabel)
		}
	}

	// For specific metrics, try to find an error label
	if len(metrics) > 0 {
		m := metrics[0]
		eld := ud.client.DiscoverErrorLabel(m)
		if eld == "" { eld = "status" }
		
		if label != "" {
			q := fmt.Sprintf("%s{%s=%q, %s!~\"2..|0|OK\"}", m, label, service, eld)
			if ud.testQuery(fmt.Sprintf("count(%s)", q)) {
				return q
			}
		}
		return fmt.Sprintf("%s{%s!~\"2..|0|OK\"}", m, eld)
	}

	return ""
}

func (ud *UniversalDiscovery) findLatencyMetric(metrics []string) string {
	for _, m := range metrics {
		if m == "spanmetrics_latency_bucket" {
			return m
		}
		if strings.HasSuffix(m, "_bucket") {
			return m
		}
	}
	for _, m := range metrics {
		if strings.Contains(m, "latency") || strings.Contains(m, "duration") {
			if !strings.HasSuffix(m, "_bucket") {
				return m
			}
		}
	}
	return ""
}

func (ud *UniversalDiscovery) generateLatencyQuery(metric, service, detectedLabel string) string {
	label := detectedLabel
	if label == "" {
		label = ud.cfg.ServiceLabel
	}

	if metric == "spanmetrics_latency_bucket" {
		return fmt.Sprintf("histogram_quantile(0.95, sum(rate(spanmetrics_latency_bucket{service_name=%q}[15m])) by (le))", service)
	}

	bucketMetric := metric
	if !strings.HasSuffix(metric, "_bucket") {
		bucketMetric = metric + "_bucket"
	}

	if label != "" {
		q := fmt.Sprintf("%s{%s=%q}", bucketMetric, label, service)
		if ud.testQuery(fmt.Sprintf("count(%s)", q)) {
			return fmt.Sprintf("histogram_quantile(0.95, sum(rate(%s[15m])) by (le))", q)
		}
	}

	return fmt.Sprintf("histogram_quantile(0.95, sum(rate(%s[15m])) by (le))", bucketMetric)
}

func (ud *UniversalDiscovery) validateQueries(queries map[string]string) map[string]string {
	validated := make(map[string]string)
	
	for queryType, query := range queries {
		// Test query with small time window
		testQuery := strings.Replace(query, "15m", "1m", -1)
		if _, err := ud.client.QueryVector(testQuery); err == nil {
			validated[queryType] = query // Original query with 15m
		}
	}
	
	return validated
}

func (ud *UniversalDiscovery) generateServiceMetadata(metrics ServiceMetrics) model.ServiceMetadata {
	metadata := model.ServiceMetadata{
		RequestMetric: "http_requests_total",    // Safe default
		LatencyMetric: "http_request_duration_seconds", // Safe default
		ErrorLabel:    "",                      
	}
	
	// Use discovered working queries to extract actual metric names
	if totalQuery, ok := metrics.WorkingQueries["total"]; ok {
		metadata.RequestMetric = ud.extractMetricName(totalQuery)
	}
	
	if latencyQuery, ok := metrics.WorkingQueries["latency"]; ok {
		metadata.LatencyMetric = ud.extractMetricName(latencyQuery)
	}

	// NEW: Use discovered labels for the service (Phase A/B)
	if metrics.DetectedLabel != "" {
		metadata.ServiceLabel = metrics.DetectedLabel
	} else if metrics.InferredLabels != nil {
		if v, ok := metrics.InferredLabels["container"]; ok && v != "" {
			metadata.ServiceLabel = "container"
		} else if v, ok := metrics.InferredLabels["pod"]; ok && v != "" {
			metadata.ServiceLabel = "pod"
		}
	}
	
	// OTel specific: if we are using spanmetrics, the trace tag is likely service_name
	if metadata.RequestMetric == "spanmetrics_calls_total" {
		metadata.ServiceLabel = "service_name"
	}
	
	// Dynamically discover error label to prevent misconfigured success flows
	if metadata.RequestMetric != "" {
		// OTel standard: status_code is the most reliable for spanmetrics
		if metadata.RequestMetric == "spanmetrics_calls_total" {
			metadata.ErrorLabel = "status_code"
		} else {
			metadata.ErrorLabel = ud.client.DiscoverErrorLabel(metadata.RequestMetric)
		}
	}
	
	// If OTel was successfully identified, mark the schema
	if metadata.RequestMetric == "spanmetrics_calls_total" {
		metadata.MetricSchema = "otel"
		ud.cfg.MetricSchema = "otel"
	}

	return metadata
}

func (ud *UniversalDiscovery) extractMetricName(query string) string {
	// If it's a wrapped query like sum(rate(metric[...]))
	if strings.Contains(query, "rate(") {
		start := strings.Index(query, "rate(") + 5
		end := strings.Index(query[start:], "[")
		if end > 0 {
			query = query[start : start+end]
		}
	} else if strings.Contains(query, "histogram_quantile") {
		// Extract from sum(rate(metric[...])) inside histogram_quantile
		if strings.Contains(query, "rate(") {
			start := strings.Index(query, "rate(") + 5
			end := strings.Index(query[start:], "[")
			if end > 0 {
				query = query[start : start+end]
			}
		}
	}
	
	// If it's a selector like metric{label="val"}, strip the labels
	if strings.Contains(query, "{") {
		query = strings.Split(query, "{")[0]
	}
	
	// Strip parentheses if any remain
	query = strings.Trim(query, "()")
	
	return strings.TrimSpace(query)
}

func (ud *UniversalDiscovery) printReport() {
	// Bubletea wizard handles rendering the discovered services UI.
	// We no longer print anything to stdout here to avoid corrupting the TUI layout.
}

func (ud *UniversalDiscovery) generateReport() DiscoveryReport {
	report := DiscoveryReport{
		TotalServices: len(ud.discovered),
		GeneratedFlows: 0,
		SuccessfulServices: []string{},
		SkippedServices: []SkippedService{},
	}
	
	for service, metrics := range ud.discovered {
		if metrics.SkipReason != "" {
			report.SkippedServices = append(report.SkippedServices, SkippedService{
				Service: service,
				Reason:  metrics.SkipReason,
			})
		} else {
			report.GeneratedFlows++
			report.SuccessfulServices = append(report.SuccessfulServices, service)
		}
	}
	
	return report
}

func discoverByServiceLabel(client prometheus.Client, cfg Config) []string {
	// Original logic - preserved for backward compatibility
	query := fmt.Sprintf("count by (%s) ({%s!=\"\"})", cfg.ServiceLabel, cfg.ServiceLabel)
	samples, err := client.QueryVector(query)
	if err != nil {
		return nil
	}
	
	var services []string
	for _, s := range samples {
		if svc, ok := s.Metric[cfg.ServiceLabel]; ok && svc != "" {
			if isSystemService(svc) {
				continue
			}
			services = append(services, svc)
		}
	}
	return services
}

func discoverByMetricNames(client prometheus.Client, cfg Config) []string {
	// Conservative discovery - only find actual application services
	query := "group by (__name__) ({__name__!=\"\"})"
	samples, err := client.QueryVector(query)
	if err != nil {
		return nil
	}
	
	serviceSet := make(map[string]bool)
	
	// Look for ONLY application-specific metric patterns
	for _, s := range samples {
		metricName := s.Metric["__name__"]
		
		// Only consider app_* metrics as application services
		if strings.HasPrefix(metricName, "app_") {
			parts := strings.Split(metricName, "_")
			if len(parts) >= 2 {
				service := parts[1]
				if !isSystemService(service) && len(service) > 0 {
					serviceSet[service] = true
				}
			}
		}
		
		// Only consider http_server metrics as a service
		if strings.Contains(metricName, "http_server") {
			serviceSet["http_server"] = true
		}
	}
	
	// Convert map to slice
	var services []string
	for service := range serviceSet {
		services = append(services, service)
	}
	return services
}

func discoverByKubernetes(cfg Config) []string {
	// Try to use local kubeconfig first, then fall back to EKS
	var rawServices []string
	
	// Method 1: Try local Kubernetes discovery (without AWS)
	localServices, err := discoverByLocalKubernetes(cfg)
	if err == nil && len(localServices) > 0 {
		rawServices = localServices
	} else {
		// Method 2: Try EKS discovery if enabled and has region
		if cfg.EKS.Enabled && cfg.EKS.Region != "" {
			eksServices, err := discoverByEKS(cfg)
			if err == nil && len(eksServices) > 0 {
				rawServices = eksServices
			}
		}
	}
	
	// Extract service names from namespace/service format and filter
	serviceSet := make(map[string]bool)
	skippedCount := 0
	for _, svc := range rawServices {
		parts := strings.Split(svc, "/")
		if len(parts) == 2 {
			namespace := parts[0]
			serviceName := parts[1]
			
			// Skip system namespaces
			if isSystemNamespace(namespace) {
				skippedCount++
				continue
			}
			
			// Skip system services
			if isSystemService(serviceName) {
				skippedCount++
				continue
			}
			
			// Extract base service name by removing common suffixes
			baseServiceName := extractBaseServiceName(serviceName)
			serviceSet[baseServiceName] = true
		}
	}
	
	// Convert to slice
	var result []string
	for service := range serviceSet {
		result = append(result, service)
	}
	
	return result
}

func discoverByLocalKubernetes(cfg Config) ([]string, error) {
	// Use local kubeconfig without AWS credentials
	discoveryConfig := discovery.K8sConfig{
		Enabled:     true,  // Always enabled for local discovery
		KubeConfig:  cfg.EKS.KubeConfig,
		KubeContext: cfg.EKS.KubeContext,
		Region:      "",    // Empty for local clusters
	}
	
	engine, err := discovery.NewDiscoveryEngine(discoveryConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create local discovery engine: %w", err)
	}
	
	// Get all namespaces
	namespaces, err := engine.DiscoverNamespaces()
	if err != nil {
		return nil, fmt.Errorf("failed to discover namespaces: %w", err)
	}
	
	// Try services first
	services, err := engine.DiscoverServices(namespaces)
	if err != nil {
		services = []string{}
	}
	
	// If no services found, try discovering from pods
	if len(services) == 0 {
		services = discoverFromPods(engine, namespaces)
	}
	
	return services, nil
}

func discoverByEKS(cfg Config) ([]string, error) {
	// Try with AWS credentials
	discoveryConfig := discovery.K8sConfig{
		Enabled:     cfg.EKS.Enabled,
		KubeConfig:  cfg.EKS.KubeConfig,
		KubeContext: cfg.EKS.KubeContext,
		AWSProfile:  cfg.EKS.AWSProfile,
		Region:      cfg.EKS.Region,
	}
	
	// First try to create EKS discovery engine
	engine, err := discovery.NewDiscoveryEngine(discoveryConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create EKS discovery engine: %w", err)
	}
	
	// Get all namespaces
	namespaces, err := engine.DiscoverNamespaces()
	if err != nil {
		return nil, fmt.Errorf("failed to discover EKS namespaces: %w", err)
	}
	
	// Try services first
	services, err := engine.DiscoverServices(namespaces)
	if err != nil {
		services = []string{}
	}
	
	// If no services found, try discovering from pods
	if len(services) == 0 {
		services = discoverFromPods(engine, namespaces)
	}
	
	return services, nil
}

func discoverFromPods(engine *discovery.DiscoveryEngine, namespaces []string) []string {
	// Discover services from pod names when no K8s services exist
	var podServices []string
	
	for _, ns := range namespaces {
		// Use the engine to get pod health - this internally lists pods
		_, workloads, err := engine.GetNamespaceHealth(ns)
		if err != nil {
			continue
		}
		
		// Extract service names from workloads (deployments)
		for _, workload := range workloads {
			if !isSystemService(workload.Name) {
				podServices = append(podServices, fmt.Sprintf("%s/%s", ns, workload.Name))
			}
		}
	}
	
	return podServices
}

func extractBaseServiceName(serviceName string) string {
	// Robust regex-based suffix stripping for Kubernetes pods/replicasets
	// Standard formats: -[replicaset-hash]-[pod-hash] or -[statefulset-ordinal]
	
	// Pattern 1: Deployment/ReplicaSet suffixes (e.g., -54746ffc84-62rg7)
	reReplica := regexp.MustCompile(`-[a-f0-9]{7,10}-[a-z0-9]{5}$`)
	if match := reReplica.FindStringIndex(serviceName); match != nil {
		return serviceName[:match[0]]
	}

	// Pattern 2: StatefulSet/Job suffixes (e.g., -0, -1, -v1-abcde)
	reOrdinal := regexp.MustCompile(`-[0-9]+$`)
	if match := reOrdinal.FindStringIndex(serviceName); match != nil {
		return serviceName[:match[0]]
	}
	
	// Fallback to legacy hardcoded list for non-standard legacy naming if needed
	suffixes := []string{"-54746ffc84-62rg7", "-7f9f64479-d9gf2", "-78874668d-g22l9", 
		"-6b59f9ccb4-g4vnf", "-67f769c69-jcwjb", "-df76cdc65-shwvq", "-55cb9c658-p8ccb",
		"-5976fc5c7b-qn6wk", "-66cbd69c8c-5qlc4", "-649b6f5fcb-xcp96", "-76fd778f49-mw27z",
		"-76b4cc75dd-h4lhh", "-b5cbc89f7-mrdbc", "-f7bf6dbb8-lshsf", "-77b458599c-5wx2f",
		"-768d9464ff-pnn4n", "-fdcdcbf8f-c6ctc", "-7b87d99574-j94gh", "-6f9c4c9487-jkqmn",
		"-7966cd784c-8w8l5", "-7cc4575d68-mtsvk"}
	
	baseName := serviceName
	for _, suffix := range suffixes {
		if strings.HasSuffix(baseName, suffix) {
			baseName = strings.TrimSuffix(baseName, suffix)
			break
		}
	}
	
	return baseName
}

func isSystemNamespace(namespace string) bool {
	systemNamespaces := []string{
		"kube-system", "monitoring", "kube-public", "kube-node-lease",
		"istio-system", "linkerd", "gatekeeper-system", "logging",
	}
	
	for _, sysNs := range systemNamespaces {
		if namespace == sysNs {
			return true
		}
	}
	return false
}

func generateServiceMetadata(service string, cfg Config) model.ServiceMetadata {
	// Universal metadata generation - detect what actually exists
	client := prometheus.Client{
		BaseURL: cfg.PrometheusURL,
		Token:   cfg.PrometheusToken,
		User:    cfg.PrometheusUser,
		Pass:    cfg.PrometheusPass,
		Timeout: 5 * time.Second,
	}
	
	metadata := model.ServiceMetadata{
		RequestMetric: "http_requests_total",    // Safe default
		LatencyMetric: "http_request_duration_seconds", // Safe default
		ErrorLabel:    "status",                 // Common default
	}
	
	// Try to discover actual metrics for this service
	discoveredMetrics := discoverMetricsForService(client, service)
	if discoveredMetrics.RequestMetric != "" {
		metadata.RequestMetric = discoveredMetrics.RequestMetric
	}
	if discoveredMetrics.LatencyMetric != "" {
		metadata.LatencyMetric = discoveredMetrics.LatencyMetric
	}
	if discoveredMetrics.ErrorLabel != "" {
		metadata.ErrorLabel = discoveredMetrics.ErrorLabel
	}
	
	return metadata
}

func discoverMetricsForService(client prometheus.Client, service string) model.ServiceMetadata {
	metadata := model.ServiceMetadata{}
	
	// Look for request/count metrics for this service
	requestPatterns := []string{
		fmt.Sprintf("app_%s_requests_total", service),
		fmt.Sprintf("app_%s_total", service),
		fmt.Sprintf("%s_requests_total", service),
		fmt.Sprintf("%s_total", service),
		fmt.Sprintf("%s_requests", service),
	}
	
	for _, pattern := range requestPatterns {
		query := fmt.Sprintf("count(%s)", pattern)
		_, err := client.QueryVector(query)
		if err == nil {
			metadata.RequestMetric = pattern
			break
		}
	}
	
	// Look for latency metrics
	latencyPatterns := []string{
		fmt.Sprintf("app_%s_request_duration_seconds", service),
		fmt.Sprintf("%s_request_duration_seconds", service),
		fmt.Sprintf("app_%s_latency", service),
		fmt.Sprintf("%s_latency", service),
		"http_server_request_duration_seconds",
	}
	
	for _, pattern := range latencyPatterns {
		query := fmt.Sprintf("count(%s_bucket)", pattern)
		_, err := client.QueryVector(query)
		if err == nil {
			metadata.LatencyMetric = pattern
			break
		}
	}
	
	// Look for common error labels
	errorLabels := []string{"status", "code", "error", "result"}
	for _, label := range errorLabels {
		// Try to find this label in the request metric
		if metadata.RequestMetric != "" {
			query := fmt.Sprintf("group by (%s) (%s)", label, metadata.RequestMetric)
			_, err := client.QueryVector(query)
			if err == nil {
				metadata.ErrorLabel = label
				break
			}
		}
	}
	
	return metadata
}

func isSystemService(svc string) bool {
	// Only filter out actual system services, not application services
	systemServices := map[string]bool{
		// Kubernetes system services
		"kubernetes": true,
		"apiserver": true,
		"kubelet": true,
		"kubeproxy": true,
		"coredns": true,
		"etcd": true,
		"vault": true,
		"metrics-server": true,
		
		// Monitoring infrastructure
		"prometheus": true,
		"grafana": true,
		"loki": true,
		"tempo": true,
		"alertmanager": true,
		"kube-state-metrics": true,
		"node-exporter": true,
		"promhttp": true,
		
		// Tracing infrastructure
		"jaeger-agent": true,
		"jaeger-collector": true,
		"jaeger-query": true,
		"otel-collector": true,
		
		// System components (not applications)
		"eks-extension-metrics-api": true,
	}
	
	// Check exact match first
	if systemServices[svc] {
		return true
	}
	
	// Check prefixes (be more specific)
	systemPrefixes := []string{
		"kube-", 
		"prometheus-", 
		"cert-manager-",
		"istio-", 
		"linkerd-",
	}
	
	svcLower := strings.ToLower(svc)
	for _, p := range systemPrefixes {
		if strings.HasPrefix(svcLower, p) {
			return true
		}
	}
	
	// Filter out services with colons (usually system labels like cluster:node)
	if strings.Contains(svc, ":") {
		return true
	}
	
	// Filter out single-character or very short service names
	if len(svc) <= 2 {
		return true
	}
	
	return false
}

func isSystemMetric(metric string) bool {
	systemMetricPrefixes := []string{
		"alertmanager_", "prometheus_", "node_", "go_", "process_", 
		"scrape_", "rest_client_", "workqueue_", "apiserver_", "etcd_", 
		"machine_", "cadvisor_", "promhttp_", "net_", "aggregator_", "storage_",
		"kube_", "container_", "hidden_",
	}
	
	// Also check for common system metric substrings
	systemSubstrings := []string{
		"_gc_", "_memstats_", "_mallocs_", "_frees_", "_panics_total",
		"python_gc_", "ruby_gc_", "jvm_gc_",
	}

	mLower := strings.ToLower(metric)
	for _, p := range systemMetricPrefixes {
		if strings.HasPrefix(mLower, p) {
			return true
		}
	}
	for _, s := range systemSubstrings {
		if strings.Contains(mLower, s) {
			return true
		}
	}
	return false
}

func testPrometheus(cfg Config) tea.Cmd {
	return func() tea.Msg {
		client := prometheus.Client{
			BaseURL: cfg.PrometheusURL,
			Token:   cfg.PrometheusToken,
			User:    cfg.PrometheusUser,
			Pass:    cfg.PrometheusPass,
			Timeout: 5 * time.Second,
		}
		// Deep check: try a real query
		_, err := client.QueryInstant("count(up)")
		if err != nil {
			// Fallback to basic Check() to see if it's just a permission issue or a connection issue
			if _, checkErr := client.Check(); checkErr != nil {
				return textResultMsg(fmt.Sprintf("❌ Prometheus: %v", checkErr))
			}
			return textResultMsg(fmt.Sprintf("❌ Prometheus: query failed (check permissions): %v", err))
		}
		return textResultMsg("✅ Prometheus: Connected and queryable")
	}
}

func testLoki(cfg Config) tea.Cmd {
	return func() tea.Msg {
		client := loki.Client{
			BaseURL: cfg.LokiURL,
			Token:   cfg.LokiToken,
			User:    cfg.LokiUser,
			Pass:    cfg.LokiPass,
			Timeout: 5 * time.Second,
		}
		// Deep check: try listing labels
		_, err := client.Labels()
		if err != nil {
			if _, checkErr := client.Check(); checkErr != nil {
				return textResultMsg(fmt.Sprintf("❌ Loki: %v", checkErr))
			}
			return textResultMsg(fmt.Sprintf("❌ Loki: labels query failed (check permissions): %v", err))
		}
		return textResultMsg("✅ Loki: Connected and queryable")
	}
}

func testTrace(cfg Config) tea.Cmd {
	return func() tea.Msg {
		client := tracing.Client{
			BaseURL: cfg.TraceURL,
			Token:   cfg.TraceToken,
			User:    cfg.TraceUser,
			Pass:    cfg.TracePass,
			Timeout: 5 * time.Second,
		}
		var msg string
		var err error
		backend := "Trace"
		if cfg.TraceBackend == "jaeger" {
			backend = "Jaeger"
			msg, err = client.JaegerHealth()
		} else {
			backend = "Tempo"
			msg, err = client.TempoHealth()
		}

		if err != nil {
			return textResultMsg(fmt.Sprintf("❌ %s: %v", backend, err))
		}
		return textResultMsg(fmt.Sprintf("✅ %s: %s", backend, msg))
	}
}
func (m WizardModel) formatServiceList(services []string) string {
	if len(services) == 0 {
		return ""
	}
	
	const width = 80
	const indent = "  "
	
	var lines []string
	var currentLine string
	
	for _, svc := range services {
		if currentLine == "" {
			currentLine = indent + svc
		} else if len(currentLine)+len(svc)+2 > width {
			lines = append(lines, currentLine)
			currentLine = indent + svc
		} else {
			currentLine += ", " + svc
		}
	}
	
	if currentLine != "" {
		lines = append(lines, currentLine)
	}
	
	return strings.Join(lines, "\n")
}

func (m *WizardModel) runInstantTest() tea.Cmd {
	return func() tea.Msg {
		client := prometheus.Client{
			BaseURL: m.config.PrometheusURL,
			Token:   m.config.PrometheusToken,
			User:    m.config.PrometheusUser,
			Pass:    m.config.PrometheusPass,
			Timeout: 15 * time.Second,
		}
		
		// Re-implement simplified version of CollectAllServicesOverview to break import cycle
		lookback := 1 * time.Hour
		seenServices := make(map[string]string)
		labelsToTry := []string{"service", "service.name", "app", "job"}
		
		for _, l := range labelsToTry {
			values, err := client.LabelValues(l, lookback)
			if err == nil {
				for _, val := range values {
					if val != "" && seenServices[val] == "" {
						seenServices[val] = l
					}
				}
			}
		}

		if len(seenServices) == 0 {
			return instantTestMsg([]model.ServiceOverview{})
		}

		// Use Parallel Probing locally
		var servicesToProbe []string
		for svc := range seenServices {
			servicesToProbe = append(servicesToProbe, svc)
		}

		pool := discovery.NewServicePool(10)
		probeFunc := func(svcName string) (interface{}, error) {
			label := seenServices[svcName]
			query := fmt.Sprintf(`sum(rate(%s{%s="%s"}[5m]))`, m.config.RequestCountMetric, label, svcName)
			rps, _ := client.QueryInstant(query)
			
			errQuery := fmt.Sprintf(`sum(rate(%s{%s="%s", %s=~"%s"}[5m])) / sum(rate(%s{%s="%s"}[5m])) * 100`, 
				m.config.RequestCountMetric, label, svcName, m.config.ErrorLabel, m.config.ErrorRegex, 
				m.config.RequestCountMetric, label, svcName)
			errRate, _ := client.QueryInstant(errQuery)

			return model.GoldenSignals{
				RequestsPerSecond: rps,
				ErrorRate:         errRate,
			}, nil
		}

		probeResults := pool.ParallelProbe(servicesToProbe, probeFunc)
		overviews := make([]model.ServiceOverview, 0, len(probeResults))

		for _, res := range probeResults {
			signals := res.Data.(model.GoldenSignals)
			overview := model.ServiceOverview{
				Name:      res.Service,
				RPS:       signals.RequestsPerSecond,
				ErrorRate: signals.ErrorRate,
				Status:    model.SAFE,
			}
			if math.IsNaN(overview.RPS) { overview.RPS = 0 }
			if math.IsNaN(overview.ErrorRate) { overview.ErrorRate = 0 }

			if overview.ErrorRate > 1 { overview.Status = model.CHECK }
			if overview.ErrorRate > 5 { overview.Status = model.RISK }
			
			overviews = append(overviews, overview)
		}

		sort.Slice(overviews, func(i, j int) bool {
			return overviews[i].Name < overviews[j].Name
		})

		return instantTestMsg(overviews)
	}
}
