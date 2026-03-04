package state

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// AppendHistory appends a single line to {phaseDir}/history.md, creating the
// file if it does not exist. Uses flock for concurrency protection.
// Errors are non-fatal — callers should ignore them or log them at warn level.
func AppendHistory(phaseDir, entry string) error {
	path := filepath.Join(phaseDir, "history.md")
	lockPath := path + ".lock"
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("opening history lock: %w", err)
	}
	defer lockFile.Close()

	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("acquiring history lock: %w", err)
	}
	defer syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintln(f, entry)
	return err
}
