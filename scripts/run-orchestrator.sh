#!/usr/bin/env bash
#
# run-orchestrator.sh - Launch orchestrator agent
#
# Usage: arc run [plan-name]
#        If no plan-name provided, shows interactive menu.

set -euo pipefail

ARC_HOME="${ARC_HOME:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
ARC_PROJECT_ROOT="${ARC_PROJECT_ROOT:-$(pwd)}"
ARC_SCRIPTS_DIR="$ARC_HOME/scripts"
ARC_PLANS_DIR="${ARC_PLANS_DIR:-$ARC_PROJECT_ROOT/.plans}"
ACTIVE_DIR="$ARC_PLANS_DIR/active"

CLAUDE_PID=""
TESTS_RUNNING_FLAG="/tmp/.arc-tests-running"

cleanup() {
    trap '' SIGTERM SIGINT
    local exit_code="${1:-1}"
    echo ""
    echo "Orchestrator shutting down..."
    [[ -n "${CLAUDE_PID:-}" ]] && kill $CLAUDE_PID 2>/dev/null || true
    sleep 1
    [[ -n "${CLAUDE_PID:-}" ]] && kill -9 $CLAUDE_PID 2>/dev/null || true
    kill -TERM -$$ 2>/dev/null || true
    sleep 1
    pkill -9 -f "claude.*-p.*#" 2>/dev/null || true
    run_safety_net
    exit "$exit_code"
}
trap 'cleanup 130' SIGTERM SIGINT

run_safety_net() {
    [[ -z "${PLAN_DIR:-}" || -z "${ARC_SCRIPTS_DIR:-}" ]] && return 0

    if [[ -d "$PLAN_DIR" && ! -f "$PLAN_DIR/COMPLETION_REPORT.md" ]]; then
        local all_complete=true
        for phase in $(jq -r '.phases[]' "$PLAN_DIR/plan.json" 2>/dev/null); do
            local phase_state="$PLAN_DIR/phases/$phase/state.json"
            if [[ -f "$phase_state" ]]; then
                local status
                status=$(jq -r '.phase_status // "unknown"' "$phase_state")
                if [[ "$status" != "complete" && "$status" != "split" && "$status" != "deferred" ]]; then
                    all_complete=false
                    break
                fi
            else
                all_complete=false
                break
            fi
        done

        if $all_complete; then
            echo ""
            echo "All phases complete. Running completion report..."
            "$ARC_SCRIPTS_DIR/generate-completion-report.sh" "$PLAN_NAME" 2>/dev/null || true
        fi
    fi
}

PLAN_NAME="${1:-}"

# Interactive menu if no plan given
if [[ -z "$PLAN_NAME" ]]; then
    PLANS=($(ls "$ACTIVE_DIR" 2>/dev/null || true))

    if [[ ${#PLANS[@]} -eq 0 ]]; then
        echo "No active plans found."
        echo "Create one with: arc plan <name> <phase1> [phase2] ..."
        exit 1
    fi

    echo "Select a plan:"
    echo ""
    select PLAN_NAME in "${PLANS[@]}" "Cancel"; do
        if [[ "$PLAN_NAME" == "Cancel" ]]; then
            exit 0
        elif [[ -n "$PLAN_NAME" ]]; then
            break
        fi
    done
    echo ""
fi

PLAN_DIR="$ACTIVE_DIR/$PLAN_NAME"

if [[ ! -d "$PLAN_DIR" ]]; then
    echo "Error: Plan '$PLAN_NAME' not found at $PLAN_DIR"
    exit 1
fi

# Verify review status
REVIEW_STATUS=$(jq -r '.review_status // "unreviewed"' "$PLAN_DIR/plan.json" 2>/dev/null)
if [[ "$REVIEW_STATUS" != "approved" && "$REVIEW_STATUS" != "conditional" ]]; then
    echo "Error: Plan '$PLAN_NAME' has review status '$REVIEW_STATUS'"
    echo ""
    echo "Plans must be reviewed before execution."
    echo "Run: arc review $PLAN_NAME"
    exit 1
fi

# Lockfile
LOCKFILE="$PLAN_DIR/.orchestrator.lock"

acquire_lock() {
    if [[ -f "$LOCKFILE" ]]; then
        local pid=$(cat "$LOCKFILE" 2>/dev/null)
        if kill -0 "$pid" 2>/dev/null; then
            echo "Error: Another orchestrator is running (PID: $pid)"
            exit 1
        else
            echo "Warning: Stale lock found, removing..."
            rm -f "$LOCKFILE"
        fi
    fi
    echo $$ > "$LOCKFILE"
}

release_lock() {
    rm -f "$LOCKFILE"
}

trap release_lock EXIT
acquire_lock

# Get phases
PHASES=$(jq -r 'if .phase_order then
    [.phases[] as $p | {n: $p, o: .phase_order[$p]}] | sort_by(.o) | .[].n
  else .phases[] end' "$PLAN_DIR/plan.json" 2>/dev/null | tr '\n' ', ' | sed 's/,$//')

echo "=========================================="
echo "  Launching Arc Orchestrator"
echo "=========================================="
echo "Plan: $PLAN_NAME"
echo "Phases: $PHASES"
echo "=========================================="
echo ""

# Set orchestrator environment
export ORCHESTRATOR_MODE=1
export ORCHESTRATOR_PLAN="$PLAN_NAME"
export ORCHESTRATOR_PLAN_DIR="$PLAN_DIR"
export ORCHESTRATOR_PHASES="$PHASES"
export ARC_HOME
export ARC_PROJECT_ROOT
export ARC_PLANS_DIR

# Load the orchestrator prompt from arc's prompts
ORCHESTRATOR_PROMPT="$ARC_HOME/prompts/orchestrator.md"
if [[ ! -f "$ORCHESTRATOR_PROMPT" ]]; then
    echo "Error: Orchestrator prompt not found at $ORCHESTRATOR_PROMPT"
    exit 1
fi

# Wall-clock timeout
ORCHESTRATOR_TIMEOUT="${ORCHESTRATOR_TIMEOUT:-14400}"
echo "Timeout: ${ORCHESTRATOR_TIMEOUT}s ($(( ORCHESTRATOR_TIMEOUT / 3600 ))h $(( (ORCHESTRATOR_TIMEOUT % 3600) / 60 ))m)"
echo ""

# Launch orchestrator
timeout -k 60 "$ORCHESTRATOR_TIMEOUT" bash -c "cat '$ORCHESTRATOR_PROMPT' | claude --print --max-turns 200 --allowedTools 'Bash,Read,Glob,Grep'" &
CLAUDE_PID=$!

# Monitor loop
ORCH_EXIT=0
while kill -0 $CLAUDE_PID 2>/dev/null; do
    if [[ ! -d "$PLAN_DIR" ]]; then
        echo ""
        echo "Plan archived. Stopping..."
        kill $CLAUDE_PID 2>/dev/null || true
        sleep 3
        kill -9 $CLAUDE_PID 2>/dev/null || true
        break
    fi

    if [[ -f "$PLAN_DIR/COMPLETION_REPORT.md" ]]; then
        echo ""
        echo "Complete. Stopping..."
        kill $CLAUDE_PID 2>/dev/null || true
        sleep 3
        kill -9 $CLAUDE_PID 2>/dev/null || true
        break
    fi

    sleep 10
done

wait $CLAUDE_PID 2>/dev/null || ORCH_EXIT=$?

if [[ $ORCH_EXIT -eq 124 ]]; then
    echo ""
    echo "=========================================="
    echo "  ORCHESTRATOR TIMED OUT"
    echo "=========================================="
    echo "Re-run to continue from where it left off."
fi

run_safety_net
