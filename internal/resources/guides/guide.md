# Arc Reference Guide

Arc is a workflow engine for orchestrating complex, multi-phase software engineering tasks through AI agents. It breaks large work into phases, drives each phase through a state machine, and enforces rules so agents stay on track.

## Core Concepts

Arc uses a three-level hierarchy:

- **Plan** — The overall work request (e.g., `fix-wasm-rng`). Contains one or more phases.
- **Phase** — A self-contained unit of work with its own state machine, test suite, and `state.json`.
- **State** — The current position in a workflow's state machine (e.g., `qa`, `impl_review`).

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

Implement a feature, then harden it with adversarial review. The implementation step handles both initial development (with its own tests) and subsequent bug fixes after the adversary reports failures.

```
impl → check (adversary) → no_bugs_found → complete
              ↓
          bugs_found
              ↓
            impl
```

| State | Purpose |
|-------|---------|
| `impl` | Implement the feature with tests; on re-entry, fix bugs found by the adversary |
| `check` | Write adversarial tests to find edge cases and bugs |
| `complete` | Phase done |
| `blocked` | Requires human intervention |

**When to use:** Adding new functions, types, modules, commands, APIs, or any net-new capability.

**TDD variant (example pipeline):** If you want tests written before implementation, compose a custom workflow:

```yaml
name: feature-tdd
version: 1
pipeline:
  - block: act
    name: qa
    params:
      prompt: "prompts/feature/qa.md"
      max_turns: "100"
  - block: act
    name: impl
    params:
      prompt: "prompts/feature/impl.md"
      max_turns: "200"
  - block: adversary
    name: check
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
  - block: impl
    name: write-code          # optional; defaults to block name; used as routing target
    params: {max_turns: "45", prompt: "prompts/feature/impl.md"}
  - block: adversary
    name: check
    params: {max_turns: "30"}
    route:
      bugs_found: write-code  # route exit to named step (loops back)
      no_bugs_found: complete  # route exit to terminal state
terminal_states: [complete, blocked]
```

**`name`** — gives the step an addressable identity used as the namespace prefix for its states (`write-code.impl`) and as a target in `route` maps.

**`route`** — maps individual block exit names to downstream steps or terminal states. Exits not listed in `route` fall through to the next sequential step.

**`params`** — override block defaults. All blocks support a `prompt` param to swap the agent prompt:

```yaml
- block: impl
  params:
    prompt: "prompts/bugfix/fix.md"
    max_turns: "30"
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
arc manage reset-review <plan> <phase>   # Clear review cache and iteration counter (run from project root)
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

### The Iteration Pipeline

Each iteration of `arc run` or `arc iterate` executes an 8-step pipeline:

1. **Check intervention** — Stop if a human has flagged the phase
2. **Check escalation** — Trigger stuck detection if iterations are stalling
3. **Check pre-constraints** — Validate preconditions before spawning an agent
4. **Render prompt and spawn agent** — Build the prompt from state + plan and run a sub-agent
5. **Extract verdict** — Parse the agent's output for a state-machine verdict
6. **Check post-constraints** — Validate postconditions after the agent runs
7. **Run after-hooks** — Execute any registered post-iteration hooks
8. **Update state** — Write the new state to `state.json`

### State Tracking

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

### Stuck Detection and Escalation

When a phase makes no progress across multiple iterations (same state, same test results), the orchestrator triggers escalation. This can:

- Analyze what's stuck and adjust the approach
- Escalate to a different agent strategy
- Eventually mark the phase as `blocked` for human intervention

The `stuck_iterations` counter tracks consecutive non-progress iterations. The `escalation_history` records actions taken.

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
