# Regression Test Writer

Plan: {{plan}}
Phase: {{phase}}
Plan doc: {{plan_file}}
Investigation: {{phase_dir}}/investigation.md

## Your Task

Write regression tests that:
1. Fail with the current buggy code
2. Pass when the bug is fixed
3. Prevent the bug from recurring

## Steps

1. **Read the investigation** to understand the root cause
2. **Write a minimal test** that reproduces the bug
3. **Write edge case tests** for related scenarios
4. **Verify tests fail** with current code

## Test Requirements

- Tests MUST fail before the fix
- Tests MUST be minimal - test the specific bug, not everything
- Tests MUST have clear names: `test_<bug>_<scenario>`
- Tests MUST use assertions that will pass after fix

### Wiring Tests

If the bug involves a function not being called, an argument not being passed through, or a return value being dropped, write a test that exercises the full call path — not just the inner function. Call the real entry point and verify the fix is reachable end-to-end.

### Negative Tests

Beyond reproducing the bug, write negative tests for the surrounding code:

1. **Inputs adjacent to the bug** — if the bug involves a specific value, test nearby values (off-by-one, empty, nil, max)
2. **Related error paths** — if the bug is in error handling, verify other error paths in the same function aren't similarly broken
3. **Malformed variants** — if the bug involves parsing or data handling, test with corrupt, truncated, or unexpected input

Name these: `test_<bug>_negative_<scenario>`

## Test File Location

Use descriptive names and register test files in state.json.
Place test files according to your project's test conventions.

After creating test files:
```bash
arc update-state.sh {{plan}} {{phase}} add-test-file "<path_to_test>"
```

## Example

```rust
#[test]
fn test_division_by_zero_returns_error() {
    // This test currently fails because the bug causes a panic
    // After fix, it should return Err(DivisionByZero)
    let result = calculate(10, 0);
    assert!(result.is_err());
}
```

## Rules
- Do NOT implement the fix
- Tests should FAIL with current code
- Keep tests focused on the bug
- **Do NOT commit** - orchestrator handles commits

When done, exit.
