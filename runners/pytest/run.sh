#!/usr/bin/env bash
#
# pytest runner - Run tests via pytest
#
# Interface:
#   run.sh <test_file> [--filter PATTERN] [--timeout SECS] [--extra-args ARGS]
#
# Output (stdout): JSON object with results

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
CMD="python -m pytest $TEST_FILE -v --tb=short"
[[ -n "$FILTER" ]] && CMD="$CMD -k \"$FILTER\""
[[ -n "$EXTRA_ARGS" ]] && CMD="$CMD $EXTRA_ARGS"

OUTPUT=$(timeout --foreground -k 10 "$TIMEOUT" bash -c "$CMD" 2>&1) || true

# Parse pytest output
PASSED=0
FAILED=0
TOTAL=0

# pytest summary: "X passed, Y failed" or "X passed"
if echo "$OUTPUT" | grep -qE '\d+ passed'; then
    PASSED=$(echo "$OUTPUT" | grep -oP '(\d+) passed' | grep -oP '\d+' | tail -1 || echo "0")
fi
if echo "$OUTPUT" | grep -qE '\d+ failed'; then
    FAILED=$(echo "$OUTPUT" | grep -oP '(\d+) failed' | grep -oP '\d+' | tail -1 || echo "0")
fi
TOTAL=$((PASSED + FAILED))

# Extract failed test names: "FAILED tests/test_foo.py::test_bar"
FAILED_NAMES=$(echo "$OUTPUT" | grep -oP '(?<=FAILED )\S+' || true)
FAILED_NAMES_JSON=$(echo "$FAILED_NAMES" | grep -v '^$' | jq -R . | jq -s . 2>/dev/null || echo '[]')

jq -n \
    --argjson total "$TOTAL" \
    --argjson passed "$PASSED" \
    --argjson failed "$FAILED" \
    --argjson failed_names "$FAILED_NAMES_JSON" \
    --arg raw "$OUTPUT" \
    '{total: $total, passed: $passed, failed: $failed, failed_names: $failed_names, raw_output: $raw}'

if [[ $TOTAL -eq 0 ]]; then
    exit 2
elif [[ $FAILED -gt 0 ]]; then
    exit 1
else
    exit 0
fi
