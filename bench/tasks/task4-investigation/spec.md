# Task 4: Investigate and Optimize Slow List Performance

## Problem

Users report that `tkit list` and `tkit stats` are noticeably slow when the task file contains more than a few hundred tasks. With 5,000 tasks, operations take several seconds when they should be nearly instant.

## Investigation Required

The root cause is not provided. You need to:

1. **Analyze the codebase** — Read through the store, filter, and CLI packages to understand the data flow
2. **Identify bottlenecks** — Find why operations are slow with large task counts
3. **Write benchmarks** — Create Go benchmark tests that demonstrate the problem
4. **Implement fixes** — Optimize the identified bottlenecks
5. **Verify improvement** — Show the benchmarks improve significantly

## Hints

- Consider how many times the task file is read from disk during a single CLI command
- Look at the `stats` command and how it gathers data
- Look at the filter and sort implementations
- Think about algorithmic complexity

## Requirements

- Write Go benchmark tests (`Benchmark*` functions) that measure list/stats performance with 5,000 tasks
- Achieve at least a 5x speedup on the benchmarks
- All existing tests must continue to pass
- The optimization should be in the data access layer, not the CLI layer
- Do NOT change the public API of any package
- Document what you found and what you changed (in code comments is fine)

## Seed Data

A helper to generate test fixtures:

```go
func generateTasks(n int) []model.Task {
    tasks := make([]model.Task, n)
    for i := range tasks {
        tasks[i] = model.Task{
            ID:       i + 1,
            Title:    fmt.Sprintf("Task %d", i+1),
            Status:   model.StatusPending,
            Priority: model.Priority((i % 3) + 1),
            CreatedAt: time.Now(),
        }
    }
    return tasks
}
```
