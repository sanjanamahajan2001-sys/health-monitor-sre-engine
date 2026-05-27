package api_latency

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"health-monitor/internal/analyse/prometheus"
	"health-monitor/internal/config"
)

type Result struct {
	P90                    float64
	P95                    float64
	P99                    float64
	BaselineWindow         string
	BaselineP95            float64
	BaselineErrorRate      float64
	BaselineRPS            float64
	DeltaP95               float64
	DeltaErrorRate         float64
	DeltaRPS               float64
	ErrorRate              float64
	ClientErrorRate        float64
	ServerErrorRate        float64
	ErrorLabel             string
	RPS                    float64
	Note                   string
	Confidence             string
	Window                 string
	TopEndpoints           []EndpointLatency
	TopEndpointsRPS        []EndpointRate
	TopEndpointsRPSNote    string
	TopServicesRPS         []ServiceRate
	TopServicesRPSNote     string
	ActiveAlerts           []string
	TopEndpointRegressions []EndpointRegression
	RegressionNote         string
	SpikeNote              string
	NoDataReasons          []NoDataReason
	RemediationHints       []string
	LokiCorrelationHints   []string
	LatencyMetric          string
	RequestMetric          string
	ServiceLabel           string
	RouteLabel             string
	LabelSelector          string
}

type EndpointLatency struct {
	Route string
	P95   float64
}

type EndpointRegression struct {
	Route       string
	NowP95      float64
	BaselineP95 float64
	DeltaP95    float64
}

type EndpointRate struct {
	Route string
	RPS   float64
}

type ServiceRate struct {
	Service string
	RPS     float64
}

type NoDataReason struct {
	Area         string
	Reason       string
	Metric       string
	Labels       string
	Window       string
	SuggestedFix string
}

func Collect(client *prometheus.Client, cfg config.Config) (Result, error) {
	if client == nil {
		return Result{}, errors.New("prometheus client is nil")
	}
	note := ""
	notes := []string{}
	noDataReasons := []NoDataReason{}
	window := cfg.Window
	if window == "" {
		window = "5m"
	}
	if cfg.AutoDiscover {
		config.DebugLog("APILatency: starting auto-discovery")
		discovered, discoveredNote, err := discoverConfig(client, cfg, window)
		if err != nil {
			config.DebugLog("APILatency: auto-discovery failed: %v", err)
			return Result{}, err
		}
		cfg = discovered
		note = discoveredNote
		config.DebugLog("APILatency: auto-discovery finished")
	}

	if cfg.AutoDiscover && (cfg.ErrorLabel == "" || strings.EqualFold(cfg.ErrorLabel, "auto")) {
		if label := discoverErrorLabel(client, cfg.RequestCountMetric); label != "" {
			cfg.ErrorLabel = label
			note = joinNotes(note, []string{"error label: " + label, "error label auto-detect: matched on request metric"})
		}
	}

	if cfg.AutoDiscover {
		if metric, label, reason := discoverRequestMetricWithRouteLabel(client, cfg.LatencyMetric, window); metric != "" {
			cfg.RequestCountMetric = metric
			if cfg.RouteLabel == "" {
				cfg.RouteLabel = label
			}
			noteItems := []string{"request metric (route labels): " + metric, "route label (requests): " + label}
			if strings.TrimSpace(reason) != "" {
				noteItems = append(noteItems, "request metric choice: "+reason)
			}
			note = joinNotes(note, noteItems)
		}
	}

	if cfg.AutoDiscover && cfg.RouteLabel != "" && !hasLabelOnMetric(client, cfg.RequestCountMetric, cfg.RouteLabel) {
		cfg.RouteLabel = ""
	}

	routeLabelForRPS := ""
	if cfg.RouteLabel != "" {
		routeLabelForRPS = cfg.RouteLabel
	} else if cfg.AutoDiscover {
		if label := discoverRouteLabelOnMetric(client, cfg.RequestCountMetric); label != "" {
			routeLabelForRPS = label
			if cfg.RouteLabel == "" {
				cfg.RouteLabel = label
			}
			note = joinNotes(note, []string{"route label (requests): " + label})
		}
	}

	if cfg.AutoDiscover {
		cfg, note = validateAutoLabels(client, cfg, window, note)
	}

	if cfg.RouteLabel != "" && !hasLabelOnMetric(client, cfg.RequestCountMetric, cfg.RouteLabel) {
		addNoDataReason(&noDataReasons, NoDataReason{
			Area:         "Route label",
			Reason:       fmt.Sprintf("label %q not found on request metric", cfg.RouteLabel),
			Metric:       cfg.RequestCountMetric,
			Labels:       strings.Join(buildLabelSelectors(cfg), ","),
			Window:       window,
			SuggestedFix: "Set route_label to a label that exists on the request metric or enable auto-discover.",
		})
		if cfg.AutoDiscover {
			note = joinNotes(note, []string{fmt.Sprintf("route label %q not found; ignoring", cfg.RouteLabel)})
			cfg.RouteLabel = ""
		}
	}
	if cfg.ServiceLabel != "" && !hasLabelOnMetric(client, cfg.RequestCountMetric, cfg.ServiceLabel) {
		addNoDataReason(&noDataReasons, NoDataReason{
			Area:         "Service label",
			Reason:       fmt.Sprintf("label %q not found on request metric", cfg.ServiceLabel),
			Metric:       cfg.RequestCountMetric,
			Labels:       strings.Join(buildLabelSelectors(cfg), ","),
			Window:       window,
			SuggestedFix: "Set service_label to a label that exists on the request metric or enable auto-discover.",
		})
		if cfg.AutoDiscover {
			note = joinNotes(note, []string{fmt.Sprintf("service label %q not found; ignoring", cfg.ServiceLabel)})
			cfg.ServiceLabel = ""
		}
	}

	extraSelectors, extraNote := sanitizeExtraLabelSelectors(cfg.ExtraLabelSelectors)
	if strings.TrimSpace(extraNote) != "" {
		notes = append(notes, extraNote)
	}
	cfgSafe := cfg
	cfgSafe.ExtraLabelSelectors = extraSelectors
	cfgSafe.ServiceLabel = sanitizePromLabelName(cfgSafe.ServiceLabel, "Service label", cfgSafe.RequestCountMetric, window, &noDataReasons)
	cfgSafe.RouteLabel = sanitizePromLabelName(cfgSafe.RouteLabel, "Route label", cfgSafe.RequestCountMetric, window, &noDataReasons)
	labelsSlice := buildLabelSelectors(cfgSafe)
	labelsJoined := strings.Join(labelsSlice, ",")

	latencyMetric := cfg.LatencyMetric
	if latencyMetric == "" {
		latencyMetric = "http_request_duration_seconds"
	}
	bucketMetric := latencyBucketMetric(latencyMetric)
	reqMetric := cfg.RequestCountMetric
	if reqMetric == "" {
		reqMetric = "http_requests_total"
	}
	if !isPromMetricName(latencyMetric) {
		addNoDataReason(&noDataReasons, NoDataReason{
			Area:         "Latency metric",
			Reason:       fmt.Sprintf("invalid metric name %q", latencyMetric),
			Metric:       latencyMetric,
			Labels:       labelsJoined,
			Window:       window,
			SuggestedFix: "Fix API_LATENCY_METRIC or enable API_AUTO_DISCOVER.",
		})
		latencyMetric = ""
		bucketMetric = ""
	}
	if !isPromMetricName(reqMetric) {
		addNoDataReason(&noDataReasons, NoDataReason{
			Area:         "Request metric",
			Reason:       fmt.Sprintf("invalid metric name %q", reqMetric),
			Metric:       reqMetric,
			Labels:       labelsJoined,
			Window:       window,
			SuggestedFix: "Fix API_REQUESTS_METRIC or enable API_AUTO_DISCOVER.",
		})
		reqMetric = ""
	}
	metricNames, _ := client.MetricNames()
	if strings.TrimSpace(latencyMetric) != "" && !metricExists(metricNames, latencyMetric) && !metricExists(metricNames, latencyBucketMetric(latencyMetric)) {
		addNoDataReason(&noDataReasons, NoDataReason{
			Area:         "Latency metric",
			Reason:       fmt.Sprintf("metric %q not found in Prometheus", latencyMetric),
			Metric:       latencyMetric,
			Labels:       labelsJoined,
			Window:       window,
			SuggestedFix: "Reset API_LATENCY_METRIC or enable API_AUTO_DISCOVER to find a valid histogram/summary.",
		})
	}
	if strings.TrimSpace(reqMetric) != "" && !metricExists(metricNames, reqMetric) {
		addNoDataReason(&noDataReasons, NoDataReason{
			Area:         "Request metric",
			Reason:       fmt.Sprintf("metric %q not found in Prometheus", reqMetric),
			Metric:       reqMetric,
			Labels:       labelsJoined,
			Window:       window,
			SuggestedFix: "Reset API_REQUESTS_METRIC or enable API_AUTO_DISCOVER to find a valid request counter.",
		})
	}

	var err error
	p90 := math.NaN()
	p95 := math.NaN()
	p99 := math.NaN()
	latencySource := ""
	usedWindow := window
	if latencyMetric == "" {
		err = prometheus.ErrNoData
	} else {
		p90, p95, p99, usedWindow, latencySource, err = queryLatencyQuantilesWithFallback(client, latencyMetric, labelsSlice, window)
	}
	if err != nil {
		if errors.Is(err, prometheus.ErrNoData) {
			addNoDataReason(&noDataReasons, NoDataReason{
				Area:         "Latency samples",
				Reason:       "no latency series returned",
				Metric:       latencyMetric,
				Labels:       labelsJoined,
				Window:       window,
				SuggestedFix: "Verify the latency metric exists and labels match; increase window or confirm Prometheus scrape target.",
			})
			notes = append(notes, "no latency samples in window")
			p90, p95, p99 = math.NaN(), math.NaN(), math.NaN()
			usedWindow = window
		} else {
			return Result{}, annotateNoDataWithContext(err, "p90 latency", cfg, labelsJoined, window)
		}
	}
	if usedWindow != window {
		notes = append(notes, "window fallback: "+usedWindow)
		window = usedWindow
	}
	if latencySource == "summary" {
		notes = append(notes, "latency source: summary quantiles (no histogram buckets)")
	}

	total := 0.0
	totalWindow := window
	if reqMetric == "" {
		err = prometheus.ErrNoData
		total = 0
		totalWindow = window
	} else {
		total, totalWindow, err = queryTotalWithFallback(client, reqMetric, labelsSlice, window)
	}
	if err != nil {
		if errors.Is(err, prometheus.ErrNoData) {
			addNoDataReason(&noDataReasons, NoDataReason{
				Area:         "Traffic",
				Reason:       "request metric returned no data",
				Metric:       reqMetric,
				Labels:       labelsJoined,
				Window:       window,
				SuggestedFix: "Verify the request metric exists and labels match; increase the window or generate traffic.",
			})
			total = 0
		} else {
			return Result{}, err
		}
	}
	if totalWindow != window {
		total, _, _ = queryTotalWithFallback(client, reqMetric, labelsSlice, window)
	}
	applyStaleDataHint(client, reqMetric, labelsJoined, window, &noDataReasons)
	cardinalityInfo := applyCardinalityHints(client, reqMetric, cfgSafe, labelsJoined, window, &notes)

	errorLabel := cfg.ErrorLabel
	if errorLabel == "" {
		errorLabel = "status"
	}
	errorRegex := cfg.ErrorRegex
	if errorRegex == "" {
		errorRegex = "5.."
	}
	errRate := math.NaN()
	clientErrRate := math.NaN()
	serverErrRate := math.NaN()
	
	// Fallback for common status labels if not explicitly configured
	if !hasLabelOnMetric(client, reqMetric, errorLabel) && cfg.ErrorLabel == "" {
		if hasLabelOnMetric(client, reqMetric, "code") {
			errorLabel = "code"
		}
	}

	errorSelectors := addErrorLabels(labelsSlice, errorLabel, errorRegex)
	errorQ := "sum(rate(" + buildFallbackQuery(reqMetric, errorSelectors, window) + "))"
	if val, err := client.QueryInstant(errorQ); err == nil {
		errRate = val
	}

	clientSelectors := addErrorLabels(labelsSlice, errorLabel, "4..")
	clientQ := "sum(rate(" + buildFallbackQuery(reqMetric, clientSelectors, window) + "))"
	if val, err := client.QueryInstant(clientQ); err == nil {
		clientErrRate = val
	}

	serverSelectors := addErrorLabels(labelsSlice, errorLabel, "5..")
	serverQ := "sum(rate(" + buildFallbackQuery(reqMetric, serverSelectors, window) + "))"
	if val, err := client.QueryInstant(serverQ); err == nil {
		serverErrRate = val
	}

	errorPct := math.NaN()
	clientErrPct := math.NaN()
	serverErrPct := math.NaN()
	if total > 0 {
		if !math.IsNaN(errRate) {
			errorPct = (errRate / total) * 100
		} else {
			errorPct = 0
		}
		if !math.IsNaN(clientErrRate) {
			clientErrPct = (clientErrRate / total) * 100
		} else {
			clientErrPct = 0
		}
		if !math.IsNaN(serverErrRate) {
			serverErrPct = (serverErrRate / total) * 100
		} else {
			serverErrPct = 0
		}
	} else {
		notes = append(notes, "no traffic in window")
		addNoDataReason(&noDataReasons, NoDataReason{
			Area:         "Traffic",
			Reason:       "no request volume in selected window",
			Metric:       reqMetric,
			Labels:       labelsJoined,
			Window:       window,
			SuggestedFix: "Increase the window or generate traffic to this service/route.",
		})
	}

	baselineWindow := chooseBaselineWindow(window)
	bp90, bp95, bp99, usedBaseline, _, berr := queryLatencyQuantilesWithFallback(client, latencyMetric, labelsSlice, baselineWindow)
	btotal, usedBaselineTotal, btotalErr := queryTotalWithFallback(client, reqMetric, labelsSlice, baselineWindow)
	_ = bp90
	_ = bp99
	baselineErrPct := math.NaN()
	if berr != nil {
		notes = append(notes, "baseline unavailable")
		if errors.Is(berr, prometheus.ErrNoData) {
			addNoDataReason(&noDataReasons, NoDataReason{
				Area:         "Baseline",
				Reason:       "no historical data in baseline window",
				Metric:       bucketMetric,
				Labels:       labelsJoined,
				Window:       baselineWindow,
				SuggestedFix: "Increase baseline window or ensure Prometheus retention covers the baseline period.",
			})
		}
	} else if btotalErr != nil {
		if errors.Is(btotalErr, prometheus.ErrNoData) {
			addNoDataReason(&noDataReasons, NoDataReason{
				Area:         "Baseline traffic",
				Reason:       "baseline request metric returned no data",
				Metric:       reqMetric,
				Labels:       labelsJoined,
				Window:       baselineWindow,
				SuggestedFix: "Increase baseline window or verify the request metric exists.",
			})
		}
	} else if btotal > 0 {
		if hasLabelOnMetric(client, reqMetric, errorLabel) {
			errorSelectors := addErrorLabels(labelsSlice, errorLabel, errorRegex)
			windowUsed := usedBaseline
			if strings.TrimSpace(usedBaselineTotal) != "" {
				windowUsed = usedBaselineTotal
			}
			errorQ := "sum(rate" + buildFallbackQuery(reqMetric, errorSelectors, windowUsed) + ")"
			if val, err := client.QueryInstant(errorQ); err == nil {
				baselineErrPct = (val / btotal) * 100
			}
		}
	}

	activeAlerts := fetchActiveAlerts(client, cfg)

	allowRouteQueries := true
	if cardinalityInfo.RouteCount >= cardinalityInfo.RouteLimit && cardinalityInfo.RouteLimit > 0 {
		allowRouteQueries = false
		addNoDataReason(&noDataReasons, NoDataReason{
			Area:         "Route cardinality",
			Reason:       fmt.Sprintf("label %q has %d values (limit %d)", cardinalityInfo.RouteLabel, cardinalityInfo.RouteCount, cardinalityInfo.RouteLimit),
			Metric:       reqMetric,
			Labels:       labelsJoined,
			Window:       window,
			SuggestedFix: "Reduce cardinality (template routes) or raise API_ROUTE_CARDINALITY_LIMIT.",
		})
		notes = append(notes, "route cardinality limit exceeded; skipping per-route queries")
	}

	topEndpoints := []EndpointLatency{}
	if allowRouteQueries && cfg.RouteLabel != "" && normalizeRoute(cfg.APIRoute) == "" {
		if endpoints, err := fetchTopEndpoints(client, cfg, bucketMetric, labelsSlice, window); err == nil {
			topEndpoints = endpoints
		} else if errors.Is(err, prometheus.ErrNoData) {
			addNoDataReason(&noDataReasons, NoDataReason{
				Area:         "Top Endpoints (P95)",
				Reason:       "route label missing on latency metric",
				Metric:       bucketMetric,
				Labels:       labelsJoined,
				Window:       window,
				SuggestedFix: "Set route_label to the endpoint/path label or use a request metric with endpoint labels.",
			})
		}
	} else if allowRouteQueries && cfg.RouteLabel == "" && normalizeRoute(cfg.APIRoute) == "" {
		addNoDataReason(&noDataReasons, NoDataReason{
			Area:         "Top Endpoints (P95)",
			Reason:       "route label not configured",
			Metric:       bucketMetric,
			Labels:       labelsJoined,
			Window:       window,
			SuggestedFix: "Set route_label or use a request metric with endpoint labels.",
		})
	}

	regressions := []EndpointRegression{}
	regressionNote := ""
	if allowRouteQueries && cfg.RouteLabel != "" && normalizeRoute(cfg.APIRoute) == "" {
		if list, note := fetchEndpointRegressions(client, cfg, bucketMetric, labelsSlice, window, baselineWindow); len(list) > 0 {
			regressions = list
			regressionNote = note
		}
	}

	topEndpointsRPS := []EndpointRate{}
	topEndpointsRPSNote := ""
	if allowRouteQueries && routeLabelForRPS != "" && normalizeRoute(cfg.APIRoute) == "" {
		if endpoints, note, err := fetchTopEndpointsRPS(client, cfg.RequestCountMetric, routeLabelForRPS, labelsSlice, window, cfg.TopEndpoints); err == nil {
			topEndpointsRPS = endpoints
			topEndpointsRPSNote = note
		} else if errors.Is(err, prometheus.ErrNoData) {
			addNoDataReason(&noDataReasons, NoDataReason{
				Area:         "Top Endpoints (RPS)",
				Reason:       "route label missing on request metric",
				Metric:       cfg.RequestCountMetric,
				Labels:       labelsJoined,
				Window:       window,
				SuggestedFix: "Set route_label to the endpoint/path label or use a request metric with endpoint labels.",
			})
		}
	} else if allowRouteQueries && routeLabelForRPS == "" && normalizeRoute(cfg.APIRoute) == "" {
		addNoDataReason(&noDataReasons, NoDataReason{
			Area:         "Top Endpoints (RPS)",
			Reason:       "route label not configured",
			Metric:       cfg.RequestCountMetric,
			Labels:       labelsJoined,
			Window:       window,
			SuggestedFix: "Set route_label or use a request metric with endpoint labels.",
		})
	}


	allowServiceQueries := true
	if cardinalityInfo.ServiceCount >= cardinalityInfo.ServiceLimit && cardinalityInfo.ServiceLimit > 0 {
		allowServiceQueries = false
		addNoDataReason(&noDataReasons, NoDataReason{
			Area:         "Service cardinality",
			Reason:       fmt.Sprintf("label %q has %d values (limit %d)", cardinalityInfo.ServiceLabel, cardinalityInfo.ServiceCount, cardinalityInfo.ServiceLimit),
			Metric:       reqMetric,
			Labels:       labelsJoined,
			Window:       window,
			SuggestedFix: "Reduce cardinality or raise API_SERVICE_CARDINALITY_LIMIT.",
		})
		notes = append(notes, "service cardinality limit exceeded; skipping service queries")
	}

	topServicesRPS := []ServiceRate{}
	topServicesRPSNote := ""
	if allowServiceQueries && cfg.ServiceLabel != "" && cfg.APIService == "" {
		if services, note, err := fetchTopServicesRPS(client, cfg.RequestCountMetric, cfg.ServiceLabel, labelsSlice, window, cfg.TopEndpoints); err == nil {
			topServicesRPS = services
			topServicesRPSNote = note
		} else if errors.Is(err, prometheus.ErrNoData) {
			addNoDataReason(&noDataReasons, NoDataReason{
				Area:         "Top Services (RPS)",
				Reason:       "service label missing on request metric",
				Metric:       cfg.RequestCountMetric,
				Labels:       labelsJoined,
				Window:       window,
				SuggestedFix: "Set service_label to the job/service label or use a request metric with service labels.",
			})
		}
	} else if allowServiceQueries && cfg.ServiceLabel == "" && cfg.APIService == "" {
		addNoDataReason(&noDataReasons, NoDataReason{
			Area:         "Top Services (RPS)",
			Reason:       "service label not configured",
			Metric:       cfg.RequestCountMetric,
			Labels:       labelsJoined,
			Window:       window,
			SuggestedFix: "Set service_label (often job/service/app) or use a request metric with service labels.",
		})
	}

	if math.IsNaN(p90) || math.IsNaN(p95) || math.IsNaN(p99) {
		notes = append(notes, "no latency samples in window")
	}

	confidence := determineConfidence(cfg, topEndpoints, topEndpointsRPS, topServicesRPS, notes)

	spikeNote := detectSpike(p95, bp95, errorPct, baselineErrPct)

	result := Result{
		P90:                    p90,
		P95:                    p95,
		P99:                    p99,
		BaselineWindow:         usedBaseline,
		BaselineP95:            bp95,
		BaselineErrorRate:      baselineErrPct,
		BaselineRPS:            btotal,
		DeltaP95:               deltaValue(p95, bp95),
		DeltaErrorRate:         deltaValue(errorPct, baselineErrPct),
		DeltaRPS:               deltaValue(total, btotal),
		ErrorRate:              errorPct,
		ClientErrorRate:        clientErrPct,
		ServerErrorRate:        serverErrPct,
		ErrorLabel:             errorLabel,
		RPS:                    total,
		Note:                   joinNotes(note, notes),
		Window:                 window,
		Confidence:             confidence,
		TopEndpoints:           topEndpoints,
		TopEndpointsRPS:        topEndpointsRPS,
		TopEndpointsRPSNote:    topEndpointsRPSNote,
		TopServicesRPS:         topServicesRPS,
		TopServicesRPSNote:     topServicesRPSNote,
		ActiveAlerts:           activeAlerts,
		TopEndpointRegressions: regressions,
		RegressionNote:         regressionNote,
		SpikeNote:              spikeNote,
		NoDataReasons:          noDataReasons,
		LatencyMetric:          latencyMetric,
		RequestMetric:          reqMetric,
		ServiceLabel:           cfg.ServiceLabel,
		RouteLabel:             cfg.RouteLabel,
		LabelSelector:          labelsJoined,
	}

	result.RemediationHints = buildRemediationHints(cfg, result)
	result.RemediationHints = append(result.RemediationHints, buildDependencyHints(client, result)...)
	result.RemediationHints = uniqueStrings(result.RemediationHints)
	result.LokiCorrelationHints = buildLokiCorrelationHints(cfg, result)
	if len(result.NoDataReasons) > 0 {
		applyTargetHealthHint(client, &result.NoDataReasons)
	}
	return result, nil
}

func buildLabelSelectors(cfg config.Config) []string {
	serviceLabels := strings.Split(cfg.ServiceLabel, ",")
	if len(serviceLabels) == 0 || (len(serviceLabels) == 1 && serviceLabels[0] == "") {
		serviceLabels = []string{""}
	}

	var results []string
	for _, sLabel := range serviceLabels {
		var labels []string
		if sLabel != "" && cfg.APIService != "" {
			labels = append(labels, fmt.Sprintf(`%s="%s"`, sLabel, escapePromLabelValue(cfg.APIService)))
		}
		if cfg.RouteLabel != "" && normalizeRoute(cfg.APIRoute) != "" {
			labels = append(labels, fmt.Sprintf(`%s="%s"`, cfg.RouteLabel, escapePromLabelValue(cfg.APIRoute)))
		}
		if cfg.ExtraLabelSelectors != "" {
			labels = append(labels, cfg.ExtraLabelSelectors)
		}
		
		if len(labels) == 0 {
			results = append(results, "{}")
		} else {
			results = append(results, "{"+strings.Join(labels, ", ")+"}")
		}
	}
	return results
}


func normalizeRoute(route string) string {
	trimmed := strings.TrimSpace(strings.ToLower(route))
	if trimmed == "" || trimmed == "all routes" || trimmed == "all" || trimmed == "*" {
		return ""
	}
	return route
}

func addErrorLabels(bases []string, label string, regex string) []string {
	if strings.TrimSpace(label) == "" {
		return bases
	}
	var results []string
	labelExpr := fmt.Sprintf(`%s=~"%s"`, label, escapePromLabelValue(regex))
	for _, base := range bases {
		if base == "" || base == "{}" {
			results = append(results, "{"+labelExpr+"}")
		} else if strings.HasSuffix(base, "}") {
			results = append(results, strings.TrimSuffix(base, "}")+","+labelExpr+"}")
		} else {
			results = append(results, base)
		}
	}
	return results
}


func latencyBucketMetric(metric string) string {
	m := strings.TrimSpace(metric)
	if strings.HasSuffix(m, "_bucket") {
		return m
	}
	return m + "_bucket"
}

func annotateNoData(err error, area string) error {
	if errors.Is(err, prometheus.ErrNoData) {
		return fmt.Errorf("no data for %s (check metric name and labels)", area)
	}
	return err
}

func annotateNoDataWithContext(err error, area string, cfg config.Config, labels string, window string) error {
	if errors.Is(err, prometheus.ErrNoData) {
		return fmt.Errorf(
			"no data for %s (metric=%s labels=%s window=%s)",
			area,
			cfg.LatencyMetric,
			labels,
			window,
		)
	}
	return err
}

func addNoDataReason(reasons *[]NoDataReason, item NoDataReason) {
	if item.Area == "" || item.Reason == "" {
		return
	}
	for _, existing := range *reasons {
		if existing.Area == item.Area && existing.Reason == item.Reason {
			return
		}
	}
	*reasons = append(*reasons, item)
}

func buildRemediationHints(cfg config.Config, result Result) []string {
	var hints []string
	if len(result.NoDataReasons) > 0 {
		hints = append(hints, "Resolve data gaps first (missing metrics/labels can hide real issues).")
	}
	if strings.Contains(strings.ToLower(result.Note), "high route cardinality") {
		hints = append(hints, "Route label has high cardinality; use templated routes or drop IDs from labels.")
	}
	if strings.Contains(strings.ToLower(result.Note), "high service cardinality") {
		hints = append(hints, "Service label has high cardinality; prefer stable service names for aggregation.")
	}
	if result.SpikeNote != "" {
		hints = append(hints, "Check recent deploys/config changes around the spike window.")
	}
	if !math.IsNaN(result.ErrorRate) && result.ErrorRate >= 1 {
		hints = append(hints, "Inspect error logs and upstream dependency health for correlated failures.")
	}
	if !math.IsNaN(result.ServerErrorRate) && result.ServerErrorRate >= 1 {
		hints = append(hints, "Focus on server-side errors: check DB/cache/queue latency and saturation.")
	}
	if !math.IsNaN(result.DeltaRPS) && result.DeltaRPS < 0 {
		hints = append(hints, "Traffic dropped vs baseline; verify load balancer, routing, and client errors.")
	}
	if len(result.ActiveAlerts) > 0 {
		hints = append(hints, "Review active Prometheus alerts to narrow the affected component.")
	}
	if cfg.GrafanaURL != "" {
		hints = append(hints, "Use the Grafana Explore link for deeper PromQL analysis and zoomed windows.")
	}
	if cfg.PrometheusQPS > 0 {
		hints = append(hints, fmt.Sprintf("Prometheus QPS limit is %.2f; refresh may be throttled.", cfg.PrometheusQPS))
	}
	return uniqueStrings(hints)
}

func buildLokiCorrelationHints(cfg config.Config, result Result) []string {
	incident := result.SpikeNote != "" ||
		(!math.IsNaN(result.ErrorRate) && result.ErrorRate >= 1) ||
		len(result.ActiveAlerts) > 0
	if !incident {
		return nil
	}
	serviceLabel := firstNonEmpty(cfg.LokiServiceLabel, cfg.ServiceLabel, result.ServiceLabel, "service")
	routeLabel := firstNonEmpty(cfg.LokiRouteLabel, cfg.RouteLabel, result.RouteLabel, "route")
	serviceValue := cfg.APIService
	if strings.TrimSpace(serviceValue) == "" {
		serviceValue = ".+"
	} else {
		serviceValue = escapeRegexLiteral(serviceValue)
	}
	routeValue := normalizeRoute(cfg.APIRoute)
	if routeValue == "" {
		routeValue = ".+"
	} else {
		routeValue = escapeRegexLiteral(routeValue)
	}
	selectorParts := []string{fmt.Sprintf(`%s=~"%s"`, serviceLabel, serviceValue)}
	if strings.TrimSpace(routeLabel) != "" {
		selectorParts = append(selectorParts, fmt.Sprintf(`%s=~"%s"`, routeLabel, routeValue))
	}
	selector := "{" + strings.Join(selectorParts, ",") + "}"
	regex := cfg.LokiErrorRegex
	if strings.TrimSpace(regex) == "" {
		regex = `(?i)(error|exception|5[0-9][0-9])`
	}
	query := fmt.Sprintf(`%s |~ "%s"`, selector, regex)
	labelHint := fmt.Sprintf("Using Loki labels: service=%s, route=%s.", serviceLabel, routeLabel)
	if strings.TrimSpace(routeLabel) == "" {
		labelHint = fmt.Sprintf("Using Loki label: service=%s.", serviceLabel)
	}
	hints := []string{
		labelHint,
		"Suggested LogQL: " + query,
		"If Loki labels differ, set loki_service_label/loki_route_label to match your logs.",
	}
	if strings.TrimSpace(cfg.LokiURL) == "" {
		hints = append(hints, "Configure Loki URL to enable log correlation.")
	}
	return hints
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func escapeRegexLiteral(value string) string {
	replacer := strings.NewReplacer(
		`\\`, `\\\\`,
		`.`, `\\.`,
		`+`, `\\+`,
		`*`, `\\*`,
		`?`, `\\?`,
		`^`, `\\^`,
		`$`, `\\$`,
		`|`, `\\|`,
		`(`, `\\(`,
		`)`, `\\)`,
		`[`, `\\[`,
		`]`, `\\]`,
		`{`, `\\{`,
		`}`, `\\}`,
		`"`, `\\\"`,
	)
	return replacer.Replace(value)
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, v := range values {
		if strings.TrimSpace(v) == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func fetchTopEndpoints(client *prometheus.Client, cfg config.Config, bucketMetric string, labels []string, window string) ([]EndpointLatency, error) {
	routeLabel := cfg.RouteLabel
	if strings.TrimSpace(routeLabel) == "" {
		return nil, prometheus.ErrNoData
	}

	k := clampTopK(cfg.TopEndpoints)
	query := fmt.Sprintf(
		`topk(%d, histogram_quantile(0.95, %s by (le, %s)))`,
		k, buildFallbackQuery(bucketMetric, labels, window), routeLabel,
	)
	samples, err := client.QueryVector(query)
	if err != nil {
		return nil, err
	}

	endpoints := make([]EndpointLatency, 0, len(samples))
	for _, s := range samples {
		route := s.Metric[routeLabel]
		if math.IsNaN(s.Value) {
			continue
		}
		if strings.TrimSpace(route) == "" {
			continue
		}
		endpoints = append(endpoints, EndpointLatency{
			Route: route,
			P95:   s.Value,
		})
	}
	return endpoints, nil
}


func fetchEndpointRegressions(client *prometheus.Client, cfg config.Config, bucketMetric string, labels []string, window string, baselineWindow string) ([]EndpointRegression, string) {
	routeLabel := cfg.RouteLabel
	if strings.TrimSpace(routeLabel) == "" {
		return nil, ""
	}

	nowWindow := window
	if nowWindow == "" {
		nowWindow = "5m"
	}
	baseWindow := baselineWindow
	if baseWindow == "" {
		baseWindow = chooseBaselineWindow(nowWindow)
	}
	k := clampTopK(cfg.TopEndpoints)
	query := fmt.Sprintf(
		`topk(%d, (histogram_quantile(0.95, sum(rate(%s%s[%s])) by (le, %s)) - histogram_quantile(0.95, sum(rate(%s%s[%s])) by (le, %s))))`,
		k, bucketMetric, labels, nowWindow, routeLabel, bucketMetric, labels, baseWindow, routeLabel,
	)
	samples, err := client.QueryVector(query)
	if err != nil {
		return nil, ""
	}
	if len(samples) == 0 {
		return nil, "no regression data found"
	}
	regressions := make([]EndpointRegression, 0, len(samples))
	for _, s := range samples {
		route := s.Metric[routeLabel]
		if strings.TrimSpace(route) == "" || math.IsNaN(s.Value) {
			continue
		}
		regressions = append(regressions, EndpointRegression{
			Route:    route,
			DeltaP95: s.Value,
		})
	}
	if len(regressions) == 0 {
		return nil, "no regression data found"
	}
	// Fill in now/baseline values for display.
	nowQuery := fmt.Sprintf(
		`histogram_quantile(0.95, sum(rate(%s%s[%s])) by (le, %s))`,
		bucketMetric, labels, nowWindow, routeLabel,
	)
	baseQuery := fmt.Sprintf(
		`histogram_quantile(0.95, sum(rate(%s%s[%s])) by (le, %s))`,
		bucketMetric, labels, baseWindow, routeLabel,
	)
	nowSamples, _ := client.QueryVector(nowQuery)
	baseSamples, _ := client.QueryVector(baseQuery)
	nowMap := map[string]float64{}
	baseMap := map[string]float64{}
	for _, s := range nowSamples {
		if route := s.Metric[routeLabel]; strings.TrimSpace(route) != "" {
			nowMap[route] = s.Value
		}
	}
	for _, s := range baseSamples {
		if route := s.Metric[routeLabel]; strings.TrimSpace(route) != "" {
			baseMap[route] = s.Value
		}
	}
	for i := range regressions {
		regressions[i].NowP95 = nowMap[regressions[i].Route]
		regressions[i].BaselineP95 = baseMap[regressions[i].Route]
	}
	return regressions, ""
}

func fetchTopEndpointsRPS(client *prometheus.Client, requestMetric string, routeLabel string, labels []string, window string, topK int) ([]EndpointRate, string, error) {
	if strings.TrimSpace(routeLabel) == "" {
		return nil, "", prometheus.ErrNoData
	}

	windowSeconds := parseWindowSeconds(window)
	k := clampTopK(topK)
	query := fmt.Sprintf(
		`topk(%d, sum(increase(%s)) by (%s) / %d)`,
		k, buildFallbackQuery(requestMetric, labels, window), routeLabel, windowSeconds,
	)

	samples, err := client.QueryVector(query)
	if err != nil {
		return nil, "", err
	}

	endpoints := make([]EndpointRate, 0, len(samples))
	for _, s := range samples {
		route := s.Metric[routeLabel]
		if strings.TrimSpace(route) == "" {
			continue
		}
		endpoints = append(endpoints, EndpointRate{
			Route: route,
			RPS:   s.Value,
		})
	}
	if len(endpoints) == 0 {
		countQuery := fmt.Sprintf(
			`topk(%d, sum(%s%s) by (%s))`,
			k, requestMetric, labels, routeLabel,
		)
		countSamples, err := client.QueryVector(countQuery)
		if err != nil {
			return nil, "", err
		}
		for _, s := range countSamples {
			route := s.Metric[routeLabel]
			if strings.TrimSpace(route) == "" {
				continue
			}
			endpoints = append(endpoints, EndpointRate{
				Route: route,
				RPS:   s.Value,
			})
		}
		if len(endpoints) > 0 {
			return endpoints, "showing total counts (no recent rate data)", nil
		}
		return nil, fmt.Sprintf("no per-endpoint data for %s{%s}", requestMetric, routeLabel), nil
	}
	return endpoints, "", nil
}

func fetchTopServicesRPS(client *prometheus.Client, requestMetric string, serviceLabel string, labels []string, window string, topK int) ([]ServiceRate, string, error) {
	if strings.TrimSpace(serviceLabel) == "" {
		return nil, "", prometheus.ErrNoData
	}
	windowSeconds := parseWindowSeconds(window)
	k := clampTopK(topK)
	query := fmt.Sprintf(
		`topk(%d, sum(increase(%s)) by (%s) / %d)`,
		k, buildFallbackQuery(requestMetric, labels, window), serviceLabel, windowSeconds,
	)
	samples, err := client.QueryVector(query)
	if err != nil {
		return nil, "", err
	}
	services := make([]ServiceRate, 0, len(samples))
	for _, s := range samples {
		name := s.Metric[serviceLabel]
		if strings.TrimSpace(name) == "" {
			continue
		}
		services = append(services, ServiceRate{
			Service: name,
			RPS:     s.Value,
		})
	}
	if len(services) == 0 {
		countQuery := fmt.Sprintf(
			`topk(%d, sum(%s) by (%s))`,
			k, strings.TrimSuffix(buildFallbackQuery(requestMetric, labels, window), "["+window+"]"), serviceLabel,
		)

		countSamples, err := client.QueryVector(countQuery)
		if err != nil {
			return nil, "", err
		}
		for _, s := range countSamples {
			name := s.Metric[serviceLabel]
			if strings.TrimSpace(name) == "" {
				continue
			}
			services = append(services, ServiceRate{
				Service: name,
				RPS:     s.Value,
			})
		}
		if len(services) > 0 {
			return services, "showing total counts (no recent rate data)", nil
		}
		return nil, fmt.Sprintf("no per-service data for %s{%s}", requestMetric, serviceLabel), nil
	}
	return services, "", nil
}

func clampTopK(value int) int {
	if value <= 0 {
		return 5
	}
	if value > 25 {
		return 25
	}
	return value
}

func determineConfidence(cfg config.Config, endpoints []EndpointLatency, endpointsRPS []EndpointRate, servicesRPS []ServiceRate, notes []string) string {
	score := 0
	if cfg.PrometheusURL != "" {
		score += 1
	}
	if cfg.LatencyMetric != "" {
		score += 1
	}
	if cfg.RequestCountMetric != "" {
		score += 1
	}
	if len(endpoints) > 0 {
		score += 2
	}
	if len(endpointsRPS) > 0 {
		score += 2
	}
	if len(servicesRPS) > 0 {
		score += 1
	}
	for _, n := range notes {
		if strings.Contains(n, "no traffic") || strings.Contains(n, "no latency samples") {
			score -= 1
		}
	}
	switch {
	case score >= 6:
		return "high"
	case score >= 3:
		return "medium"
	default:
		return "low"
	}
}

func parseWindowSeconds(window string) int64 {
	if d, err := time.ParseDuration(window); err == nil && d > 0 {
		return int64(d.Seconds())
	}
	return int64((5 * time.Minute).Seconds())
}

func chooseBaselineWindow(window string) string {
	d, err := time.ParseDuration(window)
	if err != nil || d <= 0 {
		return "1h"
	}
	baseline := d * 6
	if baseline < 30*time.Minute {
		baseline = 30 * time.Minute
	}
	if baseline > 6*time.Hour {
		baseline = 6 * time.Hour
	}
	return baseline.String()
}

func deltaValue(current float64, baseline float64) float64 {
	if math.IsNaN(current) || math.IsNaN(baseline) {
		return math.NaN()
	}
	return current - baseline
}

func fetchActiveAlerts(client *prometheus.Client, cfg config.Config) []string {
	if client == nil || client.BaseURL == "" {
		return nil
	}
	selector := `{alertstate="firing"}`
	if cfg.ServiceLabel != "" && cfg.APIService != "" {
		selector = fmt.Sprintf(`{alertstate="firing",%s="%s"}`, cfg.ServiceLabel, cfg.APIService)
	}
	query := fmt.Sprintf(`ALERTS%s`, selector)
	samples, err := client.QueryVector(query)
	if err != nil {
		return nil
	}
	alerts := make([]string, 0, len(samples))
	for _, s := range samples {
		name := s.Metric["alertname"]
		if strings.TrimSpace(name) == "" {
			continue
		}
		severity := s.Metric["severity"]
		if strings.TrimSpace(severity) != "" {
			alerts = append(alerts, fmt.Sprintf("%s (%s)", name, severity))
		} else {
			alerts = append(alerts, name)
		}
		if len(alerts) >= 5 {
			break
		}
	}
	sort.Strings(alerts)
	return alerts
}

func detectSpike(currentP95 float64, baselineP95 float64, currentErr float64, baselineErr float64) string {
	if math.IsNaN(currentP95) || math.IsNaN(baselineP95) {
		return ""
	}
	if baselineP95 <= 0 {
		return ""
	}
	ratio := currentP95 / baselineP95
	if ratio >= 2.0 && currentP95-baselineP95 >= 0.5 {
		return fmt.Sprintf("P95 spike detected (+%s vs baseline)", formatDuration(currentP95-baselineP95))
	}
	if !math.IsNaN(currentErr) && !math.IsNaN(baselineErr) && baselineErr > 0 {
		errRatio := currentErr / baselineErr
		if errRatio >= 2.0 && currentErr-baselineErr >= 1.0 {
			return fmt.Sprintf("Error rate spike detected (+%.2f%% vs baseline)", currentErr-baselineErr)
		}
	}
	return ""
}

func formatDuration(seconds float64) string {
	if math.IsNaN(seconds) {
		return "N/A"
	}
	if seconds < 0 {
		seconds = -seconds
	}
	if seconds < 1 {
		return fmt.Sprintf("%.2fms", seconds*1000)
	}
	return fmt.Sprintf("%.2fs", seconds)
}

func queryWithFallback(client *prometheus.Client, bucketMetric string, reqMetric string, labels string, window string) (float64, float64, float64, float64, string, error) {
	windows := uniqueWindows([]string{window, "30m", "2h"})
	for _, w := range windows {
		p90Q := fmt.Sprintf(`histogram_quantile(0.90, sum(rate(%s%s[%s])) by (le))`, bucketMetric, labels, w)
		p95Q := fmt.Sprintf(`histogram_quantile(0.95, sum(rate(%s%s[%s])) by (le))`, bucketMetric, labels, w)
		p99Q := fmt.Sprintf(`histogram_quantile(0.99, sum(rate(%s%s[%s])) by (le))`, bucketMetric, labels, w)
		totalQ := fmt.Sprintf(`sum(rate(%s%s[%s]))`, reqMetric, labels, w)

		p90, err := client.QueryInstant(p90Q)
		if err != nil {
			continue
		}
		p95, err := client.QueryInstant(p95Q)
		if err != nil {
			continue
		}
		p99, err := client.QueryInstant(p99Q)
		if err != nil {
			continue
		}
		total, err := client.QueryInstant(totalQ)
		if err != nil {
			continue
		}
		return p90, p95, p99, total, w, nil
	}
	return 0, 0, 0, 0, window, prometheus.ErrNoData
}

func uniqueWindows(values []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, v := range values {
		if strings.TrimSpace(v) == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func joinNotes(prefix string, notes []string) string {
	if len(notes) == 0 {
		return prefix
	}
	combined := strings.Join(notes, " • ")
	if prefix == "" {
		return combined
	}
	return prefix + " • " + combined
}

func discoverConfig(client *prometheus.Client, cfg config.Config, window string) (config.Config, string, error) {
	var notes []string

	metric, err := discoverLatencyMetric(client, window)
	if err == nil {
		cfg.LatencyMetric = metric
		notes = append(notes, "latency metric: "+metric)
	} else if cfg.LatencyMetric == "" {
		return cfg, "", err
	} else {
		notes = append(notes, "latency metric: "+cfg.LatencyMetric)
	}

	reqMetric, reason, err := discoverRequestMetric(client, cfg.LatencyMetric, window)
	if err == nil {
		cfg.RequestCountMetric = reqMetric
		notes = append(notes, "request metric: "+reqMetric)
		if routeLabel := firstRouteLabelOnMetric(client, reqMetric); routeLabel != "" {
			notes = append(notes, "request metric has route labels: "+routeLabel)
		}
		if hasAnyLabelOnMetric(client, reqMetric, []string{"status", "status_code", "code", "grpc_code"}) {
			notes = append(notes, "request metric has status/code labels")
		}
		if strings.TrimSpace(reason) != "" {
			notes = append(notes, "request metric choice: "+reason)
		}
	} else if cfg.RequestCountMetric == "" {
		return cfg, "", err
	} else {
		notes = append(notes, "request metric: "+cfg.RequestCountMetric)
	}

	if cfg.ServiceLabel == "" || strings.EqualFold(cfg.ServiceLabel, "auto") ||
		cfg.RouteLabel == "" || strings.EqualFold(cfg.RouteLabel, "auto") {
		metricForLabels := latencyBucketMetric(cfg.LatencyMetric)
		if hasLabelOnMetric(client, cfg.LatencyMetric, "quantile") {
			metricForLabels = cfg.LatencyMetric
		}
		serviceLabel, serviceValues, routeLabel, routeValues, labelNote := discoverCommonLabels(client, metricForLabels, cfg.RequestCountMetric)
		if strings.TrimSpace(labelNote) != "" {
			notes = append(notes, labelNote)
		}
		if (cfg.ServiceLabel == "" || strings.EqualFold(cfg.ServiceLabel, "auto")) && serviceLabel != "" {
			cfg.ServiceLabel = serviceLabel
			notes = append(notes, "service label: "+serviceLabel)
			if cfg.APIService == "" && len(serviceValues) == 1 {
				cfg.APIService = serviceValues[0]
				notes = append(notes, "service: "+serviceValues[0])
			} else if cfg.APIService == "" && len(serviceValues) > 1 {
				notes = append(notes, "multiple services detected; showing all")
			}
		}
		if (cfg.RouteLabel == "" || strings.EqualFold(cfg.RouteLabel, "auto")) && routeLabel != "" {
			cfg.RouteLabel = routeLabel
			notes = append(notes, "route label: "+routeLabel)
			if cfg.APIRoute == "" && len(routeValues) == 1 {
				cfg.APIRoute = routeValues[0]
				notes = append(notes, "route: "+routeValues[0])
			} else if cfg.APIRoute == "" && len(routeValues) > 1 {
				notes = append(notes, "multiple routes detected; showing all")
			}
		}
	}

	return cfg, strings.Join(notes, " • "), nil
}

func discoverLatencyMetric(client *prometheus.Client, window string) (string, error) {
	candidates, err := discoverLatencyCandidates(client, window)
	if err != nil || len(candidates) == 0 {
		return "", errors.New("unable to auto-detect latency metric; set API_LATENCY_METRIC")
	}
	return candidates[0].Metric, nil
}

func discoverRequestMetric(client *prometheus.Client, latencyMetric string, window string) (string, string, error) {
	candidates, err := discoverRequestCandidates(client, latencyMetric, window)
	if err != nil || len(candidates) == 0 {
		return "", "", errors.New("unable to auto-detect request metric; set API_REQUESTS_METRIC")
	}
	return candidates[0].Metric, candidates[0].Reason, nil
}

type metricCandidate struct {
	Metric string
	Score  int
	Reason string
}

func discoverLatencyCandidates(client *prometheus.Client, window string) ([]metricCandidate, error) {
	names, err := client.MetricNames()
	if err != nil {
		return nil, err
	}
	config.DebugLog("APILatency: processing %d metric names for latency candidates", len(names))

	var strictBuckets []string
	var fallbackBuckets []string
	var summaryCandidates []string

	for _, name := range names {
		if strings.HasPrefix(name, "http_client_") || strings.HasPrefix(name, "rpc_client_") {
			continue
		}
		lower := strings.ToLower(name)
		if strings.HasSuffix(name, "_bucket") &&
			(strings.Contains(lower, "latency") || strings.Contains(lower, "duration")) &&
			(strings.Contains(lower, "request") || strings.Contains(lower, "api")) {
			strictBuckets = append(strictBuckets, name)
			continue
		}
		if strings.HasSuffix(name, "_bucket") &&
			(strings.Contains(lower, "latency") || strings.Contains(lower, "duration")) {
			fallbackBuckets = append(fallbackBuckets, name)
			continue
		}
		if !strings.HasSuffix(name, "_bucket") &&
			(strings.Contains(lower, "latency") || strings.Contains(lower, "duration")) &&
			hasLabelOnMetric(client, name, "quantile") {
			summaryCandidates = append(summaryCandidates, name)
		}
	}

	merged := append([]string{}, summaryCandidates...)
	merged = append(merged, strictBuckets...)
	merged = append(merged, fallbackBuckets...)
	if len(merged) == 0 {
		return nil, errors.New("no latency candidates")
	}

	seen := map[string]struct{}{}
	var candidates []string
	for i, name := range merged {
		if i > 50 {
			config.DebugLog("APILatency: limit reached (50 candidates), skipping remainder")
			break
		}
		metric := strings.TrimSuffix(name, "_bucket")
		if _, ok := seen[metric]; ok {
			continue
		}
		seen[metric] = struct{}{}
		candidates = append(candidates, metric)
	}

	config.DebugLog("APILatency: scoring %d merged latency candidates in parallel", len(candidates))
	
	results := make(chan metricCandidate, len(candidates))
	var wg sync.WaitGroup
	
	// Use a semaphore to limit concurrency
	sem := make(chan struct{}, 10)

	for _, metric := range candidates {
		wg.Add(1)
		go func(m string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			score := 0
			lower := strings.ToLower(m)
			if strings.Contains(lower, "request") || strings.Contains(lower, "api") {
				score += 10
			}
			if strings.Contains(lower, "http") {
				score += 5
			}
			if latencyMetricHasData(client, m, window) {
				score += 50
			}
			if hasLabelOnMetric(client, m, "quantile") {
				score += 10
			}
			// Note: name matching for "_bucket" is lost here but we can re-infer
			results <- metricCandidate{
				Metric: m,
				Score:  score,
			}
		}(metric)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var scored []metricCandidate
	for sc := range results {
		scored = append(scored, sc)
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})
	return scored, nil
}

func discoverRequestCandidates(client *prometheus.Client, latencyMetric string, window string) ([]metricCandidate, error) {
	names, err := client.MetricNames()
	if err != nil {
		return nil, err
	}
	config.DebugLog("APILatency: processing %d metric names for request candidates", len(names))

	prefix := strings.TrimSuffix(latencyMetric, "_seconds")
	prefix = strings.TrimSuffix(prefix, "_milliseconds")
	prefix = strings.TrimSuffix(prefix, "_duration")
	prefix = strings.TrimSuffix(prefix, "_latency")

	latencyMetricForLabels := latencyBucketMetric(latencyMetric)
	if hasLabelOnMetric(client, latencyMetric, "quantile") {
		latencyMetricForLabels = latencyMetric
	}

	var candidates []string
	config.DebugLog("APILatency: filtering request candidates")
	for _, name := range names {
		if strings.HasPrefix(name, "http_client_") || strings.HasPrefix(name, "rpc_client_") {
			continue
		}
		if strings.HasPrefix(name, "prometheus_") || strings.HasPrefix(name, "go_") || strings.HasPrefix(name, "process_") {
			continue
		}
		if !(strings.HasSuffix(name, "_total") || strings.HasSuffix(name, "_count")) {
			continue
		}
		lower := strings.ToLower(name)
		if !(strings.Contains(lower, "request") || strings.Contains(lower, "api_requests") || strings.Contains(lower, "http_requests")) {
			continue
		}
		candidates = append(candidates, name)
		if len(candidates) >= 50 {
			config.DebugLog("APILatency: request candidate filtering limit reached (50)")
			break
		}
	}

	config.DebugLog("APILatency: scoring %d request candidates in parallel", len(candidates))
	results := make(chan metricCandidate, len(candidates))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 15) // Slightly higher parallelism for requests

	for _, name := range candidates {
		wg.Add(1)
		go func(n string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if !hasRequestData(client, n, window) {
				return
			}

			score := 0
			lower := strings.ToLower(n)
			if strings.HasSuffix(n, "_total") {
				score += 10
			}
			if prefix != "" && strings.Contains(n, prefix) {
				score += 8
			}
			if strings.Contains(lower, "api_requests") {
				score += 5
			}
			routeLabel := firstRouteLabelOnMetric(client, n)
			hasEndpoint := routeLabel != ""
			if hasEndpoint {
				score += 15
			}
			hasStatus := hasAnyLabelOnMetric(client, n, []string{"status", "status_code", "code", "grpc_code"})
			if hasStatus {
				score += 8
			}
			if shared := sharedRouteLabelOnMetrics(client, latencyMetricForLabels, n); shared != "" {
				score += 20
			}
			if shared := sharedServiceLabelOnMetrics(client, latencyMetricForLabels, n); shared != "" {
				score += 10
			}
			score += labelDiversityScore(client, n)

			reason := buildRequestMetricReason(n, prefix, hasEndpoint, hasStatus)
			results <- metricCandidate{
				Metric: n,
				Score:  score,
				Reason: reason,
			}
		}(name)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var scored []metricCandidate
	for sc := range results {
		scored = append(scored, sc)
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})
	return scored, nil
}

func latencyMetricHasData(client *prometheus.Client, metric string, window string) bool {
	if hasLabelOnMetric(client, metric, "quantile") {
		return hasSummaryData(client, metric, window)
	}
	return hasHistogramData(client, metric, window)
}

func hasSummaryData(client *prometheus.Client, metric string, window string) bool {
	query := fmt.Sprintf(`avg_over_time(%s%s[%s])`, metric, addLabelMatcher("", "quantile", "0.95"), window)
	val, err := client.QueryInstant(query)
	if err != nil {
		return false
	}
	return !math.IsNaN(val) && val > 0
}

func sharedRouteLabelOnMetrics(client *prometheus.Client, metricA string, metricB string) string {
	for _, candidate := range routeLabelCandidates() {
		if hasLabelOnMetric(client, metricA, candidate) && hasLabelOnMetric(client, metricB, candidate) {
			return candidate
		}
	}
	return ""
}

func sharedServiceLabelOnMetrics(client *prometheus.Client, metricA string, metricB string) string {
	for _, candidate := range preferredServiceLabelCandidates() {
		if hasLabelOnMetric(client, metricA, candidate) && hasLabelOnMetric(client, metricB, candidate) {
			return candidate
		}
	}
	if hasLabelOnMetric(client, metricA, "instance") && hasLabelOnMetric(client, metricB, "instance") {
		return "instance"
	}
	return ""
}

func preferredServiceLabelCandidates() []string {
	return []string{"service", "job", "app", "application", "svc", "service_name"}
}

func discoverCommonLabels(client *prometheus.Client, latencyMetric string, requestMetric string) (string, []string, string, []string, string) {
	if strings.TrimSpace(requestMetric) == "" || strings.TrimSpace(latencyMetric) == "" {
		return "", nil, "", nil, ""
	}

	routeLabel := sharedRouteLabelOnMetrics(client, latencyMetric, requestMetric)
	serviceLabel := sharedServiceLabelOnMetrics(client, latencyMetric, requestMetric)
	note := ""

	if routeLabel == "" && firstRouteLabelOnMetric(client, requestMetric) != "" {
		note = "route label found only on request metric; using aggregate latency"
	}
	if serviceLabel == "" && note == "" && hasAnyLabelOnMetric(client, requestMetric, preferredServiceLabelCandidates()) {
		note = "service label found only on request metric; using aggregate latency"
	}
	if routeLabel == "" && serviceLabel == "" && note == "" {
		note = "no shared service/route labels between latency and request metrics; using aggregate latency"
	}

	var serviceValues []string
	var routeValues []string
	if serviceLabel != "" {
		if values, err := FetchLabelValues(client, requestMetric, serviceLabel, time.Hour, 50); err == nil {
			serviceValues = values
		}
	}
	if routeLabel != "" {
		if values, err := FetchLabelValues(client, requestMetric, routeLabel, time.Hour, 50); err == nil {
			routeValues = values
		}
	}
	return serviceLabel, serviceValues, routeLabel, routeValues, note
}

func discoverRequestMetricWithRouteLabel(client *prometheus.Client, latencyMetric string, window string) (string, string, string) {
	names, err := client.MetricNames()
	if err != nil {
		return "", "", ""
	}

	best := ""
	bestLabel := ""
	bestReason := ""
	bestScore := -1

	for _, name := range names {
		if strings.HasPrefix(name, "http_client_") || strings.HasPrefix(name, "rpc_client_") {
			continue
		}
		if strings.HasPrefix(name, "prometheus_") || strings.HasPrefix(name, "go_") || strings.HasPrefix(name, "process_") {
			continue
		}
		if !(strings.HasSuffix(name, "_total") || strings.HasSuffix(name, "_count")) {
			continue
		}
		if !(strings.Contains(name, "request") || strings.Contains(name, "api_requests") || strings.Contains(name, "http_requests")) {
			continue
		}
		if strings.Contains(name, "_latency_") {
			continue
		}
		if !hasRequestData(client, name, window) {
			continue
		}
		label := firstRouteLabelOnMetric(client, name)
		if label == "" {
			continue
		}
		score := 0
		if strings.HasSuffix(name, "_total") {
			score += 10
		}
		if strings.Contains(name, "http_requests_total") {
			score += 8
		}
		if hasAnyLabelOnMetric(client, name, []string{"status", "status_code", "code", "grpc_code"}) {
			score += 5
		}
		score += labelDiversityScore(client, name)
		reason := buildRequestMetricReason(name, "", true, hasAnyLabelOnMetric(client, name, []string{"status", "status_code", "code", "grpc_code"}))
		if score > bestScore {
			bestScore = score
			best = name
			bestLabel = label
			bestReason = reason
		}
	}

	return best, bestLabel, bestReason
}

func buildRequestMetricReason(name string, prefix string, hasRouteLabel bool, hasStatusLabel bool) string {
	reasons := []string{}
	if hasRouteLabel {
		reasons = append(reasons, "has route labels")
	}
	if hasStatusLabel {
		reasons = append(reasons, "has status/code labels")
	}
	if prefix != "" && strings.Contains(name, prefix) {
		reasons = append(reasons, "matches latency metric prefix")
	}
	if strings.HasSuffix(name, "_total") {
		reasons = append(reasons, "counter total")
	}
	if len(reasons) == 0 {
		return ""
	}
	return strings.Join(reasons, ", ")
}

func discoverLabelsByCardinality(client *prometheus.Client, requestMetric string) (string, []string, string, []string, string) {
	series, err := client.Series(requestMetric, time.Hour)
	if err != nil {
		return "", nil, "", nil, ""
	}

	labelValues := map[string]map[string]struct{}{}
	for _, s := range series {
		for label, val := range s {
			if label == "__name__" || label == "le" || val == "" {
				continue
			}
			if labelValues[label] == nil {
				labelValues[label] = map[string]struct{}{}
			}
			labelValues[label][val] = struct{}{}
		}
	}

	if len(labelValues) == 0 {
		return "", nil, "", nil, ""
	}

	preferredServiceLabels := []string{"service", "job", "app", "application", "svc", "service_name"}
	avoidServiceLabels := map[string]struct{}{
		"instance":    {},
		"pod":         {},
		"pod_name":    {},
		"pod_uid":     {},
		"endpoint":    {},
		"route":       {},
		"path":        {},
		"status":      {},
		"status_code": {},
		"code":        {},
		"method":      {},
	}

	var (
		maxLabel string
		maxCount int
		minLabel string
		minCount int
		note     string
	)

	for label, values := range labelValues {
		count := len(values)
		if count == 0 {
			continue
		}
		if maxLabel == "" || count > maxCount {
			maxLabel = label
			maxCount = count
		}
		if count > 1 && (minLabel == "" || count < minCount) {
			minLabel = label
			minCount = count
		}
	}

	routeLabel := ""
	routeValues := []string{}
	if maxLabel != "" && maxCount > 1 {
		routeLabel = maxLabel
		routeValues = mapKeys(labelValues[maxLabel])
	}

	var serviceLabels []string
	if len(serviceLabels) == 0 {
		for _, candidate := range preferredServiceLabels {
			if _, ok := labelValues[candidate]; ok {
				serviceLabels = append(serviceLabels, candidate)
			}
		}
	}
	if len(serviceLabels) == 0 && minLabel != "" && minLabel != routeLabel {
		if _, blocked := avoidServiceLabels[minLabel]; !blocked {
			serviceLabels = append(serviceLabels, minLabel)
		}
	}
	if len(serviceLabels) == 0 {
		if _, ok := labelValues["instance"]; ok {
			serviceLabels = append(serviceLabels, "instance")
			note = "service label fallback: instance (consider adding a stable service label like service/job)"
		}
	}

	serviceLabel := ""
	serviceValues := []string{}
	if len(serviceLabels) > 0 {
		serviceLabel = strings.Join(serviceLabels, ",")
		// Use the first label for discovery values
		serviceValues = mapKeys(labelValues[serviceLabels[0]])
	}

	return serviceLabel, serviceValues, routeLabel, routeValues, note
}


func mapKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func FetchLabelValues(client *prometheus.Client, metric string, label string, lookback time.Duration, limit int) ([]string, error) {
	if client == nil {
		return nil, errors.New("missing prometheus client")
	}
	if strings.TrimSpace(metric) == "" || strings.TrimSpace(label) == "" {
		return nil, prometheus.ErrNoData
	}
	series, err := client.Series(metric, lookback)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	for _, s := range series {
		if v := strings.TrimSpace(s[label]); v != "" {
			seen[v] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return nil, prometheus.ErrNoData
	}
	values := mapKeys(seen)
	sort.Strings(values)
	if limit > 0 && len(values) > limit {
		values = values[:limit]
	}
	return values, nil
}

func isDefaultLatencyMetric(metric string) bool {
	return metric == config.Default().LatencyMetric
}

func isDefaultRequestMetric(metric string) bool {
	return metric == config.Default().RequestCountMetric
}

func metricHasLabels(client *prometheus.Client, metric string) bool {
	series, err := client.Series(metric, time.Hour)
	if err != nil || len(series) == 0 {
		return false
	}
	for _, s := range series {
		for label, val := range s {
			if label == "__name__" || label == "le" || val == "" {
				continue
			}
			return true
		}
	}
	return false
}

func labelDiversityScore(client *prometheus.Client, metric string) int {
	series, err := client.Series(metric, time.Hour)
	if err != nil || len(series) == 0 {
		return 0
	}
	labelValues := map[string]map[string]struct{}{}
	for _, s := range series {
		for label, val := range s {
			if label == "__name__" || label == "le" || val == "" {
				continue
			}
			if labelValues[label] == nil {
				labelValues[label] = map[string]struct{}{}
			}
			labelValues[label][val] = struct{}{}
		}
	}
	maxCount := 0
	for _, values := range labelValues {
		if len(values) > maxCount {
			maxCount = len(values)
		}
	}
	return maxCount
}

func hasLabelOnMetric(client *prometheus.Client, metric string, label string) bool {
	labels := strings.Split(label, ",")
	series, err := client.Series(metric, time.Hour)
	if err != nil || len(series) == 0 {
		return false
	}
	for _, s := range series {
		for _, lbl := range labels {
			if val, ok := s[lbl]; ok && val != "" {
				return true
			}
		}
	}
	return false
}


func discoverErrorLabel(client *prometheus.Client, requestMetric string) string {
	candidates := []string{"status", "code", "status_code", "grpc_code"}
	for _, label := range candidates {
		if hasLabelOnMetric(client, requestMetric, label) {
			return label
		}
	}
	return ""
}

func hasHistogramData(client *prometheus.Client, metric string, window string) bool {
	query := fmt.Sprintf(`sum(rate(%s_bucket[%s]))`, metric, window)
	val, err := client.QueryInstant(query)
	if err != nil {
		return false
	}
	return !math.IsNaN(val) && val > 0
}

func hasRequestData(client *prometheus.Client, metric string, window string) bool {
	query := fmt.Sprintf(`sum(rate(%s[%s]))`, metric, window)
	val, err := client.QueryInstant(query)
	if err != nil {
		return false
	}
	return !math.IsNaN(val) && val > 0
}

func hasHistogramDataWithLabels(client *prometheus.Client, metric string, labels string, window string) bool {
	query := fmt.Sprintf(`sum(rate(%s_bucket%s[%s]))`, metric, labels, window)
	val, err := client.QueryInstant(query)
	if err != nil {
		return false
	}
	return !math.IsNaN(val) && val > 0
}

func validateAutoLabels(client *prometheus.Client, cfg config.Config, window string, note string) (config.Config, string) {
	labelsSlice := buildLabelSelectors(cfg)
	labels := strings.Join(labelsSlice, ",")
	if labels == "" || labels == "{}" || cfg.LatencyMetric == "" {
		return cfg, note
	}
	if hasHistogramDataWithLabels(client, cfg.LatencyMetric, labels, window) {
		return cfg, note
	}

	// Auto-discovered labels produced no data; fall back to aggregate.
	cfg.APIService = ""
	cfg.APIRoute = ""
	cfg.ServiceLabel = ""
	cfg.RouteLabel = ""
	return cfg, joinNotes(note, []string{"auto labels produced no data; using aggregate metrics"})
}

func discoverRouteLabelOnMetric(client *prometheus.Client, metric string) string {
	labelValues := map[string]map[string]struct{}{}
	series, err := client.Series(metric, time.Hour)
	if err != nil {
		return ""
	}
	for _, s := range series {
		for label, val := range s {
			if label == "__name__" || label == "le" || val == "" {
				continue
			}
			if label == "status" || label == "status_code" || label == "code" || label == "grpc_code" || label == "method" || label == "job" || label == "instance" {
				continue
			}
			if labelValues[label] == nil {
				labelValues[label] = map[string]struct{}{}
			}
			labelValues[label][val] = struct{}{}
		}
	}
	if len(labelValues) == 0 {
		return ""
	}
	// Prefer common route labels if present.
	for _, candidate := range routeLabelCandidates() {
		if values, ok := labelValues[candidate]; ok && len(values) > 0 {
			return candidate
		}
	}
	// Fallback to highest-cardinality label.
	maxLabel := ""
	maxCount := 0
	for label, values := range labelValues {
		count := len(values)
		if count > maxCount {
			maxLabel = label
			maxCount = count
		}
	}
	return maxLabel
}

func routeLabelCandidates() []string {
	return []string{
		"endpoint",
		"route",
		"path",
		"path_template",
		"uri",
		"http_route",
		"url",
		"handler",
		"resource",
		"operation",
		"operation_id",
	}
}

func firstRouteLabelOnMetric(client *prometheus.Client, metric string) string {
	for _, candidate := range routeLabelCandidates() {
		if hasLabelOnMetric(client, metric, candidate) {
			return candidate
		}
	}
	return ""
}

func hasAnyLabelOnMetric(client *prometheus.Client, metric string, labels []string) bool {
	for _, label := range labels {
		if hasLabelOnMetric(client, metric, label) {
			return true
		}
	}
	return false
}

func applyStaleDataHint(client *prometheus.Client, metric string, labels string, window string, reasons *[]NoDataReason) {
	if strings.TrimSpace(metric) == "" {
		return
	}
	lastSample, err := lastSampleTimestamp(client, metric, labels)
	if err != nil {
		if isPromAuthError(err) {
			addNoDataReason(reasons, NoDataReason{
				Area:         "Prometheus auth",
				Reason:       "authentication failed while fetching samples",
				Metric:       metric,
				Labels:       labels,
				Window:       window,
				SuggestedFix: "Verify Prometheus credentials (token or basic auth) and retry.",
			})
		} else if errors.Is(err, prometheus.ErrNoData) {
			addNoDataReason(reasons, NoDataReason{
				Area:         "Stale data",
				Reason:       "no samples found for the selected metric",
				Metric:       metric,
				Labels:       labels,
				Window:       window,
				SuggestedFix: "Generate traffic or widen the window.",
			})
		}
		return
	}
	if lastSample <= 0 {
		return
	}
	age := time.Since(time.Unix(int64(lastSample), 0))
	threshold := staleThreshold(window)
	if age > threshold {
		addNoDataReason(reasons, NoDataReason{
			Area:         "Stale data",
			Reason:       fmt.Sprintf("last sample is %s old", age.Round(time.Second)),
			Metric:       metric,
			Labels:       labels,
			Window:       window,
			SuggestedFix: "Check Prometheus scrape health (target down, network, auth) or widen the window.",
		})
	}
}

func lastSampleTimestamp(client *prometheus.Client, metric string, labels string) (float64, error) {
	if strings.TrimSpace(metric) == "" {
		return 0, errors.New("metric required")
	}
	query := fmt.Sprintf(`max(timestamp(%s%s))`, metric, labels)
	return client.QueryInstant(query)
}

func isPromAuthError(err error) bool {
	var httpErr prometheus.HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode == 401 || httpErr.StatusCode == 403
	}
	return false
}

func staleThreshold(window string) time.Duration {
	windowDuration := parseWindowDuration(window)
	minThreshold := 10 * time.Minute
	threshold := 2 * windowDuration
	if threshold < minThreshold {
		threshold = minThreshold
	}
	return threshold
}

func parseWindowDuration(window string) time.Duration {
	if strings.TrimSpace(window) == "" {
		return 5 * time.Minute
	}
	if d, err := time.ParseDuration(window); err == nil && d > 0 {
		return d
	}
	return 5 * time.Minute
}

type cardinalityInfo struct {
	RouteLabel   string
	RouteCount   int
	RouteLimit   int
	ServiceLabel string
	ServiceCount int
	ServiceLimit int
}

func applyCardinalityHints(client *prometheus.Client, metric string, cfg config.Config, labels string, window string, notes *[]string) cardinalityInfo {
	info := cardinalityInfo{}
	routeLabel := cfg.RouteLabel
	if routeLabel == "" && cfg.AutoDiscover {
		routeLabel = firstRouteLabelOnMetric(client, metric)
	}
	if routeLabel != "" && normalizeRoute(cfg.APIRoute) == "" {
		limit := cfg.RouteCardinalityLimit
		if limit <= 0 {
			limit = 500
		}
		if count, ok := estimateLabelCardinality(client, metric, labels, routeLabel, limit, window); ok {
			*notes = append(*notes, fmt.Sprintf("high route cardinality (~%d distinct %s values)", count, routeLabel))
			info.RouteLabel = routeLabel
			info.RouteCount = count
			info.RouteLimit = limit
		}
	}
	serviceLabel := cfg.ServiceLabel
	if serviceLabel == "" && cfg.AutoDiscover {
		serviceLabel = "service"
	}
	if serviceLabel != "" && cfg.APIService == "" {
		limit := cfg.ServiceCardinalityLimit
		if limit <= 0 {
			limit = 200
		}
		if count, ok := estimateLabelCardinality(client, metric, labels, serviceLabel, limit, window); ok {
			*notes = append(*notes, fmt.Sprintf("high service cardinality (~%d distinct %s values)", count, serviceLabel))
			info.ServiceLabel = serviceLabel
			info.ServiceCount = count
			info.ServiceLimit = limit
		}
	}
	return info
}

func estimateLabelCardinality(client *prometheus.Client, metric string, labels string, label string, limit int, window string) (int, bool) {
	if strings.TrimSpace(metric) == "" || strings.TrimSpace(label) == "" {
		return 0, false
	}
	match := buildNameSelector(metric, labels)
	series, err := client.Series(match, parseWindowDuration(window))
	if err != nil || len(series) == 0 {
		return 0, false
	}
	seen := map[string]struct{}{}
	for _, s := range series {
		val := s[label]
		if strings.TrimSpace(val) == "" {
			continue
		}
		seen[val] = struct{}{}
		if limit > 0 && len(seen) >= limit {
			return len(seen), true
		}
	}
	if len(seen) >= limit {
		return len(seen), true
	}
	return 0, false
}

func buildNameSelector(metric string, labels string) string {
	if strings.TrimSpace(metric) == "" {
		return labels
	}
	if strings.TrimSpace(labels) == "" {
		return fmt.Sprintf(`{__name__="%s"}`, metric)
	}
	if strings.HasPrefix(labels, "{") && strings.HasSuffix(labels, "}") {
		trimmed := strings.TrimSuffix(strings.TrimPrefix(labels, "{"), "}")
		if strings.TrimSpace(trimmed) == "" {
			return fmt.Sprintf(`{__name__="%s"}`, metric)
		}
		return fmt.Sprintf(`{__name__="%s",%s}`, metric, trimmed)
	}
	return fmt.Sprintf(`{__name__="%s"}`, metric)
}

func isPromMetricName(name string) bool {
	if strings.TrimSpace(name) == "" {
		return false
	}
	re := regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`)
	return re.MatchString(name)
}

func sanitizePromLabelName(value string, area string, metric string, window string, reasons *[]NoDataReason) string {
	if value == "" {
		return ""
	}
	labels := strings.Split(value, ",")
	re := regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
	var valid []string
	for _, lbl := range labels {
		trimmed := strings.TrimSpace(lbl)
		if re.MatchString(trimmed) {
			valid = append(valid, trimmed)
		}
	}
	if len(valid) > 0 {
		return strings.Join(valid, ",")
	}
	addNoDataReason(reasons, NoDataReason{
		Area:         area,
		Reason:       fmt.Sprintf("invalid label names %q", value),
		Metric:       metric,

		Labels:       "",
		Window:       window,
		SuggestedFix: "Fix the label name or enable auto-discover.",
	})
	return ""
}

func queryLatencyQuantilesWithFallback(client *prometheus.Client, latencyMetric string, labels []string, window string) (float64, float64, float64, string, string, error) {
	windows := uniqueWindows([]string{window, "30m", "2h"})
	
	// Metrics to try for histograms
	baseMetric := strings.TrimSuffix(strings.TrimSpace(latencyMetric), "_bucket")
	histMetrics := []string{baseMetric}
	if baseMetric != "" {
		histMetrics = append(histMetrics, baseMetric)
	}
	histMetrics = append(histMetrics, "http_request_duration_seconds", "request_duration_seconds", "prometheus_http_request_duration_seconds")

	for _, w := range windows {
		for _, hm := range histMetrics {
			if hm == "" { continue }
			bucketMetric := hm
			if !strings.HasSuffix(bucketMetric, "_bucket") {
				bucketMetric += "_bucket"
			}

			var rateParts []string
			for _, lbl := range labels {
				rateParts = append(rateParts, fmt.Sprintf("sum(rate(%s%s[%s])) by (le)", bucketMetric, lbl, w))
			}
			combinedRate := "(" + strings.Join(rateParts, " or ") + ")"

			p90Q := fmt.Sprintf(`histogram_quantile(0.90, %s by (le))`, combinedRate)
			p95Q := fmt.Sprintf(`histogram_quantile(0.95, %s by (le))`, combinedRate)
			p99Q := fmt.Sprintf(`histogram_quantile(0.99, %s by (le))`, combinedRate)

			p90, _ := client.QueryInstant(p90Q)
			p95, err95 := client.QueryInstant(p95Q)
			p99, _ := client.QueryInstant(p99Q)

			if err95 == nil && !math.IsNaN(p95) && p95 > 0 {
				return p90, p95, p99, w, "histogram", nil
			}
		}

		// Try summaries if histograms fail
		summaryMetrics := []string{baseMetric, "http_request_duration_seconds", "request_duration_seconds"}
		for _, sm := range summaryMetrics {
			if sm == "" { continue }
			var p90s, p95s, p99s []string
			for _, lbl := range labels {
				p90s = append(p90s, fmt.Sprintf(`avg_over_time(%s%s[%s])`, sm, addLabelMatcher(lbl, "quantile", "0.90"), w))
				p95s = append(p95s, fmt.Sprintf(`avg_over_time(%s%s[%s])`, sm, addLabelMatcher(lbl, "quantile", "0.95"), w))
				p99s = append(p99s, fmt.Sprintf(`avg_over_time(%s%s[%s])`, sm, addLabelMatcher(lbl, "quantile", "0.99"), w))
			}
			
			p90, _ := client.QueryInstant(strings.Join(p90s, " or "))
			p95, err95 := client.QueryInstant(strings.Join(p95s, " or "))
			p99, _ := client.QueryInstant(strings.Join(p99s, " or "))

			if err95 == nil && !math.IsNaN(p95) && p95 > 0 {
				return p90, p95, p99, w, "summary", nil
			}
		}
	}
	return math.NaN(), math.NaN(), math.NaN(), window, "", prometheus.ErrNoData
}

func queryTotalWithFallback(client *prometheus.Client, reqMetric string, labels []string, window string) (float64, string, error) {
	windows := uniqueWindows([]string{window, "30m", "2h"})
	for _, w := range windows {
		var rateParts []string
		for _, lbl := range labels {
			rateParts = append(rateParts, fmt.Sprintf("sum(rate(%s%s[%s]))", reqMetric, lbl, w))
		}
		totalQ := strings.Join(rateParts, " or ")
		total, err := client.QueryInstant(totalQ)
		if err == nil && !math.IsNaN(total) {
			return total, w, nil
		}
	}
	return 0, window, prometheus.ErrNoData
}


func addLabelMatcher(base string, label string, value string) string {
	if strings.TrimSpace(label) == "" {
		return base
	}
	labelExpr := fmt.Sprintf(`%s="%s"`, label, escapePromLabelValue(value))
	if base == "" {
		return "{" + labelExpr + "}"
	}
	if strings.HasSuffix(base, "}") {
		return strings.TrimSuffix(base, "}") + "," + labelExpr + "}"
	}
	return base
}

func buildDependencyHints(client *prometheus.Client, result Result) []string {
	incident := result.SpikeNote != "" ||
		(!math.IsNaN(result.ErrorRate) && result.ErrorRate >= 1) ||
		len(result.ActiveAlerts) > 0
	if !incident {
		return nil
	}
	names, err := client.MetricNames()
	if err != nil || len(names) == 0 {
		return nil
	}
	hints := []string{}
	if matches := findMetricMatches(names, []string{"db", "sql", "mysql", "postgres", "mongo"}, []string{"duration", "latency", "query"}); len(matches) > 0 {
		hints = append(hints, "Database metrics detected (e.g., "+strings.Join(matches, ", ")+"); check DB latency and saturation.")
	}
	if matches := findMetricMatches(names, []string{"redis", "memcache", "cache"}, []string{"latency", "duration", "ops"}); len(matches) > 0 {
		hints = append(hints, "Cache metrics detected (e.g., "+strings.Join(matches, ", ")+"); check cache latency/evictions.")
	}
	if matches := findMetricMatches(names, []string{"kafka", "rabbitmq", "sqs", "queue"}, []string{"lag", "depth", "latency"}); len(matches) > 0 {
		hints = append(hints, "Queue metrics detected (e.g., "+strings.Join(matches, ", ")+"); check lag and queue depth.")
	}
	if matches := findMetricMatches(names, []string{"http_client", "outbound", "httpclient"}, []string{"duration", "latency", "requests"}); len(matches) > 0 {
		hints = append(hints, "Outbound HTTP client metrics detected (e.g., "+strings.Join(matches, ", ")+"); check dependency call latency.")
	}
	if matches := findMetricMatches(names, []string{"grpc_client"}, []string{"handled", "latency", "requests"}); len(matches) > 0 {
		hints = append(hints, "gRPC client metrics detected (e.g., "+strings.Join(matches, ", ")+"); check downstream gRPC errors/latency.")
	}
	return hints
}

func hasAnyMetric(names []string, tokens []string, suffixes []string) bool {
	for _, name := range names {
		lower := strings.ToLower(name)
		hasToken := false
		for _, token := range tokens {
			if strings.Contains(lower, token) {
				hasToken = true
				break
			}
		}
		if !hasToken {
			continue
		}
		if len(suffixes) == 0 {
			return true
		}
		for _, suffix := range suffixes {
			if strings.Contains(lower, suffix) {
				return true
			}
		}
	}
	return false
}

func findMetricMatches(names []string, tokens []string, suffixes []string) []string {
	var matches []string
	for _, name := range names {
		lower := strings.ToLower(name)
		hasToken := false
		for _, token := range tokens {
			if strings.Contains(lower, token) {
				hasToken = true
				break
			}
		}
		if !hasToken {
			continue
		}
		if len(suffixes) > 0 {
			hasSuffix := false
			for _, suffix := range suffixes {
				if strings.Contains(lower, suffix) {
					hasSuffix = true
					break
				}
			}
			if !hasSuffix {
				continue
			}
		}
		matches = append(matches, name)
		if len(matches) >= 2 {
			break
		}
	}
	return matches
}

func metricExists(names []string, metric string) bool {
	if strings.TrimSpace(metric) == "" {
		return false
	}
	for _, name := range names {
		if name == metric {
			return true
		}
	}
	return false
}

func escapePromLabelValue(value string) string {
	out := strings.ReplaceAll(value, `\`, `\\`)
	out = strings.ReplaceAll(out, `"`, `\"`)
	return out
}

func sanitizeExtraLabelSelectors(value string) (string, string) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", ""
	}
	if len(trimmed) > 1024 {
		return "", "extra label selectors ignored: too long"
	}
	parts := splitLabelMatchers(trimmed)
	if len(parts) == 0 {
		return "", "extra label selectors ignored: invalid format"
	}
	valid := []string{}
	labelMatcherRe := regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*\s*(=|!=|=~|!~)\s*"(?:[^"\\]|\\.)*"$`)
	for _, part := range parts {
		if !labelMatcherRe.MatchString(part) {
			return "", "extra label selectors ignored: invalid format"
		}
		valid = append(valid, part)
	}
	return strings.Join(valid, ","), ""
}

func splitLabelMatchers(value string) []string {
	var parts []string
	var buf strings.Builder
	inQuotes := false
	escaped := false
	for _, r := range value {
		switch {
		case escaped:
			buf.WriteRune(r)
			escaped = false
		case r == '\\':
			buf.WriteRune(r)
			escaped = true
		case r == '"':
			buf.WriteRune(r)
			inQuotes = !inQuotes
		case r == ',' && !inQuotes:
			part := strings.TrimSpace(buf.String())
			if part != "" {
				parts = append(parts, part)
			}
			buf.Reset()
		default:
			buf.WriteRune(r)
		}
	}
	if buf.Len() > 0 {
		part := strings.TrimSpace(buf.String())
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}

func applyTargetHealthHint(client *prometheus.Client, reasons *[]NoDataReason) {
	up, down, err := client.TargetsStatus()
	if err != nil || down == 0 {
		return
	}
	addNoDataReason(reasons, NoDataReason{
		Area:         "Prometheus targets",
		Reason:       fmt.Sprintf("%d targets down, %d up", down, up),
		SuggestedFix: "Check Prometheus targets page and scrape configs for failing jobs.",
	})
}

func buildFallbackQuery(metric string, labels []string, window string) string {
	if len(labels) == 0 {
		if strings.Contains(metric, "{") {
			return fmt.Sprintf(`%s[%s]`, metric, window)
		}
		return fmt.Sprintf(`%s{}[%s]`, metric, window)
	}
	var queries []string
	for _, l := range labels {
		queries = append(queries, fmt.Sprintf(`%s%s[%s]`, metric, l, window))
	}
	return strings.Join(queries, " or ")
}



