#!/bin/bash
# arc hook: Block direct file writes when in orchestrator mode.
#
# The orchestrator agent should only modify files through approved scripts
# or by spawning sub-agents via iterate.sh.
#
# Activated by: ORCHESTRATOR_MODE=1 (set by run-orchestrator.sh)
# Trigger: PreToolUse on Bash, Write, Edit

[[ -z "${ARC_HOME:-}" ]] && exit 0
[[ "$ORCHESTRATOR_MODE" != "1" ]] && exit 0

TOOL_NAME="${CLAUDE_TOOL_NAME:-}"

# Block Write and Edit tools entirely
if [[ "$TOOL_NAME" == "Write" || "$TOOL_NAME" == "Edit" ]]; then
    echo "BLOCKED: Orchestrator cannot write files directly."
    echo "         Spawn sub-agents via iterate.sh instead."
    exit 2
fi

# For Bash, block shell write operations but allow arc scripts and git
if [[ "$TOOL_NAME" == "Bash" ]]; then
    COMMAND=$(echo "$CLAUDE_TOOL_INPUT" | python3 -c "import sys,json; print(json.load(sys.stdin).get('command',''))" 2>/dev/null)

    # Allow arc scripts
    if echo "$COMMAND" | grep -qE '(^|&&|\|\||;)[[:space:]]*((\$ARC_HOME|ARC_HOME)/scripts/|arc )'; then
        exit 0
    fi

    # Allow git operations
    if echo "$COMMAND" | grep -qE '(^|&&|\|\||;)[[:space:]]*git[[:space:]]'; then
        exit 0
    fi

    # Block shell write operations
    if echo "$COMMAND" | grep -qE '(>|>>|\btee\b|\btouch\b|\bmkdir\b|\bcp\b|\bmv\b|\brm\b|\bdd\b|\bsed[[:space:]]+-i|\bchmod\b|\bchown\b)'; then
        echo "BLOCKED: Orchestrator cannot write files directly."
        echo "         Use arc scripts or spawn sub-agents via iterate.sh."
        exit 2
    fi
fi

exit 0
