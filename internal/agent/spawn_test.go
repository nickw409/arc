package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var testBin string

func TestMain(m *testing.M) {
	// Build the mock agent binary for spawn tests.
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	testBin = filepath.Join(tmpDir, "mockagent")
	cmd := exec.Command("go", "build", "-o", testBin, "./testdata/mockagent")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build mock agent: %v\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func TestSpawnCapturesOutput(t *testing.T) {
	result, err := Spawn(context.Background(), SpawnOptions{
		Prompt:      "ignored",
		CommandName: testBin,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// With default MOCK_OUTPUT unset and MOCK_ECHO_STDIN not set, no output expected.
	// Use explicit MOCK_OUTPUT instead.
	t.Setenv("MOCK_OUTPUT", "test output\n")
	result, err = Spawn(context.Background(), SpawnOptions{
		Prompt:      "ignored",
		CommandName: testBin,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "test output\n" {
		t.Fatalf("got output %q, want %q", result.Output, "test output\n")
	}
	if result.ExitCode != 0 {
		t.Fatalf("got exit code %d, want 0", result.ExitCode)
	}
	if result.TimedOut {
		t.Fatal("expected TimedOut=false")
	}
}

func TestSpawnTimeout(t *testing.T) {
	t.Setenv("MOCK_SLEEP_MS", "5000")

	result, err := Spawn(context.Background(), SpawnOptions{
		Prompt:      "test",
		CommandName: testBin,
		Timeout:     100 * time.Millisecond,
	})
	// Per spec: timeout returns (*SpawnResult, nil) — error is nil
	if err != nil {
		t.Fatalf("expected nil error for timeout, got: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil SpawnResult for timeout")
	}
	if !result.TimedOut {
		t.Fatal("expected TimedOut=true")
	}
	if result.ExitCode != -1 {
		t.Fatalf("got exit code %d, want -1 for timeout", result.ExitCode)
	}
}

func TestSpawnContextCancel(t *testing.T) {
	t.Setenv("MOCK_SLEEP_MS", "5000")

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	result, err := Spawn(ctx, SpawnOptions{
		Prompt:      "test",
		CommandName: testBin,
		Timeout:     10 * time.Second,
	})
	// Per spec: parent context cancellation returns (nil, ctx.Err())
	if err == nil {
		t.Fatal("expected error for context cancellation")
	}
	if result != nil {
		t.Fatal("expected nil result for context cancellation")
	}
	if !strings.Contains(err.Error(), "cancel") {
		t.Fatalf("expected context cancellation error, got: %v", err)
	}
}

func TestSpawnNonzeroExit(t *testing.T) {
	t.Setenv("MOCK_EXIT_CODE", "1")
	t.Setenv("MOCK_OUTPUT", "error output")

	result, err := Spawn(context.Background(), SpawnOptions{
		Prompt:      "test",
		CommandName: testBin,
	})
	// Per spec: non-zero exit is NOT a Go error
	if err != nil {
		t.Fatalf("expected nil error for non-zero exit, got: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil SpawnResult")
	}
	if result.ExitCode != 1 {
		t.Fatalf("got exit code %d, want 1", result.ExitCode)
	}
	if result.TimedOut {
		t.Fatal("expected TimedOut=false")
	}
}

func TestSpawnDefaults(t *testing.T) {
	// Verify defaults: MaxTurns=15, Timeout=600s, AllowedTools=["View","Edit","Write","Bash"], OutputFormat="json"
	t.Setenv("MOCK_ECHO_ARGS", "1")

	result, err := Spawn(context.Background(), SpawnOptions{
		Prompt:      "test prompt",
		CommandName: testBin,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	args := result.Output

	// Check default max turns
	if !strings.Contains(args, "--max-turns") || !strings.Contains(args, "15") {
		t.Fatalf("expected default --max-turns 15 in args, got: %s", args)
	}
	// Check default output format
	if !strings.Contains(args, "--output-format") || !strings.Contains(args, "json") {
		t.Fatalf("expected default --output-format json in args, got: %s", args)
	}
	// Check default allowed tools
	if !strings.Contains(args, "--allowedTools") || !strings.Contains(args, "View,Edit,Write,Bash") {
		t.Fatalf("expected default --allowedTools View,Edit,Write,Bash in args, got: %s", args)
	}
	// Check --print flag is present
	if !strings.Contains(args, "--print") {
		t.Fatalf("expected --print in args, got: %s", args)
	}
	// Check --model is NOT present (no model override)
	if strings.Contains(args, "--model") {
		t.Fatalf("expected no --model in args for defaults, got: %s", args)
	}
}

func TestSpawnWithModel(t *testing.T) {
	t.Setenv("MOCK_ECHO_ARGS", "1")

	result, err := Spawn(context.Background(), SpawnOptions{
		Prompt:      "test",
		Model:       "sonnet",
		CommandName: testBin,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("got exit code %d, want 0", result.ExitCode)
	}
	// Verify args include --model sonnet as two separate arguments
	argLines := strings.Split(result.Output, "\n")
	foundModel := false
	for i, arg := range argLines {
		if arg == "--model" && i+1 < len(argLines) && argLines[i+1] == "sonnet" {
			foundModel = true
			break
		}
	}
	if !foundModel {
		t.Fatalf("expected args to contain '--model' 'sonnet', got args: %v", argLines)
	}
}

func TestSpawnWithAllowedTools(t *testing.T) {
	t.Setenv("MOCK_ECHO_ARGS", "1")

	result, err := Spawn(context.Background(), SpawnOptions{
		Prompt:       "test",
		AllowedTools: []string{"Read", "Glob"},
		CommandName:  testBin,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("got exit code %d, want 0", result.ExitCode)
	}
	// Verify args include --allowedTools as separate arg from comma-joined tools
	argLines := strings.Split(result.Output, "\n")
	foundTools := false
	for i, arg := range argLines {
		if arg == "--allowedTools" && i+1 < len(argLines) && argLines[i+1] == "Read,Glob" {
			foundTools = true
			break
		}
	}
	if !foundTools {
		t.Fatalf("expected args to contain '--allowedTools' 'Read,Glob', got args: %v", argLines)
	}
	// Verify it does NOT contain the defaults
	for _, arg := range argLines {
		if arg == "View,Edit,Write,Bash" {
			t.Fatalf("expected custom tools, not defaults; got args: %v", argLines)
		}
	}
}

func TestSpawnJSONOutput(t *testing.T) {
	t.Setenv("MOCK_ECHO_ARGS", "1")

	result, err := Spawn(context.Background(), SpawnOptions{
		Prompt:       "test",
		OutputFormat: "json",
		CommandName:  testBin,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("got exit code %d, want 0", result.ExitCode)
	}
	// Verify args include --output-format json as two separate arguments
	argLines := strings.Split(result.Output, "\n")
	foundFormat := false
	for i, arg := range argLines {
		if arg == "--output-format" && i+1 < len(argLines) && argLines[i+1] == "json" {
			foundFormat = true
			break
		}
	}
	if !foundFormat {
		t.Fatalf("expected args to contain '--output-format' 'json', got args: %v", argLines)
	}
}

func TestSpawnCommandNotFound(t *testing.T) {
	result, err := Spawn(context.Background(), SpawnOptions{
		CommandName: "nonexistent-binary-12345",
		Prompt:      "test",
	})
	if err == nil {
		t.Fatal("expected error for command not found")
	}
	if result != nil {
		t.Fatal("expected nil result for command not found")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "not found") && !strings.Contains(errStr, "executable file not found") {
		t.Fatalf("expected 'not found' error, got: %v", err)
	}
}

func TestSpawnStdinDelivery(t *testing.T) {
	t.Setenv("MOCK_ECHO_STDIN", "1")

	result, err := Spawn(context.Background(), SpawnOptions{
		Prompt:      "hello world",
		CommandName: testBin,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Output, "hello world") {
		t.Fatalf("expected output to contain %q, got %q", "hello world", result.Output)
	}
}

func TestSpawnStderrDiscarded(t *testing.T) {
	t.Setenv("MOCK_OUTPUT", "stdout data\n")
	t.Setenv("MOCK_STDERR", "stderr noise")

	result, err := Spawn(context.Background(), SpawnOptions{
		Prompt:      "test",
		CommandName: testBin,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "stdout data\n" {
		t.Fatalf("got output %q, want %q", result.Output, "stdout data\n")
	}
	if strings.Contains(result.Output, "stderr") {
		t.Fatal("stderr content should not appear in Output")
	}
}

func TestSpawnJSONParsing(t *testing.T) {
	t.Setenv("MOCK_OUTPUT", "hello from agent")
	t.Setenv("MOCK_JSON_WRAP", "1")

	result, err := Spawn(context.Background(), SpawnOptions{
		Prompt:      "test",
		CommandName: testBin,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("got exit code %d, want 0", result.ExitCode)
	}
	// Output should be the unwrapped result text, not the JSON envelope
	if result.Output != "hello from agent" {
		t.Fatalf("got output %q, want %q", result.Output, "hello from agent")
	}
	// Usage should be populated from the JSON envelope
	if result.Usage.InputTokens != 10 {
		t.Fatalf("got InputTokens=%d, want 10", result.Usage.InputTokens)
	}
	if result.Usage.OutputTokens != 5 {
		t.Fatalf("got OutputTokens=%d, want 5", result.Usage.OutputTokens)
	}
	if result.Usage.CacheCreationInputTokens != 2 {
		t.Fatalf("got CacheCreationInputTokens=%d, want 2", result.Usage.CacheCreationInputTokens)
	}
	if result.Usage.CacheReadInputTokens != 3 {
		t.Fatalf("got CacheReadInputTokens=%d, want 3", result.Usage.CacheReadInputTokens)
	}
	if result.Usage.CostUSD != 0.001 {
		t.Fatalf("got CostUSD=%f, want 0.001", result.Usage.CostUSD)
	}
}

func TestSpawnPlainTextFallback(t *testing.T) {
	// When output is plain text (not JSON), it should pass through unmodified
	// with zero usage.
	t.Setenv("MOCK_OUTPUT", "plain text output\n")

	result, err := Spawn(context.Background(), SpawnOptions{
		Prompt:      "test",
		CommandName: testBin,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "plain text output\n" {
		t.Fatalf("got output %q, want %q", result.Output, "plain text output\n")
	}
	if !result.Usage.IsZero() {
		t.Fatalf("expected zero usage for plain text, got %+v", result.Usage)
	}
}
