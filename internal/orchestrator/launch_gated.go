package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/nwiley/arc/internal/adapter"
	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/config"
	"github.com/nwiley/arc/internal/gitops"
	"github.com/nwiley/arc/internal/intelligence"
	"github.com/nwiley/arc/internal/plan"
	"github.com/nwiley/arc/internal/state"
	"github.com/nwiley/arc/internal/testcmd"
	"github.com/nwiley/arc/internal/worktree"
)

// LaunchGated starts the orchestrator for a gate-based plan.
// Each phase uses RunPhaseGated (session → gate → retry) instead of
// the workflow state machine.
//
// Phases are scheduled by dependency — independent phases run concurrently
// (up to Config.MaxParallel), each in its own worktree if UseWorktree is set.
func LaunchGated(ctx context.Context, opts LaunchOptions) (*LaunchResult, error) {
	planDir := filepath.Join(opts.PlansDir, opts.PlanName)

	if err := acquireLock(planDir); err != nil {
		return nil, fmt.Errorf("acquiring lock: %w", err)
	}
	defer releaseLock(planDir)

	// Kill any stale agent processes recorded in phase state files before starting.
	KillStaleAgents(planDir, opts.Logger)

	// SIGHUP handler: reload config from disk when received.
	// Only set up if a config path is known. A mutex-protected holder is used
	// so that SIGHUP reloads and the main scheduling loop can access the config
	// without a data race.
	var cfgMu sync.RWMutex
	getConfig := func() *config.Config {
		cfgMu.RLock()
		defer cfgMu.RUnlock()
		return opts.Config
	}
	if opts.ConfigPath != "" {
		sighupCh := make(chan os.Signal, 1)
		signal.Notify(sighupCh, syscall.SIGHUP)
		go func() {
			for range sighupCh {
				newCfg, loadErr := config.Load(filepath.Dir(opts.ConfigPath))
				if loadErr != nil {
					opts.Logger.Warn("SIGHUP: failed to reload config", "error", loadErr)
					continue
				}
				cfgMu.Lock()
				opts.Config = newCfg
				cfgMu.Unlock()
				opts.Logger.Info("SIGHUP: config reloaded", "path", opts.ConfigPath)
			}
		}()
		defer signal.Stop(sighupCh)
	}

	// Apply wall-clock timeout
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(opts.Timeout)*time.Second)
		defer cancel()
	}

	// Load plan.json
	meta, err := state.ReadPlan(planDir)
	if err != nil {
		return nil, fmt.Errorf("reading plan.json: %w", err)
	}

	// Determine max parallelism (read config via getConfig for race safety).
	maxParallel := 3
	if cfg := getConfig(); cfg != nil && cfg.MaxParallel > 0 {
		maxParallel = cfg.MaxParallel
	}

	// Shared worktree (when not using per-phase worktrees)
	var sharedWorktree *worktree.Worktree
	workingDir := ""
	if opts.UseWorktree && !opts.PerPhaseWorktree {
		projectDir := opts.ProjectDir
		if projectDir == "" {
			projectDir, _ = os.Getwd()
		}
		wt, wtErr := worktree.Create(projectDir, opts.PlanName, "")
		if wtErr != nil {
			opts.Logger.Warn("failed to create shared worktree, running in-tree", "error", wtErr)
		} else {
			sharedWorktree = wt
			workingDir = wt.Dir
		}
	}

	// Initialize structured logger (reuse if provided).
	planLogger := opts.PlanLogger
	if planLogger == nil {
		planLogger = NewPlanLogger(planDir, opts.Logger)
		defer planLogger.Close()
	}

	// Print header
	fmt.Println("==========================================")
	fmt.Println("  Arc Orchestrator (gate-based)")
	fmt.Println("==========================================")
	fmt.Printf("Plan: %s\n", opts.PlanName)
	fmt.Printf("Phases: %s\n", strings.Join(meta.Phases, ", "))
	fmt.Printf("Max parallel: %d\n", maxParallel)
	if opts.Timeout > 0 {
		fmt.Printf("Timeout: %ds\n", opts.Timeout)
	}
	fmt.Println("==========================================")
	fmt.Println()

	buildResult := func(status, failedPhase, failedReason string) *LaunchResult {
		phaseStates := LoadAllPhaseStates(planDir, meta.Phases)
		summary := make(map[string]string, len(meta.Phases))
		var totalUsage arc.Usage
		for _, p := range meta.Phases {
			ps := phaseStates[p]
			if ps == nil {
				summary[p] = "pending"
				continue
			}
			summary[p] = ps.PhaseStatus
			totalUsage = totalUsage.Add(ps.Usage)
		}
		// Promote "complete" to "partial" when some phases completed and others
		// ended in a non-successful terminal state (blocked/deferred/failed).
		if status == "complete" {
			hasComplete := false
			hasNonTerminal := false
			for _, s := range summary {
				switch s {
				case "complete":
					hasComplete = true
				case "blocked", "deferred", "failed":
					hasNonTerminal = true
				}
			}
			if hasComplete && hasNonTerminal {
				status = "partial"
			}
		}
		return &LaunchResult{
			Status:       status,
			FailedPhase:  failedPhase,
			FailedReason: failedReason,
			PhaseSummary: summary,
			Usage:        totalUsage,
		}
	}

	// Track running phases
	running := make(map[string]bool)

	// Scheduling loop — dependency-driven
	for {
		if ctx.Err() != nil {
			return buildResult("cancelled", "", ctx.Err().Error()), ctx.Err()
		}

		phaseStates := LoadAllPhaseStates(planDir, meta.Phases)

		// Check if all done
		allDone := true
		for _, phase := range meta.Phases {
			ps := phaseStates[phase]
			if ps == nil {
				allDone = false
				continue
			}
			status := ps.PhaseStatus
			if status != "complete" && status != "blocked" && status != "deferred" {
				allDone = false
			}
		}

		if allDone {
			// Snapshot the current config under the read lock before passing it to
			// gatedPlanComplete, which runs after the SIGHUP goroutine may still be active.
			snapshotOpts := opts
			snapshotOpts.Config = getConfig()
			return GatedPlanComplete(snapshotOpts, meta, planDir, sharedWorktree, phaseStates)
		}

		// Find ready phases
		ready := state.PhasesReady(meta, phaseStates)
		var toRun []string
		for _, phase := range ready {
			if !running[phase] {
				toRun = append(toRun, phase)
			}
		}

		if len(toRun) == 0 && len(running) == 0 {
			// No phases ready and none running — stuck
			fmt.Println("\nNo phases ready to execute.")
			printBlockedSummary(meta, phaseStates)
			return buildResult("blocked", "", "no runnable phases"), fmt.Errorf("no runnable phases")
		}

		if len(toRun) == 0 {
			// Phases are running but none ready to launch — wait for results
			// This shouldn't happen in the current architecture since we collect
			// all results below before looping, but guard against it.
			return buildResult("blocked", "", "no runnable phases"), fmt.Errorf("no runnable phases")
		}

		// Limit parallelism
		if len(toRun) > maxParallel {
			toRun = toRun[:maxParallel]
		}

		// Launch phases concurrently
		type phaseResult struct {
			phase string
			err   error
		}
		results := make(chan phaseResult, len(toRun))
		var wg sync.WaitGroup

		batchCtx, batchCancel := context.WithCancel(ctx)

		for _, phase := range toRun {
			running[phase] = true
			wg.Add(1)
			go func(phaseName string) {
				defer wg.Done()
				opts.Logger.Info("starting phase", "phase", phaseName)
				fmt.Printf("\n[%s] Starting phase\n", phaseName)

				err := RunPhaseGated(batchCtx, RunPhaseOptions{
					PlanName:    opts.PlanName,
					PhaseName:   phaseName,
					PlansDir:    opts.PlansDir,
					ArcHome:     opts.ArcHome,
					ProjectDir:  opts.ProjectDir,
				Config:      getConfig(),
					Logger:      opts.Logger,
					UseWorktree: opts.UseWorktree && opts.PerPhaseWorktree,
					WorkingDir:  workingDir,
					ChatMode:    opts.ChatMode,
					Resolver:    opts.Resolver,
					PlanLogger:  planLogger,
				})
				results <- phaseResult{phase: phaseName, err: err}
			}(phase)
		}

		go func() {
			wg.Wait()
			close(results)
		}()

		// Collect results
		var fatalErr error
		var failedPhase string
		for r := range results {
			delete(running, r.phase)
			if r.err != nil {
				if ctx.Err() != nil {
					batchCancel()
					return buildResult("cancelled", "", ctx.Err().Error()), ctx.Err()
				}
				ps := LoadPhaseState(planDir, r.phase)
				if ps != nil && (ps.PhaseStatus == "blocked" || ps.PhaseStatus == "deferred") {
					// Tag the shared worktree with the failure reason.
					if sharedWorktree != nil {
						failureReason := r.err.Error()
						if ps.Blocked.Reason != nil {
							failureReason = *ps.Blocked.Reason
						}
						if metaErr := worktree.WriteMetadata(sharedWorktree, failureReason, r.phase); metaErr != nil {
							opts.Logger.Warn("failed to write worktree metadata", "error", metaErr)
						}
					}
					if opts.StopOnFailure {
						if fatalErr == nil {
							fatalErr = fmt.Errorf("phase %s %s: %w", r.phase, ps.PhaseStatus, r.err)
							failedPhase = r.phase
							batchCancel()
						}
					} else {
						fmt.Printf("[%s] Phase %s, continuing...\n", r.phase, ps.PhaseStatus)
					}
					continue
				}
				if fatalErr == nil {
					fatalErr = fmt.Errorf("phase %s failed: %w", r.phase, r.err)
					failedPhase = r.phase
					if opts.StopOnFailure {
						batchCancel()
					}
				}
			} else {
				fmt.Printf("[%s] Complete\n", r.phase)
			}
		}
		batchCancel()

		if fatalErr != nil {
			if opts.StopOnFailure {
				return buildResult("failed", failedPhase, fatalErr.Error()), nil
			}
			// Continue to next loop iteration — other phases might be ready
		}

		// Check plan-wide budget
		if budgetCfg := getConfig(); budgetCfg != nil && budgetCfg.Budget.MaxCost > 0 {
			var totalCost float64
			for _, p := range meta.Phases {
				ps := LoadPhaseState(planDir, p)
				if ps != nil {
					totalCost += ps.Usage.CostUSD
				}
			}
			if totalCost >= budgetCfg.Budget.MaxCost {
				return buildResult("budget_exceeded", "", fmt.Sprintf("budget exceeded: $%.2f spent, limit $%.2f", totalCost, budgetCfg.Budget.MaxCost)),
					fmt.Errorf("budget exceeded")
			}
		}
	}
}

// GatedPlanComplete handles the success path for a gate-based plan:
// run adversary, merge worktree, regression suite, generate report.
func GatedPlanComplete(
	opts LaunchOptions,
	meta *arc.PlanMeta,
	planDir string,
	sharedWorktree *worktree.Worktree,
	phaseStates map[string]*arc.PhaseState,
) (*LaunchResult, error) {
	// Inline buildResult — constructs a LaunchResult from current phase states.
	buildResult := func(status, failedPhase, failedReason string) *LaunchResult {
		summary := make(map[string]string, len(meta.Phases))
		var totalUsage arc.Usage
		for _, p := range meta.Phases {
			ps := phaseStates[p]
			if ps == nil {
				summary[p] = "pending"
				continue
			}
			summary[p] = ps.PhaseStatus
			totalUsage = totalUsage.Add(ps.Usage)
		}
		if status == "complete" {
			hasComplete := false
			hasNonTerminal := false
			for _, s := range summary {
				switch s {
				case "complete":
					hasComplete = true
				case "blocked", "deferred", "failed":
					hasNonTerminal = true
				}
			}
			if hasComplete && hasNonTerminal {
				status = "partial"
			}
		}
		return &LaunchResult{
			Status:       status,
			FailedPhase:  failedPhase,
			FailedReason: failedReason,
			PhaseSummary: summary,
			Usage:        totalUsage,
		}
	}

	fmt.Println("\nAll phases complete.")

	// Determine the working directory (worktree or project dir).
	workDir := opts.ProjectDir
	if sharedWorktree != nil {
		workDir = sharedWorktree.Dir
	}
	if workDir == "" {
		workDir, _ = os.Getwd()
	}

	// Use provided PlanLogger or create a new one.
	planLogger := opts.PlanLogger
	if planLogger == nil {
		planLogger = NewPlanLogger(planDir, opts.Logger)
		defer planLogger.Close()
	}

	// --- Task 1: Run plan-level adversary session ---
	// Gate on: adversary agent is configured (non-empty role) AND there are phase specs.
	adversaryConfigured := opts.Config != nil && opts.Config.AgentForRole("adversary") != ""
	hasPhaseSpecs := false
	for _, p := range meta.Phases {
		if _, err := plan.ReadSpec(opts.PlansDir, opts.PlanName, p); err == nil {
			hasPhaseSpecs = true
			break
		}
	}

	if adversaryConfigured && hasPhaseSpecs {
		fmt.Println("\nRunning plan-level adversary session...")
		adversaryResult, adversaryErr := RunAdversary(context.Background(), opts, workDir, planLogger)
		if adversaryErr != nil {
			opts.Logger.Warn("adversary session failed", "error", adversaryErr)
		} else if adversaryResult != nil {
			if adversaryResult.BugsFound {
				fmt.Printf("Adversary: BUGS FOUND across %d round(s)\n", len(adversaryResult.Rounds))
				planLogger.LogOrchestrator(PhaseEvent{
					Timestamp: time.Now().UTC(),
					Level:     "WARN",
					Component: "adversary",
					Event:     EventAdversaryCompleted,
					Detail:    fmt.Sprintf("bugs_found=true rounds=%d", len(adversaryResult.Rounds)),
				})

				// Persist the bug→phase mapping to plan.json.
				phaseToTests := make(map[string][]string)
				fileToPhase := buildFileToPhaseMap(opts, meta)
				for _, round := range adversaryResult.Rounds {
					for _, testName := range round.FailingTests {
						phase := findResponsiblePhase(testName, round.TestOutput, fileToPhase, opts, meta)
						if phase == "" && len(meta.Phases) > 0 {
							phase = meta.Phases[0]
						}
						if phase != "" {
							phaseToTests[phase] = append(phaseToTests[phase], testName)
						}
					}
				}
				if len(phaseToTests) > 0 {
					meta.AdversaryBugs = phaseToTests
					if writeErr := state.WritePlan(planDir, meta); writeErr != nil {
						opts.Logger.Warn("failed to persist adversary_bugs to plan.json", "error", writeErr)
					}
				}
			} else {
				fmt.Println("Adversary: NO BUGS FOUND — all checks passed")
				planLogger.LogOrchestrator(PhaseEvent{
					Timestamp: time.Now().UTC(),
					Level:     "INFO",
					Component: "adversary",
					Event:     EventAdversaryCompleted,
					Detail:    fmt.Sprintf("bugs_found=false rounds=%d", len(adversaryResult.Rounds)),
				})
			}
		}
	}

	// --- Merge shared worktree ---
	if sharedWorktree != nil {
		commitMsg := fmt.Sprintf("feat(%s): phase work", opts.PlanName)
		if hash, commitErr := gitops.Commit(gitops.CommitOptions{
			Message: commitMsg,
			Dir:     sharedWorktree.Dir,
			Config:  opts.Config,
		}); commitErr != nil {
			opts.Logger.Warn("pre-merge commit failed", "error", commitErr)
		} else if hash != "" {
			fmt.Printf("Committed worktree changes: %s\n", shortHash(hash))
		}

		// Check for uncommitted changes in the project dir before merging.
		// If dirty, skip the merge — the worktree is durable and can be
		// merged later once the user commits or stashes their changes.
		if isDirty, dirtyErr := gitops.IsDirty(opts.ProjectDir); dirtyErr != nil {
			opts.Logger.Warn("failed to check project dir status", "error", dirtyErr)
		} else if isDirty {
			fmt.Printf("\nSkipping merge: project directory has uncommitted changes.\n")
			fmt.Printf("Your completed work is safe in the worktree: %s\n", sharedWorktree.Dir)
			fmt.Printf("To merge manually:\n")
			fmt.Printf("  1. Commit or stash your changes\n")
			fmt.Printf("  2. git merge --no-ff %s\n", sharedWorktree.Branch)
			return buildResult("complete", "", ""), nil
		}

		if hash, mergeErr := worktree.MergeBack(sharedWorktree); mergeErr != nil {
			opts.Logger.Warn("worktree merge failed", "branch", sharedWorktree.Branch, "error", mergeErr)

			// --- Task 5: Merge conflict re-run ---
			// Identify which phase(s) caused the conflict and re-run in-tree.
			conflictPhase := identifyConflictingPhase(opts, meta, mergeErr)
			if conflictPhase != "" {
				fmt.Printf("Merge conflict detected — re-running phase %q in-tree\n", conflictPhase)
				rerunErr := rerunPhaseInTree(context.Background(), opts, meta, conflictPhase, planDir, phaseStates, planLogger)
				if rerunErr != nil {
					opts.Logger.Warn("in-tree re-run failed", "phase", conflictPhase, "error", rerunErr)
					return buildResult("failed", conflictPhase, fmt.Sprintf("merge conflict and re-run failed: %v", rerunErr)),
						fmt.Errorf("merge conflict and re-run failed: %w", rerunErr)
				}
				fmt.Printf("In-tree re-run of %q succeeded\n", conflictPhase)
				worktree.Remove(sharedWorktree)
			} else {
				return buildResult("failed", "", fmt.Sprintf("worktree merge failed: %v", mergeErr)),
					fmt.Errorf("worktree merge failed: %w", mergeErr)
			}
		} else {
			fmt.Printf("Merged worktree: %s\n", shortHash(hash))
			worktree.Remove(sharedWorktree)
		}
	}

	// --- Run full regression suite after merge ---
	tenv := testcmd.NewEnv(testcmd.WithConfig(opts.Config), testcmd.WithProjectDir(opts.ProjectDir))
	if tenv.Command != "" {
		fmt.Println("\nRunning full regression suite...")
		regressionResult, _ := tenv.RunAll(context.Background())
		if !regressionResult.Passed {
			fmt.Printf("Regression suite FAILED:\n%s\n", regressionResult.Output)
			opts.Logger.Warn("regression suite failed", "output", regressionResult.Output)

			// --- Task 2: Route regression failures to responsible phases ---
			phaseStates = LoadAllPhaseStates(planDir, meta.Phases)
			if routeErr := routeRegressionFailure(context.Background(), opts, meta, phaseStates, regressionResult.Output, opts.ProjectDir); routeErr != nil {
				opts.Logger.Warn("regression failure routing failed", "error", routeErr)
			}
		} else {
			fmt.Println("Regression suite PASSED")
		}
	}

	// Generate reports
	if err := generateCompletionReport(planDir, opts.PlanName, meta, phaseStates); err != nil {
		return nil, err
	}
	if _, err := plan.GenerateSummary(plan.SummaryOptions{
		PlanDir:     planDir,
		PlanName:    opts.PlanName,
		Meta:        meta,
		PhaseStates: phaseStates,
		ProjectDir:  opts.ProjectDir,
	}); err != nil {
		opts.Logger.Warn("failed to generate summary", "error", err)
	}

	// Record intelligence from this run.
	projectDir := opts.ProjectDir
	if projectDir == "" {
		projectDir, _ = os.Getwd()
	}
	if intel, err := intelligence.Open(projectDir); err == nil {
		// Record cost per phase.
		for _, p := range meta.Phases {
			ps := phaseStates[p]
			if ps != nil && ps.Usage.CostUSD > 0 {
				spec, _ := plan.ReadSpec(opts.PlansDir, opts.PlanName, p)
				complexity := "medium"
				if spec != nil && spec.Complexity != "" {
					complexity = spec.Complexity
				}
				intel.RecordCost(opts.PlanName, complexity, ps.Usage.CostUSD, ps.Iteration.Current)
			}
		}
		// Record file coupling from phase specs.
		for _, p := range meta.Phases {
			spec, _ := plan.ReadSpec(opts.PlansDir, opts.PlanName, p)
			if spec != nil && len(spec.Files) > 1 {
				intel.RecordFileCoupling(spec.Files)
			}
		}
		// Record conventions from successful phases.
		for _, p := range meta.Phases {
			ps := phaseStates[p]
			if ps == nil || ps.PhaseStatus != "complete" {
				continue
			}
			spec, _ := plan.ReadSpec(opts.PlansDir, opts.PlanName, p)
			if spec == nil {
				continue
			}
			// Detect test files alongside source files (Go convention).
			hasSource := false
			hasTest := false
			for _, f := range spec.Files {
				if strings.HasSuffix(f, "_test.go") {
					hasTest = true
				} else if strings.HasSuffix(f, ".go") {
					hasSource = true
				}
			}
			if hasSource && hasTest {
				intel.RecordConvention("file_structure", "Test files alongside source (*_test.go)")
			}
			// Detect TestXxx naming convention from test file names.
			for _, f := range spec.Files {
				if strings.HasSuffix(f, "_test.go") {
					intel.RecordConvention("test_naming", "Test functions use TestXxx naming (Go stdlib)")
					break
				}
			}
		}
		// Record failure patterns from blocked phases.
		for _, p := range meta.Phases {
			ps := phaseStates[p]
			if ps == nil || ps.PhaseStatus != "blocked" {
				continue
			}
			if ps.Blocked.Reason != nil && *ps.Blocked.Reason != "" {
				intel.RecordFailurePattern(*ps.Blocked.Reason, "review blocked phase output for fix guidance")
			}
		}
		if saveErr := intel.Save(); saveErr != nil {
			opts.Logger.Warn("failed to save intelligence", "error", saveErr)
		}
	}

	return buildResult("complete", "", ""), nil
}

// routeRegressionFailure parses failing tests from regression output, filters known-flaky
// tests, matches failures to responsible phases, and spawns fix sessions.
// It retries the regression suite up to 2 times after fixes.
func routeRegressionFailure(
	ctx context.Context,
	opts LaunchOptions,
	meta *arc.PlanMeta,
	phaseStates map[string]*arc.PhaseState,
	regressionOutput string,
	workDir string,
) error {
	// Open intelligence store for flaky test filtering.
	projectDir := opts.ProjectDir
	if projectDir == "" {
		projectDir, _ = os.Getwd()
	}
	var intel *intelligence.Store
	if s, openErr := intelligence.Open(projectDir); openErr == nil {
		intel = s
	}

	// Parse failing tests from the regression output.
	allFailing := testcmd.ParseFailures(regressionOutput)
	if len(allFailing) == 0 {
		// Could not identify specific test names — nothing to route.
		return nil
	}

	// Filter out known-flaky failures.
	failing := intelligence.FilterFlakyTests(intel, allFailing)
	if len(failing) == 0 {
		fmt.Println("All regression failures are known-flaky — skipping fix routing")
		// Record these as having passed (they're flaky).
		if intel != nil {
			for _, name := range allFailing {
				intel.RecordFlakyTest(name, true)
			}
			_ = intel.Save()
		}
		return nil
	}

	fmt.Printf("Regression: %d non-flaky failures to route\n", len(failing))

	// Build a map of file → phase for ownership resolution.
	fileToPhase := buildFileToPhaseMap(opts, meta)

	// Match each failing test to a responsible phase.
	phaseToTests := make(map[string][]string)
	unmatched := make([]string, 0)
	for _, testName := range failing {
		responsible := findResponsiblePhase(testName, regressionOutput, fileToPhase, opts, meta)
		if responsible != "" {
			phaseToTests[responsible] = append(phaseToTests[responsible], testName)
		} else {
			unmatched = append(unmatched, testName)
		}
	}

	// If we couldn't match any tests to phases, use the first available phase.
	if len(phaseToTests) == 0 && len(unmatched) > 0 && len(meta.Phases) > 0 {
		phaseToTests[meta.Phases[0]] = unmatched
		unmatched = nil
	}

	if len(phaseToTests) == 0 {
		return nil
	}

	// Spawn fix sessions per phase.
	adapterName := "claude"
	if opts.Config != nil {
		adapterName = opts.Config.AgentForRole("impl")
	}
	agentAdapter := adapter.Get(adapterName)

	for phase, tests := range phaseToTests {
		fixPrompt := buildRegressionFixPrompt(phase, tests, regressionOutput, opts)
		sessionCfg := arc.SessionConfig{
			MaxTurns: 50,
			Timeout:  15 * time.Minute,
		}
		fmt.Printf("Routing regression fix to phase %q (%d failing tests)\n", phase, len(tests))
		if _, spawnErr := agentAdapter.Spawn(ctx, fixPrompt, workDir, sessionCfg); spawnErr != nil {
			opts.Logger.Warn("regression fix spawn failed", "phase", phase, "error", spawnErr)
		}
	}

	// Re-run regression suite up to 2 times after fixes.
	rtenv := testcmd.NewEnv(testcmd.WithConfig(opts.Config), testcmd.WithProjectDir(workDir))
	for attempt := 1; attempt <= 2; attempt++ {
		fmt.Printf("Re-running regression suite (attempt %d/2)...\n", attempt)
		rerunResult, _ := rtenv.RunAll(ctx)
		if rerunResult.Passed {
			fmt.Println("Regression suite PASSED after fix")
			return nil
		}

		// Check if remaining failures are all flaky.
		rerunFailing := testcmd.ParseFailures(rerunResult.Output)
		stillFailing := intelligence.FilterFlakyTests(intel, rerunFailing)

		// Record tests that failed before but passed now as potentially flaky.
		if intel != nil {
			prevFailSet := make(map[string]bool, len(failing))
			for _, name := range failing {
				prevFailSet[name] = true
			}
			rerunFailSet := make(map[string]bool, len(rerunFailing))
			for _, name := range rerunFailing {
				rerunFailSet[name] = true
			}
			for _, name := range failing {
				if !rerunFailSet[name] {
					intel.RecordFlakyTest(name, true)
				}
			}
			_ = intel.Save()
		}

		if len(stillFailing) == 0 {
			fmt.Println("Regression suite: remaining failures are known-flaky — treating as passed")
			return nil
		}

		fmt.Printf("Regression still failing after fix (attempt %d/2): %d tests\n", attempt, len(stillFailing))
		if attempt == 2 {
			opts.Logger.Warn("regression suite still failing after 2 fix attempts", "failing", stillFailing)
		}
	}

	return nil
}

// buildRegressionFixPrompt constructs a prompt for fixing regression test failures.
func buildRegressionFixPrompt(phase string, tests []string, regressionOutput string, opts LaunchOptions) string {
	tenv := testcmd.NewEnv(testcmd.WithConfig(opts.Config), testcmd.WithProjectDir(opts.ProjectDir))
	return fmt.Sprintf(`You are fixing regression test failures in phase %q.

## Failing Tests
%s

## Regression Output
%s

## Instructions
- Read the failing test output carefully
- Fix the bugs in the implementation (not in the tests)
- Run the test suite to verify your fixes: %s
- Do not modify existing test files unless they contain a genuine bug
`,
		phase,
		strings.Join(tests, "\n"),
		regressionOutput,
		tenv.Command,
	)
}

// buildFileToPhaseMap creates a mapping from file path to phase name using phase specs.
func buildFileToPhaseMap(opts LaunchOptions, meta *arc.PlanMeta) map[string]string {
	fileToPhase := make(map[string]string)
	for _, p := range meta.Phases {
		spec, err := plan.ReadSpec(opts.PlansDir, opts.PlanName, p)
		if err != nil {
			continue
		}
		for _, f := range spec.Files {
			if _, exists := fileToPhase[f]; !exists {
				fileToPhase[f] = p
			}
		}
	}
	return fileToPhase
}

// findResponsiblePhase attempts to match a failing test to the phase that owns
// the relevant files, by scanning the test output for file path mentions.
func findResponsiblePhase(testName, regressionOutput string, fileToPhase map[string]string, opts LaunchOptions, meta *arc.PlanMeta) string {
	bestPhase := ""
	bestScore := 0
	for _, p := range meta.Phases {
		spec, err := plan.ReadSpec(opts.PlansDir, opts.PlanName, p)
		if err != nil {
			continue
		}
		score := 0
		for _, f := range spec.Files {
			if strings.Contains(regressionOutput, f) {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			bestPhase = p
		}
	}
	return bestPhase
}

// identifyConflictingPhase tries to determine which phase caused a merge conflict.
// It checks phase file lists against the error output for file path mentions.
// Returns "" if it cannot determine the responsible phase.
func identifyConflictingPhase(opts LaunchOptions, meta *arc.PlanMeta, mergeErr error) string {
	errStr := mergeErr.Error()
	bestPhase := ""
	bestScore := 0
	for _, p := range meta.Phases {
		spec, err := plan.ReadSpec(opts.PlansDir, opts.PlanName, p)
		if err != nil {
			continue
		}
		score := 0
		for _, f := range spec.Files {
			if strings.Contains(errStr, f) {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			bestPhase = p
		}
	}
	// If we found at least one file mention, return the phase.
	if bestScore > 0 {
		return bestPhase
	}
	// Fallback: return the first phase if there's only one.
	if len(meta.Phases) == 1 {
		return meta.Phases[0]
	}
	return ""
}

// rerunPhaseInTree re-runs a phase directly in the project directory (not in a worktree).
// Used as a recovery mechanism when a worktree merge conflict occurs.
func rerunPhaseInTree(
	ctx context.Context,
	opts LaunchOptions,
	meta *arc.PlanMeta,
	phaseName string,
	planDir string,
	phaseStates map[string]*arc.PhaseState,
	planLogger *PlanLogger,
) error {
	// Reset the phase state to pending so RunPhaseGated will execute it fresh.
	phaseDir := filepath.Join(planDir, "phases", phaseName)
	sf := state.NewStateFile(filepath.Join(phaseDir, "state.json"))
	if updateErr := sf.Update(func(s *arc.PhaseState) error {
		s.PhaseStatus = "pending"
		s.CurrentState = ""
		return nil
	}); updateErr != nil {
		return fmt.Errorf("resetting phase state: %w", updateErr)
	}

	fmt.Printf("[%s] Re-running in-tree after merge conflict\n", phaseName)
	if planLogger != nil {
		planLogger.RetryTriggered(phaseName, 0, "merge conflict recovery: re-running in-tree")
	}

	return RunPhaseGated(ctx, RunPhaseOptions{
		PlanName:   opts.PlanName,
		PhaseName:  phaseName,
		PlansDir:   opts.PlansDir,
		ArcHome:    opts.ArcHome,
		ProjectDir: opts.ProjectDir,
		Config:     opts.Config,
		Logger:     opts.Logger,
		UseWorktree: false, // explicitly in-tree
		WorkingDir:  opts.ProjectDir,
		ChatMode:   opts.ChatMode,
		Resolver:   opts.Resolver,
		PlanLogger: planLogger,
	})
}

// KillStaleAgents scans all phase state files for non-zero AgentPID values.
// For each, it checks if the process is still alive (via signal 0). If alive,
// it sends SIGTERM, waits up to 5 seconds, then SIGKILLs. After termination
// it resets AgentPID to 0 in the phase state.
func KillStaleAgents(planDir string, logger *slog.Logger) {
	phasesDir := filepath.Join(planDir, "phases")
	entries, err := os.ReadDir(phasesDir)
	if err != nil {
		return
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		phaseName := e.Name()
		phaseDir := filepath.Join(phasesDir, phaseName)
		sf := state.NewStateFile(filepath.Join(phaseDir, "state.json"))
		ps, readErr := sf.Read()
		if readErr != nil || ps == nil || ps.AgentPID == 0 {
			continue
		}

		pid := ps.AgentPID
		proc, findErr := os.FindProcess(pid)
		if findErr != nil {
			// Can't find process — clear the PID and continue.
			_ = sf.Update(func(s *arc.PhaseState) error {
				s.AgentPID = 0
				return nil
			})
			continue
		}

		// Check if process is alive using signal 0.
		if err := proc.Signal(syscall.Signal(0)); err != nil {
			// Process is not alive — clear the PID.
			_ = sf.Update(func(s *arc.PhaseState) error {
				s.AgentPID = 0
				return nil
			})
			continue
		}

		// Process is alive — send SIGTERM first.
		logger.Warn("killing stale agent process", "phase", phaseName, "pid", pid)
		fmt.Printf("[%s] Killing stale agent process (PID %d)\n", phaseName, pid)
		_ = proc.Signal(syscall.SIGTERM)

		// Wait up to 5 seconds for graceful exit.
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			time.Sleep(200 * time.Millisecond)
			if err := proc.Signal(syscall.Signal(0)); err != nil {
				break // process is gone
			}
		}

		// If still alive, SIGKILL.
		if err := proc.Signal(syscall.Signal(0)); err == nil {
			logger.Warn("SIGTERM did not terminate stale agent, sending SIGKILL", "pid", pid)
			_ = proc.Signal(syscall.SIGKILL)
		}

		// Clear the PID and reset phase to pending so it can be re-run.
		_ = sf.Update(func(s *arc.PhaseState) error {
			s.AgentPID = 0
			if s.PhaseStatus == "implementing" {
				s.PhaseStatus = "pending"
			}
			return nil
		})
	}
}
