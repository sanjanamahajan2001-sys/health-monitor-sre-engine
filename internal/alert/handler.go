package alert

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"health-monitor/internal/incident"
	_ "health-monitor/internal/metrics"
)

type Processor struct {
	Service    *incident.Service
	Cache      *FingerprintCache
	Now        func() time.Time
	History    *HistoryWriter
	Stats      *Metrics
	MaxWorkers int
	Rate       *RateLimiter
	DedupInterval time.Duration
	rulesMu    sync.RWMutex
	rules      []Rule
}

type Decision struct {
	Message string
}

func NewProcessor(service *incident.Service, rules []Rule, dedupInterval time.Duration) *Processor {
	if dedupInterval <= 0 {
		dedupInterval = 2 * time.Minute
	}
	return &Processor{
		Service:       service,
		Cache:         NewFingerprintCache(dedupInterval),
		DedupInterval: dedupInterval,
		Now:           time.Now,
		History:       NewHistoryWriter(),
		Stats:         NewMetrics(),
		MaxWorkers:    4,
		Rate:          nil,
		rules:         append([]Rule(nil), rules...),
	}
}

func (p *Processor) ResetCache() {
	p.Cache = NewFingerprintCache(p.DedupInterval)
}

func (p *Processor) ResetMetrics() {
	if p.Stats != nil {
		p.Stats.Reset()
	}
}

func (p *Processor) SetRules(rules []Rule) {
	p.rulesMu.Lock()
	defer p.rulesMu.Unlock()
	p.rules = append([]Rule(nil), rules...)
}

func (p *Processor) rulesSnapshot() []Rule {
	p.rulesMu.RLock()
	defer p.rulesMu.RUnlock()
	return append([]Rule(nil), p.rules...)
}

func (p *Processor) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if len(p.rulesSnapshot()) == 0 {
		http.Error(w, "alerts disabled", http.StatusServiceUnavailable)
		return
	}
	if p.Rate != nil && !p.Rate.Allow() {
		logWarn("rate limit exceeded", map[string]string{"path": r.URL.Path})
		http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
		return
	}
	defer r.Body.Close()
	limited := http.MaxBytesReader(w, r.Body, 1<<20)
	body, err := io.ReadAll(limited)
	if err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	var payload WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	_, warnings := p.Process(payload, false)
	for _, warning := range warnings {
		logWarn(warning, map[string]string{
			"alertname": r.Header.Get("X-Alertname"),
			"path":      r.URL.Path,
		})
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (p *Processor) Process(payload WebhookPayload, dryRun bool) ([]Decision, []string) {
	start := time.Now()
	defer func() {
		if p.Stats != nil {
			p.Stats.ObserveProcessing(time.Since(start))
		}
	}()
	var decisions []Decision
	var warnings []string
	alerts := payload.Alerts
	if len(alerts) == 0 {
		return decisions, warnings
	}
	workers := p.MaxWorkers
	if workers <= 0 {
		workers = 1
	}
	if workers <= 1 || len(alerts) == 1 {
		for _, alert := range alerts {
			msgs, warn := p.processAlert(alert, dryRun)
			decisions = append(decisions, msgs...)
			warnings = append(warnings, warn...)
		}
		return decisions, warnings
	}
	if len(alerts) > workers*10 {
		logWarn("large alert batch", map[string]string{"count": fmt.Sprintf("%d", len(alerts))})
	}
	type result struct {
		decisions []Decision
		warnings  []string
	}
	jobs := make(chan Alert)
	results := make(chan result)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for alert := range jobs {
				msgs, warn := p.processAlert(alert, dryRun)
				results <- result{decisions: msgs, warnings: warn}
			}
		}()
	}
	go func() {
		for _, alert := range alerts {
			jobs <- alert
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()
	for res := range results {
		decisions = append(decisions, res.decisions...)
		warnings = append(warnings, res.warnings...)
	}
	return decisions, warnings
}

func (p *Processor) processAlert(alert Alert, dryRun bool) ([]Decision, []string) {
	var decisions []Decision
	var warnings []string
	if p.Stats != nil {
		p.Stats.IncReceived()
	}
	alertLabels := normalizeLabels(alert.Labels)
	alertName := alertLabels["alertname"]
	if alertName == "" {
		warnings = append(warnings, "alert missing alertname label")
		return decisions, warnings
	}
	key := fingerprintKey(alert, alertLabels)
	if key != "" && p.Cache != nil && p.Cache.Seen(key, p.Now()) {
		if p.Stats != nil {
			p.Stats.IncDeduped()
		}
		warnings = append(warnings, fmt.Sprintf("duplicate alert skipped: %s", alertName))
		return decisions, warnings
	}
	rule := matchRule(p.rulesSnapshot(), alertLabels)
	if rule == nil {
		log.Printf("DEBUG: No mapping rule found for alert %s (labels: %v)", alertName, alertLabels)
		// FALLBACK: For SLO sources, create a default incident even if no mapping rule exists.
		if alertLabels["source"] == "slo" {
			log.Printf("INFO: Applying fallback for SLO alert %s", alertName)
			fallbackService := alertLabels["service"]
			fallbackSeverity := incident.P2
			fallbackTitle := alertLabels["alertname"]
			if resTitle := resolveTitle(Rule{}, alert); resTitle != "" {
				fallbackTitle = resTitle
			}
			
			if dryRun {
				decisions = append(decisions, Decision{Message: fmt.Sprintf("Would create: Default SLO incident for %s", fallbackService)})
				return decisions, warnings
			}
			
			msgs, warn := p.applyAction(fallbackService, fallbackSeverity, ModeAuto, fallbackTitle, alertName, alert)
			return msgs, warn
		}
		
		warnings = append(warnings, fmt.Sprintf("no mapping rule found for alert %s", alertName))
		return decisions, warnings
	}
	log.Printf("DEBUG: Matched rule %s for alert %s", rule.Source, alertName)
	service := rule.Match["service"]
	title := resolveTitle(*rule, alert)
	if strings.TrimSpace(title) == "" {
		title = alertName
	}
	if dryRun {
		msgs, warn := p.simulateAction(service, rule.Severity, rule.Mode, title, alertName, alert)
		return msgs, warn
	}
	msgs, warn := p.applyAction(service, rule.Severity, rule.Mode, title, alertName, alert)
	return msgs, warn
}

func (p *Processor) simulateAction(service string, severity incident.Severity, mode Mode, title string, alertName string, alert Alert) ([]Decision, []string) {
	return p.apply(service, severity, mode, title, alertName, alert, true)
}

func (p *Processor) applyAction(service string, severity incident.Severity, mode Mode, title string, alertName string, alert Alert) ([]Decision, []string) {
	return p.apply(service, severity, mode, title, alertName, alert, false)
}

func (p *Processor) apply(service string, severity incident.Severity, mode Mode, title string, alertName string, alert Alert, dryRun bool) ([]Decision, []string) {
	var decisions []Decision
	var warnings []string
	active, activeWarnings, err := p.Service.FindActiveByServiceFlow(service)
	if err != nil {
		warnings = append(warnings, "failed to load active incidents: "+err.Error())
		if p.Stats != nil {
			p.Stats.IncFailed()
		}
	}
	warnings = append(warnings, activeWarnings...)
	warnings = append(warnings, activeWarnings...)
	if alert.Status == "resolved" {
		if active == nil {
			msg := fmt.Sprintf("Received resolved signal for %s but no active incident found", service)
			decisions = append(decisions, Decision{Message: msg})
			return decisions, warnings
		}
		if dryRun {
			msg := fmt.Sprintf("Would add recovery note to incident %s for resolved alert %s", active.ID, alertName)
			decisions = append(decisions, Decision{Message: msg})
			return decisions, warnings
		}
		resolveSummary := fmt.Sprintf("✅ Alert Resolved: %s", alertName)
		if summary, ok := alert.Annotations["summary"]; ok && summary != "" {
			resolveSummary += "\nSummary: " + summary
		}
		
		// 🚀 NEW: Transition to Resolved state to trigger lifecycle persistence
		if _, _, err := p.Service.ResolveByID(active.ID, resolveSummary, "system", nil); err != nil {
			warnings = append(warnings, "automated resolution failed: "+err.Error())
		}
		
		msg := fmt.Sprintf("Transitioned incident %s to Resolved for alert %s", active.ID, alertName)
		decisions = append(decisions, Decision{Message: msg})
		return decisions, warnings
	}

	if active != nil {
		current := *active
		// If an active incident exists, we ALWAYS favor attaching to it to avoid spam.
		escalated := false
		if incident.IsHigherSeverity(severity, current.Severity) {
			if dryRun {
				decisions = append(decisions, Decision{Message: fmt.Sprintf("Would escalate severity to %s on %s", severity, current.ID)})
			} else {
				if _, _, err := p.Service.UpdateSeverityByID(current.ID, severity, "", "Severity escalated to "+string(severity)); err != nil {
					warnings = append(warnings, "severity update failed: "+err.Error())
				} else {
					escalated = true
				}
			}
		}

		if dryRun {
			decisions = append(decisions, Decision{Message: fmt.Sprintf("Would add note to active incident %s: New alert received: %s", current.ID, alertName)})
			return decisions, warnings
		}

		p.Stats.IncDeduped()
		noteMsg := "New alert received: " + alertName
		if summary, ok := alert.Annotations["summary"]; ok && summary != "" {
			noteMsg += "\nSummary: " + summary
		}
		if description, ok := alert.Annotations["description"]; ok && description != "" {
			noteMsg += "\nDescription: " + description
		}

		// Update metadata with alert labels
		if current.Metadata == nil {
			current.Metadata = make(map[string]string)
		}
		for k, v := range alert.Labels {
			current.Metadata["alert_label_"+k] = v
			if k == "prom_service_label" || k == "loki_service_label" || k == "trace_service_tag" {
				current.Metadata[k] = v
			}
		}

		if _, _, err := p.Service.NoteByID(current.ID, noteMsg, ""); err != nil {
			warnings = append(warnings, "note failed: "+err.Error())
			if p.Stats != nil {
				p.Stats.IncFailed()
			}
		}

		alertData := map[string]interface{}{
			"when":       p.Now(),
			"alert_name": alertName,
			"service":    service,
			"severity":   string(severity),
			"action":     "deduped_active",
			"mode":       mode,
			"title":      title,
		}
		if err := p.History.Write(alertData); err != nil {
			warnings = append(warnings, "history log failed: "+err.Error())
		}

		msg := fmt.Sprintf("Attached alert %s to active incident %s", alertName, current.ID)
		if escalated {
			msg += fmt.Sprintf(" (Escalated to %s)", severity)
		}
		decisions = append(decisions, Decision{Message: msg})
		return decisions, warnings
	}
	suggested, suggestWarnings, err := p.Service.FindSuggestedByServiceFlow(service)
	if err != nil {
		warnings = append(warnings, "failed to check suggested incidents: "+err.Error())
		if p.Stats != nil {
			p.Stats.IncFailed()
		}
	}
	warnings = append(warnings, suggestWarnings...)
	
	// Freshness check: If the suggested incident is older than 4 hours, ignore it and create a new one.
	if suggested != nil {
		if p.Now().Sub(suggested.CreatedAt) > 4*time.Hour {
			log.Printf("INFO: Ignoring stale suggested incident %s (created %v) for deduplication", 
				suggested.ID, suggested.CreatedAt.Format(time.RFC3339))
			suggested = nil
		}
	}

	if suggested != nil {
		escalated := false
		if incident.IsHigherSeverity(severity, suggested.Severity) {
			if dryRun {
				decisions = append(decisions, Decision{Message: fmt.Sprintf("Would escalate suggested %s to severity %s", suggested.ID, severity)})
			} else {
				if _, _, err := p.Service.UpdateSeverityByID(suggested.ID, severity, "", "Severity escalated to "+string(severity)); err != nil {
					warnings = append(warnings, "suggested severity update failed: "+err.Error())
				} else {
					escalated = true
				}
			}
		}

		if dryRun {
			decisions = append(decisions, Decision{Message: fmt.Sprintf("Would add note to suggested %s: New alert received: %s", suggested.ID, alertName)})
			return decisions, warnings
		}

		p.Stats.IncDeduped()
		noteMsg := "New alert received: " + alertName
		if summary, ok := alert.Annotations["summary"]; ok && summary != "" {
			noteMsg += "\nSummary: " + summary
		}
		if description, ok := alert.Annotations["description"]; ok && description != "" {
			noteMsg += "\nDescription: " + description
		}

		if _, _, err := p.Service.NoteByID(suggested.ID, noteMsg, ""); err != nil {
			warnings = append(warnings, "note failed: "+err.Error())
			if p.Stats != nil {
				p.Stats.IncFailed()
			}
		}

		alertData := map[string]interface{}{
			"when":       p.Now(),
			"alert_name": alertName,
			"service":    service,
			"severity":   string(severity),
			"action":     "noted_suggested",
			"mode":       mode,
			"title":      title,
		}
		if err := p.History.Write(alertData); err != nil {
			warnings = append(warnings, "history log failed: "+err.Error())
		}

		msg := fmt.Sprintf("Attached alert %s to suggested incident %s", alertName, suggested.ID)
		if escalated {
			msg += fmt.Sprintf(" (Escalated to %s)", severity)
		}
		decisions = append(decisions, Decision{Message: msg})
		return decisions, warnings
	}
	if mode == ModeAuto {
		if dryRun {
			decisions = append(decisions, Decision{Message: fmt.Sprintf("Would create: Auto incident for %s %s %s", service, severity, title)})
			return decisions, warnings
		}
		// Try to create auto incident, but if there's an active incident, create suggested instead
		var newIncident incident.Incident
		metadata := make(map[string]string)
		for k, v := range alert.Labels {
			metadata["alert_label_"+k] = v
			// Also support direct overrides if the alert contains them
			if k == "prom_service_label" || k == "loki_service_label" || k == "trace_service_tag" {
				metadata[k] = v
			}
		}
		if source, ok := alert.Labels["source"]; ok {
			metadata["source"] = source
		}
		newIncident, _, err = p.Service.StartForServiceFlowWithMetadata(service, severity, title, "", metadata)
		if err != nil && strings.Contains(err.Error(), "an active incident already exists") {
			// Active incident exists - create suggested incident instead
			if dryRun {
				decisions = append(decisions, Decision{Message: fmt.Sprintf("Would create: Suggested incident for %s %s %s (active incident exists)", service, severity, title)})
				return decisions, warnings
			}
			var incident incident.Incident
			if source, ok := alert.Labels["source"]; ok {
				metadata["source"] = source
			}
			incident, _, err = p.Service.SuggestWithMetadata(service, severity, title, "", metadata)
			if err != nil {
				warnings = append(warnings, "incident suggest failed: "+err.Error())
				if p.Stats != nil {
					p.Stats.IncFailed()
				}
				return decisions, warnings
			}
			if _, _, err := p.Service.NoteByID(incident.ID, "Created automatically from alert (active incident exists)", ""); err != nil {
				warnings = append(warnings, "note failed: "+err.Error())
				if p.Stats != nil {
					p.Stats.IncFailed()
				}
			}
			// Add webhook annotations as additional notes
			if len(alert.Annotations) > 0 {
				if summary, ok := alert.Annotations["summary"]; ok && summary != "" {
					if _, _, err := p.Service.NoteByID(incident.ID, fmt.Sprintf("Summary: %s", summary), ""); err != nil {
						warnings = append(warnings, "summary note failed: "+err.Error())
					}
				}
				if description, ok := alert.Annotations["description"]; ok && description != "" {
					if _, _, err := p.Service.NoteByID(incident.ID, fmt.Sprintf("Description: %s", description), ""); err != nil {
						warnings = append(warnings, "description note failed: "+err.Error())
					}
				}
			}
			p.Stats.IncSuggested()
			alertData := map[string]interface{}{
				"when":       p.Now(),
				"alert_name": alertName,
				"service":    service,
				"severity":   string(severity),
				"action":     "created_suggested",
				"mode":       mode,
				"title":      title,
			}
			if err := p.History.Write(alertData); err != nil {
				warnings = append(warnings, "history log failed: "+err.Error())
			}
			decisions = append(decisions, Decision{Message: fmt.Sprintf("Created suggested incident for %s (active incident exists)", service)})
			return decisions, warnings
		} else if err != nil {
			// Other error occurred
			warnings = append(warnings, "incident start failed: "+err.Error())
			if p.Stats != nil {
				p.Stats.IncFailed()
			}
			return decisions, warnings
		}
		// Successfully created auto incident
		log.Printf("INFO: Successfully created auto incident for %s", service)
		if _, _, err := p.Service.NoteByID(newIncident.ID, "Created automatically from alert", ""); err != nil {
			warnings = append(warnings, "note failed: "+err.Error())
			if p.Stats != nil {
				p.Stats.IncFailed()
			}
		}
		p.Stats.IncCreated()
		alertData := map[string]interface{}{
			"when":       p.Now(),
			"alert_name": alertName,
			"service":    service,
			"severity":   string(severity),
			"action":     "created",
			"mode":       mode,
			"title":      title,
		}
		if err := p.History.Write(alertData); err != nil {
			warnings = append(warnings, "history log failed: "+err.Error())
		}
		decisions = append(decisions, Decision{Message: fmt.Sprintf("Created auto incident for %s", service)})
		return decisions, warnings
	}
	if dryRun {
		decisions = append(decisions, Decision{Message: fmt.Sprintf("Would create: Suggested incident for %s %s %s", service, severity, title)})
		return decisions, warnings
	}
	metadata := make(map[string]string)
	for k, v := range alert.Labels {
		metadata["alert_label_"+k] = v
		if k == "prom_service_label" || k == "loki_service_label" || k == "trace_service_tag" {
			metadata[k] = v
		}
	}
	incident, _, err := p.Service.SuggestWithMetadata(service, severity, title, "", metadata)
	if err != nil {
		warnings = append(warnings, "incident suggest failed: "+err.Error())
		if p.Stats != nil {
			p.Stats.IncFailed()
		}
		return decisions, warnings
	}
	if _, _, err := p.Service.NoteByID(incident.ID, "Created automatically from alert", ""); err != nil {
		warnings = append(warnings, "note failed: "+err.Error())
		if p.Stats != nil {
			p.Stats.IncFailed()
		}
	}
	// Add webhook annotations as additional notes
	if len(alert.Annotations) > 0 {
		if summary, ok := alert.Annotations["summary"]; ok && summary != "" {
			if _, _, err := p.Service.NoteByID(incident.ID, fmt.Sprintf("Summary: %s", summary), ""); err != nil {
				warnings = append(warnings, "summary note failed: "+err.Error())
			}
		}
		if description, ok := alert.Annotations["description"]; ok && description != "" {
			if _, _, err := p.Service.NoteByID(incident.ID, fmt.Sprintf("Description: %s", description), ""); err != nil {
				warnings = append(warnings, "description note failed: "+err.Error())
			}
		}
	}
	p.Stats.IncSuggested()
	alertData := map[string]interface{}{
		"when":       p.Now(),
		"alert_name": alertName,
		"service":    service,
		"severity":   string(severity),
		"action":     "suggested",
		"mode":       mode,
		"title":      title,
	}
	if err := p.History.Write(alertData); err != nil {
		warnings = append(warnings, "history log failed: "+err.Error())
	}
	decisions = append(decisions, Decision{Message: fmt.Sprintf("Created suggested incident for %s", service)})
	return decisions, warnings
}

func matchRule(rules []Rule, labels map[string]string) *Rule {
	for i := range rules {
		rule := &rules[i]
		if ruleMatches(*rule, labels) {
			return rule
		}
	}
	return nil
}

func ruleMatches(rule Rule, labels map[string]string) bool {
	for key, value := range rule.Match {
		if labels[key] != value {
			return false
		}
	}
	return true
}

func resolveTitle(rule Rule, alert Alert) string {
	if strings.TrimSpace(rule.Title) != "" {
		return strings.TrimSpace(rule.Title)
	}
	if summary := strings.TrimSpace(alert.Annotations["summary"]); summary != "" {
		return summary
	}
	return strings.TrimSpace(alert.Labels["alertname"])
}

func normalizeLabels(labels map[string]string) map[string]string {
	out := make(map[string]string, len(labels))
	for key, value := range labels {
		trimmedKey := strings.ToLower(strings.TrimSpace(key))
		if trimmedKey == "" {
			continue
		}
		out[trimmedKey] = strings.TrimSpace(value)
	}
	return out
}

func fingerprintKey(alert Alert, labels map[string]string) string {
	if strings.TrimSpace(alert.Fingerprint) != "" {
		return strings.TrimSpace(alert.Fingerprint)
	}
	alertName := labels["alertname"]
	service := labels["service"]
	if alertName == "" || service == "" {
		return ""
	}
	return hashLabels(alertName, service, labels)
}

func hashLabels(alertName string, service string, labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(alertName)
	b.WriteString("|")
	b.WriteString(service)
	for _, key := range keys {
		b.WriteString("|")
		b.WriteString(key)
		b.WriteString("=")
		b.WriteString(labels[key])
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

type FingerprintCache struct {
	ttl     time.Duration
	mu      sync.Mutex
	entries map[string]time.Time
}

func NewFingerprintCache(ttl time.Duration) *FingerprintCache {
	return &FingerprintCache{
		ttl:     ttl,
		entries: make(map[string]time.Time),
	}
}

func (c *FingerprintCache) Seen(key string, now time.Time) bool {
	if c == nil || key == "" {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.prune(now)
	if ts, ok := c.entries[key]; ok {
		if now.Sub(ts) <= c.ttl {
			return true
		}
	}
	c.entries[key] = now
	return false
}

func (c *FingerprintCache) prune(now time.Time) {
	if c.ttl <= 0 {
		return
	}
	for key, ts := range c.entries {
		if now.Sub(ts) > c.ttl {
			delete(c.entries, key)
		}
	}
}
