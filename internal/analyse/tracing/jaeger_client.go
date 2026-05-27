package tracing

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type jaegerTracesResponse struct {
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
	Type  string      `json:"type"`
	Value interface{} `json:"value"`
}

func (c Client) JaegerHealth() (string, error) {
	if strings.TrimSpace(c.BaseURL) == "" {
		return "", errors.New("missing trace url")
	}
	base := strings.TrimSuffix(c.BaseURL, "/")
	endpoint := base + "/api/services"
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	applyAuth(req, c)
	status, body, err := doRequest(req, c.Timeout, c.BaseURL, c.QPS)
	if err != nil {
		return "", err
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return "", HTTPError{StatusCode: status, Body: sanitizeHTTPBody(body)}
	}
	if status < 200 || status >= 300 {
		return fmt.Sprintf("jaeger status check failed: %d", status), nil
	}
	return "jaeger backend reachable", nil
}

func (c Client) JaegerSearch(service string, window time.Duration, limit int) ([]Trace, error) {
	if strings.TrimSpace(c.BaseURL) == "" {
		return nil, errors.New("missing trace url")
	}
	if strings.TrimSpace(service) == "" {
		return nil, errors.New("missing service")
	}
	if limit <= 0 {
		limit = 5
	}
	base := strings.TrimSuffix(c.BaseURL, "/")
	endpoint, err := url.Parse(base + "/api/traces")
	if err != nil {
		return nil, err
	}
	q := endpoint.Query()
	q.Set("service", service)
	q.Set("lookback", fmt.Sprintf("%dm", int(window.Minutes())))
	q.Set("limit", fmt.Sprintf("%d", limit))
	endpoint.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	applyAuth(req, c)

	status, body, err := doRequest(req, c.Timeout, c.BaseURL, c.QPS)
	if err != nil {
		return nil, err
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return nil, HTTPError{StatusCode: status, Body: sanitizeHTTPBody(body)}
	}
	if status < 200 || status >= 300 {
		return nil, HTTPError{StatusCode: status, Body: sanitizeHTTPBody(body)}
	}

	var parsed jaegerTracesResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	traces := make([]Trace, 0, len(parsed.Data))
	for _, t := range parsed.Data {
		if strings.TrimSpace(t.TraceID) == "" {
			continue
		}
		trace := parseJaegerTrace(t)
		traces = append(traces, trace)
	}
	return traces, nil
}

func parseJaegerTrace(t struct {
	TraceID   string                   `json:"traceID"`
	Spans     []jaegerSpan             `json:"spans"`
	Processes map[string]jaegerProcess `json:"processes"`
}) Trace {
	trace := Trace{TraceID: t.TraceID}
	var slowest Span
	var root Span
	var spans []Span
	hasDBSpan := false
	spanByID := map[string]jaegerSpan{}
	parentIDs := map[string]struct{}{}
	for _, s := range t.Spans {
		spanByID[s.SpanID] = s
		for _, ref := range s.References {
			if strings.EqualFold(ref.RefType, "CHILD_OF") {
				parentIDs[s.SpanID] = struct{}{}
			}
		}
		isDB := hasJaegerDBTags(s.Tags)
		span := convertJaegerSpan(s, t.Processes, isDB)
		spans = append(spans, span)
		if isDB {
			hasDBSpan = true
		}
		if span.Duration > slowest.Duration {
			slowest = span
		}
	}
	for _, s := range t.Spans {
		if _, ok := parentIDs[s.SpanID]; !ok {
			root = convertJaegerSpan(s, t.Processes, hasJaegerDBTags(s.Tags))
			break
		}
	}
	if root.SpanID == "" {
		root = findRootSpan(spans)
	}
	trace.SpanCount = len(spans)
	limited, truncated := limitSpans(spans, root, MaxSpansPerTrace)
	trace.Truncated = truncated
	trace.RootSpan = root
	if slowest.SpanID == "" {
		slowest = findSlowestSpan(limited)
	}
	trace.SlowestSpan = slowest
	trace.Duration = findTraceDuration(spans)
	trace.HasDBSpan = hasDBSpan
	return trace
}

func convertJaegerSpan(s jaegerSpan, processes map[string]jaegerProcess, isDB bool) Span {
	service := ""
	if p, ok := processes[s.ProcessID]; ok {
		service = p.ServiceName
	}
	name := strings.TrimSpace(s.OperationName)
	if name == "" {
		name = firstJaegerTagValue(s.Tags, []string{"db.statement", "db.operation", "rpc.method", "messaging.operation", "messaging.destination"})
	}
	return Span{
		SpanID:      s.SpanID,
		ParentID:    findParentID(s.References),
		Service:     service,
		PeerService: extractJaegerPeerService(s.Tags),
		Name:        name,
		Start:       time.Unix(0, s.StartTime*int64(time.Microsecond)),
		Duration:    time.Duration(s.Duration) * time.Microsecond,
		IsDB:        isDB,
	}
}

func extractJaegerPeerService(tags []jaegerTag) string {
	if value := firstJaegerTagValue(tags, []string{"peer.service", "peer_service"}); value != "" {
		return value
	}
	if value := firstJaegerTagValue(tags, []string{"db.name", "db.system"}); value != "" {
		return value
	}
	if value := firstJaegerTagValue(tags, []string{"net.peer.name", "server.address", "server"}); value != "" {
		return value
	}
	return ""
}

func firstJaegerTagValue(tags []jaegerTag, keys []string) string {
	for _, key := range keys {
		for _, tag := range tags {
			if !strings.EqualFold(strings.TrimSpace(tag.Key), key) {
				continue
			}
			switch v := tag.Value.(type) {
			case string:
				if strings.TrimSpace(v) != "" {
					return v
				}
			default:
				if v != nil {
					return fmt.Sprint(v)
				}
			}
		}
	}
	return ""
}

func hasJaegerDBTags(tags []jaegerTag) bool {
	for _, tag := range tags {
		key := strings.ToLower(strings.TrimSpace(tag.Key))
		if !strings.HasPrefix(key, "db.") {
			continue
		}
		switch v := tag.Value.(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				return true
			}
		default:
			if v != nil {
				return true
			}
		}
	}
	return false
}

func findParentID(refs []jaegerRef) string {
	for _, ref := range refs {
		if strings.EqualFold(ref.RefType, "CHILD_OF") {
			return ref.SpanID
		}
	}
	return ""
}
