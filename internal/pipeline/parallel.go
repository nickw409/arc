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
}

// RunParallel spawns agents for each branch in parallel, collects results,
// and returns a verdict based on the configured strategy.
func RunParallel(ctx context.Context, logger *slog.Logger, opts RunParallelOptions) (string, error) {
	cfg := opts.Config
	if cfg == nil || len(cfg.Branches) == 0 {
		return "", fmt.Errorf("no parallel branches configured")
	}

	stateName := opts.PhaseState.CurrentState
	resultsDir := filepath.Join(opts.PhaseDir, fmt.Sprintf("parallel_%s", stateName))
	if err := os.MkdirAll(resultsDir, 0755); err != nil {
		return "", fmt.Errorf("creating results dir: %w", err)
	}

	branchNames := make([]string, len(cfg.Branches))
	for i, b := range cfg.Branches {
		branchNames[i] = b.Name
	}

	if err := state.StartParallel(opts.StateFile, resultsDir, branchNames); err != nil {
		return "", fmt.Errorf("starting parallel tracking: %w", err)
	}

	type branchResult struct {
		name     string
		exitCode int
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
				Phase:     opts.PhaseState.Phase,
				Plan:      opts.PhaseState.Plan,
				Iteration: opts.PhaseState.Iteration.Current,
				PlanMD:    opts.PlanMD,
				State:     prompt.StateToTemplateMap(opts.PhaseState),
				Params:    map[string]string{},
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
			if err != nil {
				logger.Error("parallel branch spawn failed", "branch", b.Name, "error", err)
			} else {
				exitCode = spawnResult.ExitCode
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

			resultsCh <- branchResult{name: b.Name, exitCode: exitCode, err: err}
		}(branch)
	}

	wg.Wait()
	close(resultsCh)

	exitCodes := make(map[string]int, len(cfg.Branches))
	for r := range resultsCh {
		exitCodes[r.name] = r.exitCode
	}

	verdict, err := JoinParallel(cfg.Strategy, exitCodes, cfg.N)
	if err != nil {
		return "", fmt.Errorf("joining parallel results: %w", err)
	}

	if err := state.FinishParallel(opts.StateFile, verdict); err != nil {
		return "", fmt.Errorf("finishing parallel tracking: %w", err)
	}

	logger.Info("parallel execution complete",
		"branches", len(cfg.Branches),
		"verdict", verdict,
	)

	return verdict, nil
}
