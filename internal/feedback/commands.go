package feedback

import (
	"health-monitor/internal/output"
)

// HandleFeedbackCLI manages the interactive feedback collection process
func HandleFeedbackCLI(args []string) int {
	if err := RunTUIFeedback(); err != nil {
		output.Errorf("Feedback TUI failed: %v", err)
		return 1
	}
	return 0
}
