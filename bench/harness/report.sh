#!/usr/bin/env bash
# Generates a comparison report from all benchmark results.
# Usage: report.sh [results-dir]
# Reads all results.json files and produces a summary table.

set -euo pipefail
source "$(dirname "$0")/config.sh"

SCAN_DIR="${1:-$WORKDIR}"
OUTPUT="$RESULTS_DIR/report-$(date +%Y%m%d-%H%M%S).md"
mkdir -p "$RESULTS_DIR"

# Collect all results files
RESULT_FILES=()
while IFS= read -r f; do
    RESULT_FILES+=("$f")
done < <(find "$SCAN_DIR" -name "results.json" -type f | sort)

if [ ${#RESULT_FILES[@]} -eq 0 ]; then
    echo "No results found in $SCAN_DIR"
    exit 1
fi

echo "Found ${#RESULT_FILES[@]} result files"

# Merge all results into one JSON array
MERGED=$(jq -s '.' "${RESULT_FILES[@]}")

cat > "$OUTPUT" <<'HEADER'
# Arc Benchmark Report

## Summary

Comparison of three approaches across benchmark tasks:
- **arc**: Full Arc orchestration with phased execution
- **claude-minimal**: Claude Code with task spec only
- **claude-plan**: Claude Code with task spec + detailed plan

HEADER

# --- Per-task comparison tables ---
for TASK in "${TASKS[@]}"; do
    echo "" >> "$OUTPUT"
    echo "## $TASK" >> "$OUTPUT"
    echo "" >> "$OUTPUT"

    echo "| Metric | arc | claude-minimal | claude-plan |" >> "$OUTPUT"
    echo "|--------|-----|----------------|-------------|" >> "$OUTPUT"

    for METRIC in elapsed_ms hidden_tests_pass hidden_tests_total existing_tests_pass coverage_pct vet_issues lines_added lines_removed input_tokens output_tokens cost_usd; do
        ROW="| $METRIC"
        for APPROACH in arc claude-minimal claude-plan; do
            # Average across runs
            AVG=$(echo "$MERGED" | jq -r "[.[] | select(.task==\"$TASK\" and .approach==\"$APPROACH\") | .$METRIC] | if length > 0 then (add / length | . * 100 | round / 100) else \"N/A\" end")
            ROW+=" | $AVG"
        done
        ROW+=" |"
        echo "$ROW" >> "$OUTPUT"
    done
done

# --- Aggregate comparison ---
echo "" >> "$OUTPUT"
echo "## Aggregate (All Tasks)" >> "$OUTPUT"
echo "" >> "$OUTPUT"
echo "| Metric | arc | claude-minimal | claude-plan |" >> "$OUTPUT"
echo "|--------|-----|----------------|-------------|" >> "$OUTPUT"

for METRIC in elapsed_ms hidden_tests_pass coverage_pct vet_issues lines_added input_tokens output_tokens cost_usd; do
    ROW="| $METRIC (avg)"
    for APPROACH in arc claude-minimal claude-plan; do
        AVG=$(echo "$MERGED" | jq -r "[.[] | select(.approach==\"$APPROACH\") | .$METRIC] | if length > 0 then (add / length | . * 100 | round / 100) else \"N/A\" end")
        ROW+=" | $AVG"
    done
    ROW+=" |"
    echo "$ROW" >> "$OUTPUT"
done

# --- Hidden test pass rate ---
echo "" >> "$OUTPUT"
echo "## Hidden Test Pass Rate (Correctness Score)" >> "$OUTPUT"
echo "" >> "$OUTPUT"
echo "| Task | arc | claude-minimal | claude-plan |" >> "$OUTPUT"
echo "|------|-----|----------------|-------------|" >> "$OUTPUT"

for TASK in "${TASKS[@]}"; do
    ROW="| $TASK"
    for APPROACH in arc claude-minimal claude-plan; do
        RATE=$(echo "$MERGED" | jq -r "
            [.[] | select(.task==\"$TASK\" and .approach==\"$APPROACH\") |
             if .hidden_tests_total > 0 then (.hidden_tests_pass / .hidden_tests_total * 100) else 0 end
            ] | if length > 0 then (add / length | . * 10 | round / 10) else \"N/A\" end
        ")
        ROW+=" | ${RATE}%"
    done
    ROW+=" |"
    echo "$ROW" >> "$OUTPUT"
done

# --- Verdict ---
echo "" >> "$OUTPUT"
echo "## Verdict" >> "$OUTPUT"
echo "" >> "$OUTPUT"

# Calculate overall scores
for APPROACH in arc claude-minimal claude-plan; do
    HIDDEN_RATE=$(echo "$MERGED" | jq -r "
        [.[] | select(.approach==\"$APPROACH\") |
         if .hidden_tests_total > 0 then (.hidden_tests_pass / .hidden_tests_total * 100) else 0 end
        ] | if length > 0 then (add / length | . * 10 | round / 10) else 0 end
    ")
    AVG_TIME=$(echo "$MERGED" | jq -r "[.[] | select(.approach==\"$APPROACH\") | .elapsed_ms] | if length > 0 then (add / length / 1000 | . * 10 | round / 10) else \"N/A\" end")
    AVG_COST=$(echo "$MERGED" | jq -r "[.[] | select(.approach==\"$APPROACH\") | .cost_usd] | if length > 0 then (add / length | . * 10000 | round / 10000) else \"N/A\" end")

    echo "- **$APPROACH**: ${HIDDEN_RATE}% correctness, avg ${AVG_TIME}s, avg \$${AVG_COST}/run" >> "$OUTPUT"
done

echo "" >> "$OUTPUT"
echo "---" >> "$OUTPUT"
echo "*Generated: $(date -u +%Y-%m-%dT%H:%M:%SZ)*" >> "$OUTPUT"

echo ""
echo "Report written to: $OUTPUT"
echo ""
cat "$OUTPUT"
