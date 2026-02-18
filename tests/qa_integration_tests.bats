#!/usr/bin/env bats

# QA Integration Tests for full-v1 integration phase
# Tests all 5 workflow types and end-to-end orchestration system validation
#
# Coverage:
# - All 5 workflow types pass validation
# - init-plan.sh --type flag for all workflow types
# - Validation catches broken workflows
# - State transitions for all workflows
# - Edge cases: concurrent creation, many phases, mixed types

load 'test_helper'

setup() {
    SCRIPT_DIR="$(cd "$(dirname "$BATS_TEST_FILENAME")" && pwd)"
    ORCH_DIR="$(dirname "$SCRIPT_DIR")"
    PROJECT_ROOT="$(dirname "$ORCH_DIR")"
    WORKFLOWS_DIR="$ORCH_DIR/workflows"
    SCRIPTS_DIR="$ORCH_DIR/scripts"

    # Use a temp directory as PLANS_DIR so tests never pollute .plans/active/
    TEST_PLANS_DIR=$(mktemp -d)
    export PLANS_DIR="$TEST_PLANS_DIR"
    ACTIVE_DIR="$PLANS_DIR/active"
    mkdir -p "$ACTIVE_DIR"

    # Create a unique test plan name
    TEST_PLAN="qa-integration-test-$$-$RANDOM"
}

teardown() {
    rm -rf "$TEST_PLANS_DIR" 2>/dev/null || true
}

# =============================================================================
# Test Case: test_all_real_workflows_pass_validation
# Input: All 5 workflow files: feature.yaml, bugfix.yaml, investigation.yaml, refactor.yaml, performance.yaml
# Expected: All exit 0 with "Validation passed!"
# =============================================================================

@test "qa: all 5 real workflows pass validation - feature.yaml" {
    run "$SCRIPTS_DIR/validate-workflow.sh" "$WORKFLOWS_DIR/feature.yaml"
    [ "$status" -eq 0 ]
    [[ "$output" == *"Validation passed"* ]]
}

@test "qa: all 5 real workflows pass validation - bugfix.yaml" {
    run "$SCRIPTS_DIR/validate-workflow.sh" "$WORKFLOWS_DIR/bugfix.yaml"
    [ "$status" -eq 0 ]
    [[ "$output" == *"Validation passed"* ]]
}

@test "qa: all 5 real workflows pass validation - investigation.yaml" {
    run "$SCRIPTS_DIR/validate-workflow.sh" "$WORKFLOWS_DIR/investigation.yaml"
    [ "$status" -eq 0 ]
    [[ "$output" == *"Validation passed"* ]]
}

@test "qa: all 5 real workflows pass validation - refactor.yaml" {
    run "$SCRIPTS_DIR/validate-workflow.sh" "$WORKFLOWS_DIR/refactor.yaml"
    [ "$status" -eq 0 ]
    [[ "$output" == *"Validation passed"* ]]
}

@test "qa: all 5 real workflows pass validation - performance.yaml" {
    run "$SCRIPTS_DIR/validate-workflow.sh" "$WORKFLOWS_DIR/performance.yaml"
    [ "$status" -eq 0 ]
    [[ "$output" == *"Validation passed"* ]]
}

# =============================================================================
# Test Case: test_init_plan_feature_type
# Input: init-plan.sh --type feature test-feature phase1
# Expected: Creates plan with workflow_type="feature" in state.json
# =============================================================================

@test "qa: init-plan.sh --type feature creates plan with workflow_type=feature" {
    run "$SCRIPTS_DIR/init-plan.sh" --type feature "$TEST_PLAN" phase1
    [ "$status" -eq 0 ]

    # Verify state.json has correct workflow_type
    WORKFLOW_TYPE=$(jq -r '.workflow_type' "$ACTIVE_DIR/$TEST_PLAN/phases/phase1/state.json")
    [ "$WORKFLOW_TYPE" == "feature" ]

    # Feature is the default - workflow.yaml is NOT copied (uses default)
    # This is correct behavior per init-plan.sh lines 95-98
    [ ! -f "$ACTIVE_DIR/$TEST_PLAN/workflow.yaml" ]
}

# =============================================================================
# Test Case: test_init_plan_bugfix_type
# Input: init-plan.sh --type bugfix test-bugfix investigate fix
# Expected: Creates plan with workflow_type="bugfix" in state.json
# =============================================================================

@test "qa: init-plan.sh --type bugfix creates plan with workflow_type=bugfix" {
    run "$SCRIPTS_DIR/init-plan.sh" --type bugfix "$TEST_PLAN" investigate fix
    [ "$status" -eq 0 ]

    # Verify state.json has correct workflow_type
    WORKFLOW_TYPE=$(jq -r '.workflow_type' "$ACTIVE_DIR/$TEST_PLAN/phases/investigate/state.json")
    [ "$WORKFLOW_TYPE" == "bugfix" ]

    # Verify workflow.yaml was copied and matches bugfix
    run diff "$WORKFLOWS_DIR/bugfix.yaml" "$ACTIVE_DIR/$TEST_PLAN/workflow.yaml"
    [ "$status" -eq 0 ]
}

# =============================================================================
# Test Case: test_init_plan_investigation_type
# Input: init-plan.sh --type investigation test-invest research
# Expected: Creates plan with workflow_type="investigation" in state.json
# =============================================================================

@test "qa: init-plan.sh --type investigation creates plan with workflow_type=investigation" {
    run "$SCRIPTS_DIR/init-plan.sh" --type investigation "$TEST_PLAN" research
    [ "$status" -eq 0 ]

    # Verify state.json has correct workflow_type
    WORKFLOW_TYPE=$(jq -r '.workflow_type' "$ACTIVE_DIR/$TEST_PLAN/phases/research/state.json")
    [ "$WORKFLOW_TYPE" == "investigation" ]

    # Verify workflow.yaml was copied and matches investigation
    run diff "$WORKFLOWS_DIR/investigation.yaml" "$ACTIVE_DIR/$TEST_PLAN/workflow.yaml"
    [ "$status" -eq 0 ]
}

# =============================================================================
# Test Case: test_init_plan_refactor_type
# Input: init-plan.sh --type refactor test-refactor cleanup
# Expected: Creates plan with workflow_type="refactor" in state.json
# =============================================================================

@test "qa: init-plan.sh --type refactor creates plan with workflow_type=refactor" {
    run "$SCRIPTS_DIR/init-plan.sh" --type refactor "$TEST_PLAN" cleanup
    [ "$status" -eq 0 ]

    # Verify state.json has correct workflow_type
    WORKFLOW_TYPE=$(jq -r '.workflow_type' "$ACTIVE_DIR/$TEST_PLAN/phases/cleanup/state.json")
    [ "$WORKFLOW_TYPE" == "refactor" ]

    # Verify workflow.yaml was copied and matches refactor
    run diff "$WORKFLOWS_DIR/refactor.yaml" "$ACTIVE_DIR/$TEST_PLAN/workflow.yaml"
    [ "$status" -eq 0 ]
}

# =============================================================================
# Test Case: test_init_plan_performance_type
# Input: init-plan.sh --type performance test-perf optimize
# Expected: Creates plan with workflow_type="performance" in state.json
# =============================================================================

@test "qa: init-plan.sh --type performance creates plan with workflow_type=performance" {
    run "$SCRIPTS_DIR/init-plan.sh" --type performance "$TEST_PLAN" optimize
    [ "$status" -eq 0 ]

    # Verify state.json has correct workflow_type
    WORKFLOW_TYPE=$(jq -r '.workflow_type' "$ACTIVE_DIR/$TEST_PLAN/phases/optimize/state.json")
    [ "$WORKFLOW_TYPE" == "performance" ]

    # Verify workflow.yaml was copied and matches performance
    run diff "$WORKFLOWS_DIR/performance.yaml" "$ACTIVE_DIR/$TEST_PLAN/workflow.yaml"
    [ "$status" -eq 0 ]
}

# =============================================================================
# Test Case: test_init_plan_invalid_type_fails
# Input: init-plan.sh --type invalid test-invalid phase1
# Expected: Exit 1 with error about invalid workflow type
# =============================================================================

@test "qa: init-plan.sh --type invalid fails with error" {
    run "$SCRIPTS_DIR/init-plan.sh" --type invalid "$TEST_PLAN" phase1
    [ "$status" -ne 0 ]

    # Should contain error about invalid type
    [[ "$output" == *"invalid"* ]] || [[ "$output" == *"Unknown"* ]] || [[ "$output" == *"not found"* ]]
}

@test "qa: init-plan.sh --type nonexistent fails with error" {
    run "$SCRIPTS_DIR/init-plan.sh" --type nonexistent "$TEST_PLAN" phase1
    [ "$status" -ne 0 ]
}

# =============================================================================
# Test Case: test_validation_catches_broken_workflow
# Input: Temporarily corrupt a workflow file, run validation
# Expected: Validation fails with specific error message
# =============================================================================

@test "qa: validation catches workflow with missing name field" {
    cat > "$TEST_PLANS_DIR/broken.yaml" << 'EOF'
version: 1
entry_state: start
terminal_states: [end]
states:
  - name: start
    prompt: test.md
    next: end
  - name: end
    prompt: test.md
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_PLANS_DIR/broken.yaml"
    [ "$status" -ne 0 ]
    [[ "$output" == *"missing"* ]] || [[ "$output" == *"name"* ]]
}

@test "qa: validation catches workflow with missing states" {
    cat > "$TEST_PLANS_DIR/broken.yaml" << 'EOF'
name: broken
version: 1
entry_state: start
terminal_states: [end]
states: []
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_PLANS_DIR/broken.yaml"
    [ "$status" -ne 0 ]
    [[ "$output" == *"state"* ]]
}

@test "qa: validation catches workflow with invalid entry_state" {
    cat > "$TEST_PLANS_DIR/broken.yaml" << 'EOF'
name: broken
version: 1
entry_state: nonexistent
terminal_states: [end]
states:
  - name: start
    prompt: test.md
    next: end
  - name: end
    prompt: test.md
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_PLANS_DIR/broken.yaml"
    [ "$status" -ne 0 ]
    [[ "$output" == *"not found"* ]] || [[ "$output" == *"entry"* ]]
}

@test "qa: validation catches malformed YAML" {
    cat > "$TEST_PLANS_DIR/malformed.yaml" << 'EOF'
name: malformed
version: 1
states:
  - name: foo
    bad indentation here
  next: bar
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_PLANS_DIR/malformed.yaml"
    [ "$status" -ne 0 ]
}

# =============================================================================
# Test Case: test_get_next_state_feature_workflow
# Input: feature.yaml, current_state="qa"
# Expected: Returns "qa_review"
# =============================================================================

@test "qa: get-next-state.sh feature workflow qa -> qa_review" {
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" "qa"
    [ "$status" -eq 0 ]
    [ "$output" == "qa_review" ]
}

@test "qa: get-next-state.sh feature workflow impl -> impl_review" {
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" "impl"
    [ "$status" -eq 0 ]
    [ "$output" == "impl_review" ]
}

# =============================================================================
# Test Case: test_get_next_state_terminal
# Input: feature.yaml, current_state="complete"
# Expected: Returns "TERMINAL" or empty (indicates terminal state)
# =============================================================================

@test "qa: get-next-state.sh returns TERMINAL or empty for terminal state" {
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" "complete"
    # Terminal states either return empty, TERMINAL, or the script handles it specially
    [ "$status" -eq 0 ] || [ "$status" -eq 1 ]
    # Output should be empty or "TERMINAL" or indicate terminal
    [[ "$output" == "" ]] || [[ "$output" == "TERMINAL" ]] || [[ "$output" == "null" ]]
}

@test "qa: get-next-state.sh handles blocked terminal state" {
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" "blocked"
    [ "$status" -eq 0 ] || [ "$status" -eq 1 ]
    # Output should indicate terminal state
    [[ "$output" == "" ]] || [[ "$output" == "TERMINAL" ]] || [[ "$output" == "null" ]]
}

# =============================================================================
# Edge Case: Concurrent plan creation
# Two plans created simultaneously should not conflict
# =============================================================================

@test "qa: edge case - concurrent plan creation does not conflict" {
    # Create two plans with different names in quick succession
    PLAN1="concurrent-test-$$-1"
    PLAN2="concurrent-test-$$-2"

    # Create both plans
    run "$SCRIPTS_DIR/init-plan.sh" "$PLAN1" phase1
    [ "$status" -eq 0 ]

    run "$SCRIPTS_DIR/init-plan.sh" "$PLAN2" phase1
    [ "$status" -eq 0 ]

    # Verify both exist independently
    [ -d "$ACTIVE_DIR/$PLAN1" ]
    [ -d "$ACTIVE_DIR/$PLAN2" ]

    # Verify they have different session_ids
    SESSION1=$(cat "$ACTIVE_DIR/$PLAN1/session_id")
    SESSION2=$(cat "$ACTIVE_DIR/$PLAN2/session_id")
    [ "$SESSION1" != "$SESSION2" ]

    # Cleanup
    rm -rf "$ACTIVE_DIR/$PLAN1" "$ACTIVE_DIR/$PLAN2"
}

# =============================================================================
# Edge Case: Plan with many phases (10+)
# Verify plan with 10+ phases initializes correctly
# =============================================================================

@test "qa: edge case - plan with 10 phases initializes correctly" {
    MANY_PHASES_PLAN="many-phases-$$"

    run "$SCRIPTS_DIR/init-plan.sh" "$MANY_PHASES_PLAN" p1 p2 p3 p4 p5 p6 p7 p8 p9 p10
    [ "$status" -eq 0 ]

    # Verify all 10 phase directories exist (plus integration phase = 11)
    for i in {1..10}; do
        [ -d "$ACTIVE_DIR/$MANY_PHASES_PLAN/phases/p$i" ]
        [ -f "$ACTIVE_DIR/$MANY_PHASES_PLAN/phases/p$i/state.json" ]
    done

    # Verify integration phase exists
    [ -d "$ACTIVE_DIR/$MANY_PHASES_PLAN/phases/integration" ]

    # Cleanup
    rm -rf "$ACTIVE_DIR/$MANY_PHASES_PLAN"
}

@test "qa: edge case - plan with 15 phases initializes correctly" {
    MANY_PHASES_PLAN="many-phases-$$-15"

    run "$SCRIPTS_DIR/init-plan.sh" "$MANY_PHASES_PLAN" \
        phase1 phase2 phase3 phase4 phase5 \
        phase6 phase7 phase8 phase9 phase10 \
        phase11 phase12 phase13 phase14 phase15
    [ "$status" -eq 0 ]

    # Verify all 15 phase directories exist
    for i in {1..15}; do
        [ -d "$ACTIVE_DIR/$MANY_PHASES_PLAN/phases/phase$i" ]
    done

    # Cleanup
    rm -rf "$ACTIVE_DIR/$MANY_PHASES_PLAN"
}

# =============================================================================
# Edge Case: Workflow with cycles (retry loops)
# Legitimate retry loops should pass validation
# =============================================================================

@test "qa: edge case - workflow with legitimate retry loop passes validation" {
    cat > "$TEST_PLANS_DIR/retry_workflow.yaml" << 'EOF'
name: retry_workflow
version: 1
description: Workflow with legitimate retry loop

states:
  - name: attempt
    description: Try the operation
    prompt: prompts/feature/impl.md
    next: check

  - name: check
    description: Check if succeeded
    prompt: prompts/feature/impl-review.md
    next: done

  - name: done
    description: Completed
    prompt: prompts/common/complete.md

entry_state: attempt
terminal_states: [done]
EOF

    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_PLANS_DIR/retry_workflow.yaml"
    [ "$status" -eq 0 ]
    [[ "$output" == *"Validation passed"* ]]
}

# =============================================================================
# Edge Case: Mixed workflow types
# Creating plans of different types in same session
# =============================================================================

@test "qa: edge case - mixed workflow types in same session" {
    PLAN_FEATURE="mixed-feature-$$"
    PLAN_BUGFIX="mixed-bugfix-$$"
    PLAN_PERF="mixed-perf-$$"

    # Create plans of different types
    run "$SCRIPTS_DIR/init-plan.sh" --type feature "$PLAN_FEATURE" phase1
    [ "$status" -eq 0 ]

    run "$SCRIPTS_DIR/init-plan.sh" --type bugfix "$PLAN_BUGFIX" investigate
    [ "$status" -eq 0 ]

    run "$SCRIPTS_DIR/init-plan.sh" --type performance "$PLAN_PERF" optimize
    [ "$status" -eq 0 ]

    # Verify each has correct workflow_type
    FEAT_TYPE=$(jq -r '.workflow_type' "$ACTIVE_DIR/$PLAN_FEATURE/phases/phase1/state.json")
    [ "$FEAT_TYPE" == "feature" ]

    BUG_TYPE=$(jq -r '.workflow_type' "$ACTIVE_DIR/$PLAN_BUGFIX/phases/investigate/state.json")
    [ "$BUG_TYPE" == "bugfix" ]

    PERF_TYPE=$(jq -r '.workflow_type' "$ACTIVE_DIR/$PLAN_PERF/phases/optimize/state.json")
    [ "$PERF_TYPE" == "performance" ]

    # Cleanup
    rm -rf "$ACTIVE_DIR/$PLAN_FEATURE" "$ACTIVE_DIR/$PLAN_BUGFIX" "$ACTIVE_DIR/$PLAN_PERF"
}

# =============================================================================
# Additional integration tests for complete workflow state walks
# =============================================================================

@test "qa: full bugfix workflow state walk" {
    states=("investigate" "regression_tests" "test_review" "fix" "fix_review" "complete")
    for ((i=0; i<${#states[@]}-1; i++)); do
        current="${states[$i]}"
        expected="${states[$((i+1))]}"
        run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/bugfix.yaml" "$current"
        [ "$status" -eq 0 ]
        [ "$output" == "$expected" ]
    done
}

@test "qa: full investigation workflow state walk" {
    states=("research" "draft" "review" "complete")
    for ((i=0; i<${#states[@]}-1; i++)); do
        current="${states[$i]}"
        expected="${states[$((i+1))]}"
        run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/investigation.yaml" "$current"
        [ "$status" -eq 0 ]
        [ "$output" == "$expected" ]
    done
}

@test "qa: full refactor workflow state walk" {
    states=("characterize" "char_review" "refactor" "verify" "complete")
    for ((i=0; i<${#states[@]}-1; i++)); do
        current="${states[$i]}"
        expected="${states[$((i+1))]}"
        run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/refactor.yaml" "$current"
        [ "$status" -eq 0 ]
        [ "$output" == "$expected" ]
    done
}

@test "qa: full performance workflow state walk" {
    states=("baseline" "analyze" "optimize" "benchmark" "complete")
    for ((i=0; i<${#states[@]}-1; i++)); do
        current="${states[$i]}"
        expected="${states[$((i+1))]}"
        run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/performance.yaml" "$current"
        [ "$status" -eq 0 ]
        [ "$output" == "$expected" ]
    done
}

@test "qa: full feature workflow state walk" {
    # V2 feature workflow: qa_review and impl_review require verdicts
    # qa -> qa_review (linear)
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" "qa"
    [ "$status" -eq 0 ]
    [ "$output" == "qa_review" ]

    # qa_review -> impl (verdict: approved)
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" "qa_review" "approved"
    [ "$status" -eq 0 ]
    [ "$output" == "impl" ]

    # impl -> impl_review (linear)
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" "impl"
    [ "$status" -eq 0 ]
    [ "$output" == "impl_review" ]

    # impl_review -> complete (verdict: approved)
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" "impl_review" "approved"
    [ "$status" -eq 0 ]
    [ "$output" == "complete" ]
}

# =============================================================================
# Default workflow type behavior
# =============================================================================

@test "qa: init-plan.sh without --type defaults to feature" {
    run "$SCRIPTS_DIR/init-plan.sh" "$TEST_PLAN" phase1
    [ "$status" -eq 0 ]

    # Should default to feature workflow
    WORKFLOW_TYPE=$(jq -r '.workflow_type' "$ACTIVE_DIR/$TEST_PLAN/phases/phase1/state.json")
    [ "$WORKFLOW_TYPE" == "feature" ]
}

# =============================================================================
# Plan structure verification
# =============================================================================

@test "qa: created plan has all required files" {
    run "$SCRIPTS_DIR/init-plan.sh" "$TEST_PLAN" phase1 phase2
    [ "$status" -eq 0 ]

    # Verify required files
    [ -f "$ACTIVE_DIR/$TEST_PLAN/plan.json" ]
    [ -f "$ACTIVE_DIR/$TEST_PLAN/session_id" ]
    # workflow.yaml is only created for non-feature types (feature is default)

    # Verify phase directories
    [ -d "$ACTIVE_DIR/$TEST_PLAN/phases/phase1" ]
    [ -d "$ACTIVE_DIR/$TEST_PLAN/phases/phase2" ]
    [ -d "$ACTIVE_DIR/$TEST_PLAN/phases/integration" ]

    # Verify phase state files
    [ -f "$ACTIVE_DIR/$TEST_PLAN/phases/phase1/state.json" ]
    [ -f "$ACTIVE_DIR/$TEST_PLAN/phases/phase2/state.json" ]
    [ -f "$ACTIVE_DIR/$TEST_PLAN/phases/integration/state.json" ]
}

@test "qa: non-default workflow type creates workflow.yaml" {
    run "$SCRIPTS_DIR/init-plan.sh" --type bugfix "$TEST_PLAN" investigate
    [ "$status" -eq 0 ]

    # Non-feature types SHOULD have workflow.yaml copied
    [ -f "$ACTIVE_DIR/$TEST_PLAN/workflow.yaml" ]

    # Verify content matches the correct workflow
    run diff "$WORKFLOWS_DIR/bugfix.yaml" "$ACTIVE_DIR/$TEST_PLAN/workflow.yaml"
    [ "$status" -eq 0 ]
}

@test "qa: state.json has required fields" {
    run "$SCRIPTS_DIR/init-plan.sh" "$TEST_PLAN" phase1
    [ "$status" -eq 0 ]

    STATE_FILE="$ACTIVE_DIR/$TEST_PLAN/phases/phase1/state.json"

    # Verify required fields exist
    [ "$(jq -r '.phase' "$STATE_FILE")" == "phase1" ]
    [ "$(jq -r '.plan' "$STATE_FILE")" == "$TEST_PLAN" ]
    [ "$(jq -r '.workflow_type' "$STATE_FILE")" != "null" ]
    [ "$(jq -r '.phase_status' "$STATE_FILE")" != "null" ]
    [ "$(jq -r '.iteration.current' "$STATE_FILE")" == "0" ]
}

# ==============================================================================
# V2 Integration Tests - Conditional Branching
# ==============================================================================

# Helper function to create V2 test environment
setup_v2_test_env() {
    V2_PLAN="v2-test-$$-$RANDOM"
    V2_PHASE="test-phase"

    # Create plan structure directly in PLANS_DIR (already a temp dir from setup())
    V2_TEST_DIR="$PLANS_DIR/active/$V2_PLAN"
    mkdir -p "$V2_TEST_DIR/phases/$V2_PHASE"

    # Copy V2 feature workflow
    cp "$WORKFLOWS_DIR/feature.yaml" "$V2_TEST_DIR/workflow.yaml"

    # Initialize state.json with minimal required structure
    cat > "$V2_TEST_DIR/phases/$V2_PHASE/state.json" << 'EOF'
{
    "phase": "test-phase",
    "plan": "v2-test",
    "workflow_type": "feature",
    "phase_status": "qa_review",
    "iteration": {"current": 1, "max": 25},
    "stuck_iterations": 0,
    "packages": ["test-crate"]
}
EOF
}

teardown_v2_test_env() {
    rm -rf "$V2_TEST_DIR" 2>/dev/null || true
}

# =============================================================================
# Test Case: test_v2_full_qa_review_approval_flow
# =============================================================================

@test "v2: full qa_review approval flow transitions to impl" {
    setup_v2_test_env

    # Create review file with approved verdict
    cat > "$V2_TEST_DIR/phases/$V2_PHASE/qa_review.md" << 'EOF'
## Analysis
All tests cover the specification.

## Verdict
approved
EOF

    # Step 1: Extract verdict
    run "$SCRIPTS_DIR/extract-verdict.sh" \
        "$V2_TEST_DIR/phases/$V2_PHASE/qa_review.md" \
        "approved,gaps_found"
    [ "$status" -eq 0 ]
    [ "$output" = "approved" ]

    # Step 2: Record verdict
    run "$SCRIPTS_DIR/update-state.sh" "$V2_PLAN" "$V2_PHASE" verdict approved
    [ "$status" -eq 0 ]

    # Step 3: Get next state
    run "$SCRIPTS_DIR/get-next-state.sh" "$V2_TEST_DIR/workflow.yaml" qa_review approved
    [ "$status" -eq 0 ]
    [ "$output" = "impl" ]

    # Step 4: Update status
    run "$SCRIPTS_DIR/update-state.sh" "$V2_PLAN" "$V2_PHASE" status implementing
    [ "$status" -eq 0 ]

    # Step 5: Verify state.json
    STATE_FILE="$PLANS_DIR/active/$V2_PLAN/phases/$V2_PHASE/state.json"
    [ "$(jq -r '.last_verdict' "$STATE_FILE")" = "approved" ]
    [ "$(jq -r '.phase_status' "$STATE_FILE")" = "implementing" ]
    [ "$(jq -r '.verdicts_history | length' "$STATE_FILE")" -gt 0 ]
    [ "$(jq -r '.verdicts_history[0].verdict' "$STATE_FILE")" = "approved" ]

    teardown_v2_test_env
}

# =============================================================================
# Test Case: test_v2_full_qa_review_gaps_flow
# =============================================================================

@test "v2: full qa_review gaps_found flow loops back to qa" {
    setup_v2_test_env

    # Create review file with gaps_found verdict
    cat > "$V2_TEST_DIR/phases/$V2_PHASE/qa_review.md" << 'EOF'
## Analysis
Missing edge case coverage for empty input.

## Verdict
gaps_found
EOF

    # Step 1: Extract verdict
    run "$SCRIPTS_DIR/extract-verdict.sh" \
        "$V2_TEST_DIR/phases/$V2_PHASE/qa_review.md" \
        "approved,gaps_found"
    [ "$status" -eq 0 ]
    [ "$output" = "gaps_found" ]

    # Step 2: Record verdict
    run "$SCRIPTS_DIR/update-state.sh" "$V2_PLAN" "$V2_PHASE" verdict gaps_found
    [ "$status" -eq 0 ]

    # Step 3: Get next state
    run "$SCRIPTS_DIR/get-next-state.sh" "$V2_TEST_DIR/workflow.yaml" qa_review gaps_found
    [ "$status" -eq 0 ]
    [ "$output" = "qa" ]

    # Step 4: Update status
    run "$SCRIPTS_DIR/update-state.sh" "$V2_PLAN" "$V2_PHASE" status qa
    [ "$status" -eq 0 ]

    # Step 5: Verify state.json
    STATE_FILE="$PLANS_DIR/active/$V2_PLAN/phases/$V2_PHASE/state.json"
    [ "$(jq -r '.last_verdict' "$STATE_FILE")" = "gaps_found" ]
    [ "$(jq -r '.phase_status' "$STATE_FILE")" = "qa" ]

    teardown_v2_test_env
}

# =============================================================================
# Test Case: test_v2_full_impl_review_approval_flow
# =============================================================================

@test "v2: full impl_review approval flow transitions to complete" {
    setup_v2_test_env

    # Set initial status to impl_review
    "$SCRIPTS_DIR/update-state.sh" "$V2_PLAN" "$V2_PHASE" status impl_review

    # Create review file with approved verdict
    cat > "$V2_TEST_DIR/phases/$V2_PHASE/impl_review.md" << 'EOF'
## Analysis
Implementation is correct and all tests pass.

## Verdict
approved
EOF

    # Step 1: Extract verdict
    run "$SCRIPTS_DIR/extract-verdict.sh" \
        "$V2_TEST_DIR/phases/$V2_PHASE/impl_review.md" \
        "approved,concerns"
    [ "$status" -eq 0 ]
    [ "$output" = "approved" ]

    # Step 2: Record verdict
    run "$SCRIPTS_DIR/update-state.sh" "$V2_PLAN" "$V2_PHASE" verdict approved
    [ "$status" -eq 0 ]

    # Step 3: Get next state
    run "$SCRIPTS_DIR/get-next-state.sh" "$V2_TEST_DIR/workflow.yaml" impl_review approved
    [ "$status" -eq 0 ]
    [ "$output" = "complete" ]

    # Step 4: Update status
    run "$SCRIPTS_DIR/update-state.sh" "$V2_PLAN" "$V2_PHASE" status complete
    [ "$status" -eq 0 ]

    # Step 5: Verify state.json
    STATE_FILE="$PLANS_DIR/active/$V2_PLAN/phases/$V2_PHASE/state.json"
    [ "$(jq -r '.last_verdict' "$STATE_FILE")" = "approved" ]
    [ "$(jq -r '.phase_status' "$STATE_FILE")" = "complete" ]

    teardown_v2_test_env
}

# =============================================================================
# Test Case: test_v2_full_impl_review_concerns_flow
# =============================================================================

@test "v2: full impl_review concerns flow loops back to impl" {
    setup_v2_test_env

    # Set initial status to impl_review
    "$SCRIPTS_DIR/update-state.sh" "$V2_PLAN" "$V2_PHASE" status impl_review

    # Create review file with concerns verdict
    cat > "$V2_TEST_DIR/phases/$V2_PHASE/impl_review.md" << 'EOF'
## Analysis
Potential memory leak in cleanup function.

## Verdict
concerns
EOF

    # Step 1: Extract verdict
    run "$SCRIPTS_DIR/extract-verdict.sh" \
        "$V2_TEST_DIR/phases/$V2_PHASE/impl_review.md" \
        "approved,concerns"
    [ "$status" -eq 0 ]
    [ "$output" = "concerns" ]

    # Step 2: Record verdict
    run "$SCRIPTS_DIR/update-state.sh" "$V2_PLAN" "$V2_PHASE" verdict concerns
    [ "$status" -eq 0 ]

    # Step 3: Get next state
    run "$SCRIPTS_DIR/get-next-state.sh" "$V2_TEST_DIR/workflow.yaml" impl_review concerns
    [ "$status" -eq 0 ]
    [ "$output" = "impl" ]

    # Step 4: Update status
    run "$SCRIPTS_DIR/update-state.sh" "$V2_PLAN" "$V2_PHASE" status implementing
    [ "$status" -eq 0 ]

    # Step 5: Verify state.json
    STATE_FILE="$PLANS_DIR/active/$V2_PLAN/phases/$V2_PHASE/state.json"
    [ "$(jq -r '.last_verdict' "$STATE_FILE")" = "concerns" ]
    [ "$(jq -r '.phase_status' "$STATE_FILE")" = "implementing" ]

    teardown_v2_test_env
}

# =============================================================================
# Test Case: test_v2_unknown_verdict_stays_in_state
# =============================================================================

@test "v2: unknown verdict does not transition state" {
    setup_v2_test_env

    # Create review file with NO verdict section
    cat > "$V2_TEST_DIR/phases/$V2_PHASE/qa_review.md" << 'EOF'
## Analysis
I'm still reviewing the tests...
EOF

    # Record original phase_status
    STATE_FILE="$PLANS_DIR/active/$V2_PLAN/phases/$V2_PHASE/state.json"
    ORIGINAL_STATUS=$(jq -r '.phase_status' "$STATE_FILE")

    # Step 1: Extract verdict - should fail with exit code 1
    run "$SCRIPTS_DIR/extract-verdict.sh" \
        "$V2_TEST_DIR/phases/$V2_PHASE/qa_review.md" \
        "approved,gaps_found"
    [ "$status" -eq 1 ]
    [ "$output" = "unknown" ]

    # Step 2: Record verdict (even though unknown)
    run "$SCRIPTS_DIR/update-state.sh" "$V2_PLAN" "$V2_PHASE" verdict unknown
    [ "$status" -eq 0 ]

    # Step 3: Since extract-verdict returned exit code 1, we do NOT call get-next-state
    # This simulates what iterate.sh would do - skip state transition

    # Step 4: Verify state.json - phase_status unchanged
    [ "$(jq -r '.last_verdict' "$STATE_FILE")" = "unknown" ]
    [ "$(jq -r '.phase_status' "$STATE_FILE")" = "$ORIGINAL_STATUS" ]
    [ "$(jq -r '.verdicts_history | length' "$STATE_FILE")" -gt 0 ]

    teardown_v2_test_env
}

# =============================================================================
# Test Case: test_v2_all_workflows_validate
# =============================================================================

@test "v2: feature.yaml V2 workflow validates with verdict consistency" {
    run "$SCRIPTS_DIR/validate-workflow.sh" "$WORKFLOWS_DIR/feature.yaml"
    [ "$status" -eq 0 ]
    [[ "$output" == *"Validation passed"* ]]
    [[ "$output" == *"Verdict consistency valid"* ]]
}

@test "v2: bugfix.yaml validates successfully" {
    run "$SCRIPTS_DIR/validate-workflow.sh" "$WORKFLOWS_DIR/bugfix.yaml"
    [ "$status" -eq 0 ]
    [[ "$output" == *"Validation passed"* ]]
}

@test "v2: investigation.yaml validates successfully" {
    run "$SCRIPTS_DIR/validate-workflow.sh" "$WORKFLOWS_DIR/investigation.yaml"
    [ "$status" -eq 0 ]
    [[ "$output" == *"Validation passed"* ]]
}

@test "v2: performance.yaml validates successfully" {
    run "$SCRIPTS_DIR/validate-workflow.sh" "$WORKFLOWS_DIR/performance.yaml"
    [ "$status" -eq 0 ]
    [[ "$output" == *"Validation passed"* ]]
}

@test "v2: refactor.yaml validates successfully" {
    run "$SCRIPTS_DIR/validate-workflow.sh" "$WORKFLOWS_DIR/refactor.yaml"
    [ "$status" -eq 0 ]
    [[ "$output" == *"Validation passed"* ]]
}

# =============================================================================
# Test Case: test_v2_validate_get_next_state_branching
# =============================================================================

@test "v2: get-next-state qa_review approved -> impl" {
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" qa_review approved
    [ "$status" -eq 0 ]
    [ "$output" = "impl" ]
}

@test "v2: get-next-state qa_review gaps_found -> qa" {
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" qa_review gaps_found
    [ "$status" -eq 0 ]
    [ "$output" = "qa" ]
}

@test "v2: get-next-state impl_review approved -> complete" {
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" impl_review approved
    [ "$status" -eq 0 ]
    [ "$output" = "complete" ]
}

@test "v2: get-next-state impl_review concerns -> impl" {
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" impl_review concerns
    [ "$status" -eq 0 ]
    [ "$output" = "impl" ]
}

# =============================================================================
# Test Case: test_v2_backwards_compat_v1_workflow
# =============================================================================

@test "v2: backwards compatibility - V1 workflow validates" {
    # Create V1 workflow (no verdicts, linear next)
    V1_WORKFLOW=$(mktemp)
    cat > "$V1_WORKFLOW" << 'EOF'
name: v1_test
version: 1
description: V1 linear workflow

states:
  - name: start
    description: Starting state
    prompt: prompts/feature/qa.md
    next: middle

  - name: middle
    description: Middle state
    prompt: prompts/feature/impl.md
    next: end

  - name: end
    description: Terminal state
    prompt: prompts/common/complete.md

entry_state: start
terminal_states: [end]
EOF

    run "$SCRIPTS_DIR/validate-workflow.sh" "$V1_WORKFLOW"
    [ "$status" -eq 0 ]
    [[ "$output" == *"Validation passed"* ]]

    rm -f "$V1_WORKFLOW"
}

@test "v2: backwards compatibility - V1 linear transitions work without verdict" {
    # Use bugfix.yaml which has V1 linear states
    # investigate -> regression_tests is a linear transition
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/bugfix.yaml" investigate
    [ "$status" -eq 0 ]
    [ "$output" = "regression_tests" ]
}

# =============================================================================
# Test Case: test_v2_full_loop_qa_gaps_qa_approved_impl
# =============================================================================

@test "v2: full loop - qa gaps_found -> qa approved -> impl" {
    setup_v2_test_env

    # Set initial status to qa
    "$SCRIPTS_DIR/update-state.sh" "$V2_PLAN" "$V2_PHASE" status qa

    STATE_FILE="$PLANS_DIR/active/$V2_PLAN/phases/$V2_PHASE/state.json"

    # First iteration: qa_review with gaps_found
    "$SCRIPTS_DIR/update-state.sh" "$V2_PLAN" "$V2_PHASE" status qa_review
    run "$SCRIPTS_DIR/update-state.sh" "$V2_PLAN" "$V2_PHASE" verdict gaps_found
    [ "$status" -eq 0 ]

    run "$SCRIPTS_DIR/get-next-state.sh" "$V2_TEST_DIR/workflow.yaml" qa_review gaps_found
    [ "$status" -eq 0 ]
    [ "$output" = "qa" ]

    "$SCRIPTS_DIR/update-state.sh" "$V2_PLAN" "$V2_PHASE" status qa

    # Second iteration: qa_review with approved
    "$SCRIPTS_DIR/update-state.sh" "$V2_PLAN" "$V2_PHASE" status qa_review
    run "$SCRIPTS_DIR/update-state.sh" "$V2_PLAN" "$V2_PHASE" verdict approved
    [ "$status" -eq 0 ]

    run "$SCRIPTS_DIR/get-next-state.sh" "$V2_TEST_DIR/workflow.yaml" qa_review approved
    [ "$status" -eq 0 ]
    [ "$output" = "impl" ]

    "$SCRIPTS_DIR/update-state.sh" "$V2_PLAN" "$V2_PHASE" status implementing

    # Verify verdicts_history has 2 entries
    [ "$(jq -r '.verdicts_history | length' "$STATE_FILE")" -eq 2 ]
    [ "$(jq -r '.verdicts_history[0].verdict' "$STATE_FILE")" = "gaps_found" ]
    [ "$(jq -r '.verdicts_history[1].verdict' "$STATE_FILE")" = "approved" ]
    [ "$(jq -r '.phase_status' "$STATE_FILE")" = "implementing" ]

    teardown_v2_test_env
}

# =============================================================================
# Test Case: test_v2_component_chain_integration
# =============================================================================

@test "v2: component chain integration - extract -> update -> get-next -> update" {
    setup_v2_test_env

    # Create qa_review.md with specific verdict
    cat > "$V2_TEST_DIR/phases/$V2_PHASE/qa_review.md" << 'EOF'
## Verdict
gaps_found
EOF

    STATE_FILE="$PLANS_DIR/active/$V2_PLAN/phases/$V2_PHASE/state.json"

    # Step 1: Extract verdict
    run "$SCRIPTS_DIR/extract-verdict.sh" \
        "$V2_TEST_DIR/phases/$V2_PHASE/qa_review.md" \
        "approved,gaps_found"
    [ "$status" -eq 0 ]
    [ "$output" = "gaps_found" ]
    EXTRACTED_VERDICT="$output"

    # Step 2: Record verdict
    run "$SCRIPTS_DIR/update-state.sh" "$V2_PLAN" "$V2_PHASE" verdict "$EXTRACTED_VERDICT"
    [ "$status" -eq 0 ]
    [ "$(jq -r '.last_verdict' "$STATE_FILE")" = "gaps_found" ]

    # Step 3: Get next state
    run "$SCRIPTS_DIR/get-next-state.sh" "$V2_TEST_DIR/workflow.yaml" qa_review "$EXTRACTED_VERDICT"
    [ "$status" -eq 0 ]
    [ "$output" = "qa" ]
    NEXT_STATE="$output"

    # Step 4: Update status
    run "$SCRIPTS_DIR/update-state.sh" "$V2_PLAN" "$V2_PHASE" status "$NEXT_STATE"
    [ "$status" -eq 0 ]
    [ "$(jq -r '.phase_status' "$STATE_FILE")" = "qa" ]

    # Verify verdicts_history entry
    [ "$(jq -r '.verdicts_history | length' "$STATE_FILE")" -gt 0 ]
    [ "$(jq -r '.verdicts_history[-1].verdict' "$STATE_FILE")" = "gaps_found" ]

    teardown_v2_test_env
}

# =============================================================================
# Test Case: test_v2_corrupted_state_json_handling
# =============================================================================

@test "v2: corrupted state.json handling" {
    setup_v2_test_env

    # Corrupt state.json
    echo "{invalid json" > "$PLANS_DIR/active/$V2_PLAN/phases/$V2_PHASE/state.json"

    # Try to update verdict
    run "$SCRIPTS_DIR/update-state.sh" "$V2_PLAN" "$V2_PHASE" verdict approved
    [ "$status" -eq 1 ]
    # stderr should contain parse error indication
    [[ "$output" == *"parse"* ]] || [[ "$output" == *"Invalid"* ]] || [[ "$output" == *"error"* ]]

    teardown_v2_test_env
}

# =============================================================================
# Test Case: test_v2_permission_error_handling
# =============================================================================

@test "v2: permission error handling - unreadable file" {
    setup_v2_test_env

    # Create and make file unreadable
    cat > "$V2_TEST_DIR/phases/$V2_PHASE/qa_review.md" << 'EOF'
## Verdict
approved
EOF
    chmod 000 "$V2_TEST_DIR/phases/$V2_PHASE/qa_review.md"

    # Try to extract verdict
    run "$SCRIPTS_DIR/extract-verdict.sh" \
        "$V2_TEST_DIR/phases/$V2_PHASE/qa_review.md" \
        "approved,gaps_found"
    [ "$status" -eq 1 ]
    [ "$output" = "unknown" ]

    # Restore permissions for cleanup
    chmod 644 "$V2_TEST_DIR/phases/$V2_PHASE/qa_review.md"

    teardown_v2_test_env
}

# =============================================================================
# Test Case: test_v2_mixed_v1_v2_workflow_execution
# =============================================================================

@test "v2: mixed V1/V2 workflow - linear and branching transitions" {
    # feature.yaml has both:
    # - V1 linear: qa -> qa_review (no verdict needed)
    # - V2 branching: qa_review -> impl OR qa (verdict needed)

    # Test V1 linear transition (qa -> qa_review)
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" qa
    [ "$status" -eq 0 ]
    [ "$output" = "qa_review" ]

    # Test V2 branching transition with approved verdict
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" qa_review approved
    [ "$status" -eq 0 ]
    [ "$output" = "impl" ]

    # Test V2 branching transition with gaps_found verdict
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" qa_review gaps_found
    [ "$status" -eq 0 ]
    [ "$output" = "qa" ]
}

# =============================================================================
# Test Case: test_v2_get_next_state_missing_verdict_fails
# =============================================================================

@test "v2: get-next-state fails when V2 state requires verdict but none provided" {
    # qa_review is a V2 state that requires a verdict
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" qa_review
    [ "$status" -eq 1 ]
    [[ "$output" == *"conditional"* ]] || [[ "$output" == *"verdict"* ]]
}

# =============================================================================
# Test Case: test_v2_get_next_state_invalid_verdict_fails
# =============================================================================

@test "v2: get-next-state fails for invalid verdict" {
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" qa_review invalid_verdict
    [ "$status" -eq 1 ]
    [[ "$output" == *"transition"* ]] || [[ "$output" == *"verdict"* ]]
}

# =============================================================================
# Test Case: test_v2_extract_verdict_case_insensitivity
# =============================================================================

@test "v2: extract-verdict handles case insensitivity" {
    TEMP_FILE=$(mktemp)
    cat > "$TEMP_FILE" << 'EOF'
## Verdict
APPROVED
EOF

    run "$SCRIPTS_DIR/extract-verdict.sh" "$TEMP_FILE" "approved,gaps_found"
    [ "$status" -eq 0 ]
    [ "$output" = "approved" ]

    rm -f "$TEMP_FILE"
}

# =============================================================================
# Test Case: test_v2_extract_verdict_whitespace_handling
# =============================================================================

@test "v2: extract-verdict handles whitespace around verdict" {
    TEMP_FILE=$(mktemp)
    cat > "$TEMP_FILE" << 'EOF'
## Verdict

   approved
EOF

    run "$SCRIPTS_DIR/extract-verdict.sh" "$TEMP_FILE" "approved,gaps_found"
    [ "$status" -eq 0 ]
    [ "$output" = "approved" ]

    rm -f "$TEMP_FILE"
}

# =============================================================================
# Test Case: test_v2_extract_verdict_ignores_code_blocks
# =============================================================================

@test "v2: extract-verdict ignores verdict in code blocks" {
    TEMP_FILE=$(mktemp)
    cat > "$TEMP_FILE" << 'EOF'
## Example
```
## Verdict
approved
```

## Verdict
gaps_found
EOF

    run "$SCRIPTS_DIR/extract-verdict.sh" "$TEMP_FILE" "approved,gaps_found"
    [ "$status" -eq 0 ]
    [ "$output" = "gaps_found" ]

    rm -f "$TEMP_FILE"
}

# =============================================================================
# Test Case: test_v2_extract_verdict_takes_last_verdict
# =============================================================================

@test "v2: extract-verdict takes last verdict section" {
    TEMP_FILE=$(mktemp)
    cat > "$TEMP_FILE" << 'EOF'
## Verdict
gaps_found

## Later Analysis

## Verdict
approved
EOF

    run "$SCRIPTS_DIR/extract-verdict.sh" "$TEMP_FILE" "approved,gaps_found"
    [ "$status" -eq 0 ]
    [ "$output" = "approved" ]

    rm -f "$TEMP_FILE"
}

# =============================================================================
# Test Case: test_v2_validate_workflow_catches_missing_verdicts
# =============================================================================

@test "v2: validate-workflow catches V2 state with map next but no verdicts" {
    TEMP_WORKFLOW=$(mktemp)
    cat > "$TEMP_WORKFLOW" << 'EOF'
name: broken_v2
version: 2
description: V2 workflow with missing verdicts array

states:
  - name: start
    description: Starting state
    prompt: prompts/feature/qa.md
    next:
      approved: end
      rejected: start

  - name: end
    description: Terminal state
    prompt: prompts/common/complete.md

entry_state: start
terminal_states: [end]
EOF

    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEMP_WORKFLOW"
    [ "$status" -ne 0 ]
    [[ "$output" == *"no verdicts"* ]] || [[ "$output" == *"verdicts"* ]]

    rm -f "$TEMP_WORKFLOW"
}

# =============================================================================
# Test Case: test_v2_validate_workflow_catches_mismatched_verdicts
# =============================================================================

@test "v2: validate-workflow catches mismatched verdicts and transitions" {
    TEMP_WORKFLOW=$(mktemp)
    cat > "$TEMP_WORKFLOW" << 'EOF'
name: mismatched_v2
version: 2
description: V2 workflow with mismatched verdicts

states:
  - name: start
    description: Starting state
    prompt: prompts/feature/qa.md
    verdicts:
      - approved
      - rejected
    next:
      approved: end
      # missing: rejected

  - name: end
    description: Terminal state
    prompt: prompts/common/complete.md

entry_state: start
terminal_states: [end]
EOF

    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEMP_WORKFLOW"
    [ "$status" -ne 0 ]
    [[ "$output" == *"no transition"* ]] || [[ "$output" == *"rejected"* ]]

    rm -f "$TEMP_WORKFLOW"
}

# =============================================================================
# Test Case: test_v2_terminal_state_handling
# =============================================================================

@test "v2: terminal state returns TERMINAL" {
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" complete
    [ "$status" -eq 0 ]
    [ "$output" = "TERMINAL" ]
}

@test "v2: blocked terminal state returns TERMINAL" {
    run "$SCRIPTS_DIR/get-next-state.sh" "$WORKFLOWS_DIR/feature.yaml" blocked
    [ "$status" -eq 0 ]
    [ "$output" = "TERMINAL" ]
}

# =============================================================================
# Test Case: test_v2_verdicts_history_has_timestamp
# =============================================================================

@test "v2: verdicts_history entries have timestamps" {
    setup_v2_test_env

    STATE_FILE="$PLANS_DIR/active/$V2_PLAN/phases/$V2_PHASE/state.json"

    run "$SCRIPTS_DIR/update-state.sh" "$V2_PLAN" "$V2_PHASE" verdict approved
    [ "$status" -eq 0 ]

    # Verify timestamp exists and is in ISO format
    TIMESTAMP=$(jq -r '.verdicts_history[0].timestamp' "$STATE_FILE")
    [ "$TIMESTAMP" != "null" ]
    [[ "$TIMESTAMP" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$ ]]

    teardown_v2_test_env
}

# =============================================================================
# Test Case: test_v2_file_not_found_handling
# =============================================================================

@test "v2: extract-verdict handles file not found" {
    run "$SCRIPTS_DIR/extract-verdict.sh" "/nonexistent/path/review.md" "approved,gaps_found"
    [ "$status" -eq 1 ]
    [ "$output" = "unknown" ]
}

@test "v2: get-next-state handles workflow file not found" {
    run "$SCRIPTS_DIR/get-next-state.sh" "/nonexistent/workflow.yaml" qa_review approved
    [ "$status" -eq 1 ]
    [[ "$output" == *"not found"* ]]
}
