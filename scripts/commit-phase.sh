#!/usr/bin/env bash
#
# commit-phase.sh - Create a git commit for a completed phase
#
# Reads git configuration from .arc.yaml and creates a commit
# with the appropriate style.
#
# Usage: commit-phase.sh <plan-name> <phase> [message-override]
#
# Exit codes:
#   0 - Commit created
#   1 - Error or no changes

set -euo pipefail

ARC_HOME="${ARC_HOME:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
ARC_PROJECT_ROOT="${ARC_PROJECT_ROOT:-$(pwd)}"
ARC_SCRIPTS_DIR="$ARC_HOME/scripts"
ARC_PLANS_DIR="${ARC_PLANS_DIR:-$ARC_PROJECT_ROOT/.plans}"

# Load project config
source "$ARC_SCRIPTS_DIR/config.sh"

PLAN_NAME="${1:-}"
PHASE="${2:-}"
MESSAGE_OVERRIDE="${3:-}"

if [[ -z "$PLAN_NAME" || -z "$PHASE" ]]; then
    echo "Usage: commit-phase.sh <plan-name> <phase> [message-override]" >&2
    exit 1
fi

PHASE_DIR="$ARC_PLANS_DIR/active/$PLAN_NAME/phases/$PHASE"
STATE_FILE="$PHASE_DIR/state.json"

if [[ ! -f "$STATE_FILE" ]]; then
    echo "Error: State file not found: $STATE_FILE" >&2
    exit 1
fi

# Check we're in a git repo
if ! git rev-parse --git-dir > /dev/null 2>&1; then
    echo "Error: Not a git repository" >&2
    exit 1
fi

# Check for changes
if git diff --cached --quiet 2>/dev/null && git diff --quiet 2>/dev/null && [[ -z "$(git ls-files --others --exclude-standard)" ]]; then
    echo "No changes to commit."
    exit 0
fi

# Build commit message
if [[ -n "$MESSAGE_OVERRIDE" ]]; then
    COMMIT_MSG="$MESSAGE_OVERRIDE"
else
    # Read phase objective for commit message
    OBJECTIVE=""
    PLAN_FILE="$PHASE_DIR/plan.md"
    if [[ -f "$PLAN_FILE" ]]; then
        OBJECTIVE=$(grep -A1 "^## Objective" "$PLAN_FILE" | tail -1 | sed 's/^<!-- *//' | sed 's/ *-->$//' | sed 's/^ *//' || true)
    fi

    case "$ARC_GIT_STYLE" in
        conventional)
            # Determine prefix from phase status
            STATUS=$(jq -r '.phase_status // "implementing"' "$STATE_FILE")
            PREFIX="feat"
            if [[ "$STATUS" == "qa" || "$STATUS" == "qa_review" ]]; then
                PREFIX="test"
            fi

            if [[ -n "$OBJECTIVE" && "$OBJECTIVE" != *"One sentence"* ]]; then
                COMMIT_MSG="$PREFIX($PHASE): $(echo "$OBJECTIVE" | head -c 72)"
            else
                COMMIT_MSG="$PREFIX($PHASE): complete phase implementation"
            fi
            ;;
        freeform|*)
            if [[ -n "$OBJECTIVE" && "$OBJECTIVE" != *"One sentence"* ]]; then
                COMMIT_MSG="$PHASE: $OBJECTIVE"
            else
                COMMIT_MSG="Complete $PHASE phase"
            fi
            ;;
    esac
fi

# Stage all changes
git add -A

# Build commit command
COMMIT_ARGS=(-m "$COMMIT_MSG")
[[ "$ARC_GIT_SIGN" == "true" ]] && COMMIT_ARGS+=(-S)

# Create commit
git commit "${COMMIT_ARGS[@]}" --quiet

COMMIT_HASH=$(git rev-parse HEAD)

# Update state with commit hash
jq --arg hash "$COMMIT_HASH" '.last_commit = $hash' "$STATE_FILE" > "${STATE_FILE}.tmp.$$" \
    && mv "${STATE_FILE}.tmp.$$" "$STATE_FILE"

echo "Committed: $COMMIT_HASH"
echo "Message: $COMMIT_MSG"
