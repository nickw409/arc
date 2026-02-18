#!/usr/bin/env bats

# Tests for V5 schema validation in validate-workflow.sh
# Phase: schema-updates (orchestration-v5)
#
# Tests cover:
# - validate_v5_parallel() — strategy, branches, branch fields, uniqueness, n_of_m, prompt files
# - validate_v5_parallel_verdicts() — verdict-strategy consistency
# - Version 5 acceptance without warning
# - Prompt requirement bypass for parallel states (MODIFICATION 2)
# - Backwards compatibility with V1-V4 workflows
# - Version gating (parallel blocks ignored for version < 5)

setup() {
    load 'test_helper'
    setup_temp_dir
    # Create prompt files that tests reference
    mkdir -p "$ORCH_DIR/prompts/v5"
    touch "$ORCH_DIR/prompts/v5/branch_a.md"
    touch "$ORCH_DIR/prompts/v5/branch_b.md"
    touch "$ORCH_DIR/prompts/v5/branch_c.md"
    touch "$ORCH_DIR/prompts/v5/branch_d.md"
}

teardown() {
    teardown_temp_dir
    # Clean up test prompt files
    rm -rf "$ORCH_DIR/prompts/v5"
}

# ==============================================================================
# Helper: Create a minimal valid V5 workflow with a parallel state
# Usage: create_v5_parallel_workflow <parallel_yaml> [extra_states_yaml] [terminal_states]
# parallel_yaml: the parallel block content (indented under the state)
# extra_states_yaml: additional states to append (optional)
# terminal_states: override terminal states list (default: [complete])
# ==============================================================================
create_v5_parallel_workflow() {
    local parallel_yaml="$1"
    local extra_states_yaml="${2:-}"
    local terminal_states="${3:-[complete]}"
    local output_file="$TEST_TEMP_DIR/workflow.yaml"

    cat > "$output_file" << YAML
name: test_v5
version: 5
states:
  - name: parallel_review
${parallel_yaml}
    next: complete
${extra_states_yaml}
  - name: complete
    prompt: prompts/common/complete.md
entry_state: parallel_review
terminal_states: ${terminal_states}
YAML
    echo "$output_file"
}

# ==============================================================================
# Helper: Create a V5 workflow with verdicts on the parallel state
# Usage: create_v5_verdict_workflow <strategy> <branches_yaml> <verdicts_yaml> <next_yaml>
# ==============================================================================
create_v5_verdict_workflow() {
    local strategy="$1"
    local branches_yaml="$2"
    local verdicts_yaml="$3"
    local next_yaml="$4"
    local n_field="${5:-}"
    local output_file="$TEST_TEMP_DIR/workflow.yaml"

    cat > "$output_file" << YAML
name: test_v5_verdicts
version: 5
states:
  - name: parallel_review
    parallel:
      strategy: ${strategy}
${n_field}
      branches:
${branches_yaml}
    verdicts: ${verdicts_yaml}
    next:
${next_yaml}
  - name: success
    prompt: prompts/common/complete.md
  - name: failure
    prompt: prompts/common/blocked.md
  - name: complete
    prompt: prompts/common/complete.md
entry_state: parallel_review
terminal_states: [success, failure, complete]
YAML
    echo "$output_file"
}

# ==============================================================================
# TEST: test_valid_v5_parallel_all
# V5 workflow, parallel state, strategy "all", 2 branches with name+prompt
# ==============================================================================
@test "V5: valid parallel state with strategy 'all' and 2 branches passes" {
    local wf
    wf=$(create_v5_parallel_workflow '    parallel:
      strategy: all
      branches:
        - name: branch_a
          prompt: prompts/v5/branch_a.md
        - name: branch_b
          prompt: prompts/v5/branch_b.md')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Validation passed"* ]]
}

# ==============================================================================
# TEST: test_valid_v5_parallel_any
# Strategy "any", 3 branches
# ==============================================================================
@test "V5: valid parallel state with strategy 'any' and 3 branches passes" {
    local wf
    wf=$(create_v5_parallel_workflow '    parallel:
      strategy: any
      branches:
        - name: branch_a
          prompt: prompts/v5/branch_a.md
        - name: branch_b
          prompt: prompts/v5/branch_b.md
        - name: branch_c
          prompt: prompts/v5/branch_c.md')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Validation passed"* ]]
}

# ==============================================================================
# TEST: test_valid_v5_parallel_n_of_m
# Strategy "n_of_m", n=2, 4 branches
# ==============================================================================
@test "V5: valid parallel state with strategy 'n_of_m', n=2, 4 branches passes" {
    local wf
    wf=$(create_v5_parallel_workflow '    parallel:
      strategy: n_of_m
      n: 2
      branches:
        - name: branch_a
          prompt: prompts/v5/branch_a.md
        - name: branch_b
          prompt: prompts/v5/branch_b.md
        - name: branch_c
          prompt: prompts/v5/branch_c.md
        - name: branch_d
          prompt: prompts/v5/branch_d.md')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Validation passed"* ]]
}

# ==============================================================================
# TEST: test_invalid_strategy
# Strategy "first" — not a valid strategy
# ==============================================================================
@test "V5: invalid parallel strategy 'first' fails" {
    local wf
    wf=$(create_v5_parallel_workflow '    parallel:
      strategy: first
      branches:
        - name: branch_a
          prompt: prompts/v5/branch_a.md')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"invalid parallel strategy"* ]]
}

# ==============================================================================
# TEST: test_missing_branches
# Parallel block without branches array
# ==============================================================================
@test "V5: parallel block without branches array fails" {
    local wf
    wf=$(create_v5_parallel_workflow '    parallel:
      strategy: all')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"branches"* ]]
}

# ==============================================================================
# TEST: test_empty_branches
# branches: []
# ==============================================================================
@test "V5: parallel block with empty branches array fails" {
    local wf
    wf=$(create_v5_parallel_workflow '    parallel:
      strategy: all
      branches: []')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"empty"* ]]
}

# ==============================================================================
# TEST: test_branch_missing_name
# Branch without "name"
# ==============================================================================
@test "V5: branch without name field fails" {
    local wf
    wf=$(create_v5_parallel_workflow '    parallel:
      strategy: all
      branches:
        - prompt: prompts/v5/branch_a.md')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"name"* ]]
}

# ==============================================================================
# TEST: test_branch_missing_prompt
# Branch without "prompt"
# ==============================================================================
@test "V5: branch without prompt field fails" {
    local wf
    wf=$(create_v5_parallel_workflow '    parallel:
      strategy: all
      branches:
        - name: branch_a')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"prompt"* ]]
}

# ==============================================================================
# TEST: test_duplicate_branch_names
# Two branches both named "worker"
# ==============================================================================
@test "V5: duplicate branch names fails" {
    local wf
    wf=$(create_v5_parallel_workflow '    parallel:
      strategy: all
      branches:
        - name: worker
          prompt: prompts/v5/branch_a.md
        - name: worker
          prompt: prompts/v5/branch_b.md')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"duplicate"* ]]
}

# ==============================================================================
# TEST: test_n_of_m_missing_n
# Strategy "n_of_m", no "n" field
# ==============================================================================
@test "V5: n_of_m strategy without n field fails" {
    local wf
    wf=$(create_v5_parallel_workflow '    parallel:
      strategy: n_of_m
      branches:
        - name: branch_a
          prompt: prompts/v5/branch_a.md
        - name: branch_b
          prompt: prompts/v5/branch_b.md')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"n required"* ]]
}

# ==============================================================================
# TEST: test_n_of_m_invalid_n_zero
# Strategy "n_of_m", n=0
# ==============================================================================
@test "V5: n_of_m strategy with n=0 fails" {
    local wf
    wf=$(create_v5_parallel_workflow '    parallel:
      strategy: n_of_m
      n: 0
      branches:
        - name: branch_a
          prompt: prompts/v5/branch_a.md
        - name: branch_b
          prompt: prompts/v5/branch_b.md')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"positive"* ]]
}

# ==============================================================================
# TEST: test_n_of_m_invalid_n_negative
# Strategy "n_of_m", n=-1
# ==============================================================================
@test "V5: n_of_m strategy with n=-1 fails" {
    local wf
    wf=$(create_v5_parallel_workflow '    parallel:
      strategy: n_of_m
      n: -1
      branches:
        - name: branch_a
          prompt: prompts/v5/branch_a.md
        - name: branch_b
          prompt: prompts/v5/branch_b.md')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"positive"* ]]
}

# ==============================================================================
# TEST: test_n_of_m_invalid_n_string
# Strategy "n_of_m", n="two"
# ==============================================================================
@test "V5: n_of_m strategy with n='two' (string) fails" {
    local wf
    wf=$(create_v5_parallel_workflow '    parallel:
      strategy: n_of_m
      n: "two"
      branches:
        - name: branch_a
          prompt: prompts/v5/branch_a.md
        - name: branch_b
          prompt: prompts/v5/branch_b.md')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"positive"* ]]
}

# ==============================================================================
# TEST: test_n_of_m_invalid_n_float
# Strategy "n_of_m", n=1.5
# ==============================================================================
@test "V5: n_of_m strategy with n=1.5 (float) fails" {
    local wf
    wf=$(create_v5_parallel_workflow '    parallel:
      strategy: n_of_m
      n: 1.5
      branches:
        - name: branch_a
          prompt: prompts/v5/branch_a.md
        - name: branch_b
          prompt: prompts/v5/branch_b.md')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"positive"* ]]
}

# ==============================================================================
# TEST: test_branch_prompt_not_found
# Branch prompt path that doesn't exist (relative to ORCH_DIR)
# ==============================================================================
@test "V5: branch prompt file not found fails" {
    local wf
    wf=$(create_v5_parallel_workflow '    parallel:
      strategy: all
      branches:
        - name: branch_a
          prompt: prompts/v5/nonexistent.md
        - name: branch_b
          prompt: prompts/v5/branch_b.md')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"not found"* ]]
}

# ==============================================================================
# TEST: test_parallel_state_no_prompt_ok
# State with parallel but no state-level "prompt"
# ==============================================================================
@test "V5: parallel state without state-level prompt passes" {
    local wf
    wf=$(create_v5_parallel_workflow '    parallel:
      strategy: all
      branches:
        - name: branch_a
          prompt: prompts/v5/branch_a.md
        - name: branch_b
          prompt: prompts/v5/branch_b.md')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 0 ]]
    # Ensure no error about missing prompt for the parallel state
    [[ "$output" != *"missing required field: prompt"* ]]
}

# ==============================================================================
# TEST: test_parallel_state_with_prompt_ok
# State with both "prompt" and "parallel" fields
# ==============================================================================
@test "V5: parallel state with both state-level prompt and parallel block passes" {
    local wf
    wf=$(create_v5_parallel_workflow '    prompt: prompts/feature/impl.md
    parallel:
      strategy: all
      branches:
        - name: branch_a
          prompt: prompts/v5/branch_a.md
        - name: branch_b
          prompt: prompts/v5/branch_b.md')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 0 ]]
}

# ==============================================================================
# TEST: test_parallel_terminal_state_rejected
# State with parallel block that is also listed in terminal_states
# ==============================================================================
@test "V5: parallel state listed as terminal state fails" {
    local wf
    wf=$(create_v5_parallel_workflow '    parallel:
      strategy: all
      branches:
        - name: branch_a
          prompt: prompts/v5/branch_a.md
        - name: branch_b
          prompt: prompts/v5/branch_b.md' '' '[parallel_review, complete]')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"terminal"* ]]
}

# ==============================================================================
# VERDICT TESTS
# ==============================================================================

# TEST: test_verdict_mismatch_all
# Strategy "all", verdicts ["n_complete", "insufficient"]
@test "V5: strategy 'all' with wrong verdicts fails" {
    local wf
    wf=$(create_v5_verdict_workflow "all" \
        '        - name: branch_a
          prompt: prompts/v5/branch_a.md
        - name: branch_b
          prompt: prompts/v5/branch_b.md' \
        '[n_complete, insufficient]' \
        '      n_complete: success
      insufficient: failure')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"invalid verdict"* ]]
}

# TEST: test_verdict_valid_all
# Strategy "all", verdicts ["all_complete", "any_failed"]
@test "V5: strategy 'all' with correct verdicts passes" {
    local wf
    wf=$(create_v5_verdict_workflow "all" \
        '        - name: branch_a
          prompt: prompts/v5/branch_a.md
        - name: branch_b
          prompt: prompts/v5/branch_b.md' \
        '[all_complete, any_failed]' \
        '      all_complete: success
      any_failed: failure')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 0 ]]
}

# TEST: test_verdict_mismatch_any
# Strategy "any", verdicts ["all_complete", "any_failed"]
@test "V5: strategy 'any' with wrong verdicts fails" {
    local wf
    wf=$(create_v5_verdict_workflow "any" \
        '        - name: branch_a
          prompt: prompts/v5/branch_a.md
        - name: branch_b
          prompt: prompts/v5/branch_b.md' \
        '[all_complete, any_failed]' \
        '      all_complete: success
      any_failed: failure')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"invalid verdict"* ]]
}

# TEST: test_verdict_valid_any
# Strategy "any", verdicts ["first_complete", "all_failed"]
@test "V5: strategy 'any' with correct verdicts passes" {
    local wf
    wf=$(create_v5_verdict_workflow "any" \
        '        - name: branch_a
          prompt: prompts/v5/branch_a.md
        - name: branch_b
          prompt: prompts/v5/branch_b.md' \
        '[first_complete, all_failed]' \
        '      first_complete: success
      all_failed: failure')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 0 ]]
}

# TEST: test_verdict_mismatch_n_of_m
# Strategy "n_of_m", verdicts ["all_complete", "any_failed"]
@test "V5: strategy 'n_of_m' with wrong verdicts fails" {
    local wf
    wf=$(create_v5_verdict_workflow "n_of_m" \
        '        - name: branch_a
          prompt: prompts/v5/branch_a.md
        - name: branch_b
          prompt: prompts/v5/branch_b.md' \
        '[all_complete, any_failed]' \
        '      all_complete: success
      any_failed: failure' \
        '      n: 1')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"invalid verdict"* ]]
}

# TEST: test_verdict_valid_n_of_m
# Strategy "n_of_m", verdicts ["n_complete", "insufficient"]
@test "V5: strategy 'n_of_m' with correct verdicts passes" {
    local wf
    wf=$(create_v5_verdict_workflow "n_of_m" \
        '        - name: branch_a
          prompt: prompts/v5/branch_a.md
        - name: branch_b
          prompt: prompts/v5/branch_b.md' \
        '[n_complete, insufficient]' \
        '      n_complete: success
      insufficient: failure' \
        '      n: 1')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 0 ]]
}

# TEST: test_verdict_subset_ok
# Strategy "all", verdicts ["all_complete"] (only one of two valid verdicts)
@test "V5: strategy 'all' with subset of valid verdicts passes" {
    local wf
    wf=$(create_v5_verdict_workflow "all" \
        '        - name: branch_a
          prompt: prompts/v5/branch_a.md
        - name: branch_b
          prompt: prompts/v5/branch_b.md' \
        '[all_complete]' \
        '      all_complete: success')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 0 ]]
}

# TEST: test_verdict_skipped_for_linear_next
# Parallel state with `next: some_state` (string, no verdicts array)
@test "V5: parallel state with linear next (no verdicts) passes" {
    local wf
    wf=$(create_v5_parallel_workflow '    parallel:
      strategy: all
      branches:
        - name: branch_a
          prompt: prompts/v5/branch_a.md
        - name: branch_b
          prompt: prompts/v5/branch_b.md')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 0 ]]
}

# ==============================================================================
# BACKWARDS COMPATIBILITY AND VERSION GATING TESTS
# ==============================================================================

# TEST: test_v4_workflow_unchanged
# Valid V4 workflow (no parallel) still passes
@test "V5: valid V4 workflow without parallel passes unchanged" {
    local output_file="$TEST_TEMP_DIR/workflow.yaml"
    cat > "$output_file" << 'YAML'
name: test_v4
version: 4
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
    constraints:
      max_iterations: 5
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML
    run "$SCRIPTS_DIR/validate-workflow.sh" "$output_file"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Validation passed"* ]]
}

# TEST: test_version_gating
# Version 4 workflow with parallel block present — block ignored
@test "V5: version 4 workflow ignores parallel block (version gating)" {
    local output_file="$TEST_TEMP_DIR/workflow.yaml"
    cat > "$output_file" << 'YAML'
name: test_v4_with_parallel
version: 4
states:
  - name: parallel_review
    prompt: prompts/feature/impl.md
    parallel:
      strategy: invalid_should_be_ignored
      branches: []
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: parallel_review
terminal_states: [complete]
YAML
    run "$SCRIPTS_DIR/validate-workflow.sh" "$output_file"
    [[ "$status" -eq 0 ]]
}

# TEST: test_v5_version_accepted
# V5 workflow with version: 5 — no version warning
@test "V5: version 5 is accepted without version warning" {
    local wf
    wf=$(create_v5_parallel_workflow '    parallel:
      strategy: all
      branches:
        - name: branch_a
          prompt: prompts/v5/branch_a.md
        - name: branch_b
          prompt: prompts/v5/branch_b.md')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 0 ]]
    [[ "$output" != *"expected 1"* ]]
    [[ "$output" != *"expected 1, 2"* ]]
    [[ "$output" != *"expected 1, 2, 3"* ]]
    [[ "$output" != *"expected 1, 2, 3, or 4"* ]]
}

# ==============================================================================
# MULTIPLE PARALLEL STATES TEST
# ==============================================================================

# TEST: test_multiple_parallel_states
# V5 workflow with 2 different parallel states — both validated independently
@test "V5: workflow with two parallel states passes" {
    local output_file="$TEST_TEMP_DIR/workflow.yaml"
    cat > "$output_file" << YAML
name: test_multi_parallel
version: 5
states:
  - name: first_parallel
    parallel:
      strategy: all
      branches:
        - name: branch_a
          prompt: prompts/v5/branch_a.md
        - name: branch_b
          prompt: prompts/v5/branch_b.md
    next: second_parallel
  - name: second_parallel
    parallel:
      strategy: any
      branches:
        - name: branch_c
          prompt: prompts/v5/branch_c.md
        - name: branch_d
          prompt: prompts/v5/branch_d.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: first_parallel
terminal_states: [complete]
YAML
    run "$SCRIPTS_DIR/validate-workflow.sh" "$output_file"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Validation passed"* ]]
}

# ==============================================================================
# BRANCH PARAMS OPTIONAL TEST
# ==============================================================================

# TEST: test_branch_params_optional
# Branch with name and prompt but no params field — valid
@test "V5: branch without params field passes" {
    local wf
    wf=$(create_v5_parallel_workflow '    parallel:
      strategy: all
      branches:
        - name: branch_a
          prompt: prompts/v5/branch_a.md
        - name: branch_b
          prompt: prompts/v5/branch_b.md')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 0 ]]
}

# ==============================================================================
# BRANCH EMPTY NAME TEST
# ==============================================================================

# TEST: test_branch_empty_name
# Branch with `name: ""` (empty string)
@test "V5: branch with empty string name fails" {
    local wf
    wf=$(create_v5_parallel_workflow '    parallel:
      strategy: all
      branches:
        - name: ""
          prompt: prompts/v5/branch_a.md
        - name: branch_b
          prompt: prompts/v5/branch_b.md')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"name"* ]]
}

# ==============================================================================
# EDGE CASE: n_of_m where n > branch count
# Plan edge case #4: "valid at schema level (runtime handles)"
# ==============================================================================

# TEST: test_n_of_m_n_exceeds_branch_count
# Strategy "n_of_m", n=5, but only 2 branches — valid at schema level
@test "V5: n_of_m with n greater than branch count passes schema validation" {
    local wf
    wf=$(create_v5_parallel_workflow '    parallel:
      strategy: n_of_m
      n: 5
      branches:
        - name: branch_a
          prompt: prompts/v5/branch_a.md
        - name: branch_b
          prompt: prompts/v5/branch_b.md')
    run "$SCRIPTS_DIR/validate-workflow.sh" "$wf"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Validation passed"* ]]
}
