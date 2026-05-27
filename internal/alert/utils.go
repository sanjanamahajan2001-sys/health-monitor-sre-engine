package alert

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"health-monitor/internal/incident"
	"health-monitor/internal/metrics"
)

func newMux(processor *Processor, authCfg AuthConfig, readyCheck func() bool) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", processor.HandleWebhook)
	registerMetrics(mux, processor)
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if readyCheck != nil && !readyCheck() {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready\n"))
	})
	return authMiddleware(mux, authCfg)
}

func registerMetrics(mux *http.ServeMux, processor *Processor) {
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		// Before exposing metrics, synchronize incident gauges with the
		// current state of the incident store so that metrics reflect
		// incidents created or updated by any process (CLI or alert listener).
		syncIncidentMetrics(processor)

		// Get alert system metrics first
		alertMetrics := processor.Stats.Render()
		
		// Write alert metrics
		w.Write([]byte(alertMetrics))
		
		// Then write Prometheus metrics
		promhttp.Handler().ServeHTTP(w, r)
	})
}

// syncIncidentMetrics refreshes incident-related Prometheus gauges based on
// the persisted incidents in the shared incident store. This ensures that
// /metrics reflects manual CLI changes as well as auto-created incidents.
func syncIncidentMetrics(processor *Processor) {
	if processor == nil || processor.Service == nil {
		return
	}

	incidents, _, err := processor.Service.List()
	if err != nil {
		return
	}

	counts := make(map[string]int)
	active := 0

	for _, inc := range incidents {
		stateLabel := strings.ToLower(strings.TrimSpace(string(inc.State)))
		if stateLabel == "" {
			continue
		}
		counts[stateLabel]++

		if inc.State == incident.StateStarted || inc.State == incident.StateAcknowledged {
			active++
		}
	}

	metrics.SetIncidentsActive(active)
	metrics.SetIncidentsByStateSnapshot(counts)
}

func readPayloadFile(path string) (WebhookPayload, error) {
	var payload WebhookPayload
	data, err := os.ReadFile(path)
	if err != nil {
		return payload, err
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return payload, err
	}
	return payload, nil
}

func formatMatch(match map[string]string) string {
	if len(match) == 0 {
		return ""
	}
	keys := make([]string, 0, len(match))
	for key := range match {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", key, match[key]))
	}
	return strings.Join(parts, ",")
}
