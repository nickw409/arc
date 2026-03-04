package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigLoadValid(t *testing.T) {
	dir := t.TempDir()
	yaml := `language: go
runner: go-test
default_package: myapp
build_command: "go build ./..."
test_command: "go test ./..."
git:
  commit_style: conventional
  sign: false
  co_author: false
`
	if err := os.WriteFile(filepath.Join(dir, ".arc.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Language != "go" {
		t.Fatalf("Language = %q, want %q", cfg.Language, "go")
	}
	if cfg.Runner != "go-test" {
		t.Fatalf("Runner = %q, want %q", cfg.Runner, "go-test")
	}
	if cfg.DefaultPackage != "myapp" {
		t.Fatalf("DefaultPackage = %q, want %q", cfg.DefaultPackage, "myapp")
	}
	if cfg.Git.CommitStyle != "conventional" {
		t.Fatalf("Git.CommitStyle = %q, want %q", cfg.Git.CommitStyle, "conventional")
	}
}

func TestConfigLoadMissingFile(t *testing.T) {
	_, err := Load("/nonexistent/path")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
	if !strings.Contains(err.Error(), "no such file") {
		t.Fatalf("error = %q, want it to contain 'no such file'", err.Error())
	}
}

func TestConfigLoadMissingGitSection(t *testing.T) {
	dir := t.TempDir()
	yaml := `language: go
runner: go-test
default_package: myapp
`
	if err := os.WriteFile(filepath.Join(dir, ".arc.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Git.CommitStyle != "conventional" {
		t.Fatalf("Git.CommitStyle = %q, want %q", cfg.Git.CommitStyle, "conventional")
	}
	if cfg.Git.Sign != false {
		t.Fatal("Git.Sign should default to false")
	}
	if cfg.Git.CoAuthor != false {
		t.Fatal("Git.CoAuthor should default to false")
	}
}

func TestConfigLoadExtraUnknownFields(t *testing.T) {
	dir := t.TempDir()
	yaml := `language: go
runner: go-test
future_option: true
`
	if err := os.WriteFile(filepath.Join(dir, ".arc.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(dir)
	if err != nil {
		t.Fatalf("unknown fields should not cause error, got: %v", err)
	}
}

func TestConfigValidateUnknownLanguage(t *testing.T) {
	cfg := &Config{Language: "cobol", Runner: "go-test"}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for unsupported language, got nil")
	}
	if !strings.Contains(err.Error(), `unsupported language "cobol"`) {
		t.Fatalf("error = %q, want it to contain 'unsupported language \"cobol\"'", err.Error())
	}
}

func TestConfigValidateUnknownRunner(t *testing.T) {
	cfg := &Config{Language: "go", Runner: "bazel"}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for unsupported runner, got nil")
	}
	if !strings.Contains(err.Error(), `unsupported runner "bazel"`) {
		t.Fatalf("error = %q, want it to contain 'unsupported runner \"bazel\"'", err.Error())
	}
}

func TestConfigValidateValid(t *testing.T) {
	cfg := &Config{Language: "go", Runner: "go-test"}
	err := cfg.Validate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConfigValidateBothInvalid(t *testing.T) {
	cfg := &Config{Language: "cobol", Runner: "bazel"}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for both invalid fields, got nil")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "unsupported language") {
		t.Fatalf("error should mention unsupported language, got: %q", errStr)
	}
	if !strings.Contains(errStr, "unsupported runner") {
		t.Fatalf("error should mention unsupported runner, got: %q", errStr)
	}
}

func TestConfigLoadEmptyFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".arc.yaml"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(dir)
	if err != nil {
		t.Fatalf("empty file should not error, got: %v", err)
	}
}

func TestConfigLoadInvalidYAMLSyntax(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".arc.yaml"), []byte("{invalid: yaml: [broken"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
	errStr := strings.ToLower(err.Error())
	if !strings.Contains(errStr, "yaml") && !strings.Contains(errStr, "unmarshal") {
		t.Fatalf("error = %q, want it to contain 'yaml' or 'unmarshal'", err.Error())
	}
}

func TestConfigValidateEmptyStringLanguage(t *testing.T) {
	cfg := &Config{Language: "", Runner: "go-test"}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty language, got nil")
	}
}

func TestLoadV2Fields(t *testing.T) {
	dir := t.TempDir()
	content := `language: go
runner: go-test
agents:
  default: claude
  planner: opus
  impl: sonnet
  adversary: opus
  verifier: sonnet
  orchestrator: claude
max_parallel: 5
budget:
  max_cost: 50.00
  warn_cost: 25.00
`
	if err := os.WriteFile(filepath.Join(dir, ".arc.yaml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Agents.Default != "claude" {
		t.Fatalf("Agents.Default = %q, want %q", cfg.Agents.Default, "claude")
	}
	if cfg.Agents.Planner != "opus" {
		t.Fatalf("Agents.Planner = %q, want %q", cfg.Agents.Planner, "opus")
	}
	if cfg.Agents.Impl != "sonnet" {
		t.Fatalf("Agents.Impl = %q, want %q", cfg.Agents.Impl, "sonnet")
	}
	if cfg.Agents.Adversary != "opus" {
		t.Fatalf("Agents.Adversary = %q, want %q", cfg.Agents.Adversary, "opus")
	}
	if cfg.Agents.Verifier != "sonnet" {
		t.Fatalf("Agents.Verifier = %q, want %q", cfg.Agents.Verifier, "sonnet")
	}
	if cfg.Agents.Orchestrator != "claude" {
		t.Fatalf("Agents.Orchestrator = %q, want %q", cfg.Agents.Orchestrator, "claude")
	}
	if cfg.MaxParallel != 5 {
		t.Fatalf("MaxParallel = %d, want 5", cfg.MaxParallel)
	}
	if cfg.Budget.MaxCost != 50.00 {
		t.Fatalf("Budget.MaxCost = %g, want 50.00", cfg.Budget.MaxCost)
	}
	if cfg.Budget.WarnCost != 25.00 {
		t.Fatalf("Budget.WarnCost = %g, want 25.00", cfg.Budget.WarnCost)
	}
}

func TestLoadV2Defaults(t *testing.T) {
	dir := t.TempDir()
	content := `language: go
runner: go-test
`
	if err := os.WriteFile(filepath.Join(dir, ".arc.yaml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Agents.Default != "claude" {
		t.Fatalf("Agents.Default = %q, want %q", cfg.Agents.Default, "claude")
	}
	if cfg.MaxParallel != 3 {
		t.Fatalf("MaxParallel = %d, want 3", cfg.MaxParallel)
	}
	if cfg.Budget.MaxCost != 0 {
		t.Fatalf("Budget.MaxCost = %g, want 0", cfg.Budget.MaxCost)
	}
	if cfg.Budget.WarnCost != 0 {
		t.Fatalf("Budget.WarnCost = %g, want 0", cfg.Budget.WarnCost)
	}
}

func TestLoadBackwardCompatible(t *testing.T) {
	dir := t.TempDir()
	content := `language: go
runner: go-test
default_package: myapp
build_command: "go build ./..."
test_command: "go test ./..."
git:
  commit_style: conventional
  sign: false
  co_author: false
`
	if err := os.WriteFile(filepath.Join(dir, ".arc.yaml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("v1 config should load without error, got: %v", err)
	}

	if cfg.Language != "go" {
		t.Fatalf("Language = %q, want %q", cfg.Language, "go")
	}
	if cfg.Runner != "go-test" {
		t.Fatalf("Runner = %q, want %q", cfg.Runner, "go-test")
	}
	// v2 defaults applied
	if cfg.Agents.Default != "claude" {
		t.Fatalf("Agents.Default = %q, want %q", cfg.Agents.Default, "claude")
	}
	if cfg.MaxParallel != 3 {
		t.Fatalf("MaxParallel = %d, want 3", cfg.MaxParallel)
	}
}

func TestAgentForRole(t *testing.T) {
	tests := []struct {
		name   string
		cfg    Config
		role   string
		want   string
	}{
		{
			name: "specific planner overrides default",
			cfg:  Config{Agents: AgentsConfig{Default: "sonnet", Planner: "opus"}},
			role: "planner",
			want: "opus",
		},
		{
			name: "falls back to default when role not set",
			cfg:  Config{Agents: AgentsConfig{Default: "sonnet"}},
			role: "impl",
			want: "sonnet",
		},
		{
			name: "falls back to claude when nothing configured",
			cfg:  Config{},
			role: "adversary",
			want: "claude",
		},
		{
			name: "impl role",
			cfg:  Config{Agents: AgentsConfig{Default: "claude", Impl: "haiku"}},
			role: "impl",
			want: "haiku",
		},
		{
			name: "adversary role",
			cfg:  Config{Agents: AgentsConfig{Adversary: "opus"}},
			role: "adversary",
			want: "opus",
		},
		{
			name: "verifier role",
			cfg:  Config{Agents: AgentsConfig{Default: "claude", Verifier: "sonnet"}},
			role: "verifier",
			want: "sonnet",
		},
		{
			name: "orchestrator role",
			cfg:  Config{Agents: AgentsConfig{Orchestrator: "haiku"}},
			role: "orchestrator",
			want: "haiku",
		},
		{
			name: "unknown role falls back to default",
			cfg:  Config{Agents: AgentsConfig{Default: "sonnet"}},
			role: "unknown-role",
			want: "sonnet",
		},
		{
			name: "unknown role falls back to claude",
			cfg:  Config{},
			role: "unknown-role",
			want: "claude",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.AgentForRole(tt.role)
			if got != tt.want {
				t.Fatalf("AgentForRole(%q) = %q, want %q", tt.role, got, tt.want)
			}
		})
	}
}

func TestValidateBudget(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{
			name: "zero budget is valid (unlimited)",
			cfg:  Config{Language: "go", Runner: "go-test", MaxParallel: 3},
		},
		{
			name: "valid max and warn",
			cfg:  Config{Language: "go", Runner: "go-test", MaxParallel: 3, Budget: BudgetConfig{MaxCost: 50, WarnCost: 25}},
		},
		{
			name: "warn equals max is valid",
			cfg:  Config{Language: "go", Runner: "go-test", MaxParallel: 3, Budget: BudgetConfig{MaxCost: 25, WarnCost: 25}},
		},
		{
			name:    "negative max_cost",
			cfg:     Config{Language: "go", Runner: "go-test", MaxParallel: 3, Budget: BudgetConfig{MaxCost: -1}},
			wantErr: "budget.max_cost must be >= 0",
		},
		{
			name:    "negative warn_cost",
			cfg:     Config{Language: "go", Runner: "go-test", MaxParallel: 3, Budget: BudgetConfig{WarnCost: -5}},
			wantErr: "budget.warn_cost must be >= 0",
		},
		{
			name:    "warn_cost exceeds max_cost",
			cfg:     Config{Language: "go", Runner: "go-test", MaxParallel: 3, Budget: BudgetConfig{MaxCost: 10, WarnCost: 20}},
			wantErr: "budget.warn_cost",
		},
		{
			name: "warn_cost with zero max_cost is valid (max_cost=0 means unlimited)",
			cfg:  Config{Language: "go", Runner: "go-test", MaxParallel: 3, Budget: BudgetConfig{MaxCost: 0, WarnCost: 99}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want it to contain %q", err.Error(), tt.wantErr)
				}
			}
		})
	}
}

func TestValidateMaxParallel(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{
			name: "zero is valid (means not set)",
			cfg:  Config{Language: "go", Runner: "go-test", MaxParallel: 0},
		},
		{
			name: "positive value is valid",
			cfg:  Config{Language: "go", Runner: "go-test", MaxParallel: 5},
		},
		{
			name: "one is valid",
			cfg:  Config{Language: "go", Runner: "go-test", MaxParallel: 1},
		},
		{
			name:    "negative value is invalid",
			cfg:     Config{Language: "go", Runner: "go-test", MaxParallel: -1},
			wantErr: "max_parallel must be >= 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want it to contain %q", err.Error(), tt.wantErr)
				}
			}
		})
	}
}
