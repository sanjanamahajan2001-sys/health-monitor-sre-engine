package flow

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"health-monitor/internal/config"
	"health-monitor/pkg/model"
	"gopkg.in/yaml.v3"
)

// GenerateDefaultFlows creates a default flows.yaml for a profile based on discovered services
func GenerateDefaultFlows(profileName string, services []string, metadata map[string]model.ServiceMetadata) error {
	if len(services) == 0 {
		return nil
	}

	pm := config.GetProfileManager()
	flowsDir := filepath.Join(filepath.Dir(pm.GetProfilePath(profileName)), "..", "flows.d")
	
	if err := os.MkdirAll(flowsDir, 0755); err != nil {
		return err
	}

	flowsPath := filepath.Join(flowsDir, profileName+".yaml")

	// Build the flow file content
	type flowSLO struct {
		ID        string  `yaml:"id"`
		Service   string  `yaml:"service"`
		Objective float64 `yaml:"objective"`
		Window    string  `yaml:"window"`
		Type      string  `yaml:"type"`
		ErrorQuery string `yaml:"error_query,omitempty"`
		TotalQuery string `yaml:"total_query,omitempty"`
		PromQL     string `yaml:"promql,omitempty"`
		ThresholdSeconds float64 `yaml:"threshold_seconds,omitempty"`
	}

	type flowEntry struct {
		Name     string    `yaml:"name"`
		Services []string  `yaml:"services"`
		SLOs     []flowSLO `yaml:"slos"`
	}

	type flowFile struct {
		Flows map[string]flowEntry `yaml:"flows"`
	}

	file := flowFile{
		Flows: make(map[string]flowEntry),
	}

	for _, svc := range services {
		id := strings.ToLower(strings.ReplaceAll(svc, "-", "_"))
		meta, hasMeta := metadata[svc]
		if !hasMeta {
			meta = model.ServiceMetadata{
				RequestMetric: "http_requests_total",
				LatencyMetric: "http_request_duration_seconds",
				ServiceLabel:  "service", // Safe generic default
			}
		}

		// Robustness: ensure we have at least defaults if discovery was partial
		if meta.RequestMetric == "" {
			meta.RequestMetric = "http_requests_total"
		}
		if meta.LatencyMetric == "" {
			meta.LatencyMetric = "http_request_duration_seconds"
		}
		if meta.ServiceLabel == "" {
			meta.ServiceLabel = "service"
		}

		slos := []flowSLO{}

		// Success Rate SLO
		successObjective := 95.0
		if meta.SuccessRate > 0 {
			// If observed success is > 99%, set objective to 99%
			if meta.SuccessRate > 0.99 {
				successObjective = 99.0
			} else if meta.SuccessRate > 0.95 {
				successObjective = 95.0
			}
		}

		errorLabel := meta.ErrorLabel
		if errorLabel == "" {
			// fallback for standard metrics
			if strings.Contains(meta.RequestMetric, "http_") || strings.Contains(meta.RequestMetric, "nginx_") || strings.Contains(meta.RequestMetric, "rpc_") {
				errorLabel = "status"
			}
		}
		
		// ALWAYS generate success SLO for discovered services
		// If no error classification is found, we'll still generate one, 
		// allowing the user to see the metric and fix it manually.
		slos = append(slos, flowSLO{
			ID:        fmt.Sprintf("%s_success", id),
			Service:   svc,
			Objective: successObjective,
			Window:    "15m",
			Type:      "ratio",
			ErrorQuery: generateErrorQuery(meta.RequestMetric, svc, errorLabel, meta.ServiceLabel, "15m", meta.MetricSchema),
			TotalQuery: generateTotalQuery(meta.RequestMetric, svc, meta.ServiceLabel, "15m"),
		})

		// Latency SLO - ALWAYS generate if we have a metric name
		latencyThreshold := 0.5
		if meta.P95Baseline > 0 {
			// Set threshold to 2x baseline or 500ms min
			latencyThreshold = meta.P95Baseline * 2
			if latencyThreshold < 0.2 {
				latencyThreshold = 0.2
			}
		}

		slos = append(slos, flowSLO{
			ID:        fmt.Sprintf("%s_latency", id),
			Service:   svc,
			Objective: 90,
			Window:    "15m",
			Type:      "latency",
			ThresholdSeconds: latencyThreshold,
			PromQL:    generateLatencyQuery(meta.LatencyMetric, svc, meta.ServiceLabel, "15m"),
		})

		file.Flows[id] = flowEntry{
			Name:     fmt.Sprintf("%s Auto-Discovered Flow", svc),
			Services: []string{svc},
			SLOs:     slos,
		}
	}

	data, err := yaml.Marshal(file)
	if err != nil {
		return err
	}

	header := fmt.Sprintf("# %s Flows Configuration (Auto-Generated)\n", profileName)
	content := header + string(data)

	return os.WriteFile(flowsPath, []byte(content), 0600)
}

func generateErrorQuery(metric, service, errorLabel, serviceLabel, window, schema string) string {
	if schema == "otel" || metric == "spanmetrics_calls_total" {
		// OTel standard: status_code is the most reliable
		// 0=OK, 1=Unset, 2=Error. We want to catch 1 and 2 as potential issues, or at least 2.
		// Many exporters also map status_code to "OK"/"Error" strings.
		return fmt.Sprintf(`%s{%s=%q, status_code!~"0|OK"}`, metric, serviceLabel, service)
	}
	labels := strings.Split(serviceLabel, ",")
	if len(labels) == 0 || (len(labels) == 1 && labels[0] == "") {
		labels = []string{"service", "app", "job", "container"}
	}
	
	var selectorParts []string
	for _, l := range labels {
		l = strings.TrimSpace(l)
		if l == "" { continue }
		if errorLabel != "" {
			selectorParts = append(selectorParts, fmt.Sprintf(`%s{%s="%s", %s!~"2..|OK"}`, metric, l, service, errorLabel))
		} else {
			// If we don't know the error label, default to "status" to be safe.
			// This query will return 0 if the label doesn't exist, which is safer 
			// (SLO stays healthy) than returning everything (SLO breaches instantly).
			selectorParts = append(selectorParts, fmt.Sprintf(`%s{%s="%s", status!~"2..|OK"}`, metric, l, service))
		}
	}
	
	if len(selectorParts) > 1 {
		return "(" + strings.Join(selectorParts, " or ") + ")"
	} else if len(selectorParts) == 1 {
		return selectorParts[0]
	}
	return metric
}

func generateTotalQuery(metric, service, serviceLabel, window string) string {
	labels := strings.Split(serviceLabel, ",")
	if len(labels) == 0 || (len(labels) == 1 && labels[0] == "") {
		labels = []string{"service", "app", "job", "container"}
	}
	
	var selectorParts []string
	for _, l := range labels {
		l = strings.TrimSpace(l)
		if l == "" { continue }
		selectorParts = append(selectorParts, fmt.Sprintf(`%s{%s="%s"}`, metric, l, service))
	}
	
	if len(selectorParts) > 1 {
		return "(" + strings.Join(selectorParts, " or ") + ")"
	} else if len(selectorParts) == 1 {
		return selectorParts[0]
	}
	return metric
}

func generateLatencyQuery(metric, service, serviceLabel, window string) string {
	labels := strings.Split(serviceLabel, ",")
	if len(labels) == 0 || (len(labels) == 1 && labels[0] == "") {
		labels = []string{"service", "app", "job", "container"}
	}
	
	bucketMetric := metric
	if !strings.HasSuffix(metric, "_bucket") {
		bucketMetric = metric + "_bucket"
	}
	
	var selectorParts []string
	for _, l := range labels {
		l = strings.TrimSpace(l)
		if l == "" { continue }
		selectorParts = append(selectorParts, fmt.Sprintf(`%s{%s="%s"}`, bucketMetric, l, service))
	}
	
	if len(selectorParts) > 1 {
		selector := "(" + strings.Join(selectorParts, " or ") + ")"
		return fmt.Sprintf("histogram_quantile(0.95, sum(rate(%s[%s])) by (le))", selector, window)
	} else if len(selectorParts) == 1 {
		return fmt.Sprintf("histogram_quantile(0.95, sum(rate(%s[%s])) by (le))", selectorParts[0], window)
	}

	return fmt.Sprintf("histogram_quantile(0.95, sum(rate(%s[%s])) by (le))", bucketMetric, window)
}

