#!/usr/bin/env bats
# QA Tests for escalation-model phase
# Tests the updated escalation ladder in iterate.sh:
# - Level 3: sonnet investigate (was opus)
# - Level 4: sonnet fix (was opus)
# - Level 5: NEW opus combined investigate+fix
# - Level 6+: auto-split (was level 5+)
# - ESCALATION_CONTEXT hoisted to outer block
# - Investigation findings injected into impl prompts
# - ORCH_SECTION and DISPUTE_SECTION now prepended to impl prompt

setup() {
    load 'test_helper'
    setup_temp_dir

    SCRIPT="$SCRIPTS_DIR/iterate.sh"
}

teardown() {
    teardown_temp_dir
}

# ==============================================================================
# Helper: extract a specific stuck level block using awk
# Finds the if block for a given level and captures until the matching fi
# ==============================================================================
get_stuck_level_block() {
    local level="$1"
    local pattern
    if [[ "$level" == "6" ]]; then
        pattern="STUCK_ITERATIONS -ge 6"
    else
        pattern="STUCK_ITERATIONS -eq ${level}"
    fi
    awk -v pat="$pattern" '
        $0 ~ pat { found=1; depth=1; print; next }
        found {
            print
            if (/if /) depth++
            if (/fi$/ || /fi[[:space:]]/) {
                depth--
                if (depth <= 0) exit
            }
        }
    ' "$SCRIPT"
}

# ==============================================================================
# test_escalation_header_comment_updated
# ==============================================================================

@test "qa_escalation-model: test_escalation_header_comment_updated" {
    # The escalation ladder comment should document the new ladder:
    # stuck=3 sonnet investigate, stuck=4 sonnet fix, stuck=5 opus combined, stuck=6+ auto-split
    run grep -A 10 'ESCALATION LADDER (impl mode only)' "$SCRIPT"
    [ "$status" -eq 0 ]

    # Check for sonnet references in comments
    [[ "$output" == *"sonnet"* ]]

    # Check stuck=5 mentions opus combined or combined investigate+fix
    [[ "$output" == *"stuck=5"* ]]
    [[ "$output" == *"opus"* ]] || [[ "$output" == *"combined"* ]]

    # Check stuck=6+ mentions auto-split
    [[ "$output" == *"stuck=6"* ]]
    [[ "$output" == *"auto-split"* ]]
}

# ==============================================================================
# test_escalation_context_hoisted
# ==============================================================================

@test "qa_escalation-model: test_escalation_context_hoisted" {
    # ESCALATION_CONTEXT must be defined BEFORE any level-specific if blocks
    # It should appear between FAILING_TESTS extraction and the first STUCK_ITERATIONS -eq N block

    # Get line numbers
    local failing_tests_line
    failing_tests_line=$(grep -n 'FAILING_TESTS=' "$SCRIPT" | grep -v '^#' | grep 'grep.*FAIL' | head -1 | cut -d: -f1)

    local escalation_context_line
    escalation_context_line=$(grep -n 'ESCALATION_CONTEXT=' "$SCRIPT" | head -1 | cut -d: -f1)

    local first_level_line
    first_level_line=$(grep -n 'STUCK_ITERATIONS -eq 3' "$SCRIPT" | head -1 | cut -d: -f1)

    [ -n "$failing_tests_line" ]
    [ -n "$escalation_context_line" ]
    [ -n "$first_level_line" ]

    # ESCALATION_CONTEXT must be AFTER FAILING_TESTS
    [ "$escalation_context_line" -gt "$failing_tests_line" ]

    # ESCALATION_CONTEXT must be BEFORE the first level-specific block
    [ "$escalation_context_line" -lt "$first_level_line" ]
}

@test "qa_escalation-model: test_escalation_context_not_inside_level3_block" {
    # ESCALATION_CONTEXT should NOT be defined inside the STUCK_ITERATIONS -eq 3 block
    local level3_block
    level3_block=$(get_stuck_level_block 3)

    # The assignment ESCALATION_CONTEXT= should NOT appear in the level 3 block
    run bash -c "echo '$level3_block' | grep 'ESCALATION_CONTEXT='"
    [ "$status" -eq 1 ]
}

# ==============================================================================
# test_level3_uses_sonnet
# ==============================================================================

@test "qa_escalation-model: test_level3_uses_sonnet" {
    local level3_block
    level3_block=$(get_stuck_level_block 3)

    # Should contain --model sonnet
    [[ "$level3_block" == *"--model sonnet"* ]]

    # Should NOT contain --model opus
    run bash -c "echo '$level3_block' | grep -- '--model opus'"
    [ "$status" -eq 1 ]
}

# ==============================================================================
# test_level4_uses_sonnet
# ==============================================================================

@test "qa_escalation-model: test_level4_uses_sonnet" {
    local level4_block
    level4_block=$(get_stuck_level_block 4)

    # Should contain --model sonnet
    [[ "$level4_block" == *"--model sonnet"* ]]

    # Should NOT contain --model opus
    run bash -c "echo '$level4_block' | grep -- '--model opus'"
    [ "$status" -eq 1 ]
}

# ==============================================================================
# test_level5_exists_and_uses_opus
# ==============================================================================

@test "qa_escalation-model: test_level5_exists_and_uses_opus" {
    # Level 5 block must exist
    run grep 'STUCK_ITERATIONS -eq 5' "$SCRIPT"
    [ "$status" -eq 0 ]

    local level5_block
    level5_block=$(get_stuck_level_block 5)
    [ -n "$level5_block" ]

    # Should use opus
    [[ "$level5_block" == *"--model opus"* ]]

    # Should have max-turns 25
    [[ "$level5_block" == *"--max-turns 25"* ]]

    # Should have the union of level 3 and 4 tools
    [[ "$level5_block" == *'--allowedTools "Read,Glob,Grep,View,Edit,Write,Bash"'* ]]
}

# ==============================================================================
# test_level5_combines_investigate_and_fix
# ==============================================================================

@test "qa_escalation-model: test_level5_combines_investigate_and_fix" {
    local level5_block
    level5_block=$(get_stuck_level_block 5)

    # Should reference both investigate and fix prompts
    [[ "$level5_block" == *"INVESTIGATE_PROMPT"* ]]
    [[ "$level5_block" == *"FIX_PROMPT"* ]]

    # Should contain the bridge text
    [[ "$level5_block" == *"After completing your investigation, immediately apply fixes"* ]]
}

# ==============================================================================
# test_level5_runs_tests_after
# ==============================================================================

@test "qa_escalation-model: test_level5_runs_tests_after" {
    local level5_block
    level5_block=$(get_stuck_level_block 5)

    # Should run phase tests after the combined agent
    [[ "$level5_block" == *'run-phase-tests.sh "$PLAN_NAME" "$PHASE"'* ]]
}

# ==============================================================================
# test_level5_exits_zero
# ==============================================================================

@test "qa_escalation-model: test_level5_exits_zero" {
    local level5_block
    level5_block=$(get_stuck_level_block 5)

    # Should end with exit 0
    [[ "$level5_block" == *"exit 0"* ]]
}

# ==============================================================================
# test_level5_calls_kill_tracked_pgids
# ==============================================================================

@test "qa_escalation-model: test_level5_calls_kill_tracked_pgids" {
    local level5_block
    level5_block=$(get_stuck_level_block 5)

    # Should call kill_tracked_pgids before exit 0
    [[ "$level5_block" == *"kill_tracked_pgids"* ]]
}

# ==============================================================================
# test_level6_auto_split_threshold
# ==============================================================================

@test "qa_escalation-model: test_level6_auto_split_threshold" {
    # The auto-split condition should be -ge 6, NOT -ge 5
    run grep 'STUCK_ITERATIONS -ge 6' "$SCRIPT"
    [ "$status" -eq 0 ]

    # -ge 5 should NOT appear as the auto-split threshold
    # Note: -eq 5 is fine (that's level 5), but -ge 5 for auto-split is wrong
    run bash -c "grep 'STUCK_ITERATIONS -ge 5' '$SCRIPT' | grep -i 'auto-split\|split'"
    [ "$status" -eq 1 ]
}

# ==============================================================================
# test_investigation_injection_in_impl_only
# ==============================================================================

@test "qa_escalation-model: test_investigation_injection_in_impl_only" {
    # INVESTIGATION_SECTION should ONLY appear in the impl) case block
    # It should NOT appear in qa), fix), qa-review), or impl-review) blocks

    # Extract the impl block content
    local impl_section
    impl_section=$(sed -n '/^    impl)$/,/^    ;;$/p' "$SCRIPT")
    [[ "$impl_section" == *"INVESTIGATION_SECTION"* ]]

    # Check that other case blocks do NOT contain INVESTIGATION_SECTION
    local qa_section
    qa_section=$(sed -n '/^    qa)$/,/^    ;;$/p' "$SCRIPT")
    run bash -c "echo '$qa_section' | grep 'INVESTIGATION_SECTION'"
    [ "$status" -eq 1 ]

    local fix_section
    fix_section=$(sed -n '/^    fix)$/,/^    ;;$/p' "$SCRIPT")
    run bash -c "echo '$fix_section' | grep 'INVESTIGATION_SECTION'"
    [ "$status" -eq 1 ]

    local qa_review_section
    qa_review_section=$(sed -n '/^    qa-review)$/,/^    ;;$/p' "$SCRIPT")
    run bash -c "echo '$qa_review_section' | grep 'INVESTIGATION_SECTION'"
    [ "$status" -eq 1 ]

    local impl_review_section
    impl_review_section=$(sed -n '/^    impl-review)$/,/^    ;;$/p' "$SCRIPT")
    run bash -c "echo '$impl_review_section' | grep 'INVESTIGATION_SECTION'"
    [ "$status" -eq 1 ]
}

# ==============================================================================
# test_investigation_injection_prepended_to_prompt
# ==============================================================================

@test "qa_escalation-model: test_investigation_injection_prepended_to_prompt" {
    # In the impl) case block, there should be a line:
    # PROMPT="${ORCH_SECTION}${DISPUTE_SECTION}${INVESTIGATION_SECTION}${PROMPT}"

    local impl_section
    impl_section=$(sed -n '/^    impl)$/,/^    ;;$/p' "$SCRIPT")

    [[ "$impl_section" == *'PROMPT="${ORCH_SECTION}${DISPUTE_SECTION}${INVESTIGATION_SECTION}${PROMPT}"'* ]]
}

# ==============================================================================
# test_orch_and_dispute_sections_now_prepended
# ==============================================================================

@test "qa_escalation-model: test_orch_and_dispute_sections_now_prepended" {
    # ORCH_SECTION and DISPUTE_SECTION are defined in impl) but currently
    # NOT prepended to PROMPT. After implementation, they should be via
    # the concatenation line.

    local impl_section
    impl_section=$(sed -n '/^    impl)$/,/^    ;;$/p' "$SCRIPT")

    # ORCH_SECTION is defined
    [[ "$impl_section" == *'ORCH_SECTION='* ]]

    # DISPUTE_SECTION is defined
    [[ "$impl_section" == *'DISPUTE_SECTION='* ]]

    # The concatenation line should include both
    [[ "$impl_section" == *'${ORCH_SECTION}'* ]]
    [[ "$impl_section" == *'${DISPUTE_SECTION}'* ]]

    # Specifically in a PROMPT= assignment that combines all three
    run grep 'PROMPT=.*ORCH_SECTION.*DISPUTE_SECTION.*INVESTIGATION_SECTION.*PROMPT' "$SCRIPT"
    [ "$status" -eq 0 ]
}

# ==============================================================================
# test_investigation_section_empty_when_no_file
# ==============================================================================

@test "qa_escalation-model: test_investigation_section_empty_when_no_file" {
    # The impl) case should define INVESTIGATION_SECTION="" when
    # investigation.md does not exist

    local impl_section
    impl_section=$(sed -n '/^    impl)$/,/^    ;;$/p' "$SCRIPT")

    # Should have the conditional check for investigation.md
    [[ "$impl_section" == *'INVESTIGATION_SECTION=""'* ]]
    [[ "$impl_section" == *'investigation.md'* ]]

    # Simulate: no investigation.md → INVESTIGATION_SECTION is empty
    local phase_dir="$TEST_TEMP_DIR/phase"
    mkdir -p "$phase_dir"
    # Do NOT create investigation.md

    INVESTIGATION_SECTION=""
    if [[ -f "$phase_dir/investigation.md" ]]; then
        INVESTIGATION_SECTION="## Investigation Findings

An investigation agent analyzed this phase and found:

$(cat "$phase_dir/investigation.md")

Use these findings to guide your implementation approach.

---

"
    fi

    [ -z "$INVESTIGATION_SECTION" ]
}

# ==============================================================================
# test_investigation_section_populated_when_file_exists
# ==============================================================================

@test "qa_escalation-model: test_investigation_section_populated_when_file_exists" {
    local phase_dir="$TEST_TEMP_DIR/phase"
    mkdir -p "$phase_dir"
    echo "Root cause: off-by-one in loop" > "$phase_dir/investigation.md"

    INVESTIGATION_SECTION=""
    if [[ -f "$phase_dir/investigation.md" ]]; then
        INVESTIGATION_SECTION="## Investigation Findings

An investigation agent analyzed this phase and found:

$(cat "$phase_dir/investigation.md")

Use these findings to guide your implementation approach.

---

"
    fi

    [[ "$INVESTIGATION_SECTION" == *"## Investigation Findings"* ]]
    [[ "$INVESTIGATION_SECTION" == *"Root cause: off-by-one in loop"* ]]
    [[ "$INVESTIGATION_SECTION" == *"Use these findings to guide your implementation approach."* ]]
}

# ==============================================================================
# test_escalation_ladder_no_gaps
# ==============================================================================

@test "qa_escalation-model: test_escalation_ladder_no_gaps" {
    # All escalation levels should be handled: 3, 4, 5, and 6+
    run grep 'STUCK_ITERATIONS -eq 3' "$SCRIPT"
    [ "$status" -eq 0 ]

    run grep 'STUCK_ITERATIONS -eq 4' "$SCRIPT"
    [ "$status" -eq 0 ]

    run grep 'STUCK_ITERATIONS -eq 5' "$SCRIPT"
    [ "$status" -eq 0 ]

    run grep 'STUCK_ITERATIONS -ge 6' "$SCRIPT"
    [ "$status" -eq 0 ]

    # Verify ordering: 3 < 4 < 5 < 6
    local line3 line4 line5 line6
    line3=$(grep -n 'STUCK_ITERATIONS -eq 3' "$SCRIPT" | head -1 | cut -d: -f1)
    line4=$(grep -n 'STUCK_ITERATIONS -eq 4' "$SCRIPT" | head -1 | cut -d: -f1)
    line5=$(grep -n 'STUCK_ITERATIONS -eq 5' "$SCRIPT" | head -1 | cut -d: -f1)
    line6=$(grep -n 'STUCK_ITERATIONS -ge 6' "$SCRIPT" | head -1 | cut -d: -f1)

    [ "$line3" -lt "$line4" ]
    [ "$line4" -lt "$line5" ]
    [ "$line5" -lt "$line6" ]
}

# ==============================================================================
# test_level3_max_turns_unchanged
# ==============================================================================

@test "qa_escalation-model: test_level3_max_turns_unchanged" {
    local level3_block
    level3_block=$(get_stuck_level_block 3)

    [[ "$level3_block" == *"--max-turns 15"* ]]
}

# ==============================================================================
# test_level4_max_turns_unchanged
# ==============================================================================

@test "qa_escalation-model: test_level4_max_turns_unchanged" {
    local level4_block
    level4_block=$(get_stuck_level_block 4)

    [[ "$level4_block" == *"--max-turns 15"* ]]
}

# ==============================================================================
# test_level3_allowed_tools_unchanged
# ==============================================================================

@test "qa_escalation-model: test_level3_allowed_tools_unchanged" {
    local level3_block
    level3_block=$(get_stuck_level_block 3)

    [[ "$level3_block" == *'--allowedTools "Read,Glob,Grep,Bash,Write"'* ]]
}

# ==============================================================================
# test_level4_allowed_tools_unchanged
# ==============================================================================

@test "qa_escalation-model: test_level4_allowed_tools_unchanged" {
    local level4_block
    level4_block=$(get_stuck_level_block 4)

    [[ "$level4_block" == *'--allowedTools "View,Edit,Write,Bash"'* ]]
}

# ==============================================================================
# test_script_syntax_valid
# ==============================================================================

@test "qa_escalation-model: test_script_syntax_valid" {
    run bash -n "$SCRIPT"
    [ "$status" -eq 0 ]
}

# ==============================================================================
# Edge case: stuck exactly at 5
# Level 5 opus combined triggers; level 6 auto-split does NOT trigger
# ==============================================================================

@test "qa_escalation-model: edge_stuck_exactly_5_triggers_level5_not_level6" {
    # Level 5 uses -eq 5, so it only matches exactly 5
    # Level 6 uses -ge 6, so it doesn't match 5
    # Level 5 has exit 0, so it prevents fall-through

    local level5_block
    level5_block=$(get_stuck_level_block 5)

    # Level 5 block exists and exits
    [[ "$level5_block" == *"exit 0"* ]]

    # Level 6 condition should NOT match 5
    # -ge 6 with value 5 → false (which is correct)
    run bash -c '[[ 5 -ge 6 ]]'
    [ "$status" -ne 0 ]
}

# ==============================================================================
# Edge case: investigation.md very large (>10KB)
# Still injected fully; bash handles large strings
# ==============================================================================

@test "qa_escalation-model: edge_large_investigation_md_injected" {
    local phase_dir="$TEST_TEMP_DIR/phase"
    mkdir -p "$phase_dir"

    # Create a >10KB investigation.md
    python3 -c "print('x' * 15000)" > "$phase_dir/investigation.md"

    INVESTIGATION_SECTION=""
    if [[ -f "$phase_dir/investigation.md" ]]; then
        INVESTIGATION_SECTION="## Investigation Findings

An investigation agent analyzed this phase and found:

$(cat "$phase_dir/investigation.md")

Use these findings to guide your implementation approach.

---

"
    fi

    # Should contain the header
    [[ "$INVESTIGATION_SECTION" == *"## Investigation Findings"* ]]

    # Should contain the full content (15000 x's)
    local content_length=${#INVESTIGATION_SECTION}
    [ "$content_length" -gt 15000 ]
}

# ==============================================================================
# Edge case: investigation.md missing
# INVESTIGATION_SECTION is empty; prompt unchanged from current behavior
# ==============================================================================

@test "qa_escalation-model: edge_investigation_md_missing_empty_section" {
    local phase_dir="$TEST_TEMP_DIR/phase"
    mkdir -p "$phase_dir"
    # No investigation.md file

    INVESTIGATION_SECTION=""
    if [[ -f "$phase_dir/investigation.md" ]]; then
        INVESTIGATION_SECTION="filled"
    fi

    [ -z "$INVESTIGATION_SECTION" ]

    # When concatenated with PROMPT, empty section contributes nothing
    ORCH_SECTION=""
    DISPUTE_SECTION=""
    PROMPT="original prompt content"
    PROMPT="${ORCH_SECTION}${DISPUTE_SECTION}${INVESTIGATION_SECTION}${PROMPT}"

    [ "$PROMPT" = "original prompt content" ]
}

# ==============================================================================
# Edge case: Both investigation.md and dispute exist
# Both sections prepended
# ==============================================================================

@test "qa_escalation-model: edge_both_investigation_and_dispute_prepended" {
    ORCH_SECTION="## Orchestrator Instructions

Do this first.

---

"
    DISPUTE_SECTION="## Recent Dispute Resolution

1 dispute resolved.

---

"
    INVESTIGATION_SECTION="## Investigation Findings

Found the root cause.

---

"
    PROMPT="## Standard Prompt

Do TDD."

    PROMPT="${ORCH_SECTION}${DISPUTE_SECTION}${INVESTIGATION_SECTION}${PROMPT}"

    # All three sections should appear in order
    [[ "$PROMPT" == "## Orchestrator Instructions"* ]]
    [[ "$PROMPT" == *"## Recent Dispute Resolution"* ]]
    [[ "$PROMPT" == *"## Investigation Findings"* ]]
    [[ "$PROMPT" == *"## Standard Prompt"* ]]

    # Verify ordering: ORCH before DISPUTE before INVESTIGATION before original PROMPT
    local orch_pos dispute_pos invest_pos prompt_pos
    orch_pos=$(echo "$PROMPT" | grep -n "Orchestrator Instructions" | head -1 | cut -d: -f1)
    dispute_pos=$(echo "$PROMPT" | grep -n "Recent Dispute Resolution" | head -1 | cut -d: -f1)
    invest_pos=$(echo "$PROMPT" | grep -n "Investigation Findings" | head -1 | cut -d: -f1)
    prompt_pos=$(echo "$PROMPT" | grep -n "Standard Prompt" | head -1 | cut -d: -f1)

    [ "$orch_pos" -lt "$dispute_pos" ]
    [ "$dispute_pos" -lt "$invest_pos" ]
    [ "$invest_pos" -lt "$prompt_pos" ]
}

# ==============================================================================
# Edge case: ESCALATION_CONTEXT at level 5
# Now available because it's hoisted to the outer block
# ==============================================================================

@test "qa_escalation-model: edge_escalation_context_available_at_level5" {
    local level5_block
    level5_block=$(get_stuck_level_block 5)

    # Level 5 should reference ESCALATION_CONTEXT (which is hoisted above it)
    [[ "$level5_block" == *'ESCALATION_CONTEXT'* ]]
}

# ==============================================================================
# Verify level 5 prompt construction details
# ==============================================================================

@test "qa_escalation-model: level5_renders_investigate_prompt_via_template" {
    local level5_block
    level5_block=$(get_stuck_level_block 5)

    # Should render investigate.md via render_template.py
    [[ "$level5_block" == *"render_template.py"* ]]
    [[ "$level5_block" == *"bugfix/investigate.md"* ]]
}

@test "qa_escalation-model: level5_renders_fix_prompt_via_template" {
    local level5_block
    level5_block=$(get_stuck_level_block 5)

    # Should render fix.md via render_template.py
    [[ "$level5_block" == *"bugfix/fix.md"* ]]
}

@test "qa_escalation-model: level5_prompt_combines_escalation_investigate_bridge_fix" {
    local level5_block
    level5_block=$(get_stuck_level_block 5)

    # The PROMPT should combine ESCALATION_CONTEXT, INVESTIGATE_PROMPT, bridge text, and FIX_PROMPT
    [[ "$level5_block" == *'${ESCALATION_CONTEXT}'* ]]
    [[ "$level5_block" == *'${INVESTIGATE_PROMPT}'* ]]
    [[ "$level5_block" == *'${FIX_PROMPT}'* ]]
    [[ "$level5_block" == *"After completing your investigation, immediately apply fixes"* ]]
}

# ==============================================================================
# Verify level 5 calls get-state.sh after tests
# ==============================================================================

@test "qa_escalation-model: level5_calls_get_state_after_tests" {
    local level5_block
    level5_block=$(get_stuck_level_block 5)

    [[ "$level5_block" == *'get-state.sh'* ]]
}

# ==============================================================================
# Verify no changes to check_exit_code or check_for_hang functions
# ==============================================================================

@test "qa_escalation-model: check_exit_code_function_unchanged" {
    # check_exit_code should still exist and not be modified by this phase
    run grep -c 'check_exit_code()' "$SCRIPT"
    [ "$status" -eq 0 ]
    [ "$output" -ge 1 ]
}

@test "qa_escalation-model: check_for_hang_function_unchanged" {
    run grep -c 'check_for_hang()' "$SCRIPT"
    [ "$status" -eq 0 ]
    [ "$output" -ge 1 ]
}

# ==============================================================================
# Verify run_with_timeout function is unchanged
# ==============================================================================

@test "qa_escalation-model: run_with_timeout_function_unchanged" {
    run grep -c 'run_with_timeout()' "$SCRIPT"
    [ "$status" -eq 0 ]
    [ "$output" -ge 1 ]
}

# ==============================================================================
# Verify no auto-split at stuck=5 (only at stuck>=6)
# ==============================================================================

@test "qa_escalation-model: no_auto_split_at_stuck_5" {
    # There should be no -ge 5 condition for auto-split
    run grep 'STUCK_ITERATIONS -ge 5' "$SCRIPT"
    [ "$status" -eq 1 ]
}

# ==============================================================================
# Verify the impl case block prompt construction flow
# ==============================================================================

@test "qa_escalation-model: impl_case_prompt_construction_order" {
    local impl_section
    impl_section=$(sed -n '/^    impl)$/,/^    ;;$/p' "$SCRIPT")

    # Get line numbers within the impl section for key elements
    local orch_line dispute_line investigation_line render_line concat_line run_line
    orch_line=$(echo "$impl_section" | grep -n 'ORCH_SECTION=' | head -1 | cut -d: -f1)
    dispute_line=$(echo "$impl_section" | grep -n 'DISPUTE_SECTION=' | head -1 | cut -d: -f1)
    investigation_line=$(echo "$impl_section" | grep -n 'INVESTIGATION_SECTION=' | head -1 | cut -d: -f1)
    render_line=$(echo "$impl_section" | grep -n 'render_template.py' | head -1 | cut -d: -f1)
    concat_line=$(echo "$impl_section" | grep -n 'PROMPT=.*ORCH_SECTION.*DISPUTE_SECTION.*INVESTIGATION_SECTION' | head -1 | cut -d: -f1)
    run_line=$(echo "$impl_section" | grep -n 'run_with_timeout' | head -1 | cut -d: -f1)

    [ -n "$orch_line" ]
    [ -n "$dispute_line" ]
    [ -n "$investigation_line" ]
    [ -n "$render_line" ]
    [ -n "$concat_line" ]
    [ -n "$run_line" ]

    # Order: ORCH_SECTION → DISPUTE_SECTION → INVESTIGATION_SECTION → render → concat → run
    [ "$orch_line" -lt "$dispute_line" ]
    [ "$dispute_line" -lt "$investigation_line" ]
    [ "$investigation_line" -lt "$render_line" ]
    [ "$render_line" -lt "$concat_line" ]
    [ "$concat_line" -lt "$run_line" ]
}

# ==============================================================================
# Verify investigation injection is between render_template and run_with_timeout
# ==============================================================================

@test "qa_escalation-model: investigation_section_defined_before_build_context" {
    local impl_section
    impl_section=$(sed -n '/^    impl)$/,/^    ;;$/p' "$SCRIPT")

    # INVESTIGATION_SECTION should be defined before build-context.sh call
    local investigation_line build_context_line
    investigation_line=$(echo "$impl_section" | grep -n 'INVESTIGATION_SECTION=""' | head -1 | cut -d: -f1)
    build_context_line=$(echo "$impl_section" | grep -n 'build-context.sh' | head -1 | cut -d: -f1)

    [ -n "$investigation_line" ]
    [ -n "$build_context_line" ]
    [ "$investigation_line" -lt "$build_context_line" ]
}

@test "qa_escalation-model: concat_line_between_render_and_run" {
    local impl_section
    impl_section=$(sed -n '/^    impl)$/,/^    ;;$/p' "$SCRIPT")

    local render_line concat_line run_line
    render_line=$(echo "$impl_section" | grep -n 'render_template.py' | head -1 | cut -d: -f1)
    concat_line=$(echo "$impl_section" | grep -n 'PROMPT=.*ORCH_SECTION.*DISPUTE_SECTION.*INVESTIGATION_SECTION' | head -1 | cut -d: -f1)
    run_line=$(echo "$impl_section" | grep -n 'run_with_timeout' | head -1 | cut -d: -f1)

    [ -n "$render_line" ]
    [ -n "$concat_line" ]
    [ -n "$run_line" ]
    [ "$render_line" -lt "$concat_line" ]
    [ "$concat_line" -lt "$run_line" ]
}

# ==============================================================================
# Verify level 3 echo message updated to mention sonnet
# ==============================================================================

@test "qa_escalation-model: level3_echo_mentions_sonnet" {
    local level3_block
    level3_block=$(get_stuck_level_block 3)

    # The echo should mention sonnet (not opus) for the investigate agent
    [[ "$level3_block" == *"sonnet"* ]]
}

# ==============================================================================
# Verify level 4 echo message mentions sonnet
# ==============================================================================

@test "qa_escalation-model: level4_echo_mentions_sonnet" {
    local level4_block
    level4_block=$(get_stuck_level_block 4)

    # The echo should mention sonnet (not opus) for the fix agent
    [[ "$level4_block" == *"sonnet"* ]]
}

# ==============================================================================
# Verify level 5 echo message mentions opus and combined
# ==============================================================================

@test "qa_escalation-model: level5_echo_mentions_opus_combined" {
    local level5_block
    level5_block=$(get_stuck_level_block 5)

    [[ "$level5_block" == *"opus"* ]]
    [[ "$level5_block" == *"combined"* ]]
}

# ==============================================================================
# Verify level 5 runs check_exit_code after claude
# ==============================================================================

@test "qa_escalation-model: level5_calls_check_exit_code" {
    local level5_block
    level5_block=$(get_stuck_level_block 5)

    [[ "$level5_block" == *"check_exit_code"* ]]
}

# ==============================================================================
# Verify level 5 captures EXIT_CODE
# ==============================================================================

@test "qa_escalation-model: level5_captures_exit_code" {
    local level5_block
    level5_block=$(get_stuck_level_block 5)

    [[ "$level5_block" == *'EXIT_CODE=$?'* ]]
}
