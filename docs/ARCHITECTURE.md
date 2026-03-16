# Arc Architecture

## Overview

Arc is a Go binary that orchestrates multi-phase software engineering tasks through AI agents. It breaks work into phases, drives each phase through a **session → gate → retry** loop, and supports multiple AI providers.

**Core principle:** Gates define the completion criteria. Agents do the work. The gate is the final arbiter.

There is **no state machine**. There are no workflow YAMLs, no verdict extraction, no state transitions. Each phase runs one or more agent sessions until its gate assertions pass (or attempts are exhausted).

## System Components

```
cmd/arc/              Entry point (main.go)
internal/
  adapter/            Multi-provider AI adapter (claude, codex, generic)
  agent/              Agent spawning (claude CLI subprocess)
  arc/                Core types: PhaseSpec, PhaseState, GateResult, PlanMeta
  cli/                Cobra command definitions (arc plan, arc daemon, arc manage, ...)
  config/             .arc.yaml parsing
  daemon/             Background orchestration daemon (persistent, multi-plan)
  dev/                arc dispatch pipeline (discovery → architecture → plan generation)
  gate/               Gate assertion evaluation (file_exists, grep, build_passes, ...)
  gitops/             Git commit operations
  guide/              Agent-facing reference guide (arc guide)
  intelligence/       Project intelligence store (test cmds, flaky tests, costs)
  monitor/            Live TUI (bubbletea) for arc status --live
  orchestrator/       Orchestration engine:
    orchestrator.go       Launch() entry point, LaunchOptions, lock management
    launch_gated.go       LaunchGated(): phase scheduling, worktrees, regression suite
    gated.go              RunPhaseGated(): per-phase session→gate→retry loop
    phase_types.go        RunPhaseOptions, commitPhase, discoverNewTestFiles
    intervention.go       Post-exhaustion orchestrator intervention for stuck phases
    adversary.go          Post-plan adversarial test session
    classify.go           Error tier classification (Transient/Feedback/GiveUp)
    observe.go            LoadPhaseState() and state observation helpers
    report.go             COMPLETION_REPORT.md generation
    commitment_audit.go   Post-completion commitment audit
  plan/               Plan creation, status, manage mutations, archival, summaries
  project/            Project detection & init (.arc.yaml, .plans/)
  prompt/             Prompt rendering (Handlebars shim over Go templates)
  resources/          Embedded static assets:
    prompts/          Agent prompt templates (.md) — gate/, dev/, adversaries/, validate/, commitment-audit/
    templates/        Plan scaffolding templates (.md)
    enforcement/      Hook scripts
    guides/           Agent-facing reference docs
    recipes/          Built-in recipe definitions (.yaml)
  review/             Adversarial plan review (4 adversaries with synthesizer)
  selfupdate/         Self-update (GitHub releases, SHA256 verification)
  state/              Phase state.json read/write/update
  testcmd/            Test command resolution and execution
  validate/           AI-powered test quality audit
  worktree/           Git worktree isolation for parallel phase execution
docs/                 Documentation
```

## Plan / Phase Hierarchy

```
Plan: fix-wasm-rng
├── Phase: investigate-variance    status: complete
├── Phase: port-pcg-algorithm      status: in-progress  (attempt 2/2)
└── Phase: verify-cross-engine     status: pending
```

| Concept | What it is | On disk |
|---------|------------|---------|
| **Plan** | The overall work request | `.plans/active/{name}/` |
| **Phase** | A self-contained unit of work | `.plans/active/{name}/phases/{phase}/` |

### Files in a phase directory

| File | Purpose |
|------|---------|
| `spec.yaml` | Phase specification: role, spec text, files, checkpoints, gate assertions |
| `plan.md` | Human-written context and task description for the agent |
| `state.json` | Runtime state: status, iteration count, test counts, usage, blocked reason |
| `gate-status.json` | Last gate run result (assertions, which passed/failed) |
| `last_agent_output.txt` | Captured stdout from the most recent agent session |

### Phase status values

| Status | Meaning |
|--------|---------|
| `pending` | Not yet started |
| `in-progress` | Agent session running or between retries |
| `complete` | Gate passed, committed |
| `blocked` | Exhausted retries or human intervention required |
| `deferred` | Intentionally skipped |

## Execution Model

### 1. Plan creation (`arc plan` / `arc dispatch`)

`plan.Create` always writes `spec.yaml` for every phase. The gate orchestrator requires `spec.yaml` — plans without it cannot run.

```
arc plan my-plan impl qa
  → .plans/active/my-plan/plan.json
  → .plans/active/my-plan/phases/impl/spec.yaml   (role: impl)
  → .plans/active/my-plan/phases/impl/state.json
  → .plans/active/my-plan/phases/impl/plan.md
  → .plans/active/my-plan/phases/qa/spec.yaml     (role: impl)
  → ...
```

### 2. Phase scheduling (`LaunchGated` in `launch_gated.go`)

The orchestrator reads `plan.json` to discover phase dependencies, then runs phases concurrently when their dependencies are satisfied:

```
phases with no deps → start immediately (parallel)
phases with deps    → wait for all deps to reach "complete"
```

If `StopOnFailure` is set (default), one blocked phase cancels all siblings.

### 3. Per-phase execution (`RunPhaseGated` in `gated.go`)

Each phase loops up to `MaxGatedAttempts` (2) times:

```
Attempt N
  1. Build prompt
     - Attempt 1: phase spec + plan.md + project context
     - Attempt 2+: above + gate failure details + git diff
  2. Spawn agent (adapter.Spawn)
  3. Run gate (gate.Run: evaluate all assertions in spec.yaml)
  4a. Gate passed → commit changes, mark phase complete, exit loop
  4b. Gate failed → classify error tier, decide next step:
        TierTransient  (rate limit, crash) → retry same attempt
        TierFeedback   (gate failed, has context) → next attempt with feedback
        TierGiveUp     (attempts exhausted) → orchestrator intervention, then mark blocked
```

### 4. After all phases complete

```
LaunchGated post-processing:
  1. Run regression suite (if configured in .arc.yaml)
     → route failures back to responsible phases
  2. Merge shared worktree back to main branch (if UseWorktree)
  3. Run adversarial test session (if adversary adapter configured)
  4. Generate COMPLETION_REPORT.md
  5. Generate SUMMARY.md
  6. Record intelligence (cost, complexity, failure patterns)
```

## Gate Assertions (`internal/gate/`)

Gate assertions in `spec.yaml` define what "done" means for a phase:

```yaml
gate:
  assertions:
    - type: file_exists
      path: internal/auth/handler.go
    - type: test_exists
      name: TestAuthHandler
    - type: build_passes
      command: go build ./...
    - type: grep
      pattern: "func AuthHandler"
    - type: no_untracked
      pattern: "*.tmp"
  verifier_agent: false   # set true to run AI verifier for review/investigate roles
```

| Assertion type | What it checks |
|----------------|----------------|
| `file_exists` | Path exists relative to workdir |
| `grep` | Regex pattern found in `.go` files |
| `test_exists` | Function name found in `_test.go` files |
| `build_passes` | Command exits with code 0 |
| `no_untracked` | No matching untracked files in git |

Results are written to `gate-status.json` after each gate run.

## Phase Roles

The `role` field in `spec.yaml` determines the agent prompt and gate behavior:

| Role | Agent prompt | Completion verification |
|------|-------------|------------------------|
| `impl` (default) | `gate/impl.md` — write code, run gate | Gate assertions |
| `review` | `gate/review.md` — analyze, produce findings | AI verifier agent |
| `investigate` | `gate/investigate.md` — research, document | AI verifier agent |
| `audit` | `gate/review.md` — security/quality audit | AI verifier agent |

## Error Tier Classification (`internal/orchestrator/classify.go`)

| Tier | Condition | Response |
|------|-----------|----------|
| `TierTransient` (1) | Rate limit, timeout, process crash with no output | Retry immediately |
| `TierFeedback` (2) | Gate failed with actionable context | Retry with gate failure details injected |
| `TierGiveUp` (3) | Attempts exhausted | Run orchestrator intervention, then mark phase blocked |

## Adapter System (`internal/adapter/`)

Arc supports multiple AI providers through `arc.AgentAdapter`:

```go
type AgentAdapter interface {
    Name() string
    Spawn(ctx context.Context, prompt string, workdir string, config SessionConfig) (*AgentResult, error)
}
```

Built-in adapters: `claude` (Claude Code CLI), `codex` (OpenAI Codex CLI), `generic` (any CLI tool).

Adapter selection: per-role in `.arc.yaml` (e.g. `agents.impl: claude`, `agents.adversary: claude`), or per-phase via `spec.yaml`.

### Model Routing

Model selection is configured in `.arc.yaml` under `models:` and resolved per agent role:

```yaml
models:
  default: sonnet       # fallback for all roles
  planner: opus         # arc dispatch discovery/architecture agents
  impl: sonnet          # impl phase agents
  adversary: sonnet     # adversarial test agents
  verifier: sonnet      # gate verifier agents
  orchestrator: opus    # orchestrator intervention agent
  review: sonnet        # arc review adversary agents
```

Per-phase override: `arc manage <plan> <phase> model <model>` sets `model_override` in `state.json`, taking precedence over role-based routing.

## Worktree Isolation (`internal/worktree/`)

Agents run in isolated git worktrees so the developer's working directory stays clean:

- **Shared worktree** (default): one worktree per plan run, all phases share it
- **Per-phase worktree**: each phase gets its own branch (`arc/{plan}/{phase}`)

Worktrees live under `.arc/worktrees/` in the project root. On phase completion, changes are committed and merged back. On failure, the branch is preserved for inspection.

## Prompt System (`internal/prompt/`)

Prompts are Markdown templates using a Handlebars-style syntax shim over Go `text/template`. Key template variables: `{{phase}}`, `{{plan}}`, `{{plan_md}}`, `{{iteration}}`, `{{state.tests_passing}}`, `{{params.key}}`.

Gate prompts (`gate/impl.md`, `gate/retry.md`, etc.) use raw Go template syntax with typed data structs (`ImplData`, `RetryData`), rendered via `prompt.RenderGatePrompt`.

Dev pipeline prompts (`dev/discovery.md`, `dev/architect.md`, etc.) use the Handlebars shim via `prompt.Render`.

## Automated Plan Generation (`arc dispatch`, `internal/dev/`)

```
Task description
  → RunDiscovery   — agent reads codebase, outputs JSON: complexity, phases, approach
  → ValidateComplexity — heuristic overrides if agent under/over-estimated
  → RunClarificationLoop — interactive Q&A (skipped with --yes)
  → GeneratePlanName
  → Branch by complexity:
      simple   → GeneratePlan (direct: single-phase "execute" plan)
      medium   → GeneratePlan (feature: impl phases with descriptions)
      complex  → RunArchitects (3 parallel agents) → SelectProposal → GeneratePlan
  → arc review (optional)
  → orchestrator.Launch
  → RunCodeReview  — post-execution review of git diff
```

## Daemon (`internal/daemon/`)

Persistent background process for plan execution:

- **Unix socket** at `~/.arc/daemon.sock` — JSON newline-delimited protocol
- **Bounded worker pool** (default: 3 parallel phases), dependency-aware FIFO dispatch
- **State persistence** — `~/.arc/daemon-state.json` for crash recovery
- **File locking** — exclusive flock on `~/.arc/daemon.lock` prevents duplicates
- **Auto-start** — `arc daemon submit` starts the daemon if not already running

`arc daemon submit` is the primary way to execute plans. The daemon handles multiple plans concurrently with dependency-aware scheduling.

## Intelligence Store (`internal/intelligence/`)

Arc learns from runs and uses that knowledge to improve future runs:

- Test command discovery (what command to run tests)
- Flaky test tracking (tests that fail intermittently)
- File coupling (which files change together)
- Cost tracking per plan/complexity
- Failure pattern recording

## Key Invariants

These invariants must hold across the entire codebase:

1. **Every phase has `spec.yaml`** — `plan.Create` always writes it. There are no "legacy" plans without `spec.yaml`. The gate orchestrator requires it.
2. **No state machines** — There are no workflow YAMLs, no verdict strings, no `## Verdict` extraction, no `internal/workflow/`, `internal/pipeline/`, `internal/block/`, `internal/runner/` packages. All execution goes through `RunPhaseGated`.
3. **`Launch()` always calls `LaunchGated()`** — There is no other execution path. The `IsGatedPlan()` function does not exist.
4. **Gate is the only completion criterion** — Phases complete when their gate passes, not when an agent says "done" or outputs a verdict token.
5. **Prompts are in `gate/` or `dev/`** — There are no `feature/`, `bugfix/`, `blocks/`, `common/` prompt directories. The prompt namespace is: `gate/`, `dev/`, `adversaries/`, `validate/`, `commitment-audit/`.
