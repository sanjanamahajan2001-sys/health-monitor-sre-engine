package traces

import (
	"encoding/json"
	"strconv"
	"time"
)

// TraceSummary represents correlated trace data for an incident
type TraceSummary struct {
	Backend       string     `json:"backend"`
	Window        string     `json:"window"`
	SlowestRoute  string     `json:"slowest_route"`
	P95LatencyMs  float64    `json:"p95_latency_ms"`
	ErrorCount    int        `json:"error_count"`
	ErrorRate     float64    `json:"error_rate"`
	TopSpans      []SpanInfo `json:"top_spans"`

	TopRoutes     []RouteInfo `json:"top_routes"`
	GrafanaURL    string     `json:"grafana_url"`
	TraceCount    int        `json:"trace_count"`
}

// SpanInfo represents a span with performance metrics
type SpanInfo struct {
	Name         string  `json:"name"`
	DurationMs   float64 `json:"duration_ms"`
	ErrorCount   int     `json:"error_count"`
	Service      string  `json:"service"`
}

// RouteInfo represents a route/endpoint with performance metrics
type RouteInfo struct {
	Route        string  `json:"route"`
	P95LatencyMs float64 `json:"p95_latency_ms"`
	ErrorRate    float64 `json:"error_rate"`
	RequestCount int     `json:"request_count"`
}

// TempoSearchResponse represents Tempo search API response
type TempoSearchResponse struct {
	Traces []TempoTraceMatch `json:"traces"`
	Total  int               `json:"total"`
}

// TempoTraceMatch represents a trace match from search
type TempoTraceMatch struct {
	TraceID   string            `json:"traceID"`
	RootTrace string            `json:"rootTrace"`
	Spans     []TempoSpanInfo   `json:"spans"`
	Services  map[string]int    `json:"services"`
}

// TempoSpanInfo represents span info in search results
type TempoSpanInfo struct {
	TraceID    string    `json:"traceID"`
	SpanID     string    `json:"spanID"`
	StartTime  time.Time `json:"startTime"`
	DurationMs int64     `json:"durationMs"`
	Name       string    `json:"name"`
	Service    string    `json:"service"`
}

// TempoTraceResponse represents Tempo trace API response
type TempoTraceResponse struct {
	Batches []TempoBatch `json:"batches"`
}

// TempoResource represents OTLP resource (service metadata)
type TempoResource struct {
    Attributes []OTLPAttribute `json:"attributes"`
}

// TempoBatch represents a batch of spans in Tempo
type TempoBatch struct {
	Resource   TempoResource `json:"resource"`
	ScopeSpans []ScopeSpan   `json:"scopeSpans"`
}

// ScopeSpan represents a scope and its spans
type ScopeSpan struct {
	Scope Library     `json:"scope"`
	Spans []TempoSpan `json:"spans"`
}

// Library represents instrumentation library
type Library struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// TempoSpan represents a single span
type TempoSpan struct {
	TraceID       string                   `json:"traceId"`
	SpanID        string                   `json:"spanId"`
	ParentSpanID  string                   `json:"parentSpanId"`
	TraceState    string                   `json:"traceState"`
	Name          string                   `json:"name"`
	Kind          string                   `json:"kind"`
	StartTimeUnixNano string                `json:"startTimeUnixNano"`
	EndTimeUnixNano   string                `json:"endTimeUnixNano"`
	Status        TempoStatus              `json:"status"`
	Attributes    []OTLPAttribute          `json:"attributes"`
	Events        []TempoEvent             `json:"events"`
	Links         []TempoLink              `json:"links"`
}

// TempoStatus represents span status with custom unmarshaling to handle string/int codes
type TempoStatus struct {
	Code        int             `json:"code"`
	Message     string          `json:"message"`
	Attributes  []OTLPAttribute `json:"attributes"`
}

// UnmarshalJSON handles both string ("STATUS_CODE_ERROR") and int (2) for Code
func (s *TempoStatus) UnmarshalJSON(data []byte) error {
	type Alias TempoStatus
	aux := &struct {
		Code interface{} `json:"code"`
		*Alias
	}{
		Alias: (*Alias)(s),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	
	switch v := aux.Code.(type) {
	case float64:
		s.Code = int(v)
	case string:
		// Handle standard OTEL status code strings
		switch v {
		case "STATUS_CODE_UNSET":
			s.Code = 0
			return nil
		case "STATUS_CODE_OK":
			s.Code = 1
			return nil
		case "STATUS_CODE_ERROR":
			s.Code = 2
			return nil
		}
		// Fallback to numeric string (e.g., "2")
		if code, err := strconv.Atoi(v); err == nil {
			s.Code = code
		}
	}
	return nil
}

// TempoEvent represents a span event
type TempoEvent struct {
	TimeUnixNano string                 `json:"timeUnixNano"`
	Name         string                 `json:"name"`
	Attributes   []OTLPAttribute        `json:"attributes"`
}

// TempoLink represents a span link
type TempoLink struct {
	TraceID    string                 `json:"traceID"`
	SpanID     string                 `json:"spanID"`
	TraceState string                 `json:"traceState"`
	Attributes []OTLPAttribute        `json:"attributes"`
}

// OTLPAttribute represents an OTLP attribute in the correct format
type OTLPAttribute struct {
	Key   string      `json:"key"`
	Value OTLPValue   `json:"value"`
}

// OTLPValue represents the value part of an OTLP attribute
type OTLPValue struct {
	StringValue    string  `json:"stringValue,omitempty"`
	IntValue       string  `json:"intValue,omitempty"`
	DoubleValue    float64 `json:"doubleValue,omitempty"`
	BoolValue      bool    `json:"boolValue,omitempty"`
}
