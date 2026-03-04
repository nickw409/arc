# Runner

Executes language-specific test runner scripts and aggregates results.

## File Map

| File | Purpose |
|------|---------|
| `runner.go` | `Run()` — locates and invokes `{ArcHome}/runners/{runner}/run.sh`, JSON-decodes stdout into `TestResult`. `RunAll()` — fans out concurrent `Run` calls over multiple test files and merges results. |
| `result.go` | `TestResult` struct: `Total`, `Passed`, `Failed`, `RawOutput`, `FailedNames`. |

## Key Details

- Runner scripts live on disk at `{ArcHome}/runners/{runner}/run.sh` — they are NOT embedded resources.
- Supported runners: `go-test`, `cargo-nextest`, `vitest`, `pytest`, `cargo-test`.
- `RunAll` uses `sync.WaitGroup` for concurrent execution, sums counts and concatenates raw output.
