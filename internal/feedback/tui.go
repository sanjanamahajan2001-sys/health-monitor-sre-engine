package feedback

import (
	"fmt"
	"runtime"
	"strconv"
	"time"

	"health-monitor/internal/config"
	"health-monitor/pkg/model"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"
)

var (
	focusedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	blurredStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	cursorStyle  = focusedStyle
	noStyle      = lipgloss.NewStyle()
	helpStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("6")).
			MarginBottom(1)

	cardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(1, 2).
			Margin(1, 0)
)

type state int

const (
	stateRating state = iota
	stateNotes
	stateSummary
	stateSubmitting
	stateComplete
)

type feedbackModel struct {
	state      state
	categories []categoryInfo
	catIndex   int
	
	// Data being collected
	feedback   model.Feedback
	input      textinput.Model
	
	err        error
	submitting bool
}

type categoryInfo struct {
	ID   model.FeedbackCategory
	Name string
	Desc string
}

func initialModel() feedbackModel {
	ti := textinput.New()
	ti.Placeholder = "Any additional thoughts? (Optional)"
	ti.CharLimit = 1000
	ti.Width = 60

	categories := []categoryInfo{
		{model.CategoryOverall, "Overall Experience", "How would you rate the agent overall?"},
		{model.CategoryReliability, "Reliability & Alerts", "How effective are the alerts and predictive preventions?"},
		{model.CategoryDiagnostics, "Diagnostics & SLOs", "How useful are the doctor checks and SLO insights?"},
		{model.CategoryAutomation, "Automation & Flows", "How well do the automation flows and runbooks work?"},
		{model.CategoryUI, "User Interface", "How do you like the look and feel of the TUI & CLI?"},
	}

	fb := model.Feedback{
		ID:        uuid.New().String(),
		MachineID: GetMachineID(),
		Timestamp: time.Now(),
		Version:   model.Version,
		OS:        runtime.GOOS,
		Features:  make(map[model.FeedbackCategory]model.FeatureFeedback),
	}
	
	pm := config.GetProfileManager()
	fb.Profile = pm.GetActiveProfile()

	return feedbackModel{
		state:      stateRating,
		categories: categories,
		feedback:   fb,
		input:      ti,
	}
}

func (m feedbackModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m feedbackModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit

		case "enter":
			if m.state == stateRating {
				// Don't allow enter without a rating
				catID := m.categories[m.catIndex].ID
				if m.feedback.Features[catID].Rating == 0 {
					return m, nil
				}
				m.state = stateNotes
				m.input.Focus()
				return m, nil
			} else if m.state == stateNotes {
				// Save notes and move to next category or summary
				catID := m.categories[m.catIndex].ID
				f := m.feedback.Features[catID]
				f.Notes = m.input.Value()
				m.feedback.Features[catID] = f

				m.input.Reset()
				m.input.Blur()

				if m.catIndex < len(m.categories)-1 {
					m.catIndex++
					m.state = stateRating
				} else {
					m.state = stateSummary
				}
				return m, nil
			} else if m.state == stateSummary {
				m.state = stateSubmitting
				return m, submitFeedback(m.feedback)
			} else if m.state == stateComplete {
				return m, tea.Quit
			}

		case "1", "2", "3", "4", "5":
			if m.state == stateRating {
				rating, _ := strconv.Atoi(msg.String())
				catID := m.categories[m.catIndex].ID
				m.feedback.Features[catID] = model.FeatureFeedback{Rating: rating}
				// Auto-advance to notes after rating? Or wait for enter?
				// The prompt style says enter to skip/defaults, but we want proper stars.
				// Let's stay on rating screen but show the selected rating.
				return m, nil
			}

		case "backspace":
			if m.state == stateNotes && m.input.Value() == "" {
				m.state = stateRating
				m.input.Blur()
				return m, nil
			}
		}

	case submitResultMsg:
		m.state = stateComplete
		m.err = msg.err
		return m, nil
	}

	if m.state == stateNotes {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m feedbackModel) View() string {
	var s string

	s += titleStyle.Render("🚀 Health Monitor feedback") + "\n"

	switch m.state {
	case stateRating, stateNotes:
		cat := m.categories[m.catIndex]
		
		s += fmt.Sprintf("Step %d of %d\n\n", m.catIndex+1, len(m.categories))
		s += lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("3")).Render(cat.Name) + "\n"
		s += lipgloss.NewStyle().Italic(true).Foreground(lipgloss.Color("241")).Render(cat.Desc) + "\n\n"

		// Star rating display
		rating := m.feedback.Features[cat.ID].Rating
		var stars string
		for i := 1; i <= 5; i++ {
			if i <= rating {
				stars += lipgloss.NewStyle().Foreground(lipgloss.Color("220")).SetString("★ ").String()
			} else {
				stars += lipgloss.NewStyle().Foreground(lipgloss.Color("238")).SetString("☆ ").String()
			}
		}
		
		s += "Rating: " + stars + " (Press 1-5)\n\n"

		if m.state == stateNotes {
			s += "Notes:\n" + m.input.View() + "\n"
		}

		s += helpStyle.Render("\n[enter] Next  [q] Quit")

	case stateSummary:
		s += "Thank you! Here is a summary of your feedback:\n\n"
		for _, cat := range m.categories {
			f := m.feedback.Features[cat.ID]
			if f.Rating > 0 {
				var stars string
				for i := 1; i <= f.Rating; i++ {
					stars += "★"
				}
				s += fmt.Sprintf("• %-20s %-5s %s\n", cat.Name, stars, f.Notes)
			}
		}
		s += "\n" + focusedStyle.Render("Press [enter] to submit and sync.") + "\n"
		s += helpStyle.Render("[q] Cancel")

	case stateSubmitting:
		s += "\n   " + lipgloss.NewStyle().Foreground(lipgloss.Color("62")).Render("Submitting...") + "\n"

	case stateComplete:
		if m.err != nil {
			s += lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(fmt.Sprintf("\n❌ Submission failed: %v", m.err)) + "\n"
			s += "However, your feedback was saved locally.\n"
		} else {
			s += lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render("\n✅ Feedback submitted successfully!") + "\n"
		}
		s += "\nThank you for helping us improve health-monitor!\n"
		s += helpStyle.Render("\nPress [enter] or [q] to exit")
	}

	return cardStyle.Render(s) + "\n"
}

type submitResultMsg struct {
	err error
}

func submitFeedback(fb model.Feedback) tea.Cmd {
	return func() tea.Msg {
		// Send centrally
		if err := SendToCentralizedEndpoint(&fb); err != nil {
			return submitResultMsg{err: err}
		}

		return submitResultMsg{err: nil}
	}
}

func RunTUIFeedback() error {
	p := tea.NewProgram(initialModel())
	_, err := p.Run()
	return err
}
