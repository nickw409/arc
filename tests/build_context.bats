#!/usr/bin/env bats

# Tests for build-context.sh
# Phase: context-building (orchestration-v3)
#
# Tests the context builder that merges workflow defaults, variables,
# state params, state file values, and computed values into a JSON context
# for template rendering.

setup() {
    load 'test_helper'
    setup_temp_dir

    BUILD_CONTEXT="$SCRIPTS_DIR/build-context.sh"

    # Standard phase directory structure
    export PHASE_DIR="$TEST_TEMP_DIR/.plans/active/test-plan/phases/test-phase"
    mkdir -p "$PHASE_DIR"

    # Create minimal state.json (future/flat schema)
    echo '{"iteration": 1, "current_state": "impl"}' > "$PHASE_DIR/state.json"

    # Create minimal workflow.yaml (V3)
    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test
version: 3
defaults:
  max_iterations: 10
states:
  - name: impl
    prompt: prompts/impl.md
entry_state: impl
terminal_states: [complete]
YAML

    # Create plan.md
    printf '# Test Phase\n' > "$PHASE_DIR/plan.md"
}

teardown() {
    teardown_temp_dir
}

# Helper: run build-context.sh with default test fixtures
# Usage: build_context [state_file] [workflow_file] [phase_dir] [state_name]
build_context() {
    local state_file="${1:-$PHASE_DIR/state.json}"
    local workflow_file="${2:-$TEST_TEMP_DIR/workflow.yaml}"
    local phase_dir="${3:-$PHASE_DIR}"
    local state_name="${4:-impl}"
    run "$BUILD_CONTEXT" "$state_file" "$workflow_file" "$phase_dir" "$state_name"
}

# Helper: extract a JSON field from output
# Usage: get_field ".field.path"
get_field() {
    echo "$output" | jq -r "$1"
}

# Helper: assert JSON field equals expected value
# Usage: assert_json_field ".field" "expected"
assert_json_field() {
    local actual
    actual=$(echo "$output" | jq -r "$1")
    [[ "$actual" == "$2" ]]
}

# Helper: assert JSON field equals expected value (raw jq, no -r)
assert_json_raw() {
    local actual
    actual=$(echo "$output" | jq "$1")
    [[ "$actual" == "$2" ]]
}

#=============================================================================
# Script Existence and Syntax Tests
#=============================================================================

@test "build-context.sh exists and is executable" {
    [[ -x "$BUILD_CONTEXT" ]]
}

@test "build-context.sh is syntactically valid bash" {
    run bash -n "$BUILD_CONTEXT"
    [[ "$status" -eq 0 ]]
}

#=============================================================================
# Argument Validation Tests
#=============================================================================

@test "test_missing_arguments: exit 1 with usage on too few arguments" {
    run "$BUILD_CONTEXT" "$PHASE_DIR/state.json"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Usage:"* ]]
}

@test "test_no_arguments: exit 1 with usage on no arguments" {
    run "$BUILD_CONTEXT"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Usage:"* ]]
}

@test "test_three_arguments: exit 1 with usage on three arguments" {
    run "$BUILD_CONTEXT" "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Usage:"* ]]
}

#=============================================================================
# Missing/Invalid File Tests
#=============================================================================

@test "test_missing_state_file: exit 1 when state file does not exist" {
    run "$BUILD_CONTEXT" "$TEST_TEMP_DIR/nonexistent.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" impl
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Error"* ]]
    [[ "$output" == *"State file"* ]] || [[ "$output" == *"state"* ]]
}

@test "test_missing_workflow_file: exit 1 when workflow file does not exist" {
    run "$BUILD_CONTEXT" "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/nonexistent.yaml" "$PHASE_DIR" impl
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Error"* ]]
    [[ "$output" == *"Workflow"* ]] || [[ "$output" == *"workflow"* ]]
}

@test "test_invalid_state_json: exit 1 on invalid JSON in state file" {
    echo '{not valid json' > "$PHASE_DIR/state.json"
    build_context
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Error"* ]]
    [[ "$output" == *"JSON"* ]] || [[ "$output" == *"json"* ]] || [[ "$output" == *"Invalid"* ]]
}

@test "test_invalid_workflow_yaml: exit 1 on invalid YAML in workflow file" {
    # Create truly broken YAML
    printf 'name: test\n  bad:\n indentation\n   here:\ntabs\there\n' > "$TEST_TEMP_DIR/bad_workflow.yaml"
    run "$BUILD_CONTEXT" "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/bad_workflow.yaml" "$PHASE_DIR" impl
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Error"* ]] || [[ "$output" == *"error"* ]] || [[ "$output" == *"YAML"* ]]
}

#=============================================================================
# Basic Context Build Tests
#=============================================================================

@test "test_basic_context_build: merges defaults, state, and computed values" {
    # Setup: flat iteration schema, simple workflow
    echo '{"iteration": 1, "current_state": "impl"}' > "$PHASE_DIR/state.json"
    printf '# Test Phase\n' > "$PHASE_DIR/plan.md"

    build_context
    [[ "$status" -eq 0 ]]

    # max_iterations from defaults
    assert_json_raw '.max_iterations' '10'

    # iteration computed from state.json (flat schema)
    assert_json_raw '.iteration' '1'

    # current_state from argument (4th arg = "impl")
    assert_json_field '.current_state' 'impl'

    # plan_md from plan.md file
    [[ "$(get_field '.plan_md')" == *"# Test Phase"* ]]

    # phase and plan derived from path
    assert_json_field '.phase' 'test-phase'
    assert_json_field '.plan' 'test-plan'
}

@test "test_basic_context_build_old_schema: handles old iteration schema" {
    echo '{"iteration": {"current": 1, "max": 25}, "phase_status": "impl"}' > "$PHASE_DIR/state.json"

    build_context
    [[ "$status" -eq 0 ]]

    # iteration extracted from .iteration.current
    assert_json_raw '.iteration' '1'

    # max_iterations comes from defaults (10), NOT from state.json's .iteration.max (25)
    assert_json_raw '.max_iterations' '10'

    # current_state from argument, not state.json
    assert_json_field '.current_state' 'impl'

    # plan_md from file
    [[ "$(get_field '.plan_md')" == *"# Test Phase"* ]]
}

#=============================================================================
# Defaults Merge Tests
#=============================================================================

@test "test_defaults_merged: defaults from workflow appear in output" {
    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test
version: 3
defaults:
  timeout: 600
  retries: 3
states:
  - name: impl
    prompt: prompts/impl.md
entry_state: impl
terminal_states: [complete]
YAML

    build_context
    [[ "$status" -eq 0 ]]
    assert_json_raw '.timeout' '600'
    assert_json_raw '.retries' '3'
}

@test "test_workflow_without_defaults: succeeds when no defaults section" {
    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test
version: 3
states:
  - name: impl
    prompt: prompts/impl.md
entry_state: impl
terminal_states: [complete]
YAML

    build_context
    [[ "$status" -eq 0 ]]
    # Should still have computed values
    assert_json_field '.current_state' 'impl'
}

#=============================================================================
# Variables Merge Tests
#=============================================================================

@test "test_variables_merged: variables from workflow appear in output" {
    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test
version: 3
variables:
  package: "test-package"
  test_pattern: "qa_*"
states:
  - name: impl
    prompt: prompts/impl.md
entry_state: impl
terminal_states: [complete]
YAML

    build_context
    [[ "$status" -eq 0 ]]
    assert_json_field '.crate' 'test-package'
    assert_json_field '.test_pattern' 'qa_*'
}

@test "test_workflow_without_variables: succeeds when no variables section" {
    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test
version: 3
defaults:
  max_iterations: 10
states:
  - name: impl
    prompt: prompts/impl.md
entry_state: impl
terminal_states: [complete]
YAML

    build_context
    [[ "$status" -eq 0 ]]
    assert_json_raw '.max_iterations' '10'
}

#=============================================================================
# Variables Override Defaults Tests
#=============================================================================

@test "test_variables_override_defaults: variables take precedence over defaults" {
    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test
version: 3
defaults:
  timeout: 600
variables:
  timeout: 900
states:
  - name: impl
    prompt: prompts/impl.md
entry_state: impl
terminal_states: [complete]
YAML

    build_context
    [[ "$status" -eq 0 ]]
    assert_json_raw '.timeout' '900'
}

@test "test_defaults_and_variables_same_key: variables override defaults for same key" {
    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test
version: 3
defaults:
  timeout: 600
variables:
  timeout: 900
states:
  - name: impl
    prompt: prompts/impl.md
entry_state: impl
terminal_states: [complete]
YAML

    build_context
    [[ "$status" -eq 0 ]]
    assert_json_raw '.timeout' '900'
}

#=============================================================================
# State Params Tests
#=============================================================================

@test "test_state_params_nested: params from workflow state are nested under params key" {
    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test
version: 3
states:
  - name: impl
    prompt: prompts/impl.md
    params:
      allow_test_changes: false
      focus_area: "RNG"
entry_state: impl
terminal_states: [complete]
YAML

    build_context
    [[ "$status" -eq 0 ]]
    assert_json_raw '.params.allow_test_changes' 'false'
    assert_json_field '.params.focus_area' 'RNG'
}

@test "test_params_nested_not_toplevel: params do NOT appear at top level" {
    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test
version: 3
states:
  - name: impl
    prompt: prompts/impl.md
    params:
      focus_area: "core"
entry_state: impl
terminal_states: [complete]
YAML

    build_context
    [[ "$status" -eq 0 ]]
    # Nested under params key
    assert_json_field '.params.focus_area' 'core'
    # NOT at top level
    local top_level
    top_level=$(echo "$output" | jq -r '.focus_area // "MISSING"')
    [[ "$top_level" == "MISSING" ]]
}

@test "test_state_without_params: succeeds when state has no params section" {
    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test
version: 3
states:
  - name: impl
    prompt: prompts/impl.md
entry_state: impl
terminal_states: [complete]
YAML

    build_context
    [[ "$status" -eq 0 ]]
    # params should be empty object
    local params
    params=$(echo "$output" | jq '.params')
    [[ "$params" == "{}" ]]
}

@test "test_wrong_state_name: succeeds with empty params for nonexistent state" {
    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test
version: 3
states:
  - name: qa
    prompt: prompts/qa.md
    params:
      strict: true
  - name: impl
    prompt: prompts/impl.md
    params:
      allow_test_changes: false
entry_state: qa
terminal_states: [complete]
YAML

    run "$BUILD_CONTEXT" "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" nonexistent
    [[ "$status" -eq 0 ]]
    local params
    params=$(echo "$output" | jq '.params')
    [[ "$params" == "{}" ]]
}

@test "test_complex_nested_params: handles nested objects and arrays in params" {
    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test
version: 3
states:
  - name: impl
    prompt: prompts/impl.md
    params:
      files:
        - path: src/lib.rs
          desc: Main lib
        - path: src/utils.rs
          desc: Utilities
entry_state: impl
terminal_states: [complete]
YAML

    build_context
    [[ "$status" -eq 0 ]]
    # Verify nested array of objects
    local file_count
    file_count=$(echo "$output" | jq '.params.files | length')
    [[ "$file_count" -eq 2 ]]
    assert_json_field '.params.files[0].path' 'src/lib.rs'
    assert_json_field '.params.files[0].desc' 'Main lib'
    assert_json_field '.params.files[1].path' 'src/utils.rs'
    assert_json_field '.params.files[1].desc' 'Utilities'
}

#=============================================================================
# State Object Tests
#=============================================================================

@test "test_state_object_included: full state.json is nested under state key" {
    echo '{"iteration": 5, "current_state": "impl", "tests_passing": 10, "tests_total": 12}' > "$PHASE_DIR/state.json"

    build_context
    [[ "$status" -eq 0 ]]
    assert_json_raw '.state.iteration' '5'
    assert_json_field '.state.current_state' 'impl'
    assert_json_raw '.state.tests_passing' '10'
    assert_json_raw '.state.tests_total' '12'
}

#=============================================================================
# Iteration Shorthand Tests
#=============================================================================

@test "test_iteration_shorthand_new_schema: iteration extracted from flat number" {
    echo '{"iteration": 7}' > "$PHASE_DIR/state.json"

    build_context
    [[ "$status" -eq 0 ]]
    # Top-level iteration = 7
    assert_json_raw '.iteration' '7'
    # state.iteration = 7
    assert_json_raw '.state.iteration' '7'
}

@test "test_iteration_shorthand_old_schema: iteration extracted from .iteration.current" {
    echo '{"iteration": {"current": 5, "max": 25}}' > "$PHASE_DIR/state.json"

    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test
version: 3
defaults:
  max_iterations: 10
states:
  - name: impl
    prompt: prompts/impl.md
entry_state: impl
terminal_states: [complete]
YAML

    build_context
    [[ "$status" -eq 0 ]]
    # Top-level iteration = 5 (scalar from .iteration.current)
    assert_json_raw '.iteration' '5'
    # max_iterations = 10 (from defaults, NOT from state.json .iteration.max = 25)
    assert_json_raw '.max_iterations' '10'
    # state object preserves original nested iteration
    assert_json_raw '.state.iteration.current' '5'
    assert_json_raw '.state.iteration.max' '25'
}

#=============================================================================
# current_state Tests
#=============================================================================

@test "test_current_state_from_argument_not_state_json: uses 4th argument" {
    echo '{"iteration": 1, "current_state": "old_stale_value", "phase_status": "another_stale_value"}' > "$PHASE_DIR/state.json"

    run "$BUILD_CONTEXT" "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" new_correct_value
    [[ "$status" -eq 0 ]]
    # current_state from argument, not state.json
    assert_json_field '.current_state' 'new_correct_value'
    # state object still has the stale values
    assert_json_field '.state.current_state' 'old_stale_value'
    assert_json_field '.state.phase_status' 'another_stale_value'
}

#=============================================================================
# Phase and Plan Derivation Tests
#=============================================================================

@test "test_phase_and_plan_derived: phase and plan from directory structure" {
    local custom_phase="$TEST_TEMP_DIR/.plans/active/my-feature/phases/implement-auth"
    mkdir -p "$custom_phase"
    echo '{"iteration": 1}' > "$custom_phase/state.json"
    printf '# Plan\n' > "$custom_phase/plan.md"

    run "$BUILD_CONTEXT" "$custom_phase/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$custom_phase" impl
    [[ "$status" -eq 0 ]]
    assert_json_field '.phase' 'implement-auth'
    assert_json_field '.plan' 'my-feature'
}

#=============================================================================
# plan_md Content Tests
#=============================================================================

@test "test_plan_md_content_included: plan.md content in output" {
    cat > "$PHASE_DIR/plan.md" << 'MD'
# Phase: test

## Objective
Test something.
MD

    build_context
    [[ "$status" -eq 0 ]]
    local plan_md
    plan_md=$(get_field '.plan_md')
    [[ "$plan_md" == *"# Phase: test"* ]]
    [[ "$plan_md" == *"## Objective"* ]]
    [[ "$plan_md" == *"Test something."* ]]
}

@test "test_missing_plan_md: empty string when plan.md does not exist" {
    rm -f "$PHASE_DIR/plan.md"

    build_context
    [[ "$status" -eq 0 ]]
    assert_json_field '.plan_md' ''
}

@test "test_empty_plan_md_zero_bytes: empty string when plan.md is 0 bytes" {
    truncate -s 0 "$PHASE_DIR/plan.md"

    build_context
    [[ "$status" -eq 0 ]]
    assert_json_field '.plan_md' ''
}

@test "test_multiline_plan_md: preserves multiline content" {
    # Create plan.md with many lines
    local i
    {
        echo "# Phase: multiline"
        echo ""
        echo "## Objective"
        echo "Test multiline content."
        echo ""
        echo "## Code block"
        echo '```rust'
        echo 'fn main() {'
        echo '    println!("hello");'
        echo '}'
        echo '```'
        for i in $(seq 1 90); do
            echo "Line $i of the plan."
        done
    } > "$PHASE_DIR/plan.md"

    build_context
    [[ "$status" -eq 0 ]]
    local plan_md
    plan_md=$(get_field '.plan_md')
    [[ "$plan_md" == *"# Phase: multiline"* ]]
    [[ "$plan_md" == *"## Objective"* ]]
    [[ "$plan_md" == *'fn main()'* ]]
    [[ "$plan_md" == *"Line 90 of the plan."* ]]
}

@test "test_special_characters_in_plan_md: JSON escapes special chars" {
    printf '"quotes" $variables `backticks` $(echo hello) \\backslash\n' > "$PHASE_DIR/plan.md"

    build_context
    [[ "$status" -eq 0 ]]
    local plan_md
    plan_md=$(get_field '.plan_md')
    [[ "$plan_md" == *'"quotes"'* ]]
    [[ "$plan_md" == *'$variables'* ]]
    [[ "$plan_md" == *'`backticks`'* ]]
    [[ "$plan_md" == *'$(echo hello)'* ]]
}

@test "test_plan_md_with_shell_characters: no shell expansion occurs" {
    printf '`backticks` $(echo hello) "quotes" $VAR\n' > "$PHASE_DIR/plan.md"

    build_context
    [[ "$status" -eq 0 ]]
    local plan_md
    plan_md=$(get_field '.plan_md')
    # All characters preserved literally, no shell expansion
    [[ "$plan_md" == *'$(echo hello)'* ]]
    [[ "$plan_md" == *'$VAR'* ]]
}

#=============================================================================
# max_iterations Tests
#=============================================================================

@test "test_max_iterations_not_computed: comes from merge chain not state.json" {
    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test
version: 3
defaults:
  max_iterations: 10
variables:
  max_iterations: 20
states:
  - name: impl
    prompt: prompts/impl.md
entry_state: impl
terminal_states: [complete]
YAML
    echo '{"iteration": {"current": 1, "max": 30}}' > "$PHASE_DIR/state.json"

    build_context
    [[ "$status" -eq 0 ]]
    # max_iterations = 20 (from variables, overriding defaults of 10)
    # NOT 30 from state.json's .iteration.max
    assert_json_raw '.max_iterations' '20'
    # state.json's .iteration.max is accessible via state object
    assert_json_raw '.state.iteration.max' '30'
}

@test "test_max_iterations_from_defaults: defaults provide max_iterations" {
    echo '{"iteration": {"current": 3, "max": 15}}' > "$PHASE_DIR/state.json"

    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test
version: 3
defaults:
  max_iterations: 25
states:
  - name: impl
    prompt: prompts/impl.md
entry_state: impl
terminal_states: [complete]
YAML

    build_context
    [[ "$status" -eq 0 ]]
    assert_json_raw '.max_iterations' '25'
    # Old schema .iteration.max = 15 is still accessible via state
    assert_json_raw '.state.iteration.max' '15'
}

@test "test_max_iterations_from_variables_overrides_defaults: variables win" {
    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test
version: 3
defaults:
  max_iterations: 10
variables:
  max_iterations: 20
states:
  - name: impl
    prompt: prompts/impl.md
entry_state: impl
terminal_states: [complete]
YAML
    echo '{"iteration": 3}' > "$PHASE_DIR/state.json"

    build_context
    [[ "$status" -eq 0 ]]
    assert_json_raw '.max_iterations' '20'
}

#=============================================================================
# Empty State / Missing Sections Tests
#=============================================================================

@test "test_empty_state_json: handles empty state object gracefully" {
    echo '{}' > "$PHASE_DIR/state.json"

    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test
version: 3
defaults:
  max_iterations: 10
variables:
  custom: "value"
states:
  - name: impl
    prompt: prompts/impl.md
entry_state: impl
terminal_states: [complete]
YAML

    build_context
    [[ "$status" -eq 0 ]]
    # Computed iteration = 0 (jq: null // null // 0 = 0)
    assert_json_raw '.iteration' '0'
    # state is empty object
    local state
    state=$(echo "$output" | jq '.state')
    [[ "$state" == "{}" ]]
    # Defaults and variables still merge
    assert_json_raw '.max_iterations' '10'
    assert_json_field '.custom' 'value'
}

#=============================================================================
# Data Type Preservation Tests
#=============================================================================

@test "test_null_values_preserved: null in state.json preserved as JSON null" {
    echo '{"iteration": 1, "custom_field": null}' > "$PHASE_DIR/state.json"

    build_context
    [[ "$status" -eq 0 ]]
    local null_val
    null_val=$(echo "$output" | jq '.state.custom_field')
    [[ "$null_val" == "null" ]]
}

@test "test_boolean_values_preserved: booleans in state.json preserved as JSON booleans" {
    echo '{"iteration": 1, "passing": true, "failed": false}' > "$PHASE_DIR/state.json"

    build_context
    [[ "$status" -eq 0 ]]
    assert_json_raw '.state.passing' 'true'
    assert_json_raw '.state.failed' 'false'
}

#=============================================================================
# Unicode Handling Tests
#=============================================================================

@test "test_unicode_preserved: unicode in plan.md and variables preserved" {
    printf '测试 тест テスト 🚀\n' > "$PHASE_DIR/plan.md"

    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test
version: 3
variables:
  name: "日本語"
states:
  - name: impl
    prompt: prompts/impl.md
entry_state: impl
terminal_states: [complete]
YAML

    build_context
    [[ "$status" -eq 0 ]]
    assert_json_field '.name' '日本語'
    local plan_md
    plan_md=$(get_field '.plan_md')
    [[ "$plan_md" == *"测试"* ]]
    [[ "$plan_md" == *"тест"* ]]
    [[ "$plan_md" == *"テスト"* ]]
    [[ "$plan_md" == *"🚀"* ]]
}

#=============================================================================
# Full Merge Precedence / Integration Tests
#=============================================================================

@test "full merge: defaults + variables + params + state + computed all present" {
    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test
version: 3
defaults:
  max_iterations: 10
  timeout: 600
variables:
  package: "test-package"
  timeout: 900
states:
  - name: impl
    prompt: prompts/impl.md
    params:
      allow_test_changes: false
      focus_area: "RNG"
entry_state: impl
terminal_states: [complete]
YAML
    echo '{"iteration": 3, "tests_passing": 5, "tests_total": 10}' > "$PHASE_DIR/state.json"
    printf '# Full Merge Test\n' > "$PHASE_DIR/plan.md"

    build_context
    [[ "$status" -eq 0 ]]

    # Defaults
    assert_json_raw '.max_iterations' '10'
    # Variables override defaults
    assert_json_raw '.timeout' '900'
    # Variables
    assert_json_field '.crate' 'test-package'
    # Params nested
    assert_json_raw '.params.allow_test_changes' 'false'
    assert_json_field '.params.focus_area' 'RNG'
    # State object
    assert_json_raw '.state.iteration' '3'
    assert_json_raw '.state.tests_passing' '5'
    # Computed
    assert_json_raw '.iteration' '3'
    assert_json_field '.current_state' 'impl'
    assert_json_field '.phase' 'test-phase'
    assert_json_field '.plan' 'test-plan'
    [[ "$(get_field '.plan_md')" == *"# Full Merge Test"* ]]
}

@test "computed values cannot be overridden by defaults or variables" {
    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test
version: 3
defaults:
  iteration: 999
  current_state: "should_be_overridden"
  phase: "should_be_overridden"
variables:
  plan: "should_be_overridden"
states:
  - name: impl
    prompt: prompts/impl.md
entry_state: impl
terminal_states: [complete]
YAML
    echo '{"iteration": 5}' > "$PHASE_DIR/state.json"

    build_context
    [[ "$status" -eq 0 ]]
    # Computed values always win
    assert_json_raw '.iteration' '5'
    assert_json_field '.current_state' 'impl'
    assert_json_field '.phase' 'test-phase'
    assert_json_field '.plan' 'test-plan'
}

#=============================================================================
# Output Validity Tests
#=============================================================================

@test "output is valid JSON" {
    build_context
    [[ "$status" -eq 0 ]]
    echo "$output" | jq empty
    [[ $? -eq 0 ]]
}

@test "output contains all required computed fields" {
    build_context
    [[ "$status" -eq 0 ]]

    # All computed fields exist
    echo "$output" | jq -e '.iteration' > /dev/null
    echo "$output" | jq -e '.current_state' > /dev/null
    echo "$output" | jq -e '.phase' > /dev/null
    echo "$output" | jq -e '.plan' > /dev/null
    echo "$output" | jq -e 'has("plan_md")' > /dev/null
    echo "$output" | jq -e 'has("state")' > /dev/null
    echo "$output" | jq -e 'has("params")' > /dev/null
}

#=============================================================================
# Edge Case: Large plan.md
#=============================================================================

@test "large plan.md: handles multi-KB content" {
    # Generate ~50KB plan.md
    {
        echo "# Large Plan"
        local i
        for i in $(seq 1 1000); do
            echo "This is line $i of a very large plan document with some detailed content."
        done
    } > "$PHASE_DIR/plan.md"

    build_context
    [[ "$status" -eq 0 ]]
    local plan_md
    plan_md=$(get_field '.plan_md')
    [[ "$plan_md" == *"# Large Plan"* ]]
    [[ "$plan_md" == *"line 1000"* ]]
}

#=============================================================================
# Edge Case: State with deeply nested data
#=============================================================================

@test "deeply nested state.json preserved" {
    echo '{"iteration": 1, "nested": {"a": {"b": {"c": {"d": "deep"}}}}}' > "$PHASE_DIR/state.json"

    build_context
    [[ "$status" -eq 0 ]]
    assert_json_field '.state.nested.a.b.c.d' 'deep'
}

#=============================================================================
# Edge Case: State with arrays
#=============================================================================

@test "state.json arrays preserved" {
    echo '{"iteration": 1, "test_files": ["a.bats", "b.bats"], "disputes": []}' > "$PHASE_DIR/state.json"

    build_context
    [[ "$status" -eq 0 ]]
    local file_count
    file_count=$(echo "$output" | jq '.state.test_files | length')
    [[ "$file_count" -eq 2 ]]
    assert_json_field '.state.test_files[0]' 'a.bats'
    local dispute_count
    dispute_count=$(echo "$output" | jq '.state.disputes | length')
    [[ "$dispute_count" -eq 0 ]]
}
