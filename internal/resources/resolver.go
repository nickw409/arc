package resources

import (
	"fmt"
	"strings"
)

// Resolver looks up resources by checking disk directories before the embedded FS.
// Search order: each dir in searchDirs, then embedded FS.
type Resolver struct {
	searchDirs []string // absolute paths checked in order before embedded FS
}

// NewResolver builds a Resolver that checks projectDir/.arc and homeDir/.arc before embedded FS.
// Empty string arguments are silently skipped (no search dir is added for them).
func NewResolver(projectDir, homeDir string) *Resolver {
	var dirs []string
	for _, s := range []string{projectDir, homeDir} {
		if s != "" {
			dirs = append(dirs, s+"/.arc")
		}
	}
	return &Resolver{searchDirs: dirs}
}

func validateResourceName(name string) error {
	if strings.Contains(name, "..") || strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("invalid resource name %q", name)
	}
	return nil
}
