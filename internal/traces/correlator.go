package traces

import (
	"context"
	"fmt"
	"health-monitor/internal/output"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"health-monitor/internal/config"
)

// TraceBackend defines the interface for trace backends
type TraceBackend interface {
	SearchTraces(ctx context.Context, req SearchRequest) (*TempoSearchResponse, error)
	GetTrace(ctx context.Context, traceID string) (*TempoTraceResponse, error)
}

// Correlator handles trace correlation for incidents
type Correlator struct {
	backend TraceBackend
	config  config.Config
}

// NewCorrelator creates a new trace correlator based on configuration
func NewCorrelator(cfg config.Config) (*Correlator, error) {
	var backend TraceBackend
	var err error

	switch strings.ToLower(cfg.TraceBackend) {
	case "tempo":
		if cfg.TraceURL == "" {
			return nil, fmt.Errorf("Tempo backend specified but TRACE_URL is not configured")
		}
		backend, err = NewTempoClient(
			cfg.TraceURL,
			cfg.TraceUser,
			cfg.TracePass,
			cfg.TraceToken,
		)
	case "jaeger":
		if cfg.TraceURL == "" {
			return nil, fmt.Errorf("Jaeger backend specified but TRACE_URL is not configured")
		}
		backend, err = NewJaegerClient(
			cfg.TraceURL,
			cfg.TraceUser,
			cfg.TracePass,
			cfg.TraceToken,
		)
	case "":
		// Auto-detect: try Tempo if URL is configured
		if cfg.TraceURL != "" {
			backend, err = NewTempoClient(
				cfg.TraceURL,
				cfg.TraceUser,
				cfg.TracePass,
				cfg.TraceToken,
			)
		} else {
			return nil, fmt.Errorf("no trace backend configured")
		}
	default:
		return nil, fmt.Errorf("unsupported trace backend: %s", cfg.TraceBackend)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create trace backend: %w", err)
	}

	return &Correlator{
		backend: backend,
		config:  cfg,
	}, nil
}

func (c *Correlator) Correlate(ctx context.Context, service string, incidentStart time.Time, endTime time.Time) (*TraceSummary, error) {
	// Use provided window bounds
	start := incidentStart
	end := endTime
	
	if start.IsZero() {
		// Fallback to default lookback if no start time provided
		window := 15 * time.Minute
		if c.config.TraceWindow != "" {
			if parsed, err := time.ParseDuration(c.config.TraceWindow); err == nil {
				window = parsed
			}
		}
		if !end.IsZero() {
			start = end.Add(-window)
		} else {
			end = time.Now().UTC()
			start = end.Add(-window)
		}
	} else if end.IsZero() {
		end = time.Now().UTC()
	}

	// For very fresh incidents, ensure we don't search in the future or have a negative window
	now := time.Now().UTC()
	if end.After(now) {
		end = now
	}
	if end.Before(start) {
		start = end.Add(-1 * time.Second)
	}

	output.Debugf("Trace correlation window: %s to %s", start.Format(time.RFC3339), end.Format(time.RFC3339))

	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	// Search for traces
	minDuration := int64(c.config.TraceMinDurationMs)
	if minDuration <= 0 {
		minDuration = 10 // Default 10ms
	}

	// Apply service mapping
	originalService := service
	mapper := parseServiceMap(c.config.TraceServiceMap)
	service = mapService(mapper, originalService)
	if service != originalService {
		output.Debugf("Mapped service %s to %s for trace correlation", originalService, service)
	}

	// Candidate tags to search by. "" means use top-level 'service' query parameter.
	tags := []string{"", "service.name", "service", "app", "container"}
	
	// If a specific tag is configured, prioritize it
	if c.config.TraceServiceTag != "" {
		// Put at front
		tags = append([]string{c.config.TraceServiceTag}, tags...)
	}

	var searchResp *TempoSearchResponse
	var lastErr error
	var foundTag string

	// Handle different backends
	switch c.config.TraceBackend {
	case "jaeger":
		// Jaeger usually use 'service' param directly, similar to our "" tag logic
		searchReq := SearchRequest{
			Service:       service,
			TagName:       "", // Jaeger client uses this for service param
			MinDurationMs: minDuration,
			Start:         start,
			End:           end,
		}
		resp, err := c.backend.SearchTraces(ctx, searchReq)
		if err != nil {
			output.Debugf("Jaeger search failed for service %s: %v", service, err)
			return &TraceSummary{Backend: "jaeger", TraceCount: 0}, err
		}
		searchResp = resp
		foundTag = "service"

	default: // "tempo" or empty
		for _, tag := range tags {
			searchReq := SearchRequest{
				Service:       service,
				TagName:       tag,
				MinDurationMs: minDuration,
				Start:         start,
				End:           end,
			}

			resp, err := c.backend.SearchTraces(ctx, searchReq)
			if err != nil {
				lastErr = err
				continue
			}

			if resp != nil && len(resp.Traces) > 0 {
				searchResp = resp
				foundTag = tag
				if foundTag == "" {
					foundTag = "(top-level service param)"
				}
				break
			}
		}
	}

	if searchResp == nil || len(searchResp.Traces) == 0 {
		output.Debugf("No traces found for service %s after trying all tags", service)
		return &TraceSummary{
			Backend:    c.config.TraceBackend,
			Window:     c.config.TraceWindow,
			TraceCount: 0,
		}, lastErr
	}

	output.Debugf("Found %d traces via tag %s, analyzing top %d", len(searchResp.Traces), foundTag, min(50, len(searchResp.Traces)))

	// Analyze traces to extract metrics
	summary := c.analyzeTraces(ctx, searchResp.Traces[:min(50, len(searchResp.Traces))], service)
	
	// Generate Grafana URL
	summary.GrafanaURL = c.generateGrafanaURL(service, start, end)
	summary.Backend = c.config.TraceBackend
	summary.Window = c.config.TraceWindow
	summary.TraceCount = len(searchResp.Traces)

	return summary, nil
}

// analyzeTraces processes trace data to extract performance metrics using concurrent fetching
func (c *Correlator) analyzeTraces(ctx context.Context, traces []TempoTraceMatch, service string) *TraceSummary {
	var allSpans []TempoSpan
	var routeLatencies []float64
	var errors []TempoSpan
	routeStats := make(map[string]*RouteMetrics)
	spanStats := make(map[string]*SpanMetrics)

	// Use worker pool for concurrent trace fetching
	maxWorkers := 10
	if len(traces) < maxWorkers {
		maxWorkers = len(traces)
	}

	traceChan := make(chan TempoTraceMatch, len(traces))
	resultChan := make(chan traceResult, len(traces))

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go c.traceWorker(ctx, &wg, traceChan, resultChan)
	}

	// Send work to workers
	for _, trace := range traces {
		traceChan <- trace
	}
	close(traceChan)

	// Wait for workers to complete
	wg.Wait()
	close(resultChan)

	// Process results
	for result := range resultChan {
		if result.err != nil || result.traceResp == nil {
			continue
		}
		
		for _, batch := range result.traceResp.Batches {
			for _, scopeSpan := range batch.ScopeSpans {
				for _, span := range scopeSpan.Spans {
					allSpans = append(allSpans, span)
					
					// Extract route information
					route := ExtractRouteFromSpan(span.Attributes)
					if route == "" {
						route = span.Name // Fallback to span name
					}

					// Skip system routes if configured
					if c.config.TraceExcludeSystemRoutes && IsSystemRoute(route) {
						continue
					}

					// Calculate duration in milliseconds
					startNano, err := parseNanoTime(span.StartTimeUnixNano)
					if err != nil {
						output.Warnf("Failed to parse start time for span %s: %v", span.Name, err)
						continue
					}
					endNano, err := parseNanoTime(span.EndTimeUnixNano)
					if err != nil {
						output.Warnf("Failed to parse end time for span %s: %v", span.Name, err)
						continue
					}
					durationMs := float64(endNano-startNano) / 1e6

					// Track route metrics
					if _, exists := routeStats[route]; !exists {
						routeStats[route] = &RouteMetrics{
							Route:        route,
							Latencies:    []float64{},
							ErrorCount:   0,
							TotalCount:   0,
						}
					}
					routeStats[route].Latencies = append(routeStats[route].Latencies, durationMs)
					routeStats[route].TotalCount++
					routeLatencies = append(routeLatencies, durationMs)

					// Track span metrics
					spanName := span.Name
					if _, exists := spanStats[spanName]; !exists {
						spanStats[spanName] = &SpanMetrics{
							Name:       spanName,
							Durations:  []float64{},
							ErrorCount: 0,
							TotalCount: 0,
						}
					}
					spanStats[spanName].Durations = append(spanStats[spanName].Durations, durationMs)
					spanStats[spanName].TotalCount++

					// Check for errors
					if IsErrorSpan(span) {
						errors = append(errors, span)
						routeStats[route].ErrorCount++
						spanStats[spanName].ErrorCount++
					}
				}
			}
		}
	}

	output.Debugf("Trace analysis complete - routes: %d, spans: %d, errors: %d", len(routeStats), len(spanStats), len(errors))

	// Build summary
	summary := &TraceSummary{
		ErrorCount: len(errors),
	}
	if len(allSpans) > 0 {
		summary.ErrorRate = float64(len(errors)) / float64(len(allSpans)) * 100
	}

	// Calculate P95 latency
	if len(routeLatencies) > 0 {
		sort.Float64s(routeLatencies)
		p95Index := int(float64(len(routeLatencies)) * 0.95)
		if p95Index >= len(routeLatencies) {
			p95Index = len(routeLatencies) - 1
		}
		summary.P95LatencyMs = routeLatencies[p95Index]
	}

	// Find slowest route
	slowestRoute := ""
	slowestP95 := 0.0
	for route, metrics := range routeStats {
		if len(metrics.Latencies) > 0 {
			sort.Float64s(metrics.Latencies)
			p95Index := int(float64(len(metrics.Latencies)) * 0.95)
			if p95Index >= len(metrics.Latencies) {
				p95Index = len(metrics.Latencies) - 1
			}
			p95 := metrics.Latencies[p95Index]
			if p95 > slowestP95 {
				slowestP95 = p95
				slowestRoute = route
			}
		}
	}
	summary.SlowestRoute = slowestRoute

	// Build top spans (by duration)
	var topSpans []SpanInfo
	for name, metrics := range spanStats {
		if len(metrics.Durations) > 0 {
			sort.Float64s(metrics.Durations)
			avgDuration := 0.0
			for _, d := range metrics.Durations {
				avgDuration += d
			}
			avgDuration /= float64(len(metrics.Durations))

			topSpans = append(topSpans, SpanInfo{
				Name:       name,
				DurationMs: avgDuration,
				ErrorCount: metrics.ErrorCount,
				Service:    service,
			})
		}
	}

	// Sort spans by duration (descending) and take top 5
	sort.Slice(topSpans, func(i, j int) bool {
		return topSpans[i].DurationMs > topSpans[j].DurationMs
	})
	if len(topSpans) > 5 {
		topSpans = topSpans[:5]
	}
	summary.TopSpans = topSpans

	// Build top routes (by P95 latency)
	var topRoutes []RouteInfo
	for route, metrics := range routeStats {
		if len(metrics.Latencies) > 0 {
			sort.Float64s(metrics.Latencies)
			p95Index := int(float64(len(metrics.Latencies)) * 0.95)
			if p95Index >= len(metrics.Latencies) {
				p95Index = len(metrics.Latencies) - 1
			}
			p95 := metrics.Latencies[p95Index]

			errorRate := 0.0
			if metrics.TotalCount > 0 {
				errorRate = float64(metrics.ErrorCount) / float64(metrics.TotalCount) * 100
			}

			topRoutes = append(topRoutes, RouteInfo{
				Route:        route,
				P95LatencyMs: p95,
				ErrorRate:    errorRate,
				RequestCount: metrics.TotalCount,
			})
		}
	}

	// Sort routes by P95 latency (descending) and take top 3
	sort.Slice(topRoutes, func(i, j int) bool {
		return topRoutes[i].P95LatencyMs > topRoutes[j].P95LatencyMs
	})
	if len(topRoutes) > 3 {
		topRoutes = topRoutes[:3]
	}
	summary.TopRoutes = topRoutes

	return summary
}

// generateGrafanaURL creates a Grafana Explore deep link for trace data
func (c *Correlator) generateGrafanaURL(service string, start, end time.Time) string {
	if c.config.GrafanaURL == "" || c.config.GrafanaTraceDataSource == "" {
		return ""
	}

	// Build Grafana Explore URL
	baseURL := strings.TrimSuffix(c.config.GrafanaURL, "/")
	
	// URL encode the left panel
	leftPanelJSON := fmt.Sprintf(`{"datasource":"%s","queries":[{"query":"service.name=\"%s\"","refId":"A"}],"range":{"from":"%s","to":"%s"}}`,
		c.config.GrafanaTraceDataSource, service, start.Format(time.RFC3339), end.Format(time.RFC3339))
	
	// URL encode for query parameter
	leftPanelEncoded := urlEncode(leftPanelJSON)
	
	return fmt.Sprintf("%s/explore?left=%s", baseURL, leftPanelEncoded)
}

// Helper types for metrics calculation
type RouteMetrics struct {
	Route        string
	Latencies    []float64
	ErrorCount   int
	TotalCount   int
}

type SpanMetrics struct {
	Name       string
	Durations  []float64
	ErrorCount int
	TotalCount int
}

// Helper functions
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func parseNanoTime(nanoStr string) (int64, error) {
	// Parse nanosecond timestamp
	return strconv.ParseInt(nanoStr, 10, 64)
}

func urlEncode(s string) string {
	return url.QueryEscape(s)
}

// traceResult represents the result of fetching a single trace
type traceResult struct {
	traceID    string
	traceResp  *TempoTraceResponse
	err        error
}

// traceWorker is a worker function that fetches traces concurrently
func (c *Correlator) traceWorker(ctx context.Context, wg *sync.WaitGroup, traceChan <-chan TempoTraceMatch, resultChan chan<- traceResult) {
	defer wg.Done()
	
	for trace := range traceChan {
		traceResp, err := c.backend.GetTrace(ctx, trace.TraceID)
		if err != nil {
			output.Warnf("Failed to fetch trace %s: %v", trace.TraceID, err)
		}
		
		resultChan <- traceResult{
			traceID:   trace.TraceID,
			traceResp: traceResp,
			err:       err,
		}
	}
}
