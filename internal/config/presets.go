package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"gopkg.in/yaml.v3"
)

// TeamPreset defines a template configuration for different team types
type TeamPreset struct {
	Name         string            `yaml:"name"`
	Description  string            `yaml:"description"`
	TeamSize     string            `yaml:"team_size"`     // small, medium, large, enterprise
	Complexity   string            `yaml:"complexity"`    // simple, moderate, complex
	Capacity     TeamCapacity      `yaml:"capacity"`      // NEW: Predefined capacity
	Profile      Profile           `yaml:"profile"`
	Flows        map[string]Flow   `yaml:"flows,omitempty"`
	Alerts       []AlertRule       `yaml:"alerts,omitempty"`
	QuickStart   QuickStartGuide   `yaml:"quick_start"`
}

// Flow represents a service flow configuration
type Flow struct {
	Name        string    `yaml:"name"`
	Description string    `yaml:"description"`
	Services    []string  `yaml:"services"`
	SLOs        []SLO     `yaml:"slos"`
}

// SLO represents a Service Level Objective
type SLO struct {
	ID          string  `yaml:"id"`
	Objective   float64 `yaml:"objective"`
	Window      string  `yaml:"window"`
	Type        string  `yaml:"type"`        // ratio, latency
	ErrorQuery  string  `yaml:"error_query,omitempty"`
	TotalQuery  string  `yaml:"total_query,omitempty"`
	LatencyQuery string `yaml:"latency_query,omitempty"`
	Threshold   float64 `yaml:"threshold,omitempty"`
	Description string  `yaml:"description"`
}

// AlertRule represents an alert configuration
type AlertRule struct {
	Match       map[string]string `yaml:"match"`
	Severity    string            `yaml:"severity"`
	Mode        string            `yaml:"mode"`
	Title       string            `yaml:"title"`
	Description string            `yaml:"description,omitempty"`
	Runbook     string            `yaml:"runbook,omitempty"`
}

// QuickStartGuide provides team-specific setup instructions
type QuickStartGuide struct {
	Prerequisites    []string          `yaml:"prerequisites"`
	SetupSteps       []SetupStep       `yaml:"setup_steps"`
	CommonWorkflows  []CommonWorkflow  `yaml:"common_workflows"`
	Troubleshooting  []TroubleshootingTip `yaml:"troubleshooting"`
	BestPractices    []string          `yaml:"best_practices"`
}

// SetupStep represents a setup instruction
type SetupStep struct {
	Step        int    `yaml:"step"`
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
	Command     string `yaml:"command,omitempty"`
	Expected    string `yaml:"expected,omitempty"`
}

// CommonWorkflow represents a frequent team operation
type CommonWorkflow struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Steps       []string `yaml:"steps"`
	Example     string   `yaml:"example"`
}

// TroubleshootingTip provides help for common issues
type TroubleshootingTip struct {
	Problem    string   `yaml:"problem"`
	Symptoms   []string `yaml:"symptoms"`
	Solutions  []string `yaml:"solutions"`
	Prevention []string `yaml:"prevention"`
}

// GetAvailablePresets returns all available team presets (hardcoded + external)
func GetAvailablePresets() []TeamPreset {
	presets := []TeamPreset{
		createSmallTeamPreset(),
		createMediumTeamPreset(),
		createLargeTeamPreset(),
		createEnterpriseTeamPreset(),
		createDevOpsTeamPreset(),
		createSRETeamPreset(),
	}

	// Load external presets
	external := loadExternalPresets()
	presets = append(presets, external...)

	return presets
}

func loadExternalPresets() []TeamPreset {
	var presets []TeamPreset
	paths := []string{
		"/etc/health-monitor/presets.d",
	}

	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".health-monitor", "presets.d"))
	}

	for _, dir := range paths {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() && (strings.HasSuffix(entry.Name(), ".yaml") || strings.HasSuffix(entry.Name(), ".yml")) {
				path := filepath.Join(dir, entry.Name())
				data, err := os.ReadFile(path)
				if err != nil {
					continue
				}

				var p TeamPreset
				if err := yaml.Unmarshal(data, &p); err == nil && p.Name != "" {
					presets = append(presets, p)
				}
			}
		}
	}
	return presets
}

// createSmallTeamPreset creates a preset for small teams (2-5 people)
func createSmallTeamPreset() TeamPreset {
	return TeamPreset{
		Name:        "small-team",
		Description: "Ideal for small teams (2-5 people) with simple microservices architecture",
		TeamSize:    "small",
		Complexity:  "simple",
		Profile: Profile{
			Name: "small-team",
			Config: Default(),
		},
		Capacity: TeamCapacity{
			TeamSize:                 5,
			OpsHoursPerWeekPerPerson: 40,
			TargetToilPercentage:     50,
		},
		Flows: map[string]Flow{
			"api": {
				Name:        "API Service",
				Description: "Main API endpoints",
				Services:    []string{"api_service"},
				SLOs: []SLO{
					{
						ID:          "api_success",
						Objective:   95,
						Window:      "5m",
						Type:        "ratio",
						ErrorQuery:  `http_requests_total{service="api_service",status!~"2.."}`,
						TotalQuery:  `http_requests_total{service="api_service"}`,
						Description: "Percentage of successful API requests",
					},
					{
						ID:            "api_latency",
						Objective:     90,
						Window:        "5m",
						Type:          "latency",
						LatencyQuery:  `histogram_quantile(0.90, rate(http_request_duration_seconds_bucket{service="api_service"}[5m]))`,
						Threshold:     1.0,
						Description:   "90th percentile latency under 1 second",
					},
				},
			},
		},
		Alerts: []AlertRule{
			{
				Match: map[string]string{
					"service": "api_service",
				},
				Severity:    "P2",
				Mode:        "auto",
				Title:       "API Service Alert",
				Description: "Alert for API service issues",
			},
		},
		QuickStart: QuickStartGuide{
			Prerequisites: []string{
				"Health Monitor installed",
				"Prometheus instance available",
				"Basic knowledge of your services",
			},
			SetupSteps: []SetupStep{
				{
					Step:        1,
					Title:       "Configure Prometheus",
					Description: "Set your Prometheus endpoint to start collecting metrics",
					Command:     "sudo health-monitor profile show",
					Expected:    "Shows current backend configuration",
				},
				{
					Step:        2,
					Title:       "Test Connectivity",
					Description: "Ensure Health Monitor can reach your Prometheus server",
					Command:     "sudo health-monitor config validate",
					Expected:    "Validation successful",
				},
				{
					Step:        3,
					Title:       "Start Monitoring",
					Description: "Launch the interactive dashboard",
					Command:     "sudo health-monitor",
					Expected:    "Dashboard opens with service health",
				},
			},
			CommonWorkflows: []CommonWorkflow{
				{
					Name:        "Daily Health Check",
					Description: "Quick morning check of service health",
					Steps:       []string{"Run health-monitor", "Check for any alerts", "Review incident list"},
					Example:     "sudo health-monitor --profile small-team",
				},
				{
					Name:        "Incident Response",
					Description: "Handle service incidents",
					Steps:       []string{"Start incident", "Add investigation notes", "Resolve when fixed"},
					Example:     "sudo health-monitor incident start --service api_service --severity P2 --title 'API latency spike'",
				},
			},
			Troubleshooting: []TroubleshootingTip{
				{
					Problem:  "No data showing in dashboard",
					Symptoms: []string{"Empty metrics", "No services detected"},
					Solutions: []string{
						"Check Prometheus URL is correct",
						"Verify service discovery labels",
						"Check network connectivity",
					},
					Prevention: []string{
						"Test Prometheus connection during setup",
						"Use --test-prom flag to validate",
					},
				},
			},
			BestPractices: []string{
				"Run health checks daily",
				"Set up alert notifications for critical services",
				"Document incidents for future reference",
				"Review SLO performance weekly",
			},
		},
	}
}

// createMediumTeamPreset creates a preset for medium teams (5-15 people)
func createMediumTeamPreset() TeamPreset {
	preset := createSmallTeamPreset()
	preset.Name = "medium-team"
	preset.Description = "Designed for medium teams (5-15 people) with multiple services"
	preset.TeamSize = "medium"
	preset.Complexity = "moderate"
	preset.Profile.Name = "medium-team"
	preset.Capacity = TeamCapacity{
		TeamSize:                 15,
		OpsHoursPerWeekPerPerson: 40,
		TargetToilPercentage:     50,
	}
	
	// Add more flows for medium teams
	preset.Flows["checkout"] = Flow{
		Name:        "Checkout Process",
		Description: "Complete checkout flow from cart to payment",
		Services:    []string{"api_service", "payment_service", "inventory_service"},
		SLOs: []SLO{
			{
				ID:          "checkout_success",
				Objective:   95,
				Window:      "5m",
				Type:        "ratio",
				ErrorQuery:  `http_requests_total{service="api_service",route="/checkout",status!~"2.."}`,
				TotalQuery:  `http_requests_total{service="api_service",route="/checkout"}`,
				Description: "Checkout success rate",
			},
		},
	}
	
	preset.Flows["user_auth"] = Flow{
		Name:        "User Authentication",
		Description: "User login and authentication flow",
		Services:    []string{"auth_service", "user_service"},
		SLOs: []SLO{
			{
				ID:          "auth_success",
				Objective:   99,
				Window:      "5m",
				Type:        "ratio",
				ErrorQuery:  `http_requests_total{service="auth_service",status!~"2.."}`,
				TotalQuery:  `http_requests_total{service="auth_service"}`,
				Description: "Authentication success rate",
			},
		},
	}
	
	// Add more alerts
	preset.Alerts = append(preset.Alerts, AlertRule{
		Match: map[string]string{
			"service": "payment_service",
		},
		Severity:    "P1",
		Mode:        "auto",
		Title:       "Payment Service Alert",
		Description: "Critical payment service issues",
	})
	
	// Update quick start for medium teams
	preset.QuickStart.SetupSteps = append(preset.QuickStart.SetupSteps, SetupStep{
		Step:        4,
		Title:       "Set up team notifications",
		Description: "Configure Slack or PagerDuty for team alerts",
		Command:     "sudo health-monitor notifications enable",
		Expected:    "Notifications configured",
	})
	
	preset.QuickStart.BestPractices = append(preset.QuickStart.BestPractices, 
		"Set up separate profiles for different environments",
		"Configure alert routing for different team members",
		"Use flow monitoring for business-critical processes",
		"Schedule regular incident reviews",
	)
	
	return preset
}

// createLargeTeamPreset creates a preset for large teams (15-50 people)
func createLargeTeamPreset() TeamPreset {
	preset := createMediumTeamPreset()
	preset.Name = "large-team"
	preset.Description = "Built for large teams (15-50 people) with complex microservices"
	preset.TeamSize = "large"
	preset.Complexity = "complex"
	preset.Profile.Name = "large-team"
	preset.Capacity = TeamCapacity{
		TeamSize:                 50,
		OpsHoursPerWeekPerPerson: 40,
		TargetToilPercentage:     50,
	}
	
	// Configure for larger scale
	preset.Profile.Config.RouteCardinalityLimit = 500
	preset.Profile.Config.ServiceCardinalityLimit = 200
	preset.Profile.Config.TopEndpoints = 10
	
	// Add enterprise flows
	preset.Flows["order_fulfillment"] = Flow{
		Name:        "Order Fulfillment",
		Description: "End-to-end order processing workflow",
		Services:    []string{"order_service", "inventory_service", "shipping_service", "notification_service"},
		SLOs: []SLO{
			{
				ID:          "order_success",
				Objective:   98,
				Window:      "10m",
				Type:        "ratio",
				ErrorQuery:  `http_requests_total{service="order_service",status!~"2.."}`,
				TotalQuery:  `http_requests_total{service="order_service"}`,
				Description: "Order processing success rate",
			},
		},
	}
	
	// Add more sophisticated alerts
	preset.Alerts = append(preset.Alerts, 
		AlertRule{
			Match: map[string]string{
				"service": "inventory_service",
			},
			Severity:    "P2",
			Mode:        "auto",
			Title:       "Inventory Service Alert",
			Description: "Inventory management issues",
		},
		AlertRule{
			Match: map[string]string{
				"service": "shipping_service",
			},
			Severity:    "P3",
			Mode:        "auto",
			Title:       "Shipping Service Alert",
			Description: "Shipping and logistics issues",
		},
	)
	
	// Enhanced quick start for large teams
	preset.QuickStart.SetupSteps = append(preset.QuickStart.SetupSteps, 
		SetupStep{
			Step:        5,
			Title:       "Configure multiple environments",
			Description: "Set up profiles for dev, staging, and production",
			Command:     "sudo health-monitor profile create --name production --from large-team",
			Expected:    "Production profile created",
		},
		SetupStep{
			Step:        6,
			Title:       "Set up alert listeners",
			Description: "Start background alert listeners for each environment",
			Command:     "sudo health-monitor alert-listen --background --addr :9095",
			Expected:    "Alert listener started",
		},
	)
	
	preset.QuickStart.BestPractices = append(preset.QuickStart.BestPractices,
		"Implement role-based access control",
		"Use separate monitoring instances per environment",
		"Set up automated runbook generation",
		"Configure integration with ticketing systems",
		"Monitor the monitoring system itself",
	)
	
	return preset
}

// createEnterpriseTeamPreset creates a preset for enterprise teams (50+ people)
func createEnterpriseTeamPreset() TeamPreset {
	preset := createLargeTeamPreset()
	preset.Name = "enterprise-team"
	preset.Description = "Enterprise-grade configuration for large organizations (50+ people)"
	preset.TeamSize = "enterprise"
	preset.Complexity = "complex"
	preset.Profile.Name = "enterprise-team"
	preset.Capacity = TeamCapacity{
		TeamSize:                 100,
		OpsHoursPerWeekPerPerson: 40,
		TargetToilPercentage:     50,
	}
	
	// Enterprise-grade configuration
	preset.Profile.Config.PrometheusQPS = 20
	preset.Profile.Config.LokiQPS = 10
	preset.Profile.Config.CorrelationBestEffort = true
	preset.Profile.Config.CorrelationMinSamples = 50
	preset.Profile.Config.CorrelationMaxLogs = 500
	preset.Profile.Config.ObservabilityLookbackMinutes = 30
	
	// Enable all security features
	preset.Profile.Config.Security = DefaultSecurityConfig()
	preset.Profile.Config.Security.Webhook.Auth.Enabled = true
	preset.Profile.Config.Security.Webhook.RateLimit.Enabled = true
	preset.Profile.Config.Security.Audit.Enabled = true
	
	// Add enterprise-specific flows
	preset.Flows["compliance"] = Flow{
		Name:        "Compliance Monitoring",
		Description: "Regulatory compliance and audit workflows",
		Services:    []string{"compliance_service", "audit_service", "security_service"},
		SLOs: []SLO{
			{
				ID:          "compliance_success",
				Objective:   99.9,
				Window:      "1h",
				Type:        "ratio",
				ErrorQuery:  `compliance_checks_total{status="failed"}`,
				TotalQuery:  `compliance_checks_total`,
				Description: "Compliance check success rate",
			},
		},
	}
	
	// Enterprise alerts with runbooks
	preset.Alerts = append(preset.Alerts, 
		AlertRule{
			Match: map[string]string{
				"service": "compliance_service",
			},
			Severity:    "P1",
			Mode:        "auto",
			Title:       "Compliance Service Alert",
			Description: "Critical compliance issues",
			Runbook:     "https://company-wiki/runbooks/compliance",
		},
		AlertRule{
			Match: map[string]string{
				"service": "security_service",
			},
			Severity:    "P0",
			Mode:        "auto",
			Title:       "Security Service Alert",
			Description: "Security-related incidents",
			Runbook:     "https://company-wiki/runbooks/security",
		},
	)
	
	// Enterprise quick start
	preset.QuickStart.SetupSteps = append(preset.QuickStart.SetupSteps,
		SetupStep{
			Step:        7,
			Title:       "Configure security settings",
			Description: "Set up authentication, rate limiting, and audit logging",
			Command:     "sudo health-monitor security configure",
			Expected:    "Security settings configured",
		},
		SetupStep{
			Step:        8,
			Title:       "Set up compliance monitoring",
			Description: "Configure compliance and audit workflows",
			Command:     "sudo health-monitor compliance init",
			Expected:    "Compliance monitoring enabled",
		},
	)
	
	preset.QuickStart.BestPractices = append(preset.QuickStart.BestPractices,
		"Implement comprehensive audit logging",
		"Set up compliance reporting",
		"Configure automated security scanning",
		"Use RBAC for access control",
		"Set up multi-region monitoring",
		"Implement disaster recovery procedures",
	)
	
	return preset
}

// createDevOpsTeamPreset creates a preset for DevOps-focused teams
func createDevOpsTeamPreset() TeamPreset {
	preset := createMediumTeamPreset()
	preset.Name = "devops-team"
	preset.Description = "Optimized for DevOps teams focusing on CI/CD and infrastructure"
	preset.TeamSize = "medium"
	preset.Complexity = "moderate"
	preset.Profile.Name = "devops-team"
	preset.Capacity = TeamCapacity{
		TeamSize:                 10,
		OpsHoursPerWeekPerPerson: 40,
		TargetToilPercentage:     50,
	}
	
	// DevOps-specific flows
	preset.Flows["cicd"] = Flow{
		Name:        "CI/CD Pipeline",
		Description: "Continuous integration and deployment workflows",
		Services:    []string{"jenkins", "gitlab", "docker_registry", "kubernetes"},
		SLOs: []SLO{
			{
				ID:          "build_success",
				Objective:   95,
				Window:      "1h",
				Type:        "ratio",
				ErrorQuery:  `jenkins_builds_total{result="FAILURE"}`,
				TotalQuery:  `jenkins_builds_total`,
				Description: "Build success rate",
			},
			{
				ID:          "deploy_success",
				Objective:   98,
				Window:      "1h",
				Type:        "ratio",
				ErrorQuery:  `kubernetes_deployments_total{status="failed"}`,
				TotalQuery:  `kubernetes_deployments_total`,
				Description: "Deployment success rate",
			},
		},
	}
	
	preset.Flows["infrastructure"] = Flow{
		Name:        "Infrastructure Health",
		Description: "Infrastructure and platform services monitoring",
		Services:    []string{"kubernetes", "nginx", "database", "redis"},
		SLOs: []SLO{
			{
				ID:          "infra_availability",
				Objective:   99.5,
				Window:      "5m",
				Type:        "ratio",
				ErrorQuery:  `up{job="kubernetes"} == 0`,
				TotalQuery:  `up{job="kubernetes"}`,
				Description: "Infrastructure availability",
			},
		},
	}
	
	// DevOps-specific alerts
	preset.Alerts = []AlertRule{
		{
			Match: map[string]string{
				"service": "jenkins",
			},
			Severity:    "P2",
			Mode:        "auto",
			Title:       "CI/CD Pipeline Alert",
			Description: "Build or deployment pipeline issues",
		},
		{
			Match: map[string]string{
				"service": "kubernetes",
			},
			Severity:    "P1",
			Mode:        "auto",
			Title:       "Kubernetes Cluster Alert",
			Description: "Kubernetes cluster issues",
		},
	}
	
	// DevOps quick start
	preset.QuickStart.SetupSteps = []SetupStep{
		{
			Step:        1,
			Title:       "Configure Infrastructure Metrics",
			Description: "Set up Prometheus to collect infrastructure and CI/CD metrics",
			Command:     "sudo health-monitor profile show",
			Expected:    "Shows infrastructure configuration",
		},
		{
			Step:        2,
			Title:       "Monitor CI/CD Pipelines",
			Description: "Verify Jenkins/GitLab metrics are flowing",
			Command:     "sudo health-monitor flow list",
			Expected:    "Shows CI/CD flows",
		},
		{
			Step:        3,
			Title:       "Start Dashboard",
			Description: "Launch the DevOps monitoring dashboard",
			Command:     "sudo health-monitor",
			Expected:    "Interactive dashboard opens",
		},
	}
	
	preset.QuickStart.BestPractices = []string{
		"Monitor build times and success rates",
		"Track deployment frequency and lead time",
		"Monitor infrastructure resource utilization",
		"Set up alerts for pipeline failures",
		"Track mean time to recovery (MTTR)",
		"Monitor container and pod health",
	}
	
	return preset
}

// createSRETeamPreset creates a preset for SRE-focused teams
func createSRETeamPreset() TeamPreset {
	preset := createLargeTeamPreset()
	preset.Name = "sre-team"
	preset.Description = "Site Reliability Engineering focused configuration with advanced SLOs"
	preset.TeamSize = "medium"
	preset.Complexity = "complex"
	preset.Profile.Name = "sre-team"
	preset.Capacity = TeamCapacity{
		TeamSize:                 15,
		OpsHoursPerWeekPerPerson: 40,
		TargetToilPercentage:     50,
	}
	
	// SRE-specific configuration
	preset.Profile.Config.LatencyThresholdSeconds = 0.5
	preset.Profile.Config.CorrelationBestEffort = true
	preset.Profile.Config.CorrelationMinSamples = 100
	preset.Profile.Config.CorrelationMaxLogs = 1000
	
	// SRE-specific flows with detailed SLOs
	preset.Flows["user_experience"] = Flow{
		Name:        "User Experience",
		Description: "End-to-end user experience metrics",
		Services:    []string{"frontend", "api_gateway", "backend_services"},
		SLOs: []SLO{
			{
				ID:          "page_load_success",
				Objective:   99.9,
				Window:      "5m",
				Type:        "ratio",
				ErrorQuery:  `http_requests_total{service="frontend",status!~"2.."}`,
				TotalQuery:  `http_requests_total{service="frontend"}`,
				Description: "Page load success rate",
			},
			{
				ID:            "page_load_latency",
				Objective:     95,
				Window:        "5m",
				Type:          "latency",
				LatencyQuery:  `histogram_quantile(0.95, rate(http_request_duration_seconds_bucket{service="frontend"}[5m]))`,
				Threshold:     2.0,
				Description:   "95th percentile page load under 2 seconds",
			},
		},
	}
	
	preset.Flows["service_dependencies"] = Flow{
		Name:        "Service Dependencies",
		Description: "Critical service dependencies and their health",
		Services:    []string{"api_gateway", "auth_service", "database", "cache", "queue"},
		SLOs: []SLO{
			{
				ID:          "dependency_availability",
				Objective:   99.95,
				Window:      "5m",
				Type:        "ratio",
				ErrorQuery:  `up{job=~"database|cache|queue"} == 0`,
				TotalQuery:  `up{job=~"database|cache|queue"}`,
				Description: "Critical dependency availability",
			},
		},
	}
	
	// SRE-specific alerts with detailed runbooks
	preset.Alerts = []AlertRule{
		{
			Match: map[string]string{
				"service": "frontend",
				"alertname": "HighLatency",
			},
			Severity:    "P2",
			Mode:        "auto",
			Title:       "Frontend High Latency",
			Description: "Frontend latency exceeding SLO thresholds",
			Runbook:     "https://company-wiki/runbooks/frontend-latency",
		},
		{
			Match: map[string]string{
				"service": "api_gateway",
			},
			Severity:    "P1",
			Mode:        "auto",
			Title:       "API Gateway Issues",
			Description: "API gateway errors or high latency",
			Runbook:     "https://company-wiki/runbooks/api-gateway",
		},
		{
			Match: map[string]string{
				"alertname": "SLOBurnRate",
			},
			Severity:    "P1",
			Mode:        "auto",
			Title:       "SLO Burn Rate Alert",
			Description: "SLO error budget burning too fast",
			Runbook:     "https://company-wiki/runbooks/slo-burn-rate",
		},
	}
	
	// SRE quick start
	preset.QuickStart.SetupSteps = []SetupStep{
		{
			Step:        1,
			Title:       "Define Service SLOs",
			Description: "Configure service level objectives for critical services",
			Command:     "sudo health-monitor slo list",
			Expected:    "Shows configured SLOs",
		},
		{
			Step:        2,
			Title:       "Set up Error Budget Monitoring",
			Description: "Configure error budget tracking and burn rate alerts",
			Command:     "sudo health-monitor slo status",
			Expected:    "Shows SLO status and error budgets",
		},
		{
			Step:        3,
			Title:       "Configure Postmortem Workflow",
			Description: "Set up incident postmortem and blameless culture tools",
			Command:     "sudo health-monitor incident postmortem",
			Expected:    "Postmortem workflow configured",
		},
	}
	
	preset.QuickStart.BestPractices = []string{
		"Define clear SLOs for all user-facing services",
		"Monitor error budgets and burn rates",
		"Use blameless postmortems for incidents",
		"Track reliability metrics over time",
		"Automate toil reduction where possible",
		"Participate in on-call rotations",
		"Conduct regular incident reviews",
		"Monitor the monitoring system itself",
	}
	
	return preset
}

// LoadPreset loads a specific team preset by name
func LoadPreset(presetName string) (TeamPreset, error) {
	presets := GetAvailablePresets()
	for _, preset := range presets {
		if preset.Name == presetName {
			return preset, nil
		}
	}
	return TeamPreset{}, fmt.Errorf("preset '%s' not found. Available presets: %s", 
		presetName, getPresetNames(presets))
}

// getPresetNames returns a comma-separated list of preset names
func getPresetNames(presets []TeamPreset) string {
	var names []string
	for _, preset := range presets {
		names = append(names, preset.Name)
	}
	return strings.Join(names, ", ")
}

// CreateProfileFromPreset creates a new profile based on a preset with its default configuration
func CreateProfileFromPreset(presetName string, profileName string) error {
	preset, err := LoadPreset(presetName)
	if err != nil {
		return err
	}
	return CreateProfileFromPresetWithConfig(presetName, profileName, preset.Profile.Config)
}

// CreateProfileFromPresetWithConfig creates a new profile based on a preset but uses a custom configuration
func CreateProfileFromPresetWithConfig(presetName string, profileName string, cfg Config) error {
	preset, err := LoadPreset(presetName)
	if err != nil {
		return err
	}
	
	pm := GetProfileManager()
	
	// Save profile configuration first
	if err := SaveForProfile(profileName, cfg); err != nil {
		return fmt.Errorf("failed to save profile config: %w", err)
	}
	
	// Now set it as active
	if err := pm.SetActiveProfile(profileName); err != nil {
		return fmt.Errorf("failed to set active profile: %w", err)
	}
	
	// Save flows if present
	if len(preset.Flows) > 0 {
		flowsPath := filepath.Join(pm.GetFlowsPath(), profileName+".yaml")
		if err := saveFlowsToFile(flowsPath, preset.Flows); err != nil {
			return fmt.Errorf("failed to save flows: %w", err)
		}
	}
	
	// Save alerts if present
	if len(preset.Alerts) > 0 {
		alertsPath := filepath.Join(pm.GetAlertsPath(), profileName+".yaml")
		if err := saveAlertsToFile(alertsPath, preset.Alerts); err != nil {
			return fmt.Errorf("failed to save alerts: %w", err)
		}
	}
	
	// Create state directory
	statePath := pm.GetStatePathForProfile(profileName)
	if err := os.MkdirAll(statePath, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}
	
	return nil
}

// saveFlowsToFile saves flows configuration to a YAML file
func saveFlowsToFile(filePath string, flows map[string]Flow) error {
	// This would need to be implemented with proper YAML marshaling
	// For now, create a simple structure
	flowsConfig := map[string]interface{}{
		"flows": flows,
	}
	
	data, err := yaml.Marshal(flowsConfig)
	if err != nil {
		return err
	}
	
	return os.WriteFile(filePath, data, 0600)
}

// saveAlertsToFile saves alerts configuration to a YAML file
func saveAlertsToFile(filePath string, alerts []AlertRule) error {
	// This would need to be implemented with proper YAML marshaling
	// For now, create a simple structure
	alertsConfig := map[string]interface{}{
		"alerts": alerts,
	}
	
	data, err := yaml.Marshal(alertsConfig)
	if err != nil {
		return err
	}
	
	return os.WriteFile(filePath, data, 0600)
}

// ListPresets displays all available presets with descriptions
func ListPresets() {
	presets := GetAvailablePresets()
	fmt.Println("Available Team Presets:")
	fmt.Println("======================")
	
	for _, preset := range presets {
		fmt.Printf("\n📋 %s\n", preset.Name)
		fmt.Printf("   Description: %s\n", preset.Description)
		fmt.Printf("   Team Size: %s\n", preset.TeamSize)
		fmt.Printf("   Complexity: %s\n", preset.Complexity)
		fmt.Printf("   Services: %d\n", len(preset.Flows))
		fmt.Printf("   Alerts: %d\n", len(preset.Alerts))
	}
	
	fmt.Println("\nUsage:")
	fmt.Println("  sudo health-monitor --init --preset <preset-name>")
	fmt.Println("  sudo health-monitor profile create --preset <preset-name> --name <profile-name>")
}
