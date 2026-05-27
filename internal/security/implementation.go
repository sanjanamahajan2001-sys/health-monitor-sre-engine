package security

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"health-monitor/internal/config"
	"golang.org/x/time/rate"
)

// SecurityImpl implements the SecurityConfig interface
type SecurityImpl struct {
	config          config.SecurityConfig
	rateLimitMgr    *RateLimiterManager
	validationRegex *regexp.Regexp
}

// NewSecurityImpl creates a new security implementation
func NewSecurityImpl(cfg config.SecurityConfig) (*SecurityImpl, error) {
	// Parse IP whitelist
	if err := cfg.Webhook.Auth.IPWhitelist.ParseIPWhitelist(); err != nil {
		return nil, fmt.Errorf("failed to parse IP whitelist: %w", err)
	}

	// Compile validation regex
	var validationRegex *regexp.Regexp
	if cfg.Webhook.Validation.Enabled {
		pattern := cfg.Webhook.Validation.AllowedCharacters
		if pattern == "" {
			pattern = "a-zA-Z0-9._-"
		}
		regex := regexp.MustCompile("^[" + pattern + "]*$")
		validationRegex = regex
	}

	// Create rate limiter manager
	rateLimitMgr := NewRateLimiterManager(&rateLimitConfigWrapper{
		config: cfg.Webhook.RateLimit,
	})

	return &SecurityImpl{
		config:          cfg,
		rateLimitMgr:    rateLimitMgr,
		validationRegex: validationRegex,
	}, nil
}

// IsWebhookAuthEnabled returns true if webhook authentication is enabled
func (s *SecurityImpl) IsWebhookAuthEnabled() bool {
	return s.config.Webhook.Auth.Enabled
}

// ValidateToken validates an authentication token
func (s *SecurityImpl) ValidateToken(token string) bool {
	if !s.config.Webhook.Auth.Enabled {
		return true // No auth required
	}

	for _, authToken := range s.config.Webhook.Auth.Tokens {
		if authToken.Token == token {
			return true
		}
	}
	return false
}

// IsIPAllowed checks if an IP address is allowed
func (s *SecurityImpl) IsIPAllowed(ip string) bool {
	return s.config.Webhook.Auth.IPWhitelist.IsIPAllowed(ip)
}

// IsRateLimitEnabled returns true if rate limiting is enabled
func (s *SecurityImpl) IsRateLimitEnabled() bool {
	return s.config.Webhook.RateLimit.Enabled
}

// GetRateLimiter gets a rate limiter for the current request
func (s *SecurityImpl) GetRateLimiter() *rate.Limiter {
	// For simplicity, return a global limiter
	// In production, you'd want per-client limiters
	rps := float64(s.config.Webhook.RateLimit.RequestsPerMinute) / 60.0
	return rate.NewLimiter(rate.Limit(rps), s.config.Webhook.RateLimit.BurstSize)
}

// IsValidationEnabled returns true if validation is enabled
func (s *SecurityImpl) IsValidationEnabled() bool {
	return s.config.Webhook.Validation.Enabled
}

// ValidatePayload validates a webhook payload
func (s *SecurityImpl) ValidatePayload(payload []byte) error {
	if !s.config.Webhook.Validation.Enabled {
		return nil
	}

	// Parse JSON to validate structure
	var alertData map[string]interface{}
	if err := json.Unmarshal(payload, &alertData); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	// Check for alerts array
	alerts, ok := alertData["alerts"]
	if !ok {
		return fmt.Errorf("missing alerts field")
	}

	alertsArray, ok := alerts.([]interface{})
	if !ok {
		return fmt.Errorf("alerts must be an array")
	}

	// Check alert count limit
	if len(alertsArray) > s.config.Webhook.Validation.MaxAlertsPerRequest {
		return fmt.Errorf("too many alerts: max %d, got %d", 
			s.config.Webhook.Validation.MaxAlertsPerRequest, len(alertsArray))
	}

	// Validate each alert
	for i, alert := range alertsArray {
		alertMap, ok := alert.(map[string]interface{})
		if !ok {
			return fmt.Errorf("alert %d must be an object", i)
		}

		// Validate labels
		if err := s.validateLabels(alertMap); err != nil {
			return fmt.Errorf("alert %d: %w", i, err)
		}

		// Validate annotations
		if err := s.validateAnnotations(alertMap); err != nil {
			return fmt.Errorf("alert %d: %w", i, err)
		}
	}

	return nil
}

// validateLabels validates alert labels
func (s *SecurityImpl) validateLabels(alert map[string]interface{}) error {
	labels, ok := alert["labels"]
	if !ok {
		return fmt.Errorf("missing labels field")
	}

	labelsMap, ok := labels.(map[string]interface{})
	if !ok {
		return fmt.Errorf("labels must be an object")
	}

	// Validate each label
	for key, value := range labelsMap {
		// Check key size
		if len(key) > s.config.Webhook.Validation.MaxLabelSize {
			return fmt.Errorf("label key too long: %s", key)
		}

		// Check value
		valueStr, ok := value.(string)
		if !ok {
			return fmt.Errorf("label value must be string: %s", key)
		}

		if len(valueStr) > s.config.Webhook.Validation.MaxLabelSize {
			return fmt.Errorf("label value too long: %s", key)
		}

		// Check allowed characters
		if s.validationRegex != nil {
			if !s.validationRegex.MatchString(valueStr) {
				return fmt.Errorf("label value contains invalid characters: %s", key)
			}
		}
	}

	return nil
}

// validateAnnotations validates alert annotations
func (s *SecurityImpl) validateAnnotations(alert map[string]interface{}) error {
	annotations, ok := alert["annotations"]
	if !ok {
		return nil // Annotations are optional
	}

	annotationsMap, ok := annotations.(map[string]interface{})
	if !ok {
		return fmt.Errorf("annotations must be an object")
	}

	// Validate each annotation
	for key, value := range annotationsMap {
		// Check key size
		if len(key) > s.config.Webhook.Validation.MaxAnnotationSize {
			return fmt.Errorf("annotation key too long: %s", key)
		}

		// Check value
		valueStr, ok := value.(string)
		if !ok {
			return fmt.Errorf("annotation value must be string: %s", key)
		}

		if len(valueStr) > s.config.Webhook.Validation.MaxAnnotationSize {
			return fmt.Errorf("annotation value too long: %s", key)
		}
	}

	return nil
}

// rateLimitConfigWrapper wraps RateLimitConfig for the interface
type rateLimitConfigWrapper struct {
	config config.RateLimitConfig
}

func (w *rateLimitConfigWrapper) GetRequestsPerMinute() int {
	return w.config.RequestsPerMinute
}

func (w *rateLimitConfigWrapper) GetBurstSize() int {
	return w.config.BurstSize
}

// AuditLogger provides audit logging functionality
type AuditLogger struct {
	config config.AuditConfig
}

// NewAuditLogger creates a new audit logger
func NewAuditLogger(cfg config.AuditConfig) *AuditLogger {
	return &AuditLogger{
		config: cfg,
	}
}

// LogEvent logs an audit event
func (a *AuditLogger) LogEvent(event AuditEvent) error {
	if !a.config.Enabled {
		return nil
	}

	// Check if this event type should be logged
	shouldLog := false
	for _, eventType := range a.config.Events {
		if eventType == event.Action || eventType == "*" {
			shouldLog = true
			break
		}
	}

	if !shouldLog {
		return nil
	}

	// Convert to JSON
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal audit event: %w", err)
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(a.config.LogFile), 0755); err != nil {
		return fmt.Errorf("failed to create audit log directory: %w", err)
	}

	// Open file in append mode
	file, err := os.OpenFile(a.config.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open audit log file: %w", err)
	}
	defer file.Close()

	// Write audit event with newline
	if _, err := file.WriteString(string(eventJSON) + "\n"); err != nil {
		return fmt.Errorf("failed to write audit event: %w", err)
	}

	return nil
}

// AuditEvent represents an audit log entry
type AuditEvent struct {
	Timestamp string `json:"timestamp"`
	User      string `json:"user"`
	Action    string `json:"action"`
	Resource  string `json:"resource"`
	Success   bool   `json:"success"`
	Details   string `json:"details,omitempty"`
}

// RBACChecker provides role-based access control
type RBACChecker struct {
	config config.RBACConfig
}

// NewRBACChecker creates a new RBAC checker
func NewRBACChecker(cfg config.RBACConfig) *RBACChecker {
	return &RBACChecker{
		config: cfg,
	}
}

// CheckPermission checks if a user has a specific permission
func (r *RBACChecker) CheckPermission(user, permission string) bool {
	return r.config.HasPermission(user, permission)
}

// APIKeyChecker provides API key validation
type APIKeyChecker struct {
	config config.APIKeyConfig
}

// NewAPIKeyChecker creates a new API key checker
func NewAPIKeyChecker(cfg config.APIKeyConfig) *APIKeyChecker {
	return &APIKeyChecker{
		config: cfg,
	}
}

// ValidateAPIKey checks if an API key is valid and returns its permissions
func (a *APIKeyChecker) ValidateAPIKey(key string) ([]string, bool) {
	return a.config.ValidateAPIKey(key)
}

// SecretsManager provides secrets management functionality
type SecretsManager struct {
	config config.EncryptionConfig
}

// NewSecretsManager creates a new secrets manager
func NewSecretsManager(cfg config.EncryptionConfig) *SecretsManager {
	return &SecretsManager{
		config: cfg,
	}
}

// SubstituteEnvVars substitutes environment variables in configuration values
func (s *SecretsManager) SubstituteEnvVars(value string) string {
	if !strings.Contains(value, "${") {
		return value
	}

	// Simple environment variable substitution
	// In production, you'd want more robust handling
	replacer := strings.NewReplacer("${", "", "}", "")
	envVar := replacer.Replace(value)
	if envValue := strings.TrimSpace(envVar); envValue != "" {
		return envValue
	}

	return value
}

// IsEncryptedField checks if a field should be encrypted
func (s *SecretsManager) IsEncryptedField(fieldName string) bool {
	if !s.config.Enabled {
		return false
	}

	for _, field := range s.config.EncryptedFields {
		if field == fieldName {
			return true
		}
	}
	return false
}
