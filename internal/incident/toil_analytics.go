package incident

import (
	"fmt"
	"sort"
	"time"

	"health-monitor/internal/config"
	"health-monitor/internal/ml"
)

// ToilSummary represents the aggregated toil data for a team or window
type ToilSummary struct {
	Team               string
	Profile            string
	IncidentID          string
	WindowDays          int
	TotalToilHours     float64
	ToilCostUSD        float64 // New: Cost of manual work
	PotentialSavingsUSD float64 // New: Potential ROI
	AverageHourlyCost   float64 // New: Used for ROI calculations in CLI
	CategoryBreakdown  []ToilCategoryStat
	ROISuggestions     []ROISuggestion
	AuditTrail         []ToilAuditEntry // New: Traceability for accuracy
}

// ToilAuditEntry captures the source and validation of a toil record
type ToilAuditEntry struct {
	IncidentID string
	Source     string // e.g., "manual", "auto-estimated"
	User       string
	Minutes    int
	Timestamp  time.Time
}

// ToilCategoryStat represents toil metrics for a single category
type ToilCategoryStat struct {
	Category   string
	Hours      float64
	Percentage float64
}

// ROISuggestion represents a suggestion for automation ROI
type ROISuggestion struct {
	Category         string
	PotentialSavings float64 // Hours saved per month
	SavingsUSD       float64 // New: Financial impact
	Recommendation   string
	Evidence         *AutomationEvidence // New: Data-driven justification
}

// AutomationEvidence provides the data-driven "why" behind a recommendation
type AutomationEvidence struct {
	LogPattern      string
	OccurrenceCount int
	ServiceContext  string
	ConfidenceScore float64 // 0.0 - 1.0 based on data quality
}

// CalculateToilSummary aggregates toil data from a list of incidents
func (s *Service) CalculateToilSummary(incidents []Incident, team string, profile string, windowDays int) ToilSummary {
	var totalMinutes int
	categoryMinutes := make(map[string]int)
	var auditTrail []ToilAuditEntry
	
	// Map incident ID to its lifecycle for deep analysis
	lifecycles := make(map[string]*ml.FullLifecycle)
	// Get hourly cost from config
	cfg, _ := config.LoadForProfile(profile)
	hourlyCost := cfg.TeamCapacity.AverageHourlyCost
	if hourlyCost <= 0 {
		hourlyCost = 100.0 // Fallback
	}

	var incidentID string
	if len(incidents) == 1 {
		incidentID = incidents[0].ID
	}

	for _, inc := range incidents {
		if inc.Toil == nil || inc.Toil.Minutes <= 0 {
			continue
		}
		
		totalMinutes += inc.Toil.Minutes
		category := inc.Toil.Category
		if category == "" {
			category = "unknown"
		}
		categoryMinutes[category] += inc.Toil.Minutes

		// Build audit entry for accuracy verification
		source := "manual"
		if val, ok := inc.Metadata["toil_source"]; ok {
			source = val
		}
		
		auditTrail = append(auditTrail, ToilAuditEntry{
			IncidentID: inc.ID,
			Source:     source,
			User:       inc.Profile, // Use profile as proxy for team context
			Minutes:    inc.Toil.Minutes,
			Timestamp:  inc.CreatedAt,
		})

		// Try to load lifecycle for richer evidence
		if s.mlRecorder != nil {
			if lc, err := s.mlRecorder.LoadLifecycle(inc.ID); err == nil && lc != nil {
				lifecycles[inc.ID] = lc
			}
		}
	}

	totalHours := float64(totalMinutes) / 60.0
	summary := ToilSummary{
		Team:           team,
		Profile:        profile,
		IncidentID:     incidentID,
		WindowDays:     windowDays,
		TotalToilHours: totalHours,
		ToilCostUSD:    totalHours * hourlyCost,
		AverageHourlyCost: hourlyCost,
		AuditTrail:     auditTrail,
	}

	// Calculate breakdown
	for cat, mins := range categoryMinutes {
		hours := float64(mins) / 60.0
		percentage := 0.0
		if totalMinutes > 0 {
			percentage = (float64(mins) / float64(totalMinutes)) * 100
		}
		summary.CategoryBreakdown = append(summary.CategoryBreakdown, ToilCategoryStat{
			Category:   cat,
			Hours:      hours,
			Percentage: percentage,
		})
	}

	// Sort breakdown by hours descending
	sort.Slice(summary.CategoryBreakdown, func(i, j int) bool {
		return summary.CategoryBreakdown[i].Hours > summary.CategoryBreakdown[j].Hours
	})

	// Generate ROI suggestions with evidence
	summary.ROISuggestions = s.generateROISuggestions(summary.CategoryBreakdown, incidents, lifecycles, hourlyCost)
	
	for _, sugg := range summary.ROISuggestions {
		summary.PotentialSavingsUSD += sugg.SavingsUSD
	}

	return summary
}

// CalculateOrgToilSummary aggregates toil across ALL teams/profiles
func (s *Service) CalculateOrgToilSummary(allIncidents []Incident, windowDays int) ToilSummary {
	orgSummary := ToilSummary{
		Team:       "Organization",
		WindowDays: windowDays,
	}

	// Group incidents by profile to respect individual team costs
	profileIncidents := make(map[string][]Incident)
	for _, inc := range allIncidents {
		profileIncidents[inc.Profile] = append(profileIncidents[inc.Profile], inc)
	}

	// Fetch lifecycles for ALL incidents to ensure global evidence
	allLifecycles := make(map[string]*ml.FullLifecycle)
	for _, inc := range allIncidents {
		if lc, err := s.mlRecorder.LoadLifecycle(inc.ID); err == nil && lc != nil {
			allLifecycles[inc.ID] = lc
		}
	}

	for profile, incs := range profileIncidents {
		profileSummary := s.CalculateToilSummary(incs, "", profile, windowDays)
		orgSummary.TotalToilHours += profileSummary.TotalToilHours
		orgSummary.ToilCostUSD += profileSummary.ToilCostUSD
		orgSummary.PotentialSavingsUSD += profileSummary.PotentialSavingsUSD
		orgSummary.CategoryBreakdown = append(orgSummary.CategoryBreakdown, profileSummary.CategoryBreakdown...)
		orgSummary.AuditTrail = append(orgSummary.AuditTrail, profileSummary.AuditTrail...)
	}

	if orgSummary.TotalToilHours > 0 {
		orgSummary.AverageHourlyCost = orgSummary.ToilCostUSD / orgSummary.TotalToilHours
	} else {
		orgSummary.AverageHourlyCost = 100.0 // Global Fallback
	}

	// Re-aggregate CategoryBreakdown
	catMap := make(map[string]float64)
	for _, stat := range orgSummary.CategoryBreakdown {
		catMap[stat.Category] += stat.Hours
	}
	
	orgSummary.CategoryBreakdown = nil
	var stats []ToilCategoryStat
	for cat, hours := range catMap {
		percentage := 0.0
		if orgSummary.TotalToilHours > 0 {
			percentage = (hours / orgSummary.TotalToilHours) * 100
		}
		stat := ToilCategoryStat{
			Category:   cat,
			Hours:      hours,
			Percentage: percentage,
		}
		orgSummary.CategoryBreakdown = append(orgSummary.CategoryBreakdown, stat)
		stats = append(stats, stat)
	}
	
	sort.Slice(orgSummary.CategoryBreakdown, func(i, j int) bool {
		return orgSummary.CategoryBreakdown[i].Hours > orgSummary.CategoryBreakdown[j].Hours
	})

	// 🚀 Unified Organizational ROI Analysis
	orgSummary.ROISuggestions = s.generateROISuggestions(stats, allIncidents, allLifecycles, orgSummary.AverageHourlyCost)

	return orgSummary
}

func (s *Service) generateROISuggestions(stats []ToilCategoryStat, incidents []Incident, lifecycles map[string]*ml.FullLifecycle, hourlyCost float64) []ROISuggestion {
	var suggestions []ROISuggestion
	
	// Map to track evidence per category
	type evidenceKey struct {
		Category string
		Service  string
	}
	evidenceMap := make(map[evidenceKey]*AutomationEvidence)

	for _, inc := range incidents {
		if inc.Toil == nil { continue }
		cat := inc.Toil.Category
		if cat == "" { cat = "unknown" }
		
		key := evidenceKey{Category: cat, Service: inc.Service}
		if _, ok := evidenceMap[key]; !ok {
			evidenceMap[key] = &AutomationEvidence{
				ServiceContext: inc.Service,
				ConfidenceScore: 0.5, // Base confidence
			}
		}
		
		ev := evidenceMap[key]
		ev.OccurrenceCount++
		
		// Pull "Data-Driven" log pattern from analysis if available
		if inc.Analysis != nil {
			if inc.Analysis.Pattern != "" {
				ev.LogPattern = inc.Analysis.Pattern
				ev.ConfidenceScore = 0.9 
			} else if inc.Analysis.ErrorSignature != "" {
				ev.LogPattern = inc.Analysis.ErrorSignature
				ev.ConfidenceScore = 0.8
			}
		}

		// 🚀 NEW: Overwrite with deeper Lifecycle evidence if available
		if lc, ok := lifecycles[inc.ID]; ok {
			for _, snap := range lc.Snapshots {
				if snap.Phase == "impact" && snap.FullLogs != nil && len(snap.FullLogs.TopErrors) > 0 {
					top := snap.FullLogs.TopErrors[0]
					ev.LogPattern = normalizePattern(top.Message)
					ev.ConfidenceScore = 1.0 // Maximum confidence from direct impact logs
					break
				}
			}
		}
	}

	// Suggest automation for top categories
	for i, stat := range stats {
		if i >= 3 {
			break
		}
		
		savings := stat.Hours 
		
		// Find best evidence for this category (Deterministically)
		var bestEv *AutomationEvidence
		// To keep it stable, we iterate over sorted keys if we had them, OR just use a stable comparison
		for key, ev := range evidenceMap {
			if key.Category == stat.Category {
				if bestEv == nil || ev.OccurrenceCount > bestEv.OccurrenceCount || 
					(ev.OccurrenceCount == bestEv.OccurrenceCount && key.Service < bestEv.ServiceContext) {
					bestEv = ev
				}
			}
		}
		
		topSvc := ""
		if bestEv != nil {
			topSvc = bestEv.ServiceContext
		}
		
		svcContext := ""
		if topSvc != "" {
			svcContext = fmt.Sprintf(" (Primary: %s)", topSvc)
		}
		
		patternSuffix := ""
		if bestEv != nil && bestEv.LogPattern != "" {
			patternSuffix = fmt.Sprintf(" (Evidence: detected pattern '%s' across %d incidents)", bestEv.LogPattern, bestEv.OccurrenceCount)
		}
		
		rec := ""
		patternPrefix := ""
		if bestEv != nil && bestEv.LogPattern != "" {
			patternPrefix = fmt.Sprintf("Analyze and automate the response to '%s' errors. ", bestEv.LogPattern)
		}

		switch stat.Category {
		case "manual_restart", "restart":
			rec = fmt.Sprintf("%sImplement self-healing with Kubernetes liveness/readiness probes or a custom operator to automate recovery for %s.%s", patternPrefix, topSvc, patternSuffix)
		case "alert_ack", "noisy_alerts":
			rec = fmt.Sprintf("Tune Prometheus alert thresholds for %s to reduce noisy notifications. Consider automated alert grouping/suppression.%s", topSvc, patternSuffix)
		case "config_change", "deployment":
			rec = fmt.Sprintf("Transition %s manual configuration updates to a GitOps pattern (ArgoCD/Flux) to ensure consistent, automated rollouts.%s", topSvc, patternSuffix)
		case "capacity", "capacity_mgmt", "scaling":
			rec = fmt.Sprintf("%sScale %s automatically using Horizontal Pod Autoscaling (HPA) or cluster-level autoscaling based on observed demand peaks.%s", patternPrefix, topSvc, patternSuffix)
		case "manual_investigation", "investigation":
			rec = fmt.Sprintf("%sAugment %s observability with specialized dashboards and automated diagnostic hooks (e.g., auto-capturing traces) in the incident flow.%s", patternPrefix, topSvc, patternSuffix)
		case "patching", "maintenance":
			rec = fmt.Sprintf("Replace manual SSH-based patching with an automated fleet management solution like AWS Systems Manager or Ansible.%s", patternSuffix)
		default:
			rec = fmt.Sprintf("%sStandardize the %s workflow for %s by creating an automated runbook or script to handle repetitive tasks.%s", patternPrefix, stat.Category, topSvc, patternSuffix)
		}

		suggestions = append(suggestions, ROISuggestion{
			Category:         stat.Category + svcContext,
			Recommendation:   rec,
			PotentialSavings: savings,
			SavingsUSD:       savings * hourlyCost,
			Evidence:         bestEv,
		})
	}

	return suggestions
}
