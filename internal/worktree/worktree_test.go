package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initTestRepo creates an isolated git repository with an initial commit.
func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git cmd %v failed: %v\n%s", args, err, out)
		}
	}

	// Create initial commit
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "add", "-A")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command("git", "commit", "-m", "initial")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	return dir
}

func TestCreateAndRemove(t *testing.T) {
	projectDir := initTestRepo(t)

	wt, err := Create(projectDir, "my-plan", "my-phase")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify worktree dir exists
	if _, err := os.Stat(wt.Dir); os.IsNotExist(err) {
		t.Fatal("worktree dir does not exist")
	}

	// Verify it's a git repo on the correct branch
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = wt.Dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git branch failed: %v", err)
	}
	branch := strings.TrimSpace(string(out))
	if branch != "arc/my-plan/my-phase" {
		t.Fatalf("expected branch arc/my-plan/my-phase, got %q", branch)
	}

	// Remove and verify cleanup
	if err := Remove(wt); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	if _, err := os.Stat(wt.Dir); !os.IsNotExist(err) {
		t.Fatal("worktree dir still exists after Remove")
	}

	// Verify branch was deleted
	cmd = exec.Command("git", "branch", "--list", wt.Branch)
	cmd.Dir = projectDir
	out, err = cmd.Output()
	if err != nil {
		t.Fatalf("git branch --list failed: %v", err)
	}
	if strings.TrimSpace(string(out)) != "" {
		t.Fatal("branch still exists after Remove")
	}
}

func TestMergeBack(t *testing.T) {
	projectDir := initTestRepo(t)

	wt, err := Create(projectDir, "merge-plan", "merge-phase")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	defer Remove(wt)

	// Commit a file in the worktree
	testFile := filepath.Join(wt.Dir, "new_file.txt")
	if err := os.WriteFile(testFile, []byte("hello from worktree\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "add", "-A")
	cmd.Dir = wt.Dir
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command("git", "commit", "-m", "add new_file from worktree")
	cmd.Dir = wt.Dir
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	// Merge back
	hash, err := MergeBack(wt)
	if err != nil {
		t.Fatalf("MergeBack failed: %v", err)
	}
	if hash == "" {
		t.Fatal("expected non-empty merge commit hash")
	}

	// Verify file exists in project dir
	if _, err := os.Stat(filepath.Join(projectDir, "new_file.txt")); os.IsNotExist(err) {
		t.Fatal("merged file does not exist in project dir")
	}
}

func TestMergeConflict(t *testing.T) {
	projectDir := initTestRepo(t)

	wt, err := Create(projectDir, "conflict-plan", "conflict-phase")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	defer Remove(wt)

	// Write conflicting content in the worktree
	if err := os.WriteFile(filepath.Join(wt.Dir, "README.md"), []byte("worktree version\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "add", "-A")
	cmd.Dir = wt.Dir
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command("git", "commit", "-m", "worktree change")
	cmd.Dir = wt.Dir
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	// Write conflicting content in the main repo
	if err := os.WriteFile(filepath.Join(projectDir, "README.md"), []byte("main version\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command("git", "add", "-A")
	cmd.Dir = projectDir
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command("git", "commit", "-m", "main change")
	cmd.Dir = projectDir
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	// Attempt merge — should fail with conflict
	_, err = MergeBack(wt)
	if err == nil {
		t.Fatal("expected MergeBack to return error on conflict")
	}
	if !strings.Contains(err.Error(), "merge conflict") {
		t.Fatalf("expected 'merge conflict' in error, got: %v", err)
	}
}

func TestBranchNaming(t *testing.T) {
	tests := []struct {
		plan, phase string
		want        string
	}{
		{"my-plan", "my-phase", "arc/my-plan/my-phase"},
		{"plan with spaces", "phase/special", "arc/plan-with-spaces/phase/special"},
		{"plan@#$%", "phase!!!", "arc/plan/phase"},
		{"my-plan", "", "arc/my-plan"},
	}

	for _, tc := range tests {
		branch := "arc/" + sanitizeBranch(tc.plan)
		if tc.phase != "" {
			branch += "/" + sanitizeBranch(tc.phase)
		}
		if branch != tc.want {
			t.Errorf("branchName(%q, %q) = %q, want %q", tc.plan, tc.phase, branch, tc.want)
		}
	}
}

func TestCreatePlanLevel(t *testing.T) {
	projectDir := initTestRepo(t)

	wt, err := Create(projectDir, "shared-plan", "")
	if err != nil {
		t.Fatalf("Create with empty phaseName failed: %v", err)
	}
	defer Remove(wt)

	// Verify branch name has no trailing slash
	if wt.Branch != "arc/shared-plan" {
		t.Fatalf("expected branch arc/shared-plan, got %q", wt.Branch)
	}

	// Verify the worktree is on the correct branch
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = wt.Dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git branch failed: %v", err)
	}
	branch := strings.TrimSpace(string(out))
	if branch != "arc/shared-plan" {
		t.Fatalf("expected checked-out branch arc/shared-plan, got %q", branch)
	}
}

func TestCleanupPlan(t *testing.T) {
	projectDir := initTestRepo(t)

	// Create two per-phase worktrees for "my-plan"
	wt1, err := Create(projectDir, "my-plan", "phase-a")
	if err != nil {
		t.Fatalf("Create phase-a failed: %v", err)
	}
	wt2, err := Create(projectDir, "my-plan", "phase-b")
	if err != nil {
		t.Fatalf("Create phase-b failed: %v", err)
	}

	// Create a worktree for a different plan (should not be removed)
	wtOther, err := Create(projectDir, "other-plan", "phase-x")
	if err != nil {
		t.Fatalf("Create other-plan failed: %v", err)
	}

	// Verify all worktree dirs exist
	for _, wt := range []*Worktree{wt1, wt2, wtOther} {
		if _, err := os.Stat(wt.Dir); os.IsNotExist(err) {
			t.Fatalf("worktree dir %s should exist", wt.Dir)
		}
	}

	// Clean up "my-plan"
	n := CleanupPlan(projectDir, "my-plan")
	if n != 2 {
		t.Fatalf("expected 2 worktrees removed, got %d", n)
	}

	// my-plan worktrees should be gone
	for _, wt := range []*Worktree{wt1, wt2} {
		if _, err := os.Stat(wt.Dir); !os.IsNotExist(err) {
			t.Fatalf("worktree dir %s should have been removed", wt.Dir)
		}
	}

	// other-plan worktree should still exist
	if _, err := os.Stat(wtOther.Dir); os.IsNotExist(err) {
		t.Fatal("other-plan worktree should not have been removed")
	}

	// Clean up
	Remove(wtOther)
}

func TestCleanupPlanNoWorktrees(t *testing.T) {
	projectDir := initTestRepo(t)

	n := CleanupPlan(projectDir, "nonexistent-plan")
	if n != 0 {
		t.Fatalf("expected 0 worktrees removed, got %d", n)
	}
}
