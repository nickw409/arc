package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitCreatesStructure(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	writeFile(t, filepath.Join(dir, "go.mod"), "module github.com/example/myapp\n\ngo 1.22\n")

	_, err := Init(InitOptions{ProjectRoot: dir})
	if err != nil {
		t.Fatalf("Init error: %v", err)
	}

	// Check expected files/dirs exist
	paths := []string{
		".arc.yaml",
		".plans/active",
		".plans/archive",
		".claude/commands/arc-plan.md",
	}
	for _, p := range paths {
		full := filepath.Join(dir, p)
		if _, err := os.Stat(full); os.IsNotExist(err) {
			t.Fatalf("expected %s to exist after Init", p)
		}
	}
}

func TestInitAlreadyInitialized(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	writeFile(t, filepath.Join(dir, ".arc.yaml"), "language: go\n")

	_, err := Init(InitOptions{ProjectRoot: dir})
	if err == nil {
		t.Fatal("expected error for already initialized project, got nil")
	}
	if !strings.Contains(err.Error(), "already initialized") {
		t.Fatalf("error = %q, want containing %q", err.Error(), "already initialized")
	}
}

func TestInitForceOverwrites(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	writeFile(t, filepath.Join(dir, "go.mod"), "module github.com/example/myapp\n\ngo 1.22\n")
	writeFile(t, filepath.Join(dir, ".arc.yaml"), "language: unknown\n")

	_, err := Init(InitOptions{ProjectRoot: dir, Force: true})
	if err != nil {
		t.Fatalf("Init with Force=true error: %v", err)
	}

	// .arc.yaml should be regenerated
	if _, err := os.Stat(filepath.Join(dir, ".arc.yaml")); os.IsNotExist(err) {
		t.Fatal(".arc.yaml should exist after force init")
	}
}

func TestInitNoGitRepo(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module github.com/example/myapp\n\ngo 1.22\n")

	_, err := Init(InitOptions{ProjectRoot: dir})
	if err != nil {
		t.Fatalf("Init without git should not error, got: %v", err)
	}

	// .arc.yaml and .plans/ should be created
	if _, err := os.Stat(filepath.Join(dir, ".arc.yaml")); os.IsNotExist(err) {
		t.Fatal(".arc.yaml should exist")
	}
	if _, err := os.Stat(filepath.Join(dir, ".plans", "active")); os.IsNotExist(err) {
		t.Fatal(".plans/active should exist")
	}

	// No hooks should be installed (no .git/)
	if _, err := os.Stat(filepath.Join(dir, ".git", "hooks", "pre-commit")); !os.IsNotExist(err) {
		t.Fatal("git hooks should not be installed without .git/")
	}
}

func TestGitignoreNoDuplicate(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	writeFile(t, filepath.Join(dir, "go.mod"), "module github.com/example/myapp\n\ngo 1.22\n")
	// Pre-existing .gitignore with .plans/
	writeFile(t, filepath.Join(dir, ".gitignore"), ".plans/\n")

	_, err := Init(InitOptions{ProjectRoot: dir})
	if err != nil {
		t.Fatalf("Init error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("ReadFile .gitignore error: %v", err)
	}

	count := strings.Count(string(data), ".plans/")
	if count > 1 {
		t.Fatalf(".gitignore contains %d entries for .plans/, want 1 (no duplicate)", count)
	}
}

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".git", "hooks"), 0755); err != nil {
		t.Fatalf("initGitRepo mkdir error: %v", err)
	}
}
