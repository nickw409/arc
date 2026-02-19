package plan

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/state"
)

// setupSplitTestPlan creates a minimal plan with phases for split testing.
func setupSplitTestPlan(t *testing.T, phases []string, deps map[string][]string) string {
	t.Helper()
	plansDir := t.TempDir()
	planDir := filepath.Join(plansDir, "test-plan")

	meta := arc.NewPlanMeta("test-plan", "feature", phases)
	if deps != nil {
		meta.Dependencies = deps
	}

	if err := os.MkdirAll(planDir, 0755); err != nil {
		t.Fatal(err)
	}
	metaData, _ := json.MarshalIndent(meta, "", "  ")
	if err := os.WriteFile(filepath.Join(planDir, "plan.json"), metaData, 0644); err != nil {
		t.Fatal(err)
	}

	for _, p := range phases {
		phaseDir := filepath.Join(planDir, "phases", p)
		if err := os.MkdirAll(phaseDir, 0755); err != nil {
			t.Fatal(err)
		}
		ps := arc.NewPhaseState("test-plan", p, "feature")
		sd, _ := json.MarshalIndent(ps, "", "  ")
		if err := os.WriteFile(filepath.Join(phaseDir, "state.json"), sd, 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(phaseDir, "plan.md"), []byte("# Original Plan"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	return plansDir
}

func TestSplitCreatesSubPhases(t *testing.T) {
	plansDir := setupSplitTestPlan(t, []string{"core", "api"}, nil)

	err := Split(SplitOptions{
		PlansDir: plansDir,
		PlanName: "test-plan",
		Phase:    "core",
		SubNames: []string{"core-types", "core-impl"},
	})
	if err != nil {
		t.Fatalf("Split error: %v", err)
	}

	planDir := filepath.Join(plansDir, "test-plan")
	for _, sub := range []string{"core-types", "core-impl"} {
		subDir := filepath.Join(planDir, "phases", sub)
		if _, err := os.Stat(filepath.Join(subDir, "state.json")); err != nil {
			t.Fatalf("sub-phase %s state.json missing: %v", sub, err)
		}
		if _, err := os.Stat(filepath.Join(subDir, "plan.md")); err != nil {
			t.Fatalf("sub-phase %s plan.md missing: %v", sub, err)
		}
		if _, err := os.Stat(filepath.Join(subDir, "original_plan.md")); err != nil {
			t.Fatalf("sub-phase %s original_plan.md missing: %v", sub, err)
		}

		// Verify parent_phase is set
		sf := state.NewStateFile(filepath.Join(subDir, "state.json"))
		s, err := sf.Read()
		if err != nil {
			t.Fatalf("reading sub-phase state: %v", err)
		}
		if s.ParentPhase != "core" {
			t.Fatalf("sub-phase %s parent_phase = %q, want %q", sub, s.ParentPhase, "core")
		}
	}
}

func TestSplitMarksOriginalSplit(t *testing.T) {
	plansDir := setupSplitTestPlan(t, []string{"core", "api"}, nil)

	err := Split(SplitOptions{
		PlansDir: plansDir,
		PlanName: "test-plan",
		Phase:    "core",
		SubNames: []string{"core-a", "core-b"},
	})
	if err != nil {
		t.Fatalf("Split error: %v", err)
	}

	sf := state.NewStateFile(filepath.Join(plansDir, "test-plan", "phases", "core", "state.json"))
	s, err := sf.Read()
	if err != nil {
		t.Fatalf("reading original state: %v", err)
	}
	if s.PhaseStatus != "split" {
		t.Fatalf("original phase_status = %q, want %q", s.PhaseStatus, "split")
	}
	if len(s.SplitInto) != 2 || s.SplitInto[0] != "core-a" || s.SplitInto[1] != "core-b" {
		t.Fatalf("split_into = %v, want [core-a core-b]", s.SplitInto)
	}
}

func TestSplitRewiresDependencies(t *testing.T) {
	plansDir := setupSplitTestPlan(t, []string{"core", "api"}, map[string][]string{
		"api": {"core"},
	})

	err := Split(SplitOptions{
		PlansDir: plansDir,
		PlanName: "test-plan",
		Phase:    "core",
		SubNames: []string{"core-a", "core-b"},
	})
	if err != nil {
		t.Fatalf("Split error: %v", err)
	}

	meta, err := state.ReadPlan(filepath.Join(plansDir, "test-plan"))
	if err != nil {
		t.Fatalf("reading plan: %v", err)
	}

	// api should now depend on core-b (last sub-phase)
	apiDeps := meta.Dependencies["api"]
	if len(apiDeps) != 1 || apiDeps[0] != "core-b" {
		t.Fatalf("api dependencies = %v, want [core-b]", apiDeps)
	}

	// core-b should depend on core-a
	coreBDeps := meta.Dependencies["core-b"]
	if len(coreBDeps) != 1 || coreBDeps[0] != "core-a" {
		t.Fatalf("core-b dependencies = %v, want [core-a]", coreBDeps)
	}

	// Phases should be [core-a, core-b, api]
	wantPhases := []string{"core-a", "core-b", "api"}
	if len(meta.Phases) != len(wantPhases) {
		t.Fatalf("Phases = %v, want %v", meta.Phases, wantPhases)
	}
	for i, p := range wantPhases {
		if meta.Phases[i] != p {
			t.Fatalf("Phases[%d] = %q, want %q", i, meta.Phases[i], p)
		}
	}

	// SplitPhases tracking
	if meta.SplitPhases == nil || len(meta.SplitPhases["core"]) != 2 {
		t.Fatalf("SplitPhases[core] = %v, want [core-a core-b]", meta.SplitPhases["core"])
	}
}

func TestSplitNonexistentPhase(t *testing.T) {
	plansDir := setupSplitTestPlan(t, []string{"core"}, nil)

	err := Split(SplitOptions{
		PlansDir: plansDir,
		PlanName: "test-plan",
		Phase:    "nonexistent",
		SubNames: []string{"a", "b"},
	})
	if err == nil {
		t.Fatal("expected error for nonexistent phase")
	}
}

func TestInsertBefore(t *testing.T) {
	plansDir := setupSplitTestPlan(t, []string{"core", "api"}, map[string][]string{
		"api": {"core"},
	})

	err := Insert(InsertOptions{
		PlansDir: plansDir,
		PlanName: "test-plan",
		RefPhase: "api",
		NewNames: []string{"middleware"},
		Before:   true,
	})
	if err != nil {
		t.Fatalf("Insert error: %v", err)
	}

	meta, err := state.ReadPlan(filepath.Join(plansDir, "test-plan"))
	if err != nil {
		t.Fatalf("reading plan: %v", err)
	}

	// Phases should be [core, middleware, api]
	wantPhases := []string{"core", "middleware", "api"}
	if len(meta.Phases) != len(wantPhases) {
		t.Fatalf("Phases = %v, want %v", meta.Phases, wantPhases)
	}
	for i, p := range wantPhases {
		if meta.Phases[i] != p {
			t.Fatalf("Phases[%d] = %q, want %q", i, meta.Phases[i], p)
		}
	}

	// middleware inherits api's original deps (core)
	mwDeps := meta.Dependencies["middleware"]
	if len(mwDeps) != 1 || mwDeps[0] != "core" {
		t.Fatalf("middleware deps = %v, want [core]", mwDeps)
	}

	// api now depends on middleware
	apiDeps := meta.Dependencies["api"]
	if len(apiDeps) != 1 || apiDeps[0] != "middleware" {
		t.Fatalf("api deps = %v, want [middleware]", apiDeps)
	}
}

func TestInsertAfter(t *testing.T) {
	plansDir := setupSplitTestPlan(t, []string{"core", "api"}, map[string][]string{
		"api": {"core"},
	})

	err := Insert(InsertOptions{
		PlansDir: plansDir,
		PlanName: "test-plan",
		RefPhase: "core",
		NewNames: []string{"validation"},
		Before:   false,
	})
	if err != nil {
		t.Fatalf("Insert error: %v", err)
	}

	meta, err := state.ReadPlan(filepath.Join(plansDir, "test-plan"))
	if err != nil {
		t.Fatalf("reading plan: %v", err)
	}

	// Phases should be [core, validation, api]
	wantPhases := []string{"core", "validation", "api"}
	if len(meta.Phases) != len(wantPhases) {
		t.Fatalf("Phases = %v, want %v", meta.Phases, wantPhases)
	}
	for i, p := range wantPhases {
		if meta.Phases[i] != p {
			t.Fatalf("Phases[%d] = %q, want %q", i, meta.Phases[i], p)
		}
	}

	// validation depends on core
	valDeps := meta.Dependencies["validation"]
	if len(valDeps) != 1 || valDeps[0] != "core" {
		t.Fatalf("validation deps = %v, want [core]", valDeps)
	}

	// api now depends on validation (was core)
	apiDeps := meta.Dependencies["api"]
	if len(apiDeps) != 1 || apiDeps[0] != "validation" {
		t.Fatalf("api deps = %v, want [validation]", apiDeps)
	}
}

func TestInsertCreatesDirectories(t *testing.T) {
	plansDir := setupSplitTestPlan(t, []string{"core"}, nil)

	err := Insert(InsertOptions{
		PlansDir: plansDir,
		PlanName: "test-plan",
		RefPhase: "core",
		NewNames: []string{"new-phase"},
		Before:   false,
	})
	if err != nil {
		t.Fatalf("Insert error: %v", err)
	}

	newDir := filepath.Join(plansDir, "test-plan", "phases", "new-phase")
	if _, err := os.Stat(filepath.Join(newDir, "state.json")); err != nil {
		t.Fatalf("new phase state.json missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(newDir, "plan.md")); err != nil {
		t.Fatalf("new phase plan.md missing: %v", err)
	}
}

func TestInsertNonexistentRef(t *testing.T) {
	plansDir := setupSplitTestPlan(t, []string{"core"}, nil)

	err := Insert(InsertOptions{
		PlansDir: plansDir,
		PlanName: "test-plan",
		RefPhase: "nonexistent",
		NewNames: []string{"new"},
		Before:   false,
	})
	if err == nil {
		t.Fatal("expected error for nonexistent reference phase")
	}
}

func TestDeferSetsStatus(t *testing.T) {
	plansDir := setupSplitTestPlan(t, []string{"core"}, nil)

	err := Defer(DeferOptions{
		PlansDir: plansDir,
		PlanName: "test-plan",
		Phase:    "core",
		Reason:   "waiting on upstream",
	})
	if err != nil {
		t.Fatalf("Defer error: %v", err)
	}

	sf := state.NewStateFile(filepath.Join(plansDir, "test-plan", "phases", "core", "state.json"))
	s, err := sf.Read()
	if err != nil {
		t.Fatalf("reading state: %v", err)
	}
	if s.PhaseStatus != "deferred" {
		t.Fatalf("phase_status = %q, want %q", s.PhaseStatus, "deferred")
	}
}

func TestDeferSetsReasonAndTimestamp(t *testing.T) {
	plansDir := setupSplitTestPlan(t, []string{"core"}, nil)

	err := Defer(DeferOptions{
		PlansDir: plansDir,
		PlanName: "test-plan",
		Phase:    "core",
		Reason:   "not ready yet",
	})
	if err != nil {
		t.Fatalf("Defer error: %v", err)
	}

	sf := state.NewStateFile(filepath.Join(plansDir, "test-plan", "phases", "core", "state.json"))
	s, err := sf.Read()
	if err != nil {
		t.Fatalf("reading state: %v", err)
	}
	if s.DeferredReason != "not ready yet" {
		t.Fatalf("deferred_reason = %q, want %q", s.DeferredReason, "not ready yet")
	}
	if s.DeferredAt == "" {
		t.Fatal("deferred_at should not be empty")
	}
}

func TestDeferNonexistentPhase(t *testing.T) {
	plansDir := setupSplitTestPlan(t, []string{"core"}, nil)

	err := Defer(DeferOptions{
		PlansDir: plansDir,
		PlanName: "test-plan",
		Phase:    "nonexistent",
		Reason:   "test",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent phase")
	}
}

func TestSplitTooFewSubNames(t *testing.T) {
	plansDir := setupSplitTestPlan(t, []string{"core"}, nil)

	err := Split(SplitOptions{
		PlansDir: plansDir,
		PlanName: "test-plan",
		Phase:    "core",
		SubNames: []string{"only-one"},
	})
	if err == nil {
		t.Fatal("expected error for fewer than 2 sub-names")
	}
}

func TestInsertMultiplePhasesBefore(t *testing.T) {
	plansDir := setupSplitTestPlan(t, []string{"core", "api"}, map[string][]string{
		"api": {"core"},
	})

	err := Insert(InsertOptions{
		PlansDir: plansDir,
		PlanName: "test-plan",
		RefPhase: "api",
		NewNames: []string{"auth", "middleware"},
		Before:   true,
	})
	if err != nil {
		t.Fatalf("Insert error: %v", err)
	}

	meta, err := state.ReadPlan(filepath.Join(plansDir, "test-plan"))
	if err != nil {
		t.Fatalf("reading plan: %v", err)
	}

	// Phases should be [core, auth, middleware, api]
	wantPhases := []string{"core", "auth", "middleware", "api"}
	if len(meta.Phases) != len(wantPhases) {
		t.Fatalf("Phases = %v, want %v", meta.Phases, wantPhases)
	}
	for i, p := range wantPhases {
		if meta.Phases[i] != p {
			t.Fatalf("Phases[%d] = %q, want %q", i, meta.Phases[i], p)
		}
	}

	// auth inherits api's original deps (core)
	authDeps := meta.Dependencies["auth"]
	if len(authDeps) != 1 || authDeps[0] != "core" {
		t.Fatalf("auth deps = %v, want [core]", authDeps)
	}

	// middleware depends on auth
	mwDeps := meta.Dependencies["middleware"]
	if len(mwDeps) != 1 || mwDeps[0] != "auth" {
		t.Fatalf("middleware deps = %v, want [auth]", mwDeps)
	}

	// api now depends on middleware (last inserted)
	apiDeps := meta.Dependencies["api"]
	if len(apiDeps) != 1 || apiDeps[0] != "middleware" {
		t.Fatalf("api deps = %v, want [middleware]", apiDeps)
	}
}

func TestInsertMultiplePhasesAfter(t *testing.T) {
	plansDir := setupSplitTestPlan(t, []string{"core", "api"}, map[string][]string{
		"api": {"core"},
	})

	err := Insert(InsertOptions{
		PlansDir: plansDir,
		PlanName: "test-plan",
		RefPhase: "core",
		NewNames: []string{"validate", "transform"},
		Before:   false,
	})
	if err != nil {
		t.Fatalf("Insert error: %v", err)
	}

	meta, err := state.ReadPlan(filepath.Join(plansDir, "test-plan"))
	if err != nil {
		t.Fatalf("reading plan: %v", err)
	}

	// Phases should be [core, validate, transform, api]
	wantPhases := []string{"core", "validate", "transform", "api"}
	if len(meta.Phases) != len(wantPhases) {
		t.Fatalf("Phases = %v, want %v", meta.Phases, wantPhases)
	}
	for i, p := range wantPhases {
		if meta.Phases[i] != p {
			t.Fatalf("Phases[%d] = %q, want %q", i, meta.Phases[i], p)
		}
	}

	// validate depends on core
	valDeps := meta.Dependencies["validate"]
	if len(valDeps) != 1 || valDeps[0] != "core" {
		t.Fatalf("validate deps = %v, want [core]", valDeps)
	}

	// transform depends on validate
	trDeps := meta.Dependencies["transform"]
	if len(trDeps) != 1 || trDeps[0] != "validate" {
		t.Fatalf("transform deps = %v, want [validate]", trDeps)
	}

	// api now depends on transform (last inserted, was core)
	apiDeps := meta.Dependencies["api"]
	if len(apiDeps) != 1 || apiDeps[0] != "transform" {
		t.Fatalf("api deps = %v, want [transform]", apiDeps)
	}
}

func TestDeferAlreadyCompletePhase(t *testing.T) {
	plansDir := setupSplitTestPlan(t, []string{"core"}, nil)

	// Mark phase as complete first
	sf := state.NewStateFile(filepath.Join(plansDir, "test-plan", "phases", "core", "state.json"))
	if err := sf.Update(func(s *arc.PhaseState) error {
		s.PhaseStatus = "complete"
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Defer should still work (it overwrites the status)
	err := Defer(DeferOptions{
		PlansDir: plansDir,
		PlanName: "test-plan",
		Phase:    "core",
		Reason:   "need to revisit",
	})
	if err != nil {
		t.Fatalf("Defer error: %v", err)
	}

	s, err := sf.Read()
	if err != nil {
		t.Fatalf("reading state: %v", err)
	}
	if s.PhaseStatus != "deferred" {
		t.Fatalf("phase_status = %q, want %q", s.PhaseStatus, "deferred")
	}
	if s.DeferredReason != "need to revisit" {
		t.Fatalf("deferred_reason = %q, want %q", s.DeferredReason, "need to revisit")
	}
}
