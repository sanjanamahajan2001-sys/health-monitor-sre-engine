package traces

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// doRequest helper for trace backends with robust error handling
func doRequest(ctx context.Context, client *http.Client, req *http.Request, target interface{}) error {
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("User-Agent", "HealthMonitor/1.0")
	resp, err := client.Do(req.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, sanitizeBody(body))
	}

	// Check if content is actually JSON
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(strings.ToLower(contentType), "application/json") && len(body) > 0 {
		// If it looks like HTML, report it clearly
		if strings.Contains(strings.ToLower(string(body)), "<html") {
			return fmt.Errorf("received HTML response instead of JSON (check if URL is correct or if there is a proxy/login page): %s", sanitizeBody(body))
		}
		return fmt.Errorf("received non-JSON response (Content-Type: %s): %s", contentType, sanitizeBody(body))
	}

	if err := json.Unmarshal(body, target); err != nil {
		// If decoding fails, provide context about what we tried to decode
		return fmt.Errorf("failed to decode JSON response: %w. Body snippet: %s", err, sanitizeBody(body))
	}

	return nil
}

func sanitizeBody(body []byte) string {
	if len(body) == 0 {
		return "(empty body)"
	}
	s := strings.TrimSpace(string(body))
	if len(s) > 512 {
		return s[:512] + "..."
	}
	return s
}

// setAuth sets standard authentication for trace requests
func setAuth(req *http.Request, token, user, pass string) {
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	} else if user != "" && pass != "" {
		req.SetBasicAuth(user, pass)
	}
}
