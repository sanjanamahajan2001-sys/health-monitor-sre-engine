package traces

import (
	"context"
	"testing"
	"time"

	"health-monitor/internal/config"
)

func TestNewCorrelator(t *testing.T) {
	// Test with empty config
	cfg := config.Config{}
	_, err := NewCorrelator(cfg)
	if err == nil {
		t.Error("Expected error for empty config, got nil")
	}

	// Test with tempo backend but no URL
	cfg = config.Config{
		TraceBackend: "tempo",
		TraceURL:     "",
	}
	_, err = NewCorrelator(cfg)
	if err == nil {
		t.Error("Expected error for missing URL, got nil")
	}

	// Test with valid tempo config
	cfg = config.Config{
		TraceBackend: "tempo",
		TraceURL:     "http://localhost:3200",
	}
	correlator, err := NewCorrelator(cfg)
	if err != nil {
		t.Errorf("Expected no error for valid config, got %v", err)
	}
	if correlator == nil {
		t.Error("Expected correlator instance, got nil")
	}
}

func TestCorrelatorCorrelate(t *testing.T) {
	// Test with mock backend - this would require implementing a mock backend
	// For now, we test the error path
	cfg := config.Config{
		TraceBackend: "invalid",
	}
	correlator, err := NewCorrelator(cfg)
	if err == nil {
		t.Error("Expected error for invalid backend, got nil")
	}

	// Test correlation with no backend
	cfg = config.Config{}
	correlator, _ = NewCorrelator(cfg)
	
	ctx := context.Background()
	summary, _ := correlator.Correlate(ctx, "test-service", time.Now())
	
	if summary != nil {
		t.Error("Expected nil summary for no backend, got non-nil")
	}
}

func TestIsSystemRoute(t *testing.T) {
	tests := []struct {
		route     string
		expected  bool
	}{
		{"/health", true},
		{"/healthz", true},
		{"/ready", true},
		{"/readyz", true},
		{"/metrics", true},
		{"/api/users", false},
		{"/checkout", false},
		{"/api/v1/orders", false},
	}

	for _, test := range tests {
		result := IsSystemRoute(test.route)
		if result != test.expected {
			t.Errorf("IsSystemRoute(%s) = %v, expected %v", test.route, result, test.expected)
		}
	}
}

func TestExtractRouteFromSpan(t *testing.T) {
	tests := []struct {
		attributes []OTLPAttribute
		expected  string
	}{
		{
			attributes: []OTLPAttribute{
				{Key: "http.route", Value: OTLPValue{StringValue: "/api/users"}},
			},
			expected: "/api/users",
		},
		{
			attributes: []OTLPAttribute{
				{Key: "http.target", Value: OTLPValue{StringValue: "/checkout"}},
			},
			expected: "/checkout",
		},
		{
			attributes: []OTLPAttribute{
				{Key: "service.name", Value: OTLPValue{StringValue: "user-service"}},
			},
			expected: "",
		},
	}

	for _, test := range tests {
		result := ExtractRouteFromSpan(test.attributes)
		if result != test.expected {
			t.Errorf("ExtractRouteFromSpan() = %s, expected %s", result, test.expected)
		}
	}
}

func TestExtractServiceFromSpan(t *testing.T) {
	tests := []struct {
		attributes []OTLPAttribute
		expected  string
	}{
		{
			attributes: []OTLPAttribute{
				{Key: "service.name", Value: OTLPValue{StringValue: "billing-api"}},
			},
			expected: "billing-api",
		},
		{
			attributes: []OTLPAttribute{
				{Key: "faas.name", Value: OTLPValue{StringValue: "process-order"}},
			},
			expected: "process-order",
		},
		{
			attributes: []OTLPAttribute{
				{Key: "http.route", Value: OTLPValue{StringValue: "/api/users"}},
			},
			expected: "",
		},
	}

	for _, test := range tests {
		result := ExtractServiceFromSpan(test.attributes)
		if result != test.expected {
			t.Errorf("ExtractServiceFromSpan() = %s, expected %s", result, test.expected)
		}
	}
}

func TestIsErrorSpan(t *testing.T) {
	tests := []struct {
		span     TempoSpan
		expected bool
	}{
		{
			span: TempoSpan{
				Status: TempoStatus{Code: 2}, // Error status
			},
			expected: true,
		},
		{
			span: TempoSpan{
				Status: TempoStatus{Code: 1}, // OK status
			},
			expected: false,
		},
		{
			span: TempoSpan{
				Name: "database.error",
			},
			expected: true,
		},
		{
			span: TempoSpan{
				Name: "http.request",
			},
			expected: false,
		},
	}

	for _, test := range tests {
		result := IsErrorSpan(test.span)
		if result != test.expected {
			t.Errorf("IsErrorSpan() = %v, expected %v", result, test.expected)
		}
	}
}
