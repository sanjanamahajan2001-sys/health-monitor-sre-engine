package logs

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"health-monitor/internal/config"
)

type ElasticBackend struct {
	Config config.Config
}

func (b ElasticBackend) Name() string {
	return "elastic"
}

func (b ElasticBackend) Correlate(req CorrelationRequest) (CorrelationResult, error) {
	cfg := b.Config
	if strings.TrimSpace(cfg.ElasticURL) == "" {
		return CorrelationResult{}, errors.New("elastic url not set")
	}
	window := strings.TrimSpace(req.Window)
	if window == "" {
		window = "10m"
	}
	if req.MinSamples <= 0 {
		req.MinSamples = defaultMinSamples
	}
	if req.MaxResults <= 0 {
		req.MaxResults = defaultMaxResults
	}
	if req.MaxLogs <= 0 {
		req.MaxLogs = defaultMaxLogs
	}

	start, end := windowRange(window)
	resolved, notes, err := resolveElasticConfig(cfg, req.Scope)
	if err != nil {
		return CorrelationResult{Query: "", Notes: notes, MinSamples: req.MinSamples}, err
	}
	query, err := buildElasticQuery(resolved, req.Scope, req.ErrorRegex, start, end, req.MaxLogs, req.MaxResults)
	if err != nil {
		return CorrelationResult{Query: "", Notes: append(notes, err.Error()), MinSamples: req.MinSamples}, err
	}

	endpoint := strings.TrimSuffix(resolved.ElasticURL, "/") + "/" + strings.TrimPrefix(resolved.ElasticIndex, "/") + "/_search"
	endpointURL, err := url.Parse(endpoint)
	if err != nil {
		return CorrelationResult{Query: endpoint, Notes: notes, MinSamples: req.MinSamples}, err
	}
	q := endpointURL.Query()
	q.Set("ignore_unavailable", "true")
	q.Set("allow_no_indices", "true")
	q.Set("track_total_hits", "true")
	endpointURL.RawQuery = q.Encode()

	body, _ := json.Marshal(query)
	httpReq, err := http.NewRequest(http.MethodPost, endpointURL.String(), bytes.NewReader(body))
	if err != nil {
		return CorrelationResult{Query: endpointURL.String(), MinSamples: req.MinSamples}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	applyElasticAuth(httpReq, resolved)

	client := &http.Client{Timeout: 6 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return CorrelationResult{Query: endpointURL.String(), MinSamples: req.MinSamples}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusBadRequest && strings.TrimSpace(req.ErrorRegex) != "" {
		_ = resp.Body.Close()
		query, err = buildElasticQuery(resolved, req.Scope, "", start, end, req.MaxLogs, req.MaxResults)
		if err != nil {
			return CorrelationResult{Query: endpointURL.String(), Notes: append(notes, err.Error()), MinSamples: req.MinSamples}, err
		}
		body, _ = json.Marshal(query)
		httpReq, err = http.NewRequest(http.MethodPost, endpointURL.String(), bytes.NewReader(body))
		if err != nil {
			return CorrelationResult{Query: endpointURL.String(), MinSamples: req.MinSamples}, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		applyElasticAuth(httpReq, resolved)
		resp, err = client.Do(httpReq)
		if err != nil {
			return CorrelationResult{Query: endpointURL.String(), Notes: notes, MinSamples: req.MinSamples}, err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			notes = append(notes, "regex filter removed (query error)")
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body := readElasticBody(resp.Body)
		if body != "" {
			return CorrelationResult{Query: endpointURL.String(), Notes: notes, MinSamples: req.MinSamples}, fmt.Errorf("elastic query failed: %s body: %s", resp.Status, body)
		}
		return CorrelationResult{Query: endpointURL.String(), Notes: notes, MinSamples: req.MinSamples}, fmt.Errorf("elastic query failed: %s", resp.Status)
	}

	var parsed struct {
		Hits struct {
			Total struct {
				Value int `json:"value"`
			} `json:"total"`
		} `json:"hits"`
		Aggregations map[string]struct {
			Buckets []struct {
				Key      string `json:"key"`
				DocCount int    `json:"doc_count"`
			} `json:"buckets"`
		} `json:"aggregations"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return CorrelationResult{Query: endpointURL.String(), MinSamples: req.MinSamples}, err
	}

	buckets := parsed.Aggregations["top_signatures"].Buckets
	merged := map[string]int{}
	for _, bucket := range buckets {
		signature := normalizeSignature(bucket.Key)
		if signature == "" {
			continue
		}
		merged[signature] += bucket.DocCount
	}
	signatures := make([]SignatureCount, 0, len(merged))
	total := 0
	for signature, count := range merged {
		signatures = append(signatures, SignatureCount{
			Signature: signature,
			Count:     count,
		})
		total += count
	}
	if total > 0 {
		for i := range signatures {
			signatures[i].Percent = (float64(signatures[i].Count) / float64(total)) * 100
		}
	}

	if total == 0 && strings.TrimSpace(req.Scope.Dependency) != "" {
		scopeNoDep := req.Scope
		scopeNoDep.Dependency = ""
		queryNoDep, err := buildElasticQuery(resolved, scopeNoDep, req.ErrorRegex, start, end, req.MaxLogs, req.MaxResults)
		if err == nil {
			bodyNoDep, _ := json.Marshal(queryNoDep)
			httpReqNoDep, err := http.NewRequest(http.MethodPost, endpointURL.String(), bytes.NewReader(bodyNoDep))
			if err == nil {
				httpReqNoDep.Header.Set("Content-Type", "application/json")
				applyElasticAuth(httpReqNoDep, resolved)
				respNoDep, err := client.Do(httpReqNoDep)
				if err == nil {
					defer respNoDep.Body.Close()
					if respNoDep.StatusCode == http.StatusBadRequest && strings.TrimSpace(req.ErrorRegex) != "" {
						queryNoDep, err = buildElasticQuery(resolved, scopeNoDep, "", start, end, req.MaxLogs, req.MaxResults)
						if err == nil {
							bodyNoDep, _ = json.Marshal(queryNoDep)
							httpReqNoDep, err = http.NewRequest(http.MethodPost, endpointURL.String(), bytes.NewReader(bodyNoDep))
							if err == nil {
								httpReqNoDep.Header.Set("Content-Type", "application/json")
								applyElasticAuth(httpReqNoDep, resolved)
								respNoDep, err = client.Do(httpReqNoDep)
								if err == nil {
									defer respNoDep.Body.Close()
									if respNoDep.StatusCode >= 200 && respNoDep.StatusCode < 300 {
										notes = append(notes, "regex filter removed (query error)")
									}
								}
							}
						}
					}
					if respNoDep.StatusCode >= 200 && respNoDep.StatusCode < 300 {
						var parsedNoDep struct {
							Hits struct {
								Total struct {
									Value int `json:"value"`
								} `json:"total"`
							} `json:"hits"`
							Aggregations map[string]struct {
								Buckets []struct {
									Key      string `json:"key"`
									DocCount int    `json:"doc_count"`
								} `json:"buckets"`
							} `json:"aggregations"`
						}
						if err := json.NewDecoder(respNoDep.Body).Decode(&parsedNoDep); err == nil {
							buckets = parsedNoDep.Aggregations["top_signatures"].Buckets
							signatures = make([]SignatureCount, 0, len(buckets))
							for _, bucket := range buckets {
								signature := normalizeSignature(bucket.Key)
								if signature == "" {
									continue
								}
								signatures = append(signatures, SignatureCount{
									Signature: signature,
									Count:     bucket.DocCount,
								})
							}
							total = parsedNoDep.Hits.Total.Value
							if total > 0 {
								for i := range signatures {
									signatures[i].Percent = (float64(signatures[i].Count) / float64(total)) * 100
								}
								notes = append(notes, "dependency filter removed (no matching logs)")
							}
						}
					}
				}
			}
		}
	}

	sort.Slice(signatures, func(i, j int) bool {
		if signatures[i].Count == signatures[j].Count {
			return signatures[i].Signature < signatures[j].Signature
		}
		return signatures[i].Count > signatures[j].Count
	})

	return CorrelationResult{
		Signatures: signatures,
		Query:      endpointURL.String(),
		Notes:      notes,
		Samples:    total,
		MinSamples: req.MinSamples,
	}, nil
}

func applyElasticAuth(req *http.Request, cfg config.Config) {
	if strings.TrimSpace(cfg.ElasticToken) != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.ElasticToken)
		return
	}
	if strings.TrimSpace(cfg.ElasticUser) != "" || strings.TrimSpace(cfg.ElasticPass) != "" {
		req.SetBasicAuth(cfg.ElasticUser, cfg.ElasticPass)
	}
}

func buildElasticQuery(cfg config.Config, scope Scope, errorRegex string, start time.Time, end time.Time, maxLogs int, maxResults int) (map[string]interface{}, error) {
	timeField := strings.TrimSpace(cfg.ElasticTimeField)
	errorField := strings.TrimSpace(cfg.ElasticErrorField)
	serviceField := strings.TrimSpace(cfg.ElasticServiceField)
	routeField := strings.TrimSpace(cfg.ElasticRouteField)
	if timeField == "" || errorField == "" || serviceField == "" {
		return nil, errors.New("elastic fields not configured")
	}

	filters := []map[string]interface{}{
		{"range": map[string]interface{}{
			timeField: map[string]interface{}{
				"gte": start.Format(time.RFC3339Nano),
				"lte": end.Format(time.RFC3339Nano),
			},
		}},
	}

	if strings.TrimSpace(scope.Service) != "" {
		filters = append(filters, map[string]interface{}{
			"term": map[string]interface{}{serviceField: scope.Service},
		})
	}
	if strings.TrimSpace(scope.Route) != "" && routeField != "" {
		filters = append(filters, map[string]interface{}{
			"term": map[string]interface{}{routeField: scope.Route},
		})
	}
	if strings.TrimSpace(scope.Dependency) != "" {
		filters = append(filters, map[string]interface{}{
			"match": map[string]interface{}{errorField: scope.Dependency},
		})
	}
	if strings.TrimSpace(errorRegex) != "" {
		errorRegex = normalizeElasticRegex(errorRegex)
		filters = append(filters, map[string]interface{}{
			"regexp": map[string]interface{}{errorField: errorRegex},
		})
	}

	aggs := map[string]interface{}{
		"top_signatures": map[string]interface{}{
			"terms": map[string]interface{}{
				"field": errorField,
				"size":  maxResults,
			},
		},
	}

	query := map[string]interface{}{
		"size": 0,
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"filter": filters,
			},
		},
		"aggs": aggs,
	}

	if maxLogs > 0 {
		query["track_total_hits"] = true
	}

	return query, nil
}

func readElasticBody(body io.ReadCloser) string {
	if body == nil {
		return ""
	}
	raw, _ := io.ReadAll(body)
	out := strings.TrimSpace(string(raw))
	if len(out) > 256 {
		return out[:256] + "..."
	}
	return out
}

func normalizeElasticRegex(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return trimmed
	}
	
	// Elasticsearch compatibility: Escape backslashes for regexp filter
	safeRegex := strings.ReplaceAll(trimmed, "\\", "\\\\")
	// Also replace \d with [0-9] for better compatibility
	safeRegex = strings.ReplaceAll(safeRegex, "\\\\d", "[0-9]")
	
	// OpenSearch/ES regexp query is full-term by default; wrap to allow substring matches.
	if strings.HasPrefix(safeRegex, "^") || strings.HasSuffix(safeRegex, "$") || strings.Contains(safeRegex, ".*") {
		return safeRegex
	}
	return ".*(" + safeRegex + ").*"
}

func resolveElasticConfig(cfg config.Config, scope Scope) (config.Config, []string, error) {
	notes := []string{}
	resolved := cfg
	if strings.TrimSpace(resolved.ElasticIndex) == "" {
		index, warn, err := detectElasticIndex(cfg)
		if err != nil {
			return resolved, notes, err
		}
		resolved.ElasticIndex = index
		if warn != "" {
			notes = append(notes, warn)
		}
	}
	if strings.TrimSpace(resolved.ElasticServiceField) == "" || strings.TrimSpace(resolved.ElasticErrorField) == "" || strings.TrimSpace(resolved.ElasticTimeField) == "" {
		fieldInfo, warn, err := detectElasticFields(cfg, resolved.ElasticIndex)
		if err != nil {
			return resolved, notes, err
		}
		if resolved.ElasticServiceField == "" {
			resolved.ElasticServiceField = fieldInfo.ServiceField
		}
		if resolved.ElasticRouteField == "" {
			resolved.ElasticRouteField = fieldInfo.RouteField
		}
		if resolved.ElasticErrorField == "" {
			resolved.ElasticErrorField = fieldInfo.ErrorField
		}
		if resolved.ElasticTimeField == "" {
			resolved.ElasticTimeField = fieldInfo.TimeField
		}
		if warn != "" {
			notes = append(notes, warn)
		}
	}
	if strings.TrimSpace(resolved.ElasticErrorField) != "" {
		normalized, note, err := normalizeElasticAggField(cfg, resolved.ElasticIndex, resolved.ElasticErrorField)
		if err != nil {
			return resolved, notes, err
		}
		if normalized != "" && normalized != resolved.ElasticErrorField {
			resolved.ElasticErrorField = normalized
			if note != "" {
				notes = append(notes, note)
			}
		}
	}
	if strings.TrimSpace(resolved.ElasticServiceField) == "" || strings.TrimSpace(resolved.ElasticErrorField) == "" || strings.TrimSpace(resolved.ElasticTimeField) == "" {
		return resolved, notes, errors.New("elastic fields not configured")
	}
	return resolved, notes, nil
}

type elasticFieldSelection struct {
	ServiceField string
	RouteField   string
	ErrorField   string
	TimeField    string
}

type elasticFieldInfo struct {
	Name       string
	FieldType  string
	HasKeyword bool
}

func detectElasticIndex(cfg config.Config) (string, string, error) {
	indices, err := fetchElasticIndices(cfg)
	if err != nil {
		return "", "", err
	}
	if len(indices) == 0 {
		return "", "", errors.New("no elastic indices found")
	}
	logLike := filterLogIndices(indices)
	if len(logLike) == 0 {
		return "*", "Auto-detect: using all indices. Set ELASTIC_INDEX for precision.", nil
	}
	if len(logLike) > 20 {
		logLike = logLike[:20]
		return strings.Join(logLike, ","), "Auto-detect: using first 20 log-like indices. Set ELASTIC_INDEX for precision.", nil
	}
	return strings.Join(logLike, ","), "Auto-detect: using log-like indices.", nil
}

func fetchElasticIndices(cfg config.Config) ([]string, error) {
	endpoint := strings.TrimSuffix(cfg.ElasticURL, "/") + "/_cat/indices?format=json"
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	applyElasticAuth(req, cfg)
	client := &http.Client{Timeout: 6 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, errors.New("elastic auth required")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("elastic indices query failed: %s", resp.Status)
	}
	var parsed []map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	indices := []string{}
	for _, item := range parsed {
		if name, ok := item["index"]; ok && strings.TrimSpace(name) != "" {
			indices = append(indices, name)
		}
	}
	return indices, nil
}

func filterLogIndices(indices []string) []string {
	out := []string{}
	for _, name := range indices {
		lower := strings.ToLower(name)
		if strings.Contains(lower, "log") || strings.Contains(lower, "filebeat") || strings.Contains(lower, "logstash") || strings.Contains(lower, "fluent") || strings.Contains(lower, "k8s") || strings.Contains(lower, "app") {
			out = append(out, name)
		}
	}
	return out
}

func detectElasticFields(cfg config.Config, index string) (elasticFieldSelection, string, error) {
	fields, err := fetchElasticMappings(cfg, index)
	if err != nil {
		return elasticFieldSelection{}, "", err
	}
	if len(fields) == 0 {
		return elasticFieldSelection{}, "", errors.New("elastic mappings empty")
	}
	selectField := func(candidates []string, matchType string) string {
		for _, candidate := range candidates {
			for _, field := range fields {
				if field.Name == candidate && (matchType == "" || field.FieldType == matchType) {
					if field.HasKeyword {
						return candidate + ".keyword"
					}
					return candidate
				}
			}
		}
		for _, candidate := range candidates {
			for _, field := range fields {
				if field.Name == candidate {
					if field.HasKeyword {
						return candidate + ".keyword"
					}
					return candidate
				}
			}
		}
		return ""
	}
	selectAggField := func(candidates []string) string {
		for _, candidate := range candidates {
			if info := findElasticField(fields, candidate); info != nil {
				if info.HasKeyword {
					return candidate + ".keyword"
				}
				if info.FieldType != "text" && info.FieldType != "match_only_text" {
					return candidate
				}
			}
		}
		return ""
	}
	timeField := selectField([]string{"@timestamp", "timestamp", "time", "ts", "event_time"}, "date")
	errorField := selectAggField([]string{"message", "log", "error", "err", "exception", "msg"})
	serviceField := selectAggField([]string{"service", "service.name", "app", "app.name", "application", "job", "component"})
	routeField := selectAggField([]string{"route", "path", "uri", "endpoint", "url", "http.route"})

	warn := ""
	if timeField == "" || errorField == "" || serviceField == "" {
		warn = "Auto-detect incomplete; set Elastic fields explicitly."
	}
	return elasticFieldSelection{
		ServiceField: serviceField,
		RouteField:   routeField,
		ErrorField:   errorField,
		TimeField:    timeField,
	}, warn, nil
}

func fetchElasticMappings(cfg config.Config, index string) ([]elasticFieldInfo, error) {
	endpoint := strings.TrimSuffix(cfg.ElasticURL, "/") + "/" + strings.TrimPrefix(index, "/") + "/_mapping"
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	applyElasticAuth(req, cfg)
	client := &http.Client{Timeout: 6 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, errors.New("elastic auth required")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("elastic mappings query failed: %s", resp.Status)
	}

	var payload map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	fields := []elasticFieldInfo{}
	for _, v := range payload {
		root, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		mappings, ok := root["mappings"].(map[string]interface{})
		if !ok {
			continue
		}
		props, ok := mappings["properties"].(map[string]interface{})
		if !ok {
			continue
		}
		walkElasticProperties("", props, &fields)
	}
	return fields, nil
}

func walkElasticProperties(prefix string, props map[string]interface{}, out *[]elasticFieldInfo) {
	for name, value := range props {
		node, ok := value.(map[string]interface{})
		if !ok {
			continue
		}
		fieldName := name
		if prefix != "" {
			fieldName = prefix + "." + name
		}
		if fieldType, ok := node["type"].(string); ok {
			info := elasticFieldInfo{Name: fieldName, FieldType: fieldType, HasKeyword: false}
			if subFields, ok := node["fields"].(map[string]interface{}); ok {
				if _, ok := subFields["keyword"]; ok {
					info.HasKeyword = true
				}
			}
			*out = append(*out, info)
		}
		if nested, ok := node["properties"].(map[string]interface{}); ok {
			walkElasticProperties(fieldName, nested, out)
		}
	}
}

func normalizeElasticAggField(cfg config.Config, index string, field string) (string, string, error) {
	fields, err := fetchElasticMappings(cfg, index)
	if err != nil {
		return field, "", err
	}
	if info := findElasticField(fields, field); info != nil {
		if info.HasKeyword {
			return field + ".keyword", "Using keyword field for aggregations.", nil
		}
		if info.FieldType == "text" || info.FieldType == "match_only_text" {
			return "", "", errors.New("elastic error field is not aggregatable; set ELASTIC_ERROR_FIELD to a keyword field")
		}
		return field, "", nil
	}
	return field, "", nil
}

func findElasticField(fields []elasticFieldInfo, name string) *elasticFieldInfo {
	for i := range fields {
		if fields[i].Name == name {
			return &fields[i]
		}
	}
	return nil
}
