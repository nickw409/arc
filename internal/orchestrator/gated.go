package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
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

	// Require spec content before spawning. An empty spec wastes an agent
	// session — the gate rejects it immediately as misconfigured. Simple phases
	// don't need checkpoints or assertions, but the spec field is mandatory.
	// Safety net: if spec.yaml is empty, attempt to sync from the ## Spec block
	// in plan.md (e.g. when arc review was skipped or plan.md was edited after review).
	if strings.TrimSpace(spec.Spec) == "" && strings.TrimSpace(spec.Verify) == "" {
		if synced, syncErr := plan.SyncSpecFromPlanMD(opts.PlansDir, opts.PlanName, opts.PhaseName); syncErr == nil && synced {
			opts.Logger.Info("auto-synced spec.yaml from plan.md", "phase", opts.PhaseName)
			spec, err = plan.ReadSpec(opts.PlansDir, opts.PlanName, opts.PhaseName)
			if err != nil {
				return fmt.Errorf("reading phase spec after sync: %w", err)
			}
		}
	}
	if strings.TrimSpace(spec.Spec) == "" && strings.TrimSpace(spec.Verify) == "" {
		return fmt.Errorf("phase %q has no spec content — fill in spec.yaml before running (checkpoints optional for simple fixes, but spec field is required)", opts.PhaseName)
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
	// Turn log file for audit trail
	turnsPath := filepath.Join(phaseDir, "turns.jsonl")
	turnsFile, _ := os.OpenFile(turnsPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if turnsFile != nil {
		defer turnsFile.Close()
	}

	// Stuck detection
	role := arc.DefaultRole(spec.Role)
	stuckDetector := NewStuckDetector(role, 0)
	var stuckCancel context.CancelFunc // set per-attempt to cancel stuck sessions
	var lastStuckSignal *StuckSignal   // set when stuck detection fires

	sessionCfg := arc.SessionConfig{
		MaxTurns: turnBudget,
		Timeout:  time.Duration(turnBudget) * 30 * time.Second,
		OnTurn: func(ev arc.TurnEvent) {
			_ = state.SetActivity(sf, formatTurnActivity(ev))
			if turnsFile != nil {
				if line, err := json.Marshal(ev); err == nil {
					line = append(line, '\n')
					_, _ = turnsFile.Write(line)
				}
			}
			if sig := stuckDetector.Record(ev); sig != nil {
				lastStuckSignal = sig
				opts.Logger.Warn("stuck agent detected",
					"phase", opts.PhaseName,
					"pattern", sig.Pattern,
					"reason", sig.Reason,
				)
				_ = state.SetActivity(sf, "stuck: "+sig.Reason)
				if stuckCancel != nil {
					stuckCancel()
				}
			}
		},
	}

	// Gate spec path
	specPath := filepath.Join(phaseDir, "spec.yaml")

	var lastGateResult *arc.GateResult
	var lastDiff string
	prevCheckpointsPassed := 0
	var attemptHistory []AttemptRecord
	var stuckGuidance string // injected into prompt after stuck detection
	escalationModel := ""    // set when model is escalated
	stuckEscalationLevel := 0 // tracks how far up the ladder we've gone

	// Structured logger (nil-safe — callers log methods check for nil)
	pl := opts.PlanLogger

	for attempt := 1; attempt <= MaxGatedAttempts; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Reset stuck detector for each attempt
		stuckDetector.Reset()
		lastStuckSignal = nil

		if pl != nil {
			pl.PhaseStarted(opts.PhaseName, attempt)
		}

		// Build prompt
		var agentPrompt string
		if attempt == 1 && stuckGuidance == "" {
			agentPrompt, err = buildPhasePrompt(spec, opts.PlanName, projectCtx)
		} else {
			agentPrompt, err = buildRetryPrompt(spec, opts.PlanName, projectCtx, attempt, lastGateResult, lastDiff)
		}
		if err != nil {
			return fmt.Errorf("building prompt (attempt %d): %w", attempt, err)
		}

		// Append stuck guidance if we detected stuck on previous attempt
		if stuckGuidance != "" {
			agentPrompt = agentPrompt + "\n\n" + stuckGuidance
			stuckGuidance = "" // consumed
		}

		// Apply model escalation if set
		attemptCfg := sessionCfg
		if escalationModel != "" {
			attemptCfg.Model = escalationModel
			fmt.Printf("[%s] Model escalated to %s\n", opts.PhaseName, escalationModel)
		}

		// Update state — clear stale activity from previous attempt
		if updateErr := sf.Update(func(s *arc.PhaseState) error {
			s.PhaseStatus = "implementing"
			s.Iteration.Current = attempt
			s.Activity = ""
			s.ActivityUpdatedAt = ""
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

		// Create per-attempt context for stuck cancellation
		var attemptCtx context.Context
		attemptCtx, stuckCancel = context.WithCancel(ctx)

		// Spawn agent
		if pl != nil {
			pl.AgentSpawned(opts.PhaseName, attempt,
				fmt.Sprintf("adapter=%s turns=%d", agentAdapter.Name(), attemptCfg.MaxTurns))
		}
		result, spawnErr := agentAdapter.Spawn(attemptCtx, agentPrompt, workDir, attemptCfg)
		stuckCancel() // always clean up

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

		// Handle stuck detection — if the detector cancelled the context, apply
		// the escalation ladder instead of normal retry logic.
		if lastStuckSignal != nil && attempt < MaxGatedAttempts {
			sig := lastStuckSignal
			fmt.Printf("[%s] Agent stuck: %s\n", opts.PhaseName, sig.Reason)

			if updateErr := sf.Update(func(s *arc.PhaseState) error {
				s.Notes = FormatStuckNote(sig, attempt)
				return nil
			}); updateErr != nil {
				opts.Logger.Warn("failed to persist stuck note", "error", updateErr)
			}

			// Escalation ladder:
			// Level 0 → inject guidance
			// Level 1 → escalate model + guidance
			// Level 2 → narrow scope to failing checkpoints
			stuckEscalationLevel++
			switch stuckEscalationLevel {
			case 1:
				stuckGuidance = StuckGuidance(sig)
				fmt.Printf("[%s] Escalation: injecting stuck guidance\n", opts.PhaseName)
			case 2:
				escalationModel = resolveEscalationModel()
				stuckGuidance = StuckGuidance(sig)
				fmt.Printf("[%s] Escalation: switching to %s\n", opts.PhaseName, escalationModel)
			default:
				stuckGuidance = narrowScopeGuidance(spec, lastGateResult)
				fmt.Printf("[%s] Escalation: narrowing scope\n", opts.PhaseName)
			}
			continue
		}

		// Handle spawn-level failures
		if spawnErr != nil {
			tier := classifySpawnError(result, spawnErr)
			if tier == TierTransient && attempt < MaxGatedAttempts {
				opts.Logger.Warn("transient spawn error, retrying",
					"attempt", attempt, "error", spawnErr)
				delay := transientBackoff(attempt, result != nil && result.RateLimit)
				select {
				case <-time.After(delay):
				case <-ctx.Done():
					return ctx.Err()
				}
				continue
			}
			// Agent failed hard — still run gate in case it made partial progress
			opts.Logger.Warn("agent spawn failed, running gate anyway",
				"attempt", attempt, "error", spawnErr)
		}

		// Back off if the agent was rate-limited but exited without a spawn error.
		if spawnErr == nil && result != nil && result.RateLimit && attempt < MaxGatedAttempts {
			opts.Logger.Warn("agent rate-limited, backing off", "attempt", attempt)
			delay := transientBackoff(attempt, true)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
			continue
		}

		// Run gate — role-based routing
		var gateResult *arc.GateResult
		if role != "impl" {
			// Non-impl roles: run verifier as the primary gate, skip assertions.
			fmt.Printf("[%s] Running verifier check\n", opts.PhaseName)
			passed, reasoning, verifyErr := gate.RunVerifier(ctx, spec, workDir)
			if verifyErr != nil {
				opts.Logger.Warn("verifier execution error", "error", verifyErr)
				if attempt < MaxGatedAttempts {
					continue
				}
				return fmt.Errorf("verifier failed on final attempt: %w", verifyErr)
			}
			gateResult = &arc.GateResult{
				Passed:            passed,
				ScopedTestPassed:  passed,
				ScopedTestSkipped: true,
			}
			if !passed {
				gateResult.ScopedTestOutput = reasoning
			}
		} else {
			// Impl role: run assertion-based gate as before.
			fmt.Printf("[%s] Running gate check\n", opts.PhaseName)
			var gateOpts []gate.RunOption
			if opts.Config != nil {
				gateOpts = append(gateOpts, gate.WithVerifier(
					gate.ShouldRunVerifier(nil, opts.Config.Verifier, spec.Gate.VerifierAgent, spec.Complexity)))
			}
			var gateErr error
			gateResult, gateErr = gate.Run(ctx, specPath, workDir, gateOpts...)
			if gateErr != nil {
				opts.Logger.Warn("gate execution error", "error", gateErr)
				if attempt < MaxGatedAttempts {
					continue
				}
				return fmt.Errorf("gate execution failed on final attempt: %w", gateErr)
			}
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
		// Mark the phase as blocked so the scheduler doesn't re-launch it.
		reason := fmt.Sprintf("commit failed: %v", commitErr)
		if updateErr := sf.Update(func(s *arc.PhaseState) error {
			s.PhaseStatus = "blocked"
			s.Blocked = arc.BlockedInfo{IsBlocked: true, Reason: &reason}
			return nil
		}); updateErr != nil {
			opts.Logger.Warn("failed to persist blocked state after commit failure", "error", updateErr)
		}
		return fmt.Errorf("commit failed for phase %s (work may be in worktree %s): %w", opts.PhaseName, workDir, commitErr)
	}
	if hash != "" {
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

// buildPhasePrompt dispatches to the correct prompt builder based on the phase role.
func buildPhasePrompt(spec *arc.PhaseSpec, planName, projectCtx string) (string, error) {
	role := arc.DefaultRole(spec.Role)
	switch role {
	case "review", "audit":
		return buildReviewPrompt(spec, planName, projectCtx)
	case "investigate":
		return buildInvestigatePrompt(spec, planName, projectCtx)
	default:
		return buildImplPrompt(spec, planName, projectCtx)
	}
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
		TestCommand:    "",
		ProjectContext: projectCtx,
	}

	return prompt.RenderGatePrompt("impl", data)
}

// buildReviewPrompt renders the review prompt for review/audit roles.
func buildReviewPrompt(spec *arc.PhaseSpec, planName, projectCtx string) (string, error) {
	data := prompt.ReviewData{
		Spec:           spec.Spec,
		Files:          spec.Files,
		Plan:           planName,
		Phase:          spec.Name,
		OutputFile:     fmt.Sprintf("findings-%s.md", spec.Name),
		ProjectContext: projectCtx,
	}
	return prompt.RenderGatePrompt("review", data)
}

// buildInvestigatePrompt renders the investigate prompt.
func buildInvestigatePrompt(spec *arc.PhaseSpec, planName, projectCtx string) (string, error) {
	data := prompt.InvestigateData{
		Spec:           spec.Spec,
		Files:          spec.Files,
		Plan:           planName,
		Phase:          spec.Name,
		OutputFile:     fmt.Sprintf("findings-%s.md", spec.Name),
		ProjectContext: projectCtx,
	}
	return prompt.RenderGatePrompt("investigate", data)
}

// buildRetryPrompt renders the phase prompt plus retry context.
// For non-impl roles (review, investigate, audit), the retry uses verifier feedback
// via the review-retry template instead of the standard gate-output retry.
func buildRetryPrompt(spec *arc.PhaseSpec, planName, projectCtx string, attempt int, lastGate *arc.GateResult, diff string) (string, error) {
	// Start with the full phase prompt
	phasePrompt, err := buildPhasePrompt(spec, planName, projectCtx)
	if err != nil {
		return "", err
	}

	role := arc.DefaultRole(spec.Role)

	// Non-impl roles use review-retry template with verifier feedback
	if role == "review" || role == "investigate" || role == "audit" {
		verifierFeedback := ""
		if lastGate != nil {
			// For verifier-only gate results, use the raw verifier reasoning.
			if lastGate.ScopedTestOutput != "" {
				verifierFeedback = lastGate.ScopedTestOutput
			} else {
				verifierFeedback = gate.Format(lastGate)
			}
		}

		retryData := prompt.ReviewRetryData{
			Attempt:          attempt,
			MaxAttempts:      MaxGatedAttempts,
			VerifierFeedback: verifierFeedback,
			OutputFile:       fmt.Sprintf("findings-%s.md", spec.Name),
		}

		retryAddendum, retryErr := prompt.RenderGatePrompt("review-retry", retryData)
		if retryErr != nil {
			return "", retryErr
		}
		return phasePrompt + "\n\n" + retryAddendum, nil
	}

	// Impl role uses standard retry template
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

	return phasePrompt + "\n\n" + retryAddendum, nil
}

// formatTurnActivity converts a TurnEvent into a human-readable activity string.
// Uses file paths and commands when available for richer status output.
func formatTurnActivity(ev arc.TurnEvent) string {
	var parts []string
	seen := make(map[string]bool)

	for _, tu := range ev.Tools {
		var label string
		switch tu.Name {
		case "Edit", "MultiEdit":
			if tu.File != "" {
				label = "editing " + filepath.Base(tu.File)
			} else {
				label = "editing files"
			}
		case "Read", "View":
			if tu.File != "" {
				label = "reading " + filepath.Base(tu.File)
			} else {
				label = "reading files"
			}
		case "Write":
			if tu.File != "" {
				label = "writing " + filepath.Base(tu.File)
			} else {
				label = "writing files"
			}
		case "Bash":
			if tu.Cmd != "" {
				cmd := tu.Cmd
				if len(cmd) > 40 {
					cmd = cmd[:37] + "..."
				}
				label = "running: " + cmd
			} else {
				label = "running commands"
			}
		case "Grep":
			label = "searching code"
		case "Glob":
			label = "searching files"
		case "WebFetch", "WebSearch":
			label = "searching web"
		default:
			label = friendlyToolName(tu.Name)
		}

		if !seen[label] {
			seen[label] = true
			parts = append(parts, label)
		}
	}

	return strings.Join(parts, ", ")
}

// formatToolActivity converts a list of tool names into a human-readable activity string.
// Kept for backward compatibility with non-streaming adapters.
func formatToolActivity(tools []string) string {
	seen := make(map[string]bool, len(tools))
	var parts []string
	for _, t := range tools {
		label := friendlyToolName(t)
		if !seen[label] {
			seen[label] = true
			parts = append(parts, label)
		}
	}
	return strings.Join(parts, ", ")
}

func friendlyToolName(t string) string {
	switch t {
	case "Edit", "MultiEdit":
		return "editing files"
	case "Read", "View":
		return "reading files"
	case "Write":
		return "writing files"
	case "Bash":
		return "running commands"
	case "WebFetch", "WebSearch":
		return "searching web"
	case "Glob":
		return "searching files"
	case "Grep":
		return "searching code"
	case "mcp__arc__arc_manage":
		return "updating state"
	default:
		if strings.HasPrefix(t, "mcp__arc__") {
			return strings.TrimPrefix(t, "mcp__arc__")
		}
		return t
	}
}

// resolveEscalationModel returns the model to escalate to when stuck.
func resolveEscalationModel() string {
	return "claude-opus-4-6"
}

// narrowScopeGuidance produces a prompt addendum that focuses the agent on
// just the failing checkpoints/assertions.
func narrowScopeGuidance(spec *arc.PhaseSpec, lastGate *arc.GateResult) string {
	var sb strings.Builder
	sb.WriteString("## Narrowed Scope\n\n")
	sb.WriteString("Your previous attempts made partial progress. Focus ONLY on the remaining failures below.\n")
	sb.WriteString("Do NOT re-do work that already passes.\n\n")

	if lastGate != nil {
		var passing, failing []string
		for _, cp := range lastGate.Checkpoints {
			if cp.Status == "pass" {
				passing = append(passing, cp.Name)
			} else {
				failing = append(failing, cp.Name+": "+cp.Output)
			}
		}
		for _, a := range lastGate.Assertions {
			if a.Passed {
				passing = append(passing, a.Description)
			} else {
				failing = append(failing, a.Description+": "+a.Detail)
			}
		}

		if len(passing) > 0 {
			sb.WriteString("### Already passing (do not break these)\n")
			for _, p := range passing {
				sb.WriteString(fmt.Sprintf("- %s\n", p))
			}
			sb.WriteString("\n")
		}
		if len(failing) > 0 {
			sb.WriteString("### Still failing (fix these)\n")
			for _, f := range failing {
				sb.WriteString(fmt.Sprintf("- %s\n", f))
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// transientBackoff returns the delay to wait before a transient retry.
// Rate-limited retries use exponential backoff (5s * 2^(attempt-1), capped at 60s).
// Other transient failures use a fixed 2s delay.
func transientBackoff(attempt int, rateLimit bool) time.Duration {
	if !rateLimit {
		return 2 * time.Second
	}
	delay := 5 * time.Second * (1 << uint(attempt-1))
	if delay > 60*time.Second {
		delay = 60 * time.Second
	}
	return delay
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
