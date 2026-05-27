package logs

import (
	"regexp"
	"strings"
)

var (
	isoTimestampRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}[tT ]\d{2}:\d{2}:\d{2}([.,]\d+)?(z|[+-]\d{2}:?\d{2})?\s+`)
	timePrefixRe   = regexp.MustCompile(`^\d{2}:\d{2}:\d{2}([.,]\d+)?\s+`)
	levelPrefixRe  = regexp.MustCompile(`^(?:level=)?(error|warn|warning|info|debug|trace)\b[:\s-]*`)
	levelWordRe    = regexp.MustCompile(`(?i)\b(error|warn|warning|info|debug|trace)\b`)
	reasonPairRe   = regexp.MustCompile(`(?i)\b([a-z0-9_.-]{3,})\s*:\s*([a-z0-9_.-]{3,})`)
	errorValueRe   = regexp.MustCompile(`(?i)\berror(?:\s+code)?\s*[:=]\s*([a-z0-9_.:-]{3,})`)
	exceptionRe    = regexp.MustCompile(`(?i)\bexception\s*[:=]\s*([a-z0-9_.:-]{3,})`)
	failedRe       = regexp.MustCompile(`(?i)\bfailed(?:\s+to|\s*):\s*([a-z0-9_.:-]{3,})`)
	uuidRe         = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)
	hexRe          = regexp.MustCompile(`\b[0-9a-fA-F]{8,}\b`)
	ipRe           = regexp.MustCompile(`\b\d{1,3}(\.\d{1,3}){3}\b`)
	numberRe       = regexp.MustCompile(`\b\d{3,}\b`)
	spaceRe        = regexp.MustCompile(`\s+`)
)

func normalizeSignature(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return ""
	}
	if isNoiseLog(trimmed) {
		return ""
	}
	if reason := extractReason(trimmed); reason != "" {
		return reason
	}
	out := strings.ToLower(trimmed)
	out = isoTimestampRe.ReplaceAllString(out, "")
	out = timePrefixRe.ReplaceAllString(out, "")
	out = levelPrefixRe.ReplaceAllString(out, "")
	out = uuidRe.ReplaceAllString(out, "#")
	out = hexRe.ReplaceAllString(out, "#")
	out = ipRe.ReplaceAllString(out, "ip")
	out = numberRe.ReplaceAllString(out, "#")
	out = spaceRe.ReplaceAllString(out, " ")
	out = strings.TrimSpace(out)
	if len(out) > 140 {
		out = out[:140]
	}
	return out
}

func isNoiseLog(line string) bool {
	lower := strings.ToLower(line)
	if strings.Contains(lower, "error") || strings.Contains(lower, "warn") || strings.Contains(lower, "warning") {
		return false
	}
	return levelWordRe.MatchString(line)
}

func extractReason(line string) string {
	clean := strings.TrimSpace(line)
	if clean == "" {
		return ""
	}
	lower := strings.ToLower(clean)
	lower = isoTimestampRe.ReplaceAllString(lower, "")
	lower = timePrefixRe.ReplaceAllString(lower, "")
	lower = levelPrefixRe.ReplaceAllString(lower, "")

	if match := reasonPairRe.FindStringSubmatch(lower); len(match) == 3 {
		return truncateReason(match[1] + ": " + match[2])
	}
	if match := errorValueRe.FindStringSubmatch(lower); len(match) == 2 {
		return truncateReason("error: " + match[1])
	}
	if match := exceptionRe.FindStringSubmatch(lower); len(match) == 2 {
		return truncateReason("exception: " + match[1])
	}
	if match := failedRe.FindStringSubmatch(lower); len(match) == 2 {
		return truncateReason("failed: " + match[1])
	}
	return ""
}

func truncateReason(value string) string {
	out := strings.TrimSpace(value)
	out = spaceRe.ReplaceAllString(out, " ")
	if len(out) > 140 {
		out = out[:140]
	}
	return out
}
