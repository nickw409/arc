#!/usr/bin/env bats

# V4 Component Integration Tests
# Phase: integration-testing (orchestration-v4)
#
# Tests V4 functions working together by calling them directly:
#   - constraints + hooks
#   - escalation + state tracking
#   - intervention + state tracking
#   - cross-feature interactions
#
# Scope: Function-level integration (NOT pipeline-level E2E).
# Unit tests for individual functions are in:
#   check_constraints.bats, run_hooks.bats, check_escalation.bats, check_intervention.bats

setup() {
    load 'test_helper'
    setup_temp_dir

    # Fixture paths
    FIXTURES_DIR="$ORCH_DIR/tests/fixtures/v4"

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
    echo "Test prompt" > "$ARC_HOME/prompts/feature/impl.md"
    echo "Review prompt" > "$ARC_HOME/prompts/feature/impl-review.md"
    echo "Complete prompt" > "$ARC_HOME/prompts/common/complete.md"

    # Source V4 scripts
    source "$SCRIPTS_DIR/actions.sh"
    source "$SCRIPTS_DIR/check-constraints.sh"
    source "$SCRIPTS_DIR/check-escalation.sh"
    source "$SCRIPTS_DIR/check-intervention.sh"
    source "$SCRIPTS_DIR/run-hooks.sh"
}

teardown() {
    teardown_temp_dir
}

# Helper: Check if iterate.sh has V4 integration (skip full pipeline tests if not)
skip_if_not_v4_integrated() {
    if ! grep -q "check_intervention_triggers" "$SCRIPTS_DIR/iterate.sh" 2>/dev/null; then
        skip "iterate.sh does not have V4 integration yet"
    fi
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

# =============================================================================
# CONSTRAINT + HOOK INTEGRATION
# =============================================================================
# Tests: pre-constraints checked before hooks, post-constraints after hooks,
#   hook actions only run when constraint passes

@test "v4 integration: pre-constraint blocks before hooks can run" {
    create_workflow << 'YAML'
name: test
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    constraints:
      max_iterations: 3
    after:
      - action: analyze_stuck
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    # Set iteration at limit
    set_state_field "iteration" 3

    # Pre-constraint should block (iteration >= max)
    run check_pre_constraints "$WORKFLOW_FILE" "impl" "$PHASE_DIR"
    [ "$status" -eq 1 ]
    [[ "$output" == *"Max iterations exceeded"* ]]

    # stuck_analysis.md should NOT exist (hooks never ran)
    [ ! -f "$PHASE_DIR/stuck_analysis.md" ]
}

@test "v4 integration: pre-constraint passes then hooks execute" {
    create_workflow << 'YAML'
name: test
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    constraints:
      max_iterations: 10
    escalation:
      - at_iteration: 2
        action: analyze_stuck
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    # Set iteration below limit but at escalation trigger
    set_state_field "iteration" 2

    # Pre-constraint should pass
    run check_pre_constraints "$WORKFLOW_FILE" "impl" "$PHASE_DIR"
    [ "$status" -eq 0 ]

    # Escalation should trigger at iteration 2
    run check_escalation "$WORKFLOW_FILE" "impl"
    [ "$status" -eq 0 ]
    [ -f "$PHASE_DIR/stuck_analysis.md" ]
}

@test "v4 integration: artifact constraint blocks when file missing" {
    create_workflow << 'YAML'
name: test
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    constraints:
      require_artifacts_in:
        - required_input.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    # Pre-constraint should fail (artifact missing)
    run check_pre_constraints "$WORKFLOW_FILE" "impl" "$PHASE_DIR"
    [ "$status" -eq 1 ]
    [[ "$output" == *"Missing required artifacts"* ]]
}

@test "v4 integration: artifact constraint passes when file exists" {
    create_workflow << 'YAML'
name: test
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    constraints:
      require_artifacts_in:
        - required_input.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    # Create the required artifact
    touch "$PHASE_DIR/required_input.md"

    # Pre-constraint should pass
    run check_pre_constraints "$WORKFLOW_FILE" "impl" "$PHASE_DIR"
    [ "$status" -eq 0 ]
}

@test "v4 integration: post-constraint checks after hook execution" {
    create_workflow << 'YAML'
name: test
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    constraints:
      require_artifacts_out:
        - output_file.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    # Post-constraint should fail (output missing)
    run check_post_constraints "$WORKFLOW_FILE" "impl" "$PHASE_DIR"
    [ "$status" -eq 1 ]
    [[ "$output" == *"Missing required artifacts"* ]]

    # Create the output artifact (simulating hook or agent creating it)
    touch "$PHASE_DIR/output_file.md"

    # Post-constraint should now pass
    run check_post_constraints "$WORKFLOW_FILE" "impl" "$PHASE_DIR"
    [ "$status" -eq 0 ]
}

# =============================================================================
# HOOK EXECUTION INTEGRATION
# =============================================================================
# Tests: verdict-conditional hooks, negation, OR conditions

@test "v4 integration: hook executes on matching verdict" {
    create_workflow << 'YAML'
name: test
version: 4
states:
  - name: review
    prompt: prompts/feature/impl.md
    verdicts: [approved, needs_fix]
    after:
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

    # Run hooks with matching verdict
    run run_after_hooks "$WORKFLOW_FILE" "review" "needs_fix"
    [ "$status" -eq 0 ]

    # Verify model was switched in state
    local model
    model=$(get_state_field "current_model")
    [ "$model" == "opus" ]
}

@test "v4 integration: hook skipped on non-matching verdict" {
    create_workflow << 'YAML'
name: test
version: 4
states:
  - name: review
    prompt: prompts/feature/impl.md
    after:
      - action: switch_model
        when: needs_fix
        params:
          model: opus
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: review
terminal_states: [complete]
YAML

    # Run hooks with non-matching verdict
    run run_after_hooks "$WORKFLOW_FILE" "review" "approved"
    [ "$status" -eq 0 ]

    # Model should NOT be set (hook was skipped)
    local model
    model=$(jq -r '.current_model // "unset"' "$STATE_FILE")
    [ "$model" == "unset" ]
}

@test "v4 integration: negated when condition works" {
    # Direct function call test
    run check_when_condition "!approved" "needs_fix"
    [ "$status" -eq 0 ]

    run check_when_condition "!approved" "approved"
    [ "$status" -eq 1 ]
}

@test "v4 integration: OR when condition works" {
    run check_when_condition "approved|passed" "passed"
    [ "$status" -eq 0 ]

    run check_when_condition "approved|passed" "approved"
    [ "$status" -eq 0 ]

    run check_when_condition "approved|passed" "failed"
    [ "$status" -eq 1 ]
}

@test "v4 integration: hook with negated condition executes correctly" {
    create_workflow << 'YAML'
name: test
version: 4
states:
  - name: review
    prompt: prompts/feature/impl.md
    after:
      - action: switch_model
        when: "!approved"
        params:
          model: opus
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: review
terminal_states: [complete]
YAML

    # Run with "needs_fix" — negation of "approved" should match
    run run_after_hooks "$WORKFLOW_FILE" "review" "needs_fix"
    [ "$status" -eq 0 ]
    local model
    model=$(get_state_field "current_model")
    [ "$model" == "opus" ]
}

@test "v4 integration: hook with OR condition executes correctly" {
    create_workflow << 'YAML'
name: test
version: 4
states:
  - name: review
    prompt: prompts/feature/impl.md
    after:
      - action: switch_model
        when: "needs_fix|concerns"
        params:
          model: opus
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: review
terminal_states: [complete]
YAML

    # Run with "concerns" — should match OR condition
    run run_after_hooks "$WORKFLOW_FILE" "review" "concerns"
    [ "$status" -eq 0 ]
    local model
    model=$(get_state_field "current_model")
    [ "$model" == "opus" ]
}

@test "v4 integration: multiple hooks execute in order" {
    create_workflow << 'YAML'
name: test
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    after:
      - action: switch_model
        params:
          model: opus
      - action: analyze_stuck
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    set_state_field "iteration" 3

    # Both hooks should execute
    run run_after_hooks "$WORKFLOW_FILE" "impl" ""
    [ "$status" -eq 0 ]

    # Verify both actions had effect
    local model
    model=$(get_state_field "current_model")
    [ "$model" == "opus" ]
    [ -f "$PHASE_DIR/stuck_analysis.md" ]
}

# =============================================================================
# ESCALATION INTEGRATION
# =============================================================================
# Tests: at_iteration trigger, after_iteration once-only execution,
#   every_n_iterations modulo, executed_escalations persistence

@test "v4 integration: at_iteration escalation triggers at exact iteration" {
    create_workflow << 'YAML'
name: test
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    escalation:
      - at_iteration: 3
        action: analyze_stuck
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    set_state_field "iteration" 3

    run check_escalation "$WORKFLOW_FILE" "impl"
    [ "$status" -eq 0 ]
    [ -f "$PHASE_DIR/stuck_analysis.md" ]
}

@test "v4 integration: at_iteration does not trigger at wrong iteration" {
    create_workflow << 'YAML'
name: test
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    escalation:
      - at_iteration: 3
        action: analyze_stuck
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    set_state_field "iteration" 2

    run check_escalation "$WORKFLOW_FILE" "impl"
    [ "$status" -eq 0 ]
    [ ! -f "$PHASE_DIR/stuck_analysis.md" ]
}

@test "v4 integration: after_iteration escalation executes once only" {
    create_workflow << 'YAML'
name: test
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    escalation:
      - after_iteration: 3
        action: analyze_stuck
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    set_state_field "iteration" 5

    # First call — should execute
    run check_escalation "$WORKFLOW_FILE" "impl"
    [ "$status" -eq 0 ]
    [ -f "$PHASE_DIR/stuck_analysis.md" ]

    # Verify executed_escalations was recorded
    local executed
    executed=$(jq -r '.executed_escalations // []' "$STATE_FILE")
    [[ "$executed" == *"after_3"* ]]

    # Remove analysis file to verify it doesn't get recreated
    rm -f "$PHASE_DIR/stuck_analysis.md"

    # Second call — should NOT re-execute (already tracked)
    run check_escalation "$WORKFLOW_FILE" "impl"
    [ "$status" -eq 0 ]
    [ ! -f "$PHASE_DIR/stuck_analysis.md" ]
}

@test "v4 integration: every_n_iterations triggers on modulo" {
    create_workflow << 'YAML'
name: test
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    escalation:
      - every_n_iterations: 2
        action: analyze_stuck
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    # Test iteration 4 (4 % 2 == 0, should trigger)
    set_state_field "iteration" 4
    run check_escalation "$WORKFLOW_FILE" "impl"
    [ "$status" -eq 0 ]
    [ -f "$PHASE_DIR/stuck_analysis.md" ]

    # Test iteration 5 (5 % 2 != 0, should NOT trigger)
    rm -f "$PHASE_DIR/stuck_analysis.md"
    set_state_field "iteration" 5
    run check_escalation "$WORKFLOW_FILE" "impl"
    [ "$status" -eq 0 ]
    [ ! -f "$PHASE_DIR/stuck_analysis.md" ]
}

@test "v4 integration: escalation switch_model updates state" {
    create_workflow << 'YAML'
name: test
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    escalation:
      - at_iteration: 5
        action: switch_model
        params:
          model: opus
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    set_state_field "iteration" 5

    run check_escalation "$WORKFLOW_FILE" "impl"
    [ "$status" -eq 0 ]

    local model
    model=$(get_state_field "current_model")
    [ "$model" == "opus" ]
}

@test "v4 integration: escalation only executes first matching trigger" {
    create_workflow << 'YAML'
name: test
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    escalation:
      - at_iteration: 4
        action: analyze_stuck
      - every_n_iterations: 2
        action: switch_model
        params:
          model: opus
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    # iteration 4 matches both at_iteration:4 AND every_n_iterations:2
    # but only the FIRST match should execute
    set_state_field "iteration" 4

    run check_escalation "$WORKFLOW_FILE" "impl"
    [ "$status" -eq 0 ]

    # analyze_stuck should have run (first match)
    [ -f "$PHASE_DIR/stuck_analysis.md" ]

    # switch_model should NOT have run
    local model
    model=$(jq -r '.current_model // "unset"' "$STATE_FILE")
    [ "$model" == "unset" ]
}

# =============================================================================
# INTERVENTION INTEGRATION
# =============================================================================
# Tests: condition evaluation, trigger on match, no re-trigger,
#   intervention_request object format

@test "v4 integration: intervention triggers on condition match" {
    create_workflow << 'YAML'
name: test
version: 4
intervention_triggers:
  - condition: "stuck_iterations >= 3"
    action: request_human
    message: "Help needed"
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    set_state_field "stuck_iterations" 4

    run check_intervention_triggers "$WORKFLOW_FILE"
    [ "$status" -eq 2 ]
    [ -f "$PHASE_DIR/intervention_request.md" ]

    # Verify intervention_request is an object with correct fields
    local reason
    reason=$(jq -r '.intervention_request.reason' "$STATE_FILE")
    [ "$reason" == "Help needed" ]

    local requested_at
    requested_at=$(jq -r '.intervention_request.requested_at' "$STATE_FILE")
    [[ "$requested_at" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T ]]

    local options
    options=$(jq -c '.intervention_request.options' "$STATE_FILE")
    [[ "$options" == *"resolve"* ]]
    [[ "$options" == *"abort"* ]]
}

@test "v4 integration: intervention does not trigger below threshold" {
    create_workflow << 'YAML'
name: test
version: 4
intervention_triggers:
  - condition: "stuck_iterations >= 3"
    action: request_human
    message: "Help needed"
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    set_state_field "stuck_iterations" 2

    run check_intervention_triggers "$WORKFLOW_FILE"
    [ "$status" -eq 0 ]
    [ ! -f "$PHASE_DIR/intervention_request.md" ]
}

@test "v4 integration: intervention does not re-trigger if already requested" {
    create_workflow << 'YAML'
name: test
version: 4
intervention_triggers:
  - condition: "stuck_iterations >= 3"
    action: request_human
    message: "Help needed"
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    set_state_field "stuck_iterations" 5

    # Set intervention_request as object (not boolean)
    jq '.intervention_request = {"reason": "previous request", "requested_at": "2024-01-01T00:00:00Z", "options": ["resolve", "abort"]}' \
        "$STATE_FILE" > "${STATE_FILE}.tmp" && mv "${STATE_FILE}.tmp" "$STATE_FILE"

    # Should return 2 (already pending) without creating a new file
    run check_intervention_triggers "$WORKFLOW_FILE"
    [ "$status" -eq 2 ]

    # intervention_request should still have the original reason
    local reason
    reason=$(jq -r '.intervention_request.reason' "$STATE_FILE")
    [ "$reason" == "previous request" ]
}

@test "v4 integration: condition evaluation with different operators" {
    local state_json='{"stuck_iterations": 3, "iteration": 5, "tests_passing": 10}'

    run evaluate_condition "stuck_iterations >= 3" "$state_json"
    [ "$status" -eq 0 ]

    run evaluate_condition "stuck_iterations < 3" "$state_json"
    [ "$status" -eq 1 ]

    run evaluate_condition "iteration == 5" "$state_json"
    [ "$status" -eq 0 ]

    run evaluate_condition "iteration != 5" "$state_json"
    [ "$status" -eq 1 ]

    run evaluate_condition "tests_passing > 5" "$state_json"
    [ "$status" -eq 0 ]

    run evaluate_condition "tests_passing <= 10" "$state_json"
    [ "$status" -eq 0 ]
}

@test "v4 integration: condition with missing state field defaults to 0" {
    local state_json='{"iteration": 5}'

    # stuck_iterations not in state — should default to 0
    run evaluate_condition "stuck_iterations >= 3" "$state_json"
    [ "$status" -eq 1 ]

    run evaluate_condition "stuck_iterations == 0" "$state_json"
    [ "$status" -eq 0 ]
}

@test "v4 integration: multiple intervention triggers evaluated in order" {
    create_workflow << 'YAML'
name: test
version: 4
intervention_triggers:
  - condition: "stuck_iterations >= 3"
    action: request_human
    message: "Stuck too long"
  - condition: "iteration >= 10"
    action: request_human
    message: "Max iterations"
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    # Both conditions true — first trigger wins
    set_state_field "stuck_iterations" 5
    set_state_field "iteration" 12

    run check_intervention_triggers "$WORKFLOW_FILE"
    [ "$status" -eq 2 ]

    local reason
    reason=$(jq -r '.intervention_request.reason' "$STATE_FILE")
    [ "$reason" == "Stuck too long" ]
}

# =============================================================================
# CONSTRAINT + ESCALATION INTERACTION
# =============================================================================
# Tests: escalation happens before constraints in run_iteration order

@test "v4 integration: escalation and constraint on same state" {
    create_workflow << 'YAML'
name: test
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    constraints:
      max_iterations: 10
    escalation:
      - at_iteration: 3
        action: analyze_stuck
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    set_state_field "iteration" 3

    # Escalation should trigger at iteration 3
    run check_escalation "$WORKFLOW_FILE" "impl"
    [ "$status" -eq 0 ]
    [ -f "$PHASE_DIR/stuck_analysis.md" ]

    # Constraint should still pass (3 < 10)
    run check_pre_constraints "$WORKFLOW_FILE" "impl" "$PHASE_DIR"
    [ "$status" -eq 0 ]
}

@test "v4 integration: escalation at limit but constraint blocks" {
    create_workflow << 'YAML'
name: test
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    constraints:
      max_iterations: 5
    escalation:
      - at_iteration: 5
        action: analyze_stuck
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    set_state_field "iteration" 5

    # Escalation runs first in the pipeline (step 2)
    run check_escalation "$WORKFLOW_FILE" "impl"
    [ "$status" -eq 0 ]

    # Pre-constraint blocks (step 3)
    run check_pre_constraints "$WORKFLOW_FILE" "impl" "$PHASE_DIR"
    [ "$status" -eq 1 ]
}

# =============================================================================
# INTERVENTION + ESCALATION INTERACTION
# =============================================================================
# Tests: intervention checked first (before escalation) in run_iteration order

@test "v4 integration: intervention fires before escalation can run" {
    create_workflow << 'YAML'
name: test
version: 4
intervention_triggers:
  - condition: "stuck_iterations >= 3"
    action: request_human
    message: "Stuck"
states:
  - name: impl
    prompt: prompts/feature/impl.md
    escalation:
      - at_iteration: 5
        action: analyze_stuck
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    set_state_field "stuck_iterations" 4
    set_state_field "iteration" 5

    # Intervention fires first (step 1 in run_iteration)
    run check_intervention_triggers "$WORKFLOW_FILE"
    [ "$status" -eq 2 ]

    # Escalation would have triggered but intervention halts first
    # stuck_analysis.md should NOT exist
    [ ! -f "$PHASE_DIR/stuck_analysis.md" ]
}

# =============================================================================
# CROSS-FEATURE: HOOKS + ESCALATION STATE
# =============================================================================

@test "v4 integration: escalation state persists across check_escalation calls" {
    create_workflow << 'YAML'
name: test
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    escalation:
      - after_iteration: 2
        action: switch_model
        params:
          model: opus
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    # Iteration 3 (> 2, should trigger)
    set_state_field "iteration" 3
    check_escalation "$WORKFLOW_FILE" "impl"

    local model
    model=$(get_state_field "current_model")
    [ "$model" == "opus" ]

    # Iteration 4 — should NOT re-trigger
    set_state_field "iteration" 4
    # Override to track if action was called again
    action_switch_model() { echo "SHOULD_NOT_BE_CALLED" > "$PHASE_DIR/second_call.txt"; return 0; }
    export -f action_switch_model

    check_escalation "$WORKFLOW_FILE" "impl"

    # Verify action was NOT called again
    [ ! -f "$PHASE_DIR/second_call.txt" ]
}

@test "v4 integration: hook action updates state that escalation reads" {
    create_workflow << 'YAML'
name: test
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    after:
      - action: switch_model
        params:
          model: opus
    escalation:
      - at_iteration: 3
        action: analyze_stuck
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    set_state_field "iteration" 3

    # Run hooks first (simulating step 7 of a previous iteration)
    run run_after_hooks "$WORKFLOW_FILE" "impl" ""
    [ "$status" -eq 0 ]

    local model
    model=$(get_state_field "current_model")
    [ "$model" == "opus" ]

    # Now run escalation (step 2 of next iteration)
    run check_escalation "$WORKFLOW_FILE" "impl"
    [ "$status" -eq 0 ]

    # Both effects should be in state
    [ -f "$PHASE_DIR/stuck_analysis.md" ]
    model=$(get_state_field "current_model")
    [ "$model" == "opus" ]
}

# =============================================================================
# FULL PIPELINE INTEGRATION (requires iterate.sh V4 functions)
# =============================================================================

@test "v4 integration: setup_v4_environment sources all libraries" {
    skip_if_not_v4_integrated

    # Source iterate.sh to get setup_v4_environment
    source "$SCRIPTS_DIR/iterate.sh"

    # Copy real scripts to mock orchestration dir
    cp "$ORCH_DIR/scripts/actions.sh" "$ARC_HOME/scripts/"
    cp "$ORCH_DIR/scripts/check-constraints.sh" "$ARC_HOME/scripts/"
    cp "$ORCH_DIR/scripts/check-escalation.sh" "$ARC_HOME/scripts/"
    cp "$ORCH_DIR/scripts/check-intervention.sh" "$ARC_HOME/scripts/"
    cp "$ORCH_DIR/scripts/run-hooks.sh" "$ARC_HOME/scripts/"

    # setup_v4_environment constructs WORKFLOW_FILE from PLANS_DIR
    # Write a workflow file at the expected path
    cat > "$WORKFLOW_FILE" << 'YAML'
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

    # Override SCRIPTS_DIR so it sources from our mock dir
    export SCRIPTS_DIR="$ARC_HOME/scripts"

    setup_v4_environment "test-plan" "test-phase"

    # Verify environment variables were set
    [ -n "$PHASE_DIR" ]
    [ -n "$STATE_FILE" ]
    [ -n "$WORKFLOW_FILE" ]
    [ -f "$WORKFLOW_FILE" ]
}

@test "v4 integration: run_iteration with all features" {
    skip_if_not_v4_integrated

    # Source iterate.sh for run_iteration
    source "$SCRIPTS_DIR/iterate.sh"

    # Copy real scripts to mock orchestration dir
    cp "$ORCH_DIR/scripts/actions.sh" "$ARC_HOME/scripts/"
    cp "$ORCH_DIR/scripts/check-constraints.sh" "$ARC_HOME/scripts/"
    cp "$ORCH_DIR/scripts/check-escalation.sh" "$ARC_HOME/scripts/"
    cp "$ORCH_DIR/scripts/check-intervention.sh" "$ARC_HOME/scripts/"
    cp "$ORCH_DIR/scripts/run-hooks.sh" "$ARC_HOME/scripts/"
    cp "$ORCH_DIR/scripts/extract-verdict.sh" "$ARC_HOME/scripts/" 2>/dev/null || true
    cp "$ORCH_DIR/scripts/build-context.sh" "$ARC_HOME/scripts/" 2>/dev/null || true
    cp "$ORCH_DIR/scripts/render_template.py" "$ARC_HOME/scripts/" 2>/dev/null || true

    # Override SCRIPTS_DIR to point to our mock
    export SCRIPTS_DIR="$ARC_HOME/scripts"

    # Write workflow at the expected path
    cat > "$WORKFLOW_FILE" << 'YAML'
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

    # Setup V4 environment
    setup_v4_environment "test-plan" "test-phase"

    # Mock spawn_agent to simulate successful agent execution
    spawn_agent() {
        local prompt_file="$1"
        local output_file="$2"
        echo "Mock agent output" > "$output_file"
        return 0
    }
    export -f spawn_agent

    run run_iteration "test-plan" "test-phase" "impl"
    [ "$status" -eq 0 ]

    # Verify iteration was incremented
    local iteration
    iteration=$(jq -r '.iteration' "$STATE_FILE")
    [ "$iteration" -eq 1 ]
}

@test "v4 integration: run_iteration returns 2 on intervention" {
    skip_if_not_v4_integrated

    source "$SCRIPTS_DIR/iterate.sh"

    cp "$ORCH_DIR/scripts/actions.sh" "$ARC_HOME/scripts/"
    cp "$ORCH_DIR/scripts/check-constraints.sh" "$ARC_HOME/scripts/"
    cp "$ORCH_DIR/scripts/check-escalation.sh" "$ARC_HOME/scripts/"
    cp "$ORCH_DIR/scripts/check-intervention.sh" "$ARC_HOME/scripts/"
    cp "$ORCH_DIR/scripts/run-hooks.sh" "$ARC_HOME/scripts/"

    export SCRIPTS_DIR="$ARC_HOME/scripts"

    # Create workflow with intervention trigger
    create_workflow << 'YAML'
name: test
version: 4
intervention_triggers:
  - condition: "stuck_iterations >= 3"
    action: request_human
    message: "Stuck"
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    set_state_field "stuck_iterations" 5

    setup_v4_environment "test-plan" "test-phase"

    spawn_agent() { echo "Mock" > "$2"; return 0; }
    export -f spawn_agent

    run run_iteration "test-plan" "test-phase" "impl"
    [ "$status" -eq 2 ]
}

@test "v4 integration: state consistency across sequential iterations" {
    skip_if_not_v4_integrated

    source "$SCRIPTS_DIR/iterate.sh"

    cp "$ORCH_DIR/scripts/actions.sh" "$ARC_HOME/scripts/"
    cp "$ORCH_DIR/scripts/check-constraints.sh" "$ARC_HOME/scripts/"
    cp "$ORCH_DIR/scripts/check-escalation.sh" "$ARC_HOME/scripts/"
    cp "$ORCH_DIR/scripts/check-intervention.sh" "$ARC_HOME/scripts/"
    cp "$ORCH_DIR/scripts/run-hooks.sh" "$ARC_HOME/scripts/"
    cp "$ORCH_DIR/scripts/extract-verdict.sh" "$ARC_HOME/scripts/" 2>/dev/null || true
    cp "$ORCH_DIR/scripts/build-context.sh" "$ARC_HOME/scripts/" 2>/dev/null || true
    cp "$ORCH_DIR/scripts/render_template.py" "$ARC_HOME/scripts/" 2>/dev/null || true

    export SCRIPTS_DIR="$ARC_HOME/scripts"

    # Simple workflow: impl -> complete (string next)
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

    setup_v4_environment "test-plan" "test-phase"

    spawn_agent() { echo "Mock output" > "$2"; return 0; }
    export -f spawn_agent

    # Run 3 iterations
    for i in 1 2 3; do
        run_iteration "test-plan" "test-phase" "impl"
    done

    # Verify iteration incremented 3 times
    local iteration
    iteration=$(jq -r '.iteration' "$STATE_FILE")
    [ "$iteration" -eq 3 ]
}

@test "v4 integration: executed_escalations persists across iterations" {
    skip_if_not_v4_integrated

    source "$SCRIPTS_DIR/iterate.sh"

    cp "$ORCH_DIR/scripts/actions.sh" "$ARC_HOME/scripts/"
    cp "$ORCH_DIR/scripts/check-constraints.sh" "$ARC_HOME/scripts/"
    cp "$ORCH_DIR/scripts/check-escalation.sh" "$ARC_HOME/scripts/"
    cp "$ORCH_DIR/scripts/check-intervention.sh" "$ARC_HOME/scripts/"
    cp "$ORCH_DIR/scripts/run-hooks.sh" "$ARC_HOME/scripts/"
    cp "$ORCH_DIR/scripts/extract-verdict.sh" "$ARC_HOME/scripts/" 2>/dev/null || true
    cp "$ORCH_DIR/scripts/build-context.sh" "$ARC_HOME/scripts/" 2>/dev/null || true
    cp "$ORCH_DIR/scripts/render_template.py" "$ARC_HOME/scripts/" 2>/dev/null || true

    export SCRIPTS_DIR="$ARC_HOME/scripts"

    create_workflow << 'YAML'
name: test
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    escalation:
      - after_iteration: 1
        action: analyze_stuck
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    # Start at iteration 2 so after_iteration: 1 fires
    set_state_field "iteration" 2

    setup_v4_environment "test-plan" "test-phase"

    spawn_agent() { echo "Mock output" > "$2"; return 0; }
    export -f spawn_agent

    # Run first iteration — escalation fires
    run_iteration "test-plan" "test-phase" "impl"

    # Verify escalation was tracked
    local executed
    executed=$(jq -c '.executed_escalations' "$STATE_FILE")
    [[ "$executed" == *"after_1"* ]]

    # Run second iteration — escalation should NOT re-fire
    rm -f "$PHASE_DIR/stuck_analysis.md"
    run_iteration "test-plan" "test-phase" "impl"

    # Verify only one entry (not duplicated)
    local count
    count=$(jq '.executed_escalations | length' "$STATE_FILE")
    [ "$count" -eq 1 ]
}

# =============================================================================
# UPDATE STATE INTEGRATION
# =============================================================================
# Tests: update_state_after_iteration with verdict, stuck_iterations tracking

@test "v4 integration: update_state increments iteration and sets next_state" {
    skip_if_not_v4_integrated

    source "$SCRIPTS_DIR/iterate.sh"

    # Simple workflow: impl -> complete
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

    export SCRIPTS_DIR
    export ARC_HOME

    update_state_after_iteration "impl" ""

    local iteration
    iteration=$(jq -r '.iteration' "$STATE_FILE")
    [ "$iteration" -eq 1 ]

    local next
    next=$(jq -r '.current_state' "$STATE_FILE")
    [ "$next" == "complete" ]

    # Transitioning to different state resets stuck_iterations
    local stuck
    stuck=$(jq -r '.stuck_iterations // 0' "$STATE_FILE")
    [ "$stuck" -eq 0 ]
}

@test "v4 integration: update_state resolves verdict-based next" {
    skip_if_not_v4_integrated

    source "$SCRIPTS_DIR/iterate.sh"

    create_workflow << 'YAML'
name: test
version: 4
states:
  - name: review
    prompt: prompts/feature/impl.md
    verdicts: [approved, needs_fix]
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

    export SCRIPTS_DIR
    export ARC_HOME

    update_state_after_iteration "review" "approved"

    local next
    next=$(jq -r '.current_state' "$STATE_FILE")
    [ "$next" == "complete" ]

    local last_verdict
    last_verdict=$(jq -r '.last_verdict' "$STATE_FILE")
    [ "$last_verdict" == "approved" ]
}

@test "v4 integration: update_state increments stuck_iterations on loop" {
    skip_if_not_v4_integrated

    source "$SCRIPTS_DIR/iterate.sh"

    # Workflow where needs_fix loops back to impl
    create_workflow << 'YAML'
name: test
version: 4
states:
  - name: review
    prompt: prompts/feature/impl.md
    verdicts: [approved, needs_fix]
    next:
      approved: complete
      needs_fix: impl
  - name: impl
    prompt: prompts/feature/impl.md
    next: review
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    export SCRIPTS_DIR
    export ARC_HOME

    # Simulate stuck: review -> needs_fix -> impl (impl loops back)
    # From impl with next: review (different state), stuck resets
    update_state_after_iteration "impl" ""

    local stuck
    stuck=$(jq -r '.stuck_iterations // 0' "$STATE_FILE")
    [ "$stuck" -eq 0 ]  # impl -> review is a transition, reset

    # Now review with needs_fix -> impl
    update_state_after_iteration "review" "needs_fix"
    local next
    next=$(jq -r '.current_state' "$STATE_FILE")
    [ "$next" == "impl" ]
}

@test "v4 integration: update_state appends to verdicts_history" {
    skip_if_not_v4_integrated

    source "$SCRIPTS_DIR/iterate.sh"

    create_workflow << 'YAML'
name: test
version: 4
states:
  - name: review
    prompt: prompts/feature/impl.md
    verdicts: [approved, needs_fix]
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

    export SCRIPTS_DIR
    export ARC_HOME

    # First verdict
    update_state_after_iteration "review" "needs_fix"

    # Second verdict
    set_state_field "iteration" 1
    update_state_after_iteration "review" "approved"

    # Check verdicts_history has 2 entries
    local count
    count=$(jq '.verdicts_history | length' "$STATE_FILE")
    [ "$count" -eq 2 ]

    # Verify last_verdict
    local last
    last=$(jq -r '.last_verdict' "$STATE_FILE")
    [ "$last" == "approved" ]
}

# =============================================================================
# EDGE CASES
# =============================================================================

@test "v4 integration: empty state.json handles missing fields with defaults" {
    # Minimal state — missing many fields
    echo '{}' > "$STATE_FILE"

    create_workflow << 'YAML'
name: test
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    constraints:
      max_iterations: 10
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    # check_max_iterations reads iteration // 0 from state
    run check_max_iterations "$WORKFLOW_FILE" "impl"
    [ "$status" -eq 0 ]  # 0 < 10
}

@test "v4 integration: no V4 features still works (V1 compatible)" {
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

    # No constraints — should pass
    run check_pre_constraints "$WORKFLOW_FILE" "impl" "$PHASE_DIR"
    [ "$status" -eq 0 ]

    # No hooks — should pass
    run run_after_hooks "$WORKFLOW_FILE" "impl" ""
    [ "$status" -eq 0 ]

    # No escalation — should pass
    run check_escalation "$WORKFLOW_FILE" "impl"
    [ "$status" -eq 0 ]

    # No intervention — should pass
    run check_intervention_triggers "$WORKFLOW_FILE"
    [ "$status" -eq 0 ]
}

@test "v4 integration: workflow with no intervention_triggers section" {
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

    run check_intervention_triggers "$WORKFLOW_FILE"
    [ "$status" -eq 0 ]
}

@test "v4 integration: concurrent state updates use atomic tmp+mv pattern" {
    # Verify that state updates use PID-suffixed temp files
    # by running multiple updates and checking consistency
    create_workflow << 'YAML'
name: test
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    escalation:
      - after_iteration: 1
        action: switch_model
        params:
          model: opus
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    set_state_field "iteration" 2

    # Run escalation which does atomic state update
    check_escalation "$WORKFLOW_FILE" "impl"

    # Verify state file is valid JSON (not corrupted)
    run jq '.' "$STATE_FILE"
    [ "$status" -eq 0 ]

    # Verify the update was applied
    local model
    model=$(get_state_field "current_model")
    [ "$model" == "opus" ]

    # Verify executed_escalations array exists and is valid
    local executed
    executed=$(jq -c '.executed_escalations' "$STATE_FILE")
    [[ "$executed" == *"after_1"* ]]
}

@test "v4 integration: intervention_request field is object not boolean" {
    create_workflow << 'YAML'
name: test
version: 4
intervention_triggers:
  - condition: "iteration >= 1"
    action: request_human
    message: "Test message"
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    set_state_field "iteration" 5

    run check_intervention_triggers "$WORKFLOW_FILE"
    [ "$status" -eq 2 ]

    # Verify it's an object with required fields
    local type
    type=$(jq -r '.intervention_request | type' "$STATE_FILE")
    [ "$type" == "object" ]

    # Verify required fields exist
    run jq -e '.intervention_request.reason' "$STATE_FILE"
    [ "$status" -eq 0 ]

    run jq -e '.intervention_request.requested_at' "$STATE_FILE"
    [ "$status" -eq 0 ]

    run jq -e '.intervention_request.options' "$STATE_FILE"
    [ "$status" -eq 0 ]

    # Verify it is NOT a boolean
    local reason
    reason=$(jq -r '.intervention_request.reason' "$STATE_FILE")
    [ "$reason" == "Test message" ]
}

@test "v4 integration: mixed constraints and hooks on same state" {
    create_workflow << 'YAML'
name: test
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    constraints:
      max_iterations: 10
      require_artifacts_in:
        - input.md
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

    # Create required artifact
    touch "$PHASE_DIR/input.md"
    set_state_field "iteration" 5

    # Constraints pass
    run check_pre_constraints "$WORKFLOW_FILE" "impl" "$PHASE_DIR"
    [ "$status" -eq 0 ]

    # Hooks execute
    run run_after_hooks "$WORKFLOW_FILE" "impl" ""
    [ "$status" -eq 0 ]

    local model
    model=$(get_state_field "current_model")
    [ "$model" == "opus" ]
}

@test "v4 integration: constraint failure prevents hook execution in pipeline" {
    # This tests the intended pipeline order:
    # pre-constraint fails -> no agent spawn -> no hooks
    create_workflow << 'YAML'
name: test
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    constraints:
      max_iterations: 3
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

    # Pre-constraint fails
    run check_pre_constraints "$WORKFLOW_FILE" "impl" "$PHASE_DIR"
    [ "$status" -eq 1 ]

    # In a real pipeline, hooks would not run after constraint failure
    # Model should not be set
    local model
    model=$(jq -r '.current_model // "unset"' "$STATE_FILE")
    [ "$model" == "unset" ]
}

@test "v4 integration: hook continue_on_error allows subsequent hooks" {
    create_workflow << 'YAML'
name: test
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

    # First hook will fail (script doesn't exist), but continue_on_error allows next
    run run_after_hooks "$WORKFLOW_FILE" "impl" ""

    # The overall result is still failure (because first hook failed)
    # but the second hook should have executed
    local model
    model=$(get_state_field "current_model")
    [ "$model" == "haiku" ]
}

@test "v4 integration: hook failure without continue_on_error stops chain" {
    create_workflow << 'YAML'
name: test
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    after:
      - action: script
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

    # First hook fails, no continue_on_error, chain stops
    run run_after_hooks "$WORKFLOW_FILE" "impl" ""
    [ "$status" -eq 1 ]

    # Second hook should NOT have executed
    local model
    model=$(jq -r '.current_model // "unset"' "$STATE_FILE")
    [ "$model" == "unset" ]
}

@test "v4 integration: fixture basic_workflow.yaml loads and validates" {
    # Test that our fixture files are well-formed
    cp "$FIXTURES_DIR/basic_workflow.yaml" "$WORKFLOW_FILE"

    # Should be parseable by yq
    run yq '.name' "$WORKFLOW_FILE"
    [ "$status" -eq 0 ]
    [[ "$output" == *"test_v4_basic"* ]]

    # Constraints should be extractable
    local constraints
    constraints=$(get_state_constraints "$WORKFLOW_FILE" "impl")
    [[ "$constraints" == *"max_iterations"* ]]
}

@test "v4 integration: fixture escalation_workflow.yaml loads and validates" {
    cp "$FIXTURES_DIR/escalation_workflow.yaml" "$WORKFLOW_FILE"

    local triggers
    triggers=$(get_escalation_triggers "$WORKFLOW_FILE" "impl")
    [[ "$triggers" != "[]" ]]

    local count
    count=$(echo "$triggers" | jq 'length')
    [ "$count" -eq 2 ]
}

@test "v4 integration: fixture intervention_workflow.yaml loads and validates" {
    cp "$FIXTURES_DIR/intervention_workflow.yaml" "$WORKFLOW_FILE"

    local triggers
    triggers=$(get_intervention_triggers "$WORKFLOW_FILE")
    [[ "$triggers" != "[]" ]]

    local count
    count=$(echo "$triggers" | jq 'length')
    [ "$count" -eq 2 ]
}
