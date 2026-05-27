package guide

// Chapter represents a major section of the guided tour.
type Chapter struct {
	ID          string
	Title       string
	Description string
	Steps       []Step
}

// Step represents a single interaction or information screen within a chapter.
type Step struct {
	Title       string
	Content     string
	Keywords    []string                     // New: For search functionality
	Action      func(m *UnifiedGuideModel, s *Step) error // Updated: Now accepts model for state access
	Verification func() bool                  // Optional verification check to proceed
	Hint         string                       // New: Technical deep-dive/hint (toggled with 'h')
	ManualInput  bool                         // New: Trigger input prompt immediately
}

// Scenario defines a high-level user journey.
type Scenario struct {
	ID          string
	Name        string
	Description string
	Keywords    []string // New: For search functionality
	Chapters    []Chapter
}
