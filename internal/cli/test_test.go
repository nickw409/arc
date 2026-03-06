package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nwiley/arc/internal/testcmd"
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

func TestTestCmdSuccessPath(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH")
	}

	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module testmod\n\ngo 1.21\n"), 0644)
	os.WriteFile(filepath.Join(dir, ".arc.yaml"), []byte("language: go\nrunner: go-test\n"), 0644)

	testCode := `package testmod

import "testing"

func TestPass(t *testing.T) {}
`
	os.WriteFile(filepath.Join(dir, "pass_test.go"), []byte(testCode), 0644)

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
	if !strings.Contains(output, "PASS") {
		t.Fatalf("expected PASS in output, got %q", output)
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

	var result testcmd.Result
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("expected valid JSON, got parse error: %v\nOutput: %s", err, buf.String())
	}
	if !result.Passed {
		t.Fatal("expected Passed=true")
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
	if !strings.Contains(output, "PASS") {
		t.Fatalf("expected PASS in output, got %q", output)
	}
}
