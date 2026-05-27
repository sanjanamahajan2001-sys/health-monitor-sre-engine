package scorecard

import (
	"context"
	"fmt"
	"log"
	"time"

	"health-monitor/internal/config"
	"health-monitor/internal/notify/slack"
	"math/rand"
)

// SendSlackSummary generates a summary for the given month and sends it to Slack
func (p *Provider) SendSlackSummary(ctx context.Context, profile string) error {
	now := time.Now()
	report, err := p.GenerateReport(profile, now.Year(), now.Month())
	if err != nil {
		return fmt.Errorf("failed to generate report for Slack: %w", err)
	}

	cfg, err := slack.LoadSlackConfig()
	if err != nil {
		return err
	}
	if !cfg.Enabled || cfg.WebhookURL == "" {
		return nil
	}

	notifier := slack.NewSlackNotifier(cfg)
	
	// Create a Slack-optimized message
	text := fmt.Sprintf("*Monthly Reliability Scorecard: %s*\n", report.Month)
	text += fmt.Sprintf("> *Overall Health Score*: %.1f%% (%s)\n", report.OverallScore, report.HealthStatus)
	text += fmt.Sprintf("> *Profile*: %s\n\n", report.Profile)
	
	text += "*Top Services:*\n"
	for i, s := range report.ServiceSummaries {
		if i >= 5 { break } // Limit to top 5
		text += fmt.Sprintf("• %s: %.2f%% Avail (%s MTTR)\n", s.Service, s.Availability, s.MTTR)
	}
	
	text += fmt.Sprintf("\n*Executive Summary*: %s", report.ExecutiveSummary.FocusArea)

	event := slack.NotificationEvent{
		Type:      "scorecard",
		Timestamp: time.Now(),
		Incident: slack.Incident{
			ID:      "SCORECARD-" + profile,
			Title:   "Monthly Reliability Snapshot",
			Summary: text,
		},
	}

	return notifier.Notify(ctx, event)
}

// StartScheduler (Internal use) could be hooked into a background loop
func StartScheduler(p *Provider) {
	ticker := time.NewTicker(12 * time.Hour)
	go func() {
		for range ticker.C {
			now := time.Now()
			// Only run on the 1st of the month, once per day check
			if now.Day() == 1 {
				log.Printf("INFO: 1st of the month detected. Triggering automated scorecard reports...")
				pm := config.GetProfileManager()
				profiles := pm.DiscoverProfiles()
				for _, profile := range profiles {
					// Add jitter to prevent thundering herd (up to 30 seconds wait)
					jitter := time.Duration(rand.Intn(30)) * time.Second
					time.Sleep(jitter)
					
					if err := p.SendSlackSummary(context.Background(), profile); err != nil {
						log.Printf("ERROR: Failed to send Slack scorecard for %s: %v", profile, err)
					}
				}
			}
		}
	}()
}
