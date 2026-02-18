# Arc

A workflow engine for orchestrating complex, multi-phase software engineering tasks through AI agents. It breaks large work into phases, drives each phase through a state machine (QA, review, implementation), and enforces rules so agents stay on track.

## Getting Started

### Install

```bash
# Option 1: Install from source
./install.sh

# Option 2: Symlink if developing locally
ln -s /path/to/arc/bin/arc ~/.local/bin/arc
```

Dependencies: `jq`, `yq` (mikefarah v4+), `python3`, `claude` (Claude Code CLI), `git`

### Initialize a Project

```bash
cd your-project
arc init
```

This detects your language and test runner, then creates:
- `.arc.yaml` — project configuration (language, runner, git settings)
- `.plans/` — plan storage directory
- `.claude/commands/plan.md` — `/plan` slash command for Claude Code
- Git hooks for commit and file boundary enforcement
- Claude Code hooks for orchestrator/review agent restrictions

### Create and Run a Plan

```bash
arc plan my-feature phase1 phase2 integration   # Create a plan with phases
vim .plans/active/my-feature/phases/phase1/plan.md  # Write phase plans
arc review my-feature                            # Adversarial review
arc run my-feature                               # Execute through orchestrator
```

### Manual Mode

For step-by-step control instead of full automation:

```bash
arc iterate my-feature phase1 qa          # Run one QA iteration
arc iterate my-feature phase1 qa-review   # Review QA output
arc iterate my-feature phase1 impl        # Run one impl iteration
arc iterate my-feature phase1 impl-review # Review impl output
arc status my-feature phase1              # Check state
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
| **performance** | `benchmark` | Benchmarks drive optimization, not unit tests |

Workflows are defined as YAML state machines in `workflows/`.

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

## Test Runners

Arc uses a plugin system for test runners. Each runner lives in `runners/` and provides a uniform interface:

| Runner | Language | Command |
|--------|----------|---------|
| `cargo-nextest` | Rust | `cargo nextest run` |
| `cargo-test` | Rust | `cargo test` |
| `vitest` | TypeScript | `npx vitest run` |
| `jest` | TypeScript | `npx jest` |
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

Before running a plan, each phase goes through adversarial review. The review system runs 5 specialized adversary agents in parallel:

| Adversary | Focus | Required |
|-----------|-------|----------|
| **coverage** | Every function and error variant has tests | Yes |
| **ambiguity** | Nothing a sub-agent could misinterpret | Yes |
| **scope** | Phase isn't too large to execute reliably | No (warning only) |
| **consistency** | Types, names, and contracts match across phases | Yes |
| **executability** | No blockers that prevent sub-agent execution | Yes |

The review loop iterates up to 5 times. Smart caching skips re-reviewing unchanged phases on subsequent iterations.

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
| `last_test_output.txt` | iterate.sh | Full test output for impl-review |

These live in `.plans/active/<plan>/phases/<phase>/`.

## Directory Structure

```
arc/
├── bin/              CLI entry point
├── workflows/        YAML workflow definitions (feature, bugfix, etc.)
├── prompts/          Prompt templates organized by work type
│   ├── feature/      TDD workflow prompts
│   ├── bugfix/       Bug fix workflow prompts
│   ├── investigation/
│   ├── refactor/
│   ├── performance/
│   ├── common/       Shared prompt snippets
│   └── adversaries/  Plan review adversary prompts
├── runners/          Test runner plugins (cargo-nextest, vitest, pytest, go-test)
├── templates/        Plan and command templates
├── scripts/          Execution and state management scripts
├── hooks/            Claude Code hooks for agent enforcement
├── enforcement/      Git hooks, validation scripts, schemas
├── adversaries/      Adversarial committee configuration
├── tests/            BATS test suite
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
        └── plan.md    /plan slash command
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
| `docs/IMPLEMENTATION_ROADMAP.md` | Version-by-version build history (V1 through V5) |
