package loki

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Client struct {
	BaseURL string
	Token   string
	User    string
	Pass    string
	Timeout time.Duration
	QPS     float64
}

type HTTPError struct {
	StatusCode int
	Body       string
}

func (e HTTPError) Error() string {
	return fmt.Sprintf("loki http error: status=%d", e.StatusCode)
}

type rateLimiter struct {
	interval time.Duration
	last     time.Time
}

var (
	limitersMu sync.Mutex
	limiters   = map[string]*rateLimiter{}
)

func sanitizeHTTPBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	out := strings.TrimSpace(string(body))
	if len(out) > 256 {
		out = out[:256] + "..."
	}
	return out
}

func throttle(baseURL string, qps float64) {
	if qps <= 0 {
		return
	}
	interval := time.Duration(float64(time.Second) / qps)
	if interval < time.Millisecond {
		interval = time.Millisecond
	}
	limitersMu.Lock()
	limiter := limiters[baseURL]
	if limiter == nil || limiter.interval != interval {
		limiter = &rateLimiter{interval: interval}
		limiters[baseURL] = limiter
	}
	wait := time.Until(limiter.last.Add(limiter.interval))
	if wait > 0 {
		limitersMu.Unlock()
		time.Sleep(wait)
		limitersMu.Lock()
	}
	limiter.last = time.Now()
	limitersMu.Unlock()
}

func isRetryableError(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout() || netErr.Temporary()
	}
	return false
}

func doRequest(req *http.Request, timeout time.Duration, baseURL string, qps float64) (int, io.ReadCloser, []byte, error) {
	const maxRetries = 2
	backoff := 200 * time.Millisecond
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		throttle(baseURL, qps)
		client := &http.Client{Timeout: timeout}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			if attempt < maxRetries && isRetryableError(err) {
				time.Sleep(backoff)
				backoff *= 2
				continue
			}
			return 0, nil, nil, err
		}

		if resp.StatusCode >= 500 && attempt < maxRetries {
			resp.Body.Close()
			time.Sleep(backoff)
			backoff *= 2
			continue
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return resp.StatusCode, nil, body, nil
		}

		return resp.StatusCode, resp.Body, nil, nil
	}
	return 0, nil, nil, lastErr
}

type LogEntry struct {
	Timestamp time.Time
	Line      string
}

type queryRangeResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Stream map[string]string `json:"stream"`
			Values [][]string        `json:"values"`
		} `json:"result"`
	} `json:"data"`
	Error string `json:"error"`
}

type labelValuesResponse struct {
	Status string   `json:"status"`
	Data   []string `json:"data"`
	Error  string   `json:"error"`
}

func (c Client) Check() (string, error) {
	if c.BaseURL == "" {
		return "", errors.New("missing loki url")
	}
	endpoint := strings.TrimSuffix(c.BaseURL, "/") + "/ready"
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return "", err
	}
	applyAuth(req, c)
	status, rbody, ebody, err := doRequest(req, c.Timeout, c.BaseURL, c.QPS)
	if err != nil {
		return "", err
	}
	if rbody != nil {
		defer rbody.Close()
	}

	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return "", HTTPError{StatusCode: status, Body: sanitizeHTTPBody(ebody)}
	}
	if status < 200 || status >= 300 {
		return "", HTTPError{StatusCode: status, Body: sanitizeHTTPBody(ebody)}
	}

	res, err := io.ReadAll(rbody)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(res)), nil
}

func (c Client) Labels() ([]string, error) {
	if c.BaseURL == "" {
		return nil, errors.New("missing loki url")
	}
	endpoint := strings.TrimSuffix(c.BaseURL, "/") + "/loki/api/v1/labels"
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	applyAuth(req, c)
	status, rbody, ebody, err := doRequest(req, c.Timeout, c.BaseURL, c.QPS)
	if err != nil {
		return nil, err
	}
	if rbody != nil {
		defer rbody.Close()
	}

	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return nil, HTTPError{StatusCode: status, Body: sanitizeHTTPBody(ebody)}
	}
	if status < 200 || status >= 300 {
		return nil, HTTPError{StatusCode: status, Body: sanitizeHTTPBody(ebody)}
	}
	var parsed labelValuesResponse
	if err := json.NewDecoder(rbody).Decode(&parsed); err != nil {
		return nil, err
	}
	if parsed.Status != "success" {
		if parsed.Error != "" {
			return nil, errors.New(parsed.Error)
		}
		return nil, errors.New("loki labels query failed")
	}
	return parsed.Data, nil
}

func (c Client) LabelValues(label string) ([]string, error) {
	if c.BaseURL == "" {
		return nil, errors.New("missing loki url")
	}
	if strings.TrimSpace(label) == "" {
		return nil, errors.New("missing label name")
	}
	endpoint := strings.TrimSuffix(c.BaseURL, "/") + "/loki/api/v1/label/" + url.PathEscape(label) + "/values"
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	applyAuth(req, c)
	status, rbody, ebody, err := doRequest(req, c.Timeout, c.BaseURL, c.QPS)
	if err != nil {
		return nil, err
	}
	if rbody != nil {
		defer rbody.Close()
	}

	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return nil, HTTPError{StatusCode: status, Body: sanitizeHTTPBody(ebody)}
	}
	if status < 200 || status >= 300 {
		return nil, HTTPError{StatusCode: status, Body: sanitizeHTTPBody(ebody)}
	}
	var parsed labelValuesResponse
	if err := json.NewDecoder(rbody).Decode(&parsed); err != nil {
		return nil, err
	}
	if parsed.Status != "success" {
		if parsed.Error != "" {
			return nil, errors.New(parsed.Error)
		}
		return nil, errors.New("loki label values query failed")
	}
	return parsed.Data, nil
}

func (c Client) QueryRange(query string, start time.Time, end time.Time, limit int) ([]LogEntry, error) {
	if c.BaseURL == "" {
		return nil, errors.New("missing loki url")
	}
	endpoint, err := url.Parse(strings.TrimSuffix(c.BaseURL, "/") + "/loki/api/v1/query_range")
	if err != nil {
		return nil, err
	}
	q := endpoint.Query()
	q.Set("query", query)
	q.Set("start", fmt.Sprintf("%d", start.UnixNano()))
	q.Set("end", fmt.Sprintf("%d", end.UnixNano()))
	if limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", limit))
	}
	q.Set("direction", "backward")
	endpoint.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	applyAuth(req, c)
	status, rbody, ebody, err := doRequest(req, c.Timeout, c.BaseURL, c.QPS)
	if err != nil {
		return nil, err
	}
	if rbody != nil {
		defer rbody.Close()
	}

	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return nil, HTTPError{StatusCode: status, Body: sanitizeHTTPBody(ebody)}
	}
	if status < 200 || status >= 300 {
		return nil, HTTPError{StatusCode: status, Body: sanitizeHTTPBody(ebody)}
	}

	dec := json.NewDecoder(rbody)
	
	// Helper to find key in object
	findKey := func(target string) bool {
		for dec.More() {
			t, err := dec.Token()
			if err != nil {
				return false
			}
			if s, ok := t.(string); ok && s == target {
				return true
			}
		}
		return false
	}

	// 1. Check status
	if !findKey("status") {
		return nil, errors.New("invalid loki response: missing status")
	}
	var statusStr string
	if err := dec.Decode(&statusStr); err != nil || statusStr != "success" {
		// Try to find error if status is not success
		findKey("error")
		var errStr string
		dec.Decode(&errStr)
		if errStr != "" {
			return nil, errors.New(errStr)
		}
		return nil, errors.New("loki query failed")
	}

	// 2. Navigate to data.result
	if !findKey("data") {
		return nil, errors.New("invalid loki response: missing data")
	}
	// Expect {
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return nil, errors.New("invalid loki response: data is not an object")
	}

	if !findKey("result") {
		return nil, errors.New("invalid loki response: missing result")
	}

	// 3. Process result array
	if t, err := dec.Token(); err != nil || t != json.Delim('[') {
		return nil, errors.New("invalid loki response: result is not an array")
	}

	entries := []LogEntry{}
	for dec.More() {
		var stream struct {
			Stream map[string]string `json:"stream"`
			Values [][]string        `json:"values"`
		}
		if err := dec.Decode(&stream); err != nil {
			return nil, fmt.Errorf("failed to decode stream: %w", err)
		}

		for _, pair := range stream.Values {
			if len(pair) < 2 {
				continue
			}
			ts, err := parseUnixNano(pair[0])
			if err != nil {
				continue
			}
			entries = append(entries, LogEntry{
				Timestamp: ts,
				Line:      pair[1],
			})
		}
	}

	return entries, nil
}

func applyAuth(req *http.Request, c Client) {
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
		return
	}
	if c.User != "" || c.Pass != "" {
		req.SetBasicAuth(c.User, c.Pass)
	}
}

func parseUnixNano(value string) (time.Time, error) {
	ns, err := parseInt64(value)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(0, ns), nil
}

func parseInt64(value string) (int64, error) {
	var out int64
	if _, err := fmt.Sscanf(value, "%d", &out); err != nil {
		return 0, err
	}
	return out, nil
}

// DiscoverServiceLabel attempts to find the most likely labels used for service names in Loki
func (c Client) DiscoverServiceLabel() (string, error) {
	candidates := []string{"service_name", "service", "app", "app_kubernetes_io_name", "kubernetes_name", "container", "job"}
	
	labels, err := c.Labels()
	if err != nil {
		return "", err
	}

	type labelStore struct {
		label string
		count int
	}
	var scores []labelStore

	for _, cand := range candidates {
		found := false
		for _, l := range labels {
			if l == cand {
				found = true
				break
			}
		}

		if found {
			values, err := c.LabelValues(cand)
			if err == nil && len(values) > 0 {
				scores = append(scores, labelStore{label: cand, count: len(values)})
			}
		}
	}

	// Sort by count descending
	for i := 0; i < len(scores); i++ {
		for j := i + 1; j < len(scores); j++ {
			if scores[j].count > scores[i].count {
				scores[i], scores[j] = scores[j], scores[i]
			}
		}
	}

	var results []string
	for i := 0; i < len(scores) && i < 10; i++ {
		results = append(results, scores[i].label)
	}

	// Ensure common labels are always included if they exist in the candidates but didn't make the top 10
	alwaysInclude := []string{"service", "app", "container"}
	for _, l := range labels {
		for _, ai := range alwaysInclude {
			if l == ai {
				// Check if already in results
				found := false
				for _, r := range results {
					if r == ai {
						found = true
						break
					}
				}
				if !found {
					results = append(results, ai)
				}
			}
		}
	}

	if len(results) > 0 {
		return strings.Join(results, ","), nil
	}

	return "service", nil // Default fallback
}


