# Task 3: Implementation Plan — Extract Storage Backend Interface

## Phase 1: Characterize Existing Behavior
1. Read `store.go` and identify all calls to `load()` and `save()`
2. Run existing tests, note the count and all pass
3. Understand the current `New(path string)` constructor contract

## Phase 2: Define Backend Interface
1. Create `internal/store/backend.go`
2. Define the `Backend` interface with `Load()` and `Save()` methods
3. Implement `JSONBackend` by extracting the current `load()`/`save()` logic:
   - `JSONBackend.path string`
   - `JSONBackend.Load()` — reads file, handles missing file, unmarshals
   - `JSONBackend.Save()` — marshals with indent, writes file
4. Implement `MemoryBackend`:
   - `MemoryBackend.tasks []model.Task`
   - `MemoryBackend.Load()` — returns copy of internal slice
   - `MemoryBackend.Save()` — replaces internal slice with copy

## Phase 3: Refactor Store
1. Change `Store` struct: replace `path string` with `backend Backend`
2. Add `NewWithBackend(b Backend) *Store` constructor
3. Update `New(path string)` to create a `JSONBackend` and call `NewWithBackend`
4. Replace all `s.load()` calls with `s.backend.Load()`
5. Replace all `s.save()` calls with `s.backend.Save()`
6. Remove the old private `load()` and `save()` methods

## Phase 4: Write Tests
1. Test `JSONBackend` directly (load/save round-trip, missing file, corrupt file)
2. Test `MemoryBackend` directly
3. Test `Store` with `MemoryBackend` to prove interface works
4. Run all existing tests — they should pass unchanged since `New(path)` still works

## Phase 5: Clean Up
1. Verify file organization is clean
2. Run `go vet` and fix any issues
3. Final test run
