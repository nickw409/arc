package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/nwiley/arc/internal/config"
	"github.com/nwiley/arc/internal/testcmd"
	"github.com/spf13/cobra"
)

func newTestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test <file>",
		Short: "Run scoped tests for a specific test file",
		Args:  cobra.ExactArgs(1),
		RunE:  runTest,
	}
	cmd.Flags().DurationP("timeout", "t", 5*time.Minute, "Test execution timeout")
	cmd.Flags().Bool("json", false, "Output results as JSON")
	return cmd
}

func runTest(cmd *cobra.Command, args []string) error {
	testFile := args[0]

	timeout, _ := cmd.Flags().GetDuration("timeout")
	jsonOutput, _ := cmd.Flags().GetBool("json")

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	cfg, _ := config.Load(cwd)

	tenv := testcmd.NewEnv(testcmd.WithConfig(cfg), testcmd.WithProjectDir(cwd))

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	result, err := tenv.RunFile(ctx, testFile)
	if err != nil {
		cmd.SilenceUsage = true
		return fmt.Errorf("running tests: %w", err)
	}

	if jsonOutput {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	if result.Passed {
		fmt.Fprintln(cmd.OutOrStdout(), "PASS")
	} else {
		fmt.Fprint(cmd.OutOrStdout(), result.Output)
		if len(result.FailedTests) > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "\nFailed: %d test(s)\n", len(result.FailedTests))
		}
		cmd.SilenceUsage = true
		return fmt.Errorf("tests failed")
	}
	return nil
}
