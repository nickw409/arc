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
	planMeta    planSummary
	activePhase string
	lastUpdate  time.Time
	width       int
	height      int
	quitting    bool
	err         error

	selectedIdx  int
	showDetail   bool
	detailScroll int
}

// planSummary holds aggregate stats computed during refresh.
type planSummary struct {
	WorkflowType      string
	TotalTokens       int
	TotalIterations   int
	TotalTests        int
	TotalTestsPassing int
	PhasesComplete    int
	PhasesTotal       int
	PhasesActive      int
	InterventionCount int
	BlockedCount      int
	StuckCount        int
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

	CurrentState        string
	WorkflowType        string
	GlobalIterations    int
	StuckIterations     int
	RollbackCount       int
	HangCount           int
	InputTokens         int
	OutputTokens        int
	LastCommit          string
	ModelOverride       string
	CompletedAt         string
	BlockedReason       string
	DeferredReason      string
	Notes               string
	ExecutedEscalations []string

	ChunksTotal  int
	ChunksDone   int
	ChunkCurrent string

	HasIntervention     bool
	InterventionReason  string
	InterventionOptions []string

	VerdictHistory []VerdictRow

	HasParallel      bool
	ParallelBranches map[string]string
	ParallelVerdict  string

	DisputeDetails []DisputeRow
}

// VerdictRow is a single entry in the verdict history for display.
type VerdictRow struct {
	Iteration int
	State     string
	Verdict   string
	Timestamp string
}

// DisputeRow is a single dispute for display.
type DisputeRow struct {
	TestName string
	Reason   string
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
		return m.handleKey(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tickMsg:
		return m, m.refresh

	case refreshMsg:
		m.phases = msg.phases
		m.planMeta = msg.meta
		m.lastUpdate = time.Now()
		if m.selectedIdx >= len(m.phases) && len(m.phases) > 0 {
			m.selectedIdx = len(m.phases) - 1
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
	switch key {
	case "q", "esc":
		m.quitting = true
		return m, tea.Quit
	case "down":
		if len(m.phases) > 0 && m.selectedIdx < len(m.phases)-1 {
			m.selectedIdx++
		}
	case "up":
		if m.selectedIdx > 0 {
			m.selectedIdx--
		}
	case "enter", " ":
		if len(m.phases) > 0 {
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

	if m.showDetail && m.selectedIdx < len(m.phases) {
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

		CurrentState:        state.CurrentState,
		WorkflowType:        state.WorkflowType,
		GlobalIterations:    state.GlobalIterations,
		StuckIterations:     state.StuckIterations,
		RollbackCount:       state.RollbackCount,
		HangCount:           state.HangCount,
		InputTokens:         state.Usage.InputTokens,
		OutputTokens:        state.Usage.OutputTokens,
		ModelOverride:       state.ModelOverride,
		BlockedReason:       state.BlockedReason,
		DeferredReason:      state.DeferredReason,
		Notes:               state.Notes,
		ExecutedEscalations: state.ExecutedEscalations,
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

	// Chunks
	pv.ChunksTotal = state.Chunks.Total
	pv.ChunksDone = len(state.Chunks.Completed)
	if state.Chunks.Current != nil {
		pv.ChunkCurrent = state.Chunks.Current.Name
	}

	// Intervention
	if state.InterventionRequest != nil {
		pv.HasIntervention = true
		pv.InterventionReason = state.InterventionRequest.Reason
		pv.InterventionOptions = state.InterventionRequest.Options
	}

	// Verdict history: last 10, most recent first
	if len(state.VerdictsHistory) > 0 {
		start := 0
		if len(state.VerdictsHistory) > 10 {
			start = len(state.VerdictsHistory) - 10
		}
		for i := len(state.VerdictsHistory) - 1; i >= start; i-- {
			ve := state.VerdictsHistory[i]
			ts := ve.Timestamp
			if t, err := time.Parse(time.RFC3339, ve.Timestamp); err == nil {
				ts = t.Format("15:04")
			}
			pv.VerdictHistory = append(pv.VerdictHistory, VerdictRow{
				Iteration: ve.Iteration,
				State:     ve.State,
				Verdict:   ve.Verdict,
				Timestamp: ts,
			})
		}
	}

	// Parallel execution
	if state.ParallelExecution != nil {
		pv.HasParallel = true
		pv.ParallelBranches = make(map[string]string, len(state.ParallelExecution.Branches))
		for name, bs := range state.ParallelExecution.Branches {
			pv.ParallelBranches[name] = bs.Status
		}
		pv.ParallelVerdict = state.ParallelExecution.Verdict
	}

	// Dispute details
	for _, d := range state.Disputes {
		pv.DisputeDetails = append(pv.DisputeDetails, DisputeRow{
			TestName: d.TestName,
			Reason:   d.Reason,
		})
	}

	return pv
}

// planSummaryFromViews computes aggregate stats from phase views.
func planSummaryFromViews(views []PhaseView, workflowType string) planSummary {
	s := planSummary{
		WorkflowType: workflowType,
		PhasesTotal:  len(views),
	}
	for _, pv := range views {
		s.TotalTokens += pv.InputTokens + pv.OutputTokens
		s.TotalIterations += pv.GlobalIterations
		s.TotalTests += pv.TestsTotal
		s.TotalTestsPassing += pv.TestsPassing
		if pv.Status == "complete" {
			s.PhasesComplete++
		}
		if isActiveStatus(pv.Status) {
			s.PhasesActive++
		}
		if pv.HasIntervention {
			s.InterventionCount++
		}
		if pv.Status == "blocked" {
			s.BlockedCount++
		}
		if pv.StuckIterations > 0 {
			s.StuckCount++
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
