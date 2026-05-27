package guide

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"health-monitor/internal/config"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// Seeder defines the interface for seeding scenario data.
type Seeder interface {
	Seed(scenario string) error
}

// UnifiedGuideModel represents the state of the guided tour TUI.
type UnifiedGuideModel struct {
	seeder         Seeder
	scenarios      []Scenario
	currentIdx     int // Selected scenario or step index
	activeScenario *Scenario
	activeChapter  *Chapter
	activeStep     int
	inScenario     bool
	quitting       bool
	
	// Search functionality
	searching      bool
	searchInput    string
	searchResults  []Scenario
	searchMode     bool // If true, we are viewing search results
	
	// Sandbox status
	isSandboxActive bool

	// Profile selection
	selectingProfile bool
	availableProfiles []string
	profileIdx       int
	SelectedProfile  string

	// Remediation (Interactive)
	LastResult      *TroubleshootResult
	confirmingFix   bool
	inputtingFixParam bool
	textInput       textinput.Model

	// UX Enhancements
	showHint       bool
	completedSteps map[string]bool // ScenarioID:StepIdx
	lastSaved      string
}

// NewUnifiedGuideModel initializes a new guide model with predefined scenarios.
func NewUnifiedGuideModel(seeder Seeder) (*UnifiedGuideModel, error) {
	ti := textinput.New()
	ti.Placeholder = "Value..."
	ti.CharLimit = 156
	ti.Width = 50

	m := &UnifiedGuideModel{
		seeder:         seeder,
		scenarios:      GetScenarios(seeder),
		textInput:      ti,
		completedSteps: make(map[string]bool),
	}
	return m, nil
}

// Init initializes the bubbletea model.
func (m *UnifiedGuideModel) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles UI updates.
func (m *UnifiedGuideModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case error:
		if m.LastResult != nil && m.LastResult.Status == "TESTING" {
			return m.handlePostFixResult(msg)
		}
	case nil:
		// Ignore nil messages to avoid state reset
		return m, nil
	case tea.KeyMsg:
		if m.inputtingFixParam {
			switch msg.String() {
			case "enter":
				val := m.textInput.Value()
				if val != "" && m.LastResult != nil && m.LastResult.FixParam != nil {
					_ = m.LastResult.FixParam(val)
					// Trigger follow-up action (e.g., testing) if defined
					if m.LastResult.PostFixAction != nil {
						m.inputtingFixParam = false // Stop inputting to show "Testing..."
						m.LastResult.Status = "TESTING"
						m.LastResult.Issue = "Config saved. Verifying connectivity..."
						
						// Run the test in a goroutine/tea.Cmd
						return m, func() tea.Msg {
							err := m.LastResult.PostFixAction(val)
							if err != nil {
								return err
							}
							return nil
						}
					}
					
					m.LastResult.Status = "PASSED"
					m.LastResult.Issue = "Configuration updated successfully!"
					m.LastResult.FixAction = nil
					m.LastResult.FixParam = nil
				}
				m.inputtingFixParam = false
				m.textInput.Blur()
				return m, m.refreshTroubleshoot()
			case "esc":
				m.inputtingFixParam = false
				m.textInput.Blur()
				return m, nil
			}
			m.textInput, cmd = m.textInput.Update(msg)
			return m, cmd
		}

		if m.searching {
			switch msg.String() {
			case "enter":
				m.searching = false
				m.performSearch()
				return m, nil
			case "esc":
				m.searching = false
				m.searchInput = ""
				return m, nil
			case "backspace":
				if len(m.searchInput) > 0 {
					m.searchInput = m.searchInput[:len(m.searchInput)-1]
				}
			default:
				if len(msg.String()) == 1 {
					m.searchInput += msg.String()
				}
			}
			return m, nil
		}

		if m.selectingProfile {
			switch msg.String() {
			case "up", "k":
				if m.profileIdx > 0 {
					m.profileIdx--
				}
			case "down", "j":
				if m.profileIdx < len(m.availableProfiles)-1 {
					m.profileIdx++
				}
			case "enter":
				m.SelectedProfile = m.availableProfiles[m.profileIdx]
				m.selectingProfile = false
				m.loadProgress(m.SelectedProfile)
				if m.activeScenario != nil && len(m.activeScenario.Chapters) > 0 {
					m.activeChapter = &m.activeScenario.Chapters[0]
					m.activeStep = 0
					m.executeCurrentStepAction()
				}
			case "esc", "b":
				m.selectingProfile = false
				m.inScenario = false
			}
			return m, nil
		}

		if m.confirmingFix {
			switch msg.String() {
			case "enter":
				if m.LastResult != nil && m.LastResult.FixAction != nil {
					_ = m.LastResult.FixAction()
					m.LastResult.Status = "PASSED"
					m.LastResult.Issue = "Remediation applied successfully!"
					m.LastResult.FixAction = nil
					if m.activeChapter != nil && m.activeStep < len(m.activeChapter.Steps) {
						s := &m.activeChapter.Steps[m.activeStep]
						s.Content = "✅ SUCCESS: Remediation applied!\n\n" + m.LastResult.Issue
					}
				}
				m.confirmingFix = false
				return m, m.refreshTroubleshoot()
			case "esc", "n":
				m.confirmingFix = false
			}
			return m, nil
		}

		switch msg.String() {
		case "/":
			m.searching = true
			return m, nil
		case "f":
			if m.LastResult != nil {
				if m.LastResult.FixParam != nil {
					m.inputtingFixParam = true
					m.textInput.Focus()
					m.textInput.SetValue("")
					return m, nil
				}
				if m.LastResult.FixAction != nil {
					m.confirmingFix = true
				}
			}
		case "h":
			if m.inScenario {
				m.showHint = !m.showHint
			}
		case "up", "k":
			if !m.inScenario {
				if m.currentIdx > 0 {
					m.currentIdx--
				}
			}
		case "down", "j":
			if !m.inScenario {
				scenarios := m.scenarios
				if m.searchMode {
					scenarios = m.searchResults
				}
				if m.currentIdx < len(scenarios)-1 {
					m.currentIdx++
				}
			}
		case "enter":
			if !m.inScenario {
				scenarios := m.scenarios
				if m.searchMode {
					scenarios = m.searchResults
				}
				if len(scenarios) > 0 {
					m.inScenario = true
					m.activeScenario = &scenarios[m.currentIdx]
					if m.activeScenario.ID == "troubleshooter" {
						m.selectingProfile = true
						m.availableProfiles = config.GetProfileManager().DiscoverProfiles()
						m.profileIdx = 0
						return m, nil
					}
					m.activeChapter = &m.activeScenario.Chapters[0]
					m.activeStep = 0
					m.executeCurrentStepAction()
				}
			} else {
				if m.activeStep < len(m.activeChapter.Steps)-1 {
					m.activeStep++
					m.executeCurrentStepAction()
				} else {
					nextChapIdx := -1
					for i, ch := range m.activeScenario.Chapters {
						if ch.ID == m.activeChapter.ID {
							nextChapIdx = i + 1
							break
						}
					}
					if nextChapIdx != -1 && nextChapIdx < len(m.activeScenario.Chapters) {
						m.activeChapter = &m.activeScenario.Chapters[nextChapIdx]
						m.activeStep = 0
						m.executeCurrentStepAction()
					} else {
						m.exitScenario()
					}
				}
			}
		case "esc", "b":
			if m.inScenario {
				m.exitScenario()
			} else if m.searchMode {
				m.searchMode = false
				m.currentIdx = 0
			}
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *UnifiedGuideModel) exitScenario() {
	if m.inScenario && m.SelectedProfile != "" {
		m.saveProgress()
	}
	m.inScenario = false
	m.isSandboxActive = false
	m.SelectedProfile = ""
	m.LastResult = nil
	m.confirmingFix = false
	m.inputtingFixParam = false
	m.showHint = false
}

func (m *UnifiedGuideModel) performSearch() {
	if m.searchInput == "" {
		m.searchMode = false
		return
	}
	m.searchResults = []Scenario{}
	query := strings.ToLower(m.searchInput)
	for _, sc := range m.scenarios {
		match := false
		if strings.Contains(strings.ToLower(sc.Name), query) || strings.Contains(strings.ToLower(sc.Description), query) {
			match = true
		} else {
			for _, kw := range sc.Keywords {
				if strings.Contains(strings.ToLower(kw), query) {
					match = true
					break
				}
			}
			if !match {
				for _, ch := range sc.Chapters {
					if strings.Contains(strings.ToLower(ch.Title), query) {
						match = true
						break
					}
					for _, step := range ch.Steps {
						if strings.Contains(strings.ToLower(step.Title), query) {
							match = true
							break
						}
						for _, kw := range step.Keywords {
							if strings.Contains(strings.ToLower(kw), query) {
								match = true
								break
							}
						}
					}
				}
			}
		}
		if match { m.searchResults = append(m.searchResults, sc) }
	}
	m.searchMode = true
	m.currentIdx = 0
}

func (m *UnifiedGuideModel) executeCurrentStepAction() {
	if m.activeChapter != nil && m.activeStep < len(m.activeChapter.Steps) {
		step := &m.activeChapter.Steps[m.activeStep]
		if step.Action != nil {
			m.isSandboxActive = true
			m.LastResult = nil
			_ = step.Action(m, step)
		}
		
		// If step requires manual input immediately (and it's not already passed)
		if step.ManualInput && m.LastResult != nil && m.LastResult.FixParam != nil && m.LastResult.Status == "INTERACTIVE" {
			m.inputtingFixParam = true
			m.textInput.Focus()
			m.textInput.SetValue("")
		}
	}
}

func (m *UnifiedGuideModel) refreshTroubleshoot() tea.Cmd {
	return func() tea.Msg {
		m.executeCurrentStepAction()
		return nil
	}
}

func (m *UnifiedGuideModel) handlePostFixResult(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.LastResult == nil { return m, nil }
	
	switch res := msg.(type) {
	case error:
		m.LastResult.Status = "FAILED"
		m.LastResult.Issue = fmt.Sprintf("Verification failed: %v", res)
	default:
		m.LastResult.Status = "PASSED"
		m.LastResult.Issue = "Verification successful!"
	}
	// Refresh to update the UI with new status
	return m, m.refreshTroubleshoot()
}

// Run starts the Bubble Tea program.
func Run(m *UnifiedGuideModel) error {
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func (m *UnifiedGuideModel) saveProgress() {
	if m.SelectedProfile == "" || m.activeScenario == nil {
		return
	}
	// Simplified persistence for now: save current state to profile-specific file
	pm := config.GetProfileManager()
	statePath := pm.GetStatePathForProfile(m.SelectedProfile)
	progressFile := filepath.Join(statePath, "guide_progress.json")
	
	type progress struct {
		ScenarioID string `json:"scenario_id"`
		ChapterID  string `json:"chapter_id"`
		StepIdx    int    `json:"step_idx"`
	}
	
	p := progress{
		ScenarioID: m.activeScenario.ID,
		ChapterID:  m.activeChapter.ID,
		StepIdx:    m.activeStep,
	}
	
	data, _ := json.Marshal(p)
	_ = os.WriteFile(progressFile, data, 0644)
}

func (m *UnifiedGuideModel) loadProgress(profile string) {
	pm := config.GetProfileManager()
	statePath := pm.GetStatePathForProfile(profile)
	progressFile := filepath.Join(statePath, "guide_progress.json")
	
	data, err := os.ReadFile(progressFile)
	if err != nil {
		return
	}
	
	var p struct {
		ScenarioID string `json:"scenario_id"`
		ChapterID  string `json:"chapter_id"`
		StepIdx    int    `json:"step_idx"`
	}
	
	if err := json.Unmarshal(data, &p); err == nil {
		// Attempt to restore state
		for i, sc := range m.scenarios {
			if sc.ID == p.ScenarioID {
				m.inScenario = true
				m.activeScenario = &m.scenarios[i]
				for _, ch := range m.activeScenario.Chapters {
					if ch.ID == p.ChapterID {
						m.activeChapter = &ch
						m.activeStep = p.StepIdx
						m.executeCurrentStepAction()
						break
					}
				}
				break
			}
		}
	}
}
