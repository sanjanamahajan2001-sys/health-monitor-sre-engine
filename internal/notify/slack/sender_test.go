package slack

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSendWithRetry(t *testing.T) {
	// Test successful send on first attempt
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier := &SlackNotifier{
		webhookURL: server.URL,
		config: &SlackConfig{
			TimeoutSeconds: 1,
			MaxRetries:    2,
		},
	}

	ctx := context.Background()
	err := notifier.sendWithRetry(ctx, "test message")
	if err != nil {
		t.Errorf("sendWithRetry() = %v, want nil", err)
	}
}

func TestSendWithRetryFailure(t *testing.T) {
	// Test failure after retries
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	notifier := &SlackNotifier{
		webhookURL: server.URL,
		config: &SlackConfig{
			TimeoutSeconds: 1,
			MaxRetries:    1,
		},
	}

	ctx := context.Background()
	err := notifier.sendWithRetry(ctx, "test message")
	if err == nil {
		t.Error("sendWithRetry() = nil, want error")
	}
	if !strings.Contains(err.Error(), "failed to send Slack notification") {
		t.Errorf("sendWithRetry() error = %v, want contains 'failed to send Slack notification'", err)
	}
}

func TestSendWithRetryTimeout(t *testing.T) {
	// Test timeout
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second) // Longer than timeout
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier := &SlackNotifier{
		webhookURL: server.URL,
		config: &SlackConfig{
			TimeoutSeconds: 1,
			MaxRetries:    1,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err := notifier.sendWithRetry(ctx, "test message")
	if err == nil {
		t.Error("sendWithRetry() = nil, want timeout error")
	}
}

func TestShouldNotify(t *testing.T) {
	notifier := &SlackNotifier{
		config: &SlackConfig{
			NotifyOn: []string{"start", "resolve"},
		},
	}

	tests := []struct {
		eventType string
		expected bool
	}{
		{"start", true},
		{"resolve", true},
		{"suggest", false},
		{"ack", false},
		{"unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.eventType, func(t *testing.T) {
			result := notifier.shouldNotify(tt.eventType)
			if result != tt.expected {
				t.Errorf("shouldNotify() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestTestConnection(t *testing.T) {
	// Test successful connection
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier := &SlackNotifier{
		webhookURL: server.URL,
		config:     &SlackConfig{},
	}

	ctx := context.Background()
	err := notifier.TestConnection(ctx)
	if err != nil {
		t.Errorf("TestConnection() = %v, want nil", err)
	}

	// Test failed connection
	notifier.webhookURL = "invalid-url"
	err = notifier.TestConnection(ctx)
	if err == nil {
		t.Error("TestConnection() = nil, want error for invalid URL")
	}
}

func TestNoOpNotifier(t *testing.T) {
	notifier := &NoOpNotifier{}
	ctx := context.Background()
	event := NotificationEvent{
		Type: "started",
		Incident: Incident{
			ID: "test",
		},
		Timestamp: time.Now(),
	}

	err := notifier.Notify(ctx, event)
	if err != nil {
		t.Errorf("NoOpNotifier.Notify() = %v, want nil", err)
	}
}
