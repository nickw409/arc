# Test helper for orchestration script tests

# Get the directory containing this helper
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ORCH_DIR="$(dirname "$TEST_DIR")"
SCRIPTS_DIR="$ORCH_DIR/scripts"
WORKFLOWS_DIR="$ORCH_DIR/workflows"

# Create a temporary directory for test fixtures
setup_temp_dir() {
    export TEST_TEMP_DIR="$(mktemp -d)"
}

# Clean up temporary directory
teardown_temp_dir() {
    if [[ -n "${TEST_TEMP_DIR:-}" && -d "$TEST_TEMP_DIR" ]]; then
        rm -rf "$TEST_TEMP_DIR"
    fi
}

# Create a valid test workflow file
create_test_workflow() {
    local output_file="${1:-$TEST_TEMP_DIR/workflow.yaml}"
    cat > "$output_file" << 'EOF'
name: test_workflow
version: 1
description: Test workflow for bats tests

states:
  - name: start
    description: Initial state
    prompt: prompts/feature/qa.md
    next: middle

  - name: middle
    description: Middle state
    prompt: prompts/feature/impl.md
    next: end

  - name: end
    description: Final state
    prompt: prompts/common/complete.md

entry_state: start
terminal_states: [end, blocked]
EOF
    echo "$output_file"
}

# Create an invalid workflow (missing required fields)
create_invalid_workflow() {
    local output_file="${1:-$TEST_TEMP_DIR/invalid.yaml}"
    cat > "$output_file" << 'EOF'
name: invalid
# Missing version, entry_state, terminal_states
states: []
EOF
    echo "$output_file"
}

# Create a workflow with bad YAML syntax
create_malformed_yaml() {
    local output_file="${1:-$TEST_TEMP_DIR/malformed.yaml}"
    cat > "$output_file" << 'EOF'
name: malformed
states:
  - name: foo
    bad indentation here
  next: bar
EOF
    echo "$output_file"
}
