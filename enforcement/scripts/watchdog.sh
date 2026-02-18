#!/usr/bin/env bash
#
# Watchdog: Enforce time limits on agent iterations
#
# Usage:
#   watchdog.sh start <phase> <pid>           # Start watching a process
#   watchdog.sh check <phase>                 # Check if phase has timed out
#   watchdog.sh iteration-start <phase>       # Mark iteration start
#   watchdog.sh iteration-end <phase>         # Mark iteration end

set -euo pipefail

ARC_PHASES_DIR="${ARC_PHASES_DIR:-}"
[[ -z "$ARC_PHASES_DIR" ]] && { echo "ERROR: ARC_PHASES_DIR must be set" >&2; exit 1; }

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_error() { echo -e "${RED}[WATCHDOG]${NC} $1" >&2; }
log_ok() { echo -e "${GREEN}[WATCHDOG]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WATCHDOG]${NC} $1"; }

# Timeout configuration
ITERATION_TIMEOUT_SECONDS="${ITERATION_TIMEOUT:-600}"      # 10 minutes per iteration
PHASE_TIMEOUT_SECONDS="${PHASE_TIMEOUT:-14400}"           # 4 hours per phase

get_watchdog_file() {
    local phase="$1"
    echo "$ARC_PHASES_DIR/$phase/watchdog.json"
}

init_watchdog() {
    local phase="$1"
    local pid="${2:-}"
    local file=$(get_watchdog_file "$phase")

    mkdir -p "$(dirname "$file")"

    cat > "$file" << EOF
{
    "phase": "$phase",
    "pid": ${pid:-null},
    "phase_started_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
    "iteration_started_at": null,
    "iteration_count": 0,
    "total_iteration_time_seconds": 0,
    "timeouts": {
        "iteration_seconds": $ITERATION_TIMEOUT_SECONDS,
        "phase_seconds": $PHASE_TIMEOUT_SECONDS
    },
    "status": "running"
}
EOF

    log_ok "Watchdog initialized for $phase"
}

iteration_start() {
    local phase="$1"
    local file=$(get_watchdog_file "$phase")

    if [[ ! -f "$file" ]]; then
        init_watchdog "$phase"
    fi

    jq --arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" '
        .iteration_started_at = $ts |
        .iteration_count += 1
    ' "$file" > "$file.tmp" && mv "$file.tmp" "$file"
}

iteration_end() {
    local phase="$1"
    local file=$(get_watchdog_file "$phase")

    if [[ ! -f "$file" ]]; then
        log_warn "No watchdog file found"
        return 0
    fi

    local started_at=$(jq -r '.iteration_started_at // empty' "$file")
    if [[ -z "$started_at" ]]; then
        return 0
    fi

    local started_epoch=$(date -d "$started_at" +%s 2>/dev/null || echo "0")
    local now_epoch=$(date +%s)
    local duration=$((now_epoch - started_epoch))

    jq --argjson duration "$duration" '
        .iteration_started_at = null |
        .total_iteration_time_seconds += $duration
    ' "$file" > "$file.tmp" && mv "$file.tmp" "$file"
}

check_timeouts() {
    local phase="$1"
    local file=$(get_watchdog_file "$phase")

    if [[ ! -f "$file" ]]; then
        log_warn "No watchdog file for $phase"
        return 0
    fi

    local now_epoch=$(date +%s)

    # Check iteration timeout
    local iter_started=$(jq -r '.iteration_started_at // empty' "$file")
    if [[ -n "$iter_started" ]]; then
        local iter_epoch=$(date -d "$iter_started" +%s 2>/dev/null || echo "0")
        local iter_age=$((now_epoch - iter_epoch))
        local iter_timeout=$(jq -r '.timeouts.iteration_seconds' "$file")

        if [[ $iter_age -ge $iter_timeout ]]; then
            log_error "ITERATION TIMEOUT: $iter_age seconds >= $iter_timeout seconds"
            handle_timeout "$phase" "iteration"
            return 1
        fi
    fi

    # Check phase timeout
    local phase_started=$(jq -r '.phase_started_at' "$file")
    local phase_epoch=$(date -d "$phase_started" +%s 2>/dev/null || echo "0")
    local phase_age=$((now_epoch - phase_epoch))
    local phase_timeout=$(jq -r '.timeouts.phase_seconds' "$file")

    if [[ $phase_age -ge $phase_timeout ]]; then
        log_error "PHASE TIMEOUT: $phase_age seconds >= $phase_timeout seconds"
        handle_timeout "$phase" "phase"
        return 1
    fi

    return 0
}

handle_timeout() {
    local phase="$1"
    local timeout_type="$2"
    local file=$(get_watchdog_file "$phase")

    # Update watchdog status
    jq --arg type "$timeout_type" --arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" '
        .status = "timeout" |
        .timeout_type = $type |
        .timeout_at = $ts
    ' "$file" > "$file.tmp" && mv "$file.tmp" "$file"

    # Update phase state
    local state_file="$ARC_PHASES_DIR/$phase/state.json"
    if [[ -f "$state_file" ]]; then
        jq '.phase_status = "timeout" | .blocked.is_blocked = true | .blocked.reason = "Watchdog timeout"' \
            "$state_file" > "$state_file.tmp" && mv "$state_file.tmp" "$state_file"
    fi

    # Try to kill the process
    local pid=$(jq -r '.pid // empty' "$file")
    if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
        log_warn "Killing process $pid"
        kill -TERM "$pid" 2>/dev/null || true
        sleep 2
        kill -KILL "$pid" 2>/dev/null || true
    fi

    log_error "Phase $phase has timed out ($timeout_type)"
    log_error "HUMAN INTERVENTION REQUIRED"
}

show_status() {
    local phase="$1"
    local file=$(get_watchdog_file "$phase")

    if [[ ! -f "$file" ]]; then
        echo "No watchdog data for $phase"
        return 0
    fi

    local now_epoch=$(date +%s)

    echo "========================================"
    echo "Watchdog Status: $phase"
    echo "========================================"
    echo ""

    local status=$(jq -r '.status' "$file")
    local iter_count=$(jq -r '.iteration_count' "$file")
    local phase_started=$(jq -r '.phase_started_at' "$file")
    local total_time=$(jq -r '.total_iteration_time_seconds' "$file")

    echo "Status:           $status"
    echo "Iterations:       $iter_count"
    echo "Phase started:    $phase_started"
    echo "Total time:       $total_time seconds"
    echo ""

    # Phase timeout status
    local phase_epoch=$(date -d "$phase_started" +%s 2>/dev/null || echo "0")
    local phase_age=$((now_epoch - phase_epoch))
    local phase_timeout=$(jq -r '.timeouts.phase_seconds' "$file")
    local phase_remaining=$((phase_timeout - phase_age))

    echo "Phase timeout:"
    echo "  Elapsed:    $((phase_age / 60)) minutes"
    echo "  Limit:      $((phase_timeout / 60)) minutes"
    if [[ $phase_remaining -le 0 ]]; then
        echo -e "  Remaining:  ${RED}EXPIRED${NC}"
    elif [[ $phase_remaining -le 600 ]]; then
        echo -e "  Remaining:  ${YELLOW}$((phase_remaining / 60)) minutes${NC}"
    else
        echo "  Remaining:  $((phase_remaining / 60)) minutes"
    fi

    # Current iteration status
    local iter_started=$(jq -r '.iteration_started_at // empty' "$file")
    if [[ -n "$iter_started" ]]; then
        local iter_epoch=$(date -d "$iter_started" +%s 2>/dev/null || echo "0")
        local iter_age=$((now_epoch - iter_epoch))
        local iter_timeout=$(jq -r '.timeouts.iteration_seconds' "$file")
        local iter_remaining=$((iter_timeout - iter_age))

        echo ""
        echo "Current iteration:"
        echo "  Started:    $iter_started"
        echo "  Elapsed:    $iter_age seconds"
        echo "  Limit:      $iter_timeout seconds"
        if [[ $iter_remaining -le 0 ]]; then
            echo -e "  Remaining:  ${RED}EXPIRED${NC}"
        elif [[ $iter_remaining -le 60 ]]; then
            echo -e "  Remaining:  ${YELLOW}$iter_remaining seconds${NC}"
        else
            echo "  Remaining:  $iter_remaining seconds"
        fi
    else
        echo ""
        echo "No iteration currently running"
    fi
}

# Background watchdog loop
run_watchdog_loop() {
    local phase="$1"
    local check_interval="${2:-30}"

    log_ok "Starting watchdog loop for $phase (checking every ${check_interval}s)"

    while true; do
        if ! check_timeouts "$phase"; then
            log_error "Timeout detected, exiting watchdog loop"
            exit 1
        fi
        sleep "$check_interval"
    done
}

# Main command dispatch
main() {
    local cmd="${1:-}"
    shift || true

    case "$cmd" in
        start)
            [[ $# -lt 1 ]] && { log_error "Usage: watchdog.sh start <phase> [pid]"; exit 1; }
            init_watchdog "$1" "${2:-}"
            ;;
        check)
            [[ $# -lt 1 ]] && { log_error "Usage: watchdog.sh check <phase>"; exit 1; }
            check_timeouts "$1"
            ;;
        iteration-start)
            [[ $# -lt 1 ]] && { log_error "Usage: watchdog.sh iteration-start <phase>"; exit 1; }
            iteration_start "$1"
            ;;
        iteration-end)
            [[ $# -lt 1 ]] && { log_error "Usage: watchdog.sh iteration-end <phase>"; exit 1; }
            iteration_end "$1"
            ;;
        status)
            [[ $# -lt 1 ]] && { log_error "Usage: watchdog.sh status <phase>"; exit 1; }
            show_status "$1"
            ;;
        loop)
            [[ $# -lt 1 ]] && { log_error "Usage: watchdog.sh loop <phase> [interval]"; exit 1; }
            run_watchdog_loop "$1" "${2:-30}"
            ;;
        *)
            echo "Usage: watchdog.sh <command> [args]"
            echo ""
            echo "Commands:"
            echo "  start <phase> [pid]        Initialize watchdog"
            echo "  check <phase>              Check for timeouts"
            echo "  iteration-start <phase>    Mark iteration start"
            echo "  iteration-end <phase>      Mark iteration end"
            echo "  status <phase>             Show watchdog status"
            echo "  loop <phase> [interval]    Run continuous watchdog"
            exit 1
            ;;
    esac
}

main "$@"
