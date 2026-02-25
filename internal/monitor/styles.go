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

	selectedStyle      = lipgloss.NewStyle().Background(lipgloss.AdaptiveColor{Light: "254", Dark: "237"})
	alertStyle         = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "124", Dark: "196"}).Bold(true)
	dimStyle           = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "245", Dark: "240"})
	tokenStyle         = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "33", Dark: "39"})
	verdictOKStyle     = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "22", Dark: "46"})
	verdictWarnStyle   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "136", Dark: "226"})
	verdictBadStyle    = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "124", Dark: "196"})
	sectionHeaderStyle = lipgloss.NewStyle().Bold(true).Underline(true)
)

// StatusColor returns the appropriate lipgloss.AdaptiveColor for a status.
func StatusColor(status string) lipgloss.AdaptiveColor {
	switch status {
	case "complete":
		return lipgloss.AdaptiveColor{Light: "22", Dark: "46"}
	case "blocked":
		return lipgloss.AdaptiveColor{Light: "124", Dark: "196"}
	case "disputed":
		return lipgloss.AdaptiveColor{Light: "136", Dark: "226"}
	case "pending", "":
		return lipgloss.AdaptiveColor{Light: "240", Dark: "245"}
	default:
		return lipgloss.AdaptiveColor{Light: "28", Dark: "34"}
	}
}

// VerdictStyle returns the appropriate lipgloss style for a verdict string.
func VerdictStyle(verdict string) lipgloss.Style {
	switch verdict {
	case "approved", "no_bugs_found", "coverage_sufficient", "consistent", "executable":
		return verdictOKStyle
	case "concerns", "ambiguous", "coverage_gaps", "gaps_found":
		return verdictWarnStyle
	case "rejected", "bugs_found", "inconsistent", "blocked", "scope_too_large":
		return verdictBadStyle
	default:
		return dimStyle
	}
}
