package slo

import (
	"fmt"
	"health-monitor/internal/alert"
	"log"
	"time"
)

// ResultToAlert converts an SLOResult into an alert.Alert
func ResultToAlert(res SLOResult) alert.Alert {
	service := res.Service
	if service == "" {
		service = res.FlowID
	}

	labels := map[string]string{
		"alertname": "SLOBreach",
		"service":   service,
		"flow_id":   res.FlowID,
		"slo_id":    res.SLOID,
		"slo_type":  res.Type,
		"source":    "slo",
	}

	annotations := map[string]string{
		"summary":     fmt.Sprintf("SLO %s for service %s is %s", res.SLOID, res.FlowID, res.Compliance),
		"description": fmt.Sprintf("Current: %.2f (Objective: %.2f). Risk: %s. Burn Rate: %.2f", res.Current, res.Objective, res.Risk, res.BurnRate),
		"compliance":  res.Compliance,
		"risk":        res.Risk,
	}

	return alert.Alert{
		Status:      "firing",
		Labels:      labels,
		Annotations: annotations,
		StartsAt:    time.Now(),
	}
}

// ProcessSLOResults handles a batch of SLO results by feeding them into the alert processor
func ProcessSLOResults(processor *alert.Processor, results []SLOResult) {
	var alerts []alert.Alert
	for _, res := range results {
		// Only notify on BREACHING or if risk is HIGH/CRITICAL
		if res.Compliance == "BREACHING" || res.Risk == "HIGH" || res.Risk == "CRITICAL" {
			alerts = append(alerts, ResultToAlert(res))
		}
	}

	if len(alerts) == 0 {
		return
	}

	payload := alert.WebhookPayload{
		Status: "firing",
		Alerts: alerts,
	}

	// The alert processor will handle deduplication, escalation, etc.
	decisions, warnings := processor.Process(payload, false)
	
	if len(decisions) > 0 {
		for _, d := range decisions {
			log.Printf("DEBUG: SLO Alert Processor Decision: %s", d.Message)
		}
	}
	if len(warnings) > 0 {
		for _, w := range warnings {
			log.Printf("WARN: SLO Alert Processor Warning: %s", w)
		}
	}
}
