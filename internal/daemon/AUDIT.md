# Daemon Audit

**Scope:** `internal/daemon/` (all files), `internal/cli/daemon.go`

---

## Confirmed Bugs

### [CRITICAL] Data race in `Shutdown` — goroutine writes to `errs` after function returns

**File:** `daemon.go:147–168`

The background goroutine captures `errs` by closure reference. In the timeout path, `Shutdown` appends the timeout error to `errs` and then calls `errors.Join(errs...)` and returns — while the goroutine may still be running and attempting to lock `d.mu` and append to the same `errs` slice.

**Code path:**

```
Shutdown() {
    errs := []error{}
    go func() {
        // may still be running after Shutdown returns
        d.mu.Lock()
        errs = append(errs, ...)  // WRITE to errs
        d.mu.Unlock()
        close(done)
    }()
    select {
    case <-done:
    case <-time.After(10s):
        errs = append(errs, timeoutErr)  // timeout fires
    }
    return errors.Join(errs...)  // READ errs — goroutine may still be writing
}
```

The mutex protects `d.shutdown`, not `errs`. After the timeout case, both the main goroutine (`return errors.Join(errs...)`) and the background goroutine (trying to `d.mu.Lock(); errs = append(...)`) access `errs` without synchronization. This is a data race detectable with `-race`.

**Reproduction test sketch:**

```go
// Force PersistState to block long enough for the timeout to fire,
// then check the race detector output.
func TestShutdownDrainRace(t *testing.T) {
    d := &Daemon{
        sched: &slowSched{delay: 15 * time.Second},
    }
    // Shutdown will time out after 10s; the goroutine keeps running
    done := make(chan error, 1)
    go func() { done <- d.Shutdown() }()
    select {
    case err := <-done:
        // With -race, this will report a data race
        _ = err
    case <-time.After(12 * time.Second):
        t.Fatal("timeout")
    }
}
```

**Fix:** Return `PersistState`'s error through a channel instead of mutating a shared slice:

```go
persistErr := make(chan error, 1)
go func() { persistErr <- d.sched.PersistState() }()
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

### [HIGH] Worktree leak when `sched.Register` fails in `handleSubmit`

**File:** `handlers.go:97–155`

If `UseWorktree && !PerPhaseWorktree`, a shared worktree is created at line 99. If `sched.Register(reg)` subsequently fails (e.g., the plan is already registered), `cancel()` and `releaseLock()` are called but the worktree directory is never cleaned up.

**Code path:**

```
handleSubmit():
    wt = worktree.Create(...)     // worktree created, line 99
    reg := &PlanRegistration{Worktree: wt, ...}
    if err := sched.Register(reg); err != nil {
        cancel()
        releaseLock()
        return Response{OK: false, ...}  // wt abandoned
    }
```

**Fix:** Add cleanup on the Register error path:

```go
if err := sched.Register(reg); err != nil {
    cancel()
    releaseLock()
    if wt != nil {
        _ = wt.Remove()
    }
    return Response{OK: false, Error: fmt.Sprintf("registering plan: %v", err)}
}
```

---

## Potential Issues

### State persistence is entirely dead code

**Files:** `state.go`, `scheduler.go:391–394`, `cli/daemon.go`

`PersistState()` unconditionally returns nil. `OnStateChange` is never assigned in `newDaemonStartCmd`. As a result:

- `daemon-state.json` is never written.
- `LoadState`, `SaveState`, `ToPersisted`, `ToRegistration`, and `PersistedRegistration` are tested in isolation but never called by the running daemon.
- Restarting the daemon abandons all in-progress plans with no recovery.

This is not a bug today (no restart recovery is promised), but it is misleading: the code looks like persistence works, and the tests give false confidence. Either wire it up or remove the dead types until the feature is ready.

### No deadline on socket connections — goroutine leak risk

**File:** `protocol.go:41–48`

`ReadMessage` wraps `json.Decoder.Decode` with no read deadline. A client that connects and never sends data (or sends a partial JSON frame) holds the `HandleConnection` goroutine indefinitely. With enough hung clients this could exhaust goroutine memory.

**Code path:**

```
HandleConnection → ReadMessage → json.NewDecoder(conn).Decode(v)
                                 // blocks forever if client stops writing
```

Mitigation: set `conn.SetDeadline(time.Now().Add(30 * time.Second))` at the top of `HandleConnection`.

### `GlobalStatus` loses plan-to-phase association

**File:** `scheduler.go:139–150`

`buildPlanStatus` fills `resp.Phases` with `PhaseInfo` values that have a `Name` field but no plan name. `GlobalStatus` merges all plans' phases into a single flat slice. When multiple plans are running, callers cannot tell which phase belongs to which plan.

### Completion detection reads partially stale `PhaseStates`

**File:** `scheduler.go:292–295, 327–352`

In `handlePhaseResult`, only the completing phase's state is refreshed from disk. The allTerminal/allComplete check at lines 327–352 reads all `reg.PhaseStates`, some of which may be the initial values loaded at submit time. If a runner updates a phase's state file outside the scheduler's knowledge (e.g., a concurrent retry), the completion check could see stale status.

In practice this doesn't manifest because phases are only marked terminal when their runner returns and `handlePhaseResult` fires — which always loads the fresh state for that phase. But the logic is fragile: it relies on the invariant that only `handlePhaseResult` writes `reg.PhaseStates`, which is not enforced.

---

## Correct

**`daemon.go` — Flock + socket probe sequence:** Using `LOCK_EX|LOCK_NB` as the first step before the socket probe is correct. The flock eliminates the race between two concurrent daemon starts (the flock is the single-writer gate; the socket probe is a belt-and-suspenders check for an already-live daemon). Stale socket cleanup (probe → connection refused → remove) is also correct.

**`daemon.go:Shutdown` — cleanup ordering:** Closing the listener and removing the socket/PID files *before* the drain wait is correct. This ensures the socket is unavailable to new clients immediately on shutdown, preventing new work from arriving during the drain. Even if the process is killed mid-drain, the socket is already gone.

**`daemon.go:Shutdown` — idempotency guard:** The `d.shutdown` bool under `d.mu` prevents double-shutdown from racing goroutines.

**`state.go:SaveState` — atomic write:** Write-to-`.tmp` then `os.Rename` prevents partial state files on crash.

**`scheduler.go` — semaphore pattern:** `slots chan struct{}` is idiomatic. Goroutines that fail to acquire a slot return immediately rather than blocking the dispatch loop. The slot is always released via `defer func() { <-s.slots }()` in `runWork`, even if the runner panics.

**`scheduler.go` — dependency separation:** The scheduler asks `state.PhasesReady` what's runnable rather than implementing dependency logic itself. Clean separation.

**`handlers.go:handleSubmit` — lock release on all error paths:** Every early-return path after acquiring the plan lock calls `releaseLock()` before returning (with the exception of the worktree leak noted above).

**`client.go:IsRunning` — probe-only check:** Uses a real dial rather than just stat-ing the socket file, which correctly handles the stale-socket case.

**Test coverage breadth:** scheduler_test.go covers deps, parallelism, semaphore limit, stop-on-failure, cancel, drain, duplicate registration, budget — all without mocks or external test frameworks.

---

## Files Reviewed

| File | Status |
|------|--------|
| `daemon.go` | Data race in `Shutdown` (confirmed), otherwise correct |
| `scheduler.go` | Worktree leak path; stale PhaseStates; GlobalStatus issue; status param discarded |
| `handlers.go` | Worktree leak on Register failure |
| `bridge.go` | No issues found |
| `client.go` | No issues found |
| `protocol.go` | No deadline on reads |
| `types.go` | No issues found |
| `state.go` | Dead code (persistence never wired); implementation itself is correct |
| `cli/daemon.go` | OnStateChange never set; drain has no wait |
