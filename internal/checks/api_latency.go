package checks

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"health-monitor/internal/analyse/api_latency"
	"health-monitor/internal/analyse/apm"
	"health-monitor/internal/analyse/logs"
	"health-monitor/internal/analyse/prometheus"
	"health-monitor/internal/analyse/tracing"
	"health-monitor/internal/config"
	"health-monitor/pkg/model"
)

func APILatency(report *model.Report) {
	defer func() {
		if r := recover(); r != nil {
			config.DebugLog("PANIC in APILatency: %v", r)
			report.APINote = fmt.Sprintf("Internal error: periodic check crashed (%v)", r)
		}
	}()

	config.DebugLog("APILatency: starting check")
	cfg, err := config.Load()
	if err != nil {
		config.DebugLog("APILatency: config load failed: %v", err)
		report.APINote = fmt.Sprintf("API latency check disabled: unable to load config: %v", err)
		return
	}

	path, _ := config.ConfigPath()
	report.APIConfig = buildConfigSummary(cfg, path)

	missing := config.MissingRequired(cfg)
	if len(missing) > 0 {
		config.DebugLog("APILatency: missing required config: %v", missing)
		report.APINote = missingConfigMessage(missing)
		// Only block on critical missing fields (Prometheus URL)
		if contains(missing, "PROMETHEUS_URL") {
			return
		}
	}

	client := prometheus.Client{
		BaseURL: cfg.PrometheusURL,
		Token:   cfg.PrometheusToken,
		User:    cfg.PrometheusUser,
		Pass:    cfg.PrometheusPass,
		Timeout: 6 * time.Second,
		QPS:     cfg.PrometheusQPS,
	}

	config.DebugLog("APILatency: collecting metrics from %s (with 30s safety timeout)", cfg.PrometheusURL)
	
	type collectRes struct {
		res api_latency.Result
		err error
	}
	ch := make(chan collectRes, 1)

	go func() {
		res, err := api_latency.Collect(&client, cfg)
		ch <- collectRes{res, err}
	}()

	var result api_latency.Result
	select {
	case cr := <-ch:
		result = cr.res
		err = cr.err
	case <-time.After(60 * time.Second):
		err = errors.New("collection timed out after 60s (likely due to large Prometheus metrics volume)")
	}

	if err != nil {
		config.DebugLog("APILatency: collection failed: %v", err)
		if isAuthError(err) {
			report.APINote = authRequiredMessage()
		} else {
			report.APINote = fmt.Sprintf("API latency check failed: %s", err.Error())
		}
		return
	}
	config.DebugLog("APILatency: collection success (P95: %.2fs)", result.P95)

	// Double check we actually got data. If AutoDiscover succeeded but found no metrics, 
	// ensure we show the discovery note.
	if result.P95 == 0 && result.P90 == 0 && result.Note != "" && cfg.AutoDiscover {
		report.APINote = "Discovery found no metrics: " + result.Note
	} else if result.P95 == 0 && result.P90 == 0 && cfg.AutoDiscover {
		// If both are zero and discovery was on, something is suspicious.
		if len(result.NoDataReasons) > 0 {
			report.APINote = "No data found: " + result.NoDataReasons[0].Reason
		}
	}

	threshold := cfg.LatencyThresholdSeconds
	if threshold <= 0 {
		threshold = 10
	}

	status := model.SAFE
	if result.P95 > threshold {
		status = model.RISK
	} else if result.P90 > threshold {
		status = model.CHECK
	}

	report.APILatency = &model.APILatency{
		Service:                cfg.APIService,
		Route:                  cfg.APIRoute,
		Window:                 result.Window,
		P90:                    result.P90,
		P95:                    result.P95,
		P99:                    result.P99,
		BaselineWindow:         result.BaselineWindow,
		BaselineP95:            result.BaselineP95,
		BaselineErrorRate:      result.BaselineErrorRate,
		BaselineRPS:            result.BaselineRPS,
		DeltaP95:               result.DeltaP95,
		DeltaErrorRate:         result.DeltaErrorRate,
		DeltaRPS:               result.DeltaRPS,
		ErrorRate:              result.ErrorRate,
		ClientErrorRate:        result.ClientErrorRate,
		ServerErrorRate:        result.ServerErrorRate,
		ErrorLabel:             result.ErrorLabel,
		RPS:                    result.RPS,
		Status:                 status,
		Note:                   result.Note,
		Confidence:             result.Confidence,
		TopEndpointsLimit:      cfg.TopEndpoints,
		TopEndpoints:           mapEndpoints(result.TopEndpoints),
		TopEndpointsRPS:        mapEndpointsRPS(result.TopEndpointsRPS),
		TopEndpointsRPSNote:    result.TopEndpointsRPSNote,
		TopServicesRPS:         mapServicesRPS(result.TopServicesRPS),
		TopServicesRPSNote:     result.TopServicesRPSNote,
		ActiveAlerts:           result.ActiveAlerts,
		TopEndpointRegressions: mapEndpointRegressions(result.TopEndpointRegressions),
		RegressionNote:         result.RegressionNote,
		SpikeNote:              result.SpikeNote,
		NoDataReasons:          mapNoDataReasons(result.NoDataReasons),
		RemediationHints:       result.RemediationHints,
		LokiCorrelationHints:   result.LokiCorrelationHints,
		LatencyMetric:          result.LatencyMetric,
		RequestMetric:          result.RequestMetric,
		ServiceLabel:           result.ServiceLabel,
		RouteLabel:             result.RouteLabel,
		LabelSelector:          result.LabelSelector,
	}

	apmResult, apmErr := apm.Collect(&client, cfg)
	if apmErr == nil && (len(apmResult.Edges) > 0 || len(apmResult.NoDataReasons) > 0) {
		report.APM = &model.DependencyAnalysis{
			Edges:                 mapAPMEdges(apmResult.Edges),
			RankedCauses:          mapAPMCauses(apmResult.RankedCauses),
			SuspectedUpstream:     apmResult.SuspectedUpstream,
			CorrelationConfidence: apmResult.CorrelationConfidence,
			Evidence:              apmResult.Evidence,
			ReasoningSteps:        apmResult.ReasoningSteps,
			NextChecks:            apmResult.NextChecks,
			PossibleFixes:         apmResult.PossibleFixes,
			NoDataReasons:         mapNoDataReasonsAPM(apmResult.NoDataReasons),
			Note:                  apmResult.Note,
			Window:                apmResult.Window,
			Metric:                apmResult.Metric,
			SourceLabel:           apmResult.SourceLabel,
			DestinationLabel:      apmResult.DestinationLabel,
			RouteLabel:            apmResult.RouteLabel,
		}
	}

	if report.APILatency != nil && status != model.SAFE {
		correlationScope := logs.Scope{
			Service: report.APILatency.Service,
			Route:   report.APILatency.Route,
		}
		scopeNote := ""
		if report.APM != nil {
			correlationScope.Dependency = report.APM.SuspectedUpstream
			_, dest := parseDependencyEdge(report.APM.SuspectedUpstream)
			if dest != "" {
				if !strings.EqualFold(correlationScope.Service, dest) {
					scopeNote = "Correlation scope set to dependency destination: " + dest
				}
				correlationScope.Service = dest
				correlationScope.Route = ""
			}
		}
		window := cfg.LokiWindow
		if strings.TrimSpace(window) == "" {
			window = report.APILatency.Window
		}
		summary := buildCorrelationSummary(cfg, correlationScope, window)
		if summary != nil && scopeNote != "" {
			summary.Notes = append(summary.Notes, scopeNote)
		}
		report.CorrelationSummary = summary

		if report.APM != nil && strings.EqualFold(report.APM.CorrelationConfidence, "low") {
			alternates := []model.CorrelationSummary{}
			seen := map[string]struct{}{}
			if strings.TrimSpace(report.APM.SuspectedUpstream) != "" {
				seen[report.APM.SuspectedUpstream] = struct{}{}
			}
			for _, cause := range report.APM.RankedCauses {
				if len(alternates) >= 1 {
					break
				}
				edge := strings.TrimSpace(cause.Path)
				if edge == "" {
					continue
				}
				if _, exists := seen[edge]; exists {
					continue
				}
				seen[edge] = struct{}{}
				_, dest := parseDependencyEdge(edge)
				if dest == "" {
					continue
				}
				altScope := logs.Scope{
					Service:    dest,
					Dependency: edge,
				}
				altSummary := buildCorrelationSummary(cfg, altScope, window)
				if altSummary != nil {
					altSummary.Notes = append(altSummary.Notes, "Low-confidence fallback for edge: "+edge)
					alternates = append(alternates, *altSummary)
				}
			}
			if len(alternates) > 0 {
				report.CorrelationAlternates = alternates
			}
		}
	}

	if strings.TrimSpace(cfg.TraceURL) != "" {
		tcfg := tracing.TraceConfig{
			TraceURL:                 cfg.TraceURL,
			TraceToken:               cfg.TraceToken,
			TraceUser:                cfg.TraceUser,
			TracePass:                cfg.TracePass,
			TraceBackend:             cfg.TraceBackend,
			TraceWindow:              cfg.TraceWindow,
			TraceMinDurationMs:       cfg.TraceMinDurationMs,
			TraceExcludeSystemRoutes: cfg.TraceExcludeSystemRoutes,
			TraceServiceMap:          cfg.TraceServiceMap,
			TraceDefaultService:      cfg.TraceDefaultService,
			GrafanaURL:               cfg.GrafanaURL,
			GrafanaTraceDataSource:   cfg.GrafanaTraceDataSource,
			GrafanaToken:             cfg.GrafanaToken,
			GrafanaUser:              cfg.GrafanaUser,
			GrafanaPass:              cfg.GrafanaPass,
		}
		selection := tracing.ResolveTraceService(tcfg, report.APILatency, report.APM)
		traceSummary, err := tracing.Collect(tcfg, selection.Service)
		if traceSummary != nil {
			if selection.Note != "" {
				traceSummary.Note = joinNotesInline(traceSummary.Note, []string{selection.Note})
			}
			report.Tracing = traceSummary
		} else if err != nil {
			report.Tracing = &model.TraceSummary{
				Service:    selection.Service,
				Window:     cfg.TraceWindow,
				Backend:    cfg.TraceBackend,
				BackendURL: cfg.TraceURL,
				Note:       "Trace diagnostics unavailable: " + err.Error(),
			}
		}
	}
}

func joinNotesInline(prefix string, notes []string) string {
	if len(notes) == 0 {
		return prefix
	}
	combined := strings.Join(notes, " • ")
	if prefix == "" {
		return combined
	}
	return prefix + " • " + combined
}

func parseDependencyEdge(value string) (string, string) {
	clean := strings.TrimSpace(value)
	if clean == "" {
		return "", ""
	}
	clean = strings.ReplaceAll(clean, "→", "->")
	parts := strings.Split(clean, "->")
	if len(parts) < 2 {
		return "", ""
	}
	source := strings.TrimSpace(parts[0])
	dest := strings.TrimSpace(parts[1])
	return source, dest
}

func correlationConfidence(signatures []logs.SignatureCount, samples int, minSamples int) string {
	if samples <= 0 || len(signatures) == 0 {
		return ""
	}
	topPercent := signatures[0].Percent
	if samples < minSamples {
		return "low"
	}
	switch {
	case topPercent >= 70:
		return "high"
	case topPercent >= 40:
		return "medium"
	default:
		return "low"
	}
}

func buildCorrelationSummary(cfg config.Config, scope logs.Scope, window string) *model.CorrelationSummary {
	backend := logs.BackendForConfig(cfg)
	if backend == nil {
		return &model.CorrelationSummary{
			Backend:       "loki",
			Window:        window,
			Scope:         toCorrelationScope(scope),
			NoDataReasons: []string{"Loki URL not set."},
			MinSamples:    logs.DefaultMinSamples(),
		}
	}
	errorRegex := cfg.LokiErrorRegex
	if strings.EqualFold(backend.Name(), "elastic") {
		errorRegex = strings.TrimSpace(cfg.ElasticErrorRegex)
	}
	res, err := backend.Correlate(logs.CorrelationRequest{
		Scope:      scope,
		Window:     window,
		ErrorRegex: errorRegex,
		MinSamples: cfg.CorrelationMinSamples,
		MaxLogs:    cfg.CorrelationMaxLogs,
		MaxResults: cfg.CorrelationMaxResults,
	})
	summary := &model.CorrelationSummary{
		Backend:    backend.Name(),
		Window:     window,
		Query:      res.Query,
		Notes:      res.Notes,
		Samples:    res.Samples,
		MinSamples: res.MinSamples,
		Confidence: correlationConfidence(res.Signatures, res.Samples, res.MinSamples),
		Scope:      toCorrelationScope(scope),
		Signatures: toCorrelationSignatures(res.Signatures),
	}
	if err != nil {
		summary.NoDataReasons = []string{"Correlation failed: " + err.Error()}
	} else if res.Samples == 0 {
		summary.NoDataReasons = []string{"No matching logs found for the correlation scope."}
	} else if res.Samples < res.MinSamples {
		if cfg.CorrelationBestEffort {
			summary.Notes = append(summary.Notes, fmt.Sprintf("Low confidence: %d samples (<%d).", res.Samples, res.MinSamples))
		} else {
			summary.NoDataReasons = []string{fmt.Sprintf("Insufficient samples (%d < %d).", res.Samples, res.MinSamples)}
		}
	}
	return summary
}

func toCorrelationScope(scope logs.Scope) model.CorrelationScope {
	return model.CorrelationScope{
		Service:    scope.Service,
		Route:      scope.Route,
		Dependency: scope.Dependency,
	}
}

func toCorrelationSignatures(items []logs.SignatureCount) []model.CorrelationSignature {
	if len(items) == 0 {
		return nil
	}
	out := make([]model.CorrelationSignature, 0, len(items))
	for _, item := range items {
		out = append(out, model.CorrelationSignature{
			Signature: item.Signature,
			Count:     item.Count,
			Percent:   item.Percent,
		})
	}
	return out
}

func mapEndpointRegressions(items []api_latency.EndpointRegression) []model.EndpointRegression {
	if len(items) == 0 {
		return nil
	}
	out := make([]model.EndpointRegression, 0, len(items))
	for _, item := range items {
		out = append(out, model.EndpointRegression{
			Route:       item.Route,
			NowP95:      item.NowP95,
			BaselineP95: item.BaselineP95,
			DeltaP95:    item.DeltaP95,
		})
	}
	return out
}

func mapEndpoints(items []api_latency.EndpointLatency) []model.EndpointLatency {
	if len(items) == 0 {
		return nil
	}
	out := make([]model.EndpointLatency, 0, len(items))
	for _, item := range items {
		out = append(out, model.EndpointLatency{
			Route: item.Route,
			P95:   item.P95,
		})
	}
	return out
}

func mapEndpointsRPS(items []api_latency.EndpointRate) []model.EndpointRate {
	if len(items) == 0 {
		return nil
	}
	out := make([]model.EndpointRate, 0, len(items))
	for _, item := range items {
		out = append(out, model.EndpointRate{
			Route: item.Route,
			RPS:   item.RPS,
		})
	}
	return out
}

func mapServicesRPS(items []api_latency.ServiceRate) []model.ServiceRate {
	if len(items) == 0 {
		return nil
	}
	out := make([]model.ServiceRate, 0, len(items))
	for _, item := range items {
		out = append(out, model.ServiceRate{
			Service: item.Service,
			RPS:     item.RPS,
		})
	}
	return out
}

func mapNoDataReasons(items []api_latency.NoDataReason) []model.NoDataReason {
	if len(items) == 0 {
		return nil
	}
	out := make([]model.NoDataReason, 0, len(items))
	for _, item := range items {
		out = append(out, model.NoDataReason{
			Area:         item.Area,
			Reason:       item.Reason,
			Metric:       item.Metric,
			Labels:       item.Labels,
			Window:       item.Window,
			SuggestedFix: item.SuggestedFix,
		})
	}
	return out
}

func mapNoDataReasonsAPM(items []apm.NoDataReason) []model.NoDataReason {
	if len(items) == 0 {
		return nil
	}
	out := make([]model.NoDataReason, 0, len(items))
	for _, item := range items {
		out = append(out, model.NoDataReason{
			Area:         item.Area,
			Reason:       item.Reason,
			Metric:       item.Metric,
			Labels:       item.Labels,
			Window:       item.Window,
			SuggestedFix: item.SuggestedFix,
		})
	}
	return out
}

func mapAPMEdges(items []apm.Edge) []model.DependencyEdge {
	if len(items) == 0 {
		return nil
	}
	out := make([]model.DependencyEdge, 0, len(items))
	for _, item := range items {
		out = append(out, model.DependencyEdge{
			Source:        item.Source,
			Destination:   item.Destination,
			Route:         item.Route,
			P95:           item.P95,
			BaselineP95:   item.BaselineP95,
			DeltaP95:      item.DeltaP95,
			RPS:           item.RPS,
			ErrorRate:     item.ErrorRate,
			ConfidenceTag: item.ConfidenceTag,
		})
	}
	return out
}

func mapAPMCauses(items []apm.Cause) []model.DependencyCause {
	if len(items) == 0 {
		return nil
	}
	out := make([]model.DependencyCause, 0, len(items))
	for _, item := range items {
		out = append(out, model.DependencyCause{
			Path:       item.Path,
			Score:      item.Score,
			Confidence: item.Confidence,
		})
	}
	return out
}

func isAuthError(err error) bool {
	var httpErr prometheus.HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode == 401 || httpErr.StatusCode == 403
	}
	return false
}

func missingConfigMessage(missing []string) string {
	return fmt.Sprintf(
		"API latency check disabled. Missing: %s. Set PROMETHEUS_URL (URL-only mode) or configure /etc/health-monitor/config.json. "+
			"Ask your platform/observability team for the Prometheus URL.",
		strings.Join(missing, ", "),
	)
}

func authRequiredMessage() string {
	return "Prometheus auth required. Provide PROMETHEUS_TOKEN or PROMETHEUS_USER/PROMETHEUS_PASS in env, or set PROMETHEUS_TOKEN_FILE to a read-only token file (recommended)."
}

func buildConfigSummary(cfg config.Config, path string) *model.APIConfigSummary {
	authMode := "none"
	if cfg.PrometheusToken != "" {
		authMode = "token"
	} else if cfg.PrometheusUser != "" || cfg.PrometheusPass != "" {
		authMode = "user/pass"
	}
	systemPath := config.SystemConfigPath()
	userPath, _ := config.UserConfigPath()
	tokenFilePath, tokenSource := resolveTokenSource(cfg)
	return &model.APIConfigSummary{
		ActiveConfigPath: path,
		SystemConfigPath: systemPath,
		UserConfigPath:   userPath,
		TokenFilePath:    tokenFilePath,
		TokenSource:      tokenSource,
		DisableUpdates:   cfg.DisableUpdates,
		AutoDiscover:     cfg.AutoDiscover,
		PrometheusURL:    cfg.PrometheusURL,
		APIService:       cfg.APIService,
		APIRoute:         cfg.APIRoute,
		AuthMode:         authMode,
		HasToken:         cfg.PrometheusToken != "",
		HasUserPass:      cfg.PrometheusUser != "" || cfg.PrometheusPass != "",
		Window:           cfg.Window,
		LatencyMetric:    cfg.LatencyMetric,
		RequestMetric:    cfg.RequestCountMetric,
		ServiceLabel:     cfg.ServiceLabel,
		RouteLabel:       cfg.RouteLabel,
		ErrorLabel:       cfg.ErrorLabel,
		ErrorRegex:       cfg.ErrorRegex,
		ExtraSelectors:   cfg.ExtraLabelSelectors,
	}
}

func resolveTokenSource(cfg config.Config) (string, string) {
	if v := os.Getenv("PROMETHEUS_TOKEN"); v != "" {
		return "", "env"
	}
	if v := os.Getenv("PROMETHEUS_TOKEN_FILE"); v != "" {
		return v, "token_file_env"
	}
	systemToken := "/etc/health-monitor/prometheus.token"
	if fileExists(systemToken) && cfg.PrometheusToken != "" {
		return systemToken, "token_file_system"
	}
	if userPath, err := config.UserConfigPath(); err == nil && userPath != "" {
		userToken := filepath.Join(filepath.Dir(userPath), "prometheus.token")
		if fileExists(userToken) && cfg.PrometheusToken != "" {
			return userToken, "token_file_user"
		}
	}
	if cfg.PrometheusToken != "" {
		return "", "config"
	}
	return config.TokenFilePath(), "none"
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func contains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}
