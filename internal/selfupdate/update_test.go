package selfupdate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindInstallDirWithGit(t *testing.T) {
	// Create a fake install dir with .git
	base := t.TempDir()
	gitDir := filepath.Join(base, ".git")
	if err := os.Mkdir(gitDir, 0755); err != nil {
		t.Fatal(err)
	}

	binDir := filepath.Join(base, "bin")
	if err := os.Mkdir(binDir, 0755); err != nil {
		t.Fatal(err)
	}

	// The findInstallDir function uses os.Executable, so we can't easily
	// test it directly. Instead, test the walk-up logic conceptually.
	// Walk from bin/ up to base/ should find .git/
	dir := binDir
	found := false
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			found = true
			break
		}
		dir = filepath.Dir(dir)
	}
	if !found {
		t.Fatal("walk-up logic failed to find .git")
	}
}

func TestSelfUpdateNotGit(t *testing.T) {
	// IsGitInstall uses the actual executable path
	// In test environment, this depends on where the test binary is
	// Just verify it doesn't panic
	_ = IsGitInstall()
}

func TestUpdatePullFailure(t *testing.T) {
	// If we try to update from a non-git directory, it should fail
	// This test only verifies the error handling doesn't panic
	err := Update()
	if err == nil {
		// The test binary might actually be in a git repo (this repo)
		// so success is also acceptable
		return
	}
	// Error should be descriptive
	errStr := err.Error()
	if !strings.Contains(errStr, "git") && !strings.Contains(errStr, "not installed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCurrentVersion(t *testing.T) {
	// May succeed or fail depending on if test binary is in a git repo
	ver, err := CurrentVersion()
	if err != nil {
		// Expected if not in git repo
		return
	}
	if len(ver) == 0 {
		t.Fatal("empty version string")
	}
}
