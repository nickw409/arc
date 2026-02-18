#!/usr/bin/env bats

# Integration tests for V1b orchestration system
# Tests cross-phase functionality and end-to-end workflows

load 'test_helper'

setup() {
    SCRIPT_DIR="$(cd "$(dirname "$BATS_TEST_FILENAME")" && pwd)"
    ORCH_DIR="$(dirname "$SCRIPT_DIR")"
    PROJECT_ROOT="$(dirname "$ORCH_DIR")"
    WORKFLOWS_DIR="$ORCH_DIR/workflows"
    PROMPTS_DIR="$ORCH_DIR/prompts"
    SCRIPTS_DIR="$ORCH_DIR/scripts"

    # Use temp dir so tests don't pollute .plans/active/
    export PLANS_DIR="$(mktemp -d)"
    ACTIVE_DIR="$PLANS_DIR/active"
    mkdir -p "$ACTIVE_DIR"

    TEST_PLAN="integration-test-$$"
}

teardown() {
    rm -rf "$PLANS_DIR" 2>/dev/null || true
}

# =============================================================================
# End-to-end workflow creation tests
# =============================================================================

@test "e2e: create feature plan and verify structure" {
    # Create plan
    run "$SCRIPTS_DIR/init-plan.sh" "$TEST_PLAN" phase1 phase2
    [ "$status" -eq 0 ]

    # Verify structure
    [ -d "$ACTIVE_DIR/$TEST_PLAN" ]
    [ -f "$ACTIVE_DIR/$TEST_PLAN/plan.json" ]
    [ -f "$ACTIVE_DIR/$TEST_PLAN/session_id" ]
    [ -d "$ACTIVE_DIR/$TEST_PLAN/phases/phase1" ]
    [ -d "$ACTIVE_DIR/$TEST_PLAN/phases/phase2" ]
    [ -d "$ACTIVE_DIR/$TEST_PLAN/phases/integration" ]

    # Verify state.json has correct workflow_type
    WORKFLOW_TYPE=$(jq -r '.workflow_type' "$ACTIVE_DIR/$TEST_PLAN/phases/phase1/state.json")
    [ "$WORKFLOW_TYPE" == "feature" ]
}

@test "e2e: create bugfix plan and verify workflow copied" {
    run "$SCRIPTS_DIR/init-plan.sh" --type bugfix "$TEST_PLAN" investigate
    [ "$status" -eq 0 ]

    # Verify workflow.yaml was copied
    [ -f "$ACTIVE_DIR/$TEST_PLAN/workflow.yaml" ]

    # Verify content matches bugfix workflow
    run diff "$WORKFLOWS_DIR/bugfix.yaml" "$ACTIVE_DIR/$TEST_PLAN/workflow.yaml"
    [ "$status" -eq 0 ]
}

@test "e2e: status.sh shows correct plan state" {
    "$SCRIPTS_DIR/init-plan.sh" "$TEST_PLAN" p1
    run "$SCRIPTS_DIR/status.sh" "$TEST_PLAN"
    [ "$status" -eq 0 ]
    [[ "$output" == *"$TEST_PLAN"* ]]
    [[ "$output" == *"p1"* ]]
    [[ "$output" == *"pending"* ]]
}

# =============================================================================
# Prompt loading integration tests
# =============================================================================

@test "integration: load-prompt.sh works with feature/qa.md template" {
    run "$SCRIPTS_DIR/load-prompt.sh" prompts/feature/qa.md \
        plan="test" \
        phase="p1" \
        plan_file="/tmp/plan.md" \
        state_file="/tmp/state.json" \
        phase_dir="/tmp/phase" \
        qa_review_instruction=""
    [ "$status" -eq 0 ]
    [[ "$output" == *"QA"* ]]
    [[ "$output" == *"p1"* ]]
    [[ "$output" == *"plan: **test**"* ]]
}

@test "integration: load-prompt.sh works with feature/impl.md template" {
    run "$SCRIPTS_DIR/load-prompt.sh" prompts/feature/impl.md \
        plan="test" \
        phase="p1" \
        iteration="3" \
        max_iterations="25" \
        state_file="/tmp/state.json" \
        plan_file="/tmp/plan.md" \
        phase_dir="/tmp/phase" \
        scripts_dir="/tmp/scripts" \
        crates="test-package" \
        orchestrator_section="" \
        dispute_section="" \
        escalation_section=""
    [ "$status" -eq 0 ]
    [[ "$output" == *"Implementation"* ]]
    [[ "$output" == *"p1"* ]]
    [[ "$output" == *"iteration 3"* ]]
}

@test "integration: load-prompt.sh handles multiline env var substitution" {
    CUSTOM="This is a multiline value.

Second line here."

    run bash -c "PROMPT_VAR_custom_var='$CUSTOM' '$SCRIPTS_DIR/load-prompt.sh' prompts/feature/impl.md \
        plan=test phase=p1 iteration=1 \
        state_file=/tmp/s.json plan_file=/tmp/p.md phase_dir=/tmp \
        scripts_dir=/tmp crates=core"
    [ "$status" -eq 0 ]
    # load-prompt.sh renders simple {{var}} placeholders; Handlebars constructs pass through
    [[ "$output" == *"Implementation - p1"* ]]
    [[ "$output" == *"plan: **test**"* ]]
}

# =============================================================================
# Workflow state machine integration tests
# =============================================================================

@test "integration: full feature workflow state walk" {
    # Walk through the entire feature workflow (happy path: all reviews approved)
    # V2 branching states (qa_review, impl_review) require a verdict argument
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" "qa"
    [ "$status" -eq 0 ] && [ "$output" == "qa_review" ]

    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" "qa_review" "approved"
    [ "$status" -eq 0 ] && [ "$output" == "impl" ]

    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" "impl"
    [ "$status" -eq 0 ] && [ "$output" == "impl_review" ]

    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" "impl_review" "approved"
    [ "$status" -eq 0 ] && [ "$output" == "complete" ]
}

@test "integration: full bugfix workflow state walk" {
    states=("investigate" "regression_tests" "test_review" "fix" "fix_review" "complete")
    for ((i=0; i<${#states[@]}-1; i++)); do
        current="${states[$i]}"
        expected="${states[$((i+1))]}"
        run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/bugfix.yaml" "$current"
        [ "$status" -eq 0 ]
        [ "$output" == "$expected" ]
    done
}

@test "integration: full investigation workflow state walk" {
    states=("research" "draft" "review" "complete")
    for ((i=0; i<${#states[@]}-1; i++)); do
        current="${states[$i]}"
        expected="${states[$((i+1))]}"
        run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/investigation.yaml" "$current"
        [ "$status" -eq 0 ]
        [ "$output" == "$expected" ]
    done
}

@test "integration: full refactor workflow state walk" {
    states=("characterize" "char_review" "refactor" "verify" "complete")
    for ((i=0; i<${#states[@]}-1; i++)); do
        current="${states[$i]}"
        expected="${states[$((i+1))]}"
        run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/refactor.yaml" "$current"
        [ "$status" -eq 0 ]
        [ "$output" == "$expected" ]
    done
}

@test "integration: full performance workflow state walk" {
    states=("baseline" "analyze" "optimize" "benchmark" "complete")
    for ((i=0; i<${#states[@]}-1; i++)); do
        current="${states[$i]}"
        expected="${states[$((i+1))]}"
        run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/performance.yaml" "$current"
        [ "$status" -eq 0 ]
        [ "$output" == "$expected" ]
    done
}

# =============================================================================
# State update integration tests
# =============================================================================

@test "integration: update-state.sh status changes persist" {
    "$SCRIPTS_DIR/init-plan.sh" "$TEST_PLAN" p1

    # Change status
    run "$SCRIPTS_DIR/update-state.sh" "$TEST_PLAN" p1 status implementing
    [ "$status" -eq 0 ]

    # Verify persisted
    STATUS=$(jq -r '.phase_status' "$ACTIVE_DIR/$TEST_PLAN/phases/p1/state.json")
    [ "$STATUS" == "implementing" ]
}

@test "integration: update-state.sh increment-iteration persists" {
    "$SCRIPTS_DIR/init-plan.sh" "$TEST_PLAN" p1

    # Increment
    run "$SCRIPTS_DIR/update-state.sh" "$TEST_PLAN" p1 increment-iteration
    [ "$status" -eq 0 ]

    # Verify
    ITER=$(jq -r '.iteration.current' "$ACTIVE_DIR/$TEST_PLAN/phases/p1/state.json")
    [ "$ITER" == "1" ]
}

@test "integration: update-state.sh tests command updates counts" {
    "$SCRIPTS_DIR/init-plan.sh" "$TEST_PLAN" p1

    # Update test counts
    run "$SCRIPTS_DIR/update-state.sh" "$TEST_PLAN" p1 tests 5 10
    [ "$status" -eq 0 ]

    # Verify
    PASSING=$(jq -r '.tests_passing' "$ACTIVE_DIR/$TEST_PLAN/phases/p1/state.json")
    TOTAL=$(jq -r '.tests_total' "$ACTIVE_DIR/$TEST_PLAN/phases/p1/state.json")
    [ "$PASSING" == "5" ]
    [ "$TOTAL" == "10" ]
}

# =============================================================================
# Cross-component integration tests
# =============================================================================

@test "integration: iterate.sh workflow detection with custom workflow" {
    # Create bugfix plan
    "$SCRIPTS_DIR/init-plan.sh" --type bugfix "$TEST_PLAN" investigate

    # Source iterate.sh's detect_workflow function and test it
    # We test this indirectly by checking the workflow.yaml was copied
    [ -f "$ACTIVE_DIR/$TEST_PLAN/workflow.yaml" ]

    # The detect_workflow function should find the plan-specific workflow
    WORKFLOW_NAME=$(yq -r '.name' "$ACTIVE_DIR/$TEST_PLAN/workflow.yaml")
    [ "$WORKFLOW_NAME" == "bugfix" ]
}

@test "integration: all prompt templates have required variables" {
    # Check that each prompt template can be loaded without errors
    # when provided standard variables
    templates=(
        "prompts/feature/qa.md"
        "prompts/feature/impl.md"
        "prompts/feature/fix.md"
        "prompts/feature/qa-review.md"
        "prompts/feature/impl-review.md"
        "prompts/bugfix/investigate.md"
        "prompts/bugfix/regression-tests.md"
        "prompts/investigation/research.md"
        "prompts/refactor/characterize.md"
        "prompts/performance/baseline.md"
    )

    for template in "${templates[@]}"; do
        run "$SCRIPTS_DIR/load-prompt.sh" "$template" \
            plan_name=test phase=p1 plan_file=/tmp/p.md state_file=/tmp/s.json \
            phase_dir=/tmp scripts_dir=/tmp iteration=1 max_iterations=25 \
            crates=core qa_review_instruction="" orchestrator_section="" \
            dispute_section="" escalation_section="" dispute_count=0 \
            dispute_list=""
        [ "$status" -eq 0 ]
    done
}

@test "integration: run-orchestrator.sh is non-interactive" {
    # Verify run-orchestrator.sh uses --print flag (non-interactive, stdin piping)
    grep -q "claude.*--print" "$SCRIPTS_DIR/run-orchestrator.sh"
}
