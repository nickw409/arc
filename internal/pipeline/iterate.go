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

// IterateOptions configures a single iteration.
type IterateOptions struct {
	PlanName     string
	PhaseName    string
	Mode         string
	Instructions string
	PlansDir     string
	ArcHome      string
}

// RunIteration executes a single iteration of the phase state machine.
func RunIteration(ctx context.Context, logger *slog.Logger, opts IterateOptions) *arc.IterationResult {
	phaseDir := filepath.Join(opts.PlansDir, opts.PlanName, "phases", opts.PhaseName)

	// Load state.json using atomic StateFile
	sf := state.NewStateFile(filepath.Join(phaseDir, "state.json"))
	statePtr, err := sf.Read()
	if err != nil {
		return &arc.IterationResult{Action: arc.ActionAbort, Err: fmt.Errorf("reading state.json: %w", err)}
	}
	phaseState := *statePtr

	// Check pre-constraints
	if err := checkPreConstraints(&phaseState, nil, phaseDir); err != nil {
		return &arc.IterationResult{Action: arc.ActionAbort, Err: fmt.Errorf("pre-constraint check: %w", err)}
	}

	// Check intervention
	if action := checkIntervention(&phaseState, nil); action != "" {
		return &arc.IterationResult{Action: arc.ActionAbort, Err: fmt.Errorf("intervention required: %s", action)}
	}

	// Check escalation
	if esc := checkEscalation(&phaseState, nil); esc != nil {
		return &arc.IterationResult{Action: arc.ActionAbort, Err: fmt.Errorf("escalation triggered: %s", esc.Action)}
	}

	// Check context after state load
	if ctx.Err() != nil {
		return &arc.IterationResult{Action: arc.ActionAbort, Err: ctx.Err()}
	}

	// Load workflow
	wfBytes, err := resources.WorkflowBytes(phaseState.WorkflowType)
	if err != nil {
		return &arc.IterationResult{Action: arc.ActionAbort, Err: fmt.Errorf("loading workflow %q: %w", phaseState.WorkflowType, err)}
	}

	wf, err := workflow.LoadBytes(wfBytes)
	if err != nil {
		return &arc.IterationResult{Action: arc.ActionAbort, Err: fmt.Errorf("parsing workflow: %w", err)}
	}

	machine := workflow.NewMachine(wf)

	// Check context after workflow load
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

	// Get state config for the current state
	stateConfig := machine.GetState(phaseState.CurrentState)
	if stateConfig == nil {
		return &arc.IterationResult{Action: arc.ActionAbort, Err: fmt.Errorf("state %q not found in workflow", phaseState.CurrentState)}
	}

	// Load plan.md
	planMD, err := os.ReadFile(filepath.Join(phaseDir, "plan.md"))
	if err != nil {
		return &arc.IterationResult{Action: arc.ActionAbort, Err: fmt.Errorf("reading plan.md: %w", err)}
	}

	// Load and render prompt
	promptPath := strings.TrimPrefix(stateConfig.Prompt, "prompts/")
	promptBytes, err := resources.PromptBytes(promptPath)
	if err != nil {
		return &arc.IterationResult{Action: arc.ActionAbort, Err: fmt.Errorf("loading prompt %q: %w", promptPath, err)}
	}

	tmplCtx := prompt.TemplateContext{
		Phase:     phaseState.Phase,
		Plan:      phaseState.Plan,
		Iteration: phaseState.Iteration.Current,
		PlanMD:    string(planMD),
		State:     prompt.StateToTemplateMap(&phaseState),
		Params:    map[string]string{},
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

	// Spawn agent
	spawnResult, err := agent.Spawn(ctx, agent.SpawnOptions{
		Prompt:      rendered,
		CommandName: agentCommandName,
		Model:       phaseState.ModelOverride,
	})
	if err != nil {
		if ctx.Err() != nil {
			return &arc.IterationResult{Action: arc.ActionAbort, Err: ctx.Err()}
		}
		return &arc.IterationResult{Action: arc.ActionRetry, Err: fmt.Errorf("agent spawn failed: %w", err)}
	}

	// Handle timeout
	if spawnResult.TimedOut {
		return &arc.IterationResult{Action: arc.ActionRetry, Err: fmt.Errorf("agent timed out")}
	}

	// Handle empty output
	output := strings.TrimSpace(spawnResult.Output)
	if output == "" {
		return &arc.IterationResult{Action: arc.ActionRetry, Err: fmt.Errorf("agent produced no output (empty)")}
	}

	// Handle non-zero exit
	if spawnResult.ExitCode != 0 {
		return &arc.IterationResult{Action: arc.ActionRetry, Err: fmt.Errorf("agent exited with code %d", spawnResult.ExitCode)}
	}

	// Determine next state
	validVerdicts := machine.ValidVerdicts(phaseState.CurrentState)

	var verdict arc.Verdict
	var nextState string

	if len(validVerdicts) == 0 {
		// Linear state: advance without verdict
		nextState, err = machine.NextState(phaseState.CurrentState, "")
		if err != nil {
			return &arc.IterationResult{Action: arc.ActionAbort, Err: fmt.Errorf("state transition failed: %w", err)}
		}
	} else {
		// Branching state: extract verdict
		verdict, err = prompt.ExtractVerdict(spawnResult.Output, validVerdicts)
		if err != nil {
			return &arc.IterationResult{Action: arc.ActionRetry, Err: err}
		}

		nextState, err = machine.NextState(phaseState.CurrentState, verdict)
		if err != nil {
			return &arc.IterationResult{Action: arc.ActionRetry, Err: err}
		}
	}

	// Check post-constraints after successful iteration
	if err := checkPostConstraints(nil, phaseDir); err != nil {
		return &arc.IterationResult{Action: arc.ActionAbort, Err: fmt.Errorf("post-constraint check: %w", err)}
	}

	// Run after hooks
	if err := runAfterHooks(ctx, nil, verdict, &phaseState, phaseDir); err != nil {
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
				Timestamp: time.Now().UTC().Format(time.RFC3339),
			})
		}
		return nil
	}); err != nil {
		return &arc.IterationResult{Action: arc.ActionAbort, Err: fmt.Errorf("updating state.json: %w", err)}
	}

	logger.Info("iteration complete",
		"phase", phaseState.Phase,
		"next_state", nextState,
		"verdict", string(verdict),
	)

	return &arc.IterationResult{
		NextState: nextState,
		Verdict:   verdict,
		Action:    arc.ActionContinue,
	}
}

type escalationAction struct {
	Action string
	Params map[string]string
}

func checkIntervention(state *arc.PhaseState, triggers []arc.InterventionTrigger) string {
	return ""
}

func checkEscalation(state *arc.PhaseState, rules []arc.EscalationRule) *escalationAction {
	return nil
}

func checkPreConstraints(state *arc.PhaseState, constraints *arc.ConstraintConfig, phaseDir string) error {
	return nil
}

func checkPostConstraints(constraints *arc.ConstraintConfig, phaseDir string) error {
	return nil
}

func runAfterHooks(ctx context.Context, hooks []arc.HookConfig, verdict arc.Verdict, state *arc.PhaseState, phaseDir string) error {
	return nil
}

// MapStateToStatus maps a workflow state name to a phase_status value.
func MapStateToStatus(stateName string) string {
	switch stateName {
	case "qa":
		return "qa"
	case "qa_review", "test_review", "char_review", "fix_review", "review", "verify", "benchmark", "impl_review":
		return "qa_review"
	case "complete":
		return "complete"
	case "blocked":
		return "blocked"
	default:
		return "implementing"
	}
}
