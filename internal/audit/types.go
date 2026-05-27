package audit

import (
	"fmt"
	"strings"
)

type ActionItemStatus string

const (
	ActionItemTODO        ActionItemStatus = "TODO"
	ActionItemInProgress  ActionItemStatus = "IN_PROGRESS"
	ActionItemDone        ActionItemStatus = "DONE"
	ActionItemWontFix     ActionItemStatus = "WONT_FIX"
)

type ActionItemPriority string

const (
	ActionPriorityP1 ActionItemPriority = "P1"
	ActionPriorityP2 ActionItemPriority = "P2"
	ActionPriorityP3 ActionItemPriority = "P3"
	ActionPriorityP4 ActionItemPriority = "P4"
)

func ParseActionItemPriority(value string) (ActionItemPriority, error) {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	if normalized == "" {
		return ActionPriorityP2, nil
	}
	switch ActionItemPriority(normalized) {
	case ActionPriorityP1, ActionPriorityP2, ActionPriorityP3, ActionPriorityP4:
		return ActionItemPriority(normalized), nil
	default:
		return "", fmt.Errorf("invalid action item priority %q (use P1, P2, P3, or P4)", value)
	}
}

func ParseActionItemStatus(value string) (ActionItemStatus, error) {
	normalized := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(value), "-", "_"))
	if normalized == "" {
		return ActionItemTODO, nil
	}
	switch ActionItemStatus(normalized) {
	case ActionItemTODO, ActionItemInProgress, ActionItemDone, ActionItemWontFix:
		return ActionItemStatus(normalized), nil
	default:
		return "", fmt.Errorf("invalid action item status %q (use TODO, IN_PROGRESS, DONE, or WONT_FIX)", value)
	}
}

// IncidentAnalysis represents structured RCA metadata for incident classification
type IncidentAnalysis struct {
	Component      string          `json:"component,omitempty"`       // e.g., "postgres_db", "redis_cache"
	Dependency     string          `json:"dependency,omitempty"`      // e.g., "billing_api->postgres_db"
	Pattern        string          `json:"pattern,omitempty"`         // Auto-extracted from logs
	Category       string          `json:"category,omitempty"`        // capacity, latency, dependency, config, infra, deployment, security
	RootCause      string          `json:"root_cause,omitempty"`      // Primary cause description
	FixSummary     string          `json:"fix_summary,omitempty"`     // What was done to resolve
	FailureType    string          `json:"failure_type,omitempty"`    // service, dependency, infra
	ErrorSignature string          `json:"error_signature,omitempty"` // Normalized error pattern
	Prevention     string          `json:"prevention,omitempty"`      // Prevention measures to avoid recurrence
	LessonsLearned *LessonsLearned `json:"lessons_learned,omitempty"` // Blameless postmortem details
}

// LessonsLearned captures blameless postmortem insights
type LessonsLearned struct {
	WhatWentWell      string `json:"what_went_well,omitempty"`
	WhatCouldBeBetter string `json:"what_could_be_better,omitempty"`
	WhereWeGotLucky   string `json:"where_we_got_lucky,omitempty"`
}

// ImpactMetrics captures key performance indicators for an incident
type ImpactMetrics struct {
	EstimatedDowntimeMinutes int      `json:"estimated_downtime_minutes,omitempty"`
	ImpactedFlows            []string `json:"impacted_flows,omitempty"`
	CustomMetrics            string   `json:"custom_metrics,omitempty"` // e.g. "Revenue lost: $5k"
}
