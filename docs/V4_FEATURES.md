# V4: Hooks + Escalation

## Overview

V4 adds automated actions, constraint enforcement, and human escalation
to the orchestration system. These features integrate into the `run_iteration`
pipeline in `iterate.sh`, executing in a defined 8-step order.

## Features

### 1. Constraints

Enforce rules before and after state execution.

```yaml
states:
  - name: impl
    constraints:
      max_iterations: 15              # Stop after 15 iterations
      require_artifacts_in:            # Must exist before
        - qa_reasoning.md
      require_artifacts_out:           # Must exist after
        - impl_reasoning.md
```

**Pre-constraints** (`require_artifacts_in`, `max_iterations`) are checked at step 3.
If they fail, the agent is never spawned and the iteration exits with code 1.

**Post-constraints** (`require_artifacts_out`) are checked at step 6.
If they fail, state is NOT updated and the iteration exits with code 1.

Artifact paths are relative to `PHASE_DIR`.

### 2. After Hooks

Execute actions after state completion.

```yaml
states:
  - name: impl
    after:
      - action: run_tests
        params:
          pattern: "qa_"
      - action: commit
        when: approved                 # Only on this verdict
        params:
          message: "feat: impl"
      - action: script
        continue_on_error: true        # Don't stop chain on failure
        params:
          path: "scripts/custom.sh"
```

Hooks execute in the order defined in the workflow. If a hook fails:
- With `continue_on_error: true`: the next hook still runs
- Without `continue_on_error` (default `false`): the chain stops and the iteration returns 1

#### When Conditions

The `when` field supports three syntaxes:

| Syntax | Example | Meaning |
|--------|---------|---------|
| Exact match | `when: approved` | Run only if verdict is "approved" |
| Negation | `when: "!approved"` | Run only if verdict is NOT "approved" |
| OR condition | `when: "approved\|passed"` | Run if verdict is "approved" OR "passed" |

### 3. Escalation Triggers

Execute actions at specific iteration counts.

```yaml
states:
  - name: impl
    escalation:
      - at_iteration: 3
        action: analyze_stuck
      - at_iteration: 5
        action: switch_model
        params:
          model: opus
      - after_iteration: 7           # Once, not repeated
        action: request_human
        params:
          message: "Stuck beyond iteration 7"
      - every_n_iterations: 2        # Repeats on modulo match
        action: analyze_stuck
```

#### Trigger Types

| Type | Behavior |
|------|----------|
| `at_iteration: N` | Fires when `iteration == N` exactly |
| `after_iteration: N` | Fires once when `iteration > N`, tracked in `executed_escalations` |
| `every_n_iterations: N` | Fires when `iteration % N == 0` (repeating) |

Only the **first matching trigger** executes per iteration. If multiple triggers match, the one defined first in the YAML takes precedence.

`after_iteration` triggers are tracked in the `executed_escalations` array in `state.json` to prevent re-firing across iterations.

### 4. Intervention Triggers

Request human help when conditions are met.

```yaml
intervention_triggers:
  - condition: "stuck_iterations >= 5"
    action: request_human
    message: "Phase stuck for 5+ iterations"
```

Intervention triggers are checked at **step 1** (before anything else). If a trigger fires:
- `action_request_human` creates `intervention_request.md` in the phase directory
- `state.json` gets an `intervention_request` object (not a boolean)
- The pipeline halts with exit code 2
- No agent is spawned, no hooks run, no state is updated

If an `intervention_request` already exists in state (from a previous trigger), the pipeline halts immediately with exit code 2 without re-triggering.

#### Condition Syntax

Conditions follow the format: `<field> <operator> <value>`

| Operator | Example |
|----------|---------|
| `>=` | `stuck_iterations >= 3` |
| `<=` | `tests_passing <= 0` |
| `>` | `iteration > 10` |
| `<` | `tests_total < 5` |
| `==` | `iteration == 0` |
| `!=` | `stuck_iterations != 0` |

Left operand is a field name from `state.json`. Missing fields default to `0`.
Right operand is a numeric literal.

## Actions

Available actions for hooks, escalation, and intervention:

| Action | Description | Parameters |
|--------|-------------|------------|
| `run_tests` | Execute `cargo nextest run` | `pattern`, `save_to`, `expect_failure` |
| `commit` | Create git commit | `message`, `when` (verdict condition) |
| `switch_model` | Change Claude model | `model` (opus/sonnet/haiku) |
| `analyze_stuck` | Generate stuck analysis document | (none) |
| `request_human` | Request human intervention | `message` |
| `script` | Run custom script | `path` (relative to scripts/), `args...` |

### Action Details

**`run_tests`**: Runs the configured test runner with the given pattern`. Updates `tests_total` and `tests_passing` in state. With `expect_failure: true`, succeeds if tests fail (useful for regression test setup).

**`commit`**: Runs `git add -A && git commit`. Checks `allow_commits` in state. Supports `when` verdict condition.

**`switch_model`**: Updates `current_model` in state.json. Valid values: `opus`, `sonnet`, `haiku`.

**`analyze_stuck`**: Creates `stuck_analysis.md` in the phase directory with iteration history, test failure patterns, and recommendations. Requires at least 2 iterations of history.

**`request_human`**: Creates `intervention_request.md` and sets `intervention_request` object in state with `{reason, requested_at, options}`.

**`script`**: Runs a script from `$ARC_HOME/scripts/`. Path must not contain `../` or be absolute. Script must exist and be executable.

## Execution Order

Each iteration follows this order:

1. **Check intervention triggers** (may halt with exit 2)
2. **Check escalation triggers** (may run action, continues on success)
3. **Check pre-constraints** (may fail with exit 1)
4. **Render prompt and spawn agent** (creates `iteration_NNN/` directory)
5. **Extract verdict** (if state has `verdicts` field)
6. **Check post-constraints** (may fail with exit 1)
7. **Run after hooks** (may run actions, verdict-conditional)
8. **Update state** (increment iteration, resolve next_state, track stuck)

If a step fails, subsequent steps are skipped, with one exception: **agent failure** (step 4) does NOT skip steps 5-7. Post-constraints and hooks still run after agent failure, but the final exit code reflects the agent failure.

## Exit Codes

| Code | Meaning | State Updated? |
|------|---------|----------------|
| **0** | Iteration completed successfully | Yes |
| **1** | Iteration failed (constraint, action, or agent failure) | No |
| **2** | Human intervention requested | No (intervention_request set) |

## State Updates (Step 8)

After a successful iteration, `update_state_after_iteration` performs:

1. **Increment iteration**: `iteration = iteration + 1`
2. **Resolve next_state**: From `.next` (string) or `.next[verdict]` (object)
3. **Track stuck_iterations**:
   - If `next_state == current_state` (loop): increment `stuck_iterations`
   - If `next_state != current_state` (transition): reset to 0
4. **Append to verdicts_history**: `{iteration, state, verdict, timestamp}` (if verdict present)
5. **Set last_verdict**: Latest verdict value

All state updates use atomic `tmp.$$` + `mv` pattern to prevent corruption.

## Backwards Compatibility

V4 features are additive. Workflows without V4 sections work normally:
- No `constraints` → all constraint checks return 0
- No `after` → no hooks execute
- No `escalation` → no escalation triggers
- No `intervention_triggers` → no intervention checks
- V1, V2, and V3 workflows run through the same pipeline with all V4 checks passing immediately

V3 template rendering (`build-context.sh` + `render_template.py`) works alongside V4 hooks. If V3 scripts are available, prompts are rendered with context; otherwise, prompt files are copied directly.

## Implementation Files

| File | Purpose |
|------|---------|
| `scripts/iterate.sh` | Main pipeline with `run_iteration` and `update_state_after_iteration` |
| `scripts/actions.sh` | Action function implementations |
| `scripts/check-constraints.sh` | Pre/post constraint validation |
| `scripts/check-escalation.sh` | Escalation trigger evaluation |
| `scripts/check-intervention.sh` | Intervention trigger evaluation |
| `scripts/run-hooks.sh` | After hook execution |
| `scripts/extract-verdict.sh` | Verdict extraction from review output |

## Testing

```bash
# Run V4 component integration tests
bats $ARC_HOME/tests/v4_integration.bats

# Run V4 end-to-end pipeline tests
bats $ARC_HOME/tests/v4_e2e.bats
```
