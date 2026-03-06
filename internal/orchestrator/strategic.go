package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nwiley/arc/internal/adapter"
	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/prompt"
)

// StrategicDecision represents the orchestrator agent's recommendation
// for how to handle a stuck phase.
type StrategicDecision struct {
	Action      string // "modify_spec", "adjust_gate", "split_phase", "give_up"
	Reasoning   string // agent's explanation of the decision
	NewSpec     string // updated spec text (for modify_spec)
	GateChanges string // gate adjustment description (for adjust_gate)
	RawOutput   string // full agent output for logging
}

// RunStrategicIntervention spawns a lightweight orchestrator agent to diagnose
// why a phase is stuck and recommend a course of action. This is Tier 3 of
// the retry strategy — called when mechanical retries with gate feedback fail
// to make progress.
//
// The agent receives the phase spec, attempt history (gate outputs), and the
// current code diff. It decides one of: modify the spec, adjust the gate,
// split the phase, or give up.
func RunStrategicIntervention(ctx context.Context, opts RunPhaseOptions, spec *arc.PhaseSpec, history []AttemptRecord) (*StrategicDecision, error) {
	// Resolve adapter for the orchestrator role
	adapterName := "claude"
	if opts.Config != nil {
		adapterName = opts.Config.AgentForRole("orchestrator")
	}
	agentAdapter := adapter.Get(adapterName)

	// Build the prompt
	strategicPrompt, err := buildStrategicPrompt(spec, history)
	if err != nil {
		return nil, fmt.Errorf("building strategic prompt: %w", err)
	}

	// Use minimal turns and timeout — this is a diagnostic, not an implementation
	sessionCfg := arc.SessionConfig{
		MaxTurns: 10,
		Timeout:  5 * time.Minute,
	}

	workDir := opts.WorkingDir
	if workDir == "" {
		workDir = opts.ProjectDir
	}

	result, err := agentAdapter.Spawn(ctx, strategicPrompt, workDir, sessionCfg)
	if err != nil {
		return nil, fmt.Errorf("strategic agent failed: %w", err)
	}

	output := ""
	if result != nil {
		output = result.Output
	}

	return parseStrategicDecision(output), nil
}

// AttemptRecord captures what happened during one retry attempt, for the
// strategic agent's context.
type AttemptRecord struct {
	Attempt          int
	GateOutput       string
	CheckpointsPassed int
	CheckpointsTotal  int
	DiffSummary      string
}

// buildStrategicPrompt renders the orchestrator agent prompt with attempt history.
func buildStrategicPrompt(spec *arc.PhaseSpec, history []AttemptRecord) (string, error) {
	// Try the orchestrator template first
	attempts := make([]prompt.AttemptData, len(history))
	for i, h := range history {
		attempts[i] = prompt.AttemptData{
			Attempt:           h.Attempt,
			GateOutput:        h.GateOutput,
			CheckpointsPassed: h.CheckpointsPassed,
			CheckpointsTotal:  h.CheckpointsTotal,
			DiffSummary:       h.DiffSummary,
		}
	}
	data := prompt.OrchestratorData{
		AttemptCount: len(history),
		PhaseName:    spec.Name,
		SpecSummary:  spec.Spec,
		Attempts:     attempts,
	}
	if len(history) > 0 {
		data.DiffSummary = history[len(history)-1].DiffSummary
	}

	rendered, err := prompt.RenderGatePrompt("orchestrator", data)
	if err == nil && rendered != "" {
		return rendered, nil
	}

	// Fallback: build inline prompt if template not found
	var sb strings.Builder
	sb.WriteString("A phase has failed multiple attempts. The orchestrator has exhausted mechanical retries and needs a strategic decision.\n\n")
	sb.WriteString(fmt.Sprintf("## Phase\n%s: %s\n\n", spec.Name, spec.Spec))
	sb.WriteString("## Attempt History\n")
	for _, h := range history {
		sb.WriteString(fmt.Sprintf("### Attempt %d\n", h.Attempt))
		sb.WriteString(fmt.Sprintf("Checkpoints passed: %d / %d\n", h.CheckpointsPassed, h.CheckpointsTotal))
		if h.GateOutput != "" {
			sb.WriteString(fmt.Sprintf("Gate output:\n```\n%s\n```\n", h.GateOutput))
		}
		if h.DiffSummary != "" {
			sb.WriteString(fmt.Sprintf("Diff summary:\n```\n%s\n```\n", h.DiffSummary))
		}
		sb.WriteString("\n")
	}
	sb.WriteString(`## Decision Required
Choose ONE action and explain your reasoning:

1. MODIFY_SPEC — Simplify the spec or change the approach. Provide the new spec text.
2. ADJUST_GATE — Relax gate criteria that are too strict. Specify which assertions to change.
3. SPLIT_PHASE — Break into smaller phases. Describe the split.
4. GIVE_UP — The task is not achievable in its current form. Explain why.

Start your response with the action keyword (MODIFY_SPEC, ADJUST_GATE, SPLIT_PHASE, or GIVE_UP) on the first line.
`)
	return sb.String(), nil
}

// parseStrategicDecision extracts the decision from the agent's output.
func parseStrategicDecision(output string) *StrategicDecision {
	if output == "" {
		return &StrategicDecision{
			Action:    "give_up",
			Reasoning: "strategic agent produced no output",
			RawOutput: output,
		}
	}

	upper := strings.ToUpper(strings.TrimSpace(output))

	decision := &StrategicDecision{
		RawOutput: output,
	}

	switch {
	case strings.HasPrefix(upper, "MODIFY_SPEC"):
		decision.Action = "modify_spec"
		decision.Reasoning = extractReasoning(output)
		decision.NewSpec = extractSection(output, "new spec", "updated spec")
	case strings.HasPrefix(upper, "ADJUST_GATE"):
		decision.Action = "adjust_gate"
		decision.Reasoning = extractReasoning(output)
		decision.GateChanges = extractSection(output, "gate", "assertions")
	case strings.HasPrefix(upper, "SPLIT_PHASE"):
		decision.Action = "split_phase"
		decision.Reasoning = extractReasoning(output)
	case strings.HasPrefix(upper, "GIVE_UP"):
		decision.Action = "give_up"
		decision.Reasoning = extractReasoning(output)
	default:
		// Agent didn't follow the format — try to infer
		decision.Action = inferAction(upper)
		decision.Reasoning = output
	}

	return decision
}

// extractReasoning gets everything after the first line as the reasoning.
func extractReasoning(output string) string {
	lines := strings.SplitN(output, "\n", 2)
	if len(lines) < 2 {
		return output
	}
	return strings.TrimSpace(lines[1])
}

// extractSection looks for a section header and returns the content after it.
func extractSection(output string, keywords ...string) string {
	lower := strings.ToLower(output)
	for _, kw := range keywords {
		idx := strings.Index(lower, kw)
		if idx >= 0 {
			rest := output[idx+len(kw):]
			// Skip past any header formatting
			rest = strings.TrimLeft(rest, ":# \t\n")
			// Take until the next section header or end
			if end := strings.Index(rest, "\n##"); end > 0 {
				return strings.TrimSpace(rest[:end])
			}
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// inferAction guesses the action from the output when the agent didn't follow format.
func inferAction(upper string) string {
	switch {
	case strings.Contains(upper, "MODIFY") || strings.Contains(upper, "SIMPLIF"):
		return "modify_spec"
	case strings.Contains(upper, "GATE") || strings.Contains(upper, "RELAX"):
		return "adjust_gate"
	case strings.Contains(upper, "SPLIT"):
		return "split_phase"
	default:
		return "give_up"
	}
}

// applyStrategicDecision modifies the phase spec based on the strategic decision.
// Returns true if the spec was modified and the phase should be retried.
func applyStrategicDecision(decision *StrategicDecision, spec *arc.PhaseSpec, gateResult *arc.GateResult) bool {
	switch decision.Action {
	case "modify_spec":
		if decision.NewSpec != "" {
			spec.Spec = decision.NewSpec
			return true
		}
		// Agent recommended modifying but didn't provide new text — use reasoning as hint
		if decision.Reasoning != "" {
			spec.Spec = spec.Spec + "\n\n## Approach Guidance\n" + decision.Reasoning
			return true
		}
		return false

	case "adjust_gate":
		// Relax gate: remove failing assertions
		if gateResult != nil && len(gateResult.Assertions) > 0 {
			// Build set of failing assertion descriptions
			failing := make(map[string]bool)
			for _, ga := range gateResult.Assertions {
				if !ga.Passed {
					failing[ga.Description] = true
				}
			}
			if len(failing) == 0 {
				return false
			}
			// Remove spec assertions whose description matches a failing gate result
			var kept []arc.GateAssertion
			for _, a := range spec.Gate.Assertions {
				desc := a.Type + ": " + a.Target
				if !failing[desc] {
					kept = append(kept, a)
				}
			}
			if len(kept) < len(spec.Gate.Assertions) {
				spec.Gate.Assertions = kept
				return true
			}
		}
		return false

	case "split_phase":
		// Phase splitting requires plan mutation — flag it but don't handle here.
		// The caller should create new phases.
		return false

	case "give_up":
		return false

	default:
		return false
	}
}

