# Orchestrator

Top-level execution engine that drives an entire plan run. Manages phase scheduling, worktrees, and the gate-based loop for each phase.

**Start here:** `orchestrator.go` for the plan-level entry point, `gated.go` for per-phase execution.

## File Map

| File | Purpose |
|------|---------|
| `orchestrator.go` | `Launch()` — the top-level entry point. Acquires PID lock, creates worktree, calls `LaunchGated()`. |
| `launch_gated.go` | `LaunchGated()` — phase scheduling, dependency ordering, worktrees, regression suite. |
| `gated.go` | `RunPhaseGated()` — per-phase session→gate→retry loop. `MaxGatedAttempts=2`. |
| `phase_types.go` | `RunPhaseOptions`, `commitPhase`, `discoverNewTestFiles`. |
| `classify.go` | `classifyGateFailure()`, `classifySpawnError()` — error tier classification (Transient/Feedback/GiveUp). |
| `adversary.go` | Post-plan adversarial test session. |
| `commitment_audit.go` | `CommitmentAudit` — verifies agent committed promised changes. |
| `observe.go` | `LoadPhaseState()` and state observation helpers. |
| `report.go` | `generateCompletionReport()` — writes `COMPLETION_REPORT.md` with per-phase status, iterations, tests, costs. |

## Key Design Decisions

- **`MaxGatedAttempts=2`**: each phase gets at most 2 gated attempts before being marked failed.
- **Error tiers**: `classifyGateFailure` buckets failures into Transient (retry immediately), Feedback (retry with gate output), or GiveUp (abort phase).
- **Gate assertion routing**: after each agent session, gate assertions are evaluated; result determines retry vs. complete vs. escalate.
- **Phase scheduling**: `LaunchGated` finds all phases whose dependencies are satisfied, then launches them as concurrent goroutines. `StopOnFailure` cancels sibling phases via a shared context.
- **Lock uses signal-0 probing** (`syscall.Signal(0)`) to detect stale locks from crashed runs.
- **`discoverNewTestFiles`** only scans top-level `workDir` (not recursive), looking for `_test.go` suffix — hardcoded to Go conventions.

## Call Graph

```
Launch (orchestrator.go)
  ├── acquireLock / releaseLock
  ├── worktree.Create / MergeBack
  └── LaunchGated (launch_gated.go)
        └── RunPhaseGated (gated.go)     ← concurrent per phase
              ├── classifyGateFailure / classifySpawnError (classify.go)
              ├── CommitmentAudit (commitment_audit.go)
              ├── gitops.Commit
              └── commitPhase / discoverNewTestFiles (phase_types.go)
  └── generateCompletionReport (report.go)
```
