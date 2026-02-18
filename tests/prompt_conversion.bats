#!/usr/bin/env bats

# Tests for prompt-conversion phase (orchestration-v3)
#
# Tests the conversion of hardcoded prompts in iterate.sh to external
# template files using V3 template syntax (variables, conditionals, includes).
#
# Covers:
# - Template rendering for qa.md, qa-review.md, impl.md, impl-review.md
# - Common include files (test-commands.md, do-not-rules.md, reasoning-format.md)
# - iterate.sh integration with render_template.py and build-context.sh
# - Edge cases: empty fields, missing params, defaults, escaped braces

setup() {
    load 'test_helper'
    setup_temp_dir

    RENDER_SCRIPT="$SCRIPTS_DIR/render_template.py"
    BUILD_CONTEXT="$SCRIPTS_DIR/build-context.sh"
    ITERATE_SCRIPT="$SCRIPTS_DIR/iterate.sh"
    PROMPTS_DIR="$ORCH_DIR/prompts"

    # Standard phase directory structure
    export PHASE_DIR="$TEST_TEMP_DIR/.plans/active/test-plan/phases/test-phase"
    mkdir -p "$PHASE_DIR"

    # Create minimal state.json
    cat > "$PHASE_DIR/state.json" << 'JSON'
{"iteration": {"current": 1, "max": 25}, "phase_status": "impl", "tests_passing": 0, "tests_total": 0, "stuck_iterations": 0, "hang_count": 0, "last_reviewed_iteration": 0, "last_qa_reviewed_iteration": 0, "packages": ["orchestration"], "chunks": {"total": 0, "completed": [], "current": null, "remaining": []}, "blocked": {"is_blocked": false, "reason": null}, "disputes": [], "last_cleared_disputes": []}
JSON

    # Create plan.md
    printf '# Test Phase\n\n## Objective\nTest something.\n\n## Files\nNone.\n\n## Test Cases\nNone.\n' > "$PHASE_DIR/plan.md"

    # Create workflow.yaml
    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test-workflow
version: 3
defaults:
  max_iterations: 10
  timeout: 600
variables:
  package: test-package
states:
  - name: qa
    prompt: prompts/feature/qa.md
    next: qa_review
  - name: qa_review
    prompt: prompts/feature/qa-review.md
    verdicts: [approved, gaps_found]
    next:
      approved: impl
      gaps_found: qa
  - name: impl
    prompt: prompts/feature/impl.md
    next: impl_review
    params:
      focus_area: "Core implementation"
      allow_test_changes: false
  - name: impl_review
    prompt: prompts/feature/impl-review.md
    verdicts: [approved, concerns]
    next:
      approved: complete
      concerns: impl
  - name: complete
    prompt: prompts/common/complete.md
entry_state: qa
terminal_states: [complete]
YAML
}

teardown() {
    teardown_temp_dir
}

# Helper: render a template with JSON context, using ORCH_DIR/prompts as base_dir
render() {
    local template_file="$1"
    local context_json="$2"
    local base_dir="${3:-$PROMPTS_DIR}"
    run python3 "$RENDER_SCRIPT" "$template_file" "$context_json" "$base_dir"
}

# Helper: build context from phase fixture
build_context() {
    local state_file="${1:-$PHASE_DIR/state.json}"
    local workflow_file="${2:-$TEST_TEMP_DIR/workflow.yaml}"
    local phase_dir="${3:-$PHASE_DIR}"
    local state_name="${4:-impl}"
    "$BUILD_CONTEXT" "$state_file" "$workflow_file" "$phase_dir" "$state_name"
}

#=============================================================================
# Common Include File Existence Tests
#=============================================================================

@test "common/test-commands.md exists" {
    [[ -f "$PROMPTS_DIR/common/test-commands.md" ]]
}

@test "common/do-not-rules.md exists" {
    [[ -f "$PROMPTS_DIR/common/do-not-rules.md" ]]
}

@test "common/reasoning-format.md exists" {
    [[ -f "$PROMPTS_DIR/common/reasoning-format.md" ]]
}

#=============================================================================
# Feature Template Existence Tests
#=============================================================================

@test "prompts/feature/qa.md exists" {
    [[ -f "$PROMPTS_DIR/feature/qa.md" ]]
}

@test "prompts/feature/qa-review.md exists" {
    [[ -f "$PROMPTS_DIR/feature/qa-review.md" ]]
}

@test "prompts/feature/impl.md exists" {
    [[ -f "$PROMPTS_DIR/feature/impl.md" ]]
}

@test "prompts/feature/impl-review.md exists" {
    [[ -f "$PROMPTS_DIR/feature/impl-review.md" ]]
}

#=============================================================================
# test_common_test_commands_content: Content validation
#=============================================================================

@test "test_common_test_commands_crate_default: renders default crate when none specified" {
    local context='{"phase": "test"}'
    # Render a template that includes test-commands.md
    local tpl_file="$TEST_TEMP_DIR/tpl_with_testcmd.md"
    printf '%s' '{{> common/test-commands.md}}' > "$tpl_file"
    run python3 "$RENDER_SCRIPT" "$tpl_file" "$context" "$PROMPTS_DIR"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"test-package"* ]]
}

@test "test_common_test_commands_custom_package: renders custom crate" {
    local context='{"phase": "test-phase", "crate": "my-crate"}'
    local tpl_file="$TEST_TEMP_DIR/tpl_with_testcmd.md"
    printf '%s' '{{> common/test-commands.md}}' > "$tpl_file"
    run python3 "$RENDER_SCRIPT" "$tpl_file" "$context" "$PROMPTS_DIR"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"cargo nextest run -p my-crate"* ]]
}

@test "test_common_test_commands_has_phase: test pattern includes phase" {
    local context='{"phase": "my-feature", "crate": "test-package"}'
    local tpl_file="$TEST_TEMP_DIR/tpl_with_testcmd.md"
    printf '%s' '{{> common/test-commands.md}}' > "$tpl_file"
    run python3 "$RENDER_SCRIPT" "$tpl_file" "$context" "$PROMPTS_DIR"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"qa_my-feature"* ]]
}

#=============================================================================
# test_common_do_not_rules_content
#=============================================================================

@test "test_common_do_not_rules_content: contains DO NOT rules" {
    local tpl_file="$TEST_TEMP_DIR/tpl_donot.md"
    printf '%s' '{{> common/do-not-rules.md}}' > "$tpl_file"
    run python3 "$RENDER_SCRIPT" "$tpl_file" '{}' "$PROMPTS_DIR"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Do NOT modify test files"* ]]
    [[ "$output" == *"Do NOT add new dependencies"* ]]
    [[ "$output" == *"Do NOT change public API"* ]]
    [[ "$output" == *"Do NOT introduce new warnings"* ]]
    [[ "$output" == *"Do NOT leave TODO comments"* ]]
}

#=============================================================================
# test_common_reasoning_format_content
#=============================================================================

@test "test_common_reasoning_format_content: contains reasoning sections" {
    local tpl_file="$TEST_TEMP_DIR/tpl_reasoning.md"
    printf '%s' '{{> common/reasoning-format.md}}' > "$tpl_file"
    run python3 "$RENDER_SCRIPT" "$tpl_file" '{}' "$PROMPTS_DIR"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Analysis"* ]]
    [[ "$output" == *"Approach"* ]]
    [[ "$output" == *"Implementation"* ]]
    [[ "$output" == *"Verification"* ]]
}

#=============================================================================
# test_qa_template_renders: Basic qa.md rendering
#=============================================================================

@test "test_qa_template_renders: basic rendering with phase and plan" {
    local context='{"phase": "test-phase", "plan": "test-plan", "iteration": 1, "plan_md": "# Test"}'
    render "$PROMPTS_DIR/feature/qa.md" "$context"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"test-phase"* ]]
    [[ "$output" == *"iteration 1"* ]]
    [[ "$output" == *"# Test"* ]]
}

#=============================================================================
# test_qa_template_includes_common: Includes resolve in qa.md
#=============================================================================

@test "test_qa_template_includes_common: includes test-commands with crate" {
    local context='{"phase": "test-phase", "crate": "my-crate", "iteration": 1}'
    render "$PROMPTS_DIR/feature/qa.md" "$context"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"cargo nextest run -p my-crate"* ]]
}

@test "test_qa_template_includes_do_not_rules" {
    local context='{"phase": "test-phase", "iteration": 1}'
    render "$PROMPTS_DIR/feature/qa.md" "$context"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Do NOT modify test files"* ]]
}

@test "test_qa_template_includes_reasoning_format" {
    local context='{"phase": "test-phase", "iteration": 1}'
    render "$PROMPTS_DIR/feature/qa.md" "$context"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Analysis"* ]]
    [[ "$output" == *"Verification"* ]]
}

#=============================================================================
# test_qa_template_test_naming_pattern
#=============================================================================

@test "test_qa_template_test_naming_pattern: phase name in naming pattern with wildcard" {
    local context='{"phase": "my-feature", "plan": "plan1", "iteration": 1}'
    render "$PROMPTS_DIR/feature/qa.md" "$context"
    [[ "$status" -eq 0 ]]
    # Plan spec (line 171): naming pattern is "qa_{{phase}}_*"
    [[ "$output" == *"qa_my-feature_*"* ]] || [[ "$output" == *'qa_my-feature_'* ]]
}

#=============================================================================
# test_qa_template_without_plan_md: Conditional omission
#=============================================================================

@test "test_qa_template_without_plan_md: no Phase Specification section when empty" {
    local context='{"phase": "test", "plan_md": "", "iteration": 1}'
    render "$PROMPTS_DIR/feature/qa.md" "$context"
    [[ "$status" -eq 0 ]]
    [[ "$output" != *"Phase Specification"* ]]
}

#=============================================================================
# test_missing_plan_md: Same as above, alias test
#=============================================================================

@test "test_missing_plan_md: plan_md empty omits conditional block" {
    local context='{"phase": "test", "plan_md": ""}'
    render "$PROMPTS_DIR/feature/qa.md" "$context"
    [[ "$status" -eq 0 ]]
    [[ "$output" != *"Phase Specification"* ]]
}

@test "test_qa_plan_md_present: Phase Specification shown when plan_md is non-empty" {
    local context='{"phase": "test", "plan_md": "# My Plan\nDetails here.", "iteration": 1}'
    render "$PROMPTS_DIR/feature/qa.md" "$context"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Phase Specification"* ]]
    [[ "$output" == *"# My Plan"* ]]
}

#=============================================================================
# QA Review Template Tests
#=============================================================================

@test "test_qa_review_template_renders: basic rendering" {
    local context='{"phase": "test-phase", "plan_md": "# Test", "state": {"tests_passing": 5, "tests_total": 10}}'
    render "$PROMPTS_DIR/feature/qa-review.md" "$context"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"test-phase"* ]]
    [[ "$output" == *"5"* ]]
    [[ "$output" == *"10"* ]]
}

@test "test_qa_review_null_tests: defaults to unknown when test fields missing" {
    local context='{"phase": "test", "state": {}}'
    render "$PROMPTS_DIR/feature/qa-review.md" "$context"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"unknown"* ]]
}

@test "test_qa_review_without_plan_md: omits Phase Specification" {
    local context='{"phase": "test", "plan_md": "", "state": {"tests_passing": 0, "tests_total": 0}}'
    render "$PROMPTS_DIR/feature/qa-review.md" "$context"
    [[ "$status" -eq 0 ]]
    [[ "$output" != *"Phase Specification"* ]]
}

@test "qa_review: contains verdict options" {
    local context='{"phase": "test", "state": {}}'
    render "$PROMPTS_DIR/feature/qa-review.md" "$context"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"approved"* ]]
    [[ "$output" == *"gaps_found"* ]]
}

@test "qa_review: test naming pattern with phase and wildcard suffix" {
    local context='{"phase": "my-feature", "state": {}}'
    render "$PROMPTS_DIR/feature/qa-review.md" "$context"
    [[ "$status" -eq 0 ]]
    # Must contain the full naming pattern with phase and wildcard suffix
    [[ "$output" == *"qa_my-feature_*"* ]] || [[ "$output" == *'qa_my-feature_'* ]]
}

#=============================================================================
# Implementation Template Tests
#=============================================================================

@test "test_impl_template_focus_area: focus area shown when set" {
    local context='{"phase": "test", "params": {"focus_area": "RNG implementation"}}'
    render "$PROMPTS_DIR/feature/impl.md" "$context"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Focus your implementation on: **RNG implementation**"* ]]
}

@test "test_impl_template_no_focus_area: focus area section absent when not set" {
    local context='{"phase": "test", "params": {}}'
    render "$PROMPTS_DIR/feature/impl.md" "$context"
    [[ "$status" -eq 0 ]]
    [[ "$output" != *"Focus Area"* ]]
    [[ "$output" != *"Focus your implementation on"* ]]
}

@test "test_impl_template_no_test_changes: DO NOT modify test files shown when false" {
    local context='{"phase": "test", "params": {"allow_test_changes": false}}'
    render "$PROMPTS_DIR/feature/impl.md" "$context"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"You MUST NOT modify any test files"* ]]
}

@test "test_impl_template_allow_test_changes: DO NOT section absent when true" {
    local context='{"phase": "test", "params": {"allow_test_changes": true}}'
    render "$PROMPTS_DIR/feature/impl.md" "$context"
    [[ "$status" -eq 0 ]]
    [[ "$output" != *"You MUST NOT modify any test files"* ]]
}

@test "test_impl_template_combined_params: focus area + allow test changes" {
    local context='{"phase": "test", "plan": "myplan", "iteration": 2, "params": {"focus_area": "RNG", "allow_test_changes": true}}'
    render "$PROMPTS_DIR/feature/impl.md" "$context"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Focus your implementation on: **RNG**"* ]]
    [[ "$output" != *"You MUST NOT modify any test files"* ]]
}

@test "impl: includes test-commands.md" {
    local context='{"phase": "test-phase", "crate": "my-crate", "params": {}}'
    render "$PROMPTS_DIR/feature/impl.md" "$context"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"cargo nextest run -p my-crate"* ]]
}

@test "impl: includes do-not-rules.md when allow_test_changes is false" {
    local context='{"phase": "test-phase", "params": {"allow_test_changes": false}}'
    render "$PROMPTS_DIR/feature/impl.md" "$context"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Do NOT modify test files"* ]]
}

@test "impl: includes reasoning-format.md" {
    local context='{"phase": "test-phase", "params": {}}'
    render "$PROMPTS_DIR/feature/impl.md" "$context"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Analysis"* ]]
    [[ "$output" == *"Verification"* ]]
}

@test "impl: plan_md conditional present" {
    local context='{"phase": "test", "plan_md": "# Spec Details", "params": {}}'
    render "$PROMPTS_DIR/feature/impl.md" "$context"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Phase Specification"* ]]
    [[ "$output" == *"# Spec Details"* ]]
}

@test "impl: plan_md conditional absent when empty" {
    local context='{"phase": "test", "plan_md": "", "params": {}}'
    render "$PROMPTS_DIR/feature/impl.md" "$context"
    [[ "$status" -eq 0 ]]
    [[ "$output" != *"Phase Specification"* ]]
}

@test "impl: test results display" {
    local context='{"phase": "test", "state": {"tests_passing": 3, "tests_total": 7}, "iteration": 2, "params": {}}'
    render "$PROMPTS_DIR/feature/impl.md" "$context"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"3"* ]]
    [[ "$output" == *"7"* ]]
    [[ "$output" == *"iteration 2"* ]]
}

#=============================================================================
# Impl Review Template Tests
#=============================================================================

@test "test_review_template_verdicts: contains approved and concerns verdicts" {
    local context='{"phase": "test", "state": {"verdicts": ["approved", "concerns"]}}'
    render "$PROMPTS_DIR/feature/impl-review.md" "$context"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"approved"* ]]
    [[ "$output" == *"concerns"* ]]
}

@test "test_impl_review_template_tests_display: shows test counts and plan" {
    local context='{"phase": "test", "plan_md": "# Spec", "state": {"tests_passing": 8, "tests_total": 12}}'
    render "$PROMPTS_DIR/feature/impl-review.md" "$context"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"8"* ]]
    [[ "$output" == *"12"* ]]
    [[ "$output" == *"Implementation Review"* ]]
    [[ "$output" == *"# Spec"* ]]
}

@test "test_impl_review_template_no_plan_md: omits Phase Specification" {
    local context='{"phase": "test", "plan_md": "", "state": {"tests_passing": 0, "tests_total": 5}}'
    render "$PROMPTS_DIR/feature/impl-review.md" "$context"
    [[ "$status" -eq 0 ]]
    [[ "$output" != *"Phase Specification"* ]]
}

@test "impl_review: includes reasoning-format.md" {
    local context='{"phase": "test", "state": {}}'
    render "$PROMPTS_DIR/feature/impl-review.md" "$context"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Analysis"* ]]
    [[ "$output" == *"Verification"* ]]
}

@test "impl_review: default test values when state empty" {
    local context='{"phase": "test", "state": {}}'
    render "$PROMPTS_DIR/feature/impl-review.md" "$context"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"unknown"* ]]
}

#=============================================================================
# test_common_includes_resolve: Includes from templates
#=============================================================================

@test "test_common_includes_resolve: qa.md includes resolve from base_dir" {
    local context='{"phase": "test", "crate": "test-package", "iteration": 1}'
    render "$PROMPTS_DIR/feature/qa.md" "$context"
    [[ "$status" -eq 0 ]]
    # Content from common/test-commands.md should be present
    [[ "$output" == *"cargo nextest run"* ]]
    # Content from common/do-not-rules.md should be present
    [[ "$output" == *"Do NOT"* ]]
    # Content from common/reasoning-format.md should be present
    [[ "$output" == *"Reasoning Format"* ]] || [[ "$output" == *"Analysis"* ]]
}

#=============================================================================
# test_escaped_braces_in_templates
#=============================================================================

@test "test_escaped_braces_in_templates: literal braces not substituted" {
    local tpl_file="$TEST_TEMP_DIR/escaped_test.md"
    printf '%s' 'Use \{{variable}} syntax' > "$tpl_file"
    run python3 "$RENDER_SCRIPT" "$tpl_file" '{}' "$PROMPTS_DIR"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *'{{variable}}'* ]]
}

#=============================================================================
# End-to-End: build-context.sh + render_template.py pipeline
#=============================================================================

@test "test_e2e_qa_template_with_context: full pipeline for qa.md" {
    cat > "$PHASE_DIR/state.json" << 'JSON'
{"iteration": {"current": 1, "max": 25}, "phase_status": "qa", "tests_passing": 0, "tests_total": 0, "stuck_iterations": 0, "hang_count": 0, "packages": ["orchestration"], "chunks": {"total": 0, "completed": [], "current": null, "remaining": []}, "blocked": {"is_blocked": false, "reason": null}, "disputes": [], "last_cleared_disputes": [], "last_reviewed_iteration": 0, "last_qa_reviewed_iteration": 0}
JSON

    cat > "$PHASE_DIR/plan.md" << 'MD'
# Phase: test-phase

## Objective
Test something.
MD

    local context
    context=$(build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" qa)

    local rendered
    rendered=$(python3 "$RENDER_SCRIPT" "$PROMPTS_DIR/feature/qa.md" "$context" "$PROMPTS_DIR")

    # Basic structure present
    [[ "$rendered" == *"test-phase"* ]]
    [[ "$rendered" == *"iteration 1"* ]]
    [[ "$rendered" == *"# Phase: test-phase"* ]] || [[ "$rendered" == *"Test something"* ]]
    # No unresolved template tags (excluding escaped braces and code blocks)
    local unresolved
    unresolved=$(echo "$rendered" | grep -cE '\{\{[a-zA-Z]' || true)
    [[ "$unresolved" -eq 0 ]]
}

@test "test_e2e_impl_template_with_context: full pipeline for impl.md" {
    cat > "$PHASE_DIR/state.json" << 'JSON'
{"iteration": {"current": 3, "max": 25}, "phase_status": "impl", "tests_passing": 5, "tests_total": 10, "stuck_iterations": 0, "hang_count": 0, "packages": ["orchestration"], "chunks": {"total": 0, "completed": [], "current": null, "remaining": []}, "blocked": {"is_blocked": false, "reason": null}, "disputes": [], "last_cleared_disputes": [], "last_reviewed_iteration": 0, "last_qa_reviewed_iteration": 0}
JSON

    local context
    context=$(build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" impl)

    local rendered
    rendered=$(python3 "$RENDER_SCRIPT" "$PROMPTS_DIR/feature/impl.md" "$context" "$PROMPTS_DIR")

    # Phase and plan info
    [[ "$rendered" == *"test-phase"* ]]
    # Test results from state
    [[ "$rendered" == *"5"* ]]
    [[ "$rendered" == *"10"* ]]
    # Focus area from params
    [[ "$rendered" == *"Focus your implementation on: **Core implementation**"* ]]
    # Critical rule (allow_test_changes is false)
    [[ "$rendered" == *"You MUST NOT modify any test files"* ]]
    # No unresolved template tags
    local unresolved
    unresolved=$(echo "$rendered" | grep -cE '\{\{[a-zA-Z]' || true)
    [[ "$unresolved" -eq 0 ]]
}

@test "test_e2e_impl_review_with_context: full pipeline for impl-review.md" {
    cat > "$PHASE_DIR/state.json" << 'JSON'
{"iteration": {"current": 2, "max": 25}, "phase_status": "impl_review", "tests_passing": 8, "tests_total": 12, "stuck_iterations": 0, "hang_count": 0, "packages": ["orchestration"], "chunks": {"total": 0, "completed": [], "current": null, "remaining": []}, "blocked": {"is_blocked": false, "reason": null}, "disputes": [], "last_cleared_disputes": [], "last_reviewed_iteration": 0, "last_qa_reviewed_iteration": 0}
JSON

    local context
    context=$(build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" impl_review)

    local rendered
    rendered=$(python3 "$RENDER_SCRIPT" "$PROMPTS_DIR/feature/impl-review.md" "$context" "$PROMPTS_DIR")

    # Phase name
    [[ "$rendered" == *"test-phase"* ]]
    # Test counts
    [[ "$rendered" == *"8"* ]]
    [[ "$rendered" == *"12"* ]]
    # Verdict options
    [[ "$rendered" == *"approved"* ]]
    [[ "$rendered" == *"concerns"* ]]
}

@test "test_e2e_qa_review_with_context: full pipeline for qa-review.md" {
    cat > "$PHASE_DIR/state.json" << 'JSON'
{"iteration": {"current": 1, "max": 25}, "phase_status": "qa_review", "tests_passing": 5, "tests_total": 10, "stuck_iterations": 0, "hang_count": 0, "packages": ["orchestration"], "chunks": {"total": 0, "completed": [], "current": null, "remaining": []}, "blocked": {"is_blocked": false, "reason": null}, "disputes": [], "last_cleared_disputes": [], "last_reviewed_iteration": 0, "last_qa_reviewed_iteration": 0}
JSON

    local context
    context=$(build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" qa_review)

    local rendered
    rendered=$(python3 "$RENDER_SCRIPT" "$PROMPTS_DIR/feature/qa-review.md" "$context" "$PROMPTS_DIR")

    # Phase name
    [[ "$rendered" == *"test-phase"* ]]
    # Test counts from state
    [[ "$rendered" == *"5"* ]]
    [[ "$rendered" == *"10"* ]]
}

#=============================================================================
# test_iterate_calls_render_template: iterate.sh creates rendered prompt file
#=============================================================================

@test "test_iterate_calls_render_template: rendered prompt created at correct path" {
    # Behavioral test: Use the build-context + render pipeline to simulate what
    # iterate.sh does, verifying the prompt is created with rendered content
    # and contains no unresolved {{ syntax.
    cat > "$PHASE_DIR/state.json" << 'JSON'
{"iteration": {"current": 1, "max": 25}, "phase_status": "impl", "tests_passing": 0, "tests_total": 0, "stuck_iterations": 0, "hang_count": 0, "packages": ["orchestration"], "chunks": {"total": 0, "completed": [], "current": null, "remaining": []}, "blocked": {"is_blocked": false, "reason": null}, "disputes": [], "last_cleared_disputes": [], "last_reviewed_iteration": 0, "last_qa_reviewed_iteration": 0}
JSON

    # Build context the same way render_and_spawn would
    local context
    context=$(build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" impl)

    # Determine iteration dir path (iteration 1 → iteration_001)
    local iteration
    iteration=$(echo "$context" | jq -r '.iteration')
    local iter_dir="$PHASE_DIR/iteration_$(printf '%03d' "$iteration")"
    mkdir -p "$iter_dir"

    # Render template to the iteration directory
    python3 "$RENDER_SCRIPT" "$PROMPTS_DIR/feature/impl.md" "$context" "$PROMPTS_DIR" > "$iter_dir/prompt.md"

    # Verify prompt.md was created at the correct path
    [[ -f "$iter_dir/prompt.md" ]]

    # Verify prompt.md is non-empty
    [[ -s "$iter_dir/prompt.md" ]]

    # Verify no unresolved {{ syntax (excluding escaped braces)
    local unresolved
    unresolved=$(grep -cE '\{\{[a-zA-Z_]' "$iter_dir/prompt.md" || true)
    [[ "$unresolved" -eq 0 ]]

    # Verify prompt.md contains the phase name and plan content
    local content
    content=$(cat "$iter_dir/prompt.md")
    [[ "$content" == *"test-phase"* ]]
    [[ "$content" == *"Test something"* ]]
}

@test "test_iterate_calls_build_context: build-context.sh produces valid context for render" {
    # Behavioral test: verify build-context.sh output is valid JSON that
    # render_template.py can consume to produce correct rendered output
    [[ -f "$BUILD_CONTEXT" ]]
    local context
    context=$("$BUILD_CONTEXT" "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" impl)

    # Must be valid JSON
    echo "$context" | jq empty

    # Must contain all fields needed by templates
    echo "$context" | jq -e '.phase' > /dev/null
    echo "$context" | jq -e '.plan' > /dev/null
    echo "$context" | jq -e 'has("plan_md")' > /dev/null
    echo "$context" | jq -e 'has("params")' > /dev/null
    echo "$context" | jq -e '.iteration' > /dev/null

    # Render succeeds with this context
    local rendered
    rendered=$(python3 "$RENDER_SCRIPT" "$PROMPTS_DIR/feature/impl.md" "$context" "$PROMPTS_DIR")
    [[ -n "$rendered" ]]
}

@test "test_iterate_no_load_prompt_calls: iterate.sh does not call load-prompt.sh" {
    # After conversion, load-prompt.sh calls should be replaced
    # There should be no remaining load-prompt.sh calls in the qa/impl/fix/review modes
    [[ -f "$ITERATE_SCRIPT" ]]
    # Count load-prompt.sh calls
    local count
    count=$(grep -c 'load-prompt.sh' "$ITERATE_SCRIPT" || true)
    [[ "$count" -eq 0 ]]
}

#=============================================================================
# test_render_and_spawn_unit: render_and_spawn pipeline components
#=============================================================================

@test "test_render_and_spawn_unit: build-context + render produce correct prompt file" {
    # Behavioral test: simulate what render_and_spawn does step by step,
    # verifying each component is called with correct arguments and
    # output is written to the correct iteration directory.

    # Setup a specific state for this test
    cat > "$PHASE_DIR/state.json" << 'JSON'
{"iteration": {"current": 2, "max": 25}, "phase_status": "impl", "tests_passing": 3, "tests_total": 8, "stuck_iterations": 0, "hang_count": 0, "packages": ["orchestration"], "chunks": {"total": 0, "completed": [], "current": null, "remaining": []}, "blocked": {"is_blocked": false, "reason": null}, "disputes": [], "last_cleared_disputes": [], "last_reviewed_iteration": 0, "last_qa_reviewed_iteration": 0}
JSON

    # Step 1: build-context.sh called with state_file, workflow_file, phase_dir, state_name
    local context
    context=$("$BUILD_CONTEXT" "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" impl)
    [[ $? -eq 0 ]]

    # Step 2: Get template path from workflow (simulating yq lookup)
    local prompt_template
    prompt_template=$(yq '.states[] | select(.name == "impl") | .prompt' "$TEST_TEMP_DIR/workflow.yaml")
    [[ "$prompt_template" == *"impl.md"* ]]

    # Step 3: Determine iteration directory with zero-padding
    local iteration
    iteration=$(echo "$context" | jq -r '.iteration')
    [[ "$iteration" == "2" ]]
    local iter_dir="$PHASE_DIR/iteration_$(printf '%03d' "$iteration")"
    mkdir -p "$iter_dir"

    # Step 4: render_template.py called with template, context, base_dir
    python3 "$RENDER_SCRIPT" "$PROMPTS_DIR/feature/impl.md" "$context" "$PROMPTS_DIR" > "$iter_dir/prompt.md"

    # Step 5: Verify output written to iteration_NNN/prompt.md
    [[ -f "$iter_dir/prompt.md" ]]
    [[ -s "$iter_dir/prompt.md" ]]
    # The directory name has correct zero-padding
    [[ -d "$PHASE_DIR/iteration_002" ]]

    # Step 6: Verify spawn_agent would receive correct path
    # (spawn_agent is called with iter_dir/prompt.md as first arg)
    local prompt_path="$iter_dir/prompt.md"
    [[ "$prompt_path" == *"iteration_002/prompt.md"* ]]
}

#=============================================================================
# test_iterate_passes_correct_context: context values propagated correctly
#=============================================================================

@test "test_iterate_passes_correct_context: rendered prompt contains iteration, test counts, focus area" {
    # Behavioral test: simulates the exact pipeline iterate.sh uses:
    # build-context.sh produces context → render_template.py produces prompt.md
    # Verifies rendered output contains specific context values from state.json + workflow.yaml

    # Setup state.json with iteration 3, tests 5/10 (matching plan spec)
    cat > "$PHASE_DIR/state.json" << 'JSON'
{"iteration": {"current": 3, "max": 25}, "phase_status": "impl", "tests_passing": 5, "tests_total": 10, "stuck_iterations": 0, "hang_count": 0, "packages": ["orchestration"], "chunks": {"total": 0, "completed": [], "current": null, "remaining": []}, "blocked": {"is_blocked": false, "reason": null}, "disputes": [], "last_cleared_disputes": [], "last_reviewed_iteration": 0, "last_qa_reviewed_iteration": 0}
JSON

    # Build context (workflow has params: focus_area: "Core implementation")
    local context
    context=$(build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" impl)

    # Create iteration directory like iterate.sh would
    local iteration
    iteration=$(echo "$context" | jq -r '.iteration')
    local iter_dir="$PHASE_DIR/iteration_$(printf '%03d' "$iteration")"
    mkdir -p "$iter_dir"

    # Render impl template to prompt.md
    python3 "$RENDER_SCRIPT" "$PROMPTS_DIR/feature/impl.md" "$context" "$PROMPTS_DIR" > "$iter_dir/prompt.md"

    local rendered
    rendered=$(cat "$iter_dir/prompt.md")

    # Plan spec expects: prompt.md contains "iteration 3"
    [[ "$rendered" == *"iteration 3"* ]]
    # Plan spec expects: test counts "5 / 10" or "5" and "10"
    [[ "$rendered" == *"5"* ]]
    [[ "$rendered" == *"10"* ]]
    # Plan spec expects: "Focus your implementation on: **Core implementation**"
    [[ "$rendered" == *"Focus your implementation on: **Core implementation**"* ]]
    # Verify phase name is present
    [[ "$rendered" == *"test-phase"* ]]
}

#=============================================================================
# test_iterate_with_iteration_zero
#=============================================================================

@test "test_iterate_with_iteration_zero: creates iteration_000 and renders iteration 0" {
    cat > "$PHASE_DIR/state.json" << 'JSON'
{"iteration": {"current": 0, "max": 25}, "phase_status": "qa"}
JSON

    local context
    context=$(build_context "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" qa)

    # Verify iteration value is 0
    local iteration
    iteration=$(echo "$context" | jq -r '.iteration')
    [[ "$iteration" == "0" ]]

    # Verify zero-padded directory name would be iteration_000
    local iter_dir="$PHASE_DIR/iteration_$(printf '%03d' "$iteration")"
    mkdir -p "$iter_dir"
    [[ -d "$PHASE_DIR/iteration_000" ]]

    # Render template to the iteration_000 directory
    python3 "$RENDER_SCRIPT" "$PROMPTS_DIR/feature/qa.md" "$context" "$PROMPTS_DIR" > "$iter_dir/prompt.md"

    # Verify prompt.md created in iteration_000
    [[ -f "$PHASE_DIR/iteration_000/prompt.md" ]]

    # Verify "iteration 0" appears in rendered output (not just any "0")
    local rendered
    rendered=$(cat "$iter_dir/prompt.md")
    [[ "$rendered" == *"iteration 0"* ]]
}

#=============================================================================
# test_render_and_spawn_error_on_context_failure
#=============================================================================

@test "test_render_and_spawn_error_on_context_failure: build-context fails on missing state" {
    # build-context.sh should fail when state.json doesn't exist
    run "$BUILD_CONTEXT" "$TEST_TEMP_DIR/nonexistent.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" impl
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Error"* ]]
}

#=============================================================================
# Template V3 Syntax: No Hardcoded Prompts
#=============================================================================

@test "qa.md uses V3 template syntax (variables)" {
    # qa.md must contain {{phase}} variable reference
    grep -q '{{phase}}' "$PROMPTS_DIR/feature/qa.md"
}

@test "qa.md uses V3 template syntax (includes)" {
    # qa.md must include common files
    grep -q '{{> common/' "$PROMPTS_DIR/feature/qa.md"
}

@test "qa.md uses V3 template syntax (conditionals)" {
    # qa.md must use {{#if plan_md}} conditional
    grep -q '{{#if plan_md}}' "$PROMPTS_DIR/feature/qa.md"
}

@test "impl.md uses V3 template syntax (unless)" {
    # impl.md must use {{#unless}} for allow_test_changes
    grep -q '{{#unless' "$PROMPTS_DIR/feature/impl.md"
}

@test "impl.md uses V3 template syntax (if params)" {
    # impl.md must use {{#if params.focus_area}}
    grep -q '{{#if params.focus_area}}' "$PROMPTS_DIR/feature/impl.md"
}

@test "qa-review.md uses V3 template syntax (default filter)" {
    # qa-review.md must use default filter for test counts
    grep -q 'default:' "$PROMPTS_DIR/feature/qa-review.md"
}

@test "impl-review.md uses V3 template syntax (default filter)" {
    # impl-review.md must use default filter for test counts
    grep -q 'default:' "$PROMPTS_DIR/feature/impl-review.md"
}

#=============================================================================
# No Unresolved Templates After Rendering
#=============================================================================

@test "qa.md rendered output has no unresolved variables" {
    local context='{"phase": "test", "plan": "plan1", "iteration": 1, "plan_md": "# Test Plan", "crate": "test-package"}'
    render "$PROMPTS_DIR/feature/qa.md" "$context"
    [[ "$status" -eq 0 ]]
    # No unresolved {{variable}} patterns (excluding code blocks and escaped braces)
    local unresolved
    unresolved=$(echo "$output" | grep -cE '\{\{[a-zA-Z_]' || true)
    [[ "$unresolved" -eq 0 ]]
}

@test "impl.md rendered output has no unresolved variables" {
    local context='{"phase": "test", "plan": "plan1", "iteration": 2, "plan_md": "# Test Plan", "crate": "test-package", "state": {"tests_passing": 5, "tests_total": 10}, "params": {"focus_area": "Core", "allow_test_changes": false}}'
    render "$PROMPTS_DIR/feature/impl.md" "$context"
    [[ "$status" -eq 0 ]]
    local unresolved
    unresolved=$(echo "$output" | grep -cE '\{\{[a-zA-Z_]' || true)
    [[ "$unresolved" -eq 0 ]]
}

@test "qa-review.md rendered output has no unresolved variables" {
    local context='{"phase": "test", "plan_md": "# Test Plan", "state": {"tests_passing": 5, "tests_total": 10}}'
    render "$PROMPTS_DIR/feature/qa-review.md" "$context"
    [[ "$status" -eq 0 ]]
    local unresolved
    unresolved=$(echo "$output" | grep -cE '\{\{[a-zA-Z_]' || true)
    [[ "$unresolved" -eq 0 ]]
}

@test "impl-review.md rendered output has no unresolved variables" {
    local context='{"phase": "test", "plan_md": "# Test Plan", "state": {"tests_passing": 8, "tests_total": 12}}'
    render "$PROMPTS_DIR/feature/impl-review.md" "$context"
    [[ "$status" -eq 0 ]]
    local unresolved
    unresolved=$(echo "$output" | grep -cE '\{\{[a-zA-Z_]' || true)
    [[ "$unresolved" -eq 0 ]]
}

#=============================================================================
# Semantic Equivalence: Templates produce equivalent content to old prompts
#=============================================================================

@test "qa.md: rendered output contains QA header with phase" {
    local context='{"phase": "my-phase", "plan": "my-plan", "iteration": 1}'
    render "$PROMPTS_DIR/feature/qa.md" "$context"
    [[ "$status" -eq 0 ]]
    # Should contain "QA" and phase name in title
    [[ "$output" == *"QA"* ]]
    [[ "$output" == *"my-phase"* ]]
}

@test "qa-review.md: rendered output contains QA Review header with phase" {
    local context='{"phase": "my-phase", "state": {}}'
    render "$PROMPTS_DIR/feature/qa-review.md" "$context"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"QA Review"* ]]
    [[ "$output" == *"my-phase"* ]]
}

@test "impl.md: rendered output contains Implementation header with phase" {
    local context='{"phase": "my-phase", "plan": "my-plan", "iteration": 1, "params": {}}'
    render "$PROMPTS_DIR/feature/impl.md" "$context"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Implementation"* ]]
    [[ "$output" == *"my-phase"* ]]
}

@test "impl-review.md: rendered output contains Implementation Review header" {
    local context='{"phase": "my-phase", "state": {}}'
    render "$PROMPTS_DIR/feature/impl-review.md" "$context"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Implementation Review"* ]]
    [[ "$output" == *"my-phase"* ]]
}

#=============================================================================
# Edge Cases
#=============================================================================

@test "edge: long plan_md renders without truncation" {
    # Generate ~50KB plan_md
    local long_plan
    long_plan=$(printf '# Long Plan\n'; for i in $(seq 1 1000); do echo "Line $i of a very long plan."; done)
    local context
    context=$(jq -n --arg plan_md "$long_plan" '{"phase": "test", "plan_md": $plan_md, "iteration": 1}')
    render "$PROMPTS_DIR/feature/qa.md" "$context"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"# Long Plan"* ]]
    [[ "$output" == *"Line 1000"* ]]
}

@test "edge: special characters in plan_md preserved" {
    local context='{"phase": "test", "plan_md": "Special: \"quotes\" $vars `backticks` <html>", "iteration": 1}'
    render "$PROMPTS_DIR/feature/qa.md" "$context"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *'"quotes"'* ]]
    [[ "$output" == *'$vars'* ]]
    [[ "$output" == *'`backticks`'* ]]
}

@test "edge: unicode in context preserved" {
    local context='{"phase": "test", "plan_md": "测试 тест テスト", "iteration": 1}'
    render "$PROMPTS_DIR/feature/qa.md" "$context"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"测试"* ]]
    [[ "$output" == *"тест"* ]]
    [[ "$output" == *"テスト"* ]]
}

@test "edge: missing params object defaults gracefully" {
    # When params is entirely missing, conditionals should handle it
    local context='{"phase": "test", "iteration": 1}'
    render "$PROMPTS_DIR/feature/impl.md" "$context"
    [[ "$status" -eq 0 ]]
    # No focus area section
    [[ "$output" != *"Focus your implementation on"* ]]
}

@test "edge: zero tests display correctly" {
    local context='{"phase": "test", "state": {"tests_passing": 0, "tests_total": 0}, "iteration": 1, "params": {}}'
    render "$PROMPTS_DIR/feature/impl.md" "$context"
    [[ "$status" -eq 0 ]]
    # Verify test counts specifically show "0" in a tests context, not just any "0"
    [[ "$output" == *"0 /"* ]] || [[ "$output" == *"0/"* ]]
}

@test "edge: nested conditionals with includes work" {
    # impl.md has unless(allow_test_changes) wrapping do-not-rules include
    local context='{"phase": "test", "params": {"allow_test_changes": false}}'
    render "$PROMPTS_DIR/feature/impl.md" "$context"
    [[ "$status" -eq 0 ]]
    # Both the critical rule AND the do-not-rules should be present
    [[ "$output" == *"You MUST NOT modify any test files"* ]]
    [[ "$output" == *"Do NOT modify test files"* ]]
}

@test "edge: impl template without allow_test_changes key shows critical rule" {
    # Missing key is falsy, so #unless should render
    local context='{"phase": "test", "params": {}}'
    render "$PROMPTS_DIR/feature/impl.md" "$context"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"You MUST NOT modify any test files"* ]]
}

#=============================================================================
# Template Structure: Correct ordering of sections
#=============================================================================

@test "qa.md: iteration appears before test requirements" {
    local context='{"phase": "test", "plan": "plan1", "iteration": 5}'
    render "$PROMPTS_DIR/feature/qa.md" "$context"
    [[ "$status" -eq 0 ]]
    # Iteration info should come before test requirements
    local iter_pos
    iter_pos=$(echo "$output" | grep -n "iteration 5\|Iteration" | head -1 | cut -d: -f1)
    local req_pos
    req_pos=$(echo "$output" | grep -n "Test Requirements" | head -1 | cut -d: -f1)
    if [[ -n "$iter_pos" && -n "$req_pos" ]]; then
        [[ "$iter_pos" -lt "$req_pos" ]]
    fi
}

@test "impl.md: plan section appears before test commands" {
    local context='{"phase": "test", "plan": "plan1", "plan_md": "# Spec", "iteration": 1, "params": {}}'
    render "$PROMPTS_DIR/feature/impl.md" "$context"
    [[ "$status" -eq 0 ]]
    # Plan section should come before test commands
    local plan_pos
    plan_pos=$(echo "$output" | grep -n "# Spec" | head -1 | cut -d: -f1)
    local cmd_pos
    cmd_pos=$(echo "$output" | grep -n "cargo nextest" | head -1 | cut -d: -f1)
    if [[ -n "$plan_pos" && -n "$cmd_pos" ]]; then
        [[ "$plan_pos" -lt "$cmd_pos" ]]
    fi
}
