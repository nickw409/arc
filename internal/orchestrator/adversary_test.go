package orchestrator

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/config"
	"github.com/nwiley/arc/internal/testcmd"

	"gopkg.in/yaml.v3"
)

func TestCollectChangedFiles_FromSpecs(t *testing.T) {
	plansDir := t.TempDir()
	planDir := filepath.Join(plansDir, "test-plan", "phases", "phase-a")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatal(err)
	}

	spec := &arc.PhaseSpec{
		Name:  "phase-a",
		Files: []string{"internal/foo.go", "internal/bar.go"},
	}
	data, _ := yaml.Marshal(spec)
	os.WriteFile(filepath.Join(planDir, "spec.yaml"), data, 0o644)

	files, err := collectChangedFiles(plansDir, "test-plan", t.TempDir())
	if err != nil {
		t.Fatalf("collectChangedFiles: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("got %d files, want 2", len(files))
	}
	// Check both files present (order doesn't matter since it's a map)
	found := map[string]bool{}
	for _, f := range files {
		found[f] = true
	}
	if !found["internal/foo.go"] || !found["internal/bar.go"] {
		t.Errorf("expected foo.go and bar.go, got %v", files)
	}
}

func TestCollectChangedFiles_MultiplePhases(t *testing.T) {
	plansDir := t.TempDir()
	planDir := filepath.Join(plansDir, "test-plan")

	phases := map[string][]string{
		"phase-a": {"internal/foo.go", "internal/shared.go"},
		"phase-b": {"internal/bar.go", "internal/shared.go"}, // shared.go in both
	}
	for phase, files := range phases {
		phaseDir := filepath.Join(planDir, "phases", phase)
		os.MkdirAll(phaseDir, 0o755)
		spec := &arc.PhaseSpec{Name: phase, Files: files}
		data, _ := yaml.Marshal(spec)
		os.WriteFile(filepath.Join(phaseDir, "spec.yaml"), data, 0o644)
	}

	files, err := collectChangedFiles(plansDir, "test-plan", t.TempDir())
	if err != nil {
		t.Fatalf("collectChangedFiles: %v", err)
	}
	// Should be deduplicated: foo.go, bar.go, shared.go = 3 unique files
	if len(files) != 3 {
		t.Errorf("got %d files, want 3 (deduped)", len(files))
	}
}

func TestBuildAdversaryPrompt(t *testing.T) {
	files := []string{"internal/api/handler.go", "internal/api/middleware.go"}
	prompt, err := buildAdversaryPrompt(files, "go test ./internal/api/", "Go 1.24 project")
	if err != nil {
		t.Fatalf("buildAdversaryPrompt: %v", err)
	}

	checks := []string{
		"internal/api/handler.go",
		"internal/api/middleware.go",
		"go test ./internal/api/",
	}
	for _, check := range checks {
		if !strings.Contains(prompt, check) {
			t.Errorf("prompt should contain %q", check)
		}
	}
}

func TestRunAdversary_NoChangedFiles(t *testing.T) {
	// Plan with no file specs and a non-git workdir
	plansDir := t.TempDir()
	planDir := filepath.Join(plansDir, "test-plan", "phases")
	os.MkdirAll(planDir, 0o755)

	workDir := t.TempDir()

	result, err := RunAdversary(context.Background(), LaunchOptions{
		PlanName:   "test-plan",
		PlansDir:   plansDir,
		Config:     &config.Config{},
		Logger:     slog.Default(),
		ProjectDir: workDir,
	}, workDir, nil)
	if err != nil {
		t.Fatalf("RunAdversary: %v", err)
	}
	if result.BugsFound {
		t.Error("expected no bugs found with no changed files")
	}
}

func TestRunAdversary_NoBugsFound(t *testing.T) {
	plansDir := t.TempDir()
	planDir := filepath.Join(plansDir, "test-plan", "phases", "phase-a")
	os.MkdirAll(planDir, 0o755)

	spec := &arc.PhaseSpec{Name: "phase-a", Files: []string{"main.go"}}
	data, _ := yaml.Marshal(spec)
	os.WriteFile(filepath.Join(planDir, "spec.yaml"), data, 0o644)

	workDir := t.TempDir()

	mock := &mockAdapter{
		results: []*arc.AgentResult{{ExitCode: 0, Duration: time.Second}},
	}
	registerMockAdapter(t, "adv-mock-nobugs", mock)

	result, err := RunAdversary(context.Background(), LaunchOptions{
		PlanName:   "test-plan",
		PlansDir:   plansDir,
		Config:     &config.Config{Agents: config.AgentsConfig{Default: "adv-mock-nobugs"}, TestCommand: "true"},
		Logger:     slog.Default(),
		ProjectDir: workDir,
	}, workDir, nil)
	if err != nil {
		t.Fatalf("RunAdversary: %v", err)
	}
	if result.BugsFound {
		t.Error("expected no bugs found when tests pass")
	}
	if len(result.Rounds) != 1 {
		t.Errorf("expected 1 round, got %d", len(result.Rounds))
	}
}

func TestParseFailingTests_Basic(t *testing.T) {
	output := `=== RUN   TestFoo
--- FAIL: TestFoo (0.00s)
=== RUN   TestBar
--- FAIL: TestBar (0.01s)
FAIL
`
	tests := testcmd.ParseFailures(output)
	if len(tests) != 2 {
		t.Fatalf("expected 2 failing tests, got %d: %v", len(tests), tests)
	}
	found := map[string]bool{}
	for _, name := range tests {
		found[name] = true
	}
	if !found["TestFoo"] {
		t.Error("expected TestFoo in failing tests")
	}
	if !found["TestBar"] {
		t.Error("expected TestBar in failing tests")
	}
}

func TestParseFailingTests_Deduplication(t *testing.T) {
	output := `--- FAIL: TestFoo (0.00s)
--- FAIL: TestFoo (0.00s)
--- FAIL: TestBar (0.01s)
`
	tests := testcmd.ParseFailures(output)
	if len(tests) != 2 {
		t.Errorf("expected 2 unique tests (deduped), got %d: %v", len(tests), tests)
	}
}

func TestParseFailingTests_Empty(t *testing.T) {
	tests := testcmd.ParseFailures("ok  \tgithub.com/example/pkg\t0.001s\n")
	if len(tests) != 0 {
		t.Errorf("expected 0 tests from passing output, got %d", len(tests))
	}
}

func TestParseFailingTests_SubTests(t *testing.T) {
	output := `--- FAIL: TestFoo/sub1 (0.00s)
--- FAIL: TestFoo/sub2 (0.00s)
--- FAIL: TestBar (0.01s)
`
	tests := testcmd.ParseFailures(output)
	// Should capture each as separate failing test names.
	if len(tests) < 2 {
		t.Errorf("expected at least 2 failing tests, got %d: %v", len(tests), tests)
	}
}

func TestAdversaryResult_AllTestFiles(t *testing.T) {
	r := &AdversaryResult{
		Rounds: []AdversaryRound{
			{TestFiles: []string{"a_test.go", "b_test.go"}},
			{TestFiles: []string{"c_test.go"}},
		},
	}
	all := r.allTestFiles()
	if len(all) != 3 {
		t.Errorf("got %d test files, want 3", len(all))
	}
}
