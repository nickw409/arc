#!/usr/bin/env bash
#
# auto-split-stuck.sh - Automatically split a stuck phase based on failing tests
#
# This is triggered by the escalation ladder when a phase has been stuck for 5+ iterations.
# It analyzes the failing tests, creates sub-phases, and spawns an agent to generate plans.
#
# Usage: auto-split-stuck.sh <plan> <phase>
# Exit codes:
#   0 = Split successful, new sub-phases created with plans
#   1 = Split not possible or not beneficial
#   2 = Error

set -euo pipefail

ARC_SCRIPTS_DIR="${ARC_SCRIPTS_DIR:-$ARC_HOME/scripts}"
ARC_HOME="${ARC_HOME:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
ARC_PROJECT_ROOT="${ARC_PROJECT_ROOT:-$(pwd)}"
ARC_PLANS_DIR="${ARC_PLANS_DIR:-$ARC_PROJECT_ROOT/.plans}"
ACTIVE_DIR="$ARC_PLANS_DIR/active"

if [[ $# -lt 2 ]]; then
    echo "Usage: $0 <plan> <phase>"
    exit 2
fi

PLAN="$1"
PHASE="$2"
PLAN_DIR="$ACTIVE_DIR/$PLAN"
PHASE_DIR="$PLAN_DIR/phases/$PHASE"
STATE_FILE="$PHASE_DIR/state.json"
LAST_OUTPUT="$PHASE_DIR/last_test_output.txt"

if [[ ! -f "$STATE_FILE" ]]; then
    echo "Error: State file not found: $STATE_FILE"
    exit 2
fi

if [[ ! -f "$LAST_OUTPUT" ]]; then
    echo "Error: No test output found: $LAST_OUTPUT"
    exit 1
fi

# Get current test counts
TESTS_PASSING=$(jq -r '.tests_passing // 0' "$STATE_FILE")
TESTS_TOTAL=$(jq -r '.tests_total // 0' "$STATE_FILE")
TESTS_FAILING=$((TESTS_TOTAL - TESTS_PASSING))

echo "=== Auto-Split Analysis ==="
echo "Phase: $PHASE"
echo "Tests passing: $TESTS_PASSING / $TESTS_TOTAL"
echo "Tests failing: $TESTS_FAILING"

# Don't split if too few tests failing (not worth it)
if [[ "$TESTS_FAILING" -lt 3 ]]; then
    echo "Only $TESTS_FAILING tests failing - not enough to justify split"
    exit 1
fi

# Don't split if most tests failing (problem is systemic, not isolated)
if [[ "$TESTS_TOTAL" -gt 0 ]]; then
    FAIL_PERCENT=$((TESTS_FAILING * 100 / TESTS_TOTAL))
    if [[ "$FAIL_PERCENT" -gt 70 ]]; then
        echo "$FAIL_PERCENT% of tests failing - problem is systemic, split won't help"
        exit 1
    fi
fi

# Extract failing test names
echo ""
echo "Extracting failing tests..."

# Try multiple patterns to find failing tests
FAILING_TESTS=""

# Pattern 1: qa_<phase>::test_name format
FAILING_TESTS=$(grep -oE "qa_[a-zA-Z0-9_-]+::[a-zA-Z_0-9]+" "$LAST_OUTPUT" 2>/dev/null | \
    sort -u | head -30 || true)

# Pattern 2: FAILED ... test_name format
if [[ -z "$FAILING_TESTS" ]]; then
    FAILING_TESTS=$(grep -E "^test .* FAILED" "$LAST_OUTPUT" 2>/dev/null | \
        grep -oE "test [a-zA-Z_0-9]+" | sed 's/test //' | sort -u | head -30 || true)
fi

# Pattern 3: error[E...] in specific files
if [[ -z "$FAILING_TESTS" ]]; then
    FAILING_TESTS=$(grep -oE "error\[E[0-9]+\].*--> [^:]+:[0-9]+" "$LAST_OUTPUT" 2>/dev/null | \
        sed 's/.*--> //' | sed 's/:.*//' | sort -u | head -30 || true)
fi

if [[ -z "$FAILING_TESTS" ]]; then
    echo "Could not identify specific failing tests"
    exit 1
fi

echo "Found failing tests/files:"
echo "$FAILING_TESTS" | head -10
FAIL_COUNT=$(echo "$FAILING_TESTS" | wc -l)
if [[ "$FAIL_COUNT" -gt 10 ]]; then
    echo "... and $((FAIL_COUNT - 10)) more"
fi

# Group failing tests by prefix pattern to determine sub-phases
echo ""
echo "Analyzing test groupings..."

# Extract unique prefixes (e.g., test_bootstrap, test_kernel, test_data)
PREFIXES=$(echo "$FAILING_TESTS" | \
    sed 's/::.*//' | \
    sed 's/_[^_]*$//' | \
    sed 's/_[^_]*$//' | \
    sort | uniq -c | sort -rn | head -5)

echo "Test groupings:"
echo "$PREFIXES"

# Determine sub-phase names based on groupings
# Strategy: Create 2-3 sub-phases: one for passing tests, one or more for failing groups
SUB_PHASES=()

# First sub-phase: preserve passing tests
if [[ "$TESTS_PASSING" -gt 0 ]]; then
    SUB_PHASES+=("${PHASE}-pass")
fi

# Determine failing test sub-phases from prefix analysis
# Take up to 3 most common prefixes
FAIL_GROUPS=$(echo "$PREFIXES" | awk '{print $2}' | head -3)

if [[ -z "$FAIL_GROUPS" ]]; then
    # No clear grouping - just create one "stuck" sub-phase
    SUB_PHASES+=("${PHASE}-stuck")
else
    # Create sub-phase for each group
    for group in $FAIL_GROUPS; do
        # Clean up group name for use as sub-phase
        clean_name=$(echo "$group" | sed 's/^qa_//' | sed 's/^test_//' | tr '_' '-')
        SUB_PHASES+=("${PHASE}-${clean_name}")
    done
fi

# Validate we have sub-phases
if [[ ${#SUB_PHASES[@]} -lt 2 ]]; then
    echo "Could not determine meaningful split (only ${#SUB_PHASES[@]} groups)"
    echo "Manual split recommended"
    exit 1
fi

echo ""
echo "=== Executing Split ==="
echo "Creating sub-phases: ${SUB_PHASES[*]}"

# Call split-phase.sh to create the directory structure
if ! "$ARC_SCRIPTS_DIR/split-phase.sh" "$PLAN" "$PHASE" "${SUB_PHASES[@]}"; then
    echo "Error: split-phase.sh failed"
    exit 2
fi

# Prepare environment for plan-splitter agent
export PLAN_NAME="$PLAN"
export PHASE_NAME="$PHASE"
export SUB_PHASES_LIST=$(IFS=','; echo "${SUB_PHASES[*]}")
export PHASE_DIR="$PHASE_DIR"
export FAILING_TESTS="$FAILING_TESTS"

echo ""
echo "=== Generating Sub-Phase Plans ==="
echo "Spawning plan-splitter agent..."

# Create a prompt file for the agent
PROMPT_FILE=$(mktemp)
cat > "$PROMPT_FILE" << EOF
You are splitting phase "$PHASE" of plan "$PLAN" into sub-phases.

## Environment
- PLAN_NAME: $PLAN
- PHASE_NAME: $PHASE
- SUB_PHASES: $SUB_PHASES_LIST
- PHASE_DIR: $PHASE_DIR

## Failing Tests
$FAILING_TESTS

## Instructions

1. Read the original plan at: $PHASE_DIR/plan.md
2. For each sub-phase, write a focused plan.md:
EOF

for sub in "${SUB_PHASES[@]}"; do
    echo "   - $PLAN_DIR/phases/$sub/plan.md" >> "$PROMPT_FILE"
done

cat >> "$PROMPT_FILE" << EOF

3. Distribute the work logically:
   - ${SUB_PHASES[0]}: $(if [[ "${SUB_PHASES[0]}" == *"-pass"* ]]; then echo "Preserve passing tests, foundational work"; else echo "First group of related tests"; fi)
EOF

for i in $(seq 1 $((${#SUB_PHASES[@]} - 1))); do
    echo "   - ${SUB_PHASES[$i]}: Group $((i+1)) of failing tests" >> "$PROMPT_FILE"
done

cat >> "$PROMPT_FILE" << EOF

4. Each plan.md must have: Objective, Files, Test Cases sections
5. Be specific - copy relevant test names from the failing tests list
6. When done, output: SPLIT_COMPLETE

Begin by reading the original plan.
EOF

# Run the plan-splitter agent
AGENT_TIMEOUT="${AGENT_TIMEOUT:-300}"  # 5 minute timeout for plan generation

echo "Agent prompt:"
cat "$PROMPT_FILE"
echo ""
echo "---"

# Restrict to file operations only - no bash/cargo execution
if timeout "$AGENT_TIMEOUT" claude \
    --disallowedTools "Bash,Task,MultiTurn" \
    --append-system-prompt "You MUST NOT use Bash or run any commands. Only use Read, Write, Edit, Glob, Grep to work with files. Do NOT run cargo, npm, or any build/test commands." \
    -p "$(cat "$PROMPT_FILE")" 2>&1; then
    echo ""
    echo "=== Split Complete ==="
    echo "Created sub-phases:"
    for sub in "${SUB_PHASES[@]}"; do
        if [[ -f "$PLAN_DIR/phases/$sub/plan.md" ]]; then
            echo "  ✓ $sub"
        else
            echo "  ✗ $sub (plan.md missing)"
        fi
    done

    # Clean up
    rm -f "$PROMPT_FILE"

    echo ""
    echo "Next: Orchestrator should continue with first sub-phase: ${SUB_PHASES[0]}"
    exit 0
else
    EXIT_CODE=$?
    echo ""
    echo "Warning: Plan-splitter agent failed or timed out (exit code: $EXIT_CODE)"
    echo "Sub-phase directories created but plans need manual review"

    # Clean up
    rm -f "$PROMPT_FILE"

    # Still return success since directories are created - orchestrator can handle incomplete plans
    exit 0
fi
