#!/usr/bin/env bash
#
# Develop branch merge lock - prevents concurrent merges
#
# Usage:
#   merge-lock.sh acquire <phase>    # Acquire lock for merge
#   merge-lock.sh release <phase>    # Release lock
#   merge-lock.sh status             # Show lock status
#   merge-lock.sh force-release      # Force release (human only)

set -euo pipefail

ARC_PHASES_DIR="${ARC_PHASES_DIR:-}"
[[ -z "$ARC_PHASES_DIR" ]] && { echo "ERROR: ARC_PHASES_DIR must be set" >&2; exit 1; }

LOCK_FILE="$(dirname "$ARC_PHASES_DIR")/develop.lock"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_error() { echo -e "${RED}[ERROR]${NC} $1" >&2; }
log_ok() { echo -e "${GREEN}[OK]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }

LOCK_TIMEOUT_MINUTES="${LOCK_TIMEOUT:-30}"

is_lock_stale() {
    if [[ ! -f "$LOCK_FILE" ]]; then
        return 0  # No lock = stale (i.e., available)
    fi

    local acquired_at=$(jq -r '.acquired_at' "$LOCK_FILE" 2>/dev/null || echo "")
    if [[ -z "$acquired_at" ]]; then
        return 0
    fi

    # Convert to epoch
    local acquired_epoch=$(date -d "$acquired_at" +%s 2>/dev/null || echo "0")
    local now_epoch=$(date +%s)
    local age_minutes=$(( (now_epoch - acquired_epoch) / 60 ))

    if [[ $age_minutes -ge $LOCK_TIMEOUT_MINUTES ]]; then
        log_warn "Lock is stale (acquired $age_minutes minutes ago)"
        return 0
    fi

    return 1
}

acquire_lock() {
    local phase="$1"

    mkdir -p "$(dirname "$LOCK_FILE")"

    # Check for existing lock
    if [[ -f "$LOCK_FILE" ]] && ! is_lock_stale; then
        local holder=$(jq -r '.held_by' "$LOCK_FILE")
        local acquired=$(jq -r '.acquired_at' "$LOCK_FILE")
        log_error "Lock is held by '$holder' (since $acquired)"
        log_error "Cannot acquire lock for '$phase'"
        return 1
    fi

    # If stale lock exists, log it
    if [[ -f "$LOCK_FILE" ]]; then
        log_warn "Removing stale lock from '$(jq -r '.held_by' "$LOCK_FILE")'"
    fi

    # Create lock atomically
    local tmp_lock="$LOCK_FILE.$$"
    cat > "$tmp_lock" << EOF
{
    "held_by": "$phase",
    "acquired_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
    "timeout_minutes": $LOCK_TIMEOUT_MINUTES,
    "pid": $$
}
EOF

    # Atomic rename
    mv "$tmp_lock" "$LOCK_FILE"

    log_ok "Lock acquired for '$phase'"
    return 0
}

release_lock() {
    local phase="$1"

    if [[ ! -f "$LOCK_FILE" ]]; then
        log_warn "No lock to release"
        return 0
    fi

    local holder=$(jq -r '.held_by' "$LOCK_FILE")

    if [[ "$holder" != "$phase" ]]; then
        log_error "Lock is held by '$holder', not '$phase'"
        log_error "Cannot release lock owned by another phase"
        return 1
    fi

    rm -f "$LOCK_FILE"
    log_ok "Lock released by '$phase'"
    return 0
}

force_release() {
    if [[ ! -f "$LOCK_FILE" ]]; then
        log_warn "No lock to release"
        return 0
    fi

    local holder=$(jq -r '.held_by' "$LOCK_FILE")
    log_warn "Force-releasing lock held by '$holder'"

    # Archive the lock for debugging
    local archive_dir="$(dirname "$ARC_PHASES_DIR")/lock-archive"
    mkdir -p "$archive_dir"
    mv "$LOCK_FILE" "$archive_dir/$(date +%Y%m%d-%H%M%S)-$holder.json"

    log_ok "Lock force-released"
    return 0
}

show_status() {
    echo "========================================"
    echo "Develop Branch Lock Status"
    echo "========================================"
    echo ""

    if [[ ! -f "$LOCK_FILE" ]]; then
        echo -e "Status: ${GREEN}UNLOCKED${NC}"
        echo "The develop branch is available for merges."
        return 0
    fi

    local holder=$(jq -r '.held_by' "$LOCK_FILE")
    local acquired=$(jq -r '.acquired_at' "$LOCK_FILE")
    local timeout=$(jq -r '.timeout_minutes' "$LOCK_FILE")

    local acquired_epoch=$(date -d "$acquired" +%s 2>/dev/null || echo "0")
    local now_epoch=$(date +%s)
    local age_minutes=$(( (now_epoch - acquired_epoch) / 60 ))
    local remaining=$((timeout - age_minutes))

    echo -e "Status: ${YELLOW}LOCKED${NC}"
    echo ""
    echo "Held by:      $holder"
    echo "Acquired at:  $acquired"
    echo "Age:          $age_minutes minutes"
    echo "Timeout:      $timeout minutes"

    if [[ $remaining -le 0 ]]; then
        echo -e "Remaining:    ${RED}EXPIRED${NC} (will be auto-released)"
    elif [[ $remaining -le 5 ]]; then
        echo -e "Remaining:    ${YELLOW}$remaining minutes${NC}"
    else
        echo "Remaining:    $remaining minutes"
    fi

    return 1
}

# Main command dispatch
main() {
    local cmd="${1:-status}"
    shift || true

    case "$cmd" in
        acquire)
            [[ $# -lt 1 ]] && { log_error "Usage: merge-lock.sh acquire <phase>"; exit 1; }
            acquire_lock "$1"
            ;;
        release)
            [[ $# -lt 1 ]] && { log_error "Usage: merge-lock.sh release <phase>"; exit 1; }
            release_lock "$1"
            ;;
        force-release)
            force_release
            ;;
        status)
            show_status
            ;;
        *)
            echo "Usage: merge-lock.sh <command> [args]"
            echo ""
            echo "Commands:"
            echo "  acquire <phase>    Acquire lock for merge"
            echo "  release <phase>    Release lock"
            echo "  status             Show lock status"
            echo "  force-release      Force release (human override)"
            exit 1
            ;;
    esac
}

main "$@"
