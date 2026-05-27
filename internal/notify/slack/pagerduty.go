package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"health-monitor/internal/output"
	"net/http"
	"time"
)

// PagerDutyNotifier implements the Notifier interface for PagerDuty
type PagerDutyNotifier struct {
	routingKey   string
	severityMap  map[string]string
	config       *PagerDutyConfig
	httpClient   *http.Client
}

const pagerDutyEventsURL = "https://events.pagerduty.com/v2/enqueue"

// PagerDutyEvent represents the PagerDuty Events API v2 payload
type PagerDutyEvent struct {
	RoutingKey  string              `json:"routing_key"`
	EventAction string              `json:"event_action"` // trigger, acknowledge, resolve
	DedupKey    string              `json:"dedup_key"`
	Payload     *PagerDutyPayload   `json:"payload,omitempty"`
}

// PagerDutyPayload represents the event payload details
type PagerDutyPayload struct {
	Summary       string                 `json:"summary"`
	Severity      string                 `json:"severity"` // critical, error, warning, info
	Source        string                 `json:"source"`
	CustomDetails map[string]interface{} `json:"custom_details,omitempty"`
}

// NewPagerDutyNotifier creates a new PagerDuty notifier
func NewPagerDutyNotifier(cfg *PagerDutyConfig) *PagerDutyNotifier {
	return &PagerDutyNotifier{
		routingKey:  cfg.RoutingKey,
		severityMap: cfg.SeverityMap,
		config:      cfg,
		httpClient: &http.Client{
			Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second,
		},
	}
}

// Notify sends a notification to PagerDuty
func (p *PagerDutyNotifier) Notify(ctx context.Context, event NotificationEvent) error {
	output.Debugf("PagerDuty Notify called - Enabled: %v, EventType: %s", p.config.Enabled, event.Type)
	
	if !p.config.Enabled {
		output.Debugf("PagerDuty notifications disabled, skipping")
		return nil
	}

	if p.routingKey == "" {
		output.Warnf("PagerDuty routing key not configured, skipping notification")
		return nil
	}

	// Map event type to PagerDuty action
	action := p.mapEventAction(event.Type)
	if action == "" {
		output.Debugf("Event type %s not mapped to PagerDuty action, skipping", event.Type)
		return nil
	}

	// Build PagerDuty event
	pdEvent := p.buildEvent(event, action)
	
	// Send with retry
	return p.sendWithRetry(ctx, pdEvent)
}

// mapEventAction maps incident event types to PagerDuty actions
func (p *PagerDutyNotifier) mapEventAction(eventType string) string {
	switch eventType {
	case "started":
		return "trigger"
	case "acknowledged":
		return "acknowledge"
	case "resolved":
		return "resolve"
	default:
		return "" // Don't send for other event types (suggested, etc.)
	}
}

// buildEvent constructs a PagerDuty event from an incident event
func (p *PagerDutyNotifier) buildEvent(event NotificationEvent, action string) *PagerDutyEvent {
	pdEvent := &PagerDutyEvent{
		RoutingKey:  p.routingKey,
		EventAction: action,
		DedupKey:    event.Incident.ID, // Critical: use incident ID for deduplication
	}

	// Build custom details for all event types
	customDetails := map[string]interface{}{
		"incident_id": event.Incident.ID,
		"service":     event.Incident.Service,
		"priority":    string(event.Incident.Severity),
	}
	
	// Add observability links if available
	if event.Incident.Links.Grafana != "" {
		customDetails["grafana"] = event.Incident.Links.Grafana
	}
	if event.Incident.Links.Prometheus != "" {
		customDetails["prometheus"] = event.Incident.Links.Prometheus
	}
	if event.Incident.Links.Logs != "" {
		customDetails["logs"] = event.Incident.Links.Logs
	}
	if event.Incident.Links.Traces != "" {
		customDetails["traces"] = event.Incident.Links.Traces
	}
	
	// Add trace summary if available
	if event.Incident.Traces != nil {
		if event.Incident.Traces.ErrorCount > 0 {
			customDetails["error_count"] = event.Incident.Traces.ErrorCount
		}
		if event.Incident.Traces.P95LatencyMs > 0 {
			customDetails["p95_latency_ms"] = event.Incident.Traces.P95LatencyMs
		}
		if event.Incident.Traces.SlowestRoute != "" {
			customDetails["slowest_endpoint"] = fmt.Sprintf("%s (P95: %.2fms)", 
				event.Incident.Traces.SlowestRoute, 
				event.Incident.Traces.P95LatencyMs)
		}
	}
	
	// Add log summary if available
	if event.Incident.Logs != nil && len(event.Incident.Logs.TopErrors) > 0 {
		errorMessages := make([]string, 0, len(event.Incident.Logs.TopErrors))
		for _, e := range event.Incident.Logs.TopErrors {
			errorMessages = append(errorMessages, fmt.Sprintf("%s (%dx)", e.Message, e.Count))
		}
		customDetails["top_errors"] = errorMessages
	}
	
	// Add event-specific details
	switch action {
	case "trigger":
		// Add recent notes for context
		notes := getRecentNotes(event.Incident.Events, 3)
		if len(notes) > 0 {
			noteStrings := make([]string, len(notes))
			for i, note := range notes {
				noteStrings[i] = fmt.Sprintf("[%s] %s: %s", 
					note.Timestamp.UTC().Format("15:04"), note.User, truncateString(note.Message, 100))
			}
			customDetails["notes"] = noteStrings
		}
		
	case "acknowledge":
		// Add who acknowledged
		ackedBy := getLastUserByEventType(event.Incident.Events, "ack")
		if ackedBy != "" {
			customDetails["acknowledged_by"] = ackedBy
		}
		customDetails["acknowledged_at"] = event.Timestamp.UTC().Format(time.RFC3339)
		
		// Add recent notes
		notes := getRecentNotes(event.Incident.Events, 3)
		if len(notes) > 0 {
			noteStrings := make([]string, len(notes))
			for i, note := range notes {
				noteStrings[i] = fmt.Sprintf("[%s] %s: %s", 
					note.Timestamp.UTC().Format("15:04"), note.User, truncateString(note.Message, 100))
			}
			customDetails["notes"] = noteStrings
		}
		
	case "resolve":
		// Add who resolved
		resolvedBy := getLastUserByEventType(event.Incident.Events, "resolve")
		if resolvedBy != "" {
			customDetails["resolved_by"] = resolvedBy
		}
		customDetails["resolved_at"] = event.Timestamp.UTC().Format(time.RFC3339)
		
		// Add resolution summary
		if event.Incident.Summary != "" {
			customDetails["resolution_summary"] = event.Incident.Summary
		}
		
		// Add incident duration
		if !event.Incident.CreatedAt.IsZero() {
			duration := event.Timestamp.Sub(event.Incident.CreatedAt)
			customDetails["duration"] = formatDuration(duration)
			customDetails["duration_seconds"] = int(duration.Seconds())
		}
		
		// Add more notes for resolved incidents
		notes := getRecentNotes(event.Incident.Events, 5)
		if len(notes) > 0 {
			noteStrings := make([]string, len(notes))
			for i, note := range notes {
				noteStrings[i] = fmt.Sprintf("[%s] %s: %s", 
					note.Timestamp.UTC().Format("15:04"), note.User, truncateString(note.Message, 100))
			}
			customDetails["notes"] = noteStrings
		}
	}

	// Only include payload for trigger events (PagerDuty API requirement)
	if action == "trigger" {
		severity := p.mapSeverity(string(event.Incident.Severity))
		
		pdEvent.Payload = &PagerDutyPayload{
			Summary:       fmt.Sprintf("[%s] %s - %s", event.Incident.Severity, event.Incident.Service, event.Incident.Title),
			Severity:      severity,
			Source:        "health-monitor",
			CustomDetails: customDetails,
		}
	} else {
		// For acknowledge/resolve, custom_details go in the root event
		// But PagerDuty Events API v2 doesn't support custom_details in ack/resolve
		// So we'll log them for debugging
		output.Debugf("PagerDuty %s event custom details: %+v", action, customDetails)
	}

	return pdEvent
}

// mapSeverity maps incident severity to PagerDuty severity
func (p *PagerDutyNotifier) mapSeverity(incidentSeverity string) string {
	if mapped, ok := p.severityMap[incidentSeverity]; ok {
		return mapped
	}
	// Default mapping
	switch incidentSeverity {
	case "P1":
		return "critical"
	case "P2":
		return "error"
	case "P3":
		return "warning"
	default:
		return "error"
	}
}

// sendWithRetry sends the event with exponential backoff retry
func (p *PagerDutyNotifier) sendWithRetry(ctx context.Context, event *PagerDutyEvent) error {
	maxRetries := p.config.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}

	backoffDurations := []time.Duration{
		1 * time.Second,
		3 * time.Second,
		5 * time.Second,
	}

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Apply backoff
			backoff := backoffDurations[attempt-1]
			if attempt-1 >= len(backoffDurations) {
				backoff = backoffDurations[len(backoffDurations)-1]
			}
			
			output.Debugf("PagerDuty retry attempt %d after %v", attempt+1, backoff)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return fmt.Errorf("context cancelled during retry backoff: %w", ctx.Err())
			}
		}

		err := p.sendEvent(ctx, event)
		if err == nil {
			if attempt > 0 {
				output.Infof("PagerDuty notification succeeded on retry %d", attempt+1)
			}
			return nil
		}

		lastErr = err
		output.Warnf("PagerDuty notification attempt %d failed: %v", attempt+1, err)
	}

	return fmt.Errorf("failed after %d attempts: %w", maxRetries, lastErr)
}

// sendEvent sends a single event to PagerDuty
func (p *PagerDutyNotifier) sendEvent(ctx context.Context, event *PagerDutyEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal PagerDuty event: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", pagerDutyEventsURL, bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	output.Debugf("Sending PagerDuty %s event for incident %s", event.EventAction, event.DedupKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("PagerDuty API returned status %d", resp.StatusCode)
	}

	output.Debugf("Successfully sent PagerDuty %s event for incident %s", event.EventAction, event.DedupKey)
	return nil
}
