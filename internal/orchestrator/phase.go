package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/config"
	"github.com/nwiley/arc/internal/gitops"
	"github.com/nwiley/arc/internal/pipeline"
	"github.com/nwiley/arc/internal/resources"
	"github.com/nwiley/arc/internal/runner"
	"github.com/nwiley/arc/internal/state"
	"github.com/nwiley/arc/internal/workflow"
	"github.com/nwiley/arc/internal/worktree"
)

// RunPhaseOptions configures execution of a single phase.
type RunPhaseOptions struct {
	PlanName     string
	PhaseName    string
	PlansDir     string
	ArcHome      string
	ProjectDir   string // working directory for git commits; empty uses process cwd
	Config       *config.Config
	Logger       *slog.Logger
	UseWorktree  bool   // if true, run agents in an isolated git worktree
	WorkingDir   string // override working directory for agents (set by worktree)
	ChatMode     bool   // if true, skip escalation ladder and block immediately
}

// RunPhase executes a single phase from entry state to terminal state.
func RunPhase(ctx context.Context, opts RunPhaseOptions) error {
	phaseDir := filepath.Join(opts.PlansDir, opts.PlanName, "phases", opts.PhaseName)

	// Load state
	sf := state.NewStateFile(filepath.Join(phaseDir, "state.json"))
	phaseState, err := sf.Read()
	if err != nil {
		return fmt.Errorf("reading state: %w", err)
	}

	// Load workflow
	wfBytes, err := resources.WorkflowBytes(phaseState.WorkflowType)
	if err != nil {
		return fmt.Errorf("loading workflow: %w", err)
	}
	wf, err := workflow.LoadBytes(wfBytes)
	if err != nil {
		return fmt.Errorf("parsing workflow: %w", err)
	}
	machine := workflow.NewMachine(wf)

	// Set initial state if empty
	if phaseState.CurrentState == "" {
		phaseState.CurrentState = machine.EntryState()
		if err := sf.Write(phaseState); err != nil {
			return fmt.Errorf("writing initial state: %w", err)
		}
	}

	// Set up worktree isolation if requested
	var wt *worktree.Worktree
	if opts.UseWorktree {
		projectDir := opts.ProjectDir
		if projectDir == "" {
			projectDir, _ = os.Getwd()
		}
		wt, err = worktree.Create(projectDir, opts.PlanName, opts.PhaseName)
		if err != nil {
			opts.Logger.Warn("failed to create worktree, running in-tree", "error", err)
		} else {
			opts.WorkingDir = wt.Dir
			defer func() {
				if phaseState != nil && phaseState.PhaseStatus == "complete" {
					if hash, mergeErr := worktree.MergeBack(wt); mergeErr != nil {
						opts.Logger.Warn("worktree merge failed", "error", mergeErr)
					} else {
						fmt.Printf("[%s] Merged worktree: %s\n", opts.PhaseName, hash[:7])
					}
				}
				worktree.Remove(wt)
			}()
		}
	}

	// Run iteration loop
	const maxConsecutiveRetries = 5
	consecutiveRetries := 0
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Re-read state to pick up any changes
		phaseState, err = sf.Read()
		if err != nil {
			return fmt.Errorf("reading state: %w", err)
		}

		// Check if terminal
		if machine.IsTerminal(phaseState.CurrentState) {
			return nil
		}

		// Sync phase status with current state so the monitor reflects what's happening
		if status := pipeline.MapStateToStatus(phaseState.CurrentState); phaseState.PhaseStatus != status && phaseState.PhaseStatus != "blocked" && phaseState.PhaseStatus != "disputed" {
			sf.Update(func(s *arc.PhaseState) error {
				s.PhaseStatus = status
				return nil
			})
			phaseState.PhaseStatus = status
		}

		// Handle disputed state — needs AI judgment
		if phaseState.PhaseStatus == "disputed" {
			if err := handleDispute(ctx, opts, sf, phaseState); err != nil {
				return err
			}
			continue
		}

		// Handle blocked state
		if phaseState.PhaseStatus == "blocked" {
			return fmt.Errorf("phase is blocked: %s", stringOrDefault(phaseState.Blocked.Reason, "unknown"))
		}

		// Derive mode from state name
		mode := deriveMode(phaseState.CurrentState)

		// Check for stuck iterations and generate instructions
		instructions := ""
		if !opts.ChatMode && phaseState.StuckIterations >= 3 && mode == "impl" {
			instructions, err = generateStuckInstructions(ctx, opts, phaseState)
			if err != nil {
				opts.Logger.Warn("failed to generate stuck instructions", "error", err)
			}
		}

		// Increment per-state iteration counter before running
		curState := phaseState.CurrentState
		sf.Update(func(s *arc.PhaseState) error {
			if s.StateIterations == nil {
				s.StateIterations = make(map[string]int)
			}
			s.StateIterations[curState]++
			return nil
		})
		if phaseState.StateIterations == nil {
			phaseState.StateIterations = make(map[string]int)
		}
		phaseState.StateIterations[curState]++

		fmt.Printf("[%s] %s iteration %d", opts.PhaseName, mode, phaseState.StateIterations[curState])
		if phaseState.TestsTotal > 0 {
			fmt.Printf(" (tests: %d/%d)", phaseState.TestsPassing, phaseState.TestsTotal)
		}
		fmt.Println()

		// Run iteration
		result := pipeline.RunIteration(ctx, opts.Logger, pipeline.IterateOptions{
			PlanName:     opts.PlanName,
			PhaseName:    opts.PhaseName,
			Mode:         mode,
			Instructions: instructions,
			PlansDir:     opts.PlansDir,
			ArcHome:      opts.ArcHome,
			WorkingDir:   opts.WorkingDir,
			ChatMode:     opts.ChatMode,
		})

		// Accumulate usage from this iteration into phase state
		if !result.Usage.IsZero() {
			sf.Update(func(s *arc.PhaseState) error {
				s.Usage = s.Usage.Add(result.Usage)
				return nil
			})
		}

		if result.Err != nil {
			switch result.Action {
			case arc.ActionRetry:
				consecutiveRetries++
				if consecutiveRetries >= maxConsecutiveRetries {
					fmt.Printf("[%s] Blocked: %d consecutive retries on %q\n", opts.PhaseName, consecutiveRetries, phaseState.CurrentState)
					reason := fmt.Sprintf("max consecutive retries (%d) in state %s: %v", consecutiveRetries, phaseState.CurrentState, result.Err)
					sf.Update(func(s *arc.PhaseState) error {
						s.PhaseStatus = "blocked"
						s.Blocked = arc.BlockedInfo{IsBlocked: true, Reason: &reason}
						return nil
					})
					return fmt.Errorf("phase blocked: %s", reason)
				}
				opts.Logger.Warn("iteration failed, retrying", "error", result.Err, "consecutive_retries", consecutiveRetries)
				continue
			case arc.ActionEscalate:
				if err := handleEscalation(ctx, opts, sf, phaseState); err != nil {
					return err
				}
				continue
			case arc.ActionIntervene:
				return fmt.Errorf("intervention required: %w", result.Err)
			default:
				return result.Err
			}
		}

		// Successful iteration — reset retry counter
		consecutiveRetries = 0

		// Re-read state after iteration (agent may have modified it)
		phaseState, err = sf.Read()
		if err != nil {
			return fmt.Errorf("reading state after iteration: %w", err)
		}

		// Post-iteration actions based on what state we're now in
		if err := postIterationActions(ctx, opts, sf, phaseState, machine, result); err != nil {
			return err
		}
	}
}

// postIterationActions handles test running, commits, and reviews after an iteration.
func postIterationActions(ctx context.Context, opts RunPhaseOptions, sf *state.StateFile, phaseState *arc.PhaseState, machine *workflow.Machine, result *arc.IterationResult) error {
	currentState := phaseState.CurrentState
	status := pipeline.MapStateToStatus(currentState)

	// Adversarial workflow actions
	if status == "adversary" || isAdversarialState(currentState, result) {
		return adversarialPostActions(ctx, opts, sf, phaseState, result)
	}

	switch {
	// After QA review approved: log and continue to impl
	case currentState == "impl" && result.Verdict == arc.VerdictApproved && isComingFromQAReview(phaseState):
		fmt.Printf("[%s] QA Review: APPROVED\n", opts.PhaseName)

	// In impl state: run tests after each iteration
	case currentState == "impl" || status == "implementing":
		if err := runAndRecordTests(ctx, opts, sf); err != nil {
			opts.Logger.Warn("test run failed", "error", err)
		}

	// Impl review approved: commit and mark complete
	case currentState == "complete":
		fmt.Printf("[%s] Phase COMPLETE\n", opts.PhaseName)
		desc := phaseObjective(opts)
		commitDir := opts.ProjectDir
		if opts.WorkingDir != "" {
			commitDir = opts.WorkingDir
		}
		hash, err := commitPhase(opts, "feat", desc, commitDir)
		if err != nil {
			opts.Logger.Warn("failed to commit implementation", "error", err)
		} else if hash != "" {
			fmt.Printf("[%s] Committed: feat(%s): %s [%s]\n", opts.PhaseName, opts.PhaseName, desc, hash[:7])
			sf.Update(func(s *arc.PhaseState) error {
				s.LastCommit = hash
				return nil
			})
		}

	// QA review found gaps
	case currentState == "qa" && result.Verdict == arc.VerdictGapsFound:
		fmt.Printf("[%s] QA Review: GAPS_FOUND — re-running QA\n", opts.PhaseName)

	// Impl review has concerns
	case currentState == "impl" && result.Verdict == arc.VerdictConcerns:
		fmt.Printf("[%s] Impl Review: CONCERNS — continuing implementation\n", opts.PhaseName)
	}

	return nil
}

// isAdversarialState checks if the current or previous state involves the adversary loop.
func isAdversarialState(currentState string, result *arc.IterationResult) bool {
	return result.Verdict == arc.VerdictBugsFound || result.Verdict == arc.VerdictNoBugsFound
}

// adversarialPostActions handles post-iteration logic specific to the adversarial workflow.
func adversarialPostActions(ctx context.Context, opts RunPhaseOptions, sf *state.StateFile, phaseState *arc.PhaseState, result *arc.IterationResult) error {
	switch {
	// After adversary found bugs → record new test files, increment round
	case result.Verdict == arc.VerdictBugsFound:
		round := phaseState.AdversaryRound + 1
		fmt.Printf("[%s] Adversary round %d: BUGS_FOUND\n", opts.PhaseName, round)

		// Discover new test files in the working directory
		workDir := opts.WorkingDir
		if workDir == "" {
			workDir = opts.ProjectDir
		}
		newFiles := discoverNewTestFiles(workDir, phaseState.TestFiles)

		sf.Update(func(s *arc.PhaseState) error {
			s.AdversaryRound = round
			if s.AdversaryTests == nil {
				s.AdversaryTests = make(map[string][]string)
			}
			roundKey := fmt.Sprintf("round_%d", round)
			s.AdversaryTests[roundKey] = newFiles
			s.TestFiles = append(s.TestFiles, newFiles...)
			return nil
		})

		if len(newFiles) > 0 {
			fmt.Printf("[%s] Adversary added %d test files\n", opts.PhaseName, len(newFiles))
		}

	// After adversary found no bugs → convergence
	case result.Verdict == arc.VerdictNoBugsFound:
		fmt.Printf("[%s] Adversary: NO_BUGS_FOUND — converged\n", opts.PhaseName)

	// After impl_fix → run tests as sanity check
	case pipeline.MapStateToStatus(phaseState.CurrentState) == "adversary":
		// We just left impl_fix and are back at adversary
		if err := runAndRecordTests(ctx, opts, sf); err != nil {
			opts.Logger.Warn("post-fix test run failed", "error", err)
		}
	}

	return nil
}

// discoverNewTestFiles finds test files in dir that are not already in the existing list.
func discoverNewTestFiles(dir string, existing []string) []string {
	existingSet := make(map[string]bool, len(existing))
	for _, f := range existing {
		existingSet[f] = true
	}

	var newFiles []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, "_test.go") && !existingSet[name] {
			newFiles = append(newFiles, name)
		}
	}
	return newFiles
}

// runAndRecordTests runs all test files and updates state with results.
func runAndRecordTests(ctx context.Context, opts RunPhaseOptions, sf *state.StateFile) error {
	phaseState, err := sf.Read()
	if err != nil {
		return err
	}

	if len(phaseState.TestFiles) == 0 {
		return nil
	}

	runnerName := "go-test"
	if opts.Config != nil && opts.Config.Runner != "" {
		runnerName = opts.Config.Runner
	}

	var runArgs []string
	if opts.WorkingDir != "" {
		runArgs = append(runArgs, opts.WorkingDir)
	}
	result, err := runner.RunAll(ctx, runnerName, phaseState.TestFiles, 0, opts.ArcHome, runArgs...)
	if err != nil {
		return fmt.Errorf("running tests: %w", err)
	}

	fmt.Printf("[%s] Tests: %d/%d passing\n", opts.PhaseName, result.Passed, result.Total)

	// Save test output
	phaseDir := filepath.Join(opts.PlansDir, opts.PlanName, "phases", opts.PhaseName)
	os.WriteFile(filepath.Join(phaseDir, "last_test_output.txt"), []byte(result.RawOutput), 0644)

	// Update state with test results
	return state.UpdateTests(sf, result.Passed, result.Total)
}

// commitPhase creates a git commit for the phase.
// If dir is provided, it overrides ProjectDir for the commit.
func commitPhase(opts RunPhaseOptions, commitType, description string, dir ...string) (string, error) {
	style := "conventional"
	if opts.Config != nil {
		style = opts.Config.Git.CommitStyle
	}

	commitDir := opts.ProjectDir
	if len(dir) > 0 && dir[0] != "" {
		commitDir = dir[0]
	}

	msg := gitops.FormatCommitMessage(style, commitType, opts.PhaseName, description)
	return gitops.Commit(gitops.CommitOptions{
		Message: msg,
		Dir:     commitDir,
		Config:  opts.Config,
	})
}

// handleDispute invokes AI judgment to resolve a test dispute.
func handleDispute(ctx context.Context, opts RunPhaseOptions, sf *state.StateFile, phaseState *arc.PhaseState) error {
	if len(phaseState.Disputes) == 0 {
		// No actual disputes, reset to implementing
		return state.RejectDispute(sf, "no disputes found")
	}

	fmt.Printf("[%s] Dispute: %s — %s\n", opts.PhaseName, phaseState.Disputes[0].TestName, phaseState.Disputes[0].Reason)

	// Ask AI to judge the dispute
	phaseDir := filepath.Join(opts.PlansDir, opts.PlanName, "phases", opts.PhaseName)
	resolution, err := JudgeDispute(ctx, phaseState, phaseDir)
	if err != nil {
		// On AI failure, reject dispute and continue (side with tests)
		opts.Logger.Warn("dispute judgment failed, siding with tests", "error", err)
		return state.RejectDispute(sf, "AI judgment failed, defaulting to test")
	}

	if resolution.Approve {
		fmt.Printf("[%s] Dispute APPROVED: %s\n", opts.PhaseName, resolution.Reason)
		if err := state.ApproveDispute(sf, resolution.Reason); err != nil {
			return err
		}
		// Run fix mode to correct the test
		result := pipeline.RunIteration(ctx, opts.Logger, pipeline.IterateOptions{
			PlanName:  opts.PlanName,
			PhaseName: opts.PhaseName,
			Mode:      "fix",
			PlansDir:  opts.PlansDir,
			ArcHome:   opts.ArcHome,
			ChatMode:  opts.ChatMode,
		})
		if result.Err != nil {
			opts.Logger.Warn("fix iteration failed", "error", result.Err)
		}
		// Clear disputes to return to implementing
		return sf.Update(func(s *arc.PhaseState) error {
			s.LastClearedDisputes = s.Disputes
			s.Disputes = []arc.Dispute{}
			s.PhaseStatus = "implementing"
			return nil
		})
	}

	fmt.Printf("[%s] Dispute REJECTED: %s\n", opts.PhaseName, resolution.Reason)
	return state.RejectDispute(sf, resolution.Reason)
}

// handleEscalation applies the escalation ladder for stuck phases.
func handleEscalation(ctx context.Context, opts RunPhaseOptions, sf *state.StateFile, phaseState *arc.PhaseState) error {
	// In chat mode, skip the escalation ladder entirely — block immediately
	// so the chat agent can intervene.
	if opts.ChatMode {
		reason := fmt.Sprintf("escalation in state %s (stuck_iterations=%d)",
			phaseState.CurrentState, phaseState.StuckIterations)
		sf.Update(func(s *arc.PhaseState) error {
			s.PhaseStatus = "blocked"
			s.Blocked = arc.BlockedInfo{IsBlocked: true, Reason: &reason}
			return nil
		})
		return fmt.Errorf("phase blocked: %s", reason)
	}

	stuck := phaseState.StuckIterations

	switch {
	case stuck >= 6:
		// Max stuck — try rollback
		if phaseState.RollbackCount >= 2 {
			fmt.Printf("[%s] Permanently blocked after %d rollbacks\n", opts.PhaseName, phaseState.RollbackCount)
			return sf.Update(func(s *arc.PhaseState) error {
				s.PhaseStatus = "blocked"
				reason := "max rollbacks exhausted"
				s.Blocked = arc.BlockedInfo{IsBlocked: true, Reason: &reason}
				return nil
			})
		}
		fmt.Printf("[%s] Rollback (attempt %d/2)\n", opts.PhaseName, phaseState.RollbackCount+1)
		return sf.Update(func(s *arc.PhaseState) error {
			s.Iteration.Current = 0
			s.StuckIterations = 0
			s.TestsPassing = 0
			s.TestsTotal = 0
			s.RollbackCount++
			return nil
		})

	case stuck >= 3:
		// Escalation — generate instructions for next iteration
		fmt.Printf("[%s] Stuck for %d iterations, escalating...\n", opts.PhaseName, stuck)
		// The stuck instructions will be generated in the main loop
		return nil

	default:
		return nil
	}
}

// isComingFromQAReview checks if the previous verdict was qa_review → approved.
func isComingFromQAReview(ps *arc.PhaseState) bool {
	if len(ps.VerdictsHistory) == 0 {
		return false
	}
	last := ps.VerdictsHistory[len(ps.VerdictsHistory)-1]
	return last.State == "qa_review" && last.Verdict == "approved"
}

// phaseObjective reads the plan.md to extract a short description.
func phaseObjective(opts RunPhaseOptions) string {
	phaseDir := filepath.Join(opts.PlansDir, opts.PlanName, "phases", opts.PhaseName)
	data, err := os.ReadFile(filepath.Join(phaseDir, "plan.md"))
	if err != nil {
		return "implement phase"
	}
	// Extract first heading or first non-empty line
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = strings.TrimLeft(line, "# ")
		if len(line) > 72 {
			line = line[:72]
		}
		return strings.ToLower(line)
	}
	return "implement phase"
}

// deriveMode maps a workflow state name to a prompt mode string.
func deriveMode(stateName string) string {
	// Strip block namespace prefix for mode derivation
	base := stateName
	if idx := strings.LastIndex(stateName, "."); idx >= 0 {
		base = stateName[idx+1:]
	}

	var mode string
	switch {
	case strings.Contains(base, "review"):
		mode = base
	case base == "qa":
		mode = "qa"
	case base == "adversary":
		mode = "adversary"
	case base == "impl_fix":
		mode = "impl-fix"
	default:
		mode = "impl"
	}
	return strings.ReplaceAll(mode, "_", "-")
}

func stringOrDefault(s *string, def string) string {
	if s == nil {
		return def
	}
	return *s
}
