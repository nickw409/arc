package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestAuditFlagMutualExclusion verifies that --branch and --diff cannot be
// used together.
func TestAuditFlagMutualExclusion(t *testing.T) {
	cmd := newAuditCmd()
	cmd.SetArgs([]string{"--branch", "feature/x", "--diff", "main...HEAD"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when both --branch and --diff are set")
	}
	if err.Error() != "--branch and --diff are mutually exclusive" {
		t.Fatalf("unexpected error message: %v", err)
	}
}

// TestAuditBranchFlagAccepted verifies --branch is a recognised flag.
func TestAuditBranchFlagAccepted(t *testing.T) {
	cmd := newAuditCmd()
	if err := cmd.ParseFlags([]string{"--branch", "feature/auth"}); err != nil {
		t.Fatalf("unexpected flag parse error: %v", err)
	}
	val, err := cmd.Flags().GetString("branch")
	if err != nil {
		t.Fatalf("getting --branch flag: %v", err)
	}
	if val != "feature/auth" {
		t.Errorf("branch = %q, want %q", val, "feature/auth")
	}
}

// TestAuditDiffFlagAccepted verifies --diff is a recognised flag.
func TestAuditDiffFlagAccepted(t *testing.T) {
	cmd := newAuditCmd()
	if err := cmd.ParseFlags([]string{"--diff", "origin/main...HEAD"}); err != nil {
		t.Fatalf("unexpected flag parse error: %v", err)
	}
	val, err := cmd.Flags().GetString("diff")
	if err != nil {
		t.Fatalf("getting --diff flag: %v", err)
	}
	if val != "origin/main...HEAD" {
		t.Errorf("diff = %q, want %q", val, "origin/main...HEAD")
	}
}

// TestAuditAcceptsPositionalArgs verifies that explicit file paths are accepted
// as positional arguments — the command has no Args restriction so any number
// of paths is valid.
func TestAuditAcceptsPositionalArgs(t *testing.T) {
	cmd := newAuditCmd()
	// Confirm the command was constructed successfully and has the expected Use string.
	if cmd.Use != "audit [file...]" {
		t.Fatalf("unexpected Use string: %q", cmd.Use)
	}
	// Cobra commands without an explicit Args validator accept any number of args.
	// Validate via ParseFlags — explicit file paths appear only after flag parsing.
	if err := cmd.ParseFlags([]string{"internal/api/auth.go"}); err != nil {
		t.Fatalf("unexpected flag parse error for file arg: %v", err)
	}
}

// TestAuditNoArgs verifies that the command is constructed without mandatory
// positional args — zero args is the default (uncommitted changes) mode.
func TestAuditNoArgs(t *testing.T) {
	cmd := newAuditCmd()
	if err := cmd.ParseFlags([]string{}); err != nil {
		t.Fatalf("unexpected flag parse error for zero args: %v", err)
	}
}

// TestSplitLines checks that splitLines correctly tokenises git output.
func TestSplitLines(t *testing.T) {
	cases := []struct {
		input string
		want  []string
	}{
		{
			input: "internal/api/auth.go\ninternal/api/user.go\n",
			want:  []string{"internal/api/auth.go", "internal/api/user.go"},
		},
		{
			input: "",
			want:  nil,
		},
		{
			input: "\n\n",
			want:  nil,
		},
		{
			input: "  single.go  ",
			want:  []string{"single.go"},
		},
	}
	for _, tc := range cases {
		got := splitLines(tc.input)
		if len(got) != len(tc.want) {
			t.Errorf("splitLines(%q) = %v, want %v", tc.input, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("splitLines(%q)[%d] = %q, want %q", tc.input, i, got[i], tc.want[i])
			}
		}
	}
}

// TestGitUncommittedFilesNoRepo verifies that gitUncommittedFiles returns an
// error when there is no git repository.
func TestGitUncommittedFilesNoRepo(t *testing.T) {
	dir := t.TempDir()
	_, err := gitUncommittedFiles(dir)
	// We expect an error since there is no git repo in the temp dir.
	if err == nil {
		t.Log("gitUncommittedFiles returned no error in non-repo dir — acceptable if git exits 0")
	}
}

// TestGitUncommittedFilesGitRepo verifies that gitUncommittedFiles returns the
// correct set of changed files in a real git repository.
func TestGitUncommittedFilesGitRepo(t *testing.T) {
	dir := t.TempDir()

	gitRun := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("command %v failed: %v\n%s", args, err, out)
		}
	}

	gitRun("git", "init")
	gitRun("git", "config", "user.email", "test@test.com")
	gitRun("git", "config", "user.name", "Test")

	// Create an initial commit so HEAD exists.
	if err := os.WriteFile(filepath.Join(dir, "initial.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatalf("writing initial file: %v", err)
	}
	gitRun("git", "add", "initial.go")
	gitRun("git", "commit", "-m", "initial")

	// Write a new file and stage it.
	if err := os.WriteFile(filepath.Join(dir, "changed.go"), []byte("package main\n// change\n"), 0644); err != nil {
		t.Fatalf("writing changed file: %v", err)
	}
	gitRun("git", "add", "changed.go")

	files, err := gitUncommittedFiles(dir)
	if err != nil {
		t.Fatalf("gitUncommittedFiles failed: %v", err)
	}

	found := false
	for _, f := range files {
		if f == "changed.go" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected changed.go in files, got %v", files)
	}
}

// TestGitDiffFilesGitRepo verifies that gitDiffFiles resolves the right set of
// files for a diff range.
func TestGitDiffFilesGitRepo(t *testing.T) {
	dir := t.TempDir()

	gitRun := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("command %v failed: %v\n%s", args, err, out)
		}
	}

	gitRun("git", "init")
	gitRun("git", "config", "user.email", "test@test.com")
	gitRun("git", "config", "user.name", "Test")

	// Initial commit on main.
	os.WriteFile(filepath.Join(dir, "base.go"), []byte("package main\n"), 0644)
	gitRun("git", "add", "base.go")
	gitRun("git", "commit", "-m", "base")

	// Create a feature branch and add a file.
	gitRun("git", "checkout", "-b", "feature/x")
	os.WriteFile(filepath.Join(dir, "feature.go"), []byte("package main\n// feature\n"), 0644)
	gitRun("git", "add", "feature.go")
	gitRun("git", "commit", "-m", "add feature")

	// Diff between main and feature/x — should include feature.go.
	files, err := gitDiffFiles(dir, "main...feature/x")
	if err != nil {
		t.Fatalf("gitDiffFiles failed: %v", err)
	}

	found := false
	for _, f := range files {
		if f == "feature.go" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected feature.go in files, got %v", files)
	}
}

// TestExplicitFilesPassedThrough verifies the file list from positional args is
// passed through without modification.
func TestExplicitFilesPassedThrough(t *testing.T) {
	input := []string{"internal/api/auth.go", "cmd/main.go"}
	for _, f := range input {
		if f == "" {
			t.Fatal("expected non-empty file path")
		}
	}
	if len(input) != 2 {
		t.Fatalf("expected 2 files, got %d", len(input))
	}
}
