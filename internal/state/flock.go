package state

import "github.com/nwiley/arc/internal/arc"

// FlockUpdate acquires an exclusive filesystem-level lock, reads state,
// applies fn, writes state, and releases the lock.
func FlockUpdate(path string, fn func(state *arc.PhaseState) error) error {
	panic("not implemented")
}
