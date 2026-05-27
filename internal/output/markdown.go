package output

import (
	"fmt"
	"strings"
	"time"

	"health-monitor/internal/config"
	"health-monitor/pkg/model"
)

func renderReportMarkdown(report model.Report) string {
	var sb strings.Builder
	sb.WriteString("# Health Monitor Report\n\n")
	sb.WriteString("Generated: " + time.Now().Format(time.RFC3339) + "\n\n")

	if len(report.Summary) > 0 {
		sb.WriteString("## Summary\n\n")
		sb.WriteString("| Check | Status | Value |\n")
		sb.WriteString("| --- | --- | --- |\n")
		for _, item := range report.Summary {
			sb.WriteString(fmt.Sprintf("| %s | %s | %s |\n", item.Name, item.Status, item.Value))
		}
		sb.WriteString("\n")
	}

	if len(report.Metrics) > 0 {
		sb.WriteString("## Metrics\n\n")
		for _, metric := range report.Metrics {
			sb.WriteString(fmt.Sprintf("- %s: %s\n", metric.Name, metric.Value))
		}
		sb.WriteString("\n")
	}

	if report.APINote != "" {
		sb.WriteString("## API Latency (Prometheus)\n\n")
		sb.WriteString(report.APINote + "\n\n")
	} else if report.APILatency != nil {
		sb.WriteString("## API Latency (Prometheus)\n\n")
		sb.WriteString(fmt.Sprintf("- Service: %s\n", EmptyOr(report.APILatency.Service, "all services")))
		sb.WriteString(fmt.Sprintf("- Route: %s\n", EmptyOr(report.APILatency.Route, "all routes")))
		sb.WriteString(fmt.Sprintf("- Status: %s\n", report.APILatency.Status))
		sb.WriteString(fmt.Sprintf("- Window: %s\n", report.APILatency.Window))
		sb.WriteString("\n")

		if strings.TrimSpace(report.APILatency.Note) != "" {
			sb.WriteString("### Auto-Detect Reasoning\n\n")
			for _, line := range wrapNoteLinesMarkdown(report.APILatency.Note) {
				sb.WriteString("- " + line + "\n")
			}
			sb.WriteString("\n")
		}

		sb.WriteString("### Current Metrics\n\n")
		sb.WriteString(fmt.Sprintf("- P90: %s\n", FormatLatency(report.APILatency.P90)))
		sb.WriteString(fmt.Sprintf("- P95: %s\n", FormatLatency(report.APILatency.P95)))
		sb.WriteString(fmt.Sprintf("- P99: %s\n", FormatLatency(report.APILatency.P99)))
		sb.WriteString(fmt.Sprintf("- Error Rate: %s\n", FormatPercent(report.APILatency.ErrorRate)))
		sb.WriteString(fmt.Sprintf("- 4xx Rate: %s\n", FormatPercent(report.APILatency.ClientErrorRate)))
		sb.WriteString(fmt.Sprintf("- 5xx Rate: %s\n", FormatPercent(report.APILatency.ServerErrorRate)))
		sb.WriteString(fmt.Sprintf("- RPS: %.2f\n", report.APILatency.RPS))
		if strings.TrimSpace(report.APILatency.ErrorLabel) != "" {
			sb.WriteString(fmt.Sprintf("- Error Label: %s\n", report.APILatency.ErrorLabel))
		}
		sb.WriteString("\n")

		sb.WriteString("### Trend vs Baseline\n\n")
		baselineLabel := "Baseline"
		if strings.TrimSpace(report.APILatency.BaselineWindow) != "" {
			baselineLabel = "Baseline " + report.APILatency.BaselineWindow
		}
		sb.WriteString("| Metric | Now | " + baselineLabel + " | Delta |\n")
		sb.WriteString("| --- | --- | --- | --- |\n")
		sb.WriteString(fmt.Sprintf("| P95 | %s | %s | %s |\n", FormatLatency(report.APILatency.P95), FormatLatency(report.APILatency.BaselineP95), FormatDeltaLatency(report.APILatency.DeltaP95)))
		sb.WriteString(fmt.Sprintf("| Error Rate | %s | %s | %s |\n", FormatPercent(report.APILatency.ErrorRate), FormatPercent(report.APILatency.BaselineErrorRate), FormatDeltaPercent(report.APILatency.DeltaErrorRate)))
		sb.WriteString(fmt.Sprintf("| RPS | %.2f | %.2f | %s |\n", report.APILatency.RPS, report.APILatency.BaselineRPS, FormatDeltaNumber(report.APILatency.DeltaRPS)))
		sb.WriteString("\n")

		cfg, _ := config.Load()
		cfg = mergeConfigFromSummaryMarkdown(cfg, report.APIConfig)
		window := report.APILatency.Window
		if strings.TrimSpace(window) == "" {
			window = cfg.Window
		}
		if strings.TrimSpace(window) == "" {
			window = "5m"
		}
		promQuery := PromP95Query(report.APILatency)
		promLink := BuildGrafanaExploreURL(cfg.GrafanaURL, cfg.GrafanaPromDataSource, "", "", promQuery, "now-"+window, "now")
		logQL := extractSuggestedLogQL(report.APILatency.LokiCorrelationHints)
		lokiWindow := cfg.LokiWindow
		if strings.TrimSpace(lokiWindow) == "" {
			lokiWindow = "10m"
		}
		lokiLink := BuildGrafanaExploreURL(cfg.GrafanaURL, cfg.GrafanaLokiDataSource, "", "", logQL, "now-"+lokiWindow, "now")
		if promLink != "" || lokiLink != "" {
			sb.WriteString("### Grafana Explore Links\n\n")
			if promLink != "" {
				sb.WriteString("- Prometheus: " + promLink + "\n")
			}
			if lokiLink != "" {
				sb.WriteString("- Loki: " + lokiLink + "\n")
			}
			sb.WriteString("\n")
		}

		if len(report.APILatency.ActiveAlerts) > 0 {
			sb.WriteString("### Active Alerts\n\n")
			for _, alert := range report.APILatency.ActiveAlerts {
				sb.WriteString("- " + alert + "\n")
			}
			sb.WriteString("\n")
		}

		if len(report.APILatency.NoDataReasons) > 0 {
			sb.WriteString("### Data Gaps\n\n")
			for _, reason := range report.APILatency.NoDataReasons {
				line := fmt.Sprintf("%s: %s", reason.Area, reason.Reason)
				if strings.TrimSpace(reason.SuggestedFix) != "" {
					line += " (Fix: " + reason.SuggestedFix + ")"
				}
				sb.WriteString("- " + line + "\n")
			}
			sb.WriteString("\n")
		}

		if warnings := extractCardinalityWarningsMarkdown(report.APILatency.Note); len(warnings) > 0 {
			sb.WriteString("### Warnings\n\n")
			for _, warning := range warnings {
				sb.WriteString("- " + warning + "\n")
			}
			sb.WriteString("\n")
		}

		cfg = mergeConfigFromSummaryMarkdown(cfg, report.APIConfig)
		if issues := buildConfigHealthMarkdown(cfg); len(issues) > 0 {
			sb.WriteString("### Config Health\n\n")
			for _, issue := range issues {
				sb.WriteString("- " + issue + "\n")
			}
			sb.WriteString("\n")
		}

		if len(report.APILatency.RemediationHints) > 0 {
			sb.WriteString("### Remediation Checklist\n\n")
			for _, hint := range report.APILatency.RemediationHints {
				sb.WriteString("- " + hint + "\n")
			}
			sb.WriteString("\n")
		}

		if len(report.APILatency.LokiCorrelationHints) > 0 {
			sb.WriteString("### Prometheus ↔ Loki\n\n")
			for _, hint := range report.APILatency.LokiCorrelationHints {
				sb.WriteString("- " + hint + "\n")
			}
			sb.WriteString("\n")
		}

		if report.APM != nil {
			sb.WriteString("### Suspected Upstream Cause\n\n")
			if strings.TrimSpace(report.APM.SuspectedUpstream) == "" {
				msg := "No root-cause inference available."
				if len(report.APM.NoDataReasons) > 0 {
					msg = "No root-cause inference available (see APM Data Gaps)."
				}
				sb.WriteString("- " + msg + "\n")
			}
			if strings.TrimSpace(report.APM.SuspectedUpstream) != "" {
				sb.WriteString("- Cause: " + report.APM.SuspectedUpstream + "\n")
			}
			if strings.TrimSpace(report.APM.CorrelationConfidence) != "" {
				sb.WriteString("- Confidence: " + strings.Title(report.APM.CorrelationConfidence) + "\n")
			}
			if len(report.APM.Evidence) > 0 {
				sb.WriteString("\nEvidence:\n")
				for _, item := range report.APM.Evidence {
					sb.WriteString("- " + item + "\n")
				}
			}
			sb.WriteString("\n")

			if len(report.APM.RankedCauses) > 0 {
				sb.WriteString("### APM Root Cause Ranking\n\n")
				sb.WriteString("| Path | Score | Confidence |\n")
				sb.WriteString("| --- | --- | --- |\n")
				for _, cause := range report.APM.RankedCauses {
					sb.WriteString(fmt.Sprintf("| %s | %.2f | %s |\n", cause.Path, cause.Score, strings.Title(cause.Confidence)))
				}
				sb.WriteString("\n")
			}

			if len(report.APM.ReasoningSteps) > 0 {
				sb.WriteString("### Why We Think This Is The Cause\n\n")
				for _, item := range report.APM.ReasoningSteps {
					sb.WriteString("- " + item + "\n")
				}
				sb.WriteString("\n")
			}

			if len(report.APM.NextChecks) > 0 {
				sb.WriteString("### What To Check Next\n\n")
				for _, item := range report.APM.NextChecks {
					sb.WriteString("- " + item + "\n")
				}
				sb.WriteString("\n")
			}

			if len(report.APM.PossibleFixes) > 0 {
				sb.WriteString("### Possible Fixes\n\n")
				for _, item := range report.APM.PossibleFixes {
					sb.WriteString("- " + item + "\n")
				}
				sb.WriteString("\n")
			}

			if len(report.APM.NoDataReasons) > 0 {
				sb.WriteString("### APM Data Gaps\n\n")
				for _, reason := range report.APM.NoDataReasons {
					line := fmt.Sprintf("%s: %s", reason.Area, reason.Reason)
					if strings.TrimSpace(reason.SuggestedFix) != "" {
						line += " (Fix: " + reason.SuggestedFix + ")"
					}
					sb.WriteString("- " + line + "\n")
				}
				sb.WriteString("\n")
			}

			if strings.TrimSpace(report.APM.Note) != "" {
				sb.WriteString("### APM Auto-Detect Reasoning\n\n")
				for _, line := range wrapNoteLinesMarkdown(report.APM.Note) {
					sb.WriteString("- " + line + "\n")
				}
				sb.WriteString("\n")
			}

			if len(report.APM.Edges) > 0 {
				sb.WriteString("### Top Dependency Edges (P95)\n\n")
				sb.WriteString("| Edge | P95 | Delta |\n")
				sb.WriteString("| --- | --- | --- |\n")
				for _, edge := range report.APM.Edges {
					label := edge.Source + " -> " + edge.Destination
					if strings.TrimSpace(edge.Route) != "" {
						label = label + " (" + edge.Route + ")"
					}
					sb.WriteString(fmt.Sprintf("| %s | %s | %s |\n", label, FormatLatency(edge.P95), FormatDeltaLatency(edge.DeltaP95)))
				}
				sb.WriteString("\n")
			}
		}

		if len(report.APILatency.LogSamples) > 0 {
			sb.WriteString("### Recent Logs\n\n")
			for _, line := range report.APILatency.LogSamples {
				sb.WriteString("- " + line + "\n")
			}
			sb.WriteString("\n")
		}

		if len(report.APILatency.TopEndpoints) > 0 {
			sb.WriteString("### Top Endpoints (P95)\n\n")
			sb.WriteString("| Endpoint | P95 |\n")
			sb.WriteString("| --- | --- |\n")
			for _, endpoint := range report.APILatency.TopEndpoints {
				sb.WriteString(fmt.Sprintf("| %s | %s |\n", endpoint.Route, FormatLatency(endpoint.P95)))
			}
			sb.WriteString("\n")
		}

		if len(report.APILatency.TopEndpointsRPS) > 0 {
			sb.WriteString("### Top Endpoints (RPS)\n\n")
			sb.WriteString("| Endpoint | RPS |\n")
			sb.WriteString("| --- | --- |\n")
			for _, endpoint := range report.APILatency.TopEndpointsRPS {
				sb.WriteString(fmt.Sprintf("| %s | %.2f |\n", endpoint.Route, endpoint.RPS))
			}
			sb.WriteString("\n")
		}
	}

	if report.Tracing != nil {
		sb.WriteString("## Trace Diagnostics\n\n")
		service := report.Tracing.Service
		if strings.TrimSpace(service) == "" {
			service = "auto"
		}
		sb.WriteString(fmt.Sprintf("- Service: %s\n", service))
		if strings.TrimSpace(report.Tracing.Window) != "" {
			sb.WriteString(fmt.Sprintf("- Window: %s\n", report.Tracing.Window))
		}
		if strings.TrimSpace(report.Tracing.Note) != "" {
			sb.WriteString(fmt.Sprintf("- Note: %s\n", report.Tracing.Note))
		}
		sb.WriteString("\n")
		if len(report.Tracing.TopSlowTraces) > 0 {
			sb.WriteString("### Top Slow Traces\n\n")
			for i, trace := range report.Tracing.TopSlowTraces {
				sb.WriteString(fmt.Sprintf("%d. Trace ID: %s\n", i+1, trace.TraceID))
				sb.WriteString(fmt.Sprintf("   - Total Duration: %s\n", trace.TotalDuration))
				if strings.TrimSpace(trace.RootOperation) != "" {
					sb.WriteString(fmt.Sprintf("   - Root Operation: %s\n", trace.RootOperation))
				}
				if strings.TrimSpace(trace.SlowestSpan.Service) != "" || strings.TrimSpace(trace.SlowestSpan.Operation) != "" {
					sb.WriteString("   - Slowest Downstream Span:\n")
					if strings.TrimSpace(trace.SlowestSpan.Service) != "" {
						sb.WriteString(fmt.Sprintf("     - Service: %s\n", trace.SlowestSpan.Service))
					}
					if strings.TrimSpace(trace.SlowestSpan.Operation) != "" {
						sb.WriteString(fmt.Sprintf("     - Operation: %s\n", trace.SlowestSpan.Operation))
					}
					if strings.TrimSpace(trace.SlowestSpan.Duration) != "" {
						sb.WriteString(fmt.Sprintf("     - Duration: %s\n", trace.SlowestSpan.Duration))
					}
				}
			}
			sb.WriteString("\n")
		}
		links := report.Tracing.Links
		if len(links) == 0 {
			links = ResolveTraceLinks(report.Tracing)
		}
		if len(links) > 0 {
			sb.WriteString("### Trace Links\n\n")
			for _, link := range links {
				label := strings.TrimSpace(link.Label)
				if label == "" {
					label = "Trace link"
				}
				sb.WriteString(fmt.Sprintf("- %s: %s\n", label, link.URL))
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

func wrapNoteLinesMarkdown(note string) []string {
	if strings.TrimSpace(note) == "" {
		return nil
	}
	parts := strings.Split(note, " • ")
	var out []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func extractCardinalityWarningsMarkdown(note string) []string {
	parts := strings.Split(note, " • ")
	var warnings []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if strings.Contains(trimmed, "high route cardinality") || strings.Contains(trimmed, "high service cardinality") {
			warnings = append(warnings, trimmed)
		}
	}
	return warnings
}

func extractSuggestedLogQL(hints []string) string {
	for _, hint := range hints {
		trimmed := strings.TrimSpace(hint)
		if strings.HasPrefix(trimmed, "Suggested LogQL: ") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "Suggested LogQL: "))
		}
	}
	return ""
}

func mergeConfigFromSummaryMarkdown(cfg config.Config, summary *model.APIConfigSummary) config.Config {
	if summary == nil {
		return cfg
	}
	if strings.TrimSpace(cfg.PrometheusURL) == "" {
		cfg.PrometheusURL = summary.PrometheusURL
	}
	if strings.TrimSpace(cfg.APIService) == "" {
		cfg.APIService = summary.APIService
	}
	if strings.TrimSpace(cfg.APIRoute) == "" {
		cfg.APIRoute = summary.APIRoute
	}
	if strings.TrimSpace(cfg.ServiceLabel) == "" {
		cfg.ServiceLabel = summary.ServiceLabel
	}
	if strings.TrimSpace(cfg.RouteLabel) == "" {
		cfg.RouteLabel = summary.RouteLabel
	}
	if strings.TrimSpace(cfg.LatencyMetric) == "" {
		cfg.LatencyMetric = summary.LatencyMetric
	}
	if strings.TrimSpace(cfg.RequestCountMetric) == "" {
		cfg.RequestCountMetric = summary.RequestMetric
	}
	if strings.TrimSpace(cfg.ErrorLabel) == "" {
		cfg.ErrorLabel = summary.ErrorLabel
	}
	if strings.TrimSpace(cfg.ErrorRegex) == "" {
		cfg.ErrorRegex = summary.ErrorRegex
	}
	if strings.TrimSpace(cfg.Window) == "" {
		cfg.Window = summary.Window
	}
	if !cfg.AutoDiscover {
		cfg.AutoDiscover = summary.AutoDiscover
	}
	return cfg
}

func buildConfigHealthMarkdown(cfg config.Config) []string {
	var issues []string
	if strings.TrimSpace(cfg.PrometheusURL) == "" {
		issues = append(issues, "Prometheus URL not set.")
	} else if !strings.HasPrefix(cfg.PrometheusURL, "http://") && !strings.HasPrefix(cfg.PrometheusURL, "https://") {
		issues = append(issues, "Prometheus URL must start with http:// or https://.")
	}
	if !cfg.AutoDiscover && strings.TrimSpace(cfg.ServiceLabel) != "" && strings.TrimSpace(cfg.APIService) == "" {
		issues = append(issues, "API_SERVICE required when auto-discover is off.")
	}
	if cfg.PrometheusQPS == 0 {
		issues = append(issues, "Prometheus QPS is 0; rate limiting disabled.")
	}
	if cfg.RouteCardinalityLimit == 0 {
		issues = append(issues, "Route cardinality limit is 0; per-route queries may be heavy.")
	}
	if cfg.ServiceCardinalityLimit == 0 {
		issues = append(issues, "Service cardinality limit is 0; per-service queries may be heavy.")
	}
	return issues
}
