package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nwiley/arc/internal/agent"
	"github.com/nwiley/arc/internal/state"
)

// MaxWatchAttempts is the maximum number of autonomous interventions per phase.
const MaxWatchAttempts = 3

// InterventionLog records a single autonomous intervention attempt.
// Appended as a JSON line to <phase-dir>/watch.jsonl.
type InterventionLog struct {
	Plan       string `json:"plan"`
	Phase      string `json:"phase"`
	Attempt    int    `json:"attempt"`
	StartedAt  string `json:"started_at"`
	Duration   string `json:"duration"`
	ExitCode   int    `json:"exit_code"`
	OutputTail string `json:"output_tail,omitempty"`
	Error      string `json:"error,omitempty"`
}

// runWatchInterventions launches one goroutine per blocked phase.
// Must be called without the scheduler mutex held.
func (s *Scheduler) runWatchInterventions(ctx context.Context, reg *PlanRegistration, blockedPhases []string) {
	for _, phase := range blockedPhases {
		phase := phase // capture loop variable
		go s.runOneIntervention(ctx, reg, phase)
	}
}

// runOneIntervention runs a single autonomous intervention for a blocked phase.
// It acquires a slot from s.slots before spawning, respecting maxParallel.
func (s *Scheduler) runOneIntervention(ctx context.Context, reg *PlanRegistration, phaseName string) {
	// Acquire semaphore slot — blocks until one is available.
	s.slots <- struct{}{}
	defer func() { <-s.slots }()

	pd := planDir(reg)
	phaseDir := filepath.Join(pd, "phases", phaseName)
	stateFilePath := filepath.Join(phaseDir, "state.json")
	sf := state.NewStateFile(stateFilePath)

	// Increment WatchAttempts before spawning — crash counts as an attempt.
	if err := state.IncrementWatchAttempts(sf); err != nil {
		s.logger.Error("watch: increment attempts failed",
			"plan", reg.PlanName, "phase", phaseName, "error", err)
		s.done <- PhaseResult{PlanName: reg.PlanName, PhaseName: phaseName, WatchIntervention: true}
		return
	}

	// Read incremented state to get attempt number for logging and prompt.
	ps, _ := sf.Read()
	attempt := 1
	if ps != nil {
		attempt = ps.WatchAttempts
	}

	// Build context bundle from on-disk files.
	planMD, _ := os.ReadFile(filepath.Join(phaseDir, "plan.md"))
	stateJSON, _ := os.ReadFile(stateFilePath)

	prompt := buildInterventionPrompt(reg.PlanName, phaseName, attempt, planMD, stateJSON)

	s.logger.Info("watch intervention starting",
		"plan", reg.PlanName, "phase", phaseName,
		"attempt", attempt, "max", MaxWatchAttempts)

	startedAt := time.Now()
	result, spawnErr := agent.Spawn(ctx, agent.SpawnOptions{
		Prompt:     prompt,
		WorkingDir: reg.ProjectDir,
		MaxTurns:   30,
		Timeout:    10 * time.Minute,
	})
	duration := time.Since(startedAt)

	// Build log entry.
	entry := InterventionLog{
		Plan:      reg.PlanName,
		Phase:     phaseName,
		Attempt:   attempt,
		StartedAt: startedAt.UTC().Format(time.RFC3339),
		Duration:  duration.Round(time.Second).String(),
	}
	if spawnErr != nil {
		entry.Error = spawnErr.Error()
		s.logger.Error("watch intervention spawn error",
			"plan", reg.PlanName, "phase", phaseName,
			"attempt", attempt, "error", spawnErr)
	}
	if result != nil {
		entry.ExitCode = result.ExitCode
		entry.OutputTail = tailLines(result.Output, 50)
		s.logger.Info("watch intervention done",
			"plan", reg.PlanName, "phase", phaseName,
			"attempt", attempt, "exit_code", result.ExitCode,
			"duration", duration, "output_bytes", len(result.Output))
	}

	appendInterventionLog(filepath.Join(phaseDir, "watch.jsonl"), entry)

	// Reset phase to pending so the orchestrator can retry.
	if err := state.ResetToRetry(sf); err != nil {
		s.logger.Error("watch: reset to retry failed",
			"plan", reg.PlanName, "phase", phaseName, "error", err)
	}

	s.done <- PhaseResult{PlanName: reg.PlanName, PhaseName: phaseName, WatchIntervention: true}
}

// buildInterventionPrompt constructs the agent prompt for a watch intervention.
func buildInterventionPrompt(planName, phaseName string, attempt int, planMD, stateJSON []byte) string {
	return fmt.Sprintf(`You are fixing a blocked Arc phase. Read the context below, make the fix, then exit.

Plan: %s | Phase: %s | Watch attempt: %d of %d

## plan.md
%s

## state.json
%s

Fix whatever caused the gate assertions in plan.md to fail.
- Edit source files or plan.md as needed
- Run tests with Bash to verify your fix works
- Do NOT make no-op fixes (e.g. returning hardcoded values to pass assertions)
- Do NOT call any arc CLI commands — the orchestrator handles retry automatically

After making your changes, stop. The gate will be re-evaluated automatically.`,
		planName, phaseName, attempt, MaxWatchAttempts,
		string(planMD), string(stateJSON))
}

// tailLines returns the last n lines of s.
func tailLines(s string, n int) string {
	if n <= 0 {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

// appendInterventionLog appends entry as a JSON line to path.
// Errors are silently ignored — logging failure must not block orchestration.
func appendInterventionLog(path string, entry InterventionLog) {
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	// Single write to minimize interleaving on concurrent appends.
	_, _ = f.Write(append(data, '\n'))
}
