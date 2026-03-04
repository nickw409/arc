package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nwiley/arc/internal/runner"
)

func TestTestCmdRequiresArg(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"test"})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
}

func TestTestCmdUnknownFile(t *testing.T) {
	// Test with a file extension that has no runner — we can't os.Exit(2) in test,
	// so we verify the detectRunner function returns empty for unknown extensions.
	r := detectRunner("unknown.xyz")
	if r != "" {
		t.Fatalf("expected empty runner for unknown.xyz, got %q", r)
	}
}

func TestTestCmdSuccessPath(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH")
	}

	dir := t.TempDir()

	// Create go.mod
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module testmod\n\ngo 1.21\n"), 0644)

	// Create .arc.yaml
	os.WriteFile(filepath.Join(dir, ".arc.yaml"), []byte("language: go\nrunner: go-test\n"), 0644)

	// Create passing test
	testCode := `package testmod

import "testing"

func TestPass(t *testing.T) {}
`
	os.WriteFile(filepath.Join(dir, "pass_test.go"), []byte(testCode), 0644)

	// Change to temp dir
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	cmd := newTestCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"pass_test.go"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v\nOutput: %s", err, buf.String())
	}

	output := buf.String()
	if !strings.Contains(output, "passed") {
		t.Fatalf("expected summary with 'passed', got %q", output)
	}
}

func TestTestCmdJsonFlag(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH")
	}

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module testmod\n\ngo 1.21\n"), 0644)
	os.WriteFile(filepath.Join(dir, ".arc.yaml"), []byte("language: go\nrunner: go-test\n"), 0644)

	testCode := `package testmod

import "testing"

func TestJSON(t *testing.T) {}
`
	os.WriteFile(filepath.Join(dir, "json_test.go"), []byte(testCode), 0644)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	cmd := newTestCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--json", "json_test.go"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result runner.TestResult
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("expected valid JSON, got parse error: %v\nOutput: %s", err, buf.String())
	}
	if result.Passed < 1 {
		t.Fatalf("expected at least 1 pass, got %d", result.Passed)
	}
}

func TestTestCmdFilterFlag(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH")
	}

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module testmod\n\ngo 1.21\n"), 0644)
	os.WriteFile(filepath.Join(dir, ".arc.yaml"), []byte("language: go\nrunner: go-test\n"), 0644)

	testCode := `package testmod

import "testing"

func TestSpecific(t *testing.T) {}
func TestOther(t *testing.T) {}
`
	os.WriteFile(filepath.Join(dir, "filter_test.go"), []byte(testCode), 0644)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	cmd := newTestCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--filter", "TestSpecific", "filter_test.go"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v\nOutput: %s", err, buf.String())
	}

	output := buf.String()
	if !strings.Contains(output, "1/1 passed") {
		t.Fatalf("expected 1/1 passed with filter, got %q", output)
	}
}

func TestTestCmdWithoutArcYaml(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH")
	}

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module testmod\n\ngo 1.21\n"), 0644)

	testCode := `package testmod

import "testing"

func TestFallback(t *testing.T) {}
`
	os.WriteFile(filepath.Join(dir, "fallback_test.go"), []byte(testCode), 0644)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	cmd := newTestCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"fallback_test.go"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v\nOutput: %s", err, buf.String())
	}

	output := buf.String()
	if !strings.Contains(output, "passed") {
		t.Fatalf("expected summary with 'passed', got %q", output)
	}
}

func TestTestCmdLanguageFallback(t *testing.T) {
	// For non-Go files without .arc.yaml, detectRunner returns empty
	r := detectRunner("test_auth.py")
	if r != "" {
		t.Fatalf("expected empty runner for .py file (not yet implemented), got %q", r)
	}
}

func TestDetectRunnerGoTest(t *testing.T) {
	if r := detectRunner("foo_test.go"); r != "go-test" {
		t.Fatalf("expected go-test, got %q", r)
	}
}

func TestDetectRunnerUnknown(t *testing.T) {
	if r := detectRunner("foo.rs"); r != "" {
		t.Fatalf("expected empty, got %q", r)
	}
}
