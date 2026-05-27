package ml

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"log"

	"health-monitor/internal/config"
	"health-monitor/internal/metrics"
	"health-monitor/internal/notify/slack"
	"health-monitor/internal/storage"
	"health-monitor/internal/flow"
	"health-monitor/internal/logs"
	"health-monitor/internal/analyse/loki"
	"health-monitor/internal/traces"
)

// IncidentChecker defines an interface to check for active incidents
type IncidentChecker interface {
	GetActiveIncidentInfo() (id string, service string, active bool)
}

// FileSystemIncidentChecker implements IncidentChecker by scanning the storage directory
type FileSystemIncidentChecker struct {
	profile string
}

func (c *FileSystemIncidentChecker) GetActiveIncidentInfo() (string, string, bool) {
	pm := config.GetProfileManager()
	stateDir := pm.GetStatePathForProfile(c.profile)
	incidentDir := filepath.Join(stateDir, "incidents")

	// Scan current month and last month for simplicity
	now := time.Now()
	months := []time.Time{now, now.AddDate(0, -1, 0)}

	var latestID string
	var latestService string
	var latestTime time.Time

	for _, m := range months {
		dir := filepath.Join(incidentDir, m.Format("2006"), m.Format("01"))
		files, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
				continue
			}

			path := filepath.Join(dir, f.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}

			// Partial struct to detect status without importing incident package
			var stub struct {
				ID        string    `json:"id"`
				Service   string    `json:"service"`
				State     string    `json:"state"`
				CreatedAt time.Time `json:"created_at"`
			}
			if err := json.Unmarshal(data, &stub); err == nil {
				// We assume "resolved" is the keyword for completion
				if stub.State != "" && stub.State != "resolved" {
					if stub.CreatedAt.After(latestTime) {
						latestTime = stub.CreatedAt
						latestID = stub.ID
						latestService = stub.Service
					}
				}
			}
		}
	}
	if latestID != "" {
		return latestID, latestService, true
	}
	return "", "", false
}

// PredictorDaemon runs background scanning and prediction
type PredictorDaemon struct {
	agent         *MLAgent
	recorder      *LifecycleRecorder
	ticker        *time.Ticker
	stop          chan struct{}
	onPrediction  func(Prediction)
	profile       string
	profileData   *config.Profile
	config        *config.Config
	promProvider  *metrics.PrometheusProvider
	lokiClient    *loki.Client
	logAggregator   *logs.Aggregator
	traceCorrelator *traces.Correlator
	incidentChecker IncidentChecker
	mutex           sync.Mutex
	lastPredictions map[string]time.Time // 🚀 NEW: Survive across ticks
	bootstrapWindow time.Duration       // 🚀 NEW: One-time backfill window
	bootstrapDone   bool
}

func NewPredictorDaemon(profile string, agent *MLAgent, recorder *LifecycleRecorder) *PredictorDaemon {
	pm := config.GetProfileManager()
	defaultCfg := config.Default()
	cfg := &defaultCfg
	var profileData *config.Profile
	var promProv *metrics.PrometheusProvider
	var lokiCli *loki.Client
	
	if p, err := pm.GetProfile(profile); err == nil {
		profileData = p
		cfg = &p.Config
		if cfg.PrometheusURL != "" {
			promProv = metrics.NewPrometheusProvider(cfg.PrometheusURL, cfg.PrometheusToken, cfg.PrometheusUser, cfg.PrometheusPass)
		}
		if cfg.LokiURL != "" {
			lokiCli = &loki.Client{
				BaseURL: cfg.LokiURL,
				Token:   cfg.LokiToken,
				User:    cfg.LokiUser,
				Pass:    cfg.LokiPass,
				Timeout: 15 * time.Second,
			}
		}
	}

	d := &PredictorDaemon{
		profile:         profile,
		profileData:     profileData,
		agent:           agent,
		recorder:        recorder,
		stop:            make(chan struct{}),
		config:          cfg,
		promProvider:    promProv,
		lokiClient:      lokiCli,
		lastPredictions: make(map[string]time.Time),
	}

	// Initialize log aggregator if Loki is available
	if d.lokiClient != nil {
		// Use 2m window for prediction snapshots to catch transient spikes 
		// (vs 5m-15m for full incidents)
		window := 2 * time.Minute
		if cfg.Window != "" {
			if parsed, err := time.ParseDuration(cfg.Window); err == nil {
				window = parsed
			}
		}

		backend, err := logs.NewLokiClient(
			cfg.LokiURL,
			cfg.LokiUser,
			cfg.LokiPass,
			cfg.LokiToken,
			cfg.LokiServiceLabel,
			cfg.LokiErrorRegex,
		)
		if err == nil && backend != nil {
			d.logAggregator = logs.NewAggregator(backend, window)
		}
	}

	// Initialize trace correlator if configured
	if cfg.TraceURL != "" {
		if tc, err := traces.NewCorrelator(*cfg); err == nil {
			d.traceCorrelator = tc
		}
	}

	return d
}

// SetBackfillWindow sets a one-time bootstrap window for the first scan
func (d *PredictorDaemon) SetBackfillWindow(w time.Duration) {
	d.bootstrapWindow = w
}

// SetNotifyHandler sets a callback for high-confidence predictions
func (d *PredictorDaemon) SetNotifyHandler(f func(Prediction)) {
	d.onPrediction = f
}

func (d *PredictorDaemon) SetIncidentChecker(c IncidentChecker) {
	d.incidentChecker = c
}

// Start begins the scanning loop
func (d *PredictorDaemon) Start(interval time.Duration) {
	d.ticker = time.NewTicker(interval)
	// Hydrate ML Agent with past incident fingerprints
	d.loadIncidentFingerprints()

	log.Printf("Predictor daemon started (interval: %v)", interval)

	// 🔥 IMMEDIATE FIRST TICK (with backfill if set)
	d.tick()

	go func() {
		tickCount := 0
		for {
			select {
			case <-d.ticker.C:
				tickCount++
				// Periodically reload fingerprints (every 10 ticks) 
				// to learn from new incidents without restart
				if tickCount%10 == 0 {
					d.loadIncidentFingerprints()
				}
				d.tick()
			case <-d.stop:
				return
			}
		}
	}()
}

// Stop halts the scanning loop
func (d *PredictorDaemon) Stop() {
	if d.ticker != nil {
		d.ticker.Stop()
	}
	close(d.stop)
}

func (d *PredictorDaemon) tick() {
	// 1. Combine all service sources
	serviceMap := make(map[string]bool)

	// A. Explicit config
	if d.config.APIService != "" {
		serviceMap[d.config.APIService] = true
	}

	// B. Flow-defined services (The primary source of truth)
	for _, svc := range d.collectServicesFromFlows() {
		serviceMap[svc] = true
	}

	// C. Dynamic Discovery (To catch new active services)
	for _, svc := range d.discoverActiveServices() {
		serviceMap[svc] = true
	}

	var services []string
	for svc := range serviceMap {
		services = append(services, svc)
	}

	log.Printf("[INFO] Predictor scanning %d unique services", len(services))

	// 🕵️ Check for active incident to prioritize
	var incidentService string
	var activeIncidentID string
	if d.incidentChecker != nil {
		if id, svc, active := d.incidentChecker.GetActiveIncidentInfo(); active {
			activeIncidentID = id
			incidentService = svc
		}
	}

	// 2. Parallel Isolated Scanning
	// We use a semaphore to limit concurrency (e.g., 5 concurrent scans) 
	// to avoid overloading Loki/Prometheus on large profiles.
	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup

	for _, svc := range services {
		if svc == "" {
			continue
		}

		wg.Add(1)
		go func(service string) {
			defer wg.Done()
			sem <- struct{}{}        // Acquire
			defer func() { <-sem }() // Release

			// 🚀 Only apply active incident ID to the specific service it belongs to
			scopedIncidentID := ""
			if service == incidentService {
				scopedIncidentID = activeIncidentID
			}

			// 3. Capture isolated state
			snapshot := d.captureServiceState(service, scopedIncidentID)

			// 4. Push to rolling buffer
			d.recorder.PushSnapshot(snapshot)

			// 5. Run prediction analysis
			prediction := d.agent.PredictState(snapshot)
			if prediction != nil {
				prediction.Component = service

				// 🚀 PER-SERVICE DEBOUNCING (Production Grade)
				// Silence ALL predictions for this service for 10 minutes 
				// once any prediction is triggered to prevent alarm fatigue.
				d.mutex.Lock()
				lastSeen, exists := d.lastPredictions[service]
				d.mutex.Unlock()

				if exists && time.Since(lastSeen) < 10*time.Minute {
					return
				}

				// 🚀 PERSISTENT DEBOUNCING (Survives Restarts)
				// Check disk for identical predictions in the last 10m
				if d.hasRecentPredictionOnDisk(service, prediction.Pattern) {
					return
				}

				if prediction.Confidence >= 0.1 {
					d.mutex.Lock()
					d.lastPredictions[service] = time.Now()
					d.mutex.Unlock()

					log.Printf("PREDICTION [%s]: Pattern matched (%s) with confidence %.2f. Explanation: %s\n", 
						service, prediction.Pattern, prediction.Confidence, prediction.Explanation)
					
					// 6. Notifications and Persistence
					_ = d.sendSlackNotification(*prediction)
					_ = d.persistPrediction(*prediction)
				}
			}
		}(svc)
	}

	wg.Wait()

	// 🚀 Mark bootstrap as complete
	d.mutex.Lock()
	d.bootstrapDone = true
	d.mutex.Unlock()

	// 8. Persist buffer to disk for CLI handshake
	if err := d.recorder.PersistBuffer(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to persist telemetry buffer: %v\n", err)
	}
}

func (d *PredictorDaemon) collectServicesFromFlows() []string {
	res, err := flow.LoadProfileAware()
	if err != nil {
		return nil
	}

	seen := make(map[string]bool)
	var services []string
	for _, f := range res.Flows {
		for _, svc := range f.Services {
			if !seen[svc] {
				services = append(services, svc)
				seen[svc] = true
			}
		}
	}
	return services
}

// loadIncidentFingerprints scans the incident lifecycle directory for past fingerprints
func (d *PredictorDaemon) loadIncidentFingerprints() {
	pm := config.GetProfileManager()
	stateDir := pm.GetStatePathForProfile(d.profile)
	lifecycleDir := filepath.Join(stateDir, "incidents", "lifecycle")

	files, err := os.ReadDir(lifecycleDir)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[DEBUG] Failed to read lifecycle directory: %v", err)
		}
		return
	}

	var allFingerprints []LifecycleSnapshot
	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(f.Name(), "_lifecycle.json") {
			path := filepath.Join(lifecycleDir, f.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}

			var lifecycle FullLifecycle
			if err := json.Unmarshal(data, &lifecycle); err == nil {
				// We need a snapshot that has BOTH logs (for matching) 
				// AND RCA metadata (for resolution)
				var logSnapshot *LifecycleSnapshot
				var rcaData map[string]string

				// 1. Find RCA from 'post' phase
				for i := range lifecycle.Snapshots {
					if lifecycle.Snapshots[i].Phase == "post" {
						rcaData = lifecycle.Snapshots[i].Metadata
						break
					}
				}

				// 2. Find richest log snapshot from ANY phase (pre, during, steady)
				var maxErrors int
				for i := range lifecycle.Snapshots {
					s := &lifecycle.Snapshots[i]
					if s.FullLogs != nil && len(s.FullLogs.TopErrors) > maxErrors {
						maxErrors = len(s.FullLogs.TopErrors)
						logSnapshot = s
					}
				}

				// 3. Fallback: if no enrichment during incident, use any with logs
				if logSnapshot == nil {
					for i := range lifecycle.Snapshots {
						if lifecycle.Snapshots[i].FullLogs != nil {
							logSnapshot = &lifecycle.Snapshots[i]
							break
						}
					}
				}

				if logSnapshot != nil {
					// Merge RCA into metadata so MLAgent can pull it
					if logSnapshot.Metadata == nil {
						logSnapshot.Metadata = make(map[string]string)
					}
					if rcaData != nil {
						for k, v := range rcaData {
							logSnapshot.Metadata[k] = v
						}
					}
					logSnapshot.IncidentID = lifecycle.IncidentID
					allFingerprints = append(allFingerprints, *logSnapshot)
				}
			}
		}
	}

	if len(allFingerprints) > 0 {
		d.agent.UpdateKnowledge(allFingerprints)
		log.Printf("[INFO] Loaded %d incident fingerprints into ML Knowledge Base", len(allFingerprints))
	}
}

func (d *PredictorDaemon) sendSlackNotification(p Prediction) error {
	notifiers, err := slack.CreateNotifiers()
	if err != nil {
		return err
	}

	analysis := &slack.IncidentAnalysis{
		Component:      p.Component,
		Pattern:        p.Pattern,
		Category:       p.Category,
		RootCause:      p.RootCause,
		FixSummary:     p.FixSummary,
		Prevention:     p.Prevention,
		MatchConfidence: p.Confidence,
		MatchSource:    p.ModelID,
	}

	if p.LessonsLearned != nil {
		analysis.LessonsLearned = &slack.LessonsLearned{
			WhatWentWell:      p.LessonsLearned.WhatWentWell,
			WhatCouldBeBetter: p.LessonsLearned.WhatCouldBeBetter,
			WhereWeGotLucky:   p.LessonsLearned.WhereWeGotLucky,
		}
	}

	event := slack.NotificationEvent{
		Type: "suggested", // Use suggested state for predictions
		Incident: slack.Incident{
			ID:        p.ID,
			Service:   p.Component,
			Severity:  slack.SeverityP2,
			Title:     "Predictive Prevention: " + p.Pattern,
			State:     slack.StateSuggested,
			CreatedAt: p.Timestamp,
			Summary:   p.Explanation,
			Analysis:  analysis,
		},
		Timestamp: time.Now(),
	}

	// Map ActionItems if present
	if len(p.ActionItems) > 0 {
		for i, desc := range p.ActionItems {
			event.Incident.ActionItems = append(event.Incident.ActionItems, slack.ActionItem{
				ID:          fmt.Sprintf("ACT-%d", i),
				Description: desc,
				Status:      "TODO",
				Priority:    "Medium",
			})
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var lastErr error
	for _, n := range notifiers {
		if err := n.Notify(ctx, event); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

func (d *PredictorDaemon) persistPrediction(p Prediction) error {
	pm := config.GetProfileManager()
	stateDir := pm.GetStatePathForProfile(d.profile)
	predDir := filepath.Join(stateDir, "predictions")

	if err := os.MkdirAll(predDir, 0755); err != nil {
		return fmt.Errorf("failed to create predictions directory: %w", err)
	}

	filename := fmt.Sprintf("%s.json", p.ID)
	path := filepath.Join(predDir, filename)

	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}

	return storage.AtomicWriteFile(path, data, 0600)
}

func (d *PredictorDaemon) hasRecentPredictionOnDisk(service, pattern string) bool {
	pm := config.GetProfileManager()
	stateDir := pm.GetStatePathForProfile(d.profile)
	predDir := filepath.Join(stateDir, "predictions")

	files, err := os.ReadDir(predDir)
	if err != nil {
		return false
	}

	for _, f := range files {
		if !strings.HasSuffix(f.Name(), ".json") {
			continue
		}

		info, err := f.Info()
		if err != nil {
			continue
		}

		// Only check predictions from the last 10 minutes
		if time.Since(info.ModTime()) > 10*time.Minute {
			continue
		}

		// Read and check content
		data, err := os.ReadFile(filepath.Join(predDir, f.Name()))
		if err != nil {
			continue
		}

		var existing Prediction
		if err := json.Unmarshal(data, &existing); err == nil {
			if existing.Component == service && existing.Pattern == pattern {
				return true
			}
		}
	}

	return false
}

func (d *PredictorDaemon) captureServiceState(svc string, activeIncidentID string) LifecycleSnapshot {
	stats := metrics.GetQueryMetrics().GetStats()
	
	if svc == "frontend" || svc == "valkey-cart" {
		log.Printf("[DEBUG] Starting isolated state capture for service: %s (Profile: %s)", svc, d.profile)
	}
	snapshot := LifecycleSnapshot{
		Timestamp:  time.Now(),
		Service:    svc,
		IncidentID: activeIncidentID,
		Phase:      "steady",
		Metrics:    make(map[string]float64),
		Metadata:   make(map[string]string),
	}
	
	// Tag with actual system phase (pre-incident, pre-failure, steady, etc)
	snapshot.Phase = d.agent.ClassifySignal(snapshot)

	snapshot.Metadata["profile"] = d.profile
	snapshot.Metadata["source"] = "predictor_daemon"
	snapshot.Metadata["primary_service"] = svc

	if activeIncidentID != "" {
		snapshot.Metadata["active_incident_id"] = activeIncidentID
	}

	snapshot.Metrics["loki_query_count"] = float64(stats.LokiQueryCount)
	snapshot.Metrics["prometheus_query_cnt"] = float64(stats.PrometheusQueryCount)
	snapshot.Metrics["service_error_count"] = float64(stats.TotalErrors)
	snapshot.Metrics["service_avg_latency"] = float64(stats.AverageLatency.Milliseconds())

	// 🚀 Production Metrics Enrichment (Service Isolated)
	if d.promProvider != nil {
		label := "service"
		if d.config.PrometheusServiceLabel != "" {
			label = d.config.PrometheusServiceLabel
		}

		if d.config.RequestCountMetric != "" {
			query := fmt.Sprintf("sum(rate(%s{%s=\"%s\"}[5m]))", d.config.RequestCountMetric, label, svc)
			if val, err := d.promProvider.QueryInstant(query); err == nil {
				snapshot.Metrics["service_request_rate"] = val
			}
		}

		if d.config.LatencyMetric != "" {
			query := fmt.Sprintf("avg(%s{%s=\"%s\"})", d.config.LatencyMetric, label, svc)
			if val, err := d.promProvider.QueryInstant(query); err == nil {
				snapshot.Metrics["service_avg_latency"] = val
			}
		}
	}

	// 🚀 Metadata Enrichment
	if d.profileData != nil {
		snapshot.Metadata["profile_name"] = d.profileData.Name
		snapshot.Metadata["profile_environment"] = d.profileData.Environment
		snapshot.Metadata["profile_provider"] = d.profileData.Provider
	}
	
	snapshot.Metadata["monitored_services"] = svc

	// 🚀 Log Correlation (Isolated)
	if d.logAggregator != nil {
		now := time.Now()
		
		window := 2 * time.Minute
		d.mutex.Lock()
		if !d.bootstrapDone && d.bootstrapWindow > 0 {
			window = d.bootstrapWindow
		}
		d.mutex.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), 2000*time.Millisecond)
		defer cancel()
		if summary, err := d.logAggregator.CorrelateLogsResilient(ctx, svc, now.Add(-window), now, false); err == nil {
			if len(summary.TopErrors) > 0 || svc == "frontend" || svc == "valkey-cart" {
				log.Printf("[INFO] Service %s: Found %d error patterns in last 2m", svc, len(summary.TopErrors))
			}
			if len(summary.TopErrors) > 0 {
				snapshot.FullLogs = summary
				total := 0
				for _, e := range summary.TopErrors {
					total += e.Count
				}
				snapshot.Metrics["service_error_count"] = float64(total)
			}
		}
	}

	// 🚀 Trace Correlation (Isolated)
	if d.traceCorrelator != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if summary, err := d.traceCorrelator.Correlate(ctx, svc, time.Now().Add(-10*time.Minute), time.Now()); err == nil {
			snapshot.FullTraces = summary
		}
	}
	
	// 🚀 Log Signature Enrichment (Loki)
	if d.lokiClient != nil {
		now := time.Now()
		start := now.Add(-5 * time.Minute)
		label := "service"
		if d.config.LokiServiceLabel != "" {
			label = strings.Split(d.config.LokiServiceLabel, ",")[0]
		}
		
		query := fmt.Sprintf("{%s=\"%s\"}", label, svc)
		if d.config.LokiErrorRegex != "" {
			// LogQL compatibility: Escape backslashes and fix \d
			safeRegex := strings.ReplaceAll(d.config.LokiErrorRegex, "\\", "\\\\")
			safeRegex = strings.ReplaceAll(safeRegex, "\\\\d", "[0-9]")
			
			// Avoid double (?i) prefix
			if !strings.HasPrefix(strings.ToLower(safeRegex), "(?i)") {
				safeRegex = "(?i)" + safeRegex
			}
			query += fmt.Sprintf(" |~ \"%s\"", safeRegex)
		}

		entries, err := d.lokiClient.QueryRange(query, start, now, 10)
		if err == nil {
			seen := make(map[string]bool)
			for _, e := range entries {
				sig := e.Line
				if len(sig) > 64 { sig = sig[:64] + "..." }
				if !seen[sig] {
					snapshot.LogSignatures = append(snapshot.LogSignatures, sig)
					seen[sig] = true
				}
			}
		}
	}

	snapshot.Vector = d.generateVector(snapshot)
	return snapshot
}

func (d *PredictorDaemon) generateVector(snapshot LifecycleSnapshot) Vector {
	// Simple normalization/scaling for the ML vector
	v := make(Vector, 0)
	
	// 1. Service Request Rate (scaled 0-1000 RPS)
	rate := snapshot.Metrics["service_request_rate"]
	v = append(v, rate/1000.0)
	
	// 2. Service Error Rate (0-1 percentage)
	errRate := snapshot.Metrics["service_error_rate"]
	v = append(v, errRate)
	
	// 3. Service Latency (scaled 0-5s)
	latency := snapshot.Metrics["service_avg_latency"]
	v = append(v, latency/5000.0)
	
	return v
}

// discoverActiveServices attempts to find services that are currently "active" or "noisy"
func (d *PredictorDaemon) discoverActiveServices() []string {
	if d.lokiClient == nil {
		return nil
	}

	// Strategy: Find services that pushed logs in the last 2 minutes
	// We try common labels: "service", "container", "app"
	labelsToTry := []string{"service", "container", "app"}
	if d.config.LokiServiceLabel != "" {
		// Insert configured label at the beginning
		configured := strings.Split(d.config.LokiServiceLabel, ",")[0]
		labelsToTry = append([]string{configured}, labelsToTry...)
	}

	seenServices := make(map[string]bool)
	var services []string

	for _, label := range labelsToTry {
		if label == "" { continue }
		log.Printf("[DEBUG] Discovering services via label: %s", label)
		values, err := d.lokiClient.LabelValues(label)
		if err == nil {
			for _, val := range values {
				if !seenServices[val] {
					services = append(services, val)
					seenServices[val] = true
				}
			}
		}
		if len(services) >= 10 {
			break
		}
	}

	if len(services) == 0 {
		return []string{"orders_service", "payment_service", "inventory_service"} // Default heuristics
	}

	return services
}
