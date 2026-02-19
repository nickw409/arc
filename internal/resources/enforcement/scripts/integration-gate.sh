#!/usr/bin/env bash
#
# Integration gate: Run after merge to develop
# If tests fail, automatically revert the merge
#
# Usage: integration-gate.sh <phase> <merge_commit>

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ARC_PHASES_DIR="${ARC_PHASES_DIR:-}"
[[ -z "$ARC_PHASES_DIR" ]] && { echo "ERROR: ARC_PHASES_DIR must be set" >&2; exit 1; }

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_error() { echo -e "${RED}[ERROR]${NC} $1" >&2; }
log_ok() { echo -e "${GREEN}[OK]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_info() { echo -e "[INFO] $1"; }

PHASE="${1:-}"
MERGE_COMMIT="${2:-HEAD}"

if [[ -z "$PHASE" ]]; then
    echo "Usage: integration-gate.sh <phase> [merge_commit]"
    exit 2
fi

GATE_LOG="$ARC_PHASES_DIR/$PHASE/integration-gate.log"
mkdir -p "$(dirname "$GATE_LOG")"

log_to_file() {
    echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] $1" >> "$GATE_LOG"
}

echo "========================================"
echo "Integration Gate: $PHASE"
echo "Merge Commit: $MERGE_COMMIT"
echo "========================================"
echo ""

log_to_file "Starting integration gate for $PHASE (commit: $MERGE_COMMIT)"

CHECKS_PASSED=0
CHECKS_FAILED=0
FAILURES=()

# Kill any orphaned cargo processes
cleanup_cargo() {
    pkill -9 -f "cargo.*nextest" 2>/dev/null || true
    pkill -9 -f "cargo.*build" 2>/dev/null || true
    pkill -9 -f "cargo.*test" 2>/dev/null || true
    pkill -9 -f "cargo.*clippy" 2>/dev/null || true
}

run_check() {
    local name="$1"
    local cmd="$2"
    local timeout_secs="${3:-300}"  # 5 minute default

    echo -n "  $name... "
    log_to_file "Running: $name"

    local output_file=$(mktemp)
    local start_time=$(date +%s)

    # Use setsid to create new process group so timeout can kill all children
    # -k 10 sends SIGKILL after 10s grace period if SIGTERM doesn't work
    if setsid timeout -k 10 "$timeout_secs" bash -c "$cmd" > "$output_file" 2>&1; then
        local duration=$(($(date +%s) - start_time))
        echo -e "${GREEN}PASS${NC} (${duration}s)"
        log_to_file "PASS: $name (${duration}s)"
        ((CHECKS_PASSED++))
        rm -f "$output_file"
        return 0
    else
        local exit_code=$?
        local duration=$(($(date +%s) - start_time))

        if [[ $exit_code -eq 124 ]]; then
            echo -e "${RED}TIMEOUT${NC} (${duration}s)"
            log_to_file "TIMEOUT: $name (${duration}s)"
            # Clean up orphaned cargo processes on timeout
            cleanup_cargo
        else
            echo -e "${RED}FAIL${NC} (${duration}s, exit $exit_code)"
            log_to_file "FAIL: $name (exit $exit_code)"
        fi

        log_to_file "Output: $(head -20 "$output_file")"
        FAILURES+=("$name")
        ((CHECKS_FAILED++))
        rm -f "$output_file"
        return 1
    fi
}

# 1. Build checks
echo "1. Build Verification"
run_check "cargo build" "cargo build --all-targets" 600
run_check "cargo build --release" "cargo build --release --all-targets" 600

# 2. All unit tests
echo ""
echo "2. Unit Tests"
run_check "cargo test" "cargo test --all-targets" 600

# 3. Integration tests (if they exist)
echo ""
echo "3. Integration Tests"
if [[ -d "tests/integration" ]]; then
    run_check "integration tests" "cargo test --test '*' integration" 600
else
    echo "  (no integration tests directory)"
fi

# 4. Reference tests (Rust vs Java comparison, if configured)
echo ""
echo "4. Reference Tests"
if [[ -d "tests/reference" ]]; then
    run_check "reference tests" "cargo test --test '*' reference" 600
elif [[ -f "tests/reference.rs" ]]; then
    run_check "reference tests" "cargo test reference" 600
else
    echo "  (no reference tests configured)"
fi

# 5. Property-based tests
echo ""
echo "5. Property Tests"
if [[ -d "tests/property" ]] || grep -rq "proptest" tests/ 2>/dev/null; then
    run_check "property tests" "cargo test property" 600
else
    echo "  (no property tests configured)"
fi

# 6. Code quality
echo ""
echo "6. Code Quality"
run_check "clippy" "cargo clippy --all-targets -- -D warnings" 300
run_check "fmt check" "cargo fmt -- --check" 60

# 7. Doc tests
echo ""
echo "7. Documentation"
run_check "doc tests" "cargo test --doc" 300

# Summary
echo ""
echo "========================================"
echo "Integration Gate Summary"
echo "========================================"
echo ""
echo -e "Passed: ${GREEN}$CHECKS_PASSED${NC}"
echo -e "Failed: ${RED}$CHECKS_FAILED${NC}"

if [[ $CHECKS_FAILED -gt 0 ]]; then
    echo ""
    echo -e "${RED}INTEGRATION GATE FAILED${NC}"
    echo ""
    echo "Failed checks:"
    for f in "${FAILURES[@]}"; do
        echo "  ✗ $f"
    done

    log_to_file "GATE FAILED - Initiating rollback"

    echo ""
    echo "========================================"
    echo "INITIATING AUTOMATIC ROLLBACK"
    echo "========================================"
    echo ""

    # Get the parent commit (before merge)
    PARENT_COMMIT=$(git rev-parse "$MERGE_COMMIT^1" 2>/dev/null || echo "")

    if [[ -n "$PARENT_COMMIT" ]]; then
        log_warn "Reverting merge commit $MERGE_COMMIT"
        log_warn "Returning to $PARENT_COMMIT"

        # Create a revert commit
        if git revert --no-edit "$MERGE_COMMIT" 2>/dev/null; then
            log_ok "Merge reverted successfully"
            log_to_file "Rollback successful"

            # Update phase state
            STATE_FILE="$ARC_PHASES_DIR/$PHASE/state.json"
            if [[ -f "$STATE_FILE" ]]; then
                jq '.phase_status = "integration_failed"' "$STATE_FILE" > "$STATE_FILE.tmp"
                mv "$STATE_FILE.tmp" "$STATE_FILE"
                log_info "Phase status set to 'integration_failed'"
            fi
        else
            log_error "Failed to revert merge automatically"
            log_error "Manual intervention required!"
            log_to_file "Rollback FAILED - manual intervention required"

            echo ""
            echo "To manually revert:"
            echo "  git revert $MERGE_COMMIT"
            echo "  # or"
            echo "  git reset --hard $PARENT_COMMIT"
        fi
    else
        log_error "Could not determine parent commit for rollback"
        log_error "Manual intervention required!"
        log_to_file "Rollback FAILED - could not determine parent"
    fi

    exit 1
else
    echo ""
    echo -e "${GREEN}INTEGRATION GATE PASSED${NC}"
    log_to_file "GATE PASSED"

    echo ""
    echo "Phase $PHASE successfully integrated into develop."
    exit 0
fi
