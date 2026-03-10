package orchestrator

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nwiley/arc/internal/adapter"
	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/gate"
	"github.com/nwiley/arc/internal/prompt"
)

// runOrchestratorIntervention spawns the orchestrator agent after all gated
// attempts are exhausted. The agent reads the attempt history and may modify
// plan.md (to change the spec or gate assertions) or fix code directly.
//
// Returns true if the intervention ran and the caller should retry the phase.
// Returns false only if the agent could not be spawned (hard error).
// A no-op intervention (agent exited cleanly but changed nothing) still returns
// true — the subsequent gate run will fail immediately and mark the phase blocked.
func runOrchestratorIntervention(
	ctx context.Context,
	opts RunPhaseOptions,
	spec *arc.PhaseSpec,
	workDir string,
	attemptLog []arc.AttemptSummary,
	lastGate *arc.GateResult,
	lastDiff string,
) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}

	planMDPath := filepath.Join(opts.PlansDir, opts.PlanName, "phases", opts.PhaseName, "plan.md")

	// Build attempt data from the persisted attempt log.
	attempts := make([]prompt.AttemptData, len(attemptLog))
	for i, a := range attemptLog {
		gateOut := ""
		if len(a.Assertions) > 0 {
			gateOut = gate.Format(&arc.GateResult{Assertions: a.Assertions})
		}
		attempts[i] = prompt.AttemptData{
			Attempt:    a.Attempt,
			GateOutput: gateOut,
		}
	}
	// Fallback: if attempt log is empty, use the last gate result directly.
	if len(attempts) == 0 && lastGate != nil {
		attempts = []prompt.AttemptData{{
			Attempt:    1,
			GateOutput: gate.Format(lastGate),
		}}
	}

	data := prompt.OrchestratorData{
		AttemptCount: len(attempts),
		PhaseName:    opts.PhaseName,
		SpecSummary:  spec.Spec,
		Attempts:     attempts,
		DiffSummary:  lastDiff,
		PlanMDPath:   planMDPath,
	}

	agentPrompt, err := prompt.RenderGatePrompt("orchestrator", data)
	if err != nil {
		return false, fmt.Errorf("rendering orchestrator prompt: %w", err)
	}

	adapterName := "claude"
	if opts.Config != nil {
		adapterName = opts.Config.AgentForRole("orchestrator")
	}
	agentAdapter := adapter.Get(adapterName)

	fmt.Printf("[%s] Orchestrator intervention — analyzing %d failed attempt(s)\n",
		opts.PhaseName, len(attempts))
	opts.Logger.Info("orchestrator intervention starting",
		"phase", opts.PhaseName,
		"attempts", len(attempts),
		"adapter", agentAdapter.Name(),
	)

	if opts.PlanLogger != nil {
		opts.PlanLogger.PhaseStarted(opts.PhaseName+" (orchestrator)", 0)
	}

	sessionCfg := arc.SessionConfig{
		MaxTurns: 30,
		Timeout:  3 * time.Minute,
	}

	_, spawnErr := agentAdapter.Spawn(ctx, agentPrompt, workDir, sessionCfg)
	if spawnErr != nil {
		return false, fmt.Errorf("orchestrator agent spawn failed: %w", spawnErr)
	}

	opts.Logger.Info("orchestrator intervention complete", "phase", opts.PhaseName)
	return true, nil
}

// planMDHash returns the SHA-256 hex digest of a plan.md file.
// Used to detect whether the orchestrator agent modified it.
func planMDHash(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h[:]), nil
}
