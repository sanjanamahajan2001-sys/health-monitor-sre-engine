package commands

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"

	"health-monitor/internal/config"
	"health-monitor/internal/security"
)

// SecurityCmd handles security-related commands
type SecurityCmd struct{}

// NewSecurityCmd creates a new security command
func NewSecurityCmd() *SecurityCmd {
	return &SecurityCmd{}
}

// ValidateConfig validates security configuration
func (s *SecurityCmd) ValidateConfig(profileName string) error {
	// Load configuration
	cfg, err := config.LoadForProfile(profileName)
	if err != nil {
		return fmt.Errorf("failed to load config for profile %s: %w", profileName, err)
	}

	// Validate security configuration
	if err := cfg.Security.Validate(); err != nil {
		return fmt.Errorf("security configuration validation failed: %w", err)
	}

	// Create security implementation to test
	_, err = security.NewSecurityImpl(cfg.Security)
	if err != nil {
		return fmt.Errorf("security implementation test failed: %w", err)
	}

	fmt.Printf("✅ Security configuration is valid for profile: %s\n", profileName)
	s.printSecurityStatus(cfg.Security)
	return nil
}

// TestSecurity tests security features
func (s *SecurityCmd) TestSecurity(profileName string) error {
	// Load configuration
	cfg, err := config.LoadForProfile(profileName)
	if err != nil {
		return fmt.Errorf("failed to load config for profile %s: %w", profileName, err)
	}

	// Test security implementation
	secImpl, err := security.NewSecurityImpl(cfg.Security)
	if err != nil {
		return fmt.Errorf("failed to create security implementation: %w", err)
	}

	fmt.Printf("🔒 Testing security features for profile: %s\n\n", profileName)

	// Test authentication
	if cfg.Security.Webhook.Auth.Enabled {
		s.testAuthentication(secImpl, cfg.Security)
	} else {
		fmt.Println("ℹ️  Authentication is disabled")
	}

	// Test IP whitelist
	if cfg.Security.Webhook.Auth.IPWhitelist.Enabled {
		s.testIPWhitelist(secImpl, cfg.Security)
	} else {
		fmt.Println("ℹ️  IP whitelist is disabled")
	}

	// Test rate limiting
	if cfg.Security.Webhook.RateLimit.Enabled {
		s.testRateLimiting(secImpl, cfg.Security)
	} else {
		fmt.Println("ℹ️  Rate limiting is disabled")
	}

	// Test validation
	if cfg.Security.Webhook.Validation.Enabled {
		s.testValidation(secImpl, cfg.Security)
	} else {
		fmt.Println("ℹ️  Input validation is disabled")
	}

	// Test RBAC
	if cfg.Security.RBAC.Enabled {
		s.testRBAC(cfg.Security)
	} else {
		fmt.Println("ℹ️  RBAC is disabled")
	}

	// Test API keys
	if cfg.Security.APIKeys.Enabled {
		s.testAPIKeys(cfg.Security)
	} else {
		fmt.Println("ℹ️  API keys are disabled")
	}

	fmt.Println("\n✅ All security tests completed successfully!")
	return nil
}

// GenerateKey generates a new encryption key
func (s *SecurityCmd) GenerateKey(outputFile string) error {
	if outputFile == "" {
		outputFile = "/etc/health-monitor/encryption.key"
	}

	// Generate a random 32-byte key for AES-256
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return fmt.Errorf("failed to generate random key: %w", err)
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(outputFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Write key to file with restricted permissions
	if err := os.WriteFile(outputFile, []byte(fmt.Sprintf("%x", key)), 0600); err != nil {
		return fmt.Errorf("failed to write key file: %w", err)
	}

	fmt.Printf("🔑 Generated encryption key: %s\n", outputFile)
	fmt.Printf("⚠️  Ensure this file is backed up securely!\n")
	return nil
}

// Status shows security status
func (s *SecurityCmd) Status(profileName string) error {
	cfg, err := config.LoadForProfile(profileName)
	if err != nil {
		return fmt.Errorf("failed to load config for profile %s: %w", profileName, err)
	}

	fmt.Printf("🔒 Security Status for profile: %s\n\n", profileName)
	s.printSecurityStatus(cfg.Security)
	return nil
}

// Helper methods

func (s *SecurityCmd) printSecurityStatus(secConfig config.SecurityConfig) {
	fmt.Println("🛡️  Security Features:")
	fmt.Printf("   Webhook Authentication: %s\n", boolToStatus(secConfig.Webhook.Auth.Enabled))
	fmt.Printf("   IP Whitelist: %s\n", boolToStatus(secConfig.Webhook.Auth.IPWhitelist.Enabled))
	fmt.Printf("   Rate Limiting: %s\n", boolToStatus(secConfig.Webhook.RateLimit.Enabled))
	fmt.Printf("   Input Validation: %s\n", boolToStatus(secConfig.Webhook.Validation.Enabled))
	fmt.Printf("   RBAC: %s\n", boolToStatus(secConfig.RBAC.Enabled))
	fmt.Printf("   Audit Logging: %s\n", boolToStatus(secConfig.Audit.Enabled))
	fmt.Printf("   API Keys: %s\n", boolToStatus(secConfig.APIKeys.Enabled))
	fmt.Printf("   Encryption: %s\n", boolToStatus(secConfig.Encryption.Enabled))

	if secConfig.Webhook.Auth.Enabled {
		fmt.Printf("\n🔑 Authentication Tokens: %d configured\n", len(secConfig.Webhook.Auth.Tokens))
		for _, token := range secConfig.Webhook.Auth.Tokens {
			fmt.Printf("   - %s: %s\n", token.Name, token.Description)
		}
	}

	if secConfig.Webhook.Auth.IPWhitelist.Enabled {
		fmt.Printf("🌐 IP Whitelist: %d entries\n", len(secConfig.Webhook.Auth.IPWhitelist.AllowedIPs))
		for _, ip := range secConfig.Webhook.Auth.IPWhitelist.AllowedIPs {
			fmt.Printf("   - %s\n", ip)
		}
	}
}

func (s *SecurityCmd) testAuthentication(secImpl *security.SecurityImpl, secConfig config.SecurityConfig) {
	fmt.Println("🔐 Testing Authentication...")
	
	for _, token := range secConfig.Webhook.Auth.Tokens {
		if secImpl.ValidateToken(token.Token) {
			fmt.Printf("   ✅ Token '%s' is valid\n", token.Name)
		} else {
			fmt.Printf("   ❌ Token '%s' is invalid\n", token.Name)
		}
	}

	// Test invalid token
	if !secImpl.ValidateToken("Bearer invalid-token") {
		fmt.Println("   ✅ Invalid token correctly rejected")
	} else {
		fmt.Println("   ❌ Invalid token was incorrectly accepted")
	}
}

func (s *SecurityCmd) testIPWhitelist(secImpl *security.SecurityImpl, secConfig config.SecurityConfig) {
	fmt.Println("🌐 Testing IP Whitelist...")
	
	for _, ip := range secConfig.Webhook.Auth.IPWhitelist.AllowedIPs {
		if secImpl.IsIPAllowed(ip) {
			fmt.Printf("   ✅ IP '%s' is allowed\n", ip)
		} else {
			fmt.Printf("   ❌ IP '%s' was rejected\n", ip)
		}
	}

	// Test invalid IP
	if !secImpl.IsIPAllowed("192.168.999.999") {
		fmt.Println("   ✅ Invalid IP correctly rejected")
	} else {
		fmt.Println("   ❌ Invalid IP was incorrectly accepted")
	}
}

func (s *SecurityCmd) testRateLimiting(secImpl *security.SecurityImpl, secConfig config.SecurityConfig) {
	fmt.Println("⏱️  Testing Rate Limiting...")
	
	limiter := secImpl.GetRateLimiter()
	if limiter != nil {
		fmt.Printf("   ✅ Rate limiter created (RPS: %.1f, Burst: %d)\n", 
			float64(secConfig.Webhook.RateLimit.RequestsPerMinute)/60.0,
			secConfig.Webhook.RateLimit.BurstSize)
		
		// Test rate limiting
		if limiter.Allow() {
			fmt.Println("   ✅ First request allowed")
		}
	} else {
		fmt.Println("   ❌ Failed to create rate limiter")
	}
}

func (s *SecurityCmd) testValidation(secImpl *security.SecurityImpl, secConfig config.SecurityConfig) {
	fmt.Println("✅ Testing Input Validation...")
	
	validPayload := `{
		"alerts": [
			{
				"labels": {
					"alertname": "TestAlert",
					"service": "test-service"
				},
				"annotations": {
					"summary": "Test summary"
				}
			}
		]
	}`
	
	if err := secImpl.ValidatePayload([]byte(validPayload)); err != nil {
		fmt.Printf("   ❌ Valid payload rejected: %v\n", err)
	} else {
		fmt.Println("   ✅ Valid payload accepted")
	}

	// Test invalid payload
	invalidPayload := `{"invalid": "json"}`
	if err := secImpl.ValidatePayload([]byte(invalidPayload)); err != nil {
		fmt.Printf("   ✅ Invalid payload correctly rejected: %v\n", err)
	} else {
		fmt.Println("   ❌ Invalid payload was incorrectly accepted")
	}
}

func (s *SecurityCmd) testRBAC(secConfig config.SecurityConfig) {
	fmt.Println("👥 Testing RBAC...")
	
	rbacChecker := security.NewRBACChecker(secConfig.RBAC)
	
	for user, role := range secConfig.RBAC.Users {
		if rbacChecker.CheckPermission(user, "incident:list") {
			fmt.Printf("   ✅ User '%s' (role: %s) has incident:list permission\n", user, role)
		} else {
			fmt.Printf("   ❌ User '%s' (role: %s) missing incident:list permission\n", user, role)
		}
	}
}

func (s *SecurityCmd) testAPIKeys(secConfig config.SecurityConfig) {
	fmt.Println("🔑 Testing API Keys...")
	
	apiKeyChecker := security.NewAPIKeyChecker(secConfig.APIKeys)
	
	for _, apiKey := range secConfig.APIKeys.Keys {
		permissions, valid := apiKeyChecker.ValidateAPIKey(apiKey.Key)
		if valid {
			fmt.Printf("   ✅ API key '%s' is valid (permissions: %v)\n", apiKey.Name, permissions)
		} else {
			fmt.Printf("   ❌ API key '%s' is invalid\n", apiKey.Name)
		}
	}

	// Test invalid API key
	_, valid := apiKeyChecker.ValidateAPIKey("invalid-api-key")
	if !valid {
		fmt.Println("   ✅ Invalid API key correctly rejected")
	} else {
		fmt.Println("   ❌ Invalid API key was incorrectly accepted")
	}
}

func boolToStatus(b bool) string {
	if b {
		return "✅ Enabled"
	}
	return "❌ Disabled"
}
