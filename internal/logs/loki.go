package logs

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	
	"health-monitor/internal/metrics"
	"health-monitor/internal/output"
)

// LokiClient implements LogBackend for Grafana Loki
type LokiClient struct {
	baseURL    string
	username   string
	password   string
	token      string
	serviceLabel string
	errorRegex  *regexp.Regexp
	client     *http.Client
}

// LokiQueryResponse represents a Loki query response
type LokiQueryResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Values [][]interface{}   `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

// NewLokiClient creates a new Loki client with connection pooling
func NewLokiClient(baseURL, username, password, token, serviceLabel, errorRegex string) (*LokiClient, error) {
	var regex *regexp.Regexp
	if errorRegex != "" {
		var err error
		regex, err = regexp.Compile(errorRegex)
		if err != nil {
			return nil, fmt.Errorf("invalid error regex: %w", err)
		}
	} else {
		// Default error patterns (Comprehensive: captures production-critical status codes and keywords)
		regex = regexp.MustCompile(`(?i)(error|exception|fail|timeout|refused|deadlock|warn|warning|critical|severe|fatal|panic|status\s*[45][0-9][0-9]|HTTP\s*[45][0-9][0-9])`)
	}

	// Create HTTP client with connection pooling and timeout
	client := &http.Client{
		Timeout: 30 * time.Second, // Default timeout
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
			DisableCompression:  false,
		},
	}

	return &LokiClient{
		baseURL:     strings.TrimSuffix(baseURL, "/"),
		username:    username,
		password:    password,
		token:       token,
		serviceLabel: serviceLabel,
		errorRegex:  regex,
		client:      client,
	}, nil
}

// TopErrors returns the top N error messages for a service in a time window
func (l *LokiClient) TopErrors(ctx context.Context, service string, start, end time.Time) ([]ErrorStat, error) {
	queryStart := time.Now()
	queries := l.buildQueriesForLabels(service)
	
	for _, query := range queries {
		// Check context before each attempt
		select {
		case <-ctx.Done():
			return []ErrorStat{}, ctx.Err()
		default:
		}

		if service == "frontend" || service == "valkey-cart" {
			log.Printf("[DEBUG] Loki LogQL Query for %s: %s", service, query)
		}
		queryURL := l.buildQueryURL(query, start, end)

		req, err := http.NewRequestWithContext(ctx, "GET", queryURL, nil)
		if err != nil {
			continue
		}
		l.setAuth(req)

		resp, err := l.client.Do(req)
		if err != nil {
			continue
		}
		
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			continue
		}

		var lokiResp LokiQueryResponse
		err = json.NewDecoder(resp.Body).Decode(&lokiResp)
		resp.Body.Close()
		
		if len(lokiResp.Data.Result) > 0 {
			if service == "frontend" || service == "valkey-cart" {
				log.Printf("[DEBUG] Loki: Query returned %d streams for %s", len(lokiResp.Data.Result), service)
			}
			results := l.aggregateErrors(lokiResp, service)
			if len(results) > 0 {
				if service == "frontend" || service == "valkey-cart" {
					log.Printf("[DEBUG] Loki: Found %d error patterns for %s", len(results), service)
				}
				metrics.GetQueryMetrics().RecordLokiQuery(time.Since(queryStart), true)
				return results, nil
			} else {
				if service == "frontend" || service == "valkey-cart" {
					log.Printf("[DEBUG] Loki: Results found for %s but NO error regex match. Regex: %s", service, l.errorRegex.String())
				}
				metrics.GetQueryMetrics().RecordLokiQuery(time.Since(queryStart), true)
				return []ErrorStat{}, nil // 🚀 STOP HERE: Don't check other labels if this one exists but has no errors
			}
		} else {
			// Only log "No results found" for the FIRST label to avoid 4x noise
			if (service == "frontend" || service == "valkey-cart") && queries[0] == query {
				log.Printf("[DEBUG] Loki: No results found for %s after first-sweep query", service)
			}
		}
	}

	metrics.GetQueryMetrics().RecordLokiQuery(time.Since(queryStart), true)
	return []ErrorStat{}, nil
}

// TopErrorsWithLimit returns the top N error messages with configurable limit
func (l *LokiClient) TopErrorsWithLimit(ctx context.Context, service string, start, end time.Time, limit int) ([]ErrorStat, error) {
	queryStart := time.Now()
	queries := l.buildQueriesForLabels(service)
	
	for _, query := range queries {
		output.Debugf("Loki LogQL Query for %s: %s", service, query)
		queryURL := l.buildQueryURLWithLimit(query, start, end, limit)

		req, err := http.NewRequestWithContext(ctx, "GET", queryURL, nil)
		if err != nil {
			continue
		}
		l.setAuth(req)

		resp, err := l.client.Do(req)
		if err != nil {
			continue
		}
		
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			continue
		}

		var lokiResp LokiQueryResponse
		err = json.NewDecoder(resp.Body).Decode(&lokiResp)
		resp.Body.Close()
		
		if err != nil {
			continue
		}

		if len(lokiResp.Data.Result) > 0 {
			results := l.aggregateErrors(lokiResp, service)
			metrics.GetQueryMetrics().RecordLokiQuery(time.Since(queryStart), true)
			return results, nil
		}
	}

	metrics.GetQueryMetrics().RecordLokiQuery(time.Since(queryStart), true)
	return []ErrorStat{}, nil
}

func (l *LokiClient) buildQueriesForLabels(service string) []string {
	rawLabels := strings.Split(l.serviceLabel, ",")
	var labels []string
	for _, lbl := range rawLabels {
		lbl = strings.TrimSpace(lbl)
		if lbl != "" {
			labels = append(labels, lbl)
		}
	}
	if len(labels) == 0 {
		// Prioritize container and app as they are most common in modern environments (Loki/Promtail defaults)
		labels = []string{"container", "app", "service", "job", "name", "pod", "deployment", "namespace", "pod_name"}
	}

	// Support both hyphenated and underscored variants
	variants := []string{service}
	if strings.Contains(service, "-") {
		variants = append(variants, strings.ReplaceAll(service, "-", "_"))
	}
	if strings.Contains(service, "_") {
		variants = append(variants, strings.ReplaceAll(service, "_", "-"))
	}
	variantRegex := strings.Join(variants, "|")

	// Build query for each label combinations
	var queries []string
	regex := l.errorRegex.String()
	
	for _, lbl := range labels {
		// LogQL compatibility: Escape backslashes for double-quoted strings
		safeRegex := strings.ReplaceAll(regex, "\\", "\\\\")
		// Also replace \d with [0-9] for better compatibility across Loki versions
		safeRegex = strings.ReplaceAll(safeRegex, "\\\\d", "[0-9]")
		
		// Avoid double (?i) prefix
		finalRegex := safeRegex
		if !strings.HasPrefix(strings.ToLower(finalRegex), "(?i)") {
			finalRegex = "(?i)" + finalRegex
		}
		
		query := fmt.Sprintf("{%s=~\"%s\"} |~ \"%s\"", lbl, variantRegex, finalRegex)
		if service == "frontend" || service == "valkey-cart" {
			log.Printf("[DEBUG] Generated Loki Query for %s: %s", service, query)
		}
		queries = append(queries, query)
	}
	
	return queries
}

// buildQueryURL creates the Loki query URL with time range
func (l *LokiClient) buildQueryURL(query string, start, end time.Time) string {
	baseURL := strings.TrimSuffix(l.baseURL, "/")
	
	params := url.Values{}
	params.Add("query", query)
	params.Add("start", fmt.Sprintf("%d", start.UnixNano()))
	params.Add("end", fmt.Sprintf("%d", end.UnixNano())) // inclusive range
	params.Add("limit", "200") // Safe default limit
	
	return fmt.Sprintf("%s/loki/api/v1/query_range?%s", baseURL, params.Encode())
}

// buildQueryURLWithLimit creates the Loki query URL with configurable limit
func (l *LokiClient) buildQueryURLWithLimit(query string, start, end time.Time, limit int) string {
	baseURL := strings.TrimSuffix(l.baseURL, "/")
	
	params := url.Values{}
	params.Add("query", query)
	params.Add("start", fmt.Sprintf("%d", start.UnixNano()))
	params.Add("end", fmt.Sprintf("%d", end.UnixNano())) // inclusive range
	if limit > 0 {
		params.Add("limit", fmt.Sprintf("%d", limit))
	} else {
		params.Add("limit", "200") // Safe default
	}
	
	return fmt.Sprintf("%s/loki/api/v1/query_range?%s", baseURL, params.Encode())
}

// setAuth sets authentication on the request
func (l *LokiClient) setAuth(req *http.Request) {
	if l.token != "" {
		req.Header.Set("Authorization", "Bearer "+l.token)
	} else if l.username != "" && l.password != "" {
		req.SetBasicAuth(l.username, l.password)
	}
}

// aggregateErrors processes Loki response and extracts top error messages
func (l *LokiClient) aggregateErrors(resp LokiQueryResponse, service string) []ErrorStat {
	errorCounts := make(map[string]int)
	errorSamples := make(map[string]string)
	
	for i, result := range resp.Data.Result {
		output.Debugf("Loki Stream [%d]: stream labels: %v, values: %d", i, result.Metric, len(result.Values))
		for j, value := range result.Values {
			if len(value) < 2 {
				continue
			}
			
			// Extract log message (second element in the pair)
			logLine, ok := value[1].(string)
			if !ok {
				output.Debugf("Loki Line [%d:%d]: value is not a string: %T", i, j, value[1])
				continue
			}
			
			// Check if log line matches error pattern
			matched := l.errorRegex.MatchString(logLine)
			if !matched {
				if service == "frontend" {
					log.Printf("[DEBUG] Loki (frontend): NO MATCH for regex [%s] on line [%s]", l.errorRegex.String(), logLine)
				}
				continue
			}
			
			// 🚀 META-LOG FILTERING (Production Grade)
			// Ignore logs that are actually Loki/Prometheus internal query metrics
			if strings.Contains(logLine, "query_type=filter") || 
			   strings.Contains(logLine, "latency=fast") ||
			   strings.Contains(logLine, "throughput=") ||
			   strings.Contains(logLine, "subqueries=") {
				continue
			}

			output.Debugf("Loki Line [%d:%d]: MATCH for regex [%s] on line [%s]", i, j, l.errorRegex.String(), logLine)
			
			// Extract error message
			errorMsg := l.extractErrorMessage(logLine)
			if errorMsg == "" {
				if service == "frontend" {
					log.Printf("[DEBUG] Loki (frontend): extractErrorMessage returned empty for matching line: [%s]", logLine)
				}
				continue
			}
			
			// Count occurrences
			errorCounts[errorMsg]++
			
			// Keep first sample
			if _, exists := errorSamples[errorMsg]; !exists {
				errorSamples[errorMsg] = logLine
			}
		}
	}
	
	// Convert to ErrorStat slice and sort by count
	var stats []ErrorStat
	for msg, count := range errorCounts {
		stats = append(stats, ErrorStat{
			Message: msg,
			Count:   count,
			Sample:  errorSamples[msg],
		})
	}
	
	// Sort by count (descending)
	for i := 0; i < len(stats); i++ {
		for j := i + 1; j < len(stats); j++ {
			if stats[j].Count > stats[i].Count {
				stats[i], stats[j] = stats[j], stats[i]
			}
		}
	}
	
	// Return top 5 errors
	if len(stats) > 5 {
		stats = stats[:5]
	}
	
	return stats
}

// extractErrorMessage extracts the main error message from a log line
func (l *LokiClient) extractErrorMessage(logLine string) string {
	// 1. JSON-Aware Extraction (Production Grade)
	trimmed := strings.TrimSpace(logLine)
	if strings.HasPrefix(trimmed, "{") {
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(trimmed), &data); err == nil {
			// Check Level/Severity first
			level := ""
			if l, ok := data["level"].(string); ok {
				level = strings.ToUpper(l)
			} else if l, ok := data["severity"].(string); ok {
				level = strings.ToUpper(l)
			}

			// If level is explicitly low-priority, return empty to skip this noise
			if level == "INFO" || level == "DEBUG" || level == "TRACE" || level == "VERBOSE" {
				return ""
			}

			// Priority fields for the actual error message
			msgFields := []string{"message", "msg", "error", "err", "exception", "body"}
			for _, field := range msgFields {
				if val, ok := data[field].(string); ok && val != "" {
					// Clean the extracted message
					return l.cleanExtractedMessage(val)
				}
			}
			
			// If it's JSON but we can't find a message field, don't fall back to 
			// the raw JSON string as that causes noise (like CVV/IDs)
			return ""
		}
	}

	// 2. Hardened Heuristics Fallback for Plain Text
	return l.cleanExtractedMessage(logLine)
}

func (l *LokiClient) cleanExtractedMessage(msg string) string {
	// 1. Initial sanitization: skip explicit INFO/DEBUG/TRACE markers in plain text
	// This provides a safety net if the regex is too broad.
	upper := strings.ToUpper(msg)
	if strings.Contains(upper, " INFO ") || strings.Contains(upper, " DEBUG ") || strings.Contains(upper, " TRACE ") || strings.Contains(upper, " VERBOSE ") {
		return ""
	}

	// Look for common error patterns with stricter anchors
	patterns := []string{
		`(?i)error[\s\:\-\|]+(.+)$`,
		`(?i)exception[\s\:\-\|]+(.+)$`,
		`(?i)warn(ing)?[\s\:\-\|]+(.+)$`,
		`(?i)failed[\s\:\-\|]+(.+)$`,
		`(?i)timeout[\s\:\-\|]+(.+)$`,
		`(?i)connection[\s\:\-\|]+(.+)$`,
		`(?i)(panic|fatal|severe|critical)[\s\:\-\|]+(.+)$`,
		`(?i)status[:\s]+([45][0-9][0-9])`, // Anchored status code
		`(?i)HTTP\s+([45][0-9][0-9])`,      // Anchored status code
	}
	
	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(msg)
		if len(matches) > 1 {
			cleanMsg := strings.TrimSpace(matches[len(matches)-1])
			// Cleanup: remove trailing punctuation common in JSON-like logs
			cleanMsg = strings.TrimRight(cleanMsg, ",;'\"")
			
			// Remove sub-second fragments (e.g. ,491 or .999)
			cleanMsg = regexp.MustCompile(`[,.]\d{3}`).ReplaceAllString(cleanMsg, "")

			// Remove thread names in brackets: [main], [pool-1-thread-4]
			cleanMsg = regexp.MustCompile(`\s*\[[^\]]+\]`).ReplaceAllString(cleanMsg, "")

			if len(cleanMsg) > 200 {
				cleanMsg = cleanMsg[:200] + "..."
			}
			if cleanMsg != "" {
				return cleanMsg
			}
		}
	}
	
	// If the line contains metadata identifiers but no error keywords, discard it
	if strings.Contains(msg, "traceId") || strings.Contains(msg, "spanId") || strings.Contains(msg, "timestamp") || 
	   strings.Contains(msg, "cvv") || strings.Contains(msg, "creditCard") {
		return ""
	}
	
	// Fallback: return first 100 chars of the line only if it seems meaningful 
	// (Contains at least one letter)
	hasLetter, _ := regexp.MatchString(`[a-zA-Z]`, msg)
	if !hasLetter {
		return ""
	}

	if len(msg) > 100 {
		return strings.TrimSpace(msg[:100]) + "..."
	}
	return strings.TrimSpace(msg)
}
