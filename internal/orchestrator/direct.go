package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nwiley/arc/internal/agent"
	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/prompt"
	"github.com/nwiley/arc/internal/resources"
	"github.com/nwiley/arc/internal/state"
)

// directAgentCmd is the command name for spawning the agent in runDirectPlanLoop.
// Tests override this to point to a mock binary.
var directAgentCmd = "claude"

// runDirectPlanLoop runs all phases of a direct-workflow plan in a single agent session.
// The agent is expected to call `arc manage <plan> <phase> complete` for each phase it
// completes. After the session exits, any phases not marked complete are blocked.
func runDirectPlanLoop(ctx context.Context, opts LaunchOptions, planDir string, meta *arc.PlanMeta, workingDir string) error {
	// Build combined phases content for the prompt
	var phasesSB strings.Builder
	for _, phaseName := range meta.Phases {
		phaseDir := filepath.Join(planDir, "phases", phaseName)
		planMD, err := os.ReadFile(filepath.Join(phaseDir, "plan.md"))
		if err != nil {
			return fmt.Errorf("reading plan.md for phase %q: %w", phaseName, err)
		}
		fmt.Fprintf(&phasesSB, "# Phase: %s\n\n%s\n\n---\n\n", phaseName, string(planMD))
	}

	// Load and render the multi-phase prompt template
	promptBytes, err := resources.PromptBytes("direct/multi-phase.md")
	if err != nil {
		return fmt.Errorf("loading multi-phase prompt: %w", err)
	}
	rendered, err := prompt.RenderString(string(promptBytes), prompt.TemplateContext{
		Plan:   opts.PlanName,
		PlanMD: phasesSB.String(),
	})
	if err != nil {
		return fmt.Errorf("rendering multi-phase prompt: %w", err)
	}

	spawnOpts := agent.SpawnOptions{
		Prompt:      rendered,
		CommandName: directAgentCmd,
		MaxTurns:    400,
		Timeout:     7200 * time.Second,
		WorkingDir:  workingDir,
	}

	// Spawn one agent session for all phases
	spawnResult, spawnErr := agent.Spawn(ctx, spawnOpts)
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// On failure, retry once (unless in chat mode)
	if !opts.ChatMode && (spawnErr != nil || (spawnResult != nil && spawnResult.ExitCode != 0)) {
		opts.Logger.Warn("direct plan agent failed, retrying once", "err", spawnErr)
		spawnResult, spawnErr = agent.Spawn(ctx, spawnOpts)
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}

	// Build a summary of the session output for block reasons
	sessionSummary := ""
	if spawnResult != nil && spawnResult.Output != "" {
		sessionSummary = truncate(spawnResult.Output, 400)
	}

	// Reconcile phase states: block any phases not marked complete by the agent
	var blockedPhases []string
	for _, phaseName := range meta.Phases {
		phaseDir := filepath.Join(planDir, "phases", phaseName)
		sf := state.NewStateFile(filepath.Join(phaseDir, "state.json"))
		ps, readErr := sf.Read()
		if readErr != nil || ps.PhaseStatus != "complete" {
			reason := "direct plan session ended without completing this phase"
			if spawnErr != nil {
				reason = fmt.Sprintf("agent session failed: %v", spawnErr)
			} else if sessionSummary != "" {
				reason = fmt.Sprintf("session ended without completing phase (output: %s)", sessionSummary)
			}
			_ = sf.Update(func(s *arc.PhaseState) error {
				s.PhaseStatus = "blocked"
				s.Blocked = arc.BlockedInfo{IsBlocked: true, Reason: &reason}
				return nil
			})
			blockedPhases = append(blockedPhases, phaseName)
		}
	}

	if len(blockedPhases) > 0 {
		return fmt.Errorf("phases not completed: %s", strings.Join(blockedPhases, ", "))
	}
	return nil
}
