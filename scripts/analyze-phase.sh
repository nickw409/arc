#!/bin/bash
# analyze-phase.sh - Analyze if a phase needs splitting or help
#
# Returns exit codes:
#   0 = Phase looks healthy
#   1 = Recommend intervention  
#   2 = Strongly recommend split

set -e

ARC_SCRIPTS_DIR="${ARC_SCRIPTS_DIR:-$ARC_HOME/scripts}"
BASE_DIR="$(dirname "$(dirname "$ARC_SCRIPTS_DIR")")"
ARC_PLANS_DIR="${PLANS_DIR:-$BASE_DIR/.plans}"
ACTIVE_DIR="$ARC_PLANS_DIR/active"

if [ $# -lt 2 ]; then
    echo "Usage: $0 <plan> <phase>"
    exit 1
fi

PLAN="$1"
PHASE="$2"

STATE_FILE="$ACTIVE_DIR/$PLAN/phases/$PHASE/state.json"
LAST_TEST_OUTPUT="$ACTIVE_DIR/$PLAN/phases/$PHASE/last_test_output.txt"

if [ ! -f "$STATE_FILE" ]; then
    echo "Error: Phase state not found at $STATE_FILE"
    exit 1
fi

# Read state values
ITERATION=$(jq -r '.iteration.current // 0' "$STATE_FILE")
STATUS=$(jq -r '.phase_status // "unknown"' "$STATE_FILE")
TESTS_PASS=$(jq -r '.tests_passing // 0' "$STATE_FILE")
TESTS_TOTAL=$(jq -r '.tests_total // 0' "$STATE_FILE")
STUCK=$(jq -r '.stuck_iterations // 0' "$STATE_FILE")
HANG_COUNT=$(jq -r '.hang_count // 0' "$STATE_FILE")
ROLLBACK_COUNT=$(jq -r '.rollback_count // 0' "$STATE_FILE")
PACKAGES=$(jq -r '.packages | length' "$STATE_FILE")
IS_BLOCKED=$(jq -r '.blocked.is_blocked // false' "$STATE_FILE")
BLOCK_REASON=$(jq -r '.blocked.reason // ""' "$STATE_FILE")

echo "=== Phase Analysis: $PHASE ==="
echo "Status: $STATUS"
echo "Iteration: $ITERATION"
echo "Tests: $TESTS_PASS/$TESTS_TOTAL"
echo "Stuck iterations: $STUCK"
echo "Hang count: $HANG_COUNT"
echo "Rollback count: $ROLLBACK_COUNT/2"
echo "Packages affected: $PACKAGES"
if [ "$IS_BLOCKED" = "true" ]; then
    echo "Blocked: $BLOCK_REASON"
fi
echo ""

ISSUES=0
SEVERITY=0  # 0=ok, 1=warn, 2=critical

# Check 1: Stuck iterations (escalation ladder handles 3-5)
if [ "$STUCK" -ge 6 ]; then
    echo "🚨 CRITICAL: Stuck for $STUCK iterations (escalation ladder exhausted)"
    echo "   Recommendation: Manual intervention or split phase"
    ISSUES=$((ISSUES + 1))
    SEVERITY=2
elif [ "$STUCK" -ge 5 ]; then
    echo "⚠️  WARNING: Stuck for $STUCK iterations (auto-split attempted)"
    echo "   Recommendation: Review auto-split results or manually split"
    ISSUES=$((ISSUES + 1))
    [ $SEVERITY -lt 1 ] && SEVERITY=1
elif [ "$STUCK" -ge 3 ]; then
    echo "ℹ️  INFO: Stuck for $STUCK iterations (escalation ladder active)"
    echo "   Escalation: Level 3+ instructions, Level 4+ opus model"
fi

# Check 2: Multiple hangs
if [ "$HANG_COUNT" -ge 2 ]; then
    echo "🚨 CRITICAL: Phase has hung $HANG_COUNT times (10min timeout)"
    echo "   Recommendation: Phase too complex or infinite loop"
    ISSUES=$((ISSUES + 1))
    SEVERITY=2
elif [ "$HANG_COUNT" -ge 1 ]; then
    echo "⚠️  WARNING: Phase hung once"
    echo "   Recommendation: Monitor closely"
    ISSUES=$((ISSUES + 1))
    [ $SEVERITY -lt 1 ] && SEVERITY=1
fi

# Check 3: Many iterations with low progress
if [ "$ITERATION" -ge 10 ] && [ "$TESTS_TOTAL" -gt 0 ]; then
    PASS_RATE=$(awk "BEGIN {printf \"%.0f\", ($TESTS_PASS/$TESTS_TOTAL)*100}")
    if [ "$PASS_RATE" -lt 50 ]; then
        echo "🚨 CRITICAL: $ITERATION iterations but only $PASS_RATE% tests passing"
        echo "   Recommendation: Split phase - scope too broad"
        ISSUES=$((ISSUES + 1))
        SEVERITY=2
    elif [ "$PASS_RATE" -lt 80 ]; then
        echo "⚠️  WARNING: $ITERATION iterations, $PASS_RATE% tests passing"
        echo "   Recommendation: Identify blocking tests"
        ISSUES=$((ISSUES + 1))
        [ $SEVERITY -lt 1 ] && SEVERITY=1
    fi
fi

# Check 4: Multi-package complexity
if [ "$PACKAGES" -ge 3 ]; then
    echo "⚠️  WARNING: Phase affects $PACKAGES packages"
    echo "   Recommendation: Consider splitting by package"
    ISSUES=$((ISSUES + 1))
    [ $SEVERITY -lt 1 ] && SEVERITY=1
fi

# Check 5: Compilation errors across files
if [ -f "$LAST_TEST_OUTPUT" ]; then
    ERROR_FILES=$(grep -oE "error\[E[0-9]+\].*-->.*:[0-9]+" "$LAST_TEST_OUTPUT" 2>/dev/null | \
                  sed 's/.*--> //' | sed 's/:.*//' | sort -u | wc -l || echo "0")
    
    if [ -n "$ERROR_FILES" ] && [ "$ERROR_FILES" -ge 5 ]; then
        echo "🚨 CRITICAL: Compilation errors in $ERROR_FILES different files"
        echo "   Recommendation: Split by file/module groups"
        ISSUES=$((ISSUES + 1))
        SEVERITY=2
    elif [ -n "$ERROR_FILES" ] && [ "$ERROR_FILES" -ge 3 ]; then
        echo "⚠️  WARNING: Compilation errors in $ERROR_FILES files"
        echo "   Recommendation: Focus on one file at a time"
        ISSUES=$((ISSUES + 1))
        [ $SEVERITY -lt 1 ] && SEVERITY=1
    fi
fi

# Check 6: Blocked status
if [ "$IS_BLOCKED" = "true" ]; then
    echo "🚨 CRITICAL: Phase is blocked: $BLOCK_REASON"
    ISSUES=$((ISSUES + 1))
    SEVERITY=2
fi

# Check 7: Rollback count
if [ "$ROLLBACK_COUNT" -ge 2 ]; then
    echo "🚨 CRITICAL: Phase has been rolled back $ROLLBACK_COUNT times (max reached)"
    echo "   Next block will be permanent - human intervention required"
    ISSUES=$((ISSUES + 1))
    SEVERITY=2
elif [ "$ROLLBACK_COUNT" -ge 1 ]; then
    echo "⚠️  WARNING: Phase has been rolled back $ROLLBACK_COUNT time(s)"
    echo "   Recommendation: Review impl_reasoning.md for persistent issues"
    ISSUES=$((ISSUES + 1))
    [ $SEVERITY -lt 1 ] && SEVERITY=1
fi

echo ""
echo "=== Summary ==="
if [ $ISSUES -eq 0 ]; then
    echo "✓ Phase looks healthy - continue normal iteration"
    exit 0
elif [ $SEVERITY -eq 1 ]; then
    echo "⚠️  $ISSUES warning(s) - INTERVENTION RECOMMENDED"
    echo ""
    echo "Actions:"
    echo "  1. Read impl_reasoning.md"
    echo "  2. Provide targeted instructions to next iteration"
    echo "  3. If stuck persists after 2 iterations → split"
    exit 1
else
    echo "🚨 $ISSUES critical issue(s) - INTERVENTION REQUIRED"
    echo ""

    if [ "$ROLLBACK_COUNT" -ge 2 ]; then
        echo "Rollback exhausted ($ROLLBACK_COUNT/2). Options:"
        echo "  1. Manual fix: Review impl_reasoning.md, fix code, then:"
        echo "     update-state.sh $PLAN $PHASE reset-blocked"
        echo "  2. Split phase:"
        echo "     split-phase.sh $PLAN $PHASE <sub-phase1> <sub-phase2> ..."
        echo "  3. Skip (if non-critical):"
        echo "     update-state.sh $PLAN $PHASE status complete"
    else
        echo "Split strategies:"

        if [ "$PACKAGES" -ge 3 ]; then
            PKG_LIST=$(jq -r '.packages | join(", ")' "$STATE_FILE")
            echo "  • By package: $PKG_LIST"
        fi

        if [ -n "$ERROR_FILES" ] && [ "$ERROR_FILES" -ge 5 ]; then
            echo "  • By file/module (too many files modified)"
        fi

        if [ "$STUCK" -ge 5 ] || [ "$HANG_COUNT" -ge 2 ]; then
            echo "  • Manual fix first, then split remaining work"
        fi

        echo ""
        echo "Command:"
        echo "  split-phase.sh $PLAN $PHASE <sub-phase1> <sub-phase2> ..."
    fi
    exit 2
fi

