package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunnerParseJSONResult(t *testing.T) {
	input := `{"total": 5, "passed": 3, "failed": 2, "raw_output": "output", "failed_names": ["test_a", "test_b"]}`
	var result TestResult
	if err := json.Unmarshal([]byte(input), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if result.Total != 5 {
		t.Fatalf("got Total=%d, want 5", result.Total)
	}
	if result.Passed != 3 {
		t.Fatalf("got Passed=%d, want 3", result.Passed)
	}
	if result.Failed != 2 {
		t.Fatalf("got Failed=%d, want 2", result.Failed)
	}
	if result.RawOutput != "output" {
		t.Fatalf("got RawOutput=%q, want %q", result.RawOutput, "output")
	}
	if len(result.FailedNames) != 2 || result.FailedNames[0] != "test_a" || result.FailedNames[1] != "test_b" {
		t.Fatalf("got FailedNames=%v, want [test_a test_b]", result.FailedNames)
	}
}

func TestRunnerNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := Run(context.Background(), RunOptions{
		Runner:  "nonexistent-runner",
		ArcHome: tmpDir,
	})
	if err == nil {
		t.Fatal("expected error for nonexistent runner")
	}
	errStr := strings.ToLower(err.Error())
	if !strings.Contains(errStr, "runner") && !strings.Contains(errStr, "not found") {
		t.Fatalf("expected error containing 'runner' or 'not found', got: %v", err)
	}
}

func TestRunAllAggregates(t *testing.T) {
	// Create a mock runner that outputs different JSON for different test files.
	tmpDir := t.TempDir()
	runnerDir := filepath.Join(tmpDir, "runners", "mock-runner")
	if err := os.MkdirAll(runnerDir, 0755); err != nil {
		t.Fatalf("failed to create runner dir: %v", err)
	}

	// Create a runner script that returns JSON based on the test file argument.
	script := `#!/bin/bash
TEST_FILE="$1"
if [[ "$TEST_FILE" == *"file1"* ]]; then
  echo '{"total": 3, "passed": 3, "failed": 0, "raw_output": "run1 output", "failed_names": []}'
elif [[ "$TEST_FILE" == *"file2"* ]]; then
  echo '{"total": 2, "passed": 1, "failed": 1, "raw_output": "run2 output", "failed_names": ["t1"]}'
fi
`
	scriptPath := filepath.Join(runnerDir, "run.sh")
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to write runner script: %v", err)
	}

	result, err := RunAll(context.Background(), "mock-runner", []string{"file1_test.go", "file2_test.go"}, 30*time.Second, tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 5 {
		t.Fatalf("got Total=%d, want 5", result.Total)
	}
	if result.Passed != 4 {
		t.Fatalf("got Passed=%d, want 4", result.Passed)
	}
	if result.Failed != 1 {
		t.Fatalf("got Failed=%d, want 1", result.Failed)
	}
	if len(result.FailedNames) != 1 || result.FailedNames[0] != "t1" {
		t.Fatalf("got FailedNames=%v, want [t1]", result.FailedNames)
	}
}

func TestRunAllEmptyFiles(t *testing.T) {
	result, err := RunAll(context.Background(), "go-test", []string{}, 30*time.Second, "/tmp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 0 {
		t.Fatalf("got Total=%d, want 0", result.Total)
	}
	if result.Passed != 0 {
		t.Fatalf("got Passed=%d, want 0", result.Passed)
	}
	if result.Failed != 0 {
		t.Fatalf("got Failed=%d, want 0", result.Failed)
	}
	if result.FailedNames == nil {
		t.Fatal("FailedNames should be []string{}, not nil")
	}
	if len(result.FailedNames) != 0 {
		t.Fatalf("got FailedNames=%v, want empty", result.FailedNames)
	}
}

func TestRunJSONParseFailure(t *testing.T) {
	tmpDir := t.TempDir()
	runnerDir := filepath.Join(tmpDir, "runners", "bad-json-runner")
	if err := os.MkdirAll(runnerDir, 0755); err != nil {
		t.Fatalf("failed to create runner dir: %v", err)
	}

	script := "#!/bin/bash\necho 'not valid json'\n"
	scriptPath := filepath.Join(runnerDir, "run.sh")
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to write runner script: %v", err)
	}

	_, err := Run(context.Background(), RunOptions{
		Runner:   "bad-json-runner",
		TestFile: "test.go",
		ArcHome:  tmpDir,
	})
	if err == nil {
		t.Fatal("expected error for unparseable JSON")
	}
	errStr := strings.ToLower(err.Error())
	if !strings.Contains(errStr, "json") && !strings.Contains(errStr, "parse") && !strings.Contains(errStr, "unmarshal") {
		t.Fatalf("expected error containing 'json' or 'parse', got: %v", err)
	}
}

func TestRunScriptNotExecutable(t *testing.T) {
	tmpDir := t.TempDir()
	runnerDir := filepath.Join(tmpDir, "runners", "noexec-runner")
	if err := os.MkdirAll(runnerDir, 0755); err != nil {
		t.Fatalf("failed to create runner dir: %v", err)
	}

	script := "#!/bin/bash\necho '{}'\n"
	scriptPath := filepath.Join(runnerDir, "run.sh")
	// Write with 0644 — not executable
	if err := os.WriteFile(scriptPath, []byte(script), 0644); err != nil {
		t.Fatalf("failed to write runner script: %v", err)
	}

	_, err := Run(context.Background(), RunOptions{
		Runner:   "noexec-runner",
		TestFile: "test.go",
		ArcHome:  tmpDir,
	})
	if err == nil {
		t.Fatal("expected error for non-executable runner script")
	}
	errStr := strings.ToLower(err.Error())
	if !strings.Contains(errStr, "permission") && !strings.Contains(errStr, "not executable") {
		t.Fatalf("expected error containing 'permission' or 'not executable', got: %v", err)
	}
}

func TestRunAllTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	runnerDir := filepath.Join(tmpDir, "runners", "slow-runner")
	if err := os.MkdirAll(runnerDir, 0755); err != nil {
		t.Fatalf("failed to create runner dir: %v", err)
	}

	script := "#!/bin/bash\nsleep 5\necho '{}'\n"
	scriptPath := filepath.Join(runnerDir, "run.sh")
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to write runner script: %v", err)
	}

	_, err := RunAll(context.Background(), "slow-runner", []string{"test.go"}, 100*time.Millisecond, tmpDir)
	if err == nil {
		t.Fatal("expected error for timeout")
	}
}

func TestRunAllErrorPropagates(t *testing.T) {
	tmpDir := t.TempDir()
	runnerDir := filepath.Join(tmpDir, "runners", "fail-second-runner")
	if err := os.MkdirAll(runnerDir, 0755); err != nil {
		t.Fatalf("failed to create runner dir: %v", err)
	}

	// Script succeeds for "file1", fails (non-JSON output) for "file2"
	script := `#!/bin/bash
TEST_FILE="$1"
if [[ "$TEST_FILE" == *"file1"* ]]; then
  echo '{"total": 3, "passed": 3, "failed": 0, "raw_output": "ok", "failed_names": []}'
else
  echo 'SCRIPT ERROR' >&2
  exit 1
fi
`
	scriptPath := filepath.Join(runnerDir, "run.sh")
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to write runner script: %v", err)
	}

	_, err := RunAll(context.Background(), "fail-second-runner", []string{"file1_test.go", "file2_test.go"}, 30*time.Second, tmpDir)
	if err == nil {
		t.Fatal("expected error to propagate from failing run")
	}
}

func TestRunAllPartialResultsOnError(t *testing.T) {
	tmpDir := t.TempDir()
	runnerDir := filepath.Join(tmpDir, "runners", "partial-runner")
	if err := os.MkdirAll(runnerDir, 0755); err != nil {
		t.Fatalf("failed to create runner dir: %v", err)
	}

	script := `#!/bin/bash
TEST_FILE="$1"
if [[ "$TEST_FILE" == *"file1"* ]]; then
  echo '{"total": 3, "passed": 3, "failed": 0, "raw_output": "ok", "failed_names": []}'
else
  echo 'SCRIPT ERROR' >&2
  exit 1
fi
`
	scriptPath := filepath.Join(runnerDir, "run.sh")
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to write runner script: %v", err)
	}

	result, err := RunAll(context.Background(), "partial-runner", []string{"file1_test.go", "file2_test.go"}, 30*time.Second, tmpDir)
	if err == nil {
		t.Fatal("expected error from second run")
	}
	// Should return partial aggregation from first run
	if result == nil {
		t.Fatal("expected partial results, got nil")
	}
	if result.Total != 3 {
		t.Fatalf("got partial Total=%d, want 3", result.Total)
	}
	if result.Passed != 3 {
		t.Fatalf("got partial Passed=%d, want 3", result.Passed)
	}
}

func TestRunTimeoutIndividual(t *testing.T) {
	tmpDir := t.TempDir()
	runnerDir := filepath.Join(tmpDir, "runners", "slow-individual-runner")
	if err := os.MkdirAll(runnerDir, 0755); err != nil {
		t.Fatalf("failed to create runner dir: %v", err)
	}

	script := "#!/bin/bash\nsleep 5\necho '{}'\n"
	scriptPath := filepath.Join(runnerDir, "run.sh")
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to write runner script: %v", err)
	}

	_, err := Run(context.Background(), RunOptions{
		Runner:   "slow-individual-runner",
		TestFile: "test.go",
		Timeout:  100 * time.Millisecond,
		ArcHome:  tmpDir,
	})
	if err == nil {
		t.Fatal("expected error for timeout")
	}
}

func TestRunAllRawOutputAggregation(t *testing.T) {
	tmpDir := t.TempDir()
	runnerDir := filepath.Join(tmpDir, "runners", "output-runner")
	if err := os.MkdirAll(runnerDir, 0755); err != nil {
		t.Fatalf("failed to create runner dir: %v", err)
	}

	script := `#!/bin/bash
TEST_FILE="$1"
if [[ "$TEST_FILE" == *"file1"* ]]; then
  echo '{"total": 2, "passed": 2, "failed": 0, "raw_output": "run1 output", "failed_names": []}'
elif [[ "$TEST_FILE" == *"file2"* ]]; then
  echo '{"total": 1, "passed": 1, "failed": 0, "raw_output": "run2 output", "failed_names": []}'
fi
`
	scriptPath := filepath.Join(runnerDir, "run.sh")
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to write runner script: %v", err)
	}

	result, err := RunAll(context.Background(), "output-runner", []string{"file1_test.go", "file2_test.go"}, 30*time.Second, tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.RawOutput, "run1 output") {
		t.Fatalf("expected RawOutput to contain 'run1 output', got %q", result.RawOutput)
	}
	if !strings.Contains(result.RawOutput, "run2 output") {
		t.Fatalf("expected RawOutput to contain 'run2 output', got %q", result.RawOutput)
	}
	// Check they are separated by newline
	expected := "run1 output\nrun2 output"
	if result.RawOutput != expected {
		t.Fatalf("got RawOutput=%q, want %q", result.RawOutput, expected)
	}
}
