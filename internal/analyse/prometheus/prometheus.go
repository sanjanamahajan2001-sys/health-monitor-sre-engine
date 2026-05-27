package prometheus

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

	mu                sync.RWMutex
	metricNamesCache  []string
	metricNamesLoaded bool
	seriesCache       map[string][]map[string]string
}

type HTTPError struct {
	StatusCode int
	Body       string
}

func (e HTTPError) Error() string {
	return fmt.Sprintf("prometheus http error: status=%d", e.StatusCode)
}

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

type queryResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Value  []interface{}     `json:"value"`
		} `json:"result"`
	} `json:"data"`
	Error string `json:"error"`
}

type labelValuesResponse struct {
	Status string   `json:"status"`
	Data   []string `json:"data"`
	Error  string   `json:"error"`
}

type seriesResponse struct {
	Status string              `json:"status"`
	Data   []map[string]string `json:"data"`
	Error  string              `json:"error"`
}

type targetsResponse struct {
	Status string `json:"status"`
	Data   struct {
		ActiveTargets []struct {
			Health string `json:"health"`
		} `json:"activeTargets"`
	} `json:"data"`
	Error string `json:"error"`
}

var ErrNoData = errors.New("no data")

type rateLimiter struct {
	interval time.Duration
	last     time.Time
}

var (
	limitersMu sync.Mutex
	limiters   = map[string]*rateLimiter{}
)

type Sample struct {
	Metric map[string]string
	Value  float64
}

func (c *Client) QueryInstant(query string) (float64, error) {
	if c.BaseURL == "" {
		return 0, errors.New("missing prometheus url")
	}
	endpoint, err := url.Parse(strings.TrimSuffix(c.BaseURL, "/") + "/api/v1/query")
	if err != nil {
		return 0, err
	}

	q := endpoint.Query()
	q.Set("query", query)
	endpoint.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", endpoint.String(), nil)
	if err != nil {
		return 0, err
	}

	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	} else if c.User != "" || c.Pass != "" {
		req.SetBasicAuth(c.User, c.Pass)
	}

	status, rbody, ebody, err := doRequest(req, c.Timeout, c.BaseURL, c.QPS)
	if err != nil {
		return 0, err
	}
	if rbody != nil {
		defer rbody.Close()
	}

	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return 0, HTTPError{StatusCode: status, Body: sanitizeHTTPBody(ebody)}
	}
	if status < 200 || status >= 300 {
		return 0, HTTPError{StatusCode: status, Body: sanitizeHTTPBody(ebody)}
	}

	var parsed queryResponse
	if err := json.NewDecoder(rbody).Decode(&parsed); err != nil {
		return 0, err
	}
	if parsed.Status != "success" {
		if parsed.Error != "" {
			return 0, errors.New(parsed.Error)
		}
		return 0, errors.New("prometheus query failed")
	}
	if len(parsed.Data.Result) == 0 {
		return 0, ErrNoData
	}
	if len(parsed.Data.Result) > 1 {
		return 0, fmt.Errorf("prometheus query returned %d series; expected 1", len(parsed.Data.Result))
	}
	if len(parsed.Data.Result[0].Value) < 2 {
		return 0, ErrNoData
	}

	valStr, ok := parsed.Data.Result[0].Value[1].(string)
	if !ok {
		return 0, errors.New("unexpected value type")
	}

	var val float64
	if _, err := fmt.Sscanf(valStr, "%f", &val); err != nil {
		return 0, err
	}

	return val, nil
}

func (c *Client) Check() (string, error) {
	if c.BaseURL == "" {
		return "", errors.New("missing prometheus url")
	}

	endpoint := strings.TrimSuffix(c.BaseURL, "/") + "/-/ready"
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return "", err
	}

	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	} else if c.User != "" || c.Pass != "" {
		req.SetBasicAuth(c.User, c.Pass)
	}

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

func (c *Client) QueryVector(query string) ([]Sample, error) {
	if c.BaseURL == "" {
		return nil, errors.New("missing prometheus url")
	}

	endpoint, err := url.Parse(strings.TrimSuffix(c.BaseURL, "/") + "/api/v1/query")
	if err != nil {
		return nil, err
	}

	q := endpoint.Query()
	q.Set("query", query)
	endpoint.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", endpoint.String(), nil)
	if err != nil {
		return nil, err
	}

	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	} else if c.User != "" || c.Pass != "" {
		req.SetBasicAuth(c.User, c.Pass)
	}

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
		return nil, errors.New("invalid prometheus response: missing status")
	}
	var statusStr string
	if err := dec.Decode(&statusStr); err != nil || statusStr != "success" {
		findKey("error")
		var errStr string
		dec.Decode(&errStr)
		if errStr != "" {
			return nil, errors.New(errStr)
		}
		return nil, errors.New("prometheus query failed")
	}

	// 2. Navigate to data.result
	if !findKey("data") {
		return nil, errors.New("invalid prometheus response: missing data")
	}
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return nil, errors.New("invalid prometheus response: data is not an object")
	}

	if !findKey("result") {
		return nil, errors.New("invalid prometheus response: missing result")
	}

	// 3. Process result array
	if t, err := dec.Token(); err != nil || t != json.Delim('[') {
		return nil, errors.New("invalid prometheus response: result is not an array")
	}

	samples := make([]Sample, 0)
	for dec.More() {
		var r struct {
			Metric map[string]string `json:"metric"`
			Value  []interface{}     `json:"value"`
		}
		if err := dec.Decode(&r); err != nil {
			continue
		}
		if len(r.Value) < 2 {
			continue
		}
		valStr, ok := r.Value[1].(string)
		if !ok {
			continue
		}
		var val float64
		if _, err := fmt.Sscanf(valStr, "%f", &val); err != nil {
			continue
		}
		samples = append(samples, Sample{
			Metric: r.Metric,
			Value:  val,
		})
	}

	if len(samples) == 0 {
		return nil, ErrNoData
	}
	return samples, nil
}

func (c *Client) MetricNames() ([]string, error) {
	c.mu.RLock()
	if c.metricNamesLoaded {
		data := make([]string, len(c.metricNamesCache))
		copy(data, c.metricNamesCache)
		c.mu.RUnlock()
		return data, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()
	// Double check after acquiring write lock
	if c.metricNamesLoaded {
		data := make([]string, len(c.metricNamesCache))
		copy(data, c.metricNamesCache)
		return data, nil
	}
	if c.BaseURL == "" {
		return nil, errors.New("missing prometheus url")
	}

	endpoint, err := url.Parse(strings.TrimSuffix(c.BaseURL, "/") + "/api/v1/label/__name__/values")
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("GET", endpoint.String(), nil)
	if err != nil {
		return nil, err
	}

	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	} else if c.User != "" || c.Pass != "" {
		req.SetBasicAuth(c.User, c.Pass)
	}

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
		return nil, errors.New("prometheus label values failed")
	}
	c.metricNamesCache = parsed.Data
	c.metricNamesLoaded = true
	return parsed.Data, nil
}

func (c *Client) TargetsStatus() (int, int, error) {
	if c.BaseURL == "" {
		return 0, 0, errors.New("missing prometheus url")
	}
	endpoint, err := url.Parse(strings.TrimSuffix(c.BaseURL, "/") + "/api/v1/targets")
	if err != nil {
		return 0, 0, err
	}
	req, err := http.NewRequest("GET", endpoint.String(), nil)
	if err != nil {
		return 0, 0, err
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	} else if c.User != "" || c.Pass != "" {
		req.SetBasicAuth(c.User, c.Pass)
	}
	status, rbody, ebody, err := doRequest(req, c.Timeout, c.BaseURL, c.QPS)
	if err != nil {
		return 0, 0, err
	}
	if rbody != nil {
		defer rbody.Close()
	}

	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return 0, 0, HTTPError{StatusCode: status, Body: sanitizeHTTPBody(ebody)}
	}
	if status < 200 || status >= 300 {
		return 0, 0, HTTPError{StatusCode: status, Body: sanitizeHTTPBody(ebody)}
	}
	var parsed targetsResponse
	if err := json.NewDecoder(rbody).Decode(&parsed); err != nil {
		return 0, 0, err
	}
	if parsed.Status != "success" {
		if parsed.Error != "" {
			return 0, 0, errors.New(parsed.Error)
		}
		return 0, 0, errors.New("prometheus targets query failed")
	}
	up := 0
	down := 0
	for _, target := range parsed.Data.ActiveTargets {
		if strings.EqualFold(target.Health, "up") {
			up++
		} else {
			down++
		}
	}
	return up, down, nil
}

func (c *Client) Series(match string, lookback time.Duration) ([]map[string]string, error) {
	c.mu.RLock()
	if c.seriesCache == nil {
		c.mu.RUnlock()
		c.mu.Lock()
		if c.seriesCache == nil {
			c.seriesCache = map[string][]map[string]string{}
		}
		c.mu.Unlock()
		c.mu.RLock()
	}

	cacheKey := match + "|" + lookback.String()
	if cached, ok := c.seriesCache[cacheKey]; ok {
		c.mu.RUnlock()
		return cached, nil
	}
	c.mu.RUnlock()
	if c.BaseURL == "" {
		return nil, errors.New("missing prometheus url")
	}

	endpoint, err := url.Parse(strings.TrimSuffix(c.BaseURL, "/") + "/api/v1/series")
	if err != nil {
		return nil, err
	}

	now := time.Now()
	q := endpoint.Query()
	selector := match
	if !strings.Contains(match, "{") {
		selector = fmt.Sprintf(`{__name__="%s"}`, match)
	}
	q.Add("match[]", selector)
	q.Set("start", fmt.Sprintf("%d", now.Add(-lookback).Unix()))
	q.Set("end", fmt.Sprintf("%d", now.Unix()))
	endpoint.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", endpoint.String(), nil)
	if err != nil {
		return nil, err
	}

	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	} else if c.User != "" || c.Pass != "" {
		req.SetBasicAuth(c.User, c.Pass)
	}

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

	var parsed seriesResponse
	if err := json.NewDecoder(rbody).Decode(&parsed); err != nil {
		return nil, err
	}
	if parsed.Status != "success" {
		if parsed.Error != "" {
			return nil, errors.New(parsed.Error)
		}
		return nil, errors.New("prometheus series query failed")
	}
	if len(parsed.Data) == 0 {
		return nil, ErrNoData
	}
	c.mu.Lock()
	c.seriesCache[cacheKey] = parsed.Data
	c.mu.Unlock()
	return parsed.Data, nil
}
func (c *Client) LabelValues(label string, lookback time.Duration) ([]string, error) {
	if c.BaseURL == "" {
		return nil, errors.New("missing prometheus url")
	}

	endpoint, err := url.Parse(strings.TrimSuffix(c.BaseURL, "/") + "/api/v1/label/" + label + "/values")
	if err != nil {
		return nil, err
	}

	now := time.Now()
	q := endpoint.Query()
	q.Set("start", fmt.Sprintf("%d", now.Add(-lookback).Unix()))
	q.Set("end", fmt.Sprintf("%d", now.Unix()))
	endpoint.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", endpoint.String(), nil)
	if err != nil {
		return nil, err
	}

	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	} else if c.User != "" || c.Pass != "" {
		req.SetBasicAuth(c.User, c.Pass)
	}

	status, rbody, ebody, err := doRequest(req, c.Timeout, c.BaseURL, c.QPS)
	if err != nil {
		return nil, err
	}
	if rbody != nil {
		defer rbody.Close()
	}

	if status < 200 || status >= 300 {
		return nil, HTTPError{StatusCode: status, Body: sanitizeHTTPBody(ebody)}
	}

	var parsed struct {
		Status string   `json:"status"`
		Data   []string `json:"data"`
		Error  string   `json:"error"`
	}
	if err := json.NewDecoder(rbody).Decode(&parsed); err != nil {
		return nil, err
	}
	if parsed.Status != "success" {
		if parsed.Error != "" {
			return nil, errors.New(parsed.Error)
		}
		return nil, errors.New("prometheus label values query failed")
	}

	return parsed.Data, nil
}
