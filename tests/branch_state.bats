#!/usr/bin/env bats
# QA Tests for branch-state-tracking phase (orchestration-v5)
#
# Tests the parallel state management commands added to update-state.sh:
# - parallel-start: record parallel execution start with branch names
# - parallel-update: update single branch status/exit_code
# - parallel-finish: record parallel completion verdict
# - parallel-clear: remove .parallel_execution from state.json

setup() {
    load 'test_helper'
    setup_temp_dir

    UPDATE_STATE="$SCRIPTS_DIR/update-state.sh"

    # Use temp directory as PLANS_DIR to isolate from real plans
    export PLANS_DIR="$TEST_TEMP_DIR/.plans"
    TEST_PLAN="test-plan"
    TEST_PHASE="test-phase"
    PHASE_DIR="$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE"
    STATE_FILE="$PHASE_DIR/state.json"
    mkdir -p "$PHASE_DIR"

    # Create minimal valid state.json
    cat > "$STATE_FILE" << 'JSON'
{
    "plan": "test-plan",
    "phase": "test-phase",
    "phase_status": "implementing",
    "iteration": {"current": 1, "max": 25},
    "tests_passing": 0,
    "tests_total": 0
}
JSON
}

teardown() {
    teardown_temp_dir
}

# Helper: run update-state.sh with test plan/phase
run_update() {
    run "$UPDATE_STATE" "$TEST_PLAN" "$TEST_PHASE" "$@"
}

# Helper: read a jq path from state.json (raw string output)
state_field() {
    jq -r "$1" "$STATE_FILE"
}

# Helper: read a jq path from state.json (raw jq output, preserves types)
state_raw() {
    jq "$1" "$STATE_FILE"
}

# Helper: start parallel with given branches, assert success
start_parallel() {
    run_update parallel-start /tmp/results "$1"
    [[ "$status" -eq 0 ]]
}

# ==============================================================================
# Script Preconditions
# ==============================================================================

@test "qa_branch-state-tracking_script_exists: update-state.sh exists and is executable" {
    [[ -x "$UPDATE_STATE" ]]
}

@test "qa_branch-state-tracking_syntax_valid: update-state.sh is valid bash" {
    run bash -n "$UPDATE_STATE"
    [[ "$status" -eq 0 ]]
}

# ==============================================================================
# parallel-start Tests
# ==============================================================================

@test "qa_branch-state-tracking_test_parallel_start: creates branches map with correct keys" {
    run_update parallel-start /tmp/results "auth,api,db"
    [[ "$status" -eq 0 ]]

    # Should have 3 branches
    local count
    count=$(state_raw '.parallel_execution.branches | keys | length')
    [[ "$count" -eq 3 ]]

    # All branches should be "running"
    [[ "$(state_field '.parallel_execution.branches.auth.status')" == "running" ]]
    [[ "$(state_field '.parallel_execution.branches.api.status')" == "running" ]]
    [[ "$(state_field '.parallel_execution.branches.db.status')" == "running" ]]
}

@test "qa_branch-state-tracking_test_parallel_start_timestamp: started_at is ISO8601" {
    run_update parallel-start /tmp/results "a,b"
    [[ "$status" -eq 0 ]]

    local ts
    ts=$(state_field '.parallel_execution.started_at')
    [[ -n "$ts" ]]
    [[ "$ts" != "null" ]]
    # ISO8601 with timezone offset (date -Iseconds format)
    [[ "$ts" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2} ]]
}

@test "qa_branch-state-tracking_test_parallel_start_results_dir: stores results_dir path" {
    run_update parallel-start /tmp/results "a,b"
    [[ "$status" -eq 0 ]]

    [[ "$(state_field '.parallel_execution.results_dir')" == "/tmp/results" ]]
}

@test "qa_branch-state-tracking_test_parallel_start_branch_started_at: each branch has started_at" {
    run_update parallel-start /tmp/results "a,b"
    [[ "$status" -eq 0 ]]

    local ts_a ts_b
    ts_a=$(state_field '.parallel_execution.branches.a.started_at')
    ts_b=$(state_field '.parallel_execution.branches.b.started_at')
    [[ -n "$ts_a" ]]
    [[ "$ts_a" != "null" ]]
    [[ -n "$ts_b" ]]
    [[ "$ts_b" != "null" ]]
}

@test "qa_branch-state-tracking_test_parallel_start_clears_previous: replaces existing parallel_execution" {
    # First start
    run_update parallel-start /tmp/old "old_branch"
    [[ "$status" -eq 0 ]]

    # Verify old data exists
    [[ "$(state_field '.parallel_execution.results_dir')" == "/tmp/old" ]]

    # Second start should replace
    run_update parallel-start /tmp/new "new_branch"
    [[ "$status" -eq 0 ]]

    [[ "$(state_field '.parallel_execution.results_dir')" == "/tmp/new" ]]
    # old_branch should be gone
    [[ "$(state_raw '.parallel_execution.branches.old_branch')" == "null" ]]
    # new_branch should exist
    [[ "$(state_field '.parallel_execution.branches.new_branch.status')" == "running" ]]
}

@test "qa_branch-state-tracking_test_single_branch: single branch is valid" {
    run_update parallel-start /tmp/results "only_one"
    [[ "$status" -eq 0 ]]

    local count
    count=$(state_raw '.parallel_execution.branches | keys | length')
    [[ "$count" -eq 1 ]]
    [[ "$(state_field '.parallel_execution.branches.only_one.status')" == "running" ]]
}

@test "qa_branch-state-tracking_test_branch_names_with_hyphens: hyphenated names work" {
    run_update parallel-start /tmp/results "auth-module,api-gateway"
    [[ "$status" -eq 0 ]]

    [[ "$(state_field '.parallel_execution.branches["auth-module"].status')" == "running" ]]
    [[ "$(state_field '.parallel_execution.branches["api-gateway"].status')" == "running" ]]
}

@test "qa_branch-state-tracking_test_parallel_start_empty_csv: rejects empty CSV" {
    run_update parallel-start /tmp/results ""
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"empty"* ]] || [[ "${stderr:-}" == *"empty"* ]]
}

@test "qa_branch-state-tracking_test_parallel_start_double_comma: rejects double comma" {
    run_update parallel-start /tmp/results "a,,b"
    [[ "$status" -eq 1 ]]
    # Must fail because of empty branch name, not because command is unknown
    [[ "$output" != *"Unknown command"* ]]
}

@test "qa_branch-state-tracking_test_parallel_start_trailing_comma: rejects trailing comma" {
    run_update parallel-start /tmp/results "a,b,"
    [[ "$status" -eq 1 ]]
    # Must fail because of empty branch name, not because command is unknown
    [[ "$output" != *"Unknown command"* ]]
}

@test "qa_branch-state-tracking_test_parallel_start_whitespace_trimming: trims whitespace from names" {
    run_update parallel-start /tmp/results " auth , api "
    [[ "$status" -eq 0 ]]

    local count
    count=$(state_raw '.parallel_execution.branches | keys | length')
    [[ "$count" -eq 2 ]]
    [[ "$(state_field '.parallel_execution.branches.auth.status')" == "running" ]]
    [[ "$(state_field '.parallel_execution.branches.api.status')" == "running" ]]
}

@test "qa_branch-state-tracking_test_parallel_start_whitespace_only_name: rejects whitespace-only name" {
    run_update parallel-start /tmp/results "auth, ,api"
    [[ "$status" -eq 1 ]]
    # Must fail because of empty branch name, not because command is unknown
    [[ "$output" != *"Unknown command"* ]]
}

@test "qa_branch-state-tracking_test_parallel_start_duplicate_csv_names: deduplicates silently" {
    run_update parallel-start /tmp/results "a,b,a"
    [[ "$status" -eq 0 ]]

    local count
    count=$(state_raw '.parallel_execution.branches | keys | length')
    [[ "$count" -eq 2 ]]
    [[ "$(state_field '.parallel_execution.branches.a.status')" == "running" ]]
    [[ "$(state_field '.parallel_execution.branches.b.status')" == "running" ]]
}

# ==============================================================================
# parallel-update Tests
# ==============================================================================

@test "qa_branch-state-tracking_test_parallel_update_complete: sets status and exit_code" {
    start_parallel "auth,api,db"

    run_update parallel-update auth complete 0
    [[ "$status" -eq 0 ]]

    [[ "$(state_field '.parallel_execution.branches.auth.status')" == "complete" ]]
    [[ "$(state_raw '.parallel_execution.branches.auth.exit_code')" == "0" ]]
}

@test "qa_branch-state-tracking_test_parallel_update_failed: sets failed status with exit_code" {
    start_parallel "auth,api,db"

    run_update parallel-update api failed 1
    [[ "$status" -eq 0 ]]

    [[ "$(state_field '.parallel_execution.branches.api.status')" == "failed" ]]
    [[ "$(state_raw '.parallel_execution.branches.api.exit_code')" == "1" ]]
}

@test "qa_branch-state-tracking_test_parallel_update_timeout: sets timeout status with exit_code" {
    start_parallel "auth,api,db"

    run_update parallel-update db timeout 124
    [[ "$status" -eq 0 ]]

    [[ "$(state_field '.parallel_execution.branches.db.status')" == "timeout" ]]
    [[ "$(state_raw '.parallel_execution.branches.db.exit_code')" == "124" ]]
}

@test "qa_branch-state-tracking_test_parallel_update_sets_completed_at: non-running status gets completed_at" {
    start_parallel "auth,api"

    run_update parallel-update auth complete 0
    [[ "$status" -eq 0 ]]

    local ts
    ts=$(state_field '.parallel_execution.branches.auth.completed_at')
    [[ -n "$ts" ]]
    [[ "$ts" != "null" ]]
    [[ "$ts" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2} ]]
}

@test "qa_branch-state-tracking_test_parallel_update_running_no_completed_at: running branches have no completed_at" {
    start_parallel "auth,api"

    # Check that branches in running state don't have completed_at
    local completed_at
    completed_at=$(state_raw '.parallel_execution.branches.auth.completed_at')
    [[ "$completed_at" == "null" ]]
}

@test "qa_branch-state-tracking_test_parallel_update_no_exit_code: exit_code absent when omitted" {
    start_parallel "auth,api"

    # Update with no exit_code argument
    run_update parallel-update auth complete
    [[ "$status" -eq 0 ]]

    [[ "$(state_field '.parallel_execution.branches.auth.status')" == "complete" ]]
    # exit_code should be absent (null in jq)
    [[ "$(state_raw '.parallel_execution.branches.auth.exit_code')" == "null" ]]
    # completed_at should be set
    local ts
    ts=$(state_field '.parallel_execution.branches.auth.completed_at')
    [[ -n "$ts" ]]
    [[ "$ts" != "null" ]]
}

@test "qa_branch-state-tracking_test_parallel_update_exit_code_is_integer: stored as number not string" {
    start_parallel "auth"

    run_update parallel-update auth complete 42
    [[ "$status" -eq 0 ]]

    # jq type check: exit_code should be number
    local type
    type=$(jq -r '.parallel_execution.branches.auth.exit_code | type' "$STATE_FILE")
    [[ "$type" == "number" ]]
    [[ "$(state_raw '.parallel_execution.branches.auth.exit_code')" == "42" ]]
}

@test "qa_branch-state-tracking_test_update_nonexistent_branch: rejects unknown branch name" {
    start_parallel "auth,api"

    run_update parallel-update nonexistent complete 0
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"not found"* ]]
}

@test "qa_branch-state-tracking_test_parallel_update_before_start: errors without parallel_execution" {
    # No parallel-start called, state.json has no .parallel_execution
    run_update parallel-update auth complete 0
    [[ "$status" -eq 1 ]]
    [[ "$output" != *"Unknown command"* ]]
    [[ "$output" == *"no parallel execution"* ]]
}

@test "qa_branch-state-tracking_test_parallel_update_invalid_status: rejects invalid status value" {
    start_parallel "auth"

    run_update parallel-update auth canceled 0
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"invalid status"* ]]
}

@test "qa_branch-state-tracking_test_parallel_update_back_to_running: cannot reset to running" {
    start_parallel "auth"

    run_update parallel-update auth complete 0
    [[ "$status" -eq 0 ]]

    # Try to reset back to running
    run_update parallel-update auth running
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"cannot reset branch to running"* ]]
}

@test "qa_branch-state-tracking_test_parallel_update_failed_to_complete: non-running transitions allowed" {
    start_parallel "auth"

    # Set to failed first
    run_update parallel-update auth failed 1
    [[ "$status" -eq 0 ]]
    [[ "$(state_field '.parallel_execution.branches.auth.status')" == "failed" ]]

    # Transition from failed to complete (allowed — only "running" reset is rejected)
    run_update parallel-update auth complete 0
    [[ "$status" -eq 0 ]]
    [[ "$(state_field '.parallel_execution.branches.auth.status')" == "complete" ]]
    [[ "$(state_raw '.parallel_execution.branches.auth.exit_code')" == "0" ]]
}

@test "qa_branch-state-tracking_test_parallel_update_empty_branch_name: rejects empty name" {
    start_parallel "auth"

    run_update parallel-update "" complete 0
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"not found"* ]]
}

@test "qa_branch-state-tracking_test_parallel_update_non_integer_exit_code: rejects non-numeric exit_code" {
    start_parallel "auth"

    run_update parallel-update auth complete abc
    [[ "$status" -eq 1 ]]
    # jq --argjson will reject non-numeric value
}

# ==============================================================================
# parallel-finish Tests
# ==============================================================================

@test "qa_branch-state-tracking_test_parallel_finish: sets verdict and last_verdict" {
    start_parallel "auth,api"

    run_update parallel-finish all_complete
    [[ "$status" -eq 0 ]]

    [[ "$(state_field '.parallel_execution.verdict')" == "all_complete" ]]
    [[ "$(state_field '.last_verdict')" == "all_complete" ]]
}

@test "qa_branch-state-tracking_test_parallel_finish_timestamp: completed_at is ISO8601" {
    start_parallel "auth"

    run_update parallel-finish any_failed
    [[ "$status" -eq 0 ]]

    local ts
    ts=$(state_field '.parallel_execution.completed_at')
    [[ -n "$ts" ]]
    [[ "$ts" != "null" ]]
    [[ "$ts" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2} ]]
}

@test "qa_branch-state-tracking_test_parallel_finish_before_start: errors without parallel_execution" {
    # No parallel-start called
    run_update parallel-finish all_complete
    [[ "$status" -eq 1 ]]
    [[ "$output" != *"Unknown command"* ]]
    [[ "$output" == *"no parallel execution"* ]]
}

@test "qa_branch-state-tracking_test_parallel_finish_with_running_branches: allowed even if branches still running" {
    start_parallel "auth,api"

    # Only update one branch
    run_update parallel-update auth complete 0
    [[ "$status" -eq 0 ]]

    # parallel-finish should still succeed (api is still running)
    run_update parallel-finish all_complete
    [[ "$status" -eq 0 ]]

    [[ "$(state_field '.parallel_execution.verdict')" == "all_complete" ]]
}

@test "qa_branch-state-tracking_test_parallel_finish_sets_last_verdict_at_root: last_verdict at state root" {
    start_parallel "auth"

    run_update parallel-finish all_complete
    [[ "$status" -eq 0 ]]

    # .last_verdict should be at the root level of state.json, not nested
    local root_verdict
    root_verdict=$(state_field '.last_verdict')
    [[ "$root_verdict" == "all_complete" ]]

    # Verify it's at root, not only inside parallel_execution
    local root_keys
    root_keys=$(jq 'keys' "$STATE_FILE")
    [[ "$root_keys" == *"last_verdict"* ]]
}

@test "qa_branch-state-tracking_test_parallel_finish_empty_verdict: accepts empty string verdict" {
    start_parallel "auth"

    run_update parallel-finish ""
    [[ "$status" -eq 0 ]]

    [[ "$(state_field '.parallel_execution.verdict')" == "" ]]
    [[ "$(state_field '.last_verdict')" == "" ]]
}

# ==============================================================================
# parallel-clear Tests
# ==============================================================================

@test "qa_branch-state-tracking_test_parallel_clear: removes parallel_execution from state" {
    start_parallel "auth,api"

    # Verify it exists
    [[ "$(state_raw '.parallel_execution')" != "null" ]]

    run_update parallel-clear
    [[ "$status" -eq 0 ]]

    # Should be completely gone
    [[ "$(state_raw '.parallel_execution')" == "null" ]]
}

@test "qa_branch-state-tracking_test_parallel_clear_noop: no-op when parallel_execution absent" {
    # No parallel-start was called, so .parallel_execution doesn't exist
    [[ "$(state_raw '.parallel_execution')" == "null" ]]

    run_update parallel-clear
    [[ "$status" -eq 0 ]]
    # Must not fail with "Unknown command" — the command must be recognized
    [[ "$output" != *"Unknown command"* ]]
}

@test "qa_branch-state-tracking_test_parallel_clear_after_full_execution: clears after complete workflow" {
    # Full workflow: start -> update all -> finish -> clear
    start_parallel "auth,api"

    run_update parallel-update auth complete 0
    [[ "$status" -eq 0 ]]
    run_update parallel-update api complete 0
    [[ "$status" -eq 0 ]]
    run_update parallel-finish all_complete
    [[ "$status" -eq 0 ]]

    # Verify everything is set
    [[ "$(state_field '.parallel_execution.verdict')" == "all_complete" ]]
    [[ "$(state_field '.last_verdict')" == "all_complete" ]]

    # Clear
    run_update parallel-clear
    [[ "$status" -eq 0 ]]

    [[ "$(state_raw '.parallel_execution')" == "null" ]]
    # last_verdict should still be set (it's at root, not inside parallel_execution)
    [[ "$(state_field '.last_verdict')" == "all_complete" ]]
}

# ==============================================================================
# State Preservation Tests
# ==============================================================================

@test "qa_branch-state-tracking_preserves_existing_state: parallel commands don't corrupt other fields" {
    start_parallel "auth"

    # Verify existing state fields are preserved
    [[ "$(state_field '.plan')" == "test-plan" ]]
    [[ "$(state_field '.phase')" == "test-phase" ]]
    [[ "$(state_field '.phase_status')" == "implementing" ]]
    [[ "$(state_raw '.iteration.current')" == "1" ]]
}

@test "qa_branch-state-tracking_parallel_update_preserves_other_branches: updating one branch doesn't affect others" {
    start_parallel "auth,api,db"

    # Update only auth
    run_update parallel-update auth complete 0
    [[ "$status" -eq 0 ]]

    # api and db should still be running
    [[ "$(state_field '.parallel_execution.branches.api.status')" == "running" ]]
    [[ "$(state_field '.parallel_execution.branches.db.status')" == "running" ]]
}

# ==============================================================================
# Command Recognition Tests
# ==============================================================================

@test "qa_branch-state-tracking_unknown_command_still_fails: unknown commands rejected" {
    run_update parallel-nonexistent
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Unknown command"* ]]
}
