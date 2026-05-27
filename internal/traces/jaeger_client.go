package traces

import (
	"context"
	"fmt"
	"health-monitor/internal/output"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

const (
	jaegerSearchPath = "/traces"
	jaegerTracePath  = "/traces/%s"
)

// JaegerClient implements trace backend for Jaeger
type JaegerClient struct {
	baseURL    string
	authToken  string
	username   string
	password   string
	httpClient *http.Client

	// Discovery state
	mu            sync.RWMutex
	pathPrefix    string // e.g. "/api", "/jaeger/api", "/jaeger/ui/api"
	discovered    bool
}

// NewJaegerClient creates a new Jaeger client
func NewJaegerClient(baseURL, username, password, authToken string) (*JaegerClient, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("Jaeger base URL is required")
	}

	// Ensure URL has proper scheme
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "http://" + baseURL
	}

	client := &JaegerClient{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		authToken:  authToken,
		username:   username,
		password:   password,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
	}

	return client, nil
}

// discoverPrefix probes for the correct Jaeger API prefix
func (c *JaegerClient) discoverPrefix(ctx context.Context) error {
	c.mu.Lock()
	if c.discovered {
		c.mu.Unlock()
		return nil
	}
	defer c.mu.Unlock()

	// List of potential API prefixes to probe
	prefixes := []string{"/api", "/jaeger/api", "/jaeger/ui/api"}
	
	output.Debugf("Probing Jaeger API paths at %s...", c.baseURL)
	
	for _, prefix := range prefixes {
		probeURL := fmt.Sprintf("%s%s/services", c.baseURL, prefix)
		httpReq, err := http.NewRequest("GET", probeURL, nil)
		if err != nil {
			continue
		}
		setAuth(httpReq, c.authToken, c.username, c.password)

		var dummy struct{}
		if err := doRequest(ctx, c.httpClient, httpReq, &dummy); err == nil {
			output.Infof("Discovered Jaeger API prefix: %s", prefix)
			c.pathPrefix = prefix
			c.discovered = true
			return nil
		} else {
			output.Debugf("Probe failed for %s: %v", prefix, err)
		}
	}

	return fmt.Errorf("failed to discover Jaeger API path (tried %s)", strings.Join(prefixes, ", "))
}

func (c *JaegerClient) getAPIURL(path string) (string, error) {
	c.mu.RLock()
	prefix := c.pathPrefix
	c.mu.RUnlock()

	// If the baseURL already ends with one of our prefixes, don't duplicate it
	cleanBase := c.baseURL
	if prefix != "" {
		if strings.HasSuffix(cleanBase, prefix) {
			cleanBase = strings.TrimSuffix(cleanBase, prefix)
		}
	}

	return fmt.Sprintf("%s%s%s", cleanBase, prefix, path), nil
}

// SearchTraces searches for traces matching criteria using Jaeger API
func (c *JaegerClient) SearchTraces(ctx context.Context, req SearchRequest) (*TempoSearchResponse, error) {
	// Auto-discover if not already done
	if err := c.discoverPrefix(ctx); err != nil {
		output.Warnf("Jaeger API discovery failed: %v", err)
	}

	params := url.Values{}
	if req.Service != "" {
		params.Add("service", req.Service)
	}
	if req.MinDurationMs > 0 {
		params.Add("minDuration", fmt.Sprintf("%dms", req.MinDurationMs))
	}
	if !req.Start.IsZero() {
		params.Add("start", fmt.Sprintf("%d", req.Start.UnixMicro()))
	}
	if !req.End.IsZero() {
		params.Add("end", fmt.Sprintf("%d", req.End.UnixMicro()))
	}
	params.Add("limit", fmt.Sprintf("%d", maxTracesPerRequest))

	apiURL, _ := c.getAPIURL(jaegerSearchPath)
	searchURL := fmt.Sprintf("%s?%s", apiURL, params.Encode())
	output.Debugf("Jaeger search URL: %s", searchURL)

	httpReq, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Jaeger search request: %w", err)
	}

	setAuth(httpReq, c.authToken, c.username, c.password)

	var jaegerResp jaegerResponse
	if err := doRequest(ctx, c.httpClient, httpReq, &jaegerResp); err != nil {
		return nil, fmt.Errorf("Jaeger search failed: %w (URL: %s)", err, searchURL)
	}

	// Convert Jaeger response to TempoSearchResponse format
	searchResp := &TempoSearchResponse{
		Traces: make([]TempoTraceMatch, 0, len(jaegerResp.Data)),
		Total:  len(jaegerResp.Data),
	}

	for _, t := range jaegerResp.Data {
		match := TempoTraceMatch{
			TraceID:  t.TraceID,
			Services: make(map[string]int),
		}
		
		for _, s := range t.Spans {
			match.Spans = append(match.Spans, TempoSpanInfo{
				TraceID:    s.TraceID,
				SpanID:     s.SpanID,
				Name:       s.OperationName,
				DurationMs: s.Duration / 1000,
			})
			
			if p, ok := t.Processes[s.ProcessID]; ok {
				match.Services[p.ServiceName]++
			}
		}
		searchResp.Traces = append(searchResp.Traces, match)
	}

	return searchResp, nil
}

// GetTrace retrieves a specific trace by ID from Jaeger
func (c *JaegerClient) GetTrace(ctx context.Context, traceID string) (*TempoTraceResponse, error) {
	if err := c.discoverPrefix(ctx); err != nil {
		output.Warnf("Jaeger API discovery failed: %v", err)
	}

	apiURL, _ := c.getAPIURL(fmt.Sprintf(jaegerTracePath, traceID))
	output.Debugf("Jaeger trace URL: %s", apiURL)

	httpReq, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Jaeger trace request: %w", err)
	}

	setAuth(httpReq, c.authToken, c.username, c.password)

	var jaegerResp jaegerResponse
	if err := doRequest(ctx, c.httpClient, httpReq, &jaegerResp); err != nil {
		return nil, fmt.Errorf("Jaeger trace fetch failed: %w (URL: %s)", err, apiURL)
	}

	if len(jaegerResp.Data) == 0 {
		return nil, fmt.Errorf("trace not found: %s", traceID)
	}

	traceData := jaegerResp.Data[0]
	tempoResp := &TempoTraceResponse{
		Batches: make([]TempoBatch, 0, len(traceData.Processes)),
	}

	processSpans := make(map[string][]TempoSpan)
	for _, s := range traceData.Spans {
		parentID := ""
		for _, ref := range s.References {
			if strings.EqualFold(ref.RefType, "CHILD_OF") {
				parentID = ref.SpanID
				break
			}
		}

		attrs := make([]OTLPAttribute, 0, len(s.Tags))
		isError := false
		for _, tag := range s.Tags {
			val := fmt.Sprint(tag.Value)
			attrs = append(attrs, OTLPAttribute{
				Key: tag.Key,
				Value: OTLPValue{StringValue: val},
			})
			if tag.Key == "error" && (val == "true" || val == "1") {
				isError = true
			}
		}

		status := TempoStatus{Code: 1}
		if isError {
			status.Code = 2
		}

		tempoSpan := TempoSpan{
			TraceID:           s.TraceID,
			SpanID:            s.SpanID,
			ParentSpanID:      parentID,
			Name:              s.OperationName,
			StartTimeUnixNano: fmt.Sprintf("%d", s.StartTime*1000),
			EndTimeUnixNano:   fmt.Sprintf("%d", (s.StartTime+s.Duration)*1000),
			Status:            status,
			Attributes:        attrs,
		}
		processSpans[s.ProcessID] = append(processSpans[s.ProcessID], tempoSpan)
	}

	for pid, spans := range processSpans {
		process := traceData.Processes[pid]
		batch := TempoBatch{
			Resource: TempoResource{
				Attributes: []OTLPAttribute{
					{Key: "service.name", Value: OTLPValue{StringValue: process.ServiceName}},
				},
			},
			ScopeSpans: []ScopeSpan{{Spans: spans}},
		}
		tempoResp.Batches = append(tempoResp.Batches, batch)
	}

	return tempoResp, nil
}

type jaegerResponse struct {
	Data []struct {
		TraceID   string                   `json:"traceID"`
		Spans     []jaegerSpan             `json:"spans"`
		Processes map[string]jaegerProcess `json:"processes"`
	} `json:"data"`
}

type jaegerSpan struct {
	TraceID       string      `json:"traceID"`
	SpanID        string      `json:"spanID"`
	OperationName string      `json:"operationName"`
	StartTime     int64       `json:"startTime"`
	Duration      int64       `json:"duration"`
	ProcessID     string      `json:"processID"`
	References    []jaegerRef `json:"references"`
	Tags          []jaegerTag `json:"tags"`
}

type jaegerRef struct {
	RefType string `json:"refType"`
	SpanID  string `json:"spanID"`
}

type jaegerProcess struct {
	ServiceName string `json:"serviceName"`
}

type jaegerTag struct {
	Key   string      `json:"key"`
	Value interface{} `json:"value"`
}
