package logs

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"health-monitor/internal/analyse/loki"
	"health-monitor/internal/config"
)

const (
	defaultMinSamples = 10
	defaultMaxResults = 5
	defaultMaxLogs    = 200
)

type LokiBackend struct {
	Config config.Config
}

func (b LokiBackend) Name() string {
	return "loki"
}

func (b LokiBackend) Correlate(req CorrelationRequest) (CorrelationResult, error) {
	cfg := b.Config
	if cfg.LokiURL == "" {
		return CorrelationResult{}, errors.New("loki url not set")
	}
	window := strings.TrimSpace(req.Window)
	if window == "" {
		window = cfg.LokiWindow
	}
	if strings.TrimSpace(window) == "" {
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

	client := loki.Client{
		BaseURL: cfg.LokiURL,
		Token:   cfg.LokiToken,
		User:    cfg.LokiUser,
		Pass:    cfg.LokiPass,
		Timeout: 6 * time.Second,
		QPS:     cfg.LokiQPS,
	}

	serviceLabel, routeLabel, note := detectLokiLabels(&client, cfg, req.Scope.Service, req.Scope.Route)
	selector := buildSelector(serviceLabel, routeLabel, req.Scope)
	query, notes, err := buildLogQL(selector, req.ErrorRegex, req.Scope.Dependency)
	if note != "" {
		notes = append(notes, note)
	}
	if err != nil {
		return CorrelationResult{Query: selector, Notes: notes}, err
	}

	start, end := windowRange(window)
	entries, err := client.QueryRange(query, start, end, req.MaxLogs)
	if err != nil {
		var httpErr loki.HTTPError
		shouldRetry := strings.TrimSpace(req.ErrorRegex) != ""
		if errors.As(err, &httpErr) && httpErr.StatusCode == 400 {
			shouldRetry = true
		}
		errText := strings.ToLower(err.Error())
		if strings.Contains(errText, "parse error") || strings.Contains(errText, "malformed") || strings.Contains(errText, "regexp") {
			shouldRetry = true
		}
		if shouldRetry {
			query, notes, _ = buildLogQL(selector, "", req.Scope.Dependency)
			entries, err = client.QueryRange(query, start, end, req.MaxLogs)
			if err == nil {
				notes = append(notes, "regex filter removed (query parse error)")
			}
		}
		if err != nil {
			return CorrelationResult{Query: query, Notes: notes}, err
		}
	}
	if len(entries) == 0 && strings.TrimSpace(req.Scope.Dependency) != "" {
		fallbackQuery, _, _ := buildLogQL(selector, req.ErrorRegex, "")
		fallbackEntries, fallbackErr := client.QueryRange(fallbackQuery, start, end, req.MaxLogs)
		if fallbackErr == nil && len(fallbackEntries) > 0 {
			entries = fallbackEntries
			query = fallbackQuery
			notes = dropDependencyFilterNote(notes)
			notes = append(notes, "dependency filter removed (no matching logs)")
		}
	}

	signatures := groupSignatures(entries)
	total := 0
	for _, sig := range signatures {
		total += sig.Count
	}

	if total > 0 {
		for i := range signatures {
			signatures[i].Percent = (float64(signatures[i].Count) / float64(total)) * 100
		}
	}

	if len(signatures) > req.MaxResults {
		signatures = signatures[:req.MaxResults]
	}

	return CorrelationResult{
		Signatures: signatures,
		Query:      query,
		Notes:      notes,
		Samples:    total,
		MinSamples: req.MinSamples,
	}, nil
}

func dropDependencyFilterNote(notes []string) []string {
	if len(notes) == 0 {
		return notes
	}
	filtered := make([]string, 0, len(notes))
	for _, note := range notes {
		if strings.HasPrefix(note, "dependency filter:") {
			continue
		}
		filtered = append(filtered, note)
	}
	return filtered
}

func groupSignatures(entries []loki.LogEntry) []SignatureCount {
	counts := map[string]int{}
	for _, entry := range entries {
		signature := normalizeSignature(entry.Line)
		if signature == "" {
			continue
		}
		counts[signature]++
	}
	out := make([]SignatureCount, 0, len(counts))
	for signature, count := range counts {
		out = append(out, SignatureCount{Signature: signature, Count: count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			return out[i].Signature < out[j].Signature
		}
		return out[i].Count > out[j].Count
	})
	return out
}

func detectLokiLabels(client *loki.Client, cfg config.Config, service string, route string) (string, string, string) {
	if client == nil {
		return cfg.LokiServiceLabel, cfg.LokiRouteLabel, ""
	}
	if cfg.LokiServiceLabel != "" || cfg.LokiRouteLabel != "" {
		return cfg.LokiServiceLabel, cfg.LokiRouteLabel, ""
	}
	labels, err := client.Labels()
	if err != nil || len(labels) == 0 {
		return "", "", "Loki label auto-detect failed."
	}
	labelSet := map[string]struct{}{}
	for _, l := range labels {
		labelSet[l] = struct{}{}
	}
	serviceCandidates := []string{"service", "app", "application", "job", "svc", "service_name", "container"}
	routeCandidates := []string{"endpoint", "route", "path", "uri", "url", "http_route"}
	
	var serviceLabels []string
	routeLabel := ""
	noteParts := []string{}
	for _, candidate := range serviceCandidates {
		if _, ok := labelSet[candidate]; ok {
			serviceLabels = append(serviceLabels, candidate)
			if len(serviceLabels) >= 3 {
				break
			}
		}
	}
	for _, candidate := range routeCandidates {
		if _, ok := labelSet[candidate]; ok {
			routeLabel = candidate
			break
		}
	}
	
	serviceLabel := strings.Join(serviceLabels, ",")
	if serviceLabel == "" && routeLabel == "" {
		return "", "", "No service/route labels detected in Loki."
	}
	if serviceLabel != "" {
		noteParts = append(noteParts, "service labels: "+serviceLabel)
	}
	if routeLabel != "" {
		noteParts = append(noteParts, "route label: "+routeLabel)
	}
	note := ""
	if len(noteParts) > 0 {
		note = "Auto-detect: " + strings.Join(noteParts, " • ")
	}
	return serviceLabel, routeLabel, note
}


func buildSelector(serviceLabel string, routeLabel string, scope Scope) string {
	serviceLabels := strings.Split(serviceLabel, ",")
	if len(serviceLabels) == 0 || (len(serviceLabels) == 1 && serviceLabels[0] == "") {
		serviceLabels = []string{""}
	}

	var results []string
	for _, sLabel := range serviceLabels {
		var parts []string
		if sLabel != "" && strings.TrimSpace(scope.Service) != "" {
			parts = append(parts, fmt.Sprintf(`%s="%s"`, sLabel, scope.Service))
		}
		if routeLabel != "" && strings.TrimSpace(scope.Route) != "" {
			parts = append(parts, fmt.Sprintf(`%s="%s"`, routeLabel, scope.Route))
		}
		
		if len(parts) == 0 {
			if sLabel != "" {
				parts = append(parts, fmt.Sprintf(`%s=~".+"`, sLabel))
			} else if routeLabel != "" {
				parts = append(parts, fmt.Sprintf(`%s=~".+"`, routeLabel))
			}
		}

		if len(parts) > 0 {
			results = append(results, "{"+strings.Join(parts, ", ")+"}")
		}
	}

	if len(results) == 0 {
		return "{}"
	}
	return strings.Join(results, " or ")
}


func buildLogQL(selector string, errorRegex string, dependency string) (string, []string, error) {
	notes := []string{}
	filters := ""
	if strings.TrimSpace(dependency) != "" {
		filters = filters + ` |~ "` + escapeLogQLString(escapeRegexLiteral(dependency)) + `"`
		notes = append(notes, "dependency filter: "+dependency)
	}
	if strings.TrimSpace(errorRegex) != "" {
		normalized := normalizeLokiRegex(errorRegex)
		safeRegex, normalizedFlag := sanitizeLokiRegex(normalized)
		if normalizedFlag {
			notes = append(notes, `regex normalized (\d -> [0-9])`)
		}
		if err := validateRegex(safeRegex); err != nil {
			notes = append(notes, "regex ignored: "+err.Error())
		} else {
			filters = filters + ` |~ "` + escapeLogQLString(safeRegex) + `"`
		}
	}

	if !strings.Contains(selector, " or ") {
		return selector + filters, notes, nil
	}

	// For OR selectors, we must wrap each branch: (sel1 | filter) or (sel2 | filter)
	branches := strings.Split(selector, " or ")
	var finalBlocks []string
	for _, b := range branches {
		finalBlocks = append(finalBlocks, "("+strings.TrimSpace(b)+filters+")")
	}
	return strings.Join(finalBlocks, " or "), notes, nil
}


func windowRange(window string) (time.Time, time.Time) {
	dur, err := time.ParseDuration(window)
	if err != nil || dur <= 0 {
		dur = 10 * time.Minute
	}
	end := time.Now()
	start := end.Add(-dur)
	return start, end
}

func normalizeLokiRegex(value string) string {
	if strings.TrimSpace(value) == "" {
		return value
	}
	return strings.ReplaceAll(value, "\\\\", "\\")
}

func sanitizeLokiRegex(value string) (string, bool) {
	if strings.TrimSpace(value) == "" {
		return value, false
	}
	sanitized := strings.ReplaceAll(value, `\d`, `[0-9]`)
	if sanitized != value {
		return sanitized, true
	}
	return value, false
}

func validateRegex(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	if len(trimmed) > 256 {
		return errors.New("too long")
	}
	if _, err := regexp.Compile(trimmed); err != nil {
		return err
	}
	return nil
}

func escapeRegexLiteral(value string) string {
	return regexp.QuoteMeta(value)
}

func escapeLogQLString(value string) string {
	out := strings.ReplaceAll(value, "\\", "\\\\")
	out = strings.ReplaceAll(out, "\"", "\\\"")
	return out
}
