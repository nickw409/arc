package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nwiley/arc/internal/agent"
	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/prompt"
	"github.com/nwiley/arc/internal/resources"
	"github.com/nwiley/arc/internal/state"
	"github.com/nwiley/arc/internal/workflow"
)

// agentCommandName is the binary name used for agent spawning.
// Tests override this to point to a mock binary.
var agentCommandName = "claude"

// SetAgentCommandNameForTest overrides the agent binary name for testing.
func SetAgentCommandNameForTest(name string) {
	agentCommandName = name
}

// IterateOptions configures a single state run.
type IterateOptions struct {
	PlanName     string
	PhaseName    string
	Mode         string
	Instructions string
	PlansDir     string
	ArcHome      string
	WorkingDir   string              // if set, agent runs in this directory (e.g. worktree)
	ChatMode     bool                // if true, skip workflow-defined escalation rules
	Resolver     *resources.Resolver // if nil, uses NewResolver("", "") (embedded-only)
}

// RunState executes the current phase state, running the agent until it produces a verdict.
// The agent runs with high max_turns and timeout so it can complete the work in one session.
func RunState(ctx context.Context, logger *slog.Logger, opts IterateOptions) *arc.IterationResult {
	phaseDir := filepath.Join(opts.PlansDir, opts.PlanName, "phases", opts.PhaseName)

	// Load state.json using atomic StateFile
	sf := state.NewStateFile(filepath.Join(phaseDir, "state.json"))
	statePtr, err := sf.Read()
	if err != nil {
		return &arc.IterationResult{Action: arc.ActionAbort, Err: fmt.Errorf("reading state.json: %w", err)}
	}
	phaseState := *statePtr

	// Resolve effective resolver
	r := opts.Resolver
	if r == nil {
		r = resources.NewResolver("", "")
	}

	// Load workflow
	planDir := filepath.Join(opts.PlansDir, opts.PlanName)
	wfBytes, err := resources.ResolveWorkflowBytes(r, phaseState.WorkflowType, planDir)
	if err != nil {
		return &arc.IterationResult{Action: arc.ActionAbort, Err: fmt.Errorf("loading workflow %q: %w", phaseState.WorkflowType, err)}
	}

	wf, err := workflow.LoadBytesWithBlockLoader(wfBytes, r.BlockBytes)
	if err != nil {
		return &arc.IterationResult{Action: arc.ActionAbort, Err: fmt.Errorf("parsing workflow: %w", err)}
	}

	machine := workflow.NewMachine(wf)

	// Get state config (may be nil for unknown states)
	stateConfig := machine.GetState(phaseState.CurrentState)

	// Extract workflow-defined constraints and after hooks
	var constraints *arc.ConstraintConfig
	var afterHooks []arc.HookConfig
	if stateConfig != nil {
		constraints = stateConfig.Constraints
		afterHooks = stateConfig.After
	}

	// Check pre-constraints
	if err := CheckPreConstraints(&phaseState, constraints, phaseDir); err != nil {
		return &arc.IterationResult{Action: arc.ActionAbort, Err: fmt.Errorf("pre-constraint check: %w", err)}
	}

	// Check context after state load
	if ctx.Err() != nil {
		return &arc.IterationResult{Action: arc.ActionAbort, Err: ctx.Err()}
	}

	// Check if current state is terminal
	if machine.IsTerminal(phaseState.CurrentState) {
		return &arc.IterationResult{Action: arc.ActionContinue}
	}

	// Check context cancellation before spawning
	if ctx.Err() != nil {
		return &arc.IterationResult{Action: arc.ActionAbort, Err: ctx.Err()}
	}

	if stateConfig == nil {
		return &arc.IterationResult{Action: arc.ActionAbort, Err: fmt.Errorf("state %q not found in workflow", phaseState.CurrentState)}
	}

	// run_once: skip on all visits after the first successful completion.
	// phase.go pre-increments StateIterations before calling RunState, so
	// a count of > 1 means this state has been entered before. We also require
	// a prior verdict entry for this state to confirm the previous visit
	// actually completed — if the run was interrupted mid-execution, there will
	// be no verdict entry and we should re-run rather than skip.
	hasPriorVerdict := func() bool {
		for _, e := range phaseState.VerdictsHistory {
			if e.State == phaseState.CurrentState {
				return true
			}
		}
		return false
	}
	if stateConfig.RunOnce && phaseState.StateIterations[phaseState.CurrentState] > 1 && hasPriorVerdict() {
		skipVerdict := arc.Verdict(stateConfig.SkipVerdict)
		nextState, err := machine.NextState(phaseState.CurrentState, skipVerdict)
		if err != nil {
			return &arc.IterationResult{Action: arc.ActionAbort, Err: fmt.Errorf("run_once skip: %w", err)}
		}
		curState := phaseState.CurrentState
		if err := sf.Update(func(s *arc.PhaseState) error {
			s.CurrentState = nextState
			s.PhaseStatus = MapStateToStatus(nextState)
			s.Iteration.Current++
			s.LastVerdict = string(skipVerdict)
			s.GlobalIterations++
			s.Activity = ""
			s.ActivityUpdatedAt = ""
			s.VerdictsHistory = append(s.VerdictsHistory, arc.VerdictEntry{
				Iteration: s.Iteration.Current,
				State:     curState,
				Verdict:   string(skipVerdict),
				Timestamp: timeNow(),
			})
			return nil
		}); err != nil {
			return &arc.IterationResult{Action: arc.ActionAbort, Err: fmt.Errorf("updating state after run_once skip: %w", err)}
		}
		return &arc.IterationResult{
			Action:    arc.ActionContinue,
			NextState: nextState,
			Verdict:   skipVerdict,
		}
	}

	// Parallel execution: if the state has parallel config, run branches concurrently
	if stateConfig.Parallel != nil {
		planMD, err := os.ReadFile(filepath.Join(phaseDir, "plan.md"))
		if err != nil {
			return &arc.IterationResult{Action: arc.ActionAbort, Err: fmt.Errorf("reading plan.md: %w", err)}
		}

		// Convert state verdicts ([]string) to []arc.Verdict for verdict-aware joining.
		var validVerdicts []arc.Verdict
		for _, v := range stateConfig.Verdicts {
			validVerdicts = append(validVerdicts, arc.Verdict(v))
		}

		verdict, parallelUsage, err := RunParallel(ctx, logger, RunParallelOptions{
			PhaseDir:        phaseDir,
			StateFile:       sf,
			PhaseState:      &phaseState,
			Config:          stateConfig.Parallel,
			ValidVerdicts:   validVerdicts,
			PositiveVerdict: arc.Verdict(stateConfig.SkipVerdict),
			PlanMD:          string(planMD),
			ArcHome:         opts.ArcHome,
			PlansDir:        opts.PlansDir,
			PlanName:        opts.PlanName,
		})
		if err != nil {
			return &arc.IterationResult{Action: arc.ActionRetry, Err: fmt.Errorf("parallel execution: %w", err)}
		}

		// Use verdict for state transition
		nextState, err := machine.NextState(phaseState.CurrentState, arc.Verdict(verdict))
		if err != nil {
			return &arc.IterationResult{Action: arc.ActionRetry, Err: err}
		}

		curState := phaseState.CurrentState
		if updateErr := sf.Update(func(s *arc.PhaseState) error {
			s.CurrentState = nextState
			s.PhaseStatus = MapStateToStatus(nextState)
			s.Iteration.Current++
			s.LastVerdict = verdict
			s.GlobalIterations++
			s.Activity = ""
			s.ActivityUpdatedAt = ""
			s.VerdictsHistory = append(s.VerdictsHistory, arc.VerdictEntry{
				Iteration: s.Iteration.Current,
				State:     curState,
				Verdict:   verdict,
				Timestamp: timeNow(),
			})
			return nil
		}); updateErr != nil {
			return &arc.IterationResult{Action: arc.ActionAbort, Err: fmt.Errorf("updating state.json: %w", updateErr)}
		}

		return &arc.IterationResult{
			NextState: nextState,
			Verdict:   arc.Verdict(verdict),
			Action:    arc.ActionContinue,
			Usage:     parallelUsage,
		}
	}

	// Load plan.md
	planMD, err := os.ReadFile(filepath.Join(phaseDir, "plan.md"))
	if err != nil {
		return &arc.IterationResult{Action: arc.ActionAbort, Err: fmt.Errorf("reading plan.md: %w", err)}
	}

	// Read loopback memory from previous run of this state
	previousMemory, _ := ReadMemory(phaseDir, phaseState.CurrentState)

	// Load and render prompt
	promptPath := strings.TrimPrefix(stateConfig.Prompt, "prompts/")
	promptBytes, err := resources.PromptBytes(promptPath)
	if err != nil {
		return &arc.IterationResult{Action: arc.ActionAbort, Err: fmt.Errorf("loading prompt %q: %w", promptPath, err)}
	}

	params := stateConfig.Params
	if params == nil {
		params = map[string]string{}
	}
	// Load scout report if present (written by scout states for adversary consumption)
	var scoutReport string
	if data, err := os.ReadFile(filepath.Join(phaseDir, "scout-report.md")); err == nil {
		scoutReport = string(data)
	}

	tmplCtx := prompt.TemplateContext{
		Phase:          phaseState.Phase,
		Plan:           phaseState.Plan,
		Iteration:      phaseState.Iteration.Current,
		PlanMD:         string(planMD),
		State:          prompt.StateToTemplateMap(&phaseState),
		Params:         params,
		PlanFile:       filepath.Join(opts.PlansDir, opts.PlanName, "plan.md"),
		PhaseDir:       phaseDir,
		StateFile:      filepath.Join(phaseDir, "state.json"),
		ScriptsDir:     filepath.Join(opts.ArcHome, "scripts"),
		Mode:           opts.Mode,
		DisputeCount:   len(phaseState.Disputes),
		DisputeList:    prompt.FormatDisputeList(phaseState.Disputes),
		PreviousMemory: previousMemory,
		ScoutReport:    scoutReport,
	}

	rendered, err := prompt.RenderString(string(promptBytes), tmplCtx)
	if err != nil {
		return &arc.IterationResult{Action: arc.ActionAbort, Err: fmt.Errorf("rendering prompt: %w", err)}
	}

	// Check context after prompt render
	if ctx.Err() != nil {
		return &arc.IterationResult{Action: arc.ActionAbort, Err: ctx.Err()}
	}

	// Append mode and instructions if present
	if opts.Mode != "" {
		rendered += fmt.Sprintf("\n\nMode: %s", opts.Mode)
	}
	if opts.Instructions != "" {
		rendered += fmt.Sprintf("\n\nInstructions: %s", opts.Instructions)
	}

	// Build spawn options, applying per-state agent config if present
	spawnOpts := buildSpawnOptions(stateConfig, &phaseState, rendered, opts.WorkingDir)

	// Log iteration start
	_ = state.AppendHistory(phaseDir, fmt.Sprintf("%s [%s] iter %d started", timeNow(), phaseState.CurrentState, phaseState.Iteration.Current))

	// Spawn agent
	spawnResult, err := agent.Spawn(ctx, spawnOpts)
	if err != nil {
		if ctx.Err() != nil {
			return &arc.IterationResult{Action: arc.ActionAbort, Err: ctx.Err()}
		}
		_ = state.AppendHistory(phaseDir, fmt.Sprintf("%s [%s] error: agent spawn failed: %v", timeNow(), phaseState.CurrentState, err))
		return &arc.IterationResult{Action: arc.ActionRetry, Err: fmt.Errorf("agent spawn failed: %w", err)}
	}

	// Log agent lifecycle info to history
	_ = state.AppendHistory(phaseDir, fmt.Sprintf("%s [%s] agent pid=%d exit=%d duration=%s",
		timeNow(), phaseState.CurrentState, spawnResult.PID, spawnResult.ExitCode, spawnResult.Duration.Round(time.Second)))

	// Log truncated stderr on non-zero exit
	if spawnResult.ExitCode != 0 && spawnResult.Stderr != "" {
		snippet := spawnResult.Stderr
		if len(snippet) > 500 {
			snippet = snippet[:500] + "..."
		}
		_ = state.AppendHistory(phaseDir, fmt.Sprintf("%s [%s] stderr: %s", timeNow(), phaseState.CurrentState, snippet))
	}

	// Log per-turn summaries from stream-json output
	for _, ts := range spawnResult.TurnSummaries {
		tools := strings.Join(ts.Tools, ", ")
		if tools == "" {
			tools = "(none)"
		}
		_ = state.AppendHistory(phaseDir, fmt.Sprintf("%s [%s] turn %d: in=%d out=%d tools=[%s]",
			timeNow(), phaseState.CurrentState, ts.Turn, ts.InputTokens, ts.OutputTokens, tools))
	}

	// Extract and save memory from agent output
	if mem := ExtractMemory(spawnResult.Output); mem != "" {
		if writeErr := WriteMemory(phaseDir, phaseState.CurrentState, mem); writeErr != nil {
			logger.Warn("failed to save agent memory", "state", phaseState.CurrentState, "error", writeErr)
		}
	}

	// Save scout output for downstream adversary states
	if strings.HasSuffix(phaseState.CurrentState, ".scout") || phaseState.CurrentState == "scout" {
		scoutPath := filepath.Join(phaseDir, "scout-report.md")
		if writeErr := os.WriteFile(scoutPath, []byte(spawnResult.Output), 0644); writeErr != nil {
			logger.Warn("failed to save scout report", "error", writeErr)
		}
	}

	// Determine next state
	validVerdicts := machine.ValidVerdicts(phaseState.CurrentState)

	var verdict arc.Verdict
	var nextState string

	if len(validVerdicts) == 0 {
		// Linear state: non-zero exit is a hard failure (no verdict to salvage)
		if spawnResult.ExitCode != 0 {
			return &arc.IterationResult{Action: arc.ActionRetry, Usage: spawnResult.Usage, Err: fmt.Errorf("agent exited with code %d", spawnResult.ExitCode)}
		}
		nextState, err = machine.NextState(phaseState.CurrentState, "")
		if err != nil {
			return &arc.IterationResult{Action: arc.ActionAbort, Err: fmt.Errorf("state transition failed: %w", err)}
		}
	} else {
		// Branching state: try to extract verdict even on non-zero exit
		// (agent may have written verdict before hitting max-turns)
		verdict, err = prompt.ExtractVerdict(spawnResult.Output, validVerdicts)
		if err != nil {
			if spawnResult.ExitCode != 0 {
				_ = state.AppendHistory(phaseDir, fmt.Sprintf("%s [%s] error: exit code %d, verdict extraction failed: %v", timeNow(), phaseState.CurrentState, spawnResult.ExitCode, err))
				return &arc.IterationResult{Action: arc.ActionRetry, Usage: spawnResult.Usage, Err: fmt.Errorf("agent exited with code %d (verdict extraction also failed: %v)", spawnResult.ExitCode, err)}
			}
			_ = state.AppendHistory(phaseDir, fmt.Sprintf("%s [%s] error: verdict extraction failed: %v", timeNow(), phaseState.CurrentState, err))
			return &arc.IterationResult{Action: arc.ActionRetry, Err: err}
		}

		nextState, err = machine.NextState(phaseState.CurrentState, verdict)
		if err != nil {
			return &arc.IterationResult{Action: arc.ActionRetry, Err: err}
		}
	}

	// Check post-constraints after successful run
	if err := CheckPostConstraints(constraints, phaseDir); err != nil {
		return &arc.IterationResult{Action: arc.ActionAbort, Err: fmt.Errorf("post-constraint check: %w", err)}
	}

	// Run after hooks
	if err := RunAfterHooks(ctx, afterHooks, verdict, &phaseState, phaseDir); err != nil {
		return &arc.IterationResult{Action: arc.ActionAbort, Err: fmt.Errorf("after hooks: %w", err)}
	}

	// Update state atomically
	curState := phaseState.CurrentState
	if err := sf.Update(func(s *arc.PhaseState) error {
		s.CurrentState = nextState
		s.PhaseStatus = MapStateToStatus(nextState)
		s.Iteration.Current++
		s.LastVerdict = string(verdict)
		s.GlobalIterations++
		s.Activity = ""
		s.ActivityUpdatedAt = ""
		if verdict != "" {
			s.VerdictsHistory = append(s.VerdictsHistory, arc.VerdictEntry{
				Iteration: s.Iteration.Current,
				State:     curState,
				Verdict:   string(verdict),
				Timestamp: timeNow(),
			})
		}
		return nil
	}); err != nil {
		return &arc.IterationResult{Action: arc.ActionAbort, Err: fmt.Errorf("updating state.json: %w", err)}
	}

	logger.Info("state run complete",
		"phase", phaseState.Phase,
		"next_state", nextState,
		"verdict", string(verdict),
	)

	if verdict != "" {
		_ = state.AppendHistory(phaseDir, fmt.Sprintf("%s [%s] → [%s] verdict: %s", timeNow(), curState, nextState, verdict))
	} else {
		_ = state.AppendHistory(phaseDir, fmt.Sprintf("%s [%s] → [%s]", timeNow(), curState, nextState))
	}

	return &arc.IterationResult{
		NextState: nextState,
		Verdict:   verdict,
		Action:    arc.ActionContinue,
		Usage:     spawnResult.Usage,
	}
}

// timeNow returns the current UTC time as RFC3339. Extracted for testability.
var timeNow = func() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// buildSpawnOptions constructs agent.SpawnOptions from state config and defaults.
// Default MaxTurns=200 and Timeout=3600s allow long-running sessions.
// Workflow YAML agent config overrides these defaults.
func buildSpawnOptions(stateConfig *arc.StateConfig, phaseState *arc.PhaseState, prompt string, workingDir string) agent.SpawnOptions {
	opts := agent.SpawnOptions{
		Prompt:      prompt,
		CommandName: agentCommandName,
		Model:       phaseState.ModelOverride,
		WorkingDir:  workingDir,
		MaxTurns:    200,
		Timeout:     3600 * time.Second,
	}

	if stateConfig != nil && stateConfig.Agent != nil {
		ac := stateConfig.Agent
		if ac.MaxTurns > 0 {
			opts.MaxTurns = ac.MaxTurns
		}
		if len(ac.AllowedTools) > 0 {
			opts.AllowedTools = ac.AllowedTools
		}
		if ac.Timeout > 0 {
			opts.Timeout = time.Duration(ac.Timeout) * time.Second
		}
		if ac.Model != "" && phaseState.ModelOverride == "" {
			opts.Model = ac.Model
		}
	}

	return opts
}

// MapStateToStatus maps a workflow state name to a phase_status value by
// stripping the block namespace prefix (e.g., "check.adversary" → "adversary").
func MapStateToStatus(stateName string) string {
	if idx := strings.LastIndex(stateName, "."); idx >= 0 {
		return stateName[idx+1:]
	}
	return stateName
}
