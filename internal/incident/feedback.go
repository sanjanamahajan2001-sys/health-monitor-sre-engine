package incident

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// SimilarityFeedback tracks user feedback on similarity suggestions
type SimilarityFeedback struct {
	IncidentID      string    `json:"incident_id"`       // Current incident
	SimilarID       string    `json:"similar_id"`        // Suggested similar incident
	Confidence      float64   `json:"confidence"`        // Confidence score shown
	WasHelpful      bool      `json:"was_helpful"`       // User feedback
	MatchedOn       []string  `json:"matched_on"`        // Fields that matched
	Timestamp       time.Time `json:"timestamp"`         // When feedback was given
	Command         string    `json:"command,omitempty"` // Command context (start/resolve/similar)
}

// FeedbackStore manages similarity feedback persistence
type FeedbackStore struct {
	dataDir string
}

// NewFeedbackStore creates a new feedback store
func NewFeedbackStore(dataDir string) *FeedbackStore {
	return &FeedbackStore{
		dataDir: dataDir,
	}
}

// Save saves feedback to disk
func (fs *FeedbackStore) Save(feedback SimilarityFeedback) error {
	feedbackDir := filepath.Join(fs.dataDir, "similarity_feedback")
	if err := os.MkdirAll(feedbackDir, 0755); err != nil {
		return err
	}

	filename := filepath.Join(feedbackDir, feedback.IncidentID+".json")
	
	// Load existing feedback for this incident
	var feedbacks []SimilarityFeedback
	if data, err := os.ReadFile(filename); err == nil {
		_ = json.Unmarshal(data, &feedbacks)
	}
	
	// Append new feedback
	feedbacks = append(feedbacks, feedback)
	
	// Save back
	data, err := json.MarshalIndent(feedbacks, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(filename, data, 0644)
}

// LoadAll loads all feedback records
func (fs *FeedbackStore) LoadAll() ([]SimilarityFeedback, error) {
	feedbackDir := filepath.Join(fs.dataDir, "similarity_feedback")
	
	entries, err := os.ReadDir(feedbackDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	
	var allFeedback []SimilarityFeedback
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		
		data, err := os.ReadFile(filepath.Join(feedbackDir, entry.Name()))
		if err != nil {
			continue
		}
		
		var feedbacks []SimilarityFeedback
		if err := json.Unmarshal(data, &feedbacks); err != nil {
			continue
		}
		
		allFeedback = append(allFeedback, feedbacks...)
	}
	
	return allFeedback, nil
}

// GetStats returns feedback statistics
func (fs *FeedbackStore) GetStats() (total, helpful, notHelpful int, avgConfidenceHelpful, avgConfidenceNotHelpful float64) {
	feedbacks, err := fs.LoadAll()
	if err != nil || len(feedbacks) == 0 {
		return 0, 0, 0, 0, 0
	}
	
	total = len(feedbacks)
	var sumConfidenceHelpful, sumConfidenceNotHelpful float64
	
	for _, fb := range feedbacks {
		if fb.WasHelpful {
			helpful++
			sumConfidenceHelpful += fb.Confidence
		} else {
			notHelpful++
			sumConfidenceNotHelpful += fb.Confidence
		}
	}
	
	if helpful > 0 {
		avgConfidenceHelpful = sumConfidenceHelpful / float64(helpful)
	}
	if notHelpful > 0 {
		avgConfidenceNotHelpful = sumConfidenceNotHelpful / float64(notHelpful)
	}
	
	return
}
