#!/usr/bin/env bash
# Script: validate-workflow.sh
# Purpose: Validate workflow YAML files (basic syntax + required fields for bootstrap)

set -euo pipefail

# Constants
ARC_SCRIPTS_DIR="${ARC_SCRIPTS_DIR:-$ARC_HOME/scripts}"

# Error handling
error() {
    echo "ERROR: $*" >&2
    exit 1
}

warn() {
    echo "WARNING: $*" >&2
}

# Validate required commands
require_command() {
    command -v "$1" &> /dev/null || error "$1 is required but not installed"
}

require_command yq
require_command jq

# ==============================================================================
# Helper: Get all next states for a given state name (handles V1 and V2)
# Args: $1 = state_name
# Returns: space-separated list of target state names via stdout (empty string for terminals)
# Uses global: $WORKFLOW_FILE
# ==============================================================================
get_next_states() {
    local state_name="$1"
    local idx next_tag

    # Find state index
    idx=$(yq ".states | to_entries | .[] | select(.value.name == \"$state_name\") | .key" "$WORKFLOW_FILE")
    [[ -z "$idx" ]] && return

    # Check if V1 or V2
    next_tag=$(yq ".states[$idx].next | tag" "$WORKFLOW_FILE")
    if [[ "$next_tag" == "!!map" ]]; then
        # V2: extract all branch targets (space-separated)
        yq ".states[$idx].next | .[]" "$WORKFLOW_FILE" | tr '\n' ' '
    elif [[ "$next_tag" == "!!str" ]]; then
        # V1: single string target
        yq ".states[$idx].next" "$WORKFLOW_FILE"
    fi
    # !!null means no next (terminal), return empty
}

# ==============================================================================
# Get the workflow version (1, 2, etc.)
# Args: none (uses global $WORKFLOW_FILE)
# Returns: version number via stdout (1 or 2)
# ==============================================================================
get_workflow_version() {
    yq '.version // 1' "$WORKFLOW_FILE"
}

# ==============================================================================
# Check if a verdict name is valid (alphanumeric with underscores, starts with letter)
# Args: $1 = verdict name
# Returns: 0 if valid, 1 if invalid
# ==============================================================================
is_valid_verdict_name() {
    local name="$1"
    [[ "$name" =~ ^[a-z][a-z0-9_]*$ ]]
}

# ==============================================================================
# Check V2 verdict consistency for states with branching transitions
# For each state with a map-type `next`:
#   1. All verdict names must match regex ^[a-z][a-z0-9_]*$
#   2. State must have `verdicts` array
#   3. All verdicts in the array must have a corresponding transition in `next`
#   4. All keys in `next` must be listed in `verdicts`
# Returns: 0 always (errors tracked via global $errors counter)
# Prints ❌ error messages for each issue found
# Uses global: $WORKFLOW_FILE, $errors (incremented on errors)
# ==============================================================================
check_verdict_consistency() {
    local version
    version=$(get_workflow_version)

    # Skip for V1 workflows
    if [[ "$version" == "1" ]]; then
        return 0
    fi

    local state_count terminal_states_str
    state_count=$(yq '.states | length' "$WORKFLOW_FILE")
    # Build space-padded string for reliable substring matching: " state1 state2 state3 "
    # Leading space comes from the " $(...)" prefix, trailing space from tr '\n' ' '
    terminal_states_str=" $(yq '.terminal_states[]' "$WORKFLOW_FILE" 2>/dev/null | tr '\n' ' ')"
    # Result example: " complete blocked " - matching " state_name " finds exact matches

    for ((i=0; i<state_count; i++)); do
        local state_name next_tag has_verdicts
        state_name=$(yq ".states[$i].name" "$WORKFLOW_FILE")
        next_tag=$(yq ".states[$i].next | tag" "$WORKFLOW_FILE")

        # Skip terminal states (check against terminal_states list)
        # terminal_states_str is " complete blocked " so " state " matches reliably
        if [[ "${terminal_states_str}" == *" ${state_name} "* ]]; then
            continue
        fi

        # Skip states with no next field or null next (also terminal)
        if [[ "$next_tag" == "!!null" ]]; then
            continue
        fi

        # Check if this is a V2 state (map-type next)
        if [[ "$next_tag" == "!!map" ]]; then
            has_verdicts=$(yq ".states[$i].verdicts | length" "$WORKFLOW_FILE")

            if [[ "$has_verdicts" == "0" || "$has_verdicts" == "null" ]]; then
                echo "❌ State '$state_name' has conditional transitions but no verdicts defined"
                ((errors++)) || true
                continue
            fi

            # Check each verdict name is valid and has a transition
            local verdict_count duplicate_check
            verdict_count=$(yq ".states[$i].verdicts | length" "$WORKFLOW_FILE")
            duplicate_check=""
            for ((j=0; j<verdict_count; j++)); do
                local v
                v=$(yq ".states[$i].verdicts[$j]" "$WORKFLOW_FILE")
                # Check verdict name format
                if ! is_valid_verdict_name "$v"; then
                    echo "❌ Invalid verdict name '$v': must be alphanumeric with underscores only"
                    ((errors++)) || true
                    continue
                fi

                # Check for duplicate verdicts (warning only)
                if [[ " $duplicate_check " == *" $v "* ]]; then
                    warn "Duplicate verdict '$v' in state '$state_name'"
                else
                    duplicate_check="$duplicate_check $v"
                fi

                # Check verdict has transition
                local has_transition
                has_transition=$(yq ".states[$i].next[\"$v\"]" "$WORKFLOW_FILE")
                if [[ "$has_transition" == "null" ]]; then
                    echo "❌ State '$state_name' declares verdict '$v' but has no transition for it"
                    ((errors++)) || true
                fi
            done

            # Check each transition key is in verdicts
            local transitions
            transitions=$(yq ".states[$i].next | keys | .[]" "$WORKFLOW_FILE")
            for t in $transitions; do
                local in_verdicts
                in_verdicts=$(yq ".states[$i].verdicts | contains([\"$t\"])" "$WORKFLOW_FILE")
                if [[ "$in_verdicts" != "true" ]]; then
                    echo "❌ State '$state_name' has transition for '$t' but it's not in verdicts list"
                    ((errors++)) || true
                fi
            done
        fi
    done

    return 0
}

# ==============================================================================
# Check all state names are unique
# Returns: 0 on success, 1 if duplicates found
# ==============================================================================
check_unique_state_names() {
    local all_names sorted_names duplicates
    all_names=$(yq '.states[].name' "$WORKFLOW_FILE")
    sorted_names=$(echo "$all_names" | sort)
    duplicates=$(echo "$sorted_names" | uniq -d)

    local check_errors=0
    if [[ -n "$duplicates" ]]; then
        while IFS= read -r dup; do
            [[ -n "$dup" ]] && echo "❌ Duplicate state name: $dup" && { ((check_errors++)) || true; }
        done <<< "$duplicates"
    fi

    [[ $check_errors -eq 0 ]] && echo "✓ All state names unique"
    [[ $check_errors -eq 0 ]]
}

# ==============================================================================
# Check all prompt files exist on disk
# Resolves paths as: "$ARC_HOME/$prompt_path"
# Returns: 0 on success, 1 if any missing
# ==============================================================================
check_prompt_files_exist() {
    local check_errors=0
    local prompt_path full_path
    local count
    count=$(yq '.states | length' "$WORKFLOW_FILE")

    for i in $(seq 0 $((count - 1))); do
        prompt_path=$(yq ".states[$i].prompt // \"\"" "$WORKFLOW_FILE")
        if [[ -n "$prompt_path" ]]; then
            full_path="$ARC_HOME/$prompt_path"
            if [[ -f "$full_path" ]]; then
                echo "✓ Prompt exists: $prompt_path"
            else
                echo "❌ Prompt not found: $prompt_path"
                ((check_errors++))
            fi
        fi
    done

    [[ $check_errors -eq 0 ]]
}

# ==============================================================================
# Check all next values point to existing states
# Returns: 0 on success, 1 if any invalid
# ==============================================================================
check_next_references_valid() {
    local check_errors=0
    local all_states state_name targets target next_tag
    local count terminal_states_list

    all_states=$(yq '.states[].name' "$WORKFLOW_FILE" | tr '\n' ' ')
    terminal_states_list=$(yq '.terminal_states[]' "$WORKFLOW_FILE" 2>/dev/null | tr '\n' ' ')
    count=$(yq '.states | length' "$WORKFLOW_FILE")

    for i in $(seq 0 $((count - 1))); do
        state_name=$(yq ".states[$i].name // \"\"" "$WORKFLOW_FILE")
        [[ -z "$state_name" ]] && continue

        # Check if terminal state has next field (warn but don't fail)
        if echo " $terminal_states_list " | grep -q " $state_name "; then
            next_tag=$(yq ".states[$i].next | tag" "$WORKFLOW_FILE")
            if [[ "$next_tag" != "!!null" ]]; then
                warn "Terminal state '$state_name' has next field (ignored)"
            fi
            continue
        fi

        # Get next states for this state
        targets=$(get_next_states "$state_name")

        for target in $targets; do
            if echo " $all_states " | grep -q " $target "; then
                echo "✓ Transition valid: $state_name -> $target"
            else
                echo "❌ Invalid transition: $state_name -> $target"
                ((check_errors++))
            fi
        done
    done

    [[ $check_errors -eq 0 ]]
}

# ==============================================================================
# Check no unreachable states (all states reachable from entry)
# Uses BFS from entry_state
# Returns: 0 on success, 1 if any unreachable
# ==============================================================================
check_no_unreachable_states() {
    local all_states visited queue current next_states
    all_states=$(yq '.states[].name' "$WORKFLOW_FILE" | tr '\n' ' ')
    visited=""
    queue="$entry_state"

    while [[ -n "$queue" ]]; do
        current="${queue%% *}"
        queue="${queue#* }"
        [[ "$queue" == "$current" ]] && queue=""

        # Skip if visited
        echo " $visited " | grep -q " $current " && continue
        visited="$visited $current"

        # Get next states (handles V1 string and V2 map)
        next_states=$(get_next_states "$current")
        [[ -n "$next_states" ]] && queue="$queue $next_states"
    done

    # Check all states were visited (excluding terminal states - they may not always be reachable)
    local check_errors=0
    local terminal_states_list
    terminal_states_list=$(yq '.terminal_states[]' "$WORKFLOW_FILE" 2>/dev/null | tr '\n' ' ')
    for state in $all_states; do
        # Skip terminal states - they are endpoints that may not always be reachable
        echo " $terminal_states_list " | grep -q " $state " && continue
        if ! echo " $visited " | grep -q " $state "; then
            echo "❌ Unreachable state: $state"
            ((check_errors++)) || true
        fi
    done

    [[ $check_errors -eq 0 ]] && echo "✓ All states reachable from entry"
    [[ $check_errors -eq 0 ]]
}

# ==============================================================================
# Check all non-terminal states can reach a terminal
# Uses forward propagation
# Returns: 0 on success, 1 if any cannot reach terminal
# ==============================================================================
check_all_reach_terminal() {
    local all_states terminal_states_list can_reach
    all_states=$(yq '.states[].name' "$WORKFLOW_FILE" | tr '\n' ' ')
    terminal_states_list=$(yq '.terminal_states[]' "$WORKFLOW_FILE" 2>/dev/null | tr '\n' ' ')

    # States that can reach terminal (start with terminals themselves)
    can_reach="$terminal_states_list"

    # Forward propagation: iteratively add states that point to states in can_reach
    local changed=true
    while $changed; do
        changed=false
        for state in $all_states; do
            # Skip if already known to reach terminal
            echo " $can_reach " | grep -q " $state " && continue

            # Check if this state points to any state that can reach terminal
            local targets
            targets=$(get_next_states "$state")
            for target in $targets; do
                if echo " $can_reach " | grep -q " $target "; then
                    can_reach="$can_reach $state"
                    changed=true
                    break
                fi
            done
        done
    done

    # Check all non-terminal states can reach terminal
    local check_errors=0
    for state in $all_states; do
        # Skip terminal states (they trivially reach terminal)
        echo " $terminal_states_list " | grep -q " $state " && continue

        if ! echo " $can_reach " | grep -q " $state "; then
            echo "❌ State cannot reach terminal: $state"
            ((check_errors++))
        fi
    done

    [[ $check_errors -eq 0 ]] && echo "✓ All states can reach terminal"
    [[ $check_errors -eq 0 ]]
}

# ==============================================================================
# Check entry state is not a terminal state
# Returns: 0 on success, 1 if entry is terminal
# ==============================================================================
check_entry_not_terminal() {
    local terminal_states_list
    terminal_states_list=$(yq '.terminal_states[]' "$WORKFLOW_FILE" 2>/dev/null | tr '\n' ' ')

    if echo " $terminal_states_list " | grep -q " $entry_state "; then
        echo "❌ Entry state '$entry_state' cannot be terminal"
        return 1
    fi

    echo "✓ Entry state is non-terminal"
    return 0
}

# ==============================================================================
# V3 Validation: Validate defaults section
# Checks max_iterations and timeout are positive integers if present
# Returns: 0 if valid, 1 if invalid
# Uses global: $WORKFLOW_FILE, $errors (incremented on errors)
# ==============================================================================
validate_defaults() {
    local workflow_file="$1"
    local local_errors=0

    # Check if defaults section exists
    local has_defaults
    has_defaults=$(yq '.defaults | tag' "$workflow_file")
    if [[ "$has_defaults" == "!!null" ]]; then
        return 0
    fi

    # Check max_iterations if present
    local max_iter
    max_iter=$(yq '.defaults.max_iterations' "$workflow_file")
    if [[ "$max_iter" != "null" ]]; then
        if ! [[ "$max_iter" =~ ^[1-9][0-9]*$ ]]; then
            echo "❌ max_iterations must be a positive integer, got '$max_iter'"
            ((local_errors++)) || true
        fi
    fi

    # Check timeout if present
    local timeout_val
    timeout_val=$(yq '.defaults.timeout' "$workflow_file")
    if [[ "$timeout_val" != "null" ]]; then
        if ! [[ "$timeout_val" =~ ^[1-9][0-9]*$ ]]; then
            echo "❌ timeout must be a positive integer, got '$timeout_val'"
            ((local_errors++)) || true
        fi
    fi

    return $local_errors
}

# ==============================================================================
# V3 Validation: Validate variables section
# Checks all keys are valid identifiers
# Returns: 0 if valid, 1 if invalid
# Uses global: $WORKFLOW_FILE
# ==============================================================================
validate_variables() {
    local workflow_file="$1"
    local local_errors=0

    # Check if variables section exists
    local has_variables
    has_variables=$(yq '.variables | tag' "$workflow_file")
    if [[ "$has_variables" == "!!null" ]]; then
        return 0
    fi

    # Get all variable keys
    local keys
    keys=$(yq '.variables | keys | .[]' "$workflow_file" 2>/dev/null)
    if [[ -z "$keys" ]]; then
        return 0
    fi

    while IFS= read -r key; do
        if ! [[ "$key" =~ ^[a-zA-Z_][a-zA-Z0-9_]*$ ]]; then
            echo "❌ Invalid variable name '$key': must match ^[a-zA-Z_][a-zA-Z0-9_]*$"
            ((local_errors++)) || true
        fi
    done <<< "$keys"

    return $local_errors
}

# ==============================================================================
# V3 Validation: Validate state-specific params
# Checks all param keys are valid identifiers
# Returns: 0 if valid, 1 if invalid
# Uses global: $WORKFLOW_FILE
# ==============================================================================
validate_state_params() {
    local workflow_file="$1"
    local local_errors=0

    local state_count
    state_count=$(yq '.states | length' "$workflow_file")

    for ((i=0; i<state_count; i++)); do
        local state_name has_params
        state_name=$(yq ".states[$i].name" "$workflow_file")
        has_params=$(yq ".states[$i].params | tag" "$workflow_file")

        if [[ "$has_params" == "!!null" ]]; then
            continue
        fi

        # Get all param keys (only top-level)
        local keys
        keys=$(yq ".states[$i].params | keys | .[]" "$workflow_file" 2>/dev/null)
        if [[ -z "$keys" ]]; then
            continue
        fi

        while IFS= read -r key; do
            if ! [[ "$key" =~ ^[a-zA-Z_][a-zA-Z0-9_]*$ ]]; then
                echo "❌ Invalid param name '$key' in state '$state_name': must match ^[a-zA-Z_][a-zA-Z0-9_]*$"
                ((local_errors++)) || true
            fi
        done <<< "$keys"
    done

    return $local_errors
}

# ==============================================================================
# V3 Validation: Check for reserved variable name conflicts
# Reserved names: state, iteration, current_state, phase, plan, plan_md, params
# Returns: 0 if no conflicts, 1 if conflicts found
# ==============================================================================
check_reserved_conflicts() {
    local workflow_file="$1"
    local local_errors=0
    local reserved_names=("state" "iteration" "current_state" "phase" "plan" "plan_md" "params")

    for reserved in "${reserved_names[@]}"; do
        # Check defaults
        local in_defaults
        in_defaults=$(yq ".defaults | has(\"$reserved\")" "$workflow_file" 2>/dev/null)
        if [[ "$in_defaults" == "true" ]]; then
            echo "❌ Reserved variable '$reserved' cannot be used in defaults"
            ((local_errors++)) || true
        fi

        # Check variables
        local in_variables
        in_variables=$(yq ".variables | has(\"$reserved\")" "$workflow_file" 2>/dev/null)
        if [[ "$in_variables" == "true" ]]; then
            echo "❌ Reserved variable '$reserved' cannot be used in variables"
            ((local_errors++)) || true
        fi

        # Check all state params
        local state_count
        state_count=$(yq '.states | length' "$workflow_file")
        for ((i=0; i<state_count; i++)); do
            local has_params
            has_params=$(yq ".states[$i].params | tag" "$workflow_file")
            if [[ "$has_params" == "!!null" ]]; then
                continue
            fi

            local in_params
            in_params=$(yq ".states[$i].params | has(\"$reserved\")" "$workflow_file" 2>/dev/null)
            if [[ "$in_params" == "true" ]]; then
                local state_name
                state_name=$(yq ".states[$i].name" "$workflow_file")
                echo "❌ Reserved variable '$reserved' cannot be used in params (state: $state_name)"
                ((local_errors++)) || true
            fi
        done
    done

    return $local_errors
}

# ==============================================================================
# V4 Validation: Get list of known action names
# Returns: Newline-separated list of action names to stdout
# ==============================================================================
get_known_actions() {
    echo "run_tests"
    echo "commit"
    echo "switch_model"
    echo "analyze_stuck"
    echo "request_human"
    echo "script"
}

# ==============================================================================
# V4 Validation: Validate constraints section for each state
# Checks max_iterations, require_artifacts_in/out
# Returns: 0 if valid, 1 if invalid
# ==============================================================================
validate_v4_constraints() {
    local workflow_file="$1"

    # Get all states with constraints
    local states_with_constraints
    states_with_constraints=$(yq -o=json '.states[] | select(.constraints != null) | .name' "$workflow_file" 2>/dev/null)

    for state_name in $states_with_constraints; do
        state_name=$(echo "$state_name" | tr -d '"')

        # Check max_iterations is positive integer
        local max_iter
        max_iter=$(yq ".states[] | select(.name == \"$state_name\") | .constraints.max_iterations // null" "$workflow_file")
        if [[ "$max_iter" != "null" && ! "$max_iter" =~ ^[1-9][0-9]*$ ]]; then
            echo "ERROR: State '$state_name' has invalid max_iterations: $max_iter (must be positive integer)" >&2
            return 1
        fi

        # Check require_artifacts_in is array
        local artifacts_in
        artifacts_in=$(yq -o=json ".states[] | select(.name == \"$state_name\") | .constraints.require_artifacts_in // null" "$workflow_file")
        if [[ "$artifacts_in" != "null" ]]; then
            local is_array
            is_array=$(echo "$artifacts_in" | jq 'type == "array"' 2>/dev/null)
            if [[ "$is_array" != "true" ]]; then
                echo "ERROR: State '$state_name' has invalid require_artifacts_in: must be array" >&2
                return 1
            fi
            # Validate all elements are strings
            local non_strings
            non_strings=$(echo "$artifacts_in" | jq '[.[] | select(type != "string")] | length' 2>/dev/null)
            if [[ "$non_strings" -gt 0 ]]; then
                echo "ERROR: State '$state_name' has invalid require_artifacts_in: all elements must be strings" >&2
                return 1
            fi
        fi

        # Check require_artifacts_out is array
        local artifacts_out
        artifacts_out=$(yq -o=json ".states[] | select(.name == \"$state_name\") | .constraints.require_artifacts_out // null" "$workflow_file")
        if [[ "$artifacts_out" != "null" ]]; then
            local is_array
            is_array=$(echo "$artifacts_out" | jq 'type == "array"' 2>/dev/null)
            if [[ "$is_array" != "true" ]]; then
                echo "ERROR: State '$state_name' has invalid require_artifacts_out: must be array" >&2
                return 1
            fi
            # Validate all elements are strings
            local non_strings
            non_strings=$(echo "$artifacts_out" | jq '[.[] | select(type != "string")] | length' 2>/dev/null)
            if [[ "$non_strings" -gt 0 ]]; then
                echo "ERROR: State '$state_name' has invalid require_artifacts_out: all elements must be strings" >&2
                return 1
            fi
        fi
    done

    return 0
}

# ==============================================================================
# V4 Validation: Validate after hooks for each state
# Checks action, when syntax, params, continue_on_error
# Returns: 0 if valid, 1 if invalid
# ==============================================================================
validate_v4_after_hooks() {
    local workflow_file="$1"
    local known_actions
    known_actions=$(get_known_actions)

    # Get all states with after hooks
    local states_with_hooks
    states_with_hooks=$(yq '.states[] | select(.after != null) | .name' "$workflow_file" 2>/dev/null | tr -d '"')

    for state_name in $states_with_hooks; do
        local hook_count
        hook_count=$(yq ".states[] | select(.name == \"$state_name\") | .after | length" "$workflow_file")

        for ((i=0; i<hook_count; i++)); do
            # Check action is non-empty
            local action
            action=$(yq ".states[] | select(.name == \"$state_name\") | .after[$i].action // \"\"" "$workflow_file" | tr -d '"')
            if [[ -z "$action" ]]; then
                echo "ERROR: State '$state_name' after hook $i missing required 'action' field" >&2
                return 1
            fi

            # Check action is known
            if ! echo "$known_actions" | grep -q "^$action$"; then
                echo "ERROR: State '$state_name' after hook $i has unknown action: $action" >&2
                return 1
            fi

            # Check params is object if present
            local params_type
            params_type=$(yq -o=json ".states[] | select(.name == \"$state_name\") | .after[$i].params // null" "$workflow_file")
            if [[ "$params_type" != "null" ]]; then
                local is_object
                is_object=$(echo "$params_type" | jq 'type == "object"' 2>/dev/null)
                if [[ "$is_object" != "true" ]]; then
                    echo "ERROR: State '$state_name' after hook $i has invalid 'params': must be object" >&2
                    return 1
                fi
            fi

            # Check continue_on_error is boolean if present
            local coe
            coe=$(yq ".states[] | select(.name == \"$state_name\") | .after[$i].continue_on_error // null" "$workflow_file")
            if [[ "$coe" != "null" && "$coe" != "true" && "$coe" != "false" ]]; then
                echo "ERROR: State '$state_name' after hook $i has invalid 'continue_on_error': must be boolean (true/false)" >&2
                return 1
            fi

            # Check when syntax if present
            local when
            when=$(yq ".states[] | select(.name == \"$state_name\") | .after[$i].when // \"\"" "$workflow_file" | tr -d '"')
            if [[ -n "$when" ]]; then
                # Valid when: simple value, !negation, or value|value
                if ! [[ "$when" =~ ^!?[a-z_][a-z0-9_]*(\|[a-z_][a-z0-9_]*)*$ ]]; then
                    echo "ERROR: State '$state_name' after hook $i has invalid 'when' syntax: $when" >&2
                    return 1
                fi
            fi
        done
    done

    return 0
}

# ==============================================================================
# V4 Validation: Validate escalation triggers for each state
# Checks trigger types, iteration values, action validation
# Returns: 0 if valid, 1 if invalid
# ==============================================================================
validate_v4_escalation() {
    local workflow_file="$1"
    local known_actions
    known_actions=$(get_known_actions)

    # Get all states with escalation
    local states_with_escalation
    states_with_escalation=$(yq '.states[] | select(.escalation != null) | .name' "$workflow_file" 2>/dev/null | tr -d '"')

    for state_name in $states_with_escalation; do
        local esc_count
        esc_count=$(yq ".states[] | select(.name == \"$state_name\") | .escalation | length" "$workflow_file")

        for ((i=0; i<esc_count; i++)); do
            # Check exactly one trigger type
            local at_iter after_iter every_n
            at_iter=$(yq ".states[] | select(.name == \"$state_name\") | .escalation[$i].at_iteration // null" "$workflow_file")
            after_iter=$(yq ".states[] | select(.name == \"$state_name\") | .escalation[$i].after_iteration // null" "$workflow_file")
            every_n=$(yq ".states[] | select(.name == \"$state_name\") | .escalation[$i].every_n_iterations // null" "$workflow_file")

            local trigger_count=0
            [[ "$at_iter" != "null" ]] && ((trigger_count++))
            [[ "$after_iter" != "null" ]] && ((trigger_count++))
            [[ "$every_n" != "null" ]] && ((trigger_count++))

            if [[ $trigger_count -ne 1 ]]; then
                echo "ERROR: State '$state_name' escalation $i must have exactly one of: at_iteration, after_iteration, every_n_iterations" >&2
                return 1
            fi

            # Check iteration value is positive
            local iter_value
            if [[ "$at_iter" != "null" ]]; then iter_value="$at_iter"; fi
            if [[ "$after_iter" != "null" ]]; then iter_value="$after_iter"; fi
            if [[ "$every_n" != "null" ]]; then iter_value="$every_n"; fi

            if ! [[ "$iter_value" =~ ^[1-9][0-9]*$ ]]; then
                echo "ERROR: State '$state_name' escalation $i has invalid iteration value: $iter_value" >&2
                return 1
            fi

            # Check action is required and known
            local action
            action=$(yq ".states[] | select(.name == \"$state_name\") | .escalation[$i].action // \"\"" "$workflow_file" | tr -d '"')
            if [[ -z "$action" ]]; then
                echo "ERROR: State '$state_name' escalation $i missing required 'action' field" >&2
                return 1
            fi
            if ! echo "$known_actions" | grep -q "^$action$"; then
                echo "ERROR: State '$state_name' escalation $i has unknown action: $action" >&2
                return 1
            fi

            # Check params is object if present
            local params_json
            params_json=$(yq -o=json ".states[] | select(.name == \"$state_name\") | .escalation[$i].params // null" "$workflow_file")
            if [[ "$params_json" != "null" ]]; then
                local is_object
                is_object=$(echo "$params_json" | jq 'type == "object"' 2>/dev/null)
                if [[ "$is_object" != "true" ]]; then
                    echo "ERROR: State '$state_name' escalation $i has invalid 'params': must be object" >&2
                    return 1
                fi
            fi
        done
    done

    return 0
}

# ==============================================================================
# V4 Validation: Validate global intervention_triggers section
# Checks condition syntax, action validation
# Returns: 0 if valid, 1 if invalid
# ==============================================================================
validate_v4_intervention_triggers() {
    local workflow_file="$1"
    local known_actions
    known_actions=$(get_known_actions)

    # Check if intervention_triggers exists
    local has_triggers
    has_triggers=$(yq '.intervention_triggers // null' "$workflow_file")
    if [[ "$has_triggers" == "null" ]]; then
        return 0  # Optional, not present
    fi

    local trigger_count
    trigger_count=$(yq '.intervention_triggers | length' "$workflow_file")

    for ((i=0; i<trigger_count; i++)); do
        # Check condition is present
        local condition
        condition=$(yq ".intervention_triggers[$i].condition // \"\"" "$workflow_file" | tr -d '"')
        if [[ -z "$condition" ]]; then
            echo "ERROR: intervention_triggers[$i] missing required 'condition' field" >&2
            return 1
        fi

        # Check condition syntax (variable operator value)
        if ! [[ "$condition" =~ ^[a-zA-Z_][a-zA-Z0-9_]*[[:space:]]*(==|!=|>=|<=|>|<)[[:space:]]*(.+)$ ]]; then
            echo "ERROR: intervention_triggers[$i] has invalid condition syntax: $condition" >&2
            return 1
        fi

        # Check action is present and known
        local action
        action=$(yq ".intervention_triggers[$i].action // \"\"" "$workflow_file" | tr -d '"')
        if [[ -z "$action" ]]; then
            echo "ERROR: intervention_triggers[$i] missing required 'action' field" >&2
            return 1
        fi
        if ! echo "$known_actions" | grep -q "^$action$"; then
            echo "ERROR: intervention_triggers[$i] has unknown action: $action" >&2
            return 1
        fi
    done

    return 0
}

# ==============================================================================
# V5 Validation: Validate parallel blocks in states
# Checks strategy, branches, branch fields, uniqueness, n_of_m n value, prompt files
# Returns: 0 if valid, 1 if invalid
# ==============================================================================
validate_v5_parallel() {
    local workflow_file="$1"
    local local_errors=0
    local state_count
    state_count=$(yq '.states | length' "$workflow_file")

    local terminal_states_str
    terminal_states_str=" $(yq '.terminal_states[]' "$workflow_file" 2>/dev/null | tr '\n' ' ')"

    for ((i=0; i<state_count; i++)); do
        local state_name has_parallel
        state_name=$(yq ".states[$i].name" "$workflow_file")
        has_parallel=$(yq ".states[$i].parallel | tag" "$workflow_file")

        if [[ "$has_parallel" == "!!null" ]]; then
            continue
        fi

        # Rule 8: parallel states must NOT be in terminal_states
        if [[ "${terminal_states_str}" == *" ${state_name} "* ]]; then
            echo "ERROR: State '$state_name' has parallel block but is listed in terminal states" >&2
            ((local_errors++)) || true
            continue
        fi

        # Rule 1: strategy must be "all", "any", or "n_of_m"
        local strategy
        strategy=$(yq ".states[$i].parallel.strategy // \"\"" "$workflow_file")
        if [[ "$strategy" != "all" && "$strategy" != "any" && "$strategy" != "n_of_m" ]]; then
            echo "ERROR: State '$state_name' has invalid parallel strategy '$strategy' (must be all, any, or n_of_m)" >&2
            ((local_errors++)) || true
            continue
        fi

        # Rule 2: branches must exist
        local branches_tag
        branches_tag=$(yq ".states[$i].parallel.branches | tag" "$workflow_file")
        if [[ "$branches_tag" == "!!null" ]]; then
            echo "ERROR: State '$state_name' parallel block missing branches" >&2
            ((local_errors++)) || true
            continue
        fi

        # Rule 2: branches must be non-empty
        local branch_count
        branch_count=$(yq ".states[$i].parallel.branches | length" "$workflow_file")
        if [[ "$branch_count" -eq 0 ]]; then
            echo "ERROR: State '$state_name' parallel block has empty branches array" >&2
            ((local_errors++)) || true
            continue
        fi

        # Rule 5: n_of_m requires positive integer n
        if [[ "$strategy" == "n_of_m" ]]; then
            local n_val
            n_val=$(yq ".states[$i].parallel.n // \"\"" "$workflow_file")
            if [[ -z "$n_val" || "$n_val" == "null" ]]; then
                echo "ERROR: State '$state_name' strategy n_of_m requires n required" >&2
                ((local_errors++)) || true
            elif ! [[ "$n_val" =~ ^[1-9][0-9]*$ ]]; then
                echo "ERROR: State '$state_name' strategy n_of_m requires n to be a positive integer, got '$n_val'" >&2
                ((local_errors++)) || true
            fi
        fi

        # Validate each branch
        local seen_names=""
        for ((j=0; j<branch_count; j++)); do
            # Rule 3: branch must have "name"
            local branch_name
            branch_name=$(yq ".states[$i].parallel.branches[$j].name // \"\"" "$workflow_file")
            if [[ -z "$branch_name" ]]; then
                echo "ERROR: State '$state_name' branch $j missing name" >&2
                ((local_errors++)) || true
                continue
            fi

            # Rule 4: branch names must be unique
            if [[ " $seen_names " == *" $branch_name "* ]]; then
                echo "ERROR: State '$state_name' has duplicate branch name '$branch_name'" >&2
                ((local_errors++)) || true
            fi
            seen_names="$seen_names $branch_name"

            # Rule 3: branch must have "prompt"
            local branch_prompt
            branch_prompt=$(yq ".states[$i].parallel.branches[$j].prompt // \"\"" "$workflow_file")
            if [[ -z "$branch_prompt" ]]; then
                echo "ERROR: State '$state_name' branch '$branch_name' missing prompt" >&2
                ((local_errors++)) || true
                continue
            fi

            # Rule 6: branch prompt file must exist on disk
            local arc_dir
            arc_dir="${ARC_HOME:-$(dirname "$(dirname "$(readlink -f "$0")")")}"
            local full_prompt_path="$arc_dir/$branch_prompt"
            if [[ ! -f "$full_prompt_path" ]]; then
                echo "ERROR: State '$state_name' branch '$branch_name' prompt not found: $branch_prompt" >&2
                ((local_errors++)) || true
            fi
        done
    done

    return $local_errors
}

# ==============================================================================
# V5 Validation: Validate parallel verdict consistency
# Checks that verdicts match the strategy's valid set
# Returns: 0 if valid, 1 if invalid
# ==============================================================================
validate_v5_parallel_verdicts() {
    local workflow_file="$1"
    local local_errors=0
    local state_count
    state_count=$(yq '.states | length' "$workflow_file")

    for ((i=0; i<state_count; i++)); do
        local has_parallel
        has_parallel=$(yq ".states[$i].parallel | tag" "$workflow_file")
        if [[ "$has_parallel" == "!!null" ]]; then
            continue
        fi

        # Skip if next is not a map (linear transition, no verdicts)
        local next_tag
        next_tag=$(yq ".states[$i].next | tag" "$workflow_file")
        if [[ "$next_tag" != "!!map" ]]; then
            continue
        fi

        # Check if state has verdicts
        local verdict_count
        verdict_count=$(yq ".states[$i].verdicts | length" "$workflow_file")
        if [[ "$verdict_count" == "0" || "$verdict_count" == "null" ]]; then
            continue
        fi

        local strategy state_name
        strategy=$(yq ".states[$i].parallel.strategy // \"\"" "$workflow_file")
        state_name=$(yq ".states[$i].name" "$workflow_file")

        # Determine valid verdicts for this strategy
        local valid_verdicts
        case "$strategy" in
            all) valid_verdicts="all_complete any_failed" ;;
            any) valid_verdicts="first_complete all_failed" ;;
            n_of_m) valid_verdicts="n_complete insufficient" ;;
            *) continue ;;
        esac

        # Check each verdict is in the valid set
        for ((j=0; j<verdict_count; j++)); do
            local v
            v=$(yq ".states[$i].verdicts[$j]" "$workflow_file")
            if [[ " $valid_verdicts " != *" $v "* ]]; then
                echo "ERROR: State '$state_name' has invalid verdict '$v' for strategy '$strategy' (valid: $valid_verdicts)" >&2
                ((local_errors++)) || true
            fi
        done
    done

    return $local_errors
}

# ==============================================================================
# Main script body — only runs when executed directly, not when sourced
# ==============================================================================
_validate_workflow_main() {

# Validate arguments
[[ $# -ge 1 ]] || error "Usage: $0 <workflow-file>"

WORKFLOW_FILE="$1"

# Check file exists
[[ -f "$WORKFLOW_FILE" ]] || error "Workflow file not found: $WORKFLOW_FILE"

echo "Validating workflow: $WORKFLOW_FILE"
echo ""

# 1. Validate YAML syntax
echo "Checking YAML syntax..."
if ! yq '.' "$WORKFLOW_FILE" > /dev/null 2>&1; then
    error_output=$(yq '.' "$WORKFLOW_FILE" 2>&1 || true)
    error "Invalid YAML syntax: $error_output"
fi
echo "✓ YAML syntax valid"

# 2. Check required top-level fields
echo "Checking required fields..."

name=$(yq '.name // ""' "$WORKFLOW_FILE")
[[ -n "$name" ]] || error "Workflow missing required field: name"
echo "✓ name: $name"

version=$(yq '.version // ""' "$WORKFLOW_FILE")
[[ -n "$version" ]] || error "Workflow missing required field: version"
[[ "$version" == "1" || "$version" == "2" || "$version" == "3" || "$version" == "4" || "$version" == "5" ]] || warn "Version is $version (expected 1, 2, 3, 4, or 5)"
echo "✓ version: $version"

entry_state=$(yq '.entry_state // ""' "$WORKFLOW_FILE")
[[ -n "$entry_state" ]] || error "Workflow missing required field: entry_state"
echo "✓ entry_state: $entry_state"

terminal_states=$(yq '.terminal_states | length' "$WORKFLOW_FILE")
[[ "$terminal_states" -gt 0 ]] || error "Workflow must have at least one terminal_state"
echo "✓ terminal_states: $terminal_states defined"

# 3. Check states array exists and has entries
states_count=$(yq '.states | length' "$WORKFLOW_FILE")
[[ "$states_count" -gt 0 ]] || error "Workflow must have at least one state"
echo "✓ states: $states_count defined"

# 4. Validate each state has required fields
echo ""
echo "Checking state definitions..."
errors=0

for i in $(seq 0 $((states_count - 1))); do
    state_name=$(yq ".states[$i].name // \"\"" "$WORKFLOW_FILE")
    if [[ -z "$state_name" ]]; then
        echo "❌ State $i missing required field: name"
        ((errors++))
        continue
    fi

    # Check if state is terminal (must happen before prompt check)
    is_terminal=$(yq ".terminal_states | contains([\"$state_name\"])" "$WORKFLOW_FILE")

    # Non-terminal states require either a prompt or a parallel block
    if [[ "$is_terminal" != "true" ]]; then
        state_prompt=$(yq ".states[$i].prompt // \"\"" "$WORKFLOW_FILE")
        local has_parallel
        has_parallel=$(yq ".states[$i].parallel // \"\"" "$WORKFLOW_FILE")
        if [[ -z "$state_prompt" && ( -z "$has_parallel" || "$has_parallel" == "null" ) ]]; then
            echo "❌ State '$state_name' missing required field: prompt"
            ((errors++))
        fi
    fi

    if [[ "$is_terminal" != "true" ]]; then
        # Non-terminal states must have 'next'
        state_next=$(yq ".states[$i].next // \"\"" "$WORKFLOW_FILE")
        if [[ -z "$state_next" ]]; then
            echo "❌ Non-terminal state '$state_name' missing required field: next"
            ((errors++))
        else
            echo "✓ State '$state_name' -> $state_next"
        fi
    else
        echo "✓ State '$state_name' (terminal)"
    fi
done

# 5. Verify entry_state exists in states
entry_exists=$(yq ".states[] | select(.name == \"$entry_state\") | .name" "$WORKFLOW_FILE")
if [[ -z "$entry_exists" ]]; then
    echo "❌ entry_state '$entry_state' not found in states"
    ((errors++))
fi

# 6. Verify all terminal_states exist in states
echo ""
echo "Checking terminal states exist..."
for term_state in $(yq '.terminal_states[]' "$WORKFLOW_FILE"); do
    term_exists=$(yq ".states[] | select(.name == \"$term_state\") | .name" "$WORKFLOW_FILE")
    if [[ -z "$term_exists" ]]; then
        # Terminal states might not need a definition (like 'complete', 'blocked')
        # This is a warning, not an error for bootstrap
        warn "Terminal state '$term_state' not defined in states (OK for implicit terminals)"
    else
        echo "✓ Terminal state '$term_state' defined"
    fi
done

# ==============================================================================
# Run new validation checks (added by validation-complete phase)
# ==============================================================================
echo ""
echo "Running graph validation checks..."

check_entry_not_terminal || { ((errors++)) || true; }
check_unique_state_names || { ((errors++)) || true; }
check_prompt_files_exist || { ((errors++)) || true; }
check_next_references_valid || { ((errors++)) || true; }
check_no_unreachable_states || { ((errors++)) || true; }
check_all_reach_terminal || { ((errors++)) || true; }

# Check V2 verdict consistency
check_verdict_consistency
if [[ $errors -eq 0 ]]; then
    echo "✓ Verdict consistency valid"
fi

# ==============================================================================
# V3 Validation (only for version >= 3)
# ==============================================================================
if [[ "$version" -ge 3 ]] 2>/dev/null; then
    echo ""
    echo "Running V3 schema validation..."

    validate_defaults "$WORKFLOW_FILE" || { ((errors += $?)) || true; }
    validate_variables "$WORKFLOW_FILE" || { ((errors += $?)) || true; }
    validate_state_params "$WORKFLOW_FILE" || { ((errors += $?)) || true; }
    check_reserved_conflicts "$WORKFLOW_FILE" || { ((errors += $?)) || true; }

    if [[ $errors -eq 0 ]]; then
        echo "✓ V3 schema validation passed"
    fi
fi

# ==============================================================================
# V4 Validation — all functions return 0 if their sections are absent,
# so V1-V3 workflows pass without modification.
# ==============================================================================
echo ""
echo "Running V4 schema validation..."

validate_v4_constraints "$WORKFLOW_FILE" || { ((errors++)) || true; }
validate_v4_after_hooks "$WORKFLOW_FILE" || { ((errors++)) || true; }
validate_v4_escalation "$WORKFLOW_FILE" || { ((errors++)) || true; }
validate_v4_intervention_triggers "$WORKFLOW_FILE" || { ((errors++)) || true; }

if [[ $errors -eq 0 ]]; then
    echo "✓ V4 schema validation passed"
fi

# ==============================================================================
# V5 Validation (only for version >= 5)
# ==============================================================================
if [[ "$version" -ge 5 ]] 2>/dev/null; then
    echo ""
    echo "Running V5 schema validation..."

    validate_v5_parallel "$WORKFLOW_FILE" || { ((errors++)) || true; }
    validate_v5_parallel_verdicts "$WORKFLOW_FILE" || { ((errors++)) || true; }

    if [[ $errors -eq 0 ]]; then
        echo "✓ V5 schema validation passed"
    fi
fi

echo ""
if [[ $errors -gt 0 ]]; then
    error "Validation failed with $errors error(s)"
else
    echo "Validation passed!"
fi

} # end _validate_workflow_main

# Run main only when executed directly (not sourced)
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    _validate_workflow_main "$@"
fi
