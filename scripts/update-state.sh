#!/bin/bash
set -euo pipefail

# Atomic state updates for orchestration
# Matches state.json schema: phase_status, iteration.current/max, dispute, etc.
#
# Usage: update-state.sh <plan> <phase> <command> [args...]
#
# Commands:
#   status <STATUS>              - Set phase_status (pending|implementing|complete|disputed|blocked|deferred)
#   dispute <test> <reason>      - File a dispute (sets phase_status to disputed)
#   reject-dispute <reason>      - Reject dispute (sets phase_status to implementing)
#   approve-dispute <reason>     - Approve dispute (keeps disputed until fix)
#   clear-dispute                - Clear dispute after fix (sets phase_status to implementing)
#   tests <passing> <total>      - Update test counts and auto-complete if all pass
#   increment-iteration          - Bump iteration.current
#   mark-reviewed                - Set last_reviewed_iteration to current iteration (impl)
#   mark-qa-reviewed             - Set last_qa_reviewed_iteration to current iteration
#   check-review-required        - Exit 1 if impl-review needed before commit
#   check-qa-review-required     - Exit 1 if qa-review needed before committing tests
#   add-test-file <path>         - Register a test file for this phase (relative to project root)
#   clear-test-files             - Remove all registered test files
#   verdict <verdict>            - Record verdict from review state (V2 workflow support)
#   parallel-start <dir> <csv>  - Start parallel execution with branch names
#   parallel-update <name> <st> [code] - Update branch status/exit_code
#   parallel-finish <verdict>   - Record parallel completion verdict
#   parallel-clear              - Remove .parallel_execution from state

ARC_SCRIPTS_DIR="${ARC_SCRIPTS_DIR:-$ARC_HOME/scripts}"
ARC_HOME="${ARC_HOME:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
ARC_PROJECT_ROOT="${ARC_PROJECT_ROOT:-$(pwd)}"
ARC_PLANS_DIR="${ARC_PLANS_DIR:-$ARC_PROJECT_ROOT/.plans}"

PLAN="${1:-}"
PHASE="${2:-}"
COMMAND="${3:-}"

if [[ -z "$PLAN" || -z "$PHASE" || -z "$COMMAND" ]]; then
    echo "Usage: update-state.sh <plan> <phase> <command> [args...]" >&2
    echo "" >&2
    echo "Commands:" >&2
    echo "  status <STATUS>         - pending|implementing|complete|disputed|blocked|deferred" >&2
    echo "  dispute <test> <reason> - File a dispute" >&2
    echo "  reject-dispute <reason> - Reject, continue implementing" >&2
    echo "  approve-dispute <reason>- Approve, ready for fix mode" >&2
    echo "  clear-dispute           - Clear after fix" >&2
    echo "  tests <pass> <total>    - Update test counts" >&2
    echo "  increment-iteration     - Bump iteration counter" >&2
    echo "  mark-reviewed           - Mark current iteration as impl-reviewed" >&2
    echo "  mark-qa-reviewed        - Mark current iteration as qa-reviewed" >&2
    echo "  check-review-required   - Check if impl-review needed (exit 1 if yes)" >&2
    echo "  check-qa-review-required - Check if qa-review needed (exit 1 if yes)" >&2
    echo "  add-test-file <path>    - Register a test file for this phase" >&2
    echo "  clear-test-files        - Remove all registered test files" >&2
    exit 1
fi

STATE_FILE="$ARC_PLANS_DIR/active/$PLAN/phases/$PHASE/state.json"
TEMP_FILE="$STATE_FILE.tmp.$$"

if [[ ! -f "$STATE_FILE" ]]; then
    echo "Error: State file not found: $STATE_FILE" >&2
    exit 1
fi

# Read current state
STATE=$(cat "$STATE_FILE")

# Validate JSON
if ! echo "$STATE" | jq empty 2>/dev/null; then
    echo "Error: Invalid JSON in state file: $STATE_FILE" >&2
    exit 1
fi

# Helper to write atomically
write_state() {
    echo "$1" > "$TEMP_FILE"
    mv "$TEMP_FILE" "$STATE_FILE"
}

case "$COMMAND" in
    status)
        NEW_STATUS="${4:-}"
        # Valid statuses: pending, qa, qa_review, implementing, impl_review, complete, disputed, blocked, deferred
        if [[ ! "$NEW_STATUS" =~ ^(pending|qa|qa_review|implementing|impl_review|complete|disputed|blocked|deferred)$ ]]; then
            echo "Error: Invalid status." >&2
            echo "Valid: pending|qa|qa_review|implementing|impl_review|complete|disputed|blocked|deferred" >&2
            exit 1
        fi
        NEW_STATE=$(echo "$STATE" | jq --arg s "$NEW_STATUS" '.phase_status = $s')
        write_state "$NEW_STATE"
        echo "Status set to $NEW_STATUS"
        ;;

    set-packages|set-crates)
        # Set the packages array - accepts comma-separated list or JSON array
        PKGS_ARG="${4:-}"
        if [[ -z "$PKGS_ARG" ]]; then
            echo "Usage: update-state.sh <plan> <phase> set-packages <pkg1,pkg2,...>" >&2
            echo "   or: update-state.sh <plan> <phase> set-packages '[\"pkg1\",\"pkg2\"]'" >&2
            exit 1
        fi
        # Check if it's already JSON array format
        if [[ "$PKGS_ARG" == "["* ]]; then
            PKGS_JSON="$PKGS_ARG"
        else
            # Convert comma-separated to JSON array
            PKGS_JSON=$(echo "$PKGS_ARG" | tr ',' '\n' | jq -R . | jq -s .)
        fi
        NEW_STATE=$(echo "$STATE" | jq --argjson p "$PKGS_JSON" '.packages = $p')
        write_state "$NEW_STATE"
        echo "Packages set to: $PKGS_JSON"
        ;;

    dispute)
        TEST_NAME="${4:-}"
        REASON="${5:-}"
        if [[ -z "$TEST_NAME" || -z "$REASON" ]]; then
            echo "Usage: update-state.sh <plan> <phase> dispute <test_name> <reason>" >&2
            exit 1
        fi
        # Check if already disputed
        EXISTING=$(echo "$STATE" | jq --arg t "$TEST_NAME" '.disputes // [] | map(select(.test_name == $t)) | length')
        if [[ "$EXISTING" -gt 0 ]]; then
            echo "Already disputed: $TEST_NAME"
            exit 0
        fi
        NEW_STATE=$(echo "$STATE" | jq \
            --arg t "$TEST_NAME" \
            --arg r "$REASON" \
            '.phase_status = "disputed" |
             .disputes = ((.disputes // []) + [{
                 test_name: $t,
                 reason: $r,
                 resolution: null
             }])')
        write_state "$NEW_STATE"
        DISPUTE_COUNT=$(echo "$NEW_STATE" | jq '.disputes | length')
        echo "Dispute filed on $TEST_NAME ($DISPUTE_COUNT total)"
        ;;
        
    reject-dispute)
        REASON="${4:-}"
        if [[ -z "$REASON" ]]; then
            echo "Usage: update-state.sh <plan> <phase> reject-dispute <reason>" >&2
            exit 1
        fi
        NEW_STATE=$(echo "$STATE" | jq \
            --arg r "$REASON" \
            '.phase_status = "implementing" |
             .disputes = [(.disputes // [])[] | .resolution = "rejected" | .resolution_reason = $r]')
        write_state "$NEW_STATE"
        echo "All disputes rejected, resuming implementation"
        ;;

    approve-dispute)
        REASON="${4:-}"
        if [[ -z "$REASON" ]]; then
            echo "Usage: update-state.sh <plan> <phase> approve-dispute <reason>" >&2
            exit 1
        fi
        # Keep disputed status - will be cleared by fix mode
        NEW_STATE=$(echo "$STATE" | jq \
            --arg r "$REASON" \
            '.disputes = [(.disputes // [])[] | .resolution = "approved" | .resolution_reason = $r]')
        write_state "$NEW_STATE"
        DISPUTE_COUNT=$(echo "$NEW_STATE" | jq '.disputes | length')
        echo "All $DISPUTE_COUNT disputes approved, run fix mode to correct tests"
        ;;

    clear-dispute)
        # Preserve disputes so impl agent knows what was fixed
        NEW_STATE=$(echo "$STATE" | jq \
            '.phase_status = "implementing" |
             .last_cleared_disputes = .disputes |
             .disputes = []')
        write_state "$NEW_STATE"
        echo "All disputes cleared (preserved in last_cleared_disputes)"
        ;;
        
    tests)
        PASSING="${4:-}"
        TOTAL="${5:-}"
        if [[ -z "$PASSING" || -z "$TOTAL" ]]; then
            echo "Usage: update-state.sh <plan> <phase> tests <passing> <total>" >&2
            exit 1
        fi

        # Track stuck progress
        PREV_PASSING=$(echo "$STATE" | jq '.tests_passing // 0')
        STUCK_COUNT=$(echo "$STATE" | jq '.stuck_iterations // 0')

        if [[ "$TOTAL" -eq 0 ]]; then
            # No tests at all - always stuck (can't make progress without tests)
            STUCK_COUNT=$((STUCK_COUNT + 1))
        elif [[ "$PASSING" -eq "$PREV_PASSING" && "$PASSING" -lt "$TOTAL" ]]; then
            # No progress - increment stuck counter
            STUCK_COUNT=$((STUCK_COUNT + 1))
        else
            # Progress made - reset stuck counter
            STUCK_COUNT=0
        fi

        NEW_STATE=$(echo "$STATE" | jq \
            --argjson p "$PASSING" \
            --argjson t "$TOTAL" \
            --argjson s "$STUCK_COUNT" \
            '.tests_passing = $p | .tests_total = $t | .stuck_iterations = $s')

        # Auto-rollback/block if stuck for 6+ iterations (after escalation ladder exhausted)
        if [[ "$STUCK_COUNT" -ge 6 ]]; then
            MAX_ROLLBACKS=2
            ROLLBACK_COUNT=$(echo "$STATE" | jq '.rollback_count // 0')

            if [[ "$ROLLBACK_COUNT" -lt "$MAX_ROLLBACKS" ]]; then
                # Attempt auto-rollback
                ROLLBACK_COUNT=$((ROLLBACK_COUNT + 1))
                echo ""
                echo "=========================================="
                echo "AUTO-ROLLBACK (attempt $ROLLBACK_COUNT of $MAX_ROLLBACKS)"
                echo "=========================================="
                echo "No progress for $STUCK_COUNT iterations."
                echo "Resetting phase to retry from scratch."

                NEW_STATE=$(echo "$NEW_STATE" | jq \
                    --argjson rc "$ROLLBACK_COUNT" \
                    '.phase_status = "implementing" |
                     .iteration.current = 0 |
                     .stuck_iterations = 0 |
                     .tests_passing = 0 |
                     .rollback_count = $rc |
                     .last_rollback_reason = "stuck_iterations" |
                     .last_reviewed_iteration = 0')

                echo "Phase will retry from iteration 0."
                echo "Rollback count: $ROLLBACK_COUNT/$MAX_ROLLBACKS"
                echo "=========================================="
            else
                # Max rollbacks reached - truly block
                NEW_STATE=$(echo "$NEW_STATE" | jq '.phase_status = "blocked"')
                echo ""
                echo "=========================================="
                echo "PERMANENTLY BLOCKED"
                echo "=========================================="
                echo "No progress for $STUCK_COUNT iterations."
                echo "Already rolled back $ROLLBACK_COUNT times without success."
                echo "Human intervention required."
                echo "=========================================="
            fi
        fi

        # Auto-set to impl_review if all tests pass (impl-review will mark complete)
        if [[ "$PASSING" -eq "$TOTAL" && "$TOTAL" -gt 0 ]]; then
            NEW_STATE=$(echo "$NEW_STATE" | jq '.phase_status = "impl_review" | .stuck_iterations = 0')
            echo "All tests passing - ready for impl-review"
        fi

        write_state "$NEW_STATE"
        echo "Tests: $PASSING/$TOTAL (stuck: $STUCK_COUNT)"
        ;;
        
    increment-iteration)
        NEW_STATE=$(echo "$STATE" | jq '.iteration.current += 1')
        ITERATION=$(echo "$NEW_STATE" | jq '.iteration.current')
        MAX=$(echo "$NEW_STATE" | jq '.iteration.max')
        write_state "$NEW_STATE"
        echo "Iteration $ITERATION/$MAX"
        ;;

    mark-reviewed)
        CURRENT=$(echo "$STATE" | jq '.iteration.current')
        NEW_STATE=$(echo "$STATE" | jq --argjson c "$CURRENT" '.last_reviewed_iteration = $c')
        write_state "$NEW_STATE"
        echo "Marked iteration $CURRENT as reviewed"
        ;;

    check-review-required)
        CURRENT=$(echo "$STATE" | jq '.iteration.current')
        LAST_REVIEWED=$(echo "$STATE" | jq '.last_reviewed_iteration // 0')
        if [[ "$CURRENT" -gt "$LAST_REVIEWED" ]]; then
            echo "ERROR: impl-review required before commit"
            echo "  Current iteration: $CURRENT"
            echo "  Last reviewed: $LAST_REVIEWED"
            echo ""
            echo "Run: arc/scripts/iterate.sh $PLAN $PHASE impl-review"
            exit 1
        else
            echo "Review OK: iteration $CURRENT was impl-reviewed"
            exit 0
        fi
        ;;

    mark-qa-reviewed)
        CURRENT=$(echo "$STATE" | jq '.iteration.current')
        NEW_STATE=$(echo "$STATE" | jq --argjson c "$CURRENT" '.last_qa_reviewed_iteration = $c')
        write_state "$NEW_STATE"
        echo "Marked iteration $CURRENT as qa-reviewed"
        ;;

    check-qa-review-required)
        QA_REVIEWED=$(echo "$STATE" | jq '.last_qa_reviewed_iteration // -1')
        if [[ "$QA_REVIEWED" -lt 0 ]]; then
            echo "ERROR: qa-review required before committing tests"
            echo "  QA has not been reviewed yet"
            echo ""
            echo "Run: arc/scripts/iterate.sh $PLAN $PHASE qa-review"
            exit 1
        else
            echo "QA Review OK: qa-review completed at iteration $QA_REVIEWED"
            exit 0
        fi
        ;;

    record-hang)
        HANG_COUNT=$(echo "$STATE" | jq '.hang_count // 0')
        HANG_COUNT=$((HANG_COUNT + 1))
        NEW_STATE=$(echo "$STATE" | jq --argjson h "$HANG_COUNT" '.hang_count = $h')

        if [[ "$HANG_COUNT" -ge 3 ]]; then
            # Check if we can rollback
            MAX_ROLLBACKS=2
            ROLLBACK_COUNT=$(echo "$STATE" | jq '.rollback_count // 0')

            if [[ "$ROLLBACK_COUNT" -lt "$MAX_ROLLBACKS" ]]; then
                # Attempt auto-rollback
                ROLLBACK_COUNT=$((ROLLBACK_COUNT + 1))
                echo ""
                echo "=========================================="
                echo "AUTO-ROLLBACK (attempt $ROLLBACK_COUNT of $MAX_ROLLBACKS)"
                echo "=========================================="
                echo "Sub-agent hung $HANG_COUNT times."
                echo "Resetting phase to retry from scratch."

                NEW_STATE=$(echo "$NEW_STATE" | jq \
                    --argjson rc "$ROLLBACK_COUNT" \
                    '.phase_status = "implementing" |
                     .iteration.current = 0 |
                     .stuck_iterations = 0 |
                     .hang_count = 0 |
                     .tests_passing = 0 |
                     .rollback_count = $rc |
                     .last_rollback_reason = "hangs" |
                     .last_reviewed_iteration = 0')

                echo "Phase will retry from iteration 0."
                echo "Rollback count: $ROLLBACK_COUNT/$MAX_ROLLBACKS"
                echo "=========================================="
            else
                # Max rollbacks reached - truly block
                NEW_STATE=$(echo "$NEW_STATE" | jq '.phase_status = "blocked"')
                echo ""
                echo "=========================================="
                echo "PERMANENTLY BLOCKED"
                echo "=========================================="
                echo "Sub-agent hung $HANG_COUNT times."
                echo "Already rolled back $ROLLBACK_COUNT times without success."
                echo "Human intervention required."
                echo "=========================================="
            fi
        else
            echo "HANG: Sub-agent timed out ($HANG_COUNT/3 before auto-block)"
            echo "Orchestrator should investigate and provide targeted instructions"
        fi
        write_state "$NEW_STATE"
        ;;

    clear-hangs)
        NEW_STATE=$(echo "$STATE" | jq '.hang_count = 0')
        write_state "$NEW_STATE"
        echo "Hang counter reset"
        ;;

    record-crash)
        # Track unexpected sub-agent crashes (not timeouts)
        CRASH_COUNT=$(echo "$STATE" | jq '.crash_count // 0')
        CRASH_COUNT=$((CRASH_COUNT + 1))
        NEW_STATE=$(echo "$STATE" | jq --argjson c "$CRASH_COUNT" '.crash_count = $c')

        if [[ "$CRASH_COUNT" -ge 3 ]]; then
            # Check if we can rollback
            MAX_ROLLBACKS=2
            ROLLBACK_COUNT=$(echo "$STATE" | jq '.rollback_count // 0')

            if [[ "$ROLLBACK_COUNT" -lt "$MAX_ROLLBACKS" ]]; then
                # Attempt auto-rollback
                ROLLBACK_COUNT=$((ROLLBACK_COUNT + 1))
                echo ""
                echo "=========================================="
                echo "AUTO-ROLLBACK (attempt $ROLLBACK_COUNT of $MAX_ROLLBACKS)"
                echo "=========================================="
                echo "Sub-agent crashed $CRASH_COUNT times."
                echo "Resetting phase to retry from scratch."

                NEW_STATE=$(echo "$NEW_STATE" | jq \
                    --argjson rc "$ROLLBACK_COUNT" \
                    '.phase_status = "implementing" |
                     .iteration.current = 0 |
                     .stuck_iterations = 0 |
                     .crash_count = 0 |
                     .hang_count = 0 |
                     .tests_passing = 0 |
                     .rollback_count = $rc |
                     .last_rollback_reason = "crashes" |
                     .last_reviewed_iteration = 0')

                echo "Phase will retry from iteration 0."
                echo "Rollback count: $ROLLBACK_COUNT/$MAX_ROLLBACKS"
                echo "=========================================="
            else
                # Max rollbacks reached - truly block
                NEW_STATE=$(echo "$NEW_STATE" | jq '.phase_status = "blocked"')
                echo ""
                echo "=========================================="
                echo "PERMANENTLY BLOCKED"
                echo "=========================================="
                echo "Sub-agent crashed $CRASH_COUNT times."
                echo "Already rolled back $ROLLBACK_COUNT times without success."
                echo "Human intervention required."
                echo "=========================================="
            fi
        else
            echo "CRASH: Sub-agent failed unexpectedly ($CRASH_COUNT/3 before auto-rollback)"
            echo "Orchestrator should check for orphaned processes and retry"
        fi
        write_state "$NEW_STATE"
        ;;

    clear-crashes)
        NEW_STATE=$(echo "$STATE" | jq '.crash_count = 0')
        write_state "$NEW_STATE"
        echo "Crash counter reset"
        ;;

    add-test-file)
        TEST_FILE="${4:-}"
        if [[ -z "$TEST_FILE" ]]; then
            echo "Usage: update-state.sh <plan> <phase> add-test-file <path>" >&2
            echo "  path: Relative to project root (e.g., arc/tests/my_test.bats)" >&2
            exit 1
        fi
        # Ensure test_files array exists and add the file (avoiding duplicates)
        NEW_STATE=$(echo "$STATE" | jq --arg f "$TEST_FILE" '
            .test_files = ((.test_files // []) | if index($f) then . else . + [$f] end)')
        write_state "$NEW_STATE"
        COUNT=$(echo "$NEW_STATE" | jq '.test_files | length')
        echo "Added test file: $TEST_FILE ($COUNT total)"
        ;;

    clear-test-files)
        NEW_STATE=$(echo "$STATE" | jq '.test_files = []')
        write_state "$NEW_STATE"
        echo "Test files cleared"
        ;;

    attempt-rollback)
        # Attempt to rollback a blocked phase. Returns 0 if rollback succeeded, 1 if max rollbacks reached.
        MAX_ROLLBACKS=2
        ROLLBACK_COUNT=$(echo "$STATE" | jq '.rollback_count // 0')
        CURRENT_STATUS=$(echo "$STATE" | jq -r '.phase_status')

        if [[ "$CURRENT_STATUS" != "blocked" ]]; then
            echo "Phase is not blocked (status: $CURRENT_STATUS)"
            exit 1
        fi

        if [[ "$ROLLBACK_COUNT" -ge "$MAX_ROLLBACKS" ]]; then
            echo "=========================================="
            echo "PERMANENTLY BLOCKED"
            echo "=========================================="
            echo "Phase has been rolled back $ROLLBACK_COUNT times without success."
            echo "Maximum rollbacks ($MAX_ROLLBACKS) reached."
            echo "Human intervention required."
            echo ""
            echo "Options:"
            echo "  1. Manually fix the issue and reset: update-state.sh $PLAN $PHASE reset-blocked"
            echo "  2. Split the phase: split-phase.sh $PLAN $PHASE <sub1> <sub2>"
            echo "  3. Skip the phase: update-state.sh $PLAN $PHASE status complete"
            echo "=========================================="
            exit 1
        fi

        # Perform rollback
        ROLLBACK_COUNT=$((ROLLBACK_COUNT + 1))
        echo "=========================================="
        echo "AUTO-ROLLBACK (attempt $ROLLBACK_COUNT of $MAX_ROLLBACKS)"
        echo "=========================================="

        # Reset state but preserve rollback count and some history
        NEW_STATE=$(echo "$STATE" | jq \
            --argjson rc "$ROLLBACK_COUNT" \
            '.phase_status = "implementing" |
             .iteration.current = 0 |
             .stuck_iterations = 0 |
             .hang_count = 0 |
             .tests_passing = 0 |
             .rollback_count = $rc |
             .last_rollback_reason = .phase_status |
             .last_reviewed_iteration = 0')

        write_state "$NEW_STATE"

        echo "State reset to implementing (iteration 0)"
        echo "Rollback count: $ROLLBACK_COUNT/$MAX_ROLLBACKS"
        echo ""
        echo "The phase will retry from the beginning."
        echo "Previous impl_reasoning.md preserved for context."
        echo "=========================================="
        exit 0
        ;;

    reset-blocked)
        # Manual reset after human fixes the issue
        NEW_STATE=$(echo "$STATE" | jq \
            '.phase_status = "implementing" |
             .iteration.current = 0 |
             .stuck_iterations = 0 |
             .hang_count = 0 |
             .rollback_count = 0')
        write_state "$NEW_STATE"
        echo "Phase reset (rollback count cleared)"
        ;;

    verdict)
        # Record verdict from review state (V2 workflow support)
        # Usage: update-state.sh <plan> <phase> verdict <verdict>
        VERDICT="${4:-}"
        if [[ -z "$VERDICT" ]]; then
            echo "Usage: update-state.sh <plan> <phase> verdict <verdict>" >&2
            exit 1
        fi

        TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
        ITERATION=$(echo "$STATE" | jq -r '.iteration.current // .iteration // 0')
        CURRENT_STATE=$(echo "$STATE" | jq -r '.phase_status // .current_state // "unknown"')

        NEW_STATE=$(echo "$STATE" | jq --arg v "$VERDICT" \
           --arg ts "$TIMESTAMP" \
           --argjson iter "$ITERATION" \
           --arg state "$CURRENT_STATE" \
           '.last_verdict = $v |
            .verdicts_history = (.verdicts_history // []) + [{
                iteration: $iter,
                state: $state,
                verdict: $v,
                timestamp: $ts
            }]')
        write_state "$NEW_STATE"
        echo "Recorded verdict: $VERDICT"
        ;;

    parallel-start)
        RESULTS_DIR="${4:-}"
        BRANCH_CSV="${5:-}"
        if [[ -z "$RESULTS_DIR" ]]; then
            echo "Usage: update-state.sh <plan> <phase> parallel-start <results_dir> <branch_names_csv>" >&2
            exit 1
        fi
        if [[ -z "$BRANCH_CSV" ]]; then
            echo "Error: empty branch names CSV" >&2
            exit 1
        fi

        # Parse CSV: split on commas, trim whitespace, reject empty names
        TIMESTAMP=$(date -Iseconds)
        BRANCHES_JSON="{}"
        VALID_COUNT=0

        # Check for trailing comma (read -ra drops trailing empty fields)
        if [[ "$BRANCH_CSV" == *"," ]]; then
            echo "Error: empty branch name in CSV (got: '$BRANCH_CSV')" >&2
            exit 1
        fi

        IFS=',' read -ra CSV_PARTS <<< "$BRANCH_CSV"
        for raw_name in "${CSV_PARTS[@]}"; do
            # Trim leading/trailing whitespace
            name=$(echo "$raw_name" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
            if [[ -z "$name" ]]; then
                echo "Error: empty branch name in CSV (got: '$BRANCH_CSV')" >&2
                exit 1
            fi
            BRANCHES_JSON=$(echo "$BRANCHES_JSON" | jq \
                --arg n "$name" \
                --arg ts "$TIMESTAMP" \
                '.[$n] = {"status": "running", "started_at": $ts}')
            VALID_COUNT=$((VALID_COUNT + 1))
        done

        if [[ "$VALID_COUNT" -eq 0 ]]; then
            echo "Error: empty branch names CSV" >&2
            exit 1
        fi

        # Clear any existing .parallel_execution, then set new one
        NEW_STATE=$(echo "$STATE" | jq \
            --arg rd "$RESULTS_DIR" \
            --arg ts "$TIMESTAMP" \
            --argjson branches "$BRANCHES_JSON" \
            'del(.parallel_execution) | .parallel_execution = {
                "results_dir": $rd,
                "started_at": $ts,
                "branches": $branches
            }')
        write_state "$NEW_STATE"
        echo "Parallel execution started with $VALID_COUNT branches"
        ;;

    parallel-update)
        BRANCH_NAME="${4:-}"
        BRANCH_STATUS="${5:-}"
        EXIT_CODE="${6:-}"

        # Validate parallel_execution exists
        HAS_PARALLEL=$(echo "$STATE" | jq 'has("parallel_execution")')
        if [[ "$HAS_PARALLEL" != "true" ]]; then
            echo "Error: no parallel execution in progress" >&2
            exit 1
        fi

        # Validate status
        if [[ ! "$BRANCH_STATUS" =~ ^(running|complete|failed|timeout)$ ]]; then
            echo "Error: invalid status '$BRANCH_STATUS' (valid: running, complete, failed, timeout)" >&2
            exit 1
        fi

        # Validate branch exists
        BRANCH_EXISTS=$(echo "$STATE" | jq --arg n "$BRANCH_NAME" '.parallel_execution.branches | has($n)')
        if [[ "$BRANCH_EXISTS" != "true" ]]; then
            echo "Error: branch '$BRANCH_NAME' not found" >&2
            exit 1
        fi

        # Reject resetting to "running" if branch was already in another status
        if [[ "$BRANCH_STATUS" == "running" ]]; then
            CURRENT_BRANCH_STATUS=$(echo "$STATE" | jq -r --arg n "$BRANCH_NAME" '.parallel_execution.branches[$n].status')
            if [[ "$CURRENT_BRANCH_STATUS" != "running" ]]; then
                echo "Error: cannot reset branch to running" >&2
                exit 1
            fi
        fi

        # Validate exit_code is an integer if provided
        if [[ -n "$EXIT_CODE" ]] && ! [[ "$EXIT_CODE" =~ ^-?[0-9]+$ ]]; then
            echo "Error: exit_code must be an integer, got '$EXIT_CODE'" >&2
            exit 1
        fi

        TIMESTAMP=$(date -Iseconds)

        # Build the jq update expression
        if [[ -n "$EXIT_CODE" ]]; then
            # With exit_code (--argjson for integer)
            if [[ "$BRANCH_STATUS" == "running" ]]; then
                NEW_STATE=$(echo "$STATE" | jq \
                    --arg n "$BRANCH_NAME" \
                    --arg s "$BRANCH_STATUS" \
                    --argjson ec "$EXIT_CODE" \
                    '.parallel_execution.branches[$n].status = $s |
                     .parallel_execution.branches[$n].exit_code = $ec')
            else
                NEW_STATE=$(echo "$STATE" | jq \
                    --arg n "$BRANCH_NAME" \
                    --arg s "$BRANCH_STATUS" \
                    --argjson ec "$EXIT_CODE" \
                    --arg ts "$TIMESTAMP" \
                    '.parallel_execution.branches[$n].status = $s |
                     .parallel_execution.branches[$n].exit_code = $ec |
                     .parallel_execution.branches[$n].completed_at = $ts')
            fi
        else
            # Without exit_code
            if [[ "$BRANCH_STATUS" == "running" ]]; then
                NEW_STATE=$(echo "$STATE" | jq \
                    --arg n "$BRANCH_NAME" \
                    --arg s "$BRANCH_STATUS" \
                    '.parallel_execution.branches[$n].status = $s')
            else
                NEW_STATE=$(echo "$STATE" | jq \
                    --arg n "$BRANCH_NAME" \
                    --arg s "$BRANCH_STATUS" \
                    --arg ts "$TIMESTAMP" \
                    '.parallel_execution.branches[$n].status = $s |
                     .parallel_execution.branches[$n].completed_at = $ts')
            fi
        fi
        write_state "$NEW_STATE"
        echo "Branch '$BRANCH_NAME' updated to $BRANCH_STATUS"
        ;;

    parallel-finish)
        VERDICT="${4:-}"

        # Validate parallel_execution exists
        HAS_PARALLEL=$(echo "$STATE" | jq 'has("parallel_execution")')
        if [[ "$HAS_PARALLEL" != "true" ]]; then
            echo "Error: no parallel execution in progress" >&2
            exit 1
        fi

        TIMESTAMP=$(date -Iseconds)
        NEW_STATE=$(echo "$STATE" | jq \
            --arg v "$VERDICT" \
            --arg ts "$TIMESTAMP" \
            '.parallel_execution.verdict = $v |
             .parallel_execution.completed_at = $ts |
             .last_verdict = $v')
        write_state "$NEW_STATE"
        echo "Parallel execution finished with verdict: $VERDICT"
        ;;

    parallel-clear)
        NEW_STATE=$(echo "$STATE" | jq 'del(.parallel_execution)')
        write_state "$NEW_STATE"
        echo "Parallel execution cleared"
        ;;

    *)
        echo "Unknown command: $COMMAND" >&2
        echo "Commands: status, set-crates, dispute, reject-dispute, approve-dispute, clear-dispute, tests, increment-iteration, mark-reviewed, mark-qa-reviewed, check-review-required, check-qa-review-required, add-test-file, clear-test-files, record-hang, clear-hangs, record-crash, clear-crashes, attempt-rollback, reset-blocked, verdict, parallel-start, parallel-update, parallel-finish, parallel-clear" >&2
        exit 1
        ;;
esac
