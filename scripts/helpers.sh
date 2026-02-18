#!/usr/bin/env bash
# Shared helper functions for arc scripts
# Source this file from iterate.sh: source "$ARC_SCRIPTS_DIR/helpers.sh"

# =============================================================================
# Helper Functions - Get verdicts and map states
# =============================================================================

# Get valid verdicts for a state from a workflow file
get_state_verdicts() {
    local state_name="$1"
    local workflow="$2"
    yq ".states[] | select(.name == \"$state_name\") | .verdicts[]" "$workflow" 2>/dev/null | tr '\n' ',' | sed 's/,$//'
}

# Map workflow state names to valid phase_status values
map_state_to_status() {
    local state="$1"
    case "$state" in
        impl|fix|refactor|optimize)              echo "implementing" ;;
        draft|research|characterize)             echo "implementing" ;;
        baseline|analyze|investigate)            echo "implementing" ;;
        regression_tests)                        echo "implementing" ;;
        qa)                                      echo "qa" ;;
        qa_review)                               echo "qa_review" ;;
        impl_review)                             echo "impl_review" ;;
        test_review|char_review|fix_review)      echo "qa_review" ;;
        review|verify|benchmark)                 echo "qa_review" ;;
        complete)                                echo "complete" ;;
        blocked)                                 echo "blocked" ;;
        *)
            echo "WARNING: Unknown state '$state' - no mapping defined" >&2
            echo "$state"
            ;;
    esac
}
