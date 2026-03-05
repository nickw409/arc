package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/nwiley/arc/internal/worktree"
	"github.com/spf13/cobra"
)

func newCleanupCmd() *cobra.Command {
	var ttl string

	cmd := &cobra.Command{
		Use:   "cleanup <plan-name>",
		Short: "Delete failed worktrees and temp files for a plan",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			planName := args[0]

			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}

			maxAge, err := parseTTL(ttl)
			if err != nil {
				return fmt.Errorf("invalid --ttl value %q: %w", ttl, err)
			}

			// 1. Show failure reasons from worktree metadata, then remove plan worktrees.
			displayWorktreeFailureReasons(cwd, planName)
			removed := worktree.CleanupPlan(cwd, planName)
			if removed > 0 {
				fmt.Printf("Removed %d worktree(s) for plan %q\n", removed, planName)
			} else {
				fmt.Printf("No worktrees found for plan %q\n", planName)
			}

			// 2. Remove stale log files older than TTL.
			logsDir := filepath.Join(cwd, ".plans", "active", planName, "logs")
			logsRemoved, err := removeOldLogs(logsDir, maxAge)
			if err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("cleaning logs: %w", err)
			}
			if logsRemoved > 0 {
				fmt.Printf("Removed %d log file(s) older than %s\n", logsRemoved, ttl)
			}

			// 3. Remove stale PID file if the orchestrator process is not running.
			planDir := filepath.Join(cwd, ".plans", "active", planName)
			pidRemoved, err := removeStallPIDFile(planDir)
			if err != nil {
				return fmt.Errorf("cleaning PID file: %w", err)
			}
			if pidRemoved {
				fmt.Println("Removed stale orchestrator PID file")
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&ttl, "ttl", "7d", "Maximum age of log files to keep (e.g. 7d, 24h)")
	return cmd
}

// parseTTL parses a duration string like "7d" or "24h" into a time.Duration.
// Supports "d" (days) in addition to the standard Go duration suffixes.
func parseTTL(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return 0, fmt.Errorf("invalid day count: %w", err)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}

// removeOldLogs deletes *.jsonl files in logsDir that are older than maxAge.
// Returns the count of files removed.
func removeOldLogs(logsDir string, maxAge time.Duration) (int, error) {
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		return 0, err
	}

	cutoff := time.Now().Add(-maxAge)
	var removed int
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			if err := os.Remove(filepath.Join(logsDir, e.Name())); err == nil {
				removed++
			}
		}
	}
	return removed, nil
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
// Uses signal 0 which checks process existence without sending a real signal.
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
		// Corrupt PID file — remove it.
		return true, os.Remove(pidPath)
	}

	if pidIsAlive(pid) {
		// Process is still running; leave the PID file alone.
		return false, nil
	}

	return true, os.Remove(pidPath)
}
