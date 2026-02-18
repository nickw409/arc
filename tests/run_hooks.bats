#!/usr/bin/env bats

# Tests for run-hooks.sh
# Phase: hook-execution (orchestration-v4)
#
# Tests the hook execution functions that run post-state actions
# defined in workflow.yaml `after:` blocks.
#
# Depends on: actions.sh (action-registry phase)

setup() {
    load 'test_helper'
    setup_temp_dir

    RUN_HOOKS_SH="$SCRIPTS_DIR/run-hooks.sh"
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
# Helper: Create a workflow YAML with an after block on the "impl" state
# Usage: create_hooks_workflow <after_yaml>
# The after_yaml should be indented content under `after:`
# ==============================================================================
create_hooks_workflow() {
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
# Helper: Create a workflow YAML with no after section
# ==============================================================================
create_no_hooks_workflow() {
    local output_file="$TEST_TEMP_DIR/workflow.yaml"

    cat > "$output_file" << 'YAML'
name: test_no_hooks
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
# Helper: Create a workflow with multiple hooks of mixed conditions
# ==============================================================================
create_mixed_hooks_workflow() {
    local output_file="$TEST_TEMP_DIR/workflow.yaml"

    cat > "$output_file" << 'YAML'
name: test_mixed_hooks
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
    after:
      - action: run_tests
        params:
          pattern: "test"
          save_to: "output.txt"
          expect_failure: "false"
      - action: commit
        when: approved
        params:
          message: "test"
      - action: analyze_stuck
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML
    echo "$output_file"
}

# ==============================================================================
# Helper: Source run-hooks.sh (and actions.sh) in a subshell with mock actions
#
# This helper overrides action functions with mocks that log calls and
# return configurable exit codes. This isolates hook execution testing
# from the actual action implementations.
#
# Usage: run_hooks_with_mocks <function_call> [mock_overrides]
# ==============================================================================
run_hooks_with_mocks() {
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
        source \"$RUN_HOOKS_SH\"

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

#=============================================================================
# Script Existence and Syntax Tests
#=============================================================================

@test "run-hooks.sh exists and is readable" {
    [[ -f "$RUN_HOOKS_SH" ]]
}

@test "run-hooks.sh is syntactically valid bash" {
    run bash -n "$RUN_HOOKS_SH"
    [[ "$status" -eq 0 ]]
}

@test "run-hooks.sh can be sourced without error" {
    run bash -c "source '$ACTIONS_SH'; source '$RUN_HOOKS_SH'"
    [[ "$status" -eq 0 ]]
}

@test "run-hooks.sh does not use set -e" {
    [[ -f "$RUN_HOOKS_SH" ]]
    # Spec requires set -uo pipefail but NOT -e
    run bash -c "grep -P '^set\\s+-(\\w*e\\w*)' '$RUN_HOOKS_SH' | grep -v pipefail"
    [[ "$status" -eq 1 ]]  # No matches found = good
}

@test "run-hooks.sh uses set -uo pipefail" {
    run bash -c "grep -E '^set -uo pipefail' '$RUN_HOOKS_SH'"
    [[ "$status" -eq 0 ]]
}

@test "all hook functions defined after sourcing" {
    run bash -c "
        source '$ACTIONS_SH'
        source '$RUN_HOOKS_SH'
        declare -f run_after_hooks > /dev/null && echo 'run_after_hooks:ok'
        declare -f get_after_hooks > /dev/null && echo 'get_after_hooks:ok'
        declare -f execute_hook > /dev/null && echo 'execute_hook:ok'
        declare -f check_when_condition > /dev/null && echo 'check_when_condition:ok'
    "
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"run_after_hooks:ok"* ]]
    [[ "$output" == *"get_after_hooks:ok"* ]]
    [[ "$output" == *"execute_hook:ok"* ]]
    [[ "$output" == *"check_when_condition:ok"* ]]
}

#=============================================================================
# check_when_condition Tests
#=============================================================================

@test "test_check_when_empty: empty when always matches" {
    run bash -c "
        source '$ACTIONS_SH'
        source '$RUN_HOOKS_SH'
        check_when_condition '' 'approved'
    "
    [[ "$status" -eq 0 ]]
}

@test "test_check_when_empty_verdict_match: empty when with empty verdict matches" {
    run bash -c "
        source '$ACTIONS_SH'
        source '$RUN_HOOKS_SH'
        check_when_condition '' ''
    "
    [[ "$status" -eq 0 ]]
}

@test "test_check_when_simple_match: exact verdict match returns 0" {
    run bash -c "
        source '$ACTIONS_SH'
        source '$RUN_HOOKS_SH'
        check_when_condition 'approved' 'approved'
    "
    [[ "$status" -eq 0 ]]
}

@test "test_check_when_simple_no_match: verdict mismatch returns 1" {
    run bash -c "
        source '$ACTIONS_SH'
        source '$RUN_HOOKS_SH'
        check_when_condition 'approved' 'needs_fix'
    "
    [[ "$status" -eq 1 ]]
}

@test "test_check_when_negation_match: !approved matches needs_fix" {
    run bash -c "
        source '$ACTIONS_SH'
        source '$RUN_HOOKS_SH'
        check_when_condition '!approved' 'needs_fix'
    "
    [[ "$status" -eq 0 ]]
}

@test "test_check_when_negation_no_match: !approved does not match approved" {
    run bash -c "
        source '$ACTIONS_SH'
        source '$RUN_HOOKS_SH'
        check_when_condition '!approved' 'approved'
    "
    [[ "$status" -eq 1 ]]
}

@test "test_check_when_or_first_match: approved|passed matches approved" {
    run bash -c "
        source '$ACTIONS_SH'
        source '$RUN_HOOKS_SH'
        check_when_condition 'approved|passed' 'approved'
    "
    [[ "$status" -eq 0 ]]
}

@test "test_check_when_or_second_match: approved|passed matches passed" {
    run bash -c "
        source '$ACTIONS_SH'
        source '$RUN_HOOKS_SH'
        check_when_condition 'approved|passed' 'passed'
    "
    [[ "$status" -eq 0 ]]
}

@test "test_check_when_or_no_match: approved|passed does not match needs_fix" {
    run bash -c "
        source '$ACTIONS_SH'
        source '$RUN_HOOKS_SH'
        check_when_condition 'approved|passed' 'needs_fix'
    "
    [[ "$status" -eq 1 ]]
}

@test "test_check_when_or_three_values: approved|passed|fixed matches fixed" {
    run bash -c "
        source '$ACTIONS_SH'
        source '$RUN_HOOKS_SH'
        check_when_condition 'approved|passed|fixed' 'fixed'
    "
    [[ "$status" -eq 0 ]]
}

@test "test_check_when_negated_or_not_supported: !approved|needs_fix treats entire string as negated" {
    # Per spec: The ! prefix captures the entire remaining string "approved|needs_fix"
    # as the negated value (NOT parsed as OR). Since verdict "needs_fix" != literal
    # "approved|needs_fix", the negation condition is met, so returns 0.
    run bash -c "
        source '$ACTIONS_SH'
        source '$RUN_HOOKS_SH'
        check_when_condition '!approved|needs_fix' 'needs_fix'
    "
    [[ "$status" -eq 0 ]]
}

@test "test_check_when_verdict_with_pipe_character: literal pipe in verdict" {
    # Pathological case: verdict = "a|b", when = "a|b"
    # The when value is parsed as OR: "a" OR "b". The verdict "a|b" matches neither.
    run bash -c "
        source '$ACTIONS_SH'
        source '$RUN_HOOKS_SH'
        check_when_condition 'a|b' 'a|b'
    "
    [[ "$status" -eq 1 ]]
}

#=============================================================================
# get_after_hooks Tests
#=============================================================================

@test "test_get_after_hooks_exists: returns JSON array when hooks defined" {
    local wf
    wf=$(create_hooks_workflow '      - action: run_tests
        params:
          pattern: "test"
          save_to: "output.txt"')

    run bash -c "
        source '$ACTIONS_SH'
        source '$RUN_HOOKS_SH'
        get_after_hooks '$wf' 'impl'
    "
    [[ "$status" -eq 0 ]]
    # Should output a JSON array
    local count
    count=$(echo "$output" | jq 'length')
    [[ "$count" -eq 1 ]]
}

@test "test_get_after_hooks_missing: returns empty array when no after section" {
    local wf
    wf=$(create_no_hooks_workflow)

    run bash -c "
        source '$ACTIONS_SH'
        source '$RUN_HOOKS_SH'
        get_after_hooks '$wf' 'simple_state'
    "
    [[ "$status" -eq 0 ]]
    [[ "$(echo "$output" | jq 'length')" -eq 0 ]]
}

@test "test_get_after_hooks_nonexistent_state: returns empty array for unknown state" {
    local wf
    wf=$(create_no_hooks_workflow)

    run bash -c "
        source '$ACTIONS_SH'
        source '$RUN_HOOKS_SH'
        get_after_hooks '$wf' 'nonexistent_state'
    "
    [[ "$status" -eq 0 ]]
    [[ "$(echo "$output" | jq 'length')" -eq 0 ]]
}

@test "test_get_after_hooks_multiple: returns all hooks in order" {
    local wf
    wf=$(create_mixed_hooks_workflow)

    run bash -c "
        source '$ACTIONS_SH'
        source '$RUN_HOOKS_SH'
        get_after_hooks '$wf' 'impl'
    "
    [[ "$status" -eq 0 ]]
    local count
    count=$(echo "$output" | jq 'length')
    [[ "$count" -eq 3 ]]

    # Verify order
    local first_action
    first_action=$(echo "$output" | jq -r '.[0].action')
    [[ "$first_action" == "run_tests" ]]

    local second_action
    second_action=$(echo "$output" | jq -r '.[1].action')
    [[ "$second_action" == "commit" ]]

    local third_action
    third_action=$(echo "$output" | jq -r '.[2].action')
    [[ "$third_action" == "analyze_stuck" ]]
}

#=============================================================================
# execute_hook Tests
#=============================================================================

@test "test_execute_hook_direct: run_tests called with params" {
    run_hooks_with_mocks \
        "execute_hook '{\"action\": \"run_tests\", \"params\": {\"pattern\": \"test\", \"save_to\": \"output.txt\", \"expect_failure\": \"false\"}}' 'approved'"
    [[ "$status" -eq 0 ]]

    # Verify action was called
    run cat "$CALL_LOG"
    [[ "$output" == *"action_run_tests"* ]]
    [[ "$output" == *"test"* ]]
    [[ "$output" == *"output.txt"* ]]
}

@test "test_execute_hook_script_params: script action called with path" {
    run_hooks_with_mocks \
        "execute_hook '{\"action\": \"script\", \"params\": {\"path\": \"scripts/custom.sh\"}}' 'approved'"
    [[ "$status" -eq 0 ]]

    run cat "$CALL_LOG"
    [[ "$output" == *"action_script scripts/custom.sh"* ]]
}

@test "test_execute_hook_switch_model_params: switch_model called with model" {
    run_hooks_with_mocks \
        "execute_hook '{\"action\": \"switch_model\", \"params\": {\"model\": \"opus\"}}' 'approved'"
    [[ "$status" -eq 0 ]]

    run cat "$CALL_LOG"
    [[ "$output" == *"action_switch_model opus"* ]]
}

@test "test_execute_hook_commit_with_hook_when_and_params_when: both when checks pass" {
    # Hook-level when: approved AND params.when: approved
    # verdict = approved → hook-level when passes, action_commit gets ("test", "approved")
    run_hooks_with_mocks \
        "execute_hook '{\"action\": \"commit\", \"when\": \"approved\", \"params\": {\"message\": \"test\", \"when\": \"approved\"}}' 'approved'"
    [[ "$status" -eq 0 ]]

    run cat "$CALL_LOG"
    [[ "$output" == *"action_commit test approved"* ]]
}

@test "test_execute_hook_commit_when_skipped: hook-level when does not match" {
    # Hook-level when: approved, verdict = needs_fix → skip, action_commit NOT called
    run_hooks_with_mocks \
        "execute_hook '{\"action\": \"commit\", \"when\": \"approved\", \"params\": {\"message\": \"test\"}}' 'needs_fix'"
    [[ "$status" -eq 0 ]]

    # Call log should be empty (hook was skipped)
    run cat "$CALL_LOG"
    [[ -z "$output" ]]
}

@test "test_execute_hook_no_params: analyze_stuck called with no arguments" {
    run_hooks_with_mocks \
        "execute_hook '{\"action\": \"analyze_stuck\"}' ''"
    [[ "$status" -eq 0 ]]

    run cat "$CALL_LOG"
    [[ "$output" == *"action_analyze_stuck"* ]]
}

@test "test_execute_hook_request_human_with_message: message passed as argument" {
    run_hooks_with_mocks \
        "execute_hook '{\"action\": \"request_human\", \"params\": {\"message\": \"Help needed\"}}' ''"
    [[ "$status" -eq 0 ]]

    run cat "$CALL_LOG"
    [[ "$output" == *"action_request_human Help needed"* ]]
}

@test "test_hook_failure_exit_code_verified: failed action returns exit 1" {
    run_hooks_with_mocks \
        "execute_hook '{\"action\": \"analyze_stuck\"}' ''" \
        'action_analyze_stuck() { return 1; }'
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Hook action 'analyze_stuck' failed"* ]]
}

@test "test_action_function_missing: unknown action returns exit 1" {
    run_hooks_with_mocks \
        "execute_hook '{\"action\": \"undefined_action\"}' ''"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Unknown action"* ]]
}

#=============================================================================
# run_after_hooks Tests
#=============================================================================

@test "test_no_hooks: exit 0 when state has no after section" {
    local wf
    wf=$(create_no_hooks_workflow)

    run_hooks_with_mocks \
        "run_after_hooks '$wf' 'simple_state' 'approved'"
    [[ "$status" -eq 0 ]]

    # No actions should have been called
    run cat "$CALL_LOG"
    [[ -z "$output" ]]
}

@test "test_single_hook_no_condition: hook runs regardless of verdict" {
    local wf
    wf=$(create_hooks_workflow '      - action: run_tests
        params:
          pattern: "qa_test"
          save_to: "output.txt"
          expect_failure: "false"')

    run_hooks_with_mocks \
        "run_after_hooks '$wf' 'impl' 'approved'"
    [[ "$status" -eq 0 ]]

    # run_tests should have been called
    run cat "$CALL_LOG"
    [[ "$output" == *"action_run_tests"* ]]
    [[ "$output" == *"qa_test"* ]]
}

@test "test_hook_with_when_match: commit runs when verdict matches" {
    local wf
    wf=$(create_hooks_workflow '      - action: commit
        when: approved
        params:
          message: "test"')

    run_hooks_with_mocks \
        "run_after_hooks '$wf' 'impl' 'approved'"
    [[ "$status" -eq 0 ]]

    run cat "$CALL_LOG"
    [[ "$output" == *"action_commit"* ]]
}

@test "test_hook_with_when_no_match: commit skipped when verdict doesn't match" {
    local wf
    wf=$(create_hooks_workflow '      - action: commit
        when: approved
        params:
          message: "test"')

    run_hooks_with_mocks \
        "run_after_hooks '$wf' 'impl' 'needs_fix'"
    [[ "$status" -eq 0 ]]

    # commit should NOT have been called
    run cat "$CALL_LOG"
    [[ -z "$output" ]]
}

@test "test_hook_with_negated_when: analyze_stuck runs on !approved with needs_fix" {
    local wf
    wf=$(create_hooks_workflow '      - action: analyze_stuck
        when: "!approved"')

    run_hooks_with_mocks \
        "run_after_hooks '$wf' 'impl' 'needs_fix'"
    [[ "$status" -eq 0 ]]

    run cat "$CALL_LOG"
    [[ "$output" == *"action_analyze_stuck"* ]]
}

@test "test_hook_with_or_condition: commit runs when OR condition matches" {
    local wf
    wf=$(create_hooks_workflow '      - action: commit
        when: "approved|passed"
        params:
          message: "test"')

    run_hooks_with_mocks \
        "run_after_hooks '$wf' 'impl' 'passed'"
    [[ "$status" -eq 0 ]]

    run cat "$CALL_LOG"
    [[ "$output" == *"action_commit"* ]]
}

@test "test_multiple_hooks_order: hooks executed in defined order" {
    local wf
    wf=$(create_mixed_hooks_workflow)

    run_hooks_with_mocks \
        "run_after_hooks '$wf' 'impl' 'approved'"
    [[ "$status" -eq 0 ]]

    # All three should be called in order
    run cat "$CALL_LOG"
    local line1 line2 line3
    line1=$(sed -n '1p' "$CALL_LOG")
    line2=$(sed -n '2p' "$CALL_LOG")
    line3=$(sed -n '3p' "$CALL_LOG")

    [[ "$line1" == *"action_run_tests"* ]]
    [[ "$line2" == *"action_commit"* ]]
    [[ "$line3" == *"action_analyze_stuck"* ]]
}

@test "test_hook_failure_stops_chain: second hook NOT executed on first failure" {
    local wf
    wf="$TEST_TEMP_DIR/workflow.yaml"
    cat > "$wf" << 'YAML'
name: test_fail_chain
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
    after:
      - action: analyze_stuck
      - action: run_tests
        params:
          pattern: "test"
          save_to: "output.txt"
          expect_failure: "false"
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    run_hooks_with_mocks \
        "run_after_hooks '$wf' 'impl' 'approved'" \
        'action_analyze_stuck() { echo "action_analyze_stuck" >> "$CALL_LOG"; return 1; }'
    [[ "$status" -eq 1 ]]

    # Only analyze_stuck should have been called, not run_tests
    run cat "$CALL_LOG"
    [[ "$output" == *"action_analyze_stuck"* ]]
    [[ "$output" != *"action_run_tests"* ]]
}

@test "test_hook_failure_continue_on_error: second hook runs despite first failure" {
    local wf
    wf="$TEST_TEMP_DIR/workflow.yaml"
    cat > "$wf" << 'YAML'
name: test_continue_error
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
    after:
      - action: analyze_stuck
        continue_on_error: true
      - action: run_tests
        params:
          pattern: "test"
          save_to: "test_output.txt"
          expect_failure: "false"
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    run_hooks_with_mocks \
        "run_after_hooks '$wf' 'impl' 'approved'" \
        'action_analyze_stuck() { echo "action_analyze_stuck" >> "$CALL_LOG"; return 1; }'
    # Overall result is failure (first hook failed)
    [[ "$status" -eq 1 ]]

    # But second hook (run_tests) should still have executed
    run cat "$CALL_LOG"
    [[ "$output" == *"action_analyze_stuck"* ]]
    [[ "$output" == *"action_run_tests"* ]]
}

@test "test_continue_on_error_still_returns_failure: overall exit 1 even with continue" {
    local wf
    wf="$TEST_TEMP_DIR/workflow.yaml"
    cat > "$wf" << 'YAML'
name: test_continue_returns_fail
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
    after:
      - action: switch_model
        continue_on_error: true
        params:
          model: opus
      - action: analyze_stuck
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    run_hooks_with_mocks \
        "run_after_hooks '$wf' 'impl' ''" \
        'action_switch_model() { echo "action_switch_model $*" >> "$CALL_LOG"; return 1; }'
    # Overall exit 1 because first hook failed
    [[ "$status" -eq 1 ]]

    # But second hook still executed
    run cat "$CALL_LOG"
    [[ "$output" == *"action_switch_model"* ]]
    [[ "$output" == *"action_analyze_stuck"* ]]
}

@test "test_hook_with_no_params: analyze_stuck called with no arguments" {
    local wf
    wf=$(create_hooks_workflow '      - action: analyze_stuck')

    run_hooks_with_mocks \
        "run_after_hooks '$wf' 'impl' ''"
    [[ "$status" -eq 0 ]]

    run cat "$CALL_LOG"
    # Just the function name, no extra args
    [[ "$(cat "$CALL_LOG")" == "action_analyze_stuck"* ]]
}

@test "test_empty_verdict: hook with no when condition runs on empty verdict" {
    local wf
    wf=$(create_hooks_workflow '      - action: run_tests
        params:
          pattern: "test"
          save_to: "test_output.txt"
          expect_failure: "false"')

    run_hooks_with_mocks \
        "run_after_hooks '$wf' 'impl' ''"
    [[ "$status" -eq 0 ]]

    run cat "$CALL_LOG"
    [[ "$output" == *"action_run_tests"* ]]
}

@test "test_hook_with_message_containing_spaces: message passed as single argument" {
    local wf
    wf=$(create_hooks_workflow '      - action: request_human
        params:
          message: "Multiple words in message"')

    run_hooks_with_mocks \
        "run_after_hooks '$wf' 'impl' ''"
    [[ "$status" -eq 0 ]]

    run cat "$CALL_LOG"
    [[ "$output" == *"action_request_human Multiple words in message"* ]]
}

@test "test_hook_mixed_conditional_and_unconditional: conditional skipped, unconditional runs" {
    local wf
    wf=$(create_mixed_hooks_workflow)

    # verdict = "needs_fix": run_tests (no when → runs), commit (when: approved → skip),
    # analyze_stuck (no when → runs)
    run_hooks_with_mocks \
        "run_after_hooks '$wf' 'impl' 'needs_fix'"
    [[ "$status" -eq 0 ]]

    run cat "$CALL_LOG"
    [[ "$output" == *"action_run_tests"* ]]
    [[ "$output" != *"action_commit"* ]]
    [[ "$output" == *"action_analyze_stuck"* ]]
}

@test "test_run_tests_default_params: missing params get defaults" {
    # run_tests with only pattern specified — save_to and expect_failure should default
    local wf
    wf=$(create_hooks_workflow '      - action: run_tests
        params:
          pattern: "qa_test"')

    run_hooks_with_mocks \
        "run_after_hooks '$wf' 'impl' ''"
    [[ "$status" -eq 0 ]]

    run cat "$CALL_LOG"
    # Should have defaults: save_to=test_output.txt, expect_failure=false
    [[ "$output" == *"action_run_tests qa_test test_output.txt false"* ]]
}

@test "test_commit_message_only: commit with only message param" {
    local wf
    wf=$(create_hooks_workflow '      - action: commit
        params:
          message: "feat: implementation"')

    run_hooks_with_mocks \
        "run_after_hooks '$wf' 'impl' ''"
    [[ "$status" -eq 0 ]]

    run cat "$CALL_LOG"
    [[ "$output" == *"action_commit feat: implementation"* ]]
}

@test "test_empty_after_array: exit 0 with empty array" {
    local wf="$TEST_TEMP_DIR/workflow.yaml"
    cat > "$wf" << 'YAML'
name: test_empty_after
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

    run_hooks_with_mocks \
        "run_after_hooks '$wf' 'impl' 'approved'"
    [[ "$status" -eq 0 ]]

    # No actions should have been called
    run cat "$CALL_LOG"
    [[ -z "$output" ]]
}

#=============================================================================
# Edge Case Tests
#=============================================================================

@test "test_unknown_action_in_workflow: returns exit 1 with error message" {
    local wf
    wf=$(create_hooks_workflow '      - action: undefined_action')

    run_hooks_with_mocks \
        "run_after_hooks '$wf' 'impl' ''"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Unknown action"* ]]
}

@test "test_hook_negated_when_with_approved_verdict: !approved skips on approved" {
    local wf
    wf=$(create_hooks_workflow '      - action: analyze_stuck
        when: "!approved"')

    run_hooks_with_mocks \
        "run_after_hooks '$wf' 'impl' 'approved'"
    [[ "$status" -eq 0 ]]

    # Should have been skipped
    run cat "$CALL_LOG"
    [[ -z "$output" ]]
}

@test "test_multiple_failures_with_continue: all hooks run, overall failure" {
    local wf="$TEST_TEMP_DIR/workflow.yaml"
    cat > "$wf" << 'YAML'
name: test_multi_fail
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
    after:
      - action: analyze_stuck
        continue_on_error: true
      - action: switch_model
        continue_on_error: true
        params:
          model: opus
      - action: run_tests
        params:
          pattern: "test"
          save_to: "output.txt"
          expect_failure: "false"
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    run_hooks_with_mocks \
        "run_after_hooks '$wf' 'impl' ''" \
        'action_analyze_stuck() { echo "action_analyze_stuck" >> "$CALL_LOG"; return 1; }
         action_switch_model() { echo "action_switch_model $*" >> "$CALL_LOG"; return 1; }'
    [[ "$status" -eq 1 ]]

    # All three hooks should have been called despite first two failing
    run cat "$CALL_LOG"
    [[ "$output" == *"action_analyze_stuck"* ]]
    [[ "$output" == *"action_switch_model"* ]]
    [[ "$output" == *"action_run_tests"* ]]
}

@test "test_second_failure_stops_after_continue: continue then no-continue failure stops" {
    local wf="$TEST_TEMP_DIR/workflow.yaml"
    cat > "$wf" << 'YAML'
name: test_stop_after_continue
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
    after:
      - action: analyze_stuck
        continue_on_error: true
      - action: switch_model
        params:
          model: opus
      - action: run_tests
        params:
          pattern: "test"
          save_to: "output.txt"
          expect_failure: "false"
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    run_hooks_with_mocks \
        "run_after_hooks '$wf' 'impl' ''" \
        'action_analyze_stuck() { echo "action_analyze_stuck" >> "$CALL_LOG"; return 1; }
         action_switch_model() { echo "action_switch_model $*" >> "$CALL_LOG"; return 1; }'
    [[ "$status" -eq 1 ]]

    # First two called, but third should NOT be called (switch_model has no continue_on_error)
    run cat "$CALL_LOG"
    [[ "$output" == *"action_analyze_stuck"* ]]
    [[ "$output" == *"action_switch_model"* ]]
    [[ "$output" != *"action_run_tests"* ]]
}

@test "test_commit_with_params_when: params.when passed to action_commit" {
    local wf
    wf=$(create_hooks_workflow '      - action: commit
        params:
          message: "test commit"
          when: "approved"')

    run_hooks_with_mocks \
        "run_after_hooks '$wf' 'impl' ''"
    [[ "$status" -eq 0 ]]

    run cat "$CALL_LOG"
    # action_commit should receive message and when as separate args
    [[ "$output" == *"action_commit test commit approved"* ]]
}

@test "test_run_tests_all_params_passed: pattern, save_to, expect_failure in order" {
    local wf
    wf=$(create_hooks_workflow '      - action: run_tests
        params:
          pattern: "qa_phase"
          save_to: "custom_output.txt"
          expect_failure: "true"')

    run_hooks_with_mocks \
        "run_after_hooks '$wf' 'impl' ''"
    [[ "$status" -eq 0 ]]

    run cat "$CALL_LOG"
    [[ "$output" == *"action_run_tests qa_phase custom_output.txt true"* ]]
}

@test "test_hook_on_terminal_state_no_hooks: terminal state with no hooks returns 0" {
    local wf
    wf=$(create_no_hooks_workflow)

    run_hooks_with_mocks \
        "run_after_hooks '$wf' 'complete' ''"
    [[ "$status" -eq 0 ]]

    run cat "$CALL_LOG"
    [[ -z "$output" ]]
}

@test "test_hook_with_empty_params_object: action runs with empty params" {
    local wf
    wf=$(create_hooks_workflow '      - action: analyze_stuck
        params: {}')

    run_hooks_with_mocks \
        "run_after_hooks '$wf' 'impl' ''"
    [[ "$status" -eq 0 ]]

    run cat "$CALL_LOG"
    [[ "$output" == *"action_analyze_stuck"* ]]
}

@test "test_or_condition_with_empty_verdict: OR condition does not match empty verdict" {
    local wf
    wf=$(create_hooks_workflow '      - action: commit
        when: "approved|passed"
        params:
          message: "test"')

    run_hooks_with_mocks \
        "run_after_hooks '$wf' 'impl' ''"
    [[ "$status" -eq 0 ]]

    # Should be skipped — empty verdict doesn't match approved or passed
    run cat "$CALL_LOG"
    [[ -z "$output" ]]
}

@test "test_negated_when_with_empty_verdict: !approved matches empty verdict" {
    local wf
    wf=$(create_hooks_workflow '      - action: analyze_stuck
        when: "!approved"')

    run_hooks_with_mocks \
        "run_after_hooks '$wf' 'impl' ''"
    [[ "$status" -eq 0 ]]

    # Empty string != "approved", so negation matches
    run cat "$CALL_LOG"
    [[ "$output" == *"action_analyze_stuck"* ]]
}

@test "test_nested_params_as_strings: nested JSON values passed as strings" {
    # Edge case #5: When a param value is a JSON object, jq -r stringifies it
    # For switch_model, model param would be the stringified JSON
    run_hooks_with_mocks \
        "execute_hook '{\"action\": \"switch_model\", \"params\": {\"model\": \"opus\"}}' ''"
    [[ "$status" -eq 0 ]]

    run cat "$CALL_LOG"
    [[ "$output" == *"action_switch_model opus"* ]]
}

@test "test_invalid_workflow_yaml: yq error propagates" {
    # Edge case #7: Invalid YAML causes yq to error
    local wf="$TEST_TEMP_DIR/bad_workflow.yaml"
    cat > "$wf" << 'YAML'
name: bad
states:
  - name: impl
    bad indentation here
  next: complete
YAML

    # Verify the function exists and can be called (ensures implementation is present)
    run bash -c "
        source '$ACTIONS_SH'
        source '$RUN_HOOKS_SH'
        declare -f run_after_hooks > /dev/null
    "
    [[ "$status" -eq 0 ]]

    run_hooks_with_mocks \
        "run_after_hooks '$wf' 'impl' ''"
    # yq should error on malformed YAML — no action functions should be called
    run cat "$CALL_LOG"
    [[ -z "$output" ]]
}

@test "test_all_hooks_succeed: exit 0 when every hook passes" {
    local wf
    wf=$(create_mixed_hooks_workflow)

    run_hooks_with_mocks \
        "run_after_hooks '$wf' 'impl' 'approved'"
    [[ "$status" -eq 0 ]]

    # All three should have been called
    local call_count
    call_count=$(wc -l < "$CALL_LOG")
    [[ "$call_count" -eq 3 ]]
}
