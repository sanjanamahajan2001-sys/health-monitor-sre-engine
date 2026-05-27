package feedback

import (
	"os"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
	"health-monitor/internal/config"
	"health-monitor/pkg/model"
)

// SendToCentralizedEndpoint sends feedback to the configured AWS API with retries
func SendToCentralizedEndpoint(fb *model.Feedback) error {
	endpoint := "https://l3xrj8ek2c.execute-api.us-east-1.amazonaws.com/feedback"
	
	// API Key support for production
	token := os.Getenv("HEALTH_MONITOR_FEEDBACK_TOKEN")
	
	config.DebugLog("Attempting to send feedback %s to %s", fb.ID, endpoint)
	
	data, err := json.Marshal(fb)
	if err != nil {
		return fmt.Errorf("failed to marshal feedback for sending: %w", err)
	}

	var lastErr error
	maxRetries := 3
	
	for attempt := 1; attempt <= maxRetries; attempt++ {
		client := &http.Client{
			Timeout: 10 * time.Second,
		}
		
		req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(data))
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}
		
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "health-monitor-agent/"+fb.Version)
		if token != "" {
			req.Header.Set("X-Agent-Key", token)
		}
		
		resp, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("attempt %d failed: %w", attempt, err)
			config.DebugLog("Feedback send attempt %d failed: %v", attempt, err)
			time.Sleep(time.Duration(attempt) * time.Second) // Simple backoff
			continue
		}
		defer resp.Body.Close()
		
		if resp.StatusCode >= 400 {
			lastErr = fmt.Errorf("endpoint returned status %d", resp.StatusCode)
			config.DebugLog("Feedback endpoint returned status %d on attempt %d", resp.StatusCode, attempt)
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}
		
		config.DebugLog("Feedback %s successfully sent on attempt %d", fb.ID, attempt)
		return nil
	}
	
	return fmt.Errorf("failed to send feedback after %d attempts: %w", maxRetries, lastErr)
}
