#!/bin/bash
# split-phase.sh - Split a phase into sub-phases
#
# Usage: split-phase.sh <plan> <phase> <sub-phase1> [sub-phase2] ...
#
# Example:
#   split-phase.sh distribution-auto-fitting cuda-gpu cuda-structs cuda-bootstrap cuda-kernels cuda-functions
#
# This will:
#   1. Mark the original phase as "split"
#   2. Create new phase directories for each sub-phase
#   3. Copy the original plan.md as a reference
#   4. Update the plan's phase list
#   5. Create minimal state.json for each new phase

set -e

ARC_SCRIPTS_DIR="${ARC_SCRIPTS_DIR:-$ARC_HOME/scripts}"
BASE_DIR="$(dirname "$(dirname "$ARC_SCRIPTS_DIR")")"
ARC_PLANS_DIR="${PLANS_DIR:-$BASE_DIR/.plans}"
ACTIVE_DIR="$ARC_PLANS_DIR/active"

usage() {
    echo "Usage: $0 <plan> <phase> <sub-phase1> [sub-phase2] ..."
    echo ""
    echo "Split a phase into multiple sub-phases."
    echo ""
    echo "Example:"
    echo "  $0 my-plan complex-phase phase-a phase-b phase-c"
    exit 1
}

if [ $# -lt 3 ]; then
    usage
fi

PLAN="$1"
ORIGINAL_PHASE="$2"
shift 2
SUB_PHASES=("$@")

PLAN_DIR="$ACTIVE_DIR/$PLAN"
PHASE_DIR="$PLAN_DIR/phases/$ORIGINAL_PHASE"

# Validate
if [ ! -d "$PLAN_DIR" ]; then
    echo "Error: Plan '$PLAN' not found at $PLAN_DIR"
    exit 1
fi

if [ ! -d "$PHASE_DIR" ]; then
    echo "Error: Phase '$ORIGINAL_PHASE' not found at $PHASE_DIR"
    exit 1
fi

echo "=== Splitting phase: $ORIGINAL_PHASE ==="
echo "Into sub-phases: ${SUB_PHASES[*]}"
echo ""

# 1. Mark original phase as split
ORIGINAL_STATE="$PHASE_DIR/state.json"
if [ -f "$ORIGINAL_STATE" ]; then
    jq '.phase_status = "split" | .split_into = $phases' \
        --argjson phases "$(printf '%s\n' "${SUB_PHASES[@]}" | jq -R . | jq -s .)" \
        "$ORIGINAL_STATE" > "$ORIGINAL_STATE.tmp" && mv "$ORIGINAL_STATE.tmp" "$ORIGINAL_STATE"
    echo "✓ Marked $ORIGINAL_PHASE as split"
fi

# 2. Create sub-phase directories
for i in "${!SUB_PHASES[@]}"; do
    SUB_PHASE="${SUB_PHASES[$i]}"
    SUB_DIR="$PLAN_DIR/phases/$SUB_PHASE"
    
    if [ -d "$SUB_DIR" ]; then
        echo "Warning: Sub-phase '$SUB_PHASE' already exists, skipping creation"
        continue
    fi
    
    mkdir -p "$SUB_DIR"
    
    # Copy original plan as reference
    if [ -f "$PHASE_DIR/plan.md" ]; then
        cp "$PHASE_DIR/plan.md" "$SUB_DIR/original_plan.md"
    fi
    
    # Create placeholder plan.md
    cat > "$SUB_DIR/plan.md" << PLAN_EOF
# Phase: $SUB_PHASE

## Objective

[TODO: Define the specific objective for this sub-phase]

This is sub-phase $((i+1)) of ${#SUB_PHASES[@]}, split from: $ORIGINAL_PHASE

See original_plan.md for the full original plan.

## Files

### Create
- [TODO: List files to create]

### Modify
- [TODO: List files to modify]

## Dependencies

- Previous sub-phase: ${SUB_PHASES[$((i-1))]:-"(none - this is first)"}
- Next sub-phase: ${SUB_PHASES[$((i+1))]:-"(none - this is last)"}

## Test Cases

[TODO: Define test cases for this sub-phase]

PLAN_EOF
    
    # Create initial state.json
    cat > "$SUB_DIR/state.json" << STATE_EOF
{
  "phase": "$SUB_PHASE",
  "plan": "$PLAN",
  "phase_status": "pending",
  "iteration": {
    "current": 0,
    "max": 50
  },
  "parent_phase": "$ORIGINAL_PHASE",
  "sub_phase_index": $i,
  "packages": [],
  "tests_passing": 0,
  "tests_total": 0
}
STATE_EOF
    
    echo "✓ Created sub-phase: $SUB_PHASE"
done

# 3. Update plan's phase list (plan.json or similar)
PLAN_JSON="$PLAN_DIR/plan.json"
if [ -f "$PLAN_JSON" ]; then
    # Get current phases, find position of original phase, replace with sub-phases
    PHASES=$(jq -r '.phases[]' "$PLAN_JSON")
    NEW_PHASES=()
    
    for phase in $PHASES; do
        if [ "$phase" = "$ORIGINAL_PHASE" ]; then
            # Insert sub-phases instead of original
            for sub in "${SUB_PHASES[@]}"; do
                NEW_PHASES+=("$sub")
            done
        else
            NEW_PHASES+=("$phase")
        fi
    done
    
    # Update plan.json
    jq '.phases = $new_phases | .split_phases[$orig] = $subs' \
        --argjson new_phases "$(printf '%s\n' "${NEW_PHASES[@]}" | jq -R . | jq -s .)" \
        --arg orig "$ORIGINAL_PHASE" \
        --argjson subs "$(printf '%s\n' "${SUB_PHASES[@]}" | jq -R . | jq -s .)" \
        "$PLAN_JSON" > "$PLAN_JSON.tmp" && mv "$PLAN_JSON.tmp" "$PLAN_JSON"
    
    echo "✓ Updated plan.json with new phase list"
fi

echo ""
echo "=== Split complete ==="
echo ""
echo "New phases created:"
for sub in "${SUB_PHASES[@]}"; do
    echo "  - $sub"
done
echo ""
echo "Next steps:"
echo "  1. Edit each sub-phase's plan.md with specific objectives"
echo "  2. Run: arc/scripts/status.sh $PLAN"
echo "  3. Continue orchestration with sub-phases"

