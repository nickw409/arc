#!/usr/bin/env bash
# Runs a single approach (arc, claude-minimal, claude-plan) against a task.
# Usage: run-approach.sh <task> <approach> <run-number>
# Captures: wall time, exit code, token usage, full output log

set -euo pipefail
source "$(dirname "$0")/config.sh"

TASK="$1"
APPROACH="$2"
RUN_NUM="$3"

# Setup fresh working directory
RUN_DIR=$("$(dirname "$0")/setup-workdir.sh" "$TASK" "$APPROACH" "$RUN_NUM")
PROJECT_DIR="$RUN_DIR/tkit"
LOG_DIR="$RUN_DIR/logs"
mkdir -p "$LOG_DIR"

SPEC="$TASKS_DIR/$TASK/spec.md"
PLAN="$TASKS_DIR/$TASK/plan.md"

echo "=== Run: $TASK / $APPROACH / run-$RUN_NUM ==="
echo "  workdir: $RUN_DIR"
echo "  project: $PROJECT_DIR"

# Build the prompt based on approach
build_prompt() {
    local prompt=""

    prompt+="You are working in the Go project at $PROJECT_DIR.\n"
    prompt+="Do NOT ask for clarification. Complete the task fully and autonomously.\n"
    prompt+="Run tests after making changes to verify your work.\n\n"

    prompt+="## Task Specification\n\n"
    prompt+="$(cat "$SPEC")\n"

    if [[ "$APPROACH" == "claude-plan" ]]; then
        prompt+="\n\n## Implementation Plan\n\n"
        prompt+="$(cat "$PLAN")\n"
    fi

    echo -e "$prompt"
}

PROMPT=$(build_prompt)

# Record start time
START_TIME=$(date +%s%N)

# Run the approach
EXIT_CODE=0
case "$APPROACH" in
    arc)
        echo "  [arc] Setting up arc plan..."

        # Determine phases and workflow type for this task
        ARC_PHASES_DIR="$TASKS_DIR/$TASK/arc-phases"
        case "$TASK" in
            task1-feature)
                PHASES=(model store filter cli)
                WORKFLOW="feature"
                ;;
            task2-bugfix)
                PHASES=(investigate fix)
                WORKFLOW="bugfix"
                ;;
            task3-refactor)
                PHASES=(characterize refactor)
                WORKFLOW="refactor"
                ;;
            task4-investigation)
                PHASES=(baseline optimize)
                WORKFLOW="performance"
                ;;
        esac

        PLAN_NAME="bench-${TASK}"

        # Initialize arc in the workdir (git already initialized by setup-workdir.sh)
        cd "$PROJECT_DIR"
        "$ARC_BIN" init 2>/dev/null || true

        # Update .arc.yaml with correct settings for Go
        cat > "$PROJECT_DIR/.arc.yaml" <<'ARCEOF'
language: go
runner: go
default_package: ""
build_command: "go build ./..."
test_command: "go test ./..."
git:
    commit_style: conventional
    sign: false
    co_author: false
audit:
    prompt: ""
ARCEOF

        # Scaffold the plan with arc plan command
        "$ARC_BIN" plan --type "$WORKFLOW" "$PLAN_NAME" "${PHASES[@]}"

        # Copy our pre-written phase plans into the scaffolded directories
        for phase in "${PHASES[@]}"; do
            SRC="$ARC_PHASES_DIR/$phase/plan.md"
            DEST="$PROJECT_DIR/.plans/active/$PLAN_NAME/phases/$phase/plan.md"
            if [[ -f "$SRC" ]]; then
                cp "$SRC" "$DEST"
                echo "  [arc] Copied phase plan: $phase"
            else
                echo "  [arc] WARNING: Missing phase plan: $SRC"
            fi
        done

        # Mark plan as reviewed (plans are pre-reviewed; skip arc review during bench)
        PLAN_JSON="$PROJECT_DIR/.plans/active/$PLAN_NAME/plan.json"
        jq '.review_status = "approved"' "$PLAN_JSON" > "${PLAN_JSON}.tmp" \
            && mv "${PLAN_JSON}.tmp" "$PLAN_JSON"

        # Stage arc config so the agent can see it
        git add -A && git commit -q -m "arc plan setup"

        # Run arc orchestrator
        timeout "$RUN_TIMEOUT" "$ARC_BIN" run "$PLAN_NAME" \
            > "$LOG_DIR/stdout.log" 2> "$LOG_DIR/stderr.log" || EXIT_CODE=$?
        ;;

    claude-minimal|claude-plan)
        # Run claude code with the prompt
        cd "$PROJECT_DIR"
        echo "$PROMPT" | timeout "$RUN_TIMEOUT" "$CLAUDE_BIN" \
            --print \
            --output-format json \
            --max-turns 50 \
            --allowedTools "Bash,Read,Write,Edit,Glob,Grep" \
            > "$LOG_DIR/stdout.log" 2> "$LOG_DIR/stderr.log" || EXIT_CODE=$?
        ;;
esac

# Record end time
END_TIME=$(date +%s%N)
ELAPSED_MS=$(( (END_TIME - START_TIME) / 1000000 ))

echo "  elapsed: ${ELAPSED_MS}ms, exit: $EXIT_CODE"

# Save run metadata
cat > "$RUN_DIR/run-meta.json" <<EOF
{
  "task": "$TASK",
  "approach": "$APPROACH",
  "run": $RUN_NUM,
  "elapsed_ms": $ELAPSED_MS,
  "exit_code": $EXIT_CODE,
  "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "project_dir": "$PROJECT_DIR"
}
EOF

echo "  meta: $RUN_DIR/run-meta.json"
echo "$RUN_DIR"
