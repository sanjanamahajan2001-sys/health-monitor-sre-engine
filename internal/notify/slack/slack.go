package slack

import (
	"context"
	"fmt"
	"health-monitor/internal/output"
	"strings"
	"time"

	"health-monitor/internal/config"
	"health-monitor/internal/traces"
)

// Severity represents incident severity levels
type Severity string

const (
	SeverityP1 Severity = "P1"
	SeverityP2 Severity = "P2"
	SeverityP3 Severity = "P3"
)

// State represents incident states
type State string

const (
	StateStarted      State = "started"
	StateSuggested    State = "suggested"
	StateAcknowledged State = "acknowledged"
	StateResolved     State = "resolved"
)

// ObservabilityLinks represents observability deep links
type ObservabilityLinks struct {
	Grafana    string `json:"grafana"`
	Prometheus string `json:"prometheus"`
	Logs       string `json:"logs"`
	Traces     string `json:"traces"`
}

// ErrorStat represents an aggregated error statistic
type ErrorStat struct {
	Message string `json:"message"`
	Count   int    `json:"count"`
	Sample  string `json:"sample,omitempty"`
}

// LogSummary represents correlated log data for an incident
type LogSummary struct {
	Backend   string      `json:"backend"`
	Window    string      `json:"window"`
	TopErrors []ErrorStat `json:"top_errors"`
}

type LessonsLearned struct {
	WhatWentWell      string `json:"what_went_well,omitempty"`
	WhatCouldBeBetter string `json:"what_could_be_better,omitempty"`
	WhereWeGotLucky   string `json:"where_we_got_lucky,omitempty"`
}

type IncidentAnalysis struct {
	Component      string          `json:"component,omitempty"`
	Dependency     string          `json:"dependency,omitempty"`
	Pattern        string          `json:"pattern,omitempty"`
	Category       string          `json:"category,omitempty"`
	RootCause      string          `json:"root_cause,omitempty"`
	FixSummary     string          `json:"fix_summary,omitempty"`
	FailureType    string          `json:"failure_type,omitempty"`
	ErrorSignature string          `json:"error_signature,omitempty"`
	Prevention     string          `json:"prevention,omitempty"`
	LessonsLearned *LessonsLearned `json:"lessons_learned,omitempty"`
	MatchConfidence float64        `json:"match_confidence,omitempty"`
	MatchSource    string          `json:"match_source,omitempty"`
}

type ImpactMetrics struct {
	EstimatedDowntimeMinutes int      `json:"estimated_downtime_minutes,omitempty"`
	ImpactedFlows            []string `json:"impacted_flows,omitempty"`
	CustomMetrics            string   `json:"custom_metrics,omitempty"`
}

type ActionItem struct {
	ID          string    `json:"id"`
	Description string    `json:"description"`
	Owner       string    `json:"owner"`
	Priority    string    `json:"priority"`
	Status      string    `json:"status"`
	DueDate     time.Time `json:"due_date,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Incident represents an incident for notifications
type Incident struct {
	ID          string               `json:"id"`
	Service     string               `json:"service"`
	Severity    Severity             `json:"severity"`
	Title       string               `json:"title"`
	State       State                `json:"state"`
	CreatedAt   time.Time            `json:"created_at"`
	UpdatedAt   time.Time            `json:"updated_at"`
	Summary     string               `json:"summary,omitempty"`
	Events      []IncidentEventDetail `json:"events"`
	Links       ObservabilityLinks   `json:"links"`
	Logs        *LogSummary          `json:"logs,omitempty"`
	Traces      *traces.TraceSummary `json:"traces,omitempty"`
	Analysis    *IncidentAnalysis    `json:"analysis,omitempty"`
	Impact      *ImpactMetrics       `json:"impact,omitempty"`
	ActionItems []ActionItem         `json:"action_items,omitempty"`
	Cluster     string               `json:"cluster,omitempty"`
	Namespace   string               `json:"namespace,omitempty"`
	Deployment  string               `json:"deployment,omitempty"`
	K8sContext  string               `json:"k8s_context,omitempty"`
}

// IncidentEvent represents an event in the incident timeline
type IncidentEventDetail struct {
	Timestamp time.Time `json:"timestamp"`
	User      string    `json:"user"`
	Type      string    `json:"type"`
	Message   string    `json:"message,omitempty"`
}

// NotificationEvent represents types of incident events we notify on
type NotificationEvent struct {
	Type      string   // "started", "suggested", "acknowledged", "resolved"
	Incident  Incident
	Timestamp time.Time
}

// Notifier interface for sending notifications
type Notifier interface {
	Notify(ctx context.Context, event NotificationEvent) error
}

// SlackNotifier implements the Notifier interface for Slack
type SlackNotifier struct {
	webhookURL string
	config    *SlackConfig
}

// SlackConfig holds Slack notification configuration
type SlackConfig struct {
	Enabled       bool     `json:"enabled"`
	WebhookURL    string   `json:"slack_webhook_url"`
	NotifyOn      []string `json:"notify_on"`
	TimeoutSeconds int      `json:"timeout_seconds"`
	MaxRetries    int      `json:"max_retries"`
}

// PagerDutyConfig holds PagerDuty notification configuration
type PagerDutyConfig struct {
	Enabled        bool              `json:"enabled"`
	RoutingKey     string            `json:"routing_key"`
	SeverityMap    map[string]string `json:"severity_map"`
	TimeoutSeconds int               `json:"timeout_seconds"`
	MaxRetries     int               `json:"max_retries"`
}

// SlackMessage represents the Slack webhook payload
type SlackMessage struct {
	Text string `json:"text"`
}

// NewSlackNotifier creates a new Slack notifier
func NewSlackNotifier(cfg *SlackConfig) *SlackNotifier {
	return &SlackNotifier{
		webhookURL: cfg.WebhookURL,
		config:     cfg,
	}
}

// Notify sends a notification to Slack
func (s *SlackNotifier) Notify(ctx context.Context, event NotificationEvent) error {
	output.Debugf("Slack Notify called - Enabled: %v, EventType: %s, WebhookURL: %s", s.config.Enabled, event.Type, s.webhookURL)
	
	if !s.config.Enabled {
		output.Debugf("Slack notifications disabled, skipping")
		return nil
	}

	// Check if this event type should be notified
	if !s.shouldNotify(event.Type) {
		output.Debugf("Event type %s not in notify_on list %v", event.Type, s.config.NotifyOn)
		return nil
	}

	output.Debugf("Sending Slack notification for incident %s", event.Incident.ID)

	// Format the message
	output.Debugf("Formatting message...")
	message := s.formatMessage(event)
	output.Debugf("Message formatted, length: %d", len(message))

	// Send with timeout and retry
	output.Debugf("Calling sendWithRetry...")
	return s.sendWithRetry(ctx, message)
}

// canonicalEventType normalizes event type names so that configuration
// can use either short or full forms, e.g. "ack" or "acknowledged".
func canonicalEventType(eventType string) string {
	switch strings.ToLower(eventType) {
	case "start", "started":
		return "started"
	case "suggest", "suggested":
		return "suggested"
	case "ack", "acknowledged":
		return "acknowledged"
	case "resolve", "resolved":
		return "resolved"
	default:
		return strings.ToLower(eventType)
	}
}

// shouldNotify checks if the event type should trigger a notification
func (s *SlackNotifier) shouldNotify(eventType string) bool {
	eventKey := canonicalEventType(eventType)
	for _, notifyType := range s.config.NotifyOn {
		if canonicalEventType(notifyType) == eventKey {
			return true
		}
	}
	return false
}

// LoadSlackConfig loads Slack configuration from the main config
func LoadSlackConfig() (*SlackConfig, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Use nested Slack config (after migration in config.Load())
	slackConfig := &SlackConfig{
		Enabled:        cfg.Notifications.Slack.Enabled,
		WebhookURL:     cfg.Notifications.Slack.WebhookURL,
		NotifyOn:       cfg.Notifications.NotifyOn,
		TimeoutSeconds: cfg.Notifications.Slack.TimeoutSeconds,
		MaxRetries:     cfg.Notifications.Slack.MaxRetries,
	}

	// Apply defaults if not set
	if len(slackConfig.NotifyOn) == 0 {
		slackConfig.NotifyOn = []string{"started", "suggest", "ack", "resolve"}
	}
	if slackConfig.TimeoutSeconds <= 0 {
		slackConfig.TimeoutSeconds = 10
	}
	if slackConfig.MaxRetries <= 0 {
		slackConfig.MaxRetries = 3
	}

	return slackConfig, nil
}

// CreateNotifiers creates all enabled notifiers from config
func CreateNotifiers() ([]Notifier, error) {
	var notifiers []Notifier

	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}
	res := cfg.Notifications.Resilience

	wrap := func(n Notifier) Notifier {
		if res.Enabled {
			return NewResilientNotifier(n, res.Threshold, time.Duration(res.ResetTimeout)*time.Second, res.MaxRetries)
		}
		return n
	}

	// Load Slack notifier
	slackCfg, err := LoadSlackConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load Slack config: %w", err)
	}

	if slackCfg.Enabled && slackCfg.WebhookURL != "" {
		notifiers = append(notifiers, wrap(NewSlackNotifier(slackCfg)))
		output.Debugf("Slack notifier enabled (resilience: %v)", res.Enabled)
	}

	// Load PagerDuty notifier
	if cfg.Notifications.PagerDuty.Enabled && cfg.Notifications.PagerDuty.RoutingKey != "" {
		// Import pagerduty package dynamically
		pdCfg := &PagerDutyConfig{
			Enabled:        cfg.Notifications.PagerDuty.Enabled,
			RoutingKey:     cfg.Notifications.PagerDuty.RoutingKey,
			SeverityMap:    cfg.Notifications.PagerDuty.SeverityMap,
			TimeoutSeconds: cfg.Notifications.PagerDuty.TimeoutSeconds,
			MaxRetries:     cfg.Notifications.PagerDuty.MaxRetries,
		}
		
		// Apply defaults
		if pdCfg.TimeoutSeconds <= 0 {
			pdCfg.TimeoutSeconds = 10
		}
		if pdCfg.MaxRetries <= 0 {
			pdCfg.MaxRetries = 3
		}
		if pdCfg.SeverityMap == nil {
			pdCfg.SeverityMap = map[string]string{
				"P1": "critical",
				"P2": "error",
				"P3": "warning",
			}
		}
		
		notifiers = append(notifiers, wrap(NewPagerDutyNotifier(pdCfg)))
		output.Debugf("PagerDuty notifier enabled (resilience: %v)", res.Enabled)
	}

	// Load Jira notifier
	if cfg.Notifications.Jira.Enabled && cfg.Notifications.Jira.URL != "" {
		notifiers = append(notifiers, wrap(NewJiraNotifier(
			cfg.Notifications.Jira.URL,
			cfg.Notifications.Jira.User,
			cfg.Notifications.Jira.Token,
			cfg.Notifications.Jira.ProjectKey,
			cfg.Notifications.Jira.Enabled,
		)))
		output.Debugf("Jira notifier enabled (resilience: %v)", res.Enabled)
	}

	// If no notifiers enabled, return no-op
	if len(notifiers) == 0 {
		output.Debugf("No notifiers enabled, using NoOpNotifier")
		notifiers = append(notifiers, &NoOpNotifier{})
	}

	return notifiers, nil
}

// CreateNotifier creates a Slack notifier from config (DEPRECATED: use CreateNotifiers)
// Kept for backward compatibility
func CreateNotifier() (Notifier, error) {
	notifiers, err := CreateNotifiers()
	if err != nil {
		return nil, err
	}
	if len(notifiers) > 0 {
		return notifiers[0], nil
	}
	return &NoOpNotifier{}, nil
}

// NoOpNotifier is a no-op implementation for when notifications are disabled
type NoOpNotifier struct{}

func (n *NoOpNotifier) Notify(ctx context.Context, event NotificationEvent) error {
	output.Debugf("NoOpNotifier called - notifications disabled or not configured")
	return nil
}
