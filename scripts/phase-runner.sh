#!/usr/bin/env bash
#
# phase-runner.sh - Hardened phase execution with full enforcement
#
# This script wraps the iteration loop with all enforcement mechanisms:
# - File boundary checking
# - State validation
# - Dispute limits
# - Timeout watchdog
# - Completion verification
# - Automatic rollback on failure
#
# Usage: phase-runner.sh <phase> [--qa-only] [--continue]

set -euo pipefail

ARC_SCRIPTS_DIR="${ARC_SCRIPTS_DIR:-$ARC_HOME/scripts}"
ENFORCEMENT_DIR="$ARC_SCRIPTS_DIR/../enforcement/scripts"
COORDINATION_DIR="${COORDINATION_DIR:-.claude/phase-automation/coordination}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

log_error() { echo -e "${RED}[ERROR]${NC} $1" >&2; }
log_ok() { echo -e "${GREEN}[OK]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_info() { echo -e "${CYAN}[INFO]${NC} $1"; }
log_phase() { echo -e "\n${CYAN}========================================${NC}"; echo -e "${CYAN}$1${NC}"; echo -e "${CYAN}========================================${NC}\n"; }

# Parse arguments
PHASE=""
QA_ONLY=false
CONTINUE=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --qa-only) QA_ONLY=true; shift ;;
        --continue) CONTINUE=true; shift ;;
        -*) log_error "Unknown option: $1"; exit 1 ;;
        *) PHASE="$1"; shift ;;
    esac
done

if [[ -z "$PHASE" ]]; then
    echo "Usage: phase-runner.sh <phase> [--qa-only] [--continue]"
    echo ""
    echo "Options:"
    echo "  --qa-only    Only run QA test generation, skip implementation"
    echo "  --continue   Continue from existing state (don't reinitialize)"
    exit 1
fi

# Configuration
BRANCH="phase-$PHASE"
QA_BRANCH="phase-$PHASE-qa"
STATE_FILE="$COORDINATION_DIR/phases/$PHASE/state.json"
PHASE_DIR="$COORDINATION_DIR/phases/$PHASE"
MAX_ITERATIONS="${MAX_ITERATIONS:-25}"

# Generate session ID for all agent messages to go to a single folder
if [[ -z "${ITERATE_SESSION_ID:-}" ]]; then
    export ITERATE_SESSION_ID=$(uuidgen 2>/dev/null || cat /proc/sys/kernel/random/uuid)
    log_info "Session ID: $ITERATE_SESSION_ID"
fi

# Ensure phase directory exists
mkdir -p "$PHASE_DIR"

#######################################
# Pre-flight checks
#######################################

preflight_checks() {
    log_phase "Pre-flight Checks"
    
    # Check required tools
    local required_tools=(jq git cargo)
    for tool in "${required_tools[@]}"; do
        if ! command -v "$tool" &> /dev/null; then
            log_error "Required tool not found: $tool"
            exit 1
        fi
    done
    log_ok "Required tools available"
    
    # Check enforcement scripts exist
    local required_scripts=(
        "$ENFORCEMENT_DIR/validate-state.sh"
        "$ENFORCEMENT_DIR/dispute.sh"
        "$ENFORCEMENT_DIR/watchdog.sh"
        "$ENFORCEMENT_DIR/verify-complete.sh"
    )
    for script in "${required_scripts[@]}"; do
        if [[ ! -x "$script" ]]; then
            log_error "Enforcement script missing or not executable: $script"
            exit 1
        fi
    done
    log_ok "Enforcement scripts available"
    
    # Check git hooks are installed
    if [[ ! -f ".git/hooks/pre-commit" ]]; then
        log_warn "Git hooks not installed. Installing..."
        ln -sf "../../.claude/phase-automation/enforcement/hooks/pre-commit" ".git/hooks/pre-commit"
        ln -sf "../../.claude/phase-automation/enforcement/hooks/commit-msg" ".git/hooks/commit-msg"
        chmod +x .git/hooks/pre-commit .git/hooks/commit-msg
        log_ok "Git hooks installed"
    else
        log_ok "Git hooks present"
    fi
    
    # Check we're on the right branch or create it
    local current_branch=$(git rev-parse --abbrev-ref HEAD)
    if [[ "$current_branch" != "$BRANCH" && "$current_branch" != "$QA_BRANCH" ]]; then
        if git show-ref --verify --quiet "refs/heads/$BRANCH"; then
            log_info "Switching to existing branch $BRANCH"
            git checkout "$BRANCH"
        else
            log_info "Creating new branch $BRANCH from develop"
            git checkout -b "$BRANCH" develop
        fi
    fi
    log_ok "On branch: $(git rev-parse --abbrev-ref HEAD)"
}

#######################################
# Initialize or load state
#######################################

init_or_load_state() {
    log_phase "State Initialization"
    
    if [[ "$CONTINUE" == "true" && -f "$STATE_FILE" ]]; then
        log_info "Loading existing state..."
        if ! "$ENFORCEMENT_DIR/validate-state.sh" read "$PHASE" > /dev/null; then
            log_error "Existing state is invalid!"
            log_error "Use --no-continue to reinitialize, or fix the state manually"
            exit 1
        fi
        log_ok "State loaded and validated"
    else
        log_info "Initializing new state..."
        
        local init_state=$(cat << EOF
{
    "phase": "$PHASE",
    "phase_status": "init",
    "iteration": {"current": 0, "max": $MAX_ITERATIONS},
    "chunks": {"total": 0, "completed": [], "current": null, "remaining": []},
    "blocked": {"is_blocked": false, "reason": null},
    "dispute": null,
    "last_updated_by": "system"
}
EOF
)
        "$ENFORCEMENT_DIR/validate-state.sh" write "$PHASE" "$init_state"
        log_ok "State initialized"
    fi
    
    # Initialize watchdog
    "$ENFORCEMENT_DIR/watchdog.sh" start "$PHASE" $$
    log_ok "Watchdog started"
}

#######################################
# QA Test Generation Phase
#######################################

run_qa_phase() {
    log_phase "QA Test Generation"
    
    # Switch to QA branch
    if ! git show-ref --verify --quiet "refs/heads/$QA_BRANCH"; then
        git checkout -b "$QA_BRANCH" "$BRANCH"
    else
        git checkout "$QA_BRANCH"
    fi
    
    # Set context for git hooks
    export AGENT_CONTEXT="qa"
    
    # Update state
    local state=$(cat "$STATE_FILE")
    state=$(echo "$state" | jq '.phase_status = "planning" | .last_updated_by = "qa"')
    "$ENFORCEMENT_DIR/validate-state.sh" write "$PHASE" "$state"
    
    log_info "Invoking QA test generation..."
    log_info "QA reads ONLY the plan and writes tests to tests/qa/$PHASE/"
    
    # Invoke Claude with QA prompt
    if command -v claude &> /dev/null; then
        claude --agent .claude/agents/qa-test-writer.md \
               --context "Phase: $PHASE" \
               --context "Output directory: tests/qa/$PHASE/" \
               --context "Read plan from: plans/$PHASE.md" \
               2>&1 | tee "$PHASE_DIR/qa-generation.log"
    else
        log_warn "Claude CLI not found. In production, this would invoke the QA process."
        log_warn "Simulating QA completion for testing..."
        mkdir -p "tests/qa/$PHASE"
        echo "// QA tests would be generated here" > "tests/qa/$PHASE/placeholder_test.rs"
    fi
    
    # Verify QA produced tests
    if [[ ! -d "tests/qa/$PHASE" ]] || [[ -z "$(ls -A tests/qa/$PHASE 2>/dev/null)" ]]; then
        log_error "QA did not produce any tests!"
        exit 1
    fi
    
    # Commit QA tests
    git add "tests/qa/$PHASE/"
    git commit -m "test: add QA tests for phase $PHASE" || true
    
    # Merge QA branch back to phase branch
    git checkout "$BRANCH"
    git merge "$QA_BRANCH" -m "chore: merge QA tests for phase $PHASE"
    
    log_ok "QA tests generated and merged"
    
    unset AGENT_CONTEXT
}

#######################################
# Implementation Loop
#######################################

run_implementation_loop() {
    log_phase "Implementation Phase"
    
    export AGENT_CONTEXT="impl"
    
    # Update state to implementing
    local state=$(cat "$STATE_FILE")
    state=$(echo "$state" | jq '.phase_status = "implementing" | .last_updated_by = "impl"')
    "$ENFORCEMENT_DIR/validate-state.sh" write "$PHASE" "$state"
    
    local iteration=0
    local max_iterations=$MAX_ITERATIONS
    
    while [[ $iteration -lt $max_iterations ]]; do
        ((iteration++))
        
        log_info "=== Iteration $iteration/$max_iterations ==="
        
        # Start iteration timer
        "$ENFORCEMENT_DIR/watchdog.sh" iteration-start "$PHASE"
        
        # Check for timeouts
        if ! "$ENFORCEMENT_DIR/watchdog.sh" check "$PHASE"; then
            log_error "Watchdog timeout!"
            exit 1
        fi
        
        # Load current state
        state=$("$ENFORCEMENT_DIR/validate-state.sh" read "$PHASE")
        
        # Check if blocked
        local is_blocked=$(echo "$state" | jq -r '.blocked.is_blocked')
        if [[ "$is_blocked" == "true" ]]; then
            local reason=$(echo "$state" | jq -r '.blocked.reason')
            log_warn "Phase is BLOCKED: $reason"
            log_warn "Waiting for coordinator or human intervention..."
            sleep 30
            continue
        fi
        
        # Check if disputed
        local phase_status=$(echo "$state" | jq -r '.phase_status')
        if [[ "$phase_status" == "disputed" ]]; then
            log_warn "Active dispute - waiting for resolution..."
            sleep 30
            continue
        fi
        
        # Check if complete
        if [[ "$phase_status" == "complete" ]]; then
            log_ok "Phase marked as complete"
            break
        fi
        
        # Check dispute limits before iteration
        if ! "$ENFORCEMENT_DIR/dispute.sh" check "$PHASE"; then
            log_error "Dispute limits exceeded!"
            log_error "HUMAN INTERVENTION REQUIRED"
            state=$(echo "$state" | jq '.phase_status = "blocked" | .blocked.is_blocked = true | .blocked.reason = "Dispute limits exceeded"')
            "$ENFORCEMENT_DIR/validate-state.sh" write "$PHASE" "$state"
            exit 1
        fi
        
        # Invoke implementation for one iteration
        log_info "Invoking implementation..."
        
        if command -v claude &> /dev/null; then
            local result
            result=$(claude --agent .claude/agents/phase-lead.md \
                   --context "Phase: $PHASE" \
                   --context "Iteration: $iteration" \
                   --context "State file: $STATE_FILE" \
                   --max-turns 1 \
                   2>&1) || true
            
            echo "$result" >> "$PHASE_DIR/implementation.log"
        else
            log_warn "Claude CLI not found. Simulating iteration..."
            sleep 2
        fi
        
        # End iteration timer
        "$ENFORCEMENT_DIR/watchdog.sh" iteration-end "$PHASE"
        
        # Reload state (may have been modified)
        state=$("$ENFORCEMENT_DIR/validate-state.sh" read "$PHASE")
        
        # Update iteration count in state
        state=$(echo "$state" | jq --argjson iter "$iteration" '.iteration.current = $iter')
        "$ENFORCEMENT_DIR/validate-state.sh" write "$PHASE" "$state"
        
        # Check for new disputes
        local dispute=$(echo "$state" | jq -r '.dispute // empty')
        if [[ -n "$dispute" && "$dispute" != "null" ]]; then
            local dispute_id=$(echo "$dispute" | jq -r '.id // "unknown"')
            log_warn "Dispute filed: $dispute_id"
            log_warn "Phase will block until resolved"
        fi
        
        # Check if tests pass
        log_info "Running tests..."
        if "$ARC_SCRIPTS_DIR/run-phase-tests.sh" "$PLAN_NAME" "$PHASE" 2>&1 | tee -a "$PHASE_DIR/test-results.log" | grep -q '"failed": 0'; then
            log_ok "Tests passing"
            
            # If all chunks done and tests pass, attempt completion
            local remaining=$(echo "$state" | jq '.chunks.remaining | length')
            if [[ "$remaining" -eq 0 ]]; then
                log_info "All chunks complete, verifying completion..."
                
                if "$ENFORCEMENT_DIR/verify-complete.sh" "$PHASE"; then
                    state=$(echo "$state" | jq '.phase_status = "complete"')
                    "$ENFORCEMENT_DIR/validate-state.sh" write "$PHASE" "$state"
                    log_ok "Phase complete!"
                    break
                else
                    log_warn "Completion verification failed, continuing..."
                fi
            fi
        else
            log_warn "Tests failing, continuing iteration..."
        fi
        
        # Brief pause between iterations
        sleep 2
    done
    
    unset AGENT_CONTEXT
    
    # Check final status
    state=$("$ENFORCEMENT_DIR/validate-state.sh" read "$PHASE")
    phase_status=$(echo "$state" | jq -r '.phase_status')
    
    if [[ "$phase_status" != "complete" ]]; then
        log_error "Phase did not complete within $max_iterations iterations!"
        log_error "Final status: $phase_status"
        exit 1
    fi
}

#######################################
# Merge to develop
#######################################

merge_to_develop() {
    log_phase "Merge to Develop"
    
    # Acquire merge lock
    if ! "$ENFORCEMENT_DIR/merge-lock.sh" acquire "$PHASE"; then
        log_error "Could not acquire merge lock"
        exit 1
    fi
    
    # Trap to ensure lock is released
    trap '"$ENFORCEMENT_DIR/merge-lock.sh" release "$PHASE"' EXIT
    
    # Checkout develop and merge
    git checkout develop
    
    if ! git merge "$BRANCH" -m "chore: merge phase $PHASE"; then
        log_error "Merge conflict!"
        
        # Update state
        local state=$("$ENFORCEMENT_DIR/validate-state.sh" read "$PHASE")
        state=$(echo "$state" | jq '.phase_status = "merge_conflict"')
        "$ENFORCEMENT_DIR/validate-state.sh" write "$PHASE" "$state"
        
        git merge --abort
        git checkout "$BRANCH"
        
        "$ENFORCEMENT_DIR/merge-lock.sh" release "$PHASE"
        trap - EXIT
        
        exit 1
    fi
    
    local merge_commit=$(git rev-parse HEAD)
    log_ok "Merged at commit $merge_commit"
    
    # Run integration gate
    log_info "Running integration gate..."
    if ! "$ENFORCEMENT_DIR/integration-gate.sh" "$PHASE" "$merge_commit"; then
        log_error "Integration gate failed!"
        log_error "Merge has been automatically reverted"
        
        "$ENFORCEMENT_DIR/merge-lock.sh" release "$PHASE"
        trap - EXIT
        
        exit 1
    fi
    
    # Release lock
    "$ENFORCEMENT_DIR/merge-lock.sh" release "$PHASE"
    trap - EXIT
    
    log_ok "Phase $PHASE successfully merged to develop"
}

#######################################
# Main execution
#######################################

main() {
    echo ""
    echo "========================================"
    echo "  Phase Execution (Hardened)"
    echo "  Phase: $PHASE"
    echo "========================================"
    echo ""
    
    preflight_checks
    init_or_load_state
    
    # Check if QA tests already exist
    if [[ ! -d "tests/qa/$PHASE" ]] || [[ -z "$(ls -A tests/qa/$PHASE 2>/dev/null)" ]]; then
        run_qa_phase
    else
        log_info "QA tests already exist, skipping QA phase"
    fi
    
    if [[ "$QA_ONLY" == "true" ]]; then
        log_ok "QA-only mode, stopping before implementation"
        exit 0
    fi
    
    run_implementation_loop
    merge_to_develop
    
    log_phase "Phase Complete"
    log_ok "Phase $PHASE has been successfully implemented and merged!"
    
    # Show summary
    "$ENFORCEMENT_DIR/watchdog.sh" status "$PHASE"
}

main
