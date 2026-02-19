# Characterization Tests

Plan: {{plan_name}}
Phase: {{phase}}
Plan doc: {{plan_file}}

## Your Task

Write characterization tests that capture the CURRENT behavior of the code being refactored. These tests ensure the refactoring doesn't change behavior.

## Steps

1. **Read the plan** to understand what will be refactored
2. **Identify all public interfaces** of the code
3. **Write tests for each interface** capturing current behavior
4. **Include edge cases** that might be affected by refactoring

## Test Requirements

- Tests MUST pass with CURRENT code (before refactoring)
- Tests MUST cover all public interfaces
- Tests MUST capture actual behavior, not expected behavior
- Tests MUST include edge cases

## Test File Location

Use descriptive names and register test files in state.json.
Place test files according to your project's test conventions.

After creating test files:
```bash
arc update-state.sh {{plan_name}} {{phase}} add-test-file "<path_to_test>"
```

## Example

```rust
// Characterization test - captures current behavior
#[test]
fn test_parser_handles_empty_input() {
    // Current behavior: returns empty vec (may or may not be ideal)
    let result = Parser::parse("");
    assert_eq!(result, vec![]);
}

#[test]
fn test_parser_whitespace_handling() {
    // Current behavior: trims leading whitespace
    let result = Parser::parse("  hello");
    assert_eq!(result[0], "hello");
}
```

## Characterization Document

Create `{{phase_dir}}/characterization.md`:

```markdown
# Characterization: {{phase}}

## Scope
<what code is being characterized>

## Interfaces Tested
- `function_name()` - <behavior captured>

## Edge Cases
- Empty input: <current behavior>
- Null/None: <current behavior>
- Large input: <current behavior>

## Behaviors That May Be Incorrect
<document any behaviors that seem wrong but are being preserved>
```

## Rules
- Test CURRENT behavior, not DESIRED behavior
- Do NOT fix bugs you find - just document them
- **Do NOT commit** - orchestrator handles commits

When done, exit.
