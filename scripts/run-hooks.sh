#!/usr/bin/env bash
#
# Hook Execution for V4 Workflows
#
# Executes after-hooks defined in workflow.yaml states.
# Hooks can be conditional based on verdicts.
#
# Usage: Source this file along with actions.sh, then call:
#   source actions.sh
#   source run-hooks.sh
#   run_after_hooks "$WORKFLOW_FILE" "$STATE_NAME" "$VERDICT"
#
# Required environment variables:
#   PHASE_DIR        - Path to current phase directory
#   STATE_FILE       - Path to state.json
#   ARC_HOME         - Path to arc installation directory
#   ARC_DEFAULT_PKG  - Default package name (for run_tests action)

# NOTE: We use `set -uo pipefail` (no -e) because functions must handle
# non-zero exit codes from action functions without terminating.
set -uo pipefail

# run_after_hooks <workflow_file> <state_name> <verdict>
#
# Execute all after-hooks for a state.
#
# Arguments:
#   workflow_file  Path to workflow.yaml
#   state_name     Current state name (e.g., "impl")
#   verdict        Current verdict (e.g., "approved", "needs_fix", or empty)
#
# Exit codes:
#   0  All hooks executed successfully (or skipped due to conditions)
#   1  At least one hook failed
run_after_hooks() {
    local workflow_file="$1"
    local state_name="$2"
    local verdict="${3:-}"

    local hooks
    hooks=$(get_after_hooks "$workflow_file" "$state_name")

    if [[ "$hooks" == "[]" || -z "$hooks" ]]; then
        return 0
    fi

    local failed=0

    local hook_count
    hook_count=$(echo "$hooks" | jq 'length')

    for ((i=0; i<hook_count; i++)); do
        local hook
        hook=$(echo "$hooks" | jq -c ".[$i]")

        if ! execute_hook "$hook" "$verdict"; then
            failed=1
            local continue_on_error
            continue_on_error=$(echo "$hook" | jq -r '.continue_on_error // false')
            if [[ "$continue_on_error" != "true" ]]; then
                break
            fi
        fi
    done

    return $failed
}

# get_after_hooks <workflow_file> <state_name>
#
# Extract after-hooks array for a state.
#
# Arguments:
#   workflow_file  Path to workflow.yaml
#   state_name     State name
#
# Output (stdout):
#   JSON array of hook objects, or "[]" if none
get_after_hooks() {
    local workflow_file="$1"
    local state_name="$2"

    yq -o=json ".states[] | select(.name == \"$state_name\") | .after // []" "$workflow_file"
}

# execute_hook <hook_json> <verdict>
#
# Execute a single hook.
#
# Arguments:
#   hook_json  JSON object with action, when, params
#   verdict    Current verdict for condition checking
#
# Exit codes:
#   0  Hook executed successfully or skipped
#   1  Hook failed
execute_hook() {
    local hook_json="$1"
    local verdict="$2"

    local action
    action=$(echo "$hook_json" | jq -r '.action')

    local when
    when=$(echo "$hook_json" | jq -r '.when // empty')

    local params
    params=$(echo "$hook_json" | jq -c '.params // {}')

    if [[ -n "$when" ]]; then
        if ! check_when_condition "$when" "$verdict"; then
            return 0
        fi
    fi

    local args=()
    local val

    case "$action" in
        switch_model)
            val="$(echo "$params" | jq -r '.model // empty')"
            [[ -n "$val" ]] && args+=("$val")
            ;;
        run_tests)
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

    if ! "action_$action" "${args[@]}"; then
        echo "ERROR: Hook action '$action' failed" >&2
        return 1
    fi

    return 0
}

# check_when_condition <when_value> <verdict>
#
# Check if hook's when condition matches verdict.
#
# Arguments:
#   when_value  Value from hook's "when" field (or empty)
#   verdict     Current verdict
#
# Exit codes:
#   0  Condition matches (hook should run)
#   1  Condition does not match (hook should skip)
check_when_condition() {
    local when_value="$1"
    local verdict="$2"

    if [[ -z "$when_value" ]]; then
        return 0
    fi

    if [[ "$when_value" == "!"* ]]; then
        local negated="${when_value:1}"
        if [[ "$verdict" != "$negated" ]]; then
            return 0
        else
            return 1
        fi
    fi

    if [[ "$when_value" == *"|"* ]]; then
        IFS='|' read -ra options <<< "$when_value"
        for opt in "${options[@]}"; do
            if [[ "$verdict" == "$opt" ]]; then
                return 0
            fi
        done
        return 1
    fi

    if [[ "$verdict" == "$when_value" ]]; then
        return 0
    fi

    return 1
}
