#!/usr/bin/env bash
set -euo pipefail

# arc/scripts/join-parallel.sh
#
# Evaluate parallel branch results and determine verdict.
#
# Usage: join-parallel.sh <strategy> <results-dir> [n]
#
# Reads .exit files from results directory (created by run-parallel.sh).
# Each .exit file contains a single bare integer (e.g., "0\n" or "1\n").
# Evaluates join condition based on strategy. Outputs verdict to stdout.
#
# Arguments:
#   strategy      "all", "any", or "n_of_m"
#   results_dir   Directory containing <branch>.exit files
#   n             Required for n_of_m: minimum branches that must succeed
#
# Verdicts:
#   strategy=all:    "all_complete" (all exit=0) or "any_failed"
#   strategy=any:    "first_complete" (any exit=0) or "all_failed"
#   strategy=n_of_m: "n_complete" (>=n exit=0) or "insufficient"
#
# Exit codes:
#   0 = Verdict determined successfully
#   1 = Invalid strategy, missing arguments, or invalid n value
#   2 = Results directory not found or no .exit files found

# count_results <results_dir>
#
# Count succeeded/failed branches by reading all .exit files.
# Exit code 0 = success. Any non-zero exit code = failure.
# Only reads files matching *.exit pattern (ignores .log, .pid).
#
# Output to stdout: single line "succeeded failed total" (space-separated integers)
count_results() {
    local results_dir="$1"
    local succeeded=0
    local failed=0
    local total=0

    shopt -s nullglob
    local exit_files=("$results_dir"/*.exit)
    shopt -u nullglob

    for exit_file in "${exit_files[@]}"; do
        local raw_content
        raw_content=$(<"$exit_file")
        # Trim whitespace
        local code
        code=$(echo "$raw_content" | tr -d '[:space:]')

        # Check if it's a valid non-negative integer
        if [[ "$code" =~ ^[0-9]+$ ]] && [[ "$code" -eq 0 ]]; then
            ((succeeded++))
        else
            ((failed++))
        fi
        ((total++))
    done

    echo "$succeeded $failed $total"
}

# evaluate_all <results_dir>
# Output to stdout: "all_complete" or "any_failed"
evaluate_all() {
    local results_dir="$1"
    local succeeded failed total
    read -r succeeded failed total <<< "$(count_results "$results_dir")"

    if [[ "$failed" -eq 0 ]]; then
        echo "all_complete"
    else
        echo "any_failed"
    fi
}

# evaluate_any <results_dir>
# Output to stdout: "first_complete" or "all_failed"
evaluate_any() {
    local results_dir="$1"
    local succeeded failed total
    read -r succeeded failed total <<< "$(count_results "$results_dir")"

    if [[ "$succeeded" -gt 0 ]]; then
        echo "first_complete"
    else
        echo "all_failed"
    fi
}

# evaluate_n_of_m <results_dir> <n>
# Output to stdout: "n_complete" or "insufficient"
# n must be a non-negative integer. Non-integer or negative n causes exit 1.
evaluate_n_of_m() {
    local results_dir="$1"
    local n="$2"

    # Validate n is a non-negative integer
    if [[ ! "$n" =~ ^[0-9]+$ ]]; then
        echo "Error: n must be a non-negative integer, got '$n'" >&2
        return 1
    fi

    local succeeded failed total
    read -r succeeded failed total <<< "$(count_results "$results_dir")"

    if [[ "$succeeded" -ge "$n" ]]; then
        echo "n_complete"
    else
        echo "insufficient"
    fi
}

# =============================================================================
# Main — only runs when script is executed directly, not when sourced
# =============================================================================
main() {
    if [[ $# -lt 2 ]]; then
        echo "Error: missing arguments. Usage: join-parallel.sh <strategy> <results-dir> [n]" >&2
        exit 1
    fi

    local strategy="$1"
    local results_dir="$2"

    # Validate results directory exists
    if [[ ! -d "$results_dir" ]]; then
        echo "Error: results directory not found: $results_dir" >&2
        exit 2
    fi

    # Check for .exit files
    shopt -s nullglob
    local exit_files=("$results_dir"/*.exit)
    shopt -u nullglob

    if [[ ${#exit_files[@]} -eq 0 ]]; then
        echo "Error: no .exit files found in $results_dir" >&2
        exit 2
    fi

    case "$strategy" in
        all)
            evaluate_all "$results_dir"
            ;;
        any)
            evaluate_any "$results_dir"
            ;;
        n_of_m)
            if [[ $# -lt 3 ]]; then
                echo "Error: n_of_m strategy requires n argument. Usage: join-parallel.sh n_of_m <results-dir> <n>" >&2
                exit 1
            fi
            local n="$3"
            # Validate n before calling evaluate
            if [[ ! "$n" =~ ^[0-9]+$ ]]; then
                echo "Error: n must be a non-negative integer, got '$n'" >&2
                exit 1
            fi
            evaluate_n_of_m "$results_dir" "$n"
            ;;
        *)
            echo "Error: invalid strategy '$strategy'. Must be one of: all, any, n_of_m" >&2
            exit 1
            ;;
    esac
}

# Only run main when executed directly (not sourced)
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
