#!/usr/bin/env bats

# Tests for check-escalation.sh
# Phase: escalation-triggers (orchestration-v4)
#
# Tests the escalation trigger evaluation and execution functions
# that handle iteration-based triggers defined in workflow.yaml.
#
# Depends on: actions.sh (action-registry phase)

setup() {
    load 'test_helper'
    setup_temp_dir

    CHECK_ESCALATION_SH="$SCRIPTS_DIR/check-escalation.sh"
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
# Helper: Create a workflow YAML with an escalation block on the "impl" state
# Usage: create_escalation_workflow <escalation_yaml>
# The escalation_yaml should be indented content under `escalation:`
# ==============================================================================
create_escalation_workflow() {
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
  - name: simple_state
    prompt: prompts/feature/qa.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML
    echo "$output_file"
}

# ==============================================================================
# Helper: Create a workflow YAML with no escalation section
# ==============================================================================
create_no_escalation_workflow() {
    local output_file="$TEST_TEMP_DIR/workflow.yaml"

    cat > "$output_file" << 'YAML'
name: test_no_escalation
version: 4
states:
  - name: simple_state
    prompt: prompts/feature/qa.md
    next: complete
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: simple_state
terminal_states: [complete]
YAML
    echo "$output_file"
}

# ==============================================================================
# Helper: Source check-escalation.sh (and actions.sh) in a subshell with mock
# actions. Overrides action functions with mocks that log calls and return
# configurable exit codes.
#
# Usage: run_escalation_with_mocks <function_call> [mock_overrides]
# ==============================================================================
run_escalation_with_mocks() {
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
        source \"$CHECK_ESCALATION_SH\"

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
# Helper: Set iteration in state.json
# Usage: set_iteration <n>
# ==============================================================================
set_iteration() {
    local n="$1"
    jq --argjson n "$n" '.iteration = $n' "$STATE_FILE" > "${STATE_FILE}.tmp" \
        && mv "${STATE_FILE}.tmp" "$STATE_FILE"
}

# ==============================================================================
# Helper: Set executed_escalations in state.json
# Usage: set_executed_escalations '["after_5"]'
# ==============================================================================
set_executed_escalations() {
    local arr="$1"
    jq --argjson arr "$arr" '.executed_escalations = $arr' "$STATE_FILE" > "${STATE_FILE}.tmp" \
        && mv "${STATE_FILE}.tmp" "$STATE_FILE"
}

#=============================================================================
# Script Existence and Syntax Tests
#=============================================================================

@test "check-escalation.sh exists and is readable" {
    [[ -f "$CHECK_ESCALATION_SH" ]]
}

@test "check-escalation.sh is syntactically valid bash" {
    run bash -n "$CHECK_ESCALATION_SH"
    [[ "$status" -eq 0 ]]
}

@test "check-escalation.sh can be sourced without error" {
    run bash -c "source '$ACTIONS_SH'; source '$CHECK_ESCALATION_SH'"
    [[ "$status" -eq 0 ]]
}

@test "check-escalation.sh does not use set -e" {
    [[ -f "$CHECK_ESCALATION_SH" ]]
    # Spec requires set -uo pipefail but NOT -e
    run bash -c "grep -P '^set\\s+-(\\w*e\\w*)' '$CHECK_ESCALATION_SH' | grep -v pipefail"
    [[ "$status" -eq 1 ]]  # No matches found = good
}

@test "check-escalation.sh uses set -uo pipefail" {
    run bash -c "grep -E '^set -uo pipefail' '$CHECK_ESCALATION_SH'"
    [[ "$status" -eq 0 ]]
}

@test "all escalation functions defined after sourcing" {
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_ESCALATION_SH'
        declare -f check_escalation > /dev/null && echo 'check_escalation:ok'
        declare -f get_escalation_triggers > /dev/null && echo 'get_escalation_triggers:ok'
        declare -f find_matching_trigger > /dev/null && echo 'find_matching_trigger:ok'
        declare -f execute_escalation > /dev/null && echo 'execute_escalation:ok'
        declare -f was_trigger_executed > /dev/null && echo 'was_trigger_executed:ok'
        declare -f mark_trigger_executed > /dev/null && echo 'mark_trigger_executed:ok'
    "
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"check_escalation:ok"* ]]
    [[ "$output" == *"get_escalation_triggers:ok"* ]]
    [[ "$output" == *"find_matching_trigger:ok"* ]]
    [[ "$output" == *"execute_escalation:ok"* ]]
    [[ "$output" == *"was_trigger_executed:ok"* ]]
    [[ "$output" == *"mark_trigger_executed:ok"* ]]
}

#=============================================================================
# check_escalation Tests
#=============================================================================

@test "test_no_escalation_triggers: exit 0 when state has no escalation section" {
    local wf
    wf=$(create_no_escalation_workflow)

    run_escalation_with_mocks \
        "check_escalation '$wf' 'simple_state'"
    [[ "$status" -eq 0 ]]

    # No actions should have been called
    run cat "$CALL_LOG"
    [[ -z "$output" ]]
}

@test "test_at_iteration_match: action called when iteration equals at_iteration" {
    local wf
    wf=$(create_escalation_workflow '      - at_iteration: 3
        action: analyze_stuck')
    set_iteration 3

    run_escalation_with_mocks \
        "check_escalation '$wf' 'impl'"
    [[ "$status" -eq 0 ]]

    run cat "$CALL_LOG"
    [[ "$output" == *"action_analyze_stuck"* ]]
}

@test "test_at_iteration_no_match: action NOT called when iteration != at_iteration" {
    local wf
    wf=$(create_escalation_workflow '      - at_iteration: 3
        action: analyze_stuck')
    set_iteration 2

    run_escalation_with_mocks \
        "check_escalation '$wf' 'impl'"
    [[ "$status" -eq 0 ]]

    run cat "$CALL_LOG"
    [[ -z "$output" ]]
}

@test "test_after_iteration_first_time: action called and trigger tracked" {
    local wf
    wf=$(create_escalation_workflow '      - after_iteration: 5
        action: request_human
        params:
          message: "help"')
    set_iteration 6

    run_escalation_with_mocks \
        "check_escalation '$wf' 'impl'"
    [[ "$status" -eq 0 ]]

    # Action should have been called
    run cat "$CALL_LOG"
    [[ "$output" == *"action_request_human"* ]]

    # Trigger should be tracked in executed_escalations
    local executed
    executed=$(jq -r '.executed_escalations' "$STATE_FILE")
    [[ "$executed" != "null" ]]
    [[ "$(jq '.executed_escalations | length' "$STATE_FILE")" -gt 0 ]]
}

@test "test_after_iteration_already_executed: action NOT called when already tracked" {
    local wf
    wf=$(create_escalation_workflow '      - after_iteration: 5
        action: request_human
        params:
          message: "help"')
    set_iteration 7
    set_executed_escalations '["after_5"]'

    run_escalation_with_mocks \
        "check_escalation '$wf' 'impl'"
    [[ "$status" -eq 0 ]]

    # Action should NOT have been called (already executed)
    run cat "$CALL_LOG"
    [[ -z "$output" ]]
}

@test "test_every_n_iterations_match: action called when iteration divisible by N" {
    local wf
    wf=$(create_escalation_workflow '      - every_n_iterations: 2
        action: run_tests
        params:
          pattern: "test"
          save_to: "test_output.txt"
          expect_failure: "false"')
    set_iteration 4

    run_escalation_with_mocks \
        "check_escalation '$wf' 'impl'"
    [[ "$status" -eq 0 ]]

    # 4 % 2 == 0, action should be called
    run cat "$CALL_LOG"
    [[ "$output" == *"action_run_tests"* ]]
}

@test "test_every_n_iterations_no_match: action NOT called when iteration not divisible" {
    local wf
    wf=$(create_escalation_workflow '      - every_n_iterations: 2
        action: run_tests
        params:
          pattern: "test"
          save_to: "test_output.txt"
          expect_failure: "false"')
    set_iteration 3

    run_escalation_with_mocks \
        "check_escalation '$wf' 'impl'"
    [[ "$status" -eq 0 ]]

    # 3 % 2 != 0, action should NOT be called
    run cat "$CALL_LOG"
    [[ -z "$output" ]]
}

@test "test_escalation_with_params: switch_model called with model param" {
    local wf
    wf=$(create_escalation_workflow '      - at_iteration: 5
        action: switch_model
        params:
          model: opus')
    set_iteration 5

    run_escalation_with_mocks \
        "check_escalation '$wf' 'impl'"
    [[ "$status" -eq 0 ]]

    run cat "$CALL_LOG"
    [[ "$output" == *"action_switch_model opus"* ]]
}

@test "test_first_matching_trigger_only: only first matching trigger executed" {
    # Create workflow where multiple triggers match iteration 6
    local wf
    wf=$(create_escalation_workflow '      - every_n_iterations: 2
        action: run_tests
        params:
          pattern: "test"
          save_to: "test_output.txt"
          expect_failure: "false"
      - every_n_iterations: 3
        action: analyze_stuck')
    set_iteration 6

    run_escalation_with_mocks \
        "check_escalation '$wf' 'impl'"
    [[ "$status" -eq 0 ]]

    # Only first matching trigger (every 2) should execute
    run cat "$CALL_LOG"
    local call_count
    call_count=$(wc -l < "$CALL_LOG")
    [[ "$call_count" -eq 1 ]]
    [[ "$output" == *"action_run_tests"* ]]
    [[ "$output" != *"action_analyze_stuck"* ]]
}

@test "test_escalation_action_failure: exit 1 with error when action fails" {
    local wf
    wf=$(create_escalation_workflow '      - at_iteration: 3
        action: switch_model
        params:
          model: opus')
    set_iteration 3

    run_escalation_with_mocks \
        "check_escalation '$wf' 'impl'" \
        'action_switch_model() { echo "action_switch_model $*" >> "$CALL_LOG"; return 1; }'
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Escalation action 'switch_model' failed"* ]]
}

@test "test_iteration_zero_handling: every_n_iterations does NOT trigger at iteration 0" {
    local wf
    wf=$(create_escalation_workflow '      - every_n_iterations: 2
        action: run_tests
        params:
          pattern: "test"
          save_to: "test_output.txt"
          expect_failure: "false"')
    # iteration is already 0 by default

    run_escalation_with_mocks \
        "check_escalation '$wf' 'impl'"
    [[ "$status" -eq 0 ]]

    # Iteration 0 is special case — should NOT trigger
    run cat "$CALL_LOG"
    [[ -z "$output" ]]
}

@test "test_multiple_trigger_types: at_iteration wins when it matches first" {
    local wf
    wf=$(create_escalation_workflow '      - at_iteration: 4
        action: analyze_stuck
      - after_iteration: 3
        action: request_human
        params:
          message: "help"
      - every_n_iterations: 2
        action: run_tests
        params:
          pattern: "test"
          save_to: "test_output.txt"
          expect_failure: "false"')
    set_iteration 4

    run_escalation_with_mocks \
        "check_escalation '$wf' 'impl'"
    [[ "$status" -eq 0 ]]

    # at_iteration: 4 is listed first and matches → only it executes
    run cat "$CALL_LOG"
    local call_count
    call_count=$(wc -l < "$CALL_LOG")
    [[ "$call_count" -eq 1 ]]
    [[ "$output" == *"action_analyze_stuck"* ]]
}

@test "test_after_iteration_exact_boundary_no_trigger: after_iteration:5 does NOT trigger at iteration 5" {
    local wf
    wf=$(create_escalation_workflow '      - after_iteration: 5
        action: request_human
        params:
          message: "help"')
    set_iteration 5

    run_escalation_with_mocks \
        "check_escalation '$wf' 'impl'"
    [[ "$status" -eq 0 ]]

    # 5 is NOT > 5, so action should NOT be called
    run cat "$CALL_LOG"
    [[ -z "$output" ]]
}

@test "test_unknown_action_error: exit 1 for unknown action name" {
    local wf
    wf=$(create_escalation_workflow '      - at_iteration: 1
        action: unknown_action')
    set_iteration 1

    run_escalation_with_mocks \
        "check_escalation '$wf' 'impl'"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Unknown action"* ]]
}

@test "test_every_n_iterations_1: triggers every iteration (5 % 1 == 0)" {
    local wf
    wf=$(create_escalation_workflow '      - every_n_iterations: 1
        action: run_tests
        params:
          pattern: "test"
          save_to: "test_output.txt"
          expect_failure: "false"')
    set_iteration 5

    run_escalation_with_mocks \
        "check_escalation '$wf' 'impl'"
    [[ "$status" -eq 0 ]]

    run cat "$CALL_LOG"
    [[ "$output" == *"action_run_tests"* ]]
}

@test "test_multiple_triggers_same_iteration: only first trigger executed" {
    local wf
    wf=$(create_escalation_workflow '      - at_iteration: 3
        action: analyze_stuck
      - at_iteration: 3
        action: switch_model
        params:
          model: opus')
    set_iteration 3

    run_escalation_with_mocks \
        "check_escalation '$wf' 'impl'"
    [[ "$status" -eq 0 ]]

    # Only first trigger should execute
    run cat "$CALL_LOG"
    local call_count
    call_count=$(wc -l < "$CALL_LOG")
    [[ "$call_count" -eq 1 ]]
    [[ "$output" == *"action_analyze_stuck"* ]]
    [[ "$output" != *"action_switch_model"* ]]
}

#=============================================================================
# get_escalation_triggers Tests
#=============================================================================

@test "test_get_escalation_triggers_exists: returns JSON array when triggers defined" {
    local wf
    wf=$(create_escalation_workflow '      - at_iteration: 3
        action: analyze_stuck
      - at_iteration: 5
        action: switch_model
        params:
          model: opus')

    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_ESCALATION_SH'
        get_escalation_triggers '$wf' 'impl'
    "
    [[ "$status" -eq 0 ]]
    # Should output a JSON array with 2 elements
    local count
    count=$(echo "$output" | jq 'length')
    [[ "$count" -eq 2 ]]
}

@test "test_get_escalation_triggers_missing: returns empty array when no escalation section" {
    local wf
    wf=$(create_no_escalation_workflow)

    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_ESCALATION_SH'
        get_escalation_triggers '$wf' 'simple_state'
    "
    [[ "$status" -eq 0 ]]
    [[ "$(echo "$output" | jq 'length')" -eq 0 ]]
}

#=============================================================================
# find_matching_trigger Tests
#=============================================================================

@test "test_find_matching_trigger_direct: at_iteration match returns trigger object" {
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_ESCALATION_SH'
        find_matching_trigger '[{\"at_iteration\": 3, \"action\": \"analyze_stuck\"}]' 3
    "
    [[ "$status" -eq 0 ]]
    # Should output a JSON object with the matching trigger
    local action
    action=$(echo "$output" | jq -r '.action')
    [[ "$action" == "analyze_stuck" ]]
}

@test "test_find_matching_trigger_empty_array: returns null for empty array" {
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_ESCALATION_SH'
        find_matching_trigger '[]' 5
    "
    [[ "$status" -eq 0 ]]
    [[ "$output" == "null" ]]
}

@test "test_find_matching_trigger_no_match: returns null when no trigger matches" {
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_ESCALATION_SH'
        find_matching_trigger '[{\"at_iteration\": 3, \"action\": \"analyze_stuck\"}]' 5
    "
    [[ "$status" -eq 0 ]]
    [[ "$output" == "null" ]]
}

@test "test_find_matching_trigger_after_iteration: matches when iteration > N" {
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_ESCALATION_SH'
        find_matching_trigger '[{\"after_iteration\": 5, \"action\": \"request_human\"}]' 7
    "
    [[ "$status" -eq 0 ]]
    local action
    action=$(echo "$output" | jq -r '.action')
    [[ "$action" == "request_human" ]]
}

@test "test_find_matching_trigger_after_not_at_boundary: no match when iteration == N" {
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_ESCALATION_SH'
        find_matching_trigger '[{\"after_iteration\": 5, \"action\": \"request_human\"}]' 5
    "
    [[ "$status" -eq 0 ]]
    [[ "$output" == "null" ]]
}

@test "test_find_matching_trigger_every_n: matches when iteration divisible by N" {
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_ESCALATION_SH'
        find_matching_trigger '[{\"every_n_iterations\": 3, \"action\": \"run_tests\"}]' 9
    "
    [[ "$status" -eq 0 ]]
    local action
    action=$(echo "$output" | jq -r '.action')
    [[ "$action" == "run_tests" ]]
}

@test "test_find_matching_trigger_every_n_zero: no match at iteration 0" {
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_ESCALATION_SH'
        find_matching_trigger '[{\"every_n_iterations\": 2, \"action\": \"run_tests\"}]' 0
    "
    [[ "$status" -eq 0 ]]
    [[ "$output" == "null" ]]
}

@test "test_find_matching_trigger_first_match: returns first matching trigger only" {
    run bash -c "
        source '$ACTIONS_SH'
        source '$CHECK_ESCALATION_SH'
        find_matching_trigger '[{\"at_iteration\": 6, \"action\": \"analyze_stuck\"}, {\"every_n_iterations\": 2, \"action\": \"run_tests\"}]' 6
    "
    [[ "$status" -eq 0 ]]
    local action
    action=$(echo "$output" | jq -r '.action')
    [[ "$action" == "analyze_stuck" ]]
}

#=============================================================================
# execute_escalation Tests
#=============================================================================

@test "test_execute_escalation_direct: analyze_stuck called with no arguments" {
    set_iteration 3
    run_escalation_with_mocks \
        "execute_escalation '{\"action\": \"analyze_stuck\"}'"
    [[ "$status" -eq 0 ]]

    run cat "$CALL_LOG"
    [[ "$output" == *"action_analyze_stuck"* ]]
}

@test "test_execute_escalation_empty_params: analyze_stuck called with empty params" {
    set_iteration 3
    run_escalation_with_mocks \
        "execute_escalation '{\"action\": \"analyze_stuck\", \"params\": {}}'"
    [[ "$status" -eq 0 ]]

    run cat "$CALL_LOG"
    [[ "$output" == *"action_analyze_stuck"* ]]
}

@test "test_execute_escalation_message_with_spaces: message passed as single argument" {
    set_iteration 8
    run_escalation_with_mocks \
        "execute_escalation '{\"action\": \"request_human\", \"params\": {\"message\": \"Tests consistently failing on edge case X\"}}'"
    [[ "$status" -eq 0 ]]

    run cat "$CALL_LOG"
    [[ "$output" == *"action_request_human Tests consistently failing on edge case X"* ]]
}

@test "test_execute_escalation_unknown_function: exit 1 for unknown action" {
    set_iteration 1
    run_escalation_with_mocks \
        "execute_escalation '{\"action\": \"nonexistent\", \"params\": {}}'"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Unknown action 'nonexistent'"* ]]
}

@test "test_execute_escalation_commit_params: commit called with message and when" {
    set_iteration 5
    run_escalation_with_mocks \
        "execute_escalation '{\"action\": \"commit\", \"params\": {\"message\": \"auto-commit\", \"when\": \"approved\"}}'"
    [[ "$status" -eq 0 ]]

    run cat "$CALL_LOG"
    [[ "$output" == *"action_commit auto-commit approved"* ]]
}

@test "test_execute_escalation_script_params: script called with path" {
    set_iteration 5
    run_escalation_with_mocks \
        "execute_escalation '{\"action\": \"script\", \"params\": {\"path\": \"scripts/custom.sh\"}}'"
    [[ "$status" -eq 0 ]]

    run cat "$CALL_LOG"
    [[ "$output" == *"action_script scripts/custom.sh"* ]]
}

@test "test_execute_escalation_run_tests_all_params: run_tests called with 3 args in order" {
    set_iteration 4
    run_escalation_with_mocks \
        "execute_escalation '{\"action\": \"run_tests\", \"params\": {\"pattern\": \"qa_test\", \"save_to\": \"output.txt\", \"expect_failure\": \"true\"}}'"
    [[ "$status" -eq 0 ]]

    run cat "$CALL_LOG"
    [[ "$output" == *"action_run_tests qa_test output.txt true"* ]]
}

@test "test_execute_escalation_run_tests_missing_save_to: defaults used for missing params" {
    set_iteration 4
    run_escalation_with_mocks \
        "execute_escalation '{\"action\": \"run_tests\", \"params\": {\"pattern\": \"qa_test\"}}'"
    [[ "$status" -eq 0 ]]

    # save_to defaults to "test_output.txt", expect_failure defaults to "false"
    run cat "$CALL_LOG"
    [[ "$output" == *"action_run_tests qa_test test_output.txt false"* ]]
}

@test "test_execute_escalation_switch_model: switch_model called with model" {
    set_iteration 5
    run_escalation_with_mocks \
        "execute_escalation '{\"action\": \"switch_model\", \"params\": {\"model\": \"opus\"}}'"
    [[ "$status" -eq 0 ]]

    run cat "$CALL_LOG"
    [[ "$output" == *"action_switch_model opus"* ]]
}

@test "test_execute_escalation_action_failure_propagates: exit 1 when action returns 1" {
    set_iteration 3
    run_escalation_with_mocks \
        "execute_escalation '{\"action\": \"analyze_stuck\"}'" \
        'action_analyze_stuck() { echo "action_analyze_stuck" >> "$CALL_LOG"; return 1; }'
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Escalation action 'analyze_stuck' failed"* ]]
}

@test "test_execute_escalation_logs_to_stderr: ESCALATION log message on stderr" {
    set_iteration 3
    run_escalation_with_mocks \
        "execute_escalation '{\"action\": \"analyze_stuck\"}'"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"ESCALATION: Executing analyze_stuck at iteration 3"* ]]
}

#=============================================================================
# was_trigger_executed Tests
#=============================================================================

@test "test_was_trigger_executed_true: returns 0 when trigger in executed_escalations" {
    set_executed_escalations '["after_5"]'

    run bash -c "
        export STATE_FILE=\"$STATE_FILE\"
        source '$ACTIONS_SH'
        source '$CHECK_ESCALATION_SH'
        was_trigger_executed 'impl' 'after_5'
    "
    [[ "$status" -eq 0 ]]
}

@test "test_was_trigger_executed_false: returns 1 when trigger not in list" {
    set_executed_escalations '[]'

    run bash -c "
        export STATE_FILE=\"$STATE_FILE\"
        source '$ACTIONS_SH'
        source '$CHECK_ESCALATION_SH'
        was_trigger_executed 'impl' 'after_5'
    "
    [[ "$status" -eq 1 ]]
}

@test "test_was_trigger_executed_no_array: returns 1 when executed_escalations missing" {
    # Default state.json has no executed_escalations field
    run bash -c "
        export STATE_FILE=\"$STATE_FILE\"
        source '$ACTIONS_SH'
        source '$CHECK_ESCALATION_SH'
        was_trigger_executed 'impl' 'after_5'
    "
    [[ "$status" -eq 1 ]]
}

#=============================================================================
# mark_trigger_executed Tests
#=============================================================================

@test "test_mark_trigger_executed: adds trigger to executed_escalations" {
    run bash -c "
        export STATE_FILE=\"$STATE_FILE\"
        source '$ACTIONS_SH'
        source '$CHECK_ESCALATION_SH'
        mark_trigger_executed 'impl' 'after_5'
    "
    [[ "$status" -eq 0 ]]

    # Verify state.json updated
    local executed
    executed=$(jq -r '.executed_escalations' "$STATE_FILE")
    [[ "$executed" != "null" ]]
    [[ "$(jq -r '.executed_escalations[0]' "$STATE_FILE")" == "after_5" ]]
}

@test "test_mark_trigger_executed_idempotent: duplicate not added" {
    set_executed_escalations '["after_5"]'

    run bash -c "
        export STATE_FILE=\"$STATE_FILE\"
        source '$ACTIONS_SH'
        source '$CHECK_ESCALATION_SH'
        mark_trigger_executed 'impl' 'after_5'
    "
    [[ "$status" -eq 0 ]]

    # Should still have exactly one entry (not duplicated)
    local count
    count=$(jq '.executed_escalations | length' "$STATE_FILE")
    [[ "$count" -eq 1 ]]
}

@test "test_mark_trigger_executed_preserves_existing: appends without clobbering" {
    set_executed_escalations '["after_3"]'

    run bash -c "
        export STATE_FILE=\"$STATE_FILE\"
        source '$ACTIONS_SH'
        source '$CHECK_ESCALATION_SH'
        mark_trigger_executed 'impl' 'after_7'
    "
    [[ "$status" -eq 0 ]]

    # Should have both entries
    local count
    count=$(jq '.executed_escalations | length' "$STATE_FILE")
    [[ "$count" -eq 2 ]]
    [[ "$(jq -r '.executed_escalations | sort | .[0]' "$STATE_FILE")" == "after_3" ]]
    [[ "$(jq -r '.executed_escalations | sort | .[1]' "$STATE_FILE")" == "after_7" ]]
}

#=============================================================================
# Edge Case Tests
#=============================================================================

@test "test_state_file_not_set: check_escalation fails when STATE_FILE unset" {
    local wf
    wf=$(create_escalation_workflow '      - at_iteration: 3
        action: analyze_stuck')
    set_iteration 3

    run bash -c "
        unset STATE_FILE
        export PHASE_DIR=\"$PHASE_DIR\"
        source '$ACTIONS_SH'
        source '$CHECK_ESCALATION_SH'

        action_analyze_stuck() { return 0; }

        check_escalation '$wf' 'impl'
    "
    [[ "$status" -ne 0 ]]
}

@test "test_empty_escalation_array: exit 0 with explicit empty array" {
    local wf="$TEST_TEMP_DIR/workflow.yaml"
    cat > "$wf" << 'YAML'
name: test_empty_escalation
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

    run_escalation_with_mocks \
        "check_escalation '$wf' 'impl'"
    [[ "$status" -eq 0 ]]

    run cat "$CALL_LOG"
    [[ -z "$output" ]]
}

@test "test_state_json_valid_after_escalation: state.json remains valid JSON" {
    local wf
    wf=$(create_escalation_workflow '      - after_iteration: 2
        action: request_human
        params:
          message: "help"')
    set_iteration 5

    run_escalation_with_mocks \
        "check_escalation '$wf' 'impl'"
    [[ "$status" -eq 0 ]]

    # State file should still be valid JSON
    run jq '.' "$STATE_FILE"
    [[ "$status" -eq 0 ]]

    # executed_escalations should exist
    local has_field
    has_field=$(jq 'has("executed_escalations")' "$STATE_FILE")
    [[ "$has_field" == "true" ]]
}

@test "test_escalation_preserves_other_state_fields: existing fields remain" {
    local wf
    wf=$(create_escalation_workflow '      - after_iteration: 2
        action: request_human
        params:
          message: "test"')

    # Add extra fields to state
    jq '.iteration = 5 | .current_state = "impl" | .tests_total = 10' "$STATE_FILE" > "${STATE_FILE}.tmp" \
        && mv "${STATE_FILE}.tmp" "$STATE_FILE"

    run_escalation_with_mocks \
        "check_escalation '$wf' 'impl'"
    [[ "$status" -eq 0 ]]

    # Original fields should be preserved
    local iter
    iter=$(jq -r '.iteration' "$STATE_FILE")
    [[ "$iter" -eq 5 ]]

    local tests
    tests=$(jq -r '.tests_total' "$STATE_FILE")
    [[ "$tests" -eq 10 ]]
}

@test "test_check_escalation_nonexistent_state: exit 0 when state not in workflow" {
    local wf
    wf=$(create_escalation_workflow '      - at_iteration: 3
        action: analyze_stuck')
    set_iteration 3

    run_escalation_with_mocks \
        "check_escalation '$wf' 'nonexistent_state'"
    [[ "$status" -eq 0 ]]

    # No action should be called for a state that doesn't exist
    run cat "$CALL_LOG"
    [[ -z "$output" ]]
}

@test "test_at_iteration_high_value: works with large iteration numbers" {
    local wf
    wf=$(create_escalation_workflow '      - at_iteration: 100
        action: analyze_stuck')
    set_iteration 100

    run_escalation_with_mocks \
        "check_escalation '$wf' 'impl'"
    [[ "$status" -eq 0 ]]

    run cat "$CALL_LOG"
    [[ "$output" == *"action_analyze_stuck"* ]]
}

@test "test_every_n_large_interval: every_n_iterations with large N works" {
    local wf
    wf=$(create_escalation_workflow '      - every_n_iterations: 10
        action: run_tests
        params:
          pattern: "test"
          save_to: "test_output.txt"
          expect_failure: "false"')
    set_iteration 30

    run_escalation_with_mocks \
        "check_escalation '$wf' 'impl'"
    [[ "$status" -eq 0 ]]

    # 30 % 10 == 0
    run cat "$CALL_LOG"
    [[ "$output" == *"action_run_tests"* ]]
}

@test "test_after_iteration_tracks_correct_id: trigger_id is after_N format" {
    local wf
    wf=$(create_escalation_workflow '      - after_iteration: 7
        action: request_human
        params:
          message: "stuck after 7"')
    set_iteration 8

    run_escalation_with_mocks \
        "check_escalation '$wf' 'impl'"
    [[ "$status" -eq 0 ]]

    # Should track "after_7" in executed_escalations
    local trigger_id
    trigger_id=$(jq -r '.executed_escalations[0]' "$STATE_FILE")
    [[ "$trigger_id" == "after_7" ]]
}

@test "test_full_escalation_scenario: at_iteration 3 then after_iteration 5 across iterations" {
    local wf
    wf=$(create_escalation_workflow '      - at_iteration: 3
        action: analyze_stuck
      - after_iteration: 5
        action: request_human
        params:
          message: "help"')

    # Iteration 3: at_iteration should trigger
    set_iteration 3
    run_escalation_with_mocks \
        "check_escalation '$wf' 'impl'"
    [[ "$status" -eq 0 ]]
    run cat "$CALL_LOG"
    [[ "$output" == *"action_analyze_stuck"* ]]

    # Reset call log
    : > "$CALL_LOG"

    # Iteration 6: after_iteration should trigger (first time)
    set_iteration 6
    run_escalation_with_mocks \
        "check_escalation '$wf' 'impl'"
    [[ "$status" -eq 0 ]]
    run cat "$CALL_LOG"
    [[ "$output" == *"action_request_human"* ]]

    # Reset call log
    : > "$CALL_LOG"

    # Iteration 7: after_iteration should NOT trigger again (already executed)
    set_iteration 7
    run_escalation_with_mocks \
        "check_escalation '$wf' 'impl'"
    [[ "$status" -eq 0 ]]
    run cat "$CALL_LOG"
    [[ -z "$output" ]]
}

@test "test_execute_escalation_commit_message_with_spaces: commit message not word-split" {
    set_iteration 5
    run_escalation_with_mocks \
        "execute_escalation '{\"action\": \"commit\", \"params\": {\"message\": \"feat: implement new feature\"}}'"
    [[ "$status" -eq 0 ]]

    run cat "$CALL_LOG"
    [[ "$output" == *"action_commit feat: implement new feature"* ]]
}

@test "test_execute_escalation_run_tests_pattern_with_spaces: handles pattern with underscores" {
    set_iteration 4
    run_escalation_with_mocks \
        "execute_escalation '{\"action\": \"run_tests\", \"params\": {\"pattern\": \"qa_escalation_triggers\", \"save_to\": \"test_output.txt\", \"expect_failure\": \"false\"}}'"
    [[ "$status" -eq 0 ]]

    run cat "$CALL_LOG"
    [[ "$output" == *"action_run_tests qa_escalation_triggers test_output.txt false"* ]]
}
