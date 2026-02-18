## Test Writing Guidelines (Rust / cargo-nextest)

### File Location
- Unit tests: Inside the source file in a `#[cfg(test)] mod tests { ... }` block
- Integration tests: In `<crate>/tests/<name>.rs`
- The phase's `state.json` must have test files registered in `test_files[]`

### Test Structure
```rust
#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_descriptive_name() {
        // Arrange
        let input = ...;

        // Act
        let result = function_under_test(input);

        // Assert
        assert_eq!(result, expected);
    }

    #[test]
    #[should_panic(expected = "error message")]
    fn test_error_case() {
        function_that_panics();
    }
}
```

### Assertions
- Use `assert_eq!(actual, expected)` for equality
- Use `assert!(condition)` for boolean checks
- Use `assert!(result.is_ok())` and `assert!(result.is_err())` for Result types
- Use `#[should_panic]` for expected panics
- For floating point: `assert!((actual - expected).abs() < epsilon)`

### Running Tests
```bash
arc iterate <plan> <phase> qa        # Write tests
arc iterate <plan> <phase> impl      # Implement to pass tests
```

### Common Patterns
- Use `#[ignore]` for slow tests that need `--run-ignored all`
- Use `proptest!` or `quickcheck!` for property-based testing
- Mock external dependencies with trait objects
