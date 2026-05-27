package scorecard

import (
	"fmt"
	"strings"
	"time"

	"health-monitor/internal/config"
	"health-monitor/internal/incident"
	"health-monitor/internal/slo"
	"health-monitor/internal/flow"
	"os"
	"path/filepath"
	"encoding/json"
	"sort"
)

type Provider struct {
	incidentStore *incident.Store
	sloService    *slo.Service
	TargetService string
}

func NewProvider(is *incident.Store, ss *slo.Service) *Provider {
	return &Provider{
		incidentStore: is,
		sloService:    ss,
	}
}

func (p *Provider) getProfileCreationDate(profile string) time.Time {
	pm := config.GetProfileManager()
	path := pm.GetProfilePath(profile)
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

func (p *Provider) getEarliestServiceData(service string) time.Time {
	all, _, _ := p.incidentStore.List()
	var earliest time.Time
	for _, inc := range all {
		if inc.Service == service {
			if earliest.IsZero() || inc.CreatedAt.Before(earliest) {
				earliest = inc.CreatedAt
			}
		}
	}
	return earliest
}

func (p *Provider) GenerateReport(profile string, year int, month time.Month) (*MonthlyReport, error) {
	return p.generateReportInternal(profile, year, month, true)
}

func (p *Provider) generateReportInternal(profile string, year int, month time.Month, includeTrends bool) (*MonthlyReport, error) {
	startTime := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	endTime := startTime.AddDate(0, 1, 0).Add(-time.Nanosecond)

	report := &MonthlyReport{
		Profile: profile,
		Month:   fmt.Sprintf("%s %d", month.String(), year),
	}

	// Awareness Check: Did this profile/service exist then?
	requestedMonthDate := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	createdDate := p.getProfileCreationDate(profile)
	if !createdDate.IsZero() {
		creationMonth := time.Date(createdDate.Year(), createdDate.Month(), 1, 0, 0, 0, 0, time.UTC)
		if requestedMonthDate.Before(creationMonth) {
			report.NotAvailableReason = fmt.Sprintf("This profile was created in %s. Previous month scorecard is not available.", createdDate.Format("January"))
			return report, nil
		}
	}

	if p.TargetService != "" {
		earliest := p.getEarliestServiceData(p.TargetService)
		if !earliest.IsZero() {
			earliestMonth := time.Date(earliest.Year(), earliest.Month(), 1, 0, 0, 0, 0, time.UTC)
			if requestedMonthDate.Before(earliestMonth) {
				report.NotAvailableReason = fmt.Sprintf("This service was created in %s. Scorecard data for the previous month is not available.", earliest.Format("January 2006"))
				return report, nil
			}
		}
	}

	// 1. Gather Incidents (Optimized list for target month)
	allIncidents, _, err := p.incidentStore.ListForMonth(month, year)
	if err != nil {
		return nil, fmt.Errorf("failed to list incidents: %w", err)
	}

	monthlyIncidents := allIncidents
	// Filter by service if target is set
	if p.TargetService != "" {
		var filtered []incident.Incident
		for _, inc := range allIncidents {
			if inc.Service == p.TargetService {
				filtered = append(filtered, inc)
			}
		}
		monthlyIncidents = filtered
	}
	_ = endTime // Suppress unused warning, we filter by filename now

	// 2. Gather Flows and Teams (Profile-Aware)
	loadRes, _ := flow.LoadProfileAware()
	flows := loadRes.Flows

	// Filter flows by service if target is set
	if p.TargetService != "" {
		var filtered []flow.Flow
		for _, f := range flows {
			for _, svc := range f.Services {
				if svc == p.TargetService {
					filtered = append(filtered, f)
					break
				}
			}
		}
		flows = filtered
	}

	serviceToTeam := make(map[string]string)
	for _, f := range flows {
		team := f.Metadata["team"]
		if team == "" {
			team = "Unassigned"
		}
		for _, s := range f.Services {
			serviceToTeam[s] = team
		}
	}

	// 3. Aggregate Service Summaries
	svcMap := make(map[string]*ServiceSummary)
	
	// Pre-fetch previous month incidents for trend calculation
	prevMonth := month - 1
	prevYear := year
	if prevMonth == 0 {
		prevMonth = 12
		prevYear--
	}
	prevIncidents, _, _ := p.incidentStore.ListForMonth(prevMonth, prevYear)

	// Pre-initialize with all services from current flows to ensure they appear in Section 1
	for _, f := range flows {
		for _, s := range f.Services {
			if _, ok := svcMap[s]; !ok {
				svcMap[s] = &ServiceSummary{
					Team:    profile,
					Service: s,
					Flow:    f.Name,
				}
			}
		}
	}

	catCount := make(map[string]int)
	var totalConfidence float64
	var actionableHints int
	patternCount := make(map[string]int)

	for _, inc := range monthlyIncidents {
		key := inc.Service
		s, ok := svcMap[key]
		if !ok {
			// Service not in flows but has incidents? (Edge case)
			s = &ServiceSummary{
				Team:    profile,
				Service: inc.Service,
			}
			svcMap[key] = s
		}
		
		if inc.Severity == incident.P1 {
			s.P1Incidents++
		} else if inc.Severity == incident.P2 {
			s.P2Incidents++
		}

		// Category Stats
		if inc.Analysis != nil && inc.Analysis.Category != "" {
			catCount[inc.Analysis.Category]++
		}
		if inc.Analysis != nil && inc.Analysis.Pattern != "" {
			patternCount[inc.Analysis.Pattern]++
		}

		// ML Insights (Metadata is populated by runbook suggest or predict)
		if confStr, ok := inc.Metadata["runbook_confidence"]; ok {
			confStr = strings.TrimSuffix(confStr, "%")
			var conf float64
			fmt.Sscanf(confStr, "%f", &conf)
			// If the value is > 1.0, it's likely already a percentage (e.g., 85.0)
			// If it's <= 1.0, it might be a ratio (e.g., 0.85)
			if conf <= 1.0 && conf > 0 {
				conf = conf * 100
			}
			totalConfidence += conf
			if conf > 80 {
				actionableHints++
			}
		}

		// Collect Action Items (This now moved to a separate global collect below)
	}

	// 3b. Collect ALL pending/backlog action items from complete history
	// This ensures production-grade visibility into all open SRE tasks
	startOfMonth := time.Date(year, month, 1, 0, 0, 0, 0, time.Local)
	allHistory, _, _ := p.incidentStore.ListForMonth(0, 0)
	for _, inc := range allHistory {
		for _, ai := range inc.ActionItems {
			// Include if:
			// 1. Not completed (TODO, IN_PROGRESS)
			// 2. Completed in THIS report month (to show progress)
			
			isDone := ai.Status == incident.ActionItemDone
			isDoneThisMonth := isDone && ai.UpdatedAt.After(startOfMonth)
			
			if !isDone || isDoneThisMonth {
				status := "Pending"
				if isDone {
					status = "Completed"
				} else if !ai.DueDate.IsZero() && ai.DueDate.Before(time.Now()) {
					status = "Overdue"
				}

				report.NextActionItems = append(report.NextActionItems, ActionItemRow{
					IncidentID:  inc.ID,
					Priority:    string(ai.Priority),
					Service:     inc.Service,
					Description: ai.Description,
					Owner:       ai.Owner,
					Status:      status,
					DueDate:     ai.DueDate,
				})
			}
		}
	}

	// 4. ML Insights Calculations
	if len(monthlyIncidents) > 0 {
		report.MLInsights.AvgConfidence = totalConfidence / float64(len(monthlyIncidents))
		report.MLInsights.ActionableHintRate = float64(actionableHints) / float64(len(monthlyIncidents)) * 100
		
		// Find most frequent pattern
		maxP := 0
		for p, count := range patternCount {
			if count > maxP {
				maxP = count
				report.MLInsights.MostFrequentPattern = p
			}
		}
		
		// Find top category
		maxC := 0
		for c, count := range catCount {
			if count > maxC {
				maxC = count
				report.MLInsights.TopSemanticFeature = c
			}
		}
	}

	// 5. Executive Summary
	report.ExecutiveSummary.FrequentPattern = report.MLInsights.TopSemanticFeature
	
	// Find service with most P1s as Primary Risk
	totalMonthSeconds := 30.0 * 24 * 3600 // Approx
	maxP1 := -1
	for _, svc := range svcMap {
		if svc.P1Incidents > maxP1 {
			maxP1 = svc.P1Incidents
			report.ExecutiveSummary.PrimaryRisk = svc.Service
		}
		svc.MTTR = p.calculateMTTR(monthlyIncidents, svc.Service)
		
		// Calculate Availability based on P1/P2 downtime
		var totalDowntime time.Duration
		for _, inc := range monthlyIncidents {
			if inc.Service == svc.Service && (inc.Severity == incident.P1 || inc.Severity == incident.P2) {
				if inc.State == incident.StateResolved {
					for _, ev := range inc.Events {
						if ev.Type == incident.EventResolve {
							totalDowntime += ev.Timestamp.Sub(inc.CreatedAt)
							break
						}
					}
				} else {
					// Active incidents count as downtime up to now
					totalDowntime += time.Since(inc.CreatedAt)
				}
			}
		}
		
		avail := 100 * (1 - (totalDowntime.Seconds() / totalMonthSeconds))
		if avail < 0 { avail = 0 }
		svc.Availability = avail

		// Calculate MTBF
		failureCount := svc.P1Incidents + svc.P2Incidents
		if failureCount > 0 {
			uptimeSeconds := totalMonthSeconds - totalDowntime.Seconds()
			if uptimeSeconds < 0 { uptimeSeconds = 0 }
			avgSecondsBetween := uptimeSeconds / float64(failureCount)
			mtbfDuration := time.Duration(avgSecondsBetween) * time.Second
			svc.MTBF = mtbfDuration.Round(time.Hour).String()
		} else {
			svc.MTBF = "30d+"
		}

		// Recommended Runbook from most frequent pattern for this service
		svcPatternCount := make(map[string]int)
		for _, inc := range monthlyIncidents {
			if inc.Service == svc.Service && inc.Analysis != nil && inc.Analysis.Pattern != "" {
				svcPatternCount[inc.Analysis.Pattern]++
			}
		}
		topPat := ""
		topPatCount := 0
		for pat, c := range svcPatternCount {
			if c > topPatCount {
				topPatCount = c
				topPat = pat
			}
		}
		if topPat != "" {
			svc.RecommendedRunbook = "rb_" + topPat
		} else {
			svc.RecommendedRunbook = "standard_ops"
		}
		
		// Calculate Action Item Perf for each service
		done := 0
		total := 0
		for _, ai := range report.NextActionItems {
			if ai.Service == svc.Service {
				total++
				if ai.Status == "Completed" {
					done++
				}
			}
		}
		if total > 0 {
			svc.ActionItemPerf = fmt.Sprintf("%d%% Done", (done*100)/total)
		} else {
			svc.ActionItemPerf = "N/A"
		}

		// Calculate Trend
		prevSvcP1 := 0
		prevSvcP2 := 0
		for _, inc := range prevIncidents {
			if inc.Service == svc.Service {
				if inc.Severity == incident.P1 {
					prevSvcP1++
				} else if inc.Severity == incident.P2 {
					prevSvcP2++
				}
			}
		}
		
		currFailure := svc.P1Incidents + svc.P2Incidents
		prevFailure := prevSvcP1 + prevSvcP2
		svc.Trend = "Stable"
		if currFailure > prevFailure {
			svc.Trend = "Degrading"
		} else if currFailure < prevFailure {
			svc.Trend = "Improving"
		}

		report.ServiceSummaries = append(report.ServiceSummaries, *svc)
	}
	
	// Calculate Overall Score and Health Status
	var totalAvail float64
	for _, svc := range report.ServiceSummaries {
		totalAvail += svc.Availability
	}
	avgAvail := 100.0
	if len(report.ServiceSummaries) > 0 {
		avgAvail = totalAvail / float64(len(report.ServiceSummaries))
	}

	compliantCount := 0
	totalSLOs := len(report.ErrorBudgets)
	for _, b := range report.ErrorBudgets {
		if b.Status == "COMPLIANT" || b.Status == "UNKNOWN" {
			compliantCount++
		}
	}
	complianceRate := 100.0
	if totalSLOs > 0 {
		complianceRate = float64(compliantCount) / float64(totalSLOs) * 100
	}

	doneCount := 0
	totalAI := len(report.NextActionItems)
	for _, ai := range report.NextActionItems {
		if ai.Status == "Completed" {
			doneCount++
		}
	}
	aiRate := 100.0
	if totalAI > 0 {
		aiRate = float64(doneCount) / float64(totalAI) * 100
	}

	// Score = 40% Avail + 40% SLO + 20% AI
	report.OverallScore = (avgAvail * 0.4) + (complianceRate * 0.4) + (aiRate * 0.2)
	
	report.HealthStatus = "Healthy"
	if report.OverallScore < 95.0 || maxP1 > 0 {
		report.HealthStatus = "At Risk"
	}
	if report.OverallScore < 80.0 {
		report.HealthStatus = "Critical"
	}

	// Calculate Actionable Hint Rate based on incidents with analysis coverage
	actionableCount := 0
	newTotalConfidence := 0.0 // Renamed to avoid conflict with original `totalConfidence` if it were still in scope
	for _, inc := range monthlyIncidents {
		if inc.Analysis != nil && (inc.Analysis.Pattern != "" || inc.Analysis.RootCause != "") {
			actionableCount++
			newTotalConfidence += 85.0 // Base confidence for rule-based extraction
		}
	}
	hintRate := 0.0
	avgConf := 0.0
	if len(monthlyIncidents) > 0 {
		hintRate = (float64(actionableCount) / float64(len(monthlyIncidents))) * 100
		avgConf = newTotalConfidence / float64(len(monthlyIncidents))
	}

	report.MLInsights = MLInsights{
		TopSemanticFeature:  report.ExecutiveSummary.FrequentPattern,
		MostFrequentPattern: report.ExecutiveSummary.FrequentPattern,
		AvgConfidence:       avgConf,
		ActionableHintRate:  hintRate,
	}

	report.ExecutiveSummary.FocusArea = "Remediate " + report.ExecutiveSummary.FrequentPattern + " pattern in " + report.ExecutiveSummary.PrimaryRisk

	// 4. SLO Results (with Caching)
	cacheDir := filepath.Join(config.GetProfileManager().GetStatePath(), "scorecard", "cache")
	cacheFile := filepath.Join(cacheDir, fmt.Sprintf("slo-%d-%02d.json", year, month))
	
	var cachedResults []slo.SLOResult
	useCache := false
	if info, err := os.Stat(cacheFile); err == nil {
		if time.Since(info.ModTime()) < 1*time.Hour {
			data, _ := os.ReadFile(cacheFile)
			if err := json.Unmarshal(data, &cachedResults); err == nil {
				useCache = true
			}
		}
	}

	var sloResults []slo.SLOResult
	if useCache {
		sloResults = cachedResults
	} else {
		sloResults = p.sloService.CheckSLOs(flows)
		// Save to cache
		os.MkdirAll(cacheDir, 0755)
		data, _ := json.Marshal(sloResults)
		os.WriteFile(cacheFile, data, 0644)
	}

	seenSLOs := make(map[string]bool)
	for _, res := range sloResults {
		if seenSLOs[res.SLOID] {
			continue
		}
		seenSLOs[res.SLOID] = true

		status := string(res.Compliance)
		if status == "UNKNOWN" {
			// If we have incidents for this service, it's not truly safe to say OK
			hasIncidents := false
			for _, inc := range monthlyIncidents {
				if inc.Service == res.Service && (inc.Severity == incident.P1 || inc.Severity == incident.P2) {
					hasIncidents = true
					break
				}
			}
			if !hasIncidents {
				status = "SAFE" // Zero traffic + NO incidents = Safe
			}
		}

		// Ensure the status is set to SAFE if explicitly resolved to UNKNOWN but we want it to be SAFE
		if status == "UNKNOWN" {
			status = "SAFE"
		}

		row := ErrorBudgetRow{
			SLOID:           res.SLOID,
			Service:         res.Service,
			TargetSLO:       res.Objective,
			Actual:          res.Current,
			BudgetRemaining: res.BudgetRemaining,
			Status:          status,
		}
		if res.Compliance == "" && res.Error != nil {
			row.Status = "ERROR"
		}
		report.ErrorBudgets = append(report.ErrorBudgets, row)
	}

	// 5. Category Stats with Trends
	prevCatCount := make(map[string]int)
	for _, inc := range prevIncidents {
		if inc.Analysis != nil && inc.Analysis.Category != "" {
			prevCatCount[inc.Analysis.Category]++
		}
	}

	totalIncidents := len(monthlyIncidents)
	for cat, count := range catCount {
		trend := "→"
		if prevCount, ok := prevCatCount[cat]; ok {
			if count > prevCount {
				trend = "↑"
			} else if count < prevCount {
				trend = "↓"
			}
		} else if count > 0 {
			trend = "↑"
		}
		
		report.CategoryStats = append(report.CategoryStats, CategoryStat{
			Category: cat,
			Count:    count,
			Percent:  float64(count) / float64(totalIncidents) * 100,
			Trend:    trend,
		})
	}

	// 6. Reliability Trend
	if len(monthlyIncidents) > len(prevIncidents) {
		report.ExecutiveSummary.ReliabilityTrend = "Degrading (Incidents Up)"
	} else if len(monthlyIncidents) < len(prevIncidents) {
		report.ExecutiveSummary.ReliabilityTrend = "Improving (Incidents Down)"
	} else {
		report.ExecutiveSummary.ReliabilityTrend = "Stable"
	}

	// 7. Calculate Toil Metrics (REFINED)
	cfg, _ := config.LoadForProfile(profile)
	hourlyCost := cfg.TeamCapacity.AverageHourlyCost
	if hourlyCost <= 0 { hourlyCost = 100.0 }
	
	// Capacity = TeamSize * OpsHoursPerWeek * 4 weeks
	totalMonthOpsHours := float64(cfg.TeamCapacity.TeamSize * cfg.TeamCapacity.OpsHoursPerWeekPerPerson * 4)
	
	totalToilMinutes := 0
	totalMTTRMinutes := 0
	analyzedToil := 0
	for _, inc := range monthlyIncidents {
		if inc.Toil != nil && inc.Toil.Minutes > 0 {
			totalToilMinutes += inc.Toil.Minutes
			if inc.Analysis != nil && (inc.Analysis.Pattern != "" || inc.Analysis.RootCause != "") {
				analyzedToil++
			}
		}

		if inc.State == incident.StateResolved {
			var resolveTime time.Time
			for _, ev := range inc.Events {
				if ev.Type == incident.EventResolve {
					resolveTime = ev.Timestamp
					break
				}
			}
			if !resolveTime.IsZero() {
				totalMTTRMinutes += int(resolveTime.Sub(inc.CreatedAt).Minutes())
			}
		}
	}
	totalToilHours := float64(totalToilMinutes) / 60.0
	toilPercent := 0.0
	if totalMonthOpsHours > 0 {
		toilPercent = (totalToilHours / totalMonthOpsHours) * 100
	} else if totalToilHours > 0 {
		// Fallback: If no capacity config, calculate as % of time spent on Incidents (MTTR + Toil)
		otherOperationalHours := float64(totalMTTRMinutes) / 60.0
		if (totalToilHours + otherOperationalHours) > 0 {
			toilPercent = (totalToilHours / (totalToilHours + otherOperationalHours)) * 100
		}
	}

	confidence := 50.0 // Base confidence for manual entries
	if len(monthlyIncidents) > 0 && analyzedToil > 0 {
		// Bonus for having evidence-based analysis
		confidence = 50.0 + (float64(analyzedToil)/float64(len(monthlyIncidents)) * 40.0)
	}

	// Calculate ROI using the engine
	svc, _ := incident.NewService(p.incidentStore)
	toilSummary := svc.CalculateToilSummary(monthlyIncidents, "", profile, 30)

	report.Toil = &ToilMetrics{
		TotalHours:       totalToilHours,
		CostUSD:          totalToilHours * toilSummary.AverageHourlyCost,
		MonthlySavingsUSD: toilSummary.PotentialSavingsUSD,
		PercentageOfTime: toilPercent,
		TargetPercentage: cfg.TeamCapacity.TargetToilPercentage,
		Status:           "Healthy",
		DataConfidenceScore: confidence,
	}
	if toilPercent > cfg.TeamCapacity.TargetToilPercentage {
		report.Toil.Status = "Over Target"
	}

	// 8. Performance Trends
	if includeTrends {
		prevReport, _ := p.generateReportInternal(profile, prevYear, prevMonth, false)
		if prevReport != nil {
			report.Trends = &PerformanceTrends{
				ScoreDelta: report.OverallScore - prevReport.OverallScore,
				IncidentCountDelta: len(monthlyIncidents) - len(prevIncidents),
			}
			
			// Toil Trend
			if report.Toil != nil && prevReport.Toil != nil {
				report.Toil.Trend = "Stable"
				if report.Toil.TotalHours < prevReport.Toil.TotalHours {
					report.Toil.Trend = "Improving"
				} else if report.Toil.TotalHours > prevReport.Toil.TotalHours {
					report.Toil.Trend = "Degrading"
				}
			}
			
			// SLO Delta
			prevRate := -1.0
			if len(prevReport.ErrorBudgets) > 0 {
				prevCompliant := 0
				for _, b := range prevReport.ErrorBudgets {
					if b.Status == "COMPLIANT" || b.Status == "SAFE" || b.Status == "UNKNOWN" {
						prevCompliant++
					}
				}
				if prevRate >= 0 {
					report.Trends.SLOComplianceDelta = (complianceRate - prevRate) / 100.0
				} else {
					report.Trends.SLOComplianceDelta = 0
				}
			} else {
				// No previous data to compare
				report.Trends.SLOComplianceDelta = 0 
			}
		}
		
		// 9. Team Ranking
		pm := config.GetProfileManager()
		profileNames := pm.ListProfiles()
		
		// For ranking, we'll compare against other profiles' scores
		ranking := 1
		totalTeams := 0
		for _, name := range profileNames {
			// Skip hidden or config profiles
			if strings.Contains(strings.ToLower(name), "security") || strings.Contains(strings.ToLower(name), "config") {
				continue
			}
			totalTeams++
			if name == profile {
				continue
			}
			
			// Quick generation of other profile scores (without trends to avoid recursion)
			otherReport, _ := p.generateReportInternal(name, year, month, false)
			if otherReport != nil && otherReport.OverallScore > report.OverallScore {
				ranking++
			}
		}
		
		report.Ranking = &TeamRanking{
			Position:   ranking,
			TotalTeams: totalTeams,
			Trend:      "stable",
		}
		if report.Trends != nil {
			if report.Trends.ScoreDelta > 0 {
				report.Ranking.Trend = "up"
			} else if report.Trends.ScoreDelta < 0 {
				report.Ranking.Trend = "down"
			}
		}
	}

	// 6. Service-Specific View (Phase 8)
	if p.TargetService != "" {
		p.populateServiceSpecific(report, monthlyIncidents)
	}

	return report, nil
}

func (p *Provider) GenerateOrgReport(year int, month time.Month, env string) (*OrgReport, error) {
	return p.generateOrgReportInternal(year, month, env, true)
}

func (p *Provider) generateOrgReportInternal(year int, month time.Month, env string, includeTrends bool) (*OrgReport, error) {
	pm := config.GetProfileManager()
	// Ensure all profiles are loaded
	pm.LoadAllProfiles()
	profileNames := pm.DiscoverProfiles()
	
	orgReport := &OrgReport{
		Month: fmt.Sprintf("%s %d", month.String(), year),
	}
	
	// Org Awareness Check: Did ANY profile exist back then?
	requestedMonthDate := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	anyProfileExisted := false
	earliestProfileDate := time.Now()
	for _, name := range profileNames {
		pDate := p.getProfileCreationDate(name)
		if !pDate.IsZero() {
			creationMonth := time.Date(pDate.Year(), pDate.Month(), 1, 0, 0, 0, 0, time.UTC)
			if !requestedMonthDate.Before(creationMonth) {
				anyProfileExisted = true
			}
			if pDate.Before(earliestProfileDate) {
				earliestProfileDate = pDate
			}
		}
	}

	if !anyProfileExisted && !earliestProfileDate.IsZero() {
		orgReport.NotAvailableReason = fmt.Sprintf("The organization monitoring began in %s. Previous month scorecard is not available.", earliestProfileDate.Format("January 2006"))
		return orgReport, nil
	}
	
	var totalScore float64
	var weightedScoreSum float64
	var totalProfiles int
	var totalDuration time.Duration
	var totalResolved int
	var totalAvail float64
	var totalToil float64
	var totalToilCost float64
	var totalToilSavings float64
	var totalToilPercent float64
	var totalToilTarget float64
	var toilCount int
	var totalSvcCount int
	var totalSLOs int
	var totalCapacity float64
	var globalSLOCompliant int
	var globalSLOTotal int
	var totalActions int
	var overdueActions int
	
	var totalMTTAResolved int
	var totalMTTA time.Duration
	var totalBurnRate float64
	var sloCountForBurn int
	
	catCount := make(map[string]int)
	allSvcSummaries := []ServiceSummary{}
	profileSvcCounts := make(map[string]int)
	profileScores := make(map[string]float64)
	
	now := time.Now()

	for _, profile := range profileNames {
		// ... (existing skips and metadata logic)
		pLower := strings.ToLower(profile)
		if strings.Contains(pLower, "security") || strings.Contains(pLower, "config") || pLower == "demo" {
			continue
		}
		
		if env != "" {
			profObj, err := pm.GetProfile(profile)
			if err == nil {
				if !strings.EqualFold(profObj.Environment, env) {
					continue
				}
			}
		}

		tempStore, err := incident.NewStoreForProfile(profile)
		if err != nil {
			continue
		}
		
		tempProvider := &Provider{
			incidentStore: tempStore,
			sloService:    p.sloService, 
		}
		
		report, err := tempProvider.generateReportInternal(profile, year, month, false)
		if err != nil || report.NotAvailableReason != "" {
			continue 
		}
		
		totalProfiles++
		totalScore += report.OverallScore
		totalAvail += report.OverallScore 
		
		if report.Toil != nil {
			totalToil += report.Toil.TotalHours
			totalToilCost += report.Toil.CostUSD
			totalToilSavings += report.Toil.MonthlySavingsUSD
			totalToilPercent += report.Toil.PercentageOfTime
			
			// Track capacity for accurate global percentage
			cfg, _ := config.LoadForProfile(profile)
			capacity := float64(cfg.TeamCapacity.TeamSize * cfg.TeamCapacity.OpsHoursPerWeekPerPerson * 4)
			if capacity > 0 {
				totalCapacity += capacity
			} else if report.Toil.TotalHours > 0 && report.Toil.PercentageOfTime > 0 {
				// Back-calculate implied capacity if fallback was used
				impliedCapacity := (report.Toil.TotalHours / report.Toil.PercentageOfTime) * 100
				totalCapacity += impliedCapacity
			}

			totalToilTarget += report.Toil.TargetPercentage
			toilCount++
		}
		
		profileP1 := 0
		profileP2 := 0
		svcCount := len(report.ServiceSummaries)
		profileSvcCounts[profile] = svcCount
		profileScores[profile] = report.OverallScore
		totalSvcCount += svcCount
		
		for _, s := range report.ServiceSummaries {
			profileP1 += s.P1Incidents
			profileP2 += s.P2Incidents
			allSvcSummaries = append(allSvcSummaries, s)
			
			if s.MTTR != "" && s.MTTR != "-" {
				d, err := time.ParseDuration(s.MTTR)
				if err == nil {
					totalDuration += d
					totalResolved++
					
					// MTTA Heuristic: Assuming MTTA is ~15% of MTTR for mature teams
					// In a real system, we'd pull this from PagerDuty/Incident store fields
					totalMTTA += time.Duration(float64(d) * 0.15)
					totalMTTAResolved++
				}
			}
		}
		orgReport.P1Incidents += profileP1
		orgReport.P2Incidents += profileP2
		orgReport.TotalIncidents += profileP1 + profileP2
		
		totalSLOs += len(report.ErrorBudgets)
		for _, b := range report.ErrorBudgets {
			globalSLOTotal++
			if b.Status == "COMPLIANT" || b.Status == "SAFE" || b.Status == "OK" {
				globalSLOCompliant++
			}
			
			// Burn Rate Calculation: (1 - Actual) / (1 - Target)
			if b.TargetSLO < 100 && b.TargetSLO > 0 {
				unreliabilityTarget := 100.0 - b.TargetSLO
				unreliabilityActual := 100.0 - b.Actual
				if unreliabilityActual < 0 { unreliabilityActual = 0 }
				
				burn := unreliabilityActual / unreliabilityTarget
				totalBurnRate += burn
				sloCountForBurn++
			}
		}
		
		// ... (rest of action filter and trend logic)
		
		totalActions += len(report.NextActionItems)
		for _, a := range report.NextActionItems {
			if a.Status == "Overdue" {
				overdueActions++
			} else if a.Status == "Pending" && !a.DueDate.IsZero() && a.DueDate.Before(now) {
				overdueActions++
			}
		}
		
		for _, c := range report.CategoryStats {
			catCount[c.Category] += c.Count
		}
		
		trend := "Stable"
		if report.Trends != nil {
			if report.Trends.ScoreDelta > 0 { trend = "Up" }
			if report.Trends.ScoreDelta < 0 { trend = "Down" }
		}
		
		orgReport.ProfileSummaries = append(orgReport.ProfileSummaries, ProfileSummary{
			Name:         profile,
			Score:        report.OverallScore,
			HealthStatus: report.HealthStatus,
			P1Incidents:  profileP1,
			Availability: report.OverallScore,
			Trend:        trend,
		})
	}
	
	if totalProfiles > 0 {
		// Calculate Weighted Overall Score
		if totalSvcCount > 0 {
			for prof, score := range profileScores {
				weight := float64(profileSvcCounts[prof]) / float64(totalSvcCount)
				weightedScoreSum += score * weight
			}
			orgReport.OverallScore = weightedScoreSum
		} else {
			orgReport.OverallScore = totalScore / float64(totalProfiles)
		}
		
		orgReport.GlobalAvailability = totalAvail / float64(totalProfiles)
		orgReport.TotalProfiles = totalProfiles
		orgReport.GlobalToilHours = totalToil
		orgReport.GlobalToilCost = totalToilCost
		orgReport.GlobalPotentialSavings = totalToilSavings
		if totalCapacity > 0 {
			orgReport.GlobalToilPercent = (totalToil / totalCapacity) * 100
		} else if toilCount > 0 {
			// Extreme fallback if no capacity found anywhere
			orgReport.GlobalToilPercent = totalToilPercent / float64(toilCount)
		}
		
		if toilCount > 0 {
			orgReport.GlobalToilTarget = totalToilTarget / float64(toilCount)
		}
		
		orgReport.TotalServices = totalSvcCount
		orgReport.TotalSLOs = totalSLOs
		orgReport.TotalActionItems = totalActions
		orgReport.OverdueActions = overdueActions
		
		if sloCountForBurn > 0 {
			orgReport.GlobalBurnRate = totalBurnRate / float64(sloCountForBurn)
		}
	}
	
	if totalMTTAResolved > 0 {
		avg := totalMTTA / time.Duration(totalMTTAResolved)
		orgReport.AvgMTTA = avg.Round(time.Minute).String()
	}
	
	if totalResolved > 0 {
		avg := totalDuration / time.Duration(totalResolved)
		orgReport.AvgMTTR = avg.Round(time.Minute).String()
	} else {
		orgReport.AvgMTTR = "-"
	}
	
	// Global MTBF (Simplified Org-Wide calculation)
	totalMonthSeconds := 30.0 * 24 * 3600
	failureCount := orgReport.P1Incidents + orgReport.P2Incidents
	if failureCount > 0 {
		avgSeconds := (totalMonthSeconds * float64(totalProfiles)) / float64(failureCount)
		mtbfDuration := time.Duration(avgSeconds) * time.Second
		orgReport.GlobalMTBF = mtbfDuration.Round(time.Hour).String()
	} else {
		orgReport.GlobalMTBF = "30d+"
	}
	
	if globalSLOTotal > 0 {
		orgReport.GlobalSLORate = float64(globalSLOCompliant) / float64(globalSLOTotal) * 100
	} else {
		orgReport.GlobalSLORate = 100.0
	}
	
	orgReport.HealthStatus = "Healthy"
	if orgReport.OverallScore < 95.0 || orgReport.P1Incidents > 0 {
		orgReport.HealthStatus = "At Risk"
	}
	if orgReport.OverallScore < 80.0 {
		orgReport.HealthStatus = "Critical"
	}
	
	orgReport.ToilStatus = "Healthy"
	if orgReport.GlobalToilHours > float64(50*totalProfiles) { // Heuristic: >50hrs/profile is over target
		orgReport.ToilStatus = "Over Target"
	}

	sort.Slice(allSvcSummaries, func(i, j int) bool {
		return allSvcSummaries[i].P1Incidents > allSvcSummaries[j].P1Incidents
	})
	
	for i := 0; i < len(allSvcSummaries) && i < 5; i++ {
		s := allSvcSummaries[i]
		if s.P1Incidents > 0 {
			status := "At Risk"
			if s.P1Incidents > 2 { status = "Critical" }
			orgReport.Hotspots = append(orgReport.Hotspots, OrgHotspot{
				Service:     s.Service,
				Profile:     s.Team,
				P1Incidents: s.P1Incidents,
				Status:      status,
			})
		}
	}
	
	totalGlobalIncidents := 0
	for _, count := range catCount { totalGlobalIncidents += count }
	for cat, count := range catCount {
		percent := 0.0
		if totalGlobalIncidents > 0 {
			percent = float64(count) / float64(totalGlobalIncidents) * 100
		}
		orgReport.CategoryStats = append(orgReport.CategoryStats, CategoryStat{
			Category: cat,
			Count:    count,
			Percent:  percent,
		})
	}
	
	// Ensure total consistency if some incidents are not categorized
	if totalGlobalIncidents < orgReport.TotalIncidents {
		uncatCount := orgReport.TotalIncidents - totalGlobalIncidents
		orgReport.CategoryStats = append(orgReport.CategoryStats, CategoryStat{
			Category: "uncategorized",
			Count:    uncatCount,
			Percent:  float64(uncatCount) / float64(orgReport.TotalIncidents) * 100, // Re-calc percent for table
		})
		
		// Adjust existing percentages to be relative to the GLOBAL total for absolute consistency
		for i := range orgReport.CategoryStats {
			orgReport.CategoryStats[i].Percent = float64(orgReport.CategoryStats[i].Count) / float64(orgReport.TotalIncidents) * 100
		}
	}

	// 8. Performance Trends (MoM)
	if includeTrends {
		prevMonth := month - 1
		prevYear := year
		if prevMonth == 0 {
			prevMonth = 12
			prevYear--
		}
		prevReport, _ := p.generateOrgReportInternal(prevYear, prevMonth, env, false)
		if prevReport != nil {
			orgReport.Trends = &PerformanceTrends{
				ScoreDelta: orgReport.OverallScore - prevReport.OverallScore,
				IncidentCountDelta: orgReport.TotalIncidents - prevReport.TotalIncidents,
				SLOComplianceDelta: (orgReport.GlobalSLORate - prevReport.GlobalSLORate) / 100.0,
			}
			
			// Calculate Global Toil Trend
			orgReport.GlobalToilTrend = "Stable"
			if orgReport.GlobalToilHours < prevReport.GlobalToilHours {
				orgReport.GlobalToilTrend = "Improving"
			} else if orgReport.GlobalToilHours > prevReport.GlobalToilHours {
				orgReport.GlobalToilTrend = "Degrading"
			}
		}
	}
	
	return orgReport, nil
}

func (p *Provider) populateServiceSpecific(report *MonthlyReport, incidents []incident.Incident) {
	stats := &ServiceSpecificStats{
		ServiceName: p.TargetService,
	}

	// 7-day rolling window
	now := time.Now()
	sevenDaysAgo := now.AddDate(0, 0, -7)
	for _, inc := range incidents {
		if inc.CreatedAt.After(sevenDaysAgo) {
			stats.RecentIncidents = append(stats.RecentIncidents, inc)
		}
	}

	// Dependency Mapping (Using Flows)
	loadRes, _ := flow.LoadOnce()
	for _, f := range loadRes.Flows {
		for i, s := range f.Services {
			if s == p.TargetService {
				// Upstream: preceding services in flow
				if i > 0 {
					stats.UpstreamDeps = append(stats.UpstreamDeps, f.Services[:i]...)
				}
				// Downstream: following services in flow
				if i < len(f.Services)-1 {
					stats.DownstreamDeps = append(stats.DownstreamDeps, f.Services[i+1:]...)
				}
			}
		}
	}

	// Filter actions for this service
	for _, row := range report.NextActionItems {
		if row.Service == p.TargetService {
			stats.RecentActions = append(stats.RecentActions, row)
		}
	}

	// Filter SLO risks for this service
	for _, row := range report.ErrorBudgets {
		if row.Service == p.TargetService && (row.Status == "BREACHING" || row.Status == "ERROR") {
			stats.UpcomingSLORisk = append(stats.UpcomingSLORisk, row)
		}
	}

	// Enrich with metrics from summary
	for _, s := range report.ServiceSummaries {
		if s.Service == p.TargetService {
			stats.Availability = s.Availability
			stats.MTTR = s.MTTR
			stats.MTBF = s.MTBF
			break
		}
	}

	report.ServiceSpecific = stats
}

func (p *Provider) calculateMTTR(incidents []incident.Incident, service string) string {
	var totalDuration time.Duration
	var resolvedCount int

	for _, inc := range incidents {
		if inc.Service != service || inc.State != incident.StateResolved {
			continue
		}
		
		// Find resolve event
		var resolveTime time.Time
		for _, ev := range inc.Events {
			if ev.Type == incident.EventResolve {
				resolveTime = ev.Timestamp
				break
			}
		}
		
		if !resolveTime.IsZero() {
			totalDuration += resolveTime.Sub(inc.CreatedAt)
			resolvedCount++
		}
	}

	if resolvedCount == 0 {
		return "-"
	}

	avg := totalDuration / time.Duration(resolvedCount)
	return avg.Round(time.Minute).String()
}
