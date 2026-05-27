package slack

import (
	"context"
	"health-monitor/internal/output"
)

// JiraNotifier implements the Notifier interface for Jira
type JiraNotifier struct {
	url        string
	user       string
	token      string
	projectKey string
	enabled    bool
}

// NewJiraNotifier creates a new Jira notifier
func NewJiraNotifier(url, user, token, projectKey string, enabled bool) *JiraNotifier {
	return &JiraNotifier{
		url:        url,
		user:       user,
		token:      token,
		projectKey: projectKey,
		enabled:    enabled,
	}
}

// Notify creates or updates a Jira issue
func (j *JiraNotifier) Notify(ctx context.Context, event NotificationEvent) error {
	if !j.enabled {
		output.Debugf("Jira notifier disabled, skipping")
		return nil
	}

	if j.url == "" || j.projectKey == "" {
		output.Warnf("Jira notifier enabled but URL or ProjectKey is missing")
		return nil
	}

	output.Debugf("Jira Notify called for incident %s (State: %s)", event.Incident.ID, event.Type)

	// TODO: Implement actual Jira API calls
	// For now, we just log since the user requested it to be disabled by default
	// and we don't have a real Jira instance to test against.
	
	switch event.Type {
	case "started":
		output.Infof("[Jira] Would create issue in project %s for incident %s: %s", j.projectKey, event.Incident.ID, event.Incident.Title)
	case "resolved":
		output.Infof("[Jira] Would resolve issue for incident %s", event.Incident.ID)
	}

	return nil
}
