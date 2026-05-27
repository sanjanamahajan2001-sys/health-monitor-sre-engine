package incident

import (
	"context"
	"fmt"
	"health-monitor/internal/output"
	"strings"
	"time"

	"health-monitor/internal/config"
	"health-monitor/internal/logs"
)

// LogCorrelator handles log correlation for incidents
type LogCorrelator struct {
	aggregator *logs.Aggregator
}

// NewLogCorrelator creates a new log correlator based on configuration
func NewLogCorrelator(cfg config.Config) (*LogCorrelator, error) {
	// Determine window duration
	window := 5 * time.Minute // Default window
	if cfg.Window != "" {
		if parsed, err := time.ParseDuration(cfg.Window); err == nil {
			window = parsed
		}
	}

	// Create backend based on configuration
	var backend logs.LogBackend
	var err error

	switch strings.ToLower(cfg.LogBackend) {
	case "loki":
		if cfg.LokiURL == "" {
			return nil, fmt.Errorf("Loki backend specified but LOKI_URL is not configured")
		}
		backend, err = logs.NewLokiClient(
			cfg.LokiURL,
			cfg.LokiUser,
			cfg.LokiPass,
			cfg.LokiToken,
			cfg.LokiServiceLabel,
			cfg.LokiErrorRegex,
		)
	case "elastic", "elasticsearch":
		if cfg.ElasticURL == "" {
			return nil, fmt.Errorf("Elasticsearch backend specified but ELASTIC_URL is not configured")
		}
		backend, err = logs.NewElasticsearchClient(
			cfg.ElasticURL,
			cfg.ElasticUser,
			cfg.ElasticPass,
			cfg.ElasticToken,
			cfg.ElasticIndex,
			cfg.ElasticServiceField,
			cfg.ElasticErrorField,
			cfg.ElasticTimeField,
			cfg.ElasticErrorRegex,
		)
	default:
		// Auto-detect backend based on available configuration
		if cfg.LokiURL != "" {
			backend, err = logs.NewLokiClient(
				cfg.LokiURL,
				cfg.LokiUser,
				cfg.LokiPass,
				cfg.LokiToken,
				cfg.LokiServiceLabel,
				cfg.LokiErrorRegex,
			)
		} else if cfg.ElasticURL != "" {
			backend, err = logs.NewElasticsearchClient(
				cfg.ElasticURL,
				cfg.ElasticUser,
				cfg.ElasticPass,
				cfg.ElasticToken,
				cfg.ElasticIndex,
				cfg.ElasticServiceField,
				cfg.ElasticErrorField,
				cfg.ElasticTimeField,
				cfg.ElasticErrorRegex,
			)
		} else {
			// No log backend configured
			return nil, nil
		}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create log backend: %w", err)
	}

	if backend == nil {
		return nil, nil // No log backend available
	}

	aggregator := logs.NewAggregator(backend, window)

	return &LogCorrelator{
		aggregator: aggregator,
	}, nil
}

// CorrelateLogs performs log correlation for an incident
func (lc *LogCorrelator) CorrelateLogs(ctx context.Context, incident Incident, endTime time.Time) (*LogSummary, error) {
	if lc.aggregator == nil {
		return nil, nil
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second) // Increased from 5s
	defer cancel()

	output.Debugf("Starting resilient log correlation for incident %s, service %s", incident.ID, incident.Service)
	summary, err := lc.aggregator.CorrelateLogsResilient(timeoutCtx, incident.Service, incident.CreatedAt, endTime, true)
	if err != nil {
		output.Warnf("Log correlation failed for incident %s: %v", incident.ID, err)
		return nil, err
	}

	output.Debugf("Resilient log correlation completed for incident %s, found %d error types", incident.ID, len(summary.TopErrors))
	
	incidentSummary := &LogSummary{
		Backend:   summary.Backend,
		Window:    summary.Window,
		TopErrors: convertErrorStats(summary.TopErrors),
	}
	
	return incidentSummary, nil
}

// convertErrorStats converts []logs.ErrorStat to []ErrorStat
func convertErrorStats(logStats []logs.ErrorStat) []ErrorStat {
	stats := make([]ErrorStat, len(logStats))
	for i, stat := range logStats {
		stats[i] = ErrorStat{
			Message: stat.Message,
			Count:   stat.Count,
			Sample:  stat.Sample,
		}
	}
	return stats
}

// CorrelateLogsLive performs live correlation for incident view (no timeout limit)
func (lc *LogCorrelator) CorrelateLogsLive(ctx context.Context, incident Incident, endTime time.Time) (*LogSummary, error) {
	if lc.aggregator == nil {
		return nil, nil
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second) // Increased from 10s
	defer cancel()

	output.Debugf("Starting live log correlation for incident %s, service %s", incident.ID, incident.Service)
	summary, err := lc.aggregator.CorrelateLogsResilient(timeoutCtx, incident.Service, incident.CreatedAt, endTime, false)
	if err != nil {
		output.Warnf("Live log correlation failed for incident %s: %v", incident.ID, err)
		return nil, err
	}

	output.Debugf("Live log correlation completed for incident %s, found %d error types", incident.ID, len(summary.TopErrors))
	
	incidentSummary := &LogSummary{
		Backend:   summary.Backend,
		Window:    summary.Window,
		TopErrors: convertErrorStats(summary.TopErrors),
	}
	
	return incidentSummary, nil
}

// AttachLogSummary attaches log summary to incident (best effort)
func (lc *LogCorrelator) AttachLogSummary(incident *Incident) {
	lc.AttachLogSummaryWithEndTime(incident, time.Time{})
}

// AttachLogSummaryWithEndTime attaches log summary to incident with specific end time
func (lc *LogCorrelator) AttachLogSummaryWithEndTime(incident *Incident, endTime time.Time) {
	if lc == nil || lc.aggregator == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second) // Increased from 5s
		defer cancel()

		summary, err := lc.CorrelateLogs(ctx, *incident, endTime)
		if err != nil {
			output.Warnf("Failed to correlate logs for incident %s: %v", incident.ID, err)
			return
		}

		if summary != nil && len(summary.TopErrors) > 0 {
			incident.Logs = summary
			output.Debugf("Attached log summary to incident %s with %d errors", incident.ID, len(summary.TopErrors))
		}
	}()
}

// AttachLogSummarySync attaches log summary to incident synchronously
func (lc *LogCorrelator) AttachLogSummarySync(ctx context.Context, incident *Incident, endTime time.Time) error {
	if lc == nil || lc.aggregator == nil {
		return nil
	}

	summary, err := lc.CorrelateLogs(ctx, *incident, endTime)
	if err != nil {
		return err
	}

	if summary != nil && len(summary.TopErrors) > 0 {
		incident.Logs = summary
		output.Debugf("Attached log summary to incident %s with %d errors", incident.ID, len(summary.TopErrors))
	}

	return nil
}
