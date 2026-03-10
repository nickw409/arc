package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/intelligence"
	"github.com/nwiley/arc/internal/orchestrator"
	"github.com/nwiley/arc/internal/state"
)

// PhaseRunner runs a single phase. Called by the scheduler in a goroutine.
type PhaseRunner func(ctx context.Context, reg *PlanRegistration, phaseName string) error

// Finalizer runs plan finalization (adversary, merge, regression).
type Finalizer func(reg *PlanRegistration)

// Scheduler dispatches phases from all registered plans into a bounded worker pool.
type Scheduler struct {
	mu             sync.Mutex
	registrations  map[string]*PlanRegistration
	running        map[string]map[string]bool // planName -> phaseName -> running
	dirty          map[string]bool
	slots          chan struct{} // semaphore
	wake           chan struct{} // buffered(1)
	done           chan PhaseResult
	draining       bool
	runner         PhaseRunner
	finalizer      Finalizer
	OnStateChange  func()
	ShutdownFn     func() // called when draining and all plans have finished
	logger         *slog.Logger
	maxParallel    int
	throttleSlots  int
	throttleTimer  *time.Timer
	IntelStore     *intelligence.Store
}

// NewScheduler creates a scheduler with the given concurrency limit.
func NewScheduler(maxParallel int, runner PhaseRunner, finalizer Finalizer) *Scheduler {
	return &Scheduler{
		registrations: make(map[string]*PlanRegistration),
		running:       make(map[string]map[string]bool),
		dirty:         make(map[string]bool),
		slots:         make(chan struct{}, maxParallel),
		wake:          make(chan struct{}, 1),
		done:          make(chan PhaseResult, maxParallel),
		runner:        runner,
		finalizer:     finalizer,
		logger:        slog.Default(),
		maxParallel:   maxParallel,
	}
}

// SetRunner replaces the phase runner. Used to break initialization cycles
// when the runner itself needs a reference to the scheduler.
func (s *Scheduler) SetRunner(runner PhaseRunner) {
	s.runner = runner
}

// RateLimitSignal is called when a phase agent receives a rate-limit response.
// It adaptively reduces the concurrency by one slot (down to a minimum of 1)
// and schedules a slot restoration after 30 seconds of quiescence.
func (s *Scheduler) RateLimitSignal() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.throttleSlots >= s.maxParallel-1 {
		// Already at minimum concurrency; nothing more to drain.
		return
	}

	// Try to drain one token from the semaphore (non-blocking).
	select {
	case <-s.slots:
		// Successfully drained a slot.
	default:
		// All slots are currently held by running goroutines; no free slot to drain.
		return
	}

	s.throttleSlots++

	// Record rate limit event in the intelligence store.
	// adapterName defaults to "claude" — the scheduler doesn't track per-adapter concurrency yet;
	// this is a simplification that assumes claude is the primary adapter.
	if s.IntelStore != nil {
		currentRunning := 0
		for _, phases := range s.running {
			currentRunning += len(phases)
		}
		adapterName := "claude"
		go s.IntelStore.RecordRateLimit(adapterName, currentRunning)
	}

	// Reset (or create) the restore timer.
	if s.throttleTimer != nil {
		s.throttleTimer.Stop()
	}
	s.throttleTimer = time.AfterFunc(30*time.Second, s.restoreSlot)
}

// restoreSlot returns one previously throttled slot to the semaphore.
func (s *Scheduler) restoreSlot() {
	s.mu.Lock()
	if s.throttleSlots <= 0 {
		s.mu.Unlock()
		return
	}
	s.throttleSlots--
	s.mu.Unlock()

	// Send the token back outside the lock to avoid deadlock.
	s.slots <- struct{}{}
}

// Run is the main dispatch loop. It blocks until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case result := <-s.done:
			s.handlePhaseResult(result)
		case <-s.wake:
		}

		// Drain all pending events before dispatching.
		for {
			select {
			case result := <-s.done:
				s.handlePhaseResult(result)
			case <-s.wake:
			default:
				goto dispatch
			}
		}
	dispatch:
		s.dispatchReady()
	}
}

// Register adds a plan to the scheduler. Returns error if draining or duplicate.
func (s *Scheduler) Register(reg *PlanRegistration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.draining {
		return fmt.Errorf("daemon is draining, not accepting new plans")
	}
	if _, exists := s.registrations[reg.PlanName]; exists {
		return fmt.Errorf("plan %q is already registered", reg.PlanName)
	}

	s.registrations[reg.PlanName] = reg
	s.dirty[reg.PlanName] = true

	if s.OnStateChange != nil {
		s.OnStateChange()
	}

	// Wake the scheduler.
	select {
	case s.wake <- struct{}{}:
	default:
	}
	return nil
}

// Cancel cancels a running plan.
func (s *Scheduler) Cancel(planName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	reg, ok := s.registrations[planName]
	if !ok {
		return fmt.Errorf("plan %q not found", planName)
	}
	s.cancelPlanLocked(reg, "cancelled by user")
	return nil
}

// Sync reloads all phase states from disk for the named plan and re-evaluates
// scheduling. This is useful after an external tool (e.g. arc manage) modifies
// a phase state directly without going through the daemon.
func (s *Scheduler) Sync(planName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	reg, ok := s.registrations[planName]
	if !ok {
		return fmt.Errorf("plan %q not found in daemon (not submitted or already finalized)", planName)
	}

	pd := planDir(reg)
	updated := orchestrator.LoadAllPhaseStates(pd, reg.Meta.Phases)
	for phase, ps := range updated {
		reg.PhaseStates[phase] = ps
	}
	s.dirty[planName] = true
	select {
	case s.wake <- struct{}{}:
	default:
	}
	return nil
}

// Drain stops accepting new plans and shuts down once all current plans finish.
func (s *Scheduler) Drain() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.draining = true
	// If already idle, shut down immediately.
	if len(s.registrations) == 0 && s.ShutdownFn != nil {
		go s.ShutdownFn()
	}
}

// Status returns the status of a single plan.
func (s *Scheduler) Status(planName string) *Response {
	s.mu.Lock()
	defer s.mu.Unlock()

	reg, ok := s.registrations[planName]
	if !ok {
		return &Response{OK: false, Error: fmt.Sprintf("plan %q not found", planName)}
	}

	return s.buildPlanStatus(reg)
}

// GlobalStatus returns the status of all registered plans.
func (s *Scheduler) GlobalStatus() *Response {
	s.mu.Lock()
	defer s.mu.Unlock()

	resp := &Response{OK: true}
	for _, reg := range s.registrations {
		sub := s.buildPlanStatus(reg)
		resp.Phases = append(resp.Phases, sub.Phases...)
		resp.QueuedPhases += sub.QueuedPhases
	}
	return resp
}

// Registrations returns a snapshot of all current registrations.
func (s *Scheduler) Registrations() []*PlanRegistration {
	s.mu.Lock()
	defer s.mu.Unlock()

	regs := make([]*PlanRegistration, 0, len(s.registrations))
	for _, reg := range s.registrations {
		regs = append(regs, reg)
	}
	return regs
}

// ListPlans returns per-plan diagnostics for all active registrations, sorted by plan name.
func (s *Scheduler) ListPlans() []ActivePlanInfo {
	s.mu.Lock()
	defer s.mu.Unlock()

	names := make([]string, 0, len(s.registrations))
	for name := range s.registrations {
		names = append(names, name)
	}
	sort.Strings(names)

	plans := make([]ActivePlanInfo, 0, len(s.registrations))
	for _, name := range names {
		reg := s.registrations[name]
		status := s.buildPlanStatus(reg)
		plans = append(plans, ActivePlanInfo{
			PlanName:    reg.PlanName,
			ProjectDir:  reg.ProjectDir,
			Phases:      status.Phases,
			SubmittedAt: reg.SubmittedAt.UTC().Format(time.RFC3339),
		})
	}
	return plans
}

func (s *Scheduler) buildPlanStatus(reg *PlanRegistration) *Response {
	resp := &Response{
		OK:       true,
		PlanName: reg.PlanName,
	}

	if reg.PendingFinalize {
		resp.PlanStatus = "finalizing"
	} else {
		resp.PlanStatus = "running"
	}

	running := s.running[reg.PlanName]
	for phaseName, ps := range reg.PhaseStates {
		if ps == nil {
			continue
		}
		info := PhaseInfo{
			Name:         phaseName,
			Iteration:    ps.Iteration.Current,
			TestsPassing: ps.TestsPassing,
			TestsTotal:   ps.TestsTotal,
		}
		if running[phaseName] {
			info.Status = "running"
		} else {
			info.Status = ps.PhaseStatus
			if ps.PhaseStatus == "blocked" {
				info.BlockedReason = ps.BlockedReason
			}
		}
		resp.Phases = append(resp.Phases, info)
	}
	return resp
}

func (s *Scheduler) dispatchReady() {
	ready := s.collectReadyWork()
	for _, item := range ready {
		select {
		case s.slots <- struct{}{}: // acquire semaphore slot
			s.mu.Lock()
			if item.IsFinalize {
				// No running map update needed for finalize.
			} else {
				if s.running[item.PlanName] == nil {
					s.running[item.PlanName] = make(map[string]bool)
				}
				s.running[item.PlanName][item.PhaseName] = true
			}
			s.mu.Unlock()
			go s.runWork(item)
		default:
			return // no free slots
		}
	}
}

func (s *Scheduler) collectReadyWork() []WorkItem {
	s.mu.Lock()
	defer s.mu.Unlock()

	var items []WorkItem
	for planName := range s.dirty {
		reg, ok := s.registrations[planName]
		if !ok {
			continue
		}

		if reg.PendingFinalize {
			items = append(items, WorkItem{
				PlanName:     planName,
				IsFinalize:   true,
				Registration: reg,
			})
			continue
		}

		ready := state.PhasesReady(reg.Meta, reg.PhaseStates)
		running := s.running[planName]
		for _, phaseName := range ready {
			if running[phaseName] {
				continue
			}
			items = append(items, WorkItem{
				PlanName:     planName,
				PhaseName:    phaseName,
				Registration: reg,
			})
		}
	}

	// Clear dirty set.
	s.dirty = make(map[string]bool)

	// Sort by SubmittedAt (FIFO).
	sort.Slice(items, func(i, j int) bool {
		return items[i].Registration.SubmittedAt.Before(items[j].Registration.SubmittedAt)
	})

	return items
}

func (s *Scheduler) runWork(item WorkItem) {
	defer func() { <-s.slots }() // release semaphore

	if item.IsFinalize {
		if s.finalizer != nil {
			s.finalizer(item.Registration)
		}
		s.done <- PhaseResult{PlanName: item.PlanName, Finalize: true}
		return
	}

	err := s.runner(item.Registration.Ctx, item.Registration, item.PhaseName)
	s.done <- PhaseResult{PlanName: item.PlanName, PhaseName: item.PhaseName, Err: err}
}

func (s *Scheduler) handlePhaseResult(result PhaseResult) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Handle watch intervention results.
	if result.WatchIntervention {
		reg, ok := s.registrations[result.PlanName]
		if !ok {
			return
		}

		// Reload phase state — intervention agent modified files on disk.
		pd := planDir(reg)
		updated := orchestrator.LoadPhaseState(pd, result.PhaseName)
		if updated != nil {
			reg.PhaseStates[result.PhaseName] = updated
		}

		reg.WatchInflight--
		s.logger.Info("watch intervention result",
			"plan", result.PlanName, "phase", result.PhaseName,
			"inflight_remaining", reg.WatchInflight)

		if reg.WatchInflight == 0 {
			reg.PendingWatch = false

			// Re-evaluate plan state now that all interventions have completed.
			allTerminal := true
			allComplete := true
			for _, phase := range reg.Meta.Phases {
				ps := reg.PhaseStates[phase]
				if ps == nil {
					allTerminal = false
					allComplete = false
					break
				}
				switch ps.PhaseStatus {
				case "complete":
					// good
				case "blocked", "deferred":
					allComplete = false
				default:
					allTerminal = false
					allComplete = false
				}
			}

			if !allTerminal {
				// Some phases are now pending (ResetToRetry succeeded) — let normal scheduling handle it.
				s.dirty[result.PlanName] = true
				select {
				case s.wake <- struct{}{}:
				default:
				}
			} else if allComplete {
				reg.PendingFinalize = true
				s.dirty[result.PlanName] = true
				select {
				case s.wake <- struct{}{}:
				default:
				}
			} else {
				// Still terminal but not all complete. Check for another watch round.
				var eligible []string
				for _, phase := range reg.Meta.Phases {
					ps := reg.PhaseStates[phase]
					if ps != nil && ps.PhaseStatus == "blocked" && ps.WatchAttempts < MaxWatchAttempts {
						eligible = append(eligible, phase)
					}
				}
				if len(eligible) > 0 {
					reg.PendingWatch = true
					reg.WatchInflight = len(eligible)
					for _, phase := range eligible {
						if ps := reg.PhaseStates[phase]; ps != nil {
							ps.WatchAttempts++
						}
					}
					s.logger.Info("watch: firing interventions (retry round)",
						"plan", reg.PlanName, "count", len(eligible), "phases", eligible)
					go s.runWatchInterventions(reg.Ctx, reg, eligible)
				} else {
					s.releasePlanLocked(reg, "partial")
				}
			}
		}
		return
	}

	reg, ok := s.registrations[result.PlanName]
	if !ok {
		return
	}

	if result.Finalize {
		s.releasePlanLocked(reg, "complete")
		return
	}

	// Load updated phase state from disk.
	planDir := planDir(reg)
	updated := orchestrator.LoadPhaseState(planDir, result.PhaseName)
	if updated != nil {
		reg.PhaseStates[result.PhaseName] = updated
	}

	// Remove from running map.
	if running, ok := s.running[result.PlanName]; ok {
		delete(running, result.PhaseName)
	}

	// If the runner returned an error and the phase is still pending on disk,
	// the runner failed before it could write any state (e.g. unparseable spec).
	// Block the phase so the scheduler does not immediately re-queue it.
	if result.Err != nil {
		ps := reg.PhaseStates[result.PhaseName]
		if ps == nil || ps.PhaseStatus == "pending" {
			reason := result.Err.Error()
			statePath := filepath.Join(planDir, "phases", result.PhaseName, "state.json")
			sf := state.NewStateFile(statePath)
			if blockErr := sf.Update(func(s *arc.PhaseState) error {
				s.PhaseStatus = "blocked"
				s.BlockedReason = reason
				s.BlockedAt = time.Now().UTC().Format(time.RFC3339)
				s.Blocked = arc.BlockedInfo{IsBlocked: true, Reason: &reason}
				return nil
			}); blockErr != nil {
				s.logger.Warn("could not block phase after runner error", "phase", result.PhaseName, "err", blockErr)
			}
			if updated := orchestrator.LoadPhaseState(planDir, result.PhaseName); updated != nil {
				reg.PhaseStates[result.PhaseName] = updated
			}
		}
	}

	// Mark plan dirty for re-evaluation.
	s.dirty[result.PlanName] = true

	// Budget check.
	if reg.Config != nil && reg.Config.Budget.MaxCost > 0 {
		var totalCost float64
		for _, ps := range reg.PhaseStates {
			if ps != nil {
				totalCost += ps.Usage.CostUSD
			}
		}
		if totalCost >= reg.Config.Budget.MaxCost {
			s.cancelPlanLocked(reg, fmt.Sprintf("budget exceeded: $%.2f spent, limit $%.2f",
				totalCost, reg.Config.Budget.MaxCost))
			return
		}
	}

	// StopOnFailure check.
	if reg.StopOnFailure && result.Err != nil {
		s.cancelPlanLocked(reg, fmt.Sprintf("phase %q failed: %v", result.PhaseName, result.Err))
		return
	}

	// Plan completion detection from cached states.
	allTerminal := true
	allComplete := true
	for _, phase := range reg.Meta.Phases {
		ps, ok := reg.PhaseStates[phase]
		if !ok || ps == nil {
			allTerminal = false
			allComplete = false
			break
		}
		switch ps.PhaseStatus {
		case "complete":
			// good
		case "blocked", "deferred":
			allComplete = false
		default:
			allTerminal = false
			allComplete = false
		}
	}

	if allTerminal && allComplete {
		reg.PendingFinalize = true
		// dirty is already set, dispatchReady will pick it up
	} else if allTerminal {
		// Some phases are blocked/deferred. Check for watch-eligible phases.
		var eligible []string
		for _, phase := range reg.Meta.Phases {
			ps, ok := reg.PhaseStates[phase]
			if !ok || ps == nil {
				continue
			}
			if ps.PhaseStatus == "blocked" && ps.WatchAttempts < MaxWatchAttempts {
				eligible = append(eligible, phase)
			}
		}

		if len(eligible) > 0 {
			reg.PendingWatch = true
			reg.WatchInflight = len(eligible)
			// Eagerly increment in-memory WatchAttempts so eligibility is updated
			// even if the disk state file write fails in runOneIntervention.
			for _, phase := range eligible {
				if ps := reg.PhaseStates[phase]; ps != nil {
					ps.WatchAttempts++
				}
			}
			s.logger.Info("watch: firing interventions",
				"plan", reg.PlanName, "count", len(eligible), "phases", eligible)
			go s.runWatchInterventions(reg.Ctx, reg, eligible)
		} else {
			s.releasePlanLocked(reg, "partial")
		}
	}
}

func (s *Scheduler) cancelPlanLocked(reg *PlanRegistration, reason string) {
	if reg.Cancel != nil {
		reg.Cancel()
	}
	s.logger.Warn("plan cancelled", "plan", reg.PlanName, "reason", reason)
	s.releasePlanLocked(reg, "failed")
}

func (s *Scheduler) releasePlanLocked(reg *PlanRegistration, status string) {
	_ = status // used for logging context

	// Release per-plan lock.
	pd := planDir(reg)
	orchestrator.ReleasePlanLock(pd)

	// Close PlanLogger.
	if reg.PlanLogger != nil {
		reg.PlanLogger.Close()
	}

	// Cancel context.
	if reg.Cancel != nil {
		reg.Cancel()
	}

	// Remove from all maps.
	delete(s.registrations, reg.PlanName)
	delete(s.running, reg.PlanName)
	delete(s.dirty, reg.PlanName)

	if s.OnStateChange != nil {
		s.OnStateChange()
	}

	// If draining and now idle, trigger shutdown.
	if s.draining && len(s.registrations) == 0 && s.ShutdownFn != nil {
		go s.ShutdownFn()
	}
}

// PersistState is called by the daemon during shutdown to save state.
func (s *Scheduler) PersistState() error {
	// State persistence is handled by OnStateChange callback.
	return nil
}

// planDir returns the plan directory for a registration.
func planDir(reg *PlanRegistration) string {
	return filepath.Join(reg.PlansDir, reg.PlanName)
}
