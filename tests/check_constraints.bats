#!/usr/bin/env bats

# Tests for check-constraints.sh
# Phase: constraint-validation (orchestration-v4)
#
# Tests constraint validation for V4 workflow states:
#   - max_iterations enforcement
#   - require_artifacts_in (pre-execution)
#   - require_artifacts_out (post-execution)

setup() {
    load 'test_helper'
    setup_temp_dir

    CHECK_CONSTRAINTS_SH="$SCRIPTS_DIR/check-constraints.sh"

    # Standard phase directory structure
    export PHASE_DIR="$TEST_TEMP_DIR/phase"
    mkdir -p "$PHASE_DIR"

    # Create minimal state.json
    export STATE_FILE="$PHASE_DIR/state.json"
    cat > "$STATE_FILE" << 'JSON'
{
    "iteration": 0,
    "current_state": "impl"
}
JSON
}

teardown() {
    teardown_temp_dir
}

# Helper: create a workflow.yaml with a given state and optional constraints block
# Usage: create_constrained_workflow <state_name> [constraints_yaml]
# If no constraints_yaml given, no constraints section is added.
create_constrained_workflow() {
    local state_name="${1:-impl}"
    local constraints_yaml="${2:-}"
    local output_file="$TEST_TEMP_DIR/workflow.yaml"

    if [[ -z "$constraints_yaml" ]]; then
        cat > "$output_file" << YAML
name: test_workflow
version: 4
states:
  - name: ${state_name}
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: ${state_name}
terminal_states: [complete]
YAML
    else
        cat > "$output_file" << YAML
name: test_workflow
version: 4
states:
  - name: ${state_name}
    prompt: prompts/feature/impl.md
    next: complete
    constraints:
${constraints_yaml}
  - name: complete
    prompt: prompts/common/complete.md
entry_state: ${state_name}
terminal_states: [complete]
YAML
    fi

    echo "$output_file"
}

# Helper: create a workflow with multiple states, only one having constraints
# Usage: create_multi_state_workflow <constrained_state> <constraints_yaml>
create_multi_state_workflow() {
    local constrained_state="$1"
    local constraints_yaml="$2"
    local output_file="$TEST_TEMP_DIR/workflow.yaml"

    cat > "$output_file" << YAML
name: test_workflow
version: 4
states:
  - name: simple_state
    prompt: prompts/feature/qa.md
    next: ${constrained_state}
  - name: ${constrained_state}
    prompt: prompts/feature/impl.md
    next: complete
    constraints:
${constraints_yaml}
  - name: complete
    prompt: prompts/common/complete.md
entry_state: simple_state
terminal_states: [complete]
YAML

    echo "$output_file"
}

# Helper: source check-constraints.sh and run a function
# Usage: run_constraint_fn <function_name> [args...]
run_constraint_fn() {
    local func="$1"
    shift
    run bash -c "
        export STATE_FILE=\"$STATE_FILE\"
        source \"$CHECK_CONSTRAINTS_SH\"
        $func \"\$@\"
    " -- "$@"
}

# Helper: run a constraint function with STATE_FILE unset
# Usage: run_constraint_fn_unset_state <function_name> [args...]
run_constraint_fn_unset_state() {
    local func="$1"
    shift
    run bash -c "
        unset STATE_FILE
        source \"$CHECK_CONSTRAINTS_SH\"
        $func \"\$@\"
    " -- "$@"
}

#=============================================================================
# Script Existence and Syntax Tests
#=============================================================================

@test "check-constraints.sh exists and is readable" {
    [[ -f "$CHECK_CONSTRAINTS_SH" ]]
}

@test "check-constraints.sh is syntactically valid bash" {
    run bash -n "$CHECK_CONSTRAINTS_SH"
    [[ "$status" -eq 0 ]]
}

@test "check-constraints.sh can be sourced without error" {
    run bash -c "source '$CHECK_CONSTRAINTS_SH'"
    [[ "$status" -eq 0 ]]
}

@test "check-constraints.sh does not use set -e" {
    [[ -f "$CHECK_CONSTRAINTS_SH" ]]
    run bash -c "grep -P '^set\\s+-(\\w*e\\w*)' '$CHECK_CONSTRAINTS_SH' | grep -v pipefail"
    [[ "$status" -eq 1 ]]  # No matches = good
}

@test "check-constraints.sh uses set -uo pipefail" {
    run bash -c "grep -E '^set -uo pipefail' '$CHECK_CONSTRAINTS_SH'"
    [[ "$status" -eq 0 ]]
}

@test "all constraint functions defined after sourcing" {
    run bash -c "
        source '$CHECK_CONSTRAINTS_SH'
        declare -f check_pre_constraints > /dev/null && echo 'pre:ok'
        declare -f check_post_constraints > /dev/null && echo 'post:ok'
        declare -f check_max_iterations > /dev/null && echo 'max_iter:ok'
        declare -f check_artifacts_exist > /dev/null && echo 'artifacts:ok'
        declare -f get_state_constraints > /dev/null && echo 'get_constraints:ok'
    "
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"pre:ok"* ]]
    [[ "$output" == *"post:ok"* ]]
    [[ "$output" == *"max_iter:ok"* ]]
    [[ "$output" == *"artifacts:ok"* ]]
    [[ "$output" == *"get_constraints:ok"* ]]
}

#=============================================================================
# get_state_constraints Tests
#=============================================================================

@test "test_get_state_constraints_exists: outputs JSON with constraint fields" {
    local wf
    wf=$(create_constrained_workflow "impl" "      max_iterations: 10
      require_artifacts_in:
        - qa_reasoning.md")

    run_constraint_fn get_state_constraints "$wf" "impl"
    [[ "$status" -eq 0 ]]

    # Output should be valid JSON
    echo "$output" | jq '.' > /dev/null
    [[ $? -eq 0 ]]

    # Should contain max_iterations
    local max_iter
    max_iter=$(echo "$output" | jq -r '.max_iterations')
    [[ "$max_iter" -eq 10 ]]

    # Should contain require_artifacts_in
    local artifacts_in
    artifacts_in=$(echo "$output" | jq -r '.require_artifacts_in[0]')
    [[ "$artifacts_in" == "qa_reasoning.md" ]]
}

@test "test_get_state_constraints_missing: outputs empty object for state without constraints" {
    local wf
    wf=$(create_constrained_workflow "simple_state")

    run_constraint_fn get_state_constraints "$wf" "simple_state"
    [[ "$status" -eq 0 ]]

    # Should be empty object
    local key_count
    key_count=$(echo "$output" | jq 'keys | length')
    [[ "$key_count" -eq 0 ]]
}

@test "test_get_state_constraints_nonexistent_state: outputs empty object for unknown state" {
    local wf
    wf=$(create_constrained_workflow "impl" "      max_iterations: 5")

    run_constraint_fn get_state_constraints "$wf" "nonexistent"
    [[ "$status" -eq 0 ]]

    local key_count
    key_count=$(echo "$output" | jq 'keys | length')
    [[ "$key_count" -eq 0 ]]
}

#=============================================================================
# check_max_iterations Tests
#=============================================================================

@test "test_pre_constraints_max_iterations_ok: exit 0 when iteration < max" {
    local wf
    wf=$(create_constrained_workflow "impl" "      max_iterations: 10")

    jq '.iteration = 5' "$STATE_FILE" > "${STATE_FILE}.tmp" && mv "${STATE_FILE}.tmp" "$STATE_FILE"

    run_constraint_fn check_pre_constraints "$wf" "impl" "$PHASE_DIR"
    [[ "$status" -eq 0 ]]
}

@test "test_pre_constraints_max_iterations_exceeded: exit 1 when iteration >= max" {
    local wf
    wf=$(create_constrained_workflow "impl" "      max_iterations: 10")

    jq '.iteration = 10' "$STATE_FILE" > "${STATE_FILE}.tmp" && mv "${STATE_FILE}.tmp" "$STATE_FILE"

    run_constraint_fn check_pre_constraints "$wf" "impl" "$PHASE_DIR"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Max iterations exceeded"* ]]
}

@test "test_max_iterations_boundary: exit 0 when iteration is one below max" {
    local wf
    wf=$(create_constrained_workflow "impl" "      max_iterations: 10")

    jq '.iteration = 9' "$STATE_FILE" > "${STATE_FILE}.tmp" && mv "${STATE_FILE}.tmp" "$STATE_FILE"

    run_constraint_fn check_pre_constraints "$wf" "impl" "$PHASE_DIR"
    [[ "$status" -eq 0 ]]
}

@test "test_max_iterations_at_boundary: exit 1 when iteration equals max" {
    local wf
    wf=$(create_constrained_workflow "impl" "      max_iterations: 10")

    jq '.iteration = 10' "$STATE_FILE" > "${STATE_FILE}.tmp" && mv "${STATE_FILE}.tmp" "$STATE_FILE"

    run_constraint_fn check_pre_constraints "$wf" "impl" "$PHASE_DIR"
    [[ "$status" -eq 1 ]]
}

@test "test_max_iterations_zero: exit 1 immediately when max_iterations is 0" {
    local wf
    wf=$(create_constrained_workflow "impl" "      max_iterations: 0")

    jq '.iteration = 0' "$STATE_FILE" > "${STATE_FILE}.tmp" && mv "${STATE_FILE}.tmp" "$STATE_FILE"

    run_constraint_fn check_pre_constraints "$wf" "impl" "$PHASE_DIR"
    [[ "$status" -eq 1 ]]
}

@test "test_state_file_not_set: exit 1 with error about STATE_FILE" {
    local wf
    wf=$(create_constrained_workflow "impl" "      max_iterations: 10")

    run_constraint_fn_unset_state check_max_iterations "$wf" "impl"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"STATE_FILE"* ]]
}

#=============================================================================
# check_pre_constraints Tests
#=============================================================================

@test "test_pre_constraints_no_constraints: exit 0 for state without constraints" {
    local wf
    wf=$(create_constrained_workflow "simple_state")

    run_constraint_fn check_pre_constraints "$wf" "simple_state" "$PHASE_DIR"
    [[ "$status" -eq 0 ]]
}

@test "test_pre_constraints_empty_constraints_object: exit 0 for empty constraints" {
    local output_file="$TEST_TEMP_DIR/workflow.yaml"
    cat > "$output_file" << 'YAML'
name: test_workflow
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
    constraints: {}
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    run_constraint_fn check_pre_constraints "$output_file" "impl" "$PHASE_DIR"
    [[ "$status" -eq 0 ]]
}

@test "test_pre_constraints_artifacts_in_exist: exit 0 when all input artifacts present" {
    local wf
    wf=$(create_constrained_workflow "impl" "      require_artifacts_in:
        - qa_reasoning.md")

    touch "$PHASE_DIR/qa_reasoning.md"

    run_constraint_fn check_pre_constraints "$wf" "impl" "$PHASE_DIR"
    [[ "$status" -eq 0 ]]
}

@test "test_pre_constraints_artifacts_in_missing: exit 1 listing missing artifact" {
    local wf
    wf=$(create_constrained_workflow "impl" "      require_artifacts_in:
        - qa_reasoning.md
        - test_plan.md")

    # Only create qa_reasoning.md, test_plan.md is missing
    touch "$PHASE_DIR/qa_reasoning.md"

    run_constraint_fn check_pre_constraints "$wf" "impl" "$PHASE_DIR"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"test_plan.md"* ]]
}

@test "test_multiple_constraints: exit 0 when all pre-constraints satisfied" {
    local wf
    wf=$(create_constrained_workflow "impl" "      max_iterations: 10
      require_artifacts_in:
        - qa_reasoning.md")

    jq '.iteration = 5' "$STATE_FILE" > "${STATE_FILE}.tmp" && mv "${STATE_FILE}.tmp" "$STATE_FILE"
    touch "$PHASE_DIR/qa_reasoning.md"

    run_constraint_fn check_pre_constraints "$wf" "impl" "$PHASE_DIR"
    [[ "$status" -eq 0 ]]
}

@test "test_artifacts_in_subdirectory: exit 0 for artifacts in subdirectories" {
    local wf
    wf=$(create_constrained_workflow "review" "      require_artifacts_in:
        - iteration_001/output.txt")

    mkdir -p "$PHASE_DIR/iteration_001"
    touch "$PHASE_DIR/iteration_001/output.txt"

    run_constraint_fn check_pre_constraints "$wf" "review" "$PHASE_DIR"
    [[ "$status" -eq 0 ]]
}

@test "test_artifact_path_with_spaces: exit 0 for artifacts with spaces in name" {
    local wf
    wf=$(create_constrained_workflow "impl" "      require_artifacts_in:
        - my artifact.md")

    touch "$PHASE_DIR/my artifact.md"

    run_constraint_fn check_pre_constraints "$wf" "impl" "$PHASE_DIR"
    [[ "$status" -eq 0 ]]
}

@test "test_invalid_workflow_yaml: exit 1 on malformed YAML" {
    local output_file="$TEST_TEMP_DIR/workflow.yaml"
    cat > "$output_file" << 'YAML'
name: malformed
states:
  - name: impl
    bad indentation here
  next: complete
YAML

    run_constraint_fn check_pre_constraints "$output_file" "impl" "$PHASE_DIR"
    [[ "$status" -ne 0 ]]
}

#=============================================================================
# check_post_constraints Tests
#=============================================================================

@test "test_post_constraints_artifacts_out_exist: exit 0 when output artifacts present" {
    local wf
    wf=$(create_constrained_workflow "impl" "      require_artifacts_out:
        - impl_reasoning.md")

    touch "$PHASE_DIR/impl_reasoning.md"

    run_constraint_fn check_post_constraints "$wf" "impl" "$PHASE_DIR"
    [[ "$status" -eq 0 ]]
}

@test "test_post_constraints_artifacts_out_missing: exit 1 listing all missing files" {
    local wf
    wf=$(create_constrained_workflow "impl" "      require_artifacts_out:
        - impl_reasoning.md
        - changes.diff")

    # Neither file exists

    run_constraint_fn check_post_constraints "$wf" "impl" "$PHASE_DIR"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"impl_reasoning.md"* ]]
    [[ "$output" == *"changes.diff"* ]]
}

@test "test_post_constraints_empty_constraints_object: exit 0 for empty constraints" {
    local output_file="$TEST_TEMP_DIR/workflow.yaml"
    cat > "$output_file" << 'YAML'
name: test_workflow
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
    constraints: {}
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    run_constraint_fn check_post_constraints "$output_file" "impl" "$PHASE_DIR"
    [[ "$status" -eq 0 ]]
}

@test "test_post_constraints_no_constraints: exit 0 for state without constraints" {
    local wf
    wf=$(create_constrained_workflow "simple_state")

    run_constraint_fn check_post_constraints "$wf" "simple_state" "$PHASE_DIR"
    [[ "$status" -eq 0 ]]
}

@test "test_post_constraints_artifacts_in_subdirectory: exit 0 for nested output artifacts" {
    local wf
    wf=$(create_constrained_workflow "impl" "      require_artifacts_out:
        - iteration_001/output.txt")

    mkdir -p "$PHASE_DIR/iteration_001"
    touch "$PHASE_DIR/iteration_001/output.txt"

    run_constraint_fn check_post_constraints "$wf" "impl" "$PHASE_DIR"
    [[ "$status" -eq 0 ]]
}

#=============================================================================
# check_artifacts_exist Direct Tests
#=============================================================================

@test "test_check_artifacts_exist_direct_all_exist: exit 0 when all files present" {
    touch "$PHASE_DIR/file1.md"
    touch "$PHASE_DIR/file2.md"

    run_constraint_fn check_artifacts_exist "$PHASE_DIR" '["file1.md", "file2.md"]'
    [[ "$status" -eq 0 ]]
}

@test "test_check_artifacts_exist_direct_some_missing: exit 1 listing missing file" {
    touch "$PHASE_DIR/file1.md"
    # file2.md does not exist

    run_constraint_fn check_artifacts_exist "$PHASE_DIR" '["file1.md", "file2.md"]'
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"file2.md"* ]]
}

@test "test_check_artifacts_exist_direct_empty_array: exit 0 for no artifacts" {
    run_constraint_fn check_artifacts_exist "$PHASE_DIR" '[]'
    [[ "$status" -eq 0 ]]
}

@test "test_check_artifacts_exist_directory_not_file: exit 1 for directory instead of file" {
    mkdir -p "$PHASE_DIR/output"

    run_constraint_fn check_artifacts_exist "$PHASE_DIR" '["output"]'
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"output"* ]]
}

#=============================================================================
# Multi-state Workflow Tests
#=============================================================================

@test "pre-constraints only check the specified state's constraints" {
    local wf
    wf=$(create_multi_state_workflow "impl" "      max_iterations: 10
      require_artifacts_in:
        - qa_reasoning.md")

    jq '.iteration = 5' "$STATE_FILE" > "${STATE_FILE}.tmp" && mv "${STATE_FILE}.tmp" "$STATE_FILE"

    # simple_state has no constraints — should pass
    run_constraint_fn check_pre_constraints "$wf" "simple_state" "$PHASE_DIR"
    [[ "$status" -eq 0 ]]
}

@test "pre-constraints fail when constrained state's artifacts missing" {
    local wf
    wf=$(create_multi_state_workflow "impl" "      max_iterations: 10
      require_artifacts_in:
        - qa_reasoning.md")

    jq '.iteration = 5' "$STATE_FILE" > "${STATE_FILE}.tmp" && mv "${STATE_FILE}.tmp" "$STATE_FILE"

    # impl requires qa_reasoning.md — should fail
    run_constraint_fn check_pre_constraints "$wf" "impl" "$PHASE_DIR"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"qa_reasoning.md"* ]]
}

#=============================================================================
# Combined Constraint Scenarios
#=============================================================================

@test "pre-constraints: max_iterations fails even when artifacts exist" {
    local wf
    wf=$(create_constrained_workflow "impl" "      max_iterations: 3
      require_artifacts_in:
        - qa_reasoning.md")

    jq '.iteration = 5' "$STATE_FILE" > "${STATE_FILE}.tmp" && mv "${STATE_FILE}.tmp" "$STATE_FILE"
    touch "$PHASE_DIR/qa_reasoning.md"

    run_constraint_fn check_pre_constraints "$wf" "impl" "$PHASE_DIR"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Max iterations exceeded"* ]]
}

@test "post-constraints: only checks artifacts_out, ignores max_iterations" {
    local wf
    wf=$(create_constrained_workflow "impl" "      max_iterations: 3
      require_artifacts_out:
        - impl_reasoning.md")

    # Iteration exceeds max — but post-constraints don't check that
    jq '.iteration = 5' "$STATE_FILE" > "${STATE_FILE}.tmp" && mv "${STATE_FILE}.tmp" "$STATE_FILE"
    touch "$PHASE_DIR/impl_reasoning.md"

    run_constraint_fn check_post_constraints "$wf" "impl" "$PHASE_DIR"
    [[ "$status" -eq 0 ]]
}

@test "post-constraints: only checks artifacts_out, ignores artifacts_in" {
    local wf
    wf=$(create_constrained_workflow "impl" "      require_artifacts_in:
        - qa_reasoning.md
      require_artifacts_out:
        - impl_reasoning.md")

    # qa_reasoning.md missing — but post-constraints only check artifacts_out
    touch "$PHASE_DIR/impl_reasoning.md"

    run_constraint_fn check_post_constraints "$wf" "impl" "$PHASE_DIR"
    [[ "$status" -eq 0 ]]
}

#=============================================================================
# Error Message Quality
#=============================================================================

@test "max_iterations error includes state name and counts" {
    local wf
    wf=$(create_constrained_workflow "impl" "      max_iterations: 5")

    jq '.iteration = 7' "$STATE_FILE" > "${STATE_FILE}.tmp" && mv "${STATE_FILE}.tmp" "$STATE_FILE"

    run_constraint_fn check_max_iterations "$wf" "impl"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"impl"* ]]
    [[ "$output" == *"7"* ]]
    [[ "$output" == *"5"* ]]
}

@test "missing artifacts error lists each missing file" {
    local wf
    wf=$(create_constrained_workflow "impl" "      require_artifacts_in:
        - file_a.md
        - file_b.md
        - file_c.md")

    # Only file_a.md exists
    touch "$PHASE_DIR/file_a.md"

    run_constraint_fn check_pre_constraints "$wf" "impl" "$PHASE_DIR"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"file_b.md"* ]]
    [[ "$output" == *"file_c.md"* ]]
    # file_a.md should NOT appear in missing list
    [[ "$output" != *"file_a.md"* ]]
}
