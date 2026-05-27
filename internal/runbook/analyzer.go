package runbook

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"health-monitor/internal/config"
	"health-monitor/internal/incident"
)

// PatternAnalyzer extends existing pattern detection for runbook generation
type PatternAnalyzer struct {
	incidentStore *incident.Store
	templates     map[string]*RunbookTemplate
	config        config.RunbookConfig
}

// NewPatternAnalyzer creates a new pattern analyzer
func NewPatternAnalyzer(store *incident.Store, config config.RunbookConfig) *PatternAnalyzer {
	return &PatternAnalyzer{
		incidentStore: store,
		templates:     make(map[string]*RunbookTemplate),
		config:        config,
	}
}

// AnalyzeIncidentPatterns analyzes patterns from incident history
func (pa *PatternAnalyzer) AnalyzeIncidentPatterns(service string, days int) ([]*PatternAnalysis, error) {
	// Load incidents from the store
	incidents, _, err := pa.incidentStore.List() // Get all incidents
	if err != nil {
		return nil, fmt.Errorf("failed to load incidents: %w", err)
	}

	// Filter incidents by date and analysis
	cutoff := time.Now().AddDate(0, 0, -days)
	var filteredIncidents []incident.Incident
	for _, inc := range incidents {
		if inc.CreatedAt.After(cutoff) && inc.Analysis != nil {
			// Skip disabled services
			if pa.contains(pa.config.DisabledServices, inc.Service) {
				continue
			}
			// Only include enabled categories
			if len(pa.config.EnabledCategories) > 0 && !pa.contains(pa.config.EnabledCategories, inc.Analysis.Category) {
				continue
			}
			filteredIncidents = append(filteredIncidents, inc)
		}
	}

	// Group incidents by pattern
	patternGroups := make(map[string]*PatternAnalysis)
	for _, inc := range filteredIncidents {
		pattern := pa.normalizePatternForAnalysis(inc.Analysis.Pattern, inc.Analysis.ErrorSignature)
		
		if pattern == "" {
			continue
		}

		if _, exists := patternGroups[pattern]; !exists {
			patternGroups[pattern] = &PatternAnalysis{
				Pattern:     pattern,
				Service:     inc.Service,
				Component:   inc.Analysis.Component,
				Category:    inc.Analysis.Category,
				Frequency:   0,
				IncidentIDs: []string{},
				Tags:        []string{},
				Metadata:    make(map[string]string),
			}
		}

		group := patternGroups[pattern]
		group.Frequency++
		group.IncidentIDs = append(group.IncidentIDs, inc.ID)
		
		if inc.CreatedAt.After(group.LastSeen) {
			group.LastSeen = inc.CreatedAt
		}

		// Add tags from incident
		if inc.Analysis.Component != "" {
			group.Tags = pa.addUnique(group.Tags, inc.Analysis.Component)
		}
		if inc.Analysis.Category != "" {
			group.Tags = pa.addUnique(group.Tags, inc.Analysis.Category)
		}
	}

	// Convert to slice and calculate confidence
	var analyses []*PatternAnalysis
	for _, group := range patternGroups {
		if group.Frequency >= 2 { // Only include patterns that appear at least twice
			group.Confidence = pa.calculateConfidence(group)
			group.SuggestedSteps = pa.generateSuggestedSteps(group)
			group.RelatedMetrics = pa.generateMetricQueries(group)
			group.RelatedLogs = pa.generateLogQueries(group)
			analyses = append(analyses, group)
		}
	}

	// Sort by frequency and confidence
	sort.Slice(analyses, func(i, j int) bool {
		if analyses[i].Frequency == analyses[j].Frequency {
			return analyses[i].Confidence > analyses[j].Confidence
		}
		return analyses[i].Frequency > analyses[j].Frequency
	})

	return analyses, nil
}

// AnalyzeIncident analyzes a specific incident for runbook suggestions
func (pa *PatternAnalyzer) AnalyzeIncident(incidentID string) (*PatternAnalysis, error) {
	inc, err := pa.incidentStore.Load(incidentID)
	if err != nil {
		return nil, fmt.Errorf("failed to load incident %s: %w", incidentID, err)
	}

	// First try to get pattern from analysis data, then from metadata
	var pattern string
	var component, category string
	var confidence float64 = 0.5 // Default confidence

	// Try analysis data first (for resolved incidents)
	if inc.Analysis != nil {
		pattern = pa.normalizePatternForAnalysis(inc.Analysis.Pattern, inc.Analysis.ErrorSignature)
		component = inc.Analysis.Component
		category = inc.Analysis.Category
		// For resolved incidents, use high confidence since pattern was explicitly provided during resolution
		if pattern != "" && pattern != "general_error" {
			confidence = 0.95 // High confidence for user-specified patterns during resolution
		}
	}
	
	// If no analysis data, try metadata (for active incidents with runbook suggestions)
	if pattern == "" && inc.Metadata != nil {
		if metadataPattern := inc.Metadata["runbook_pattern"]; metadataPattern != "" {
			pattern = pa.normalizePatternForAnalysis(metadataPattern, "")
			// Extract confidence from metadata if available
			if confStr := inc.Metadata["runbook_confidence"]; confStr != "" {
				// Parse confidence from string like "85%"
				if strings.HasSuffix(confStr, "%") {
					if confVal, err := strconv.ParseFloat(strings.TrimSuffix(confStr, "%"), 64); err == nil {
						confidence = confVal / 100.0
					}
				}
			}
		}
	}
	
	// If still no pattern or pattern is too generic, extract from logs directly
	if pattern == "" || pattern == "general_error" {
		if inc.Logs != nil && len(inc.Logs.TopErrors) > 0 {
			// Use the sample field if available, otherwise use message
			topError := inc.Logs.TopErrors[0].Message
			if inc.Logs.TopErrors[0].Sample != "" {
				sample := inc.Logs.TopErrors[0].Sample
				// Use sample if it's significantly longer or contains key pattern words
				if len(sample) > (len(topError)*3)/2 || 
				   strings.Contains(strings.ToLower(sample), "payment") ||
				   strings.Contains(strings.ToLower(sample), "gateway") ||
				   strings.Contains(strings.ToLower(sample), "timeout") {
					topError = sample
				}
			}
			pattern = pa.extractPatternFromLog(topError)
			// Use higher confidence for direct log extraction
			confidence = 0.85
		}
	}
	
	if pattern == "" {
		return nil, fmt.Errorf("no pattern found in incident %s", incidentID)
	}

	// NOW SEARCH FOR ALL INCIDENTS WITH THE SAME PATTERN
	allIncidents, _, err := pa.incidentStore.List()
	if err != nil {
		return nil, fmt.Errorf("failed to list incidents: %w", err)
	}

	var relatedIncidents []string
	var frequency = 0
	var lastSeen time.Time

	for _, incident := range allIncidents {
		var incidentPattern string
		
		// Check resolved incidents
		if incident.Analysis != nil && incident.Analysis.Pattern != "" {
			incidentPattern = pa.normalizePatternForAnalysis(incident.Analysis.Pattern, incident.Analysis.ErrorSignature)
		}
		
		// Check metadata for active incidents
		if incidentPattern == "" && incident.Metadata != nil {
			if metadataPattern := incident.Metadata["runbook_pattern"]; metadataPattern != "" {
				incidentPattern = pa.normalizePatternForAnalysis(metadataPattern, "")
			}
		}
		
		// If still no pattern, try extracting from logs
		if incidentPattern == "" && incident.Logs != nil && len(incident.Logs.TopErrors) > 0 {
			topError := incident.Logs.TopErrors[0].Message
			if incident.Logs.TopErrors[0].Sample != "" {
				sample := incident.Logs.TopErrors[0].Sample
				if len(sample) > (len(topError)*3)/2 || 
				   strings.Contains(strings.ToLower(sample), "payment") ||
				   strings.Contains(strings.ToLower(sample), "gateway") ||
				   strings.Contains(strings.ToLower(sample), "timeout") {
					topError = sample
				}
			}
			incidentPattern = pa.extractPatternFromLog(topError)
		}
		
		// If this incident matches our pattern, add it to related incidents
		if incidentPattern == pattern {
			relatedIncidents = append(relatedIncidents, incident.ID)
			frequency++
			if incident.CreatedAt.After(lastSeen) {
				lastSeen = incident.CreatedAt
			}
		}
	}

	analysis := &PatternAnalysis{
		Pattern:     pattern,
		Service:     inc.Service,
		Component:   component,
		Category:    category,
		Dependency:  "", // Will be set below
		Frequency:   frequency,
		LastSeen:    lastSeen,
		IncidentIDs: relatedIncidents,
		Confidence:  confidence,
		Tags:        []string{},
		Metadata:    make(map[string]string),
	}
	
	// Set dependency from analysis if available
	if inc.Analysis != nil && inc.Analysis.Dependency != "" {
		analysis.Dependency = inc.Analysis.Dependency
	}
	
	// Add RCA data to metadata for runbook generation
	if inc.Analysis != nil {
		if inc.Analysis.RootCause != "" {
			analysis.Metadata["root_cause"] = inc.Analysis.RootCause
		}
		if inc.Analysis.FixSummary != "" {
			analysis.Metadata["fix"] = inc.Analysis.FixSummary
		}
		if inc.Analysis.Prevention != "" {
			analysis.Metadata["prevention"] = inc.Analysis.Prevention
		}
		if inc.Analysis.FailureType != "" {
			analysis.Metadata["failure_type"] = inc.Analysis.FailureType
		}
	}

	// Add tags
	if component != "" {
		analysis.Tags = append(analysis.Tags, component)
	}
	if category != "" {
		analysis.Tags = append(analysis.Tags, category)
	}

	// Generate suggestions
	analysis.SuggestedSteps = pa.generateSuggestedSteps(analysis)
	analysis.RelatedMetrics = pa.generateMetricQueries(analysis)
	analysis.RelatedLogs = pa.generateLogQueries(analysis)

	return analysis, nil
}

// extractPatternFromLog extracts pattern from log message
func (pa *PatternAnalyzer) extractPatternFromLog(errorMessage string) string {
	return incident.ExtractPatternFromLog(errorMessage)
}

// normalizePatternForAnalysis normalizes patterns for better grouping
func (pa *PatternAnalyzer) normalizePatternForAnalysis(pattern, errorSignature string) string {
	// Use the provided pattern if it's a specific pattern (not empty and not generic)
	// Only fall back to error signature if pattern is empty or too generic
	if pattern != "" && pattern != "general_error" && pattern != "unknown" {
		// Keep the user-specified pattern
		return pattern
	}
	
	// Fall back to error signature if pattern is empty or generic
	if errorSignature != "" {
		pattern = errorSignature
	}
	
	if pattern == "" {
		return ""
	}

	// Normalize the pattern
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	
	// Remove common variations
	pattern = regexp.MustCompile(`\d+`).ReplaceAllString(pattern, "N") // Numbers
	pattern = regexp.MustCompile(`[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}`).ReplaceAllString(pattern, "UUID") // UUIDs
	pattern = regexp.MustCompile(`\d+\.\d+\.\d+\.\d+`).ReplaceAllString(pattern, "IP") // IP addresses
	
	// Map similar patterns to canonical forms
	patternMappings := map[string]string{
		"connection refused":           "connection_refused",
		"connection timeout":           "connection_timeout", 
		"query timeout":               "query_timeout",
		"postgres timeout":            "query_timeout",
		"database timeout":            "query_timeout",
		"out of memory":               "out_of_memory",
		"oom":                         "out_of_memory",
		"permission denied":           "permission_denied",
		"access denied":               "permission_denied",
		"not found":                   "not_found",
		"404 not found":               "not_found",
		"internal server error":       "internal_server_error",
		"500 internal server error":   "internal_server_error",
		"database error":              "database_error",
		"deadlock":                    "deadlock",
		"too many connections":        "too_many_connections",
		"connection pool":             "connection_pool",
		"pool exhausted":              "connection_pool",
	}
	
	if canonical, exists := patternMappings[pattern]; exists {
		return canonical
	}
	
	return pattern
}

// calculateConfidence calculates confidence score for a pattern
func (pa *PatternAnalyzer) calculateConfidence(analysis *PatternAnalysis) float64 {
	confidence := 0.0
	
	// Base confidence from frequency
	freqScore := float64(analysis.Frequency) / 10.0
	if freqScore > 1.0 {
		freqScore = 1.0
	}
	confidence += freqScore * 0.4
	
	// Recency score (more recent = higher confidence)
	daysSinceLast := time.Since(analysis.LastSeen).Hours() / 24
	recencyScore := 1.0 - (daysSinceLast / 90.0) // 90-day window
	if recencyScore < 0 {
		recencyScore = 0
	}
	confidence += recencyScore * 0.3
	
	// Component specificity score
	if analysis.Component != "" {
		confidence += 0.2
	}
	
	// Category specificity score
	if analysis.Category != "" {
		confidence += 0.1
	}
	
	if confidence > 1.0 {
		confidence = 1.0
	}
	
	return confidence
}

// generateSuggestedSteps generates troubleshooting steps based on pattern and incident data
func (pa *PatternAnalyzer) generateSuggestedSteps(analysis *PatternAnalysis) []RunbookStep {
	steps := []RunbookStep{}
	
	// Generate dynamic steps based on pattern analysis
	steps = append(steps, pa.generateInvestigationSteps(analysis)...)
	steps = append(steps, pa.generateDiagnosticSteps(analysis)...)
	steps = append(steps, pa.generateResolutionSteps(analysis)...)
	steps = pa.generateCategorySpecificSteps(analysis, steps)
	
	// Fix step ordering by renumbering sequentially
	for i := range steps {
		steps[i].Order = i + 1
	}
	
	return steps
}

// generateInvestigationSteps creates initial investigation steps
func (pa *PatternAnalyzer) generateInvestigationSteps(analysis *PatternAnalysis) []RunbookStep {
	steps := []RunbookStep{}
	
	// Step 1: Verify service status
	steps = append(steps, RunbookStep{
		Order:       len(steps) + 1,
		Title:       "Verify Service Status",
		Description: fmt.Sprintf("Check if %s is running and accessible", analysis.Service),
		Command:     fmt.Sprintf("systemctl status %s || ps aux | grep %s", analysis.Service, analysis.Service),
		Critical:    true,
	})
	
	// Step 2: Check recent logs
	steps = append(steps, RunbookStep{
		Order:       len(steps) + 1,
		Title:       "Review Recent Logs",
		Description: fmt.Sprintf("Examine recent error logs for %s", analysis.Service),
		Command:     fmt.Sprintf("journalctl -u %s -n 50 --no-pager || tail -100 /var/log/%s.log", analysis.Service, analysis.Service),
		Critical:    true,
	})
	
	// Step 3: Check resource utilization
	steps = append(steps, RunbookStep{
		Order:       len(steps) + 1,
		Title:       "Check Resource Utilization",
		Description: "Monitor CPU, memory, and disk usage",
		Command:     "top -b -n 1 | head -20 && df -h && free -h",
		Critical:    false,
	})
	
	return steps
}

// generateDiagnosticSteps creates pattern-specific diagnostic steps
func (pa *PatternAnalyzer) generateDiagnosticSteps(analysis *PatternAnalysis) []RunbookStep {
	steps := []RunbookStep{}
	
	// Pattern-specific diagnostics
	switch analysis.Pattern {
	case "payment_gateway_timeout", "payment_gateway_connection_failed", "stripe_api_outage", "payment_gateway_issue":
		steps = append(steps, RunbookStep{
			Order:       len(steps) + 1,
			Title:       "Test Payment Gateway Connectivity",
			Description: "Verify connectivity to payment gateway APIs",
			Command:     "curl -I https://api.stripe.com/v1 && ping -c 3 api.stripe.com",
			Critical:    true,
		})
		
	case "postgres_connection_timeout", "postgres_connection_failed", "database_timeout", "database_connection_failed":
		steps = append(steps, RunbookStep{
			Order:       len(steps) + 1,
			Title:       "Test PostgreSQL Connection",
			Description: "Verify connectivity to PostgreSQL database",
			Command:     "psql -h localhost -U postgres -c 'SELECT 1;' 2>/dev/null || echo 'PostgreSQL connection failed'",
			Critical:    true,
		})
		steps = append(steps, RunbookStep{
			Order:       len(steps) + 1,
			Title:       "Check PostgreSQL Connection Pool",
			Description: "Monitor database connection pool status",
			Command:     "ps aux | grep postgres | grep -v grep || echo 'No PostgreSQL processes found'",
			Critical:    true,
		})
		steps = append(steps, RunbookStep{
			Order:       len(steps) + 1,
			Title:       "Validate Database Configuration",
			Description: "Check database configuration and connectivity settings",
			Command:     "echo 'Checking database configuration...' && env | grep -i db || echo 'No database env vars found'",
			Critical:    false,
		})
		
	case "mysql_connection_timeout", "mysql_connection_failed", "mysql_timeout":
		steps = append(steps, RunbookStep{
			Order:       len(steps) + 1,
			Title:       "Test MySQL Connection",
			Description: "Verify connectivity to MySQL database",
			Command:     "mysql -h localhost -u root -e 'SELECT 1;' 2>/dev/null || echo 'MySQL connection failed'",
			Critical:    true,
		})
		steps = append(steps, RunbookStep{
			Order:       len(steps) + 1,
			Title:       "Check MySQL Process Status",
			Description: "Monitor MySQL server process status",
			Command:     "ps aux | grep mysql | grep -v grep || echo 'No MySQL processes found'",
			Critical:    true,
		})
		
	case "redis_connection_timeout", "redis_connection_failed", "cache_timeout":
		steps = append(steps, RunbookStep{
			Order:       len(steps) + 1,
			Title:       "Test Redis Connection",
			Description: "Verify connectivity to Redis cache",
			Command:     "redis-cli ping 2>/dev/null || echo 'Redis connection failed'",
			Critical:    true,
		})
		steps = append(steps, RunbookStep{
			Order:       len(steps) + 1,
			Title:       "Check Redis Memory Usage",
			Description: "Monitor Redis memory consumption",
			Command:     "redis-cli info memory 2>/dev/null | grep used_memory_human || echo 'Cannot get Redis memory info'",
			Critical:    false,
		})
		
	case "connection_timeout", "connection_refused":
		steps = append(steps, RunbookStep{
			Order:       len(steps) + 1,
			Title:       "Test Network Connectivity",
			Description: "Verify network connectivity to dependent services",
			Command:     "netstat -tlnp | grep :80 && netstat -tlnp | grep :443",
			Critical:    true,
		})
		
	case "authentication_issue":
		steps = append(steps, RunbookStep{
			Order:       len(steps) + 1,
			Title:       "Check Authentication Service",
			Description: "Verify authentication service is functioning",
			Command:     "curl -X POST http://localhost:8080/auth/test -H 'Content-Type: application/json' -d '{\"test\": true}'",
			Critical:    true,
		})
		
	// Vault/JWKS specific patterns
	case "vault_access_denied", "vault_policy_mismatch", "vault_issue":
		steps = append(steps, RunbookStep{
			Order:       len(steps) + 1,
			Title:       "Check Vault Connectivity",
			Description: "Verify connectivity to Vault server",
			Command:     "curl -f $VAULT_ADDR/v1/sys/health || echo 'Vault health check failed'",
			Critical:    true,
		})
		steps = append(steps, RunbookStep{
			Order:       len(steps) + 1,
			Title:       "Verify Vault Token",
			Description: "Check if Vault token is valid and has proper permissions",
			Command:     "vault token lookup || echo 'Vault token lookup failed'",
			Critical:    true,
		})
		steps = append(steps, RunbookStep{
			Order:       len(steps) + 1,
			Title:       "Check Vault Policies",
			Description: "Verify current Vault policies and permissions",
			Command:     "vault policy read $(whoami) || echo 'Policy read failed'",
			Critical:    false,
		})
		
	case "jwks_refresh_failure", "signing_key_expired", "jwt_token_issue":
		steps = append(steps, RunbookStep{
			Order:       len(steps) + 1,
			Title:       "Check JWKS Endpoint",
			Description: "Verify JWKS endpoint is accessible and returning valid keys",
			Command:     "curl -f $VAULT_ADDR/v1/identity/oidc/.well-known/jwks.json || echo 'JWKS endpoint failed'",
			Critical:    true,
		})
		steps = append(steps, RunbookStep{
			Order:       len(steps) + 1,
			Title:       "Verify Token Signing Configuration",
			Description: "Check token signing key configuration and rotation status",
			Command:     "vault read identity/oidc/key || echo 'Key configuration read failed'",
			Critical:    true,
		})
		steps = append(steps, RunbookStep{
			Order:       len(steps) + 1,
			Title:       "Test JWT Token Generation",
			Description: "Test if new JWT tokens can be generated and validated",
			Command:     "curl -X POST $VAULT_ADDR/v1/identity/oidc/token/introspect -H 'Authorization: Bearer $VAULT_TOKEN' -d '{}' || echo 'Token introspection failed'",
			Critical:    false,
		})
		
	case "token_expired", "token_verification_failed":
		steps = append(steps, RunbookStep{
			Order:       len(steps) + 1,
			Title:       "Check Token Validity",
			Description: "Verify current token expiration and validity",
			Command:     "vault token lookup -format=json | jq '.data.expire_time' || echo 'Token lookup failed'",
			Critical:    true,
		})
		steps = append(steps, RunbookStep{
			Order:       len(steps) + 1,
			Title:       "Renew Token if Needed",
			Description: "Attempt to renew expired token",
			Command:     "vault token renew || echo 'Token renewal failed'",
			Critical:    false,
		})
	}
	
	// Component-specific diagnostics
	if analysis.Component != "" {
		steps = append(steps, RunbookStep{
			Order:       len(steps) + 1,
			Title:       fmt.Sprintf("Check %s Health", analysis.Component),
			Description: fmt.Sprintf("Verify health status of %s component", analysis.Component),
			Command:     fmt.Sprintf("curl -f http://localhost:8080/health/%s || echo 'Health check failed'", analysis.Component),
			Critical:    false,
		})
	}
	
	return steps
}

// generateResolutionSteps creates general resolution steps
func (pa *PatternAnalyzer) generateResolutionSteps(analysis *PatternAnalysis) []RunbookStep {
	steps := []RunbookStep{}
	
	// Step 1: Check for similar incidents
	steps = append(steps, RunbookStep{
		Order:       len(steps) + 1,
		Title:       "Review Similar Incidents",
		Description: "Check for similar resolved incidents and their solutions",
		Command:     fmt.Sprintf("health-monitor incident list --service %s --state resolved --format json | jq '.incidents[-3:]'", analysis.Service),
		Critical:    false,
	})
	
	// Step 2: Check dependencies
	if analysis.Dependency != "" {
		steps = append(steps, RunbookStep{
			Order:       len(steps) + 1,
			Title:       "Verify Service Dependencies",
			Description: fmt.Sprintf("Check health of dependencies: %s", analysis.Dependency),
			Command:     fmt.Sprintf("echo 'Checking dependency: %s' && curl -f http://localhost:8080/health", analysis.Dependency),
			Critical:    false,
		})
	}
	
	// Step 3: Prepare escalation
	steps = append(steps, RunbookStep{
		Order:       len(steps) + 1,
		Title:       "Prepare Escalation Plan",
		Description: "Document findings and prepare escalation if needed",
		Command:     "echo 'Document current status, steps taken, and next actions for escalation'",
		Critical:    false,
	})
	
	return steps
}

// generateCategorySpecificSteps adds category-specific troubleshooting steps
func (pa *PatternAnalyzer) generateCategorySpecificSteps(analysis *PatternAnalysis, steps []RunbookStep) []RunbookStep {
	// Add category-specific steps
	switch analysis.Category {
	case "capacity":
		steps = append(steps, RunbookStep{
			Order:       len(steps) + 1,
			Title:       "Check Resource Capacity",
			Description: "Monitor system resource utilization",
			Command:     "df -h && free -h",
			Critical:    true,
		})
	case "latency":
		steps = append(steps, RunbookStep{
			Order:       len(steps) + 1,
			Title:       "Measure Response Latency",
			Description: "Check service response times",
			Command:     "curl -w '@curl-format.txt' -o /dev/null -s {{url}}",
			Critical:    true,
		})
	}
	
	return steps
}

// generateMetricQueries generates relevant Prometheus queries
func (pa *PatternAnalyzer) generateMetricQueries(analysis *PatternAnalysis) []MetricQuery {
	queries := []MetricQuery{}
	
	// Service-specific metrics
	queries = append(queries, MetricQuery{
		Name:        "Error Rate",
		Query:       fmt.Sprintf(`rate(http_requests_total{service="%s",status=~"5.."}[5m])`, analysis.Service),
		Description: "Error rate for the service",
	})
	
	queries = append(queries, MetricQuery{
		Name:        "Request Rate",
		Query:       fmt.Sprintf(`rate(http_requests_total{service="%s"}[5m])`, analysis.Service),
		Description: "Request rate for the service",
	})
	
	queries = append(queries, MetricQuery{
		Name:        "Response Time",
		Query:       fmt.Sprintf(`histogram_quantile(0.95, rate(http_request_duration_seconds_bucket{service="%s"}[5m]))`, analysis.Service),
		Description: "95th percentile response time",
	})
	
	// Pattern-specific metrics
	switch analysis.Pattern {
	case "postgres_connection_timeout", "postgres_connection_failed", "database_timeout", "database_connection_failed":
		queries = append(queries, MetricQuery{
			Name:        "PostgreSQL Connections",
			Query:       `pg_stat_activity_count`,
			Description: "Number of active PostgreSQL connections",
		})
		queries = append(queries, MetricQuery{
			Name:        "PostgreSQL Connection Pool",
			Query:       `pg_stat_activity_count / pg_settings_max_connections * 100`,
			Description: "PostgreSQL connection pool utilization percentage",
		})
		queries = append(queries, MetricQuery{
			Name:        "PostgreSQL Query Duration",
			Query:       `histogram_quantile(0.95, rate(pg_stat_statements_mean_time_seconds[5m]))`,
			Description: "95th percentile PostgreSQL query response time",
		})
		
	case "mysql_connection_timeout", "mysql_connection_failed", "mysql_timeout":
		queries = append(queries, MetricQuery{
			Name:        "MySQL Connections",
			Query:       `mysql_global_status_threads_connected`,
			Description: "Number of active MySQL connections",
		})
		queries = append(queries, MetricQuery{
			Name:        "MySQL Connection Pool",
			Query:       `mysql_global_status_threads_connected / mysql_global_variables_max_connections * 100`,
			Description: "MySQL connection pool utilization percentage",
		})
		queries = append(queries, MetricQuery{
			Name:        "MySQL Query Duration",
			Query:       `histogram_quantile(0.95, rate(mysql_global_status_slow_queries[5m]))`,
			Description: "95th percentile MySQL query response time",
		})
		
	case "redis_connection_timeout", "redis_connection_failed", "cache_timeout":
		queries = append(queries, MetricQuery{
			Name:        "Redis Connections",
			Query:       `redis_connected_clients`,
			Description: "Number of active Redis connections",
		})
		queries = append(queries, MetricQuery{
			Name:        "Redis Memory Usage",
			Query:       `redis_memory_used_bytes / 1024 / 1024`,
			Description: "Redis memory usage in MB",
		})
		queries = append(queries, MetricQuery{
			Name:        "Redis Hit Rate",
			Query:       `rate(redis_keyspace_hits_total[5m]) / (rate(redis_keyspace_hits_total[5m]) + rate(redis_keyspace_misses_total[5m])) * 100`,
			Description: "Redis cache hit rate percentage",
		})
		
	case "payment_gateway_timeout", "payment_gateway_connection_failed", "stripe_api_outage", "payment_gateway_issue":
		queries = append(queries, MetricQuery{
			Name:        "Payment Gateway Errors",
			Query:       fmt.Sprintf(`rate(payment_gateway_errors_total{service="%s"}[5m])`, analysis.Service),
			Description: "Payment gateway error rate",
		})
		queries = append(queries, MetricQuery{
			Name:        "Payment Gateway Latency",
			Query:       fmt.Sprintf(`histogram_quantile(0.95, rate(payment_gateway_request_duration_seconds{service="%s"}[5m]))`, analysis.Service),
			Description: "95th percentile payment gateway response time",
		})
		
	case "connection_refused", "connection_timeout":
		queries = append(queries, MetricQuery{
			Name:        "Connection Errors",
			Query:       fmt.Sprintf(`rate(connection_errors_total{service="%s"}[5m])`, analysis.Service),
			Description: "Connection error rate",
		})
	case "query_timeout":
		queries = append(queries, MetricQuery{
			Name:        "Database Query Time",
			Query:       `histogram_quantile(0.95, rate(pg_stat_statements_mean_time_seconds[5m]))`,
			Description: "Database query response time",
		})
	case "out_of_memory":
		queries = append(queries, MetricQuery{
			Name:        "Memory Usage",
			Query:       `process_resident_memory_bytes / 1024 / 1024`,
			Description: "Memory usage in MB",
		})
	}
	
	return queries
}

// generateLogQueries generates relevant log queries
func (pa *PatternAnalyzer) generateLogQueries(analysis *PatternAnalysis) []LogQuery {
	queries := []LogQuery{}
	
	// Service-specific log queries
	queries = append(queries, LogQuery{
		Name:        "Service Errors",
		Query:       fmt.Sprintf(`{service="%s"} |= "error"`, analysis.Service),
		Description: "Error logs for the service",
		TimeRange:   "1h",
	})
	
	queries = append(queries, LogQuery{
		Name:        "Service Timeouts",
		Query:       fmt.Sprintf(`{service="%s"} |= "timeout"`, analysis.Service),
		Description: "Timeout logs for the service",
		TimeRange:   "1h",
	})
	
	// Pattern-specific log queries
	switch analysis.Pattern {
	case "postgres_connection_timeout", "postgres_connection_failed", "database_timeout", "database_connection_failed":
		queries = append(queries, LogQuery{
			Name:        "PostgreSQL Connection Errors",
			Query:       `{service="` + analysis.Service + `"} |= "postgres" |= "connection" |= "timeout"`,
			Description: "PostgreSQL connection and timeout logs",
			TimeRange:   "1h",
		})
		queries = append(queries, LogQuery{
			Name:        "Database Query Errors",
			Query:       `{service="` + analysis.Service + `"} |= "sql" |= "query" |= "database"`,
			Description: "Database query related error logs",
			TimeRange:   "1h",
		})
		
	case "mysql_connection_timeout", "mysql_connection_failed", "mysql_timeout":
		queries = append(queries, LogQuery{
			Name:        "MySQL Connection Errors",
			Query:       `{service="` + analysis.Service + `"} |= "mysql" |= "connection" |= "timeout"`,
			Description: "MySQL connection and timeout logs",
			TimeRange:   "1h",
		})
		queries = append(queries, LogQuery{
			Name:        "Database Query Errors",
			Query:       `{service="` + analysis.Service + `"} |= "sql" |= "query" |= "database"`,
			Description: "Database query related error logs",
			TimeRange:   "1h",
		})
		
	case "redis_connection_timeout", "redis_connection_failed", "cache_timeout":
		queries = append(queries, LogQuery{
			Name:        "Redis Connection Errors",
			Query:       `{service="` + analysis.Service + `"} |= "redis" |= "cache" |= "timeout"`,
			Description: "Redis connection and cache timeout logs",
			TimeRange:   "1h",
		})
		
	case "payment_gateway_timeout", "payment_gateway_connection_failed", "stripe_api_outage", "payment_gateway_issue":
		queries = append(queries, LogQuery{
			Name:        "Payment Gateway Errors",
			Query:       `{service="` + analysis.Service + `"} |= "payment" |= "gateway" |= "stripe"`,
			Description: "Payment gateway related error logs",
			TimeRange:   "1h",
		})
	}
	
	// Component-specific log queries
	if analysis.Component != "" {
		queries = append(queries, LogQuery{
			Name:        "Component Errors",
			Query:       fmt.Sprintf(`{component="%s"} |= "error"`, analysis.Component),
			Description: fmt.Sprintf("Error logs for %s", analysis.Component),
			TimeRange:   "1h",
		})
	}
	
	return queries
}

// Helper functions
func (pa *PatternAnalyzer) contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func (pa *PatternAnalyzer) addUnique(slice []string, item string) []string {
	for _, s := range slice {
		if s == item {
			return slice
		}
	}
	return append(slice, item)
}
