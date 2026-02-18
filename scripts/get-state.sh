#!/bin/bash
set -euo pipefail

# Read phase state in human-readable format
# Matches state.json schema: phase_status, iteration.current/max, dispute, etc.
#
# Usage: get-state.sh <plan> <phase>

ARC_SCRIPTS_DIR="${ARC_SCRIPTS_DIR:-$ARC_HOME/scripts}"
ARC_HOME="${ARC_HOME:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
ARC_PROJECT_ROOT="${ARC_PROJECT_ROOT:-$(pwd)}"
ARC_PLANS_DIR="${ARC_PLANS_DIR:-$ARC_PROJECT_ROOT/.plans}"

PLAN="${1:-}"
PHASE="${2:-}"

if [[ -z "$PLAN" || -z "$PHASE" ]]; then
    echo "Usage: get-state.sh <plan> <phase>" >&2
    exit 1
fi

STATE_FILE="$ARC_PLANS_DIR/active/$PLAN/phases/$PHASE/state.json"

if [[ ! -f "$STATE_FILE" ]]; then
    echo "Error: State file not found: $STATE_FILE" >&2
    exit 1
fi

STATE=$(cat "$STATE_FILE")

PHASE_STATUS=$(echo "$STATE" | jq -r '.phase_status // "unknown"')
ITER_CURRENT=$(echo "$STATE" | jq -r '.iteration.current // 0')
ITER_MAX=$(echo "$STATE" | jq -r '.iteration.max // 25')
TESTS_PASS=$(echo "$STATE" | jq -r '.tests_passing // 0')
TESTS_TOTAL=$(echo "$STATE" | jq -r '.tests_total // 0')
STUCK_COUNT=$(echo "$STATE" | jq -r '.stuck_iterations // 0')
HANG_COUNT=$(echo "$STATE" | jq -r '.hang_count // 0')
PACKAGES=$(echo "$STATE" | jq -r '.packages // [] | join(", ")')
LAST_REVIEWED=$(echo "$STATE" | jq -r '.last_reviewed_iteration // 0')
IS_BLOCKED=$(echo "$STATE" | jq -r '.blocked.is_blocked // false')
BLOCKED_REASON=$(echo "$STATE" | jq -r '.blocked.reason // empty')

# Format status for display
case "$PHASE_STATUS" in
    qa) STATUS_DISPLAY="qa (writing tests)" ;;
    qa_review) STATUS_DISPLAY="qa_review (reviewing tests)" ;;
    implementing) STATUS_DISPLAY="implementing (writing code)" ;;
    impl_review) STATUS_DISPLAY="impl_review (reviewing code)" ;;
    complete) STATUS_DISPLAY="complete ✓" ;;
    disputed) STATUS_DISPLAY="disputed ⚠" ;;
    blocked) STATUS_DISPLAY="blocked ✗" ;;
    *) STATUS_DISPLAY="$PHASE_STATUS" ;;
esac

echo "Plan: $PLAN"
echo "Phase: $PHASE"
echo "Status: $STATUS_DISPLAY"
echo "Iteration: $ITER_CURRENT/$ITER_MAX"
echo "Tests: $TESTS_PASS/$TESTS_TOTAL passing"
if [[ "$STUCK_COUNT" -gt 0 ]]; then
    echo "No-progress: $STUCK_COUNT iterations with same test count (blocks at 5)"
fi
if [[ "$HANG_COUNT" -gt 0 ]]; then
    echo "Timeouts: $HANG_COUNT sub-agent hangs (blocks at 3)"
fi
echo "Last reviewed: iteration $LAST_REVIEWED"
echo "Packages: ${PACKAGES:-none set}"

if [[ "$IS_BLOCKED" == "true" ]]; then
    echo ""
    echo "=== BLOCKED ==="
    echo "Reason: $BLOCKED_REASON"
fi

if [[ "$PHASE_STATUS" == "disputed" ]]; then
    DISPUTE_COUNT=$(echo "$STATE" | jq '.disputes // [] | length')
    echo ""
    echo "=== DISPUTES ($DISPUTE_COUNT) ==="
    echo "$STATE" | jq -r '.disputes // [] | .[] | "  - \(.test_name): \(.reason) [\(.resolution // "pending")]"'
fi

# Chunks info if present
CHUNKS_TOTAL=$(echo "$STATE" | jq -r '.chunks.total // 0')
if [[ "$CHUNKS_TOTAL" -gt 0 ]]; then
    CHUNKS_COMPLETED=$(echo "$STATE" | jq -r '.chunks.completed | length')
    CHUNKS_CURRENT=$(echo "$STATE" | jq -r '.chunks.current // "none"')
    echo ""
    echo "Chunks: $CHUNKS_COMPLETED/$CHUNKS_TOTAL completed"
    echo "Current chunk: $CHUNKS_CURRENT"
fi

# Exit with appropriate code
case "$PHASE_STATUS" in
    complete) exit 0 ;;
    pending|qa|qa_review|implementing|impl_review) exit 0 ;;
    disputed) exit 1 ;;
    blocked) exit 1 ;;
    *) exit 2 ;;
esac
