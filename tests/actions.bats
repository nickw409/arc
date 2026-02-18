#!/usr/bin/env bats

# Tests for actions.sh
# Phase: action-registry (orchestration-v4)
#
# Tests the action registry that provides reusable action functions
# callable from workflow hooks, escalation triggers, and intervention handlers.

setup() {
    load 'test_helper'
    setup_temp_dir

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

    # Set up mock bin directory for overriding cargo/git
    export MOCK_BIN="$TEST_TEMP_DIR/mock_bin"
    mkdir -p "$MOCK_BIN"

    # Default environment
    export ARC_DEFAULT_PKG="test-package"
    export VERDICT=""
    export ARC_HOME="$TEST_TEMP_DIR/orchestration"
    mkdir -p "$ARC_HOME/scripts"
}

teardown() {
    teardown_temp_dir
}

# Helper: create a mock cargo nextest that produces configurable output
# Usage: create_mock_cargo <total> <passed> <failed> [exit_code]
create_mock_cargo() {
    local total="${1:-10}"
    local passed="${2:-10}"
    local failed="${3:-0}"
    local exit_code="${4:-0}"
    local skipped="${5:-0}"

    cat > "$MOCK_BIN/cargo" << SCRIPT
#!/usr/bin/env bash
echo "    Compiling test-package v0.1.0"
echo "     Running tests"
echo ""
echo "Summary [   0.456s] ${total} tests run: ${passed} passed, ${failed} failed, ${skipped} skipped"
exit ${exit_code}
SCRIPT
    chmod +x "$MOCK_BIN/cargo"
}

# Helper: create a mock cargo nextest that finds no tests
create_mock_cargo_no_tests() {
    cat > "$MOCK_BIN/cargo" << 'SCRIPT'
#!/usr/bin/env bash
echo "error: no tests to run" >&2
exit 4
SCRIPT
    chmod +x "$MOCK_BIN/cargo"
}

# Helper: create a mock cargo nextest that errors (crate not found)
create_mock_cargo_crate_error() {
    cat > "$MOCK_BIN/cargo" << 'SCRIPT'
#!/usr/bin/env bash
echo "error[E0463]: can't find crate for \`nonexistent_crate\`" >&2
exit 101
SCRIPT
    chmod +x "$MOCK_BIN/cargo"
}

# Helper: create a mock cargo that produces stderr along with stdout
create_mock_cargo_with_stderr() {
    cat > "$MOCK_BIN/cargo" << 'SCRIPT'
#!/usr/bin/env bash
echo "    Compiling test-package v0.1.0"
echo "warning: unused variable" >&2
echo "Summary [   0.456s] 5 tests run: 5 passed, 0 failed, 0 skipped"
exit 0
SCRIPT
    chmod +x "$MOCK_BIN/cargo"
}

# Helper: create a mock git
# Usage: create_mock_git [commit_exit_code]
create_mock_git() {
    local commit_exit="${1:-0}"

    cat > "$MOCK_BIN/git" << SCRIPT
#!/usr/bin/env bash
case "\$1" in
    diff)
        # Simulate changes exist (exit 1 = differences found)
        exit 1
        ;;
    add)
        exit 0
        ;;
    commit)
        exit ${commit_exit}
        ;;
    rev-parse)
        echo "abc123def456"
        exit 0
        ;;
    *)
        exit 0
        ;;
esac
SCRIPT
    chmod +x "$MOCK_BIN/git"
}

# Helper: create a mock git with no changes
create_mock_git_clean() {
    cat > "$MOCK_BIN/git" << 'SCRIPT'
#!/usr/bin/env bash
case "$1" in
    diff)
        # No differences (exit 0 = clean)
        exit 0
        ;;
    *)
        exit 0
        ;;
esac
SCRIPT
    chmod +x "$MOCK_BIN/git"
}

# Helper: create a mock git that fails (not a repo)
create_mock_git_not_repo() {
    cat > "$MOCK_BIN/git" << 'SCRIPT'
#!/usr/bin/env bash
echo "fatal: not a git repository (or any parent up to mount point /)" >&2
exit 128
SCRIPT
    chmod +x "$MOCK_BIN/git"
}

# Helper: source actions.sh and run a function with mocked PATH
# Usage: run_action <function_name> [args...]
run_action() {
    local func="$1"
    shift
    run bash -c "
        export PATH=\"$MOCK_BIN:\$PATH\"
        export PHASE_DIR=\"$PHASE_DIR\"
        export STATE_FILE=\"$STATE_FILE\"
        export ARC_DEFAULT_PKG=\"${ARC_DEFAULT_PKG:-test-package}\"
        export VERDICT=\"${VERDICT:-}\"
        export ARC_HOME=\"${ARC_HOME:-}\"
        source \"$ACTIONS_SH\"
        $func \"\$@\"
    " -- "$@"
}

# Helper: source actions.sh with specific env vars unset
# Usage: run_action_unset <var_to_unset> <function_name> [args...]
run_action_unset() {
    local unset_var="$1"
    local func="$2"
    shift 2
    run bash -c "
        export PATH=\"$MOCK_BIN:\$PATH\"
        export PHASE_DIR=\"$PHASE_DIR\"
        export STATE_FILE=\"$STATE_FILE\"
        export ARC_DEFAULT_PKG=\"${ARC_DEFAULT_PKG:-test-package}\"
        export VERDICT=\"${VERDICT:-}\"
        export ARC_HOME=\"${ARC_HOME:-}\"
        unset $unset_var
        source \"$ACTIONS_SH\"
        $func \"\$@\"
    " -- "$@"
}

#=============================================================================
# Script Existence and Syntax Tests
#=============================================================================

@test "actions.sh exists and is readable" {
    [[ -f "$ACTIONS_SH" ]]
}

@test "actions.sh is syntactically valid bash" {
    run bash -n "$ACTIONS_SH"
    [[ "$status" -eq 0 ]]
}

@test "actions.sh can be sourced without error" {
    run bash -c "source '$ACTIONS_SH'"
    [[ "$status" -eq 0 ]]
}

@test "actions.sh does not use set -e" {
    # File must exist first
    [[ -f "$ACTIONS_SH" ]]

    # The spec requires set -uo pipefail but NOT -e
    # Check for 'set -e' or 'set -...e...' but exclude 'set -uo pipefail' and comments
    run bash -c "grep -P '^set\\s+-(\\w*e\\w*)' '$ACTIONS_SH' | grep -v pipefail"
    [[ "$status" -eq 1 ]]  # No matches found = good
}

@test "actions.sh uses set -uo pipefail" {
    run bash -c "grep -E '^set -uo pipefail' '$ACTIONS_SH'"
    [[ "$status" -eq 0 ]]
}

#=============================================================================
# action_run_tests Tests
#=============================================================================

@test "test_run_tests_passing: exit 0 when all tests pass" {
    create_mock_cargo 10 10 0 0
    run_action action_run_tests "existing_passing" "test_output.txt" "false"
    [[ "$status" -eq 0 ]]
}

@test "test_run_tests_passing: state.json updated with test counts" {
    create_mock_cargo 10 10 0 0
    run_action action_run_tests "existing_passing" "test_output.txt" "false"
    [[ "$status" -eq 0 ]]

    local total
    total=$(jq -r '.tests_total' "$STATE_FILE")
    [[ "$total" -eq 10 ]]

    local passing
    passing=$(jq -r '.tests_passing' "$STATE_FILE")
    [[ "$passing" -eq 10 ]]
}

@test "test_run_tests_failing_expected: exit 0 when tests fail and expect_failure=true" {
    create_mock_cargo 10 8 2 1
    run_action action_run_tests "qa_phase" "test_output.txt" "true"
    [[ "$status" -eq 0 ]]
}

@test "test_run_tests_failing_unexpected: exit 1 when tests fail and expect_failure=false" {
    create_mock_cargo 10 8 2 1
    run_action action_run_tests "qa_phase" "test_output.txt" "false"
    [[ "$status" -eq 1 ]]
}

@test "test_run_tests_output_saved: output file created at PHASE_DIR/save_to" {
    create_mock_cargo 5 5 0 0
    run_action action_run_tests "pattern" "custom_output.txt" "false"
    [[ "$status" -eq 0 ]]
    [[ -f "$PHASE_DIR/custom_output.txt" ]]
}

@test "test_run_tests_output_saved: output file contains cargo output" {
    create_mock_cargo 5 5 0 0
    run_action action_run_tests "pattern" "custom_output.txt" "false"
    [[ "$status" -eq 0 ]]

    # Check that output file has content from cargo
    local content
    content=$(cat "$PHASE_DIR/custom_output.txt")
    [[ "$content" == *"Summary"* ]]
    [[ "$content" == *"5 tests run"* ]]
}

@test "test_run_tests_no_tests_found: exit 0 with tests_total=0" {
    create_mock_cargo_no_tests
    run_action action_run_tests "nonexistent_pattern" "test_output.txt" "false"
    [[ "$status" -eq 0 ]]

    local total
    total=$(jq -r '.tests_total' "$STATE_FILE")
    [[ "$total" -eq 0 ]]

    local passing
    passing=$(jq -r '.tests_passing' "$STATE_FILE")
    [[ "$passing" -eq 0 ]]
}

@test "test_run_tests_expect_failure_but_pass: exit 1 when expect_failure=true but tests pass" {
    create_mock_cargo 10 10 0 0
    run_action action_run_tests "passing" "output.txt" "true"
    [[ "$status" -eq 1 ]]
}

@test "test_run_tests_state_file_not_set: exit 1 with guard clause error" {
    create_mock_cargo 5 5 0 0
    run_action_unset STATE_FILE action_run_tests "pattern" "output.txt" "false"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"ERROR: STATE_FILE not set"* ]]
}

@test "test_run_tests_phase_dir_not_set: exit 1 with guard clause error" {
    create_mock_cargo 5 5 0 0
    run_action_unset PHASE_DIR action_run_tests "pattern" "output.txt" "false"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"ERROR: PHASE_DIR not set"* ]]
}

@test "test_run_tests_phase_dir_not_exist: exit 1 with descriptive error" {
    create_mock_cargo 5 5 0 0
    export PHASE_DIR="$TEST_TEMP_DIR/nonexistent_dir"
    run_action action_run_tests "pattern" "output.txt" "false"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"ERROR: PHASE_DIR does not exist"* ]]
}

@test "test_run_tests_crate_not_found: exit 1 on cargo error" {
    create_mock_cargo_crate_error
    export ARC_DEFAULT_PKG="nonexistent-crate"
    run_action action_run_tests "pattern" "output.txt" "false"
    [[ "$status" -eq 1 ]]
}

@test "test_run_tests_stderr_captured: both stdout and stderr in output file" {
    create_mock_cargo_with_stderr
    run_action action_run_tests "pattern" "output.txt" "false"
    [[ "$status" -eq 0 ]]

    local content
    content=$(cat "$PHASE_DIR/output.txt")
    # stdout content
    [[ "$content" == *"Compiling"* ]]
    # stderr content should also be captured (> file 2>&1 order)
    [[ "$content" == *"warning"* ]]
}

@test "test_run_tests_corrupt_state_json: exit 1 when state.json is invalid" {
    create_mock_cargo 5 5 0 0
    echo "not valid json" > "$STATE_FILE"
    run_action action_run_tests "pattern" "output.txt" "false"
    [[ "$status" -ne 0 ]]
}

@test "test_state_update_atomic: state.json is valid JSON after update" {
    create_mock_cargo 8 6 2 1
    run_action action_run_tests "pattern" "output.txt" "false"

    # State file should still be valid JSON regardless of test pass/fail
    run jq '.' "$STATE_FILE"
    [[ "$status" -eq 0 ]]

    local total
    total=$(jq -r '.tests_total' "$STATE_FILE")
    [[ "$total" -eq 8 ]]

    local passing
    passing=$(jq -r '.tests_passing' "$STATE_FILE")
    [[ "$passing" -eq 6 ]]
}

#=============================================================================
# action_commit Tests
#=============================================================================

@test "test_commit_simple: exit 0 and commit created with changes" {
    create_mock_git 0
    export VERDICT="approved"
    run_action action_commit "test commit"
    [[ "$status" -eq 0 ]]
}

@test "test_commit_simple: state.json updated with last_commit hash" {
    create_mock_git 0
    export VERDICT="approved"
    run_action action_commit "test commit"
    [[ "$status" -eq 0 ]]

    local hash
    hash=$(jq -r '.last_commit' "$STATE_FILE")
    [[ "$hash" == "abc123def456" ]]
}

@test "test_commit_with_when_match: commit created when VERDICT matches" {
    create_mock_git 0
    export VERDICT="approved"
    run_action action_commit "test commit" "approved"
    [[ "$status" -eq 0 ]]

    local hash
    hash=$(jq -r '.last_commit' "$STATE_FILE")
    [[ "$hash" == "abc123def456" ]]
}

@test "test_commit_with_when_no_match: no commit when VERDICT doesn't match" {
    create_mock_git 0
    export VERDICT="needs_fix"
    run_action action_commit "test commit" "approved"
    [[ "$status" -eq 0 ]]

    # last_commit should not exist in state (was never set)
    local hash
    hash=$(jq -r '.last_commit // "null"' "$STATE_FILE")
    [[ "$hash" == "null" ]]
}

@test "test_commit_no_changes: exit 0 with clean working directory" {
    create_mock_git_clean
    run_action action_commit "test commit"
    [[ "$status" -eq 0 ]]
}

@test "test_commit_not_allowed: exit 1 when allow_commits=false" {
    create_mock_git 0
    jq '.allow_commits = false' "$STATE_FILE" > "${STATE_FILE}.tmp" && mv "${STATE_FILE}.tmp" "$STATE_FILE"
    run_action action_commit "test commit"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Commits not allowed"* ]]
}

@test "test_commit_when_verdict_unset_vs_empty: no commit when when='approved' and VERDICT unset" {
    create_mock_git 0
    run_action_unset VERDICT action_commit "test commit" "approved"
    [[ "$status" -eq 0 ]]

    # Should have skipped — no last_commit set
    local hash
    hash=$(jq -r '.last_commit // "null"' "$STATE_FILE")
    [[ "$hash" == "null" ]]
}

@test "test_commit_state_file_not_set: exit 1 with guard clause error" {
    create_mock_git 0
    run_action_unset STATE_FILE action_commit "test message"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"ERROR: STATE_FILE not set"* ]]
}

@test "test_commit_phase_dir_not_set: exit 1 with guard clause error" {
    create_mock_git 0
    run_action_unset PHASE_DIR action_commit "test message"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"ERROR: PHASE_DIR not set"* ]]
}

@test "test_commit_git_not_initialized: exit 1 when not a git repo" {
    create_mock_git_not_repo
    run_action action_commit "test message"
    [[ "$status" -ne 0 ]]
}

@test "test_commit_special_characters: commit with quotes in message" {
    # Create a mock git that records the commit message
    cat > "$MOCK_BIN/git" << 'SCRIPT'
#!/usr/bin/env bash
case "$1" in
    diff)
        exit 1  # changes exist
        ;;
    add)
        exit 0
        ;;
    commit)
        # Just succeed — the shell handles quoting
        exit 0
        ;;
    rev-parse)
        echo "abc123def456"
        exit 0
        ;;
    *)
        exit 0
        ;;
esac
SCRIPT
    chmod +x "$MOCK_BIN/git"

    export VERDICT="approved"
    run_action action_commit "fix: handle \"quotes\" and 'apostrophes'"
    [[ "$status" -eq 0 ]]
}

#=============================================================================
# check_commit_allowed Tests
#=============================================================================

@test "check_commit_allowed: returns 0 when allow_commits not set" {
    run_action check_commit_allowed
    [[ "$status" -eq 0 ]]
}

@test "check_commit_allowed: returns 0 when allow_commits=true" {
    jq '.allow_commits = true' "$STATE_FILE" > "${STATE_FILE}.tmp" && mv "${STATE_FILE}.tmp" "$STATE_FILE"
    run_action check_commit_allowed
    [[ "$status" -eq 0 ]]
}

@test "check_commit_allowed: returns 1 when allow_commits=false" {
    jq '.allow_commits = false' "$STATE_FILE" > "${STATE_FILE}.tmp" && mv "${STATE_FILE}.tmp" "$STATE_FILE"
    run_action check_commit_allowed
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Commits not allowed"* ]]
}

#=============================================================================
# action_switch_model Tests
#=============================================================================

@test "test_switch_model_valid: exit 0 and state updated for opus" {
    run_action action_switch_model "opus"
    [[ "$status" -eq 0 ]]

    local model
    model=$(jq -r '.current_model' "$STATE_FILE")
    [[ "$model" == "opus" ]]
}

@test "test_switch_model_all_valid_values: sonnet works" {
    run_action action_switch_model "sonnet"
    [[ "$status" -eq 0 ]]

    local model
    model=$(jq -r '.current_model' "$STATE_FILE")
    [[ "$model" == "sonnet" ]]
}

@test "test_switch_model_all_valid_values: haiku works" {
    run_action action_switch_model "haiku"
    [[ "$status" -eq 0 ]]

    local model
    model=$(jq -r '.current_model' "$STATE_FILE")
    [[ "$model" == "haiku" ]]
}

@test "test_switch_model_invalid: exit 1 for unrecognized model" {
    run_action action_switch_model "gpt-4"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Invalid model"* ]]
}

@test "test_switch_model_empty_string: exit 1 for empty model name" {
    run_action action_switch_model ""
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Invalid model"* ]]
}

@test "test_switch_model_state_file_not_set: exit 1 with guard clause error" {
    run_action_unset STATE_FILE action_switch_model "opus"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"ERROR: STATE_FILE not set"* ]]
}

@test "test_switch_model_state_file_missing: exit 1 when state file doesn't exist" {
    export STATE_FILE="$TEST_TEMP_DIR/nonexistent_state.json"
    run_action action_switch_model "opus"
    [[ "$status" -ne 0 ]]
}

@test "test_switch_model_preserves_other_fields: existing fields remain after update" {
    jq '.iteration = 5 | .current_state = "impl"' "$STATE_FILE" > "${STATE_FILE}.tmp" && mv "${STATE_FILE}.tmp" "$STATE_FILE"
    run_action action_switch_model "opus"
    [[ "$status" -eq 0 ]]

    local iter
    iter=$(jq -r '.iteration' "$STATE_FILE")
    [[ "$iter" -eq 5 ]]

    local state
    state=$(jq -r '.current_state' "$STATE_FILE")
    [[ "$state" == "impl" ]]
}

#=============================================================================
# action_analyze_stuck Tests
#=============================================================================

@test "test_analyze_stuck: exit 0 and analysis file created at iteration 3" {
    jq '.iteration = 3' "$STATE_FILE" > "${STATE_FILE}.tmp" && mv "${STATE_FILE}.tmp" "$STATE_FILE"

    mkdir -p "$PHASE_DIR/iteration_002"
    mkdir -p "$PHASE_DIR/iteration_003"
    echo "FAIL test_foo" > "$PHASE_DIR/iteration_002/test_output.txt"
    echo "FAIL test_foo" > "$PHASE_DIR/iteration_003/test_output.txt"

    run_action action_analyze_stuck
    [[ "$status" -eq 0 ]]
    [[ -f "$PHASE_DIR/stuck_analysis.md" ]]
}

@test "test_analyze_stuck: state.json updated with stuck_analysis_iteration" {
    jq '.iteration = 3' "$STATE_FILE" > "${STATE_FILE}.tmp" && mv "${STATE_FILE}.tmp" "$STATE_FILE"

    mkdir -p "$PHASE_DIR/iteration_002"
    mkdir -p "$PHASE_DIR/iteration_003"
    echo "FAIL test_foo" > "$PHASE_DIR/iteration_002/test_output.txt"
    echo "FAIL test_foo" > "$PHASE_DIR/iteration_003/test_output.txt"

    run_action action_analyze_stuck
    [[ "$status" -eq 0 ]]

    local stuck_iter
    stuck_iter=$(jq -r '.stuck_analysis_iteration' "$STATE_FILE")
    [[ "$stuck_iter" -eq 3 ]]
}

@test "test_analyze_stuck: analysis contains failure comparison" {
    jq '.iteration = 3' "$STATE_FILE" > "${STATE_FILE}.tmp" && mv "${STATE_FILE}.tmp" "$STATE_FILE"

    mkdir -p "$PHASE_DIR/iteration_002"
    mkdir -p "$PHASE_DIR/iteration_003"
    echo "FAIL test_foo" > "$PHASE_DIR/iteration_002/test_output.txt"
    echo "FAIL test_bar" > "$PHASE_DIR/iteration_003/test_output.txt"

    run_action action_analyze_stuck
    [[ "$status" -eq 0 ]]

    local content
    content=$(cat "$PHASE_DIR/stuck_analysis.md")
    [[ "$content" == *"Stuck Analysis"* ]]
    [[ "$content" == *"iteration 3"* ]]
    [[ "$content" == *"FAIL"* ]]
}

@test "test_analyze_stuck_insufficient_iterations: exit 0 with no analysis at iteration 1" {
    jq '.iteration = 1' "$STATE_FILE" > "${STATE_FILE}.tmp" && mv "${STATE_FILE}.tmp" "$STATE_FILE"

    run_action action_analyze_stuck
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Not enough iterations"* ]]
}

@test "test_analyze_stuck_missing_test_outputs: exit 0 with not found message" {
    jq '.iteration = 3' "$STATE_FILE" > "${STATE_FILE}.tmp" && mv "${STATE_FILE}.tmp" "$STATE_FILE"

    # Create directories but no test_output.txt files
    mkdir -p "$PHASE_DIR/iteration_002"
    mkdir -p "$PHASE_DIR/iteration_003"

    run_action action_analyze_stuck
    [[ "$status" -eq 0 ]]
    [[ -f "$PHASE_DIR/stuck_analysis.md" ]]

    local content
    content=$(cat "$PHASE_DIR/stuck_analysis.md")
    [[ "$content" == *"not found"* ]] || [[ "$content" == *"Not found"* ]] || [[ "$content" == *"not found"* ]]
}

@test "test_analyze_stuck_no_test_outputs: analysis still generated without test output files" {
    jq '.iteration = 3' "$STATE_FILE" > "${STATE_FILE}.tmp" && mv "${STATE_FILE}.tmp" "$STATE_FILE"

    mkdir -p "$PHASE_DIR/iteration_002"
    mkdir -p "$PHASE_DIR/iteration_003"

    run_action action_analyze_stuck
    [[ "$status" -eq 0 ]]
    [[ -f "$PHASE_DIR/stuck_analysis.md" ]]

    local content
    content=$(cat "$PHASE_DIR/stuck_analysis.md")
    [[ "$content" == *"Recommendations"* ]]
}

@test "test_analyze_stuck_iteration_boundary: works at minimum iteration=2" {
    jq '.iteration = 2' "$STATE_FILE" > "${STATE_FILE}.tmp" && mv "${STATE_FILE}.tmp" "$STATE_FILE"

    mkdir -p "$PHASE_DIR/iteration_001"
    mkdir -p "$PHASE_DIR/iteration_002"
    echo "FAIL test_old" > "$PHASE_DIR/iteration_001/test_output.txt"
    echo "FAIL test_new" > "$PHASE_DIR/iteration_002/test_output.txt"

    run_action action_analyze_stuck
    [[ "$status" -eq 0 ]]
    [[ -f "$PHASE_DIR/stuck_analysis.md" ]]

    local stuck_iter
    stuck_iter=$(jq -r '.stuck_analysis_iteration' "$STATE_FILE")
    [[ "$stuck_iter" -eq 2 ]]
}

@test "test_analyze_stuck_phase_dir_not_set: exit 1 with guard clause error" {
    run_action_unset PHASE_DIR action_analyze_stuck
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"ERROR: PHASE_DIR not set"* ]]
}

@test "test_analyze_stuck_state_file_not_set: exit 1 with guard clause error" {
    run_action_unset STATE_FILE action_analyze_stuck
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"ERROR: STATE_FILE not set"* ]]
}

#=============================================================================
# action_request_human Tests
#=============================================================================

@test "test_request_human: exit 0 and intervention file created" {
    run_action action_request_human "Tests consistently failing on edge case X"
    [[ "$status" -eq 0 ]]
    [[ -f "$PHASE_DIR/intervention_request.md" ]]
}

@test "test_request_human: state.json updated with intervention_request object" {
    run_action action_request_human "Tests consistently failing on edge case X"
    [[ "$status" -eq 0 ]]

    # Check intervention_request is an object with required fields
    local reason
    reason=$(jq -r '.intervention_request.reason' "$STATE_FILE")
    [[ "$reason" == "Tests consistently failing on edge case X" ]]

    local requested_at
    requested_at=$(jq -r '.intervention_request.requested_at' "$STATE_FILE")
    [[ "$requested_at" != "null" ]]
    # Should be ISO8601 format
    [[ "$requested_at" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T ]]

    # Should have options array
    local options_count
    options_count=$(jq '.intervention_request.options | length' "$STATE_FILE")
    [[ "$options_count" -gt 0 ]]
}

@test "test_request_human_content: intervention file contains expected fields" {
    jq '.iteration = 5 | .current_state = "impl"' "$STATE_FILE" > "${STATE_FILE}.tmp" && mv "${STATE_FILE}.tmp" "$STATE_FILE"

    run_action action_request_human "Help needed"
    [[ "$status" -eq 0 ]]

    local content
    content=$(cat "$PHASE_DIR/intervention_request.md")
    # Contains timestamp
    [[ "$content" == *"Timestamp"* ]]
    # Contains message
    [[ "$content" == *"Help needed"* ]]
    # Contains phase name
    [[ "$content" == *"phase"* ]]
    # Contains iteration
    [[ "$content" == *"5"* ]]
}

@test "test_request_human_special_characters: handles quotes and backticks" {
    run_action action_request_human 'Tests failing: "edge case" with special chars'
    [[ "$status" -eq 0 ]]
    [[ -f "$PHASE_DIR/intervention_request.md" ]]

    local content
    content=$(cat "$PHASE_DIR/intervention_request.md")
    [[ "$content" == *"edge case"* ]]
}

@test "test_request_human_phase_dir_not_set: exit 1 with guard clause error" {
    run_action_unset PHASE_DIR action_request_human "test message"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"ERROR: PHASE_DIR not set"* ]]
}

@test "test_request_human_state_file_not_set: exit 1 with guard clause error" {
    run_action_unset STATE_FILE action_request_human "test message"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"ERROR: STATE_FILE not set"* ]]
}

@test "test_request_human_corrupt_state_json: exit 1 on invalid state file" {
    echo "not valid json" > "$STATE_FILE"
    run_action action_request_human "test message"
    [[ "$status" -ne 0 ]]
}

#=============================================================================
# action_script Tests
#=============================================================================

@test "test_script_exists: execute script with args" {
    cat > "$ARC_HOME/scripts/test-script.sh" << 'SCRIPT'
#!/usr/bin/env bash
echo "args: $@"
SCRIPT
    chmod +x "$ARC_HOME/scripts/test-script.sh"

    run_action action_script "scripts/test-script.sh" "arg1" "arg2"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"arg1"* ]]
    [[ "$output" == *"arg2"* ]]
}

@test "test_script_not_found: exit 1 for missing script" {
    run_action action_script "scripts/nonexistent.sh"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Script not found"* ]]
}

@test "test_script_not_executable: exit 1 for non-executable script" {
    cat > "$ARC_HOME/scripts/not-executable.sh" << 'SCRIPT'
#!/usr/bin/env bash
echo "hello"
SCRIPT
    # Do NOT chmod +x

    run_action action_script "scripts/not-executable.sh"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"not executable"* ]]
}

@test "test_script_non_zero_exit: propagates script exit code" {
    cat > "$ARC_HOME/scripts/failing-script.sh" << 'SCRIPT'
#!/usr/bin/env bash
exit 5
SCRIPT
    chmod +x "$ARC_HOME/scripts/failing-script.sh"

    run_action action_script "scripts/failing-script.sh"
    [[ "$status" -eq 5 ]]
}

@test "test_script_empty_args: executes with no additional arguments" {
    cat > "$ARC_HOME/scripts/test-script.sh" << 'SCRIPT'
#!/usr/bin/env bash
echo "argc=$#"
SCRIPT
    chmod +x "$ARC_HOME/scripts/test-script.sh"

    run_action action_script "scripts/test-script.sh"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"argc=0"* ]]
}

@test "test_script_spaces_in_path: handles spaces in script path" {
    mkdir -p "$ARC_HOME/scripts/sub dir"
    cat > "$ARC_HOME/scripts/sub dir/my script.sh" << 'SCRIPT'
#!/usr/bin/env bash
echo "ran: $@"
SCRIPT
    chmod +x "$ARC_HOME/scripts/sub dir/my script.sh"

    run_action action_script "scripts/sub dir/my script.sh" "arg1"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"ran: arg1"* ]]
}

@test "test_script_path_traversal_rejected: blocks ../ in path" {
    run_action action_script "../../etc/passwd"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"cannot contain '..'"* ]]
}

@test "test_script_absolute_path_rejected: blocks absolute paths" {
    run_action action_script "/etc/passwd"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"must be relative"* ]]
}

@test "test_script_orchestration_dir_not_set: exit 1 with guard clause error" {
    run_action_unset ARC_HOME action_script "scripts/test.sh"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"ERROR: ARC_HOME not set"* ]]
}

#=============================================================================
# Cross-cutting Concerns
#=============================================================================

@test "all action functions defined after sourcing" {
    run bash -c "
        source '$ACTIONS_SH'
        declare -f action_run_tests > /dev/null && echo 'run_tests:ok'
        declare -f action_commit > /dev/null && echo 'commit:ok'
        declare -f action_switch_model > /dev/null && echo 'switch_model:ok'
        declare -f action_analyze_stuck > /dev/null && echo 'analyze_stuck:ok'
        declare -f action_request_human > /dev/null && echo 'request_human:ok'
        declare -f action_script > /dev/null && echo 'script:ok'
        declare -f check_commit_allowed > /dev/null && echo 'check_commit:ok'
    "
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"run_tests:ok"* ]]
    [[ "$output" == *"commit:ok"* ]]
    [[ "$output" == *"switch_model:ok"* ]]
    [[ "$output" == *"analyze_stuck:ok"* ]]
    [[ "$output" == *"request_human:ok"* ]]
    [[ "$output" == *"script:ok"* ]]
    [[ "$output" == *"check_commit:ok"* ]]
}

@test "state.json remains valid JSON after multiple action calls" {
    create_mock_cargo 10 10 0 0
    run_action action_run_tests "pattern" "output.txt" "false"
    [[ "$status" -eq 0 ]]

    run_action action_switch_model "opus"
    [[ "$status" -eq 0 ]]

    # State file should still be valid JSON
    run jq '.' "$STATE_FILE"
    [[ "$status" -eq 0 ]]

    # Both fields should be present
    local model
    model=$(jq -r '.current_model' "$STATE_FILE")
    [[ "$model" == "opus" ]]

    local total
    total=$(jq -r '.tests_total' "$STATE_FILE")
    [[ "$total" -eq 10 ]]
}
