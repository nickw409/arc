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
	m := NewModel("my-plan", "/tmp/plans")
	if m.planFilter != "my-plan" {
		t.Errorf("planFilter=%q, want 'my-plan'", m.planFilter)
	}
	if m.plans == nil {
		t.Error("plans is nil, want empty slice")
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

func TestNewModelEmptyFilter(t *testing.T) {
	m := NewModel("", "/tmp/plans")
	if m.planFilter != "" {
		t.Errorf("planFilter=%q, want empty", m.planFilter)
	}
	if m.plans == nil {
		t.Error("plans is nil, want empty slice")
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
	}

	pv := PhaseViewFromState(ps)
	if pv.Status != "implementing" {
		t.Errorf("Status=%q", pv.Status)
	}
	if pv.Icon != "[>]" {
		t.Errorf("Icon=%q, want '[>]'", pv.Icon)
	}
	if pv.Iteration != 5 {
		t.Errorf("Iteration=%d, want 5", pv.Iteration)
	}
	if pv.TestsPassing != 3 {
		t.Errorf("TestsPassing=%d", pv.TestsPassing)
	}
	if pv.TestsTotal != 10 {
		t.Errorf("TestsTotal=%d", pv.TestsTotal)
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
	if pv.Icon != "[>]" {
		t.Errorf("Icon=%q, want '[>]'", pv.Icon)
	}
}

func TestPhaseViewFromStateAllStatuses(t *testing.T) {
	tests := []struct {
		status string
		icon   string
	}{
		{"pending", "[ ]"},
		{"complete", "[x]"},
		{"act", "[>]"},
		{"tests", "[>]"},
		{"adversary", "[!]"},
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

func TestPhaseViewIterationFromGateSystem(t *testing.T) {
	// Iteration is sourced from state.Iteration.Current (gate system field).
	ps := &arc.PhaseState{
		Iteration: arc.Iteration{Current: 8, Max: 20},
	}
	pv := PhaseViewFromState(ps)
	if pv.Iteration != 8 {
		t.Errorf("Iteration=%d, want 8", pv.Iteration)
	}
	if pv.MaxIteration != 20 {
		t.Errorf("MaxIteration=%d, want 20", pv.MaxIteration)
	}
}

func TestPhaseViewIterationEdgeCases(t *testing.T) {
	// Max set but current 0.
	ps := &arc.PhaseState{
		Iteration: arc.Iteration{Current: 0, Max: 25},
	}
	pv := PhaseViewFromState(ps)
	if pv.Iteration != 0 {
		t.Errorf("Iteration=%d, want 0", pv.Iteration)
	}
	if pv.MaxIteration != 25 {
		t.Errorf("MaxIteration=%d, want 25", pv.MaxIteration)
	}

	// Both zero.
	ps2 := &arc.PhaseState{
		Iteration: arc.Iteration{Current: 0, Max: 0},
	}
	pv2 := PhaseViewFromState(ps2)
	if pv2.Iteration != 0 || pv2.MaxIteration != 0 {
		t.Error("expected both zero")
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

func TestPhaseViewFromStateNotes(t *testing.T) {
	ps := &arc.PhaseState{Notes: "investigating flaky test"}
	pv := PhaseViewFromState(ps)
	if pv.Notes != "investigating flaky test" {
		t.Errorf("Notes=%q", pv.Notes)
	}
}

// --- PlanView construction ---

func TestPlanViewConstruction(t *testing.T) {
	phases := []PhaseView{
		{Name: "a", Status: "complete", Iteration: 3},
		{Name: "b", Status: "implementing", Iteration: 5},
	}
	pv := PlanView{
		Name:         "my-plan",
		WorkflowType: "feature",
		Phases:       phases,
		Meta:         planSummaryFromViews(phases, "feature"),
	}
	if pv.Name != "my-plan" {
		t.Errorf("Name=%q", pv.Name)
	}
	if len(pv.Phases) != 2 {
		t.Errorf("Phases len=%d", len(pv.Phases))
	}
	if pv.Meta.CompletedCount != 1 {
		t.Errorf("CompletedCount=%d, want 1", pv.Meta.CompletedCount)
	}
	if pv.Meta.TotalIterations != 8 {
		t.Errorf("TotalIterations=%d, want 8", pv.Meta.TotalIterations)
	}
}

// --- planSummary ---

func TestPlanSummaryFromViews(t *testing.T) {
	views := []PhaseView{
		{Status: "complete", Iteration: 5, TestsTotal: 10, TestsPassing: 10},
		{Status: "implementing", Iteration: 7, TestsTotal: 15, TestsPassing: 12},
		{Status: "blocked"},
	}
	s := planSummaryFromViews(views, "feature")
	if s.TotalIterations != 12 {
		t.Errorf("TotalIterations=%d, want 12", s.TotalIterations)
	}
	if s.CompletedCount != 1 {
		t.Errorf("CompletedCount=%d, want 1", s.CompletedCount)
	}
	if s.RunningCount != 1 {
		t.Errorf("RunningCount=%d, want 1", s.RunningCount)
	}
	if s.FailedCount != 1 {
		t.Errorf("FailedCount=%d, want 1", s.FailedCount)
	}
	if s.PendingCount != 0 {
		t.Errorf("PendingCount=%d, want 0", s.PendingCount)
	}
}

func TestPlanSummaryFromViewsEmpty(t *testing.T) {
	s := planSummaryFromViews(nil, "")
	if s.TotalIterations != 0 || s.CompletedCount != 0 {
		t.Error("expected zero summary for nil views")
	}
}

// --- totalPhases ---

func TestTotalPhasesEmpty(t *testing.T) {
	m := NewModel("", "/tmp")
	if m.totalPhases() != 0 {
		t.Errorf("totalPhases=%d, want 0", m.totalPhases())
	}
}

func TestTotalPhasesMixed(t *testing.T) {
	m := NewModel("", "/tmp")
	m.plans = []PlanView{
		{Name: "a", Phases: []PhaseView{{Name: "p1"}, {Name: "p2"}}},
		{Name: "b", Phases: []PhaseView{{Name: "p3"}}},
	}
	if m.totalPhases() != 3 {
		t.Errorf("totalPhases=%d, want 3", m.totalPhases())
	}
}

func TestTotalPhasesMultiplePlans(t *testing.T) {
	m := NewModel("", "/tmp")
	m.plans = []PlanView{
		{Name: "a", Phases: []PhaseView{{Name: "p1"}}},
		{Name: "b", Phases: []PhaseView{{Name: "p2"}, {Name: "p3"}}},
		{Name: "c", Phases: []PhaseView{{Name: "p4"}, {Name: "p5"}, {Name: "p6"}}},
	}
	if m.totalPhases() != 6 {
		t.Errorf("totalPhases=%d, want 6", m.totalPhases())
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
	m.plans = []PlanView{{Name: "p", Phases: []PhaseView{{Name: "a"}, {Name: "b"}, {Name: "c"}}}}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	model := updated.(Model)
	if model.selectedIdx != 1 {
		t.Errorf("selectedIdx=%d, want 1", model.selectedIdx)
	}
}

func TestUpdateSelectUp(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.plans = []PlanView{{Name: "p", Phases: []PhaseView{{Name: "a"}, {Name: "b"}}}}
	m.selectedIdx = 1

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	model := updated.(Model)
	if model.selectedIdx != 0 {
		t.Errorf("selectedIdx=%d, want 0", model.selectedIdx)
	}
}

func TestUpdateSelectClampLow(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.plans = []PlanView{{Name: "p", Phases: []PhaseView{{Name: "a"}}}}
	m.selectedIdx = 0

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	model := updated.(Model)
	if model.selectedIdx != 0 {
		t.Errorf("selectedIdx=%d, want 0", model.selectedIdx)
	}
}

func TestUpdateSelectClampHigh(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.plans = []PlanView{{Name: "p", Phases: []PhaseView{{Name: "a"}, {Name: "b"}}}}
	m.selectedIdx = 1

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	model := updated.(Model)
	if model.selectedIdx != 1 {
		t.Errorf("selectedIdx=%d, want 1 (clamped)", model.selectedIdx)
	}
}

func TestUpdateOpenDetail(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.plans = []PlanView{{Name: "p", Phases: []PhaseView{{Name: "a"}}}}

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
	m.plans = []PlanView{}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := updated.(Model)
	if model.showDetail {
		t.Error("should not open detail with no phases")
	}
}

func TestUpdateCloseDetailWithEsc(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.plans = []PlanView{{Name: "p", Phases: []PhaseView{{Name: "a"}}}}
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
	m.plans = []PlanView{{Name: "p", Phases: []PhaseView{{Name: "a"}}}}
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
	m.plans = []PlanView{{Name: "p", Phases: []PhaseView{{Name: "a"}}}}
	m.showDetail = true

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	model := updated.(Model)
	if model.detailScroll != 1 {
		t.Errorf("detailScroll=%d, want 1", model.detailScroll)
	}
}

func TestUpdateScrollDetailUpClamped(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.plans = []PlanView{{Name: "p", Phases: []PhaseView{{Name: "a"}}}}
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
	m.plans = []PlanView{{Name: "p", Phases: []PhaseView{{Name: "a"}}}}
	m.showDetail = true
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if cmd == nil {
		t.Error("expected non-nil command from force refresh in detail")
	}
}

func TestNavigationWhenTotalPhasesZero(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.plans = []PlanView{}

	// Down on empty does nothing.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	model := updated.(Model)
	if model.selectedIdx != 0 {
		t.Errorf("selectedIdx=%d, want 0", model.selectedIdx)
	}

	// Up on empty does nothing.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	model = updated.(Model)
	if model.selectedIdx != 0 {
		t.Errorf("selectedIdx=%d, want 0", model.selectedIdx)
	}
}

func TestNavigationClampWhenTotalPhasesZero(t *testing.T) {
	// If plans go from having phases to being empty, selectedIdx is clamped.
	m := NewModel("test", "/tmp")
	m.selectedIdx = 5
	updated, _ := m.Update(refreshMsg{plans: []PlanView{}})
	model := updated.(Model)
	if model.selectedIdx != 0 {
		t.Errorf("selectedIdx=%d, want 0 (clamped to zero for empty)", model.selectedIdx)
	}
}

func TestNavigationAcrossPlanBoundaries(t *testing.T) {
	m := NewModel("", "/tmp")
	m.plans = []PlanView{
		{Name: "plan-a", Phases: []PhaseView{{Name: "p1"}, {Name: "p2"}}},
		{Name: "plan-b", Phases: []PhaseView{{Name: "p3"}}},
	}
	// Navigate from last phase of plan-a (idx=1) to first phase of plan-b (idx=2).
	m.selectedIdx = 1
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	model := updated.(Model)
	if model.selectedIdx != 2 {
		t.Errorf("selectedIdx=%d, want 2 (cross plan boundary)", model.selectedIdx)
	}

	// Verify which plan/phase that maps to.
	planIdx, phaseIdx := model.selectedPhase()
	if planIdx != 1 || phaseIdx != 0 {
		t.Errorf("selectedPhase=(%d,%d), want (1,0)", planIdx, phaseIdx)
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
	updated, _ := m.Update(refreshMsg{plans: []PlanView{{Name: "p", Phases: phases}}})
	model := updated.(Model)
	if len(model.plans) != 1 {
		t.Fatalf("plans count=%d, want 1", len(model.plans))
	}
	if len(model.plans[0].Phases) != 3 {
		t.Errorf("phases count=%d, want 3", len(model.plans[0].Phases))
	}
	if model.lastUpdate.IsZero() {
		t.Error("lastUpdate not set")
	}
}

func TestMonitorUpdateRefreshClampsSelection(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.selectedIdx = 5
	phases := []PhaseView{{Name: "a"}, {Name: "b"}}
	updated, _ := m.Update(refreshMsg{plans: []PlanView{{Name: "p", Phases: phases}}})
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
	m.plans = []PlanView{{Name: "p", Phases: []PhaseView{
		{Name: "a", Status: "pending", Icon: "[ ]"},
		{Name: "b", Status: "implementing", Icon: "[>]"},
		{Name: "c", Status: "complete", Icon: "[x]"},
	}}}
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
	m.plans = []PlanView{}
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
	m.plans = []PlanView{{Name: "test-plan", Phases: []PhaseView{{Name: "auth-core", Status: "implementing"}}}}
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
}

// --- Phase row rendering ---

func TestRenderPhaseRowWithNewColumns(t *testing.T) {
	pv := PhaseView{
		Name: "core", Status: "running", Icon: "[>]",
		Iteration: 5, MaxIteration: 25,
		TestsPassing: 7, TestsTotal: 12, InputTokens: 28000, OutputTokens: 4800,
		Activity: "running gate checks",
	}
	row := renderPhaseRow(pv, 100, false)
	if !strings.Contains(row, "5/25") {
		t.Error("missing iter/max column")
	}
	if !strings.Contains(row, "7/12") {
		t.Error("missing tests column")
	}
	if !strings.Contains(row, "32k") {
		t.Error("missing tokens column")
	}
	// Activity line should NOT be dimmed — it should appear as plain text.
	if !strings.Contains(row, "running gate checks") {
		t.Error("missing activity line")
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
		Iteration: 5, MaxIteration: 25,
		InputTokens: 28000, OutputTokens: 4800,
	}
	row := renderPhaseRow(pv, 60, false)
	if strings.Contains(row, "32k") {
		t.Error("tokens should be hidden in narrow terminal")
	}
}

func TestRenderPhaseRowComplete(t *testing.T) {
	pv := PhaseView{Name: "core", Status: "complete", Icon: "[x]", CompletedAt: "14:02"}
	row := renderPhaseRow(pv, 100, false)
	if !strings.Contains(row, "[x]") {
		t.Error("missing icon")
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

func TestRenderPhaseTableActivityNotDimmed(t *testing.T) {
	// Verify activity text appears as-is for running phases.
	pv := PhaseView{
		Name: "core", Status: "running", Icon: "[>]",
		Activity: "compiling package",
	}
	row := renderPhaseRow(pv, 100, false)
	if !strings.Contains(row, "compiling package") {
		t.Error("activity line missing")
	}
	// The activity line in the source code is NOT wrapped in dimStyle.Render.
	// We verify it's present and that the row contains a newline (activity on separate line).
	lines := strings.Split(row, "\n")
	if len(lines) < 2 {
		t.Error("expected activity on a separate line")
	}
}

// --- Activity line rendering ---

func TestRenderPhaseRowActivityShownForActive(t *testing.T) {
	pv := PhaseView{
		Name:     "core",
		Status:   "running",
		Icon:     "[>]",
		Activity: "writing tests",
	}
	row := renderPhaseRow(pv, 100, false)
	if !strings.Contains(row, "writing tests") {
		t.Error("activity line should appear for running phases on wide terminal")
	}
}

func TestRenderPhaseRowActivityHiddenForInactive(t *testing.T) {
	for _, status := range []string{"pending", "complete", "blocked", "deferred"} {
		pv := PhaseView{
			Name:     "core",
			Status:   status,
			Icon:     "[ ]",
			Activity: "some activity",
		}
		row := renderPhaseRow(pv, 100, false)
		if strings.Contains(row, "some activity") {
			t.Errorf("activity should be hidden for status=%q", status)
		}
	}
}

func TestRenderPhaseRowActivityTruncated(t *testing.T) {
	pv := PhaseView{
		Name:     "core",
		Status:   "running",
		Icon:     "[>]",
		Activity: strings.Repeat("x", 200),
	}
	row := renderPhaseRow(pv, 80, false)
	if strings.Contains(row, strings.Repeat("x", 100)) {
		t.Error("activity should be truncated to fit terminal width")
	}
	if !strings.Contains(row, "...") {
		t.Error("truncated activity should end with '...'")
	}
}

// --- Detail panel ---

func TestRenderDetailPanel(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.height = 60
	m.width = 100
	m.showDetail = true
	m.plans = []PlanView{{Name: "my-plan", Phases: []PhaseView{{
		Name:         "auth-core",
		Status:       "implementing",
		Iteration:    7,
		MaxIteration: 25,
		TestsPassing: 14,
		TestsTotal:   20,
		LastCommit:   "a3f92b1",
		InputTokens:  28000,
		OutputTokens: 4800,
		Notes:        "investigating flaky test",
	}}}}

	view := m.renderDetailPanel()
	if !strings.Contains(view, "auth-core") {
		t.Error("missing phase name")
	}
	if !strings.Contains(view, "my-plan") {
		t.Error("missing plan name in header")
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
	// AdversaryRound 0 should NOT be shown.
	if strings.Contains(view, "Adversary") {
		t.Error("AdversaryRound 0 should not be shown")
	}
}

func TestRenderDetailPanelAdversaryRound(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.height = 60
	m.width = 100
	m.showDetail = true
	m.plans = []PlanView{{Name: "plan", Phases: []PhaseView{{
		Name:           "phase",
		Status:         "adversary",
		AdversaryRound: 2,
	}}}}

	view := m.renderDetailPanel()
	if !strings.Contains(view, "Adversary") {
		t.Error("AdversaryRound > 0 should be shown")
	}
	if !strings.Contains(view, "round 2") {
		t.Error("missing adversary round number")
	}
}

func TestRenderDetailPanelNoOptionalSections(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.height = 50
	m.plans = []PlanView{{Name: "plan", Phases: []PhaseView{{Name: "minimal", Status: "pending"}}}}
	m.showDetail = true

	view := m.renderDetailPanel()
	if strings.Contains(view, "Adversary") {
		t.Error("should not show adversary section when round is 0")
	}
}

func TestRenderDetailPanelScroll(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.height = 5 // very small to force scrolling
	m.plans = []PlanView{{Name: "plan", Phases: []PhaseView{{
		Name:              "scrollable",
		Status:            "implementing",
		Iteration:         3,
		MaxIteration:      25,
		TestsPassing:      5,
		TestsTotal:        10,
		InputTokens:       10000,
		OutputTokens:      2000,
		LastCommit:        "abc1234",
		Notes:             "some notes here",
		Activity:          "running tests",
		ActivityUpdatedAt: "2025-01-15T14:02:30Z",
		BlockedReason:     "dep failed",
		AdversaryRound:    1,
	}}}}
	m.showDetail = true
	m.detailScroll = 2

	view := m.renderDetailPanel()
	if !strings.Contains(view, "line") {
		t.Error("missing scroll indicator")
	}
}

func TestRenderDetailPanelOutOfBounds(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.height = 50
	m.plans = []PlanView{}
	m.selectedIdx = 0
	view := m.renderDetailPanel()
	if view != "" {
		t.Errorf("expected empty string for out-of-bounds selectedIdx, got %q", view)
	}
}

func TestRenderDetailPanelDeferredReason(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.height = 50
	m.plans = []PlanView{{Name: "plan", Phases: []PhaseView{{
		Name:           "deferred-phase",
		Status:         "deferred",
		DeferredReason: "waiting on dependency",
	}}}}
	m.showDetail = true

	view := m.renderDetailPanel()
	if !strings.Contains(view, "waiting on dependency") {
		t.Error("detail panel should show deferred reason")
	}
}

func TestRenderDetailPanelActivity(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.height = 50
	m.plans = []PlanView{{Name: "plan", Phases: []PhaseView{{
		Name:              "active-phase",
		Status:            "implementing",
		Activity:          "running test suite",
		ActivityUpdatedAt: "2025-01-15T14:02:30Z",
	}}}}
	m.showDetail = true

	view := m.renderDetailPanel()
	if !strings.Contains(view, "running test suite") {
		t.Error("detail panel should show activity")
	}
	if !strings.Contains(view, "14:02:30") {
		t.Error("detail panel should show activity timestamp")
	}
}

func TestRenderDetailPanelActivityDashForActive(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.height = 50
	m.plans = []PlanView{{Name: "plan", Phases: []PhaseView{{
		Name:     "active-phase",
		Status:   "implementing",
		Activity: "",
	}}}}
	m.showDetail = true

	view := m.renderDetailPanel()
	if !strings.Contains(view, "Activity") {
		t.Error("detail panel should show Activity line for active phase")
	}
}

func TestRenderDetailPanelScrollClamp(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.height = 50
	m.plans = []PlanView{{Name: "plan", Phases: []PhaseView{{Name: "phase", Status: "pending"}}}}
	m.showDetail = true
	m.detailScroll = 9999

	view := m.renderDetailPanel()
	if view == "" {
		t.Error("expected non-empty view even with scroll past end")
	}
}

// --- Header ---

func TestRenderHeader(t *testing.T) {
	m := NewModel("test-plan", "/tmp")
	m.plans = []PlanView{{
		Name:   "test-plan",
		Phases: []PhaseView{{Status: "complete"}, {Status: "pending"}, {Status: "pending"}},
		Meta:   planSummary{CompletedCount: 1},
	}}
	header := m.renderHeader()
	if !strings.Contains(header, "test-plan") {
		t.Error("header missing plan name")
	}
	if !strings.Contains(header, "1/3") {
		t.Error("header missing progress")
	}
}

func TestRenderHeaderWithIterations(t *testing.T) {
	m := NewModel("plan", "/tmp")
	m.plans = []PlanView{{
		Meta: planSummary{
			TotalIterations: 47,
			RunningCount:    2,
		},
	}}
	header := m.renderHeader()
	if !strings.Contains(header, "47 iter") {
		t.Error("header missing total iterations")
	}
}

func TestRenderHeaderWithBlocked(t *testing.T) {
	m := NewModel("plan", "/tmp")
	m.plans = []PlanView{{
		Meta: planSummary{
			FailedCount: 2,
		},
	}}
	header := m.renderHeader()
	if !strings.Contains(header, "2 blocked") {
		t.Error("header missing blocked count")
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
	if !strings.Contains(header, "ITER") {
		t.Error("missing ITER column")
	}
	if !strings.Contains(header, "TESTS") {
		t.Error("missing TESTS column")
	}
	if !strings.Contains(header, "TOKENS") {
		t.Error("missing TOKENS column")
	}
}

func TestRenderColumnHeaderNoStateVerdict(t *testing.T) {
	header := renderColumnHeader(100)
	if strings.Contains(header, "STATE") {
		t.Error("STATE column should not exist")
	}
	if strings.Contains(header, "VERDICT") {
		t.Error("VERDICT column should not exist")
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

// --- Phase table ---

func TestRenderPhaseTable(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.plans = []PlanView{{Name: "plan", Phases: []PhaseView{
		{Name: "a", Icon: "[ ]"},
		{Name: "b", Icon: "[>]"},
	}}}
	m.width = 100
	table := m.renderPhaseTable()
	if !strings.Contains(table, "a") || !strings.Contains(table, "b") {
		t.Error("table missing phases")
	}
	if !strings.Contains(table, "PHASE") {
		t.Error("table missing column header")
	}
}

func TestRenderPhaseTableNoplans(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.plans = []PlanView{}
	m.width = 100
	table := m.renderPhaseTable()
	if !strings.Contains(table, "No phases") {
		t.Error("expected 'No phases' message for empty plans")
	}
}

func TestRenderPhaseTablePlansWithNoPhases(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.plans = []PlanView{{Name: "empty-plan", Phases: []PhaseView{}}}
	m.width = 100
	table := m.renderPhaseTable()
	if !strings.Contains(table, "empty-plan") {
		t.Error("should show plan header even with no phases")
	}
}

func TestRenderPhaseTableMultiplePlans(t *testing.T) {
	m := NewModel("", "/tmp")
	m.plans = []PlanView{
		{Name: "plan-a", Phases: []PhaseView{{Name: "p1", Icon: "[ ]"}}},
		{Name: "plan-b", Phases: []PhaseView{{Name: "p2", Icon: "[>]"}}},
	}
	m.width = 100
	table := m.renderPhaseTable()
	if !strings.Contains(table, "plan-a") {
		t.Error("table missing plan-a header")
	}
	if !strings.Contains(table, "plan-b") {
		t.Error("table missing plan-b header")
	}
	if !strings.Contains(table, "p1") || !strings.Contains(table, "p2") {
		t.Error("table missing phase names")
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

// --- formatIter ---

func TestFormatIter(t *testing.T) {
	tests := []struct {
		pv   PhaseView
		want string
	}{
		{PhaseView{Iteration: 0, MaxIteration: 0}, "—"},
		{PhaseView{Iteration: 3, MaxIteration: 25}, "3/25"},
		{PhaseView{Iteration: 0, MaxIteration: 25}, "0/25"},
		{PhaseView{MaxIteration: 25}, "0/25"},
	}
	for _, tc := range tests {
		got := formatIter(tc.pv)
		if got != tc.want {
			t.Errorf("formatIter(%+v)=%q, want %q", tc.pv, got, tc.want)
		}
	}
}

// --- formatTests ---

func TestFormatTests(t *testing.T) {
	tests := []struct {
		pv   PhaseView
		want string
	}{
		{PhaseView{TestsTotal: 0}, "—"},
		{PhaseView{TestsPassing: 7, TestsTotal: 12}, "7/12"},
		{PhaseView{TestsPassing: 0, TestsTotal: 5}, "0/5"},
	}
	for _, tc := range tests {
		got := formatTests(tc.pv)
		if got != tc.want {
			t.Errorf("formatTests(%+v)=%q, want %q", tc.pv, got, tc.want)
		}
	}
}

// --- isActiveStatus ---

func TestIsActiveStatus(t *testing.T) {
	inactive := []string{"", "pending", "complete", "blocked", "deferred", "split", "disputed"}
	for _, s := range inactive {
		if isActiveStatus(s) {
			t.Errorf("isActiveStatus(%q) = true, want false", s)
		}
	}
	active := []string{"implementing", "running", "act", "tests"}
	for _, s := range active {
		if !isActiveStatus(s) {
			t.Errorf("isActiveStatus(%q) = false, want true", s)
		}
	}
}

// --- phaseNameWidth breakpoints ---

func TestPhaseNameWidthBreakpoints(t *testing.T) {
	if w := phaseNameWidth(50); w != 15 {
		t.Errorf("phaseNameWidth(50)=%d, want 15", w)
	}
	if w := phaseNameWidth(80); w != 20 {
		t.Errorf("phaseNameWidth(80)=%d, want 20", w)
	}
	if w := phaseNameWidth(100); w != 24 {
		t.Errorf("phaseNameWidth(100)=%d, want 24", w)
	}
}

// --- Refresh reads files ---

func TestRefreshReadsFiles(t *testing.T) {
	parentDir := t.TempDir()
	planDir := filepath.Join(parentDir, "test-plan")
	os.MkdirAll(planDir, 0755)

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

	m := NewModel("test-plan", parentDir)
	msg := m.refresh()
	rm, ok := msg.(refreshMsg)
	if !ok {
		t.Fatalf("expected refreshMsg, got %T", msg)
	}
	if len(rm.plans) != 1 {
		t.Fatalf("plans=%d, want 1", len(rm.plans))
	}
	if len(rm.plans[0].Phases) != 2 {
		t.Fatalf("phases=%d, want 2", len(rm.plans[0].Phases))
	}
	if rm.plans[0].Phases[0].Status != "implementing" {
		t.Errorf("phase 0 status=%q", rm.plans[0].Phases[0].Status)
	}
	if rm.plans[0].Phases[0].InputTokens != 10000 {
		t.Errorf("phase 0 input tokens=%d, want 10000", rm.plans[0].Phases[0].InputTokens)
	}
}

func TestRefreshReadsMultiplePlans(t *testing.T) {
	parentDir := t.TempDir()

	for _, planName := range []string{"plan-a", "plan-b"} {
		planDir := filepath.Join(parentDir, planName)
		os.MkdirAll(planDir, 0755)
		meta := arc.NewPlanMeta(planName, "feature", []string{"phase1"})
		metaData, _ := json.MarshalIndent(meta, "", "  ")
		os.WriteFile(filepath.Join(planDir, "plan.json"), metaData, 0644)

		phaseDir := filepath.Join(planDir, "phases", "phase1")
		os.MkdirAll(phaseDir, 0755)
		ps := arc.NewPhaseState(planName, "phase1", "feature")
		ps.PhaseStatus = "pending"
		data, _ := json.MarshalIndent(ps, "", "  ")
		os.WriteFile(filepath.Join(phaseDir, "state.json"), data, 0644)
	}

	m := NewModel("", parentDir) // empty filter = all plans
	msg := m.refresh()
	rm := msg.(refreshMsg)
	if len(rm.plans) != 2 {
		t.Fatalf("plans=%d, want 2", len(rm.plans))
	}
}

func TestRefreshWithFilter(t *testing.T) {
	parentDir := t.TempDir()

	for _, planName := range []string{"plan-a", "plan-b"} {
		planDir := filepath.Join(parentDir, planName)
		os.MkdirAll(planDir, 0755)
		meta := arc.NewPlanMeta(planName, "feature", []string{"phase1"})
		metaData, _ := json.MarshalIndent(meta, "", "  ")
		os.WriteFile(filepath.Join(planDir, "plan.json"), metaData, 0644)

		phaseDir := filepath.Join(planDir, "phases", "phase1")
		os.MkdirAll(phaseDir, 0755)
		ps := arc.NewPhaseState(planName, "phase1", "feature")
		data, _ := json.MarshalIndent(ps, "", "  ")
		os.WriteFile(filepath.Join(phaseDir, "state.json"), data, 0644)
	}

	m := NewModel("plan-a", parentDir) // filter to plan-a only
	msg := m.refresh()
	rm := msg.(refreshMsg)
	if len(rm.plans) != 1 {
		t.Fatalf("plans=%d, want 1", len(rm.plans))
	}
	if rm.plans[0].Name != "plan-a" {
		t.Errorf("plan name=%q, want 'plan-a'", rm.plans[0].Name)
	}
}

func TestRefreshFilterMatchesNone(t *testing.T) {
	parentDir := t.TempDir()

	planDir := filepath.Join(parentDir, "plan-a")
	os.MkdirAll(planDir, 0755)
	meta := arc.NewPlanMeta("plan-a", "feature", []string{"phase1"})
	metaData, _ := json.MarshalIndent(meta, "", "  ")
	os.WriteFile(filepath.Join(planDir, "plan.json"), metaData, 0644)

	phaseDir := filepath.Join(planDir, "phases", "phase1")
	os.MkdirAll(phaseDir, 0755)
	ps := arc.NewPhaseState("plan-a", "phase1", "feature")
	data, _ := json.MarshalIndent(ps, "", "  ")
	os.WriteFile(filepath.Join(phaseDir, "state.json"), data, 0644)

	m := NewModel("nonexistent-plan", parentDir)
	msg := m.refresh()
	rm := msg.(refreshMsg)
	if len(rm.plans) != 0 {
		t.Errorf("plans=%d, want 0 (filter should match nothing)", len(rm.plans))
	}
}

func TestRefreshHandlesPlanJSONReadError(t *testing.T) {
	parentDir := t.TempDir()
	// Create a directory but no plan.json inside it.
	os.MkdirAll(filepath.Join(parentDir, "plan-a"), 0755)

	m := NewModel("", parentDir)
	msg := m.refresh()
	rm := msg.(refreshMsg)
	// plan-a has no plan.json, so it should be skipped.
	if len(rm.plans) != 0 {
		t.Errorf("plans=%d, want 0 (no plan.json)", len(rm.plans))
	}
}

func TestRefreshHandlesPlanJSONUnmarshalError(t *testing.T) {
	parentDir := t.TempDir()
	planDir := filepath.Join(parentDir, "plan-a")
	os.MkdirAll(planDir, 0755)
	os.WriteFile(filepath.Join(planDir, "plan.json"), []byte("not json"), 0644)

	m := NewModel("", parentDir)
	msg := m.refresh()
	rm := msg.(refreshMsg)
	if len(rm.plans) != 0 {
		t.Errorf("plans=%d, want 0 (invalid plan.json)", len(rm.plans))
	}
}

func TestRefreshHandlesStateJSONUnmarshalError(t *testing.T) {
	parentDir := t.TempDir()
	planDir := filepath.Join(parentDir, "plan-a")
	os.MkdirAll(planDir, 0755)
	meta := arc.NewPlanMeta("plan-a", "feature", []string{"bad-phase"})
	metaData, _ := json.MarshalIndent(meta, "", "  ")
	os.WriteFile(filepath.Join(planDir, "plan.json"), metaData, 0644)

	phaseDir := filepath.Join(planDir, "phases", "bad-phase")
	os.MkdirAll(phaseDir, 0755)
	os.WriteFile(filepath.Join(phaseDir, "state.json"), []byte("not json"), 0644)

	m := NewModel("", parentDir)
	msg := m.refresh()
	rm := msg.(refreshMsg)
	if len(rm.plans) != 1 {
		t.Fatalf("plans=%d, want 1", len(rm.plans))
	}
	if len(rm.plans[0].Phases) != 1 {
		t.Fatalf("phases=%d, want 1", len(rm.plans[0].Phases))
	}
	if rm.plans[0].Phases[0].Status != "error" {
		t.Errorf("status=%q, want 'error'", rm.plans[0].Phases[0].Status)
	}
	if rm.plans[0].Phases[0].Icon != "[?]" {
		t.Errorf("icon=%q, want '[?]'", rm.plans[0].Phases[0].Icon)
	}
}

func TestRefreshMissingStateFile(t *testing.T) {
	parentDir := t.TempDir()
	planDir := filepath.Join(parentDir, "plan-a")
	os.MkdirAll(planDir, 0755)
	meta := arc.NewPlanMeta("plan-a", "feature", []string{"missing-phase"})
	metaData, _ := json.MarshalIndent(meta, "", "  ")
	os.WriteFile(filepath.Join(planDir, "plan.json"), metaData, 0644)

	m := NewModel("", parentDir)
	msg := m.refresh()
	rm := msg.(refreshMsg)
	if len(rm.plans) != 1 {
		t.Fatalf("plans=%d, want 1", len(rm.plans))
	}
	if rm.plans[0].Phases[0].Status != "unknown" {
		t.Errorf("status=%q, want 'unknown'", rm.plans[0].Phases[0].Status)
	}
	if rm.plans[0].Phases[0].Icon != "[?]" {
		t.Errorf("icon=%q, want '[?]'", rm.plans[0].Phases[0].Icon)
	}
}

// --- Narrow rendering ---

func TestRenderPhaseRowVeryNarrow(t *testing.T) {
	pv := PhaseView{
		Name:     "core",
		Status:   "implementing",
		Icon:     "[>]",
		Iteration: 5, MaxIteration: 25,
		TestsPassing: 7, TestsTotal: 12,
	}
	row := renderPhaseRow(pv, 70, false)
	if !strings.Contains(row, "[>]") {
		t.Error("narrow: missing icon")
	}
	if !strings.Contains(row, "core") {
		t.Error("narrow: missing name")
	}
	// Iter and tests columns should not appear in narrow mode.
	if strings.Contains(row, "5/25") {
		t.Error("narrow: iter column should be hidden")
	}
	if strings.Contains(row, "7/12") {
		t.Error("narrow: tests column should be hidden")
	}
}
