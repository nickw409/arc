#!/usr/bin/env bash
#
# go-test runner - Run tests via go test
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

# Extract package path from test file
PKG_DIR=$(dirname "$TEST_FILE")

# Make relative to current directory if absolute
if [[ "$PKG_DIR" == /* ]]; then
    PKG_DIR=$(realpath --relative-to="$(pwd)" "$PKG_DIR" 2>/dev/null || echo "$PKG_DIR")
fi

PKG_PATH="./$PKG_DIR"

# Build command
CMD="go test $PKG_PATH -v -count=1"
[[ -n "$FILTER" ]] && CMD="$CMD -run \"$FILTER\""
[[ -n "$EXTRA_ARGS" ]] && CMD="$CMD $EXTRA_ARGS"

OUTPUT=$(timeout --foreground -k 10 "$TIMEOUT" bash -c "$CMD" 2>&1) || true

# Parse go test output
PASSED=$(echo "$OUTPUT" | grep -c -e '--- PASS:' || true)
FAILED=$(echo "$OUTPUT" | grep -c -e '--- FAIL:' || true)
TOTAL=$((PASSED + FAILED))

# Extract failed test names as JSON array
FAILED_NAMES_JSON="[]"
if [[ $FAILED -gt 0 ]]; then
    FAILED_NAMES=$(echo "$OUTPUT" | grep -oP '(?<=--- FAIL: )\S+' || true)
    if [[ -n "$FAILED_NAMES" ]]; then
        FAILED_NAMES_JSON=$(echo "$FAILED_NAMES" | jq -R . | jq -s .)
    fi
fi

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
