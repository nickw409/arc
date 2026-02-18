#!/usr/bin/env bash
# extract-verdict.sh - Extract verdict from review agent output
# Phase: 01-verdict-extraction (orchestration-v2)
#
# Usage: extract-verdict.sh <review_file> <valid_verdicts>
#
# Arguments:
#   review_file     Path to review output file (e.g., qa_review.md, impl_review.md)
#   valid_verdicts  Comma-separated list of valid verdict names (e.g., "approved,gaps_found,concerns")
#
# Output (stdout):
#   On success: The extracted verdict name (lowercase, no whitespace)
#   On failure: "unknown"
#
# Exit codes:
#   0  Verdict found and valid
#   1  No verdict found or verdict not in valid list

set -euo pipefail

# --- Argument validation ---

if [[ $# -lt 2 ]]; then
    echo "Usage: extract-verdict.sh <review_file> <valid_verdicts>" >&2
    exit 1
fi

review_file="$1"
valid_verdicts="$2"

# --- File validation ---

if [[ ! -f "$review_file" ]]; then
    echo "unknown"
    exit 1
fi

if [[ ! -r "$review_file" ]]; then
    echo "unknown"
    exit 1
fi

# --- Read file content and normalize line endings ---

content=$(tr -d '\r' < "$review_file")

# --- Remove code blocks ---
# We need to handle unbalanced code blocks: if odd number of ``` markers,
# treat everything after the last odd marker as inside a code block.

remove_code_blocks() {
    local input="$1"
    local result=""
    local in_code_block=false
    local line

    while IFS= read -r line || [[ -n "$line" ]]; do
        # Check if line starts with ``` (code block marker)
        if [[ "$line" =~ ^'```' ]]; then
            if $in_code_block; then
                in_code_block=false
            else
                in_code_block=true
            fi
            continue
        fi

        if ! $in_code_block; then
            result+="$line"$'\n'
        fi
    done <<< "$input"

    echo "$result"
}

content_no_code_blocks=$(remove_code_blocks "$content")

# --- Find LAST "## Verdict" section ---
# Use grep -n to get line numbers, take the last match
# Note: grep returns 1 when no matches found, so we use || true to avoid exit on no match

verdict_line_num=$(echo "$content_no_code_blocks" | grep -n '^## Verdict$' | tail -1 | cut -d: -f1 || true)

if [[ -z "$verdict_line_num" ]]; then
    echo "unknown"
    exit 1
fi

# --- Extract first non-empty line after the verdict header ---
# Skip blank lines and whitespace-only lines
# Note: grep returns 1 when no matches found, so we use || true

verdict_content=$(echo "$content_no_code_blocks" | tail -n +"$((verdict_line_num + 1))" | grep -v '^[[:space:]]*$' | head -1 || true)

if [[ -z "$verdict_content" ]]; then
    echo "unknown"
    exit 1
fi

# --- Normalize: lowercase, trim whitespace, extract only identifier characters ---
# Verdicts must match ^[a-z][a-z0-9_]*$ (start with letter, then letters/digits/underscores)
# Use sed to:
# 1. Convert to lowercase
# 2. Strip leading whitespace
# 3. Extract only valid identifier characters (stop at first non-identifier char)

normalized=$(echo "$verdict_content" | tr '[:upper:]' '[:lower:]' | sed 's/^[[:space:]]*//' | sed 's/[^a-z0-9_].*//')

# --- Verify identifier format: must start with a letter ---
if [[ ! "$normalized" =~ ^[a-z] ]]; then
    echo "unknown"
    exit 1
fi

# --- Validate against allowed verdicts ---
# Split valid_verdicts by comma and check if normalized verdict is in the list

IFS=',' read -ra verdicts <<< "$valid_verdicts"
for v in "${verdicts[@]}"; do
    # Trim whitespace from each verdict in the list
    v_trimmed=$(echo "$v" | sed 's/^[[:space:]]*//' | sed 's/[[:space:]]*$//')
    if [[ "$normalized" == "$v_trimmed" ]]; then
        echo "$normalized"
        exit 0
    fi
done

# Verdict not in valid list
echo "unknown"
exit 1
