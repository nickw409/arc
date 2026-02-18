#!/usr/bin/env bats

# Full system integration tests for V3 orchestration
# Phase: integration (orchestration-v3)
#
# Tests the complete V3 orchestration system by verifying cross-phase
# interactions between:
#   - validate-workflow.sh (schema-updates)
#   - build-context.sh (context-building)
#   - render_template.py (template-engine)
#   - update-state.sh (state management)
#   - iterate.sh data flow (prompt-conversion)
#
# These tests verify that all V3 phases work together correctly.
# They do NOT spawn Claude agents — they test the scripts by calling
# them directly and verifying their output.

setup() {
    load 'test_helper'
    setup_temp_dir

    BUILD_CONTEXT="$SCRIPTS_DIR/build-context.sh"
    RENDER_TEMPLATE="$SCRIPTS_DIR/render_template.py"
    VALIDATE_WORKFLOW="$SCRIPTS_DIR/validate-workflow.sh"
    UPDATE_STATE="$SCRIPTS_DIR/update-state.sh"

    # Standard phase directory structure
    PLAN_DIR="$TEST_TEMP_DIR/.plans/active/test-plan"
    PHASE_DIR="$PLAN_DIR/phases/test-phase"
    mkdir -p "$PHASE_DIR"

    # Prompts directory with real prompt templates
    PROMPTS_DIR="$TEST_TEMP_DIR/prompts"
    mkdir -p "$PROMPTS_DIR/feature" "$PROMPTS_DIR/common"

    # Export PLANS_DIR so update-state.sh and iterate.sh use our temp dir
    export PLANS_DIR="$TEST_TEMP_DIR/.plans"
}

teardown() {
    teardown_temp_dir
}

# Helper: run build-context.sh with defaults
build_context() {
    local state_file="${1:-$PHASE_DIR/state.json}"
    local workflow_file="${2:-$TEST_TEMP_DIR/workflow.yaml}"
    local phase_dir="${3:-$PHASE_DIR}"
    local state_name="${4:-impl}"
    "$BUILD_CONTEXT" "$state_file" "$workflow_file" "$phase_dir" "$state_name"
}

# Helper: render template
render_template() {
    local template_file="$1"
    local context_json="$2"
    local base_dir="${3:-$PROMPTS_DIR}"
    python3 "$RENDER_TEMPLATE" "$template_file" "$context_json" "$base_dir"
}

# Helper: create the full initial state.json needed by update-state.sh
create_full_state() {
    local iter_current="${1:-1}"
    local iter_max="${2:-25}"
    local status="${3:-impl}"
    cat > "$PHASE_DIR/state.json" << EOF
{
  "iteration": {"current": $iter_current, "max": $iter_max},
  "phase_status": "$status",
  "tests_passing": 0,
  "tests_total": 0,
  "stuck_iterations": 0,
  "hang_count": 0,
  "last_reviewed_iteration": 0,
  "last_qa_reviewed_iteration": 0,
  "packages": [],
  "chunks": {"total": 0, "completed": [], "current": null, "remaining": []},
  "blocked": {"is_blocked": false, "reason": null},
  "disputes": [],
  "last_cleared_disputes": []
}
EOF
}

#=============================================================================
# test_integration_full_qa_cycle
# Verifies the complete QA cycle data flow:
#   build-context.sh → render_template.py with qa.md template
# Tests that the rendered prompt contains expected content from all sources.
#=============================================================================

@test "integration: full QA cycle - context to rendered prompt" {
    # Setup workflow
    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test-workflow
version: 3
defaults:
  max_iterations: 10
variables:
  package: test-package
states:
  - name: qa
    prompt: prompts/feature/qa.md
    next: qa_review
  - name: qa_review
    prompt: prompts/feature/qa-review.md
    next: impl
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: qa
terminal_states: [complete]
YAML

    # Setup state.json
    cat > "$PHASE_DIR/state.json" << 'JSON'
{"iteration": {"current": 1, "max": 25}, "phase_status": "qa", "tests_passing": 0, "tests_total": 0}
JSON

    # Setup plan.md
    cat > "$PHASE_DIR/plan.md" << 'MD'
# Test Phase

## Objective
Test something.

## Files

### Create
- `src/test.rs` -- Test file

## Test Cases

### test_basic
**Input:** None
**Expected:** Success
MD

    # Create test-commands.md include
    cat > "$PROMPTS_DIR/common/test-commands.md" << 'TEMPLATE'
### Running Tests

Use these commands to run tests:

```bash
# Run phase-specific tests
cargo nextest run -p {{crate | default: "test-package"}} --test "*qa_{{phase}}*"

# Run with verbose output
cargo nextest run -p {{crate | default: "test-package"}} --test "*qa_{{phase}}*" --nocapture
```
TEMPLATE

    # Create qa.md template
    cat > "$PROMPTS_DIR/feature/qa.md" << 'TEMPLATE'
# QA - {{phase}}

You are a QA engineer writing tests for phase: **{{phase}}** of plan: **{{plan}}**.

## Context

{{#if plan_md}}
### Phase Specification

{{plan_md}}
{{/if}}

### Iteration
This is iteration {{iteration}}.

## Instructions

Write comprehensive tests based on the phase specification above.

{{> common/test-commands.md}}
TEMPLATE

    # Step 1: Build context
    local context
    context=$(build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" "qa")

    # Step 2: Render the qa prompt template
    local rendered
    rendered=$(render_template "$PROMPTS_DIR/feature/qa.md" "$context" "$PROMPTS_DIR")

    # Verify prompt content from template header
    [[ "$rendered" == *"# QA - test-phase"* ]]

    # Verify plan.md content is included
    [[ "$rendered" == *"# Test Phase"* ]]

    # Verify iteration from context
    [[ "$rendered" == *"iteration 1"* ]]

    # Verify test-commands.md include was resolved with crate variable
    [[ "$rendered" == *"cargo nextest run"* ]]

    # Verify no unresolved template tags remain
    # (Check for {{ that isn't an escaped brace or doc content)
    local unresolved
    unresolved=$(echo "$rendered" | grep -cE '\{\{[a-zA-Z_#/>]' || true)
    [[ "$unresolved" -eq 0 ]]
}

#=============================================================================
# test_integration_context_precedence
# Verifies merge precedence: defaults < variables, params nested under params key.
#=============================================================================

@test "integration: context precedence - variables override defaults" {
    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test-workflow
version: 3
defaults:
  timeout: 600
  retries: 3
variables:
  timeout: 900
  package: test-package
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
    params:
      focus_area: "Core"
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    cat > "$PHASE_DIR/state.json" << 'JSON'
{"iteration": {"current": 2, "max": 25}}
JSON
    printf '' > "$PHASE_DIR/plan.md"

    # Build context
    local context
    context=$(build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" "impl")

    # Variables override defaults: timeout = 900 (not 600)
    local timeout
    timeout=$(echo "$context" | jq -r '.timeout')
    [[ "$timeout" == "900" ]]

    # Defaults not overridden: retries = 3
    local retries
    retries=$(echo "$context" | jq -r '.retries')
    [[ "$retries" == "3" ]]

    # Variables present: crate
    local crate
    crate=$(echo "$context" | jq -r '.crate')
    [[ "$crate" == "test-package" ]]

    # Params nested under params key
    local focus
    focus=$(echo "$context" | jq -r '.params.focus_area')
    [[ "$focus" == "Core" ]]

    # Iteration computed from state.json
    local iteration
    iteration=$(echo "$context" | jq '.iteration')
    [[ "$iteration" == "2" ]]

    # Phase and plan computed from path
    local phase plan
    phase=$(echo "$context" | jq -r '.phase')
    plan=$(echo "$context" | jq -r '.plan')
    [[ "$phase" == "test-phase" ]]
    [[ "$plan" == "test-plan" ]]

    # Render a template that displays all context fields
    cat > "$PROMPTS_DIR/feature/precedence_test.md" << 'TEMPLATE'
Timeout: {{timeout}}
Retries: {{retries}}
Crate: {{crate}}
Focus: {{params.focus_area}}
Iteration: {{iteration}}
Phase: {{phase}}
Plan: {{plan}}
TEMPLATE

    local rendered
    rendered=$(render_template "$PROMPTS_DIR/feature/precedence_test.md" "$context" "$PROMPTS_DIR")

    [[ "$rendered" == *"Timeout: 900"* ]]
    [[ "$rendered" == *"Retries: 3"* ]]
    [[ "$rendered" == *"Crate: test-package"* ]]
    [[ "$rendered" == *"Focus: Core"* ]]
    [[ "$rendered" == *"Iteration: 2"* ]]
    [[ "$rendered" == *"Phase: test-phase"* ]]
    [[ "$rendered" == *"Plan: test-plan"* ]]
}

#=============================================================================
# test_integration_validation_then_context
# Validates workflow, then builds context — both succeed on the same file.
#=============================================================================

@test "integration: validation then context - both agree on workflow" {
    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test-workflow
version: 3
defaults:
  max_iterations: 10
variables:
  package: test-package
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
    params:
      focus_area: "Core"
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    cat > "$PHASE_DIR/state.json" << 'JSON'
{"iteration": {"current": 1, "max": 25}}
JSON
    printf '# Plan\n' > "$PHASE_DIR/plan.md"

    # Step 1: Validation passes
    run "$VALIDATE_WORKFLOW" "$TEST_TEMP_DIR/workflow.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Validation passed"* ]]

    # Step 2: Context builds correctly with same workflow
    local context
    context=$(build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" "impl")

    # Both agree: workflow is valid and context is usable
    echo "$context" | jq -e '.phase' > /dev/null
    echo "$context" | jq -e '.crate' > /dev/null
    echo "$context" | jq -e '.params.focus_area' > /dev/null
}

#=============================================================================
# test_integration_invalid_workflow_rejected
# Invalid V3 workflow (reserved variable conflict) fails validation cleanly.
#=============================================================================

@test "integration: invalid workflow rejected - reserved variable conflict" {
    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test-bad
version: 3
variables:
  iteration: 5
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    # Validation fails
    run "$VALIDATE_WORKFLOW" "$TEST_TEMP_DIR/workflow.yaml"
    [[ "$status" -ne 0 ]]
    [[ "$output" == *"Reserved"* ]] || [[ "$output" == *"reserved"* ]] || [[ "$output" == *"iteration"* ]]
}

#=============================================================================
# test_integration_template_with_all_features
# Template using variables, conditionals, each, and includes — all rendered.
#=============================================================================

@test "integration: template with all V3 features combined" {
    # Create include
    cat > "$PROMPTS_DIR/common/header.md" << 'TEMPLATE'
## {{title | default: "Default Title"}}
TEMPLATE

    # Create template using all features
    cat > "$PROMPTS_DIR/feature/all_features.md" << 'TEMPLATE'
{{> common/header.md}}

Phase: {{phase}}, Iteration: {{iteration}}

{{#if params.focus_area}}Focus: {{params.focus_area}}{{/if}}

{{#unless params.skip_tests}}Run tests for {{crate}}{{/unless}}

{{#each params.files}}- {{this.path}}: {{this.desc}}
{{/each}}

Timeout: {{timeout | default: "300"}}
TEMPLATE

    # Build context with matching data
    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test-all
version: 3
defaults:
  timeout: 600
variables:
  package: test-package
  title: "All Features Test"
states:
  - name: impl
    prompt: prompts/feature/all_features.md
    next: complete
    params:
      focus_area: "Template engine"
      skip_tests: false
      files:
        - path: src/a.rs
          desc: File A
        - path: src/b.rs
          desc: File B
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    cat > "$PHASE_DIR/state.json" << 'JSON'
{"iteration": {"current": 3, "max": 25}}
JSON
    printf '# Plan\n' > "$PHASE_DIR/plan.md"

    local context
    context=$(build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" "impl")

    local rendered
    rendered=$(render_template "$PROMPTS_DIR/feature/all_features.md" "$context" "$PROMPTS_DIR")

    # Include resolved
    [[ "$rendered" == *"## All Features Test"* ]]

    # Variables resolved
    [[ "$rendered" == *"Phase: test-phase"* ]]
    [[ "$rendered" == *"Iteration: 3"* ]]

    # Conditional rendered
    [[ "$rendered" == *"Focus: Template engine"* ]]

    # Unless block rendered (skip_tests is false)
    [[ "$rendered" == *"Run tests for test-package"* ]]

    # Each loop rendered
    [[ "$rendered" == *"- src/a.rs: File A"* ]]
    [[ "$rendered" == *"- src/b.rs: File B"* ]]

    # Default value resolved
    [[ "$rendered" == *"Timeout: 600"* ]]

    # No rendering errors — no unresolved tags
    local unresolved
    unresolved=$(echo "$rendered" | grep -cE '\{\{[a-zA-Z_#/>]' || true)
    [[ "$unresolved" -eq 0 ]]
}

#=============================================================================
# test_integration_state_update_after_render
# Verifies update-state.sh increment-iteration preserves other state fields.
#=============================================================================

@test "integration: state update preserves fields after increment" {
    create_full_state 1 25 "impl"

    # Manually set test counts
    jq '.tests_passing = 3 | .tests_total = 10' "$PHASE_DIR/state.json" > "$PHASE_DIR/state.json.tmp"
    mv "$PHASE_DIR/state.json.tmp" "$PHASE_DIR/state.json"

    # Verify initial state
    local initial_iter
    initial_iter=$(jq '.iteration.current' "$PHASE_DIR/state.json")
    [[ "$initial_iter" -eq 1 ]]

    # Increment iteration
    "$UPDATE_STATE" test-plan test-phase increment-iteration

    # Verify iteration incremented
    local new_iter
    new_iter=$(jq '.iteration.current' "$PHASE_DIR/state.json")
    [[ "$new_iter" -eq 2 ]]

    # Verify other fields preserved
    local max tests_passing tests_total
    max=$(jq '.iteration.max' "$PHASE_DIR/state.json")
    tests_passing=$(jq '.tests_passing' "$PHASE_DIR/state.json")
    tests_total=$(jq '.tests_total' "$PHASE_DIR/state.json")

    [[ "$max" -eq 25 ]]
    [[ "$tests_passing" -eq 3 ]]
    [[ "$tests_total" -eq 10 ]]
}

#=============================================================================
# test_integration_multiple_iterations
# Verifies iteration counter increments correctly over multiple runs.
#=============================================================================

@test "integration: multiple iteration increments" {
    create_full_state 1 25 "impl"

    # Increment 3 times
    "$UPDATE_STATE" test-plan test-phase increment-iteration
    "$UPDATE_STATE" test-plan test-phase increment-iteration
    "$UPDATE_STATE" test-plan test-phase increment-iteration

    # Verify iteration is now 4
    local final_iter
    final_iter=$(jq '.iteration.current' "$PHASE_DIR/state.json")
    [[ "$final_iter" -eq 4 ]]
}

#=============================================================================
# test_integration_multiple_iterations_with_context
# Verifies that each iteration produces correct context with updated iteration.
#=============================================================================

@test "integration: multiple iterations produce correct context" {
    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test
version: 3
defaults:
  max_iterations: 25
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    printf '# Plan\n' > "$PHASE_DIR/plan.md"

    # Create template showing iteration
    cat > "$PROMPTS_DIR/feature/iter_test.md" << 'TEMPLATE'
Iteration: {{iteration}}/{{max_iterations}}
TEMPLATE

    # Iteration 1
    create_full_state 1 25 "impl"
    local ctx1 rendered1
    ctx1=$(build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" "impl")
    rendered1=$(render_template "$PROMPTS_DIR/feature/iter_test.md" "$ctx1" "$PROMPTS_DIR")
    [[ "$rendered1" == *"Iteration: 1/25"* ]]

    # Increment to iteration 2
    "$UPDATE_STATE" test-plan test-phase increment-iteration
    local ctx2 rendered2
    ctx2=$(build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" "impl")
    rendered2=$(render_template "$PROMPTS_DIR/feature/iter_test.md" "$ctx2" "$PROMPTS_DIR")
    [[ "$rendered2" == *"Iteration: 2/25"* ]]

    # Increment to iteration 3
    "$UPDATE_STATE" test-plan test-phase increment-iteration
    local ctx3 rendered3
    ctx3=$(build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" "impl")
    rendered3=$(render_template "$PROMPTS_DIR/feature/iter_test.md" "$ctx3" "$PROMPTS_DIR")
    [[ "$rendered3" == *"Iteration: 3/25"* ]]
}

#=============================================================================
# test_integration_error_recovery
# State with stuck_iterations — context still builds correctly.
#=============================================================================

@test "integration: error recovery - stuck iterations preserved in context" {
    create_full_state 5 25 "impl"

    # Set stuck_iterations
    jq '.stuck_iterations = 2' "$PHASE_DIR/state.json" > "$PHASE_DIR/state.json.tmp"
    mv "$PHASE_DIR/state.json.tmp" "$PHASE_DIR/state.json"

    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test
version: 3
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML
    printf '# Plan\n' > "$PHASE_DIR/plan.md"

    # Context still builds
    local context
    context=$(build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" "impl")

    # Iteration reflects current state
    local iteration
    iteration=$(echo "$context" | jq '.iteration')
    [[ "$iteration" == "5" ]]

    # stuck_iterations accessible via state object
    local stuck
    stuck=$(echo "$context" | jq '.state.stuck_iterations')
    [[ "$stuck" == "2" ]]

    # Increment preserves stuck_iterations
    "$UPDATE_STATE" test-plan test-phase increment-iteration
    local new_iter new_stuck
    new_iter=$(jq '.iteration.current' "$PHASE_DIR/state.json")
    new_stuck=$(jq '.stuck_iterations' "$PHASE_DIR/state.json")
    [[ "$new_iter" -eq 6 ]]
    [[ "$new_stuck" -eq 2 ]]
}

#=============================================================================
# test_cross_phase_error_template_not_found
# render_template.py fails with exit 1 when template file is missing.
#=============================================================================

@test "integration: error propagation - template not found" {
    run python3 "$RENDER_TEMPLATE" "$TEST_TEMP_DIR/nonexistent_template.md" '{}'
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"not found"* ]] || [[ "$output" == *"No such file"* ]] || [[ "$output" == *"Error"* ]]
}

#=============================================================================
# test_cross_phase_error_invalid_context
# build-context.sh fails with clear error on malformed state.json.
#=============================================================================

@test "integration: error propagation - malformed state.json" {
    echo '{not valid json' > "$PHASE_DIR/state.json"

    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test
version: 3
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    run "$BUILD_CONTEXT" "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" impl
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Error"* ]] || [[ "$output" == *"error"* ]] || [[ "$output" == *"Invalid"* ]]
}

#=============================================================================
# test_cross_phase_error_render_failure
# render_template.py fails with exit 1 on invalid JSON context.
#=============================================================================

@test "integration: error propagation - invalid JSON context to render" {
    cat > "$PROMPTS_DIR/feature/simple.md" << 'TEMPLATE'
Hello {{name}}
TEMPLATE

    run python3 "$RENDER_TEMPLATE" "$PROMPTS_DIR/feature/simple.md" "not json"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"JSON"* ]] || [[ "$output" == *"json"* ]]
}

#=============================================================================
# test_integration_backwards_compat_v1
# V1 workflow validates, builds context, and renders correctly.
#=============================================================================

@test "integration: backwards compat V1 - full pipeline" {
    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test
version: 1
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    cat > "$PHASE_DIR/state.json" << 'JSON'
{"iteration": {"current": 1, "max": 10}}
JSON
    cat > "$PHASE_DIR/plan.md" << 'MD'
# Simple Phase

## Objective
Simple test.

## Files

### Create
- `src/test.rs` -- Test

## Test Cases

### test_one
**Input:** None
**Expected:** Pass
MD

    # Step 1: Validate
    run "$VALIDATE_WORKFLOW" "$TEST_TEMP_DIR/workflow.yaml"
    [[ "$status" -eq 0 ]]

    # Step 2: Build context
    local context
    context=$(build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" "impl")

    # Computed values present
    local iteration phase plan
    iteration=$(echo "$context" | jq '.iteration')
    phase=$(echo "$context" | jq -r '.phase')
    plan=$(echo "$context" | jq -r '.plan')
    [[ "$iteration" == "1" ]]
    [[ "$phase" == "test-phase" ]]
    [[ "$plan" == "test-plan" ]]

    # V1 has no defaults or variables — context should NOT contain them
    local defaults_timeout
    defaults_timeout=$(echo "$context" | jq '.timeout // "MISSING"')
    [[ "$defaults_timeout" == '"MISSING"' ]]

    # plan_md present
    local plan_md
    plan_md=$(echo "$context" | jq -r '.plan_md')
    [[ "$plan_md" == *"# Simple Phase"* ]]

    # Step 3: Render template
    cat > "$PROMPTS_DIR/feature/v1_test.md" << 'TEMPLATE'
Phase: {{phase}}, Iteration: {{iteration}}
Plan: {{plan_md}}
TEMPLATE

    local rendered
    rendered=$(render_template "$PROMPTS_DIR/feature/v1_test.md" "$context" "$PROMPTS_DIR")
    [[ "$rendered" == *"Phase: test-phase"* ]]
    [[ "$rendered" == *"Iteration: 1"* ]]
    [[ "$rendered" == *"# Simple Phase"* ]]
}

#=============================================================================
# test_integration_backwards_compat_v2
# V2 workflow with verdicts validates and produces correct context.
#=============================================================================

@test "integration: backwards compat V2 - verdicts and branching" {
    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test
version: 2
states:
  - name: review
    prompt: prompts/feature/impl-review.md
    verdicts: [approved, concerns]
    next:
      approved: complete
      concerns: fix
  - name: fix
    prompt: prompts/feature/impl.md
    next: review
  - name: complete
    prompt: prompts/common/complete.md
entry_state: review
terminal_states: [complete]
YAML

    cat > "$PHASE_DIR/state.json" << 'JSON'
{"iteration": {"current": 2, "max": 10}}
JSON
    printf '# Review Phase\n' > "$PHASE_DIR/plan.md"

    # Step 1: Validate
    run "$VALIDATE_WORKFLOW" "$TEST_TEMP_DIR/workflow.yaml"
    [[ "$status" -eq 0 ]]

    # Step 2: Build context for review state
    local context
    context=$(build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" "review")

    local current_state
    current_state=$(echo "$context" | jq -r '.current_state')
    [[ "$current_state" == "review" ]]

    # V2 has no defaults/variables/params — params should be empty
    local params
    params=$(echo "$context" | jq '.params')
    [[ "$params" == "{}" ]]

    # Step 3: Render template
    cat > "$PROMPTS_DIR/feature/v2_test.md" << 'TEMPLATE'
State: {{current_state}}, Iteration: {{iteration}}
TEMPLATE

    local rendered
    rendered=$(render_template "$PROMPTS_DIR/feature/v2_test.md" "$context" "$PROMPTS_DIR")
    [[ "$rendered" == *"State: review"* ]]
    [[ "$rendered" == *"Iteration: 2"* ]]
}

#=============================================================================
# test_integration_v3_with_v2_features
# V3 workflow combining verdicts (V2) with defaults/variables/params (V3).
#=============================================================================

@test "integration: V3 with V2 features - verdicts + params" {
    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test
version: 3
defaults:
  max_iterations: 10
variables:
  package: test-package
states:
  - name: review
    prompt: prompts/feature/impl-review.md
    verdicts: [approved, concerns]
    next:
      approved: complete
      concerns: impl
    params:
      strict_mode: true
  - name: impl
    prompt: prompts/feature/impl.md
    next: review
    params:
      focus_area: "Core"
  - name: complete
    prompt: prompts/common/complete.md
entry_state: review
terminal_states: [complete]
YAML

    cat > "$PHASE_DIR/state.json" << 'JSON'
{"iteration": {"current": 1, "max": 10}}
JSON
    printf '# V3+V2 Phase\n' > "$PHASE_DIR/plan.md"

    # Step 1: Validate
    run "$VALIDATE_WORKFLOW" "$TEST_TEMP_DIR/workflow.yaml"
    [[ "$status" -eq 0 ]]

    # Step 2: Build context for review state (has strict_mode param)
    local context
    context=$(build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" "review")

    # V3 params
    local strict_mode
    strict_mode=$(echo "$context" | jq '.params.strict_mode')
    [[ "$strict_mode" == "true" ]]

    # V3 variables
    local crate
    crate=$(echo "$context" | jq -r '.crate')
    [[ "$crate" == "test-package" ]]

    # Step 3: Build context for impl state (different params)
    local context_impl
    context_impl=$(build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" "impl")

    local focus_area
    focus_area=$(echo "$context_impl" | jq -r '.params.focus_area')
    [[ "$focus_area" == "Core" ]]

    # Render review template
    cat > "$PROMPTS_DIR/feature/v3v2_test.md" << 'TEMPLATE'
{{#if params.strict_mode}}Strict mode enabled{{/if}}
Crate: {{crate}}
TEMPLATE

    local rendered
    rendered=$(render_template "$PROMPTS_DIR/feature/v3v2_test.md" "$context" "$PROMPTS_DIR")
    [[ "$rendered" == *"Strict mode enabled"* ]]
    [[ "$rendered" == *"Crate: test-package"* ]]
}

#=============================================================================
# test_integration_status_transitions
# Verifies status transitions through update-state.sh affect context.
#=============================================================================

@test "integration: status transitions flow through context" {
    create_full_state 1 25 "qa"

    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test
version: 3
states:
  - name: qa
    prompt: prompts/feature/qa.md
    next: impl
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: qa
terminal_states: [complete]
YAML
    printf '# Plan\n' > "$PHASE_DIR/plan.md"

    # Initial status: qa
    local ctx
    ctx=$(build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" "qa")
    local status
    status=$(echo "$ctx" | jq -r '.state.phase_status')
    [[ "$status" == "qa" ]]

    # Transition to implementing
    "$UPDATE_STATE" test-plan test-phase status implementing

    ctx=$(build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" "impl")
    status=$(echo "$ctx" | jq -r '.state.phase_status')
    [[ "$status" == "implementing" ]]
}

#=============================================================================
# test_integration_data_flow_end_to_end
# Complete data flow: validate → build context → render → verify output.
#=============================================================================

@test "integration: end-to-end data flow" {
    # Setup V3 workflow
    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: e2e-test
version: 3
defaults:
  max_iterations: 15
  timeout: 600
variables:
  package: test-package
  project: RSImulation
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: impl_review
    params:
      focus_area: "End-to-end testing"
      allow_test_changes: false
  - name: impl_review
    prompt: prompts/feature/impl-review.md
    verdicts: [approved, concerns]
    next:
      approved: complete
      concerns: impl
  - name: complete
    prompt: prompts/common/complete.md
  - name: blocked
    prompt: prompts/common/blocked.md
entry_state: impl
terminal_states: [complete, blocked]
YAML

    create_full_state 1 25 "impl"
    cat > "$PHASE_DIR/plan.md" << 'MD'
# E2E Phase

## Objective
Test end-to-end data flow.

## Files

### Create
- `src/e2e.rs` -- E2E implementation

## Test Cases

### test_e2e
**Input:** Full pipeline data
**Expected:** All components integrate correctly
MD

    # Step 1: Validate
    run "$VALIDATE_WORKFLOW" "$TEST_TEMP_DIR/workflow.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Validation passed"* ]]

    # Step 2: Build context
    local context
    context=$(build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" "impl")

    # Verify full context shape
    echo "$context" | jq -e 'has("state")' > /dev/null
    echo "$context" | jq -e 'has("params")' > /dev/null
    echo "$context" | jq -e 'has("iteration")' > /dev/null
    echo "$context" | jq -e 'has("current_state")' > /dev/null
    echo "$context" | jq -e 'has("phase")' > /dev/null
    echo "$context" | jq -e 'has("plan")' > /dev/null
    echo "$context" | jq -e 'has("plan_md")' > /dev/null
    echo "$context" | jq -e 'has("crate")' > /dev/null
    echo "$context" | jq -e 'has("project")' > /dev/null
    echo "$context" | jq -e 'has("max_iterations")' > /dev/null
    echo "$context" | jq -e 'has("timeout")' > /dev/null

    # Step 3: Render with template using all feature types
    cat > "$PROMPTS_DIR/common/test-commands.md" << 'TEMPLATE'
```bash
cargo nextest run -p {{crate}}
```
TEMPLATE

    cat > "$PROMPTS_DIR/feature/e2e_template.md" << 'TEMPLATE'
# Implementation - {{phase}}

Project: {{project}}
Crate: {{crate}}
Iteration: {{iteration}}/{{max_iterations}}
Timeout: {{timeout}}s

{{#if params.focus_area}}
## Focus
{{params.focus_area}}
{{/if}}

{{#unless params.allow_test_changes}}
DO NOT modify test files.
{{/unless}}

## Plan
{{plan_md}}

## Commands
{{> common/test-commands.md}}
TEMPLATE

    local rendered
    rendered=$(render_template "$PROMPTS_DIR/feature/e2e_template.md" "$context" "$PROMPTS_DIR")

    # All data sources verified in output
    [[ "$rendered" == *"# Implementation - test-phase"* ]]
    [[ "$rendered" == *"Project: RSImulation"* ]]
    [[ "$rendered" == *"Crate: test-package"* ]]
    [[ "$rendered" == *"Iteration: 1/15"* ]]
    [[ "$rendered" == *"Timeout: 600s"* ]]
    [[ "$rendered" == *"End-to-end testing"* ]]
    [[ "$rendered" == *"DO NOT modify test files."* ]]
    [[ "$rendered" == *"# E2E Phase"* ]]
    [[ "$rendered" == *"cargo nextest run -p test-package"* ]]

    # No unresolved template tags
    local unresolved
    unresolved=$(echo "$rendered" | grep -cE '\{\{[a-zA-Z_#/>]' || true)
    [[ "$unresolved" -eq 0 ]]
}

#=============================================================================
# test_integration_state_consistency
# Verifies state files remain consistent across multiple operations.
#=============================================================================

@test "integration: state consistency across operations" {
    create_full_state 1 25 "qa"

    # Sequence of operations: status change → increment → test update → increment
    "$UPDATE_STATE" test-plan test-phase status implementing
    "$UPDATE_STATE" test-plan test-phase increment-iteration
    "$UPDATE_STATE" test-plan test-phase tests 3 10
    "$UPDATE_STATE" test-plan test-phase increment-iteration

    # Verify final state is consistent
    local state
    state=$(cat "$PHASE_DIR/state.json")
    local iter status passing total
    iter=$(echo "$state" | jq '.iteration.current')
    status=$(echo "$state" | jq -r '.phase_status')
    passing=$(echo "$state" | jq '.tests_passing')
    total=$(echo "$state" | jq '.tests_total')

    [[ "$iter" -eq 3 ]]
    [[ "$status" == "implementing" ]]
    [[ "$passing" -eq 3 ]]
    [[ "$total" -eq 10 ]]

    # state.json is still valid JSON
    jq empty "$PHASE_DIR/state.json"
}

#=============================================================================
# test_integration_ordering_dependency
# Validation must pass before context building is meaningful.
#=============================================================================

@test "integration: ordering - validate before context build" {
    # Create a valid V3 workflow
    cat > "$TEST_TEMP_DIR/good.yaml" << 'YAML'
name: good
version: 3
defaults:
  max_iterations: 10
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    # Create an invalid V3 workflow (reserved conflict)
    cat > "$TEST_TEMP_DIR/bad.yaml" << 'YAML'
name: bad
version: 3
variables:
  state: "conflicts with reserved"
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    cat > "$PHASE_DIR/state.json" << 'JSON'
{"iteration": {"current": 1, "max": 25}}
JSON
    printf '# Plan\n' > "$PHASE_DIR/plan.md"

    # Good workflow: validate passes, context builds
    run "$VALIDATE_WORKFLOW" "$TEST_TEMP_DIR/good.yaml"
    [[ "$status" -eq 0 ]]

    local context
    context=$(build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/good.yaml" "$PHASE_DIR" "impl")
    echo "$context" | jq -e '.max_iterations' > /dev/null

    # Bad workflow: validate fails — context may still build but is semantically wrong
    run "$VALIDATE_WORKFLOW" "$TEST_TEMP_DIR/bad.yaml"
    [[ "$status" -ne 0 ]]
}

#=============================================================================
# test_integration_partial_failure
# One component fails, others handle gracefully.
#=============================================================================

@test "integration: partial failure - render fails, context succeeds" {
    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test
version: 3
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    cat > "$PHASE_DIR/state.json" << 'JSON'
{"iteration": {"current": 1, "max": 25}}
JSON
    printf '# Plan\n' > "$PHASE_DIR/plan.md"

    # Context builds successfully
    local context
    context=$(build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" "impl")
    echo "$context" | jq -e '.phase' > /dev/null

    # But render fails with bad context string
    run python3 "$RENDER_TEMPLATE" "$PROMPTS_DIR/feature/simple.md" "broken json" 2>&1
    # Either fails because template doesn't exist or because JSON is bad
    [[ "$status" -ne 0 ]]
}

#=============================================================================
# test_integration_real_prompt_templates
# Tests with the actual orchestration prompt templates from the repo.
#=============================================================================

@test "integration: renders real qa.md prompt template" {
    local real_qa="$ORCH_DIR/prompts/feature/qa.md"
    [[ -f "$real_qa" ]] || skip "qa.md template not found"

    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test
version: 3
defaults:
  max_iterations: 10
variables:
  package: test-package
states:
  - name: qa
    prompt: prompts/feature/qa.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: qa
terminal_states: [complete]
YAML

    cat > "$PHASE_DIR/state.json" << 'JSON'
{"iteration": {"current": 1, "max": 25}, "phase_status": "qa"}
JSON
    cat > "$PHASE_DIR/plan.md" << 'MD'
# Real QA Test

## Objective
Test real template rendering.

## Files

### Create
- `src/test.rs` -- Test

## Test Cases

### test_one
**Input:** None
**Expected:** Pass
MD

    local context
    context=$(build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" "qa")

    local rendered
    rendered=$(render_template "$real_qa" "$context" "$ORCH_DIR/prompts")

    # Should contain QA header
    [[ "$rendered" == *"QA"* ]]
    # Should contain phase name
    [[ "$rendered" == *"test-phase"* ]]
    # Should contain plan content
    [[ "$rendered" == *"Real QA Test"* ]]
    # Should contain iteration
    [[ "$rendered" == *"iteration 1"* ]]
    # Should contain cargo nextest (from test-commands include)
    [[ "$rendered" == *"cargo nextest"* ]]
    # No unresolved tags
    [[ "$rendered" != *'{{phase}}'* ]]
    [[ "$rendered" != *'{{iteration}}'* ]]
}

@test "integration: renders real impl.md prompt template" {
    local real_impl="$ORCH_DIR/prompts/feature/impl.md"
    [[ -f "$real_impl" ]] || skip "impl.md template not found"

    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test
version: 3
defaults:
  max_iterations: 10
variables:
  package: test-package
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
    params:
      focus_area: "Core implementation"
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    cat > "$PHASE_DIR/state.json" << 'JSON'
{"iteration": {"current": 3, "max": 25}, "phase_status": "implementing", "tests_passing": 5, "tests_total": 10}
JSON
    cat > "$PHASE_DIR/plan.md" << 'MD'
# Impl Test

## Objective
Test impl rendering.

## Files

### Create
- `src/test.rs` -- Test

## Test Cases

### test_one
**Input:** None
**Expected:** Pass
MD

    local context
    context=$(build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" "impl")

    local rendered
    rendered=$(render_template "$real_impl" "$context" "$ORCH_DIR/prompts")

    # Should be non-empty and contain key content
    [[ -n "$rendered" ]]
    [[ "$rendered" == *"test-phase"* ]]
    [[ "$rendered" != *'{{phase}}'* ]]
}

#=============================================================================
# Edge Cases
#=============================================================================

#=============================================================================
# Edge: Phase boundary - context built for one state, params from another.
#=============================================================================

@test "integration edge: different states get different params" {
    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test
version: 3
variables:
  package: test-package
states:
  - name: qa
    prompt: prompts/feature/qa.md
    next: impl
    params:
      strict: true
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
    params:
      allow_test_changes: false
      focus_area: "Engine"
  - name: complete
    prompt: prompts/common/complete.md
entry_state: qa
terminal_states: [complete]
YAML

    cat > "$PHASE_DIR/state.json" << 'JSON'
{"iteration": {"current": 1, "max": 25}}
JSON
    printf '# Plan\n' > "$PHASE_DIR/plan.md"

    # QA state params
    local ctx_qa
    ctx_qa=$(build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" "qa")
    local qa_strict qa_focus
    qa_strict=$(echo "$ctx_qa" | jq '.params.strict')
    qa_focus=$(echo "$ctx_qa" | jq '.params.focus_area // "NONE"')
    [[ "$qa_strict" == "true" ]]
    [[ "$qa_focus" == '"NONE"' ]]

    # Impl state params
    local ctx_impl
    ctx_impl=$(build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" "impl")
    local impl_strict impl_focus
    impl_strict=$(echo "$ctx_impl" | jq '.params.strict // "NONE"')
    impl_focus=$(echo "$ctx_impl" | jq -r '.params.focus_area')
    [[ "$impl_strict" == '"NONE"' ]]
    [[ "$impl_focus" == "Engine" ]]

    # Both share the same variables
    local qa_crate impl_crate
    qa_crate=$(echo "$ctx_qa" | jq -r '.crate')
    impl_crate=$(echo "$ctx_impl" | jq -r '.crate')
    [[ "$qa_crate" == "test-package" ]]
    [[ "$impl_crate" == "test-package" ]]
}

#=============================================================================
# Edge: Resource cleanup - temp files don't leak across tests.
#=============================================================================

@test "integration edge: state.json operations are atomic" {
    create_full_state 1 25 "impl"

    # Run several rapid state updates
    "$UPDATE_STATE" test-plan test-phase increment-iteration
    "$UPDATE_STATE" test-plan test-phase tests 5 10
    "$UPDATE_STATE" test-plan test-phase increment-iteration
    "$UPDATE_STATE" test-plan test-phase tests 7 10

    # State file should be valid JSON at every point
    jq empty "$PHASE_DIR/state.json"

    # No leftover .tmp files
    local tmp_count
    tmp_count=$(find "$PHASE_DIR" -name "*.tmp.*" 2>/dev/null | wc -l)
    [[ "$tmp_count" -eq 0 ]]
}

#=============================================================================
# Edge: Empty plan.md handled throughout pipeline.
#=============================================================================

@test "integration edge: empty plan.md flows through pipeline" {
    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test
version: 3
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    cat > "$PHASE_DIR/state.json" << 'JSON'
{"iteration": {"current": 1, "max": 25}}
JSON
    truncate -s 0 "$PHASE_DIR/plan.md"

    local context
    context=$(build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" "impl")

    local plan_md
    plan_md=$(echo "$context" | jq -r '.plan_md')
    [[ "$plan_md" == "" ]]

    # Template with conditional on plan_md handles empty gracefully
    cat > "$PROMPTS_DIR/feature/empty_plan.md" << 'TEMPLATE'
{{#if plan_md}}Plan: {{plan_md}}{{else}}No plan provided.{{/if}}
TEMPLATE

    local rendered
    rendered=$(render_template "$PROMPTS_DIR/feature/empty_plan.md" "$context" "$PROMPTS_DIR")
    [[ "$rendered" == *"No plan provided."* ]]
}

#=============================================================================
# Edge: Special characters in plan.md survive full pipeline.
#=============================================================================

@test "integration edge: special characters in plan.md survive pipeline" {
    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test
version: 3
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    cat > "$PHASE_DIR/state.json" << 'JSON'
{"iteration": {"current": 1, "max": 25}}
JSON

    cat > "$PHASE_DIR/plan.md" << 'MD'
## Test "quotes" and $variables

Code block:
```rust
fn test() { println!("hello"); }
```

Special: <>&
MD

    local context
    context=$(build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" "impl")

    cat > "$PROMPTS_DIR/feature/special_test.md" << 'TEMPLATE'
{{plan_md}}
TEMPLATE

    local rendered
    rendered=$(render_template "$PROMPTS_DIR/feature/special_test.md" "$context" "$PROMPTS_DIR")

    [[ "$rendered" == *'"quotes"'* ]]
    [[ "$rendered" == *'$variables'* ]]
    [[ "$rendered" == *'println!("hello")'* ]]
    [[ "$rendered" == *'<>&'* ]]
}

#=============================================================================
# Edge: Version check gates V3 validation.
#=============================================================================

@test "integration edge: V1 workflow skips V3 validation entirely" {
    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test
version: 1
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    run "$VALIDATE_WORKFLOW" "$TEST_TEMP_DIR/workflow.yaml"
    [[ "$status" -eq 0 ]]
    # Should NOT mention V3 schema validation
    [[ "$output" != *"V3 schema validation"* ]]
}

#=============================================================================
# Edge: Unicode preserved through full pipeline.
#=============================================================================

@test "integration edge: unicode preserved through full pipeline" {
    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test
version: 3
variables:
  greeting: "Hola"
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    cat > "$PHASE_DIR/state.json" << 'JSON'
{"iteration": {"current": 1, "max": 25}}
JSON
    printf '测试 тест テスト' > "$PHASE_DIR/plan.md"

    local context
    context=$(build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" "impl")

    cat > "$PROMPTS_DIR/feature/unicode_test.md" << 'TEMPLATE'
Greeting: {{greeting}}
Content: {{plan_md}}
TEMPLATE

    local rendered
    rendered=$(render_template "$PROMPTS_DIR/feature/unicode_test.md" "$context" "$PROMPTS_DIR")
    [[ "$rendered" == *"Greeting: Hola"* ]]
    [[ "$rendered" == *"测试"* ]]
    [[ "$rendered" == *"тест"* ]]
    [[ "$rendered" == *"テスト"* ]]
}

#=============================================================================
# test_integration_validate_rejects_multiple_errors
# Validates that V3 validation reports all errors, not just the first.
#=============================================================================

@test "integration: validate reports multiple V3 errors together" {
    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test
version: 3
defaults:
  max_iterations: "not_a_number"
  state: "reserved in defaults"
variables:
  plan: "reserved in variables"
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
    params:
      bad-key: "invalid identifier"
      iteration: "reserved in params"
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    run "$VALIDATE_WORKFLOW" "$TEST_TEMP_DIR/workflow.yaml"
    [[ "$status" -ne 0 ]]
    # Multiple errors reported
    [[ "$output" == *"max_iterations"* ]]
    [[ "$output" == *"state"* ]]
    [[ "$output" == *"plan"* ]]
    [[ "$output" == *"bad-key"* ]]
    [[ "$output" == *"iteration"* ]]
}

#=============================================================================
# test_integration_old_and_new_state_schemas
# Both old (nested) and new (flat) state.json schemas produce same iteration.
#=============================================================================

@test "integration: old and new state schemas produce same iteration" {
    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test
version: 3
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML
    printf '# Plan\n' > "$PHASE_DIR/plan.md"

    # Old schema: iteration is nested object
    echo '{"iteration": {"current": 3, "max": 25}, "phase_status": "impl"}' > "$PHASE_DIR/state.json"
    local ctx_old
    ctx_old=$(build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" "impl")
    local iter_old
    iter_old=$(echo "$ctx_old" | jq '.iteration')

    # New schema: iteration is flat number
    echo '{"iteration": 3, "current_state": "impl"}' > "$PHASE_DIR/state.json"
    local ctx_new
    ctx_new=$(build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" "impl")
    local iter_new
    iter_new=$(echo "$ctx_new" | jq '.iteration')

    # Both produce iteration = 3
    [[ "$iter_old" == "3" ]]
    [[ "$iter_new" == "3" ]]
    [[ "$iter_old" == "$iter_new" ]]
}

#=============================================================================
# test_integration_context_json_contract
# Verifies build-context.sh output is valid JSON consumed by render_template.py.
#=============================================================================

@test "integration: context JSON contract between build-context and render" {
    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test
version: 3
defaults:
  max_iterations: 10
variables:
  package: test-package
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
    params:
      focus_area: "Core"
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    cat > "$PHASE_DIR/state.json" << 'JSON'
{"iteration": {"current": 2, "max": 25}, "phase_status": "impl", "tests_passing": 4, "tests_total": 8}
JSON
    printf '# Plan\n' > "$PHASE_DIR/plan.md"

    # Step 1: Build context
    local context
    context=$(build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" "impl")

    # Step 2: Context is valid JSON
    echo "$context" | jq empty

    # Step 3: Pass context to render — must not error
    cat > "$PROMPTS_DIR/feature/contract_test.md" << 'TEMPLATE'
Phase: {{phase}}, Crate: {{crate}}, Iter: {{iteration}}
Focus: {{params.focus_area}}
Tests: {{state.tests_passing}}/{{state.tests_total}}
TEMPLATE

    local rendered
    rendered=$(render_template "$PROMPTS_DIR/feature/contract_test.md" "$context" "$PROMPTS_DIR")

    [[ "$rendered" == *"Phase: test-phase"* ]]
    [[ "$rendered" == *"Crate: test-package"* ]]
    [[ "$rendered" == *"Iter: 2"* ]]
    [[ "$rendered" == *"Focus: Core"* ]]
    [[ "$rendered" == *"Tests: 4/8"* ]]
}
