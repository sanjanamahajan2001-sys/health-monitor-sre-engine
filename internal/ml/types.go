package ml

import (
	"health-monitor/internal/logs"
	"health-monitor/internal/traces"
	"time"
)

// LessonsLearned captures constructive feedback for blameless postmortems
type LessonsLearned struct {
	WhatWentWell      string `json:"what_went_well,omitempty"`
	WhatCouldBeBetter string `json:"what_could_be_better,omitempty"`
	WhereWeGotLucky   string `json:"where_we_got_lucky,omitempty"`
}

// Prediction represents a semantic hint or a detected system pattern
type Prediction struct {
	ID             string            `json:"id"`
	Pattern        string            `json:"pattern"`
	Category       string            `json:"category"`
	Component      string            `json:"component"`
	Confidence     float64           `json:"confidence"`
	Explanation    string            `json:"explanation"`
	ResolutionHint string            `json:"resolution_hint,omitempty"`
	RootCause      string            `json:"root_cause,omitempty"`
	Prevention     string            `json:"prevention,omitempty"`
	FixSummary     string            `json:"fix_summary,omitempty"`
	LessonsLearned *LessonsLearned   `json:"lessons_learned,omitempty"`
	ActionItems    []string          `json:"action_items,omitempty"`
	ModelID        string            `json:"model_id"`
	Timestamp      time.Time         `json:"timestamp"`
	Notified       bool              `json:"notified"`
	Snapshot       LifecycleSnapshot `json:"snapshot,omitempty"`
}

// ModelMetadata stores info about a trained model
type ModelMetadata struct {
	ID        string    `json:"id"`
	Version   string    `json:"version"`
	Accuracy  float64   `json:"accuracy"`
	Samples   int       `json:"samples"`
	CreatedAt time.Time `json:"created_at"`
	Features  []string  `json:"features"`
}

// Vector represents a numerical representation of text or state
type Vector []float64

// LifecycleSnapshot captures the system state at a point in time for incident forecasting
type LifecycleSnapshot struct {
	Timestamp      time.Time         `json:"timestamp"`
	Service        string            `json:"service,omitempty"`
	IncidentID     string            `json:"incident_id,omitempty"`
	Phase          string            `json:"phase"` // "pre", "during", "post", "steady"
	Metrics        map[string]float64 `json:"metrics,omitempty"`
	LogSignatures  []string          `json:"log_signatures,omitempty"`
	FullLogs       *logs.LogSummary  `json:"full_logs,omitempty"`
	FullTraces     *traces.TraceSummary `json:"full_traces,omitempty"`
	K8sEvents      []string          `json:"k8s_events,omitempty"`
	Vector         Vector            `json:"vector,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// FullLifecycle captures the end-to-end data for an incident
type FullLifecycle struct {
	IncidentID string              `json:"incident_id"`
	Snapshots  []LifecycleSnapshot `json:"snapshots"`
	ResolvedAt time.Time           `json:"resolved_at,omitempty"`
}
