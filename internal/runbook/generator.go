package runbook

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"health-monitor/internal/config"
)

// RunbookGenerator generates runbooks from patterns and templates
type RunbookGenerator struct {
	analyzer  *PatternAnalyzer
	templates map[string]*template.Template
	config    config.RunbookConfig
}

// NewRunbookGenerator creates a new runbook generator
func NewRunbookGenerator(analyzer *PatternAnalyzer, config config.RunbookConfig) *RunbookGenerator {
	return &RunbookGenerator{
		analyzer:  analyzer,
		templates: make(map[string]*template.Template),
		config:    config,
	}
}

// GenerateFromPattern generates a runbook from pattern analysis
func (rg *RunbookGenerator) GenerateFromPattern(analysis *PatternAnalysis) (*Runbook, error) {
	if analysis.Confidence < rg.config.ConfidenceThreshold {
		return nil, fmt.Errorf("pattern confidence %.2f below threshold %.2f", 
			analysis.Confidence, rg.config.ConfidenceThreshold)
	}

	// Generate runbook ID
	runbookID := rg.generateRunbookID(analysis)
	
	// Create runbook
	runbook := &Runbook{
		ID:          runbookID,
		Title:       rg.generateTitle(analysis),
		Service:     analysis.Service,
		Pattern:     analysis.Pattern,
		Category:    analysis.Category,
		Component:   analysis.Component,
		Severity:    rg.inferSeverity(analysis),
		Content:     "", // Will be generated
		Format:      rg.config.OutputFormat,
		Steps:       analysis.SuggestedSteps,
		Metrics:     analysis.RelatedMetrics,
		LogQueries:  analysis.RelatedLogs,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		CreatedBy:   "health-monitor",
		Published:   false,
		Tags:        analysis.Tags,
		Metadata:    analysis.Metadata,
		Cluster:     analysis.Cluster,
		Namespace:   analysis.Namespace,
		Deployment:  analysis.Deployment,
	}

	// Generate content using template
	content, err := rg.generateContent(analysis, runbook)
	if err != nil {
		return nil, fmt.Errorf("failed to generate content: %w", err)
	}
	runbook.Content = content

	return runbook, nil
}

// GenerateForIncident generates a runbook for a specific incident
func (rg *RunbookGenerator) GenerateForIncident(incidentID string) (*Runbook, error) {
	// Analyze the incident
	analysis, err := rg.analyzer.AnalyzeIncident(incidentID)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze incident: %w", err)
	}

	// Generate runbook from analysis
	return rg.GenerateFromPattern(analysis)
}

// SaveRunbook saves a runbook to the configured storage
func (rg *RunbookGenerator) SaveRunbook(runbook *Runbook) error {
	// Create output directory if it doesn't exist
	if err := os.MkdirAll(rg.config.OutputDirectory, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Generate filename
	filename := fmt.Sprintf("%s.%s", runbook.ID, rg.config.OutputFormat)
	filepath := filepath.Join(rg.config.OutputDirectory, filename)

	// Write runbook to file
	if err := os.WriteFile(filepath, []byte(runbook.Content), 0644); err != nil {
		return fmt.Errorf("failed to write runbook file: %w", err)
	}

	return nil
}

// LoadTemplate loads a runbook template from file
func (rg *RunbookGenerator) LoadTemplate(name, filename string) error {
	content, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read template file: %w", err)
	}

	tmpl, err := template.New(name).Parse(string(content))
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	rg.templates[name] = tmpl
	return nil
}

// generateRunbookID generates a unique runbook ID
func (rg *RunbookGenerator) generateRunbookID(analysis *PatternAnalysis) string {
	timestamp := time.Now().Format("20060102-150405")
	pattern := strings.ReplaceAll(analysis.Pattern, "_", "-")
	return fmt.Sprintf("rb-%s-%s-%s", timestamp, analysis.Service, pattern)
}

// generateTitle generates a descriptive title for the runbook
func (rg *RunbookGenerator) generateTitle(analysis *PatternAnalysis) string {
	patternTitle := strings.ReplaceAll(analysis.Pattern, "_", " ")
	patternTitle = strings.Title(patternTitle)
	
	if analysis.Component != "" {
		return fmt.Sprintf("%s - %s Troubleshooting", analysis.Component, patternTitle)
	}
	
	return fmt.Sprintf("%s - %s Troubleshooting", analysis.Service, patternTitle)
}

// inferSeverity infers severity from pattern analysis
func (rg *RunbookGenerator) inferSeverity(analysis *PatternAnalysis) string {
	// High severity patterns
	highSeverityPatterns := []string{
		"out_of_memory", "deadlock", "too_many_connections", 
		"connection_refused", "internal_server_error",
	}
	
	for _, pattern := range highSeverityPatterns {
		if analysis.Pattern == pattern {
			return "P1"
		}
	}
	
	// Medium severity patterns
	mediumSeverityPatterns := []string{
		"connection_timeout", "query_timeout", "database_error",
		"permission_denied",
	}
	
	for _, pattern := range mediumSeverityPatterns {
		if analysis.Pattern == pattern {
			return "P2"
		}
	}
	
	// Default to P3
	return "P3"
}

// generateContent generates runbook content using templates
func (rg *RunbookGenerator) generateContent(analysis *PatternAnalysis, runbook *Runbook) (string, error) {
	// Try to use pattern-specific template first
	templateName := analysis.Pattern
	if tmpl, exists := rg.templates[templateName]; exists {
		return rg.renderTemplate(tmpl, analysis, runbook)
	}
	
	// Try category-specific template
	templateName = analysis.Category
	if tmpl, exists := rg.templates[templateName]; exists {
		return rg.renderTemplate(tmpl, analysis, runbook)
	}
	
	// Use default template
	return rg.generateDefaultContent(analysis, runbook), nil
}

// renderTemplate renders a template with the given data
func (rg *RunbookGenerator) renderTemplate(tmpl *template.Template, analysis *PatternAnalysis, runbook *Runbook) (string, error) {
	var buf bytes.Buffer
	
	data := struct {
		Analysis *PatternAnalysis
		Runbook  *Runbook
	}{
		Analysis: analysis,
		Runbook:  runbook,
	}
	
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}
	
	return buf.String(), nil
}

// generateDefaultContent generates default markdown content
func (rg *RunbookGenerator) generateDefaultContent(analysis *PatternAnalysis, runbook *Runbook) string {
	var content strings.Builder
	
	// Header
	content.WriteString(fmt.Sprintf("# %s\n\n", runbook.Title))
	
	// Overview
	content.WriteString("## Overview\n\n")
	content.WriteString(fmt.Sprintf("**Service**: %s\n", runbook.Service))
	if runbook.Component != "" {
		content.WriteString(fmt.Sprintf("**Component**: %s\n", runbook.Component))
	}
	content.WriteString(fmt.Sprintf("**Pattern**: `%s`\n", runbook.Pattern))
	if runbook.Category != "" {
		content.WriteString(fmt.Sprintf("**Category**: %s\n", runbook.Category))
	}
	content.WriteString(fmt.Sprintf("**Severity**: %s\n", runbook.Severity))
	
	if runbook.Cluster != "" || runbook.Namespace != "" || runbook.Deployment != "" {
		content.WriteString("\n### Kubernetes Context\n")
		if runbook.Cluster != "" {
			content.WriteString(fmt.Sprintf("**Cluster**: %s\n", runbook.Cluster))
		}
		if runbook.Namespace != "" {
			content.WriteString(fmt.Sprintf("**Namespace**: %s\n", runbook.Namespace))
		}
		if runbook.Deployment != "" {
			content.WriteString(fmt.Sprintf("**Deployment**: %s\n", runbook.Deployment))
		}
	}
	content.WriteString(fmt.Sprintf("**Frequency**: %d occurrences\n", analysis.Frequency))
	content.WriteString(fmt.Sprintf("**Last Seen**: %s\n", analysis.LastSeen.Format("2006-01-02 15:04:05")))
	content.WriteString(fmt.Sprintf("**Confidence**: %.1f%%\n\n", analysis.Confidence*100))
	
	// Symptoms
	content.WriteString("## Symptoms\n\n")
	content.WriteString(fmt.Sprintf("- Service: %s\n", runbook.Service))
	if runbook.Component != "" {
		content.WriteString(fmt.Sprintf("- Component: %s\n", runbook.Component))
	}
	content.WriteString(fmt.Sprintf("- Error Pattern: `%s`\n", runbook.Pattern))
	content.WriteString(fmt.Sprintf("- Category: %s\n\n", runbook.Category))
	
	// Root Cause Analysis
	content.WriteString("## Root Cause Analysis\n\n")
	
	// Add RCA data if available from resolved incident
	if analysis.Metadata != nil {
		if rootCause, exists := analysis.Metadata["root_cause"]; exists && rootCause != "" {
			content.WriteString("### Root Cause\n\n")
			content.WriteString(fmt.Sprintf("%s\n\n", rootCause))
		}
		if fix, exists := analysis.Metadata["fix"]; exists && fix != "" {
			content.WriteString("### Applied Fix\n\n")
			content.WriteString(fmt.Sprintf("%s\n\n", fix))
		}
		if failureType, exists := analysis.Metadata["failure_type"]; exists && failureType != "" {
			content.WriteString(fmt.Sprintf("**Failure Type**: %s\n\n", failureType))
		}
		if dependency, exists := analysis.Metadata["dependency"]; exists && dependency != "" {
			content.WriteString(fmt.Sprintf("**Dependency**: %s\n\n", dependency))
		}
	}
	
	content.WriteString("### Historical Analysis\n\n")
	content.WriteString("Based on historical incident analysis:\n\n")
	content.WriteString(fmt.Sprintf("- **Pattern**: %s\n", runbook.Pattern))
	content.WriteString(fmt.Sprintf("- **Frequency**: This pattern has occurred %d times\n", analysis.Frequency))
	if len(analysis.IncidentIDs) > 0 {
		content.WriteString("- **Related Incidents**:\n")
		for _, id := range analysis.IncidentIDs {
			content.WriteString(fmt.Sprintf("  - [%s](/incidents/%s)\n", id, id))
		}
	}
	content.WriteString("\n")
	
	// Troubleshooting Steps
	if len(runbook.Steps) > 0 {
		content.WriteString("## Troubleshooting Steps\n\n")
		for i, step := range runbook.Steps {
			critical := ""
			if step.Critical {
				critical = " 🔴"
			}
			content.WriteString(fmt.Sprintf("%d. **%s**%s\n", i+1, step.Title, critical))
			content.WriteString(fmt.Sprintf("   %s\n", step.Description))
			if step.Command != "" {
				content.WriteString("   ```bash\n")
				content.WriteString(fmt.Sprintf("   %s\n", step.Command))
				content.WriteString("   ```\n")
			}
			if step.Expected != "" {
				content.WriteString(fmt.Sprintf("   **Expected**: %s\n", step.Expected))
			}
			content.WriteString("\n")
		}
	}
	
	// Metrics to Monitor
	if len(runbook.Metrics) > 0 {
		content.WriteString("## Metrics to Monitor\n\n")
		for _, metric := range runbook.Metrics {
			content.WriteString(fmt.Sprintf("### %s\n\n", metric.Name))
			content.WriteString(fmt.Sprintf("**Description**: %s\n\n", metric.Description))
			content.WriteString("**Prometheus Query**:\n```\n")
			content.WriteString(fmt.Sprintf("%s\n", metric.Query))
			content.WriteString("```\n\n")
		}
	}
	
	// Log Queries
	if len(runbook.LogQueries) > 0 {
		content.WriteString("## Log Queries\n\n")
		for _, logQuery := range runbook.LogQueries {
			content.WriteString(fmt.Sprintf("### %s\n\n", logQuery.Name))
			content.WriteString(fmt.Sprintf("**Description**: %s\n", logQuery.Description))
			content.WriteString(fmt.Sprintf("**Time Range**: %s\n", logQuery.TimeRange))
			content.WriteString("**Loki Query**:\n```\n")
			content.WriteString(fmt.Sprintf("%s\n", logQuery.Query))
			content.WriteString("```\n\n")
		}
	}
	
	// Verification
	content.WriteString("## Verification\n\n")
	content.WriteString("After completing the troubleshooting steps:\n\n")
	content.WriteString("1. Verify service is responding normally\n")
	content.WriteString("2. Check error rates have decreased\n")
	content.WriteString("3. Monitor for recurrence of the pattern\n")
	content.WriteString("4. Validate performance metrics are within normal ranges\n\n")
	
	// Prevention
	content.WriteString("## Prevention\n\n")
	
	// Add prevention from RCA if available
	if analysis.Metadata != nil {
		if prevention, exists := analysis.Metadata["prevention"]; exists && prevention != "" {
			content.WriteString("### Recommended Prevention\n\n")
			content.WriteString(fmt.Sprintf("%s\n\n", prevention))
		}
	}
	
	content.WriteString("### General Prevention Strategies\n\n")
	content.WriteString("To prevent recurrence of this issue:\n\n")
	switch runbook.Pattern {
	case "connection_refused":
		content.WriteString("- Implement service health checks\n")
		content.WriteString("- Set up connection retry logic\n")
		content.WriteString("- Monitor service availability\n")
	case "connection_timeout":
		content.WriteString("- Implement proper timeout handling\n")
		content.WriteString("- Monitor resource utilization\n")
		content.WriteString("- Consider connection pooling\n")
	case "query_timeout":
		content.WriteString("- Optimize database queries\n")
		content.WriteString("Add appropriate indexes\n")
		content.WriteString("- Monitor connection pool usage\n")
		content.WriteString("- Set up database query monitoring\n")
	case "out_of_memory":
		content.WriteString("- Monitor memory usage trends\n")
		content.WriteString("- Implement memory limits\n")
		content.WriteString("- Set up memory usage alerts\n")
	case "too_many_connections":
		content.WriteString("- Monitor connection pool usage\n")
		content.WriteString("- Implement connection limits\n")
		content.WriteString("- Set up connection monitoring\n")
	default:
		content.WriteString("- Monitor system resources\n")
		content.WriteString("- Set up appropriate alerts\n")
		content.WriteString("- Regular performance reviews\n")
	}
	content.WriteString("\n")
	
	// Related Information
	if len(analysis.IncidentIDs) > 0 {
		content.WriteString("## Related Incidents\n\n")
		for _, id := range analysis.IncidentIDs {
			content.WriteString(fmt.Sprintf("- [%s](/incidents/%s)\n", id, id))
		}
		content.WriteString("\n")
	}
	
	// Metadata
	content.WriteString("---\n\n")
	content.WriteString(fmt.Sprintf("**Generated**: %s\n", runbook.CreatedAt.Format("2006-01-02 15:04:05")))
	content.WriteString(fmt.Sprintf("**Pattern**: %s\n", runbook.Pattern))
	content.WriteString(fmt.Sprintf("**Confidence**: %.1f%%\n", analysis.Confidence*100))
	if len(runbook.Tags) > 0 {
		content.WriteString(fmt.Sprintf("**Tags**: %s\n", strings.Join(runbook.Tags, ", ")))
	}
	
	return content.String()
}

// LoadDefaultTemplates loads built-in templates
func (rg *RunbookGenerator) LoadDefaultTemplates() error {
	// PostgreSQL timeout template
	postgresTemplate := `# {{.Runbook.Title}}

## Overview
**Service**: {{.Runbook.Service}}
**Pattern**: {{.Runbook.Pattern}}
**Component**: {{.Runbook.Component}}

## PostgreSQL Query Timeout Troubleshooting

### Symptoms
- Database queries are timing out
- Application experiencing slow response times
- Connection pool exhaustion

### Root Cause Analysis
{{range $incident := .Analysis.IncidentIDs}}
- Related Incident: [{{$incident}}](/incidents/{{$incident}})
{{end}}

### Troubleshooting Steps
{{range $step := .Runbook.Steps}}
{{.Order}}. **{{.Title}}**
   {{.Description}}
   {{if .Command}}` + "`" + `sql
   {{.Command}}
   ` + "`" + `{{end}}
{{end}}

### Prevention
- Optimize slow queries
- Add appropriate database indexes
- Monitor connection pool usage
- Set up query performance monitoring
`
	
	tmpl, err := template.New("query_timeout").Parse(postgresTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse postgres template: %w", err)
	}
	rg.templates["query_timeout"] = tmpl
	
	// Connection refused template
	connectionTemplate := `# {{.Runbook.Title}}

## Overview
**Service**: {{.Runbook.Service}}
**Pattern**: {{.Runbook.Pattern}}

## Connection Refused Troubleshooting

### Symptoms
- Unable to establish connection to service
- Network connection errors
- Service appears to be down

### Troubleshooting Steps
{{range $step := .Runbook.Steps}}
{{.Order}}. **{{.Title}}**
   {{.Description}}
   {{if .Command}}` + "`" + `bash
   {{.Command}}
   ` + "`" + `{{end}}
{{end}}

### Prevention
- Implement service health checks
- Set up automated service restart
- Monitor service availability
- Set up connection monitoring
`
	
	tmpl, err = template.New("connection_refused").Parse(connectionTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse connection template: %w", err)
	}
	rg.templates["connection_refused"] = tmpl
	
	return nil
}
