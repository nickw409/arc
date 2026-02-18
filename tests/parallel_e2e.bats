#!/usr/bin/env bats

# V5 End-to-End Parallel Execution Tests
# Phase: integration-testing (orchestration-v5)
#
# Tests the full parallel execution pipeline using real scripts:
# - validate-workflow.sh (V5 validation)
# - run-parallel.sh (branch spawning)
# - join-parallel.sh (result evaluation)
# - update-state.sh (state tracking)
# - iterate.sh (run_iteration with parallel detection)
#
# Only the `claude` CLI is mocked via PATH stub. All orchestration scripts
# are real. This verifies end-to-end integration.

setup() {
    load 'test_helper'
    setup_temp_dir

    # Plan/phase directory structure
    export PLANS_DIR="$TEST_TEMP_DIR/plans"
    export ARC_HOME="$TEST_TEMP_DIR/orchestration"
    mkdir -p "$PLANS_DIR/active/test-plan/phases/test-phase"
    mkdir -p "$ARC_HOME/scripts"
    mkdir -p "$ARC_HOME/prompts/v5_prompts"
    mkdir -p "$ARC_HOME/prompts/feature"
    mkdir -p "$ARC_HOME/prompts/common"

    # Standard paths
    export PLAN_DIR="$PLANS_DIR/active/test-plan"
    export PHASE_DIR="$PLAN_DIR/phases/test-phase"
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
    "plan": "test-plan",
    "phase": "test-phase",
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

# Default claude stub: consumes stdin, exits 0
create_default_claude_stub() {
    cat > "$STUB_DIR/claude" << 'STUB'
#!/bin/bash
cat - > /dev/null
exit 0
STUB
    chmod +x "$STUB_DIR/claude"
}

# Branch-controlled claude stub: reads exit code from control file per branch
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

# Slow claude stub: sleeps forever (for timeout tests)
create_slow_claude_stub() {
    cat > "$STUB_DIR/claude" << 'STUB'
#!/bin/bash
cat - > /dev/null
exec sleep 999
STUB
    chmod +x "$STUB_DIR/claude"
}

# Capturing claude stub: writes stdin to capture file per branch
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

# Helper: Copy fixture workflow to plan directory
use_fixture_workflow() {
    local fixture_name="$1"
    cp "$BATS_TEST_DIRNAME/fixtures/$fixture_name" "$WORKFLOW_FILE"
}

# Helper: Run run_iteration in an E2E subshell with real scripts
# This sources iterate.sh, sources V4 libraries, mocks only spawn_agent
# (for linear states), and runs run_iteration.
run_e2e() {
    local plan_name="${1:-test-plan}"
    local phase_name="${2:-test-phase}"
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
    local plan_name="${1:-test-plan}"
    local phase_name="${2:-test-phase}"
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
# Workflow Validation Tests
# ==============================================================================

@test "qa_integration-testing_test_e2e_validate_v5_workflow: validate-workflow.sh accepts valid V5 workflow" {
    use_fixture_workflow "v5_workflow_all.yaml"

    # Need prompt files to exist relative to ORCH_DIR for validation
    run "$SCRIPTS_DIR/validate-workflow.sh" "$WORKFLOW_FILE"
    [ "$status" -eq 0 ]
}

@test "qa_integration-testing_test_e2e_validate_rejects_invalid_v5: rejects invalid parallel strategy" {
    cat > "$WORKFLOW_FILE" << 'YAML'
name: test-invalid
version: 5
defaults:
  timeout: 60
states:
  - name: parallel_step
    parallel:
      strategy: first
      branches:
        - name: branch_a
          prompt: prompts/v5_prompts/branch_pass.md
    verdicts:
      - first_complete
      - all_failed
    next:
      first_complete: done
      all_failed: blocked
  - name: done
    description: Success
  - name: blocked
    description: Failure
entry_state: parallel_step
terminal_states:
  - done
  - blocked
YAML

    run "$SCRIPTS_DIR/validate-workflow.sh" "$WORKFLOW_FILE"
    [ "$status" -eq 1 ]
    [[ "${output,,}" == *"invalid parallel strategy"* ]] || [[ "${output,,}" == *"invalid"*"strategy"* ]]
}

# ==============================================================================
# Strategy "all" Tests
# ==============================================================================

@test "qa_integration-testing_test_e2e_parallel_all_pass: all branches pass gives all_complete" {
    use_fixture_workflow "v5_workflow_all.yaml"
    create_default_claude_stub

    run_e2e "test-plan" "test-phase" "parallel_step"
    [ "$status" -eq 0 ]

    # Verify results directory was created with .exit files
    local results_dir="$PHASE_DIR/parallel_parallel_step"
    [ -d "$results_dir" ]

    # Both branches should have exit code 0
    [ -f "$results_dir/branch_a.exit" ]
    [ -f "$results_dir/branch_b.exit" ]
    [ "$(cat "$results_dir/branch_a.exit")" -eq 0 ]
    [ "$(cat "$results_dir/branch_b.exit")" -eq 0 ]

    # Verdict should be all_complete
    local last_verdict
    last_verdict=$(jq -r '.last_verdict // empty' "$STATE_FILE")
    [ "$last_verdict" == "all_complete" ]
}

@test "qa_integration-testing_test_e2e_parallel_all_one_fails: one failure gives any_failed" {
    use_fixture_workflow "v5_workflow_all.yaml"
    create_controlled_claude_stub

    # branch_a passes, branch_b fails
    echo "0" > "$CONTROL_DIR/branch_a.exit_code"
    echo "1" > "$CONTROL_DIR/branch_b.exit_code"

    run_e2e "test-plan" "test-phase" "parallel_step"
    [ "$status" -eq 0 ]

    local last_verdict
    last_verdict=$(jq -r '.last_verdict // empty' "$STATE_FILE")
    [ "$last_verdict" == "any_failed" ]
}

@test "qa_integration-testing_test_e2e_parallel_all_all_fail: all fail gives any_failed" {
    use_fixture_workflow "v5_workflow_all.yaml"
    create_controlled_claude_stub

    echo "1" > "$CONTROL_DIR/branch_a.exit_code"
    echo "1" > "$CONTROL_DIR/branch_b.exit_code"

    run_e2e "test-plan" "test-phase" "parallel_step"
    [ "$status" -eq 0 ]

    local last_verdict
    last_verdict=$(jq -r '.last_verdict // empty' "$STATE_FILE")
    [ "$last_verdict" == "any_failed" ]

    local current_state
    current_state=$(jq -r '.current_state' "$STATE_FILE")
    [ "$current_state" == "blocked" ]
}

# ==============================================================================
# Strategy "any" Tests
# ==============================================================================

@test "qa_integration-testing_test_e2e_parallel_any_one_passes: one pass gives first_complete" {
    use_fixture_workflow "v5_workflow_any.yaml"
    create_controlled_claude_stub

    echo "0" > "$CONTROL_DIR/branch_a.exit_code"
    echo "1" > "$CONTROL_DIR/branch_b.exit_code"

    run_e2e "test-plan" "test-phase" "parallel_step"
    [ "$status" -eq 0 ]

    local last_verdict
    last_verdict=$(jq -r '.last_verdict // empty' "$STATE_FILE")
    [ "$last_verdict" == "first_complete" ]

    local current_state
    current_state=$(jq -r '.current_state' "$STATE_FILE")
    [ "$current_state" == "done" ]
}

# ==============================================================================
# Strategy "n_of_m" Tests
# ==============================================================================

@test "qa_integration-testing_test_e2e_parallel_n_of_m: 1 of 3 pass with n=1 gives n_complete" {
    use_fixture_workflow "v5_workflow_n_of_m.yaml"
    create_controlled_claude_stub

    echo "0" > "$CONTROL_DIR/branch_a.exit_code"
    echo "1" > "$CONTROL_DIR/branch_b.exit_code"
    echo "1" > "$CONTROL_DIR/branch_c.exit_code"

    run_e2e "test-plan" "test-phase" "parallel_step"
    [ "$status" -eq 0 ]

    local last_verdict
    last_verdict=$(jq -r '.last_verdict // empty' "$STATE_FILE")
    [ "$last_verdict" == "n_complete" ]

    local current_state
    current_state=$(jq -r '.current_state' "$STATE_FILE")
    [ "$current_state" == "done" ]
}

@test "qa_integration-testing_test_e2e_parallel_n_of_m_all_fail: all fail gives insufficient" {
    use_fixture_workflow "v5_workflow_n_of_m.yaml"
    create_controlled_claude_stub

    echo "1" > "$CONTROL_DIR/branch_a.exit_code"
    echo "1" > "$CONTROL_DIR/branch_b.exit_code"
    echo "1" > "$CONTROL_DIR/branch_c.exit_code"

    run_e2e "test-plan" "test-phase" "parallel_step"
    [ "$status" -eq 0 ]

    local last_verdict
    last_verdict=$(jq -r '.last_verdict // empty' "$STATE_FILE")
    [ "$last_verdict" == "insufficient" ]

    local current_state
    current_state=$(jq -r '.current_state' "$STATE_FILE")
    [ "$current_state" == "blocked" ]
}

# ==============================================================================
# State Tracking Lifecycle
# ==============================================================================

@test "qa_integration-testing_test_e2e_state_tracking_lifecycle: parallel execution clears parallel_execution" {
    use_fixture_workflow "v5_workflow_all.yaml"
    create_default_claude_stub

    run_e2e "test-plan" "test-phase" "parallel_step"
    [ "$status" -eq 0 ]

    # After run_iteration, last_verdict should be set
    local last_verdict
    last_verdict=$(jq -r '.last_verdict // empty' "$STATE_FILE")
    [ "$last_verdict" == "all_complete" ]

    # parallel_execution should be null/absent (cleared by parallel-clear)
    local pe
    pe=$(jq -r '.parallel_execution // "absent"' "$STATE_FILE")
    [ "$pe" == "absent" ] || [ "$pe" == "null" ]
}

# ==============================================================================
# Verdict-Driven Transitions
# ==============================================================================

@test "qa_integration-testing_test_e2e_verdict_drives_transition: all_complete maps to done" {
    use_fixture_workflow "v5_workflow_all.yaml"
    create_default_claude_stub

    run_e2e "test-plan" "test-phase" "parallel_step"
    [ "$status" -eq 0 ]

    local current_state
    current_state=$(jq -r '.current_state' "$STATE_FILE")
    [ "$current_state" == "done" ]
}

@test "qa_integration-testing_test_e2e_verdict_drives_failure_transition: any_failed maps to blocked" {
    use_fixture_workflow "v5_workflow_all.yaml"
    create_controlled_claude_stub

    echo "0" > "$CONTROL_DIR/branch_a.exit_code"
    echo "1" > "$CONTROL_DIR/branch_b.exit_code"

    run_e2e "test-plan" "test-phase" "parallel_step"
    [ "$status" -eq 0 ]

    local current_state
    current_state=$(jq -r '.current_state' "$STATE_FILE")
    [ "$current_state" == "blocked" ]
}

# ==============================================================================
# Timeout Tests
# ==============================================================================

@test "qa_integration-testing_test_e2e_branch_timeout: slow branch gets killed with exit 124" {
    use_fixture_workflow "v5_workflow_timeout.yaml"
    create_slow_claude_stub

    # This test uses a 2-second timeout; the stub sleeps forever
    run_e2e "test-plan" "test-phase" "parallel_step"
    [ "$status" -eq 0 ]

    local results_dir="$PHASE_DIR/parallel_parallel_step"
    [ -d "$results_dir" ]
    [ -f "$results_dir/branch_slow.exit" ]

    # timeout(1) returns 124 when it kills a child
    local exit_code
    exit_code=$(cat "$results_dir/branch_slow.exit")
    [ "$exit_code" -eq 124 ]

    # Verdict should be any_failed (124 is non-zero)
    local last_verdict
    last_verdict=$(jq -r '.last_verdict // empty' "$STATE_FILE")
    [ "$last_verdict" == "any_failed" ]

    # State should transition to blocked
    local current_state
    current_state=$(jq -r '.current_state' "$STATE_FILE")
    [ "$current_state" == "blocked" ]
}

# ==============================================================================
# Mixed Workflow (Linear → Parallel → Linear → Done)
# ==============================================================================

@test "qa_integration-testing_test_e2e_mixed_workflow: linear-parallel-linear reaches done" {
    use_fixture_workflow "v5_workflow_mixed.yaml"
    create_default_claude_stub
    set_state_string "current_state" "setup_state"

    run_e2e_loop "test-plan" "test-phase"
    [ "$status" -eq 0 ]

    # Should have reached terminal state "done"
    local current_state
    current_state=$(jq -r '.current_state' "$STATE_FILE")
    [ "$current_state" == "done" ]

    # Three iterations: setup_state, parallel_step, finalize
    [[ "$output" == *"ITERATIONS_COMPLETED=3"* ]]
}

# ==============================================================================
# Backward Compatibility
# ==============================================================================

@test "qa_integration-testing_test_e2e_backward_compat_v4: V4 workflow runs without parallel side-effects" {
    # Use the real feature.yaml (V2 workflow)
    cp "$WORKFLOWS_DIR/feature.yaml" "$WORKFLOW_FILE"

    # Read the entry state from the workflow
    local entry_state
    entry_state=$(yq '.entry_state' "$WORKFLOW_FILE")
    set_state_string "current_state" "$entry_state"

    run_e2e "test-plan" "test-phase" "$entry_state"
    [ "$status" -eq 0 ]

    # No parallel-related state.json keys
    local pe
    pe=$(jq -r '.parallel_execution // "absent"' "$STATE_FILE")
    [ "$pe" == "absent" ]
}

# ==============================================================================
# Template Rendering
# ==============================================================================

@test "qa_integration-testing_test_e2e_template_rendering: branch params render into prompt template" {
    # Create inline V5 workflow with branch params
    cat > "$WORKFLOW_FILE" << 'YAML'
name: test-template
version: 5
defaults:
  timeout: 60
states:
  - name: parallel_step
    parallel:
      strategy: all
      branches:
        - name: auth
          prompt: prompts/v5_prompts/branch_template.md
          params:
            module: auth
    verdicts:
      - all_complete
      - any_failed
    next:
      all_complete: done
      any_failed: blocked
  - name: done
    description: Success
  - name: blocked
    description: Failure
entry_state: parallel_step
terminal_states:
  - done
  - blocked
YAML

    create_capturing_claude_stub

    run_e2e "test-plan" "test-phase" "parallel_step"
    [ "$status" -eq 0 ]

    # Verify the captured stdin contains rendered template
    [ -f "$CAPTURE_DIR/auth.stdin" ]
    local rendered
    rendered=$(cat "$CAPTURE_DIR/auth.stdin")
    [[ "$rendered" == *"Module: auth"* ]]
}

# ==============================================================================
# Parallel as Entry State
# ==============================================================================

@test "qa_integration-testing_test_e2e_parallel_entry_state: parallel state works as workflow entry point" {
    use_fixture_workflow "v5_workflow_all.yaml"
    create_default_claude_stub

    # entry_state is already parallel_step (set in state.json via setup)
    run_e2e "test-plan" "test-phase" "parallel_step"
    [ "$status" -eq 0 ]

    local current_state
    current_state=$(jq -r '.current_state' "$STATE_FILE")
    [ "$current_state" == "done" ]
}

# ==============================================================================
# Single Branch Parallel
# ==============================================================================

@test "qa_integration-testing_test_e2e_single_branch_parallel: single branch works correctly" {
    cat > "$WORKFLOW_FILE" << 'YAML'
name: test-single-branch
version: 5
defaults:
  timeout: 60
states:
  - name: parallel_step
    parallel:
      strategy: all
      branches:
        - name: only_branch
          prompt: prompts/v5_prompts/branch_pass.md
    verdicts:
      - all_complete
      - any_failed
    next:
      all_complete: done
      any_failed: blocked
  - name: done
    description: Success
  - name: blocked
    description: Failure
entry_state: parallel_step
terminal_states:
  - done
  - blocked
YAML

    create_default_claude_stub

    run_e2e "test-plan" "test-phase" "parallel_step"
    [ "$status" -eq 0 ]

    local results_dir="$PHASE_DIR/parallel_parallel_step"
    [ -f "$results_dir/only_branch.exit" ]
    [ "$(cat "$results_dir/only_branch.exit")" -eq 0 ]

    local last_verdict
    last_verdict=$(jq -r '.last_verdict // empty' "$STATE_FILE")
    [ "$last_verdict" == "all_complete" ]

    local current_state
    current_state=$(jq -r '.current_state' "$STATE_FILE")
    [ "$current_state" == "done" ]
}

# ==============================================================================
# Results Directory Recreation
# ==============================================================================

@test "qa_integration-testing_test_e2e_results_dir_recreated: stale results cleared before new run" {
    use_fixture_workflow "v5_workflow_all.yaml"
    create_default_claude_stub

    # Create stale results directory with old files
    local results_dir="$PHASE_DIR/parallel_parallel_step"
    mkdir -p "$results_dir"
    echo "stale" > "$results_dir/old_branch.exit"
    echo "stale" > "$results_dir/leftover.log"
    echo "stale" > "$results_dir/stale.pid"

    run_e2e "test-plan" "test-phase" "parallel_step"
    [ "$status" -eq 0 ]

    # Stale files should be gone
    [ ! -f "$results_dir/old_branch.exit" ]
    [ ! -f "$results_dir/leftover.log" ]
    [ ! -f "$results_dir/stale.pid" ]

    # Fresh files from the new run should exist
    [ -f "$results_dir/branch_a.exit" ]
    [ -f "$results_dir/branch_b.exit" ]
}
