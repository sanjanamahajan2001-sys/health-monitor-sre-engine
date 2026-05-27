package incident

import (
	"fmt"
	"os"
	"strings"
	"time"

	"health-monitor/internal/audit"
	"health-monitor/internal/logs"
	"health-monitor/internal/ml"
	"health-monitor/internal/traces"
)

// ErrorStat is a type alias for logs.ErrorStat to avoid breaking existing code
type ErrorStat = logs.ErrorStat

// LogSummary is a type alias for logs.LogSummary to avoid breaking existing code
type LogSummary = logs.LogSummary

// LessonsLearned captures constructive feedback for blameless postmortems
// ActionItemStatus represents the status of an action item
// Use types from audit package
type ActionItemStatus = audit.ActionItemStatus
type ActionItemPriority = audit.ActionItemPriority

// Use types from audit package
type IncidentAnalysis = audit.IncidentAnalysis
type LessonsLearned = audit.LessonsLearned
type ImpactMetrics = audit.ImpactMetrics

const (
	ActionItemTODO       = audit.ActionItemTODO
	ActionItemInProgress = audit.ActionItemInProgress
	ActionItemDone       = audit.ActionItemDone
	ActionItemWontFix    = audit.ActionItemWontFix

	ActionPriorityP1 = audit.ActionPriorityP1
	ActionPriorityP2 = audit.ActionPriorityP2
	ActionPriorityP3 = audit.ActionPriorityP3
	ActionPriorityP4 = audit.ActionPriorityP4
)

// ActionItem represents a post-incident action item to prevent recurrence
type ActionItem struct {
	ID          string             `json:"id"`
	Description string             `json:"description"`
	Owner       string             `json:"owner"`
	Priority    ActionItemPriority `json:"priority"`
	Status      ActionItemStatus   `json:"status"`
	DueDate     time.Time          `json:"due_date,omitempty"`
	CreatedAt   time.Time          `json:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at"`
}

// ToilMetadata captures manual operational work spent on an incident
type ToilMetadata struct {
	Minutes  int    `json:"minutes"`
	Category string `json:"category"`
}

type Incident struct {
	ID          string               `json:"id"`
	Service     string               `json:"service"`
	Severity    Severity             `json:"severity"`
	Title       string               `json:"title"`
	State       State                `json:"state"`
	Profile     string               `json:"profile"`
	CreatedAt   time.Time            `json:"created_at"`
	UpdatedAt   time.Time            `json:"updated_at"`
	Summary     string               `json:"summary,omitempty"`
	Events      []Event              `json:"events"`
	Links       ObservabilityLinks   `json:"links,omitempty"`
	Logs        *logs.LogSummary          `json:"logs,omitempty"`
	Traces      *traces.TraceSummary `json:"traces,omitempty"`
	Analysis    *IncidentAnalysis    `json:"analysis,omitempty"`
	Impact      *ImpactMetrics       `json:"impact,omitempty"`
	Toil        *ToilMetadata        `json:"toil,omitempty"`
	ActionItems []ActionItem         `json:"action_items,omitempty"`
	Metadata    map[string]string    `json:"metadata,omitempty"`
	LifecycleSnapshots []ml.LifecycleSnapshot `json:"lifecycle_snapshots,omitempty"`
	Cluster            string               `json:"cluster,omitempty"`   // New field: Cluster name
	Namespace          string               `json:"namespace,omitempty"` // New field: Kubernetes namespace
	Deployment         string               `json:"deployment,omitempty"`// New field: Kubernetes deployment name
	K8sContext         string               `json:"k8s_context,omitempty"`// New field: Kubernetes context
}

// IsResolved returns true if the incident is in a terminal state
func (i Incident) IsResolved() bool {
	return i.State.IsTerminal()
}

type Event struct {
	Timestamp time.Time `json:"timestamp"`
	User      string    `json:"user"`
	Type      EventType `json:"type"`
	Message   string    `json:"message,omitempty"`
}

type Severity string

const (
	P1 Severity = "P1"
	P2 Severity = "P2"
	P3 Severity = "P3"
	P4 Severity = "P4"
)

func ParseSeverity(value string) (Severity, error) {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	switch Severity(normalized) {
	case P1, P2, P3, P4:
		return Severity(normalized), nil
	default:
		return "", fmt.Errorf("invalid severity %q (use P1, P2, P3, or P4)", value)
	}
}

type State string

const (
	StateSuggested    State = "Suggested"
	StateStarted      State = "Started"
	StateAcknowledged State = "Acknowledged"
	StateResolved     State = "Resolved"
)

func (s State) IsTerminal() bool {
	return s == StateResolved
}

type EventType string

const (
	EventStart    EventType = "start"
	EventNote     EventType = "note"
	EventAck      EventType = "ack"
	EventResolve  EventType = "resolve"
	EventSuggest  EventType = "suggest"
	EventSeverity EventType = "severity"
)

func SeverityRank(severity Severity) int {
	switch severity {
	case P1:
		return 4
	case P2:
		return 3
	case P3:
		return 2
	case P4:
		return 1
	default:
		return 0
	}
}

func IsHigherSeverity(next Severity, current Severity) bool {
	return SeverityRank(next) > SeverityRank(current)
}

func SystemUserLabel() string {
	user := strings.TrimSpace(getEnvUser())
	if user == "" {
		user = "unknown"
	}
	sudoUser := strings.TrimSpace(getEnvSudoUser())
	if sudoUser != "" && sudoUser != user {
		return fmt.Sprintf("%s, sudo: %s", user, sudoUser)
	}
	return user
}

var getEnvUser = func() string {
	return os.Getenv("USER")
}

var getEnvSudoUser = func() string {
	return os.Getenv("SUDO_USER")
}

func ComposeUser(provided string) string {
	trimmed := strings.TrimSpace(provided)
	if trimmed == "" {
		return SystemUserLabel()
	}
	return fmt.Sprintf("%s (system: %s)", trimmed, SystemUserLabel())
}

// ValidCategories defines allowed incident categories for RCA
var ValidCategories = []string{
	"capacity",
	"latency",
	"dependency",
	"configuration",
	"infra",
	"deployment",
	"security",
	"unknown",
}

// ValidFailureTypes defines allowed failure types for RCA
var ValidFailureTypes = []string{
	"service",
	"dependency",
	"infra",
}

// ValidToilCategories defines allowed categories for manual operational work
var ValidToilCategories = []string{
	"manual_restart",
	"alert_ack",
	"config_change",
	"investigation",
	"escalation",
	"capacity_mgmt",
	"other",
}

// ValidateCategory checks if the provided category is valid
func ValidateCategory(category string) error {
	if category == "" {
		return nil // Optional field
	}
	normalized := strings.ToLower(strings.TrimSpace(category))
	for _, valid := range ValidCategories {
		if normalized == valid {
			return nil
		}
	}
	return fmt.Errorf("invalid category %q. Allowed: %s", category, strings.Join(ValidCategories, ", "))
}

// ValidateFailureType checks if the provided failure type is valid
func ValidateFailureType(failureType string) error {
	if failureType == "" {
		return nil // Optional field
	}
	normalized := strings.ToLower(strings.TrimSpace(failureType))
	for _, valid := range ValidFailureTypes {
		if normalized == valid {
			return nil
		}
	}
	return fmt.Errorf("invalid failure type %q. Allowed: %s", failureType, strings.Join(ValidFailureTypes, ", "))
}

// ValidateToilCategory checks if the provided toil category is valid
func ValidateToilCategory(category string) error {
	if category == "" {
		return nil
	}
	normalized := strings.ToLower(strings.TrimSpace(category))
	for _, valid := range ValidToilCategories {
		if normalized == valid {
			return nil
		}
	}
	return fmt.Errorf("invalid toil category %q. Allowed: %s", category, strings.Join(ValidToilCategories, ", "))
}

// ValidActionItemStatuses defines allowed action item statuses
var ValidActionItemStatuses = []string{
	"TODO",
	"IN_PROGRESS",
	"DONE",
	"WONT_FIX",
}

// GenerateActionItemID generates a unique action item ID
// Format: ACT-<incident-id>-<sequence>
func GenerateActionItemID(incidentID string, sequence int) string {
	return fmt.Sprintf("ACT-%s-%d", incidentID, sequence)
}
