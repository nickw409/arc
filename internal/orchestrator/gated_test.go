package orchestrator

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nwiley/arc/internal/adapter"
	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/config"
	"github.com/nwiley/arc/internal/state"

	"gopkg.in/yaml.v3"
)

// initTestGitRepo initializes a git repo in dir with an initial commit.
func initTestGitRepo(t *testing.T, dir string) {
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

// mockAdapter is a test adapter that records calls and returns configured results.
type mockAdapter struct {
	name      string
	calls     []mockCall
	results   []*arc.AgentResult // one per call, cycled if shorter
	errors    []error
	workFn    func(workdir string) // side effect to run during Spawn (e.g., create files)
	callCount int
}

type mockCall struct {
	Prompt  string
	WorkDir string
	Config  arc.SessionConfig
}

func (m *mockAdapter) Name() string { return m.name }

func (m *mockAdapter) Preflight(_ context.Context, _ string) error { return nil }

func (m *mockAdapter) Spawn(_ context.Context, prompt string, workdir string, cfg arc.SessionConfig) (*arc.AgentResult, error) {
	m.calls = append(m.calls, mockCall{Prompt: prompt, WorkDir: workdir, Config: cfg})
	idx := m.callCount
	m.callCount++

	// Run side effect
	if m.workFn != nil {
		m.workFn(workdir)
	}

	// Return configured result
	var result *arc.AgentResult
	if idx < len(m.results) {
		result = m.results[idx]
	} else if len(m.results) > 0 {
		result = m.results[len(m.results)-1]
	} else {
		result = &arc.AgentResult{ExitCode: 0, Duration: time.Second}
	}

	var err error
	if idx < len(m.errors) {
		err = m.errors[idx]
	}

	return result, err
}

// setupGatedTest creates a temp directory with a plan, phase, spec.yaml, and state.json.
// Returns the plans dir, a cleanup func, and the phase dir.
func setupGatedTest(t *testing.T, spec *arc.PhaseSpec) (plansDir, phaseDir string) {
	t.Helper()
	dir := t.TempDir()
	plansDir = dir
	planDir := filepath.Join(dir, "test-plan")
	phaseDir = filepath.Join(planDir, "phases", "test-phase")

	if err := os.MkdirAll(phaseDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Write spec.yaml
	spec.Name = "test-phase"
	data, err := yaml.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal spec: %v", err)
	}
	if err := os.WriteFile(filepath.Join(phaseDir, "spec.yaml"), data, 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	// Write plan.json
	meta := &arc.PlanMeta{
		Name:   "test-plan",
		Phases: []string{"test-phase"},
	}
	planData, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		t.Fatalf("marshal plan: %v", err)
	}
	if err := os.WriteFile(filepath.Join(planDir, "plan.json"), planData, 0o644); err != nil {
		t.Fatalf("write plan.json: %v", err)
	}

	// Write initial state.json
	sf := state.NewStateFile(filepath.Join(phaseDir, "state.json"))
	initial := arc.NewPhaseState("test-plan", "test-phase", "feature")
	initial.PhaseStatus = "pending"
	if err := sf.Write(initial); err != nil {
		t.Fatalf("write state: %v", err)
	}

	return plansDir, phaseDir
}

// registerMockAdapter registers a mock adapter under the given name and returns it.
func registerMockAdapter(t *testing.T, name string, mock *mockAdapter) {
	t.Helper()
	mock.name = name
	adapter.Registry[name] = func() arc.AgentAdapter { return mock }
	t.Cleanup(func() { delete(adapter.Registry, name) })
}

func TestRunPhaseGated_GatePassFirstAttempt(t *testing.T) {
	// Create a spec with a file_exists assertion
	spec := &arc.PhaseSpec{
		Spec:       "Create a hello.go file",
		Complexity: "simple",
		Gate: arc.GateSpec{
			Assertions: []arc.GateAssertion{
				{Type: "file_exists", Target: "hello.go", FileExists: "hello.go"},
			},
		},
	}
	plansDir, phaseDir := setupGatedTest(t, spec)

	// Mock adapter creates the file during Spawn
	mock := &mockAdapter{
		results: []*arc.AgentResult{{ExitCode: 0, Duration: time.Second}},
		workFn: func(workdir string) {
			os.WriteFile(filepath.Join(workdir, "hello.go"), []byte("package main\n"), 0o644)
		},
	}
	registerMockAdapter(t, "test-mock", mock)

	// WorkingDir = phaseDir so gate can find the file there
	// But we need a separate workdir since gate checks files there
	workDir := t.TempDir()
	initTestGitRepo(t, workDir)
	mock.workFn = func(_ string) {
		os.WriteFile(filepath.Join(workDir, "hello.go"), []byte("package main\n"), 0o644)
	}

	err := RunPhaseGated(context.Background(), RunPhaseOptions{
		PlanName:   "test-plan",
		PhaseName:  "test-phase",
		PlansDir:   plansDir,
		Config:     &config.Config{Agents: config.AgentsConfig{Default: "test-mock"}},
		Logger:     slog.Default(),
		WorkingDir: workDir,
	})
	if err != nil {
		t.Fatalf("RunPhaseGated: %v", err)
	}

	// Verify state is complete
	sf := state.NewStateFile(filepath.Join(phaseDir, "state.json"))
	ps, err := sf.Read()
	if err != nil {
		t.Fatalf("reading state: %v", err)
	}
	if ps.PhaseStatus != "complete" {
		t.Errorf("phase status: got %q, want %q", ps.PhaseStatus, "complete")
	}

	// Verify only one spawn call
	if len(mock.calls) != 1 {
		t.Errorf("spawn calls: got %d, want 1", len(mock.calls))
	}
}

func TestRunPhaseGated_GateFailThenPass(t *testing.T) {
	spec := &arc.PhaseSpec{
		Spec:       "Create a widget.go file",
		Complexity: "simple",
		Gate: arc.GateSpec{
			Assertions: []arc.GateAssertion{
				{Type: "file_exists", Target: "widget.go", FileExists: "widget.go"},
			},
		},
	}
	plansDir, phaseDir := setupGatedTest(t, spec)
	workDir := t.TempDir()
	initTestGitRepo(t, workDir)

	callCount := 0
	mock := &mockAdapter{
		results: []*arc.AgentResult{
			{ExitCode: 0, Duration: time.Second},
			{ExitCode: 0, Duration: time.Second},
		},
		workFn: func(_ string) {
			callCount++
			if callCount >= 2 {
				// Create file on second attempt
				os.WriteFile(filepath.Join(workDir, "widget.go"), []byte("package widget\n"), 0o644)
			}
		},
	}
	registerMockAdapter(t, "test-mock-retry", mock)

	err := RunPhaseGated(context.Background(), RunPhaseOptions{
		PlanName:   "test-plan",
		PhaseName:  "test-phase",
		PlansDir:   plansDir,
		Config:     &config.Config{Agents: config.AgentsConfig{Default: "test-mock-retry"}},
		Logger:     slog.Default(),
		WorkingDir: workDir,
	})
	if err != nil {
		t.Fatalf("RunPhaseGated: %v", err)
	}

	// Should have taken 2 attempts
	if len(mock.calls) != 2 {
		t.Errorf("spawn calls: got %d, want 2", len(mock.calls))
	}

	// Second call should include retry context
	if len(mock.calls) >= 2 {
		if !strings.Contains(mock.calls[1].Prompt, "Previous Attempt") {
			t.Error("retry prompt should contain 'Previous Attempt'")
		}
		if !strings.Contains(mock.calls[1].Prompt, "FAIL") {
			t.Error("retry prompt should contain gate failure output")
		}
	}

	// Verify complete
	sf := state.NewStateFile(filepath.Join(phaseDir, "state.json"))
	ps, _ := sf.Read()
	if ps.PhaseStatus != "complete" {
		t.Errorf("phase status: got %q, want %q", ps.PhaseStatus, "complete")
	}
}

func TestRunPhaseGated_MaxAttemptsBlocked(t *testing.T) {
	spec := &arc.PhaseSpec{
		Spec:       "Create impossible.go",
		Complexity: "simple",
		Gate: arc.GateSpec{
			Assertions: []arc.GateAssertion{
				{Type: "file_exists", Target: "impossible.go", FileExists: "impossible.go"},
			},
		},
	}
	plansDir, phaseDir := setupGatedTest(t, spec)
	workDir := t.TempDir()

	mock := &mockAdapter{
		results: []*arc.AgentResult{
			{ExitCode: 0, Duration: time.Second},
			{ExitCode: 0, Duration: time.Second},
			{ExitCode: 0, Duration: time.Second},
			{ExitCode: 0, Duration: time.Second},
		},
		// Never creates the file
	}
	registerMockAdapter(t, "test-mock-fail", mock)

	err := RunPhaseGated(context.Background(), RunPhaseOptions{
		PlanName:   "test-plan",
		PhaseName:  "test-phase",
		PlansDir:   plansDir,
		Config:     &config.Config{Agents: config.AgentsConfig{Default: "test-mock-fail"}},
		Logger:     slog.Default(),
		WorkingDir: workDir,
	})

	if err == nil {
		t.Fatal("expected error after max attempts")
	}
	if !strings.Contains(err.Error(), "blocked") {
		t.Errorf("error should mention 'blocked', got: %v", err)
	}

	// Verify state is blocked
	sf := state.NewStateFile(filepath.Join(phaseDir, "state.json"))
	ps, _ := sf.Read()
	if ps.PhaseStatus != "blocked" {
		t.Errorf("phase status: got %q, want %q", ps.PhaseStatus, "blocked")
	}

	// With strategic intervention, the loop exits early: 2 impl attempts + 1 strategic agent = 3 calls.
	// The strategic agent returns empty output → parsed as give_up → loop breaks.
	if len(mock.calls) < 2 {
		t.Errorf("spawn calls: got %d, want at least 2", len(mock.calls))
	}
}

func TestRunPhaseGated_ContextCancellation(t *testing.T) {
	spec := &arc.PhaseSpec{
		Spec:       "Cancelled task",
		Complexity: "simple",
		Gate: arc.GateSpec{
			Assertions: []arc.GateAssertion{
				{Type: "file_exists", Target: "never.go", FileExists: "never.go"},
			},
		},
	}
	plansDir, _ := setupGatedTest(t, spec)
	workDir := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	mock := &mockAdapter{}
	registerMockAdapter(t, "test-mock-cancel", mock)

	err := RunPhaseGated(ctx, RunPhaseOptions{
		PlanName:   "test-plan",
		PhaseName:  "test-phase",
		PlansDir:   plansDir,
		Config:     &config.Config{Agents: config.AgentsConfig{Default: "test-mock-cancel"}},
		Logger:     slog.Default(),
		WorkingDir: workDir,
	})

	if err == nil {
		t.Fatal("expected context cancellation error")
	}
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got: %v", err)
	}

	// No spawn calls should have been made
	if len(mock.calls) != 0 {
		t.Errorf("spawn calls: got %d, want 0", len(mock.calls))
	}
}

func TestRunPhaseGated_TransientRetry(t *testing.T) {
	spec := &arc.PhaseSpec{
		Spec:       "Create hello.go",
		Complexity: "simple",
		Gate: arc.GateSpec{
			Assertions: []arc.GateAssertion{
				{Type: "file_exists", Target: "hello.go", FileExists: "hello.go"},
			},
		},
	}
	plansDir, phaseDir := setupGatedTest(t, spec)
	workDir := t.TempDir()
	initTestGitRepo(t, workDir)

	callIdx := 0
	mock := &mockAdapter{
		results: []*arc.AgentResult{
			{ExitCode: 1, TimedOut: true, Duration: time.Second}, // transient: timeout
			{ExitCode: 0, Duration: time.Second},                 // success
		},
		errors: []error{
			nil, // timeout result but no error
			nil,
		},
		workFn: func(_ string) {
			callIdx++
			if callIdx >= 2 {
				os.WriteFile(filepath.Join(workDir, "hello.go"), []byte("package main\n"), 0o644)
			}
		},
	}
	registerMockAdapter(t, "test-mock-transient", mock)

	err := RunPhaseGated(context.Background(), RunPhaseOptions{
		PlanName:   "test-plan",
		PhaseName:  "test-phase",
		PlansDir:   plansDir,
		Config:     &config.Config{Agents: config.AgentsConfig{Default: "test-mock-transient"}},
		Logger:     slog.Default(),
		WorkingDir: workDir,
	})
	if err != nil {
		t.Fatalf("RunPhaseGated: %v", err)
	}

	sf := state.NewStateFile(filepath.Join(phaseDir, "state.json"))
	ps, _ := sf.Read()
	if ps.PhaseStatus != "complete" {
		t.Errorf("phase status: got %q, want %q", ps.PhaseStatus, "complete")
	}
}

func TestRunPhaseGated_CheckpointProgress(t *testing.T) {
	spec := &arc.PhaseSpec{
		Spec:       "Implement feature with checkpoints",
		Complexity: "medium",
		Checkpoints: []arc.Checkpoint{
			{Name: "step-1", Description: "First step", Test: "true"},  // always passes
			{Name: "step-2", Description: "Second step", Test: "false"}, // always fails
		},
		Gate: arc.GateSpec{},
	}
	plansDir, phaseDir := setupGatedTest(t, spec)
	workDir := t.TempDir()

	mock := &mockAdapter{
		results: []*arc.AgentResult{
			{ExitCode: 0, Duration: time.Second},
			{ExitCode: 0, Duration: time.Second},
			{ExitCode: 0, Duration: time.Second},
			{ExitCode: 0, Duration: time.Second},
		},
	}
	registerMockAdapter(t, "test-mock-cp", mock)

	err := RunPhaseGated(context.Background(), RunPhaseOptions{
		PlanName:   "test-plan",
		PhaseName:  "test-phase",
		PlansDir:   plansDir,
		Config:     &config.Config{Agents: config.AgentsConfig{Default: "test-mock-cp"}},
		Logger:     slog.Default(),
		WorkingDir: workDir,
	})

	// Should fail because step-2 always fails
	if err == nil {
		t.Fatal("expected error due to failing checkpoint")
	}

	// Check gate status was written
	status, readErr := readGateStatus(phaseDir)
	if readErr != nil {
		t.Fatalf("reading gate status: %v", readErr)
	}
	if status == nil {
		t.Fatal("gate status should have been written")
	}

	sf := state.NewStateFile(filepath.Join(phaseDir, "state.json"))
	ps, _ := sf.Read()
	if ps.PhaseStatus != "blocked" {
		t.Errorf("phase status: got %q, want %q", ps.PhaseStatus, "blocked")
	}
}

func TestRunPhaseGated_UsageAccumulation(t *testing.T) {
	spec := &arc.PhaseSpec{
		Spec:       "Create hello.go",
		Complexity: "simple",
		Gate: arc.GateSpec{
			Assertions: []arc.GateAssertion{
				{Type: "file_exists", Target: "hello.go", FileExists: "hello.go"},
			},
		},
	}
	plansDir, phaseDir := setupGatedTest(t, spec)
	workDir := t.TempDir()
	initTestGitRepo(t, workDir)

	mock := &mockAdapter{
		results: []*arc.AgentResult{
			{ExitCode: 0, Duration: time.Second, Usage: arc.Usage{InputTokens: 100, OutputTokens: 50}},
			{ExitCode: 0, Duration: time.Second, Usage: arc.Usage{InputTokens: 200, OutputTokens: 75}},
		},
		workFn: func(_ string) {
			// Create file on every call (gate always passes)
			os.WriteFile(filepath.Join(workDir, "hello.go"), []byte("package main\n"), 0o644)
		},
	}
	registerMockAdapter(t, "test-mock-usage", mock)

	err := RunPhaseGated(context.Background(), RunPhaseOptions{
		PlanName:   "test-plan",
		PhaseName:  "test-phase",
		PlansDir:   plansDir,
		Config:     &config.Config{Agents: config.AgentsConfig{Default: "test-mock-usage"}},
		Logger:     slog.Default(),
		WorkingDir: workDir,
	})
	if err != nil {
		t.Fatalf("RunPhaseGated: %v", err)
	}

	sf := state.NewStateFile(filepath.Join(phaseDir, "state.json"))
	ps, _ := sf.Read()

	// Should have usage from first attempt (gate passed on first try)
	if ps.Usage.InputTokens != 100 {
		t.Errorf("input tokens: got %d, want 100", ps.Usage.InputTokens)
	}
	if ps.Usage.OutputTokens != 50 {
		t.Errorf("output tokens: got %d, want 50", ps.Usage.OutputTokens)
	}
}

func TestClassifySpawnError(t *testing.T) {
	tests := []struct {
		name   string
		result *arc.AgentResult
		err    error
		want   ErrorTier
	}{
		{"nil result", nil, nil, TierTransient},
		{"timeout", &arc.AgentResult{TimedOut: true}, nil, TierTransient},
		{"inactivity kill", &arc.AgentResult{InactivityKill: true}, nil, TierTransient},
		{"crash no output", &arc.AgentResult{ExitCode: 1, Output: ""}, nil, TierTransient},
		{"exit with output", &arc.AgentResult{ExitCode: 1, Output: "some progress"}, nil, TierFeedback},
		{"success", &arc.AgentResult{ExitCode: 0, Output: "done"}, nil, TierFeedback},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifySpawnError(tt.result, tt.err)
			if got != tt.want {
				t.Errorf("classifySpawnError: got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestClassifyGateFailure(t *testing.T) {
	tests := []struct {
		name        string
		passed      int // checkpoints passed in current result
		attempt     int
		maxAttempts int
		prevPassed  int
		want        ErrorTier
	}{
		{"first attempt fail", 0, 1, 4, 0, TierFeedback},
		{"second attempt with progress", 1, 2, 4, 0, TierFeedback},
		{"second attempt no progress", 0, 2, 4, 0, TierStrategic},
		{"max attempts", 0, 4, 4, 0, TierGiveUp},
		{"max attempts with progress", 2, 4, 4, 1, TierGiveUp},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &arc.GateResult{
				Checkpoints: make([]arc.CheckpointStatus, tt.passed),
			}
			for i := range result.Checkpoints {
				result.Checkpoints[i].Status = "pass"
			}
			got := classifyGateFailure(result, tt.attempt, tt.maxAttempts, tt.prevPassed)
			if got != tt.want {
				t.Errorf("classifyGateFailure: got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestBuildImplPrompt(t *testing.T) {
	spec := &arc.PhaseSpec{
		Spec:  "Add a new API endpoint",
		Files: []string{"internal/api/handler.go"},
		Name:  "api-endpoint",
		Verify: "go test ./internal/api/",
		Checkpoints: []arc.Checkpoint{
			{Name: "handler", Description: "Handler function exists", Test: "go test -run TestHandler"},
		},
	}

	result, err := buildImplPrompt(spec, "test-plan", "Go 1.24 project")
	if err != nil {
		t.Fatalf("buildImplPrompt: %v", err)
	}

	checks := []string{
		"Add a new API endpoint",
		"internal/api/handler.go",
		"handler",
		"Handler function exists",
		"arc gate",
		"Go 1.24 project",
	}
	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("prompt should contain %q", check)
		}
	}
}

func TestBuildRetryPrompt(t *testing.T) {
	spec := &arc.PhaseSpec{
		Spec: "Add a feature",
		Name: "feat",
	}

	gateResult := &arc.GateResult{
		Passed: false,
		Assertions: []arc.AssertionResult{
			{Passed: false, Detail: "file not found"},
		},
	}

	result, err := buildRetryPrompt(spec, "test-plan", "", 2, gateResult, "M internal/foo.go")
	if err != nil {
		t.Fatalf("buildRetryPrompt: %v", err)
	}

	if !strings.Contains(result, "Add a feature") {
		t.Error("retry prompt should contain original spec")
	}
	if !strings.Contains(result, "attempt 2 of 4") {
		t.Error("retry prompt should contain attempt count")
	}
	if !strings.Contains(result, "FAIL") {
		t.Error("retry prompt should contain gate output")
	}
	if !strings.Contains(result, "M internal/foo.go") {
		t.Error("retry prompt should contain diff summary")
	}
}

// readGateStatus reads gate-status.json from a phase directory.
func readGateStatus(phaseDir string) (*arc.GateStatus, error) {
	data, err := os.ReadFile(filepath.Join(phaseDir, "gate-status.json"))
	if err != nil {
		return nil, err
	}
	var status arc.GateStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return nil, err
	}
	return &status, nil
}
