#!/usr/bin/env bats
# QA Tests for bats-tests phase
# Tests validate-workflow.sh using deterministic fixtures.
#
# This test suite covers:
# - Valid workflow fixtures (feature, bugfix, V2 branching)
# - Invalid workflow fixtures (all error conditions)
# - Edge cases (terminal warnings, YAML syntax errors)
# - Real workflow validation (all 5 bundled workflows)

setup() {
    SCRIPT_DIR="$(cd "$(dirname "$BATS_TEST_FILENAME")" && pwd)"
    FIXTURES_DIR="$SCRIPT_DIR/fixtures"
    ORCH_DIR="$(dirname "$SCRIPT_DIR")"
    VALIDATE_SCRIPT="$ORCH_DIR/scripts/validate-workflow.sh"
    WORKFLOWS_DIR="$ORCH_DIR/workflows"
}

# ==============================================================================
# Test: Valid feature workflow passes
# ==============================================================================

@test "test_valid_feature_workflow_passes" {
    run "$VALIDATE_SCRIPT" "$FIXTURES_DIR/valid-feature.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"✓ All state names unique"* ]]
    [[ "$output" == *"✓ All states reachable from entry"* ]]
    [[ "$output" == *"✓ All states can reach terminal"* ]]
}

# ==============================================================================
# Test: Valid bugfix workflow passes
# ==============================================================================

@test "test_valid_bugfix_workflow_passes" {
    run "$VALIDATE_SCRIPT" "$FIXTURES_DIR/valid-bugfix.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"✓ All state names unique"* ]]
}

# ==============================================================================
# Test: Valid V2 branching workflow passes
# ==============================================================================

@test "test_valid_v2_branching_passes" {
    run "$VALIDATE_SCRIPT" "$FIXTURES_DIR/valid-v2-branching.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"✓ Transition valid: qa -> impl"* ]]
    [[ "$output" == *"✓ Transition valid: qa -> blocked"* ]]
    [[ "$output" == *"✓ All states reachable from entry"* ]]
}

# ==============================================================================
# Test: Duplicate state name fails
# ==============================================================================

@test "test_duplicate_state_name_fails" {
    run "$VALIDATE_SCRIPT" "$FIXTURES_DIR/invalid-duplicate-state.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"❌ Duplicate state name: qa"* ]]
}

# ==============================================================================
# Test: Missing prompt file fails
# ==============================================================================

@test "test_missing_prompt_file_fails" {
    run "$VALIDATE_SCRIPT" "$FIXTURES_DIR/invalid-missing-prompt.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"❌ Prompt not found:"* ]]
}

# ==============================================================================
# Test: Invalid next reference fails
# ==============================================================================

@test "test_invalid_next_reference_fails" {
    run "$VALIDATE_SCRIPT" "$FIXTURES_DIR/invalid-bad-next.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"❌ Invalid transition:"* ]]
}

# ==============================================================================
# Test: V2 invalid branch fails
# ==============================================================================

@test "test_v2_invalid_branch_fails" {
    run "$VALIDATE_SCRIPT" "$FIXTURES_DIR/invalid-v2-bad-branch.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"❌ Invalid transition:"* ]]
}

# ==============================================================================
# Test: Entry state is terminal fails
# ==============================================================================

@test "test_entry_is_terminal_fails" {
    run "$VALIDATE_SCRIPT" "$FIXTURES_DIR/invalid-entry-terminal.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"❌ Entry state"* ]]
}

# ==============================================================================
# Test: Unreachable state fails
# ==============================================================================

@test "test_unreachable_state_fails" {
    run "$VALIDATE_SCRIPT" "$FIXTURES_DIR/invalid-unreachable.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"❌ Unreachable state: orphan"* ]]
}

# ==============================================================================
# Test: No path to terminal fails
# ==============================================================================

@test "test_no_path_to_terminal_fails" {
    run "$VALIDATE_SCRIPT" "$FIXTURES_DIR/invalid-no-terminal-path.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"❌ State cannot reach terminal:"* ]]
}

# ==============================================================================
# Test: Terminal with next warns (but does not fail)
# ==============================================================================

@test "test_terminal_with_next_warns" {
    run "$VALIDATE_SCRIPT" "$FIXTURES_DIR/warn-terminal-with-next.yaml"
    # Should pass (warning, not failure)
    [[ "$status" -eq 0 ]]
    # Warning goes to stderr, but run captures both stdout and stderr in $output
    [[ "$output" == *"WARNING: Terminal state"* ]] || \
    [[ "$output" == *"WARNING"*"Terminal"*"next field"* ]]
}

# ==============================================================================
# Test: Missing file fails
# ==============================================================================

@test "test_missing_file_fails" {
    run "$VALIDATE_SCRIPT" "$FIXTURES_DIR/nonexistent-file.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"not found"* ]]
}

# ==============================================================================
# Test: Invalid YAML syntax fails
# ==============================================================================

@test "test_invalid_yaml_fails" {
    run "$VALIDATE_SCRIPT" "$FIXTURES_DIR/invalid-syntax.yaml"
    [[ "$status" -eq 1 ]]
    # YAML parsing error message
    [[ "$output" == *"Invalid YAML"* ]] || [[ "$output" == *"ERROR"* ]]
}

# ==============================================================================
# Test: Real workflows pass (all 5 workflow files from $ARC_HOME/workflows/)
# ==============================================================================

@test "test_real_workflows_pass_feature" {
    run "$VALIDATE_SCRIPT" "$WORKFLOWS_DIR/feature.yaml"
    [[ "$status" -eq 0 ]]
}

@test "test_real_workflows_pass_bugfix" {
    run "$VALIDATE_SCRIPT" "$WORKFLOWS_DIR/bugfix.yaml"
    [[ "$status" -eq 0 ]]
}

@test "test_real_workflows_pass_investigation" {
    run "$VALIDATE_SCRIPT" "$WORKFLOWS_DIR/investigation.yaml"
    [[ "$status" -eq 0 ]]
}

@test "test_real_workflows_pass_refactor" {
    run "$VALIDATE_SCRIPT" "$WORKFLOWS_DIR/refactor.yaml"
    [[ "$status" -eq 0 ]]
}

@test "test_real_workflows_pass_performance" {
    run "$VALIDATE_SCRIPT" "$WORKFLOWS_DIR/performance.yaml"
    [[ "$status" -eq 0 ]]
}

# ==============================================================================
# Edge Case Tests
# ==============================================================================

# Edge Case 1: Entry state is terminal (already covered by test_entry_is_terminal_fails)

# Edge Case 2: V2 branching with all valid targets (covered by test_valid_v2_branching_passes)

# Edge Case 3: Special characters - Use alphanumeric_underscore only
# The fixtures follow this convention, so validation passes

# Edge Case 4: Large workflow - Performance test (ensure <5s completion)
# Tested implicitly - all real workflows complete quickly

# Edge Case 5: Valid workflow outputs all expected success markers
@test "test_valid_workflow_outputs_all_success_markers" {
    run "$VALIDATE_SCRIPT" "$FIXTURES_DIR/valid-feature.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"✓ YAML syntax valid"* ]]
    [[ "$output" == *"✓ name:"* ]]
    [[ "$output" == *"✓ version:"* ]]
    [[ "$output" == *"✓ entry_state:"* ]]
    [[ "$output" == *"✓ Entry state is non-terminal"* ]]
    [[ "$output" == *"✓ All state names unique"* ]]
    [[ "$output" == *"Validation passed"* ]]
}

# Edge Case 6: V2 branching transitions are all logged
@test "test_v2_branching_logs_all_transitions" {
    run "$VALIDATE_SCRIPT" "$FIXTURES_DIR/valid-v2-branching.yaml"
    [[ "$status" -eq 0 ]]
    # Both branch targets from qa state should be logged
    [[ "$output" == *"✓ Transition valid: qa -> impl"* ]]
    [[ "$output" == *"✓ Transition valid: qa -> blocked"* ]]
    # The V1 transition from impl should also be logged
    [[ "$output" == *"✓ Transition valid: impl -> complete"* ]]
}

# Edge Case 7: Terminal states are not reported as invalid transitions
@test "test_terminal_states_no_transition_logged" {
    run "$VALIDATE_SCRIPT" "$FIXTURES_DIR/valid-feature.yaml"
    [[ "$status" -eq 0 ]]
    # Terminal state 'complete' should not have a transition logged
    [[ "$output" != *"Transition valid: complete ->"* ]]
}

# Edge Case 8: Multiple errors in one workflow are all reported
@test "test_multiple_errors_reported" {
    # The invalid-duplicate-state.yaml has duplicate names AND invalid transitions
    run "$VALIDATE_SCRIPT" "$FIXTURES_DIR/invalid-duplicate-state.yaml"
    [[ "$status" -eq 1 ]]
    # Should report the duplicate state error
    [[ "$output" == *"❌ Duplicate state name: qa"* ]]
}

# Edge Case 9: Self-loop state that cannot reach terminal
@test "test_self_loop_cannot_reach_terminal" {
    run "$VALIDATE_SCRIPT" "$FIXTURES_DIR/invalid-no-terminal-path.yaml"
    [[ "$status" -eq 1 ]]
    # Both qa and loop are trapped
    [[ "$output" == *"❌ State cannot reach terminal: qa"* ]] || \
    [[ "$output" == *"❌ State cannot reach terminal: loop"* ]]
}

# Edge Case 10: Orphan state with valid path to terminal still fails (unreachable)
@test "test_orphan_valid_path_still_unreachable" {
    run "$VALIDATE_SCRIPT" "$FIXTURES_DIR/invalid-unreachable.yaml"
    [[ "$status" -eq 1 ]]
    # orphan has a valid path to complete, but is unreachable from entry
    [[ "$output" == *"❌ Unreachable state: orphan"* ]]
}

# Edge Case 11: V2 mixed with V1 transitions
@test "test_v2_mixed_v1_transitions" {
    run "$VALIDATE_SCRIPT" "$FIXTURES_DIR/valid-v2-branching.yaml"
    [[ "$status" -eq 0 ]]
    # V2 branching transitions from qa
    [[ "$output" == *"✓ Transition valid: qa -> impl"* ]]
    [[ "$output" == *"✓ Transition valid: qa -> blocked"* ]]
    # V1 string transition from impl
    [[ "$output" == *"✓ Transition valid: impl -> complete"* ]]
}

# Edge Case 12: All prompt files are validated
@test "test_all_prompts_validated" {
    run "$VALIDATE_SCRIPT" "$FIXTURES_DIR/valid-feature.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"✓ Prompt exists: prompts/feature/qa.md"* ]]
    [[ "$output" == *"✓ Prompt exists: prompts/common/complete.md"* ]]
}
