package project

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/nwiley/arc/internal/config"
	"gopkg.in/yaml.v3"
)

// InitOptions configures project initialization.
type InitOptions struct {
	ProjectRoot string
	Force       bool
}

// Init initializes arc in a project directory.
func Init(opts InitOptions) (*config.Config, error) {
	arcYamlPath := filepath.Join(opts.ProjectRoot, ".arc.yaml")

	// 1. Check if already initialized
	if _, err := os.Stat(arcYamlPath); err == nil && !opts.Force {
		return nil, fmt.Errorf("already initialized: .arc.yaml exists (use --force to overwrite)")
	}

	// 2. Auto-detect language/runner
	detection := Detect(opts.ProjectRoot)

	// 3. Build config
	cfg := &config.Config{
		Language:       detection.Language,
		Runner:         detection.Runner,
		DefaultPackage: detection.DefaultPackage,
		BuildCommand:   detection.BuildCommand,
		TestCommand:    detection.TestCommand,
		Git: config.GitConfig{
			CommitStyle: "conventional",
			Sign:        false,
			CoAuthor:    false,
		},
	}

	// Write .arc.yaml
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(arcYamlPath, data, 0644); err != nil {
		return nil, fmt.Errorf("write .arc.yaml: %w", err)
	}

	// 4. Create .plans/active/ and .plans/archive/
	for _, dir := range []string{
		filepath.Join(opts.ProjectRoot, ".plans", "active"),
		filepath.Join(opts.ProjectRoot, ".plans", "archive"),
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("create directory %s: %w", dir, err)
		}
	}

	// 5. Check for .git/
	hasGit := false
	if _, err := os.Stat(filepath.Join(opts.ProjectRoot, ".git")); err == nil {
		hasGit = true
	}

	if hasGit {
		// Install git hooks
		if err := InstallGitHooks(opts.ProjectRoot); err != nil {
			return nil, fmt.Errorf("install git hooks: %w", err)
		}

		// Install Claude Code hooks
		if err := InstallClaudeHooks(opts.ProjectRoot); err != nil {
			return nil, fmt.Errorf("install claude hooks: %w", err)
		}

		// Add .plans/ to .gitignore
		if err := addToGitignore(opts.ProjectRoot, ".plans/"); err != nil {
			return nil, fmt.Errorf("update .gitignore: %w", err)
		}
	} else {
		slog.Warn("no .git directory found, skipping hook installation")
	}

	// 6. Write .claude/commands/arc-plan.md
	if err := InstallSlashCommand(opts.ProjectRoot); err != nil {
		return nil, fmt.Errorf("install slash command: %w", err)
	}

	return cfg, nil
}

func addToGitignore(projectRoot, entry string) error {
	gitignorePath := filepath.Join(projectRoot, ".gitignore")

	existing, err := os.ReadFile(gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	if err == nil {
		// Check if entry already exists
		for _, line := range strings.Split(string(existing), "\n") {
			if strings.TrimSpace(line) == entry {
				return nil // already present
			}
		}
		// Append
		content := string(existing)
		if !strings.HasSuffix(content, "\n") && len(content) > 0 {
			content += "\n"
		}
		content += entry + "\n"
		return os.WriteFile(gitignorePath, []byte(content), 0644)
	}

	// Create new
	return os.WriteFile(gitignorePath, []byte(entry+"\n"), 0644)
}
