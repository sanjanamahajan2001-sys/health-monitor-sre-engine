package alert

import (
	"fmt"
	"sync/atomic"
	"time"
)

type Metrics struct {
	received         uint64
	deduped          uint64
	created          uint64
	suggested        uint64
	failed           uint64
	processSumMicros uint64
	processCount     uint64
	startedAt        int64
	alertsLoaded     int64
	flowsLoaded      int64
	configValid      int64
}

func NewMetrics() *Metrics {
	return &Metrics{startedAt: time.Now().Unix()}
}

func (m *Metrics) IncReceived() {
	atomic.AddUint64(&m.received, 1)
}

func (m *Metrics) IncDeduped() {
	atomic.AddUint64(&m.deduped, 1)
}

func (m *Metrics) IncCreated() {
	atomic.AddUint64(&m.created, 1)
}

func (m *Metrics) IncSuggested() {
	atomic.AddUint64(&m.suggested, 1)
}

func (m *Metrics) IncFailed() {
	atomic.AddUint64(&m.failed, 1)
}

func (m *Metrics) ObserveProcessing(duration time.Duration) {
	atomic.AddUint64(&m.processSumMicros, uint64(duration.Microseconds()))
	atomic.AddUint64(&m.processCount, 1)
}

func (m *Metrics) UptimeSeconds() int64 {
	started := atomic.LoadInt64(&m.startedAt)
	if started == 0 {
		return 0
	}
	return time.Now().Unix() - started
}

func (m *Metrics) Render() string {
	received := atomic.LoadUint64(&m.received)
	deduped := atomic.LoadUint64(&m.deduped)
	created := atomic.LoadUint64(&m.created)
	suggested := atomic.LoadUint64(&m.suggested)
	failed := atomic.LoadUint64(&m.failed)
	processSumMicros := atomic.LoadUint64(&m.processSumMicros)
	processCount := atomic.LoadUint64(&m.processCount)
	alertsLoaded := atomic.LoadInt64(&m.alertsLoaded)
	flowsLoaded := atomic.LoadInt64(&m.flowsLoaded)
	configValid := atomic.LoadInt64(&m.configValid)
	return fmt.Sprintf(
		"alerts_received_total %d\nalerts_deduped_total %d\nalerts_failed_total %d\nalerts_processing_seconds_sum %.6f\nalerts_processing_seconds_count %d\nincidents_created_total %d\nincidents_suggested_total %d\nlistener_uptime_seconds %d\nalerts_config_loaded %d\nflows_loaded %d\nconfig_valid %d\n",
		received,
		deduped,
		failed,
		float64(processSumMicros)/1_000_000.0,
		processCount,
		created,
		suggested,
		m.UptimeSeconds(),
		alertsLoaded,
		flowsLoaded,
		configValid,
	)
}

func (m *Metrics) Reset() {
	atomic.StoreUint64(&m.received, 0)
	atomic.StoreUint64(&m.deduped, 0)
	atomic.StoreUint64(&m.created, 0)
	atomic.StoreUint64(&m.suggested, 0)
	atomic.StoreUint64(&m.failed, 0)
	atomic.StoreUint64(&m.processSumMicros, 0)
	atomic.StoreUint64(&m.processCount, 0)
	atomic.StoreInt64(&m.startedAt, time.Now().Unix())
	atomic.StoreInt64(&m.alertsLoaded, 0)
	atomic.StoreInt64(&m.flowsLoaded, 0)
	atomic.StoreInt64(&m.configValid, 0)
}

func (m *Metrics) SetAlertsLoaded(loaded bool) {
	if loaded {
		atomic.StoreInt64(&m.alertsLoaded, 1)
	} else {
		atomic.StoreInt64(&m.alertsLoaded, 0)
	}
}

func (m *Metrics) SetFlowsLoaded(loaded bool) {
	if loaded {
		atomic.StoreInt64(&m.flowsLoaded, 1)
	} else {
		atomic.StoreInt64(&m.flowsLoaded, 0)
	}
}

func (m *Metrics) SetConfigValid(valid bool) {
	if valid {
		atomic.StoreInt64(&m.configValid, 1)
	} else {
		atomic.StoreInt64(&m.configValid, 0)
	}
}
