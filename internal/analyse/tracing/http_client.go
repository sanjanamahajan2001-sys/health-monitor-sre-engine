package tracing

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type HTTPError struct {
	StatusCode int
	Body       string
}

func (e HTTPError) Error() string {
	return fmt.Sprintf("trace http error: status=%d", e.StatusCode)
}

type rateLimiter struct {
	interval time.Duration
	last     time.Time
}

var (
	limitersMu sync.Mutex
	limiters   = map[string]*rateLimiter{}
)

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

func doRequest(req *http.Request, timeout time.Duration, baseURL string, qps float64) (int, []byte, error) {
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
			return 0, nil, err
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode >= 500 && attempt < maxRetries {
			time.Sleep(backoff)
			backoff *= 2
			continue
		}
		return resp.StatusCode, body, nil
	}
	return 0, nil, lastErr
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

func applyAuth(req *http.Request, c Client) {
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
		return
	}
	if c.User != "" || c.Pass != "" {
		req.SetBasicAuth(c.User, c.Pass)
	}
}
