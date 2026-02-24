# Phase: refactor

## Objective

Extract the file I/O from Store into a Backend interface with JSONBackend and MemoryBackend implementations. Migrate Store to use the Backend interface.

## Files

### Create
- `internal/store/backend.go` — Backend interface, JSONBackend implementation

### Modify
- `internal/store/store.go` — Replace `path string` field with `backend Backend`, update all methods to use `backend.Load()`/`backend.Save()`, add `NewWithBackend` constructor, keep `New(path)` as convenience wrapper

## Types and Signatures

```go
// In backend.go:

// Backend abstracts raw task persistence.
type Backend interface {
    Load() ([]model.Task, error)
    Save(tasks []model.Task) error
}

// JSONBackend reads and writes tasks to a JSON file.
type JSONBackend struct {
    Path string
}

// NewJSONBackend creates a Backend that persists to the given file path.
func NewJSONBackend(path string) *JSONBackend

func (b *JSONBackend) Load() ([]model.Task, error)
// - Reads file with os.ReadFile
// - If file doesn't exist (os.IsNotExist), returns empty slice, nil
// - If file is empty (len(data) == 0), returns empty slice, nil
// - Unmarshals JSON into []model.Task
// - On JSON parse error, returns nil, fmt.Errorf("parse %s: %w", b.Path, err)

func (b *JSONBackend) Save(tasks []model.Task) error
// - Marshals with json.MarshalIndent (prefix "", indent "  ")
// - Writes with os.WriteFile, perm 0644

// MemoryBackend stores tasks in memory for testing.
type MemoryBackend struct {
    tasks []model.Task
}

// NewMemoryBackend creates an in-memory Backend, optionally pre-loaded with tasks.
func NewMemoryBackend(initial ...[]model.Task) *MemoryBackend
// If len(initial) > 0 && initial[0] != nil, copies the slice:
//   b.tasks = make([]model.Task, len(initial[0]))
//   copy(b.tasks, initial[0])
// Otherwise starts with empty slice: b.tasks = []model.Task{}

func (b *MemoryBackend) Load() ([]model.Task, error)
// Returns a COPY of the internal slice (not a reference):
//   result := make([]model.Task, len(b.tasks))
//   copy(result, b.tasks)
//   return result, nil

func (b *MemoryBackend) Save(tasks []model.Task) error
// Replaces internal slice with a COPY of the input:
//   b.tasks = make([]model.Task, len(tasks))
//   copy(b.tasks, tasks)
//   return nil


// In store.go (modified):

type Store struct {
    backend Backend  // replaces: path string (remove the path field entirely)
}

// NewWithBackend creates a Store with the given Backend.
func NewWithBackend(b Backend) *Store
// Implementation: return &Store{backend: b}

// New creates a Store backed by a JSON file (preserves existing API).
func New(path string) *Store
// Implementation: return NewWithBackend(NewJSONBackend(path))

// Private methods (keep these, modify implementation only):

func (s *Store) load() ([]model.Task, error)
// Implementation: return s.backend.Load()

func (s *Store) save(tasks []model.Task) error
// Implementation: return s.backend.Save(tasks)
```

## Error Types

```go
// No new error types. Existing fmt.Errorf patterns remain.
// Backend implementations propagate os and json errors as-is.
```

## Dependencies

None new.

## DO NOT

- [ ] Do NOT change the public method signatures on Store (Add, List, Get, Delete, Update, Complete, Search, Count, CountByStatus, ListByStatus, ListByPriority)
- [ ] Do NOT change `New(path string) *Store` — it must still work as before
- [ ] Do NOT add caching or performance improvements (that is task 4)
- [ ] Do NOT implement a SQLite backend
- [ ] Do NOT change the JSON format (field names, indentation)
- [ ] Do NOT leave `load()` and `save()` partially migrated — Store currently has two private methods `func (s *Store) load() ([]model.Task, error)` and `func (s *Store) save(tasks []model.Task) error`. Keep these methods but change their implementations to delegate to `s.backend.Load()` and `s.backend.Save()` respectively. All callers (Add, List, Get, etc.) continue calling `s.load()`/`s.save()` as before; only the internal implementation of these private methods changes to use the backend.
- [ ] Do NOT change `func (s *Store) nextID() int` — this is a private method on Store that computes the next available task ID. Its source code must remain byte-for-byte identical. Do not add any calls to `s.backend`; it must continue calling `s.load()` exactly as before (the delegation to backend happens inside `s.load()`, not inside `nextID()`).

## Test Cases

All tests go in `internal/store/backend_test.go` unless otherwise specified.

### TestJSONBackendRoundTrip
**Setup:** Create temp file path (do not create the file) using `filepath.Join(t.TempDir(), "test.json")`
**Input:** Create JSONBackend with the temp path. Create 3 tasks with specific values (ID: 1, Title: "Task1", Status: model.StatusPending; ID: 2, Title: "Task2", Status: model.StatusCompleted; ID: 3, Title: "Task3", Status: model.StatusPending). Call `Save()` with the 3 tasks via the JSONBackend instance, then call `Load()` via the same backend instance
**Expected:** Loaded tasks slice has len()==3. Use `reflect.DeepEqual(loaded[i], saved[i])` to verify each element matches the corresponding saved task exactly (all fields including time.Time)

### TestJSONBackendMissingFile
**Input:** Load from JSONBackend pointing to non-existent path
**Expected:** Returns empty slice, nil error

### TestJSONBackendEmptyFile
**Setup:** Write zero-byte file to temp path
**Input:** Load from JSONBackend
**Expected:** Returns empty slice, nil error

### TestJSONBackendCorruptFile
**Setup:** Write `{invalid json` to temp file
**Input:** Load from JSONBackend
**Expected:** Returns non-nil error

### TestJSONBackendPathIsDirectory
**Setup:** Create temp directory using `t.TempDir()`
**Input:** Load from JSONBackend pointing to the directory path (not a file)
**Expected:** Returns non-nil error

### TestJSONBackendSaveFailure
**Setup:** Create temp directory using `t.TempDir()`, create a subdirectory inside it, then remove write permissions from the parent directory using `os.Chmod(parentDir, 0444)`
**Input:** Save 2 tasks (ID: 1, Title: "A", Status: model.StatusPending; ID: 2, Title: "B", Status: model.StatusCompleted) via JSONBackend pointing to a file inside the read-only directory
**Expected:** Returns non-nil error

### TestJSONBackendEmptyStringPath
**Input:** Create `NewJSONBackend("")`, save 1 task (ID: 1, Title: "Test", Status: model.StatusPending)
**Expected:** Returns non-nil error (cannot save to empty path)

### TestMemoryBackendRoundTrip
**Input:** Create MemoryBackend using `NewMemoryBackend(nil)`. Create 3 tasks (ID: 1, Title: "A", Status: model.StatusPending; ID: 2, Title: "B", Status: model.StatusCompleted; ID: 3, Title: "C", Status: model.StatusPending). Call Save with the 3 tasks, then Load
**Expected:** Loaded tasks slice has len()==3. Use `reflect.DeepEqual(loaded[i], saved[i])` to verify each element matches the corresponding saved task exactly (all fields)

### TestMemoryBackendIsolation
**Input:** Save 2 tasks to MemoryBackend, Load, modify the returned slice (append a third task), Load again
**Expected:** Second Load returns original 2 tasks (copies, not references — mutation does not affect internal state)

### TestMemoryBackendNoInitialTasks
**Input:** Create `NewMemoryBackend()` with no arguments, call `Load()`
**Expected:** Returns empty slice `[]model.Task{}`, nil error

### TestMemoryBackendWithNilSlice
**Input:** Create `NewMemoryBackend(nil)`, call `Load()`
**Expected:** Returns empty slice `[]model.Task{}`, nil error (behaves identically to no-argument form)

### TestMemoryBackendWithInitialTasks
**Input:** Create 2 tasks (ID: 1, Title: "Init1", Status: model.StatusPending; ID: 2, Title: "Init2", Status: model.StatusCompleted). Create `NewMemoryBackend(initialTasks)` passing the 2 tasks, then mutate the original `initialTasks` slice by appending a third task with ID: 3, Title: "Init3", Status: model.StatusPending. Call `Load()` on the backend
**Expected:** `Load()` returns a slice with `len() == 2` containing only the first 2 tasks (backend copied the initial slice, mutation does not affect it)

### TestMemoryBackendSaveEmptySlice
**Input:** Create `NewMemoryBackend()` with 2 initial tasks, call `Save([]model.Task{})`, then `Load()`
**Expected:** `Load()` returns empty slice `[]model.Task{}`, nil error (empty save clears backend)

### TestMemoryBackendSaveNilSlice
**Input:** Create `NewMemoryBackend()`, call `Save(nil)`, then `Load()`
**Expected:** `Load()` returns empty slice `[]model.Task{}`, nil error (nil save clears backend)

### TestJSONBackendPreservesIndent
**Setup:** Create temp file using `filepath.Join(t.TempDir(), "indent.json")`. Create 2 tasks (ID: 1, Title: "First", Status: model.StatusPending; ID: 2, Title: "Second", Status: model.StatusCompleted). Save via JSONBackend pointing to temp file
**Input:** Read the file bytes using `os.ReadFile`, then Load via the same JSONBackend instance
**Expected:**
- File bytes when converted to string must contain newlines and 2-space indentation (verify by checking `strings.Contains(string(fileBytes), "\n  ")` returns true)
- Loaded tasks slice has len()==2 and each element matches the corresponding saved task in all fields (ID, Title, Description, Status, Priority, CreatedAt)

### TestBackendInterfacePolymorphism
**Input:** Create `var b Backend = NewJSONBackend(tempFile)`, save 2 tasks via `b.Save()`, load via `b.Load()`
**Expected:** Loaded tasks match saved tasks (verifies interface contract works)

### TestMemoryBackendInterfacePolymorphism
**Input:** Create `var b Backend = NewMemoryBackend()`, save 2 tasks (ID: 1, Title: "X", Status: model.StatusPending; ID: 2, Title: "Y", Status: model.StatusCompleted) via `b.Save()`, load via `b.Load()`
**Expected:** Loaded tasks match saved tasks (verifies MemoryBackend satisfies Backend interface)

### TestStoreListPropagatesBackendError
**Setup:** Create JSONBackend pointing to a directory (not a file) that will cause Load to fail
**Input:** Create Store with `NewWithBackend(backend)`, call `List()`
**Expected:** `List()` returns non-nil error from backend.Load

### TestStoreAddPropagatesBackendError
**Setup:** Create temp directory, remove write permissions using `os.Chmod(dir, 0444)`, create JSONBackend pointing to file in read-only directory
**Input:** Create Store with `NewWithBackend(backend)`, call `Add("Test", "desc", model.PriorityMedium)`
**Expected:** Returns non-nil error from backend.Save

### TestStoreGetPropagatesBackendError
**Setup:** Create JSONBackend pointing to a directory (not a file)
**Input:** Create Store with `NewWithBackend(backend)`, call `Get(1)`
**Expected:** Returns nil task and non-nil error from backend.Load

### TestStoreUpdatePropagatesBackendError
**Setup:** Create temp directory, remove write permissions, create JSONBackend pointing to file in read-only directory
**Input:** Create Store with `NewWithBackend(backend)`, call `Update(1, "title", "desc", model.StatusCompleted, model.PriorityHigh)`
**Expected:** Returns non-nil error from backend.Save

### TestStoreCompletePropagatesBackendError
**Setup:** Create temp directory, remove write permissions, create JSONBackend pointing to file in read-only directory
**Input:** Create Store with `NewWithBackend(backend)`, call `Complete(1)`
**Expected:** Returns non-nil error from backend.Save

### TestStoreDeletePropagatesBackendError
**Setup:** Create temp directory, remove write permissions, create JSONBackend pointing to file in read-only directory
**Input:** Create Store with `NewWithBackend(backend)`, call `Delete(1)`
**Expected:** Returns non-nil error from backend.Save

### TestStoreSearchPropagatesBackendError
**Setup:** Create JSONBackend pointing to a directory (not a file)
**Input:** Create Store with `NewWithBackend(backend)`, call `Search("test")`
**Expected:** Returns empty slice and non-nil error from backend.Load

### TestStoreCountPropagatesBackendError
**Setup:** Create JSONBackend pointing to a directory (not a file)
**Input:** Create Store with `NewWithBackend(backend)`, call `Count()`
**Expected:** Returns 0 and non-nil error from backend.Load

### TestStoreCountByStatusPropagatesBackendError
**Setup:** Create JSONBackend pointing to a directory (not a file)
**Input:** Create Store with `NewWithBackend(backend)`, call `CountByStatus(model.StatusPending)`
**Expected:** Returns 0 and non-nil error from backend.Load

### TestStoreListByStatusPropagatesBackendError
**Setup:** Create JSONBackend pointing to a directory (not a file)
**Input:** Create Store with `NewWithBackend(backend)`, call `ListByStatus(model.StatusPending)`
**Expected:** Returns empty slice and non-nil error from backend.Load

### TestStoreListByPriorityPropagatesBackendError
**Setup:** Create JSONBackend pointing to a directory (not a file)
**Input:** Create Store with `NewWithBackend(backend)`, call `ListByPriority(model.PriorityHigh)`
**Expected:** Returns empty slice and non-nil error from backend.Load

### TestStoreAddBackwardsCompatible
**Input:** Create Store with `New(tempFile)`, call `Add("Task 1", "desc", model.PriorityHigh)`, then `Get(1)`
**Expected:** `Get(1)` returns task with `Title == "Task 1"`, `Priority == model.PriorityHigh` (explicit regression test proving New still works)

### TestAllExistingTestsPass
**Input:** `go test ./...`
**Expected:** All existing tests pass without modification

## Edge Cases

1. **JSONBackend file permissions** — Must use 0644 to match existing behavior
3. **JSON indentation** — Must use `json.MarshalIndent(tasks, "", "  ")` to match existing format

## Integration Points

### Consumed by
- No consumers yet — interface and JSONBackend are created but not integrated with Store this phase

### Depends on
- Phase characterize: Characterization tests validate the refactor didn't break anything

### Exports
- `Backend` interface
- `JSONBackend` / `NewJSONBackend`
- `MemoryBackend` / `NewMemoryBackend`
- `NewWithBackend`
