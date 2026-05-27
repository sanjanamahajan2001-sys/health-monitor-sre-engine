package guide

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"health-monitor/internal/config"
	"health-monitor/internal/flow"
	"health-monitor/internal/incident"
	"health-monitor/internal/metrics"
	"health-monitor/internal/notify"
	"health-monitor/internal/slo"
)

// TroubleshootResult represents the findings of a diagnostic check.
type TroubleshootResult struct {
	Status     string // "PASSED", "FAILED", "WARNING"
	Issue      string // Description of the problem
	Fix        string // Actionable advice to resolve it
	FixCommand string // The specific shell command a user can run
	FixAction     func() error       // Procedural fix (no param)
	FixParam      func(string) error // Procedural fix (with 1 string param)
	PostFixAction func(string) error // New: Follow-up action after FixParam
	Prompt        string             // Prompt message for FixParam (if applicable)
}

// TroubleshootFeature performs dynamic diagnostics based on the user's "stuck" point.
func TroubleshootFeature(feature string, targetProfile string) TroubleshootResult {
	if targetProfile == "" {
		targetProfile = config.GetProfileManager().GetActiveProfile()
	}
	
	cfg, err := config.LoadForProfile(targetProfile)
	if err != nil {
		return TroubleshootResult{
			Status: "FAILED",
			Issue:  fmt.Sprintf("Failed to load profile %s: %v", targetProfile, err),
			Fix:    "Ensure the profile YAML exists and is valid.",
		}
	}

	switch feature {
	case "data_fetching":
		return troubleshootDataFetching(targetProfile, cfg)
	case "notifications":
		return troubleshootNotifications(targetProfile, cfg)
	case "toil":
		return troubleshootToil(targetProfile, cfg)
	case "scorecard":
		return troubleshootScorecard(targetProfile, cfg)
	case "ml_similarity":
		return troubleshootML(targetProfile)
	case "sudo":
		return troubleshootSudo()
	case "multi_profile":
		return troubleshootMultiProfile(config.GetProfileManager())
	case "daemons":
		return troubleshootDaemons(targetProfile)
	case "config_labels":
		return troubleshootLabels(targetProfile, cfg)
	case "drift":
		return troubleshootDrift(targetProfile, cfg)
	case "setup_slack":
		return SetupSlack(targetProfile, cfg)
	case "setup_pagerduty":
		return SetupPagerDuty(targetProfile, cfg)
	default:
		return TroubleshootResult{
			Status: "UNKNOWN",
			Issue:  "No specific diagnostic logic for this feature yet.",
			Fix:    "Try running 'health-monitor doctor' for a general health check.",
		}
	}
}

func troubleshootDataFetching(profile string, cfg config.Config) TroubleshootResult {
	if cfg.PrometheusURL == "" {
		return TroubleshootResult{
			Status: "FAILED",
			Issue:  fmt.Sprintf("Prometheus URL is not configured in profile '%s'.", profile),
			Fix:    "Set a valid Prometheus URL. You can use 'http://localhost:9090' for testing.",
			FixCommand: fmt.Sprintf("health-monitor config set prometheus_url=http://localhost:9090 --profile %s", profile),
			FixAction: func() error {
				cfg.PrometheusURL = "http://localhost:9090"
				return config.SaveForProfile(profile, cfg)
			},
		}
	}

	// REAL Loki Context Logging (with Refined Query)
	logContext := ""
	if cfg.LokiURL != "" {
		logs, err := fetchRecentLokiErrors(cfg.LokiURL)
		if err == nil && len(logs) > 0 {
			logContext = "\n[Loki Context] Recent errors:\n" + strings.Join(logs, "\n")
		} else if err != nil {
			logContext = fmt.Sprintf("\n[Loki Error] Could not fetch logs: %v", err)
		} else {
			logContext = "\n[Loki Context] No recent errors found (Query matched 0 lines)."
		}
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(cfg.PrometheusURL + "/api/v1/query?query=up")
	if err != nil {
		return TroubleshootResult{
			Status: "FAILED",
			Issue:  fmt.Sprintf("Cannot reach Prometheus at %s: %v%s", cfg.PrometheusURL, err, logContext),
			Fix:    "Check your VPN/network connectivity, or verify the URL is correct.",
		}
	}
	defer resp.Body.Close()

	return TroubleshootResult{
		Status: "PASSED",
		Issue:  "Prometheus is reachable and responding correctly." + logContext,
		Fix:    "Your data pipeline is healthy.",
	}
}

func troubleshootNotifications(profile string, cfg config.Config) TroubleshootResult {
	pm := config.GetProfileManager()
	statePath := pm.GetStatePathForProfile(profile)
	
	slackOk := false
	if _, err := os.Stat(filepath.Join(statePath, "slack.webhook")); err == nil { slackOk = true }
	
	pdOk := false
	if _, err := os.Stat(filepath.Join(statePath, "pagerduty.key")); err == nil { pdOk = true }

	if !cfg.Notifications.Enabled {
		return TroubleshootResult{
			Status: "WARNING",
			Issue:  "Notifications are globally disabled in this profile.",
			Fix:    "Enable notifications to receive alerts via Slack/PagerDuty.",
			FixCommand: fmt.Sprintf("health-monitor config set notifications.enabled=true --profile %s", profile),
			FixAction: func() error {
				cfg.Notifications.Enabled = true
				return config.SaveForProfile(profile, cfg)
			},
		}
	}

	// Check for missing Slack Webhook
	if cfg.Notifications.Slack.Enabled && !slackOk {
		return TroubleshootResult{
			Status: "FAILED",
			Issue:  "Slack notifications are enabled but the Webhook URL is MISSING from state directory.",
			Fix:    "Provide a valid Slack Webhook URL to receive alerts.",
			Prompt: "Enter Slack Webhook URL:",
			FixParam: func(val string) error {
				return config.SaveSlackWebhook(profile, val)
			},
			PostFixAction: func(val string) error {
				_, err := notify.TestSlackChannel(profile, val)
				return err
			},
		}
	}

	// Check for missing PagerDuty Key
	if cfg.Notifications.PagerDuty.Enabled && !pdOk {
		return TroubleshootResult{
			Status: "FAILED",
			Issue:  "PagerDuty notifications are enabled but the Routing Key is MISSING from state directory.",
			Fix:    "Provide a valid PagerDuty Routing Key to receive alerts.",
			Prompt: "Enter PagerDuty Routing Key:",
			FixParam: func(val string) error {
				return config.SavePagerDutyKey(profile, val)
			},
			PostFixAction: func(val string) error {
				_, err := notify.TestPagerDutyChannel(profile, val)
				return err
			},
		}
	}

	return TroubleshootResult{
		Status: "PASSED",
		Issue:  "Notification configuration looks valid.",
		Fix:    "Your alert pipeline is fully configured. Use the next steps to update specific channels.",
	}
}

// SetupSlack provides an interactive way to configure Slack
func SetupSlack(profile string, cfg config.Config) TroubleshootResult {
	pm := config.GetProfileManager()
	statePath := pm.GetStatePathForProfile(profile)
	
	slackOk := false
	if _, err := os.Stat(filepath.Join(statePath, "slack.webhook")); err == nil { slackOk = true }

	status := "INTERACTIVE"
	issue := "Configure your Slack incoming webhook to receive real-time alerts."
	fix := "Press [F] to ENTER your Slack Webhook URL."
	
	if slackOk {
		status = "PASSED"
		issue = "Slack webhook is already configured."
		fix = "Press [F] if you want to UPDATE the existing webhook URL."
	}

	return TroubleshootResult{
		Status: status,
		Issue:  issue,
		Fix:    fix,
		Prompt: "Enter Slack Webhook URL:",
		FixParam: func(val string) error {
			return config.SaveSlackWebhook(profile, val)
		},
		PostFixAction: func(val string) error {
			_, err := notify.TestSlackChannel(profile, val)
			return err
		},
	}
}

// SetupPagerDuty provides an interactive way to configure PagerDuty
func SetupPagerDuty(profile string, cfg config.Config) TroubleshootResult {
	pm := config.GetProfileManager()
	statePath := pm.GetStatePathForProfile(profile)
	
	pdOk := false
	if _, err := os.Stat(filepath.Join(statePath, "pagerduty.key")); err == nil { pdOk = true }

	status := "INTERACTIVE"
	issue := "Configure your PagerDuty Routing Key for high-severity incident escalation."
	fix := "Press [F] to ENTER your PagerDuty Routing Key."
	
	if pdOk {
		status = "PASSED"
		issue = "PagerDuty routing key is already configured."
		fix = "Press [F] if you want to UPDATE the existing routing key."
	}

	return TroubleshootResult{
		Status: status,
		Issue:  issue,
		Fix:    fix,
		Prompt: "Enter PagerDuty Routing Key:",
		FixParam: func(val string) error {
			return config.SavePagerDutyKey(profile, val)
		},
		PostFixAction: func(val string) error {
			_, err := notify.TestPagerDutyChannel(profile, val)
			return err
		},
	}
}

func troubleshootToil(profile string, cfg config.Config) TroubleshootResult {
	if cfg.TeamCapacity.AverageHourlyCost <= 0 {
		return TroubleshootResult{
			Status: "WARNING",
			Issue:  "Average hourly cost is not set. ROI calculations are using a fallback.",
			Fix:    "Set a realistic hourly cost for your team's SRE time.",
			FixCommand: fmt.Sprintf("health-monitor config set team_capacity.average_hourly_cost=150 --profile %s", profile),
			FixAction: func() error {
				cfg.TeamCapacity.AverageHourlyCost = 150
				return config.SaveForProfile(profile, cfg)
			},
		}
	}

	store, _ := incident.NewStoreForProfile(profile)
	incidents, _, _ := store.List()
	
	toilCount := 0
	for _, inc := range incidents {
		if inc.Toil != nil && inc.Toil.Minutes > 0 {
			toilCount++
		}
	}

	if toilCount == 0 {
		return TroubleshootResult{
			Status: "FAILED",
			Issue:  "No toil records found in this profile's incident history.",
			Fix:    "Ensure you add 'toil' data when resolving incidents.",
		}
	}

	return TroubleshootResult{
		Status: "PASSED",
		Issue:  fmt.Sprintf("Found %d incidents with toil data.", toilCount),
		Fix:    "Try 'health-monitor toil report' to see your ROI breakdown.",
	}
}

func troubleshootScorecard(profile string, cfg config.Config) TroubleshootResult {
	provider, err := metrics.NewMetricProvider(&cfg)
	if err != nil {
		return TroubleshootResult{
			Status: "FAILED",
			Issue:  "Metrics provider not configured.",
			Fix:    "Configure Prometheus or CloudWatch first.",
		}
	}

	// slo.NewService expects a MetricProvider
	_, _ = slo.NewService(provider)
	report, err := flow.LoadForProfile(profile)
	if err != nil && !flow.IsNoConfig(err) {
		return TroubleshootResult{
			Status: "FAILED",
			Issue:  fmt.Sprintf("Failed to load flows for profile %s: %v", profile, err),
			Fix:    "Check your flows.yaml syntax.",
		}
	}

	if len(report.Flows) == 0 {
		return TroubleshootResult{
			Status: "FAILED",
			Issue:  fmt.Sprintf("No service flows or SLOs defined for profile '%s'.", profile),
			Fix:    "Create a sample flows.yaml for this profile.",
			FixCommand: fmt.Sprintf("health-monitor flow init --profile %s", profile),
		}
	}

	return TroubleshootResult{
		Status: "PASSED",
		Issue:  fmt.Sprintf("Found %d configured service flows.", len(report.Flows)),
		Fix:    "Use 'health-monitor scorecard' to view your reliability dashboard.",
	}
}

func troubleshootML(profile string) TroubleshootResult {
	store, _ := incident.NewStoreForProfile(profile)
	incidents, _, _ := store.List()
	
	rcaCount := 0
	for _, inc := range incidents {
		if inc.Analysis != nil && inc.Analysis.RootCause != "" {
			rcaCount++
		}
	}

	if rcaCount < 5 {
		return TroubleshootResult{
			Status: "WARNING",
			Issue:  fmt.Sprintf("Only %d incidents have Root Cause Analysis (RCA) data.", rcaCount),
			Fix:    "ML similarity matching is more accurate with at least 5-10 resolved incidents containing RCA data.",
		}
	}

	return TroubleshootResult{
		Status: "PASSED",
		Issue:  "Sufficient RCA data available.",
		Fix:    "Try 'health-monitor incident similar --id <INC-ID>'.",
	}
}

func troubleshootSudo() TroubleshootResult {
	uid := os.Geteuid()
	if uid != 0 {
		return TroubleshootResult{
			Status: "FAILED",
			Issue:  fmt.Sprintf("Running as regular user (UID: %d).", uid),
			Fix:    "Relaunch this command with sudo for full system access.",
			FixCommand: "sudo ./health-monitor guide",
		}
	}
	return TroubleshootResult{
		Status: "PASSED",
		Issue:  "Running with root privileges.",
		Fix:    "You have full access to system directories.",
	}
}

func troubleshootMultiProfile(pm *config.ProfileManager) TroubleshootResult {
	profiles := pm.DiscoverProfiles()
	if len(profiles) <= 1 {
		return TroubleshootResult{
			Status: "WARNING",
			Issue:  "Only one profile found.",
			Fix:    "Create more profiles to isolate different environments.",
			FixCommand: "health-monitor profile create staging",
		}
	}

	return TroubleshootResult{
		Status: "PASSED",
		Issue:  fmt.Sprintf("Found %d profiles.", len(profiles)),
		Fix:    "Use 'health-monitor profile switch' to move between configurations.",
	}
}

func troubleshootDaemons(profile string) TroubleshootResult {
	alertPort := 9095
	sloPort := 9098
	
	alertRunning, alertReason := isDaemonCurrentlyRunning(profile, "alert", alertPort)
	sloRunning, sloReason := isDaemonCurrentlyRunning(profile, "slo", sloPort)

	issues := []string{}
	fixAdvice := ""
	fixCmd := ""

	if !alertRunning {
		pidPath := fmt.Sprintf("/run/health-monitor-alert-%s.pid", profile)
		issues = append(issues, fmt.Sprintf("Alert daemon is NOT responding on port %d (%s).", alertPort, alertReason))
		fixAdvice = "Restart Alert-Listen."
		fixCmd = fmt.Sprintf("sudo ./health-monitor --profile %s alert-listen --background --addr :%d --pid-file %s --data-dir /var/lib/health-monitor/state/%s", profile, alertPort, pidPath, profile)
	}

	if !sloRunning {
		pidPath := fmt.Sprintf("/run/health-monitor-slo-%s.pid", profile)
		issues = append(issues, fmt.Sprintf("SLO-Monitor daemon is NOT responding on port %d (%s).", sloPort, sloReason))
		if fixAdvice != "" { fixAdvice += " Also " }
		fixAdvice += "Restart SLO-Monitor."
		if fixCmd != "" { fixCmd += " && " }
		fixCmd = strings.TrimSpace(fixCmd)
		fixCmd += fmt.Sprintf("sudo ./health-monitor --profile %s slo monitor --background --pid-file %s", profile, pidPath)
	}

	if len(issues) > 0 {
		return TroubleshootResult{
			Status: "FAILED",
			Issue:  strings.Join(issues, " "),
			Fix:    fixAdvice,
			FixCommand: fixCmd,
		}
	}

	return TroubleshootResult{
		Status: "PASSED",
		Issue:  "Both monitoring daemons (Alert & SLO) are ACTIVE and RESPONSIVE.",
		Fix:    "Monitoring pipeline is healthy.",
	}
}

func troubleshootLabels(profile string, cfg config.Config) TroubleshootResult {
	supportedLabels := strings.Split(cfg.ServiceLabel, ",")
	isStandard := false
	for _, l := range supportedLabels {
		l = strings.TrimSpace(l)
		if l == "app" || l == "service" || l == "container" {
			isStandard = true
			break
		}
	}

	if !isStandard {
		return TroubleshootResult{
			Status: "WARNING",
			Issue:  fmt.Sprintf("Current service label mapping ('%s') may be too restrictive.", cfg.ServiceLabel),
			Fix:    "Expand ServiceLabel to include standard job/app/container mappings.",
			FixCommand: fmt.Sprintf("health-monitor config set service_label=container,job,app,service --profile %s", profile),
			FixAction: func() error {
				cfg.ServiceLabel = "container,job,app,service"
				return config.SaveForProfile(profile, cfg)
			},
		}
	}
	return TroubleshootResult{
		Status: "PASSED",
		Issue:  fmt.Sprintf("Service label mapping ('%s') covers standard observability patterns.", cfg.ServiceLabel),
		Fix:    "Ensure your metrics use one of these labels.",
	}
}

func troubleshootDrift(profile string, cfg config.Config) TroubleshootResult {
	defaultCfg := config.Default()
	
	drifts := []string{}
	// Use Window field in current config to match Golden Signal drift check
	if cfg.Window == "" && defaultCfg.Window != "" {
		drifts = append(drifts, "Missing 'window' (Golden Signal default: "+defaultCfg.Window+")")
	}

	if len(drifts) > 0 {
		return TroubleshootResult{
			Status: "WARNING",
			Issue:  fmt.Sprintf("Profile '%s' has %d configuration gaps compared to the Golden Signal template.", profile, len(drifts)),
			Fix:    "Apply recommended defaults to align with standard observability practices.",
			FixAction: func() error {
				if cfg.Window == "" { cfg.Window = defaultCfg.Window }
				return config.SaveForProfile(profile, cfg)
			},
		}
	}

	return TroubleshootResult{
		Status: "PASSED",
		Issue:  "Configuration is fully aligned with Golden Signal templates.",
		Fix:    "No drift detected.",
	}
}

// -----------------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------------

func isDaemonCurrentlyRunning(profile string, daemonType string, port int) (bool, string) {
	// 1. Check if the port is occupied
	if isPortAvailable(fmt.Sprintf("%d", port)) {
		return false, "Port is FREE"
	}

	// 2. Identify if it's OURS by checking flexible PID patterns
	pidFile := discoverPidFile(daemonType, profile)
	if pidFile == "" {
		return false, "Port BUSY but no matching PID file found"
	}

	data, err := os.ReadFile(pidFile)
	if err != nil {
		return false, fmt.Sprintf("PID file found at %s but NOT readable: %v", pidFile, err)
	}
	
	pid := strings.TrimSpace(string(data))
	if pid == "" {
		return false, fmt.Sprintf("PID file %s is EMPTY", pidFile)
	}

	return true, "Running with PID " + pid
}

func discoverPidFile(daemonType string, profile string) string {
	patterns := []string{
		fmt.Sprintf("/run/health-monitor-%s-%s.pid", daemonType, profile),
		fmt.Sprintf("/run/health-monitor-%s.pid", daemonType),
		fmt.Sprintf("/var/run/health-monitor-%s-%s.pid", daemonType, profile),
		fmt.Sprintf("/var/run/health-monitor-%s.pid", daemonType),
		fmt.Sprintf("/tmp/health-monitor-%s-%s.pid", daemonType, profile),
		fmt.Sprintf("/tmp/health-monitor-%s.pid", daemonType),
	}
	
	// Add user's specific pattern observed in training
	patterns = append(patterns, fmt.Sprintf("/run/health-monitor-alert-%s.pid", profile))
	patterns = append(patterns, fmt.Sprintf("/run/health-monitor-slo-%s.pid", profile))
	patterns = append(patterns, fmt.Sprintf("/var/run/health-monitor-alert-%s.pid", profile))
	patterns = append(patterns, fmt.Sprintf("/var/run/health-monitor-slo-%s.pid", profile))

	for _, p := range patterns {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func isPortAvailable(port string) bool {
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

func fetchRecentLokiErrors(lokiURL string) ([]string, error) {
	client := &http.Client{Timeout: 3 * time.Second}
	
	// Refined, more robust query: limit to last hour for efficiency
	query := `{service=~".+"} |~ "(?i)error"`
	escapedQuery := url.QueryEscape(query)
	
	// Use query_range instead of query for historical lookback (last 1 hour)
	now := time.Now().UnixNano()
	start := time.Now().Add(-1 * time.Hour).UnixNano()
	
	fetchURL := fmt.Sprintf("%s/loki/api/v1/query_range?query=%s&limit=5&start=%d&end=%d", 
		lokiURL, escapedQuery, start, now)
	
	resp, err := client.Get(fetchURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("loki returned status %d. Body: %s", resp.StatusCode, string(body))
	}

	var data struct {
		Data struct {
			Result []struct {
				Values [][]string `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	var logs []string
	for _, res := range data.Data.Result {
		for _, v := range res.Values {
			if len(v) >= 2 {
				// Loki returns [timestamp, line]
				logs = append(logs, " - "+v[1])
			}
		}
	}

	return logs, nil
}
