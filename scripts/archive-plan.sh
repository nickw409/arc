#!/usr/bin/env bash
#
# archive-plan.sh - Archive a completed plan and clean up session data
#
# Usage: archive-plan.sh [--force] <plan-name>
#
# Validates all phases are in a terminal state (complete/split/deferred),
# moves the plan from active/ to archive/, and removes claude session data.
#
# Options:
#   --force    Skip phase validation and archive regardless of state

set -euo pipefail

ARC_SCRIPTS_DIR="${ARC_SCRIPTS_DIR:-$ARC_HOME/scripts}"
ARC_HOME="${ARC_HOME:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
ARC_PROJECT_ROOT="${ARC_PROJECT_ROOT:-$(pwd)}"
ARC_PLANS_DIR="${ARC_PLANS_DIR:-$ARC_PROJECT_ROOT/.plans}"
ACTIVE_DIR="$ARC_PLANS_DIR/active"
ARCHIVE_DIR="$ARC_PLANS_DIR/archive"

# Parse flags
FORCE=false
if [[ "${1:-}" == "--force" ]]; then
    FORCE=true
    shift
fi

PLAN_NAME="${1:-}"

if [[ -z "$PLAN_NAME" ]]; then
    echo "Usage: archive-plan.sh [--force] <plan-name>"
    echo ""
    echo "Archives a completed plan and cleans up session data."
    echo ""
    echo "Options:"
    echo "  --force    Skip phase validation and archive regardless of state"
    exit 1
fi

PLAN_DIR="$ACTIVE_DIR/$PLAN_NAME"

# Verify plan exists
if [[ ! -d "$PLAN_DIR" ]]; then
    echo "Error: Plan '$PLAN_NAME' not found at $PLAN_DIR"
    exit 1
fi

# Validate all phases are in a terminal state (unless --force)
if [[ "$FORCE" == false && -d "$PLAN_DIR/phases" ]]; then
    echo "=== Validating Phase States ==="
    INCOMPLETE_PHASES=()

    # Get phases from plan.json for ordered iteration
    if [[ -f "$PLAN_DIR/plan.json" ]]; then
        PHASES=$(jq -r '.phases[]' "$PLAN_DIR/plan.json" 2>/dev/null)
    else
        # Fallback to directory listing
        PHASES=$(ls "$PLAN_DIR/phases/" 2>/dev/null)
    fi

    for phase in $PHASES; do
        state_file="$PLAN_DIR/phases/$phase/state.json"
        if [[ -f "$state_file" ]]; then
            status=$(jq -r '.phase_status' "$state_file")
            case "$status" in
                complete|split|deferred)
                    # Terminal states — OK
                    ;;
                *)
                    INCOMPLETE_PHASES+=("$phase ($status)")
                    ;;
            esac
        else
            INCOMPLETE_PHASES+=("$phase (no state)")
        fi
    done

    if [[ ${#INCOMPLETE_PHASES[@]} -gt 0 ]]; then
        echo "Error: Cannot archive — phases not in terminal state:"
        for phase in "${INCOMPLETE_PHASES[@]}"; do
            echo "  - $phase"
        done
        echo ""
        echo "Terminal states: complete, split, deferred"
        echo "Use --force to skip this check."
        exit 1
    fi
    echo "All phases in terminal state."
    echo ""
fi

# Check for completion report
if [[ ! -f "$PLAN_DIR/COMPLETION_REPORT.md" && "$FORCE" == false ]]; then
    echo "Warning: No COMPLETION_REPORT.md found."
    echo "Run generate-completion-report.sh first, or use --force to skip."
    exit 1
fi

# Read session ID for cleanup
SESSION_ID=""
if [[ -f "$PLAN_DIR/session_id" ]]; then
    SESSION_ID=$(cat "$PLAN_DIR/session_id")
fi

# Update plan.json status
if [[ -f "$PLAN_DIR/plan.json" ]]; then
    jq '.status = "archived" | .archived = now | .archived_date = (now | strftime("%Y-%m-%d"))' \
        "$PLAN_DIR/plan.json" > "$PLAN_DIR/plan.json.tmp" && \
        mv "$PLAN_DIR/plan.json.tmp" "$PLAN_DIR/plan.json"
fi

# Clean up orchestrator lock file
rm -f "$PLAN_DIR/.orchestrator.lock"

# Clean up temporary/intermediate files
rm -f "$PLAN_DIR/.aggregated_data.md"

# Ensure archive directory exists
mkdir -p "$ARCHIVE_DIR"

# Check if already archived
if [[ -d "$ARCHIVE_DIR/$PLAN_NAME" ]]; then
    echo "Warning: Plan already exists in archive. Adding timestamp suffix."
    TIMESTAMP=$(date +%Y%m%d-%H%M%S)
    ARCHIVE_NAME="${PLAN_NAME}-${TIMESTAMP}"
else
    ARCHIVE_NAME="$PLAN_NAME"
fi

# Move to archive
mv "$PLAN_DIR" "$ARCHIVE_DIR/$ARCHIVE_NAME"
echo "Archived plan to: $ARCHIVE_DIR/$ARCHIVE_NAME"

# Clean up claude session data if session ID exists
if [[ -n "$SESSION_ID" ]]; then
    # Claude stores sessions in ~/.claude/projects/<project-hash>/sessions/<session-id>/
    # Find and remove the session directory
    CLAUDE_DIR="$HOME/.claude"
    if [[ -d "$CLAUDE_DIR" ]]; then
        # Search for the session directory
        SESSION_DIR=$(find "$CLAUDE_DIR" -type d -name "$SESSION_ID" 2>/dev/null | head -1)
        if [[ -n "$SESSION_DIR" && -d "$SESSION_DIR" ]]; then
            rm -rf "$SESSION_DIR"
            echo "Cleaned up session data: $SESSION_ID"
        else
            echo "Session data not found (may already be cleaned): $SESSION_ID"
        fi
    fi
fi

echo ""
echo "Plan '$PLAN_NAME' archived successfully."
