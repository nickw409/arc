#!/usr/bin/env bash
#
# Dispute tracking and limit enforcement
#
# Usage:
#   dispute.sh file <phase> <dispute_json>    # File a new dispute
#   dispute.sh resolve <phase> <id> <decision> <reason>  # Resolve dispute
#   dispute.sh check <phase>                  # Check if disputes are allowed
#   dispute.sh history <phase>                # Show dispute history
#   dispute.sh lock-test <phase> <test>       # Lock a test from further disputes

set -euo pipefail

ARC_PHASES_DIR="${ARC_PHASES_DIR:-}"
[[ -z "$ARC_PHASES_DIR" ]] && { echo "ERROR: ARC_PHASES_DIR must be set" >&2; exit 1; }

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_error() { echo -e "${RED}[ERROR]${NC} $1" >&2; }
log_ok() { echo -e "${GREEN}[OK]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }

# Limits
MAX_DISPUTES_PER_TEST=3
MAX_DISPUTES_PER_PHASE=10
MAX_REJECTIONS_BEFORE_LOCK=2

get_disputes_file() {
    local phase="$1"
    echo "$(dirname "$ARC_PHASES_DIR")/disputes/${phase}-disputes.json"
}

get_locked_tests_file() {
    local phase="$1"
    echo "$ARC_PHASES_DIR/$phase/locked-tests.json"
}

init_disputes_file() {
    local phase="$1"
    local file=$(get_disputes_file "$phase")

    mkdir -p "$(dirname "$file")"

    if [[ ! -f "$file" ]]; then
        cat > "$file" << 'EOF'
{
  "phase": "",
  "disputes": [],
  "stats": {
    "total_filed": 0,
    "total_approved": 0,
    "total_rejected": 0,
    "total_escalated": 0
  },
  "per_test_counts": {}
}
EOF
        jq --arg phase "$phase" '.phase = $phase' "$file" > "$file.tmp" && mv "$file.tmp" "$file"
    fi
}

init_locked_tests_file() {
    local phase="$1"
    local file=$(get_locked_tests_file "$phase")

    mkdir -p "$(dirname "$file")"

    if [[ ! -f "$file" ]]; then
        echo '{"locked_tests": [], "lock_reasons": {}}' > "$file"
    fi
}

is_test_locked() {
    local phase="$1"
    local test_name="$2"
    local file=$(get_locked_tests_file "$phase")

    init_locked_tests_file "$phase"

    jq -e --arg test "$test_name" '.locked_tests | index($test) != null' "$file" > /dev/null 2>&1
}

lock_test() {
    local phase="$1"
    local test_name="$2"
    local reason="${3:-Max rejections reached}"
    local file=$(get_locked_tests_file "$phase")

    init_locked_tests_file "$phase"

    jq --arg test "$test_name" --arg reason "$reason" '
        .locked_tests += [$test] |
        .locked_tests |= unique |
        .lock_reasons[$test] = $reason
    ' "$file" > "$file.tmp" && mv "$file.tmp" "$file"

    log_warn "Test '$test_name' is now LOCKED from further disputes"
}

check_dispute_limits() {
    local phase="$1"
    local test_name="${2:-}"
    local file=$(get_disputes_file "$phase")

    init_disputes_file "$phase"

    # Check phase limit
    local total=$(jq '.stats.total_filed' "$file")
    if [[ "$total" -ge "$MAX_DISPUTES_PER_PHASE" ]]; then
        log_error "Phase dispute limit reached ($total >= $MAX_DISPUTES_PER_PHASE)"
        log_error "HUMAN INTERVENTION REQUIRED"
        return 1
    fi

    # Check per-test limit if test specified
    if [[ -n "$test_name" ]]; then
        # Check if test is locked
        if is_test_locked "$phase" "$test_name"; then
            log_error "Test '$test_name' is LOCKED from disputes"
            return 1
        fi

        local test_count=$(jq --arg test "$test_name" '.per_test_counts[$test] // 0' "$file")
        if [[ "$test_count" -ge "$MAX_DISPUTES_PER_TEST" ]]; then
            log_error "Dispute limit for test '$test_name' reached ($test_count >= $MAX_DISPUTES_PER_TEST)"
            return 1
        fi

        # Check rejection count for auto-lock
        local rejections=$(jq --arg test "$test_name" '
            [.disputes[] | select(.test_name == $test and .resolution == "rejected")] | length
        ' "$file")

        if [[ "$rejections" -ge "$MAX_REJECTIONS_BEFORE_LOCK" ]]; then
            log_warn "Test '$test_name' has been rejected $rejections times"
            lock_test "$phase" "$test_name" "Rejected $rejections times"
            log_error "Test is now LOCKED"
            return 1
        fi
    fi

    return 0
}

check_duplicate_dispute() {
    local phase="$1"
    local dispute_json="$2"
    local file=$(get_disputes_file "$phase")

    # Extract key fields from new dispute
    local test_name=$(echo "$dispute_json" | jq -r '.test_name')
    local reason=$(echo "$dispute_json" | jq -r '.reason')

    # Check for similar existing dispute
    local similar=$(jq --arg test "$test_name" --arg reason "$reason" '
        [.disputes[] | select(.test_name == $test and .reason == $reason)] | length
    ' "$file")

    if [[ "$similar" -gt 0 ]]; then
        log_error "Duplicate dispute detected for test '$test_name'"
        log_error "Same or very similar dispute was already filed"
        log_error "AUTO-ESCALATING to human"
        return 1
    fi

    return 0
}

file_dispute() {
    local phase="$1"
    local dispute_json="$2"
    local file=$(get_disputes_file "$phase")

    init_disputes_file "$phase"

    # Extract test name
    local test_name=$(echo "$dispute_json" | jq -r '.test_name')

    # Check limits
    if ! check_dispute_limits "$phase" "$test_name"; then
        return 1
    fi

    # Check for duplicates
    if ! check_duplicate_dispute "$phase" "$dispute_json"; then
        # Mark as auto-escalated
        dispute_json=$(echo "$dispute_json" | jq '.auto_escalated = true')
    fi

    # Generate dispute ID
    local total=$(jq '.stats.total_filed' "$file")
    local dispute_id="dispute-$((total + 1))"

    # Get attempt number for this test
    local attempt=$(jq --arg test "$test_name" '(.per_test_counts[$test] // 0) + 1' "$file")

    # Add metadata
    dispute_json=$(echo "$dispute_json" | jq \
        --arg id "$dispute_id" \
        --arg filed "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
        --argjson attempt "$attempt" '
        .id = $id |
        .filed_at = $filed |
        .attempt_number = $attempt |
        .resolution = null |
        .resolution_reason = null |
        .resolved_at = null |
        .resolved_by = null
    ')

    # Update disputes file
    jq --argjson dispute "$dispute_json" --arg test "$test_name" '
        .disputes += [$dispute] |
        .stats.total_filed += 1 |
        .per_test_counts[$test] = ((.per_test_counts[$test] // 0) + 1)
    ' "$file" > "$file.tmp" && mv "$file.tmp" "$file"

    log_ok "Dispute filed: $dispute_id"
    echo "$dispute_id"
}

resolve_dispute() {
    local phase="$1"
    local dispute_id="$2"
    local decision="$3"  # approved, rejected, escalated
    local reason="$4"
    local resolved_by="${5:-orchestrator}"
    local file=$(get_disputes_file "$phase")

    if [[ ! "$decision" =~ ^(approved|rejected|escalated)$ ]]; then
        log_error "Invalid decision: $decision (must be approved, rejected, or escalated)"
        return 1
    fi

    # Update the dispute
    jq --arg id "$dispute_id" \
       --arg decision "$decision" \
       --arg reason "$reason" \
       --arg resolved_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
       --arg resolved_by "$resolved_by" '
        (.disputes[] | select(.id == $id)) |= (
            .resolution = $decision |
            .resolution_reason = $reason |
            .resolved_at = $resolved_at |
            .resolved_by = $resolved_by
        ) |
        .stats["total_" + $decision] += 1
    ' "$file" > "$file.tmp" && mv "$file.tmp" "$file"

    # Check if we need to lock the test after rejection
    if [[ "$decision" == "rejected" ]]; then
        local test_name=$(jq -r --arg id "$dispute_id" '.disputes[] | select(.id == $id) | .test_name' "$file")
        local rejections=$(jq --arg test "$test_name" '
            [.disputes[] | select(.test_name == $test and .resolution == "rejected")] | length
        ' "$file")

        if [[ "$rejections" -ge "$MAX_REJECTIONS_BEFORE_LOCK" ]]; then
            lock_test "$phase" "$test_name" "Rejected $rejections times"
        fi
    fi

    log_ok "Dispute $dispute_id resolved: $decision"
}

show_history() {
    local phase="$1"
    local file=$(get_disputes_file "$phase")

    init_disputes_file "$phase"

    echo "========================================"
    echo "Dispute History: $phase"
    echo "========================================"
    echo ""

    jq -r '
        "Stats:",
        "  Total filed: \(.stats.total_filed)",
        "  Approved: \(.stats.total_approved)",
        "  Rejected: \(.stats.total_rejected)",
        "  Escalated: \(.stats.total_escalated)",
        "",
        "Limits:",
        "  Phase: \(.stats.total_filed)/'"$MAX_DISPUTES_PER_PHASE"'",
        "",
        "Per-test counts:",
        (.per_test_counts | to_entries[] | "  \(.key): \(.value)/'"$MAX_DISPUTES_PER_TEST"'"),
        "",
        "Recent disputes:",
        (.disputes[-5:][] | "  [\(.id)] \(.test_name) - \(.resolution // "pending")")
    ' "$file" 2>/dev/null || echo "No disputes recorded"

    # Show locked tests
    local locked_file=$(get_locked_tests_file "$phase")
    if [[ -f "$locked_file" ]]; then
        echo ""
        echo "Locked tests:"
        jq -r '.locked_tests[] | "  🔒 \(.)"' "$locked_file" 2>/dev/null || true
    fi
}

# Main command dispatch
main() {
    local cmd="${1:-}"
    shift || true

    case "$cmd" in
        file)
            [[ $# -lt 2 ]] && { log_error "Usage: dispute.sh file <phase> <dispute_json>"; exit 1; }
            file_dispute "$1" "$2"
            ;;
        resolve)
            [[ $# -lt 4 ]] && { log_error "Usage: dispute.sh resolve <phase> <id> <decision> <reason> [resolved_by]"; exit 1; }
            resolve_dispute "$1" "$2" "$3" "$4" "${5:-orchestrator}"
            ;;
        check)
            [[ $# -lt 1 ]] && { log_error "Usage: dispute.sh check <phase> [test_name]"; exit 1; }
            if check_dispute_limits "$1" "${2:-}"; then
                log_ok "Disputes allowed"
            else
                exit 1
            fi
            ;;
        history)
            [[ $# -lt 1 ]] && { log_error "Usage: dispute.sh history <phase>"; exit 1; }
            show_history "$1"
            ;;
        lock-test)
            [[ $# -lt 2 ]] && { log_error "Usage: dispute.sh lock-test <phase> <test_name> [reason]"; exit 1; }
            init_locked_tests_file "$1"
            lock_test "$1" "$2" "${3:-Manual lock}"
            ;;
        *)
            echo "Usage: dispute.sh <command> [args]"
            echo ""
            echo "Commands:"
            echo "  file <phase> <json>        File a new dispute"
            echo "  resolve <phase> <id> <decision> <reason>  Resolve dispute"
            echo "  check <phase> [test]       Check if disputes allowed"
            echo "  history <phase>            Show dispute history"
            echo "  lock-test <phase> <test>   Lock test from disputes"
            exit 1
            ;;
    esac
}

main "$@"
