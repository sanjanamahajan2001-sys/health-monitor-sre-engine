package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"health-monitor/internal/output"
	"net/http"
	"strings"
	"time"
)

// sendWithRetry sends message with retry logic and exponential backoff
func (s *SlackNotifier) sendWithRetry(ctx context.Context, message string) error {
	output.Debugf("sendWithRetry called with message length: %d", len(message))
	
	output.Debugf("Setting up timeout and retries...")
	// Use hardcoded values to avoid config access issues
	timeout := 10 * time.Second
	maxRetries := 3
	
	output.Debugf("Using timeout: %v, maxRetries: %d", timeout, maxRetries)
	output.Debugf("Starting retry loop...")

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		output.Debugf("Attempt %d/%d", attempt+1, maxRetries+1)
		
		select {
		case <-ctx.Done():
			output.Debugf("Context cancelled in retry loop")
			return ctx.Err()
		default:
		}

		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s...
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
			output.Debugf("Retrying after %v backoff (attempt %d)", backoff, attempt+1)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				output.Debugf("Context cancelled during backoff")
				return ctx.Err()
			}
		}

		output.Debugf("About to call sendMessage...")
		err := s.sendMessage(ctx, message, timeout)
		output.Debugf("sendMessage returned with error: %v", err)
		
		if err == nil {
			output.Debugf("Message sent successfully on attempt %d", attempt+1)
			return nil
		}

		lastErr = err
		if attempt == 0 {
			// Log failure once (no spam)
			output.Warnf("Slack notification failed (attempt %d/%d): %v", attempt+1, maxRetries+1, lastErr)
		}
	}

	output.Errorf("Failed to send Slack notification after %d attempts. Last error: %v", maxRetries+1, lastErr)
	return fmt.Errorf("failed to send Slack notification after %d attempts: %w", maxRetries+1, lastErr)
}

// sendMessage sends a single message to Slack webhook
func (s *SlackNotifier) sendMessage(ctx context.Context, message string, timeout time.Duration) error {
	output.Debugf("sendMessage called - URL: %s, timeout: %v", s.webhookURL, timeout)
	
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	output.Debugf("Creating SlackMessage payload...")
	payload := SlackMessage{Text: message}
	output.Debugf("Marshaling payload to JSON...")
	data, err := json.Marshal(payload)
	if err != nil {
		output.Debugf("Failed to marshal Slack message: %v", err)
		return fmt.Errorf("failed to marshal Slack message: %w", err)
	}

	output.Debugf("Payload marshaled successfully, length: %d", len(data))
	output.Debugf("Sending payload: %s", string(data))

	output.Debugf("Creating HTTP request...")
	req, err := http.NewRequestWithContext(ctx, "POST", s.webhookURL, strings.NewReader(string(data)))
	if err != nil {
		output.Debugf("Failed to create Slack request: %v", err)
		return fmt.Errorf("failed to create Slack request: %w", err)
	}

	output.Debugf("Setting Content-Type header...")
	req.Header.Set("Content-Type", "application/json")
	output.Debugf("HTTP request created successfully, sending to Slack...")

	output.Debugf("About to call http.DefaultClient.Do(req)...")
	resp, err := http.DefaultClient.Do(req)
	output.Debugf("http.DefaultClient.Do(req) completed")
	
	if err != nil {
		output.Debugf("Failed to send Slack request: %v", err)
		if ctx.Err() != nil {
			output.Debugf("Context error: %v", ctx.Err())
		}
		return fmt.Errorf("failed to send Slack request: %w", err)
	}
	defer resp.Body.Close()

	output.Debugf("Response status: %d", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		output.Debugf("Slack webhook returned status %d", resp.StatusCode)
		return fmt.Errorf("Slack webhook returned status %d", resp.StatusCode)
	}

	output.Debugf("Slack notification sent successfully")
	return nil
}

// TestConnection tests the Slack webhook connection
func (s *SlackNotifier) TestConnection(ctx context.Context) error {
	if s.webhookURL == "" {
		return fmt.Errorf("slack webhook URL not configured")
	}

	testMessage := "🧪 Health Monitor Slack Test\n\nThis is a test message to verify Slack notifications are working.\n\nIf you see this, notifications are configured correctly!"

	return s.sendWithRetry(ctx, testMessage)
}
