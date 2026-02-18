#!/usr/bin/env bats
# Tests for verify-phase-complete.sh

setup() {
    SCRIPT_DIR="$(cd "$(dirname "$BATS_TEST_FILENAME")/.."/scripts && pwd)"
    PROJECT_ROOT="$(dirname "$(dirname "$SCRIPT_DIR")")"
    TEST_PLAN="test-verify-$$"
    TEST_PHASE="01-test"

    # Use temp dir so tests don't pollute .plans/active/
    export PLANS_DIR="$(mktemp -d)"
    PLAN_DIR="$PLANS_DIR/active/$TEST_PLAN"
    PHASE_DIR="$PLAN_DIR/phases/$TEST_PHASE"
    mkdir -p "$PHASE_DIR"

    # Create minimal state.json
    cat > "$PHASE_DIR/state.json" << 'EOF'
{
  "phase_status": "implementing",
  "test_files": [],
  "tests_passing": 0,
  "tests_total": 0,
  "dispute": null
}
EOF
}

teardown() {
    rm -rf "$PLANS_DIR" 2>/dev/null || true
}

@test "verify-phase-complete.sh exists and is executable" {
    [ -x "$SCRIPT_DIR/verify-phase-complete.sh" ]
}

@test "fails with no arguments" {
    run "$SCRIPT_DIR/verify-phase-complete.sh"
    [ "$status" -eq 2 ]
}

@test "fails when no test files registered" {
    run "$SCRIPT_DIR/verify-phase-complete.sh" "$TEST_PLAN" "$TEST_PHASE"
    [ "$status" -eq 1 ]
    [[ "$output" == *"Test files registered"*"FAIL"* ]]
}

@test "fails when tests_total is 0" {
    # Add test file but no tests run
    jq '.test_files = ["test.bats"]' "$PHASE_DIR/state.json" > tmp.json && mv tmp.json "$PHASE_DIR/state.json"

    run "$SCRIPT_DIR/verify-phase-complete.sh" "$TEST_PLAN" "$TEST_PHASE"
    [ "$status" -eq 1 ]
    [[ "$output" == *"Tests were run"*"FAIL"* ]]
}

@test "fails when not all tests pass" {
    # Add test file and partial pass
    jq '.test_files = ["test.bats"] | .tests_passing = 3 | .tests_total = 5' "$PHASE_DIR/state.json" > tmp.json && mv tmp.json "$PHASE_DIR/state.json"

    run "$SCRIPT_DIR/verify-phase-complete.sh" "$TEST_PLAN" "$TEST_PHASE"
    [ "$status" -eq 1 ]
    [[ "$output" == *"All tests passing"*"FAIL"* ]]
}

@test "fails when active dispute exists" {
    # All tests pass but dispute active
    jq '.test_files = ["test.bats"] | .tests_passing = 5 | .tests_total = 5 | .dispute = {"test_name": "test_foo", "reason": "wrong"}' "$PHASE_DIR/state.json" > tmp.json && mv tmp.json "$PHASE_DIR/state.json"

    run "$SCRIPT_DIR/verify-phase-complete.sh" "$TEST_PLAN" "$TEST_PHASE"
    [ "$status" -eq 1 ]
    [[ "$output" == *"No active disputes"*"FAIL"* ]]
}

@test "passes when resolved dispute exists" {
    # All tests pass and dispute resolved
    jq '.test_files = ["test.bats"] | .tests_passing = 5 | .tests_total = 5 | .dispute = {"test_name": "test_foo", "reason": "wrong", "resolution": "approved"}' "$PHASE_DIR/state.json" > tmp.json && mv tmp.json "$PHASE_DIR/state.json"

    run "$SCRIPT_DIR/verify-phase-complete.sh" "$TEST_PLAN" "$TEST_PHASE"
    [ "$status" -eq 0 ]
    [[ "$output" == *"PHASE READY FOR COMPLETION"* ]]
}

@test "passes when all conditions met" {
    # All tests pass, no dispute
    jq '.test_files = ["test.bats"] | .tests_passing = 5 | .tests_total = 5' "$PHASE_DIR/state.json" > tmp.json && mv tmp.json "$PHASE_DIR/state.json"

    run "$SCRIPT_DIR/verify-phase-complete.sh" "$TEST_PLAN" "$TEST_PHASE"
    [ "$status" -eq 0 ]
    [[ "$output" == *"PHASE READY FOR COMPLETION"* ]]
}

@test "iterate.sh calls verify-phase-complete after impl-review" {
    grep -q 'verify-phase-complete.sh' "$SCRIPT_DIR/iterate.sh"
    grep -q 'IMPL_REVIEW_APPROVED' "$SCRIPT_DIR/iterate.sh"
}

@test "iterate.sh initializes IMPL_REVIEW_APPROVED before case" {
    # Should be initialized to false before the case statement
    grep -B5 'case "\$MODE" in' "$SCRIPT_DIR/iterate.sh" | grep -q 'IMPL_REVIEW_APPROVED=false'
}

@test "iterate.sh sets complete only after verification passes" {
    # The status complete should only be set inside the verification block
    # Not directly after checking APPROVED in impl-review case
    ! grep -A2 'grep -q "APPROVED"' "$SCRIPT_DIR/iterate.sh" | grep -q 'status complete'
}
