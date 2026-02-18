#!/usr/bin/env bats

# Tests for plan-file-reference phase (token-optimization plan)
#
# Validates:
# - build-context.sh outputs plan_file path field
# - build-context.sh conditionally omits plan_md when already sent to state type
# - iterate.sh tracks plan_md_sent_to in state.json
# - Templates have {{else}} fallback with file reference when plan_md is empty
# - fix.md is NOT modified

setup() {
    load 'test_helper'
    setup_temp_dir

    BUILD_CONTEXT="$SCRIPTS_DIR/build-context.sh"
    ITERATE="$SCRIPTS_DIR/iterate.sh"
    PROMPTS_DIR="$ORCH_DIR/prompts/feature"

    # Standard phase directory structure
    export PHASE_DIR="$TEST_TEMP_DIR/.plans/active/test-plan/phases/test-phase"
    mkdir -p "$PHASE_DIR"

    # Create minimal state.json
    echo '{"iteration": 1, "current_state": "impl"}' > "$PHASE_DIR/state.json"

    # Create minimal workflow.yaml (V3)
    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test
version: 3
defaults:
  max_iterations: 10
states:
  - name: impl
    prompt: prompts/impl.md
  - name: qa
    prompt: prompts/qa.md
  - name: qa_review
    prompt: prompts/qa-review.md
  - name: impl_review
    prompt: prompts/impl-review.md
entry_state: qa
terminal_states: [complete]
YAML

    # Create plan.md with content
    cat > "$PHASE_DIR/plan.md" << 'MD'
# Test Phase

## Objective
Test the plan file reference feature.

## Details
This plan has multiple lines of content to verify embedding works.
MD
}

teardown() {
    teardown_temp_dir
}

# Helper: run build-context.sh with default test fixtures
build_context() {
    local state_file="${1:-$PHASE_DIR/state.json}"
    local workflow_file="${2:-$TEST_TEMP_DIR/workflow.yaml}"
    local phase_dir="${3:-$PHASE_DIR}"
    local state_name="${4:-impl}"
    run "$BUILD_CONTEXT" "$state_file" "$workflow_file" "$phase_dir" "$state_name"
}

# Helper: extract a JSON field from output (raw string)
get_field() {
    echo "$output" | jq -r "$1"
}

# Helper: assert JSON field equals expected value (string, uses -r)
assert_json_field() {
    local actual
    actual=$(echo "$output" | jq -r "$1")
    [[ "$actual" == "$2" ]]
}

# Helper: assert JSON field equals expected value (raw jq, no -r)
assert_json_raw() {
    local actual
    actual=$(echo "$output" | jq "$1")
    [[ "$actual" == "$2" ]]
}

#=============================================================================
# Script Syntax Tests
#=============================================================================

@test "qa_plan-file-reference_build_context_script_syntax_valid" {
    run bash -n "$BUILD_CONTEXT"
    [[ "$status" -eq 0 ]]
}

@test "qa_plan-file-reference_iterate_script_syntax_valid" {
    run bash -n "$ITERATE"
    [[ "$status" -eq 0 ]]
}

#=============================================================================
# plan_file Field in Output
#=============================================================================

@test "qa_plan-file-reference_plan_file_in_output" {
    # plan_file field should contain the path to plan.md
    build_context
    [[ "$status" -eq 0 ]]
    local plan_file
    plan_file=$(get_field '.plan_file')
    [[ "$plan_file" == "$PHASE_DIR/plan.md" ]]
}

@test "qa_plan-file-reference_plan_file_always_set_even_when_file_missing" {
    # plan_file should be set to the path even when plan.md doesn't exist
    rm -f "$PHASE_DIR/plan.md"
    build_context
    [[ "$status" -eq 0 ]]
    local plan_file
    plan_file=$(get_field '.plan_file')
    [[ "$plan_file" == "$PHASE_DIR/plan.md" ]]
}

#=============================================================================
# plan_md Conditional Embedding - First Call Behavior
#=============================================================================

@test "qa_plan-file-reference_plan_md_populated_first_call" {
    # No plan_md_sent_to field → plan_md should be embedded
    echo '{"iteration": 1}' > "$PHASE_DIR/state.json"
    build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" "impl"
    [[ "$status" -eq 0 ]]
    local plan_md
    plan_md=$(get_field '.plan_md')
    [[ -n "$plan_md" ]]
    [[ "$plan_md" == *"# Test Phase"* ]]
}

@test "qa_plan-file-reference_plan_md_sent_to_missing_field" {
    # State with no plan_md_sent_to at all → first call, embed plan
    echo '{"iteration": 1}' > "$PHASE_DIR/state.json"
    build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" "impl"
    [[ "$status" -eq 0 ]]
    local plan_md
    plan_md=$(get_field '.plan_md')
    [[ -n "$plan_md" ]]
    [[ "$plan_md" != "" ]]
}

@test "qa_plan-file-reference_plan_md_sent_to_empty_array" {
    # Empty plan_md_sent_to array → impl not in array → embed plan
    echo '{"iteration": 1, "plan_md_sent_to": []}' > "$PHASE_DIR/state.json"
    build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" "impl"
    [[ "$status" -eq 0 ]]
    local plan_md
    plan_md=$(get_field '.plan_md')
    [[ -n "$plan_md" ]]
    [[ "$plan_md" == *"# Test Phase"* ]]
}

#=============================================================================
# plan_md Conditional Embedding - Repeat Call Behavior
#=============================================================================

@test "qa_plan-file-reference_plan_md_empty_repeat_call" {
    # impl already in plan_md_sent_to → plan_md should be empty
    echo '{"iteration": 2, "plan_md_sent_to": ["impl"]}' > "$PHASE_DIR/state.json"
    build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" "impl"
    [[ "$status" -eq 0 ]]
    assert_json_field '.plan_md' ''
}

@test "qa_plan-file-reference_plan_md_populated_different_state" {
    # Only qa in plan_md_sent_to → impl hasn't received it → embed
    echo '{"iteration": 2, "plan_md_sent_to": ["qa"]}' > "$PHASE_DIR/state.json"
    build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" "impl"
    [[ "$status" -eq 0 ]]
    local plan_md
    plan_md=$(get_field '.plan_md')
    [[ -n "$plan_md" ]]
    [[ "$plan_md" == *"# Test Phase"* ]]
}

#=============================================================================
# Underscore Consistency
#=============================================================================

@test "qa_plan-file-reference_state_name_underscore_consistency" {
    # qa_review in sent_to, calling with qa_review → match → empty plan_md
    echo '{"iteration": 2, "plan_md_sent_to": ["qa_review"]}' > "$PHASE_DIR/state.json"
    build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" "qa_review"
    [[ "$status" -eq 0 ]]
    assert_json_field '.plan_md' ''
}

#=============================================================================
# plan_md_sent_to Tracking in iterate.sh
#=============================================================================

@test "qa_plan-file-reference_plan_md_sent_to_no_duplicates" {
    # If plan_md was empty (already sent), tracking code should NOT fire,
    # so no duplicate entry is added
    echo '{"iteration": 2, "plan_md_sent_to": ["impl"]}' > "$PHASE_DIR/state.json"

    # Simulate what iterate.sh does: call build-context, check if plan_md non-empty, conditionally update
    local context
    context=$("$BUILD_CONTEXT" "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" "impl")

    # plan_md should be empty since impl already received it
    local plan_md_val
    plan_md_val=$(echo "$context" | jq -e '.plan_md != ""' 2>/dev/null) || plan_md_val="false"

    # The tracking code only fires if plan_md is non-empty
    if [[ "$plan_md_val" == "true" ]]; then
        jq --arg s "impl" \
            '.plan_md_sent_to = ((.plan_md_sent_to // []) | if map(. == $s) | any then . else . + [$s] end)' \
            "$PHASE_DIR/state.json" > "${PHASE_DIR}/state.json.tmp.$$" && mv "${PHASE_DIR}/state.json.tmp.$$" "$PHASE_DIR/state.json"
    fi

    # Verify no duplicate
    local sent_to
    sent_to=$(jq -c '.plan_md_sent_to' "$PHASE_DIR/state.json")
    [[ "$sent_to" == '["impl"]' ]]
}

@test "qa_plan-file-reference_plan_md_sent_to_accumulates" {
    # First call with qa (plan_md_sent_to becomes ["qa"]), second with impl → ["qa","impl"]
    echo '{"iteration": 1}' > "$PHASE_DIR/state.json"

    # First call: qa
    local context1
    context1=$("$BUILD_CONTEXT" "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" "qa")

    # Tracking: plan_md should be non-empty (first call)
    if echo "$context1" | jq -e '.plan_md != ""' > /dev/null 2>&1; then
        jq --arg s "qa" \
            '.plan_md_sent_to = ((.plan_md_sent_to // []) | if map(. == $s) | any then . else . + [$s] end)' \
            "$PHASE_DIR/state.json" > "${PHASE_DIR}/state.json.tmp.$$" && mv "${PHASE_DIR}/state.json.tmp.$$" "$PHASE_DIR/state.json"
    fi

    # Second call: impl
    local context2
    context2=$("$BUILD_CONTEXT" "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" "impl")

    # Tracking: plan_md should be non-empty (impl not yet sent)
    if echo "$context2" | jq -e '.plan_md != ""' > /dev/null 2>&1; then
        jq --arg s "impl" \
            '.plan_md_sent_to = ((.plan_md_sent_to // []) | if map(. == $s) | any then . else . + [$s] end)' \
            "$PHASE_DIR/state.json" > "${PHASE_DIR}/state.json.tmp.$$" && mv "${PHASE_DIR}/state.json.tmp.$$" "$PHASE_DIR/state.json"
    fi

    # Verify accumulation
    local sent_to
    sent_to=$(jq -c '.plan_md_sent_to' "$PHASE_DIR/state.json")
    [[ "$sent_to" == '["qa","impl"]' ]]
}

#=============================================================================
# iterate.sh STATE_TYPE Values
#=============================================================================

@test "qa_plan-file-reference_iterate_qa_review_writes_underscore" {
    # The qa-review case block should use STATE_TYPE="qa_review" (underscore, not hyphen)
    # Read the iterate.sh and check the qa-review block
    local qa_review_block
    qa_review_block=$(sed -n '/^[[:space:]]*qa-review)/,/^[[:space:]]*;;/p' "$ITERATE")

    # Should contain STATE_TYPE="qa_review" (underscore)
    echo "$qa_review_block" | grep -q 'STATE_TYPE="qa_review"' || \
    echo "$qa_review_block" | grep -q "STATE_TYPE='qa_review'" || \
    echo "$qa_review_block" | grep -q 'qa_review'

    # The 4th arg to build-context.sh should also be "qa_review"
    echo "$qa_review_block" | grep -q '"qa_review"'
}

@test "qa_plan-file-reference_iterate_impl_review_writes_underscore" {
    # The impl-review case block should use STATE_TYPE="impl_review" (underscore, not hyphen)
    local impl_review_block
    impl_review_block=$(sed -n '/^[[:space:]]*impl-review)/,/^[[:space:]]*;;/p' "$ITERATE")

    # Should contain STATE_TYPE="impl_review" (underscore)
    echo "$impl_review_block" | grep -q 'STATE_TYPE="impl_review"' || \
    echo "$impl_review_block" | grep -q "STATE_TYPE='impl_review'" || \
    echo "$impl_review_block" | grep -q 'impl_review'

    # The 4th arg to build-context.sh should also be "impl_review"
    echo "$impl_review_block" | grep -q '"impl_review"'
}

#=============================================================================
# Template Tests - {{else}} fallback with file reference
#=============================================================================

@test "qa_plan-file-reference_template_impl_has_else_branch" {
    local template="$PROMPTS_DIR/impl.md"
    [[ -f "$template" ]]

    # Must contain {{else}} after {{#if plan_md}}
    grep -q '{{#if plan_md}}' "$template"
    grep -q '{{else}}' "$template"

    # Must contain plan_file reference
    grep -q '{{plan_file}}' "$template"

    # Must contain instruction to read the file
    grep -q 'Read the full phase specification from' "$template"
}

@test "qa_plan-file-reference_template_qa_has_else_branch" {
    local template="$PROMPTS_DIR/qa.md"
    [[ -f "$template" ]]

    grep -q '{{#if plan_md}}' "$template"
    grep -q '{{else}}' "$template"
    grep -q '{{plan_file}}' "$template"
    grep -q 'Read the full phase specification from' "$template"
}

@test "qa_plan-file-reference_template_qa_review_has_else_branch" {
    local template="$PROMPTS_DIR/qa-review.md"
    [[ -f "$template" ]]

    grep -q '{{#if plan_md}}' "$template"
    grep -q '{{else}}' "$template"
    grep -q '{{plan_file}}' "$template"
    grep -q 'Read the full phase specification from' "$template"
}

@test "qa_plan-file-reference_template_impl_review_has_else_branch" {
    local template="$PROMPTS_DIR/impl-review.md"
    [[ -f "$template" ]]

    grep -q '{{#if plan_md}}' "$template"
    grep -q '{{else}}' "$template"
    grep -q '{{plan_file}}' "$template"
    grep -q 'Read the full phase specification from' "$template"
}

@test "qa_plan-file-reference_template_fix_not_modified" {
    local template="$PROMPTS_DIR/fix.md"
    [[ -f "$template" ]]

    # fix.md must NOT contain {{else}} or plan_file in the plan_md context
    # It uses a completely different pattern ({{plan_file}} is already there
    # but NOT inside {{#if plan_md}} / {{else}} blocks)
    ! grep -q '{{#if plan_md}}' "$template"
}

#=============================================================================
# Edge Cases
#=============================================================================

@test "qa_plan-file-reference_both_plan_md_and_plan_file_when_no_plan_file" {
    # When plan.md doesn't exist: plan_md="" and plan_file still has path
    rm -f "$PHASE_DIR/plan.md"
    echo '{"iteration": 1}' > "$PHASE_DIR/state.json"

    build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" "impl"
    [[ "$status" -eq 0 ]]

    # plan_md empty (file doesn't exist)
    assert_json_field '.plan_md' ''

    # plan_file still has the path
    local plan_file
    plan_file=$(get_field '.plan_file')
    [[ "$plan_file" == "$PHASE_DIR/plan.md" ]]
}

@test "qa_plan-file-reference_plan_md_sent_to_non_array" {
    # If plan_md_sent_to is a string (corrupted), should fall back to "false"
    echo '{"iteration": 1, "plan_md_sent_to": "not_an_array"}' > "$PHASE_DIR/state.json"
    build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" "impl"
    [[ "$status" -eq 0 ]]

    # Should embed plan_md (treated as first call due to error fallback)
    local plan_md
    plan_md=$(get_field '.plan_md')
    [[ -n "$plan_md" ]]
    [[ "$plan_md" == *"# Test Phase"* ]]
}

@test "qa_plan-file-reference_state_file_missing" {
    # When there's no state.json file in the phase dir, the if guard should skip
    # the jq check and always_sent should be "false"
    # Note: state_file (1st arg) must still exist for build-context.sh validation,
    # but the plan_md_sent_to check uses "$phase_dir/state.json"
    local alt_state="$TEST_TEMP_DIR/alt_state.json"
    echo '{"iteration": 1}' > "$alt_state"

    # Phase dir without state.json (use a separate phase dir)
    local alt_phase="$TEST_TEMP_DIR/.plans/active/test-plan/phases/alt-phase"
    mkdir -p "$alt_phase"
    cat > "$alt_phase/plan.md" << 'MD'
# Alt Phase Plan
MD
    # No state.json in alt_phase dir

    build_context "$alt_state" "$TEST_TEMP_DIR/workflow.yaml" "$alt_phase" "impl"
    [[ "$status" -eq 0 ]]

    # plan_md should be embedded (no state.json to check sent_to)
    local plan_md
    plan_md=$(get_field '.plan_md')
    [[ -n "$plan_md" ]]
    [[ "$plan_md" == *"# Alt Phase Plan"* ]]
}

#=============================================================================
# Existing Tests Still Pass
#=============================================================================

@test "qa_plan-file-reference_existing_build_context_tests_pass" {
    # Verify the existing build_context.bats tests still pass
    # This is a meta-test that runs the existing test suite
    run bats "$TEST_DIR/build_context.bats"
    [[ "$status" -eq 0 ]]
}

#=============================================================================
# Integration: plan_md + plan_file work together correctly
#=============================================================================

@test "qa_plan-file-reference_plan_file_and_plan_md_both_present_first_call" {
    # On first call, both plan_file (path) and plan_md (content) should be present
    echo '{"iteration": 1}' > "$PHASE_DIR/state.json"
    build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" "impl"
    [[ "$status" -eq 0 ]]

    # plan_md has content
    local plan_md
    plan_md=$(get_field '.plan_md')
    [[ "$plan_md" == *"# Test Phase"* ]]

    # plan_file has path
    local plan_file
    plan_file=$(get_field '.plan_file')
    [[ "$plan_file" == "$PHASE_DIR/plan.md" ]]
}

@test "qa_plan-file-reference_plan_file_present_plan_md_empty_repeat_call" {
    # On repeat call, plan_file has path but plan_md is empty
    echo '{"iteration": 2, "plan_md_sent_to": ["impl"]}' > "$PHASE_DIR/state.json"
    build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" "impl"
    [[ "$status" -eq 0 ]]

    # plan_md is empty
    assert_json_field '.plan_md' ''

    # plan_file still has path
    local plan_file
    plan_file=$(get_field '.plan_file')
    [[ "$plan_file" == "$PHASE_DIR/plan.md" ]]
}
