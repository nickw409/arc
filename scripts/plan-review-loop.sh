#!/usr/bin/env bash
#
# plan-review-loop.sh - Run adversarial review committee on a plan (single pass)
#
# Runs all adversaries against the plan, writes review files and a failures
# summary, then exits. The calling agent (with full context) handles fixes.
#
# Usage: plan-review-loop.sh <plan-name> [phase]
#
# Tracks iterations automatically in adversary_history.json.
# Caches passing adversaries when phases haven't changed.
# Auto-approves after 5 iterations.
#
# Exit codes:
#   0 - All required adversaries passed
#   1 - One or more required adversaries failed
#   3 - Max iterations exceeded

set -euo pipefail

# Clean up child processes and temp dir on exit
PARALLEL_TEMP_DIR=""
cleanup() {
    pkill -P $$ 2>/dev/null || true
    [[ -n "$PARALLEL_TEMP_DIR" ]] && rm -rf "$PARALLEL_TEMP_DIR"
}
trap cleanup SIGTERM SIGINT EXIT

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ARC_HOME="${ARC_HOME:-$(dirname "$SCRIPT_DIR")}"
ARC_PROJECT_ROOT="${ARC_PROJECT_ROOT:-$(pwd)}"
ARC_PLANS_DIR="${ARC_PLANS_DIR:-$ARC_PROJECT_ROOT/.plans}"
ACTIVE_DIR="$ARC_PLANS_DIR/active"
ADVERSARIES_FILE="$ARC_HOME/adversaries/planning-adversaries.yaml"

# Read review config from .arc.yaml (with defaults)
ARC_CONFIG="$ARC_PROJECT_ROOT/.arc.yaml"
if [[ -f "$ARC_CONFIG" ]]; then
    REVIEW_MAX_TURNS=$(yq '.review.max_turns // 15' "$ARC_CONFIG")
    REVIEW_TIMEOUT=$(yq '.review.timeout // 300' "$ARC_CONFIG")
else
    REVIEW_MAX_TURNS=15
    REVIEW_TIMEOUT=300
fi

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

error() {
    echo -e "${RED}ERROR: $*${NC}" >&2
    exit 1
}

warn() {
    echo -e "${YELLOW}WARNING: $*${NC}" >&2
}

success() {
    echo -e "${GREEN}$*${NC}"
}

# Check prerequisites
command -v yq &> /dev/null || error "yq is required. Install with: brew install yq"
command -v jq &> /dev/null || error "jq is required. Install with: brew install jq"
command -v claude &> /dev/null || error "claude CLI is required"

# Parse arguments
PLAN_NAME=""
PHASE_FILTER=""
MAX_ITERATIONS=5

while [[ $# -gt 0 ]]; do
    case "$1" in
        -*)
            error "Unknown option: $1"
            ;;
        *)
            if [[ -z "$PLAN_NAME" ]]; then
                PLAN_NAME="$1"
            elif [[ -z "$PHASE_FILTER" ]]; then
                PHASE_FILTER="$1"
            else
                error "Too many arguments"
            fi
            shift
            ;;
    esac
done

if [[ -z "$PLAN_NAME" ]]; then
    echo "Usage: plan-review-loop.sh [options] <plan-name> [phase]"
    echo ""
    echo "Arguments:"
    echo "  plan-name  Name of the plan to review"
    echo "  phase      Optional: specific phase to review (default: all phases)"
    echo ""
    echo "Runs 5 adversaries against the plan in a single pass:"
    echo "  - coverage: Test coverage completeness"
    echo "  - ambiguity: Specification clarity"
    echo "  - scope: Phase size appropriateness"
    echo "  - consistency: Cross-phase alignment"
    echo "  - executability: Can sub-agents actually execute this?"
    echo ""
    echo "Iteration Tracking:"
    echo "  Tracks pass/fail history across runs. Caches passing adversaries"
    echo "  when plan phases haven't changed. Auto-approves after $MAX_ITERATIONS iterations."
    echo "  If a previously passing adversary starts failing, exits with code 2."
    echo ""
    echo "Exit codes:"
    echo "  0 - All required adversaries passed"
    echo "  1 - One or more required adversaries failed"
    echo "  2 - Regression detected (previously passing adversary now fails)"
    echo "  3 - Max iterations ($MAX_ITERATIONS) exceeded (auto-approved)"
    exit 1
fi

PLAN_DIR="$ACTIVE_DIR/$PLAN_NAME"

[[ -d "$PLAN_DIR" ]] || error "Plan '$PLAN_NAME' not found at $PLAN_DIR"
[[ -f "$ADVERSARIES_FILE" ]] || error "Adversaries config not found at $ADVERSARIES_FILE"

# Create reviews directory
REVIEWS_DIR="$PLAN_DIR/reviews"
mkdir -p "$REVIEWS_DIR"

# History file tracks pass/fail across runs for regression detection
HISTORY_FILE="$REVIEWS_DIR/adversary_history.json"

# Initialize history file if it doesn't exist
if [[ ! -f "$HISTORY_FILE" ]]; then
    echo '{"iterations":[],"next_iteration":1}' > "$HISTORY_FILE"
fi

# Get current iteration number from history
ITERATION=$(jq -r '.next_iteration // 1' "$HISTORY_FILE")

# Check max iteration limit
if [[ $ITERATION -gt $MAX_ITERATIONS ]]; then
    echo "=========================================="
    echo -e "${RED}  MAX ITERATIONS EXCEEDED ($MAX_ITERATIONS)${NC}"
    echo "=========================================="
    echo ""
    echo "The plan has been reviewed $MAX_ITERATIONS times without all adversaries passing."
    echo "Approving plan as-is since no more review passes can run."
    echo "Delete reviews/adversary_history.json to start fresh."

    # Approve the plan since we've exhausted review iterations
    if [[ -f "$PLAN_DIR/plan.json" ]]; then
        jq --arg status "approved" \
           --arg reviewed "$(date -Iseconds)" \
           --argjson max_iter "$MAX_ITERATIONS" \
           --argjson iters "$MAX_ITERATIONS" \
           '. + {review_status: $status, reviewed_at: $reviewed, review_iterations: $iters, review_note: "auto-approved after \($max_iter) iterations"}' \
           "$PLAN_DIR/plan.json" > "$PLAN_DIR/plan.json.tmp" && \
           mv "$PLAN_DIR/plan.json.tmp" "$PLAN_DIR/plan.json"
    fi

    exit 3
fi

# Get list of phases to review
if [[ -n "$PHASE_FILTER" ]]; then
    PHASES=("$PHASE_FILTER")
    [[ -d "$PLAN_DIR/phases/$PHASE_FILTER" ]] || error "Phase '$PHASE_FILTER' not found"
else
    PHASES=($(ls "$PLAN_DIR/phases" 2>/dev/null))
fi

[[ ${#PHASES[@]} -gt 0 ]] || error "No phases found in plan"

echo "=========================================="
echo "  Plan Review: $PLAN_NAME (run $ITERATION)"
echo "=========================================="
echo "Phases: ${PHASES[*]}"
echo "Reviews dir: $REVIEWS_DIR"
echo "Adversaries: $(yq '.adversaries[].name' "$ADVERSARIES_FILE" | tr '\n' ' ')"
echo "=========================================="
echo ""

# Extract verdict from adversary output
# Looks for "## Verdict" or "### Verdict" section and extracts the verdict word
extract_verdict() {
    local output_file="$1"
    local valid_verdicts="$2"  # comma-separated list

    # Look for verdict after "## Verdict" or "### Verdict" header
    # IMPORTANT: Use $ anchor to avoid matching headings like "#### Verdict Extraction"
    local verdict
    verdict=$(grep -A2 "^###* Verdict$" "$output_file" 2>/dev/null | \
              grep -v "^###* Verdict" | \
              grep -v "^--" | \
              head -1 | \
              tr -d '[:space:]' | \
              tr '[:upper:]' '[:lower:]')

    # Validate against allowed verdicts
    # Use -- to prevent $verdict from being interpreted as grep options
    if echo "$valid_verdicts" | grep -qw -- "$verdict"; then
        echo "$verdict"
    else
        echo "unknown"
    fi
}

# Run a single adversary agent
run_adversary() {
    local adversary_name="$1"
    local plan_content="$2"
    local output_file="$3"

    local prompt_path
    prompt_path=$(yq ".adversaries[] | select(.name == \"$adversary_name\") | .prompt" "$ADVERSARIES_FILE")
    local prompt_file="$ARC_HOME/$prompt_path"

    echo "" >&2
    echo "[DEBUG] Adversary: $adversary_name" >&2
    echo "[DEBUG] Prompt file: $prompt_file" >&2
    echo "[DEBUG] Output file: $output_file" >&2

    [[ -f "$prompt_file" ]] || error "Adversary prompt not found: $prompt_file"

    # Build full prompt with plan context
    local full_prompt
    full_prompt=$(cat "$prompt_file")
    full_prompt+=$'\n\n---\n\n# Plan Under Review\n\n'
    full_prompt+="$plan_content"

    local prompt_length=${#full_prompt}
    echo "[DEBUG] Prompt length: $prompt_length chars" >&2
    echo "[DEBUG] Starting Claude agent (timeout: ${REVIEW_TIMEOUT}s, max-turns: $REVIEW_MAX_TURNS)..." >&2

    # Run Claude with the adversary prompt
    # IMPORTANT: Restrict to read-only tools via allowlist - adversaries analyze, not execute
    local start_time
    start_time=$(date +%s)

    # Write prompt to temp file and pipe via stdin.
    # IMPORTANT: The -p flag (positional prompt) hangs indefinitely when claude
    # is spawned from within a parent Claude Code session. Stdin piping works.
    local prompt_file_tmp
    prompt_file_tmp=$(mktemp /tmp/adversary-prompt-XXXXXX.txt)
    echo "$full_prompt" > "$prompt_file_tmp"

    # Capture stderr separately for error diagnostics
    local stderr_file="${output_file%.md}.stderr"
    if ! timeout "$REVIEW_TIMEOUT" claude --print --output-format text \
        --max-turns "$REVIEW_MAX_TURNS" \
        --tools "Read,Glob,Grep" \
        --append-system-prompt "CRITICAL: You are a read-only reviewer. You MUST NOT use Bash, Write, Edit, or any tool that modifies files or runs commands. Only use Read, Glob, Grep to examine files. Do NOT run cargo, npm, or any build/test commands. All plan content is provided in the prompt - do NOT use tools to read plan files. Produce your analysis and verdict directly from the provided content." \
        < "$prompt_file_tmp" > "$output_file" 2>"$stderr_file"; then
        rm -f "$prompt_file_tmp"
        local end_time
        end_time=$(date +%s)
        local duration=$((end_time - start_time))
        local exit_code=$?
        echo "[DEBUG] Agent failed after ${duration}s (exit code: $exit_code)" >&2
        if [[ -s "$stderr_file" ]]; then
            echo "[DEBUG] stderr:" >&2
            head -20 "$stderr_file" >&2
        fi
        # Preserve any partial output for diagnostics
        local partial_output=""
        [[ -s "$output_file" ]] && partial_output=$(cat "$output_file")
        {
            echo "AGENT_ERROR (exit=$exit_code, duration=${duration}s)"
            echo ""
            if [[ -s "$stderr_file" ]]; then
                echo "## stderr"
                echo '```'
                cat "$stderr_file"
                echo '```'
                echo ""
            fi
            if [[ -n "$partial_output" ]]; then
                echo "## Partial Output"
                echo "$partial_output"
            fi
        } > "$output_file"
        rm -f "$stderr_file"
        return 1
    fi

    rm -f "$prompt_file_tmp" "$stderr_file"

    local end_time
    end_time=$(date +%s)
    local duration=$((end_time - start_time))
    echo "[DEBUG] Agent completed in ${duration}s" >&2

    local output_size
    output_size=$(wc -c < "$output_file")
    echo "[DEBUG] Output size: $output_size bytes" >&2

    return 0
}

# Collect all plan.md content for review
collect_plan_content() {
    local plan_dir="$1"
    local phases=("${@:2}")

    echo "# Plan: $(basename "$plan_dir")"
    echo ""

    for phase in "${phases[@]}"; do
        local plan_file="$plan_dir/phases/$phase/plan.md"
        if [[ -f "$plan_file" ]]; then
            echo "---"
            echo ""
            echo "## Phase: $phase"
            echo ""
            cat "$plan_file"
            echo ""
        fi
    done
}

# Compute SHA256 hashes of each phase's plan.md file
compute_phase_hashes() {
    local plan_dir="$1"; shift
    # remaining args are phase names

    local json_args=()
    for phase in "$@"; do
        local plan_file="$plan_dir/phases/$phase/plan.md"
        if [[ ! -f "$plan_file" ]]; then
            json_args+=(--arg "$phase" "missing")
        else
            local hash
            hash=$(sha256sum "$plan_file" | cut -d' ' -f1) || exit 1
            json_args+=(--arg "$phase" "$hash")
        fi
    done

    if [[ ${#json_args[@]} -eq 0 ]]; then
        echo '{}'
        return 0
    fi

    # Build JSON object from args using jq
    # Each --arg pair becomes a key-value in the object
    jq -n "${json_args[@]}" '
        [
            $ARGS.named | to_entries[]
        ] | from_entries
    '
}

# Compare current hashes against the hashes recorded in the previous iteration
get_changed_phases() {
    local current_hashes="$1"
    local history_file="$2"
    local prev_iteration="$3"

    # Get old hashes from history; on any error treat as no old hashes
    local old_hashes
    old_hashes=$(jq -r ".iterations[] | select(.iteration == $prev_iteration) | .phase_hashes // empty" "$history_file" 2>/dev/null) || old_hashes=""

    if [[ -z "$old_hashes" || "$old_hashes" == "null" ]]; then
        # No previous hashes — all phases are changed
        echo "$current_hashes" | jq -r 'keys[]' | tr '\n' ' ' | sed 's/ $//'
        return 0
    fi

    # Compare each key in current_hashes against old_hashes
    local changed
    changed=$(jq -r -n --argjson current "$current_hashes" --argjson old "$old_hashes" '
        [$current | to_entries[] | select(.value != ($old[.key] // "")) | .key] | join(" ")
    ')

    echo "$changed"
}

# Look up the verdict for a specific adversary in a specific iteration
get_prev_verdict() {
    local history_file="$1"
    local iteration="$2"
    local adversary="$3"

    local verdict
    verdict=$(jq -r ".iterations[] | select(.iteration == $iteration) | .results[\"$adversary\"] // \"none\"" "$history_file" 2>/dev/null) || verdict="none"

    if [[ -z "$verdict" || "$verdict" == "null" ]]; then
        verdict="none"
    fi

    echo "$verdict"
}

# Collect plan content
echo "[DEBUG] Collecting plan content..."
# Full plan content for cross-phase adversaries (always the complete concatenation)
PLAN_CONTENT_FULL=$(collect_plan_content "$PLAN_DIR" "${PHASES[@]}")
PLAN_CONTENT_LENGTH=${#PLAN_CONTENT_FULL}
echo "[DEBUG] Plan content collected: $PLAN_CONTENT_LENGTH chars"
echo ""

# Compute phase hashes for skip logic
CURRENT_HASHES=$(compute_phase_hashes "$PLAN_DIR" "${PHASES[@]}")

# Determine which phases changed since last iteration
CHANGED_PHASES=("${PHASES[@]}")  # default: all phases
if [[ $ITERATION -ge 2 ]]; then
    prev=$((ITERATION - 1))
    changed_output=$(get_changed_phases "$CURRENT_HASHES" "$HISTORY_FILE" "$prev")
    if [[ -n "$changed_output" ]]; then
        mapfile -t CHANGED_PHASES < <(echo "$changed_output" | tr ' ' '\n')
    else
        CHANGED_PHASES=()  # Truly empty — no phases changed
    fi
fi

# Safety valve: force full re-review every 3rd iteration
if [[ $((ITERATION % 3)) -eq 0 ]]; then
    CHANGED_PHASES=("${PHASES[@]}")
fi

# Changed-only plan content for per-phase adversaries (iteration 2+ only)
# Falls back to full content when all phases changed or on iteration 1
PLAN_CONTENT_CHANGED="$PLAN_CONTENT_FULL"  # default
if [[ $ITERATION -ge 2 && ${#CHANGED_PHASES[@]} -gt 0 && ${#CHANGED_PHASES[@]} -lt ${#PHASES[@]} ]]; then
    PLAN_CONTENT_CHANGED=$(collect_plan_content "$PLAN_DIR" "${CHANGED_PHASES[@]}")
fi

# Run all adversaries in parallel
RESULTS=""
REQUIRED_FAILED=false
ALL_PASSED=true

ADVERSARY_NAMES=($(yq '.adversaries[].name' "$ADVERSARIES_FILE"))
echo "Launching ${#ADVERSARY_NAMES[@]} adversaries in parallel..."
echo ""

PARALLEL_TEMP_DIR=$(mktemp -d /tmp/plan-review-XXXXXX)
PIDS=()

for adversary in "${ADVERSARY_NAMES[@]}"; do
    (
        review_file="$REVIEWS_DIR/iteration_${ITERATION}_${adversary}.md"
        result_file="$PARALLEL_TEMP_DIR/${adversary}.result"

        # Read cross_phase field for this adversary (used by per-phase-content phase)
        is_cross_phase=$(yq ".adversaries[] | select(.name == \"$adversary\") | .cross_phase // false" "$ADVERSARIES_FILE")

        # Skip logic: check if we can reuse cached result
        can_skip=false
        if [[ $ITERATION -ge 2 ]]; then
            prev=$((ITERATION - 1))
            prev_verdict=$(get_prev_verdict "$HISTORY_FILE" "$prev" "$adversary")
            if [[ "$prev_verdict" == "passed" ]] && [[ ${#CHANGED_PHASES[@]} -eq 0 ]]; then
                # Check that the previous review file exists for copying
                source_review="$REVIEWS_DIR/iteration_${prev}_${adversary}.md"
                if [[ -f "$source_review" ]]; then
                    can_skip=true
                fi
            fi
        fi

        if [[ "$can_skip" == "true" ]]; then
            # Copy cached review file and write cached result
            cp "$source_review" "$review_file"
            echo "cached:$prev_verdict" > "$result_file"
        else
            # Select plan input based on cross_phase field
            # is_cross_phase is set earlier in this subshell by the skip logic from adversary-skip-cache
            if [[ "$is_cross_phase" == "true" || ${#CHANGED_PHASES[@]} -eq ${#PHASES[@]} ]]; then
                plan_input="$PLAN_CONTENT_FULL"
            else
                plan_input="$PLAN_CONTENT_CHANGED"
            fi

            # Prepend context note for partial reviews
            if [[ "$plan_input" != "$PLAN_CONTENT_FULL" ]]; then
                plan_input="NOTE: Only the following phases changed since the last review iteration. Previously unchanged phases passed this adversary.

$plan_input"
            fi

            if run_adversary "$adversary" "$plan_input" "$review_file" 2>"$PARALLEL_TEMP_DIR/${adversary}.log"; then
                pass_verdict=$(yq ".adversaries[] | select(.name == \"$adversary\") | .verdicts.pass" "$ADVERSARIES_FILE")
                all_verdicts=$(yq ".adversaries[] | select(.name == \"$adversary\") | .verdicts | to_entries | .[].value" "$ADVERSARIES_FILE" | tr '\n' ',')
                verdict=$(extract_verdict "$review_file" "$all_verdicts")

                if [[ "$verdict" == "$pass_verdict" ]]; then
                    echo "passed:$verdict" > "$result_file"
                else
                    required=$(yq ".adversaries[] | select(.name == \"$adversary\") | .required" "$ADVERSARIES_FILE")
                    if [[ "$required" == "true" ]]; then
                        echo "failed:$verdict" > "$result_file"
                    else
                        echo "warning:$verdict" > "$result_file"
                    fi
                fi
            else
                echo "error:agent_error" > "$result_file"
            fi
        fi
    ) &
    PIDS+=($!)
done

# Wait for all adversaries to complete
for pid in "${PIDS[@]}"; do
    wait "$pid" 2>/dev/null || true
done

# Collect and display results
for adversary in "${ADVERSARY_NAMES[@]}"; do
    result_file="$PARALLEL_TEMP_DIR/${adversary}.result"
    if [[ -f "$result_file" ]]; then
        IFS=: read -r status verdict < "$result_file"
        case "$status" in
            passed)
                success "  $adversary: PASSED ($verdict)"
                RESULTS+="$adversary:passed "
                ;;
            failed)
                echo -e "  ${RED}$adversary: FAILED ($verdict)${NC}"
                RESULTS+="$adversary:failed "
                REQUIRED_FAILED=true
                ALL_PASSED=false
                ;;
            warning)
                warn "  $adversary: WARNING ($verdict)"
                RESULTS+="$adversary:warning "
                ALL_PASSED=false
                ;;
            cached)
                success "  $adversary: SKIPPED (cached: $verdict)"
                RESULTS+="$adversary:passed "
                ;;
            *)
                echo -e "  ${RED}$adversary: AGENT ERROR${NC}"
                RESULTS+="$adversary:error "
                REQUIRED_FAILED=true
                ALL_PASSED=false
                ;;
        esac
    else
        echo -e "  ${RED}$adversary: NO RESULT${NC}"
        RESULTS+="$adversary:error "
        REQUIRED_FAILED=true
        ALL_PASSED=false
    fi
done

rm -rf "$PARALLEL_TEMP_DIR"
PARALLEL_TEMP_DIR=""

echo ""

# Build JSON object from space-separated "adversary:status" pairs
build_results_json() {
    local results="$*"
    local json="{"
    local first=true
    for result in $results; do
        local adversary="${result%%:*}"
        local status="${result##*:}"
        if $first; then
            first=false
        else
            json+=","
        fi
        json+="\"$adversary\":\"$status\""
    done
    json+="}"
    echo "$json"
}

# Record results to history
record_results() {
    local iteration="$1"
    local phase_hashes="$2"
    shift 2
    local results="$*"  # space-separated "adversary:status" pairs

    local iter_json="{\"iteration\":$iteration,\"phase_hashes\":null,\"results\":{"
    local first=true
    for result in $results; do
        local adversary="${result%%:*}"
        local status="${result##*:}"
        if $first; then
            first=false
        else
            iter_json+=","
        fi
        iter_json+="\"$adversary\":\"$status\""
    done
    iter_json+="}}"

    # Replace null with actual phase_hashes JSON via jq
    local next=$((iteration + 1))
    jq --argjson iter "$iter_json" --argjson phase_hashes "$phase_hashes" --argjson next "$next" \
        '.iterations += [$iter | .phase_hashes = $phase_hashes] | .next_iteration = $next' "$HISTORY_FILE" > "$HISTORY_FILE.tmp" && \
        mv "$HISTORY_FILE.tmp" "$HISTORY_FILE"
}

record_results "$ITERATION" "$CURRENT_HASHES" $RESULTS

# Check for regression against previous run
if [[ $ITERATION -ge 2 ]]; then
    prev_iteration=$((ITERATION - 1))

    prev_passed=$(jq -r ".iterations[] | select(.iteration == $prev_iteration) | .results | to_entries[] | select(.value == \"passed\") | .key" "$HISTORY_FILE" 2>/dev/null)
    curr_failed=$(jq -r ".iterations[] | select(.iteration == $ITERATION) | .results | to_entries[] | select(.value == \"failed\" or .value == \"error\") | .key" "$HISTORY_FILE" 2>/dev/null)

    regressed=""
    for prev in $prev_passed; do
        for curr in $curr_failed; do
            if [[ "$prev" == "$curr" ]]; then
                regressed+="$prev "
            fi
        done
    done

    if [[ -n "$regressed" ]]; then
        echo "=========================================="
        echo -e "${RED}  REGRESSION DETECTED${NC}"
        echo "=========================================="
        echo ""
        echo "These adversaries PASSED in run $prev_iteration but now FAIL:"
        for adv in $regressed; do
            echo "  - $adv"
        done
        echo ""

        # Write regression report
        REGRESSION_REPORT="$REVIEWS_DIR/regression_report.md"
        {
            echo "# Regression Report - Run $ITERATION"
            echo ""
            for adv in $regressed; do
                prev_file="$REVIEWS_DIR/iteration_${prev_iteration}_${adv}.md"
                curr_file="$REVIEWS_DIR/iteration_${ITERATION}_${adv}.md"

                echo "## $adv"
                echo ""
                echo "### BEFORE (run $prev_iteration - PASSED)"
                echo ""
                if [[ -f "$prev_file" ]]; then
                    cat "$prev_file"
                else
                    echo "_Previous review not found_"
                fi
                echo ""
                echo "### AFTER (run $ITERATION - FAILED)"
                echo ""
                if [[ -f "$curr_file" ]]; then
                    cat "$curr_file"
                else
                    echo "_Current review not found_"
                fi
                echo ""
                echo "---"
                echo ""
            done
        } > "$REGRESSION_REPORT"

        echo "Regression report: $REGRESSION_REPORT"

        if [[ -f "$PLAN_DIR/plan.json" ]]; then
            RESULTS_JSON=$(build_results_json $RESULTS)
            jq --arg status "regression_detected" \
               --arg reviewed "$(date -Iseconds)" \
               --argjson iters "$ITERATION" \
               --argjson results "$RESULTS_JSON" \
               '. + {review_status: $status, reviewed_at: $reviewed, review_iterations: $iters, review_results: $results}' \
               "$PLAN_DIR/plan.json" > "$PLAN_DIR/plan.json.tmp" && \
               mv "$PLAN_DIR/plan.json.tmp" "$PLAN_DIR/plan.json"
        fi

        exit 2
    fi
fi

# Write failures summary
if ! $ALL_PASSED; then
    FAILURES_FILE="$REVIEWS_DIR/iteration_${ITERATION}_failures.md"
    {
        echo "# Adversary Review Failures"
        echo ""
        echo "The following adversaries found issues that must be addressed:"
        echo ""

        for adversary in $(yq '.adversaries[].name' "$ADVERSARIES_FILE"); do
            pass_verdict=$(yq ".adversaries[] | select(.name == \"$adversary\") | .verdicts.pass" "$ADVERSARIES_FILE")
            all_verdicts=$(yq ".adversaries[] | select(.name == \"$adversary\") | .verdicts | to_entries | .[].value" "$ADVERSARIES_FILE" | tr '\n' ',')

            review_file="$REVIEWS_DIR/iteration_${ITERATION}_${adversary}.md"

            if [[ -f "$review_file" ]]; then
                verdict=$(extract_verdict "$review_file" "$all_verdicts")

                if [[ "$verdict" != "$pass_verdict" ]]; then
                    echo "## $adversary Adversary (FAILED: $verdict)"
                    echo ""
                    cat "$review_file"
                    echo ""
                    echo "---"
                    echo ""
                fi
            fi
        done
    } > "$FAILURES_FILE"
fi

# Update plan status
if [[ -f "$PLAN_DIR/plan.json" ]]; then
    local_status="approved"
    if $REQUIRED_FAILED; then
        local_status="needs_review"
    elif ! $ALL_PASSED; then
        local_status="conditional"
    fi

    RESULTS_JSON=$(build_results_json $RESULTS)
    jq --arg status "$local_status" \
       --arg reviewed "$(date -Iseconds)" \
       --argjson iters "$ITERATION" \
       --argjson results "$RESULTS_JSON" \
       '. + {review_status: $status, reviewed_at: $reviewed, review_iterations: $iters, review_results: $results}' \
       "$PLAN_DIR/plan.json" > "$PLAN_DIR/plan.json.tmp" && \
       mv "$PLAN_DIR/plan.json.tmp" "$PLAN_DIR/plan.json"
fi

# Final summary
if $ALL_PASSED; then
    echo "=========================================="
    success "  ALL ADVERSARIES PASSED"
    echo "=========================================="
    echo ""
    echo "Plan approved. Reviews saved to: $REVIEWS_DIR"
    exit 0
elif ! $REQUIRED_FAILED; then
    echo "=========================================="
    warn "  WARNINGS ONLY (non-required adversaries failed)"
    echo "=========================================="
    echo ""
    echo "Plan conditionally approved. Reviews saved to: $REVIEWS_DIR"
    exit 0
else
    echo "=========================================="
    echo -e "${RED}  REQUIRED ADVERSARIES FAILED${NC}"
    echo "=========================================="
    echo ""
    echo "Reviews: $REVIEWS_DIR"
    echo "Failures summary: $REVIEWS_DIR/iteration_${ITERATION}_failures.md"
    exit 1
fi
