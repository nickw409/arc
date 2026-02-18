#!/usr/bin/env bash
#
# Escalation Trigger Handling for V4 Workflows
#
# Evaluates escalation triggers based on iteration count and
# executes corresponding actions.
#
# Usage: Source this file along with actions.sh, then call:
#   source actions.sh
#   source check-escalation.sh
#   check_escalation "$WORKFLOW_FILE" "$STATE_NAME"
#
# Required environment variables:
#   STATE_FILE   - Path to state.json (for reading iteration)
#   PHASE_DIR    - Path to current phase directory
#   ARC_HOME - Path to arc installation directory

# NOTE: We use `set -uo pipefail` (no -e) because functions like check_escalation
# must handle non-zero exit codes from action functions without terminating.
set -uo pipefail

# check_escalation <workflow_file> <state_name>
#
# Check and execute escalation triggers for current iteration.
#
# Arguments:
#   workflow_file  Path to workflow.yaml
#   state_name     Current state name (e.g., "impl")
#
# Exit codes:
#   0  No escalation triggered, or escalation action succeeded
#   1  Escalation action failed
#
# Behavior:
#   - Reads current iteration from STATE_FILE
#   - Finds escalation trigger matching current iteration
#   - Executes corresponding action if found
#   - Only executes FIRST matching trigger (no chaining)
check_escalation() {
    local workflow_file="$1"
    local state_name="$2"

    # Guard: validate STATE_FILE is set
    if [[ -z "${STATE_FILE:-}" ]]; then
        echo "ERROR: STATE_FILE not set" >&2
        return 1
    fi

    # Get current iteration from state
    local iteration
    iteration=$(jq -r '.iteration // 0' "$STATE_FILE")

    # Get escalation triggers for this state
    local triggers
    triggers=$(get_escalation_triggers "$workflow_file" "$state_name")

    # If no triggers, succeed immediately
    if [[ "$triggers" == "[]" || -z "$triggers" ]]; then
        return 0
    fi

    # Find matching trigger
    local matching
    matching=$(find_matching_trigger "$triggers" "$iteration")

    if [[ "$matching" == "null" || -z "$matching" ]]; then
        return 0  # No trigger matched
    fi

    # Check if after_iteration trigger was already executed
    local trigger_type
    trigger_type=$(echo "$matching" | jq -r 'if .after_iteration then "after" else "at" end')

    if [[ "$trigger_type" == "after" ]]; then
        local trigger_id
        trigger_id=$(echo "$matching" | jq -r '.after_iteration')
        if was_trigger_executed "$state_name" "after_$trigger_id"; then
            return 0  # Already executed
        fi
        mark_trigger_executed "$state_name" "after_$trigger_id"
    fi

    # Execute the escalation action
    execute_escalation "$matching"
}

# get_escalation_triggers <workflow_file> <state_name>
#
# Extract escalation triggers array for a state.
#
# Arguments:
#   workflow_file  Path to workflow.yaml
#   state_name     State name
#
# Output (stdout):
#   JSON array of escalation objects, or "[]" if none
get_escalation_triggers() {
    local workflow_file="$1"
    local state_name="$2"

    # Extract escalation array, default to empty array
    yq -o=json ".states[] | select(.name == \"$state_name\") | .escalation // []" "$workflow_file"
}

# find_matching_trigger <triggers_json> <iteration>
#
# Find escalation trigger matching current iteration.
#
# Arguments:
#   triggers_json  JSON array of escalation triggers
#   iteration      Current iteration number
#
# Output (stdout):
#   JSON object of matching trigger, or "null" if none
#
# Matching rules:
#   - at_iteration: N — triggers at exactly iteration N
#   - after_iteration: N — triggers at iteration > N (once)
#   - every_n_iterations: N — triggers at iterations divisible by N
find_matching_trigger() {
    local triggers_json="$1"
    local iteration="$2"

    # Run the jq filter ONCE and capture the result to avoid duplicate output
    local result
    result=$(echo "$triggers_json" | jq -c --argjson iter "$iteration" '
        .[] | select(
            (.at_iteration != null and .at_iteration == $iter) or
            (.after_iteration != null and .after_iteration < $iter) or
            (.every_n_iterations != null and ($iter > 0) and ($iter % .every_n_iterations == 0))
        )
    ' | head -1)

    # Output the result or "null" if empty
    if [[ -n "$result" ]]; then
        echo "$result"
    else
        echo "null"
    fi
}

# execute_escalation <trigger_json>
#
# Execute an escalation trigger's action.
#
# Arguments:
#   trigger_json  JSON object with action and params
#
# Exit codes:
#   0  Action succeeded
#   1  Action failed
execute_escalation() {
    local trigger_json="$1"

    # Extract action and params
    local action
    action=$(echo "$trigger_json" | jq -r '.action')

    local params
    params=$(echo "$trigger_json" | jq -c '.params // {}')

    # Build argument list from params
    local args=()

    # Special handling for known actions with defined parameter order
    # IMPORTANT: All $() expansions are double-quoted to prevent word-splitting
    # on values containing spaces (e.g., commit messages, human intervention messages)
    local val
    case "$action" in
        switch_model)
            val="$(echo "$params" | jq -r '.model // empty')"
            [[ -n "$val" ]] && args+=("$val")
            ;;
        run_tests)
            # IMPORTANT: action_run_tests takes positional args (pattern, save_to, expect_failure).
            # All 3 must always be passed in order — skipping save_to would shift expect_failure
            # into the save_to position. Use defaults for missing params.
            args+=("$(echo "$params" | jq -r '.pattern // "test"')")
            args+=("$(echo "$params" | jq -r '.save_to // "test_output.txt"')")
            args+=("$(echo "$params" | jq -r '.expect_failure // "false"')")
            ;;
        request_human)
            val="$(echo "$params" | jq -r '.message // empty')"
            [[ -n "$val" ]] && args+=("$val")
            ;;
        commit)
            val="$(echo "$params" | jq -r '.message // empty')"
            [[ -n "$val" ]] && args+=("$val")
            val="$(echo "$params" | jq -r '.when // empty')"
            [[ -n "$val" ]] && args+=("$val")
            ;;
        analyze_stuck)
            # No parameters
            ;;
        script)
            val="$(echo "$params" | jq -r '.path // empty')"
            [[ -n "$val" ]] && args+=("$val")
            ;;
        *)
            echo "ERROR: Unknown action '$action'" >&2
            return 1
            ;;
    esac

    # Log escalation
    echo "ESCALATION: Executing $action at iteration $(jq -r '.iteration' "$STATE_FILE")" >&2

    # Call the action function
    if ! "action_$action" "${args[@]}"; then
        echo "ERROR: Escalation action '$action' failed" >&2
        return 1
    fi

    return 0
}

# was_trigger_executed <state_name> <trigger_id>
#
# Check if a trigger was already executed (for after_iteration).
#
# Arguments:
#   state_name   State name (reserved for future per-state tracking, currently unused)
#   trigger_id   Unique trigger identifier
#
# Exit codes:
#   0  Trigger was already executed
#   1  Trigger not yet executed
was_trigger_executed() {
    local state_name="$1"
    local trigger_id="$2"

    # Check if trigger_id is in executed_escalations array
    local executed
    executed=$(jq -r --arg id "$trigger_id" '.executed_escalations // [] | index($id)' "$STATE_FILE")

    if [[ "$executed" != "null" ]]; then
        return 0  # Already executed
    fi
    return 1
}

# mark_trigger_executed <state_name> <trigger_id>
#
# Mark a trigger as executed in state.json.
#
# Arguments:
#   state_name   State name (reserved for future per-state tracking, currently unused)
#   trigger_id   Unique trigger identifier
mark_trigger_executed() {
    local state_name="$1"
    local trigger_id="$2"

    # Add trigger_id to executed_escalations array (use $$ PID to prevent race conditions)
    jq --arg id "$trigger_id" \
       '.executed_escalations = ((.executed_escalations // []) + [$id] | unique)' \
       "$STATE_FILE" > "${STATE_FILE}.tmp.$$" && mv "${STATE_FILE}.tmp.$$" "$STATE_FILE"
}
