# Arc

A workflow engine for orchestrating complex, multi-phase software engineering tasks through AI agents. It breaks large work into phases, drives each phase through a state machine (QA, review, implementation), and enforces rules so agents stay on track.

## Getting Started

### Install

```bash
# Install the latest release binary
curl -fsSL https://raw.githubusercontent.com/nickw409/arc/main/install.sh | bash
```

This downloads a prebuilt binary to `~/.local/bin/arc`. No build toolchain required.

To update to the latest version:

```bash
arc update
```

For private repo access, `arc update` automatically uses your `gh` CLI credentials. You can also set `GITHUB_TOKEN` or `GH_TOKEN` explicitly.

Dependencies: `claude` (Claude Code CLI), `git`, `jq`, `yq` (mikefarah v4+), `python3`

### Initialize a Project

```bash
cd your-project
arc init
```

This detects your language and test runner, then creates:
- `.arc.yaml` — project configuration (language, runner, git settings)
- `.plans/` — plan storage directory
- `.claude/commands/arc-plan.md` — `/arc-plan` slash command for Claude Code
- Git hooks for commit and file boundary enforcement
- Claude Code hooks for orchestrator/review agent restrictions

### Interactive Mode with `arc chat`

Launch an interactive Claude session with Arc tools available as MCP tools:

```bash
arc chat                        # Start interactive session
arc chat --model opus           # Use a specific model
```

The chat agent can plan, review, run, and monitor Arc plans conversationally. Orchestrator runs are async — `arc_run` returns immediately and you can poll with `arc_run_status` or cancel with `arc_run_cancel`.

### Quick Start with `arc dev`

For the fastest path from idea to code, use `arc dev`:

```bash
arc dev "Add user authentication with JWT tokens"
```

This runs the full pipeline automatically: discovers relevant code, classifies task complexity, generates a plan with phases, runs adversarial review, and launches the orchestrator. Options:

```bash
arc dev --skip-review "Fix the login bug"          # Skip adversarial review
arc dev --timeout 7200 "Refactor the auth module"  # Custom timeout (seconds)
arc dev --interactive "Add caching layer"           # Prompt before review/launch
```

### Create and Run a Plan Manually

```bash
arc plan my-feature phase1 phase2 integration   # Create a plan with phases
vim .plans/active/my-feature/phases/phase1/plan.md  # Write phase plans
arc review my-feature                            # Adversarial review (auto-remediates)
arc review my-feature --phase phase1             # Review a single phase
arc run my-feature                               # Execute through orchestrator
```

### Manual Mode

For step-by-step control instead of full automation:

```bash
arc iterate my-feature phase1             # Run one iteration (advances current state)
arc status my-feature                     # Check plan/phase status
arc manage my-feature phase1 show         # Inspect phase state.json
```

### Validate Tests

```bash
arc validate                    # Audit test quality in current directory
arc validate ./pkg/auth ./pkg/db  # Audit specific paths
arc validate --workers 8        # Run with more parallel agents
arc validate set-prompt my.md   # Use a custom audit prompt
arc validate clear-prompt       # Revert to built-in prompt
```

### Monitor

```bash
arc monitor my-feature   # Live TUI for orchestration progress
```

## How It Works

### Three-Level Hierarchy

```
Plan (e.g., "fix-wasm-rng")
  └── Phase: investigate-variance
  │     └── States: research → draft → review → complete
  └── Phase: port-pcg-algorithm
  │     └── States: qa → qa_review → impl → impl_review → complete
  └── Phase: verify-cross-engine
        └── States: qa → qa_review → impl → impl_review → complete
```

- **Plan** — The overall work request. Contains one or more phases.
- **Phase** — A self-contained unit of work with its own state machine, test suite, and `state.json`.
- **State** — The current position in a workflow's state machine (e.g., `qa`, `impl_review`).

### Execution Flow

```
arc plan             Create plan directory structure and phase scaffolding
    │
    ▼
arc review           Adversarial review validates each phase plan
    │
    ▼
arc run              Launch the orchestrator agent (read-only, cannot edit code)
    │
    ▼
  ┌────────────────────────────────────────────┐
  │  For each phase:                           │
  │    iterate  ──► spawn sub-agent            │
  │        │        (writes code/tests)        │
  │        ▼                                   │
  │    extract verdict from review output      │
  │        │                                   │
  │        ▼                                   │
  │    resolve next state ──► advance or loop  │
  │        │                                   │
  │        ▼                                   │
  │    run hooks (run_tests, commit, etc.)     │
  └────────────────────────────────────────────┘
    │
    ▼
COMPLETION_REPORT.md  Generated when all phases finish
```

### The Iteration Pipeline

Each iteration runs an 8-step pipeline:

1. **Check intervention** — Exit if human input is needed.
2. **Check escalation** — If stuck, trigger escalation actions (analyze, switch model, auto-split).
3. **Check pre-constraints** — Verify required input artifacts exist and iteration limits aren't exceeded.
4. **Render prompt and spawn agent** — Build the prompt from the workflow's template, create an `iteration_NNN/` directory, and launch a sub-agent.
5. **Extract verdict** — Parse the sub-agent's output for a verdict (e.g., `approved`, `gaps_found`).
6. **Check post-constraints** — Verify required output artifacts were produced.
7. **Run after-hooks** — Execute actions like `run_tests` or `commit`.
8. **Update state** — Increment iteration count, resolve the next state, track stuck iterations.

## Work Types

Five workflow types, each with its own state machine and prompt set:

| Type | Entry State | Description |
|------|-------------|-------------|
| **feature** | `qa` | TDD — write tests first, then implement |
| **bugfix** | `investigate` | Reproduce the bug, write regression tests, fix |
| **investigation** | `research` | Research only, no code changes, outputs documentation |
| **refactor** | `characterize` | Characterization tests must pass before and after changes |
| **performance** | `baseline` | Benchmarks drive optimization, not unit tests |
| **adversarial** | `impl` | Implement freely, then adversary writes tests to find bugs |
| **direct** | `impl` | Single-phase execution for simple tasks (used by `arc dev`) |

Workflows are defined as YAML state machines in `workflows/`. The **adversarial** and **direct** workflows are composed from reusable blocks (see Composable Blocks below).

## Workflow YAML

Workflows are data, not code. A simplified example:

```yaml
name: feature
version: 4

states:
  - name: qa
    prompt: prompts/feature/qa.md
    next: qa_review

  - name: qa_review
    prompt: prompts/feature/qa-review.md
    verdicts: [approved, gaps_found]
    next:
      approved: impl
      gaps_found: qa

  - name: impl
    prompt: prompts/feature/impl.md
    constraints:
      require_artifacts_in: [qa_reasoning.md]
    after:
      - action: run_tests
    escalation:
      - at_iteration: 3
        action: analyze_stuck
      - at_iteration: 5
        action: switch_model
        params: { model: opus }
    next: impl_review

  - name: impl_review
    prompt: prompts/feature/impl-review.md
    verdicts: [approved, concerns]
    next:
      approved: complete
      concerns: impl

entry_state: qa
terminal_states: [complete, blocked]
```

Key capabilities by schema version:

| Version | Feature |
|---------|---------|
| V1 | Linear state transitions |
| V2 | Conditional branching (verdict determines next state) |
| V3 | Parameters and Handlebars-style template variables |
| V4 | Hooks, constraints, escalation triggers, intervention |
| V5 | Parallel state execution with join strategies |

All versions are backwards compatible — V1 workflows run on the V5 engine.

## Composable Blocks

Workflows can be composed from reusable, parameterized blocks instead of writing monolithic YAML state machines. A block is a self-contained group of states with entry/exit points:

```yaml
# blocks/adversary.yaml
name: adversary
params:
  max_rounds: {default: 5}
  max_turns: {default: 30}
entry: adversary
exits: [done]
states:
  - name: adversary
    verdicts: [bugs_found, no_bugs_found]
    constraints:
      max_iterations: ${max_rounds}
    next:
      bugs_found: adversary
      no_bugs_found: $done
```

Workflows compose blocks into pipelines:

```yaml
# workflows/adversarial.yaml
name: adversarial
pipeline:
  - block: impl
    params: {max_turns: 45}
  - block: adversary
    params: {max_rounds: 3}
```

The loader resolves blocks into a flat state machine at load time — the runtime always operates on a flat state machine. States are namespaced by block (e.g., `adversary.adversary`). Exit points wire to the next block's entry.

Built-in blocks: `impl`, `qa-loop`, `review`, `adversary`.

## Git Worktree Isolation

Agents can run in isolated git worktrees so developers can keep working in the main tree:

```bash
arc run my-plan --worktree    # Each phase gets its own worktree branch
```

Each phase gets a branch like `arc/my-plan/phase-name` in a temp directory. On completion, the worktree branch is merged back into the main branch. On failure, the branch is preserved for inspection but the worktree directory is cleaned up.

## `arc dev` Pipeline

`arc dev` automates the full lifecycle from task description to running code:

```
Task description
    │
    ▼
Discovery agent (read-only) ──► Analyzes codebase, classifies complexity
    │
    ▼
Complexity routing:
    ├── Simple: Single "direct" phase, no review
    ├── Medium: Multi-phase with built-in workflow, adversarial review
    └── Complex: 3 architect agents propose designs, best selected,
                 custom workflow generated, adversarial review
    │
    ▼
Orchestrator launch ──► Executes plan phases
    │
    ▼
SUMMARY.md ──► Generated on completion with stats, cost, files changed
```

## Plan Summaries

When a plan completes, a `SUMMARY.md` is generated in the plan directory containing:

- Objective (from first phase's `plan.md`)
- Phase completion status and per-phase details (iterations, tests, commits, cost)
- Files changed across all phases (collected from git history)
- Total cost and next steps for blocked/deferred phases

## Test Runners

Arc uses a plugin system for test runners. Each runner lives in `runners/` and provides a uniform interface:

| Runner | Language | Command |
|--------|----------|---------|
| `cargo-nextest` | Rust | `cargo nextest run` |
| `vitest` | TypeScript | `npx vitest run` |
| `pytest` | Python | `pytest` |
| `go-test` | Go | `go test ./...` |

The runner is selected automatically by `arc init` based on project detection, or set manually in `.arc.yaml`.

## State Tracking

Each phase maintains a `state.json` file:

```json
{
  "phase": "port-pcg",
  "current_state": "impl",
  "iteration": 5,
  "stuck_iterations": 2,
  "tests_passing": 8,
  "tests_total": 12,
  "verdicts_history": ["gaps_found", "approved", "concerns", "concerns"],
  "disputes": [],
  "escalation_history": ["analyze_stuck@3"]
}
```

## Stuck Detection and Escalation

A phase is considered **stuck** when the same tests fail for 2+ consecutive iterations with >80% similar error signatures. The escalation ladder:

| Stuck Count | Action |
|-------------|--------|
| 0-2 | Normal iteration |
| 3 | Spawn a read-only investigator agent to diagnose the issue |
| 4 | Spawn a targeted fix agent |
| 5 | Switch to a more capable model (opus) |
| 6+ | Attempt to auto-split the phase into smaller sub-phases |

If max iterations are exceeded, the system requests human intervention.

## Plan Review

Before running a plan, each phase goes through adversarial review with auto-remediation. The review system runs 5 specialized adversary agents in parallel:

| Adversary | Focus | Priority | Required |
|-----------|-------|----------|----------|
| **executability** | No blockers that prevent sub-agent execution | 1 (highest) | Yes |
| **consistency** | Types, names, and contracts match across phases | 2 | Yes |
| **coverage** | Every function and error variant has tests | 3 | Yes |
| **ambiguity** | Nothing a sub-agent could misinterpret | 4 | Yes |
| **scope** | Phase isn't too large to execute reliably | 5 (lowest) | No (warning only) |

When adversaries find issues, they emit structured suggestions (find-and-replace blocks targeting `plan.md`). Suggestions can include confidence scores:

```
<<<ORIGINAL (confidence: 85)
exact text from plan.md
<<<SUGGESTED
replacement text
>>>
```

The review loop:

1. Runs all 5 adversaries in parallel
2. Parses suggestions from any that failed
3. Filters by confidence threshold (default: 80) — low-confidence suggestions are dropped
4. Merges suggestions by priority (higher-priority adversary wins conflicts)
5. Applies fixes to `plan.md` mechanically
6. Re-reviews until all pass or iteration limit (5) is hit

Smart caching skips re-reviewing when `plan.md` hasn't changed (SHA256 hash match). Use `arc manage reset-review <plan> <phase>` to clear the cache and iteration counter.

## Sub-Agent Enforcement

The orchestrator agent cannot edit code directly. Sub-agents are restricted from running test commands directly — they must use the runner plugins. This is enforced through:

1. **Claude Code hooks** — Block orchestrator writes and restrict review agents to their output files.
2. **Git hooks** — Enforce file boundaries based on agent role (qa, impl, review).
3. **Watchdog** — Background process that kills unauthorized test processes.

## Review Artifacts

Each phase produces reasoning and review documents:

| File | Written By | Purpose |
|------|------------|---------|
| `qa_reasoning.md` | QA agent | Explains what's tested and why |
| `qa_review.md` | QA reviewer | Verifies coverage, finds gaps |
| `impl_reasoning.md` | Impl agent | Hypothesis, evidence, alternatives |
| `impl_review.md` | Impl reviewer | Challenges weak reasoning |
| `last_test_output.txt` | Pipeline | Full test output for impl-review |

These live in `.plans/active/<plan>/phases/<phase>/`.

## Directory Structure

```
arc/
├── cmd/arc/          CLI entry point (main.go)
├── internal/         All Go packages
│   ├── cli/          Cobra command definitions
│   ├── selfupdate/   GitHub Releases-based self-update
│   ├── orchestrator/ Top-level orchestrator loop
│   ├── mcp/          MCP server and tool handlers (arc chat backend)
│   ├── pipeline/     Phase iteration, escalation, hooks
│   ├── agent/        Agent spawning
│   ├── runner/       Subprocess runner (claude CLI)
│   ├── review/       Adversarial plan review
│   ├── workflow/     Workflow YAML loading & validation
│   ├── state/        Phase state (state.json) management
│   ├── config/       .arc.yaml parsing
│   ├── prompt/       Prompt rendering & extraction
│   ├── plan/         Plan creation, status & summary generation
│   ├── project/      Project detection & init
│   ├── gitops/       Git commit operations
│   ├── monitor/      Live TUI (bubbletea)
│   ├── resources/    Embedded templates, prompts & blocks
│   ├── block/        Composable workflow block loading & composition
│   ├── worktree/     Git worktree isolation for parallel execution
│   ├── dev/          Arc dev pipeline (discovery → architecture → plan generation)
│   ├── logging/      Structured logger
│   ├── migrate/      State migration
│   ├── guide/        Agent-facing reference guide
│   ├── validate/     AI-powered test quality audit
│   └── arc/          Core types (verdict, result, errors, state)
├── workflows/        YAML workflow definitions (feature, bugfix, etc.)
├── prompts/          Prompt templates organized by work type
├── runners/          Test runner plugins (cargo-nextest, vitest, pytest, go-test)
├── templates/        Plan and command templates
├── testdata/         Test fixtures
└── docs/             Detailed documentation
```

In each project that uses arc:

```
your-project/
├── .arc.yaml         Project configuration (language, runner, git settings)
├── .plans/
│   ├── active/       Plans currently being worked on
│   │   └── <plan>/
│   │       ├── plan.json
│   │       └── phases/
│   │           └── <phase>/
│   │               ├── plan.md
│   │               └── state.json
│   └── archive/      Completed plans
└── .claude/
    ├── settings.json  Claude Code hooks (installed by arc init)
    └── commands/
        └── arc-plan.md  /arc-plan slash command
```

## Further Reading

| Document | Content |
|----------|---------|
| `docs/ARCHITECTURE.md` | System design goals and component overview |
| `docs/WORKFLOW_SCHEMA.md` | Complete YAML specification |
| `docs/STATE_SCHEMA.md` | `state.json` field definitions |
| `docs/ADVERSARY_SYSTEM.md` | Plan review design |
| `docs/PLANNING_PROCESS.md` | How to write phase plans |
| `docs/INTERVENTION_SYSTEM.md` | Escape hatches and overrides |
| `docs/V4_FEATURES.md` | Hooks, constraints, and escalation details |
| `docs/PROMPT_TEMPLATES.md` | Template variable system |
| `docs/IMPLEMENTATION_ROADMAP.md` | Version-by-version build history (V1 through V5) |
