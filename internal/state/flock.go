package state

import (
	"fmt"
	"os"
	"syscall"

	"github.com/nwiley/arc/internal/arc"
)

// FlockUpdate acquires an exclusive filesystem-level lock, reads state,
// applies fn, writes state, and releases the lock.
func FlockUpdate(path string, fn func(state *arc.PhaseState) error) (retErr error) {
	lockPath := path + ".lock"
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("opening lock file %s: %w", lockPath, err)
	}
	defer lockFile.Close()

	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("acquiring flock: %w", err)
	}
	defer func() {
		if unlockErr := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN); unlockErr != nil && retErr == nil {
			retErr = fmt.Errorf("releasing flock: %w", unlockErr)
		}
	}()

	state, err := readState(path)
	if err != nil {
		return err
	}
	if err := fn(state); err != nil {
		return err
	}
	return writeStateAtomic(path, state)
}
