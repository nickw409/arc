package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/config"
)

func TestRunActionUnknown(t *testing.T) {
	actx := ActionContext{
		PhaseDir: t.TempDir(),
		Config:   &config.Config{},
		State:    arc.NewPhaseState("plan", "phase", "feature"),
	}

	err := RunAction(context.Background(), "nonexistent_action", map[string]string{}, actx)
	if err == nil {
		t.Fatal("expected error for unknown action")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "unknown action") {
		t.Fatalf("expected error containing 'unknown action', got: %v", err)
	}
}

func TestRunActionScriptPathTraversal(t *testing.T) {
	actx := ActionContext{
		PhaseDir: t.TempDir(),
		ArcHome:  t.TempDir(),
		Config:   &config.Config{},
		State:    arc.NewPhaseState("plan", "phase", "feature"),
	}

	err := RunAction(context.Background(), "script", map[string]string{"path": "../../../etc/passwd"}, actx)
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
	errStr := strings.ToLower(err.Error())
	if !strings.Contains(errStr, "..") && !strings.Contains(errStr, "path traversal") {
		t.Fatalf("expected error about path traversal, got: %v", err)
	}
}

func TestRunActionSwitchModelValid(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")
	actx := ActionContext{
		PhaseDir: t.TempDir(),
		Config:   &config.Config{},
		State:    state,
	}

	err := RunAction(context.Background(), "switch_model", map[string]string{"model": "sonnet"}, actx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if state.ModelOverride != "sonnet" {
		t.Fatalf("got ModelOverride=%q, want %q", state.ModelOverride, "sonnet")
	}
}

func TestRunActionRequestHuman(t *testing.T) {
	phaseDir := t.TempDir()
	state := arc.NewPhaseState("plan", "phase", "feature")
	actx := ActionContext{
		PhaseDir: phaseDir,
		Config:   &config.Config{},
		State:    state,
	}

	err := RunAction(context.Background(), "request_human", map[string]string{"message": "help needed"}, actx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Check that intervention_request.md was created
	reqPath := filepath.Join(phaseDir, "intervention_request.md")
	if _, err := os.Stat(reqPath); os.IsNotExist(err) {
		t.Fatal("expected intervention_request.md to be created")
	}

	// Check that state.InterventionRequest was set
	if state.InterventionRequest == nil {
		t.Fatal("expected InterventionRequest to be set")
	}
}

func TestRunActionRunTests(t *testing.T) {
	phaseDir := t.TempDir()
	arcHome := t.TempDir()
	state := arc.NewPhaseState("plan", "phase", "feature")
	state.TestFiles = []string{"tests/test.go"}
	actx := ActionContext{
		PhaseDir: phaseDir,
		Config:   &config.Config{Runner: "go-test"},
		State:    state,
		ArcHome:  arcHome,
	}

	// run_tests will attempt to call the runner; since no actual runner
	// scripts exist, this may error, but it shouldn't panic.
	err := RunAction(context.Background(), "run_tests", map[string]string{"pattern": "test_foo"}, actx)
	// This is expected to fail since no runner scripts exist, but should not panic
	_ = err
}

func TestRunActionCommit(t *testing.T) {
	// Create a temp git repo
	dir := t.TempDir()
	actx := ActionContext{
		PhaseDir: dir,
		Config:   &config.Config{Git: config.GitConfig{CommitStyle: "conventional"}},
		State:    arc.NewPhaseState("plan", "phase", "feature"),
	}

	// This will fail because there's no git repo, but it shouldn't panic
	err := RunAction(context.Background(), "commit", map[string]string{"message": "feat: add feature"}, actx)
	_ = err
}

func TestRunActionScriptSuccess(t *testing.T) {
	arcHome := t.TempDir()

	// Create a valid script
	scriptDir := filepath.Join(arcHome, "scripts")
	if err := os.MkdirAll(scriptDir, 0755); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(scriptDir, "test.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}

	actx := ActionContext{
		PhaseDir: t.TempDir(),
		ArcHome:  arcHome,
		Config:   &config.Config{},
		State:    arc.NewPhaseState("plan", "phase", "feature"),
	}

	err := RunAction(context.Background(), "script", map[string]string{"path": "scripts/test.sh", "args": "arg1 arg2"}, actx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestRunActionSwitchModelInvalid(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")
	actx := ActionContext{
		PhaseDir: t.TempDir(),
		Config:   &config.Config{},
		State:    state,
	}

	err := RunAction(context.Background(), "switch_model", map[string]string{"model": ""}, actx)
	if err == nil {
		t.Fatal("expected error for empty model")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "model") {
		t.Fatalf("expected error containing 'model', got: %v", err)
	}
}

func TestRunActionAnalyzeStuck(t *testing.T) {
	phaseDir := t.TempDir()
	state := arc.NewPhaseState("plan", "phase", "feature")
	actx := ActionContext{
		PhaseDir: phaseDir,
		Config:   &config.Config{},
		State:    state,
	}

	err := RunAction(context.Background(), "analyze_stuck", nil, actx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Check that stuck_analysis.md was created
	analysisPath := filepath.Join(phaseDir, "stuck_analysis.md")
	if _, err := os.Stat(analysisPath); os.IsNotExist(err) {
		t.Fatal("expected stuck_analysis.md to be created")
	}
}

func TestRunActionRunTestsEmptyTestfiles(t *testing.T) {
	phaseDir := t.TempDir()
	state := arc.NewPhaseState("plan", "phase", "feature")
	state.TestFiles = []string{} // empty
	actx := ActionContext{
		PhaseDir: phaseDir,
		Config:   &config.Config{Runner: "go-test"},
		State:    state,
		ArcHome:  t.TempDir(),
	}

	err := RunAction(context.Background(), "run_tests", map[string]string{}, actx)
	if err != nil {
		t.Fatalf("expected no error for empty test files, got %v", err)
	}
}

func TestRunActionNilParams(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")
	actx := ActionContext{
		PhaseDir: t.TempDir(),
		Config:   &config.Config{},
		State:    state,
	}

	err := RunAction(context.Background(), "switch_model", nil, actx)
	if err == nil {
		t.Fatal("expected error for nil params (model is empty)")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "model") {
		t.Fatalf("expected error containing 'model', got: %v", err)
	}
}

func TestRunActionScriptMissingPath(t *testing.T) {
	actx := ActionContext{
		PhaseDir: t.TempDir(),
		ArcHome:  t.TempDir(),
		Config:   &config.Config{},
		State:    arc.NewPhaseState("plan", "phase", "feature"),
	}

	err := RunAction(context.Background(), "script", map[string]string{}, actx)
	if err == nil {
		t.Fatal("expected error for missing path")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "path") {
		t.Fatalf("expected error containing 'path', got: %v", err)
	}
}

func TestRunActionScriptNonzeroExit(t *testing.T) {
	arcHome := t.TempDir()

	// Create a script that exits with code 1
	scriptDir := filepath.Join(arcHome, "scripts")
	if err := os.MkdirAll(scriptDir, 0755); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(scriptDir, "fail.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}

	actx := ActionContext{
		PhaseDir: t.TempDir(),
		ArcHome:  arcHome,
		Config:   &config.Config{},
		State:    arc.NewPhaseState("plan", "phase", "feature"),
	}

	err := RunAction(context.Background(), "script", map[string]string{"path": "scripts/fail.sh"}, actx)
	if err == nil {
		t.Fatal("expected error for non-zero exit")
	}
	errStr := strings.ToLower(err.Error())
	if !strings.Contains(errStr, "exit") && !strings.Contains(errStr, "non-zero") {
		t.Fatalf("expected error about exit/non-zero, got: %v", err)
	}
}

func TestRunActionCommitNoSigning(t *testing.T) {
	actx := ActionContext{
		PhaseDir: t.TempDir(),
		Config:   &config.Config{Git: config.GitConfig{Sign: false}},
		State:    arc.NewPhaseState("plan", "phase", "feature"),
	}

	// Will fail because no git repo, but should not panic
	err := RunAction(context.Background(), "commit", map[string]string{"message": "test"}, actx)
	_ = err
}

// TestRunTestsActionSaveToWriteError verifies that if the save_to destination
// cannot be written (e.g. parent directory is read-only), runTestsAction returns
// an error rather than silently discarding it.
func TestRunTestsActionSaveToWriteError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root — cannot test read-only directory permission enforcement")
	}

	arcHome := t.TempDir()
	phaseDir := t.TempDir()

	// Create a runner script that exits 0 (so the exec error is nil and save_to is reached).
	scriptDir := filepath.Join(arcHome, "scripts")
	if err := os.MkdirAll(scriptDir, 0755); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(scriptDir, "run-phase-tests.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\necho 'test output'\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create a read-only subdirectory inside phaseDir that will be the save_to target's parent.
	roDir := filepath.Join(phaseDir, "readonly")
	if err := os.MkdirAll(roDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(roDir, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(roDir, 0755) })

	state := arc.NewPhaseState("plan", "phase", "feature")
	state.TestFiles = []string{"tests/foo_test.go"}

	actx := ActionContext{
		PhaseDir: phaseDir,
		Config:   &config.Config{Runner: "go-test"},
		State:    state,
		ArcHome:  arcHome,
	}

	// save_to points into the read-only directory — write must fail.
	err := RunAction(context.Background(), "run_tests", map[string]string{
		"save_to": "readonly/results.txt",
	}, actx)
	if err == nil {
		t.Fatal("expected error when save_to destination is not writable")
	}
	if !strings.Contains(err.Error(), "writing test output") {
		t.Fatalf("expected error about 'writing test output', got: %v", err)
	}
}
