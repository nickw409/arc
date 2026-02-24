# Arc Benchmark Suite

Benchmarking framework to compare **Arc** (multi-phase AI orchestration) against **Claude Code** (single agent) on complex coding tasks.

## Purpose

Determine whether Arc's phased orchestration produces better results than a single Claude Code agent. Three approaches are compared:

- **arc** — Full Arc orchestration with phased execution
- **claude-minimal** — Claude Code with only the task spec (no plan)
- **claude-plan** — Claude Code with task spec + a detailed implementation plan

If Arc beats both, orchestration matters. If claude-plan matches Arc, the value is just in planning. If claude-minimal wins, Arc is overhead.

## Project Structure

```
testbed/          tkit — a Go CLI task tracker used as the target codebase
  cmd/tkit/       Entry point
  internal/
    cli/          Cobra commands (add, list, complete, delete, show, stats)
    model/        Task struct, status/priority types
    store/        JSON file-backed store (deliberately monolithic, has bugs)
    filter/       Filtering and sorting (deliberately O(n²) sorts)
tasks/            Benchmark task definitions (4 tasks)
  task1-feature/  Add label/tag support across 4+ packages
  task2-bugfix/   Fix off-by-one bug that corrupts adjacent task priority
  task3-refactor/ Extract storage backend interface from monolith
  task4-investigation/  Profile and optimize slow list performance
harness/          Benchmark orchestration scripts
  bench.sh        Main entrypoint
  run-approach.sh Executes one approach against one task
  setup-workdir.sh Creates fresh working copy per run
  collect.sh      Gathers metrics (hidden tests, coverage, lint, tokens, diff)
  report.sh       Aggregates results into markdown comparison table
  config.sh       Shared configuration
results/          Output reports (gitignored)
.work/            Temporary working directories per run (gitignored)
```

## Test Bed: tkit

A simple task tracker CLI written in Go. Has 22 passing tests. Contains **deliberate flaws** that each benchmark task targets:

| Flaw | Location | Benchmark Task |
|------|----------|----------------|
| No label/tag support | model, store, filter, cli | Task 1 (feature) |
| `Complete()` zeroes next task's priority (off-by-one) | `store/store.go:99` | Task 2 (bugfix) |
| Store is monolithic — CRUD + file I/O in one file | `store/store.go` | Task 3 (refactor) |
| File re-read on every method call + O(n²) bubble sorts | `store/store.go`, `filter/filter.go` | Task 4 (investigation) |

## Each Benchmark Task Has

- `spec.md` — Task specification given to all three approaches
- `plan.md` — Detailed implementation plan given only to claude-plan (human-readable outline)
- `arc-phases/` — Per-phase plan files in Arc's required format (used only by the arc approach)
- `hidden_test.go` — Validation tests the agent never sees, run after completion to score correctness
- Task 2 also has `failing_test.go` that gets copied into the workdir as the starting clue

### Arc Phase Plans

Each task has an `arc-phases/` directory with subdirectories per phase. Each phase has a `plan.md` following Arc's required format (Objective, Files, Types and Signatures, DO NOT, Test Cases with concrete values, Edge Cases, Integration Points). The harness copies these into Arc's `.plans/active/` structure at runtime.

| Task | Workflow | Phases |
|------|----------|--------|
| task1-feature | feature | model → store → filter → cli |
| task2-bugfix | bugfix | investigate → fix |
| task3-refactor | refactor | characterize → refactor |
| task4-investigation | performance | baseline → optimize |

## Running Benchmarks

```bash
# See what would run
bash harness/bench.sh --dry-run

# Single task, single approach, 1 run (good for testing)
bash harness/bench.sh --task task2-bugfix --approach claude-minimal --runs 1

# Full suite: 4 tasks × 3 approaches × 3 runs = 36 runs
bash harness/bench.sh

# Just collect metrics from existing runs (skip re-running)
bash harness/bench.sh --collect-only

# Just regenerate report from collected results
bash harness/bench.sh --report-only
```

## Metrics Collected Per Run

| Metric | Source |
|--------|--------|
| Wall-clock time (ms) | Harness timing |
| Hidden test pass/total | Hidden test suite run post-completion |
| Existing test pass/total | `go test ./...` |
| Test coverage % | `go test -coverprofile` |
| go vet issues | `go vet ./...` |
| Lines added/removed | `git diff` |
| Files changed | `git diff` |
| Input/output tokens | Claude JSON output parsing |
| Estimated cost (USD) | Derived from token counts |

## Known Issues / TODO

- Token parsing from Claude output depends on `--output-format json` format which may need adjustment based on actual Claude Code output structure.
- Arc token/cost tracking is not yet implemented — arc runs sub-agents internally and doesn't expose aggregate token counts in stdout.

## Key Commands

```bash
# Run tkit tests directly
cd testbed && go test ./...

# Build tkit
cd testbed && go build -o tkit ./cmd/tkit/

# Verify the planted bug (should FAIL)
cp tasks/task2-bugfix/failing_test.go testbed/internal/store/
cd testbed && go test ./internal/store/ -run TestCompleteDoesNotCorruptAdjacentTask -v
# Clean up: rm testbed/internal/store/failing_test.go
```
