package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/nwiley/arc/internal/adapter"
	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/config"
	"github.com/nwiley/arc/internal/prompt"
	"github.com/spf13/cobra"
)

func newAuditCmd() *cobra.Command {
	var branch string
	var diffRange string

	cmd := &cobra.Command{
		Use:   "audit [file...]",
		Short: "Run adversarial testing against code changes",
		Long: `Run adversarial testing against arbitrary code changes.

Without flags, audits uncommitted changes (git diff --name-only).
With --branch, diffs the named branch against the current branch.
With --diff, uses an explicit git diff range (e.g. origin/main...HEAD).
Positional arguments are treated as explicit file paths to audit.

Examples:
  arc audit                              # audit uncommitted changes
  arc audit --branch feature/auth        # audit changes on a branch vs HEAD
  arc audit --diff origin/main...HEAD    # audit a diff range
  arc audit internal/api/auth.go         # audit specific files`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if branch != "" && diffRange != "" {
				return fmt.Errorf("--branch and --diff are mutually exclusive")
			}

			projectRoot, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}

			// Collect the list of files to audit.
			var files []string
			switch {
			case len(args) > 0:
				files = args
			case branch != "":
				files, err = gitDiffFiles(projectRoot, branch+"...HEAD")
				if err != nil {
					return fmt.Errorf("collecting changed files from branch %q: %w", branch, err)
				}
			case diffRange != "":
				files, err = gitDiffFiles(projectRoot, diffRange)
				if err != nil {
					return fmt.Errorf("collecting changed files from diff range %q: %w", diffRange, err)
				}
			default:
				files, err = gitUncommittedFiles(projectRoot)
				if err != nil {
					return fmt.Errorf("collecting uncommitted changes: %w", err)
				}
			}

			if len(files) == 0 {
				fmt.Println("No changed files found — nothing to audit.")
				return nil
			}

			fmt.Printf("[audit] Auditing %d file(s):\n", len(files))
			for _, f := range files {
				fmt.Printf("  %s\n", f)
			}

			// Load config (non-fatal if missing).
			cfg, _ := config.Load(projectRoot)

			adapterName := "claude"
			testCmd := "go test ./..."
			if cfg != nil {
				adapterName = cfg.AgentForRole("adversary")
				if cfg.TestCommand != "" {
					testCmd = cfg.TestCommand
				}
			}

			projectCtx := prompt.LoadProjectContext(projectRoot)

			adversaryPrompt, err := prompt.RenderGatePrompt("adversary", prompt.AdversaryData{
				ChangedFiles:   files,
				TestCommand:    testCmd,
				ProjectContext: projectCtx,
			})
			if err != nil {
				return fmt.Errorf("building adversary prompt: %w", err)
			}

			agentAdapter := adapter.Get(adapterName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			go func() {
				<-sigCh
				fmt.Fprintf(os.Stderr, "\nReceived interrupt, shutting down...\n")
				cancel()
			}()

			fmt.Println("[audit] Spawning adversary agent...")
			sessionCfg := arc.SessionConfig{
				MaxTurns: 100,
				Timeout:  30 * time.Minute,
			}
			result, err := agentAdapter.Spawn(ctx, adversaryPrompt, projectRoot, sessionCfg)
			if err != nil {
				return fmt.Errorf("spawning adversary agent: %w", err)
			}

			if result.TimedOut {
				fmt.Fprintln(os.Stderr, "[audit] Warning: adversary agent timed out")
			}

			fmt.Println("[audit] Running test suite...")
			testOutput, testErr := runAuditTestCommand(testCmd, projectRoot)
			if testErr != nil {
				fmt.Printf("[audit] BUGS FOUND — tests failed:\n%s\n", testOutput)
				cmd.SilenceUsage = true
				return fmt.Errorf("audit found bugs: tests failed")
			}

			if result.Usage.CostUSD > 0 {
				fmt.Printf("[audit] Cost: $%.4f\n", result.Usage.CostUSD)
			}
			fmt.Println("[audit] No bugs found — all tests pass.")
			return nil
		},
	}

	cmd.Flags().StringVar(&branch, "branch", "", "Audit changes on this branch vs HEAD")
	cmd.Flags().StringVar(&diffRange, "diff", "", "Audit an explicit git diff range (e.g. origin/main...HEAD)")
	return cmd
}

// gitDiffFiles returns the list of files changed in the given git diff range.
func gitDiffFiles(dir, diffRange string) ([]string, error) {
	cmd := exec.Command("git", "diff", "--name-only", diffRange)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff --name-only %s: %w", diffRange, err)
	}
	return splitLines(string(out)), nil
}

// gitUncommittedFiles returns files with uncommitted changes (staged or unstaged).
func gitUncommittedFiles(dir string) ([]string, error) {
	// Collect staged changes (index vs HEAD).
	staged, err := runGitDiffNameOnly(dir, "--cached")
	if err != nil {
		return nil, err
	}
	// Collect unstaged changes (working tree vs index).
	unstaged, err := runGitDiffNameOnly(dir)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	var files []string
	for _, f := range append(staged, unstaged...) {
		if !seen[f] {
			seen[f] = true
			files = append(files, f)
		}
	}
	return files, nil
}

// runGitDiffNameOnly runs `git diff --name-only` with optional extra args.
func runGitDiffNameOnly(dir string, extra ...string) ([]string, error) {
	args := append([]string{"diff", "--name-only"}, extra...)
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff --name-only %s: %w", strings.Join(extra, " "), err)
	}
	return splitLines(string(out)), nil
}

// splitLines splits newline-delimited output into non-empty trimmed strings.
func splitLines(s string) []string {
	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// runAuditTestCommand runs the test command and returns combined output.
func runAuditTestCommand(testCmd, dir string) (string, error) {
	c := exec.Command("sh", "-c", testCmd)
	c.Dir = dir
	out, err := c.CombinedOutput()
	return string(out), err
}
