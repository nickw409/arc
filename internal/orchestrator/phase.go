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
)

// RunPhaseOptions configures execution of a single phase.
type RunPhaseOptions struct {
	PlanName   string
	PhaseName  string
	PlansDir   string
	ArcHome    string
	ProjectDir string // working directory for git commits; empty uses process cwd
	Config     *config.Config
	Logger     *slog.Logger
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
		if phaseState.StuckIterations >= 3 && mode == "impl" {
			instructions, err = generateStuckInstructions(ctx, opts, phaseState)
			if err != nil {
				opts.Logger.Warn("failed to generate stuck instructions", "error", err)
			}
		}

		fmt.Printf("[%s] %s iteration %d", opts.PhaseName, mode, phaseState.Iteration.Current+1)
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

	switch {
	// After QA review approved: log and continue to impl
	case currentState == "impl" && result.Verdict == arc.VerdictApproved && isComingFromQAReview(phaseState):
		fmt.Printf("[%s] QA Review: APPROVED\n", opts.PhaseName)

	// In impl state: run tests after each iteration
	case currentState == "impl" || pipeline.MapStateToStatus(currentState) == "implementing":
		if err := runAndRecordTests(ctx, opts, sf); err != nil {
			opts.Logger.Warn("test run failed", "error", err)
		}

	// Impl review approved: commit and mark complete
	case currentState == "complete":
		fmt.Printf("[%s] Impl Review: APPROVED\n", opts.PhaseName)
		desc := phaseObjective(opts)
		hash, err := commitPhase(opts, "feat", desc)
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

	result, err := runner.RunAll(ctx, runnerName, phaseState.TestFiles, 0, opts.ArcHome)
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
func commitPhase(opts RunPhaseOptions, commitType, description string) (string, error) {
	style := "conventional"
	if opts.Config != nil {
		style = opts.Config.Git.CommitStyle
	}

	msg := gitops.FormatCommitMessage(style, commitType, opts.PhaseName, description)
	return gitops.Commit(gitops.CommitOptions{
		Message: msg,
		Dir:     opts.ProjectDir,
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
	var mode string
	if strings.Contains(stateName, "review") {
		mode = stateName
	} else if stateName == "qa" {
		mode = "qa"
	} else {
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
