#!/usr/bin/env bash
# Script: get-next-state.sh
# Purpose: Get the next state from a workflow (V1: linear transitions only)

set -euo pipefail

ARC_SCRIPTS_DIR="${ARC_SCRIPTS_DIR:-$ARC_HOME/scripts}"

error() {
    echo "ERROR: $*" >&2
    exit 1
}

require_command() {
    command -v "$1" &> /dev/null || error "$1 is required but not installed"
}

require_command yq

# Usage
usage() {
    echo "Usage: $0 <workflow-file> <current-state> [verdict]"
    echo ""
    echo "Arguments:"
    echo "  workflow-file   Path to workflow YAML file"
    echo "  current-state   Name of current state"
    echo "  verdict         (V2+) Verdict from review state (ignored in V1)"
    echo ""
    echo "Output:"
    echo "  Prints the next state name, or 'TERMINAL' if current state is terminal"
    echo ""
    echo "Exit codes:"
    echo "  0  Success"
    echo "  1  Error (missing args, file not found, state not found)"
    exit 1
}

[[ $# -ge 2 ]] || usage

WORKFLOW_FILE="$1"
CURRENT_STATE="$2"
VERDICT="${3:-}"  # Ignored in V1, used in V2+

# Validate workflow file exists
[[ -f "$WORKFLOW_FILE" ]] || error "Workflow file not found: $WORKFLOW_FILE"

# Check if current state is a terminal state
is_terminal=$(yq ".terminal_states | contains([\"$CURRENT_STATE\"])" "$WORKFLOW_FILE")
if [[ "$is_terminal" == "true" ]]; then
    echo "TERMINAL"
    exit 0
fi

# Get the next state (V1: simple string lookup)
next_state=$(yq ".states[] | select(.name == \"$CURRENT_STATE\") | .next" "$WORKFLOW_FILE")

# Handle case where state not found
if [[ -z "$next_state" || "$next_state" == "null" ]]; then
    error "State '$CURRENT_STATE' not found in workflow or has no 'next' field"
fi

# V2 check: if next is an object (branching), we need a verdict
if [[ "$next_state" == *":"* || "$next_state" == "{"* ]]; then
    if [[ -z "$VERDICT" ]]; then
        error "State '$CURRENT_STATE' has conditional transitions (V2+) but no verdict provided"
    fi
    # V2: resolve by verdict
    next_state=$(yq ".states[] | select(.name == \"$CURRENT_STATE\") | .next.$VERDICT" "$WORKFLOW_FILE")
    if [[ -z "$next_state" || "$next_state" == "null" ]]; then
        error "No transition defined for verdict '$VERDICT' in state '$CURRENT_STATE'"
    fi
fi

echo "$next_state"
