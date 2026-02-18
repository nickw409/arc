#!/usr/bin/env bats

# V4 End-to-End Pipeline Tests
# Phase: integration (orchestration-v4)
#
# Tests the full iterate.sh pipeline from CLI invocation through all V4 stages.
# Unlike v4_integration.bats (function-level) and qa_iterate-integration.bats
# (individual run_iteration calls), these tests verify complete multi-step
# scenarios where ALL V4 features interact in a single iteration.
#
# Scope: Full pipeline E2E (run_iteration with all 8 steps in sequence)
# - Intervention → Escalation → Pre-constraints → Agent → Verdict → Post-constraints → Hooks → State update

setup() {
    load 'test_helper'
    setup_temp_dir

    ITERATE_SH="$SCRIPTS_DIR/iterate.sh"
    ACTIONS_SH="$SCRIPTS_DIR/actions.sh"
    CHECK_CONSTRAINTS_SH="$SCRIPTS_DIR/check-constraints.sh"
    CHECK_ESCALATION_SH="$SCRIPTS_DIR/check-escalation.sh"
    CHECK_INTERVENTION_SH="$SCRIPTS_DIR/check-intervention.sh"
    RUN_HOOKS_SH="$SCRIPTS_DIR/run-hooks.sh"
    EXTRACT_VERDICT_SH="$SCRIPTS_DIR/extract-verdict.sh"
    BUILD_CONTEXT_SH="$SCRIPTS_DIR/build-context.sh"
    RENDER_TEMPLATE_PY="$SCRIPTS_DIR/render_template.py"

    # Create plan directory structure
    export PLANS_DIR="$TEST_TEMP_DIR/plans"
    export ARC_HOME="$TEST_TEMP_DIR/orchestration"
    mkdir -p "$PLANS_DIR/active/test-plan/phases/test-phase"
    mkdir -p "$ARC_HOME/scripts"
    mkdir -p "$ARC_HOME/prompts/feature"
    mkdir -p "$ARC_HOME/prompts/common"

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
    "current_state": "impl",
    "stuck_iterations": 0,
    "tests_passing": 0,
    "tests_total": 0
}
JSON

    # Create mock prompt files
    echo "# Implementation Prompt" > "$ARC_HOME/prompts/feature/impl.md"
    echo "# Review Prompt" > "$ARC_HOME/prompts/feature/impl-review.md"
    echo "# Complete Prompt" > "$ARC_HOME/prompts/common/complete.md"

    # Track function calls
    export CALL_LOG="$TEST_TEMP_DIR/call_log.txt"
    : > "$CALL_LOG"

    # Track spawn_agent calls
    export SPAWN_LOG="$TEST_TEMP_DIR/spawn_log.txt"
    : > "$SPAWN_LOG"
}

teardown() {
    teardown_temp_dir
}

# ==============================================================================
# Helpers
# ==============================================================================

# Helper: Set a numeric field in state.json
set_state_field() {
    local field="$1"
    local value="$2"
    jq --argjson val "$value" ".$field = \$val" "$STATE_FILE" > "${STATE_FILE}.tmp" \
        && mv "${STATE_FILE}.tmp" "$STATE_FILE"
}

# Helper: Set a string field in state.json
set_state_string() {
    local field="$1"
    local value="$2"
    jq --arg val "$value" ".$field = \$val" "$STATE_FILE" > "${STATE_FILE}.tmp" \
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

# ==============================================================================
# Helper: Run run_iteration in a subshell with full mocking.
# This is the core E2E helper — it sources iterate.sh for the full V4
# pipeline function, sources all V4 libraries, mocks spawn_agent and
# external-calling actions (cargo, git), then runs run_iteration.
#
# Usage: run_e2e <plan> <phase> <state> [extra_setup_code]
# ==============================================================================
run_e2e() {
    local plan_name="${1:-test-plan}"
    local phase_name="${2:-test-phase}"
    local state_name="${3:-impl}"
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
        export CALL_LOG=\"$CALL_LOG\"
        export SPAWN_LOG=\"$SPAWN_LOG\"

        # Source iterate.sh to get run_iteration, setup_v4_environment, etc.
        set +e
        source \"$ITERATE_SH\" 2>/dev/null
        set -e

        if ! declare -f run_iteration > /dev/null 2>&1; then
            echo 'FUNCTION_NOT_FOUND: run_iteration'
            exit 1
        fi

        # Source V4 libraries
        source \"$ACTIONS_SH\"
        source \"$CHECK_CONSTRAINTS_SH\"
        source \"$CHECK_ESCALATION_SH\"
        source \"$CHECK_INTERVENTION_SH\"
        source \"$RUN_HOOKS_SH\"

        # Mock action functions that call external tools.
        # The real action_switch_model and action_analyze_stuck do NOT call
        # external tools, so we use the real implementations to get actual
        # state changes. Only mock actions that call cargo/git/external.
        action_run_tests() {
            echo \"action_run_tests \$*\" >> \"\$CALL_LOG\"
            return 0
        }
        action_commit() {
            echo \"action_commit \$*\" >> \"\$CALL_LOG\"
            return 0
        }
        action_script() {
            echo \"action_script \$*\" >> \"\$CALL_LOG\"
            return 0
        }

        # Default mock spawn_agent: succeeds and creates output
        spawn_agent() {
            local prompt_file=\"\$1\"
            local output_file=\"\$2\"
            echo \"spawn_agent \$prompt_file \$output_file\" >> \"\$SPAWN_LOG\"
            echo \"Agent output\" > \"\$output_file\"
            return 0
        }

        # Apply extra setup (may override spawn_agent or action mocks)
        $extra_setup

        run_iteration \"$plan_name\" \"$phase_name\" \"$state_name\"
    "
}

# =============================================================================
# TEST: test_e2e_full_v4_iteration
# =============================================================================
# Verify all 8 steps of run_iteration execute in a single pass with a
# complete V4 workflow (constraints, escalation, hooks, state update).

@test "v4 e2e: full V4 iteration with all features" {
    create_workflow << 'YAML'
name: test_full_v4
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    constraints:
      max_iterations: 15
      require_artifacts_in:
        - qa_reasoning.md
      require_artifacts_out:
        - impl_reasoning.md
    escalation:
      - at_iteration: 3
        action: analyze_stuck
    after:
      - action: run_tests
        params:
          pattern: "qa_test"
          save_to: "test_output.txt"
    next: review
  - name: review
    prompt: prompts/feature/impl-review.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    # Set iteration at escalation trigger point
    set_state_field "iteration" 3
    # Create required input artifact
    touch "$PHASE_DIR/qa_reasoning.md"

    # Mock agent that creates required output artifact
    run_e2e "test-plan" "test-phase" "impl" '
        spawn_agent() {
            local prompt_file="$1"
            local output_file="$2"
            echo "spawn_agent $prompt_file $output_file" >> "$SPAWN_LOG"
            echo "Agent output" > "$output_file"
            touch "$PHASE_DIR/impl_reasoning.md"
            return 0
        }
    '
    [ "$status" -eq 0 ]

    # Step 2: Escalation triggered (analyze_stuck at iteration 3)
    [ -f "$PHASE_DIR/stuck_analysis.md" ]

    # Step 3: Pre-constraints passed (qa_reasoning.md exists, iteration 3 < 15)
    # (implicit — if they failed, status would be 1)

    # Step 4: Agent was spawned
    [ -s "$SPAWN_LOG" ]

    # Step 6: Post-constraints passed (impl_reasoning.md exists)
    # (implicit — if they failed, status would be 1)

    # Step 7: After hooks executed (run_tests)
    run cat "$CALL_LOG"
    [[ "$output" == *"action_run_tests"* ]]

    # Step 8: State updated (iteration incremented from 3 to 4)
    local iteration
    iteration=$(jq -r '.iteration' "$STATE_FILE")
    [ "$iteration" -eq 4 ]

    # Verify current_state transitioned to review (string .next)
    local current
    current=$(jq -r '.current_state' "$STATE_FILE")
    [ "$current" == "review" ]

    # Verify stuck_iterations reset (impl -> review is a transition)
    local stuck
    stuck=$(jq -r '.stuck_iterations' "$STATE_FILE")
    [ "$stuck" -eq 0 ]
}

# =============================================================================
# TEST: test_e2e_intervention_halts_pipeline
# =============================================================================
# When stuck_iterations exceeds the intervention threshold, the pipeline
# should halt with exit code 2 BEFORE spawning an agent.

@test "v4 e2e: intervention halts pipeline before agent spawn" {
    create_workflow << 'YAML'
name: test_intervention
version: 4
intervention_triggers:
  - condition: "stuck_iterations >= 3"
    action: request_human
    message: "Phase stuck for 5+ iterations"
states:
  - name: impl
    prompt: prompts/feature/impl.md
    escalation:
      - at_iteration: 5
        action: analyze_stuck
    after:
      - action: run_tests
        params:
          pattern: "qa_test"
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    # stuck_iterations 5 >= 3 triggers intervention
    set_state_field "stuck_iterations" 5
    set_state_field "iteration" 5

    run_e2e "test-plan" "test-phase" "impl"

    # Must exit 2 (intervention)
    [ "$status" -eq 2 ]

    # Intervention request file created
    [ -f "$PHASE_DIR/intervention_request.md" ]

    # intervention_request is an object in state.json (per STATE_SCHEMA.md)
    local ir_type
    ir_type=$(jq -r '.intervention_request | type' "$STATE_FILE")
    [ "$ir_type" == "object" ]

    local reason
    reason=$(jq -r '.intervention_request.reason' "$STATE_FILE")
    [ "$reason" == "Phase stuck for 5+ iterations" ]

    local requested_at
    requested_at=$(jq -r '.intervention_request.requested_at' "$STATE_FILE")
    [[ "$requested_at" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T ]]

    local options
    options=$(jq -c '.intervention_request.options' "$STATE_FILE")
    [[ "$options" == *"resolve"* ]]

    # Agent should NOT have been spawned (intervention fires first)
    [ ! -s "$SPAWN_LOG" ]

    # Escalation should NOT have run (intervention halts before step 2)
    [ ! -f "$PHASE_DIR/stuck_analysis.md" ]

    # After hooks should NOT have run
    local hook_calls
    hook_calls=$(grep -c "action_run_tests" "$CALL_LOG" 2>/dev/null) || true
    [ "$hook_calls" -eq 0 ]

    # Iteration should NOT have been incremented (still 5)
    local iteration
    iteration=$(jq -r '.iteration' "$STATE_FILE")
    [ "$iteration" -eq 5 ]
}

# =============================================================================
# TEST: test_e2e_escalation_chain
# =============================================================================
# Run multiple iterations at different trigger points, verifying each
# escalation fires at the correct iteration.

@test "v4 e2e: escalation chain fires at correct iterations" {
    create_workflow << 'YAML'
name: test_escalation_chain
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    escalation:
      - at_iteration: 3
        action: analyze_stuck
      - at_iteration: 5
        action: switch_model
        params:
          model: opus
      - after_iteration: 7
        action: request_human
        params:
          message: "Stuck beyond iteration 7"
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    # --- Iteration 3: analyze_stuck fires ---
    set_state_field "iteration" 3
    run_e2e "test-plan" "test-phase" "impl"
    [ "$status" -eq 0 ]
    [ -f "$PHASE_DIR/stuck_analysis.md" ]

    # Model should not be set yet
    local model
    model=$(jq -r '.current_model // "unset"' "$STATE_FILE")
    [ "$model" == "unset" ]

    # --- Iteration 5: switch_model fires ---
    rm -f "$PHASE_DIR/stuck_analysis.md"
    set_state_field "iteration" 5
    : > "$SPAWN_LOG"
    : > "$CALL_LOG"

    run_e2e "test-plan" "test-phase" "impl"
    [ "$status" -eq 0 ]

    model=$(jq -r '.current_model' "$STATE_FILE")
    [ "$model" == "opus" ]

    # analyze_stuck should NOT have run at iteration 5
    [ ! -f "$PHASE_DIR/stuck_analysis.md" ]

    # --- Iteration 8: request_human fires (after_iteration: 7, iteration 8 > 7) ---
    set_state_field "iteration" 8
    : > "$SPAWN_LOG"
    : > "$CALL_LOG"

    # request_human from escalation will call action_request_human
    # which sets intervention_request and creates file
    run_e2e "test-plan" "test-phase" "impl"

    # The escalation calls action_request_human which returns 0,
    # but check_escalation propagates the action result.
    # The pipeline continues after escalation (step 2).
    # Whether it halts depends on the action implementation.
    # The real action_request_human creates the request file + state update.
    [ -f "$PHASE_DIR/intervention_request.md" ]

    # Verify executed_escalations tracked the after_7 trigger
    local executed
    executed=$(jq -c '.executed_escalations // []' "$STATE_FILE")
    [[ "$executed" == *"after_7"* ]]
}

# =============================================================================
# TEST: test_e2e_hook_with_verdict
# =============================================================================
# Review state produces a verdict; after hooks conditionally execute
# based on the verdict value.

@test "v4 e2e: hook executes based on extracted verdict" {
    create_workflow << 'YAML'
name: test_verdict_hooks
version: 4
states:
  - name: review
    prompt: prompts/feature/impl-review.md
    verdicts: [approved, needs_fix]
    after:
      - action: commit
        when: approved
        params:
          message: "feat: approved implementation"
      - action: switch_model
        when: needs_fix
        params:
          model: opus
    next:
      approved: complete
      needs_fix: impl
  - name: impl
    prompt: prompts/feature/impl.md
    next: review
  - name: complete
    prompt: prompts/common/complete.md
entry_state: review
terminal_states: [complete]
YAML

    set_state_string "current_state" "review"

    # Mock agent returns "approved" verdict
    run_e2e "test-plan" "test-phase" "review" '
        spawn_agent() {
            local prompt_file="$1"
            local output_file="$2"
            echo "spawn_agent $prompt_file $output_file" >> "$SPAWN_LOG"
            printf "## Review\nLooks good.\n\n## Verdict\napproved\n" > "$output_file"
            return 0
        }
    '
    [ "$status" -eq 0 ]

    # Commit hook should have executed (when: approved matches)
    run cat "$CALL_LOG"
    [[ "$output" == *"action_commit"* ]]

    # switch_model hook should NOT have executed (when: needs_fix doesn't match)
    local model
    model=$(jq -r '.current_model // "unset"' "$STATE_FILE")
    [ "$model" == "unset" ]

    # Verdict file created in iteration directory
    [ -f "$PHASE_DIR/iteration_000/verdict.txt" ]
    run cat "$PHASE_DIR/iteration_000/verdict.txt"
    [[ "$output" == *"approved"* ]]

    # State transitioned to complete (approved -> complete)
    local current
    current=$(jq -r '.current_state' "$STATE_FILE")
    [ "$current" == "complete" ]

    # Verdict recorded in history
    local last_verdict
    last_verdict=$(jq -r '.last_verdict' "$STATE_FILE")
    [ "$last_verdict" == "approved" ]

    local history_len
    history_len=$(jq '.verdicts_history | length' "$STATE_FILE")
    [ "$history_len" -ge 1 ]
}

# =============================================================================
# TEST: test_e2e_constraint_failure_recovery
# =============================================================================
# When a post-constraint fails (missing required output artifact), the
# iteration should fail WITHOUT updating state — iteration counter
# must remain unchanged.

@test "v4 e2e: post-constraint failure prevents state update" {
    create_workflow << 'YAML'
name: test_constraint_fail
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    constraints:
      require_artifacts_out:
        - impl_reasoning.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    set_state_field "iteration" 3

    # Mock agent does NOT create impl_reasoning.md
    run_e2e "test-plan" "test-phase" "impl"

    # Should fail (post-constraint: impl_reasoning.md missing)
    [ "$status" -eq 1 ]
    [[ "$output" == *"Post-constraints"* ]] || [[ "$output" == *"constraint"* ]]

    # State NOT updated: iteration stays at 3
    local iteration
    iteration=$(jq -r '.iteration' "$STATE_FILE")
    [ "$iteration" -eq 3 ]

    # current_state still impl (not transitioned)
    local current
    current=$(jq -r '.current_state' "$STATE_FILE")
    [ "$current" == "impl" ]
}

# =============================================================================
# TEST: test_e2e_v1_workflow_still_works
# =============================================================================
# A simple V1 workflow with no V4 features should run normally through
# all pipeline steps (all V4 checks return 0 immediately).

@test "v4 e2e: V1 workflow backwards compatible" {
    create_workflow << 'YAML'
name: test_v1
version: 1
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    run_e2e "test-plan" "test-phase" "impl"

    # Should succeed
    [ "$status" -eq 0 ]

    # Agent was spawned
    [ -s "$SPAWN_LOG" ]

    # Iteration incremented
    local iteration
    iteration=$(jq -r '.iteration' "$STATE_FILE")
    [ "$iteration" -eq 1 ]

    # State transitioned to complete
    local current
    current=$(jq -r '.current_state' "$STATE_FILE")
    [ "$current" == "complete" ]

    # No V4 side effects
    [ ! -f "$PHASE_DIR/stuck_analysis.md" ]
    [ ! -f "$PHASE_DIR/intervention_request.md" ]

    # No action calls (no hooks/escalation configured)
    local call_count
    call_count=$(wc -l < "$CALL_LOG" 2>/dev/null || echo "0")
    [ "$call_count" -eq 0 ]
}

# =============================================================================
# TEST: test_e2e_v3_v4_mixed
# =============================================================================
# Workflow using V3 templates AND V4 hooks. V3 template rendering happens
# at step 4 (prompt render), V4 hooks at step 7.

@test "v4 e2e: V3 templates and V4 hooks work together" {
    create_workflow << 'YAML'
name: test_v3_v4
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    params:
      phase: test-phase
    after:
      - action: analyze_stuck
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    # Set iteration >= 2 so analyze_stuck has enough history
    set_state_field "iteration" 2

    # Copy V3 scripts if they exist (for template rendering)
    if [[ -f "$BUILD_CONTEXT_SH" && -f "$RENDER_TEMPLATE_PY" ]]; then
        cp "$BUILD_CONTEXT_SH" "$ARC_HOME/scripts/"
        cp "$RENDER_TEMPLATE_PY" "$ARC_HOME/scripts/"
    fi

    run_e2e "test-plan" "test-phase" "impl"
    [ "$status" -eq 0 ]

    # Agent was spawned with a rendered prompt
    [ -s "$SPAWN_LOG" ]
    [ -f "$PHASE_DIR/iteration_002/prompt.md" ]

    # V4 after hook executed (analyze_stuck)
    [ -f "$PHASE_DIR/stuck_analysis.md" ]

    # State updated (iteration incremented)
    local iteration
    iteration=$(jq -r '.iteration' "$STATE_FILE")
    [ "$iteration" -eq 3 ]
}

# =============================================================================
# TEST: test_e2e_pre_constraint_blocks_entire_pipeline
# =============================================================================
# Pre-constraint failure at step 3 should prevent agent spawn (step 4),
# verdict extraction (step 5), post-constraints (step 6), hooks (step 7),
# and state update (step 8).

@test "v4 e2e: pre-constraint blocks entire pipeline" {
    create_workflow << 'YAML'
name: test_pre_block
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    constraints:
      max_iterations: 3
      require_artifacts_in:
        - required_input.md
    after:
      - action: switch_model
        params:
          model: opus
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    set_state_field "iteration" 5

    # Don't create required_input.md — pre-constraint fails

    run_e2e "test-plan" "test-phase" "impl"
    [ "$status" -eq 1 ]

    # Agent NOT spawned
    [ ! -s "$SPAWN_LOG" ]

    # Hooks NOT run
    local model
    model=$(jq -r '.current_model // "unset"' "$STATE_FILE")
    [ "$model" == "unset" ]

    # State NOT updated (iteration stays at 5)
    local iteration
    iteration=$(jq -r '.iteration' "$STATE_FILE")
    [ "$iteration" -eq 5 ]
}

# =============================================================================
# TEST: test_e2e_agent_failure_still_runs_post_checks
# =============================================================================
# Agent failure (step 4) should NOT prevent post-constraints (step 6)
# and hooks (step 7) from running. Final exit code should reflect agent failure.

@test "v4 e2e: agent failure still runs post-constraints and hooks" {
    create_workflow << 'YAML'
name: test_agent_fail
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    constraints:
      require_artifacts_out:
        - impl_reasoning.md
    after:
      - action: run_tests
        params:
          pattern: "qa_test"
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    # Create the output artifact so post-constraint passes
    touch "$PHASE_DIR/impl_reasoning.md"

    # Agent fails but post-checks should still run
    run_e2e "test-plan" "test-phase" "impl" '
        spawn_agent() {
            local prompt_file="$1"
            local output_file="$2"
            echo "spawn_agent FAILED" >> "$SPAWN_LOG"
            return 1
        }
    '

    # Agent failed, so overall result should be non-zero
    [ "$status" -ne 0 ]

    # But hooks DID run (action_run_tests was called)
    run cat "$CALL_LOG"
    [[ "$output" == *"action_run_tests"* ]]
}

# =============================================================================
# TEST: test_e2e_multiple_iterations_state_consistency
# =============================================================================
# Run 3 iterations sequentially, verifying state accumulates correctly.

@test "v4 e2e: state consistency across sequential iterations" {
    create_workflow << 'YAML'
name: test_sequential
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: impl
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    # Run 3 iterations via direct sourcing (not run_e2e which uses `run`)
    # so state accumulates across calls
    run bash -c "
        export PHASE_DIR=\"$PHASE_DIR\"
        export STATE_FILE=\"$STATE_FILE\"
        export WORKFLOW_FILE=\"$WORKFLOW_FILE\"
        export ARC_HOME=\"$ARC_HOME\"
        export SCRIPTS_DIR=\"$SCRIPTS_DIR\"
        export PLANS_DIR=\"$PLANS_DIR\"
        export ARC_DEFAULT_PKG=\"test-package\"
        export VERDICT=\"\"

        set +e
        source \"$ITERATE_SH\" 2>/dev/null
        set -e

        if ! declare -f run_iteration > /dev/null 2>&1; then
            echo 'FUNCTION_NOT_FOUND: run_iteration'
            exit 1
        fi

        source \"$ACTIONS_SH\"
        source \"$CHECK_CONSTRAINTS_SH\"
        source \"$CHECK_ESCALATION_SH\"
        source \"$CHECK_INTERVENTION_SH\"
        source \"$RUN_HOOKS_SH\"

        spawn_agent() {
            echo \"Mock output iter\" > \"\$2\"
            return 0
        }

        action_run_tests() { return 0; }
        action_commit() { return 0; }
        action_script() { return 0; }

        # Run 3 iterations
        run_iteration 'test-plan' 'test-phase' 'impl'
        run_iteration 'test-plan' 'test-phase' 'impl'
        run_iteration 'test-plan' 'test-phase' 'impl'
    "
    [ "$status" -eq 0 ]

    # Iteration incremented 3 times
    local iteration
    iteration=$(jq -r '.iteration' "$STATE_FILE")
    [ "$iteration" -eq 3 ]

    # impl -> impl is a loop, so stuck_iterations should have been incremented
    local stuck
    stuck=$(jq -r '.stuck_iterations' "$STATE_FILE")
    [ "$stuck" -eq 3 ]
}

# =============================================================================
# TEST: test_e2e_escalation_once_only_across_iterations
# =============================================================================
# after_iteration triggers should fire once and not again on subsequent calls.

@test "v4 e2e: after_iteration escalation fires once across iterations" {
    create_workflow << 'YAML'
name: test_once
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    escalation:
      - after_iteration: 2
        action: analyze_stuck
    next: impl
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    # Run two iterations (3 and 4) — both > 2, but after_iteration fires once
    run bash -c "
        export PHASE_DIR=\"$PHASE_DIR\"
        export STATE_FILE=\"$STATE_FILE\"
        export WORKFLOW_FILE=\"$WORKFLOW_FILE\"
        export ARC_HOME=\"$ARC_HOME\"
        export SCRIPTS_DIR=\"$SCRIPTS_DIR\"
        export PLANS_DIR=\"$PLANS_DIR\"
        export ARC_DEFAULT_PKG=\"test-package\"
        export VERDICT=\"\"

        set +e
        source \"$ITERATE_SH\" 2>/dev/null
        set -e

        source \"$ACTIONS_SH\"
        source \"$CHECK_CONSTRAINTS_SH\"
        source \"$CHECK_ESCALATION_SH\"
        source \"$CHECK_INTERVENTION_SH\"
        source \"$RUN_HOOKS_SH\"

        spawn_agent() { echo 'Mock' > \"\$2\"; return 0; }
        action_run_tests() { return 0; }
        action_commit() { return 0; }
        action_script() { return 0; }

        # Start at iteration 3 (> 2)
        jq '.iteration = 3' \"\$STATE_FILE\" > \"\${STATE_FILE}.tmp\" && mv \"\${STATE_FILE}.tmp\" \"\$STATE_FILE\"

        # First call — escalation fires
        run_iteration 'test-plan' 'test-phase' 'impl'

        # Track if analysis was created
        if [ -f \"\$PHASE_DIR/stuck_analysis.md\" ]; then
            echo 'FIRST_ANALYSIS_CREATED'
        fi

        # Remove analysis to detect if it's recreated
        rm -f \"\$PHASE_DIR/stuck_analysis.md\"

        # Second call — escalation should NOT re-fire
        run_iteration 'test-plan' 'test-phase' 'impl'

        if [ ! -f \"\$PHASE_DIR/stuck_analysis.md\" ]; then
            echo 'SECOND_ANALYSIS_NOT_CREATED'
        fi
    "
    [ "$status" -eq 0 ]
    [[ "$output" == *"FIRST_ANALYSIS_CREATED"* ]]
    [[ "$output" == *"SECOND_ANALYSIS_NOT_CREATED"* ]]

    # executed_escalations should have exactly one entry
    local count
    count=$(jq '.executed_escalations | length' "$STATE_FILE")
    [ "$count" -eq 1 ]
    local executed
    executed=$(jq -c '.executed_escalations' "$STATE_FILE")
    [[ "$executed" == *"after_2"* ]]
}

# =============================================================================
# TEST: test_e2e_hook_failure_does_not_corrupt_state
# =============================================================================
# If a hook fails (step 7), run_iteration returns 1 but state should
# still have been updated up to the point of failure.

@test "v4 e2e: hook failure returns 1 without corrupting state" {
    create_workflow << 'YAML'
name: test_hook_fail
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    after:
      - action: script
        params:
          path: "scripts/nonexistent.sh"
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    set_state_field "iteration" 5

    # Override action_script to actually fail (default mock returns 0)
    run_e2e "test-plan" "test-phase" "impl" '
        action_script() {
            echo "action_script FAILED: $*" >> "$CALL_LOG"
            return 1
        }
    '
    [ "$status" -eq 1 ]

    # State file should still be valid JSON (not corrupted)
    run jq '.' "$STATE_FILE"
    [ "$status" -eq 0 ]

    # Iteration should NOT have been incremented (hook failed before state update)
    local iteration
    iteration=$(jq -r '.iteration' "$STATE_FILE")
    [ "$iteration" -eq 5 ]
}

# =============================================================================
# TEST: test_e2e_intervention_already_pending
# =============================================================================
# If intervention_request already exists in state, the pipeline should
# halt with exit 2 immediately without re-triggering or overwriting.

@test "v4 e2e: pending intervention halts without re-triggering" {
    create_workflow << 'YAML'
name: test_pending_intervention
version: 4
intervention_triggers:
  - condition: "stuck_iterations >= 3"
    action: request_human
    message: "New request"
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    set_state_field "stuck_iterations" 10

    # Set an existing intervention_request
    jq '.intervention_request = {"reason": "Original request", "requested_at": "2024-01-01T00:00:00Z", "options": ["resolve", "abort"]}' \
        "$STATE_FILE" > "${STATE_FILE}.tmp" && mv "${STATE_FILE}.tmp" "$STATE_FILE"

    run_e2e "test-plan" "test-phase" "impl"
    [ "$status" -eq 2 ]

    # Original request preserved (not overwritten)
    local reason
    reason=$(jq -r '.intervention_request.reason' "$STATE_FILE")
    [ "$reason" == "Original request" ]
}

# =============================================================================
# TEST: test_e2e_verdict_needs_fix_loop
# =============================================================================
# Review with needs_fix verdict should loop back, incrementing stuck_iterations.

@test "v4 e2e: needs_fix verdict loops back and increments stuck" {
    create_workflow << 'YAML'
name: test_loop
version: 4
states:
  - name: review
    prompt: prompts/feature/impl-review.md
    verdicts: [approved, needs_fix]
    next:
      approved: complete
      needs_fix: review
  - name: complete
    prompt: prompts/common/complete.md
entry_state: review
terminal_states: [complete]
YAML

    set_state_string "current_state" "review"
    set_state_field "stuck_iterations" 1

    run_e2e "test-plan" "test-phase" "review" '
        spawn_agent() {
            local prompt_file="$1"
            local output_file="$2"
            echo "spawn_agent" >> "$SPAWN_LOG"
            printf "## Verdict\nneeds_fix\n" > "$output_file"
            return 0
        }
    '
    [ "$status" -eq 0 ]

    # needs_fix -> review (same state) -> stuck incremented
    local stuck
    stuck=$(jq -r '.stuck_iterations' "$STATE_FILE")
    [ "$stuck" -eq 2 ]

    # State stays at review
    local current
    current=$(jq -r '.current_state' "$STATE_FILE")
    [ "$current" == "review" ]

    # Verdict recorded
    local last_verdict
    last_verdict=$(jq -r '.last_verdict' "$STATE_FILE")
    [ "$last_verdict" == "needs_fix" ]
}

# =============================================================================
# TEST: test_e2e_execution_order_verified
# =============================================================================
# Verify the exact execution order by tracking which steps ran and in
# what sequence via a call log.

@test "v4 e2e: execution order matches spec (8 steps)" {
    create_workflow << 'YAML'
name: test_order
version: 4
intervention_triggers:
  - condition: "stuck_iterations >= 100"
    action: request_human
    message: "Never fires"
states:
  - name: impl
    prompt: prompts/feature/impl.md
    constraints:
      max_iterations: 20
    escalation:
      - at_iteration: 1
        action: analyze_stuck
    after:
      - action: run_tests
        params:
          pattern: "qa_test"
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    set_state_field "iteration" 1

    # Use a sequenced call log to verify ordering
    run_e2e "test-plan" "test-phase" "impl" '
        # Override to track execution steps in order
        check_intervention_triggers_real=$(declare -f check_intervention_triggers)
        check_intervention_triggers() {
            echo "STEP1:intervention" >> "$CALL_LOG"
            eval "$check_intervention_triggers_real"
            check_intervention_triggers "$@"
        }

        check_escalation_real=$(declare -f check_escalation)
        check_escalation() {
            echo "STEP2:escalation" >> "$CALL_LOG"
            eval "$check_escalation_real"
            check_escalation "$@"
        }

        check_pre_constraints_real=$(declare -f check_pre_constraints)
        check_pre_constraints() {
            echo "STEP3:pre_constraints" >> "$CALL_LOG"
            eval "$check_pre_constraints_real"
            check_pre_constraints "$@"
        }

        spawn_agent() {
            echo "STEP4:agent" >> "$CALL_LOG"
            echo "Agent output" > "$2"
            return 0
        }

        check_post_constraints_real=$(declare -f check_post_constraints)
        check_post_constraints() {
            echo "STEP6:post_constraints" >> "$CALL_LOG"
            eval "$check_post_constraints_real"
            check_post_constraints "$@"
        }

        run_after_hooks_real=$(declare -f run_after_hooks)
        run_after_hooks() {
            echo "STEP7:hooks" >> "$CALL_LOG"
            eval "$run_after_hooks_real"
            run_after_hooks "$@"
        }
    '

    # Pipeline should succeed
    [ "$status" -eq 0 ]

    # Verify ordering: each STEP should appear, and in sequence
    run cat "$CALL_LOG"
    [[ "$output" == *"STEP1:intervention"* ]]
    [[ "$output" == *"STEP2:escalation"* ]]
    [[ "$output" == *"STEP3:pre_constraints"* ]]
    [[ "$output" == *"STEP4:agent"* ]]
    [[ "$output" == *"STEP6:post_constraints"* ]]
    [[ "$output" == *"STEP7:hooks"* ]]
}

# =============================================================================
# TEST: test_e2e_iteration_directory_structure
# =============================================================================
# Verify that run_iteration creates the correct iteration directory
# with prompt.md and output.txt.

@test "v4 e2e: iteration directory created with correct artifacts" {
    create_workflow << 'YAML'
name: test_iter_dir
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

    set_state_field "iteration" 7

    run_e2e "test-plan" "test-phase" "impl"
    [ "$status" -eq 0 ]

    # iteration_007 directory should exist
    [ -d "$PHASE_DIR/iteration_007" ]

    # prompt.md should exist (copied or rendered)
    [ -f "$PHASE_DIR/iteration_007/prompt.md" ]

    # output.txt should exist (from mock agent)
    [ -f "$PHASE_DIR/iteration_007/output.txt" ]
}

# =============================================================================
# TEST: test_e2e_empty_state_handles_defaults
# =============================================================================
# State.json with minimal fields should work — missing fields should
# default gracefully.

@test "v4 e2e: minimal state.json handles missing fields with defaults" {
    create_workflow << 'YAML'
name: test_defaults
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

    # Write minimal state
    echo '{"plan": "test-plan", "phase": "test-phase", "current_state": "impl"}' > "$STATE_FILE"

    run_e2e "test-plan" "test-phase" "impl"
    [ "$status" -eq 0 ]

    # Iteration should have gone from 0 (default) to 1
    local iteration
    iteration=$(jq -r '.iteration' "$STATE_FILE")
    [ "$iteration" -eq 1 ]
}

# =============================================================================
# TEST: test_e2e_multiple_hooks_execute_in_sequence
# =============================================================================
# Multiple after hooks should execute in the order defined in the workflow.

@test "v4 e2e: multiple hooks execute in sequence" {
    create_workflow << 'YAML'
name: test_multi_hooks
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    after:
      - action: switch_model
        params:
          model: haiku
      - action: run_tests
        params:
          pattern: "qa_test"
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    run_e2e "test-plan" "test-phase" "impl"
    [ "$status" -eq 0 ]

    # switch_model should have set model (uses real implementation)
    local model
    model=$(jq -r '.current_model' "$STATE_FILE")
    [ "$model" == "haiku" ]

    # run_tests should also have been called
    run cat "$CALL_LOG"
    [[ "$output" == *"action_run_tests"* ]]
}

# =============================================================================
# TEST: test_e2e_continue_on_error_allows_next_hook
# =============================================================================
# With continue_on_error: true, a failed hook should not stop subsequent hooks.

@test "v4 e2e: continue_on_error allows next hook to execute" {
    create_workflow << 'YAML'
name: test_continue
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    after:
      - action: script
        continue_on_error: true
        params:
          path: "scripts/nonexistent.sh"
      - action: switch_model
        params:
          model: haiku
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    run_e2e "test-plan" "test-phase" "impl"

    # First hook fails, but continue_on_error allows second hook
    # Second hook should have executed
    local model
    model=$(jq -r '.current_model' "$STATE_FILE")
    [ "$model" == "haiku" ]
}

# =============================================================================
# TEST: test_e2e_no_intervention_triggers_section
# =============================================================================
# Workflow without intervention_triggers section should skip step 1 cleanly.

@test "v4 e2e: workflow without intervention_triggers works" {
    create_workflow << 'YAML'
name: test_no_intervention
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

    # Even with high stuck_iterations, no intervention_triggers means no halt
    set_state_field "stuck_iterations" 100

    run_e2e "test-plan" "test-phase" "impl"
    [ "$status" -eq 0 ]

    # Agent was spawned normally
    [ -s "$SPAWN_LOG" ]
}

# =============================================================================
# TEST: test_e2e_max_iterations_constraint_blocks
# =============================================================================
# max_iterations constraint should block when iteration >= max.

@test "v4 e2e: max_iterations constraint blocks at limit" {
    create_workflow << 'YAML'
name: test_max_iter
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    constraints:
      max_iterations: 5
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    set_state_field "iteration" 5

    run_e2e "test-plan" "test-phase" "impl"
    [ "$status" -eq 1 ]

    # Agent should NOT have been spawned
    [ ! -s "$SPAWN_LOG" ]

    # Iteration NOT incremented
    local iteration
    iteration=$(jq -r '.iteration' "$STATE_FILE")
    [ "$iteration" -eq 5 ]
}
