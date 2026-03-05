package state

import (
	"encoding/json"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"sync"

	"github.com/nwiley/arc/internal/arc"
)

// CurrentSchemaVersion is the schema version written to every new state.json.
// Older files with schema_version == 0 are treated as v0 and are transparently
// read as v1 (the migration is an identity operation).
// Files with a schema_version higher than CurrentSchemaVersion cause a hard
// error — the binary must be upgraded to read them.
const CurrentSchemaVersion = 1

// StateFile provides thread-safe atomic access to a phase's state.json.
type StateFile struct {
	path string
	mu   sync.Mutex
}

// NewStateFile creates a StateFile for the given path.
func NewStateFile(path string) *StateFile {
	return &StateFile{path: path}
}

// Path returns the file path.
func (s *StateFile) Path() string {
	return s.path
}

// Read reads and deserializes the state file.
// If the primary file fails to parse (corrupt JSON, empty file), it falls back
// to the .bak file and logs a warning.
func (s *StateFile) Read() (*arc.PhaseState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return readStateWithFallback(s.path)
}

// Write atomically writes state to disk.
func (s *StateFile) Write(state *arc.PhaseState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return writeStateAtomic(s.path, state)
}

// Update reads, applies fn, and atomically writes back.
func (s *StateFile) Update(fn func(state *arc.PhaseState) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := readStateWithFallback(s.path)
	if err != nil {
		return err
	}
	if err := fn(state); err != nil {
		return err
	}
	return writeStateAtomic(s.path, state)
}

func readState(path string) (*arc.PhaseState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading state file %s: %w", path, err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("state file %s is empty", path)
	}
	var s arc.PhaseState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing state file %s: %w", path, err)
	}
	// schema_version == 0 means pre-versioning (v0); treat as compatible.
	if s.SchemaVersion > CurrentSchemaVersion {
		return nil, fmt.Errorf("state file schema version %d is newer than supported version %d — upgrade arc", s.SchemaVersion, CurrentSchemaVersion)
	}
	// Verify CRC32 checksum when present (old files without checksums are fine).
	if s.Checksum != "" {
		savedChecksum := s.Checksum
		s.Checksum = ""
		body, marshalErr := json.MarshalIndent(&s, "", "  ")
		if marshalErr != nil {
			return nil, fmt.Errorf("re-marshaling state for checksum verification %s: %w", path, marshalErr)
		}
		computed := fmt.Sprintf("%08x", crc32.ChecksumIEEE(body))
		if computed != savedChecksum {
			return nil, fmt.Errorf("state file %s checksum mismatch: got %s, want %s", path, computed, savedChecksum)
		}
		s.Checksum = savedChecksum
	}
	return &s, nil
}

// readStateWithFallback tries the primary path first, and if it fails to read
// or parse, falls back to the .bak file. A warning is printed when falling back.
func readStateWithFallback(path string) (*arc.PhaseState, error) {
	state, err := readState(path)
	if err == nil {
		return state, nil
	}
	// Primary failed — try backup
	bakPath := path + ".bak"
	if _, statErr := os.Stat(bakPath); statErr != nil {
		// No backup available — return the original error
		return nil, err
	}
	bakState, bakErr := readState(bakPath)
	if bakErr != nil {
		return nil, err // return original error, not backup error
	}
	fmt.Fprintf(os.Stderr, "warning: state file %s failed (%v); falling back to %s\n", path, err, bakPath)
	return bakState, nil
}

func writeStateAtomic(path string, state *arc.PhaseState) error {
	state.SchemaVersion = CurrentSchemaVersion
	// Compute CRC32 over the JSON without the checksum field present.
	state.Checksum = ""
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}
	state.Checksum = fmt.Sprintf("%08x", crc32.ChecksumIEEE(body))
	// Re-marshal with the checksum included.
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state with checksum: %w", err)
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "state.json.*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("closing temp file: %w", err)
	}
	// Backup existing file before overwriting
	if _, statErr := os.Stat(path); statErr == nil {
		backupPath := path + ".bak"
		// Best-effort backup — don't fail the write if backup fails
		if existing, readErr := os.ReadFile(path); readErr == nil {
			os.WriteFile(backupPath, existing, 0644)
		}
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming temp file: %w", err)
	}
	return nil
}
