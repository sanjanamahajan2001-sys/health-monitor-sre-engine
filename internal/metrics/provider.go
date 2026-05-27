package metrics

import (
	"fmt"
	"health-monitor/internal/config"
)

var ErrNoData = fmt.Errorf("no data")

// MetricSample represents a single metric data point with labels
type MetricSample struct {
	Labels map[string]string
	Value  float64
}

// MetricProvider defines the interface for fetching metrics from different backends
type MetricProvider interface {
	// QueryInstant performs an instant query and returns a single value
	QueryInstant(query string) (float64, error)
	
	// QueryVector performs a vector query and returns multiple samples
	QueryVector(query string) ([]MetricSample, error)
	
	// Check verifies the health of the metric provider
	Check() (string, error)
}

// NewMetricProvider returns the appropriate MetricProvider based on configuration
func NewMetricProvider(cfg *config.Config) (MetricProvider, error) {
	// If EKS is enabled and CloudWatch is preferred/fallback
	if cfg.EKS.Enabled && cfg.EKS.CloudWatchEnabled {
		return NewCloudWatchProvider(cfg.EKS.Region)
	}

	// Default to Prometheus if URL is provided
	if cfg.PrometheusURL != "" {
		return NewPrometheusProvider(cfg.PrometheusURL, cfg.PrometheusToken, cfg.PrometheusUser, cfg.PrometheusPass), nil
	}

	return nil, fmt.Errorf("no metric provider configured (Prometheus URL or EKS/CloudWatch missing)")
}
