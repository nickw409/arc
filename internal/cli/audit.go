package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
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
	var format string

	cmd := &cobra.Command{
		Use:   "audit [file...]",
		Short: "Run adversarial testing against code changes",
		Long: `Run adversarial testing against arbitrary code changes.

Without flags, audits uncommitted changes (git diff --name-only).
With --branch, diffs the named branch against the current branch.
With --diff, uses an explicit git diff range (e.g. origin/main...HEAD).
Positional arguments are treated as explicit file paths to audit.

The --format flag controls output format:
  text    (default) Human-readable output printed to stdout.
  github  GitHub Actions annotation format for CI integration.

Examples:
  arc audit                                          # audit uncommitted changes
  arc audit --branch feature/auth                    # audit changes on a branch vs HEAD
  arc audit --diff origin/main...HEAD                # audit a diff range
  arc audit internal/api/auth.go                     # audit specific files
  arc audit --diff origin/main...HEAD --format github  # GitHub Actions CI mode`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if branch != "" && diffRange != "" {
				return fmt.Errorf("--branch and --diff are mutually exclusive")
			}
			if format != "text" && format != "github" {
				return fmt.Errorf("--format must be one of: text, github")
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

			if format == "text" {
				fmt.Printf("[audit] Auditing %d file(s):\n", len(files))
				for _, f := range files {
					fmt.Printf("  %s\n", f)
				}
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

			if format == "text" {
				fmt.Println("[audit] Spawning adversary agent...")
			}
			sessionCfg := arc.SessionConfig{
				MaxTurns: 100,
				Timeout:  30 * time.Minute,
			}
			result, err := agentAdapter.Spawn(ctx, adversaryPrompt, projectRoot, sessionCfg)
			if err != nil {
				return fmt.Errorf("spawning adversary agent: %w", err)
			}

			if result.TimedOut {
				if format == "github" {
					fmt.Println("::warning::arc audit: adversary agent timed out")
				} else {
					fmt.Fprintln(os.Stderr, "[audit] Warning: adversary agent timed out")
				}
			}

			if format == "text" {
				fmt.Println("[audit] Running test suite...")
			}
			testOutput, testErr := runAuditTestCommand(testCmd, projectRoot)
			if testErr != nil {
				if format == "github" {
					printGitHubTestFailure(testOutput)
				} else {
					fmt.Printf("[audit] BUGS FOUND — tests failed:\n%s\n", testOutput)
				}
				cmd.SilenceUsage = true
				return fmt.Errorf("audit found bugs: tests failed")
			}

			if format == "text" {
				if result.Usage.CostUSD > 0 {
					fmt.Printf("[audit] Cost: $%.4f\n", result.Usage.CostUSD)
				}
				fmt.Println("[audit] No bugs found — all tests pass.")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&branch, "branch", "", "Audit changes on this branch vs HEAD")
	cmd.Flags().StringVar(&diffRange, "diff", "", "Audit an explicit git diff range (e.g. origin/main...HEAD)")
	cmd.Flags().StringVar(&format, "format", "text", "Output format: text or github")
	return cmd
}

// printGitHubTestFailure emits GitHub Actions annotation lines for test failures
// parsed from go test output. Falls back to a single error annotation when
// individual test names and locations cannot be extracted.
func printGitHubTestFailure(testOutput string) {
	annotations := parseTestFailuresAsAnnotations(testOutput)
	if len(annotations) == 0 {
		// Emit a single catch-all error annotation.
		msg := sanitizeAnnotationMessage(testOutput)
		if msg == "" {
			msg = "tests failed"
		}
		fmt.Printf("::error::%s\n", msg)
		return
	}
	for _, a := range annotations {
		fmt.Println(a)
	}
}

// testFailRe matches "--- FAIL: TestName (Xs)" lines in go test output.
var testFailRe = regexp.MustCompile(`^--- FAIL: (\S+)`)

// fileLineRe matches file:line references like "foo_test.go:42:" in test output.
var fileLineRe = regexp.MustCompile(`(\S+\.go):(\d+):`)

// parseTestFailuresAsAnnotations converts go test output into GitHub Actions
// annotation strings of the form ::error file={f},line={n}::{msg}.
func parseTestFailuresAsAnnotations(output string) []string {
	lines := strings.Split(output, "\n")

	type failBlock struct {
		testName string
		file     string
		line     string
		msgs     []string
	}

	var blocks []failBlock
	var current *failBlock

	for _, line := range lines {
		if m := testFailRe.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			if current != nil {
				blocks = append(blocks, *current)
			}
			current = &failBlock{testName: m[1]}
			continue
		}
		if current != nil {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				current.msgs = append(current.msgs, trimmed)
				// Try to extract a file:line reference from this line.
				if current.file == "" {
					if fm := fileLineRe.FindStringSubmatch(trimmed); fm != nil {
						current.file = fm[1]
						current.line = fm[2]
					}
				}
			}
		}
	}
	if current != nil {
		blocks = append(blocks, *current)
	}

	var annotations []string
	for _, b := range blocks {
		msg := b.testName
		if len(b.msgs) > 0 {
			detail := sanitizeAnnotationMessage(strings.Join(b.msgs, " "))
			if detail != "" {
				msg = b.testName + ": " + detail
			}
		}
		if b.file != "" {
			annotations = append(annotations, fmt.Sprintf("::error file=%s,line=%s::%s", b.file, b.line, msg))
		} else {
			annotations = append(annotations, fmt.Sprintf("::error file=unknown,line=1::%s", msg))
		}
	}
	return annotations
}

// sanitizeAnnotationMessage removes newlines and truncates long messages so they
// fit cleanly in a GitHub Actions annotation value.
func sanitizeAnnotationMessage(s string) string {
	// Collapse whitespace/newlines to a single space.
	s = strings.Join(strings.Fields(s), " ")
	const maxLen = 500
	if len(s) > maxLen {
		s = s[:maxLen] + "..."
	}
	return s
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
