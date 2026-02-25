package pipeline

import (
	"os"
	"path/filepath"
	"strings"
)

// ReadMemory returns the saved memory for stateName from {phaseDir}/memory/{stateName}.md.
// Returns "", nil if the file does not exist.
func ReadMemory(phaseDir, stateName string) (string, error) {
	path := filepath.Join(phaseDir, "memory", stateName+".md")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ExtractMemory parses the ## Memory section from agent stdout.
// Returns "" if no ## Memory section is present.
// Captures everything from "## Memory" until the next "##" section or end of output.
func ExtractMemory(output string) string {
	const header = "## Memory"
	idx := strings.Index(output, header)
	if idx < 0 {
		return ""
	}
	start := idx + len(header)
	rest := output[start:]
	// Find the next ## section
	nextHeader := strings.Index(rest, "\n##")
	if nextHeader >= 0 {
		rest = rest[:nextHeader]
	}
	return strings.TrimSpace(rest)
}

// WriteMemory saves content to {phaseDir}/memory/{stateName}.md.
func WriteMemory(phaseDir, stateName, content string) error {
	memDir := filepath.Join(phaseDir, "memory")
	if err := os.MkdirAll(memDir, 0755); err != nil {
		return err
	}
	path := filepath.Join(memDir, stateName+".md")
	return os.WriteFile(path, []byte(content), 0644)
}
