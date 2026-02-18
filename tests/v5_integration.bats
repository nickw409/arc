#!/usr/bin/env bats

# V5 Integration Tests — Cross-component data flow and error propagation
# Phase: integration (orchestration-v5)
#
# Tests the full V5 parallel execution lifecycle focusing on:
# - State.json snapshots at each step of the parallel lifecycle
# - Cross-script data flow (run-parallel → join → update-state)
# - Error propagation (missing workflows, missing prompts)
# - Backward compatibility with V1-V4 workflows
# - Sequential parallel states and mixed parallel/linear workflows
# - Branch parameter rendering
# - Cleanup and orphan process detection
#
# Only the `claude` CLI is mocked via PATH stub. All orchestration scripts
# are real.

setup() {
    load 'test_helper'
    setup_temp_dir

    # Plan/phase directory structure
    export PLANS_DIR="$TEST_TEMP_DIR/plans"
    export ARC_HOME="$TEST_TEMP_DIR/orchestration"
    mkdir -p "$PLANS_DIR/active/test-v5/phases/e2e"
    mkdir -p "$ARC_HOME/scripts"
    mkdir -p "$ARC_HOME/prompts/v5_prompts"
    mkdir -p "$ARC_HOME/prompts/feature"
    mkdir -p "$ARC_HOME/prompts/common"

    # Standard paths
    export PLAN_DIR="$PLANS_DIR/active/test-v5"
    export PHASE_DIR="$PLAN_DIR/phases/e2e"
    export STATE_FILE="$PHASE_DIR/state.json"
    export WORKFLOW_FILE="$PLAN_DIR/workflow.yaml"
    export ARC_DEFAULT_PKG="test-package"
    export VERDICT=""

    # Point SCRIPTS_DIR at real scripts
    export SCRIPTS_DIR="$SCRIPTS_DIR"

    # MOCK_ORCH_DIR tells run-parallel.sh where to find prompts
    export MOCK_ORCH_DIR="$ARC_HOME"

    # Copy fixture prompt files into the orchestration directory
    cp "$BATS_TEST_DIRNAME/fixtures/v5_prompts/"*.md "$ARC_HOME/prompts/v5_prompts/"

    # Create standard prompts for linear states and V4 backward compat
    echo "# Implementation Prompt" > "$ARC_HOME/prompts/feature/impl.md"
    echo "# Review Prompt" > "$ARC_HOME/prompts/feature/impl-review.md"
    echo "# QA Prompt" > "$ARC_HOME/prompts/feature/qa.md"
    echo "# QA Review Prompt" > "$ARC_HOME/prompts/feature/qa-review.md"
    echo "# Complete Prompt" > "$ARC_HOME/prompts/common/complete.md"
    echo "# Blocked Prompt" > "$ARC_HOME/prompts/common/blocked.md"

    # Copy real V4 library scripts so setup_v4_environment can source them
    cp "$ORCH_DIR/scripts/actions.sh" "$ARC_HOME/scripts/"
    cp "$ORCH_DIR/scripts/check-constraints.sh" "$ARC_HOME/scripts/"
    cp "$ORCH_DIR/scripts/check-escalation.sh" "$ARC_HOME/scripts/"
    cp "$ORCH_DIR/scripts/check-intervention.sh" "$ARC_HOME/scripts/"
    cp "$ORCH_DIR/scripts/run-hooks.sh" "$ARC_HOME/scripts/"
    cp "$ORCH_DIR/scripts/extract-verdict.sh" "$ARC_HOME/scripts/" 2>/dev/null || true
    cp "$ORCH_DIR/scripts/build-context.sh" "$ARC_HOME/scripts/" 2>/dev/null || true
    cp "$ORCH_DIR/scripts/render_template.py" "$ARC_HOME/scripts/" 2>/dev/null || true

    # Create minimal state.json
    cat > "$STATE_FILE" << 'JSON'
{
    "plan": "test-v5",
    "phase": "e2e",
    "iteration": 0,
    "current_state": "parallel_step",
    "stuck_iterations": 0,
    "tests_passing": 0,
    "tests_total": 0
}
JSON

    # Claude CLI stub directory — prepend to PATH
    STUB_DIR="$BATS_TEST_TMPDIR/stubs"
    mkdir -p "$STUB_DIR"
    export PATH="$STUB_DIR:$PATH"

    # Control and capture directories for branch-specific behavior
    CONTROL_DIR="$BATS_TEST_TMPDIR/controls"
    CAPTURE_DIR="$BATS_TEST_TMPDIR/captures"
    mkdir -p "$CONTROL_DIR" "$CAPTURE_DIR"

    # Default claude stub: consumes stdin, exits 0
    create_default_claude_stub

    # Source iterate.sh for function definitions
    source "$ORCH_DIR/scripts/iterate.sh"
}

teardown() {
    # Kill any leftover parallel branch processes
    if [[ -d "$PHASE_DIR" ]]; then
        shopt -s nullglob
        for pidfile in "$PHASE_DIR"/parallel_*/*.pid; do
            pid=$(cat "$pidfile" 2>/dev/null) || continue
            if [[ -n "$pid" ]]; then
                kill -KILL -- -"$pid" 2>/dev/null || true
                kill -KILL "$pid" 2>/dev/null || true
            fi
        done
        shopt -u nullglob
    fi
    teardown_temp_dir
}

# ==============================================================================
# Helpers
# ==============================================================================

create_default_claude_stub() {
    cat > "$STUB_DIR/claude" << 'STUB'
#!/bin/bash
cat - > /dev/null
exit 0
STUB
    chmod +x "$STUB_DIR/claude"
}

create_controlled_claude_stub() {
    cat > "$STUB_DIR/claude" << STUB
#!/bin/bash
INPUT=\$(cat -)
BRANCH_NAME="\${PARALLEL_BRANCH_NAME:-unknown}"
CONTROL_FILE="$CONTROL_DIR/\${BRANCH_NAME}.exit_code"
if [[ -f "\$CONTROL_FILE" ]]; then
    exit \$(cat "\$CONTROL_FILE")
fi
exit 0
STUB
    chmod +x "$STUB_DIR/claude"
}

create_slow_claude_stub() {
    cat > "$STUB_DIR/claude" << 'STUB'
#!/bin/bash
cat - > /dev/null
exec sleep 999
STUB
    chmod +x "$STUB_DIR/claude"
}

create_capturing_claude_stub() {
    cat > "$STUB_DIR/claude" << STUB
#!/bin/bash
BRANCH_NAME="\${PARALLEL_BRANCH_NAME:-unknown}"
cat - > "$CAPTURE_DIR/\${BRANCH_NAME}.stdin"
exit 0
STUB
    chmod +x "$STUB_DIR/claude"
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

# Helper: Run run_iteration in an E2E subshell with real scripts
run_e2e() {
    local plan_name="${1:-test-v5}"
    local phase_name="${2:-e2e}"
    local state_name="${3:-parallel_step}"
    local extra_setup="${4:-}"

    run bash -c "
        export PHASE_DIR=\"$PHASE_DIR\"
        export STATE_FILE=\"$STATE_FILE\"
        export WORKFLOW_FILE=\"$WORKFLOW_FILE\"
        export ARC_HOME=\"$ARC_HOME\"
        export SCRIPTS_DIR=\"$SCRIPTS_DIR\"
        export PLANS_DIR=\"$PLANS_DIR\"
        export ARC_DEFAULT_PKG=\"${ARC_DEFAULT_PKG:-test-package}\"
        export VERDICT=\"${VERDICT:-}\"
        export MOCK_ORCH_DIR=\"$ARC_HOME\"
        export PATH=\"$STUB_DIR:\$PATH\"

        # Source iterate.sh to get run_iteration, setup_v4_environment, etc.
        set +e
        source \"$ORCH_DIR/scripts/iterate.sh\" 2>/dev/null
        set -e

        if ! declare -f run_iteration > /dev/null 2>&1; then
            echo 'FUNCTION_NOT_FOUND: run_iteration'
            exit 1
        fi

        # Mock spawn_agent for linear states (parallel states use run-parallel.sh)
        spawn_agent() {
            local prompt_file=\"\$1\"
            local output_file=\"\$2\"
            echo \"Agent output\" > \"\$output_file\"
            return 0
        }

        # Apply extra setup (may override spawn_agent or other mocks)
        $extra_setup

        setup_v4_environment \"$plan_name\" \"$phase_name\"
        run_iteration \"$plan_name\" \"$phase_name\" \"$state_name\"
    "
}

# Helper: Run iteration loop until terminal state
run_e2e_loop() {
    local plan_name="${1:-test-v5}"
    local phase_name="${2:-e2e}"
    local extra_setup="${3:-}"

    run bash -c "
        export PHASE_DIR=\"$PHASE_DIR\"
        export STATE_FILE=\"$STATE_FILE\"
        export WORKFLOW_FILE=\"$WORKFLOW_FILE\"
        export ARC_HOME=\"$ARC_HOME\"
        export SCRIPTS_DIR=\"$SCRIPTS_DIR\"
        export PLANS_DIR=\"$PLANS_DIR\"
        export ARC_DEFAULT_PKG=\"${ARC_DEFAULT_PKG:-test-package}\"
        export VERDICT=\"${VERDICT:-}\"
        export MOCK_ORCH_DIR=\"$ARC_HOME\"
        export PATH=\"$STUB_DIR:\$PATH\"

        set +e
        source \"$ORCH_DIR/scripts/iterate.sh\" 2>/dev/null
        set -e

        if ! declare -f run_iteration > /dev/null 2>&1; then
            echo 'FUNCTION_NOT_FOUND: run_iteration'
            exit 1
        fi

        spawn_agent() {
            local prompt_file=\"\$1\"
            local output_file=\"\$2\"
            echo \"Agent output\" > \"\$output_file\"
            return 0
        }

        $extra_setup

        setup_v4_environment \"$plan_name\" \"$phase_name\"

        STATE=\$(jq -r '.current_state' \"$STATE_FILE\")
        TERMINAL_STATES=\$(yq '.terminal_states[]' \"$WORKFLOW_FILE\")
        ITERATION_COUNT=0
        MAX_ITERATIONS=20

        while ! echo \"\$TERMINAL_STATES\" | grep -qx \"\$STATE\"; do
            run_iteration \"$plan_name\" \"$phase_name\" \"\$STATE\"
            STATE=\$(jq -r '.current_state' \"$STATE_FILE\")
            ITERATION_COUNT=\$((ITERATION_COUNT + 1))
            if [[ \$ITERATION_COUNT -ge \$MAX_ITERATIONS ]]; then
                echo \"ERROR: Max iterations exceeded\"
                exit 1
            fi
        done
        echo \"ITERATIONS_COMPLETED=\$ITERATION_COUNT\"
    "
}

# ==============================================================================
# Test: Full V5 Lifecycle — State.json Snapshots
# ==============================================================================

@test "qa_integration_test_full_v5_lifecycle_state_json_snapshots" {
    # Setup: V5 workflow with 3 branches (strategy: all)
    cat > "$WORKFLOW_FILE" << 'YAML'
name: test-lifecycle
version: 5
defaults:
  timeout: 60
states:
  - name: parallel_step
    parallel:
      strategy: all
      branches:
        - name: a
          prompt: prompts/v5_prompts/branch_pass.md
        - name: b
          prompt: prompts/v5_prompts/branch_pass.md
        - name: c
          prompt: prompts/v5_prompts/branch_pass.md
    next:
      all_complete: done
      any_failed: done
  - name: done
    description: Done
entry_state: parallel_step
terminal_states:
  - done
YAML

    local RESULTS_DIR="$PHASE_DIR/parallel_parallel_step"

    # Step 1: parallel-start — should create .parallel_execution with 3 branches
    run "$SCRIPTS_DIR/update-state.sh" test-v5 e2e parallel-start "$RESULTS_DIR" "a,b,c"
    [ "$status" -eq 0 ]

    # Assert: 3 branches in parallel_execution
    local branch_count
    branch_count=$(jq '.parallel_execution.branches | keys | length' "$STATE_FILE")
    [ "$branch_count" -eq 3 ]
    [ "$(jq -r '.parallel_execution.branches.a.status' "$STATE_FILE")" == "running" ]
    [ "$(jq -r '.parallel_execution.branches.b.status' "$STATE_FILE")" == "running" ]
    [ "$(jq -r '.parallel_execution.branches.c.status' "$STATE_FILE")" == "running" ]

    # Step 2: run-parallel.sh — spawns all branches
    run "$SCRIPTS_DIR/run-parallel.sh" "$WORKFLOW_FILE" "parallel_step" "$PHASE_DIR" "test-v5" "e2e"
    [ "$status" -eq 0 ]

    # Verify exit files created
    [ -f "$RESULTS_DIR/a.exit" ]
    [ -f "$RESULTS_DIR/b.exit" ]
    [ -f "$RESULTS_DIR/c.exit" ]

    # Step 3: Process .exit files and update branch status
    for exit_file in "$RESULTS_DIR"/*.exit; do
        local bname
        bname=$(basename "${exit_file%.exit}")
        local ecode
        ecode=$(cat "$exit_file")
        local bstatus
        if [[ "$ecode" -eq 0 ]]; then
            bstatus="complete"
        else
            bstatus="failed"
        fi
        "$SCRIPTS_DIR/update-state.sh" test-v5 e2e parallel-update "$bname" "$bstatus" "$ecode"
    done

    # Assert: branch a is complete
    [ "$(jq -r '.parallel_execution.branches.a.status' "$STATE_FILE")" == "complete" ]
    [ "$(jq -r '.parallel_execution.branches.b.status' "$STATE_FILE")" == "complete" ]
    [ "$(jq -r '.parallel_execution.branches.c.status' "$STATE_FILE")" == "complete" ]

    # Step 4-5: join-parallel and parallel-finish
    local verdict
    verdict=$("$SCRIPTS_DIR/join-parallel.sh" all "$RESULTS_DIR")
    [ "$verdict" == "all_complete" ]

    "$SCRIPTS_DIR/update-state.sh" test-v5 e2e parallel-finish "$verdict"

    # Assert: last_verdict set
    [ "$(jq -r '.last_verdict' "$STATE_FILE")" == "all_complete" ]

    # Step 6: parallel-clear
    "$SCRIPTS_DIR/update-state.sh" test-v5 e2e parallel-clear

    # Assert: parallel_execution removed
    local pe
    pe=$(jq -r '.parallel_execution // "absent"' "$STATE_FILE")
    [ "$pe" == "absent" ]

    # last_verdict should persist
    [ "$(jq -r '.last_verdict' "$STATE_FILE")" == "all_complete" ]
}

# ==============================================================================
# Test: Run-Parallel to Join Data Flow
# ==============================================================================

@test "qa_integration_test_run_parallel_to_join_data_flow" {
    # Setup: Controlled stub where branch c fails
    create_controlled_claude_stub
    echo "0" > "$CONTROL_DIR/a.exit_code"
    echo "0" > "$CONTROL_DIR/b.exit_code"
    echo "1" > "$CONTROL_DIR/c.exit_code"

    cat > "$WORKFLOW_FILE" << 'YAML'
name: test-data-flow
version: 5
defaults:
  timeout: 60
states:
  - name: parallel_step
    parallel:
      strategy: all
      branches:
        - name: a
          prompt: prompts/v5_prompts/branch_pass.md
        - name: b
          prompt: prompts/v5_prompts/branch_pass.md
        - name: c
          prompt: prompts/v5_prompts/branch_pass.md
    next:
      all_complete: done
      any_failed: blocked
  - name: done
    description: Done
  - name: blocked
    description: Blocked
entry_state: parallel_step
terminal_states:
  - done
  - blocked
YAML

    local RESULTS_DIR="$PHASE_DIR/parallel_parallel_step"

    # Execute: run-parallel.sh spawns branches
    run "$SCRIPTS_DIR/run-parallel.sh" "$WORKFLOW_FILE" "parallel_step" "$PHASE_DIR" "test-v5" "e2e"
    [ "$status" -eq 0 ]

    # Assert: .exit files have correct values
    [ "$(cat "$RESULTS_DIR/a.exit")" -eq 0 ]
    [ "$(cat "$RESULTS_DIR/b.exit")" -eq 0 ]
    [ "$(cat "$RESULTS_DIR/c.exit")" -eq 1 ]

    # Execute: join-parallel
    local verdict
    verdict=$("$SCRIPTS_DIR/join-parallel.sh" all "$RESULTS_DIR")

    # Assert: verdict is any_failed because c failed
    [ "$verdict" == "any_failed" ]
}

# ==============================================================================
# Test: Join Verdict Matches Schema Validation (6 scenarios)
# ==============================================================================

@test "qa_integration_test_join_verdict_matches_schema_validation" {
    # Scenario 1: strategy=all, all exit=0 → all_complete
    local dir1="$TEST_TEMP_DIR/results1"
    mkdir -p "$dir1"
    echo "0" > "$dir1/a.exit"
    echo "0" > "$dir1/b.exit"
    local v1
    v1=$("$SCRIPTS_DIR/join-parallel.sh" all "$dir1")
    [ "$v1" == "all_complete" ]

    # Scenario 2: strategy=all, one exit=1 → any_failed
    local dir2="$TEST_TEMP_DIR/results2"
    mkdir -p "$dir2"
    echo "0" > "$dir2/a.exit"
    echo "1" > "$dir2/b.exit"
    local v2
    v2=$("$SCRIPTS_DIR/join-parallel.sh" all "$dir2")
    [ "$v2" == "any_failed" ]

    # Scenario 3: strategy=any, one exit=0 → first_complete
    local dir3="$TEST_TEMP_DIR/results3"
    mkdir -p "$dir3"
    echo "0" > "$dir3/a.exit"
    echo "1" > "$dir3/b.exit"
    local v3
    v3=$("$SCRIPTS_DIR/join-parallel.sh" any "$dir3")
    [ "$v3" == "first_complete" ]

    # Scenario 4: strategy=any, all exit=1 → all_failed
    local dir4="$TEST_TEMP_DIR/results4"
    mkdir -p "$dir4"
    echo "1" > "$dir4/a.exit"
    echo "1" > "$dir4/b.exit"
    local v4
    v4=$("$SCRIPTS_DIR/join-parallel.sh" any "$dir4")
    [ "$v4" == "all_failed" ]

    # Scenario 5: strategy=n_of_m n=1, one exit=0 → n_complete
    local dir5="$TEST_TEMP_DIR/results5"
    mkdir -p "$dir5"
    echo "0" > "$dir5/a.exit"
    echo "1" > "$dir5/b.exit"
    echo "1" > "$dir5/c.exit"
    local v5
    v5=$("$SCRIPTS_DIR/join-parallel.sh" n_of_m "$dir5" 1)
    [ "$v5" == "n_complete" ]

    # Scenario 6: strategy=n_of_m n=3, only 1 exit=0 out of 2 → insufficient
    local dir6="$TEST_TEMP_DIR/results6"
    mkdir -p "$dir6"
    echo "0" > "$dir6/a.exit"
    echo "1" > "$dir6/b.exit"
    local v6
    v6=$("$SCRIPTS_DIR/join-parallel.sh" n_of_m "$dir6" 3)
    [ "$v6" == "insufficient" ]
}

# ==============================================================================
# Test: Error Propagation — Missing Workflow
# ==============================================================================

@test "qa_integration_test_error_propagation_missing_workflow" {
    # No workflow file — run-parallel.sh should fail
    run "$SCRIPTS_DIR/run-parallel.sh" "/nonexistent/workflow.yaml" "parallel_step" "$PHASE_DIR" "test-v5" "e2e"
    [ "$status" -eq 1 ]

    # state.json should be unchanged — no parallel_execution key
    local pe
    pe=$(jq -r '.parallel_execution // "absent"' "$STATE_FILE")
    [ "$pe" == "absent" ]
}

# ==============================================================================
# Test: Error Propagation — Missing Prompt
# ==============================================================================

@test "qa_integration_test_error_propagation_missing_prompt" {
    cat > "$WORKFLOW_FILE" << 'YAML'
name: test-missing-prompt
version: 5
defaults:
  timeout: 60
states:
  - name: parallel_step
    parallel:
      strategy: all
      branches:
        - name: branch_a
          prompt: prompts/v5_prompts/this_does_not_exist.md
    next:
      all_complete: done
  - name: done
    description: Done
entry_state: parallel_step
terminal_states:
  - done
YAML

    local RESULTS_DIR="$PHASE_DIR/parallel_parallel_step"

    run "$SCRIPTS_DIR/run-parallel.sh" "$WORKFLOW_FILE" "parallel_step" "$PHASE_DIR" "test-v5" "e2e"
    [ "$status" -eq 1 ]

    # No .pid or .exit files should exist (failed before spawning)
    if [[ -d "$RESULTS_DIR" ]]; then
        shopt -s nullglob
        local pid_files=("$RESULTS_DIR"/*.pid)
        local exit_files=("$RESULTS_DIR"/*.exit)
        shopt -u nullglob
        [ ${#pid_files[@]} -eq 0 ]
        [ ${#exit_files[@]} -eq 0 ]
    fi
}

# ==============================================================================
# Test: Cleanup — No Orphans After SIGTERM
# ==============================================================================

@test "qa_integration_test_cleanup_no_orphans" {
    create_slow_claude_stub

    cat > "$WORKFLOW_FILE" << 'YAML'
name: test-cleanup
version: 5
defaults:
  timeout: 600
states:
  - name: parallel_step
    parallel:
      strategy: all
      branches:
        - name: branch_a
          prompt: prompts/v5_prompts/branch_pass.md
        - name: branch_b
          prompt: prompts/v5_prompts/branch_pass.md
    next:
      all_complete: done
      any_failed: done
  - name: done
    description: Done
entry_state: parallel_step
terminal_states:
  - done
YAML

    local RESULTS_DIR="$PHASE_DIR/parallel_parallel_step"

    # Start run-parallel.sh in background
    "$SCRIPTS_DIR/run-parallel.sh" "$WORKFLOW_FILE" "parallel_step" "$PHASE_DIR" "test-v5" "e2e" &
    local RUN_PARALLEL_PID=$!

    # Wait up to 10 seconds for .pid files to appear
    local waited=0
    while [[ $waited -lt 10 ]]; do
        if [[ -d "$RESULTS_DIR" ]]; then
            shopt -s nullglob
            local pidfiles=("$RESULTS_DIR"/*.pid)
            shopt -u nullglob
            if [[ ${#pidfiles[@]} -ge 2 ]]; then
                break
            fi
        fi
        sleep 1
        waited=$((waited + 1))
    done

    # Collect branch PIDs
    local BRANCH_PIDS=()
    if [[ -d "$RESULTS_DIR" ]]; then
        shopt -s nullglob
        for pidfile in "$RESULTS_DIR"/*.pid; do
            local pid
            pid=$(cat "$pidfile" 2>/dev/null) || continue
            if [[ -n "$pid" ]]; then
                BRANCH_PIDS+=("$pid")
            fi
        done
        shopt -u nullglob
    fi

    # Kill run-parallel.sh to trigger cleanup trap
    kill -TERM "$RUN_PARALLEL_PID" 2>/dev/null || true

    # Wait for cleanup to complete (up to 15 seconds)
    waited=0
    while [[ $waited -lt 15 ]]; do
        local any_alive=false
        for pid in "${BRANCH_PIDS[@]}"; do
            if kill -0 "$pid" 2>/dev/null; then
                any_alive=true
                break
            fi
        done
        if [[ "$any_alive" == "false" ]]; then
            break
        fi
        sleep 1
        waited=$((waited + 1))
    done

    # Assert: all branch processes should be dead
    for pid in "${BRANCH_PIDS[@]}"; do
        ! kill -0 "$pid" 2>/dev/null
    done
}

# ==============================================================================
# Test: V4 Backward Compatibility — Escalation
# ==============================================================================

@test "qa_integration_test_v4_backward_compat_escalation" {
    # Use the real feature.yaml (V2 workflow — no parallel blocks)
    cp "$WORKFLOWS_DIR/feature.yaml" "$WORKFLOW_FILE"

    local entry_state
    entry_state=$(yq '.entry_state' "$WORKFLOW_FILE")
    set_state_string "current_state" "$entry_state"

    # Claude stub that logs invocations
    cat > "$STUB_DIR/claude" << STUB
#!/bin/bash
cat - > /dev/null
echo "invoked" >> "$STUB_DIR/claude.log"
exit 0
STUB
    chmod +x "$STUB_DIR/claude"

    run_e2e "test-v5" "e2e" "$entry_state"
    [ "$status" -eq 0 ]

    # No parallel_execution key should exist
    local pe
    pe=$(jq -r '.parallel_execution // "absent"' "$STATE_FILE")
    [ "$pe" == "absent" ]
}

# ==============================================================================
# Test: V1 Linear Backward Compatibility
# ==============================================================================

@test "qa_integration_test_v1_linear_backward_compat" {
    cat > "$WORKFLOW_FILE" << 'YAML'
name: test-v1
version: 1
states:
  - name: run
    prompt: prompts/v5_prompts/branch_pass.md
    next: done
  - name: done
    description: Done
entry_state: run
terminal_states:
  - done
YAML

    set_state_string "current_state" "run"

    run_e2e "test-v5" "e2e" "run"
    [ "$status" -eq 0 ]

    # No parallel_execution key
    local pe
    pe=$(jq -r '.parallel_execution // "absent"' "$STATE_FILE")
    [ "$pe" == "absent" ]

    # current_state should advance to "done"
    local current_state
    current_state=$(jq -r '.current_state' "$STATE_FILE")
    [ "$current_state" == "done" ]
}

# ==============================================================================
# Test: Version Field Missing Defaults to V1
# ==============================================================================

@test "qa_integration_test_version_field_missing_defaults_to_v1" {
    # Workflow with no version field
    cat > "$WORKFLOW_FILE" << 'YAML'
name: test-no-version
states:
  - name: run
    prompt: prompts/v5_prompts/branch_pass.md
    next: done
  - name: done
    description: Done
entry_state: run
terminal_states:
  - done
YAML

    set_state_string "current_state" "run"

    run_e2e "test-v5" "e2e" "run"
    [ "$status" -eq 0 ]

    # No parallel_execution key (no parallel block → no parallel detection)
    local pe
    pe=$(jq -r '.parallel_execution // "absent"' "$STATE_FILE")
    [ "$pe" == "absent" ]

    # current_state should advance normally
    local current_state
    current_state=$(jq -r '.current_state' "$STATE_FILE")
    [ "$current_state" == "done" ]
}

# ==============================================================================
# Test: Parallel State as Entry State
# ==============================================================================

@test "qa_integration_test_parallel_state_as_entry_state" {
    cat > "$WORKFLOW_FILE" << 'YAML'
name: test-entry
version: 5
defaults:
  timeout: 60
states:
  - name: parallel_step
    parallel:
      strategy: all
      branches:
        - name: branch_a
          prompt: prompts/v5_prompts/branch_pass.md
        - name: branch_b
          prompt: prompts/v5_prompts/branch_pass.md
    next:
      all_complete: done
      any_failed: done
  - name: done
    description: Done
entry_state: parallel_step
terminal_states:
  - done
YAML

    # current_state is already "parallel_step" from setup
    create_default_claude_stub

    run_e2e "test-v5" "e2e" "parallel_step"
    [ "$status" -eq 0 ]

    # Should have reached done
    local current_state
    current_state=$(jq -r '.current_state' "$STATE_FILE")
    [ "$current_state" == "done" ]

    # parallel_execution should be absent (cleared)
    local pe
    pe=$(jq -r '.parallel_execution // "absent"' "$STATE_FILE")
    [ "$pe" == "absent" ]
}

# ==============================================================================
# Test: All Branches Fail — Transition to Blocked
# ==============================================================================

@test "qa_integration_test_all_fail_transition" {
    create_controlled_claude_stub
    echo "1" > "$CONTROL_DIR/branch_a.exit_code"
    echo "1" > "$CONTROL_DIR/branch_b.exit_code"

    cat > "$WORKFLOW_FILE" << 'YAML'
name: test-allfail
version: 5
defaults:
  timeout: 60
states:
  - name: parallel_step
    parallel:
      strategy: all
      branches:
        - name: branch_a
          prompt: prompts/v5_prompts/branch_pass.md
        - name: branch_b
          prompt: prompts/v5_prompts/branch_pass.md
    next:
      all_complete: done
      any_failed: blocked
  - name: done
    description: Done
  - name: blocked
    description: Blocked
entry_state: parallel_step
terminal_states:
  - done
  - blocked
YAML

    run_e2e "test-v5" "e2e" "parallel_step"
    [ "$status" -eq 0 ]

    # Should transition to blocked
    local current_state
    current_state=$(jq -r '.current_state' "$STATE_FILE")
    [ "$current_state" == "blocked" ]

    # Verdict should be any_failed
    local last_verdict
    last_verdict=$(jq -r '.last_verdict // empty' "$STATE_FILE")
    [ "$last_verdict" == "any_failed" ]
}

# ==============================================================================
# Test: Parallel → Linear Transition (Mixed Workflow)
# ==============================================================================

@test "qa_integration_test_parallel_then_linear_transition" {
    cat > "$WORKFLOW_FILE" << 'YAML'
name: test-mixed
version: 5
defaults:
  timeout: 60
states:
  - name: parallel_step
    parallel:
      strategy: all
      branches:
        - name: branch_a
          prompt: prompts/v5_prompts/branch_pass.md
        - name: branch_b
          prompt: prompts/v5_prompts/branch_pass.md
    next:
      all_complete: linear_state
      any_failed: complete
  - name: linear_state
    prompt: prompts/v5_prompts/branch_pass.md
    next: complete
  - name: complete
    description: Complete
entry_state: parallel_step
terminal_states:
  - complete
YAML

    create_default_claude_stub

    # First iteration: parallel state
    run_e2e "test-v5" "e2e" "parallel_step"
    [ "$status" -eq 0 ]

    # After first call: should transition to linear_state
    local current_state
    current_state=$(jq -r '.current_state' "$STATE_FILE")
    [ "$current_state" == "linear_state" ]

    # parallel_execution should be absent
    local pe
    pe=$(jq -r '.parallel_execution // "absent"' "$STATE_FILE")
    [ "$pe" == "absent" ]

    # Second iteration: linear state
    run_e2e "test-v5" "e2e" "linear_state"
    [ "$status" -eq 0 ]

    # After second call: should reach complete
    current_state=$(jq -r '.current_state' "$STATE_FILE")
    [ "$current_state" == "complete" ]
}

# ==============================================================================
# Test: Sequential Parallel States
# ==============================================================================

@test "qa_integration_test_sequential_parallel_states" {
    cat > "$WORKFLOW_FILE" << 'YAML'
name: test-sequential
version: 5
defaults:
  timeout: 60
states:
  - name: parallel_1
    parallel:
      strategy: all
      branches:
        - name: branch_a
          prompt: prompts/v5_prompts/branch_pass.md
        - name: branch_b
          prompt: prompts/v5_prompts/branch_pass.md
    next:
      all_complete: parallel_2
      any_failed: done
  - name: parallel_2
    parallel:
      strategy: all
      branches:
        - name: branch_c
          prompt: prompts/v5_prompts/branch_pass.md
        - name: branch_d
          prompt: prompts/v5_prompts/branch_pass.md
    next:
      all_complete: done
      any_failed: done
  - name: done
    description: Done
entry_state: parallel_1
terminal_states:
  - done
YAML

    set_state_string "current_state" "parallel_1"
    create_default_claude_stub

    # First iteration: parallel_1
    run_e2e "test-v5" "e2e" "parallel_1"
    [ "$status" -eq 0 ]

    # Verify results directory for parallel_1
    [ -d "$PHASE_DIR/parallel_parallel_1" ]

    # After first iteration, parallel_execution should be absent (cleared)
    local pe
    pe=$(jq -r '.parallel_execution // "absent"' "$STATE_FILE")
    [ "$pe" == "absent" ]

    # Should have advanced to parallel_2
    local current_state
    current_state=$(jq -r '.current_state' "$STATE_FILE")
    [ "$current_state" == "parallel_2" ]

    # Second iteration: parallel_2
    run_e2e "test-v5" "e2e" "parallel_2"
    [ "$status" -eq 0 ]

    # Verify results directory for parallel_2
    [ -d "$PHASE_DIR/parallel_parallel_2" ]

    # After second iteration
    local last_verdict
    last_verdict=$(jq -r '.last_verdict' "$STATE_FILE")
    [ "$last_verdict" == "all_complete" ]

    current_state=$(jq -r '.current_state' "$STATE_FILE")
    [ "$current_state" == "done" ]
}

# ==============================================================================
# Test: Parallel State with Branch Params (Template Rendering)
# ==============================================================================

@test "qa_integration_test_parallel_state_with_branch_params" {
    cat > "$WORKFLOW_FILE" << 'YAML'
name: test-params
version: 5
defaults:
  timeout: 60
states:
  - name: parallel_step
    parallel:
      strategy: all
      branches:
        - name: auth
          params:
            module: auth
          prompt: prompts/v5_prompts/branch_template.md
        - name: api
          params:
            module: api
          prompt: prompts/v5_prompts/branch_template.md
    next:
      all_complete: done
      any_failed: done
  - name: done
    description: Done
entry_state: parallel_step
terminal_states:
  - done
YAML

    create_capturing_claude_stub

    run_e2e "test-v5" "e2e" "parallel_step"
    [ "$status" -eq 0 ]

    # Verify captured stdin contains rendered templates
    [ -f "$CAPTURE_DIR/auth.stdin" ]
    [ -f "$CAPTURE_DIR/api.stdin" ]

    local auth_content
    auth_content=$(cat "$CAPTURE_DIR/auth.stdin")
    [[ "$auth_content" == *"Module: auth"* ]]

    local api_content
    api_content=$(cat "$CAPTURE_DIR/api.stdin")
    [[ "$api_content" == *"Module: api"* ]]
}

# ==============================================================================
# Test: n_of_m Boundary — Exactly n Succeed
# ==============================================================================

@test "qa_integration_test_n_of_m_boundary" {
    create_controlled_claude_stub
    echo "0" > "$CONTROL_DIR/a.exit_code"
    echo "0" > "$CONTROL_DIR/b.exit_code"
    echo "1" > "$CONTROL_DIR/c.exit_code"

    cat > "$WORKFLOW_FILE" << 'YAML'
name: test-nom
version: 5
defaults:
  timeout: 60
states:
  - name: parallel_step
    parallel:
      strategy: n_of_m
      n: 2
      branches:
        - name: a
          prompt: prompts/v5_prompts/branch_pass.md
        - name: b
          prompt: prompts/v5_prompts/branch_pass.md
        - name: c
          prompt: prompts/v5_prompts/branch_pass.md
    next:
      n_complete: done
      insufficient: blocked
  - name: done
    description: Done
  - name: blocked
    description: Blocked
entry_state: parallel_step
terminal_states:
  - done
  - blocked
YAML

    run_e2e "test-v5" "e2e" "parallel_step"
    [ "$status" -eq 0 ]

    # Exactly 2 succeeded, n=2, so verdict should be n_complete
    local last_verdict
    last_verdict=$(jq -r '.last_verdict // empty' "$STATE_FILE")
    [ "$last_verdict" == "n_complete" ]

    # Should transition to done
    local current_state
    current_state=$(jq -r '.current_state' "$STATE_FILE")
    [ "$current_state" == "done" ]
}
