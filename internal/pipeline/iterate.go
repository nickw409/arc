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

// workflowBytesFunc loads raw workflow YAML for a given workflow type.
// Tests override this to inject custom workflows.
var workflowBytesFunc = resources.WorkflowBytes

// IterateOptions configures a single state run.
type IterateOptions struct {
	PlanName     string
	PhaseName    string
	Mode         string
	Instructions string
	PlansDir     string
	ArcHome      string
	WorkingDir   string // if set, agent runs in this directory (e.g. worktree)
	ChatMode     bool   // if true, skip workflow-defined escalation rules
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

	// Load workflow
	wfBytes, err := workflowBytesFunc(phaseState.WorkflowType)
	if err != nil {
		return &arc.IterationResult{Action: arc.ActionAbort, Err: fmt.Errorf("loading workflow %q: %w", phaseState.WorkflowType, err)}
	}

	wf, err := workflow.LoadBytes(wfBytes)
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

	// run_once: skip on all visits after the first.
	// phase.go pre-increments StateIterations before calling RunState, so
	// a count of > 1 means this state has already executed at least once.
	if stateConfig.RunOnce && phaseState.StateIterations[phaseState.CurrentState] > 1 {
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

		verdict, parallelUsage, err := RunParallel(ctx, logger, RunParallelOptions{
			PhaseDir:   phaseDir,
			StateFile:  sf,
			PhaseState: &phaseState,
			Config:     stateConfig.Parallel,
			PlanMD:     string(planMD),
			ArcHome:    opts.ArcHome,
			PlansDir:   opts.PlansDir,
			PlanName:   opts.PlanName,
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

	tmplCtx := prompt.TemplateContext{
		Phase:          phaseState.Phase,
		Plan:           phaseState.Plan,
		Iteration:      phaseState.Iteration.Current,
		PlanMD:         string(planMD),
		State:          prompt.StateToTemplateMap(&phaseState),
		Params:         map[string]string{},
		PlanFile:       filepath.Join(opts.PlansDir, opts.PlanName, "plan.md"),
		PhaseDir:       phaseDir,
		StateFile:      filepath.Join(phaseDir, "state.json"),
		ScriptsDir:     filepath.Join(opts.ArcHome, "scripts"),
		Mode:           opts.Mode,
		DisputeCount:   len(phaseState.Disputes),
		DisputeList:    prompt.FormatDisputeList(phaseState.Disputes),
		PreviousMemory: previousMemory,
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

	// Spawn agent
	spawnResult, err := agent.Spawn(ctx, spawnOpts)
	if err != nil {
		if ctx.Err() != nil {
			return &arc.IterationResult{Action: arc.ActionAbort, Err: ctx.Err()}
		}
		return &arc.IterationResult{Action: arc.ActionRetry, Err: fmt.Errorf("agent spawn failed: %w", err)}
	}

	// Extract and save memory from agent output
	if mem := ExtractMemory(spawnResult.Output); mem != "" {
		if writeErr := WriteMemory(phaseDir, phaseState.CurrentState, mem); writeErr != nil {
			logger.Warn("failed to save agent memory", "state", phaseState.CurrentState, "error", writeErr)
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
				return &arc.IterationResult{Action: arc.ActionRetry, Usage: spawnResult.Usage, Err: fmt.Errorf("agent exited with code %d (verdict extraction also failed: %v)", spawnResult.ExitCode, err)}
			}
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
