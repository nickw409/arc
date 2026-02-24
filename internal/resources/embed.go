package resources

import (
	"embed"
	"io/fs"
	"path/filepath"
	"strings"
)

//go:embed workflows/*.yaml
var workflowsFS embed.FS

//go:embed prompts/**/*.md prompts/*.md
var promptsFS embed.FS

//go:embed templates/*.md
var templatesFS embed.FS

//go:embed enforcement/hooks/*
var enforcementFS embed.FS

//go:embed guides/*.md
var guidesFS embed.FS

// WorkflowBytes returns the raw YAML for a workflow type (e.g., "feature").
func WorkflowBytes(workflowType string) ([]byte, error) {
	return workflowsFS.ReadFile(filepath.Join("workflows", workflowType+".yaml"))
}

// PromptBytes returns the raw markdown for a prompt path (e.g., "feature/qa.md").
func PromptBytes(promptPath string) ([]byte, error) {
	return promptsFS.ReadFile(filepath.Join("prompts", promptPath))
}

// TemplateBytes returns the raw markdown for a template (e.g., "plan-template.md").
func TemplateBytes(name string) ([]byte, error) {
	return templatesFS.ReadFile(filepath.Join("templates", name))
}

// HookBytes returns the raw bytes for an enforcement hook (e.g., "pre-commit").
func HookBytes(name string) ([]byte, error) {
	return enforcementFS.ReadFile(filepath.Join("enforcement", "hooks", name))
}

// ListWorkflows returns all available workflow type names (without .yaml extension).
func ListWorkflows() []string {
	entries, err := fs.ReadDir(workflowsFS, "workflows")
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") {
			names = append(names, strings.TrimSuffix(e.Name(), ".yaml"))
		}
	}
	return names
}

// ListPrompts returns all prompt file paths relative to "prompts/" (e.g., "feature/qa.md").
func ListPrompts() []string {
	var paths []string
	fs.WalkDir(promptsFS, "prompts", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".md") {
			paths = append(paths, strings.TrimPrefix(path, "prompts/"))
		}
		return nil
	})
	return paths
}

// GuideBytes returns the raw markdown for a guide file (e.g., "guide.md").
func GuideBytes(name string) ([]byte, error) {
	return guidesFS.ReadFile(filepath.Join("guides", name))
}
