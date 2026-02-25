package monitor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/nwiley/arc/internal/arc"
)

// --- Model creation ---

func TestNewModel(t *testing.T) {
	m := NewModel("my-plan", "/tmp/plans/my-plan")
	if m.planName != "my-plan" {
		t.Errorf("planName=%q, want 'my-plan'", m.planName)
	}
	if m.phases == nil {
		t.Error("phases is nil, want empty slice")
	}
	if m.quitting {
		t.Error("quitting should be false")
	}
	if m.selectedIdx != 0 {
		t.Errorf("selectedIdx=%d, want 0", m.selectedIdx)
	}
	if m.showDetail {
		t.Error("showDetail should be false")
	}
}

func TestNewModelEmptyPlanName(t *testing.T) {
	m := NewModel("", "/tmp/plans/empty")
	if m.planName != "" {
		t.Errorf("planName=%q, want empty", m.planName)
	}
	_ = m.View()
}

// --- PhaseViewFromState ---

func TestPhaseViewFromState(t *testing.T) {
	ps := &arc.PhaseState{
		Phase:           "core",
		PhaseStatus:     "implementing",
		CurrentState:    "impl",
		Iteration:       arc.Iteration{Current: 5, Max: 25},
		StateIterations: map[string]int{"impl": 5},
		TestsPassing:    3,
		TestsTotal:      10,
		Disputes:        []arc.Dispute{{TestName: "t1", Reason: "r"}},
		LastVerdict:     "concerns",
	}

	pv := PhaseViewFromState(ps)
	if pv.Status != "implementing" {
		t.Errorf("Status=%q", pv.Status)
	}
	if pv.Icon != "[>]" {
		t.Errorf("Icon=%q, want '[>]'", pv.Icon)
	}
	if pv.Iteration != 5 {
		t.Errorf("Iteration=%d", pv.Iteration)
	}
	if pv.TestsPassing != 3 {
		t.Errorf("TestsPassing=%d", pv.TestsPassing)
	}
	if pv.TestsTotal != 10 {
		t.Errorf("TestsTotal=%d", pv.TestsTotal)
	}
	if pv.Disputes != 1 {
		t.Errorf("Disputes=%d", pv.Disputes)
	}
	if pv.LastVerdict != "concerns" {
		t.Errorf("LastVerdict=%q", pv.LastVerdict)
	}
	if pv.CurrentState != "impl" {
		t.Errorf("CurrentState=%q", pv.CurrentState)
	}
}

func TestPhaseViewNilState(t *testing.T) {
	pv := PhaseViewFromState(nil)
	if pv.Name != "" || pv.Status != "" {
		t.Error("expected zero-value PhaseView for nil state")
	}
}

func TestPhaseViewUnknownStatus(t *testing.T) {
	ps := &arc.PhaseState{PhaseStatus: "weird_status"}
	pv := PhaseViewFromState(ps)
	if pv.Icon != "[?]" {
		t.Errorf("Icon=%q, want '[?]'", pv.Icon)
	}
}

func TestPhaseViewFromStateAllStatuses(t *testing.T) {
	tests := []struct {
		status string
		icon   string
	}{
		{"pending", "[ ]"},
		{"complete", "[x]"},
		{"implementing", "[>]"},
		{"qa", "[>]"},
		{"qa_review", "[>]"},
		{"disputed", "[!]"},
		{"blocked", "[X]"},
		{"deferred", "[~]"},
		{"split", "[/]"},
	}
	for _, tc := range tests {
		pv := PhaseViewFromState(&arc.PhaseState{PhaseStatus: tc.status})
		if pv.Icon != tc.icon {
			t.Errorf("status %q: icon=%q, want %q", tc.status, pv.Icon, tc.icon)
		}
	}
}

func TestPhaseViewZeroIteration(t *testing.T) {
	ps := &arc.PhaseState{
		PhaseStatus: "implementing",
		Iteration:   arc.Iteration{Current: 0, Max: 0},
	}
	pv := PhaseViewFromState(ps)
	if pv.Iteration != 0 || pv.MaxIteration != 0 {
		t.Error("zero iteration not handled")
	}
}

func TestPhaseViewLastVerdictPopulated(t *testing.T) {
	ps := &arc.PhaseState{LastVerdict: "approved"}
	pv := PhaseViewFromState(ps)
	if pv.LastVerdict != "approved" {
		t.Errorf("LastVerdict=%q, want 'approved'", pv.LastVerdict)
	}
}

func TestPhaseViewFromStateUsage(t *testing.T) {
	ps := &arc.PhaseState{
		Usage: arc.Usage{
			InputTokens:  28000,
			OutputTokens: 4800,
		},
	}
	pv := PhaseViewFromState(ps)
	if pv.InputTokens != 28000 {
		t.Errorf("InputTokens=%d, want 28000", pv.InputTokens)
	}
	if pv.OutputTokens != 4800 {
		t.Errorf("OutputTokens=%d, want 4800", pv.OutputTokens)
	}
}

func TestPhaseViewFromStateChunks(t *testing.T) {
	ps := &arc.PhaseState{
		Chunks: arc.Chunks{
			Total:     5,
			Completed: []arc.ChunkResult{{ID: 1}, {ID: 2}, {ID: 3}},
			Current:   &arc.ChunkCurrent{ID: 4, Name: "write service tests", Status: "running"},
		},
	}
	pv := PhaseViewFromState(ps)
	if pv.ChunksTotal != 5 {
		t.Errorf("ChunksTotal=%d, want 5", pv.ChunksTotal)
	}
	if pv.ChunksDone != 3 {
		t.Errorf("ChunksDone=%d, want 3", pv.ChunksDone)
	}
	if pv.ChunkCurrent != "write service tests" {
		t.Errorf("ChunkCurrent=%q", pv.ChunkCurrent)
	}
}

func TestPhaseViewFromStateChunksNoCurrent(t *testing.T) {
	ps := &arc.PhaseState{
		Chunks: arc.Chunks{
			Total:     3,
			Completed: []arc.ChunkResult{{ID: 1}},
		},
	}
	pv := PhaseViewFromState(ps)
	if pv.ChunkCurrent != "" {
		t.Errorf("ChunkCurrent=%q, want empty", pv.ChunkCurrent)
	}
}

func TestPhaseViewFromStateIntervention(t *testing.T) {
	ps := &arc.PhaseState{
		InterventionRequest: &arc.Intervention{
			Reason:  "Tests keep regressing",
			Options: []string{"continue", "rollback"},
		},
	}
	pv := PhaseViewFromState(ps)
	if !pv.HasIntervention {
		t.Error("HasIntervention should be true")
	}
	if pv.InterventionReason != "Tests keep regressing" {
		t.Errorf("InterventionReason=%q", pv.InterventionReason)
	}
	if len(pv.InterventionOptions) != 2 {
		t.Errorf("InterventionOptions len=%d", len(pv.InterventionOptions))
	}
}

func TestPhaseViewFromStateNoIntervention(t *testing.T) {
	ps := &arc.PhaseState{}
	pv := PhaseViewFromState(ps)
	if pv.HasIntervention {
		t.Error("HasIntervention should be false")
	}
}

func TestPhaseViewFromStateVerdictHistory(t *testing.T) {
	ps := &arc.PhaseState{
		VerdictsHistory: []arc.VerdictEntry{
			{Iteration: 1, State: "impl", Verdict: "approved", Timestamp: "2025-01-01T10:00:00Z"},
			{Iteration: 2, State: "qa_review", Verdict: "concerns", Timestamp: "2025-01-01T10:30:00Z"},
			{Iteration: 3, State: "impl", Verdict: "approved", Timestamp: "2025-01-01T11:00:00Z"},
		},
	}
	pv := PhaseViewFromState(ps)
	if len(pv.VerdictHistory) != 3 {
		t.Fatalf("VerdictHistory len=%d, want 3", len(pv.VerdictHistory))
	}
	// Most recent first
	if pv.VerdictHistory[0].Iteration != 3 {
		t.Errorf("first entry iter=%d, want 3", pv.VerdictHistory[0].Iteration)
	}
	if pv.VerdictHistory[0].Timestamp != "11:00" {
		t.Errorf("first entry timestamp=%q, want '11:00'", pv.VerdictHistory[0].Timestamp)
	}
	if pv.VerdictHistory[2].Iteration != 1 {
		t.Errorf("last entry iter=%d, want 1", pv.VerdictHistory[2].Iteration)
	}
}

func TestPhaseViewFromStateVerdictHistoryCapped(t *testing.T) {
	var entries []arc.VerdictEntry
	for i := 0; i < 15; i++ {
		entries = append(entries, arc.VerdictEntry{Iteration: i + 1, State: "impl", Verdict: "approved", Timestamp: "2025-01-01T10:00:00Z"})
	}
	ps := &arc.PhaseState{VerdictsHistory: entries}
	pv := PhaseViewFromState(ps)
	if len(pv.VerdictHistory) != 10 {
		t.Errorf("VerdictHistory len=%d, want 10 (capped)", len(pv.VerdictHistory))
	}
	// Most recent first: should be iteration 15
	if pv.VerdictHistory[0].Iteration != 15 {
		t.Errorf("first entry iter=%d, want 15", pv.VerdictHistory[0].Iteration)
	}
}

func TestPhaseViewFromStateDisputeDetails(t *testing.T) {
	ps := &arc.PhaseState{
		Disputes: []arc.Dispute{
			{TestName: "TestAuth", Reason: "401 error"},
			{TestName: "TestToken", Reason: "expired"},
		},
	}
	pv := PhaseViewFromState(ps)
	if len(pv.DisputeDetails) != 2 {
		t.Fatalf("DisputeDetails len=%d, want 2", len(pv.DisputeDetails))
	}
	if pv.DisputeDetails[0].TestName != "TestAuth" {
		t.Errorf("first dispute=%q", pv.DisputeDetails[0].TestName)
	}
}

func TestPhaseViewFromStateLastCommitTruncated(t *testing.T) {
	ps := &arc.PhaseState{LastCommit: "abc1234567890"}
	pv := PhaseViewFromState(ps)
	if pv.LastCommit != "abc1234" {
		t.Errorf("LastCommit=%q, want 'abc1234'", pv.LastCommit)
	}
}

func TestPhaseViewFromStateLastCommitShort(t *testing.T) {
	ps := &arc.PhaseState{LastCommit: "abc"}
	pv := PhaseViewFromState(ps)
	if pv.LastCommit != "abc" {
		t.Errorf("LastCommit=%q, want 'abc'", pv.LastCommit)
	}
}

func TestPhaseViewFromStateParallelExecution(t *testing.T) {
	ps := &arc.PhaseState{
		ParallelExecution: &arc.ParallelExec{
			Branches: map[string]arc.BranchStatus{
				"branch-a": {Status: "complete"},
				"branch-b": {Status: "running"},
			},
			Verdict: "pending",
		},
	}
	pv := PhaseViewFromState(ps)
	if !pv.HasParallel {
		t.Error("HasParallel should be true")
	}
	if len(pv.ParallelBranches) != 2 {
		t.Errorf("ParallelBranches len=%d", len(pv.ParallelBranches))
	}
	if pv.ParallelBranches["branch-a"] != "complete" {
		t.Errorf("branch-a=%q", pv.ParallelBranches["branch-a"])
	}
	if pv.ParallelVerdict != "pending" {
		t.Errorf("ParallelVerdict=%q", pv.ParallelVerdict)
	}
}

func TestPhaseViewFromStateModelOverride(t *testing.T) {
	ps := &arc.PhaseState{ModelOverride: "sonnet"}
	pv := PhaseViewFromState(ps)
	if pv.ModelOverride != "sonnet" {
		t.Errorf("ModelOverride=%q", pv.ModelOverride)
	}
}

func TestPhaseViewFromStateBlockedReason(t *testing.T) {
	ps := &arc.PhaseState{BlockedReason: "dependency failed"}
	pv := PhaseViewFromState(ps)
	if pv.BlockedReason != "dependency failed" {
		t.Errorf("BlockedReason=%q", pv.BlockedReason)
	}
}

func TestPhaseViewFromStateBlockedReasonLegacy(t *testing.T) {
	reason := "legacy reason"
	ps := &arc.PhaseState{Blocked: arc.BlockedInfo{IsBlocked: true, Reason: &reason}}
	pv := PhaseViewFromState(ps)
	if pv.BlockedReason != "legacy reason" {
		t.Errorf("BlockedReason=%q, want 'legacy reason'", pv.BlockedReason)
	}
}

func TestPhaseViewFromStateCompletedAt(t *testing.T) {
	ps := &arc.PhaseState{CompletedAt: "2025-01-15T14:02:00Z"}
	pv := PhaseViewFromState(ps)
	if pv.CompletedAt != "14:02" {
		t.Errorf("CompletedAt=%q, want '14:02'", pv.CompletedAt)
	}
}

func TestPhaseViewFromStateCompletedAtInvalid(t *testing.T) {
	ps := &arc.PhaseState{CompletedAt: "not-a-date"}
	pv := PhaseViewFromState(ps)
	if pv.CompletedAt != "not-a-date" {
		t.Errorf("CompletedAt=%q, want 'not-a-date' (fallback)", pv.CompletedAt)
	}
}

func TestPhaseViewFromStateStuckAndRollback(t *testing.T) {
	ps := &arc.PhaseState{
		StuckIterations:  3,
		RollbackCount:    2,
		HangCount:        1,
		GlobalIterations: 12,
	}
	pv := PhaseViewFromState(ps)
	if pv.StuckIterations != 3 {
		t.Errorf("StuckIterations=%d", pv.StuckIterations)
	}
	if pv.RollbackCount != 2 {
		t.Errorf("RollbackCount=%d", pv.RollbackCount)
	}
	if pv.HangCount != 1 {
		t.Errorf("HangCount=%d", pv.HangCount)
	}
	if pv.GlobalIterations != 12 {
		t.Errorf("GlobalIterations=%d", pv.GlobalIterations)
	}
}

func TestPhaseViewFromStateNotes(t *testing.T) {
	ps := &arc.PhaseState{Notes: "investigating flaky test"}
	pv := PhaseViewFromState(ps)
	if pv.Notes != "investigating flaky test" {
		t.Errorf("Notes=%q", pv.Notes)
	}
}

func TestPhaseViewFromStateEscalations(t *testing.T) {
	ps := &arc.PhaseState{ExecutedEscalations: []string{"switch_model", "analyze_stuck"}}
	pv := PhaseViewFromState(ps)
	if len(pv.ExecutedEscalations) != 2 {
		t.Errorf("ExecutedEscalations len=%d", len(pv.ExecutedEscalations))
	}
}

// --- planSummary ---

func TestPlanSummaryFromViews(t *testing.T) {
	views := []PhaseView{
		{Status: "complete", InputTokens: 10000, OutputTokens: 5000, GlobalIterations: 5, TestsTotal: 10, TestsPassing: 10},
		{Status: "implementing", InputTokens: 20000, OutputTokens: 3000, GlobalIterations: 7, TestsTotal: 15, TestsPassing: 12, StuckIterations: 1},
		{Status: "blocked", InputTokens: 5000, OutputTokens: 1000, HasIntervention: true},
	}
	s := planSummaryFromViews(views, "feature")
	if s.WorkflowType != "feature" {
		t.Errorf("WorkflowType=%q", s.WorkflowType)
	}
	if s.TotalTokens != 44000 {
		t.Errorf("TotalTokens=%d, want 44000", s.TotalTokens)
	}
	if s.TotalIterations != 12 {
		t.Errorf("TotalIterations=%d, want 12", s.TotalIterations)
	}
	if s.TotalTests != 25 {
		t.Errorf("TotalTests=%d, want 25", s.TotalTests)
	}
	if s.TotalTestsPassing != 22 {
		t.Errorf("TotalTestsPassing=%d, want 22", s.TotalTestsPassing)
	}
	if s.PhasesComplete != 1 {
		t.Errorf("PhasesComplete=%d, want 1", s.PhasesComplete)
	}
	if s.PhasesTotal != 3 {
		t.Errorf("PhasesTotal=%d, want 3", s.PhasesTotal)
	}
	if s.PhasesActive != 1 {
		t.Errorf("PhasesActive=%d, want 1", s.PhasesActive)
	}
	if s.InterventionCount != 1 {
		t.Errorf("InterventionCount=%d, want 1", s.InterventionCount)
	}
	if s.BlockedCount != 1 {
		t.Errorf("BlockedCount=%d, want 1", s.BlockedCount)
	}
	if s.StuckCount != 1 {
		t.Errorf("StuckCount=%d, want 1", s.StuckCount)
	}
}

func TestPlanSummaryFromViewsEmpty(t *testing.T) {
	s := planSummaryFromViews(nil, "")
	if s.PhasesTotal != 0 || s.TotalTokens != 0 {
		t.Error("expected zero summary for nil views")
	}
}

// --- Navigation tests ---

func TestMonitorUpdateQuitQ(t *testing.T) {
	m := NewModel("test", "/tmp")
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	model := updated.(Model)
	if !model.quitting {
		t.Error("expected quitting=true")
	}
	if cmd == nil {
		t.Error("expected tea.Quit command")
	}
}

func TestMonitorUpdateQuitCtrlC(t *testing.T) {
	m := NewModel("test", "/tmp")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	model := updated.(Model)
	if !model.quitting {
		t.Error("expected quitting=true")
	}
}

func TestMonitorUpdateQuitEsc(t *testing.T) {
	m := NewModel("test", "/tmp")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	model := updated.(Model)
	if !model.quitting {
		t.Error("expected quitting=true")
	}
}

func TestUpdateSelectDown(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.phases = []PhaseView{{Name: "a"}, {Name: "b"}, {Name: "c"}}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	model := updated.(Model)
	if model.selectedIdx != 1 {
		t.Errorf("selectedIdx=%d, want 1", model.selectedIdx)
	}
}

func TestUpdateSelectUp(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.phases = []PhaseView{{Name: "a"}, {Name: "b"}}
	m.selectedIdx = 1

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	model := updated.(Model)
	if model.selectedIdx != 0 {
		t.Errorf("selectedIdx=%d, want 0", model.selectedIdx)
	}
}

func TestUpdateSelectClampLow(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.phases = []PhaseView{{Name: "a"}}
	m.selectedIdx = 0

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	model := updated.(Model)
	if model.selectedIdx != 0 {
		t.Errorf("selectedIdx=%d, want 0", model.selectedIdx)
	}
}

func TestUpdateSelectClampHigh(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.phases = []PhaseView{{Name: "a"}, {Name: "b"}}
	m.selectedIdx = 1

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	model := updated.(Model)
	if model.selectedIdx != 1 {
		t.Errorf("selectedIdx=%d, want 1 (clamped)", model.selectedIdx)
	}
}

func TestUpdateOpenDetail(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.phases = []PhaseView{{Name: "a"}}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := updated.(Model)
	if !model.showDetail {
		t.Error("expected showDetail=true")
	}
	if model.detailScroll != 0 {
		t.Errorf("detailScroll=%d, want 0", model.detailScroll)
	}
}

func TestUpdateOpenDetailEmptyPhases(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.phases = []PhaseView{}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := updated.(Model)
	if model.showDetail {
		t.Error("should not open detail with no phases")
	}
}

func TestUpdateCloseDetailWithEsc(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.phases = []PhaseView{{Name: "a"}}
	m.showDetail = true

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	model := updated.(Model)
	if model.showDetail {
		t.Error("expected showDetail=false")
	}
	if model.quitting {
		t.Error("should not quit from detail view")
	}
}

func TestUpdateCloseDetailWithQ(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.phases = []PhaseView{{Name: "a"}}
	m.showDetail = true

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	model := updated.(Model)
	if model.showDetail {
		t.Error("expected showDetail=false")
	}
	if model.quitting {
		t.Error("should not quit from detail view")
	}
}

func TestUpdateCtrlCAlwaysQuits(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.showDetail = true

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	model := updated.(Model)
	if !model.quitting {
		t.Error("ctrl+c should always quit")
	}
}

func TestUpdateScrollDetailDown(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.phases = []PhaseView{{Name: "a"}}
	m.showDetail = true

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	model := updated.(Model)
	if model.detailScroll != 1 {
		t.Errorf("detailScroll=%d, want 1", model.detailScroll)
	}
}

func TestUpdateScrollDetailUpClamped(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.phases = []PhaseView{{Name: "a"}}
	m.showDetail = true
	m.detailScroll = 0

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	model := updated.(Model)
	if model.detailScroll != 0 {
		t.Errorf("detailScroll=%d, want 0 (clamped)", model.detailScroll)
	}
}

func TestUpdateForceRefresh(t *testing.T) {
	m := NewModel("test", "/tmp")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if cmd == nil {
		t.Error("expected non-nil command from force refresh")
	}
}

func TestUpdateForceRefreshInDetail(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.phases = []PhaseView{{Name: "a"}}
	m.showDetail = true
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if cmd == nil {
		t.Error("expected non-nil command from force refresh in detail")
	}
}

// --- Window resize ---

func TestMonitorUpdateResize(t *testing.T) {
	m := NewModel("test", "/tmp")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := updated.(Model)
	if model.width != 120 || model.height != 40 {
		t.Errorf("size=%dx%d, want 120x40", model.width, model.height)
	}
}

// --- Refresh ---

func TestMonitorUpdateRefreshMsg(t *testing.T) {
	m := NewModel("test", "/tmp")
	phases := []PhaseView{
		{Name: "a", Status: "pending"},
		{Name: "b", Status: "implementing"},
		{Name: "c", Status: "complete"},
	}
	meta := planSummary{PhasesTotal: 3, PhasesComplete: 1}
	updated, _ := m.Update(refreshMsg{phases: phases, meta: meta})
	model := updated.(Model)
	if len(model.phases) != 3 {
		t.Errorf("phases count=%d, want 3", len(model.phases))
	}
	if model.lastUpdate.IsZero() {
		t.Error("lastUpdate not set")
	}
	if model.planMeta.PhasesTotal != 3 {
		t.Errorf("planMeta.PhasesTotal=%d, want 3", model.planMeta.PhasesTotal)
	}
}

func TestMonitorUpdateRefreshClampsSelection(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.selectedIdx = 5
	phases := []PhaseView{{Name: "a"}, {Name: "b"}}
	updated, _ := m.Update(refreshMsg{phases: phases})
	model := updated.(Model)
	if model.selectedIdx != 1 {
		t.Errorf("selectedIdx=%d, want 1 (clamped)", model.selectedIdx)
	}
}

func TestMonitorUpdateTick(t *testing.T) {
	m := NewModel("test", "/tmp")
	_, cmd := m.Update(tickMsg{})
	if cmd == nil {
		t.Error("expected refresh command from tick")
	}
}

// --- View rendering ---

func TestMonitorViewRendersAllPhases(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.phases = []PhaseView{
		{Name: "a", Status: "pending", Icon: "[ ]"},
		{Name: "b", Status: "implementing", Icon: "[>]"},
		{Name: "c", Status: "complete", Icon: "[x]"},
	}
	m.width = 100

	view := m.View()
	if !strings.Contains(view, "a") || !strings.Contains(view, "b") || !strings.Contains(view, "c") {
		t.Error("view missing phase names")
	}
	if !strings.Contains(view, "[ ]") || !strings.Contains(view, "[>]") || !strings.Contains(view, "[x]") {
		t.Error("view missing phase icons")
	}
}

func TestMonitorViewNoPhases(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.phases = []PhaseView{}
	view := m.View()
	if !strings.Contains(view, "No phases") {
		t.Error("expected 'No phases' message")
	}
}

func TestModelViewQuitting(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.quitting = true
	view := m.View()
	if view != "" {
		t.Errorf("expected empty view when quitting, got %q", view)
	}
}

func TestViewShowsDetail(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.phases = []PhaseView{{Name: "auth-core", Status: "implementing", CurrentState: "qa_review"}}
	m.showDetail = true
	m.height = 50
	m.width = 100

	view := m.View()
	if !strings.Contains(view, "auth-core") {
		t.Error("detail should show phase name")
	}
	if !strings.Contains(view, "esc: back") {
		t.Error("detail should show back hint")
	}
	if !strings.Contains(view, "qa_review") {
		t.Error("detail should show current state")
	}
}

func TestViewShowsAlerts(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.phases = []PhaseView{
		{Name: "auth", Status: "implementing", HasIntervention: true, InterventionReason: "need help"},
		{Name: "api", Status: "pending"},
	}
	m.width = 100

	view := m.View()
	if !strings.Contains(view, "INTERVENTION") {
		t.Error("view should show intervention alert")
	}
	if !strings.Contains(view, "auth") {
		t.Error("intervention alert should name the phase")
	}
}

// --- Phase row rendering ---

func TestRenderPhaseRowWithNewColumns(t *testing.T) {
	pv := PhaseView{
		Name: "core", Status: "implementing", Icon: "[>]",
		CurrentState: "qa_review", Iteration: 5, MaxIteration: 25,
		TestsPassing: 7, TestsTotal: 12, InputTokens: 28000, OutputTokens: 4800,
		LastVerdict: "concerns",
	}
	row := renderPhaseRow(pv, 100, false)
	if !strings.Contains(row, "qa_review") {
		t.Error("missing state column")
	}
	if !strings.Contains(row, "5/25") {
		t.Error("missing iter/max column")
	}
	if !strings.Contains(row, "7/12") {
		t.Error("missing tests column")
	}
	if !strings.Contains(row, "32k") {
		t.Error("missing tokens column")
	}
	if !strings.Contains(row, "concerns") {
		t.Error("missing verdict column")
	}
}

func TestRenderPhaseRowStuck(t *testing.T) {
	pv := PhaseView{
		Name: "core", Status: "implementing", Icon: "[>]",
		Iteration: 5, MaxIteration: 25, StuckIterations: 2,
	}
	row := renderPhaseRow(pv, 100, false)
	if !strings.Contains(row, "~5/25") {
		t.Errorf("missing stuck indicator, got %q", row)
	}
}

func TestRenderPhaseRowModelOverride(t *testing.T) {
	pv := PhaseView{
		Name: "core", Status: "implementing", Icon: "[>]",
		ModelOverride: "sonnet",
	}
	row := renderPhaseRow(pv, 100, false)
	if !strings.Contains(row, "core*") {
		t.Error("missing model override indicator")
	}
}

func TestRenderPhaseRowSelected(t *testing.T) {
	pv := PhaseView{Name: "core", Status: "pending", Icon: "[ ]"}
	row := renderPhaseRow(pv, 100, true)
	if !strings.Contains(row, ">") {
		t.Error("missing selection cursor")
	}
}

func TestRenderPhaseRowNotSelected(t *testing.T) {
	pv := PhaseView{Name: "core", Status: "pending", Icon: "[ ]"}
	row := renderPhaseRow(pv, 100, false)
	if strings.HasPrefix(row, ">") {
		t.Error("should not have selection cursor")
	}
}

func TestRenderPhaseRowNarrow(t *testing.T) {
	pv := PhaseView{
		Name: "core", Status: "implementing", Icon: "[>]",
		CurrentState: "impl", Iteration: 5, MaxIteration: 25,
		InputTokens: 28000, OutputTokens: 4800, LastVerdict: "approved",
	}
	row := renderPhaseRow(pv, 60, false)
	// Tokens and verdict should be hidden in narrow mode
	if strings.Contains(row, "32k") {
		t.Error("tokens should be hidden in narrow terminal")
	}
	if strings.Contains(row, "approved") {
		t.Error("verdict should be hidden in narrow terminal")
	}
}

func TestRenderPhaseRowComplete(t *testing.T) {
	pv := PhaseView{Name: "core", Status: "complete", Icon: "[x]", CompletedAt: "14:02", LastVerdict: "approved"}
	row := renderPhaseRow(pv, 100, false)
	if !strings.Contains(row, "[x]") {
		t.Error("missing icon")
	}
	if !strings.Contains(row, "14:02") {
		t.Error("missing completed timestamp")
	}
}

func TestRenderPhaseRowVeryLongName(t *testing.T) {
	pv := PhaseView{Name: "a-very-long-phase-name-that-exceeds-column-width-significantly", Status: "implementing", Icon: "[>]"}
	row := renderPhaseRow(pv, 100, false)
	if strings.Contains(row, "significantly") {
		t.Error("name should be truncated")
	}
}

func TestRenderPhaseRowZeroWidth(t *testing.T) {
	pv := PhaseView{Name: "core", Status: "implementing", Icon: "[>]"}
	row := renderPhaseRow(pv, 0, false)
	if row == "" {
		t.Error("expected non-empty row")
	}
}

// --- Intervention alerts ---

func TestRenderInterventionAlertsNone(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.phases = []PhaseView{
		{Name: "a", Status: "implementing"},
		{Name: "b", Status: "pending"},
	}
	alerts := m.renderInterventionAlerts()
	if alerts != "" {
		t.Errorf("expected empty alerts, got %q", alerts)
	}
}

func TestRenderInterventionAlertsOne(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.phases = []PhaseView{
		{Name: "auth", HasIntervention: true, InterventionReason: "need guidance"},
	}
	alerts := m.renderInterventionAlerts()
	if !strings.Contains(alerts, "INTERVENTION") {
		t.Error("missing INTERVENTION keyword")
	}
	if !strings.Contains(alerts, "auth") {
		t.Error("missing phase name")
	}
	if !strings.Contains(alerts, "need guidance") {
		t.Error("missing reason")
	}
}

func TestRenderInterventionAlertsMultiple(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.phases = []PhaseView{
		{Name: "auth", HasIntervention: true, InterventionReason: "reason1"},
		{Name: "api", HasIntervention: true, InterventionReason: "reason2"},
	}
	alerts := m.renderInterventionAlerts()
	if !strings.Contains(alerts, "auth") || !strings.Contains(alerts, "api") {
		t.Error("missing one of the phase names")
	}
}

// --- Detail panel ---

func TestRenderDetailPanel(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.height = 60
	m.width = 100
	m.showDetail = true
	m.phases = []PhaseView{{
		Name:             "auth-core",
		Status:           "implementing",
		CurrentState:     "qa_review",
		WorkflowType:     "feature",
		Iteration:        7,
		MaxIteration:     25,
		GlobalIterations: 12,
		TestsPassing:     14,
		TestsTotal:       20,
		RollbackCount:    2,
		StuckIterations:  1,
		HangCount:        0,
		LastCommit:       "a3f92b1",
		InputTokens:      28000,
		OutputTokens:     4800,
		Notes:            "investigating flaky test",
		VerdictHistory: []VerdictRow{
			{Iteration: 7, State: "qa_review", Verdict: "concerns", Timestamp: "14:31"},
			{Iteration: 6, State: "impl", Verdict: "approved", Timestamp: "14:28"},
		},
		ExecutedEscalations: []string{"switch_model"},
		DisputeDetails:      []DisputeRow{{TestName: "TestAuth", Reason: "401 error"}},
	}}

	view := m.renderDetailPanel()
	if !strings.Contains(view, "auth-core") {
		t.Error("missing phase name")
	}
	if !strings.Contains(view, "qa_review") {
		t.Error("missing current state")
	}
	if !strings.Contains(view, "feature") {
		t.Error("missing workflow type")
	}
	if !strings.Contains(view, "7/25") {
		t.Error("missing iteration")
	}
	if !strings.Contains(view, "14/20") {
		t.Error("missing tests")
	}
	if !strings.Contains(view, "a3f92b1") {
		t.Error("missing commit")
	}
	if !strings.Contains(view, "investigating flaky test") {
		t.Error("missing notes")
	}
	if !strings.Contains(view, "Verdict History") {
		t.Error("missing verdict history section")
	}
	if !strings.Contains(view, "Escalations") {
		t.Error("missing escalations section")
	}
	if !strings.Contains(view, "Disputes") {
		t.Error("missing disputes section")
	}
	if !strings.Contains(view, "TestAuth") {
		t.Error("missing dispute detail")
	}
}

func TestRenderDetailPanelNoOptionalSections(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.height = 50
	m.phases = []PhaseView{{Name: "minimal", Status: "pending"}}
	m.showDetail = true

	view := m.renderDetailPanel()
	if strings.Contains(view, "Verdict History") {
		t.Error("should not show verdict history section when empty")
	}
	if strings.Contains(view, "Escalations") {
		t.Error("should not show escalations section when empty")
	}
	if strings.Contains(view, "Disputes") {
		t.Error("should not show disputes section when empty")
	}
	if strings.Contains(view, "Chunks") {
		t.Error("should not show chunks section when empty")
	}
}

func TestRenderDetailPanelChunks(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.height = 50
	m.phases = []PhaseView{{
		Name: "chunked", Status: "implementing",
		ChunksTotal: 5, ChunksDone: 3, ChunkCurrent: "write tests",
	}}
	m.showDetail = true

	view := m.renderDetailPanel()
	if !strings.Contains(view, "3/5") {
		t.Error("missing chunk progress")
	}
	if !strings.Contains(view, "write tests") {
		t.Error("missing current chunk name")
	}
}

func TestRenderDetailPanelIntervention(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.height = 50
	m.phases = []PhaseView{{
		Name: "stuck", Status: "implementing",
		HasIntervention: true, InterventionReason: "need help",
		InterventionOptions: []string{"continue", "rollback"},
	}}
	m.showDetail = true

	view := m.renderDetailPanel()
	if !strings.Contains(view, "INTERVENTION") {
		t.Error("missing intervention alert")
	}
	if !strings.Contains(view, "need help") {
		t.Error("missing intervention reason")
	}
	if !strings.Contains(view, "continue") || !strings.Contains(view, "rollback") {
		t.Error("missing intervention options")
	}
}

func TestRenderDetailPanelParallel(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.height = 50
	m.phases = []PhaseView{{
		Name:             "parallel",
		Status:           "implementing",
		HasParallel:      true,
		ParallelBranches: map[string]string{"branch-a": "complete", "branch-b": "running"},
		ParallelVerdict:  "pending",
	}}
	m.showDetail = true

	view := m.renderDetailPanel()
	if !strings.Contains(view, "Parallel Execution") {
		t.Error("missing parallel section")
	}
	if !strings.Contains(view, "branch-a") || !strings.Contains(view, "branch-b") {
		t.Error("missing branch names")
	}
}

func TestRenderDetailPanelScroll(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.height = 10
	m.phases = []PhaseView{{
		Name:         "scrollable",
		Status:       "implementing",
		CurrentState: "impl",
		VerdictHistory: []VerdictRow{
			{Iteration: 1, State: "impl", Verdict: "approved", Timestamp: "10:00"},
			{Iteration: 2, State: "qa", Verdict: "concerns", Timestamp: "10:30"},
			{Iteration: 3, State: "impl", Verdict: "approved", Timestamp: "11:00"},
			{Iteration: 4, State: "qa", Verdict: "concerns", Timestamp: "11:30"},
			{Iteration: 5, State: "impl", Verdict: "approved", Timestamp: "12:00"},
		},
	}}
	m.showDetail = true
	m.detailScroll = 2

	view := m.renderDetailPanel()
	// The scroll indicator should show the offset
	if !strings.Contains(view, "line") {
		t.Error("missing scroll indicator")
	}
}

// --- Header ---

func TestRenderHeader(t *testing.T) {
	m := NewModel("test-plan", "/tmp")
	m.phases = []PhaseView{
		{Status: "complete"},
		{Status: "pending"},
		{Status: "pending"},
	}
	m.planMeta = planSummary{
		PhasesComplete: 1,
		PhasesTotal:    3,
		WorkflowType:   "feature",
	}
	header := m.renderHeader()
	if !strings.Contains(header, "test-plan") {
		t.Error("header missing plan name")
	}
	if !strings.Contains(header, "1/3") {
		t.Error("header missing progress")
	}
	if !strings.Contains(header, "feature") {
		t.Error("header missing workflow type")
	}
}

func TestRenderHeaderWithTokens(t *testing.T) {
	m := NewModel("plan", "/tmp")
	m.planMeta = planSummary{
		TotalTokens:    142000,
		TotalIterations: 47,
		PhasesActive:   2,
	}
	header := m.renderHeader()
	if !strings.Contains(header, "142k tokens") {
		t.Error("header missing total tokens")
	}
	if !strings.Contains(header, "47 iter") {
		t.Error("header missing total iterations")
	}
}

func TestRenderHeaderWithProblems(t *testing.T) {
	m := NewModel("plan", "/tmp")
	m.planMeta = planSummary{
		InterventionCount: 1,
		BlockedCount:      2,
		StuckCount:        1,
	}
	header := m.renderHeader()
	if !strings.Contains(header, "1 intervention") {
		t.Error("header missing intervention count")
	}
	if !strings.Contains(header, "2 blocked") {
		t.Error("header missing blocked count")
	}
	if !strings.Contains(header, "1 stuck") {
		t.Error("header missing stuck count")
	}
}

// --- Footer ---

func TestRenderFooter(t *testing.T) {
	m := NewModel("test", "/tmp")
	footer := m.renderFooter()
	if !strings.Contains(footer, "q: quit") {
		t.Error("footer missing quit hint")
	}
	if !strings.Contains(footer, "select") {
		t.Error("footer missing select hint")
	}
	if !strings.Contains(footer, "detail") {
		t.Error("footer missing detail hint")
	}
}

// --- Column header ---

func TestRenderColumnHeader(t *testing.T) {
	header := renderColumnHeader(100)
	if !strings.Contains(header, "PHASE") {
		t.Error("missing PHASE column")
	}
	if !strings.Contains(header, "STATE") {
		t.Error("missing STATE column")
	}
	if !strings.Contains(header, "ITER") {
		t.Error("missing ITER column")
	}
	if !strings.Contains(header, "TESTS") {
		t.Error("missing TESTS column")
	}
	if !strings.Contains(header, "TOKENS") {
		t.Error("missing TOKENS column")
	}
	if !strings.Contains(header, "VERDICT") {
		t.Error("missing VERDICT column")
	}
}

func TestRenderColumnHeaderNarrow(t *testing.T) {
	header := renderColumnHeader(70)
	if !strings.Contains(header, "PHASE") {
		t.Error("missing PHASE column")
	}
	if strings.Contains(header, "TOKENS") {
		t.Error("TOKENS should be hidden in narrow mode")
	}
}

// --- Formatting helpers ---

func TestFormatTokens(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, ""},
		{500, "500"},
		{1000, "1.0k"},
		{1500, "1.5k"},
		{9999, "10.0k"},
		{10000, "10k"},
		{28000, "28k"},
		{142000, "142k"},
		{-1, ""},
	}
	for _, tc := range tests {
		got := formatTokens(tc.n)
		if got != tc.want {
			t.Errorf("formatTokens(%d)=%q, want %q", tc.n, got, tc.want)
		}
	}
}

// --- Verdict styles ---

func TestVerdictStyleApproved(t *testing.T) {
	s := VerdictStyle("approved")
	if s.GetForeground() == dimStyle.GetForeground() {
		t.Error("approved should not use dim style")
	}
}

func TestVerdictStyleConcerns(t *testing.T) {
	s := VerdictStyle("concerns")
	if s.GetForeground() == dimStyle.GetForeground() {
		t.Error("concerns should not use dim style")
	}
}

func TestVerdictStyleRejected(t *testing.T) {
	s := VerdictStyle("rejected")
	if s.GetForeground() == dimStyle.GetForeground() {
		t.Error("rejected should not use dim style")
	}
}

func TestVerdictStyleUnknown(t *testing.T) {
	_ = VerdictStyle("some_unknown_verdict")
	// Should not panic
}

func TestStatusColorMapping(t *testing.T) {
	_ = StatusColor("implementing")
	_ = StatusColor("complete")
	_ = StatusColor("blocked")
	_ = StatusColor("disputed")
	_ = StatusColor("pending")
	_ = StatusColor("")
	_ = StatusColor("unknown")
}

func TestStatusColorEmptyString(t *testing.T) {
	c := StatusColor("")
	if c.Light == "" && c.Dark == "" {
		t.Error("expected non-empty color for default")
	}
}

// --- Phase table ---

func TestRenderPhaseTable(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.phases = []PhaseView{
		{Name: "a", Icon: "[ ]"},
		{Name: "b", Icon: "[>]"},
	}
	m.width = 100
	table := m.renderPhaseTable()
	if !strings.Contains(table, "a") || !strings.Contains(table, "b") {
		t.Error("table missing phases")
	}
	if !strings.Contains(table, "PHASE") {
		t.Error("table missing column header")
	}
}

// --- Init and tick ---

func TestTickReturnsCommand(t *testing.T) {
	cmd := tick()
	if cmd == nil {
		t.Error("tick() returned nil")
	}
}

func TestMonitorInitReturnsCmd(t *testing.T) {
	m := NewModel("plan", "/tmp")
	cmd := m.Init()
	if cmd == nil {
		t.Error("Init() returned nil")
	}
}

// --- Refresh reads files ---

func TestRefreshReadsFiles(t *testing.T) {
	planDir := t.TempDir()

	meta := arc.NewPlanMeta("test-plan", "feature", []string{"core", "api"})
	metaData, _ := json.MarshalIndent(meta, "", "  ")
	os.WriteFile(filepath.Join(planDir, "plan.json"), metaData, 0644)

	for _, phase := range []string{"core", "api"} {
		phaseDir := filepath.Join(planDir, "phases", phase)
		os.MkdirAll(phaseDir, 0755)
		ps := arc.NewPhaseState("test-plan", phase, "feature")
		ps.PhaseStatus = "implementing"
		ps.Iteration.Current = 3
		ps.Usage = arc.Usage{InputTokens: 10000, OutputTokens: 2000}
		data, _ := json.MarshalIndent(ps, "", "  ")
		os.WriteFile(filepath.Join(phaseDir, "state.json"), data, 0644)
	}

	m := NewModel("test-plan", planDir)
	msg := m.refresh()
	rm, ok := msg.(refreshMsg)
	if !ok {
		t.Fatalf("expected refreshMsg, got %T", msg)
	}
	if len(rm.phases) != 2 {
		t.Fatalf("phases=%d, want 2", len(rm.phases))
	}
	if rm.phases[0].Status != "implementing" {
		t.Errorf("phase 0 status=%q", rm.phases[0].Status)
	}
	if rm.phases[0].InputTokens != 10000 {
		t.Errorf("phase 0 input tokens=%d, want 10000", rm.phases[0].InputTokens)
	}
	if rm.meta.WorkflowType != "feature" {
		t.Errorf("meta.WorkflowType=%q", rm.meta.WorkflowType)
	}
	if rm.meta.TotalTokens != 24000 {
		t.Errorf("meta.TotalTokens=%d, want 24000", rm.meta.TotalTokens)
	}
}
