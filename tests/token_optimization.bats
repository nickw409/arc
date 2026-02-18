#!/usr/bin/env bats
# Integration tests for token optimization
# Verifies all 5 optimizations work together:
#   1. state-trimming (build-context.sh state projection)
#   2. plan-file-reference (plan_file field, plan_md conditional)
#   3. escalation-model (model assignments, ESCALATION_CONTEXT hoisting)
#   4. adversary-skip-cache (hash functions, cached results in plan-review-loop.sh)
#   5. per-phase-content (content routing in plan-review-loop.sh)

setup() {
    load 'test_helper'
    setup_temp_dir

    BUILD_CONTEXT="$SCRIPTS_DIR/build-context.sh"
    ITERATE="$SCRIPTS_DIR/iterate.sh"
    PLAN_REVIEW_LOOP="$SCRIPTS_DIR/plan-review-loop.sh"
    ADVERSARIES_FILE="$ORCH_DIR/adversaries/planning-adversaries.yaml"

    # Standard phase directory structure for build-context tests
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
entry_state: qa
terminal_states: [complete]
YAML

    # Create plan.md with content
    cat > "$PHASE_DIR/plan.md" << 'MD'
# Test Phase

## Objective
Integration test phase.

## Details
Multiple lines of plan content for testing.
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
# Adversary YAML Structural Tests
#=============================================================================

@test "qa_integration: test_adversary_yaml_has_cross_phase_on_all_entries" {
    # All 5 adversaries must have a cross_phase field explicitly set
    [[ -f "$ADVERSARIES_FILE" ]]

    # Count adversaries
    local count
    count=$(yq '.adversaries | length' "$ADVERSARIES_FILE")
    [[ "$count" -eq 5 ]]

    # Check each adversary's cross_phase value
    local coverage_cp ambiguity_cp scope_cp consistency_cp executability_cp

    coverage_cp=$(yq '.adversaries[] | select(.name == "coverage") | .cross_phase' "$ADVERSARIES_FILE")
    [[ "$coverage_cp" == "false" ]]

    ambiguity_cp=$(yq '.adversaries[] | select(.name == "ambiguity") | .cross_phase' "$ADVERSARIES_FILE")
    [[ "$ambiguity_cp" == "false" ]]

    scope_cp=$(yq '.adversaries[] | select(.name == "scope") | .cross_phase' "$ADVERSARIES_FILE")
    [[ "$scope_cp" == "true" ]]

    consistency_cp=$(yq '.adversaries[] | select(.name == "consistency") | .cross_phase' "$ADVERSARIES_FILE")
    [[ "$consistency_cp" == "true" ]]

    executability_cp=$(yq '.adversaries[] | select(.name == "executability") | .cross_phase' "$ADVERSARIES_FILE")
    [[ "$executability_cp" == "false" ]]
}

#=============================================================================
# Script Syntax Validation Tests
#=============================================================================

@test "qa_integration: test_plan_review_loop_syntax_valid" {
    run bash -n "$PLAN_REVIEW_LOOP"
    [[ "$status" -eq 0 ]]
}

@test "qa_integration: test_iterate_syntax_valid" {
    run bash -n "$ITERATE"
    [[ "$status" -eq 0 ]]
}

@test "qa_integration: test_build_context_syntax_valid" {
    run bash -n "$BUILD_CONTEXT"
    [[ "$status" -eq 0 ]]
}

#=============================================================================
# Existing Test Suite Regression Tests
#=============================================================================

@test "qa_integration: test_existing_build_context_tests_pass" {
    run bats "$TEST_DIR/build_context.bats"
    [[ "$status" -eq 0 ]]
}

@test "qa_integration: test_existing_extract_verdict_tests_pass" {
    run bats "$TEST_DIR/extract_verdict.bats"
    [[ "$status" -eq 0 ]]
}

@test "qa_integration: test_existing_render_template_tests_pass" {
    run bats "$TEST_DIR/render_template.bats"
    [[ "$status" -eq 0 ]]
}

#=============================================================================
# Combined State-Trimming + Plan-File-Reference Tests
#=============================================================================

@test "qa_integration: test_build_context_state_trimming_and_plan_file_combined" {
    # Create state with excluded fields AND plan_md_sent_to tracking
    cat > "$PHASE_DIR/state.json" << 'JSON'
{
    "iteration": 1,
    "current_state": "impl",
    "verdicts_history": [{"iteration": 0, "state": "qa", "verdict": "approved"}],
    "hang_count": 2,
    "plan_md_sent_to": []
}
JSON

    build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" "impl"
    [[ "$status" -eq 0 ]]

    # (1) state-trimming: verdicts_history excluded
    assert_json_raw '.state.verdicts_history' 'null'

    # (2) state-trimming: hang_count excluded
    assert_json_raw '.state.hang_count' 'null'

    # (3) plan-file-reference: plan_file is non-empty
    local plan_file
    plan_file=$(get_field '.plan_file')
    [[ -n "$plan_file" ]]
    [[ "$plan_file" == *"plan.md" ]]

    # (4) plan-file-reference: plan_md is non-empty (first call for "impl")
    local plan_md
    plan_md=$(get_field '.plan_md')
    [[ -n "$plan_md" ]]
    [[ "$plan_md" == *"# Test Phase"* ]]

    # (5) state-trimming: plan_md_sent_to preserved by whitelist
    local sent_to
    sent_to=$(echo "$output" | jq -c '.state.plan_md_sent_to')
    [[ "$sent_to" == "[]" ]]
}

@test "qa_integration: test_build_context_plan_md_repeat_call_with_trimmed_state" {
    # State with iteration 2, impl already received plan_md, AND excluded fields present
    cat > "$PHASE_DIR/state.json" << 'JSON'
{
    "iteration": 2,
    "plan_md_sent_to": ["impl"],
    "verdicts_history": [{"iteration": 0, "state": "qa", "verdict": "approved"}]
}
JSON

    build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" "impl"
    [[ "$status" -eq 0 ]]

    # plan_md should be empty (repeat call for impl)
    assert_json_field '.plan_md' ''

    # verdicts_history should be null (trimmed)
    assert_json_raw '.state.verdicts_history' 'null'
}

@test "qa_integration: test_build_context_output_excludes_verdicts_history" {
    echo '{"iteration": 1, "verdicts_history": [{"iteration": 0}]}' > "$PHASE_DIR/state.json"

    build_context
    [[ "$status" -eq 0 ]]

    assert_json_raw '.state.verdicts_history' 'null'
}

@test "qa_integration: test_build_context_output_includes_plan_file" {
    build_context
    [[ "$status" -eq 0 ]]

    local plan_file
    plan_file=$(get_field '.plan_file')
    [[ -n "$plan_file" ]]
    [[ "$plan_file" != "null" ]]
}

#=============================================================================
# Escalation Ladder Tests (iterate.sh)
#=============================================================================

@test "qa_integration: test_escalation_ladder_model_assignments" {
    # Read the escalation blocks from iterate.sh
    local iterate_content
    iterate_content=$(cat "$ITERATE")

    # stuck=3: uses sonnet
    local level3_block
    level3_block=$(sed -n '/STUCK_ITERATIONS -eq 3/,/exit 0/p' "$ITERATE" | head -30)
    [[ "$level3_block" == *"--model sonnet"* ]]

    # stuck=4: uses sonnet
    local level4_block
    level4_block=$(sed -n '/STUCK_ITERATIONS -eq 4/,/exit 0/p' "$ITERATE" | head -30)
    [[ "$level4_block" == *"--model sonnet"* ]]

    # stuck=5: uses opus
    local level5_block
    level5_block=$(sed -n '/STUCK_ITERATIONS -eq 5/,/exit 0/p' "$ITERATE" | head -40)
    [[ "$level5_block" == *"--model opus"* ]]

    # stuck=6+: uses auto-split
    [[ "$iterate_content" == *"auto-split"* ]]
}

@test "qa_integration: test_no_opus_in_level_3_or_4" {
    # Extract stuck=3 and stuck=4 blocks and verify no opus
    local level3_block
    level3_block=$(sed -n '/STUCK_ITERATIONS -eq 3/,/exit 0/p' "$ITERATE" | head -30)
    ! echo "$level3_block" | grep -q '\-\-model opus'

    local level4_block
    level4_block=$(sed -n '/STUCK_ITERATIONS -eq 4/,/exit 0/p' "$ITERATE" | head -30)
    ! echo "$level4_block" | grep -q '\-\-model opus'
}

@test "qa_integration: test_escalation_context_available_at_level_5" {
    # ESCALATION_CONTEXT must be defined BEFORE the stuck=3 block
    # so it's available for all escalation levels (3, 4, 5)
    local context_line stuck3_line

    context_line=$(grep -n 'ESCALATION_CONTEXT=' "$ITERATE" | head -1 | cut -d: -f1)
    stuck3_line=$(grep -n 'STUCK_ITERATIONS -eq 3' "$ITERATE" | head -1 | cut -d: -f1)

    [[ -n "$context_line" ]]
    [[ -n "$stuck3_line" ]]
    [[ "$context_line" -lt "$stuck3_line" ]]
}

@test "qa_integration: test_investigation_injection_with_escalation" {
    # The impl case block should:
    # 1. Define INVESTIGATION_SECTION that reads investigation.md
    # 2. Concatenate it in the PROMPT line
    local impl_block
    impl_block=$(sed -n '/^[[:space:]]*impl)/,/^[[:space:]]*;;/p' "$ITERATE")

    # Check INVESTIGATION_SECTION definition
    echo "$impl_block" | grep -q 'INVESTIGATION_SECTION'
    echo "$impl_block" | grep -q 'investigation.md'

    # Check the concatenation line includes INVESTIGATION_SECTION
    echo "$impl_block" | grep -q 'ORCH_SECTION.*DISPUTE_SECTION.*INVESTIGATION_SECTION.*PROMPT'
}

#=============================================================================
# Adversary Skip Cache Tests (plan-review-loop.sh)
#=============================================================================

@test "qa_integration: test_adversary_history_schema_supports_phase_hashes" {
    # Create a history file using the format from adversary-skip-cache phase
    local history_file="$TEST_TEMP_DIR/history.json"
    cat > "$history_file" << 'JSON'
{
    "iterations": [
        {
            "iteration": 1,
            "phase_hashes": {
                "phase-a": "abc123",
                "phase-b": "def456"
            },
            "results": {
                "coverage": "passed",
                "ambiguity": "failed"
            }
        }
    ],
    "next_iteration": 2
}
JSON

    # Validate with jq
    run jq '.' "$history_file"
    [[ "$status" -eq 0 ]]

    # Check phase_hashes is an object
    local phase_hashes_type
    phase_hashes_type=$(jq -r '.iterations[0].phase_hashes | type' "$history_file")
    [[ "$phase_hashes_type" == "object" ]]
}

@test "qa_integration: test_plan_review_loop_has_no_bare_plan_content_var" {
    # PLAN_CONTENT= should be replaced by PLAN_CONTENT_FULL=
    # Grep for bare PLAN_CONTENT= excluding valid variants
    local matches
    matches=$(grep 'PLAN_CONTENT=' "$PLAN_REVIEW_LOOP" | grep -v 'PLAN_CONTENT_FULL\|PLAN_CONTENT_CHANGED\|PLAN_CONTENT_LENGTH' || true)

    [[ -z "$matches" ]]
}

@test "qa_integration: test_result_collection_has_cached_case" {
    # The result collection loop must have a cached) case
    local result_block
    result_block=$(sed -n '/case "\$status"/,/esac/p' "$PLAN_REVIEW_LOOP")

    # Must contain cached) case
    echo "$result_block" | grep -q 'cached)'

    # Must display "SKIPPED (cached:"
    echo "$result_block" | grep -q 'SKIPPED (cached:'

    # Must record passed result for cached entries
    echo "$result_block" | grep -q 'passed'
}

#=============================================================================
# Skip + Content Routing Pipeline Test (adversary-skip-cache + per-phase-content)
#=============================================================================

@test "qa_integration: test_skip_and_content_routing_pipeline" {
    local script_content
    script_content=$(cat "$PLAN_REVIEW_LOOP")

    # 1. compute_phase_hashes function defined
    echo "$script_content" | grep -q 'compute_phase_hashes()'

    # 2. CURRENT_HASHES computed in main body
    echo "$script_content" | grep -q 'CURRENT_HASHES='

    # 3. CHANGED_PHASES computed from hash comparison
    echo "$script_content" | grep -q 'CHANGED_PHASES='

    # 4. Safety valve at ITERATION % 3 == 0
    echo "$script_content" | grep -q 'ITERATION % 3'

    # 5. PLAN_CONTENT_FULL and PLAN_CONTENT_CHANGED both defined
    echo "$script_content" | grep -q 'PLAN_CONTENT_FULL='
    echo "$script_content" | grep -q 'PLAN_CONTENT_CHANGED='

    # 6. Dispatch loop has skip logic (can_skip check)
    echo "$script_content" | grep -q 'can_skip'

    # 7. Dispatch loop has content routing (is_cross_phase -> plan_input selection)
    echo "$script_content" | grep -q 'is_cross_phase'
    echo "$script_content" | grep -q 'plan_input='

    # 8. cached) case in result collection
    echo "$script_content" | grep -q 'cached)'
}

#=============================================================================
# Edge Case: All optimizations active simultaneously
#=============================================================================

@test "qa_integration: test_all_optimizations_active_simultaneously" {
    # State with excluded fields, plan_md tracking, all conditions active
    cat > "$PHASE_DIR/state.json" << 'JSON'
{
    "iteration": 3,
    "current_state": "impl",
    "stuck_iterations": 2,
    "tests_passing": 8,
    "tests_total": 10,
    "last_verdict": "concerns",
    "packages": ["test-package"],
    "plan_md_sent_to": ["qa"],
    "verdicts_history": [{"iteration": 0}, {"iteration": 1}],
    "hang_count": 1,
    "crash_count": 0,
    "transitions": [{"from": "qa", "to": "impl"}],
    "escalation_history": [{"level": 3}],
    "phase_status": "implementing",
    "current_model": "sonnet",
    "disputes": [],
    "last_cleared_disputes": []
}
JSON

    build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" "impl"
    [[ "$status" -eq 0 ]]

    # State-trimming: excluded fields are null
    assert_json_raw '.state.verdicts_history' 'null'
    assert_json_raw '.state.hang_count' 'null'
    assert_json_raw '.state.crash_count' 'null'
    assert_json_raw '.state.transitions' 'null'
    assert_json_raw '.state.escalation_history' 'null'

    # State-trimming: whitelisted fields preserved
    assert_json_raw '.state.iteration' '3'
    assert_json_raw '.state.stuck_iterations' '2'
    assert_json_raw '.state.tests_passing' '8'
    assert_json_raw '.state.tests_total' '10'
    assert_json_field '.state.last_verdict' 'concerns'
    assert_json_field '.state.current_model' 'sonnet'
    assert_json_field '.state.phase_status' 'implementing'

    # Plan-file-reference: plan_file present
    local plan_file
    plan_file=$(get_field '.plan_file')
    [[ -n "$plan_file" ]]

    # Plan-file-reference: plan_md embedded (impl not yet in sent_to, only qa)
    local plan_md
    plan_md=$(get_field '.plan_md')
    [[ -n "$plan_md" ]]
    [[ "$plan_md" == *"# Test Phase"* ]]

    # Output is valid JSON
    echo "$output" | jq empty
    [[ $? -eq 0 ]]
}

#=============================================================================
# Combined jq merge correctness
#=============================================================================

@test "qa_integration: test_combined_jq_merge_correctness" {
    # Verify the single jq merge in build-context.sh correctly includes
    # both state_json_slim AND plan_file
    local merge_block
    merge_block=$(sed -n '/jq -n/,/'"'"'$/p' "$BUILD_CONTEXT")

    # Must use state_json_slim (not state_json)
    echo "$merge_block" | grep -q 'state_json_slim'

    # Must include plan_file
    echo "$merge_block" | grep -q 'plan_file'

    # Must include plan_md
    echo "$merge_block" | grep -q 'plan_md'
}

@test "qa_integration: test_state_file_on_disk_never_modified_by_build_context" {
    # The on-disk state.json must NOT be modified during context building
    cat > "$PHASE_DIR/state.json" << 'JSON'
{
    "iteration": 1,
    "verdicts_history": [{"iteration": 0}],
    "hang_count": 5,
    "plan_md_sent_to": []
}
JSON
    local checksum_before
    checksum_before=$(md5sum "$PHASE_DIR/state.json" | cut -d' ' -f1)

    build_context
    [[ "$status" -eq 0 ]]

    local checksum_after
    checksum_after=$(md5sum "$PHASE_DIR/state.json" | cut -d' ' -f1)
    [[ "$checksum_before" == "$checksum_after" ]]
}
