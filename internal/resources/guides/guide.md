# Arc Reference Guide

Arc is a workflow engine for orchestrating complex, multi-phase software engineering tasks through AI agents. It breaks large work into phases, drives each phase through a state machine, and enforces rules so agents stay on track.

## Core Concepts

Arc uses a three-level hierarchy:

- **Plan** — The overall work request (e.g., `fix-wasm-rng`). Contains one or more phases.
- **Phase** — A self-contained unit of work with its own state machine, test suite, and `state.json`.
- **State** — The current position in a workflow's state machine (e.g., `impl.act`, `check.adversary`).

The execution flow is: `arc plan` → `arc review` → `arc run`.

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

This creates a `.arc.yaml` configuration file in your project root. The file controls how Arc behaves for this project.

### `.arc.yaml` Fields

```yaml
project:
  name: my-project              # Project name
  root: .                       # Project root (relative to config)

workflows:
  dir: .arc/workflows           # Custom workflow overrides (optional)

plans:
  dir: .plans/active            # Where active plans live

settings:
  max_iterations: 50            # Max iterations per phase before blocking
  claude_model: sonnet          # Model for sub-agents
```

The plans directory (`.plans/active/`) is where all plan state lives. Each plan gets a subdirectory containing phase directories with `plan.md` and `state.json` files.

<!-- /section: setup -->

<!-- section: plans -->

## Writing Plans

### Creating a Plan

```bash
arc plan <plan-name> <phase1> [phase2] ...
arc plan --type bugfix <plan-name> <phase1> [phase2] ...
```

This scaffolds a plan directory with a `plan.md` file for each phase. You then fill in each `plan.md` with the detailed specification.

### Plan Structure

Every phase's `plan.md` must contain these sections:

```markdown
# Phase: [PHASE_NAME]

## Objective
One sentence describing what this phase accomplishes.

## Files

### Create
- `path/to/new_file.go` — brief description

### Modify
- `path/to/existing.go` — what changes

## Types and Signatures
Complete, exact signatures. No pseudocode.

## Error Types
Full error enums/types with all variants.

## Dependencies
External dependency additions with exact versions.

## DO NOT
- [ ] Do NOT modify files outside the scope listed above
- [ ] Do NOT use panic/unwrap — propagate errors properly
- [ ] Do NOT [common mistake specific to this phase]

## Test Cases

### test_name_1
**Input:**
    let input = SomeStruct { field: value };
**Expected:** `function(&input)` returns `Ok(expected_value)`

## Edge Cases
1. **Empty collections** — valid or error?
2. **Null/None fields** — which fields can be None, behavior when None
3. **Boundary values** — max/min values, behavior at boundaries

## Integration Points

### Consumed by
- Phase XX: how it uses this phase's output

### Depends on
- Phase ZZ: what this phase needs from previous phases

### Exports
- `TypeName` — used by consumer
- `function_name` — called by consumer
```

### Rules for Good Plans

1. **Explore the codebase first.** Read the actual code before proposing anything. Understand existing patterns, naming conventions, and module structure.

2. **Be concrete, not vague.** Every function signature, error type, struct field, and test value must be exact. Never write "appropriate error handling" — write `ErrInvalidInput{Field: "name", Reason: "empty"}`.

3. **Write test cases with real values.** Not "some input" — actual values with actual expected outputs. An agent cannot implement a test from "it should handle edge cases correctly."

4. **Keep phases small.** Each phase should be completable in roughly 15 agent iterations. If a phase touches more than 10 files or 3 packages, split it.

5. **Specify the DO NOT section.** Every phase has likely mistakes an agent will make. Anticipate them. Common entries: don't modify out-of-scope files, don't add unnecessary dependencies, don't use deprecated APIs.

6. **Document integration points.** If phase B depends on types from phase A, say exactly which types and how they're used.

### Plan Quality Checklist

- [ ] Every struct/enum has exact field definitions with types
- [ ] Every function has full signature with generics and bounds
- [ ] Every error variant has specific message format
- [ ] File paths are explicit (not "somewhere in the module")
- [ ] DO NOT section covers likely mistakes
- [ ] Test cases have concrete inputs and expected outputs
- [ ] Edge cases are enumerated
- [ ] Integration points are documented

<!-- /section: plans -->

<!-- section: workflows -->

## Workflow Types

Arc provides these built-in workflow types. Each defines a state machine that controls how a phase progresses.

### feature

Implement a feature, then harden it with a one-shot adversarial review. The implementation step handles both initial development (with its own tests) and subsequent bug fixes after the adversary reports failures. The adversary runs exactly once (`run_once`) — if bugs are found and fixed, it auto-produces `no_bugs_found` on re-entry rather than re-running.

```
impl.act → check.adversary → no_bugs_found → complete
                ↓
            bugs_found
                ↓
            impl.act (re-entry: fix adversary tests)
                ↓
      check.adversary (auto-skip → no_bugs_found)
                ↓
             complete
```

| State | Purpose |
|-------|---------|
| `impl.act` | First entry: write implementation and tests. Re-entry after `bugs_found`: fix implementation to pass adversary tests (no test modifications). |
| `check.adversary` | Adversarially review code, write failing tests to prove bugs. Runs once — auto-skips with `no_bugs_found` on re-entry. |
| `complete` | Phase done |
| `blocked` | Requires human intervention |

**When to use:** Adding new functions, types, modules, commands, APIs, or any net-new capability.

**TDD variant (example pipeline):** If you want tests written before implementation, compose a custom workflow using the `qa` and `act` blocks with your own prompts:

```yaml
name: feature-tdd
version: 1
pipeline:
  - block: qa
    name: tests
    params:
      max_turns: "100"
  - block: act
    name: impl
    params:
      prompt: "prompts/my-project/impl.md"
      max_turns: "200"
  - block: adversary
    name: check
    run_once: true
    skip_exit: no_bugs_found
    params: {max_turns: "30"}
    route:
      bugs_found: impl
      no_bugs_found: complete
terminal_states: [complete, blocked]
```

### bugfix

Fix incorrect behavior. Linear workflow: reproduce, test, fix.

```
investigate → regression_tests → test_review → fix → fix_review → complete
```

| State | Purpose |
|-------|---------|
| `investigate` | Understand current behavior and identify root cause |
| `regression_tests` | Write tests that define correct behavior (should fail before fix) |
| `test_review` | Review regression test coverage |
| `fix` | Implement the fix |
| `fix_review` | Review fix implementation |

**When to use:** Fixing a bug where the current behavior differs from the expected behavior.

### investigation

Research and produce findings. No code changes — output is documentation only.

```
research → draft → review → complete
```

| State | Purpose |
|-------|---------|
| `research` | Examine codebase and gather information |
| `draft` | Write findings document |
| `review` | Review findings for completeness |

**When to use:** Understanding how something works, evaluating options, auditing code, answering technical questions.

### refactor

Change code structure without changing behavior. Characterization tests prove behavior is preserved.

```
characterize → char_review → refactor → verify → complete
```

| State | Purpose |
|-------|---------|
| `characterize` | Write tests capturing current behavior |
| `char_review` | Review characterization tests |
| `refactor` | Perform structural changes |
| `verify` | Verify all characterization tests still pass |

**When to use:** Restructuring modules, renaming, extracting abstractions, consolidating duplicated code.

### performance

Optimize code with benchmarks as the success criteria.

```
baseline → analyze → optimize → benchmark → complete
```

| State | Purpose |
|-------|---------|
| `baseline` | Establish performance baseline measurements |
| `analyze` | Profile and identify bottlenecks |
| `optimize` | Implement optimizations |
| `benchmark` | Verify improvement and correctness |

**When to use:** Making code faster, reducing memory usage, improving throughput.

### audit

Adversarial loop: adversary finds bugs, impl fixes, repeat until clean.

```
audit (adversary) → bugs_found → fix (impl) → done → audit
                  → no_bugs_found → complete
```

| State | Purpose |
|-------|---------|
| `audit` | Write adversarial tests to find bugs in existing code |
| `fix` | Fix the bugs found by the adversary |

**When to use:** Hardening existing code against edge cases and bugs without a known defect to fix.

### Terminal States

All workflows share two terminal states:

- **`complete`** — Phase finished successfully.
- **`blocked`** — Phase cannot proceed without human intervention.

### Custom Pipeline Workflows

Block-composed workflows use a `pipeline:` key instead of `states:`. Each pipeline step references a block by type. Steps can be named and can route individual block exits to specific downstream steps.

```yaml
name: my-workflow
version: 1
pipeline:
  - block: act
    name: write-code          # optional; defaults to block name; used as routing target
    params: {max_turns: "45", prompt: "prompts/my-project/impl.md"}
  - block: adversary
    name: check
    params: {max_turns: "30"}
    route:
      bugs_found: write-code  # route exit to named step (loops back)
      no_bugs_found: complete  # route exit to terminal state
terminal_states: [complete, blocked]
```

**`name`** — gives the step an addressable identity used as the namespace prefix for its states (`write-code.act`) and as a target in `route` maps.

**`route`** — maps individual block exit names to downstream steps or terminal states. Exits not listed in `route` fall through to the next sequential step.

**`params`** — override block defaults. All blocks support a `prompt` param to swap the agent prompt:

```yaml
- block: act
  params:
    prompt: "prompts/my-project/impl.md"
    max_turns: "30"
```

**`run_once`** / **`skip_exit`** — limits a block to a single execution. On the first visit the block runs normally. On all subsequent visits the pipeline auto-produces the `skip_exit` verdict without spawning an agent, then advances. Useful for adversarial steps that should only get one pass:

```yaml
- block: adversary
  name: check
  run_once: true
  skip_exit: no_bugs_found
  params: {max_turns: "30"}
  route:
    bugs_found: impl
    no_bugs_found: complete
```

### Block Selection Guide

Pick blocks by **purpose**, not by verdict shape. Matching verdict names is not enough — using a block outside its domain produces semantically wrong workflows.

| Block | Purpose | Verdicts | Do NOT use for |
|-------|---------|----------|----------------|
| `act` | Free-form implementation, any coding task | `done` (linear) | Writing tests — use `tests` |
| `tests` | Writing tests specifically | `done` (linear) | General implementation — use `act` |
| `test-review` | Reviewing **test** quality | `approved`, `gaps_found` | Code review, document review, any non-test domain |
| `review` | Reviewing **implementation/code** quality | `approved`, `concerns` | Test review, document review, any non-code domain |
| `judge` | Generic branching with custom verdicts | configurable | Nothing — this is the default when no other block fits |
| `adversary` | Adversarially finding bugs by writing failing tests | `bugs_found`, `no_bugs_found` | Quality review — use `review` or `judge` |

**Default rule:** if you need branching and the domain doesn't clearly match `test-review` or `review`, use `judge` with parameterized verdicts. Never pick a block solely because its exit names happen to match what you need.

### Writing Custom Prompts

When writing a prompt for a custom pipeline, use params for any branching logic — never rely on `{{state.last_verdict}}` or other implicit workflow state:

```markdown
{{#if params.fix_mode}}
Fix the implementation to make the failing tests pass. Do not modify test files.
{{else}}
Write the implementation and tests from scratch.
{{/if}}
```

The calling pipeline step sets the param explicitly:

```yaml
- block: act
  name: fix
  params:
    prompt: "prompts/my-project/impl.md"
    fix_mode: "true"
```

**Premade workflow prompts** (under `prompts/feature/`, `prompts/bugfix/`, etc.) are owned by their workflow and rely on workflow-internal state. Do not reference them from custom pipelines — write your own prompt instead.

### Available Blocks

All blocks support a `prompt` param to swap the agent prompt and a `max_turns` param to control agent length.

| Block | Exits | Params | Purpose |
|-------|-------|--------|---------|
| `act` | `done` | `prompt`, `max_turns` (45), `timeout` (900), `model` | Generic linear work — does something and exits unconditionally |
| `adversary` | `bugs_found`, `no_bugs_found` | `prompt`, `max_turns` (30) | Writes adversarial failing tests to expose bugs in existing code |
| `qa` | `done` | `prompt`, `max_turns` (100), `timeout` (1800) | Writes tests capturing intended behavior |
| `qa-review` | `approved`, `gaps_found` | `prompt`, `max_turns` (15), `max_state_iterations`, `on_max_iterations` | Reviews test coverage and quality |
| `review` | `approved`, `concerns` | `prompt`, `max_turns` (50), `timeout` (900) | Reviews implementation quality |
| `judge` | configurable | `prompt`, `max_turns` (15), `verdict_a` (approved), `verdict_b` (rejected), `max_state_iterations`, `on_max_iterations` | Generic two-verdict branching block; override `verdict_a`/`verdict_b` to name your own exits |

**`judge` example** — approve or reject a draft:

```yaml
- block: judge
  name: check
  params:
    prompt: "prompts/my-project/draft-review.md"
    verdict_a: "ship_it"
    verdict_b: "needs_work"
  route:
    needs_work: draft
    ship_it: complete
```

<!-- /section: workflows -->

<!-- section: execution -->

## Execution

### Commands

```bash
arc plan <name> <phase1> [phase2] ...   # Create plan scaffolding
arc review <plan-name>                   # Adversarial review with auto-remediation
arc review <plan-name> --phase <phase>   # Review a single phase
arc run <plan-name>                      # Launch orchestrator for all phases
arc iterate <plan-name> <phase-name>     # Run a single iteration for a phase
arc status <plan-name>                   # Show plan/phase status
arc manage reset-review <plan> <phase>   # Clear review cache and iteration counter
arc chat                                 # Launch interactive Claude session with Arc MCP tools
```

### Adversarial Review

`arc review` validates each phase plan using 5 parallel adversaries with auto-remediation:

| Adversary | Focus | Priority | Blocking |
|-----------|-------|----------|----------|
| **executability** | No blockers that prevent agent execution | 1 (highest) | Yes |
| **consistency** | Types, names, and contracts match across phases | 2 | Yes |
| **coverage** | Every function and error variant has tests | 3 | Yes |
| **ambiguity** | Nothing an agent could misinterpret | 4 | Yes |
| **scope** | Phase isn't too large to execute reliably | 5 (lowest) | No (warning) |

When adversaries find issues, they emit structured suggestions (find-and-replace blocks). The review loop automatically:

1. Parses suggestions from failed adversaries
2. Merges by priority (higher-priority adversary wins conflicts)
3. Applies fixes to `plan.md`
4. Re-reviews until all pass or iteration limit (5) is hit

All blocking adversaries must approve before a plan can run.

### Execution Model

Each phase runs as a single long-lived agent session (up to 200 turns / 3600s for implementation states). The agent works until it reaches a terminal verdict, then exits. The orchestrator reads the verdict from the agent's output, advances the state machine, and starts the next session.

If a session crashes (non-zero exit, no extractable verdict), the orchestrator retries once. If it fails again, the phase is marked `blocked`.

After each session the agent's `## Memory` output section is saved to `{phase-dir}/memory/{state-name}.md`. On re-entry to the same state (e.g., after a review loop sends impl back), that memory is injected into the prompt as `## Previous Run Notes` so the agent picks up where it left off.

### State Tracking

Each phase maintains a `state.json` file:

```json
{
  "phase": "port-pcg",
  "plan": "my-plan",
  "workflow_type": "feature",
  "phase_status": "in_progress",
  "current_state": "impl.act",
  "iteration": {"current": 2, "max": 25},
  "tests_passing": 8,
  "tests_total": 12,
  "last_verdict": "done",
  "verdicts_history": [
    {"iteration": 1, "state": "impl.act", "verdict": "done", "timestamp": "2025-01-01T00:00:00Z"}
  ],
  "notes": "Completed core types, working on error handling"
}
```

Use `arc manage <plan> <phase> show` to print a phase's current `state.json`.

### Arc Chat (MCP Mode)

`arc chat` launches an interactive Claude session with Arc registered as an MCP server. In this mode, Arc tools are available as MCP tool calls — use these instead of CLI commands.

**Call `arc_guide` first** if you're unsure of any Arc convention. The guide is authoritative.

| MCP Tool | CLI Equivalent | Purpose |
|----------|---------------|---------|
| `arc_guide` | `arc guide` | Print Arc reference guide (call this first if unsure) |
| `arc_list_plans` | `arc status` | List all active plans |
| `arc_status` | `arc status <plan>` | Show phases and state for a specific plan |
| `arc_plan` | `arc plan` | Create a new plan with phases |
| `arc_review` | `arc review` | Run adversarial review on a plan |
| `arc_run` | `arc run` | Launch orchestrator (runs async, returns immediately) |
| `arc_run_status` | — | Check progress of a running orchestrator |
| `arc_run_cancel` | — | Cancel a running orchestrator |
| `arc_iterate` | `arc iterate` | Run a single phase iteration |
| `arc_manage` | `arc manage` | Update phase state (complete, block, note, tests, etc.) |
| `arc_archive` | `arc archive` | Archive a completed plan |

`arc_run` is **asynchronous** — it returns immediately and the orchestrator runs in the background. Poll `arc_run_status` to check progress. While a run is in progress, you can do other work: plan the next task, answer questions, explore the codebase.

<!-- /section: execution -->

<!-- section: mistakes -->

## Common Mistakes

### Vague Specifications

**Wrong:** "Handle errors appropriately."
**Right:** "Return `ErrNotFound{ID: id}` when the record doesn't exist. Return `ErrPermission{User: u, Action: a}` when the user lacks access."

Agents execute literally. If the spec says "handle errors," the agent will add a generic `if err != nil { return err }` that tells you nothing.

### Missing Test Cases

**Wrong:** "Test the happy path and edge cases."
**Right:**
```
### test_parse_valid_input
Input: `Parse("2024-01-15")`
Expected: `Date{Year: 2024, Month: 1, Day: 15}, nil`

### test_parse_invalid_month
Input: `Parse("2024-13-01")`
Expected: `nil, ErrInvalidMonth{Value: 13}`
```

Every test case needs a name, concrete input, and concrete expected output.

### Phases Too Large

**Wrong:** One phase that modifies 20 files across 5 packages.
**Right:** Split into 3-4 phases of 5-7 files each, with clear dependencies.

Large phases hit the iteration limit before completing. The agent loses context, repeats mistakes, and gets stuck. Aim for 15 iterations or fewer per phase.

### Pseudocode Instead of Real Signatures

**Wrong:** "A function that takes user data and returns the processed result."
**Right:** `func ProcessUser(ctx context.Context, u *User) (*Result, error)`

Agents need exact function names, parameter types, and return types. Pseudocode forces the agent to guess, leading to inconsistencies across phases.

### Forgetting the DO NOT Section

Every phase has predictable failure modes. Without a DO NOT section, agents will:
- Modify files outside scope, breaking other phases
- Add unnecessary dependencies
- Use deprecated or disallowed APIs
- Change public interfaces that other phases depend on

### Ignoring Integration Points

When phase B depends on types from phase A, both plans must agree on exact type names, field names, and method signatures. Mismatches cause phase B to fail with confusing errors.

<!-- /section: mistakes -->
