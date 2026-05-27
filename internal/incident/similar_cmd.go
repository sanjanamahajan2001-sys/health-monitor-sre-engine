package incident

import (
	"flag"
	"fmt"
	"health-monitor/internal/output"
	"os"
	"strings"
	"time"
)

// handleSimilar shows similar historical incidents
func handleSimilar(service *Service, args []string) int {
	fs := flag.NewFlagSet("incident similar", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	id := fs.String("id", "", "incident ID (optional, uses active if not specified)")
	days := fs.Int("days", 90, "number of days to search back")
	minConf := fs.Float64("min-confidence", 0.6, "minimum confidence threshold (0.0-1.0)")
	limit := fs.Int("limit", 5, "maximum number of results")
	dataDir := fs.String("data-dir", "", "data directory")
	
	// Override flags for manual matching
	component := fs.String("component", "", "override component for matching (e.g., postgres_db)")
	category := fs.String("category", "", "override category for matching (capacity, latency, dependency, etc.)")
	pattern := fs.String("pattern", "", "override error pattern for matching")
	dependency := fs.String("dependency", "", "override dependency for matching (e.g., api->db)")
	
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if strings.TrimSpace(*dataDir) != "" {
		_ = os.Setenv("HEALTH_MONITOR_DATA_DIR", strings.TrimSpace(*dataDir))
	}
	
	// Get the incident to compare
	var current *Incident
	if strings.TrimSpace(*id) != "" {
		inc, err := service.store.Load(strings.TrimSpace(*id))
		if err != nil {
			output.Errorf("Failed to load incident: %v", err)
			return 1
		}
		current = &inc
	} else {
		inc, _, err := service.getActive()
		if err != nil {
			output.Errorf("Failed to get active incident: %v", err)
			return 1
		}
		if inc == nil {
			output.Errorf("No active incident. Use --id to specify an incident.")
			return 1
		}
		current = inc
	}
	
	// Apply overrides if provided
	if *component != "" || *category != "" || *pattern != "" || *dependency != "" {
		// Create temporary incident with overrides for matching
		overridden := *current
		if overridden.Analysis == nil {
			overridden.Analysis = &IncidentAnalysis{}
		} else {
			// Copy analysis to avoid modifying original
			analysisCopy := *overridden.Analysis
			overridden.Analysis = &analysisCopy
		}
		
		if *component != "" {
			overridden.Analysis.Component = *component
		}
		if *category != "" {
			overridden.Analysis.Category = *category
		}
		if *pattern != "" {
			overridden.Analysis.Pattern = *pattern
		}
		if *dependency != "" {
			overridden.Analysis.Dependency = *dependency
		}
		
		current = &overridden
		fmt.Println("\n🔧 Using manual override criteria:")
		if *component != "" {
			fmt.Printf("   Component: %s\n", *component)
		}
		if *category != "" {
			fmt.Printf("   Category: %s\n", *category)
		}
		if *pattern != "" {
			fmt.Printf("   Pattern: %s\n", *pattern)
		}
		if *dependency != "" {
			fmt.Printf("   Dependency: %s\n", *dependency)
		}
		fmt.Println()
	}
	
	// Check if current incident has RCA data
	hasRCAData := current.Analysis != nil && 
		(current.Analysis.Component != "" || current.Analysis.Category != "" || 
		 current.Analysis.Pattern != "" || current.Analysis.Dependency != "")
	
	// Configure search options
	opts := SimilarityOptions{
		MaxResults:    *limit,
		MinConfidence: *minConf,
		DaysBack:      *days,
		IncludeStates: []State{StateResolved},
	}
	
	// Find similar incidents
	similar, err := service.FindSimilarIncidents(current, opts)
	if err != nil {
		output.Errorf("Failed to find similar incidents: %v", err)
		return 1
	}
	
	// Display results
	if len(similar) == 0 {
		fmt.Printf("\nNo similar incidents found in last %d days.\n\n", *days)
		
		// Provide helpful suggestions based on search parameters
		if *minConf > 0.5 {
			fmt.Println("💡 Try lowering the confidence threshold:")
			fmt.Printf("   health-monitor incident similar --min-confidence 0.5\n\n")
		}
		
		if *days < 180 {
			fmt.Println("💡 Try searching further back:")
			fmt.Printf("   health-monitor incident similar --days 180\n\n")
		}
		
		// Check if current incident has RCA data
		if current.Analysis == nil || 
		   (current.Analysis.Component == "" && current.Analysis.Category == "" && current.Analysis.Pattern == "") {
			fmt.Println("💡 This incident lacks RCA data, which reduces matching accuracy.")
			fmt.Println("   You can manually specify matching criteria:")
			fmt.Println("   health-monitor incident similar --component postgres_db --category capacity")
			fmt.Println()
			fmt.Println("   Or provide RCA fields when resolving for better future matching:")
			fmt.Println("   health-monitor incident resolve --root-cause \"...\" --fix \"...\" --category \"...\"")
		} else {
			fmt.Println("💡 This may be a new issue. Document it well for future reference!")
		}
		
		fmt.Println()
		return 0
	}
	
	// Warn if matches are low confidence due to missing RCA data
	if !hasRCAData && len(similar) > 0 {
		maxConf := similar[0].Confidence
		if maxConf < 0.6 {
			fmt.Println("\n⚠️  Low confidence matches (incident not yet analyzed)")
			fmt.Println("   For better results, try manual criteria:")
			fmt.Println("   health-monitor incident similar --component <name> --category <type>")
			fmt.Println()
		}
	}
	
	fmt.Printf("\nSimilar past incidents (last %dd):\n\n", *days)
	
	for i, sim := range similar {
		// Confidence stars
		stars := getConfidenceStars(sim.Confidence)
		confidencePct := int(sim.Confidence * 100)
		
		fmt.Printf("%d) %s  [%s]  %s %d%% match\n",
			i+1,
			sim.Incident.ID,
			sim.Incident.CreatedAt.Format("02 Jan 2006 15:04:05"),
			stars,
			confidencePct,
		)
		
		fmt.Printf("   Service: %s\n", sim.Incident.Service)
		
		if sim.Incident.Analysis != nil {
			if sim.Incident.Analysis.Dependency != "" {
				fmt.Printf("   Dependency: %s\n", sim.Incident.Analysis.Dependency)
			}
			if sim.Incident.Analysis.Pattern != "" {
				fmt.Printf("   Pattern: %s\n", sim.Incident.Analysis.Pattern)
			}
			if sim.Incident.Analysis.RootCause != "" {
				fmt.Printf("   Root cause: %s\n", sim.Incident.Analysis.RootCause)
			}
			if sim.Incident.Analysis.FixSummary != "" {
				fmt.Printf("   Fix: %s\n", sim.Incident.Analysis.FixSummary)
			}
		} else {
			if sim.Incident.Summary != "" {
				fmt.Printf("   Summary: %s\n", sim.Incident.Summary)
			}
		}
		
		if len(sim.MatchedOn) > 0 {
			fmt.Printf("   Matched on: %s\n", strings.Join(sim.MatchedOn, ", "))
		}
		
		fmt.Println()
	}
	
	fmt.Println("💡 Tip: Use 'health-monitor incident view --id <ID>' to see full details")
	fmt.Println()
	
	// Ask for feedback (optional, non-blocking)
	if len(similar) > 0 {
		collectFeedback(service, current, similar)
	}
	
	return 0
}

// collectFeedback asks user if similar incidents were helpful
func collectFeedback(service *Service, current *Incident, similar []SimilarIncident) {
	fmt.Print("Were these suggestions helpful? (y/n/skip): ")
	
	var response string
	fmt.Scanln(&response)
	
	response = strings.ToLower(strings.TrimSpace(response))
	if response == "skip" || response == "" {
		return
	}
	
	wasHelpful := response == "y" || response == "yes"
	
	// If helpful, ask which one
	var selectedID string
	if wasHelpful && len(similar) > 1 {
		fmt.Print("Which one was most helpful? (1-" + fmt.Sprintf("%d", len(similar)) + " or 'all'): ")
		var selection string
		fmt.Scanln(&selection)
		selection = strings.TrimSpace(selection)
		
		if selection != "all" && selection != "" {
			// Parse selection number
			var idx int
			if _, err := fmt.Sscanf(selection, "%d", &idx); err == nil && idx > 0 && idx <= len(similar) {
				selectedID = similar[idx-1].Incident.ID
			}
		}
	}
	
	// Save feedback
	feedbackStore := NewFeedbackStore(service.store.dir)
	
	if selectedID != "" {
		// Save feedback for specific incident
		for _, sim := range similar {
			if sim.Incident.ID == selectedID {
				feedback := SimilarityFeedback{
					IncidentID: current.ID,
					SimilarID:  sim.Incident.ID,
					Confidence: sim.Confidence,
					WasHelpful: true,
					MatchedOn:  sim.MatchedOn,
					Timestamp:  time.Now(),
					Command:    "similar",
				}
				_ = feedbackStore.Save(feedback)
				break
			}
		}
	} else {
		// Save feedback for all shown incidents
		for _, sim := range similar {
			feedback := SimilarityFeedback{
				IncidentID: current.ID,
				SimilarID:  sim.Incident.ID,
				Confidence: sim.Confidence,
				WasHelpful: wasHelpful,
				MatchedOn:  sim.MatchedOn,
				Timestamp:  time.Now(),
				Command:    "similar",
			}
			_ = feedbackStore.Save(feedback)
		}
	}
	
	if wasHelpful {
		fmt.Println("✅ Thanks for the feedback! This helps improve similarity matching.")
	} else {
		fmt.Println("📝 Thanks for the feedback! We'll use this to improve matching accuracy.")
	}
}

// getConfidenceStars returns star rating based on confidence
func getConfidenceStars(confidence float64) string {
	if confidence >= 0.8 {
		return "⭐⭐⭐"
	}
	if confidence >= 0.7 {
		return "⭐⭐"
	}
	return "⭐"
}
