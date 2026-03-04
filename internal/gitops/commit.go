package gitops

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/nwiley/arc/internal/config"
)

// CommitOptions configures a git commit operation.
type CommitOptions struct {
	Message string
	Dir     string // working directory for git commands; empty uses process cwd
	Config  *config.Config
}

// Commit stages all changes and creates a commit.
func Commit(opts CommitOptions) (string, error) {
	if strings.TrimSpace(opts.Message) == "" {
		return "", fmt.Errorf("commit message cannot be empty")
	}

	// Check if there are any changes to stage
	statusCmd := exec.Command("git", "status", "--porcelain")
	statusCmd.Dir = opts.Dir
	statusOut, err := statusCmd.Output()
	if err != nil {
		return "", fmt.Errorf("git status failed: %w", err)
	}

	if len(strings.TrimSpace(string(statusOut))) == 0 {
		return "", nil
	}

	// Stage all changes
	addCmd := exec.Command("git", "add", "-A")
	addCmd.Dir = opts.Dir
	if err := addCmd.Run(); err != nil {
		return "", fmt.Errorf("git add failed: %w", err)
	}

	// Build commit args
	args := []string{"commit", "-m", opts.Message}
	if opts.Config != nil && opts.Config.Git.Sign {
		args = append(args, "-S")
	}

	commitCmd := exec.Command("git", args...)
	commitCmd.Dir = opts.Dir
	if err := commitCmd.Run(); err != nil {
		return "", fmt.Errorf("git commit failed: %w", err)
	}

	// Get the commit hash
	hashCmd := exec.Command("git", "rev-parse", "HEAD")
	hashCmd.Dir = opts.Dir
	hashOut, err := hashCmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse failed: %w", err)
	}

	return strings.TrimSpace(string(hashOut)), nil
}

// FormatCommitMessage formats a commit message based on config style.
func FormatCommitMessage(style, commitType, scope, description string) string {
	switch style {
	case "conventional":
		if scope != "" {
			return fmt.Sprintf("%s(%s): %s", commitType, scope, description)
		}
		return fmt.Sprintf("%s: %s", commitType, description)
	default:
		// freeform and unknown styles
		if scope != "" {
			return fmt.Sprintf("%s: %s", scope, description)
		}
		return description
	}
}
