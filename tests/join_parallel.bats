#!/usr/bin/env bats

# Tests for join-parallel.sh
# Phase: join-strategies (orchestration-v5)
#
# Tests the join strategy evaluation functions that read parallel branch
# results (.exit files) and determine verdicts based on strategy
# (all, any, n_of_m).

setup() {
    load 'test_helper'
    setup_temp_dir

    JOIN_PARALLEL_SH="$SCRIPTS_DIR/join-parallel.sh"

    # Standard results directory for most tests
    export RESULTS_DIR="$TEST_TEMP_DIR/results"
    mkdir -p "$RESULTS_DIR"
}

teardown() {
    teardown_temp_dir
}

# ==============================================================================
# Helper: Create .exit files in a results directory
# Usage: create_exit_files <dir> <branch1>=<code> [branch2=<code> ...]
# Example: create_exit_files "$RESULTS_DIR" auth=0 db=0 api=1
# ==============================================================================
create_exit_files() {
    local dir="$1"
    shift
    mkdir -p "$dir"
    for entry in "$@"; do
        local branch="${entry%%=*}"
        local code="${entry#*=}"
        printf '%s\n' "$code" > "$dir/${branch}.exit"
    done
}

# ==============================================================================
# Script Existence and Syntax Tests
# ==============================================================================

@test "qa_join-strategies_script_exists: join-parallel.sh exists and is executable" {
    [[ -x "$JOIN_PARALLEL_SH" ]]
}

@test "qa_join-strategies_syntax_valid: script is syntactically valid bash" {
    run bash -n "$JOIN_PARALLEL_SH"
    [[ "$status" -eq 0 ]]
}

@test "qa_join-strategies_strict_mode: script uses set -euo pipefail" {
    run grep -q 'set -euo pipefail' "$JOIN_PARALLEL_SH"
    [[ "$status" -eq 0 ]]
}

# ==============================================================================
# count_results Tests
# ==============================================================================

@test "qa_join-strategies_count_results: counts succeeded/failed/total correctly" {
    create_exit_files "$RESULTS_DIR" a=0 b=0 c=1

    # Source the script to call count_results directly
    source "$JOIN_PARALLEL_SH"
    run count_results "$RESULTS_DIR"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "2 1 3" ]]
}

@test "qa_join-strategies_count_results_ignores_non_exit_files: only reads .exit files" {
    create_exit_files "$RESULTS_DIR" a=0 b=1

    # Create non-.exit files that should be ignored
    echo "log content" > "$RESULTS_DIR/a.log"
    echo "12345" > "$RESULTS_DIR/a.pid"
    echo "log content" > "$RESULTS_DIR/b.log"
    echo "67890" > "$RESULTS_DIR/b.pid"

    source "$JOIN_PARALLEL_SH"
    run count_results "$RESULTS_DIR"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "1 1 2" ]]
}

# ==============================================================================
# Strategy: all
# ==============================================================================

@test "qa_join-strategies_all_strategy_all_pass: all branches exit 0 gives all_complete" {
    create_exit_files "$RESULTS_DIR" auth=0 db=0 api=0

    run "$JOIN_PARALLEL_SH" all "$RESULTS_DIR"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "all_complete" ]]
}

@test "qa_join-strategies_all_strategy_one_fails: one non-zero exit gives any_failed" {
    create_exit_files "$RESULTS_DIR" auth=0 db=1 api=0

    run "$JOIN_PARALLEL_SH" all "$RESULTS_DIR"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "any_failed" ]]
}

@test "qa_join-strategies_all_strategy_all_fail: all non-zero exits give any_failed" {
    create_exit_files "$RESULTS_DIR" auth=1 db=1 api=1

    run "$JOIN_PARALLEL_SH" all "$RESULTS_DIR"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "any_failed" ]]
}

@test "qa_join-strategies_single_branch_all_pass: single branch exit 0 gives all_complete" {
    create_exit_files "$RESULTS_DIR" solo=0

    run "$JOIN_PARALLEL_SH" all "$RESULTS_DIR"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "all_complete" ]]
}

# ==============================================================================
# Strategy: any
# ==============================================================================

@test "qa_join-strategies_any_strategy_one_passes: one exit 0 gives first_complete" {
    create_exit_files "$RESULTS_DIR" auth=0 db=1 api=1

    run "$JOIN_PARALLEL_SH" any "$RESULTS_DIR"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "first_complete" ]]
}

@test "qa_join-strategies_any_strategy_all_fail: all non-zero gives all_failed" {
    create_exit_files "$RESULTS_DIR" auth=1 db=1 api=1

    run "$JOIN_PARALLEL_SH" any "$RESULTS_DIR"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "all_failed" ]]
}

@test "qa_join-strategies_any_strategy_all_pass: all exit 0 gives first_complete" {
    create_exit_files "$RESULTS_DIR" auth=0 db=0 api=0

    run "$JOIN_PARALLEL_SH" any "$RESULTS_DIR"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "first_complete" ]]
}

# ==============================================================================
# Strategy: n_of_m
# ==============================================================================

@test "qa_join-strategies_n_of_m_enough_pass: more than n pass gives n_complete" {
    create_exit_files "$RESULTS_DIR" a=0 b=0 c=0 d=1 e=1
    # 3 pass, n=2

    run "$JOIN_PARALLEL_SH" n_of_m "$RESULTS_DIR" 2
    [[ "$status" -eq 0 ]]
    [[ "$output" == "n_complete" ]]
}

@test "qa_join-strategies_n_of_m_exact_n: exactly n pass gives n_complete" {
    create_exit_files "$RESULTS_DIR" a=0 b=0 c=1 d=1 e=1
    # 2 pass, n=2

    run "$JOIN_PARALLEL_SH" n_of_m "$RESULTS_DIR" 2
    [[ "$status" -eq 0 ]]
    [[ "$output" == "n_complete" ]]
}

@test "qa_join-strategies_n_of_m_insufficient: fewer than n pass gives insufficient" {
    create_exit_files "$RESULTS_DIR" a=0 b=1 c=1 d=1 e=1
    # 1 pass, n=2

    run "$JOIN_PARALLEL_SH" n_of_m "$RESULTS_DIR" 2
    [[ "$status" -eq 0 ]]
    [[ "$output" == "insufficient" ]]
}

@test "qa_join-strategies_n_of_m_n_greater_than_total: n > total gives insufficient" {
    create_exit_files "$RESULTS_DIR" a=0 b=0
    # 2 pass, but n=5

    run "$JOIN_PARALLEL_SH" n_of_m "$RESULTS_DIR" 5
    [[ "$status" -eq 0 ]]
    [[ "$output" == "insufficient" ]]
}

@test "qa_join-strategies_n_of_m_n_equals_zero: n=0 always gives n_complete" {
    create_exit_files "$RESULTS_DIR" a=1 b=1 c=1
    # All fail, but n=0 is trivially satisfied

    run "$JOIN_PARALLEL_SH" n_of_m "$RESULTS_DIR" 0
    [[ "$status" -eq 0 ]]
    [[ "$output" == "n_complete" ]]
}

@test "qa_join-strategies_n_of_m_n_equals_total: n = total with all passing gives n_complete" {
    create_exit_files "$RESULTS_DIR" a=0 b=0 c=0
    # 3 pass, n=3 (equivalent to "all")

    run "$JOIN_PARALLEL_SH" n_of_m "$RESULTS_DIR" 3
    [[ "$status" -eq 0 ]]
    [[ "$output" == "n_complete" ]]
}

# ==============================================================================
# Exit Code Handling: Timeout and Crash Codes
# ==============================================================================

@test "qa_join-strategies_timeout_counts_as_failure: exit 124 counted as failure" {
    create_exit_files "$RESULTS_DIR" a=0 b=124 c=0

    run "$JOIN_PARALLEL_SH" all "$RESULTS_DIR"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "any_failed" ]]
}

@test "qa_join-strategies_crash_exit_code_counts_as_failure: SIGKILL (137) and generic error (2) are failures" {
    create_exit_files "$RESULTS_DIR" a=0 b=137 c=2

    run "$JOIN_PARALLEL_SH" all "$RESULTS_DIR"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "any_failed" ]]
}

# ==============================================================================
# Exit File Content Edge Cases
# ==============================================================================

@test "qa_join-strategies_exit_file_with_trailing_whitespace: whitespace trimmed from exit code" {
    mkdir -p "$RESULTS_DIR"
    # Write exit files with various whitespace
    printf ' 0 \n' > "$RESULTS_DIR/a.exit"
    printf '0\n' > "$RESULTS_DIR/b.exit"
    printf '  1  \n' > "$RESULTS_DIR/c.exit"

    run "$JOIN_PARALLEL_SH" all "$RESULTS_DIR"
    [[ "$status" -eq 0 ]]
    # a=0, b=0, c=1 => any_failed
    [[ "$output" == "any_failed" ]]
}

@test "qa_join-strategies_exit_file_with_non_integer: non-integer content treated as failure" {
    mkdir -p "$RESULTS_DIR"
    printf '0\n' > "$RESULTS_DIR/a.exit"
    printf 'error\n' > "$RESULTS_DIR/b.exit"

    run "$JOIN_PARALLEL_SH" all "$RESULTS_DIR"
    [[ "$status" -eq 0 ]]
    # a=0, b=failure => any_failed
    [[ "$output" == "any_failed" ]]
}

@test "qa_join-strategies_non_integer_counted_in_any: non-integer file counted as failure for any strategy" {
    mkdir -p "$RESULTS_DIR"
    printf 'error\n' > "$RESULTS_DIR/a.exit"
    printf 'crash\n' > "$RESULTS_DIR/b.exit"

    run "$JOIN_PARALLEL_SH" any "$RESULTS_DIR"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "all_failed" ]]
}

# ==============================================================================
# Error Cases: Invalid Arguments
# ==============================================================================

@test "qa_join-strategies_invalid_strategy: unknown strategy exits 1 with error" {
    create_exit_files "$RESULTS_DIR" a=0

    run "$JOIN_PARALLEL_SH" unknown "$RESULTS_DIR"
    [[ "$status" -eq 1 ]]
    [[ "${output,,}" == *"invalid strategy"* ]]
}

@test "qa_join-strategies_missing_n_for_n_of_m: n_of_m without n argument exits 1" {
    create_exit_files "$RESULTS_DIR" a=0

    run "$JOIN_PARALLEL_SH" n_of_m "$RESULTS_DIR"
    [[ "$status" -eq 1 ]]
    # stderr should contain an error about missing n
    [[ "$output" == *[Ee]rror* ]] || [[ "$output" == *[Uu]sage* ]] || [[ "$output" == *"n"* ]]
}

@test "qa_join-strategies_missing_results_dir_argument: no results-dir exits 1" {
    run "$JOIN_PARALLEL_SH" all
    [[ "$status" -eq 1 ]]
    # stderr should mention missing arguments
    [[ "$output" == *[Ee]rror* ]] || [[ "$output" == *[Uu]sage* ]] || [[ "$output" == *"argument"* ]]
}

@test "qa_join-strategies_n_of_m_non_integer_n: n='two' exits 1" {
    create_exit_files "$RESULTS_DIR" a=0

    run "$JOIN_PARALLEL_SH" n_of_m "$RESULTS_DIR" "two"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *[Ee]rror* ]] || [[ "$output" == *"n"* ]] || [[ "$output" == *"integer"* ]]
}

@test "qa_join-strategies_n_of_m_negative_n: n=-1 exits 1" {
    create_exit_files "$RESULTS_DIR" a=0

    run "$JOIN_PARALLEL_SH" n_of_m "$RESULTS_DIR" "-1"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *[Ee]rror* ]] || [[ "$output" == *"n"* ]]
}

# ==============================================================================
# Error Cases: Results Directory
# ==============================================================================

@test "qa_join-strategies_empty_results_dir: no .exit files exits 2" {
    # RESULTS_DIR exists but has no .exit files
    run "$JOIN_PARALLEL_SH" all "$RESULTS_DIR"
    [[ "$status" -eq 2 ]]
}

@test "qa_join-strategies_nonexistent_results_dir: missing directory exits 2" {
    run "$JOIN_PARALLEL_SH" all "$TEST_TEMP_DIR/nonexistent"
    [[ "$status" -eq 2 ]]
}

@test "qa_join-strategies_empty_dir_with_non_exit_files: dir with only .log/.pid files exits 2" {
    # Results dir has files but none are .exit files
    echo "log" > "$RESULTS_DIR/a.log"
    echo "123" > "$RESULTS_DIR/a.pid"

    run "$JOIN_PARALLEL_SH" all "$RESULTS_DIR"
    [[ "$status" -eq 2 ]]
}

# ==============================================================================
# No-argument invocation
# ==============================================================================

@test "qa_join-strategies_no_arguments: no arguments exits 1" {
    run "$JOIN_PARALLEL_SH"
    [[ "$status" -eq 1 ]]
}
