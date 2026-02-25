# Workflow Schema Specification

## Overview

Workflows are YAML files that define the state machine for a type of work. They specify:
- What states exist
- How to transition between states
- What prompts to use
- What constraints to enforce
- What actions to run

## Complete Schema (V4 Target)

```yaml
# Top-level fields
name: string                    # Required. Workflow identifier
version: integer                # Required. Schema version (1-4)
extends: string | null          # Optional. Parent workflow to inherit from
description: string             # Optional. Human-readable description

# Default values applied to all states
defaults:
  max_iterations: integer       # Default: 10
  timeout: integer              # Default: 600 (seconds)
  model: string                 # Default: "sonnet"

# Template variables available in prompts
variables:
  custom_var: "{{source.path}}" # Define custom variables

# State definitions
states:
  - name: string                # Required. Unique state identifier
    description: string         # Optional. Human-readable description
    prompt: string              # Path to prompt template (required unless parallel)

    # V3: Parameters passed to prompt template
    params:
      key: value                # Arbitrary key-value pairs

    # V4: Constraints enforced by scripts
    constraints:
      max_iterations: integer   # Override default for this state
      require_artifacts_in:     # Must exist before state runs (relative to phase_dir)
        - path/to/artifact.md
      require_artifacts_out:    # Must exist after state completes (relative to phase_dir)
        - path/to/output.md

    # V4: Actions run after state completes
    after:
      - action: string          # Action name (see Action Registry)
        params:                 # Action-specific parameters
          key: value
        when: string            # Optional. Only run if verdict matches

    # V4: Escalation behavior when stuck
    escalation:
      - at_iteration: integer   # Trigger at this iteration count
        action: string          # Action to take
        params:
          key: value

    # V3: Context sources for prompt injection
    context_sources:
      - source: string          # Variable name in template
        file: string            # Path to file (supports {{variables}})
        optional: boolean       # Default: false. If true, skip when file missing
        extract: string         # Optional regex to extract subset of file
        limit: integer          # Optional max lines/matches to include
        parse: string           # Optional parser (attempts_list, json, etc.)
        condition: string       # Optional condition expression

    # V2: Verdicts this state can produce
    verdicts:
      - verdict_name            # List of valid verdict strings

    # Transition definition
    next: string | object       # V1: string (linear), V2: object (branching)
    # As string: next: state_name
    # As object:
    #   next:
    #     verdict_a: state_name_1
    #     verdict_b: state_name_2

    # V5: Parallel execution
    parallel:
      strategy: all | any | n_of_m  # How to join branches
      n: integer                    # For n_of_m strategy
      branches:
        - name: string
          prompt: string
          params: object

    # Positioning for inherited workflows
    insert_after: string        # Insert this state after named state
    insert_before: string       # Insert this state before named state

# Entry point
entry_state: string             # Required. First state to execute

# Terminal states
terminal_states:
  - name: string                # State name
    description: string         # Optional description
  # Or simple list:
  # - complete
  # - blocked

# V4: Intervention triggers
intervention_triggers:
  - condition: string           # Expression evaluated against state
    action: request_human       # Currently only request_human
    message: string             # Message to display
```

## Path Conventions

All paths in the orchestration system follow these conventions:

### Base Paths

| Variable | Resolves To |
|----------|-------------|
| `ARC_HOME` | `$ARC_HOME/` (from project root) |
| `PLAN_DIR` | `.plans/active/<plan>/` |
| `PHASE_DIR` | `.plans/active/<plan>/phases/<phase>/` |

### Path Types in Workflow Files

| Context | Relative To | Example |
|---------|-------------|---------|
| `prompt:` | `ARC_HOME` | `prompts/feature/impl.md` → `$ARC_HOME/prompts/feature/impl.md` |
| `require_artifacts_in/out:` | `PHASE_DIR` | `impl_reasoning.md` → `.plans/active/<plan>/phases/<phase>/impl_reasoning.md` |
| `extends:` | `ARC_HOME/workflows/` | `bugfix` → `$ARC_HOME/workflows/bugfix.yaml` |
| `context_sources.file:` | `PHASE_DIR` (unless prefixed) | `orchestrator_notes.md` → `PHASE_DIR/orchestrator_notes.md` |

### Special Prefixes

| Prefix | Meaning | Example |
|--------|---------|---------|
| `{{plan_dir}}/` | Relative to plan directory | `{{plan_dir}}/manifest.yaml` |
| `{{phase_dir}}/` | Relative to phase directory | `{{phase_dir}}/output.txt` |
| `/` or `./` | Relative to project root | `./src/lib.rs` |

### Example Resolution

```yaml
# In workflow.yaml
states:
  - name: impl
    prompt: prompts/feature/impl.md          # → $ARC_HOME/prompts/feature/impl.md
    constraints:
      require_artifacts_in:
        - qa_reasoning.md                     # → .plans/active/<plan>/phases/<phase>/qa_reasoning.md
      require_artifacts_out:
        - impl_reasoning.md                   # → .plans/active/<plan>/phases/<phase>/impl_reasoning.md
    context_sources:
      - source: notes
        file: orchestrator_notes.md           # → PHASE_DIR/orchestrator_notes.md
      - source: manifest
        file: "{{plan_dir}}/manifest.yaml"    # → PLAN_DIR/manifest.yaml
```

---

## Action Registry

Built-in actions available in `after` and `escalation`:

| Action | Description | Parameters | Implemented In |
|--------|-------------|------------|----------------|
| `run_tests` | Execute tests | `pattern`, `expect_failure`, `save_to` | V1b |
| `save_output` | Save last command output | `to` (file path, relative to PHASE_DIR) | V1b |
| `commit` | Git commit | `message`, `when` (verdict condition) | V4 |
| `require_artifact` | Verify file exists, fail if missing | `path` (relative to PHASE_DIR) | V4 |
| `analyze_stuck` | Generate stuck analysis document | none | V4 |
| `switch_model` | Change AI model for next iteration | `model` (opus, sonnet, haiku) | V4 |
| `attempt_split` | Try to auto-split phase | none | V4 |
| `script` | Run custom script | `path` (relative to scripts/), `args` | V4 |
| `request_human` | Stop and request human intervention | `message` | V4 |

### Action Implementations

```bash
# $ARC_HOME/scripts/actions.sh

action_run_tests() {
    local pattern="$1"
    local expect_failure="${2:-false}"
    local save_to="${3:-test_output.txt}"

    local output
    output=$("$ARC_RUNNER_DIR/run.sh" "$PHASE_DIR" "$pattern" 2>&1) || true
    echo "$output" > "$PHASE_DIR/$save_to"

    local has_failures=$(echo "$output" | grep -c "FAILED" || true)

    if [[ "$expect_failure" == "true" ]]; then
        # We WANT failures (regression test setup)
        [[ $has_failures -gt 0 ]] && return 0 || return 1
    else
        # We want all passing
        [[ $has_failures -eq 0 ]] && return 0 || return 1
    fi
}

action_save_output() {
    local to="$1"
    # Save the output from the last agent run
    local last_iter=$(printf '%03d' "$(jq '.iteration' "$STATE_FILE")")
    cp "$PHASE_DIR/iteration_$last_iter/output.txt" "$PHASE_DIR/$to"
}

action_commit() {
    local message="$1"
    local when="${2:-}"

    # Check verdict condition
    if [[ -n "$when" && "$LAST_VERDICT" != "$when" ]]; then
        return 0  # Skip, condition not met
    fi

    # Render message template
    message=$(echo "$message" | sed "s/{{phase}}/$PHASE/g; s/{{plan}}/$PLAN/g")

    git add -A
    git commit -m "$message" || true  # May fail if nothing to commit
}

action_require_artifact() {
    local path="$1"
    local full_path="$PHASE_DIR/$path"

    if [[ ! -f "$full_path" ]]; then
        echo "ERROR: Required artifact missing: $path" >&2
        return 1
    fi
}

action_analyze_stuck() {
    # Generate stuck analysis document
    cat > "$PHASE_DIR/stuck_analysis.md" << EOF
# Stuck Analysis: $PHASE

## Iteration History
$(for i in "$PHASE_DIR"/iteration_*/; do
    iter=$(basename "$i")
    echo "### $iter"
    head -50 "$i/output.txt" 2>/dev/null || echo "(no output)"
    echo ""
done)

## Test Failure Patterns
$(grep -h "FAILED\|error\|panic" "$PHASE_DIR"/iteration_*/test_output.txt 2>/dev/null | sort | uniq -c | sort -rn | head -20)

## Recommendations
Review the patterns above. Consider:
1. Is there a fundamental misunderstanding of the requirements?
2. Is a test expectation incorrect?
3. Is there a dependency issue?
EOF
}

action_switch_model() {
    local model="$1"
    jq --arg model "$model" '.current_model = $model' "$STATE_FILE" > tmp && mv tmp "$STATE_FILE"
    echo "Switched to model: $model"
}

action_attempt_split() {
    # Trigger the phase splitter
    "$ARC_HOME/scripts/split-phase.sh" "$PLAN" "$PHASE"
}

action_script() {
    local path="$1"
    shift
    local args=("$@")

    local script_path="$ARC_HOME/scripts/$path"
    if [[ ! -x "$script_path" ]]; then
        echo "ERROR: Script not found or not executable: $path" >&2
        return 1
    fi

    "$script_path" "${args[@]}"
}

action_request_human() {
    local message="$1"

    # Update state to awaiting_human
    jq --arg msg "$message" '
        .previous_state = .current_state |
        .current_state = "awaiting_human" |
        .intervention_request = {
            reason: $msg,
            requested_at: (now | todate)
        }
    ' "$STATE_FILE" > tmp && mv tmp "$STATE_FILE"

    echo "HUMAN INTERVENTION REQUIRED: $message"
    return 1  # Stop execution
}
```

## Template Variables

Variables available in prompt templates:

### Built-in Variables

| Variable | Source | Description |
|----------|--------|-------------|
| `{{phase}}` | state.json | Current phase name |
| `{{phase_dir}}` | computed | Path to phase directory |
| `{{plan}}` | state.json | Plan name |
| `{{plan_dir}}` | computed | Path to plan directory |
| `{{iteration}}` | state.json | Current iteration number |
| `{{max_iterations}}` | workflow | Max iterations for state |
| `{{state_file}}` | computed | Path to state.json |
| `{{workflow_file}}` | computed | Path to workflow.yaml |

### From Plan Frontmatter

| Variable | Source | Description |
|----------|--------|-------------|
| `{{objective}}` | plan.md | Phase objective |
| `{{crates}}` | state.json | Affected crates |

### Context Injection

| Variable | Source | Description |
|----------|--------|-------------|
| `{{orchestrator_notes}}` | orchestrator_notes.md | Notes from orchestrator |
| `{{orchestrator_context}}` | state.json | Context object |
| `{{escalation_active}}` | computed | Boolean if stuck >= 3 |
| `{{stuck_iterations}}` | state.json | Number of stuck iterations |
| `{{recent_test_failures}}` | last_test_output.txt | Extracted failures |
| `{{cleared_disputes}}` | state.json | Recently cleared disputes |

### Conditional Blocks

```markdown
{{#if escalation_active}}
Content shown only during escalation
{{/if}}

{{#each previous_attempts}}
- Iteration {{this.iteration}}: {{this.outcome}}
{{/each}}

{{#unless has_disputes}}
Content shown when no disputes
{{/unless}}
```

## Validation Rules

### Syntax Validation
- Valid YAML
- Required fields present: `name`, `states`, `entry_state`, `terminal_states`
- No placeholder text

### State Validation
- Each state has unique name
- Non-terminal states have `prompt` or `parallel`
- Non-terminal states have `next` transition
- Entry state exists in states list
- Terminal states exist in states list

### Prompt Validation
- Prompt files exist at specified paths
- Template variables are defined or known built-ins

### Graph Validation
- All states reachable from entry state
- All non-terminal states can reach a terminal state
- Cycles have escape conditions (max_iterations or verdict branch)
- Entry state MUST NOT be a terminal state (error if so)

## State Machine Edge Cases

### Entry State Points to Terminal
**Behavior:** Validation error. Entry state must be a non-terminal state.
```
ERROR: entry_state 'complete' is a terminal state. Entry must be non-terminal.
```

### Phase Complete But Dependencies Not Met
**Behavior:** Orchestrator blocks execution until dependencies complete.
```
Phase 'port-pcg' depends on 'investigate' which is in state 'impl' (not complete).
Waiting for dependency to complete...
```

### Workflow Modified Mid-Execution
**Behavior:** Changes take effect on NEXT iteration, not current.
- Current iteration completes with old workflow
- State transition uses new workflow
- If current state no longer exists in new workflow → error, request human intervention

```bash
# In iterate.sh
validate_current_state() {
    local current=$(jq -r '.current_state' "$STATE_FILE")
    local exists=$(yq ".states[] | select(.name == \"$current\")" "$WORKFLOW_FILE")

    if [[ -z "$exists" ]]; then
        error "Current state '$current' not found in workflow. Was workflow modified?"
    fi
}
```

### Concurrent Phase Execution Race Conditions
**Behavior:** Each phase has independent state.json. No shared state between phases.
- Phases can run in parallel without locking
- Orchestrator tracks phase completion via polling state files
- No cross-phase writes except to plan-level manifest.yaml (orchestrator only)

```bash
# Orchestrator polling loop (not phases)
check_phase_status() {
    local phase="$1"
    local state_file="$PLAN_DIR/phases/$phase/state.json"

    if [[ ! -f "$state_file" ]]; then
        echo "pending"
        return
    fi

    jq -r '.current_state' "$state_file"
}
```

### State Transition Failures
**Behavior:** If `get-next-state.sh` returns invalid state, remain in current state and increment stuck_iterations.
```bash
next_state=$(get_next_state "$current" "$verdict")
if ! state_exists "$next_state"; then
    warn "Next state '$next_state' does not exist. Staying in '$current'."
    increment_stuck_iterations
    return 1
fi
```

### Verdict Validation
- All transition verdicts are declared in `verdicts` list
- All declared verdicts have transitions

### Action Validation
- All actions are in registry or are valid custom scripts
- Custom scripts exist and are executable

### Artifact Path Resolution
- Paths in `require_artifacts_in` and `require_artifacts_out` are relative to `phase_dir`
- Example: `findings.md` resolves to `.plans/active/<plan>/phases/<phase>/findings.md`
- Use `{{plan_dir}}/` prefix for plan-level artifacts
- Use absolute paths starting with `$ARC_HOME/` for shared artifacts

## Workflow File Resolution

### File Locations

| Type | Location |
|------|----------|
| Base workflows | `$ARC_HOME/workflows/<type>.yaml` |
| Plan workflow | `.plans/active/<plan>/workflow.yaml` |

### How `extends` Works

When a plan workflow uses `extends: bugfix`:

1. System looks for `$ARC_HOME/workflows/bugfix.yaml`
2. Loads parent workflow
3. Merges child states into parent
4. Applies insertions and overrides
5. Result is used for execution (not written to disk unless --save-merged)

### Precedence

1. Plan-specific workflow (if exists)
2. Type-specific base workflow (from plan.md frontmatter)
3. Default to `feature.yaml`

## Workflow Inheritance

Workflows can extend base workflows using the `extends` field.

### Inheritance Rules

1. **State merging**: Child states are merged with parent states
2. **Same-name override**: If child defines state with same name as parent, child wins
3. **Insertion**: Use `insert_after` or `insert_before` to position new states
4. **No recursive inheritance**: Only one level of extension supported (child → parent)

### Insertion Mechanics

```yaml
# Parent workflow (bugfix.yaml)
states:
  - name: investigate
    next: regression_tests
  - name: regression_tests
    next: test_review
  - name: test_review
    next: fix
  - name: fix
    next: fix_review
  - name: fix_review
    next: complete

# Child workflow (extends bugfix)
extends: bugfix

states:
  - name: start_server
    prompt: prompts/common/start-server.md
    insert_before: regression_tests  # Insert BEFORE regression_tests
    # next is auto-set to regression_tests

  - name: cross_verify
    prompt: prompts/custom/cross-verify.md
    insert_after: fix  # Insert AFTER fix
    # next is auto-set to fix_review (what fix previously pointed to)
```

**Result after merge:**
```
investigate → start_server → regression_tests → test_review → fix → cross_verify → fix_review → complete
```

### Transition Auto-Adjustment

When inserting states:
- `insert_before: X` — The new state's `next` is set to X. The state that previously pointed to X now points to the new state.
- `insert_after: X` — The new state's `next` is set to whatever X pointed to. X's `next` is updated to point to the new state.

### Override Example

```yaml
extends: bugfix

states:
  # Override the fix state with custom parameters
  - name: fix
    prompt: prompts/bugfix/fix.md
    params:
      custom_param: true  # Added parameter
    constraints:
      max_iterations: 20  # Override default
    # Inherits: next, escalation, after from parent (unless specified)
```

### Inheritance Edge Cases

#### Child Redefines entry_state

**Scenario:** Parent has `entry_state: investigate`, child says `entry_state: setup`.

**Behavior:** Child's `entry_state` wins. The child must ensure `setup` state exists (either defined in child or inherited from parent).

```yaml
# Parent (bugfix.yaml)
entry_state: investigate

# Child
extends: bugfix
entry_state: setup  # Child overrides

states:
  - name: setup
    prompt: prompts/custom/setup.md
    insert_before: investigate
```

**Validation:** If child specifies `entry_state` that doesn't exist after merge → error.

#### Same-Name State with Different Verdicts

**Scenario:** Parent's `fix` state has `verdicts: [all_passing, some_failing]`, child redefines `fix` with `verdicts: [pass, fail]`.

**Behavior:** Child's state definition completely replaces parent's. No merging of verdicts.

```yaml
# Parent
states:
  - name: fix
    verdicts: [all_passing, some_failing]
    next:
      all_passing: fix_review
      some_failing: fix

# Child - THIS REPLACES PARENT ENTIRELY
states:
  - name: fix
    verdicts: [pass, fail]  # Different verdicts
    next:
      pass: fix_review
      fail: fix  # Must define all transitions
```

**Validation:** Child must provide complete state definition including `next` transitions.

#### Recursive Inheritance Attempt

**Scenario:** `workflow-a.yaml` extends `workflow-b.yaml`, which extends `workflow-c.yaml`.

**Behavior:** Error at validation time. Recursive inheritance is not supported.

```
ERROR: Recursive inheritance detected.
  workflow-a.yaml extends workflow-b.yaml
  workflow-b.yaml extends workflow-c.yaml
Only one level of inheritance is supported.
```

**Workaround:** Flatten the hierarchy manually or use composition via includes.

#### Insertion Conflict

**Scenario:** Child has two states both with `insert_before: regression_tests`.

**Behavior:** States are inserted in the order they appear in the child's `states` array.

```yaml
states:
  - name: state_a
    insert_before: regression_tests  # Inserted first
  - name: state_b
    insert_before: regression_tests  # Inserted second (before state_a)
```

**Result:** `... → state_b → state_a → regression_tests → ...`

#### Parent State Removal

**Scenario:** Child wants to remove a state that exists in parent.

**Behavior:** Not directly supported. Workarounds:
1. Override the state to immediately transition to next state
2. Create a new workflow without extending

```yaml
# "Remove" investigate by making it a pass-through
states:
  - name: investigate
    prompt: prompts/common/noop.md  # Empty prompt that just exits
    next: regression_tests  # Skip directly to next
```

## Computed Verdicts

Some verdicts are computed automatically rather than extracted from review output.

### Test-Based Verdicts

After a `run_tests` action, the system computes:
- `all_passing` — All tests passed (exit code 0, no FAILED in output)
- `some_failing` — At least one test failed

```yaml
states:
  - name: fix
    after:
      - action: run_tests
        params:
          pattern: "qa_{{phase}}"
    verdicts:
      - all_passing    # Computed from test results
      - some_failing   # Computed from test results
    next:
      all_passing: fix_review
      some_failing: fix
```

### Parallel-Based Verdicts

After parallel execution, the system computes based on strategy:
- `all_complete` — All branches completed successfully
- `any_failed` — At least one branch failed
- `n_complete` — N or more branches completed (for n_of_m strategy)

### Review-Based Verdicts

Extracted from review output by looking for:
```markdown
## Verdict
approved
```

The verdict is normalized (lowercased, whitespace removed) and matched against declared verdicts.

#### Verdict Extraction Algorithm

```bash
extract_verdict() {
    local review_file="$1"
    local valid_verdicts="$2"  # comma-separated: "approved,gaps_found,concerns"

    # Step 1: Remove code blocks (verdicts inside ``` don't count)
    local content=$(sed '/^```/,/^```/d' "$review_file")

    # Step 2: Find "## Verdict" section header
    # Look for the LAST occurrence (final verdict after revisions)
    local verdict_section=$(echo "$content" | \
        grep -A5 -i "^##\s*verdict" | \
        tail -n +2)  # Skip the header line itself

    # Step 3: Extract first non-empty line after header
    local raw_verdict=$(echo "$verdict_section" | \
        grep -v "^$" | \
        head -1 | \
        tr '[:upper:]' '[:lower:]' | \
        tr -d '[:space:]' | \
        sed 's/[^a-z_].*//') # Stop at first non-identifier char

    # Step 4: Validate against allowed verdicts
    if echo ",$valid_verdicts," | grep -q ",$raw_verdict,"; then
        echo "$raw_verdict"
        return 0
    fi

    # Step 5: No valid verdict found
    echo "unknown"
    return 1
}
```

#### Edge Case Handling

| Scenario | Behavior |
|----------|----------|
| Multiple "## Verdict" sections | Use the LAST one (final decision after revisions) |
| Verdict inside code block | Ignored (code blocks stripped before extraction) |
| "Verdict: approved" inline | Not matched (requires `## Verdict` header format) |
| Mixed case "APPROVED" | Normalized to lowercase |
| Verdict with explanation "approved — Tests pass" | Extracts "approved" only |
| No verdict found | Returns "unknown", triggers stuck handling |
| Verdict not in valid list | Returns "unknown", triggers stuck handling |

#### Why Last Verdict Wins

Reviews may contain revision history:
```markdown
## Initial Assessment
...

## Verdict
gaps_found

## After Revision
...

## Verdict
approved
```

The final `## Verdict` section represents the reviewer's final decision.

### Verdict Priority

1. If state has `after` with `run_tests` → use test-based verdict
2. If state has `parallel` → use parallel-based verdict
3. Otherwise → extract from output file (review-based)

## Examples

### Minimal V1 Workflow

```yaml
name: simple
version: 1

states:
  - name: do_work
    prompt: prompts/simple/work.md
    next: complete

entry_state: do_work
terminal_states:
  - complete
```

### V2 with Branches

```yaml
name: reviewed
version: 2

states:
  - name: work
    prompt: prompts/work.md
    next: review

  - name: review
    prompt: prompts/review.md
    verdicts:
      - approved
      - needs_work
    next:
      approved: complete
      needs_work: work

entry_state: work
terminal_states:
  - complete
```

### V3 with Defaults, Variables, and Params

```yaml
name: feature-impl
version: 3

defaults:
  max_iterations: 10
  timeout: 600

variables:
  package: my-package
  test_pattern: "qa_"

states:
  - name: impl
    prompt: prompts/feature/impl.md
    params:
      allow_test_changes: false
      debug: true
    next: review

  - name: review
    prompt: prompts/feature/review.md
    verdicts:
      - approved
      - needs_fix
    next:
      approved: complete
      needs_fix: impl

entry_state: impl
terminal_states:
  - complete
```

#### V3 Validation Rules

**Reserved Variable Names** — Cannot be used in `defaults`, `variables`, or state `params`:
- `state`, `iteration`, `current_state`, `phase`, `plan`, `plan_md`, `params`

**Identifier Rules** — Keys in `defaults`, `variables`, and `params` must match: `^[a-zA-Z_][a-zA-Z0-9_]*$`

**Type Validation** — `defaults.max_iterations` and `defaults.timeout` must be positive integers (> 0) if present.

**V3 fields are optional** — Omitting `defaults`, `variables`, or `params` entirely is valid. Empty sections (`defaults: {}`) are also valid.

**Version gating** — V3 validation only runs for `version: 3` or higher. V1/V2 workflows ignore these sections.

### V4 Schema Additions

V4 introduces four new sections to the workflow schema:

#### State-Level: `constraints`

```yaml
constraints:
  max_iterations: integer           # Fail if iteration >= this value
  require_artifacts_in:             # Must exist before agent spawns
    - path/to/artifact.md           # Relative to PHASE_DIR
  require_artifacts_out:            # Must exist after agent completes
    - path/to/output.md             # Relative to PHASE_DIR
```

#### State-Level: `after`

```yaml
after:
  - action: string                  # Action name from registry
    params:                         # Action-specific parameters
      key: value
    when: string                    # Verdict condition (optional)
    continue_on_error: boolean      # Continue hook chain on failure (default: false)
```

`when` supports: exact match (`approved`), negation (`!approved`), OR (`approved|passed`).

#### State-Level: `escalation`

```yaml
escalation:
  - at_iteration: integer           # Fire at exact iteration
    action: string
    params: {}
  - after_iteration: integer        # Fire once when iteration > N
    action: string
    params: {}
  - every_n_iterations: integer     # Fire when iteration % N == 0
    action: string
    params: {}
```

Only the first matching trigger fires per iteration. `after_iteration` triggers are tracked in `state.json` `executed_escalations` array.

#### Top-Level: `intervention_triggers`

```yaml
intervention_triggers:
  - condition: "field >= value"     # Condition against state.json
    action: request_human           # Currently only request_human
    message: string                 # Human-readable reason
```

Conditions support `==`, `!=`, `>`, `<`, `>=`, `<=`. Missing state fields default to 0. Intervention is checked first in the pipeline (step 1) and halts with exit code 2.

#### V4 Execution Order

Each `run_iteration` call follows this 8-step pipeline:

1. Check intervention triggers → exit 2 if triggered
2. Check escalation triggers → execute action if matched
3. Check pre-constraints → exit 1 if failed
4. Render prompt + spawn agent → create `iteration_NNN/`
5. Extract verdict → if state has `verdicts` field
6. Check post-constraints → exit 1 if failed
7. Run after hooks → verdict-conditional actions
8. Update state → increment iteration, resolve next_state

### V4 Full Featured

```yaml
name: bugfix
version: 4
description: Fix incorrect behavior in existing code

defaults:
  max_iterations: 10
  timeout: 600

states:
  - name: investigate
    description: Understand current behavior and root cause
    prompt: prompts/bugfix/investigate.md
    params:
      allow_code_changes: false
    constraints:
      require_artifacts_out:
        - findings.md
    next: regression_tests

  - name: regression_tests
    prompt: prompts/bugfix/regression-tests.md
    params:
      test_should_fail: true
    after:
      - action: run_tests
        params:
          pattern: "qa_{{phase}}"
          expect_failure: true
          save_to: test_output.txt
    next: test_review

  - name: test_review
    prompt: prompts/bugfix/test-review.md
    constraints:
      require_artifacts_in:
        - qa_reasoning.md
      require_artifacts_out:
        - qa_review.md
    verdicts:
      - approved
      - gaps_found
    next:
      approved: fix
      gaps_found: regression_tests
    after:
      - action: commit
        when: approved
        params:
          message: "test({{phase}}): add regression tests"

  - name: fix
    prompt: prompts/bugfix/fix.md
    params:
      allow_test_changes: false
    constraints:
      max_iterations: 15
      require_artifacts_out:
        - impl_reasoning.md
    after:
      - action: run_tests
        params:
          pattern: "qa_{{phase}}"
          save_to: last_test_output.txt
    escalation:
      - at_iteration: 3
        action: analyze_stuck
      - at_iteration: 5
        action: switch_model
        params:
          model: opus
      - at_iteration: 8
        action: attempt_split
    verdicts:
      - all_passing
      - some_failing
    next:
      all_passing: fix_review
      some_failing: fix

  - name: fix_review
    prompt: prompts/bugfix/fix-review.md
    constraints:
      require_artifacts_in:
        - impl_reasoning.md
        - last_test_output.txt
      require_artifacts_out:
        - impl_review.md
    verdicts:
      - approved
      - concerns
    next:
      approved: complete
      concerns: fix
    after:
      - action: commit
        when: approved
        params:
          message: "fix({{phase}}): {{objective}}"

entry_state: investigate

terminal_states:
  - name: complete
    description: Bug fixed successfully
  - name: blocked
    description: Cannot proceed without human intervention

intervention_triggers:
  - condition: "stuck_iterations >= max_iterations"
    action: request_human
    message: "Exceeded maximum iterations without progress"
```

### V5 with Parallel States

```yaml
name: multi-module-refactor
version: 5

states:
  - name: characterize
    parallel:
      strategy: all
      branches:
        - name: characterize_auth
          prompt: prompts/refactor/characterize.md
          params:
            module: auth
        - name: characterize_api
          prompt: prompts/refactor/characterize.md
          params:
            module: api
        - name: characterize_db
          prompt: prompts/refactor/characterize.md
          params:
            module: db
    verdicts:
      - all_complete
      - any_failed
    next:
      all_complete: refactor
      any_failed: blocked

  - name: refactor
    prompt: prompts/refactor/refactor.md
    next: verify

  - name: verify
    prompt: prompts/refactor/verify.md
    next: complete

entry_state: characterize
terminal_states:
  - complete
  - blocked
```

## Per-State Agent Config

Each state can configure the agent that executes it:

```yaml
states:
  - name: impl
    prompt: prompts/feature/impl.md
    agent:
      max_turns: 45       # Agent turn budget
      allowed_tools:       # Tool whitelist
        - View
        - Write
        - Bash
      timeout: 900         # Agent timeout in seconds
      model: sonnet        # Model override
    next: impl_review
```

When `agent` is nil/omitted, the system uses defaults. This is backward compatible with all existing workflows.

## Composable Blocks

Workflows can be composed from reusable blocks instead of (or in addition to) writing monolithic state machines. Blocks are self-contained state groups with parameterized entry/exit points.

### Block Format

```yaml
name: adversary
description: Adversary writes tests to find bugs in existing code
params:
  max_rounds: {default: 5}
  max_turns: {default: 30}
entry: adversary
exits: [done]
states:
  - name: adversary
    prompt: prompts/adversarial/adversary.md
    agent:
      max_turns: ${max_turns}
      allowed_tools: [Read, Grep, Glob, Write, Bash]
    verdicts: [bugs_found, no_bugs_found]
    constraints:
      max_iterations: ${max_rounds}
    next:
      bugs_found: adversary
      no_bugs_found: $done
```

### Parameter Substitution

`${param}` placeholders are resolved before the workflow is compiled:
- Parameters with defaults can be overridden at the pipeline level
- Missing required parameters (no default) cause a load error

### Pipeline Composition

Workflows with a `pipeline:` key are composed from blocks:

```yaml
name: adversarial
version: 1
description: Adversarial testing workflow
pipeline:
  - block: act
    name: impl
    params: {max_turns: 45}
  - block: adversary
    name: check
    run_once: true       # block executes once; subsequent visits auto-produce skip_exit
    skip_exit: no_bugs_found
    params: {max_turns: 30}
    route:
      bugs_found: impl
      no_bugs_found: complete
terminal_states: [complete, blocked]
```

**Pipeline step fields:**

| Field | Type | Purpose |
|-------|------|---------|
| `block` | string | Block type to instantiate |
| `name` | string | Step identity; used as state namespace prefix and routing target |
| `params` | map | Override block param defaults |
| `route` | map | Map block exits to step names or terminal states |
| `run_once` | bool | If true, block runs once; subsequent visits auto-produce `skip_exit` without spawning an agent |
| `skip_exit` | string | Exit name to auto-produce on re-entry when `run_once: true` |

### Resolution Rules

**Sequential**: States get block-prefixed names. Exit points wire to the next block's entry:
```
impl.impl → adversary.adversary → ... → complete
```

**Parallel** (pipeline step with `parallel:`):
```yaml
pipeline:
  - block: impl
  - parallel:
      strategy: all      # "all" or "any"
      blocks:
        - name: security
          block: adversary
          params: {focus: security}
        - name: correctness
          block: adversary
          params: {focus: correctness}
```

Parallel groups generate synthetic `_fork_N` and `_join_N` states. The orchestrator forks into concurrent `RunPhase` calls per block instance and joins based on strategy.

### Built-in Blocks

| Block | Entry State | Params | Purpose |
|-------|-------------|--------|---------|
| `act` | `act` | prompt, max_turns, timeout, model | Generic linear action — do work and exit unconditionally |
| `adversary` | `adversary` | prompt, max_turns | Adversarial review: find bugs, write failing tests, verdict bugs_found/no_bugs_found |
| `qa` | `qa` | prompt, max_turns | Write tests (QA loop entry) |
| `qa-review` | `qa_review` | prompt, max_turns | Review tests, verdict approved/gaps_found |
| `review` | `impl_review` | prompt, max_turns | Review implementation, verdict approved/concerns |
| `judge` | `judge` | prompt, verdict_a, verdict_b, max_turns | Generic two-verdict evaluator |

### Composition Validation

The composition system validates:
1. All block exit points are wired to a next block's entry or to `complete`
2. No dangling exits (referenced but not mapped)
3. Every non-terminal state can reach at least one terminal state (reverse-reachability)
4. No cycles without exit conditions
