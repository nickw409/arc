# Characterization Tests

Plan: {{plan}}
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

Place test files according to your project's test conventions.

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

When done, write:

## Memory
[What interfaces were characterized, test files created, edge cases covered. Future runs of this state will see this.]
