#!/bin/bash
# manage-phase.sh - Easy phase state management
#
# Usage:
#   manage-phase.sh <plan> <phase> <command> [args...]
#
# Commands:
#   complete [note]              - Mark phase as complete
#   pending                      - Reset to pending
#   defer <reason>               - Defer phase with reason
#   block <reason>               - Block phase with reason
#   tests <passing> <total>      - Set test counts
#   packages <pkg1> [pkg2...]    - Set affected packages
#   note <message>               - Add a note
#   iteration <n>                - Set iteration count
#   copy-from <other-phase>      - Copy state from another phase
#   show                         - Show current state
#
# Examples:
#   manage-phase.sh my-plan phase-a complete "Finished with commit abc123"
#   manage-phase.sh my-plan phase-b tests 47 47
#   manage-phase.sh my-plan phase-c packages my-package
#   manage-phase.sh my-plan phase-d defer "Waiting for dependencies"

set -e

ARC_SCRIPTS_DIR="${ARC_SCRIPTS_DIR:-$ARC_HOME/scripts}"
BASE_DIR="$(dirname "$(dirname "$ARC_SCRIPTS_DIR")")"
ARC_PLANS_DIR="${PLANS_DIR:-$BASE_DIR/.plans}"
ACTIVE_DIR="$ARC_PLANS_DIR/active"

usage() {
    cat << 'USAGE'
Usage: manage-phase.sh <plan> <phase> <command> [args...]

Commands:
  complete [note]              - Mark phase as complete
  pending                      - Reset to pending  
  defer <reason>               - Defer phase with reason
  block <reason>               - Block phase with reason
  tests <passing> <total>      - Set test counts
  packages <pkg1> [pkg2...]    - Set affected packages
  note <message>               - Add a note
  iteration <n>                - Set iteration count
  copy-from <other-phase>      - Copy state from another phase
  show                         - Show current state

Examples:
  manage-phase.sh my-plan phase-a complete "Finished with commit abc123"
  manage-phase.sh my-plan phase-b tests 47 47
  manage-phase.sh my-plan phase-c packages my-package
USAGE
    exit 1
}

if [ $# -lt 3 ]; then
    usage
fi

PLAN="$1"
PHASE="$2"
CMD="$3"
shift 3

PLAN_DIR="$ACTIVE_DIR/$PLAN"
PHASE_DIR="$PLAN_DIR/phases/$PHASE"
STATE_FILE="$PHASE_DIR/state.json"

# Create phase dir if it doesn't exist
if [ ! -d "$PHASE_DIR" ]; then
    mkdir -p "$PHASE_DIR"
fi

# Create state file if it doesn't exist
if [ ! -f "$STATE_FILE" ]; then
    cat > "$STATE_FILE" << INIT
{
  "phase": "$PHASE",
  "plan": "$PLAN",
  "phase_status": "pending",
  "iteration": {"current": 0, "max": 50},
  "packages": [],
  "tests_passing": 0,
  "tests_total": 0
}
INIT
fi

case "$CMD" in
    complete)
        NOTE="${1:-}"
        if [ -n "$NOTE" ]; then
            jq '.phase_status = "complete" | .completed_at = now | .notes = $note' \
                --arg note "$NOTE" "$STATE_FILE" > "$STATE_FILE.tmp"
        else
            jq '.phase_status = "complete" | .completed_at = now' \
                "$STATE_FILE" > "$STATE_FILE.tmp"
        fi
        mv "$STATE_FILE.tmp" "$STATE_FILE"
        echo "✓ $PHASE marked complete"
        ;;
        
    pending)
        jq '.phase_status = "pending" | del(.completed_at) | del(.deferred_reason) | del(.blocked_reason)' \
            "$STATE_FILE" > "$STATE_FILE.tmp"
        mv "$STATE_FILE.tmp" "$STATE_FILE"
        echo "✓ $PHASE reset to pending"
        ;;
        
    defer)
        REASON="${1:?Error: defer requires a reason}"
        jq '.phase_status = "deferred" | .deferred_reason = $reason | .deferred_at = now' \
            --arg reason "$REASON" "$STATE_FILE" > "$STATE_FILE.tmp"
        mv "$STATE_FILE.tmp" "$STATE_FILE"
        echo "✓ $PHASE deferred: $REASON"
        ;;
        
    block)
        REASON="${1:?Error: block requires a reason}"
        jq '.phase_status = "blocked" | .blocked_reason = $reason | .blocked_at = now' \
            --arg reason "$REASON" "$STATE_FILE" > "$STATE_FILE.tmp"
        mv "$STATE_FILE.tmp" "$STATE_FILE"
        echo "✓ $PHASE blocked: $REASON"
        ;;
        
    tests)
        PASSING="${1:?Error: tests requires passing count}"
        TOTAL="${2:?Error: tests requires total count}"
        jq '.tests_passing = ($p | tonumber) | .tests_total = ($t | tonumber)' \
            --arg p "$PASSING" --arg t "$TOTAL" "$STATE_FILE" > "$STATE_FILE.tmp"
        mv "$STATE_FILE.tmp" "$STATE_FILE"
        echo "✓ $PHASE tests: $PASSING/$TOTAL"
        ;;
        
    packages)
        if [ $# -eq 0 ]; then
            echo "Error: packages requires at least one package name"
            exit 1
        fi
        PKGS_JSON=$(printf '%s\n' "$@" | jq -R . | jq -s .)
        jq '.packages = $p' --argjson p "$PKGS_JSON" "$STATE_FILE" > "$STATE_FILE.tmp"
        mv "$STATE_FILE.tmp" "$STATE_FILE"
        echo "✓ $PHASE packages: $*"
        ;;
        
    note)
        NOTE="${1:?Error: note requires a message}"
        jq '.notes = $note' --arg note "$NOTE" "$STATE_FILE" > "$STATE_FILE.tmp"
        mv "$STATE_FILE.tmp" "$STATE_FILE"
        echo "✓ $PHASE note added"
        ;;
        
    iteration|iter)
        N="${1:?Error: iteration requires a number}"
        jq '.iteration.current = ($n | tonumber)' --arg n "$N" "$STATE_FILE" > "$STATE_FILE.tmp"
        mv "$STATE_FILE.tmp" "$STATE_FILE"
        echo "✓ $PHASE iteration: $N"
        ;;
        
    copy-from)
        SOURCE="${1:?Error: copy-from requires source phase name}"
        SOURCE_FILE="$PLAN_DIR/phases/$SOURCE/state.json"
        if [ ! -f "$SOURCE_FILE" ]; then
            echo "Error: Source phase '$SOURCE' not found"
            exit 1
        fi
        # Copy but update phase name
        jq '.phase = $phase' --arg phase "$PHASE" "$SOURCE_FILE" > "$STATE_FILE"
        echo "✓ $PHASE state copied from $SOURCE"
        ;;
        
    show)
        echo "=== $PHASE ==="
        jq '.' "$STATE_FILE"
        ;;
        
    *)
        echo "Unknown command: $CMD"
        usage
        ;;
esac

