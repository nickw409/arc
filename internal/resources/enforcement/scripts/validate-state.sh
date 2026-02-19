#!/usr/bin/env bash
#
# State management with validation and atomic writes
#
# Usage:
#   validate-state.sh read <phase>           # Read and validate state
#   validate-state.sh write <phase> <json>   # Validate and atomically write state
#   validate-state.sh transition <phase> <new_status>  # Validate state transition
#   validate-state.sh checksum <json>        # Generate checksum for state

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCHEMA_FILE="$SCRIPT_DIR/../schemas/state.schema.json"
ARC_PHASES_DIR="${ARC_PHASES_DIR:-}"
[[ -z "$ARC_PHASES_DIR" ]] && { echo "ERROR: ARC_PHASES_DIR must be set" >&2; exit 1; }

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_error() { echo -e "${RED}[ERROR]${NC} $1" >&2; }
log_ok() { echo -e "${GREEN}[OK]${NC} $1" >&2; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1" >&2; }

# Check for required tools
check_dependencies() {
    if ! command -v jq &> /dev/null; then
        log_error "jq is required but not installed"
        exit 1
    fi

    # Check for jsonschema validator (python or node)
    if command -v jsonschema &> /dev/null; then
        VALIDATOR="jsonschema"
    elif command -v ajv &> /dev/null; then
        VALIDATOR="ajv"
    else
        log_warn "No JSON schema validator found. Schema validation disabled."
        log_warn "Install: pip install jsonschema OR npm install -g ajv-cli"
        VALIDATOR="none"
    fi
}

# Generate checksum of state (excluding checksum field)
generate_checksum() {
    local json="$1"
    echo "$json" | jq -S 'del(.checksum)' | sha256sum | cut -d' ' -f1
}

# Validate JSON against schema
validate_schema() {
    local json="$1"

    if [[ "$VALIDATOR" == "none" ]]; then
        # Basic validation without schema
        if ! echo "$json" | jq empty 2>/dev/null; then
            log_error "Invalid JSON"
            return 1
        fi
        return 0
    fi

    local tmp_file=$(mktemp)
    echo "$json" > "$tmp_file"

    local result=0
    case "$VALIDATOR" in
        jsonschema)
            if ! jsonschema -i "$tmp_file" "$SCHEMA_FILE" 2>/dev/null; then
                result=1
            fi
            ;;
        ajv)
            if ! ajv validate -s "$SCHEMA_FILE" -d "$tmp_file" 2>/dev/null; then
                result=1
            fi
            ;;
    esac

    rm -f "$tmp_file"
    return $result
}

# Validate state transition
validate_transition() {
    local current_status="$1"
    local new_status="$2"

    # Define valid transitions
    declare -A VALID_TRANSITIONS
    VALID_TRANSITIONS["init"]="planning"
    VALID_TRANSITIONS["planning"]="implementing blocked"
    VALID_TRANSITIONS["implementing"]="complete blocked disputed failed timeout"
    VALID_TRANSITIONS["blocked"]="implementing disputed failed"
    VALID_TRANSITIONS["disputed"]="implementing blocked failed"
    VALID_TRANSITIONS["complete"]=""  # Terminal state (can only go to integration_failed or reverted)
    VALID_TRANSITIONS["failed"]="implementing"  # Can retry
    VALID_TRANSITIONS["integration_failed"]="implementing"
    VALID_TRANSITIONS["merge_conflict"]="implementing failed"
    VALID_TRANSITIONS["timeout"]="implementing failed"
    VALID_TRANSITIONS["reverted"]="implementing"

    local allowed="${VALID_TRANSITIONS[$current_status]:-}"

    if [[ -z "$allowed" && "$new_status" != "$current_status" ]]; then
        if [[ "$current_status" == "complete" ]]; then
            # Complete can only go to error states
            if [[ "$new_status" =~ ^(integration_failed|reverted)$ ]]; then
                return 0
            fi
        fi
        log_error "No transitions allowed from '$current_status'"
        return 1
    fi

    if [[ ! " $allowed " =~ " $new_status " && "$new_status" != "$current_status" ]]; then
        log_error "Invalid transition: $current_status -> $new_status"
        log_error "Allowed transitions: $allowed"
        return 1
    fi

    return 0
}

# Verify checksum
verify_checksum() {
    local json="$1"
    local stored_checksum=$(echo "$json" | jq -r '.checksum // empty')

    if [[ -z "$stored_checksum" ]]; then
        log_warn "No checksum in state file"
        return 0
    fi

    local computed_checksum=$(generate_checksum "$json")

    if [[ "$stored_checksum" != "$computed_checksum" ]]; then
        log_error "Checksum mismatch!"
        log_error "Stored:   $stored_checksum"
        log_error "Computed: $computed_checksum"
        return 1
    fi

    return 0
}

# Read state file
read_state() {
    local phase="$1"
    local state_file="$ARC_PHASES_DIR/$phase/state.json"

    if [[ ! -f "$state_file" ]]; then
        log_error "State file not found: $state_file"
        return 1
    fi

    local json=$(cat "$state_file")

    # Verify checksum
    if ! verify_checksum "$json"; then
        log_error "State file may be corrupted!"
        return 1
    fi

    # Validate schema
    if ! validate_schema "$json"; then
        log_error "State file fails schema validation"
        return 1
    fi

    echo "$json"
}

# Write state file atomically
write_state() {
    local phase="$1"
    local json="$2"

    local state_dir="$ARC_PHASES_DIR/$phase"
    local state_file="$state_dir/state.json"
    local tmp_file="$state_file.tmp.$$"
    local backup_file="$state_file.backup"

    # Ensure directory exists
    mkdir -p "$state_dir"

    # Add/update timestamp
    json=$(echo "$json" | jq --arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" '.last_updated = $ts')

    # Generate and add checksum
    local checksum=$(generate_checksum "$json")
    json=$(echo "$json" | jq --arg cs "$checksum" '.checksum = $cs')

    # Validate before writing
    if ! validate_schema "$json"; then
        log_error "Refusing to write invalid state"
        return 1
    fi

    # If existing state, validate transition
    if [[ -f "$state_file" ]]; then
        local current_status=$(jq -r '.phase_status' "$state_file")
        local new_status=$(echo "$json" | jq -r '.phase_status')

        if ! validate_transition "$current_status" "$new_status"; then
            log_error "Invalid state transition"
            return 1
        fi

        # Backup current state
        cp "$state_file" "$backup_file"
    fi

    # Write to temp file
    echo "$json" | jq '.' > "$tmp_file"

    # Verify temp file is valid
    if ! jq empty "$tmp_file" 2>/dev/null; then
        log_error "Failed to write valid JSON to temp file"
        rm -f "$tmp_file"
        return 1
    fi

    # Atomic rename
    mv "$tmp_file" "$state_file"

    log_ok "State written to $state_file"
    return 0
}

# Main command dispatch
main() {
    check_dependencies

    local cmd="${1:-}"
    shift || true

    case "$cmd" in
        read)
            [[ $# -lt 1 ]] && { log_error "Usage: validate-state.sh read <phase>"; exit 1; }
            read_state "$1"
            ;;
        write)
            [[ $# -lt 2 ]] && { log_error "Usage: validate-state.sh write <phase> <json>"; exit 1; }
            write_state "$1" "$2"
            ;;
        transition)
            [[ $# -lt 2 ]] && { log_error "Usage: validate-state.sh transition <current> <new>"; exit 1; }
            if validate_transition "$1" "$2"; then
                log_ok "Transition $1 -> $2 is valid"
            else
                exit 1
            fi
            ;;
        checksum)
            [[ $# -lt 1 ]] && { log_error "Usage: validate-state.sh checksum <json>"; exit 1; }
            generate_checksum "$1"
            ;;
        *)
            echo "Usage: validate-state.sh <command> [args]"
            echo ""
            echo "Commands:"
            echo "  read <phase>              Read and validate state file"
            echo "  write <phase> <json>      Atomically write state"
            echo "  transition <old> <new>    Check if transition is valid"
            echo "  checksum <json>           Generate checksum"
            exit 1
            ;;
    esac
}

main "$@"
