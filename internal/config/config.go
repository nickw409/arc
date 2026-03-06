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
	Language       string      `yaml:"language"`
	Runner         string      `yaml:"runner"`
	DefaultPackage string      `yaml:"default_package"`
	BuildCommand   string      `yaml:"build_command"`
	TestCommand    string      `yaml:"test_command"`
	Git            GitConfig   `yaml:"git"`
	Audit          AuditConfig `yaml:"audit"`

	// v2 fields
	Agents      AgentsConfig `yaml:"agents"`
	MaxParallel int          `yaml:"max_parallel"`
	Budget      BudgetConfig `yaml:"budget"`
	Verifier    string       `yaml:"verifier"` // "always", "never", or "auto" (default: auto — enabled for medium/complex phases)
}

// AgentsConfig maps role names to agent adapter names.
type AgentsConfig struct {
	Default      string `yaml:"default"`
	Planner      string `yaml:"planner"`
	Impl         string `yaml:"impl"`
	Adversary    string `yaml:"adversary"`
	Verifier     string `yaml:"verifier"`
	Orchestrator string `yaml:"orchestrator"`
}

// BudgetConfig holds cost limit settings.
type BudgetConfig struct {
	MaxCost  float64 `yaml:"max_cost"`
	WarnCost float64 `yaml:"warn_cost"`
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
	BaseBranch  string `yaml:"base_branch"` // branch worktrees diverge from and merge into (default: current branch)
}

// AgentForRole returns the configured agent adapter name for a given role.
// Falls back to the default agent, then to "claude" if nothing is configured.
// Valid roles: "planner", "impl", "adversary", "verifier", "orchestrator".
func (c *Config) AgentForRole(role string) string {
	var specific string
	switch role {
	case "planner":
		specific = c.Agents.Planner
	case "impl":
		specific = c.Agents.Impl
	case "adversary":
		specific = c.Agents.Adversary
	case "verifier":
		specific = c.Agents.Verifier
	case "orchestrator":
		specific = c.Agents.Orchestrator
	}
	if specific != "" {
		return specific
	}
	if c.Agents.Default != "" {
		return c.Agents.Default
	}
	return "claude"
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
	// v2 defaults
	if cfg.Agents.Default == "" {
		cfg.Agents.Default = "claude"
	}
	if cfg.MaxParallel == 0 {
		cfg.MaxParallel = 3
	}
	if cfg.Verifier == "" {
		cfg.Verifier = "auto"
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

// Validate checks that language is in SupportedLanguages and runner is in SupportedRunners,
// and validates v2 fields.
func (c *Config) Validate() error {
	var errs []string
	if !contains(SupportedLanguages, c.Language) {
		errs = append(errs, fmt.Sprintf("unsupported language %q", c.Language))
	}
	if !contains(SupportedRunners, c.Runner) {
		errs = append(errs, fmt.Sprintf("unsupported runner %q", c.Runner))
	}
	if c.MaxParallel != 0 && c.MaxParallel < 1 {
		errs = append(errs, fmt.Sprintf("max_parallel must be >= 1, got %d", c.MaxParallel))
	}
	if c.Budget.MaxCost < 0 {
		errs = append(errs, fmt.Sprintf("budget.max_cost must be >= 0, got %g", c.Budget.MaxCost))
	}
	if c.Budget.WarnCost < 0 {
		errs = append(errs, fmt.Sprintf("budget.warn_cost must be >= 0, got %g", c.Budget.WarnCost))
	}
	if c.Verifier != "" && c.Verifier != "always" && c.Verifier != "never" && c.Verifier != "auto" {
		errs = append(errs, fmt.Sprintf("verifier must be always, never, or auto; got %q", c.Verifier))
	}
	if c.Budget.MaxCost > 0 && c.Budget.WarnCost > c.Budget.MaxCost {
		errs = append(errs, fmt.Sprintf("budget.warn_cost (%g) must be <= budget.max_cost (%g)", c.Budget.WarnCost, c.Budget.MaxCost))
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
