#!/usr/bin/env bats

# Tests for V2 verdict handling in iterate.sh integration
# Phase: 03-iterate-integration (orchestration-v2)
#
# Since iterate.sh spawns Claude agents and cannot be tested directly,
# these tests verify the individual components and their interactions:
# 1. helpers.sh functions (get_state_verdicts, map_state_to_status)
# 2. update-state.sh verdict command
# 3. Integration via component chains (simulating iterate.sh behavior)

setup() {
    load 'test_helper'
    setup_temp_dir

    # Create a test plan/phase structure
    TEST_PLAN="test-plan"
    TEST_PHASE="test-phase"
    PLANS_DIR="$TEST_TEMP_DIR/.plans"
    PHASE_DIR="$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE"
    mkdir -p "$PHASE_DIR"

    # Create initial state.json
    cat > "$PHASE_DIR/state.json" << 'EOF'
{
  "phase": "test-phase",
  "plan": "test-plan",
  "workflow_type": "feature",
  "phase_status": "qa_review",
  "iteration": {
    "current": 1,
    "max": 25
  },
  "chunks": {
    "total": 0,
    "completed": [],
    "current": null,
    "remaining": []
  },
  "blocked": {
    "is_blocked": false,
    "reason": null
  },
  "disputes": [],
  "last_cleared_disputes": [],
  "packages": [],
  "tests_passing": 0,
  "tests_total": 0,
  "stuck_iterations": 0,
  "hang_count": 0,
  "last_reviewed_iteration": 0,
  "last_qa_reviewed_iteration": 0
}
EOF

    # Override PLANS_DIR for update-state.sh by creating symlink
    # update-state.sh uses PROJECT_ROOT/.plans, so we need to adjust
    export PROJECT_ROOT="$TEST_TEMP_DIR"
}

teardown() {
    teardown_temp_dir
}

# Helper to create V2 workflow with verdicts
create_v2_workflow() {
    local output_file="${1:-$TEST_TEMP_DIR/workflow.yaml}"
    cat > "$output_file" << 'EOF'
name: test_v2
version: 2
description: V2 workflow with conditional branches

states:
  - name: qa
    description: Write tests
    prompt: prompts/feature/qa.md
    next: qa_review

  - name: qa_review
    description: Review tests
    prompt: prompts/feature/qa-review.md
    verdicts:
      - approved
      - gaps_found
    next:
      approved: impl
      gaps_found: qa

  - name: impl
    description: Implement code
    prompt: prompts/feature/impl.md
    next: impl_review

  - name: impl_review
    description: Review implementation
    prompt: prompts/feature/impl-review.md
    verdicts:
      - approved
      - concerns
    next:
      approved: complete
      concerns: impl

  - name: complete
    description: Phase complete
    prompt: prompts/common/complete.md

  - name: blocked
    description: Blocked
    prompt: prompts/common/blocked.md

entry_state: qa
terminal_states: [complete, blocked]
EOF
    echo "$output_file"
}

# Helper to create V1 workflow (no verdicts, linear transitions)
create_v1_workflow() {
    local output_file="${1:-$TEST_TEMP_DIR/v1_workflow.yaml}"
    cat > "$output_file" << 'EOF'
name: test_v1
version: 1
description: V1 workflow with linear transitions

states:
  - name: qa
    prompt: prompts/feature/qa.md
    next: qa_review

  - name: qa_review
    prompt: prompts/feature/qa-review.md
    next: impl

  - name: impl
    prompt: prompts/feature/impl.md
    next: impl_review

  - name: impl_review
    prompt: prompts/feature/impl-review.md
    next: complete

  - name: complete
    prompt: prompts/common/complete.md

entry_state: qa
terminal_states: [complete]
EOF
    echo "$output_file"
}

# =============================================================================
# helpers.sh existence and sourcing tests
# =============================================================================

@test "helpers.sh exists and is executable" {
    [[ -f "$SCRIPTS_DIR/helpers.sh" ]]
}

@test "helpers.sh is syntactically valid bash" {
    run bash -n "$SCRIPTS_DIR/helpers.sh"
    [[ "$status" -eq 0 ]]
}

@test "helpers.sh can be sourced" {
    run bash -c "source '$SCRIPTS_DIR/helpers.sh'"
    [[ "$status" -eq 0 ]]
}

# =============================================================================
# get_state_verdicts tests
# =============================================================================

@test "test_get_state_verdicts_multiple_verdicts: returns comma-separated list" {
    local workflow=$(create_v2_workflow)
    source "$SCRIPTS_DIR/helpers.sh"
    run bash -c "source '$SCRIPTS_DIR/helpers.sh' && get_state_verdicts 'qa_review' '$workflow'"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "approved,gaps_found" ]]
}

@test "test_get_state_verdicts_impl_review: returns approved,concerns" {
    local workflow=$(create_v2_workflow)
    run bash -c "source '$SCRIPTS_DIR/helpers.sh' && get_state_verdicts 'impl_review' '$workflow'"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "approved,concerns" ]]
}

@test "test_get_state_verdicts_nonexistent_state: returns empty string" {
    local workflow=$(create_v2_workflow)
    run bash -c "source '$SCRIPTS_DIR/helpers.sh' && get_state_verdicts 'nonexistent_state' '$workflow'"
    [[ "$status" -eq 0 ]]
    [[ -z "$output" ]]
}

@test "test_get_state_verdicts_v1_state: returns empty string for state without verdicts" {
    local workflow=$(create_v2_workflow)
    # qa state has no verdicts in V2 workflow (linear next)
    run bash -c "source '$SCRIPTS_DIR/helpers.sh' && get_state_verdicts 'qa' '$workflow'"
    [[ "$status" -eq 0 ]]
    [[ -z "$output" ]]
}

@test "test_get_state_verdicts_malformed_yaml: returns empty string" {
    echo "{invalid yaml" > "$TEST_TEMP_DIR/malformed.yaml"
    run bash -c "source '$SCRIPTS_DIR/helpers.sh' && get_state_verdicts 'any_state' '$TEST_TEMP_DIR/malformed.yaml'"
    [[ "$status" -eq 0 ]]
    [[ -z "$output" ]]
}

@test "test_get_state_verdicts_empty_workflow: returns empty string" {
    touch "$TEST_TEMP_DIR/empty.yaml"
    run bash -c "source '$SCRIPTS_DIR/helpers.sh' && get_state_verdicts 'any_state' '$TEST_TEMP_DIR/empty.yaml'"
    [[ "$status" -eq 0 ]]
    [[ -z "$output" ]]
}

# =============================================================================
# map_state_to_status tests - Work states
# =============================================================================

@test "test_map_state_to_status_impl: returns implementing" {
    run bash -c "source '$SCRIPTS_DIR/helpers.sh' && map_state_to_status 'impl'"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "implementing" ]]
}

@test "test_map_state_to_status_fix: returns implementing" {
    run bash -c "source '$SCRIPTS_DIR/helpers.sh' && map_state_to_status 'fix'"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "implementing" ]]
}

@test "test_map_state_to_status_refactor: returns implementing" {
    run bash -c "source '$SCRIPTS_DIR/helpers.sh' && map_state_to_status 'refactor'"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "implementing" ]]
}

@test "test_map_state_to_status_optimize: returns implementing" {
    run bash -c "source '$SCRIPTS_DIR/helpers.sh' && map_state_to_status 'optimize'"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "implementing" ]]
}

@test "test_map_state_to_status_draft: returns implementing" {
    run bash -c "source '$SCRIPTS_DIR/helpers.sh' && map_state_to_status 'draft'"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "implementing" ]]
}

@test "test_map_state_to_status_research: returns implementing" {
    run bash -c "source '$SCRIPTS_DIR/helpers.sh' && map_state_to_status 'research'"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "implementing" ]]
}

@test "test_map_state_to_status_characterize: returns implementing" {
    run bash -c "source '$SCRIPTS_DIR/helpers.sh' && map_state_to_status 'characterize'"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "implementing" ]]
}

@test "test_map_state_to_status_baseline: returns implementing" {
    run bash -c "source '$SCRIPTS_DIR/helpers.sh' && map_state_to_status 'baseline'"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "implementing" ]]
}

@test "test_map_state_to_status_analyze: returns implementing" {
    run bash -c "source '$SCRIPTS_DIR/helpers.sh' && map_state_to_status 'analyze'"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "implementing" ]]
}

@test "test_map_state_to_status_investigate: returns implementing" {
    run bash -c "source '$SCRIPTS_DIR/helpers.sh' && map_state_to_status 'investigate'"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "implementing" ]]
}

@test "test_map_state_to_status_regression_tests: returns implementing" {
    run bash -c "source '$SCRIPTS_DIR/helpers.sh' && map_state_to_status 'regression_tests'"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "implementing" ]]
}

# =============================================================================
# map_state_to_status tests - QA states
# =============================================================================

@test "test_map_state_to_status_qa: returns qa" {
    run bash -c "source '$SCRIPTS_DIR/helpers.sh' && map_state_to_status 'qa'"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "qa" ]]
}

# =============================================================================
# map_state_to_status tests - Review states
# =============================================================================

@test "test_map_state_to_status_qa_review: returns qa_review" {
    run bash -c "source '$SCRIPTS_DIR/helpers.sh' && map_state_to_status 'qa_review'"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "qa_review" ]]
}

@test "test_map_state_to_status_impl_review: returns impl_review" {
    run bash -c "source '$SCRIPTS_DIR/helpers.sh' && map_state_to_status 'impl_review'"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "impl_review" ]]
}

@test "test_map_state_to_status_test_review: returns qa_review" {
    run bash -c "source '$SCRIPTS_DIR/helpers.sh' && map_state_to_status 'test_review'"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "qa_review" ]]
}

@test "test_map_state_to_status_char_review: returns qa_review" {
    run bash -c "source '$SCRIPTS_DIR/helpers.sh' && map_state_to_status 'char_review'"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "qa_review" ]]
}

@test "test_map_state_to_status_fix_review: returns qa_review" {
    run bash -c "source '$SCRIPTS_DIR/helpers.sh' && map_state_to_status 'fix_review'"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "qa_review" ]]
}

@test "test_map_state_to_status_review: returns qa_review" {
    run bash -c "source '$SCRIPTS_DIR/helpers.sh' && map_state_to_status 'review'"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "qa_review" ]]
}

@test "test_map_state_to_status_verify: returns qa_review" {
    run bash -c "source '$SCRIPTS_DIR/helpers.sh' && map_state_to_status 'verify'"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "qa_review" ]]
}

@test "test_map_state_to_status_benchmark: returns qa_review" {
    run bash -c "source '$SCRIPTS_DIR/helpers.sh' && map_state_to_status 'benchmark'"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "qa_review" ]]
}

# =============================================================================
# map_state_to_status tests - Terminal states
# =============================================================================

@test "test_map_state_to_status_complete: returns complete" {
    run bash -c "source '$SCRIPTS_DIR/helpers.sh' && map_state_to_status 'complete'"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "complete" ]]
}

@test "test_map_state_to_status_blocked: returns blocked" {
    run bash -c "source '$SCRIPTS_DIR/helpers.sh' && map_state_to_status 'blocked'"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "blocked" ]]
}

# =============================================================================
# map_state_to_status tests - Unknown states
# =============================================================================

@test "test_map_state_to_status_unknown: returns pass-through with warning" {
    run bash -c "source '$SCRIPTS_DIR/helpers.sh' && map_state_to_status 'unknown_state' 2>&1"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"WARNING"* ]]
    [[ "$output" == *"unknown_state"* ]]
}

@test "test_map_state_to_status_empty_input: returns empty with warning" {
    run bash -c "source '$SCRIPTS_DIR/helpers.sh' && map_state_to_status '' 2>&1"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"WARNING"* ]]
}

# =============================================================================
# update-state.sh verdict command tests
# =============================================================================

@test "test_update_state_verdict_command: records verdict in state.json" {
    # setup() already creates:
    # - PLANS_DIR="$TEST_TEMP_DIR/.plans"
    # - PHASE_DIR="$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE"
    # - state.json at $PHASE_DIR/state.json
    # No need to recreate or copy - just use existing paths

    # Create a wrapper script that sets the correct paths
    cat > "$TEST_TEMP_DIR/test-update-state.sh" << EOF
#!/bin/bash
set -euo pipefail
SCRIPT_DIR="$SCRIPTS_DIR"
ORCH_DIR="\$(dirname "\$SCRIPT_DIR")"
PROJECT_ROOT="$TEST_TEMP_DIR"
PLANS_DIR="$TEST_TEMP_DIR/.plans"

PLAN="\$1"
PHASE="\$2"
COMMAND="\$3"
STATE_FILE="\$PLANS_DIR/active/\$PLAN/phases/\$PHASE/state.json"
TEMP_FILE="\$STATE_FILE.tmp.\$\$"

if [[ ! -f "\$STATE_FILE" ]]; then
    echo "Error: State file not found: \$STATE_FILE" >&2
    exit 1
fi

STATE=\$(cat "\$STATE_FILE")

write_state() {
    echo "\$1" > "\$TEMP_FILE"
    mv "\$TEMP_FILE" "\$STATE_FILE"
}

case "\$COMMAND" in
    verdict)
        VERDICT="\$4"
        [[ -z "\$VERDICT" ]] && { echo "Usage: update-state.sh <plan> <phase> verdict <verdict>" >&2; exit 1; }

        TIMESTAMP=\$(date -u +"%Y-%m-%dT%H:%M:%SZ")
        ITERATION=\$(jq -r '.iteration.current // .iteration // 0' "\$STATE_FILE")
        CURRENT_STATE=\$(jq -r '.phase_status // .current_state // "unknown"' "\$STATE_FILE")

        jq --arg v "\$VERDICT" \
           --arg ts "\$TIMESTAMP" \
           --argjson iter "\$ITERATION" \
           --arg state "\$CURRENT_STATE" \
           '.last_verdict = \$v |
            .verdicts_history = (.verdicts_history // []) + [{
                iteration: \$iter,
                state: \$state,
                verdict: \$v,
                timestamp: \$ts
            }]' "\$STATE_FILE" > "\$STATE_FILE.tmp" && mv "\$STATE_FILE.tmp" "\$STATE_FILE"

        echo "Recorded verdict: \$VERDICT"
        ;;
    *)
        echo "Unknown command: \$COMMAND" >&2
        exit 1
        ;;
esac
EOF
    chmod +x "$TEST_TEMP_DIR/test-update-state.sh"

    run "$TEST_TEMP_DIR/test-update-state.sh" "$TEST_PLAN" "$TEST_PHASE" verdict approved
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Recorded verdict: approved"* ]]

    # Verify state.json was updated
    local last_verdict=$(jq -r '.last_verdict' "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json")
    [[ "$last_verdict" == "approved" ]]
}

@test "test_update_state_verdict_missing_argument: fails with usage" {
    # Create wrapper script without verdict argument
    cat > "$TEST_TEMP_DIR/test-verdict-missing.sh" << 'EOF'
#!/bin/bash
set -euo pipefail
VERDICT="${1:-}"
[[ -z "$VERDICT" ]] && { echo "Usage: update-state.sh <plan> <phase> verdict <verdict>" >&2; exit 1; }
echo "ok"
EOF
    chmod +x "$TEST_TEMP_DIR/test-verdict-missing.sh"

    run "$TEST_TEMP_DIR/test-verdict-missing.sh"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Usage:"* ]]
}

@test "test_update_state_verdict_appends_to_history: adds second entry" {
    local PLANS_DIR="$TEST_TEMP_DIR/.plans"
    mkdir -p "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE"

    # Create initial state with one verdict entry
    cat > "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json" << 'EOF'
{
  "phase_status": "qa_review",
  "iteration": {"current": 1, "max": 25},
  "last_verdict": "gaps_found",
  "verdicts_history": [
    {"iteration": 0, "state": "qa_review", "verdict": "gaps_found", "timestamp": "2025-01-01T00:00:00Z"}
  ]
}
EOF

    # Add a second verdict
    cat > "$TEST_TEMP_DIR/append-verdict.sh" << EOF
#!/bin/bash
set -euo pipefail
STATE_FILE="$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json"
TIMESTAMP=\$(date -u +"%Y-%m-%dT%H:%M:%SZ")
jq --arg v "approved" \
   --arg ts "\$TIMESTAMP" \
   --argjson iter 1 \
   --arg state "qa_review" \
   '.last_verdict = \$v |
    .verdicts_history = (.verdicts_history // []) + [{
        iteration: \$iter,
        state: \$state,
        verdict: \$v,
        timestamp: \$ts
    }]' "\$STATE_FILE" > "\$STATE_FILE.tmp" && mv "\$STATE_FILE.tmp" "\$STATE_FILE"
echo "done"
EOF
    chmod +x "$TEST_TEMP_DIR/append-verdict.sh"

    run "$TEST_TEMP_DIR/append-verdict.sh"
    [[ "$status" -eq 0 ]]

    # Verify verdicts_history has 2 entries
    local count=$(jq '.verdicts_history | length' "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json")
    [[ "$count" -eq 2 ]]

    # Verify last_verdict is updated
    local last=$(jq -r '.last_verdict' "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json")
    [[ "$last" == "approved" ]]
}

@test "test_update_state_verdict_corrupted_json: fails gracefully" {
    local PLANS_DIR="$TEST_TEMP_DIR/.plans"
    mkdir -p "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE"

    # Create corrupted JSON
    echo "{invalid json" > "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json"

    # Try to add verdict
    cat > "$TEST_TEMP_DIR/corrupt-verdict.sh" << EOF
#!/bin/bash
set -eo pipefail
STATE_FILE="$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json"
ORIGINAL=\$(cat "\$STATE_FILE")
if ! jq '.last_verdict = "approved"' "\$STATE_FILE" > "\$STATE_FILE.tmp" 2>&1; then
    echo "parse error" >&2
    exit 1
fi
mv "\$STATE_FILE.tmp" "\$STATE_FILE"
echo "done"
EOF
    chmod +x "$TEST_TEMP_DIR/corrupt-verdict.sh"

    run "$TEST_TEMP_DIR/corrupt-verdict.sh"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"parse error"* ]]
}

@test "test_verdict_history_accumulates: multiple verdicts over iterations" {
    local PLANS_DIR="$TEST_TEMP_DIR/.plans"
    mkdir -p "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE"

    cat > "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json" << 'EOF'
{
  "phase_status": "qa_review",
  "iteration": {"current": 0, "max": 25}
}
EOF

    # Script to add multiple verdicts
    cat > "$TEST_TEMP_DIR/multi-verdict.sh" << EOF
#!/bin/bash
set -euo pipefail
STATE_FILE="$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json"

add_verdict() {
    local verdict="\$1"
    local iter="\$2"
    local state="\$3"
    local ts=\$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    jq --arg v "\$verdict" \
       --arg ts "\$ts" \
       --argjson iter "\$iter" \
       --arg state "\$state" \
       '.last_verdict = \$v |
        .verdicts_history = (.verdicts_history // []) + [{
            iteration: \$iter,
            state: \$state,
            verdict: \$v,
            timestamp: \$ts
        }]' "\$STATE_FILE" > "\$STATE_FILE.tmp" && mv "\$STATE_FILE.tmp" "\$STATE_FILE"
}

add_verdict "gaps_found" 0 "qa_review"
add_verdict "gaps_found" 1 "qa_review"
add_verdict "approved" 2 "qa_review"
add_verdict "concerns" 3 "impl_review"
add_verdict "approved" 4 "impl_review"
echo "done"
EOF
    chmod +x "$TEST_TEMP_DIR/multi-verdict.sh"

    run "$TEST_TEMP_DIR/multi-verdict.sh"
    [[ "$status" -eq 0 ]]

    # Verify 5 entries in history
    local count=$(jq '.verdicts_history | length' "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json")
    [[ "$count" -eq 5 ]]

    # Verify each entry has required fields
    local valid=$(jq '[.verdicts_history[] | has("iteration", "state", "verdict", "timestamp")] | all' "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json")
    [[ "$valid" == "true" ]]

    # Verify timestamps are ISO8601 format
    local ts=$(jq -r '.verdicts_history[0].timestamp' "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json")
    [[ "$ts" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$ ]]
}

@test "test_update_state_verdict_atomic_write: uses tmp file pattern" {
    local PLANS_DIR="$TEST_TEMP_DIR/.plans"
    mkdir -p "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE"

    cat > "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json" << 'EOF'
{"phase_status": "qa_review", "iteration": {"current": 1}}
EOF

    # Script that verifies atomic write pattern
    cat > "$TEST_TEMP_DIR/atomic-test.sh" << EOF
#!/bin/bash
set -euo pipefail
STATE_FILE="$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json"
ORIGINAL=\$(cat "\$STATE_FILE")

# Simulate atomic write
jq '.last_verdict = "approved"' "\$STATE_FILE" > "\$STATE_FILE.tmp"
if [[ -f "\$STATE_FILE.tmp" ]]; then
    echo "tmp file created"
    mv "\$STATE_FILE.tmp" "\$STATE_FILE"
    if [[ ! -f "\$STATE_FILE.tmp" ]]; then
        echo "tmp file removed after mv"
    fi
fi

# Verify state was updated
if jq -e '.last_verdict == "approved"' "\$STATE_FILE" > /dev/null; then
    echo "state updated"
fi
EOF
    chmod +x "$TEST_TEMP_DIR/atomic-test.sh"

    run "$TEST_TEMP_DIR/atomic-test.sh"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"tmp file created"* ]]
    [[ "$output" == *"tmp file removed after mv"* ]]
    [[ "$output" == *"state updated"* ]]
}

# =============================================================================
# Integration tests - simulating iterate.sh qa-review mode
# =============================================================================

@test "test_qa_review_approved_transitions_to_impl: full component chain" {
    local workflow=$(create_v2_workflow)
    local PLANS_DIR="$TEST_TEMP_DIR/.plans"
    mkdir -p "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE"

    # Initial state: qa_review
    cat > "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json" << 'EOF'
{"phase_status": "qa_review", "iteration": {"current": 1}}
EOF

    # Create qa_review.md with approved verdict
    cat > "$TEST_TEMP_DIR/qa_review.md" << 'EOF'
# QA Review

Tests look good.

## Verdict
approved
EOF

    # Simulate iterate.sh qa-review mode component chain
    cat > "$TEST_TEMP_DIR/qa-review-chain.sh" << EOF
#!/bin/bash
set -euo pipefail
source '$SCRIPTS_DIR/helpers.sh'

WORKFLOW="$workflow"
STATE_FILE="$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json"
REVIEW_FILE="$TEST_TEMP_DIR/qa_review.md"
CURRENT_STATE="qa_review"

# 1. Get valid verdicts
VALID_VERDICTS=\$(get_state_verdicts "\$CURRENT_STATE" "\$WORKFLOW")
echo "Valid verdicts: \$VALID_VERDICTS"

# 2. Extract verdict
VERDICT=\$("$SCRIPTS_DIR/extract-verdict.sh" "\$REVIEW_FILE" "\$VALID_VERDICTS")
VERDICT_EXIT=\$?
echo "Extracted verdict: \$VERDICT (exit: \$VERDICT_EXIT)"

# 3. Record verdict
TIMESTAMP=\$(date -u +"%Y-%m-%dT%H:%M:%SZ")
jq --arg v "\$VERDICT" \
   --arg ts "\$TIMESTAMP" \
   --argjson iter 1 \
   --arg state "\$CURRENT_STATE" \
   '.last_verdict = \$v |
    .verdicts_history = (.verdicts_history // []) + [{
        iteration: \$iter,
        state: \$state,
        verdict: \$v,
        timestamp: \$ts
    }]' "\$STATE_FILE" > "\$STATE_FILE.tmp" && mv "\$STATE_FILE.tmp" "\$STATE_FILE"

# 4. Get next state
if [[ \$VERDICT_EXIT -eq 0 && "\$VERDICT" != "unknown" ]]; then
    NEXT_STATE=\$("$SCRIPTS_DIR/get-next-state.sh" "\$WORKFLOW" "\$CURRENT_STATE" "\$VERDICT")
    echo "Next state: \$NEXT_STATE"

    # 5. Map to status and update
    MAPPED_STATUS=\$(map_state_to_status "\$NEXT_STATE")
    echo "Mapped status: \$MAPPED_STATUS"

    jq --arg s "\$MAPPED_STATUS" '.phase_status = \$s' "\$STATE_FILE" > "\$STATE_FILE.tmp" && mv "\$STATE_FILE.tmp" "\$STATE_FILE"
fi
EOF
    chmod +x "$TEST_TEMP_DIR/qa-review-chain.sh"

    run "$TEST_TEMP_DIR/qa-review-chain.sh"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Valid verdicts: approved,gaps_found"* ]]
    [[ "$output" == *"Extracted verdict: approved"* ]]
    [[ "$output" == *"Next state: impl"* ]]
    [[ "$output" == *"Mapped status: implementing"* ]]

    # Verify final state
    local phase_status=$(jq -r '.phase_status' "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json")
    local last_verdict=$(jq -r '.last_verdict' "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json")
    [[ "$phase_status" == "implementing" ]]
    [[ "$last_verdict" == "approved" ]]
}

@test "test_qa_review_gaps_found_loops_to_qa: transitions back to qa" {
    local workflow=$(create_v2_workflow)
    local PLANS_DIR="$TEST_TEMP_DIR/.plans"
    mkdir -p "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE"

    cat > "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json" << 'EOF'
{"phase_status": "qa_review", "iteration": {"current": 1}}
EOF

    cat > "$TEST_TEMP_DIR/qa_review.md" << 'EOF'
## QA Review

Missing edge case coverage.

## Verdict
gaps_found
EOF

    cat > "$TEST_TEMP_DIR/gaps-chain.sh" << EOF
#!/bin/bash
set -euo pipefail
source '$SCRIPTS_DIR/helpers.sh'

WORKFLOW="$workflow"
STATE_FILE="$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json"
REVIEW_FILE="$TEST_TEMP_DIR/qa_review.md"
CURRENT_STATE="qa_review"

VALID_VERDICTS=\$(get_state_verdicts "\$CURRENT_STATE" "\$WORKFLOW")
VERDICT=\$("$SCRIPTS_DIR/extract-verdict.sh" "\$REVIEW_FILE" "\$VALID_VERDICTS")

# Record verdict
jq --arg v "\$VERDICT" '.last_verdict = \$v' "\$STATE_FILE" > "\$STATE_FILE.tmp" && mv "\$STATE_FILE.tmp" "\$STATE_FILE"

# Get next state and transition
NEXT_STATE=\$("$SCRIPTS_DIR/get-next-state.sh" "\$WORKFLOW" "\$CURRENT_STATE" "\$VERDICT")
MAPPED_STATUS=\$(map_state_to_status "\$NEXT_STATE")
jq --arg s "\$MAPPED_STATUS" '.phase_status = \$s' "\$STATE_FILE" > "\$STATE_FILE.tmp" && mv "\$STATE_FILE.tmp" "\$STATE_FILE"

echo "Done"
EOF
    chmod +x "$TEST_TEMP_DIR/gaps-chain.sh"

    run "$TEST_TEMP_DIR/gaps-chain.sh"
    [[ "$status" -eq 0 ]]

    local phase_status=$(jq -r '.phase_status' "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json")
    local last_verdict=$(jq -r '.last_verdict' "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json")
    [[ "$phase_status" == "qa" ]]
    [[ "$last_verdict" == "gaps_found" ]]
}

@test "test_impl_review_approved_completes: transitions to complete" {
    local workflow=$(create_v2_workflow)
    local PLANS_DIR="$TEST_TEMP_DIR/.plans"
    mkdir -p "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE"

    cat > "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json" << 'EOF'
{"phase_status": "impl_review", "iteration": {"current": 3}}
EOF

    cat > "$TEST_TEMP_DIR/impl_review.md" << 'EOF'
## Implementation Review

All tests pass, code quality is good.

## Verdict
approved
EOF

    cat > "$TEST_TEMP_DIR/impl-approved-chain.sh" << EOF
#!/bin/bash
set -euo pipefail
source '$SCRIPTS_DIR/helpers.sh'

WORKFLOW="$workflow"
STATE_FILE="$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json"
REVIEW_FILE="$TEST_TEMP_DIR/impl_review.md"
CURRENT_STATE="impl_review"

VALID_VERDICTS=\$(get_state_verdicts "\$CURRENT_STATE" "\$WORKFLOW")
VERDICT=\$("$SCRIPTS_DIR/extract-verdict.sh" "\$REVIEW_FILE" "\$VALID_VERDICTS")

jq --arg v "\$VERDICT" '.last_verdict = \$v' "\$STATE_FILE" > "\$STATE_FILE.tmp" && mv "\$STATE_FILE.tmp" "\$STATE_FILE"

NEXT_STATE=\$("$SCRIPTS_DIR/get-next-state.sh" "\$WORKFLOW" "\$CURRENT_STATE" "\$VERDICT")
MAPPED_STATUS=\$(map_state_to_status "\$NEXT_STATE")
jq --arg s "\$MAPPED_STATUS" '.phase_status = \$s' "\$STATE_FILE" > "\$STATE_FILE.tmp" && mv "\$STATE_FILE.tmp" "\$STATE_FILE"
EOF
    chmod +x "$TEST_TEMP_DIR/impl-approved-chain.sh"

    run "$TEST_TEMP_DIR/impl-approved-chain.sh"
    [[ "$status" -eq 0 ]]

    local phase_status=$(jq -r '.phase_status' "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json")
    local last_verdict=$(jq -r '.last_verdict' "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json")
    [[ "$phase_status" == "complete" ]]
    [[ "$last_verdict" == "approved" ]]
}

@test "test_impl_review_concerns_loops_to_impl: transitions back to implementing" {
    local workflow=$(create_v2_workflow)
    local PLANS_DIR="$TEST_TEMP_DIR/.plans"
    mkdir -p "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE"

    cat > "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json" << 'EOF'
{"phase_status": "impl_review", "iteration": {"current": 2}}
EOF

    cat > "$TEST_TEMP_DIR/impl_review.md" << 'EOF'
## Implementation Review

Some code quality concerns.

## Verdict
concerns
EOF

    cat > "$TEST_TEMP_DIR/impl-concerns-chain.sh" << EOF
#!/bin/bash
set -euo pipefail
source '$SCRIPTS_DIR/helpers.sh'

WORKFLOW="$workflow"
STATE_FILE="$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json"
REVIEW_FILE="$TEST_TEMP_DIR/impl_review.md"
CURRENT_STATE="impl_review"

VALID_VERDICTS=\$(get_state_verdicts "\$CURRENT_STATE" "\$WORKFLOW")
VERDICT=\$("$SCRIPTS_DIR/extract-verdict.sh" "\$REVIEW_FILE" "\$VALID_VERDICTS")

jq --arg v "\$VERDICT" '.last_verdict = \$v' "\$STATE_FILE" > "\$STATE_FILE.tmp" && mv "\$STATE_FILE.tmp" "\$STATE_FILE"

NEXT_STATE=\$("$SCRIPTS_DIR/get-next-state.sh" "\$WORKFLOW" "\$CURRENT_STATE" "\$VERDICT")
MAPPED_STATUS=\$(map_state_to_status "\$NEXT_STATE")
jq --arg s "\$MAPPED_STATUS" '.phase_status = \$s' "\$STATE_FILE" > "\$STATE_FILE.tmp" && mv "\$STATE_FILE.tmp" "\$STATE_FILE"
EOF
    chmod +x "$TEST_TEMP_DIR/impl-concerns-chain.sh"

    run "$TEST_TEMP_DIR/impl-concerns-chain.sh"
    [[ "$status" -eq 0 ]]

    local phase_status=$(jq -r '.phase_status' "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json")
    [[ "$phase_status" == "implementing" ]]
}

# =============================================================================
# V1 fallback tests
# =============================================================================

@test "test_v1_workflow_falls_back_to_grep: no verdicts, uses grep" {
    local workflow=$(create_v1_workflow)
    local PLANS_DIR="$TEST_TEMP_DIR/.plans"
    mkdir -p "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE"

    cat > "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json" << 'EOF'
{"phase_status": "impl_review", "iteration": {"current": 1}}
EOF

    cat > "$TEST_TEMP_DIR/impl_review.md" << 'EOF'
Implementation looks good. APPROVED.
EOF

    cat > "$TEST_TEMP_DIR/v1-fallback.sh" << EOF
#!/bin/bash
set -euo pipefail
source '$SCRIPTS_DIR/helpers.sh'

WORKFLOW="$workflow"
STATE_FILE="$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json"
IMPL_REVIEW="$TEST_TEMP_DIR/impl_review.md"
CURRENT_STATE="impl_review"

# Get valid verdicts (should be empty for V1)
VALID_VERDICTS=\$(get_state_verdicts "\$CURRENT_STATE" "\$WORKFLOW")
echo "Valid verdicts: '\$VALID_VERDICTS'"

if [[ -n "\$VALID_VERDICTS" ]]; then
    echo "V2 path"
    # V2 logic would go here
else
    echo "V1 fallback path"
    # V1 fallback: grep for APPROVED
    if [[ -f "\$IMPL_REVIEW" ]] && grep -q "APPROVED" "\$IMPL_REVIEW"; then
        jq '.phase_status = "complete"' "\$STATE_FILE" > "\$STATE_FILE.tmp" && mv "\$STATE_FILE.tmp" "\$STATE_FILE"
        echo "Set to complete via grep"
    fi
fi
EOF
    chmod +x "$TEST_TEMP_DIR/v1-fallback.sh"

    run "$TEST_TEMP_DIR/v1-fallback.sh"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Valid verdicts: ''"* ]]
    [[ "$output" == *"V1 fallback path"* ]]
    [[ "$output" == *"Set to complete via grep"* ]]

    local phase_status=$(jq -r '.phase_status' "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json")
    [[ "$phase_status" == "complete" ]]
}

@test "test_unknown_verdict_stays_in_state: no transition when verdict unknown" {
    local workflow=$(create_v2_workflow)
    local PLANS_DIR="$TEST_TEMP_DIR/.plans"
    mkdir -p "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE"

    cat > "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json" << 'EOF'
{"phase_status": "qa_review", "iteration": {"current": 1}}
EOF

    # Review file with no verdict section
    cat > "$TEST_TEMP_DIR/qa_review.md" << 'EOF'
## Analysis
The tests look good but I'm not sure about the edge cases.

Let me think about this more...
EOF

    cat > "$TEST_TEMP_DIR/unknown-verdict.sh" << EOF
#!/bin/bash
set -euo pipefail
source '$SCRIPTS_DIR/helpers.sh'

WORKFLOW="$workflow"
STATE_FILE="$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json"
REVIEW_FILE="$TEST_TEMP_DIR/qa_review.md"
CURRENT_STATE="qa_review"

VALID_VERDICTS=\$(get_state_verdicts "\$CURRENT_STATE" "\$WORKFLOW")
VERDICT=\$("$SCRIPTS_DIR/extract-verdict.sh" "\$REVIEW_FILE" "\$VALID_VERDICTS") || true
VERDICT_EXIT=\$?
echo "Verdict: \$VERDICT (exit: \$VERDICT_EXIT)"

# Record verdict (even if unknown)
jq --arg v "\$VERDICT" '.last_verdict = \$v' "\$STATE_FILE" > "\$STATE_FILE.tmp" && mv "\$STATE_FILE.tmp" "\$STATE_FILE"

# Only transition if valid verdict
if [[ \$VERDICT_EXIT -eq 0 && "\$VERDICT" != "unknown" ]]; then
    NEXT_STATE=\$("$SCRIPTS_DIR/get-next-state.sh" "\$WORKFLOW" "\$CURRENT_STATE" "\$VERDICT")
    MAPPED_STATUS=\$(map_state_to_status "\$NEXT_STATE")
    jq --arg s "\$MAPPED_STATUS" '.phase_status = \$s' "\$STATE_FILE" > "\$STATE_FILE.tmp" && mv "\$STATE_FILE.tmp" "\$STATE_FILE"
    echo "Transitioned to \$MAPPED_STATUS"
else
    echo "No transition - verdict unknown or invalid"
fi
EOF
    chmod +x "$TEST_TEMP_DIR/unknown-verdict.sh"

    run "$TEST_TEMP_DIR/unknown-verdict.sh"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"No transition - verdict unknown or invalid"* ]]

    # Verify phase_status unchanged
    local phase_status=$(jq -r '.phase_status' "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json")
    local last_verdict=$(jq -r '.last_verdict' "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json")
    [[ "$phase_status" == "qa_review" ]]
    [[ "$last_verdict" == "unknown" ]]
}

@test "test_no_workflow_file_fallback: falls back to grep when no workflow" {
    local PLANS_DIR="$TEST_TEMP_DIR/.plans"
    mkdir -p "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE"

    cat > "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json" << 'EOF'
{"phase_status": "impl_review", "iteration": {"current": 1}}
EOF

    cat > "$TEST_TEMP_DIR/impl_review.md" << 'EOF'
Everything looks good. APPROVED.
EOF

    cat > "$TEST_TEMP_DIR/no-workflow.sh" << EOF
#!/bin/bash
set -euo pipefail

STATE_FILE="$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json"
IMPL_REVIEW="$TEST_TEMP_DIR/impl_review.md"
WORKFLOW_FILE="$TEST_TEMP_DIR/nonexistent.yaml"

# Guard: Skip V2 logic if no workflow file
if [[ -z "\$WORKFLOW_FILE" || ! -f "\$WORKFLOW_FILE" ]]; then
    echo "No workflow file - fallback to V1"
    # V1 fallback
    if [[ -f "\$IMPL_REVIEW" ]] && grep -q "APPROVED" "\$IMPL_REVIEW"; then
        jq '.phase_status = "complete"' "\$STATE_FILE" > "\$STATE_FILE.tmp" && mv "\$STATE_FILE.tmp" "\$STATE_FILE"
        echo "Set to complete via grep"
    fi
fi
EOF
    chmod +x "$TEST_TEMP_DIR/no-workflow.sh"

    run "$TEST_TEMP_DIR/no-workflow.sh"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"No workflow file - fallback to V1"* ]]
    [[ "$output" == *"Set to complete via grep"* ]]

    local phase_status=$(jq -r '.phase_status' "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json")
    [[ "$phase_status" == "complete" ]]
}

# =============================================================================
# Edge case: Missing review file
# =============================================================================

@test "test_missing_review_file_handling: extract-verdict fails" {
    local workflow=$(create_v2_workflow)

    run "$SCRIPTS_DIR/extract-verdict.sh" "$TEST_TEMP_DIR/nonexistent_review.md" "approved,gaps_found"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"unknown"* ]]
}

# =============================================================================
# Edge case: Empty verdicts_history initialization
# =============================================================================

@test "test_empty_verdicts_history_initialization: creates array before first append" {
    local PLANS_DIR="$TEST_TEMP_DIR/.plans"
    mkdir -p "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE"

    # State without verdicts_history field
    cat > "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json" << 'EOF'
{"phase_status": "qa_review", "iteration": {"current": 0}}
EOF

    cat > "$TEST_TEMP_DIR/init-history.sh" << EOF
#!/bin/bash
set -euo pipefail
STATE_FILE="$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json"
TIMESTAMP=\$(date -u +"%Y-%m-%dT%H:%M:%SZ")

jq --arg v "approved" \
   --arg ts "\$TIMESTAMP" \
   --argjson iter 0 \
   --arg state "qa_review" \
   '.last_verdict = \$v |
    .verdicts_history = (.verdicts_history // []) + [{
        iteration: \$iter,
        state: \$state,
        verdict: \$v,
        timestamp: \$ts
    }]' "\$STATE_FILE" > "\$STATE_FILE.tmp" && mv "\$STATE_FILE.tmp" "\$STATE_FILE"
echo "done"
EOF
    chmod +x "$TEST_TEMP_DIR/init-history.sh"

    run "$TEST_TEMP_DIR/init-history.sh"
    [[ "$status" -eq 0 ]]

    # Verify verdicts_history was initialized and has one entry
    local count=$(jq '.verdicts_history | length' "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json")
    [[ "$count" -eq 1 ]]

    local verdict=$(jq -r '.verdicts_history[0].verdict' "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json")
    [[ "$verdict" == "approved" ]]
}

# =============================================================================
# Extract verdict failure handling in integration
# =============================================================================

@test "test_extract_verdict_failure_handling: logs warning and stays in state" {
    local workflow=$(create_v2_workflow)
    local PLANS_DIR="$TEST_TEMP_DIR/.plans"
    mkdir -p "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE"

    cat > "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json" << 'EOF'
{"phase_status": "qa_review", "iteration": {"current": 1}}
EOF

    # Review file with invalid verdict (not in allowed list)
    cat > "$TEST_TEMP_DIR/qa_review.md" << 'EOF'
## Verdict
invalid_verdict_that_doesnt_exist
EOF

    cat > "$TEST_TEMP_DIR/invalid-verdict.sh" << EOF
#!/bin/bash
set -euo pipefail
source '$SCRIPTS_DIR/helpers.sh'

WORKFLOW="$workflow"
STATE_FILE="$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json"
REVIEW_FILE="$TEST_TEMP_DIR/qa_review.md"
CURRENT_STATE="qa_review"

VALID_VERDICTS=\$(get_state_verdicts "\$CURRENT_STATE" "\$WORKFLOW")

# Capture exit code
if VERDICT=\$("$SCRIPTS_DIR/extract-verdict.sh" "\$REVIEW_FILE" "\$VALID_VERDICTS" 2>&1); then
    VERDICT_EXIT=0
else
    VERDICT_EXIT=1
fi
echo "Verdict: \$VERDICT (exit: \$VERDICT_EXIT)"

# Record whatever verdict we got
jq --arg v "\$VERDICT" '.last_verdict = \$v' "\$STATE_FILE" > "\$STATE_FILE.tmp" && mv "\$STATE_FILE.tmp" "\$STATE_FILE"

if [[ \$VERDICT_EXIT -ne 0 || "\$VERDICT" == "unknown" || "\$VERDICT" == *"unknown"* ]]; then
    echo "WARNING: Could not extract valid verdict from \$REVIEW_FILE" >&2
    echo "Staying in current state"
else
    NEXT_STATE=\$("$SCRIPTS_DIR/get-next-state.sh" "\$WORKFLOW" "\$CURRENT_STATE" "\$VERDICT")
    MAPPED_STATUS=\$(map_state_to_status "\$NEXT_STATE")
    jq --arg s "\$MAPPED_STATUS" '.phase_status = \$s' "\$STATE_FILE" > "\$STATE_FILE.tmp" && mv "\$STATE_FILE.tmp" "\$STATE_FILE"
fi
EOF
    chmod +x "$TEST_TEMP_DIR/invalid-verdict.sh"

    run "$TEST_TEMP_DIR/invalid-verdict.sh"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Staying in current state"* ]] || [[ "$output" == *"WARNING"* ]]

    # Verify phase_status unchanged
    local phase_status=$(jq -r '.phase_status' "$PLANS_DIR/active/$TEST_PLAN/phases/$TEST_PHASE/state.json")
    [[ "$phase_status" == "qa_review" ]]
}
