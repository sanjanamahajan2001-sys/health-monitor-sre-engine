package apm

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"health-monitor/internal/analyse/prometheus"
	"health-monitor/internal/config"
)

type Result struct {
	Edges                 []Edge
	RankedCauses          []Cause
	SuspectedUpstream     string
	CorrelationConfidence string
	Evidence              []string
	ReasoningSteps        []string
	NextChecks            []string
	PossibleFixes         []string
	NoDataReasons         []NoDataReason
	Note                  string
	Window                string
	Metric                string
	SourceLabel           string
	DestinationLabel      string
	RouteLabel            string
}

type Edge struct {
	Source        string
	Destination   string
	Route         string
	P95           float64
	BaselineP95   float64
	DeltaP95      float64
	RPS           float64
	ErrorRate     float64
	ConfidenceTag string
}

type Cause struct {
	Path       string
	Score      float64
	Confidence string
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
	result := Result{
		Edges:         []Edge{},
		Evidence:      []string{},
		NoDataReasons: []NoDataReason{},
	}
	window := strings.TrimSpace(cfg.APMWindow)
	if window == "" {
		window = "10m"
	}
	result.Window = window

	metric, note, noData := detectDependencyMetric(client, cfg)
	if noData != nil {
		result.NoDataReasons = append(result.NoDataReasons, *noData)
		result.Note = note
		return result, nil
	}
	result.Metric = metric

	sourceLabel, destLabel, routeLabel, labelNote := detectDependencyLabels(client, cfg, metric)
	if labelNote != "" {
		note = joinNotes(note, []string{labelNote})
	}
	if destLabel == "" {
		result.NoDataReasons = append(result.NoDataReasons, NoDataReason{
			Area:         "Dependency metrics",
			Reason:       "destination label not detected",
			Metric:       metric,
			Labels:       "",
			Window:       window,
			SuggestedFix: "Set APM_DEST_LABEL or configure OpenTelemetry client metrics with destination labels.",
		})
		result.Note = note
		return result, nil
	}
	result.SourceLabel = sourceLabel
	result.DestinationLabel = destLabel
	result.RouteLabel = routeLabel
	if strings.TrimSpace(cfg.ServiceLabel) != "" && cfg.ServiceLabel != sourceLabel && cfg.ServiceLabel != destLabel {
		note = joinNotes(note, []string{fmt.Sprintf("APM labels differ from service_label (%s); set APM_SOURCE_LABEL/APM_DEST_LABEL to align", cfg.ServiceLabel)})
	}

	seriesMetric := metric + "_bucket"
	series, err := client.Series(seriesMetric, time.Hour)
	if err != nil {
		series, err = client.Series(metric, time.Hour)
	}
	if err == nil {
		if limit := apmCardinalityLimit(cfg); limit > 0 {
			count := estimateLabelCardinality(series, destLabel, sourceLabel, cfg.APIService)
			if count >= limit {
				result.NoDataReasons = append(result.NoDataReasons, NoDataReason{
					Area:         "Dependency metrics",
					Reason:       fmt.Sprintf("destination label has %d values (limit %d)", count, limit),
					Metric:       metric,
					Labels:       "",
					Window:       window,
					SuggestedFix: "Reduce cardinality or raise APM_CARDINALITY_LIMIT.",
				})
				result.Note = joinNotes(note, []string{"dependency cardinality limit exceeded; skipping per-edge queries"})
				return result, nil
			}
		}
	}

	selector := buildDependencySelector(metric, sourceLabel, destLabel, routeLabel, cfg)
	groupLabels := buildGroupLabels(sourceLabel, destLabel, routeLabel, cfg)
	if fallbackSelector, fallbackGroup, fallbackNote := applyCardinalityFallback(series, selector, groupLabels, destLabel, routeLabel, cfg); fallbackNote != "" {
		selector = fallbackSelector
		groupLabels = fallbackGroup
		note = joinNotes(note, []string{fallbackNote})
	}
	p95Query := fmt.Sprintf(`histogram_quantile(0.95, sum(rate(%s_bucket%s[%s])) by (le,%s))`, metric, selector, window, groupLabels)
	samples, err := client.QueryVector(p95Query)
	if err != nil {
		if errors.Is(err, prometheus.ErrNoData) {
			result.NoDataReasons = append(result.NoDataReasons, NoDataReason{
				Area:         "Dependency metrics",
				Reason:       "no dependency latency series returned",
				Metric:       metric,
				Labels:       selector,
				Window:       window,
				SuggestedFix: "Verify client span metrics exist and labels match; increase APM_WINDOW or generate traffic.",
			})
			result.Note = note
			return result, nil
		}
		return Result{}, err
	}

	baselineWindow := baselineWindow(window)
	baselineQuery := fmt.Sprintf(`histogram_quantile(0.95, sum(rate(%s_bucket%s[%s])) by (le,%s))`, metric, selector, baselineWindow, groupLabels)
	baselineSamples, _ := client.QueryVector(baselineQuery)
	baselineMap := mapEdgeValues(baselineSamples, sourceLabel, destLabel, routeLabel, cfg)

	rpsQuery := fmt.Sprintf(`sum(rate(%s_count%s[%s])) by (%s)`, metric, selector, window, groupLabels)
	rpsSamples, _ := client.QueryVector(rpsQuery)
	rpsMap := mapEdgeValues(rpsSamples, sourceLabel, destLabel, routeLabel, cfg)

	errorRateMap := map[string]float64{}
	if statusLabel := detectStatusLabel(series); statusLabel != "" {
		errorSelector := selector + fmt.Sprintf(`,%s=~"5.."`, statusLabel)
		errorQuery := fmt.Sprintf(`sum(rate(%s_count%s[%s])) by (%s)`, metric, errorSelector, window, groupLabels)
		errorSamples, _ := client.QueryVector(errorQuery)
		errorMap := mapEdgeValues(errorSamples, sourceLabel, destLabel, routeLabel, cfg)
		for key, errRate := range errorMap {
			if total, ok := rpsMap[key]; ok && total > 0 {
				errorRateMap[key] = (errRate / total) * 100
			}
		}
	}

	edges := make([]Edge, 0, len(samples))
	for _, sample := range samples {
		src := sample.Metric[sourceLabel]
		dest := sample.Metric[destLabel]
		route := ""
		if routeLabel != "" {
			route = sample.Metric[routeLabel]
		}
		key := edgeKey(src, dest, route)
		base := baselineMap[key]
		delta := sample.Value - base
		edge := Edge{
			Source:      src,
			Destination: dest,
			Route:       route,
			P95:         sample.Value,
			BaselineP95: base,
			DeltaP95:    delta,
			RPS:         rpsMap[key],
			ErrorRate:   errorRateMap[key],
		}
		edges = append(edges, edge)
	}

	if len(edges) > 1 {
		sort.Slice(edges, func(i, j int) bool {
			a := edges[i]
			b := edges[j]
			if !math.IsNaN(a.DeltaP95) && !math.IsNaN(b.DeltaP95) && a.DeltaP95 != b.DeltaP95 {
				return a.DeltaP95 > b.DeltaP95
			}
			return a.P95 > b.P95
		})
	}

	var truncated bool
	edges, truncated = pruneEdges(edges, cfg.APMTopEdges)
	result.Edges = edges
	if truncated {
		note = joinNotes(note, []string{fmt.Sprintf("graph truncated to top %d edges", cfg.APMTopEdges)})
	}
	result.Note = note

	if len(edges) > 0 {
		minRPS := cfg.APMMinRPS
		if minRPS < 0 {
			minRPS = 0
		}
		allowHigh := hasBaseline(edges)
		ranked := rankCauses(edges, minRPS, allowHigh)
		result.RankedCauses = ranked
		if len(ranked) > 0 {
			result.SuspectedUpstream = ranked[0].Path
			result.CorrelationConfidence = ranked[0].Confidence
		}
		topEdge := selectSuspectedEdge(edges, cfg.APIService)
		if len(ranked) > 0 {
			if rankedEdge, ok := edgeForPath(edges, ranked[0].Path); ok {
				topEdge = rankedEdge
			}
		}
		if minRPS > 0 && topEdge.RPS < minRPS {
			result.NoDataReasons = append(result.NoDataReasons, NoDataReason{
				Area:         "Dependency metrics",
				Reason:       fmt.Sprintf("insufficient traffic for root-cause inference (RPS %.2f < %.2f)", topEdge.RPS, minRPS),
				Metric:       metric,
				Labels:       selector,
				Window:       window,
				SuggestedFix: "Increase dependency traffic or lower APM_MIN_RPS to allow inference.",
			})
			result.ReasoningSteps = uniqueStrings([]string{
				"Dependency traffic is below the minimum threshold; skipping root-cause inference.",
			})
			result.NextChecks = uniqueStrings([]string{
				"Increase traffic or widen the APM window to improve confidence.",
				"Verify dependency metrics in Prometheus to confirm active edges.",
			})
			return result, nil
		}
		secondHop := selectSecondHop(edges, topEdge)
		chain := formatSuspectedChain(topEdge, secondHop)
		if chain != "" {
			result.SuspectedUpstream = chain
			confidence, evidence := scoreChain(topEdge, secondHop, minRPS, allowHigh)
			result.CorrelationConfidence = confidence
			result.Evidence = uniqueStrings(evidence)
			steps, checks, fixes := buildReasoning(topEdge, secondHop, confidence, minRPS)
			result.ReasoningSteps = uniqueStrings(steps)
			result.NextChecks = uniqueStrings(checks)
			result.PossibleFixes = uniqueStrings(fixes)
		}
	}

	return result, nil
}

func detectDependencyMetric(client *prometheus.Client, cfg config.Config) (string, string, *NoDataReason) {
	metric := strings.TrimSpace(cfg.APMDependencyMetric)
	note := ""
	metricNames, _ := client.MetricNames()
	if metric != "" {
		base := normalizeHistogramBase(metric)
		if metricExists(metricNames, base) || metricExists(metricNames, base+"_bucket") {
			if base != metric {
				note = joinNotes(note, []string{"dependency metric normalized: " + base})
			}
			return base, note, nil
		}
		return "", note, &NoDataReason{
			Area:         "Dependency metrics",
			Reason:       fmt.Sprintf("metric %q not found in Prometheus", metric),
			Metric:       metric,
			Labels:       "",
			Window:       strings.TrimSpace(cfg.APMWindow),
			SuggestedFix: "Set APM_DEPENDENCY_METRIC to an existing client histogram metric or enable auto-detect.",
		}
	}

	candidates := []string{
		"http_client_duration_seconds",
		"http_client_request_duration_seconds",
		"http_client_duration_milliseconds",
		"rpc_client_duration_seconds",
		"istio_request_duration_seconds",
	}
	for _, candidate := range candidates {
		if metricExists(metricNames, candidate) || metricExists(metricNames, candidate+"_bucket") {
			tag := "dependency metric auto-detect: " + candidate
			if strings.HasPrefix(candidate, "istio_") {
				tag = "dependency metric auto-detect (istio): " + candidate
			} else if strings.HasPrefix(candidate, "rpc_client_") {
				tag = "dependency metric auto-detect (grpc): " + candidate
			}
			return candidate, joinNotes(note, []string{tag}), nil
		}
	}
	return "", note, &NoDataReason{
		Area:         "Dependency metrics",
		Reason:       "no dependency latency metrics detected",
		Metric:       "",
		Labels:       "",
		Window:       strings.TrimSpace(cfg.APMWindow),
		SuggestedFix: "Expose OpenTelemetry http_client_duration_seconds histogram or set APM_DEPENDENCY_METRIC.",
	}
}

func detectDependencyLabels(client *prometheus.Client, cfg config.Config, metric string) (string, string, string, string) {
	sourceLabel := strings.TrimSpace(cfg.APMSourceLabel)
	destLabel := strings.TrimSpace(cfg.APMDestinationLabel)
	routeLabel := strings.TrimSpace(cfg.APMRouteLabel)
	note := ""

	seriesMetric := metric + "_bucket"
	series, err := client.Series(seriesMetric, time.Hour)
	if err != nil {
		series, err = client.Series(metric, time.Hour)
	}
	if err != nil {
		return sourceLabel, destLabel, routeLabel, note
	}
	labelValues := collectLabelValues(series)

	if sourceLabel != "" {
		if _, ok := labelValues[sourceLabel]; !ok {
			note = joinNotes(note, []string{fmt.Sprintf("configured APM_SOURCE_LABEL %q not found; auto-detecting", sourceLabel)})
			sourceLabel = ""
		}
	}
	if destLabel != "" {
		if _, ok := labelValues[destLabel]; !ok {
			note = joinNotes(note, []string{fmt.Sprintf("configured APM_DEST_LABEL %q not found; auto-detecting", destLabel)})
			destLabel = ""
		}
	}
	if routeLabel != "" {
		if _, ok := labelValues[routeLabel]; !ok {
			note = joinNotes(note, []string{fmt.Sprintf("configured APM_ROUTE_LABEL %q not found; auto-detecting", routeLabel)})
			routeLabel = ""
		}
	}

	if destLabel == "" {
		destCandidates := []string{
			"destination_service", "destination_service_name", "destination_workload", "destination_app",
			"peer", "peer_service", "net_peer_name", "server_address", "server", "service", "upstream", "dest",
			"rpc_service", "rpc_peer",
			"destination_workload_namespace", "destination_service_namespace", "destination_service_name",
		}
		destLabel = selectBestLabel(labelValues, destCandidates)
		if destLabel != "" {
			note = joinNotes(note, []string{"dest label: " + destLabel})
		}
	}
	if sourceLabel == "" {
		sourceCandidates := []string{
			"source_service", "source_service_name", "source_workload", "source_app",
			"service", "service_name", "job", "app", "client", "peer", "k8s_app", "source",
			"source_workload_namespace", "source_service_namespace",
		}
		sourceLabel = selectBestLabel(labelValues, sourceCandidates)
		if sourceLabel != "" {
			note = joinNotes(note, []string{"source label: " + sourceLabel})
		}
	}
	if routeLabel == "" {
		routeCandidates := []string{"http_route", "route", "endpoint", "path", "rpc_method", "rpc_route", "rpc_operation", "request_url", "request_protocol", "destination_service"}
		routeLabel = selectBestLabel(labelValues, routeCandidates)
		if routeLabel != "" {
			note = joinNotes(note, []string{"route label: " + routeLabel})
		}
	}
	return sourceLabel, destLabel, routeLabel, note
}

func collectLabelValues(series []map[string]string) map[string]map[string]struct{} {
	values := map[string]map[string]struct{}{}
	for _, s := range series {
		for k, v := range s {
			if k == "__name__" || k == "le" || v == "" {
				continue
			}
			if values[k] == nil {
				values[k] = map[string]struct{}{}
			}
			values[k][v] = struct{}{}
		}
	}
	return values
}

func firstLabelMatch(labels map[string]struct{}, candidates []string) string {
	for _, candidate := range candidates {
		if _, ok := labels[candidate]; ok {
			return candidate
		}
	}
	return ""
}

func selectBestLabel(values map[string]map[string]struct{}, candidates []string) string {
	best := ""
	bestScore := -1
	for _, candidate := range candidates {
		vals, ok := values[candidate]
		if !ok {
			continue
		}
		count := len(vals)
		if count == 0 {
			continue
		}
		score := 10
		if count == 1 {
			score += 2
		} else if count < 50 {
			score += 5
		}
		if score > bestScore {
			bestScore = score
			best = candidate
		}
	}
	return best
}

func detectStatusLabel(series []map[string]string) string {
	if len(series) == 0 {
		return ""
	}
	labels := map[string]struct{}{}
	for _, s := range series {
		for k := range s {
			labels[k] = struct{}{}
		}
	}
	candidates := []string{"status", "status_code", "http_status_code", "grpc_status_code", "grpc_code"}
	return firstLabelMatch(labels, candidates)
}

func normalizeHistogramBase(metric string) string {
	trimmed := strings.TrimSpace(metric)
	if strings.HasSuffix(trimmed, "_bucket") {
		return strings.TrimSuffix(trimmed, "_bucket")
	}
	if strings.HasSuffix(trimmed, "_count") {
		return strings.TrimSuffix(trimmed, "_count")
	}
	return trimmed
}

func metricExists(names []string, metric string) bool {
	for _, name := range names {
		if name == metric {
			return true
		}
	}
	return false
}

func apmCardinalityLimit(cfg config.Config) int {
	if cfg.APMCardinalityLimit > 0 {
		return cfg.APMCardinalityLimit
	}
	return 200
}

func estimateLabelCardinality(series []map[string]string, label string, sourceLabel string, sourceValue string) int {
	seen := map[string]struct{}{}
	for _, item := range series {
		if sourceLabel != "" && strings.TrimSpace(sourceValue) != "" {
			if item[sourceLabel] != sourceValue {
				continue
			}
		}
		value := item[label]
		if value == "" {
			continue
		}
		seen[value] = struct{}{}
	}
	return len(seen)
}

func applyCardinalityFallback(series []map[string]string, selector string, groupLabels string, destLabel string, routeLabel string, cfg config.Config) (string, string, string) {
	limit := apmCardinalityLimit(cfg)
	if limit <= 0 {
		return selector, groupLabels, ""
	}
	if routeLabel != "" && strings.TrimSpace(cfg.APIRoute) == "" {
		routeCount := estimateLabelCardinality(series, routeLabel, "", "")
		if routeCount >= limit {
			groupLabels = removeLabel(groupLabels, routeLabel)
			return selector, groupLabels, "APM fallback: dropped route label due to high cardinality"
		}
	}
	destCount := estimateLabelCardinality(series, destLabel, "", "")
	if destCount >= limit {
		groupLabels = destLabel
		return selector, groupLabels, "APM fallback: aggregated by destination only"
	}
	return selector, groupLabels, ""
}

func removeLabel(groupLabels string, label string) string {
	parts := strings.Split(groupLabels, ",")
	keep := []string{}
	for _, part := range parts {
		if strings.TrimSpace(part) == "" || strings.TrimSpace(part) == label {
			continue
		}
		keep = append(keep, strings.TrimSpace(part))
	}
	return strings.Join(keep, ",")
}

func buildDependencySelector(metric string, sourceLabel string, destLabel string, routeLabel string, cfg config.Config) string {
	parts := []string{}
	if sourceLabel != "" && strings.TrimSpace(cfg.APIService) != "" {
		parts = append(parts, fmt.Sprintf(`%s="%s"`, sourceLabel, escapeLabelValue(cfg.APIService)))
	}
	if routeLabel != "" && strings.TrimSpace(cfg.APIRoute) != "" {
		parts = append(parts, fmt.Sprintf(`%s="%s"`, routeLabel, escapeLabelValue(cfg.APIRoute)))
	}
	if len(parts) == 0 {
		return "{}"
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func buildGroupLabels(sourceLabel string, destLabel string, routeLabel string, cfg config.Config) string {
	labels := []string{}
	if sourceLabel != "" {
		labels = append(labels, sourceLabel)
	}
	if destLabel != "" {
		labels = append(labels, destLabel)
	}
	if routeLabel != "" && strings.TrimSpace(cfg.APIRoute) != "" {
		labels = append(labels, routeLabel)
	}
	return strings.Join(labels, ",")
}

func escapeLabelValue(value string) string {
	replacer := strings.NewReplacer(`\\`, `\\\\`, `"`, `\"`)
	return replacer.Replace(strings.TrimSpace(value))
}

func baselineWindow(window string) string {
	parsed, err := time.ParseDuration(window)
	if err != nil || parsed <= 0 {
		return "30m"
	}
	target := parsed * 6
	if target < 30*time.Minute {
		target = 30 * time.Minute
	}
	if target > 6*time.Hour {
		target = 6 * time.Hour
	}
	return target.String()
}

func mapEdgeValues(samples []prometheus.Sample, sourceLabel string, destLabel string, routeLabel string, cfg config.Config) map[string]float64 {
	values := map[string]float64{}
	for _, sample := range samples {
		src := sample.Metric[sourceLabel]
		dest := sample.Metric[destLabel]
		route := ""
		if routeLabel != "" && strings.TrimSpace(cfg.APIRoute) != "" {
			route = sample.Metric[routeLabel]
		}
		values[edgeKey(src, dest, route)] = sample.Value
	}
	return values
}

func edgeKey(source string, dest string, route string) string {
	return source + "->" + dest + "::" + route
}

func pruneEdges(edges []Edge, limit int) ([]Edge, bool) {
	if limit <= 0 || len(edges) <= limit {
		return edges, false
	}
	sort.Slice(edges, func(i, j int) bool {
		a := edges[i]
		b := edges[j]
		if !math.IsNaN(a.DeltaP95) && !math.IsNaN(b.DeltaP95) && a.DeltaP95 != b.DeltaP95 {
			return a.DeltaP95 > b.DeltaP95
		}
		return a.P95 > b.P95
	})
	return edges[:limit], true
}

func selectSuspectedEdge(edges []Edge, service string) Edge {
	if strings.TrimSpace(service) == "" {
		return edges[0]
	}
	candidates := []Edge{}
	for _, edge := range edges {
		if edge.Source == service {
			candidates = append(candidates, edge)
		}
	}
	if len(candidates) == 0 {
		return edges[0]
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].DeltaP95 > candidates[j].DeltaP95
	})
	return candidates[0]
}

func formatSuspected(edge Edge) string {
	if strings.TrimSpace(edge.Source) == "" || strings.TrimSpace(edge.Destination) == "" {
		return ""
	}
	if strings.TrimSpace(edge.Route) != "" {
		return fmt.Sprintf("%s -> %s (%s)", edge.Source, edge.Destination, edge.Route)
	}
	return fmt.Sprintf("%s -> %s", edge.Source, edge.Destination)
}

func edgeForPath(edges []Edge, path string) (Edge, bool) {
	for _, edge := range edges {
		if formatSuspected(edge) == path {
			return edge, true
		}
	}
	return Edge{}, false
}

func scoreEdge(edge Edge, minRPS float64, allowHigh bool) (string, []string) {
	evidence := []string{}
	confidence := "low"
	if !math.IsNaN(edge.DeltaP95) && edge.DeltaP95 > 0 {
		evidence = append(evidence, fmt.Sprintf("Edge P95 increased by %.2fs vs baseline.", edge.DeltaP95))
		if edge.DeltaP95 > 1 || (edge.BaselineP95 > 0 && edge.DeltaP95/edge.BaselineP95 > 0.2) {
			confidence = "medium"
		}
	}
	if edge.ErrorRate > 1 {
		evidence = append(evidence, fmt.Sprintf("Edge error rate is %.2f%%.", edge.ErrorRate))
		if allowHigh {
			confidence = "high"
		} else {
			confidence = "medium"
		}
	}
	if edge.RPS > 0 {
		evidence = append(evidence, fmt.Sprintf("Edge RPS is %.2f.", edge.RPS))
	}
	if edge.RPS < minRPS {
		evidence = append(evidence, fmt.Sprintf("Edge RPS below threshold (%.2f < %.2f).", edge.RPS, minRPS))
		confidence = "low"
	}
	if len(evidence) == 0 {
		evidence = append(evidence, "Dependency edge shows elevated latency.")
	}
	return confidence, evidence
}

func selectSecondHop(edges []Edge, first Edge) Edge {
	if strings.TrimSpace(first.Destination) == "" {
		return Edge{}
	}
	candidates := []Edge{}
	for _, edge := range edges {
		if edge.Source == first.Destination {
			candidates = append(candidates, edge)
		}
	}
	if len(candidates) == 0 {
		return Edge{}
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].DeltaP95 > candidates[j].DeltaP95
	})
	return candidates[0]
}

func formatSuspectedChain(first Edge, second Edge) string {
	if second.Source != "" && second.Destination != "" {
		return fmt.Sprintf("%s -> %s -> %s", first.Source, first.Destination, second.Destination)
	}
	return formatSuspected(first)
}

func scoreChain(first Edge, second Edge, minRPS float64, allowHigh bool) (string, []string) {
	confidence, evidence := scoreEdge(first, minRPS, allowHigh)
	if second.Source == "" || second.Destination == "" {
		return confidence, evidence
	}
	secondConfidence, secondEvidence := scoreEdge(second, minRPS, allowHigh)
	evidence = append(evidence, secondEvidence...)
	if secondConfidence == "high" {
		return "high", evidence
	}
	if secondConfidence == "medium" && confidence != "high" {
		return "medium", evidence
	}
	return confidence, evidence
}

func rankCauses(edges []Edge, minRPS float64, allowHigh bool) []Cause {
	candidates := []Cause{}
	for _, edge := range edges {
		score := edgeScore(edge, minRPS)
		if score <= 0 {
			continue
		}
		candidates = append(candidates, Cause{
			Path:       formatSuspected(edge),
			Score:      score,
			Confidence: confidenceFromScore(score, allowHigh),
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})
	if len(candidates) > 3 {
		candidates = candidates[:3]
	}
	return candidates
}

func edgeScore(edge Edge, minRPS float64) float64 {
	score := 0.0
	if edge.RPS < minRPS {
		return 0
	}
	if edge.DeltaP95 > 0 {
		score += edge.DeltaP95
	}
	if edge.ErrorRate > 0 {
		score += edge.ErrorRate / 10
	}
	if edge.RPS > 0 {
		score += math.Min(edge.RPS, 10) / 10
	}
	return score
}

func confidenceFromScore(score float64, allowHigh bool) string {
	if score >= 2.0 {
		if allowHigh {
			return "high"
		}
		return "medium"
	}
	if score >= 0.5 {
		return "medium"
	}
	return "low"
}

func hasBaseline(edges []Edge) bool {
	for _, edge := range edges {
		if edge.BaselineP95 > 0 {
			return true
		}
	}
	return false
}

func buildReasoning(first Edge, second Edge, confidence string, minRPS float64) ([]string, []string, []string) {
	steps := []string{}
	next := []string{}
	fixes := []string{}

	if strings.TrimSpace(first.Source) != "" && strings.TrimSpace(first.Destination) != "" {
		steps = append(steps, fmt.Sprintf("Primary dependency edge %s -> %s selected based on latency and traffic signals.", first.Source, first.Destination))
	}
	if first.DeltaP95 > 0 {
		steps = append(steps, fmt.Sprintf("Edge P95 increased by %.2fs vs baseline.", first.DeltaP95))
	} else {
		steps = append(steps, "Edge P95 did not increase vs baseline; confidence remains low.")
	}
	if first.ErrorRate > 1 {
		steps = append(steps, fmt.Sprintf("Edge error rate is %.2f%%.", first.ErrorRate))
	}
	if first.RPS < minRPS {
		steps = append(steps, fmt.Sprintf("Traffic is low (RPS %.2f < %.2f); results may be noisy.", first.RPS, minRPS))
	}
	if second.Source != "" && second.Destination != "" {
		steps = append(steps, fmt.Sprintf("Second-hop edge %s -> %s also considered for root cause.", second.Source, second.Destination))
		if second.DeltaP95 > 0 {
			steps = append(steps, fmt.Sprintf("Second-hop P95 increased by %.2fs vs baseline.", second.DeltaP95))
		}
	}

	next = append(next, "Check upstream service logs around the spike window.")
	next = append(next, "Verify dependency latency metrics in Grafana for the suspected edge.")
	next = append(next, "Confirm baseline window has sufficient traffic.")

	if first.ErrorRate > 1 {
		fixes = append(fixes, "Investigate error patterns (timeouts, 5xx, circuit breakers) between services.")
		fixes = append(fixes, "Check dependency health: DB/queue/cache saturation, error budgets, and retries.")
	} else if first.DeltaP95 > 0 {
		fixes = append(fixes, "Check latency sources: DB slow queries, cache misses, network egress latency.")
		fixes = append(fixes, "Review recent deploys or config changes affecting upstream services.")
	} else {
		fixes = append(fixes, "Increase traffic or widen the window to improve confidence.")
	}

	if confidence == "high" {
		fixes = append(fixes, "Mitigate by scaling or throttling the upstream dependency temporarily.")
	}

	return steps, next, fixes
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
