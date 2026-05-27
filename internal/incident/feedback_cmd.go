package incident

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

// handleFeedbackStats shows similarity feedback statistics
func handleFeedbackStats(service *Service, args []string) int {
	fs := flag.NewFlagSet("incident feedback-stats", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dataDir := fs.String("data-dir", "", "data directory")
	
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if strings.TrimSpace(*dataDir) != "" {
		_ = os.Setenv("HEALTH_MONITOR_DATA_DIR", strings.TrimSpace(*dataDir))
	}
	
	feedbackStore := NewFeedbackStore(service.store.dir)
	total, helpful, notHelpful, avgConfHelpful, avgConfNotHelpful := feedbackStore.GetStats()
	
	if total == 0 {
		fmt.Println("\n📊 Similarity Feedback Statistics")
		fmt.Println("═══════════════════════════════════")
		fmt.Println("\nNo feedback collected yet.")
		fmt.Println("\n💡 Feedback is collected when you use 'incident similar' command.")
		fmt.Println()
		return 0
	}
	
	fmt.Println("\n📊 Similarity Feedback Statistics")
	fmt.Println("═══════════════════════════════════")
	fmt.Printf("\nTotal feedback: %d\n", total)
	fmt.Printf("  ✅ Helpful: %d (%.1f%%)\n", helpful, float64(helpful)/float64(total)*100)
	fmt.Printf("  ❌ Not helpful: %d (%.1f%%)\n", notHelpful, float64(notHelpful)/float64(total)*100)
	
	fmt.Println("\nAverage Confidence Scores:")
	if helpful > 0 {
		fmt.Printf("  ✅ Helpful suggestions: %.1f%%\n", avgConfHelpful*100)
	}
	if notHelpful > 0 {
		fmt.Printf("  ❌ Not helpful suggestions: %.1f%%\n", avgConfNotHelpful*100)
	}
	
	// Load all feedback to analyze patterns
	feedbacks, _ := feedbackStore.LoadAll()
	if len(feedbacks) > 0 {
		// Count by matched fields
		matchedFieldCounts := make(map[string]int)
		matchedFieldHelpful := make(map[string]int)
		
		for _, fb := range feedbacks {
			for _, field := range fb.MatchedOn {
				matchedFieldCounts[field]++
				if fb.WasHelpful {
					matchedFieldHelpful[field]++
				}
			}
		}
		
		if len(matchedFieldCounts) > 0 {
			fmt.Println("\nMost Helpful Match Types:")
			for field, count := range matchedFieldCounts {
				helpfulCount := matchedFieldHelpful[field]
				helpfulPct := float64(helpfulCount) / float64(count) * 100
				fmt.Printf("  • %s: %.1f%% helpful (%d/%d)\n", field, helpfulPct, helpfulCount, count)
			}
		}
	}
	
	fmt.Println("\n💡 Use this data to tune similarity thresholds and weights.")
	fmt.Println("   Feedback is stored in: " + service.store.dir + "/similarity_feedback/")
	fmt.Println()
	
	return 0
}
