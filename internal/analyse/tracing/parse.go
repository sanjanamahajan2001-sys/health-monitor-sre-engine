package tracing

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type tempoTraceResponse struct {
	Batches []struct {
		Spans []struct {
			TraceID    string `json:"traceId"`
			SpanID     string `json:"spanId"`
			ParentID   string `json:"parentSpanId"`
			Name       string `json:"name"`
			StartTime  string `json:"startTimeUnixNano"`
			EndTime    string `json:"endTimeUnixNano"`
			DurationNs string `json:"durationNano"`
			Attributes []struct {
				Key   string `json:"key"`
				Value struct {
					StringValue string `json:"stringValue"`
				} `json:"value"`
			} `json:"attributes"`
		} `json:"spans"`
		ScopeSpans []struct {
			Spans []struct {
				TraceID    string `json:"traceId"`
				SpanID     string `json:"spanId"`
				ParentID   string `json:"parentSpanId"`
				Name       string `json:"name"`
				StartTime  string `json:"startTimeUnixNano"`
				EndTime    string `json:"endTimeUnixNano"`
				DurationNs string `json:"durationNano"`
				Attributes []struct {
					Key   string `json:"key"`
					Value struct {
						StringValue string `json:"stringValue"`
					} `json:"value"`
				} `json:"attributes"`
			} `json:"spans"`
		} `json:"scopeSpans"`
		Resource struct {
			Attributes []struct {
				Key   string `json:"key"`
				Value struct {
					StringValue string `json:"stringValue"`
				} `json:"value"`
			} `json:"attributes"`
		} `json:"resource"`
	} `json:"batches"`
}

type jaegerTraceWrapper struct {
	Data []struct {
		TraceID   string                   `json:"traceID"`
		Spans     []jaegerSpan             `json:"spans"`
		Processes map[string]jaegerProcess `json:"processes"`
	} `json:"data"`
}

func parseAnyTraceResponse(traceID string, body []byte) (Trace, error) {
	var jaeger jaegerTraceWrapper
	if err := json.Unmarshal(body, &jaeger); err == nil && len(jaeger.Data) > 0 {
		return parseJaegerTrace(jaeger.Data[0]), nil
	}

	var tempo tempoTraceResponse
	if err := json.Unmarshal(body, &tempo); err == nil && len(tempo.Batches) > 0 {
		return parseTempoTrace(traceID, tempo), nil
	}
	var tempoWrapped struct {
		Data struct {
			Batches []struct {
				Spans []struct {
					TraceID    string `json:"traceId"`
					SpanID     string `json:"spanId"`
					ParentID   string `json:"parentSpanId"`
					Name       string `json:"name"`
					StartTime  string `json:"startTimeUnixNano"`
					EndTime    string `json:"endTimeUnixNano"`
					DurationNs string `json:"durationNano"`
					Attributes []struct {
						Key   string `json:"key"`
						Value struct {
							StringValue string `json:"stringValue"`
						} `json:"value"`
					} `json:"attributes"`
				} `json:"spans"`
				ScopeSpans []struct {
					Spans []struct {
						TraceID    string `json:"traceId"`
						SpanID     string `json:"spanId"`
						ParentID   string `json:"parentSpanId"`
						Name       string `json:"name"`
						StartTime  string `json:"startTimeUnixNano"`
						EndTime    string `json:"endTimeUnixNano"`
						DurationNs string `json:"durationNano"`
						Attributes []struct {
							Key   string `json:"key"`
							Value struct {
								StringValue string `json:"stringValue"`
							} `json:"value"`
						} `json:"attributes"`
					} `json:"spans"`
				} `json:"scopeSpans"`
				Resource struct {
					Attributes []struct {
						Key   string `json:"key"`
						Value struct {
							StringValue string `json:"stringValue"`
						} `json:"value"`
					} `json:"attributes"`
				} `json:"resource"`
			} `json:"batches"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &tempoWrapped); err == nil && len(tempoWrapped.Data.Batches) > 0 {
		tempo.Batches = tempoWrapped.Data.Batches
		return parseTempoTrace(traceID, tempo), nil
	}
	return Trace{}, errors.New("unsupported trace response format")
}

func parseTempoTrace(traceID string, resp tempoTraceResponse) Trace {
	trace := Trace{TraceID: traceID}
	var spans []Span
	hasDBSpan := false
	for _, batch := range resp.Batches {
		service := extractOtelService(batch.Resource.Attributes)
		for _, s := range batch.Spans {
			duration := parseDurationNanoString(s.DurationNs)
			if duration == 0 {
				duration = deriveDuration(s.StartTime, s.EndTime)
			}
			name := strings.TrimSpace(s.Name)
			if name == "" {
				name = extractSpanName(s.Attributes)
			}
			isDB := hasDBAttributes(s.Attributes)
			span := Span{
				SpanID:      s.SpanID,
				ParentID:    s.ParentID,
				Service:     service,
				PeerService: extractPeerService(s.Attributes),
				Name:        name,
				Start:       parseUnixNanoString(s.StartTime),
				Duration:    duration,
				IsDB:        isDB,
			}
			if span.Service == "" {
				span.Service = extractOtelService(s.Attributes)
			}
			spans = append(spans, span)
			if isDB {
				hasDBSpan = true
			}
		}
		for _, scope := range batch.ScopeSpans {
			for _, s := range scope.Spans {
				duration := parseDurationNanoString(s.DurationNs)
				if duration == 0 {
					duration = deriveDuration(s.StartTime, s.EndTime)
				}
				name := strings.TrimSpace(s.Name)
				if name == "" {
					name = extractSpanName(s.Attributes)
				}
				isDB := hasDBAttributes(s.Attributes)
				span := Span{
					SpanID:      s.SpanID,
					ParentID:    s.ParentID,
					Service:     service,
					PeerService: extractPeerService(s.Attributes),
					Name:        name,
					Start:       parseUnixNanoString(s.StartTime),
					Duration:    duration,
					IsDB:        isDB,
				}
				if span.Service == "" {
					span.Service = extractOtelService(s.Attributes)
				}
				spans = append(spans, span)
				if isDB {
					hasDBSpan = true
				}
			}
		}
	}
	trace.SpanCount = len(spans)
	trace.RootSpan = findRootSpan(spans)
	limited, truncated := limitSpans(spans, trace.RootSpan, MaxSpansPerTrace)
	trace.Truncated = truncated
	trace.SlowestSpan = findSlowestDownstreamSpan(limited, trace.RootSpan)
	trace.Duration = findTraceDuration(spans)
	trace.HasDBSpan = hasDBSpan
	return trace
}

func extractOtelService(attrs []struct {
	Key   string `json:"key"`
	Value struct {
		StringValue string `json:"stringValue"`
	} `json:"value"`
}) string {
	for _, attr := range attrs {
		if strings.EqualFold(attr.Key, "service.name") {
			return attr.Value.StringValue
		}
	}
	return ""
}

func extractPeerService(attrs []struct {
	Key   string `json:"key"`
	Value struct {
		StringValue string `json:"stringValue"`
	} `json:"value"`
}) string {
	preferred := []string{
		"peer.service", "peer_service",
		"db.name", "db.system",
		"rpc.service", "rpc.peer",
		"messaging.system", "messaging.destination",
		"net.peer.name", "server.address", "server",
	}
	values := map[string]string{}
	for _, attr := range attrs {
		if strings.TrimSpace(attr.Value.StringValue) == "" {
			continue
		}
		values[strings.ToLower(attr.Key)] = attr.Value.StringValue
	}
	for _, key := range preferred {
		if value, ok := values[key]; ok && strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func extractSpanName(attrs []struct {
	Key   string `json:"key"`
	Value struct {
		StringValue string `json:"stringValue"`
	} `json:"value"`
}) string {
	preferred := []string{
		"db.statement",
		"db.operation",
		"rpc.method",
		"messaging.operation",
		"messaging.destination",
	}
	values := map[string]string{}
	for _, attr := range attrs {
		if strings.TrimSpace(attr.Value.StringValue) == "" {
			continue
		}
		values[strings.ToLower(attr.Key)] = attr.Value.StringValue
	}
	for _, key := range preferred {
		if value, ok := values[key]; ok && strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func hasDBAttributes(attrs []struct {
	Key   string `json:"key"`
	Value struct {
		StringValue string `json:"stringValue"`
	} `json:"value"`
}) bool {
	for _, attr := range attrs {
		if strings.HasPrefix(strings.ToLower(attr.Key), "db.") && strings.TrimSpace(attr.Value.StringValue) != "" {
			return true
		}
	}
	return false
}

func findRootSpan(spans []Span) Span {
	var earliest Span
	var longest Span
	for _, s := range spans {
		parent := strings.TrimSpace(s.ParentID)
		if parent == "" || parent == "0000000000000000" {
			return s
		}
		if !s.Start.IsZero() {
			if earliest.SpanID == "" || s.Start.Before(earliest.Start) {
				earliest = s
			}
		}
		if s.Duration > longest.Duration {
			longest = s
		}
	}
	if earliest.SpanID != "" {
		return earliest
	}
	if longest.SpanID != "" {
		return longest
	}
	if len(spans) > 0 {
		return spans[0]
	}
	return Span{}
}

func limitSpans(spans []Span, root Span, max int) ([]Span, bool) {
	if max <= 0 || len(spans) <= max {
		return spans, false
	}
	sorted := make([]Span, len(spans))
	copy(sorted, spans)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Duration > sorted[j].Duration
	})
	limited := sorted[:max]
	if root.SpanID != "" {
		found := false
		for _, s := range limited {
			if s.SpanID == root.SpanID {
				found = true
				break
			}
		}
		if !found {
			limited[max-1] = root
		}
	}
	return limited, true
}

func findSlowestSpan(spans []Span) Span {
	var slowest Span
	for _, s := range spans {
		if slowest.SpanID == "" || s.Duration > slowest.Duration {
			slowest = s
		}
	}
	return slowest
}

func findSlowestDownstreamSpan(spans []Span, root Span) Span {
	if len(spans) == 0 {
		return Span{}
	}
	var candidates []Span
	for _, s := range spans {
		if s.SpanID == root.SpanID {
			continue
		}
		if s.Duration <= 0 {
			continue
		}
		if root.Service != "" {
			if s.Service == root.Service && s.PeerService == "" {
				continue
			}
			if s.PeerService != "" && strings.EqualFold(s.PeerService, root.Service) {
				continue
			}
		}
		if strings.TrimSpace(s.Service) == "" && strings.TrimSpace(s.PeerService) == "" {
			continue
		}
		candidates = append(candidates, s)
	}
	if len(candidates) > 0 {
		return findSlowestSpan(candidates)
	}
	return Span{}
}

func findTraceDuration(spans []Span) time.Duration {
	if len(spans) == 0 {
		return 0
	}
	minStart := spans[0].Start
	maxEnd := spans[0].Start.Add(spans[0].Duration)
	for _, s := range spans {
		if s.Start.IsZero() {
			continue
		}
		if s.Start.Before(minStart) {
			minStart = s.Start
		}
		end := s.Start.Add(s.Duration)
		if end.After(maxEnd) {
			maxEnd = end
		}
	}
	if maxEnd.Before(minStart) {
		return 0
	}
	return maxEnd.Sub(minStart)
}

func parseUnixNanoString(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	nanos, err := parseInt64(value)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(0, nanos)
}

func parseDurationNanoString(value string) time.Duration {
	if value == "" {
		return 0
	}
	nanos, err := parseInt64(value)
	if err != nil {
		return 0
	}
	return time.Duration(nanos)
}

func deriveDuration(startValue, endValue string) time.Duration {
	if startValue == "" || endValue == "" {
		return 0
	}
	start := parseUnixNanoString(startValue)
	end := parseUnixNanoString(endValue)
	if start.IsZero() || end.IsZero() || end.Before(start) {
		return 0
	}
	return end.Sub(start)
}

func parseInt64(value string) (int64, error) {
	var out int64
	_, err := fmt.Sscanf(value, "%d", &out)
	return out, err
}
