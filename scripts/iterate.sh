#!/usr/bin/env bash
#
# iterate.sh - Run one iteration via sub-agent
#
# Sub-agents do NOT commit. They only edit files.
# Orchestrator (or phase-runner) commits at phase boundaries.
#
# Usage: iterate.sh <plan-name> <phase> [qa|impl|fix] [orchestrator-instructions]
#
# The optional orchestrator-instructions argument allows the orchestrator to pass
# additional context to impl agents (e.g., structural changes from plan's Delete section).
# These instructions are PREPENDED to the standard TDD workflow on iteration 1 only.

set -euo pipefail

# =============================================================================
# PROCESS GROUP TRACKING
# =============================================================================
# Track spawned process groups so we can clean up ONLY our processes,
# not all cargo processes system-wide.
# =============================================================================

declare -a TRACKED_PGIDS=()

# Add a PGID to tracking
track_pgid() {
    local pgid="$1"
    TRACKED_PGIDS+=("$pgid")
}

# Remove a PGID from tracking
untrack_pgid() {
    local pgid="$1"
    local new_array=()
    for p in "${TRACKED_PGIDS[@]:-}"; do
        [[ "$p" != "$pgid" ]] && new_array+=("$p")
    done
    TRACKED_PGIDS=("${new_array[@]:-}")
}

# Kill all tracked process groups
kill_tracked_pgids() {
    for pgid in "${TRACKED_PGIDS[@]:-}"; do
        [[ -z "$pgid" ]] && continue
        # SIGTERM first for graceful shutdown
        kill -TERM -"$pgid" 2>/dev/null || true
    done
    sleep 1
    for pgid in "${TRACKED_PGIDS[@]:-}"; do
        [[ -z "$pgid" ]] && continue
        # SIGKILL for stubborn processes
        kill -KILL -"$pgid" 2>/dev/null || true
    done
    TRACKED_PGIDS=()
}

# Clean up on exit
cleanup() {
    echo "Cleaning up tracked process groups..."
    kill_tracked_pgids
    # Also kill our own process group as fallback
    kill -TERM -$$ 2>/dev/null || true
    sleep 1
    kill -KILL -$$ 2>/dev/null || true
    exit 1
}
trap cleanup SIGTERM SIGINT

# Run command with timeout in a new process group
# This allows us to kill ONLY processes spawned by this command
run_with_timeout() {
    local timeout_secs=$1
    shift

    # Run command in new process group using setsid
    # The timeout command becomes the process group leader
    setsid timeout -k 30 "$timeout_secs" "$@" &
    local pid=$!

    # The PGID equals the PID of the session leader (the setsid'd process)
    local pgid=$pid
    track_pgid "$pgid"

    # Wait for completion
    wait $pid 2>/dev/null
    local exit_code=$?

    if [[ $exit_code -eq 124 ]]; then
        # Timeout occurred - kill the process group
        echo "Timeout: killing process group $pgid..."
        kill -TERM -"$pgid" 2>/dev/null || true
        sleep 1
        kill -KILL -"$pgid" 2>/dev/null || true
    fi

    # Remove from tracking
    untrack_pgid "$pgid"

    return $exit_code
}

# =============================================================================
# V4 FUNCTIONS
# =============================================================================
# These functions integrate all V4 features into the iteration loop:
#   - setup_v4_environment: Environment setup and V4 library sourcing
#   - run_iteration: Main V4 iteration loop (7-step execution order)
#   - update_state_after_iteration: State updates
#   - spawn_agent: Wrapper around existing run_with_timeout + claude
# =============================================================================

# spawn_agent <prompt_file> <output_file>
#
# Wrapper around existing run_with_timeout + claude invocation.
#
# Arguments:
#   prompt_file  Path to prompt file
#   output_file  Path to write agent output
#
# Exit codes:
#   0    Agent completed successfully
#   124  Agent timed out
#   *    Other failure
spawn_agent() {
    local prompt_file="$1"
    local output_file="$2"

    local timeout="${SUB_AGENT_TIMEOUT:-600}"

    run_with_timeout "$timeout" claude -p "$(cat "$prompt_file")" \
        --allowedTools "View,Edit,Write,Bash" \
        --max-turns 15 > "$output_file" 2>&1

    return $?
}

# setup_v4_environment <plan_name> <phase_name>
#
# Set up environment variables required by V4 scripts.
#
# Sets:
#   PHASE_DIR, STATE_FILE, WORKFLOW_FILE, ARC_HOME, ARC_DEFAULT_PKG
setup_v4_environment() {
    local plan_name="$1"
    local phase_name="$2"

    # Set paths
    export ARC_PLANS_DIR="${ARC_PLANS_DIR:-$ARC_PROJECT_ROOT/.plans}"
    export ARC_SCRIPTS_DIR="${ARC_SCRIPTS_DIR:-$ARC_HOME/scripts}"

    export PLAN_DIR="$ARC_PLANS_DIR/active/$plan_name"
    export PHASE_DIR="$PLAN_DIR/phases/$phase_name"
    export STATE_FILE="$PHASE_DIR/state.json"
    export WORKFLOW_FILE="$PLAN_DIR/workflow.yaml"

    # Set defaults
    export ARC_DEFAULT_PKG="${ARC_DEFAULT_PKG:-}"

    # Validate required files exist
    if [[ ! -f "$STATE_FILE" ]]; then
        echo "ERROR: State file not found: $STATE_FILE" >&2
        return 1
    fi

    if [[ ! -f "$WORKFLOW_FILE" ]]; then
        echo "ERROR: Workflow file not found: $WORKFLOW_FILE" >&2
        return 1
    fi

    # Source V4 libraries (AFTER ARC_SCRIPTS_DIR is set)
    source "$ARC_SCRIPTS_DIR/actions.sh"
    source "$ARC_SCRIPTS_DIR/check-constraints.sh"
    source "$ARC_SCRIPTS_DIR/check-escalation.sh"
    source "$ARC_SCRIPTS_DIR/check-intervention.sh"
    source "$ARC_SCRIPTS_DIR/run-hooks.sh"

    return 0
}

# =============================================================================
# V5 PARALLEL HELPER FUNCTIONS
# =============================================================================

# get_branch_names <workflow_file> <state_name>
#
# Extract comma-separated branch names from workflow parallel block.
get_branch_names() {
    local workflow_file="$1" state_name="$2"
    yq ".states[] | select(.name == \"$state_name\") | .parallel.branches[].name" "$workflow_file" | paste -sd,
}

# get_parallel_strategy <workflow_file> <state_name>
#
# Extract strategy from workflow parallel block.
get_parallel_strategy() {
    local workflow_file="$1" state_name="$2"
    yq ".states[] | select(.name == \"$state_name\") | .parallel.strategy" "$workflow_file"
}

# get_parallel_n <workflow_file> <state_name>
#
# Extract n value for n_of_m strategy.
get_parallel_n() {
    local workflow_file="$1" state_name="$2"
    yq ".states[] | select(.name == \"$state_name\") | .parallel.n // \"\"" "$workflow_file"
}

# run_parallel_state <plan_name> <phase_name> <state_name> <workflow_file>
#
# Handle a state that has a `parallel` block in the workflow.
# Orchestrates the full parallel lifecycle:
#   1. Read parallel config from workflow
#   2. Compute results_dir
#   3. Call update-state.sh parallel-start
#   4. Call run-parallel.sh
#   5. Process .exit files and update branch status
#   6. Determine verdict via join-parallel.sh
#   7. Call update-state.sh parallel-finish
#   8. Call update-state.sh parallel-clear
#
# Echoes verdict to stdout. All log/debug output goes to stderr.
run_parallel_state() {
    local plan_name="$1"
    local phase_name="$2"
    local state_name="$3"
    local workflow_file="$4"

    local results_dir="$PHASE_DIR/parallel_${state_name}"

    # Export STUB_DIR if set (allows test stubs to reference their directory)
    [[ -n "${STUB_DIR:-}" ]] && export STUB_DIR

    # Step 1-2: Read config and compute results_dir
    local branch_names
    branch_names=$(get_branch_names "$workflow_file" "$state_name")

    # Step 3: parallel-start
    # All sub-script stdout goes to stderr so only the final verdict reaches stdout
    if ! "$ARC_SCRIPTS_DIR/update-state.sh" "$plan_name" "$phase_name" parallel-start "$results_dir" "$branch_names" >&2; then
        return 1
    fi

    # Step 4: run-parallel.sh
    if ! "$ARC_SCRIPTS_DIR/run-parallel.sh" "$workflow_file" "$state_name" "$PHASE_DIR" "$plan_name" "$phase_name" >&2; then
        return 1
    fi

    # Step 5: Process .exit files
    for file in "$results_dir"/*.exit; do
        [[ -f "$file" ]] || continue
        local branch_name
        branch_name=$(basename "${file%.exit}")
        local exit_code
        exit_code=$(cat "$file")
        local status
        if [[ "$exit_code" -eq 0 ]]; then
            status="complete"
        elif [[ "$exit_code" -eq 124 ]]; then
            status="timeout"
        else
            status="failed"
        fi
        "$ARC_SCRIPTS_DIR/update-state.sh" "$plan_name" "$phase_name" parallel-update "$branch_name" "$status" "$exit_code" >&2
    done

    # Step 6: Determine verdict
    local strategy
    strategy=$(get_parallel_strategy "$workflow_file" "$state_name")
    local n_arg=""
    [[ "$strategy" == "n_of_m" ]] && n_arg=$(get_parallel_n "$workflow_file" "$state_name")
    local verdict
    verdict=$("$ARC_SCRIPTS_DIR/join-parallel.sh" "$strategy" "$results_dir" $n_arg) || return $?

    # Step 7: parallel-finish
    "$ARC_SCRIPTS_DIR/update-state.sh" "$plan_name" "$phase_name" parallel-finish "$verdict" >&2

    # Step 8: parallel-clear
    "$ARC_SCRIPTS_DIR/update-state.sh" "$plan_name" "$phase_name" parallel-clear >&2

    # Echo verdict to stdout
    echo "$verdict"
}

# =============================================================================
# END V5 PARALLEL HELPER FUNCTIONS
# =============================================================================

# run_iteration <plan_name> <phase_name> <state_name>
#
# Execute a single iteration with V4 features.
#
# Arguments:
#   plan_name   Name of the plan
#   phase_name  Name of the phase
#   state_name  Name of the current state
#
# Exit codes:
#   0  Iteration completed successfully
#   1  Iteration failed (constraint, action, or agent failure)
#   2  Human intervention requested
#
# Execution order:
#   1. Check intervention triggers (may exit 2)
#   2. Check escalation triggers (may execute actions)
#   3. Check pre-constraints (may exit 1)
#   4. Render and spawn agent
#   5. Check post-constraints (may exit 1)
#   6. Run after hooks (may execute actions)
#   7. Update state
run_iteration() {
    local plan_name="$1"
    local phase_name="$2"
    local state_name="$3"
    local verdict=""

    # 1. Check intervention triggers first
    # IMPORTANT: Capture exit code BEFORE the if statement.
    # Using `if ! cmd; then local exit_code=$?` is a bug — $? after `if !`
    # reflects the negation result (always 0 inside the if-block), not the
    # original command's exit code.
    local intervention_exit=0
    check_intervention_triggers "$WORKFLOW_FILE" || intervention_exit=$?
    if [[ $intervention_exit -ne 0 ]]; then
        if [[ $intervention_exit -eq 2 ]]; then
            echo "Intervention requested, halting iteration" >&2
            return 2
        fi
        return 1
    fi

    # Detect parallel state
    local is_parallel=""
    local has_parallel
    has_parallel=$(yq ".states[] | select(.name == \"$state_name\") | .parallel // \"\"" "$WORKFLOW_FILE")

    if [[ -n "$has_parallel" && "$has_parallel" != "null" ]]; then
        # PARALLEL PATH
        is_parallel=true

        # Replace steps 2-5 with single call to run_parallel_state
        # Capture exit code explicitly since set -e may be disabled by callers (e.g. bats)
        local parallel_exit=0
        verdict=$(run_parallel_state "$plan_name" "$phase_name" "$state_name" "$WORKFLOW_FILE") || parallel_exit=$?
        if [[ $parallel_exit -ne 0 ]]; then
            return $parallel_exit
        fi

        # Export for hook conditions (like line 387-388)
        export VERDICT="$verdict"
        export LAST_VERDICT="$verdict"

        # Set agent_failed=0 for parallel (error handling done in run_parallel_state)
        local agent_failed=0

    else
        # LINEAR PATH (existing steps 2-5)

    # 2. Check escalation triggers
    if ! check_escalation "$WORKFLOW_FILE" "$state_name"; then
        echo "Escalation action failed" >&2
        return 1
    fi

    # 3. Check pre-constraints
    if ! check_pre_constraints "$WORKFLOW_FILE" "$state_name" "$PHASE_DIR"; then
        echo "Pre-constraints not satisfied" >&2
        return 1
    fi

    # 4. Render prompt and spawn agent
    local iteration
    iteration=$(jq -r '.iteration // 0' "$STATE_FILE")
    local iter_dir="$PHASE_DIR/iteration_$(printf '%03d' "$iteration")"
    mkdir -p "$iter_dir"

    # Get prompt template
    local prompt_template
    prompt_template=$(yq ".states[] | select(.name == \"$state_name\") | .prompt" "$WORKFLOW_FILE" | tr -d '"')

    # Validate prompt template exists before copying/rendering
    if [[ ! -f "$ARC_HOME/$prompt_template" ]]; then
        echo "ERROR: Prompt template not found: $ARC_HOME/$prompt_template" >&2
        return 1
    fi

    # Build context and render template (V3 dependency)
    # If V3 scripts exist, use them. Otherwise fall back to simple copy.
    if [[ -f "$ARC_SCRIPTS_DIR/build-context.sh" && -f "$ARC_SCRIPTS_DIR/render_template.py" ]]; then
        local context
        context=$("$ARC_SCRIPTS_DIR/build-context.sh" "$STATE_FILE" "$WORKFLOW_FILE" "$PHASE_DIR" "$state_name")

        python3 "$ARC_SCRIPTS_DIR/render_template.py" \
            "$ARC_HOME/$prompt_template" \
            "$context" \
            "$ARC_HOME/prompts" \
            > "$iter_dir/prompt.md"
    else
        # Fallback: copy prompt file directly (no template rendering)
        cp "$ARC_HOME/$prompt_template" "$iter_dir/prompt.md"
    fi

    # Spawn agent
    # Track agent failure so we can still run post-checks but ultimately return failure.
    local agent_failed=0
    if ! spawn_agent "$iter_dir/prompt.md" "$iter_dir/output.txt"; then
        echo "Agent execution failed" >&2
        agent_failed=1
        # Continue to post-checks and hooks despite agent failure
    fi

    # 5. Extract verdict if this is a review state
    local verdicts
    verdicts=$(yq ".states[] | select(.name == \"$state_name\") | .verdicts // null" "$WORKFLOW_FILE")
    if [[ "$verdicts" != "null" && -f "$iter_dir/output.txt" ]]; then
        # Convert yq YAML array to comma-separated string for extract-verdict.sh
        local verdicts_csv
        verdicts_csv=$(yq -o=json ".states[] | select(.name == \"$state_name\") | .verdicts" "$WORKFLOW_FILE" | jq -r 'join(",")')
        verdict=$("$ARC_SCRIPTS_DIR/extract-verdict.sh" "$iter_dir/output.txt" "$verdicts_csv")
        echo "$verdict" > "$iter_dir/verdict.txt"
        # Export for hook conditions - both names for compatibility
        export VERDICT="$verdict"
        export LAST_VERDICT="$verdict"
    fi
    fi

    # 6. Check post-constraints
    if ! check_post_constraints "$WORKFLOW_FILE" "$state_name" "$PHASE_DIR"; then
        echo "Post-constraints not satisfied" >&2
        return 1
    fi

    # 7. Run after hooks
    if ! run_after_hooks "$WORKFLOW_FILE" "$state_name" "$verdict"; then
        echo "After hooks failed" >&2
        return 1
    fi

    # 8. Update state for next iteration
    # For parallel states, the verdict is used for state transition but NOT
    # written to verdicts_history/.last_verdict (parallel-finish already did that).
    # We perform the next-state lookup here, then pass empty verdict to
    # update_state_after_iteration so it only increments iteration and tracks stuck.
    if [[ -n "$is_parallel" ]]; then
        # Determine next state using the parallel verdict
        local next_type
        next_type=$(yq -o=json ".states[] | select(.name == \"$state_name\") | .next | type" "$WORKFLOW_FILE" | tr -d '"')
        local next_state=""
        if [[ "$next_type" == "!!str" ]]; then
            next_state=$(yq ".states[] | select(.name == \"$state_name\") | .next" "$WORKFLOW_FILE" | tr -d '"')
        elif [[ "$next_type" == "!!map" && -n "$verdict" ]]; then
            next_state=$(yq ".states[] | select(.name == \"$state_name\") | .next.$verdict // \"\"" "$WORKFLOW_FILE" | tr -d '"')
        fi
        if [[ -n "$next_state" && "$next_state" != "null" ]]; then
            jq --arg next "$next_state" '.current_state = $next' "$STATE_FILE" > "${STATE_FILE}.tmp.$$" \
                && mv "${STATE_FILE}.tmp.$$" "$STATE_FILE"
        fi
        # Pass empty verdict to skip verdicts_history/.last_verdict write
        update_state_after_iteration "$state_name" ""
    else
        update_state_after_iteration "$state_name" "$verdict"
    fi

    # Return failure if agent failed (post-checks/hooks still ran)
    return $agent_failed
}

# update_state_after_iteration <state_name> <verdict>
#
# Update state.json after an iteration completes.
#
# Arguments:
#   state_name  Name of the current state
#   verdict     Verdict from review (may be empty)
update_state_after_iteration() {
    local state_name="$1"
    local verdict="$2"

    local timestamp
    timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

    # Get current iteration before incrementing
    local current_iter
    current_iter=$(jq -r '.iteration // 0' "$STATE_FILE")

    # Increment iteration (use $$ PID to prevent race conditions)
    jq '.iteration = (.iteration // 0) + 1' "$STATE_FILE" > "${STATE_FILE}.tmp.$$" \
        && mv "${STATE_FILE}.tmp.$$" "$STATE_FILE"

    # Update current_state if verdict determines next state
    # Handle two cases:
    #   1. .next is a string (e.g., "complete") — always use it
    #   2. .next is an object (e.g., {approved: complete, needs_fix: impl}) — look up by verdict
    local next_state
    local next_type
    next_type=$(yq -o=json ".states[] | select(.name == \"$state_name\") | .next | type" "$WORKFLOW_FILE" | tr -d '"')

    if [[ "$next_type" == "!!str" ]]; then
        # Simple string: always go to this state
        next_state=$(yq ".states[] | select(.name == \"$state_name\") | .next" "$WORKFLOW_FILE" | tr -d '"')
    elif [[ "$next_type" == "!!map" ]]; then
        # Object: look up by verdict, fall back to no transition
        if [[ -n "$verdict" ]]; then
            next_state=$(yq ".states[] | select(.name == \"$state_name\") | .next.$verdict // \"\"" "$WORKFLOW_FILE" | tr -d '"')
        else
            next_state=""
        fi
    else
        next_state=""
    fi

    if [[ -n "$next_state" && "$next_state" != "null" ]]; then
        jq --arg next "$next_state" '.current_state = $next' "$STATE_FILE" > "${STATE_FILE}.tmp.$$" \
            && mv "${STATE_FILE}.tmp.$$" "$STATE_FILE"
    fi

    # Update stuck_iterations counter
    # If next_state loops back to the same state (or no transition), increment stuck_iterations.
    # If transitioning to a different state, reset stuck_iterations to 0.
    if [[ -z "$next_state" || "$next_state" == "null" || "$next_state" == "$state_name" ]]; then
        # No progress — increment stuck_iterations
        jq '.stuck_iterations = ((.stuck_iterations // 0) + 1)' \
            "$STATE_FILE" > "${STATE_FILE}.tmp.$$" && mv "${STATE_FILE}.tmp.$$" "$STATE_FILE"
    else
        # Progress — reset stuck_iterations
        jq '.stuck_iterations = 0' \
            "$STATE_FILE" > "${STATE_FILE}.tmp.$$" && mv "${STATE_FILE}.tmp.$$" "$STATE_FILE"
    fi

    # Add verdict to history as object (per STATE_SCHEMA.md)
    # Structure: { iteration, state, verdict, timestamp }
    if [[ -n "$verdict" ]]; then
        jq --arg v "$verdict" \
           --arg s "$state_name" \
           --argjson i "$current_iter" \
           --arg t "$timestamp" \
           '.verdicts_history = ((.verdicts_history // []) + [{
               "iteration": $i,
               "state": $s,
               "verdict": $v,
               "timestamp": $t
           }]) |
           .last_verdict = $v' \
           "$STATE_FILE" > "${STATE_FILE}.tmp.$$" && mv "${STATE_FILE}.tmp.$$" "$STATE_FILE"
    fi
}

# =============================================================================
# END V4 FUNCTIONS
# =============================================================================

# If this script is being sourced (not executed directly), stop here.
# This allows tests and other scripts to source iterate.sh to get function
# definitions (setup_v4_environment, run_iteration, update_state_after_iteration,
# spawn_agent) without executing the main body.
if [[ "${BASH_SOURCE[0]}" != "${0}" ]]; then
    return 0 2>/dev/null || true
fi

# Sub-agent timeout (10 minutes default, configurable via env)
SUB_AGENT_TIMEOUT="${SUB_AGENT_TIMEOUT:-600}"

# Test execution timeout (5 minutes default, configurable via env)
TEST_TIMEOUT="${TEST_TIMEOUT:-300}"

# Save starting state for potential rollback
STARTING_SHA=$(git rev-parse HEAD 2>/dev/null || echo "unknown")

# Check sub-agent exit code and handle failures
check_exit_code() {
    local exit_code=$1

    # Success - clear counters
    if [[ $exit_code -eq 0 ]]; then
        "$ARC_SCRIPTS_DIR/update-state.sh" "$PLAN_NAME" "$PHASE" clear-hangs 2>/dev/null || true
        "$ARC_SCRIPTS_DIR/update-state.sh" "$PLAN_NAME" "$PHASE" clear-crashes 2>/dev/null || true
        return 0
    fi

    echo ""
    echo "WARNING: Files may be in a partial state."
    echo ""
    echo "To check changes:  git diff"
    echo "To keep changes:   git stash"
    echo "To discard all:    git checkout ."
    echo "To full reset:     git reset --hard $STARTING_SHA"
    echo ""

    if [[ $exit_code -eq 124 ]]; then
        echo "TIMEOUT: Sub-agent hung after ${SUB_AGENT_TIMEOUT}s"
        "$ARC_SCRIPTS_DIR/update-state.sh" "$PLAN_NAME" "$PHASE" record-hang
        echo ""
        echo "Orchestrator: Read impl_reasoning.md and provide targeted instructions"
        exit 124
    else
        echo "CRASH: Sub-agent failed unexpectedly (exit code: $exit_code)"
        echo ""
        echo "Cleaning up tracked process groups..."
        kill_tracked_pgids
        "$ARC_SCRIPTS_DIR/update-state.sh" "$PLAN_NAME" "$PHASE" record-crash
        echo ""
        echo "Orchestrator: Check state and retry the iteration"
        exit $exit_code
    fi
}

# Legacy alias for compatibility
check_for_hang() {
    check_exit_code "$1"
}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ARC_HOME="${ARC_HOME:-$(dirname "$SCRIPT_DIR")}"
ARC_PROJECT_ROOT="${ARC_PROJECT_ROOT:-$(pwd)}"
ARC_SCRIPTS_DIR="$SCRIPT_DIR"
ARC_PLANS_DIR="${ARC_PLANS_DIR:-$ARC_PROJECT_ROOT/.plans}"
ACTIVE_DIR="$ARC_PLANS_DIR/active"

# Load project config
source "$ARC_SCRIPTS_DIR/config.sh"

PLAN_NAME="${1:-}"
PHASE="${2:-}"
MODE="${3:-impl}"
ORCHESTRATOR_INSTRUCTIONS="${4:-}"

if [[ -z "$PLAN_NAME" || -z "$PHASE" ]]; then
    echo "Usage: iterate.sh <plan-name> <phase> [qa|impl|fix] [orchestrator-instructions]"
    echo ""
    echo "Arguments:"
    echo "  plan-name                 Name of the plan (folder in active/)"
    echo "  phase                     Phase to work on"
    echo "  mode                      qa, impl, or fix (default: impl)"
    echo "  orchestrator-instructions Optional instructions from orchestrator (impl mode, iteration 1 only)"
    echo ""
    echo "Sub-agents do NOT commit. Orchestrator commits at boundaries."
    exit 1
fi

PLAN_DIR="$ACTIVE_DIR/$PLAN_NAME"
PHASE_DIR="$PLAN_DIR/phases/$PHASE"
STATE_FILE="$PHASE_DIR/state.json"
PLAN_FILE="$PHASE_DIR/plan.md"
SESSION_FILE="$PLAN_DIR/session_id"
WORKFLOWS_DIR="$ARC_HOME/workflows"

# Clean up any zombie cargo processes from previous iterations
[[ -x "$ARC_SCRIPTS_DIR/kill-zombie-cargo.sh" ]] && "$ARC_SCRIPTS_DIR/kill-zombie-cargo.sh" --max-age 3 2>/dev/null || true

# =============================================================================
# WORKFLOW DETECTION (V1a)
# =============================================================================
# Check for workflow file in order of precedence:
# 1. Plan-specific workflow: $PLAN_DIR/workflow.yaml
# 2. Base workflow by type: $WORKFLOWS_DIR/<type>.yaml (type from plan metadata)
# 3. Default: $WORKFLOWS_DIR/feature.yaml
# =============================================================================

detect_workflow() {
    # 1. Check for plan-specific workflow
    if [[ -f "$PLAN_DIR/workflow.yaml" ]]; then
        echo "$PLAN_DIR/workflow.yaml"
        return 0
    fi

    # 2. Check for type in state.json or plan.md frontmatter
    local workflow_type=""
    if [[ -f "$STATE_FILE" ]]; then
        workflow_type=$(jq -r '.workflow_type // ""' "$STATE_FILE" 2>/dev/null)
    fi

    # 3. If type found, use corresponding base workflow
    if [[ -n "$workflow_type" && -f "$WORKFLOWS_DIR/${workflow_type}.yaml" ]]; then
        echo "$WORKFLOWS_DIR/${workflow_type}.yaml"
        return 0
    fi

    # 4. Default to feature workflow if it exists
    if [[ -f "$WORKFLOWS_DIR/feature.yaml" ]]; then
        echo "$WORKFLOWS_DIR/feature.yaml"
        return 0
    fi

    # No workflow found
    return 1
}

# Detect workflow (empty if not found - backwards compatibility)
WORKFLOW_FILE=""
if detected=$(detect_workflow 2>/dev/null); then
    WORKFLOW_FILE="$detected"
fi

# =============================================================================

# Verify plan exists
if [[ ! -d "$PLAN_DIR" ]]; then
    echo "Error: Plan '$PLAN_NAME' not found at $PLAN_DIR"
    echo "Run init-plan.sh first to create the plan."
    exit 1
fi

# Verify phase exists
if [[ ! -d "$PHASE_DIR" ]]; then
    echo "Error: Phase '$PHASE' not found in plan '$PLAN_NAME'"
    echo "Available phases:"
    ls "$PLAN_DIR/phases/" 2>/dev/null || echo "  (none)"
    exit 1
fi

# Validate plan.md has required sections
validate_plan() {
    local plan_file="$1"
    local errors=()

    if [[ ! -f "$plan_file" ]]; then
        echo "Error: Plan file not found: $plan_file"
        return 1
    fi

    # Check for required sections
    if ! grep -q "^## Objective" "$plan_file"; then
        errors+=("Missing '## Objective' section")
    fi

    if ! grep -q "^## Files" "$plan_file"; then
        errors+=("Missing '## Files' section")
    fi

    if ! grep -q "^## Test Cases" "$plan_file"; then
        errors+=("Missing '## Test Cases' section")
    fi

    # Check that Objective is not placeholder text
    # Match [TODO, TODO:, [FIXME, FIXME: as placeholders, but not TODO/FIXME as prose substrings
    if grep -qE "\[One sentence describing|\[TODO|^TODO:|^\s*TODO:|\[FIXME|^FIXME:|^\s*FIXME:" "$plan_file"; then
        errors+=("Plan contains placeholder text - fill in all sections")
    fi

    if [[ ${#errors[@]} -gt 0 ]]; then
        echo "Error: Invalid plan.md - missing required content:"
        for err in "${errors[@]}"; do
            echo "  - $err"
        done
        echo ""
        echo "See $ARC_HOME/templates/plan-template.md for required structure"
        return 1
    fi

    return 0
}

# Only validate on first run of qa or impl (not on reviews or fix)
if [[ "$MODE" =~ ^(qa|impl)$ ]]; then
    if ! validate_plan "$PLAN_FILE"; then
        exit 1
    fi
fi

# Check phase dependencies before starting
check_dependencies() {
    local plan_dir="$1"
    local phase="$2"
    local plan_json="$plan_dir/plan.json"

    [[ -f "$plan_json" ]] || return 0  # No plan.json = no dependencies to check

    # Get dependencies for this phase
    local deps
    deps=$(jq -r ".dependencies[\"$phase\"] // [] | .[]" "$plan_json" 2>/dev/null)
    [[ -z "$deps" ]] && return 0  # No dependencies

    local blocked_by=()
    for dep in $deps; do
        local dep_state="$plan_dir/phases/$dep/state.json"
        if [[ -f "$dep_state" ]]; then
            local dep_status
            dep_status=$(jq -r '.phase_status // "pending"' "$dep_state")
            if [[ "$dep_status" != "complete" ]]; then
                blocked_by+=("$dep ($dep_status)")
            fi
        else
            blocked_by+=("$dep (no state)")
        fi
    done

    if [[ ${#blocked_by[@]} -gt 0 ]]; then
        echo "Error: Phase '$phase' has incomplete dependencies:"
        for b in "${blocked_by[@]}"; do
            echo "  - $b"
        done
        echo ""
        echo "Complete these phases first before starting '$phase'."
        return 1
    fi

    return 0
}

if ! check_dependencies "$PLAN_DIR" "$PHASE"; then
    exit 1
fi

# Session ID (for display only - sub-agents run independently)
SESSION_ID="standalone"
if [[ -f "$SESSION_FILE" ]]; then
    SESSION_ID=$(cat "$SESSION_FILE")
fi

# Read current state
CURRENT_STATUS=$(jq -r '.phase_status' "$STATE_FILE" 2>/dev/null || echo "pending")
ITER_CURRENT=$(jq -r '.iteration.current' "$STATE_FILE" 2>/dev/null || echo "0")
ITER_MAX=$(jq -r '.iteration.max' "$STATE_FILE" 2>/dev/null || echo "25")
PACKAGES=$(jq -r '.packages // [] | join(",")' "$STATE_FILE" 2>/dev/null || echo "")
STUCK_ITERATIONS=$(jq -r '.stuck_iterations // 0' "$STATE_FILE" 2>/dev/null || echo "0")
TESTS_PASSING=$(jq -r '.tests_passing // 0' "$STATE_FILE" 2>/dev/null || echo "0")
TESTS_TOTAL=$(jq -r '.tests_total // 0' "$STATE_FILE" 2>/dev/null || echo "0")

# Pre-flight checks
if [[ "$CURRENT_STATUS" == "complete" ]]; then
    echo "Phase $PHASE is already complete."
    exit 0
fi

if [[ "$CURRENT_STATUS" == "disputed" && "$MODE" != "fix" ]]; then
    echo "Error: Phase $PHASE has an unresolved dispute."
    echo "Resolve with: iterate.sh $PLAN_NAME $PHASE fix"
    exit 1
fi

if [[ "$MODE" == "impl" && "$ITER_CURRENT" -ge "$ITER_MAX" ]]; then
    echo "Error: Hit iteration limit ($ITER_MAX)"
    exit 1
fi

# Global iteration limit: cap total iterate.sh calls per phase across ALL modes.
# This prevents infinite loops in qa, qa-review, impl-review, and fix modes which
# otherwise have no iteration limits. The impl ITER_MAX above is a stricter per-mode
# limit; this is the safety net for everything else.
GLOBAL_ITER_MAX="${GLOBAL_ITER_MAX:-50}"
GLOBAL_ITER_CURRENT=$(jq -r '.global_iterations // 0' "$STATE_FILE" 2>/dev/null || echo "0")
if [[ "$GLOBAL_ITER_CURRENT" -ge "$GLOBAL_ITER_MAX" ]]; then
    echo "Error: Hit global iteration limit ($GLOBAL_ITER_MAX) across all modes"
    echo "This phase has consumed $GLOBAL_ITER_CURRENT total iterations (qa + impl + review + fix)."
    echo "Override with: GLOBAL_ITER_MAX=100 iterate.sh ..."
    "$ARC_SCRIPTS_DIR/update-state.sh" "$PLAN_NAME" "$PHASE" status blocked 2>/dev/null || true
    exit 1
fi
# Increment global iteration counter
jq '.global_iterations = ((.global_iterations // 0) + 1)' "$STATE_FILE" > "${STATE_FILE}.tmp.$$" \
    && mv "${STATE_FILE}.tmp.$$" "$STATE_FILE"

if [[ "$MODE" == "fix" ]]; then
    APPROVED_COUNT=$(jq '[.disputes // [] | .[] | select(.resolution == "approved")] | length' "$STATE_FILE" 2>/dev/null)
    if [[ "$APPROVED_COUNT" -eq 0 ]]; then
        echo "Error: No approved disputes to fix"
        exit 1
    fi
    echo "Approved disputes to fix: $APPROVED_COUNT"
fi

# Update state before running
if [[ "$MODE" == "impl" ]]; then
    "$ARC_SCRIPTS_DIR/update-state.sh" "$PLAN_NAME" "$PHASE" increment-iteration
    ITER_CURRENT=$((ITER_CURRENT + 1))
fi

# =============================================================================
# ESCALATION LADDER (impl mode only)
# =============================================================================
# stuck=0-2: Normal iteration
# stuck=3:   Spawn sonnet investigate agent (read-only diagnosis)
# stuck=4:   Spawn sonnet fix agent (targeted fix from investigation)
# stuck=5:   Spawn opus combined investigate+fix agent (single call)
# stuck=6+:  Attempt auto-split of failing tests
# =============================================================================

if [[ "$MODE" == "impl" && $STUCK_ITERATIONS -ge 3 ]]; then
    echo ""
    echo "=== ESCALATION LADDER ACTIVATED ==="
    echo "Stuck iterations: $STUCK_ITERATIONS"
    echo "Tests: $TESTS_PASSING/$TESTS_TOTAL"

    # Extract failing tests info for context
    LAST_OUTPUT="$PHASE_DIR/last_test_output.txt"
    FAILING_TESTS=""
    if [[ -f "$LAST_OUTPUT" ]]; then
        FAILING_TESTS=$(grep -E "FAIL|FAILED|error\[" "$LAST_OUTPUT" 2>/dev/null | head -10 || true)
    fi

    # Build escalation context (used by levels 3, 4, and 5)
    ESCALATION_CONTEXT="## Escalation Context

This phase has been stuck for $STUCK_ITERATIONS iterations with $TESTS_PASSING/$TESTS_TOTAL tests passing.
The impl agent cannot make further progress.

### Failing Tests
\`\`\`
$FAILING_TESTS
\`\`\`

### Previous Reasoning
Read \`$PHASE_DIR/impl_reasoning.md\` for what the impl agent has tried so far.

---

"

    # Level 3: Spawn investigate agent (read-only, sonnet)
    if [[ $STUCK_ITERATIONS -eq 3 ]]; then
        echo "Escalation: Spawning investigate agent (read-only, sonnet)..."
        echo "=================================="
        echo ""

        PROMPT=$(python3 "$ARC_SCRIPTS_DIR/render_template.py" \
            "$ARC_HOME/prompts/bugfix/investigate.md" \
            "$(jq -n --arg plan_name "$PLAN_NAME" --arg phase "$PHASE" \
                --arg plan_file "$PLAN_FILE" --arg phase_dir "$PHASE_DIR" \
                '{plan_name: $plan_name, phase: $phase, plan_file: $plan_file, phase_dir: $phase_dir}')" \
            "$ARC_HOME/prompts")

        # Prepend escalation context to the loaded prompt
        PROMPT="${ESCALATION_CONTEXT}${PROMPT}"

        run_with_timeout $SUB_AGENT_TIMEOUT claude --model sonnet -p "$PROMPT" \
            --allowedTools "Read,Glob,Grep,Bash,Write" \
            --max-turns 15

        EXIT_CODE=$?
        check_exit_code $EXIT_CODE || true

        echo ""
        echo "Investigation complete. Results at: $PHASE_DIR/investigation.md"
        echo "Next iteration will spawn the fix agent."
        echo ""
        "$ARC_SCRIPTS_DIR/get-state.sh" "$PLAN_NAME" "$PHASE"
        kill_tracked_pgids
        exit 0
    fi

    # Level 4: Spawn fix agent (sonnet)
    if [[ $STUCK_ITERATIONS -eq 4 ]]; then
        echo "Escalation: Spawning fix agent (sonnet)..."
        echo "=================================="
        echo ""

        PROMPT=$(python3 "$ARC_SCRIPTS_DIR/render_template.py" \
            "$ARC_HOME/prompts/bugfix/fix.md" \
            "$(jq -n --arg plan_name "$PLAN_NAME" --arg phase "$PHASE" \
                --arg plan_file "$PLAN_FILE" --arg phase_dir "$PHASE_DIR" \
                --arg scripts_dir "$ARC_SCRIPTS_DIR" \
                '{plan_name: $plan_name, phase: $phase, plan_file: $plan_file, phase_dir: $phase_dir, scripts_dir: $scripts_dir}')" \
            "$ARC_HOME/prompts")

        run_with_timeout $SUB_AGENT_TIMEOUT claude --model sonnet -p "$PROMPT" \
            --allowedTools "View,Edit,Write,Bash" \
            --max-turns 15

        EXIT_CODE=$?
        check_exit_code $EXIT_CODE || true

        # Run tests to check whether the fix worked
        echo ""
        echo "Running phase tests after fix..."
        "$ARC_SCRIPTS_DIR/run-phase-tests.sh" "$PLAN_NAME" "$PHASE" || true

        echo ""
        "$ARC_SCRIPTS_DIR/get-state.sh" "$PLAN_NAME" "$PHASE"
        kill_tracked_pgids
        exit 0
    fi

    # Level 5: Opus combined investigate+fix
    if [[ $STUCK_ITERATIONS -eq 5 ]]; then
        echo "Escalation: Spawning opus combined investigate+fix agent..."
        echo "=================================="
        echo ""

        INVESTIGATE_PROMPT=$(python3 "$ARC_SCRIPTS_DIR/render_template.py" \
            "$ARC_HOME/prompts/bugfix/investigate.md" \
            "$(jq -n --arg plan_name "$PLAN_NAME" --arg phase "$PHASE" \
                --arg plan_file "$PLAN_FILE" --arg phase_dir "$PHASE_DIR" \
                '{plan_name: $plan_name, phase: $phase, plan_file: $plan_file, phase_dir: $phase_dir}')" \
            "$ARC_HOME/prompts")

        FIX_PROMPT=$(python3 "$ARC_SCRIPTS_DIR/render_template.py" \
            "$ARC_HOME/prompts/bugfix/fix.md" \
            "$(jq -n --arg plan_name "$PLAN_NAME" --arg phase "$PHASE" \
                --arg plan_file "$PLAN_FILE" --arg phase_dir "$PHASE_DIR" \
                --arg scripts_dir "$ARC_SCRIPTS_DIR" \
                '{plan_name: $plan_name, phase: $phase, plan_file: $plan_file, phase_dir: $phase_dir, scripts_dir: $scripts_dir}')" \
            "$ARC_HOME/prompts")

        PROMPT="${ESCALATION_CONTEXT}${INVESTIGATE_PROMPT}

---

After completing your investigation, immediately apply fixes. You have full write access.

${FIX_PROMPT}"

        run_with_timeout $SUB_AGENT_TIMEOUT claude --model opus -p "$PROMPT" \
            --allowedTools "Read,Glob,Grep,View,Edit,Write,Bash" \
            --max-turns 25

        EXIT_CODE=$?
        check_exit_code $EXIT_CODE || true

        echo ""
        echo "Running phase tests after combined investigate+fix..."
        "$ARC_SCRIPTS_DIR/run-phase-tests.sh" "$PLAN_NAME" "$PHASE" || true

        echo ""
        "$ARC_SCRIPTS_DIR/get-state.sh" "$PLAN_NAME" "$PHASE"
        kill_tracked_pgids
        exit 0
    fi

    # Level 6+: Attempt auto-split
    if [[ $STUCK_ITERATIONS -ge 6 ]]; then
        echo "Escalation: Attempting auto-split of failing tests..."

        # Check if auto-split script exists
        if [[ -x "$ARC_SCRIPTS_DIR/auto-split-stuck.sh" ]]; then
            if "$ARC_SCRIPTS_DIR/auto-split-stuck.sh" "$PLAN_NAME" "$PHASE"; then
                echo ""
                echo "=========================================="
                echo "AUTO-SPLIT SUCCESSFUL"
                echo "=========================================="
                echo "Phase '$PHASE' has been split into sub-phases."
                echo "The original phase is now marked as 'split'."
                echo ""
                echo "Run status.sh to see the new sub-phases:"
                echo "  $ARC_SCRIPTS_DIR/status.sh $PLAN_NAME"
                echo ""
                echo "Continue with the first sub-phase."
                echo "=========================================="
                exit 0
            else
                echo "Auto-split failed or not applicable. Continuing with escalated impl..."
            fi
        else
            echo "Auto-split script not found. Continuing with escalated impl..."
        fi
    fi


    echo "=================================="
    echo ""
fi

echo "=== Spawning sub-agent ==="
echo "Plan: $PLAN_NAME"
echo "Phase: $PHASE"
echo "Mode: $MODE"
echo "Iteration: $ITER_CURRENT/$ITER_MAX"
echo "Packages: ${PACKAGES:-not set}"
echo "Session: $SESSION_ID"
if [[ -n "$WORKFLOW_FILE" ]]; then
    echo "Workflow: $(basename "$WORKFLOW_FILE")"
fi
if [[ -n "$ORCHESTRATOR_INSTRUCTIONS" ]]; then
    echo "Orchestrator instructions: YES (structural setup)"
fi
echo "==========================="

# Track if impl-review approved (used for completion verification)
IMPL_REVIEW_APPROVED=false

case "$MODE" in
    qa)
        # Check if qa_review.md exists (feedback from previous QA run)
        QA_REVIEW_INSTRUCTION=""
        if [[ -f "$PHASE_DIR/qa_review.md" ]]; then
            QA_REVIEW_INSTRUCTION="
## CRITICAL: Fix Review Feedback First

A qa_review.md file exists at $PHASE_DIR/qa_review.md - this means your previous test iteration had gaps.

**BEFORE doing anything else:**
1. Read $PHASE_DIR/qa_review.md
2. Check the verdict (APPROVED or GAPS_FOUND)
3. If GAPS_FOUND, focus ONLY on fixing the identified gaps
4. Address every item listed in the 'Verdict' section

Common gaps to fix:
- Empty test bodies (add assertions)
- Weak conditional assertions (replace 'if fit.valid' with definitive asserts)
- Missing edge case coverage
- Syntax errors

DO NOT rewrite tests that are already good. ONLY fix the gaps identified in the review.
"
        fi

        # Set status to qa
        "$ARC_SCRIPTS_DIR/update-state.sh" "$PLAN_NAME" "$PHASE" status qa

        # Build context and render prompt
        CONTEXT=$("$ARC_SCRIPTS_DIR/build-context.sh" "$STATE_FILE" "$WORKFLOW_FILE" "$PHASE_DIR" "qa")

        # Record that this state type received plan_md (if it was non-empty)
        STATE_TYPE="qa"
        if echo "$CONTEXT" | jq -e '.plan_md != ""' > /dev/null 2>&1; then
            jq --arg s "$STATE_TYPE" \
                '.plan_md_sent_to = ((.plan_md_sent_to // []) | if map(. == $s) | any then . else . + [$s] end)' \
                "$STATE_FILE" > "${STATE_FILE}.tmp.$$" && mv "${STATE_FILE}.tmp.$$" "$STATE_FILE"
        fi

        PROMPT=$(python3 "$ARC_SCRIPTS_DIR/render_template.py" \
            "$ARC_HOME/prompts/feature/qa.md" "$CONTEXT" "$ARC_HOME/prompts")

        run_with_timeout $SUB_AGENT_TIMEOUT claude -p "$PROMPT" \
            --allowedTools "View,Edit,Write,Bash" \
            --max-turns 25

        EXIT_CODE=$?
        check_for_hang $EXIT_CODE || true
        ;;

    impl)
        # Require packages to be set (QA should have set this)
        if [[ -z "$PACKAGES" ]]; then
            echo "Error: No packages set in state. QA must set packages before impl."
            echo "Set packages with: jq '.packages = [\"my-package\"]' $STATE_FILE > tmp && mv tmp $STATE_FILE"
            exit 1
        fi

        # Set status to implementing
        "$ARC_SCRIPTS_DIR/update-state.sh" "$PLAN_NAME" "$PHASE" status implementing

        # Build orchestrator section if instructions provided
        ORCH_SECTION=""
        if [[ -n "$ORCHESTRATOR_INSTRUCTIONS" ]]; then
            ORCH_SECTION="## Orchestrator Instructions (DO THIS FIRST)

The orchestrator has identified structural changes from the plan that must be done
before running tests. Complete these steps first, then proceed to TDD.

$ORCHESTRATOR_INSTRUCTIONS

After completing the above, proceed to the standard TDD workflow below.

---

"
        fi

        # Check if disputes were just cleared - inject context so impl knows
        DISPUTE_SECTION=""
        CLEARED_COUNT=$(jq '.last_cleared_disputes // [] | length' "$STATE_FILE" 2>/dev/null)
        if [[ "$CLEARED_COUNT" -gt 0 ]]; then
            CLEARED_TESTS=$(jq -r '.last_cleared_disputes // [] | .[].test_name' "$STATE_FILE" 2>/dev/null | tr '\n' ', ' | sed 's/,$//')
            DISPUTE_SECTION="## Recent Dispute Resolution

$CLEARED_COUNT dispute(s) were just resolved. Fixed tests: **$CLEARED_TESTS**

These tests have been corrected by the fix agent. You should now be able to pass them.
Do NOT file another dispute on these tests unless there's a NEW issue.

---

"
        fi

        # Inject investigation findings if they exist
        INVESTIGATION_SECTION=""
        if [[ -f "$PHASE_DIR/investigation.md" ]]; then
            INVESTIGATION_SECTION="## Investigation Findings

An investigation agent analyzed this phase and found:

$(cat "$PHASE_DIR/investigation.md")

Use these findings to guide your implementation approach.

---

"
        fi

        # Build context and render prompt
        CONTEXT=$("$ARC_SCRIPTS_DIR/build-context.sh" "$STATE_FILE" "$WORKFLOW_FILE" "$PHASE_DIR" "impl")

        # Record that this state type received plan_md (if it was non-empty)
        STATE_TYPE="impl"
        if echo "$CONTEXT" | jq -e '.plan_md != ""' > /dev/null 2>&1; then
            jq --arg s "$STATE_TYPE" \
                '.plan_md_sent_to = ((.plan_md_sent_to // []) | if map(. == $s) | any then . else . + [$s] end)' \
                "$STATE_FILE" > "${STATE_FILE}.tmp.$$" && mv "${STATE_FILE}.tmp.$$" "$STATE_FILE"
        fi

        PROMPT=$(python3 "$ARC_SCRIPTS_DIR/render_template.py" \
            "$ARC_HOME/prompts/feature/impl.md" "$CONTEXT" "$ARC_HOME/prompts")

        PROMPT="${ORCH_SECTION}${DISPUTE_SECTION}${INVESTIGATION_SECTION}${PROMPT}"

        run_with_timeout $SUB_AGENT_TIMEOUT claude -p "$PROMPT" \
            --allowedTools "View,Edit,Write,Bash" \
            --max-turns 15

        EXIT_CODE=$?
        check_for_hang $EXIT_CODE || true
        ;;

    fix)
        DISPUTE_COUNT=$(jq '.disputes // [] | length' "$STATE_FILE")
        DISPUTE_LIST=$(jq -r '.disputes // [] | .[] | "- \(.test_name): \(.reason)"' "$STATE_FILE")

        # Render prompt with manually-constructed context (fix.md uses variables not in build-context.sh)
        PROMPT=$(python3 "$ARC_SCRIPTS_DIR/render_template.py" \
            "$ARC_HOME/prompts/feature/fix.md" \
            "$(jq -n --arg plan_name "$PLAN_NAME" --arg phase "$PHASE" \
                --arg state_file "$STATE_FILE" --arg plan_file "$PLAN_FILE" \
                --arg scripts_dir "$ARC_SCRIPTS_DIR" --arg dispute_count "$DISPUTE_COUNT" \
                --arg dispute_list "$DISPUTE_LIST" \
                '{plan_name: $plan_name, phase: $phase, state_file: $state_file, plan_file: $plan_file, scripts_dir: $scripts_dir, dispute_count: $dispute_count, dispute_list: $dispute_list}')" \
            "$ARC_HOME/prompts")

        run_with_timeout $SUB_AGENT_TIMEOUT claude -p "$PROMPT" \
            --allowedTools "View,Edit,Write,Bash" \
            --max-turns 15

        EXIT_CODE=$?
        check_for_hang $EXIT_CODE || true
        ;;

    qa-review)
        # Set status to qa_review
        "$ARC_SCRIPTS_DIR/update-state.sh" "$PLAN_NAME" "$PHASE" status qa_review

        QA_REVIEW="$PHASE_DIR/qa_review.md"

        # Build context and render prompt
        CONTEXT=$("$ARC_SCRIPTS_DIR/build-context.sh" "$STATE_FILE" "$WORKFLOW_FILE" "$PHASE_DIR" "qa_review")

        # Record that this state type received plan_md (if it was non-empty)
        STATE_TYPE="qa_review"
        if echo "$CONTEXT" | jq -e '.plan_md != ""' > /dev/null 2>&1; then
            jq --arg s "$STATE_TYPE" \
                '.plan_md_sent_to = ((.plan_md_sent_to // []) | if map(. == $s) | any then . else . + [$s] end)' \
                "$STATE_FILE" > "${STATE_FILE}.tmp.$$" && mv "${STATE_FILE}.tmp.$$" "$STATE_FILE"
        fi

        PROMPT=$(python3 "$ARC_SCRIPTS_DIR/render_template.py" \
            "$ARC_HOME/prompts/feature/qa-review.md" "$CONTEXT" "$ARC_HOME/prompts")

        REVIEW_AGENT=1 REVIEW_OUTPUT_FILE="$QA_REVIEW" run_with_timeout $SUB_AGENT_TIMEOUT claude -p "$PROMPT" \
            --allowedTools "Read,Glob,Grep,Write" \
            --max-turns 15

        EXIT_CODE=$?
        check_for_hang $EXIT_CODE || true

        # Mark QA as reviewed
        "$ARC_SCRIPTS_DIR/update-state.sh" "$PLAN_NAME" "$PHASE" mark-qa-reviewed
        ;;

    impl-review)
        # Check that impl_reasoning.md exists (optional — review can proceed without it)
        IMPL_REASONING="$PHASE_DIR/impl_reasoning.md"
        if [[ ! -f "$IMPL_REASONING" ]]; then
            echo "WARNING: No impl_reasoning.md found at $IMPL_REASONING"
            echo "Impl-review will proceed without implementation reasoning context."
        fi

        # Set status to impl_review
        "$ARC_SCRIPTS_DIR/update-state.sh" "$PLAN_NAME" "$PHASE" status impl_review

        IMPL_REVIEW="$PHASE_DIR/impl_review.md"

        # Build context and render prompt
        CONTEXT=$("$ARC_SCRIPTS_DIR/build-context.sh" "$STATE_FILE" "$WORKFLOW_FILE" "$PHASE_DIR" "impl_review")

        # Record that this state type received plan_md (if it was non-empty)
        STATE_TYPE="impl_review"
        if echo "$CONTEXT" | jq -e '.plan_md != ""' > /dev/null 2>&1; then
            jq --arg s "$STATE_TYPE" \
                '.plan_md_sent_to = ((.plan_md_sent_to // []) | if map(. == $s) | any then . else . + [$s] end)' \
                "$STATE_FILE" > "${STATE_FILE}.tmp.$$" && mv "${STATE_FILE}.tmp.$$" "$STATE_FILE"
        fi

        PROMPT=$(python3 "$ARC_SCRIPTS_DIR/render_template.py" \
            "$ARC_HOME/prompts/feature/impl-review.md" "$CONTEXT" "$ARC_HOME/prompts")

        REVIEW_AGENT=1 REVIEW_OUTPUT_FILE="$IMPL_REVIEW" run_with_timeout $SUB_AGENT_TIMEOUT claude -p "$PROMPT" \
            --allowedTools "Read,Glob,Grep,Write" \
            --max-turns 15

        EXIT_CODE=$?
        check_for_hang $EXIT_CODE || true

        # Mark this iteration as reviewed
        "$ARC_SCRIPTS_DIR/update-state.sh" "$PLAN_NAME" "$PHASE" mark-reviewed

        # Check impl_review verdict - completion will be verified after tests run
        IMPL_REVIEW="$PHASE_DIR/impl_review.md"
        IMPL_REVIEW_APPROVED=false
        if [[ -f "$IMPL_REVIEW" ]]; then
            if grep -q "APPROVED" "$IMPL_REVIEW"; then
                IMPL_REVIEW_APPROVED=true
                echo ""
                echo "Impl-review APPROVED. Will verify completion after tests run."
            else
                echo ""
                echo "WARNING: impl-review did not approve. Check $IMPL_REVIEW for concerns."
                echo "Continue implementation to address concerns, or commit if acceptable."
            fi
        fi
        ;;

    *)
        echo "Unknown mode: $MODE (use 'qa', 'impl', 'fix', 'qa-review', or 'impl-review')"
        exit 1
        ;;
esac

# Post-iteration
echo ""
echo "=== Sub-agent finished (exit code: $EXIT_CODE) ==="
echo ""
echo "Working tree changes (uncommitted):"
git status --short 2>/dev/null || echo "(not a git repo)"
echo ""

# Run phase-specific tests and update state
if [[ "$MODE" =~ ^(impl|impl-review|fix)$ ]]; then
    echo ""
    echo "Running phase tests..."
    TEST_EXIT=0
    "$ARC_SCRIPTS_DIR/run-phase-tests.sh" "$PLAN_NAME" "$PHASE" || TEST_EXIT=$?
    if [[ $TEST_EXIT -eq 2 ]]; then
        echo ""
        echo "WARNING: No test files registered for this phase."
        echo "Tests must be registered before the phase can complete."
        echo "Register with: $ARC_SCRIPTS_DIR/update-state.sh $PLAN_NAME $PHASE add-test-file <path>"
    fi
fi

# Verify and set completion status AFTER tests run (Option A + D)
if [[ "$IMPL_REVIEW_APPROVED" == "true" ]]; then
    echo ""
    echo "Verifying phase completion..."
    if "$ARC_SCRIPTS_DIR/verify-phase-complete.sh" "$PLAN_NAME" "$PHASE"; then
        "$ARC_SCRIPTS_DIR/update-state.sh" "$PLAN_NAME" "$PHASE" status complete
        echo ""
        echo "Phase marked complete."
    else
        echo ""
        echo "Phase NOT marked complete - verification failed."
        echo "Fix the issues above and run another impl iteration."
    fi
fi

# Clear hang count on successful completion (iteration didn't timeout)
"$ARC_SCRIPTS_DIR/update-state.sh" "$PLAN_NAME" "$PHASE" clear-hangs 2>/dev/null || true

# Show current state
echo ""
"$ARC_SCRIPTS_DIR/get-state.sh" "$PLAN_NAME" "$PHASE"

# Guidance for orchestrator
echo ""
echo "=== Next steps ==="
FINAL_STATUS=$(jq -r '.phase_status' "$STATE_FILE" 2>/dev/null || echo "unknown")

# Workflow-aware next steps (V1a)
if [[ -n "$WORKFLOW_FILE" ]]; then
    echo "(Using workflow: $(basename "$WORKFLOW_FILE"))"

    # Map MODE to workflow state for next-state lookup
    CURRENT_WORKFLOW_STATE="$MODE"
    case "$MODE" in
        qa-review) CURRENT_WORKFLOW_STATE="qa_review" ;;
        impl-review) CURRENT_WORKFLOW_STATE="impl_review" ;;
    esac

    # Get next state from workflow
    NEXT_STATE=$("$ARC_SCRIPTS_DIR/get-next-state.sh" "$WORKFLOW_FILE" "$CURRENT_WORKFLOW_STATE" 2>/dev/null || echo "")

    if [[ "$NEXT_STATE" == "TERMINAL" ]]; then
        echo "Current state '$CURRENT_WORKFLOW_STATE' is terminal."
        if [[ "$FINAL_STATUS" == "complete" ]]; then
            echo "Phase complete! Commit and move to next phase."
            echo "  git add -A && git commit -m 'feat($PHASE): <description>'"
        fi
    elif [[ -n "$NEXT_STATE" ]]; then
        # Map workflow state back to MODE for command
        NEXT_MODE="$NEXT_STATE"
        case "$NEXT_STATE" in
            qa_review) NEXT_MODE="qa-review" ;;
            impl_review) NEXT_MODE="impl-review" ;;
        esac

        echo "Workflow next state: $NEXT_STATE"
        echo "  $0 $PLAN_NAME $PHASE $NEXT_MODE"
    fi
fi

# Legacy mode-based guidance (backwards compatibility)
case "$MODE" in
    qa)
        echo "QA finished. Run qa-review to verify coverage:"
        echo "  $0 $PLAN_NAME $PHASE qa-review"
        echo "Then commit tests and start impl:"
        echo "  git add -A && git commit -m 'test($PHASE): add tests from spec'"
        echo "  $0 $PLAN_NAME $PHASE impl"
        ;;
    impl)
        if [[ "$FINAL_STATUS" == "impl_review" ]]; then
            echo "All tests passing! Run impl-review before committing:"
            echo "  $0 $PLAN_NAME $PHASE impl-review"
        elif [[ "$FINAL_STATUS" == "complete" ]]; then
            echo "Impl-review approved! Commit implementation:"
            echo "  git add -A && git commit -m 'feat($PHASE): <description>'"
        elif [[ "$FINAL_STATUS" == "disputed" ]]; then
            echo "Dispute filed. Review and resolve:"
            echo "  $ARC_SCRIPTS_DIR/get-state.sh $PLAN_NAME $PHASE"
            echo "  $ARC_SCRIPTS_DIR/update-state.sh $PLAN_NAME $PHASE approve-dispute 'reason'"
            echo "  $0 $PLAN_NAME $PHASE fix"
        else
            echo "Tests not all passing. Run another iteration:"
            echo "  $0 $PLAN_NAME $PHASE impl"
        fi
        ;;
    fix)
        echo "Fix complete. Continue implementation:"
        echo "  $0 $PLAN_NAME $PHASE impl"
        ;;
esac

# Final cleanup: kill any remaining tracked process groups
# This catches processes that slipped through timeout/crash handlers
kill_tracked_pgids
