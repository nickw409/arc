#!/usr/bin/env bats

# Tests for extract-verdict.sh
# Phase: 01-verdict-extraction (orchestration-v2)

setup() {
    load 'test_helper'
    setup_temp_dir
}

teardown() {
    # Restore permissions before cleanup (for test_unreadable_file)
    if [[ -f "$TEST_TEMP_DIR/unreadable.md" ]]; then
        chmod 644 "$TEST_TEMP_DIR/unreadable.md" 2>/dev/null || true
    fi
    teardown_temp_dir
}

# Helper function to create test file with content
create_review_file() {
    local content="$1"
    local filename="${2:-input.md}"
    printf '%s' "$content" > "$TEST_TEMP_DIR/$filename"
    echo "$TEST_TEMP_DIR/$filename"
}

# Helper function to create test file with CRLF line endings
create_review_file_crlf() {
    local content="$1"
    local filename="${2:-input.md}"
    # Convert LF to CRLF
    printf '%s' "$content" | sed 's/$/\r/' > "$TEST_TEMP_DIR/$filename"
    echo "$TEST_TEMP_DIR/$filename"
}

#=============================================================================
# Basic Script Existence and Syntax Tests
#=============================================================================

@test "extract-verdict.sh exists and is executable" {
    [[ -x "$SCRIPTS_DIR/extract-verdict.sh" ]]
}

@test "script is syntactically valid bash" {
    run bash -n "$SCRIPTS_DIR/extract-verdict.sh"
    [[ "$status" -eq 0 ]]
}

#=============================================================================
# Argument Validation Tests
#=============================================================================

@test "test_no_valid_verdicts_argument: shows usage with missing second argument" {
    local file=$(create_review_file "## Verdict
approved")
    run "$SCRIPTS_DIR/extract-verdict.sh" "$file"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Usage"* ]]
}

@test "shows usage with no arguments" {
    run "$SCRIPTS_DIR/extract-verdict.sh"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Usage"* ]]
}

#=============================================================================
# Happy Path Tests
#=============================================================================

@test "test_approved_verdict: extracts approved verdict" {
    local file=$(create_review_file '# Review

Some analysis here.

## Verdict
approved

Additional notes.')
    run "$SCRIPTS_DIR/extract-verdict.sh" "$file" "approved,gaps_found"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "approved" ]]
}

@test "test_gaps_found_verdict: extracts gaps_found verdict" {
    local file=$(create_review_file '## Analysis
Missing coverage for edge cases.

## Verdict
gaps_found')
    run "$SCRIPTS_DIR/extract-verdict.sh" "$file" "approved,gaps_found"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "gaps_found" ]]
}

@test "test_verdict_with_digits: verdicts can contain digits after first character" {
    local file=$(create_review_file '## Verdict
approved2')
    run "$SCRIPTS_DIR/extract-verdict.sh" "$file" "approved2,gaps_found"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "approved2" ]]
}

#=============================================================================
# Verdict Not Found / Invalid Tests
#=============================================================================

@test "test_unknown_verdict: returns unknown when no verdict section" {
    local file=$(create_review_file '## Analysis
The implementation looks good.
No verdict section here.')
    run "$SCRIPTS_DIR/extract-verdict.sh" "$file" "approved,gaps_found"
    [[ "$status" -eq 1 ]]
    [[ "$output" == "unknown" ]]
}

@test "test_verdict_not_in_valid_list: returns unknown for invalid verdict" {
    local file=$(create_review_file '## Verdict
invalid_verdict_name')
    run "$SCRIPTS_DIR/extract-verdict.sh" "$file" "approved,gaps_found"
    [[ "$status" -eq 1 ]]
    [[ "$output" == "unknown" ]]
}

@test "test_empty_file: returns unknown for empty file" {
    local file=$(create_review_file '')
    run "$SCRIPTS_DIR/extract-verdict.sh" "$file" "approved,gaps_found"
    [[ "$status" -eq 1 ]]
    [[ "$output" == "unknown" ]]
}

@test "test_very_long_verdict_line: returns unknown when verdict not in valid list" {
    local file=$(create_review_file '## Verdict
approved_with_a_very_long_explanation_that_goes_on_and_on_and_includes_many_words_about_the_decision_making_process')
    run "$SCRIPTS_DIR/extract-verdict.sh" "$file" "approved,gaps_found"
    [[ "$status" -eq 1 ]]
    [[ "$output" == "unknown" ]]
}

#=============================================================================
# File Error Tests
#=============================================================================

@test "test_file_not_found: fails when file does not exist" {
    run "$SCRIPTS_DIR/extract-verdict.sh" "$TEST_TEMP_DIR/nonexistent.md" "approved,gaps_found"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"unknown"* ]]
}

@test "test_unreadable_file: fails when file is unreadable" {
    # Skip if running as root (root can read anything)
    if [[ "$(id -u)" -eq 0 ]]; then
        skip "Test requires non-root user (root can read files regardless of permissions)"
    fi

    local file="$TEST_TEMP_DIR/unreadable.md"
    printf '## Verdict\napproved' > "$file"
    chmod 000 "$file"

    run "$SCRIPTS_DIR/extract-verdict.sh" "$file" "approved,gaps_found"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"unknown"* ]]
}

#=============================================================================
# Normalization Tests
#=============================================================================

@test "test_case_insensitive: normalizes uppercase to lowercase" {
    local file=$(create_review_file '## Verdict
APPROVED')
    run "$SCRIPTS_DIR/extract-verdict.sh" "$file" "approved,gaps_found"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "approved" ]]
}

@test "test_whitespace_in_verdict: trims leading whitespace" {
    local file=$(create_review_file '## Verdict
  approved')
    run "$SCRIPTS_DIR/extract-verdict.sh" "$file" "approved,gaps_found"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "approved" ]]
}

@test "test_verdict_with_explanation: extracts only first word" {
    local file=$(create_review_file '## Verdict
approved — all tests pass and coverage is good')
    run "$SCRIPTS_DIR/extract-verdict.sh" "$file" "approved,gaps_found"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "approved" ]]
}

@test "test_unicode_in_verdict: stops at non-ASCII character" {
    local file=$(create_review_file '## Verdict
approved✓')
    run "$SCRIPTS_DIR/extract-verdict.sh" "$file" "approved,gaps_found"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "approved" ]]
}

@test "test_whitespace_only_verdict_line: skips whitespace-only lines" {
    local file=$(create_review_file '## Verdict

approved')
    run "$SCRIPTS_DIR/extract-verdict.sh" "$file" "approved,gaps_found"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "approved" ]]
}

@test "test_multiple_verdicts_same_line: extracts first word only" {
    local file=$(create_review_file '## Verdict
approved gaps_found')
    run "$SCRIPTS_DIR/extract-verdict.sh" "$file" "approved,gaps_found"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "approved" ]]
}

@test "test_crlf_line_endings: handles Windows CRLF line endings" {
    local file=$(create_review_file_crlf '## Verdict
approved')
    run "$SCRIPTS_DIR/extract-verdict.sh" "$file" "approved,gaps_found"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "approved" ]]
}

#=============================================================================
# Multiple Verdict Section Tests
#=============================================================================

@test "test_multiple_verdict_sections: uses LAST verdict (final decision)" {
    local file=$(create_review_file '## Initial Assessment

## Verdict
gaps_found

## After Revision

## Verdict
approved')
    run "$SCRIPTS_DIR/extract-verdict.sh" "$file" "approved,gaps_found"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "approved" ]]
}

#=============================================================================
# Code Block Handling Tests
#=============================================================================

@test "test_verdict_inside_code_block_ignored: ignores verdict in code block" {
    local file=$(create_review_file '## Verdict
gaps_found

Example of wrong output:
```
## Verdict
approved
```

## Verdict
gaps_found')
    run "$SCRIPTS_DIR/extract-verdict.sh" "$file" "approved,gaps_found"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "gaps_found" ]]
}

@test "test_multiple_code_blocks: ignores all code blocks" {
    local file=$(create_review_file 'First code block:
```
## Verdict
approved
```

Some text.

```
## Verdict
gaps_found
```

## Verdict
approved')
    run "$SCRIPTS_DIR/extract-verdict.sh" "$file" "approved,gaps_found"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "approved" ]]
}

@test "test_unbalanced_code_blocks: treats content after odd marker as inside code block" {
    local file=$(create_review_file '## Verdict
gaps_found

Here'\''s an example:
```
code block 1
```

Some text

```
This code block is never closed, so anything after here should be treated as inside a code block

## Verdict
approved

This verdict should be ignored because we'\''re inside an unclosed code block.')
    run "$SCRIPTS_DIR/extract-verdict.sh" "$file" "approved,gaps_found"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "gaps_found" ]]
}

@test "code block with language specifier: ignores verdict in tagged code blocks" {
    local file=$(create_review_file '```markdown
## Verdict
approved
```

## Verdict
gaps_found')
    run "$SCRIPTS_DIR/extract-verdict.sh" "$file" "approved,gaps_found"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "gaps_found" ]]
}

#=============================================================================
# Header Format Tests
#=============================================================================

@test "test_verdict_header_with_colon: does not match Verdict: format" {
    local file=$(create_review_file '## Verdict: approved')
    run "$SCRIPTS_DIR/extract-verdict.sh" "$file" "approved,gaps_found"
    [[ "$status" -eq 1 ]]
    [[ "$output" == "unknown" ]]
}

@test "does not match inline Verdict pattern" {
    local file=$(create_review_file 'The verdict is approved.')
    run "$SCRIPTS_DIR/extract-verdict.sh" "$file" "approved,gaps_found"
    [[ "$status" -eq 1 ]]
    [[ "$output" == "unknown" ]]
}

@test "does not match h1 Verdict header" {
    local file=$(create_review_file '# Verdict
approved')
    run "$SCRIPTS_DIR/extract-verdict.sh" "$file" "approved,gaps_found"
    [[ "$status" -eq 1 ]]
    [[ "$output" == "unknown" ]]
}

@test "does not match h3 Verdict header" {
    local file=$(create_review_file '### Verdict
approved')
    run "$SCRIPTS_DIR/extract-verdict.sh" "$file" "approved,gaps_found"
    [[ "$status" -eq 1 ]]
    [[ "$output" == "unknown" ]]
}

@test "does not match Verdict with trailing text on header line" {
    local file=$(create_review_file '## Verdict Section
approved')
    run "$SCRIPTS_DIR/extract-verdict.sh" "$file" "approved,gaps_found"
    [[ "$status" -eq 1 ]]
    [[ "$output" == "unknown" ]]
}

#=============================================================================
# Edge Cases for Valid Verdicts Argument
#=============================================================================

@test "single valid verdict in list: works correctly" {
    local file=$(create_review_file '## Verdict
approved')
    run "$SCRIPTS_DIR/extract-verdict.sh" "$file" "approved"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "approved" ]]
}

@test "many valid verdicts in list: works correctly" {
    local file=$(create_review_file '## Verdict
needs_fix')
    run "$SCRIPTS_DIR/extract-verdict.sh" "$file" "approved,gaps_found,needs_fix,blocked,concerns"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "needs_fix" ]]
}

@test "verdict with underscores: extracts correctly" {
    local file=$(create_review_file '## Verdict
needs_more_work')
    run "$SCRIPTS_DIR/extract-verdict.sh" "$file" "approved,needs_more_work"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "needs_more_work" ]]
}

#=============================================================================
# Content After Verdict Tests
#=============================================================================

@test "verdict followed by more sections: extracts correctly" {
    local file=$(create_review_file '## Verdict
approved

## Next Steps
1. Merge PR
2. Deploy')
    run "$SCRIPTS_DIR/extract-verdict.sh" "$file" "approved,gaps_found"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "approved" ]]
}

@test "verdict at end of file with no newline: extracts correctly" {
    # Use printf without trailing newline
    printf '## Verdict\napproved' > "$TEST_TEMP_DIR/no_newline.md"
    run "$SCRIPTS_DIR/extract-verdict.sh" "$TEST_TEMP_DIR/no_newline.md" "approved,gaps_found"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "approved" ]]
}

#=============================================================================
# Mixed Case and Whitespace in Valid Verdicts List
#=============================================================================

@test "valid verdicts list with spaces around commas: requires strict format without spaces" {
    local file=$(create_review_file '## Verdict
approved')
    # The plan specifies comma-separated list without spaces (e.g., "approved,gaps_found")
    # Spaces around commas are NOT supported - this is by design for simplicity
    run "$SCRIPTS_DIR/extract-verdict.sh" "$file" "approved, gaps_found"
    # With space after comma, "approved" won't match " gaps_found" but should still match "approved"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "approved" ]]
}

#=============================================================================
# Tab and Special Whitespace Tests
#=============================================================================

@test "verdict line with tabs: trims correctly" {
    # Use printf to create file with tab character
    printf '## Verdict\n\tapproved' > "$TEST_TEMP_DIR/tabs.md"
    run "$SCRIPTS_DIR/extract-verdict.sh" "$TEST_TEMP_DIR/tabs.md" "approved,gaps_found"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "approved" ]]
}

@test "multiple whitespace-only lines after Verdict: skips to first content" {
    local file=$(create_review_file '## Verdict



approved')
    run "$SCRIPTS_DIR/extract-verdict.sh" "$file" "approved,gaps_found"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "approved" ]]
}

#=============================================================================
# Regression Tests
#=============================================================================

@test "verdict with dash separator: extracts only verdict word" {
    local file=$(create_review_file '## Verdict
approved - looks good')
    run "$SCRIPTS_DIR/extract-verdict.sh" "$file" "approved,gaps_found"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "approved" ]]
}

@test "verdict starting with number: should fail (identifiers start with letter)" {
    local file=$(create_review_file '## Verdict
2approved')
    run "$SCRIPTS_DIR/extract-verdict.sh" "$file" "approved,2approved"
    # Per the plan, identifiers match ^[a-z][a-z0-9_]*$, so starting with digit is invalid
    [[ "$status" -eq 1 ]]
    [[ "$output" == "unknown" ]]
}

@test "entirely numeric verdict: should fail" {
    local file=$(create_review_file '## Verdict
12345')
    run "$SCRIPTS_DIR/extract-verdict.sh" "$file" "approved,12345"
    [[ "$status" -eq 1 ]]
    [[ "$output" == "unknown" ]]
}

@test "verdict with mixed case in file matches lowercase in list" {
    local file=$(create_review_file '## Verdict
GapsFound')
    run "$SCRIPTS_DIR/extract-verdict.sh" "$file" "approved,gapsfound"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "gapsfound" ]]
}
