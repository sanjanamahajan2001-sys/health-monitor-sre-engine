package logs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"health-monitor/internal/analyse/loki"
	"health-monitor/internal/metrics"
)

// ElasticsearchClient implements log fetching for Elasticsearch
type ElasticsearchClient struct {
	baseURL      string
	username     string
	password     string
	token        string
	index        string
	serviceField string
	errorField   string
	timeField    string
	errorRegex   *regexp.Regexp
	client       *http.Client
}

// NewElasticsearchClient creates a new Elasticsearch client
func NewElasticsearchClient(baseURL, username, password, token, index, serviceField, errorField, timeField, errorRegex string) (*ElasticsearchClient, error) {
	// Default error patterns
	if errorRegex == "" {
		errorRegex = `(?i)(error|exception|fail|timeout|refused|deadlock|warn|warning|critical|severe|fatal|panic|status\s*[45][0-9][0-9]|HTTP\s*[45][0-9][0-9])`
	}

	// Elasticsearch compatibility: Escape backslashes for regexp filter
	safeRegex := strings.ReplaceAll(errorRegex, "\\", "\\\\")
	// Also replace \d with [0-9] for better compatibility
	safeRegex = strings.ReplaceAll(safeRegex, "\\\\d", "[0-9]")
	
	regex, err := regexp.Compile(safeRegex)
	if err != nil {
		return nil, fmt.Errorf("invalid error regex: %w", err)
	}

	return &ElasticsearchClient{
		baseURL:      strings.TrimSuffix(baseURL, "/"),
		username:     username,
		password:     password,
		token:        token,
		index:        index,
		serviceField: serviceField,
		errorField:   errorField,
		timeField:    timeField,
		errorRegex:   regex,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// TopErrors returns the top N error messages for a service in a time window
func (e *ElasticsearchClient) TopErrors(ctx context.Context, service string, from, to time.Time) ([]ErrorStat, error) {
	queryStart := time.Now()
	// Build Elasticsearch query
	query := e.buildQuery(service, from, to)
	
	// Build query URL
	index := e.index
	if index == "" {
		index = "*" // Search all indices if none specified
	}
	queryURL := fmt.Sprintf("%s/%s/_search", e.baseURL, index)
	
	// Marshal query
	queryBody, err := json.Marshal(query)
	if err != nil {
		metrics.GetQueryMetrics().RecordElasticQuery(time.Since(queryStart), false)
		return nil, fmt.Errorf("failed to marshal query: %w", err)
	}
	
	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", queryURL, bytes.NewBuffer(queryBody))
	if err != nil {
		metrics.GetQueryMetrics().RecordElasticQuery(time.Since(queryStart), false)
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	req.Header.Set("Content-Type", "application/json")
	
	// Set authentication
	e.setAuth(req)
	
	// Execute request
	resp, err := e.client.Do(req)
	if err != nil {
		metrics.GetQueryMetrics().RecordElasticQuery(time.Since(queryStart), false)
		return nil, fmt.Errorf("failed to query Elasticsearch: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		metrics.GetQueryMetrics().RecordElasticQuery(time.Since(queryStart), false)
		return nil, fmt.Errorf("Elasticsearch query failed with status %d", resp.StatusCode)
	}
	
	// Parse response
	var esResp struct {
		Aggregations struct {
			ErrorMessages struct {
				Buckets []struct {
					Key      string `json:"key"`
					DocCount int    `json:"doc_count"`
					Samples  struct {
						Hits struct {
							Hits []struct {
								Source map[string]interface{} `json:"_source"`
							} `json:"hits"`
						} `json:"hits"`
					} `json:"hits"`
				} `json:"buckets"`
			} `json:"error_messages"`
		} `json:"aggregations"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&esResp); err != nil {
		metrics.GetQueryMetrics().RecordElasticQuery(time.Since(queryStart), false)
		return nil, fmt.Errorf("failed to parse Elasticsearch response: %w", err)
	}
	
	// Aggregate results (Simplified version of what was in the previous state)
	var stats []ErrorStat
	for _, bucket := range esResp.Aggregations.ErrorMessages.Buckets {
		msg := bucket.Key
		if len(bucket.Samples.Hits.Hits) > 0 {
			hit := bucket.Samples.Hits.Hits[0].Source
			if m, ok := hit["message"].(string); ok && m != "" {
				msg = m
			}
		}
		stats = append(stats, ErrorStat{
			Message: msg,
			Count:   bucket.DocCount,
		})
	}
	
	metrics.GetQueryMetrics().RecordElasticQuery(time.Since(queryStart), true)
	return stats, nil
}

// QueryLogs returns raw log entries for a service in a time window
func (e *ElasticsearchClient) QueryLogs(ctx context.Context, service, route string, from, to time.Time, limit int) ([]loki.LogEntry, error) {
	queryStart := time.Now()
	
	// Build query
	query := e.buildRawQuery(service, route, from, to, limit)
	
	index := e.index
	if index == "" {
		index = "*"
	}
	queryURL := fmt.Sprintf("%s/%s/_search", e.baseURL, index)
	
	queryBody, err := json.Marshal(query)
	if err != nil {
		metrics.GetQueryMetrics().RecordElasticQuery(time.Since(queryStart), false)
		return nil, err
	}
	
	req, err := http.NewRequestWithContext(ctx, "POST", queryURL, bytes.NewBuffer(queryBody))
	if err != nil {
		metrics.GetQueryMetrics().RecordElasticQuery(time.Since(queryStart), false)
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	e.setAuth(req)
	
	resp, err := e.client.Do(req)
	if err != nil {
		metrics.GetQueryMetrics().RecordElasticQuery(time.Since(queryStart), false)
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		metrics.GetQueryMetrics().RecordElasticQuery(time.Since(queryStart), false)
		return nil, fmt.Errorf("Elasticsearch query failed: %d", resp.StatusCode)
	}
	
	var esResp struct {
		Hits struct {
			Hits []struct {
				Source map[string]interface{} `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&esResp); err != nil {
		metrics.GetQueryMetrics().RecordElasticQuery(time.Since(queryStart), false)
		return nil, err
	}
	
	var entries []loki.LogEntry
	timeField := e.timeField
	if timeField == "" {
		timeField = "@timestamp"
	}
	
	for _, hit := range esResp.Hits.Hits {
		line := ""
		msgFields := []string{"message", "msg", "error.message", "error", "err"}
		for _, field := range msgFields {
			if val, ok := hit.Source[field].(string); ok && val != "" {
				line = val
				break
			}
		}
		
		if line == "" {
			if b, err := json.Marshal(hit.Source); err == nil {
				line = string(b)
			}
		}
		
		ts := time.Now()
		if tsStr, ok := hit.Source[timeField].(string); ok {
			if t, err := time.Parse(time.RFC3339, tsStr); err == nil {
				ts = t
			}
		}
		
		entries = append(entries, loki.LogEntry{
			Timestamp: ts,
			Line:      line,
		})
	}
	
	metrics.GetQueryMetrics().RecordElasticQuery(time.Since(queryStart), true)
	return entries, nil
}

func (e *ElasticsearchClient) buildRawQuery(service, route string, from, to time.Time, limit int) map[string]interface{} {
	timeField := e.timeField
	if timeField == "" {
		timeField = "@timestamp"
	}
	
	filters := []interface{}{
		map[string]interface{}{
			"range": map[string]interface{}{
				timeField: map[string]interface{}{
					"gte": from.Format(time.RFC3339),
					"lte": to.Format(time.RFC3339),
				},
			},
		},
	}
	
	if service != "" {
		serviceField := e.serviceField
		if serviceField == "" {
			serviceField = "service.name"
		}
		filters = append(filters, map[string]interface{}{
			"term": map[string]interface{}{serviceField: service},
		})
	}
	
	if route != "" {
		routeField := "http.route"
		filters = append(filters, map[string]interface{}{
			"term": map[string]interface{}{routeField: route},
		})
	}
	
	return map[string]interface{}{
		"size": limit,
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"filter": filters,
			},
		},
		"sort": []interface{}{
			map[string]interface{}{
				timeField: map[string]interface{}{"order": "desc"},
			},
		},
	}
}

func (e *ElasticsearchClient) buildQuery(service string, from, to time.Time) map[string]interface{} {
	timeField := e.timeField
	if timeField == "" {
		timeField = "@timestamp"
	}
	
	serviceField := e.serviceField
	if serviceField == "" {
		serviceField = "service.name"
	}
	
	errorField := e.errorField
	if errorField == "" {
		errorField = "message"
	}

	return map[string]interface{}{
		"size": 0,
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"filter": []interface{}{
					map[string]interface{}{
						"range": map[string]interface{}{
							timeField: map[string]interface{}{
								"gte": from.Format(time.RFC3339),
								"lte": to.Format(time.RFC3339),
							},
						},
					},
					map[string]interface{}{
						"term": map[string]interface{}{serviceField: service},
					},
					map[string]interface{}{
						"regexp": map[string]interface{}{
							errorField: e.errorRegex.String(),
						},
					},
				},
			},
		},
		"aggs": map[string]interface{}{
			"error_messages": map[string]interface{}{
				"terms": map[string]interface{}{
					"field": errorField + ".keyword",
					"size":  5,
				},
				"aggs": map[string]interface{}{
					"samples": map[string]interface{}{
						"top_hits": map[string]interface{}{
							"size": 1,
							"sort": []interface{}{
								map[string]interface{}{
									timeField: map[string]interface{}{"order": "desc"},
								},
							},
						},
					},
				},
			},
		},
	}
}

func (e *ElasticsearchClient) setAuth(req *http.Request) {
	if e.token != "" {
		req.Header.Set("Authorization", "Bearer "+e.token)
	} else if e.username != "" && e.password != "" {
		req.SetBasicAuth(e.username, e.password)
	}
}
