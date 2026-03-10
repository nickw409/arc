package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/nwiley/arc/internal/gate"
	"github.com/nwiley/arc/internal/plan"
	"github.com/spf13/cobra"
)

func newGateCmd() *cobra.Command {
	var workdir string

	cmd := &cobra.Command{
		Use:   "gate <plan> <phase>",
		Short: "Run gate checks for a phase",
		Long: `Run objective gate checks for a phase and report pass/fail.

Gate checks verify:
  - File existence assertions
  - Pattern grep assertions across .go files
  - Test function existence in _test.go files
  - Checkpoint test commands
  - Phase scoped test command

Exit code 0 means all checks passed. Exit code 1 means one or more checks failed.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			planName := args[0]
			phaseName := args[1]

			// Default workdir to current directory.
			if workdir == "" {
				wd, err := os.Getwd()
				if err != nil {
					return fmt.Errorf("getting working directory: %w", err)
				}
				workdir = wd
			}

			// Find the project root (handles worktrees where CWD may not have .plans/).
			// Use git's common git dir to locate the main worktree root.
			projectRoot := findProjectRoot(workdir)

			// Determine the phase directory for status persistence.
			phaseDir := filepath.Join(projectRoot, ".plans", "active", planName, "phases", phaseName)

			// Load the phase spec from plan.md.
			plansDir := filepath.Join(projectRoot, ".plans", "active")
			spec, err := plan.ReadSpec(plansDir, planName, phaseName)
			if err != nil {
				return fmt.Errorf("reading phase spec: %w", err)
			}

			// Run gate.
			result, err := gate.Run(cmd.Context(), spec, workdir)
			if err != nil {
				return fmt.Errorf("running gate: %w", err)
			}

			// Persist gate status.
			if werr := gate.WriteStatus(phaseDir, result); werr != nil {
				// Non-fatal — log but don't fail the command.
				fmt.Fprintf(os.Stderr, "warning: could not write gate status: %v\n", werr)
			}

			// Increment run count for loop detection.
			runCount, _ := gate.IncrementRunCount(phaseDir)

			// Print formatted result.
			fmt.Print(gate.FormatWithRunCount(result, runCount))

			// Exit 1 on failure.
			if !result.Passed {
				os.Exit(1)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&workdir, "workdir", "", "Working directory for file assertions (default: current directory)")
	return cmd
}

// findProjectRoot returns the root of the main git worktree, which is where
// .plans/ lives. When called from inside a git worktree, git rev-parse
// --git-common-dir returns the absolute path to the shared .git directory
// (e.g. /project/.git), so its parent is the project root. When called from
// the main worktree it returns a relative path (.git), so the CWD is the root.
func findProjectRoot(fromDir string) string {
	out, err := exec.Command("git", "-C", fromDir, "rev-parse", "--git-common-dir").Output()
	if err != nil {
		return fromDir
	}
	gitCommonDir := strings.TrimSpace(string(out))
	if filepath.IsAbs(gitCommonDir) {
		return filepath.Dir(gitCommonDir)
	}
	// Relative path means we're in the main worktree; fromDir is already the root.
	return fromDir
}
