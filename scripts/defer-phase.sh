#!/bin/bash
# defer-phase.sh - Mark a phase as deferred (skip for now)
#
# Usage: defer-phase.sh <plan> <phase> [reason]
#
# Example:
#   defer-phase.sh my-plan cuda-gpu "GPU optimization deferred until CPU impl stable"

set -e

ARC_SCRIPTS_DIR="${ARC_SCRIPTS_DIR:-$ARC_HOME/scripts}"
BASE_DIR="$(dirname "$(dirname "$ARC_SCRIPTS_DIR")")"
ARC_PLANS_DIR="${PLANS_DIR:-$BASE_DIR/.plans}"
ACTIVE_DIR="$ARC_PLANS_DIR/active"

if [ $# -lt 2 ]; then
    echo "Usage: $0 <plan> <phase> [reason]"
    exit 1
fi

PLAN="$1"
PHASE="$2"
REASON="${3:-No reason provided}"

STATE_FILE="$ACTIVE_DIR/$PLAN/phases/$PHASE/state.json"

if [ ! -f "$STATE_FILE" ]; then
    echo "Error: Phase state not found at $STATE_FILE"
    exit 1
fi

jq '.phase_status = "deferred" | .deferred_reason = $reason | .deferred_at = now' \
    --arg reason "$REASON" \
    "$STATE_FILE" > "$STATE_FILE.tmp" && mv "$STATE_FILE.tmp" "$STATE_FILE"

echo "✓ Phase '$PHASE' deferred: $REASON"

