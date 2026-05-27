package tui

import (
	"health-monitor/pkg/model"
)

// Print prints the TUI using the interactive scrollable interface
func Print(report model.Report) {
	PrintTUI(report)
}

// PrintTUI prints the TUI using the interactive scrollable interface
func PrintTUI(report model.Report) {
	// Use interactive scrollable TUI
	PrintInteractiveTUI(report)
}
