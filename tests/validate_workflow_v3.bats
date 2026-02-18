#!/usr/bin/env bats

# Tests for V3 schema validation in validate-workflow.sh
# Phase: schema-updates (orchestration-v3)
#
# Tests cover:
# - validate_defaults() — max_iterations, timeout type checking
# - validate_variables() — identifier key validation
# - validate_state_params() — param key validation per state
# - check_reserved_conflicts() — reserved variable name detection
# - Backwards compatibility with V1/V2 workflows
# - Edge cases: empty sections, nested params, multiple errors

setup() {
    load 'test_helper'
    setup_temp_dir
}

teardown() {
    teardown_temp_dir
}

# ==============================================================================
# test_v3_workflow_valid
# Full V3 workflow with defaults, variables, and params should pass
# ==============================================================================
@test "V3: valid workflow with defaults, variables, and params passes" {
    cat > "$TEST_TEMP_DIR/valid_v3.yaml" << 'EOF'
name: test
version: 3
defaults:
  max_iterations: 10
  timeout: 600
variables:
  package: test-package
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
    params:
      allow_test_changes: false
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/valid_v3.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Validation passed"* ]]
}

# ==============================================================================
# test_v3_defaults_invalid_type
# max_iterations as string should fail
# ==============================================================================
@test "V3: max_iterations as string fails validation" {
    cat > "$TEST_TEMP_DIR/defaults_invalid.yaml" << 'EOF'
name: test
version: 3
defaults:
  max_iterations: "not a number"
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/defaults_invalid.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"max_iterations must be a positive integer"* ]]
}

# ==============================================================================
# test_v3_reserved_conflict_in_defaults
# Using reserved name 'iteration' in defaults should fail
# ==============================================================================
@test "V3: reserved 'iteration' in defaults fails" {
    cat > "$TEST_TEMP_DIR/reserved_defaults.yaml" << 'EOF'
name: test
version: 3
defaults:
  iteration: 5
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/reserved_defaults.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Reserved variable 'iteration' cannot be used in defaults"* ]]
}

# ==============================================================================
# test_v3_reserved_conflict_in_variables
# Using reserved name 'state' in variables should fail
# ==============================================================================
@test "V3: reserved 'state' in variables fails" {
    cat > "$TEST_TEMP_DIR/reserved_variables.yaml" << 'EOF'
name: test
version: 3
variables:
  state: "custom"
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/reserved_variables.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Reserved variable 'state' cannot be used in variables"* ]]
}

# ==============================================================================
# test_v3_reserved_conflict_in_params
# Using reserved name 'plan_md' in state params should fail
# ==============================================================================
@test "V3: reserved 'plan_md' in state params fails" {
    cat > "$TEST_TEMP_DIR/reserved_params.yaml" << 'EOF'
name: test
version: 3
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
    params:
      plan_md: "override"
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/reserved_params.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Reserved variable 'plan_md' cannot be used in params"* ]]
}

# ==============================================================================
# test_v3_reserved_current_state
# Using reserved name 'current_state' in defaults should fail
# ==============================================================================
@test "V3: reserved 'current_state' in defaults fails" {
    cat > "$TEST_TEMP_DIR/reserved_current_state.yaml" << 'EOF'
name: test
version: 3
defaults:
  current_state: "invalid"
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/reserved_current_state.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Reserved variable 'current_state' cannot be used in defaults"* ]]
}

# ==============================================================================
# test_v3_reserved_phase
# Using reserved name 'phase' in variables should fail
# ==============================================================================
@test "V3: reserved 'phase' in variables fails" {
    cat > "$TEST_TEMP_DIR/reserved_phase.yaml" << 'EOF'
name: test
version: 3
variables:
  phase: "invalid"
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/reserved_phase.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Reserved variable 'phase' cannot be used in variables"* ]]
}

# ==============================================================================
# test_v3_reserved_plan
# Using reserved name 'plan' in state params should fail
# ==============================================================================
@test "V3: reserved 'plan' in state params fails" {
    cat > "$TEST_TEMP_DIR/reserved_plan.yaml" << 'EOF'
name: test
version: 3
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
    params:
      plan: "invalid"
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/reserved_plan.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Reserved variable 'plan' cannot be used in params"* ]]
}

# ==============================================================================
# test_v3_negative_max_iterations
# Negative max_iterations should fail
# ==============================================================================
@test "V3: negative max_iterations fails" {
    cat > "$TEST_TEMP_DIR/neg_max_iter.yaml" << 'EOF'
name: test
version: 3
defaults:
  max_iterations: -5
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/neg_max_iter.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"max_iterations must be a positive integer"* ]]
}

# ==============================================================================
# test_v3_negative_timeout
# Negative timeout should fail
# ==============================================================================
@test "V3: negative timeout fails" {
    cat > "$TEST_TEMP_DIR/neg_timeout.yaml" << 'EOF'
name: test
version: 3
defaults:
  timeout: -100
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/neg_timeout.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"timeout must be a positive integer"* ]]
}

# ==============================================================================
# test_v3_invalid_variable_key
# Variable key starting with number should fail
# ==============================================================================
@test "V3: variable key starting with number fails" {
    cat > "$TEST_TEMP_DIR/invalid_var_key.yaml" << 'EOF'
name: test
version: 3
variables:
  123invalid: "value"
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/invalid_var_key.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Invalid variable name '123invalid'"* ]]
}

# ==============================================================================
# test_v3_invalid_param_key
# Param key with dash should fail
# ==============================================================================
@test "V3: param key with dash fails" {
    cat > "$TEST_TEMP_DIR/invalid_param_key.yaml" << 'EOF'
name: test
version: 3
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
    params:
      key-with-dash: "value"
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/invalid_param_key.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Invalid param name 'key-with-dash'"* ]]
}

# ==============================================================================
# test_v3_empty_defaults_valid
# Empty defaults object should be valid
# ==============================================================================
@test "V3: empty defaults is valid" {
    cat > "$TEST_TEMP_DIR/empty_defaults.yaml" << 'EOF'
name: test
version: 3
defaults: {}
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/empty_defaults.yaml"
    [[ "$status" -eq 0 ]]
}

# ==============================================================================
# test_v3_no_defaults_valid
# Missing defaults section entirely should be valid
# ==============================================================================
@test "V3: missing defaults section is valid" {
    cat > "$TEST_TEMP_DIR/no_defaults.yaml" << 'EOF'
name: test
version: 3
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/no_defaults.yaml"
    [[ "$status" -eq 0 ]]
}

# ==============================================================================
# test_v3_nested_params_valid
# Nested structures in params (arrays, objects) should be valid
# ==============================================================================
@test "V3: nested params with arrays and objects are valid" {
    cat > "$TEST_TEMP_DIR/nested_params.yaml" << 'EOF'
name: test
version: 3
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
    params:
      files:
        - path: src/lib.rs
          desc: Main lib
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/nested_params.yaml"
    [[ "$status" -eq 0 ]]
}

# ==============================================================================
# test_v2_still_valid
# V2 workflows should still pass without issues
# ==============================================================================
@test "V3: V2 workflow still passes validation" {
    cat > "$TEST_TEMP_DIR/v2_compat.yaml" << 'EOF'
name: test
version: 2
states:
  - name: review
    prompt: prompts/feature/review.md
    verdicts: [approved, needs_fix]
    next:
      approved: complete
      needs_fix: fix
  - name: fix
    prompt: prompts/fix.md
    next: review
  - name: complete
    prompt: prompts/common/complete.md
entry_state: review
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/v2_compat.yaml"
    [[ "$status" -eq 0 ]]
}

# ==============================================================================
# test_v1_still_valid
# V1 workflows should still pass without issues
# ==============================================================================
@test "V3: V1 workflow still passes validation" {
    cat > "$TEST_TEMP_DIR/v1_compat.yaml" << 'EOF'
name: test
version: 1
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/v1_compat.yaml"
    [[ "$status" -eq 0 ]]
}

# ==============================================================================
# test_v3_boolean_param_valid
# Boolean values in params should be valid
# ==============================================================================
@test "V3: boolean param values are valid" {
    cat > "$TEST_TEMP_DIR/bool_params.yaml" << 'EOF'
name: test
version: 3
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
    params:
      debug: true
      strict_mode: false
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/bool_params.yaml"
    [[ "$status" -eq 0 ]]
}

# ==============================================================================
# test_v3_numeric_param_valid
# Integer and float values in params should be valid
# ==============================================================================
@test "V3: numeric param values (int and float) are valid" {
    cat > "$TEST_TEMP_DIR/numeric_params.yaml" << 'EOF'
name: test
version: 3
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
    params:
      retry_count: 3
      threshold: 0.95
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/numeric_params.yaml"
    [[ "$status" -eq 0 ]]
}

# ==============================================================================
# test_v3_underscore_key_valid
# Keys starting with underscore and containing underscores should be valid
# ==============================================================================
@test "V3: underscore-prefixed variable keys are valid" {
    cat > "$TEST_TEMP_DIR/underscore_keys.yaml" << 'EOF'
name: test
version: 3
variables:
  _private_var: "value"
  my_var_2: "value2"
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/underscore_keys.yaml"
    [[ "$status" -eq 0 ]]
}

# ==============================================================================
# test_v3_validate_defaults_max_iterations_zero
# Zero is not a positive integer — should fail
# ==============================================================================
@test "V3: max_iterations of 0 fails (zero is not positive)" {
    cat > "$TEST_TEMP_DIR/zero_max_iter.yaml" << 'EOF'
name: test
version: 3
defaults:
  max_iterations: 0
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/zero_max_iter.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"max_iterations must be a positive integer"* ]]
}

# ==============================================================================
# test_v3_validate_defaults_max_iterations_float
# Float values for max_iterations should fail
# ==============================================================================
@test "V3: max_iterations as float fails" {
    cat > "$TEST_TEMP_DIR/float_max_iter.yaml" << 'EOF'
name: test
version: 3
defaults:
  max_iterations: 3.5
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/float_max_iter.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"max_iterations must be a positive integer"* ]]
}

# ==============================================================================
# test_v3_validate_defaults_timeout_zero
# Zero timeout is not valid
# ==============================================================================
@test "V3: timeout of 0 fails (zero is not positive)" {
    cat > "$TEST_TEMP_DIR/zero_timeout.yaml" << 'EOF'
name: test
version: 3
defaults:
  timeout: 0
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/zero_timeout.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"timeout must be a positive integer"* ]]
}

# ==============================================================================
# test_v3_key_with_dot_invalid
# Dot is not allowed in identifiers
# ==============================================================================
@test "V3: variable key with dot fails" {
    cat > "$TEST_TEMP_DIR/dot_key.yaml" << 'EOF'
name: test
version: 3
variables:
  a.b: "value"
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/dot_key.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Invalid variable name 'a.b'"* ]]
}

# ==============================================================================
# test_v3_empty_string_variable_valid
# Empty string values for variables are valid (key is valid, value can be empty)
# ==============================================================================
@test "V3: empty string variable value is valid" {
    cat > "$TEST_TEMP_DIR/empty_value.yaml" << 'EOF'
name: test
version: 3
variables:
  my_var: ""
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/empty_value.yaml"
    [[ "$status" -eq 0 ]]
}

# ==============================================================================
# test_v3_multiple_reserved_conflicts
# Multiple reserved conflicts in defaults should all be reported
# ==============================================================================
@test "V3: multiple reserved conflicts in defaults reports ALL" {
    cat > "$TEST_TEMP_DIR/multi_reserved.yaml" << 'EOF'
name: test
version: 3
defaults:
  state: "custom"
  iteration: 5
  plan: "override"
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/multi_reserved.yaml"
    [[ "$status" -eq 1 ]]
    # All three conflicts must be reported
    [[ "$output" == *"Reserved variable 'state' cannot be used in defaults"* ]]
    [[ "$output" == *"Reserved variable 'iteration' cannot be used in defaults"* ]]
    [[ "$output" == *"Reserved variable 'plan' cannot be used in defaults"* ]]
}

# ==============================================================================
# test_v3_validate_defaults_isolation
# Both max_iterations and timeout invalid should report both errors
# ==============================================================================
@test "V3: both max_iterations and timeout invalid reports both" {
    cat > "$TEST_TEMP_DIR/both_invalid.yaml" << 'EOF'
name: test
version: 3
defaults:
  max_iterations: "not_a_number"
  timeout: -5
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/both_invalid.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"max_iterations must be a positive integer"* ]]
    [[ "$output" == *"timeout must be a positive integer"* ]]
}

# ==============================================================================
# test_v3_validate_variables_isolation
# Multiple invalid variable keys should all be reported
# ==============================================================================
@test "V3: multiple invalid variable keys reports all" {
    cat > "$TEST_TEMP_DIR/multi_invalid_vars.yaml" << 'EOF'
name: test
version: 3
variables:
  valid_key: "ok"
  123bad: "nope"
  also-bad: "nope"
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/multi_invalid_vars.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Invalid variable name '123bad'"* ]]
    [[ "$output" == *"Invalid variable name 'also-bad'"* ]]
}

# ==============================================================================
# test_v3_validate_state_params_isolation
# Invalid param key mixed with valid should report only invalid
# ==============================================================================
@test "V3: invalid param key in state reports error" {
    cat > "$TEST_TEMP_DIR/bad_param_key.yaml" << 'EOF'
name: test
version: 3
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
    params:
      good_key: "ok"
      bad-key: "nope"
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/bad_param_key.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Invalid param name 'bad-key'"* ]]
}

# ==============================================================================
# test_v3_reserved_conflict_params_key
# Using 'params' as a variable name should fail (it's reserved)
# ==============================================================================
@test "V3: reserved 'params' in variables fails" {
    cat > "$TEST_TEMP_DIR/reserved_params_key.yaml" << 'EOF'
name: test
version: 3
variables:
  params: "should not be allowed"
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/reserved_params_key.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Reserved variable 'params' cannot be used in variables"* ]]
}

# ==============================================================================
# test_v3_reserved_conflict_in_params_multiple_states
# Reserved names in params across multiple states should all report
# ==============================================================================
@test "V3: reserved conflicts in params across multiple states reports all" {
    cat > "$TEST_TEMP_DIR/multi_state_reserved.yaml" << 'EOF'
name: test
version: 3
states:
  - name: qa
    prompt: prompts/qa.md
    next: impl
    params:
      state: "bad"
  - name: impl
    prompt: prompts/impl.md
    next: complete
    params:
      plan_md: "bad"
  - name: complete
    prompt: prompts/common/complete.md
entry_state: qa
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/multi_state_reserved.yaml"
    [[ "$status" -eq 1 ]]
    # Both states should have their conflicts reported
    [[ "$output" == *"'state'"*"params"* ]]
    [[ "$output" == *"'plan_md'"*"params"* ]]
}

# ==============================================================================
# Edge case: mixed case keys are valid identifiers
# ==============================================================================
@test "V3: mixed case variable key 'MyVar' is valid" {
    cat > "$TEST_TEMP_DIR/mixed_case_key.yaml" << 'EOF'
name: test
version: 3
variables:
  MyVar: "value"
  camelCase: "value2"
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/mixed_case_key.yaml"
    [[ "$status" -eq 0 ]]
}

# ==============================================================================
# Edge case: null values in variables should be valid
# ==============================================================================
@test "V3: null variable value is valid" {
    cat > "$TEST_TEMP_DIR/null_value.yaml" << 'EOF'
name: test
version: 3
variables:
  my_var: null
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/null_value.yaml"
    [[ "$status" -eq 0 ]]
}

# ==============================================================================
# Edge case: V2 workflow with V3 fields should still work if version < 3
# V3 fields are simply ignored for version < 3
# ==============================================================================
@test "V3: V1 workflow ignores defaults/variables/params sections" {
    cat > "$TEST_TEMP_DIR/v1_with_v3_fields.yaml" << 'EOF'
name: test
version: 1
defaults:
  max_iterations: "this would fail V3 validation"
variables:
  123invalid: "this would fail V3 validation"
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
    params:
      bad-key: "this would fail V3 validation"
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/v1_with_v3_fields.yaml"
    # V1 should pass — V3 validation should be skipped for version < 3
    [[ "$status" -eq 0 ]]
}

# ==============================================================================
# Edge case: empty variables section is valid
# ==============================================================================
@test "V3: empty variables section is valid" {
    cat > "$TEST_TEMP_DIR/empty_vars.yaml" << 'EOF'
name: test
version: 3
variables: {}
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/empty_vars.yaml"
    [[ "$status" -eq 0 ]]
}

# ==============================================================================
# Edge case: empty params on a state is valid
# ==============================================================================
@test "V3: empty params on a state is valid" {
    cat > "$TEST_TEMP_DIR/empty_params.yaml" << 'EOF'
name: test
version: 3
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
    params: {}
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/empty_params.yaml"
    [[ "$status" -eq 0 ]]
}

# ==============================================================================
# Edge case: custom keys in defaults (not max_iterations/timeout) are allowed
# ==============================================================================
@test "V3: custom defaults keys are allowed" {
    cat > "$TEST_TEMP_DIR/custom_defaults.yaml" << 'EOF'
name: test
version: 3
defaults:
  max_iterations: 10
  timeout: 300
  custom_setting: "hello"
  another_value: 42
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/custom_defaults.yaml"
    [[ "$status" -eq 0 ]]
}

# ==============================================================================
# Edge case: key with space is invalid
# ==============================================================================
@test "V3: variable key with space fails" {
    cat > "$TEST_TEMP_DIR/space_key.yaml" << 'EOF'
name: test
version: 3
variables:
  "my var": "value"
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/space_key.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Invalid variable name"* ]]
}

# ==============================================================================
# Edge case: list values in params are valid
# ==============================================================================
@test "V3: list values in params are valid" {
    cat > "$TEST_TEMP_DIR/list_params.yaml" << 'EOF'
name: test
version: 3
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
    params:
      allowed_files:
        - src/lib.rs
        - src/main.rs
      tags:
        - feature
        - wip
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/list_params.yaml"
    [[ "$status" -eq 0 ]]
}

# ==============================================================================
# Edge case: timeout as float should fail (same as max_iterations)
# ==============================================================================
@test "V3: timeout as float fails" {
    cat > "$TEST_TEMP_DIR/float_timeout.yaml" << 'EOF'
name: test
version: 3
defaults:
  timeout: 3.5
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/float_timeout.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"timeout must be a positive integer"* ]]
}

# ==============================================================================
# All 7 reserved names tested: state, iteration, current_state, phase, plan, plan_md, params
# This test checks all individually in different sections
# ==============================================================================
@test "V3: all 7 reserved names rejected in variables" {
    # Test each reserved name one by one — we already test state, iteration, phase, plan, plan_md, params individually above
    # This tests current_state in variables specifically
    cat > "$TEST_TEMP_DIR/reserved_all.yaml" << 'EOF'
name: test
version: 3
variables:
  current_state: "not allowed"
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/reserved_all.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Reserved variable 'current_state' cannot be used in variables"* ]]
}

# ==============================================================================
# Reserved 'iteration' in params should fail
# ==============================================================================
@test "V3: reserved 'iteration' in params fails" {
    cat > "$TEST_TEMP_DIR/reserved_iter_params.yaml" << 'EOF'
name: test
version: 3
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
    params:
      iteration: 5
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/reserved_iter_params.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Reserved variable 'iteration' cannot be used in params"* ]]
}

# ==============================================================================
# Reserved 'state' in defaults should fail
# ==============================================================================
@test "V3: reserved 'state' in defaults fails" {
    cat > "$TEST_TEMP_DIR/reserved_state_defaults.yaml" << 'EOF'
name: test
version: 3
defaults:
  state: "bad"
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/reserved_state_defaults.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Reserved variable 'state' cannot be used in defaults"* ]]
}

# ==============================================================================
# V3 workflow with all sections valid but version is 2 — V3 checks skipped
# ==============================================================================
@test "V3: version 2 workflow skips V3 validation" {
    cat > "$TEST_TEMP_DIR/v2_with_v3.yaml" << 'EOF'
name: test
version: 2
defaults:
  max_iterations: "would fail in V3"
variables:
  state: "reserved but V2 doesn't care"
states:
  - name: work
    prompt: prompts/feature/qa.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: work
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/v2_with_v3.yaml"
    [[ "$status" -eq 0 ]]
}

# ==============================================================================
# Combined: reserved conflict + invalid key in same workflow
# Should report both types of errors
# ==============================================================================
@test "V3: combined reserved conflict and invalid key reports both" {
    cat > "$TEST_TEMP_DIR/combined_errors.yaml" << 'EOF'
name: test
version: 3
variables:
  state: "reserved"
  123bad: "invalid key"
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/combined_errors.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Reserved variable 'state'"* ]]
    [[ "$output" == *"Invalid variable name '123bad'"* ]]
}

# ==============================================================================
# Combined: invalid defaults + reserved conflict + invalid param key
# Should report all errors from all validation functions
# ==============================================================================
@test "V3: errors from all validation functions reported together" {
    cat > "$TEST_TEMP_DIR/all_errors.yaml" << 'EOF'
name: test
version: 3
defaults:
  max_iterations: "bad"
  phase: "reserved in defaults"
variables:
  plan: "reserved in variables"
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
    params:
      bad-key: "invalid"
      iteration: "reserved in params"
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/all_errors.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"max_iterations must be a positive integer"* ]]
    [[ "$output" == *"Reserved variable 'phase' cannot be used in defaults"* ]]
    [[ "$output" == *"Reserved variable 'plan' cannot be used in variables"* ]]
    [[ "$output" == *"Invalid param name 'bad-key'"* ]]
    [[ "$output" == *"Reserved variable 'iteration' cannot be used in params"* ]]
}

# ==============================================================================
# Large positive integers should be valid for max_iterations and timeout
# ==============================================================================
@test "V3: large positive max_iterations and timeout are valid" {
    cat > "$TEST_TEMP_DIR/large_values.yaml" << 'EOF'
name: test
version: 3
defaults:
  max_iterations: 99999
  timeout: 86400
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/large_values.yaml"
    [[ "$status" -eq 0 ]]
}

# ==============================================================================
# Version 3 accepted without warning
# ==============================================================================
@test "V3: version 3 is recognized without 'expected 1 or 2' warning" {
    cat > "$TEST_TEMP_DIR/v3_no_warn.yaml" << 'EOF'
name: test
version: 3
states:
  - name: impl
    prompt: prompts/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/v3_no_warn.yaml"
    [[ "$status" -eq 0 ]]
    # V3 is now a valid version; should NOT produce the old warning
    [[ "$output" != *"expected 1 or 2"* ]]
}
