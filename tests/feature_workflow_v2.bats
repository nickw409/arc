#!/usr/bin/env bats

# Tests for phase 04-workflow-updates (orchestration-v2)
# Tests that feature.yaml is correctly updated to V2 schema with:
# - verdicts arrays for qa_review and impl_review states
# - conditional next transitions for review states
# - preserved behavior for non-review states (linear V1-style)

setup() {
    load 'test_helper'
    setup_temp_dir
}

teardown() {
    teardown_temp_dir
}

# ==============================================================================
# Test Case: test_feature_workflow_validates
# Validates that the updated feature.yaml passes all workflow validation checks
# ==============================================================================
@test "feature.yaml: validates with V2 schema (exit code 0)" {
    run "$SCRIPTS_DIR/validate-workflow.sh" "$WORKFLOWS_DIR/feature.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Validation passed"* ]]
}

@test "feature.yaml: verdict consistency is valid" {
    run "$SCRIPTS_DIR/validate-workflow.sh" "$WORKFLOWS_DIR/feature.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Verdict consistency valid"* ]]
}

# ==============================================================================
# Test Case: test_feature_qa_review_branches
# qa_review with approved verdict should transition to impl
# ==============================================================================
@test "feature.yaml: qa_review + approved -> impl" {
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" "qa_review" "approved"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "impl" ]]
}

# ==============================================================================
# Test Case: test_feature_qa_review_loops_on_gaps
# qa_review with gaps_found verdict should loop back to qa
# ==============================================================================
@test "feature.yaml: qa_review + gaps_found -> qa" {
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" "qa_review" "gaps_found"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "qa" ]]
}

# ==============================================================================
# Test Case: test_feature_impl_review_completes
# impl_review with approved verdict should transition to complete
# ==============================================================================
@test "feature.yaml: impl_review + approved -> complete" {
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" "impl_review" "approved"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "complete" ]]
}

# ==============================================================================
# Test Case: test_feature_impl_review_loops_on_concerns
# impl_review with concerns verdict should loop back to impl
# ==============================================================================
@test "feature.yaml: impl_review + concerns -> impl" {
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" "impl_review" "concerns"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "impl" ]]
}

# ==============================================================================
# Test Case: test_feature_has_blocked_terminal
# blocked should be in terminal_states
# ==============================================================================
@test "feature.yaml: blocked is in terminal_states" {
    run yq '.terminal_states | contains(["blocked"])' "$WORKFLOWS_DIR/feature.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "true" ]]
}

@test "feature.yaml: complete is in terminal_states" {
    run yq '.terminal_states | contains(["complete"])' "$WORKFLOWS_DIR/feature.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "true" ]]
}

# ==============================================================================
# Test Case: test_get_next_state_invalid_verdict
# Invalid verdict should fail with exit code 1
# ==============================================================================
@test "feature.yaml: qa_review + invalid_verdict fails (exit 1)" {
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" "qa_review" "invalid_verdict"
    [[ "$status" -eq 1 ]]
}

@test "feature.yaml: impl_review + invalid_verdict fails (exit 1)" {
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" "impl_review" "invalid_verdict"
    [[ "$status" -eq 1 ]]
}

# ==============================================================================
# Test Case: test_get_next_state_case_sensitivity
# Verdicts are case-sensitive - APPROVED should fail
# ==============================================================================
@test "feature.yaml: qa_review + APPROVED fails (case sensitive)" {
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" "qa_review" "APPROVED"
    [[ "$status" -eq 1 ]]
}

@test "feature.yaml: impl_review + APPROVED fails (case sensitive)" {
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" "impl_review" "APPROVED"
    [[ "$status" -eq 1 ]]
}

@test "feature.yaml: qa_review + Approved fails (mixed case)" {
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" "qa_review" "Approved"
    [[ "$status" -eq 1 ]]
}

# ==============================================================================
# Non-review states remain V1-style (linear next)
# ==============================================================================
@test "feature.yaml: qa -> qa_review (V1-style linear)" {
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" "qa"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "qa_review" ]]
}

@test "feature.yaml: impl -> impl_review (V1-style linear)" {
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" "impl"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "impl_review" ]]
}

# ==============================================================================
# Terminal states
# ==============================================================================
@test "feature.yaml: complete is TERMINAL" {
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" "complete"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "TERMINAL" ]]
}

@test "feature.yaml: blocked is TERMINAL" {
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" "blocked"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "TERMINAL" ]]
}

# ==============================================================================
# Version check - should be version 2
# ==============================================================================
@test "feature.yaml: version is 2" {
    run yq '.version' "$WORKFLOWS_DIR/feature.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "2" ]]
}

# ==============================================================================
# Workflow name unchanged
# ==============================================================================
@test "feature.yaml: name is still 'feature'" {
    run yq '.name' "$WORKFLOWS_DIR/feature.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "feature" ]]
}

# ==============================================================================
# Entry state unchanged
# ==============================================================================
@test "feature.yaml: entry_state is still 'qa'" {
    run yq '.entry_state' "$WORKFLOWS_DIR/feature.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "qa" ]]
}

# ==============================================================================
# Verdicts arrays defined correctly for review states
# ==============================================================================
@test "feature.yaml: qa_review has verdicts array" {
    run yq '.states[] | select(.name == "qa_review") | .verdicts | length' "$WORKFLOWS_DIR/feature.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" -gt 0 ]]
}

@test "feature.yaml: qa_review verdicts include 'approved'" {
    run yq '.states[] | select(.name == "qa_review") | .verdicts | contains(["approved"])' "$WORKFLOWS_DIR/feature.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "true" ]]
}

@test "feature.yaml: qa_review verdicts include 'gaps_found'" {
    run yq '.states[] | select(.name == "qa_review") | .verdicts | contains(["gaps_found"])' "$WORKFLOWS_DIR/feature.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "true" ]]
}

@test "feature.yaml: impl_review has verdicts array" {
    run yq '.states[] | select(.name == "impl_review") | .verdicts | length' "$WORKFLOWS_DIR/feature.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" -gt 0 ]]
}

@test "feature.yaml: impl_review verdicts include 'approved'" {
    run yq '.states[] | select(.name == "impl_review") | .verdicts | contains(["approved"])' "$WORKFLOWS_DIR/feature.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "true" ]]
}

@test "feature.yaml: impl_review verdicts include 'concerns'" {
    run yq '.states[] | select(.name == "impl_review") | .verdicts | contains(["concerns"])' "$WORKFLOWS_DIR/feature.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "true" ]]
}

# ==============================================================================
# Non-review states should NOT have verdicts
# ==============================================================================
@test "feature.yaml: qa state does not have verdicts" {
    run yq '.states[] | select(.name == "qa") | .verdicts // "null"' "$WORKFLOWS_DIR/feature.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "null" ]]
}

@test "feature.yaml: impl state does not have verdicts" {
    run yq '.states[] | select(.name == "impl") | .verdicts // "null"' "$WORKFLOWS_DIR/feature.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "null" ]]
}

# ==============================================================================
# Prompt paths should remain unchanged
# ==============================================================================
@test "feature.yaml: qa_review prompt path unchanged" {
    run yq '.states[] | select(.name == "qa_review") | .prompt' "$WORKFLOWS_DIR/feature.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "prompts/feature/qa-review.md" ]]
}

@test "feature.yaml: impl_review prompt path unchanged" {
    run yq '.states[] | select(.name == "impl_review") | .prompt' "$WORKFLOWS_DIR/feature.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "prompts/feature/impl-review.md" ]]
}

# ==============================================================================
# Graph connectivity - all loops can escape to terminal
# ==============================================================================
@test "feature.yaml: qa_review approved path reaches terminal" {
    # qa_review -> impl -> impl_review -> complete (with approved)
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" "qa_review" "approved"
    [[ "$status" -eq 0 ]]
    local next="$output"
    [[ "$next" == "impl" ]]

    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" "impl"
    [[ "$status" -eq 0 ]]
    next="$output"
    [[ "$next" == "impl_review" ]]

    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" "impl_review" "approved"
    [[ "$status" -eq 0 ]]
    next="$output"
    [[ "$next" == "complete" ]]

    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" "complete"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "TERMINAL" ]]
}

@test "feature.yaml: qa_review gaps_found path can eventually reach terminal" {
    # qa_review(gaps_found) -> qa -> qa_review -> ... (eventually approved) -> terminal
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" "qa_review" "gaps_found"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "qa" ]]

    # qa -> qa_review (can then be approved)
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" "qa"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "qa_review" ]]
}

@test "feature.yaml: impl_review concerns path can eventually reach terminal" {
    # impl_review(concerns) -> impl -> impl_review -> ... (eventually approved) -> terminal
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" "impl_review" "concerns"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "impl" ]]

    # impl -> impl_review (can then be approved)
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" "impl"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "impl_review" ]]
}

# ==============================================================================
# Edge Case: Missing verdict for V2 branching state should fail
# ==============================================================================
@test "feature.yaml: qa_review without verdict fails (V2 requires verdict)" {
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" "qa_review"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"conditional transitions"* ]] || [[ "$output" == *"verdict"* ]]
}

@test "feature.yaml: impl_review without verdict fails (V2 requires verdict)" {
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" "impl_review"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"conditional transitions"* ]] || [[ "$output" == *"verdict"* ]]
}

# ==============================================================================
# Edge Case: Verify exact verdict count (no extra verdicts added)
# ==============================================================================
@test "feature.yaml: qa_review has exactly 2 verdicts" {
    run yq '.states[] | select(.name == "qa_review") | .verdicts | length' "$WORKFLOWS_DIR/feature.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "2" ]]
}

@test "feature.yaml: impl_review has exactly 2 verdicts" {
    run yq '.states[] | select(.name == "impl_review") | .verdicts | length' "$WORKFLOWS_DIR/feature.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "2" ]]
}

# ==============================================================================
# Edge Case: Verify transitions match verdicts exactly (no extra transitions)
# ==============================================================================
@test "feature.yaml: qa_review next has exactly 2 transitions" {
    run yq '.states[] | select(.name == "qa_review") | .next | keys | length' "$WORKFLOWS_DIR/feature.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "2" ]]
}

@test "feature.yaml: impl_review next has exactly 2 transitions" {
    run yq '.states[] | select(.name == "impl_review") | .next | keys | length' "$WORKFLOWS_DIR/feature.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "2" ]]
}

# ==============================================================================
# Validate all 6 states are present
# ==============================================================================
@test "feature.yaml: has 6 states defined" {
    run yq '.states | length' "$WORKFLOWS_DIR/feature.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "6" ]]
}

@test "feature.yaml: all expected states exist" {
    local expected_states=("qa" "qa_review" "impl" "impl_review" "complete" "blocked")
    for state in "${expected_states[@]}"; do
        run yq ".states[] | select(.name == \"$state\") | .name" "$WORKFLOWS_DIR/feature.yaml"
        [[ "$status" -eq 0 ]]
        [[ "$output" == "$state" ]]
    done
}
