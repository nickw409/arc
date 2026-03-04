# Plan

Full lifecycle management of plans and phases on disk: creation, status display, phase management mutations, split/insert operations, summary generation, and archival.

## File Map

| File | Purpose |
|------|---------|
| `create.go` | `Create()` — creates plan directory, `plan.json`, phase subdirectories with `state.json` and `plan.md`. Validates plan name regex, deduplicates phases. |
| `phase.go` | `Split()` — replaces a phase with sub-phases, rewires dependencies. `Insert()` — adds phases before/after a reference. `Defer()` — marks phase deferred. |
| `status.go` | `Status()` — displays plan/phase status with icons. `StatusIcon()` maps status strings to `[ ]`, `[x]`, `[X]`, etc. |
| `manage.go` | All `Manage*` functions: `Complete`, `Pending`, `Defer`, `Block`, `Tests`, `Packages`, `Note`, `Iteration`, `Activity`, `CopyFrom`, `Reset`, `ResetPlan`, `Show`. Each calls `stateFileFor(opts).Update()`. |
| `summary.go` | `GenerateSummary()` — writes `SUMMARY.md` with objective, status counts, per-phase details, changed files, costs. |
| `archive.go` | `Archive()` — validates terminal status, sets archived metadata, cleans up worktrees, moves plan to archive directory. |

## Key Details

- Plan name validation: `^[a-z][a-z0-9-]*[a-z0-9]$`.
- `ManageReset` replaces state with `NewPhaseState`, preserving only `ParentPhase`.
- `Archive` without `--force` requires all phases to be terminal (`complete`, `split`, or `deferred`) and `COMPLETION_REPORT.md` to exist.
