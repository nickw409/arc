#!/usr/bin/env bash
# Kill zombie cargo processes that are stuck consuming CPU
#
# Usage:
#   kill-zombie-cargo.sh [--dry-run] [--max-age MINUTES]
#
# Options:
#   --dry-run       Show what would be killed without actually killing
#   --max-age MIN   Kill processes older than MIN minutes (default: 5)

set -euo pipefail

DRY_RUN=false
MAX_AGE_MINUTES=5

while [[ $# -gt 0 ]]; do
    case $1 in
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        --max-age)
            MAX_AGE_MINUTES="$2"
            shift 2
            ;;
        *)
            echo "Unknown option: $1" >&2
            exit 1
            ;;
    esac
done

# Convert minutes to seconds for comparison
MAX_AGE_SECONDS=$((MAX_AGE_MINUTES * 60))

killed_count=0
checked_count=0

# Find cargo/rustc processes, get their PID, CPU%, elapsed time, and command
# Using ps with custom format: pid, %cpu, etimes (elapsed seconds), args
while IFS= read -r line; do
    [[ -z "$line" ]] && continue

    # Parse the ps output
    pid=$(echo "$line" | awk '{print $1}')
    cpu=$(echo "$line" | awk '{print $2}' | cut -d. -f1)  # Integer part of CPU%
    elapsed=$(echo "$line" | awk '{print $3}')
    cmd=$(echo "$line" | awk '{$1=$2=$3=""; print $0}' | sed 's/^[[:space:]]*//')

    checked_count=$((checked_count + 1))

    # Skip if process is too young
    if [[ "$elapsed" -lt "$MAX_AGE_SECONDS" ]]; then
        continue
    fi

    # Skip if CPU usage is low (probably legitimately waiting on I/O)
    if [[ "$cpu" -lt 50 ]]; then
        continue
    fi

    # This is likely a zombie: old + high CPU
    elapsed_min=$((elapsed / 60))

    if [[ "$DRY_RUN" == "true" ]]; then
        echo "[DRY-RUN] Would kill PID $pid (CPU: ${cpu}%, age: ${elapsed_min}m)"
        echo "          Command: ${cmd:0:80}..."
    else
        echo "Killing PID $pid (CPU: ${cpu}%, age: ${elapsed_min}m)"
        echo "  Command: ${cmd:0:80}..."
        kill "$pid" 2>/dev/null || echo "  Failed to kill (already dead?)"
    fi

    killed_count=$((killed_count + 1))

done < <(ps -eo pid,%cpu,etimes,args 2>/dev/null | grep -E 'cargo|rustc' | grep -v 'grep\|kill-zombie' || true)

if [[ "$killed_count" -eq 0 ]]; then
    echo "No zombie cargo processes found (checked $checked_count processes, max age: ${MAX_AGE_MINUTES}m, min CPU: 50%)"
else
    if [[ "$DRY_RUN" == "true" ]]; then
        echo "Would kill $killed_count zombie process(es)"
    else
        echo "Killed $killed_count zombie process(es)"
    fi
fi
