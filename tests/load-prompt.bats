#!/usr/bin/env bats

# Tests for load-prompt.sh

load 'test_helper'

setup() {
    SCRIPT_DIR="$(cd "$(dirname "$BATS_TEST_FILENAME")" && pwd)"
    ORCH_DIR="$(dirname "$SCRIPT_DIR")"
    LOAD_PROMPT="$ORCH_DIR/scripts/load-prompt.sh"
    PROMPTS_DIR="$ORCH_DIR/prompts"

    # Create temp dir for test files
    TEST_TEMP=$(mktemp -d)
}

teardown() {
    rm -rf "$TEST_TEMP"
}

@test "load-prompt.sh exists and is executable" {
    [ -x "$LOAD_PROMPT" ]
}

@test "load-prompt.sh is syntactically valid bash" {
    bash -n "$LOAD_PROMPT"
}

@test "shows usage with no arguments" {
    run "$LOAD_PROMPT"
    [ "$status" -eq 1 ]
    [[ "$output" == *"Usage:"* ]]
}

@test "fails when prompt file does not exist" {
    run "$LOAD_PROMPT" "nonexistent.md"
    [ "$status" -eq 1 ]
    [[ "$output" == *"not found"* ]]
}

@test "loads prompt file and returns content" {
    echo "Hello World" > "$TEST_TEMP/test.md"
    run "$LOAD_PROMPT" "$TEST_TEMP/test.md"
    [ "$status" -eq 0 ]
    [[ "$output" == *"Hello World"* ]]
}

@test "substitutes simple variable" {
    echo "Hello {{name}}" > "$TEST_TEMP/test.md"
    run "$LOAD_PROMPT" "$TEST_TEMP/test.md" name=Claude
    [ "$status" -eq 0 ]
    [[ "$output" == "Hello Claude" ]]
}

@test "substitutes multiple variables" {
    echo "Plan: {{plan}} Phase: {{phase}}" > "$TEST_TEMP/test.md"
    run "$LOAD_PROMPT" "$TEST_TEMP/test.md" plan=my-plan phase=phase1
    [ "$status" -eq 0 ]
    [[ "$output" == "Plan: my-plan Phase: phase1" ]]
}

@test "leaves unknown variables unchanged" {
    echo "Hello {{unknown}}" > "$TEST_TEMP/test.md"
    run "$LOAD_PROMPT" "$TEST_TEMP/test.md"
    [ "$status" -eq 0 ]
    [[ "$output" == "Hello {{unknown}}" ]]
}

@test "substitutes from environment variable with PROMPT_VAR_ prefix" {
    echo "Hello {{name}}" > "$TEST_TEMP/test.md"
    run bash -c "PROMPT_VAR_name=World '$LOAD_PROMPT' '$TEST_TEMP/test.md'"
    [ "$status" -eq 0 ]
    [[ "$output" == "Hello World" ]]
}

@test "command line args override environment variables" {
    echo "Hello {{name}}" > "$TEST_TEMP/test.md"
    run bash -c "PROMPT_VAR_name=World '$LOAD_PROMPT' '$TEST_TEMP/test.md' name=Override"
    [ "$status" -eq 0 ]
    [[ "$output" == "Hello Override" ]]
}

@test "handles multiline values from environment" {
    echo "Content: {{section}}" > "$TEST_TEMP/test.md"
    MULTILINE=$'Line 1\nLine 2\nLine 3'
    run bash -c "PROMPT_VAR_section='$MULTILINE' '$LOAD_PROMPT' '$TEST_TEMP/test.md'"
    [ "$status" -eq 0 ]
    [[ "$output" == *"Line 1"* ]]
    [[ "$output" == *"Line 2"* ]]
    [[ "$output" == *"Line 3"* ]]
}

@test "resolves relative paths from $ARC_HOME/" {
    run "$LOAD_PROMPT" prompts/feature/qa.md plan_name=test phase=p1 plan_file=/tmp/plan.md state_file=/tmp/state.json phase_dir=/tmp/phase qa_review_instruction=""
    [ "$status" -eq 0 ]
    [[ "$output" == *"QA Test Writer"* ]]
}

@test "feature/qa.md prompt file exists" {
    [ -f "$PROMPTS_DIR/feature/qa.md" ]
}

@test "feature/impl.md prompt file exists" {
    [ -f "$PROMPTS_DIR/feature/impl.md" ]
}

@test "feature/fix.md prompt file exists" {
    [ -f "$PROMPTS_DIR/feature/fix.md" ]
}

@test "feature/qa-review.md prompt file exists" {
    [ -f "$PROMPTS_DIR/feature/qa-review.md" ]
}

@test "feature/impl-review.md prompt file exists" {
    [ -f "$PROMPTS_DIR/feature/impl-review.md" ]
}

@test "common/header.md prompt file exists" {
    [ -f "$PROMPTS_DIR/common/header.md" ]
}
