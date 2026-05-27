package guide

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	maxWindowWidth = 85
)

var (
	// Standard Colors
	primaryColor   = lipgloss.Color("5") // Purple
	secondaryColor = lipgloss.Color("6") // Cyan
	accentColor    = lipgloss.Color("2") // Green
	warningColor   = lipgloss.Color("3") // Yellow
	errorColor     = lipgloss.Color("1") // Red
	mutedColor     = lipgloss.Color("8") // Gray
	sandboxColor   = lipgloss.Color("208") // Orange

	// Styles
	windowStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(primaryColor).
			Padding(1, 2).
			Margin(1, 2).
			Width(maxWindowWidth)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor).
			Underline(true).
			MarginBottom(1)

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(secondaryColor).
			MarginBottom(1)

	bodyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("7"))

	footerStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			MarginTop(1)

	searchStyle = lipgloss.NewStyle().
			Foreground(secondaryColor).
			Italic(true)

	hintTitleStyle = lipgloss.NewStyle().
			Foreground(warningColor).
			Bold(true).
			PaddingLeft(1).
			PaddingRight(1).
			BorderStyle(lipgloss.NormalBorder()).
			BorderLeft(true)

	hintBodyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			Italic(true).
			PaddingLeft(2)

	roadmapStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			Faint(true)
	
	activeChapterStyle = lipgloss.NewStyle().
			Foreground(secondaryColor).
			Bold(true)

	sandboxBannerStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("0")).
				Background(sandboxColor).
				Padding(0, 1).
				MarginBottom(1)

	highlightStyle = lipgloss.NewStyle().
			Foreground(accentColor).
			Bold(true)

	remediationStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(warningColor).
				Padding(0, 1).
				Margin(1, 0).
				Width(maxWindowWidth - 6)

	commandStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			Italic(true).
			Width(maxWindowWidth - 10)

	inputPromptStyle = lipgloss.NewStyle().
				Foreground(secondaryColor).
				Bold(true).
				MarginBottom(1)
)

// View renders the TUI.
func (m *UnifiedGuideModel) View() string {
	if m.quitting {
		return "Thanks for using the Guided Tour! See you soon.\n"
	}

	var s strings.Builder

	// Header
	s.WriteString(headerStyle.Render("🚀 health-monitor Guided Tour"))
	s.WriteString("\n")

	// Sandbox Banner
	if m.isSandboxActive {
		s.WriteString(sandboxBannerStyle.Render("⚠️  SANDBOX MODE ACTIVE: Non-destructive environment"))
		s.WriteString("\n")
	} else {
		s.WriteString("\n")
	}

	if m.searching {
		s.WriteString(searchStyle.Render(fmt.Sprintf("🔍 Find feature or troubleshooting topic: %s█", m.searchInput)))
		s.WriteString("\n\n")
		return windowStyle.Render(s.String())
	}

	if m.inputtingFixParam {
		prompt := "Enter Parameter:"
		if m.LastResult != nil && m.LastResult.Prompt != "" {
			prompt = m.LastResult.Prompt
		}
		s.WriteString(titleStyle.Render("🛠️  Interactive Configuration"))
		s.WriteString("\n")
		s.WriteString(inputPromptStyle.Render(prompt))
		s.WriteString("\n")
		s.WriteString(m.textInput.View())
		s.WriteString("\n\n")
		s.WriteString(footerStyle.Render("Enter: Save • Esc: Cancel"))
		return windowStyle.Render(s.String())
	}

	if m.selectingProfile {
		s.WriteString(titleStyle.Render("Select Target Profile for Diagnostics"))
		s.WriteString("\n")
		s.WriteString(bodyStyle.Render("Which environment are you troubleshooting?"))
		s.WriteString("\n\n")

		for i, p := range m.availableProfiles {
			prefix := "  "
			style := bodyStyle
			if i == m.profileIdx {
				prefix = highlightStyle.Render("▶ ")
				style = lipgloss.NewStyle().Bold(true).Foreground(secondaryColor)
			}
			s.WriteString(fmt.Sprintf("%s %s\n", prefix, style.Render(p)))
		}

		s.WriteString("\n")
		s.WriteString(footerStyle.Render("↑/↓: Navigate • Enter: Select • b: Back to Menu"))
		return windowStyle.Render(s.String())
	}

	if m.confirmingFix {
		s.WriteString(titleStyle.Render("⚠️  Confirm Remediation"))
		s.WriteString("\n")
		s.WriteString(bodyStyle.Render("The guide is ready to automatically apply this fix:"))
		s.WriteString("\n\n")
		s.WriteString(remediationStyle.Render(m.LastResult.Fix))
		s.WriteString("\n\n")
		s.WriteString(bodyStyle.Render("Proceed with automated fix?"))
		s.WriteString("\n")
		s.WriteString(footerStyle.Render("Enter: YES (Apply) • Esc/n: NO (Cancel)"))
		return windowStyle.Render(s.String())
	}

	if !m.inScenario {
		if m.searchMode {
			s.WriteString(searchStyle.Render(fmt.Sprintf("🔍 Showing results for: %s (Esc to clear)", m.searchInput)))
			s.WriteString("\n\n")
		}

		// Scenario Menu
		scenarios := m.scenarios
		title := "Choose Your Scenario"
		if m.searchMode {
			scenarios = m.searchResults
			title = "Search Results"
		}

		s.WriteString(titleStyle.Render(title))
		s.WriteString("\n")
		if len(scenarios) == 0 {
			s.WriteString(bodyStyle.Render("No features matched your search. Try another keyword!"))
			s.WriteString("\n")
		} else {
			s.WriteString(bodyStyle.Render("Select a journey or type '/' to search..."))
			s.WriteString("\n\n")

			for i, sc := range scenarios {
				prefix := "  "
				style := bodyStyle
				if i == m.currentIdx {
					prefix = highlightStyle.Render("▶ ")
					style = lipgloss.NewStyle().Bold(true).Foreground(secondaryColor)
				}
				s.WriteString(fmt.Sprintf("%s %s\n", prefix, style.Render(sc.Name)))
				if i == m.currentIdx {
					descText := lipgloss.NewStyle().Foreground(mutedColor).Width(maxWindowWidth - 10).Render(sc.Description)
					s.WriteString(fmt.Sprintf("    %s\n", descText))
				}
			}
		}

		s.WriteString("\n")
		footer := "↑/↓: Navigate • Enter: Select • /: Search • q: Quit"
		if m.searchMode {
			footer = "↑/↓: Navigate • Enter: Select • Esc: Clear Search • q: Quit"
		}
		s.WriteString(footerStyle.Render(footer))
	} else {
		// Active Scenario View
		titleStr := m.activeScenario.Name
		if m.SelectedProfile != "" {
			titleStr = fmt.Sprintf("%s [%s]", titleStr, m.SelectedProfile)
		}
		s.WriteString(titleStyle.Render(titleStr))
		s.WriteString("\n")
		
		// Chapter Roadmap
		var roadmap []string
		for _, ch := range m.activeScenario.Chapters {
			if ch.ID == m.activeChapter.ID {
				roadmap = append(roadmap, activeChapterStyle.Render("▶ "+ch.Title))
			} else {
				roadmap = append(roadmap, roadmapStyle.Render("  "+ch.Title))
			}
		}
		s.WriteString(strings.Join(roadmap, "  ") + "\n")
		
		s.WriteString(strings.Repeat("─", maxWindowWidth-6))
		s.WriteString("\n\n")

		step := m.activeChapter.Steps[m.activeStep]
		s.WriteString(lipgloss.NewStyle().Bold(true).Foreground(accentColor).Render(step.Title))
		s.WriteString("\n\n")
		
		// If we have a TroubleshootResult, render it specially
		if m.LastResult != nil {
			statusColor := accentColor
			if m.LastResult.Status == "FAILED" {
				statusColor = errorColor
			} else if m.LastResult.Status == "WARNING" {
				statusColor = warningColor
			}
			
			s.WriteString(lipgloss.NewStyle().Foreground(statusColor).Bold(true).Render("Result: " + m.LastResult.Status))
			s.WriteString("\n")
			
			// Wrap Issue description
			issText := lipgloss.NewStyle().Width(maxWindowWidth - 10).Render("Issue: " + m.LastResult.Issue)
			s.WriteString(bodyStyle.Render(issText))
			s.WriteString("\n\n")
			
			// Wrap Fix advice
			fixAdvice := lipgloss.NewStyle().Width(maxWindowWidth - 10).Foreground(accentColor).Render("💡 Fix: " + m.LastResult.Fix)
			s.WriteString(fixAdvice)
			s.WriteString("\n")
			
			if m.LastResult.FixCommand != "" {
				s.WriteString("\n")
				s.WriteString(commandStyle.Render("Command: " + m.LastResult.FixCommand))
				s.WriteString("\n")
			}
			
			if m.LastResult.FixAction != nil || m.LastResult.FixParam != nil {
				s.WriteString("\n")
				s.WriteString(highlightStyle.Render("➜ Press [F] to automatically apply this fix"))
				s.WriteString("\n")
			}
		} else {
			// Wrap generic content
			wrappedContent := lipgloss.NewStyle().Width(maxWindowWidth - 10).Render(step.Content)
			s.WriteString(bodyStyle.Render(wrappedContent))
		}
		s.WriteString("\n\n")

		// Hint Panel
		if m.showHint && step.Hint != "" {
			s.WriteString(hintTitleStyle.Render("TECHNICAL DEEP-DIVE"))
			s.WriteString("\n")
			s.WriteString(hintBodyStyle.Width(maxWindowWidth - 10).Render(step.Hint))
			s.WriteString("\n\n")
		}

		// Progress indicator
		progress := fmt.Sprintf("Step %d of %d", m.activeStep+1, len(m.activeChapter.Steps))
		s.WriteString(lipgloss.NewStyle().Foreground(mutedColor).Render(progress))
		s.WriteString("\n")

		s.WriteString("\n")
		footer := "Enter: Next • h: Hint • b: Back to Menu • /: Search • q: Quit"
		if m.LastResult != nil && (m.LastResult.FixAction != nil || m.LastResult.FixParam != nil) {
			footer = "Enter: Next • f: APPLY FIX • h: Hint • b: Back to Menu • /: Search • q: Quit"
		}
		s.WriteString(footerStyle.Render(footer))
	}

	return windowStyle.Render(s.String())
}
