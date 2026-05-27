package notify

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"health-monitor/internal/config"
	"health-monitor/internal/output"
	"health-monitor/internal/notify/slack"
)

// HandleCLI handles notification CLI commands
func HandleCLI(args []string) int {
	if len(args) == 0 {
		printUsage()
		return 1
	}

	switch args[0] {
	case "enable":
		if len(args) < 2 {
			output.Errorf("notification type required")
			output.Errorf("Usage: health-monitor notifications enable <slack|pagerduty>")
			return 1
		}
		return handleEnable(args[1])
	case "disable":
		if len(args) < 2 {
			output.Errorf("notification type required")
			output.Errorf("Usage: health-monitor notifications disable <slack|pagerduty>")
			return 1
		}
		return handleDisable(args[1])
	case "pagerduty":
		if len(args) < 2 {
			output.Errorf("pagerduty subcommand required")
			output.Errorf("Usage: health-monitor notifications pagerduty <enable|disable|set-key>")
			return 1
		}
		return handlePagerDuty(args[1:])
	case "test":
		if len(args) < 2 {
			output.Errorf("notification type required")
			output.Errorf("Usage: health-monitor notifications test <slack|pagerduty>")
			return 1
		}
		return handleTest(args[1:])
	case "status":
		return handleStatus(args[1:])
	default:
		output.Errorf("unknown command '%s'", args[0])
		printUsage()
		return 1
	}
}

func printUsage() {
	output.Infof(`Usage: health-monitor notifications <command>

Commands:
  enable <slack|pagerduty>   Enable notifications
  disable <slack|pagerduty>  Disable notifications
  pagerduty <subcommand>     PagerDuty-specific commands
  test <slack|pagerduty>     Send test notification
  status                     Show notification status

Examples:
  health-monitor notifications enable slack
  health-monitor notifications pagerduty set-key R0XXXXXXXXXXXXX
  health-monitor notifications test slack
  health-monitor notifications status`)
}

func handleEnable(service string) int {
	if service != "slack" && service != "pagerduty" {
		output.Errorf("only 'slack' and 'pagerduty' notifications are supported")
		return 1
	}

	if service == "pagerduty" {
		return handlePagerDutyEnable()
	}

	// Load current config
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to load config: %v\n", err)
		return 1
	}

	// Prompt for webhook URL
	fmt.Print("Enter Slack webhook URL: ")
	reader := bufio.NewReader(os.Stdin)
	webhookURL, err := reader.ReadString('\n')
	if err != nil {
		output.Errorf("failed to read input: %v", err)
		return 1
	}
	webhookURL = strings.TrimSpace(webhookURL)

	// Validate URL format
	if !strings.HasPrefix(webhookURL, "https://hooks.slack.com/") {
		output.Errorf("invalid Slack webhook URL format")
		return 1
	}

	// Test the webhook
	fmt.Print("Testing webhook...")
	testNotifier := slack.NewSlackNotifier(&slack.SlackConfig{
		Enabled:         true,
		WebhookURL:      webhookURL,
		NotifyOn:        []string{"start", "suggest", "ack", "resolve"},
		TimeoutSeconds:  3,
		MaxRetries:     3,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := testNotifier.TestConnection(ctx); err != nil {
		fmt.Printf(" failed: %v\n", err)
		output.Errorf("webhook test failed. Please check the URL and try again.")
		return 1
	}
	fmt.Println(" OK")

	// Update config (nested format)
	cfg.Notifications.Enabled = true
	cfg.Notifications.Slack.Enabled = true
	cfg.Notifications.Slack.WebhookURL = webhookURL

	// Save config
	if err := saveConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to save config: %v\n", err)
		return 1
	}

	output.Infof("Slack notifications enabled successfully!")
	return 0
}

func handleDisable(service string) int {
	if service != "slack" && service != "pagerduty" {
		output.Errorf("only 'slack' and 'pagerduty' notifications are supported")
		return 1
	}

	if service == "pagerduty" {
		return handlePagerDutyDisable()
	}

	// Load current config
	cfg, err := config.Load()
	if err != nil {
		output.Errorf("failed to load config: %v", err)
		return 1
	}

	// Update config (disable but keep URL)
	cfg.Notifications.Slack.Enabled = false

	// Save config
	if err := saveConfig(cfg); err != nil {
		output.Errorf("failed to save config: %v", err)
		return 1
	}

	output.Infof("Slack notifications disabled successfully!")
	return 0
}

func handleStatus(args []string) int {
	cfg, err := config.Load()
	if err != nil {
		output.Errorf("failed to load config: %v", err)
		return 1
	}

	pm := config.GetProfileManager()
	activeProfile := pm.GetActiveProfile()

	output.Infof("Profile: %s", activeProfile)
	output.Infof("Notifications Status:")
	output.Infof("  Slack: %s", formatStatus(cfg.Notifications.Slack.Enabled))
	if cfg.Notifications.Slack.WebhookURL != "" {
		output.Infof("    Webhook: configured")
	}
	
	output.Infof("  PagerDuty: %s", formatStatus(cfg.Notifications.PagerDuty.Enabled))
	if cfg.Notifications.PagerDuty.RoutingKey != "" {
		output.Infof("    Routing Key: configured")
	}
	
	if len(cfg.Notifications.NotifyOn) > 0 {
		output.Infof("  Events: %s", strings.Join(cfg.Notifications.NotifyOn, ", "))
	}

	return 0
}

func formatStatus(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

func saveConfig(cfg config.Config) error {
	// Use profile-aware save instead of custom file write
	return config.Save(cfg)
}
