package notify

import (
	"context"
	"fmt"
	"health-monitor/internal/output"
	"time"

	"health-monitor/internal/config"
	"health-monitor/internal/notify/slack"
)

// handleTest handles notification testing
func handleTest(args []string) int {
	if len(args) < 1 {
		output.Errorf("notification type required")
		output.Errorf("Usage: health-monitor notifications test <slack|pagerduty>")
		return 1
	}

	notifType := args[0]
	
	switch notifType {
	case "slack":
		msg, err := TestSlackChannel("", "") // Uses default config
		if err != nil {
			output.Errorf("%v", err)
			return 1
		}
		output.Infof("%s", msg)
		return 0
	case "pagerduty":
		msg, err := TestPagerDutyChannel("", "") // Uses default config
		if err != nil {
			output.Errorf("%v", err)
			return 1
		}
		output.Infof("%s", msg)
		return 0
	default:
		output.Errorf("unknown notification type '%s'", notifType)
		output.Errorf("Supported types: slack, pagerduty")
		return 1
	}
}

// TestSlackChannel sends a test notification to Slack and returns a status message
func TestSlackChannel(profile string, overrideWebhook string) (string, error) {
	var cfg config.Config
	var err error
	if profile != "" {
		cfg, err = config.LoadForProfile(profile)
	} else {
		cfg, err = config.Load()
	}
	
	if err != nil {
		return "", fmt.Errorf("error loading config: %v", err)
	}

	webhook := cfg.Notifications.Slack.WebhookURL
	if overrideWebhook != "" {
		webhook = overrideWebhook
	}

	if !cfg.Notifications.Slack.Enabled && overrideWebhook == "" {
		return "", fmt.Errorf("Slack notifications are not enabled")
	}

	if webhook == "" {
		return "", fmt.Errorf("Slack webhook URL not configured")
	}

	// Create test incident
	testIncident := slack.Incident{
		ID:       "TEST-" + time.Now().Format("20060102-150405"),
		Service:  "health-monitor-test",
		Severity: slack.SeverityP3,
		Title:    "Test Notification from Guided Tour",
		State:    slack.StateStarted,
		CreatedAt: time.Now(),
	}

	event := slack.NotificationEvent{
		Type:      "started",
		Incident:  testIncident,
		Timestamp: time.Now(),
	}

	// Create notifier
	slackCfg := slack.SlackConfig{
		Enabled:    true, // Force enabled for test
		WebhookURL: webhook,
		NotifyOn:   []string{"started"}, // Ensure it triggers
	}
	notifier := slack.NewSlackNotifier(&slackCfg)

	// Send test notification
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = notifier.Notify(ctx, event)
	if err != nil {
		return "", fmt.Errorf("failed to send Slack notification: %v", err)
	}

	return "Test notification sent successfully! Check your Slack channel.", nil
}

// TestPagerDutyChannel sends a test notification to PagerDuty and returns a status message
func TestPagerDutyChannel(profile string, overrideKey string) (string, error) {
	var cfg config.Config
	var err error
	if profile != "" {
		cfg, err = config.LoadForProfile(profile)
	} else {
		cfg, err = config.Load()
	}
	
	if err != nil {
		return "", fmt.Errorf("error loading config: %v", err)
	}

	key := cfg.Notifications.PagerDuty.RoutingKey
	if overrideKey != "" {
		key = overrideKey
	}

	if !cfg.Notifications.PagerDuty.Enabled && overrideKey == "" {
		return "", fmt.Errorf("PagerDuty notifications are not enabled")
	}

	if key == "" {
		return "", fmt.Errorf("PagerDuty routing key not configured")
	}

	// Create test incident
	testIncident := slack.Incident{
		ID:       "TEST-" + time.Now().Format("20060102-150405"),
		Service:  "health-monitor-test",
		Severity: slack.SeverityP3,
		Title:    "Test Notification from Guided Tour",
		State:    slack.StateStarted,
		CreatedAt: time.Now(),
	}

	event := slack.NotificationEvent{
		Type:      "started",
		Incident:  testIncident,
		Timestamp: time.Now(),
	}

	// Create notifier
	pdCfg := slack.PagerDutyConfig{
		Enabled:    true, // Force enabled for test
		RoutingKey: key,
	}
	notifier := slack.NewPagerDutyNotifier(&pdCfg)

	// Send test notification
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = notifier.Notify(ctx, event)
	if err != nil {
		return "", fmt.Errorf("failed to send PagerDuty event: %v", err)
	}

	return "Test event sent successfully! Check your PagerDuty dashboard.", nil
}
