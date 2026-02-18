#!/usr/bin/env bash
#
# monitor-orchestrator.sh - Monitor orchestrator progress
#
# Usage: monitor-orchestrator.sh [plan-name] [--interval N]
#        Auto-detects active plan if not specified
#        Default interval is 5 seconds

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ARC_HOME="${ARC_HOME:-$(dirname "$SCRIPT_DIR")}"
ARC_PROJECT_ROOT="${ARC_PROJECT_ROOT:-$(pwd)}"
ARC_PLANS_DIR="${ARC_PLANS_DIR:-$ARC_PROJECT_ROOT/.plans}"
PLANS_DIR="$ARC_PLANS_DIR"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
MAGENTA='\033[0;35m'
BOLD='\033[1m'
DIM='\033[2m'
NC='\033[0m'

# Parse arguments
PLAN_NAME=""
INTERVAL=5
OUTPUT_LINES=8

while [[ $# -gt 0 ]]; do
    case "$1" in
        --interval|-i)
            INTERVAL="$2"
            shift 2
            ;;
        --output-lines|-o)
            OUTPUT_LINES="$2"
            shift 2
            ;;
        -*)
            echo "Unknown option: $1" >&2
            exit 1
            ;;
        *)
            PLAN_NAME="$1"
            shift
            ;;
    esac
done

# Check if orchestrator is actively running for a plan (via lock file)
check_active_orchestrator() {
    local plan_dir="$1"
    local lock_file="$plan_dir/.orchestrator.lock"
    [[ -f "$lock_file" ]] || return 1
    local pid=$(cat "$lock_file" 2>/dev/null)
    [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null
}

# Auto-detect active plan if not specified
if [[ -z "$PLAN_NAME" ]]; then
    for plan_dir in "$PLANS_DIR/active"/*/; do
        [[ -d "$plan_dir" ]] || continue
        if check_active_orchestrator "$plan_dir"; then
            PLAN_NAME=$(basename "$plan_dir")
            break
        fi
    done

    if [[ -z "$PLAN_NAME" ]]; then
        PLANS=($(ls "$PLANS_DIR/active" 2>/dev/null || true))
        if [[ ${#PLANS[@]} -eq 0 ]]; then
            echo "No plans found in $PLANS_DIR/active"
            exit 1
        fi
        echo "No active orchestration detected. Select a plan to monitor:"
        echo ""
        select PLAN_NAME in "${PLANS[@]}" "Cancel"; do
            [[ "$PLAN_NAME" == "Cancel" ]] && exit 0
            [[ -n "$PLAN_NAME" ]] && break
        done
    fi
fi

PLAN_DIR="$PLANS_DIR/active/$PLAN_NAME"
[[ -d "$PLAN_DIR" ]] || { echo "Plan not found: $PLAN_NAME" >&2; exit 1; }
command -v jq &>/dev/null || { echo "jq required" >&2; exit 1; }

# Get list of phases in execution order (from phase_order map, fallback to array order, fallback to ls)
if [[ -f "$PLAN_DIR/plan.json" ]]; then
    PHASES=($(jq -r 'if .phase_order then
        [.phases[] as $p | {n: $p, o: .phase_order[$p]}] | sort_by(.o) | .[].n
      else .phases[] end' "$PLAN_DIR/plan.json" 2>/dev/null))
else
    PHASES=($(ls "$PLAN_DIR/phases" 2>/dev/null))
fi
PHASE_COUNT=${#PHASES[@]}

# Helper functions
format_duration() {
    local secs=$1
    if [[ $secs -lt 60 ]]; then
        printf "%ds" "$secs"
    elif [[ $secs -lt 3600 ]]; then
        printf "%dm %ds" "$((secs / 60))" "$((secs % 60))"
    else
        printf "%dh %dm" "$((secs / 3600))" "$((secs % 3600 / 60))"
    fi
}

time_ago() {
    local file="$1"
    [[ -f "$file" ]] || { echo "n/a"; return; }
    local mtime now
    mtime=$(stat -c %Y "$file" 2>/dev/null || stat -f %m "$file" 2>/dev/null)
    now=$(date +%s)
    format_duration $((now - mtime))
}

get_agent_type() {
    case "$1" in
        qa)          echo "test-writer" ;;
        qa_review)   echo "qa-reviewer" ;;
        impl|implementing) echo "impl (claude)" ;;
        impl_review) echo "impl-reviewer" ;;
        fix)         echo "impl (claude)" ;;
        blocked)     echo "none (blocked)" ;;
        complete)    echo "none (done)" ;;
        pending)     echo "none (waiting)" ;;
        *)           echo "unknown" ;;
    esac
}

get_mode_desc() {
    case "$1" in
        qa)          echo "Writing Tests" ;;
        qa_review)   echo "Reviewing Tests" ;;
        impl|implementing) echo "Implementing" ;;
        impl_review) echo "Reviewing Implementation" ;;
        fix)         echo "Fixing Issues" ;;
        blocked)     echo "Blocked" ;;
        complete)    echo "Complete" ;;
        pending)     echo "Pending" ;;
        *)           echo "$1" ;;
    esac
}

# Line cache for change detection
declare -A LINE_CACHE

# Update a line only if content changed
update_line() {
    local line_num=$1
    local content=$2
    local cache_key="line_$line_num"

    if [[ "${LINE_CACHE[$cache_key]:-}" != "$content" ]]; then
        printf '\033[%d;1H%b\033[K' "$line_num" "$content"
        LINE_CACHE[$cache_key]="$content"
    fi
}

# Clear a range of lines and remove from cache
clear_lines() {
    local start=$1
    local count=$2
    for ((i=0; i<count; i++)); do
        local ln=$((start + i))
        printf '\033[%d;1H\033[K' "$ln"
        unset "LINE_CACHE[line_$ln]"
    done
}

# Cleanup on exit
cleanup() {
    tput rmcup 2>/dev/null || true
    tput cnorm 2>/dev/null || true
}
trap cleanup EXIT INT TERM

# Enter alternate screen and draw static content
tput smcup
tput civis
printf '\033[H\033[2J'

# Draw static header frame (never changes)
echo -e "${BOLD}╔══════════════════════════════════════════════════════════════════════════════╗${NC}"
echo ""  # Line 2: dynamic
echo -e "${BOLD}╠══════════════════════════════════════════════════════════════════════════════╣${NC}"
echo ""  # Line 4: dynamic
echo -e "${BOLD}╚══════════════════════════════════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "${BOLD}Phases:${NC}"

# Draw phase placeholders
for ((i=0; i<PHASE_COUNT; i++)); do
    echo ""
done

# Calculate line numbers for dynamic content
LINE_TITLE=2
LINE_PROGRESS=4
LINE_PHASES_START=8
LINE_DETAILS_START=$((LINE_PHASES_START + PHASE_COUNT + 1))

# Track orchestration time from lock file (when orchestrator actually started)
# Falls back to session start if no lock file
get_orchestrator_start_time() {
    local lock_file="$PLAN_DIR/.orchestrator.lock"
    if [[ -f "$lock_file" ]]; then
        stat -c %Y "$lock_file" 2>/dev/null || stat -f %m "$lock_file" 2>/dev/null || date +%s
    else
        echo ""
    fi
}

# Session tracking
SESSION_START=""
PAUSED_ELAPSED=0

# Previous active phases fingerprint for detecting changes
PREV_ACTIVE_FINGERPRINT=""

# Main loop
while true; do
    # Calculate progress and collect ALL active phases
    completed=0
    active_phases=()
    active_state_files=()
    for phase in "${PHASES[@]}"; do
        state_file="$PLAN_DIR/phases/$phase/state.json"
        if [[ -f "$state_file" ]]; then
            status=$(jq -r '.phase_status // "pending"' "$state_file")
            if [[ "$status" == "complete" ]]; then
                ((completed++)) || true
            elif [[ "$status" != "pending" ]]; then
                active_phases+=("$phase")
                active_state_files+=("$state_file")
            fi
        fi
    done
    active_count=${#active_phases[@]}

    # Orchestrator status and elapsed time tracking
    if check_active_orchestrator "$PLAN_DIR"; then
        orch_status="${GREEN}running${NC}"
        # Start or continue session timing
        if [[ -z "$SESSION_START" ]]; then
            SESSION_START=$(get_orchestrator_start_time)
            [[ -z "$SESSION_START" ]] && SESSION_START=$(date +%s)
        fi
        elapsed=$(($(date +%s) - SESSION_START + PAUSED_ELAPSED))
    else
        orch_status="${YELLOW}stopped${NC}"
        # Pause timing - save elapsed and clear session
        if [[ -n "$SESSION_START" ]]; then
            PAUSED_ELAPSED=$(($(date +%s) - SESSION_START + PAUSED_ELAPSED))
            SESSION_START=""
        fi
        elapsed=$PAUSED_ELAPSED
    fi

    # Update title line (line 2) - fixed width 80 chars inside box
    time_str=$(date '+%H:%M:%S')
    title_text="  Orchestrator Monitor: ${PLAN_NAME}"
    padding=$((76 - ${#title_text} - ${#time_str}))
    title_content="${BOLD}║${NC}${BOLD}  Orchestrator Monitor:${NC} ${CYAN}${PLAN_NAME}${NC}$(printf '%*s' "$padding" '')${DIM}${time_str}${NC}  ${BOLD}║${NC}"
    update_line $LINE_TITLE "$title_content"

    # Update progress line (line 4) - show active count when >1
    elapsed_str=$(format_duration $elapsed)
    if [[ -z "$SESSION_START" && $elapsed -gt 0 ]]; then
        elapsed_display="${DIM}${elapsed_str} (paused)${NC}"
    else
        elapsed_display="${CYAN}${elapsed_str}${NC}"
    fi
    if [[ $active_count -gt 1 ]]; then
        progress_content="${BOLD}║${NC}  ${GREEN}${completed}${NC}/${CYAN}${PHASE_COUNT}${NC} done │ ${BLUE}${active_count}${NC} active │ Elapsed: $(printf '%-18b' "$elapsed_display") │ Orch: ${orch_status}$(printf '%*s' 1 '')${BOLD}║${NC}"
    else
        progress_content="${BOLD}║${NC}  Progress: ${GREEN}${completed}${NC}/${CYAN}${PHASE_COUNT}${NC} complete │ Elapsed: $(printf '%-18b' "$elapsed_display") │ Orchestrator: ${orch_status}$(printf '%*s' 1 '')${BOLD}║${NC}"
    fi
    update_line $LINE_PROGRESS "$progress_content"

    # Update each phase line
    line=$LINE_PHASES_START
    for phase in "${PHASES[@]}"; do
        state_file="$PLAN_DIR/phases/$phase/state.json"
        phase_content=""

        if [[ -f "$state_file" ]]; then
            status=$(jq -r '.phase_status // "unknown"' "$state_file")
            iteration=$(jq -r '.iteration.current // 0' "$state_file")
            max_iter=$(jq -r '.iteration.max // 25' "$state_file")
            tests_pass=$(jq -r '.tests_passing // 0' "$state_file")
            tests_total=$(jq -r '.tests_total // 0' "$state_file")
            is_blocked=$(jq -r '.blocked.is_blocked // false' "$state_file")

            # Status indicator
            case "$status" in
                complete) indicator="${GREEN}●${NC}" ;;
                implementing|impl|impl_review|qa|qa_review|fix) indicator="${BLUE}◐${NC}" ;;
                blocked) indicator="${RED}■${NC}" ;;
                *) indicator="○" ;;
            esac
            [[ "$is_blocked" == "true" ]] && indicator="${RED}■${NC}"

            # Status with color
            status_padded=$(printf "%-12s" "$status")
            case "$status" in
                complete) status_str="${GREEN}${status_padded}${NC}" ;;
                implementing|impl|impl_review|qa|qa_review|fix) status_str="${BLUE}${status_padded}${NC}" ;;
                blocked) status_str="${RED}${status_padded}${NC}" ;;
                pending) status_str="${DIM}${status_padded}${NC}" ;;
                *) status_str="$status_padded" ;;
            esac

            phase_content="  ${indicator} $(printf '%-22s' "$phase") ${status_str}"

            if [[ "$status" != "pending" && "$status" != "complete" ]]; then
                phase_content+=" iter ${CYAN}$(printf '%2d' "$iteration")${NC}/${max_iter}"
                if [[ "$tests_total" -gt 0 ]]; then
                    if [[ "$tests_pass" -eq "$tests_total" ]]; then
                        phase_content+=" ${GREEN}✓ ${tests_pass}/${tests_total}${NC}"
                    else
                        phase_content+=" ${YELLOW}○ ${tests_pass}/${tests_total}${NC}"
                    fi
                fi
                phase_content+="  ${DIM}$(time_ago "$state_file") ago${NC}"
            fi
        else
            phase_content="  ○ $(printf '%-22s' "$phase") ${DIM}no state${NC}"
        fi

        update_line $line "$phase_content"
        ((line++))
    done

    # Active phase details section — show ALL active phases
    details_line=$LINE_DETAILS_START

    # Detect if the set of active phases changed
    active_fingerprint="${active_phases[*]:-none}"
    if [[ "$active_fingerprint" != "$PREV_ACTIVE_FINGERPRINT" ]]; then
        # Clear entire details area when active phases change
        clear_lines $details_line 60
        PREV_ACTIVE_FINGERPRINT="$active_fingerprint"
    fi

    if [[ $active_count -gt 0 ]]; then
        # Determine output lines per phase: split budget across active phases
        # Reserve 7 lines of metadata per active phase + 3 for separators
        local_output_lines=$OUTPUT_LINES
        if [[ $active_count -gt 1 ]]; then
            # Scale output lines down for multiple active phases
            local_output_lines=$(( (OUTPUT_LINES + active_count - 1) / active_count ))
            [[ $local_output_lines -lt 3 ]] && local_output_lines=3
        fi

        for ((ai=0; ai<active_count; ai++)); do
            active_phase="${active_phases[$ai]}"
            active_state_file="${active_state_files[$ai]}"

            status=$(jq -r '.phase_status // "unknown"' "$active_state_file")
            iteration=$(jq -r '.iteration.current // 0' "$active_state_file")
            packages=$(jq -r '.packages // [] | join(", ")' "$active_state_file")
            is_blocked=$(jq -r '.blocked.is_blocked // false' "$active_state_file")
            block_reason=$(jq -r '.blocked.reason // ""' "$active_state_file")
            stuck=$(jq -r '.stuck_iterations // 0' "$active_state_file")
            last_verdict=$(jq -r '.last_verdict // "n/a"' "$active_state_file")
            tests_pass=$(jq -r '.tests_passing // 0' "$active_state_file")
            tests_total=$(jq -r '.tests_total // 0' "$active_state_file")

            # Phase header with separator between multiple phases
            if [[ $ai -eq 0 ]]; then
                update_line $((details_line++)) ""
            else
                update_line $((details_line++)) "${DIM}──────────────────────────────────────────────────────────────${NC}"
            fi

            # Phase title with test status inline
            title_line="${BOLD}Active Phase:${NC} ${CYAN}${active_phase}${NC}"
            if [[ "$tests_total" -gt 0 ]]; then
                if [[ "$tests_pass" -eq "$tests_total" ]]; then
                    title_line+="  ${GREEN}✓ ${tests_pass}/${tests_total}${NC}"
                else
                    title_line+="  ${YELLOW}○ ${tests_pass}/${tests_total}${NC}"
                fi
            fi
            update_line $((details_line++)) "$title_line"

            update_line $((details_line++)) "├─ Mode: ${BLUE}${status}${NC} ($(get_mode_desc "$status"))  Agent: ${MAGENTA}$(get_agent_type "$status")${NC}"
            update_line $((details_line++)) "├─ Iteration: ${CYAN}${iteration}${NC} (stuck: ${stuck})  ${DIM}Updated $(time_ago "$active_state_file") ago${NC}"

            if [[ -n "$packages" ]]; then
                update_line $((details_line++)) "├─ Packages: ${DIM}${packages}${NC}"
            fi

            if [[ "$is_blocked" == "true" && -n "$block_reason" ]]; then
                update_line $((details_line++)) "├─ ${RED}BLOCKED:${NC} ${block_reason}"
            fi

            update_line $((details_line++)) "└─ Last verdict: ${YELLOW}${last_verdict}${NC}"

            # Recent output section
            update_line $((details_line++)) ""
            update_line $((details_line++)) "${BOLD}  Output:${NC}"

            # Find output file
            output_file=""
            test_output="$PLAN_DIR/phases/$active_phase/last_test_output.txt"
            iter_output="$PLAN_DIR/phases/$active_phase/test_output/iteration_${iteration}.txt"
            [[ -f "$test_output" ]] && output_file="$test_output"
            [[ -z "$output_file" && -f "$iter_output" ]] && output_file="$iter_output"

            output_rendered=0
            if [[ -n "$output_file" && -f "$output_file" ]]; then
                while IFS= read -r out_line; do
                    [[ -z "$out_line" ]] && continue
                    out_line="${out_line:0:76}"
                    if [[ "$out_line" =~ (PASS|passed|✓|ok) ]]; then
                        update_line $((details_line++)) "  ${GREEN}${out_line}${NC}"
                    elif [[ "$out_line" =~ (FAIL|failed|✗|error|Error) ]]; then
                        update_line $((details_line++)) "  ${RED}${out_line}${NC}"
                    elif [[ "$out_line" =~ (SKIP|skipped|warning|Warning) ]]; then
                        update_line $((details_line++)) "  ${YELLOW}${out_line}${NC}"
                    else
                        update_line $((details_line++)) "  ${DIM}${out_line}${NC}"
                    fi
                    ((++output_rendered))
                done < <(tail -n "$local_output_lines" "$output_file" 2>/dev/null || true)
            fi

            if [[ $output_rendered -eq 0 ]]; then
                update_line $((details_line++)) "  ${DIM}No output available${NC}"
            fi
        done
    else
        update_line $((details_line++)) ""
        if [[ $completed -eq $PHASE_COUNT ]]; then
            update_line $((details_line++)) "${GREEN}${BOLD}✓ ALL PHASES COMPLETE${NC}"
        else
            update_line $((details_line++)) "${DIM}No active phase (waiting for orchestrator)${NC}"
        fi
    fi

    # Check for completion
    if [[ -f "$PLAN_DIR/COMPLETION_REPORT.md" ]]; then
        update_line $((details_line + 1)) "${GREEN}${BOLD}✓ ORCHESTRATION COMPLETE${NC}"
        update_line $((details_line + 2)) "See: ${CYAN}${PLAN_DIR}/COMPLETION_REPORT.md${NC}"
        sleep 5
        exit 0
    fi

    # Footer below all content
    update_line $((details_line + 1)) ""
    update_line $((details_line + 2)) "${DIM}Refresh: ${INTERVAL}s │ Ctrl+C to exit${NC}"

    sleep "$INTERVAL"
done
