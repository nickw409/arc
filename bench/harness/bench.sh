#!/usr/bin/env bash
# Main benchmark orchestrator.
# Usage:
#   bench.sh                           # Run all tasks, all approaches, all runs
#   bench.sh --task task1-feature      # Run only one task
#   bench.sh --approach claude-minimal # Run only one approach
#   bench.sh --runs 1                  # Override number of runs
#   bench.sh --collect-only            # Skip running, just collect metrics
#   bench.sh --report-only             # Skip running and collecting, just report
#   bench.sh --dry-run                 # Print what would be run

set -euo pipefail
source "$(dirname "$0")/config.sh"

# Parse arguments
FILTER_TASK=""
FILTER_APPROACH=""
OVERRIDE_RUNS=""
COLLECT_ONLY=false
REPORT_ONLY=false
DRY_RUN=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --task) FILTER_TASK="${2:?--task requires an argument}"; shift 2 ;;
        --approach) FILTER_APPROACH="${2:?--approach requires an argument}"; shift 2 ;;
        --runs) OVERRIDE_RUNS="${2:?--runs requires an argument}"; shift 2 ;;
        --collect-only) COLLECT_ONLY=true; shift ;;
        --report-only) REPORT_ONLY=true; shift ;;
        --dry-run) DRY_RUN=true; shift ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

ACTUAL_RUNS=${OVERRIDE_RUNS:-$RUNS_PER}

# Determine what to run
RUN_TASKS=("${TASKS[@]}")
if [ -n "$FILTER_TASK" ]; then
    RUN_TASKS=("$FILTER_TASK")
fi

RUN_APPROACHES=("${APPROACHES[@]}")
if [ -n "$FILTER_APPROACH" ]; then
    RUN_APPROACHES=("$FILTER_APPROACH")
fi

TOTAL_RUNS=$(( ${#RUN_TASKS[@]} * ${#RUN_APPROACHES[@]} * ACTUAL_RUNS ))

echo "=== Arc Benchmark Suite ==="
echo "Tasks:      ${RUN_TASKS[*]}"
echo "Approaches: ${RUN_APPROACHES[*]}"
echo "Runs each:  $ACTUAL_RUNS"
echo "Total runs: $TOTAL_RUNS"
echo ""

if $DRY_RUN; then
    echo "[DRY RUN] Would execute:"
    for task in "${RUN_TASKS[@]}"; do
        for approach in "${RUN_APPROACHES[@]}"; do
            for run in $(seq 1 "$ACTUAL_RUNS"); do
                echo "  $task / $approach / run-$run"
            done
        done
    done
    exit 0
fi

if $REPORT_ONLY; then
    echo "[REPORT ONLY] Generating report from existing results..."
    bash "$(dirname "$0")/report.sh"
    exit 0
fi

# Phase 1: Run approaches (unless collect-only)
if ! $COLLECT_ONLY; then
    COMPLETED=0
    for task in "${RUN_TASKS[@]}"; do
        for approach in "${RUN_APPROACHES[@]}"; do
            for run in $(seq 1 "$ACTUAL_RUNS"); do
                COMPLETED=$((COMPLETED + 1))
                echo ""
                echo "[$COMPLETED/$TOTAL_RUNS] Running $task / $approach / run-$run"
                echo "---"

                RUN_DIR=$(bash "$(dirname "$0")/run-approach.sh" "$task" "$approach" "$run") || {
                    echo "  WARNING: run failed, continuing..."
                }

                echo ""
            done
        done
    done
fi

# Phase 2: Collect metrics
echo ""
echo "=== Collecting Metrics ==="
for task in "${RUN_TASKS[@]}"; do
    for approach in "${RUN_APPROACHES[@]}"; do
        for run in $(seq 1 "$ACTUAL_RUNS"); do
            RUN_DIR="$WORKDIR/${task}/${approach}/run-${run}"
            if [ -f "$RUN_DIR/run-meta.json" ]; then
                bash "$(dirname "$0")/collect.sh" "$RUN_DIR" || {
                    echo "  WARNING: collection failed for $RUN_DIR"
                }
            fi
        done
    done
done

# Phase 3: Generate report
echo ""
echo "=== Generating Report ==="
bash "$(dirname "$0")/report.sh"

echo ""
echo "=== Benchmark Complete ==="
