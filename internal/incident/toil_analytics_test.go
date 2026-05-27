package incident

import (
	"testing"
	"time"
)

func TestCalculateToilSummary(t *testing.T) {
	service := newTestService(t)
	
	// Mock incidents with toil
	incidents := []Incident{
		{
			ID:        "INC-1",
			Service:   "order_api",
			Profile:   "testing",
			CreatedAt: time.Now(),
			Toil: &ToilMetadata{
				Minutes:  60,
				Category: "manual_restart",
			},
			Analysis: &IncidentAnalysis{
				Pattern: "connection_timeout",
			},
		},
		{
			ID:        "INC-2",
			Service:   "order_api",
			Profile:   "testing",
			CreatedAt: time.Now(),
			Toil: &ToilMetadata{
				Minutes:  120,
				Category: "manual_restart",
			},
			Analysis: &IncidentAnalysis{
				Pattern: "connection_timeout",
			},
		},
	}

	summary := service.CalculateToilSummary(incidents, "Order Team", "testing", 30)

	if summary.TotalToilHours != 3.0 {
		t.Errorf("expected 3.0 hours, got %f", summary.TotalToilHours)
	}

	// Cost: 3 hours * $100 (default) = $300
	if summary.ToilCostUSD != 300.0 {
		t.Errorf("expected $300.0, got $%f", summary.ToilCostUSD)
	}

	if len(summary.ROISuggestions) == 0 {
		t.Fatal("expected ROI suggestions")
	}

	sug := summary.ROISuggestions[0]
	if sug.SavingsUSD != 300.0 {
		t.Errorf("expected $300.0 savings, got $%f", sug.SavingsUSD)
	}

	if sug.Evidence == nil || sug.Evidence.LogPattern != "connection_timeout" {
		t.Errorf("expected evidence pattern 'connection_timeout'")
	}
}

func TestOrgAggregation(t *testing.T) {
	service := newTestService(t)
	
	incidents := []Incident{
		{ID: "I1", Profile: "team-a", Toil: &ToilMetadata{Minutes: 60}},
		{ID: "I2", Profile: "team-b", Toil: &ToilMetadata{Minutes: 60}},
	}

	summary := service.CalculateOrgToilSummary(incidents, 30)
	
	if summary.TotalToilHours != 2.0 {
		t.Errorf("expected 2.0 hours, got %f", summary.TotalToilHours)
	}
	
	if summary.Team != "Organization" {
		t.Errorf("expected Organization team, got %s", summary.Team)
	}
}
