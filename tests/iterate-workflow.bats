#!/usr/bin/env bats

# Tests for iterate.sh workflow integration (V1a)
# These tests verify workflow detection without running full iterations

setup() {
    load 'test_helper'
    setup_temp_dir

    # Create a mock plan structure
    export MOCK_PLAN_DIR="$TEST_TEMP_DIR/active/test-plan"
    export MOCK_PHASE_DIR="$MOCK_PLAN_DIR/phases/test-phase"
    mkdir -p "$MOCK_PHASE_DIR"

    # Create minimal state.json
    cat > "$MOCK_PHASE_DIR/state.json" << 'EOF'
{
  "phase_status": "pending",
  "iteration": {"current": 0, "max": 25},
  "packages": ["test-package"],
  "tests_passing": 0,
  "tests_total": 0
}
EOF

    # Create minimal plan.md
    cat > "$MOCK_PHASE_DIR/plan.md" << 'EOF'
## Objective
Test objective

## Files
- test.rs

## Test Cases
- test_foo
EOF
}

teardown() {
    teardown_temp_dir
}

@test "iterate.sh exists and is executable" {
    [[ -x "$SCRIPTS_DIR/iterate.sh" ]]
}

@test "iterate.sh is syntactically valid bash" {
    run bash -n "$SCRIPTS_DIR/iterate.sh"
    [[ "$status" -eq 0 ]]
}

@test "iterate.sh contains workflow detection function" {
    grep -q "detect_workflow()" "$SCRIPTS_DIR/iterate.sh"
}

@test "iterate.sh checks for plan-specific workflow.yaml" {
    grep -q 'PLAN_DIR/workflow.yaml' "$SCRIPTS_DIR/iterate.sh"
}

@test "iterate.sh falls back to base workflows directory" {
    grep -q 'WORKFLOWS_DIR' "$SCRIPTS_DIR/iterate.sh"
}

@test "iterate.sh has WORKFLOW_FILE variable" {
    grep -q 'WORKFLOW_FILE=' "$SCRIPTS_DIR/iterate.sh"
}

@test "iterate.sh uses get-next-state.sh for workflow transitions" {
    grep -q 'get-next-state.sh' "$SCRIPTS_DIR/iterate.sh"
}

@test "iterate.sh shows workflow in banner when detected" {
    grep -q 'Workflow:' "$SCRIPTS_DIR/iterate.sh"
}

@test "detect_workflow function can be sourced and tested" {
    # Extract and test the detect_workflow function
    # Set up the variables it needs
    ORCH_DIR="$TEST_DIR/.."
    WORKFLOWS_DIR="$ORCH_DIR/workflows"
    PLAN_DIR="$MOCK_PLAN_DIR"
    STATE_FILE="$MOCK_PHASE_DIR/state.json"

    # Source just the function
    eval "$(sed -n '/^detect_workflow()/,/^}/p' "$SCRIPTS_DIR/iterate.sh")"

    # Test: should detect feature.yaml as default
    run detect_workflow
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"feature.yaml"* ]]
}

@test "detect_workflow prefers plan-specific workflow" {
    ORCH_DIR="$TEST_DIR/.."
    WORKFLOWS_DIR="$ORCH_DIR/workflows"
    PLAN_DIR="$MOCK_PLAN_DIR"
    STATE_FILE="$MOCK_PHASE_DIR/state.json"

    # Create a plan-specific workflow
    cat > "$MOCK_PLAN_DIR/workflow.yaml" << 'EOF'
name: custom
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

    eval "$(sed -n '/^detect_workflow()/,/^}/p' "$SCRIPTS_DIR/iterate.sh")"

    run detect_workflow
    [[ "$status" -eq 0 ]]
    [[ "$output" == "$MOCK_PLAN_DIR/workflow.yaml" ]]
}

@test "detect_workflow uses workflow_type from state.json" {
    ORCH_DIR="$TEST_DIR/.."
    WORKFLOWS_DIR="$ORCH_DIR/workflows"
    PLAN_DIR="$MOCK_PLAN_DIR"
    STATE_FILE="$MOCK_PHASE_DIR/state.json"

    # Update state.json with workflow_type
    cat > "$MOCK_PHASE_DIR/state.json" << 'EOF'
{
  "phase_status": "pending",
  "workflow_type": "feature",
  "iteration": {"current": 0, "max": 25}
}
EOF

    eval "$(sed -n '/^detect_workflow()/,/^}/p' "$SCRIPTS_DIR/iterate.sh")"

    run detect_workflow
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"feature.yaml"* ]]
}
