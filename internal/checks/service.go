package checks

import (
	"time"

	"health-monitor/internal/analyse/prometheus"
	service_analyse "health-monitor/internal/analyse/service"
	"health-monitor/internal/config"
	"health-monitor/pkg/model"
)

// ServiceDrilldown performs detailed service health checks
func ServiceDrilldown(report *model.Report, selectedService string) {
	cfg, err := config.Load()
	if err != nil {
		config.DebugLog("ServiceDrilldown: Failed to load config: %v", err)
		return
	}
	serviceName := selectedService
	if serviceName == "" {
		serviceName = cfg.APIService
	}
	config.DebugLog("ServiceDrilldown: Started check for service: %s", serviceName)
	if cfg.PrometheusURL != "" {
		promClient := &prometheus.Client{
			BaseURL: cfg.PrometheusURL,
			Token:   cfg.PrometheusToken,
			User:    cfg.PrometheusUser,
			Pass:    cfg.PrometheusPass,
			Timeout: 5 * time.Second,
			QPS:     cfg.PrometheusQPS,
		}
		
		// 0. Override config with discovered metrics if available
		if report.APILatency != nil {
			if report.APILatency.RequestMetric != "" {
				cfg.RequestCountMetric = report.APILatency.RequestMetric
			}
			if report.APILatency.LatencyMetric != "" {
				cfg.LatencyMetric = report.APILatency.LatencyMetric
			}
			if report.APILatency.ServiceLabel != "" {
				cfg.ServiceLabel = report.APILatency.ServiceLabel
			}
			config.DebugLog("ServiceDrilldown: Using discovered metrics: req=%s, lat=%s, label=%s", 
				cfg.RequestCountMetric, cfg.LatencyMetric, cfg.ServiceLabel)
		}

		// 1. Fetch all services for the overview
		allServices, err := service_analyse.CollectAllServicesOverview(promClient, cfg)
		if err == nil {
			report.AllServices = allServices
		} else {
			config.DebugLog("ServiceDrilldown: Failed to collect all services: %v", err)
		}

		// 2. Fetch specific drilldown if configured
		if serviceName != "" {
			drilldown, err := service_analyse.CollectServiceDrilldown(promClient, cfg, serviceName)
			if err == nil {
				report.ServiceDrilldown = drilldown
			} else {
				config.DebugLog("ServiceDrilldown: Specific collection failed for %s: %v", serviceName, err)
			}
		}
	}
}
