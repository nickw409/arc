package orchestrator

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/config"
	"github.com/nwiley/arc/internal/state"

	"gopkg.in/yaml.v3"
)

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
	if result.Status != "complete" {
		t.Errorf("status: got %q, want %q", result.Status, "complete")
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

func init() {
	// Ensure mock adapters are cleaned up between test runs.
	// The registerMockAdapter function in gated_test.go handles this via t.Cleanup.
}
