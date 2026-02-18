#!/usr/bin/env bash
#
# run-phase-tests.sh - Run tests for a phase using the configured runner
#
# Usage: run-phase-tests.sh <plan-name> <phase> [--filter PATTERN] [--timeout SECS]
#
# This is the ONLY way agents should run tests. It:
# - Reads test_files[] from state.json
# - Dispatches to the runner configured in .arc.yaml
# - Updates state.json with results
# - Handles process cleanup
#
# Exit codes:
#   0 - All tests passed
#   1 - Some tests failed
#   2 - No tests found
#   124 - Timeout

set -euo pipefail

ARC_HOME="${ARC_HOME:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
ARC_PROJECT_ROOT="${ARC_PROJECT_ROOT:-$(pwd)}"
ARC_SCRIPTS_DIR="$ARC_HOME/scripts"
ARC_PLANS_DIR="${ARC_PLANS_DIR:-$ARC_PROJECT_ROOT/.plans}"

# Load project config
source "$ARC_SCRIPTS_DIR/config.sh"

# Defaults
TEST_TIMEOUT="${TEST_TIMEOUT:-300}"
FILTER=""
RETRY_FAILED=false

# Parse arguments
PLAN_NAME=""
PHASE=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --filter|-f)     FILTER="$2"; shift 2 ;;
        --timeout|-t)    TEST_TIMEOUT="$2"; shift 2 ;;
        --retry-failed|-r) RETRY_FAILED=true; shift ;;
        --help|-h)
            echo "Usage: run-phase-tests.sh <plan-name> <phase> [--filter PATTERN] [--timeout SECS] [--retry-failed]"
            exit 0
            ;;
        -*)              echo "Unknown option: $1" >&2; exit 1 ;;
        *)
            if [[ -z "$PLAN_NAME" ]]; then
                PLAN_NAME="$1"
            elif [[ -z "$PHASE" ]]; then
                PHASE="$1"
            fi
            shift
            ;;
    esac
done

if [[ -z "$PLAN_NAME" || -z "$PHASE" ]]; then
    echo "Usage: run-phase-tests.sh <plan-name> <phase> [--filter PATTERN]" >&2
    exit 1
fi

PLAN_DIR="$ARC_PLANS_DIR/active/$PLAN_NAME"
PHASE_DIR="$PLAN_DIR/phases/$PHASE"
STATE_FILE="$PHASE_DIR/state.json"

if [[ ! -f "$STATE_FILE" ]]; then
    echo "Error: State file not found: $STATE_FILE" >&2
    exit 2
fi

FAILED_TESTS_FILE="$PHASE_DIR/failed_tests.txt"

# Handle --retry-failed
if [[ "$RETRY_FAILED" == "true" ]]; then
    if [[ ! -f "$FAILED_TESTS_FILE" ]]; then
        echo "No failed tests file found. Run tests first."
        exit 0
    fi
    FAILED_LIST=$(cat "$FAILED_TESTS_FILE" | tr '\n' '|' | sed 's/|$//')
    [[ -z "$FAILED_LIST" ]] && { echo "No failed tests recorded."; exit 0; }
    echo "Retrying $(wc -l < "$FAILED_TESTS_FILE") failed test(s)"
    FILTER="$FAILED_LIST"
fi

# Read test files from state.json
TEST_FILES=$(jq -r '.test_files // [] | .[]' "$STATE_FILE" 2>/dev/null || true)
TEST_EXTRA_ARGS=$(jq -r '.test_extra_args // ""' "$STATE_FILE" 2>/dev/null || true)

if [[ -z "$TEST_FILES" ]]; then
    echo "No test files registered in state.json"
    echo "Register tests with: arc iterate ... then the agent adds test files"
    "$ARC_SCRIPTS_DIR/update-state.sh" "$PLAN_NAME" "$PHASE" tests 0 0
    exit 2
fi

# Resolve runner
RUNNER_SCRIPT="$ARC_RUNNER_DIR/run.sh"
if [[ ! -x "$RUNNER_SCRIPT" ]]; then
    echo "Error: Runner script not found: $RUNNER_SCRIPT" >&2
    echo "Check your .arc.yaml runner setting." >&2
    exit 1
fi

echo "=== Running Phase Tests ==="
echo "Plan: $PLAN_NAME"
echo "Phase: $PHASE"
echo "Runner: $ARC_RUNNER"
echo "Timeout: ${TEST_TIMEOUT}s"
[[ -n "$FILTER" ]] && echo "Filter: $FILTER"
[[ -n "$TEST_EXTRA_ARGS" ]] && echo "Extra args: $TEST_EXTRA_ARGS"
echo "==========================="
echo ""

# Track results
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0
TEST_OUTPUT=""
FAILED_TEST_NAMES=""

while IFS= read -r test_file; do
    [[ -z "$test_file" ]] && continue

    # Resolve relative paths
    if [[ "$test_file" != /* ]]; then
        test_file="$ARC_PROJECT_ROOT/$test_file"
    fi

    if [[ ! -f "$test_file" ]]; then
        echo "Warning: Test file not found: $test_file"
        continue
    fi

    echo "-> $(basename "$test_file")"

    # Build runner args
    RUNNER_ARGS=("$test_file")
    [[ -n "$FILTER" ]] && RUNNER_ARGS+=(--filter "$FILTER")
    RUNNER_ARGS+=(--timeout "$TEST_TIMEOUT")
    [[ -n "$TEST_EXTRA_ARGS" ]] && RUNNER_ARGS+=(--extra-args "$TEST_EXTRA_ARGS")

    # Run via runner plugin
    RESULT=$("$RUNNER_SCRIPT" "${RUNNER_ARGS[@]}" 2>/dev/null) || true

    # Parse JSON result from runner
    FILE_TOTAL=$(echo "$RESULT" | jq -r '.total // 0' 2>/dev/null || echo "0")
    FILE_PASSED=$(echo "$RESULT" | jq -r '.passed // 0' 2>/dev/null || echo "0")
    FILE_FAILED=$(echo "$RESULT" | jq -r '.failed // 0' 2>/dev/null || echo "0")
    RAW_OUTPUT=$(echo "$RESULT" | jq -r '.raw_output // ""' 2>/dev/null || echo "")

    TOTAL_TESTS=$((TOTAL_TESTS + FILE_TOTAL))
    PASSED_TESTS=$((PASSED_TESTS + FILE_PASSED))
    FAILED_TESTS=$((FAILED_TESTS + FILE_FAILED))

    # Collect failed test names
    FILE_FAILED_NAMES=$(echo "$RESULT" | jq -r '.failed_names // [] | .[]' 2>/dev/null || true)
    [[ -n "$FILE_FAILED_NAMES" ]] && FAILED_TEST_NAMES+="$FILE_FAILED_NAMES"$'\n'

    TEST_OUTPUT+="$RAW_OUTPUT"$'\n'

    # Show summary for this file
    if [[ $FILE_TOTAL -gt 0 ]]; then
        if [[ $FILE_FAILED -eq 0 ]]; then
            echo "  OK $FILE_PASSED/$FILE_TOTAL passed"
        else
            echo "  FAIL $FILE_PASSED/$FILE_TOTAL passed ($FILE_FAILED failed)"
        fi
    fi
    echo ""

done <<< "$TEST_FILES"

# Save output
TEST_OUTPUT_DIR="$PHASE_DIR/test_output"
mkdir -p "$TEST_OUTPUT_DIR"
ITERATION=$(jq -r '.iteration.current // 0' "$STATE_FILE")
echo "$TEST_OUTPUT" > "$TEST_OUTPUT_DIR/iteration_${ITERATION}.txt"
echo "$TEST_OUTPUT" > "$PHASE_DIR/last_test_output.txt"

# Save failed test names
if [[ -n "$FAILED_TEST_NAMES" ]]; then
    echo "$FAILED_TEST_NAMES" | grep -v '^$' | sort -u > "$FAILED_TESTS_FILE"
else
    rm -f "$FAILED_TESTS_FILE"
fi

# Update state
if [[ $TOTAL_TESTS -gt 0 ]]; then
    "$ARC_SCRIPTS_DIR/update-state.sh" "$PLAN_NAME" "$PHASE" tests "$PASSED_TESTS" "$TOTAL_TESTS"
fi

# Summary
echo "==========================="
echo "Results: $PASSED_TESTS/$TOTAL_TESTS passed"
[[ $FAILED_TESTS -gt 0 ]] && echo "Failed: $FAILED_TESTS"
echo "Output: $PHASE_DIR/last_test_output.txt"
echo "==========================="

if [[ $TOTAL_TESTS -eq 0 ]]; then
    exit 2
elif [[ $FAILED_TESTS -gt 0 ]]; then
    exit 1
else
    exit 0
fi
