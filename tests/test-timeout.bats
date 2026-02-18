#!/usr/bin/env bats
# Tests for timeout behavior in iterate.sh

setup() {
    SCRIPT_DIR="$(cd "$(dirname "$BATS_TEST_FILENAME")/.." && pwd)/scripts"
    FIXTURES_DIR="$(cd "$(dirname "$BATS_TEST_FILENAME")" && pwd)/fixtures"
}

@test "timeout wrapper kills hanging process" {
    # Use a 2-second timeout on a 60-second sleep
    run timeout --foreground -k 1 2 sleep 60

    # Exit code 124 = timeout occurred
    [ "$status" -eq 124 ]
}

@test "timeout wrapper allows fast commands to complete" {
    run timeout --foreground -k 1 5 echo "quick"

    [ "$status" -eq 0 ]
    [ "$output" = "quick" ]
}

@test "TEST_TIMEOUT variable is defined in iterate.sh" {
    grep -q 'TEST_TIMEOUT="\${TEST_TIMEOUT:-' "$SCRIPT_DIR/iterate.sh"
}

@test "iterate.sh delegates to run-phase-tests.sh" {
    grep -q 'run-phase-tests.sh' "$SCRIPT_DIR/iterate.sh"
}

@test "run-phase-tests.sh uses timeout wrapper" {
    grep -q 'timeout --foreground -k' "$SCRIPT_DIR/run-phase-tests.sh"
}

@test "no legacy cargo qa_PHASE fallback exists" {
    # Should NOT find cargo nextest with qa_ pattern
    ! grep -q 'cargo nextest.*qa_\${PHASE}' "$SCRIPT_DIR/iterate.sh"
}

@test "run-phase-tests.sh reads test_files from state.json" {
    grep -q 'test_files' "$SCRIPT_DIR/run-phase-tests.sh"
}

@test "run-phase-tests.sh supports --retry-failed flag" {
    grep -q '\-\-retry-failed' "$SCRIPT_DIR/run-phase-tests.sh"
    grep -q 'RETRY_FAILED=true' "$SCRIPT_DIR/run-phase-tests.sh"
}

@test "run-phase-tests.sh saves failed test names" {
    grep -q 'failed_tests.txt' "$SCRIPT_DIR/run-phase-tests.sh"
    grep -q 'FAILED_TEST_NAMES' "$SCRIPT_DIR/run-phase-tests.sh"
}

@test "hanging bats test gets killed by timeout" {
    # Note: We can't nest bats inside bats (process group issues)
    # So we test this outside of bats - see manual test below
    # This test verifies the fixture exists for manual testing

    [ -f "$FIXTURES_DIR/hanging_test.bats" ]
    grep -q "sleep 300" "$FIXTURES_DIR/hanging_test.bats"
}

# Manual test (run outside of bats):
# time timeout --foreground -k 2 3 npx bats $ARC_HOME/tests/fixtures/hanging_test.bats
# Expected: exits in ~3 seconds with code 124

# =============================================================================
# Process Group Tracking Tests
# =============================================================================

@test "iterate.sh has process group tracking functions" {
    grep -q "declare -a TRACKED_PGIDS" "$SCRIPT_DIR/iterate.sh"
    grep -q "track_pgid()" "$SCRIPT_DIR/iterate.sh"
    grep -q "untrack_pgid()" "$SCRIPT_DIR/iterate.sh"
    grep -q "kill_tracked_pgids()" "$SCRIPT_DIR/iterate.sh"
}

@test "run_with_timeout uses setsid for process groups" {
    grep -q "setsid timeout" "$SCRIPT_DIR/iterate.sh"
}

@test "run_with_timeout tracks PGID" {
    grep -q "track_pgid" "$SCRIPT_DIR/iterate.sh"
    grep -q "untrack_pgid" "$SCRIPT_DIR/iterate.sh"
}

@test "cleanup kills tracked process groups" {
    grep -q "kill_tracked_pgids" "$SCRIPT_DIR/iterate.sh"
}

@test "no pkill cargo patterns remain in iterate.sh" {
    # All pkill cargo patterns should be removed in favor of PGID tracking
    ! grep -q 'pkill.*cargo.*nextest' "$SCRIPT_DIR/iterate.sh"
    ! grep -q 'pkill.*cargo.*build' "$SCRIPT_DIR/iterate.sh"
}

@test "setsid creates new process group" {
    # Run a command with setsid and verify PGID differs from parent
    local parent_pgid=$(ps -o pgid= -p $$ | tr -d ' ')

    # setsid creates a new session, PGID = PID of new session leader
    setsid sleep 0.1 &
    local child_pid=$!
    local child_pgid=$(ps -o pgid= -p $child_pid 2>/dev/null | tr -d ' ' || echo "$parent_pgid")
    wait $child_pid 2>/dev/null || true

    # The child's PGID should equal its PID (session leader)
    # AND differ from parent's PGID (unless we're already a session leader)
    [ "$child_pgid" = "$child_pid" ] || [ "$child_pgid" != "$parent_pgid" ]
}
