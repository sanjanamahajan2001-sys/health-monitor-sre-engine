package tui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Align(lipgloss.Center).
			Foreground(lipgloss.Color("6")).
			Width(80).
			MarginTop(1)

	disclaimerStyle = lipgloss.NewStyle().
			Italic(true).
			Foreground(lipgloss.Color("8")).
			Align(lipgloss.Center).
			Width(80)

	legendStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			Align(lipgloss.Center).
			Width(80)

	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("8")).
			Padding(1, 2)

	keyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("7"))

	valueStyle = lipgloss.NewStyle().
			Bold(true)

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))

	green  = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)
	yellow = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true)
	red    = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)

	bannerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			Align(lipgloss.Center).
			Width(80)

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			Italic(true).
			Align(lipgloss.Center).
			Width(80)

	cleanupMessageStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("2")).
				Bold(true)

	// Additional common styles often used
	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("9")).
			Bold(true)

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")).
			Bold(true)

	warningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("11")).
			Bold(true)

	infoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("12")).
			Bold(true)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Italic(true)

	keywordStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("205")).
			Bold(true)

	subtleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("1")).
			Bold(true)

	docStyle = lipgloss.NewStyle().Padding(1, 2, 1, 2)

	cardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(1, 2).
			Margin(0, 1)

	incidentBoxStyle = lipgloss.NewStyle().
				Border(lipgloss.DoubleBorder()).
				BorderForeground(lipgloss.Color("197")).
				Padding(1, 1).
				Margin(1, 0).
				Width(78)
)
