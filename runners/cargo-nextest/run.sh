#!/usr/bin/env bash
#
# cargo-nextest runner - Run tests via cargo nextest
#
# Interface:
#   run.sh <test_file> [--filter PATTERN] [--timeout SECS] [--extra-args ARGS]
#
# Output (stdout): JSON object with results
#   { "total": N, "passed": N, "failed": N, "failed_names": [...], "raw_output": "..." }
#
# Exit codes:
#   0 - All tests passed
#   1 - Some tests failed
#   2 - No tests found or error

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

CARGO_BIN=$(command -v cargo 2>/dev/null || echo "$HOME/.cargo/bin/cargo")

# Extract crate from path: <crate>/tests/<name>.rs
CRATE=$(echo "$TEST_FILE" | grep -oP '[^/]+(?=/tests/)' || echo "")
TEST_NAME=$(basename "$TEST_FILE" .rs)

if [[ -z "$CRATE" ]]; then
    echo '{"total":0,"passed":0,"failed":0,"failed_names":[],"raw_output":"Cannot determine crate from path"}'
    exit 2
fi

# Build command
CMD="$CARGO_BIN nextest run -p $CRATE --test $TEST_NAME --no-fail-fast"
[[ -n "$FILTER" ]] && CMD="$CMD -E \"test($FILTER)\""
[[ -n "$EXTRA_ARGS" ]] && CMD="$CMD $EXTRA_ARGS"

# Run with timeout
OUTPUT=$(timeout --foreground -k 10 "$TIMEOUT" bash -c "$CMD" 2>&1) || true
EXIT_CODE=$?

# Parse nextest output
PASSED=0
FAILED=0
TOTAL=0

if echo "$OUTPUT" | grep -q "passed"; then
    PASSED=$(echo "$OUTPUT" | grep -oP '\d+(?= passed)' | tail -1 || echo "0")
    FAILED=$(echo "$OUTPUT" | grep -oP '\d+(?= failed)' | tail -1 || echo "0")
    TOTAL=$((PASSED + FAILED))
fi

# Extract failed test names
FAILED_NAMES=$(echo "$OUTPUT" | grep -oP '(?<=FAIL \[)[^\]]+\] \S+ \K\S+' || true)
FAILED_NAMES_JSON=$(echo "$FAILED_NAMES" | grep -v '^$' | jq -R . | jq -s . 2>/dev/null || echo '[]')

# Output JSON result
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
