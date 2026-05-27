package config

import (
	"fmt"
	"os"
)

// HandleCLI handles config-related CLI commands
func HandleCLI(args []string) int {
	if len(args) == 0 {
		printUsage()
		return 1
	}

	switch args[0] {
	case "validate":
		return handleValidate()
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown command '%s'\n", args[0])
		printUsage()
		return 1
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `Usage: health-monitor config <command>

Commands:
  validate    Validate configuration and show warnings

Examples:
  health-monitor config validate
`)
}

// handleValidate validates the configuration and shows detailed output
func handleValidate() int {
	cfg, err := Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error loading config: %v\n", err)
		return 1
	}

	fmt.Printf("Validating configuration...\n\n")

	hasErrors := false
	hasWarnings := false

	// Validate Prometheus
	fmt.Printf("📊 Prometheus:\n")
	if cfg.PrometheusURL == "" {
		fmt.Printf("  ⚠️  prometheus_url not configured\n")
		hasWarnings = true
	} else if !isValidURL(cfg.PrometheusURL) {
		fmt.Printf("  ❌ prometheus_url format is invalid: %s\n", cfg.PrometheusURL)
		hasErrors = true
	} else {
		fmt.Printf("  ✅ %s\n", cfg.PrometheusURL)
	}
	if cfg.PrometheusServiceLabel != "" {
		fmt.Printf("  ✅ Service label: %s\n", cfg.PrometheusServiceLabel)
	}

	// Validate Grafana
	fmt.Printf("\n📈 Grafana:\n")
	if cfg.GrafanaURL == "" {
		fmt.Printf("  ⚠️  grafana_url not configured\n")
		hasWarnings = true
	} else if !isValidURL(cfg.GrafanaURL) {
		fmt.Printf("  ❌ grafana_url format is invalid: %s\n", cfg.GrafanaURL)
		hasErrors = true
	} else {
		fmt.Printf("  ✅ %s\n", cfg.GrafanaURL)
		if cfg.GrafanaPromDataSource == "" {
			fmt.Printf("  ⚠️  grafana_prom_ds not configured\n")
			hasWarnings = true
		} else {
			fmt.Printf("  ✅ Prometheus datasource: %s\n", cfg.GrafanaPromDataSource)
		}
		if cfg.GrafanaLokiDataSource != "" {
			fmt.Printf("  ✅ Loki datasource: %s\n", cfg.GrafanaLokiDataSource)
		}
		if cfg.GrafanaTraceDataSource != "" {
			fmt.Printf("  ✅ Trace datasource: %s\n", cfg.GrafanaTraceDataSource)
		}
		fmt.Printf("  ✅ Org ID: %d\n", cfg.GrafanaOrgId)
	}

	// Validate Loki
	fmt.Printf("\n📝 Loki:\n")
	if cfg.LokiURL == "" && cfg.GrafanaLokiDataSource == "" {
		fmt.Printf("  ⚠️  No log backend configured\n")
		hasWarnings = true
	} else {
		if cfg.LokiURL != "" {
			if !isValidURL(cfg.LokiURL) {
				fmt.Printf("  ❌ loki_url format is invalid: %s\n", cfg.LokiURL)
				hasErrors = true
			} else {
				fmt.Printf("  ✅ %s\n", cfg.LokiURL)
			}
		}
		if cfg.LokiServiceLabel != "" {
			fmt.Printf("  ✅ Service label: %s\n", cfg.LokiServiceLabel)
		}
	}

	// Validate Traces
	fmt.Printf("\n🔍 Traces:\n")
	if cfg.TraceURL == "" && cfg.GrafanaTraceDataSource == "" {
		fmt.Printf("  ⚠️  No trace backend configured\n")
		hasWarnings = true
	} else {
		if cfg.TraceURL != "" {
			if !isValidURL(cfg.TraceURL) {
				fmt.Printf("  ❌ trace_url format is invalid: %s\n", cfg.TraceURL)
				hasErrors = true
			} else {
				fmt.Printf("  ✅ %s\n", cfg.TraceURL)
			}
		}
		if cfg.TraceServiceTag != "" {
			fmt.Printf("  ✅ Service tag: %s\n", cfg.TraceServiceTag)
		}
		if cfg.TraceTimeUnit != "" {
			fmt.Printf("  ✅ Time unit: %s\n", cfg.TraceTimeUnit)
		}
	}

	// Validate Notifications
	fmt.Printf("\n🔔 Notifications:\n")
	if !cfg.Notifications.Enabled {
		fmt.Printf("  ⚠️  Notifications disabled\n")
	} else {
		fmt.Printf("  ✅ Notifications enabled\n")
		
		if cfg.Notifications.Slack.Enabled {
			fmt.Printf("  ✅ Slack enabled\n")
			if cfg.Notifications.Slack.WebhookURL == "" {
				fmt.Printf("    ❌ Slack webhook URL not configured\n")
				hasErrors = true
			}
		}
		
		if cfg.Notifications.PagerDuty.Enabled {
			fmt.Printf("  ✅ PagerDuty enabled\n")
			if cfg.Notifications.PagerDuty.RoutingKey == "" {
				fmt.Printf("    ❌ PagerDuty routing key not configured\n")
				hasErrors = true
			}
		}
	}

	// Observability settings
	fmt.Printf("\n⚙️  Observability Settings:\n")
	fmt.Printf("  ✅ Lookback: %d minutes\n", cfg.ObservabilityLookbackMinutes)

	// Summary
	fmt.Printf("\n==================================================\n")
	if hasErrors {
		fmt.Printf("❌ Configuration has errors\n")
		return 1
	} else if hasWarnings {
		fmt.Printf("⚠️  Configuration valid with warnings\n")
		return 0
	} else {
		fmt.Printf("✅ Configuration is valid\n")
		return 0
	}
}
