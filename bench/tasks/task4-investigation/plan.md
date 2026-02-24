# Task 4: Implementation Plan — Investigate and Optimize List Performance

## Phase 1: Research
1. Read all source files: `store/store.go`, `filter/filter.go`, `cli/list.go`, `cli/stats.go`
2. Map out the call graph for `tkit list` and `tkit stats`
3. Count how many times `load()` (file read + JSON parse) is called per command
4. Note algorithmic complexity of filter and sort operations

## Phase 2: Benchmark
1. Create `internal/store/bench_test.go` with:
   - `BenchmarkStoreList` — measure List() with 5000 tasks
   - `BenchmarkStoreCount` — measure Count() + CountByStatus() calls
   - `BenchmarkStoreStatsPattern` — simulate what the `stats` command does (Count + 3x CountByStatus)
2. Create `internal/filter/bench_test.go` with:
   - `BenchmarkFilterApply` — measure filtering 5000 tasks
   - `BenchmarkSortByPriority` — measure sorting 5000 tasks
3. Run benchmarks, record baseline numbers

## Phase 3: Identify Issues
Expected findings:
- **Issue 1:** Store reads the file on EVERY method call. `stats` calls Count() + CountByStatus() x3 = 4 file reads for one command
- **Issue 2:** `nextID()` in Add does an extra file read before the Add itself reads again
- **Issue 3:** Filter sort uses O(n^2) bubble sort instead of O(n log n)
- **Issue 4:** Multiple separate passes in filter.Apply when one pass would suffice

## Phase 4: Optimize
1. **Store caching:** Add a read cache to Store that caches the last load result. Invalidate on save. This eliminates redundant file reads within a single command.
   - Add `cached []model.Task` and `cacheValid bool` fields to Store
   - `load()` returns cache if valid, otherwise reads file and caches
   - `save()` invalidates cache (or updates it)
2. **Fix nextID:** Have Add() use the loaded tasks to compute next ID instead of calling nextID() separately
3. **Fix sort:** Replace bubble sort with `sort.Slice` (uses introsort, O(n log n))
4. **Optimize filter:** Combine filter passes into a single iteration

## Phase 5: Verify
1. Run benchmarks again, compare with baseline
2. Verify at least 5x improvement
3. Run all existing tests — no regressions
