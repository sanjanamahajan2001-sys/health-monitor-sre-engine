package incident

import (
	"fmt"
	"regexp"
	"strings"

	"health-monitor/internal/flow"
)

// enrichAnalysis auto-extracts missing RCA fields from incident data
func (s *Service) enrichAnalysis(incident *Incident, analysis *IncidentAnalysis) *IncidentAnalysis {
	if analysis == nil {
		analysis = &IncidentAnalysis{}
	}

	// Auto-extract component from flow configuration
	if analysis.Component == "" {
		analysis.Component = s.extractComponentFromFlow(incident.Service)
	}

	// Auto-extract pattern from logs
	if analysis.Pattern == "" && incident.Logs != nil && len(incident.Logs.TopErrors) > 0 {
		analysis.Pattern = extractPatternFromLogs(incident.Logs)
	}

	// Auto-extract error signature
	if analysis.ErrorSignature == "" && incident.Logs != nil && len(incident.Logs.TopErrors) > 0 {
		analysis.ErrorSignature = extractErrorSignature(incident.Logs)
	}

	// Auto-extract dependency from flow
	if analysis.Dependency == "" {
		analysis.Dependency = s.extractDependencyFromFlow(incident.Service)
	}

	return analysis
}

// extractComponentFromFlow attempts to extract component name from flow configuration
func (s *Service) extractComponentFromFlow(serviceName string) string {
	result, err := flow.LoadOnce()
	if err != nil || len(result.Flows) == 0 {
		return ""
	}

	// Check if service is a known component in any flow
	for _, f := range result.Flows {
		for _, svc := range f.Services {
			if svc == serviceName {
				// Return the service as component
				return serviceName
			}
		}
	}
	return ""
}

// extractDependencyFromFlow attempts to extract dependency relationship from flow configuration
func (s *Service) extractDependencyFromFlow(serviceName string) string {
	result, err := flow.LoadOnce()
	if err != nil || len(result.Flows) == 0 {
		return ""
	}

	// Look for flows where this service appears with others
	// Simple heuristic: if service is in a flow with a database/cache, assume dependency
	dbKeywords := []string{"db", "database", "postgres", "mysql", "mongo", "redis", "cache"}

	for _, f := range result.Flows {
		hasService := false
		var dbService string

		for _, svc := range f.Services {
			if svc == serviceName {
				hasService = true
			}
			// Check if this is a database/cache service
			svcLower := strings.ToLower(svc)
			for _, keyword := range dbKeywords {
				if strings.Contains(svcLower, keyword) {
					dbService = svc
					break
				}
			}
		}

		// If we found both the service and a DB in the same flow, create dependency string
		if hasService && dbService != "" && dbService != serviceName {
			return serviceName + "->" + dbService
		}
	}

	return ""
}

// extractPatternFromLogs extracts a normalized pattern from log errors
func extractPatternFromLogs(logs *LogSummary) string {
	if len(logs.TopErrors) == 0 {
		return ""
	}

	// Use the most frequent error as the pattern
	topError := logs.TopErrors[0]

	// Use sample if available, otherwise use message
	logMessage := topError.Message
	if topError.Sample != "" {
		// Use sample if it's significantly longer or contains key pattern words
		if len(topError.Sample) > (len(topError.Message)*3)/2 || 
		   strings.Contains(strings.ToLower(topError.Sample), "mysql") ||
		   strings.Contains(strings.ToLower(topError.Sample), "redis") ||
		   strings.Contains(strings.ToLower(topError.Sample), "payment") ||
		   strings.Contains(strings.ToLower(topError.Sample), "gateway") ||
		   strings.Contains(strings.ToLower(topError.Sample), "timeout") {
			logMessage = topError.Sample
		}
	}
	return normalizePattern(logMessage)
}

// extractErrorSignature extracts key error terms from logs
func extractErrorSignature(logs *LogSummary) string {
	if len(logs.TopErrors) == 0 {
		return ""
	}

	// Extract key error terms (simplified pattern matching)
	topError := logs.TopErrors[0]
	msg := topError.Message
	
	// Use sample if available and more informative
	if topError.Sample != "" {
		if len(topError.Sample) > (len(topError.Message)*3)/2 || 
		   strings.Contains(strings.ToLower(topError.Sample), "postgres") ||
		   strings.Contains(strings.ToLower(topError.Sample), "mysql") ||
		   strings.Contains(strings.ToLower(topError.Sample), "redis") ||
		   strings.Contains(strings.ToLower(topError.Sample), "payment") ||
		   strings.Contains(strings.ToLower(topError.Sample), "gateway") ||
		   strings.Contains(strings.ToLower(topError.Sample), "timeout") {
			msg = topError.Sample
		}
	}

	// Common error patterns
	patterns := []string{
		"connection refused",
		"connection timeout",
		"timeout",
		"out of memory",
		"permission denied",
		"not found",
		"internal server error",
		"database error",
		"query timeout",
		"deadlock",
		"too many connections",
	}

	msgLower := strings.ToLower(msg)
	for _, pattern := range patterns {
		if strings.Contains(msgLower, pattern) {
			return pattern
		}
	}

	// Fallback: first few words
	words := strings.Fields(msg)
	if len(words) > 3 {
		return strings.ToLower(strings.Join(words[:3], " "))
	}
	return strings.ToLower(msg)
}

// normalizePattern removes variable parts from error messages for pattern matching
func normalizePattern(message string) string {
	msg := strings.ToLower(message)
	
	// Remove common prefixes
	msg = regexp.MustCompile(`(?i)^(error|fatal|warn|warning|info|debug|err|msg|log|trace|stdout|stderr)[\s\:\-\|]+`).ReplaceAllString(msg, "")
	
	// Remove timestamps (YYYY-MM-DD, HH:MM:SS, etc.)
	msg = regexp.MustCompile(`\d{4}-\d{2}-\d{2}`).ReplaceAllString(msg, "")
	msg = regexp.MustCompile(`\d{2}:\d{2}:\d{2}`).ReplaceAllString(msg, "")
	
	// Remove IP addresses
	msg = regexp.MustCompile(`\d+\.\d+\.\d+\.\d+`).ReplaceAllString(msg, "")
	
	// Remove UUIDs
	msg = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`).ReplaceAllString(msg, "")
	
	// Remove sub-second fragments (e.g. ,491 or .999)
	msg = regexp.MustCompile(`[,.]\d{3}`).ReplaceAllString(msg, "")

	// Remove thread names in brackets: [main], [pool-1-thread-4]
	msg = regexp.MustCompile(`\[[^\]]+\]`).ReplaceAllString(msg, "")

	// Remove Java-style class names: com.example.service.MyClass
	msg = regexp.MustCompile(`\b[a-z0-9]+\.[a-z0-9\.]+\.[A-Z][a-zA-Z0-9]+\b`).ReplaceAllString(msg, "")

	// Remove common filler words (strictly on word boundaries to avoid mangling words like 'confirmation')
	fillerWords := []string{
		"the", "a", "an", "and", "or", "but", "in", "on", "at", "to", "for", 
		"of", "with", "by", "from", "up", "about", "into", "through", "during",
		"before", "after", "above", "below", "between", "among", "under", "over",
		"due to", "because of", "as a result of", "caused by", "resulting from",
	}
	for _, word := range fillerWords {
		re := regexp.MustCompile(fmt.Sprintf(`(?i)\b%s\b`, regexp.QuoteMeta(word)))
		msg = re.ReplaceAllString(msg, " ")
	}
	
	// Remove extra whitespace and normalize
	msg = regexp.MustCompile(`\s+`).ReplaceAllString(msg, " ")
	msg = strings.TrimSpace(msg)
	
	// Truncate if too long
	if len(msg) > 50 {
		words := strings.Fields(msg)
		if len(words) > 3 {
			msg = strings.Join(words[:3], " ")
		}
	}
	
	return msg
}

// ExtractPatternFromLog extracts pattern from log message
func ExtractPatternFromLog(errorMessage string) string {
	msg := strings.ToLower(errorMessage)
	
	// Priority-based pattern matching (most specific first)
	
	// Payment gateway patterns (check BEFORE general timeout)
	if strings.Contains(msg, "payment") && strings.Contains(msg, "gateway") {
		if strings.Contains(msg, "timeout") {
			return "payment_gateway_timeout"
		}
		if strings.Contains(msg, "connection") && (strings.Contains(msg, "refused") || strings.Contains(msg, "failed")) {
			return "payment_gateway_connection_failed"
		}
		if strings.Contains(msg, "stripe") {
			if strings.Contains(msg, "api") && (strings.Contains(msg, "not responding") || strings.Contains(msg, "unavailable")) {
				return "stripe_api_outage"
			}
			return "payment_gateway_issue"
		}
		return "payment_gateway_issue"
	}
	
	// Timeout patterns
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "timed out") {
		if strings.Contains(msg, "connection") {
			return "connection_timeout"
		}
		if strings.Contains(msg, "read") {
			return "read_timeout"
		}
		if strings.Contains(msg, "write") {
			return "write_timeout"
		}
		return "general_timeout"
	}
	
	// Authentication patterns
	if strings.Contains(msg, "authentication") && strings.Contains(msg, "failed") {
		return "authentication_issue"
	}
	
	// Vault/JWKS patterns
	if strings.Contains(msg, "vault") {
		if strings.Contains(msg, "access denied") {
			return "vault_access_denied"
		}
		if strings.Contains(msg, "jwks") && (strings.Contains(msg, "refresh") || strings.Contains(msg, "expired")) {
			return "jwks_refresh_failure"
		}
		if strings.Contains(msg, "signing key") && strings.Contains(msg, "expired") {
			return "signing_key_expired"
		}
		if strings.Contains(msg, "policy") && strings.Contains(msg, "mismatch") {
			return "vault_policy_mismatch"
		}
		return "vault_issue"
	}
	
	// JWT/Token patterns
	if strings.Contains(msg, "jwt") || strings.Contains(msg, "token") {
		if strings.Contains(msg, "expired") {
			return "token_expired"
		}
		if strings.Contains(msg, "invalid") || strings.Contains(msg, "verification") {
			return "token_verification_failed"
		}
		return "jwt_token_issue"
	}
	
	// Default pattern
	return "general_error"
}
