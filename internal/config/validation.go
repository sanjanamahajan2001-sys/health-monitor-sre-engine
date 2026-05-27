package config

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"health-monitor/internal/analyse/loki"
	"health-monitor/internal/analyse/prometheus"
	"health-monitor/internal/analyse/tracing"
)

// ValidateConfigUnsafe returns warnings for configuration issues (doesn't print)
func ValidateConfigUnsafe(cfg Config) []string {
	warnings := make([]string, 0)
	
	// Validate Grafana URL
	if cfg.GrafanaURL != "" {
		if !isValidURL(cfg.GrafanaURL) {
			warnings = append(warnings, "grafana_url format is invalid, grafana links disabled")
		} else {
			// Check for required datasources
			if cfg.GrafanaPromDataSource == "" {
				warnings = append(warnings, "grafana_prom_ds not configured, grafana links disabled")
			}
			if cfg.GrafanaLokiDataSource == "" && cfg.GrafanaTraceDataSource == "" {
				warnings = append(warnings, "no grafana datasources configured, grafana links may be limited")
			}
		}
	} else {
		warnings = append(warnings, "grafana_url not configured, grafana links disabled")
	}
	
	// Validate Prometheus URL
	if cfg.PrometheusURL != "" {
		if !isValidURL(cfg.PrometheusURL) {
			warnings = append(warnings, "prometheus_url format is invalid, prometheus links disabled")
		}
	} else {
		warnings = append(warnings, "prometheus_url not configured, prometheus links disabled")
	}
	
	// Validate Loki URL
	if cfg.LokiURL != "" {
		if !isValidURL(cfg.LokiURL) {
			warnings = append(warnings, "loki_url format is invalid, loki links disabled")
		}
	} else if cfg.GrafanaURL != "" && cfg.GrafanaLokiDataSource == "" {
		warnings = append(warnings, "loki_url not configured and grafana_loki_ds missing, log links disabled")
	}
	
	// Validate Trace URL
	if cfg.TraceURL != "" {
		if !isValidURL(cfg.TraceURL) {
			warnings = append(warnings, "trace_url format is invalid, trace links disabled")
		}
	} else if cfg.GrafanaURL != "" && cfg.GrafanaTraceDataSource == "" {
		warnings = append(warnings, "trace_url not configured and grafana_trace_ds missing, trace links disabled")
	}
	
	// Validate Kibana URL
	if cfg.KibanaURL != "" {
		if !isValidURL(cfg.KibanaURL) {
			warnings = append(warnings, "kibana_url format is invalid, kibana links disabled")
		}
	}
	
	return warnings
}

// ValidateConfig checks configuration and prints warnings for missing backends
func ValidateConfig(cfg Config) {
	warnings := make([]string, 0)
	
	// Validate Grafana URL
	if cfg.GrafanaURL != "" {
		if !isValidURL(cfg.GrafanaURL) {
			warnings = append(warnings, "grafana_url format is invalid, grafana links disabled")
		} else {
			// Check for required datasources
			if cfg.GrafanaPromDataSource == "" {
				warnings = append(warnings, "grafana_prom_ds not configured, grafana links disabled")
			}
			if cfg.GrafanaLokiDataSource == "" && cfg.GrafanaTraceDataSource == "" {
				warnings = append(warnings, "no grafana datasources configured, grafana links may be limited")
			}
		}
	} else {
		warnings = append(warnings, "grafana_url not configured, grafana links disabled")
	}
	
	// Validate Prometheus URL
	if cfg.PrometheusURL != "" {
		if !isValidURL(cfg.PrometheusURL) {
			warnings = append(warnings, "prometheus_url format is invalid, prometheus links disabled")
		}
	} else {
		warnings = append(warnings, "prometheus_url not configured, prometheus links disabled")
	}
	
	// Validate Loki URL
	if cfg.LokiURL != "" {
		if !isValidURL(cfg.LokiURL) {
			warnings = append(warnings, "loki_url format is invalid, loki links disabled")
		}
	} else if cfg.GrafanaURL != "" && cfg.GrafanaLokiDataSource == "" {
		warnings = append(warnings, "loki_url not configured and grafana_loki_ds missing, log links disabled")
	}
	
	// Validate Trace URL
	if cfg.TraceURL != "" {
		if !isValidURL(cfg.TraceURL) {
			warnings = append(warnings, "trace_url format is invalid, trace links disabled")
		}
	} else if cfg.GrafanaURL != "" && cfg.GrafanaTraceDataSource == "" {
		warnings = append(warnings, "trace_url not configured and grafana_trace_ds missing, trace links disabled")
	}
	
	// Validate Kibana URL
	if cfg.KibanaURL != "" {
		if !isValidURL(cfg.KibanaURL) {
			warnings = append(warnings, "kibana_url format is invalid, kibana links disabled")
		}
	}
	
	// Print all warnings
	for _, warning := range warnings {
		fmt.Fprintf(os.Stderr, "WARN: %s\n", warning)
	}
}

// isValidURL checks if a string is a valid URL
func isValidURL(rawURL string) bool {
	if strings.TrimSpace(rawURL) == "" {
		return false
	}
	
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	
	scheme := strings.ToLower(parsedURL.Scheme)
	return scheme == "http" || scheme == "https"
}

// TestBackendConnectivity attempts to connect to configured backends and run test queries
func TestBackendConnectivity(cfg Config) []string {
	results := make([]string, 0)

	// 1. Prometheus
	if cfg.PrometheusURL != "" {
		client := prometheus.Client{
			BaseURL: cfg.PrometheusURL,
			Token:   cfg.PrometheusToken,
			User:    cfg.PrometheusUser,
			Pass:    cfg.PrometheusPass,
			Timeout: 5 * time.Second,
		}
		// Attempt a simple query that should always work if permissions are OK
		_, err := client.QueryInstant("count(up)")
		if err != nil {
			results = append(results, fmt.Sprintf("Prometheus (%s): ❌ %v", cfg.PrometheusURL, err))
		} else {
			results = append(results, fmt.Sprintf("Prometheus (%s): ✅ Connected and queryable", cfg.PrometheusURL))
		}
	}

	// 2. Loki
	if cfg.LokiURL != "" {
		client := loki.Client{
			BaseURL: cfg.LokiURL,
			Token:   cfg.LokiToken,
			User:    cfg.LokiUser,
			Pass:    cfg.LokiPass,
			Timeout: 5 * time.Second,
		}
		// Attempt to list labels
		_, err := client.Labels()
		if err != nil {
			results = append(results, fmt.Sprintf("Loki (%s): ❌ %v", cfg.LokiURL, err))
		} else {
			results = append(results, fmt.Sprintf("Loki (%s): ✅ Connected and queryable", cfg.LokiURL))
		}
	}

	// 3. Tracing (Tempo/Jaeger)
	if cfg.TraceURL != "" {
		client := tracing.Client{
			BaseURL: cfg.TraceURL,
			Token:   cfg.TraceToken,
			User:    cfg.TraceUser,
			Pass:    cfg.TracePass,
			Timeout: 5 * time.Second,
		}
		var err error
		backend := cfg.TraceBackend
		if backend == "" {
			backend = "Tempo"
		}
		
		if strings.ToLower(backend) == "jaeger" {
			_, err = client.JaegerHealth()
		} else {
			_, err = client.TempoHealth()
		}

		if err != nil {
			results = append(results, fmt.Sprintf("Trace %s (%s): ❌ %v", backend, cfg.TraceURL, err))
		} else {
			results = append(results, fmt.Sprintf("Trace %s (%s): ✅ Connected and healthy", backend, cfg.TraceURL))
		}
	}

	// 4. Elasticsearch
	if cfg.ElasticURL != "" {
		// Basic connectivity check: endpoint + /_cluster/health or just GET endpoint
		req, err := http.NewRequest("GET", cfg.ElasticURL, nil)
		if err == nil {
			if cfg.ElasticToken != "" {
				req.Header.Set("Authorization", "Bearer "+cfg.ElasticToken)
			} else if cfg.ElasticUser != "" || cfg.ElasticPass != "" {
				req.SetBasicAuth(cfg.ElasticUser, cfg.ElasticPass)
			}
			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				results = append(results, fmt.Sprintf("Elasticsearch (%s): ❌ %v", cfg.ElasticURL, err))
			} else {
				if resp.StatusCode < 400 {
					results = append(results, fmt.Sprintf("Elasticsearch (%s): ✅ Connected and reachable", cfg.ElasticURL))
				} else {
					results = append(results, fmt.Sprintf("Elasticsearch (%s): ❌ Status %d", cfg.ElasticURL, resp.StatusCode))
				}
				resp.Body.Close()
			}
		} else {
			results = append(results, fmt.Sprintf("Elasticsearch (%s): ❌ Failed to create request: %v", cfg.ElasticURL, err))
		}
	}

	return results
}
