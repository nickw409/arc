#!/usr/bin/env bash
#
# status.sh - Quick overview of plan phases
#
# Usage: arc status [plan-name] [phase]

set -euo pipefail

ARC_HOME="${ARC_HOME:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
ARC_PROJECT_ROOT="${ARC_PROJECT_ROOT:-$(pwd)}"
ARC_PLANS_DIR="${ARC_PLANS_DIR:-$ARC_PROJECT_ROOT/.plans}"
ACTIVE_DIR="$ARC_PLANS_DIR/active"

PLAN_NAME="${1:-}"
PHASE="${2:-}"

if [[ -z "$PLAN_NAME" ]]; then
    echo "Usage: arc status <plan-name> [phase]"
    echo ""
    echo "Available plans:"
    ls "$ACTIVE_DIR" 2>/dev/null | sed 's/^/  /' || echo "  (none)"
    exit 1
fi

PLAN_DIR="$ACTIVE_DIR/$PLAN_NAME"
PHASES_DIR="$PLAN_DIR/phases"

if [[ ! -d "$PLAN_DIR" ]]; then
    echo "Error: Plan '$PLAN_NAME' not found"
    exit 1
fi

show_phase() {
    local phase="$1"
    local state_file="$PHASES_DIR/$phase/state.json"

    if [[ ! -f "$state_file" ]]; then
        printf "  %-30s %s\n" "$phase:" "not started"
        return
    fi

    local status=$(jq -r '.phase_status // "pending"' "$state_file")
    local iter=$(jq -r '.iteration.current // 0' "$state_file")
    local max=$(jq -r '.iteration.max // 25' "$state_file")
    local dispute_count=$(jq -r '.disputes // [] | length' "$state_file")
    local tests_pass=$(jq -r '.tests_passing // 0' "$state_file")
    local tests_total=$(jq -r '.tests_total // 0' "$state_file")
    local rollback_count=$(jq -r '.rollback_count // 0' "$state_file")

    local icon="[ ]"
    case "$status" in
        complete) icon="[x]" ;;
        split) icon="[/]" ;;
        deferred) icon="[~]" ;;
        qa|qa_review|implementing|impl_review) icon="[>]" ;;
        disputed) icon="[!]" ;;
        blocked) icon="[X]" ;;
        pending) icon="[ ]" ;;
    esac

    printf "  %s %-26s %s" "$icon" "$phase" "$status"

    if [[ "$status" =~ ^(qa|qa_review|implementing|impl_review)$ ]]; then
        printf " (iter %s/%s)" "$iter" "$max"
    fi

    if [[ "$tests_total" -gt 0 ]]; then
        printf " tests: %s/%s" "$tests_pass" "$tests_total"
    fi

    if [[ "$dispute_count" -gt 0 ]]; then
        printf " [DISPUTES: %s]" "$dispute_count"
    fi

    if [[ "$status" != "complete" && -f "$PLAN_DIR/plan.json" ]]; then
        local deps
        deps=$(jq -r ".dependencies[\"$phase\"] // [] | .[]" "$PLAN_DIR/plan.json" 2>/dev/null)
        if [[ -n "$deps" ]]; then
            local incomplete_deps=()
            for dep in $deps; do
                local dep_state="$PHASES_DIR/$dep/state.json"
                local dep_status="pending"
                [[ -f "$dep_state" ]] && dep_status=$(jq -r '.phase_status // "pending"' "$dep_state")
                [[ "$dep_status" != "complete" ]] && incomplete_deps+=("$dep")
            done
            if [[ ${#incomplete_deps[@]} -gt 0 ]]; then
                printf " [BLOCKED BY: %s]" "${incomplete_deps[*]}"
            fi
        fi
    fi

    if [[ "$status" == "split" ]]; then
        local split_into=$(jq -r '.split_into // [] | join(", ")' "$state_file")
        [[ -n "$split_into" ]] && printf " -> %s" "$split_into"
    fi

    [[ "$rollback_count" -gt 0 ]] && printf " [ROLLBACK: %s/2]" "$rollback_count"

    echo ""
}

if [[ -n "$PHASE" ]]; then
    state_file="$PHASES_DIR/$PHASE/state.json"

    echo "=== Phase: $PHASE ==="
    echo ""

    if [[ -f "$state_file" ]]; then
        echo "State:"
        jq . "$state_file"
    else
        echo "State: not started"
    fi

    echo ""
    plan_file="$PHASES_DIR/$PHASE/plan.md"
    if [[ -f "$plan_file" ]]; then
        echo "Plan file: $plan_file"
        echo "Plan objective:"
        head -20 "$plan_file" | grep -A2 "^## Objective" || head -5 "$plan_file"
    fi
else
    echo "=== Plan: $PLAN_NAME ==="
    echo ""

    if [[ -f "$PLAN_DIR/plan.json" ]]; then
        phases=$(jq -r 'if .phase_order then
            [.phases[] as $p | {n: $p, o: .phase_order[$p]}] | sort_by(.o) | .[].n
          else .phases[] end' "$PLAN_DIR/plan.json" 2>/dev/null)

        for phase in $phases; do
            show_phase "$phase"
        done
    else
        echo "No plan.json found"
    fi

    echo ""

    total=$(jq -r '.phases | length' "$PLAN_DIR/plan.json" 2>/dev/null || echo "0")
    complete=$(find "$PHASES_DIR" -name "state.json" -exec grep -l '"phase_status": "complete"' {} \; 2>/dev/null | wc -l || echo "0")

    echo "Progress: $complete/$total phases complete"

    # Show review status if available
    review_status=$(jq -r '.review_status // empty' "$PLAN_DIR/plan.json" 2>/dev/null)
    if [[ -n "$review_status" ]]; then
        review_iters=$(jq -r '.review_iterations // 0' "$PLAN_DIR/plan.json" 2>/dev/null)
        reviewed_at=$(jq -r '.reviewed_at // empty' "$PLAN_DIR/plan.json" 2>/dev/null)

        echo ""
        echo "=== Review Status ==="

        # Status with color
        case "$review_status" in
            approved)
                echo -e "  Status: \033[0;32m$review_status\033[0m (iteration $review_iters/5)" ;;
            needs_review)
                echo -e "  Status: \033[0;31m$review_status\033[0m (iteration $review_iters/5)" ;;
            conditional)
                echo -e "  Status: \033[1;33m$review_status\033[0m (iteration $review_iters/5)" ;;
            regression_detected)
                echo -e "  Status: \033[0;31m$review_status\033[0m (iteration $review_iters/5)" ;;
            *)
                echo "  Status: $review_status (iteration $review_iters/5)" ;;
        esac

        [[ -n "$reviewed_at" ]] && echo "  Last reviewed: $reviewed_at"

        # Per-adversary results
        review_results=$(jq -r '.review_results // empty' "$PLAN_DIR/plan.json" 2>/dev/null)
        if [[ -n "$review_results" && "$review_results" != "null" ]]; then
            echo "  Adversaries:"
            jq -r '.review_results | to_entries[] | "    \(.key): \(.value)"' "$PLAN_DIR/plan.json" 2>/dev/null | \
            while read -r line; do
                if echo "$line" | grep -q "passed"; then
                    echo -e "\033[0;32m$line\033[0m"
                elif echo "$line" | grep -q "failed\|error"; then
                    echo -e "\033[0;31m$line\033[0m"
                elif echo "$line" | grep -q "warning"; then
                    echo -e "\033[1;33m$line\033[0m"
                else
                    echo "$line"
                fi
            done
        fi

        # Show review note if present (e.g., auto-approved message)
        review_note=$(jq -r '.review_note // empty' "$PLAN_DIR/plan.json" 2>/dev/null)
        [[ -n "$review_note" ]] && echo "  Note: $review_note"
    fi

    echo ""
    echo "=== Git Status ==="
    git status --short 2>/dev/null | head -10 || echo "(not a git repo)"
fi
