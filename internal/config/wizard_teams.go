package config

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"health-monitor/internal/analyse/loki"
	"health-monitor/internal/analyse/prometheus"
	"health-monitor/internal/discovery"
	"health-monitor/pkg/model"
)


// TeamWizardState represents different states in the team setup wizard
type TeamWizardState int

const (
	StateTeamIntro TeamWizardState = iota
	StateTeamSize
	StatePresetSelection
	StatePresetConfirmation
	StateTeamProfileName
	StateTeamPrometheusURL
	StateTeamPrometheusAuth
	StateTeamLokiURL
	StateTeamLokiAuth
	StateTeamGrafanaURL
	StateTeamGrafanaAuth
	StateTeamTracingURL
	StateTeamTracingAuth
	StateTeamNotificationsSlack
	StateTeamNotificationsPagerDuty
	StateTeamElasticURL
	StateTeamElasticAuth
	StateTeamElasticIndex
	StateTeamConnectivityTest
	StateQuickStart
	StateTeamDone
	StateTeamConflictWarning
	StateTeamPrometheusSampling
	StateInfraSelection
	StateKubeConfig
	StateKubeContextSelection
	StateAWSConfigPath
	StateAWSProfile
	StateAWSRegion
)

var (
	cliTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	cliLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("7")).Bold(true)
	cliDimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	cliBoxStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2)
	cliSuccessStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	cliErrorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
)

// TeamWizardModel represents the team setup wizard model
type TeamWizardModel struct {
	state          TeamWizardState
	selectedPreset TeamPreset
	teamSize       string
	profileName    string
	config         Config
	textInput      textinput.Model
	err            error
	quitting       bool
	testingConn    bool
	testResult     string
	backends       map[string]bool
	notifications  map[string]bool
	slackWebhook   string
	pagerDutyKey   string
	quickStart     bool
	authMethod     string
	username       string
	password       string
	conflicts      []string
	infraType      string // "bare-metal", "kubernetes", "eks"
	kubeConfig     string
	kubeContext    string
	awsConfigPath  string
	awsProfile     string
	awsRegion      string
	presets        []TeamPreset
	selectedPresetIndex int
	services       []string
}

func NewTeamWizard() *TeamWizardModel {
	ti := textinput.New()
	ti.Placeholder = "Enter profile name"
	ti.Focus()
	ti.Width = 100
	ti.CharLimit = 512

	m := &TeamWizardModel{
		state:         StateTeamIntro,
		config:        Default(),
		textInput:     ti,
		backends:      map[string]bool{"prometheus": true},
		notifications: make(map[string]bool),
		infraType:     "bare-metal",
		presets:       GetAvailablePresets(),
	}
	if len(m.presets) > 0 {
		m.selectedPreset = m.presets[0]
	}
	return m
}

// Init initializes the team wizard
func (m *TeamWizardModel) Init() tea.Cmd {
	return nil
}

// Update handles updates in the team wizard
func (m *TeamWizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			m.quitting = true
			return m, tea.Quit

		case tea.KeyEnter:
			return m.nextStep()

		case tea.KeyUp, tea.KeyDown:
			return m.handleArrowKey(msg.Type)

		case tea.KeyTab:
			return m.handleTab()

		default:
			if m.state == StateTeamConnectivityTest {
				key := strings.ToLower(msg.String())
				if key == "r" {
					return m, m.testConnectivity()
				}
				if key == "s" {
					// User chose to skip/ignore errors
					m.state = StateTeamDone
					return m, nil
				}
			}
			if m.isInputState() {
				m.textInput, cmd = m.textInput.Update(msg)
				return m, cmd
			}
		}

	case testResultMsg:
		m.testingConn = false
		m.testResult = string(msg)
		// If there are errors, we stay in the state to let the user see them
		return m, nil

	case samplingResultMsg:
		if msg != "" {
			m.config.ServiceLabel = string(msg)
		}
		m.state = StateTeamDone
		return m, nil
	}

	return m, cmd
}

// nextStep processes the transition to the next state
func (m *TeamWizardModel) nextStep() (tea.Model, tea.Cmd) {
	switch m.state {
	case StateTeamIntro:
		m.state = StatePresetSelection

	case StateTeamSize:
		m.state = StatePresetSelection

	case StatePresetSelection:
		if m.selectedPreset.Name == "" {
			m.err = fmt.Errorf("please select a preset")
			return m, nil
		}
		m.state = StatePresetConfirmation

	case StatePresetConfirmation:
		m.state = StateInfraSelection
		return m, nil

	case StateInfraSelection:
		if m.infraType == "bare-metal" {
			m.state = StateTeamProfileName
			m.textInput.Focus()
			m.textInput.Placeholder = "Enter profile name (e.g., my-team)"
		} else {
			m.state = StateKubeConfig
			m.textInput.Focus()
			m.textInput.SetValue("~/.kube/config")
			m.textInput.Placeholder = "Path to kubeconfig"
		}

	case StateKubeConfig:
		m.kubeConfig = ExpandTilde(m.textInput.Value())
		m.config.EKS.KubeConfig = m.kubeConfig
		m.state = StateKubeContextSelection
		m.textInput.Reset()
		m.textInput.Placeholder = "Context Name (e.g., arn:aws:eks:region:account:cluster/name)"
		return m, nil

	case StateKubeContextSelection:
		m.kubeContext = m.textInput.Value()
		m.config.EKS.KubeContext = m.kubeContext
		m.textInput.Reset()
		if m.infraType == "eks" || m.config.EKS.Enabled {
			m.state = StateAWSConfigPath
			m.textInput.SetValue("~/.aws/config")
			m.textInput.Placeholder = "AWS Config File Path"
		} else {
			m.state = StateTeamProfileName
			m.textInput.Placeholder = "Profile Name"
		}

	case StateAWSConfigPath:
		val := m.textInput.Value()
		if val == "" {
			val = "~/.aws/config"
		}
		m.awsConfigPath = ExpandTilde(val)
		m.state = StateAWSProfile
		m.textInput.Reset()
		m.textInput.SetValue("default")
		m.textInput.Placeholder = "AWS Profile Name"

	case StateAWSProfile:
		val := m.textInput.Value()
		if val == "" {
			val = "default"
		}
		m.awsProfile = val
		m.config.EKS.AWSProfile = m.awsProfile
		if m.infraType == "eks" {
			m.state = StateAWSRegion
			m.textInput.SetValue("us-east-1")
			m.textInput.Placeholder = "AWS Region"
		} else {
			m.state = StateTeamProfileName
			m.textInput.Reset()
			m.textInput.Placeholder = "Profile Name (e.g., prod-team)"
		}

	case StateAWSRegion:
		m.awsRegion = m.textInput.Value()
		m.config.EKS.Region = m.awsRegion
		m.state = StateTeamProfileName
		m.textInput.Reset()
		m.textInput.Placeholder = "Profile Name (e.g., prod-team)"

	case StateTeamProfileName:
		if m.textInput.Value() == "" {
			m.err = fmt.Errorf("profile name cannot be empty")
			return m, nil
		}
		m.profileName = m.textInput.Value()
		// Apply preset defaults to config
		m.applyPresetDefaults()
		m.state = StateTeamPrometheusURL
		m.textInput.SetValue(m.config.PrometheusURL)
		m.textInput.Placeholder = "http://prometheus:9090"

	case StateTeamPrometheusURL:
		m.config.PrometheusURL = m.textInput.Value()
		if len(m.conflicts) > 0 {
			m.state = StateTeamConflictWarning
			m.textInput.Reset()
			m.textInput.Placeholder = "y/n"
		} else {
			m.state = StateTeamPrometheusAuth
			m.textInput.SetValue("")
			m.textInput.Placeholder = "Auth (token or user:pass, leave empty if none)"
		}
		return m, nil

	case StateTeamConflictWarning:
		val := strings.ToLower(m.textInput.Value())
		if val == "n" || val == "no" {
			m.state = StateTeamPrometheusURL
			m.textInput.SetValue(m.config.PrometheusURL)
			return m, nil
		}
		m.state = StateTeamPrometheusAuth
		m.textInput.Reset()
		m.textInput.Placeholder = "Auth (token or user:pass, leave empty if none)"
		return m, nil

	case StateTeamPrometheusAuth:
		val := m.textInput.Value()
		if strings.Contains(val, ":") {
			parts := strings.SplitN(val, ":", 2)
			m.username = parts[0]
			m.password = parts[1]
			m.config.PrometheusToken = ""
		} else {
			m.config.PrometheusToken = val
			m.username = ""
			m.password = ""
		}
		m.state = StateTeamLokiURL
		m.textInput.SetValue(m.config.LokiURL)
		m.textInput.Placeholder = "http://loki:3100 (leave empty to skip)"

	case StateTeamLokiURL:
		m.config.LokiURL = m.textInput.Value()
		if m.config.LokiURL == "" {
			m.state = StateTeamGrafanaURL
		} else {
			m.state = StateTeamLokiAuth
			m.textInput.SetValue("")
			m.textInput.Placeholder = "Auth (token or user:pass, leave empty if none)"
		}

	case StateTeamLokiAuth:
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
		m.state = StateTeamGrafanaURL
		m.textInput.SetValue(m.config.GrafanaURL)
		m.textInput.Placeholder = "http://grafana:3000 (leave empty to skip)"

	case StateTeamGrafanaURL:
		m.config.GrafanaURL = m.textInput.Value()
		if m.config.GrafanaURL == "" {
			m.state = StateTeamElasticURL
			m.textInput.SetValue(m.config.ElasticURL)
			m.textInput.Placeholder = "http://elasticsearch:9200 (leave empty to skip)"
		} else {
			m.state = StateTeamGrafanaAuth
			m.textInput.SetValue("")
			m.textInput.Placeholder = "Auth (token or user:pass, leave empty if none)"
		}

	case StateTeamGrafanaAuth:
		val := m.textInput.Value()
		if strings.Contains(val, ":") {
			parts := strings.SplitN(val, ":", 2)
			m.config.GrafanaUser = parts[0]
			m.config.GrafanaPass = parts[1]
			m.config.GrafanaToken = ""
		} else {
			m.config.GrafanaToken = val
			m.config.GrafanaUser = ""
			m.config.GrafanaPass = ""
		}
		m.state = StateTeamElasticURL
		m.textInput.SetValue(m.config.ElasticURL)
		m.textInput.Placeholder = "http://elasticsearch:9200 (leave empty to skip)"

	case StateTeamElasticURL:
		m.config.ElasticURL = m.textInput.Value()
		if m.config.ElasticURL == "" {
			m.state = StateTeamTracingURL
		} else {
			m.state = StateTeamElasticAuth
			m.textInput.SetValue("")
			m.textInput.Placeholder = "Auth (token or user:pass, leave empty if none)"
		}

	case StateTeamElasticAuth:
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
		m.state = StateTeamElasticIndex
		m.textInput.SetValue(m.config.ElasticIndex)
		m.textInput.Placeholder = "Index name (e.g., logs-*, leave empty for *)"

	case StateTeamElasticIndex:
		m.config.ElasticIndex = m.textInput.Value()
		m.state = StateTeamTracingURL
		m.textInput.SetValue(m.config.TraceURL)
		m.textInput.Placeholder = "http://tempo:3200 (leave empty to skip)"

	case StateTeamTracingURL:
		m.config.TraceURL = m.textInput.Value()
		if m.config.TraceURL == "" {
			m.state = StateTeamNotificationsSlack
		} else {
			m.state = StateTeamTracingAuth
			m.textInput.SetValue("")
			m.textInput.Placeholder = "Auth (token or user:pass, leave empty if none)"
		}

	case StateTeamTracingAuth:
		val := m.textInput.Value()
		if strings.Contains(val, ":") {
			parts := strings.SplitN(val, ":", 2)
			m.config.TraceUser = parts[0]
			m.config.TracePass = parts[1]
			m.config.TraceToken = ""
		} else {
			m.config.TraceToken = val
			m.config.TraceUser = ""
			m.config.TracePass = ""
		}
		m.state = StateTeamNotificationsSlack
		m.textInput.SetValue(m.slackWebhook)
		m.textInput.Placeholder = "https://hooks.slack.com/... (leave empty to skip)"

	case StateTeamNotificationsSlack:
		m.slackWebhook = m.textInput.Value()
		m.state = StateTeamNotificationsPagerDuty
		m.textInput.SetValue(m.pagerDutyKey)
		m.textInput.Placeholder = "PagerDuty Routing Key (leave empty to skip)"

	case StateTeamNotificationsPagerDuty:
		m.pagerDutyKey = m.textInput.Value()
		m.state = StateTeamConnectivityTest
		return m, m.testConnectivity()

	case StateTeamConnectivityTest:
		// If there are errors (indicated by ❌), we don't automatically proceed
		if strings.Contains(m.testResult, "❌") {
			// Stay here until user explicitly decides to skip or fix
			return m, nil
		}

		// If Prometheus is connected, try to sample metrics
		if strings.Contains(m.testResult, "✅ Prometheus") {
			m.state = StateTeamPrometheusSampling
			return m, m.samplePrometheusMetrics()
		}
		m.state = StateTeamDone

	case StateTeamPrometheusSampling:
		m.state = StateTeamDone

	case StateTeamDone:
		m.quitting = true
		return m, tea.Quit
	}

	return m, nil
}

func (m *TeamWizardModel) applyPresetDefaults() {
	m.config.PrometheusServiceLabel = m.selectedPreset.Profile.Config.PrometheusServiceLabel
	m.config.LokiServiceLabel = m.selectedPreset.Profile.Config.LokiServiceLabel
	m.config.LokiErrorRegex = m.selectedPreset.Profile.Config.LokiErrorRegex

	// Set provider based on infra type
	if m.infraType == "eks" {
		m.config.Provider = "eks"
		m.config.EKS.Enabled = true
	} else if m.infraType == "kubernetes" {
		m.config.Provider = "kubernetes"
		m.config.EKS.Enabled = true
	} else {
		m.config.Provider = "bare-metal"
		m.config.EKS.Enabled = false
	}

	// Apply predefined capacity from preset
	m.config.TeamCapacity = m.selectedPreset.Capacity
}
func (m *TeamWizardModel) handleArrowKey(keyType tea.KeyType) (tea.Model, tea.Cmd) {
	if m.state == StatePresetSelection {
		if keyType == tea.KeyUp {
			m.selectedPresetIndex--
			if m.selectedPresetIndex < 0 {
				m.selectedPresetIndex = len(m.presets) - 1
			}
		} else {
			m.selectedPresetIndex++
			if m.selectedPresetIndex >= len(m.presets) {
				m.selectedPresetIndex = 0
			}
		}
		if len(m.presets) > 0 {
			m.selectedPreset = m.presets[m.selectedPresetIndex]
			m.err = nil // Clear error on selection
		}
	} else if m.state == StateInfraSelection {
		types := []string{"bare-metal", "kubernetes", "eks"}
		currentIndex := -1
		for i, t := range types {
			if t == m.infraType {
				currentIndex = i
				break
			}
		}

		if keyType == tea.KeyUp {
			currentIndex--
			if currentIndex < 0 {
				currentIndex = len(types) - 1
			}
		} else {
			currentIndex++
			if currentIndex >= len(types) {
				currentIndex = 0
			}
		}
		m.infraType = types[currentIndex]
	}
	return m, nil
}

func (m *TeamWizardModel) handleTab() (tea.Model, tea.Cmd) {
	if m.state == StatePresetSelection && m.selectedPreset.Name != "" {
		m.state = StatePresetConfirmation
	}
	return m, nil
}

type testResultMsg string

func (m *TeamWizardModel) isInputState() bool {
	switch m.state {
	case StateTeamProfileName, StateTeamPrometheusURL, StateTeamPrometheusAuth,
		StateTeamLokiURL, StateTeamLokiAuth, StateTeamGrafanaURL, StateTeamGrafanaAuth,
		StateTeamTracingURL, StateTeamTracingAuth, StateTeamNotificationsSlack,
		StateTeamNotificationsPagerDuty, StateTeamElasticURL, StateTeamElasticAuth, StateTeamElasticIndex, StateKubeConfig, StateKubeContextSelection, StateAWSConfigPath, StateAWSProfile, StateAWSRegion:
		return true
	}
	return false
}
func (m *TeamWizardModel) testConnectivity() tea.Cmd {
	m.testingConn = true
	m.testResult = "Testing connections..."

	return func() tea.Msg {
		var results []string

		// Test Prometheus
		if m.config.PrometheusURL != "" {
			client := prometheus.Client{
				BaseURL: m.config.PrometheusURL, 
				Token: m.config.PrometheusToken,
				User: m.username,
				Pass: m.password,
			}
			if _, err := client.Check(); err != nil {
				results = append(results, fmt.Sprintf("❌ Prometheus: %v", err))
			} else {
				results = append(results, "✅ Prometheus: Connected")
			}
		}

		// Test Loki
		if m.config.LokiURL != "" {
			client := loki.Client{BaseURL: m.config.LokiURL, Token: m.config.LokiToken}
			if _, err := client.Check(); err != nil {
				results = append(results, fmt.Sprintf("❌ Loki: %v", err))
			} else {
				results = append(results, "✅ Loki: Connected")
			}
		}

		// Test AWS CLI and Credentials if EKS
		if m.infraType == "eks" || m.config.EKS.Enabled {
			// Apply AWS Config paths and profiles before testing
			if m.awsConfigPath != "" {
				os.Setenv("AWS_CONFIG_FILE", m.awsConfigPath)
				os.Setenv("AWS_SHARED_CREDENTIALS_FILE", strings.Replace(m.awsConfigPath, "config", "credentials", 1))
			}
			if m.awsProfile != "" {
				os.Setenv("AWS_PROFILE", m.awsProfile)
			}
			
			importCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if _, err := exec.LookPath("aws"); err != nil {
				results = append(results, "❌ AWS CLI: Not found in path")
			} else if out, err := exec.CommandContext(importCtx, "aws", "sts", "get-caller-identity").CombinedOutput(); err != nil {
				outStr := string(out)
				if strings.Contains(outStr, "ExpiredToken") {
					results = append(results, "❌ AWS: Session expired. Run 'aws sso login' or 'aws configure'.")
				} else {
					results = append(results, "❌ AWS: Credentials not found. Run 'aws configure'.")
				}
			} else {
				results = append(results, "✅ AWS: Credentials verified")
			}
		}

		// Test Kubernetes/EKS connectivity and discover resources
		if m.config.EKS.Enabled {
			engine, err := discovery.NewDiscoveryEngine(discovery.K8sConfig{
				Enabled:    m.config.EKS.Enabled,
				KubeConfig: m.config.EKS.KubeConfig,
				Region:     m.config.EKS.Region,
			})
			if err != nil {
				results = append(results, fmt.Sprintf("❌ K8s/EKS: %v", err))
			} else {
				results = append(results, "✅ K8s: Connected")
				
				// Auto-discover namespaces
				namespaces, err := engine.DiscoverNamespaces()
				if err == nil && len(namespaces) > 0 {
					m.config.EKS.Namespaces = namespaces
					results = append(results, fmt.Sprintf("✅ Discovered %d namespaces", len(namespaces)))
				}

				// If EKS, discover cluster details
				if m.infraType == "eks" && m.config.EKS.Region != "" {
					// We need a cluster name, usually it's in the context or we can try to guess it
					clusterName := m.kubeContext
					if clusterName != "" {
						_, err := engine.DiscoverEKSCluster(clusterName)
						if err == nil {
							results = append(results, "✅ EKS Cluster Metadata: Retrieved")
						}
					}
				}
			}
		}

		// Test Slack
		if m.slackWebhook != "" {
			// Using Head request to just check reachability
			resp, err := http.Head(m.slackWebhook)
			if err != nil {
				results = append(results, fmt.Sprintf("❌ Slack: %v", err))
			} else {
				if resp.StatusCode < 500 {
					results = append(results, "✅ Slack: Reachable")
				} else {
					results = append(results, fmt.Sprintf("❌ Slack: Status %d", resp.StatusCode))
				}
				resp.Body.Close()
			}
		}

		// Test PagerDuty
		if m.pagerDutyKey != "" {
			resp, err := http.Head("https://events.pagerduty.com/v2/enqueue")
			if err != nil {
				results = append(results, fmt.Sprintf("❌ PagerDuty API: %v", err))
			} else {
				results = append(results, "✅ PagerDuty API: Reachable")
				resp.Body.Close()
			}
		}

		// Test Elasticsearch
		if m.config.ElasticURL != "" {
			// Basic connectivity check: endpoint + /_cluster/health or just GET endpoint
			req, err := http.NewRequest("GET", m.config.ElasticURL, nil)
			if err == nil {
				if m.config.ElasticToken != "" {
					req.Header.Set("Authorization", "Bearer "+m.config.ElasticToken)
				} else if m.config.ElasticUser != "" || m.config.ElasticPass != "" {
					req.SetBasicAuth(m.config.ElasticUser, m.config.ElasticPass)
				}
				client := &http.Client{Timeout: 5 * time.Second}
				resp, err := client.Do(req)
				if err != nil {
					results = append(results, fmt.Sprintf("❌ Elasticsearch: %v", err))
				} else {
					if resp.StatusCode < 400 {
						results = append(results, "✅ Elasticsearch: Reachable")
					} else {
						results = append(results, fmt.Sprintf("❌ Elasticsearch: Status %d", resp.StatusCode))
					}
					resp.Body.Close()
				}
			}
		}

		if len(results) == 0 {
			return testResultMsg("No backends configured to test.")
		}

		return testResultMsg(strings.Join(results, "\n"))
	}
}

type samplingResultMsg string

func (m *TeamWizardModel) samplePrometheusMetrics() tea.Cmd {
	return func() tea.Msg {
		client := prometheus.Client{
			BaseURL: m.config.PrometheusURL,
			Token:   m.config.PrometheusToken,
			User:    m.username,
			Pass:    m.password,
			Timeout: 5 * time.Second,
		}

		if label, err := client.DiscoverServiceLabel(); err == nil {
			return samplingResultMsg(label)
		}

		return samplingResultMsg("")
	}
}


// View renders the team wizard UI
func (m *TeamWizardModel) View() string {
	if m.quitting {
		return ""
	}

	var sb strings.Builder

	// Header
	sb.WriteString(cliTitleStyle.Render("Team Setup Wizard"))
	sb.WriteString("\n\n")

	// Progress indicator (Text based)
	progress := m.getProgressText()
	sb.WriteString(cliDimStyle.Render(progress) + "\n\n")

	// Main content based on current state
	switch m.state {
	case StateTeamIntro:
		sb.WriteString(m.renderTeamIntro())
	case StateTeamSize:
		sb.WriteString(m.renderTeamSize())
	case StatePresetSelection:
		sb.WriteString(m.renderPresetSelection())
	case StatePresetConfirmation:
		sb.WriteString(m.renderPresetConfirmation())
	case StateTeamProfileName:
		sb.WriteString(m.renderProfileName())
	case StateTeamPrometheusURL:
		sb.WriteString(m.renderInputStep("Prometheus URL", "Enter the endpoint of your Prometheus server.", "http://localhost:9090"))
	case StateTeamPrometheusAuth:
		sb.WriteString(m.renderInputStep("Prometheus Authentication", "Enter token or user:pass (optional).", "Auth (token or user:pass)"))
	case StateTeamLokiURL:
		sb.WriteString(m.renderInputStep("Loki URL", "Enter the endpoint of your Loki server (optional).", "http://localhost:3100"))
	case StateTeamLokiAuth:
		sb.WriteString(m.renderInputStep("Loki Authentication", "Enter token or user:pass (optional).", "Auth (token or user:pass)"))
	case StateTeamGrafanaURL:
		sb.WriteString(m.renderInputStep("Grafana URL", "Enter your Grafana endpoint (optional).", "http://localhost:3000"))
	case StateTeamGrafanaAuth:
		sb.WriteString(m.renderInputStep("Grafana Authentication", "Enter token or API Key or user:pass (optional).", "Auth (token or user:pass)"))
	case StateTeamTracingURL:
		backend := "Tracing"
		if m.config.TraceBackend == "jaeger" {
			backend = "Jaeger"
		} else if m.config.TraceBackend == "tempo" {
			backend = "Tempo"
		}
		placeholder := "http://localhost:3200"
		if m.config.TraceBackend == "jaeger" {
			placeholder = "http://localhost:16686"
		}
		sb.WriteString(m.renderInputStep(fmt.Sprintf("%s URL", backend), "Enter your trace backend endpoint (optional).", placeholder))
	case StateTeamTracingAuth:
		backend := "Tracing"
		if m.config.TraceBackend == "jaeger" {
			backend = "Jaeger"
		} else if m.config.TraceBackend == "tempo" {
			backend = "Tempo"
		}
		sb.WriteString(m.renderInputStep(fmt.Sprintf("%s Authentication", backend), "Enter token or user:pass (optional).", "Auth (token or user:pass)"))
	case StateTeamNotificationsSlack:
		sb.WriteString(m.renderInputStep("Slack Notifications", "Enter your Slack Webhook URL (optional).", "https://hooks.slack.com/..."))
	case StateTeamNotificationsPagerDuty:
		sb.WriteString(m.renderInputStep("PagerDuty Notifications", "Enter your PagerDuty Routing Key (optional).", "Routing Key"))
	case StateTeamElasticURL:
		sb.WriteString(m.renderInputStep("Elasticsearch URL", "Enter your Elasticsearch endpoint (optional).", "http://localhost:9200"))
	case StateTeamElasticAuth:
		sb.WriteString(m.renderInputStep("Elasticsearch Authentication", "Enter credentials (token or user:pass, optional).", "Auth"))
	case StateTeamElasticIndex:
		sb.WriteString(m.renderInputStep("Elasticsearch Index", "Enter your log index pattern (e.g., logs-*, optional).", "*"))
	case StateTeamConnectivityTest:
		sb.WriteString(m.renderConnectivityTest())
	case StateTeamConflictWarning:
		sb.WriteString(m.renderConflictWarning())
	case StateTeamPrometheusSampling:
		sb.WriteString(m.renderPrometheusSampling())
	case StateInfraSelection:
		sb.WriteString(m.renderInfraSelection())
	case StateKubeConfig:
		sb.WriteString(m.renderInputStep("Kubernetes Configuration", "Enter path to your kubeconfig file.", "~/.kube/config"))
	case StateKubeContextSelection:
		sb.WriteString(m.renderInputStep("Kubernetes Context", "Enter the cluster context name to monitor.", "default"))
	case StateAWSConfigPath:
		sb.WriteString(m.renderInputStep("AWS Configuration", "Enter path to your aws config file.", "~/.aws/config"))
	case StateAWSProfile:
		sb.WriteString(m.renderInputStep("AWS Profile", "Enter the AWS profile name to use.", "default"))
	case StateAWSRegion:
		sb.WriteString(m.renderInputStep("AWS Region", "Enter the region of your EKS cluster.", "us-east-1"))
	case StateQuickStart:
		sb.WriteString(m.renderQuickStart())
	case StateTeamDone:
		sb.WriteString(m.renderDone())
	}

	// Error message
	if m.err != nil {
		sb.WriteString("\n" + cliErrorStyle.Render("Error: "+m.err.Error()))
	}

	// Footer
	sb.WriteString("\n\n" + cliDimStyle.Render(m.getFooter()))

	// Wrap in a rounded box
	return cliBoxStyle.Render(sb.String()) + "\n"
}

func (m *TeamWizardModel) getProgressText() string {
	steps := []string{"Intro", "Preset", "Profile", "Backends", "Notify", "Done"}
	currentStep := 0

	switch m.state {
	case StateTeamIntro, StateTeamSize:
		currentStep = 0
	case StatePresetSelection, StatePresetConfirmation:
		currentStep = 1
	case StateTeamProfileName:
		currentStep = 2
	case StateTeamPrometheusURL, StateTeamConflictWarning, StateTeamPrometheusAuth,
		StateTeamLokiURL, StateTeamLokiAuth, StateTeamGrafanaURL, StateTeamGrafanaAuth,
		StateTeamTracingURL, StateTeamTracingAuth:
		currentStep = 3
	case StateTeamNotificationsSlack, StateTeamNotificationsPagerDuty:
		currentStep = 4
	case StateKubeConfig, StateKubeContextSelection, StateAWSConfigPath, StateAWSProfile, StateAWSRegion, StateInfraSelection:
		currentStep = 1 // Mapping to Preset/Infra Phase
	case StateTeamConnectivityTest, StateQuickStart, StateTeamDone:
		currentStep = 5
	}

	var progress []string
	for i, step := range steps {
		if i < currentStep {
			progress = append(progress, "[x] "+step)
		} else if i == currentStep {
			progress = append(progress, "> "+step)
		} else {
			progress = append(progress, "[ ] "+step)
		}
	}

	return strings.Join(progress, "  ")
}

// renderTeamIntro renders the introduction screen
func (m *TeamWizardModel) renderTeamIntro() string {
	var sb strings.Builder
	sb.WriteString(cliLabelStyle.Render("Welcome to Health Monitor!") + "\n\n")
	sb.WriteString("Health Monitor provides production-grade monitoring for teams of all sizes.\n")
	sb.WriteString("This wizard will help you set up a complete monitoring configuration in minutes.\n\n")
	sb.WriteString(cliLabelStyle.Render("What you'll get:") + "\n")
	sb.WriteString("• Team-specific preset configuration\n")
	sb.WriteString("• Pre-configured service flows and SLOs\n")
	sb.WriteString("• Intelligent alert rules\n")
	sb.WriteString("• Isolated state management\n")
	sb.WriteString("• Team-specific best practices\n\n")
	sb.WriteString("Let's start by understanding your team size and needs.")
	return sb.String()
}

// renderTeamSize renders the team size selection
func (m *TeamWizardModel) renderTeamSize() string {
	var sb strings.Builder
	sb.WriteString(cliLabelStyle.Render("How big is your team?") + "\n\n")
	sb.WriteString("Choose your team size to get the most appropriate preset configuration:\n\n")
	sb.WriteString("• Small (2-5 people)\n")
	sb.WriteString("  Simple microservices architecture\n")
	sb.WriteString("• Medium (5-15 people)\n")
	sb.WriteString("  Multiple services with dependencies\n")
	sb.WriteString("• Large (15-50 people)\n")
	sb.WriteString("  Complex microservices architecture\n")
	sb.WriteString("• Enterprise (50+ people)\n")
	sb.WriteString("  Enterprise-grade security and compliance\n")
	return sb.String()
}

// renderPresetSelection renders the preset selection screen
func (m *TeamWizardModel) renderPresetSelection() string {
	var sb strings.Builder
	sb.WriteString(cliLabelStyle.Render("Choose your team preset") + "\n\n")
	sb.WriteString("Select the preset that best matches your team:\n\n")

	presets := GetAvailablePresets()
	for _, preset := range presets {
		marker := "  "
		if preset.Name == m.selectedPreset.Name {
			marker = "> "
		}

		style := lipgloss.NewStyle()
		if preset.Name == m.selectedPreset.Name {
			style = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
		}

		sb.WriteString(style.Render(fmt.Sprintf("%s%s", marker, preset.Name)) + "\n")
		sb.WriteString(cliDimStyle.Render(fmt.Sprintf("  %s", preset.Description)) + "\n")
		sb.WriteString(cliDimStyle.Render(fmt.Sprintf("  Size: %s | Services: %d | Alerts: %d", 
			preset.TeamSize, len(preset.Flows), len(preset.Alerts))) + "\n\n")
	}

	return sb.String()
}

// renderPresetConfirmation renders the preset confirmation screen
func (m *TeamWizardModel) renderPresetConfirmation() string {
	var sb strings.Builder
	sb.WriteString(cliLabelStyle.Render("Preset Summary: ")+m.selectedPreset.Name+"\n\n")
	sb.WriteString(fmt.Sprintf("Team Size: %s\n", m.selectedPreset.TeamSize))
	sb.WriteString(fmt.Sprintf("Complexity: %s\n", m.selectedPreset.Complexity))
	sb.WriteString(fmt.Sprintf("Description: %s\n\n", m.selectedPreset.Description))
	sb.WriteString(cliLabelStyle.Render("What's included:") + "\n")
	sb.WriteString(fmt.Sprintf("• %d pre-configured service flows\n", len(m.selectedPreset.Flows)))
	sb.WriteString(fmt.Sprintf("• %d intelligent alert rules\n", len(m.selectedPreset.Alerts)))
	sb.WriteString("• Optimized configuration settings\n")
	sb.WriteString("• Team-specific best practices guide\n\n")
	sb.WriteString("This preset includes everything you need to get started.")
	return sb.String()
}

// renderProfileName renders the profile name input screen
func (m *TeamWizardModel) renderProfileName() string {
	var sb strings.Builder
	sb.WriteString(cliLabelStyle.Render("Name your profile") + "\n\n")
	sb.WriteString("Your profile identifies your configuration and separates your data.\n")
	sb.WriteString("Example: 'my-team', 'platform', 'production'\n\n")
	sb.WriteString(m.textInput.View())
	return sb.String()
}

// renderInputStep renders a generic input screen
func (m *TeamWizardModel) renderInputStep(title, description, placeholder string) string {
	var sb strings.Builder
	sb.WriteString(cliLabelStyle.Render(title) + "\n\n")
	sb.WriteString(description + "\n\n")
	sb.WriteString(m.textInput.View())
	return sb.String()
}

// renderConnectivityTest renders the connectivity test screen
func (m *TeamWizardModel) renderConnectivityTest() string {
	var sb strings.Builder
	sb.WriteString(cliLabelStyle.Render("Testing Connectivity") + "\n\n")

	if m.testingConn {
		sb.WriteString("Testing connections...\n")
		sb.WriteString(cliDimStyle.Render("Please wait (this can take up to 30 seconds)...") + "\n")
		return sb.String()
	}

	if m.testResult != "" {
		// Clean up common error messages for better display
		result := m.testResult
		result = strings.ReplaceAll(result, "loki http error: status=503", "❌ Loki: Service Unavailable (503) - Check if Loki is ready")
		sb.WriteString(result + "\n\n")

		if strings.Contains(m.testResult, "❌") || strings.Contains(m.testResult, "Failed") {
			sb.WriteString(cliLabelStyle.Render("Some checks failed!") + "\n")
			sb.WriteString("Press 'r' to retry, 's' to skip if this is expected, or 'esc' to fix config.\n")
		} else {
			sb.WriteString(cliSuccessStyle.Render("All checks passed! Press enter to continue.") + "\n")
		}
	} else {
		sb.WriteString("Verifying backend connections...\n")
	}
	return sb.String()
}

// renderQuickStart renders the quick start screen (placeholder if still needed)
func (m *TeamWizardModel) renderQuickStart() string {
	return "Quick Start configuration applied."
}

// renderDone renders the completion screen
func (m *TeamWizardModel) renderDone() string {
	var sb strings.Builder
	sb.WriteString(cliSuccessStyle.Render("Success! Setup Complete") + "\n\n")
	sb.WriteString(fmt.Sprintf("Profile '%s' created with '%s' preset.\n\n", m.profileName, m.selectedPreset.Name))
	
	sb.WriteString(cliLabelStyle.Render("Created Files:") + "\n")
	sb.WriteString(fmt.Sprintf("• Profile: /etc/health-monitor/profiles/%s.yaml\n", m.profileName))
	if len(m.selectedPreset.Flows) > 0 {
		sb.WriteString(fmt.Sprintf("• Flows:   /etc/health-monitor/flows.d/%s.yaml\n", m.profileName))
	}
	if len(m.selectedPreset.Alerts) > 0 {
		sb.WriteString(fmt.Sprintf("• Alerts:  /etc/health-monitor/alerts.d/%s.yaml\n", m.profileName))
	}
	sb.WriteString("\n")

	sb.WriteString(cliDimStyle.Render("Note: All configurations are editable at any time.") + "\n")
	sb.WriteString(cliDimStyle.Render("You can manually tweak the YAML files or use the CLI.") + "\n\n")

	sb.WriteString(cliLabelStyle.Render("Next steps:") + "\n")
	sb.WriteString(fmt.Sprintf("1. sudo health-monitor --profile %s\n", m.profileName))
	sb.WriteString("2. sudo health-monitor profile show\n")
	sb.WriteString("3. sudo health-monitor flow list\n")
	return sb.String()
}

// getFooter returns appropriate footer text
func (m *TeamWizardModel) getFooter() string {
	switch m.state {
	case StatePresetSelection:
		return "arrows: navigate | tab: confirm | enter: next | esc: exit"
	case StateTeamConflictWarning:
		return "y: proceed | n: back | esc: exit"
	case StateTeamDone:
		return "press enter to finish"
	default:
		return "enter: continue | esc: exit"
	}
}

// renderInfraSelection renders the infrastructure type selection screen
func (m *TeamWizardModel) renderInfraSelection() string {
	var sb strings.Builder
	sb.WriteString(cliLabelStyle.Render("Choose your infrastructure type") + "\n\n")
	sb.WriteString("Select how your services are hosted to optimize monitoring:\n\n")

	options := []struct {
		ID    string
		Label string
		Desc  string
	}{
		{"bare-metal", "🏠 Bare Metal / VM", "Traditional hosting on physical or virtual servers."},
		{"kubernetes", "☸️  Generic Kubernetes", "Self-managed Kubernetes clusters on any cloud/site."},
		{"eks", "☁️  Amazon EKS", "Managed Kubernetes on AWS with CloudWatch integration."},
	}

	for _, opt := range options {
		marker := "  "
		style := lipgloss.NewStyle()
		if opt.ID == m.infraType {
			marker = "> "
			style = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
		}
		sb.WriteString(style.Render(fmt.Sprintf("%s%s", marker, opt.Label)) + "\n")
		sb.WriteString(cliDimStyle.Render(fmt.Sprintf("  %s", opt.Desc)) + "\n\n")
	}

	return sb.String()
}

// renderPrometheusSampling renders the sampling screen
func (m *TeamWizardModel) renderPrometheusSampling() string {
	return "Sampling Prometheus metrics to suggest best defaults..."
}

func (m *TeamWizardModel) checkPrometheusConflicts(url string) []string {
	var conflicts []string
	pm := GetProfileManager()
	profiles := pm.ListProfiles()
	
	for _, name := range profiles {
		if name == m.profileName {
			continue
		}
		// Load actual config to check URL
		cfg, err := LoadForProfile(name)
		if err == nil && cfg.PrometheusURL == url {
			conflicts = append(conflicts, name)
		}
	}
	return conflicts
}

// TeamWizardPrefs allows pre-populating the wizard from CLI
type TeamWizardPrefs struct {
	InfraType  string
	KubeConfig string
	Region     string
}

// RunTeamWizard starts the team setup wizard
func RunTeamWizard() (Config, []string, map[string]model.ServiceMetadata, error) {
	return RunTeamWizardWithPrefs(TeamWizardPrefs{})
}

// RunTeamWizardWithPrefs starts the team setup wizard with initial preferences
func RunTeamWizardWithPrefs(prefs TeamWizardPrefs) (Config, []string, map[string]model.ServiceMetadata, error) {
	m := NewTeamWizard()
	if prefs.InfraType != "" {
		m.infraType = prefs.InfraType
		m.config.EKS.Enabled = (prefs.InfraType == "eks" || prefs.InfraType == "kubernetes")
	}
	if prefs.KubeConfig != "" {
		m.kubeConfig = prefs.KubeConfig
		m.config.EKS.KubeConfig = prefs.KubeConfig
	}
	if prefs.Region != "" {
		m.awsRegion = prefs.Region
		m.config.EKS.Region = prefs.Region
	}

	p := tea.NewProgram(m)
	tm, err := p.Run()
	if err != nil {
		return Config{}, nil, nil, fmt.Errorf("failed to run team wizard: %w", err)
	}

	if wizardModel, ok := tm.(*TeamWizardModel); ok {
		if wizardModel.profileName != "" && wizardModel.selectedPreset.Name != "" {
			// Update notifications in config
			if wizardModel.slackWebhook != "" {
				wizardModel.config.Notifications.Slack.WebhookURL = wizardModel.slackWebhook
				wizardModel.config.Notifications.Slack.Enabled = true
				wizardModel.config.Notifications.Enabled = true
			}
			if wizardModel.pagerDutyKey != "" {
				wizardModel.config.Notifications.PagerDuty.RoutingKey = wizardModel.pagerDutyKey
				wizardModel.config.Notifications.PagerDuty.Enabled = true
				wizardModel.config.Notifications.Enabled = true
			}
			if wizardModel.username != "" && wizardModel.password != "" {
				wizardModel.config.PrometheusUser = wizardModel.username
				wizardModel.config.PrometheusPass = wizardModel.password
			}

			// Apply EKS/K8s configuration
			if wizardModel.infraType == "eks" || wizardModel.infraType == "kubernetes" {
				wizardModel.config.EKS.Enabled = true
				if wizardModel.infraType == "eks" {
					wizardModel.config.EKS.CloudWatchEnabled = true
				}
			}

			// Save the profile using the official SaveNewProfile which handles secrets
			if err := SaveNewProfile(wizardModel.profileName, wizardModel.config); err != nil {
				return Config{}, nil, nil, fmt.Errorf("failed to save profile: %w", err)
			}
			
			// Additional logic to save flows and alerts from preset
			pm := GetProfileManager()
			if len(wizardModel.selectedPreset.Flows) > 0 {
				flowsPath := filepath.Join(pm.GetFlowsPath(), wizardModel.profileName+".yaml")
				if err := saveFlowsToFile(flowsPath, wizardModel.selectedPreset.Flows); err != nil {
					return Config{}, nil, nil, fmt.Errorf("failed to save flows: %w", err)
				}
			}
			if len(wizardModel.selectedPreset.Alerts) > 0 {
				alertsPath := filepath.Join(pm.GetAlertsPath(), wizardModel.profileName+".yaml")
				if err := saveAlertsToFile(alertsPath, wizardModel.selectedPreset.Alerts); err != nil {
					return Config{}, nil, nil, fmt.Errorf("failed to save alerts: %w", err)
				}
			}

			// Print success message
			fmt.Printf("\n✅ Successfully created profile '%s' with preset '%s'\n", 
				wizardModel.profileName, wizardModel.selectedPreset.Name)
			
			fmt.Printf("\nCreated Configuration:\n")
			fmt.Printf("• Profile: /etc/health-monitor/profiles/%s.yaml\n", wizardModel.profileName)
			if len(wizardModel.selectedPreset.Flows) > 0 {
				fmt.Printf("• Flows:   /etc/health-monitor/flows.d/%s.yaml\n", wizardModel.profileName)
			}
			if len(wizardModel.selectedPreset.Alerts) > 0 {
				fmt.Printf("• Alerts:  /etc/health-monitor/alerts.d/%s.yaml\n", wizardModel.profileName)
			}
			statePath := pm.GetStatePathForProfile(wizardModel.profileName)
			if wizardModel.config.PrometheusToken != "" {
				fmt.Printf("• Prom Token: %s/prometheus.token\n", statePath)
			}
			if wizardModel.username != "" {
				fmt.Printf("• Prom Auth: %s/prometheus.basic\n", statePath)
			}
			if wizardModel.config.LokiToken != "" {
				fmt.Printf("• Loki Token: %s/loki.token\n", statePath)
			}
			if wizardModel.slackWebhook != "" {
				fmt.Printf("• Slack Webhook: %s/slack.webhook\n", statePath)
			}
			if wizardModel.pagerDutyKey != "" {
				fmt.Printf("• PagerDuty Key: %s/pagerduty.key\n", statePath)
			}

			fmt.Printf("\nNext steps:\n")
			fmt.Printf("1. Review your configuration:\n")
			fmt.Printf("   sudo health-monitor --profile %s profile show\n", wizardModel.profileName)
			fmt.Printf("2. Start monitoring:\n")
			fmt.Printf("   sudo health-monitor --profile %s\n", wizardModel.profileName)
			fmt.Printf("3. Review your flows:\n")
			fmt.Printf("   sudo health-monitor --profile %s flow list\n", wizardModel.profileName)
		}
	}

	wizardModel := tm.(*TeamWizardModel)
	return wizardModel.config, wizardModel.services, nil, nil
}


// ListTeamPresets displays all available team presets with detailed information
func ListTeamPresets() {
	presets := GetAvailablePresets()
	
	fmt.Println("🚀 Health Monitor - Team Presets")
	fmt.Println("================================")
	fmt.Println()
	
	for _, preset := range presets {
		fmt.Printf("📋 %s\n", preset.Name)
		fmt.Printf("   Description: %s\n", preset.Description)
		fmt.Printf("   Team Size: %s\n", preset.TeamSize)
		fmt.Printf("   Complexity: %s\n", preset.Complexity)
		fmt.Printf("   Services: %d\n", len(preset.Flows))
		fmt.Printf("   Alerts: %d\n", len(preset.Alerts))
		
		if len(preset.Flows) > 0 {
			fmt.Printf("   Flows: ")
			var flowNames []string
			for flowName := range preset.Flows {
				flowNames = append(flowNames, flowName)
			}
			fmt.Printf("%s\n", strings.Join(flowNames, ", "))
		}
		
		fmt.Println()
	}
	
	fmt.Println("Usage:")
	fmt.Println("  sudo health-monitor --init --preset <preset-name>")
	fmt.Println("  sudo health-monitor wizard --team")
	fmt.Println()
	
	fmt.Println("Examples:")
	fmt.Println("  sudo health-monitor --init --preset small-team")
	fmt.Println("  sudo health-monitor --init --preset devops-team")
	fmt.Println("  sudo health-monitor --init --preset sre-team")
}

func (m *TeamWizardModel) renderConflictWarning() string {
	var sb strings.Builder
	sb.WriteString(cliErrorStyle.Render("⚠️  Configuration Conflict") + "\n\n")
	sb.WriteString(fmt.Sprintf("Another profile is already using this Prometheus URL:\n%s\n\n", m.config.PrometheusURL))
	sb.WriteString("Existing profiles:\n")
	for _, p := range m.conflicts {
		sb.WriteString(fmt.Sprintf("• %s\n", p))
	}
	sb.WriteString("\nDo you want to proceed and share this configuration? (y/n)")
	return sb.String()
}
