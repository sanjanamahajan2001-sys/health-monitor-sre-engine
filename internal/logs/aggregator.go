package logs

import (
	"context"
	"fmt"
	"time"
)

// ErrorStat represents a aggregated error statistic
type ErrorStat struct {
	Message string `json:"message"`
	Count   int    `json:"count"`
	Sample  string `json:"sample,omitempty"`
}

// LogBackend defines the interface for log backends
type LogBackend interface {
	// TopErrors returns the top N error messages for a service in a time window
	TopErrors(ctx context.Context, service string, from, to time.Time) ([]ErrorStat, error)
}

// LogSummary represents the log correlation data attached to incidents
type LogSummary struct {
	Backend   string      `json:"backend"`
	Window    string      `json:"window"`
	TopErrors []ErrorStat `json:"top_errors"`
}

// Aggregator handles log correlation across different backends
type Aggregator struct {
	backend LogBackend
	window  time.Duration
}

// NewAggregator creates a new log aggregator
func NewAggregator(backend LogBackend, window time.Duration) *Aggregator {
	return &Aggregator{
		backend: backend,
		window:  window,
	}
}

// CorrelateLogs fetches and aggregates logs for a service in a time window
func (a *Aggregator) CorrelateLogs(ctx context.Context, service string, start time.Time, end time.Time) (*LogSummary, error) {
	if start.IsZero() {
		start = time.Now().Add(-15 * time.Minute)
	}
	if end.IsZero() {
		end = time.Now().UTC()
	}

	// Get top errors from the backend
	topErrors, err := a.backend.TopErrors(ctx, service, start, end)
	if err != nil {
		return nil, err
	}

	// Create log summary
	summary := &LogSummary{
		Backend:   getBackendName(a.backend),
		Window:    fmt.Sprintf("from %s to %s", start.Format("15:04:05"), end.Format("15:04:05")),
		TopErrors: topErrors,
	}

	return summary, nil
}

// CorrelateLogsResilient fetches logs with optional retry logic for empty results (handling ingestion lag)
func (a *Aggregator) CorrelateLogsResilient(ctx context.Context, service string, start time.Time, end time.Time, retryOnEmpty bool) (*LogSummary, error) {
	maxAttempts := 1
	if retryOnEmpty {
		maxAttempts = 5
	}
	
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		timeoutCtx, cancel := context.WithTimeout(ctx, 8*time.Second) // Increased from 3s for lookback reliability
		summary, err := a.CorrelateLogs(timeoutCtx, service, start, end)
		cancel()
		
		// If we succeed and either find errors OR we don't care about retrying, return
		if err == nil && summary != nil {
			if len(summary.TopErrors) > 0 || !retryOnEmpty {
				return summary, nil
			}
		}
		
		if attempt == maxAttempts {
			break
		}
		
		// Wait before retry (handling Loki index lag)
		select {
		case <-time.After(2 * time.Second):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	
	// Fallback to empty summary if we exhausted retries
	return &LogSummary{
		Backend:   getBackendName(a.backend),
		Window:    fmt.Sprintf("last %s to %s", start.Format("15:04:05"), end.Format("15:04:05")),
		TopErrors: []ErrorStat{},
	}, nil
}

// getBackendName returns the backend type name
func getBackendName(backend LogBackend) string {
	switch backend.(type) {
	case *LokiClient:
		return "loki"
	case *ElasticsearchClient:
		return "elastic"
	default:
		return "unknown"
	}
}
