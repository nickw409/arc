package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/nwiley/arc/internal/arc"
)

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
func (s *StateFile) Read() (*arc.PhaseState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return readState(s.path)
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

	state, err := readState(s.path)
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
	var state arc.PhaseState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parsing state file %s: %w", path, err)
	}
	return &state, nil
}

func writeStateAtomic(path string, state *arc.PhaseState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
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
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming temp file: %w", err)
	}
	return nil
}
