package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/nwiley/arc/internal/worktree"
	"github.com/spf13/cobra"
)

// newCleanupCmd is a hidden alias for arc reset (backwards compatibility).
func newCleanupCmd() *cobra.Command {
	cmd := newResetCmd()
	cmd.Use = "cleanup <plan-name>"
	cmd.Short = "Alias for 'arc reset'"
	cmd.Hidden = true
	return cmd
}

// displayWorktreeFailureReasons finds all git worktrees for the given plan,
// reads their .arc/worktree-meta.json (if present), and prints the failure
// reason to stdout before they are deleted.
func displayWorktreeFailureReasons(projectDir, planName string) {
	prefix := "arc/" + planName

	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = projectDir
	out, err := cmd.Output()
	if err != nil {
		return
	}

	var currentDir, currentBranch string
	for _, line := range strings.Split(string(out), "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			currentDir = strings.TrimPrefix(line, "worktree ")
			currentBranch = ""
		case strings.HasPrefix(line, "branch refs/heads/"):
			currentBranch = strings.TrimPrefix(line, "branch refs/heads/")
		case line == "":
			if currentBranch == prefix || strings.HasPrefix(currentBranch, prefix+"/") {
				meta, metaErr := worktree.ReadMetadata(currentDir)
				if metaErr == nil && meta != nil {
					fmt.Printf("Worktree %s (branch %s):\n", currentDir, currentBranch)
					fmt.Printf("  Phase:   %s\n", meta.Phase)
					fmt.Printf("  Failure: %s\n", meta.FailureReason)
					fmt.Printf("  Time:    %s\n", meta.Timestamp)
				}
			}
			currentDir = ""
			currentBranch = ""
		}
	}
}

// pidIsAlive returns true if a process with the given PID is alive.
func pidIsAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

// removeStallPIDFile checks whether the orchestrator PID recorded in
// orchestrator.pid is still alive. If the process is gone, the file is
// removed and true is returned.
func removeStallPIDFile(planDir string) (bool, error) {
	pidPath := filepath.Join(planDir, "orchestrator.pid")
	pidData, err := os.ReadFile(pidPath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
	if err != nil {
		return true, os.Remove(pidPath)
	}

	if pidIsAlive(pid) {
		return false, nil
	}

	return true, os.Remove(pidPath)
}
