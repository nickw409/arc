package state

import (
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
	panic("not implemented")
}

// Write atomically writes state to disk.
func (s *StateFile) Write(state *arc.PhaseState) error {
	panic("not implemented")
}

// Update reads, applies fn, and atomically writes back.
func (s *StateFile) Update(fn func(state *arc.PhaseState) error) error {
	panic("not implemented")
}
