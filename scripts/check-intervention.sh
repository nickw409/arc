#!/usr/bin/env bash
#
# Intervention Trigger Handling for V4 Workflows
#
# Evaluates global intervention triggers that request human help
# when certain conditions are met.
#
# Usage: Source this file along with actions.sh, then call:
#   source actions.sh
#   source check-intervention.sh
#   check_intervention_triggers "$WORKFLOW_FILE"
#
# Required environment variables:
#   STATE_FILE   - Path to state.json
#   PHASE_DIR    - Path to current phase directory

# NOTE: We use `set -uo pipefail` (no -e) because functions must handle
# non-zero exit codes from evaluate_condition and action_request_human.
set -uo pipefail

# check_intervention_triggers <workflow_file>
#
# Check all intervention triggers and request human help if any match.
#
# Arguments:
#   workflow_file  Path to workflow.yaml
#
# Exit codes:
#   0  No intervention needed
#   2  Intervention requested (special exit code to halt execution)
#
# Behavior:
#   - Evaluates each intervention_triggers condition
#   - If any condition is true, calls action_request_human
#   - Returns 2 to signal orchestrator to pause
check_intervention_triggers() {
    local workflow_file="$1"

    # Guard: validate STATE_FILE is set and exists
    if [[ -z "${STATE_FILE:-}" ]]; then
        echo "ERROR: STATE_FILE not set" >&2
        return 1
    fi
    if [[ ! -f "$STATE_FILE" ]]; then
        echo "ERROR: STATE_FILE not found: $STATE_FILE" >&2
        return 1
    fi

    # Check if intervention already requested (intervention_request is object, null when not set)
    local already_requested
    already_requested=$(jq -r '.intervention_request != null' "$STATE_FILE")
    if [[ "$already_requested" == "true" ]]; then
        # Already waiting for human, don't re-trigger
        return 2
    fi

    # Get intervention triggers
    local triggers
    triggers=$(get_intervention_triggers "$workflow_file")

    # If no triggers, succeed
    if [[ "$triggers" == "[]" || -z "$triggers" ]]; then
        return 0
    fi

    # Read current state
    local state_json
    state_json=$(cat "$STATE_FILE")

    # Check each trigger
    local trigger_count
    trigger_count=$(echo "$triggers" | jq 'length')

    for ((i=0; i<trigger_count; i++)); do
        local trigger
        trigger=$(echo "$triggers" | jq -c ".[$i]")

        local condition
        condition=$(echo "$trigger" | jq -r '.condition')

        if evaluate_condition "$condition" "$state_json"; then
            # Condition met, request intervention
            local message
            message=$(echo "$trigger" | jq -r '.message // "Human intervention required"')

            action_request_human "$message"

            echo "INTERVENTION: $message" >&2
            return 2
        fi
    done

    return 0
}

# get_intervention_triggers <workflow_file>
#
# Extract intervention_triggers array from workflow.
#
# Arguments:
#   workflow_file  Path to workflow.yaml
#
# Output (stdout):
#   JSON array of intervention trigger objects, or "[]" if none
get_intervention_triggers() {
    local workflow_file="$1"

    # Extract intervention_triggers array, default to empty array
    yq -o=json '.intervention_triggers // []' "$workflow_file"
}

# evaluate_condition <condition> <state_json>
#
# Evaluate a condition expression against state.
#
# Arguments:
#   condition   Condition string (e.g., "stuck_iterations >= 5")
#   state_json  JSON string of current state
#
# Exit codes:
#   0  Condition is true
#   1  Condition is false
#
# Supported operators:
#   ==, !=        — String comparison (works for both strings and numbers)
#   >, <, >=, <=  — Integer comparison (bash -gt/-lt/-ge/-le, operands MUST be integers)
evaluate_condition() {
    local condition="$1"
    local state_json="$2"

    # Parse condition into parts
    local left_op operator right_op
    read -r left_op operator right_op < <(parse_condition "$condition")

    # Get left value from state
    local left_value
    left_value=$(get_state_value "$state_json" "$left_op")

    # Resolve right operand: numeric/boolean literals used directly,
    # otherwise look up as variable from state
    local right_value
    if [[ "$right_op" =~ ^-?[0-9]+$ ]]; then
        # Numeric literal (including negative)
        right_value="$right_op"
    elif [[ "$right_op" == "true" || "$right_op" == "false" ]]; then
        # Boolean literal
        right_value="$right_op"
    else
        # Variable reference — look up from state
        right_value=$(get_state_value "$state_json" "$right_op")
        if [[ -z "$right_value" && -n "$left_value" ]]; then
            # Right operand not found in state, but left operand IS present —
            # use raw operand text so numeric validation catches non-integer literals
            right_value="$right_op"
        fi
    fi

    # Left side is always a state variable; default missing to "0"
    left_value="${left_value:-0}"

    # Evaluate based on operator
    # For string operators (==, !=): missing right-side variables fall back to the literal operand text
    # For numeric operators (>, <, >=, <=): missing values default to "0"
    case "$operator" in
        "==")
            local rv="${right_value:-$right_op}"
            [[ "$left_value" == "$rv" ]] && return 0 || return 1
            ;;
        "!=")
            local rv="${right_value:-$right_op}"
            [[ "$left_value" != "$rv" ]] && return 0 || return 1
            ;;
        ">"|"<"|">="|"<=")
            # Default missing values to "0" for numeric comparison
            left_value="${left_value:-0}"
            right_value="${right_value:-0}"
            # Validate both operands are integers before comparing
            if ! [[ "$left_value" =~ ^-?[0-9]+$ ]]; then
                echo "ERROR: Non-integer left operand '$left_value' for operator '$operator'" >&2
                return 1
            fi
            if ! [[ "$right_value" =~ ^-?[0-9]+$ ]]; then
                echo "ERROR: Non-integer right operand '$right_value' for operator '$operator'" >&2
                return 1
            fi
            # Perform the integer comparison
            case "$operator" in
                ">")  [[ "$left_value" -gt "$right_value" ]] && return 0 || return 1 ;;
                "<")  [[ "$left_value" -lt "$right_value" ]] && return 0 || return 1 ;;
                ">=") [[ "$left_value" -ge "$right_value" ]] && return 0 || return 1 ;;
                "<=") [[ "$left_value" -le "$right_value" ]] && return 0 || return 1 ;;
            esac
            ;;
        *)
            echo "ERROR: Unknown operator '$operator'" >&2
            return 1
            ;;
    esac
}

# parse_condition <condition>
#
# Parse a condition into left operand, operator, right operand.
#
# Arguments:
#   condition  Condition string
#
# Output (stdout):
#   Single space-separated line: left_operand operator right_operand
parse_condition() {
    local condition="$1"

    # Remove extra whitespace
    condition=$(echo "$condition" | tr -s ' ')

    # Match pattern: left_op operator right_op
    # Operators: ==, !=, >=, <=, >, <
    # Variable names: alphabetic + underscores + digits (must start with alpha/underscore)
    # Right side: variable name, numeric literal (including negative), or boolean
    if [[ "$condition" =~ ^([a-zA-Z_][a-zA-Z0-9_]*)[[:space:]]*(==|!=|>=|<=|>|<)[[:space:]]*(.+)$ ]]; then
        echo "${BASH_REMATCH[1]} ${BASH_REMATCH[2]} ${BASH_REMATCH[3]}"
    else
        echo "ERROR: Cannot parse condition: $condition" >&2
        return 1
    fi
}

# get_state_value <state_json> <variable_name>
#
# Get a value from state JSON by variable name.
#
# Arguments:
#   state_json     JSON string of state
#   variable_name  Name of variable to extract
#
# Output (stdout):
#   Value of variable, or empty string if not found
get_state_value() {
    local state_json="$1"
    local variable_name="$2"

    echo "$state_json" | jq -r --arg var "$variable_name" '.[$var] // empty'
}
