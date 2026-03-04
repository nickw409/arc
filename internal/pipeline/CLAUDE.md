# Pipeline

Executes a single workflow state-machine iteration. Sits between the orchestrator (which manages the phase loop) and the agent layer (which spawns the `claude` subprocess).

**Start here:** `iterate.go` — the primary entry point. `RunState()` is called by the orchestrator on each loop iteration.

## File Map

| File | Purpose |
|------|---------|
| `iterate.go` | Core entry point. Reads state, loads workflow, checks constraints, spawns agent, extracts verdict, updates state. Also has `MapStateToStatus` (strips namespace prefix from dotted state names like `"check.adversary"` → `"adversary"`). |
| `parallel.go` | Concurrent agent execution for parallel workflow states. Creates one goroutine per `ParallelBranch`, writes per-branch logs. |
| `join.go` | Pure verdict computation for parallel results. Maps strategies (`all`/`any`/`n_of_m`) to verdict strings. No I/O. |
| `constraints.go` | Pre/post execution constraint checks (max iterations, required artifacts). Path traversal defense is double-layered. |
| `escalation.go` | Escalation rule matching — pure logic. Called by the **orchestrator's outer loop**, not by `RunState`. |
| `intervention.go` | Intervention trigger evaluation — pure logic. Also called by the orchestrator, not `RunState`. |
| `hooks.go` | Post-state hook execution. Filters hooks by verdict using a mini-DSL (`!`, `\|` operators). |
| `actions.go` | Named action implementations invoked by hooks: `run_tests`, `commit`, `switch_model`, `analyze_stuck`, `request_human`, `script`. |
| `memory.go` | Loopback memory: persists agent notes across visits to the same state via `{phaseDir}/memory/{stateName}.md`. |

## Key Design Decisions

- **`run_once` checks `VerdictsHistory`**, not just `StateIterations`. This handles interrupted runs — if killed mid-execution before state.json was updated, the state re-runs rather than being erroneously skipped.
- **Branching vs linear states** are distinguished by `len(validVerdicts) == 0`. Linear states treat non-zero exit as hard retry; branching states attempt verdict extraction even on failure (agent may have emitted verdict before hitting max_turns).
- **`switch_model` is in-memory only** — mutates `State.ModelOverride` but doesn't persist. The next `RunState` call re-reads from disk, so it's ephemeral unless something else writes state.
- **`escalation.go` and `intervention.go` are NOT called by `RunState`** — they're evaluation utilities for the orchestrator's outer loop between state runs.
- **`agentCommandName`** is a package-level var with `SetAgentCommandNameForTest` so tests can point at a mock binary.

## Call Graph

```
orchestrator/phase.go
    └── RunState()                    ← entry point
            ├── constraints.go    CheckPreConstraints / CheckPostConstraints
            ├── memory.go         ReadMemory (before) / ExtractMemory+WriteMemory (after)
            ├── hooks.go          RunAfterHooks
            │       └── actions.go    RunAction
            └── parallel.go       RunParallel (when stateConfig.Parallel != nil)
                    └── join.go       JoinParallel

orchestrator/phase.go (outer loop, not RunState):
    ├── escalation.go    CheckEscalation
    └── intervention.go  CheckIntervention
```
