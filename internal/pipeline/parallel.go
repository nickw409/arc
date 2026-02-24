package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/nwiley/arc/internal/agent"
	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/prompt"
	"github.com/nwiley/arc/internal/state"
)

// RunParallelOptions configures a parallel execution run.
type RunParallelOptions struct {
	PhaseDir   string
	StateFile  *state.StateFile
	PhaseState *arc.PhaseState
	Config     *arc.ParallelConfig
	PlanMD     string
	ArcHome    string
	PlansDir   string
	PlanName   string
}

// RunParallel spawns agents for each branch in parallel, collects results,
// and returns a verdict and accumulated usage based on the configured strategy.
func RunParallel(ctx context.Context, logger *slog.Logger, opts RunParallelOptions) (string, arc.Usage, error) {
	cfg := opts.Config
	if cfg == nil || len(cfg.Branches) == 0 {
		return "", arc.Usage{}, fmt.Errorf("no parallel branches configured")
	}

	stateName := opts.PhaseState.CurrentState
	resultsDir := filepath.Join(opts.PhaseDir, fmt.Sprintf("parallel_%s", stateName))
	if err := os.MkdirAll(resultsDir, 0755); err != nil {
		return "", arc.Usage{}, fmt.Errorf("creating results dir: %w", err)
	}

	branchNames := make([]string, len(cfg.Branches))
	for i, b := range cfg.Branches {
		branchNames[i] = b.Name
	}

	if err := state.StartParallel(opts.StateFile, resultsDir, branchNames); err != nil {
		return "", arc.Usage{}, fmt.Errorf("starting parallel tracking: %w", err)
	}

	type branchResult struct {
		name     string
		exitCode int
		usage    arc.Usage
		err      error
	}

	resultsCh := make(chan branchResult, len(cfg.Branches))
	var wg sync.WaitGroup

	for _, branch := range cfg.Branches {
		wg.Add(1)
		go func(b arc.ParallelBranch) {
			defer wg.Done()

			if ctx.Err() != nil {
				resultsCh <- branchResult{name: b.Name, exitCode: -1, err: ctx.Err()}
				return
			}

			tmplCtx := prompt.TemplateContext{
				Phase:        opts.PhaseState.Phase,
				Plan:         opts.PhaseState.Plan,
				Iteration:    opts.PhaseState.Iteration.Current,
				PlanMD:       opts.PlanMD,
				State:        prompt.StateToTemplateMap(opts.PhaseState),
				Params:       map[string]string{},
				PlanFile:     filepath.Join(opts.PlansDir, opts.PlanName, "plan.md"),
				PhaseDir:     opts.PhaseDir,
				StateFile:    filepath.Join(opts.PhaseDir, "state.json"),
				ScriptsDir:   filepath.Join(opts.ArcHome, "scripts"),
				Mode:         "",
				DisputeCount: len(opts.PhaseState.Disputes),
				DisputeList:  prompt.FormatDisputeList(opts.PhaseState.Disputes),
			}

			rendered, err := prompt.RenderString(b.Prompt, tmplCtx)
			if err != nil {
				resultsCh <- branchResult{name: b.Name, exitCode: -1, err: err}
				return
			}

			spawnResult, err := agent.Spawn(ctx, agent.SpawnOptions{
				Prompt:      rendered,
				CommandName: agentCommandName,
				Model:       opts.PhaseState.ModelOverride,
			})

			exitCode := -1
			var branchUsage arc.Usage
			if err != nil {
				logger.Error("parallel branch spawn failed", "branch", b.Name, "error", err)
			} else {
				exitCode = spawnResult.ExitCode
				branchUsage = spawnResult.Usage
			}

			// Write log and exit code
			if spawnResult != nil {
				os.WriteFile(filepath.Join(resultsDir, b.Name+".log"), []byte(spawnResult.Output), 0644)
			}
			os.WriteFile(filepath.Join(resultsDir, b.Name+".exit"), []byte(fmt.Sprintf("%d", exitCode)), 0644)

			status := "complete"
			if exitCode != 0 {
				status = "failed"
			}
			if updateErr := state.UpdateParallelBranch(opts.StateFile, b.Name, status, exitCode); updateErr != nil {
				logger.Error("failed to update parallel branch", "branch", b.Name, "error", updateErr)
			}

			resultsCh <- branchResult{name: b.Name, exitCode: exitCode, usage: branchUsage, err: err}
		}(branch)
	}

	wg.Wait()
	close(resultsCh)

	var totalUsage arc.Usage
	exitCodes := make(map[string]int, len(cfg.Branches))
	for r := range resultsCh {
		exitCodes[r.name] = r.exitCode
		totalUsage = totalUsage.Add(r.usage)
	}

	verdict, err := JoinParallel(cfg.Strategy, exitCodes, cfg.N)
	if err != nil {
		return "", totalUsage, fmt.Errorf("joining parallel results: %w", err)
	}

	if err := state.FinishParallel(opts.StateFile, verdict); err != nil {
		return "", totalUsage, fmt.Errorf("finishing parallel tracking: %w", err)
	}

	logger.Info("parallel execution complete",
		"branches", len(cfg.Branches),
		"verdict", verdict,
	)

	return verdict, totalUsage, nil
}
