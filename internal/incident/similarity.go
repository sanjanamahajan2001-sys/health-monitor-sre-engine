package incident

import (
	"fmt"
	"strings"
	"time"
)

// SimilarIncident represents an incident similar to the current one
type SimilarIncident struct {
	Incident   Incident
	Confidence float64  // 0.0 to 1.0
	MatchedOn  []string // Fields that matched: "component", "category", "pattern", etc.
	Age        string   // Human-readable age: "5 days ago"
}

// SimilarityOptions configures the similarity search
type SimilarityOptions struct {
	MaxResults    int     // Default: 5
	MinConfidence float64 // Default: 0.6 (60%)
	DaysBack      int     // Default: 90
	IncludeStates []State // Default: [StateResolved]
}

// DefaultSimilarityOptions returns production-safe defaults
func DefaultSimilarityOptions() SimilarityOptions {
	return SimilarityOptions{
		MaxResults:    5,
		MinConfidence: 0.6,
		DaysBack:      90,
		IncludeStates: []State{StateResolved},
	}
}

// CalculateSimilarity computes confidence score between current and historical incident
// Returns score from 0.0 to 1.0, and list of matched fields
func CalculateSimilarity(current, historical *Incident) (float64, []string) {
	if current == nil || historical == nil {
		return 0.0, nil
	}

	var score float64
	var matched []string

	// If either incident lacks analysis, use fallback matching
	if current.Analysis == nil || historical.Analysis == nil {
		return fallbackSimilarity(current, historical)
	}

	// Component match (30% weight)
	if componentMatch := compareComponents(current.Analysis.Component, historical.Analysis.Component); componentMatch > 0 {
		score += componentMatch * 0.30
		if componentMatch > 0.8 {
			matched = append(matched, "component")
		}
	}

	// Category match (25% weight)
	if current.Analysis.Category != "" && historical.Analysis.Category != "" {
		if strings.EqualFold(current.Analysis.Category, historical.Analysis.Category) {
			score += 0.25
			matched = append(matched, "category")
		}
	}

	// Pattern match (20% weight)
	if patternSim := patternSimilarity(current.Analysis.Pattern, historical.Analysis.Pattern); patternSim > 0 {
		score += patternSim * 0.20
		if patternSim > 0.6 {
			matched = append(matched, "pattern")
		}
	}

	// Dependency match (15% weight)
	if dependencyMatch := compareDependencies(current.Analysis.Dependency, historical.Analysis.Dependency); dependencyMatch > 0 {
		score += dependencyMatch * 0.15
		if dependencyMatch > 0.8 {
			matched = append(matched, "dependency")
		}
	}

	// Error signature match (10% weight)
	if errorSigMatch := compareErrorSignatures(current.Analysis.ErrorSignature, historical.Analysis.ErrorSignature); errorSigMatch > 0 {
		score += errorSigMatch * 0.10
		if errorSigMatch > 0.8 {
			matched = append(matched, "error_signature")
		}
	}

	// Apply time decay factor
	score = applyTimeDecay(score, historical.CreatedAt)

	return score, matched
}

// compareComponents uses fuzzy matching for component names
func compareComponents(comp1, comp2 string) float64 {
	if comp1 == "" || comp2 == "" {
		return 0.0
	}

	c1 := strings.ToLower(strings.TrimSpace(comp1))
	c2 := strings.ToLower(strings.TrimSpace(comp2))

	// Exact match
	if c1 == c2 {
		return 1.0
	}

	// Check service aliases (handle renamed services)
	if areAliases(c1, c2) {
		return 0.95 // High confidence for known aliases
	}

	// Substring match (e.g., "postgres" in "postgres_db")
	if strings.Contains(c1, c2) || strings.Contains(c2, c1) {
		return 0.9
	}

	// Fuzzy match using Levenshtein distance
	distance := levenshteinDistance(c1, c2)
	maxLen := float64(max(len(c1), len(c2)))
	if maxLen == 0 {
		return 0.0
	}

	similarity := 1.0 - (float64(distance) / maxLen)
	if similarity > 0.7 {
		return similarity
	}

	return 0.0
}

// serviceAliases maps service names to their known aliases
var serviceAliases = map[string][]string{
	"billing_api":    {"billing-service", "billing", "billing-api"},
	"postgres_db":    {"postgres", "postgresql", "postgres-db"},
	"redis_cache":    {"redis", "redis-cache"},
	"auth_service":   {"auth", "auth-service", "authentication"},
	"payment_api":    {"payment", "payment-service", "payment-api"},
	"notification":   {"notifier", "notification-service"},
}

// areAliases checks if two component names are known aliases of each other
func areAliases(comp1, comp2 string) bool {
	// Check if comp1 is a key and comp2 is in its aliases
	if aliases, ok := serviceAliases[comp1]; ok {
		for _, alias := range aliases {
			if alias == comp2 {
				return true
			}
		}
	}
	
	// Check if comp2 is a key and comp1 is in its aliases
	if aliases, ok := serviceAliases[comp2]; ok {
		for _, alias := range aliases {
			if alias == comp1 {
				return true
			}
		}
	}
	
	// Check if both are aliases of the same service
	for _, aliases := range serviceAliases {
		foundComp1 := false
		foundComp2 := false
		for _, alias := range aliases {
			if alias == comp1 {
				foundComp1 = true
			}
			if alias == comp2 {
				foundComp2 = true
			}
		}
		if foundComp1 && foundComp2 {
			return true
		}
	}
	
	return false
}

// compareDependencies checks if dependency relationships match
func compareDependencies(dep1, dep2 string) float64 {
	if dep1 == "" || dep2 == "" {
		return 0.0
	}

	d1 := strings.ToLower(strings.TrimSpace(dep1))
	d2 := strings.ToLower(strings.TrimSpace(dep2))

	// Exact match
	if d1 == d2 {
		return 1.0
	}

	// Parse dependency (format: "service->component")
	parts1 := strings.Split(d1, "->")
	parts2 := strings.Split(d2, "->")

	if len(parts1) == 2 && len(parts2) == 2 {
		// Check if target component matches (more important than source)
		targetMatch := compareComponents(parts1[1], parts2[1])
		sourceMatch := compareComponents(parts1[0], parts2[0])
		return (targetMatch*0.7 + sourceMatch*0.3)
	}

	return 0.0
}

// compareErrorSignatures compares normalized error signatures
func compareErrorSignatures(sig1, sig2 string) float64 {
	if sig1 == "" || sig2 == "" {
		return 0.0
	}

	s1 := strings.ToLower(strings.TrimSpace(sig1))
	s2 := strings.ToLower(strings.TrimSpace(sig2))

	if s1 == s2 {
		return 1.0
	}

	// Check if one contains the other
	if strings.Contains(s1, s2) || strings.Contains(s2, s1) {
		return 0.8
	}

	return 0.0
}

// patternSimilarity uses Jaccard index for pattern matching
func patternSimilarity(pattern1, pattern2 string) float64 {
	if pattern1 == "" || pattern2 == "" {
		return 0.0
	}

	// Extract terms from patterns
	terms1 := extractTerms(pattern1)
	terms2 := extractTerms(pattern2)

	if len(terms1) == 0 || len(terms2) == 0 {
		return 0.0
	}

	// Calculate Jaccard index
	intersection := 0
	for term := range terms1 {
		if terms2[term] {
			intersection++
		}
	}

	union := len(terms1) + len(terms2) - intersection
	if union == 0 {
		return 0.0
	}

	return float64(intersection) / float64(union)
}

// extractTerms extracts meaningful terms from a pattern string
func extractTerms(pattern string) map[string]bool {
	terms := make(map[string]bool)
	words := strings.Fields(strings.ToLower(pattern))

	// Filter out common words
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true,
		"but": true, "in": true, "on": true, "at": true, "to": true,
		"for": true, "of": true, "with": true, "by": true,
	}

	for _, word := range words {
		word = strings.Trim(word, ".,;:!?\"'")
		if len(word) > 2 && !stopWords[word] {
			terms[word] = true
		}
	}

	return terms
}

// applyTimeDecay reduces score for older incidents
func applyTimeDecay(score float64, incidentTime time.Time) float64 {
	ageDays := time.Since(incidentTime).Hours() / 24

	// No decay for incidents < 30 days old
	if ageDays < 30 {
		return score
	}

	// Linear decay: 30% reduction over 180 days
	decayFactor := 1.0 - ((ageDays - 30) / 150) * 0.3
	if decayFactor < 0.7 {
		decayFactor = 0.7 // Minimum 70% of original score
	}

	return score * decayFactor
}

// fallbackSimilarity provides basic matching when RCA data is missing
func fallbackSimilarity(current, historical *Incident) (float64, []string) {
	var score float64
	var matched []string

	// Service name match (40% weight)
	if current.Service != "" && current.Service == historical.Service {
		score += 0.4
		matched = append(matched, "service")
	}

	// If one has analysis and the other doesn't, try to match on available fields
	if current.Analysis != nil && historical.Analysis == nil {
		// Current has analysis, historical doesn't - use service + title similarity
		if current.Service == historical.Service {
			score += 0.2 // Bonus for same service
		}
	} else if current.Analysis == nil && historical.Analysis != nil {
		// Historical has analysis, current doesn't - match on service + category if available
		if current.Service == historical.Service {
			score += 0.3 // Higher weight when historical has RCA data
			
			// Try to match on title/summary keywords
			if historical.Analysis.Category != "" {
				currentText := strings.ToLower(current.Title + " " + current.Summary)
				category := strings.ToLower(historical.Analysis.Category)
				if strings.Contains(currentText, category) {
					score += 0.2
					matched = append(matched, "category_keyword")
				}
			}
		}
	}

	// Log pattern match (30% weight) - if available
	if current.Logs != nil && historical.Logs != nil {
		if len(current.Logs.TopErrors) > 0 && len(historical.Logs.TopErrors) > 0 {
			currentPattern := current.Logs.TopErrors[0].Message
			historicalPattern := historical.Logs.TopErrors[0].Message
			if patternSim := patternSimilarity(currentPattern, historicalPattern); patternSim > 0.5 {
				score += patternSim * 0.3
				matched = append(matched, "log_pattern")
			}
		}
	}

	// Severity match (10% weight)
	if current.Severity == historical.Severity {
		score += 0.1
	}

	// Title similarity (20% weight)
	if titleSim := patternSimilarity(current.Title, historical.Title); titleSim > 0.3 {
		score += titleSim * 0.2
		if titleSim > 0.6 {
			matched = append(matched, "title")
		}
	}

	// Apply time decay
	score = applyTimeDecay(score, historical.CreatedAt)

	// Cap fallback similarity at 50% (lower confidence without RCA)
	// This prevents service-only matches from appearing too confident
	if score > 0.5 {
		score = 0.5
	}

	return score, matched
}

// levenshteinDistance computes edit distance between two strings
func levenshteinDistance(s1, s2 string) int {
	if len(s1) == 0 {
		return len(s2)
	}
	if len(s2) == 0 {
		return len(s1)
	}

	matrix := make([][]int, len(s1)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(s2)+1)
		matrix[i][0] = i
	}
	for j := range matrix[0] {
		matrix[0][j] = j
	}

	for i := 1; i <= len(s1); i++ {
		for j := 1; j <= len(s2); j++ {
			cost := 1
			if s1[i-1] == s2[j-1] {
				cost = 0
			}
			matrix[i][j] = min(
				matrix[i-1][j]+1,      // deletion
				matrix[i][j-1]+1,      // insertion
				matrix[i-1][j-1]+cost, // substitution
			)
		}
	}

	return matrix[len(s1)][len(s2)]
}

// Helper functions
func min(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// formatAge converts time to human-readable age
func formatAge(t time.Time) string {
	duration := time.Since(t)
	days := int(duration.Hours() / 24)

	if days == 0 {
		hours := int(duration.Hours())
		if hours == 0 {
			return "just now"
		}
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	}

	if days == 1 {
		return "1 day ago"
	}

	if days < 30 {
		return fmt.Sprintf("%d days ago", days)
	}

	months := days / 30
	if months == 1 {
		return "1 month ago"
	}

	return fmt.Sprintf("%d months ago", months)
}

// groupByFix groups similar incidents with identical fixes
func groupByFix(incidents []SimilarIncident) []SimilarIncident {
	if len(incidents) <= 1 {
		return incidents
	}

	// Group by fix summary
	groups := make(map[string][]SimilarIncident)
	for _, inc := range incidents {
		fix := ""
		if inc.Incident.Analysis != nil {
			fix = strings.ToLower(strings.TrimSpace(inc.Incident.Analysis.FixSummary))
		}
		if fix == "" {
			fix = "_no_fix_" // Group incidents without fix separately
		}
		groups[fix] = append(groups[fix], inc)
	}

	// If no grouping possible, return original
	if len(groups) == len(incidents) {
		return incidents
	}

	// Rebuild list with grouped incidents
	var result []SimilarIncident
	for _, group := range groups {
		if len(group) == 1 {
			result = append(result, group[0])
		} else {
			// Take the most recent one from the group and note count
			mostRecent := group[0]
			for _, inc := range group[1:] {
				if inc.Incident.CreatedAt.After(mostRecent.Incident.CreatedAt) {
					mostRecent = inc
				}
			}
			// Add count to matched fields
			mostRecent.MatchedOn = append(mostRecent.MatchedOn, fmt.Sprintf("(+%d similar)", len(group)-1))
			result = append(result, mostRecent)
		}
	}

	return result
}
// GetDemoSimilarIncidents returns hardcoded similar incidents for demo purposes.
// This is used to ensure consistent demo experience across CLI and TUI.
func GetDemoSimilarIncidents(inc Incident) []SimilarIncident {
	var similar []SimilarIncident

	// Generate similar incidents based on service and patterns
	switch inc.Service {
	case "orders_service":
		if inc.ID == "INC-20260324-052436" {
			similar = append(similar, SimilarIncident{
				Incident: Incident{
					ID:      "INC-20260212-091522",
					Title:   "Payment Gateway Latency Spike",
					Service: "orders_service",
					State:   StateResolved,
				},
				Confidence: 0.89,
				MatchedOn:  []string{"service", "pattern"},
				Age:        "6 weeks ago",
			})
			similar = append(similar, SimilarIncident{
				Incident: Incident{
					ID:      "INC-20260105-144031",
					Title:   "Egress NAT Gateway Failure",
					Service: "infrastructure",
					State:   StateResolved,
				},
				Confidence: 0.76,
				MatchedOn:  []string{"service", "component"},
				Age:        "3 months ago",
			})
		}
	case "user-profile":
		if inc.ID == "INC-20260324-052436" || strings.Contains(inc.Title, "Profile") {
			similar = append(similar, SimilarIncident{
				Incident: Incident{
					ID:      "INC-DEMO-DB-SPIKE",
					Title:   "Latency spike in Postgres DB queries",
					Service: "postgres_db",
					State:   StateStarted,
				},
				Confidence: 0.82,
				MatchedOn:  []string{"pattern", "database"},
				Age:        "2 hours ago",
			})
		}
	}

	return similar
}
