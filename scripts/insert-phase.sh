#!/bin/bash
# insert-phase.sh - Insert new phases before or after an existing phase
#
# Usage: 
#   insert-phase.sh <plan> --before <phase> <new-phase1> [new-phase2] ...
#   insert-phase.sh <plan> --after <phase> <new-phase1> [new-phase2] ...
#
# Example:
#   insert-phase.sh my-plan --before complex-phase prep-phase
#   insert-phase.sh my-plan --after phase-a cleanup-phase validation-phase

set -e

ARC_SCRIPTS_DIR="${ARC_SCRIPTS_DIR:-$ARC_HOME/scripts}"
BASE_DIR="$(dirname "$(dirname "$ARC_SCRIPTS_DIR")")"
ARC_PLANS_DIR="${PLANS_DIR:-$BASE_DIR/.plans}"
ACTIVE_DIR="$ARC_PLANS_DIR/active"

usage() {
    echo "Usage: $0 <plan> --before|--after <phase> <new-phase1> [new-phase2] ..."
    echo ""
    echo "Insert new phases before or after an existing phase."
    echo ""
    echo "Examples:"
    echo "  $0 my-plan --before complex-phase prep-phase"
    echo "  $0 my-plan --after phase-a cleanup-phase"
    exit 1
}

if [ $# -lt 4 ]; then
    usage
fi

PLAN="$1"
POSITION="$2"
REF_PHASE="$3"
shift 3
NEW_PHASES=("$@")

if [ "$POSITION" != "--before" ] && [ "$POSITION" != "--after" ]; then
    echo "Error: Second argument must be --before or --after"
    usage
fi

PLAN_DIR="$ACTIVE_DIR/$PLAN"
PLAN_JSON="$PLAN_DIR/plan.json"

# Validate
if [ ! -d "$PLAN_DIR" ]; then
    echo "Error: Plan '$PLAN' not found"
    exit 1
fi

if [ ! -d "$PLAN_DIR/phases/$REF_PHASE" ]; then
    echo "Error: Reference phase '$REF_PHASE' not found"
    exit 1
fi

echo "=== Inserting phases $POSITION $REF_PHASE ==="
echo "New phases: ${NEW_PHASES[*]}"
echo ""

# Create phase directories
for NEW_PHASE in "${NEW_PHASES[@]}"; do
    NEW_DIR="$PLAN_DIR/phases/$NEW_PHASE"
    
    if [ -d "$NEW_DIR" ]; then
        echo "Warning: Phase '$NEW_PHASE' already exists, skipping"
        continue
    fi
    
    mkdir -p "$NEW_DIR"
    
    # Create placeholder plan.md
    cat > "$NEW_DIR/plan.md" << PLAN_EOF
# Phase: $NEW_PHASE

## Objective

[TODO: Define the objective for this phase]

Inserted $POSITION: $REF_PHASE

## Files

### Create
- [TODO: List files to create]

### Modify  
- [TODO: List files to modify]

## Test Cases

[TODO: Define test cases]

PLAN_EOF
    
    # Create state.json
    cat > "$NEW_DIR/state.json" << STATE_EOF
{
  "phase": "$NEW_PHASE",
  "plan": "$PLAN",
  "phase_status": "pending",
  "iteration": {
    "current": 0,
    "max": 50
  },
  "packages": [],
  "tests_passing": 0,
  "tests_total": 0
}
STATE_EOF
    
    echo "✓ Created phase: $NEW_PHASE"
done

# Update plan.json
if [ -f "$PLAN_JSON" ]; then
    PHASES=$(jq -r '.phases[]' "$PLAN_JSON")
    UPDATED_PHASES=()
    
    for phase in $PHASES; do
        if [ "$phase" = "$REF_PHASE" ]; then
            if [ "$POSITION" = "--before" ]; then
                for new in "${NEW_PHASES[@]}"; do
                    UPDATED_PHASES+=("$new")
                done
                UPDATED_PHASES+=("$phase")
            else
                UPDATED_PHASES+=("$phase")
                for new in "${NEW_PHASES[@]}"; do
                    UPDATED_PHASES+=("$new")
                done
            fi
        else
            UPDATED_PHASES+=("$phase")
        fi
    done
    
    jq '.phases = $phases' \
        --argjson phases "$(printf '%s\n' "${UPDATED_PHASES[@]}" | jq -R . | jq -s .)" \
        "$PLAN_JSON" > "$PLAN_JSON.tmp" && mv "$PLAN_JSON.tmp" "$PLAN_JSON"
    
    echo "✓ Updated plan.json"
fi

echo ""
echo "=== Insert complete ==="
echo "Don't forget to edit each new phase's plan.md!"

