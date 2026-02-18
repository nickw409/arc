#!/usr/bin/env bash
#
# verify-phase-complete.sh - Verify a phase is ready to be marked complete
#
# Usage: verify-phase-complete.sh <plan-name> <phase>
#
# Checks:
#   1. test_files[] is not empty (tests are registered)
#   2. tests_total > 0 (tests were actually run)
#   3. tests_passing == tests_total (all tests pass)
#   4. No active disputes
#
# Exit codes:
#   0 - Phase is ready to be marked complete
#   1 - Phase is NOT ready (details in output)
#   2 - Usage error

set -euo pipefail

ARC_SCRIPTS_DIR="${ARC_SCRIPTS_DIR:-$ARC_HOME/scripts}"
PROJECT_ROOT="$(dirname "$(dirname "$ARC_SCRIPTS_DIR")")"
ARC_PLANS_DIR="${ARC_PLANS_DIR:-$ARC_PROJECT_ROOT/.plans}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

PLAN_NAME="${1:-}"
PHASE="${2:-}"

if [[ -z "$PLAN_NAME" || -z "$PHASE" ]]; then
    echo "Usage: verify-phase-complete.sh <plan-name> <phase>" >&2
    exit 2
fi

PLAN_DIR="$ARC_PLANS_DIR/active/$PLAN_NAME"
PHASE_DIR="$PLAN_DIR/phases/$PHASE"
STATE_FILE="$PHASE_DIR/state.json"

if [[ ! -f "$STATE_FILE" ]]; then
    echo -e "${RED}ERROR${NC}: State file not found: $STATE_FILE" >&2
    exit 1
fi

echo "=== Phase Completion Verification ==="
echo "Plan: $PLAN_NAME"
echo "Phase: $PHASE"
echo ""

CHECKS_PASSED=0
CHECKS_FAILED=0
FAILURES=()

check() {
    local name="$1"
    local result="$2"
    local details="${3:-}"

    echo -n "  $name: "
    if [[ "$result" == "pass" ]]; then
        echo -e "${GREEN}PASS${NC}"
        ((CHECKS_PASSED++)) || true
    else
        echo -e "${RED}FAIL${NC}"
        [[ -n "$details" ]] && FAILURES+=("$name: $details")
        ((CHECKS_FAILED++)) || true
    fi
}

# Read state values
TEST_FILES=$(jq -r '.test_files // [] | length' "$STATE_FILE" 2>/dev/null || echo "0")
TESTS_PASSING=$(jq -r '.tests_passing // 0' "$STATE_FILE" 2>/dev/null || echo "0")
TESTS_TOTAL=$(jq -r '.tests_total // 0' "$STATE_FILE" 2>/dev/null || echo "0")
DISPUTE=$(jq -r '.dispute // null' "$STATE_FILE" 2>/dev/null || echo "null")
DISPUTE_RESOLUTION=$(jq -r '.dispute.resolution // null' "$STATE_FILE" 2>/dev/null || echo "null")

echo "Current state:"
echo "  test_files count: $TEST_FILES"
echo "  tests_passing: $TESTS_PASSING"
echo "  tests_total: $TESTS_TOTAL"
echo "  dispute: $DISPUTE"
echo ""

echo "Checks:"

# 1. Test files registered
if [[ "$TEST_FILES" -gt 0 ]]; then
    check "Test files registered" "pass"
else
    check "Test files registered" "fail" "No test files in state.json test_files[]"
fi

# 2. Tests were run (tests_total > 0)
if [[ "$TESTS_TOTAL" -gt 0 ]]; then
    check "Tests were run" "pass"
else
    check "Tests were run" "fail" "tests_total is 0 - no tests have been executed"
fi

# 3. All tests pass
if [[ "$TESTS_TOTAL" -gt 0 && "$TESTS_PASSING" -eq "$TESTS_TOTAL" ]]; then
    check "All tests passing" "pass"
elif [[ "$TESTS_TOTAL" -eq 0 ]]; then
    check "All tests passing" "fail" "Cannot verify - no tests run"
else
    check "All tests passing" "fail" "$TESTS_PASSING/$TESTS_TOTAL passing"
fi

# 4. No active disputes
if [[ "$DISPUTE" == "null" ]]; then
    check "No active disputes" "pass"
elif [[ "$DISPUTE_RESOLUTION" != "null" ]]; then
    check "No active disputes" "pass" # Dispute was resolved
else
    DISPUTE_TEST=$(jq -r '.dispute.test_name // "unknown"' "$STATE_FILE" 2>/dev/null || echo "unknown")
    check "No active disputes" "fail" "Active dispute on test: $DISPUTE_TEST"
fi

# Summary
echo ""
echo "=== Summary ==="
echo -e "Passed: ${GREEN}$CHECKS_PASSED${NC}"
echo -e "Failed: ${RED}$CHECKS_FAILED${NC}"
echo ""

if [[ $CHECKS_FAILED -gt 0 ]]; then
    echo -e "${RED}PHASE NOT READY FOR COMPLETION${NC}"
    echo ""
    echo "Failures:"
    for failure in "${FAILURES[@]}"; do
        echo "  - $failure"
    done
    exit 1
else
    echo -e "${GREEN}PHASE READY FOR COMPLETION${NC}"
    exit 0
fi
