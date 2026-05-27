package runbook

import (
	"fmt"
	"os"
	"strings"

	"health-monitor/internal/config"
	"health-monitor/internal/incident"
)

// HandleRunbookCommand handles runbook-related CLI commands
func HandleRunbookCommand(args []string, incidentStore *incident.Store) int {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
		return 1
	}

	// Create runbook command handler
	runbookCmd, err := NewRunbookCmd(incidentStore, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing runbook command: %v\n", err)
		return 1
	}

	if len(args) == 0 {
		printRunbookUsage()
		return 1
	}

	command := args[0]
	var commandArgs []string
	if len(args) > 1 {
		commandArgs = args[1:]
	}

	switch command {
	case "suggest":
		return runbookCmd.HandleSuggestCommand(commandArgs)
	case "patterns":
		return runbookCmd.HandlePatternsCommand(commandArgs)
	case "generate":
		return runbookCmd.HandleGenerateCommand(commandArgs)
	case "list":
		return runbookCmd.HandleListCommand(commandArgs)
	case "save":
		return runbookCmd.HandleSaveCommand(commandArgs)
	case "test":
		return runbookCmd.HandleTestCommand(commandArgs)
	case "stats":
		return runbookCmd.HandleStatsCommand(commandArgs)
	case "export":
		return handleExportCommand(runbookCmd, commandArgs)
	case "help", "--help", "-h":
		printRunbookUsage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "Error: Unknown runbook command: %s\n\n", command)
		printRunbookUsage()
		return 1
	}
}

// handleExportCommand handles runbook export commands
func handleExportCommand(runbookCmd *RunbookCmd, args []string) int {
	if len(args) < 2 {
		fmt.Fprintf(os.Stderr, "Error: runbook ID and format required\n")
		fmt.Fprintf(os.Stderr, "Usage: health-monitor runbook export <runbook-id> <format>\n")
		fmt.Fprintf(os.Stderr, "Formats: json, markdown\n")
		return 1
	}

	runbookID := args[0]
	format := args[1]

	content, err := runbookCmd.ExportRunbook(runbookID, format)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error exporting runbook: %v\n", err)
		return 1
	}

	fmt.Print(content)
	return 0
}

// printRunbookUsage prints usage information for runbook commands
func printRunbookUsage() {
	fmt.Printf("Runbook Suggestion System\n\n")
	fmt.Printf("Usage: health-monitor runbook <command> [options]\n\n")
	fmt.Printf("Commands:\n")
	fmt.Printf("  suggest <incident-id>          Suggest runbook for an incident\n")
	fmt.Printf("  patterns [--service <svc>] [--days <n>]    Analyze incident patterns\n")
	fmt.Printf("  generate --incident <id>      Generate runbook from incident\n")
	fmt.Printf("  generate --pattern <p> --service <s>  Generate from pattern\n")
	fmt.Printf("           [--save] [--publish]                      \n")
	fmt.Printf("  list [--service <svc>] [--pattern <p>] [--limit <n>]  List runbooks\n")
	fmt.Printf("  save <runbook-id> [--publish] Save runbook to storage\n")
	fmt.Printf("  test <runbook-id>            Test runbook steps\n")
	fmt.Printf("  stats                        Show runbook statistics\n")
	fmt.Printf("  export <id> <format>         Export runbook (json, markdown)\n")
	fmt.Printf("  help                         Show this help\n\n")
	
	fmt.Printf("Examples:\n")
	fmt.Printf("  # Suggest runbook for current incident\n")
	fmt.Printf("  health-monitor runbook suggest INC-20260220-041706\n\n")
	
	fmt.Printf("  # Analyze patterns for billing service\n")
	fmt.Printf("  health-monitor runbook patterns --service billing_api --days 30\n\n")
	
	fmt.Printf("  # Generate runbook from incident\n")
	fmt.Printf("  health-monitor runbook generate --incident INC-20260220-041706 --save\n\n")
	
	fmt.Printf("  # Generate runbook from pattern\n")
	fmt.Printf("  health-monitor runbook generate --pattern query_timeout --service billing_api --save --publish\n\n")
	
	fmt.Printf("  # List all runbooks\n")
	fmt.Printf("  health-monitor runbook list\n\n")
	
	fmt.Printf("  # Test a runbook\n")
	fmt.Printf("  health-monitor runbook test rb-20260220-billing_api-query_timeout\n\n")
	
	fmt.Printf("Configuration:\n")
	fmt.Printf("  Runbook features are disabled by default. Enable in config:\n")
	fmt.Printf("  runbook_suggestions:\n")
	fmt.Printf("    enabled: true\n")
	fmt.Printf("    confidence_threshold: 0.7\n")
	fmt.Printf("    output_directory: \"/var/lib/health-monitor/runbooks\"\n\n")
}

// ValidateRunbookConfig validates runbook configuration
func ValidateRunbookConfig(cfg config.Config) error {
	if err := cfg.RunbookSuggestions.Validate(); err != nil {
		return fmt.Errorf("runbook configuration validation failed: %w", err)
	}
	return nil
}

// GetRunbookStatus returns current runbook system status
func GetRunbookStatus(cfg config.Config) string {
	var status strings.Builder
	
	status.WriteString("📚 Runbook System Status:\n")
	
	if cfg.RunbookSuggestions.Enabled {
		status.WriteString("   Status: ✅ Enabled\n")
	} else {
		status.WriteString("   Status: ❌ Disabled\n")
	}
	
	status.WriteString(fmt.Sprintf("   Confidence Threshold: %.1f%%\n", cfg.RunbookSuggestions.ConfidenceThreshold*100))
	status.WriteString(fmt.Sprintf("   Output Directory: %s\n", cfg.RunbookSuggestions.OutputDirectory))
	status.WriteString(fmt.Sprintf("   Output Format: %s\n", cfg.RunbookSuggestions.OutputFormat))
	
	if len(cfg.RunbookSuggestions.EnabledCategories) > 0 {
		status.WriteString(fmt.Sprintf("   Enabled Categories: %s\n", strings.Join(cfg.RunbookSuggestions.EnabledCategories, ", ")))
	}
	
	if len(cfg.RunbookSuggestions.DisabledServices) > 0 {
		status.WriteString(fmt.Sprintf("   Disabled Services: %s\n", strings.Join(cfg.RunbookSuggestions.DisabledServices, ", ")))
	}
	
	if cfg.RunbookSuggestions.AutoGenerate {
		status.WriteString("   Auto-Generate: ✅ Enabled\n")
	} else {
		status.WriteString("   Auto-Generate: ❌ Disabled\n")
	}
	
	if cfg.RunbookSuggestions.PublishToWiki {
		status.WriteString(fmt.Sprintf("   Wiki Publishing: ✅ Enabled (%s)\n", cfg.RunbookSuggestions.WikiURL))
	} else {
		status.WriteString("   Wiki Publishing: ❌ Disabled\n")
	}
	
	return status.String()
}
