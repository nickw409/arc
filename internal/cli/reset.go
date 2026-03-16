package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nwiley/arc/internal/plan"
	"github.com/nwiley/arc/internal/worktree"
	"github.com/spf13/cobra"
)

func newResetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reset <plan-name>",
		Short: "Reset a plan for re-execution: clean worktrees, reset phase states, remove locks",
		Long: `Reset a failed or stalled plan so it can be re-run from scratch.

This command:
  1. Removes all git worktrees for the plan (and their branches)
  2. Resets all phase states to initial (pending, iteration 0, no attempt log)
  3. Removes stale orchestrator lock and PID files
  4. Removes log files

After reset, the plan is ready for 'arc daemon submit'.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			planName := args[0]

			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}

			plansDir := filepath.Join(cwd, ".plans", "active")
			planDir := filepath.Join(plansDir, planName)

			// Verify plan exists.
			if _, err := os.Stat(filepath.Join(planDir, "plan.json")); err != nil {
				return fmt.Errorf("plan %q not found", planName)
			}

			// 1. Remove worktrees (and their branches).
			displayWorktreeFailureReasons(cwd, planName)
			removed := worktree.CleanupPlan(cwd, planName)
			if removed > 0 {
				fmt.Printf("Removed %d worktree(s)\n", removed)
			}

			// 2. Reset all phase states.
			opts := plan.ManageOptions{
				PlansDir: plansDir,
				PlanName: planName,
			}
			if err := plan.ManageResetPlan(opts); err != nil {
				return fmt.Errorf("resetting phase states: %w", err)
			}
			fmt.Println("Reset all phase states to pending")

			// 3. Remove stale lock file.
			lockPath := filepath.Join(planDir, ".orchestrator.lock")
			if err := os.Remove(lockPath); err == nil {
				fmt.Println("Removed orchestrator lock")
			}

			// 4. Remove stale PID file.
			pidRemoved, err := removeStallPIDFile(planDir)
			if err != nil {
				return fmt.Errorf("cleaning PID file: %w", err)
			}
			if pidRemoved {
				fmt.Println("Removed stale PID file")
			}

			// 5. Remove log files.
			logsDir := filepath.Join(planDir, "logs")
			if entries, err := os.ReadDir(logsDir); err == nil {
				for _, e := range entries {
					os.Remove(filepath.Join(logsDir, e.Name()))
				}
				fmt.Println("Cleared log files")
			}

			fmt.Printf("\nPlan %q is ready to re-run.\n", planName)
			return nil
		},
	}
	return cmd
}
