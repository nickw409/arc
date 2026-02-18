#!/usr/bin/env bats

# Tests for workflow files and init-plan.sh --type flag

load 'test_helper'

setup() {
    SCRIPT_DIR="$(cd "$(dirname "$BATS_TEST_FILENAME")" && pwd)"
    ORCH_DIR="$(dirname "$SCRIPT_DIR")"
    PROJECT_ROOT="$(dirname "$ORCH_DIR")"
    WORKFLOWS_DIR="$ORCH_DIR/workflows"
    PROMPTS_DIR="$ORCH_DIR/prompts"
    VALIDATE="$ORCH_DIR/scripts/validate-workflow.sh"
    INIT_PLAN="$ORCH_DIR/scripts/init-plan.sh"
    GET_NEXT="$ORCH_DIR/scripts/get-next-state.sh"

    # Use temp dir so tests don't pollute .plans/active/
    export PLANS_DIR="$(mktemp -d)"
    ACTIVE_DIR="$PLANS_DIR/active"
    mkdir -p "$ACTIVE_DIR"
}

teardown() {
    rm -rf "$PLANS_DIR" 2>/dev/null || true
}

# Workflow file existence tests
@test "bugfix.yaml workflow exists" {
    [ -f "$WORKFLOWS_DIR/bugfix.yaml" ]
}

@test "investigation.yaml workflow exists" {
    [ -f "$WORKFLOWS_DIR/investigation.yaml" ]
}

@test "refactor.yaml workflow exists" {
    [ -f "$WORKFLOWS_DIR/refactor.yaml" ]
}

@test "performance.yaml workflow exists" {
    [ -f "$WORKFLOWS_DIR/performance.yaml" ]
}

# Workflow validation tests
@test "bugfix.yaml validates successfully" {
    run "$VALIDATE" "$WORKFLOWS_DIR/bugfix.yaml"
    [ "$status" -eq 0 ]
    [[ "$output" == *"Validation passed"* ]]
}

@test "investigation.yaml validates successfully" {
    run "$VALIDATE" "$WORKFLOWS_DIR/investigation.yaml"
    [ "$status" -eq 0 ]
    [[ "$output" == *"Validation passed"* ]]
}

@test "refactor.yaml validates successfully" {
    run "$VALIDATE" "$WORKFLOWS_DIR/refactor.yaml"
    [ "$status" -eq 0 ]
    [[ "$output" == *"Validation passed"* ]]
}

@test "performance.yaml validates successfully" {
    run "$VALIDATE" "$WORKFLOWS_DIR/performance.yaml"
    [ "$status" -eq 0 ]
    [[ "$output" == *"Validation passed"* ]]
}

# Bugfix workflow state transitions
@test "bugfix: investigate -> regression_tests" {
    run "$GET_NEXT" "$WORKFLOWS_DIR/bugfix.yaml" investigate
    [ "$status" -eq 0 ]
    [ "$output" == "regression_tests" ]
}

@test "bugfix: regression_tests -> test_review" {
    run "$GET_NEXT" "$WORKFLOWS_DIR/bugfix.yaml" regression_tests
    [ "$status" -eq 0 ]
    [ "$output" == "test_review" ]
}

@test "bugfix: fix_review -> complete" {
    run "$GET_NEXT" "$WORKFLOWS_DIR/bugfix.yaml" fix_review
    [ "$status" -eq 0 ]
    [ "$output" == "complete" ]
}

# Investigation workflow state transitions
@test "investigation: research -> draft" {
    run "$GET_NEXT" "$WORKFLOWS_DIR/investigation.yaml" research
    [ "$status" -eq 0 ]
    [ "$output" == "draft" ]
}

@test "investigation: draft -> review" {
    run "$GET_NEXT" "$WORKFLOWS_DIR/investigation.yaml" draft
    [ "$status" -eq 0 ]
    [ "$output" == "review" ]
}

@test "investigation: review -> complete" {
    run "$GET_NEXT" "$WORKFLOWS_DIR/investigation.yaml" review
    [ "$status" -eq 0 ]
    [ "$output" == "complete" ]
}

# Refactor workflow state transitions
@test "refactor: characterize -> char_review" {
    run "$GET_NEXT" "$WORKFLOWS_DIR/refactor.yaml" characterize
    [ "$status" -eq 0 ]
    [ "$output" == "char_review" ]
}

@test "refactor: refactor -> verify" {
    run "$GET_NEXT" "$WORKFLOWS_DIR/refactor.yaml" refactor
    [ "$status" -eq 0 ]
    [ "$output" == "verify" ]
}

# Performance workflow state transitions
@test "performance: baseline -> analyze" {
    run "$GET_NEXT" "$WORKFLOWS_DIR/performance.yaml" baseline
    [ "$status" -eq 0 ]
    [ "$output" == "analyze" ]
}

@test "performance: optimize -> benchmark" {
    run "$GET_NEXT" "$WORKFLOWS_DIR/performance.yaml" optimize
    [ "$status" -eq 0 ]
    [ "$output" == "benchmark" ]
}

# init-plan.sh --type flag tests
@test "init-plan.sh --type requires value" {
    run "$INIT_PLAN" --type
    [ "$status" -eq 1 ]
    [[ "$output" == *"requires a value"* ]]
}

@test "init-plan.sh --type invalid type errors" {
    run "$INIT_PLAN" --type invalid test-plan phase1
    [ "$status" -eq 1 ]
    [[ "$output" == *"Invalid workflow type"* ]]
    [[ "$output" == *"Valid types:"* ]]
}

@test "init-plan.sh --type bugfix creates workflow.yaml" {
    run "$INIT_PLAN" --type bugfix test-workflow-bugfix p1
    [ "$status" -eq 0 ]
    [ -f "$ACTIVE_DIR/test-workflow-bugfix/workflow.yaml" ]
}

@test "init-plan.sh --type bugfix sets workflow_type in state" {
    "$INIT_PLAN" --type bugfix test-workflow-state p1
    WORKFLOW_TYPE=$(jq -r '.workflow_type' "$ACTIVE_DIR/test-workflow-state/phases/p1/state.json")
    [ "$WORKFLOW_TYPE" == "bugfix" ]
}

@test "init-plan.sh default type is feature (no workflow.yaml copied)" {
    run "$INIT_PLAN" test-workflow-default p1
    [ "$status" -eq 0 ]
    # Feature workflow should NOT copy workflow.yaml (uses default)
    [ ! -f "$ACTIVE_DIR/test-workflow-default/workflow.yaml" ]
}

@test "init-plan.sh default sets workflow_type to feature" {
    "$INIT_PLAN" test-workflow-feat p1
    WORKFLOW_TYPE=$(jq -r '.workflow_type' "$ACTIVE_DIR/test-workflow-feat/phases/p1/state.json")
    [ "$WORKFLOW_TYPE" == "feature" ]
}

# Prompt file existence tests for new workflows
@test "bugfix prompts: all 5 files exist" {
    [ -f "$PROMPTS_DIR/bugfix/investigate.md" ]
    [ -f "$PROMPTS_DIR/bugfix/regression-tests.md" ]
    [ -f "$PROMPTS_DIR/bugfix/test-review.md" ]
    [ -f "$PROMPTS_DIR/bugfix/fix.md" ]
    [ -f "$PROMPTS_DIR/bugfix/fix-review.md" ]
}

@test "investigation prompts: all 3 files exist" {
    [ -f "$PROMPTS_DIR/investigation/research.md" ]
    [ -f "$PROMPTS_DIR/investigation/draft.md" ]
    [ -f "$PROMPTS_DIR/investigation/review.md" ]
}

@test "refactor prompts: all 4 files exist" {
    [ -f "$PROMPTS_DIR/refactor/characterize.md" ]
    [ -f "$PROMPTS_DIR/refactor/char-review.md" ]
    [ -f "$PROMPTS_DIR/refactor/refactor.md" ]
    [ -f "$PROMPTS_DIR/refactor/verify.md" ]
}

@test "performance prompts: all 4 files exist" {
    [ -f "$PROMPTS_DIR/performance/baseline.md" ]
    [ -f "$PROMPTS_DIR/performance/analyze.md" ]
    [ -f "$PROMPTS_DIR/performance/optimize.md" ]
    [ -f "$PROMPTS_DIR/performance/benchmark.md" ]
}

@test "common prompts: complete.md and blocked.md exist" {
    [ -f "$PROMPTS_DIR/common/complete.md" ]
    [ -f "$PROMPTS_DIR/common/blocked.md" ]
}
