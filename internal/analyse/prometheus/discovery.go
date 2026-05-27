package prometheus

import (
	"fmt"
	"math"
	"strings"
	"time"

	"health-monitor/pkg/model"
)

// ServiceBaseline stores historical performance metrics
type ServiceBaseline struct {
	P95         float64
	P99         float64
	SuccessRate float64
	RPS         float64
}

// DiscoverServiceMetrics searches for the best latency and request metrics for a service
func (c *Client) DiscoverServiceMetrics(service, serviceLabel string, metricMap, labelMap map[string]string) (model.ServiceMetadata, error) {
	meta := model.ServiceMetadata{}

	// 0. Check for explicit overrides first
	if val, ok := metricMap[service+"_requests"]; ok {
		meta.RequestMetric = val
	}
	if val, ok := metricMap[service+"_latency"]; ok {
		meta.LatencyMetric = val
	}
	if val, ok := labelMap[service]; ok {
		meta.ServiceLabel = val
	}

	// 1. Adaptive Probing: Ask Prometheus what it actually has for this service
	activeMetrics, labels, err := c.ProbeActiveServiceMetrics(service, serviceLabel)
	if err == nil && len(activeMetrics) > 0 {
		// Use the labels we actually found if not explicitly mapped
		if meta.ServiceLabel == "" && len(labels) > 0 {
			meta.ServiceLabel = strings.Join(labels, ",")
		}

		// Look for request candidates among active metrics if not mapped
		if meta.RequestMetric == "" {
			// CRITICAL: Prioritize counters (_total, _count, _sum) to avoid gauges like _in_flight
			counterSuffixes := []string{"_total", "_count", "_sum"}
			reqKeys := []string{"request", "total", "count", "calls"}
			
			// First pass: look for explicit counters
			for _, m := range activeMetrics {
				if isSystemMetric(m) { continue }
				mLower := strings.ToLower(m)
				if containsAny(mLower, reqKeys) && !containsAny(mLower, []string{"duration", "latency", "size", "bytes"}) {
					for _, suffix := range counterSuffixes {
						if strings.HasSuffix(mLower, suffix) {
							meta.RequestMetric = m
							break
						}
					}
				}
				if meta.RequestMetric != "" { break }
			}
			
			// Second pass: fallback to general request keywords if no explicit counter found
			if meta.RequestMetric == "" {
				for _, m := range activeMetrics {
					if isSystemMetric(m) { continue }
					mLower := strings.ToLower(m)
					if containsAny(mLower, reqKeys) && !containsAny(mLower, []string{"duration", "latency", "size", "bytes", "in_flight", "current", "active"}) {
						meta.RequestMetric = m
						break
					}
				}
			}
		}

		// Look for latency candidates among active metrics if not mapped
		if meta.LatencyMetric == "" {
			latKeys := []string{"duration", "latency", "time", "seconds", "ms"}
			for _, m := range activeMetrics {
				if isSystemMetric(m) { continue }
				mLower := strings.ToLower(m)
				if containsAny(mLower, latKeys) && !strings.Contains(mLower, "_count") && !strings.Contains(mLower, "_total") {
					// Avoid summary/quantiles if bucket exists
					if strings.HasSuffix(m, "_bucket") {
						meta.LatencyMetric = strings.TrimSuffix(m, "_bucket")
						break
					}
					meta.LatencyMetric = m
					break
				}
			}
		}
	}

	// 2. Fallback to older static scan if adaptive probing failed
	if meta.RequestMetric == "" || meta.LatencyMetric == "" {
		names, err := c.MetricNames()
		if err == nil {
			if meta.RequestMetric == "" {
				requestCandidates := []string{"http_requests_total", "requests_total", "rpc_requests_total"}
				for _, cand := range requestCandidates {
					if contains(names, cand) {
						meta.RequestMetric = cand
						break
					}
				}
			}
			if meta.LatencyMetric == "" {
				latencyCandidates := []string{"http_request_duration_seconds", "request_duration_seconds", "grpc_server_handling_seconds"}
				for _, cand := range latencyCandidates {
					if contains(names, cand) || contains(names, cand+"_bucket") {
						meta.LatencyMetric = cand
						break
					}
				}
			}
		}
	}

	// 3. Discover Error Label
	if meta.RequestMetric != "" {
		meta.ErrorLabel = c.DiscoverErrorLabel(meta.RequestMetric)
	}

	return meta, nil
}

// ProbeActiveServiceMetrics queries Prometheus to find all metric names where ANY candidate label equals the service name.
func (c *Client) ProbeActiveServiceMetrics(service, serviceLabel string) ([]string, []string, error) {
	candidates := strings.Split(serviceLabel, ",")
	if len(candidates) == 0 || candidates[0] == "" {
		candidates = []string{"service", "app", "job", "container"}
	}

	foundMetrics := make(map[string]bool)
	foundLabels := make(map[string]bool)

	for _, label := range candidates {
		if label == "" { continue }
		// Query series for this label+service combo
		series, err := c.Series(fmt.Sprintf("{%s=\"%s\"}", label, service), 2*time.Hour)
		if err != nil {
			continue
		}
		for _, s := range series {
			if name, ok := s["__name__"]; ok {
				foundMetrics[name] = true
				foundLabels[label] = true
			}
		}
	}

	metricsList := make([]string, 0, len(foundMetrics))
	for m := range foundMetrics {
		metricsList = append(metricsList, m)
	}
	
	labelsList := make([]string, 0, len(foundLabels))
	for l := range foundLabels {
		labelsList = append(labelsList, l)
	}

	return metricsList, labelsList, nil
}

func containsAny(s string, keywords []string) bool {
	for _, k := range keywords {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

// DiscoverErrorLabel tries to find labels like status, code, status_code
func (c *Client) DiscoverErrorLabel(metric string) string {
	series, err := c.Series(metric, 1*time.Hour)
	if err != nil {
		return ""
	}

	candidates := []string{"status", "code", "status_code", "http_status", "response_code"}
	for _, s := range series {
		for _, cand := range candidates {
			if _, ok := s[cand]; ok {
				return cand
			}
		}
	}
	return ""
}

// PopulateBaseline calculates P95 and RPS for a service over a lookback period and updates the metadata
func (c *Client) PopulateBaseline(meta *model.ServiceMetadata, service, serviceLabel, window string) error {
	// Split labels just in case we have legacy comma-separated values
	labels := strings.Split(serviceLabel, ",")
	
	// Helper to build robust query with label regex for multi-label support
	// This is cleaner than 'or' chaining for many labels
	buildFlexibleQuery := func(metric string, extraFilter string, template string) string {
		labelSelector := ""
		if len(labels) == 1 {
			labelSelector = fmt.Sprintf("%s=\"%s\"", labels[0], service)
		} else {
			// use regex: label1|label2|... ="service"
			// Actually Prometheus doesn't support regex for label NAMES, only VALUES.
			// So we stick to 'or' but keep it clean.
			var parts []string
			for _, lbl := range labels {
				if lbl == "" { continue }
				filter := fmt.Sprintf("%s=\"%s\"", lbl, service)
				if extraFilter != "" {
					filter = fmt.Sprintf("%s, %s", filter, extraFilter)
				}
				parts = append(parts, fmt.Sprintf("%s{%s}", metric, filter))
			}
			return fmt.Sprintf(template, strings.Join(parts, " or "))
		}
		
		filter := labelSelector
		if extraFilter != "" {
			filter = fmt.Sprintf("%s, %s", filter, extraFilter)
		}
		return fmt.Sprintf(template, fmt.Sprintf("%s{%s}", metric, filter))
	}

	// Calculate P95 Latency Baseline
	if meta.LatencyMetric != "" {
		// SAFETY: Only append _bucket if the metric doesn't already have a counter-related suffix
		// and if we find the bucket metric in Prometheus
		metricBase := meta.LatencyMetric
		if !strings.HasSuffix(metricBase, "_bucket") && !strings.HasSuffix(metricBase, "_sum") && !strings.HasSuffix(metricBase, "_count") {
			metricBase = metricBase + "_bucket"
		}
		
		p95Template := fmt.Sprintf("histogram_quantile(0.95, sum(rate(%%s[%s])) by (le))", window)
		p95Query := buildFlexibleQuery(metricBase, "", p95Template)
		if val, err := c.QueryInstant(p95Query); err == nil && !math.IsNaN(val) {
			meta.P95Baseline = val
		}

		p99Template := fmt.Sprintf("histogram_quantile(0.99, sum(rate(%%s[%s])) by (le))", window)
		p99Query := buildFlexibleQuery(metricBase, "", p99Template)
		if val, err := c.QueryInstant(p99Query); err == nil && !math.IsNaN(val) {
			meta.P99Baseline = val
		}
	}

	// Calculate RPS and Success Rate Baseline
	if meta.RequestMetric != "" {
		rpsTemplate := fmt.Sprintf("sum(rate(%%s[%s]))", window)
		rpsQuery := buildFlexibleQuery(meta.RequestMetric, "", rpsTemplate)
		if val, err := c.QueryInstant(rpsQuery); err == nil && !math.IsNaN(val) {
			meta.RPS = val
		}

		if meta.ErrorLabel != "" {
			numQuery := buildFlexibleQuery(meta.RequestMetric, fmt.Sprintf("%s=~\"2..|OK\"", meta.ErrorLabel), fmt.Sprintf("sum(rate(%%s[%s]))", window))
			denQuery := buildFlexibleQuery(meta.RequestMetric, "", fmt.Sprintf("sum(rate(%%s[%s]))", window))
			
			successQuery := fmt.Sprintf("(%s) / (%s)", numQuery, denQuery)

			if val, err := c.QueryInstant(successQuery); err == nil && !math.IsNaN(val) {
				meta.SuccessRate = val
			}
		}
	}

	return nil
}


// DiscoverServiceLabel attempts to find the most likely labels used for service names, ordered by usage
func (c *Client) DiscoverServiceLabel() (string, error) {
	candidates := []string{"service_name", "service", "app", "app_kubernetes_io_name", "kubernetes_name", "deployment", "job", "container"}
	
	type labelScore struct {
		label string
		count int
	}
	var scores []labelScore

	for _, label := range candidates {
		query := fmt.Sprintf("count(count by (%v) ({%v!=\"\"}))", label, label)
		val, err := c.QueryInstant(query)
		if err == nil && val > 0 {
			scores = append(scores, labelScore{label: label, count: int(val)})
		}
	}

	// Sort by count descending
	for i := 0; i < len(scores); i++ {
		for j := i + 1; j < len(scores); j++ {
			if scores[j].count > scores[i].count {
				scores[i], scores[j] = scores[j], scores[i]
			}
		}
	}

	// We return ONLY the top label to prevent downstream syntax errors 
	// (e.g., in LogQL or URLs that expect a single label name)
	if len(scores) > 0 {
		return scores[0].label, nil
	}

	return "service", nil // Default fallback
}


func isSystemMetric(metric string) bool {
	systemMetricPrefixes := []string{
		"alertmanager_", "prometheus_", "node_", "go_", "process_",
		"scrape_", "rest_client_", "workqueue_", "apiserver_", "etcd_",
		"machine_", "cadvisor_", "promhttp_", "net_", "aggregator_", "storage_",
		"kube_", "container_", "hidden_",
	}

	// Also check for common system metric substrings
	systemSubstrings := []string{
		"_gc_", "_memstats_", "_mallocs_", "_frees_", "_panics_total",
		"python_gc_", "ruby_gc_", "jvm_gc_",
	}

	mLower := strings.ToLower(metric)
	for _, p := range systemMetricPrefixes {
		if strings.HasPrefix(mLower, p) {
			return true
		}
	}
	for _, s := range systemSubstrings {
		if strings.Contains(mLower, s) {
			return true
		}
	}
	return false
}

func contains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

