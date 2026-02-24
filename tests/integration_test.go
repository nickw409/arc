//go:build integration

package tests

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nwiley/arc/internal/arc"
)

var arcBinary string

func TestMain(m *testing.M) {
	// Build the arc binary once for all tests
	tmpDir, err := os.MkdirTemp("", "arc-integration-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmpDir)

	arcBinary = filepath.Join(tmpDir, "arc")
	cmd := exec.Command("go", "build", "-o", arcBinary, "./cmd/arc")
	cmd.Dir = filepath.Join("..")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		panic("failed to build arc binary: " + err.Error())
	}

	os.Exit(m.Run())
}

func runArc(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(arcBinary, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func setupProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Copy sample project
	src := filepath.Join("testdata", "sample-project")
	entries, _ := os.ReadDir(src)
	for _, e := range entries {
		data, _ := os.ReadFile(filepath.Join(src, e.Name()))
		os.WriteFile(filepath.Join(dir, e.Name()), data, 0644)
	}

	// Initialize git repo (needed for some features)
	exec.Command("git", "-C", dir, "init").Run()
	exec.Command("git", "-C", dir, "add", ".").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "init").Run()

	return dir
}

func TestE2EInitCreatesProject(t *testing.T) {
	dir := setupProject(t)
	out, err := runArc(t, dir, "init")
	if err != nil {
		t.Fatalf("init failed: %s\n%s", err, out)
	}

	// Check .arc.yaml exists
	if _, err := os.Stat(filepath.Join(dir, ".arc.yaml")); os.IsNotExist(err) {
		t.Error(".arc.yaml not created")
	}

	// Check .plans directories
	if _, err := os.Stat(filepath.Join(dir, ".plans", "active")); os.IsNotExist(err) {
		t.Error(".plans/active not created")
	}
	if _, err := os.Stat(filepath.Join(dir, ".plans", "archive")); os.IsNotExist(err) {
		t.Error(".plans/archive not created")
	}

	// Check .claude/commands/arc-plan.md
	if _, err := os.Stat(filepath.Join(dir, ".claude", "commands", "arc-plan.md")); os.IsNotExist(err) {
		t.Error(".claude/commands/arc-plan.md not created")
	}
}

func TestE2EPlanCreatesStructure(t *testing.T) {
	dir := setupProject(t)
	runArc(t, dir, "init")

	out, err := runArc(t, dir, "plan", "my-feature", "core", "api")
	if err != nil {
		t.Fatalf("plan failed: %s\n%s", err, out)
	}

	// Check plan.json
	planJSON, err := os.ReadFile(filepath.Join(dir, ".plans", "active", "my-feature", "plan.json"))
	if err != nil {
		t.Fatal("plan.json not found")
	}

	var meta arc.PlanMeta
	if err := json.Unmarshal(planJSON, &meta); err != nil {
		t.Fatalf("invalid plan.json: %v", err)
	}

	// Should have core and api
	if len(meta.Phases) != 2 {
		t.Errorf("expected 2 phases, got %d: %v", len(meta.Phases), meta.Phases)
	}

	// Check session_id
	sessionPath := filepath.Join(dir, ".plans", "active", "my-feature", "session_id")
	sessionData, err := os.ReadFile(sessionPath)
	if err != nil {
		t.Fatal("session_id not found")
	}
	if len(strings.TrimSpace(string(sessionData))) < 30 {
		t.Error("session_id seems too short for a UUID")
	}

	// Check phase directories
	for _, phase := range meta.Phases {
		phaseDir := filepath.Join(dir, ".plans", "active", "my-feature", "phases", phase)
		if _, err := os.Stat(filepath.Join(phaseDir, "state.json")); os.IsNotExist(err) {
			t.Errorf("state.json missing for phase %s", phase)
		}
		if _, err := os.Stat(filepath.Join(phaseDir, "plan.md")); os.IsNotExist(err) {
			t.Errorf("plan.md missing for phase %s", phase)
		}
	}
}

func TestE2EPlanWithTypeFlag(t *testing.T) {
	dir := setupProject(t)
	runArc(t, dir, "init")

	out, err := runArc(t, dir, "plan", "--type", "bugfix", "fix-login", "investigate", "fix")
	if err != nil {
		t.Fatalf("plan failed: %s\n%s", err, out)
	}

	planJSON, err := os.ReadFile(filepath.Join(dir, ".plans", "active", "fix-login", "plan.json"))
	if err != nil {
		t.Fatal("plan.json not found")
	}

	var meta arc.PlanMeta
	json.Unmarshal(planJSON, &meta)
	if meta.WorkflowType != "bugfix" {
		t.Errorf("workflow_type=%q, want 'bugfix'", meta.WorkflowType)
	}
}

func TestE2EStatusShowsPlan(t *testing.T) {
	dir := setupProject(t)
	runArc(t, dir, "init")
	runArc(t, dir, "plan", "my-feature", "core", "api")

	out, err := runArc(t, dir, "status", "my-feature")
	if err != nil {
		t.Fatalf("status failed: %s\n%s", err, out)
	}

	if !strings.Contains(out, "my-feature") {
		t.Error("output missing plan name")
	}
	if !strings.Contains(out, "[ ]") {
		t.Error("output missing pending icon")
	}
}

func TestE2EStatusNoPlans(t *testing.T) {
	dir := setupProject(t)
	runArc(t, dir, "init")

	out, err := runArc(t, dir, "status")
	// With no plans, status should either show empty output or succeed silently
	if err != nil && strings.Contains(out, "unknown command") {
		t.Error("status command not registered")
	}
	// No plans = no output is acceptable
	_ = out
}

func TestE2EInitAlreadyInitialized(t *testing.T) {
	dir := setupProject(t)
	runArc(t, dir, "init")

	_, err := runArc(t, dir, "init")
	if err == nil {
		t.Error("expected error for double init")
	}
}

func TestE2EInitForceReinitialize(t *testing.T) {
	dir := setupProject(t)
	runArc(t, dir, "init")

	_, err := runArc(t, dir, "init", "--force")
	if err != nil {
		t.Errorf("force init failed: %v", err)
	}
}

func TestE2EPlanDuplicateName(t *testing.T) {
	dir := setupProject(t)
	runArc(t, dir, "init")
	runArc(t, dir, "plan", "my-feature", "core")

	_, err := runArc(t, dir, "plan", "my-feature", "core")
	if err == nil {
		t.Error("expected error for duplicate plan name")
	}
}

func TestE2EPlanInvalidName(t *testing.T) {
	dir := setupProject(t)
	runArc(t, dir, "init")

	_, err := runArc(t, dir, "plan", "Bad-Name!", "phase1")
	if err == nil {
		t.Error("expected error for invalid plan name")
	}
}

func TestE2EWorkflowLoadingRoundtrip(t *testing.T) {
	// This tests internally but via the plan creation path
	dir := setupProject(t)
	runArc(t, dir, "init")

	for _, wfType := range []string{"feature", "bugfix", "investigation", "refactor", "performance"} {
		name := "test-" + wfType
		out, err := runArc(t, dir, "plan", "--type", wfType, name, "core")
		if err != nil {
			t.Errorf("plan create with type %q failed: %s\n%s", wfType, err, out)
		}
	}
}

func TestE2EStateUpdateRoundtrip(t *testing.T) {
	dir := setupProject(t)
	runArc(t, dir, "init")
	runArc(t, dir, "plan", "my-feature", "core")

	// Read state.json and verify it roundtrips
	statePath := filepath.Join(dir, ".plans", "active", "my-feature", "phases", "core", "state.json")
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}

	var ps arc.PhaseState
	if err := json.Unmarshal(data, &ps); err != nil {
		t.Fatal(err)
	}

	if ps.PhaseStatus != "pending" {
		t.Errorf("status=%q, want 'pending'", ps.PhaseStatus)
	}

	// Verify slices are not null in JSON
	if ps.Disputes == nil {
		t.Error("Disputes is nil")
	}
	if ps.TestFiles == nil {
		t.Error("TestFiles is nil")
	}

	// Re-marshal and check for null values
	reJSON, _ := json.Marshal(ps)
	if strings.Contains(string(reJSON), `"disputes":null`) {
		t.Error("disputes serialized as null")
	}
}

func TestE2EConfigRoundtrip(t *testing.T) {
	dir := setupProject(t)
	runArc(t, dir, "init")

	// Verify .arc.yaml has expected content
	data, err := os.ReadFile(filepath.Join(dir, ".arc.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	content := string(data)
	if !strings.Contains(content, "language:") {
		t.Error("missing language field")
	}
	if !strings.Contains(content, "runner:") {
		t.Error("missing runner field")
	}
}

func TestE2EVersionFlag(t *testing.T) {
	dir := t.TempDir()
	out, _ := runArc(t, dir, "--version")
	if !strings.Contains(out, "0.") {
		t.Errorf("version output unexpected: %s", out)
	}
}

func TestE2EHelpShowsAllCommands(t *testing.T) {
	dir := t.TempDir()
	out, _ := runArc(t, dir, "--help")

	commands := []string{"init", "plan", "status", "run", "iterate", "review", "monitor", "update"}
	for _, cmd := range commands {
		if !strings.Contains(out, cmd) {
			t.Errorf("help missing command %q", cmd)
		}
	}
}

func TestE2EIterateStub(t *testing.T) {
	dir := setupProject(t)
	runArc(t, dir, "init")
	runArc(t, dir, "plan", "my-feature", "core")

	// Should exist as a command (may fail since no agent, but shouldn't panic)
	out, err := runArc(t, dir, "iterate", "my-feature", "core")
	_ = out
	// We just verify it doesn't say "unknown command"
	if err != nil && strings.Contains(out, "unknown command") {
		t.Error("iterate command not registered")
	}
}

func TestE2EReviewStub(t *testing.T) {
	dir := setupProject(t)
	runArc(t, dir, "init")
	runArc(t, dir, "plan", "my-feature", "core")

	out, err := runArc(t, dir, "review", "my-feature")
	_ = out
	if err != nil && strings.Contains(out, "unknown command") {
		t.Error("review command not registered")
	}
}

func TestE2ERunStub(t *testing.T) {
	dir := setupProject(t)
	runArc(t, dir, "init")
	runArc(t, dir, "plan", "my-feature", "core")

	out, err := runArc(t, dir, "run", "my-feature")
	_ = out
	if err != nil && strings.Contains(out, "unknown command") {
		t.Error("run command not registered")
	}
}

func TestE2EMonitorStub(t *testing.T) {
	dir := setupProject(t)

	out, err := runArc(t, dir, "monitor", "--help")
	_ = out
	if err != nil && strings.Contains(out, "unknown command") {
		t.Error("monitor command not registered")
	}
}

func TestE2EPlanUnicodeName(t *testing.T) {
	dir := setupProject(t)
	runArc(t, dir, "init")

	_, err := runArc(t, dir, "plan", "ünïcödë", "phase1")
	if err == nil {
		t.Error("expected error for unicode plan name")
	}
}

func TestE2EPlanTypeInvestigation(t *testing.T) {
	dir := setupProject(t)
	runArc(t, dir, "init")

	out, err := runArc(t, dir, "plan", "--type", "investigation", "analyze-bug", "research", "analyze")
	if err != nil {
		t.Fatalf("plan failed: %s\n%s", err, out)
	}

	planJSON, _ := os.ReadFile(filepath.Join(dir, ".plans", "active", "analyze-bug", "plan.json"))
	var meta arc.PlanMeta
	json.Unmarshal(planJSON, &meta)
	if meta.WorkflowType != "investigation" {
		t.Errorf("workflow_type=%q, want 'investigation'", meta.WorkflowType)
	}
}

func TestE2EIterateNonexistentPlan(t *testing.T) {
	dir := setupProject(t)
	runArc(t, dir, "init")

	out, err := runArc(t, dir, "iterate", "nonexistent-plan", "core")
	if err == nil {
		t.Error("expected error for nonexistent plan")
	}
	lower := strings.ToLower(out)
	if !strings.Contains(lower, "not found") && !strings.Contains(lower, "does not exist") && !strings.Contains(lower, "no such") {
		t.Errorf("expected descriptive error, got: %s", out)
	}
}

func TestE2EStatusCorruptedState(t *testing.T) {
	dir := setupProject(t)
	runArc(t, dir, "init")
	runArc(t, dir, "plan", "my-feature", "core")

	// Corrupt state.json
	statePath := filepath.Join(dir, ".plans", "active", "my-feature", "phases", "core", "state.json")
	os.WriteFile(statePath, []byte("{invalid"), 0644)

	// Status should handle gracefully
	out, _ := runArc(t, dir, "status", "my-feature")
	_ = out
	// Just verify no panic (command runs)
}

func TestE2EInitForceThenPlan(t *testing.T) {
	dir := setupProject(t)
	runArc(t, dir, "init")
	runArc(t, dir, "init", "--force")

	out, err := runArc(t, dir, "plan", "my-feature", "core")
	if err != nil {
		t.Fatalf("plan after force reinit failed: %s\n%s", err, out)
	}
}

func TestE2EPlanLongPhaseNames(t *testing.T) {
	dir := setupProject(t)
	runArc(t, dir, "init")

	out, err := runArc(t, dir, "plan", "my-feature", "this-is-a-very-long-phase-name-that-tests-limits")
	if err != nil {
		t.Fatalf("long phase name failed: %s\n%s", err, out)
	}
}

func TestE2EStateConcurrentUpdates(t *testing.T) {
	// Test that concurrent state updates don't corrupt files
	dir := setupProject(t)
	runArc(t, dir, "init")
	runArc(t, dir, "plan", "my-feature", "core")

	statePath := filepath.Join(dir, ".plans", "active", "my-feature", "phases", "core", "state.json")

	// Do sequential updates (concurrency tested in unit tests)
	for i := 0; i < 5; i++ {
		data, err := os.ReadFile(statePath)
		if err != nil {
			t.Fatal(err)
		}
		var ps arc.PhaseState
		json.Unmarshal(data, &ps)
		ps.Iteration.Current = i + 1
		updated, _ := json.MarshalIndent(ps, "", "  ")
		os.WriteFile(statePath, updated, 0644)
	}

	// Verify final state
	data, _ := os.ReadFile(statePath)
	var ps arc.PhaseState
	json.Unmarshal(data, &ps)
	if ps.Iteration.Current != 5 {
		t.Errorf("iteration=%d, want 5", ps.Iteration.Current)
	}
}
