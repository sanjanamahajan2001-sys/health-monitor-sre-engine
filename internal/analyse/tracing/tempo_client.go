package tracing

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type tempoSearchResponse struct {
	Traces []struct {
		TraceID    string `json:"traceID"`
		TraceIdAlt string `json:"traceId"`
		DurationMs int64  `json:"durationMs"`
		RootSvc    string `json:"rootServiceName"`
	} `json:"traces"`
}

type tempoServiceCount struct {
	name  string
	count int
}

func (c Client) TempoHealth() (string, error) {
	if strings.TrimSpace(c.BaseURL) == "" {
		return "", errors.New("missing trace url")
	}
	base := strings.TrimSuffix(c.BaseURL, "/")
	endpoints := []string{
		base + "/ready",
		base + "/status",
		base + "/api/status",
	}
	var lastNon404 int
	for _, endpoint := range endpoints {
		req, err := http.NewRequest(http.MethodGet, endpoint, nil)
		if err != nil {
			continue
		}
		applyAuth(req, c)
		status, body, err := doRequest(req, c.Timeout, c.BaseURL, c.QPS)
		if err != nil {
			continue
		}
		if status == http.StatusUnauthorized || status == http.StatusForbidden {
			return "", HTTPError{StatusCode: status, Body: sanitizeHTTPBody(body)}
		}
		if status >= 200 && status < 300 {
			return "tempo backend reachable", nil
		}
		if status != http.StatusNotFound {
			lastNon404 = status
		}
	}
	if lastNon404 != 0 {
		return fmt.Sprintf("tempo status check failed: %d", lastNon404), nil
	}
	return "tempo status check skipped (no status endpoint)", nil
}

func (c Client) TempoSearch(service string, window time.Duration, limit int) ([]Trace, error) {
	if strings.TrimSpace(c.BaseURL) == "" {
		return nil, errors.New("missing trace url")
	}
	if limit <= 0 {
		limit = 5
	}
	base := strings.TrimSuffix(c.BaseURL, "/")
	endpoint, err := url.Parse(base + "/api/search")
	if err != nil {
		return nil, err
	}
	q := endpoint.Query()
	if strings.TrimSpace(service) != "" {
		q.Set("service", service)
	}
	q.Set("limit", fmt.Sprintf("%d", limit))
	q.Set("start", fmt.Sprintf("%d", time.Now().Add(-window).Unix()))
	q.Set("end", fmt.Sprintf("%d", time.Now().Unix()))
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

	var parsed tempoSearchResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	traces := make([]Trace, 0, len(parsed.Traces))
	for _, t := range parsed.Traces {
		traceID := strings.TrimSpace(t.TraceID)
		if traceID == "" {
			traceID = strings.TrimSpace(t.TraceIdAlt)
		}
		if traceID == "" {
			continue
		}
		traces = append(traces, Trace{
			TraceID:  traceID,
			Duration: time.Duration(t.DurationMs) * time.Millisecond,
		})
	}
	return traces, nil
}

func (c Client) TempoSearchServices(window time.Duration, limit int) ([]string, error) {
	if strings.TrimSpace(c.BaseURL) == "" {
		return nil, errors.New("missing trace url")
	}
	if limit <= 0 {
		limit = 5
	}
	base := strings.TrimSuffix(c.BaseURL, "/")
	endpoint, err := url.Parse(base + "/api/search")
	if err != nil {
		return nil, err
	}
	q := endpoint.Query()
	q.Set("limit", fmt.Sprintf("%d", limit))
	q.Set("start", fmt.Sprintf("%d", time.Now().Add(-window).Unix()))
	q.Set("end", fmt.Sprintf("%d", time.Now().Unix()))
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

	var parsed tempoSearchResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, t := range parsed.Traces {
		name := strings.TrimSpace(t.RootSvc)
		if name == "" {
			continue
		}
		counts[name]++
	}
	if len(counts) == 0 {
		return nil, nil
	}
	scored := make([]tempoServiceCount, 0, len(counts))
	for name, count := range counts {
		scored = append(scored, tempoServiceCount{name: name, count: count})
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].count == scored[j].count {
			return scored[i].name < scored[j].name
		}
		return scored[i].count > scored[j].count
	})
	services := make([]string, 0, len(scored))
	for _, item := range scored {
		services = append(services, item.name)
	}
	return services, nil
}

func (c Client) TempoFetchTrace(traceID string) (Trace, error) {
	if strings.TrimSpace(traceID) == "" {
		return Trace{}, errors.New("missing trace id")
	}
	base := strings.TrimSuffix(c.BaseURL, "/")
	endpoint := base + "/api/traces/" + url.PathEscape(traceID)
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return Trace{}, err
	}
	applyAuth(req, c)

	status, body, err := doRequest(req, c.Timeout, c.BaseURL, c.QPS)
	if err != nil {
		return Trace{}, err
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return Trace{}, HTTPError{StatusCode: status, Body: sanitizeHTTPBody(body)}
	}
	if status < 200 || status >= 300 {
		return Trace{}, HTTPError{StatusCode: status, Body: sanitizeHTTPBody(body)}
	}
	return parseAnyTraceResponse(traceID, body)
}
