#!/usr/bin/env bash
# Collects metrics for a completed run.
# Usage: collect.sh <run-dir>
# Reads the run-meta.json, runs hidden tests, gathers coverage/lint stats,
# parses token usage from logs, and writes results.json.

set -euo pipefail
source "$(dirname "$0")/config.sh"

RUN_DIR="$1"
PROJECT_DIR="$RUN_DIR/tkit"
LOG_DIR="$RUN_DIR/logs"
META_FILE="$RUN_DIR/run-meta.json"

TASK=$(jq -r '.task' "$META_FILE")
APPROACH=$(jq -r '.approach' "$META_FILE")
RUN_NUM=$(jq -r '.run' "$META_FILE")
ELAPSED_MS=$(jq -r '.elapsed_ms' "$META_FILE")
EXIT_CODE=$(jq -r '.exit_code' "$META_FILE")

echo "=== Collecting metrics: $TASK / $APPROACH / run-$RUN_NUM ==="

# --- 1. Existing tests ---
echo "  [1/6] Running existing tests..."
EXISTING_TESTS_PASS=0
EXISTING_TESTS_TOTAL=0
cd "$PROJECT_DIR"
if go test ./... -json > "$LOG_DIR/existing-tests.json" 2>/dev/null; then
    EXISTING_TESTS_PASS=$(jq -s '[.[] | select(.Action=="pass" and .Test!=null)] | length' "$LOG_DIR/existing-tests.json")
    EXISTING_TESTS_TOTAL=$(jq -s '[.[] | select(.Action=="pass" and .Test!=null or .Action=="fail" and .Test!=null)] | length' "$LOG_DIR/existing-tests.json")
else
    EXISTING_TESTS_TOTAL=$(jq -s '[.[] | select(.Action=="pass" and .Test!=null or .Action=="fail" and .Test!=null)] | length' "$LOG_DIR/existing-tests.json")
    EXISTING_TESTS_PASS=$(jq -s '[.[] | select(.Action=="pass" and .Test!=null)] | length' "$LOG_DIR/existing-tests.json")
fi
echo "    existing tests: $EXISTING_TESTS_PASS / $EXISTING_TESTS_TOTAL passed"

# --- 2. Hidden tests ---
echo "  [2/6] Running hidden tests..."
HIDDEN_TESTS_PASS=0
HIDDEN_TESTS_TOTAL=0
HIDDEN_BUILD_FAILURES=0

# Count expected test functions from hidden test source files BEFORE running.
# This ensures we show the correct denominator even when packages fail to build.
TASK_DIR="$TASKS_DIR/$TASK"
EXPECTED_HIDDEN=0
for hidden in "$TASK_DIR"/hidden_*.go; do
    [ -f "$hidden" ] || continue
    count=$(grep -c '^func TestHidden' "$hidden" || true)
    EXPECTED_HIDDEN=$((EXPECTED_HIDDEN + count))
done

# Copy hidden tests into the project
for hidden in "$TASK_DIR"/hidden_*.go; do
    [ -f "$hidden" ] || continue
    BASENAME=$(basename "$hidden")

    # Determine destination package from the file's package declaration
    PKG=$(head -5 "$hidden" | grep '^package ' | awk '{print $2}')
    case "$PKG" in
        store)  DEST="$PROJECT_DIR/internal/store/" ;;
        filter) DEST="$PROJECT_DIR/internal/filter/" ;;
        model)  DEST="$PROJECT_DIR/internal/model/" ;;
        cli)    DEST="$PROJECT_DIR/internal/cli/" ;;
        *)      DEST="$PROJECT_DIR/" ;;
    esac
    cp "$hidden" "$DEST/$BASENAME"
done

if go test ./... -json -run 'TestHidden|BenchmarkHidden' > "$LOG_DIR/hidden-tests.json" 2>/dev/null; then
    HIDDEN_TESTS_PASS=$(jq -s '[.[] | select(.Action=="pass" and .Test!=null)] | length' "$LOG_DIR/hidden-tests.json")
    HIDDEN_TESTS_TOTAL=$(jq -s '[.[] | select(.Action=="pass" and .Test!=null or .Action=="fail" and .Test!=null)] | length' "$LOG_DIR/hidden-tests.json")
else
    HIDDEN_TESTS_TOTAL=$(jq -s '[.[] | select(.Action=="pass" and .Test!=null or .Action=="fail" and .Test!=null)] | length' "$LOG_DIR/hidden-tests.json" 2>/dev/null || echo 0)
    HIDDEN_TESTS_PASS=$(jq -s '[.[] | select(.Action=="pass" and .Test!=null)] | length' "$LOG_DIR/hidden-tests.json" 2>/dev/null || echo 0)
fi

# Detect build failures: packages that failed to compile have no test results
# but show "[build failed]" in their output lines
if [ -f "$LOG_DIR/hidden-tests.json" ]; then
    HIDDEN_BUILD_FAILURES=$(jq -s '[.[] | select(.Action=="output" and .Test==null and (.Output | test("\\[build failed\\]")))] | length' "$LOG_DIR/hidden-tests.json" 2>/dev/null || echo 0)
fi

# Use expected count as denominator when build failures hide tests from the count.
# This prevents "0/0" when a package can't build — shows "0/12" instead.
if [ "$HIDDEN_TESTS_TOTAL" -lt "$EXPECTED_HIDDEN" ]; then
    HIDDEN_TESTS_TOTAL=$EXPECTED_HIDDEN
fi

if [ "$HIDDEN_BUILD_FAILURES" -gt 0 ]; then
    echo "    hidden tests: $HIDDEN_TESTS_PASS / $HIDDEN_TESTS_TOTAL passed ($HIDDEN_BUILD_FAILURES package build failure(s))"
else
    echo "    hidden tests: $HIDDEN_TESTS_PASS / $HIDDEN_TESTS_TOTAL passed"
fi

# Clean up hidden tests so they don't affect other metrics
for hidden in "$TASK_DIR"/hidden_*.go; do
    [ -f "$hidden" ] || continue
    BASENAME=$(basename "$hidden")
    find "$PROJECT_DIR" -name "$BASENAME" -delete 2>/dev/null || true
done

# --- 3. Test coverage ---
echo "  [3/6] Measuring test coverage..."
COVERAGE=0
if go test ./... -coverprofile="$LOG_DIR/coverage.out" > /dev/null 2>&1; then
    COVERAGE=$(go tool cover -func="$LOG_DIR/coverage.out" | tail -1 | awk '{print $3}' | tr -d '%')
fi
echo "    coverage: ${COVERAGE}%"

# --- 4. Code quality (go vet + staticcheck if available) ---
echo "  [4/6] Running code quality checks..."
VET_ISSUES=0
if ! go vet ./... > "$LOG_DIR/vet.log" 2>&1; then
    VET_ISSUES=$(wc -l < "$LOG_DIR/vet.log")
fi
echo "    vet issues: $VET_ISSUES"

# --- 5. Diff stats ---
# Diff against the bench-baseline tag to capture ALL changes (committed and uncommitted)
# regardless of how many intermediate commits the agent made.
echo "  [5/6] Calculating diff stats..."
LINES_ADDED=0
LINES_REMOVED=0
FILES_CHANGED=0

# Combine committed + uncommitted changes by diffing working tree against baseline
# Exclude arc metadata files (.arc.yaml, .plans/, .claude/) from the diff
FULL_DIFF=$(git -C "$PROJECT_DIR" diff bench-baseline -- . \
    ':!.arc.yaml' ':!.plans/' ':!.claude/' ':!.gitignore' 2>/dev/null || echo "")
if [ -n "$FULL_DIFF" ]; then
    LINES_ADDED=$(echo "$FULL_DIFF" | grep -c '^+[^+]' || true)
    LINES_REMOVED=$(echo "$FULL_DIFF" | grep -c '^-[^-]' || true)
    FILES_CHANGED=$(git -C "$PROJECT_DIR" diff --name-only bench-baseline -- . \
        ':!.arc.yaml' ':!.plans/' ':!.claude/' ':!.gitignore' 2>/dev/null | wc -l || echo 0)
fi
echo "    diff: +$LINES_ADDED -$LINES_REMOVED across $FILES_CHANGED files"

# --- 6. Token usage (from claude JSON output) ---
echo "  [6/6] Parsing token usage..."
INPUT_TOKENS=0
OUTPUT_TOKENS=0
COST_USD="0"

if [ "$APPROACH" = "arc" ]; then
    # Arc tracks usage per phase in state.json files.
    # Aggregate across all phases for total input/output tokens and cost.
    PLAN_NAME="bench-${TASK}"
    PLAN_DIR="$PROJECT_DIR/.plans/active/$PLAN_NAME"
    if [ -d "$PLAN_DIR/phases" ]; then
        for state_file in "$PLAN_DIR"/phases/*/state.json; do
            [ -f "$state_file" ] || continue
            if jq -e '.usage' "$state_file" > /dev/null 2>&1; then
                phase_in=$(jq '(.usage.input_tokens // 0) + (.usage.cache_creation_input_tokens // 0) + (.usage.cache_read_input_tokens // 0)' "$state_file" 2>/dev/null || echo 0)
                phase_out=$(jq '.usage.output_tokens // 0' "$state_file" 2>/dev/null || echo 0)
                phase_cost=$(jq '.usage.cost_usd // 0' "$state_file" 2>/dev/null || echo 0)
                INPUT_TOKENS=$((INPUT_TOKENS + phase_in))
                OUTPUT_TOKENS=$((OUTPUT_TOKENS + phase_out))
                COST_USD=$(echo "$COST_USD + $phase_cost" | bc 2>/dev/null || echo "$COST_USD")
            fi
        done
    fi
elif [ -f "$LOG_DIR/stdout.log" ]; then
    # Claude Code --output-format json produces a single JSON object with usage data.
    # Input tokens include direct + cache_creation + cache_read for true total.
    # Cost comes from total_cost_usd which accounts for actual models and cache pricing.
    if jq -e '.usage' "$LOG_DIR/stdout.log" > /dev/null 2>&1; then
        INPUT_TOKENS=$(jq '(.usage.input_tokens // 0) + (.usage.cache_creation_input_tokens // 0) + (.usage.cache_read_input_tokens // 0)' "$LOG_DIR/stdout.log" 2>/dev/null || echo 0)
        OUTPUT_TOKENS=$(jq '.usage.output_tokens // 0' "$LOG_DIR/stdout.log" 2>/dev/null || echo 0)
    fi

    # Use total_cost_usd from Claude if available (accurate across models and cache tiers)
    if jq -e '.total_cost_usd' "$LOG_DIR/stdout.log" > /dev/null 2>&1; then
        COST_USD=$(jq '.total_cost_usd' "$LOG_DIR/stdout.log" 2>/dev/null || echo "0")
    elif [ "$INPUT_TOKENS" != "0" ] || [ "$OUTPUT_TOKENS" != "0" ]; then
        # Fallback estimate (Sonnet pricing: $3/M input, $15/M output)
        COST_USD=$(echo "scale=4; ($INPUT_TOKENS * 3 + $OUTPUT_TOKENS * 15) / 1000000" | bc 2>/dev/null || echo "0")
    fi
fi
echo "    tokens: ${INPUT_TOKENS} in / ${OUTPUT_TOKENS} out (cost: \$${COST_USD})"

# --- Write results ---
RESULTS_FILE="$RUN_DIR/results.json"
cat > "$RESULTS_FILE" <<EOF
{
  "task": "$TASK",
  "approach": "$APPROACH",
  "run": $RUN_NUM,
  "elapsed_ms": $ELAPSED_MS,
  "exit_code": $EXIT_CODE,
  "existing_tests_pass": $EXISTING_TESTS_PASS,
  "existing_tests_total": $EXISTING_TESTS_TOTAL,
  "hidden_tests_pass": $HIDDEN_TESTS_PASS,
  "hidden_tests_total": $HIDDEN_TESTS_TOTAL,
  "hidden_build_failures": $HIDDEN_BUILD_FAILURES,
  "coverage_pct": $COVERAGE,
  "vet_issues": $VET_ISSUES,
  "lines_added": $LINES_ADDED,
  "lines_removed": $LINES_REMOVED,
  "files_changed": $FILES_CHANGED,
  "input_tokens": $INPUT_TOKENS,
  "output_tokens": $OUTPUT_TOKENS,
  "cost_usd": $COST_USD
}
EOF

echo "  results: $RESULTS_FILE"
