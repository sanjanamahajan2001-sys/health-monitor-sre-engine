package metrics

import (
	"health-monitor/internal/analyse/prometheus"
	"time"
)

// PrometheusProvider fetches metrics from Prometheus
type PrometheusProvider struct {
	client *prometheus.Client
}

// NewPrometheusProvider creates a new Prometheus provider
func NewPrometheusProvider(url, token, user, pass string) *PrometheusProvider {
	return &PrometheusProvider{
		client: &prometheus.Client{
			BaseURL: url,
			Token:   token,
			User:    user,
			Pass:    pass,
			Timeout: 10 * time.Second,
		},
	}
}

// QueryInstant implements the MetricProvider interface
func (p *PrometheusProvider) QueryInstant(query string) (float64, error) {
	return p.client.QueryInstant(query)
}

// QueryVector implements the MetricProvider interface
func (p *PrometheusProvider) QueryVector(query string) ([]MetricSample, error) {
	// Map prometheus results to MetricSample
	// This would need a QueryVector method in prometheus.Client
	// For now, return empty or implement basic mapping if needed
	return []MetricSample{}, nil
}

// Check implements the MetricProvider interface
func (p *PrometheusProvider) Check() (string, error) {
	return p.client.Check()
}
