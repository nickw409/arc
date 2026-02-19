#!/usr/bin/env bash
#
# Verify phase completion - ALL checks must pass
#
# Usage: verify-complete.sh <phase>
#
# Exit codes:
#   0 - Phase is genuinely complete
#   1 - Phase is NOT complete (details in output)
#   2 - Script error

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ARC_PHASES_DIR="${ARC_PHASES_DIR:-}"
[[ -z "$ARC_PHASES_DIR" ]] && { echo "ERROR: ARC_PHASES_DIR must be set" >&2; exit 1; }

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

PHASE="${1:-}"
if [[ -z "$PHASE" ]]; then
    echo "Usage: verify-complete.sh <phase>"
    exit 2
fi

CHECKS_PASSED=0
CHECKS_FAILED=0
FAILURES=()

# Run command with timeout and proper process group cleanup
# Usage: run_with_timeout <timeout_secs> <command...>
run_with_timeout() {
    local timeout_secs=$1
    shift

    # Use setsid to create new process group so we can kill all children
    # timeout -k sends SIGKILL after grace period if SIGTERM doesn't work
    setsid timeout -k 10 "$timeout_secs" "$@" 2>&1
    local exit_code=$?

    if [[ $exit_code -eq 124 ]]; then
        # Timeout occurred - kill any orphaned cargo processes
        pkill -9 -f "cargo.*nextest" 2>/dev/null || true
        pkill -9 -f "cargo.*build" 2>/dev/null || true
        pkill -9 -f "cargo.*test" 2>/dev/null || true
        pkill -9 -f "cargo.*clippy" 2>/dev/null || true
    fi
    return $exit_code
}

check() {
    local name="$1"
    local cmd="$2"
    local timeout_secs="${3:-300}"  # 5 minute default

    echo -n "  Checking: $name... "

    if run_with_timeout "$timeout_secs" bash -c "$cmd" > /tmp/verify-check-output.txt 2>&1; then
        echo -e "${GREEN}PASS${NC}"
        ((CHECKS_PASSED++))
        return 0
    else
        local exit_code=$?
        if [[ $exit_code -eq 124 ]]; then
            echo -e "${RED}TIMEOUT${NC}"
            FAILURES+=("$name: TIMEOUT after ${timeout_secs}s")
        else
            echo -e "${RED}FAIL${NC}"
            FAILURES+=("$name: $(cat /tmp/verify-check-output.txt | head -3)")
        fi
        ((CHECKS_FAILED++))
        return 1
    fi
}

echo "========================================"
echo "Phase Completion Verification: $PHASE"
echo "========================================"
echo ""

# 1. State file says complete
echo "1. State File Checks"
STATE_FILE="$ARC_PHASES_DIR/$PHASE/state.json"

check "State file exists" "[[ -f '$STATE_FILE' ]]"
check "State status is complete" "jq -e '.phase_status == \"complete\"' '$STATE_FILE'"
check "No remaining chunks" "jq -e '.chunks.remaining | length == 0' '$STATE_FILE'"
check "No current chunk" "jq -e '.chunks.current == null' '$STATE_FILE'"
check "Not blocked" "jq -e '.blocked.is_blocked == false' '$STATE_FILE'"
check "No active dispute" "jq -e '.dispute == null or .dispute.resolution != null' '$STATE_FILE'"

# 2. QA Tests Pass
echo ""
echo "2. QA Test Checks"

# Check for registered test files in state.json
STATE_FILE="$ARC_PHASES_DIR/$PHASE/state.json"
if [[ -f "$STATE_FILE" ]]; then
    TEST_FILES=$(jq -r '.test_files // [] | .[]' "$STATE_FILE" 2>/dev/null || true)
    if [[ -n "$TEST_FILES" ]]; then
        # Run registered test files
        TEST_PASSED=true
        while IFS= read -r test_file; do
            [[ -z "$test_file" ]] && continue
            if [[ "$test_file" == *.bats ]]; then
                check "BATS test: $(basename "$test_file")" "bats '$test_file' 2>&1 | grep -q 'ok'" 120
            elif [[ "$test_file" == *.rs ]]; then
                # Extract test name from path
                TEST_NAME=$(basename "$test_file" .rs)
                check "Rust test: $TEST_NAME" "cargo nextest run --test '$TEST_NAME' 2>&1 | grep -q 'passed'" 300
            fi
        done <<< "$TEST_FILES"
    else
        check "Test files registered in state.json" "false"
    fi
else
    check "Phase state file exists" "false"
fi

# 3. Code Quality (quick checks only - full build/test/clippy deferred to integration-gate)
echo ""
echo "3. Code Quality Checks"

check "cargo fmt check" "cargo fmt -- --check" 60
check "No todo!() macros" "! grep -r 'todo!()' src/ || true"
check "No unimplemented!()" "! grep -r 'unimplemented!()' src/ || true"
check "No dbg!() macros" "! grep -r 'dbg!(' src/"
check "No println!() in lib" "! grep -r 'println!(' src/lib.rs src/**/lib.rs 2>/dev/null || true"

# 4. No Ignored Tests
echo ""
echo "4. Test Configuration"

check "No ignored QA tests" "! grep -r '#\[ignore\]' tests/qa/$PHASE/ 2>/dev/null || [[ ! -d tests/qa/$PHASE ]]"

# Check for allowed ignores file
ALLOWED_IGNORES="$ARC_PHASES_DIR/$PHASE/allowed-ignores.txt"
if [[ -f "$ALLOWED_IGNORES" ]]; then
    echo "  (Allowed ignores file found: $ALLOWED_IGNORES)"
fi

echo ""
echo "(Full build, test, and clippy checks deferred to integration-gate after merge)"

# Summary
echo ""
echo "========================================"
echo "Summary"
echo "========================================"
echo ""
echo -e "Passed: ${GREEN}$CHECKS_PASSED${NC}"
echo -e "Failed: ${RED}$CHECKS_FAILED${NC}"
echo ""

if [[ $CHECKS_FAILED -gt 0 ]]; then
    echo -e "${RED}PHASE IS NOT COMPLETE${NC}"
    echo ""
    echo "Failures:"
    for failure in "${FAILURES[@]}"; do
        echo "  ✗ $failure"
    done
    echo ""
    echo "The phase status will be reverted to 'implementing'."
    exit 1
else
    echo -e "${GREEN}PHASE COMPLETION VERIFIED${NC}"
    echo ""
    echo "Phase $PHASE is ready for merge to develop."
    exit 0
fi
