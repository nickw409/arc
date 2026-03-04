# Orchestrator

Top-level execution engine that drives an entire plan run. Manages phase scheduling, worktrees, and the state-machine loop for each phase.

**Start here:** `orchestrator.go` for the plan-level loop, `phase.go` for per-phase state machine execution.

## File Map

| File | Purpose |
|------|---------|
| `orchestrator.go` | `Launch()` — the top-level entry point. Acquires PID lock, creates shared worktree, schedules phases by dependency, runs them concurrently, merges worktree on completion. |
| `phase.go` | `RunPhase()` — runs a single phase through all states to terminal. Contains `postIterationActions` (test running, adversary tracking, commits) and `handleDispute` (AI-judged test disputes). |
| `judge.go` | `JudgeDispute()` — spawns a minimal 1-turn, 0-tools Claude agent to determine if a test or the implementation is wrong. |
| `direct.go` | `runDirectPlanLoop()` — single-session execution for "direct" workflow. All phases run in one Claude invocation (400 turns, 2hr timeout). |
| `report.go` | `generateCompletionReport()` — writes `COMPLETION_REPORT.md` with per-phase status, iterations, tests, costs. |

## Key Design Decisions

- **`Launch` branches on workflow type**: `"direct"` delegates to `runDirectPlanLoop` (single agent session); all others use the concurrent phase scheduling loop.
- **Phase scheduling**: `state.PhasesReady()` finds all phases whose dependencies are satisfied, then launches them all as concurrent goroutines. `StopOnFailure` cancels sibling phases via a shared `batchCtx`.
- **Lock uses signal-0 probing** (`syscall.Signal(0)`) to detect stale locks from crashed runs.
- **Double terminal-state check** in `RunPhase`: handles the case where an agent updates state.json to terminal but then crashes/returns non-zero — without this, the phase would be incorrectly blocked.
- **`discoverNewTestFiles`** only scans top-level `workDir` (not recursive), looking for `_test.go` suffix — hardcoded to Go conventions.
- **`adversarialPostActions`**: on `bugs_found`, increments `AdversaryRound`, discovers new test files, stores them under `AdversaryTests["round_N"]`.

## Call Graph

```
Launch (orchestrator.go)
  ├── acquireLock / releaseLock
  ├── worktree.Create / MergeBack
  ├── state.PhasesReady (scheduling)
  ├── runDirectPlanLoop (direct.go)     ← "direct" workflow only
  └── RunPhase (phase.go)              ← all other workflows (concurrent per phase)
        ├── pipeline.RunState           ← one state iteration
        ├── handleDispute → JudgeDispute (judge.go)
        ├── runner.RunAll               ← test execution
        ├── gitops.Commit               ← post-phase commit
        └── postIterationActions / adversarialPostActions
  └── generateCompletionReport (report.go)
```
