package plan

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/state"
)

func boolPtr(b bool) *bool { return &b }

// makeTestPlan creates a plan in a temp directory and returns plansDir.
func makeTestPlan(t *testing.T, planName string, phases []string) string {
	t.Helper()
	plansDir := t.TempDir()
	_, err := Create(CreateOptions{
		PlansDir: plansDir,
		Name:     planName,
		Phases:   phases,
	})
	if err != nil {
		t.Fatalf("Create plan: %v", err)
	}
	return plansDir
}

func TestWriteReadSpec(t *testing.T) {
	plansDir := makeTestPlan(t, "my-plan", []string{"phase-a"})

	spec := &arc.PhaseSpec{
		Spec:       "Implement the feature",
		Verify:     "go test ./...",
		Complexity: "medium",
		Files:      []string{"internal/foo/bar.go", "internal/foo/baz.go"},
		Deps:       []string{"phase-b"},
		Gate: arc.GateSpec{
			Assertions: []arc.GateAssertion{
				{Type: "file_exists", Target: "internal/foo/bar.go", FileExists: "internal/foo/bar.go"},
			},
			VerifierAgent: boolPtr(true),
		},
	}

	if err := WriteSpec(plansDir, "my-plan", "phase-a", spec); err != nil {
		t.Fatalf("WriteSpec: %v", err)
	}

	got, err := ReadSpec(plansDir, "my-plan", "phase-a")
	if err != nil {
		t.Fatalf("ReadSpec: %v", err)
	}

	if got.Spec != spec.Spec {
		t.Errorf("Spec mismatch: got %q, want %q", got.Spec, spec.Spec)
	}
	if got.Verify != spec.Verify {
		t.Errorf("Verify mismatch: got %q, want %q", got.Verify, spec.Verify)
	}
	if got.Complexity != spec.Complexity {
		t.Errorf("Complexity mismatch: got %q, want %q", got.Complexity, spec.Complexity)
	}
	if len(got.Files) != len(spec.Files) {
		t.Errorf("Files length mismatch: got %d, want %d", len(got.Files), len(spec.Files))
	} else {
		for i, f := range spec.Files {
			if got.Files[i] != f {
				t.Errorf("Files[%d]: got %q, want %q", i, got.Files[i], f)
			}
		}
	}
	if len(got.Deps) != len(spec.Deps) {
		t.Errorf("Deps length mismatch: got %d, want %d", len(got.Deps), len(spec.Deps))
	}
	if len(got.Gate.Assertions) != len(spec.Gate.Assertions) {
		t.Errorf("Gate.Assertions length mismatch: got %d, want %d", len(got.Gate.Assertions), len(spec.Gate.Assertions))
	} else if len(spec.Gate.Assertions) > 0 && got.Gate.Assertions[0] != spec.Gate.Assertions[0] {
		t.Errorf("Gate.Assertions[0]: got %q, want %q", got.Gate.Assertions[0], spec.Gate.Assertions[0])
	}
	wantVA := spec.Gate.VerifierAgent != nil && *spec.Gate.VerifierAgent
	gotVA := got.Gate.VerifierAgent != nil && *got.Gate.VerifierAgent
	if gotVA != wantVA {
		t.Errorf("Gate.VerifierAgent: got %v, want %v", gotVA, wantVA)
	}
}

func TestAddPhase(t *testing.T) {
	plansDir := makeTestPlan(t, "my-plan", []string{"phase-a"})

	spec := &arc.PhaseSpec{
		Spec:       "Add a second phase",
		Complexity: "simple",
		Deps:       []string{"phase-a"},
	}

	if err := AddPhase(plansDir, "my-plan", "phase-b", spec); err != nil {
		t.Fatalf("AddPhase: %v", err)
	}

	// Verify spec was written to plan.md
	got, err := ReadSpec(plansDir, "my-plan", "phase-b")
	if err != nil {
		t.Fatalf("ReadSpec after AddPhase: %v", err)
	}
	if got.Spec != spec.Spec {
		t.Errorf("spec.Spec: got %q, want %q", got.Spec, spec.Spec)
	}

	// Verify state.json was written with pending status
	statePath := filepath.Join(plansDir, "my-plan", "phases", "phase-b", "state.json")
	sf := state.NewStateFile(statePath)
	ps, err := sf.Read()
	if err != nil {
		t.Fatalf("read state.json: %v", err)
	}
	if ps.PhaseStatus != "pending" {
		t.Errorf("PhaseStatus: got %q, want %q", ps.PhaseStatus, "pending")
	}
	if ps.Phase != "phase-b" {
		t.Errorf("Phase: got %q, want %q", ps.Phase, "phase-b")
	}

	// Verify plan.json was updated
	planDir := filepath.Join(plansDir, "my-plan")
	meta, err := state.ReadPlan(planDir)
	if err != nil {
		t.Fatalf("ReadPlan: %v", err)
	}

	found := false
	for _, p := range meta.Phases {
		if p == "phase-b" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("phase-b not in Phases list: %v", meta.Phases)
	}

	if meta.PhaseOrder["phase-b"] == 0 {
		t.Errorf("phase-b missing from PhaseOrder")
	}

	deps, ok := meta.Dependencies["phase-b"]
	if !ok {
		t.Errorf("phase-b missing from Dependencies")
	} else if len(deps) != 1 || deps[0] != "phase-a" {
		t.Errorf("Dependencies[phase-b]: got %v, want [phase-a]", deps)
	}
}

func TestAddPhaseDuplicate(t *testing.T) {
	plansDir := makeTestPlan(t, "my-plan", []string{"phase-a"})

	spec := &arc.PhaseSpec{Spec: "Duplicate phase"}
	err := AddPhase(plansDir, "my-plan", "phase-a", spec)
	if err == nil {
		t.Fatal("expected error adding duplicate phase, got nil")
	}
}

func TestRemovePhase(t *testing.T) {
	plansDir := makeTestPlan(t, "my-plan", []string{"phase-a", "phase-b"})

	if err := RemovePhase(plansDir, "my-plan", "phase-b"); err != nil {
		t.Fatalf("RemovePhase: %v", err)
	}

	// Verify plan.json updated
	planDir := filepath.Join(plansDir, "my-plan")
	meta, err := state.ReadPlan(planDir)
	if err != nil {
		t.Fatalf("ReadPlan: %v", err)
	}

	for _, p := range meta.Phases {
		if p == "phase-b" {
			t.Errorf("phase-b still in Phases list after removal")
		}
	}

	if _, ok := meta.PhaseOrder["phase-b"]; ok {
		t.Errorf("phase-b still in PhaseOrder after removal")
	}

	// phase-a should still be present
	found := false
	for _, p := range meta.Phases {
		if p == "phase-a" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("phase-a missing from Phases after removing phase-b")
	}

	// PhaseOrder for remaining phases should be contiguous
	if meta.PhaseOrder["phase-a"] != 1 {
		t.Errorf("PhaseOrder[phase-a]: got %d, want 1", meta.PhaseOrder["phase-a"])
	}
}

func TestRemovePhasePending(t *testing.T) {
	plansDir := makeTestPlan(t, "my-plan", []string{"phase-a", "phase-b"})

	// Mark phase-b as complete
	statePath := filepath.Join(plansDir, "my-plan", "phases", "phase-b", "state.json")
	sf := state.NewStateFile(statePath)
	if err := sf.Update(func(s *arc.PhaseState) error {
		s.PhaseStatus = "complete"
		return nil
	}); err != nil {
		t.Fatalf("update state: %v", err)
	}

	err := RemovePhase(plansDir, "my-plan", "phase-b")
	if err == nil {
		t.Fatal("expected error removing non-pending phase, got nil")
	}
}

func TestUpdateSpec(t *testing.T) {
	plansDir := makeTestPlan(t, "my-plan", []string{"phase-a"})

	// Write initial spec
	initial := &arc.PhaseSpec{Spec: "original spec", Complexity: "simple"}
	if err := WriteSpec(plansDir, "my-plan", "phase-a", initial); err != nil {
		t.Fatalf("WriteSpec: %v", err)
	}

	// Update with new spec
	updated := &arc.PhaseSpec{Spec: "updated spec", Complexity: "complex"}
	if err := UpdateSpec(plansDir, "my-plan", "phase-a", updated); err != nil {
		t.Fatalf("UpdateSpec: %v", err)
	}

	got, err := ReadSpec(plansDir, "my-plan", "phase-a")
	if err != nil {
		t.Fatalf("ReadSpec: %v", err)
	}
	if got.Spec != "updated spec" {
		t.Errorf("Spec: got %q, want %q", got.Spec, "updated spec")
	}
	if got.Complexity != "complex" {
		t.Errorf("Complexity: got %q, want %q", got.Complexity, "complex")
	}
}

func TestUpdateSpecRunning(t *testing.T) {
	plansDir := makeTestPlan(t, "my-plan", []string{"phase-a"})

	// Mark phase-a as running
	statePath := filepath.Join(plansDir, "my-plan", "phases", "phase-a", "state.json")
	sf := state.NewStateFile(statePath)
	if err := sf.Update(func(s *arc.PhaseState) error {
		s.PhaseStatus = "running"
		return nil
	}); err != nil {
		t.Fatalf("update state: %v", err)
	}

	spec := &arc.PhaseSpec{Spec: "should fail"}
	err := UpdateSpec(plansDir, "my-plan", "phase-a", spec)
	if err == nil {
		t.Fatal("expected error updating running phase, got nil")
	}
}

func TestUpdateGate(t *testing.T) {
	plansDir := makeTestPlan(t, "my-plan", []string{"phase-a"})

	// Write initial spec
	initial := &arc.PhaseSpec{Spec: "phase spec"}
	if err := WriteSpec(plansDir, "my-plan", "phase-a", initial); err != nil {
		t.Fatalf("WriteSpec: %v", err)
	}

	newGate := arc.GateSpec{
		Assertions: []arc.GateAssertion{
			{Type: "file_exists", Target: "go.mod", FileExists: "go.mod"},
			{Type: "grep", Target: "package main", Grep: "package main"},
		},
		VerifierAgent: boolPtr(true),
	}
	if err := UpdateGate(plansDir, "my-plan", "phase-a", newGate); err != nil {
		t.Fatalf("UpdateGate: %v", err)
	}

	got, err := ReadSpec(plansDir, "my-plan", "phase-a")
	if err != nil {
		t.Fatalf("ReadSpec: %v", err)
	}
	if len(got.Gate.Assertions) != 2 {
		t.Errorf("Gate.Assertions length: got %d, want 2", len(got.Gate.Assertions))
	}
	if got.Gate.VerifierAgent == nil || !*got.Gate.VerifierAgent {
		t.Errorf("Gate.VerifierAgent: got false, want true")
	}
	// Original spec field must be preserved
	if got.Spec != "phase spec" {
		t.Errorf("Spec: got %q, want %q (should be preserved)", got.Spec, "phase spec")
	}
}

func TestUpdateDeps(t *testing.T) {
	plansDir := makeTestPlan(t, "my-plan", []string{"phase-a", "phase-b"})

	if err := UpdateDeps(plansDir, "my-plan", "phase-b", []string{"phase-a"}); err != nil {
		t.Fatalf("UpdateDeps: %v", err)
	}

	planDir := filepath.Join(plansDir, "my-plan")
	meta, err := state.ReadPlan(planDir)
	if err != nil {
		t.Fatalf("ReadPlan: %v", err)
	}

	deps := meta.Dependencies["phase-b"]
	if len(deps) != 1 || deps[0] != "phase-a" {
		t.Errorf("Dependencies[phase-b]: got %v, want [phase-a]", deps)
	}
}

// TestUpdateSpecBlockedAllowed verifies that UpdateSpec succeeds on a blocked phase.
func TestUpdateSpecBlockedAllowed(t *testing.T) {
	plansDir := makeTestPlan(t, "my-plan", []string{"phase-a"})

	statePath := filepath.Join(plansDir, "my-plan", "phases", "phase-a", "state.json")
	sf := state.NewStateFile(statePath)
	if err := sf.Update(func(s *arc.PhaseState) error {
		s.PhaseStatus = "blocked"
		return nil
	}); err != nil {
		t.Fatalf("update state: %v", err)
	}

	spec := &arc.PhaseSpec{Spec: "updated on blocked phase"}
	if err := UpdateSpec(plansDir, "my-plan", "phase-a", spec); err != nil {
		t.Fatalf("UpdateSpec on blocked phase should succeed, got: %v", err)
	}
}

// TestUpdateSpecCompleteBlocked verifies that UpdateSpec fails on a complete phase.
func TestUpdateSpecCompleteBlocked(t *testing.T) {
	plansDir := makeTestPlan(t, "my-plan", []string{"phase-a"})

	statePath := filepath.Join(plansDir, "my-plan", "phases", "phase-a", "state.json")
	sf := state.NewStateFile(statePath)
	if err := sf.Update(func(s *arc.PhaseState) error {
		s.PhaseStatus = "complete"
		return nil
	}); err != nil {
		t.Fatalf("update state: %v", err)
	}

	spec := &arc.PhaseSpec{Spec: "should fail"}
	if err := UpdateSpec(plansDir, "my-plan", "phase-a", spec); err == nil {
		t.Fatal("expected error updating complete phase, got nil")
	}
}

// TestUpdateGateRunningBlocked verifies that UpdateGate fails on a running phase.
func TestUpdateGateRunningBlocked(t *testing.T) {
	plansDir := makeTestPlan(t, "my-plan", []string{"phase-a"})

	initial := &arc.PhaseSpec{Spec: "phase spec"}
	if err := WriteSpec(plansDir, "my-plan", "phase-a", initial); err != nil {
		t.Fatalf("WriteSpec: %v", err)
	}

	statePath := filepath.Join(plansDir, "my-plan", "phases", "phase-a", "state.json")
	sf := state.NewStateFile(statePath)
	if err := sf.Update(func(s *arc.PhaseState) error {
		s.PhaseStatus = "running"
		return nil
	}); err != nil {
		t.Fatalf("update state: %v", err)
	}

	gate := arc.GateSpec{VerifierAgent: boolPtr(true)}
	if err := UpdateGate(plansDir, "my-plan", "phase-a", gate); err == nil {
		t.Fatal("expected error updating gate on running phase, got nil")
	}
}

// TestUpdateGateCompleteBlocked verifies that UpdateGate fails on a complete phase.
func TestUpdateGateCompleteBlocked(t *testing.T) {
	plansDir := makeTestPlan(t, "my-plan", []string{"phase-a"})

	initial := &arc.PhaseSpec{Spec: "phase spec"}
	if err := WriteSpec(plansDir, "my-plan", "phase-a", initial); err != nil {
		t.Fatalf("WriteSpec: %v", err)
	}

	statePath := filepath.Join(plansDir, "my-plan", "phases", "phase-a", "state.json")
	sf := state.NewStateFile(statePath)
	if err := sf.Update(func(s *arc.PhaseState) error {
		s.PhaseStatus = "complete"
		return nil
	}); err != nil {
		t.Fatalf("update state: %v", err)
	}

	gate := arc.GateSpec{VerifierAgent: boolPtr(true)}
	if err := UpdateGate(plansDir, "my-plan", "phase-a", gate); err == nil {
		t.Fatal("expected error updating gate on complete phase, got nil")
	}
}

// TestUpdateDepsNonPendingBlocked verifies that UpdateDeps fails on running and
// complete phases.
func TestUpdateDepsNonPendingBlocked(t *testing.T) {
	for _, status := range []string{"running", "complete", "blocked", "deferred"} {
		t.Run(status, func(t *testing.T) {
			plansDir := makeTestPlan(t, "my-plan", []string{"phase-a", "phase-b"})

			statePath := filepath.Join(plansDir, "my-plan", "phases", "phase-b", "state.json")
			sf := state.NewStateFile(statePath)
			if err := sf.Update(func(s *arc.PhaseState) error {
				s.PhaseStatus = status
				return nil
			}); err != nil {
				t.Fatalf("update state: %v", err)
			}

			if err := UpdateDeps(plansDir, "my-plan", "phase-b", []string{"phase-a"}); err == nil {
				t.Fatalf("expected error updating deps on %q phase, got nil", status)
			}
		})
	}
}

func TestAddPhaseWithDeps(t *testing.T) {
	plansDir := makeTestPlan(t, "my-plan", []string{"phase-a", "phase-b"})

	spec := &arc.PhaseSpec{
		Spec: "Phase with multiple deps",
		Deps: []string{"phase-a", "phase-b"},
	}
	if err := AddPhase(plansDir, "my-plan", "phase-c", spec); err != nil {
		t.Fatalf("AddPhase: %v", err)
	}

	planDir := filepath.Join(plansDir, "my-plan")
	meta, err := state.ReadPlan(planDir)
	if err != nil {
		t.Fatalf("ReadPlan: %v", err)
	}

	deps := meta.Dependencies["phase-c"]
	if len(deps) != 2 {
		t.Fatalf("Dependencies[phase-c]: got %v, want [phase-a phase-b]", deps)
	}
	if deps[0] != "phase-a" || deps[1] != "phase-b" {
		t.Errorf("Dependencies[phase-c]: got %v, want [phase-a phase-b]", deps)
	}
}

func TestUpdateSpecMergesFields(t *testing.T) {
	plansDir := makeTestPlan(t, "my-plan", []string{"phase-a"})

	// Write initial spec with all fields populated
	initial := &arc.PhaseSpec{
		Spec:       "original spec",
		Verify:     "go test ./...",
		Complexity: "complex",
		Files:      []string{"main.go", "lib.go"},
		Gate: arc.GateSpec{
			VerifierAgent: boolPtr(true),
			Assertions: []arc.GateAssertion{
				{Type: "file_exists", FileExists: "main.go", Target: "main.go"},
			},
		},
	}
	if err := WriteSpec(plansDir, "my-plan", "phase-a", initial); err != nil {
		t.Fatalf("WriteSpec: %v", err)
	}

	// Update only spec text — everything else should survive
	update := &arc.PhaseSpec{Spec: "updated spec"}
	if err := UpdateSpec(plansDir, "my-plan", "phase-a", update); err != nil {
		t.Fatalf("UpdateSpec: %v", err)
	}

	got, err := ReadSpec(plansDir, "my-plan", "phase-a")
	if err != nil {
		t.Fatalf("ReadSpec: %v", err)
	}
	if got.Spec != "updated spec" {
		t.Errorf("Spec: got %q, want %q", got.Spec, "updated spec")
	}
	if got.Verify != "go test ./..." {
		t.Errorf("Verify was wiped: got %q, want %q", got.Verify, "go test ./...")
	}
	if got.Complexity != "complex" {
		t.Errorf("Complexity was wiped: got %q, want %q", got.Complexity, "complex")
	}
	if len(got.Files) != 2 {
		t.Errorf("Files was wiped: got %v, want [main.go lib.go]", got.Files)
	}
	if got.Gate.VerifierAgent == nil || !*got.Gate.VerifierAgent {
		t.Error("Gate.VerifierAgent was wiped")
	}
	if len(got.Gate.Assertions) != 1 {
		t.Errorf("Gate.Assertions was wiped: got %v", got.Gate.Assertions)
	}
}

func TestUpdateSpecMergesRole(t *testing.T) {
	plansDir := makeTestPlan(t, "my-plan", []string{"phase-a"})

	initial := &arc.PhaseSpec{Spec: "original", Role: "impl", Complexity: "simple"}
	if err := WriteSpec(plansDir, "my-plan", "phase-a", initial); err != nil {
		t.Fatalf("WriteSpec: %v", err)
	}

	// Update only role — other fields should survive
	update := &arc.PhaseSpec{Role: "review"}
	if err := UpdateSpec(plansDir, "my-plan", "phase-a", update); err != nil {
		t.Fatalf("UpdateSpec: %v", err)
	}

	got, err := ReadSpec(plansDir, "my-plan", "phase-a")
	if err != nil {
		t.Fatalf("ReadSpec: %v", err)
	}
	if got.Role != "review" {
		t.Errorf("Role: got %q, want %q", got.Role, "review")
	}
	if got.Spec != "original" {
		t.Errorf("Spec was wiped: got %q, want %q", got.Spec, "original")
	}
}

func TestValidateSpec_InvalidRole(t *testing.T) {
	spec := &arc.PhaseSpec{
		Role:       "unknown",
		Complexity: "simple",
	}
	warnings := ValidateSpec(spec)
	found := false
	for _, w := range warnings {
		if w.Field == "role" {
			found = true
		}
	}
	if !found {
		t.Error("expected warning about invalid role")
	}
}

func TestValidateSpec_ValidRole(t *testing.T) {
	for _, role := range []string{"impl", "review", "investigate", "audit"} {
		spec := &arc.PhaseSpec{
			Role:       role,
			Complexity: "simple",
		}
		warnings := ValidateSpec(spec)
		for _, w := range warnings {
			if w.Field == "role" {
				t.Errorf("unexpected role warning for %q: %s", role, w)
			}
		}
	}
}

func TestValidateSpec_NumberedListTest(t *testing.T) {
	spec := &arc.PhaseSpec{
		Verify: "1. Check this\n2. Check that",
	}
	warnings := ValidateSpec(spec)
	found := false
	for _, w := range warnings {
		if w.Field == "verify" {
			found = true
		}
	}
	if !found {
		t.Error("expected warning about numbered list in test field")
	}
}

func TestValidateSpec_DescriptionTest(t *testing.T) {
	spec := &arc.PhaseSpec{
		Verify: "Recovery with no state file returns nil",
	}
	warnings := ValidateSpec(spec)
	found := false
	for _, w := range warnings {
		if w.Field == "verify" {
			found = true
		}
	}
	if !found {
		t.Error("expected warning about description-like test field")
	}
}

func TestValidateSpec_ValidTest(t *testing.T) {
	spec := &arc.PhaseSpec{
		Verify:     "go test ./... && go build ./...",
		Complexity: "medium",
	}
	warnings := ValidateSpec(spec)
	for _, w := range warnings {
		if w.Field == "verify" {
			t.Errorf("unexpected verify warning: %s", w)
		}
	}
}

func TestValidateSpec_MissingComplexity(t *testing.T) {
	spec := &arc.PhaseSpec{
		Verify: "go test ./...",
	}
	warnings := ValidateSpec(spec)
	found := false
	for _, w := range warnings {
		if w.Field == "complexity" {
			found = true
		}
	}
	if !found {
		t.Error("expected warning about missing complexity")
	}
}

func TestValidateSpec_EmptySpec(t *testing.T) {
	spec := &arc.PhaseSpec{
		Complexity: "simple",
	}
	warnings := ValidateSpec(spec)
	found := false
	for _, w := range warnings {
		if w.Field == "spec" && strings.Contains(w.Message, "empty") {
			found = true
		}
	}
	if !found {
		t.Error("expected warning about empty spec content")
	}
}

func TestValidateSpec_FilesNoGate(t *testing.T) {
	// Files listed without explicit gate assertions should NOT produce a warning
	// because gate.Run auto-derives file_exists assertions from spec.Files.
	spec := &arc.PhaseSpec{
		Files:      []string{"main.go"},
		Complexity: "simple",
	}
	warnings := ValidateSpec(spec)
	for _, w := range warnings {
		if w.Field == "gate" && strings.Contains(w.Message, "file_exists") {
			t.Errorf("unexpected warning about files with no gate assertions: %s", w.Message)
		}
	}
}

// ---------------------------------------------------------------------------
// ValidateSpec — promises tests
// ---------------------------------------------------------------------------

func TestValidateSpec_Promise_NoField(t *testing.T) {
	spec := &arc.PhaseSpec{
		Spec:       "do something",
		Complexity: "simple",
		Promises:   []arc.Promise{{}},
	}
	warnings := ValidateSpec(spec)
	found := false
	for _, w := range warnings {
		if w.Field == "promises" && w.Fatal {
			found = true
		}
	}
	if !found {
		t.Error("expected fatal warning for promise with no field")
	}
}

func TestValidateSpec_Promise_MultipleFields(t *testing.T) {
	spec := &arc.PhaseSpec{
		Spec:       "do something",
		Complexity: "simple",
		Promises:   []arc.Promise{{FuncExists: "func A()", TestExists: "TestA"}},
	}
	warnings := ValidateSpec(spec)
	found := false
	for _, w := range warnings {
		if w.Field == "promises" && w.Fatal {
			found = true
		}
	}
	if !found {
		t.Error("expected fatal warning for promise with multiple fields")
	}
}

func TestValidateSpec_Promise_AllFieldsSet(t *testing.T) {
	spec := &arc.PhaseSpec{
		Spec:       "do something",
		Complexity: "simple",
		Promises:   []arc.Promise{{FuncExists: "func A()", TestExists: "TestA", FileExists: "a.go"}},
	}
	warnings := ValidateSpec(spec)
	found := false
	for _, w := range warnings {
		if w.Field == "promises" && w.Fatal {
			found = true
		}
	}
	if !found {
		t.Error("expected fatal warning for promise with all fields set")
	}
}

func TestValidateSpec_Promise_TestCovers_NoTest(t *testing.T) {
	spec := &arc.PhaseSpec{
		Spec:       "do something",
		Complexity: "simple",
		Promises:   []arc.Promise{{TestCovers: "NewFoo"}},
	}
	warnings := ValidateSpec(spec)
	found := false
	for _, w := range warnings {
		if w.Field == "promises" && w.Fatal {
			found = true
		}
	}
	if !found {
		t.Error("expected fatal warning for test_covers without test")
	}
}

func TestValidateSpec_Promise_TestCovers_EmptyString(t *testing.T) {
	// test_covers with whitespace-only value → treated as no field
	spec := &arc.PhaseSpec{
		Spec:       "do something",
		Complexity: "simple",
		Promises:   []arc.Promise{{TestCovers: "   ", Test: "TestFoo"}},
	}
	warnings := ValidateSpec(spec)
	found := false
	for _, w := range warnings {
		if w.Field == "promises" && w.Fatal {
			found = true
		}
	}
	if !found {
		t.Error("expected fatal warning for whitespace-only test_covers (treated as no field)")
	}
}

func TestValidateSpec_Promise_Valid_FuncExists(t *testing.T) {
	spec := &arc.PhaseSpec{
		Spec:       "do something",
		Complexity: "simple",
		Promises:   []arc.Promise{{FuncExists: "func NewFoo()"}},
	}
	for _, w := range ValidateSpec(spec) {
		if w.Field == "promises" {
			t.Errorf("unexpected promise warning: %s", w.Message)
		}
	}
}

func TestValidateSpec_Promise_Valid_TestExists(t *testing.T) {
	spec := &arc.PhaseSpec{
		Spec:       "do something",
		Complexity: "simple",
		Promises:   []arc.Promise{{TestExists: "TestFoo"}},
	}
	for _, w := range ValidateSpec(spec) {
		if w.Field == "promises" {
			t.Errorf("unexpected promise warning: %s", w.Message)
		}
	}
}

func TestValidateSpec_Promise_Valid_FileExists(t *testing.T) {
	spec := &arc.PhaseSpec{
		Spec:       "do something",
		Complexity: "simple",
		Promises:   []arc.Promise{{FileExists: "foo.go"}},
	}
	for _, w := range ValidateSpec(spec) {
		if w.Field == "promises" {
			t.Errorf("unexpected promise warning: %s", w.Message)
		}
	}
}

func TestValidateSpec_Promise_Valid_TestCovers(t *testing.T) {
	spec := &arc.PhaseSpec{
		Spec:       "do something",
		Complexity: "simple",
		Promises:   []arc.Promise{{TestCovers: "NewFoo", Test: "TestNewFoo"}},
	}
	for _, w := range ValidateSpec(spec) {
		if w.Field == "promises" {
			t.Errorf("unexpected promise warning: %s", w.Message)
		}
	}
}

func TestValidateSpec_Promise_MultipleWarnings(t *testing.T) {
	spec := &arc.PhaseSpec{
		Spec:       "do something",
		Complexity: "simple",
		Promises: []arc.Promise{
			{},
			{},
			{},
		},
	}
	count := 0
	for _, w := range ValidateSpec(spec) {
		if w.Field == "promises" && w.Fatal {
			count++
		}
	}
	if count != 3 {
		t.Errorf("expected 3 promise warnings, got %d", count)
	}
}

func TestValidateSpec_Promise_EmptyString(t *testing.T) {
	// promise with empty string value in func_exists → treated as no field
	spec := &arc.PhaseSpec{
		Spec:       "do something",
		Complexity: "simple",
		Promises:   []arc.Promise{{FuncExists: ""}},
	}
	found := false
	for _, w := range ValidateSpec(spec) {
		if w.Field == "promises" && w.Fatal {
			found = true
		}
	}
	if !found {
		t.Error("expected fatal warning for promise with empty func_exists")
	}
}

// TestValidateSpec_SpecCoverage verifies that an assertion with only spec_coverage
// set is not flagged as empty by ValidateSpec.
func TestValidateSpec_SpecCoverage(t *testing.T) {
	spec := &arc.PhaseSpec{
		Spec:       "do something",
		Complexity: "simple",
		Gate: arc.GateSpec{
			Assertions: []arc.GateAssertion{
				{SpecCoverage: "TestFoo"},
			},
		},
	}
	for _, w := range ValidateSpec(spec) {
		if w.Field == "gate.assertions" && w.Fatal {
			t.Errorf("unexpected fatal gate.assertions warning for spec_coverage assertion: %s", w.Message)
		}
	}
}

func TestValidateSpec_Promises_NilVsEmptySlice(t *testing.T) {
	nilSpec := &arc.PhaseSpec{Spec: "do something", Complexity: "simple", Promises: nil}
	emptySpec := &arc.PhaseSpec{Spec: "do something", Complexity: "simple", Promises: []arc.Promise{}}

	for _, w := range ValidateSpec(nilSpec) {
		if w.Field == "promises" {
			t.Errorf("unexpected warning for nil Promises: %s", w.Message)
		}
	}
	for _, w := range ValidateSpec(emptySpec) {
		if w.Field == "promises" {
			t.Errorf("unexpected warning for empty Promises: %s", w.Message)
		}
	}
}

// ---------------------------------------------------------------------------
// Integration tests — combined features (Files, Promises, spec_coverage)
// ---------------------------------------------------------------------------

func TestValidateSpec_MultipleWarnings(t *testing.T) {
	// Empty spec + empty assertion + invalid promise → at least 3 warnings.
	spec := &arc.PhaseSpec{
		Spec:       "",
		Complexity: "simple",
		Gate: arc.GateSpec{
			Assertions: []arc.GateAssertion{
				{SpecCoverage: ""},
			},
		},
		Promises: []arc.Promise{{}},
	}
	warnings := ValidateSpec(spec)
	if len(warnings) < 2 {
		t.Fatalf("expected at least 2 warnings, got %d: %v", len(warnings), warnings)
	}
	// Verify both spec-empty and promise-invalid warnings are present.
	hasSpec := false
	hasPromise := false
	for _, w := range warnings {
		if w.Field == "spec" {
			hasSpec = true
		}
		if w.Field == "promises" && w.Fatal {
			hasPromise = true
		}
	}
	if !hasSpec {
		t.Error("expected warning for empty spec field")
	}
	if !hasPromise {
		t.Error("expected fatal warning for invalid promise")
	}
}

func TestValidateSpec_TestCoversWithoutTestField_Warns(t *testing.T) {
	spec := &arc.PhaseSpec{
		Spec:       "do something",
		Complexity: "simple",
		Promises:   []arc.Promise{{TestCovers: "handles nil input", Test: ""}},
	}
	warnings := ValidateSpec(spec)
	found := false
	for _, w := range warnings {
		if w.Field == "promises" && w.Fatal {
			found = true
		}
	}
	if !found {
		t.Error("expected fatal warning for test_covers without test field")
	}
}

func TestValidateSpec_PromiseWithEmptyString_Warns(t *testing.T) {
	spec := &arc.PhaseSpec{
		Spec:       "do something",
		Complexity: "simple",
		Promises:   []arc.Promise{{FuncExists: ""}},
	}
	found := false
	for _, w := range ValidateSpec(spec) {
		if w.Field == "promises" && w.Fatal {
			found = true
		}
	}
	if !found {
		t.Error("expected fatal warning for promise with empty func_exists value")
	}
}

func TestValidateSpec_NilSpec_HandlesGracefully(t *testing.T) {
	// ValidateSpec should return nil without panicking when given a nil spec.
	warnings := ValidateSpec(nil)
	if warnings != nil {
		t.Errorf("expected nil warnings for nil spec, got %v", warnings)
	}
}

func TestValidateSpec_NilGate_HandlesGracefully(t *testing.T) {
	// GateSpec is a value type — "nil gate" means zero-value with empty Assertions.
	// ValidateSpec should not panic and should not warn about an empty Gate.
	spec := &arc.PhaseSpec{
		Spec:       "do something",
		Complexity: "simple",
		Gate:       arc.GateSpec{},
	}
	warnings := ValidateSpec(spec)
	for _, w := range warnings {
		if w.Field == "gate.assertions" {
			t.Errorf("unexpected gate warning for empty Gate: %s", w.Message)
		}
	}
}

func TestValidateSpec_FilesWithInvalidPaths_Warns(t *testing.T) {
	spec := &arc.PhaseSpec{
		Spec:       "do something",
		Complexity: "simple",
		Files:      []string{"../../../etc/passwd", "valid.go"},
	}
	warnings := ValidateSpec(spec)
	found := false
	for _, w := range warnings {
		if w.Field == "files" && strings.Contains(w.Message, "..") {
			found = true
		}
	}
	if !found {
		t.Error("expected warning about path traversal in files list")
	}
}

func TestValidateSpec_SpecCoverageWithNonEmptyValue_Warns(t *testing.T) {
	// spec_coverage with a multi-word prose value should generate an advisory
	// warning suggesting the use of a concrete identifier instead.
	spec := &arc.PhaseSpec{
		Spec:       "do something",
		Complexity: "simple",
		Gate: arc.GateSpec{
			Assertions: []arc.GateAssertion{
				{SpecCoverage: "should warn about this description"},
			},
		},
	}
	warnings := ValidateSpec(spec)
	found := false
	for _, w := range warnings {
		if w.Field == "gate.assertions" && !w.Fatal && strings.Contains(w.Message, "spec_coverage") {
			found = true
		}
	}
	if !found {
		t.Error("expected non-fatal warning for spec_coverage with prose description value")
	}
}
