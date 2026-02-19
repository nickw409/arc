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
}

func TestNewModelEmptyPlanName(t *testing.T) {
	m := NewModel("", "/tmp/plans/empty")
	if m.planName != "" {
		t.Errorf("planName=%q, want empty", m.planName)
	}
	// Should not panic when rendering
	_ = m.View()
}

func TestPhaseViewFromState(t *testing.T) {
	ps := &arc.PhaseState{
		Phase:       "core",
		PhaseStatus: "implementing",
		Iteration:   arc.Iteration{Current: 5, Max: 25},
		TestsPassing: 3,
		TestsTotal:   10,
		Disputes:    []arc.Dispute{{TestName: "t1", Reason: "r"}},
		LastVerdict:  "concerns",
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

func TestMonitorUpdateResize(t *testing.T) {
	m := NewModel("test", "/tmp")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := updated.(Model)
	if model.width != 120 || model.height != 40 {
		t.Errorf("size=%dx%d, want 120x40", model.width, model.height)
	}
}

func TestMonitorUpdateRefreshMsg(t *testing.T) {
	m := NewModel("test", "/tmp")
	phases := []PhaseView{
		{Name: "a", Status: "pending"},
		{Name: "b", Status: "implementing"},
		{Name: "c", Status: "complete"},
	}
	updated, _ := m.Update(refreshMsg{phases: phases})
	model := updated.(Model)
	if len(model.phases) != 3 {
		t.Errorf("phases count=%d, want 3", len(model.phases))
	}
	if model.lastUpdate.IsZero() {
		t.Error("lastUpdate not set")
	}
}

func TestMonitorUpdateTick(t *testing.T) {
	m := NewModel("test", "/tmp")
	_, cmd := m.Update(tickMsg{})
	if cmd == nil {
		t.Error("expected refresh command from tick")
	}
}

func TestMonitorViewRendersAllPhases(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.phases = []PhaseView{
		{Name: "a", Status: "pending", Icon: "[ ]"},
		{Name: "b", Status: "implementing", Icon: "[>]"},
		{Name: "c", Status: "complete", Icon: "[x]"},
	}
	m.width = 80

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

func TestRenderPhaseRowInProgress(t *testing.T) {
	pv := PhaseView{Name: "core", Status: "implementing", Icon: "[>]", Iteration: 5, TestsPassing: 7, TestsTotal: 12}
	row := renderPhaseRow(pv, 80)
	if !strings.Contains(row, "[>]") {
		t.Error("missing icon")
	}
	if !strings.Contains(row, "core") {
		t.Error("missing name")
	}
	if !strings.Contains(row, "iter 5") {
		t.Error("missing iteration")
	}
	if !strings.Contains(row, "7/12") {
		t.Error("missing test counts")
	}
}

func TestRenderPhaseRowComplete(t *testing.T) {
	pv := PhaseView{Name: "core", Status: "complete", Icon: "[x]"}
	row := renderPhaseRow(pv, 80)
	if !strings.Contains(row, "[x]") || !strings.Contains(row, "complete") {
		t.Error("missing completion info")
	}
}

func TestRenderPhaseRowNarrowTerminal(t *testing.T) {
	pv := PhaseView{Name: "core", Status: "implementing", Icon: "[>]", Iteration: 5, TestsPassing: 7, TestsTotal: 12}
	row := renderPhaseRow(pv, 50)
	// Tests should be hidden in narrow mode
	if strings.Contains(row, "7/12") {
		t.Error("test counts should be hidden in narrow terminal")
	}
}

func TestRenderPhaseRowVeryLongName(t *testing.T) {
	pv := PhaseView{Name: "a-very-long-phase-name-that-exceeds-column-width-significantly", Status: "implementing", Icon: "[>]"}
	row := renderPhaseRow(pv, 80)
	// Name should be truncated
	if strings.Contains(row, "significantly") {
		t.Error("name should be truncated")
	}
}

func TestRenderPhaseRowZeroWidth(t *testing.T) {
	pv := PhaseView{Name: "core", Status: "implementing", Icon: "[>]"}
	row := renderPhaseRow(pv, 0)
	// Should not panic, should use default
	if row == "" {
		t.Error("expected non-empty row")
	}
}

func TestStatusColorMapping(t *testing.T) {
	// Just verify these don't panic
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
	// Default should be white-ish
	if c.Light == "" && c.Dark == "" {
		t.Error("expected non-empty color for default")
	}
}

func TestRenderHeader(t *testing.T) {
	m := NewModel("test-plan", "/tmp")
	m.phases = []PhaseView{
		{Status: "complete"},
		{Status: "pending"},
		{Status: "pending"},
	}
	header := m.renderHeader()
	if !strings.Contains(header, "test-plan") {
		t.Error("header missing plan name")
	}
	if !strings.Contains(header, "1/3") {
		t.Error("header missing progress")
	}
}

func TestRenderPhaseTable(t *testing.T) {
	m := NewModel("test", "/tmp")
	m.phases = []PhaseView{
		{Name: "a", Icon: "[ ]"},
		{Name: "b", Icon: "[>]"},
	}
	m.width = 80
	table := m.renderPhaseTable()
	if !strings.Contains(table, "a") || !strings.Contains(table, "b") {
		t.Error("table missing phases")
	}
}

func TestRenderFooter(t *testing.T) {
	m := NewModel("test", "/tmp")
	footer := m.renderFooter()
	if !strings.Contains(footer, "q: quit") || !strings.Contains(footer, "3s") {
		t.Error("footer missing expected content")
	}
}

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

func TestRefreshReadsFiles(t *testing.T) {
	planDir := t.TempDir()

	// Create plan.json
	meta := arc.NewPlanMeta("test-plan", "feature", []string{"core", "api"})
	metaData, _ := json.MarshalIndent(meta, "", "  ")
	os.WriteFile(filepath.Join(planDir, "plan.json"), metaData, 0644)

	// Create phase state files
	for _, phase := range []string{"core", "api"} {
		phaseDir := filepath.Join(planDir, "phases", phase)
		os.MkdirAll(phaseDir, 0755)
		ps := arc.NewPhaseState("test-plan", phase, "feature")
		ps.PhaseStatus = "implementing"
		ps.Iteration.Current = 3
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
}
