#!/bin/bash
# get-phase-order.sh - Get the execution order of phases in a plan
#
# Usage: get-phase-order.sh <plan> [--json|--list|--phase <name>]
#
# Options:
#   --json         Output as JSON object (default)
#   --list         Output as ordered list (one phase per line)
#   --phase <name> Get the order position of a specific phase
#
# Examples:
#   get-phase-order.sh my-plan                   # JSON output
#   get-phase-order.sh my-plan --list            # One phase per line in order
#   get-phase-order.sh my-plan --phase impl-core # Position of impl-core

set -e

ARC_SCRIPTS_DIR="${ARC_SCRIPTS_DIR:-$ARC_HOME/scripts}"
BASE_DIR="$(dirname "$(dirname "$ARC_SCRIPTS_DIR")")"
ARC_PLANS_DIR="${PLANS_DIR:-$BASE_DIR/.plans}"
ACTIVE_DIR="$ARC_PLANS_DIR/active"

usage() {
    echo "Usage: $0 <plan> [--json|--list|--phase <name>]"
    echo ""
    echo "Options:"
    echo "  --json         Output as JSON object (default)"
    echo "  --list         Output as ordered list (one phase per line)"
    echo "  --phase <name> Get the order position of a specific phase"
    exit 1
}

if [ $# -lt 1 ]; then
    usage
fi

PLAN="$1"
shift

# Default output mode
OUTPUT_MODE="json"
PHASE_NAME=""

# Parse options
while [ $# -gt 0 ]; do
    case "$1" in
        --json)
            OUTPUT_MODE="json"
            shift
            ;;
        --list)
            OUTPUT_MODE="list"
            shift
            ;;
        --phase)
            if [ -z "${2:-}" ]; then
                echo "Error: --phase requires a phase name" >&2
                exit 1
            fi
            OUTPUT_MODE="single"
            PHASE_NAME="$2"
            shift 2
            ;;
        *)
            echo "Error: Unknown option: $1" >&2
            usage
            ;;
    esac
done

PLAN_DIR="$ACTIVE_DIR/$PLAN"
PLAN_JSON="$PLAN_DIR/plan.json"

if [ ! -d "$PLAN_DIR" ]; then
    echo "Error: Plan '$PLAN' not found" >&2
    exit 1
fi

if [ ! -f "$PLAN_JSON" ]; then
    echo "Error: plan.json not found for '$PLAN'" >&2
    exit 1
fi

# Check if phase_order exists, if not compute from phases array
if jq -e '.phase_order' "$PLAN_JSON" > /dev/null 2>&1; then
    # phase_order exists, use it
    PHASE_ORDER=$(jq '.phase_order' "$PLAN_JSON")
else
    # Compute from phases array (for backwards compatibility)
    PHASE_ORDER=$(jq '
        .phases | to_entries |
        map({key: .value, value: (.key + 1)}) |
        from_entries
    ' "$PLAN_JSON")
fi

case "$OUTPUT_MODE" in
    json)
        echo "$PHASE_ORDER"
        ;;
    list)
        # Output phases sorted by phase_order
        jq -r 'if .phase_order then
            [.phases[] as $p | {n: $p, o: .phase_order[$p]}] | sort_by(.o) | .[].n
          else .phases[] end' "$PLAN_JSON"
        ;;
    single)
        # Get order of specific phase
        ORDER=$(echo "$PHASE_ORDER" | jq -r --arg phase "$PHASE_NAME" '.[$phase] // empty')
        if [ -z "$ORDER" ]; then
            echo "Error: Phase '$PHASE_NAME' not found in plan" >&2
            exit 1
        fi
        echo "$ORDER"
        ;;
esac
