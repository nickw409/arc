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
