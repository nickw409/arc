package monitor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/nwiley/arc/internal/arc"
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

// refresh reads all phase state.json files from disk and returns a refreshMsg.
func (m Model) refresh() tea.Msg {
	planDir := filepath.Join(m.plansDir, m.planFilter)

	// Read plan.json to get phase order
	planJSON, err := os.ReadFile(filepath.Join(planDir, "plan.json"))
	if err != nil {
		return refreshMsg{plans: m.plans}
	}

	var meta arc.PlanMeta
	if err := json.Unmarshal(planJSON, &meta); err != nil {
		return refreshMsg{plans: m.plans}
	}

	var views []PhaseView
	for _, phase := range meta.Phases {
		statePath := filepath.Join(planDir, "phases", phase, "state.json")
		data, err := os.ReadFile(statePath)
		if err != nil {
			views = append(views, PhaseView{Name: phase, Status: "unknown", Icon: "[?]"})
			continue
		}

		var ps arc.PhaseState
		if err := json.Unmarshal(data, &ps); err != nil {
			views = append(views, PhaseView{Name: phase, Status: "error", Icon: "[?]"})
			continue
		}

		views = append(views, PhaseViewFromState(&ps))
	}

	summary := planSummaryFromViews(views, meta.WorkflowType)

	plan := PlanView{
		Name:         meta.Name,
		WorkflowType: meta.WorkflowType,
		Phases:       views,
		Meta:         summary,
	}

	return refreshMsg{plans: []PlanView{plan}}
}
