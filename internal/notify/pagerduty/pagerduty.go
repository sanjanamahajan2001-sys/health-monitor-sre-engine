package pagerduty

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"health-monitor/internal/config"
	"health-monitor/internal/notify/slack"
)

const (
	pagerDutyEventsURL = "https://events.pagerduty.com/v2/enqueue"
)

// PagerDutyNotifier implements the Notifier interface for PagerDuty
type PagerDutyNotifier struct {
	routingKey   string
	severityMap  map[string]string
	config       *PagerDutyConfig
	httpClient   *http.Client
}

// PagerDutyConfig holds PagerDuty notification configuration
type PagerDutyConfig struct {
	Enabled        bool              `json:"enabled"`
	RoutingKey     string            `json:"routing_key"`
	SeverityMap    map[string]string `json:"severity_map"`
	TimeoutSeconds int               `json:"timeout_seconds"`
	MaxRetries     int               `json:"max_retries"`
}

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
func (p *PagerDutyNotifier) Notify(ctx context.Context, event slack.NotificationEvent) error {
	log.Printf("DEBUG: PagerDuty Notify called - Enabled: %v, EventType: %s", p.config.Enabled, event.Type)
	
	if !p.config.Enabled {
		log.Printf("DEBUG: PagerDuty notifications disabled, skipping")
		return nil
	}

	if p.routingKey == "" {
		log.Printf("WARN: PagerDuty routing key not configured, skipping notification")
		return nil
	}

	// Map event type to PagerDuty action
	action := p.mapEventAction(event.Type)
	if action == "" {
		log.Printf("DEBUG: Event type %s not mapped to PagerDuty action, skipping", event.Type)
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
func (p *PagerDutyNotifier) buildEvent(event slack.NotificationEvent, action string) *PagerDutyEvent {
	pdEvent := &PagerDutyEvent{
		RoutingKey:  p.routingKey,
		EventAction: action,
		DedupKey:    p.buildDedupKey(event), // Profile-aware dedup key
	}

	// Only include payload for trigger events
	if action == "trigger" {
		severity := p.mapSeverity(string(event.Incident.Severity))
		
		customDetails := map[string]interface{}{
			"incident_id": event.Incident.ID,
			"service":     event.Incident.Service,
		}
		
		// Add profile context to custom details
		pm := config.GetProfileManager()
		activeProfile := pm.GetActiveProfile()
		if activeProfile != "default" {
			customDetails["profile"] = activeProfile
			customDetails["environment"] = getProfileEnvironment(activeProfile)
			customDetails["cluster"] = getProfileCluster(activeProfile)
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
			customDetails["trace_count"] = event.Incident.Traces.TraceCount
			customDetails["p95_latency_ms"] = event.Incident.Traces.P95LatencyMs
			customDetails["error_rate"] = fmt.Sprintf("%.1f%%", event.Incident.Traces.ErrorRate)
			if event.Incident.Traces.SlowestRoute != "" {
				customDetails["slowest_endpoint"] = fmt.Sprintf("%s (%.2fs)", 
					event.Incident.Traces.SlowestRoute, 
					event.Incident.Traces.P95LatencyMs/1000)
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

		// Add RCA Analysis if available 
		if event.Incident.Analysis != nil {
			a := event.Incident.Analysis
			customDetails["rca_category"] = a.Category
			customDetails["rca_component"] = a.Component
			customDetails["rca_root_cause"] = a.RootCause
			customDetails["rca_fix_summary"] = a.FixSummary
			customDetails["rca_prevention"] = a.Prevention
			if a.LessonsLearned != nil {
				customDetails["lessons_went_well"] = a.LessonsLearned.WhatWentWell
				customDetails["lessons_improvement"] = a.LessonsLearned.WhatCouldBeBetter
			}
		}

		// Add Impact metrics if available
		if event.Incident.Impact != nil {
			customDetails["impact_downtime_min"] = event.Incident.Impact.EstimatedDowntimeMinutes
			customDetails["impact_flows"] = event.Incident.Impact.ImpactedFlows
			customDetails["impact_metrics"] = event.Incident.Impact.CustomMetrics
		}

		// Add Action Items summary
		if len(event.Incident.ActionItems) > 0 {
			items := make([]string, 0, len(event.Incident.ActionItems))
			for _, it := range event.Incident.ActionItems {
				items = append(items, fmt.Sprintf("[%s] %s (%s) - %s", it.Priority, it.Status, it.Owner, it.Description))
			}
			customDetails["action_items"] = items
		}
		
		pdEvent.Payload = &PagerDutyPayload{
			Summary:       fmt.Sprintf("%s %s - %s", event.Incident.Severity, event.Incident.Service, event.Incident.Title),
			Severity:      severity,
			Source:        "health-monitor",
			CustomDetails: customDetails,
		}
	}

	return pdEvent
}

// buildDedupKey creates a profile-aware deduplication key
func (p *PagerDutyNotifier) buildDedupKey(event slack.NotificationEvent) string {
	pm := config.GetProfileManager()
	activeProfile := pm.GetActiveProfile()
	
	if activeProfile == "default" {
		return event.Incident.ID // Backward compatibility
	}
	
	return fmt.Sprintf("%s:%s", activeProfile, event.Incident.ID)
}

// Helper functions to get profile metadata
func getProfileEnvironment(profileName string) string {
	pm := config.GetProfileManager()
	if profile, err := pm.GetProfile(profileName); err == nil {
		return profile.Environment
	}
	return ""
}

func getProfileCluster(profileName string) string {
	pm := config.GetProfileManager()
	if profile, err := pm.GetProfile(profileName); err == nil {
		return profile.Cluster
	}
	return ""
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
			
			log.Printf("DEBUG: PagerDuty retry attempt %d after %v", attempt+1, backoff)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return fmt.Errorf("context cancelled during retry backoff: %w", ctx.Err())
			}
		}

		err := p.sendEvent(ctx, event)
		if err == nil {
			if attempt > 0 {
				log.Printf("INFO: PagerDuty notification succeeded on retry %d", attempt+1)
			}
			return nil
		}

		lastErr = err
		log.Printf("WARN: PagerDuty notification attempt %d failed: %v", attempt+1, err)
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

	log.Printf("DEBUG: Sending PagerDuty %s event for incident %s", event.EventAction, event.DedupKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("PagerDuty API returned status %d", resp.StatusCode)
	}

	log.Printf("DEBUG: Successfully sent PagerDuty %s event for incident %s", event.EventAction, event.DedupKey)
	return nil
}

// LoadPagerDutyConfig loads PagerDuty configuration from the main config
func LoadPagerDutyConfig() (*PagerDutyConfig, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Use nested PagerDuty config
	pdConfig := &PagerDutyConfig{
		Enabled:        cfg.Notifications.PagerDuty.Enabled,
		RoutingKey:     cfg.Notifications.PagerDuty.RoutingKey,
		SeverityMap:    cfg.Notifications.PagerDuty.SeverityMap,
		TimeoutSeconds: cfg.Notifications.PagerDuty.TimeoutSeconds,
		MaxRetries:     cfg.Notifications.PagerDuty.MaxRetries,
	}

	// Apply defaults if not set
	if pdConfig.TimeoutSeconds <= 0 {
		pdConfig.TimeoutSeconds = 10
	}
	if pdConfig.MaxRetries <= 0 {
		pdConfig.MaxRetries = 3
	}
	if pdConfig.SeverityMap == nil {
		pdConfig.SeverityMap = map[string]string{
			"P1": "critical",
			"P2": "error",
			"P3": "warning",
		}
	}

	return pdConfig, nil
}
