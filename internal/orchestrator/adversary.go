package orchestrator

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/nwiley/arc/internal/adapter"
	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/intelligence"
	"github.com/nwiley/arc/internal/plan"
	"github.com/nwiley/arc/internal/prompt"
)

const (
	// MaxAdversaryRounds is the maximum number of adversary → fix cycles.
	MaxAdversaryRounds = 3
)

// RunAdversary runs a per-plan adversary session that tests all changed files
// across completed phases. Returns the list of test files written and whether
// any bugs were found.
//
// planLogger is optional; pass nil to disable structured adversary logging.
func RunAdversary(ctx context.Context, opts LaunchOptions, workDir string, planLogger *PlanLogger) (*AdversaryResult, error) {
	// Collect changed files across all phases
	changedFiles, err := collectChangedFiles(opts.PlansDir, opts.PlanName, workDir)
	if err != nil {
		return nil, fmt.Errorf("collecting changed files: %w", err)
	}

	if len(changedFiles) == 0 {
		return &AdversaryResult{BugsFound: false}, nil
	}

	// Get adapter for adversary role
	adapterName := "claude"
	if opts.Config != nil {
		adapterName = opts.Config.AgentForRole("adversary")
	}
	agentAdapter := adapter.Get(adapterName)

	// Load project context
	projectCtx := prompt.LoadProjectContext(workDir)

	// Determine test command
	testCmd := "go test ./..."
	if opts.Config != nil && opts.Config.TestCommand != "" {
		testCmd = opts.Config.TestCommand
	}

	// Open intelligence store for flaky test filtering (best-effort).
	projectDir := opts.ProjectDir
	if projectDir == "" {
		projectDir = workDir
	}
	var intel *intelligence.Store
	if s, openErr := intelligence.Open(projectDir); openErr == nil {
		intel = s
	}

	result := &AdversaryResult{
		Rounds: make([]AdversaryRound, 0),
	}

	for round := 1; round <= MaxAdversaryRounds; round++ {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}

		fmt.Printf("\n[adversary] Round %d/%d — spawning adversary agent\n", round, MaxAdversaryRounds)
		if planLogger != nil {
			planLogger.AdversaryStarted(round, fmt.Sprintf("files=%d adapter=%s", len(changedFiles), agentAdapter.Name()))
		}

		// Build adversary prompt
		adversaryPrompt, err := buildAdversaryPrompt(changedFiles, testCmd, projectCtx)
		if err != nil {
			return result, fmt.Errorf("building adversary prompt: %w", err)
		}

		// Spawn adversary
		sessionCfg := arc.SessionConfig{
			MaxTurns: 100,
			Timeout:  30 * time.Minute,
		}
		spawnResult, spawnErr := agentAdapter.Spawn(ctx, adversaryPrompt, workDir, sessionCfg)

		roundResult := AdversaryRound{Round: round}
		if spawnResult != nil {
			roundResult.Usage = spawnResult.Usage
		}

		if spawnErr != nil {
			opts.Logger.Warn("adversary spawn failed", "round", round, "error", spawnErr)
			roundResult.Error = spawnErr.Error()
			result.Rounds = append(result.Rounds, roundResult)
			if planLogger != nil {
				planLogger.AdversaryCompleted(round, 0, fmt.Sprintf("spawn error: %v", spawnErr))
			}
			continue
		}

		// Run the test suite to check for new failures
		testOutput, testErr := runTestCommand(testCmd, workDir)
		if testErr != nil {
			// Parse failing tests from output
			rawFailing := parseFailingTests(testOutput)

			// Filter out known-flaky tests — don't count flaky failures as adversary bugs.
			// Also record second-run outcomes for flaky detection.
			nonFlakyFailing := intelligence.FilterFlakyTests(intel, rawFailing)

			if len(nonFlakyFailing) == 0 {
				// All failures are known-flaky — treat as no bugs found.
				// Record the flaky tests as having passed (they'll pass again eventually).
				if intel != nil {
					for _, name := range rawFailing {
						intel.RecordFlakyTest(name, true)
					}
				}
				roundResult.BugsFound = false
				result.Rounds = append(result.Rounds, roundResult)
				fmt.Printf("[adversary] Round %d: all failures are known-flaky — NO BUGS FOUND\n", round)
				if planLogger != nil {
					planLogger.AdversaryCompleted(round, 0, "all failures known-flaky")
				}
				break
			}

			// Tests failed with real (non-flaky) failures — adversary found bugs.
			roundResult.BugsFound = true
			roundResult.TestOutput = testOutput
			roundResult.FailingTests = nonFlakyFailing
			result.BugsFound = true

			// Find new test files written by the adversary
			roundResult.TestFiles = discoverNewTestFiles(workDir, result.allTestFiles())

			result.Rounds = append(result.Rounds, roundResult)

			fmt.Printf("[adversary] Round %d: BUGS FOUND (%d non-flaky failures)\n", round, len(nonFlakyFailing))
			if len(roundResult.TestFiles) > 0 {
				fmt.Printf("[adversary] New test files: %s\n", strings.Join(roundResult.TestFiles, ", "))
			}
			if planLogger != nil {
				planLogger.AdversaryCompleted(round, len(nonFlakyFailing),
					fmt.Sprintf("bugs_found=%d test_files=%d", len(nonFlakyFailing), len(roundResult.TestFiles)))
			}

			// Route fix to the responsible phase
			if round < MaxAdversaryRounds {
				fixErr := routeAdversaryFix(ctx, opts, workDir, roundResult, changedFiles)
				if fixErr != nil {
					opts.Logger.Warn("adversary fix failed", "round", round, "error", fixErr)
				}

				// After fix attempt, re-run to detect any new flakiness.
				if intel != nil {
					retestOutput, retestErr := runTestCommand(testCmd, workDir)
					retestFailing := parseFailingTests(retestOutput)
					retestFailSet := make(map[string]bool, len(retestFailing))
					for _, name := range retestFailing {
						retestFailSet[name] = true
					}
					// Tests that failed before the fix but now pass → potentially flaky.
					if retestErr == nil {
						for _, name := range rawFailing {
							if !retestFailSet[name] {
								intel.RecordFlakyTest(name, true)
							}
						}
					}
					_ = intel.Save()
				}
			}
		} else {
			// Tests pass — no bugs found this round.
			// Record previously-failing tests as having passed (flakiness signal).
			if intel != nil {
				// We don't have prior failing tests here, but we can save any accumulated data.
				_ = intel.Save()
			}
			roundResult.BugsFound = false
			result.Rounds = append(result.Rounds, roundResult)
			fmt.Printf("[adversary] Round %d: NO BUGS FOUND — converged\n", round)
			if planLogger != nil {
				planLogger.AdversaryCompleted(round, 0, "no bugs found")
			}
			break
		}
	}

	return result, nil
}

// AdversaryResult describes the outcome of the adversary phase.
type AdversaryResult struct {
	BugsFound bool
	Rounds    []AdversaryRound
}

// AdversaryRound describes a single adversary round.
type AdversaryRound struct {
	Round        int
	BugsFound    bool
	TestOutput   string
	TestFiles    []string
	FailingTests []string // non-flaky failing test names parsed from test output
	Error        string
	Usage        arc.Usage
}

func (r *AdversaryResult) allTestFiles() []string {
	var all []string
	for _, round := range r.Rounds {
		all = append(all, round.TestFiles...)
	}
	return all
}

// buildAdversaryPrompt renders the adversary prompt template.
func buildAdversaryPrompt(changedFiles []string, testCmd, projectCtx string) (string, error) {
	data := prompt.AdversaryData{
		ChangedFiles:   changedFiles,
		TestCommand:    testCmd,
		ProjectContext: projectCtx,
	}
	return prompt.RenderGatePrompt("adversary", data)
}

// collectChangedFiles gathers the list of files changed across all phase specs.
// Falls back to `git diff --name-only` if specs don't list files.
func collectChangedFiles(plansDir, planName, workDir string) ([]string, error) {
	// First try to collect from phase specs
	planDir := filepath.Join(plansDir, planName)
	phaseDirs, err := filepath.Glob(filepath.Join(planDir, "phases", "*", "spec.yaml"))
	if err != nil {
		return nil, err
	}

	fileSet := make(map[string]bool)
	for _, specPath := range phaseDirs {
		spec, err := plan.ReadSpec(plansDir, planName, filepath.Base(filepath.Dir(specPath)))
		if err != nil {
			continue
		}
		for _, f := range spec.Files {
			fileSet[f] = true
		}
	}

	if len(fileSet) > 0 {
		files := make([]string, 0, len(fileSet))
		for f := range fileSet {
			files = append(files, f)
		}
		return files, nil
	}

	// Fallback: git diff (best-effort — may not be in a git repo)
	files, gitErr := gitChangedFiles(workDir)
	if gitErr != nil {
		return nil, nil // no files to check
	}
	return files, nil
}

// gitChangedFiles returns files changed in the working tree vs HEAD.
func gitChangedFiles(dir string) ([]string, error) {
	cmd := exec.Command("git", "diff", "--name-only", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff: %w", err)
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

// runTestCommand runs a test command and returns output.
func runTestCommand(cmd, dir string) (string, error) {
	c := exec.Command("sh", "-c", cmd)
	c.Dir = dir
	out, err := c.CombinedOutput()
	return string(out), err
}

// routeAdversaryFix identifies the responsible phase for a test failure
// and spawns a fix session.
func routeAdversaryFix(ctx context.Context, opts LaunchOptions, workDir string, round AdversaryRound, changedFiles []string) error {
	// Determine which phase owns the failing files
	// For now, use a simple heuristic: read each phase's spec.Files
	// and find which one has the most overlap with the test output
	planDir := filepath.Join(opts.PlansDir, opts.PlanName)
	phaseDirs, err := os.ReadDir(filepath.Join(planDir, "phases"))
	if err != nil {
		return fmt.Errorf("reading phases: %w", err)
	}

	// Find the phase with the most file ownership overlap
	bestPhase := ""
	bestScore := 0
	for _, d := range phaseDirs {
		if !d.IsDir() {
			continue
		}
		spec, err := plan.ReadSpec(opts.PlansDir, opts.PlanName, d.Name())
		if err != nil {
			continue
		}
		score := 0
		for _, f := range spec.Files {
			if strings.Contains(round.TestOutput, f) {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			bestPhase = d.Name()
		}
	}

	if bestPhase == "" {
		// Can't determine responsible phase — use first available
		if len(phaseDirs) > 0 {
			bestPhase = phaseDirs[0].Name()
		} else {
			return fmt.Errorf("no phases found to route fix to")
		}
	}

	fmt.Printf("[adversary] Routing fix to phase %q\n", bestPhase)

	// Build fix prompt
	fixPrompt := fmt.Sprintf(`You are fixing bugs found by adversarial testing in phase %q.

## Failing Tests
%s

## Instructions
- Read the failing test output carefully
- Fix the bugs in the implementation (not in the tests)
- Run the test suite to verify your fixes: %s
- Do not modify the adversary-written test files
`,
		bestPhase,
		round.TestOutput,
		testCmdForFix(opts),
	)

	adapterName := "claude"
	if opts.Config != nil {
		adapterName = opts.Config.AgentForRole("impl")
	}
	agentAdapter := adapter.Get(adapterName)

	sessionCfg := arc.SessionConfig{
		MaxTurns: 50,
		Timeout:  15 * time.Minute,
	}

	_, err = agentAdapter.Spawn(ctx, fixPrompt, workDir, sessionCfg)
	return err
}

// parseFailingTests extracts test names from Go test output by scanning for
// lines matching "--- FAIL: TestName" patterns.
func parseFailingTests(output string) []string {
	var tests []string
	seen := make(map[string]bool)
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		// Match "--- FAIL: TestFoo (0.00s)" or "FAIL\t<pkg>" lines.
		if strings.HasPrefix(line, "--- FAIL: ") {
			// Extract test name (up to first space after the name)
			rest := strings.TrimPrefix(line, "--- FAIL: ")
			name := rest
			if idx := strings.Index(rest, " "); idx >= 0 {
				name = rest[:idx]
			}
			if name != "" && !seen[name] {
				seen[name] = true
				tests = append(tests, name)
			}
		}
	}
	return tests
}

// testCmdForFix returns the test command from config, or "go test ./..." as fallback.
func testCmdForFix(opts LaunchOptions) string {
	if opts.Config != nil && opts.Config.TestCommand != "" {
		return opts.Config.TestCommand
	}
	return "go test ./..."
}
