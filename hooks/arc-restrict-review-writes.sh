#!/bin/bash
# arc hook: Restrict review agents to only write their designated review file.
#
# Review agents (qa_review, impl_review) should only be able to write to
# their specific output file, not modify source code or other plan files.
#
# Activated by: REVIEW_AGENT=1 (set by iterate.sh)
# Env: REVIEW_OUTPUT_FILE=path (the specific file this agent may write)
# Trigger: PreToolUse on Write, Edit

# Skip if not a review agent
[[ "$REVIEW_AGENT" != "1" ]] && exit 0

# Parse the file path from tool input
FILE_PATH=$(echo "$CLAUDE_TOOL_INPUT" | python3 -c "import sys,json; print(json.load(sys.stdin).get('file_path',''))" 2>/dev/null)

# If REVIEW_OUTPUT_FILE is set, only allow that exact file
if [[ -n "$REVIEW_OUTPUT_FILE" ]]; then
    if [[ "$FILE_PATH" == "$REVIEW_OUTPUT_FILE" ]]; then
        exit 0
    else
        echo "BLOCKED: Review agent can only write to: $REVIEW_OUTPUT_FILE" >&2
        echo "         Attempted to write to: $FILE_PATH" >&2
        exit 2
    fi
fi

# Fallback: allow any review file in plans directory
if [[ "$FILE_PATH" =~ \.plans/.*/((qa|impl)_review\.md)$ ]]; then
    exit 0
else
    echo "BLOCKED: Review agents can only write to qa_review.md or impl_review.md in plans directory" >&2
    echo "         Attempted to write to: $FILE_PATH" >&2
    exit 2
fi
