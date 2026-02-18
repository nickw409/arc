#!/usr/bin/env bats
# QA Tests for state-trimming phase (token-optimization plan)
#
# Tests that build-context.sh projects state.json through a whitelist before
# passing it to templates, stripping unused fields (verdicts_history,
# transitions, escalation_history, hang_count, crash_count, and any unknown
# custom fields) while preserving all template-referenced fields.
#
# The on-disk state.json is NEVER modified; only the in-memory copy used
# in the jq merge is slimmed.

setup() {
    load 'test_helper'
    setup_temp_dir

    BUILD_CONTEXT="$SCRIPTS_DIR/build-context.sh"

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
entry_state: impl
terminal_states: [complete]
YAML

    # Create plan.md
    printf '# Test Phase\n' > "$PHASE_DIR/plan.md"
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

# Helper: extract a JSON field from output
get_field() {
    echo "$output" | jq -r "$1"
}

# Helper: assert JSON field equals expected value
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
# Script Syntax
#=============================================================================

@test "qa_state-trimming: test_script_syntax_valid" {
    run bash -n "$BUILD_CONTEXT"
    [ "$status" -eq 0 ]
}

#=============================================================================
# Excluded Fields — must be null/absent in .state
#=============================================================================

@test "qa_state-trimming: test_slim_state_excludes_verdicts_history" {
    echo '{"iteration": 1, "verdicts_history": [{"iteration": 0, "state": "qa", "verdict": "approved"}]}' > "$PHASE_DIR/state.json"

    build_context
    [ "$status" -eq 0 ]

    assert_json_raw '.state.verdicts_history' 'null'
}

@test "qa_state-trimming: test_slim_state_excludes_transitions" {
    echo '{"iteration": 1, "transitions": [{"from": "qa", "to": "impl"}]}' > "$PHASE_DIR/state.json"

    build_context
    [ "$status" -eq 0 ]

    assert_json_raw '.state.transitions' 'null'
}

@test "qa_state-trimming: test_slim_state_excludes_escalation_history" {
    echo '{"iteration": 1, "escalation_history": [{"level": 3, "action": "investigate"}]}' > "$PHASE_DIR/state.json"

    build_context
    [ "$status" -eq 0 ]

    assert_json_raw '.state.escalation_history' 'null'
}

@test "qa_state-trimming: test_slim_state_excludes_hang_count" {
    echo '{"iteration": 1, "hang_count": 2}' > "$PHASE_DIR/state.json"

    build_context
    [ "$status" -eq 0 ]

    assert_json_raw '.state.hang_count' 'null'
}

@test "qa_state-trimming: test_slim_state_excludes_crash_count" {
    echo '{"iteration": 1, "crash_count": 1}' > "$PHASE_DIR/state.json"

    build_context
    [ "$status" -eq 0 ]

    assert_json_raw '.state.crash_count' 'null'
}

@test "qa_state-trimming: test_slim_state_excludes_unknown_custom_fields" {
    echo '{"iteration": 1, "custom_data": "foo", "my_plugin_state": {"key": "value"}}' > "$PHASE_DIR/state.json"

    build_context
    [ "$status" -eq 0 ]

    assert_json_raw '.state.custom_data' 'null'
    assert_json_raw '.state.my_plugin_state' 'null'
}

#=============================================================================
# Whitelisted Fields — must be preserved in .state
#=============================================================================

@test "qa_state-trimming: test_slim_state_keeps_iteration" {
    echo '{"iteration": 5}' > "$PHASE_DIR/state.json"

    build_context
    [ "$status" -eq 0 ]

    assert_json_raw '.state.iteration' '5'
}

@test "qa_state-trimming: test_slim_state_keeps_tests_passing" {
    echo '{"iteration": 1, "tests_passing": 8, "tests_total": 10}' > "$PHASE_DIR/state.json"

    build_context
    [ "$status" -eq 0 ]

    assert_json_raw '.state.tests_passing' '8'
    assert_json_raw '.state.tests_total' '10'
}

@test "qa_state-trimming: test_slim_state_keeps_stuck_iterations" {
    echo '{"iteration": 1, "stuck_iterations": 3}' > "$PHASE_DIR/state.json"

    build_context
    [ "$status" -eq 0 ]

    assert_json_raw '.state.stuck_iterations' '3'
}

@test "qa_state-trimming: test_slim_state_keeps_last_verdict" {
    echo '{"iteration": 1, "last_verdict": "approved"}' > "$PHASE_DIR/state.json"

    build_context
    [ "$status" -eq 0 ]

    assert_json_field '.state.last_verdict' 'approved'
}

@test "qa_state-trimming: test_slim_state_keeps_crates" {
    echo '{"iteration": 1, "packages": ["test-package", "test-shared-package"]}' > "$PHASE_DIR/state.json"

    build_context
    [ "$status" -eq 0 ]

    local crate_count
    crate_count=$(echo "$output" | jq '.state.packages | length')
    [ "$crate_count" -eq 2 ]
}

@test "qa_state-trimming: test_slim_state_keeps_disputes" {
    echo '{"iteration": 1, "disputes": [{"test_name": "test_foo", "reason": "flaky"}]}' > "$PHASE_DIR/state.json"

    build_context
    [ "$status" -eq 0 ]

    local dispute_count
    dispute_count=$(echo "$output" | jq '.state.disputes | length')
    [ "$dispute_count" -eq 1 ]
    assert_json_field '.state.disputes[0].test_name' 'test_foo'
}

@test "qa_state-trimming: test_slim_state_keeps_phase_status" {
    echo '{"iteration": 1, "phase_status": "implementing"}' > "$PHASE_DIR/state.json"

    build_context
    [ "$status" -eq 0 ]

    assert_json_field '.state.phase_status' 'implementing'
}

@test "qa_state-trimming: test_slim_state_keeps_plan_md_sent_to" {
    echo '{"iteration": 1, "plan_md_sent_to": ["qa", "impl"]}' > "$PHASE_DIR/state.json"

    build_context
    [ "$status" -eq 0 ]

    local sent_count
    sent_count=$(echo "$output" | jq '.state.plan_md_sent_to | length')
    [ "$sent_count" -eq 2 ]
}

@test "qa_state-trimming: test_slim_state_keeps_current_model" {
    echo '{"iteration": 1, "current_model": "sonnet"}' > "$PHASE_DIR/state.json"

    build_context
    [ "$status" -eq 0 ]

    assert_json_field '.state.current_model' 'sonnet'
}

@test "qa_state-trimming: test_slim_state_keeps_current_state" {
    echo '{"iteration": 1, "current_state": "impl"}' > "$PHASE_DIR/state.json"

    build_context
    [ "$status" -eq 0 ]

    assert_json_field '.state.current_state' 'impl'
}

@test "qa_state-trimming: test_slim_state_keeps_last_cleared_disputes" {
    echo '{"iteration": 1, "last_cleared_disputes": [{"test_name": "test_bar"}]}' > "$PHASE_DIR/state.json"

    build_context
    [ "$status" -eq 0 ]

    local count
    count=$(echo "$output" | jq '.state.last_cleared_disputes | length')
    [ "$count" -eq 1 ]
}

#=============================================================================
# Edge Cases
#=============================================================================

@test "qa_state-trimming: test_missing_whitelist_fields_are_null" {
    # Minimal state: only iteration, no optional fields
    echo '{"iteration": 1}' > "$PHASE_DIR/state.json"

    build_context
    [ "$status" -eq 0 ]

    # Script doesn't fail; missing whitelisted fields are null
    assert_json_raw '.state.stuck_iterations' 'null'
    assert_json_raw '.state.tests_passing' 'null'
    assert_json_raw '.state.tests_total' 'null'
    assert_json_raw '.state.last_verdict' 'null'
    assert_json_raw '.state.packages' 'null'
    assert_json_raw '.state.disputes' 'null'
    assert_json_raw '.state.current_model' 'null'
    assert_json_raw '.state.plan_md_sent_to' 'null'
    assert_json_raw '.state.phase_status' 'null'
    assert_json_raw '.state.current_state' 'null'
    assert_json_raw '.state.last_cleared_disputes' 'null'
}

@test "qa_state-trimming: test_iteration_shorthand_still_works" {
    # Old iteration schema: object with current/max
    echo '{"iteration": {"current": 3, "max": 25}}' > "$PHASE_DIR/state.json"

    build_context
    [ "$status" -eq 0 ]

    # Top-level .iteration extracts from .current
    assert_json_raw '.iteration' '3'
}

@test "qa_state-trimming: test_slim_state_preserves_nested_disputes" {
    echo '{"iteration": 1, "disputes": [{"test_name": "test_foo", "reason": "flaky", "resolution": "approved", "details": {"file": "test.rs", "line": 42}}]}' > "$PHASE_DIR/state.json"

    build_context
    [ "$status" -eq 0 ]

    assert_json_field '.state.disputes[0].details.file' 'test.rs'
    assert_json_raw '.state.disputes[0].details.line' '42'
}

@test "qa_state-trimming: test_slim_state_is_smaller" {
    echo '{"iteration": 1, "tests_passing": 5, "verdicts_history": [{"iteration":0},{"iteration":1},{"iteration":2}], "transitions": [{"from":"qa","to":"impl"}], "hang_count": 0, "crash_count": 0}' > "$PHASE_DIR/state.json"

    build_context
    [ "$status" -eq 0 ]

    # Excluded fields must not be in state
    assert_json_raw '.state.verdicts_history' 'null'
    assert_json_raw '.state.transitions' 'null'
    assert_json_raw '.state.hang_count' 'null'
    assert_json_raw '.state.crash_count' 'null'

    # Whitelisted fields are present
    assert_json_raw '.state.iteration' '1'
    assert_json_raw '.state.tests_passing' '5'
}

@test "qa_state-trimming: test_slim_state_empty_object" {
    echo '{}' > "$PHASE_DIR/state.json"

    build_context
    [ "$status" -eq 0 ]

    # All whitelisted fields are null
    assert_json_raw '.state.iteration' 'null'
    assert_json_raw '.state.stuck_iterations' 'null'
    assert_json_raw '.state.tests_passing' 'null'

    # Top-level iteration defaults to 0 via // 0 fallback
    assert_json_raw '.iteration' '0'
}

@test "qa_state-trimming: test_state_with_only_excluded_fields" {
    # State file with ONLY excluded fields — edge case 1
    echo '{"verdicts_history": [{"iteration": 0}], "hang_count": 2, "crash_count": 1}' > "$PHASE_DIR/state.json"

    build_context
    [ "$status" -eq 0 ]

    # All excluded fields stripped
    assert_json_raw '.state.verdicts_history' 'null'
    assert_json_raw '.state.hang_count' 'null'
    assert_json_raw '.state.crash_count' 'null'

    # All whitelisted fields are null
    assert_json_raw '.state.iteration' 'null'

    # Top-level iteration defaults to 0
    assert_json_raw '.iteration' '0'
}

@test "qa_state-trimming: test_state_with_no_excluded_fields" {
    # State file with no excluded fields — edge case 2
    echo '{"iteration": 1, "tests_passing": 5}' > "$PHASE_DIR/state.json"

    build_context
    [ "$status" -eq 0 ]

    # Whitelisted fields preserved
    assert_json_raw '.state.iteration' '1'
    assert_json_raw '.state.tests_passing' '5'

    # Unknown fields still stripped (only whitelisted pass through)
    # The state object should not have extra keys beyond the whitelist
    local key_count
    key_count=$(echo "$output" | jq '.state | keys | length')
    # Whitelist has 12 fields; all present as keys even if null
    [ "$key_count" -eq 12 ]
}

@test "qa_state-trimming: test_null_values_in_whitelisted_fields" {
    # Edge case 5: null values in whitelisted fields preserved as null
    echo '{"iteration": 1, "packages": null, "disputes": null}' > "$PHASE_DIR/state.json"

    build_context
    [ "$status" -eq 0 ]

    assert_json_raw '.state.packages' 'null'
    assert_json_raw '.state.disputes' 'null'
    assert_json_raw '.state.iteration' '1'
}

#=============================================================================
# Regression — output structure unchanged
#=============================================================================

@test "qa_state-trimming: test_output_still_has_required_fields" {
    echo '{"iteration": 3, "tests_passing": 5, "tests_total": 10}' > "$PHASE_DIR/state.json"

    build_context
    [ "$status" -eq 0 ]

    # All required computed fields exist
    echo "$output" | jq -e '.iteration' > /dev/null
    echo "$output" | jq -e '.current_state' > /dev/null
    echo "$output" | jq -e '.phase' > /dev/null
    echo "$output" | jq -e '.plan' > /dev/null
    echo "$output" | jq -e 'has("plan_md")' > /dev/null
    echo "$output" | jq -e 'has("state")' > /dev/null
    echo "$output" | jq -e 'has("params")' > /dev/null
}

@test "qa_state-trimming: test_output_is_valid_json" {
    echo '{"iteration": 1, "verdicts_history": [{"iteration": 0}]}' > "$PHASE_DIR/state.json"

    build_context
    [ "$status" -eq 0 ]

    echo "$output" | jq empty
    [ $? -eq 0 ]
}

@test "qa_state-trimming: test_defaults_still_merged" {
    echo '{"iteration": 1}' > "$PHASE_DIR/state.json"

    build_context
    [ "$status" -eq 0 ]

    # max_iterations from workflow defaults still present
    assert_json_raw '.max_iterations' '10'
}

@test "qa_state-trimming: test_computed_values_still_correct" {
    echo '{"iteration": 7}' > "$PHASE_DIR/state.json"

    build_context
    [ "$status" -eq 0 ]

    assert_json_raw '.iteration' '7'
    assert_json_field '.current_state' 'impl'
    assert_json_field '.phase' 'test-phase'
    assert_json_field '.plan' 'test-plan'
}

@test "qa_state-trimming: test_plan_md_still_included" {
    echo '{"iteration": 1}' > "$PHASE_DIR/state.json"

    build_context
    [ "$status" -eq 0 ]

    local plan_md
    plan_md=$(get_field '.plan_md')
    [[ "$plan_md" == *"# Test Phase"* ]]
}

@test "qa_state-trimming: test_state_file_on_disk_not_modified" {
    # The on-disk state.json must NOT be modified by build-context.sh
    echo '{"iteration": 1, "verdicts_history": [{"iteration": 0}], "hang_count": 5}' > "$PHASE_DIR/state.json"
    local checksum_before
    checksum_before=$(md5sum "$PHASE_DIR/state.json" | cut -d' ' -f1)

    build_context
    [ "$status" -eq 0 ]

    local checksum_after
    checksum_after=$(md5sum "$PHASE_DIR/state.json" | cut -d' ' -f1)
    [ "$checksum_before" = "$checksum_after" ]
}

#=============================================================================
# Verify the projection variable exists in script (structural tests)
#=============================================================================

@test "qa_state-trimming: test_state_json_slim_variable_exists" {
    # build-context.sh should define state_json_slim
    run grep 'state_json_slim=' "$BUILD_CONTEXT"
    [ "$status" -eq 0 ]
}

@test "qa_state-trimming: test_jq_merge_uses_slim_state" {
    # The final jq -n merge should use state_json_slim, not state_json
    run grep -E -- '--argjson state "\$state_json_slim"' "$BUILD_CONTEXT"
    [ "$status" -eq 0 ]
}

@test "qa_state-trimming: test_slim_projection_has_whitelist_fields" {
    # The jq projection should include all 12 whitelisted fields
    local script_content
    script_content=$(cat "$BUILD_CONTEXT")

    [[ "$script_content" == *"iteration:"* ]]
    [[ "$script_content" == *"stuck_iterations:"* ]]
    [[ "$script_content" == *"tests_passing:"* ]]
    [[ "$script_content" == *"tests_total:"* ]]
    [[ "$script_content" == *"last_verdict:"* ]]
    [[ "$script_content" == *"crates:"* ]]
    [[ "$script_content" == *"current_model:"* ]]
    [[ "$script_content" == *"plan_md_sent_to:"* ]]
    [[ "$script_content" == *"phase_status:"* ]]
    [[ "$script_content" == *"current_state:"* ]]
    [[ "$script_content" == *"disputes:"* ]]
    [[ "$script_content" == *"last_cleared_disputes:"* ]]
}

@test "qa_state-trimming: test_slim_projection_after_validation_before_workflow" {
    # state_json_slim must be defined AFTER state_json is read/validated
    # and BEFORE the workflow defaults extraction
    local slim_line state_read_line workflow_line

    # state_json is read via: state_json=$(jq '.' "$state_file" ...)
    state_read_line=$(grep -n 'state_json=' "$BUILD_CONTEXT" | head -1 | cut -d: -f1)
    slim_line=$(grep -n 'state_json_slim=' "$BUILD_CONTEXT" | head -1 | cut -d: -f1)
    # defaults_json extraction is the first workflow processing step
    workflow_line=$(grep -n 'defaults_json=' "$BUILD_CONTEXT" | head -1 | cut -d: -f1)

    [ -n "$state_read_line" ]
    [ -n "$slim_line" ]
    [ -n "$workflow_line" ]
    [ "$state_read_line" -lt "$slim_line" ]
    [ "$slim_line" -lt "$workflow_line" ]
}

#=============================================================================
# Existing test regression
#=============================================================================

@test "qa_state-trimming: test_existing_build_context_tests_pass" {
    run bats "$TEST_DIR/build_context.bats"
    [ "$status" -eq 0 ]
}
