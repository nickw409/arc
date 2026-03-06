package prompt

import (
	"os"
	"path/filepath"
)

// LoadProjectContext reads .arc/context.md from the project root if it exists.
// Returns empty string if the file doesn't exist.
func LoadProjectContext(projectRoot string) string {
	path := filepath.Join(projectRoot, ".arc", "context.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}
