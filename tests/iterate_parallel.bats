#!/usr/bin/env bats

# QA Tests for iterate-integration phase (orchestration-v5)
#
# Tests the integration of parallel execution into iterate.sh:
# - Detection of parallel states via `.parallel` block in workflow
# - run_parallel_state() function orchestrating the lifecycle
# - Helper functions: get_branch_names, get_parallel_strategy, get_parallel_n
# - Interaction with existing V4 features (escalation, constraints, hooks)
# - State transitions via verdict from parallel execution
#
# Strategy: Source iterate.sh (gets V4 function definitions), stub external
# scripts (run-parallel.sh, join-parallel.sh, update-state.sh) via SCRIPTS_DIR
# override, and call run_iteration directly.

setup() {
    load 'test_helper'
    setup_temp_dir

    # Plan/phase directory structure
    export PLANS_DIR="$TEST_TEMP_DIR/plans"
    export ARC_HOME="$TEST_TEMP_DIR/orchestration"
    mkdir -p "$PLANS_DIR/active/test-plan/phases/test-phase"
    mkdir -p "$ARC_HOME/scripts"
    mkdir -p "$ARC_HOME/prompts/feature"
    mkdir -p "$ARC_HOME/prompts/common"
    mkdir -p "$ARC_HOME/prompts/v5"

    # Standard paths
    export PLAN_DIR="$PLANS_DIR/active/test-plan"
    export PHASE_DIR="$PLAN_DIR/phases/test-phase"
    export STATE_FILE="$PHASE_DIR/state.json"
    export WORKFLOW_FILE="$PLAN_DIR/workflow.yaml"
    export ARC_DEFAULT_PKG="test-package"
    export VERDICT=""

    # Create minimal state.json
    cat > "$STATE_FILE" << 'JSON'
{
    "plan": "test-plan",
    "phase": "test-phase",
    "iteration": 0,
    "current_state": "characterize",
    "stuck_iterations": 0,
    "tests_passing": 0,
    "tests_total": 0
}
JSON

    # Create mock prompt files
    echo "Test prompt" > "$ARC_HOME/prompts/feature/impl.md"
    echo "Review prompt" > "$ARC_HOME/prompts/feature/impl-review.md"
    echo "Complete prompt" > "$ARC_HOME/prompts/common/complete.md"
    echo "Parallel prompt" > "$ARC_HOME/prompts/v5/characterize.md"

    # Stub directory — override SCRIPTS_DIR so iterate.sh calls our stubs
    STUB_DIR="$BATS_TEST_TMPDIR/stubs"
    mkdir -p "$STUB_DIR"
    export SCRIPTS_DIR="$STUB_DIR"

    # Copy real V4 library scripts to stub dir so setup_v4_environment can source them
    cp "$ORCH_DIR/scripts/actions.sh" "$STUB_DIR/"
    cp "$ORCH_DIR/scripts/check-constraints.sh" "$STUB_DIR/"
    cp "$ORCH_DIR/scripts/check-escalation.sh" "$STUB_DIR/"
    cp "$ORCH_DIR/scripts/check-intervention.sh" "$STUB_DIR/"
    cp "$ORCH_DIR/scripts/run-hooks.sh" "$STUB_DIR/"
    cp "$ORCH_DIR/scripts/extract-verdict.sh" "$STUB_DIR/" 2>/dev/null || true
    cp "$ORCH_DIR/scripts/build-context.sh" "$STUB_DIR/" 2>/dev/null || true
    cp "$ORCH_DIR/scripts/render_template.py" "$STUB_DIR/" 2>/dev/null || true

    # Create default stubs for parallel scripts
    create_default_stubs

    # Source iterate.sh to get V4 function definitions
    source "$ORCH_DIR/scripts/iterate.sh"

    # Mock spawn_agent globally (used by linear states)
    spawn_agent() {
        local prompt_file="$1"
        local output_file="$2"
        echo "Mock agent output" > "$output_file"
        return 0
    }
    export -f spawn_agent
}

teardown() {
    teardown_temp_dir
}

# ==============================================================================
# Helpers
# ==============================================================================

# Create default stubs for run-parallel.sh, join-parallel.sh, update-state.sh
create_default_stubs() {
    # run-parallel.sh: creates .exit files in results dir, logs args
    cat > "$STUB_DIR/run-parallel.sh" << 'STUB'
#!/bin/bash
echo "$@" >> "$STUB_DIR/run-parallel.log"
# Args: workflow_file state_name phase_dir plan_name phase_name
RESULTS="$3/parallel_$2"
mkdir -p "$RESULTS"
echo "0" > "$RESULTS/branch_a.exit"
echo "0" > "$RESULTS/branch_b.exit"
exit 0
STUB
    chmod +x "$STUB_DIR/run-parallel.sh"

    # join-parallel.sh: echoes verdict, logs args
    cat > "$STUB_DIR/join-parallel.sh" << 'STUB'
#!/bin/bash
echo "$@" >> "$STUB_DIR/join-parallel.log"
echo "all_complete"
exit 0
STUB
    chmod +x "$STUB_DIR/join-parallel.sh"

    # update-state.sh: logs args, writes .last_verdict for parallel-finish
    cat > "$STUB_DIR/update-state.sh" << STUB
#!/bin/bash
echo "\$@" >> "$STUB_DIR/update-state.log"
# For parallel-finish, write .last_verdict so state transition works
if [[ "\$3" == "parallel-finish" ]]; then
    jq --arg v "\$4" '.last_verdict = \$v' "$STATE_FILE" > "${STATE_FILE}.tmp" && mv "${STATE_FILE}.tmp" "$STATE_FILE"
fi
exit 0
STUB
    chmod +x "$STUB_DIR/update-state.sh"
}

# Helper: Set a field in state.json
set_state_field() {
    local field="$1"
    local value="$2"
    jq --argjson v "$value" ".$field = \$v" "$STATE_FILE" > "${STATE_FILE}.tmp" \
        && mv "${STATE_FILE}.tmp" "$STATE_FILE"
}

# Helper: Set a string field in state.json
set_state_string() {
    local field="$1"
    local value="$2"
    jq --arg v "$value" ".$field = \$v" "$STATE_FILE" > "${STATE_FILE}.tmp" \
        && mv "${STATE_FILE}.tmp" "$STATE_FILE"
}

# Helper: Get a field from state.json
get_state_field() {
    local field="$1"
    jq -r ".$field // empty" "$STATE_FILE"
}

# Helper: Create a workflow inline
create_workflow() {
    cat > "$WORKFLOW_FILE"
}

# Helper: Create V5 parallel workflow
create_parallel_workflow() {
    create_workflow << 'YAML'
name: test_parallel
version: 5
description: Test workflow with parallel state
states:
  - name: characterize
    description: Characterize modules in parallel
    prompt: prompts/v5/characterize.md
    parallel:
      strategy: all
      branches:
        - name: branch_a
          prompt: prompts/v5/characterize.md
        - name: branch_b
          prompt: prompts/v5/characterize.md
    verdicts: [all_complete, any_failed]
    next:
      all_complete: refactor
      any_failed: blocked
  - name: refactor
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
  - name: blocked
    prompt: prompts/common/complete.md
entry_state: characterize
terminal_states: [complete, blocked]
YAML
}

# Helper: Create a linear (non-parallel) V5 workflow
create_linear_workflow() {
    create_workflow << 'YAML'
name: test_linear
version: 5
description: V5 workflow without parallel states
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML
}

# Helper: skip if iterate.sh lacks parallel integration
skip_if_not_parallel_integrated() {
    if ! grep -q "run_parallel_state" "$ORCH_DIR/scripts/iterate.sh" 2>/dev/null; then
        skip "iterate.sh does not have parallel integration yet"
    fi
}

# Helper: Clear stub logs
clear_stub_logs() {
    rm -f "$STUB_DIR/run-parallel.log"
    rm -f "$STUB_DIR/join-parallel.log"
    rm -f "$STUB_DIR/update-state.log"
}

# ==============================================================================
# Parallel State Detection
# ==============================================================================

@test "qa_iterate-integration_test_parallel_state_detection: parallel state calls run-parallel.sh" {
    skip_if_not_parallel_integrated

    create_parallel_workflow
    setup_v4_environment "test-plan" "test-phase"

    run run_iteration "test-plan" "test-phase" "characterize"
    [ "$status" -eq 0 ]

    # Verify run-parallel.sh was called
    [ -f "$STUB_DIR/run-parallel.log" ]
}

@test "qa_iterate-integration_test_linear_state_unchanged: linear state does not call run-parallel.sh" {
    skip_if_not_parallel_integrated

    create_linear_workflow
    set_state_string "current_state" "impl"
    setup_v4_environment "test-plan" "test-phase"

    run run_iteration "test-plan" "test-phase" "impl"
    [ "$status" -eq 0 ]

    # Verify run-parallel.sh was NOT called
    [ ! -f "$STUB_DIR/run-parallel.log" ]
}

# ==============================================================================
# run_parallel_state Function
# ==============================================================================

@test "qa_iterate-integration_test_run_parallel_state_calls_run_parallel: correct args passed" {
    skip_if_not_parallel_integrated

    create_parallel_workflow
    setup_v4_environment "test-plan" "test-phase"

    run run_iteration "test-plan" "test-phase" "characterize"
    [ "$status" -eq 0 ]

    # Verify run-parallel.sh was called with correct args
    [ -f "$STUB_DIR/run-parallel.log" ]
    local log_content
    log_content=$(cat "$STUB_DIR/run-parallel.log")
    # Args should include: workflow_file, "characterize", phase_dir
    [[ "$log_content" == *"characterize"* ]]
    [[ "$log_content" == *"$PHASE_DIR"* ]]
}

@test "qa_iterate-integration_test_parallel_state_updates_branch_status: exit codes mapped correctly" {
    skip_if_not_parallel_integrated

    create_parallel_workflow
    setup_v4_environment "test-plan" "test-phase"

    # run-parallel.sh stub already creates branch_a.exit=0 and branch_b.exit=1
    cat > "$STUB_DIR/run-parallel.sh" << STUB
#!/bin/bash
echo "\$@" >> "$STUB_DIR/run-parallel.log"
RESULTS="\$3/parallel_\$2"
mkdir -p "\$RESULTS"
echo "0" > "\$RESULTS/branch_a.exit"
echo "1" > "\$RESULTS/branch_b.exit"
exit 0
STUB
    chmod +x "$STUB_DIR/run-parallel.sh"

    run run_iteration "test-plan" "test-phase" "characterize"
    [ "$status" -eq 0 ]

    # Verify update-state.sh was called with parallel-update for each branch
    local log_content
    log_content=$(cat "$STUB_DIR/update-state.log")
    [[ "$log_content" == *"parallel-update branch_a complete 0"* ]]
    [[ "$log_content" == *"parallel-update branch_b failed 1"* ]]
}

@test "qa_iterate-integration_test_parallel_state_calls_join: join-parallel.sh called with strategy" {
    skip_if_not_parallel_integrated

    create_parallel_workflow
    setup_v4_environment "test-plan" "test-phase"

    run run_iteration "test-plan" "test-phase" "characterize"
    [ "$status" -eq 0 ]

    # Verify join-parallel.sh was called with "all" strategy
    [ -f "$STUB_DIR/join-parallel.log" ]
    local log_content
    log_content=$(cat "$STUB_DIR/join-parallel.log")
    [[ "$log_content" == *"all"* ]]
}

@test "qa_iterate-integration_test_parallel_verdict_feeds_branch_resolution: state transitions on verdict" {
    skip_if_not_parallel_integrated

    create_parallel_workflow
    setup_v4_environment "test-plan" "test-phase"

    # join stub echoes "all_complete", workflow maps all_complete -> refactor
    run run_iteration "test-plan" "test-phase" "characterize"
    [ "$status" -eq 0 ]

    # State should transition to "refactor"
    local current_state
    current_state=$(jq -r '.current_state' "$STATE_FILE")
    [ "$current_state" == "refactor" ]
}

@test "qa_iterate-integration_test_parallel_finish_updates_state: parallel-finish called with verdict" {
    skip_if_not_parallel_integrated

    # Use join stub that echoes "any_failed"
    cat > "$STUB_DIR/join-parallel.sh" << STUB
#!/bin/bash
echo "\$@" >> "$STUB_DIR/join-parallel.log"
echo "any_failed"
exit 0
STUB
    chmod +x "$STUB_DIR/join-parallel.sh"

    create_parallel_workflow
    setup_v4_environment "test-plan" "test-phase"

    run run_iteration "test-plan" "test-phase" "characterize"
    [ "$status" -eq 0 ]

    # Verify update-state.sh log contains parallel-finish with verdict
    local log_content
    log_content=$(cat "$STUB_DIR/update-state.log")
    [[ "$log_content" == *"parallel-finish any_failed"* ]]
}

@test "qa_iterate-integration_test_parallel_clear_called: parallel-clear is final parallel command" {
    skip_if_not_parallel_integrated

    create_parallel_workflow
    setup_v4_environment "test-plan" "test-phase"

    run run_iteration "test-plan" "test-phase" "characterize"
    [ "$status" -eq 0 ]

    # Verify update-state.sh log contains parallel-clear
    local log_content
    log_content=$(cat "$STUB_DIR/update-state.log")
    [[ "$log_content" == *"parallel-clear"* ]]

    # Verify parallel-clear is after parallel-finish
    local finish_line clear_line
    finish_line=$(grep -n "parallel-finish" "$STUB_DIR/update-state.log" | head -1 | cut -d: -f1)
    clear_line=$(grep -n "parallel-clear" "$STUB_DIR/update-state.log" | head -1 | cut -d: -f1)
    [ "$clear_line" -gt "$finish_line" ]
}

# ==============================================================================
# Helper Functions
# ==============================================================================

@test "qa_iterate-integration_test_get_branch_names: extracts CSV branch names" {
    skip_if_not_parallel_integrated

    create_workflow << 'YAML'
name: test
version: 5
states:
  - name: characterize
    parallel:
      strategy: all
      branches:
        - name: auth
          prompt: prompts/v5/characterize.md
        - name: api
          prompt: prompts/v5/characterize.md
        - name: db
          prompt: prompts/v5/characterize.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: characterize
terminal_states: [complete]
YAML

    run get_branch_names "$WORKFLOW_FILE" "characterize"
    [ "$status" -eq 0 ]
    [ "$output" == "auth,api,db" ]
}

@test "qa_iterate-integration_test_get_parallel_strategy: returns strategy name" {
    skip_if_not_parallel_integrated

    create_workflow << 'YAML'
name: test
version: 5
states:
  - name: characterize
    parallel:
      strategy: n_of_m
      n: 2
      branches:
        - name: a
          prompt: prompts/v5/characterize.md
        - name: b
          prompt: prompts/v5/characterize.md
        - name: c
          prompt: prompts/v5/characterize.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: characterize
terminal_states: [complete]
YAML

    run get_parallel_strategy "$WORKFLOW_FILE" "characterize"
    [ "$status" -eq 0 ]
    [ "$output" == "n_of_m" ]
}

@test "qa_iterate-integration_test_get_parallel_n: returns n value for n_of_m" {
    skip_if_not_parallel_integrated

    create_workflow << 'YAML'
name: test
version: 5
states:
  - name: characterize
    parallel:
      strategy: n_of_m
      n: 2
      branches:
        - name: a
          prompt: prompts/v5/characterize.md
        - name: b
          prompt: prompts/v5/characterize.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: characterize
terminal_states: [complete]
YAML

    run get_parallel_n "$WORKFLOW_FILE" "characterize"
    [ "$status" -eq 0 ]
    [ "$output" == "2" ]
}

@test "qa_iterate-integration_test_get_parallel_n_non_n_of_m: returns empty for all strategy" {
    skip_if_not_parallel_integrated

    create_parallel_workflow

    run get_parallel_n "$WORKFLOW_FILE" "characterize"
    [ "$status" -eq 0 ]
    [ -z "$output" ]
}

@test "qa_iterate-integration_test_get_branch_names_single_branch: no trailing comma" {
    skip_if_not_parallel_integrated

    create_workflow << 'YAML'
name: test
version: 5
states:
  - name: characterize
    parallel:
      strategy: all
      branches:
        - name: auth
          prompt: prompts/v5/characterize.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: characterize
terminal_states: [complete]
YAML

    run get_branch_names "$WORKFLOW_FILE" "characterize"
    [ "$status" -eq 0 ]
    [ "$output" == "auth" ]
}

# ==============================================================================
# Timeout Handling
# ==============================================================================

@test "qa_iterate-integration_test_parallel_with_timeout_branch: exit 124 maps to timeout" {
    skip_if_not_parallel_integrated

    create_parallel_workflow
    setup_v4_environment "test-plan" "test-phase"

    # run-parallel.sh stub creates branch with exit 124 (timeout)
    cat > "$STUB_DIR/run-parallel.sh" << STUB
#!/bin/bash
echo "\$@" >> "$STUB_DIR/run-parallel.log"
RESULTS="\$3/parallel_\$2"
mkdir -p "\$RESULTS"
echo "124" > "\$RESULTS/branch.exit"
exit 0
STUB
    chmod +x "$STUB_DIR/run-parallel.sh"

    run run_iteration "test-plan" "test-phase" "characterize"
    [ "$status" -eq 0 ]

    local log_content
    log_content=$(cat "$STUB_DIR/update-state.log")
    [[ "$log_content" == *"parallel-update branch timeout 124"* ]]
}

# ==============================================================================
# V4 Feature Interaction: Escalation
# ==============================================================================

@test "qa_iterate-integration_test_escalation_skipped_for_parallel: step 2 skipped" {
    skip_if_not_parallel_integrated

    create_parallel_workflow
    setup_v4_environment "test-plan" "test-phase"

    # Stub check_escalation to detect if it's called
    check_escalation() { touch "$BATS_TEST_TMPDIR/escalation_called"; return 0; }
    export -f check_escalation

    run run_iteration "test-plan" "test-phase" "characterize"
    [ "$status" -eq 0 ]

    # Escalation should NOT have been called for parallel state
    [ ! -f "$BATS_TEST_TMPDIR/escalation_called" ]
}

# ==============================================================================
# V4 Feature Interaction: Post-Constraints
# ==============================================================================

@test "qa_iterate-integration_test_post_constraints_run_for_parallel: step 6 runs" {
    skip_if_not_parallel_integrated

    create_parallel_workflow
    setup_v4_environment "test-plan" "test-phase"

    # Stub check_post_constraints to detect if it's called
    check_post_constraints() { touch "$BATS_TEST_TMPDIR/post_constraints_called"; return 0; }
    export -f check_post_constraints

    run run_iteration "test-plan" "test-phase" "characterize"
    [ "$status" -eq 0 ]

    # Post-constraints should be called for parallel states
    [ -f "$BATS_TEST_TMPDIR/post_constraints_called" ]
}

# ==============================================================================
# V4 Feature Interaction: After Hooks
# ==============================================================================

@test "qa_iterate-integration_test_after_hooks_run_for_parallel: step 7 runs with verdict" {
    skip_if_not_parallel_integrated

    create_parallel_workflow
    setup_v4_environment "test-plan" "test-phase"

    # Stub run_after_hooks to capture args
    run_after_hooks() { echo "$@" > "$BATS_TEST_TMPDIR/after_hooks_args"; return 0; }
    export -f run_after_hooks

    run run_iteration "test-plan" "test-phase" "characterize"
    [ "$status" -eq 0 ]

    # After hooks should be called
    [ -f "$BATS_TEST_TMPDIR/after_hooks_args" ]

    # Verify verdict "all_complete" was passed
    local hook_args
    hook_args=$(cat "$BATS_TEST_TMPDIR/after_hooks_args")
    [[ "$hook_args" == *"all_complete"* ]]
}

# ==============================================================================
# Precedence
# ==============================================================================

@test "qa_iterate-integration_test_prompt_and_parallel_precedence: parallel takes precedence over prompt" {
    skip_if_not_parallel_integrated

    # State with both prompt and parallel fields
    create_workflow << 'YAML'
name: test
version: 5
states:
  - name: characterize
    prompt: prompts/v5/characterize.md
    parallel:
      strategy: all
      branches:
        - name: branch_a
          prompt: prompts/v5/characterize.md
        - name: branch_b
          prompt: prompts/v5/characterize.md
    verdicts: [all_complete, any_failed]
    next:
      all_complete: complete
      any_failed: blocked
  - name: complete
    prompt: prompts/common/complete.md
  - name: blocked
    prompt: prompts/common/complete.md
entry_state: characterize
terminal_states: [complete, blocked]
YAML

    setup_v4_environment "test-plan" "test-phase"

    run run_iteration "test-plan" "test-phase" "characterize"
    [ "$status" -eq 0 ]

    # Parallel path taken (run-parallel.sh was called)
    [ -f "$STUB_DIR/run-parallel.log" ]
}

# ==============================================================================
# Error Propagation
# ==============================================================================

@test "qa_iterate-integration_test_parallel_state_config_error: run-parallel.sh failure propagates" {
    skip_if_not_parallel_integrated

    create_parallel_workflow
    setup_v4_environment "test-plan" "test-phase"

    # run-parallel.sh stub exits 1
    cat > "$STUB_DIR/run-parallel.sh" << STUB
#!/bin/bash
echo "\$@" >> "$STUB_DIR/run-parallel.log"
exit 1
STUB
    chmod +x "$STUB_DIR/run-parallel.sh"

    run run_iteration "test-plan" "test-phase" "characterize"
    [ "$status" -ne 0 ]
}

@test "qa_iterate-integration_test_join_failure_propagation: join-parallel.sh failure propagates" {
    skip_if_not_parallel_integrated

    create_parallel_workflow
    setup_v4_environment "test-plan" "test-phase"

    # join-parallel.sh stub exits 1
    cat > "$STUB_DIR/join-parallel.sh" << STUB
#!/bin/bash
echo "\$@" >> "$STUB_DIR/join-parallel.log"
exit 1
STUB
    chmod +x "$STUB_DIR/join-parallel.sh"

    run run_iteration "test-plan" "test-phase" "characterize"
    [ "$status" -ne 0 ]

    # No parallel-finish or parallel-clear should be in log (lifecycle aborted)
    if [ -f "$STUB_DIR/update-state.log" ]; then
        local log_content
        log_content=$(cat "$STUB_DIR/update-state.log")
        [[ "$log_content" != *"parallel-finish"* ]]
        [[ "$log_content" != *"parallel-clear"* ]]
    fi
}

@test "qa_iterate-integration_test_update_state_start_failure_propagation: parallel-start failure aborts" {
    skip_if_not_parallel_integrated

    create_parallel_workflow
    setup_v4_environment "test-plan" "test-phase"

    # update-state.sh stub fails on parallel-start
    cat > "$STUB_DIR/update-state.sh" << STUB
#!/bin/bash
echo "\$@" >> "$STUB_DIR/update-state.log"
if [[ "\$3" == "parallel-start" ]]; then
    exit 1
fi
exit 0
STUB
    chmod +x "$STUB_DIR/update-state.sh"

    run run_iteration "test-plan" "test-phase" "characterize"
    [ "$status" -ne 0 ]

    # run-parallel.sh should NOT have been called (lifecycle aborted before spawning)
    [ ! -f "$STUB_DIR/run-parallel.log" ]
}

@test "qa_iterate-integration_test_zero_exit_files: empty results dir causes join failure" {
    skip_if_not_parallel_integrated

    create_parallel_workflow
    setup_v4_environment "test-plan" "test-phase"

    # run-parallel.sh creates results dir but no .exit files
    cat > "$STUB_DIR/run-parallel.sh" << STUB
#!/bin/bash
echo "\$@" >> "$STUB_DIR/run-parallel.log"
RESULTS="\$3/parallel_\$2"
mkdir -p "\$RESULTS"
# No .exit files created
exit 0
STUB
    chmod +x "$STUB_DIR/run-parallel.sh"

    # join-parallel.sh stub exits 2 for empty results
    cat > "$STUB_DIR/join-parallel.sh" << STUB
#!/bin/bash
echo "\$@" >> "$STUB_DIR/join-parallel.log"
exit 2
STUB
    chmod +x "$STUB_DIR/join-parallel.sh"

    run run_iteration "test-plan" "test-phase" "characterize"
    [ "$status" -ne 0 ]
}

# ==============================================================================
# n_of_m Strategy
# ==============================================================================

@test "qa_iterate-integration_test_n_of_m_passes_n_to_join: n argument forwarded" {
    skip_if_not_parallel_integrated

    create_workflow << 'YAML'
name: test
version: 5
states:
  - name: characterize
    parallel:
      strategy: n_of_m
      n: 2
      branches:
        - name: a
          prompt: prompts/v5/characterize.md
        - name: b
          prompt: prompts/v5/characterize.md
        - name: c
          prompt: prompts/v5/characterize.md
    verdicts: [n_complete, insufficient]
    next:
      n_complete: complete
      insufficient: blocked
  - name: complete
    prompt: prompts/common/complete.md
  - name: blocked
    prompt: prompts/common/complete.md
entry_state: characterize
terminal_states: [complete, blocked]
YAML

    setup_v4_environment "test-plan" "test-phase"

    # Stub with 3 branches succeeding
    cat > "$STUB_DIR/run-parallel.sh" << STUB
#!/bin/bash
echo "\$@" >> "$STUB_DIR/run-parallel.log"
RESULTS="\$3/parallel_\$2"
mkdir -p "\$RESULTS"
echo "0" > "\$RESULTS/a.exit"
echo "0" > "\$RESULTS/b.exit"
echo "1" > "\$RESULTS/c.exit"
exit 0
STUB
    chmod +x "$STUB_DIR/run-parallel.sh"

    cat > "$STUB_DIR/join-parallel.sh" << STUB
#!/bin/bash
echo "\$@" >> "$STUB_DIR/join-parallel.log"
echo "n_complete"
exit 0
STUB
    chmod +x "$STUB_DIR/join-parallel.sh"

    run run_iteration "test-plan" "test-phase" "characterize"
    [ "$status" -eq 0 ]

    # Verify join was called with: n_of_m <results_dir> 2
    local log_content
    log_content=$(cat "$STUB_DIR/join-parallel.log")
    [[ "$log_content" == *"n_of_m"* ]]
    [[ "$log_content" == *"2"* ]]
}

# ==============================================================================
# Verdict / State Consistency
# ==============================================================================

@test "qa_iterate-integration_test_last_verdict_not_double_written: parallel-finish sets verdict once" {
    skip_if_not_parallel_integrated

    create_parallel_workflow
    setup_v4_environment "test-plan" "test-phase"

    run run_iteration "test-plan" "test-phase" "characterize"
    [ "$status" -eq 0 ]

    # Verify .last_verdict is "all_complete" (set by parallel-finish stub)
    local last_verdict
    last_verdict=$(jq -r '.last_verdict // empty' "$STATE_FILE")
    [ "$last_verdict" == "all_complete" ]

    # Count parallel-finish calls — should be exactly one
    local finish_count
    finish_count=$(grep -c "parallel-finish" "$STUB_DIR/update-state.log" || true)
    [ "$finish_count" -eq 1 ]

    # There should be no separate verdicts_history entry from update_state_after_iteration
    # because the verdict write path is skipped for parallel states (empty verdict passed).
    # The verdicts_history should either not exist or be empty.
    local history_len
    history_len=$(jq '.verdicts_history // [] | length' "$STATE_FILE")
    [ "$history_len" -eq 0 ]
}

# ==============================================================================
# .exit File Glob Filtering
# ==============================================================================

@test "qa_iterate-integration_test_run_parallel_state_exit_file_glob: only .exit files processed" {
    skip_if_not_parallel_integrated

    create_parallel_workflow
    setup_v4_environment "test-plan" "test-phase"

    # run-parallel.sh creates .exit, .log, and .pid files
    cat > "$STUB_DIR/run-parallel.sh" << STUB
#!/bin/bash
echo "\$@" >> "$STUB_DIR/run-parallel.log"
RESULTS="\$3/parallel_\$2"
mkdir -p "\$RESULTS"
echo "0" > "\$RESULTS/branch_a.exit"
echo "agent output" > "\$RESULTS/branch_a.log"
echo "12345" > "\$RESULTS/branch_a.pid"
echo "1" > "\$RESULTS/branch_b.exit"
echo "agent output" > "\$RESULTS/branch_b.log"
echo "12346" > "\$RESULTS/branch_b.pid"
exit 0
STUB
    chmod +x "$STUB_DIR/run-parallel.sh"

    run run_iteration "test-plan" "test-phase" "characterize"
    [ "$status" -eq 0 ]

    # Count parallel-update calls — should be exactly 2 (one per .exit file)
    local update_count
    update_count=$(grep -c "parallel-update" "$STUB_DIR/update-state.log" || true)
    [ "$update_count" -eq 2 ]
}

# ==============================================================================
# V4 Backward Compatibility
# ==============================================================================

@test "qa_iterate-integration_test_v4_workflow_no_parallel: V4 workflow runs normally" {
    skip_if_not_parallel_integrated

    # V4 workflow — no parallel blocks
    create_workflow << 'YAML'
name: test
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    set_state_string "current_state" "impl"
    setup_v4_environment "test-plan" "test-phase"

    run run_iteration "test-plan" "test-phase" "impl"
    [ "$status" -eq 0 ]

    # run-parallel.sh should NOT have been called
    [ ! -f "$STUB_DIR/run-parallel.log" ]

    # State should transition normally
    local current_state
    current_state=$(jq -r '.current_state' "$STATE_FILE")
    [ "$current_state" == "complete" ]
}
