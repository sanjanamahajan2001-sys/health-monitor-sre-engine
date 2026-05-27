package output

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

var (
	updateNotificationStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("2")).
		Background(lipgloss.Color("0")).
		Padding(1, 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("2")).
		Width(80)
)

// PrintUpdateNotification displays a notification that an update was just performed
func PrintUpdateNotification(version string) {
	message := fmt.Sprintf("✓ Successfully updated to version %s", version)
	fmt.Println()
	fmt.Println(updateNotificationStyle.Render(message))
	fmt.Println()
}

// PrintUpdateInstalledMessage displays a message that new version is installed and user should run again
func PrintUpdateInstalledMessage(version string) {
	message := fmt.Sprintf("✓ New version %s installed and downloaded. Please run the agent again for the new updates.", version)
	fmt.Println()
	fmt.Println(updateNotificationStyle.Render(message))
	fmt.Println()
}
