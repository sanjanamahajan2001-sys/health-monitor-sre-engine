package metrics

import (
	"sync"
	"time"
)

// QueryMetrics tracks query performance metrics
type QueryMetrics struct {
	mu sync.RWMutex
	
	// Loki metrics
	LokiQueryCount    int64         `json:"loki_query_count"`
	LokiQueryDuration time.Duration `json:"loki_query_duration"`
	LokiErrorCount    int64         `json:"loki_error_count"`
	
	// Prometheus metrics
	PrometheusQueryCount    int64         `json:"prometheus_query_count"`
	PrometheusQueryDuration time.Duration `json:"prometheus_query_duration"`
	PrometheusErrorCount    int64         `json:"prometheus_error_count"`

	// Elastic metrics (NEW: parity with Loki)
	ElasticQueryCount    int64         `json:"elastic_query_count"`
	ElasticQueryDuration time.Duration `json:"elastic_query_duration"`
	ElasticErrorCount    int64         `json:"elastic_error_count"`
	
	// General metrics
	TotalQueries     int64         `json:"total_queries"`
	TotalErrors      int64         `json:"total_errors"`
	AverageLatency   time.Duration `json:"average_latency"`
	LastQueryTime    time.Time     `json:"last_query_time"`
}

var (
	instance *QueryMetrics
	once     sync.Once
)

// GetQueryMetrics returns the singleton instance
func GetQueryMetrics() *QueryMetrics {
	once.Do(func() {
		instance = &QueryMetrics{
			LastQueryTime: time.Now(),
		}
	})
	return instance
}

// RecordLokiQuery records a Loki query execution
func (qm *QueryMetrics) RecordLokiQuery(duration time.Duration, success bool) {
	qm.mu.Lock()
	defer qm.mu.Unlock()
	
	qm.LokiQueryCount++
	qm.TotalQueries++
	qm.LokiQueryDuration += duration
	qm.AverageLatency = qm.calculateAverageLatency()
	qm.LastQueryTime = time.Now()
	
	if !success {
		qm.LokiErrorCount++
		qm.TotalErrors++
	}
	
	// log.Printf("METRICS: Loki query duration=%v success=%v total=%d", duration, success, qm.LokiQueryCount)
}

// RecordPrometheusQuery records a Prometheus query execution
func (qm *QueryMetrics) RecordPrometheusQuery(duration time.Duration, success bool) {
	qm.mu.Lock()
	defer qm.mu.Unlock()
	
	qm.PrometheusQueryCount++
	qm.TotalQueries++
	qm.PrometheusQueryDuration += duration
	qm.AverageLatency = qm.calculateAverageLatency()
	qm.LastQueryTime = time.Now()
	
	if !success {
		qm.PrometheusErrorCount++
		qm.TotalErrors++
	}
	
	// log.Printf("METRICS: Prometheus query duration=%v success=%v total=%d", duration, success, qm.PrometheusQueryCount)
}

// RecordElasticQuery records an Elasticsearch query execution
func (qm *QueryMetrics) RecordElasticQuery(duration time.Duration, success bool) {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	qm.ElasticQueryCount++
	qm.TotalQueries++
	qm.ElasticQueryDuration += duration
	qm.AverageLatency = qm.calculateAverageLatency()
	qm.LastQueryTime = time.Now()

	if !success {
		qm.ElasticErrorCount++
		qm.TotalErrors++
	}
}

// calculateAverageLatency computes the average query latency
func (qm *QueryMetrics) calculateAverageLatency() time.Duration {
	if qm.TotalQueries == 0 {
		return 0
	}
	totalDuration := qm.LokiQueryDuration + qm.PrometheusQueryDuration + qm.ElasticQueryDuration
	return time.Duration(int64(totalDuration) / qm.TotalQueries)
}

// GetStats returns a copy of current metrics
func (qm *QueryMetrics) GetStats() QueryMetrics {
	qm.mu.RLock()
	defer qm.mu.RUnlock()
	return *qm
}

// Reset resets all metrics
func (qm *QueryMetrics) Reset() {
	qm.mu.Lock()
	defer qm.mu.Unlock()
	
	*qm = QueryMetrics{
		LastQueryTime: time.Now(),
	}
}
