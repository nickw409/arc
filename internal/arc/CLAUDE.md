# Arc (Core Types)

Pure types package — no I/O, no external dependencies beyond stdlib and `gopkg.in/yaml.v3`. Imported by nearly every other `internal/` package as the shared vocabulary.

## File Map

| File | Purpose |
|------|---------|
| `state.go` | `PhaseState` (per-phase runtime state serialized to `state.json`) and `PlanMeta` (plan-level metadata in `plan.json`). The largest file. |
| `spec.go` | `PhaseSpec` type — the structured plan spec parsed from plan.md. |
| `gate.go` | `GateResult`, `GateAssertion` types — gate evaluation results. |
| `adapter.go` | `AdapterConfig`, `AdapterSpec` types — adapter configuration. |
| `workflow.go` | `Workflow`, `StateConfig`, `Transition`, `ParallelGroup`, `EscalationRule`, `HookConfig` — internal representation of workflow definitions. |
| `unmarshal.go` | Custom `UnmarshalYAML` for `Transition` — handles polymorphic `next:` field (scalar string, map, or null). |
| `verdict.go` | `Verdict` type and constants (`bugs_found`, `no_bugs_found`, `approved`, etc.). `ParseVerdict` normalizes raw agent output. |
| `result.go` | `IterationResult` and `ResultAction` enum (`ActionContinue`, `ActionRetry`, `ActionAbort`). |
| `errors.go` | `PhaseError` with typed `PhaseErrorKind` enum for structured error handling. |
| `usage.go` | `Usage` struct for token counts and USD cost tracking. |

## Key Design Decisions

- **`NewPhaseState` initializes all slices to non-nil** — prevents JSON `null` for empty lists.
- **`NewPlanMeta` creates phases with no dependencies** — phases run in parallel by default; dependencies must be declared explicitly.
- **`Transition` key `""` for unconditional transitions** — linear flow uses empty-string verdict key, unifying the routing logic.
- **`ParseVerdict`** strips markdown bold/backticks, lowercases, takes first whitespace-split token, validates against allowed set per state.
