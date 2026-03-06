package monitor

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/nwiley/arc/internal/arc"
)

// Model is the bubbletea model for the monitor TUI.
type Model struct {
	plansDir   string
	planFilter string
	plans      []PlanView
	lastUpdate time.Time
	width      int
	height     int
	quitting   bool

	selectedIdx  int
	showDetail   bool
	detailScroll int
}

// planSummary holds aggregate stats computed during refresh.
type planSummary struct {
	CompletedCount  int
	RunningCount    int
	PendingCount    int
	FailedCount     int
	TotalIterations int
}

// PlanView holds the display state for a single plan.
type PlanView struct {
	Name         string
	WorkflowType string
	Phases       []PhaseView
	Meta         planSummary
}

// PhaseView is the display state for a single phase.
type PhaseView struct {
	PlanName          string
	Name              string
	Status            string
	Icon              string
	Iteration         int
	MaxIteration      int
	TestsPassing      int
	TestsTotal        int
	InputTokens       int
	OutputTokens      int
	LastCommit        string
	CompletedAt       string
	BlockedReason     string
	DeferredReason    string
	Notes             string
	Activity          string
	ActivityUpdatedAt string
	AdversaryRound    int
}

// NewModel creates a monitor model.
func NewModel(planFilter, plansDir string) Model {
	return Model{
		planFilter: planFilter,
		plansDir:   plansDir,
		plans:      []PlanView{},
	}
}

// firstPlan returns the first PlanView or a zero value if none.
func (m Model) firstPlan() PlanView {
	if len(m.plans) > 0 {
		return m.plans[0]
	}
	return PlanView{}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(tick(), m.refresh)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tickMsg:
		return m, m.refresh

	case refreshMsg:
		m.plans = msg.plans
		m.lastUpdate = time.Now()
		phases := m.firstPlan().Phases
		if m.selectedIdx >= len(phases) && len(phases) > 0 {
			m.selectedIdx = len(phases) - 1
		}
		return m, tick()
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// ctrl+c always quits
	if key == "ctrl+c" {
		m.quitting = true
		return m, tea.Quit
	}

	if m.showDetail {
		return m.handleDetailKey(key)
	}
	return m.handleOverviewKey(key)
}

func (m Model) handleOverviewKey(key string) (tea.Model, tea.Cmd) {
	phases := m.firstPlan().Phases
	switch key {
	case "q", "esc":
		m.quitting = true
		return m, tea.Quit
	case "down":
		if len(phases) > 0 && m.selectedIdx < len(phases)-1 {
			m.selectedIdx++
		}
	case "up":
		if m.selectedIdx > 0 {
			m.selectedIdx--
		}
	case "enter", " ":
		if len(phases) > 0 {
			m.showDetail = true
			m.detailScroll = 0
		}
	case "r":
		return m, m.refresh
	}
	return m, nil
}

func (m Model) handleDetailKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "esc":
		m.showDetail = false
	case "down":
		m.detailScroll++
	case "up":
		if m.detailScroll > 0 {
			m.detailScroll--
		}
	case "r":
		return m, m.refresh
	}
	return m, nil
}

func (m Model) View() string {
	if m.quitting {
		return ""
	}

	if m.showDetail && m.selectedIdx < len(m.firstPlan().Phases) {
		return m.renderDetailPanel()
	}

	var s string
	s += m.renderHeader()
	s += "\n"
	alerts := m.renderInterventionAlerts()
	if alerts != "" {
		s += alerts
	}
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

	pv := PhaseView{
		Name:              state.Phase,
		Status:            state.PhaseStatus,
		Icon:              icon,
		Iteration:         state.Iteration.Current,
		MaxIteration:      state.Iteration.Max,
		TestsPassing:      state.TestsPassing,
		TestsTotal:        state.TestsTotal,
		AdversaryRound:    state.AdversaryRound,
		InputTokens:       state.Usage.InputTokens,
		OutputTokens:      state.Usage.OutputTokens,
		BlockedReason:     state.BlockedReason,
		DeferredReason:    state.DeferredReason,
		Notes:             state.Notes,
		Activity:          state.Activity,
		ActivityUpdatedAt: state.ActivityUpdatedAt,
	}

	// Last commit: truncate to 7 chars
	if len(state.LastCommit) >= 7 {
		pv.LastCommit = state.LastCommit[:7]
	} else {
		pv.LastCommit = state.LastCommit
	}

	// Completed at: format for display
	if state.CompletedAt != "" {
		if t, err := time.Parse(time.RFC3339, state.CompletedAt); err == nil {
			pv.CompletedAt = t.Format("15:04")
		} else {
			pv.CompletedAt = state.CompletedAt
		}
	}

	// Blocked reason from legacy field
	if pv.BlockedReason == "" && state.Blocked.Reason != nil {
		pv.BlockedReason = *state.Blocked.Reason
	}

	return pv
}

// planSummaryFromViews computes aggregate stats from phase views.
func planSummaryFromViews(views []PhaseView, workflowType string) planSummary {
	s := planSummary{}
	for _, v := range views {
		s.TotalIterations += v.Iteration
		switch v.Status {
		case "complete":
			s.CompletedCount++
		case "blocked":
			s.FailedCount++
		case "pending", "":
			s.PendingCount++
		default:
			if isActiveStatus(v.Status) {
				s.RunningCount++
			}
		}
	}
	return s
}

func statusIcon(status string) string {
	switch status {
	case "pending", "":
		return "[ ]"
	case "complete":
		return "[x]"
	case "blocked":
		return "[X]"
	case "adversary":
		return "[!]"
	case "disputed":
		return "[!]"
	case "deferred":
		return "[~]"
	case "split":
		return "[/]"
	default:
		return "[>]"
	}
}

// isActiveStatus reports whether a status represents a phase that is currently
// running (i.e. not pending, terminal, or otherwise inactive).
func isActiveStatus(status string) bool {
	switch status {
	case "", "pending", "complete", "blocked", "deferred", "split", "disputed":
		return false
	default:
		return true
	}
}
