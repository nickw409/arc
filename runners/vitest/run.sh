#!/usr/bin/env bash
#
# vitest runner - Run tests via vitest
#
# Interface:
#   run.sh <test_file> [--filter PATTERN] [--timeout SECS] [--extra-args ARGS]
#
# Output (stdout): JSON object with results
#   { "total": N, "passed": N, "failed": N, "failed_names": [...], "raw_output": "..." }

set -euo pipefail

TEST_FILE="$1"
shift

FILTER=""
TIMEOUT=300
EXTRA_ARGS=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --filter)  FILTER="$2"; shift 2 ;;
        --timeout) TIMEOUT="$2"; shift 2 ;;
        --extra-args) EXTRA_ARGS="$2"; shift 2 ;;
        *) shift ;;
    esac
done

# Build command
CMD="npx vitest run $TEST_FILE --reporter=json"
[[ -n "$FILTER" ]] && CMD="$CMD -t \"$FILTER\""
[[ -n "$EXTRA_ARGS" ]] && CMD="$CMD $EXTRA_ARGS"

# Run with timeout
OUTPUT=$(timeout --foreground -k 10 "$TIMEOUT" bash -c "$CMD" 2>&1) || true
EXIT_CODE=$?

# Parse vitest JSON output
PASSED=0
FAILED=0
TOTAL=0

# Try to extract JSON from output (vitest may mix stdout)
JSON_OUTPUT=$(echo "$OUTPUT" | grep -oP '\{.*"testResults".*\}' | tail -1 || echo "")

if [[ -n "$JSON_OUTPUT" ]]; then
    PASSED=$(echo "$JSON_OUTPUT" | jq '.numPassedTests // 0' 2>/dev/null || echo "0")
    FAILED=$(echo "$JSON_OUTPUT" | jq '.numFailedTests // 0' 2>/dev/null || echo "0")
    TOTAL=$((PASSED + FAILED))
    FAILED_NAMES_JSON=$(echo "$JSON_OUTPUT" | jq '[.testResults[].assertionResults[] | select(.status == "failed") | .fullName] // []' 2>/dev/null || echo '[]')
else
    # Fallback: parse text output
    if echo "$OUTPUT" | grep -qE 'Tests\s+\d+'; then
        PASSED=$(echo "$OUTPUT" | grep -oP '(\d+)\s+passed' | grep -oP '\d+' || echo "0")
        FAILED=$(echo "$OUTPUT" | grep -oP '(\d+)\s+failed' | grep -oP '\d+' || echo "0")
        TOTAL=$((PASSED + FAILED))
    fi
    FAILED_NAMES_JSON='[]'
fi

jq -n \
    --argjson total "$TOTAL" \
    --argjson passed "$PASSED" \
    --argjson failed "$FAILED" \
    --argjson failed_names "${FAILED_NAMES_JSON:-[]}" \
    --arg raw "$OUTPUT" \
    '{total: $total, passed: $passed, failed: $failed, failed_names: $failed_names, raw_output: $raw}'

if [[ $TOTAL -eq 0 ]]; then
    exit 2
elif [[ $FAILED -gt 0 ]]; then
    exit 1
else
    exit 0
fi
