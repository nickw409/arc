#!/usr/bin/env bats

# Tests for V2 verdict consistency validation in validate-workflow.sh
# Phase: 02-branch-resolution (orchestration-v2)

setup() {
    load 'test_helper'
    setup_temp_dir
}

teardown() {
    teardown_temp_dir
}

# ==============================================================================
# test_v2_valid_workflow
# Valid V2 workflow with proper verdicts and transitions should pass
# ==============================================================================
@test "V2: valid workflow with verdicts passes validation" {
    cat > "$TEST_TEMP_DIR/valid_v2.yaml" << 'EOF'
name: test
version: 2
states:
  - name: work
    prompt: prompts/feature/qa.md
    next: review
  - name: review
    prompt: prompts/feature/review.md
    verdicts:
      - approved
      - needs_work
    next:
      approved: complete
      needs_work: work
  - name: complete
    prompt: prompts/common/complete.md
entry_state: work
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/valid_v2.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Verdict consistency valid"* ]]
}

# ==============================================================================
# test_v2_missing_verdicts_field
# State with map next but no verdicts array should fail
# ==============================================================================
@test "V2: state with conditional transitions but no verdicts fails" {
    cat > "$TEST_TEMP_DIR/missing_verdicts.yaml" << 'EOF'
name: test
version: 2
states:
  - name: review
    prompt: prompts/feature/review.md
    next:
      approved: complete
      needs_work: work
  - name: complete
    prompt: prompts/common/complete.md
  - name: work
    prompt: prompts/feature/qa.md
    next: review
entry_state: review
terminal_states: [complete, work]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/missing_verdicts.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"State 'review' has conditional transitions but no verdicts defined"* ]]
}

# ==============================================================================
# test_v2_verdict_without_transition
# Verdict declared but no corresponding transition should fail
# ==============================================================================
@test "V2: verdict without transition fails" {
    cat > "$TEST_TEMP_DIR/verdict_no_transition.yaml" << 'EOF'
name: test
version: 2
states:
  - name: review
    prompt: prompts/feature/review.md
    verdicts:
      - approved
      - needs_work
      - escalate
    next:
      approved: complete
      needs_work: work
  - name: complete
    prompt: prompts/common/complete.md
  - name: work
    prompt: prompts/feature/qa.md
    next: review
entry_state: review
terminal_states: [complete, work]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/verdict_no_transition.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"State 'review' declares verdict 'escalate' but has no transition for it"* ]]
}

# ==============================================================================
# test_v2_transition_without_verdict
# Transition key not in verdicts list should fail
# ==============================================================================
@test "V2: transition without verdict fails" {
    cat > "$TEST_TEMP_DIR/transition_no_verdict.yaml" << 'EOF'
name: test
version: 2
states:
  - name: review
    prompt: prompts/feature/review.md
    verdicts:
      - approved
    next:
      approved: complete
      needs_work: work
  - name: complete
    prompt: prompts/common/complete.md
  - name: work
    prompt: prompts/feature/qa.md
    next: review
entry_state: review
terminal_states: [complete, work]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/transition_no_verdict.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"State 'review' has transition for 'needs_work' but it's not in verdicts list"* ]]
}

# ==============================================================================
# test_v1_workflow_still_valid
# V1 workflows should pass without requiring verdicts
# ==============================================================================
@test "V1: workflow without verdicts still passes" {
    cat > "$TEST_TEMP_DIR/v1_workflow.yaml" << 'EOF'
name: test
version: 1
states:
  - name: work
    prompt: prompts/feature/qa.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: work
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/v1_workflow.yaml"
    [[ "$status" -eq 0 ]]
}

# ==============================================================================
# test_mixed_v1_v2_states
# V2 workflow with mixed V1/V2 states should pass
# ==============================================================================
@test "V2: mixed V1/V2 states (string and map next) passes" {
    cat > "$TEST_TEMP_DIR/mixed_states.yaml" << 'EOF'
name: test
version: 2
states:
  - name: work
    prompt: prompts/feature/qa.md
    next: review
  - name: review
    prompt: prompts/feature/review.md
    verdicts:
      - approved
      - needs_work
    next:
      approved: complete
      needs_work: work
  - name: complete
    prompt: prompts/common/complete.md
entry_state: work
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/mixed_states.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Verdict consistency valid"* ]]
}

# ==============================================================================
# test_empty_verdicts_array
# Empty verdicts array with transitions should fail
# ==============================================================================
@test "V2: empty verdicts array with transitions fails" {
    cat > "$TEST_TEMP_DIR/empty_verdicts.yaml" << 'EOF'
name: test
version: 2
states:
  - name: review
    prompt: prompts/feature/review.md
    verdicts: []
    next:
      approved: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: review
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/empty_verdicts.yaml"
    [[ "$status" -eq 1 ]]
    # Empty verdicts is caught by the "no verdicts defined" check since length is 0
    [[ "$output" == *"has conditional transitions but no verdicts defined"* ]]
}

# ==============================================================================
# test_duplicate_verdicts_warning
# Duplicate verdicts should pass with warning
# ==============================================================================
@test "V2: duplicate verdicts passes with warning" {
    cat > "$TEST_TEMP_DIR/duplicate_verdicts.yaml" << 'EOF'
name: test
version: 2
states:
  - name: review
    prompt: prompts/feature/review.md
    verdicts:
      - approved
      - approved
      - needs_work
    next:
      approved: complete
      needs_work: work
  - name: complete
    prompt: prompts/common/complete.md
  - name: work
    prompt: prompts/feature/qa.md
    next: review
entry_state: review
terminal_states: [complete, work]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/duplicate_verdicts.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"WARNING"* ]]
    [[ "$output" == *"Duplicate verdict 'approved' in state 'review'"* ]]
}

# ==============================================================================
# test_verdict_with_special_characters
# Verdict with dash should fail (not alphanumeric+underscore)
# ==============================================================================
@test "V2: verdict with dash fails validation" {
    cat > "$TEST_TEMP_DIR/dash_verdict.yaml" << 'EOF'
name: test
version: 2
states:
  - name: review
    prompt: prompts/feature/review.md
    verdicts:
      - approved
      - needs-work
    next:
      approved: complete
      needs-work: work
  - name: complete
    prompt: prompts/common/complete.md
  - name: work
    prompt: prompts/feature/qa.md
    next: review
entry_state: review
terminal_states: [complete, work]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/dash_verdict.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Invalid verdict name 'needs-work': must be alphanumeric with underscores only"* ]]
}

# ==============================================================================
# test_terminal_state_with_verdicts_ignored
# Terminal states with verdicts should be ignored (no validation)
# ==============================================================================
@test "V2: terminal state with verdicts is ignored" {
    cat > "$TEST_TEMP_DIR/terminal_verdicts.yaml" << 'EOF'
name: test
version: 2
states:
  - name: work
    prompt: prompts/feature/qa.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
    verdicts:
      - ignored
    next:
      ignored: work
entry_state: work
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/terminal_verdicts.yaml"
    [[ "$status" -eq 0 ]]
}

# ==============================================================================
# test_v1_version_with_map_next
# V1 workflow with V2-style fields should skip verdict validation
# ==============================================================================
@test "V1: workflow with map next skips verdict validation" {
    cat > "$TEST_TEMP_DIR/v1_map_next.yaml" << 'EOF'
name: test
version: 1
states:
  - name: review
    prompt: prompts/feature/review.md
    verdicts:
      - approved
    next:
      approved: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: review
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/v1_map_next.yaml"
    [[ "$status" -eq 0 ]]
}

# ==============================================================================
# test_get_workflow_version
# Version 2 should be recognized without warning
# ==============================================================================
@test "V2: version 2 is recognized without warning" {
    cat > "$TEST_TEMP_DIR/version_2.yaml" << 'EOF'
name: test
version: 2
states:
  - name: work
    prompt: prompts/feature/qa.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: work
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/version_2.yaml"
    [[ "$status" -eq 0 ]]
    # Should NOT contain the version warning
    [[ "$output" != *"expected 1 or 2"* ]]
}

# ==============================================================================
# test_get_workflow_version_missing
# Missing version should default to V1
# ==============================================================================
@test "V1: missing version defaults to V1" {
    cat > "$TEST_TEMP_DIR/no_version.yaml" << 'EOF'
name: test
states:
  - name: work
    prompt: prompts/work.md
    next: complete
entry_state: work
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/no_version.yaml"
    # Note: existing validation requires version field, so this tests the default behavior
    # The plan says version is required but get_workflow_version should return 1 if missing
    # This test verifies the behavior when version IS required
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"missing required field: version"* ]]
}

# ==============================================================================
# test_get_workflow_version_string
# Version as string "2" should be recognized
# ==============================================================================
@test "V2: string version '2' is handled correctly" {
    cat > "$TEST_TEMP_DIR/string_version.yaml" << 'EOF'
name: test
version: "2"
states:
  - name: work
    prompt: prompts/feature/qa.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: work
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/string_version.yaml"
    [[ "$status" -eq 0 ]]
    # Should NOT contain the version warning
    [[ "$output" != *"expected 1 or 2"* ]]
}

# ==============================================================================
# test_is_valid_verdict_name_valid
# Simple lowercase verdict should pass
# ==============================================================================
@test "V2: valid verdict name 'approved' passes" {
    cat > "$TEST_TEMP_DIR/valid_verdict.yaml" << 'EOF'
name: test
version: 2
states:
  - name: review
    prompt: prompts/feature/review.md
    verdicts: [approved]
    next: {approved: complete}
  - name: complete
    prompt: prompts/common/complete.md
entry_state: review
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/valid_verdict.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Verdict consistency valid"* ]]
}

# ==============================================================================
# test_is_valid_verdict_name_with_underscore
# Verdict with underscore should pass
# ==============================================================================
@test "V2: verdict name with underscore passes" {
    cat > "$TEST_TEMP_DIR/underscore_verdict.yaml" << 'EOF'
name: test
version: 2
states:
  - name: review
    prompt: prompts/feature/review.md
    verdicts: [needs_work]
    next: {needs_work: complete}
  - name: complete
    prompt: prompts/common/complete.md
entry_state: review
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/underscore_verdict.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Verdict consistency valid"* ]]
}

# ==============================================================================
# test_is_valid_verdict_name_with_digits
# Verdict with digits (not leading) should pass
# ==============================================================================
@test "V2: verdict name with digits after first char passes" {
    cat > "$TEST_TEMP_DIR/digit_verdict.yaml" << 'EOF'
name: test
version: 2
states:
  - name: review
    prompt: prompts/feature/review.md
    verdicts: [v2_ready]
    next: {v2_ready: complete}
  - name: complete
    prompt: prompts/common/complete.md
entry_state: review
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/digit_verdict.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Verdict consistency valid"* ]]
}

# ==============================================================================
# test_is_valid_verdict_name_invalid_uppercase
# Uppercase verdict should fail
# ==============================================================================
@test "V2: uppercase verdict name fails" {
    cat > "$TEST_TEMP_DIR/uppercase_verdict.yaml" << 'EOF'
name: test
version: 2
states:
  - name: review
    prompt: prompts/feature/review.md
    verdicts: [APPROVED]
    next: {APPROVED: complete}
  - name: complete
    prompt: prompts/common/complete.md
entry_state: review
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/uppercase_verdict.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Invalid verdict name 'APPROVED'"* ]]
}

# ==============================================================================
# test_is_valid_verdict_name_invalid_dash
# Verdict with dash should fail
# ==============================================================================
@test "V2: verdict name with dash fails" {
    cat > "$TEST_TEMP_DIR/dash_verdict2.yaml" << 'EOF'
name: test
version: 2
states:
  - name: review
    prompt: prompts/feature/review.md
    verdicts: [needs-work]
    next: {needs-work: complete}
  - name: complete
    prompt: prompts/common/complete.md
entry_state: review
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/dash_verdict2.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Invalid verdict name 'needs-work'"* ]]
}

# ==============================================================================
# test_is_valid_verdict_name_invalid_leading_digit
# Verdict starting with digit should fail
# ==============================================================================
@test "V2: verdict name starting with digit fails" {
    cat > "$TEST_TEMP_DIR/leading_digit.yaml" << 'EOF'
name: test
version: 2
states:
  - name: review
    prompt: prompts/feature/review.md
    verdicts: [1approved]
    next: {1approved: complete}
  - name: complete
    prompt: prompts/common/complete.md
entry_state: review
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/leading_digit.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Invalid verdict name '1approved'"* ]]
}

# ==============================================================================
# test_is_valid_verdict_name_invalid_empty
# Empty verdict name should fail
# ==============================================================================
@test "V2: empty verdict name fails" {
    cat > "$TEST_TEMP_DIR/empty_verdict.yaml" << 'EOF'
name: test
version: 2
states:
  - name: review
    prompt: prompts/feature/review.md
    verdicts: [""]
    next: {"": complete}
  - name: complete
    prompt: prompts/common/complete.md
entry_state: review
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/empty_verdict.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Invalid verdict name ''"* ]]
}

# ==============================================================================
# test_verdict_leading_underscore
# Verdict starting with underscore should fail
# ==============================================================================
@test "V2: verdict starting with underscore fails" {
    cat > "$TEST_TEMP_DIR/underscore_leading.yaml" << 'EOF'
name: test
version: 2
states:
  - name: review
    prompt: prompts/feature/review.md
    verdicts:
      - _approved
    next:
      _approved: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: review
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/underscore_leading.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Invalid verdict name '_approved'"* ]]
}

# ==============================================================================
# test_verdict_with_digits_valid
# Verdicts with digits after first char should pass
# ==============================================================================
@test "V2: verdicts with digits (approved1, v2_ready) pass" {
    cat > "$TEST_TEMP_DIR/digit_verdicts.yaml" << 'EOF'
name: test
version: 2
states:
  - name: review
    prompt: prompts/feature/review.md
    verdicts:
      - approved1
      - v2_ready
    next:
      approved1: complete
      v2_ready: work
  - name: complete
    prompt: prompts/common/complete.md
  - name: work
    prompt: prompts/feature/qa.md
    next: review
entry_state: review
terminal_states: [complete, work]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/digit_verdicts.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Verdict consistency valid"* ]]
}

# ==============================================================================
# test_verdict_mixed_case
# Mixed case verdict should fail
# ==============================================================================
@test "V2: mixed case verdict name fails" {
    cat > "$TEST_TEMP_DIR/mixed_case.yaml" << 'EOF'
name: test
version: 2
states:
  - name: review
    prompt: prompts/feature/review.md
    verdicts:
      - Approved
    next:
      Approved: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: review
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/mixed_case.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Invalid verdict name 'Approved'"* ]]
}

# ==============================================================================
# test_single_char_verdict_valid
# Single character verdict should pass
# ==============================================================================
@test "V2: single character verdict names pass" {
    cat > "$TEST_TEMP_DIR/single_char.yaml" << 'EOF'
name: test
version: 2
states:
  - name: review
    prompt: prompts/feature/review.md
    verdicts:
      - a
      - b
    next:
      a: complete
      b: work
  - name: complete
    prompt: prompts/common/complete.md
  - name: work
    prompt: prompts/feature/qa.md
    next: review
entry_state: review
terminal_states: [complete, work]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/single_char.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Verdict consistency valid"* ]]
}

# ==============================================================================
# Additional edge case: multiple states with verdicts
# ==============================================================================
@test "V2: multiple states with verdicts all validate" {
    cat > "$TEST_TEMP_DIR/multi_verdicts.yaml" << 'EOF'
name: test
version: 2
states:
  - name: qa
    prompt: prompts/feature/qa.md
    verdicts:
      - approved
      - needs_fix
    next:
      approved: impl
      needs_fix: qa
  - name: impl
    prompt: prompts/feature/impl.md
    verdicts:
      - done
      - blocked
    next:
      done: complete
      blocked: qa
  - name: complete
    prompt: prompts/common/complete.md
entry_state: qa
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/multi_verdicts.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Verdict consistency valid"* ]]
}

# ==============================================================================
# Edge case: null next field should be treated as terminal
# ==============================================================================
@test "V2: state with null next is treated as terminal" {
    cat > "$TEST_TEMP_DIR/null_next.yaml" << 'EOF'
name: test
version: 2
states:
  - name: work
    prompt: prompts/feature/qa.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
    next: null
entry_state: work
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/null_next.yaml"
    [[ "$status" -eq 0 ]]
}

# ==============================================================================
# Edge case: multiple verdicts missing transitions
# ==============================================================================
@test "V2: multiple verdicts without transitions reports all" {
    cat > "$TEST_TEMP_DIR/multi_missing.yaml" << 'EOF'
name: test
version: 2
states:
  - name: review
    prompt: prompts/feature/review.md
    verdicts:
      - approved
      - needs_work
      - escalate
      - defer
    next:
      approved: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: review
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/multi_missing.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"declares verdict 'needs_work' but has no transition"* ]]
    [[ "$output" == *"declares verdict 'escalate' but has no transition"* ]]
    [[ "$output" == *"declares verdict 'defer' but has no transition"* ]]
}

# ==============================================================================
# Edge case: multiple transitions without verdicts
# ==============================================================================
@test "V2: multiple transitions without verdicts reports all" {
    cat > "$TEST_TEMP_DIR/multi_extra.yaml" << 'EOF'
name: test
version: 2
states:
  - name: review
    prompt: prompts/feature/review.md
    verdicts:
      - approved
    next:
      approved: complete
      needs_work: work
      escalate: blocked
  - name: complete
    prompt: prompts/common/complete.md
  - name: work
    prompt: prompts/feature/qa.md
    next: review
  - name: blocked
    prompt: prompts/common/blocked.md
entry_state: review
terminal_states: [complete, work, blocked]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/multi_extra.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"transition for 'needs_work' but it's not in verdicts list"* ]]
    [[ "$output" == *"transition for 'escalate' but it's not in verdicts list"* ]]
}

# ==============================================================================
# Edge case: V3 version should warn (not 1 or 2)
# ==============================================================================
@test "V3: unknown version produces warning" {
    cat > "$TEST_TEMP_DIR/v3_workflow.yaml" << 'EOF'
name: test
version: 3
states:
  - name: work
    prompt: prompts/feature/qa.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: work
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/v3_workflow.yaml"
    # Should still pass (warning not error), but emit warning
    [[ "$output" == *"expected 1 or 2"* ]]
}
