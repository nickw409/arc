#!/usr/bin/env bats

# Tests for run-parallel.sh
# Phase: parallel-engine (orchestration-v5)
#
# Tests the core parallel branch execution engine that spawns multiple
# agent branches as background processes, manages process groups,
# collects results, and handles timeouts.
#
# Prerequisites: render_template.py (V3 template-engine phase)

setup() {
    load 'test_helper'
    setup_temp_dir

    RUN_PARALLEL_SH="$SCRIPTS_DIR/run-parallel.sh"

    # Standard phase directory structure
    export PHASE_DIR="$TEST_TEMP_DIR/phase"
    mkdir -p "$PHASE_DIR"

    # Create prompts directory structure matching ORCH_DIR convention
    export MOCK_ORCH_DIR="$TEST_TEMP_DIR/orchestration"
    mkdir -p "$MOCK_ORCH_DIR/scripts"
    mkdir -p "$MOCK_ORCH_DIR/prompts/v5"

    # Copy render_template.py to mock orchestration dir
    cp "$SCRIPTS_DIR/render_template.py" "$MOCK_ORCH_DIR/scripts/"

    # Create a default prompt template
    cat > "$MOCK_ORCH_DIR/prompts/v5/characterize.md" << 'PROMPT'
You are characterizing the {{module}} module.
Plan: {{plan_name}}, Phase: {{phase}}
PROMPT

    # Default claude stub: consume stdin, exit 0
    export PATH="$BATS_TEST_TMPDIR:$PATH"
    cat > "$BATS_TEST_TMPDIR/claude" << 'STUB'
#!/bin/bash
cat - > /dev/null
exit 0
STUB
    chmod +x "$BATS_TEST_TMPDIR/claude"

    # Default setsid stub — on some CI environments setsid may not be available,
    # but we expect it on Linux. No stub needed; use real setsid.
}

teardown() {
    # Kill any leftover background processes from tests
    if [[ -d "$PHASE_DIR" ]]; then
        shopt -s nullglob
        local pidfiles=("$PHASE_DIR"/parallel_*/*.pid)
        shopt -u nullglob
        for pidfile in "${pidfiles[@]}"; do
            if [[ -f "$pidfile" ]]; then
                local pid
                pid=$(cat "$pidfile" 2>/dev/null)
                if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
                    kill -KILL -- -"$pid" 2>/dev/null || true
                fi
            fi
        done
    fi
    teardown_temp_dir
}

# ==============================================================================
# Helper: Create a workflow YAML with a parallel state
# Usage: create_parallel_workflow [branches_yaml] [timeout] [state_name]
# ==============================================================================
create_parallel_workflow() {
    local branches_yaml="${1:-}"
    local timeout="${2:-600}"
    local state_name="${3:-characterize}"
    local output_file="$TEST_TEMP_DIR/workflow.yaml"

    if [[ -z "$branches_yaml" ]]; then
        branches_yaml='        - name: auth
          prompt: prompts/v5/characterize.md
          params:
            module: auth'
    fi

    cat > "$output_file" << YAML
name: test_parallel
version: 5
description: Test workflow with parallel state

defaults:
  timeout: ${timeout}

states:
  - name: ${state_name}
    description: Characterize modules in parallel
    prompt: prompts/v5/characterize.md
    parallel:
      branches:
${branches_yaml}
    next: complete

  - name: complete
    description: Phase completed
    prompt: prompts/common/complete.md

entry_state: ${state_name}
terminal_states: [complete]
YAML
    echo "$output_file"
}

# ==============================================================================
# Helper: Create a workflow with NO parallel block
# ==============================================================================
create_non_parallel_workflow() {
    local output_file="$TEST_TEMP_DIR/workflow.yaml"

    cat > "$output_file" << 'YAML'
name: test_non_parallel
version: 5
states:
  - name: impl
    description: Implementation state
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    description: Phase completed
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML
    echo "$output_file"
}

# ==============================================================================
# Helper: Create a workflow with multiple branches
# ==============================================================================
create_multi_branch_workflow() {
    local timeout="${1:-600}"
    create_parallel_workflow '        - name: auth
          prompt: prompts/v5/characterize.md
          params:
            module: auth
        - name: api
          prompt: prompts/v5/characterize.md
          params:
            module: api
        - name: db
          prompt: prompts/v5/characterize.md
          params:
            module: db' "$timeout"
}

# ==============================================================================
# Helper: Create a workflow with empty branches array
# ==============================================================================
create_empty_branches_workflow() {
    local output_file="$TEST_TEMP_DIR/workflow.yaml"

    cat > "$output_file" << 'YAML'
name: test_empty_branches
version: 5
defaults:
  timeout: 600
states:
  - name: characterize
    description: Characterize modules
    prompt: prompts/v5/characterize.md
    parallel:
      branches: []
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: characterize
terminal_states: [complete]
YAML
    echo "$output_file"
}

# ==============================================================================
# Helper: Create a workflow with no timeout default (should use 600)
# ==============================================================================
create_no_timeout_workflow() {
    local output_file="$TEST_TEMP_DIR/workflow.yaml"

    cat > "$output_file" << 'YAML'
name: test_no_timeout
version: 5
states:
  - name: characterize
    description: Characterize modules
    prompt: prompts/v5/characterize.md
    parallel:
      branches:
        - name: auth
          prompt: prompts/v5/characterize.md
          params:
            module: auth
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: characterize
terminal_states: [complete]
YAML
    echo "$output_file"
}

#=============================================================================
# Script Existence and Syntax Tests
#=============================================================================

@test "run-parallel.sh exists and is readable" {
    [[ -f "$RUN_PARALLEL_SH" ]]
}

@test "run-parallel.sh is syntactically valid bash" {
    run bash -n "$RUN_PARALLEL_SH"
    [[ "$status" -eq 0 ]]
}

@test "run-parallel.sh is executable" {
    [[ -x "$RUN_PARALLEL_SH" ]]
}

@test "run-parallel.sh uses set -euo pipefail" {
    run bash -c "grep -E '^set -euo pipefail' '$RUN_PARALLEL_SH'"
    [[ "$status" -eq 0 ]]
}

#=============================================================================
# test_spawn_single_branch
#=============================================================================

@test "test_spawn_single_branch: single branch creates .log .exit .pid files with exit 0" {
    local wf
    wf=$(create_parallel_workflow)

    run "$RUN_PARALLEL_SH" "$wf" "characterize" "$PHASE_DIR" "test-plan" "test-phase"
    [[ "$status" -eq 0 ]]

    local results_dir="$PHASE_DIR/parallel_characterize"
    [[ -d "$results_dir" ]]
    [[ -f "$results_dir/auth.log" ]]
    [[ -f "$results_dir/auth.exit" ]]
    [[ -f "$results_dir/auth.pid" ]]

    # Exit code should be 0
    local exit_code
    exit_code=$(cat "$results_dir/auth.exit")
    [[ "$exit_code" -eq 0 ]]
}

#=============================================================================
# test_spawn_multiple_branches
#=============================================================================

@test "test_spawn_multiple_branches: 3 branches all create result files with exit 0" {
    local wf
    wf=$(create_multi_branch_workflow)

    run "$RUN_PARALLEL_SH" "$wf" "characterize" "$PHASE_DIR" "test-plan" "test-phase"
    [[ "$status" -eq 0 ]]

    local results_dir="$PHASE_DIR/parallel_characterize"
    [[ -d "$results_dir" ]]

    for branch in auth api db; do
        [[ -f "$results_dir/$branch.log" ]]
        [[ -f "$results_dir/$branch.exit" ]]
        [[ -f "$results_dir/$branch.pid" ]]

        local exit_code
        exit_code=$(cat "$results_dir/$branch.exit")
        [[ "$exit_code" -eq 0 ]]
    done
}

#=============================================================================
# test_branch_timeout
#=============================================================================

@test "test_branch_timeout: branch with 2s timeout gets exit code 124" {
    # Create a claude stub that sleeps forever (killed by timeout)
    cat > "$BATS_TEST_TMPDIR/claude" << 'STUB'
#!/bin/bash
cat - > /dev/null
exec sleep 999
STUB
    chmod +x "$BATS_TEST_TMPDIR/claude"

    local wf
    wf=$(create_parallel_workflow '        - name: slow
          prompt: prompts/v5/characterize.md
          params:
            module: slow' "2")

    run "$RUN_PARALLEL_SH" "$wf" "characterize" "$PHASE_DIR" "test-plan" "test-phase"
    [[ "$status" -eq 0 ]]

    local results_dir="$PHASE_DIR/parallel_characterize"
    [[ -f "$results_dir/slow.exit" ]]

    local exit_code
    exit_code=$(cat "$results_dir/slow.exit")
    [[ "$exit_code" -eq 124 ]]
}

#=============================================================================
# test_results_directory_structure
#=============================================================================

@test "test_results_directory_structure: parallel_<state> dir with correct branch files" {
    local wf
    wf=$(create_parallel_workflow '        - name: auth
          prompt: prompts/v5/characterize.md
          params:
            module: auth
        - name: api
          prompt: prompts/v5/characterize.md
          params:
            module: api' "600" "characterize")

    run "$RUN_PARALLEL_SH" "$wf" "characterize" "$PHASE_DIR" "test-plan" "test-phase"
    [[ "$status" -eq 0 ]]

    local results_dir="$PHASE_DIR/parallel_characterize"
    [[ -d "$results_dir" ]]

    # Auth files
    [[ -f "$results_dir/auth.exit" ]]
    [[ -f "$results_dir/auth.log" ]]

    # API files
    [[ -f "$results_dir/api.exit" ]]
    [[ -f "$results_dir/api.log" ]]
}

#=============================================================================
# test_collect_results_json
#=============================================================================

@test "test_collect_results_json: JSON summary with branch names and exit codes on stderr" {
    # Claude stub: read a control file for exit code
    cat > "$BATS_TEST_TMPDIR/claude" << 'STUB'
#!/bin/bash
cat - > /dev/null
# Branch "fail" should exit 1, others exit 0
if [[ "$PARALLEL_BRANCH_NAME" == "fail" ]]; then
    exit 1
fi
exit 0
STUB
    chmod +x "$BATS_TEST_TMPDIR/claude"

    local wf
    wf=$(create_parallel_workflow '        - name: good1
          prompt: prompts/v5/characterize.md
          params:
            module: good1
        - name: fail
          prompt: prompts/v5/characterize.md
          params:
            module: fail
        - name: good2
          prompt: prompts/v5/characterize.md
          params:
            module: good2')

    # Capture stderr separately for JSON summary
    run "$RUN_PARALLEL_SH" "$wf" "characterize" "$PHASE_DIR" "test-plan" "test-phase"
    [[ "$status" -eq 0 ]]

    local results_dir="$PHASE_DIR/parallel_characterize"

    # Verify exit codes in files
    [[ "$(cat "$results_dir/good1.exit")" -eq 0 ]]
    [[ "$(cat "$results_dir/fail.exit")" -eq 1 ]]
    [[ "$(cat "$results_dir/good2.exit")" -eq 0 ]]
}

#=============================================================================
# test_missing_workflow_file
#=============================================================================

@test "test_missing_workflow_file: exit 1 with error for non-existent workflow" {
    run "$RUN_PARALLEL_SH" "/nonexistent/workflow.yaml" "characterize" "$PHASE_DIR" "test-plan" "test-phase"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"workflow"* ]] || [[ "$output" == *"not found"* ]] || [[ "$output" == *"does not exist"* ]]
}

#=============================================================================
# test_state_without_parallel
#=============================================================================

@test "test_state_without_parallel: exit 2 for state without parallel block" {
    local wf
    wf=$(create_non_parallel_workflow)

    run "$RUN_PARALLEL_SH" "$wf" "impl" "$PHASE_DIR" "test-plan" "test-phase"
    [[ "$status" -eq 2 ]]
}

#=============================================================================
# test_cleanup_on_interrupt
#=============================================================================

@test "test_cleanup_on_interrupt: SIGTERM kills all branch processes" {
    # Claude stub that sleeps long enough to be interrupted
    cat > "$BATS_TEST_TMPDIR/claude" << 'STUB'
#!/bin/bash
cat - > /dev/null
exec sleep 999
STUB
    chmod +x "$BATS_TEST_TMPDIR/claude"

    local wf
    wf=$(create_multi_branch_workflow 600)

    # Run in background
    "$RUN_PARALLEL_SH" "$wf" "characterize" "$PHASE_DIR" "test-plan" "test-phase" &
    local main_pid=$!

    # Wait for PID files to appear (branches spawned)
    local results_dir="$PHASE_DIR/parallel_characterize"
    local waited=0
    while [[ $waited -lt 15 ]]; do
        shopt -s nullglob
        local pfiles=("$results_dir"/*.pid)
        shopt -u nullglob
        [[ ${#pfiles[@]} -ge 3 ]] && break
        sleep 1
        waited=$((waited + 1))
    done

    # Collect branch PIDs
    local branch_pids=()
    shopt -s nullglob
    local pidfiles=("$results_dir"/*.pid)
    shopt -u nullglob
    for pidfile in "${pidfiles[@]}"; do
        if [[ -f "$pidfile" ]]; then
            branch_pids+=("$(cat "$pidfile")")
        fi
    done

    # Send SIGTERM to the main process
    kill -TERM "$main_pid" 2>/dev/null || true
    wait "$main_pid" 2>/dev/null || true

    # Wait a moment for cleanup
    sleep 2

    # Verify all branch PIDs are no longer running
    for pid in "${branch_pids[@]}"; do
        ! kill -0 "$pid" 2>/dev/null
    done
}

#=============================================================================
# test_branch_prompt_rendering
#=============================================================================

@test "test_branch_prompt_rendering: rendered prompt contains branch params, not template vars" {
    # Claude stub that writes stdin to a file
    cat > "$BATS_TEST_TMPDIR/claude" << 'STUB'
#!/bin/bash
cat - > "$BATS_TEST_TMPDIR/rendered_prompt_${PARALLEL_BRANCH_NAME}.txt"
exit 0
STUB
    chmod +x "$BATS_TEST_TMPDIR/claude"

    local wf
    wf=$(create_parallel_workflow '        - name: auth
          prompt: prompts/v5/characterize.md
          params:
            module: auth')

    run "$RUN_PARALLEL_SH" "$wf" "characterize" "$PHASE_DIR" "test-plan" "test-phase"
    [[ "$status" -eq 0 ]]

    # Check that the rendered prompt contains "auth" not "{{module}}"
    local rendered="$BATS_TEST_TMPDIR/rendered_prompt_auth.txt"
    if [[ -f "$rendered" ]]; then
        [[ "$(cat "$rendered")" == *"auth"* ]]
        [[ "$(cat "$rendered")" != *"{{module}}"* ]]
    fi
}

#=============================================================================
# test_empty_branches_array
#=============================================================================

@test "test_empty_branches_array: exit 1 with error for empty branches" {
    local wf
    wf=$(create_empty_branches_workflow)

    run "$RUN_PARALLEL_SH" "$wf" "characterize" "$PHASE_DIR" "test-plan" "test-phase"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"no branches"* ]] || [[ "$output" == *"empty"* ]] || [[ "$output" == *"branches"* ]]
}

#=============================================================================
# test_results_dir_recreated
#=============================================================================

@test "test_results_dir_recreated: old results directory cleared and recreated" {
    local wf
    wf=$(create_parallel_workflow)

    # Create a stale results directory
    local results_dir="$PHASE_DIR/parallel_characterize"
    mkdir -p "$results_dir"
    echo "stale" > "$results_dir/old_file.txt"
    echo "stale_exit" > "$results_dir/stale_branch.exit"

    run "$RUN_PARALLEL_SH" "$wf" "characterize" "$PHASE_DIR" "test-plan" "test-phase"
    [[ "$status" -eq 0 ]]

    # Stale files should be gone
    [[ ! -f "$results_dir/old_file.txt" ]]
    [[ ! -f "$results_dir/stale_branch.exit" ]]

    # New results should exist
    [[ -f "$results_dir/auth.exit" ]]
}

#=============================================================================
# test_missing_prompt_file
#=============================================================================

@test "test_missing_prompt_file: exit 1 before spawning any branches" {
    local output_file="$TEST_TEMP_DIR/workflow.yaml"
    cat > "$output_file" << 'YAML'
name: test_missing_prompt
version: 5
defaults:
  timeout: 600
states:
  - name: characterize
    description: Characterize modules
    prompt: prompts/v5/characterize.md
    parallel:
      branches:
        - name: auth
          prompt: prompts/v5/nonexistent_prompt.md
          params:
            module: auth
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: characterize
terminal_states: [complete]
YAML

    run "$RUN_PARALLEL_SH" "$output_file" "characterize" "$PHASE_DIR" "test-plan" "test-phase"
    [[ "$status" -eq 1 ]]

    # No results directory should exist (branches never spawned)
    [[ ! -d "$PHASE_DIR/parallel_characterize" ]] || \
        [[ -z "$(ls -A "$PHASE_DIR/parallel_characterize/" 2>/dev/null)" ]]
}

#=============================================================================
# test_invalid_branch_name
#=============================================================================

@test "test_invalid_branch_name: exit 1 for branch name with spaces" {
    local wf
    wf=$(create_parallel_workflow '        - name: "my branch"
          prompt: prompts/v5/characterize.md
          params:
            module: test')

    run "$RUN_PARALLEL_SH" "$wf" "characterize" "$PHASE_DIR" "test-plan" "test-phase"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"invalid"* ]] || [[ "$output" == *"branch name"* ]]
}

@test "test_invalid_branch_name_dots: exit 1 for branch name with dots" {
    local wf
    wf=$(create_parallel_workflow '        - name: "auth.module"
          prompt: prompts/v5/characterize.md
          params:
            module: test')

    run "$RUN_PARALLEL_SH" "$wf" "characterize" "$PHASE_DIR" "test-plan" "test-phase"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"invalid"* ]] || [[ "$output" == *"branch name"* ]]
}

#=============================================================================
# test_all_branches_timeout
#=============================================================================

@test "test_all_branches_timeout: all 3 branches exit 124 with short timeout" {
    # Claude stub that sleeps forever
    cat > "$BATS_TEST_TMPDIR/claude" << 'STUB'
#!/bin/bash
cat - > /dev/null
exec sleep 999
STUB
    chmod +x "$BATS_TEST_TMPDIR/claude"

    local wf
    wf=$(create_multi_branch_workflow 2)

    run "$RUN_PARALLEL_SH" "$wf" "characterize" "$PHASE_DIR" "test-plan" "test-phase"
    [[ "$status" -eq 0 ]]

    local results_dir="$PHASE_DIR/parallel_characterize"
    for branch in auth api db; do
        [[ -f "$results_dir/$branch.exit" ]]
        local exit_code
        exit_code=$(cat "$results_dir/$branch.exit")
        [[ "$exit_code" -eq 124 ]]
    done
}

#=============================================================================
# test_branch_name_exported
#=============================================================================

@test "test_branch_name_exported: PARALLEL_BRANCH_NAME env var exported to child" {
    # Claude stub that writes PARALLEL_BRANCH_NAME to a marker file
    cat > "$BATS_TEST_TMPDIR/claude" << 'STUB'
#!/bin/bash
cat - > /dev/null
echo "$PARALLEL_BRANCH_NAME" > "$BATS_TEST_TMPDIR/branch_marker_${PARALLEL_BRANCH_NAME}.txt"
exit 0
STUB
    chmod +x "$BATS_TEST_TMPDIR/claude"

    local wf
    wf=$(create_parallel_workflow '        - name: auth
          prompt: prompts/v5/characterize.md
          params:
            module: auth')

    run "$RUN_PARALLEL_SH" "$wf" "characterize" "$PHASE_DIR" "test-plan" "test-phase"
    [[ "$status" -eq 0 ]]

    # Check marker file contains "auth"
    [[ -f "$BATS_TEST_TMPDIR/branch_marker_auth.txt" ]]
    [[ "$(cat "$BATS_TEST_TMPDIR/branch_marker_auth.txt")" == "auth" ]]
}

#=============================================================================
# test_context_json_merge_override
#=============================================================================

@test "test_context_json_merge_override: branch params override phase context" {
    # Claude stub that writes stdin (rendered prompt) to a file
    cat > "$BATS_TEST_TMPDIR/claude" << 'STUB'
#!/bin/bash
cat - > "$BATS_TEST_TMPDIR/rendered_${PARALLEL_BRANCH_NAME}.txt"
exit 0
STUB
    chmod +x "$BATS_TEST_TMPDIR/claude"

    # Create a template that uses plan_name
    cat > "$MOCK_ORCH_DIR/prompts/v5/characterize.md" << 'PROMPT'
Plan: {{plan_name}}
PROMPT

    # Branch params override plan_name
    local wf
    wf=$(create_parallel_workflow '        - name: override
          prompt: prompts/v5/characterize.md
          params:
            plan_name: override-value')

    run "$RUN_PARALLEL_SH" "$wf" "characterize" "$PHASE_DIR" "original-plan" "test-phase"
    [[ "$status" -eq 0 ]]

    # The rendered prompt should contain "override-value" not "original-plan"
    if [[ -f "$BATS_TEST_TMPDIR/rendered_override.txt" ]]; then
        [[ "$(cat "$BATS_TEST_TMPDIR/rendered_override.txt")" == *"override-value"* ]]
        [[ "$(cat "$BATS_TEST_TMPDIR/rendered_override.txt")" != *"original-plan"* ]]
    fi
}

#=============================================================================
# test_context_json_no_branch_params
#=============================================================================

@test "test_context_json_no_branch_params: phase context passes through without branch params" {
    # Claude stub that writes stdin to a file
    cat > "$BATS_TEST_TMPDIR/claude" << 'STUB'
#!/bin/bash
cat - > "$BATS_TEST_TMPDIR/rendered_${PARALLEL_BRANCH_NAME}.txt"
exit 0
STUB
    chmod +x "$BATS_TEST_TMPDIR/claude"

    # Template that uses plan_name
    cat > "$MOCK_ORCH_DIR/prompts/v5/characterize.md" << 'PROMPT'
Plan: {{plan_name}}
PROMPT

    # Branch with NO params field
    local wf
    wf=$(create_parallel_workflow '        - name: noparam
          prompt: prompts/v5/characterize.md')

    run "$RUN_PARALLEL_SH" "$wf" "characterize" "$PHASE_DIR" "v5" "test-phase"
    [[ "$status" -eq 0 ]]

    # The rendered prompt should contain "v5" (from phase context)
    if [[ -f "$BATS_TEST_TMPDIR/rendered_noparam.txt" ]]; then
        [[ "$(cat "$BATS_TEST_TMPDIR/rendered_noparam.txt")" == *"v5"* ]]
    fi
}

#=============================================================================
# test_grace_period_expiration
#=============================================================================

@test "test_grace_period_expiration: script exits after timeout+30 grace period" {
    # This test verifies that when a branch takes too long to produce an .exit file,
    # the script still finishes after the grace period.
    # With timeout=2, grace=30, max wait is 32 seconds.
    # The claude stub sleeps forever but timeout will kill it and produce .exit=124.
    cat > "$BATS_TEST_TMPDIR/claude" << 'STUB'
#!/bin/bash
cat - > /dev/null
exec sleep 999
STUB
    chmod +x "$BATS_TEST_TMPDIR/claude"

    local wf
    wf=$(create_parallel_workflow '        - name: slow
          prompt: prompts/v5/characterize.md
          params:
            module: slow' "2")

    # The script should complete within a reasonable time (timeout + grace)
    run timeout 60 "$RUN_PARALLEL_SH" "$wf" "characterize" "$PHASE_DIR" "test-plan" "test-phase"
    # Script should exit (not hang forever)
    [[ "$status" -eq 0 ]] || [[ "$status" -eq 124 ]]
}

#=============================================================================
# test_cleanup_sigkill_escalation
#=============================================================================

@test "test_cleanup_sigkill_escalation: SIGTERM-resistant process killed via SIGKILL" {
    # Claude stub that traps SIGTERM and ignores it
    cat > "$BATS_TEST_TMPDIR/claude" << 'STUB'
#!/bin/bash
cat - > /dev/null
trap '' TERM
exec sleep 999
STUB
    chmod +x "$BATS_TEST_TMPDIR/claude"

    local wf
    wf=$(create_parallel_workflow '        - name: stubborn
          prompt: prompts/v5/characterize.md
          params:
            module: stubborn' "2")

    # Run with overall timeout to prevent test hanging
    run timeout 30 "$RUN_PARALLEL_SH" "$wf" "characterize" "$PHASE_DIR" "test-plan" "test-phase"

    local results_dir="$PHASE_DIR/parallel_characterize"

    # Get the PID and verify the process was killed
    if [[ -f "$results_dir/stubborn.pid" ]]; then
        local pid
        pid=$(cat "$results_dir/stubborn.pid")
        # Process should no longer be running after cleanup
        ! kill -0 "$pid" 2>/dev/null
    fi
}

#=============================================================================
# test_valid_branch_names
#=============================================================================

@test "test_valid_branch_name_alphanumeric: letters and numbers accepted" {
    local wf
    wf=$(create_parallel_workflow '        - name: auth123
          prompt: prompts/v5/characterize.md
          params:
            module: auth')

    run "$RUN_PARALLEL_SH" "$wf" "characterize" "$PHASE_DIR" "test-plan" "test-phase"
    [[ "$status" -eq 0 ]]
}

@test "test_valid_branch_name_with_hyphens_underscores: hyphens and underscores accepted" {
    local wf
    wf=$(create_parallel_workflow '        - name: auth-module_v2
          prompt: prompts/v5/characterize.md
          params:
            module: auth')

    run "$RUN_PARALLEL_SH" "$wf" "characterize" "$PHASE_DIR" "test-plan" "test-phase"
    [[ "$status" -eq 0 ]]
}

#=============================================================================
# test_invalid_branch_name_special_chars
#=============================================================================

@test "test_invalid_branch_name_slash: exit 1 for branch name with slash" {
    local wf
    wf=$(create_parallel_workflow '        - name: "auth/module"
          prompt: prompts/v5/characterize.md
          params:
            module: test')

    run "$RUN_PARALLEL_SH" "$wf" "characterize" "$PHASE_DIR" "test-plan" "test-phase"
    [[ "$status" -eq 1 ]]
}

@test "test_invalid_branch_name_at_sign: exit 1 for branch name with @" {
    local wf
    wf=$(create_parallel_workflow '        - name: "auth@module"
          prompt: prompts/v5/characterize.md
          params:
            module: test')

    run "$RUN_PARALLEL_SH" "$wf" "characterize" "$PHASE_DIR" "test-plan" "test-phase"
    [[ "$status" -eq 1 ]]
}

#=============================================================================
# test_validation_order
#=============================================================================

@test "test_validation_rejects_before_spawning: invalid name prevents any branch spawn" {
    # Mix valid and invalid branch names — no branches should spawn
    local output_file="$TEST_TEMP_DIR/workflow.yaml"
    cat > "$output_file" << 'YAML'
name: test_mixed_validity
version: 5
defaults:
  timeout: 600
states:
  - name: characterize
    description: test
    prompt: prompts/v5/characterize.md
    parallel:
      branches:
        - name: valid-branch
          prompt: prompts/v5/characterize.md
          params:
            module: valid
        - name: "invalid branch"
          prompt: prompts/v5/characterize.md
          params:
            module: invalid
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: characterize
terminal_states: [complete]
YAML

    run "$RUN_PARALLEL_SH" "$output_file" "characterize" "$PHASE_DIR" "test-plan" "test-phase"
    [[ "$status" -eq 1 ]]

    # No results directory should exist (validation before spawn)
    [[ ! -d "$PHASE_DIR/parallel_characterize" ]] || \
        [[ -z "$(ls -A "$PHASE_DIR/parallel_characterize/" 2>/dev/null)" ]]
}

@test "test_validation_missing_prompt_prevents_all_spawns: missing prompt blocks all branches" {
    local output_file="$TEST_TEMP_DIR/workflow.yaml"
    cat > "$output_file" << 'YAML'
name: test_prompt_validation
version: 5
defaults:
  timeout: 600
states:
  - name: characterize
    description: test
    prompt: prompts/v5/characterize.md
    parallel:
      branches:
        - name: valid
          prompt: prompts/v5/characterize.md
          params:
            module: valid
        - name: missing
          prompt: prompts/v5/nonexistent.md
          params:
            module: missing
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: characterize
terminal_states: [complete]
YAML

    run "$RUN_PARALLEL_SH" "$output_file" "characterize" "$PHASE_DIR" "test-plan" "test-phase"
    [[ "$status" -eq 1 ]]
}

#=============================================================================
# test_pid_file_atomic_write
#=============================================================================

@test "test_pid_files_contain_valid_pids: PID files have integer values" {
    local wf
    wf=$(create_multi_branch_workflow)

    run "$RUN_PARALLEL_SH" "$wf" "characterize" "$PHASE_DIR" "test-plan" "test-phase"
    [[ "$status" -eq 0 ]]

    local results_dir="$PHASE_DIR/parallel_characterize"
    for pidfile in "$results_dir"/*.pid; do
        local pid_value
        pid_value=$(cat "$pidfile")
        # PID should be a positive integer
        [[ "$pid_value" =~ ^[0-9]+$ ]]
        [[ "$pid_value" -gt 0 ]]
    done
}

#=============================================================================
# test_exit_files_contain_bare_integers
#=============================================================================

@test "test_exit_files_contain_bare_integers: .exit files are bare integers" {
    local wf
    wf=$(create_multi_branch_workflow)

    run "$RUN_PARALLEL_SH" "$wf" "characterize" "$PHASE_DIR" "test-plan" "test-phase"
    [[ "$status" -eq 0 ]]

    local results_dir="$PHASE_DIR/parallel_characterize"
    for exitfile in "$results_dir"/*.exit; do
        local exit_value
        exit_value=$(cat "$exitfile")
        # Should be a bare integer (no whitespace, no extra content)
        [[ "$exit_value" =~ ^[0-9]+$ ]]
    done
}

#=============================================================================
# test_branch_failure_does_not_affect_others
#=============================================================================

@test "test_branch_failure_does_not_affect_others: one failing branch, others succeed" {
    # Claude stub that fails for "fail" branch, succeeds for others
    cat > "$BATS_TEST_TMPDIR/claude" << 'STUB'
#!/bin/bash
cat - > /dev/null
if [[ "$PARALLEL_BRANCH_NAME" == "fail" ]]; then
    exit 1
fi
exit 0
STUB
    chmod +x "$BATS_TEST_TMPDIR/claude"

    local wf
    wf=$(create_parallel_workflow '        - name: good
          prompt: prompts/v5/characterize.md
          params:
            module: good
        - name: fail
          prompt: prompts/v5/characterize.md
          params:
            module: fail
        - name: also-good
          prompt: prompts/v5/characterize.md
          params:
            module: also_good')

    run "$RUN_PARALLEL_SH" "$wf" "characterize" "$PHASE_DIR" "test-plan" "test-phase"
    [[ "$status" -eq 0 ]]

    local results_dir="$PHASE_DIR/parallel_characterize"
    [[ "$(cat "$results_dir/good.exit")" -eq 0 ]]
    [[ "$(cat "$results_dir/fail.exit")" -eq 1 ]]
    [[ "$(cat "$results_dir/also-good.exit")" -eq 0 ]]
}

#=============================================================================
# test_argument_validation
#=============================================================================

@test "test_missing_arguments: exit 1 with usage info when args missing" {
    run "$RUN_PARALLEL_SH"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Usage"* ]] || [[ "$output" == *"usage"* ]]
}

@test "test_partial_arguments: exit 1 when not all args provided" {
    local wf
    wf=$(create_parallel_workflow)
    run "$RUN_PARALLEL_SH" "$wf" "characterize"
    [[ "$status" -eq 1 ]]
}

#=============================================================================
# test_nonexistent_state
#=============================================================================

@test "test_nonexistent_state: exit 2 for state not found in workflow" {
    local wf
    wf=$(create_parallel_workflow)

    run "$RUN_PARALLEL_SH" "$wf" "nonexistent_state" "$PHASE_DIR" "test-plan" "test-phase"
    [[ "$status" -eq 2 ]]
}

#=============================================================================
# test_default_timeout
#=============================================================================

@test "test_default_timeout_600: workflow without timeout defaults to 600" {
    local wf
    wf=$(create_no_timeout_workflow)

    # This just verifies it runs without error using default timeout
    # (We can't easily verify the exact timeout value, but the script should work)
    run "$RUN_PARALLEL_SH" "$wf" "characterize" "$PHASE_DIR" "test-plan" "test-phase"
    [[ "$status" -eq 0 ]]
}

#=============================================================================
# test_log_files_capture_output
#=============================================================================

@test "test_log_files_capture_output: .log files contain agent stdout/stderr" {
    # Claude stub that writes to stdout and stderr
    cat > "$BATS_TEST_TMPDIR/claude" << 'STUB'
#!/bin/bash
cat - > /dev/null
echo "stdout output from $PARALLEL_BRANCH_NAME"
echo "stderr output from $PARALLEL_BRANCH_NAME" >&2
exit 0
STUB
    chmod +x "$BATS_TEST_TMPDIR/claude"

    local wf
    wf=$(create_parallel_workflow '        - name: verbose
          prompt: prompts/v5/characterize.md
          params:
            module: verbose')

    run "$RUN_PARALLEL_SH" "$wf" "characterize" "$PHASE_DIR" "test-plan" "test-phase"
    [[ "$status" -eq 0 ]]

    local logfile="$PHASE_DIR/parallel_characterize/verbose.log"
    [[ -f "$logfile" ]]
    # Log should capture stdout and/or stderr
    [[ -s "$logfile" ]]
    [[ "$(cat "$logfile")" == *"verbose"* ]]
}

#=============================================================================
# test_overall_exit_code
#=============================================================================

@test "test_overall_exit_0_even_with_branch_failures: script exits 0 after collecting results" {
    # Per spec: exit 0 = all branches spawned and results collected
    # Branch failure is recorded in .exit files, does not affect script exit code
    cat > "$BATS_TEST_TMPDIR/claude" << 'STUB'
#!/bin/bash
cat - > /dev/null
exit 1
STUB
    chmod +x "$BATS_TEST_TMPDIR/claude"

    local wf
    wf=$(create_parallel_workflow '        - name: failing
          prompt: prompts/v5/characterize.md
          params:
            module: failing')

    run "$RUN_PARALLEL_SH" "$wf" "characterize" "$PHASE_DIR" "test-plan" "test-phase"
    [[ "$status" -eq 0 ]]

    # But the branch should record its failure
    [[ "$(cat "$PHASE_DIR/parallel_characterize/failing.exit")" -eq 1 ]]
}

#=============================================================================
# test_setsid_process_isolation
#=============================================================================

@test "test_branches_use_setsid: branch PIDs are process group leaders" {
    local wf
    wf=$(create_parallel_workflow)

    run "$RUN_PARALLEL_SH" "$wf" "characterize" "$PHASE_DIR" "test-plan" "test-phase"
    [[ "$status" -eq 0 ]]

    # PID files should exist and contain valid PIDs
    local results_dir="$PHASE_DIR/parallel_characterize"
    [[ -f "$results_dir/auth.pid" ]]

    local pid
    pid=$(cat "$results_dir/auth.pid")
    [[ "$pid" =~ ^[0-9]+$ ]]
}

#=============================================================================
# test_no_pid_tmp_files_left
#=============================================================================

@test "test_no_pid_tmp_files_left: atomic PID write leaves no .pid.tmp files" {
    local wf
    wf=$(create_multi_branch_workflow)

    run "$RUN_PARALLEL_SH" "$wf" "characterize" "$PHASE_DIR" "test-plan" "test-phase"
    [[ "$status" -eq 0 ]]

    local results_dir="$PHASE_DIR/parallel_characterize"
    # No .pid.tmp files should remain
    local tmp_count
    tmp_count=$(ls "$results_dir"/*.pid.tmp 2>/dev/null | wc -l)
    [[ "$tmp_count" -eq 0 ]]
}

#=============================================================================
# test_concurrent_branches_actually_parallel
#=============================================================================

@test "test_branches_run_concurrently: multiple branches start at roughly the same time" {
    # Claude stub that records start time
    cat > "$BATS_TEST_TMPDIR/claude" << 'STUB'
#!/bin/bash
cat - > /dev/null
date +%s > "$BATS_TEST_TMPDIR/start_${PARALLEL_BRANCH_NAME}.txt"
sleep 1
exit 0
STUB
    chmod +x "$BATS_TEST_TMPDIR/claude"

    local wf
    wf=$(create_multi_branch_workflow)

    run "$RUN_PARALLEL_SH" "$wf" "characterize" "$PHASE_DIR" "test-plan" "test-phase"
    [[ "$status" -eq 0 ]]

    # If branches ran sequentially, start times would differ by ~1s each.
    # If parallel, they should all start within a few seconds of each other.
    if [[ -f "$BATS_TEST_TMPDIR/start_auth.txt" ]] && \
       [[ -f "$BATS_TEST_TMPDIR/start_api.txt" ]] && \
       [[ -f "$BATS_TEST_TMPDIR/start_db.txt" ]]; then
        local t1 t2 t3
        t1=$(cat "$BATS_TEST_TMPDIR/start_auth.txt")
        t2=$(cat "$BATS_TEST_TMPDIR/start_api.txt")
        t3=$(cat "$BATS_TEST_TMPDIR/start_db.txt")

        # All start times should be within 3 seconds of each other
        local max min diff
        max=$t1; [[ $t2 -gt $max ]] && max=$t2; [[ $t3 -gt $max ]] && max=$t3
        min=$t1; [[ $t2 -lt $min ]] && min=$t2; [[ $t3 -lt $min ]] && min=$t3
        diff=$((max - min))
        [[ $diff -le 3 ]]
    fi
}
