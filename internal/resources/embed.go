package resources

import (
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

//go:embed prompts/**/*.md
var promptsFS embed.FS

//go:embed templates/*.md
var templatesFS embed.FS

//go:embed enforcement/hooks/*
var enforcementFS embed.FS

//go:embed guides/*.md
var guidesFS embed.FS

//go:embed recipes/*.yaml
var recipesFS embed.FS

// PromptBytes returns the raw markdown for a prompt path (e.g., "gate/impl.md").
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

// ListPrompts returns all prompt file paths relative to "prompts/" (e.g., "gate/impl.md").
func ListPrompts() ([]string, error) {
	var paths []string
	err := fs.WalkDir(promptsFS, "prompts", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.IsDir() && strings.HasSuffix(path, ".md") {
			paths = append(paths, strings.TrimPrefix(path, "prompts/"))
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing prompts: %w", err)
	}
	return paths, nil
}

// GuideBytes returns the raw markdown for a guide file (e.g., "guide.md").
func GuideBytes(name string) ([]byte, error) {
	return guidesFS.ReadFile(filepath.Join("guides", name))
}

// RecipeBytes returns the raw YAML for a built-in recipe (e.g., "add-endpoint").
func RecipeBytes(name string) ([]byte, error) {
	return recipesFS.ReadFile(filepath.Join("recipes", name+".yaml"))
}

// ListBuiltInRecipes returns all built-in recipe names (without .yaml extension).
func ListBuiltInRecipes() []string {
	entries, err := fs.ReadDir(recipesFS, "recipes")
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
