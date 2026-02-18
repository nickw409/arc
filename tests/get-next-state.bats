#!/usr/bin/env bats

# Tests for get-next-state.sh

setup() {
    load 'test_helper'
    setup_temp_dir
}

teardown() {
    teardown_temp_dir
}

@test "get-next-state.sh exists and is executable" {
    [[ -x "$SCRIPTS_DIR/get-next-state.sh" ]]
}

@test "script is syntactically valid bash" {
    run bash -n "$SCRIPTS_DIR/get-next-state.sh"
    [[ "$status" -eq 0 ]]
}

@test "shows usage with no arguments" {
    run "$SCRIPTS_DIR/get-next-state.sh"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Usage"* ]]
}

@test "shows usage with only one argument" {
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Usage"* ]]
}

@test "fails when workflow file does not exist" {
    run "$SCRIPTS_DIR/get-next-state.sh" "$TEST_TEMP_DIR/nonexistent.yaml" "start"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"not found"* ]]
}

@test "returns next state for valid transition" {
    local workflow=$(create_test_workflow)
    run "$SCRIPTS_DIR/get-next-state.sh" "$workflow" "start"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "middle" ]]
}

@test "returns TERMINAL for terminal state" {
    local workflow=$(create_test_workflow)
    run "$SCRIPTS_DIR/get-next-state.sh" "$workflow" "end"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "TERMINAL" ]]
}

@test "returns TERMINAL for blocked state" {
    local workflow=$(create_test_workflow)
    run "$SCRIPTS_DIR/get-next-state.sh" "$workflow" "blocked"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "TERMINAL" ]]
}

@test "fails for nonexistent state" {
    local workflow=$(create_test_workflow)
    run "$SCRIPTS_DIR/get-next-state.sh" "$workflow" "nonexistent"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"not found"* ]]
}

# Tests using the bundled feature.yaml workflow
@test "feature.yaml: qa -> qa_review" {
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" "qa"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "qa_review" ]]
}

@test "feature.yaml: qa_review -> impl" {
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" "qa_review"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "impl" ]]
}

@test "feature.yaml: impl -> impl_review" {
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" "impl"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "impl_review" ]]
}

@test "feature.yaml: impl_review -> complete" {
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" "impl_review"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "complete" ]]
}

@test "feature.yaml: complete is terminal" {
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" "complete"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "TERMINAL" ]]
}

@test "feature.yaml: blocked is terminal" {
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" "blocked"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "TERMINAL" ]]
}

@test "handles middle-of-chain transitions" {
    local workflow=$(create_test_workflow)
    run "$SCRIPTS_DIR/get-next-state.sh" "$workflow" "middle"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "end" ]]
}

@test "ignores verdict argument in V1 (linear only)" {
    local workflow=$(create_test_workflow)
    # Verdict is passed but should be ignored for linear workflows
    run "$SCRIPTS_DIR/get-next-state.sh" "$workflow" "start" "approved"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "middle" ]]
}
