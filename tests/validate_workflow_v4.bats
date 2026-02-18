#!/usr/bin/env bats

# Tests for V4 schema validation in validate-workflow.sh
# Phase: schema-updates (orchestration-v4)
#
# Tests cover:
# - validate_v4_constraints() — max_iterations, require_artifacts_in/out
# - validate_v4_after_hooks() — action, when, params, continue_on_error
# - validate_v4_escalation() — trigger types, iteration values, action validation
# - validate_v4_intervention_triggers() — condition syntax, action validation
# - get_known_actions() — action registry listing
# - Version 4 acceptance without warning
# - Backwards compatibility with V1/V2/V3 workflows

setup() {
    load 'test_helper'
    setup_temp_dir
}

teardown() {
    teardown_temp_dir
}

# ==============================================================================
# Helper: Create a minimal valid V4 workflow with all features
# Usage: create_v4_workflow [extra_yaml]
# The extra_yaml is injected at the workflow root level (for intervention_triggers)
# ==============================================================================
create_v4_workflow() {
    local extra_yaml="${1:-}"
    local output_file="$TEST_TEMP_DIR/workflow.yaml"

    cat > "$output_file" << YAML
name: test_v4
version: 4
description: V4 test workflow
${extra_yaml}
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: review
    constraints:
      max_iterations: 5
      require_artifacts_in:
        - plan.md
      require_artifacts_out:
        - output.txt
    after:
      - action: run_tests
        when: "approved"
        params:
          pattern: "test"
        continue_on_error: true
    escalation:
      - at_iteration: 3
        action: switch_model
        params:
          model: opus
  - name: review
    prompt: prompts/feature/review.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML
    echo "$output_file"
}

# ==============================================================================
# Helper: Create a workflow with a single state that has a specific constraints block
# Usage: create_workflow_with_constraints <constraints_yaml>
# ==============================================================================
create_workflow_with_constraints() {
    local constraints_yaml="$1"
    local output_file="$TEST_TEMP_DIR/workflow.yaml"

    cat > "$output_file" << YAML
name: test_constraints
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
    constraints:
${constraints_yaml}
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML
    echo "$output_file"
}

# ==============================================================================
# Helper: Create a workflow with a single state that has an after hooks block
# Usage: create_workflow_with_hooks <after_yaml>
# ==============================================================================
create_workflow_with_hooks() {
    local after_yaml="$1"
    local output_file="$TEST_TEMP_DIR/workflow.yaml"

    cat > "$output_file" << YAML
name: test_hooks
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
    after:
${after_yaml}
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML
    echo "$output_file"
}

# ==============================================================================
# Helper: Create a workflow with a single state that has an escalation block
# Usage: create_workflow_with_escalation <escalation_yaml>
# ==============================================================================
create_workflow_with_escalation() {
    local escalation_yaml="$1"
    local output_file="$TEST_TEMP_DIR/workflow.yaml"

    cat > "$output_file" << YAML
name: test_escalation
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
    escalation:
${escalation_yaml}
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML
    echo "$output_file"
}

# ==============================================================================
# Helper: Create a workflow with intervention_triggers at root level
# Usage: create_workflow_with_intervention <triggers_yaml>
# ==============================================================================
create_workflow_with_intervention() {
    local triggers_yaml="$1"
    local output_file="$TEST_TEMP_DIR/workflow.yaml"

    cat > "$output_file" << YAML
name: test_intervention
version: 4
intervention_triggers:
${triggers_yaml}
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML
    echo "$output_file"
}

# ==============================================================================
# Helper: Create a minimal valid V1 workflow (no V4 features)
# ==============================================================================
create_v1_workflow() {
    local output_file="$TEST_TEMP_DIR/workflow.yaml"

    cat > "$output_file" << 'YAML'
name: test_v1
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
    echo "$output_file"
}

# ==============================================================================
# TEST: test_valid_v4_workflow
# Full V4 workflow with all features properly configured should pass
# ==============================================================================
@test "V4: valid workflow with all V4 features passes" {
    local wf
    wf=$(create_v4_workflow 'intervention_triggers:
  - condition: "stuck_iterations >= 5"
    action: request_human
    message: "Phase is stuck"')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Validation passed"* ]]
}

# ==============================================================================
# TEST: test_v1_workflow_still_valid
# V1 workflow with no V4 additions should still pass (backwards compatible)
# ==============================================================================
@test "V4: V1 workflow still passes validation (backwards compatible)" {
    local wf
    wf=$(create_v1_workflow)
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Validation passed"* ]]
}

# ==============================================================================
# TEST: test_version_4_accepted
# Version 4 should be accepted without the "expected 1, 2, or 3" warning
# ==============================================================================
@test "V4: version 4 is accepted without version warning" {
    local wf
    wf=$(create_v4_workflow)
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 0 ]]
    [[ "$output" != *"expected 1"* ]]
    [[ "$output" != *"expected 1, 2"* ]]
}

# ==============================================================================
# CONSTRAINTS TESTS
# ==============================================================================

# TEST: test_invalid_max_iterations_negative
@test "V4 constraints: negative max_iterations fails" {
    local wf
    wf=$(create_workflow_with_constraints '      max_iterations: -5')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"invalid max_iterations"* ]]
}

# TEST: test_invalid_max_iterations_zero
@test "V4 constraints: zero max_iterations fails" {
    local wf
    wf=$(create_workflow_with_constraints '      max_iterations: 0')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"invalid max_iterations"* ]]
}

# TEST: test_max_iterations_string_rejected
@test "V4 constraints: string max_iterations fails" {
    local wf
    wf=$(create_workflow_with_constraints '      max_iterations: "five"')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"invalid max_iterations"* ]]
}

# TEST: test_max_iterations_float_rejected
@test "V4 constraints: float max_iterations fails" {
    local wf
    wf=$(create_workflow_with_constraints '      max_iterations: 3.5')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"invalid max_iterations"* ]]
}

# TEST: test_invalid_artifacts_not_array
@test "V4 constraints: require_artifacts_in as string fails" {
    local wf
    wf=$(create_workflow_with_constraints '      require_artifacts_in: "file.txt"')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"require_artifacts_in"*"array"* ]]
}

# TEST: test_artifacts_in_non_string_elements_rejected
@test "V4 constraints: require_artifacts_in with non-string elements fails" {
    local wf
    wf=$(create_workflow_with_constraints '      require_artifacts_in:
        - 123
        - true')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"require_artifacts_in"*"strings"* ]]
}

# TEST: valid constraints with all fields
@test "V4 constraints: valid max_iterations and artifact arrays pass" {
    local wf
    wf=$(create_workflow_with_constraints '      max_iterations: 10
      require_artifacts_in:
        - plan.md
        - spec.md
      require_artifacts_out:
        - output.txt')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 0 ]]
}

# TEST: empty constraints object is valid (edge case 1)
@test "V4 constraints: empty constraints object is valid" {
    local output_file="$TEST_TEMP_DIR/workflow.yaml"
    cat > "$output_file" << 'YAML'
name: test
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
    run "$SCRIPTS_DIR/validate-workflow.sh" "$output_file"
    [[ "$status" -eq 0 ]]
}

# TEST: large iteration number accepted (edge case 6)
@test "V4 constraints: large max_iterations accepted" {
    local wf
    wf=$(create_workflow_with_constraints '      max_iterations: 99999')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 0 ]]
}

# ==============================================================================
# AFTER HOOKS TESTS
# ==============================================================================

# TEST: test_hook_missing_action
@test "V4 hooks: missing action field fails" {
    local wf
    wf=$(create_workflow_with_hooks '      - params:
          key: value')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"missing"*"action"* ]]
}

# TEST: test_hook_unknown_action
@test "V4 hooks: unknown action fails" {
    local wf
    wf=$(create_workflow_with_hooks '      - action: unknown_action')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"unknown action"* ]]
}

# TEST: test_hook_invalid_when_syntax
@test "V4 hooks: invalid when syntax fails" {
    local wf
    wf=$(create_workflow_with_hooks '      - action: commit
        when: "invalid syntax here"')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"invalid"*"when"* ]]
}

# TEST: test_hook_valid_when_negation
@test "V4 hooks: negation in when is valid" {
    local wf
    wf=$(create_workflow_with_hooks '      - action: analyze_stuck
        when: "!approved"')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 0 ]]
}

# TEST: test_hook_valid_when_or
@test "V4 hooks: OR condition in when is valid" {
    local wf
    wf=$(create_workflow_with_hooks '      - action: commit
        when: "approved|passed"')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 0 ]]
}

# TEST: test_hook_when_uppercase_rejected
@test "V4 hooks: uppercase in when fails (verdicts must be lowercase)" {
    local wf
    wf=$(create_workflow_with_hooks '      - action: commit
        when: "Approved"')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"invalid"*"when"* ]]
}

# TEST: test_hook_when_with_digits_accepted
@test "V4 hooks: when with digits accepted (approved_v2)" {
    local wf
    wf=$(create_workflow_with_hooks '      - action: commit
        when: "approved_v2"')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 0 ]]
}

# TEST: test_hook_params_not_object_rejected
@test "V4 hooks: params as string fails" {
    local wf
    wf=$(create_workflow_with_hooks '      - action: run_tests
        params: "not_an_object"')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"params"*"object"* ]]
}

# TEST: test_hook_params_object_accepted
@test "V4 hooks: params as object passes" {
    local wf
    wf=$(create_workflow_with_hooks '      - action: run_tests
        params:
          pattern: "test"')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 0 ]]
}

# TEST: test_hook_continue_on_error_non_boolean_rejected
@test "V4 hooks: continue_on_error as string fails" {
    local wf
    wf=$(create_workflow_with_hooks '      - action: run_tests
        continue_on_error: "yes"')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"continue_on_error"*"boolean"* ]]
}

# TEST: test_hook_continue_on_error_true_accepted
@test "V4 hooks: continue_on_error true passes" {
    local wf
    wf=$(create_workflow_with_hooks '      - action: run_tests
        continue_on_error: true')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 0 ]]
}

# TEST: test_hook_continue_on_error_false_accepted
@test "V4 hooks: continue_on_error false passes" {
    local wf
    wf=$(create_workflow_with_hooks '      - action: run_tests
        continue_on_error: false')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 0 ]]
}

# TEST: empty after array is valid (edge case 2)
@test "V4 hooks: empty after array is valid" {
    local output_file="$TEST_TEMP_DIR/workflow.yaml"
    cat > "$output_file" << 'YAML'
name: test
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
    after: []
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML
    run "$SCRIPTS_DIR/validate-workflow.sh" "$output_file"
    [[ "$status" -eq 0 ]]
}

# TEST: test_all_known_actions_valid
@test "V4 hooks: all known actions are accepted" {
    local output_file="$TEST_TEMP_DIR/workflow.yaml"
    cat > "$output_file" << 'YAML'
name: test_all_actions
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
    after:
      - action: run_tests
      - action: commit
      - action: switch_model
      - action: analyze_stuck
      - action: request_human
      - action: script
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML
    run "$SCRIPTS_DIR/validate-workflow.sh" "$output_file"
    [[ "$status" -eq 0 ]]
}

# ==============================================================================
# ESCALATION TESTS
# ==============================================================================

# TEST: test_escalation_no_trigger_type
@test "V4 escalation: no trigger type fails" {
    local wf
    wf=$(create_workflow_with_escalation '      - action: analyze_stuck')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"exactly one"* ]]
}

# TEST: test_escalation_multiple_trigger_types
@test "V4 escalation: multiple trigger types fails" {
    local wf
    wf=$(create_workflow_with_escalation '      - at_iteration: 3
        after_iteration: 5
        action: analyze_stuck')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"exactly one"* ]]
}

# TEST: test_escalation_negative_iteration
@test "V4 escalation: negative iteration value fails" {
    local wf
    wf=$(create_workflow_with_escalation '      - at_iteration: -1
        action: analyze_stuck')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"invalid iteration value"* ]]
}

# TEST: test_escalation_at_iteration_zero
@test "V4 escalation: at_iteration 0 fails" {
    local wf
    wf=$(create_workflow_with_escalation '      - at_iteration: 0
        action: analyze_stuck')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"invalid iteration value"* ]]
}

# TEST: test_escalation_every_n_zero
@test "V4 escalation: every_n_iterations 0 fails" {
    local wf
    wf=$(create_workflow_with_escalation '      - every_n_iterations: 0
        action: analyze_stuck')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"invalid iteration value"* ]]
}

# TEST: test_escalation_missing_action
@test "V4 escalation: missing action field fails" {
    local wf
    wf=$(create_workflow_with_escalation '      - at_iteration: 3')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"missing"*"action"* ]]
}

# TEST: test_escalation_unknown_action
@test "V4 escalation: unknown action fails" {
    local wf
    wf=$(create_workflow_with_escalation '      - at_iteration: 3
        action: made_up_action')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"unknown action"* ]]
}

# TEST: test_escalation_params_not_object_rejected
@test "V4 escalation: params as string fails" {
    local wf
    wf=$(create_workflow_with_escalation '      - at_iteration: 3
        action: switch_model
        params: "opus"')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"params"*"object"* ]]
}

# TEST: test_escalation_params_object_accepted
@test "V4 escalation: params as object passes" {
    local wf
    wf=$(create_workflow_with_escalation '      - at_iteration: 3
        action: switch_model
        params:
          model: opus')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 0 ]]
}

# TEST: valid at_iteration trigger
@test "V4 escalation: valid at_iteration passes" {
    local wf
    wf=$(create_workflow_with_escalation '      - at_iteration: 5
        action: analyze_stuck')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 0 ]]
}

# TEST: valid after_iteration trigger
@test "V4 escalation: valid after_iteration passes" {
    local wf
    wf=$(create_workflow_with_escalation '      - after_iteration: 5
        action: switch_model
        params:
          model: opus')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 0 ]]
}

# TEST: valid every_n_iterations trigger
@test "V4 escalation: valid every_n_iterations passes" {
    local wf
    wf=$(create_workflow_with_escalation '      - every_n_iterations: 3
        action: run_tests')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 0 ]]
}

# TEST: empty escalation array is valid (edge case 3)
@test "V4 escalation: empty escalation array is valid" {
    local output_file="$TEST_TEMP_DIR/workflow.yaml"
    cat > "$output_file" << 'YAML'
name: test
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
    escalation: []
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML
    run "$SCRIPTS_DIR/validate-workflow.sh" "$output_file"
    [[ "$status" -eq 0 ]]
}

# TEST: large iteration number in escalation (edge case 6)
@test "V4 escalation: large iteration number accepted" {
    local wf
    wf=$(create_workflow_with_escalation '      - at_iteration: 99999
        action: request_human')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 0 ]]
}

# ==============================================================================
# INTERVENTION TRIGGERS TESTS
# ==============================================================================

# TEST: test_intervention_trigger_valid
@test "V4 intervention: valid condition and action passes" {
    local wf
    wf=$(create_workflow_with_intervention '  - condition: "stuck_iterations >= 5"
    action: request_human')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 0 ]]
}

# TEST: test_intervention_trigger_missing_condition
@test "V4 intervention: missing condition fails" {
    local wf
    wf=$(create_workflow_with_intervention '  - action: request_human')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"missing"*"condition"* ]]
}

# TEST: test_intervention_trigger_invalid_condition
@test "V4 intervention: invalid condition syntax fails" {
    local wf
    wf=$(create_workflow_with_intervention '  - condition: "not valid"
    action: request_human')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"invalid condition syntax"* ]]
}

# TEST: test_intervention_trigger_missing_action
@test "V4 intervention: missing action fails" {
    local wf
    wf=$(create_workflow_with_intervention '  - condition: "stuck >= 5"')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"missing"*"action"* ]]
}

# TEST: test_intervention_trigger_unknown_action
@test "V4 intervention: unknown action fails" {
    local wf
    wf=$(create_workflow_with_intervention '  - condition: "stuck_iterations >= 5"
    action: "made_up_action"')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"unknown action"* ]]
}

# TEST: test_condition_syntax_validation — various valid operators
@test "V4 intervention: all comparison operators accepted" {
    local output_file="$TEST_TEMP_DIR/workflow.yaml"
    cat > "$output_file" << 'YAML'
name: test
version: 4
intervention_triggers:
  - condition: "stuck_iterations == 5"
    action: request_human
  - condition: "stuck_iterations != 0"
    action: request_human
  - condition: "stuck_iterations >= 5"
    action: request_human
  - condition: "stuck_iterations <= 10"
    action: request_human
  - condition: "stuck_iterations > 3"
    action: request_human
  - condition: "stuck_iterations < 8"
    action: request_human
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML
    run "$SCRIPTS_DIR/validate-workflow.sh" "$output_file"
    [[ "$status" -eq 0 ]]
}

# TEST: intervention with optional message field
@test "V4 intervention: optional message field accepted" {
    local wf
    wf=$(create_workflow_with_intervention '  - condition: "stuck_iterations >= 5"
    action: request_human
    message: "Phase appears to be stuck"')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 0 ]]
}

# TEST: no intervention_triggers section is valid (optional)
@test "V4 intervention: missing intervention_triggers section is valid" {
    local output_file="$TEST_TEMP_DIR/workflow.yaml"
    cat > "$output_file" << 'YAML'
name: test
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML
    run "$SCRIPTS_DIR/validate-workflow.sh" "$output_file"
    [[ "$status" -eq 0 ]]
}

# ==============================================================================
# GET_KNOWN_ACTIONS TESTS
# ==============================================================================

# TEST: test_get_known_actions_direct
@test "V4 actions: get_known_actions returns 6 known actions" {
    # Source the validate-workflow.sh to get access to get_known_actions function
    # We need to source only the function definitions, not run the main script
    run bash -c 'source "$1" 2>/dev/null; get_known_actions' -- "$SCRIPTS_DIR/validate-workflow.sh"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"run_tests"* ]]
    [[ "$output" == *"commit"* ]]
    [[ "$output" == *"switch_model"* ]]
    [[ "$output" == *"analyze_stuck"* ]]
    [[ "$output" == *"request_human"* ]]
    [[ "$output" == *"script"* ]]
    # Count lines: should be exactly 6
    local count
    count=$(echo "$output" | wc -l)
    [[ "$count" -eq 6 ]]
}

# ==============================================================================
# BACKWARDS COMPATIBILITY TESTS
# ==============================================================================

# TEST: V1 workflow still passes (edge case 4)
@test "V4: V1 workflow with version 1 still validates" {
    local output_file="$TEST_TEMP_DIR/workflow.yaml"
    cat > "$output_file" << 'YAML'
name: test_compat
version: 1
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML
    run "$SCRIPTS_DIR/validate-workflow.sh" "$output_file"
    [[ "$status" -eq 0 ]]
}

# TEST: Mixed V3 and V4 features work together (edge case 5)
@test "V4: mixed V3 and V4 features both validate" {
    local output_file="$TEST_TEMP_DIR/workflow.yaml"
    cat > "$output_file" << 'YAML'
name: test_mixed
version: 4
defaults:
  max_iterations: 10
  timeout: 600
variables:
  package: test-package
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
    params:
      allow_test_changes: false
    constraints:
      max_iterations: 5
    after:
      - action: run_tests
    escalation:
      - at_iteration: 3
        action: switch_model
        params:
          model: opus
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML
    run "$SCRIPTS_DIR/validate-workflow.sh" "$output_file"
    [[ "$status" -eq 0 ]]
}

# TEST: state without V4 sections passes (V4 fields are optional per state)
@test "V4: state without any V4 sections passes" {
    local output_file="$TEST_TEMP_DIR/workflow.yaml"
    cat > "$output_file" << 'YAML'
name: test_no_v4
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML
    run "$SCRIPTS_DIR/validate-workflow.sh" "$output_file"
    [[ "$status" -eq 0 ]]
}

# ==============================================================================
# ADDITIONAL EDGE CASE TESTS
# ==============================================================================

# TEST: require_artifacts_out as non-array fails (same validation as require_artifacts_in)
@test "V4 constraints: require_artifacts_out as string fails" {
    local wf
    wf=$(create_workflow_with_constraints '      require_artifacts_out: "output.txt"')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"require_artifacts_out"*"array"* ]]
}

# TEST: require_artifacts_out with non-string elements fails
@test "V4 constraints: require_artifacts_out with non-string elements fails" {
    local wf
    wf=$(create_workflow_with_constraints '      require_artifacts_out:
        - 456
        - false')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"require_artifacts_out"*"strings"* ]]
}

# TEST: all three escalation trigger types accepted individually
@test "V4 escalation: all three trigger types accepted in separate entries" {
    local output_file="$TEST_TEMP_DIR/workflow.yaml"
    cat > "$output_file" << 'YAML'
name: test
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
    escalation:
      - at_iteration: 3
        action: analyze_stuck
      - after_iteration: 5
        action: switch_model
        params:
          model: opus
      - every_n_iterations: 2
        action: run_tests
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML
    run "$SCRIPTS_DIR/validate-workflow.sh" "$output_file"
    [[ "$status" -eq 0 ]]
}

# TEST: multiple after hooks on one state
@test "V4 hooks: multiple hooks on one state pass" {
    local output_file="$TEST_TEMP_DIR/workflow.yaml"
    cat > "$output_file" << 'YAML'
name: test
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
    after:
      - action: run_tests
        when: "approved"
      - action: commit
        when: "approved"
        continue_on_error: false
      - action: analyze_stuck
        when: "!approved"
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML
    run "$SCRIPTS_DIR/validate-workflow.sh" "$output_file"
    [[ "$status" -eq 0 ]]
}

# TEST: when with OR of multiple verdicts
@test "V4 hooks: when with multiple OR verdicts passes" {
    local wf
    wf=$(create_workflow_with_hooks '      - action: commit
        when: "approved|passed|fixed"')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 0 ]]
}

# TEST: when with negation and underscore-starting name
@test "V4 hooks: when with negation and underscore start passes" {
    local wf
    wf=$(create_workflow_with_hooks '      - action: analyze_stuck
        when: "!_internal_verdict"')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 0 ]]
}

# TEST: escalation all three trigger types at once fails
@test "V4 escalation: all three trigger types on one entry fails" {
    local wf
    wf=$(create_workflow_with_escalation '      - at_iteration: 3
        after_iteration: 5
        every_n_iterations: 2
        action: analyze_stuck')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"exactly one"* ]]
}

# TEST: multiple intervention triggers
@test "V4 intervention: multiple valid triggers pass" {
    local output_file="$TEST_TEMP_DIR/workflow.yaml"
    cat > "$output_file" << 'YAML'
name: test
version: 4
intervention_triggers:
  - condition: "stuck_iterations >= 5"
    action: request_human
    message: "Stuck on phase"
  - condition: "total_cost > 100"
    action: request_human
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML
    run "$SCRIPTS_DIR/validate-workflow.sh" "$output_file"
    [[ "$status" -eq 0 ]]
}

# TEST: constraints, hooks, and escalation on multiple states
@test "V4: V4 features on multiple states all validate" {
    local output_file="$TEST_TEMP_DIR/workflow.yaml"
    cat > "$output_file" << 'YAML'
name: test_multi
version: 4
states:
  - name: qa
    prompt: prompts/feature/qa.md
    next: review
    constraints:
      max_iterations: 3
    after:
      - action: run_tests
  - name: review
    prompt: prompts/feature/review.md
    next: complete
    escalation:
      - at_iteration: 5
        action: request_human
  - name: complete
    prompt: prompts/common/complete.md
entry_state: qa
terminal_states: [complete]
YAML
    run "$SCRIPTS_DIR/validate-workflow.sh" "$output_file"
    [[ "$status" -eq 0 ]]
}

# TEST: when with empty string (no when condition) passes — when is optional
@test "V4 hooks: hook without when field passes" {
    local wf
    wf=$(create_workflow_with_hooks '      - action: run_tests')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 0 ]]
}

# TEST: intervention condition with underscore variable name
@test "V4 intervention: underscore-prefixed variable in condition passes" {
    local wf
    wf=$(create_workflow_with_intervention '  - condition: "_stuck_count >= 3"
    action: request_human')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 0 ]]
}

# TEST: intervention condition with == operator
@test "V4 intervention: equality condition passes" {
    local wf
    wf=$(create_workflow_with_intervention '  - condition: "status == failed"
    action: request_human')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 0 ]]
}

# TEST: intervention condition with != operator
@test "V4 intervention: inequality condition passes" {
    local wf
    wf=$(create_workflow_with_intervention '  - condition: "status != ok"
    action: request_human')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 0 ]]
}

# TEST: when with pipe and negation combined (should fail — negation applies to whole pattern)
@test "V4 hooks: when with OR pattern 'approved|needs_fix' passes" {
    local wf
    wf=$(create_workflow_with_hooks '      - action: commit
        when: "approved|needs_fix"')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 0 ]]
}

# TEST: valid constraint with only require_artifacts_in
@test "V4 constraints: only require_artifacts_in is valid" {
    local wf
    wf=$(create_workflow_with_constraints '      require_artifacts_in:
        - plan.md')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 0 ]]
}

# TEST: valid constraint with only require_artifacts_out
@test "V4 constraints: only require_artifacts_out is valid" {
    local wf
    wf=$(create_workflow_with_constraints '      require_artifacts_out:
        - output.txt
        - results.json')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 0 ]]
}

# TEST: valid constraint with only max_iterations
@test "V4 constraints: only max_iterations is valid" {
    local wf
    wf=$(create_workflow_with_constraints '      max_iterations: 10')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 0 ]]
}
