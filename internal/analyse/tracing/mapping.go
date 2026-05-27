package tracing

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"health-monitor/pkg/model"
)

type Selection struct {
	Service string
	Note    string
}

func ResolveTraceService(cfg TraceConfig, latency *model.APILatency, apm *model.DependencyAnalysis) Selection {
	noteParts := []string{}
	mapper := parseServiceMap(cfg.TraceServiceMap)
	if cfg.TraceDefaultService != "" {
		service := mapService(mapper, cfg.TraceDefaultService)
		if service != cfg.TraceDefaultService {
			noteParts = append(noteParts, fmt.Sprintf("mapped default service: %s -> %s", cfg.TraceDefaultService, service))
		}
		return Selection{Service: service, Note: joinNotes(noteParts)}
	}

	if latency != nil && strings.TrimSpace(latency.Service) != "" {
		service := mapService(mapper, latency.Service)
		if service != latency.Service {
			noteParts = append(noteParts, fmt.Sprintf("mapped api service: %s -> %s", latency.Service, service))
		}
		return Selection{Service: service, Note: joinNotes(noteParts)}
	}

	if apm != nil && len(apm.Edges) > 0 {
		best := pickLikelyDependency(apm.Edges)
		if strings.TrimSpace(best) != "" {
			service := mapService(mapper, best)
			if service != best {
				noteParts = append(noteParts, fmt.Sprintf("mapped dependency: %s -> %s", best, service))
			} else {
				noteParts = append(noteParts, "using top dependency edge destination")
			}
			return Selection{Service: service, Note: joinNotes(noteParts)}
		}
	}

	return Selection{Service: "", Note: joinNotes(noteParts)}
}

func mapService(mapping map[string]string, name string) string {
	if mapping == nil || strings.TrimSpace(name) == "" {
		return name
	}
	if mapped, ok := mapping[name]; ok && strings.TrimSpace(mapped) != "" {
		return mapped
	}
	normalized := normalizeServiceName(name)
	for k, v := range mapping {
		if normalizeServiceName(k) == normalized && strings.TrimSpace(v) != "" {
			return v
		}
	}
	return name
}

func parseServiceMap(value string) map[string]string {
	out := map[string]string{}
	parts := strings.Split(value, ",")
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		// Support both key=value and key:value
		kv := strings.SplitN(trimmed, "=", 2)
		if len(kv) != 2 {
			kv = strings.SplitN(trimmed, ":", 2)
		}
		if len(kv) != 2 {
			continue
		}
		left := strings.TrimSpace(kv[0])
		right := strings.TrimSpace(kv[1])
		if left == "" || right == "" {
			continue
		}
		out[left] = right
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeServiceName(value string) string {
	re := regexp.MustCompile(`[^a-zA-Z0-9]+`)
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = re.ReplaceAllString(normalized, "-")
	return strings.Trim(normalized, "-")
}

func pickLikelyDependency(edges []model.DependencyEdge) string {
	type scoredEdge struct {
		dest  string
		score float64
	}
	var scored []scoredEdge
	for _, e := range edges {
		if strings.TrimSpace(e.Destination) == "" {
			continue
		}
		score := e.P95 + e.DeltaP95
		if e.ErrorRate > 0 {
			score += e.ErrorRate
		}
		scored = append(scored, scoredEdge{dest: e.Destination, score: score})
	}
	if len(scored) == 0 {
		return ""
	}
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})
	return scored[0].dest
}

func joinNotes(notes []string) string {
	if len(notes) == 0 {
		return ""
	}
	return strings.Join(notes, " • ")
}
