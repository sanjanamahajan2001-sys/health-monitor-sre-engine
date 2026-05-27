package security

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/time/rate"
)

// SecurityMetrics holds security-related metrics
type SecurityMetrics struct {
	AuthAttempts      *prometheus.CounterVec
	AuthFailures      *prometheus.CounterVec
	AuthSuccesses     *prometheus.CounterVec
	RateLimitHits     *prometheus.CounterVec
	ValidationFailures *prometheus.CounterVec
	IPBlocked         *prometheus.CounterVec
}

// NewSecurityMetrics creates new security metrics
func NewSecurityMetrics() *SecurityMetrics {
	return &SecurityMetrics{
		AuthAttempts: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "security_auth_attempts_total",
				Help: "Total number of authentication attempts",
			},
			[]string{"method", "status"},
		),
		AuthFailures: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "security_auth_failures_total",
				Help: "Total number of authentication failures",
			},
			[]string{"reason", "method"},
		),
		AuthSuccesses: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "security_auth_successes_total",
				Help: "Total number of successful authentications",
			},
			[]string{"method"},
		),
		RateLimitHits: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "security_rate_limit_hits_total",
				Help: "Total number of rate limit violations",
			},
			[]string{"client"},
		),
		ValidationFailures: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "security_validation_failures_total",
				Help: "Total number of validation failures",
			},
			[]string{"field", "reason"},
		),
		IPBlocked: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "security_ip_blocked_total",
				Help: "Total number of blocked IP addresses",
			},
			[]string{"ip"},
		),
	}
}

// Register registers all security metrics with Prometheus
func (m *SecurityMetrics) Register() {
	prometheus.MustRegister(m.AuthAttempts)
	prometheus.MustRegister(m.AuthFailures)
	prometheus.MustRegister(m.AuthSuccesses)
	prometheus.MustRegister(m.RateLimitHits)
	prometheus.MustRegister(m.ValidationFailures)
	prometheus.MustRegister(m.IPBlocked)
}

// SecurityConfig interface for configuration
type SecurityConfig interface {
	IsWebhookAuthEnabled() bool
	ValidateToken(token string) bool
	IsIPAllowed(ip string) bool
	IsRateLimitEnabled() bool
	GetRateLimiter() *rate.Limiter
	IsValidationEnabled() bool
	ValidatePayload(payload []byte) error
}

// AuthMiddleware provides authentication middleware
type AuthMiddleware struct {
	config  SecurityConfig
	metrics *SecurityMetrics
}

// NewAuthMiddleware creates a new authentication middleware
func NewAuthMiddleware(config SecurityConfig, metrics *SecurityMetrics) *AuthMiddleware {
	return &AuthMiddleware{
		config:  config,
		metrics: metrics,
	}
}

// Middleware returns the HTTP middleware function
func (a *AuthMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Record authentication attempt
		a.metrics.AuthAttempts.WithLabelValues("webhook", "attempt").Inc()

		// Check IP whitelist first
		if !a.config.IsIPAllowed(getClientIP(r)) {
			a.metrics.IPBlocked.WithLabelValues(getClientIP(r)).Inc()
			a.metrics.AuthFailures.WithLabelValues("ip_blocked", "webhook").Inc()
			http.Error(w, "IP address not allowed", http.StatusForbidden)
			return
		}

		// Check rate limiting
		if a.config.IsRateLimitEnabled() {
			limiter := a.config.GetRateLimiter()
			if !limiter.Allow() {
				a.metrics.RateLimitHits.WithLabelValues(getClientIP(r)).Inc()
				http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
				return
			}
		}

		// Check authentication if enabled
		if a.config.IsWebhookAuthEnabled() {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				a.metrics.AuthFailures.WithLabelValues("missing_token", "webhook").Inc()
				http.Error(w, "Authorization header required", http.StatusUnauthorized)
				return
			}

			if !a.config.ValidateToken(authHeader) {
				a.metrics.AuthFailures.WithLabelValues("invalid_token", "webhook").Inc()
				http.Error(w, "Invalid authentication token", http.StatusUnauthorized)
				return
			}
		}

		// Authentication successful
		a.metrics.AuthSuccesses.WithLabelValues("webhook").Inc()
		next.ServeHTTP(w, r)
	})
}

// ValidationMiddleware provides payload validation middleware
type ValidationMiddleware struct {
	config  SecurityConfig
	metrics *SecurityMetrics
}

// NewValidationMiddleware creates a new validation middleware
func NewValidationMiddleware(config SecurityConfig, metrics *SecurityMetrics) *ValidationMiddleware {
	return &ValidationMiddleware{
		config:  config,
		metrics: metrics,
	}
}

// Middleware returns the HTTP middleware function
func (v *ValidationMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !v.config.IsValidationEnabled() {
			next.ServeHTTP(w, r)
			return
		}

		// Only validate POST requests with content
		if r.Method == "POST" && r.ContentLength > 0 {
			// Read and validate payload
			buf, err := io.ReadAll(r.Body)
			if err != nil {
				v.metrics.ValidationFailures.WithLabelValues("body", "read_error").Inc()
				http.Error(w, "Failed to read request body", http.StatusBadRequest)
				return
			}

			if err := v.config.ValidatePayload(buf); err != nil {
				v.metrics.ValidationFailures.WithLabelValues("payload", err.Error()).Inc()
				http.Error(w, fmt.Sprintf("Validation failed: %s", err), http.StatusBadRequest)
				return
			}

			// Restore the body for the next handler
			r.Body = &requestBody{buf: buf, r: r}
		}

		next.ServeHTTP(w, r)
	})
}

// requestBody is a wrapper to restore request body
type requestBody struct {
	buf []byte
	r   *http.Request
	pos int
}

func (rb *requestBody) Read(p []byte) (n int, err error) {
	if rb.pos >= len(rb.buf) {
		return 0, io.EOF  // Return EOF to signal end of body
	}
	n = copy(p, rb.buf[rb.pos:])
	rb.pos += n
	return n, nil
}

func (rb *requestBody) Close() error {
	return nil
}

// getClientIP extracts the real client IP address
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	// Fall back to RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// RateLimiterManager manages rate limiters for different clients
type RateLimiterManager struct {
	limiters map[string]*rate.Limiter
	mutex    sync.RWMutex
	config   RateLimitConfig
}

// RateLimitConfig interface for rate limit configuration
type RateLimitConfig interface {
	GetRequestsPerMinute() int
	GetBurstSize() int
}

// NewRateLimiterManager creates a new rate limiter manager
func NewRateLimiterManager(config RateLimitConfig) *RateLimiterManager {
	return &RateLimiterManager{
		limiters: make(map[string]*rate.Limiter),
		config:   config,
	}
}

// GetLimiter gets or creates a rate limiter for a client
func (rm *RateLimiterManager) GetLimiter(clientIP string) *rate.Limiter {
	rm.mutex.RLock()
	limiter, exists := rm.limiters[clientIP]
	rm.mutex.RUnlock()

	if !exists {
		rm.mutex.Lock()
		defer rm.mutex.Unlock()

		// Double-check after acquiring write lock
		if limiter, exists := rm.limiters[clientIP]; exists {
			return limiter
		}

		// Create new limiter
		rps := float64(rm.config.GetRequestsPerMinute()) / 60.0
		limiter = rate.NewLimiter(rate.Limit(rps), rm.config.GetBurstSize())
		rm.limiters[clientIP] = limiter
	}

	return limiter
}

// CleanupOldLimiters removes limiters that haven't been used recently
func (rm *RateLimiterManager) CleanupOldLimiters(maxAge time.Duration) {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	// This is a simplified cleanup - in production, you'd want to track last usage time
	if len(rm.limiters) > 1000 { // Prevent unlimited growth
		// Clear all limiters (simplified approach)
		rm.limiters = make(map[string]*rate.Limiter)
	}
}

// SecurityContext provides security context for requests
type SecurityContext struct {
	User        string
	Permissions []string
	Authenticated bool
	ClientIP    string
	Token       string
}

// ContextKey is the type for context keys
type ContextKey string

const SecurityContextKey ContextKey = "security_context"

// SetSecurityContext sets security context in the request context
func SetSecurityContext(ctx context.Context, secCtx *SecurityContext) context.Context {
	return context.WithValue(ctx, SecurityContextKey, secCtx)
}

// GetSecurityContext gets security context from the request context
func GetSecurityContext(ctx context.Context) *SecurityContext {
	if secCtx, ok := ctx.Value(SecurityContextKey).(*SecurityContext); ok {
		return secCtx
	}
	return &SecurityContext{}
}
