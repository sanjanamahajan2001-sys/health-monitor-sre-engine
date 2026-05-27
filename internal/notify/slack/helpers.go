package slack

import (
	"fmt"
	"time"
)

// Helper functions for enhanced notifications

// getLastUserByEventType finds the user who performed the last event of a specific type
func getLastUserByEventType(events []IncidentEventDetail, eventType string) string {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type == eventType {
			return events[i].User
		}
	}
	return ""
}

// getRecentNotes extracts the most recent note events
func getRecentNotes(events []IncidentEventDetail, limit int) []IncidentEventDetail {
	var notes []IncidentEventDetail
	for i := len(events) - 1; i >= 0 && len(notes) < limit; i-- {
		if events[i].Type == "note" && events[i].Message != "" {
			notes = append(notes, events[i])
		}
	}
	// Reverse to show chronological order
	for i, j := 0, len(notes)-1; i < j; i, j = i+1, j-1 {
		notes[i], notes[j] = notes[j], notes[i]
	}
	return notes
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d seconds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	}
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	if minutes == 0 {
		return fmt.Sprintf("%d hours", hours)
	}
	return fmt.Sprintf("%dh %dm", hours, minutes)
}

// truncateString truncates a string to maxLen characters
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
