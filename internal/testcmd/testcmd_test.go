package testcmd

import (
	"context"
	"runtime"
	"testing"

	"github.com/nwiley/arc/internal/config"
)

func TestNewEnv_Override(t *testing.T) {
	env := NewEnv(WithOverride("make test"))
	if env.Command != "make test" {
		t.Errorf("Command = %q, want %q", env.Command, "make test")
	}
}

func TestNewEnv_Config(t *testing.T) {
	cfg := &config.Config{
		TestCommand: "npm test",
		Language:    "typescript",
		Runner:      "vitest",
	}
	env := NewEnv(WithConfig(cfg))
	if env.Command != "npm test" {
		t.Errorf("Command = %q, want %q", env.Command, "npm test")
	}
	if env.Language != "typescript" {
		t.Errorf("Language = %q, want %q", env.Language, "typescript")
	}
}

func TestNewEnv_OverrideTrumpsConfig(t *testing.T) {
	cfg := &config.Config{TestCommand: "npm test"}
	env := NewEnv(WithConfig(cfg), WithOverride("cargo test"))
	if env.Command != "cargo test" {
		t.Errorf("Command = %q, want %q", env.Command, "cargo test")
	}
}

func TestNewEnv_DetectionFallback(t *testing.T) {
	// With no config, no override, and a project dir that has go.mod
	// (this project itself), detection should find "go test ./..."
	env := NewEnv(WithProjectDir("."))
	if env.Command == "" {
		t.Error("Command should not be empty")
	}
}

func TestNewEnv_Fallback(t *testing.T) {
	// No config, no override, non-existent project dir → fallback
	env := NewEnv(WithProjectDir("/nonexistent/path"))
	if env.Command != "go test ./..." {
		t.Errorf("Command = %q, want fallback %q", env.Command, "go test ./...")
	}
}

func TestCommandForFile_Go(t *testing.T) {
	env := &Env{Language: "go"}
	cmd := env.CommandForFile("internal/foo/bar_test.go")
	if cmd != "go test ./internal/foo/ -v -count=1" {
		t.Errorf("got %q", cmd)
	}
}

func TestCommandForFile_GoRoot(t *testing.T) {
	env := &Env{Language: "go"}
	cmd := env.CommandForFile("main_test.go")
	if cmd != "go test ./ -v -count=1" {
		t.Errorf("got %q", cmd)
	}
}

func TestCommandForFile_Python(t *testing.T) {
	env := &Env{Language: "python"}
	cmd := env.CommandForFile("tests/test_auth.py")
	if cmd != "pytest tests/test_auth.py -v" {
		t.Errorf("got %q", cmd)
	}
}

func TestCommandForFile_Typescript(t *testing.T) {
	env := &Env{Language: "typescript"}
	cmd := env.CommandForFile("src/auth.test.ts")
	if cmd != "npx vitest run src/auth.test.ts" {
		t.Errorf("got %q", cmd)
	}
}

func TestCommandForFile_Rust(t *testing.T) {
	env := &Env{Language: "rust"}
	cmd := env.CommandForFile("tests/integration.rs")
	if cmd != "cargo test --test integration" {
		t.Errorf("got %q", cmd)
	}
}

func TestCommandForFile_UnknownFallback(t *testing.T) {
	env := &Env{Language: "", Command: "make test"}
	cmd := env.CommandForFile("tests/unknown.xyz")
	if cmd != "make test" {
		t.Errorf("got %q, want fallback to full command", cmd)
	}
}

func TestCommandForFile_DetectsLanguageFromExtension(t *testing.T) {
	env := &Env{Language: ""} // no language set
	cmd := env.CommandForFile("internal/foo/bar_test.go")
	if cmd != "go test ./internal/foo/ -v -count=1" {
		t.Errorf("got %q", cmd)
	}
}

func TestCommandForPackage_Go(t *testing.T) {
	env := &Env{Language: "go"}
	cmd := env.commandForPackage("internal/foo")
	if cmd != "go test ./internal/foo/ -count=1" {
		t.Errorf("got %q", cmd)
	}
}

func TestCommandForPackage_Python(t *testing.T) {
	env := &Env{Language: "python"}
	cmd := env.commandForPackage("tests/unit")
	if cmd != "pytest tests/unit -v" {
		t.Errorf("got %q", cmd)
	}
}

func TestParseFailures(t *testing.T) {
	output := `=== RUN   TestFoo
--- PASS: TestFoo (0.01s)
=== RUN   TestBar
--- FAIL: TestBar (0.02s)
=== RUN   TestBaz/sub1
--- FAIL: TestBaz/sub1 (0.01s)
--- FAIL: TestBaz (0.01s)
FAIL
`
	failures := ParseFailures(output)
	// TestBaz should be deduplicated (TestBaz/sub1 is more specific)
	if len(failures) != 2 {
		t.Fatalf("got %d failures, want 2: %v", len(failures), failures)
	}
	want := map[string]bool{"TestBar": true, "TestBaz/sub1": true}
	for _, f := range failures {
		if !want[f] {
			t.Errorf("unexpected failure: %q", f)
		}
	}
}

func TestParseFailures_Empty(t *testing.T) {
	failures := ParseFailures("ok  \tgithub.com/foo\t0.5s\n")
	if len(failures) != 0 {
		t.Errorf("expected 0 failures, got %v", failures)
	}
}

func TestRunAll(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh -c not available on windows")
	}
	env := &Env{Command: "true", Dir: t.TempDir()}
	result, err := env.RunAll(context.Background())
	if err != nil {
		t.Fatalf("RunAll error: %v", err)
	}
	if !result.Passed {
		t.Error("expected Passed=true for 'true' command")
	}
}

func TestRunAll_Failure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh -c not available on windows")
	}
	env := &Env{Command: "false", Dir: t.TempDir()}
	result, err := env.RunAll(context.Background())
	if err != nil {
		t.Fatalf("RunAll error: %v", err)
	}
	if result.Passed {
		t.Error("expected Passed=false for 'false' command")
	}
	if result.ExitCode == 0 {
		t.Error("expected non-zero exit code")
	}
}

func TestRunCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh -c not available on windows")
	}
	env := &Env{Dir: t.TempDir()}
	result, err := env.RunCommand(context.Background(), "echo hello")
	if err != nil {
		t.Fatalf("RunCommand error: %v", err)
	}
	if !result.Passed {
		t.Error("expected Passed=true")
	}
	if result.Output == "" {
		t.Error("expected non-empty output")
	}
}

func TestDetectLanguageFromFile(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"foo_test.go", "go"},
		{"foo.go", "go"},
		{"lib.rs", "rust"},
		{"app.test.ts", "typescript"},
		{"app.spec.js", "typescript"},
		{"test_auth.py", "python"},
		{"unknown.txt", ""},
	}
	for _, tt := range tests {
		got := detectLanguageFromFile(tt.path)
		if got != tt.want {
			t.Errorf("detectLanguageFromFile(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}
