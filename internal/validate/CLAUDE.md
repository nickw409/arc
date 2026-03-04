# Validate

AI-powered test quality auditing. Scans source/test files, batches them, and fans out parallel Claude agents to find test quality issues.

## File Map

| File | Purpose |
|------|---------|
| `validate.go` | `Run()` — entry point. Dispatches to `runParallel` (default) or `runLegacy` (custom prompt). `ParseReport()` extracts findings from `## Verdict` / `### Critical` / `### Warning` sections. |
| `scan.go` | `Scan()` — walks directories, groups files by package into `Batch` objects. `splitBatch` recursively halves oversized batches (source files duplicated, test files divided). |
| `parallel.go` | `RunParallel()` — fans out one agent per batch with bounded concurrency. Agents get all file content inline (no tools). `MergeReports()` combines findings. |

## Key Details

- Two modes: **parallel** (default, no tools, content inline, `MaxTurns: 2`) and **legacy** (single agent with `View`/`Grep`/`Glob` tools, `MaxTurns: 30`).
- Verdict is `"pass"` or `"fail"` (fail if any critical findings).
- Supported languages for file detection: Go, Python, TypeScript.
- Max batch size: 3000 lines (configurable). Oversized batches split recursively.
