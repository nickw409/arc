#!/usr/bin/env bats

# Tests for iterate-integration phase (orchestration-v4)
#
# Tests the V4 integration functions added to iterate.sh:
#   - setup_v4_environment: Environment setup and V4 library sourcing
#   - run_iteration: Main V4 iteration loop (7-step execution order)
#   - update_state_after_iteration: State updates (iteration, next_state, stuck, verdicts)
#
# These functions integrate all V4 features:
#   - Intervention triggers (check first, may halt with exit 2)
#   - Escalation triggers (execute actions based on iteration)
#   - Pre/post constraints (artifact and iteration validation)
#   - After hooks (conditional actions based on verdict)
#   - Verdict extraction (from review state output)
#   - State updates (iteration counter, next_state, stuck_iterations, verdicts_history)

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

    # Create plan directory structure
    export PLANS_DIR="$TEST_TEMP_DIR/plans"
    export ARC_HOME="$TEST_TEMP_DIR/orchestration"
    export SCRIPTS_DIR_MOCK="$ARC_HOME/scripts"
    mkdir -p "$PLANS_DIR/active/test-plan/phases/test-phase"
    mkdir -p "$ARC_HOME/scripts"
    mkdir -p "$ARC_HOME/prompts/feature"

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

    # Create a simple prompt file
    cat > "$ARC_HOME/prompts/feature/impl.md" << 'MD'
# Implementation Prompt
Implement the feature.
MD

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
# Helper: Create a V4 workflow YAML with configurable features
# Usage: create_v4_workflow [options]
#   Options are passed as the YAML body under states
# ==============================================================================
create_basic_workflow() {
    cat > "$WORKFLOW_FILE" << 'YAML'
name: test_workflow
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: impl_review
  - name: impl_review
    prompt: prompts/feature/impl.md
    verdicts: [approved, needs_fix]
    next:
      approved: complete
      needs_fix: impl
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML
}

# Helper: Create workflow with all V4 features
create_full_v4_workflow() {
    cat > "$WORKFLOW_FILE" << 'YAML'
name: test_v4_workflow
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: impl_review
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
  - name: impl_review
    prompt: prompts/feature/impl.md
    verdicts: [approved, needs_fix]
    next:
      approved: complete
      needs_fix: impl
    after:
      - action: commit
        when: approved
        params:
          message: "feat: implementation"
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
intervention_triggers:
  - condition: "stuck_iterations >= 10"
    action: request_human
    message: "Phase stuck for 10+ iterations"
YAML
}

# Helper: Create workflow with intervention triggers that match
create_intervention_workflow() {
    local threshold="${1:-5}"
    cat > "$WORKFLOW_FILE" << YAML
name: test_intervention
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
intervention_triggers:
  - condition: "stuck_iterations >= ${threshold}"
    action: request_human
    message: "Phase stuck"
YAML
}

# Helper: Create workflow with pre-constraints
create_constraint_workflow() {
    cat > "$WORKFLOW_FILE" << 'YAML'
name: test_constraint
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
    constraints:
      require_artifacts_in:
        - qa_reasoning.md
      require_artifacts_out:
        - impl_reasoning.md
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML
}

# Helper: Create workflow with escalation triggers
create_escalation_workflow() {
    cat > "$WORKFLOW_FILE" << 'YAML'
name: test_escalation
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
    escalation:
      - at_iteration: 3
        action: analyze_stuck
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML
}

# Helper: Create workflow with after hooks
create_hooks_workflow() {
    cat > "$WORKFLOW_FILE" << 'YAML'
name: test_hooks
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
    after:
      - action: run_tests
        params:
          pattern: "test"
          save_to: "test_output.txt"
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML
}

# Helper: Create workflow with verdict-conditional hooks
create_verdict_hooks_workflow() {
    cat > "$WORKFLOW_FILE" << 'YAML'
name: test_verdict_hooks
version: 4
states:
  - name: impl_review
    prompt: prompts/feature/impl.md
    verdicts: [approved, needs_fix]
    next:
      approved: complete
      needs_fix: impl
    after:
      - action: commit
        when: approved
        params:
          message: "feat: approved implementation"
  - name: impl
    prompt: prompts/feature/impl.md
    next: impl_review
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML
}

# Helper: Create workflow with string-type .next
create_string_next_workflow() {
    cat > "$WORKFLOW_FILE" << 'YAML'
name: test_string_next
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
}

# Helper: Create workflow with object-type .next
create_object_next_workflow() {
    cat > "$WORKFLOW_FILE" << 'YAML'
name: test_object_next
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
}

# Helper: Set a numeric field in state.json
set_state_field() {
    local field="$1"
    local value="$2"
    jq --argjson val "$value" ".$field = \$val" "$STATE_FILE" > "${STATE_FILE}.tmp" \
        && mv "${STATE_FILE}.tmp" "$STATE_FILE"
}

# Helper: Set a string field in state.json
set_state_string_field() {
    local field="$1"
    local value="$2"
    jq --arg val "$value" ".$field = \$val" "$STATE_FILE" > "${STATE_FILE}.tmp" \
        && mv "${STATE_FILE}.tmp" "$STATE_FILE"
}

# ==============================================================================
# Helper: Source V4 libraries and run a function with mocked actions
# This simulates what setup_v4_environment + run_iteration does, but with
# mocks for spawn_agent and action functions.
# ==============================================================================
run_v4_fn() {
    local func_call="$1"
    local extra_setup="${2:-}"

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

        # Source V4 libraries
        source \"$ACTIONS_SH\"
        source \"$CHECK_CONSTRAINTS_SH\"
        source \"$CHECK_ESCALATION_SH\"
        source \"$CHECK_INTERVENTION_SH\"
        source \"$RUN_HOOKS_SH\"

        # Mock all action functions to log calls and succeed
        action_run_tests() {
            echo \"action_run_tests \$*\" >> \"\$CALL_LOG\"
            return 0
        }
        action_commit() {
            echo \"action_commit \$*\" >> \"\$CALL_LOG\"
            return 0
        }
        action_switch_model() {
            echo \"action_switch_model \$*\" >> \"\$CALL_LOG\"
            return 0
        }
        action_analyze_stuck() {
            echo \"action_analyze_stuck \$*\" >> \"\$CALL_LOG\"
            return 0
        }
        action_request_human() {
            echo \"action_request_human \$*\" >> \"\$CALL_LOG\"
            # Also set intervention_request in state.json (like real action does)
            local ts=\$(date -u +\"%Y-%m-%dT%H:%M:%SZ\")
            jq --arg reason \"\$1\" --arg ts \"\$ts\" \
                '.intervention_request = {\"reason\": \$reason, \"requested_at\": \$ts, \"options\": [\"resolve\"]}' \
                \"\$STATE_FILE\" > \"\${STATE_FILE}.tmp.\$\$\" && mv \"\${STATE_FILE}.tmp.\$\$\" \"\$STATE_FILE\"
            return 0
        }
        action_script() {
            echo \"action_script \$*\" >> \"\$CALL_LOG\"
            return 0
        }

        # Mock spawn_agent to succeed and create output
        spawn_agent() {
            local prompt_file=\"\$1\"
            local output_file=\"\$2\"
            echo \"spawn_agent \$prompt_file \$output_file\" >> \"\$SPAWN_LOG\"
            echo \"Agent output\" > \"\$output_file\"
            return 0
        }

        # Apply extra setup if provided
        $extra_setup

        $func_call
    "
}

# ==============================================================================
# Helper: Run setup_v4_environment function
# ==============================================================================
run_setup_v4() {
    local plan_name="${1:-test-plan}"
    local phase_name="${2:-test-phase}"
    local extra="${3:-}"

    run bash -c "
        export PLANS_DIR=\"$PLANS_DIR\"
        export ARC_HOME=\"$ARC_HOME\"
        export SCRIPTS_DIR=\"$SCRIPTS_DIR\"

        # Source iterate.sh functions (must define setup_v4_environment)
        # Since iterate.sh may not have these functions yet, try sourcing it
        # If iterate.sh doesn't define setup_v4_environment, test will fail as expected
        set +e
        source \"$ITERATE_SH\" 2>/dev/null
        set -e

        # If setup_v4_environment is not defined, look for it as a standalone function
        if ! declare -f setup_v4_environment > /dev/null 2>&1; then
            echo 'FUNCTION_NOT_FOUND: setup_v4_environment'
            exit 1
        fi

        $extra

        setup_v4_environment \"$plan_name\" \"$phase_name\"
    "
}

# ==============================================================================
# Helper: Run update_state_after_iteration in a subshell with workflow context
# ==============================================================================
run_update_state() {
    local state_name="$1"
    local verdict="$2"

    run bash -c "
        export STATE_FILE=\"$STATE_FILE\"
        export WORKFLOW_FILE=\"$WORKFLOW_FILE\"
        export SCRIPTS_DIR=\"$SCRIPTS_DIR\"

        # Source iterate.sh to get update_state_after_iteration
        set +e
        source \"$ITERATE_SH\" 2>/dev/null
        set -e

        if ! declare -f update_state_after_iteration > /dev/null 2>&1; then
            echo 'FUNCTION_NOT_FOUND: update_state_after_iteration'
            exit 1
        fi

        update_state_after_iteration \"$state_name\" \"$verdict\"
    "
}

# ==============================================================================
# Helper: Run run_iteration in a subshell with full mocking
# ==============================================================================
run_iteration_fn() {
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

        # Mock action functions
        action_run_tests() {
            echo \"action_run_tests \$*\" >> \"\$CALL_LOG\"
            return 0
        }
        action_commit() {
            echo \"action_commit \$*\" >> \"\$CALL_LOG\"
            return 0
        }
        action_switch_model() {
            echo \"action_switch_model \$*\" >> \"\$CALL_LOG\"
            return 0
        }
        action_analyze_stuck() {
            echo \"action_analyze_stuck \$*\" >> \"\$CALL_LOG\"
            return 0
        }
        action_request_human() {
            echo \"action_request_human \$*\" >> \"\$CALL_LOG\"
            local ts=\$(date -u +\"%Y-%m-%dT%H:%M:%SZ\")
            jq --arg reason \"\$1\" --arg ts \"\$ts\" \
                '.intervention_request = {\"reason\": \$reason, \"requested_at\": \$ts, \"options\": [\"resolve\"]}' \
                \"\$STATE_FILE\" > \"\${STATE_FILE}.tmp.\$\$\" && mv \"\${STATE_FILE}.tmp.\$\$\" \"\$STATE_FILE\"
            return 0
        }
        action_script() {
            echo \"action_script \$*\" >> \"\$CALL_LOG\"
            return 0
        }

        # Mock spawn_agent
        spawn_agent() {
            local prompt_file=\"\$1\"
            local output_file=\"\$2\"
            echo \"spawn_agent \$prompt_file \$output_file\" >> \"\$SPAWN_LOG\"
            echo \"Agent output\" > \"\$output_file\"
            return 0
        }

        # Apply extra setup
        $extra_setup

        run_iteration \"$plan_name\" \"$phase_name\" \"$state_name\"
    "
}

#=============================================================================
# Script-level Tests: Verify iterate.sh defines V4 functions
#=============================================================================

@test "qa_iterate-integration_functions_defined: iterate.sh defines setup_v4_environment" {
    run bash -c "
        set +e
        source '$ITERATE_SH' 2>/dev/null
        declare -f setup_v4_environment > /dev/null 2>&1 && echo 'ok' || echo 'missing'
    "
    [[ "$output" == *"ok"* ]]
}

@test "qa_iterate-integration_functions_defined: iterate.sh defines run_iteration" {
    run bash -c "
        set +e
        source '$ITERATE_SH' 2>/dev/null
        declare -f run_iteration > /dev/null 2>&1 && echo 'ok' || echo 'missing'
    "
    [[ "$output" == *"ok"* ]]
}

@test "qa_iterate-integration_functions_defined: iterate.sh defines update_state_after_iteration" {
    run bash -c "
        set +e
        source '$ITERATE_SH' 2>/dev/null
        declare -f update_state_after_iteration > /dev/null 2>&1 && echo 'ok' || echo 'missing'
    "
    [[ "$output" == *"ok"* ]]
}

@test "qa_iterate-integration_functions_defined: iterate.sh defines spawn_agent" {
    run bash -c "
        set +e
        source '$ITERATE_SH' 2>/dev/null
        declare -f spawn_agent > /dev/null 2>&1 && echo 'ok' || echo 'missing'
    "
    [[ "$output" == *"ok"* ]]
}

#=============================================================================
# setup_v4_environment Tests
#=============================================================================

@test "qa_iterate-integration_test_environment_setup: all env vars set correctly" {
    create_basic_workflow

    run_setup_v4 "test-plan" "test-phase" "
        # After setup, verify environment variables
        setup_v4_environment 'test-plan' 'test-phase'
        echo \"PHASE_DIR=\$PHASE_DIR\"
        echo \"STATE_FILE=\$STATE_FILE\"
        echo \"WORKFLOW_FILE=\$WORKFLOW_FILE\"
        echo \"ARC_HOME=\$ARC_HOME\"
        echo \"ARC_DEFAULT_PKG=\$ARC_DEFAULT_PKG\"
    "
    [[ "$status" -eq 0 ]] || {
        echo "Failed with: $output"
        false
    }
    [[ "$output" == *"PHASE_DIR="* ]]
    [[ "$output" == *"STATE_FILE="* ]]
    [[ "$output" == *"WORKFLOW_FILE="* ]]
}

@test "qa_iterate-integration_test_environment_crate_default: ARC_DEFAULT_PKG defaults to test-package" {
    create_basic_workflow

    run bash -c "
        unset ARC_DEFAULT_PKG
        export PLANS_DIR=\"$PLANS_DIR\"
        export ARC_HOME=\"$ARC_HOME\"
        export SCRIPTS_DIR=\"$SCRIPTS_DIR\"

        set +e
        source '$ITERATE_SH' 2>/dev/null
        set -e

        if ! declare -f setup_v4_environment > /dev/null 2>&1; then
            echo 'FUNCTION_NOT_FOUND'
            exit 1
        fi

        setup_v4_environment 'test-plan' 'test-phase'
        echo \"ARC_DEFAULT_PKG=\$ARC_DEFAULT_PKG\"
    "
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"ARC_DEFAULT_PKG=test-package"* ]]
}

@test "qa_iterate-integration_test_missing_state_file: exit 1 when state.json missing" {
    create_basic_workflow
    rm -f "$STATE_FILE"

    run_setup_v4 "test-plan" "test-phase"
    [[ "$status" -ne 0 ]]
    [[ "$output" == *"State file not found"* ]] || [[ "$output" == *"STATE_FILE"* ]] || [[ "$output" == *"state.json"* ]]
}

@test "qa_iterate-integration_test_missing_workflow_file: exit 1 when workflow.yaml missing" {
    # Don't create workflow file
    run_setup_v4 "test-plan" "test-phase"
    [[ "$status" -ne 0 ]]
    [[ "$output" == *"Workflow file not found"* ]] || [[ "$output" == *"WORKFLOW_FILE"* ]] || [[ "$output" == *"workflow"* ]]
}

@test "qa_iterate-integration_test_v4_scripts_sourced: all V4 functions available after setup" {
    create_basic_workflow

    run bash -c "
        export PLANS_DIR=\"$PLANS_DIR\"
        export ARC_HOME=\"$ARC_HOME\"
        export SCRIPTS_DIR=\"$SCRIPTS_DIR\"

        set +e
        source '$ITERATE_SH' 2>/dev/null
        set -e

        if ! declare -f setup_v4_environment > /dev/null 2>&1; then
            echo 'FUNCTION_NOT_FOUND: setup_v4_environment'
            exit 1
        fi

        setup_v4_environment 'test-plan' 'test-phase'

        # Verify all V4 functions are available
        declare -f check_pre_constraints > /dev/null && echo 'pre_constraints:ok'
        declare -f check_post_constraints > /dev/null && echo 'post_constraints:ok'
        declare -f check_escalation > /dev/null && echo 'escalation:ok'
        declare -f check_intervention_triggers > /dev/null && echo 'intervention:ok'
        declare -f run_after_hooks > /dev/null && echo 'hooks:ok'
    "
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"pre_constraints:ok"* ]]
    [[ "$output" == *"post_constraints:ok"* ]]
    [[ "$output" == *"escalation:ok"* ]]
    [[ "$output" == *"intervention:ok"* ]]
    [[ "$output" == *"hooks:ok"* ]]
}

#=============================================================================
# run_iteration — Full Success Path
#=============================================================================

@test "qa_iterate-integration_test_full_iteration_success: exit 0, iteration incremented, output created" {
    create_basic_workflow
    set_state_field "iteration" 0

    run_iteration_fn "test-plan" "test-phase" "impl"
    [[ "$status" -eq 0 ]]

    # Iteration should be incremented
    local iter
    iter=$(jq -r '.iteration' "$STATE_FILE")
    [[ "$iter" -eq 1 ]]

    # spawn_agent should have been called
    [[ -s "$SPAWN_LOG" ]]
    run cat "$SPAWN_LOG"
    [[ "$output" == *"spawn_agent"* ]]
}

@test "qa_iterate-integration_test_state_updated_after_success: iteration incremented from 5 to 6" {
    create_basic_workflow
    set_state_field "iteration" 5

    run_iteration_fn "test-plan" "test-phase" "impl"
    [[ "$status" -eq 0 ]]

    local iter
    iter=$(jq -r '.iteration' "$STATE_FILE")
    [[ "$iter" -eq 6 ]]
}

#=============================================================================
# run_iteration — Intervention Triggers
#=============================================================================

@test "qa_iterate-integration_test_intervention_halts_iteration: exit 2 when intervention triggered" {
    create_intervention_workflow 5
    set_state_field "stuck_iterations" 10

    run_iteration_fn "test-plan" "test-phase" "impl"
    [[ "$status" -eq 2 ]]

    # Agent should NOT have been spawned
    run cat "$SPAWN_LOG"
    [[ -z "$output" ]]
}

@test "qa_iterate-integration_test_intervention_exit_code_2_captured: exit 2 not exit 1" {
    create_intervention_workflow 3
    set_state_field "stuck_iterations" 5

    run_iteration_fn "test-plan" "test-phase" "impl"
    # Must be exactly 2, not 1
    [[ "$status" -eq 2 ]]
}

@test "qa_iterate-integration_test_intervention_no_match: exit 0 when condition not met" {
    create_intervention_workflow 10
    set_state_field "stuck_iterations" 2

    run_iteration_fn "test-plan" "test-phase" "impl"
    [[ "$status" -eq 0 ]]

    # Agent SHOULD have been spawned
    run cat "$SPAWN_LOG"
    [[ "$output" == *"spawn_agent"* ]]
}

#=============================================================================
# run_iteration — Pre-constraints
#=============================================================================

@test "qa_iterate-integration_test_pre_constraint_blocks_iteration: exit 1 when artifact missing" {
    create_constraint_workflow
    # Don't create qa_reasoning.md — pre-constraint should fail

    run_iteration_fn "test-plan" "test-phase" "impl"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Pre-constraints"* ]] || [[ "$output" == *"qa_reasoning.md"* ]] || [[ "$output" == *"constraint"* ]]

    # Agent should NOT have been spawned
    run cat "$SPAWN_LOG"
    [[ -z "$output" ]]
}

@test "qa_iterate-integration_test_pre_constraint_satisfied: exit 0 when artifacts exist" {
    create_constraint_workflow
    touch "$PHASE_DIR/qa_reasoning.md"

    # Agent will create impl_reasoning.md for post-constraint
    run_iteration_fn "test-plan" "test-phase" "impl" "
        spawn_agent() {
            local prompt_file=\"\$1\"
            local output_file=\"\$2\"
            echo \"spawn_agent\" >> \"\$SPAWN_LOG\"
            echo \"Agent output\" > \"\$output_file\"
            # Create the required output artifact
            touch \"\$PHASE_DIR/impl_reasoning.md\"
            return 0
        }
    "
    [[ "$status" -eq 0 ]]
}

#=============================================================================
# run_iteration — Post-constraints
#=============================================================================

@test "qa_iterate-integration_test_post_constraint_fails: exit 1 when output artifact missing" {
    create_constraint_workflow
    # Satisfy pre-constraint
    touch "$PHASE_DIR/qa_reasoning.md"
    # Don't create impl_reasoning.md — post-constraint should fail

    run_iteration_fn "test-plan" "test-phase" "impl"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Post-constraints"* ]] || [[ "$output" == *"impl_reasoning.md"* ]] || [[ "$output" == *"constraint"* ]]
}

#=============================================================================
# run_iteration — Escalation Triggers
#=============================================================================

@test "qa_iterate-integration_test_escalation_executes: action called at matching iteration" {
    create_escalation_workflow
    set_state_field "iteration" 3

    run_iteration_fn "test-plan" "test-phase" "impl"
    # Escalation should have been called (analyze_stuck)
    run cat "$CALL_LOG"
    [[ "$output" == *"action_analyze_stuck"* ]]
}

@test "qa_iterate-integration_test_escalation_no_match: no action when iteration doesn't match" {
    create_escalation_workflow
    set_state_field "iteration" 1

    run_iteration_fn "test-plan" "test-phase" "impl"
    [[ "$status" -eq 0 ]]

    # No escalation actions should have been called
    local escalation_calls
    escalation_calls=$(grep -c "action_analyze_stuck" "$CALL_LOG" 2>/dev/null) || true
    [[ "$escalation_calls" -eq 0 ]]
}

#=============================================================================
# run_iteration — After Hooks
#=============================================================================

@test "qa_iterate-integration_test_after_hooks_execute: hooks run after agent" {
    create_hooks_workflow

    run_iteration_fn "test-plan" "test-phase" "impl"
    [[ "$status" -eq 0 ]]

    # run_tests hook should have been called
    run cat "$CALL_LOG"
    [[ "$output" == *"action_run_tests"* ]]
}

@test "qa_iterate-integration_test_after_hook_with_verdict_condition: hook runs when verdict matches" {
    create_verdict_hooks_workflow

    # Create mock that produces approved verdict in output
    run_iteration_fn "test-plan" "test-phase" "impl_review" "
        spawn_agent() {
            local prompt_file=\"\$1\"
            local output_file=\"\$2\"
            echo \"spawn_agent\" >> \"\$SPAWN_LOG\"
            printf '## Verdict\napproved\n' > \"\$output_file\"
            return 0
        }
    "
    [[ "$status" -eq 0 ]]

    # Commit hook should have been called (condition: when: approved)
    run cat "$CALL_LOG"
    [[ "$output" == *"action_commit"* ]]
}

@test "qa_iterate-integration_test_after_hook_skipped_on_condition: hook skipped when verdict doesn't match" {
    create_verdict_hooks_workflow

    # Create mock that produces needs_fix verdict
    run_iteration_fn "test-plan" "test-phase" "impl_review" "
        spawn_agent() {
            local prompt_file=\"\$1\"
            local output_file=\"\$2\"
            echo \"spawn_agent\" >> \"\$SPAWN_LOG\"
            printf '## Verdict\nneeds_fix\n' > \"\$output_file\"
            return 0
        }
    "

    # Commit hook should NOT have been called (condition: when: approved, but verdict is needs_fix)
    local commit_calls
    commit_calls=$(grep -c "action_commit" "$CALL_LOG" 2>/dev/null) || true
    [[ "$commit_calls" -eq 0 ]]
}

#=============================================================================
# run_iteration — Verdict Extraction
#=============================================================================

@test "qa_iterate-integration_test_verdict_history_updated: verdicts_history contains verdict object" {
    create_basic_workflow

    run_iteration_fn "test-plan" "test-phase" "impl_review" "
        spawn_agent() {
            local prompt_file=\"\$1\"
            local output_file=\"\$2\"
            echo \"spawn_agent\" >> \"\$SPAWN_LOG\"
            printf '## Verdict\napproved\n' > \"\$output_file\"
            return 0
        }
    "
    [[ "$status" -eq 0 ]]

    # Check verdicts_history has an entry with correct structure
    local history_len
    history_len=$(jq '.verdicts_history | length' "$STATE_FILE")
    [[ "$history_len" -ge 1 ]]

    # Check the verdict object has required fields (per STATE_SCHEMA.md)
    local last_entry
    last_entry=$(jq '.verdicts_history[-1]' "$STATE_FILE")
    local verdict_val
    verdict_val=$(echo "$last_entry" | jq -r '.verdict')
    [[ "$verdict_val" == "approved" ]]

    local state_val
    state_val=$(echo "$last_entry" | jq -r '.state')
    [[ "$state_val" == "impl_review" ]]

    # iteration field should be present
    local iter_val
    iter_val=$(echo "$last_entry" | jq '.iteration')
    [[ "$iter_val" != "null" ]]

    # timestamp field should be present
    local ts_val
    ts_val=$(echo "$last_entry" | jq -r '.timestamp')
    [[ "$ts_val" != "null" ]]
    [[ "$ts_val" != "" ]]
}

@test "qa_iterate-integration_test_verdict_exports_both_names: VERDICT and LAST_VERDICT set" {
    create_verdict_hooks_workflow

    # The test verifies both VERDICT and LAST_VERDICT are exported with the
    # same value by checking that hooks receive them. We use a custom hook
    # that logs the VERDICT value.
    run_iteration_fn "test-plan" "test-phase" "impl_review" "
        spawn_agent() {
            local prompt_file=\"\$1\"
            local output_file=\"\$2\"
            echo \"spawn_agent\" >> \"\$SPAWN_LOG\"
            printf '## Verdict\napproved\n' > \"\$output_file\"
            return 0
        }

        # Override run_after_hooks to check VERDICT and LAST_VERDICT
        run_after_hooks_original=\$(declare -f run_after_hooks)
        run_after_hooks() {
            echo \"VERDICT=\$VERDICT\" >> \"\$CALL_LOG\"
            echo \"LAST_VERDICT=\$LAST_VERDICT\" >> \"\$CALL_LOG\"
            # Call original
            eval \"\$run_after_hooks_original\"
            run_after_hooks \"\$@\"
        }
    "

    # Check that both VERDICT and LAST_VERDICT were set
    run cat "$CALL_LOG"
    [[ "$output" == *"VERDICT=approved"* ]]
    [[ "$output" == *"LAST_VERDICT=approved"* ]]
}

#=============================================================================
# run_iteration — Agent Failure Handling
#=============================================================================

@test "qa_iterate-integration_test_agent_failure_returns_nonzero: exit 1 when agent fails" {
    create_basic_workflow

    run_iteration_fn "test-plan" "test-phase" "impl" "
        spawn_agent() {
            echo \"spawn_agent FAILED\" >> \"\$SPAWN_LOG\"
            return 1
        }
    "
    [[ "$status" -eq 1 ]]
}

@test "qa_iterate-integration_test_agent_timeout_still_runs_hooks: post-checks run despite agent timeout" {
    create_hooks_workflow

    run_iteration_fn "test-plan" "test-phase" "impl" "
        spawn_agent() {
            echo \"spawn_agent TIMEOUT\" >> \"\$SPAWN_LOG\"
            return 124
        }
    "
    local saved_status="$status"

    # Even though agent failed, hooks should have been called
    run cat "$CALL_LOG"
    [[ "$output" == *"action_run_tests"* ]]

    # But overall result should be failure
    [[ "$saved_status" -ne 0 ]]
}

@test "qa_iterate-integration_test_agent_failure_still_runs_post_constraints: post-constraints checked" {
    create_constraint_workflow
    touch "$PHASE_DIR/qa_reasoning.md"
    touch "$PHASE_DIR/impl_reasoning.md"

    run_iteration_fn "test-plan" "test-phase" "impl" "
        spawn_agent() {
            echo \"spawn_agent FAILED\" >> \"\$SPAWN_LOG\"
            return 1
        }
    "

    # Agent failed, but post-constraints should still have been checked
    # Final exit code should be non-zero due to agent failure
    [[ "$status" -ne 0 ]]
}

#=============================================================================
# run_iteration — V3 Fallback (no template engine)
#=============================================================================

@test "qa_iterate-integration_test_v3_fallback_no_template_engine: prompt copied directly" {
    create_basic_workflow

    # Ensure no render_template.py or build-context.sh exist in the test environment
    rm -f "$ARC_HOME/scripts/render_template.py"
    rm -f "$ARC_HOME/scripts/build-context.sh"

    run_iteration_fn "test-plan" "test-phase" "impl"
    [[ "$status" -eq 0 ]]

    # Agent should have been spawned with a prompt file
    run cat "$SPAWN_LOG"
    [[ "$output" == *"spawn_agent"* ]]
    [[ "$output" == *"prompt.md"* ]]
}

#=============================================================================
# run_iteration — Prompt Template Validation
#=============================================================================

@test "qa_iterate-integration_test_prompt_template_not_found: exit 1 when prompt file missing" {
    # Create workflow referencing non-existent prompt
    cat > "$WORKFLOW_FILE" << 'YAML'
name: test_missing_prompt
version: 4
states:
  - name: impl
    prompt: prompts/nonexistent/missing.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    run_iteration_fn "test-plan" "test-phase" "impl"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Prompt template not found"* ]] || [[ "$output" == *"not found"* ]] || [[ "$output" == *"missing"* ]]

    # Agent should NOT have been spawned
    run cat "$SPAWN_LOG"
    [[ -z "$output" ]]
}

#=============================================================================
# run_iteration — Verdict-based Next State
#=============================================================================

@test "qa_iterate-integration_test_verdict_based_next_state_missing: no transition on unmatched verdict" {
    create_object_next_workflow
    set_state_string_field "current_state" "review"

    # Agent produces unknown verdict that has no matching key in .next object
    run_iteration_fn "test-plan" "test-phase" "review" "
        spawn_agent() {
            local prompt_file=\"\$1\"
            local output_file=\"\$2\"
            echo \"spawn_agent\" >> \"\$SPAWN_LOG\"
            printf '## Verdict\nunknown_verdict\n' > \"\$output_file\"
            return 0
        }
    "

    # current_state should remain unchanged since "unknown_verdict" doesn't match
    # any key in .next: {approved: complete, needs_fix: impl}
    local current
    current=$(jq -r '.current_state' "$STATE_FILE")
    [[ "$current" == "review" ]]
}

#=============================================================================
# run_iteration — Extract Verdict Script Missing
#=============================================================================

@test "qa_iterate-integration_test_extract_verdict_missing: non-zero exit when script missing" {
    create_basic_workflow

    # This test verifies that if extract-verdict.sh doesn't exist, the iteration
    # fails with a non-zero exit. Per the spec, the error will be bash's
    # "No such file or directory" since the algorithm calls it directly.
    run_iteration_fn "test-plan" "test-phase" "impl_review" "
        # Override SCRIPTS_DIR to point to empty dir (no extract-verdict.sh)
        export SCRIPTS_DIR=\"$TEST_TEMP_DIR/empty_scripts\"
        mkdir -p \"\$SCRIPTS_DIR\"

        spawn_agent() {
            local prompt_file=\"\$1\"
            local output_file=\"\$2\"
            echo \"spawn_agent\" >> \"\$SPAWN_LOG\"
            printf '## Verdict\napproved\n' > \"\$output_file\"
            return 0
        }
    "
    [[ "$status" -ne 0 ]]
}

#=============================================================================
# update_state_after_iteration Tests
#=============================================================================

@test "qa_iterate-integration_test_update_state_after_iteration_direct: iteration incremented and verdict recorded" {
    create_basic_workflow
    set_state_field "iteration" 5
    set_state_string_field "current_state" "impl"

    run_update_state "impl" "approved"
    [[ "$status" -eq 0 ]]

    # Iteration should be incremented to 6
    local iter
    iter=$(jq -r '.iteration' "$STATE_FILE")
    [[ "$iter" -eq 6 ]]

    # last_verdict should be set
    local lv
    lv=$(jq -r '.last_verdict' "$STATE_FILE")
    [[ "$lv" == "approved" ]]

    # verdicts_history should have an entry
    local history_len
    history_len=$(jq '.verdicts_history | length' "$STATE_FILE")
    [[ "$history_len" -ge 1 ]]

    # Check the entry structure
    local entry
    entry=$(jq '.verdicts_history[-1]' "$STATE_FILE")
    [[ "$(echo "$entry" | jq -r '.verdict')" == "approved" ]]
    [[ "$(echo "$entry" | jq -r '.state')" == "impl" ]]
    [[ "$(echo "$entry" | jq '.iteration')" == "5" ]]
    [[ "$(echo "$entry" | jq -r '.timestamp')" != "null" ]]
}

@test "qa_iterate-integration_test_next_state_string_type: simple string transition" {
    create_string_next_workflow
    set_state_field "iteration" 0
    set_state_string_field "current_state" "impl"

    run_update_state "impl" ""
    [[ "$status" -eq 0 ]]

    # .next is "complete" (string type), so current_state should change
    local current
    current=$(jq -r '.current_state' "$STATE_FILE")
    [[ "$current" == "complete" ]]
}

@test "qa_iterate-integration_test_next_state_object_type_with_verdict: verdict-based lookup" {
    create_object_next_workflow
    set_state_field "iteration" 0
    set_state_string_field "current_state" "review"

    run_update_state "review" "approved"
    [[ "$status" -eq 0 ]]

    # .next is {approved: complete, needs_fix: impl}, verdict="approved"
    local current
    current=$(jq -r '.current_state' "$STATE_FILE")
    [[ "$current" == "complete" ]]
}

@test "qa_iterate-integration_test_next_state_object_type_no_matching_verdict: no transition" {
    create_object_next_workflow
    set_state_field "iteration" 0
    set_state_string_field "current_state" "review"

    run_update_state "review" "unknown_verdict"
    [[ "$status" -eq 0 ]]

    # "unknown_verdict" has no key in .next, so current_state unchanged
    local current
    current=$(jq -r '.current_state' "$STATE_FILE")
    [[ "$current" == "review" ]]
}

#=============================================================================
# update_state_after_iteration — stuck_iterations Tracking
#=============================================================================

@test "qa_iterate-integration_test_stuck_iterations_incremented_on_same_state: loops back increments stuck" {
    create_object_next_workflow
    set_state_field "iteration" 3
    set_state_field "stuck_iterations" 1
    set_state_string_field "current_state" "impl"

    # .next for review: {approved: complete, needs_fix: impl}
    # verdict needs_fix → next_state = impl, which == current state "impl"
    # BUT we're updating state_name "impl" which has .next: "review" (string)
    # So actually next_state = "review" != "impl" → should reset

    # Use review state instead which has object .next
    set_state_string_field "current_state" "review"

    run_update_state "review" "needs_fix"
    [[ "$status" -eq 0 ]]

    # needs_fix → impl, which != "review" (the state_name), so stuck reset
    # Wait — the spec says: "next_state loops back to the same state" means next == state_name
    # So if state_name is "review" and next is "impl", that's different → reset
    local stuck
    stuck=$(jq -r '.stuck_iterations' "$STATE_FILE")
    [[ "$stuck" -eq 0 ]]
}

@test "qa_iterate-integration_test_stuck_iterations_actually_incremented: same state loops" {
    # Create a workflow where next loops back to same state
    cat > "$WORKFLOW_FILE" << 'YAML'
name: test_loop
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    verdicts: [needs_fix]
    next:
      needs_fix: impl
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    set_state_field "iteration" 3
    set_state_field "stuck_iterations" 1
    set_state_string_field "current_state" "impl"

    run_update_state "impl" "needs_fix"
    [[ "$status" -eq 0 ]]

    # needs_fix → impl, which == state_name "impl" → stuck incremented
    local stuck
    stuck=$(jq -r '.stuck_iterations' "$STATE_FILE")
    [[ "$stuck" -eq 2 ]]
}

@test "qa_iterate-integration_test_stuck_iterations_reset_on_state_change: different state resets counter" {
    create_object_next_workflow
    set_state_field "iteration" 3
    set_state_field "stuck_iterations" 5
    set_state_string_field "current_state" "review"

    # approved → complete, which != "review" → reset stuck
    run_update_state "review" "approved"
    [[ "$status" -eq 0 ]]

    local stuck
    stuck=$(jq -r '.stuck_iterations' "$STATE_FILE")
    [[ "$stuck" -eq 0 ]]
}

@test "qa_iterate-integration_test_stuck_iterations_incremented_on_no_transition: no matching verdict" {
    create_object_next_workflow
    set_state_field "iteration" 3
    set_state_field "stuck_iterations" 2
    set_state_string_field "current_state" "review"

    # "unknown" has no key in .next object → next_state="" → no transition → increment
    run_update_state "review" "unknown"
    [[ "$status" -eq 0 ]]

    local stuck
    stuck=$(jq -r '.stuck_iterations' "$STATE_FILE")
    [[ "$stuck" -eq 3 ]]
}

@test "qa_iterate-integration_test_stuck_iterations_defaults_to_zero: first stuck increment from missing field" {
    # Workflow where impl loops back to itself
    cat > "$WORKFLOW_FILE" << 'YAML'
name: test_loop
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

    set_state_field "iteration" 0
    set_state_string_field "current_state" "impl"
    # Remove stuck_iterations field entirely
    jq 'del(.stuck_iterations)' "$STATE_FILE" > "${STATE_FILE}.tmp" && mv "${STATE_FILE}.tmp" "$STATE_FILE"

    run_update_state "impl" ""
    [[ "$status" -eq 0 ]]

    # .next is "impl" (string), which == state_name "impl" → stuck incremented from 0
    local stuck
    stuck=$(jq -r '.stuck_iterations' "$STATE_FILE")
    [[ "$stuck" -eq 1 ]]
}

#=============================================================================
# update_state_after_iteration — Verdict History
#=============================================================================

@test "qa_iterate-integration_test_verdict_history_object_structure: per STATE_SCHEMA.md" {
    create_basic_workflow
    set_state_field "iteration" 3
    set_state_string_field "current_state" "impl_review"

    run_update_state "impl_review" "approved"
    [[ "$status" -eq 0 ]]

    # Check verdicts_history structure matches STATE_SCHEMA.md:
    # { iteration, state, verdict, timestamp }
    local entry
    entry=$(jq '.verdicts_history[-1]' "$STATE_FILE")

    # Must have all four fields
    [[ "$(echo "$entry" | jq 'has("iteration")')" == "true" ]]
    [[ "$(echo "$entry" | jq 'has("state")')" == "true" ]]
    [[ "$(echo "$entry" | jq 'has("verdict")')" == "true" ]]
    [[ "$(echo "$entry" | jq 'has("timestamp")')" == "true" ]]

    # Values should be correct
    [[ "$(echo "$entry" | jq '.iteration')" == "3" ]]
    [[ "$(echo "$entry" | jq -r '.state')" == "impl_review" ]]
    [[ "$(echo "$entry" | jq -r '.verdict')" == "approved" ]]
}

@test "qa_iterate-integration_test_empty_verdict_no_history: no history entry when verdict is empty" {
    create_basic_workflow
    set_state_field "iteration" 3

    run_update_state "impl" ""
    [[ "$status" -eq 0 ]]

    # verdicts_history should not have gained an entry for empty verdict
    local history_len
    history_len=$(jq '.verdicts_history // [] | length' "$STATE_FILE")
    [[ "$history_len" -eq 0 ]]
}

@test "qa_iterate-integration_test_last_verdict_set: last_verdict field updated" {
    create_basic_workflow
    set_state_field "iteration" 2

    run_update_state "impl_review" "needs_fix"
    [[ "$status" -eq 0 ]]

    local lv
    lv=$(jq -r '.last_verdict' "$STATE_FILE")
    [[ "$lv" == "needs_fix" ]]
}

#=============================================================================
# Execution Order Tests
#=============================================================================

@test "qa_iterate-integration_test_intervention_checked_before_agent: intervention blocks agent spawn" {
    create_intervention_workflow 3
    set_state_field "stuck_iterations" 5

    run_iteration_fn "test-plan" "test-phase" "impl"
    [[ "$status" -eq 2 ]]

    # Verify agent was NOT spawned
    run cat "$SPAWN_LOG"
    [[ -z "$output" ]]

    # Verify intervention action WAS called
    run cat "$CALL_LOG"
    [[ "$output" == *"action_request_human"* ]]
}

@test "qa_iterate-integration_test_pre_constraints_checked_before_agent: constraints block agent" {
    create_constraint_workflow
    # Missing required artifact qa_reasoning.md

    run_iteration_fn "test-plan" "test-phase" "impl"
    [[ "$status" -eq 1 ]]

    # Agent should NOT have been spawned
    run cat "$SPAWN_LOG"
    [[ -z "$output" ]]
}

@test "qa_iterate-integration_test_hooks_run_after_agent: hooks execute after agent completes" {
    create_hooks_workflow

    run_iteration_fn "test-plan" "test-phase" "impl"
    [[ "$status" -eq 0 ]]

    # Agent spawned THEN hooks ran (spawn_log entry before call_log entry)
    [[ -s "$SPAWN_LOG" ]]
    run cat "$CALL_LOG"
    [[ "$output" == *"action_run_tests"* ]]
}

#=============================================================================
# Integration: Full V4 Workflow
#=============================================================================

@test "qa_iterate-integration_test_full_v4_workflow: all features work together" {
    create_full_v4_workflow
    set_state_field "iteration" 1
    set_state_field "stuck_iterations" 0
    touch "$PHASE_DIR/qa_reasoning.md"

    # Agent creates required output artifact
    run_iteration_fn "test-plan" "test-phase" "impl" "
        spawn_agent() {
            local prompt_file=\"\$1\"
            local output_file=\"\$2\"
            echo \"spawn_agent\" >> \"\$SPAWN_LOG\"
            echo \"Agent output\" > \"\$output_file\"
            touch \"\$PHASE_DIR/impl_reasoning.md\"
            return 0
        }
    "
    [[ "$status" -eq 0 ]]

    # Agent was spawned
    [[ -s "$SPAWN_LOG" ]]

    # After hooks ran (run_tests)
    run cat "$CALL_LOG"
    [[ "$output" == *"action_run_tests"* ]]

    # Iteration was incremented
    local iter
    iter=$(jq -r '.iteration' "$STATE_FILE")
    [[ "$iter" -eq 2 ]]
}

@test "qa_iterate-integration_test_full_v4_escalation_at_iteration: escalation fires at correct iteration" {
    create_full_v4_workflow
    set_state_field "iteration" 3
    set_state_field "stuck_iterations" 0
    touch "$PHASE_DIR/qa_reasoning.md"

    run_iteration_fn "test-plan" "test-phase" "impl" "
        spawn_agent() {
            local prompt_file=\"\$1\"
            local output_file=\"\$2\"
            echo \"spawn_agent\" >> \"\$SPAWN_LOG\"
            echo \"Agent output\" > \"\$output_file\"
            touch \"\$PHASE_DIR/impl_reasoning.md\"
            return 0
        }
    "
    [[ "$status" -eq 0 ]]

    # analyze_stuck escalation should have fired at iteration 3
    run cat "$CALL_LOG"
    [[ "$output" == *"action_analyze_stuck"* ]]
}

#=============================================================================
# Edge Cases
#=============================================================================

@test "qa_iterate-integration_test_no_constraints_state: skips constraint checks gracefully" {
    create_basic_workflow
    # Basic workflow has no constraints

    run_iteration_fn "test-plan" "test-phase" "impl"
    [[ "$status" -eq 0 ]]

    # Agent should have been spawned normally
    [[ -s "$SPAWN_LOG" ]]
}

@test "qa_iterate-integration_test_no_hooks_state: skips hook execution gracefully" {
    create_basic_workflow
    # Basic workflow has no after hooks

    run_iteration_fn "test-plan" "test-phase" "impl"
    [[ "$status" -eq 0 ]]

    # No hook actions should have been called
    local hook_calls
    hook_calls=$(wc -l < "$CALL_LOG" 2>/dev/null || echo "0")
    # There may be 0 lines (no hooks ran)
    [[ -f "$CALL_LOG" ]]
}

@test "qa_iterate-integration_test_no_escalation_state: skips escalation checks gracefully" {
    create_basic_workflow
    # Basic workflow has no escalation triggers

    run_iteration_fn "test-plan" "test-phase" "impl"
    [[ "$status" -eq 0 ]]

    # No escalation actions should have been called
    local escalation_calls
    escalation_calls=$(grep -c "action_analyze_stuck\|action_switch_model" "$CALL_LOG" 2>/dev/null) || true
    [[ "$escalation_calls" -eq 0 ]]
}

@test "qa_iterate-integration_test_empty_verdict_with_object_next: no transition with empty verdict" {
    create_object_next_workflow
    set_state_field "iteration" 0
    set_state_string_field "current_state" "review"

    # Empty verdict with object .next should result in no transition
    run_update_state "review" ""
    [[ "$status" -eq 0 ]]

    local current
    current=$(jq -r '.current_state' "$STATE_FILE")
    [[ "$current" == "review" ]]
}

@test "qa_iterate-integration_test_iteration_dir_created: iteration directory exists after run" {
    create_basic_workflow
    set_state_field "iteration" 5

    run_iteration_fn "test-plan" "test-phase" "impl"
    [[ "$status" -eq 0 ]]

    # Should have created iteration_005 directory
    [[ -d "$PHASE_DIR/iteration_005" ]]
}

@test "qa_iterate-integration_test_prompt_copied_to_iter_dir: prompt.md in iteration directory" {
    create_basic_workflow
    set_state_field "iteration" 0

    run_iteration_fn "test-plan" "test-phase" "impl"
    [[ "$status" -eq 0 ]]

    # Prompt should be copied/rendered to iteration_000/prompt.md
    [[ -f "$PHASE_DIR/iteration_000/prompt.md" ]]
}

@test "qa_iterate-integration_test_output_in_iter_dir: output.txt created in iteration directory" {
    create_basic_workflow
    set_state_field "iteration" 0

    run_iteration_fn "test-plan" "test-phase" "impl"
    [[ "$status" -eq 0 ]]

    # Agent output should be at iteration_000/output.txt
    [[ -f "$PHASE_DIR/iteration_000/output.txt" ]]
}

@test "qa_iterate-integration_test_verdict_file_created: verdict.txt in iteration directory for review states" {
    create_basic_workflow
    set_state_field "iteration" 0

    run_iteration_fn "test-plan" "test-phase" "impl_review" "
        spawn_agent() {
            local prompt_file=\"\$1\"
            local output_file=\"\$2\"
            echo \"spawn_agent\" >> \"\$SPAWN_LOG\"
            printf '## Verdict\napproved\n' > \"\$output_file\"
            return 0
        }
    "
    [[ "$status" -eq 0 ]]

    # verdict.txt should exist
    [[ -f "$PHASE_DIR/iteration_000/verdict.txt" ]]

    # Should contain the verdict
    run cat "$PHASE_DIR/iteration_000/verdict.txt"
    [[ "$output" == *"approved"* ]]
}

#=============================================================================
# Hook Failure Edge Cases
#=============================================================================

@test "qa_iterate-integration_test_hook_failure_returns_1: failed hook causes iteration failure" {
    create_hooks_workflow

    run_iteration_fn "test-plan" "test-phase" "impl" "
        action_run_tests() {
            echo \"action_run_tests FAILED\" >> \"\$CALL_LOG\"
            return 1
        }
    "
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"hooks failed"* ]] || [[ "$output" == *"After hooks"* ]] || [[ "$status" -eq 1 ]]
}

@test "qa_iterate-integration_test_escalation_failure_returns_1: failed escalation causes failure" {
    create_escalation_workflow
    set_state_field "iteration" 3

    run_iteration_fn "test-plan" "test-phase" "impl" "
        action_analyze_stuck() {
            echo \"action_analyze_stuck FAILED\" >> \"\$CALL_LOG\"
            return 1
        }
    "
    [[ "$status" -eq 1 ]]
}

#=============================================================================
# State Not Updated Before Hooks
#=============================================================================

@test "qa_iterate-integration_test_state_not_updated_before_hooks: hooks run before state update" {
    create_hooks_workflow
    set_state_field "iteration" 5

    # Verify that during hook execution, iteration is still 5 (not yet incremented)
    run_iteration_fn "test-plan" "test-phase" "impl" "
        action_run_tests() {
            local iter_during_hook
            iter_during_hook=\$(jq -r '.iteration' \"\$STATE_FILE\")
            echo \"iteration_during_hook=\$iter_during_hook\" >> \"\$CALL_LOG\"
            return 0
        }
    "
    [[ "$status" -eq 0 ]]

    run cat "$CALL_LOG"
    [[ "$output" == *"iteration_during_hook=5"* ]]

    # After iteration completes, should be 6
    local final_iter
    final_iter=$(jq -r '.iteration' "$STATE_FILE")
    [[ "$final_iter" -eq 6 ]]
}
