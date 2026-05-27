package guide

// GetScenarios returns the predefined scenarios for the guided tour.
func GetScenarios(seeder Seeder) []Scenario {
	return []Scenario{
		{
			ID:          "novice",
			Name:        "🆕 The Novice: Getting Started",
			Description: "First-time setup? Learn how to initialize your profile (Linux/K8s) and run health checks.",
			Keywords:    []string{"start", "setup", "init", "wizard", "first", "new", "onboarding"},
			Chapters: []Chapter{
				{
					ID:    "setup",
					Title: "Profile Initialization",
					Steps: []Step{
						{
							Title: "Init vs Wizard",
							Content: "init (Dynamic Discovery): Scans your actual infra/Prometheus (2-5 mins) for 100% specific config.\n\n--wizard-team (Presets): Quick standardized templates (10-30 secs) for common setups like microservices.",
							Keywords: []string{"difference", "wizard", "init"},
							Hint:     "The 'init' command uses the discovery engine to find 100% of your metrics automatically. The 'wizard-team' uses predefined O11y templates (Prometheus/CloudWatch) to get you started in seconds.",
						},
						{
							Title: "Kubernetes Setup",
							Content: "Command: `sudo ./health-monitor --init --infra kubernetes --kubeconfig ~/.kube/config`",
							Keywords: []string{"k8s", "kubernetes", "kubeconfig", "clusters"},
						},
						{
							Title: "Linux/VM Setup",
							Content: "Command: `./health-monitor --init`",
							Keywords: []string{"linux", "vm", "bare-metal", "server"},
						},
						{
							Title: "Verification",
							Content: "1. `health-monitor doctor` (Check connections)\n2. `health-monitor flow list` (Check services)\n3. `health-monitor flow validate` (Check flow config)\n4. `health-monitor alert list-rules` (Check alert rules)\n5. `health-monitor alert validate-config` (Check alert config)",
							Keywords: []string{"doctor", "validate", "check", "verify", "health"},
						},
					},
				},
			},
		},
		{
			ID:          "labs",
			Name:        "🧪 SRE Training Labs",
			Description: "Learn by doing. Solve real-world reliability mysteries in a safe sandbox.",
			Keywords:    []string{"lab", "training", "learn", "mystery", "solve", "practice"},
			Chapters: []Chapter{
				{
					ID:    "lab_hygiene",
					Title: "Lab 1: The Label Mystery",
					Steps: []Step{
						{
							Title: "Data Hygiene & Mapping",
							Content: "Goal: Fix a 'No Data' issue caused by a metric label mismatch.\n\nSimulating Label Drift...",
							Keywords: []string{"label", "drift", "mapping", "hygiene"},
							Action: func(m *UnifiedGuideModel, s *Step) error {
								_ = seeder.Seed("label_drift")
								s.Content = "⚠️  ALERT: Scenario Loaded.\n\nYour config expects 'service', but metrics use 'app'.\nNavigate to the Troubleshooter to find and apply the fix!"
								return nil
							},
						},
					},
				},
				{
					ID:    "lab_observability",
					Title: "Lab 2: Cascading Failure",
					Steps: []Step{
						{
							Title: "Trace the Ripple Effect",
							Content: "Goal: Identify a downstream dependency failure using Traces.\n\nSimulating Cascading Latency...",
							Keywords: []string{"cascade", "failure", "trace", "dependency"},
							Action: func(m *UnifiedGuideModel, s *Step) error {
								_ = seeder.Seed("cascading_failure")
								s.Content = "🔥 ALERT: payment-svc is slow, causing api-gateway timeouts.\n\nUse 'health-monitor incident start --collaborative' to debug with your team!"
								return nil
							},
						},
					},
				},
				{
					ID:    "lab_practice",
					Title: "Lab 3: Incident Replay",
					Steps: []Step{
						{
							Title: "Safe Resolution Practice",
							Content: "Goal: Resolve a historical high-severity incident in sandbox mode.\n\nLoading Historical Incident INC-742...",
							Keywords: []string{"replay", "history", "practice", "incident"},
							Action: func(m *UnifiedGuideModel, s *Step) error {
								_ = seeder.Seed("incident_replay")
								s.Content = "📦 INC-742 LOADED.\n\nRoot Cause: Out of Memory on shipping-db.\nPractice your resolution flow: 'incident start' -> 'incident resolve'."
								return nil
							},
						},
					},
				},
			},
		},
		{
			ID:          "encyclopedia",
			Name:        "📚 Command Encyclopedia",
			Description: "A complete list of ALL commands and flags, classified for quick reference.",
			Keywords:    []string{"help", "commands", "flags", "list", "usage", "search", "reference", "manual"},
			Chapters: []Chapter{
				{
					ID:    "setup_cmds",
					Title: "Setup & Discovery",
					Steps: []Step{
						{
							Title: "Init & Profile",
							Content: "`--init`, `--wizard-team`, `profile list`, `profile switch`, `profile create <name>`",
							Keywords: []string{"profile", "switch", "create", "list"},
						},
						{
							Title: "Connectivity & Security",
							Content: "`doctor`, `flow validate`, `alert validate-config`, `config validate` \n\nSecurity: `security validate-config`, `security status`, `security test`, `security generate-key` ",
							Keywords: []string{"connectivity", "validation", "security", "doctor", "health", "config"},
						},
						{
							Title: "Global Flags",
							Content: "Commonly used flags across commands:\n`--profile <name>` (override active profile)\n`--id <incident-id>` (target specific incident)\n`--pid <file-path>` (custom pid file location)\n`--format <json|md>` (output format)",
							Keywords: []string{"flags", "global", "profile", "id", "pid", "format"},
						},
					},
				},
				{
					ID:    "incident_cmds",
					Title: "Incident & Reliability",
					Steps: []Step{
						{
							Title: "Incidents",
							Content: "`incident start`, `incident resolve`, `incident list`, `incident similar`, `incident note`, `incident postmortem` \n\nViews: `incident view <id>`, `incident view --tui <id>` \n\nExports: `incident export --format markdown`, `incident export --format json` ",
							Keywords: []string{"incident", "resolution", "start", "resolve", "similar", "note", "postmortem", "view", "export", "markdown", "json"},
						},
						{
							Title: "SLO & Monitoring",
							Content: "`flow list`, `alert list-rules` \n\nReliability Daemons: \n`alert-listen`, `slo monitor` \n\nCheck Status: \n`alert status --pid-file /run/health-monitor-alert-<profile>.pid` \n`slo monitor status --pid-file /run/health-monitor-slo-<profile>.pid` \n\nFlags: `--background`, `--addr :9095`, `--pid-file <path>`, `--data-dir <path>`",
							Keywords: []string{"slo", "alerts", "monitor", "daemon", "background", "listen", "status"},
						},
						{
							Title: "Toil & Prevention",
							Content: "`toil list`, `toil analyze`, `toil report`, `prevent list`, `prevent describe <id>` \n\nExample: `health-monitor prevent describe PRED-123456789`",
							Keywords: []string{"toil", "prevention", "analyze", "report", "describe"},
						},
						{
							Title: "Runbook Automation",
							Content: "`runbook suggest <inc-id>`, `runbook generate --incident <inc-id> --save`, `runbook list`, `runbook pattern` \n\nExample Case (Payment Processor): \n`health-monitor runbook suggest INC-20260324-052436` \n\n💡 To generate and save this runbook, run:\n`health-monitor runbook generate --pattern \"payment processor unreachable\" --service orders_service --save` \nOR \n`health-monitor runbook generate --incident INC-20260324-052436 --save` ",
							Keywords: []string{"runbook", "suggest", "generate", "list", "pattern"},
						},
						{
							Title: "SRE Knowledge Base",
							Content: "Connecting to internal Git repo for private runbooks...",
							Keywords: []string{"knowledge", "base", "git", "runbook", "internal", "private"},
							Action: func(m *UnifiedGuideModel, s *Step) error {
								s.Content = "✅ CONNECTED: Internal Runbook Repository.\n\nFound 12 private runbooks for 'payment-svc'.\nUse 'health-monitor runbook suggest --private' to view them."
								return nil
							},
						},
					},
				},
				{
					ID:    "admin_cmds",
					Title: "Governance & Power Features",
					Steps: []Step{
						{
							Title: "Organization",
							Content: "`scorecard`, `scorecard --org`, `scorecard --profile <name>`",
							Keywords: []string{"scorecard", "org", "governance", "organization"},
						},
						{
							Title: "Collaboration",
							Content: "`incident start --collaborative` \n\nCollaborative Flags: `--ssh-port <port>`, `--ssh-host <host>`\n\n`feedback` (share agent experience)",
							Keywords: []string{"collaborative", "tunnel", "feedback", "ssh", "port", "host"},
						},
					},
				},
			},
		},
		{
			ID:          "troubleshooter",
			Name:        "🛠️  Dynamic Troubleshooter",
			Description: "Stuck? Let's analyze your environment and apply proactive fixes.",
			Keywords:    []string{"fix", "stuck", "error", "broken", "debug", "issue", "troubleshoot", "remediation"},
			Chapters: []Chapter{
				{
					ID:    "pipeline",
					Title: "Pipeline & Environment",
					Steps: []Step{
						{
							Title:   "Sudo & Permissions",
							Content: "Analyzing current UID and directory access...",
							Keywords: []string{"sudo", "user", "permissions", "uid", "access", "root"},
							Action: func(m *UnifiedGuideModel, s *Step) error {
								m.LastResult = new(TroubleshootResult)
								*m.LastResult = TroubleshootFeature("sudo", m.SelectedProfile)
								return nil
							},
						},
						{
							Title:   "Prometheus/Loki Probing",
							Content: "Probing backend connectivity and auth status...",
							Keywords: []string{"prometheus", "loki", "fetching", "connection", "http", "timeout", "url"},
							Action: func(m *UnifiedGuideModel, s *Step) error {
								m.LastResult = new(TroubleshootResult)
								*m.LastResult = TroubleshootFeature("data_fetching", m.SelectedProfile)
								return nil
							},
							Hint: "Loki queries use LogQL. We probe backend health using the /api/v1/query?query={service=~\".+\"} endpoint to ensure your logs are reaching the aggregator.",
						},
						{
							Title:   "Metric Labels",
							Content: "Searching for misconfigured service identifiers...",
							Keywords: []string{"labels", "service_label", "metric", "wrong", "mismatch", "mapping"},
							Action: func(m *UnifiedGuideModel, s *Step) error {
								m.LastResult = new(TroubleshootResult)
								*m.LastResult = TroubleshootFeature("config_labels", m.SelectedProfile)
								return nil
							},
						},
						{
							Title:   "Configuration Drift",
							Content: "Comparing active profile against Golden Signal defaults...",
							Keywords: []string{"drift", "config", "default", "standard", "missing"},
							Action: func(m *UnifiedGuideModel, s *Step) error {
								m.LastResult = new(TroubleshootResult)
								*m.LastResult = TroubleshootFeature("drift", m.SelectedProfile)
								return nil
							},
						},
					},
				},
				{
					ID:    "features",
					Title: "Feature Data & Quality",
					Steps: []Step{
						{
							Title:   "Toil & ROI Accuracy",
							Content: "Analyzing incident toil metadata and team capacity...",
							Keywords: []string{"toil", "roi", "savings", "minutes", "cost", "accuracy", "empty"},
							Action: func(m *UnifiedGuideModel, s *Step) error {
								m.LastResult = new(TroubleshootResult)
								*m.LastResult = TroubleshootFeature("toil", m.SelectedProfile)
								return nil
							},
						},
						{
							Title:   "Scorecard & SLOs",
							Content: "Verifying SLO density and service name linkage...",
							Keywords: []string{"scorecard", "empty", "slo", "objective", "reliability", "status"},
							Action: func(m *UnifiedGuideModel, s *Step) error {
								m.LastResult = new(TroubleshootResult)
								*m.LastResult = TroubleshootFeature("scorecard", m.SelectedProfile)
								return nil
							},
						},
						{
							Title:   "ML & Similar Incidents",
							Content: "Evaluating RCA data quality for similarity matching...",
							Keywords: []string{"ml", "similarity", "confidence", "rca", "root cause", "history"},
							Action: func(m *UnifiedGuideModel, s *Step) error {
								m.LastResult = new(TroubleshootResult)
								*m.LastResult = TroubleshootFeature("ml_similarity", m.SelectedProfile)
								return nil
							},
						},
					},
				},
				{
					ID:    "governance",
					Title: "Governance & Operations",
					Steps: []Step{
						{
							Title:   "Daemon Vitality",
							Content: "Checking Alert-Listen and SLO-Monitor background processes...",
							Keywords: []string{"daemon", "background", "process", "zombie", "pid", "9095", "9098", "running"},
							Action: func(m *UnifiedGuideModel, s *Step) error {
								m.LastResult = new(TroubleshootResult)
								*m.LastResult = TroubleshootFeature("daemons", m.SelectedProfile)
								return nil
							},
							Hint: "Daemon vitality is checked by verifying port occupancy (9095/9098) and matching the PID file in /run/health-monitor-{type}-{profile}.pid.",
						},
						{
							Title:   "Multi-Profile Health",
							Content: "Iterating through all profiles to find data gaps...",
							Keywords: []string{"multi-profile", "profiles", "switch", "active", "gaps", "redundancy"},
							Action: func(m *UnifiedGuideModel, s *Step) error {
								m.LastResult = new(TroubleshootResult)
								*m.LastResult = TroubleshootFeature("multi_profile", m.SelectedProfile)
								return nil
							},
						},
						{
							Title:   "Notifications Health",
							Content: "Checking overall notification vitality and global settings...",
							Keywords: []string{"notifications", "status", "health", "global"},
							Action: func(m *UnifiedGuideModel, s *Step) error {
								m.LastResult = new(TroubleshootResult)
								*m.LastResult = TroubleshootFeature("notifications", m.SelectedProfile)
								return nil
							},
						},
						{
							Title:   "Setup Slack",
							Content: "Interactive configuration for Slack alerts.",
							Keywords: []string{"slack", "webhook", "setup"},
							ManualInput: true,
							Action: func(m *UnifiedGuideModel, s *Step) error {
								m.LastResult = new(TroubleshootResult)
								*m.LastResult = TroubleshootFeature("setup_slack", m.SelectedProfile)
								return nil
							},
						},
						{
							Title:   "Setup PagerDuty",
							Content: "Interactive configuration for PagerDuty escalation.",
							Keywords: []string{"pagerduty", "routing", "key", "setup"},
							ManualInput: true,
							Action: func(m *UnifiedGuideModel, s *Step) error {
								m.LastResult = new(TroubleshootResult)
								*m.LastResult = TroubleshootFeature("setup_pagerduty", m.SelectedProfile)
								return nil
							},
						},
					},
				},
			},
		},
	}
}
