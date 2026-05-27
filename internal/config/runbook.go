package config

import "fmt"

// RunbookConfig represents configuration for the runbook suggestion system
type RunbookConfig struct {
	Enabled             bool     `json:"enabled" yaml:"enabled"`
	AutoGenerate        bool     `json:"auto_generate" yaml:"auto_generate"`
	ConfidenceThreshold float64  `json:"confidence_threshold" yaml:"confidence_threshold"`
	TemplateDirectory   string   `json:"template_directory" yaml:"template_directory"`
	OutputDirectory     string   `json:"output_directory" yaml:"output_directory"`
	OutputFormat        string   `json:"output_format" yaml:"output_format"`
	PublishToWiki       bool     `json:"publish_to_wiki" yaml:"publish_to_wiki"`
	WikiURL             string   `json:"wiki_url" yaml:"wiki_url"`
	NotificationWebhook string   `json:"notification_webhook" yaml:"notification_webhook"`
	RetentionDays       int      `json:"retention_days" yaml:"retention_days"`
	MaxRunbooksPerPattern int    `json:"max_runbooks_per_pattern" yaml:"max_runbooks_per_pattern"`
	EnabledCategories   []string `json:"enabled_categories" yaml:"enabled_categories"`
	DisabledServices    []string `json:"disabled_services" yaml:"disabled_services"`
	
	// New production-ready fields
	MaxSuggestions      int      `json:"max_suggestions" yaml:"max_suggestions"`
	CacheTTL            int      `json:"cache_ttl" yaml:"cache_ttl"`
	CustomPatterns      []CustomPatternConfig `json:"custom_patterns" yaml:"custom_patterns"`
	MetricsEnabled      bool     `json:"metrics_enabled" yaml:"metrics_enabled"`
	FeedbackEnabled     bool     `json:"feedback_enabled" yaml:"feedback_enabled"`
	MLModelEnabled      bool     `json:"ml_model_enabled" yaml:"ml_model_enabled"`
}

// CustomPatternConfig allows users to define custom error patterns
type CustomPatternConfig struct {
	Name        string  `json:"name" yaml:"name"`
	Pattern     string  `json:"pattern" yaml:"pattern"`
	Runbook     string  `json:"runbook" yaml:"runbook"`
	Confidence  float64 `json:"confidence" yaml:"confidence"`
	Description string  `json:"description" yaml:"description"`
}

// DefaultRunbookConfig returns safe default configuration
func DefaultRunbookConfig() RunbookConfig {
	return RunbookConfig{
		Enabled:                true, // Enabled by default to increase feature discoverability
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
		
		// New production-ready defaults
		MaxSuggestions:         3,
		CacheTTL:              3600, // 1 hour
		CustomPatterns:         []CustomPatternConfig{},
		MetricsEnabled:         true,
		FeedbackEnabled:        false, // Requires additional setup
		MLModelEnabled:         false, // Requires ML model setup
	}
}

// Validate validates the runbook configuration
func (r *RunbookConfig) Validate() error {
	// Validate confidence threshold
	if r.ConfidenceThreshold < 0.0 || r.ConfidenceThreshold > 1.0 {
		return fmt.Errorf("confidence_threshold must be between 0.0 and 1.0, got %.2f", r.ConfidenceThreshold)
	}

	// Validate output format
	validFormats := []string{"markdown", "html", "json"}
	validFormat := false
	for _, format := range validFormats {
		if r.OutputFormat == format {
			validFormat = true
			break
		}
	}
	if !validFormat {
		return fmt.Errorf("output_format must be one of: %v", validFormats)
	}

	// Validate retention days
	if r.RetentionDays < 0 {
		return fmt.Errorf("retention_days must be non-negative, got %d", r.RetentionDays)
	}

	// Validate max runbooks per pattern
	if r.MaxRunbooksPerPattern < 1 {
		return fmt.Errorf("max_runbooks_per_pattern must be at least 1, got %d", r.MaxRunbooksPerPattern)
	}

	// Validate template directory
	if r.TemplateDirectory == "" {
		return fmt.Errorf("template_directory cannot be empty")
	}

	// Validate output directory
	if r.OutputDirectory == "" {
		return fmt.Errorf("output_directory cannot be empty")
	}

	// If wiki publishing is enabled, wiki URL must be set
	if r.PublishToWiki && r.WikiURL == "" {
		return fmt.Errorf("wiki_url must be set when publish_to_wiki is enabled")
	}

	return nil
}

// IsServiceEnabled checks if a service is enabled for runbook generation
func (r *RunbookConfig) IsServiceEnabled(service string) bool {
	for _, disabled := range r.DisabledServices {
		if disabled == service {
			return false
		}
	}
	return true
}

// IsCategoryEnabled checks if a category is enabled for runbook generation
func (r *RunbookConfig) IsCategoryEnabled(category string) bool {
	if len(r.EnabledCategories) == 0 {
		return true // No restrictions means all categories are enabled
	}
	
	for _, enabled := range r.EnabledCategories {
		if enabled == category {
			return true
		}
	}
	return false
}

// IsRunbookSuggestionsEnabled checks if runbook suggestions are globally enabled
func IsRunbookSuggestionsEnabled() bool {
	cfg, err := Load()
	if err != nil {
		return false
	}
	return cfg.RunbookSuggestions.Enabled
}

// GetRunbookConfidenceThreshold returns the global confidence threshold
func GetRunbookConfidenceThreshold() float64 {
	cfg, err := Load()
	if err != nil {
		return 0.7
	}
	return cfg.RunbookSuggestions.ConfidenceThreshold
}

// IsRunbookMetricsEnabled checks if runbook metrics tracking is enabled
func IsRunbookMetricsEnabled() bool {
	cfg, err := Load()
	if err != nil {
		return true
	}
	return cfg.RunbookSuggestions.MetricsEnabled
}
