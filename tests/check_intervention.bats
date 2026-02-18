#!/usr/bin/env bats

# Tests for check-intervention.sh
# Phase: intervention-triggers (orchestration-v4)
#
# Tests the intervention trigger evaluation functions that check global
# conditions defined at the workflow level and request human intervention
# when triggered.
#
# Depends on: actions.sh (action-registry phase)

setup() {
    load 'test_helper'
    setup_temp_dir

    CHECK_INTERVENTION_SH="$SCRIPTS_DIR/check-intervention.sh"
    ACTIONS_SH="$SCRIPTS_DIR/actions.sh"

    # Standard phase directory structure
    export PHASE_DIR="$TEST_TEMP_DIR/phase"
    mkdir -p "$PHASE_DIR"

    # Create minimal state.json
    export STATE_FILE="$PHASE_DIR/state.json"
    cat > "$STATE_FILE" << 'JSON'
{
    "iteration": 0,
    "current_state": "impl",
    "tests_total": 0,
    "tests_passing": 0
}
JSON

    # Default environment
    export ARC_DEFAULT_PKG="test-package"
    export VERDICT=""
    export ARC_HOME="$TEST_TEMP_DIR/orchestration"
    mkdir -p "$ARC_HOME/scripts"

    # Track which action functions were called
    export CALL_LOG="$TEST_TEMP_DIR/call_log.txt"
    : > "$CALL_LOG"
}

teardown() {
    teardown_temp_dir
}

# ==============================================================================
# Helper: Create a workflow YAML with intervention_triggers at root level
# Usage: create_intervention_workflow <triggers_yaml>
# The triggers_yaml should be the indented content under `intervention_triggers:`
# ==============================================================================
create_intervention_workflow() {
    local triggers_yaml="$1"
    local output_file="$TEST_TEMP_DIR/workflow.yaml"

    cat > "$output_file" << YAML
name: test_intervention
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
intervention_triggers:
${triggers_yaml}
YAML
    echo "$output_file"
}

# ==============================================================================
# Helper: Create a workflow YAML with no intervention_triggers section
# ==============================================================================
create_no_intervention_workflow() {
    local output_file="$TEST_TEMP_DIR/workflow.yaml"

    cat > "$output_file" << 'YAML'
name: test_no_intervention
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
    echo "$output_file"
}

# ==============================================================================
# Helper: Source check-intervention.sh (and actions.sh) in a subshell with mock
# actions. Overrides action functions with mocks that log calls and return
# configurable exit codes.
#
# Usage: run_intervention_with_mocks <function_call> [mock_overrides]
# ==============================================================================
run_intervention_with_mocks() {
    local func_call="$1"
    local mock_overrides="${2:-}"

    run bash -c "
        export PHASE_DIR=\"$PHASE_DIR\"
        export STATE_FILE=\"$STATE_FILE\"
        export ARC_DEFAULT_PKG=\"${ARC_DEFAULT_PKG:-test-package}\"
        export VERDICT=\"${VERDICT:-}\"
        export ARC_HOME=\"${ARC_HOME:-}\"
        export CALL_LOG=\"$CALL_LOG\"

        source \"$ACTIONS_SH\"
        source \"$CHECK_INTERVENTION_SH\"

        # Override action functions with mocks that log calls
        action_run_tests() {
            echo \"action_run_tests \$*\" >> \"\$CALL_LOG\"
            return 0
        }
        action_commit() {
            echo \"action_commit \$*\" >> \"\$CALL_LOG\"
            return 0
        }
        action_switch_model() {
            echo \"action_switch_model \$*\" >> \"\$CALL_LOG\"
            return 0
        }
        action_analyze_stuck() {
            echo \"action_analyze_stuck \$*\" >> \"\$CALL_LOG\"
            return 0
        }
        action_request_human() {
            echo \"action_request_human \$*\" >> \"\$CALL_LOG\"
            return 0
        }
        action_script() {
            echo \"action_script \$*\" >> \"\$CALL_LOG\"
            return 0
        }

        # Apply any test-specific mock overrides
        $mock_overrides

        $func_call
    "
}

# ==============================================================================
# Helper: Set a field in state.json
# Usage: set_state_field <field> <value>
# ==============================================================================
set_state_field() {
    local field="$1"
    local value="$2"
    jq --argjson val "$value" ".$field = \$val" "$STATE_FILE" > "${STATE_FILE}.tmp" \
        && mv "${STATE_FILE}.tmp" "$STATE_FILE"
}

# ==============================================================================
# Helper: Set a string field in state.json
# Usage: set_state_string_field <field> <value>
# ==============================================================================
set_state_string_field() {
    local field="$1"
    local value="$2"
    jq --arg val "$value" ".$field = \$val" "$STATE_FILE" > "${STATE_FILE}.tmp" \
        && mv "${STATE_FILE}.tmp" "$STATE_FILE"
}

#=============================================================================
# Script Existence and Syntax Tests
#=============================================================================

@test "check-intervention.sh exists and is readable" {
    [[ -f "$CHECK_INTERVENTION_SH" ]]
}

@test "check-intervention.sh is syntactically valid bash" {
    run bash -n "$CHECK_INTERVENTION_SH"
    [[ "$status" -eq 0 ]]
}

@test "check-intervention.sh can be sourced without error" {
    run bash -c "source '$ACTIONS_SH'; source '$CHECK_INTERVENTION_SH'"
    [[ "$status" -eq 0 ]]
}

@test "check-intervention.sh does not use set -e" {
    [[ -f "$CHECK_INTERVENTION_SH" ]]
    # Spec requires set -uo pipefail but NOT -e
    run bash -c "grep -P '^set\\s+-(\\w*e\\w*)' '$CHECK_INTERVENTION_SH' | grep -v pipefail"
    [[ "$status" -eq 1 ]]  # No matches found = good
}

@test "check-intervention.sh uses set -uo pipefail" {
    run bash -c "grep -E '^set -uo pipefail' '$CHECK_INTERVENTION_SH'"
    [[ "$status" -eq 0 ]]
}

@test "all intervention functions defined after sourcing" {
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_INTERVENTION_SH'
        declare -f check_intervention_triggers > /dev/null && echo 'check_intervention_triggers:ok'
        declare -f get_intervention_triggers > /dev/null && echo 'get_intervention_triggers:ok'
        declare -f evaluate_condition > /dev/null && echo 'evaluate_condition:ok'
        declare -f parse_condition > /dev/null && echo 'parse_condition:ok'
        declare -f get_state_value > /dev/null && echo 'get_state_value:ok'
    "
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"check_intervention_triggers:ok"* ]]
    [[ "$output" == *"get_intervention_triggers:ok"* ]]
    [[ "$output" == *"evaluate_condition:ok"* ]]
    [[ "$output" == *"parse_condition:ok"* ]]
    [[ "$output" == *"get_state_value:ok"* ]]
}

#=============================================================================
# check_intervention_triggers Tests
#=============================================================================

@test "test_no_intervention_triggers: exit 0 when no intervention_triggers section" {
    local wf
    wf=$(create_no_intervention_workflow)

    run_intervention_with_mocks \
        "check_intervention_triggers '$wf'"
    [[ "$status" -eq 0 ]]

    # No actions should have been called
    run cat "$CALL_LOG"
    [[ -z "$output" ]]
}

@test "test_condition_not_met: exit 0 when condition not satisfied" {
    local wf
    wf=$(create_intervention_workflow '  - condition: "stuck_iterations >= 5"
    action: request_human
    message: "help"')
    set_state_field "stuck_iterations" 2

    run_intervention_with_mocks \
        "check_intervention_triggers '$wf'"
    [[ "$status" -eq 0 ]]

    # No action should have been called
    run cat "$CALL_LOG"
    [[ -z "$output" ]]
}

@test "test_condition_met: exit 2 when condition satisfied, action_request_human called" {
    local wf
    wf=$(create_intervention_workflow '  - condition: "stuck_iterations >= 5"
    action: request_human
    message: "help needed"')
    set_state_field "stuck_iterations" 6

    run_intervention_with_mocks \
        "check_intervention_triggers '$wf'"
    [[ "$status" -eq 2 ]]

    # action_request_human should have been called with the message
    run cat "$CALL_LOG"
    [[ "$output" == *"action_request_human help needed"* ]]
}

@test "test_already_requested: exit 2 without calling action when intervention_request exists" {
    local wf
    wf=$(create_intervention_workflow '  - condition: "stuck_iterations >= 5"
    action: request_human
    message: "help"')
    set_state_field "stuck_iterations" 10

    # Set intervention_request as object (per STATE_SCHEMA.md)
    jq '.intervention_request = {"reason": "previous request", "requested_at": "2024-01-01T00:00:00Z", "options": ["resolve", "abort"]}' \
        "$STATE_FILE" > "${STATE_FILE}.tmp" && mv "${STATE_FILE}.tmp" "$STATE_FILE"

    run_intervention_with_mocks \
        "check_intervention_triggers '$wf'"
    [[ "$status" -eq 2 ]]

    # action_request_human should NOT have been called (already requested)
    run cat "$CALL_LOG"
    [[ -z "$output" ]]
}

@test "test_equality_condition: exit 2 when tests_passing == 0" {
    local wf
    wf=$(create_intervention_workflow '  - condition: "tests_passing == 0"
    action: request_human
    message: "no tests"')
    # tests_passing is already 0 in default state

    run_intervention_with_mocks \
        "check_intervention_triggers '$wf'"
    [[ "$status" -eq 2 ]]

    run cat "$CALL_LOG"
    [[ "$output" == *"action_request_human"* ]]
}

@test "test_inequality_condition: exit 2 when tests_passing != tests_total" {
    local wf
    wf=$(create_intervention_workflow '  - condition: "tests_passing != tests_total"
    action: request_human
    message: "some failing"')
    set_state_field "tests_passing" 5
    set_state_field "tests_total" 10

    run_intervention_with_mocks \
        "check_intervention_triggers '$wf'"
    [[ "$status" -eq 2 ]]

    run cat "$CALL_LOG"
    [[ "$output" == *"action_request_human some failing"* ]]
}

@test "test_greater_than_condition: exit 2 when iteration > 10" {
    local wf
    wf=$(create_intervention_workflow '  - condition: "iteration > 10"
    action: request_human
    message: "too many"')
    set_state_field "iteration" 12

    run_intervention_with_mocks \
        "check_intervention_triggers '$wf'"
    [[ "$status" -eq 2 ]]

    run cat "$CALL_LOG"
    [[ "$output" == *"action_request_human too many"* ]]
}

@test "test_less_than_condition: exit 2 when tests_passing < 3" {
    local wf
    wf=$(create_intervention_workflow '  - condition: "tests_passing < 3"
    action: request_human
    message: "too few"')
    set_state_field "tests_passing" 1

    run_intervention_with_mocks \
        "check_intervention_triggers '$wf'"
    [[ "$status" -eq 2 ]]

    run cat "$CALL_LOG"
    [[ "$output" == *"action_request_human too few"* ]]
}

@test "test_multiple_triggers_first_match: only first matching trigger message used" {
    local wf
    wf=$(create_intervention_workflow '  - condition: "stuck_iterations >= 3"
    action: request_human
    message: "first trigger"
  - condition: "iteration >= 1"
    action: request_human
    message: "second trigger"')
    set_state_field "stuck_iterations" 5
    set_state_field "iteration" 10

    run_intervention_with_mocks \
        "check_intervention_triggers '$wf'"
    [[ "$status" -eq 2 ]]

    # Only first matching trigger's message should be used
    run cat "$CALL_LOG"
    [[ "$output" == *"action_request_human first trigger"* ]]
    [[ "$output" != *"second trigger"* ]]
}

@test "test_default_message: uses default when trigger has no message field" {
    local wf
    wf=$(create_intervention_workflow '  - condition: "stuck_iterations >= 5"
    action: request_human')
    set_state_field "stuck_iterations" 10

    run_intervention_with_mocks \
        "check_intervention_triggers '$wf'"
    [[ "$status" -eq 2 ]]

    # Should use default message
    run cat "$CALL_LOG"
    [[ "$output" == *"action_request_human Human intervention required"* ]]
}

@test "test_less_than_or_equal_condition: exit 2 when tests_passing <= 2 (boundary)" {
    local wf
    wf=$(create_intervention_workflow '  - condition: "tests_passing <= 2"
    action: request_human
    message: "few passing"')
    set_state_field "tests_passing" 2

    run_intervention_with_mocks \
        "check_intervention_triggers '$wf'"
    [[ "$status" -eq 2 ]]

    run cat "$CALL_LOG"
    [[ "$output" == *"action_request_human few passing"* ]]
}

@test "test_boolean_comparison: exit 2 when stuck == true" {
    local wf
    wf=$(create_intervention_workflow '  - condition: "stuck == true"
    action: request_human
    message: "stuck"')
    set_state_field "stuck" true

    run_intervention_with_mocks \
        "check_intervention_triggers '$wf'"
    [[ "$status" -eq 2 ]]

    run cat "$CALL_LOG"
    [[ "$output" == *"action_request_human stuck"* ]]
}

@test "test_intervention_request_empty_object: exit 2 without action when empty object" {
    local wf
    wf=$(create_intervention_workflow '  - condition: "stuck_iterations >= 5"
    action: request_human
    message: "help"')
    set_state_field "stuck_iterations" 10

    # Set intervention_request to empty object (still not null)
    jq '.intervention_request = {}' "$STATE_FILE" > "${STATE_FILE}.tmp" \
        && mv "${STATE_FILE}.tmp" "$STATE_FILE"

    run_intervention_with_mocks \
        "check_intervention_triggers '$wf'"
    [[ "$status" -eq 2 ]]

    # action_request_human should NOT be called (empty object != null)
    run cat "$CALL_LOG"
    [[ -z "$output" ]]
}

@test "test_missing_state_file: error when STATE_FILE points to nonexistent file" {
    local wf
    wf=$(create_intervention_workflow '  - condition: "stuck_iterations >= 5"
    action: request_human
    message: "help"')

    # Point STATE_FILE to nonexistent path
    export STATE_FILE="$TEST_TEMP_DIR/nonexistent/state.json"

    run_intervention_with_mocks \
        "check_intervention_triggers '$wf'"
    [[ "$status" -ne 0 ]]
}

@test "test_intervention_stderr_message: INTERVENTION message printed to stderr" {
    local wf
    wf=$(create_intervention_workflow '  - condition: "stuck_iterations >= 5"
    action: request_human
    message: "help needed"')
    set_state_field "stuck_iterations" 6

    run_intervention_with_mocks \
        "check_intervention_triggers '$wf'"
    [[ "$status" -eq 2 ]]

    # stderr captured in output by bats
    [[ "$output" == *"INTERVENTION: help needed"* ]]
}

#=============================================================================
# parse_condition Tests
#=============================================================================

@test "test_parse_condition_valid: parses stuck_iterations >= 5" {
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_INTERVENTION_SH'
        parse_condition 'stuck_iterations >= 5'
    "
    [[ "$status" -eq 0 ]]
    [[ "$output" == "stuck_iterations >= 5" ]]
}

@test "test_parse_condition_invalid: exit 1 for invalid format" {
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_INTERVENTION_SH'
        parse_condition 'invalid condition format'
    "
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Cannot parse condition"* ]]
}

@test "test_parse_condition_operator_only: exit 1 for just operator" {
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_INTERVENTION_SH'
        parse_condition '>='
    "
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Cannot parse condition"* ]] || [[ "$output" == *"ERROR"* ]]
}

@test "test_parse_condition_empty_operand: exit 1 for missing left operand" {
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_INTERVENTION_SH'
        parse_condition ' >= 5'
    "
    [[ "$status" -eq 1 ]]
}

@test "test_parse_condition_variable_with_digits: handles tests_v2 >= 5" {
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_INTERVENTION_SH'
        parse_condition 'tests_v2 >= 5'
    "
    [[ "$status" -eq 0 ]]
    [[ "$output" == "tests_v2 >= 5" ]]
}

@test "test_parse_condition_equality: parses status == active" {
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_INTERVENTION_SH'
        parse_condition 'status == active'
    "
    [[ "$status" -eq 0 ]]
    [[ "$output" == "status == active" ]]
}

@test "test_parse_condition_inequality: parses tests_passing != tests_total" {
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_INTERVENTION_SH'
        parse_condition 'tests_passing != tests_total'
    "
    [[ "$status" -eq 0 ]]
    [[ "$output" == "tests_passing != tests_total" ]]
}

@test "test_parse_condition_less_than: parses iteration < 10" {
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_INTERVENTION_SH'
        parse_condition 'iteration < 10'
    "
    [[ "$status" -eq 0 ]]
    [[ "$output" == "iteration < 10" ]]
}

@test "test_parse_condition_greater_than: parses iteration > 5" {
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_INTERVENTION_SH'
        parse_condition 'iteration > 5'
    "
    [[ "$status" -eq 0 ]]
    [[ "$output" == "iteration > 5" ]]
}

#=============================================================================
# get_state_value Tests
#=============================================================================

@test "test_get_state_value_exists: returns value when field present" {
    local state_json='{"iteration": 7, "tests_passing": 5}'
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_INTERVENTION_SH'
        get_state_value '$state_json' 'iteration'
    "
    [[ "$status" -eq 0 ]]
    [[ "$output" == "7" ]]
}

@test "test_get_state_value_missing: returns empty string when field absent" {
    local state_json='{"iteration": 7}'
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_INTERVENTION_SH'
        get_state_value '$state_json' 'missing_field'
    "
    [[ "$status" -eq 0 ]]
    [[ -z "$output" ]]
}

@test "test_get_state_value_not_found_returns_empty: max_iterations not in state" {
    local state_json='{"iteration": 7}'
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_INTERVENTION_SH'
        get_state_value '$state_json' 'max_iterations'
    "
    [[ "$status" -eq 0 ]]
    [[ -z "$output" ]]
}

@test "test_get_state_value_boolean: returns true for boolean field" {
    local state_json='{"stuck": true}'
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_INTERVENTION_SH'
        get_state_value '$state_json' 'stuck'
    "
    [[ "$status" -eq 0 ]]
    [[ "$output" == "true" ]]
}

@test "test_get_state_value_string: returns string value" {
    local state_json='{"status": "active"}'
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_INTERVENTION_SH'
        get_state_value '$state_json' 'status'
    "
    [[ "$status" -eq 0 ]]
    [[ "$output" == "active" ]]
}

#=============================================================================
# evaluate_condition Tests
#=============================================================================

@test "test_evaluate_condition_gte_true: stuck_iterations >= 5 with value 6" {
    local state_json='{"stuck_iterations": 6}'
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_INTERVENTION_SH'
        evaluate_condition 'stuck_iterations >= 5' '$state_json'
    "
    [[ "$status" -eq 0 ]]
}

@test "test_evaluate_condition_gte_false: stuck_iterations >= 5 with value 2" {
    local state_json='{"stuck_iterations": 2}'
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_INTERVENTION_SH'
        evaluate_condition 'stuck_iterations >= 5' '$state_json'
    "
    [[ "$status" -eq 1 ]]
}

@test "test_evaluate_condition_eq_true: tests_passing == 0 with value 0" {
    local state_json='{"tests_passing": 0}'
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_INTERVENTION_SH'
        evaluate_condition 'tests_passing == 0' '$state_json'
    "
    [[ "$status" -eq 0 ]]
}

@test "test_evaluate_condition_eq_false: tests_passing == 0 with value 5" {
    local state_json='{"tests_passing": 5}'
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_INTERVENTION_SH'
        evaluate_condition 'tests_passing == 0' '$state_json'
    "
    [[ "$status" -eq 1 ]]
}

@test "test_evaluate_condition_neq_true: tests_passing != tests_total" {
    local state_json='{"tests_passing": 5, "tests_total": 10}'
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_INTERVENTION_SH'
        evaluate_condition 'tests_passing != tests_total' '$state_json'
    "
    [[ "$status" -eq 0 ]]
}

@test "test_evaluate_condition_neq_false: tests_passing != tests_total when equal" {
    local state_json='{"tests_passing": 10, "tests_total": 10}'
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_INTERVENTION_SH'
        evaluate_condition 'tests_passing != tests_total' '$state_json'
    "
    [[ "$status" -eq 1 ]]
}

@test "test_evaluate_condition_gt_true: iteration > 10 with value 12" {
    local state_json='{"iteration": 12}'
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_INTERVENTION_SH'
        evaluate_condition 'iteration > 10' '$state_json'
    "
    [[ "$status" -eq 0 ]]
}

@test "test_evaluate_condition_gt_false: iteration > 10 with value 10" {
    local state_json='{"iteration": 10}'
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_INTERVENTION_SH'
        evaluate_condition 'iteration > 10' '$state_json'
    "
    [[ "$status" -eq 1 ]]
}

@test "test_evaluate_condition_lt_true: tests_passing < 3 with value 1" {
    local state_json='{"tests_passing": 1}'
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_INTERVENTION_SH'
        evaluate_condition 'tests_passing < 3' '$state_json'
    "
    [[ "$status" -eq 0 ]]
}

@test "test_evaluate_condition_lt_false: tests_passing < 3 with value 5" {
    local state_json='{"tests_passing": 5}'
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_INTERVENTION_SH'
        evaluate_condition 'tests_passing < 3' '$state_json'
    "
    [[ "$status" -eq 1 ]]
}

@test "test_evaluate_condition_lte_true: tests_passing <= 2 with value 2" {
    local state_json='{"tests_passing": 2}'
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_INTERVENTION_SH'
        evaluate_condition 'tests_passing <= 2' '$state_json'
    "
    [[ "$status" -eq 0 ]]
}

@test "test_evaluate_condition_lte_false: tests_passing <= 2 with value 3" {
    local state_json='{"tests_passing": 3}'
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_INTERVENTION_SH'
        evaluate_condition 'tests_passing <= 2' '$state_json'
    "
    [[ "$status" -eq 1 ]]
}

@test "test_evaluate_condition_with_spaces: handles extra whitespace" {
    local state_json='{"stuck_iterations": 6}'
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_INTERVENTION_SH'
        evaluate_condition 'stuck_iterations   >=   5' '$state_json'
    "
    [[ "$status" -eq 0 ]]
}

@test "test_evaluate_condition_non_numeric_with_gt: error for non-integer left operand" {
    local state_json='{"status": "active"}'
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_INTERVENTION_SH'
        evaluate_condition 'status > 5' '$state_json'
    "
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Non-integer left operand"* ]]
    [[ "$output" == *"active"* ]]
}

@test "test_evaluate_condition_negative_numbers: 0 > -1 is true" {
    local state_json='{"balance": 0}'
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_INTERVENTION_SH'
        evaluate_condition 'balance > -1' '$state_json'
    "
    [[ "$status" -eq 0 ]]
}

@test "test_evaluate_condition_boolean_eq: stuck == true" {
    local state_json='{"stuck": true}'
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_INTERVENTION_SH'
        evaluate_condition 'stuck == true' '$state_json'
    "
    [[ "$status" -eq 0 ]]
}

@test "test_evaluate_condition_boolean_false: stuck == false when value is true" {
    local state_json='{"stuck": true}'
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_INTERVENTION_SH'
        evaluate_condition 'stuck == false' '$state_json'
    "
    [[ "$status" -eq 1 ]]
}

@test "test_evaluate_condition_both_operands_missing: defaults to 0 >= 0 (true)" {
    local state_json='{"iteration": 7}'
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_INTERVENTION_SH'
        evaluate_condition 'nonexistent_field >= another_missing_field' '$state_json'
    "
    [[ "$status" -eq 0 ]]
}

@test "test_evaluate_condition_unknown_operator: error for unsupported operator" {
    local state_json='{"iteration": 7}'
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_INTERVENTION_SH'
        evaluate_condition 'iteration ~= 5' '$state_json'
    "
    [[ "$status" -eq 1 ]]
}

@test "test_evaluate_condition_variable_vs_variable: right side variable reference" {
    local state_json='{"tests_passing": 5, "tests_total": 10}'
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_INTERVENTION_SH'
        evaluate_condition 'tests_passing < tests_total' '$state_json'
    "
    [[ "$status" -eq 0 ]]
}

#=============================================================================
# get_intervention_triggers Tests
#=============================================================================

@test "test_get_intervention_triggers_direct: returns JSON array for valid workflow" {
    local wf
    wf=$(create_intervention_workflow '  - condition: "stuck_iterations >= 5"
    action: request_human
    message: "help"')

    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_INTERVENTION_SH'
        get_intervention_triggers '$wf'
    "
    [[ "$status" -eq 0 ]]
    # Should output a JSON array with one object
    local count
    count=$(echo "$output" | jq 'length')
    [[ "$count" -eq 1 ]]
    # Verify condition field
    local condition
    condition=$(echo "$output" | jq -r '.[0].condition')
    [[ "$condition" == "stuck_iterations >= 5" ]]
}

@test "test_get_intervention_triggers_no_section: returns empty array" {
    local wf
    wf=$(create_no_intervention_workflow)

    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_INTERVENTION_SH'
        get_intervention_triggers '$wf'
    "
    [[ "$status" -eq 0 ]]
    [[ "$(echo "$output" | jq 'length')" -eq 0 ]]
}

@test "test_get_intervention_triggers_multiple: returns all triggers" {
    local wf
    wf=$(create_intervention_workflow '  - condition: "stuck_iterations >= 5"
    action: request_human
    message: "stuck"
  - condition: "iteration >= 15"
    action: request_human
    message: "max iterations"
  - condition: "tests_passing == 0"
    action: request_human
    message: "all failing"')

    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_INTERVENTION_SH'
        get_intervention_triggers '$wf'
    "
    [[ "$status" -eq 0 ]]
    local count
    count=$(echo "$output" | jq 'length')
    [[ "$count" -eq 3 ]]
}

#=============================================================================
# Edge Case Tests
#=============================================================================

@test "test_empty_intervention_triggers_array: exit 0 with explicit empty array" {
    local wf="$TEST_TEMP_DIR/workflow.yaml"
    cat > "$wf" << 'YAML'
name: test_empty_intervention
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
intervention_triggers: []
YAML

    run_intervention_with_mocks \
        "check_intervention_triggers '$wf'"
    [[ "$status" -eq 0 ]]

    run cat "$CALL_LOG"
    [[ -z "$output" ]]
}

@test "test_condition_met_but_not_first: only first matching condition triggers" {
    local wf
    wf=$(create_intervention_workflow '  - condition: "stuck_iterations >= 10"
    action: request_human
    message: "first"
  - condition: "iteration >= 5"
    action: request_human
    message: "second"')
    set_state_field "stuck_iterations" 3
    set_state_field "iteration" 7

    run_intervention_with_mocks \
        "check_intervention_triggers '$wf'"
    [[ "$status" -eq 2 ]]

    # First condition not met (3 < 10), second is met (7 >= 5)
    run cat "$CALL_LOG"
    [[ "$output" == *"action_request_human second"* ]]
    [[ "$output" != *"first"* ]]

    # Only one call made
    local call_count
    call_count=$(wc -l < "$CALL_LOG")
    [[ "$call_count" -eq 1 ]]
}

@test "test_condition_missing_variable_defaults_to_zero: missing var in numeric comparison" {
    local wf
    wf=$(create_intervention_workflow '  - condition: "nonexistent >= 1"
    action: request_human
    message: "should not trigger"')

    run_intervention_with_mocks \
        "check_intervention_triggers '$wf'"
    # nonexistent defaults to 0, 0 >= 1 is false
    [[ "$status" -eq 0 ]]

    run cat "$CALL_LOG"
    [[ -z "$output" ]]
}

@test "test_condition_missing_variable_defaults_eq_zero: missing var == 0" {
    local wf
    wf=$(create_intervention_workflow '  - condition: "nonexistent == 0"
    action: request_human
    message: "triggers"')

    run_intervention_with_mocks \
        "check_intervention_triggers '$wf'"
    # nonexistent defaults to 0, "0" == "0" is true
    [[ "$status" -eq 2 ]]

    run cat "$CALL_LOG"
    [[ "$output" == *"action_request_human triggers"* ]]
}

@test "test_no_re_trigger_after_intervention: second call returns 2 without action" {
    local wf
    wf=$(create_intervention_workflow '  - condition: "stuck_iterations >= 3"
    action: request_human
    message: "help"')
    set_state_field "stuck_iterations" 5

    # First call: triggers intervention
    run_intervention_with_mocks \
        "check_intervention_triggers '$wf'"
    [[ "$status" -eq 2 ]]

    run cat "$CALL_LOG"
    [[ "$output" == *"action_request_human"* ]]

    # Now simulate that action_request_human set the intervention_request in state
    # (the real action_request_human does this, but our mock doesn't)
    jq '.intervention_request = {"reason": "help", "requested_at": "2024-01-01T00:00:00Z", "options": ["resolve"]}' \
        "$STATE_FILE" > "${STATE_FILE}.tmp" && mv "${STATE_FILE}.tmp" "$STATE_FILE"

    : > "$CALL_LOG"

    # Second call: should detect existing request and return 2 without calling action
    run_intervention_with_mocks \
        "check_intervention_triggers '$wf'"
    [[ "$status" -eq 2 ]]

    run cat "$CALL_LOG"
    [[ -z "$output" ]]
}

@test "test_condition_gte_boundary_equal: stuck_iterations >= 5 with value exactly 5" {
    local wf
    wf=$(create_intervention_workflow '  - condition: "stuck_iterations >= 5"
    action: request_human
    message: "boundary"')
    set_state_field "stuck_iterations" 5

    run_intervention_with_mocks \
        "check_intervention_triggers '$wf'"
    [[ "$status" -eq 2 ]]

    run cat "$CALL_LOG"
    [[ "$output" == *"action_request_human boundary"* ]]
}

@test "test_condition_gt_boundary_not_met: iteration > 10 with value exactly 10" {
    local wf
    wf=$(create_intervention_workflow '  - condition: "iteration > 10"
    action: request_human
    message: "should not trigger"')
    set_state_field "iteration" 10

    run_intervention_with_mocks \
        "check_intervention_triggers '$wf'"
    [[ "$status" -eq 0 ]]

    run cat "$CALL_LOG"
    [[ -z "$output" ]]
}

@test "test_non_integer_right_operand_with_gt: error for non-integer right operand" {
    local state_json='{"iteration": 5}'
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_INTERVENTION_SH'
        evaluate_condition 'iteration > abc' '$state_json'
    "
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Non-integer right operand"* ]]
}

@test "test_evaluate_condition_string_equality: string == comparison" {
    local state_json='{"current_state": "impl"}'
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_INTERVENTION_SH'
        evaluate_condition 'current_state == impl' '$state_json'
    "
    [[ "$status" -eq 0 ]]
}

@test "test_evaluate_condition_string_inequality: string != comparison" {
    local state_json='{"current_state": "impl"}'
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_INTERVENTION_SH'
        evaluate_condition 'current_state != qa' '$state_json'
    "
    [[ "$status" -eq 0 ]]
}

@test "test_evaluate_condition_negative_number_comparison: -5 < 0" {
    local state_json='{"balance": -5}'
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_INTERVENTION_SH'
        evaluate_condition 'balance < 0' '$state_json'
    "
    [[ "$status" -eq 0 ]]
}

@test "test_evaluate_condition_both_variables: compares two state values" {
    local state_json='{"tests_passing": 10, "tests_total": 10}'
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_INTERVENTION_SH'
        evaluate_condition 'tests_passing == tests_total' '$state_json'
    "
    [[ "$status" -eq 0 ]]
}
