package metrics

import (
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	// Gauges
	incidentsActive = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "health_monitor_incidents_active",
		Help: "Number of currently active incidents (Started + Acknowledged)",
	})

	incidentsByState = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "health_monitor_incidents_by_state",
		Help: "Number of incidents by state",
	}, []string{"state"})

	// Counters
	incidentsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "health_monitor_incidents_total",
		Help: "Total number of incidents by state transition",
	}, []string{"state"})

	// Histogram
	incidentDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "health_monitor_incident_duration_seconds",
		Help:    "Duration of incidents from creation to resolution",
		Buckets: []float64{30, 60, 120, 300, 600, 1800, 3600, 7200, 14400},
	})
)

func init() {
	// Register all metrics once during startup
	prometheus.MustRegister(incidentsActive)
	prometheus.MustRegister(incidentsByState)
	prometheus.MustRegister(incidentsTotal)
	prometheus.MustRegister(incidentDuration)
}

// IncidentMetrics tracks incident metrics in a thread-safe manner
type IncidentMetrics struct {
	mu sync.RWMutex
}

// NewIncidentMetrics creates a new incident metrics tracker
func NewIncidentMetrics() *IncidentMetrics {
	return &IncidentMetrics{}
}

// RecordIncidentStarted records when an incident is started
func (m *IncidentMetrics) RecordIncidentStarted() {
	incidentsActive.Inc()
	incidentsTotal.WithLabelValues("started").Inc()
	incidentsByState.WithLabelValues("started").Inc()
}

// RecordIncidentAcknowledged records when an incident is acknowledged
func (m *IncidentMetrics) RecordIncidentAcknowledged() {
	// State transition: started -> acknowledged
	incidentsByState.WithLabelValues("started").Dec()
	incidentsByState.WithLabelValues("acknowledged").Inc()
	incidentsTotal.WithLabelValues("acknowledged").Inc()
}

// RecordIncidentSuggested records when an incident is suggested
func (m *IncidentMetrics) RecordIncidentSuggested() {
	incidentsTotal.WithLabelValues("suggested").Inc()
	incidentsByState.WithLabelValues("suggested").Inc()
}

// RecordIncidentStartedFromSuggested records when a suggested incident is started
func (m *IncidentMetrics) RecordIncidentStartedFromSuggested() {
	// State transition: suggested -> started
	incidentsByState.WithLabelValues("suggested").Dec()
	incidentsByState.WithLabelValues("started").Inc()
	incidentsTotal.WithLabelValues("started").Inc()
	incidentsActive.Inc()
}

// RecordIncidentResolved records when an incident is resolved
func (m *IncidentMetrics) RecordIncidentResolved(createdAt time.Time) {
	// Record duration before state changes. This is best-effort and only
	// applies within the current process; other processes should prefer
	// to compute gauges from the incident store.
	duration := time.Since(createdAt).Seconds()
	incidentDuration.Observe(duration)
}

// normalizeState converts various state representations to a canonical,
// lowercase label used in Prometheus metrics (e.g. "Started" -> "started").
func normalizeState(state string) string {
	if state == "" {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(state))
}

// RecordStateTransition handles state changes properly
func (m *IncidentMetrics) RecordStateTransition(oldState, newState string) {
	old := normalizeState(oldState)
	new := normalizeState(newState)

	if old != "" {
		incidentsByState.WithLabelValues(old).Dec()
	}
	if new != "" {
		incidentsByState.WithLabelValues(new).Inc()
		incidentsTotal.WithLabelValues(new).Inc()
	}

	// Handle active count changes (Started + Acknowledged considered active).
	if (old == "started" || old == "acknowledged") && (new != "started" && new != "acknowledged") {
		incidentsActive.Dec()
	}
	if (old != "started" && old != "acknowledged") && (new == "started" || new == "acknowledged") {
		incidentsActive.Inc()
	}
}

// SetIncidentsActive allows a caller (typically a long-running process like
// the alert listener) to atomically override the active incident gauge based
// on a snapshot of the incident store.
func SetIncidentsActive(count int) {
	if count < 0 {
		count = 0
	}
	incidentsActive.Set(float64(count))
}

// SetIncidentsByStateSnapshot replaces the incidents_by_state gauge with the
// provided snapshot. This is useful for processes that periodically scan the
// incident store and want metrics to reflect persisted state across multiple
// processes (e.g. CLI + alert listener).
func SetIncidentsByStateSnapshot(counts map[string]int) {
	incidentsByState.Reset()
	for state, count := range counts {
		if state == "" {
			continue
		}
		if count < 0 {
			count = 0
		}
		incidentsByState.WithLabelValues(state).Set(float64(count))
	}
}
