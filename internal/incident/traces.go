package incident

import (
	"context"
	"fmt"
	"health-monitor/internal/output"
	"strings"
	"time"

	"health-monitor/internal/config"
	"health-monitor/internal/traces"
)

// TraceCorrelator handles trace correlation for incidents
type TraceCorrelator struct {
	correlator *traces.Correlator
}

// NewTraceCorrelator creates a new trace correlator based on configuration
func NewTraceCorrelator(cfg config.Config) (*TraceCorrelator, error) {
	// Create trace correlator
	correlator, err := traces.NewCorrelator(cfg)
	if err != nil {
		// If trace backend is not configured, return a no-op correlator
		if strings.Contains(err.Error(), "no trace backend configured") ||
		   strings.Contains(err.Error(), "TRACE_URL is not configured") {
			output.Infof("Trace correlation disabled: %v", err)
			return &TraceCorrelator{correlator: nil}, nil
		}
		return nil, fmt.Errorf("failed to create trace correlator: %w", err)
	}

	return &TraceCorrelator{
		correlator: correlator,
	}, nil
}

// Correlate is a proxy for the internal correlator's Correlate method
func (tc *TraceCorrelator) Correlate(ctx context.Context, service string, startTime time.Time, endTime time.Time) (*traces.TraceSummary, error) {
	if tc.correlator == nil {
		return nil, fmt.Errorf("trace correlation not configured")
	}
	return tc.correlator.Correlate(ctx, service, startTime, endTime)
}

// CorrelateTracesResilient performs trace correlation with retry logic
func (tc *TraceCorrelator) CorrelateTracesResilient(ctx context.Context, service string, incidentStart time.Time, endTime time.Time) *traces.TraceSummary {
	if tc.correlator == nil {
		return nil
	}

	maxAttempts := 5
	for attempt := 0; attempt < maxAttempts; attempt++ {
		output.Debugf("Trace correlation attempt %d for service %s", attempt+1, service)
		
		attemptCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		summary, err := tc.correlator.Correlate(attemptCtx, service, incidentStart, endTime)
		cancel()
		
		if err == nil && summary != nil && summary.TraceCount > 0 {
			return summary
		}
		
		if attempt < maxAttempts-1 {
			time.Sleep(2 * time.Second)
		}
	}
	
	return &traces.TraceSummary{
		Backend:    "unknown",
		Window:     "",
		TraceCount: 0,
	}
}

// CorrelateTracesLive performs live trace correlation when viewing an incident
func (tc *TraceCorrelator) CorrelateTracesLive(ctx context.Context, service string, incidentStart time.Time, endTime time.Time) (*traces.TraceSummary, error) {
	if tc.correlator == nil {
		return nil, fmt.Errorf("trace correlation not configured")
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	summary, err := tc.correlator.Correlate(ctx, service, incidentStart, endTime)
	if err != nil {
		return nil, fmt.Errorf("live trace correlation failed: %w", err)
	}

	return summary, nil
}

// IsEnabled returns true if trace correlation is configured and available
func (tc *TraceCorrelator) IsEnabled() bool {
	return tc.correlator != nil
}
