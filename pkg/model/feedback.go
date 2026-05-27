package model

import (
	"time"
)

// FeedbackCategory represents a specific area of the agent
type FeedbackCategory string

const (
	CategoryReliability FeedbackCategory = "reliability_alerts"
	CategoryDiagnostics FeedbackCategory = "diagnostics_slos"
	CategoryAutomation  FeedbackCategory = "automation_flows"
	CategoryUI          FeedbackCategory = "ui_ux"
	CategoryOverall     FeedbackCategory = "overall"
)

// FeatureFeedback holds rating and notes for a specific category
type FeatureFeedback struct {
	Rating int    `json:"rating"` // 1-5
	Notes  string `json:"notes"`
}

// Feedback represents the complete feedback payload
type Feedback struct {
	ID        string                     `json:"id"`         // Unique ID for this specific feedback entry
	MachineID string                     `json:"machine_id"` // Unique identifier for the user's system
	Timestamp time.Time                  `json:"timestamp"`
	Version   string                     `json:"version"`
	OS        string                     `json:"os"`
	Profile   string                     `json:"profile"`
	Features  map[FeedbackCategory]FeatureFeedback `json:"features"`
}
