# Task 3: Extract Storage Backend Interface

## Overview

The `internal/store/store.go` file is a monolith — it mixes business logic (CRUD operations, ID generation, searching) with file I/O (JSON marshaling, `os.ReadFile`, `os.WriteFile`). Extract the file I/O into a backend interface so the storage layer can be swapped (e.g., JSON file, SQLite, in-memory for tests).

## Requirements

### Interface Definition

Create a `Backend` interface that abstracts raw persistence:

```go
type Backend interface {
    Load() ([]model.Task, error)
    Save(tasks []model.Task) error
}
```

### JSON Backend

Extract the current `load()` / `save()` methods into a `JSONBackend` struct that implements `Backend`:
- `JSONBackend` holds the file path
- `JSONBackend.Load()` reads and unmarshals the JSON file
- `JSONBackend.Save()` marshals and writes the JSON file

### Store Refactor

- `Store` should accept a `Backend` in its constructor instead of a file path
- All current `Store` methods (`Add`, `List`, `Get`, `Complete`, `Delete`, `Update`, `Search`, `Count`, etc.) should work through the `Backend` interface instead of calling `load()`/`save()` directly
- The public API of `Store` must not change (same method signatures)

### In-Memory Backend

Create an `MemoryBackend` for testing:
- Stores tasks in a slice in memory
- Useful for unit tests that don't need disk I/O

### Constructor Helpers

Provide convenience constructors:
```go
func New(path string) *Store          // uses JSONBackend (preserves existing API)
func NewWithBackend(b Backend) *Store // uses any backend
```

### Constraints

- **All existing tests must pass without modification** (the `New(path)` constructor must still work)
- New code must have tests (backend interface, MemoryBackend, JSONBackend independently)
- The file organization should be:
  - `store.go` — Store struct and business logic
  - `backend.go` — Backend interface + JSONBackend + MemoryBackend
  - Or similar clean separation

## Non-Goals

- Do NOT implement a SQLite backend (just the interface and JSON/Memory implementations)
- Do NOT add caching or performance improvements (that's a separate task)
- Do NOT change the Store's public method signatures
