#!/usr/bin/env bash
#
# Constraint Validation for V4 Workflows
#
# Validates state constraints defined in workflow.yaml:
#   - max_iterations: Maximum allowed iterations for a state
#   - require_artifacts_in: Files that must exist before state execution
#   - require_artifacts_out: Files that must exist after state execution
#
# Usage: Source this file, then call validation functions
#   source check-constraints.sh
#   check_pre_constraints "$WORKFLOW_FILE" "$STATE_NAME" "$PHASE_DIR"
#   check_post_constraints "$WORKFLOW_FILE" "$STATE_NAME" "$PHASE_DIR"
#
# Required environment variables:
#   STATE_FILE   - Path to state.json

# NOTE: We use `set -uo pipefail` (no -e) because functions must handle
# non-zero exit codes without terminating.
# Requires: yq (mikefarah/yq v4+), jq
set -uo pipefail

# get_state_constraints <workflow_file> <state_name>
#
# Extract constraints object for a state.
#
# Arguments:
#   workflow_file  Path to workflow.yaml
#   state_name     State name
#
# Output (stdout):
#   JSON object with constraints, or "{}" if none
get_state_constraints() {
    local workflow_file="$1"
    local state_name="$2"

    local result
    result=$(yq -o=json ".states[] | select(.name == \"$state_name\") | .constraints // {}" "$workflow_file") || return 1

    if [[ -z "$result" || "$result" == "null" ]]; then
        echo "{}"
    else
        echo "$result"
    fi
}

# check_max_iterations <workflow_file> <state_name>
#
# Check if current iteration exceeds max_iterations constraint.
#
# Arguments:
#   workflow_file  Path to workflow.yaml
#   state_name     Current state name
#
# Exit codes:
#   0  Within limit
#   1  Exceeded limit
#
# Reads current iteration from STATE_FILE environment variable.
check_max_iterations() {
    local workflow_file="$1"
    local state_name="$2"

    if [[ -z "${STATE_FILE:-}" ]]; then
        echo "ERROR: STATE_FILE not set" >&2
        return 1
    fi

    local max_iter
    max_iter=$(yq ".states[] | select(.name == \"$state_name\") | .constraints.max_iterations // 999" "$workflow_file")

    local current_iter
    current_iter=$(jq -r '.iteration // 0' "$STATE_FILE")

    if [[ "$current_iter" -ge "$max_iter" ]]; then
        echo "ERROR: Max iterations exceeded for state '$state_name': $current_iter >= $max_iter" >&2
        return 1
    fi

    return 0
}

# check_artifacts_exist <phase_dir> <artifact_list>
#
# Check if all artifacts in the list exist.
#
# Arguments:
#   phase_dir      Path to phase directory
#   artifact_list  JSON array of artifact paths (relative to phase_dir)
#
# Exit codes:
#   0  All artifacts exist
#   1  At least one artifact missing (lists missing to stderr)
check_artifacts_exist() {
    local phase_dir="$1"
    local artifact_list="$2"

    local missing=()

    while IFS= read -r artifact; do
        [[ -z "$artifact" ]] && continue
        local full_path="$phase_dir/$artifact"

        if [[ ! -f "$full_path" ]]; then
            missing+=("$artifact")
        fi
    done < <(echo "$artifact_list" | jq -r '.[]')

    if [[ ${#missing[@]} -gt 0 ]]; then
        echo "ERROR: Missing required artifacts:" >&2
        for m in "${missing[@]}"; do
            echo "  - $m" >&2
        done
        return 1
    fi

    return 0
}

# check_pre_constraints <workflow_file> <state_name> <phase_dir>
#
# Check constraints before state execution.
#
# Arguments:
#   workflow_file  Path to workflow.yaml
#   state_name     Current state name (e.g., "impl")
#   phase_dir      Path to phase directory
#
# Exit codes:
#   0  All pre-constraints satisfied
#   1  Constraint violated (with error to stderr)
#
# Checks:
#   - max_iterations not exceeded
#   - require_artifacts_in all exist
check_pre_constraints() {
    local workflow_file="$1"
    local state_name="$2"
    local phase_dir="$3"

    local constraints
    constraints=$(get_state_constraints "$workflow_file" "$state_name") || return 1

    if [[ "$constraints" == "{}" || -z "$constraints" ]]; then
        return 0
    fi

    local max_iter
    max_iter=$(echo "$constraints" | jq -r '.max_iterations // empty')
    if [[ -n "$max_iter" ]]; then
        check_max_iterations "$workflow_file" "$state_name" || return 1
    fi

    local artifacts_in
    artifacts_in=$(echo "$constraints" | jq -c '.require_artifacts_in // []')
    if [[ "$artifacts_in" != "[]" ]]; then
        check_artifacts_exist "$phase_dir" "$artifacts_in" || return 1
    fi

    return 0
}

# check_post_constraints <workflow_file> <state_name> <phase_dir>
#
# Check constraints after state execution.
#
# Arguments:
#   workflow_file  Path to workflow.yaml
#   state_name     Current state name
#   phase_dir      Path to phase directory
#
# Exit codes:
#   0  All post-constraints satisfied
#   1  Constraint violated (with error to stderr)
#
# Checks:
#   - require_artifacts_out all exist
check_post_constraints() {
    local workflow_file="$1"
    local state_name="$2"
    local phase_dir="$3"

    local constraints
    constraints=$(get_state_constraints "$workflow_file" "$state_name") || return 1

    if [[ "$constraints" == "{}" || -z "$constraints" ]]; then
        return 0
    fi

    local artifacts_out
    artifacts_out=$(echo "$constraints" | jq -c '.require_artifacts_out // []')
    if [[ "$artifacts_out" != "[]" ]]; then
        check_artifacts_exist "$phase_dir" "$artifacts_out" || return 1
    fi

    return 0
}
