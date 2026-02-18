# Orchestration Workflow

A workflow engine for orchestrating complex, multi-phase software engineering tasks through AI agents. It breaks large work into phases, drives each phase through a state machine (QA, review, implementation), and enforces rules so agents stay on track.

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
init-plan.sh         Create plan directory structure and phase scaffolding
        │
        ▼
plan-review-loop.sh  Adversarial review validates each phase plan
        │
        ▼
run-orchestrator.sh  Launch the orchestrator agent (read-only, cannot edit code)
        │
        ▼
   ┌────────────────────────────────────────────┐
   │  For each phase:                           │
   │    iterate.sh  ──► spawn sub-agent         │
   │        │           (writes code/tests)     │
   │        ▼                                   │
   │    extract verdict from review output      │
   │        │                                   │
   │        ▼                                   │
   │    get-next-state.sh  ──► advance or loop  │
   │        │                                   │
   │        ▼                                   │
   │    run hooks (run_tests, commit, etc.)     │
   └────────────────────────────────────────────┘
        │
        ▼
COMPLETION_REPORT.md  Generated when all phases finish
```

### The Iteration Pipeline

Each call to `iterate.sh` runs an 8-step pipeline:

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

This tracks iteration counts, test results, verdict history, disputes between agents, and escalation actions taken. Scripts use `update-state.sh` and `get-state.sh` to read and write state atomically.

## Stuck Detection and Escalation

A phase is considered **stuck** when the same tests fail for 2+ consecutive iterations with >80% similar error signatures. The escalation ladder for implementation states:

| Stuck Count | Action |
|-------------|--------|
| 0-2 | Normal iteration |
| 3 | Spawn a read-only investigator agent to diagnose the issue |
| 4 | Spawn a targeted fix agent |
| 5 | Switch to a more capable model (opus) |
| 6+ | Attempt to auto-split the phase into smaller sub-phases |

If max iterations are exceeded, the system requests human intervention.

## Plan Review

Before running a plan, each phase goes through adversarial review to catch issues early. The review system runs 5 specialized adversary agents in parallel, each examining the plan from a different angle:

| Adversary | Focus | Required |
|-----------|-------|----------|
| **coverage** | Every function and error variant has tests | Yes |
| **ambiguity** | Nothing a sub-agent could misinterpret | Yes |
| **scope** | Phase isn't too large to execute reliably | No (warning only) |
| **consistency** | Types, names, and contracts match across phases | Yes |
| **executability** | No blockers that prevent sub-agent execution | Yes |

The review loop (`plan-review-loop.sh`) iterates up to 5 times:

1. Spawn all 5 adversaries in parallel as read-only agents.
2. Each adversary produces a verdict (pass or fail) and a review file.
3. If all required adversaries pass → exit 0 (approved).
4. If any required adversary fails → exit 1, write a failures summary. Fix the plan and re-run.
5. If a previously passing adversary now fails → exit 2 (regression detected).
6. If 5 iterations are exceeded → exit 3 (auto-approve with warning).

Smart caching skips re-reviewing unchanged phases on subsequent iterations. Phase hashes track which `plan.md` files changed between runs, and cross-phase adversaries (scope, consistency) always receive the full plan while per-phase adversaries only re-examine changed phases.

## Sub-Agent Enforcement

The orchestrator agent cannot edit code directly. Sub-agents it spawns are restricted from running `cargo` or `bats` directly — they must use `scripts/run-phase-tests.sh` instead. This is enforced through three layers:

1. **PATH shims** — Wrapper scripts in `bin/` intercept `cargo`/`bats` and return an error when `ORCHESTRATOR_MODE=1`.
2. **Tool restrictions** — Review agents only receive read-only tools (no Bash).
3. **Background process** — A fallback that kills unauthorized `cargo`/`bats` processes.

## Usage

```bash
# 1. Create a plan with phases
$ARC_HOME/scripts/init-plan.sh my-feature phase1 phase2 integration

# 2. Write phase plans
vim .plans/active/my-feature/phases/phase1/plan.md

# 3. Validate plans through adversarial review
$ARC_HOME/scripts/plan-review-loop.sh my-feature

# 4. Run the orchestrator
$ARC_HOME/scripts/run-orchestrator.sh my-feature
```

The orchestrator executes phases in order, spawning sub-agents for each state, running tests, committing at phase boundaries, and handling disputes and stuck states autonomously. When finished, it produces a `COMPLETION_REPORT.md`.

### Manual Mode

For step-by-step control instead of full automation:

```bash
# Run QA + review
$ARC_HOME/scripts/iterate.sh my-feature phase1 qa
$ARC_HOME/scripts/iterate.sh my-feature phase1 qa-review

# Run implementation + review (repeat until approved)
$ARC_HOME/scripts/iterate.sh my-feature phase1 impl
$ARC_HOME/scripts/iterate.sh my-feature phase1 impl-review

# Check state
$ARC_HOME/scripts/get-state.sh my-feature phase1
```

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

## State Commands

The `update-state.sh` script provides atomic state operations:

| Command | Purpose |
|---------|---------|
| `status <STATUS>` | Set phase status |
| `tests <pass> <total>` | Update test counts |
| `increment-iteration` | Bump iteration counter |
| `dispute <test> <reason>` | File a dispute on a test |
| `approve-dispute <reason>` | Approve dispute, enable fix mode |
| `reject-dispute <reason>` | Reject dispute, continue impl |
| `check-review-required` | Exit 1 if impl-review needed (gates commits) |
| `check-qa-review-required` | Exit 1 if qa-review needed (gates test commits) |

## Directory Structure

```
$ARC_HOME/
├── workflows/        YAML workflow definitions (feature, bugfix, etc.)
├── prompts/          Prompt templates organized by work type
│   ├── feature/      TDD workflow prompts
│   ├── bugfix/       Bug fix workflow prompts
│   ├── investigation/
│   ├── refactor/
│   ├── performance/
│   ├── common/       Shared prompt snippets
│   └── adversaries/  Plan review adversary prompts
├── templates/        Plan templates (plan-template.md, etc.)
├── scripts/          Execution and state management scripts
├── docs/             Detailed documentation
├── adversaries/      Adversarial committee configuration
├── bin/              PATH shims for cargo/bats enforcement
├── enforcement/      Hooks and validation scripts
├── active/           Active plans (created at runtime)
└── archive/          Completed plans
```

## Further Reading

| Document | Content |
|----------|---------|
| `docs/ARCHITECTURE.md` | System design goals and component overview |
| `docs/IMPLEMENTATION_ROADMAP.md` | Version-by-version build history (V1a through V5) |
| `docs/WORKFLOW_SCHEMA.md` | Complete YAML specification |
| `docs/STATE_SCHEMA.md` | `state.json` field definitions |
| `docs/ADVERSARY_SYSTEM.md` | Plan review design |
| `docs/PLANNING_PROCESS.md` | How to write phase plans |
| `docs/INTERVENTION_SYSTEM.md` | Escape hatches and overrides |
| `docs/V4_FEATURES.md` | Hooks, constraints, and escalation details |
