package config

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"health-monitor/internal/storage"
	"gopkg.in/yaml.v3"
	"time"
)

// DebugLog writes a timestamped message to /tmp/health-monitor-debug.log
func DebugLog(format string, args ...any) {
	f, err := os.OpenFile("/tmp/health-monitor-debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	msg := fmt.Sprintf(format, args...)
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	fmt.Fprintf(f, "[%s] %s\n", timestamp, msg)
}

// ExpandTilde expands a path starting with ~/ to the user's home directory
func ExpandTilde(path string) string {
	if strings.HasPrefix(path, "~/") {
		// If running under sudo, use SUDO_USER's home directory if available
		if os.Geteuid() == 0 {
			if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
				if u, err := user.Lookup(sudoUser); err == nil && u.HomeDir != "" {
					return filepath.Join(u.HomeDir, path[2:])
				}
			}
		}

		dirname, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(dirname, path[2:])
		}
	}
	return path
}


var (
	globalSecretProvider SecretProvider
	secretProviderMu    sync.Once
	testConfigBasePath  string
)

func getSecretProvider() SecretProvider {
	secretProviderMu.Do(func() {
		if globalSecretProvider == nil {
			globalSecretProvider = GetGlobalSecretProvider()
		}
	})
	return globalSecretProvider
}

const (
	envPromURL           = "PROMETHEUS_URL"
	envPromToken         = "PROMETHEUS_TOKEN"
	envPromTokenFile     = "PROMETHEUS_TOKEN_FILE"
	envPromUser          = "PROMETHEUS_USER"
	envPromPass          = "PROMETHEUS_PASS"
	envDisableUpdates    = "HEALTH_MONITOR_DISABLE_UPDATES"
	envLokiURL           = "LOKI_URL"
	envLokiToken         = "LOKI_TOKEN"
	envLokiTokenFile     = "LOKI_TOKEN_FILE"
	envLokiUser          = "LOKI_USER"
	envLokiPass          = "LOKI_PASS"
	envLokiServiceLabel  = "LOKI_SERVICE_LABEL"
	envLokiRouteLabel    = "LOKI_ROUTE_LABEL"
	envLokiErrorRegex    = "LOKI_ERROR_REGEX"
	envLokiWindow        = "LOKI_WINDOW"
	envLokiQueryLimit    = "LOKI_QUERY_LIMIT"
	envLokiQueryTimeout  = "LOKI_QUERY_TIMEOUT"
	envGrafanaURL        = "GRAFANA_URL"
	envGrafanaPromDS     = "GRAFANA_PROM_DS"
	envGrafanaLokiDS     = "GRAFANA_LOKI_DS"
	envGrafanaTraceDS    = "GRAFANA_TRACE_DS"
	envGrafanaToken      = "GRAFANA_TOKEN"
	envGrafanaTokenFile  = "GRAFANA_TOKEN_FILE"
	envGrafanaUser       = "GRAFANA_USER"
	envGrafanaPass       = "GRAFANA_PASS"
	envTraceBackend      = "TRACE_BACKEND"
	envTraceURL          = "TRACE_URL"
	envTraceToken        = "TRACE_TOKEN"
	envTraceUser         = "TRACE_USER"
	envTracePass         = "TRACE_PASS"
	envTraceServiceMap   = "TRACE_SERVICE_MAP"
	envTraceDefaultSvc   = "TRACE_DEFAULT_SERVICE"
	envTraceWindow       = "TRACE_WINDOW"
	envTraceMinDuration  = "TRACE_MIN_DURATION_MS"
	envTraceExcludeSys   = "TRACE_EXCLUDE_SYSTEM_ROUTES"
	envAPIService        = "API_SERVICE"
	envAPIRoute          = "API_ROUTE"
	envServiceLbl        = "API_SERVICE_LABEL"
	envRouteLbl          = "API_ROUTE_LABEL"
	envLatencyM          = "API_LATENCY_METRIC"
	envReqCountM         = "API_REQUESTS_METRIC"
	envAutoDiscover      = "API_AUTO_DISCOVER"
	envLatencyTh         = "API_LATENCY_THRESHOLD"
	envTopEndpoints      = "API_TOP_ENDPOINTS"
	envPromWindow        = "PROMETHEUS_WINDOW"
	envExtraLabels       = "PROMETHEUS_EXTRA_LABELS"
	envErrLabel          = "API_ERROR_LABEL"
	envErrRegex          = "API_ERROR_REGEX"
	envRouteCardLimit    = "API_ROUTE_CARDINALITY_LIMIT"
	envServiceCardLimit  = "API_SERVICE_CARDINALITY_LIMIT"
	envAPMMetric         = "APM_DEPENDENCY_METRIC"
	envAPMSourceLabel    = "APM_SOURCE_LABEL"
	envAPMDestLabel      = "APM_DEST_LABEL"
	envAPMRouteLabel     = "APM_ROUTE_LABEL"
	envAPMWindow         = "APM_WINDOW"
	envAPMTopEdges       = "APM_TOP_EDGES"
	envAPMCardLimit      = "APM_CARDINALITY_LIMIT"
	envAPMMinRPS         = "APM_MIN_RPS"
	envExportPath        = "HEALTH_MONITOR_EXPORT_PATH"
	envDataDir           = "HEALTH_MONITOR_DATA_DIR"
	envPromQPS           = "PROMETHEUS_QPS"
	envLokiQPS           = "LOKI_QPS"
	envCorrelationBest   = "CORRELATION_BEST_EFFORT"
	envCorrelationMin    = "CORRELATION_MIN_SAMPLES"
	envCorrelationMax    = "CORRELATION_MAX_LOGS"
	envCorrelationTop    = "CORRELATION_MAX_RESULTS"
	envCorrelationWindow = "CORRELATION_WINDOW"
	envLogBackend        = "LOG_BACKEND"
	envElasticURL        = "ELASTIC_URL"
	envElasticIndex      = "ELASTIC_INDEX"
	envElasticToken      = "ELASTIC_TOKEN"
	envElasticUser       = "ELASTIC_USER"
	envElasticPass       = "ELASTIC_PASS"
	envElasticService    = "ELASTIC_SERVICE_FIELD"
	envElasticRoute      = "ELASTIC_ROUTE_FIELD"
	envElasticError      = "ELASTIC_ERROR_FIELD"
	envElasticTime       = "ELASTIC_TIME_FIELD"
	envElasticErrorRegex = "ELASTIC_ERROR_REGEX"
	envKibanaURL         = "KIBANA_URL"
	envKibanaIndex       = "KIBANA_INDEX"
	envJiraURL          = "JIRA_URL"
	envJiraUser         = "JIRA_USER"
	envJiraToken        = "JIRA_TOKEN"
	envJiraProject      = "JIRA_PROJECT"
	envMetricSchema     = "METRIC_SCHEMA"
)

const (
	systemConfigPath            = "/etc/health-monitor/config.json"
	defaultTokenFile            = "/etc/health-monitor/prometheus.token"
	defaultBasicAuthFile        = "/etc/health-monitor/prometheus.basic"
	defaultLokiTokenFile        = "/etc/health-monitor/loki.token"
	defaultLokiBasicAuthFile    = "/etc/health-monitor/loki.basic"
	defaultGrafanaTokenFile     = "/etc/health-monitor/grafana.token"
	defaultGrafanaBasicAuthFile = "/etc/health-monitor/grafana.basic"
	defaultTraceTokenFile       = "/etc/health-monitor/trace.token"
	defaultTraceBasicAuthFile   = "/etc/health-monitor/trace.basic"
)

// SlackConfig holds Slack-specific notification settings
type SlackConfig struct {
	Enabled        bool   `json:"enabled"`
	WebhookURL     string `json:"webhook_url"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	MaxRetries     int    `json:"max_retries"`
}

// JiraConfig holds Jira-specific settings
type JiraConfig struct {
	Enabled    bool   `json:"enabled"`
	URL        string `json:"url"`
	User       string `json:"user"`
	Token      string `json:"token"`
	ProjectKey string `json:"project_key"`
}

// PagerDutyConfig holds PagerDuty-specific notification settings
type PagerDutyConfig struct {
	Enabled        bool              `json:"enabled"`
	RoutingKey     string            `json:"routing_key"`
	SeverityMap    map[string]string `json:"severity_map"`
	TimeoutSeconds int               `json:"timeout_seconds"`
	MaxRetries     int               `json:"max_retries"`
}

// ResilienceConfig holds settings for circuit breaking and retries
type ResilienceConfig struct {
	Enabled      bool `json:"enabled"`
	Threshold    int  `json:"threshold"`
	ResetTimeout int  `json:"reset_timeout_seconds"`
	MaxRetries   int  `json:"max_retries"`
}

// Notifications holds notification configuration
type Notifications struct {
	Enabled  bool     `json:"enabled"`
	NotifyOn []string `json:"notify_on"`
	
	// Resilience settings
	Resilience ResilienceConfig `json:"resilience"`
	
	// Nested config (new format)
	Slack     SlackConfig     `json:"slack"`
	PagerDuty PagerDutyConfig `json:"pagerduty"`
	Jira      JiraConfig      `json:"jira"`
	
	// Flat fields (old format, kept for backward compatibility)
	SlackWebhookURL string `json:"slack_webhook_url"`
	TimeoutSeconds  int    `json:"timeout_seconds"`
	MaxRetries      int    `json:"max_retries"`
}


// EKSThresholds defines health thresholds for EKS resources
type EKSThresholds struct {
	PodReadyPercentage   float64 `json:"pod_ready_percentage" yaml:"pod_ready_percentage"`
	NodePressureAllowed  bool    `json:"node_pressure_allowed" yaml:"node_pressure_allowed"`
	MaxRestartCount      int     `json:"max_restart_count" yaml:"max_restart_count"`
}

// EKSConfig holds Amazon EKS specific configuration
type EKSConfig struct {
	Enabled           bool          `json:"enabled" yaml:"enabled"`
	KubeConfig        string        `json:"kubeconfig,omitempty" yaml:"kubeconfig,omitempty"`
	KubeContext       string        `json:"kube_context,omitempty" yaml:"kube_context,omitempty"`
	AWSProfile        string        `json:"aws_profile,omitempty" yaml:"aws_profile,omitempty"`
	Region            string        `json:"region,omitempty" yaml:"region,omitempty"`
	CloudWatchEnabled bool          `json:"cloudwatch_enabled" yaml:"cloudwatch_enabled"`
	AutoDiscover      bool          `json:"auto_discover" yaml:"auto_discover"`
	Namespaces        []string      `json:"namespaces,omitempty" yaml:"namespaces,omitempty"`
	Thresholds        EKSThresholds `json:"thresholds" yaml:"thresholds"`
}

// TeamCapacity holds settings for team-level toil and reliability calculations
type TeamCapacity struct {
	TeamSize               int     `json:"team_size" yaml:"team_size"`
	OpsHoursPerWeekPerPerson int     `json:"ops_hours_per_week_per_person" yaml:"ops_hours_per_week_per_person"`
	TargetToilPercentage   float64 `json:"target_toil_percentage" yaml:"target_toil_percentage"`
	AverageHourlyCost      float64 `json:"average_hourly_cost" yaml:"average_hourly_cost"`
}

type Config struct {
	Provider                 string  `json:"provider" yaml:"provider"`
	PrometheusURL            string  `json:"prometheus_url" yaml:"prometheus_url"`
	PrometheusToken          string  `json:"prometheus_token" yaml:"prometheus_token"`
	PrometheusUser           string  `json:"prometheus_user" yaml:"prometheus_user"`
	PrometheusPass           string  `json:"prometheus_pass" yaml:"prometheus_pass"`
	PrometheusServiceLabel   string  `json:"prometheus_service_label" yaml:"prometheus_service_label"`
	PrometheusMetricMap      map[string]string `json:"prometheus_metric_map" yaml:"prometheus_metric_map"`
	PrometheusLabelMap       map[string]string `json:"prometheus_label_map" yaml:"prometheus_label_map"`
	DisableUpdates           bool    `json:"disable_updates" yaml:"disable_updates"`
	LokiURL                  string  `json:"loki_url" yaml:"loki_url"`
	LokiToken                string  `json:"loki_token" yaml:"loki_token"`
	LokiUser                 string  `json:"loki_user" yaml:"loki_user"`
	LokiPass                 string  `json:"loki_pass" yaml:"loki_pass"`
	LokiServiceLabel         string  `json:"loki_service_label" yaml:"loki_service_label"`
	LokiRouteLabel           string  `json:"loki_route_label" yaml:"loki_route_label"`
	LokiErrorRegex           string  `json:"loki_error_regex" yaml:"loki_error_regex"`
	LokiWindow               string  `json:"loki_window" yaml:"loki_window"`
	LokiQueryLimit           int     `json:"loki_query_limit" yaml:"loki_query_limit"`
	LokiQueryTimeout         string  `json:"loki_query_timeout" yaml:"loki_query_timeout"`
	GrafanaURL               string  `json:"grafana_url" yaml:"grafana_url"`
	GrafanaPromDataSource    string  `json:"grafana_prom_ds" yaml:"grafana_prom_ds"`
	GrafanaLokiDataSource    string  `json:"grafana_loki_ds" yaml:"grafana_loki_ds"`
	GrafanaTraceDataSource   string  `json:"grafana_trace_ds" yaml:"grafana_trace_ds"`
	GrafanaOrgId             int     `json:"grafana_org_id" yaml:"grafana_org_id"`
	GrafanaToken             string  `json:"grafana_token" yaml:"grafana_token"`
	GrafanaUser              string  `json:"grafana_user" yaml:"grafana_user"`
	GrafanaPass              string  `json:"grafana_pass" yaml:"grafana_pass"`
	TraceBackend             string  `json:"trace_backend" yaml:"trace_backend"`
	TraceURL                 string  `json:"trace_url" yaml:"trace_url"`
	TraceToken               string  `json:"trace_token" yaml:"trace_token"`
	TraceUser                string  `json:"trace_user" yaml:"trace_user"`
	TracePass                string  `json:"trace_pass" yaml:"trace_pass"`
	TraceServiceMap          string  `json:"trace_service_map" yaml:"trace_service_map"`
	TraceServiceTag          string  `json:"trace_service_tag" yaml:"trace_service_tag"`
	TraceDefaultService      string  `json:"trace_default_service" yaml:"trace_default_service"`
	TraceWindow              string  `json:"trace_window" yaml:"trace_window"`
	TraceMinDurationMs       int     `json:"trace_min_duration_ms" yaml:"trace_min_duration_ms"`
	TraceExcludeSystemRoutes bool    `json:"trace_exclude_system_routes" yaml:"trace_exclude_system_routes"`
	TraceTimeUnit            string  `json:"trace_time_unit" yaml:"trace_time_unit"`
	CorrelationBestEffort    bool    `json:"correlation_best_effort" yaml:"correlation_best_effort"`
	CorrelationMinSamples    int     `json:"correlation_min_samples" yaml:"correlation_min_samples"`
	CorrelationMaxLogs       int     `json:"correlation_max_logs" yaml:"correlation_max_logs"`
	CorrelationMaxResults    int     `json:"correlation_max_results" yaml:"correlation_max_results"`
	CorrelationWindow        string  `json:"correlation_window" yaml:"correlation_window"`
	LogBackend               string  `json:"log_backend" yaml:"log_backend"`
	ElasticURL               string  `json:"elastic_url" yaml:"elastic_url"`
	ElasticIndex             string  `json:"elastic_index" yaml:"elastic_index"`
	ElasticToken             string  `json:"elastic_token" yaml:"elastic_token"`
	ElasticUser              string  `json:"elastic_user" yaml:"elastic_user"`
	ElasticPass              string  `json:"elastic_pass" yaml:"elastic_pass"`
	ElasticServiceField      string  `json:"elastic_service_field" yaml:"elastic_service_field"`
	ElasticRouteField        string  `json:"elastic_route_field" yaml:"elastic_route_field"`
	MetricSchema             string  `json:"metric_schema" yaml:"metric_schema"` // "auto", "otel", "prometheus", "custom"
	ElasticErrorField        string  `json:"elastic_error_field" yaml:"elastic_error_field"`
	ElasticTimeField         string  `json:"elastic_time_field" yaml:"elastic_time_field"`
	ElasticErrorRegex        string  `json:"elastic_error_regex" yaml:"elastic_error_regex"`
	KibanaURL                string  `json:"kibana_url" yaml:"kibana_url"`
	KibanaIndex              string  `json:"kibana_index" yaml:"kibana_index"`
	APIService               string  `json:"api_service" yaml:"api_service"`
	APIRoute                 string  `json:"api_route" yaml:"api_route"`
	ServiceLabel             string  `json:"service_label" yaml:"service_label"`
	RouteLabel               string  `json:"route_label" yaml:"route_label"`
	LatencyMetric            string  `json:"latency_metric" yaml:"latency_metric"`
	RequestCountMetric       string  `json:"request_count_metric" yaml:"request_count_metric"`
	ErrorLabel               string  `json:"error_label" yaml:"error_label"`
	ErrorRegex               string  `json:"error_regex" yaml:"error_regex"`
	AutoDiscover             bool    `json:"auto_discover" yaml:"auto_discover"`
	LatencyThresholdSeconds  float64 `json:"latency_threshold_seconds" yaml:"latency_threshold_seconds"`
	TopEndpoints             int     `json:"top_endpoints" yaml:"top_endpoints"`
	Window                   string  `json:"window" yaml:"window"`
	ExtraLabelSelectors      string  `json:"extra_label_selectors" yaml:"extra_label_selectors"`
	RouteCardinalityLimit    int     `json:"route_cardinality_limit" yaml:"route_cardinality_limit"`
	ServiceCardinalityLimit  int     `json:"service_cardinality_limit" yaml:"service_cardinality_limit"`
	APMDependencyMetric      string  `json:"apm_dependency_metric" yaml:"apm_dependency_metric"`
	APMSourceLabel           string  `json:"apm_source_label" yaml:"apm_source_label"`
	APMDestinationLabel      string  `json:"apm_dest_label" yaml:"apm_dest_label"`
	APMRouteLabel            string  `json:"apm_route_label" yaml:"apm_route_label"`
	APMWindow                string  `json:"apm_window" yaml:"apm_window"`
	APMTopEdges              int     `json:"apm_top_edges" yaml:"apm_top_edges"`
	APMCardinalityLimit      int     `json:"apm_cardinality_limit" yaml:"apm_cardinality_limit"`
	APMMinRPS                float64 `json:"apm_min_rps" yaml:"apm_min_rps"`
	RunbookURLs              map[string]string `json:"runbook_urls" yaml:"runbook_urls"`
	ExportPath               string  `json:"export_path" yaml:"export_path"`
	DataDir                  string  `json:"data_dir" yaml:"data_dir"`
	PrometheusQPS            float64 `json:"prometheus_qps" yaml:"prometheus_qps"`
	LokiQPS                  float64 `json:"loki_qps" yaml:"loki_qps"`
	ObservabilityLookbackMinutes int `json:"observability_lookback_minutes" yaml:"observability_lookback_minutes"`
	AlertDedupIntervalMinutes    int `json:"alert_dedup_interval_minutes" yaml:"alert_dedup_interval_minutes"`
	EKS                      EKSConfig     `json:"eks" yaml:"eks"`
	Notifications            Notifications `json:"notifications" yaml:"notifications"`
	Security                 SecurityConfig `json:"security" yaml:"security"`
	RunbookSuggestions       RunbookConfig `json:"runbook_suggestions" yaml:"runbook_suggestions"`
	TeamCapacity             TeamCapacity  `json:"team_capacity" yaml:"team_capacity"`
}

func Default() Config {
	return Config{
		EKS: EKSConfig{
			Enabled:      false,
			AutoDiscover: true,
			Thresholds: EKSThresholds{
				PodReadyPercentage:  90.0,
				NodePressureAllowed: false,
				MaxRestartCount:     5,
			},
		},
		PrometheusURL:            "",
		PrometheusToken:          "",
		PrometheusUser:           "",
		PrometheusPass:           "",
		PrometheusServiceLabel:   "service",
		DisableUpdates:           false,
		LokiURL:                  "",
		LokiToken:                "",
		LokiUser:                 "",
		LokiPass:                 "",
		LokiServiceLabel:         "container,job,app,service",
		LokiRouteLabel:           "",
		LokiErrorRegex:           `(?i)(error|exception|fail|timeout|refused|deadlock|warn|warning|critical|severe|fatal|panic|\bstatus[:\s]+[45][0-9][0-9]\b|\bHTTP\s+[45][0-9][0-9]\b)`,
		LokiWindow:               "5m",
		GrafanaURL:               "",
		GrafanaPromDataSource:    "",
		GrafanaLokiDataSource:    "",
		GrafanaTraceDataSource:   "",
		GrafanaOrgId:             1,
		GrafanaToken:             "",
		GrafanaUser:              "",
		GrafanaPass:              "",
		TraceBackend:             "",
		TraceURL:                 "",
		TraceToken:               "",
		TraceUser:                "",
		TracePass:                "",
		TraceServiceMap:          "",
		TraceServiceTag:          "service.name",
		TraceDefaultService:      "",
		TraceWindow:              "4m",
		TraceMinDurationMs:       10,
		TraceExcludeSystemRoutes: true,
		TraceTimeUnit:            "microseconds",
		CorrelationBestEffort:    false,
		CorrelationMinSamples:    10,
		CorrelationMaxLogs:       200,
		CorrelationMaxResults:    5,
		CorrelationWindow:        "",
		LogBackend:               "",
		ElasticURL:               "",
		ElasticIndex:             "",
		ElasticToken:             "",
		ElasticUser:              "",
		ElasticPass:              "",
		ElasticServiceField:      "",
		ElasticRouteField:        "",
		ElasticErrorField:        "",
		ElasticTimeField:         "",
		ElasticErrorRegex:        "",
		KibanaURL:                "",
		KibanaIndex:              "",
		APIService:               "",
		APIRoute:                 "",
		ServiceLabel:             "service",
		RouteLabel:               "route",
		LatencyMetric:            "http_request_duration_seconds",
		RequestCountMetric:       "http_requests_total",
		ErrorLabel:               "status",
		ErrorRegex:               "5..",
		AutoDiscover:             true,
		LatencyThresholdSeconds:  10,
		TopEndpoints:             5,
		Window:                   "5m",
		ExtraLabelSelectors:      "",
		RouteCardinalityLimit:    500,
		ServiceCardinalityLimit:  200,
		APMDependencyMetric:      "",
		APMSourceLabel:           "",
		APMDestinationLabel:      "",
		APMRouteLabel:            "",
		APMWindow:                "10m",
		APMTopEdges:              10,
		APMCardinalityLimit:      200,
		APMMinRPS:                0.1,
		RunbookURLs:              make(map[string]string),
		ExportPath:               "",
		DataDir:                  "",
		PrometheusQPS:            10,
		LokiQPS:                  5,
		ObservabilityLookbackMinutes: 15,
		AlertDedupIntervalMinutes:    2,
		Notifications: Notifications{
			Enabled:  false,
			NotifyOn: []string{"started", "suggest", "ack", "resolve"},
			Resilience: ResilienceConfig{
				Enabled:      true,
				Threshold:    3,
				ResetTimeout: 60,
				MaxRetries:   2,
			},
			
			// Old flat format defaults (for backward compatibility)
			SlackWebhookURL: "",
			TimeoutSeconds:  10,
			MaxRetries:      3,
			
			// New nested format defaults
			Slack: SlackConfig{
				Enabled:        false,
				WebhookURL:     "",
				TimeoutSeconds: 10,
				MaxRetries:     3,
			},
			PagerDuty: PagerDutyConfig{
				Enabled:        false,
				RoutingKey:     "",
				SeverityMap: map[string]string{
					"P1": "critical",
					"P2": "error",
					"P3": "warning",
				},
				TimeoutSeconds: 10,
				MaxRetries:     3,
			},
			Jira: JiraConfig{
				Enabled: false,
			},
		},
		Security: DefaultSecurityConfig(),
		RunbookSuggestions: DefaultRunbookConfig(),
		TeamCapacity: TeamCapacity{
			TeamSize:               5,
			OpsHoursPerWeekPerPerson: 40,
			TargetToilPercentage:   50.0,
			AverageHourlyCost:      100.0,
		},
		MetricSchema: "auto",
	}
}

func Load() (Config, error) {
	pm := GetProfileManager()
	activeProfile := pm.GetActiveProfile()
	return LoadForProfile(activeProfile)
}

// LoadForProfile loads configuration for a specific profile
func LoadForProfile(profileName string) (Config, error) {
	pm := GetProfileManager()
	
	if profileName == "" {
		profileName = "default"
	}
	
	profile, err := pm.GetProfile(profileName)
	if err != nil {
		return Default(), err
	}
	
	cfg := profile.Config
	
	// Propagate profile-level fields to config safely
	if cfg.Provider == "" {
		cfg.Provider = profile.Provider
	}
	if cfg.EKS.Region == "" {
		cfg.EKS.Region = profile.Region
	}
	if len(cfg.EKS.Namespaces) == 0 {
		cfg.EKS.Namespaces = profile.Namespaces
	}
	
	// Apply environment overrides (profile-scoped first, then global)
	applyProfileEnvOverrides(&cfg, profileName)
	
	// Apply token files (profile-scoped)
	applyProfileTokenFiles(&cfg, profileName)
	
	// Normalize and validate
	normalizeConfig(&cfg)
	
	// Migrate old flat notification config to new nested format for backward compatibility
	migrateNotificationConfig(&cfg)
	
	return cfg, nil
}

// migrateNotificationConfig migrates old flat notification config to new nested format
func migrateNotificationConfig(cfg *Config) {
	// If nested Slack config is empty but flat fields exist, migrate them
	if cfg.Notifications.Slack.WebhookURL == "" && cfg.Notifications.SlackWebhookURL != "" {
		cfg.Notifications.Slack.Enabled = cfg.Notifications.Enabled
		cfg.Notifications.Slack.WebhookURL = cfg.Notifications.SlackWebhookURL
		
		if cfg.Notifications.TimeoutSeconds > 0 {
			cfg.Notifications.Slack.TimeoutSeconds = cfg.Notifications.TimeoutSeconds
		}
		if cfg.Notifications.MaxRetries > 0 {
			cfg.Notifications.Slack.MaxRetries = cfg.Notifications.MaxRetries
		}
	}
	
	// If nested config exists, it takes precedence (no migration needed)
	// PagerDuty config is new, so no migration needed
}

func normalizeConfig(cfg *Config) {
	if cfg.PrometheusQPS < 0 {
		cfg.PrometheusQPS = 0
	}
	if cfg.LokiQPS < 0 {
		cfg.LokiQPS = 0
	}
	if cfg.RouteCardinalityLimit < 0 {
		cfg.RouteCardinalityLimit = 0
	}
	if cfg.ServiceCardinalityLimit < 0 {
		cfg.ServiceCardinalityLimit = 0
	}
	if cfg.TopEndpoints <= 0 {
		cfg.TopEndpoints = Default().TopEndpoints
	}
	if cfg.APMTopEdges <= 0 {
		cfg.APMTopEdges = Default().APMTopEdges
	}
	if cfg.APMMinRPS < 0 {
		cfg.APMMinRPS = 0
	}
	if strings.TrimSpace(cfg.TraceWindow) == "" {
		cfg.TraceWindow = Default().TraceWindow
	}
	if cfg.TraceMinDurationMs < 0 {
		cfg.TraceMinDurationMs = 0
	}

	// Auto-enable infrastructure features based on provider
	if strings.EqualFold(cfg.Provider, "eks") || strings.EqualFold(cfg.Provider, "aws-eks") {
		cfg.EKS.Enabled = true
	}
	
	// Synchronize Prometheus/Loki labels with the unified ServiceLabel/RouteLabel
	if cfg.ServiceLabel == "" || cfg.ServiceLabel == "service" {
		if cfg.PrometheusServiceLabel != "" && cfg.PrometheusServiceLabel != "service" {
			cfg.ServiceLabel = cfg.PrometheusServiceLabel
		} else if cfg.LokiServiceLabel != "" {
			cfg.ServiceLabel = cfg.LokiServiceLabel
		}
	}
	if cfg.RouteLabel == "" || cfg.RouteLabel == "route" {
		if cfg.LokiRouteLabel != "" {
			cfg.RouteLabel = cfg.LokiRouteLabel
		}
	}
}

func ConfigPath() (string, error) {
	pm := GetProfileManager()
	activeProfile := pm.GetActiveProfile()
	return ConfigPathForProfile(activeProfile)
}

// ConfigPathForProfile returns the config path for a specific profile
func ConfigPathForProfile(profileName string) (string, error) {
	pm := GetProfileManager()
	return pm.getProfilePath(profileName), nil
}

// StatePath returns the state path for the active profile
func StatePath() string {
	pm := GetProfileManager()
	activeProfile := pm.GetActiveProfile()
	return StatePathForProfile(activeProfile)
}

// StatePathForProfile returns the state path for a specific profile
func StatePathForProfile(profileName string) string {
	pm := GetProfileManager()
	return pm.getStatePath(profileName)
}

func Save(cfg Config) error {
	pm := GetProfileManager()
	activeProfile := pm.GetActiveProfile()
	return SaveForProfile(activeProfile, cfg)
}

// SaveForProfile saves configuration for a specific profile
func SaveForProfile(profileName string, cfg Config) error {
	pm := GetProfileManager()
	
	if profileName == "" {
		profileName = "default"
	}
	
	profile, err := pm.GetProfile(profileName)
	if err != nil {
		// Create a new profile if it doesn't exist
		profile = &Profile{
			Name:   profileName,
		}
		// Update cache
		pm.mutex.Lock()
		pm.profiles[profileName] = profile
		pm.mutex.Unlock()
	}
	
	// Sanitize secrets before saving
	safe := cfg
	safe.PrometheusToken = ""
	safe.PrometheusUser = ""
	safe.PrometheusPass = ""
	safe.LokiToken = ""
	safe.LokiUser = ""
	safe.LokiPass = ""
	safe.GrafanaToken = ""
	safe.GrafanaUser = ""
	safe.GrafanaPass = ""
	safe.TraceToken = ""
	safe.TraceUser = ""
	safe.TracePass = ""
	safe.Notifications.Jira.Token = ""
	safe.Notifications.Jira.User = ""
	
	profile.Config = safe
	
	profilePath := pm.getProfilePath(profileName)
	if err := os.MkdirAll(filepath.Dir(profilePath), 0755); err != nil {
		return err
	}
	
	data, err := yaml.Marshal(profile)
	if err != nil {
		return err
	}
	
	if err := storage.AtomicWriteFile(profilePath, data, 0600); err != nil {
		return err
	}
	
	return nil
}

func MissingRequired(cfg Config) []string {
	var missing []string
	if cfg.PrometheusURL == "" {
		missing = append(missing, "PROMETHEUS_URL")
	}
	
	// API_SERVICE is now optional; if missing, we show the Top Services overview.
	// Auto-discovery handles configuration if enabled.
	
	return missing
}

func configPath() (string, error) {
	if os.Geteuid() == 0 {
		if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
			if u, err := user.Lookup(sudoUser); err == nil && u.HomeDir != "" {
				return filepath.Join(u.HomeDir, ".health-monitor", "config.json"), nil
			}
		}
	}

	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", errors.New("unable to determine home directory")
	}
	return filepath.Join(home, ".health-monitor", "config.json"), nil
}

// GetCurrentUserName returns the name of the current user, or "unknown" if not found
func GetCurrentUserName() (string, error) {
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		return sudoUser, nil
	}
	u, err := user.Current()
	if err != nil {
		return "unknown", err
	}
	return u.Username, nil
}

func SystemConfigPath() string {
	return systemConfigPath
}

func TokenFilePath() string {
	return activeTokenFilePath()
}

func BasicAuthFilePath() string {
	return activeBasicAuthFilePath()
}

func LokiTokenFilePath() string {
	return activeLokiTokenFilePath()
}

func LokiBasicAuthFilePath() string {
	return activeLokiBasicAuthFilePath()
}

func GrafanaTokenFilePath() string {
	return activeGrafanaTokenFilePath()
}

func GrafanaBasicAuthFilePath() string {
	return activeGrafanaBasicAuthFilePath()
}

func TraceTokenFilePath() string {
	return activeTraceTokenFilePath()
}

func TraceBasicAuthFilePath() string {
	return activeTraceBasicAuthFilePath()
}

func ClearTokenFile() error {
	if err := os.Remove(activeTokenFilePath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func ClearBasicAuthFile() error {
	if err := os.Remove(activeBasicAuthFilePath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func ClearLokiTokenFile() error {
	if err := os.Remove(activeLokiTokenFilePath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func ClearLokiBasicAuthFile() error {
	if err := os.Remove(activeLokiBasicAuthFilePath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func ClearGrafanaTokenFile() error {
	if err := os.Remove(activeGrafanaTokenFilePath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func ClearGrafanaBasicAuthFile() error {
	if err := os.Remove(activeGrafanaBasicAuthFilePath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func ClearTraceTokenFile() error {
	if err := os.Remove(activeTraceTokenFilePath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func ClearTraceBasicAuthFile() error {
	if err := os.Remove(activeTraceBasicAuthFilePath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func UserConfigPath() (string, error) {
	return configPath()
}

func activeTokenFilePath() string {
	if os.Geteuid() == 0 {
		return defaultTokenFile
	}
	if path := userTokenFilePath(); path != "" {
		return path
	}
	return defaultTokenFile
}

func activeBasicAuthFilePath() string {
	if os.Geteuid() == 0 {
		return defaultBasicAuthFile
	}
	if path := userBasicAuthFilePath(); path != "" {
		return path
	}
	return defaultBasicAuthFile
}

func activeLokiTokenFilePath() string {
	if os.Geteuid() == 0 {
		return defaultLokiTokenFile
	}
	if path := userLokiTokenFilePath(); path != "" {
		return path
	}
	return defaultLokiTokenFile
}

func activeLokiBasicAuthFilePath() string {
	if os.Geteuid() == 0 {
		return defaultLokiBasicAuthFile
	}
	if path := userLokiBasicAuthFilePath(); path != "" {
		return path
	}
	return defaultLokiBasicAuthFile
}

func activeGrafanaTokenFilePath() string {
	if os.Geteuid() == 0 {
		return defaultGrafanaTokenFile
	}
	if path := userGrafanaTokenFilePath(); path != "" {
		return path
	}
	return defaultGrafanaTokenFile
}

func activeGrafanaBasicAuthFilePath() string {
	if os.Geteuid() == 0 {
		return defaultGrafanaBasicAuthFile
	}
	if path := userGrafanaBasicAuthFilePath(); path != "" {
		return path
	}
	return defaultGrafanaBasicAuthFile
}

func activeTraceTokenFilePath() string {
	if os.Geteuid() == 0 {
		return defaultTraceTokenFile
	}
	if path := userTraceTokenFilePath(); path != "" {
		return path
	}
	return defaultTraceTokenFile
}

func activeTraceBasicAuthFilePath() string {
	if os.Geteuid() == 0 {
		return defaultTraceBasicAuthFile
	}
	if path := userTraceBasicAuthFilePath(); path != "" {
		return path
	}
	return defaultTraceBasicAuthFile
}

func userTokenFilePath() string {
	if userConfig, err := configPath(); err == nil && userConfig != "" {
		return filepath.Join(filepath.Dir(userConfig), "prometheus.token")
	}
	return ""
}

func userBasicAuthFilePath() string {
	if userConfig, err := configPath(); err == nil && userConfig != "" {
		return filepath.Join(filepath.Dir(userConfig), "prometheus.basic")
	}
	return ""
}

func userLokiTokenFilePath() string {
	if userConfig, err := configPath(); err == nil && userConfig != "" {
		return filepath.Join(filepath.Dir(userConfig), "loki.token")
	}
	return ""
}

func userLokiBasicAuthFilePath() string {
	if userConfig, err := configPath(); err == nil && userConfig != "" {
		return filepath.Join(filepath.Dir(userConfig), "loki.basic")
	}
	return ""
}

func userGrafanaTokenFilePath() string {
	if userConfig, err := configPath(); err == nil && userConfig != "" {
		return filepath.Join(filepath.Dir(userConfig), "grafana.token")
	}
	return ""
}

func userGrafanaBasicAuthFilePath() string {
	if userConfig, err := configPath(); err == nil && userConfig != "" {
		return filepath.Join(filepath.Dir(userConfig), "grafana.basic")
	}
	return ""
}

func userTraceTokenFilePath() string {
	if userConfig, err := configPath(); err == nil && userConfig != "" {
		return filepath.Join(filepath.Dir(userConfig), "trace.token")
	}
	return ""
}

func userTraceBasicAuthFilePath() string {
	if userConfig, err := configPath(); err == nil && userConfig != "" {
		return filepath.Join(filepath.Dir(userConfig), "trace.basic")
	}
	return ""
}

func tokenFileCandidates() []string {
	userPath := userTokenFilePath()
	if os.Geteuid() == 0 {
		if userPath != "" {
			return []string{defaultTokenFile, userPath}
		}
		return []string{defaultTokenFile}
	}
	if userPath != "" {
		return []string{userPath, defaultTokenFile}
	}
	return []string{defaultTokenFile}
}

func basicAuthFileCandidates() []string {
	userPath := userBasicAuthFilePath()
	if os.Geteuid() == 0 {
		if userPath != "" {
			return []string{defaultBasicAuthFile, userPath}
		}
		return []string{defaultBasicAuthFile}
	}
	if userPath != "" {
		return []string{userPath, defaultBasicAuthFile}
	}
	return []string{defaultBasicAuthFile}
}

func lokiTokenFileCandidates() []string {
	userPath := userLokiTokenFilePath()
	if os.Geteuid() == 0 {
		if userPath != "" {
			return []string{defaultLokiTokenFile, userPath}
		}
		return []string{defaultLokiTokenFile}
	}
	if userPath != "" {
		return []string{userPath, defaultLokiTokenFile}
	}
	return []string{defaultLokiTokenFile}
}

func grafanaTokenFileCandidates() []string {
	userPath := userGrafanaTokenFilePath()
	if os.Geteuid() == 0 {
		if userPath != "" {
			return []string{defaultGrafanaTokenFile, userPath}
		}
		return []string{defaultGrafanaTokenFile}
	}
	if userPath != "" {
		return []string{userPath, defaultGrafanaTokenFile}
	}
	return []string{defaultGrafanaTokenFile}
}

func grafanaBasicAuthFileCandidates() []string {
	userPath := userGrafanaBasicAuthFilePath()
	if os.Geteuid() == 0 {
		if userPath != "" {
			return []string{defaultGrafanaBasicAuthFile, userPath}
		}
		return []string{defaultGrafanaBasicAuthFile}
	}
	if userPath != "" {
		return []string{userPath, defaultGrafanaBasicAuthFile}
	}
	return []string{defaultGrafanaBasicAuthFile}
}

func traceTokenFileCandidates() []string {
	userPath := userTraceTokenFilePath()
	if os.Geteuid() == 0 {
		if userPath != "" {
			return []string{defaultTraceTokenFile, userPath}
		}
		return []string{defaultTraceTokenFile}
	}
	if userPath != "" {
		return []string{userPath, defaultTraceTokenFile}
	}
	return []string{defaultTraceTokenFile}
}

func traceBasicAuthFileCandidates() []string {
	userPath := userTraceBasicAuthFilePath()
	if os.Geteuid() == 0 {
		if userPath != "" {
			return []string{defaultTraceBasicAuthFile, userPath}
		}
		return []string{defaultTraceBasicAuthFile}
	}
	if userPath != "" {
		return []string{userPath, defaultTraceBasicAuthFile}
	}
	return []string{defaultTraceBasicAuthFile}
}

func lokiBasicAuthFileCandidates() []string {
	userPath := userLokiBasicAuthFilePath()
	if os.Geteuid() == 0 {
		if userPath != "" {
			return []string{defaultLokiBasicAuthFile, userPath}
		}
		return []string{defaultLokiBasicAuthFile}
	}
	if userPath != "" {
		return []string{userPath, defaultLokiBasicAuthFile}
	}
	return []string{defaultLokiBasicAuthFile}
}

func activeConfigPath() (string, error) {
	if fileExists(systemConfigPath) {
		return systemConfigPath, nil
	}
	return configPath()
}

func activeConfigPathForSave() (string, error) {
	if os.Geteuid() == 0 {
		return systemConfigPath, nil
	}
	return configPath()
}

func WriteTokenFile(token string) error {
	clean, err := sanitizeToken(token)
	if err != nil {
		return err
	}
	if strings.TrimSpace(clean) == "" {
		return errors.New("token is empty")
	}
	path := activeTokenFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.TrimSpace(clean)), 0600)
}

func WriteBasicAuthFile(user string, pass string) error {
	cleanUser, err := sanitizeToken(user)
	if err != nil {
		return err
	}
	cleanPass, err := sanitizeToken(pass)
	if err != nil {
		return err
	}
	if strings.TrimSpace(cleanUser) == "" || strings.TrimSpace(cleanPass) == "" {
		return errors.New("basic auth user/pass is empty")
	}
	path := activeBasicAuthFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	value := strings.TrimSpace(cleanUser) + ":" + strings.TrimSpace(cleanPass)
	return os.WriteFile(path, []byte(value), 0600)
}

func WriteLokiTokenFile(token string) error {
	clean, err := sanitizeToken(token)
	if err != nil {
		return err
	}
	if strings.TrimSpace(clean) == "" {
		return errors.New("token is empty")
	}
	path := activeLokiTokenFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.TrimSpace(clean)), 0600)
}

func WriteLokiBasicAuthFile(user string, pass string) error {
	cleanUser, err := sanitizeToken(user)
	if err != nil {
		return err
	}
	cleanPass, err := sanitizeToken(pass)
	if err != nil {
		return err
	}
	if strings.TrimSpace(cleanUser) == "" || strings.TrimSpace(cleanPass) == "" {
		return errors.New("basic auth user/pass is empty")
	}
	path := activeLokiBasicAuthFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	value := strings.TrimSpace(cleanUser) + ":" + strings.TrimSpace(cleanPass)
	return os.WriteFile(path, []byte(value), 0600)
}

func WriteGrafanaTokenFile(token string) error {
	clean, err := sanitizeToken(token)
	if err != nil {
		return err
	}
	if strings.TrimSpace(clean) == "" {
		return errors.New("token is empty")
	}
	path := activeGrafanaTokenFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.TrimSpace(clean)), 0600)
}

func WriteGrafanaBasicAuthFile(user string, pass string) error {
	cleanUser, err := sanitizeToken(user)
	if err != nil {
		return err
	}
	cleanPass, err := sanitizeToken(pass)
	if err != nil {
		return err
	}
	if strings.TrimSpace(cleanUser) == "" || strings.TrimSpace(cleanPass) == "" {
		return errors.New("basic auth user/pass is empty")
	}
	path := activeGrafanaBasicAuthFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	value := strings.TrimSpace(cleanUser) + ":" + strings.TrimSpace(cleanPass)
	return os.WriteFile(path, []byte(value), 0600)
}

func WriteTraceTokenFile(token string) error {
	clean, err := sanitizeToken(token)
	if err != nil {
		return err
	}
	if strings.TrimSpace(clean) == "" {
		return errors.New("token is empty")
	}
	path := activeTraceTokenFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.TrimSpace(clean)), 0600)
}

func WriteTraceBasicAuthFile(user string, pass string) error {
	cleanUser, err := sanitizeToken(user)
	if err != nil {
		return err
	}
	cleanPass, err := sanitizeToken(pass)
	if err != nil {
		return err
	}
	if strings.TrimSpace(cleanUser) == "" || strings.TrimSpace(cleanPass) == "" {
		return errors.New("basic auth user/pass is empty")
	}
	path := activeTraceBasicAuthFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	value := strings.TrimSpace(cleanUser) + ":" + strings.TrimSpace(cleanPass)
	return os.WriteFile(path, []byte(value), 0600)
}

func sanitizeURL(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if looksLikeTerminalGarbage(trimmed) {
		return ""
	}
	cleaned := stripControlChars(trimmed)
	if strings.HasPrefix(cleaned, "http://") || strings.HasPrefix(cleaned, "https://") {
		return cleaned
	}
	return ""
}

func sanitizeToken(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}
	if looksLikeTerminalGarbage(trimmed) {
		return "", errors.New("token contains terminal escape data")
	}
	cleaned := stripControlChars(trimmed)
	if cleaned == "" {
		return "", errors.New("token contains invalid characters")
	}
	return cleaned, nil
}

func looksLikeTerminalGarbage(value string) bool {
	return strings.Contains(value, "rgb:") ||
		strings.Contains(value, "]11;") ||
		strings.Contains(value, "c/0c") ||
		strings.Contains(value, "0c/")
}

func stripControlChars(value string) string {
	var b strings.Builder
	b.Grow(len(value))
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if ch == 0x1b || ch < 0x20 || ch == 0x7f {
			continue
		}
		b.WriteByte(ch)
	}
	return b.String()
}

func InitConfig() (string, bool, error) {
	path, err := activeConfigPathForSave()
	if err != nil {
		return "", false, err
	}

	if fileExists(path) {
		return path, false, nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", false, err
	}

	if err := Save(Default()); err != nil {
		return "", false, err
	}

	return path, true, nil
}

func applyEnv(cfg *Config) {
	if v := os.Getenv(envPromURL); v != "" {
		cfg.PrometheusURL = v
	}
	if v := os.Getenv(envPromToken); v != "" {
		cfg.PrometheusToken = v
	}
	if v := os.Getenv(envPromTokenFile); v != "" {
		cfg.PrometheusToken = readTokenFile(v)
	}
	if v := os.Getenv(envPromUser); v != "" {
		cfg.PrometheusUser = v
	}
	if v := os.Getenv(envPromPass); v != "" {
		cfg.PrometheusPass = v
	}
	if v := os.Getenv(envDisableUpdates); v != "" {
		cfg.DisableUpdates = v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
	}
	if v := os.Getenv(envLokiURL); v != "" {
		cfg.LokiURL = v
	}
	if v := os.Getenv(envLokiToken); v != "" {
		cfg.LokiToken = v
	}
	if v := os.Getenv(envLokiTokenFile); v != "" {
		cfg.LokiToken = readTokenFile(v)
	}
	if v := os.Getenv(envLokiUser); v != "" {
		cfg.LokiUser = v
	}
	if v := os.Getenv(envLokiPass); v != "" {
		cfg.LokiPass = v
	}
	if v := os.Getenv(envLokiServiceLabel); v != "" {
		cfg.LokiServiceLabel = v
	}
	if v := os.Getenv(envLokiRouteLabel); v != "" {
		cfg.LokiRouteLabel = v
	}
	if v := os.Getenv(envLokiErrorRegex); v != "" {
		cfg.LokiErrorRegex = v
	}
	if v := os.Getenv(envLokiWindow); v != "" {
		cfg.LokiWindow = v
	}
	if v := os.Getenv(envLokiQueryLimit); v != "" {
		if limit, err := strconv.Atoi(v); err == nil {
			cfg.LokiQueryLimit = limit
		}
	}
	if v := os.Getenv(envLokiQueryTimeout); v != "" {
		cfg.LokiQueryTimeout = v
	}
	if v := os.Getenv(envGrafanaToken); v != "" {
		cfg.GrafanaToken = v
	}
	if v := os.Getenv(envGrafanaTokenFile); v != "" {
		cfg.GrafanaToken = readTokenFile(v)
	}
	if v := os.Getenv(envGrafanaUser); v != "" {
		cfg.GrafanaUser = v
	}
	if v := os.Getenv(envGrafanaPass); v != "" {
		cfg.GrafanaPass = v
	}
	if v := os.Getenv(envGrafanaURL); v != "" {
		cfg.GrafanaURL = v
	}
	if v := os.Getenv(envGrafanaPromDS); v != "" {
		cfg.GrafanaPromDataSource = v
	}
	if v := os.Getenv(envGrafanaLokiDS); v != "" {
		cfg.GrafanaLokiDataSource = v
	}
	if v := os.Getenv(envGrafanaTraceDS); v != "" {
		cfg.GrafanaTraceDataSource = v
	}
	if v := os.Getenv(envTraceBackend); v != "" {
		cfg.TraceBackend = v
	}
	if v := os.Getenv(envTraceURL); v != "" {
		cfg.TraceURL = v
	}
	if v := os.Getenv(envTraceToken); v != "" {
		cfg.TraceToken = v
	}
	if v := os.Getenv(envTraceUser); v != "" {
		cfg.TraceUser = v
	}
	if v := os.Getenv(envTracePass); v != "" {
		cfg.TracePass = v
	}
	if v := os.Getenv(envTraceServiceMap); v != "" {
		cfg.TraceServiceMap = v
	}
	if v := os.Getenv(envTraceDefaultSvc); v != "" {
		cfg.TraceDefaultService = v
	}
	if v := os.Getenv(envTraceWindow); v != "" {
		cfg.TraceWindow = v
	}
	if v := os.Getenv(envTraceMinDuration); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			cfg.TraceMinDurationMs = i
		}
	}
	if v := os.Getenv(envTraceExcludeSys); v != "" {
		cfg.TraceExcludeSystemRoutes = v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
	}
	if v := os.Getenv(envAPIService); v != "" {
		cfg.APIService = v
	}
	if v := os.Getenv(envAPIRoute); v != "" {
		cfg.APIRoute = v
	}
	if v := os.Getenv(envServiceLbl); v != "" {
		cfg.ServiceLabel = v
	}
	if v := os.Getenv(envRouteLbl); v != "" {
		cfg.RouteLabel = v
	}
	if v := os.Getenv(envLatencyM); v != "" {
		cfg.LatencyMetric = v
	}
	if v := os.Getenv(envReqCountM); v != "" {
		cfg.RequestCountMetric = v
	}
	if v := os.Getenv(envAutoDiscover); v != "" {
		cfg.AutoDiscover = v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
	}
	if v := os.Getenv(envErrLabel); v != "" {
		cfg.ErrorLabel = v
	}
	if v := os.Getenv(envErrRegex); v != "" {
		cfg.ErrorRegex = v
	}
	if v := os.Getenv(envLatencyTh); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.LatencyThresholdSeconds = f
		}
	}
	if v := os.Getenv(envTopEndpoints); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			cfg.TopEndpoints = i
		}
	}
	if v := os.Getenv(envPromWindow); v != "" {
		cfg.Window = v
	}
	if v := os.Getenv(envExtraLabels); v != "" {
		cfg.ExtraLabelSelectors = v
	}
	if v := os.Getenv(envRouteCardLimit); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			cfg.RouteCardinalityLimit = i
		}
	}
	if v := os.Getenv(envServiceCardLimit); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			cfg.ServiceCardinalityLimit = i
		}
	}
	if v := os.Getenv(envAPMMetric); v != "" {
		cfg.APMDependencyMetric = v
	}
	if v := os.Getenv(envAPMSourceLabel); v != "" {
		cfg.APMSourceLabel = v
	}
	if v := os.Getenv(envAPMDestLabel); v != "" {
		cfg.APMDestinationLabel = v
	}
	if v := os.Getenv(envAPMRouteLabel); v != "" {
		cfg.APMRouteLabel = v
	}
	if v := os.Getenv(envAPMWindow); v != "" {
		cfg.APMWindow = v
	}
	if v := os.Getenv(envAPMTopEdges); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			cfg.APMTopEdges = i
		}
	}
	if v := os.Getenv(envAPMCardLimit); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			cfg.APMCardinalityLimit = i
		}
	}
	if v := os.Getenv(envAPMMinRPS); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 {
			cfg.APMMinRPS = f
		}
	}
	if v := os.Getenv(envExportPath); v != "" {
		cfg.ExportPath = v
	}
	if v := os.Getenv(envDataDir); v != "" {
		cfg.DataDir = v
	}
	if v := os.Getenv(envPromQPS); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.PrometheusQPS = f
		}
	}
	if v := os.Getenv(envLokiQPS); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.LokiQPS = f
		}
	}
	if v := os.Getenv(envCorrelationBest); v != "" {
		cfg.CorrelationBestEffort = v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
	}
	if v := os.Getenv(envCorrelationMin); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			cfg.CorrelationMinSamples = i
		}
	}
	if v := os.Getenv(envCorrelationMax); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			cfg.CorrelationMaxLogs = i
		}
	}
	if v := os.Getenv(envCorrelationTop); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			cfg.CorrelationMaxResults = i
		}
	}
	if v := os.Getenv(envCorrelationWindow); v != "" {
		cfg.CorrelationWindow = v
	}
	if v := os.Getenv(envLogBackend); v != "" {
		cfg.LogBackend = strings.TrimSpace(strings.ToLower(v))
	}
	if v := os.Getenv(envElasticURL); v != "" {
		cfg.ElasticURL = v
	}
	if v := os.Getenv(envElasticIndex); v != "" {
		cfg.ElasticIndex = v
	}
	if v := os.Getenv(envElasticToken); v != "" {
		cfg.ElasticToken = v
	}
	if v := os.Getenv(envElasticUser); v != "" {
		cfg.ElasticUser = v
	}
	if v := os.Getenv(envElasticPass); v != "" {
		cfg.ElasticPass = v
	}
	if v := os.Getenv(envElasticService); v != "" {
		cfg.ElasticServiceField = v
	}
	if v := os.Getenv(envElasticRoute); v != "" {
		cfg.ElasticRouteField = v
	}
	if v := os.Getenv(envElasticError); v != "" {
		cfg.ElasticErrorField = v
	}
	if v := os.Getenv(envElasticTime); v != "" {
		cfg.ElasticTimeField = v
	}
	if v := os.Getenv(envElasticErrorRegex); v != "" {
		cfg.ElasticErrorRegex = v
	}
	if v := os.Getenv(envKibanaURL); v != "" {
		cfg.KibanaURL = v
	}
	if v := os.Getenv(envKibanaIndex); v != "" {
		cfg.KibanaIndex = v
	}
	if v := os.Getenv(envJiraURL); v != "" {
		cfg.Notifications.Jira.URL = v
	}
	if v := os.Getenv(envJiraUser); v != "" {
		cfg.Notifications.Jira.User = v
	}
	if v := os.Getenv(envJiraToken); v != "" {
		cfg.Notifications.Jira.Token = v
	}
	if v := os.Getenv(envJiraProject); v != "" {
		cfg.Notifications.Jira.ProjectKey = v
	}
	if v := os.Getenv(envMetricSchema); v != "" {
		cfg.MetricSchema = strings.ToLower(v)
	}
}

func applyTokenFile(cfg *Config) {
	if cfg.PrometheusToken != "" {
		return
	}
	for _, path := range tokenFileCandidates() {
		if token := readTokenFile(path); token != "" {
			cfg.PrometheusToken = token
			return
		}
	}

	// Try SecretProvider as fallback
	if sp := getSecretProvider(); sp != nil {
		if token, err := sp.GetSecret("prometheus.token"); err == nil && token != "" {
			cfg.PrometheusToken = token
		}
	}
}

func applyBasicAuthFile(cfg *Config) {
	if cfg.PrometheusUser != "" || cfg.PrometheusPass != "" {
		return
	}
	for _, path := range basicAuthFileCandidates() {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		parts := strings.SplitN(strings.TrimSpace(string(data)), ":", 2)
		if len(parts) != 2 {
			continue
		}
		cfg.PrometheusUser = parts[0]
		cfg.PrometheusPass = parts[1]
		return
	}

	// Try SecretProvider as fallback
	if sp := getSecretProvider(); sp != nil {
		if auth, err := sp.GetSecret("prometheus.basic"); err == nil && auth != "" {
			parts := strings.SplitN(auth, ":", 2)
			if len(parts) == 2 {
				cfg.PrometheusUser = parts[0]
				cfg.PrometheusPass = parts[1]
			}
		}
	}
}

func applyLokiTokenFile(cfg *Config) {
	if cfg.LokiToken != "" {
		return
	}
	for _, path := range lokiTokenFileCandidates() {
		if token := readTokenFile(path); token != "" {
			cfg.LokiToken = token
			return
		}
	}

	// Try SecretProvider as fallback
	if sp := getSecretProvider(); sp != nil {
		if token, err := sp.GetSecret("loki.token"); err == nil && token != "" {
			cfg.LokiToken = token
		}
	}
}

func applyLokiBasicAuthFile(cfg *Config) {
	if cfg.LokiUser != "" || cfg.LokiPass != "" {
		return
	}
	for _, path := range lokiBasicAuthFileCandidates() {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		parts := strings.SplitN(strings.TrimSpace(string(data)), ":", 2)
		if len(parts) != 2 {
			continue
		}
		cfg.LokiUser = parts[0]
		cfg.LokiPass = parts[1]
		return
	}

	// Try SecretProvider as fallback
	if sp := getSecretProvider(); sp != nil {
		if auth, err := sp.GetSecret("loki.basic"); err == nil && auth != "" {
			parts := strings.SplitN(auth, ":", 2)
			if len(parts) == 2 {
				cfg.LokiUser = parts[0]
				cfg.LokiPass = parts[1]
			}
		}
	}
}

func applyGrafanaTokenFile(cfg *Config) {
	if cfg.GrafanaToken != "" {
		return
	}
	for _, path := range grafanaTokenFileCandidates() {
		if token := readTokenFile(path); token != "" {
			cfg.GrafanaToken = token
			return
		}
	}

	// Try SecretProvider as fallback
	if sp := getSecretProvider(); sp != nil {
		if token, err := sp.GetSecret("grafana.token"); err == nil && token != "" {
			cfg.GrafanaToken = token
		}
	}
}

func applyGrafanaBasicAuthFile(cfg *Config) {
	if cfg.GrafanaUser != "" || cfg.GrafanaPass != "" {
		return
	}
	for _, path := range grafanaBasicAuthFileCandidates() {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		parts := strings.SplitN(strings.TrimSpace(string(data)), ":", 2)
		if len(parts) != 2 {
			continue
		}
		cfg.GrafanaUser = parts[0]
		cfg.GrafanaPass = parts[1]
		return
	}

	// Try SecretProvider as fallback
	if sp := getSecretProvider(); sp != nil {
		if auth, err := sp.GetSecret("grafana.basic"); err == nil && auth != "" {
			parts := strings.SplitN(auth, ":", 2)
			if len(parts) == 2 {
				cfg.GrafanaUser = parts[0]
				cfg.GrafanaPass = parts[1]
			}
		}
	}
}

func applyTraceTokenFile(cfg *Config) {
	if cfg.TraceToken != "" {
		return
	}
	for _, path := range traceTokenFileCandidates() {
		if token := readTokenFile(path); token != "" {
			cfg.TraceToken = token
			return
		}
	}

	// Try SecretProvider as fallback
	if sp := getSecretProvider(); sp != nil {
		if token, err := sp.GetSecret("trace.token"); err == nil && token != "" {
			cfg.TraceToken = token
		}
	}
}

func applyTraceBasicAuthFile(cfg *Config) {
	if cfg.TraceUser != "" || cfg.TracePass != "" {
		return
	}
	for _, path := range traceBasicAuthFileCandidates() {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		parts := strings.SplitN(strings.TrimSpace(string(data)), ":", 2)
		if len(parts) != 2 {
			continue
		}
		cfg.TraceUser = parts[0]
		cfg.TracePass = parts[1]
		return
	}

	// Try SecretProvider as fallback
	if sp := getSecretProvider(); sp != nil {
		if auth, err := sp.GetSecret("trace.basic"); err == nil && auth != "" {
			parts := strings.SplitN(auth, ":", 2)
			if len(parts) == 2 {
				cfg.TraceUser = parts[0]
				cfg.TracePass = parts[1]
			}
		}
	}
}

func readTokenFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// Profile-aware environment variable application
func applyProfileEnvOverrides(cfg *Config, profileName string) {
	// Apply profile-scoped env vars first
	profilePrefix := fmt.Sprintf("HEALTH_MONITOR_PROFILES__%s__", strings.ToUpper(profileName))
	
	if v := os.Getenv(profilePrefix + "PROMETHEUS_URL"); v != "" {
		cfg.PrometheusURL = v
	}
	if v := os.Getenv(profilePrefix + "PROMETHEUS_TOKEN"); v != "" {
		cfg.PrometheusToken = v
	}
	if v := os.Getenv(profilePrefix + "PROMETHEUS_USER"); v != "" {
		cfg.PrometheusUser = v
	}
	if v := os.Getenv(profilePrefix + "PROMETHEUS_PASS"); v != "" {
		cfg.PrometheusPass = v
	}
	if v := os.Getenv(profilePrefix + "LOKI_URL"); v != "" {
		cfg.LokiURL = v
	}
	if v := os.Getenv(profilePrefix + "LOKI_TOKEN"); v != "" {
		cfg.LokiToken = v
	}
	if v := os.Getenv(profilePrefix + "LOKI_USER"); v != "" {
		cfg.LokiUser = v
	}
	if v := os.Getenv(profilePrefix + "LOKI_PASS"); v != "" {
		cfg.LokiPass = v
	}
	if v := os.Getenv(profilePrefix + "GRAFANA_URL"); v != "" {
		cfg.GrafanaURL = v
	}
	if v := os.Getenv(profilePrefix + "GRAFANA_TOKEN"); v != "" {
		cfg.GrafanaToken = v
	}
	if v := os.Getenv(profilePrefix + "GRAFANA_USER"); v != "" {
		cfg.GrafanaUser = v
	}
	if v := os.Getenv(profilePrefix + "GRAFANA_PASS"); v != "" {
		cfg.GrafanaPass = v
	}
	if v := os.Getenv(profilePrefix + "TRACE_URL"); v != "" {
		cfg.TraceURL = v
	}
	if v := os.Getenv(profilePrefix + "TRACE_TOKEN"); v != "" {
		cfg.TraceToken = v
	}
	if v := os.Getenv(profilePrefix + "TRACE_USER"); v != "" {
		cfg.TraceUser = v
	}
	if v := os.Getenv(profilePrefix + "TRACE_PASS"); v != "" {
		cfg.TracePass = v
	}
	if v := os.Getenv(profilePrefix + "API_SERVICE"); v != "" {
		cfg.APIService = v
	}
	if v := os.Getenv(profilePrefix + "API_ROUTE"); v != "" {
		cfg.APIRoute = v
	}
	if v := os.Getenv(profilePrefix + envMetricSchema); v != "" {
		cfg.MetricSchema = strings.ToLower(v)
	}
	
	// Then apply global env vars for backward compatibility
	applyEnv(cfg)
}

// Profile-aware token file application
func applyProfileTokenFiles(cfg *Config, profileName string) {
	pm := GetProfileManager()
	statePath := pm.getStatePath(profileName)
	
	// Check profile-specific token files first
	if cfg.PrometheusToken == "" {
		profileTokenFile := filepath.Join(statePath, "prometheus.token")
		if token := readTokenFile(profileTokenFile); token != "" {
			cfg.PrometheusToken = token
		}
	}
	
	if cfg.PrometheusUser == "" || cfg.PrometheusPass == "" {
		profileBasicAuthFile := filepath.Join(statePath, "prometheus.basic")
		if data, err := os.ReadFile(profileBasicAuthFile); err == nil {
			parts := strings.SplitN(strings.TrimSpace(string(data)), ":", 2)
			if len(parts) == 2 {
				cfg.PrometheusUser = parts[0]
				cfg.PrometheusPass = parts[1]
			}
		}
	}
	
	if cfg.LokiToken == "" {
		profileTokenFile := filepath.Join(statePath, "loki.token")
		if token := readTokenFile(profileTokenFile); token != "" {
			cfg.LokiToken = token
		}
	}
	
	if cfg.GrafanaToken == "" {
		profileTokenFile := filepath.Join(statePath, "grafana.token")
		if token := readTokenFile(profileTokenFile); token != "" {
			cfg.GrafanaToken = token
		}
	}
	
	if cfg.TraceToken == "" {
		profileTokenFile := filepath.Join(statePath, "trace.token")
		if token := readTokenFile(profileTokenFile); token != "" {
			cfg.TraceToken = token
		}
	}

	// Load Slack Webhook URL from state directory
	if cfg.Notifications.Slack.WebhookURL == "" {
		slackWebhookFile := filepath.Join(statePath, "slack.webhook")
		if webhook := readTokenFile(slackWebhookFile); webhook != "" {
			cfg.Notifications.Slack.WebhookURL = webhook
		}
	}
	// Support both nested and flat structure during migration
	if cfg.Notifications.SlackWebhookURL == "" && cfg.Notifications.Slack.WebhookURL != "" {
		cfg.Notifications.SlackWebhookURL = cfg.Notifications.Slack.WebhookURL
	}

	// Load PagerDuty Routing Key from state directory
	if cfg.Notifications.PagerDuty.RoutingKey == "" {
		pdKeyFile := filepath.Join(statePath, "pagerduty.key")
		if key := readTokenFile(pdKeyFile); key != "" {
			cfg.Notifications.PagerDuty.RoutingKey = key
		}
	}
	
	// Fallback to global token files
	applyTokenFile(cfg)
	applyBasicAuthFile(cfg)
	applyLokiTokenFile(cfg)
	applyLokiBasicAuthFile(cfg)
	applyGrafanaTokenFile(cfg)
	applyGrafanaBasicAuthFile(cfg)
	applyTraceTokenFile(cfg)
	applyTraceBasicAuthFile(cfg)
	
	// Prefer basic auth when present
	if cfg.PrometheusUser != "" || cfg.PrometheusPass != "" {
		cfg.PrometheusToken = ""
	}
	if cfg.LokiUser != "" || cfg.LokiPass != "" {
		cfg.LokiToken = ""
	}
	if cfg.GrafanaUser != "" || cfg.GrafanaPass != "" {
		cfg.GrafanaToken = ""
	}
	if cfg.TraceUser != "" || cfg.TracePass != "" {
		cfg.TraceToken = ""
	}
	
	// Sanitize URLs
	cfg.PrometheusURL = sanitizeURL(cfg.PrometheusURL)
	cfg.LokiURL = sanitizeURL(cfg.LokiURL)
	cfg.GrafanaURL = sanitizeURL(cfg.GrafanaURL)
	cfg.TraceURL = sanitizeURL(cfg.TraceURL)
}

// SaveProfileSecrets saves tokens and basic auth to the profile's state directory
func SaveProfileSecrets(profileName string, cfg Config) error {
	pm := GetProfileManager()
	statePath := pm.getStatePath(profileName)
	
	if err := os.MkdirAll(statePath, 0755); err != nil {
		return err
	}
	
	// Prometheus
	if cfg.PrometheusToken != "" {
		if err := os.WriteFile(filepath.Join(statePath, "prometheus.token"), []byte(cfg.PrometheusToken+"\n"), 0600); err != nil {
			return err
		}
	}
	if cfg.PrometheusUser != "" || cfg.PrometheusPass != "" {
		auth := fmt.Sprintf("%s:%s\n", cfg.PrometheusUser, cfg.PrometheusPass)
		if err := os.WriteFile(filepath.Join(statePath, "prometheus.basic"), []byte(auth), 0600); err != nil {
			return err
		}
	}
	
	// Loki
	if cfg.LokiToken != "" {
		if err := os.WriteFile(filepath.Join(statePath, "loki.token"), []byte(cfg.LokiToken+"\n"), 0600); err != nil {
			return err
		}
	}
	if cfg.LokiUser != "" || cfg.LokiPass != "" {
		auth := fmt.Sprintf("%s:%s\n", cfg.LokiUser, cfg.LokiPass)
		if err := os.WriteFile(filepath.Join(statePath, "loki.basic"), []byte(auth), 0600); err != nil {
			return err
		}
	}
	
	// Grafana
	if cfg.GrafanaToken != "" {
		if err := os.WriteFile(filepath.Join(statePath, "grafana.token"), []byte(cfg.GrafanaToken+"\n"), 0600); err != nil {
			return err
		}
	}
	if cfg.GrafanaUser != "" || cfg.GrafanaPass != "" {
		auth := fmt.Sprintf("%s:%s\n", cfg.GrafanaUser, cfg.GrafanaPass)
		if err := os.WriteFile(filepath.Join(statePath, "grafana.basic"), []byte(auth), 0600); err != nil {
			return err
		}
	}
	
	// Trace
	if cfg.TraceToken != "" {
		if err := os.WriteFile(filepath.Join(statePath, "trace.token"), []byte(cfg.TraceToken+"\n"), 0600); err != nil {
			return err
		}
	}
	if cfg.TraceUser != "" || cfg.TracePass != "" {
		auth := fmt.Sprintf("%s:%s\n", cfg.TraceUser, cfg.TracePass)
		if err := os.WriteFile(filepath.Join(statePath, "trace.basic"), []byte(auth), 0600); err != nil {
			return err
		}
	}
	
	return nil
}

// SaveSlackWebhook saves a Slack webhook URL to the profile's state directory
func SaveSlackWebhook(profileName string, webhookURL string) error {
	pm := GetProfileManager()
	statePath := pm.getStatePath(profileName)
	if err := os.MkdirAll(statePath, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(statePath, "slack.webhook"), []byte(webhookURL+"\n"), 0600)
}

// SavePagerDutyKey saves a PagerDuty routing key to the profile's state directory
func SavePagerDutyKey(profileName string, routingKey string) error {
	pm := GetProfileManager()
	statePath := pm.getStatePath(profileName)
	if err := os.MkdirAll(statePath, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(statePath, "pagerduty.key"), []byte(routingKey+"\n"), 0600)
}

// SaveNewProfile saves configuration for a new profile
func SaveNewProfile(profileName string, cfg Config) error {
	pm := GetProfileManager()
	
	if profileName == "" {
		profileName = "default"
	}
	
	// Save secrets securely first
	if err := SaveProfileSecrets(profileName, cfg); err != nil {
		return fmt.Errorf("failed to save profile secrets: %w", err)
	}

	if cfg.Notifications.Slack.WebhookURL != "" {
		if err := SaveSlackWebhook(profileName, cfg.Notifications.Slack.WebhookURL); err != nil {
			return fmt.Errorf("failed to save slack webhook: %w", err)
		}
	} else if cfg.Notifications.SlackWebhookURL != "" {
		if err := SaveSlackWebhook(profileName, cfg.Notifications.SlackWebhookURL); err != nil {
			return fmt.Errorf("failed to save slack webhook: %w", err)
		}
	}

	if cfg.Notifications.PagerDuty.RoutingKey != "" {
		if err := SavePagerDutyKey(profileName, cfg.Notifications.PagerDuty.RoutingKey); err != nil {
			return fmt.Errorf("failed to save pagerduty key: %w", err)
		}
	}
	
	// Sanitize config before saving to YAML
	safeCfg := cfg
	safeCfg.PrometheusToken = ""
	safeCfg.PrometheusUser = ""
	safeCfg.PrometheusPass = ""
	safeCfg.LokiToken = ""
	safeCfg.LokiUser = ""
	safeCfg.LokiPass = ""
	safeCfg.GrafanaToken = ""
	safeCfg.GrafanaUser = ""
	safeCfg.GrafanaPass = ""
	safeCfg.TraceToken = ""
	safeCfg.TraceUser = ""
	safeCfg.TracePass = ""
	safeCfg.Notifications.Slack.WebhookURL = ""
	safeCfg.Notifications.SlackWebhookURL = ""
	safeCfg.Notifications.PagerDuty.RoutingKey = ""

	// Create profile object
	profile := &Profile{
		Name:       profileName,
		Config:     safeCfg,
		Region:     cfg.EKS.Region,
		Namespaces: cfg.EKS.Namespaces,
		Provider:   cfg.Provider,
	}
	if profile.Provider == "" && cfg.EKS.Enabled {
		profile.Provider = "eks"
	}
	
	// Add to manager's internal map
	pm.mutex.Lock()
	pm.profiles[profileName] = profile
	pm.mutex.Unlock()
	
	return pm.saveProfile(profileName)
}

// TestGetConfigBasePath returns the base path for configuration files (for testing)
func TestGetConfigBasePath() string {
	return getConfigBasePath()
}

// TestSetConfigBasePath sets the base path for configuration files (for testing)
func TestSetConfigBasePath(path string) {
	testConfigBasePath = path
	// Reset global manager so it re-initializes with the new path
	profileManagerMu.Lock()
	globalProfileManager = nil
	profileManagerMu.Unlock()
}
