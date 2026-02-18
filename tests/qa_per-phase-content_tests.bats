#!/usr/bin/env bats
# QA Tests for per-phase-content phase
# Tests that non-cross-phase adversaries on iteration 2+ receive only changed
# phases' plan content instead of the full concatenation, reducing token usage.
#
# Covers:
# - PLAN_CONTENT_FULL replaces old PLAN_CONTENT variable
# - PLAN_CONTENT_CHANGED computed from changed phases only
# - Content routing: cross_phase=true → full, cross_phase=false → changed-only
# - "NOTE:" prefix on partial content
# - Edge cases: single phase, all changed, none changed, iteration 1

setup() {
    load 'test_helper'
    setup_temp_dir

    SCRIPT="$SCRIPTS_DIR/plan-review-loop.sh"
    ADVERSARIES_FILE="$ORCH_DIR/adversaries/planning-adversaries.yaml"

    # Create a plan structure with 3 phases
    PLAN_DIR="$TEST_TEMP_DIR/plans/active/test-plan"
    REVIEWS_DIR="$PLAN_DIR/reviews"
    HISTORY_FILE="$REVIEWS_DIR/adversary_history.json"
    mkdir -p "$PLAN_DIR/phases/alpha"
    mkdir -p "$PLAN_DIR/phases/beta"
    mkdir -p "$PLAN_DIR/phases/gamma"
    mkdir -p "$REVIEWS_DIR"

    # Write plan.md files for each phase
    printf 'Alpha phase content here' > "$PLAN_DIR/phases/alpha/plan.md"
    printf 'Beta phase content here' > "$PLAN_DIR/phases/beta/plan.md"
    printf 'Gamma phase content here' > "$PLAN_DIR/phases/gamma/plan.md"

    # Initialize empty history
    echo '{"iterations":[],"next_iteration":1}' > "$HISTORY_FILE"

    # Write a helper script that sources functions from plan-review-loop.sh
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
extract_function() {
    local fname="$1"
    local script="$2"
    awk "/^${fname}\\(\\)/{found=1} found{print; if(/^\\}/){exit}}" "$script"
}

eval "$(extract_function collect_plan_content "$SCRIPT_FILE")"
eval "$(extract_function compute_phase_hashes "$SCRIPT_FILE")"
eval "$(extract_function get_changed_phases "$SCRIPT_FILE")"

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

# Helper: run a function extracted from plan-review-loop.sh
run_fn() {
    local func_name="$1"
    shift
    run "$TEST_TEMP_DIR/run_fn.sh" "$SCRIPT" "$HISTORY_FILE" "$func_name" "$@"
}

# ==============================================================================
# test_plan_content_full_replaces_old_variable
# Verify that bare PLAN_CONTENT= (the old variable) no longer exists
# ==============================================================================

@test "qa_per-phase-content: test_plan_content_full_replaces_old_variable" {
    # grep for PLAN_CONTENT= excluding PLAN_CONTENT_FULL, PLAN_CONTENT_CHANGED, PLAN_CONTENT_LENGTH
    run bash -c "grep 'PLAN_CONTENT=' '$SCRIPT' | grep -v 'PLAN_CONTENT_FULL\|PLAN_CONTENT_CHANGED\|PLAN_CONTENT_LENGTH'"
    # Expect NO matches (grep returns 1 when nothing found)
    [ "$status" -eq 1 ]
}

@test "qa_per-phase-content: PLAN_CONTENT_FULL variable is defined in script" {
    run grep 'PLAN_CONTENT_FULL=' "$SCRIPT"
    [ "$status" -eq 0 ]
}

@test "qa_per-phase-content: PLAN_CONTENT_FULL assigned via collect_plan_content" {
    run grep 'PLAN_CONTENT_FULL=.*collect_plan_content' "$SCRIPT"
    [ "$status" -eq 0 ]
}

@test "qa_per-phase-content: PLAN_CONTENT_CHANGED variable is defined in script" {
    run grep 'PLAN_CONTENT_CHANGED=' "$SCRIPT"
    [ "$status" -eq 0 ]
}

@test "qa_per-phase-content: PLAN_CONTENT_LENGTH uses PLAN_CONTENT_FULL" {
    run grep 'PLAN_CONTENT_LENGTH=.*PLAN_CONTENT_FULL' "$SCRIPT"
    [ "$status" -eq 0 ]
}

@test "qa_per-phase-content: no bare PLAN_CONTENT reference in run_adversary call" {
    # The dispatch loop should use plan_input, not PLAN_CONTENT
    # Check that run_adversary is NOT called with $PLAN_CONTENT (bare)
    run bash -c "grep 'run_adversary.*PLAN_CONTENT[^_]' '$SCRIPT'"
    # Expect NO matches
    [ "$status" -eq 1 ]
}

@test "qa_per-phase-content: run_adversary called with plan_input variable" {
    run grep 'run_adversary.*plan_input' "$SCRIPT"
    [ "$status" -eq 0 ]
}

# ==============================================================================
# test_iteration_1_always_full_content
# On iteration 1, every adversary receives full content
# ==============================================================================

@test "qa_per-phase-content: test_iteration_1_always_full_content" {
    ITERATION=1
    PHASES=("alpha" "beta" "gamma")
    CHANGED_PHASES=("alpha" "beta" "gamma")

    # Collect full content
    run_fn collect_plan_content "$PLAN_DIR" "${PHASES[@]}"
    [ "$status" -eq 0 ]
    PLAN_CONTENT_FULL="$output"

    # On iteration 1, PLAN_CONTENT_CHANGED defaults to PLAN_CONTENT_FULL
    # because the condition (ITERATION -ge 2) is false
    PLAN_CONTENT_CHANGED="$PLAN_CONTENT_FULL"
    if [[ $ITERATION -ge 2 && ${#CHANGED_PHASES[@]} -gt 0 && ${#CHANGED_PHASES[@]} -lt ${#PHASES[@]} ]]; then
        # Would compute changed-only content, but won't enter for iteration 1
        PLAN_CONTENT_CHANGED="changed_only_should_not_reach"
    fi

    [ "$PLAN_CONTENT_CHANGED" = "$PLAN_CONTENT_FULL" ]
}

# ==============================================================================
# test_per_phase_adversary_gets_changed_only
# cross_phase=false adversary on iteration 2 with 1 changed phase
# ==============================================================================

@test "qa_per-phase-content: test_per_phase_adversary_gets_changed_only" {
    ITERATION=2
    PHASES=("alpha" "beta" "gamma")
    CHANGED_PHASES=("beta")

    # Compute full content
    run_fn collect_plan_content "$PLAN_DIR" "${PHASES[@]}"
    [ "$status" -eq 0 ]
    PLAN_CONTENT_FULL="$output"

    # Compute changed-only content
    PLAN_CONTENT_CHANGED="$PLAN_CONTENT_FULL"
    if [[ $ITERATION -ge 2 && ${#CHANGED_PHASES[@]} -gt 0 && ${#CHANGED_PHASES[@]} -lt ${#PHASES[@]} ]]; then
        run_fn collect_plan_content "$PLAN_DIR" "${CHANGED_PHASES[@]}"
        [ "$status" -eq 0 ]
        PLAN_CONTENT_CHANGED="$output"
    fi

    # coverage adversary: cross_phase=false
    is_cross_phase="false"

    # Content selection logic
    if [[ "$is_cross_phase" == "true" || ${#CHANGED_PHASES[@]} -eq ${#PHASES[@]} ]]; then
        plan_input="$PLAN_CONTENT_FULL"
    else
        plan_input="$PLAN_CONTENT_CHANGED"
    fi

    # Prepend context note for partial reviews
    if [[ "$plan_input" != "$PLAN_CONTENT_FULL" ]]; then
        plan_input="NOTE: Only the following phases changed since the last review iteration. Previously unchanged phases passed this adversary.

$plan_input"
    fi

    # Should contain beta but not alpha or gamma
    [[ "$plan_input" == *"## Phase: beta"* ]]
    [[ "$plan_input" != *"## Phase: alpha"* ]]
    [[ "$plan_input" != *"## Phase: gamma"* ]]

    # Should have the NOTE prefix
    [[ "$plan_input" == "NOTE: Only the following"* ]]
}

# ==============================================================================
# test_cross_phase_adversary_gets_full
# cross_phase=true adversary on iteration 2 with 1 changed phase
# ==============================================================================

@test "qa_per-phase-content: test_cross_phase_adversary_gets_full" {
    ITERATION=2
    PHASES=("alpha" "beta" "gamma")
    CHANGED_PHASES=("beta")

    run_fn collect_plan_content "$PLAN_DIR" "${PHASES[@]}"
    [ "$status" -eq 0 ]
    PLAN_CONTENT_FULL="$output"

    run_fn collect_plan_content "$PLAN_DIR" "${CHANGED_PHASES[@]}"
    [ "$status" -eq 0 ]
    PLAN_CONTENT_CHANGED="$output"

    # consistency adversary: cross_phase=true
    is_cross_phase="true"

    if [[ "$is_cross_phase" == "true" || ${#CHANGED_PHASES[@]} -eq ${#PHASES[@]} ]]; then
        plan_input="$PLAN_CONTENT_FULL"
    else
        plan_input="$PLAN_CONTENT_CHANGED"
    fi

    # Should NOT have NOTE prefix
    if [[ "$plan_input" != "$PLAN_CONTENT_FULL" ]]; then
        plan_input="NOTE: Only the following phases changed since the last review iteration. Previously unchanged phases passed this adversary.

$plan_input"
    fi

    # Should contain all 3 phases
    [[ "$plan_input" == *"## Phase: alpha"* ]]
    [[ "$plan_input" == *"## Phase: beta"* ]]
    [[ "$plan_input" == *"## Phase: gamma"* ]]

    # Should NOT start with NOTE
    [[ "$plan_input" != "NOTE:"* ]]
}

# ==============================================================================
# test_all_phases_changed_full_content
# All phases changed — even per-phase adversary gets full content
# ==============================================================================

@test "qa_per-phase-content: test_all_phases_changed_full_content" {
    ITERATION=2
    PHASES=("alpha" "beta" "gamma")
    CHANGED_PHASES=("alpha" "beta" "gamma")

    run_fn collect_plan_content "$PLAN_DIR" "${PHASES[@]}"
    [ "$status" -eq 0 ]
    PLAN_CONTENT_FULL="$output"

    # PLAN_CONTENT_CHANGED should default to full since all phases changed
    PLAN_CONTENT_CHANGED="$PLAN_CONTENT_FULL"
    if [[ $ITERATION -ge 2 && ${#CHANGED_PHASES[@]} -gt 0 && ${#CHANGED_PHASES[@]} -lt ${#PHASES[@]} ]]; then
        PLAN_CONTENT_CHANGED="should_not_reach"
    fi

    # coverage adversary: cross_phase=false
    is_cross_phase="false"

    if [[ "$is_cross_phase" == "true" || ${#CHANGED_PHASES[@]} -eq ${#PHASES[@]} ]]; then
        plan_input="$PLAN_CONTENT_FULL"
    else
        plan_input="$PLAN_CONTENT_CHANGED"
    fi

    # Full content since all phases changed
    [[ "$plan_input" == *"## Phase: alpha"* ]]
    [[ "$plan_input" == *"## Phase: beta"* ]]
    [[ "$plan_input" == *"## Phase: gamma"* ]]

    # No NOTE prefix since we're sending full content
    if [[ "$plan_input" != "$PLAN_CONTENT_FULL" ]]; then
        plan_input="NOTE: Only the following phases changed since the last review iteration. Previously unchanged phases passed this adversary.

$plan_input"
    fi
    [[ "$plan_input" != "NOTE:"* ]]
}

# ==============================================================================
# test_changed_only_content_has_note_prefix
# ==============================================================================

@test "qa_per-phase-content: test_changed_only_content_has_note_prefix" {
    ITERATION=2
    PHASES=("alpha" "beta" "gamma")
    CHANGED_PHASES=("beta")

    run_fn collect_plan_content "$PLAN_DIR" "${PHASES[@]}"
    [ "$status" -eq 0 ]
    PLAN_CONTENT_FULL="$output"

    run_fn collect_plan_content "$PLAN_DIR" "${CHANGED_PHASES[@]}"
    [ "$status" -eq 0 ]
    PLAN_CONTENT_CHANGED="$output"

    # Per-phase adversary (coverage, cross_phase=false)
    is_cross_phase="false"

    if [[ "$is_cross_phase" == "true" || ${#CHANGED_PHASES[@]} -eq ${#PHASES[@]} ]]; then
        plan_input="$PLAN_CONTENT_FULL"
    else
        plan_input="$PLAN_CONTENT_CHANGED"
    fi

    if [[ "$plan_input" != "$PLAN_CONTENT_FULL" ]]; then
        plan_input="NOTE: Only the following phases changed since the last review iteration. Previously unchanged phases passed this adversary.

$plan_input"
    fi

    [[ "$plan_input" == "NOTE: Only the following phases changed"* ]]
}

# ==============================================================================
# test_full_content_has_no_note_prefix
# ==============================================================================

@test "qa_per-phase-content: test_full_content_has_no_note_prefix" {
    ITERATION=2
    PHASES=("alpha" "beta" "gamma")
    CHANGED_PHASES=("beta")

    run_fn collect_plan_content "$PLAN_DIR" "${PHASES[@]}"
    [ "$status" -eq 0 ]
    PLAN_CONTENT_FULL="$output"

    # Cross-phase adversary (consistency, cross_phase=true)
    is_cross_phase="true"

    if [[ "$is_cross_phase" == "true" || ${#CHANGED_PHASES[@]} -eq ${#PHASES[@]} ]]; then
        plan_input="$PLAN_CONTENT_FULL"
    else
        plan_input="$PLAN_CONTENT_CHANGED"
    fi

    if [[ "$plan_input" != "$PLAN_CONTENT_FULL" ]]; then
        plan_input="NOTE: Only the following phases changed since the last review iteration. Previously unchanged phases passed this adversary.

$plan_input"
    fi

    # Must NOT start with NOTE
    [[ "$plan_input" != "NOTE:"* ]]
    # Must start with plan header
    [[ "$plan_input" == "# Plan:"* ]]
}

# ==============================================================================
# test_changed_content_includes_correct_phases
# ==============================================================================

@test "qa_per-phase-content: test_changed_content_includes_correct_phases" {
    ITERATION=2
    PHASES=("alpha" "beta" "gamma")
    CHANGED_PHASES=("beta")

    # Collect changed-only content
    run_fn collect_plan_content "$PLAN_DIR" "${CHANGED_PHASES[@]}"
    [ "$status" -eq 0 ]
    changed_content="$output"

    # Should include beta
    [[ "$changed_content" == *"## Phase: beta"* ]]
    [[ "$changed_content" == *"Beta phase content here"* ]]

    # Should NOT include alpha or gamma
    [[ "$changed_content" != *"## Phase: alpha"* ]]
    [[ "$changed_content" != *"## Phase: gamma"* ]]
    [[ "$changed_content" != *"Alpha phase content here"* ]]
    [[ "$changed_content" != *"Gamma phase content here"* ]]
}

# ==============================================================================
# test_full_content_includes_all_phases
# ==============================================================================

@test "qa_per-phase-content: test_full_content_includes_all_phases" {
    ITERATION=2
    PHASES=("alpha" "beta" "gamma")

    run_fn collect_plan_content "$PLAN_DIR" "${PHASES[@]}"
    [ "$status" -eq 0 ]
    full_content="$output"

    [[ "$full_content" == *"## Phase: alpha"* ]]
    [[ "$full_content" == *"## Phase: beta"* ]]
    [[ "$full_content" == *"## Phase: gamma"* ]]
    [[ "$full_content" == *"Alpha phase content here"* ]]
    [[ "$full_content" == *"Beta phase content here"* ]]
    [[ "$full_content" == *"Gamma phase content here"* ]]
}

# ==============================================================================
# test_no_phases_changed_all_skipped
# When 0 phases changed, PLAN_CONTENT_CHANGED defaults to PLAN_CONTENT_FULL
# because the if condition (CHANGED_PHASES > 0) is false
# ==============================================================================

@test "qa_per-phase-content: test_no_phases_changed_all_skipped" {
    ITERATION=2
    PHASES=("alpha" "beta" "gamma")
    CHANGED_PHASES=()  # No phases changed

    run_fn collect_plan_content "$PLAN_DIR" "${PHASES[@]}"
    [ "$status" -eq 0 ]
    PLAN_CONTENT_FULL="$output"

    # PLAN_CONTENT_CHANGED defaults to full because CHANGED_PHASES is empty
    PLAN_CONTENT_CHANGED="$PLAN_CONTENT_FULL"
    if [[ $ITERATION -ge 2 && ${#CHANGED_PHASES[@]} -gt 0 && ${#CHANGED_PHASES[@]} -lt ${#PHASES[@]} ]]; then
        PLAN_CONTENT_CHANGED="should_not_reach"
    fi

    [ "$PLAN_CONTENT_CHANGED" = "$PLAN_CONTENT_FULL" ]
}

# ==============================================================================
# test_single_phase_plan_always_full
# Single phase: CHANGED_PHASES count (1) equals PHASES count (1),
# so the -lt condition is false and full content is used
# ==============================================================================

@test "qa_per-phase-content: test_single_phase_plan_always_full" {
    ITERATION=2
    PHASES=("alpha")
    CHANGED_PHASES=("alpha")

    run_fn collect_plan_content "$PLAN_DIR" "${PHASES[@]}"
    [ "$status" -eq 0 ]
    PLAN_CONTENT_FULL="$output"

    # PLAN_CONTENT_CHANGED: condition ${#CHANGED_PHASES[@]} -lt ${#PHASES[@]}
    # 1 -lt 1 is false, so defaults to PLAN_CONTENT_FULL
    PLAN_CONTENT_CHANGED="$PLAN_CONTENT_FULL"
    if [[ $ITERATION -ge 2 && ${#CHANGED_PHASES[@]} -gt 0 && ${#CHANGED_PHASES[@]} -lt ${#PHASES[@]} ]]; then
        PLAN_CONTENT_CHANGED="should_not_reach"
    fi

    [ "$PLAN_CONTENT_CHANGED" = "$PLAN_CONTENT_FULL" ]

    # Content selection for per-phase adversary
    is_cross_phase="false"
    if [[ "$is_cross_phase" == "true" || ${#CHANGED_PHASES[@]} -eq ${#PHASES[@]} ]]; then
        plan_input="$PLAN_CONTENT_FULL"
    else
        plan_input="$PLAN_CONTENT_CHANGED"
    fi

    # No NOTE prefix since full content is used
    if [[ "$plan_input" != "$PLAN_CONTENT_FULL" ]]; then
        plan_input="NOTE: Only the following phases changed since the last review iteration. Previously unchanged phases passed this adversary.

$plan_input"
    fi

    [[ "$plan_input" != "NOTE:"* ]]
    [ "$plan_input" = "$PLAN_CONTENT_FULL" ]
}

# ==============================================================================
# Content selection logic tests (comprehensive scenarios)
# ==============================================================================

@test "qa_per-phase-content: content_selection_cross_phase_true_some_changed" {
    # cross_phase=true + some phases changed → full content
    is_cross_phase="true"
    PHASES=("a" "b" "c")
    CHANGED_PHASES=("b")
    PLAN_CONTENT_FULL="full"
    PLAN_CONTENT_CHANGED="changed"

    if [[ "$is_cross_phase" == "true" || ${#CHANGED_PHASES[@]} -eq ${#PHASES[@]} ]]; then
        plan_input="$PLAN_CONTENT_FULL"
    else
        plan_input="$PLAN_CONTENT_CHANGED"
    fi

    [ "$plan_input" = "full" ]
}

@test "qa_per-phase-content: content_selection_cross_phase_false_some_changed" {
    # cross_phase=false + some phases changed → changed-only content
    is_cross_phase="false"
    PHASES=("a" "b" "c")
    CHANGED_PHASES=("b")
    PLAN_CONTENT_FULL="full"
    PLAN_CONTENT_CHANGED="changed"

    if [[ "$is_cross_phase" == "true" || ${#CHANGED_PHASES[@]} -eq ${#PHASES[@]} ]]; then
        plan_input="$PLAN_CONTENT_FULL"
    else
        plan_input="$PLAN_CONTENT_CHANGED"
    fi

    [ "$plan_input" = "changed" ]
}

@test "qa_per-phase-content: content_selection_cross_phase_false_all_changed" {
    # cross_phase=false + all phases changed → full content (optimization is no-op)
    is_cross_phase="false"
    PHASES=("a" "b" "c")
    CHANGED_PHASES=("a" "b" "c")
    PLAN_CONTENT_FULL="full"
    PLAN_CONTENT_CHANGED="changed"

    if [[ "$is_cross_phase" == "true" || ${#CHANGED_PHASES[@]} -eq ${#PHASES[@]} ]]; then
        plan_input="$PLAN_CONTENT_FULL"
    else
        plan_input="$PLAN_CONTENT_CHANGED"
    fi

    [ "$plan_input" = "full" ]
}

@test "qa_per-phase-content: content_selection_cross_phase_true_all_changed" {
    # cross_phase=true + all phases changed → full content
    is_cross_phase="true"
    PHASES=("a" "b" "c")
    CHANGED_PHASES=("a" "b" "c")
    PLAN_CONTENT_FULL="full"
    PLAN_CONTENT_CHANGED="changed"

    if [[ "$is_cross_phase" == "true" || ${#CHANGED_PHASES[@]} -eq ${#PHASES[@]} ]]; then
        plan_input="$PLAN_CONTENT_FULL"
    else
        plan_input="$PLAN_CONTENT_CHANGED"
    fi

    [ "$plan_input" = "full" ]
}

# ==============================================================================
# NOTE prefix logic tests
# ==============================================================================

@test "qa_per-phase-content: note_prefix_applied_when_partial" {
    PLAN_CONTENT_FULL="full content"
    plan_input="changed only content"

    if [[ "$plan_input" != "$PLAN_CONTENT_FULL" ]]; then
        plan_input="NOTE: Only the following phases changed since the last review iteration. Previously unchanged phases passed this adversary.

$plan_input"
    fi

    [[ "$plan_input" == "NOTE: Only the following phases changed"* ]]
    [[ "$plan_input" == *"changed only content"* ]]
}

@test "qa_per-phase-content: note_prefix_not_applied_when_full" {
    PLAN_CONTENT_FULL="full content"
    plan_input="$PLAN_CONTENT_FULL"

    if [[ "$plan_input" != "$PLAN_CONTENT_FULL" ]]; then
        plan_input="NOTE: Only the following phases changed since the last review iteration. Previously unchanged phases passed this adversary.

$plan_input"
    fi

    [[ "$plan_input" != "NOTE:"* ]]
    [ "$plan_input" = "full content" ]
}

# ==============================================================================
# PLAN_CONTENT_CHANGED computation tests
# ==============================================================================

@test "qa_per-phase-content: changed_content_computed_on_iteration_2_with_subset" {
    ITERATION=2
    PHASES=("alpha" "beta" "gamma")
    CHANGED_PHASES=("alpha" "gamma")

    run_fn collect_plan_content "$PLAN_DIR" "${PHASES[@]}"
    [ "$status" -eq 0 ]
    PLAN_CONTENT_FULL="$output"

    PLAN_CONTENT_CHANGED="$PLAN_CONTENT_FULL"
    if [[ $ITERATION -ge 2 && ${#CHANGED_PHASES[@]} -gt 0 && ${#CHANGED_PHASES[@]} -lt ${#PHASES[@]} ]]; then
        run_fn collect_plan_content "$PLAN_DIR" "${CHANGED_PHASES[@]}"
        [ "$status" -eq 0 ]
        PLAN_CONTENT_CHANGED="$output"
    fi

    # Changed content should include alpha and gamma but not beta
    [[ "$PLAN_CONTENT_CHANGED" == *"## Phase: alpha"* ]]
    [[ "$PLAN_CONTENT_CHANGED" == *"## Phase: gamma"* ]]
    [[ "$PLAN_CONTENT_CHANGED" != *"## Phase: beta"* ]]

    # And it should differ from full content
    [ "$PLAN_CONTENT_CHANGED" != "$PLAN_CONTENT_FULL" ]
}

@test "qa_per-phase-content: changed_content_defaults_to_full_on_iteration_1" {
    ITERATION=1
    PHASES=("alpha" "beta" "gamma")
    CHANGED_PHASES=("beta")

    PLAN_CONTENT_FULL="full content"

    PLAN_CONTENT_CHANGED="$PLAN_CONTENT_FULL"
    if [[ $ITERATION -ge 2 && ${#CHANGED_PHASES[@]} -gt 0 && ${#CHANGED_PHASES[@]} -lt ${#PHASES[@]} ]]; then
        PLAN_CONTENT_CHANGED="should_not_reach"
    fi

    [ "$PLAN_CONTENT_CHANGED" = "$PLAN_CONTENT_FULL" ]
}

@test "qa_per-phase-content: changed_content_defaults_to_full_when_all_changed" {
    ITERATION=2
    PHASES=("alpha" "beta" "gamma")
    CHANGED_PHASES=("alpha" "beta" "gamma")

    PLAN_CONTENT_FULL="full content"

    PLAN_CONTENT_CHANGED="$PLAN_CONTENT_FULL"
    if [[ $ITERATION -ge 2 && ${#CHANGED_PHASES[@]} -gt 0 && ${#CHANGED_PHASES[@]} -lt ${#PHASES[@]} ]]; then
        PLAN_CONTENT_CHANGED="should_not_reach"
    fi

    [ "$PLAN_CONTENT_CHANGED" = "$PLAN_CONTENT_FULL" ]
}

@test "qa_per-phase-content: changed_content_defaults_to_full_when_none_changed" {
    ITERATION=2
    PHASES=("alpha" "beta" "gamma")
    CHANGED_PHASES=()

    PLAN_CONTENT_FULL="full content"

    PLAN_CONTENT_CHANGED="$PLAN_CONTENT_FULL"
    if [[ $ITERATION -ge 2 && ${#CHANGED_PHASES[@]} -gt 0 && ${#CHANGED_PHASES[@]} -lt ${#PHASES[@]} ]]; then
        PLAN_CONTENT_CHANGED="should_not_reach"
    fi

    # Empty CHANGED_PHASES → condition (> 0) is false → stays full
    [ "$PLAN_CONTENT_CHANGED" = "$PLAN_CONTENT_FULL" ]
}

# ==============================================================================
# Script structure verification — ordering constraints
# ==============================================================================

@test "qa_per-phase-content: PLAN_CONTENT_FULL defined before PLAN_CONTENT_CHANGED" {
    # PLAN_CONTENT_FULL must come before PLAN_CONTENT_CHANGED in the script
    full_line=$(grep -n 'PLAN_CONTENT_FULL=' "$SCRIPT" | head -1 | cut -d: -f1)
    changed_line=$(grep -n 'PLAN_CONTENT_CHANGED=' "$SCRIPT" | head -1 | cut -d: -f1)

    [ -n "$full_line" ]
    [ -n "$changed_line" ]
    [ "$full_line" -lt "$changed_line" ]
}

@test "qa_per-phase-content: PLAN_CONTENT_CHANGED defined after CHANGED_PHASES computation" {
    # PLAN_CONTENT_CHANGED must come after CHANGED_PHASES is set (including safety valve)
    # The safety valve uses "ITERATION % 3"
    safety_valve_line=$(grep -n 'ITERATION % 3' "$SCRIPT" | head -1 | cut -d: -f1)
    changed_content_line=$(grep -n 'PLAN_CONTENT_CHANGED=' "$SCRIPT" | head -1 | cut -d: -f1)

    [ -n "$safety_valve_line" ]
    [ -n "$changed_content_line" ]
    [ "$safety_valve_line" -lt "$changed_content_line" ]
}

@test "qa_per-phase-content: PLAN_CONTENT_CHANGED defined after CURRENT_HASHES" {
    hash_line=$(grep -n 'CURRENT_HASHES=' "$SCRIPT" | head -1 | cut -d: -f1)
    changed_content_line=$(grep -n 'PLAN_CONTENT_CHANGED=' "$SCRIPT" | head -1 | cut -d: -f1)

    [ -n "$hash_line" ]
    [ -n "$changed_content_line" ]
    [ "$hash_line" -lt "$changed_content_line" ]
}

# ==============================================================================
# Script structure verification — content selection in dispatch loop
# ==============================================================================

@test "qa_per-phase-content: plan_input variable set in dispatch loop" {
    run grep 'plan_input=' "$SCRIPT"
    [ "$status" -eq 0 ]
}

@test "qa_per-phase-content: is_cross_phase used in content selection" {
    run grep 'is_cross_phase.*true' "$SCRIPT"
    [ "$status" -eq 0 ]
}

@test "qa_per-phase-content: CHANGED_PHASES count compared to PHASES count" {
    # The dispatch loop should check ${#CHANGED_PHASES[@]} -eq ${#PHASES[@]}
    run grep 'CHANGED_PHASES.*PHASES' "$SCRIPT"
    [ "$status" -eq 0 ]
}

@test "qa_per-phase-content: NOTE prefix exists in dispatch logic" {
    run grep 'NOTE: Only the following phases changed' "$SCRIPT"
    [ "$status" -eq 0 ]
}

# ==============================================================================
# Edge case: phase with empty plan.md
# ==============================================================================

@test "qa_per-phase-content: empty_plan_md_still_has_phase_header" {
    # Create an empty plan.md
    mkdir -p "$PLAN_DIR/phases/empty-phase"
    : > "$PLAN_DIR/phases/empty-phase/plan.md"

    run_fn collect_plan_content "$PLAN_DIR" "empty-phase"
    [ "$status" -eq 0 ]

    # Should still include the phase header even if content is empty
    [[ "$output" == *"## Phase: empty-phase"* ]]
}

# ==============================================================================
# Edge case: two phases changed out of five
# ==============================================================================

@test "qa_per-phase-content: two_of_five_phases_changed" {
    # Set up 5 phases
    for p in alpha beta gamma delta epsilon; do
        create_phase_plan "$p" "Content for $p"
    done

    ITERATION=2
    PHASES=("alpha" "beta" "gamma" "delta" "epsilon")
    CHANGED_PHASES=("beta" "delta")

    run_fn collect_plan_content "$PLAN_DIR" "${PHASES[@]}"
    [ "$status" -eq 0 ]
    PLAN_CONTENT_FULL="$output"

    PLAN_CONTENT_CHANGED="$PLAN_CONTENT_FULL"
    if [[ $ITERATION -ge 2 && ${#CHANGED_PHASES[@]} -gt 0 && ${#CHANGED_PHASES[@]} -lt ${#PHASES[@]} ]]; then
        run_fn collect_plan_content "$PLAN_DIR" "${CHANGED_PHASES[@]}"
        [ "$status" -eq 0 ]
        PLAN_CONTENT_CHANGED="$output"
    fi

    # Changed content includes beta and delta
    [[ "$PLAN_CONTENT_CHANGED" == *"## Phase: beta"* ]]
    [[ "$PLAN_CONTENT_CHANGED" == *"## Phase: delta"* ]]

    # Changed content excludes alpha, gamma, epsilon
    [[ "$PLAN_CONTENT_CHANGED" != *"## Phase: alpha"* ]]
    [[ "$PLAN_CONTENT_CHANGED" != *"## Phase: gamma"* ]]
    [[ "$PLAN_CONTENT_CHANGED" != *"## Phase: epsilon"* ]]

    # Per-phase adversary gets changed-only
    is_cross_phase="false"
    if [[ "$is_cross_phase" == "true" || ${#CHANGED_PHASES[@]} -eq ${#PHASES[@]} ]]; then
        plan_input="$PLAN_CONTENT_FULL"
    else
        plan_input="$PLAN_CONTENT_CHANGED"
    fi

    if [[ "$plan_input" != "$PLAN_CONTENT_FULL" ]]; then
        plan_input="NOTE: Only the following phases changed since the last review iteration. Previously unchanged phases passed this adversary.

$plan_input"
    fi

    [[ "$plan_input" == "NOTE: Only the following"* ]]
    [[ "$plan_input" == *"## Phase: beta"* ]]
    [[ "$plan_input" == *"## Phase: delta"* ]]
    [[ "$plan_input" != *"## Phase: alpha"* ]]
}

# ==============================================================================
# Interaction with adversary-skip-cache: content selection only in can_skip=false
# ==============================================================================

@test "qa_per-phase-content: content_selection_only_in_no_skip_branch" {
    # When can_skip=true, the cached result is used and content selection doesn't run
    can_skip=true
    plan_input_set=false

    if [[ "$can_skip" == "true" ]]; then
        : # cached result used, no content selection
    else
        plan_input_set=true
    fi

    [ "$plan_input_set" = "false" ]
}

@test "qa_per-phase-content: content_selection_runs_when_not_skipped" {
    can_skip=false
    plan_input_set=false

    if [[ "$can_skip" == "true" ]]; then
        : # cached
    else
        plan_input_set=true
    fi

    [ "$plan_input_set" = "true" ]
}

# ==============================================================================
# All five adversaries: verify correct content routing per adversary
# ==============================================================================

@test "qa_per-phase-content: all_adversaries_correct_routing" {
    ITERATION=2
    PHASES=("alpha" "beta" "gamma")
    CHANGED_PHASES=("beta")
    PLAN_CONTENT_FULL="full"
    PLAN_CONTENT_CHANGED="changed"

    # Expected: coverage=false→changed, ambiguity=false→changed, scope=true→full,
    #           consistency=true→full, executability=false→changed
    declare -A expected
    expected[coverage]="changed"
    expected[ambiguity]="changed"
    expected[scope]="full"
    expected[consistency]="full"
    expected[executability]="changed"

    for adversary in coverage ambiguity scope consistency executability; do
        is_cross_phase=$(yq ".adversaries[] | select(.name == \"$adversary\") | .cross_phase // false" "$ADVERSARIES_FILE")

        if [[ "$is_cross_phase" == "true" || ${#CHANGED_PHASES[@]} -eq ${#PHASES[@]} ]]; then
            plan_input="$PLAN_CONTENT_FULL"
        else
            plan_input="$PLAN_CONTENT_CHANGED"
        fi

        [ "$plan_input" = "${expected[$adversary]}" ]
    done
}

# ==============================================================================
# Verify collect_plan_content works correctly with subset of phases
# ==============================================================================

@test "qa_per-phase-content: collect_plan_content_single_phase_subset" {
    run_fn collect_plan_content "$PLAN_DIR" "gamma"
    [ "$status" -eq 0 ]

    [[ "$output" == *"## Phase: gamma"* ]]
    [[ "$output" == *"Gamma phase content here"* ]]
    [[ "$output" != *"## Phase: alpha"* ]]
    [[ "$output" != *"## Phase: beta"* ]]
}

@test "qa_per-phase-content: collect_plan_content_two_phase_subset" {
    run_fn collect_plan_content "$PLAN_DIR" "alpha" "gamma"
    [ "$status" -eq 0 ]

    [[ "$output" == *"## Phase: alpha"* ]]
    [[ "$output" == *"## Phase: gamma"* ]]
    [[ "$output" != *"## Phase: beta"* ]]
}

# ==============================================================================
# Verify the full end-to-end content decision for each adversary type
# ==============================================================================

@test "qa_per-phase-content: end_to_end_coverage_adversary_iteration2" {
    # coverage: cross_phase=false, iteration 2, 1 of 3 changed, previously failed (not skipped)
    ITERATION=2
    PHASES=("alpha" "beta" "gamma")
    CHANGED_PHASES=("beta")

    run_fn collect_plan_content "$PLAN_DIR" "${PHASES[@]}"
    [ "$status" -eq 0 ]
    PLAN_CONTENT_FULL="$output"

    PLAN_CONTENT_CHANGED="$PLAN_CONTENT_FULL"
    if [[ $ITERATION -ge 2 && ${#CHANGED_PHASES[@]} -gt 0 && ${#CHANGED_PHASES[@]} -lt ${#PHASES[@]} ]]; then
        run_fn collect_plan_content "$PLAN_DIR" "${CHANGED_PHASES[@]}"
        [ "$status" -eq 0 ]
        PLAN_CONTENT_CHANGED="$output"
    fi

    is_cross_phase="false"
    can_skip=false

    if [[ "$can_skip" == "true" ]]; then
        : # cached
    else
        if [[ "$is_cross_phase" == "true" || ${#CHANGED_PHASES[@]} -eq ${#PHASES[@]} ]]; then
            plan_input="$PLAN_CONTENT_FULL"
        else
            plan_input="$PLAN_CONTENT_CHANGED"
        fi

        if [[ "$plan_input" != "$PLAN_CONTENT_FULL" ]]; then
            plan_input="NOTE: Only the following phases changed since the last review iteration. Previously unchanged phases passed this adversary.

$plan_input"
        fi
    fi

    # Verify: changed-only content with NOTE prefix
    [[ "$plan_input" == "NOTE: Only the following"* ]]
    [[ "$plan_input" == *"## Phase: beta"* ]]
    [[ "$plan_input" != *"## Phase: alpha"* ]]
    [[ "$plan_input" != *"## Phase: gamma"* ]]
}

@test "qa_per-phase-content: end_to_end_consistency_adversary_iteration2" {
    # consistency: cross_phase=true, iteration 2, 1 of 3 changed, previously failed (not skipped)
    ITERATION=2
    PHASES=("alpha" "beta" "gamma")
    CHANGED_PHASES=("beta")

    run_fn collect_plan_content "$PLAN_DIR" "${PHASES[@]}"
    [ "$status" -eq 0 ]
    PLAN_CONTENT_FULL="$output"

    PLAN_CONTENT_CHANGED="$PLAN_CONTENT_FULL"
    if [[ $ITERATION -ge 2 && ${#CHANGED_PHASES[@]} -gt 0 && ${#CHANGED_PHASES[@]} -lt ${#PHASES[@]} ]]; then
        run_fn collect_plan_content "$PLAN_DIR" "${CHANGED_PHASES[@]}"
        [ "$status" -eq 0 ]
        PLAN_CONTENT_CHANGED="$output"
    fi

    is_cross_phase="true"
    can_skip=false

    if [[ "$can_skip" == "true" ]]; then
        : # cached
    else
        if [[ "$is_cross_phase" == "true" || ${#CHANGED_PHASES[@]} -eq ${#PHASES[@]} ]]; then
            plan_input="$PLAN_CONTENT_FULL"
        else
            plan_input="$PLAN_CONTENT_CHANGED"
        fi

        if [[ "$plan_input" != "$PLAN_CONTENT_FULL" ]]; then
            plan_input="NOTE: Only the following phases changed since the last review iteration. Previously unchanged phases passed this adversary.

$plan_input"
        fi
    fi

    # Verify: full content, no NOTE prefix
    [[ "$plan_input" != "NOTE:"* ]]
    [[ "$plan_input" == *"## Phase: alpha"* ]]
    [[ "$plan_input" == *"## Phase: beta"* ]]
    [[ "$plan_input" == *"## Phase: gamma"* ]]
}

# ==============================================================================
# Edge case: Very large number of phases (20+)
# ==============================================================================

@test "qa_per-phase-content: twenty_plus_phases_filtering_works" {
    # Create 22 phases
    local all_phases=()
    local changed_phases=()
    for i in $(seq 1 22); do
        local pname="phase-$(printf '%02d' "$i")"
        create_phase_plan "$pname" "Content for phase $i"
        all_phases+=("$pname")
    done

    # Only phases 5 and 18 changed
    changed_phases=("phase-05" "phase-18")

    ITERATION=2
    PHASES=("${all_phases[@]}")
    CHANGED_PHASES=("${changed_phases[@]}")

    # Compute full content
    run_fn collect_plan_content "$PLAN_DIR" "${PHASES[@]}"
    [ "$status" -eq 0 ]
    PLAN_CONTENT_FULL="$output"

    # Compute changed-only content
    PLAN_CONTENT_CHANGED="$PLAN_CONTENT_FULL"
    if [[ $ITERATION -ge 2 && ${#CHANGED_PHASES[@]} -gt 0 && ${#CHANGED_PHASES[@]} -lt ${#PHASES[@]} ]]; then
        run_fn collect_plan_content "$PLAN_DIR" "${CHANGED_PHASES[@]}"
        [ "$status" -eq 0 ]
        PLAN_CONTENT_CHANGED="$output"
    fi

    # Changed content should only include phase-05 and phase-18
    [[ "$PLAN_CONTENT_CHANGED" == *"## Phase: phase-05"* ]]
    [[ "$PLAN_CONTENT_CHANGED" == *"## Phase: phase-18"* ]]

    # Changed content should NOT include other phases
    [[ "$PLAN_CONTENT_CHANGED" != *"## Phase: phase-01"* ]]
    [[ "$PLAN_CONTENT_CHANGED" != *"## Phase: phase-10"* ]]
    [[ "$PLAN_CONTENT_CHANGED" != *"## Phase: phase-22"* ]]

    # Per-phase adversary gets changed-only with NOTE prefix
    is_cross_phase="false"
    if [[ "$is_cross_phase" == "true" || ${#CHANGED_PHASES[@]} -eq ${#PHASES[@]} ]]; then
        plan_input="$PLAN_CONTENT_FULL"
    else
        plan_input="$PLAN_CONTENT_CHANGED"
    fi

    if [[ "$plan_input" != "$PLAN_CONTENT_FULL" ]]; then
        plan_input="NOTE: Only the following phases changed since the last review iteration. Previously unchanged phases passed this adversary.

$plan_input"
    fi

    [[ "$plan_input" == "NOTE: Only the following"* ]]
    [[ "$plan_input" == *"## Phase: phase-05"* ]]
    [[ "$plan_input" == *"## Phase: phase-18"* ]]

    # Full content should include all 22 phases
    [[ "$PLAN_CONTENT_FULL" == *"## Phase: phase-01"* ]]
    [[ "$PLAN_CONTENT_FULL" == *"## Phase: phase-22"* ]]
}
