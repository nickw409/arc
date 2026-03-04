# Workflow

Loads, validates, and navigates workflow state machine definitions from YAML. Supports both traditional state-machine format and composable pipeline (block) format.

## File Map

| File | Purpose |
|------|---------|
| `loader.go` | `Load`/`LoadBytes` — reads workflow YAML. If `pipeline:` key present, delegates to `loadComposed` (block composition via `internal/block`). Otherwise builds `arc.Workflow` directly from raw states. |
| `machine.go` | `Machine` — wraps `*arc.Workflow` with O(1) state lookup. `NextState(current, verdict)` drives transitions. `IsTerminal()`, `EntryState()`, `ValidVerdicts()`. |
| `validate.go` | `Validate()` — 9 checks including forward/reverse reachability (BFS), verdict/branch set equality, no duplicate states, entry not terminal. |

## Key Details

- **Two workflow formats**: Traditional (inline `states:` map) and composed (`pipeline:` referencing blocks). The loader auto-detects based on YAML structure.
- **`loadComposed`** calls `block.LoadBlock` → `block.ComposePipeline` → `block.ValidateComposition`.
- **`Machine.NextState`** handles both linear (empty-string verdict key) and branching (verdict lookup) transitions uniformly.
- **Validation is thorough**: forward BFS from entry (all states reachable), reverse BFS from terminals (all states can reach a terminal — catches infinite cycles).
