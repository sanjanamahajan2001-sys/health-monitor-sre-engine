package incident

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"sort"
	"health-monitor/internal/audit"
	"health-monitor/internal/config"
	"health-monitor/internal/discovery"
	"health-monitor/internal/flow"
	"health-monitor/internal/logs"
	"health-monitor/internal/metrics"
	"health-monitor/internal/ml"
	"health-monitor/internal/notify/slack"
	"health-monitor/internal/output"
)

// Broadcaster is an interface for sending live updates to collaborative sessions
type Broadcaster interface {
	Broadcast(msg string)
	GetConnectionInfo() string
	OnUpdate(func(msg string))
}

var idPattern = regexp.MustCompile(`^INC-\d{8}-\d{6}$`)

func (s *Service) persistLifecycle(inc Incident, now time.Time) {
	if s.mlRecorder == nil {
		return
	}
	go func(inc Incident) {
		metadata := map[string]string{
			"summary": inc.Summary,
			"service": inc.Service,
			"profile": inc.Profile,
		}

		if inc.Analysis != nil {
			metadata["fix"] = inc.Analysis.FixSummary
			metadata["root_cause"] = inc.Analysis.RootCause
			metadata["prevention"] = inc.Analysis.Prevention
			metadata["category"] = inc.Analysis.Category
			metadata["component"] = inc.Analysis.Component
			metadata["failure_type"] = inc.Analysis.FailureType

			if inc.Analysis.LessonsLearned != nil {
				if llData, err := json.Marshal(inc.Analysis.LessonsLearned); err == nil {
					metadata["lessons_learned"] = string(llData)
				}
			}
		}

		if len(inc.ActionItems) > 0 {
			var actionTexts []string
			for _, item := range inc.ActionItems {
				actionTexts = append(actionTexts, item.Description)
			}
			if aiData, err := json.Marshal(actionTexts); err == nil {
				metadata["action_items"] = string(aiData)
			}
		}

		// Concatenate notes for semantic context
		var notes []string
		for _, evt := range inc.Events {
			if evt.Type == EventNote && evt.Message != "" {
				notes = append(notes, evt.Message)
			}
		}
		if len(notes) > 0 {
			metadata["notes"] = strings.Join(notes, " | ")
		}
		
		// Filter snapshots from the rolling buffer to isolated only those relevant to this incident/service.
		rawSnapshots := s.mlRecorder.GetPreIncidentBuffer()
		var filteredSnapshots []ml.LifecycleSnapshot
		
		// 🚀 SEED MERGE: Explicitly load reconstructed snapshots from the seed file
		// This ensures they are included even if the daemon overwrote the shared buffer.
		if seeds, err := s.mlRecorder.LoadSeed(inc.ID); err == nil && len(seeds) > 0 {
			filteredSnapshots = append(filteredSnapshots, seeds...)
			output.Debugf("[Lifecycle] Incident %s merged %d seed snapshots from disk", inc.ID, len(seeds))
		}

		// Pruning logic: only include contemporary snapshots (last 2 hours) 
		// and those explicitly belonging to this service OR incident.
		cutoff := time.Now().Add(-2 * time.Hour)
		
		output.Debugf("[Lifecycle] Filtering %d raw snapshots for incident %s", len(rawSnapshots), inc.ID)
		
		cleanIncID := strings.TrimSpace(inc.ID)
		
		for _, snap := range rawSnapshots {
			if snap.Timestamp.Before(cutoff) {
				continue
			}
			
			snapID := strings.TrimSpace(snap.IncidentID)
			snapActiveID := strings.TrimSpace(snap.Metadata["active_incident_id"])

			// Match criteria:
			// 1. If tagged with a different incident, DISCARD (Strict Ownership)
			// 2. If tagged with this incident, keep.
			// 3. If untagged, keep if the service was being monitored.

			hasOtherIncidentID := (snapID != "" && snapID != cleanIncID) || 
			                     (snapActiveID != "" && snapActiveID != cleanIncID)
			
			if hasOtherIncidentID {
				continue // Belongs to a different incident, skip even if service matches
			}

			isSameIncident := snapID == cleanIncID || snapActiveID == cleanIncID
			isSameService := snap.Metadata["primary_service"] == inc.Service || 
			                snap.Metadata["service"] == inc.Service ||
							snap.Metadata["monitored_services"] == inc.Service
			
			monitoredStr := snap.Metadata["monitored_services"]
			isMonitored := false
			if monitoredStr != "" {
				for _, svc := range strings.Split(monitoredStr, ",") {
					if strings.TrimSpace(svc) == inc.Service {
						isMonitored = true
						break
					}
				}
			}
			
			if isSameIncident || isSameService || isMonitored {
				// Avoid duplicates if already merged from seed
				exists := false
				for _, fs := range filteredSnapshots {
					if fs.Timestamp.Equal(snap.Timestamp) {
						exists = true
						break
					}
				}
				if !exists {
					filteredSnapshots = append(filteredSnapshots, snap)
				}
			}
		}
		output.Debugf("[Lifecycle] Incident %s accepted %d snapshots after filtering", inc.ID, len(filteredSnapshots))

		lifecycle := ml.FullLifecycle{
			IncidentID: inc.ID,
			Snapshots:  filteredSnapshots,
			ResolvedAt: now,
		}
		
		// 🚀 NEW: Add 'impact' snapshot with the FINAL logs and traces captured at resolution.
		// This captures the peak of the incident, especially important when the daemon is offline.
		if inc.State == StateResolved {
			impactSnap := ml.LifecycleSnapshot{
				Timestamp:  now,
				IncidentID: inc.ID,
				Phase:      "impact", // Distinct from pre/post to signify the failure peak
				Metadata:   metadata,
			}
			
			// Copy telemetry from the incident object (populated by PerformFinalSync)
			if inc.Logs != nil {
				impactSnap.FullLogs = inc.Logs
				
				// Sum up errors for metrics
				logErrCount := 0
				for _, e := range inc.Logs.TopErrors {
					logErrCount += e.Count
				}
				
				impactSnap.Metrics = map[string]float64{
					"service_error_count": float64(logErrCount),
				}
			}
			if inc.Traces != nil {
				impactSnap.FullTraces = inc.Traces
				if impactSnap.Metrics == nil {
					impactSnap.Metrics = make(map[string]float64)
				}
				impactSnap.Metrics["trace_error_count"] = float64(inc.Traces.ErrorCount)
				impactSnap.Metrics["p95_latency"] = inc.Traces.P95LatencyMs
			}
			
			lifecycle.Snapshots = append(lifecycle.Snapshots, impactSnap)
			output.Debugf("[Lifecycle] Incident %s added 'impact' snapshot with %d errors", inc.ID, int(impactSnap.Metrics["service_error_count"]))
		}

		// Add final snapshot with all resolution metadata
		lifecycle.Snapshots = append(lifecycle.Snapshots, ml.LifecycleSnapshot{
			Timestamp:  now.Add(1 * time.Second), // Slightly after to preserve order
			IncidentID: inc.ID,
			Phase:      "post",
			Metadata:   metadata,
		})
		if err := s.mlRecorder.PersistLifecycle(lifecycle); err != nil {
			output.Warnf("Failed to persist incident lifecycle for %s: %v", inc.ID, err)
		}

		// 🚀 NEW: Update the main incident record with the lifecycle snapshots
		// This makes them available for 'incident view' without hitting the ML directory.
		inc.LifecycleSnapshots = lifecycle.Snapshots
		if err := s.store.Save(inc); err != nil {
			output.Warnf("Failed to update incident record %s with lifecycle: %v", inc.ID, err)
		}
	}(inc)
}

type Service struct {
	store           *Store
	now             func() time.Time
	user            func() string
	notifiers       []slack.Notifier
	logCorrelator   *LogCorrelator
	traceCorrelator *TraceCorrelator
	discoveryEngine *discovery.DiscoveryEngine
	incidentMetrics *metrics.IncidentMetrics
	metricsProvider *metrics.PrometheusProvider
	mlRecorder      *ml.LifecycleRecorder
}

// GetLogCorrelator returns the log correlator for external access
func (s *Service) GetLogCorrelator() *LogCorrelator {
	return s.logCorrelator
}

// GetTraceCorrelator returns the trace correlator for external access
func (s *Service) GetTraceCorrelator() *TraceCorrelator {
	return s.traceCorrelator
}

// GetDiscoveryEngine returns the discovery engine for external access
func (s *Service) GetDiscoveryEngine() *discovery.DiscoveryEngine {
	return s.discoveryEngine
}

// GetStore returns the underlying incident store
func (s *Service) GetStore() *Store {
	return s.store
}

func NewService(store *Store) (*Service, error) {
	if store == nil {
		var err error
		store, err = NewStore()
		if err != nil {
			return nil, err
		}
	}
	
	// Create notifiers (will include Slack, PagerDuty, or NoOpNotifier if all disabled)
	notifiers, err := slack.CreateNotifiers()
	if err != nil {
		output.Warnf("Failed to create notifiers: %v", err)
		notifiers = []slack.Notifier{&slack.NoOpNotifier{}}
	}
	
	// Create log correlator
	cfg, err := config.Load()
	if err != nil {
		output.Warnf("Failed to load config for log correlator: %v", err)
		cfg = config.Default()
	}
	
	logCorrelator, err := NewLogCorrelator(cfg)
	if err != nil {
		output.Warnf("Failed to create log correlator: %v", err)
		logCorrelator = nil
	}
	
	// Create trace correlator
	traceCorrelator, err := NewTraceCorrelator(cfg)
	if err != nil {
		output.Warnf("Failed to create trace correlator: %v", err)
		traceCorrelator = nil
	}
	
	// Create discovery engine if EKS/K8s is enabled
	var discoveryEngine *discovery.DiscoveryEngine
	if cfg.EKS.Enabled {
		discoveryEngine, err = discovery.NewDiscoveryEngine(discovery.K8sConfig{
			Enabled:    cfg.EKS.Enabled,
			KubeConfig: cfg.EKS.KubeConfig,
			Region:     cfg.EKS.Region,
		})
		if err != nil {
			output.Warnf("Failed to create discovery engine: %v", err)
		} else {
			output.Debugf("Discovery engine created successfully for K8s/EKS")
		}
	}
	
	// Validate configuration on startup and print warnings once
	if len(os.Args) > 1 {
		args := os.Args[1:]
		if config.ShouldShowWarnings(args) {
			config.ValidateConfigOnce(cfg)
		}
	} else {
		// First CLI invocation
		config.ValidateConfigOnce(cfg)
	}
	
	// Create metrics provider for lookback reconstruction
	var metricsProvider *metrics.PrometheusProvider
	if cfg.PrometheusURL != "" {
		metricsProvider = metrics.NewPrometheusProvider(cfg.PrometheusURL, cfg.PrometheusToken, cfg.PrometheusUser, cfg.PrometheusPass)
	}

	return &Service{
		store:           store,
		now:             time.Now,
		user:            SystemUserLabel,
		notifiers:       notifiers,
		logCorrelator:   logCorrelator,
		traceCorrelator: traceCorrelator,
		discoveryEngine: discoveryEngine,
		incidentMetrics: metrics.NewIncidentMetrics(),
		metricsProvider: metricsProvider,
		mlRecorder:      ml.NewLifecycleRecorder(config.GetActiveProfileName()),
	}, nil
}

// convertToSlackIncident converts internal incident to slack incident for notifications
func convertToSlackIncident(inc Incident) slack.Incident {
	// Convert events to slack format
	slackEvents := make([]slack.IncidentEventDetail, len(inc.Events))
	for i, evt := range inc.Events {
		slackEvents[i] = slack.IncidentEventDetail{
			Timestamp: evt.Timestamp,
			User:      evt.User,
			Type:      string(evt.Type),
			Message:   evt.Message,
		}
	}
	
	// Convert LogSummary if present
	var slackLogs *slack.LogSummary
	if inc.Logs != nil {
		topErrors := make([]slack.ErrorStat, len(inc.Logs.TopErrors))
		for i, err := range inc.Logs.TopErrors {
			topErrors[i] = slack.ErrorStat{
				Message: err.Message,
				Count:   err.Count,
				Sample:  err.Sample,
			}
		}
		slackLogs = &slack.LogSummary{
			Backend:   inc.Logs.Backend,
			Window:    inc.Logs.Window,
			TopErrors: topErrors,
		}
	}
	
	return slack.Incident{
		ID:        inc.ID,
		Service:   inc.Service,
		Severity:  slack.Severity(inc.Severity),
		Title:     inc.Title,
		State:     slack.State(inc.State),
		CreatedAt: inc.CreatedAt,
		UpdatedAt: inc.UpdatedAt,
		Summary:   inc.Summary,
		Events:    slackEvents,
		Links: slack.ObservabilityLinks{
			Grafana:    inc.Links.Grafana,
			Prometheus: inc.Links.Prometheus,
			Logs:       inc.Links.Logs,
			Traces:     inc.Links.Traces,
		},
		Logs:   slackLogs,
		Traces: inc.Traces,
		Analysis: func() *slack.IncidentAnalysis {
			if inc.Analysis == nil {
				return nil
			}
			return &slack.IncidentAnalysis{
				Component:      inc.Analysis.Component,
				Dependency:     inc.Analysis.Dependency,
				Pattern:        inc.Analysis.Pattern,
				Category:       inc.Analysis.Category,
				RootCause:      inc.Analysis.RootCause,
				FixSummary:     inc.Analysis.FixSummary,
				FailureType:    inc.Analysis.FailureType,
				ErrorSignature: inc.Analysis.ErrorSignature,
				Prevention:     inc.Analysis.Prevention,
			}
		}(),
		Impact: func() *slack.ImpactMetrics {
			if inc.Impact == nil {
				return nil
			}
			return &slack.ImpactMetrics{
				EstimatedDowntimeMinutes: inc.Impact.EstimatedDowntimeMinutes,
				ImpactedFlows:            inc.Impact.ImpactedFlows,
				CustomMetrics:            inc.Impact.CustomMetrics,
			}
		}(),
		Cluster:    inc.Cluster,
		Namespace:  inc.Namespace,
		Deployment: inc.Deployment,
		K8sContext: inc.K8sContext,
	}
}

func (s *Service) Start(service string, severity Severity, title string, user string) (Incident, []string, error) {
	var incident Incident
	if strings.TrimSpace(service) == "" {
		return incident, nil, errors.New("service is required")
	}
	if strings.TrimSpace(title) == "" {
		return incident, nil, errors.New("title is required")
	}
	if severity == "" {
		return incident, nil, errors.New("severity is required")
	}
	active, warnings, err := s.activeIncidents()
	if err != nil {
		return incident, warnings, err
	}
	if len(active) > 0 {
		return incident, warnings, fmt.Errorf("an active incident already exists (%s). Resolve it first", active[0].ID)
	}
	if user == "" {
		user = s.user()
	} else {
		user = ComposeUser(user)
	}
	for attempts := 0; attempts < 3; attempts++ {
		now := s.now()
		id := s.generateID(now)
		incident = Incident{
			ID:        id,
			Service:   strings.TrimSpace(service),
			Severity:  severity,
			Title:     strings.TrimSpace(title),
			State:     StateStarted,
			Profile:   config.GetActiveProfileName(),
			CreatedAt: now,
			UpdatedAt: now,
			Events: []Event{
				{
					Timestamp: now,
					User:      user,
					Type:      EventStart,
				},
			},
		}
		if err := s.store.SaveNew(incident); err != nil {
			if errors.Is(err, ErrIncidentExists) {
				time.Sleep(1100 * time.Millisecond)
				continue
			}
			return incident, warnings, err
		}
		
		// 🚀 NEW: Enrich with Kubernetes metadata and events synchronously
		if s.discoveryEngine != nil {
			s.EnrichWithK8sMetadata(&incident)
			if err := s.store.Save(incident); err != nil {
				output.Debugf("Failed to save incident %s after K8s enrichment: %v", incident.ID, err)
			}
		}

		// Generate observability links after saving
		output.Debugf("Generating observability links for incident %s", incident.ID)
		updatedIncident, linkErr := s.generateObservabilityLinks(incident)
		output.Debugf("Observability links generation completed with error: %v", linkErr)
		if linkErr != nil {
			// Links generation failure is not critical, log but continue
			warnings = append(warnings, fmt.Sprintf("Failed to generate observability links: %v", linkErr))
		} else {
			incident = updatedIncident
		}
		
		// Start log correlation (async, non-blocking)
		if s.logCorrelator != nil {
			s.logCorrelator.AttachLogSummary(&incident)
			
			// 🚀 NEW: Auto-Lookback Enrichment (Synchronous for CLI reliability)
			s.AutoLookbackEnrichment(&incident, 15*time.Minute)

			// 🚀 NEW: Suggest runbooks after log correlation completes
			// Wait a brief moment for async log correlation to finish
			time.Sleep(2 * time.Second)
			
			output.Infof("Generating runbook suggestions for active incident %s", incident.ID)
			if err := s.suggestRunbooksForActiveIncidentInMemory(&incident); err != nil {
				output.Warnf("Failed to generate runbook suggestions for incident %s: %v", incident.ID, err)
			} else {
				output.Infof("Successfully generated runbook suggestions for incident %s", incident.ID)
			}
			
			// Save the incident one final time with runbook suggestions
			if err := s.store.Save(incident); err != nil {
				output.Warnf("Failed to save incident with runbook suggestions %s: %v", incident.ID, err)
			} else {
				output.Debugf("Successfully saved incident with runbook suggestions %s", incident.ID)
			}
		} else {
			output.Debugf("No log correlator available for incident %s", incident.ID)
		}
		
		// Start trace correlation (async, non-blocking)
		if s.traceCorrelator != nil {
			go func() {
				output.Debugf("Starting trace correlation for incident %s", incident.ID)
				summary := s.traceCorrelator.CorrelateTracesResilient(context.Background(), incident.Service, incident.CreatedAt, time.Time{})
				if summary != nil {
					incident.Traces = summary
					// Copy Grafana URL to Links.Traces for metadata persistence
					if summary.GrafanaURL != "" {
						incident.Links.Traces = summary.GrafanaURL
					}
					// Update incident with trace data
					if err := s.store.Save(incident); err != nil {
						output.Warnf("Failed to update incident with trace data: %v", err)
					} else {
						output.Debugf("Successfully updated incident %s with trace data", incident.ID)
					}
				}
			}()
		}
		
		// Record incident metrics
		s.incidentMetrics.RecordIncidentStarted()
		
		// Send notification synchronously with timeout so that short-lived
		// CLI processes don't exit before the notification is delivered.
		output.Debugf("Starting notification for incident %s", incident.ID)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		
		event := slack.NotificationEvent{
			Type:      "started",
			Incident:  convertToSlackIncident(incident),
			Timestamp: s.now(),
		}
		
		for _, notifier := range s.notifiers {
			if err := notifier.Notify(ctx, event); err != nil {
				output.Warnf("Notifier failed to send incident started notification: %v", err)
			} else {
				output.Debugf("Notifier successfully sent incident started notification for %s", incident.ID)
			}
		}
		
		return incident, warnings, nil
	}
	return incident, warnings, errors.New("failed to allocate incident ID")
}

func (s *Service) StartForServiceFlow(service string, severity Severity, title string, user string) (Incident, []string, error) {
	var incident Incident
	if strings.TrimSpace(service) == "" {
		return incident, nil, errors.New("service is required")
	}
	if strings.TrimSpace(title) == "" {
		return incident, nil, errors.New("title is required")
	}
	if severity == "" {
		return incident, nil, errors.New("severity is required")
	}
	active, warnings, err := s.FindActiveByServiceFlow(service)
	if err != nil {
		return incident, warnings, err
	}
	if active != nil {
		return incident, warnings, fmt.Errorf("an active incident already exists for service %s", strings.TrimSpace(service))
	}
	if user == "" {
		user = s.user()
	} else {
		user = ComposeUser(user)
	}
	for attempts := 0; attempts < 3; attempts++ {
		now := s.now()
		id := s.generateID(now)
		incident = Incident{
			ID:        id,
			Service:   strings.TrimSpace(service),
			Severity:  severity,
			Title:     strings.TrimSpace(title),
			State:     StateStarted,
			Profile:   config.GetActiveProfileName(),
			CreatedAt: now,
			UpdatedAt: now,
			Events: []Event{
				{
					Timestamp: now,
					User:      user,
					Type:      EventStart,
				},
			},
		}
		if err := s.store.SaveNew(incident); err != nil {
			if errors.Is(err, ErrIncidentExists) {
				time.Sleep(1100 * time.Millisecond)
				continue
			}
			return incident, warnings, err
		}
		
		// 🚀 NEW: Enrich with Kubernetes metadata and events synchronously
		if s.discoveryEngine != nil {
			s.EnrichWithK8sMetadata(&incident)
			if err := s.store.Save(incident); err != nil {
				output.Debugf("Failed to save incident %s after K8s enrichment: %v", incident.ID, err)
			}
		}

		// Generate observability links after saving
		updatedIncident, linkErr := s.generateObservabilityLinks(incident)
		if linkErr != nil {
			// Links generation failure is not critical, log but continue
			warnings = append(warnings, fmt.Sprintf("Failed to generate observability links: %v", linkErr))
		} else {
			incident = updatedIncident
		}
		
		// Record incident metrics for auto-created incidents as well.
		s.incidentMetrics.RecordIncidentStarted()
		
		// 🚀 NEW: Auto-Lookback Enrichment (Synchronous for CLI/Monitor reliability)
		if s.logCorrelator != nil {
			s.AutoLookbackEnrichment(&incident, 15*time.Minute)
		}
		
		// Send notification synchronously so that auto-created incidents from
		// the alert listener also produce Slack notifications.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		
		event := slack.NotificationEvent{
			Type:      "started",
			Incident:  convertToSlackIncident(incident),
			Timestamp: s.now(),
		}
		
		for _, notifier := range s.notifiers {
			if err := notifier.Notify(ctx, event); err != nil {
				output.Warnf("Notifier failed to send auto incident started notification: %v", err)
			}
		}
		
		return incident, warnings, nil
	}
	return incident, warnings, errors.New("failed to allocate incident ID")
}

// StartWithMetadata is like Start but also sets metadata
func (s *Service) StartWithMetadata(service string, severity Severity, title string, user string, metadata map[string]string) (Incident, []string, error) {
	inc, warnings, err := s.Start(service, severity, title, user)
	if err == nil && len(metadata) > 0 {
		inc.Metadata = metadata
		if s.store != nil {
			err = s.store.SaveMetadata(inc.ID, metadata)
		}
	}
	return inc, warnings, err
}

// StartForServiceFlowWithMetadata is like StartForServiceFlow but also sets metadata
func (s *Service) StartForServiceFlowWithMetadata(service string, severity Severity, title string, user string, metadata map[string]string) (Incident, []string, error) {
	inc, warnings, err := s.StartForServiceFlow(service, severity, title, user)
	if err == nil && len(metadata) > 0 {
		inc.Metadata = metadata
		if s.store != nil {
			_ = s.store.SaveMetadata(inc.ID, metadata)
		}
		
		if s.logCorrelator != nil {
			output.Infof("Triggering async lookback enrichment for %s with new metadata...", inc.ID)
			go s.AutoLookbackEnrichment(&inc, 15*time.Minute)
		}
	}
	return inc, warnings, err
}

func (s *Service) Suggest(service string, severity Severity, title string, user string) (Incident, []string, error) {
	var incident Incident
	if strings.TrimSpace(service) == "" {
		return incident, nil, errors.New("service is required")
	}
	if strings.TrimSpace(title) == "" {
		return incident, nil, errors.New("title is required")
	}
	if severity == "" {
		return incident, nil, errors.New("severity is required")
	}
	if user == "" {
		user = s.user()
	} else {
		user = ComposeUser(user)
	}
	for attempts := 0; attempts < 3; attempts++ {
		now := s.now()
		id := s.generateID(now)
		incident = Incident{
			ID:        id,
			Service:   strings.TrimSpace(service),
			Severity:  severity,
			Title:     strings.TrimSpace(title),
			State:     StateSuggested,
			CreatedAt: now,
			UpdatedAt: now,
			Events: []Event{
				{
					Timestamp: now,
					User:      user,
					Type:      EventSuggest,
					Message:   "Suggested incident",
				},
			},
		}
		if err := s.store.SaveNew(incident); err != nil {
			if errors.Is(err, ErrIncidentExists) {
				time.Sleep(1100 * time.Millisecond)
				continue
			}
			return incident, nil, err
		}
		
		// Record incident metrics
		s.incidentMetrics.RecordIncidentSuggested()
		
		// Generate observability links after saving
		output.Debugf("Generating observability links for incident %s", incident.ID)
		updatedIncident, linkErr := s.generateObservabilityLinks(incident)
		if linkErr != nil {
			// Links generation failure is not critical, log but continue
			// Note: for Suggest method, we don't have warnings slice, so we just ignore the error
		} else {
			incident = updatedIncident
		}
		
		// Start log correlation (async, non-blocking)
		if s.logCorrelator != nil {
			s.logCorrelator.AttachLogSummary(&incident)
		}
		
		// Send notification asynchronously (non-blocking). This is safe for
		// the long-running alert listener process.
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			
			event := slack.NotificationEvent{
				Type:      "suggested",
				Incident:  convertToSlackIncident(incident),
				Timestamp: s.now(),
			}
			
			for _, notifier := range s.notifiers {
				if err := notifier.Notify(ctx, event); err != nil {
					output.Warnf("Notifier failed to send incident suggested notification: %v", err)
				}
			}
		}()
		
		return incident, nil, nil
	}
	return incident, nil, errors.New("failed to allocate incident ID")
}

// SuggestWithMetadata is like Suggest but also sets metadata
func (s *Service) SuggestWithMetadata(service string, severity Severity, title string, user string, metadata map[string]string) (Incident, []string, error) {
	inc, warnings, err := s.Suggest(service, severity, title, user)
	if err == nil && len(metadata) > 0 {
		inc.Metadata = metadata
		if s.store != nil {
			_ = s.store.SaveMetadata(inc.ID, metadata)
		}
	}
	return inc, warnings, err
}

func (s *Service) Note(message string, user string) (Incident, []string, error) {
	current, warnings, err := s.getActive()
	if err != nil {
		return Incident{}, warnings, err
	}
	if current == nil {
		return Incident{}, warnings, errors.New("no active incident")
	}
	return s.AddEventByID(current.ID, EventNote, message, user)
}

func (s *Service) NoteByID(id string, message string, user string) (Incident, []string, error) {
	return s.AddEventByID(id, EventNote, message, user)
}

func (s *Service) AddEventByID(id string, eventType EventType, message string, user string) (Incident, []string, error) {
	if strings.TrimSpace(id) == "" {
		return Incident{}, nil, errors.New("incident ID is required")
	}
	target, err := s.store.Load(strings.TrimSpace(id))
	if err != nil {
		return Incident{}, nil, err
	}
	if target.State == StateResolved && eventType != EventNote {
		return Incident{}, nil, errors.New("cannot add non-note events to a resolved incident")
	}
	if user == "" {
		user = s.user()
	} else {
		user = ComposeUser(user)
	}
	now := s.now()
	target.Events = append(target.Events, Event{
		Timestamp: now,
		User:      user,
		Type:      eventType,
		Message:   strings.TrimSpace(message),
	})
	target.UpdatedAt = now
	if err := s.store.Save(target); err != nil {
		return Incident{}, nil, err
	}
	_ = audit.LogAction(id, user, fmt.Sprintf("added %s: %s", strings.ToLower(string(eventType)), message))
	return target, nil, nil
}

func (s *Service) UpdateSeverityByID(id string, severity Severity, user string, message string) (Incident, []string, error) {
	if severity == "" {
		return Incident{}, nil, errors.New("severity is required")
	}
	current, err := s.store.Load(id)
	if err != nil {
		return Incident{}, nil, fmt.Errorf("failed to load incident %s: %w", id, err)
	}
	
	if user == "" {
		user = s.user()
	} else {
		user = ComposeUser(user)
	}
	now := s.now()
	current.Severity = severity
	current.UpdatedAt = now
	eventMessage := strings.TrimSpace(message)
	if eventMessage == "" {
		eventMessage = "Severity updated to " + string(severity)
	}
	current.Events = append(current.Events, Event{
		Timestamp: now,
		User:      user,
		Type:      EventSeverity,
		Message:   eventMessage,
	})
	if err := s.store.Save(current); err != nil {
		return Incident{}, nil, err
	}
	return current, nil, nil
}

func (s *Service) UpdateSeverity(severity Severity, user string, message string) (Incident, []string, error) {
	current, warnings, err := s.getActive()
	if err != nil {
		return Incident{}, warnings, err
	}
	if current == nil {
		return Incident{}, warnings, errors.New("no active incident")
	}
	return s.UpdateSeverityByID(current.ID, severity, user, message)
}

func (s *Service) Acknowledge(user string) (Incident, []string, error) {
	var incident Incident
	current, warnings, err := s.getActive()
	if err != nil {
		return incident, warnings, err
	}
	if current == nil {
		return incident, warnings, errors.New("no active incident")
	}
	if err := ValidateTransition(current.State, StateAcknowledged); err != nil {
		return incident, warnings, err
	}
	if user == "" {
		user = s.user()
	} else {
		user = ComposeUser(user)
	}
	now := s.now()
	oldState := current.State
	current.State = StateAcknowledged
	current.UpdatedAt = now
	current.Events = append(current.Events, Event{
		Timestamp: now,
		User:      user,
		Type:      EventAck,
	})
	
	// Record incident metrics with proper state transition
	s.incidentMetrics.RecordStateTransition(string(oldState), string(current.State))
	
	if err := s.store.Save(*current); err != nil {
		return incident, warnings, err
	}
	
	// Send notification synchronously with timeout so the CLI does not exit
	// before the notification is sent.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	event := slack.NotificationEvent{
		Type:      "acknowledged",
		Incident:  convertToSlackIncident(*current),
		Timestamp: s.now(),
	}
	
	for _, notifier := range s.notifiers {
		if err := notifier.Notify(ctx, event); err != nil {
			output.Warnf("Notifier failed to send incident acknowledged notification: %v", err)
		}
	}
	
	return *current, warnings, nil
}

func (s *Service) AcknowledgeByID(id string, user string) (Incident, []string, error) {
	var incident Incident
	if strings.TrimSpace(id) == "" {
		return incident, nil, errors.New("incident ID is required")
	}
	target, err := s.store.Load(strings.TrimSpace(id))
	if err != nil {
		return incident, nil, err
	}
	if err := ValidateTransition(target.State, StateAcknowledged); err != nil {
		return incident, nil, err
	}
	if user == "" {
		user = s.user()
	} else {
		user = ComposeUser(user)
	}
	now := s.now()
	target.State = StateAcknowledged
	target.UpdatedAt = now
	target.Events = append(target.Events, Event{
		Timestamp: now,
		User:      user,
		Type:      EventAck,
	})
	if err := s.store.Save(target); err != nil {
		return incident, nil, err
	}
	_ = audit.LogAction(id, user, "acknowledged incident")
	return target, nil, nil
}

func (s *Service) PromoteSuggested(id string, user string) (Incident, []string, error) {
	var incident Incident
	target, warnings, err := s.findSuggested(id)
	if err != nil {
		return incident, warnings, err
	}
	if target == nil {
		return incident, warnings, errors.New("no suggested incident")
	}
	if err := ValidateTransition(target.State, StateStarted); err != nil {
		return incident, warnings, err
	}
	if user == "" {
		user = s.user()
	} else {
		user = ComposeUser(user)
	}
	now := s.now()
	oldState := target.State
	target.State = StateStarted
	target.UpdatedAt = now
	target.Events = append(target.Events, Event{
		Timestamp: now,
		User:      user,
		Type:      EventStart,
	})
	
	// Record incident metrics with proper state transition
	s.incidentMetrics.RecordStateTransition(string(oldState), string(target.State))
	
	if err := s.store.Save(*target); err != nil {
		return incident, warnings, err
	}
	return *target, warnings, nil
}

func (s *Service) Resolve(summary string, user string, toil *ToilMetadata) (Incident, []string, error) {
	var incident Incident
	if strings.TrimSpace(summary) == "" {
		return incident, nil, errors.New("summary is required")
	}
	current, warnings, err := s.getActive()
	if err != nil {
		return incident, warnings, err
	}
	if current == nil {
		return incident, warnings, errors.New("no active incident")
	}
	if err := ValidateTransition(current.State, StateResolved); err != nil {
		return incident, warnings, err
	}
	if user == "" {
		user = s.user()
	} else {
		user = ComposeUser(user)
	}
	now := s.now()
	oldState := current.State
	current.State = StateResolved
	current.Summary = strings.TrimSpace(summary)
	current.Toil = toil

	// 🚀 NEW: Auto-enrich analysis findings (pattern, signature) from logs before freezing
	current.Analysis = s.enrichAnalysis(current, current.Analysis)
	current.UpdatedAt = now
	current.Events = append(current.Events, Event{
		Timestamp: now,
		User:      user,
		Type:      EventResolve,
		Message:   strings.TrimSpace(summary),
	})
	
	// Record incident metrics with duration and state transition
	s.incidentMetrics.RecordStateTransition(string(oldState), string(current.State))
	s.incidentMetrics.RecordIncidentResolved(current.CreatedAt)

	// Perform final sync of logs and traces synchronously to freeze the state
	s.PerformFinalSync(current, now)

	// 🚀 NEW: Persist full incident lifecycle context
	s.persistLifecycle(*current, now)
	
	// Send notification synchronously with timeout so the CLI does not exit
	// before the notification is sent.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	event := slack.NotificationEvent{
		Type:      "resolved",
		Incident:  convertToSlackIncident(*current),
		Timestamp: s.now(),
	}
	
	for _, notifier := range s.notifiers {
		if err := notifier.Notify(ctx, event); err != nil {
			output.Warnf("Notifier failed to send incident resolved notification: %v", err)
		}
	}
	
	return *current, warnings, nil
}

func (s *Service) ResolveByID(id string, summary string, user string, toil *ToilMetadata) (Incident, []string, error) {
	var incident Incident
	if strings.TrimSpace(id) == "" {
		return incident, nil, errors.New("incident ID is required")
	}
	if strings.TrimSpace(summary) == "" {
		return incident, nil, errors.New("summary is required")
	}
	target, err := s.store.Load(strings.TrimSpace(id))
	if err != nil {
		return incident, nil, err
	}
	if err := ValidateTransition(target.State, StateResolved); err != nil {
		return incident, nil, err
	}
	if user == "" {
		user = s.user()
	} else {
		user = ComposeUser(user)
	}
	now := s.now()
	target.State = StateResolved
	target.Summary = strings.TrimSpace(summary)
	target.Toil = toil

	// 🚀 NEW: Auto-enrich analysis findings (pattern, signature) from logs before freezing
	target.Analysis = s.enrichAnalysis(&target, target.Analysis)
	target.UpdatedAt = now
	target.Events = append(target.Events, Event{
		Timestamp: now,
		User:      user,
		Type:      EventResolve,
		Message:   strings.TrimSpace(summary),
	})
	if err := s.store.Save(target); err != nil {
		return incident, nil, err
	}

	// Perform final sync to capture logs/traces
	s.PerformFinalSync(&target, now)

	// 🚀 NEW: Persist full incident lifecycle context
	s.persistLifecycle(target, now)

	return target, nil, nil
}

// ResolveWithAnalysis resolves the active incident with RCA metadata
func (s *Service) ResolveWithAnalysis(summary string, user string, analysis *IncidentAnalysis, toil *ToilMetadata) (Incident, []string, error) {
	var incident Incident
	if strings.TrimSpace(summary) == "" {
		return incident, nil, errors.New("summary is required")
	}
	current, warnings, err := s.getActive()
	if err != nil {
		return incident, warnings, err
	}
	if current == nil {
		return incident, warnings, errors.New("no active incident")
	}
	if err := ValidateTransition(current.State, StateResolved); err != nil {
		return incident, warnings, err
	}
	
	// Validate and normalize analysis fields (but don't auto-extract)
	if analysis != nil {
		// Validate analysis fields
		if err := ValidateCategory(analysis.Category); err != nil {
			return incident, warnings, err
		}
		if err := ValidateFailureType(analysis.FailureType); err != nil {
			return incident, warnings, err
		}
		
		// Normalize category and failure type to lowercase
		if analysis.Category != "" {
			analysis.Category = strings.ToLower(strings.TrimSpace(analysis.Category))
		}
		if analysis.FailureType != "" {
			analysis.FailureType = strings.ToLower(strings.TrimSpace(analysis.FailureType))
		}
	}
	
	// Set analysis and toil
	current.Analysis = analysis
	current.Toil = toil
	
	if user == "" {
		user = s.user()
	} else {
		user = ComposeUser(user)
	}
	now := s.now()
	oldState := current.State
	current.State = StateResolved
	current.Summary = strings.TrimSpace(summary)
	current.UpdatedAt = now
	current.Events = append(current.Events, Event{
		Timestamp: now,
		User:      user,
		Type:      EventResolve,
		Message:   strings.TrimSpace(summary),
	})
	
	// Record incident metrics with duration and state transition
	s.incidentMetrics.RecordStateTransition(string(oldState), string(current.State))
	s.incidentMetrics.RecordIncidentResolved(current.CreatedAt)
	
	if err := s.store.Save(*current); err != nil {
		return incident, warnings, err
	}
	_ = audit.LogAction(current.ID, user, fmt.Sprintf("resolved: %s", summary))
	
	// Perform final sync of logs and traces synchronously to freeze the state
	s.PerformFinalSync(current, now)

	// 🚀 NEW: Persist full incident lifecycle context
	s.persistLifecycle(*current, now)
	
	// Send notification synchronously with timeout so the CLI does not exit
	// before the notification is sent.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	event := slack.NotificationEvent{
		Type:      "resolved",
		Incident:  convertToSlackIncident(*current),
		Timestamp: s.now(),
	}
	
	for _, notifier := range s.notifiers {
		if err := notifier.Notify(ctx, event); err != nil {
			output.Warnf("Notifier failed to send incident resolved notification: %v", err)
		}
	}
	
	return *current, warnings, nil
}

// ResolveByIDWithAnalysis resolves a specific incident by ID with RCA metadata
func (s *Service) ResolveByIDWithAnalysis(id string, summary string, user string, analysis *IncidentAnalysis, toil *ToilMetadata) (Incident, []string, error) {
	var incident Incident
	if strings.TrimSpace(id) == "" {
		return incident, nil, errors.New("incident ID is required")
	}
	if strings.TrimSpace(summary) == "" {
		return incident, nil, errors.New("summary is required")
	}
	target, err := s.store.Load(strings.TrimSpace(id))
	if err != nil {
		return incident, nil, err
	}
	if err := ValidateTransition(target.State, StateResolved); err != nil {
		return incident, nil, err
	}
	
	// Validate and normalize analysis fields (but don't auto-extract)
	if analysis != nil {
		// Validate analysis fields
		if err := ValidateCategory(analysis.Category); err != nil {
			return incident, nil, err
		}
		if err := ValidateFailureType(analysis.FailureType); err != nil {
			return incident, nil, err
		}
		
		// Normalize category and failure type to lowercase
		if analysis.Category != "" {
			analysis.Category = strings.ToLower(strings.TrimSpace(analysis.Category))
		}
		if analysis.FailureType != "" {
			analysis.FailureType = strings.ToLower(strings.TrimSpace(analysis.FailureType))
		}
	}
	
	// Set analysis and toil
	target.Analysis = analysis
	target.Toil = toil
	
	if user == "" {
		user = s.user()
	} else {
		user = ComposeUser(user)
	}
	now := s.now()
	target.State = StateResolved
	target.Summary = strings.TrimSpace(summary)
	target.UpdatedAt = now
	target.Events = append(target.Events, Event{
		Timestamp: now,
		User:      user,
		Type:      EventResolve,
		Message:   strings.TrimSpace(summary),
	})
	if err := s.store.Save(target); err != nil {
		return incident, nil, err
	}

	// Perform final sync to capture logs/traces
	s.PerformFinalSync(&target, now)

	// 🚀 NEW: Persist full incident lifecycle context
	s.persistLifecycle(target, now)

	return target, nil, nil
}

func (s *Service) View(id string) (*Incident, []string, error) {
	if strings.TrimSpace(id) == "" {
		return s.getLatest()
	}
	incident, err := s.store.Load(strings.TrimSpace(id))
	if err != nil {
		return nil, nil, err
	}
	
	// Hydrate lifecycle snapshots
	s.hydrateLifecycle(&incident)
	
	// Populate Profile field for existing incidents that don't have it
	if incident.Profile == "" {
		incident.Profile = config.GetActiveProfileName()
	}
	
	// Generate observability links if they don't exist
	if incident.Links.Grafana == "" && incident.Links.Prometheus == "" && incident.Links.Logs == "" && incident.Links.Traces == "" {
		updatedIncident, linkErr := s.generateObservabilityLinks(incident)
		if linkErr != nil {
			// Links generation failure is not critical, return original incident
			return &incident, []string{fmt.Sprintf("Failed to generate observability links: %v", linkErr)}, nil
		}
		return &updatedIncident, nil, nil
	}
	
	return &incident, nil, nil
}

func (s *Service) List() ([]Incident, []string, error) {
	incidents, warnings, err := s.store.List()
	if err != nil {
		return incidents, warnings, err
	}
	
	// Get current profile for backward compatibility
	currentProfile := config.GetActiveProfileName()
	
	// Populate Profile field for existing incidents that don't have it
	for i := range incidents {
		if incidents[i].Profile == "" {
			incidents[i].Profile = currentProfile
		}
	}
	
	return incidents, warnings, nil
}

func (s *Service) ListActive() ([]Incident, []string, error) {
	return s.activeIncidents()
}

func (s *Service) FindSuggestedByServiceFlow(service string) (*Incident, []string, error) {
	incidents, warnings, err := s.store.List()
	if err != nil {
		return nil, warnings, err
	}
	var suggested []Incident
	for _, item := range incidents {
		if item.State == StateSuggested {
			suggested = append(suggested, item)
		}
	}
	if len(suggested) == 0 {
		return nil, warnings, nil
	}
	flowIDs, err := flowIDsForService(service)
	if err != nil || len(flowIDs) == 0 {
		for _, item := range suggested {
			if strings.EqualFold(strings.TrimSpace(item.Service), strings.TrimSpace(service)) {
				return &item, warnings, nil
			}
		}
		return nil, warnings, nil
	}
	for _, item := range suggested {
		itemFlows, err := flowIDsForService(item.Service)
		if err != nil {
			continue
		}
		if flowsIntersect(flowIDs, itemFlows) {
			return &item, warnings, nil
		}
	}
	return nil, warnings, nil
}

func (s *Service) FindActiveByServiceFlow(service string) (*Incident, []string, error) {
	incidents, warnings, err := s.activeIncidents()
	if err != nil {
		return nil, warnings, err
	}
	if len(incidents) == 0 {
		return nil, warnings, nil
	}
	flowIDs, err := flowIDsForService(service)
	if err != nil || len(flowIDs) == 0 {
		for _, item := range incidents {
			if strings.EqualFold(strings.TrimSpace(item.Service), strings.TrimSpace(service)) {
				return &item, warnings, nil
			}
		}
		return nil, warnings, nil
	}
	for _, item := range incidents {
		itemFlows, err := flowIDsForService(item.Service)
		if err != nil {
			continue
		}
		if flowsIntersect(flowIDs, itemFlows) {
			return &item, warnings, nil
		}
	}
	return nil, warnings, nil
}

func (s *Service) Export(format string, id string) (string, []string, error) {
	current, warnings, err := s.View(id)
	if err != nil {
		return "", warnings, err
	}
	if current == nil {
		return "", warnings, errors.New("no incidents found")
	}
	path, err := exportIncident(*current, format)
	return path, warnings, err
}

func (s *Service) getActive() (*Incident, []string, error) {
	active, warnings, err := s.activeIncidents()
	if err != nil {
		return nil, warnings, err
	}
	if len(active) == 0 {
		return nil, warnings, nil
	}
	if len(active) > 1 {
		return nil, warnings, fmt.Errorf("multiple active incidents found: %d", len(active))
	}
	target := active[0]
	s.hydrateLifecycle(&target)
	return &target, warnings, nil
}

func (s *Service) activeIncidents() ([]Incident, []string, error) {
	incidents, warnings, err := s.store.List()
	if err != nil {
		return nil, warnings, err
	}
	
	// Get current profile for profile-aware filtering
	currentProfile := config.GetActiveProfileName()
	
	var active []Incident
	for _, incident := range incidents {
		// Skip malformed or empty incidents
		if strings.TrimSpace(incident.ID) == "" {
			continue
		}

		// Populate Profile field for existing incidents that don't have it
		if incident.Profile == "" {
			incident.Profile = currentProfile
		}
		
		// Only consider incidents from the current profile
		if incident.Profile != currentProfile {
			continue
		}
		
		// State must be non-empty and not resolved/suggested
		if incident.State != "" && incident.State != StateResolved && incident.State != StateSuggested {
			active = append(active, incident)
		}
	}
	return active, warnings, nil
}

func (s *Service) getLatest() (*Incident, []string, error) {
	incidents, warnings, err := s.store.List()
	if err != nil {
		return nil, warnings, err
	}
	if len(incidents) == 0 {
		return nil, warnings, nil
	}
	target := incidents[0]
	s.hydrateLifecycle(&target)
	return &target, warnings, nil
}

func (s *Service) findSuggested(id string) (*Incident, []string, error) {
	if strings.TrimSpace(id) != "" {
		incident, err := s.store.Load(strings.TrimSpace(id))
		if err != nil {
			return nil, nil, err
		}
		if incident.State != StateSuggested {
			return nil, nil, errors.New("incident is not suggested")
		}
		return &incident, nil, nil
	}
	incidents, warnings, err := s.store.List()
	if err != nil {
		return nil, warnings, err
	}
	for _, item := range incidents {
		if item.State == StateSuggested {
			return &item, warnings, nil
		}
	}
	return nil, warnings, nil
}

func flowIDsForService(service string) ([]string, error) {
	result, err := flow.LoadOnce()
	if err != nil {
		return nil, err
	}
	for _, warning := range result.Warnings {
		output.Warnf("%s", warning)
	}
	if len(result.Flows) == 0 {
		return nil, nil
	}
	catalog := flow.NewCatalog(result.Flows)
	impacted := catalog.ImpactedFlows(service)
	if len(impacted) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(impacted))
	for _, item := range impacted {
		ids = append(ids, item.ID)
	}
	return ids, nil
}

func flowsIntersect(first []string, second []string) bool {
	if len(first) == 0 || len(second) == 0 {
		return false
	}
	seen := make(map[string]struct{}, len(first))
	for _, id := range first {
		seen[id] = struct{}{}
	}
	for _, id := range second {
		if _, ok := seen[id]; ok {
			return true
		}
	}
	return false
}

// generateObservabilityLinks creates and updates observability links for an incident
func (s *Service) generateObservabilityLinks(incident Incident) (Incident, error) {
	cfg, err := config.Load()
	if err != nil {
		// If config fails, return incident without links (feature is optional)
		return incident, nil
	}
	
	// Generate links based on current incident data
	incident.Links = GenerateObservabilityLinks(incident, cfg)
	
	// Save the updated incident with links
	if err := s.store.Save(incident); err != nil {
		return incident, fmt.Errorf("failed to save incident with observability links: %w", err)
	}
	
	return incident, nil
}

func (s *Service) generateID(now time.Time) string {
	return "INC-" + now.Format("20060102-150405")
}

// FindSimilarIncidents finds historical incidents similar to the given incident
func (s *Service) FindSimilarIncidents(current *Incident, opts SimilarityOptions) ([]SimilarIncident, error) {
	if current == nil {
		return nil, errors.New("current incident is nil")
	}

	// Load all incidents
	allIncidents, _, err := s.store.List()
	if err != nil {
		return nil, fmt.Errorf("failed to load incidents: %w", err)
	}

	// Filter by time window and state
	cutoffTime := time.Now().AddDate(0, 0, -opts.DaysBack)
	var candidates []Incident
	for _, inc := range allIncidents {
		// Skip the current incident itself
		if inc.ID == current.ID {
			continue
		}

		// Filter by time
		if inc.CreatedAt.Before(cutoffTime) {
			continue
		}

		// Filter by state
		if len(opts.IncludeStates) > 0 {
			stateMatch := false
			for _, state := range opts.IncludeStates {
				if inc.State == state {
					stateMatch = true
					break
				}
			}
			if !stateMatch {
				continue
			}
		}

		candidates = append(candidates, inc)
	}

	// Calculate similarity for each candidate
	var results []SimilarIncident
	for _, candidate := range candidates {
		score, matched := CalculateSimilarity(current, &candidate)

		// Filter by minimum confidence
		if score < opts.MinConfidence {
			continue
		}

		results = append(results, SimilarIncident{
			Incident:   candidate,
			Confidence: score,
			MatchedOn:  matched,
			Age:        formatAge(candidate.CreatedAt),
		})
	}

	// Sort by confidence (highest first)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Confidence > results[j].Confidence
	})

	// Apply grouping for identical fixes
	results = groupByFix(results)

	// Limit results
	if len(results) > opts.MaxResults {
		results = results[:opts.MaxResults]
	}

	return results, nil
}

func ValidateIncidentID(id string) error {
	if !idPattern.MatchString(strings.TrimSpace(id)) {
		return fmt.Errorf("invalid incident ID %q (expected INC-YYYYMMDD-HHMMSS)", id)
	}
	return nil
}

// AddActionItem adds a new action item to an incident
func (s *Service) AddActionItem(incidentID, description, owner string, priority ActionItemPriority, status ActionItemStatus, dueDate time.Time) (ActionItem, error) {
	var actionItem ActionItem
	
	if strings.TrimSpace(incidentID) == "" {
		return actionItem, errors.New("incident ID is required")
	}
	if strings.TrimSpace(description) == "" {
		return actionItem, errors.New("description is required")
	}
	if strings.TrimSpace(owner) == "" {
		return actionItem, errors.New("owner is required")
	}
	
	// Load the incident
	incident, err := s.store.Load(strings.TrimSpace(incidentID))
	if err != nil {
		return actionItem, err
	}
	
	// Generate action item ID (sequence is current count + 1)
	sequence := len(incident.ActionItems) + 1
	now := s.now()
	
	actionItem = ActionItem{
		ID:          GenerateActionItemID(incident.ID, sequence),
		Description: strings.TrimSpace(description),
		Owner:       strings.TrimSpace(owner),
		Priority:    priority,
		Status:      status,
		DueDate:     dueDate,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	
	// Add action item to incident
	incident.ActionItems = append(incident.ActionItems, actionItem)
	incident.UpdatedAt = now
	
	// Save incident
	if err := s.store.Save(incident); err != nil {
		return actionItem, err
	}
	_ = audit.LogAction(incidentID, "system", fmt.Sprintf("added action item: %s", description))
	
	return actionItem, nil
}

// UpdateActionItemStatus updates the status of an action item
func (s *Service) UpdateActionItemStatus(incidentID, actionItemID string, newStatus ActionItemStatus) error {
	if strings.TrimSpace(incidentID) == "" {
		return errors.New("incident ID is required")
	}
	if strings.TrimSpace(actionItemID) == "" {
		return errors.New("action item ID is required")
	}
	
	// Load the incident
	incident, err := s.store.Load(strings.TrimSpace(incidentID))
	if err != nil {
		return err
	}
	
	// Find and update the action item
	found := false
	for i := range incident.ActionItems {
		if incident.ActionItems[i].ID == strings.TrimSpace(actionItemID) {
			incident.ActionItems[i].Status = newStatus
			incident.ActionItems[i].UpdatedAt = s.now()
			found = true
			break
		}
	}
	
	if !found {
		return fmt.Errorf("action item %q not found in incident %q", actionItemID, incidentID)
	}
	
	incident.UpdatedAt = s.now()
	
	// Save incident
	return s.store.Save(incident)
}

// UpdateActionItemOwner updates the owner of an action item
func (s *Service) UpdateActionItemOwner(incidentID, actionItemID, newOwner string) error {
	if strings.TrimSpace(incidentID) == "" {
		return errors.New("incident ID is required")
	}
	if strings.TrimSpace(actionItemID) == "" {
		return errors.New("action item ID is required")
	}
	if strings.TrimSpace(newOwner) == "" {
		return errors.New("new owner is required")
	}
	
	// Load the incident
	incident, err := s.store.Load(strings.TrimSpace(incidentID))
	if err != nil {
		return err
	}
	
	// Find and update the action item
	found := false
	for i := range incident.ActionItems {
		if incident.ActionItems[i].ID == strings.TrimSpace(actionItemID) {
			incident.ActionItems[i].Owner = strings.TrimSpace(newOwner)
			incident.ActionItems[i].UpdatedAt = s.now()
			found = true
			break
		}
	}
	
	if !found {
		return fmt.Errorf("action item %q not found in incident %q", actionItemID, incidentID)
	}
	
	incident.UpdatedAt = s.now()
	
	// Save incident
	return s.store.Save(incident)
}

// ListActionItems returns all action items for an incident
func (s *Service) ListActionItems(incidentID string) ([]ActionItem, error) {
	if strings.TrimSpace(incidentID) == "" {
		return nil, errors.New("incident ID is required")
	}
	
	// Load the incident
	incident, err := s.store.Load(strings.TrimSpace(incidentID))
	if err != nil {
		return nil, err
	}
	
	return incident.ActionItems, nil
}

// DeleteActionItem removes an action item from an incident
func (s *Service) DeleteActionItem(incidentID, actionItemID string) error {
	if strings.TrimSpace(incidentID) == "" {
		return errors.New("incident ID is required")
	}
	if strings.TrimSpace(actionItemID) == "" {
		return errors.New("action item ID is required")
	}
	
	// Load the incident
	incident, err := s.store.Load(strings.TrimSpace(incidentID))
	if err != nil {
		return err
	}
	
	// Find and remove the action item
	found := false
	newActionItems := make([]ActionItem, 0, len(incident.ActionItems))
	for _, item := range incident.ActionItems {
		if item.ID == strings.TrimSpace(actionItemID) {
			found = true
			continue // Skip this item (delete it)
		}
		newActionItems = append(newActionItems, item)
	}
	
	if !found {
		return fmt.Errorf("action item %q not found in incident %q", actionItemID, incidentID)
	}
	
	incident.ActionItems = newActionItems
	incident.UpdatedAt = s.now()
	
	// Save incident
	return s.store.Save(incident)
}

// UpdateActionItemPriority updates the priority of an action item
func (s *Service) UpdateActionItemPriority(incidentID, actionItemID string, newPriority ActionItemPriority) error {
	if strings.TrimSpace(incidentID) == "" {
		return errors.New("incident ID is required")
	}
	if strings.TrimSpace(actionItemID) == "" {
		return errors.New("action item ID is required")
	}
	
	// Load the incident
	incident, err := s.store.Load(strings.TrimSpace(incidentID))
	if err != nil {
		return err
	}
	
	// Find and update the action item
	found := false
	for i := range incident.ActionItems {
		if incident.ActionItems[i].ID == strings.TrimSpace(actionItemID) {
			incident.ActionItems[i].Priority = newPriority
			incident.ActionItems[i].UpdatedAt = s.now()
			found = true
			break
		}
	}
	
	if !found {
		return fmt.Errorf("action item %q not found in incident %q", actionItemID, incidentID)
	}
	
	incident.UpdatedAt = s.now()
	
	// Save incident
	return s.store.Save(incident)
}

// UpdateActionItemDueDate updates the due date of an action item
func (s *Service) UpdateActionItemDueDate(incidentID, actionItemID string, newDueDate time.Time) error {
	if strings.TrimSpace(incidentID) == "" {
		return errors.New("incident ID is required")
	}
	if strings.TrimSpace(actionItemID) == "" {
		return errors.New("action item ID is required")
	}
	
	// Load the incident
	incident, err := s.store.Load(strings.TrimSpace(incidentID))
	if err != nil {
		return err
	}
	
	// Find and update the action item
	found := false
	for i := range incident.ActionItems {
		if incident.ActionItems[i].ID == strings.TrimSpace(actionItemID) {
			incident.ActionItems[i].DueDate = newDueDate
			incident.ActionItems[i].UpdatedAt = s.now()
			found = true
			break
		}
	}
	
	if !found {
		return fmt.Errorf("action item %q not found in incident %q", actionItemID, incidentID)
	}
	
	incident.UpdatedAt = s.now()
	
	// Save incident
	return s.store.Save(incident)
}

// suggestRunbooksForActiveIncident generates runbook suggestions for an active incident
func (s *Service) suggestRunbooksForActiveIncident(incidentID string) error {
	output.Debugf("suggestRunbooksForActiveIncident called for %s", incidentID)
	
	// Load the incident to get current state
	incident, err := s.store.Load(incidentID)
	if err != nil {
		return fmt.Errorf("failed to load incident %s: %w", incidentID, err)
	}
	
	output.Debugf("Loaded incident %s, state=%s", incidentID, incident.State)
	
	// Only suggest for active incidents (not resolved)
	if incident.State == StateResolved {
		output.Debugf("Incident %s is already resolved, skipping runbook suggestions", incidentID)
		return nil
	}
	
	// Check if incident has log data for pattern analysis
	if incident.Logs == nil {
		output.Debugf("Incident %s has no Logs object, skipping runbook suggestions", incidentID)
		return nil
	}
	
	if len(incident.Logs.TopErrors) == 0 {
		output.Debugf("Incident %s has no TopErrors (count=0), skipping runbook suggestions", incidentID)
		return nil
	}
	
	output.Debugf("Incident %s has %d top errors, proceeding with runbook suggestions", incidentID, len(incident.Logs.TopErrors))
	
	// 🚀 NEW: Context-aware runbook matching
	if !config.IsRunbookSuggestionsEnabled() {
		output.Debugf("Runbook suggestions are disabled, skipping for incident %s", incidentID)
		return nil
	}
	
	// Print planned suggestion for preview
	output.Infof("Would generate runbook suggestions for incident %s with pattern: %s", 
		incidentID, incident.Logs.TopErrors[0].Message)
	
	return nil
}

// suggestRunbooksForActiveIncidentInMemory generates runbook suggestions using in-memory incident data
func (s *Service) suggestRunbooksForActiveIncidentInMemory(incident *Incident) error {
	output.Debugf("suggestRunbooksForActiveIncidentInMemory called for %s", incident.ID)
	
	// Context-aware runbook matching
	if !config.IsRunbookSuggestionsEnabled() {
		output.Debugf("Runbook suggestions are disabled, skipping for incident %s", incident.ID)
		return nil
	}
	
	if incident.Logs == nil || len(incident.Logs.TopErrors) == 0 {
		output.Debugf("No log data available for incident %s, skipping suggestions", incident.ID)
		return nil
	}
	
	output.Debugf("Incident %s has %d top errors in memory, proceeding with runbook suggestions", incident.ID, len(incident.Logs.TopErrors))
	
	topError := incident.Logs.TopErrors[0].Message
	output.Debugf("Top error message for incident %s: %s", incident.ID, topError)
	
	// Check if this error matches any known pattern
	pattern, confidence, runbookURL := s.matchErrorToRunbook(topError)
	
	// Fallback to sample message if pattern extraction returns empty
	if pattern == "" {
		topError = incident.Logs.TopErrors[0].Sample
		if topError != "" {
			output.Debugf("Using sample message for incident %s: %s", incident.ID, topError)
			pattern, confidence, runbookURL = s.matchErrorToRunbook(topError)
		}
	}
	
	output.Debugf("Extracted pattern for incident %s: %s", incident.ID, pattern)
	
	var confVal float64
	if confidence != "" {
		output.Debugf("Calculated confidence for pattern %s: %s", pattern, confidence)
		
		// Parse confidence to float
		var err error
		confVal, err = strconv.ParseFloat(strings.TrimSuffix(confidence, "%"), 64)
		if err != nil {
			output.Warnf("Failed to parse confidence '%s' for incident %s: %v", confidence, incident.ID, err)
			confVal = 0
		}
		
		if confVal < config.GetRunbookConfidenceThreshold() {
			output.Debugf("Pattern confidence %.2f below threshold %.2f for incident %s, skipping suggestion", 
				confVal, config.GetRunbookConfidenceThreshold(), incident.ID)
			return nil
		}
	}
	
	if runbookURL != "" {
		output.Infof("Generated runbook suggestion for incident %s: pattern='%s' -> runbook='%s' (confidence: %s)", 
			incident.ID, pattern, runbookURL, confidence)
	
		// Track metrics if enabled
		if config.IsRunbookMetricsEnabled() {
			s.trackRunbookMetrics(pattern, confVal)
		}
		
		// Add to incident metadata
		output.Debugf("Attempting to save incident %s with runbook suggestion to disk", incident.ID)
		if err := s.store.SaveRunbookSuggestion(incident.ID, pattern, runbookURL); err != nil {
			output.Warnf("Failed to save runbook suggestion for incident %s: %v", incident.ID, err)
		} else {
			output.Debugf("Successfully saved runbook suggestion to incident metadata for %s", incident.ID)
		}
	}
	
	return nil
}

// checkCustomPatterns checks if any custom patterns match the error message
func (s *Service) checkCustomPatterns(customPatterns []config.CustomPatternConfig, errorMessage string) (string, string, string) {
	msg := strings.ToLower(errorMessage)
	for _, cp := range customPatterns {
		pattern := strings.ToLower(cp.Pattern)
		if strings.Contains(msg, pattern) {
			output.Debugf("Matched custom pattern '%s' for error", cp.Name)
			// Return pattern name, high confidence for manual patterns, and runbook name
			return cp.Name, "95%", "/runbooks/" + cp.Runbook
		}
	}
	
	return "", "", ""
}

// trackRunbookMetrics tracks metrics for runbook suggestions
func (s *Service) trackRunbookMetrics(pattern string, confidence float64) {
	// This would integrate with the metrics system
	// For now, just log the metrics
	output.Metricf("Runbook suggestion generated - pattern: %s, confidence: %.2f", pattern, confidence)
}

// matchErrorToRunbook matches an error message to a runbook pattern (localized to break import cycle)
func (s *Service) matchErrorToRunbook(errorMessage string) (string, string, string) {
	pattern := ExtractPatternFromLog(errorMessage)
	if pattern == "general_error" || pattern == "" {
		return "", "", ""
	}
	
	// For now, return pattern as confidence since we don't have historical context here
	// In a real scenario, this would look up historical confidence
	confidence := "85%" 
	
	// Mock runbook URL - in production this would come from a database or config
	runbookURL := fmt.Sprintf("/runbooks/%s", pattern)
	
	return pattern, confidence, runbookURL
}

// EnrichWithK8sMetadata fetches recent K8s events for the service and attaches them to the incident analysis
func (s *Service) EnrichWithK8sMetadata(inc *Incident) {
	if s.discoveryEngine == nil {
		return
	}

	output.Debugf("Enriching incident %s with K8s metadata and events", inc.ID)
	
	// Set Kubernetes metadata if not already set
	if inc.Cluster == "" || inc.Namespace == "" {
		pm := config.GetProfileManager()
		activeProfileName := pm.GetActiveProfile()
		activeProfile, _ := pm.GetProfile(activeProfileName)
		
		var preferredNamespaces []string
		if activeProfile != nil {
			if activeProfile.Cluster != "" {
				inc.Cluster = activeProfile.Cluster
			}
			preferredNamespaces = activeProfile.Namespaces
		}
		
		// Map K8s context
		inc.K8sContext = s.discoveryEngine.GetCurrentContext()
		
		// Parse namespace/deployment from service
		// Standard format: "namespace/deployment"
		parts := strings.Split(inc.Service, "/")
		if len(parts) == 2 {
			inc.Namespace = parts[0]
			inc.Deployment = parts[1]
		} else {
			// Smarter discovery: Try to find the exact namespace for this resource name
			inc.Deployment = inc.Service
			foundNamespace, foundType, err := s.discoveryEngine.FindNamespaceForResource(inc.Service, preferredNamespaces)
			if err == nil && foundNamespace != "" {
				inc.Namespace = foundNamespace
				output.Debugf("Dynamically discovered %s in namespace %q for resource %q", foundType, inc.Namespace, inc.Service)
			} else if len(preferredNamespaces) > 0 {
				// Fallback ONLY to specifically preferred namespaces if provided in profile
				inc.Namespace = preferredNamespaces[0]
				output.Debugf("Could not discover namespace for %q, falling back to preferred %q", inc.Service, inc.Namespace)
			}
		}
	}

	// 1. Fetch Workload Events (Deployment/Svc etc)
	events, err := s.discoveryEngine.DiscoverEvents(inc.Namespace, inc.Deployment)
	if err != nil {
		output.Warnf("Failed to discover workload events for %s/%s: %v", inc.Namespace, inc.Deployment, err)
	}

	// 2. Fetch Pod Events (Deep diagnostic for ImagePullBackOff, OOMKilled etc)
	podEvents, err := s.discoveryEngine.DiscoverPodEvents(inc.Namespace, inc.Deployment)
	if err == nil && len(podEvents) > 0 {
		events = append(events, podEvents...)
	}

	if len(events) == 0 {
		return
	}

	if inc.Analysis == nil {
		inc.Analysis = &IncidentAnalysis{}
	}

	// Format events for analysis
	eventText := "K8s Events:\n" + strings.Join(events, "\n")
	
	if inc.Analysis.RootCause == "" {
		inc.Analysis.RootCause = eventText
	} else if strings.Contains(inc.Analysis.RootCause, "K8s Events:") {
		// Replace old K8s events with fresh ones
		parts := strings.Split(inc.Analysis.RootCause, "K8s Events:")
		inc.Analysis.RootCause = strings.TrimSpace(parts[0]) + "\n\n" + eventText
	} else {
		// Append to existing root cause
		inc.Analysis.RootCause = strings.TrimSpace(inc.Analysis.RootCause) + "\n\n" + eventText
	}
	
	output.Debugf("Successfully enriched incident %s with %d events", inc.ID, len(events))
}

// PerformFinalSync captures the final state of logs and traces at resolution
func (s *Service) PerformFinalSync(inc *Incident, endTime time.Time) {
	if inc == nil {
		return
	}
	
	output.Infof("Performing final log/trace sync for incident %s", inc.ID)
	
	// 1. Sync Logs
	if s.logCorrelator != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := s.logCorrelator.AttachLogSummarySync(ctx, inc, endTime)
		cancel()
		if err != nil {
			output.Warnf("Final log sync failed: %v", err)
		}
	}
	
	// 2. Sync Traces
	if s.traceCorrelator != nil && s.traceCorrelator.IsEnabled() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		summary, err := s.traceCorrelator.CorrelateTracesLive(ctx, inc.Service, inc.CreatedAt, endTime)
		cancel()
		if err != nil {
			output.Warnf("Final trace sync failed: %v", err)
		} else if summary != nil {
			inc.Traces = summary
			if summary.GrafanaURL != "" {
				inc.Links.Traces = summary.GrafanaURL
			}
		}
	}

	// 🚀 NEW: Persist the incident after final sync
	if err := s.store.Save(*inc); err != nil {
		output.Warnf("Failed to save incident %s after final sync: %v", inc.ID, err)
	} else {
		output.Infof("Incident %s persisted after final log/trace sync", inc.ID)
	}
}

// AutoLookbackEnrichment attempts to reconstruct history for an incident by querying Loki
// for the previous 15 minutes of logs and classifying signals.
func (s *Service) AutoLookbackEnrichment(inc *Incident, lookback time.Duration) {
	if s.logCorrelator == nil || s.mlRecorder == nil || s.logCorrelator.aggregator == nil {
		return
	}

	// 1. Check if we already have a relevant and fresh buffer (maybe the daemon IS running)
	existing := s.mlRecorder.GetPreIncidentBuffer()
	hasFreshRelevantData := false
	if len(existing) > 5 {
		// Check the last few snapshots for freshness and service relevance
		last := existing[len(existing)-1]
		isFresh := time.Since(last.Timestamp) < 5*time.Minute
		isRelevant := last.Metadata["primary_service"] == inc.Service || 
		             strings.Contains(last.Metadata["monitored_services"], inc.Service)
		
		if isFresh && isRelevant {
			hasFreshRelevantData = true
		}
	}

	if hasFreshRelevantData {
		output.Infof("Auto-Lookback Enrichment: Lifecycle buffer for %s is fresh and relevant, seeding incident %s", inc.Profile, inc.ID)
		// 🚀 NEW: Even if we skip reconstruction, we MUST seed the incident 
		// so that 'incident view' can show the pre-incident context immediately.
		if err := s.mlRecorder.PersistSeed(inc.ID, existing); err != nil {
			output.Warnf("Failed to persist existing buffer as seed: %v", err)
		}
		return
	}

	output.Infof("Auto-Lookback Enrichment: Reconstructing history for incident %s (%v lookback)...", inc.ID, lookback)

	// 2. Divide lookback into 3-minute segments (5 segments for 15m)
	segment := 3 * time.Minute
	if lookback < segment {
		segment = lookback / 2
	}

	agent := ml.NewMLAgent("backfill_v1")
	
	// Process segments in chronological order
	start := inc.CreatedAt.Add(-lookback)
	numSegments := int(lookback / segment)
	
	var reconstructed []ml.LifecycleSnapshot
	
	for i := 0; i < numSegments; i++ {
		bucketStart := start.Add(time.Duration(i) * segment)
		bucketEnd := bucketStart.Add(segment)

		segmentCtx, segmentCancel := context.WithTimeout(context.Background(), 60*time.Second) // Increased from 15s
		
		// 🚀 Optimization: Only retry on empty results for the VERY LAST segment (closest to incident start).
		// This handles Loki ingestion lag for pre-failure signals while keeping the rest of the lookback fast.
		retryOnEmpty := (i == numSegments-1)
		summary, err := s.logCorrelator.aggregator.CorrelateLogsResilient(segmentCtx, inc.Service, bucketStart, bucketEnd, retryOnEmpty)
		
		if err != nil || summary == nil {
			segmentCancel()
			continue
		}

		// Create snapshot
		snapshot := ml.LifecycleSnapshot{
			Timestamp:  bucketEnd,
			Metrics:    make(map[string]float64), // Ensure metrics is initialized
			FullLogs: &logs.LogSummary{
				Backend:   summary.Backend,
				Window:    fmt.Sprintf("backfill %s to %s", bucketStart.Format("15:04:05"), bucketEnd.Format("15:04:05")),
				TopErrors: summary.TopErrors,
			},
			Metadata: map[string]string{
				"primary_service":    inc.Service,
				"service":            inc.Service, // redundant for filter
				"monitored_services": inc.Service, // Tag for inclusive filtering
				"source":             "auto_lookback",
			},
		}

		// Reconstruct Traces if correlator is available
		if s.traceCorrelator != nil {
			if traceSummary, err := s.traceCorrelator.Correlate(segmentCtx, inc.Service, bucketStart, bucketEnd); err == nil && traceSummary != nil {
				snapshot.FullTraces = traceSummary
			}
		}
		segmentCancel()

		// 3. Error count from logs (Total occurrences, not just unique messages)
		totalErrors := 0
		for _, e := range summary.TopErrors {
			totalErrors += e.Count
		}

		// Reconstruct Metrics if provider is available
		if s.metricsProvider != nil {
			cfg, _ := config.Load() // Reload to get metric names
			// 1. RPS
			if cfg.RequestCountMetric != "" {
				query := fmt.Sprintf("sum(rate(%s{service=\"%s\"}[5m]))", cfg.RequestCountMetric, inc.Service)
				if val, err := s.metricsProvider.QueryInstant(query); err == nil {
					snapshot.Metrics["service_request_rate"] = val
				}
			}
			// 2. Latency
			if cfg.LatencyMetric != "" {
				query := fmt.Sprintf("avg(%s{service=\"%s\"})", cfg.LatencyMetric, inc.Service)
				if val, err := s.metricsProvider.QueryInstant(query); err == nil {
					snapshot.Metrics["service_avg_latency"] = val
				}
			}
			
			snapshot.Metrics["service_error_count"] = float64(totalErrors)
		}

		// 🚀 IMPORTANT: Classify BEFORE tagging with incident ID
		// Otherwise, every snapshot becomes "during" phase automatically.
		snapshot.Phase = agent.ClassifySignal(snapshot)
		
		// NOW tag for strict ownership filtering
		snapshot.IncidentID = inc.ID
		snapshot.Metadata["active_incident_id"] = inc.ID
		
		output.Debugf("  • Reconstructed snapshot at %s: phase=%s, errors=%d", 
			bucketEnd.Format("15:04:05"), snapshot.Phase, totalErrors)
		
		reconstructed = append(reconstructed, snapshot)
		s.mlRecorder.PushSnapshot(snapshot)
	}
	fmt.Println()

	// 3. Persist the newly "seeded" buffer to incident-specific seed file
	// This ensures reconstructed history survives daemon overwrites!
	if err := s.mlRecorder.PersistSeed(inc.ID, reconstructed); err != nil {
		output.Warnf("Failed to persist reconstructed seed: %v", err)
	}

	if err := s.mlRecorder.PersistBuffer(); err != nil {
		output.Warnf("Failed to persist reconstructed buffer: %v", err)
	}

	output.Infof("Auto-Lookback Enrichment complete for %s. Reconstructed %d segments.", inc.ID, len(reconstructed))
}

func (s *Service) hydrateLifecycle(incident *Incident) {
	if s.mlRecorder == nil || incident == nil {
		return
	}
	if incident.IsResolved() {
		if lc, err := s.mlRecorder.LoadLifecycle(incident.ID); err == nil && lc != nil {
			incident.LifecycleSnapshots = lc.Snapshots
		}
	} else {
		// For active incidents, check for reconstructed seeds (Lookback Persistence)
		if seeds, err := s.mlRecorder.LoadSeed(incident.ID); err == nil && len(seeds) > 0 {
			incident.LifecycleSnapshots = seeds
		}
	}
}
