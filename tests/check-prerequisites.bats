#!/usr/bin/env bats

# Tests for check-prerequisites.sh

setup() {
    load 'test_helper'
    setup_temp_dir
}

teardown() {
    teardown_temp_dir
}

@test "check-prerequisites.sh exists and is executable" {
    [[ -x "$SCRIPTS_DIR/check-prerequisites.sh" ]]
}

@test "passes when all prerequisites are installed" {
    # This test assumes the dev environment has all tools installed
    run "$SCRIPTS_DIR/check-prerequisites.sh"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"All prerequisites satisfied"* ]]
}

@test "detects yq and validates version" {
    run "$SCRIPTS_DIR/check-prerequisites.sh"
    [[ "$output" == *"yq"* ]]
    [[ "$output" == *"mikefarah"* ]] || [[ "$output" == *"version v4"* ]]
}

@test "detects jq" {
    run "$SCRIPTS_DIR/check-prerequisites.sh"
    [[ "$output" == *"jq found"* ]]
}

@test "detects python3 and validates version" {
    run "$SCRIPTS_DIR/check-prerequisites.sh"
    [[ "$output" == *"python3"* ]]
    [[ "$output" == *"version OK"* ]]
}

@test "detects cargo" {
    run "$SCRIPTS_DIR/check-prerequisites.sh"
    [[ "$output" == *"cargo found"* ]]
}

@test "detects cargo-nextest" {
    run "$SCRIPTS_DIR/check-prerequisites.sh"
    [[ "$output" == *"cargo-nextest found"* ]]
}

@test "output includes checking message" {
    run "$SCRIPTS_DIR/check-prerequisites.sh"
    [[ "$output" == *"Checking prerequisites"* ]]
}

@test "check functions are defined correctly" {
    # Source the script and verify it has proper structure
    # This is a basic sanity check that the script is well-formed
    run bash -n "$SCRIPTS_DIR/check-prerequisites.sh"
    [[ "$status" -eq 0 ]]
}
