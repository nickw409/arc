package monitor

import (
	"fmt"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
)

// StartOptions configures the monitor TUI.
type StartOptions struct {
	PlanName string
	PlansDir string
}

// Start launches the bubbletea TUI monitor.
func Start(opts StartOptions) error {
	planDir := filepath.Join(opts.PlansDir, opts.PlanName)
	model := NewModel(opts.PlanName, planDir)

	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("monitor error: %w", err)
	}

	return nil
}
