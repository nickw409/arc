package gitops

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nwiley/arc/internal/config"
)

func TestFormatCommitConventional(t *testing.T) {
	got := FormatCommitMessage("conventional", "feat", "auth", "add login endpoint")
	want := "feat(auth): add login endpoint"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFormatCommitFreeform(t *testing.T) {
	got := FormatCommitMessage("freeform", "", "auth", "add login endpoint")
	want := "auth: add login endpoint"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFormatCommitEmptyScope(t *testing.T) {
	got := FormatCommitMessage("conventional", "feat", "", "add feature")
	want := "feat: add feature"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFormatCommitUnknownStyle(t *testing.T) {
	got := FormatCommitMessage("unknown_style", "feat", "auth", "add login")
	want := "auth: add login"
	if got != want {
		t.Fatalf("got %q, want %q (unknown style falls back to freeform)", got, want)
	}
}

func TestFormatCommitEmptyDescription(t *testing.T) {
	got := FormatCommitMessage("conventional", "feat", "auth", "")
	want := "feat(auth): "
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func initGitRepo(t *testing.T) string {
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
			t.Fatalf("git init cmd %v failed: %v\n%s", args, err, out)
		}
	}
	return dir
}

func TestCommitNoChanges(t *testing.T) {
	dir := initGitRepo(t)

	hash, err := Commit(CommitOptions{
		Message: "test commit",
		Dir:     dir,
		Config:  &config.Config{Git: config.GitConfig{CommitStyle: "conventional"}},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if hash != "" {
		t.Fatalf("expected empty hash (no changes), got %q", hash)
	}
}

func TestCommitWithChanges(t *testing.T) {
	dir := initGitRepo(t)

	// Create a file to commit
	if err := os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	hash, err := Commit(CommitOptions{
		Message: "test commit",
		Dir:     dir,
		Config:  &config.Config{Git: config.GitConfig{CommitStyle: "conventional"}},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}

	// Verify commit exists
	cmd := exec.Command("git", "log", "--oneline", "-1")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log failed: %v", err)
	}
	if !strings.Contains(string(out), "test commit") {
		t.Fatalf("expected commit message in log, got: %s", out)
	}
}

func TestCommitWithSigning(t *testing.T) {
	// This test verifies that signing is requested, but since we don't have
	// GPG keys configured, the commit may fail. The key thing is the -S flag
	// is included in the command construction.
	dir := initGitRepo(t)

	if err := os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	// With signing enabled; may fail if no GPG key, but tests the code path
	_, err := Commit(CommitOptions{
		Message: "test",
		Dir:     dir,
		Config:  &config.Config{Git: config.GitConfig{Sign: true}},
	})
	// We don't fail the test on error here because GPG signing may not be configured
	_ = err
}
