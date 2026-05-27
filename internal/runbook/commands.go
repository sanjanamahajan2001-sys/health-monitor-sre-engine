package runbook

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"health-monitor/internal/config"
	"health-monitor/internal/incident"
)

// RunbookCmd handles runbook-related CLI commands
type RunbookCmd struct {
	store          Store
	analyzer       *PatternAnalyzer
	generator      *RunbookGenerator
	incidentStore  *incident.Store
	config         config.RunbookConfig
}

// NewRunbookCmd creates a new runbook command handler
func NewRunbookCmd(incidentStore *incident.Store, cfg config.Config) (*RunbookCmd, error) {
	// Load runbook config from main config
	runbookConfig := cfg.RunbookSuggestions
	
	// Create store
	store := NewFileStore(runbookConfig.OutputDirectory, runbookConfig)
	
	// Create analyzer
	analyzer := NewPatternAnalyzer(incidentStore, runbookConfig)
	
	// Create generator
	generator := NewRunbookGenerator(analyzer, runbookConfig)
	
	// Load default templates
	if err := generator.LoadDefaultTemplates(); err != nil {
		return nil, fmt.Errorf("failed to load default templates: %w", err)
	}

	return &RunbookCmd{
		store:         store,
		analyzer:      analyzer,
		generator:     generator,
		incidentStore: incidentStore,
		config:        runbookConfig,
	}, nil
}

// HandleSuggestCommand handles runbook suggestion commands
func (rc *RunbookCmd) HandleSuggestCommand(args []string) int {
	if !rc.config.Enabled {
		fmt.Fprintf(os.Stderr, "Error: Runbook suggestions are disabled. Enable them in configuration.\n")
		return 1
	}

	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Error: incident ID required\n")
		fmt.Fprintf(os.Stderr, "Usage: health-monitor runbook suggest <incident-id>\n")
		return 1
	}

	incidentID := args[0]
	
	// Validate incident ID
	if err := incident.ValidateIncidentID(incidentID); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	// Analyze incident
	analysis, err := rc.analyzer.AnalyzeIncident(incidentID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error analyzing incident: %v\n", err)
		return 1
	}

	fmt.Printf("🔍 Pattern Analysis for %s\n\n", incidentID)
	fmt.Printf("Pattern: %s\n", analysis.Pattern)
	fmt.Printf("Service: %s\n", analysis.Service)
	if analysis.Component != "" {
		fmt.Printf("Component: %s\n", analysis.Component)
	}
	if analysis.Category != "" {
		fmt.Printf("Category: %s\n", analysis.Category)
	}
	fmt.Printf("Confidence: %.1f%%\n\n", analysis.Confidence*100)

	// Generate runbook if confidence is high enough
	if analysis.Confidence >= rc.config.ConfidenceThreshold {
		runbook, err := rc.generator.GenerateFromPattern(analysis)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error generating runbook: %v\n", err)
			return 1
		}

		fmt.Printf("📚 Suggested Runbook: %s\n", runbook.ID)
		fmt.Printf("Title: %s\n", runbook.Title)
		fmt.Printf("Severity: %s\n", runbook.Severity)
		fmt.Printf("Steps: %d\n", len(runbook.Steps))
		
		// Save suggestion
		suggestion := &RunbookSuggestion{
			RunbookID:   runbook.ID,
			Title:       runbook.Title,
			Confidence:  analysis.Confidence,
			MatchReason: fmt.Sprintf("Pattern match: %s (confidence: %.1f%%)", analysis.Pattern, analysis.Confidence*100),
			IncidentID:  incidentID,
			SuggestedAt: time.Now(),
			Applied:     false,
		}

		if err := rc.store.SaveSuggestion(suggestion); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to save suggestion: %v\n", err)
		} else {
			fmt.Printf("💡 To generate and save this runbook, run:\n")
			fmt.Printf("   health-monitor runbook generate --pattern %s --service %s --save\n", analysis.Pattern, analysis.Service)
			fmt.Printf("   health-monitor runbook generate --incident %s --save\n", incidentID)
		}
	} else {
		fmt.Printf("⚠️  Confidence %.1f%% below threshold %.1f%% - no runbook suggested\n", 
			analysis.Confidence*100, rc.config.ConfidenceThreshold*100)
	}

	return 0
}

// HandlePatternsCommand handles pattern analysis commands
func (rc *RunbookCmd) HandlePatternsCommand(args []string) int {
	if !rc.config.Enabled {
		fmt.Fprintf(os.Stderr, "Error: Runbook patterns are disabled. Enable them in configuration.\n")
		return 1
	}

	service := ""
	days := 30
	
	// Parse arguments
	for i, arg := range args {
		switch arg {
		case "--service":
			if i+1 < len(args) {
				service = args[i+1]
			}
		case "--days":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &days)
			}
		}
	}

	fmt.Printf("🔍 Analyzing patterns")
	if service != "" {
		fmt.Printf(" for service: %s", service)
	}
	fmt.Printf(" (last %d days)\n\n", days)

	// Analyze patterns
	patterns, err := rc.analyzer.AnalyzeIncidentPatterns(service, days)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error analyzing patterns: %v\n", err)
		return 1
	}

	if len(patterns) == 0 {
		fmt.Printf("No patterns found")
		if service != "" {
			fmt.Printf(" for service %s", service)
		}
		fmt.Printf(" in the last %d days\n", days)
		return 0
	}

	fmt.Printf("Found %d patterns:\n\n", len(patterns))
	
	for i, pattern := range patterns {
		fmt.Printf("%d. %s\n", i+1, strings.ReplaceAll(pattern.Pattern, "_", " "))
		fmt.Printf("   Service: %s\n", pattern.Service)
		if pattern.Component != "" {
			fmt.Printf("   Component: %s\n", pattern.Component)
		}
		if pattern.Category != "" {
			fmt.Printf("   Category: %s\n", pattern.Category)
		}
		fmt.Printf("   Frequency: %d occurrences\n", pattern.Frequency)
		fmt.Printf("   Last Seen: %s\n", pattern.LastSeen.Format("2006-01-02 15:04"))
		fmt.Printf("   Confidence: %.1f%%\n", pattern.Confidence*100)
		
		if len(pattern.SuggestedSteps) > 0 {
			fmt.Printf("   Suggested Steps: %d\n", len(pattern.SuggestedSteps))
		}
		
		fmt.Printf("   💡 Generate runbook: health-monitor runbook generate --pattern %s --service %s\n\n", 
			pattern.Pattern, pattern.Service)
	}

	return 0
}

// HandleGenerateCommand handles runbook generation commands
func (rc *RunbookCmd) HandleGenerateCommand(args []string) int {
	if !rc.config.Enabled {
		fmt.Fprintf(os.Stderr, "Error: Runbook generation is disabled. Enable them in configuration.\n")
		return 1
	}

	// Parse generation options
	var incidentID, pattern, service string
	autoSave := false
	publish := false

	for i, arg := range args {
		switch arg {
		case "--incident":
			if i+1 < len(args) {
				incidentID = args[i+1]
			}
		case "--pattern":
			if i+1 < len(args) {
				pattern = args[i+1]
			}
		case "--service":
			if i+1 < len(args) {
				service = args[i+1]
			}
		case "--save":
			autoSave = true
		case "--publish":
			publish = true
		}
	}

	var analysis *PatternAnalysis
	var err error

	// Get analysis data
	if incidentID != "" {
		analysis, err = rc.analyzer.AnalyzeIncident(incidentID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error analyzing incident: %v\n", err)
			return 1
		}
	} else if pattern != "" && service != "" {
		// Create analysis from pattern and service
		analysis = &PatternAnalysis{
			Pattern:  pattern,
			Service:  service,
			Confidence: 0.8, // Default confidence for manual generation
		}
		analysis.SuggestedSteps = rc.analyzer.generateSuggestedSteps(analysis)
		analysis.RelatedMetrics = rc.analyzer.generateMetricQueries(analysis)
		analysis.RelatedLogs = rc.analyzer.generateLogQueries(analysis)
	} else {
		fmt.Fprintf(os.Stderr, "Error: Either --incident <id> or --pattern <pattern> --service <service> required\n")
		fmt.Fprintf(os.Stderr, "Usage: health-monitor runbook generate --incident <id>\n")
		fmt.Fprintf(os.Stderr, "       health-monitor runbook generate --pattern <pattern> --service <service>\n")
		return 1
	}

	// Generate runbook
	runbook, err := rc.generator.GenerateFromPattern(analysis)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating runbook: %v\n", err)
		return 1
	}

	fmt.Printf("📚 Generated Runbook\n\n")
	fmt.Printf("ID: %s\n", runbook.ID)
	fmt.Printf("Title: %s\n", runbook.Title)
	fmt.Printf("Service: %s\n", runbook.Service)
	fmt.Printf("Pattern: %s\n", runbook.Pattern)
	fmt.Printf("Severity: %s\n", runbook.Severity)
	fmt.Printf("Steps: %d\n", len(runbook.Steps))
	fmt.Printf("Metrics: %d\n", len(runbook.Metrics))
	fmt.Printf("Log Queries: %d\n", len(runbook.LogQueries))
	fmt.Printf("Format: %s\n", runbook.Format)
	fmt.Printf("Created: %s\n\n", runbook.CreatedAt.Format("2006-01-02 15:04:05"))

	// Preview content
	fmt.Printf("📄 Content Preview:\n")
	fmt.Printf("%s\n\n", strings.Split(runbook.Content, "\n\n")[0]) // First section

	if autoSave {
		if err := rc.store.Save(runbook); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving runbook: %v\n", err)
			return 1
		}
		// Use actual file extension, not format name
		ext := "json"
		if runbook.Format == "markdown" {
			ext = "md"
		}
		fmt.Printf("✅ Runbook saved to: %s/%s.%s\n", rc.config.OutputDirectory, runbook.ID, ext)
		
		if publish {
			runbook.Published = true
			if err := rc.store.Update(runbook); err != nil {
				fmt.Fprintf(os.Stderr, "Error publishing runbook: %v\n", err)
				return 1
			}
			fmt.Printf("✅ Runbook published\n")
		}
	} else {
		fmt.Printf("💡 To generate and save this runbook, run:\n")
		fmt.Printf("   health-monitor runbook save %s\n", runbook.ID)
		fmt.Printf("   health-monitor runbook save %s --publish\n", runbook.ID)
		fmt.Printf("   Or regenerate with: health-monitor runbook generate --incident %s --save\n", incidentID)
	}

	return 0
}

// HandleListCommand handles runbook listing commands
func (rc *RunbookCmd) HandleListCommand(args []string) int {
	service := ""
	pattern := ""
	limit := 20

	// Parse arguments
	for i, arg := range args {
		switch arg {
		case "--service":
			if i+1 < len(args) {
				service = args[i+1]
			}
		case "--pattern":
			if i+1 < len(args) {
				pattern = args[i+1]
			}
		case "--limit":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &limit)
			}
		}
	}

	runbooks, err := rc.store.List(service, pattern, limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing runbooks: %v\n", err)
		return 1
	}

	if len(runbooks) == 0 {
		fmt.Printf("No runbooks found")
		if service != "" {
			fmt.Printf(" for service %s", service)
		}
		if pattern != "" {
			fmt.Printf(" with pattern %s", pattern)
		}
		fmt.Printf("\n")
		return 0
	}

	fmt.Printf("📚 Runbooks (%d found):\n\n", len(runbooks))
	
	for _, runbook := range runbooks {
		status := "📝 Draft"
		if runbook.Published {
			status = "✅ Published"
		}
		
		fmt.Printf("%s %s\n", status, runbook.ID)
		fmt.Printf("   Title: %s\n", runbook.Title)
		fmt.Printf("   Service: %s\n", runbook.Service)
		if runbook.Component != "" {
			fmt.Printf("   Component: %s\n", runbook.Component)
		}
		fmt.Printf("   Pattern: %s\n", runbook.Pattern)
		fmt.Printf("   Severity: %s\n", runbook.Severity)
		fmt.Printf("   Created: %s\n", runbook.CreatedAt.Format("2006-01-02 15:04"))
		if len(runbook.Tags) > 0 {
			fmt.Printf("   Tags: %s\n", strings.Join(runbook.Tags, ", "))
		}
		fmt.Printf("\n")
	}

	return 0
}

// HandleSaveCommand handles runbook saving commands
func (rc *RunbookCmd) HandleSaveCommand(args []string) int {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Error: runbook ID required\n")
		fmt.Fprintf(os.Stderr, "Usage: health-monitor runbook save <runbook-id>\n")
		return 1
	}

	// Load runbook (it should exist in memory or be regenerated)
	// For now, we'll need to regenerate it from the pattern
	// In a real implementation, we'd have a memory cache or temporary storage
	
	fmt.Fprintf(os.Stderr, "Error: Runbook saving from memory not implemented yet\n")
	fmt.Fprintf(os.Stderr, "Please use 'health-monitor runbook generate --save' to create and save in one step\n")
	
	return 1
}

// HandleTestCommand handles runbook testing commands
func (rc *RunbookCmd) HandleTestCommand(args []string) int {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Error: runbook ID required\n")
		fmt.Fprintf(os.Stderr, "Usage: health-monitor runbook test <runbook-id>\n")
		return 1
	}

	runbookID := args[0]
	
	// Load runbook
	runbook, err := rc.store.Get(runbookID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading runbook: %v\n", err)
		return 1
	}

	fmt.Printf("🧪 Testing Runbook: %s\n\n", runbook.Title)
	fmt.Printf("Service: %s\n", runbook.Service)
	fmt.Printf("Pattern: %s\n", runbook.Pattern)
	fmt.Printf("Steps: %d\n\n", len(runbook.Steps))

	// Test each step
	for i, step := range runbook.Steps {
		fmt.Printf("Step %d: %s\n", i+1, step.Title)
		if step.Command != "" {
			fmt.Printf("  Command: %s\n", step.Command)
			fmt.Printf("  Status: ⏳ Ready to execute\n")
		}
		if step.Critical {
			fmt.Printf("  Priority: 🔴 Critical\n")
		} else {
			fmt.Printf("  Priority: 🟡 Optional\n")
		}
		fmt.Printf("\n")
	}

	fmt.Printf("📊 Test Summary:\n")
	fmt.Printf("  Total Steps: %d\n", len(runbook.Steps))
	fmt.Printf("  Critical Steps: %d\n", func() int {
		count := 0
		for _, step := range runbook.Steps {
			if step.Critical {
				count++
			}
		}
		return count
	}())
	fmt.Printf("  Metrics to Monitor: %d\n", len(runbook.Metrics))
	fmt.Printf("  Log Queries: %d\n", len(runbook.LogQueries))

	return 0
}

// HandleStatsCommand handles runbook statistics commands
func (rc *RunbookCmd) HandleStatsCommand(args []string) int {
	stats, err := rc.store.GetStats()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting stats: %v\n", err)
		return 1
	}

	fmt.Printf("📊 Runbook Statistics\n\n")
	fmt.Printf("Total Runbooks: %d\n", stats.TotalRunbooks)
	fmt.Printf("Published Runbooks: %d\n", stats.PublishedRunbooks)
	
	if len(stats.Services) > 0 {
		fmt.Printf("\n📈 Top Services:\n")
		for service, count := range stats.Services {
			fmt.Printf("  %s: %d runbooks\n", service, count)
		}
	}
	
	if len(stats.Patterns) > 0 {
		fmt.Printf("\n🔍 Top Patterns:\n")
		for pattern, count := range stats.Patterns {
			fmt.Printf("  %s: %d runbooks\n", strings.ReplaceAll(pattern, "_", " "), count)
		}
	}
	
	if len(stats.Categories) > 0 {
		fmt.Printf("\n📂 Categories:\n")
		for category, count := range stats.Categories {
			fmt.Printf("  %s: %d runbooks\n", category, count)
		}
	}

	return 0
}

// ExportRunbook exports a runbook in different formats
func (rc *RunbookCmd) ExportRunbook(runbookID, format string) (string, error) {
	runbook, err := rc.store.Get(runbookID)
	if err != nil {
		return "", err
	}

	switch format {
	case "json":
		data, err := json.MarshalIndent(runbook, "", "  ")
		if err != nil {
			return "", err
		}
		return string(data), nil
	case "markdown":
		return runbook.Content, nil
	default:
		return "", fmt.Errorf("unsupported export format: %s", format)
	}
}
