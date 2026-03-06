# Daemon Audit — Code Review Findings

**Scope:** `internal/daemon/` (all files), `internal/cli/daemon.go`

---

## 1. Critical Issues

### 1.1 Data race in `Shutdown` — goroutine writes to `errs` after function returns

**File:** `internal/daemon/daemon.go:147–168`

The background goroutine captures `errs` by reference via closure. In the timeout path, `Shutdown` appends to `errs` and then calls `errors.Join(errs...)` and returns, while the goroutine may still be running and attempting to append to the same `errs` slice. This is a data race: the goroutine calls `d.mu.Lock(); errs = append(errs, ...)` concurrently with the caller's read of `errs`.

```go
// Current (racy):
go func() {
    if err := d.sched.PersistState(); err != nil {
        d.mu.Lock()
        errs = append(errs, ...)  // may race with caller reading errs
        d.mu.Unlock()
    }
    close(done)
}()
select {
case <-done:
case <-time.After(10 * time.Second):
    errs = append(errs, ...)  // timeout: caller moves on, goroutine still running
}
return errors.Join(errs...)
```

**Fix:** Have the goroutine return its error through a channel rather than appending to a shared slice.

```go
persistErr := make(chan error, 1)
go func() {
    persistErr <- d.sched.PersistState()
}()
select {
case err := <-persistErr:
    if err != nil {
        errs = append(errs, fmt.Errorf("persisting state: %w", err))
    }
case <-time.After(10 * time.Second):
    errs = append(errs, fmt.Errorf("timed out waiting for scheduler to persist"))
}
```

---

## 2. Important Issues

### 2.1 State persistence is entirely dead code — in-progress plans are never saved or restored

**Files:** `internal/daemon/state.go`, `internal/daemon/scheduler.go:391–394`, `internal/cli/daemon.go`

`PersistState()` is a no-op (returns nil immediately). `OnStateChange` is never assigned in `newDaemonStartCmd`. As a result:

- `daemon-state.json` is never written in production.
- `LoadState`, `SaveState`, `ToPersisted`, `ToRegistration`, and `PersistedRegistration` are tested in isolation but are never called by the daemon process itself.
- Killing and restarting the daemon abandons all in-progress plans with no recovery path.

Either wire up `OnStateChange` to call `SaveState`, and load state at startup, or remove the persistence types and tests until the feature is ready.

### 2.2 Worktree leak when `sched.Register` fails

**File:** `internal/daemon/handlers.go:97–155`

If a shared worktree is created (lines 98–105) but `sched.Register(reg)` subsequently fails (e.g., duplicate plan), the worktree directory is abandoned. `cancel()` is called but `worktree.Remove()` / cleanup is not.

```go
// Lines 97-155 — created wt is leaked if Register fails at line 151
if req.UseWorktree && !req.PerPhaseWorktree {
    created, wtErr := worktree.Create(...)
    wt = created
}
// ...
if err := sched.Register(reg); err != nil {
    cancel()
    releaseLock()
    // wt is never cleaned up here
    return Response{OK: false, Error: ...}
}
```

**Fix:** Add worktree cleanup on the `sched.Register` error path.

### 2.3 No read/write deadline on socket connections

**File:** `internal/daemon/protocol.go:41–48`

`ReadMessage` and `WriteMessage` use bare `json.NewDecoder(conn).Decode` / `json.NewEncoder(conn).Encode` with no deadline. A misbehaving client that connects and then sends no data (or a hung client) will hold the `HandleConnection` goroutine indefinitely — one goroutine per connection, no bound.

**Fix:** Call `conn.SetDeadline(time.Now().Add(someTimeout))` before reading in `HandleConnection`, or set a read deadline inside `ReadMessage`.

### 2.4 `GlobalStatus` loses plan-to-phase association

**File:** `internal/daemon/scheduler.go:139–150`

`GlobalStatus` merges phases from all registered plans into a flat `resp.Phases` slice. Each `PhaseInfo` has a `Name` field but no plan name. Callers cannot tell which plan a phase belongs to when multiple plans are running.

**Fix:** Add a `PlanName` field to `PhaseInfo` and populate it in `buildPlanStatus`, or return a `[]PlanStatus` instead of a flat phase slice.

### 2.5 `handlePhaseResult` completion detection uses partially stale `PhaseStates`

**File:** `internal/daemon/scheduler.go:326–352`

When a phase completes, only that phase's state is refreshed from disk (line 292–295). The completion check then iterates `reg.Meta.Phases` and reads all cached `reg.PhaseStates`. For parallel plans, other phases' states may have been written to disk by their respective runners but not yet loaded (their `handlePhaseResult` hasn't run yet). This can cause a plan to be incorrectly classified as "partial" and released prematurely.

In practice this is unlikely because completion requires all phases to be in a terminal state, and a phase can only be terminal if its runner returned and `handlePhaseResult` ran. But the logic reads phase statuses from cache rather than from disk, so stale values are possible if the runner mutates the state file without going through the scheduler's refresh.

**Fix:** In `handlePhaseResult`, reload all phase states from disk before the completion check, not just the completing phase.

---

## 3. Minor Issues

### 3.1 `releasePlanLocked` silently discards `status` parameter

**File:** `internal/daemon/scheduler.go:363–364`

```go
func (s *Scheduler) releasePlanLocked(reg *PlanRegistration, status string) {
    _ = status // used for logging context
```

The parameter is accepted but immediately discarded. All release events (complete, partial, failed) are logged identically (or not at all). Either log the status or remove the parameter.

### 3.2 `EnsureRunning` has a TOCTOU race between `IsRunning` check and subprocess start

**File:** `internal/daemon/daemon.go:276–313`

Two concurrent callers can both observe `IsRunning == false` and both spawn a daemon subprocess. The second spawned daemon will fail to acquire the flock and exit, but the caller of `EnsureRunning` won't observe this failure — it polls the socket which will eventually become ready from the first daemon. The extra process silently exits. Not a correctness issue, but the error from the duplicate start is swallowed.

### 3.3 `arc daemon stop` has no way to confirm the daemon actually stopped

**File:** `internal/cli/daemon.go:82–110`

The stop command sends `drain` and prints "will stop after current work completes" without waiting. There is no polling or confirmation that the process has exited. For scripting purposes, this makes it hard to sequence a stop-then-restart.

### 3.4 `unmarshalDaemonConfig` is a package-internal wrapper with a misleading comment

**File:** `internal/daemon/daemon.go:269–273`, `daemon_test.go:346–351`

`unmarshalDaemonConfig` exists solely to let the test call `yaml.Unmarshal` without importing yaml.v3 directly in the test file. The comment in the test (`yamlUnmarshalForTest`) is confusing. Since the test is already in `package daemon`, it could import yaml.v3 directly. Consider removing the indirection.

---

## 4. Positive Observations

- **Flock-based double-start prevention** (`daemon.go:60–63`) is the right mechanism: `LOCK_EX|LOCK_NB` gives a race-free single-writer guarantee, and the PID-file + socket probe combo handles the stale-socket case cleanly.

- **Atomic state saves** (`state.go:49–63`) via write-to-.tmp then rename is correct and prevents partial reads on crash.

- **Semaphore-based concurrency limiting** in the scheduler (`slots chan struct{}`) is idiomatic and avoids goroutine proliferation.

- **Dependency ordering via `state.PhasesReady`** is cleanly separated from the scheduler: the scheduler just asks "what's ready?" and trusts the state package to evaluate deps. Good separation of concerns.

- **Test coverage is broad**: all major scheduler behaviors (deps, parallelism, stop-on-failure, cancel, drain, budget) have unit tests with no external test framework.

- **`Shutdown` idempotency** guard (`d.shutdown` flag under mutex) is correct and tested.
