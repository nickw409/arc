package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nwiley/arc/internal/gate"
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

			// Resolve spec path.
			specPath := filepath.Join(".plans", "active", planName, "phases", phaseName, "spec.yaml")
			if _, err := os.Stat(specPath); os.IsNotExist(err) {
				return fmt.Errorf("spec not found at %s — does phase %q exist in plan %q?", specPath, phaseName, planName)
			}

			// Determine the phase directory for status persistence.
			phaseDir := filepath.Join(".plans", "active", planName, "phases", phaseName)

			// Run gate.
			result, err := gate.Run(cmd.Context(), specPath, workdir)
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
