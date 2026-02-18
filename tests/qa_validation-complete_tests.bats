#!/usr/bin/env bats
# QA Tests for validation-complete phase
# Tests the 6 new validation functions added to validate-workflow.sh:
# - check_unique_state_names
# - check_prompt_files_exist
# - check_next_references_valid
# - check_no_unreachable_states
# - check_all_reach_terminal
# - check_entry_not_terminal
# Plus the get_next_states helper function

setup() {
    load 'test_helper'
    setup_temp_dir

    # Create test prompt files that will be referenced by workflows
    mkdir -p "$ORCH_DIR/prompts/feature"
    mkdir -p "$ORCH_DIR/prompts/common"
    mkdir -p "$ORCH_DIR/prompts/test"

    # Ensure required prompts exist (some may already exist)
    touch "$ORCH_DIR/prompts/feature/qa.md" 2>/dev/null || true
    touch "$ORCH_DIR/prompts/feature/qa-review.md" 2>/dev/null || true
    touch "$ORCH_DIR/prompts/feature/impl.md" 2>/dev/null || true
    touch "$ORCH_DIR/prompts/feature/review.md" 2>/dev/null || true
    touch "$ORCH_DIR/prompts/feature/orphan.md" 2>/dev/null || true
    touch "$ORCH_DIR/prompts/feature/orphan1.md" 2>/dev/null || true
    touch "$ORCH_DIR/prompts/feature/orphan2.md" 2>/dev/null || true
    touch "$ORCH_DIR/prompts/feature/loop.md" 2>/dev/null || true
    touch "$ORCH_DIR/prompts/common/complete.md" 2>/dev/null || true
    touch "$ORCH_DIR/prompts/common/blocked.md" 2>/dev/null || true
    touch "$ORCH_DIR/prompts/common/only.md" 2>/dev/null || true
}

teardown() {
    teardown_temp_dir
}

# ==============================================================================
# check_unique_state_names tests
# ==============================================================================

@test "test_unique_names_pass - all state names are unique" {
    cat > "$TEST_TEMP_DIR/unique_names.yaml" << 'EOF'
name: test-unique-names
version: 1
states:
  - name: qa
    prompt: prompts/feature/qa.md
    next: qa_review
  - name: qa_review
    prompt: prompts/feature/qa-review.md
    next: impl
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: qa
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/unique_names.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"✓ All state names unique"* ]]
}

@test "test_unique_names_fail - detects duplicate state name" {
    cat > "$TEST_TEMP_DIR/duplicate_names.yaml" << 'EOF'
name: test-duplicate-names
version: 1
states:
  - name: qa
    prompt: prompts/feature/qa.md
    next: qa_review
  - name: qa
    prompt: prompts/feature/qa-dup.md
    next: impl
entry_state: qa
terminal_states: [impl]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/duplicate_names.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"❌ Duplicate state name: qa"* ]]
}

@test "test_unique_names_multiple_duplicates - reports all duplicate names" {
    cat > "$TEST_TEMP_DIR/many_duplicates.yaml" << 'EOF'
name: test-many-duplicates
version: 1
states:
  - name: qa
    prompt: prompts/feature/qa.md
    next: impl
  - name: qa
    prompt: prompts/feature/qa2.md
    next: impl
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: impl
    prompt: prompts/feature/impl2.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: qa
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/many_duplicates.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"❌ Duplicate state name: qa"* ]]
    [[ "$output" == *"❌ Duplicate state name: impl"* ]]
}

# ==============================================================================
# check_prompt_files_exist tests
# ==============================================================================

@test "test_prompt_exists_pass - all prompt files exist" {
    cat > "$TEST_TEMP_DIR/prompts_exist.yaml" << 'EOF'
name: test-prompt-exists
version: 1
states:
  - name: qa
    prompt: prompts/feature/qa.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: qa
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/prompts_exist.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"✓ Prompt exists: prompts/feature/qa.md"* ]]
    [[ "$output" == *"✓ Prompt exists: prompts/common/complete.md"* ]]
}

@test "test_prompt_not_found - detects missing prompt file" {
    cat > "$TEST_TEMP_DIR/prompt_missing.yaml" << 'EOF'
name: test-prompt-missing
version: 1
states:
  - name: qa
    prompt: prompts/nonexistent/fake.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: qa
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/prompt_missing.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"❌ Prompt not found: prompts/nonexistent/fake.md"* ]]
}

@test "test_prompt_multiple_missing - reports all missing prompts" {
    cat > "$TEST_TEMP_DIR/multiple_missing.yaml" << 'EOF'
name: test-multiple-missing
version: 1
states:
  - name: qa
    prompt: prompts/nonexistent/fake1.md
    next: impl
  - name: impl
    prompt: prompts/nonexistent/fake2.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: qa
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/multiple_missing.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"❌ Prompt not found: prompts/nonexistent/fake1.md"* ]]
    [[ "$output" == *"❌ Prompt not found: prompts/nonexistent/fake2.md"* ]]
}

# ==============================================================================
# check_next_references_valid tests
# ==============================================================================

@test "test_next_valid_pass - all next references point to valid states" {
    cat > "$TEST_TEMP_DIR/valid_next.yaml" << 'EOF'
name: test-valid-next
version: 1
states:
  - name: qa
    prompt: prompts/feature/qa.md
    next: qa_review
  - name: qa_review
    prompt: prompts/feature/qa-review.md
    next: impl
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: qa
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/valid_next.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"✓ Transition valid: qa -> qa_review"* ]]
    [[ "$output" == *"✓ Transition valid: qa_review -> impl"* ]]
    [[ "$output" == *"✓ Transition valid: impl -> complete"* ]]
}

@test "test_next_invalid - detects invalid next reference" {
    cat > "$TEST_TEMP_DIR/invalid_next.yaml" << 'EOF'
name: test-invalid-next
version: 1
states:
  - name: qa
    prompt: prompts/feature/qa.md
    next: typo_state
  - name: complete
    prompt: prompts/common/complete.md
entry_state: qa
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/invalid_next.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"❌ Invalid transition: qa -> typo_state"* ]]
}

# ==============================================================================
# check_no_unreachable_states tests
# ==============================================================================

@test "test_reachable_pass - all states reachable from entry" {
    cat > "$TEST_TEMP_DIR/reachable.yaml" << 'EOF'
name: test-reachable
version: 1
states:
  - name: qa
    prompt: prompts/feature/qa.md
    next: qa_review
  - name: qa_review
    prompt: prompts/feature/qa-review.md
    next: impl
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: qa
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/reachable.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"✓ All states reachable from entry"* ]]
}

@test "test_reachable_with_cycle - valid cycle still reachable" {
    cat > "$TEST_TEMP_DIR/cycle_reachable.yaml" << 'EOF'
name: test-cycle-reachable
version: 1
states:
  - name: qa
    prompt: prompts/feature/qa.md
    next: review
  - name: review
    prompt: prompts/feature/review.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
    next: qa
entry_state: qa
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/cycle_reachable.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"✓ All states reachable from entry"* ]]
    [[ "$output" == *"✓ All states can reach terminal"* ]]
}

@test "test_unreachable_state - detects orphan state" {
    cat > "$TEST_TEMP_DIR/unreachable.yaml" << 'EOF'
name: test-unreachable
version: 1
states:
  - name: qa
    prompt: prompts/feature/qa.md
    next: complete
  - name: orphan
    prompt: prompts/feature/orphan.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: qa
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/unreachable.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"❌ Unreachable state: orphan"* ]]
}

@test "test_multiple_unreachable - reports all orphan states" {
    cat > "$TEST_TEMP_DIR/multi_unreachable.yaml" << 'EOF'
name: test-multi-unreachable
version: 1
states:
  - name: qa
    prompt: prompts/feature/qa.md
    next: complete
  - name: orphan1
    prompt: prompts/feature/orphan1.md
    next: complete
  - name: orphan2
    prompt: prompts/feature/orphan2.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: qa
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/multi_unreachable.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"❌ Unreachable state: orphan1"* ]]
    [[ "$output" == *"❌ Unreachable state: orphan2"* ]]
}

# ==============================================================================
# check_all_reach_terminal tests
# ==============================================================================

@test "test_reaches_terminal_pass - all states can reach terminal" {
    cat > "$TEST_TEMP_DIR/reaches_terminal.yaml" << 'EOF'
name: test-reaches-terminal
version: 1
states:
  - name: qa
    prompt: prompts/feature/qa.md
    next: review
  - name: review
    prompt: prompts/feature/qa-review.md
    next: impl
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
  - name: blocked
    prompt: prompts/common/blocked.md
entry_state: qa
terminal_states: [complete, blocked]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/reaches_terminal.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"✓ All states can reach terminal"* ]]
}

@test "test_no_path_to_terminal - detects states trapped in infinite loop" {
    cat > "$TEST_TEMP_DIR/no_terminal_path.yaml" << 'EOF'
name: test-no-terminal-path
version: 1
states:
  - name: qa
    prompt: prompts/feature/qa.md
    next: infinite_loop
  - name: infinite_loop
    prompt: prompts/feature/loop.md
    next: infinite_loop
  - name: complete
    prompt: prompts/common/complete.md
entry_state: qa
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/no_terminal_path.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"❌ State cannot reach terminal: qa"* ]]
    [[ "$output" == *"❌ State cannot reach terminal: infinite_loop"* ]]
}

# ==============================================================================
# check_entry_not_terminal tests
# ==============================================================================

@test "test_entry_not_terminal_pass - entry is not a terminal state" {
    cat > "$TEST_TEMP_DIR/entry_not_terminal.yaml" << 'EOF'
name: test-entry-not-terminal
version: 1
states:
  - name: qa
    prompt: prompts/feature/qa.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: qa
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/entry_not_terminal.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"✓ Entry state is non-terminal"* ]]
}

@test "test_entry_is_terminal_fails - entry state cannot be terminal" {
    cat > "$TEST_TEMP_DIR/entry_is_terminal.yaml" << 'EOF'
name: test-entry-is-terminal
version: 1
states:
  - name: only
    prompt: prompts/common/only.md
entry_state: only
terminal_states: [only]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/entry_is_terminal.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"❌ Entry state 'only' cannot be terminal"* ]]
}

# ==============================================================================
# V2 branching tests (object next field)
# ==============================================================================

@test "test_v2_branching_next_valid - V2 branch map with valid targets" {
    cat > "$TEST_TEMP_DIR/v2_branching.yaml" << 'EOF'
name: test-v2-branching
version: 2
states:
  - name: qa
    prompt: prompts/feature/qa.md
    next:
      approved: impl
      rejected: blocked
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
  - name: blocked
    prompt: prompts/common/blocked.md
entry_state: qa
terminal_states: [complete, blocked]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/v2_branching.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"✓ Transition valid: qa -> impl"* ]]
    [[ "$output" == *"✓ Transition valid: qa -> blocked"* ]]
    [[ "$output" == *"✓ All states reachable from entry"* ]]
    [[ "$output" == *"✓ All states can reach terminal"* ]]
}

@test "test_v2_branching_next_invalid - V2 branch map with invalid target" {
    cat > "$TEST_TEMP_DIR/v2_branching_invalid.yaml" << 'EOF'
name: test-v2-branching-invalid
version: 2
states:
  - name: qa
    prompt: prompts/feature/qa.md
    next:
      approved: impl
      rejected: nonexistent
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: qa
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/v2_branching_invalid.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"❌ Invalid transition: qa -> nonexistent"* ]]
}

@test "test_v2_multiple_invalid_branches - reports all invalid branches" {
    cat > "$TEST_TEMP_DIR/v2_multi_invalid.yaml" << 'EOF'
name: test-v2-multi-invalid
version: 2
states:
  - name: qa
    prompt: prompts/feature/qa.md
    next:
      approved: nonexistent1
      rejected: nonexistent2
  - name: complete
    prompt: prompts/common/complete.md
entry_state: qa
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/v2_multi_invalid.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"❌ Invalid transition: qa -> nonexistent1"* ]]
    [[ "$output" == *"❌ Invalid transition: qa -> nonexistent2"* ]]
}

@test "test_v2_empty_branch_map - empty branch map cannot reach terminal" {
    cat > "$TEST_TEMP_DIR/v2_empty_branch.yaml" << 'EOF'
name: test-v2-empty-branch
version: 2
states:
  - name: qa
    prompt: prompts/feature/qa.md
    next: {}
  - name: complete
    prompt: prompts/common/complete.md
entry_state: qa
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/v2_empty_branch.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"❌ State cannot reach terminal: qa"* ]]
}

@test "test_mixed_v1_v2_next - mixed V1 string and V2 object next fields" {
    cat > "$TEST_TEMP_DIR/mixed_v1_v2.yaml" << 'EOF'
name: test-mixed-v1-v2
version: 2
states:
  - name: qa
    prompt: prompts/feature/qa.md
    next:
      approved: impl
      rejected: blocked
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
  - name: blocked
    prompt: prompts/common/blocked.md
entry_state: qa
terminal_states: [complete, blocked]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/mixed_v1_v2.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"✓ Transition valid: qa -> impl"* ]]
    [[ "$output" == *"✓ Transition valid: qa -> blocked"* ]]
    [[ "$output" == *"✓ Transition valid: impl -> complete"* ]]
}

# ==============================================================================
# get_next_states helper tests
# ==============================================================================

@test "test_get_next_states_v1_string - returns single target for V1 string next" {
    cat > "$TEST_TEMP_DIR/get_next_v1.yaml" << 'EOF'
name: test-get-next-v1
version: 1
states:
  - name: qa
    prompt: prompts/feature/qa.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: qa
terminal_states: [complete]
EOF
    # Source the script to get access to get_next_states function
    # We need to test this indirectly by checking that transitions validate correctly
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/get_next_v1.yaml"
    [[ "$status" -eq 0 ]]
    # The fact that "qa -> complete" validates proves get_next_states works for V1
    [[ "$output" == *"✓ Transition valid: qa -> complete"* ]]
}

@test "test_get_next_states_v2_map - returns all targets for V2 map next" {
    cat > "$TEST_TEMP_DIR/get_next_v2.yaml" << 'EOF'
name: test-get-next-v2
version: 2
states:
  - name: qa
    prompt: prompts/feature/qa.md
    next:
      approved: impl
      rejected: blocked
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
  - name: blocked
    prompt: prompts/common/blocked.md
entry_state: qa
terminal_states: [complete, blocked]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/get_next_v2.yaml"
    [[ "$status" -eq 0 ]]
    # Both branch targets should be validated
    [[ "$output" == *"✓ Transition valid: qa -> impl"* ]]
    [[ "$output" == *"✓ Transition valid: qa -> blocked"* ]]
}

@test "test_get_next_states_terminal_null - terminal state returns empty" {
    cat > "$TEST_TEMP_DIR/get_next_null.yaml" << 'EOF'
name: test-get-next-null
version: 1
states:
  - name: qa
    prompt: prompts/feature/qa.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: qa
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/get_next_null.yaml"
    [[ "$status" -eq 0 ]]
    # Complete state has no next, so there's no transition line for it
    # But validation should still pass
    [[ "$output" != *"Transition valid: complete ->"* ]]
}

@test "test_get_next_states_nonexistent - nonexistent state returns empty" {
    # This is tested implicitly - if a state references a nonexistent state,
    # check_next_references_valid catches it
    cat > "$TEST_TEMP_DIR/nonexistent_ref.yaml" << 'EOF'
name: test-nonexistent-ref
version: 1
states:
  - name: qa
    prompt: prompts/feature/qa.md
    next: does_not_exist
  - name: complete
    prompt: prompts/common/complete.md
entry_state: qa
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/nonexistent_ref.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"❌ Invalid transition: qa -> does_not_exist"* ]]
}

# ==============================================================================
# Terminal state with next field warning
# ==============================================================================

@test "test_terminal_with_next_warns - warns when terminal has next but does not fail" {
    cat > "$TEST_TEMP_DIR/terminal_with_next.yaml" << 'EOF'
name: test-terminal-with-next
version: 1
states:
  - name: qa
    prompt: prompts/feature/qa.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
    next: qa
entry_state: qa
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/terminal_with_next.yaml"
    # Should pass (warning, not failure)
    [[ "$status" -eq 0 ]]
    # Warning goes to stderr, check combined output
    [[ "$output" == *"WARNING: Terminal state 'complete' has next field (ignored)"* ]] || \
    [[ "${lines[*]}" == *"WARNING"*"Terminal state"*"complete"*"next field"* ]]
}

# ==============================================================================
# Error aggregation tests
# ==============================================================================

@test "test_error_aggregation_across_checks - all checks run and report all errors" {
    cat > "$TEST_TEMP_DIR/multi_error.yaml" << 'EOF'
name: test-multi-error
version: 1
states:
  - name: qa
    prompt: prompts/feature/qa.md
    next: qa
  - name: qa
    prompt: prompts/nonexistent/missing.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: qa
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/multi_error.yaml"
    [[ "$status" -eq 1 ]]
    # Should report duplicate state name
    [[ "$output" == *"❌ Duplicate state name: qa"* ]]
    # Should report missing prompt
    [[ "$output" == *"❌ Prompt not found: prompts/nonexistent/missing.md"* ]]
}

@test "error_aggregation_all_checks_run - validation runs all checks even with early failures" {
    # Create a workflow with errors in multiple validation categories
    cat > "$TEST_TEMP_DIR/all_errors.yaml" << 'EOF'
name: test-all-errors
version: 1
states:
  - name: qa
    prompt: prompts/feature/qa.md
    next: invalid_target
  - name: qa
    prompt: prompts/nonexistent/fake.md
    next: complete
  - name: orphan
    prompt: prompts/feature/orphan.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: qa
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/all_errors.yaml"
    [[ "$status" -eq 1 ]]
    # Verify multiple error categories are reported
    [[ "$output" == *"❌ Duplicate state name: qa"* ]]
    [[ "$output" == *"❌ Prompt not found"* ]]
    [[ "$output" == *"❌ Invalid transition"* ]] || [[ "$output" == *"❌ Unreachable state"* ]]
}

# ==============================================================================
# Edge case: V2 reachability through branching
# ==============================================================================

@test "v2_reachability_through_branches - states reachable via any branch are valid" {
    cat > "$TEST_TEMP_DIR/v2_branch_reach.yaml" << 'EOF'
name: test-v2-branch-reach
version: 2
states:
  - name: start
    prompt: prompts/feature/qa.md
    next:
      path_a: mid_a
      path_b: mid_b
  - name: mid_a
    prompt: prompts/feature/impl.md
    next: end
  - name: mid_b
    prompt: prompts/feature/review.md
    next: end
  - name: end
    prompt: prompts/common/complete.md
entry_state: start
terminal_states: [end]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/v2_branch_reach.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"✓ All states reachable from entry"* ]]
    [[ "$output" == *"✓ All states can reach terminal"* ]]
}

@test "v2_terminal_reachability_any_branch - state can reach terminal via any branch" {
    cat > "$TEST_TEMP_DIR/v2_any_branch_terminal.yaml" << 'EOF'
name: test-v2-any-branch-terminal
version: 2
states:
  - name: review
    prompt: prompts/feature/qa.md
    next:
      approved: complete
      needs_work: fix
  - name: fix
    prompt: prompts/feature/impl.md
    next: review
  - name: complete
    prompt: prompts/common/complete.md
entry_state: review
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/v2_any_branch_terminal.yaml"
    [[ "$status" -eq 0 ]]
    # fix can reach terminal via review->complete path
    [[ "$output" == *"✓ All states can reach terminal"* ]]
}

# ==============================================================================
# Edge case: Self-loop in terminal reachability
# ==============================================================================

@test "self_loop_still_reaches_terminal - self-loop with escape path is valid" {
    cat > "$TEST_TEMP_DIR/self_loop_escape.yaml" << 'EOF'
name: test-self-loop-escape
version: 2
states:
  - name: retry
    prompt: prompts/feature/qa.md
    next:
      retry: retry
      success: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: retry
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/self_loop_escape.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"✓ All states can reach terminal"* ]]
}

# ==============================================================================
# Integration: Validates bundled workflows still pass
# ==============================================================================

@test "bundled_feature_workflow_passes_all_checks - feature.yaml validates completely" {
    run "$SCRIPTS_DIR/validate-workflow.sh" "$WORKFLOWS_DIR/feature.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"✓ All state names unique"* ]]
    [[ "$output" == *"✓ Entry state is non-terminal"* ]]
    [[ "$output" == *"✓ All states reachable from entry"* ]]
    [[ "$output" == *"✓ All states can reach terminal"* ]]
}
