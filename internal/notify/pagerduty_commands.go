package notify

import (
	"bufio"
	"fmt"
	"health-monitor/internal/output"
	"os"
	"strings"

	"health-monitor/internal/config"
)

// handlePagerDuty handles pagerduty subcommands
func handlePagerDuty(args []string) int {
	if len(args) == 0 {
		output.Errorf("pagerduty subcommand required")
		output.Errorf("Usage: health-monitor notifications pagerduty <enable|disable|set-key>")
		return 1
	}

	switch args[0] {
	case "enable":
		return handlePagerDutyEnable()
	case "disable":
		return handlePagerDutyDisable()
	case "set-key":
		return handlePagerDutySetKey()
	default:
		output.Errorf("unknown pagerduty subcommand '%s'", args[0])
		output.Errorf("Usage: health-monitor notifications pagerduty <enable|disable|set-key>")
		return 1
	}
}

func handlePagerDutyEnable() int {
	// Load current config
	cfg, err := config.Load()
	if err != nil {
		output.Errorf("failed to load config: %v", err)
		return 1
	}

	// Check if routing key is configured
	if cfg.Notifications.PagerDuty.RoutingKey == "" {
		output.Errorf("PagerDuty routing key not configured")
		output.Errorf("Run 'health-monitor notifications pagerduty set-key' first")
		return 1
	}

	// Enable PagerDuty
	cfg.Notifications.PagerDuty.Enabled = true

	// Save config
	if err := saveConfig(cfg); err != nil {
		output.Errorf("failed to save config: %v", err)
		return 1
	}

	output.Infof("PagerDuty notifications enabled successfully!")
	return 0
}

func handlePagerDutyDisable() int {
	// Load current config
	cfg, err := config.Load()
	if err != nil {
		output.Errorf("failed to load config: %v", err)
		return 1
	}

	// Disable PagerDuty (keep routing key)
	cfg.Notifications.PagerDuty.Enabled = false

	// Save config
	if err := saveConfig(cfg); err != nil {
		output.Errorf("failed to save config: %v", err)
		return 1
	}

	output.Infof("PagerDuty notifications disabled successfully!")
	return 0
}

func handlePagerDutySetKey() int {
	// Load current config
	cfg, err := config.Load()
	if err != nil {
		output.Errorf("failed to load config: %v", err)
		return 1
	}

	// Prompt for routing key
	fmt.Print("Enter PagerDuty routing key (integration key): ")
	reader := bufio.NewReader(os.Stdin)
	routingKey, err := reader.ReadString('\n')
	if err != nil {
		output.Errorf("failed to read input: %v", err)
		return 1
	}
	routingKey = strings.TrimSpace(routingKey)

	// Validate routing key (basic check - should be 32 chars)
	if len(routingKey) < 20 {
		output.Errorf("routing key seems too short. Please check and try again.")
		return 1
	}

	// Update config
	cfg.Notifications.PagerDuty.RoutingKey = routingKey
	
	// Apply defaults if not set
	if cfg.Notifications.PagerDuty.TimeoutSeconds <= 0 {
		cfg.Notifications.PagerDuty.TimeoutSeconds = 10
	}
	if cfg.Notifications.PagerDuty.MaxRetries <= 0 {
		cfg.Notifications.PagerDuty.MaxRetries = 3
	}
	if cfg.Notifications.PagerDuty.SeverityMap == nil {
		cfg.Notifications.PagerDuty.SeverityMap = map[string]string{
			"P1": "critical",
			"P2": "error",
			"P3": "warning",
		}
	}

	// Save config
	if err := saveConfig(cfg); err != nil {
		output.Errorf("failed to save config: %v", err)
		return 1
	}

	output.Infof("PagerDuty routing key configured successfully!")
	output.Infof("Run 'health-monitor notifications pagerduty enable' to enable notifications")
	return 0
}
