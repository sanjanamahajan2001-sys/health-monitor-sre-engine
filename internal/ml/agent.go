package ml

import (
	"fmt"
	"time"
	"strings"
	"log"
	"math"
	"encoding/json"
)

// MLAgent manages semantic analysis logic
type MLAgent struct {
	vectorizer    *SimpleVectorizer
	version       string
	KnowledgeBase []LifecycleSnapshot
}

// Global vocabulary for SRE/Ops context
var defaultVocab = []string{
	"timeout", "refused", "exhausted", "capacity", "latency",
	"memory", "cpu", "disk", "network", "database", "postgres",
	"redis", "connection", "gateway", "unauthenticated", "denied",
	"slow", "error", "critical", "warning", "deadlock", "zombie",
	"restart", "scaling", "spike", "drop", "missing", "invalid",
	"saturated", "exhaustion", "leak", "contention", "throttled",
}

func NewMLAgent(version string) *MLAgent {
	return &MLAgent{
		vectorizer: NewSimpleVectorizer(defaultVocab),
		version:    version,
	}
}

// Predict provides a semantic hint based on text (e.g., log sample or title)
func (a *MLAgent) Predict(text string) *Prediction {
	v := a.vectorizer.Vectorize(text)
	
	// Prototype logic: identify the most prominent word
	var topWord string
	var topVal float64
	for word, idx := range a.vectorizer.Vocabulary {
		if v[idx] > topVal {
			topVal = v[idx]
			topWord = word
		}
	}

	if topWord == "" {
		return nil
	}

	prediction := &Prediction{
		ID:          fmt.Sprintf("PRED-%d", time.Now().UnixNano()),
		Pattern:     topWord + "_detected",
		Confidence:  0.4, // 🔌 REDUCED CONFIDENCE for generic keywords
		Explanation: fmt.Sprintf("Identified '%s' as the key semantic feature.", topWord),
		ModelID:     "prototype_" + a.version,
		Timestamp:   time.Now(),
	}

	// Map features to categories and hints
	switch topWord {
	case "connection", "refused", "gateway":
		prediction.Category = "connectivity"
		prediction.ResolutionHint = "Check network availability, firewall rules, and target service status."
	case "timeout", "slow":
		prediction.Category = "latency"
		prediction.ResolutionHint = "Investigate resource saturation or dependency performance degradation."
	case "memory", "cpu", "disk", "capacity", "exhausted":
		prediction.Category = "capacity"
		prediction.ResolutionHint = "Scale resources or perform cleanup of temporary files/leaked connections."
	case "database", "postgres", "redis":
		prediction.Category = "database"
		prediction.ResolutionHint = "Check database logs, connection pool saturation, and lock contention."
	case "denied", "unauthenticated":
		prediction.Category = "security"
		prediction.ResolutionHint = "Verify credentials, token expiry, and IAM/Access Control policies."
	default:
		prediction.Category = "general"
		prediction.ResolutionHint = "Review logs for full stack trace and correlation IDs."
	}

	return prediction
}

// UpdateKnowledge hydrates the agent with past incident snapshots
func (a *MLAgent) UpdateKnowledge(snapshots []LifecycleSnapshot) {
	a.KnowledgeBase = snapshots
}

// ClassifySignal determines the system phase based on signal strength
func (a *MLAgent) ClassifySignal(s LifecycleSnapshot) string {
	return a.classifySignalInternal(s)
}

func (a *MLAgent) classifySignalInternal(s LifecycleSnapshot) string {
	// 1. Check for Critical Errors -> "pre-failure"
	if s.FullLogs != nil {
		for _, e := range s.FullLogs.TopErrors {
			msg := strings.ToLower(e.Message)
			sample := strings.ToLower(e.Sample)
			// Look for severe degradation or failures
			if strings.Contains(msg, "error") || strings.Contains(msg, "fail") || 
			   strings.Contains(msg, "exhausted") || strings.Contains(msg, "timeout") ||
			   strings.Contains(msg, "refused") || strings.Contains(msg, "deadlock") ||
			   strings.Contains(msg, "panic") || strings.Contains(msg, "fatal") ||
			   strings.Contains(msg, "severe") || strings.Contains(msg, "critical") ||
			   strings.Contains(sample, "error") || strings.Contains(sample, "exception") ||
			   strings.Contains(sample, "panic") || strings.Contains(sample, "500") ||
			   strings.Contains(sample, "503") || strings.Contains(sample, "504") {
				return "pre-failure"
			}
		}
	}

	// 2. Check for Saturation/Warning Signals -> "pre-incident"
	if s.FullLogs != nil {
		for _, e := range s.FullLogs.TopErrors {
			msg := strings.ToLower(e.Message)
			sample := strings.ToLower(e.Sample)
			if strings.Contains(msg, "warning") || strings.Contains(msg, "warn") ||
			   strings.Contains(msg, "saturation") || strings.Contains(msg, "pool") || 
			   strings.Contains(msg, "near limit") || strings.Contains(msg, "high") ||
			   strings.Contains(msg, "usage") || strings.Contains(msg, "backlog") ||
			   strings.Contains(msg, "slow") || strings.Contains(msg, "retry") ||
			   strings.Contains(sample, "warn") || strings.Contains(sample, "400") ||
			   strings.Contains(sample, "401") || strings.Contains(sample, "403") ||
			   strings.Contains(sample, "404") || strings.Contains(sample, "429") {
				return "pre-incident"
			}
		}
	}

	// 3. Check for Latency spikes (Metrics and Traces)
	latency := 0.0
	if s.Metrics != nil {
		if mLat, ok := s.Metrics["service_avg_latency"]; ok {
			latency = mLat
		}
	}
	if s.FullTraces != nil && s.FullTraces.P95LatencyMs > latency {
		latency = s.FullTraces.P95LatencyMs
	}

	if latency > 5000 {
		// output.Infof silenced for noise reduction
		return "pre-failure" // Severe latency
	}
	if latency > 1000 {
		return "pre-incident" // Noticeable degradation
	}
	
	// 4. Check for Error Rates (Metrics and Traces)
	errRate := 0.0
	if s.Metrics != nil {
		if mErr, ok := s.Metrics["service_error_rate"]; ok {
			errRate = mErr
		}
	}
	if s.FullTraces != nil {
		// Normalize trace error rate (0-100) to 0.0-1.0 if it looks like percentage
		tRate := s.FullTraces.ErrorRate
		if tRate > 1.0 {
			tRate = tRate / 100.0
		}
		if tRate > errRate {
			errRate = tRate
		}
	}

	if errRate > 0.05 && (s.FullTraces != nil && s.FullTraces.ErrorCount >= 10) {
		return "pre-failure" // >5% error rate is failure
	}
	if errRate > 0.01 {
		return "pre-incident" // >1% error rate is degradation
	}

	// 5. Absolute Error Count (Log-Centric Priority)
	errCount := 0.0
	hasCriticalLogs := false
	if s.FullLogs != nil {
		for _, e := range s.FullLogs.TopErrors {
			errCount += float64(e.Count)
			msg := strings.ToUpper(e.Message)
			if strings.Contains(msg, "WARN") || strings.Contains(msg, "FAIL") || strings.Contains(msg, "ERROR") || strings.Contains(msg, "TIMEOUT") {
				hasCriticalLogs = true
			}
		}
	}
	
	if s.Metrics != nil {
		if mCount, ok := s.Metrics["service_error_count"]; ok && mCount > errCount {
			errCount = mCount
		}
	}
	if s.FullTraces != nil && float64(s.FullTraces.ErrorCount) > errCount {
		errCount = float64(s.FullTraces.ErrorCount)
	}

	// For low-volume traffic, use absolute counts and keywords
	// Raised thresholds to reduce noise in large environments
	if errCount > 50 || (errCount >= 20 && hasCriticalLogs) {
		return "pre-failure"
	}
	if errCount > 10 || (errCount > 0 && hasCriticalLogs) {
		return "pre-incident"
	}

	// 4. Default: Steady or Active Fallback
	if s.Metadata != nil {
		if _, ok := s.Metadata["active_incident_id"]; ok {
			return "during"
		}
	}
	if s.IncidentID != "" {
		return "during"
	}

	return "steady"
}

// PredictState provides a prediction based on a system state snapshot
func (a *MLAgent) PredictState(s LifecycleSnapshot) *Prediction {
	var bestMatch *LifecycleSnapshot
	var maxSimilarity float64

	// 1. Dynamic Matching: Check Knowledge Base for similar past incidents
	currentPhase := a.ClassifySignal(s)
	if len(a.KnowledgeBase) > 0 {
		// ... (vectorize currentLogs)
		currentLogs := ""
		if s.FullLogs != nil {
			for _, e := range s.FullLogs.TopErrors {
				currentLogs += e.Message + " "
			}
		} else if len(s.LogSignatures) > 0 {
			currentLogs = s.LogSignatures[0]
		}

		if currentLogs != "" {
			currentV := a.vectorizer.Vectorize(currentLogs)

			for i := range a.KnowledgeBase {
				past := &a.KnowledgeBase[i]
				
				// Weight phase alignment: if phases match, boost similarity
				phaseBoost := 1.0
				if past.Phase == currentPhase && currentPhase != "steady" {
					phaseBoost = 1.1 // Slight boost for phase alignment
				}

				// 🚀 SERVICE MATCH BOOST (Noise Reduction)
				// If this past incident was actually for THIS service, it's MUCH more relevant.
				serviceBoost := 1.0
				if past.Metadata["primary_service"] == s.Service || past.Metadata["service"] == s.Service {
					serviceBoost = 1.5 // Strong boost for self-matching
				}

				pastLogs := ""
				if past.FullLogs != nil {
					for _, e := range past.FullLogs.TopErrors {
						pastLogs += e.Message + " "
					}
				} else if len(past.LogSignatures) > 0 {
					pastLogs = past.LogSignatures[0]
				}

				if pastLogs != "" {
					pastV := a.vectorizer.Vectorize(pastLogs)
					sim := CosineSimilarity(currentV, pastV) * phaseBoost * serviceBoost
					if sim > maxSimilarity {
						maxSimilarity = sim
						bestMatch = past
					}
				}
			}
			
			// 🔍 DEBUG LOG: Crucial for identifying why certain services are silent
			if maxSimilarity > 0.1 {
				log.Printf("[DEBUG] Service %s: BEST KB match similarity = %.2f (Incident: %s, PastSvc: %s, CurrentPhase: %s)", 
					s.Service, maxSimilarity, bestMatch.IncidentID, bestMatch.Metadata["primary_service"], currentPhase)
			}
		} else {
			// No logs found for this service in this window
		}
	}
		// 🚀 KNOWLEDGE BASE ONLY (Production Requirement)
		// Lowered threshold to 0.6 to catch partial matches (e.g. 3 out of 5 logs).
		if maxSimilarity > 0.6 && bestMatch != nil {
			log.Printf("[TRIGGERED] Service %s: Similarity %.2f matched past incident %s", s.Service, maxSimilarity, bestMatch.IncidentID)
			prediction := &Prediction{
				ID:          fmt.Sprintf("PRED-%d", time.Now().UnixNano()),
				Pattern:     fmt.Sprintf("matched_past_%s", bestMatch.Phase),
				Component:   bestMatch.Metadata["primary_service"],
				Confidence:  math.Min(maxSimilarity, 1.0),
				Explanation: fmt.Sprintf("Strong semantic match (%.2f) to past incident %s during its %s phase.", 
					math.Min(maxSimilarity, 1.0), bestMatch.IncidentID, bestMatch.Phase),
				Timestamp:   time.Now(),
				Snapshot:    s,
				ModelID:     "dynamic_" + a.version,
			}

			// Map RCA data from metadata
			prediction.RootCause = bestMatch.Metadata["root_cause"]
			prediction.Prevention = bestMatch.Metadata["prevention"]
			prediction.FixSummary = bestMatch.Metadata["fix"]
			prediction.Category = bestMatch.Metadata["category"]
			
			// 🚀 NEW: Extract RCA Lessons and Actions
			if lessonsJSON, ok := bestMatch.Metadata["lessons_learned"]; ok && lessonsJSON != "" {
				var ll LessonsLearned
				if err := json.Unmarshal([]byte(lessonsJSON), &ll); err == nil {
					prediction.LessonsLearned = &ll
				}
			}
			if actionsJSON, ok := bestMatch.Metadata["action_items"]; ok && actionsJSON != "" {
				var ai []string
				if err := json.Unmarshal([]byte(actionsJSON), &ai); err == nil {
					prediction.ActionItems = ai
				}
			}

			// 🚀 RESOLUTION INTELLIGENCE: Build a highly actionable hint
			if prediction.Prevention != "" {
				prediction.ResolutionHint = fmt.Sprintf("PAST RESOLUTION: %s. RECOMMENDED PREVENTION: %s", 
					prediction.FixSummary, prediction.Prevention)
			} else if prediction.RootCause != "" {
				prediction.ResolutionHint = fmt.Sprintf("KNOWLEDGE BASE: This signal matched past root cause '%s'. Review logs for alignment.", 
					prediction.RootCause)
			} else {
				prediction.ResolutionHint = "Compare current logs with past incident " + bestMatch.IncidentID
			}

			return prediction
		}

	// 🛑 BOOTSTRAP FALLBACKS DISABLED (No Noise Requirement)
	// We no longer return generic "error_detected" or "high_latency" predictions
	// unless they are part of a Knowledge Base fingerprint.
	
	return nil
}

// FindSimilar matches a search text against a set of candidate texts
func (a *MLAgent) FindSimilar(text string, candidates []string) []float64 {
	targetV := a.vectorizer.Vectorize(text)
	results := make([]float64, len(candidates))
	
	for i, candidate := range candidates {
		candidateV := a.vectorizer.Vectorize(candidate)
		results[i] = CosineSimilarity(targetV, candidateV)
	}
	
	return results
}

// Summarize tries to extract a concise pattern from text
func (a *MLAgent) Summarize(text string) string {
	prediction := a.Predict(text)
	if prediction != nil {
		return prediction.Pattern
	}
	return "unknown_pattern"
}
