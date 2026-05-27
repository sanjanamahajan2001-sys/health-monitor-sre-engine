package alert

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"health-monitor/internal/config"
	"health-monitor/internal/security"
)

// SecureWebhookHandler wraps the original webhook handler with security middleware
type SecureWebhookHandler struct {
	processor      *Processor
	securityConfig config.SecurityConfig
	securityImpl   *security.SecurityImpl
	metrics        *security.SecurityMetrics
	auditLogger    *security.AuditLogger
}

// NewSecureWebhookHandler creates a new secure webhook handler
func NewSecureWebhookHandler(processor *Processor, securityConfig config.SecurityConfig) (*SecureWebhookHandler, error) {
	// Create security implementation
	securityImpl, err := security.NewSecurityImpl(securityConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create security implementation: %w", err)
	}

	// Create security metrics
	metrics := security.NewSecurityMetrics()
	metrics.Register()

	// Create audit logger
	auditLogger := security.NewAuditLogger(securityConfig.Audit)

	return &SecureWebhookHandler{
		processor:      processor,
		securityConfig: securityConfig,
		securityImpl:   securityImpl,
		metrics:        metrics,
		auditLogger:    auditLogger,
	}, nil
}

// HandleWebhook handles secure webhook requests
func (s *SecureWebhookHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	clientIP := getClientIP(r)

	// Log audit event
	defer func() {
		s.auditLogger.LogEvent(security.AuditEvent{
			Timestamp: startTime.Format(time.RFC3339),
			User:      "webhook",
			Action:    "webhook_request",
			Resource:  r.URL.Path,
			Success:   true,
			Details:   fmt.Sprintf("client_ip=%s, method=%s", clientIP, r.Method),
		})
	}()

	// Apply security middleware chain
	handler := s.applySecurityMiddleware(http.HandlerFunc(s.processWebhook))
	handler.ServeHTTP(w, r)
}

// applySecurityMiddleware applies all security middleware
func (s *SecureWebhookHandler) applySecurityMiddleware(next http.Handler) http.Handler {
	// Apply middleware in order: IP -> Rate Limit -> Auth -> Validation -> Handler
	handler := next

	if s.securityConfig.Webhook.Validation.Enabled {
		validationMiddleware := security.NewValidationMiddleware(s.securityImpl, s.metrics)
		handler = validationMiddleware.Middleware(handler)
	}

	if s.securityConfig.Webhook.Auth.Enabled || s.securityConfig.Webhook.RateLimit.Enabled {
		authMiddleware := security.NewAuthMiddleware(s.securityImpl, s.metrics)
		handler = authMiddleware.Middleware(handler)
	}

	return handler
}

// processWebhook is the actual webhook processing logic
func (s *SecureWebhookHandler) processWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if len(s.processor.rulesSnapshot()) == 0 {
		http.Error(w, "alerts disabled", http.StatusServiceUnavailable)
		return
	}

	// Read request body with size limit
	const maxBodySize = 10 << 20 // 10MB
	limited := http.MaxBytesReader(w, r.Body, maxBodySize)
	body, err := io.ReadAll(limited)
	if err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	// Parse webhook payload
	var payload WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	// Process alerts
	_, warnings := s.processor.Process(payload, false)

	// Log warnings
	for _, warning := range warnings {
		logWarn(warning, map[string]string{
			"alertname": r.Header.Get("X-Alertname"),
			"path":      r.URL.Path,
		})
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

// NewSecureMux creates a new HTTP mux with security middleware
func NewSecureMux(processor *Processor, securityConfig config.SecurityConfig, readyCheck func() bool) (http.Handler, error) {
	// Create secure webhook handler
	secureHandler, err := NewSecureWebhookHandler(processor, securityConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create secure webhook handler: %w", err)
	}

	mux := http.NewServeMux()
	
	// Register secure webhook handler
	mux.HandleFunc("/webhook", secureHandler.HandleWebhook)
	
	// Register metrics endpoint
	registerMetrics(mux, processor)
	
	// Register readiness check
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if readyCheck != nil && !readyCheck() {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready\n"))
	})

	// Register security metrics endpoint
	mux.HandleFunc("/security-metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Return security metrics as JSON
		fmt.Fprintf(w, `{"security_enabled": {"auth": %t, "rate_limit": %t, "validation": %t}}`,
			securityConfig.Webhook.Auth.Enabled,
			securityConfig.Webhook.RateLimit.Enabled,
			securityConfig.Webhook.Validation.Enabled)
	})

	return mux, nil
}

// getClientIP extracts the real client IP address
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if ips := splitIPs(xff); len(ips) > 0 {
			return ips[0]
		}
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	return r.RemoteAddr
}

// splitIPs splits a comma-separated list of IPs
func splitIPs(ipList string) []string {
	var ips []string
	for _, ip := range strings.Split(ipList, ",") {
		if trimmed := strings.TrimSpace(ip); trimmed != "" {
			ips = append(ips, trimmed)
		}
	}
	return ips
}
