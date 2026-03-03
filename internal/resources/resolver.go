package resources

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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
			dirs = append(dirs, filepath.Join(s, ".arc"))
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

// WorkflowBytes returns raw YAML for a workflow type (e.g., "my-flow").
// Tries {dir}/workflows/{name}.yaml for each search dir, then falls back to embedded FS.
// Returns error if name contains ".." or "/" or "\" (path traversal protection).
func (r *Resolver) WorkflowBytes(name string) ([]byte, error) {
	if err := validateResourceName(name); err != nil {
		return nil, err
	}
	for _, dir := range r.searchDirs {
		path := filepath.Join(dir, "workflows", name+".yaml")
		data, err := os.ReadFile(path)
		if err == nil {
			return data, nil
		}
		if errors.Is(err, fs.ErrNotExist) || os.IsNotExist(err) {
			continue
		}
		return nil, err
	}
	return workflowsFS.ReadFile(filepath.Join("workflows", name+".yaml"))
}

// BlockBytes returns raw YAML for a block (e.g., "my-block").
// Tries {dir}/blocks/{name}.yaml for each search dir, then falls back to embedded FS.
// Returns error if name contains ".." or "/" or "\" (path traversal protection).
func (r *Resolver) BlockBytes(name string) ([]byte, error) {
	if err := validateResourceName(name); err != nil {
		return nil, err
	}
	for _, dir := range r.searchDirs {
		path := filepath.Join(dir, "blocks", name+".yaml")
		data, err := os.ReadFile(path)
		if err == nil {
			return data, nil
		}
		if errors.Is(err, fs.ErrNotExist) || os.IsNotExist(err) {
			continue
		}
		return nil, err
	}
	return blocksFS.ReadFile(filepath.Join("blocks", name+".yaml"))
}

// ListWorkflows returns all workflow type names (without .yaml extension).
// Disk names from all search dirs appear first; embedded names are appended if not already present.
func (r *Resolver) ListWorkflows() []string {
	seen := make(map[string]bool)
	var result []string
	for _, dir := range r.searchDirs {
		entries, err := os.ReadDir(filepath.Join(dir, "workflows"))
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") {
				name := strings.TrimSuffix(e.Name(), ".yaml")
				if !seen[name] {
					seen[name] = true
					result = append(result, name)
				}
			}
		}
	}
	for _, name := range ListWorkflows() {
		if !seen[name] {
			seen[name] = true
			result = append(result, name)
		}
	}
	return result
}

// ListBlocks returns all block names (without .yaml extension).
// Disk names from all search dirs appear first; embedded names are appended if not already present.
func (r *Resolver) ListBlocks() []string {
	seen := make(map[string]bool)
	var result []string
	for _, dir := range r.searchDirs {
		entries, err := os.ReadDir(filepath.Join(dir, "blocks"))
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") {
				name := strings.TrimSuffix(e.Name(), ".yaml")
				if !seen[name] {
					seen[name] = true
					result = append(result, name)
				}
			}
		}
	}
	for _, name := range ListBlocks() {
		if !seen[name] {
			seen[name] = true
			result = append(result, name)
		}
	}
	return result
}
