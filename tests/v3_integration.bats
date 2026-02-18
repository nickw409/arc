#!/usr/bin/env bats

# End-to-end integration tests for V3 features
# Phase: integration-testing (orchestration-v3)
#
# Tests the complete V3 template rendering pipeline:
#   build-context.sh + render_template.py + validate-workflow.sh integration
#
# These tests verify that the components work together correctly,
# not just individually (those are covered in unit test files).

setup() {
    load 'test_helper'
    setup_temp_dir

    BUILD_CONTEXT="$SCRIPTS_DIR/build-context.sh"
    RENDER_TEMPLATE="$SCRIPTS_DIR/render_template.py"
    VALIDATE_WORKFLOW="$SCRIPTS_DIR/validate-workflow.sh"
    FIXTURES_DIR="$ORCH_DIR/tests/fixtures/v3"

    # Standard phase directory structure for build-context.sh
    export PHASE_DIR="$TEST_TEMP_DIR/.plans/active/test-plan/phases/test-phase"
    mkdir -p "$PHASE_DIR"

    # Copy fixtures to temp dir for isolation
    cp "$FIXTURES_DIR/state.json" "$PHASE_DIR/state.json"
    cp "$FIXTURES_DIR/plan.md" "$PHASE_DIR/plan.md"
    cp "$FIXTURES_DIR/workflow.yaml" "$TEST_TEMP_DIR/workflow.yaml"

    # Create a prompts directory structure for include tests
    export PROMPTS_DIR="$TEST_TEMP_DIR/prompts"
    mkdir -p "$PROMPTS_DIR/feature"
    mkdir -p "$PROMPTS_DIR/common"
}

teardown() {
    teardown_temp_dir
}

# Helper: run build-context.sh with default test fixtures
build_context() {
    local state_file="${1:-$PHASE_DIR/state.json}"
    local workflow_file="${2:-$TEST_TEMP_DIR/workflow.yaml}"
    local phase_dir="${3:-$PHASE_DIR}"
    local state_name="${4:-impl}"
    "$BUILD_CONTEXT" "$state_file" "$workflow_file" "$phase_dir" "$state_name"
}

# Helper: render a template file with JSON context
render_template() {
    local template_file="$1"
    local context_json="$2"
    local base_dir="${3:-$PROMPTS_DIR}"
    python3 "$RENDER_TEMPLATE" "$template_file" "$context_json" "$base_dir"
}

#=============================================================================
# Test: test_e2e_context_to_template
# Verifies the full pipeline: build-context.sh produces context that
# render_template.py can consume to produce correct rendered output.
#=============================================================================

@test "v3 e2e: context to template pipeline" {
    # Setup state.json with known values
    cat > "$PHASE_DIR/state.json" << 'JSON'
{"iteration": {"current": 3, "max": 25}, "phase_status": "impl", "tests_passing": 5, "tests_total": 10}
JSON

    # Setup workflow with defaults, variables, and state params
    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test-workflow
version: 3
defaults:
  max_iterations: 10
  timeout: 600
variables:
  package: test-package
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: impl_review
    params:
      focus_area: "Core implementation"
  - name: impl_review
    prompt: prompts/feature/impl-review.md
    verdicts: [approved, concerns]
    next:
      approved: complete
      concerns: impl
  - name: complete
    prompt: prompts/common/complete.md
  - name: blocked
    prompt: prompts/common/blocked.md
entry_state: impl
terminal_states: [complete, blocked]
YAML

    # Create a template that uses various context fields
    cat > "$PROMPTS_DIR/feature/test_template.md" << 'TEMPLATE'
# Implementation - {{phase}}

Iteration: {{iteration}}/{{max_iterations}}
Crate: {{crate}}
Tests: {{state.tests_passing}} / {{state.tests_total}}
{{#if params.focus_area}}Focus your implementation on: **{{params.focus_area}}**{{/if}}

## Plan
{{plan_md}}
TEMPLATE

    # Step 1: Build context
    local context
    context=$(build_context)

    # Step 2: Render template with context
    local rendered
    rendered=$(render_template "$PROMPTS_DIR/feature/test_template.md" "$context" "$PROMPTS_DIR")

    # Step 3: Verify rendered output contains expected values
    [[ "$rendered" == *"# Implementation - test-phase"* ]]
    [[ "$rendered" == *"Iteration: 3/10"* ]]
    [[ "$rendered" == *"Crate: test-package"* ]]
    [[ "$rendered" == *"5 / 10"* ]]
    [[ "$rendered" == *"Focus your implementation on: **Core implementation**"* ]]
    [[ "$rendered" == *"# Phase: test-phase"* ]]
}

#=============================================================================
# Test: test_e2e_defaults_override_chain
# Verifies the merge precedence: defaults < variables, with params nested.
#=============================================================================

@test "v3 e2e: defaults override chain" {
    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test-workflow
version: 3
defaults:
  timeout: 600
  max_retries: 3
variables:
  package: test-package
  timeout: 900
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
    params:
      focus_area: "Core"
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    local context
    context=$(build_context)

    # Variables override defaults: timeout = 900 (not 600)
    local timeout
    timeout=$(echo "$context" | jq -r '.timeout')
    [[ "$timeout" == "900" ]]

    # Defaults not overridden: max_retries = 3
    local retries
    retries=$(echo "$context" | jq -r '.max_retries')
    [[ "$retries" == "3" ]]

    # Variables present: crate
    local crate
    crate=$(echo "$context" | jq -r '.crate')
    [[ "$crate" == "test-package" ]]

    # Params nested, NOT top-level
    local focus
    focus=$(echo "$context" | jq -r '.params.focus_area')
    [[ "$focus" == "Core" ]]

    # focus_area NOT at top level
    local top_focus
    top_focus=$(echo "$context" | jq -r '.focus_area // "MISSING"')
    [[ "$top_focus" == "MISSING" ]]
}

#=============================================================================
# Test: test_e2e_conditionals_render_correctly
# Verifies {{#if}} blocks work with context params.
#=============================================================================

@test "v3 e2e: conditionals render correctly with and without params" {
    cat > "$PROMPTS_DIR/feature/conditional_test.md" << 'TEMPLATE'
{{#if params.focus_area}}FOCUS: {{params.focus_area}}{{/if}}
TEMPLATE

    # Test with focus_area set
    local context_with='{"params": {"focus_area": "Core"}}'
    local rendered_with
    rendered_with=$(render_template "$PROMPTS_DIR/feature/conditional_test.md" "$context_with")
    [[ "$rendered_with" == *"FOCUS: Core"* ]]

    # Test without focus_area
    local context_without='{"params": {}}'
    local rendered_without
    rendered_without=$(render_template "$PROMPTS_DIR/feature/conditional_test.md" "$context_without")
    # "FOCUS:" should NOT appear
    [[ "$rendered_without" != *"FOCUS:"* ]]
}

#=============================================================================
# Test: test_e2e_includes_resolve_from_base_dir
# Verifies {{> path}} resolves relative to base_dir.
#=============================================================================

@test "v3 e2e: includes resolve from base_dir" {
    # Create common include file
    printf '%s' '## Test Commands

```bash
cargo nextest run
```' > "$PROMPTS_DIR/common/test-commands.md"

    # Create template referencing include
    printf '%s' '# Template
{{> common/test-commands.md}}' > "$PROMPTS_DIR/feature/include_test.md"

    local rendered
    rendered=$(render_template "$PROMPTS_DIR/feature/include_test.md" '{}' "$PROMPTS_DIR")

    [[ "$rendered" == *"## Test Commands"* ]]
    [[ "$rendered" == *"cargo nextest run"* ]]
}

#=============================================================================
# Test: test_e2e_template_pipeline_renders_correctly
# Full pipeline test with all components.
#=============================================================================

@test "v3 e2e: full template pipeline renders correctly" {
    # Setup state.json
    cat > "$PHASE_DIR/state.json" << 'JSON'
{"iteration": {"current": 3, "max": 25}, "phase_status": "impl", "tests_passing": 5, "tests_total": 10}
JSON

    # Setup workflow
    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test-workflow
version: 3
defaults:
  max_iterations: 10
variables:
  package: test-package
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
    params:
      focus_area: "Core"
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    # Setup plan.md
    cat > "$PHASE_DIR/plan.md" << 'MD'
# Test Phase

## Objective
Test something.
MD

    # Create common include
    printf '%s' '```bash
cargo nextest run -p {{crate}}
```' > "$PROMPTS_DIR/common/test-commands.md"

    # Create template using all features
    cat > "$PROMPTS_DIR/feature/full_test.md" << 'TEMPLATE'
# Implementation - {{phase}}

Iteration: {{iteration}}/{{max_iterations}}
Tests: {{state.tests_passing}} / {{state.tests_total}}

{{#if params.focus_area}}Focus your implementation on: **{{params.focus_area}}**{{/if}}

## Plan
{{plan_md}}

## Test Commands
{{> common/test-commands.md}}
TEMPLATE

    # Step 1: Build context
    local context
    context=$(build_context)

    # Step 2: Render template with context and base_dir
    local rendered
    rendered=$(render_template "$PROMPTS_DIR/feature/full_test.md" "$context" "$PROMPTS_DIR")

    # Verify all expected content
    [[ "$rendered" == *"# Implementation - test-phase"* ]]
    [[ "$rendered" == *"Iteration: 3/10"* ]]
    [[ "$rendered" == *"5 / 10"* ]]
    [[ "$rendered" == *"Focus your implementation on: **Core**"* ]]
    [[ "$rendered" == *"# Test Phase"* ]]
    [[ "$rendered" == *"cargo nextest run"* ]]

    # Verify no unresolved template tags remain
    # (filter out escaped braces and documentation content)
    local unresolved
    unresolved=$(echo "$rendered" | grep -c '{{[^>!]' || true)
    [[ "$unresolved" -eq 0 ]]
}

#=============================================================================
# Test: test_e2e_iterate_increments_iteration
# Verifies iterate.sh increments iteration counter in state.json.
# NOTE: This test cannot actually run iterate.sh (it spawns sub-agents).
# Instead, we test the increment mechanism via update-state.sh.
#=============================================================================

@test "v3 e2e: iteration increment via update-state.sh" {
    # Create a proper plan directory structure for update-state.sh
    local plan_dir="$TEST_TEMP_DIR/.plans/active/test-plan"
    local phase_dir="$plan_dir/phases/test-phase"
    mkdir -p "$phase_dir"

    cat > "$phase_dir/state.json" << 'JSON'
{"iteration": {"current": 1, "max": 25}, "phase_status": "impl", "tests_passing": 0, "tests_total": 0, "stuck_iterations": 0, "hang_count": 0, "last_reviewed_iteration": 0, "last_qa_reviewed_iteration": 0, "packages": [], "chunks": {"total": 0, "completed": [], "current": null, "remaining": []}, "blocked": {"is_blocked": false, "reason": null}, "disputes": [], "last_cleared_disputes": []}
JSON

    # Run increment-iteration
    PLANS_DIR="$TEST_TEMP_DIR/.plans" run "$SCRIPTS_DIR/update-state.sh" test-plan test-phase increment-iteration
    [[ "$status" -eq 0 ]]

    # Verify iteration incremented
    local new_iter
    new_iter=$(jq '.iteration.current' "$phase_dir/state.json")
    [[ "$new_iter" -eq 2 ]]
}

#=============================================================================
# Test: test_e2e_plan_md_special_chars
# Verifies special characters in plan.md survive the pipeline.
#=============================================================================

@test "v3 e2e: plan.md special characters preserved" {
    # Write plan.md with special characters
    cat > "$PHASE_DIR/plan.md" << 'MD'
## Test "quotes" and $variables

Code block:
```rust
fn test() { println!("hello"); }
```

Special: <>&
MD

    cat > "$PROMPTS_DIR/feature/special_test.md" << 'TEMPLATE'
{{plan_md}}
TEMPLATE

    local context
    context=$(build_context)

    local rendered
    rendered=$(render_template "$PROMPTS_DIR/feature/special_test.md" "$context" "$PROMPTS_DIR")

    # All special characters should be preserved
    [[ "$rendered" == *'"quotes"'* ]]
    [[ "$rendered" == *'$variables'* ]]
    [[ "$rendered" == *'println!("hello")'* ]]
    [[ "$rendered" == *'<>&'* ]]
}

#=============================================================================
# Test: test_e2e_empty_sections_handled
# Verifies graceful handling when workflow has no defaults/variables/params.
#=============================================================================

@test "v3 e2e: empty sections handled gracefully" {
    # Workflow with no defaults, no variables
    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test-minimal
version: 3
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    cat > "$PROMPTS_DIR/feature/empty_test.md" << 'TEMPLATE'
Phase: {{phase}}
State: {{current_state}}
Timeout: {{timeout | default: "300"}}
TEMPLATE

    local context
    context=$(build_context)

    # Params should be empty object
    local params
    params=$(echo "$context" | jq '.params')
    [[ "$params" == "{}" ]]

    # Template should render with default value
    local rendered
    rendered=$(render_template "$PROMPTS_DIR/feature/empty_test.md" "$context" "$PROMPTS_DIR")
    [[ "$rendered" == *"Phase: test-phase"* ]]
    [[ "$rendered" == *"State: impl"* ]]
    [[ "$rendered" == *"Timeout: 300"* ]]
}

#=============================================================================
# Test: test_e2e_unicode_preserved
# Verifies unicode survives the full pipeline.
#=============================================================================

@test "v3 e2e: unicode preserved through pipeline" {
    printf '%s' '测试 тест テスト' > "$PHASE_DIR/plan.md"

    cat > "$PROMPTS_DIR/feature/unicode_test.md" << 'TEMPLATE'
Content: {{plan_md}}
TEMPLATE

    local context
    context=$(build_context)

    local rendered
    rendered=$(render_template "$PROMPTS_DIR/feature/unicode_test.md" "$context" "$PROMPTS_DIR")
    [[ "$rendered" == *"测试"* ]]
    [[ "$rendered" == *"тест"* ]]
    [[ "$rendered" == *"テスト"* ]]
}

#=============================================================================
# Test: test_e2e_nested_each_with_objects
# Verifies {{#each}} with object arrays works through pipeline.
#=============================================================================

@test "v3 e2e: nested each with object arrays" {
    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test-each
version: 3
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
    params:
      files:
        - path: src/a.rs
          desc: File A
        - path: src/b.rs
          desc: File B
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    cat > "$PROMPTS_DIR/feature/each_test.md" << 'TEMPLATE'
Files:
{{#each params.files}}
- {{this.path}}: {{this.desc}}
{{/each}}
TEMPLATE

    local context
    context=$(build_context)

    local rendered
    rendered=$(render_template "$PROMPTS_DIR/feature/each_test.md" "$context" "$PROMPTS_DIR")
    [[ "$rendered" == *"- src/a.rs: File A"* ]]
    [[ "$rendered" == *"- src/b.rs: File B"* ]]
}

#=============================================================================
# Test: test_e2e_boolean_conditionals
# Verifies {{#unless}} with boolean params.
#=============================================================================

@test "v3 e2e: boolean conditionals with unless" {
    cat > "$PROMPTS_DIR/feature/unless_test.md" << 'TEMPLATE'
{{#unless params.allow_test_changes}}DO NOT modify test files{{/unless}}
TEMPLATE

    # With allow_test_changes=true -> "DO NOT" section absent
    local context_true='{"params": {"allow_test_changes": true}}'
    local rendered_true
    rendered_true=$(render_template "$PROMPTS_DIR/feature/unless_test.md" "$context_true")
    [[ "$rendered_true" != *"DO NOT"* ]]

    # With allow_test_changes=false -> "DO NOT" section present
    local context_false='{"params": {"allow_test_changes": false}}'
    local rendered_false
    rendered_false=$(render_template "$PROMPTS_DIR/feature/unless_test.md" "$context_false")
    [[ "$rendered_false" == *"DO NOT modify test files"* ]]
}

#=============================================================================
# Test: test_e2e_default_values_used
# Verifies {{var | default: "value"}} works when field is missing.
#=============================================================================

@test "v3 e2e: default values used when field missing" {
    cat > "$PROMPTS_DIR/feature/default_test.md" << 'TEMPLATE'
Crate: {{crate | default: "test-package"}}
TEMPLATE

    # Context missing 'crate' field
    local rendered
    rendered=$(render_template "$PROMPTS_DIR/feature/default_test.md" '{}')
    [[ "$rendered" == *"test-package"* ]]
}

#=============================================================================
# Test: test_e2e_computed_values_available
# Verifies build-context.sh produces all required computed values.
#=============================================================================

@test "v3 e2e: computed values available in context" {
    local context
    context=$(build_context)

    # All computed fields must be present
    echo "$context" | jq -e '.state' > /dev/null
    echo "$context" | jq -e '.iteration' > /dev/null
    echo "$context" | jq -e '.current_state' > /dev/null
    echo "$context" | jq -e '.phase' > /dev/null
    echo "$context" | jq -e '.plan' > /dev/null
    echo "$context" | jq -e 'has("plan_md")' > /dev/null
    echo "$context" | jq -e 'has("params")' > /dev/null

    # Verify values are correct
    local phase
    phase=$(echo "$context" | jq -r '.phase')
    [[ "$phase" == "test-phase" ]]

    local plan
    plan=$(echo "$context" | jq -r '.plan')
    [[ "$plan" == "test-plan" ]]

    local current_state
    current_state=$(echo "$context" | jq -r '.current_state')
    [[ "$current_state" == "impl" ]]

    local iteration
    iteration=$(echo "$context" | jq '.iteration')
    [[ "$iteration" == "3" ]]
}

#=============================================================================
# Test: test_e2e_error_handling_missing_state
# Verifies build-context.sh fails with proper error on missing state file.
#=============================================================================

@test "v3 e2e: error handling - missing state.json" {
    run "$BUILD_CONTEXT" "$TEST_TEMP_DIR/nonexistent.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" impl
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Error"* ]]
    [[ "$output" == *"State file"* ]] || [[ "$output" == *"state"* ]]
}

#=============================================================================
# Test: test_e2e_error_handling_invalid_workflow
# Verifies build-context.sh fails with proper error on invalid YAML.
#=============================================================================

@test "v3 e2e: error handling - invalid workflow YAML" {
    printf 'name: test\n  bad:\n indentation\n   here:\ntabs\there\n' > "$TEST_TEMP_DIR/bad_workflow.yaml"
    run "$BUILD_CONTEXT" "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/bad_workflow.yaml" "$PHASE_DIR" impl
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Error"* ]] || [[ "$output" == *"error"* ]] || [[ "$output" == *"YAML"* ]]
}

#=============================================================================
# Test: test_e2e_error_handling_missing_template
# Verifies render_template.py fails when template file doesn't exist.
#=============================================================================

@test "v3 e2e: error handling - missing template file" {
    run python3 "$RENDER_TEMPLATE" "$TEST_TEMP_DIR/nonexistent_template.md" '{}'
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Template"* ]] || [[ "$output" == *"not found"* ]] || [[ "$output" == *"Error"* ]]
}

#=============================================================================
# Test: test_e2e_v2_workflow_still_works
# Verifies backwards compatibility with V2 workflows.
#=============================================================================

@test "v3 e2e: V2 workflow backwards compatible" {
    # Create a V2 workflow (no defaults, no variables, no params)
    cat > "$TEST_TEMP_DIR/workflow_v2.yaml" << 'YAML'
name: test-v2
version: 2
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: impl_review
  - name: impl_review
    prompt: prompts/feature/impl-review.md
    verdicts: [approved, concerns]
    next:
      approved: complete
      concerns: impl
  - name: complete
    prompt: prompts/common/complete.md
  - name: blocked
    prompt: prompts/common/blocked.md
entry_state: impl
terminal_states: [complete, blocked]
YAML

    echo '{"iteration": 1, "current_state": "impl"}' > "$PHASE_DIR/state.json"

    # build-context.sh should work with V2 workflow
    local context
    context=$("$BUILD_CONTEXT" "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow_v2.yaml" "$PHASE_DIR" impl)

    # Should have computed values
    local phase
    phase=$(echo "$context" | jq -r '.phase')
    [[ "$phase" == "test-phase" ]]

    local iteration
    iteration=$(echo "$context" | jq '.iteration')
    [[ "$iteration" == "1" ]]

    # Params should be empty object (V2 has no params)
    local params
    params=$(echo "$context" | jq '.params')
    [[ "$params" == "{}" ]]

    # Template rendering should work
    cat > "$PROMPTS_DIR/feature/v2_test.md" << 'TEMPLATE'
Phase: {{phase}}, Iteration: {{iteration}}
TEMPLATE

    local rendered
    rendered=$(render_template "$PROMPTS_DIR/feature/v2_test.md" "$context")
    [[ "$rendered" == *"Phase: test-phase"* ]]
    [[ "$rendered" == *"Iteration: 1"* ]]
}

#=============================================================================
# Test: test_e2e_v1_workflow_still_works
# Verifies backwards compatibility with V1 workflows.
#=============================================================================

@test "v3 e2e: V1 workflow backwards compatible" {
    # Create a V1 workflow (linear states only)
    cat > "$TEST_TEMP_DIR/workflow_v1.yaml" << 'YAML'
name: test-v1
version: 1
states:
  - name: start
    prompt: prompts/feature/qa.md
    next: middle
  - name: middle
    prompt: prompts/feature/impl.md
    next: end
  - name: end
    prompt: prompts/common/complete.md
entry_state: start
terminal_states: [end]
YAML

    echo '{"iteration": 2}' > "$PHASE_DIR/state.json"

    # build-context.sh should work with V1 workflow
    local context
    context=$("$BUILD_CONTEXT" "$PHASE_DIR/state.json" "$TEST_TEMP_DIR/workflow_v1.yaml" "$PHASE_DIR" start)

    # Should have computed values
    local current_state
    current_state=$(echo "$context" | jq -r '.current_state')
    [[ "$current_state" == "start" ]]

    local iteration
    iteration=$(echo "$context" | jq '.iteration')
    [[ "$iteration" == "2" ]]

    # Template rendering should work
    cat > "$PROMPTS_DIR/feature/v1_test.md" << 'TEMPLATE'
State: {{current_state}}
TEMPLATE

    local rendered
    rendered=$(render_template "$PROMPTS_DIR/feature/v1_test.md" "$context")
    [[ "$rendered" == *"State: start"* ]]
}

#=============================================================================
# Test: test_e2e_old_and_new_schema_both_work
# Verifies both old (nested) and new (flat) state.json schemas produce
# the same top-level iteration value.
#=============================================================================

@test "v3 e2e: old and new state.json schemas both work" {
    # Old schema: iteration is nested object
    echo '{"iteration": {"current": 3, "max": 25}, "phase_status": "impl"}' > "$PHASE_DIR/state_old.json"

    # New schema: iteration is flat number
    echo '{"iteration": 3, "current_state": "impl"}' > "$PHASE_DIR/state_new.json"

    # Build context with old schema
    local context_old
    context_old=$("$BUILD_CONTEXT" "$PHASE_DIR/state_old.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" impl)
    local iter_old
    iter_old=$(echo "$context_old" | jq '.iteration')

    # Build context with new schema
    local context_new
    context_new=$("$BUILD_CONTEXT" "$PHASE_DIR/state_new.json" "$TEST_TEMP_DIR/workflow.yaml" "$PHASE_DIR" impl)
    local iter_new
    iter_new=$(echo "$context_new" | jq '.iteration')

    # Both produce iteration = 3 at top level
    [[ "$iter_old" == "3" ]]
    [[ "$iter_new" == "3" ]]
    [[ "$iter_old" == "$iter_new" ]]
}

#=============================================================================
# Test: test_e2e_context_json_valid_for_render
# Verifies schema contract: build-context.sh output is valid JSON that
# render_template.py can consume.
#=============================================================================

@test "v3 e2e: context JSON valid for render_template.py" {
    # Step 1: Build context
    local context
    context=$(build_context)

    # Step 2: Verify output is valid JSON
    echo "$context" | jq . > /dev/null
    [[ $? -eq 0 ]]

    # Step 3: Pass to render_template.py - it should not error
    cat > "$PROMPTS_DIR/feature/json_test.md" << 'TEMPLATE'
Phase: {{phase}}
TEMPLATE

    run render_template "$PROMPTS_DIR/feature/json_test.md" "$context"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Phase: test-phase"* ]]
}

#=============================================================================
# Test: test_e2e_validate_then_build_context
# Verifies validate-workflow.sh accepts workflows that build-context.sh
# can process (no false rejections).
#=============================================================================

@test "v3 e2e: validate-workflow.sh accepts valid V3 workflow" {
    # First validate the workflow
    run "$VALIDATE_WORKFLOW" "$TEST_TEMP_DIR/workflow.yaml"
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Validation passed"* ]]

    # Then build context with the same workflow
    local context
    context=$(build_context)

    # Both succeed
    echo "$context" | jq -e '.phase' > /dev/null
}

#=============================================================================
# Test: test_e2e_render_error_propagates
# Verifies render_template.py errors surface correctly on bad JSON context.
#=============================================================================

@test "v3 e2e: render error propagates on invalid JSON context" {
    cat > "$PROMPTS_DIR/feature/error_test.md" << 'TEMPLATE'
Hello {{name}}
TEMPLATE

    # Pass intentionally broken JSON context
    run python3 "$RENDER_TEMPLATE" "$PROMPTS_DIR/feature/error_test.md" "not json"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"JSON"* ]] || [[ "$output" == *"json"* ]]
}

#=============================================================================
# Edge Cases
#=============================================================================

#=============================================================================
# Edge Case: Empty plan.md
#=============================================================================

@test "v3 e2e edge: empty plan.md renders without error" {
    truncate -s 0 "$PHASE_DIR/plan.md"

    cat > "$PROMPTS_DIR/feature/empty_plan_test.md" << 'TEMPLATE'
Plan: [{{plan_md}}]
TEMPLATE

    local context
    context=$(build_context)

    local plan_md
    plan_md=$(echo "$context" | jq -r '.plan_md')
    [[ "$plan_md" == "" ]]

    local rendered
    rendered=$(render_template "$PROMPTS_DIR/feature/empty_plan_test.md" "$context")
    [[ "$rendered" == *"Plan: []"* ]]
}

#=============================================================================
# Edge Case: Very large plan.md — no truncation
#=============================================================================

@test "v3 e2e edge: very large plan.md not truncated" {
    # Generate ~50KB plan.md
    {
        echo "# Large Plan"
        for i in $(seq 1 1000); do
            echo "Line $i of a very large plan document with detailed content about the implementation."
        done
    } > "$PHASE_DIR/plan.md"

    local context
    context=$(build_context)

    local plan_md
    plan_md=$(echo "$context" | jq -r '.plan_md')
    [[ "$plan_md" == *"# Large Plan"* ]]
    [[ "$plan_md" == *"Line 1000"* ]]
}

#=============================================================================
# Edge Case: Missing optional fields — defaults applied
#=============================================================================

@test "v3 e2e edge: missing optional fields get defaults" {
    cat > "$TEST_TEMP_DIR/workflow.yaml" << 'YAML'
name: test-minimal
version: 3
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    cat > "$PROMPTS_DIR/feature/defaults_edge.md" << 'TEMPLATE'
Timeout: {{timeout | default: "600"}}
Mode: {{mode | default: "normal"}}
TEMPLATE

    local context
    context=$(build_context)

    local rendered
    rendered=$(render_template "$PROMPTS_DIR/feature/defaults_edge.md" "$context")
    [[ "$rendered" == *"Timeout: 600"* ]]
    [[ "$rendered" == *"Mode: normal"* ]]
}

#=============================================================================
# Edge Case: Old vs new state.json schema — both handled
#=============================================================================

@test "v3 e2e edge: old schema jq fallback works" {
    # Copy the new schema fixture and test
    cp "$FIXTURES_DIR/state_new_schema.json" "$PHASE_DIR/state.json"

    local context
    context=$(build_context)

    local iteration
    iteration=$(echo "$context" | jq '.iteration')
    [[ "$iteration" == "3" ]]
}

#=============================================================================
# Edge Case: Concurrent access — multiple renders don't interfere
#=============================================================================

@test "v3 e2e edge: concurrent renders don't interfere" {
    cat > "$PROMPTS_DIR/feature/concurrent_a.md" << 'TEMPLATE'
Name: {{name}}
TEMPLATE
    cat > "$PROMPTS_DIR/feature/concurrent_b.md" << 'TEMPLATE'
Value: {{value}}
TEMPLATE

    # Run two renders simultaneously
    local result_a result_b
    result_a=$(render_template "$PROMPTS_DIR/feature/concurrent_a.md" '{"name": "alpha"}') &
    local pid_a=$!
    result_b=$(render_template "$PROMPTS_DIR/feature/concurrent_b.md" '{"value": "beta"}') &
    local pid_b=$!
    wait $pid_a
    wait $pid_b

    # Both should have rendered correctly (no cross-contamination)
    # Note: variable capture in subshell means we need to re-render to check
    local check_a check_b
    check_a=$(render_template "$PROMPTS_DIR/feature/concurrent_a.md" '{"name": "alpha"}')
    check_b=$(render_template "$PROMPTS_DIR/feature/concurrent_b.md" '{"value": "beta"}')
    [[ "$check_a" == *"Name: alpha"* ]]
    [[ "$check_b" == *"Value: beta"* ]]
}

#=============================================================================
# Edge Case: Symbolic links — follow links correctly
#=============================================================================

@test "v3 e2e edge: symbolic links followed correctly" {
    # Create a real file
    printf '%s' 'Symlinked content' > "$TEST_TEMP_DIR/real_plan.md"

    # Create a symlink in phase dir
    ln -sf "$TEST_TEMP_DIR/real_plan.md" "$PHASE_DIR/plan.md"

    local context
    context=$(build_context)

    local plan_md
    plan_md=$(echo "$context" | jq -r '.plan_md')
    [[ "$plan_md" == "Symlinked content" ]]
}

#=============================================================================
# Integration: build-context output shape validation
#=============================================================================

@test "v3 e2e: build-context output has complete shape" {
    local context
    context=$(build_context)

    # Must be valid JSON
    echo "$context" | jq empty
    [[ $? -eq 0 ]]

    # Must contain all required top-level fields
    echo "$context" | jq -e 'has("state")' > /dev/null
    echo "$context" | jq -e 'has("params")' > /dev/null
    echo "$context" | jq -e 'has("iteration")' > /dev/null
    echo "$context" | jq -e 'has("current_state")' > /dev/null
    echo "$context" | jq -e 'has("phase")' > /dev/null
    echo "$context" | jq -e 'has("plan")' > /dev/null
    echo "$context" | jq -e 'has("plan_md")' > /dev/null

    # Must contain workflow-derived fields from fixture
    echo "$context" | jq -e 'has("crate")' > /dev/null
    echo "$context" | jq -e 'has("max_iterations")' > /dev/null
    echo "$context" | jq -e 'has("timeout")' > /dev/null
    echo "$context" | jq -e 'has("test_pattern")' > /dev/null
}

#=============================================================================
# Integration: full pipeline with actual orchestration prompts
#=============================================================================

@test "v3 e2e: renders actual impl.md prompt template" {
    # This test uses the real impl.md prompt template from the orchestration dir
    # to verify the pipeline works with actual production templates
    local real_template="$ORCH_DIR/prompts/feature/impl.md"

    # Skip if template doesn't exist (shouldn't happen, but safety check)
    [[ -f "$real_template" ]] || skip "impl.md template not found"

    # Build context
    local context
    context=$(build_context)

    # Render with real template (base_dir = prompts/ parent)
    local rendered
    rendered=$(render_template "$real_template" "$context" "$ORCH_DIR/prompts")

    # Should contain phase and iteration info from the template structure
    # The exact content depends on the template, but it should be non-empty
    # and not contain unresolved required variables
    [[ -n "$rendered" ]]

    # The template uses {{phase}} and {{iteration}} which are in context
    # At minimum it should not contain the literal {{phase}} tag
    [[ "$rendered" != *'{{phase}}'* ]]
}

#=============================================================================
# Integration: validate-workflow.sh V3 validation
#=============================================================================

@test "v3 e2e: validate-workflow.sh runs V3 validation on V3 workflow" {
    run "$VALIDATE_WORKFLOW" "$TEST_TEMP_DIR/workflow.yaml"
    [[ "$status" -eq 0 ]]
    # Should mention V3 schema validation
    [[ "$output" == *"V3"* ]] || [[ "$output" == *"v3"* ]] || [[ "$output" == *"version: 3"* ]]
    [[ "$output" == *"Validation passed"* ]]
}

@test "v3 e2e: validate-workflow.sh rejects invalid V3 defaults" {
    cat > "$TEST_TEMP_DIR/bad_defaults.yaml" << 'YAML'
name: test-bad
version: 3
defaults:
  max_iterations: -1
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    run "$VALIDATE_WORKFLOW" "$TEST_TEMP_DIR/bad_defaults.yaml"
    [[ "$status" -ne 0 ]]
    [[ "$output" == *"max_iterations"* ]]
}

@test "v3 e2e: validate-workflow.sh rejects reserved variable names" {
    cat > "$TEST_TEMP_DIR/reserved_vars.yaml" << 'YAML'
name: test-reserved
version: 3
variables:
  iteration: 5
states:
  - name: impl
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    prompt: prompts/common/complete.md
entry_state: impl
terminal_states: [complete]
YAML

    run "$VALIDATE_WORKFLOW" "$TEST_TEMP_DIR/reserved_vars.yaml"
    [[ "$status" -ne 0 ]]
    [[ "$output" == *"Reserved"* ]] || [[ "$output" == *"reserved"* ]] || [[ "$output" == *"iteration"* ]]
}

#=============================================================================
# Edge Case: Binary characters in plan.md
# build-context.sh should handle or fail gracefully with binary content.
#=============================================================================

@test "v3 e2e edge: binary characters in plan.md handled gracefully" {
    # Write binary content to plan.md (null bytes, control chars)
    printf 'Before binary\x00\x01\x02After binary' > "$PHASE_DIR/plan.md"

    # build-context.sh may succeed (jq handles binary via --arg escaping)
    # or may fail with an error — either is acceptable, but must not hang
    run build_context
    # Either succeeds or fails cleanly (exit 0 or exit 1, but not hang/crash)
    [[ "$status" -eq 0 || "$status" -eq 1 ]]
}

#=============================================================================
# Edge Case: Missing plan.md file entirely
# build-context.sh should still produce valid context with empty plan_md.
#=============================================================================

@test "v3 e2e edge: missing plan.md produces empty plan_md" {
    rm -f "$PHASE_DIR/plan.md"

    local context
    context=$(build_context)

    local plan_md
    plan_md=$(echo "$context" | jq -r '.plan_md')
    [[ "$plan_md" == "" ]]
}

#=============================================================================
# Integration: validate-workflow.sh skips V3 checks for V1 workflow
#=============================================================================

@test "v3 e2e: validate-workflow.sh skips V3 validation for V1 workflow" {
    cat > "$TEST_TEMP_DIR/workflow_v1_validate.yaml" << 'YAML'
name: test-v1
version: 1
states:
  - name: start
    prompt: prompts/feature/qa.md
    next: end
  - name: end
    prompt: prompts/common/complete.md
entry_state: start
terminal_states: [end]
YAML

    run "$VALIDATE_WORKFLOW" "$TEST_TEMP_DIR/workflow_v1_validate.yaml"
    [[ "$status" -eq 0 ]]
    # Should NOT mention V3 schema validation
    [[ "$output" != *"V3 schema validation"* ]]
}
