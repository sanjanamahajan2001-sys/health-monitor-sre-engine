package service

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"health-monitor/internal/analyse/api_latency"
	"health-monitor/internal/analyse/prometheus"
	"health-monitor/internal/config"
	"health-monitor/internal/discovery"
	"health-monitor/internal/incident"
	"health-monitor/pkg/model"
)

// CollectServiceDrilldown collects detailed service-level metrics using the production-grade APILatency collector
// and enriches them with incident-grade logs and traces.
func CollectServiceDrilldown(client *prometheus.Client, cfg config.Config, serviceName string) (*model.ServiceDrilldown, error) {
	if client == nil {
		return nil, fmt.Errorf("prometheus client not initialized")
	}

	config.DebugLog("CollectServiceDrilldown: Collecting metrics for %s", serviceName)
	
	// 1. Identify the best identifying label for this service by probing
	labelsToTry := []string{"container", "app", "service", "job", "app.kubernetes.io/name", "service.name"}
	if cfg.ServiceLabel != "" && cfg.ServiceLabel != "job" {
		labelsToTry = append([]string{cfg.ServiceLabel}, labelsToTry...)
	}
	bestLabel := "job" 
	foundLabel := false

	for _, l := range labelsToTry {
		query := fmt.Sprintf(`{__name__=~".*",%s=~".*%s.*"}`, l, serviceName)
		if series, err := client.Series(query, 1*time.Hour); err == nil && len(series) > 0 {
			bestLabel = l
			foundLabel = true
			config.DebugLog("CollectServiceDrilldown: [DEBUG] Found active label: %s", l)
			break
		}
	}

	// 1b. Wildcard fallback
	if !foundLabel {
		wildcardQuery := fmt.Sprintf(`{__name__=~".*", .+~"(?i).*%s.*"}`, serviceName)
		if series, err := client.Series(wildcardQuery, 1*time.Hour); err == nil && len(series) > 0 {
			for _, s := range series {
				for k, v := range s {
					if strings.Contains(strings.ToLower(v), strings.ToLower(serviceName)) && 
					   k != "__name__" && k != "instance" && k != "pod" && k != "namespace" && k != "node" {
						bestLabel = k
						foundLabel = true
						config.DebugLog("CollectServiceDrilldown: [DEBUG] Deep wildcard matched label: %s=%s", k, v)
						break
					}
				}
				if foundLabel { break }
			}
		}
	}

	// 2. Use the established api_latency.Collect logic
	svcCfg := cfg
	svcCfg.APIService = serviceName
	svcCfg.ServiceLabel = bestLabel
	svcCfg.AutoDiscover = false

	result, err := api_latency.Collect(client, svcCfg)
	if err != nil {
		config.DebugLog("CollectServiceDrilldown: api_latency.Collect failed for %s: %v", serviceName, err)
		return nil, err
	}

	drilldown := FromAPILatencyResult(serviceName, result)

	// 3. Robust Log Correlation (Incident-Grade Parity)
	endTime := time.Now().UTC()
	startTime := endTime.Add(-48 * time.Hour)

	logCorrelator, err := incident.NewLogCorrelator(cfg)
	if err == nil && logCorrelator != nil {
		config.DebugLog("CollectServiceDrilldown:   - Fetching Incident-Grade logs")
		// Create a dummy incident to satisfy the correlator interface
		dummyInc := incident.Incident{
			ID:        "DRILLDOWN",
			Service:   serviceName,
			CreatedAt: startTime,
		}
		
		logSummary, err := logCorrelator.CorrelateLogsLive(context.Background(), dummyInc, endTime)
		if err == nil && logSummary != nil {
			for _, errStat := range logSummary.TopErrors {
				drilldown.CorrelatedLogs = append(drilldown.CorrelatedLogs, fmt.Sprintf("[%dx] %s", errStat.Count, errStat.Message))
			}
		}
	}

	// 4. Robust Trace Correlation (Incident-Grade Parity)
	traceCorrelator, err := incident.NewTraceCorrelator(cfg)
	if err == nil && traceCorrelator != nil && traceCorrelator.IsEnabled() {
		config.DebugLog("CollectServiceDrilldown:   - Fetching Incident-Grade traces")
		traceSummary, err := traceCorrelator.CorrelateTracesLive(context.Background(), serviceName, startTime, endTime)
		if err == nil && traceSummary != nil {
			if traceSummary.SlowestRoute != "" {
				drilldown.TraceLinks = append(drilldown.TraceLinks, fmt.Sprintf("Slowest route: %s (%.0fms)", traceSummary.SlowestRoute, traceSummary.P95LatencyMs))
			}
			for _, span := range traceSummary.TopSpans {
				drilldown.TraceLinks = append(drilldown.TraceLinks, fmt.Sprintf("• %s (%.0fms)", span.Name, span.DurationMs))
			}
		}
	}

	// 5. Deployment Activity
	config.DebugLog("CollectServiceDrilldown:   - Fetching Deployments")
	drilldown.RecentDeployments = fetchRecentDeployments(client, serviceName)

	return drilldown, nil
}

// FromAPILatencyResult converts a production api_latency.Result to a model.ServiceDrilldown
func FromAPILatencyResult(serviceName string, result api_latency.Result) *model.ServiceDrilldown {
	drilldown := &model.ServiceDrilldown{
		Service: serviceName,
		GoldenSignals: model.GoldenSignals{
			RequestsPerSecond: sanitizeNaN(result.RPS),
			ErrorRate:         sanitizeNaN(result.ErrorRate),
			LatencyP95:        sanitizeNaN(result.P95),
			LatencyBaseline:   sanitizeNaN(result.BaselineP95),
		},
	}

	// Mock SLOs for now (or use result.RemediationHints / alerts if applicable)
	drilldown.SLOs = []model.SLOResult{
		{
			Name:    "Availability (99.9%)",
			Target:  99.9,
			Current: 100.0 - result.ErrorRate,
			Status:  model.SAFE,
		},
		{
			Name:    "Latency (P95 < 500ms)",
			Target:  95,
			Current: 98, // Dummy current value for now
			Status:  model.SAFE,
		},
	}
	if result.ErrorRate > 0.1 {
		drilldown.SLOs[0].Status = model.CHECK
	}
	if result.ErrorRate > 1.0 {
		drilldown.SLOs[0].Status = model.RISK
	}

	return drilldown
}

func fetchGoldenSignals(client *prometheus.Client, cfg config.Config, service string) model.GoldenSignals {
	signals := model.GoldenSignals{}
	window := "5m"

	// RPS
	rpsQuery := fmt.Sprintf(`sum(rate(%s{%s="%s"}[%s]))`, cfg.RequestCountMetric, cfg.ServiceLabel, service, window)
	if val, err := client.QueryInstant(rpsQuery); err == nil {
		signals.RequestsPerSecond = val
		config.DebugLog("fetchGoldenSignals: %s RPS = %.2f", service, val)
	} else {
		config.DebugLog("fetchGoldenSignals: %s RPS query failed: %v", service, err)
	}

	// Error Rate
	errLabel := cfg.ErrorLabel
	if errLabel == "" {
		errLabel = "status"
	}
	errRegex := cfg.ErrorRegex
	if errRegex == "" {
		errRegex = "5.."
	}
	errQuery := fmt.Sprintf(`sum(rate(%s{%s="%s", %s=~"%s"}[%s])) / sum(rate(%s{%s="%s"}[%s])) * 100`, 
		cfg.RequestCountMetric, cfg.ServiceLabel, service, errLabel, errRegex, window,
		cfg.RequestCountMetric, cfg.ServiceLabel, service, window)
	if val, err := client.QueryInstant(errQuery); err == nil {
		signals.ErrorRate = val
	}

	// Latency P95
	latMetric := cfg.LatencyMetric
	if latMetric != "" && !strings.HasSuffix(latMetric, "_bucket") {
		latMetric += "_bucket"
	}
	if latMetric == "" {
		latMetric = "http_request_duration_seconds_bucket"
	}

	latQuery := fmt.Sprintf(`histogram_quantile(0.95, sum(rate(%s{%s="%s"}[%s])) by (le))`, latMetric, cfg.ServiceLabel, service, window)
	if val, err := client.QueryInstant(latQuery); err == nil {
		signals.LatencyP95 = val
	}

	// Baseline Latency (1h ago)
	baseLatQuery := fmt.Sprintf(`histogram_quantile(0.95, sum(rate(%s{%s="%s"}[5m] offset 1h)) by (le))`, latMetric, cfg.ServiceLabel, service)
	if val, err := client.QueryInstant(baseLatQuery); err == nil {
		signals.LatencyBaseline = val
	}

	// Sanitize NaNs
	signals.RequestsPerSecond = sanitizeNaN(signals.RequestsPerSecond)
	signals.ErrorRate = sanitizeNaN(signals.ErrorRate)
	signals.LatencyP95 = sanitizeNaN(signals.LatencyP95)
	signals.LatencyBaseline = sanitizeNaN(signals.LatencyBaseline)

	return signals
}

func sanitizeNaN(val float64) float64 {
	if math.IsNaN(val) {
		return 0
	}
	return val
}

func fetchSLOResults(client *prometheus.Client, cfg config.Config, service string) []model.SLOResult {
	// For now, return some dummy SLOs based on availability and latency
	// Real implementation would use flow.Loader/generator
	return []model.SLOResult{
		{
			Name:    "Availability",
			Target:  99.9,
			Current: 99.95,
			Status:  model.SAFE,
		},
		{
			Name:    "Latency (P95 < 500ms)",
			Target:  95,
			Current: 98,
			Status:  model.SAFE,
		},
	}
}

func fetchRecentDeployments(client *prometheus.Client, service string) []model.DeploymentEvent {
	if client == nil {
		return nil
	}
	// Query pod creation times for this service in the last 24h
	results := []model.DeploymentEvent{}
	series, err := client.Series("kube_pod_created", 24*time.Hour)
	if err == nil {
		for _, s := range series {
			// Filter by service name in ANY matching label
			match := false
			for _, l := range []string{"service", "job", "app", "label_app"} {
				if s[l] == service {
					match = true
					break
				}
			}
			if match {
				if startTime, ok := s["__start_time__"]; ok {
					// Prometheus Series API often returns timestamp of discovery
					t, _ := time.Parse(time.RFC3339, startTime)
					results = append(results, model.DeploymentEvent{
						Time:    t,
						Version: s["image"],
						Status:  "Created",
					})
				}
			}
		}
	}

	// Mocking for now as complex join queries are needed
	if len(results) == 0 {
		return []model.DeploymentEvent{
			{
				Time:    time.Now().Add(-4 * time.Hour),
				Version: "v1.2.3",
				Status:  "Completed",
			},
		}
	}
	// Sort by time descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Time.After(results[j].Time)
	})

	return results
}

func hasLabelOnKubeMetric(client *prometheus.Client, metric string, label string) bool {
	series, _ := client.Series(metric, time.Hour)
	for _, s := range series {
		if _, ok := s[label]; ok {
			return true
		}
	}
	return false
}

// CollectAllServicesOverview fetches high-level metrics for all discovered services
func CollectAllServicesOverview(client *prometheus.Client, cfg config.Config) ([]model.ServiceOverview, error) {
	if client == nil {
		return nil, fmt.Errorf("prometheus client not initialized")
	}

	// 1. Identify all services using LabelValues API (Broad discovery)
	// We probe multiple common labels to catch otel-demo and other frameworks
	labelsToTry := []string{"service", "service.name", "app", "application", "app.kubernetes.io/name", "job"}
	if cfg.ServiceLabel != "" {
		labelsToTry = append([]string{cfg.ServiceLabel}, labelsToTry...)
	}

	seenServices := make(map[string]string) // name -> label
	lookback := 2 * time.Hour

	for _, l := range labelsToTry {
		values, err := client.LabelValues(l, lookback)
		if err == nil && len(values) > 0 {
			foundNew := 0
			for _, val := range values {
				if val != "" && seenServices[val] == "" {
					seenServices[val] = l
					foundNew++
				}
			}
			config.DebugLog("CollectAllServicesOverview:   - Found %d unique services from label: %s", foundNew, l)
		}
	}

	// 2. Fallback: If still very few services, try a Series query to find names from any label
	if len(seenServices) < 5 {
		config.DebugLog("CollectAllServicesOverview:   - Sparse results, attempting Series discovery")
		series, err := client.Series(`{__name__=~".*request.*"}`, lookback)
		if err == nil {
			for _, s := range series {
				for _, l := range labelsToTry {
					if val, ok := s[l]; ok && val != "" && seenServices[val] == "" {
						seenServices[val] = l
					}
				}
			}
		}
	}

	// 3. Filter and Prepare services for parallel probing
	systemBlacklist := []string{
		"prometheus", "grafana", "tempo", "loki", "alertmanager", 
		"kube-system", "kubernetes", "metrics-server", "otel-collector", 
		"jaeger-collector", "node-exporter", "kube-state-metrics",
		"istio-proxy", "ingress-nginx", "cert-manager", "vpa-recommender",
		"controller-manager", "scheduler", "kube-proxy", "etcd",
		"cni", "weave", "calico", "flannel", "coredns", "storage-provisioner",
		"local-path", "nfs", "csi", "snapshot", "argocd", "flux",
	}

	var servicesToProbe []string
	for svcName := range seenServices {
		isSystem := false
		lowerSvc := strings.ToLower(svcName)
		for _, black := range systemBlacklist {
			if strings.Contains(lowerSvc, black) {
				isSystem = true
				break
			}
		}
		if isSystem && !strings.Contains(lowerSvc, "orders") && !strings.Contains(lowerSvc, "payment") && !strings.Contains(lowerSvc, "cart") && !strings.Contains(lowerSvc, "checkout") && !strings.Contains(lowerSvc, "frontend") && !strings.Contains(lowerSvc, "shipping") {
			continue
		}
		servicesToProbe = append(servicesToProbe, svcName)
	}

	// 4. Parallel Probing using ServicePool
	pool := discovery.NewServicePool(20) // Use 20 workers for high-volume discovery
	probeFunc := func(svcName string) (interface{}, error) {
		bestLabel := seenServices[svcName]
		if bestLabel == "" {
			bestLabel = "job"
		}
		svcCfg := cfg
		svcCfg.ServiceLabel = bestLabel
		signals := fetchGoldenSignals(client, svcCfg, svcName)
		return signals, nil
	}

	probeResults := pool.ParallelProbe(servicesToProbe, probeFunc)

	// 5. Build final overviews from results
	finalOverviews := make([]model.ServiceOverview, 0, len(probeResults))
	for _, res := range probeResults {
		signals := res.Data.(model.GoldenSignals)
		overview := model.ServiceOverview{
			Name:      res.Service,
			RPS:       signals.RequestsPerSecond,
			ErrorRate: signals.ErrorRate,
			P95:       signals.LatencyP95,
			Status:    model.SAFE,
		}

		if overview.ErrorRate > 5 || (overview.P95 > 1.0 && overview.RPS > 0) {
			overview.Status = model.RISK
		} else if overview.ErrorRate > 1 || (overview.P95 > 0.5 && overview.RPS > 0) {
			overview.Status = model.CHECK
		}
		finalOverviews = append(finalOverviews, overview)
	}

	// Sort alphabetically for stable UI
	sort.Slice(finalOverviews, func(i, j int) bool {
		return finalOverviews[i].Name < finalOverviews[j].Name
	})

	config.DebugLog("CollectAllServicesOverview: Returning %d overviews (parallelized from %d)", len(finalOverviews), len(seenServices))
	return finalOverviews, nil
}

func mapsKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
