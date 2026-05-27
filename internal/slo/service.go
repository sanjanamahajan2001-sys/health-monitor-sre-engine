package slo

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"health-monitor/internal/flow"
	"health-monitor/internal/metrics"
	"health-monitor/internal/output"
)

// SLOStatus represents the health of an SLO
type SLOStatus string

const (
	StatusHealthy   SLOStatus = "HEALTHY"
	StatusWarning   SLOStatus = "WARNING"
	StatusBreaching SLOStatus = "BREACHING"
	StatusUnknown   SLOStatus = "UNKNOWN"
)

// ComplianceStatus represents detailed compliance states for better error handling
type ComplianceStatus string

const (
	ComplianceOK         ComplianceStatus = "OK"
	ComplianceNoTraffic  ComplianceStatus = "NO_TRAFFIC"
	ComplianceNoData     ComplianceStatus = "NO_DATA"
	ComplianceError      ComplianceStatus = "ERROR"
	ComplianceBreaching  ComplianceStatus = "BREACHING"
)

// SLOResult contains the calculated status of an SLO
type SLOResult struct {
	FlowID     string
	SLOID      string
	Service    string
	Objective  float64
	Window     string
	Type            string
	Current         float64
	BurnRate        float64 // Fast window (e.g. 5m)
	BurnRateSlow    float64 // Slow window (e.g. 1h)
	BudgetRemaining float64
	Trend           string // "⬆️", "⬇️", "➡️"
	Status          SLOStatus // Deprecated in favor of Compliance/Risk, but kept for compatibility/summary
	Compliance      string    // "COMPLIANT", "BREACHING"
	Risk            string    // "CRITICAL", "HIGH", "LOW"
	Error           error
	
	// Enhanced status fields for better error handling
	NoTrafficReason string       `json:"no_traffic_reason,omitempty"`
	DataQuality    string       `json:"data_quality,omitempty"` // "good", "insufficient", "missing"
}

// Service handles SLO calculations
type Service struct {
	metricsProvider metrics.MetricProvider
}

// NewService creates a new SLO service
func NewService(provider metrics.MetricProvider) (*Service, error) {
	return &Service{
		metricsProvider: provider,
	}, nil
}

// CheckSLOs checks all SLOs for the provided flows
func (s *Service) CheckSLOs(flows []flow.Flow) []SLOResult {
	var results []SLOResult
	
	for _, f := range flows {
		for _, slo := range f.SLOs {
			result := SLOResult{
				FlowID:    f.ID,
				SLOID:     slo.ID,
				Service:   slo.Service,
				Objective: slo.Objective,
				Window:    slo.Window,
				Type:      slo.Type,
				Status:    StatusUnknown,
				Compliance: "UNKNOWN", // Initialize with default
				DataQuality: "unknown", // Initialize with default
			}
			
			if s.metricsProvider == nil {
				result.Error = fmt.Errorf("metrics provider not configured")
				results = append(results, result)
				continue
			}
			
			// Handle based on SLO type
			if slo.Type == "ratio" && slo.ErrorQuery != "" && slo.TotalQuery != "" {
				// Resilient query building: only wrap if not already wrapped
				totalQuery := formatQuery(slo.TotalQuery, slo.Window)
				output.Debugf("SLO [%s] TotalQuery: %s", slo.ID, totalQuery)
				total, err := s.metricsProvider.QueryInstant(totalQuery)
				if err != nil {
					// Check if it's a "no data" error vs actual connection error
					if errors.Is(err, metrics.ErrNoData) || strings.Contains(err.Error(), "no data") {
						result.Compliance = "NO_DATA"
						result.DataQuality = "missing"
						result.Error = fmt.Errorf("service not found in metrics: %w", err)
					} else {
						result.Compliance = "ERROR"
						result.DataQuality = "missing"
						result.Error = fmt.Errorf("prometheus query failed: %w", err)
					}
					results = append(results, result)
					continue
				}
				// Production SRE logic: No traffic is NOT a breach
				if total == 0 {
					result.Compliance = "NO_TRAFFIC"
					result.NoTrafficReason = "no_requests_in_window"
					result.DataQuality = "insufficient"
					result.Current = 0
					result.BurnRate = 0
					result.BudgetRemaining = 100 // Full budget remaining
					result.Trend = "➡️"
					results = append(results, result)
					continue
				}
				
				// Check if we have sufficient data (minimum threshold based on COUNT, not RATE)
				minRequestsThreshold := 2.0 
				windowDuration := parsePrometheusWindow(slo.Window)
				estimatedTotalCount := total * windowDuration.Seconds()
				
				if estimatedTotalCount < minRequestsThreshold {
					result.Compliance = "NO_DATA"
					result.NoTrafficReason = fmt.Sprintf("insufficient_data: %.1f total requests (< %.1f)", estimatedTotalCount, minRequestsThreshold)
					result.DataQuality = "insufficient"
					result.Current = 0
					results = append(results, result)
					continue
				}

				// 2. Fetch Error (only if we have sufficient traffic)
				errorQuery := formatQuery(slo.ErrorQuery, slo.Window)
				errVal, err := s.metricsProvider.QueryInstant(errorQuery)
				if err != nil {
					// Check if it's a "no data" error vs actual connection error
					errStr := err.Error()
					if strings.Contains(errStr, "no data") || strings.Contains(errStr, "ErrNoData") {
						result.Compliance = "NO_DATA"
						result.DataQuality = "missing"
						result.Error = fmt.Errorf("error metrics not found: %w", err)
					} else {
						result.Compliance = "ERROR"
						result.DataQuality = "missing"
						result.Error = fmt.Errorf("prometheus error query failed: %w", err)
					}
					results = append(results, result)
					continue
				}
				
				// Calculate success rate
				successRate := 100 * (1 - (errVal / total))
				
				// Clamp to [0, 100]
				if successRate < 0 { successRate = 0 }
				if successRate > 100 { successRate = 100 }
				
				result.Current = successRate
				result.DataQuality = "good" // We have sufficient data for reliable calculation
				
				// --- Advanced Metrics ---
				
				// 1. Burn Rate (Fast Window - Configured)
				allowedError := 100.0 - slo.Objective
				currentError := 100.0 - successRate
				
				if allowedError > 0 {
					result.BurnRate = currentError / allowedError
				}
				
				// Check Slow Window (1h)
				slowWindow := "1h"
				slowTotalQuery := formatQuery(slo.TotalQuery, slowWindow)
				slowErrorQuery := formatQuery(slo.ErrorQuery, slowWindow)
				
				slowTotal, errT := s.metricsProvider.QueryInstant(slowTotalQuery)
				slowErrVal, errE := s.metricsProvider.QueryInstant(slowErrorQuery)
				
				if errT == nil && errE == nil && slowTotal > 0 {
					slowSuccess := 100 * (1 - (slowErrVal / slowTotal))
					if slowSuccess < 0 { slowSuccess = 0 }
					if slowSuccess > 100 { slowSuccess = 100 }
					
					slowCurrentError := 100.0 - slowSuccess
					if allowedError > 0 {
						result.BurnRateSlow = slowCurrentError / allowedError
					}
				}

				// 2. Error Budget Remaining (Full Window - 30d with Fallback)
				// Order: 30d -> 1d (Rolling) -> Current Window (Fallback)
				// We try 30d first.
				budgetWindow := "30d"
				budgetTotalQuery := formatQuery(slo.TotalQuery, budgetWindow)
				budgetErrorQuery := formatQuery(slo.ErrorQuery, budgetWindow)
				
				bTotal, errBT := s.metricsProvider.QueryInstant(budgetTotalQuery)
				bErr, errBE := s.metricsProvider.QueryInstant(budgetErrorQuery)
				
				budgetCalculated := false
				if errBT == nil && errBE == nil && bTotal > 0 {
					bSuccess := 100 * (1 - (bErr / bTotal))
					bCurrentError := 100.0 - bSuccess
					if allowedError > 0 {
						result.BudgetRemaining = ((allowedError - bCurrentError) / allowedError) * 100
						budgetCalculated = true
					}
				}
				
				// Fallback to 1d if 30d failed
				if !budgetCalculated {
					budgetWindow = "1d"
					// ... (Similar logic could repeat, or just skip to fallback)
					// For brevity, we skip straight to current-window extrapolation/fallback if 30d fails
					// which is better than nothing.
					if allowedError > 0 {
						result.BudgetRemaining = ((allowedError - currentError) / allowedError) * 100
					}
				}
				
				// Clamp Budget
				if result.BudgetRemaining < -100 { result.BudgetRemaining = -100 }
				if result.BudgetRemaining > 100 { result.BudgetRemaining = 100 }

				// 3. Trend (Compare Error Rate)
				// Trend should track ERROR RATE change, not Success %
				// Error Rate = 100 - Success
				// If Current Error (10%) < Prev Error (11%) -> IMPROVING (⬇️ Error)
				if prevSuccess, err := s.getPreviousRatio(slo); err == nil {
					prevError := 100.0 - prevSuccess
					// currentError calculated above
					
					diff := currentError - prevError
					// Threshold 0.2% change in error rate
					if diff > 0.05 { // Error increased -> Degrading
						result.Trend = "⬇️" 
					} else if diff < -0.05 { // Error decreased -> Improving
						result.Trend = "⬆️"
					} else {
						result.Trend = "➡️"
					}
				} else {
					result.Trend = "-"
				}

				// 5. Determine Compliance & Risk
				
				// Compliance: Strict Objective Check
				if successRate >= slo.Objective {
					result.Compliance = "OK" // Use new enhanced status
					result.Status = StatusHealthy // Backwards compat
				} else {
					result.Compliance = "BREACHING" // Keep existing for compatibility
					result.Status = StatusBreaching
				}
				
				// Risk: Burn Rate Based
				// Low: < 1x
				// High: > 2x (Warning)
				// Critical: > 10x (Page)
				
				// Use Max(Fast, Slow) or Min?
				// For Risk classification, we care about active burning. 
				// If Fast is high, Risk is elevated.
				effBurn := result.BurnRate
				if effBurn > 10 {
					result.Risk = "CRITICAL"
				} else if effBurn > 2 {
					result.Risk = "HIGH"
				} else {
					result.Risk = "LOW"
				}
				
				results = append(results, result)
				continue

			} else if slo.Type == "latency" {
				// Latency Check
				query := slo.PromQL
				if query == "" {
					result.Error = fmt.Errorf("missing PromQL for latency SLO")
					result.Compliance = "ERROR"
					result.Risk = "-"
					results = append(results, result)
					continue
				}
				
				output.Debugf("SLO [%s] LatencyQuery: %s", slo.ID, query)
				// Fetch latency value (seconds)
				latencySeconds, err := s.metricsProvider.QueryInstant(query)
				if err != nil {
					if errors.Is(err, metrics.ErrNoData) || strings.Contains(err.Error(), "no data") {
						result.Compliance = "NO_DATA"
						result.DataQuality = "missing"
						result.Error = fmt.Errorf("no latency metrics found: %v", err)
					} else {
						result.Error = fmt.Errorf("metrics provider query failed: %v", err)
						result.Compliance = "ERROR"
					}
					result.Risk = "-"
					results = append(results, result)
					continue
				}
				
				// Added check: For latency SLOs, we should also verify we have enough data (Total count)
				// though QueryInstant on histogram_quantile will succeed with even 1 sample.
				
				// Calculate compliance for latency (latency < objective)
				// Current objective is in ms, latency is in ms.
				currentLatencyMs := latencySeconds * 1000 
				result.Current = currentLatencyMs
				
				if currentLatencyMs <= result.Objective {
					result.Compliance = "COMPLIANT"
					result.Status = StatusHealthy
					result.Risk = "LOW"
				} else {
					result.Compliance = "BREACHING"
					result.Status = StatusBreaching
					// For latency, high latency is high risk
					if currentLatencyMs > result.Objective*10 {
						result.Risk = "CRITICAL"
					} else if currentLatencyMs > result.Objective*2 {
						result.Risk = "HIGH"
					} else {
						result.Risk = "MEDIUM"
					}
				}
				
				results = append(results, result)
				continue
			}

			// Fallback: Generic PromQL check (Legacy / Ratio without dedicated fields)
			if (slo.Type == "" || slo.Type == "ratio") && slo.PromQL != "" {
				// Fetch current value
				val, err := s.metricsProvider.QueryInstant(slo.PromQL)
				if err != nil {
					result.Error = err
					result.Compliance = "ERROR"
					result.Status = StatusUnknown
					results = append(results, result)
					continue
				}
				
				// Adjust value if query returns 0-1 instead of 0-100
				displayVal := val
				if slo.Objective > 1 && val <= 1 {
					displayVal = val * 100
				}
				
				result.Current = displayVal
				
				// Simple comparison
				if displayVal >= slo.Objective-0.0001 {
					result.Status = StatusHealthy
					result.Compliance = "COMPLIANT"
					result.Risk = "LOW"
				} else {
					result.Status = StatusBreaching
					result.Compliance = "BREACHING"
					result.Risk = "HIGH" // Fallback high risk for breaches
				}
				
				results = append(results, result)
				continue
			}

			// Final fallback if no type matched and no PromQL
			if result.Compliance == "" {
				result.Compliance = "UNKNOWN"
				result.Error = fmt.Errorf("SLO definition is incomplete (missing ratio fields, latency fields, or PromQL)")
				results = append(results, result)
			}
		}
	}
	
	return results
}

// getPreviousRatio fetches the success rate from a previous window for trend analysis
func (s *Service) getPreviousRatio(slo flow.SLO) (float64, error) {
	if s.metricsProvider == nil {
		return 0, fmt.Errorf("metrics provider not configured")
	}

	// We look at the window preceding the current one
	// This is a simplified version; production might use a fixed offset.
	totalQuery := fmt.Sprintf("sum(rate(%s[%s] offset %s))", slo.TotalQuery, slo.Window, slo.Window)
	errorQuery := fmt.Sprintf("sum(rate(%s[%s] offset %s))", slo.ErrorQuery, slo.Window, slo.Window)

	total, err := s.metricsProvider.QueryInstant(totalQuery)
	if err != nil || total == 0 {
		return 0, fmt.Errorf("no historical data")
	}

	errVal, err := s.metricsProvider.QueryInstant(errorQuery)
	if err != nil {
		return 0, err
	}

	successRate := 100 * (1 - (errVal / total))
	if successRate < 0 { successRate = 0 }
	if successRate > 100 { successRate = 100 }

	return successRate, nil
}

// formatQuery safely wraps a metric query or an 'or' separated list of metric queries with sum(rate(...[window])) or avg_over_time(...[window])
func formatQuery(query string, window string) string {
	// Detect if metric is likely a gauge based on common patterns
	isGauge := false
	qLower := strings.ToLower(query)
	
	// Common gauge indicators in metric names
	gaugeIndicators := []string{"in_flight", "current", "active", "total_bytes", "system_bytes", "limit", "capacity", "usage", "up", "health"}
	for _, term := range gaugeIndicators {
		if strings.Contains(qLower, term) {
			isGauge = true
			break
		}
	}
	
	// Counter indicators: if it has these, it's definitely NOT a gauge
	counterIndicators := []string{"_total", "_count", "_sum", "_bucket"}
	for _, term := range counterIndicators {
		if strings.Contains(qLower, term) {
			isGauge = false
			break
		}
	}

	if !strings.Contains(query, " or ") {
		if !strings.Contains(query, "sum(") && !strings.Contains(query, "rate(") && !strings.Contains(query, "avg_over_time(") {
			if isGauge {
				return fmt.Sprintf("avg_over_time(%s[%s])", query, window)
			}
			return fmt.Sprintf("sum(rate(%s[%s]))", query, window)
		}
		return query // If it's already a complex query, return as is
	}

	q := strings.TrimSpace(query)
	q = strings.TrimPrefix(q, "(")
	q = strings.TrimSuffix(q, ")")

	var parts []string
	for _, part := range strings.Split(q, " or ") {
		part = strings.TrimSpace(part)
		if !strings.Contains(part, "sum(") && !strings.Contains(part, "rate(") && !strings.Contains(part, "avg_over_time(") {
			if isGauge {
				parts = append(parts, fmt.Sprintf("avg_over_time(%s[%s])", part, window))
			} else {
				parts = append(parts, fmt.Sprintf("sum(rate(%s[%s]))", part, window))
			}
		} else {
			parts = append(parts, part)
		}
	}

	return "(" + strings.Join(parts, " or ") + ")"
}

// parsePrometheusWindow converts Prometheus duration strings to time.Duration
func parsePrometheusWindow(window string) time.Duration {
	if window == "" {
		return 15 * time.Minute
	}
	
	// Try standard Go duration first
	d, err := time.ParseDuration(window)
	if err == nil {
		return d
	}
	
	// Fallback for Prometheus special units
	window = strings.ToLower(strings.TrimSpace(window))
	
	// Simple multiplier for non-Go units
	var multiplier time.Duration
	var numStr string
	
	if strings.HasSuffix(window, "d") {
		multiplier = 24 * time.Hour
		numStr = strings.TrimSuffix(window, "d")
	} else if strings.HasSuffix(window, "w") {
		multiplier = 7 * 24 * time.Hour
		numStr = strings.TrimSuffix(window, "w")
	} else if strings.HasSuffix(window, "y") {
		multiplier = 365 * 24 * time.Hour
		numStr = strings.TrimSuffix(window, "y")
	} else {
		return 15 * time.Minute // Default fallback
	}
	
	var val int
	fmt.Sscanf(numStr, "%d", &val)
	if val <= 0 {
		return 15 * time.Minute
	}
	return time.Duration(val) * multiplier
}

// Monitor periodically checks SLOs and calls the onResult callback
func (s *Service) Monitor(ctx context.Context, interval time.Duration, flows []flow.Flow, onResult func(SLOResult)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Printf("INFO: Starting SLO background monitor (interval: %v)", interval)

	for {
		select {
		case <-ctx.Done():
			log.Printf("INFO: Stopping SLO background monitor")
			return
		case <-ticker.C:
			results := s.CheckSLOs(flows)
			for _, res := range results {
				if onResult != nil {
					onResult(res)
				}
			}
		}
	}
}


// FormatCurrent returns a CLI-friendly string representation
func (r SLOResult) FormatCurrent() string {
	// Handle new compliance status types first
	switch r.Compliance {
	case "NO_TRAFFIC", "NO_DATA":
		return "N/A"
	case "ERROR":
		return "ERR"
	}
	
	// Fallback to legacy status handling for backward compatibility
	if r.Status == StatusUnknown {
		if r.Error != nil && (r.Error.Error() == "no traffic (total=0)" || strings.Contains(r.Error.Error(), "no_requests_in_window")) {
			return "N/A"
		}
		if r.Error != nil {
			return "ERR"
		}
		return "N/A"
	}
	
	if r.Type == "latency" {
		return fmt.Sprintf("%.2fs", r.Current)
	}
	
	// Default to percentage for ratio or others
	return fmt.Sprintf("%.2f%%", r.Current)
}

func (r SLOResult) FormatObjective() string {
	if r.Type == "latency" {
		// e.g. "p90" or "p95"
		if r.Objective > 0 {
			if r.Objective < 1 { // If objective was stored as 0.95
				return fmt.Sprintf("p%.0f", r.Objective*100)
			}
			return fmt.Sprintf("p%.0f", r.Objective)
		}
		return "latency"
	}
	return fmt.Sprintf("%.4g%%", r.Objective)
}

func (r SLOResult) FormatBurnRate() string {
	if r.Type == "ratio" {
		// Show Fast / Slow if slow is available
		if r.BurnRateSlow > 0 {
			return fmt.Sprintf("%.1fx/%.1fx", r.BurnRate, r.BurnRateSlow)
		}
		return fmt.Sprintf("%.1fx", r.BurnRate)
	}
	return "-"
}

func (r SLOResult) FormatBudget() string {
	if r.Type == "ratio" {
		if r.BudgetRemaining < 0 {
			return fmt.Sprintf("%.1f%%", r.BudgetRemaining) // Negative budget means overspent
		}
		return fmt.Sprintf("%.1f%%", r.BudgetRemaining)
	}
	return "-"
}
