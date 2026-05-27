package tui

import (
	"fmt"
	"strconv"
	"strings"

	"health-monitor/internal/incident"
)

// showDemoIncidentBrowser shows the demo incidents in a proper TUI screen
func showDemoIncidentBrowser() error {
	// Clear screen and hide cursor
	fmt.Print("\033[2J\033[H\033[?25l")
	defer fmt.Print("\033[?25h") // Show cursor on exit
	
	// Load demo incidents
	service, err := incident.NewService(nil)
	if err != nil {
		return fmt.Errorf("failed to create incident service: %v", err)
	}
	
	incidents, _, err := service.List()
	if err != nil {
		return fmt.Errorf("failed to load incidents: %v", err)
	}
	
	selectedIndex := 0
	showDetails := false
	
	for {
		// Clear screen and position cursor at top
		fmt.Print("\033[2J\033[H")
		
		// Draw header
		fmt.Printf("┌─────────────────────────────────────────────────────────────┐\n")
		fmt.Printf("│                    🎮 Demo Incident Browser                 │\n")
		fmt.Printf("└─────────────────────────────────────────────────────────────┘\n\n")
		
		if !showDetails {
			// Show incident list
			fmt.Printf("📋 Demo Incidents (%d total):\n\n", len(incidents))
			
			for i, incident := range incidents {
				// Highlight selected incident
				if i == selectedIndex {
					fmt.Printf("▶ ")
				} else {
					fmt.Printf("  ")
				}
				
				// Status icon
				status := "🔴"
				if incident.State == "resolved" {
					status = "🟢"
				}
				
				fmt.Printf("%d. %s %s\n", i+1, status, incident.Title)
				fmt.Printf("     Service: %s | Severity: %s\n", incident.Service, incident.Severity)
				fmt.Printf("     %s\n\n", incident.Summary)
			}
			
			// Show controls
			fmt.Printf("┌─────────────────────────────────────────────────────────────┐\n")
			fmt.Printf("│ Controls: ↑/k Up | ↓/j Down | Enter Details | h Help | q Quit │\n")
			fmt.Printf("└─────────────────────────────────────────────────────────────┘\n")
		} else {
			// Show incident details
			showDetailedViewForDemo(incidents[selectedIndex], selectedIndex+1)
			
			// Show controls for detail view
			fmt.Printf("┌─────────────────────────────────────────────────────────────┐\n")
			fmt.Printf("│ Controls: Enter Back to List | h Help | q Quit              │\n")
			fmt.Printf("└─────────────────────────────────────────────────────────────┘\n")
		}
		
		// Read input with proper handling
		fmt.Printf("\n> ")
		var input string
		fmt.Scanln(&input)
		
		// Handle input
		if !showDetails {
			switch input {
			case "q":
				fmt.Print("\033[2J\033[H") // Clear screen
				fmt.Printf("👋 Exiting demo mode...\n")
				return nil
			case "h":
				showHelpScreenForDemo()
			case "":
				// Enter key - show details
				showDetails = true
			case "k":
				// Up arrow alternative
				if selectedIndex > 0 {
					selectedIndex--
				}
			case "j":
				// Down arrow alternative
				if selectedIndex < len(incidents)-1 {
					selectedIndex++
				}
			case "1", "2", "3", "4", "5", "6":
				// Number selection
				if incidentNum, err := strconv.Atoi(input); err == nil && incidentNum >= 1 && incidentNum <= len(incidents) {
					selectedIndex = incidentNum - 1
					showDetails = true
				}
			}
		} else {
			// Detail view controls
			switch input {
			case "q":
				fmt.Print("\033[2J\033[H") // Clear screen
				fmt.Printf("👋 Exiting demo mode...\n")
				return nil
			case "h":
				showHelpScreenForDemo()
			case "":
				// Enter key - back to list
				showDetails = false
			}
		}
	}
}

// showDetailedViewForDemo displays detailed incident information in TUI format
func showDetailedViewForDemo(incident incident.Incident, num int) {
	fmt.Printf("┌─────────────────────────────────────────────────────────────┐\n")
	fmt.Printf("│                    🔍 Incident #%d Details                  │\n", num)
	fmt.Printf("└─────────────────────────────────────────────────────────────┘\n\n")
	
	status := "🔴 OPEN"
	if incident.State == "resolved" {
		status = "🟢 RESOLVED"
	}
	
	fmt.Printf("Status: %s\n", status)
	fmt.Printf("Title: %s\n", incident.Title)
	fmt.Printf("Service: %s\n", incident.Service)
	fmt.Printf("Severity: %s\n", incident.Severity)
	fmt.Printf("Created: %s\n", incident.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Updated: %s\n", incident.UpdatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("\nSummary:\n%s\n\n", incident.Summary)
	
	// Show Analysis if available
	if incident.Analysis != nil {
		fmt.Printf("🔍 Root Cause Analysis:\n")
		if incident.Analysis.RootCause != "" {
			fmt.Printf("  • Root Cause: %s\n", incident.Analysis.RootCause)
		}
		if incident.Analysis.Component != "" {
			fmt.Printf("  • Component: %s\n", incident.Analysis.Component)
		}
		if incident.Analysis.Category != "" {
			fmt.Printf("  • Category: %s\n", incident.Analysis.Category)
		}
		if incident.Analysis.FixSummary != "" {
			fmt.Printf("  • Fix Summary: %s\n", incident.Analysis.FixSummary)
		}
		fmt.Printf("\n")
	}
	
	// Show Action Items if available
	if len(incident.ActionItems) > 0 {
		fmt.Printf("⚡ Action Items:\n")
		for i, action := range incident.ActionItems {
			statusIcon := "⏳"
			if action.Status == "DONE" {
				statusIcon = "✅"
			}
			fmt.Printf("  %d. %s %s\n", i+1, statusIcon, action.Description)
			fmt.Printf("     Owner: %s | Priority: %s\n", action.Owner, action.Priority)
		}
		fmt.Printf("\n")
	}
	
	// Show Impact if available
	if incident.Impact != nil {
		fmt.Printf("📊 Impact Analysis:\n")
		if incident.Impact.EstimatedDowntimeMinutes > 0 {
			fmt.Printf("  • Estimated Downtime: %d minutes\n", incident.Impact.EstimatedDowntimeMinutes)
		}
		if len(incident.Impact.ImpactedFlows) > 0 {
			fmt.Printf("  • Impacted Flows: %s\n", strings.Join(incident.Impact.ImpactedFlows, ", "))
		}
		if incident.Impact.CustomMetrics != "" {
			fmt.Printf("  • Custom Metrics: %s\n", incident.Impact.CustomMetrics)
		}
		fmt.Printf("\n")
	}
}

// showHelpScreenForDemo displays help in TUI format
func showHelpScreenForDemo() {
	fmt.Print("\033[2J\033[H") // Clear screen
	
	fmt.Printf("┌─────────────────────────────────────────────────────────────┐\n")
	fmt.Printf("│                        📖 Help Screen                         │\n")
	fmt.Printf("└─────────────────────────────────────────────────────────────┘\n\n")
	
	fmt.Printf("This demo showcases the incident management system with:\n\n")
	
	fmt.Printf("🎯 Features:\n")
	fmt.Printf("  • Incident detection and tracking\n")
	fmt.Printf("  • RCA (Root Cause Analysis)\n")
	fmt.Printf("  • Action items and remediation\n")
	fmt.Printf("  • Impact analysis and correlation\n")
	fmt.Printf("  • Profile-based flow detection\n")
	fmt.Printf("  • Similar incident identification\n\n")
	
	fmt.Printf("📊 Services in demo:\n")
	fmt.Printf("  • checkout - Payment processing\n")
	fmt.Printf("  • auth - Authentication service\n")
	fmt.Printf("  • billing - Billing service\n\n")
	
	fmt.Printf("🔧 For full experience with all features:\n")
	fmt.Printf("  sudo ./health-monitor demo --force-cli\n\n")
	
	fmt.Printf("Press Enter to continue...")
	fmt.Scanln()
}
