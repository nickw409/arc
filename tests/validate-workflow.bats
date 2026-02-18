#!/usr/bin/env bats

# Tests for validate-workflow.sh

setup() {
    load 'test_helper'
    setup_temp_dir
}

teardown() {
    teardown_temp_dir
}

@test "validate-workflow.sh exists and is executable" {
    [[ -x "$SCRIPTS_DIR/validate-workflow.sh" ]]
}

@test "script is syntactically valid bash" {
    run bash -n "$SCRIPTS_DIR/validate-workflow.sh"
    [[ "$status" -eq 0 ]]
}

@test "fails with no arguments" {
    run "$SCRIPTS_DIR/validate-workflow.sh"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Usage"* ]]
}

@test "fails when workflow file does not exist" {
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/nonexistent.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"not found"* ]]
}

@test "passes for valid workflow" {
    local workflow=$(create_test_workflow)
    run "$SCRIPTS_DIR/validate-workflow.sh" "$workflow"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Validation passed"* ]]
}

@test "validates the bundled feature.yaml workflow" {
    run "$SCRIPTS_DIR/validate-workflow.sh" "$WORKFLOWS_DIR/feature.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Validation passed"* ]]
}

@test "detects missing name field" {
    cat > "$TEST_TEMP_DIR/no_name.yaml" << 'EOF'
version: 1
entry_state: start
terminal_states: [end]
states:
  - name: start
    prompt: test.md
    next: end
  - name: end
    prompt: test.md
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/no_name.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"missing required field: name"* ]]
}

@test "detects missing version field" {
    cat > "$TEST_TEMP_DIR/no_version.yaml" << 'EOF'
name: test
entry_state: start
terminal_states: [end]
states:
  - name: start
    prompt: test.md
    next: end
  - name: end
    prompt: test.md
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/no_version.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"missing required field: version"* ]]
}

@test "detects missing entry_state field" {
    cat > "$TEST_TEMP_DIR/no_entry.yaml" << 'EOF'
name: test
version: 1
terminal_states: [end]
states:
  - name: start
    prompt: test.md
    next: end
  - name: end
    prompt: test.md
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/no_entry.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"missing required field: entry_state"* ]]
}

@test "detects missing terminal_states" {
    cat > "$TEST_TEMP_DIR/no_terminal.yaml" << 'EOF'
name: test
version: 1
entry_state: start
terminal_states: []
states:
  - name: start
    prompt: test.md
    next: end
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/no_terminal.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"at least one terminal_state"* ]]
}

@test "detects missing states array" {
    cat > "$TEST_TEMP_DIR/no_states.yaml" << 'EOF'
name: test
version: 1
entry_state: start
terminal_states: [end]
states: []
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/no_states.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"at least one state"* ]]
}

@test "detects state missing name" {
    cat > "$TEST_TEMP_DIR/state_no_name.yaml" << 'EOF'
name: test
version: 1
entry_state: start
terminal_states: [end]
states:
  - prompt: test.md
    next: end
  - name: end
    prompt: test.md
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/state_no_name.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"missing required field: name"* ]]
}

@test "detects state missing prompt" {
    cat > "$TEST_TEMP_DIR/state_no_prompt.yaml" << 'EOF'
name: test
version: 1
entry_state: start
terminal_states: [end]
states:
  - name: start
    next: end
  - name: end
    prompt: test.md
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/state_no_prompt.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"missing required field: prompt"* ]]
}

@test "detects non-terminal state missing next" {
    cat > "$TEST_TEMP_DIR/state_no_next.yaml" << 'EOF'
name: test
version: 1
entry_state: start
terminal_states: [end]
states:
  - name: start
    prompt: test.md
  - name: end
    prompt: test.md
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/state_no_next.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"missing required field: next"* ]]
}

@test "allows terminal state without next field" {
    cat > "$TEST_TEMP_DIR/terminal_no_next.yaml" << 'EOF'
name: test
version: 1
entry_state: start
terminal_states: [end]
states:
  - name: start
    prompt: prompts/feature/qa.md
    next: end
  - name: end
    prompt: prompts/common/complete.md
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/terminal_no_next.yaml"
    [[ "$status" -eq 0 ]]
}

@test "detects entry_state not in states" {
    cat > "$TEST_TEMP_DIR/bad_entry.yaml" << 'EOF'
name: test
version: 1
entry_state: nonexistent
terminal_states: [end]
states:
  - name: start
    prompt: test.md
    next: end
  - name: end
    prompt: test.md
EOF
    run "$SCRIPTS_DIR/validate-workflow.sh" "$TEST_TEMP_DIR/bad_entry.yaml"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"not found in states"* ]]
}

@test "reports correct state count" {
    local workflow=$(create_test_workflow)
    run "$SCRIPTS_DIR/validate-workflow.sh" "$workflow"
    [[ "$output" == *"states: 3 defined"* ]]
}
