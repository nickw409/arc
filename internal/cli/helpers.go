package cli

import (
	"os"
	"path/filepath"
)

// resolveArcHome returns the Arc home directory. It checks the ARC_HOME
// environment variable first, then falls back to the directory two levels
// above the running executable (i.e. the install prefix).
func resolveArcHome() string {
	if v := os.Getenv("ARC_HOME"); v != "" {
		return v
	}
	ex, err := os.Executable()
	if err == nil {
		return filepath.Dir(filepath.Dir(ex))
	}
	return ""
}
