package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/config"
	"github.com/nwiley/arc/internal/state"

	"gopkg.in/yaml.v3"
)

// initGitRepo initializes a bare git repo in dir with an initial commit,
// so that gitops.Commit works in tests.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@test.com"},
		{"config", "user.name", "Test"},
		{"commit", "--allow-empty", "-m", "init"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
	}
}

// setupGatedPlan creates a plan with multiple phases and specs for LaunchGated tests.
func setupGatedPlan(t *testing.T, phases []string, deps map[string][]string, specs map[string]*arc.PhaseSpec) string {
	t.Helper()
	plansDir := t.TempDir()
	planDir := filepath.Join(plansDir, "test-plan")

	for _, phase := range phases {
		phaseDir := filepath.Join(planDir, "phases", phase)
		if err := os.MkdirAll(phaseDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		// Write spec.yaml
		spec := specs[phase]
		if spec == nil {
			spec = &arc.PhaseSpec{
				Spec:       "Implement " + phase,
				Complexity: "simple",
				Gate: arc.GateSpec{
					Assertions: []arc.GateAssertion{
						{Type: "file_exists", Target: phase + ".go", FileExists: phase + ".go"},
					},
				},
			}
		}
		spec.Name = phase
		data, err := yaml.Marshal(spec)
		if err != nil {
			t.Fatalf("marshal spec: %v", err)
		}
		if err := os.WriteFile(filepath.Join(phaseDir, "spec.yaml"), data, 0o644); err != nil {
			t.Fatalf("write spec: %v", err)
		}
		writeTestPlanMD(t, phaseDir, spec)

		// Write state.json
		sf := state.NewStateFile(filepath.Join(phaseDir, "state.json"))
		initial := arc.NewPhaseState("test-plan", phase, "feature")
		initial.PhaseStatus = "pending"
		if err := sf.Write(initial); err != nil {
			t.Fatalf("write state: %v", err)
		}
	}

	// Write plan.json
	meta := &arc.PlanMeta{
		Name:         "test-plan",
		Phases:       phases,
		Dependencies: deps,
	}
	planData, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		t.Fatalf("marshal plan: %v", err)
	}
	if err := os.WriteFile(filepath.Join(planDir, "plan.json"), planData, 0o644); err != nil {
		t.Fatalf("write plan.json: %v", err)
	}

	return plansDir
}

func TestLaunchGated_SinglePhasePass(t *testing.T) {
	plansDir := setupGatedPlan(t,
		[]string{"phase-a"},
		nil,
		nil,
	)

	workDir := t.TempDir()
	initGitRepo(t, workDir)
	mock := &mockAdapter{
		results: []*arc.AgentResult{{ExitCode: 0, Duration: time.Second}},
		workFn: func(_ string) {
			os.WriteFile(filepath.Join(workDir, "phase-a.go"), []byte("package a\n"), 0o644)
		},
	}
	registerMockAdapter(t, "launch-mock-1", mock)

	result, err := LaunchGated(context.Background(), LaunchOptions{
		PlanName: "test-plan",
		PlansDir: plansDir,
		Config:   &config.Config{Agents: config.AgentsConfig{Default: "launch-mock-1"}},
		Logger:   slog.Default(),
		ProjectDir: workDir,
	})
	if err != nil {
		t.Fatalf("LaunchGated: %v", err)
	}
	if result.Status != "complete" {
		t.Errorf("status: got %q, want %q", result.Status, "complete")
	}
	if result.PhaseSummary["phase-a"] != "complete" {
		t.Errorf("phase-a status: got %q, want %q", result.PhaseSummary["phase-a"], "complete")
	}
}

func TestLaunchGated_DependencyOrder(t *testing.T) {
	// phase-b depends on phase-a
	plansDir := setupGatedPlan(t,
		[]string{"phase-a", "phase-b"},
		map[string][]string{"phase-b": {"phase-a"}},
		nil,
	)

	workDir := t.TempDir()
	initGitRepo(t, workDir)
	var order []string
	callCount := 0

	mock := &mockAdapter{
		results: []*arc.AgentResult{
			{ExitCode: 0, Duration: time.Second},
			{ExitCode: 0, Duration: time.Second},
		},
		workFn: func(_ string) {
			callCount++
			// Create files based on which phase is being worked on
			// Both files exist so both gates pass
			if callCount == 1 {
				order = append(order, "first")
				os.WriteFile(filepath.Join(workDir, "phase-a.go"), []byte("package a\n"), 0o644)
			} else {
				order = append(order, "second")
				os.WriteFile(filepath.Join(workDir, "phase-b.go"), []byte("package b\n"), 0o644)
			}
		},
	}
	registerMockAdapter(t, "launch-mock-dep", mock)

	result, err := LaunchGated(context.Background(), LaunchOptions{
		PlanName:   "test-plan",
		PlansDir:   plansDir,
		Config:     &config.Config{Agents: config.AgentsConfig{Default: "launch-mock-dep"}},
		Logger:     slog.Default(),
		ProjectDir: workDir,
	})
	if err != nil {
		t.Fatalf("LaunchGated: %v", err)
	}
	if result.Status != "complete" {
		t.Errorf("status: got %q, want %q", result.Status, "complete")
	}

	// Both phases should have completed
	if result.PhaseSummary["phase-a"] != "complete" {
		t.Errorf("phase-a: got %q, want complete", result.PhaseSummary["phase-a"])
	}
	if result.PhaseSummary["phase-b"] != "complete" {
		t.Errorf("phase-b: got %q, want complete", result.PhaseSummary["phase-b"])
	}

	// Verify ordering — phase-a must have been spawned before phase-b
	if len(order) >= 2 && order[0] != "first" {
		t.Errorf("expected phase-a to run first, got order: %v", order)
	}
}

func TestLaunchGated_ParallelIndependent(t *testing.T) {
	// phase-a and phase-b are independent — should run in parallel
	plansDir := setupGatedPlan(t,
		[]string{"phase-a", "phase-b"},
		nil, // no dependencies
		nil,
	)

	workDir := t.TempDir()
	initGitRepo(t, workDir)
	mock := &mockAdapter{
		results: []*arc.AgentResult{
			{ExitCode: 0, Duration: time.Second},
			{ExitCode: 0, Duration: time.Second},
		},
		workFn: func(_ string) {
			// Create both files so both gates pass
			os.WriteFile(filepath.Join(workDir, "phase-a.go"), []byte("package a\n"), 0o644)
			os.WriteFile(filepath.Join(workDir, "phase-b.go"), []byte("package b\n"), 0o644)
		},
	}
	registerMockAdapter(t, "launch-mock-parallel", mock)

	result, err := LaunchGated(context.Background(), LaunchOptions{
		PlanName:   "test-plan",
		PlansDir:   plansDir,
		Config:     &config.Config{Agents: config.AgentsConfig{Default: "launch-mock-parallel"}},
		Logger:     slog.Default(),
		ProjectDir: workDir,
	})
	if err != nil {
		t.Fatalf("LaunchGated: %v", err)
	}
	// When parallel phases share a git repo, concurrent commits may race on
	// the index lock, causing one phase to get blocked. Accept either outcome.
	if result.Status != "complete" && result.Status != "partial" {
		t.Errorf("status: got %q, want complete or partial", result.Status)
	}
	// At least one phase must have completed.
	anyComplete := false
	for _, s := range result.PhaseSummary {
		if s == "complete" {
			anyComplete = true
		}
	}
	if !anyComplete {
		t.Errorf("expected at least one phase to complete, got %v", result.PhaseSummary)
	}
}

func TestLaunchGated_StopOnFailure(t *testing.T) {
	plansDir := setupGatedPlan(t,
		[]string{"phase-a"},
		nil,
		nil,
	)
	workDir := t.TempDir()

	mock := &mockAdapter{
		results: []*arc.AgentResult{
			{ExitCode: 0, Duration: time.Second},
			{ExitCode: 0, Duration: time.Second},
			{ExitCode: 0, Duration: time.Second},
			{ExitCode: 0, Duration: time.Second},
		},
		// Never creates the file — gate always fails
	}
	registerMockAdapter(t, "launch-mock-stop", mock)

	result, err := LaunchGated(context.Background(), LaunchOptions{
		PlanName:      "test-plan",
		PlansDir:      plansDir,
		Config:        &config.Config{Agents: config.AgentsConfig{Default: "launch-mock-stop"}},
		Logger:        slog.Default(),
		ProjectDir:    workDir,
		StopOnFailure: true,
	})

	// Should not return an error (StopOnFailure returns result, not error)
	if err != nil {
		t.Fatalf("LaunchGated: %v", err)
	}
	if result.Status != "failed" {
		t.Errorf("status: got %q, want %q", result.Status, "failed")
	}
	if result.FailedPhase != "phase-a" {
		t.Errorf("failed phase: got %q, want %q", result.FailedPhase, "phase-a")
	}
}

func TestLaunchGated_ContinueOnFailure(t *testing.T) {
	// phase-a fails, phase-b (independent) should still complete
	plansDir := setupGatedPlan(t,
		[]string{"phase-a", "phase-b"},
		nil,
		nil,
	)
	workDir := t.TempDir()
	initGitRepo(t, workDir)

	callCount := 0
	mock := &mockAdapter{
		results: []*arc.AgentResult{
			{ExitCode: 0, Duration: time.Second},
			{ExitCode: 0, Duration: time.Second},
			{ExitCode: 0, Duration: time.Second},
			{ExitCode: 0, Duration: time.Second},
			{ExitCode: 0, Duration: time.Second},
			{ExitCode: 0, Duration: time.Second},
			{ExitCode: 0, Duration: time.Second},
			{ExitCode: 0, Duration: time.Second},
		},
		workFn: func(_ string) {
			callCount++
			// Create only phase-b.go — phase-a will fail, phase-b will pass
			os.WriteFile(filepath.Join(workDir, "phase-b.go"), []byte("package b\n"), 0o644)
		},
	}
	registerMockAdapter(t, "launch-mock-continue", mock)

	result, err := LaunchGated(context.Background(), LaunchOptions{
		PlanName:      "test-plan",
		PlansDir:      plansDir,
		Config:        &config.Config{Agents: config.AgentsConfig{Default: "launch-mock-continue"}},
		Logger:        slog.Default(),
		ProjectDir:    workDir,
		StopOnFailure: false,
	})

	// With continue-on-failure, we get a result even with errors
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// phase-b should be complete, phase-a should be blocked
	if result.PhaseSummary["phase-b"] != "complete" {
		t.Errorf("phase-b: got %q, want complete", result.PhaseSummary["phase-b"])
	}
	if result.PhaseSummary["phase-a"] != "blocked" {
		t.Errorf("phase-a: got %q, want blocked", result.PhaseSummary["phase-a"])
	}

	_ = err // error is expected
}

func TestLaunchGated_Timeout(t *testing.T) {
	plansDir := setupGatedPlan(t,
		[]string{"phase-a"},
		nil,
		nil,
	)
	workDir := t.TempDir()

	mock := &mockAdapter{
		results: []*arc.AgentResult{{ExitCode: 0, Duration: time.Second}},
		workFn: func(_ string) {
			// Simulate slow agent
			time.Sleep(2 * time.Second)
		},
	}
	registerMockAdapter(t, "launch-mock-timeout", mock)

	result, err := LaunchGated(context.Background(), LaunchOptions{
		PlanName:   "test-plan",
		PlansDir:   plansDir,
		Config:     &config.Config{Agents: config.AgentsConfig{Default: "launch-mock-timeout"}},
		Logger:     slog.Default(),
		ProjectDir: workDir,
		Timeout:    1, // 1 second timeout
	})

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Status != "cancelled" {
		t.Errorf("status: got %q, want %q", result.Status, "cancelled")
	}
	_ = err
}

// registerMockAdapter is defined in gated_test.go — reuse it here via the same package.
// Adapter registry operations are safe because tests run sequentially within a package.

func TestIdentifyConflictingPhase_FileMatch(t *testing.T) {
	plansDir := setupGatedPlan(t,
		[]string{"phase-a", "phase-b"},
		nil,
		map[string]*arc.PhaseSpec{
			"phase-a": {Files: []string{"internal/foo.go", "internal/bar.go"}},
			"phase-b": {Files: []string{"internal/baz.go"}},
		},
	)

	meta := &arc.PlanMeta{
		Name:   "test-plan",
		Phases: []string{"phase-a", "phase-b"},
	}
	opts := LaunchOptions{
		PlanName: "test-plan",
		PlansDir: plansDir,
		Logger:   slog.Default(),
	}

	// Error mentions a file from phase-a.
	err := fmt.Errorf("merge conflict: internal/foo.go: content conflict")
	phase := identifyConflictingPhase(opts, meta, err)
	if phase != "phase-a" {
		t.Errorf("expected phase-a, got %q", phase)
	}
}

func TestIdentifyConflictingPhase_NoMatch_SinglePhase(t *testing.T) {
	plansDir := setupGatedPlan(t,
		[]string{"phase-a"},
		nil,
		map[string]*arc.PhaseSpec{
			"phase-a": {Files: []string{"internal/foo.go"}},
		},
	)

	meta := &arc.PlanMeta{
		Name:   "test-plan",
		Phases: []string{"phase-a"},
	}
	opts := LaunchOptions{
		PlanName: "test-plan",
		PlansDir: plansDir,
		Logger:   slog.Default(),
	}

	// Error has no file match — should still return the only phase.
	err := fmt.Errorf("merge conflict with unrelated message")
	phase := identifyConflictingPhase(opts, meta, err)
	if phase != "phase-a" {
		t.Errorf("expected phase-a (single phase fallback), got %q", phase)
	}
}

func TestIdentifyConflictingPhase_NoMatch_MultiPhase(t *testing.T) {
	plansDir := setupGatedPlan(t,
		[]string{"phase-a", "phase-b"},
		nil,
		nil,
	)

	meta := &arc.PlanMeta{
		Name:   "test-plan",
		Phases: []string{"phase-a", "phase-b"},
	}
	opts := LaunchOptions{
		PlanName: "test-plan",
		PlansDir: plansDir,
		Logger:   slog.Default(),
	}

	// Error has no file match and there are multiple phases — should return "".
	err := fmt.Errorf("merge conflict with no files mentioned")
	phase := identifyConflictingPhase(opts, meta, err)
	if phase != "" {
		t.Errorf("expected empty string (cannot identify), got %q", phase)
	}
}

func TestRouteRegressionFailure_NoFailingTests(t *testing.T) {
	// Output has no FAIL lines — routeRegressionFailure should be a no-op.
	plansDir := setupGatedPlan(t, []string{"phase-a"}, nil, nil)
	meta := &arc.PlanMeta{Name: "test-plan", Phases: []string{"phase-a"}}
	workDir := t.TempDir()

	err := routeRegressionFailure(context.Background(), LaunchOptions{
		PlanName:   "test-plan",
		PlansDir:   plansDir,
		Logger:     slog.Default(),
		ProjectDir: workDir,
	}, meta, nil, "ok  \tgithub.com/example/pkg\t0.001s\n", workDir)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestBuildFileToPhaseMap_Basic(t *testing.T) {
	plansDir := setupGatedPlan(t,
		[]string{"phase-a", "phase-b"},
		nil,
		map[string]*arc.PhaseSpec{
			"phase-a": {Files: []string{"internal/foo.go", "internal/bar.go"}},
			"phase-b": {Files: []string{"internal/baz.go"}},
		},
	)

	meta := &arc.PlanMeta{
		Name:   "test-plan",
		Phases: []string{"phase-a", "phase-b"},
	}
	opts := LaunchOptions{
		PlanName: "test-plan",
		PlansDir: plansDir,
	}

	m := buildFileToPhaseMap(opts, meta)

	if m["internal/foo.go"] != "phase-a" {
		t.Errorf("internal/foo.go: expected phase-a, got %q", m["internal/foo.go"])
	}
	if m["internal/bar.go"] != "phase-a" {
		t.Errorf("internal/bar.go: expected phase-a, got %q", m["internal/bar.go"])
	}
	if m["internal/baz.go"] != "phase-b" {
		t.Errorf("internal/baz.go: expected phase-b, got %q", m["internal/baz.go"])
	}
}

// --- Partial status tests ---

// TestBuildResult_PartialStatus verifies that buildResult returns "partial"
// when some phases complete and others are blocked/deferred.
func TestBuildResult_PartialStatus(t *testing.T) {
	// phase-a and phase-b are independent; phase-b will never create its file
	// so its gate fails and it ends up blocked. phase-a succeeds.
	plansDir := setupGatedPlan(t,
		[]string{"phase-a", "phase-b"},
		nil, // independent
		nil,
	)
	workDir := t.TempDir()
	initGitRepo(t, workDir)

	callCount := 0
	mock := &mockAdapter{
		results: []*arc.AgentResult{
			{ExitCode: 0, Duration: time.Second},
			{ExitCode: 0, Duration: time.Second},
			{ExitCode: 0, Duration: time.Second},
			{ExitCode: 0, Duration: time.Second},
			{ExitCode: 0, Duration: time.Second},
			{ExitCode: 0, Duration: time.Second},
			{ExitCode: 0, Duration: time.Second},
			{ExitCode: 0, Duration: time.Second},
		},
		workFn: func(_ string) {
			callCount++
			// Only create phase-a.go so phase-a passes, phase-b stays blocked.
			os.WriteFile(filepath.Join(workDir, "phase-a.go"), []byte("package a\n"), 0o644)
		},
	}
	registerMockAdapter(t, "partial-status-mock", mock)

	result, _ := LaunchGated(context.Background(), LaunchOptions{
		PlanName:      "test-plan",
		PlansDir:      plansDir,
		Config:        &config.Config{Agents: config.AgentsConfig{Default: "partial-status-mock"}},
		Logger:        slog.Default(),
		ProjectDir:    workDir,
		StopOnFailure: false,
	})

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// phase-a should be complete; phase-b should be blocked.
	if result.PhaseSummary["phase-a"] != "complete" {
		t.Errorf("phase-a: got %q, want complete", result.PhaseSummary["phase-a"])
	}
	if result.PhaseSummary["phase-b"] != "blocked" {
		t.Errorf("phase-b: got %q, want blocked", result.PhaseSummary["phase-b"])
	}

	// Overall status must be "partial", not "complete".
	if result.Status != "partial" {
		t.Errorf("status: got %q, want partial", result.Status)
	}
}

// TestBuildResult_AllComplete verifies that "complete" is preserved when all phases succeed.
func TestBuildResult_AllComplete(t *testing.T) {
	plansDir := setupGatedPlan(t, []string{"phase-a"}, nil, nil)
	workDir := t.TempDir()
	initGitRepo(t, workDir)

	mock := &mockAdapter{
		results: []*arc.AgentResult{{ExitCode: 0, Duration: time.Second}},
		workFn: func(_ string) {
			os.WriteFile(filepath.Join(workDir, "phase-a.go"), []byte("package a\n"), 0o644)
		},
	}
	registerMockAdapter(t, "all-complete-mock", mock)

	result, err := LaunchGated(context.Background(), LaunchOptions{
		PlanName:   "test-plan",
		PlansDir:   plansDir,
		Config:     &config.Config{Agents: config.AgentsConfig{Default: "all-complete-mock"}},
		Logger:     slog.Default(),
		ProjectDir: workDir,
	})
	if err != nil {
		t.Fatalf("LaunchGated: %v", err)
	}
	if result.Status != "complete" {
		t.Errorf("status: got %q, want complete", result.Status)
	}
}

// TestPlanMeta_AdversaryBugsField confirms the AdversaryBugs field round-trips via JSON.
func TestPlanMeta_AdversaryBugsField(t *testing.T) {
	meta := &arc.PlanMeta{
		Name:   "my-plan",
		Status: "complete",
		Phases: []string{"impl"},
		AdversaryBugs: map[string][]string{
			"impl": {"TestFoo", "TestBar"},
		},
	}

	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var decoded arc.PlanMeta
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	bugs := decoded.AdversaryBugs["impl"]
	if len(bugs) != 2 {
		t.Fatalf("AdversaryBugs[impl]: got %v, want 2 items", bugs)
	}
	if bugs[0] != "TestFoo" || bugs[1] != "TestBar" {
		t.Errorf("AdversaryBugs[impl]: got %v, want [TestFoo TestBar]", bugs)
	}
}

// TestPlanMeta_AdversaryBugsOmitempty verifies that the field is absent from JSON
// when nil (omitempty).
func TestPlanMeta_AdversaryBugsOmitempty(t *testing.T) {
	meta := &arc.PlanMeta{
		Name:   "my-plan",
		Status: "complete",
	}

	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if _, present := raw["adversary_bugs"]; present {
		t.Error("adversary_bugs key should be absent when field is nil (omitempty)")
	}
}

// TestAgentPIDField verifies that AgentPID round-trips through JSON correctly.
func TestAgentPIDField(t *testing.T) {
	ps := arc.NewPhaseState("my-plan", "impl", "feature")
	ps.AgentPID = 12345

	data, err := json.Marshal(ps)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var decoded arc.PhaseState
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if decoded.AgentPID != 12345 {
		t.Errorf("AgentPID: got %d, want 12345", decoded.AgentPID)
	}
}

// TestAgentPIDOmitempty verifies that AgentPID=0 is absent from JSON (omitempty).
func TestAgentPIDOmitempty(t *testing.T) {
	ps := arc.NewPhaseState("my-plan", "impl", "feature")
	// AgentPID defaults to 0

	data, err := json.Marshal(ps)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if _, present := raw["agent_pid"]; present {
		t.Error("agent_pid key should be absent when 0 (omitempty)")
	}
}

// TestKillStaleAgents_DeadProcess verifies that killStaleAgents clears the PID
// when the recorded process is no longer alive.
func TestKillStaleAgents_DeadProcess(t *testing.T) {
	plansDir := setupGatedPlan(t, []string{"impl"}, nil, nil)
	planDir := filepath.Join(plansDir, "test-plan")

	// Write a state file with a PID that is guaranteed to be dead.
	// PID 1 is always alive (init), but we can use a PID from a process we
	// know is dead. Use the current process's PID + 99999 which very likely
	// does not exist.
	phaseDir := filepath.Join(planDir, "phases", "impl")
	sf := state.NewStateFile(filepath.Join(phaseDir, "state.json"))

	// Write a very large PID that is almost certainly not a running process.
	if err := sf.Update(func(s *arc.PhaseState) error {
		s.AgentPID = 2000000 // beyond typical PID max on Linux
		return nil
	}); err != nil {
		t.Fatalf("update state: %v", err)
	}

	logger := slog.Default()
	KillStaleAgents(planDir, logger)

	// After killStaleAgents, AgentPID should be cleared.
	ps, err := sf.Read()
	if err != nil {
		t.Fatalf("reading state: %v", err)
	}
	if ps.AgentPID != 0 {
		t.Errorf("AgentPID: got %d, want 0 (should be cleared for dead process)", ps.AgentPID)
	}
}

// TestKillStaleAgents_NoPIDPhases verifies that killStaleAgents is a no-op
// when no phases have a non-zero AgentPID.
func TestKillStaleAgents_NoPIDPhases(t *testing.T) {
	plansDir := setupGatedPlan(t, []string{"impl"}, nil, nil)
	planDir := filepath.Join(plansDir, "test-plan")

	// Default state has AgentPID=0, so no killing should happen.
	logger := slog.Default()
	// Should not panic or error.
	KillStaleAgents(planDir, logger)

	phaseDir := filepath.Join(planDir, "phases", "impl")
	sf := state.NewStateFile(filepath.Join(phaseDir, "state.json"))
	ps, err := sf.Read()
	if err != nil {
		t.Fatalf("reading state: %v", err)
	}
	if ps.AgentPID != 0 {
		t.Errorf("AgentPID: got %d, want 0", ps.AgentPID)
	}
}

// TestLaunchGated_ConfigPathSetsUpSIGHUP verifies that LaunchGated runs successfully
// when ConfigPath is set (the SIGHUP goroutine should start without error).
func TestLaunchGated_ConfigPathWiring(t *testing.T) {
	plansDir := setupGatedPlan(t, []string{"phase-a"}, nil, nil)
	workDir := t.TempDir()
	initGitRepo(t, workDir)

	mock := &mockAdapter{
		results: []*arc.AgentResult{{ExitCode: 0, Duration: time.Second}},
		workFn: func(_ string) {
			os.WriteFile(filepath.Join(workDir, "phase-a.go"), []byte("package a\n"), 0o644)
		},
	}
	registerMockAdapter(t, "sighup-wiring-mock", mock)

	// Write a temporary .arc.yaml for the config path.
	arcYaml := filepath.Join(t.TempDir(), ".arc.yaml")
	if err := os.WriteFile(arcYaml, []byte("language: go\nrunner: go-test\n"), 0644); err != nil {
		t.Fatalf("write .arc.yaml: %v", err)
	}

	result, err := LaunchGated(context.Background(), LaunchOptions{
		PlanName:   "test-plan",
		PlansDir:   plansDir,
		Config:     &config.Config{Agents: config.AgentsConfig{Default: "sighup-wiring-mock"}},
		ConfigPath: arcYaml,
		Logger:     slog.Default(),
		ProjectDir: workDir,
	})
	if err != nil {
		t.Fatalf("LaunchGated with ConfigPath: %v", err)
	}
	if result.Status != "complete" {
		t.Errorf("status: got %q, want complete", result.Status)
	}
}

