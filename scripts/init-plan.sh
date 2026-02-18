#!/usr/bin/env bash
#
# init-plan.sh - Initialize a new plan with all folders created upfront
#
# Usage: arc plan [--type TYPE] <plan-name> <phase1> [phase2] [phase3] ...
#
# Creates the entire folder structure so no prompts occur during implementation.

set -euo pipefail

ARC_HOME="${ARC_HOME:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
ARC_PROJECT_ROOT="${ARC_PROJECT_ROOT:-$(pwd)}"
ARC_PLANS_DIR="${ARC_PLANS_DIR:-$ARC_PROJECT_ROOT/.plans}"
ACTIVE_DIR="$ARC_PLANS_DIR/active"
TEMPLATE_DIR="$ARC_HOME/templates"
WORKFLOWS_DIR="$ARC_HOME/workflows"

VALID_TYPES="feature bugfix investigation refactor performance"

usage() {
    echo "Usage: arc plan [--type TYPE] <plan-name> <phase1> [phase2] ..."
    echo ""
    echo "Options:"
    echo "  --type TYPE  Workflow type: feature (default), bugfix, investigation, refactor, performance"
    echo ""
    echo "Examples:"
    echo "  arc plan auth-system 01-models 02-handlers 03-middleware"
    echo "  arc plan --type bugfix login-crash investigate fix"
    echo "  arc plan --type performance query-opt baseline analyze"
    exit 1
}

# Parse --type flag
WORKFLOW_TYPE="feature"
if [[ "${1:-}" == "--type" ]]; then
    if [[ -z "${2:-}" ]]; then
        echo "Error: --type requires a value" >&2
        echo "Valid types: $VALID_TYPES" >&2
        exit 1
    fi
    WORKFLOW_TYPE="$2"
    shift 2

    if ! echo "$VALID_TYPES" | grep -qw "$WORKFLOW_TYPE"; then
        echo "Error: Invalid workflow type '$WORKFLOW_TYPE'" >&2
        echo "Valid types: $VALID_TYPES" >&2
        exit 1
    fi

    if [[ ! -f "$WORKFLOWS_DIR/${WORKFLOW_TYPE}.yaml" ]]; then
        echo "Error: Workflow file not found: $WORKFLOWS_DIR/${WORKFLOW_TYPE}.yaml" >&2
        exit 1
    fi
fi

if [[ $# -lt 2 ]]; then
    usage
fi

PLAN_NAME="$1"
shift
USER_PHASES=("$@")

# Auto-add integration phase at the end
PHASES=("${USER_PHASES[@]}" "integration")

PLAN_DIR="$ACTIVE_DIR/$PLAN_NAME"

if [[ -d "$PLAN_DIR" ]]; then
    echo "Error: Plan '$PLAN_NAME' already exists at $PLAN_DIR"
    echo "Use a different name or remove the existing plan first."
    exit 1
fi

echo "Creating plan: $PLAN_NAME"
echo "Type: $WORKFLOW_TYPE"
echo "Phases: ${PHASES[*]}"
echo ""

mkdir -p "$PLAN_DIR/phases"

# Copy workflow file if not using default feature workflow
if [[ "$WORKFLOW_TYPE" != "feature" ]]; then
    cp "$WORKFLOWS_DIR/${WORKFLOW_TYPE}.yaml" "$PLAN_DIR/workflow.yaml"
    echo "Copied ${WORKFLOW_TYPE}.yaml workflow"
fi

# Generate session ID
SESSION_ID=$(uuidgen 2>/dev/null || cat /proc/sys/kernel/random/uuid)
echo "$SESSION_ID" > "$PLAN_DIR/session_id"

# Build dependencies (linear chain for user phases, integration depends on all)
DEPS="{}"
for ((i=1; i<${#USER_PHASES[@]}; i++)); do
    PREV="${USER_PHASES[$((i-1))]}"
    CURR="${USER_PHASES[$i]}"
    DEPS=$(echo "$DEPS" | jq --arg curr "$CURR" --arg prev "$PREV" '.[$curr] = [$prev]')
done

if [[ ${#USER_PHASES[@]} -gt 0 ]]; then
    USER_PHASES_JSON=$(printf '%s\n' "${USER_PHASES[@]}" | jq -R . | jq -s .)
    DEPS=$(echo "$DEPS" | jq --argjson deps "$USER_PHASES_JSON" '.["integration"] = $deps')
fi

# Build phase_order
PHASE_ORDER="{}"
for ((i=0; i<${#PHASES[@]}; i++)); do
    PHASE="${PHASES[$i]}"
    ORDER=$((i + 1))
    PHASE_ORDER=$(echo "$PHASE_ORDER" | jq --arg phase "$PHASE" --argjson order "$ORDER" '.[$phase] = $order')
done

# Create plan.json
PHASES_JSON=$(printf '%s\n' "${PHASES[@]}" | jq -R . | jq -s .)
cat > "$PLAN_DIR/plan.json" << EOF
{
  "name": "$PLAN_NAME",
  "created": "$(date -I)",
  "status": "active",
  "phases": $PHASES_JSON,
  "phase_order": $PHASE_ORDER,
  "dependencies": $DEPS
}
EOF

echo "Created plan.json"

# Create phase folders with state.json and plan.md
for PHASE in "${PHASES[@]}"; do
    PHASE_DIR="$PLAN_DIR/phases/$PHASE"
    mkdir -p "$PHASE_DIR"

    cat > "$PHASE_DIR/state.json" << EOF
{
  "phase": "$PHASE",
  "plan": "$PLAN_NAME",
  "workflow_type": "$WORKFLOW_TYPE",
  "phase_status": "pending",
  "iteration": {"current": 0, "max": 25},
  "chunks": {"total": 0, "completed": [], "current": null, "remaining": []},
  "blocked": {"is_blocked": false, "reason": null},
  "disputes": [],
  "last_cleared_disputes": [],
  "packages": [],
  "tests_passing": 0,
  "tests_total": 0,
  "stuck_iterations": 0,
  "hang_count": 0,
  "last_reviewed_iteration": 0,
  "last_qa_reviewed_iteration": 0
}
EOF

    if [[ "$PHASE" == "integration" && -f "$TEMPLATE_DIR/integration-template.md" ]]; then
        sed "s/\[PLAN_NAME\]/$PLAN_NAME/g" "$TEMPLATE_DIR/integration-template.md" > "$PHASE_DIR/plan.md"
    elif [[ -f "$TEMPLATE_DIR/plan-template.md" ]]; then
        sed "s/\[PHASE_NAME\]/$PHASE/g" "$TEMPLATE_DIR/plan-template.md" > "$PHASE_DIR/plan.md"
    else
        cat > "$PHASE_DIR/plan.md" << 'PLANEOF'
# Phase: PHASE_PLACEHOLDER

## Objective
<!-- One sentence describing what this phase accomplishes -->

## Files
### Create
<!-- - `path/to/file` — description -->

### Modify
<!-- - `path/to/file` — what changes -->

## Types and Signatures
```
// Exact signatures here
```

## DO NOT
- [ ] <!-- Common mistake to avoid -->

## Test Cases
### test_name
**Input:** ...
**Expected:** ...

## Edge Cases
1. <!-- Edge case -->
PLANEOF
        sed -i "s/PHASE_PLACEHOLDER/$PHASE/" "$PHASE_DIR/plan.md"
    fi

    echo "Created phase: $PHASE"
done

echo ""
echo "Plan initialized!"
echo ""
echo "Next steps:"
echo "  1. Edit phase plans: $PLAN_DIR/phases/*/plan.md"
echo "  2. Review the plan:  arc review $PLAN_NAME"
echo "  3. Execute the plan: arc run $PLAN_NAME"
