package state

import (
	"fmt"
	"os"
	"path/filepath"
)

// AppendHistory appends a single line to {phaseDir}/history.md, creating the
// file if it does not exist. Errors are non-fatal — callers should ignore them
// or log them at warn level.
func AppendHistory(phaseDir, entry string) error {
	path := filepath.Join(phaseDir, "history.md")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintln(f, entry)
	return err
}
