package runbook

import (
	"time"
)

// Runbook represents a generated troubleshooting document
type Runbook struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Service     string            `json:"service"`
	Pattern     string            `json:"pattern"`
	Category    string            `json:"category"`
	Component   string            `json:"component"`
	Severity    string            `json:"severity"`
	Content     string            `json:"content"`
	Format      string            `json:"format"` // markdown, html, etc.
	Steps       []RunbookStep     `json:"steps"`
	Metrics     []MetricQuery     `json:"metrics"`
	LogQueries  []LogQuery        `json:"log_queries"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	CreatedBy   string            `json:"created_by"`
	Published   bool              `json:"published"`
	Tags        []string          `json:"tags"`
	Metadata    map[string]string `json:"metadata"`
	Cluster     string            `json:"cluster,omitempty"`
	Namespace   string            `json:"namespace,omitempty"`
	Deployment  string            `json:"deployment,omitempty"`
}

// RunbookStep represents a single troubleshooting step
type RunbookStep struct {
	Order       int    `json:"order"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Command     string `json:"command,omitempty"`
	Expected    string `json:"expected,omitempty"`
	Timeout     string `json:"timeout,omitempty"`
	Critical    bool   `json:"critical"`
}

// MetricQuery represents a Prometheus query for diagnostics
type MetricQuery struct {
	Name        string `json:"name"`
	Query       string `json:"query"`
	Description string `json:"description"`
	Panel       string `json:"panel,omitempty"`
}

// LogQuery represents a log search query
type LogQuery struct {
	Name        string `json:"name"`
	Query       string `json:"query"`
	Description string `json:"description"`
	TimeRange   string `json:"time_range"`
}

// PatternAnalysis represents the analysis of incident patterns
type PatternAnalysis struct {
	Pattern        string            `json:"pattern"`
	Service        string            `json:"service"`
	Component      string            `json:"component"`
	Category       string            `json:"category"`
	Dependency     string            `json:"dependency"`
	Frequency      int               `json:"frequency"`
	LastSeen       time.Time         `json:"last_seen"`
	IncidentIDs    []string          `json:"incident_ids"`
	Confidence     float64           `json:"confidence"`
	SuggestedSteps []RunbookStep     `json:"suggested_steps"`
	RelatedMetrics []MetricQuery     `json:"related_metrics"`
	RelatedLogs    []LogQuery        `json:"related_logs"`
	Tags           []string          `json:"tags"`
	Metadata       map[string]string `json:"metadata"`
	Cluster        string            `json:"cluster,omitempty"`
	Namespace      string            `json:"namespace,omitempty"`
	Deployment     string            `json:"deployment,omitempty"`
}

// RunbookTemplate represents a template for generating runbooks
type RunbookTemplate struct {
	Name        string            `json:"name"`
	Pattern     string            `json:"pattern"`
	Category    string            `json:"category"`
	Content     string            `json:"content"`
	Variables   []TemplateVariable `json:"variables"`
	Steps       []RunbookStep     `json:"steps"`
	Metrics     []MetricQuery     `json:"metrics"`
	LogQueries  []LogQuery        `json:"log_queries"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// TemplateVariable represents a variable in a runbook template
type TemplateVariable struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Type         string `json:"type"` // string, int, bool, list
	Required     bool   `json:"required"`
	DefaultValue string `json:"default_value"`
}

// RunbookSuggestion represents a suggested runbook for an incident
type RunbookSuggestion struct {
	RunbookID    string    `json:"runbook_id"`
	Title        string    `json:"title"`
	Confidence   float64   `json:"confidence"`
	MatchReason  string    `json:"match_reason"`
	IncidentID   string    `json:"incident_id"`
	SuggestedAt  time.Time `json:"suggested_at"`
	Applied      bool      `json:"applied"`
	Feedback     *Feedback `json:"feedback,omitempty"`
}

// Feedback represents user feedback on suggestions
type Feedback struct {
	Helpful     bool      `json:"helpful"`
	Rating      int       `json:"rating"` // 1-5
	Comment     string    `json:"comment"`
	SubmittedAt time.Time `json:"submitted_at"`
	SubmittedBy string    `json:"submitted_by"`
}

// RunbookConfig represents configuration for the runbook system
type RunbookConfig struct {
	Enabled             bool     `yaml:"enabled"`
	AutoGenerate        bool     `yaml:"auto_generate"`
	ConfidenceThreshold float64  `yaml:"confidence_threshold"`
	TemplateDirectory   string   `yaml:"template_directory"`
	OutputDirectory     string   `yaml:"output_directory"`
	OutputFormat        string   `yaml:"output_format"`
	PublishToWiki       bool     `yaml:"publish_to_wiki"`
	WikiURL             string   `yaml:"wiki_url"`
	NotificationWebhook string   `yaml:"notification_webhook"`
	RetentionDays       int      `yaml:"retention_days"`
	MaxRunbooksPerPattern int    `yaml:"max_runbooks_per_pattern"`
	EnabledCategories   []string `yaml:"enabled_categories"`
	DisabledServices    []string `yaml:"disabled_services"`
}

// DefaultRunbookConfig returns safe default configuration
func DefaultRunbookConfig() RunbookConfig {
	return RunbookConfig{
		Enabled:                false, // Disabled by default for safety
		AutoGenerate:           false,
		ConfidenceThreshold:    0.7,
		TemplateDirectory:      "/etc/health-monitor/runbook-templates",
		OutputDirectory:        "/var/lib/health-monitor/runbooks",
		OutputFormat:           "markdown",
		PublishToWiki:          false,
		WikiURL:                "",
		NotificationWebhook:    "",
		RetentionDays:          365,
		MaxRunbooksPerPattern:  10,
		EnabledCategories:      []string{"capacity", "latency", "dependency", "infra"},
		DisabledServices:       []string{},
	}
}
