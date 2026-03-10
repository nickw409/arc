package daemon

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nwiley/arc/internal/arc"
)

// makeReg creates a minimal PlanRegistration for testing.
func makeReg(name string, phases []string, deps map[string][]string) *PlanRegistration {
	meta := arc.NewPlanMeta(name, "feature", phases)
	if deps != nil {
		meta.Dependencies = deps
	}
	ctx, cancel := context.WithCancel(context.Background())
	phaseStates := make(map[string]*arc.PhaseState, len(phases))
	for _, p := range phases {
		phaseStates[p] = arc.NewPhaseState(name, p, "feature")
	}
	return &PlanRegistration{
		PlanName:    name,
		Meta:        meta,
		PhaseStates: phaseStates,
		SubmittedAt: time.Now(),
		Ctx:         ctx,
		Cancel:      cancel,
	}
}

func TestSchedulerRunsPhases(t *testing.T) {
	var ran sync.Map
	runner := func(ctx context.Context, reg *PlanRegistration, phase string) error {
		ran.Store(reg.PlanName+"/"+phase, true)
		// Simulate phase completion by marking state complete.
		reg.PhaseStates[phase].PhaseStatus = "complete"
		return nil
	}

	finalized := make(chan string, 1)
	finalizer := func(reg *PlanRegistration) {
		finalized <- reg.PlanName
	}

	s := NewScheduler(2, runner, finalizer)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go s.Run(ctx)

	reg := makeReg("plan-a", []string{"impl"}, nil)
	if err := s.Register(reg); err != nil {
		t.Fatal(err)
	}

	// Wait for finalize.
	select {
	case name := <-finalized:
		if name != "plan-a" {
			t.Fatalf("expected plan-a, got %s", name)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for finalize")
	}

	if _, ok := ran.Load("plan-a/impl"); !ok {
		t.Fatal("phase impl was not run")
	}
}

func TestSchedulerDependencyOrder(t *testing.T) {
	var order []string
	var mu sync.Mutex

	runner := func(ctx context.Context, reg *PlanRegistration, phase string) error {
		mu.Lock()
		order = append(order, phase)
		mu.Unlock()
		reg.PhaseStates[phase].PhaseStatus = "complete"
		return nil
	}

	finalized := make(chan string, 1)
	finalizer := func(reg *PlanRegistration) {
		finalized <- reg.PlanName
	}

	// maxParallel=1 to enforce serial execution.
	s := NewScheduler(1, runner, finalizer)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go s.Run(ctx)

	reg := makeReg("dep-plan", []string{"a", "b"}, map[string][]string{
		"b": {"a"},
	})
	if err := s.Register(reg); err != nil {
		t.Fatal(err)
	}

	select {
	case <-finalized:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for finalize")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(order) != 2 || order[0] != "a" || order[1] != "b" {
		t.Fatalf("expected [a, b], got %v", order)
	}
}

func TestSchedulerParallelPhases(t *testing.T) {
	// Two independent phases should run in parallel with maxParallel=2.
	var concurrent int32
	var maxConcurrent int32

	runner := func(ctx context.Context, reg *PlanRegistration, phase string) error {
		cur := atomic.AddInt32(&concurrent, 1)
		for {
			old := atomic.LoadInt32(&maxConcurrent)
			if cur <= old || atomic.CompareAndSwapInt32(&maxConcurrent, old, cur) {
				break
			}
		}
		// Hold the slot briefly so both phases overlap.
		time.Sleep(50 * time.Millisecond)
		atomic.AddInt32(&concurrent, -1)
		reg.PhaseStates[phase].PhaseStatus = "complete"
		return nil
	}

	finalized := make(chan string, 1)
	finalizer := func(reg *PlanRegistration) {
		finalized <- reg.PlanName
	}

	s := NewScheduler(2, runner, finalizer)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go s.Run(ctx)

	// Two phases with no dependencies.
	reg := makeReg("par", []string{"x", "y"}, map[string][]string{})
	if err := s.Register(reg); err != nil {
		t.Fatal(err)
	}

	select {
	case <-finalized:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out")
	}

	if atomic.LoadInt32(&maxConcurrent) < 2 {
		t.Fatalf("expected concurrency >= 2, got %d", maxConcurrent)
	}
}

func TestSchedulerStopOnFailure(t *testing.T) {
	runner := func(ctx context.Context, reg *PlanRegistration, phase string) error {
		if phase == "a" {
			reg.PhaseStates[phase].PhaseStatus = "blocked"
			return fmt.Errorf("phase a failed")
		}
		reg.PhaseStates[phase].PhaseStatus = "complete"
		return nil
	}

	s := NewScheduler(1, runner, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go s.Run(ctx)

	reg := makeReg("stop-plan", []string{"a", "b"}, map[string][]string{
		"b": {"a"},
	})
	reg.StopOnFailure = true
	if err := s.Register(reg); err != nil {
		t.Fatal(err)
	}

	// Wait for the plan to be released (removed from registrations).
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for plan release")
		default:
		}
		regs := s.Registrations()
		if len(regs) == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestSchedulerDrain(t *testing.T) {
	runner := func(ctx context.Context, reg *PlanRegistration, phase string) error {
		reg.PhaseStates[phase].PhaseStatus = "complete"
		return nil
	}

	s := NewScheduler(2, runner, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go s.Run(ctx)

	s.Drain()

	reg := makeReg("drain-plan", []string{"a"}, nil)
	err := s.Register(reg)
	if err == nil {
		t.Fatal("expected error when registering during drain")
	}
}

func TestSchedulerCancel(t *testing.T) {
	started := make(chan struct{})
	runner := func(ctx context.Context, reg *PlanRegistration, phase string) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}

	s := NewScheduler(2, runner, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go s.Run(ctx)

	reg := makeReg("cancel-plan", []string{"a"}, nil)
	if err := s.Register(reg); err != nil {
		t.Fatal(err)
	}

	// Wait for phase to start.
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for phase start")
	}

	if err := s.Cancel("cancel-plan"); err != nil {
		t.Fatal(err)
	}

	// Wait for the plan to be released.
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for plan release after cancel")
		default:
		}
		if len(s.Registrations()) == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestSchedulerCancelNotFound(t *testing.T) {
	s := NewScheduler(1, nil, nil)
	err := s.Cancel("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent plan")
	}
}

func TestSchedulerDuplicateRegister(t *testing.T) {
	s := NewScheduler(1, nil, nil)
	reg := makeReg("dup", []string{"a"}, nil)
	if err := s.Register(reg); err != nil {
		t.Fatal(err)
	}
	reg2 := makeReg("dup", []string{"a"}, nil)
	err := s.Register(reg2)
	if err == nil {
		t.Fatal("expected error for duplicate registration")
	}
}

func TestSchedulerStatus(t *testing.T) {
	s := NewScheduler(1, nil, nil)
	reg := makeReg("status-plan", []string{"a", "b"}, nil)
	if err := s.Register(reg); err != nil {
		t.Fatal(err)
	}

	resp := s.Status("status-plan")
	if !resp.OK {
		t.Fatalf("expected OK, got error: %s", resp.Error)
	}
	if resp.PlanName != "status-plan" {
		t.Fatalf("expected plan name status-plan, got %s", resp.PlanName)
	}
	if len(resp.Phases) != 2 {
		t.Fatalf("expected 2 phases, got %d", len(resp.Phases))
	}
}

func TestSchedulerStatusNotFound(t *testing.T) {
	s := NewScheduler(1, nil, nil)
	resp := s.Status("nope")
	if resp.OK {
		t.Fatal("expected not OK for nonexistent plan")
	}
}

func TestSchedulerGlobalStatus(t *testing.T) {
	s := NewScheduler(1, nil, nil)
	reg1 := makeReg("p1", []string{"a"}, nil)
	reg2 := makeReg("p2", []string{"b", "c"}, nil)
	s.Register(reg1)
	s.Register(reg2)

	resp := s.GlobalStatus()
	if !resp.OK {
		t.Fatal("expected OK")
	}
	if len(resp.Phases) != 3 {
		t.Fatalf("expected 3 phases total, got %d", len(resp.Phases))
	}
}

func TestSchedulerOnStateChange(t *testing.T) {
	var calls int32
	s := NewScheduler(1, func(ctx context.Context, reg *PlanRegistration, phase string) error {
		reg.PhaseStates[phase].PhaseStatus = "complete"
		return nil
	}, nil)
	s.OnStateChange = func() {
		atomic.AddInt32(&calls, 1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)

	reg := makeReg("sc-plan", []string{"a"}, nil)
	if err := s.Register(reg); err != nil {
		t.Fatal(err)
	}

	// Wait for plan to be released (no finalizer means releasePlan called directly).
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out")
		default:
		}
		if len(s.Registrations()) == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// OnStateChange called at least on Register and releasePlan.
	if c := atomic.LoadInt32(&calls); c < 2 {
		t.Fatalf("expected at least 2 OnStateChange calls, got %d", c)
	}
}

func TestSchedulerMultiplePlans(t *testing.T) {
	var ran sync.Map
	runner := func(ctx context.Context, reg *PlanRegistration, phase string) error {
		ran.Store(reg.PlanName+"/"+phase, true)
		reg.PhaseStates[phase].PhaseStatus = "complete"
		return nil
	}

	finalized := make(chan string, 2)
	finalizer := func(reg *PlanRegistration) {
		finalized <- reg.PlanName
	}

	s := NewScheduler(4, runner, finalizer)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go s.Run(ctx)

	reg1 := makeReg("multi-1", []string{"x"}, nil)
	reg2 := makeReg("multi-2", []string{"y"}, nil)
	s.Register(reg1)
	s.Register(reg2)

	seen := map[string]bool{}
	for i := 0; i < 2; i++ {
		select {
		case name := <-finalized:
			seen[name] = true
		case <-time.After(5 * time.Second):
			t.Fatal("timed out")
		}
	}

	if !seen["multi-1"] || !seen["multi-2"] {
		t.Fatalf("expected both plans finalized, got %v", seen)
	}
}

func TestSchedulerPartialCompletion(t *testing.T) {
	runner := func(ctx context.Context, reg *PlanRegistration, phase string) error {
		if phase == "a" {
			reg.PhaseStates[phase].PhaseStatus = "complete"
		} else {
			reg.PhaseStates[phase].PhaseStatus = "blocked"
		}
		return nil
	}

	s := NewScheduler(2, runner, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go s.Run(ctx)

	// Two independent phases — one completes, one blocks.
	reg := makeReg("partial", []string{"a", "b"}, map[string][]string{})
	if err := s.Register(reg); err != nil {
		t.Fatal(err)
	}

	// Plan should be released as "partial" (no finalizer called).
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for partial plan release")
		default:
		}
		if len(s.Registrations()) == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestSchedulerSemaphoreLimit(t *testing.T) {
	// With maxParallel=1, phases should not overlap.
	var concurrent int32
	var maxConcurrent int32

	runner := func(ctx context.Context, reg *PlanRegistration, phase string) error {
		cur := atomic.AddInt32(&concurrent, 1)
		for {
			old := atomic.LoadInt32(&maxConcurrent)
			if cur <= old || atomic.CompareAndSwapInt32(&maxConcurrent, old, cur) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt32(&concurrent, -1)
		reg.PhaseStates[phase].PhaseStatus = "complete"
		return nil
	}

	finalized := make(chan string, 1)
	finalizer := func(reg *PlanRegistration) {
		finalized <- reg.PlanName
	}

	s := NewScheduler(1, runner, finalizer)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go s.Run(ctx)

	reg := makeReg("serial", []string{"a", "b"}, map[string][]string{})
	s.Register(reg)

	select {
	case <-finalized:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out")
	}

	if mc := atomic.LoadInt32(&maxConcurrent); mc > 1 {
		t.Fatalf("expected max concurrency 1, got %d", mc)
	}
}

func TestSchedulerRegistrations(t *testing.T) {
	s := NewScheduler(1, nil, nil)
	if len(s.Registrations()) != 0 {
		t.Fatal("expected empty registrations")
	}

	reg := makeReg("reg-test", []string{"a"}, nil)
	s.Register(reg)

	regs := s.Registrations()
	if len(regs) != 1 || regs[0].PlanName != "reg-test" {
		t.Fatalf("unexpected registrations: %v", regs)
	}
}

func TestRateLimitSignalReducesSlots(t *testing.T) {
	s := NewScheduler(3, nil, nil)

	// Pre-fill all slots so we can check a drain occurred.
	// First put tokens into the channel to simulate idle state.
	s.slots <- struct{}{}
	s.slots <- struct{}{}
	s.slots <- struct{}{}

	// Drain one slot to match what's "in use" — leave 2 free.
	<-s.slots

	// slots now has 2 tokens (1 in use).
	if len(s.slots) != 2 {
		t.Fatalf("expected 2 slots before signal, got %d", len(s.slots))
	}

	s.RateLimitSignal()

	s.mu.Lock()
	throttled := s.throttleSlots
	s.mu.Unlock()

	if throttled != 1 {
		t.Fatalf("expected throttleSlots=1 after signal, got %d", throttled)
	}
	if len(s.slots) != 1 {
		t.Fatalf("expected 1 slot after signal, got %d", len(s.slots))
	}

	// Stop the timer to avoid goroutine leak.
	s.mu.Lock()
	if s.throttleTimer != nil {
		s.throttleTimer.Stop()
	}
	s.mu.Unlock()
}

func TestRateLimitSignalCap(t *testing.T) {
	// maxParallel=3: cap is maxParallel-1 = 2.
	s := NewScheduler(3, nil, nil)

	// Fill all 3 slots (simulate idle).
	s.slots <- struct{}{}
	s.slots <- struct{}{}
	s.slots <- struct{}{}

	s.RateLimitSignal() // throttleSlots=1
	s.RateLimitSignal() // throttleSlots=2

	s.mu.Lock()
	throttled := s.throttleSlots
	s.mu.Unlock()

	if throttled != 2 {
		t.Fatalf("expected throttleSlots=2 at cap, got %d", throttled)
	}

	// Third call should be a noop (cap reached).
	s.RateLimitSignal()

	s.mu.Lock()
	throttledAfter := s.throttleSlots
	s.mu.Unlock()

	if throttledAfter != 2 {
		t.Fatalf("expected throttleSlots to remain 2, got %d", throttledAfter)
	}

	// Stop timer.
	s.mu.Lock()
	if s.throttleTimer != nil {
		s.throttleTimer.Stop()
	}
	s.mu.Unlock()
}

func TestRestoreSlotReturnsCapacity(t *testing.T) {
	s := NewScheduler(3, nil, nil)

	// Fill all 3 slots.
	s.slots <- struct{}{}
	s.slots <- struct{}{}
	s.slots <- struct{}{}

	// Drain one via RateLimitSignal.
	s.RateLimitSignal()

	s.mu.Lock()
	if s.throttleSlots != 1 {
		s.mu.Unlock()
		t.Fatalf("expected throttleSlots=1, got %d", s.throttleSlots)
	}
	if s.throttleTimer != nil {
		s.throttleTimer.Stop()
	}
	s.mu.Unlock()

	slotsBefore := len(s.slots)

	// Manually call restoreSlot.
	s.restoreSlot()

	s.mu.Lock()
	throttled := s.throttleSlots
	s.mu.Unlock()

	if throttled != 0 {
		t.Fatalf("expected throttleSlots=0 after restore, got %d", throttled)
	}
	if len(s.slots) != slotsBefore+1 {
		t.Fatalf("expected %d slots after restore, got %d", slotsBefore+1, len(s.slots))
	}
}

func TestRateLimitSignalNoFreeSlotsIsNoop(t *testing.T) {
	s := NewScheduler(2, nil, nil)

	// Do NOT add any tokens — all slots are "in use" by running goroutines.
	if len(s.slots) != 0 {
		t.Fatalf("expected empty slots channel, got %d", len(s.slots))
	}

	s.RateLimitSignal()

	s.mu.Lock()
	throttled := s.throttleSlots
	s.mu.Unlock()

	// No free slot to drain; should remain 0.
	if throttled != 0 {
		t.Fatalf("expected throttleSlots=0 (noop), got %d", throttled)
	}
	if len(s.slots) != 0 {
		t.Fatalf("expected 0 slots (noop), got %d", len(s.slots))
	}
}

func TestListPlansSorted(t *testing.T) {
	s := NewScheduler(2, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Register in unsorted order.
	for _, name := range []string{"plan-z", "plan-a", "plan-m"} {
		reg := makeReg(name, []string{"impl"}, nil)
		s.registrations[name] = reg
		s.running[name] = nil
	}

	plans := s.ListPlans()

	if len(plans) != 3 {
		t.Fatalf("expected 3 plans, got %d", len(plans))
	}
	if plans[0].PlanName != "plan-a" {
		t.Errorf("plans[0]: got %q, want plan-a", plans[0].PlanName)
	}
	if plans[1].PlanName != "plan-m" {
		t.Errorf("plans[1]: got %q, want plan-m", plans[1].PlanName)
	}
	if plans[2].PlanName != "plan-z" {
		t.Errorf("plans[2]: got %q, want plan-z", plans[2].PlanName)
	}
	_ = ctx
}

func TestBuildPlanStatusBlockedReason(t *testing.T) {
	s := NewScheduler(2, nil, nil)

	reg := makeReg("test-plan", []string{"impl"}, nil)
	reg.PhaseStates["impl"].PhaseStatus = "blocked"
	reg.PhaseStates["impl"].BlockedReason = "gate failed"
	s.registrations["test-plan"] = reg
	s.running["test-plan"] = nil

	s.mu.Lock()
	resp := s.buildPlanStatus(reg)
	s.mu.Unlock()

	if len(resp.Phases) != 1 {
		t.Fatalf("expected 1 phase, got %d", len(resp.Phases))
	}
	if resp.Phases[0].BlockedReason != "gate failed" {
		t.Errorf("BlockedReason: got %q, want %q", resp.Phases[0].BlockedReason, "gate failed")
	}
}

func TestBuildPlanStatusNonBlockedNoReason(t *testing.T) {
	s := NewScheduler(2, nil, nil)

	reg := makeReg("test-plan", []string{"impl"}, nil)
	reg.PhaseStates["impl"].PhaseStatus = "running"
	s.registrations["test-plan"] = reg
	s.running["test-plan"] = nil

	s.mu.Lock()
	resp := s.buildPlanStatus(reg)
	s.mu.Unlock()

	if len(resp.Phases) != 1 {
		t.Fatalf("expected 1 phase, got %d", len(resp.Phases))
	}
	if resp.Phases[0].BlockedReason != "" {
		t.Errorf("expected empty BlockedReason for non-blocked phase, got %q", resp.Phases[0].BlockedReason)
	}
}
