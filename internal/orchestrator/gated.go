package orchestrator

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/nwiley/arc/internal/adapter"
	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/gate"
	"github.com/nwiley/arc/internal/plan"
	"github.com/nwiley/arc/internal/prompt"
	"github.com/nwiley/arc/internal/state"
)

const (
	// MaxGatedAttempts is the maximum number of agent sessions per phase (1 initial + retries).
	MaxGatedAttempts = 4
)

// RunPhaseGated executes a phase using the gate-based verification model.
//
// Flow per attempt:
//  1. Build prompt (impl on first attempt, retry with gate feedback on subsequent)
//  2. Spawn agent via adapter
//  3. Run hard gate after agent exits
//  4. If gate passes → commit and mark complete
//  5. If gate fails → classify error tier, retry or give up
//
// The working directory (worktree) is preserved across retries — agents build
// on previous attempts rather than starting from scratch.
func RunPhaseGated(ctx context.Context, opts RunPhaseOptions) error {
	phaseDir := filepath.Join(opts.PlansDir, opts.PlanName, "phases", opts.PhaseName)

	// State file for tracking progress
	sf := state.NewStateFile(filepath.Join(phaseDir, "state.json"))

	// Read phase spec
	spec, err := plan.ReadSpec(opts.PlansDir, opts.PlanName, opts.PhaseName)
	if err != nil {
		return fmt.Errorf("reading phase spec: %w", err)
	}

	// Resolve adapter
	adapterName := "claude"
	if opts.Config != nil {
		adapterName = opts.Config.AgentForRole("impl")
	}
	agentAdapter := adapter.Get(adapterName)

	// Determine working directory
	workDir := opts.WorkingDir
	if workDir == "" {
		workDir = opts.ProjectDir
		if workDir == "" {
			workDir, _ = os.Getwd()
		}
	}

	// Pre-flight check
	if err := agentAdapter.Preflight(ctx, workDir); err != nil {
		return fmt.Errorf("adapter preflight failed: %w", err)
	}

	// Load project context
	projectCtx := prompt.LoadProjectContext(workDir)

	// Build session config from complexity
	turnBudget := arc.DefaultTurnBudget(spec.Complexity)
	sessionCfg := arc.SessionConfig{
		MaxTurns: turnBudget,
		Timeout:  time.Duration(turnBudget) * 30 * time.Second,
	}

	// Gate spec path
	specPath := filepath.Join(phaseDir, "spec.yaml")

	var lastGateResult *arc.GateResult
	var lastDiff string
	prevCheckpointsPassed := 0
	var attemptHistory []AttemptRecord

	// Structured logger (nil-safe — callers log methods check for nil)
	pl := opts.PlanLogger

	for attempt := 1; attempt <= MaxGatedAttempts; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if pl != nil {
			pl.PhaseStarted(opts.PhaseName, attempt)
		}

		// Build prompt
		var agentPrompt string
		if attempt == 1 {
			agentPrompt, err = buildImplPrompt(spec, opts.PlanName, projectCtx)
		} else {
			agentPrompt, err = buildRetryPrompt(spec, opts.PlanName, projectCtx, attempt, lastGateResult, lastDiff)
		}
		if err != nil {
			return fmt.Errorf("building prompt (attempt %d): %w", attempt, err)
		}

		// Update state
		if updateErr := sf.Update(func(s *arc.PhaseState) error {
			s.PhaseStatus = "implementing"
			s.Iteration.Current = attempt
			return nil
		}); updateErr != nil {
			opts.Logger.Warn("failed to update state", "error", updateErr)
		}

		opts.Logger.Info("spawning agent",
			"phase", opts.PhaseName,
			"attempt", attempt,
			"max_attempts", MaxGatedAttempts,
			"adapter", agentAdapter.Name(),
		)
		fmt.Printf("[%s] Attempt %d/%d — spawning %s agent\n",
			opts.PhaseName, attempt, MaxGatedAttempts, agentAdapter.Name())

		// Spawn agent
		if pl != nil {
			pl.AgentSpawned(opts.PhaseName, attempt,
				fmt.Sprintf("adapter=%s turns=%d", agentAdapter.Name(), sessionCfg.MaxTurns))
		}
		result, spawnErr := agentAdapter.Spawn(ctx, agentPrompt, workDir, sessionCfg)

		// Persist the agent PID from the result so crash recovery can kill stale
		// processes. Written immediately after spawn returns (process has exited),
		// so on next startup, the signal-0 probe will find it dead and clear it.
		if result != nil && result.PID != 0 {
			if pidErr := sf.Update(func(s *arc.PhaseState) error {
				s.AgentPID = result.PID
				return nil
			}); pidErr != nil {
				opts.Logger.Warn("failed to persist agent PID", "error", pidErr)
			}
		}

		if pl != nil {
			detail := fmt.Sprintf("exit=%d", 0)
			if result != nil {
				detail = fmt.Sprintf("exit=%d duration=%s", result.ExitCode, result.Duration)
			}
			pl.AgentExited(opts.PhaseName, attempt, detail)
		}

		// Clear agent PID now that the subprocess has exited.
		if clearErr := sf.Update(func(s *arc.PhaseState) error {
			s.AgentPID = 0
			return nil
		}); clearErr != nil {
			opts.Logger.Warn("failed to clear agent PID", "error", clearErr)
		}

		// Accumulate usage
		if result != nil && !result.Usage.IsZero() {
			if updateErr := sf.Update(func(s *arc.PhaseState) error {
				s.Usage = s.Usage.Add(result.Usage)
				return nil
			}); updateErr != nil {
				opts.Logger.Warn("failed to persist usage", "error", updateErr)
			}
		}

		// Check budget after accumulating usage
		if opts.Config != nil && opts.Config.Budget.MaxCost > 0 {
			currentState, _ := sf.Read()
			if currentState != nil && currentState.Usage.CostUSD > 0 {
				if currentState.Usage.CostUSD >= opts.Config.Budget.MaxCost {
					return fmt.Errorf("budget exceeded: $%.2f spent, limit $%.2f", currentState.Usage.CostUSD, opts.Config.Budget.MaxCost)
				}
				if opts.Config.Budget.WarnCost > 0 && currentState.Usage.CostUSD >= opts.Config.Budget.WarnCost {
					opts.Logger.Warn("approaching budget limit",
						"spent", currentState.Usage.CostUSD,
						"warn_threshold", opts.Config.Budget.WarnCost,
						"max", opts.Config.Budget.MaxCost)
				}
			}
		}

		// Handle spawn-level failures
		if spawnErr != nil {
			tier := classifySpawnError(result, spawnErr)
			if tier == TierTransient && attempt < MaxGatedAttempts {
				opts.Logger.Warn("transient spawn error, retrying",
					"attempt", attempt, "error", spawnErr)
				continue
			}
			// Agent failed hard — still run gate in case it made partial progress
			opts.Logger.Warn("agent spawn failed, running gate anyway",
				"attempt", attempt, "error", spawnErr)
		}

		// Run hard gate
		fmt.Printf("[%s] Running gate check\n", opts.PhaseName)
		gateResult, gateErr := gate.Run(ctx, specPath, workDir)
		if gateErr != nil {
			opts.Logger.Warn("gate execution error", "error", gateErr)
			if attempt < MaxGatedAttempts {
				continue
			}
			return fmt.Errorf("gate execution failed on final attempt: %w", gateErr)
		}

		// Write gate status and increment run count
		if writeErr := gate.WriteStatus(phaseDir, gateResult); writeErr != nil {
			opts.Logger.Warn("failed to write gate status", "error", writeErr)
		}
		runCount, incErr := gate.IncrementRunCount(phaseDir)
		if incErr != nil {
			opts.Logger.Warn("failed to increment gate run count", "error", incErr)
		}
		if pl != nil {
			pl.GateRun(opts.PhaseName, attempt, gateResult.Passed, gate.FormatWithRunCount(gateResult, runCount))
		}

		if gateResult.Passed {
			if pl != nil {
				pl.PhaseCompleted(opts.PhaseName, attempt, "gate passed")
			}
			return gatedPhaseComplete(opts, sf, spec, workDir)
		}

		// Gate failed — log and prepare for retry
		lastGateResult = gateResult
		formatted := gate.FormatWithRunCount(gateResult, runCount)
		fmt.Printf("[%s] Gate FAILED (attempt %d/%d):\n%s\n",
			opts.PhaseName, attempt, MaxGatedAttempts, formatted)

		// Capture diff for retry context
		lastDiff = captureDiff(workDir)

		// Record attempt for strategic agent context
		attemptHistory = append(attemptHistory, AttemptRecord{
			Attempt:           attempt,
			GateOutput:        formatted,
			CheckpointsPassed: countCheckpointsPassed(gateResult),
			CheckpointsTotal:  len(gateResult.Assertions) + len(gateResult.Checkpoints),
			DiffSummary:       lastDiff,
		})

		// Classify failure
		tier := classifyGateFailure(gateResult, attempt, MaxGatedAttempts, prevCheckpointsPassed)
		prevCheckpointsPassed = countCheckpointsPassed(gateResult)

		opts.Logger.Info("gate failed",
			"attempt", attempt,
			"tier", tier,
			"checkpoints_passed", prevCheckpointsPassed,
		)

		switch tier {
		case TierGiveUp:
			// Fall through to mark failed below
		case TierStrategic:
			// Tier 3: spawn orchestrator agent for strategic diagnosis
			fmt.Printf("[%s] No progress after %d attempts — running strategic intervention\n",
				opts.PhaseName, attempt)
			decision, stratErr := RunStrategicIntervention(ctx, opts, spec, attemptHistory)
			if stratErr != nil {
				opts.Logger.Warn("strategic intervention failed", "error", stratErr)
				continue // fall back to feedback retry
			}
			opts.Logger.Info("strategic decision",
				"action", decision.Action,
				"phase", opts.PhaseName,
			)
			fmt.Printf("[%s] Strategic decision: %s\n", opts.PhaseName, decision.Action)

			if applyStrategicDecision(decision, spec, gateResult) {
				// Spec or gate was modified — retry with updated context
				opts.Logger.Info("applied strategic changes, retrying",
					"phase", opts.PhaseName,
					"action", decision.Action,
				)
				continue
			}
			// Strategic agent said give_up or split_phase (not handled inline)
			if decision.Action == "give_up" {
				break
			}
			continue
		default:
			continue
		}
		break
	}

	// Exhausted all attempts — mark phase as blocked
	if pl != nil {
		pl.PhaseFailed(opts.PhaseName, MaxGatedAttempts, "exhausted all attempts")
	}
	reason := fmt.Sprintf("gate did not pass after %d attempts", MaxGatedAttempts)
	if lastGateResult != nil {
		reason = fmt.Sprintf("gate failed after %d attempts:\n%s", MaxGatedAttempts, gate.Format(lastGateResult))
	}

	if updateErr := sf.Update(func(s *arc.PhaseState) error {
		s.PhaseStatus = "blocked"
		s.Blocked = arc.BlockedInfo{IsBlocked: true, Reason: &reason}
		return nil
	}); updateErr != nil {
		return fmt.Errorf("persisting blocked state: %w", updateErr)
	}

	return fmt.Errorf("phase %s blocked: gate did not pass after %d attempts", opts.PhaseName, MaxGatedAttempts)
}

// gatedPhaseComplete handles the success path: commit changes and mark phase complete.
func gatedPhaseComplete(opts RunPhaseOptions, sf *state.StateFile, spec *arc.PhaseSpec, workDir string) error {
	fmt.Printf("[%s] Gate PASSED\n", opts.PhaseName)

	// Commit changes
	desc := spec.Spec
	if desc == "" {
		desc = spec.Description
	}
	if len(desc) > 72 {
		desc = desc[:72]
	}
	desc = strings.ToLower(strings.TrimSpace(desc))
	if desc == "" {
		desc = "implement phase"
	}

	hash, commitErr := commitPhase(opts, "feat", desc, workDir)
	if commitErr != nil {
		opts.Logger.Warn("commit failed", "error", commitErr)
	} else if hash != "" {
		fmt.Printf("[%s] Committed: %s\n", opts.PhaseName, shortHash(hash))
		if updateErr := sf.Update(func(s *arc.PhaseState) error {
			s.LastCommit = hash
			return nil
		}); updateErr != nil {
			opts.Logger.Warn("failed to persist commit hash", "error", updateErr)
		}
	}

	// Mark complete
	if updateErr := sf.Update(func(s *arc.PhaseState) error {
		s.PhaseStatus = "complete"
		return nil
	}); updateErr != nil {
		return fmt.Errorf("persisting complete state: %w", updateErr)
	}

	return nil
}

// buildImplPrompt renders the implementation prompt from the phase spec.
// planName is the plan name used in the arc gate command.
func buildImplPrompt(spec *arc.PhaseSpec, planName, projectCtx string) (string, error) {
	checkpoints := make([]prompt.CheckpointData, len(spec.Checkpoints))
	for i, cp := range spec.Checkpoints {
		checkpoints[i] = prompt.CheckpointData{
			Name:        cp.Name,
			Description: cp.Description,
			Test:        cp.Test,
		}
	}

	data := prompt.ImplData{
		Spec:           spec.Spec,
		Files:          spec.Files,
		Checkpoints:    checkpoints,
		Plan:           planName, // used in arc gate command
		Phase:          spec.Name,
		TestCommand:    spec.Test,
		ProjectContext: projectCtx,
	}

	return prompt.RenderGatePrompt("impl", data)
}

// buildRetryPrompt renders the implementation prompt plus retry context.
func buildRetryPrompt(spec *arc.PhaseSpec, planName, projectCtx string, attempt int, lastGate *arc.GateResult, diff string) (string, error) {
	// Start with the full impl prompt
	implPrompt, err := buildImplPrompt(spec, planName, projectCtx)
	if err != nil {
		return "", err
	}

	// Render retry addendum
	gateOutput := ""
	if lastGate != nil {
		gateOutput = gate.Format(lastGate)
	}

	retryData := prompt.RetryData{
		Attempt:     attempt,
		MaxAttempts: MaxGatedAttempts,
		GateOutput:  gateOutput,
		DiffSummary: diff,
	}

	retryAddendum, err := prompt.RenderGatePrompt("retry", retryData)
	if err != nil {
		return "", err
	}

	return implPrompt + "\n\n" + retryAddendum, nil
}

// captureDiff runs `git diff` in the given directory and returns the output.
// Returns empty string on error.
func captureDiff(dir string) string {
	cmd := exec.Command("git", "diff", "--stat")
	cmd.Dir = dir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &bytes.Buffer{}
	if err := cmd.Run(); err != nil {
		return ""
	}
	diff := buf.String()
	// Limit diff size for prompt context
	if len(diff) > 4096 {
		diff = diff[:4096] + "\n... (truncated)"
	}
	return diff
}
