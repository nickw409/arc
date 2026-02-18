#!/usr/bin/env bash
#
# build-context.sh - Build JSON context for template rendering
#
# Merges workflow defaults, variables, state params, state file values,
# and computed values into a single JSON context object.
#
# Usage: build-context.sh <state_file> <workflow_file> <phase_dir> <state_name>
#
# Arguments:
#   state_file    Path to state.json
#   workflow_file Path to workflow.yaml
#   phase_dir     Path to phase directory
#   state_name    Current workflow state name (e.g., "impl", "qa_review")
#
# Output (stdout): JSON object containing merged context
#
# Exit codes:
#   0  Success
#   1  Missing required file or invalid input

set -euo pipefail

# --- Argument Validation ---
if [[ $# -ne 4 ]]; then
    echo "Usage: build-context.sh <state_file> <workflow_file> <phase_dir> <state_name>" >&2
    exit 1
fi

state_file="$1"
workflow_file="$2"
phase_dir="$3"
state_name="$4"

# --- File Validation ---
if [[ ! -f "$state_file" ]]; then
    echo "Error: State file not found: $state_file" >&2
    exit 1
fi

if [[ ! -f "$workflow_file" ]]; then
    echo "Error: Workflow file not found: $workflow_file" >&2
    exit 1
fi

# --- Validate state.json ---
state_json=$(jq '.' "$state_file" 2>/dev/null) || {
    echo "Error: Invalid JSON in state file: $state_file" >&2
    exit 1
}

# --- Project state.json to only fields referenced by templates ---
# Full state.json remains on disk untouched for orchestrator auditing.
state_json_slim=$(echo "$state_json" | jq '{
    iteration: .iteration,
    stuck_iterations: .stuck_iterations,
    tests_passing: .tests_passing,
    tests_total: .tests_total,
    last_verdict: .last_verdict,
    packages: .packages,
    current_model: .current_model,
    plan_md_sent_to: .plan_md_sent_to,
    phase_status: .phase_status,
    current_state: .current_state,
    disputes: .disputes,
    last_cleared_disputes: .last_cleared_disputes
}')

# --- Validate workflow.yaml ---
# Try to parse the workflow YAML; if yq fails, it's invalid
if ! yq '.' "$workflow_file" > /dev/null 2>&1; then
    echo "Error: Invalid YAML in workflow file: $workflow_file" >&2
    exit 1
fi

# --- Extract workflow defaults ---
defaults_json=$(yq -o=json '.defaults // {}' "$workflow_file" 2>/dev/null || echo '{}')
# Ensure it's valid JSON (yq may output "null" for missing sections)
if [[ "$defaults_json" == "null" ]] || ! echo "$defaults_json" | jq '.' > /dev/null 2>&1; then
    defaults_json='{}'
fi

# --- Extract workflow variables ---
variables_json=$(yq -o=json '.variables // {}' "$workflow_file" 2>/dev/null || echo '{}')
if [[ "$variables_json" == "null" ]] || ! echo "$variables_json" | jq '.' > /dev/null 2>&1; then
    variables_json='{}'
fi

# --- Extract state-specific params ---
# Find the state matching state_name and extract its params
params_json=$(yq -o=json "(.states[] | select(.name == \"$state_name\") | .params) // {}" "$workflow_file" 2>/dev/null || echo '{}')
if [[ "$params_json" == "null" ]] || ! echo "$params_json" | jq '.' > /dev/null 2>&1; then
    params_json='{}'
fi

# --- Derive phase and plan names ---
phase_name=$(basename "$phase_dir")
phases_dir=$(dirname "$phase_dir")
plan_dir=$(dirname "$phases_dir")
plan_name=$(basename "$plan_dir")

# --- Read plan.md ---
plan_md_file="$phase_dir/plan.md"
plan_md_content=""
if [[ -f "$plan_md_file" && -s "$plan_md_file" ]]; then
    plan_md_content=$(cat "$plan_md_file")
fi

# Absolute path to plan.md for file reference fallback
plan_file_path="$plan_md_file"

# Check if this state type already received full plan_md.
# Uses underscore-normalized state names to match what iterate.sh writes.
# build-context.sh receives the 4th arg as state_name. In iterate.sh, the callers
# pass: "qa", "impl", "qa_review", "impl_review" (already underscored by iterate.sh).
already_sent="false"
if [[ -f "$phase_dir/state.json" ]]; then
    already_sent=$(jq -r --arg s "$state_name" \
        '(.plan_md_sent_to // []) | if map(. == $s) | any then "true" else "false" end' \
        "$phase_dir/state.json" 2>/dev/null || echo "false")
fi

if [[ "$already_sent" == "true" ]]; then
    plan_md_content=""  # Skip embedding; agent will read from file
fi

# --- Merge everything with jq ---
jq -n \
    --argjson defaults "$defaults_json" \
    --argjson variables "$variables_json" \
    --argjson params "$params_json" \
    --argjson state "$state_json_slim" \
    --arg phase "$phase_name" \
    --arg plan "$plan_name" \
    --arg plan_md "$plan_md_content" \
    --arg plan_file "$plan_file_path" \
    --arg current "$state_name" \
    '
    $defaults +
    $variables +
    {
        params: $params,
        state: $state,
        iteration: (($state.iteration | if type == "object" then .current else . end) // 0),
        current_state: $current,
        phase: $phase,
        plan: $plan,
        plan_md: $plan_md,
        plan_file: $plan_file
    }
    '
