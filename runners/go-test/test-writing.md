## Test Writing Guidelines (Go / go test)

### File Location
- Same package: `mypackage/handler_test.go` next to `handler.go`
- The phase's `state.json` must have test files registered in `test_files[]`

### Test Structure
```go
package mypackage

import (
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestMyFunction(t *testing.T) {
    result := MyFunction("input")
    assert.Equal(t, "expected", result)
}

func TestMyFunction_ErrorCase(t *testing.T) {
    _, err := MyFunction("")
    require.Error(t, err)
    assert.Contains(t, err.Error(), "invalid")
}

func TestMyFunction_TableDriven(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
    }{
        {"basic", "hello", "HELLO"},
        {"empty", "", ""},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := MyFunction(tt.input)
            assert.Equal(t, tt.expected, result)
        })
    }
}
```

### Assertions (testify)
- `assert.Equal(t, expected, actual)` — equality
- `assert.Nil(t, value)` / `assert.NotNil(t, value)`
- `assert.Error(t, err)` / `assert.NoError(t, err)`
- `require.*` — same as assert but fails immediately

### Running Tests
```bash
arc iterate <plan> <phase> qa        # Write tests
arc iterate <plan> <phase> impl      # Implement to pass tests
```
