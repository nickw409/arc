package selfupdate

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Update performs a self-update by pulling the latest from git.
func Update() error {
	installDir, err := findInstallDir()
	if err != nil {
		return err
	}

	// Fetch latest
	fetchCmd := exec.Command("git", "-C", installDir, "fetch", "origin")
	fetchCmd.Stdout = os.Stdout
	fetchCmd.Stderr = os.Stderr
	if err := fetchCmd.Run(); err != nil {
		return fmt.Errorf("git fetch failed: %w", err)
	}

	// Pull latest
	pullCmd := exec.Command("git", "-C", installDir, "pull", "origin", "main")
	pullCmd.Stdout = os.Stdout
	pullCmd.Stderr = os.Stderr
	if err := pullCmd.Run(); err != nil {
		return fmt.Errorf("git pull failed: %w", err)
	}

	fmt.Println("Arc updated successfully.")
	return nil
}

// findInstallDir walks up from the executable path to find a .git directory.
func findInstallDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("finding executable: %w", err)
	}

	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return "", fmt.Errorf("resolving symlinks: %w", err)
	}

	dir := filepath.Dir(resolved)
	for i := 0; i < 10; i++ {
		gitDir := filepath.Join(dir, ".git")
		if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("arc not installed via git (no .git found within 10 parent levels of %s)", resolved)
}

// FindInstallDir is exported for testing.
func FindInstallDir() (string, error) {
	return findInstallDir()
}

// IsGitInstall returns true if arc appears to be installed via git clone.
func IsGitInstall() bool {
	_, err := findInstallDir()
	return err == nil
}

// CurrentVersion returns the current git commit hash of the installation.
func CurrentVersion() (string, error) {
	installDir, err := findInstallDir()
	if err != nil {
		return "", err
	}

	cmd := exec.Command("git", "-C", installDir, "rev-parse", "--short", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse failed: %w", err)
	}

	return strings.TrimSpace(string(out)), nil
}
