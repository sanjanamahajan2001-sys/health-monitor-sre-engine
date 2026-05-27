package tui

import (
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"health-monitor/pkg/model"
)

func padRight(value string, width int) string {
	vWidth := lipgloss.Width(value)
	if vWidth >= width {
		return value
	}
	return value + strings.Repeat(" ", width-vWidth)
}

func buildDiagnosticHints(latency *model.APILatency) []string {
	if latency == nil {
		return nil
	}
	hints := []string{}
	if strings.TrimSpace(latency.SpikeNote) != "" {
		hints = append(hints, latency.SpikeNote)
	}
	if len(latency.ActiveAlerts) > 0 {
		hints = append(hints, "Active alerts are firing; investigate alert conditions and recent deployments.")
	}
	if len(latency.TopEndpointRegressions) > 0 {
		hints = append(hints, "Some endpoints show increased P95 vs baseline; focus on the top regressions.")
	}
	if !math.IsNaN(latency.DeltaErrorRate) {
		switch {
		case latency.DeltaErrorRate >= 1.0:
			hints = append(hints, "Error rate increased vs baseline; check error logs for the affected endpoints.")
		case latency.DeltaErrorRate <= -1.0:
			hints = append(hints, "Error rate decreased vs baseline; recent changes may have improved stability.")
		default:
			hints = append(hints, "Error rate is stable vs baseline; issue may be performance rather than errors.")
		}
	}
	if !math.IsNaN(latency.DeltaP95) && latency.DeltaP95 >= 0.5 {
		hints = append(hints, "Latency increased materially vs baseline; check upstream dependencies or recent config changes.")
	}
	if math.IsNaN(latency.DeltaP95) && math.IsNaN(latency.DeltaErrorRate) {
		hints = append(hints, "Baseline data unavailable; try a longer window or verify Prometheus retention.")
	}
	return hints
}

func isLogQLQuery(query string) bool {
	q := strings.TrimSpace(query)
	return strings.Contains(q, "|~") || strings.Contains(q, "|=") || strings.Contains(q, "| ")
}

func escapeJSON(value string) string {
	out := strings.ReplaceAll(value, "\\", "\\\\")
	out = strings.ReplaceAll(out, "\"", "\\\"")
	return out
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
