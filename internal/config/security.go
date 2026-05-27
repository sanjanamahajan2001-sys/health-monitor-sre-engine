package config

import (
	"fmt"
	"net/netip"
)

// SecurityConfig holds all security-related configurations
type SecurityConfig struct {
	Webhook WebhookSecurityConfig `yaml:"webhook"`
	RBAC    RBACConfig            `yaml:"rbac"`
	Audit   AuditConfig           `yaml:"audit"`
	APIKeys APIKeyConfig          `yaml:"api_keys"`
	Encryption EncryptionConfig    `yaml:"encryption"`
}

// WebhookSecurityConfig handles webhook authentication and authorization
type WebhookSecurityConfig struct {
	Auth WebhookAuthConfig `yaml:"auth"`
	RateLimit RateLimitConfig `yaml:"rate_limit"`
	Validation ValidationConfig `yaml:"validation"`
}

// WebhookAuthConfig handles webhook authentication methods
type WebhookAuthConfig struct {
	Enabled     bool              `yaml:"enabled"`
	Tokens      []AuthToken       `yaml:"tokens"`
	IPWhitelist IPWhitelistConfig `yaml:"ip_whitelist"`
}

// AuthToken represents an authentication token
type AuthToken struct {
	Name        string `yaml:"name"`
	Token       string `yaml:"token"`
	Description string `yaml:"description"`
}

// IPWhitelistConfig handles IP-based access control
type IPWhitelistConfig struct {
	Enabled    bool     `yaml:"enabled"`
	AllowedIPs []string `yaml:"allowed_ips"`
	cachedIPs  []netip.Prefix
}

// RateLimitConfig handles request rate limiting
type RateLimitConfig struct {
	Enabled           bool `yaml:"enabled"`
	RequestsPerMinute int  `yaml:"requests_per_minute"`
	BurstSize         int  `yaml:"burst_size"`
}

// ValidationConfig handles input validation
type ValidationConfig struct {
	Enabled             bool   `yaml:"enabled"`
	MaxAlertsPerRequest int    `yaml:"max_alerts_per_request"`
	MaxLabelSize        int    `yaml:"max_label_size"`
	MaxAnnotationSize   int    `yaml:"max_annotation_size"`
	AllowedCharacters   string `yaml:"allowed_characters"`
}

// RBACConfig handles role-based access control
type RBACConfig struct {
	Enabled bool              `yaml:"enabled"`
	Roles   map[string][]string `yaml:"roles"`
	Users   map[string]string  `yaml:"users"`
}

// AuditConfig handles audit logging
type AuditConfig struct {
	Enabled  bool     `yaml:"enabled"`
	LogFile  string   `yaml:"log_file"`
	Events   []string `yaml:"events"`
	RetentionDays int `yaml:"retention_days"`
}

// APIKeyConfig handles API key authentication
type APIKeyConfig struct {
	Enabled bool      `yaml:"enabled"`
	Keys    []APIKey `yaml:"keys"`
}

// APIKey represents an API key with permissions
type APIKey struct {
	Name        string   `yaml:"name"`
	Key         string   `yaml:"key"`
	Permissions []string `yaml:"permissions"`
	Description string   `yaml:"description"`
}

// EncryptionConfig handles data encryption
type EncryptionConfig struct {
	Enabled         bool     `yaml:"enabled"`
	KeyFile         string   `yaml:"key_file"`
	EncryptedFields []string `yaml:"encrypted_fields"`
}

// DefaultSecurityConfig returns a secure default configuration with all features disabled
func DefaultSecurityConfig() SecurityConfig {
	return SecurityConfig{
		Webhook: WebhookSecurityConfig{
			Auth: WebhookAuthConfig{
				Enabled: false,
				Tokens: []AuthToken{},
				IPWhitelist: IPWhitelistConfig{
					Enabled:    false,
					AllowedIPs: []string{},
				},
			},
			RateLimit: RateLimitConfig{
				Enabled:           false,
				RequestsPerMinute: 1000,
				BurstSize:         100,
			},
			Validation: ValidationConfig{
				Enabled:             false,
				MaxAlertsPerRequest: 100,
				MaxLabelSize:        1024,
				MaxAnnotationSize:   4096,
				AllowedCharacters:   "a-zA-Z0-9._-",
			},
		},
		RBAC: RBACConfig{
			Enabled: false,
			Roles: map[string][]string{
				"operator": {"incident:list", "incident:view", "incident:resolve"},
				"admin":    {"*"},
			},
			Users: map[string]string{},
		},
		Audit: AuditConfig{
			Enabled:      false,
			LogFile:      "/var/log/health-monitor/audit.log",
			Events:       []string{"incident_created", "incident_resolved", "config_changed"},
			RetentionDays: 90,
		},
		APIKeys: APIKeyConfig{
			Enabled: false,
			Keys:    []APIKey{},
		},
		Encryption: EncryptionConfig{
			Enabled:         false,
			KeyFile:         "/etc/health-monitor/encryption.key",
			EncryptedFields: []string{"webhook_url", "api_key"},
		},
	}
}

// Validate validates the security configuration
func (s *SecurityConfig) Validate() error {
	// Validate webhook auth tokens
	for _, token := range s.Webhook.Auth.Tokens {
		if token.Name == "" {
			return fmt.Errorf("auth token name cannot be empty")
		}
		if token.Token == "" {
			return fmt.Errorf("auth token value cannot be empty for token %s", token.Name)
		}
	}

	// Validate IP whitelist
	if s.Webhook.Auth.IPWhitelist.Enabled {
		if len(s.Webhook.Auth.IPWhitelist.AllowedIPs) == 0 {
			return fmt.Errorf("IP whitelist enabled but no IPs specified")
		}
		for _, ip := range s.Webhook.Auth.IPWhitelist.AllowedIPs {
			if _, err := netip.ParsePrefix(ip); err != nil {
				return fmt.Errorf("invalid IP address in whitelist: %s", ip)
			}
		}
	}

	// Validate rate limiting
	if s.Webhook.RateLimit.Enabled {
		if s.Webhook.RateLimit.RequestsPerMinute <= 0 {
			return fmt.Errorf("rate limit requests per minute must be positive")
		}
		if s.Webhook.RateLimit.BurstSize <= 0 {
			return fmt.Errorf("rate limit burst size must be positive")
		}
	}

	// Validate validation config
	if s.Webhook.Validation.Enabled {
		if s.Webhook.Validation.MaxAlertsPerRequest <= 0 {
			return fmt.Errorf("max alerts per request must be positive")
		}
		if s.Webhook.Validation.MaxLabelSize <= 0 {
			return fmt.Errorf("max label size must be positive")
		}
		if s.Webhook.Validation.MaxAnnotationSize <= 0 {
			return fmt.Errorf("max annotation size must be positive")
		}
	}

	return nil
}

// ParseIPWhitelist parses and caches IP whitelist entries
func (w *IPWhitelistConfig) ParseIPWhitelist() error {
	if !w.Enabled {
		return nil
	}

	w.cachedIPs = make([]netip.Prefix, 0, len(w.AllowedIPs))
	for _, ipStr := range w.AllowedIPs {
		prefix, err := netip.ParsePrefix(ipStr)
		if err != nil {
			return fmt.Errorf("invalid IP prefix %s: %w", ipStr, err)
		}
		w.cachedIPs = append(w.cachedIPs, prefix)
	}
	return nil
}

// IsIPAllowed checks if an IP address is allowed by the whitelist
func (w *IPWhitelistConfig) IsIPAllowed(ipStr string) bool {
	if !w.Enabled {
		return true
	}

	ip, err := netip.ParseAddr(ipStr)
	if err != nil {
		return false
	}

	for _, prefix := range w.cachedIPs {
		if prefix.Contains(ip) {
			return true
		}
	}
	return false
}

// HasPermission checks if a user has a specific permission
func (r *RBACConfig) HasPermission(user, permission string) bool {
	if !r.Enabled {
		return true // No RBAC means all permissions granted
	}

	role, exists := r.Users[user]
	if !exists {
		return false
	}

	permissions, exists := r.Roles[role]
	if !exists {
		return false
	}

	// Admin role has all permissions
	for _, perm := range permissions {
		if perm == "*" {
			return true
		}
		if perm == permission {
			return true
		}
	}
	return false
}

// ValidateAPIKey checks if an API key is valid and returns its permissions
func (a *APIKeyConfig) ValidateAPIKey(key string) ([]string, bool) {
	if !a.Enabled {
		return []string{"*"}, true // No API key auth means full permissions
	}

	for _, apiKey := range a.Keys {
		if apiKey.Key == key {
			return apiKey.Permissions, true
		}
	}
	return nil, false
}
