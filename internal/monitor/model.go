package monitor

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/nwiley/arc/internal/arc"
)

// Model is the bubbletea model for the monitor TUI.
type Model struct {
	planName    string
	planDir     string
	phases      []PhaseView
	activePhase string
	lastUpdate  time.Time
	width       int
	height      int
	quitting    bool
	err         error
}

// PhaseView is the display state for a single phase.
type PhaseView struct {
	Name           string
	Status         string
	Icon           string
	Iteration      int
	MaxIteration   int
	TestsPassing   int
	TestsTotal     int
	Disputes       int
	LastVerdict    string
	AdversaryRound int
}

// NewModel creates a monitor model.
func NewModel(planName, planDir string) Model {
	return Model{
		planName: planName,
		planDir:  planDir,
		phases:   []PhaseView{},
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(tick(), m.refresh)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			m.quitting = true
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tickMsg:
		return m, m.refresh

	case refreshMsg:
		m.phases = msg.phases
		m.lastUpdate = time.Now()
		return m, tick()
	}

	return m, nil
}

func (m Model) View() string {
	if m.quitting {
		return ""
	}

	var s string
	s += m.renderHeader()
	s += "\n"
	s += m.renderPhaseTable()
	s += "\n"
	s += m.renderFooter()
	return s
}

// PhaseViewFromState creates a PhaseView from a PhaseState.
func PhaseViewFromState(state *arc.PhaseState) PhaseView {
	if state == nil {
		return PhaseView{}
	}

	icon := statusIcon(state.PhaseStatus)

	return PhaseView{
		Name:           state.Phase,
		Status:         state.PhaseStatus,
		Icon:           icon,
		Iteration:      state.StateIterations[state.CurrentState],
		MaxIteration:   state.Iteration.Max,
		TestsPassing:   state.TestsPassing,
		TestsTotal:     state.TestsTotal,
		Disputes:       len(state.Disputes),
		LastVerdict:    state.LastVerdict,
		AdversaryRound: state.AdversaryRound,
	}
}

func statusIcon(status string) string {
	switch status {
	case "pending":
		return "[ ]"
	case "complete":
		return "[x]"
	case "implementing", "qa", "qa_review", "impl_review":
		return "[>]"
	case "adversary":
		return "[!]"
	case "disputed":
		return "[!]"
	case "blocked":
		return "[X]"
	case "deferred":
		return "[~]"
	case "split":
		return "[/]"
	default:
		return "[?]"
	}
}
