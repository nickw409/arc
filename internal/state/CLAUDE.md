# State

Persistence layer for Arc's runtime data. Owns all read/write access to `state.json` (per-phase) and `plan.json` (per-plan). Pure I/O package — no business logic.

## File Map

| File | Purpose |
|------|---------|
| `file.go` | `StateFile` — core I/O primitive. `Read`/`Write`/`Update` with in-process mutex. Uses atomic temp-file-then-rename writes. |
| `flock.go` | `FlockUpdate` — cross-process locking via POSIX `flock(2)`. Used by parallel pipeline agents. |
| `update.go` | High-level mutation API: `SetStatus`, `SetActivity`, `UpdateTests`, `IncrementIteration`, `AddTestFile`, `IncrementWatchAttempts`, `ResetToRetry`, `AppendAttemptLog`. |
| `plan.go` | `ReadPlan`/`WritePlan` for `plan.json`. `NextPhase`/`PhasesReady` for dependency-aware scheduling. |
| `history.go` | `AppendHistory` — best-effort append-only audit log at `{phaseDir}/history.md`. |

## Key Design Decisions

- **Two layers of concurrency protection**: `StateFile.mu` (sync.Mutex) for in-process goroutines; `FlockUpdate` (syscall.Flock) for cross-process parallel agents. Normal orchestrator code uses `StateFile`; the parallel pipeline uses `FlockUpdate`.
- **`FlockUpdate` uses a separate `.lock` file** — avoids races where one process truncates the state file before another finishes reading.
- **`UpdateTests` has embedded stuck-detection**: resets `stuck_iterations` only when tests actually improve (`passing > previous && previous > 0`). Going from 0→N doesn't count as progress.
- **`PhasesReady` iterates `meta.Phases` in declaration order** — the canonical execution order. Dependencies are stored as `map[string][]string` allowing non-linear DAGs.
- **History logging is best-effort** — errors are non-fatal, never blocks the pipeline.
