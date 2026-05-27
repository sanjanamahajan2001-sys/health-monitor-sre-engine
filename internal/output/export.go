package output

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"health-monitor/internal/config"
	"health-monitor/pkg/model"
)


func ExportReport(report model.Report, format string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		report = sanitizeReportForExport(report)
		health := buildConfigHealthForExport(report)
		payload, err := json.MarshalIndent(reportExport{
			Report:       report,
			ConfigHealth: health,
		}, "", "  ")
		if err != nil {
			return "", err
		}
		return string(payload), nil
	case "md", "markdown":
		return renderReportMarkdown(report), nil
	default:
		return "", fmt.Errorf("unsupported format %q (use json or md)", format)
	}
}

type reportExport struct {
	model.Report
	ConfigHealth []string `json:"config_health,omitempty"`
}

func buildConfigHealthForExport(report model.Report) []string {
	cfg, err := config.Load()
	if err != nil {
		if report.APIConfig == nil {
			return nil
		}
		cfg = config.Default()
	}
	cfg = mergeConfigFromSummaryExport(cfg, report.APIConfig)
	return buildConfigHealthExport(cfg)
}

func mergeConfigFromSummaryExport(cfg config.Config, summary *model.APIConfigSummary) config.Config {
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

func buildConfigHealthExport(cfg config.Config) []string {
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
	if cfg.APMCardinalityLimit == 0 {
		issues = append(issues, "APM cardinality limit is 0; dependency queries may be heavy.")
	}
	if cfg.APMTopEdges == 0 {
		issues = append(issues, "APM top edges is 0; dependency graph is unbounded.")
	}
	if cfg.APMMinRPS == 0 {
		issues = append(issues, "APM min RPS is 0; low-traffic edges may look noisy.")
	}
	return issues
}

func sanitizeReportForExport(report model.Report) model.Report {
	report = sanitizeAPILatency(report)
	report = sanitizeAPM(report)
	return report
}

func sanitizeAPILatency(report model.Report) model.Report {
	if report.APILatency == nil {
		return report
	}
	api := *report.APILatency
	api.P90 = sanitizeFloat(api.P90)
	api.P95 = sanitizeFloat(api.P95)
	api.P99 = sanitizeFloat(api.P99)
	api.BaselineP95 = sanitizeFloat(api.BaselineP95)
	api.BaselineErrorRate = sanitizeFloat(api.BaselineErrorRate)
	api.BaselineRPS = sanitizeFloat(api.BaselineRPS)
	api.DeltaP95 = sanitizeFloat(api.DeltaP95)
	api.DeltaErrorRate = sanitizeFloat(api.DeltaErrorRate)
	api.DeltaRPS = sanitizeFloat(api.DeltaRPS)
	api.ErrorRate = sanitizeFloat(api.ErrorRate)
	api.ClientErrorRate = sanitizeFloat(api.ClientErrorRate)
	api.ServerErrorRate = sanitizeFloat(api.ServerErrorRate)
	api.RPS = sanitizeFloat(api.RPS)
	for i := range api.TopEndpoints {
		api.TopEndpoints[i].P95 = sanitizeFloat(api.TopEndpoints[i].P95)
	}
	for i := range api.TopEndpointsRPS {
		api.TopEndpointsRPS[i].RPS = sanitizeFloat(api.TopEndpointsRPS[i].RPS)
	}
	for i := range api.TopServicesRPS {
		api.TopServicesRPS[i].RPS = sanitizeFloat(api.TopServicesRPS[i].RPS)
	}
	for i := range api.TopEndpointRegressions {
		api.TopEndpointRegressions[i].NowP95 = sanitizeFloat(api.TopEndpointRegressions[i].NowP95)
		api.TopEndpointRegressions[i].BaselineP95 = sanitizeFloat(api.TopEndpointRegressions[i].BaselineP95)
		api.TopEndpointRegressions[i].DeltaP95 = sanitizeFloat(api.TopEndpointRegressions[i].DeltaP95)
	}
	report.APILatency = &api
	return report
}

func sanitizeAPM(report model.Report) model.Report {
	if report.APM == nil {
		return report
	}
	apm := *report.APM
	for i := range apm.Edges {
		apm.Edges[i].P95 = sanitizeFloat(apm.Edges[i].P95)
		apm.Edges[i].BaselineP95 = sanitizeFloat(apm.Edges[i].BaselineP95)
		apm.Edges[i].DeltaP95 = sanitizeFloat(apm.Edges[i].DeltaP95)
		apm.Edges[i].RPS = sanitizeFloat(apm.Edges[i].RPS)
		apm.Edges[i].ErrorRate = sanitizeFloat(apm.Edges[i].ErrorRate)
	}
	for i := range apm.RankedCauses {
		apm.RankedCauses[i].Score = sanitizeFloat(apm.RankedCauses[i].Score)
	}
	report.APM = &apm
	return report
}

func sanitizeFloat(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return value
}
