#!/usr/bin/env python3
"""
Template engine for rendering orchestration prompt templates.

Supports:
- Variable substitution: {{variable}}, {{nested.path}}
- Default values: {{var | default: "fallback"}}
- Conditionals: {{#if}}, {{#unless}}, {{else}}, {{/if}}, {{/unless}}
- Iteration: {{#each array}}...{{/each}} with {{this}}, {{@index}}, {{@key}}, {{@first}}, {{@last}}
- File includes: {{> path/to/include.md}}
- Escaped braces: \\{{literal}}

Usage: render_template.py <template_file> <context_json> [base_dir]
"""

import json
import os
import re
import sys

# Sentinel values for escape handling
_BSLASH_SENTINEL = "\x00BSLASH\x00"
_OPEN_SENTINEL = "\x00OPEN\x00"
_CLOSE_SENTINEL = "\x00CLOSE\x00"


def get_value(context, key):
    """
    Get nested value from context dict using dot notation.

    Args:
        context: The context dictionary
        key: Dot-separated key path (e.g., "state.iteration")

    Returns:
        The value at the key path, or empty string if not found
    """
    parts = key.split(".")
    current = context
    for part in parts:
        if isinstance(current, dict):
            if part in current:
                current = current[part]
            else:
                return ""
        elif isinstance(current, list):
            try:
                idx = int(part)
                current = current[idx]
            except (ValueError, IndexError):
                return ""
        else:
            return ""
    return current


def is_truthy(value):
    """
    Check if value is truthy for conditionals.

    Returns:
        False for: None, False, 0, 0.0, "", [], {}
        True for: everything else
    """
    if value is None:
        return False
    if value is False:
        return False
    if value == 0 and not isinstance(value, bool):
        return False
    if value == "":
        return False
    if isinstance(value, (list, dict)) and len(value) == 0:
        return False
    return True


def _value_to_str(value):
    """Convert a value to string for template output."""
    if value is None:
        return ""
    if isinstance(value, bool):
        return "true" if value else "false"
    if isinstance(value, (dict, list)):
        return json.dumps(value, ensure_ascii=False)
    return str(value)


def _find_matching_end(template, tag_type, start_pos):
    """
    Find the matching closing tag for a block directive, handling nesting.

    Args:
        template: The template string
        tag_type: "if", "unless", or "each"
        start_pos: Position after the opening tag

    Returns:
        Position of the start of the closing tag, or -1 if not found
    """
    depth = 1
    pos = start_pos
    open_pattern = re.compile(r'\{\{#' + tag_type + r'[\s}]')
    close_tag = "{{/" + tag_type + "}}"

    while depth > 0 and pos < len(template):
        next_open = open_pattern.search(template, pos)
        next_close_pos = template.find(close_tag, pos)

        if next_close_pos == -1:
            return -1

        if next_open and next_open.start() < next_close_pos:
            depth += 1
            pos = next_open.end()
        else:
            depth -= 1
            if depth == 0:
                return next_close_pos
            pos = next_close_pos + len(close_tag)

    return -1


def process_conditionals(template, context):
    """
    Process {{#if}}, {{#unless}}, {{else}}, {{/if}} blocks.
    """
    # Process {{#if condition}}...{{/if}} with optional {{else}}
    template = _process_if_blocks(template, context)
    # Process {{#unless condition}}...{{/unless}}
    template = _process_unless_blocks(template, context)
    return template


def _process_if_blocks(template, context):
    """Process all {{#if condition}}...{{/if}} blocks, handling nesting."""
    pattern = re.compile(r'\{\{#if\s+(.+?)\}\}', re.DOTALL)

    while True:
        match = pattern.search(template)
        if not match:
            break

        condition_key = match.group(1).strip()
        block_start = match.end()

        # Find matching {{/if}}
        end_pos = _find_matching_end(template, "if", block_start)
        if end_pos == -1:
            break

        block_content = template[block_start:end_pos]
        close_tag_end = end_pos + len("{{/if}}")

        # Find {{else}} at the same nesting level
        else_pos = _find_else(block_content, "if")

        condition_value = get_value(context, condition_key)

        if is_truthy(condition_value):
            if else_pos != -1:
                replacement = block_content[:else_pos]
            else:
                replacement = block_content
        else:
            if else_pos != -1:
                replacement = block_content[else_pos + len("{{else}}"):]
            else:
                replacement = ""

        template = template[:match.start()] + replacement + template[close_tag_end:]

    return template


def _process_unless_blocks(template, context):
    """Process all {{#unless condition}}...{{/unless}} blocks."""
    pattern = re.compile(r'\{\{#unless\s+(.+?)\}\}', re.DOTALL)

    while True:
        match = pattern.search(template)
        if not match:
            break

        condition_key = match.group(1).strip()
        block_start = match.end()

        end_pos = _find_matching_end(template, "unless", block_start)
        if end_pos == -1:
            break

        block_content = template[block_start:end_pos]
        close_tag_end = end_pos + len("{{/unless}}")

        condition_value = get_value(context, condition_key)

        if not is_truthy(condition_value):
            replacement = block_content
        else:
            replacement = ""

        template = template[:match.start()] + replacement + template[close_tag_end:]

    return template


def _find_else(block_content, tag_type):
    """
    Find {{else}} at the top level of block_content (not inside nested #if/#unless blocks).
    """
    depth = 0
    pos = 0
    open_pattern = re.compile(r'\{\{#if[\s}]')

    while pos < len(block_content):
        # Check for nested {{#if}} opening
        open_match = open_pattern.match(block_content, pos)
        if open_match:
            depth += 1
            pos = open_match.end()
            continue

        # Check for {{/if}} closing
        if block_content[pos:].startswith("{{/if}}"):
            depth -= 1
            pos += len("{{/if}}")
            continue

        # Check for {{else}} at depth 0
        if depth == 0 and block_content[pos:].startswith("{{else}}"):
            return pos

        pos += 1

    return -1


def process_each(template, context, base_dir=""):
    """
    Process {{#each array}}...{{/each}} blocks.
    """
    pattern = re.compile(r'\{\{#each\s+(.+?)\}\}', re.DOTALL)

    while True:
        match = pattern.search(template)
        if not match:
            break

        array_key = match.group(1).strip()
        block_start = match.end()

        end_pos = _find_matching_end(template, "each", block_start)
        if end_pos == -1:
            break

        block_body = template[block_start:end_pos]
        close_tag_end = end_pos + len("{{/each}}")

        iterable = get_value(context, array_key)

        parts = []
        if isinstance(iterable, list):
            for i, item in enumerate(iterable):
                iter_context = dict(context)
                iter_context["this"] = item
                iter_context["@index"] = i
                iter_context["@first"] = (i == 0)
                iter_context["@last"] = (i == len(iterable) - 1)
                rendered = render(block_body, iter_context, base_dir)
                parts.append(rendered)
        elif isinstance(iterable, dict):
            keys = list(iterable.keys())
            for i, key in enumerate(keys):
                iter_context = dict(context)
                iter_context["this"] = iterable[key]
                iter_context["@key"] = key
                iter_context["@first"] = (i == 0)
                iter_context["@last"] = (i == len(keys) - 1)
                rendered = render(block_body, iter_context, base_dir)
                parts.append(rendered)
        # For non-iterable values (string, number, null, bool): block is removed

        replacement = "".join(parts)
        template = template[:match.start()] + replacement + template[close_tag_end:]

    return template


def process_includes(template, context, base_dir, depth=0):
    """
    Process {{> path/to/include}} directives.
    """
    if depth > 10:
        return template

    pattern = re.compile(r'\{\{>\s*(.+?)\s*\}\}')

    while True:
        match = pattern.search(template)
        if not match:
            break

        include_path = match.group(1).strip()
        full_path = os.path.join(base_dir, include_path)

        if os.path.isfile(full_path):
            with open(full_path, "r", encoding="utf-8") as f:
                include_content = f.read()
            # Recursively process includes in the included content
            include_content = process_includes(include_content, context, base_dir, depth + 1)
        else:
            include_content = "<!-- Include not found: {} -->".format(include_path)

        template = template[:match.start()] + include_content + template[match.end():]

    return template


def process_variables(template, context):
    """
    Process {{variable}} substitutions.
    """
    # First: handle default values: {{var | default: "fallback"}}
    default_pattern = re.compile(r'\{\{\s*(.+?)\s*\|\s*default:\s*"([^"]*)"\s*\}\}')

    def replace_default(m):
        var_name = m.group(1).strip()
        default_val = m.group(2)
        value = get_value(context, var_name)
        if value == "" and var_name not in context:
            # Check nested path: only use default if key is truly missing
            return default_val
        if value == "":
            # Key exists but value resolves to empty string
            # Check if the key actually exists at the top level or nested
            parts = var_name.split(".")
            current = context
            found = True
            for part in parts:
                if isinstance(current, dict) and part in current:
                    current = current[part]
                elif isinstance(current, list):
                    try:
                        current = current[int(part)]
                    except (ValueError, IndexError):
                        found = False
                        break
                else:
                    found = False
                    break
            if not found:
                return default_val
            return _value_to_str(value)
        return _value_to_str(value)

    template = default_pattern.sub(replace_default, template)

    # Then: handle simple variables: {{variable}}
    var_pattern = re.compile(r'\{\{\s*([a-zA-Z_@][a-zA-Z0-9_.@]*)\s*\}\}')

    def replace_var(m):
        var_name = m.group(1).strip()
        value = get_value(context, var_name)
        return _value_to_str(value)

    template = var_pattern.sub(replace_var, template)

    # Final pass: remove any remaining unmatched {{...}} patterns
    # (e.g., {{#if}} with no condition, {{/if}}, or invalid variable names)
    template = re.sub(r'\{\{.*?\}\}', '', template)

    return template


def render(template, context, base_dir=""):
    """
    Full template rendering pipeline.

    Order of processing:
        1. process_includes (first, so included content is processed)
        2. process_each (before conditionals, so @index etc work in conditionals)
        3. process_conditionals (before variables, so conditional blocks are resolved)
        4. process_variables (last, substitutes remaining placeholders)
    """
    # Escape handling: protect escaped sequences before processing
    # Step 1: Replace \\ with sentinel
    template = template.replace("\\\\", _BSLASH_SENTINEL)
    # Step 2: Replace \{{ with sentinel
    template = template.replace("\\{{", _OPEN_SENTINEL)
    # Step 3: Replace \}} with sentinel
    template = template.replace("\\}}", _CLOSE_SENTINEL)

    # Processing pipeline
    template = process_includes(template, context, base_dir)
    template = process_each(template, context, base_dir)
    template = process_conditionals(template, context)
    template = process_variables(template, context)

    # Restore sentinels
    template = template.replace(_BSLASH_SENTINEL, "\\")
    template = template.replace(_OPEN_SENTINEL, "{{")
    template = template.replace(_CLOSE_SENTINEL, "}}")

    return template


def main():
    if len(sys.argv) < 3:
        print("Usage: render_template.py <template_file> <context_json> [base_dir]", file=sys.stderr)
        sys.exit(1)

    template_file = sys.argv[1]
    context_json = sys.argv[2]

    if not os.path.isfile(template_file):
        print("Error: Template file not found: {}".format(template_file), file=sys.stderr)
        sys.exit(1)

    try:
        context = json.loads(context_json)
    except json.JSONDecodeError as e:
        print("Error: Invalid JSON context: {}".format(e), file=sys.stderr)
        sys.exit(1)

    # Determine base_dir
    if len(sys.argv) >= 4:
        base_dir = sys.argv[3]
    else:
        # Default to parent of parent of template file
        template_dir = os.path.dirname(os.path.abspath(template_file))
        base_dir = os.path.dirname(template_dir)

    with open(template_file, "r", encoding="utf-8") as f:
        template = f.read()

    result = render(template, context, base_dir)

    # Output without trailing newline (print adds one, so use sys.stdout.write)
    sys.stdout.write(result)


if __name__ == "__main__":
    main()
