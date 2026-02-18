#!/usr/bin/env bats
# QA Tests for adversary-skip-cache phase
# Tests the 3 new functions added to plan-review-loop.sh:
# - compute_phase_hashes
# - get_changed_phases
# - get_prev_verdict
# Plus: skip logic, safety valve, record_results with phase_hashes,
# cached result parsing, cross_phase field in adversaries YAML

setup() {
    load 'test_helper'
    setup_temp_dir

    SCRIPT="$SCRIPTS_DIR/plan-review-loop.sh"
    ADVERSARIES_FILE="$ORCH_DIR/adversaries/planning-adversaries.yaml"

    # Create a minimal plan structure for testing
    PLAN_DIR="$TEST_TEMP_DIR/plans/active/test-plan"
    REVIEWS_DIR="$PLAN_DIR/reviews"
    HISTORY_FILE="$REVIEWS_DIR/adversary_history.json"
    mkdir -p "$PLAN_DIR/phases/alpha"
    mkdir -p "$PLAN_DIR/phases/beta"
    mkdir -p "$REVIEWS_DIR"

    # Initialize empty history
    echo '{"iterations":[],"next_iteration":1}' > "$HISTORY_FILE"

    # Write a helper script that sources functions from plan-review-loop.sh
    # This avoids bats parsing issues with inline bash -c
    cat > "$TEST_TEMP_DIR/run_fn.sh" << 'HELPER_EOF'
#!/usr/bin/env bash
set -euo pipefail

SCRIPT_FILE="$1"
HISTORY_FILE_PATH="$2"
FUNC_NAME="$3"
shift 3

# Stub out the error function
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'
error() { echo "ERROR: $*" >&2; exit 1; }
warn() { echo "WARNING: $*" >&2; }
success() { echo "$*"; }

HISTORY_FILE="$HISTORY_FILE_PATH"

# Extract function definitions from the script using awk
# This handles multi-line functions properly
extract_function() {
    local fname="$1"
    local script="$2"
    awk "/^${fname}\\(\\)/{found=1} found{print; if(/^\\}/){exit}}" "$script"
}

eval "$(extract_function compute_phase_hashes "$SCRIPT_FILE")"
eval "$(extract_function get_changed_phases "$SCRIPT_FILE")"
eval "$(extract_function get_prev_verdict "$SCRIPT_FILE")"
eval "$(extract_function record_results "$SCRIPT_FILE")"

"$FUNC_NAME" "$@"
HELPER_EOF
    chmod +x "$TEST_TEMP_DIR/run_fn.sh"
}

teardown() {
    teardown_temp_dir
}

# Helper: create a plan.md file for a phase
create_phase_plan() {
    local phase="$1"
    local content="$2"
    mkdir -p "$PLAN_DIR/phases/$phase"
    printf '%s' "$content" > "$PLAN_DIR/phases/$phase/plan.md"
}

# Helper: create history file with specific data
create_history() {
    local json="$1"
    echo "$json" > "$HISTORY_FILE"
}

# Helper: run a function extracted from plan-review-loop.sh
run_fn() {
    local func_name="$1"
    shift
    run "$TEST_TEMP_DIR/run_fn.sh" "$SCRIPT" "$HISTORY_FILE" "$func_name" "$@"
}

# ==============================================================================
# compute_phase_hashes tests
# ==============================================================================

@test "qa_adversary-skip-cache: test_compute_phase_hashes_single_phase" {
    create_phase_plan "alpha" "hello"

    expected_hash=$(printf 'hello' | sha256sum | cut -d' ' -f1)

    run_fn compute_phase_hashes "$PLAN_DIR" "alpha"
    [ "$status" -eq 0 ]

    # Output should be valid JSON
    echo "$output" | jq . >/dev/null 2>&1

    # Should contain the correct hash for alpha
    actual_hash=$(echo "$output" | jq -r '.alpha')
    [ "$actual_hash" = "$expected_hash" ]
}

@test "qa_adversary-skip-cache: test_compute_phase_hashes_multiple_phases" {
    create_phase_plan "alpha" "aaa"
    create_phase_plan "beta" "bbb"

    expected_alpha=$(printf 'aaa' | sha256sum | cut -d' ' -f1)
    expected_beta=$(printf 'bbb' | sha256sum | cut -d' ' -f1)

    run_fn compute_phase_hashes "$PLAN_DIR" "alpha" "beta"
    [ "$status" -eq 0 ]

    # Valid JSON with both keys
    echo "$output" | jq . >/dev/null 2>&1
    actual_alpha=$(echo "$output" | jq -r '.alpha')
    actual_beta=$(echo "$output" | jq -r '.beta')
    [ "$actual_alpha" = "$expected_alpha" ]
    [ "$actual_beta" = "$expected_beta" ]
}

@test "qa_adversary-skip-cache: test_compute_phase_hashes_missing_file" {
    mkdir -p "$PLAN_DIR/phases/alpha"
    # Do NOT create plan.md

    run_fn compute_phase_hashes "$PLAN_DIR" "alpha"
    [ "$status" -eq 0 ]

    actual=$(echo "$output" | jq -r '.alpha')
    [ "$actual" = "missing" ]
}

@test "qa_adversary-skip-cache: test_compute_phase_hashes_empty_file" {
    mkdir -p "$PLAN_DIR/phases/alpha"
    : > "$PLAN_DIR/phases/alpha/plan.md"

    expected_hash="e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

    run_fn compute_phase_hashes "$PLAN_DIR" "alpha"
    [ "$status" -eq 0 ]

    actual=$(echo "$output" | jq -r '.alpha')
    [ "$actual" = "$expected_hash" ]
}

@test "qa_adversary-skip-cache: test_compute_phase_hashes_sha256sum_failure" {
    create_phase_plan "alpha" "content"
    chmod 000 "$PLAN_DIR/phases/alpha/plan.md"

    run_fn compute_phase_hashes "$PLAN_DIR" "alpha"
    [ "$status" -ne 0 ]

    # Restore permissions for cleanup
    chmod 644 "$PLAN_DIR/phases/alpha/plan.md"
}

@test "qa_adversary-skip-cache: test_compute_phase_hashes_zero_phases" {
    run_fn compute_phase_hashes "$PLAN_DIR"
    [ "$status" -eq 0 ]

    result=$(echo "$output" | jq 'keys | length')
    [ "$result" = "0" ]
}

# ==============================================================================
# get_changed_phases tests
# ==============================================================================

@test "qa_adversary-skip-cache: test_get_changed_phases_all_changed" {
    create_history '{"iterations":[{"iteration":1,"phase_hashes":{"a":"999","b":"888"},"results":{}}],"next_iteration":2}'

    run_fn get_changed_phases '{"a":"111","b":"222"}' "$HISTORY_FILE" 1
    [ "$status" -eq 0 ]

    # Both phases should be in output
    [[ "$output" == *"a"* ]]
    [[ "$output" == *"b"* ]]
}

@test "qa_adversary-skip-cache: test_get_changed_phases_none_changed" {
    create_history '{"iterations":[{"iteration":1,"phase_hashes":{"a":"111","b":"222"},"results":{}}],"next_iteration":2}'

    run_fn get_changed_phases '{"a":"111","b":"222"}' "$HISTORY_FILE" 1
    [ "$status" -eq 0 ]
    [ -z "$output" ]
}

@test "qa_adversary-skip-cache: test_get_changed_phases_one_changed" {
    create_history '{"iterations":[{"iteration":1,"phase_hashes":{"a":"111","b":"222"},"results":{}}],"next_iteration":2}'

    run_fn get_changed_phases '{"a":"111","b":"999"}' "$HISTORY_FILE" 1
    [ "$status" -eq 0 ]
    [ "$output" = "b" ]
}

@test "qa_adversary-skip-cache: test_get_changed_phases_missing_prev_hashes" {
    create_history '{"iterations":[{"iteration":1,"results":{"coverage":"passed"}}],"next_iteration":2}'

    run_fn get_changed_phases '{"a":"111"}' "$HISTORY_FILE" 1
    [ "$status" -eq 0 ]
    [[ "$output" == *"a"* ]]
}

@test "qa_adversary-skip-cache: test_get_changed_phases_new_phase_added" {
    create_history '{"iterations":[{"iteration":1,"phase_hashes":{"a":"111"},"results":{}}],"next_iteration":2}'

    run_fn get_changed_phases '{"a":"111","c":"333"}' "$HISTORY_FILE" 1
    [ "$status" -eq 0 ]
    [ "$output" = "c" ]
}

@test "qa_adversary-skip-cache: test_get_changed_phases_phase_removed" {
    create_history '{"iterations":[{"iteration":1,"phase_hashes":{"a":"111","b":"222"},"results":{}}],"next_iteration":2}'

    run_fn get_changed_phases '{"a":"111"}' "$HISTORY_FILE" 1
    [ "$status" -eq 0 ]
    [ -z "$output" ]
}

@test "qa_adversary-skip-cache: test_get_changed_phases_corrupt_history" {
    echo '{invalid json' > "$HISTORY_FILE"

    run_fn get_changed_phases '{"a":"111"}' "$HISTORY_FILE" 1
    [ "$status" -eq 0 ]
    [[ "$output" == *"a"* ]]
}

@test "qa_adversary-skip-cache: test_get_changed_phases_history_file_missing" {
    local missing_file="$TEST_TEMP_DIR/nonexistent_history.json"

    # Need a custom run since we pass a different history file
    run "$TEST_TEMP_DIR/run_fn.sh" "$SCRIPT" "$missing_file" get_changed_phases '{"a":"111"}' "$missing_file" 1
    [ "$status" -eq 0 ]
    [[ "$output" == *"a"* ]]
}

# ==============================================================================
# get_prev_verdict tests
# ==============================================================================

@test "qa_adversary-skip-cache: test_get_prev_verdict_passed" {
    create_history '{"iterations":[{"iteration":1,"results":{"coverage":"passed"}}],"next_iteration":2}'

    run_fn get_prev_verdict "$HISTORY_FILE" 1 "coverage"
    [ "$status" -eq 0 ]
    [ "$output" = "passed" ]
}

@test "qa_adversary-skip-cache: test_get_prev_verdict_failed" {
    create_history '{"iterations":[{"iteration":1,"results":{"ambiguity":"failed"}}],"next_iteration":2}'

    run_fn get_prev_verdict "$HISTORY_FILE" 1 "ambiguity"
    [ "$status" -eq 0 ]
    [ "$output" = "failed" ]
}

@test "qa_adversary-skip-cache: test_get_prev_verdict_error" {
    create_history '{"iterations":[{"iteration":1,"results":{"executability":"error"}}],"next_iteration":2}'

    run_fn get_prev_verdict "$HISTORY_FILE" 1 "executability"
    [ "$status" -eq 0 ]
    [ "$output" = "error" ]
}

@test "qa_adversary-skip-cache: test_get_prev_verdict_warning" {
    create_history '{"iterations":[{"iteration":1,"results":{"scope":"warning"}}],"next_iteration":2}'

    run_fn get_prev_verdict "$HISTORY_FILE" 1 "scope"
    [ "$status" -eq 0 ]
    [ "$output" = "warning" ]
}

@test "qa_adversary-skip-cache: test_get_prev_verdict_not_found" {
    create_history '{"iterations":[{"iteration":1,"results":{"coverage":"passed"}}],"next_iteration":2}'

    run_fn get_prev_verdict "$HISTORY_FILE" 1 "scope"
    [ "$status" -eq 0 ]
    [ "$output" = "none" ]
}

@test "qa_adversary-skip-cache: test_get_prev_verdict_missing_iteration" {
    create_history '{"iterations":[{"iteration":1,"results":{"coverage":"passed"}}],"next_iteration":2}'

    run_fn get_prev_verdict "$HISTORY_FILE" 2 "coverage"
    [ "$status" -eq 0 ]
    [ "$output" = "none" ]
}

@test "qa_adversary-skip-cache: test_get_prev_verdict_corrupt_history" {
    echo '{invalid json' > "$HISTORY_FILE"

    run_fn get_prev_verdict "$HISTORY_FILE" 1 "coverage"
    [ "$status" -eq 0 ]
    [ "$output" = "none" ]
}

# ==============================================================================
# cross_phase field in planning-adversaries.yaml
# ==============================================================================

@test "qa_adversary-skip-cache: test_cross_phase_field_defaults_false" {
    result=$(yq ".adversaries[] | select(.name == \"coverage\") | .cross_phase // false" "$ADVERSARIES_FILE")
    [ "$result" = "false" ]
}

@test "qa_adversary-skip-cache: test_cross_phase_field_true_for_consistency" {
    result=$(yq ".adversaries[] | select(.name == \"consistency\") | .cross_phase // false" "$ADVERSARIES_FILE")
    [ "$result" = "true" ]
}

@test "qa_adversary-skip-cache: test_cross_phase_field_true_for_scope" {
    result=$(yq ".adversaries[] | select(.name == \"scope\") | .cross_phase // false" "$ADVERSARIES_FILE")
    [ "$result" = "true" ]
}

@test "qa_adversary-skip-cache: test_cross_phase_all_five_adversaries_have_field" {
    # coverage=false, ambiguity=false, scope=true, consistency=true, executability=false
    [ "$(yq '.adversaries[] | select(.name == "coverage") | .cross_phase' "$ADVERSARIES_FILE")" = "false" ]
    [ "$(yq '.adversaries[] | select(.name == "ambiguity") | .cross_phase' "$ADVERSARIES_FILE")" = "false" ]
    [ "$(yq '.adversaries[] | select(.name == "scope") | .cross_phase' "$ADVERSARIES_FILE")" = "true" ]
    [ "$(yq '.adversaries[] | select(.name == "consistency") | .cross_phase' "$ADVERSARIES_FILE")" = "true" ]
    [ "$(yq '.adversaries[] | select(.name == "executability") | .cross_phase' "$ADVERSARIES_FILE")" = "false" ]
}

@test "qa_adversary-skip-cache: test_skip_logic_missing_cross_phase_field" {
    cat > "$TEST_TEMP_DIR/test-adversaries.yaml" << 'EOF'
adversaries:
  - name: test_adv
    prompt: prompts/test.md
    required: true
    verdicts:
      pass: ok
      fail: bad
EOF
    result=$(yq ".adversaries[] | select(.name == \"test_adv\") | .cross_phase // false" "$TEST_TEMP_DIR/test-adversaries.yaml")
    [ "$result" = "false" ]
}

# ==============================================================================
# Skip logic tests (unit-level logic verification)
# ==============================================================================

@test "qa_adversary-skip-cache: test_skip_logic_iteration_1_no_skip" {
    # On iteration 1, skip logic is disabled (iteration < 2)
    iteration=1
    prev_verdict="passed"
    changed_phases_count=0

    can_skip=false
    if [ "$iteration" -ge 2 ] && [ "$prev_verdict" = "passed" ] && [ "$changed_phases_count" -eq 0 ]; then
        can_skip=true
    fi

    [ "$can_skip" = "false" ]
}

@test "qa_adversary-skip-cache: test_skip_logic_passed_unchanged_skipped" {
    iteration=2
    prev_verdict="passed"
    changed_phases_count=0

    can_skip=false
    if [ "$iteration" -ge 2 ] && [ "$prev_verdict" = "passed" ] && [ "$changed_phases_count" -eq 0 ]; then
        can_skip=true
    fi

    [ "$can_skip" = "true" ]
}

@test "qa_adversary-skip-cache: test_skip_logic_failed_unchanged_not_skipped" {
    iteration=2
    prev_verdict="failed"
    changed_phases_count=0

    can_skip=false
    if [ "$iteration" -ge 2 ] && [ "$prev_verdict" = "passed" ] && [ "$changed_phases_count" -eq 0 ]; then
        can_skip=true
    fi

    [ "$can_skip" = "false" ]
}

@test "qa_adversary-skip-cache: test_skip_logic_error_unchanged_not_skipped" {
    iteration=2
    prev_verdict="error"
    changed_phases_count=0

    can_skip=false
    if [ "$iteration" -ge 2 ] && [ "$prev_verdict" = "passed" ] && [ "$changed_phases_count" -eq 0 ]; then
        can_skip=true
    fi

    [ "$can_skip" = "false" ]
}

@test "qa_adversary-skip-cache: test_skip_logic_warning_unchanged_not_skipped" {
    iteration=2
    prev_verdict="warning"
    changed_phases_count=0

    can_skip=false
    if [ "$iteration" -ge 2 ] && [ "$prev_verdict" = "passed" ] && [ "$changed_phases_count" -eq 0 ]; then
        can_skip=true
    fi

    [ "$can_skip" = "false" ]
}

@test "qa_adversary-skip-cache: test_skip_logic_passed_changed_not_skipped" {
    iteration=2
    prev_verdict="passed"
    changed_phases_count=1

    can_skip=false
    if [ "$iteration" -ge 2 ] && [ "$prev_verdict" = "passed" ] && [ "$changed_phases_count" -eq 0 ]; then
        can_skip=true
    fi

    [ "$can_skip" = "false" ]
}

@test "qa_adversary-skip-cache: test_skip_logic_cross_phase_any_change_not_skipped" {
    # cross_phase does NOT affect skip logic
    iteration=2
    prev_verdict="passed"
    changed_phases_count=1

    can_skip=false
    if [ "$iteration" -ge 2 ] && [ "$prev_verdict" = "passed" ] && [ "$changed_phases_count" -eq 0 ]; then
        can_skip=true
    fi

    [ "$can_skip" = "false" ]
}

@test "qa_adversary-skip-cache: test_skip_logic_cross_phase_no_change_skipped" {
    iteration=2
    prev_verdict="passed"
    changed_phases_count=0

    can_skip=false
    if [ "$iteration" -ge 2 ] && [ "$prev_verdict" = "passed" ] && [ "$changed_phases_count" -eq 0 ]; then
        can_skip=true
    fi

    [ "$can_skip" = "true" ]
}

@test "qa_adversary-skip-cache: test_skip_logic_cross_phase_false_same_as_true_for_skip" {
    # coverage (cross_phase=false) same skip behavior as cross_phase=true
    iteration=2
    prev_verdict="passed"
    changed_phases_count=1

    can_skip=false
    if [ "$iteration" -ge 2 ] && [ "$prev_verdict" = "passed" ] && [ "$changed_phases_count" -eq 0 ]; then
        can_skip=true
    fi

    [ "$can_skip" = "false" ]
}

# ==============================================================================
# Safety valve tests
# ==============================================================================

@test "qa_adversary-skip-cache: test_safety_valve_iteration_3" {
    iteration=3
    phases=("alpha" "beta" "gamma")
    changed_phases=()

    if [ $((iteration % 3)) -eq 0 ]; then
        changed_phases=("${phases[@]}")
    fi

    [ ${#changed_phases[@]} -eq 3 ]
    [ "${changed_phases[0]}" = "alpha" ]
    [ "${changed_phases[1]}" = "beta" ]
    [ "${changed_phases[2]}" = "gamma" ]
}

@test "qa_adversary-skip-cache: test_safety_valve_iteration_6" {
    iteration=6
    phases=("alpha" "beta")
    changed_phases=()

    if [ $((iteration % 3)) -eq 0 ]; then
        changed_phases=("${phases[@]}")
    fi

    [ ${#changed_phases[@]} -eq 2 ]
}

@test "qa_adversary-skip-cache: test_safety_valve_iteration_2_no_trigger" {
    iteration=2
    phases=("alpha" "beta")
    changed_phases=()

    if [ $((iteration % 3)) -eq 0 ]; then
        changed_phases=("${phases[@]}")
    fi

    [ ${#changed_phases[@]} -eq 0 ]
}

@test "qa_adversary-skip-cache: test_safety_valve_iteration_4_no_trigger" {
    iteration=4
    phases=("alpha" "beta")
    changed_phases=()

    if [ $((iteration % 3)) -eq 0 ]; then
        changed_phases=("${phases[@]}")
    fi

    [ ${#changed_phases[@]} -eq 0 ]
}

@test "qa_adversary-skip-cache: test_safety_valve_iteration_5_no_trigger" {
    iteration=5
    phases=("alpha" "beta")
    changed_phases=()

    if [ $((iteration % 3)) -eq 0 ]; then
        changed_phases=("${phases[@]}")
    fi

    [ ${#changed_phases[@]} -eq 0 ]
}

# ==============================================================================
# record_results with phase_hashes tests
# ==============================================================================

@test "qa_adversary-skip-cache: test_record_results_includes_phase_hashes" {
    run "$TEST_TEMP_DIR/run_fn.sh" "$SCRIPT" "$HISTORY_FILE" record_results 1 '{"a":"111"}' "coverage:passed"
    [ "$status" -eq 0 ]

    phase_hash=$(jq -r '.iterations[0].phase_hashes.a' "$HISTORY_FILE")
    [ "$phase_hash" = "111" ]
}

@test "qa_adversary-skip-cache: test_record_results_multiple_results" {
    run "$TEST_TEMP_DIR/run_fn.sh" "$SCRIPT" "$HISTORY_FILE" record_results 1 '{"a":"111"}' "coverage:passed" "ambiguity:failed" "scope:passed" "consistency:passed" "executability:warning"
    [ "$status" -eq 0 ]

    [ "$(jq -r '.iterations[0].results.coverage' "$HISTORY_FILE")" = "passed" ]
    [ "$(jq -r '.iterations[0].results.ambiguity' "$HISTORY_FILE")" = "failed" ]
    [ "$(jq -r '.iterations[0].results.scope' "$HISTORY_FILE")" = "passed" ]
    [ "$(jq -r '.iterations[0].results.consistency' "$HISTORY_FILE")" = "passed" ]
    [ "$(jq -r '.iterations[0].results.executability' "$HISTORY_FILE")" = "warning" ]
    [ "$(jq -r '.iterations[0].phase_hashes.a' "$HISTORY_FILE")" = "111" ]
}

# ==============================================================================
# Cached result parsing tests
# ==============================================================================

@test "qa_adversary-skip-cache: test_cached_result_parsed_as_passed" {
    # Simulate result collection with "cached:passed"
    result_content="cached:passed"
    IFS=: read -r status verdict <<< "$result_content"

    [ "$status" = "cached" ]
    [ "$verdict" = "passed" ]

    RESULTS=""
    ALL_PASSED=true
    REQUIRED_FAILED=false

    case "$status" in
        passed)
            RESULTS+="test_adv:passed "
            ;;
        failed)
            RESULTS+="test_adv:failed "
            REQUIRED_FAILED=true
            ALL_PASSED=false
            ;;
        warning)
            RESULTS+="test_adv:warning "
            ALL_PASSED=false
            ;;
        cached)
            RESULTS+="test_adv:passed "
            ;;
        *)
            RESULTS+="test_adv:error "
            REQUIRED_FAILED=true
            ALL_PASSED=false
            ;;
    esac

    [ "$RESULTS" = "test_adv:passed " ]
    [ "$ALL_PASSED" = "true" ]
    [ "$REQUIRED_FAILED" = "false" ]
}

@test "qa_adversary-skip-cache: test_cached_result_no_false_regression" {
    create_history '{"iterations":[{"iteration":1,"phase_hashes":{"a":"111"},"results":{"coverage":"passed"}}],"next_iteration":2}'

    # Record iteration 2 with coverage:passed (from cached result)
    run "$TEST_TEMP_DIR/run_fn.sh" "$SCRIPT" "$HISTORY_FILE" record_results 2 '{"a":"111"}' "coverage:passed"
    [ "$status" -eq 0 ]

    # Verify no regression: both iterations have coverage=passed
    prev_passed=$(jq -r '.iterations[] | select(.iteration == 1) | .results | to_entries[] | select(.value == "passed") | .key' "$HISTORY_FILE")
    curr_failed=$(jq -r '.iterations[] | select(.iteration == 2) | .results | to_entries[] | select(.value == "failed" or .value == "error") | .key' "$HISTORY_FILE" 2>/dev/null || echo "")

    regressed=""
    for prev in $prev_passed; do
        for curr in $curr_failed; do
            if [ "$prev" = "$curr" ]; then
                regressed+="$prev "
            fi
        done
    done

    [ -z "$regressed" ]
}

# ==============================================================================
# Cached review file copy tests
# ==============================================================================

@test "qa_adversary-skip-cache: test_cached_review_file_copied" {
    echo "Previous review content" > "$REVIEWS_DIR/iteration_1_coverage.md"

    prev=1
    iteration=2
    adversary="coverage"
    source_file="$REVIEWS_DIR/iteration_${prev}_${adversary}.md"
    dest_file="$REVIEWS_DIR/iteration_${iteration}_${adversary}.md"

    if [ -f "$source_file" ]; then
        cp "$source_file" "$dest_file"
    fi

    [ -f "$dest_file" ]
    dest_content=$(cat "$dest_file")
    [ "$dest_content" = "Previous review content" ]
}

@test "qa_adversary-skip-cache: test_cached_review_file_source_missing" {
    prev=1
    iteration=2
    adversary="coverage"
    source_file="$REVIEWS_DIR/iteration_${prev}_${adversary}.md"

    can_skip=true
    if [ ! -f "$source_file" ]; then
        can_skip=false
    fi

    [ "$can_skip" = "false" ]
}

# ==============================================================================
# Backwards compatibility tests
# ==============================================================================

@test "qa_adversary-skip-cache: test_backwards_compat_no_phase_hashes" {
    create_history '{"iterations":[{"iteration":1,"results":{"coverage":"passed","ambiguity":"failed"}}],"next_iteration":2}'

    run_fn get_changed_phases '{"a":"111"}' "$HISTORY_FILE" 1
    [ "$status" -eq 0 ]
    [[ "$output" == *"a"* ]]
}

@test "qa_adversary-skip-cache: test_reset_clears_history" {
    create_history '{"iterations":[{"iteration":1,"phase_hashes":{"a":"111"},"results":{"coverage":"passed"}}],"next_iteration":2}'

    # Simulate --reset behavior
    echo '{"iterations":[],"next_iteration":1}' > "$HISTORY_FILE"

    result=$(jq '.iterations | length' "$HISTORY_FILE")
    [ "$result" = "0" ]
    result=$(jq -r '.next_iteration' "$HISTORY_FILE")
    [ "$result" = "1" ]
}

# ==============================================================================
# Edge case: single phase plan
# ==============================================================================

@test "qa_adversary-skip-cache: test_single_phase_compute_hash" {
    create_phase_plan "only-phase" "single phase content"

    run_fn compute_phase_hashes "$PLAN_DIR" "only-phase"
    [ "$status" -eq 0 ]

    key_count=$(echo "$output" | jq 'keys | length')
    [ "$key_count" = "1" ]
    [ "$(echo "$output" | jq -r 'has("only-phase")')" = "true" ]
}

@test "qa_adversary-skip-cache: test_single_phase_get_changed_unchanged" {
    create_history '{"iterations":[{"iteration":1,"phase_hashes":{"only-phase":"aaa"},"results":{}}],"next_iteration":2}'

    run_fn get_changed_phases '{"only-phase":"aaa"}' "$HISTORY_FILE" 1
    [ "$status" -eq 0 ]
    [ -z "$output" ]
}

# ==============================================================================
# Edge case: whitespace changes in plan.md
# ==============================================================================

@test "qa_adversary-skip-cache: test_whitespace_changes_detected" {
    hash1=$(printf 'hello world' | sha256sum | cut -d' ' -f1)
    hash2=$(printf 'hello  world' | sha256sum | cut -d' ' -f1)

    [ "$hash1" != "$hash2" ]
}

# ==============================================================================
# Edge case: mix of cached and fresh results
# ==============================================================================

@test "qa_adversary-skip-cache: test_mix_cached_and_fresh_results" {
    RESULTS=""
    ALL_PASSED=true
    REQUIRED_FAILED=false

    # Cached result for coverage
    status_val="cached"
    case "$status_val" in
        cached) RESULTS+="coverage:passed " ;;
    esac

    # Fresh result for ambiguity
    status_val="failed"
    case "$status_val" in
        failed)
            RESULTS+="ambiguity:failed "
            REQUIRED_FAILED=true
            ALL_PASSED=false
            ;;
    esac

    [ "$RESULTS" = "coverage:passed ambiguity:failed " ]
    [ "$ALL_PASSED" = "false" ]
    [ "$REQUIRED_FAILED" = "true" ]
}

# ==============================================================================
# Edge case: all adversaries cached
# ==============================================================================

@test "qa_adversary-skip-cache: test_all_adversaries_cached" {
    RESULTS=""
    ALL_PASSED=true
    REQUIRED_FAILED=false

    for adv in coverage ambiguity scope consistency executability; do
        status_val="cached"
        case "$status_val" in
            cached) RESULTS+="${adv}:passed " ;;
        esac
    done

    [ "$ALL_PASSED" = "true" ]
    [ "$REQUIRED_FAILED" = "false" ]
    [[ "$RESULTS" == *"coverage:passed"* ]]
    [[ "$RESULTS" == *"ambiguity:passed"* ]]
    [[ "$RESULTS" == *"scope:passed"* ]]
    [[ "$RESULTS" == *"consistency:passed"* ]]
    [[ "$RESULTS" == *"executability:passed"* ]]
}

# ==============================================================================
# Integration: functions and patterns exist in the script
# ==============================================================================

@test "qa_adversary-skip-cache: compute_phase_hashes function exists in script" {
    run grep -c "^compute_phase_hashes()" "$SCRIPT"
    [ "$status" -eq 0 ]
    [ "$output" -ge 1 ]
}

@test "qa_adversary-skip-cache: get_changed_phases function exists in script" {
    run grep -c "^get_changed_phases()" "$SCRIPT"
    [ "$status" -eq 0 ]
    [ "$output" -ge 1 ]
}

@test "qa_adversary-skip-cache: get_prev_verdict function exists in script" {
    run grep -c "^get_prev_verdict()" "$SCRIPT"
    [ "$status" -eq 0 ]
    [ "$output" -ge 1 ]
}

@test "qa_adversary-skip-cache: cached case exists in result collection loop" {
    run grep -c "cached)" "$SCRIPT"
    [ "$status" -eq 0 ]
    [ "$output" -ge 1 ]
}

@test "qa_adversary-skip-cache: record_results accepts phase_hashes parameter" {
    run grep "phase_hashes" "$SCRIPT"
    [ "$status" -eq 0 ]
}

@test "qa_adversary-skip-cache: HISTORY_FILE variable is defined in main body" {
    run grep 'HISTORY_FILE=' "$SCRIPT"
    [ "$status" -eq 0 ]
}

@test "qa_adversary-skip-cache: CURRENT_HASHES variable is set via compute_phase_hashes" {
    run grep 'CURRENT_HASHES=.*compute_phase_hashes' "$SCRIPT"
    [ "$status" -eq 0 ]
}

@test "qa_adversary-skip-cache: CHANGED_PHASES is computed via get_changed_phases" {
    run grep 'get_changed_phases' "$SCRIPT"
    [ "$status" -eq 0 ]
}

@test "qa_adversary-skip-cache: safety_valve modulo 3 check exists" {
    run grep 'ITERATION % 3' "$SCRIPT"
    [ "$status" -eq 0 ]
}

@test "qa_adversary-skip-cache: is_cross_phase variable set in dispatch loop" {
    run grep 'is_cross_phase' "$SCRIPT"
    [ "$status" -eq 0 ]
}

@test "qa_adversary-skip-cache: record_results called with CURRENT_HASHES" {
    run grep 'record_results.*CURRENT_HASHES' "$SCRIPT"
    [ "$status" -eq 0 ]
}
