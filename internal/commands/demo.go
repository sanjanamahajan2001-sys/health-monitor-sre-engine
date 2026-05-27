package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"health-monitor/internal/backend"
	"health-monitor/internal/checks"
	"health-monitor/internal/config"
	"health-monitor/internal/incident"
	"health-monitor/internal/output"
	"health-monitor/internal/tui"
	"health-monitor/pkg/model"
)

// HandleDemoCommand initializes the sandbox environment and launches the demo
func HandleDemoCommand(args []string) int {
	fmt.Println("🚀 Initializing Demo Sandbox environment...")

	// Check for CLI mode flag
	forceCLI := false
	for _, arg := range args {
		if arg == "--force-cli" || arg == "-c" {
			forceCLI = true
			break
		}
	}
	
	if forceCLI {
		fmt.Println("📱 Forced CLI mode - skipping TUI")
	}

	// Create temporary sandbox directory
	sandboxDir, err := CreateTempSandbox()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create sandbox: %v\n", err)
		return 1
	}
	fmt.Printf("📁 Created temporary sandbox: %s\n", sandboxDir)

	// Set environment variables EARLY to ensure all components use the sandbox
	os.Setenv("HEALTH_MONITOR_PROFILE", "demo")
	os.Setenv("HEALTH_MONITOR_CONFIG_DIR", filepath.Join(sandboxDir, "config"))
	
	// Reset the global profile manager to ensure it re-initializes with the new config dir
	config.TestSetConfigBasePath(filepath.Join(sandboxDir, "config"))
	config.Reset()

	// Ensure demo profile exists in the sandbox
	pm := config.GetProfileManager()
	if _, err := pm.GetProfile("demo"); err != nil {
		if err := config.SaveForProfile("demo", config.Default()); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create demo profile: %v\n", err)
			return 1
		}
	}

	if err := pm.SetActiveProfile("demo"); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to activate demo profile: %v\n", err)
		return 1
	}

	// Ensure clean slate
	statePath := pm.GetStatePathForProfile("demo")
	os.RemoveAll(statePath)
	os.MkdirAll(statePath, 0755)

	// Create a new incident store for the demo profile
	incidentStore, err := incident.NewStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize incident store: %v\n", err)
		return 1
	}

	// Seed all demo incidents directly into the store
	err = SeedDemoData(incidentStore, sandboxDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to seed demo data: %v\n", err)
		return 1
	}
	fmt.Println("🌱 Seeded demo incidents with RCA and action items")

	// Create incident service and connect fake backend
	service, err := incident.NewService(incidentStore)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create incident service: %v\n", err)
		return 1
	}

	// Initialize fake backend and connect it to the incident service
	demoBackend := backend.NewDemoBackend()
	fmt.Println("🎭 Initialized fake metrics/logs/traces backend")
	
	// Connect fake backend to incident service for rich incident views
	ConnectFakeBackendToService(service, demoBackend)

	// Set demo mode flag for TUI
	os.Setenv("HEALTH_MONITOR_DEMO_MODE", "true")

	// Construct basic system report wrapper
	report := model.Report{IsDemo: true}
	
	// Add System specs just to populate the top cards
	checks.System(&report)

	fmt.Println("✨ Demo Sandbox Ready!")

	// Load demo incidents from store
	incidents, warnings, err := service.List()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load demo incidents: %v\n", err)
		return 1
	}
	
	// Print warnings if any
	for _, warning := range warnings {
		fmt.Printf("Warning: %s\n", warning)
	}

	// Launch incident browser with service
	if forceCLI {
		fmt.Println("📱 Showing rich CLI incident view...")
		showDetailedIncidentView(incidents, service)
	} else {
		launchDemoIncidentBrowser(service)
	}

	fmt.Println("\n👋 Exited Demo Mode.")
	fmt.Printf("🗂️  Your demo sandbox state is preserved at: %s\n", statePath)
	fmt.Printf("🗑️  Temporary sandbox will be cleaned up on system restart\n")
	return 0
}

// CreateTempSandbox creates a temporary directory for the demo sandbox
func CreateTempSandbox() (string, error) {
	// Create a temporary directory with a predictable name
	tempDir := filepath.Join(os.TempDir(), "health-monitor-demo-"+time.Now().Format("20060102-150405"))
	
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create temp sandbox: %w", err)
	}
	
	// Create subdirectories for different components
	subdirs := []string{"config", "state", "logs", "metrics", "traces"}
	for _, subdir := range subdirs {
		if err := os.MkdirAll(filepath.Join(tempDir, subdir), 0755); err != nil {
			return "", fmt.Errorf("failed to create sandbox subdir %s: %w", subdir, err)
		}
	}
	
	return tempDir, nil
}

// launchDemoIncidentBrowser creates an interactive TUI with demo incidents
func launchDemoIncidentBrowser(service *incident.Service) {
	// Load demo incidents from store
	incidents, warnings, err := service.List()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load demo incidents: %v\n", err)
		return
	}
	
	// Print warnings if any
	for _, warning := range warnings {
		fmt.Printf("Warning: %s\n", warning)
	}

	// Create demo report with fake backend integration
	report := CreateDemoReport()
	
	// Launch the standard interactive TUI (it will load incidents from the store)
	// Check if we're in a terminal that supports TUI
	if os.Getenv("SSH_CONNECTION") != "" || os.Getenv("TERM") == "dumb" {
		fmt.Println("⚠️  Detected SSH/dumb terminal - TUI may not work properly")
		fmt.Println("🔄 Showing CLI incident details instead...")
		showDetailedIncidentView(incidents, service)
		return
	}
	
	// Set environment variable to indicate demo mode
	os.Setenv("HEALTH_MONITOR_DEMO_MODE", "true")

	err = tui.PrintInteractiveTUI(report)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running demo TUI: %v\n", err)
		fmt.Println("🔄 Falling back to CLI incident display...")
		showDetailedIncidentView(incidents, service)
	} else {
		fmt.Println("✅ TUI completed successfully")
		fmt.Println("🤔 If you didn't see anything, the TUI might have started and exited immediately")
		fmt.Println("💡 Try running: TERM=xterm ./health-monitor demo")
		fmt.Println("💡 Or use CLI mode: ./health-monitor demo --force-cli")
	}
}

// ConnectFakeBackendToService integrates the fake backend with the incident service
func ConnectFakeBackendToService(service *incident.Service, demoBackend *backend.DemoBackend) {
	// This would typically connect the backend to the service
	// For now, we'll just set environment variables to indicate demo mode
	os.Setenv("HEALTH_MONITOR_DEMO_BACKEND", "true")
	os.Setenv("HEALTH_MONITOR_DEMO_METRICS", "true")
	os.Setenv("HEALTH_MONITOR_DEMO_LOGS", "true")
	os.Setenv("HEALTH_MONITOR_DEMO_TRACES", "true")
}

// showDetailedIncidentView displays rich incident details in CLI format
func showDetailedIncidentView(incidents []incident.Incident, service *incident.Service) {
	fmt.Printf("\n📋 Detailed Demo Incident Views (%d incidents)\n", len(incidents))
	fmt.Println("═════════════════════════════════════════════════════════════════")
	
	for i, inc := range incidents {
		fmt.Printf("\n🎯 Incident %d/%d: %s\n", i+1, len(incidents), inc.ID)
		fmt.Printf("📝 %s\n", inc.Title)
		fmt.Printf("🔧 Service: %s | 🚨 Severity: %s | 📊 State: %s\n", 
			inc.Service, inc.Severity, inc.State)
		fmt.Println(strings.Repeat("─", 70))
		
		// Show full incident details using the enhanced CLI formatter
		fullDetails := incident.FormatIncidentViewCLI(inc)
		fmt.Print(fullDetails)
		
		// Show additional demo features info
		fmt.Println("\n💡 Demo Features Available:")
		fmt.Println("   🔗 Similar incidents: Use 'health-monitor incident similar --id " + inc.ID + "'")
		fmt.Println("   📚 Runbook suggestions: Use 'health-monitor runbook suggest --id " + inc.ID + "'")
		fmt.Println("   🎯 Full incident view: Use 'health-monitor incident view --id " + inc.ID + "'")
		
		if i < len(incidents)-1 {
			fmt.Printf("\n%s\n", strings.Repeat("═", 70))
		}
	}
	
	fmt.Println("\n✅ Demo incident viewing completed!")
	fmt.Println("💡 Additional demo commands to try:")
	fmt.Println("   • View individual incidents: ./health-monitor incident view --id <ID>")
	fmt.Println("   • Find similar incidents: ./health-monitor incident similar --id <ID>")
	fmt.Println("   • Get runbook suggestions: ./health-monitor runbook suggest --id <ID>")
	fmt.Println("   • Export incident data: ./health-monitor incident export --id <ID>")
	fmt.Println("   • Generate runbook: ./health-monitor runbook generate --pattern <pattern>")
}

// CreateDemoReport creates a demo report with fake backend data
func CreateDemoReport() model.Report {
	report := model.Report{IsDemo: true}
	
	// Add System specs to populate top cards
	checks.System(&report)

	// Override with fake metrics for consistent demo experience (matches actual system TUI)
	report.Metrics = []model.Metric{
		{Name: "Disk Usage", Value: "14G / 1007G (1%)"},
		{Name: "Memory Usage", Value: "Available: 6960 MB / 7943 MB"},
		{Name: "GPU Temp", Value: "N/A"},
		{Name: "GPU Note", Value: "No GPU present"},
		{Name: "Disk I/O Wait", Value: "0.00% (SAFE)"},
		{Name: "Zombie Processes", Value: "0 (SAFE)"},
		{Name: "System Uptime", Value: "1.6 hours"},
		{Name: "Load Average (1m, 5m, 15m)", Value: "0.00, 0.03, 0.00"},
		{Name: "CPU Usage", Value: "2.4%"},
	}

	// 1. API Latency - The core of the dashboard
	report.APILatency = &model.APILatency{
		Service:           "checkout",
		Route:             "/api/v1/process-payment",
		Window:            "30m",
		P95:               3450.0,
		P90:               2800.0,
		P99:               4200.0,
		BaselineP95:       850.0,
		DeltaP95:          305.8, // % increase
		RPS:               124.5,
		BaselineRPS:       118.2,
		DeltaRPS:          5.3,
		ErrorRate:         4.2,
		BaselineErrorRate: 0.1,
		DeltaErrorRate:    4100.0,
		Status:            model.RISK,
		Note:              "Significant latency regression and error spike detected in checkout service. Correlates with upstream timeout errors.",
		TopEndpoints: []model.EndpointLatency{
			{Route: "/api/v1/process-payment", P95: 3450.0},
			{Route: "/api/v1/add-to-cart", P95: 120.0},
			{Route: "/api/v1/inventory", P95: 45.0},
		},
		TopEndpointsRPS: []model.EndpointRate{
			{Route: "/api/v1/process-payment", RPS: 45.2},
			{Route: "/api/v1/add-to-cart", RPS: 312.4},
		},
		RemediationHints: []string{
			"Check Stripe API status page - reports of intermittent latency",
			"Review deployment 'checkout-v2.4.1' - potential regression in JSON parsing",
			"Scaling group 'checkout-pro' is at 95% CPU; consider increasing max-capacity",
		},
	}

	// 2. APM / Dependency Analysis
	report.APM = &model.DependencyAnalysis{
		SuspectedUpstream:     "stripe_api (External)",
		CorrelationConfidence: "High",
		Window:                "30m",
		Evidence: []string{
			"Stripe API latency increased from 400ms to 2.8s",
			"Correlation of 0.94 between checkout error rate and Stripe timeout errors",
			"5xx errors in billing_service match time-series of Stripe rate-limit events",
		},
		RankedCauses: []model.DependencyCause{
			{Path: "checkout -> billing_service -> stripe_api", Score: 0.94, Confidence: "High"},
			{Path: "checkout -> postgres_db", Score: 0.12, Confidence: "Low"},
		},
		NextChecks: []string{
			"Verify if fallback payment provider is receiving traffic",
			"Check billing_service sidecar logs for rate-limit details",
		},
	}

	// 3. Tracing Summary
	report.Tracing = &model.TraceSummary{
		Service: "checkout",
		Window:  "30m",
		Backend: "Demo (Jaeger)",
		TopSlowTraces: []model.TraceItem{
			{
				TraceID:       "5f3a9e1d2c4b8a7f",
				TotalDuration: "3.45s",
				RootOperation: "POST /api/v1/process-payment",
				SlowestSpan: model.TraceSpanSummary{
					Service:   "billing_service",
					Operation: "stripe.charge.create",
					Duration:  "2.8s",
				},
			},
			{
				TraceID:       "9d8f7e6c5b4a3210",
				TotalDuration: "2.1s",
				RootOperation: "POST /api/v1/process-payment",
				SlowestSpan: model.TraceSpanSummary{
					Service:   "checkout",
					Operation: "validate_cart",
					Duration:  "0.4s",
				},
			},
		},
		Links: []model.TraceLink{
			{Label: "View p99 Traces", URL: "http://demo-jaeger.local/search?service=checkout&minDuration=3s"},
			{Label: "Critical Path Analysis", URL: "http://demo-jaeger.local/trace/5f3a9e1d2c4b8a7f"},
		},
	}

	// 4. Correlation Summary (Signatures)
	report.CorrelationSummary = &model.CorrelationSummary{
		Backend:    "Demo (Loki)",
		Window:     "30m",
		Query:      "{service=\"checkout\"} | pattern `<_> <_> <_> <_> <_> <_> <_>`",
		Confidence: "High",
		Notes: []string{
			"Significant increase in 'stripe_timeout' signatures in checkout logs",
			"Correlates with 5xx error spike in billing service",
		},
		Signatures: []model.CorrelationSignature{
			{Signature: "error: request to stripe timed out after 2.5s", Count: 145, Percent: 68.2},
			{Signature: "warning: retrying payment intent: %s", Count: 42, Percent: 19.5},
			{Signature: "info: processed payment: %s", Count: 25, Percent: 11.8},
		},
	}

	// 5. No Data Reasons (to show reasoning panel)
	report.APILatency.NoDataReasons = []model.NoDataReason{
		{
			Area:         "Baseline",
			Reason:       "Historical baseline for /api/v1/refund is unavailable for this window",
			Metric:       "http_request_duration_seconds",
			SuggestedFix: "Verify recording rules for 7d baselines are active in Prometheus",
		},
	}

	// 6. Config Summary (to suppress health warnings)
	report.APIConfig = &model.APIConfigSummary{
		PrometheusURL: "http://demo-prometheus.local",
		APIService:     "checkout",
		APIRoute:      "/api/v1/process-payment",
		HasToken:      true,
		TokenSource:   "Demo",
		Window:        "30m",
	}

	// 7. Summary Table Items (System Metrics)
	report.Summary = []model.SummaryItem{
		{Name: "DISK", Status: model.SAFE, Value: "14G/1007G"},
		{Name: "MEMORY", Status: model.SAFE, Value: "6960Mi/7943Mi"},
		{Name: "GPU", Status: model.SAFE, Value: "N/A"},
		{Name: "SSH", Status: model.SAFE, Value: "OK"},
	}
	
	return report
}

// SeedScenarioData seeds specific scenario-driven data into the sandbox.
func SeedScenarioData(service *incident.Service, scenario string) error {
	output.Silence()
	defer output.Unsilence()

	switch scenario {
	case "label_drift":
		// Create a specific training flow with mismatched labels
		pm := config.GetProfileManager()
		statePath := pm.GetStatePathForProfile(pm.GetActiveProfile())
		trainingFlowPath := filepath.Join(statePath, "flows.d", "training_drift.yaml")
		
		content := `flows:
  training_auth:
    name: "Training Auth Service"
    services: ["auth-svc"]
    slos:
      - id: "latency"
        service: "auth-svc"
        objective: 99.9
        type: "latency"
        promql: "http_request_duration_seconds_bucket{service=\"auth-svc\"}"`
		
		_ = os.MkdirAll(filepath.Join(statePath, "flows.d"), 0755)
		_ = os.WriteFile(trainingFlowPath, []byte(content), 0644)
		return nil

	case "cascading_failure":
		// Seed a P1 incident with multiple linked services
		if service != nil {
			inc, _, _ := service.Start("api-gateway", incident.P1, "CRITICAL: Cascading Latency Spike", "guide-bot")
			// Add a comment about suspected upstream
			_, _, _ = service.AddEventByID(inc.ID, incident.EventNote, "Suspected bottleneck in payment-svc (downstream)", "system")
			return nil
		}
		return nil

	case "incident_replay":
		// Seed a historical incident for practice
		if service != nil {
			inc, _, _ := service.Start("shipping-db", incident.P2, "HISTORICAL: Database Connection Pool Exhausted", "guide-bot")
			// Add some "history"
			_, _, _ = service.AddEventByID(inc.ID, incident.EventNote, "Connection count increased 400% in 5 minutes", "monitor")
			return nil
		}
		return nil
	}
	return nil
}
