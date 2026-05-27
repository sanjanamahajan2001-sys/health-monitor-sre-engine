package scorecard

import (
	"time"
	"health-monitor/internal/incident"
)

// MonthlyReport represents the aggregated data for a specific profile and month
type MonthlyReport struct {
	Profile       string            `json:"profile"`
	Month         string            `json:"month"` // "February 2026"
	OverallScore  float64           `json:"overall_score"`
	HealthStatus  string            `json:"health_status"` // Healthy, At Risk, Critical
	NotAvailableReason string       `json:"not_available_reason,omitempty"`
	ProfileCreatedAt   time.Time    `json:"profile_created_at,omitempty"`
	
	ServiceSummaries []ServiceSummary   `json:"service_summaries"`
	ErrorBudgets    []ErrorBudgetRow   `json:"error_budgets"`
	CategoryStats   []CategoryStat     `json:"category_stats"`
	MLInsights      MLInsights         `json:"ml_insights"`
	NextActionItems []ActionItemRow    `json:"next_action_items"`
	ExecutiveSummary ExecutiveSummary  `json:"executive_summary"`
	ServiceSpecific  *ServiceSpecificStats `json:"service_specific,omitempty"`
	
	// NEW: Trends, Toil and Ranking
	Trends          *PerformanceTrends `json:"trends,omitempty"`
	Toil            *ToilMetrics       `json:"toil,omitempty"`
	Ranking         *TeamRanking       `json:"ranking,omitempty"`
}

// OrgReport represents the aggregated data across all profiles
type OrgReport struct {
	Month           string           `json:"month"`
	NotAvailableReason string        `json:"not_available_reason,omitempty"`
	OverallScore    float64          `json:"overall_score"`
	HealthStatus    string           `json:"health_status"`
	TotalProfiles   int              `json:"total_profiles"`
	TotalIncidents  int              `json:"total_incidents"`
	P1Incidents     int              `json:"p1_incidents"`
	P2Incidents     int              `json:"p2_incidents"`
	AvgMTTR         string           `json:"avg_mttr"`
	AvgMTTA         string           `json:"avg_mtta"` // Mean Time to Acknowledge
	GlobalMTBF      string           `json:"global_mtbf"`
	GlobalAvailability float64       `json:"global_availability"`
	GlobalToilHours float64          `json:"global_toil_hours"`
	GlobalToilCost  float64          `json:"global_toil_cost"` // New
	GlobalPotentialSavings float64   `json:"global_potential_savings"` // New
	GlobalToilTrend string           `json:"global_toil_trend"` // New: Improving, Degrading
	GlobalToilPercent float64        `json:"global_toil_percent"`
	GlobalToilTarget  float64        `json:"global_toil_target"`
	ToilStatus      string           `json:"toil_status"`
	GlobalSLORate   float64          `json:"global_slo_rate"` // % of SLOs compliant
	GlobalBurnRate  float64          `json:"global_burn_rate"` // Average budget burn rate
	
	// NEW: Organization Data Summary
	TotalServices    int             `json:"total_services"`
	TotalSLOs        int             `json:"total_slos"`
	TotalActionItems int             `json:"total_action_items"`
	OverdueActions   int             `json:"overdue_actions"`
	
	Trends          *PerformanceTrends `json:"trends,omitempty"`
	ProfileSummaries []ProfileSummary `json:"profile_summaries"`
	Hotspots         []OrgHotspot     `json:"hotspots"`
	CategoryStats    []CategoryStat   `json:"category_stats"`
}

type ProfileSummary struct {
	Name         string  `json:"name"`
	Score        float64 `json:"score"`
	HealthStatus string  `json:"health_status"`
	P1Incidents  int     `json:"p1_incidents"`
	Availability float64 `json:"availability"`
	Trend        string  `json:"trend"` // Up, Down, Stable
}

type OrgHotspot struct {
	Service     string `json:"service"`
	Profile     string `json:"profile"`
	P1Incidents int    `json:"p1_incidents"`
	Status      string `json:"status"` // E.g., "Critical"
}

type PerformanceTrends struct {
	SLOComplianceDelta float64 `json:"slo_compliance_delta"` // e.g., -0.02 for -2%
	IncidentCountDelta int     `json:"incident_count_delta"`
	MTTRDeltaMinutes   int     `json:"mttr_delta_minutes"`
	ScoreDelta         float64 `json:"score_delta"`
}

type ToilMetrics struct {
	TotalHours       float64 `json:"total_hours"`
	CostUSD          float64 `json:"cost_usd"` // New
	MonthlySavingsUSD float64 `json:"monthly_savings_usd"` // New
	PercentageOfTime float64 `json:"percentage_of_time"`
	TargetPercentage float64 `json:"target_percentage"`
	Status           string  `json:"status"` // "Healthy", "Over Target"
	DataConfidenceScore float64 `json:"data_confidence_score"` // New
	Trend            string  `json:"trend"` // New
}

type TeamRanking struct {
	Position int `json:"position"`
	TotalTeams int `json:"total_teams"`
	Trend      string `json:"trend"` // "up", "down", "stable"
}

type ServiceSpecificStats struct {
	ServiceName     string             `json:"service_name"`
	RecentIncidents []incident.Incident `json:"recent_incidents"` // Past 7 days
	RecentActions   []ActionItemRow    `json:"recent_actions"`
	UpcomingSLORisk []ErrorBudgetRow   `json:"upcoming_slo_risk"`
	Availability    float64            `json:"availability"`
	MTTR            string             `json:"mttr"`
	MTBF            string             `json:"mtbf"`
	UpstreamDeps    []string           `json:"upstream_deps"`
	DownstreamDeps  []string           `json:"downstream_deps"`
}

type ServiceSummary struct {
	Team            string  `json:"team"`
	Service         string  `json:"service"`
	Flow            string  `json:"flow"`
	Availability    float64 `json:"availability"`
	P1Incidents     int     `json:"p1_incidents"`
	P2Incidents     int     `json:"p2_incidents"`
	MTTR            string  `json:"mttr"`
	MTBF            string  `json:"mtbf"`
	ActionItemPerf  string  `json:"action_item_perf"` // "80% Done"
	Trend           string  `json:"trend"`           // "Improving", "Degrading", "Stable"
	RecommendedRunbook string `json:"recommended_runbook"`
}

type ErrorBudgetRow struct {
	SLOID           string  `json:"slo_id"`
	Service         string  `json:"service"`
	TargetSLO       float64 `json:"target_slo"`
	Actual          float64 `json:"actual"`
	BudgetRemaining float64 `json:"budget_remaining"`
	Status          string  `json:"status"` // Healthy, Breaching
}

type CategoryStat struct {
	Category string  `json:"category"`
	Count    int     `json:"count"`
	Percent  float64 `json:"percent"`
	Trend    string  `json:"trend"` // ↑, ↓, →
}

type MLInsights struct {
	TopSemanticFeature string  `json:"top_semantic_feature"`
	MostFrequentPattern string  `json:"most_frequent_pattern"`
	AvgConfidence      float64 `json:"avg_confidence"`
	ActionableHintRate float64 `json:"actionable_hint_rate"`
}

type ActionItemRow struct {
	IncidentID  string    `json:"incident_id"`
	Priority    string    `json:"priority"`
	Service     string    `json:"service"`
	Description string    `json:"description"`
	Owner       string    `json:"owner"`
	Status      string    `json:"status"` // "Pending", "Completed", "Overdue"
	DueDate     time.Time `json:"due_date"`
}

type ExecutiveSummary struct {
	ReliabilityTrend string `json:"reliability_trend"`
	PrimaryRisk      string `json:"primary_risk"`
	FrequentPattern  string `json:"frequent_pattern"`
	FocusArea        string `json:"focus_area"`
}
