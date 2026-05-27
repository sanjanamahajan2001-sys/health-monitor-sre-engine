package traces

import (
	"context"
	"fmt"
	"health-monitor/internal/output"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	tempoSearchEndpoint = "/api/search"
	tempoTraceEndpoint  = "/api/traces/%s"
	defaultTimeout      = 15 * time.Second
	maxTracesPerRequest = 100
)

// TempoClient implements trace backend for Grafana Tempo
type TempoClient struct {
	baseURL    string
	authToken  string
	username   string
	password   string
	httpClient *http.Client
}

// NewTempoClient creates a new Tempo client
func NewTempoClient(baseURL, username, password, authToken string) (*TempoClient, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("Tempo base URL is required")
	}

	// Ensure URL has proper scheme
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "http://" + baseURL
	}

	client := &TempoClient{
		baseURL:   strings.TrimSuffix(baseURL, "/"),
		authToken: authToken,
		username:  username,
		password:  password,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
	}

	return client, nil
}

// SearchTraces searches for traces matching criteria
func (c *TempoClient) SearchTraces(ctx context.Context, req SearchRequest) (*TempoSearchResponse, error) {
	params := url.Values{}
	
	if req.Service != "" {
		if req.TagName == "" {
			params.Add("service", req.Service)
		} else {
			params.Add("tags", fmt.Sprintf(`%s=%s`, req.TagName, req.Service))
		}
	}
	
	if req.MinDurationMs > 0 {
		params.Add("minDuration", fmt.Sprintf("%dms", req.MinDurationMs))
	}
	
	if !req.Start.IsZero() {
		params.Add("start", strconv.FormatInt(req.Start.Unix(), 10))
	}
	
	if !req.End.IsZero() {
		params.Add("end", strconv.FormatInt(req.End.Unix(), 10))
	}
	
	params.Add("limit", strconv.Itoa(maxTracesPerRequest))

	searchURL := fmt.Sprintf("%s%s?%s", c.baseURL, tempoSearchEndpoint, params.Encode())
	output.Debugf("Tempo search URL: %s", searchURL)

	httpReq, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, err
	}

	setAuth(httpReq, c.authToken, c.username, c.password)

	var searchResp TempoSearchResponse
	if err := doRequest(ctx, c.httpClient, httpReq, &searchResp); err != nil {
		return nil, err
	}

	return &searchResp, nil
}

// GetTrace retrieves a specific trace by ID
func (c *TempoClient) GetTrace(ctx context.Context, traceID string) (*TempoTraceResponse, error) {
	traceURL := fmt.Sprintf("%s%s", c.baseURL, fmt.Sprintf(tempoTraceEndpoint, traceID))
	output.Debugf("Tempo trace URL: %s", traceURL)

	httpReq, err := http.NewRequest("GET", traceURL, nil)
	if err != nil {
		return nil, err
	}

	setAuth(httpReq, c.authToken, c.username, c.password)

	var traceResp TempoTraceResponse
	if err := doRequest(ctx, c.httpClient, httpReq, &traceResp); err != nil {
		return nil, err
	}

	return &traceResp, nil
}

// SearchRequest represents a trace search request
type SearchRequest struct {
	Service       string
	TagName       string
	MinDurationMs int64
	Start         time.Time
	End           time.Time
}

// IsSystemRoute checks if a route should be excluded as system route
func IsSystemRoute(route string) bool {
	route = strings.ToLower(route)
	systemRoutes := []string{
		"/health", "/healthz", "/ready", "/readyz", "/live", "/livez",
		"/metrics", "/status", "/ping", "/favicon.ico", "/robots.txt",
		"/__/health", "/__/ready", "/__/metrics",
	}
	
	for _, sysRoute := range systemRoutes {
		if strings.HasPrefix(route, sysRoute) {
			return true
		}
	}
	
	if strings.Contains(route, "health") || strings.Contains(route, "metrics") {
		return true
	}
	
	return false
}

// ExtractRouteFromSpan extracts route information from span attributes
func ExtractRouteFromSpan(attributes []OTLPAttribute) string {
	routeKeys := []string{
		"http.route", "http.target", "http.path", "http.url",
		"faas.route", "faas.path",
		"rpc.service", "rpc.method",
		"messaging.destination",
	}
	
	for _, key := range routeKeys {
		if val := getAttributeValue(attributes, key); val != "" {
			return val
		}
	}
	
	return ""
}

// ExtractServiceFromSpan extracts service information from span attributes
func ExtractServiceFromSpan(attributes []OTLPAttribute) string {
	serviceKeys := []string{
		"service.name", "service.namespace",
		"faas.name", "faas.version",
		"rpc.service",
	}
	
	for _, key := range serviceKeys {
		if val := getAttributeValue(attributes, key); val != "" {
			return val
		}
	}
	
	return ""
}

// IsErrorSpan checks if a span represents an error
func IsErrorSpan(span TempoSpan) bool {
	if span.Status.Code >= 2 {
		return true
	}
	
	if val := getAttributeValue(span.Attributes, "error"); val != "" {
		if val == "true" {
			return true
		}
	}
	
	if val := getAttributeValue(span.Attributes, "error.type"); val != "" {
		return true
	}
	
	if strings.Contains(strings.ToLower(span.Name), "error") {
		return true
	}
	
	return false
}

// getAttributeValue extracts a string value from OTLP attributes by key
func getAttributeValue(attributes []OTLPAttribute, key string) string {
	for _, attr := range attributes {
		if attr.Key == key {
			if attr.Value.StringValue != "" {
				return attr.Value.StringValue
			}
			if key == "error" || key == "error.type" {
				if attr.Value.BoolValue {
					return "true"
				} else {
					return "false"
				}
			}
			if attr.Value.IntValue != "" || key == "http.status_code" {
				return attr.Value.IntValue
			}
			if attr.Value.DoubleValue != 0 {
				return strconv.FormatFloat(attr.Value.DoubleValue, 'f', -1, 64)
			}
			if !attr.Value.BoolValue && (key == "error" || key == "error.type") {
				return "false"
			}
		}
	}
	return ""
}
