#!/usr/bin/env bash
set -euo pipefail

# arc/scripts/run-parallel.sh
#
# Run parallel branches from a workflow parallel state.
#
# Usage: run-parallel.sh <workflow-file> <state-name> <phase-dir> <plan-name> <phase>

ARC_SCRIPTS_DIR="${ARC_SCRIPTS_DIR:-$ARC_HOME/scripts}"

# =============================================================================
# Argument parsing
# =============================================================================

if [[ $# -lt 5 ]]; then
    echo "Usage: run-parallel.sh <workflow-file> <state-name> <phase-dir> <plan-name> <phase>" >&2
    exit 1
fi

WORKFLOW_FILE="$1"
STATE_NAME="$2"
PHASE_DIR="$3"
PLAN_NAME="$4"
PHASE="$5"

# =============================================================================
# Validation
# =============================================================================

# 1. Workflow file exists
if [[ ! -f "$WORKFLOW_FILE" ]]; then
    echo "Error: workflow file does not exist: $WORKFLOW_FILE" >&2
    exit 1
fi

# 2. State has parallel block
parallel_block=$(yq ".states[] | select(.name == \"$STATE_NAME\") | .parallel" "$WORKFLOW_FILE")
if [[ -z "$parallel_block" || "$parallel_block" == "null" ]]; then
    echo "Error: state '$STATE_NAME' has no parallel block" >&2
    exit 2
fi

# 3. Branches array non-empty
branch_count=$(yq ".states[] | select(.name == \"$STATE_NAME\") | .parallel.branches | length" "$WORKFLOW_FILE")
if [[ "$branch_count" -eq 0 ]]; then
    echo "Error: no branches defined in parallel block for state '$STATE_NAME'" >&2
    exit 1
fi

# Read all branch names and prompts for validation
mapfile -t branch_names < <(yq ".states[] | select(.name == \"$STATE_NAME\") | .parallel.branches[].name" "$WORKFLOW_FILE")
mapfile -t branch_prompts < <(yq ".states[] | select(.name == \"$STATE_NAME\") | .parallel.branches[].prompt" "$WORKFLOW_FILE")

# 4. All branch names valid ([a-zA-Z0-9_-]+)
for name in "${branch_names[@]}"; do
    if [[ ! "$name" =~ ^[a-zA-Z0-9_-]+$ ]]; then
        echo "Error: invalid branch name '$name' — must match [a-zA-Z0-9_-]+" >&2
        exit 1
    fi
done

# 5. All branch prompt files exist on disk
for prompt_path in "${branch_prompts[@]}"; do
    local_path="$ARC_HOME/$prompt_path"
    if [[ ! -f "$local_path" ]]; then
        echo "Error: prompt file not found: $local_path" >&2
        exit 1
    fi
done

# =============================================================================
# Setup
# =============================================================================

# Results directory
results_dir="$PHASE_DIR/parallel_${STATE_NAME}"
if [[ -d "$results_dir" ]]; then
    rm -rf "$results_dir"
fi
mkdir -p "$results_dir"

# Read timeout from workflow defaults
TIMEOUT=$(yq ".defaults.timeout // 600" "$WORKFLOW_FILE")

# =============================================================================
# Functions
# =============================================================================

cleanup_branches() {
    local rdir="$1"
    if [[ ! -d "$rdir" ]]; then
        return 0
    fi

    shopt -s nullglob
    local pidfiles=("$rdir"/*.pid)
    shopt -u nullglob

    for pidfile in "${pidfiles[@]}"; do
        if [[ -f "$pidfile" ]]; then
            local pid
            pid=$(cat "$pidfile" 2>/dev/null) || continue
            if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
                # Send SIGTERM to process group
                kill -TERM -- -"$pid" 2>/dev/null || true
            fi
        fi
    done

    # Wait for SIGTERM to take effect
    sleep 5

    # Escalate to SIGKILL for any remaining processes
    for pidfile in "${pidfiles[@]}"; do
        if [[ -f "$pidfile" ]]; then
            local pid
            pid=$(cat "$pidfile" 2>/dev/null) || continue
            if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
                kill -KILL -- -"$pid" 2>/dev/null || true
            fi
        fi
    done
}

spawn_branch() {
    local branch_name="$1"
    local prompt_path="$2"
    local context_json="$3"
    local rdir="$4"
    local timeout="$5"

    # Render the prompt template
    local rendered_prompt
    rendered_prompt=$(python3 "$ARC_HOME/scripts/render_template.py" \
        "$ARC_HOME/$prompt_path" "$context_json" "$ARC_HOME/prompts")

    # Spawn agent in new process group via setsid
    (
        set +e
        export PARALLEL_BRANCH_NAME="$branch_name"
        setsid timeout "$timeout" claude --print --output-format text \
            --max-turns 15 \
            --allowedTools "Read,Glob,Grep,Bash,Edit,Write" \
            < <(echo "$rendered_prompt") \
            > "$rdir/$branch_name.log" 2>&1 &
        local child_pid=$!

        # Write PID atomically
        echo "$child_pid" > "$rdir/$branch_name.pid.tmp"
        mv "$rdir/$branch_name.pid.tmp" "$rdir/$branch_name.pid"

        # Wait for completion and record exit code
        wait "$child_pid" 2>/dev/null
        local exit_code=$?
        echo "$exit_code" > "$rdir/$branch_name.exit"
    ) &
}

collect_results() {
    local rdir="$1"
    local timeout="$2"
    local expected_count="$3"
    local max_wait=$((timeout + 30))
    local waited=0

    while [[ $waited -lt $max_wait ]]; do
        # Count .exit files
        shopt -s nullglob
        local exitfiles=("$rdir"/*.exit)
        shopt -u nullglob

        local exit_count=${#exitfiles[@]}

        # All expected branches have written exit files
        if [[ $exit_count -ge $expected_count ]]; then
            break
        fi

        sleep 2
        waited=$((waited + 2))
    done

    # Build JSON summary for stderr
    local json_branches="["
    local first=true
    shopt -s nullglob
    local all_exit_files=("$rdir"/*.exit)
    shopt -u nullglob
    for exitfile in "${all_exit_files[@]}"; do
        local bname
        bname=$(basename "$exitfile" .exit)
        local ecode
        ecode=$(cat "$exitfile")
        if [[ "$first" == "true" ]]; then
            first=false
        else
            json_branches+=","
        fi
        json_branches+="{\"name\":\"$bname\",\"exit_code\":$ecode}"
    done
    json_branches+="]"
    echo "{\"branches\":$json_branches}" >&2
}

# =============================================================================
# Main
# =============================================================================

# Set up cleanup trap
trap 'cleanup_branches "$results_dir"' EXIT SIGTERM SIGINT

# Build phase context
phase_ctx=$(jq -n --arg plan_name "$PLAN_NAME" --arg phase "$PHASE" \
    '{plan_name: $plan_name, phase: $phase}')

# Spawn all branches
for i in $(seq 0 $((branch_count - 1))); do
    bname="${branch_names[$i]}"
    bprompt="${branch_prompts[$i]}"

    # Get branch params (may be null)
    branch_params=$(yq -o=json ".states[] | select(.name == \"$STATE_NAME\") | .parallel.branches[$i].params" "$WORKFLOW_FILE")
    if [[ "$branch_params" == "null" || -z "$branch_params" ]]; then
        branch_params='{}'
    fi

    # Merge: branch params override phase context
    context_json=$(jq -n --argjson branch "$branch_params" --argjson ctx "$phase_ctx" '$ctx * $branch')

    spawn_branch "$bname" "$bprompt" "$context_json" "$results_dir" "$TIMEOUT"
done

# Collect results
collect_results "$results_dir" "$TIMEOUT" "$branch_count"

exit 0
