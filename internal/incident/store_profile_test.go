package incident

import (
	"path/filepath"
	"testing"
	"time"

	"health-monitor/internal/config"
)

func TestProfileIsolatedIncidentStore(t *testing.T) {
	tempDir := t.TempDir()
	
	// Mock profile manager
	pm := config.GetProfileManager()
	pm.basePath = tempDir
	
	// Create stores for different profiles
	pm.SetActiveProfile("profile1")
	store1, err := NewStore()
	if err != nil {
		t.Fatalf("Failed to create store1: %v", err)
	}
	
	pm.SetActiveProfile("profile2")
	store2, err := NewStore()
	if err != nil {
		t.Fatalf("Failed to create store2: %v", err)
	}
	
	// Verify stores use different directories
	if store1.dir == store2.dir {
		t.Error("Incident stores should use profile-isolated directories")
	}
	
	// Verify directory structure
	expected1 := filepath.Join(tempDir, "state", "profile1", "incidents")
	expected2 := filepath.Join(tempDir, "state", "profile2", "incidents")
	
	if store1.dir != expected1 {
		t.Errorf("Expected %s, got %s", expected1, store1.dir)
	}
	
	if store2.dir != expected2 {
		t.Errorf("Expected %s, got %s", expected2, store2.dir)
	}
}

func TestProfileIncidentIsolation(t *testing.T) {
	tempDir := t.TempDir()
	
	// Mock profile manager
	pm := config.GetProfileManager()
	pm.basePath = tempDir
	
	// Create incident in profile1
	pm.SetActiveProfile("profile1")
	store1, err := NewStore()
	if err != nil {
		t.Fatalf("Failed to create store1: %v", err)
	}
	
	incident1 := Incident{
		ID:        "incident-1",
		Title:     "Profile 1 Incident",
		Service:   "service1",
		Severity:  "P1",
		State:     "active",
		CreatedAt: time.Now(),
	}
	
	err = store1.Save(incident1)
	if err != nil {
		t.Fatalf("Failed to save incident1: %v", err)
	}
	
	// Verify incident exists in profile1
	loaded1, err := store1.Load("incident-1")
	if err != nil {
		t.Fatalf("Failed to load incident1 from profile1: %v", err)
	}
	
	if loaded1.Title != "Profile 1 Incident" {
		t.Error("Incident title mismatch in profile1")
	}
	
	// Verify incident doesn't exist in profile2
	pm.SetActiveProfile("profile2")
	store2, err := NewStore()
	if err != nil {
		t.Fatalf("Failed to create store2: %v", err)
	}
	
	_, err = store2.Load("incident-1")
	if err == nil {
		t.Error("Incident should not be accessible from different profile")
	}
	
	// Create different incident in profile2
	incident2 := Incident{
		ID:        "incident-1", // Same ID but different profile
		Title:     "Profile 2 Incident",
		Service:   "service2",
		Severity:  "P2",
		State:     "active",
		CreatedAt: time.Now(),
	}
	
	err = store2.Save(incident2)
	if err != nil {
		t.Fatalf("Failed to save incident2: %v", err)
	}
	
	// Verify incidents are separate
	loaded2, err := store2.Load("incident-1")
	if err != nil {
		t.Fatalf("Failed to load incident2 from profile2: %v", err)
	}
	
	if loaded2.Title != "Profile 2 Incident" {
		t.Error("Incident title mismatch in profile2")
	}
	
	// Verify profile1 still has original incident
	pm.SetActiveProfile("profile1")
	loaded1Again, err := store1.Load("incident-1")
	if err != nil {
		t.Fatalf("Failed to reload incident1 from profile1: %v", err)
	}
	
	if loaded1Again.Title != "Profile 1 Incident" {
		t.Error("Profile 1 incident should be unchanged")
	}
}

func TestProfileIncidentListIsolation(t *testing.T) {
	tempDir := t.TempDir()
	
	// Mock profile manager
	pm := config.GetProfileManager()
	pm.basePath = tempDir
	
	// Create incidents in profile1
	pm.SetActiveProfile("profile1")
	store1, err := NewStore()
	if err != nil {
		t.Fatalf("Failed to create store1: %v", err)
	}
	
	incident1a := Incident{
		ID:        "incident-1a",
		Title:     "Profile 1 Incident A",
		Service:   "service1",
		Severity:  "P1",
		State:     "active",
		CreatedAt: time.Now(),
	}
	
	incident1b := Incident{
		ID:        "incident-1b",
		Title:     "Profile 1 Incident B",
		Service:   "service1",
		Severity:  "P2",
		State:     "resolved",
		CreatedAt: time.Now(),
	}
	
	err = store1.Save(incident1a)
	if err != nil {
		t.Fatalf("Failed to save incident1a: %v", err)
	}
	
	err = store1.Save(incident1b)
	if err != nil {
		t.Fatalf("Failed to save incident1b: %v", err)
	}
	
	// Create incidents in profile2
	pm.SetActiveProfile("profile2")
	store2, err := NewStore()
	if err != nil {
		t.Fatalf("Failed to create store2: %v", err)
	}
	
	incident2a := Incident{
		ID:        "incident-2a",
		Title:     "Profile 2 Incident A",
		Service:   "service2",
		Severity:  "P1",
		State:     "active",
		CreatedAt: time.Now(),
	}
	
	err = store2.Save(incident2a)
	if err != nil {
		t.Fatalf("Failed to save incident2a: %v", err)
	}
	
	// Verify list isolation
	incidents1, _, err := store1.List()
	if err != nil {
		t.Fatalf("Failed to list incidents from profile1: %v", err)
	}
	
	incidents2, _, err := store2.List()
	if err != nil {
		t.Fatalf("Failed to list incidents from profile2: %v", err)
	}
	
	if len(incidents1) != 2 {
		t.Errorf("Expected 2 incidents in profile1, got %d", len(incidents1))
	}
	
	if len(incidents2) != 1 {
		t.Errorf("Expected 1 incident in profile2, got %d", len(incidents2))
	}
	
	// Verify correct incidents
	var found1a, found1b bool
	for _, inc := range incidents1 {
		if inc.ID == "incident-1a" {
			found1a = true
		}
		if inc.ID == "incident-1b" {
			found1b = true
		}
	}
	
	if !found1a || !found1b {
		t.Error("Profile 1 incidents not found in list")
	}
	
	if incidents2[0].ID != "incident-2a" {
		t.Error("Profile 2 incident not found in list")
	}
}

func TestFeedbackStoreIsolation(t *testing.T) {
	tempDir := t.TempDir()
	
	// Mock profile manager
	pm := config.GetProfileManager()
	pm.basePath = tempDir
	
	// Create feedback stores for different profiles
	pm.SetActiveProfile("profile1")
	store1, err := NewStore()
	if err != nil {
		t.Fatalf("Failed to create store1: %v", err)
	}
	
	feedbackStore1 := NewFeedbackStore(store1)
	
	pm.SetActiveProfile("profile2")
	store2, err := NewStore()
	if err != nil {
		t.Fatalf("Failed to create store2: %v", err)
	}
	
	feedbackStore2 := NewFeedbackStore(store2)
	
	// Verify feedback stores use different directories
	if feedbackStore1.dataDir == feedbackStore2.dataDir {
		t.Error("Feedback stores should use profile-isolated directories")
	}
	
	// Create feedback in profile1
	pm.SetActiveProfile("profile1")
	feedback1 := SimilarityFeedback{
		IncidentID:     "incident-1",
		SimilarIncident: "similar-1",
		Helpful:       true,
		Confidence:    0.8,
		Timestamp:     time.Now(),
	}
	
	err = feedbackStore1.Save(feedback1)
	if err != nil {
		t.Fatalf("Failed to save feedback1: %v", err)
	}
	
	// Verify feedback exists in profile1
	feedbacks1, err := feedbackStore1.LoadAll()
	if err != nil {
		t.Fatalf("Failed to load feedbacks from profile1: %v", err)
	}
	
	if len(feedbacks1) != 1 {
		t.Errorf("Expected 1 feedback in profile1, got %d", len(feedbacks1))
	}
	
	// Verify feedback doesn't exist in profile2
	pm.SetActiveProfile("profile2")
	feedbacks2, err := feedbackStore2.LoadAll()
	if err != nil {
		t.Fatalf("Failed to load feedbacks from profile2: %v", err)
	}
	
	if len(feedbacks2) != 0 {
		t.Error("Feedback should not be accessible from different profile")
	}
}
