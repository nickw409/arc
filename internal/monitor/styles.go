package monitor

import "github.com/charmbracelet/lipgloss"

var (
	headerStyle    = lipgloss.NewStyle().Bold(true).Padding(1, 0)
	activeStyle    = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "28", Dark: "34"})
	completedStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "22", Dark: "46"}).Bold(true)
	blockedStyle   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "124", Dark: "196"})
	disputedStyle  = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "136", Dark: "226"})
	pendingStyle   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "240", Dark: "245"})
	footerStyle    = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "240", Dark: "245"}).Padding(1, 0)
)

// StatusColor returns the appropriate lipgloss.AdaptiveColor for a status.
func StatusColor(status string) lipgloss.AdaptiveColor {
	switch status {
	case "implementing", "qa", "qa_review":
		return lipgloss.AdaptiveColor{Light: "28", Dark: "34"}
	case "complete":
		return lipgloss.AdaptiveColor{Light: "22", Dark: "46"}
	case "blocked":
		return lipgloss.AdaptiveColor{Light: "124", Dark: "196"}
	case "disputed":
		return lipgloss.AdaptiveColor{Light: "136", Dark: "226"}
	case "pending":
		return lipgloss.AdaptiveColor{Light: "240", Dark: "245"}
	default:
		return lipgloss.AdaptiveColor{Light: "15", Dark: "255"}
	}
}
