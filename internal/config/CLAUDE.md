# Config

Loads, validates, and saves the project-level `.arc.yaml` configuration file. Single file.

## File Map

| File | Purpose |
|------|---------|
| `config.go` | `Config` struct with `Language`, `Runner`, `BuildCommand`, `TestCommand`, `Git` (commit style, signing), `Audit` (custom prompt path). `Load`/`Save`/`Validate`. |

## Key Details

- Supported languages: go, rust, typescript, python, unknown.
- Supported runners: go-test, cargo-nextest, vitest, pytest, cargo-test.
- Commit styles: conventional (`type(scope): desc`) or freeform.
- `Validate` checks language and runner against supported lists.
