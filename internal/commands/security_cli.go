package commands

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"health-monitor/internal/config"
)

// HandleSecurityCommand handles security-related CLI commands
func HandleSecurityCommand(args []string) int {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Error: security subcommand required\n")
		fmt.Fprintf(os.Stderr, "Usage: health-monitor security <validate-config|status|test|generate-key>\n")
		return 1
	}

	securityCmd := NewSecurityCmd()

	switch args[0] {
	case "validate-config":
		return handleSecurityValidateConfig(securityCmd, args[1:])
	case "status":
		return handleSecurityStatus(securityCmd, args[1:])
	case "test":
		return handleSecurityTest(securityCmd, args[1:])
	case "generate-key":
		return handleSecurityGenerateKey(securityCmd, args[1:])
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown security subcommand '%s'\n", args[0])
		fmt.Fprintf(os.Stderr, "Usage: health-monitor security <validate-config|status|test|generate-key>\n")
		return 1
	}
}

func handleSecurityValidateConfig(cmd *SecurityCmd, args []string) int {
	fs := flag.NewFlagSet("security validate-config", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	profile := fs.String("profile", "", "Profile name (defaults to active)")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	profileName := *profile
	if profileName == "" {
		profileName = config.GetActiveProfileName()
	}

	if err := cmd.ValidateConfig(profileName); err != nil {
		fmt.Fprintf(os.Stderr, "Security configuration validation failed: %v\n", err)
		return 1
	}

	return 0
}

func handleSecurityStatus(cmd *SecurityCmd, args []string) int {
	fs := flag.NewFlagSet("security status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	profile := fs.String("profile", "", "Profile name (defaults to active)")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	profileName := *profile
	if profileName == "" {
		profileName = config.GetActiveProfileName()
	}

	if err := cmd.Status(profileName); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get security status: %v\n", err)
		return 1
	}

	return 0
}

func handleSecurityTest(cmd *SecurityCmd, args []string) int {
	fs := flag.NewFlagSet("security test", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	profile := fs.String("profile", "", "Profile name (defaults to active)")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	profileName := *profile
	if profileName == "" {
		profileName = config.GetActiveProfileName()
	}

	if err := cmd.TestSecurity(profileName); err != nil {
		fmt.Fprintf(os.Stderr, "Security tests failed: %v\n", err)
		return 1
	}

	return 0
}

func handleSecurityGenerateKey(cmd *SecurityCmd, args []string) int {
	fs := flag.NewFlagSet("security generate-key", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	outputFile := fs.String("output", "", "Output file for encryption key")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	// Set default output file if not specified
	if *outputFile == "" {
		*outputFile = "/etc/health-monitor/encryption.key"
	}

	// Convert to absolute path
	if !filepath.IsAbs(*outputFile) {
		wd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get working directory: %v\n", err)
			return 1
		}
		*outputFile = filepath.Join(wd, *outputFile)
	}

	if err := cmd.GenerateKey(*outputFile); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to generate encryption key: %v\n", err)
		return 1
	}

	return 0
}
