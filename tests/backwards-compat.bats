#!/usr/bin/env bats

# Tests for backwards compatibility
# Ensures old plans without workflow.yaml still work

setup() {
    load 'test_helper'
    setup_temp_dir

    # Create a mock plan structure WITHOUT workflow
    export MOCK_PLAN_DIR="$TEST_TEMP_DIR/active/legacy-plan"
    export MOCK_PHASE_DIR="$MOCK_PLAN_DIR/phases/legacy-phase"
    mkdir -p "$MOCK_PHASE_DIR"

    # Create minimal state.json (old format without workflow_type)
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

@test "backwards compat: detect_workflow returns feature.yaml as default" {
    # Even without explicit workflow, should fall back to feature.yaml
    ORCH_DIR="$TEST_DIR/.."
    WORKFLOWS_DIR="$ORCH_DIR/workflows"
    PLAN_DIR="$MOCK_PLAN_DIR"
    STATE_FILE="$MOCK_PHASE_DIR/state.json"

    eval "$(sed -n '/^detect_workflow()/,/^}/p' "$SCRIPTS_DIR/iterate.sh")"

    run detect_workflow
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"feature.yaml"* ]]
}

@test "backwards compat: old state.json format still readable" {
    # Test that phase_status field is still read correctly
    run jq -r '.phase_status' "$MOCK_PHASE_DIR/state.json"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "pending" ]]
}

@test "backwards compat: iteration fields still readable" {
    run jq -r '.iteration.current' "$MOCK_PHASE_DIR/state.json"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "0" ]]

    run jq -r '.iteration.max' "$MOCK_PHASE_DIR/state.json"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "25" ]]
}

@test "backwards compat: crates field still readable" {
    run jq -r '.packages | join(",")' "$MOCK_PHASE_DIR/state.json"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "test-package" ]]
}

@test "backwards compat: iterate.sh preserves legacy mode-based guidance" {
    # Check that legacy case statement still exists
    grep -q 'case "\$MODE" in' "$SCRIPTS_DIR/iterate.sh"
    grep -q 'qa)' "$SCRIPTS_DIR/iterate.sh"
    grep -q 'impl)' "$SCRIPTS_DIR/iterate.sh"
    grep -q 'fix)' "$SCRIPTS_DIR/iterate.sh"
}

@test "backwards compat: iterate.sh preserves qa-review mode" {
    grep -q 'qa-review)' "$SCRIPTS_DIR/iterate.sh"
}

@test "backwards compat: iterate.sh preserves impl-review mode" {
    grep -q 'impl-review)' "$SCRIPTS_DIR/iterate.sh"
}

@test "backwards compat: update-state.sh commands unchanged" {
    # Check that common update-state.sh commands are still referenced
    grep -q 'update-state.sh.*increment-iteration' "$SCRIPTS_DIR/iterate.sh"
    grep -q 'update-state.sh.*status' "$SCRIPTS_DIR/iterate.sh"
    # Tests are now run via run-phase-tests.sh (which calls update-state.sh tests internally)
    grep -q 'run-phase-tests.sh' "$SCRIPTS_DIR/iterate.sh"
}

@test "backwards compat: get-state.sh still called" {
    grep -q 'get-state.sh' "$SCRIPTS_DIR/iterate.sh"
}

@test "backwards compat: dispute handling still present" {
    grep -q 'disputed' "$SCRIPTS_DIR/iterate.sh"
    grep -q 'approve-dispute' "$SCRIPTS_DIR/iterate.sh"
}
