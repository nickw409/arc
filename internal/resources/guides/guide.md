# Arc Reference Guide

Arc is a workflow engine for orchestrating multi-phase software engineering tasks through AI agents. It breaks large work into phases, drives each phase through an agent session loop, and verifies completion through objective gate assertions.

## Core Concepts

- **Plan** — The overall work request (e.g., `fix-auth`). Contains one or more phases.
- **Phase** — A self-contained unit of work with a `spec.yaml` (machine-readable spec), `plan.md` (human-readable instructions), `state.json` (runtime state), and gate assertions.

There is no state machine. There are no workflows. Execution flow is: `arc plan` → `arc review` → `arc run`.

<!-- section: setup -->

## Project Setup

### Prerequisites

- `claude` — Claude Code CLI (the agent runtime)
- `git` — Version control
- `jq` — JSON processing
- `yq` — YAML processing (mikefarah v4+)
- `python3` — Utility scripts

### Initializing a Project

```bash
arc init
```

Creates `.arc.yaml` in the project root.

### `.arc.yaml` Fields

```yaml
language: go                  # go, rust, typescript, python, unknown
runner: go-test               # go-test, cargo-nextest, vitest, pytest, etc.
build_command: go build ./... # optional override
test_command: go test ./...   # optional override
default_package: ./...        # default test target

git:
  commit_style: conventional  # conventional or freeform
  sign: false
  co_author: false
  base_branch: main           # branch worktrees diverge from and merge into

agents:
  default: claude             # default agent adapter
  impl: claude                # override per role: impl, adversary, verifier, orchestrator
  verifier: claude

max_parallel: 3               # max phases running concurrently
verifier: auto                # always, never, or auto (auto enables for medium/complex phases)

budget:
  max_cost: 0                 # USD hard limit (0 = unlimited)
  warn_cost: 0                # USD warning threshold
```

<!-- /section: setup -->

<!-- section: plans -->

## Writing Plans

### Creating a Plan

```bash
arc plan <plan-name> <phase1> [phase2] ...
arc plan --type bugfix <plan-name> <phase1> [phase2] ...
```

This scaffolds a plan directory with a `plan.md` and `spec.yaml` for each phase. Write the phase specification to `plan.md` before running `arc review`.

**Plan file paths:**

```
.plans/active/<plan-name>/<phase-name>/plan.md
.plans/active/<plan-name>/<phase-name>/spec.yaml
```

### Phase Roles

| Role | Purpose | Gate mechanism |
|------|---------|----------------|
| `impl` (default) | Write code | Gate assertions (file_exists, grep, etc.) |
| `review` | Analyze code quality | AI verifier agent |
| `investigate` | Research questions | AI verifier agent |
| `audit` | Security/quality audit | AI verifier agent |

### PhaseSpec Fields (spec.yaml)

```yaml
name: my-phase
role: impl                    # impl, review, investigate, audit
complexity: medium            # simple (50 turns), medium (100), complex (200)
spec: |
  Detailed instructions for the agent.
verify: |
  Acceptance criteria for the AI verifier (review/investigate/audit roles only).
files:
  - internal/foo/bar.go
checkpoints:
  - name: builds
    description: Package compiles
    test: go build ./internal/foo/
  - name: tests-pass
    description: All tests pass
    test: go test ./internal/foo/
gate:
  assertions:
    - file_exists: internal/foo/bar.go
    - grep: "func NewBar"
    - test_exists: TestNewBar
    - build_passes: go build ./...
    - no_untracked: "true"
  verifier_agent: false
deps:
  - previous-phase
```

### Gate Assertion Types

| Type | Field | What it checks |
|------|-------|---------------|
| `file_exists` | path relative to workdir | File or directory exists |
| `grep` | pattern string | Pattern found in any `.go` file in workdir |
| `test_exists` | function name | Test function found in `_test.go` files |
| `build_passes` | shell command | Command exits with code 0 |
| `no_untracked` | any value | No debug/temp artifact files untracked in git |

Both explicit-field and `type`+`target` formats are accepted:

```yaml
# Explicit field (preferred):
- file_exists: internal/foo/bar.go
- grep: "func NewBar"
- build_passes: go build ./...

# Legacy type+target:
- type: file_exists
  target: internal/foo/bar.go
  description: Optional human-readable label
```

### Rules for Good Plans

1. **Explore first.** Read actual code before proposing anything.
2. **Be concrete.** Every function signature, error type, and test value must be exact.
3. **Write real test cases.** Not "test edge cases" — actual values and expected outputs.
4. **Keep phases small.** Each phase should complete in roughly 15 agent iterations. If a phase touches more than 10 files, split it.
5. **Write gate assertions for every integration point.** A phase is not done when its package builds — it is done when it is wired in. Assert on every file that must change.

<!-- /section: plans -->

<!-- section: execution -->

## Execution

### Commands

```bash
# Project setup
arc init                                   # Initialize project

# Plan lifecycle
arc plan <name> <phase1> [phase2] ...      # Create plan scaffolding
arc plan --type bugfix <name> <phases...>  # Create with workflow type label
arc review <plan-name>                     # Adversarial review with auto-remediation
arc review <plan-name> --phase <phase>     # Review a single phase
arc run <plan-name>                        # Launch orchestrator for all phases
arc status [plan-name]                     # Show plan/phase status
arc archive [--force] <plan-name>          # Archive completed plan
arc dispatch <task description...>         # Auto-generate plan and run it

# Phase management
arc manage <plan> <phase> complete         # Mark phase complete
arc manage <plan> <phase> pending          # Reset phase to pending
arc manage <plan> <phase> defer <reason>   # Defer phase with reason
arc manage <plan> <phase> block <reason>   # Block phase
arc manage <plan> <phase> tests <pass> <total>
arc manage <plan> <phase> packages <pkgs>
arc manage <plan> <phase> note <text>
arc manage <plan> <phase> iteration <n>
arc manage <plan> <phase> copy-from <src>
arc manage <plan> <phase> show             # Print state.json

# Other
arc guide                                  # Print this guide
arc validate [paths...]                    # AI-powered test quality audit
arc daemon start|stop|list|status          # Manage background daemon
arc cancel <plan-name>                     # Cancel a running plan
arc cleanup <plan-name>                    # Clean up worktree/lock for a plan
arc chat                                   # Interactive Claude session with Arc guide as system context
```

### Gate-Based Execution

Each phase runs through this loop:

1. Spawn an agent session (impl prompt or retry prompt with gate feedback)
2. Run gate assertions after agent exits
3. If gate passes → commit and mark phase complete
4. If gate fails → classify error tier, retry
5. After `MaxGatedAttempts` (2) failed attempts → phase is blocked
6. The daemon watcher auto-retries blocked phases up to `MaxWatchAttempts` (3) times

The working directory is preserved across retries — agents build on prior work.

**Turn budgets by complexity:**
- `simple`: 50 turns
- `medium`: 100 turns (default)
- `complex`: 200 turns

### Adversarial Review

`arc review` validates each phase plan using 5 parallel adversaries with auto-remediation:

| Adversary | Focus | Blocking |
|-----------|-------|----------|
| **executability** | No blockers that prevent agent execution | Yes |
| **consistency** | Types, names, contracts match across phases | Yes |
| **coverage** | Every function and error variant has tests | Yes |
| **ambiguity** | Nothing an agent could misinterpret | Yes |
| **scope** | Phase isn't too large to execute reliably | No (warning) |

Review runs up to 5 auto-remediation iterations. If unresolved after 5, status is set to `"conditional"` — `arc run` accepts both `"approved"` and `"conditional"`.

### State Tracking

Each phase maintains `state.json`:

```json
{
  "phase": "my-phase",
  "plan": "my-plan",
  "workflow_type": "feature",
  "phase_status": "in_progress",
  "iteration": {"current": 2, "max": 25},
  "tests_passing": 8,
  "tests_total": 12,
  "last_verdict": "",
  "notes": "Working on error handling",
  "blocked_reason": "",
  "watch_attempts": 0,
  "attempt_log": []
}
```

Key `phase_status` values: `pending`, `in_progress`, `complete`, `blocked`, `deferred`.

Use `arc manage <plan> <phase> show` to print a phase's `state.json`.

### Plan Metadata (plan.json)

```json
{
  "name": "fix-auth",
  "status": "active",
  "phases": ["investigate", "fix"],
  "phase_order": {"investigate": 1, "fix": 2},
  "dependencies": {"fix": ["investigate"]},
  "review_status": "approved",
  "workflow_type": "bugfix"
}
```

### Worktree Behavior

By default `arc run` creates a **shared git worktree** — a separate branch where all phases execute, merged back on completion.

| Option | Behavior |
|--------|---------|
| `worktree: true` (default) | One shared worktree for the plan; all phases share it |
| `per_phase_worktree: true` | Each phase gets its own worktree |
| `worktree: false` | No isolation — runs in-tree |

Branch names: `arc/<plan-name>` (shared) or `arc/<plan-name>/<phase-name>` (per-phase).

### Failure Intervention

When a run stops with a blocked phase:

1. **Inspect the failed phase** — `arc manage <plan> <phase> show` reads `state.json`. Check `attempt_log` for gate feedback from the last attempt.
2. **Find the worktree** — `git worktree list` to locate `arc-worktree-*`.
3. **Diagnose:**
   - **Ambiguous spec** — edit `spec.yaml` and `plan.md` to be more concrete.
   - **Phase too large** — split into smaller phases.
   - **Wrong gate assertions** — tighten or fix assertions.
   - **Environment issue** — fix the underlying problem.
4. **Reset and resume** — `arc manage <plan> <phase> pending`, then `arc run <plan>`.

<!-- /section: execution -->

<!-- section: mistakes -->

## Common Mistakes

### Gates That Only Check New Code

A phase implementing a new system should assert on **every integration point** — not just that the new package builds, but that every caller that must wire it in has actually done so.

```yaml
gate:
  assertions:
    - build_passes: go build ./...
    - grep: "daemon\\.Submit"       # existing CLI must reference the new daemon
    - test_exists: TestDaemonSubmit
```

### Vague Specs

**Wrong:** "Handle errors appropriately."
**Right:** "Return `ErrNotFound{ID: id}` when the record doesn't exist."

### Phases Too Large

If a phase touches more than 10 files or 3 packages, split it. Large phases exhaust the turn budget before completing.

### Missing Test Cases

**Wrong:** "Test the happy path and edge cases."
**Right:** Named test cases with concrete inputs and expected outputs.

<!-- /section: mistakes -->
