package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nwiley/arc/internal/config"
	"github.com/nwiley/arc/internal/runner"
	"github.com/spf13/cobra"
)

func newTestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test <file>",
		Short: "Run scoped tests for a specific test file",
		Args:  cobra.ExactArgs(1),
		RunE:  runTest,
	}
	cmd.Flags().StringP("filter", "f", "", "Run only tests matching this pattern")
	cmd.Flags().DurationP("timeout", "t", 5*time.Minute, "Test execution timeout")
	cmd.Flags().Bool("json", false, "Output results as JSON")
	return cmd
}

func runTest(cmd *cobra.Command, args []string) error {
	testFile := args[0]

	filter, _ := cmd.Flags().GetString("filter")
	timeout, _ := cmd.Flags().GetDuration("timeout")
	jsonOutput, _ := cmd.Flags().GetBool("json")

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	// Load config, ignore errors (fallback to defaults)
	cfg, _ := config.Load(cwd)

	var language, runnerName, testCommand string
	if cfg != nil {
		language = cfg.Language
		runnerName = cfg.Runner
		testCommand = cfg.TestCommand
	}

	// If no runner or test_command configured, fall back based on file extension
	if runnerName == "" && testCommand == "" {
		runnerName = detectRunner(testFile)
		if runnerName == "" {
			cmd.SilenceUsage = true
			return fmt.Errorf("no runner configured in .arc.yaml for %s", testFile)
		}
	}

	opts := runner.RunBuiltinOptions{
		TestFile:    testFile,
		Filter:      filter,
		Timeout:     timeout,
		Dir:         cwd,
		Language:    language,
		Runner:      runnerName,
		TestCommand: testCommand,
	}

	result, err := runner.RunBuiltin(context.Background(), opts)
	if err != nil {
		cmd.SilenceUsage = true
		return fmt.Errorf("running tests: %w", err)
	}

	if jsonOutput {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	fmt.Fprintln(cmd.OutOrStdout(), result.Summary())
	if result.Failed > 0 {
		cmd.SilenceUsage = true
		return fmt.Errorf("tests failed: %d failure(s)", result.Failed)
	}
	return nil
}

// detectRunner guesses the runner from the test file extension.
func detectRunner(testFile string) string {
	if strings.HasSuffix(testFile, "_test.go") {
		return "go-test"
	}
	// Other language detection deferred to multi-language phase
	return ""
}
