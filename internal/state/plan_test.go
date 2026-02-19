package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nwiley/arc/internal/arc"
)

func TestReadPlan(t *testing.T) {
	dir := t.TempDir()
	meta := arc.NewPlanMeta("test-plan", "feature", []string{"qa", "impl"})

	if err := WritePlan(dir, meta); err != nil {
		t.Fatalf("WritePlan error: %v", err)
	}

	got, err := ReadPlan(dir)
	if err != nil {
		t.Fatalf("ReadPlan error: %v", err)
	}
	if got.Name != "test-plan" {
		t.Fatalf("Name = %q, want %q", got.Name, "test-plan")
	}
	if len(got.Phases) != 2 {
		t.Fatalf("len(Phases) = %d, want 2", len(got.Phases))
	}
	if got.Phases[0] != "qa" || got.Phases[1] != "impl" {
		t.Fatalf("Phases = %v, want [qa impl]", got.Phases)
	}
}

func TestWritePlanAtomic(t *testing.T) {
	dir := t.TempDir()
	meta := arc.NewPlanMeta("test-plan", "feature", []string{"qa"})

	if err := WritePlan(dir, meta); err != nil {
		t.Fatalf("WritePlan error: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir error: %v", err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp.") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
}

func TestWritePlanNonexistentDir(t *testing.T) {
	meta := arc.NewPlanMeta("test-plan", "feature", []string{"qa"})
	err := WritePlan("/nonexistent/dir", meta)
	if err == nil {
		t.Fatal("expected error for nonexistent directory, got nil")
	}
}

func TestNextPhaseAllComplete(t *testing.T) {
	meta := arc.NewPlanMeta("plan", "feature", []string{"a", "b", "c"})
	states := map[string]*arc.PhaseState{
		"a": {PhaseStatus: "complete"},
		"b": {PhaseStatus: "complete"},
		"c": {PhaseStatus: "complete"},
	}

	next := NextPhase(meta, states)
	if next != "" {
		t.Fatalf("NextPhase = %q, want empty", next)
	}
}

func TestNextPhaseRespectsDependencies(t *testing.T) {
	meta := &arc.PlanMeta{
		Phases:       []string{"qa", "impl"},
		PhaseOrder:   map[string]int{"qa": 1, "impl": 2},
		Dependencies: map[string][]string{"impl": {"qa"}},
	}
	states := map[string]*arc.PhaseState{
		"qa":   {PhaseStatus: "pending"},
		"impl": {PhaseStatus: "pending"},
	}

	next := NextPhase(meta, states)
	if next == "impl" {
		t.Fatal("NextPhase should not return 'impl' when its dependency 'qa' is not complete")
	}
}

func TestNextPhaseReturnsFirstReady(t *testing.T) {
	meta := &arc.PlanMeta{
		Phases:       []string{"a", "b", "c"},
		PhaseOrder:   map[string]int{"a": 1, "b": 2, "c": 3},
		Dependencies: map[string][]string{},
	}
	states := map[string]*arc.PhaseState{
		"a": {PhaseStatus: "complete"},
		"b": {PhaseStatus: "pending"},
		"c": {PhaseStatus: "pending"},
	}

	next := NextPhase(meta, states)
	if next != "b" {
		t.Fatalf("NextPhase = %q, want %q (first ready in phase order)", next, "b")
	}
}

func TestNextPhaseSinglePendingNoDeps(t *testing.T) {
	meta := &arc.PlanMeta{
		Phases:       []string{"a"},
		PhaseOrder:   map[string]int{"a": 1},
		Dependencies: map[string][]string{},
	}
	states := map[string]*arc.PhaseState{
		"a": {PhaseStatus: "pending"},
	}

	next := NextPhase(meta, states)
	if next != "a" {
		t.Fatalf("NextPhase = %q, want %q", next, "a")
	}
}

func TestNextPhaseAllBlocked(t *testing.T) {
	meta := &arc.PlanMeta{
		Phases:       []string{"a", "b"},
		PhaseOrder:   map[string]int{"a": 1, "b": 2},
		Dependencies: map[string][]string{},
	}
	states := map[string]*arc.PhaseState{
		"a": {PhaseStatus: "blocked"},
		"b": {PhaseStatus: "blocked"},
	}

	next := NextPhase(meta, states)
	if next != "" {
		t.Fatalf("NextPhase = %q, want empty (all blocked)", next)
	}
}

func TestNextPhaseEmptyStates(t *testing.T) {
	meta := &arc.PlanMeta{
		Phases:       []string{"a", "b"},
		PhaseOrder:   map[string]int{"a": 1, "b": 2},
		Dependencies: map[string][]string{"b": {"a"}},
	}
	states := map[string]*arc.PhaseState{}

	next := NextPhase(meta, states)
	if next != "" {
		t.Fatalf("NextPhase = %q, want empty (all nil states)", next)
	}
}

func TestNextPhasePartialNilStates(t *testing.T) {
	meta := &arc.PlanMeta{
		Phases:       []string{"core", "api"},
		PhaseOrder:   map[string]int{"core": 1, "api": 2},
		Dependencies: map[string][]string{"api": {"core"}},
	}
	states := map[string]*arc.PhaseState{
		"core": nil,
		"api":  {PhaseStatus: "pending"},
	}

	next := NextPhase(meta, states)
	if next != "" {
		t.Fatalf("NextPhase = %q, want empty (core is nil, api depends on core)", next)
	}
}

func TestPhasesReady(t *testing.T) {
	meta := &arc.PlanMeta{
		Phases:       []string{"a", "b", "c"},
		PhaseOrder:   map[string]int{"a": 1, "b": 2, "c": 3},
		Dependencies: map[string][]string{"b": {"a"}, "c": {"a", "b"}},
	}
	states := map[string]*arc.PhaseState{
		"a": {PhaseStatus: "complete"},
		"b": {PhaseStatus: "pending"},
		"c": {PhaseStatus: "pending"},
	}

	ready := PhasesReady(meta, states)
	if len(ready) != 1 {
		t.Fatalf("len(PhasesReady) = %d, want 1", len(ready))
	}
	if ready[0] != "b" {
		t.Fatalf("PhasesReady[0] = %q, want %q", ready[0], "b")
	}
}

func TestPhasesReadyMultiple(t *testing.T) {
	meta := &arc.PlanMeta{
		Phases:       []string{"a", "b", "c"},
		PhaseOrder:   map[string]int{"a": 1, "b": 2, "c": 3},
		Dependencies: map[string][]string{"c": {"a", "b"}},
	}
	states := map[string]*arc.PhaseState{
		"a": {PhaseStatus: "complete"},
		"b": {PhaseStatus: "complete"},
		"c": {PhaseStatus: "pending"},
	}

	ready := PhasesReady(meta, states)
	if len(ready) != 1 {
		t.Fatalf("len(PhasesReady) = %d, want 1", len(ready))
	}
	if ready[0] != "c" {
		t.Fatalf("PhasesReady[0] = %q, want %q", ready[0], "c")
	}
}

func TestPhasesReadyBlockedStatus(t *testing.T) {
	meta := &arc.PlanMeta{
		Phases:       []string{"a", "b"},
		PhaseOrder:   map[string]int{"a": 1, "b": 2},
		Dependencies: map[string][]string{"b": {"a"}},
	}
	states := map[string]*arc.PhaseState{
		"a": {PhaseStatus: "blocked"},
		"b": {PhaseStatus: "pending"},
	}

	ready := PhasesReady(meta, states)
	if len(ready) != 0 {
		t.Fatalf("len(PhasesReady) = %d, want 0 (a is blocked, b depends on a)", len(ready))
	}
}

func TestPhasesReadyEmptyStates(t *testing.T) {
	meta := &arc.PlanMeta{
		Phases:       []string{"a", "b"},
		PhaseOrder:   map[string]int{"a": 1, "b": 2},
		Dependencies: map[string][]string{"b": {"a"}},
	}
	states := map[string]*arc.PhaseState{}

	ready := PhasesReady(meta, states)
	if len(ready) != 0 {
		t.Fatalf("len(PhasesReady) = %d, want 0 (no known states)", len(ready))
	}
}

func TestPhasesReadyFirstPhasePending(t *testing.T) {
	meta := &arc.PlanMeta{
		Phases:       []string{"a", "b"},
		PhaseOrder:   map[string]int{"a": 1, "b": 2},
		Dependencies: map[string][]string{"b": {"a"}},
	}
	states := map[string]*arc.PhaseState{
		"a": {PhaseStatus: "pending"},
		"b": {PhaseStatus: "pending"},
	}

	ready := PhasesReady(meta, states)
	if len(ready) != 1 {
		t.Fatalf("len(PhasesReady) = %d, want 1", len(ready))
	}
	if ready[0] != "a" {
		t.Fatalf("PhasesReady[0] = %q, want %q", ready[0], "a")
	}
}

func TestReadPlanFromWritten(t *testing.T) {
	dir := t.TempDir()
	meta := arc.NewPlanMeta("my-plan", "feature", []string{"phase1", "phase2", "phase3"})

	if err := WritePlan(dir, meta); err != nil {
		t.Fatalf("WritePlan error: %v", err)
	}

	got, err := ReadPlan(dir)
	if err != nil {
		t.Fatalf("ReadPlan error: %v", err)
	}

	if got.Name != meta.Name {
		t.Fatalf("Name = %q, want %q", got.Name, meta.Name)
	}
	if got.WorkflowType != meta.WorkflowType {
		t.Fatalf("WorkflowType = %q, want %q", got.WorkflowType, meta.WorkflowType)
	}
	if len(got.Phases) != len(meta.Phases) {
		t.Fatalf("len(Phases) = %d, want %d", len(got.Phases), len(meta.Phases))
	}

	// Check plan.json file exists
	planPath := filepath.Join(dir, "plan.json")
	if _, err := os.Stat(planPath); os.IsNotExist(err) {
		t.Fatal("plan.json file not created")
	}
}
