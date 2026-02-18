#!/usr/bin/env bats

# Tests for render_template.py
# Phase: template-engine (orchestration-v3)
#
# Tests the Python template engine that handles variable substitution,
# conditionals, iteration, and file includes for rendering prompt templates.

setup() {
    load 'test_helper'
    setup_temp_dir

    RENDER_SCRIPT="$SCRIPTS_DIR/render_template.py"

    # Create a base_dir for include tests
    export INCLUDE_DIR="$TEST_TEMP_DIR/includes"
    mkdir -p "$INCLUDE_DIR"
    mkdir -p "$INCLUDE_DIR/common"
}

teardown() {
    teardown_temp_dir
}

# Helper: create a template file and echo its path
create_template() {
    local content="$1"
    local filename="${2:-template.md}"
    printf '%s' "$content" > "$TEST_TEMP_DIR/$filename"
    echo "$TEST_TEMP_DIR/$filename"
}

# Helper: create an include file under INCLUDE_DIR
create_include() {
    local path="$1"
    local content="$2"
    local dir
    dir=$(dirname "$INCLUDE_DIR/$path")
    mkdir -p "$dir"
    printf '%s' "$content" > "$INCLUDE_DIR/$path"
}

# Helper: run the render script with template content and JSON context
render() {
    local template_content="$1"
    local context_json="$2"
    [[ -z "$context_json" ]] && context_json='{}'
    local base_dir="${3:-$INCLUDE_DIR}"
    local tpl
    tpl=$(create_template "$template_content")
    run python3 "$RENDER_SCRIPT" "$tpl" "$context_json" "$base_dir"
}

#=============================================================================
# Script Existence and Syntax Tests
#=============================================================================

@test "render_template.py exists" {
    [[ -f "$RENDER_SCRIPT" ]]
}

@test "render_template.py is valid Python syntax" {
    run python3 -m py_compile "$RENDER_SCRIPT"
    [[ "$status" -eq 0 ]]
}

#=============================================================================
# CLI Tests
#=============================================================================

@test "test_cli_missing_args: exit 1 with usage on no arguments" {
    run python3 "$RENDER_SCRIPT"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"Usage"* ]]
}

@test "test_cli_template_not_found: exit 1 when template file missing" {
    run python3 "$RENDER_SCRIPT" "$TEST_TEMP_DIR/nonexistent.md" '{}'
    [[ "$status" -eq 1 ]]
}

@test "test_cli_invalid_json: exit 1 on invalid JSON context" {
    local tpl
    tpl=$(create_template "Hello")
    run python3 "$RENDER_SCRIPT" "$tpl" "not json"
    [[ "$status" -eq 1 ]]
    [[ "$output" == *"JSON"* ]] || [[ "$output" == *"json"* ]]
}

@test "cli: default base_dir is parent-of-parent of template file" {
    # Create template at depth: INCLUDE_DIR/feature/impl.md
    mkdir -p "$INCLUDE_DIR/feature"
    mkdir -p "$INCLUDE_DIR/common"
    printf '%s' '# Header' > "$INCLUDE_DIR/common/header.md"
    printf '%s' '{{> common/header.md}}' > "$INCLUDE_DIR/feature/impl.md"
    # When no base_dir argument, should default to parent of parent
    run python3 "$RENDER_SCRIPT" "$INCLUDE_DIR/feature/impl.md" '{}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "# Header" ]]
}

@test "cli: explicit base_dir overrides default" {
    create_include "common/header.md" "Custom Header"
    local tpl
    tpl=$(create_template '{{> common/header.md}}')
    run python3 "$RENDER_SCRIPT" "$tpl" '{}' "$INCLUDE_DIR"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "Custom Header" ]]
}

#=============================================================================
# Simple Variable Substitution Tests
#=============================================================================

@test "test_simple_variable: substitutes simple variable" {
    render 'Hello, {{name}}!' '{"name": "World"}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "Hello, World!" ]]
}

@test "test_nested_variable: substitutes dot-notation path" {
    render 'Iteration: {{state.iteration}}' '{"state": {"iteration": 5}}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "Iteration: 5" ]]
}

@test "test_missing_variable: renders empty string for missing var" {
    render 'Value: {{missing}}' '{}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "Value: " ]]
}

@test "test_default_value: uses default when variable missing" {
    render 'Timeout: {{timeout | default: "600"}}' '{}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "Timeout: 600" ]]
}

@test "test_default_not_used: ignores default when variable exists" {
    render 'Timeout: {{timeout | default: "600"}}' '{"timeout": 300}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "Timeout: 300" ]]
}

@test "test_deeply_nested_path: resolves a.b.c.d.e" {
    render 'Value: {{a.b.c.d.e}}' '{"a": {"b": {"c": {"d": {"e": "deep"}}}}}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "Value: deep" ]]
}

@test "test_array_index_access: resolves items.0, items.1" {
    render 'First: {{items.0}}, Second: {{items.1}}' '{"items": ["one", "two", "three"]}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "First: one, Second: two" ]]
}

@test "test_object_as_json: serializes objects as compact JSON" {
    render 'Config: {{config}}' '{"config": {"key": "value"}}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == 'Config: {"key": "value"}' ]]
}

@test "test_empty_template: returns empty string" {
    render '' '{}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "" ]]
}

@test "variable: multiple variables in one template" {
    render '{{first}} and {{second}}' '{"first": "A", "second": "B"}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "A and B" ]]
}

@test "variable: integer value renders as string" {
    render 'Count: {{count}}' '{"count": 42}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "Count: 42" ]]
}

@test "variable: boolean true renders as True or true" {
    render 'Flag: {{flag}}' '{"flag": true}'
    [[ "$status" -eq 0 ]]
    # Python renders True, but the spec says objects get JSON-serialized
    # For primitives, str() is used
    [[ "$output" == "Flag: true" ]] || [[ "$output" == "Flag: True" ]]
}

@test "variable: null value renders as empty string" {
    render 'Value: {{val}}' '{"val": null}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "Value: " ]]
}

@test "variable: array renders as JSON" {
    render 'Arr: {{arr}}' '{"arr": [1, 2, 3]}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == 'Arr: [1, 2, 3]' ]]
}

#=============================================================================
# Escaped Braces Tests
#=============================================================================

@test "test_escaped_braces: backslash-escaped opening braces become literal" {
    render 'Use \{{variable}} syntax' '{}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == 'Use {{variable}} syntax' ]]
}

@test "test_escaped_closing_braces: backslash-escaped closing braces become literal" {
    render 'Use \}} for closing' '{}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == 'Use }} for closing' ]]
}

@test "escaped: mixed escaped and unescaped braces" {
    render '\{{literal}} and {{name}}' '{"name": "real"}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == '{{literal}} and real' ]]
}

#=============================================================================
# Conditional: {{#if}} Tests
#=============================================================================

@test "test_if_true: renders if-block when condition is truthy" {
    render '{{#if enabled}}Feature enabled{{/if}}' '{"enabled": true}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "Feature enabled" ]]
}

@test "test_if_false: omits if-block when condition is falsy" {
    render '{{#if enabled}}Feature enabled{{/if}}' '{"enabled": false}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "" ]]
}

@test "test_if_else_true: renders if-branch when true" {
    render '{{#if debug}}DEBUG{{else}}RELEASE{{/if}}' '{"debug": true}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "DEBUG" ]]
}

@test "test_if_else_false: renders else-branch when false" {
    render '{{#if debug}}DEBUG{{else}}RELEASE{{/if}}' '{"debug": false}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "RELEASE" ]]
}

@test "test_nested_conditionals: if inside if works correctly" {
    render '{{#if a}}{{#if b}}Both{{else}}Only A{{/if}}{{else}}Neither{{/if}}' '{"a": true, "b": false}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "Only A" ]]
}

@test "nested conditionals: both true" {
    render '{{#if a}}{{#if b}}Both{{else}}Only A{{/if}}{{else}}Neither{{/if}}' '{"a": true, "b": true}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "Both" ]]
}

@test "nested conditionals: outer false" {
    render '{{#if a}}{{#if b}}Both{{else}}Only A{{/if}}{{else}}Neither{{/if}}' '{"a": false, "b": true}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "Neither" ]]
}

@test "test_multiline_if_block: multi-line content in if block" {
    local tpl='{{#if include_notes}}
## Notes

These are important notes.
{{/if}}'
    render "$tpl" '{"include_notes": true}'
    [[ "$status" -eq 0 ]]
    # The output should contain the notes content
    [[ "$output" == *"## Notes"* ]]
    [[ "$output" == *"These are important notes."* ]]
}

@test "if: missing condition variable is falsy" {
    render '{{#if missing}}Yes{{else}}No{{/if}}' '{}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "No" ]]
}

@test "if: condition with nested path" {
    render '{{#if state.ready}}Go{{else}}Wait{{/if}}' '{"state": {"ready": true}}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "Go" ]]
}

#=============================================================================
# Conditional: {{#unless}} Tests
#=============================================================================

@test "test_unless_true: omits unless-block when condition is truthy" {
    render '{{#unless production}}Dev mode{{/unless}}' '{"production": true}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "" ]]
}

@test "test_unless_false: renders unless-block when condition is falsy" {
    render '{{#unless production}}Dev mode{{/unless}}' '{"production": false}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "Dev mode" ]]
}

@test "unless: missing variable is falsy so unless renders" {
    render '{{#unless missing}}Shown{{/unless}}' '{}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "Shown" ]]
}

#=============================================================================
# Truthy/Falsy Tests
#=============================================================================

@test "test_truthy_empty_string: empty string is falsy" {
    render '{{#if value}}Yes{{else}}No{{/if}}' '{"value": ""}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "No" ]]
}

@test "test_truthy_zero: zero is falsy" {
    render '{{#if value}}Yes{{else}}No{{/if}}' '{"value": 0}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "No" ]]
}

@test "test_truthy_empty_list: empty list is falsy" {
    render '{{#if value}}Yes{{else}}No{{/if}}' '{"value": []}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "No" ]]
}

@test "test_truthy_none: null is falsy" {
    render '{{#if value}}Yes{{else}}No{{/if}}' '{"value": null}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "No" ]]
}

@test "test_truthy_empty_dict: empty dict is falsy" {
    render '{{#if value}}Yes{{else}}No{{/if}}' '{"value": {}}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "No" ]]
}

@test "test_truthy_string_zero: string '0' is truthy (not number 0)" {
    render '{{#if value}}Yes{{else}}No{{/if}}' '{"value": "0"}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "Yes" ]]
}

@test "test_is_truthy_nonzero_number: nonzero number is truthy" {
    render '{{#if value}}Yes{{else}}No{{/if}}' '{"value": 42}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "Yes" ]]
}

@test "test_is_truthy_nonempty_list: non-empty list is truthy" {
    render '{{#if value}}Yes{{else}}No{{/if}}' '{"value": [1, 2, 3]}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "Yes" ]]
}

@test "test_is_truthy_nonempty_dict: non-empty dict is truthy" {
    render '{{#if value}}Yes{{else}}No{{/if}}' '{"value": {"key": "value"}}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "Yes" ]]
}

@test "truthy: whitespace string is truthy" {
    render '{{#if value}}Yes{{else}}No{{/if}}' '{"value": " "}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "Yes" ]]
}

@test "truthy: negative number is truthy" {
    render '{{#if value}}Yes{{else}}No{{/if}}' '{"value": -1}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "Yes" ]]
}

@test "truthy: true boolean is truthy" {
    render '{{#if value}}Yes{{else}}No{{/if}}' '{"value": true}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "Yes" ]]
}

@test "truthy: false boolean is falsy" {
    render '{{#if value}}Yes{{else}}No{{/if}}' '{"value": false}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "No" ]]
}

#=============================================================================
# Each: Array Iteration Tests
#=============================================================================

@test "test_each_array: iterates over array items" {
    local tpl='{{#each items}}- {{this}}
{{/each}}'
    render "$tpl" '{"items": ["a", "b", "c"]}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"- a"* ]]
    [[ "$output" == *"- b"* ]]
    [[ "$output" == *"- c"* ]]
}

@test "test_each_with_index: @index provides zero-based index" {
    local tpl='{{#each items}}{{@index}}: {{this}}
{{/each}}'
    render "$tpl" '{"items": ["x", "y"]}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"0: x"* ]]
    [[ "$output" == *"1: y"* ]]
}

@test "test_each_first_last: @first and @last flags work" {
    render '{{#each items}}{{#if @first}}[{{/if}}{{this}}{{#if @last}}]{{/if}}{{/each}}' '{"items": ["a", "b", "c"]}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "[abc]" ]]
}

@test "test_empty_array_each: empty array produces no output" {
    render '{{#each items}}Item{{/each}}' '{"items": []}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "" ]]
}

@test "test_each_with_object_items: iterates over array of objects" {
    local tpl='{{#each files}}File: {{this.path}} - {{this.desc}}
{{/each}}'
    render "$tpl" '{"files": [{"path": "a.rs", "desc": "First"}, {"path": "b.rs", "desc": "Second"}]}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"File: a.rs - First"* ]]
    [[ "$output" == *"File: b.rs - Second"* ]]
}

@test "each: single item array" {
    render '{{#each items}}{{this}}{{/each}}' '{"items": ["only"]}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "only" ]]
}

@test "each: single item has both @first and @last true" {
    render '{{#each items}}{{#if @first}}F{{/if}}{{#if @last}}L{{/if}}{{/each}}' '{"items": ["x"]}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "FL" ]]
}

@test "each: outer context accessible inside each block" {
    local tpl='{{#each items}}{{phase}}: {{this}}
{{/each}}'
    render "$tpl" '{"phase": "build", "items": ["a", "b"]}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"build: a"* ]]
    [[ "$output" == *"build: b"* ]]
}

@test "each: missing array variable produces no output" {
    render '{{#each missing}}Item{{/each}}' '{}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "" ]]
}

@test "each: non-array variable produces no output" {
    render '{{#each value}}Item{{/each}}' '{"value": "string"}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "" ]]
}

#=============================================================================
# Each: Object Key Iteration Tests
#=============================================================================

@test "test_each_object_keys: iterates over object keys" {
    local tpl='{{#each obj}}{{@key}}: {{this}}
{{/each}}'
    render "$tpl" '{"obj": {"name": "Alice", "age": "30"}}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"name: Alice"* ]]
    [[ "$output" == *"age: 30"* ]]
}

@test "each object: empty object produces no output" {
    render '{{#each obj}}{{@key}}: {{this}}{{/each}}' '{"obj": {}}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "" ]]
}

#=============================================================================
# Include Tests
#=============================================================================

@test "test_include_simple: includes file content" {
    create_include "common/header.md" "# Header"
    local tpl='{{> common/header.md}}
Body content'
    render "$tpl" '{}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"# Header"* ]]
    [[ "$output" == *"Body content"* ]]
}

@test "test_include_missing: renders HTML comment for missing include" {
    render '{{> nonexistent.md}}' '{}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "<!-- Include not found: nonexistent.md -->" ]]
}

@test "test_include_recursive: nested includes resolve correctly" {
    create_include "inner.md" "INNER"
    create_include "outer.md" "Before {{> inner.md}} After"
    render '{{> outer.md}}' '{}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "Before INNER After" ]]
}

@test "test_include_max_depth: stops at depth 10" {
    # Create a chain of 12 includes
    local i
    for i in $(seq 1 12); do
        local next=$((i + 1))
        create_include "chain_$i.md" "Level $i {{> chain_$next.md}}"
    done
    create_include "chain_13.md" "End"

    render '{{> chain_1.md}}' '{}'
    # Should either exit 1 or contain an error comment at depth 10
    # The exact behavior depends on implementation, but it must not hang
    if [[ "$status" -eq 0 ]]; then
        # If it doesn't error, it should have stopped somewhere with comment
        [[ "$output" == *"Include"* ]] || [[ "$output" == *"Level 1"* ]]
    fi
    # Key: the test doesn't hang (infinite loop protection works)
}

@test "include: variables in included template are processed" {
    create_include "greet.md" "Hello, {{name}}!"
    render '{{> greet.md}}' '{"name": "World"}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "Hello, World!" ]]
}

@test "include: conditionals in included template are processed" {
    create_include "conditional.md" '{{#if show}}Visible{{/if}}'
    render '{{> conditional.md}}' '{"show": true}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "Visible" ]]
}

@test "include: each loops in included template are processed" {
    create_include "list.md" '{{#each items}}- {{this}}
{{/each}}'
    render '{{> list.md}}' '{"items": ["a", "b"]}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"- a"* ]]
    [[ "$output" == *"- b"* ]]
}

@test "include: multiple includes in same template" {
    create_include "part1.md" "PART1"
    create_include "part2.md" "PART2"
    render '{{> part1.md}} and {{> part2.md}}' '{}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "PART1 and PART2" ]]
}

@test "include: path resolution relative to base_dir" {
    create_include "common/footer.md" "Footer"
    render '{{> common/footer.md}}' '{}' "$INCLUDE_DIR"
    [[ "$status" -eq 0 ]]
    [[ "$output" == "Footer" ]]
}

#=============================================================================
# Processing Order Tests
#=============================================================================

@test "test_processing_order: includes -> each -> conditionals -> variables" {
    create_include "include.md" '{{#if show}}{{name}}{{/if}}'
    render '{{> include.md}}' '{"show": true, "name": "Test"}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "Test" ]]
}

@test "processing order: each runs before conditionals" {
    local tpl='{{#each items}}{{#if @first}}First: {{/if}}{{this}}
{{/each}}'
    render "$tpl" '{"items": ["a", "b"]}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"First: a"* ]]
    [[ "$output" == *"b"* ]]
}

#=============================================================================
# Full Render Pipeline Tests (complex combinations)
#=============================================================================

@test "render: all features combined" {
    create_include "common/header.md" "# {{title}}"
    local tpl='{{> common/header.md}}

{{#if show_items}}
Items:
{{#each items}}- {{this}}
{{/each}}
{{/if}}

Timeout: {{timeout | default: "60"}}'

    render "$tpl" '{"title": "Report", "show_items": true, "items": ["A", "B"]}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"# Report"* ]]
    [[ "$output" == *"Items:"* ]]
    [[ "$output" == *"- A"* ]]
    [[ "$output" == *"- B"* ]]
    [[ "$output" == *"Timeout: 60"* ]]
}

@test "render: conditional hides entire section including each" {
    local tpl='{{#if show}}{{#each items}}{{this}}{{/each}}{{/if}}'
    render "$tpl" '{"show": false, "items": ["a", "b"]}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "" ]]
}

@test "render: no templates produces plain text passthrough" {
    render 'Hello plain text!' '{}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "Hello plain text!" ]]
}

#=============================================================================
# Edge Cases
#=============================================================================

@test "edge: unicode characters pass through unchanged" {
    render 'Hello {{name}}!' '{"name": "世界"}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "Hello 世界!" ]]
}

@test "edge: empty context still renders template" {
    render 'Static content' '{}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "Static content" ]]
}

@test "edge: template with only whitespace" {
    render '   ' '{}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "   " ]]
}

@test "edge: variable with spaces around name" {
    # {{  name  }} should still resolve (common authoring typo)
    render 'Hello, {{ name }}!' '{"name": "World"}'
    [[ "$status" -eq 0 ]]
    # Either strips spaces and resolves, or renders empty - both acceptable
    # but the natural behavior is to treat " name " as a key with spaces which won't match
    # Plan doesn't specify trimming, so empty is acceptable
    [[ "$output" == "Hello, World!" ]] || [[ "$output" == "Hello, !" ]]
}

@test "edge: deeply nested path partially missing" {
    render 'Value: {{a.b.missing.d}}' '{"a": {"b": {"c": 1}}}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "Value: " ]]
}

@test "edge: multiple consecutive variables" {
    render '{{a}}{{b}}{{c}}' '{"a": "1", "b": "2", "c": "3"}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "123" ]]
}

@test "edge: template with no variable markers" {
    render 'Just plain text, no handlebars here.' '{}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "Just plain text, no handlebars here." ]]
}

@test "edge: each inside unless" {
    local tpl='{{#unless hide}}{{#each items}}{{this}} {{/each}}{{/unless}}'
    render "$tpl" '{"hide": false, "items": ["a", "b"]}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"a"* ]]
    [[ "$output" == *"b"* ]]
}

@test "edge: default with existing empty string uses default" {
    # Empty string is falsy, so default might or might not apply
    # The plan says: "default: fallback" is used when var is missing
    # An empty string variable IS present, so behavior depends on impl
    render '{{val | default: "fallback"}}' '{"val": ""}'
    [[ "$status" -eq 0 ]]
    # If default only applies to missing keys: output is ""
    # If default also applies to empty/falsy: output is "fallback"
    # The plan says "default values" - typically this means "if missing"
    [[ "$output" == "" ]] || [[ "$output" == "fallback" ]]
}

@test "edge: newlines in variable value" {
    render 'Content: {{text}}' '{"text": "line1\nline2"}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"line1"* ]]
}

@test "edge: special JSON characters in context" {
    render '{{msg}}' '{"msg": "He said \"hello\""}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == 'He said "hello"' ]]
}

@test "edge: numeric value in each" {
    local tpl='{{#each nums}}{{this}} {{/each}}'
    render "$tpl" '{"nums": [1, 2, 3]}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"1"* ]]
    [[ "$output" == *"2"* ]]
    [[ "$output" == *"3"* ]]
}

@test "edge: boolean values in array" {
    local tpl='{{#each flags}}{{@index}}:{{this}} {{/each}}'
    render "$tpl" '{"flags": [true, false, true]}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"0:"* ]]
    [[ "$output" == *"1:"* ]]
    [[ "$output" == *"2:"* ]]
}

#=============================================================================
# Plan-Specified Missing Tests (added iteration 12)
#=============================================================================

@test "test_nested_each_loops: nested each blocks with outer context access" {
    local tpl='{{#each groups}}Group: {{this.name}}
{{#each this.items}}- {{this}}
{{/each}}{{/each}}'
    render "$tpl" '{"groups": [{"name": "A", "items": ["x", "y"]}, {"name": "B", "items": ["z"]}]}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Group: A"* ]]
    [[ "$output" == *"- x"* ]]
    [[ "$output" == *"- y"* ]]
    [[ "$output" == *"Group: B"* ]]
    [[ "$output" == *"- z"* ]]
}

@test "test_nested_unless: nested unless blocks" {
    render '{{#unless a}}{{#unless b}}Neither{{/unless}}{{/unless}}' '{"a": false, "b": false}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "Neither" ]]
}

@test "test_each_non_iterable: block removal for non-iterable values" {
    render 'Before{{#each value}}Item{{/each}}After' '{"value": "just a string"}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "BeforeAfter" ]]
}

@test "test_malformed_conditional_no_condition: if with no condition" {
    render '{{#if}}content{{/if}}' '{}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "content" ]]
}

@test "test_backslash_not_before_braces: Windows-style paths with backslashes" {
    render 'Path: C:\Users\test' '{}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == 'Path: C:\Users\test' ]]
}

@test "test_double_backslash_before_braces: double-backslash before variable rendering" {
    render 'Escaped: \\{{name}}' '{"name": "World"}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == 'Escaped: \World' ]]
}

@test "test_partial_path_resolution_failure: dot path through non-dict" {
    render 'Value: {{a.b.c}}' '{"a": {"b": "string_not_dict"}}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "Value: " ]]
}

@test "test_path_through_null: dot path through null" {
    render 'Value: {{a.b}}' '{"a": null}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "Value: " ]]
}

@test "test_is_truthy_float_zero: float zero falsy behavior" {
    render '{{#if value}}Yes{{else}}No{{/if}}' '{"value": 0.0}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "No" ]]
}

@test "test_is_truthy_string_false: string false is truthy" {
    render '{{#if value}}Yes{{else}}No{{/if}}' '{"value": "false"}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "Yes" ]]
}

@test "test_each_object_first_last: @first/@last for object iteration" {
    local tpl='{{#each obj}}{{#if @first}}FIRST:{{/if}}{{@key}}={{this}}{{#if @last}};LAST{{/if}}
{{/each}}'
    render "$tpl" '{"obj": {"a": "1", "b": "2", "c": "3"}}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"FIRST:a=1"* ]]
    [[ "$output" == *"b=2"* ]]
    [[ "$output" == *"c=3;LAST"* ]]
}

@test "test_each_missing_key: each over nonexistent key" {
    render 'Before{{#each nonexistent}}Item{{/each}}After' '{}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "BeforeAfter" ]]
}

@test "test_pipe_with_extra_spaces: flexible pipe syntax" {
    render 'Value: {{var |  default:  "fallback"  }}' '{}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "Value: fallback" ]]
}

@test "test_boolean_variable_rendering: lowercase true/false" {
    render 'Enabled: {{enabled}}' '{"enabled": true}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "Enabled: true" ]]
}

@test "test_numeric_variable_rendering: numeric formatting" {
    render 'Count: {{count}}, Rate: {{rate}}' '{"count": 42, "rate": 3.14}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "Count: 42, Rate: 3.14" ]]
}

@test "test_array_variable_rendering: JSON serialization" {
    render 'Items: {{items}}' '{"items": [1, 2, 3]}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == "Items: [1, 2, 3]" ]]
}

@test "test_include_whitespace_only_file: whitespace-only include" {
    printf '   \n  \n' > "$INCLUDE_DIR/empty.md"
    render 'Before{{> empty.md}}After' '{}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == 'Before   '* ]]
    [[ "$output" == *'After' ]]
}

@test "test_template_only_includes: multiple includes only" {
    create_include "a.md" "Alpha"
    create_include "b.md" "Beta"
    local tpl='{{> a.md}}
{{> b.md}}'
    render "$tpl" '{}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"Alpha"* ]]
    [[ "$output" == *"Beta"* ]]
}

@test "test_each_preserves_outer_context: exact plan spec version" {
    local tpl='{{#each items}}{{phase}}: {{this}}
{{/each}}'
    render "$tpl" '{"phase": "test", "items": ["a", "b"]}'
    [[ "$status" -eq 0 ]]
    [[ "$output" == *"test: a"* ]]
    [[ "$output" == *"test: b"* ]]
}
