package slack

import (
	"testing"
	"time"
)

func TestFormatMessage(t *testing.T) {
	notifier := &SlackNotifier{
		config: &SlackConfig{},
	}

	tests := []struct {
		name     string
		event    NotificationEvent
		expected string
	}{
		{
			name: "P1 incident started",
			event: NotificationEvent{
				Type: "started",
				Incident: Incident{
					ID:       "INC-20260209-123456",
					Service:  "billing_api",
					Severity: SeverityP1,
					Title:    "Checkout latency spike",
					State:    StateStarted,
					Links: ObservabilityLinks{
						Grafana:    "https://grafana.com/explore",
						Prometheus: "https://prometheus.com/graph",
						Logs:       "https://logs.com/view",
						Traces:     "https://traces.com/search",
					},
				},
				Timestamp: time.Date(2026, 2, 9, 10, 21, 0, 0, time.UTC),
			},
			expected: "🚨 Incident Started | P1 | billing_api\nTitle: Checkout latency spike\nService: billing_api\nSeverity: P1\nState: Started\nIncident ID: INC-20260209-123456\n\nGrafana: https://grafana.com/explore\nPrometheus: https://prometheus.com/graph\nLogs: https://logs.com/view\nTraces: https://traces.com/search\n\nTriggered at: 2026-02-09 10:21 UTC",
		},
		{
			name: "P2 incident suggested",
			event: NotificationEvent{
				Type: "suggested",
				Incident: Incident{
					ID:       "INC-20260209-123456",
					Service:  "user_api",
					Severity: SeverityP2,
					Title:    "Login failures increasing",
					State:    StateSuggested,
					Links: ObservabilityLinks{
						Grafana: "https://grafana.com/explore",
					},
				},
				Timestamp: time.Date(2026, 2, 9, 10, 21, 0, 0, time.UTC),
			},
			expected: "⚠️ Incident Suggested | P2 | user_api\nTitle: Login failures increasing\nService: user_api\nSeverity: P2\nState: Suggested\nIncident ID: INC-20260209-123456\n\nGrafana: https://grafana.com/explore\n\nTriggered at: 2026-02-09 10:21 UTC",
		},
		{
			name: "P3 incident resolved",
			event: NotificationEvent{
				Type: "resolved",
				Incident: Incident{
					ID:       "INC-20260209-123456",
					Service:  "payment_api",
					Severity: SeverityP3,
					Title:    "Payment processing delays",
					State:    StateResolved,
					Links:    ObservabilityLinks{},
				},
				Timestamp: time.Date(2026, 2, 9, 10, 21, 0, 0, time.UTC),
			},
			expected: "ℹ️ Incident Resolved | P3 | payment_api\nTitle: Payment processing delays\nService: payment_api\nSeverity: P3\nState: Resolved\nIncident ID: INC-20260209-123456\n\nTriggered at: 2026-02-09 10:21 UTC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := notifier.formatMessage(tt.event)
			if result != tt.expected {
				t.Errorf("formatMessage() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestGetSeverityEmoji(t *testing.T) {
	tests := []struct {
		severity Severity
		expected string
	}{
		{SeverityP1, "🚨"},
		{SeverityP2, "⚠️"},
		{SeverityP3, "ℹ️"},
		{Severity("unknown"), "ℹ️"},
	}

	for _, tt := range tests {
		t.Run(string(tt.severity), func(t *testing.T) {
			result := getSeverityEmoji(tt.severity)
			if result != tt.expected {
				t.Errorf("getSeverityEmoji() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestFormatStateForTitle(t *testing.T) {
	tests := []struct {
		eventType string
		expected string
	}{
		{"started", "Started"},
		{"suggested", "Suggested"},
		{"acknowledged", "Acknowledged"},
		{"resolved", "Resolved"},
		{"unknown", "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.eventType, func(t *testing.T) {
			result := formatStateForTitle(tt.eventType)
			if result != tt.expected {
				t.Errorf("formatStateForTitle() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestHasObservabilityLinks(t *testing.T) {
	tests := []struct {
		name     string
		links    ObservabilityLinks
		expected bool
	}{
		{
			name:     "all links",
			links:    ObservabilityLinks{Grafana: "url", Prometheus: "url", Logs: "url", Traces: "url"},
			expected: true,
		},
		{
			name:     "single link",
			links:    ObservabilityLinks{Grafana: "url"},
			expected: true,
		},
		{
			name:     "no links",
			links:    ObservabilityLinks{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasObservabilityLinks(tt.links)
			if result != tt.expected {
				t.Errorf("hasObservabilityLinks() = %v, want %v", result, tt.expected)
			}
		})
	}
}
