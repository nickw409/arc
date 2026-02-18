#!/bin/bash
# list-phases.sh - List all phases in a plan with their status
#
# Usage: list-phases.sh <plan>

set -e

ARC_SCRIPTS_DIR="${ARC_SCRIPTS_DIR:-$ARC_HOME/scripts}"
BASE_DIR="$(dirname "$(dirname "$ARC_SCRIPTS_DIR")")"
ARC_PLANS_DIR="${PLANS_DIR:-$BASE_DIR/.plans}"
ACTIVE_DIR="$ARC_PLANS_DIR/active"

if [ $# -lt 1 ]; then
    echo "Usage: $0 <plan>"
    exit 1
fi

PLAN="$1"
PLAN_DIR="$ACTIVE_DIR/$PLAN"

if [ ! -d "$PLAN_DIR" ]; then
    echo "Error: Plan '$PLAN' not found"
    exit 1
fi

echo "=== Phases in plan: $PLAN ==="
echo ""

# Check for plan.json first
PLAN_JSON="$PLAN_DIR/plan.json"
if [ -f "$PLAN_JSON" ]; then
    PHASES=$(jq -r 'if .phase_order then
        [.phases[] as $p | {n: $p, o: .phase_order[$p]}] | sort_by(.o) | .[].n
      else .phases[] end' "$PLAN_JSON" 2>/dev/null || echo "")
    # Load phase_order if it exists
    if jq -e '.phase_order' "$PLAN_JSON" > /dev/null 2>&1; then
        PHASE_ORDER=$(jq '.phase_order' "$PLAN_JSON")
    else
        PHASE_ORDER="{}"
    fi
else
    # Fall back to directory listing
    PHASES=$(ls -1 "$PLAN_DIR/phases" 2>/dev/null | sort)
    PHASE_ORDER="{}"
fi

ORDER_NUM=0
for phase in $PHASES; do
    ORDER_NUM=$((ORDER_NUM + 1))
    # Try to get order from phase_order, fallback to sequential
    STORED_ORDER=$(echo "$PHASE_ORDER" | jq -r --arg p "$phase" '.[$p] // empty')
    if [ -n "$STORED_ORDER" ]; then
        ORDER_NUM="$STORED_ORDER"
    fi
    STATE_FILE="$PLAN_DIR/phases/$phase/state.json"
    if [ -f "$STATE_FILE" ]; then
        STATUS=$(jq -r '.phase_status // "unknown"' "$STATE_FILE")
        TESTS_PASS=$(jq -r '.tests_passing // 0' "$STATE_FILE")
        TESTS_TOTAL=$(jq -r '.tests_total // 0' "$STATE_FILE")
        ITERATION=$(jq -r '.iteration.current // 0' "$STATE_FILE")
        
        # Status indicator
        case "$STATUS" in
            complete) ICON="✓" ;;
            implementing|impl_review) ICON="▶" ;;
            qa|qa_review) ICON="📝" ;;
            disputed) ICON="⚠" ;;
            blocked) ICON="🚫" ;;
            deferred) ICON="⏸" ;;
            split) ICON="↔" ;;
            pending) ICON=" " ;;
            *) ICON="?" ;;
        esac
        
        printf "  %2d. [%s] %-25s %s" "$ORDER_NUM" "$ICON" "$phase" "$STATUS"
        
        if [ "$TESTS_TOTAL" != "0" ]; then
            printf " (%d/%d tests)" "$TESTS_PASS" "$TESTS_TOTAL"
        fi
        
        if [ "$ITERATION" != "0" ]; then
            printf " iter:%d" "$ITERATION"
        fi
        
        # Show deferred reason if applicable
        if [ "$STATUS" = "deferred" ]; then
            REASON=$(jq -r '.deferred_reason // ""' "$STATE_FILE")
            if [ -n "$REASON" ]; then
                printf "\n      Reason: %s" "$REASON"
            fi
        fi
        
        # Show split info if applicable
        if [ "$STATUS" = "split" ]; then
            SPLIT_INTO=$(jq -r '.split_into // [] | join(", ")' "$STATE_FILE")
            if [ -n "$SPLIT_INTO" ]; then
                printf "\n      Split into: %s" "$SPLIT_INTO"
            fi
        fi
        
        echo ""
    else
        printf "  %2d. [ ] %-25s (no state)\n" "$ORDER_NUM" "$phase"
    fi
done

echo ""

