#!/usr/bin/env bash
#
# Action Registry for V4 Hooks
#
# Each action function follows this pattern:
#   action_<name>() - Takes parameters from workflow, returns exit code
#
# Usage: Source this file, then call actions with parameters
#   source actions.sh
#   action_run_tests "pattern" "output.txt" "false"
#
# Available actions:
#   action_run_tests <pattern> <save_to> [expect_failure]
#   action_commit <message> [when_verdict]
#   action_switch_model <model>
#   action_analyze_stuck
#   action_request_human <message>
#   action_script <script_path> [args...]

# Required environment variables (must be set before calling):
#   PHASE_DIR         - Path to current phase directory
#   ARC_DEFAULT_PKG   - Default package name for tests
#   STATE_FILE        - Path to state.json
#   VERDICT           - Current verdict (for conditional actions)
#   ARC_HOME          - Path to arc installation (for action_script)
#   ARC_RUNNER_DIR    - Path to runner plugin directory

# NOTE: We use `set -uo pipefail` (no -e) because functions like action_run_tests
# must capture non-zero exit codes via `|| exit_code=$?`. Using `set -e` would
# cause the function to exit before the exit code capture.
set -uo pipefail

# action_run_tests <pattern> <save_to> [expect_failure]
#
# Run tests using the configured runner plugin and save output.
#
# Arguments:
#   pattern        Test name pattern (e.g., "qa_phase_name")
#   save_to        Output file path relative to PHASE_DIR
#   expect_failure Optional: "true" if tests should fail (QA phase)
#
# Exit codes:
#   0  Tests match expectation (passed when expect_failure=false, failed when expect_failure=true)
#   1  Tests did not match expectation
#
# Side effects:
#   - Writes test output to $PHASE_DIR/$save_to
#   - Updates state.json with tests_total, tests_passing
action_run_tests() {
    local pattern="$1"
    local save_to="$2"
    local expect_failure="${3:-false}"

    # Guard: validate required environment variables
    if [[ -z "${PHASE_DIR:-}" ]]; then
        echo "ERROR: PHASE_DIR not set" >&2
        return 1
    fi
    if [[ -z "${STATE_FILE:-}" ]]; then
        echo "ERROR: STATE_FILE not set" >&2
        return 1
    fi
    if [[ ! -d "$PHASE_DIR" ]]; then
        echo "ERROR: PHASE_DIR does not exist: $PHASE_DIR" >&2
        return 1
    fi

    # Load runner config if not already loaded
    if [[ -z "${ARC_RUNNER_DIR:-}" ]]; then
        local scripts_dir="${ARC_SCRIPTS_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)}"
        source "$scripts_dir/config.sh"
    fi

    if [[ -z "${ARC_RUNNER_DIR:-}" || ! -x "$ARC_RUNNER_DIR/run.sh" ]]; then
        echo "ERROR: No runner plugin configured. Run 'arc init' first." >&2
        return 1
    fi

    local output_path="$PHASE_DIR/$save_to"
    local json_output
    json_output=$(mktemp)

    # Run tests via runner plugin (allow failure)
    local exit_code=0
    "$ARC_RUNNER_DIR/run.sh" "$PHASE_DIR" "$pattern" > "$json_output" 2>&1 || exit_code=$?

    # Parse JSON output from runner
    local total passed failed
    total=$(jq -r '.total // 0' "$json_output" 2>/dev/null || echo "0")
    passed=$(jq -r '.passed // 0' "$json_output" 2>/dev/null || echo "0")
    failed=$(jq -r '.failed // 0' "$json_output" 2>/dev/null || echo "0")

    # Save raw output for debugging
    jq -r '.raw_output // ""' "$json_output" 2>/dev/null > "$output_path" || cp "$json_output" "$output_path"
    rm -f "$json_output"

    # If runner exited non-zero AND no tests found, this is a hard error
    if [[ "$exit_code" -ne 0 && "$total" -eq 0 ]]; then
        jq --argjson total 0 --argjson passed 0 \
           '.tests_total = $total | .tests_passing = $passed' \
           "$STATE_FILE" > "${STATE_FILE}.tmp.$$" && mv "${STATE_FILE}.tmp.$$" "$STATE_FILE"
        return 1
    fi

    # Update state.json atomically (use $$ PID suffix to prevent race conditions)
    jq --argjson total "${total:-0}" \
       --argjson passed "${passed:-0}" \
       '.tests_total = $total | .tests_passing = $passed' \
       "$STATE_FILE" > "${STATE_FILE}.tmp.$$" && mv "${STATE_FILE}.tmp.$$" "$STATE_FILE" || {
        echo "ERROR: Failed to update state file: $STATE_FILE" >&2
        return 1
    }

    # Determine success based on expectation
    if [[ "$expect_failure" == "true" ]]; then
        # QA phase: expect tests to fail (no implementation yet)
        [[ "${failed:-0}" -gt 0 ]] && return 0 || return 1
    else
        # Impl phase: expect tests to pass
        [[ "${failed:-0}" -eq 0 ]] && return 0 || return 1
    fi
}

# action_commit <message> [when_verdict]
#
# Create a git commit with staged changes.
#
# Arguments:
#   message      Commit message (may contain template variables)
#   when_verdict Optional: Only commit if VERDICT matches this value
#
# Exit codes:
#   0  Commit succeeded or skipped (condition not met)
#   1  Commit failed
#
# Side effects:
#   - Creates git commit if conditions met
#   - Writes commit hash to state.json "last_commit" field
action_commit() {
    local message="$1"
    local when="${2:-}"

    # Guard: validate required environment variables
    if [[ -z "${PHASE_DIR:-}" ]]; then
        echo "ERROR: PHASE_DIR not set" >&2
        return 1
    fi
    if [[ -z "${STATE_FILE:-}" ]]; then
        echo "ERROR: STATE_FILE not set" >&2
        return 1
    fi

    # Check when condition — compares against VERDICT env var
    # Note: VERDICT unset and VERDICT="" are both treated as "no verdict"
    if [[ -n "$when" && "${VERDICT:-}" != "$when" ]]; then
        return 0  # Skip, condition not met
    fi

    # Check commit gates
    check_commit_allowed || return 1

    # Check we're in a git repository
    if ! git rev-parse --git-dir > /dev/null 2>&1; then
        echo "ERROR: Not a git repository" >&2
        return 1
    fi

    # Check for changes (staged OR unstaged)
    if git diff --cached --quiet 2>/dev/null && git diff --quiet 2>/dev/null; then
        # No changes to commit
        return 0
    fi

    # Stage all changes in repository
    git add -A || { echo "ERROR: git add failed" >&2; return 1; }

    # Create commit and capture hash
    git commit -m "$message" --quiet || { echo "ERROR: git commit failed" >&2; return 1; }
    local commit_hash
    commit_hash=$(git rev-parse HEAD)

    # Update state.json atomically (use $$ PID suffix to prevent race conditions)
    jq --arg hash "$commit_hash" '.last_commit = $hash' "$STATE_FILE" > "${STATE_FILE}.tmp.$$" \
        && mv "${STATE_FILE}.tmp.$$" "$STATE_FILE"

    return 0
}

# check_commit_allowed
#
# Internal helper: check if commits are allowed in current state.
#
# Exit codes:
#   0  Commits allowed
#   1  Commits not allowed (allow_commits=false in state.json)
check_commit_allowed() {
    # Guard: validate STATE_FILE
    if [[ -z "${STATE_FILE:-}" ]]; then
        echo "ERROR: STATE_FILE not set" >&2
        return 1
    fi

    # Check if commits are explicitly disabled
    # NOTE: Cannot use jq's // (alternative) operator here because it treats
    # boolean false as falsy and would return "true" for allow_commits=false.
    local allow_commits
    allow_commits=$(jq -r 'if .allow_commits == false then "false" else "true" end' "$STATE_FILE") || {
        echo "ERROR: Failed to read state file: $STATE_FILE" >&2
        return 1
    }

    if [[ "$allow_commits" == "false" ]]; then
        echo "ERROR: Commits not allowed in current state" >&2
        return 1
    fi

    return 0
}

# action_switch_model <model>
#
# Change the model used for subsequent agent spawns.
#
# Arguments:
#   model  Model name: "opus", "sonnet", or "haiku"
#
# Exit codes:
#   0  Model switched
#   1  Invalid model name
#
# Side effects:
#   - Updates state.json "current_model" field (per STATE_SCHEMA.md)
action_switch_model() {
    local model="$1"

    # Guard: validate required environment variables
    if [[ -z "${STATE_FILE:-}" ]]; then
        echo "ERROR: STATE_FILE not set" >&2
        return 1
    fi

    # Validate model name
    case "$model" in
        opus|sonnet|haiku)
            ;;
        *)
            echo "ERROR: Invalid model '$model'. Must be opus, sonnet, or haiku." >&2
            return 1
            ;;
    esac

    # Validate state file exists
    if [[ ! -f "$STATE_FILE" ]]; then
        echo "ERROR: State file not found: $STATE_FILE" >&2
        return 1
    fi

    # Update state.json atomically (field is "current_model" per STATE_SCHEMA.md)
    jq --arg model "$model" '.current_model = $model' "$STATE_FILE" > "${STATE_FILE}.tmp.$$" \
        && mv "${STATE_FILE}.tmp.$$" "$STATE_FILE" || {
        echo "ERROR: Failed to update state file: $STATE_FILE" >&2
        return 1
    }

    return 0
}

# action_analyze_stuck
#
# Generate stuck analysis for the current phase.
#
# Arguments:
#   None (uses PHASE_DIR)
#
# Exit codes:
#   0  Analysis generated
#
# Side effects:
#   - Creates $PHASE_DIR/stuck_analysis.md
#   - Updates state.json "stuck_analysis_iteration" field
action_analyze_stuck() {
    # Guard: validate required environment variables
    if [[ -z "${PHASE_DIR:-}" ]]; then
        echo "ERROR: PHASE_DIR not set" >&2
        return 1
    fi
    if [[ -z "${STATE_FILE:-}" ]]; then
        echo "ERROR: STATE_FILE not set" >&2
        return 1
    fi

    local iteration
    iteration=$(jq -r '.iteration // 0' "$STATE_FILE")

    # Need at least 2 iterations to compare
    if [[ "$iteration" -lt 2 ]]; then
        echo "Not enough iterations to analyze" >&2
        return 0
    fi

    local prev_iter=$((iteration - 1))
    local curr_dir="$PHASE_DIR/iteration_$(printf '%03d' "$iteration")"
    local prev_dir="$PHASE_DIR/iteration_$(printf '%03d' "$prev_iter")"

    local analysis_file="$PHASE_DIR/stuck_analysis.md"

    {
        echo "# Stuck Analysis"
        echo ""
        echo "Generated at iteration $iteration"
        echo ""
        echo "## Repeated Failures"
        echo ""

        # Compare test failures if files exist
        if [[ -f "$curr_dir/test_output.txt" && -f "$prev_dir/test_output.txt" ]]; then
            echo "### Current Iteration ($iteration)"
            echo '```'
            grep -E "^FAIL|error\[" "$curr_dir/test_output.txt" 2>/dev/null | head -20 || echo "No failures captured"
            echo '```'
            echo ""
            echo "### Previous Iteration ($prev_iter)"
            echo '```'
            grep -E "^FAIL|error\[" "$prev_dir/test_output.txt" 2>/dev/null | head -20 || echo "No failures captured"
            echo '```'
        else
            echo "Test output files not found for comparison."
        fi

        echo ""
        echo "## Recommendations"
        echo ""
        echo "1. Review the error patterns above for common root cause"
        echo "2. Consider switching to a more capable model (opus)"
        echo "3. Consider requesting human intervention if fundamentally blocked"
    } > "$analysis_file"

    # Update state.json atomically (use $$ PID suffix to prevent race conditions)
    jq --argjson iter "$iteration" '.stuck_analysis_iteration = $iter' "$STATE_FILE" > "${STATE_FILE}.tmp.$$" \
        && mv "${STATE_FILE}.tmp.$$" "$STATE_FILE"

    return 0
}

# action_request_human <message>
#
# Request human intervention.
#
# Arguments:
#   message  Message to display to human
#
# Exit codes:
#   0  Request recorded
#
# Side effects:
#   - Creates $PHASE_DIR/intervention_request.md
#   - Updates state.json "intervention_request" field as object (per STATE_SCHEMA.md):
#     { "reason": <message>, "requested_at": <timestamp>, "options": [...] }
action_request_human() {
    local message="$1"

    # Guard: validate required environment variables
    if [[ -z "${PHASE_DIR:-}" ]]; then
        echo "ERROR: PHASE_DIR not set" >&2
        return 1
    fi
    if [[ -z "${STATE_FILE:-}" ]]; then
        echo "ERROR: STATE_FILE not set" >&2
        return 1
    fi

    local request_file="$PHASE_DIR/intervention_request.md"
    local timestamp
    timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

    {
        echo "# Human Intervention Requested"
        echo ""
        echo "**Timestamp:** $timestamp"
        echo ""
        echo "## Message"
        echo ""
        echo "$message"
        echo ""
        echo "## Context"
        echo ""
        echo "- Phase: $(basename "$PHASE_DIR")"
        echo "- Iteration: $(jq -r '.iteration // "unknown"' "$STATE_FILE")"
        echo "- Current State: $(jq -r '.current_state // "unknown"' "$STATE_FILE")"
    } > "$request_file"

    # Update state.json atomically with intervention_request object (per STATE_SCHEMA.md)
    jq --arg reason "$message" \
       --arg timestamp "$timestamp" \
       '.intervention_request = {
           "reason": $reason,
           "requested_at": $timestamp,
           "options": ["resolve", "modify_workflow", "skip", "abort"]
       }' \
       "$STATE_FILE" > "${STATE_FILE}.tmp.$$" && mv "${STATE_FILE}.tmp.$$" "$STATE_FILE" || {
        echo "ERROR: Failed to update state file: $STATE_FILE" >&2
        rm -f "${STATE_FILE}.tmp.$$"
        return 1
    }

    return 0
}

# action_script <script_path> [args...]
#
# Execute a custom script.
#
# Arguments:
#   script_path  Path to script relative to ARC_HOME
#   args...      Additional arguments passed to script
#
# Exit codes:
#   Returns exit code from script
action_script() {
    local script_path="$1"
    shift
    local args=("$@")

    # Guard: validate required environment variables
    if [[ -z "${ARC_HOME:-}" ]]; then
        echo "ERROR: ARC_HOME not set" >&2
        return 1
    fi

    # Reject path traversal attempts (../ or absolute paths)
    if [[ "$script_path" == /* ]]; then
        echo "ERROR: Script path must be relative and cannot contain '..': $script_path" >&2
        return 1
    fi
    if [[ "$script_path" == *..* ]]; then
        echo "ERROR: Script path must be relative and cannot contain '..': $script_path" >&2
        return 1
    fi

    # Resolve path relative to ARC_HOME
    local full_path="$ARC_HOME/$script_path"

    # Validate script exists
    if [[ ! -f "$full_path" ]]; then
        echo "ERROR: Script not found: $full_path" >&2
        return 1
    fi

    # Validate script is executable
    if [[ ! -x "$full_path" ]]; then
        echo "ERROR: Script not executable: $full_path" >&2
        return 1
    fi

    # Execute script with arguments
    if [[ ${#args[@]} -eq 0 ]]; then
        "$full_path"
    else
        "$full_path" "${args[@]}"
    fi
}
