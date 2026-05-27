package traces

import (
	"regexp"
	"strings"
)

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
