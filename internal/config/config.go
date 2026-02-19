package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Supported values for validation.
var (
	SupportedLanguages = []string{"rust", "go", "typescript", "python", "unknown"}
	SupportedRunners   = []string{"cargo-nextest", "cargo-test", "go-test", "vitest", "pytest", "unknown"}
	SupportedStyles    = []string{"conventional", "freeform"}
)

// Config represents the project-level .arc.yaml configuration.
type Config struct {
	Language       string         `yaml:"language"`
	Runner         string         `yaml:"runner"`
	DefaultPackage string         `yaml:"default_package"`
	BuildCommand   string         `yaml:"build_command"`
	TestCommand    string         `yaml:"test_command"`
	Git            GitConfig    `yaml:"git"`
	Audit          AuditConfig  `yaml:"audit"`
}

// AuditConfig holds settings for the validate (audit) command.
type AuditConfig struct {
	Prompt string `yaml:"prompt"`
}

// GitConfig holds git-related settings.
type GitConfig struct {
	CommitStyle string `yaml:"commit_style"`
	Sign        bool   `yaml:"sign"`
	CoAuthor    bool   `yaml:"co_author"`
}

// Load reads .arc.yaml from the given project root.
func Load(projectRoot string) (*Config, error) {
	data, err := os.ReadFile(filepath.Join(projectRoot, ".arc.yaml"))
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.Git.CommitStyle == "" {
		cfg.Git.CommitStyle = "conventional"
	}
	return &cfg, nil
}

// Save writes the config back to .arc.yaml in the given project root.
func Save(projectRoot string, cfg *Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return os.WriteFile(filepath.Join(projectRoot, ".arc.yaml"), data, 0644)
}

// Validate checks that language is in SupportedLanguages and runner is in SupportedRunners.
func (c *Config) Validate() error {
	var errs []string
	if !contains(SupportedLanguages, c.Language) {
		errs = append(errs, fmt.Sprintf("unsupported language %q", c.Language))
	}
	if !contains(SupportedRunners, c.Runner) {
		errs = append(errs, fmt.Sprintf("unsupported runner %q", c.Runner))
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
