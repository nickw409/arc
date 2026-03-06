package monitor

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type tickMsg struct{}
type refreshMsg struct {
	plans []PlanView
}

// tick returns a command that sends a tickMsg after 3 seconds.
func tick() tea.Cmd {
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

