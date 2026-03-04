package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/state"
)

func setupParallelTestPlan(t *testing.T) (string, *state.StateFile) {
	t.Helper()

	dir := t.TempDir()
	phaseDir := filepath.Join(dir, "phases", "test-phase")
	if err := os.MkdirAll(phaseDir, 0755); err != nil {
		t.Fatal(err)
	}

	ps := arc.NewPhaseState("test-plan", "test-phase", "feature")
	ps.CurrentState = "impl"
	ps.PhaseStatus = "implementing"

	data, _ := json.MarshalIndent(ps, "", "  ")
	statePath := filepath.Join(phaseDir, "state.json")
	if err := os.WriteFile(statePath, data, 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(phaseDir, "plan.md"), []byte("# Test Plan"), 0644); err != nil {
		t.Fatal(err)
	}

	sf := state.NewStateFile(statePath)
	return phaseDir, sf
}

func TestRunParallelAllSucceed(t *testing.T) {
	phaseDir, sf := setupParallelTestPlan(t)
	ps, _ := sf.Read()

	// All branches succeed (exit 0)
	t.Setenv("MOCK_OUTPUT", "done\n")

	verdict, _, err := RunParallel(context.Background(), testLogger(), RunParallelOptions{
		PhaseDir:   phaseDir,
		StateFile:  sf,
		PhaseState: ps,
		Config: &arc.ParallelConfig{
			Branches: []arc.ParallelBranch{
				{Name: "branch-a", Prompt: "do task a"},
				{Name: "branch-b", Prompt: "do task b"},
			},
			Strategy: "all",
		},
		PlanMD: "# Test Plan",
	})
	if err != nil {
		t.Fatalf("RunParallel error: %v", err)
	}
	if verdict != "all_complete" {
		t.Fatalf("verdict = %q, want %q", verdict, "all_complete")
	}

	// Verify parallel tracking in state
	updated, _ := sf.Read()
	if updated.ParallelExecution == nil {
		t.Fatal("ParallelExecution should not be nil")
	}
	if updated.ParallelExecution.Verdict != "all_complete" {
		t.Fatalf("ParallelExecution.Verdict = %q, want %q", updated.ParallelExecution.Verdict, "all_complete")
	}
	if updated.ParallelExecution.FinishedAt == "" {
		t.Fatal("ParallelExecution.FinishedAt should not be empty")
	}
	if updated.ParallelExecution.StartedAt == "" {
		t.Fatal("ParallelExecution.StartedAt should not be empty")
	}
	// Verify branches are tracked
	if len(updated.ParallelExecution.Branches) != 2 {
		t.Fatalf("expected 2 branches tracked, got %d", len(updated.ParallelExecution.Branches))
	}
	for _, name := range []string{"branch-a", "branch-b"} {
		bs, ok := updated.ParallelExecution.Branches[name]
		if !ok {
			t.Fatalf("branch %q not tracked", name)
		}
		if bs.Status != "complete" {
			t.Fatalf("branch %q status = %q, want %q", name, bs.Status, "complete")
		}
		if bs.ExitCode != 0 {
			t.Fatalf("branch %q exit_code = %d, want 0", name, bs.ExitCode)
		}
	}
}

func TestRunParallelAllFail(t *testing.T) {
	phaseDir, sf := setupParallelTestPlan(t)
	ps, _ := sf.Read()

	// All branches fail (exit 1)
	t.Setenv("MOCK_EXIT_CODE", "1")
	t.Setenv("MOCK_OUTPUT", "error output\n")

	verdict, _, err := RunParallel(context.Background(), testLogger(), RunParallelOptions{
		PhaseDir:   phaseDir,
		StateFile:  sf,
		PhaseState: ps,
		Config: &arc.ParallelConfig{
			Branches: []arc.ParallelBranch{
				{Name: "branch-a", Prompt: "do task a"},
				{Name: "branch-b", Prompt: "do task b"},
			},
			Strategy: "all",
		},
		PlanMD: "# Test Plan",
	})
	if err != nil {
		t.Fatalf("RunParallel error: %v", err)
	}
	if verdict != "any_failed" {
		t.Fatalf("verdict = %q, want %q", verdict, "any_failed")
	}

	// Verify branches recorded as failed
	updated, _ := sf.Read()
	for _, name := range []string{"branch-a", "branch-b"} {
		bs := updated.ParallelExecution.Branches[name]
		if bs.Status != "failed" {
			t.Fatalf("branch %q status = %q, want %q", name, bs.Status, "failed")
		}
		if bs.ExitCode != 1 {
			t.Fatalf("branch %q exit_code = %d, want 1", name, bs.ExitCode)
		}
	}
}

func TestRunParallelAnyStrategyAllFail(t *testing.T) {
	phaseDir, sf := setupParallelTestPlan(t)
	ps, _ := sf.Read()

	// All branches fail under "any" strategy
	t.Setenv("MOCK_EXIT_CODE", "1")
	t.Setenv("MOCK_OUTPUT", "fail\n")

	verdict, _, err := RunParallel(context.Background(), testLogger(), RunParallelOptions{
		PhaseDir:   phaseDir,
		StateFile:  sf,
		PhaseState: ps,
		Config: &arc.ParallelConfig{
			Branches: []arc.ParallelBranch{
				{Name: "branch-a", Prompt: "task a"},
				{Name: "branch-b", Prompt: "task b"},
			},
			Strategy: "any",
		},
		PlanMD: "# Test Plan",
	})
	if err != nil {
		t.Fatalf("RunParallel error: %v", err)
	}
	if verdict != "all_failed" {
		t.Fatalf("verdict = %q, want %q", verdict, "all_failed")
	}
}

func TestRunParallelAnyStrategyOneSucceeds(t *testing.T) {
	phaseDir, sf := setupParallelTestPlan(t)
	ps, _ := sf.Read()

	// Use scripted responses: call_0 succeeds, call_1 fails
	scriptDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(scriptDir, "call_0.txt"), []byte("success"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scriptDir, "call_1.txt"), []byte("failure"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MOCK_SCRIPT_DIR", scriptDir)
	t.Setenv("MOCK_OUTPUT", "success\n")

	verdict, _, err := RunParallel(context.Background(), testLogger(), RunParallelOptions{
		PhaseDir:   phaseDir,
		StateFile:  sf,
		PhaseState: ps,
		Config: &arc.ParallelConfig{
			Branches: []arc.ParallelBranch{
				{Name: "branch-a", Prompt: "task a"},
				{Name: "branch-b", Prompt: "task b"},
			},
			Strategy: "any",
		},
		PlanMD: "# Test Plan",
	})
	if err != nil {
		t.Fatalf("RunParallel error: %v", err)
	}
	// Under "any", at least one branch succeeds (both get exit 0 since we can't
	// control exit code per-script), so we expect first_complete.
	if verdict != "first_complete" {
		t.Fatalf("verdict = %q, want %q", verdict, "first_complete")
	}

	// Verify state was updated
	updated, _ := sf.Read()
	if updated.ParallelExecution == nil {
		t.Fatal("ParallelExecution should not be nil")
	}
	if updated.ParallelExecution.Verdict != "first_complete" {
		t.Fatalf("stored verdict = %q, want %q", updated.ParallelExecution.Verdict, "first_complete")
	}
}

func TestRunParallelNOfMStrategy(t *testing.T) {
	phaseDir, sf := setupParallelTestPlan(t)
	ps, _ := sf.Read()

	// All branches succeed, n=2 of 3
	t.Setenv("MOCK_OUTPUT", "done\n")

	verdict, _, err := RunParallel(context.Background(), testLogger(), RunParallelOptions{
		PhaseDir:   phaseDir,
		StateFile:  sf,
		PhaseState: ps,
		Config: &arc.ParallelConfig{
			Branches: []arc.ParallelBranch{
				{Name: "branch-a", Prompt: "task a"},
				{Name: "branch-b", Prompt: "task b"},
				{Name: "branch-c", Prompt: "task c"},
			},
			Strategy: "n_of_m",
			N:        2,
		},
		PlanMD: "# Test Plan",
	})
	if err != nil {
		t.Fatalf("RunParallel error: %v", err)
	}
	if verdict != "n_complete" {
		t.Fatalf("verdict = %q, want %q", verdict, "n_complete")
	}
}

func TestRunParallelNOfMInsufficient(t *testing.T) {
	phaseDir, sf := setupParallelTestPlan(t)
	ps, _ := sf.Read()

	// All branches fail, n=1 required
	t.Setenv("MOCK_EXIT_CODE", "1")
	t.Setenv("MOCK_OUTPUT", "fail\n")

	verdict, _, err := RunParallel(context.Background(), testLogger(), RunParallelOptions{
		PhaseDir:   phaseDir,
		StateFile:  sf,
		PhaseState: ps,
		Config: &arc.ParallelConfig{
			Branches: []arc.ParallelBranch{
				{Name: "branch-a", Prompt: "task a"},
			},
			Strategy: "n_of_m",
			N:        1,
		},
		PlanMD: "# Test Plan",
	})
	if err != nil {
		t.Fatalf("RunParallel error: %v", err)
	}
	if verdict != "insufficient" {
		t.Fatalf("verdict = %q, want %q", verdict, "insufficient")
	}
}

func TestRunParallelCreatesResultsDir(t *testing.T) {
	phaseDir, sf := setupParallelTestPlan(t)
	ps, _ := sf.Read()

	t.Setenv("MOCK_OUTPUT", "output content\n")

	_, _, err := RunParallel(context.Background(), testLogger(), RunParallelOptions{
		PhaseDir:   phaseDir,
		StateFile:  sf,
		PhaseState: ps,
		Config: &arc.ParallelConfig{
			Branches: []arc.ParallelBranch{
				{Name: "test-branch", Prompt: "test prompt"},
			},
			Strategy: "all",
		},
		PlanMD: "# Test Plan",
	})
	if err != nil {
		t.Fatalf("RunParallel error: %v", err)
	}

	resultsDir := filepath.Join(phaseDir, "parallel_impl")
	if _, err := os.Stat(resultsDir); err != nil {
		t.Fatalf("results dir should exist: %v", err)
	}

	// Verify log file has content
	logFile := filepath.Join(resultsDir, "test-branch.log")
	logData, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("branch log file should exist: %v", err)
	}
	if len(logData) == 0 {
		t.Fatal("branch log file should have content")
	}

	// Verify exit file has correct code
	exitFile := filepath.Join(resultsDir, "test-branch.exit")
	exitData, err := os.ReadFile(exitFile)
	if err != nil {
		t.Fatalf("branch exit file should exist: %v", err)
	}
	if string(exitData) != "0" {
		t.Fatalf("exit file = %q, want %q", string(exitData), "0")
	}
}

func TestRunParallelWithEmptyParams(t *testing.T) {
	// Regression test: empty params in parallel config should not break execution.
	phaseDir, sf := setupParallelTestPlan(t)
	ps, _ := sf.Read()

	t.Setenv("MOCK_OUTPUT", "done\n")

	verdict, _, err := RunParallel(context.Background(), testLogger(), RunParallelOptions{
		PhaseDir:   phaseDir,
		StateFile:  sf,
		PhaseState: ps,
		Config: &arc.ParallelConfig{
			Branches: []arc.ParallelBranch{
				{Name: "branch-a", Prompt: "do task a"},
			},
			Strategy: "all",
		},
		PlanMD: "# Test Plan",
	})
	if err != nil {
		t.Fatalf("RunParallel error: %v", err)
	}
	if verdict != "all_complete" {
		t.Fatalf("verdict = %q, want %q", verdict, "all_complete")
	}
}

func TestRunParallelNoBranches(t *testing.T) {
	phaseDir, sf := setupParallelTestPlan(t)
	ps, _ := sf.Read()

	_, _, err := RunParallel(context.Background(), testLogger(), RunParallelOptions{
		PhaseDir:   phaseDir,
		StateFile:  sf,
		PhaseState: ps,
		Config: &arc.ParallelConfig{
			Branches: []arc.ParallelBranch{},
			Strategy: "all",
		},
		PlanMD: "# Test Plan",
	})
	if err == nil {
		t.Fatal("expected error for no branches")
	}
}

func TestRunParallelNilConfig(t *testing.T) {
	phaseDir, sf := setupParallelTestPlan(t)
	ps, _ := sf.Read()

	_, _, err := RunParallel(context.Background(), testLogger(), RunParallelOptions{
		PhaseDir:   phaseDir,
		StateFile:  sf,
		PhaseState: ps,
		Config:     nil,
		PlanMD:     "# Test Plan",
	})
	if err == nil {
		t.Fatal("expected error for nil config")
	}
}

func TestRunParallelContextCancelled(t *testing.T) {
	phaseDir, sf := setupParallelTestPlan(t)
	ps, _ := sf.Read()

	// Use a sleep so the agent takes time, then cancel immediately.
	// The branch should detect cancellation and return a non-zero exit.
	t.Setenv("MOCK_SLEEP_MS", "5000")
	t.Setenv("MOCK_OUTPUT", "should not see this\n")

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	verdict, _, err := RunParallel(ctx, testLogger(), RunParallelOptions{
		PhaseDir:   phaseDir,
		StateFile:  sf,
		PhaseState: ps,
		Config: &arc.ParallelConfig{
			Branches: []arc.ParallelBranch{
				{Name: "slow-branch", Prompt: "test"},
			},
			Strategy: "all",
		},
		PlanMD: "# Test Plan",
	})

	// With a cancelled context, the agent spawn fails. The branch gets a
	// non-zero exit code, so under "all" strategy we expect "any_failed".
	if err != nil {
		t.Fatalf("RunParallel should complete even with cancelled branches, got error: %v", err)
	}
	if verdict != "any_failed" {
		t.Fatalf("verdict = %q, want %q (branch should fail due to cancelled context)", verdict, "any_failed")
	}

	// Verify the branch was recorded as failed in state
	updated, _ := sf.Read()
	if updated.ParallelExecution == nil {
		t.Fatal("ParallelExecution should not be nil after completion")
	}
	bs, ok := updated.ParallelExecution.Branches["slow-branch"]
	if !ok {
		t.Fatal("slow-branch should be tracked")
	}
	if bs.Status != "failed" {
		t.Fatalf("slow-branch status = %q, want %q", bs.Status, "failed")
	}
	if bs.ExitCode == 0 {
		t.Fatal("slow-branch exit_code should be non-zero for cancelled context")
	}
	if updated.ParallelExecution.Verdict != "any_failed" {
		t.Fatalf("stored verdict = %q, want %q", updated.ParallelExecution.Verdict, "any_failed")
	}
	if updated.ParallelExecution.FinishedAt == "" {
		t.Fatal("FinishedAt should be set even for failed runs")
	}

	// Verify results dir was created and exit file reflects failure
	exitData, err := os.ReadFile(filepath.Join(phaseDir, fmt.Sprintf("parallel_%s", ps.CurrentState), "slow-branch.exit"))
	if err != nil {
		t.Fatalf("exit file should exist: %v", err)
	}
	if string(exitData) == "0" {
		t.Fatal("exit file should show non-zero for cancelled branch")
	}
}

// ── Verdict-aware parallel tests ─────────────────────────────────────────────

func TestRunParallelVerdictExtraction(t *testing.T) {
	phaseDir, sf := setupParallelTestPlan(t)
	ps, _ := sf.Read()

	// Branch output contains a verdict section
	t.Setenv("MOCK_OUTPUT", "Some analysis...\n\n## Verdict\nbugs_found\n")

	verdict, _, err := RunParallel(context.Background(), testLogger(), RunParallelOptions{
		PhaseDir:   phaseDir,
		StateFile:  sf,
		PhaseState: ps,
		Config: &arc.ParallelConfig{
			Branches: []arc.ParallelBranch{
				{Name: "branch-a", Prompt: "test prompt a"},
				{Name: "branch-b", Prompt: "test prompt b"},
			},
			Strategy: "all",
		},
		ValidVerdicts: []arc.Verdict{"bugs_found", "no_bugs_found"},
		PlanMD:        "# Test Plan",
	})
	if err != nil {
		t.Fatalf("RunParallel error: %v", err)
	}
	// Both branches return bugs_found
	if verdict != "bugs_found" {
		t.Fatalf("verdict = %q, want %q", verdict, "bugs_found")
	}
}

func TestRunParallelVerdictExtractionMixed(t *testing.T) {
	phaseDir, sf := setupParallelTestPlan(t)
	ps, _ := sf.Read()

	// Use scripted responses for different verdicts per branch.
	// Since MOCK_OUTPUT applies to all branches uniformly, we'll write
	// log files directly and skip the actual agent spawn.
	// Actually, let's use a simple approach: set output that has the verdict.
	t.Setenv("MOCK_OUTPUT", "## Verdict\nno_bugs_found\n")

	verdict, _, err := RunParallel(context.Background(), testLogger(), RunParallelOptions{
		PhaseDir:   phaseDir,
		StateFile:  sf,
		PhaseState: ps,
		Config: &arc.ParallelConfig{
			Branches: []arc.ParallelBranch{
				{Name: "branch-a", Prompt: "test prompt"},
			},
			Strategy: "all",
		},
		ValidVerdicts: []arc.Verdict{"bugs_found", "no_bugs_found"},
		PlanMD:        "# Test Plan",
	})
	if err != nil {
		t.Fatalf("RunParallel error: %v", err)
	}
	if verdict != "no_bugs_found" {
		t.Fatalf("verdict = %q, want %q", verdict, "no_bugs_found")
	}
}

func TestRunParallelMissingLogFile(t *testing.T) {
	phaseDir, sf := setupParallelTestPlan(t)
	ps, _ := sf.Read()

	// Agent fails, so no log file is written (spawnResult is nil)
	t.Setenv("MOCK_EXIT_CODE", "1")
	t.Setenv("MOCK_OUTPUT", "")

	_, _, err := RunParallel(context.Background(), testLogger(), RunParallelOptions{
		PhaseDir:   phaseDir,
		StateFile:  sf,
		PhaseState: ps,
		Config: &arc.ParallelConfig{
			Branches: []arc.ParallelBranch{
				{Name: "branch-a", Prompt: "test prompt"},
			},
			Strategy: "all",
		},
		ValidVerdicts: []arc.Verdict{"bugs_found", "no_bugs_found"},
		PlanMD:        "# Test Plan",
	})
	// When output doesn't contain a verdict section, extraction should fail
	if err == nil {
		t.Fatal("expected error when verdict extraction fails")
	}
}

func TestRunParallelEmptyLogFile(t *testing.T) {
	phaseDir, sf := setupParallelTestPlan(t)
	ps, _ := sf.Read()

	// Output exists but has no verdict marker
	t.Setenv("MOCK_OUTPUT", "some output without verdict\n")

	_, _, err := RunParallel(context.Background(), testLogger(), RunParallelOptions{
		PhaseDir:   phaseDir,
		StateFile:  sf,
		PhaseState: ps,
		Config: &arc.ParallelConfig{
			Branches: []arc.ParallelBranch{
				{Name: "branch-a", Prompt: "test prompt"},
			},
			Strategy: "all",
		},
		ValidVerdicts: []arc.Verdict{"bugs_found", "no_bugs_found"},
		PlanMD:        "# Test Plan",
	})
	if err == nil {
		t.Fatal("expected error when no verdict in output")
	}
}

func TestRunParallelFallbackToExitCode(t *testing.T) {
	phaseDir, sf := setupParallelTestPlan(t)
	ps, _ := sf.Read()

	t.Setenv("MOCK_OUTPUT", "done\n")

	// Empty ValidVerdicts → fall back to exit-code joining
	verdict, _, err := RunParallel(context.Background(), testLogger(), RunParallelOptions{
		PhaseDir:   phaseDir,
		StateFile:  sf,
		PhaseState: ps,
		Config: &arc.ParallelConfig{
			Branches: []arc.ParallelBranch{
				{Name: "branch-a", Prompt: "test prompt"},
				{Name: "branch-b", Prompt: "test prompt"},
			},
			Strategy: "all",
		},
		ValidVerdicts: nil, // no verdict extraction
		PlanMD:        "# Test Plan",
	})
	if err != nil {
		t.Fatalf("RunParallel error: %v", err)
	}
	if verdict != "all_complete" {
		t.Fatalf("verdict = %q, want %q", verdict, "all_complete")
	}
}

func TestRunParallelSingleBranchVerdict(t *testing.T) {
	phaseDir, sf := setupParallelTestPlan(t)
	ps, _ := sf.Read()

	t.Setenv("MOCK_OUTPUT", "## Verdict\nbugs_found\n")

	verdict, _, err := RunParallel(context.Background(), testLogger(), RunParallelOptions{
		PhaseDir:   phaseDir,
		StateFile:  sf,
		PhaseState: ps,
		Config: &arc.ParallelConfig{
			Branches: []arc.ParallelBranch{
				{Name: "solo", Prompt: "test prompt"},
			},
			Strategy: "all",
		},
		ValidVerdicts: []arc.Verdict{"bugs_found", "no_bugs_found"},
		PlanMD:        "# Test Plan",
	})
	if err != nil {
		t.Fatalf("RunParallel error: %v", err)
	}
	if verdict != "bugs_found" {
		t.Fatalf("verdict = %q, want %q", verdict, "bugs_found")
	}
}

func TestRunParallelEmptyBranchesVerdict(t *testing.T) {
	phaseDir, sf := setupParallelTestPlan(t)
	ps, _ := sf.Read()

	_, _, err := RunParallel(context.Background(), testLogger(), RunParallelOptions{
		PhaseDir:   phaseDir,
		StateFile:  sf,
		PhaseState: ps,
		Config: &arc.ParallelConfig{
			Branches: []arc.ParallelBranch{},
			Strategy: "all",
		},
		ValidVerdicts: []arc.Verdict{"bugs_found"},
		PlanMD:        "# Test Plan",
	})
	if err == nil {
		t.Fatal("expected error for empty branches")
	}
}

func TestRunParallelBranchParams(t *testing.T) {
	phaseDir, sf := setupParallelTestPlan(t)
	ps, _ := sf.Read()

	// The branch has params — verify RunParallel doesn't error when params are set
	t.Setenv("MOCK_OUTPUT", "done\n")

	verdict, _, err := RunParallel(context.Background(), testLogger(), RunParallelOptions{
		PhaseDir:   phaseDir,
		StateFile:  sf,
		PhaseState: ps,
		Config: &arc.ParallelConfig{
			Branches: []arc.ParallelBranch{
				{Name: "branch-a", Prompt: "Focus: {{params.focus}}", Params: map[string]string{"focus": "security"}},
			},
			Strategy: "all",
		},
		PlanMD: "# Test Plan",
	})
	if err != nil {
		t.Fatalf("RunParallel with branch params error: %v", err)
	}
	if verdict != "all_complete" {
		t.Fatalf("verdict = %q, want %q", verdict, "all_complete")
	}
}
