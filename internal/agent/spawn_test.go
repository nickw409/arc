package agent

import (
	"context"
	"encoding/json"
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

// --- Existing tests (buffered path) ---

func TestSpawnCapturesOutput(t *testing.T) {
	result, err := Spawn(context.Background(), SpawnOptions{
		Prompt:       "ignored",
		CommandName:  testBin,
		OutputFormat: "json",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// With default MOCK_OUTPUT unset and MOCK_ECHO_STDIN not set, no output expected.
	// Use explicit MOCK_OUTPUT instead.
	t.Setenv("MOCK_OUTPUT", "test output\n")
	result, err = Spawn(context.Background(), SpawnOptions{
		Prompt:       "ignored",
		CommandName:  testBin,
		OutputFormat: "json",
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
	t.Setenv("MOCK_ECHO_ARGS", "1")

	result, err := Spawn(context.Background(), SpawnOptions{
		Prompt:      "test prompt",
		CommandName: testBin,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	args := result.Output

	if !strings.Contains(args, "--max-turns") || !strings.Contains(args, "15") {
		t.Fatalf("expected default --max-turns 15 in args, got: %s", args)
	}
	if !strings.Contains(args, "--output-format") || !strings.Contains(args, "stream-json") {
		t.Fatalf("expected default --output-format stream-json in args, got: %s", args)
	}
	if !strings.Contains(args, "--verbose") {
		t.Fatalf("expected --verbose in args for stream-json, got: %s", args)
	}
	if !strings.Contains(args, "--allowedTools") || !strings.Contains(args, "View,Edit,Write,Bash") {
		t.Fatalf("expected default --allowedTools View,Edit,Write,Bash in args, got: %s", args)
	}
	if !strings.Contains(args, "--print") {
		t.Fatalf("expected --print in args, got: %s", args)
	}
	if strings.Contains(args, "--model") {
		t.Fatalf("expected no --model in args for defaults, got: %s", args)
	}
}

func TestSpawnWithModel(t *testing.T) {
	t.Setenv("MOCK_ECHO_ARGS", "1")

	result, err := Spawn(context.Background(), SpawnOptions{
		Prompt:       "test",
		Model:        "sonnet",
		CommandName:  testBin,
		OutputFormat: "json",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("got exit code %d, want 0", result.ExitCode)
	}
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
		OutputFormat: "json",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("got exit code %d, want 0", result.ExitCode)
	}
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
		Prompt:       "hello world",
		CommandName:  testBin,
		OutputFormat: "json",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Output, "hello world") {
		t.Fatalf("expected output to contain %q, got %q", "hello world", result.Output)
	}
}

func TestSpawnCapturesStderr(t *testing.T) {
	t.Setenv("MOCK_OUTPUT", "ok")
	t.Setenv("MOCK_STDERR", "warning: something happened")

	result, err := Spawn(context.Background(), SpawnOptions{
		Prompt:      "test",
		CommandName: testBin,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Stderr != "warning: something happened" {
		t.Fatalf("got Stderr %q, want %q", result.Stderr, "warning: something happened")
	}
	if !strings.Contains(result.Output, "ok") {
		t.Fatalf("got Output %q, want to contain %q", result.Output, "ok")
	}
}

func TestSpawnStderrEmptyOnSuccess(t *testing.T) {
	t.Setenv("MOCK_OUTPUT", "ok")

	result, err := Spawn(context.Background(), SpawnOptions{
		Prompt:      "test",
		CommandName: testBin,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Stderr != "" {
		t.Fatalf("got Stderr %q, want empty", result.Stderr)
	}
}

func TestSpawnResultHasDuration(t *testing.T) {
	t.Setenv("MOCK_SLEEP_MS", "100")
	t.Setenv("MOCK_OUTPUT", "ok")

	result, err := Spawn(context.Background(), SpawnOptions{
		Prompt:      "test",
		CommandName: testBin,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Duration < 100*time.Millisecond {
		t.Fatalf("got Duration %v, want >= 100ms", result.Duration)
	}
}

func TestSpawnResultHasPIDAndZeroDuration(t *testing.T) {
	t.Setenv("MOCK_OUTPUT", "ok")

	result, err := Spawn(context.Background(), SpawnOptions{
		Prompt:      "test",
		CommandName: testBin,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.PID <= 0 {
		t.Fatalf("got PID %d, want > 0", result.PID)
	}
	if result.Duration < 0 {
		t.Fatalf("got Duration %v, want >= 0", result.Duration)
	}
}

func TestSpawnStderrOnNonZeroExit(t *testing.T) {
	t.Setenv("MOCK_EXIT_CODE", "1")
	t.Setenv("MOCK_STDERR", "error: details")

	result, err := Spawn(context.Background(), SpawnOptions{
		Prompt:      "test",
		CommandName: testBin,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Fatalf("got ExitCode %d, want 1", result.ExitCode)
	}
	if !strings.Contains(result.Stderr, "error: details") {
		t.Fatalf("got Stderr %q, want to contain %q", result.Stderr, "error: details")
	}
}

func TestSpawnStderrOnTimeout(t *testing.T) {
	t.Setenv("MOCK_SLEEP_MS", "5000")
	t.Setenv("MOCK_STDERR", "partial")

	result, err := Spawn(context.Background(), SpawnOptions{
		Prompt:      "test",
		CommandName: testBin,
		Timeout:     1 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.TimedOut {
		t.Fatal("expected TimedOut=true")
	}
	if result.PID <= 0 {
		t.Fatalf("got PID %d, want > 0", result.PID)
	}
	if result.Duration < 1*time.Second {
		t.Fatalf("got Duration %v, want >= 1s", result.Duration)
	}
}

func TestSpawnContextCancellation(t *testing.T) {
	t.Setenv("MOCK_SLEEP_MS", "5000")

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	result, err := Spawn(ctx, SpawnOptions{
		Prompt:      "test",
		CommandName: testBin,
		Timeout:     10 * time.Second,
	})
	if result != nil {
		t.Fatal("expected nil result for context cancellation")
	}
	if err == nil {
		t.Fatal("expected error for context cancellation")
	}
}

func TestSpawnLongStderr(t *testing.T) {
	longStderr := strings.Repeat("x", 600)
	t.Setenv("MOCK_STDERR", longStderr)
	t.Setenv("MOCK_OUTPUT", "ok")

	result, err := Spawn(context.Background(), SpawnOptions{
		Prompt:      "test",
		CommandName: testBin,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Stderr) != 600 {
		t.Fatalf("got Stderr length %d, want 600 (no truncation in Spawn)", len(result.Stderr))
	}
}

func TestSpawnStderrWithNewlines(t *testing.T) {
	t.Setenv("MOCK_STDERR", "line1\nline2\nline3")
	t.Setenv("MOCK_OUTPUT", "ok")

	result, err := Spawn(context.Background(), SpawnOptions{
		Prompt:      "test",
		CommandName: testBin,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Stderr, "line1\nline2\nline3") {
		t.Fatalf("got Stderr %q, want to contain newlines", result.Stderr)
	}
}

func TestSpawnStderrNotInOutput(t *testing.T) {
	t.Setenv("MOCK_OUTPUT", "stdout data\n")
	t.Setenv("MOCK_STDERR", "stderr noise")

	result, err := Spawn(context.Background(), SpawnOptions{
		Prompt:       "test",
		CommandName:  testBin,
		OutputFormat: "json",
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
	if result.Stderr != "stderr noise" {
		t.Fatalf("got Stderr %q, want %q", result.Stderr, "stderr noise")
	}
}

func TestSpawnJSONParsing(t *testing.T) {
	t.Setenv("MOCK_OUTPUT", "hello from agent")
	t.Setenv("MOCK_JSON_WRAP", "1")

	result, err := Spawn(context.Background(), SpawnOptions{
		Prompt:       "test",
		CommandName:  testBin,
		OutputFormat: "json",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("got exit code %d, want 0", result.ExitCode)
	}
	if result.Output != "hello from agent" {
		t.Fatalf("got output %q, want %q", result.Output, "hello from agent")
	}
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
	t.Setenv("MOCK_OUTPUT", "plain text output\n")

	result, err := Spawn(context.Background(), SpawnOptions{
		Prompt:       "test",
		CommandName:  testBin,
		OutputFormat: "json",
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

// --- Stream-json helpers ---

func mustJSON(v interface{}) string {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(data)
}

var streamInitLine = mustJSON(map[string]interface{}{
	"type":       "system",
	"subtype":    "init",
	"session_id": "test-session",
	"tools":      []string{},
	"model":      "claude-sonnet-4-20250514",
})

func makeAssistantLine(inputTokens, outputTokens int, tools ...string) string {
	content := []map[string]string{{"type": "text", "name": ""}}
	for _, t := range tools {
		content = append(content, map[string]string{"type": "tool_use", "name": t})
	}
	return mustJSON(map[string]interface{}{
		"type": "assistant",
		"message": map[string]interface{}{
			"content": content,
			"usage": map[string]int{
				"input_tokens":                inputTokens,
				"output_tokens":               outputTokens,
				"cache_creation_input_tokens":  0,
				"cache_read_input_tokens":      0,
			},
		},
	})
}

func makeResultLine(result string, cost float64, inputTokens, outputTokens, numTurns int) string {
	return mustJSON(map[string]interface{}{
		"type":           "result",
		"subtype":        "success",
		"is_error":       false,
		"result":         result,
		"total_cost_usd": cost,
		"num_turns":      numTurns,
		"usage": map[string]int{
			"input_tokens":                inputTokens,
			"output_tokens":               outputTokens,
			"cache_creation_input_tokens":  0,
			"cache_read_input_tokens":      0,
		},
	})
}

// --- Stream-json spawn tests ---

func TestStreamFallbackToBuffered(t *testing.T) {
	t.Setenv("MOCK_OUTPUT", "hello from agent")
	t.Setenv("MOCK_JSON_WRAP", "1")

	result, err := Spawn(context.Background(), SpawnOptions{
		Prompt:       "test",
		CommandName:  testBin,
		OutputFormat: "json",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "hello from agent" {
		t.Fatalf("got output %q, want %q", result.Output, "hello from agent")
	}
	if result.Usage.InputTokens != 10 {
		t.Fatalf("got InputTokens=%d, want 10", result.Usage.InputTokens)
	}
	if len(result.TurnSummaries) != 0 {
		t.Fatalf("expected no TurnSummaries for json format, got %d", len(result.TurnSummaries))
	}
}

func TestStreamSpawnWithMockAgent(t *testing.T) {
	stream := strings.Join([]string{
		streamInitLine,
		makeAssistantLine(100, 50, "Read", "Edit"),
		makeResultLine("task complete", 0.05, 500, 200, 1),
	}, "\n")
	t.Setenv("MOCK_STREAM_JSON", stream)

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
	if result.Output != "task complete" {
		t.Fatalf("got output %q, want %q", result.Output, "task complete")
	}
	if result.Usage.InputTokens != 500 {
		t.Fatalf("got InputTokens=%d, want 500", result.Usage.InputTokens)
	}
	if result.Usage.OutputTokens != 200 {
		t.Fatalf("got OutputTokens=%d, want 200", result.Usage.OutputTokens)
	}
	if result.Usage.CostUSD != 0.05 {
		t.Fatalf("got CostUSD=%f, want 0.05", result.Usage.CostUSD)
	}
	if len(result.TurnSummaries) != 1 {
		t.Fatalf("got %d TurnSummaries, want 1", len(result.TurnSummaries))
	}
	ts := result.TurnSummaries[0]
	if ts.Turn != 1 {
		t.Fatalf("Turn=%d, want 1", ts.Turn)
	}
	if ts.InputTokens != 100 {
		t.Fatalf("TurnSummary.InputTokens=%d, want 100", ts.InputTokens)
	}
	if len(ts.Tools) != 2 || ts.Tools[0] != "Read" || ts.Tools[1] != "Edit" {
		t.Fatalf("TurnSummary.Tools=%v, want [Read Edit]", ts.Tools)
	}
}

func TestStreamSpawnMultipleTurns(t *testing.T) {
	stream := strings.Join([]string{
		streamInitLine,
		makeAssistantLine(100, 50, "Read"),
		makeAssistantLine(200, 80, "Edit", "Write"),
		makeAssistantLine(150, 60),
		makeResultLine("done", 0.10, 1000, 500, 3),
	}, "\n")
	t.Setenv("MOCK_STREAM_JSON", stream)

	result, err := Spawn(context.Background(), SpawnOptions{
		Prompt:      "test",
		CommandName: testBin,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "done" {
		t.Fatalf("got output %q, want %q", result.Output, "done")
	}
	if len(result.TurnSummaries) != 3 {
		t.Fatalf("got %d TurnSummaries, want 3", len(result.TurnSummaries))
	}
	for i, ts := range result.TurnSummaries {
		if ts.Turn != i+1 {
			t.Fatalf("TurnSummaries[%d].Turn=%d, want %d", i, ts.Turn, i+1)
		}
	}
	// First turn: Read
	if len(result.TurnSummaries[0].Tools) != 1 || result.TurnSummaries[0].Tools[0] != "Read" {
		t.Fatalf("turn 1 tools=%v, want [Read]", result.TurnSummaries[0].Tools)
	}
	// Second turn: Edit, Write
	if len(result.TurnSummaries[1].Tools) != 2 {
		t.Fatalf("turn 2 tools=%v, want [Edit Write]", result.TurnSummaries[1].Tools)
	}
	// Third turn: no tools
	if len(result.TurnSummaries[2].Tools) != 0 {
		t.Fatalf("turn 3 tools=%v, want empty", result.TurnSummaries[2].Tools)
	}
}

func TestMockagentStreamJSONMode(t *testing.T) {
	// Verify mockagent correctly emits valid stream-json lines
	stream := strings.Join([]string{
		streamInitLine,
		makeAssistantLine(100, 50, "Read"),
		makeResultLine("ok", 0.01, 100, 50, 1),
	}, "\n")
	t.Setenv("MOCK_STREAM_JSON", stream)

	result, err := Spawn(context.Background(), SpawnOptions{
		Prompt:      "test",
		CommandName: testBin,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code %d, want 0", result.ExitCode)
	}
	if result.Output != "ok" {
		t.Fatalf("output=%q, want %q", result.Output, "ok")
	}
	if len(result.TurnSummaries) != 1 {
		t.Fatalf("TurnSummaries=%d, want 1", len(result.TurnSummaries))
	}
}

func TestWatchdogKillsInactiveAgent(t *testing.T) {
	// Set short inactivity timeout for testing
	origTimeout := inactivityTimeout
	origInterval := watchdogInterval
	inactivityTimeout = 500 * time.Millisecond
	watchdogInterval = 100 * time.Millisecond
	t.Cleanup(func() {
		inactivityTimeout = origTimeout
		watchdogInterval = origInterval
	})

	// Emit init line then sleep long enough for watchdog to kill
	t.Setenv("MOCK_STREAM_JSON", streamInitLine)
	t.Setenv("MOCK_SLEEP_MS", "5000")

	result, err := Spawn(context.Background(), SpawnOptions{
		Prompt:      "test",
		CommandName: testBin,
		Timeout:     10 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.InactivityKill {
		t.Fatal("expected InactivityKill=true")
	}
	if result.TimedOut {
		t.Fatal("expected TimedOut=false")
	}
	if result.ExitCode == 0 {
		t.Fatal("expected non-zero exit code")
	}
}

func TestWatchdogDoesNotKillActiveAgent(t *testing.T) {
	origTimeout := inactivityTimeout
	origInterval := watchdogInterval
	inactivityTimeout = 2 * time.Second
	watchdogInterval = 200 * time.Millisecond
	t.Cleanup(func() {
		inactivityTimeout = origTimeout
		watchdogInterval = origInterval
	})

	// Emit lines every 100ms for 1s — well within the 2s timeout
	stream := strings.Join([]string{
		streamInitLine,
		makeAssistantLine(100, 50, "Read"),
		makeAssistantLine(200, 80, "Edit"),
		makeResultLine("alive", 0.01, 100, 50, 2),
	}, "\n")
	t.Setenv("MOCK_STREAM_JSON", stream)
	t.Setenv("MOCK_STREAM_DELAY_MS", "100")

	result, err := Spawn(context.Background(), SpawnOptions{
		Prompt:      "test",
		CommandName: testBin,
		Timeout:     10 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.InactivityKill {
		t.Fatal("expected InactivityKill=false")
	}
	if result.TimedOut {
		t.Fatal("expected TimedOut=false")
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code %d, want 0", result.ExitCode)
	}
	if result.Output != "alive" {
		t.Fatalf("output=%q, want %q", result.Output, "alive")
	}
}

func TestOverallTimeoutInStreamMode(t *testing.T) {
	origTimeout := inactivityTimeout
	origInterval := watchdogInterval
	inactivityTimeout = 30 * time.Second // High so watchdog doesn't fire
	watchdogInterval = 5 * time.Second
	t.Cleanup(func() {
		inactivityTimeout = origTimeout
		watchdogInterval = origInterval
	})

	// Emit init then sleep indefinitely
	t.Setenv("MOCK_STREAM_JSON", streamInitLine)
	t.Setenv("MOCK_SLEEP_MS", "30000")

	result, err := Spawn(context.Background(), SpawnOptions{
		Prompt:      "test",
		CommandName: testBin,
		Timeout:     500 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.TimedOut {
		t.Fatal("expected TimedOut=true")
	}
	if result.InactivityKill {
		t.Fatal("expected InactivityKill=false")
	}
	if result.ExitCode != -1 {
		t.Fatalf("exit code %d, want -1", result.ExitCode)
	}
}

func TestStreamMalformedJSON(t *testing.T) {
	// Mix non-JSON lines with valid stream-json
	stream := strings.Join([]string{
		"this is not json",
		streamInitLine,
		"{malformed",
		makeAssistantLine(100, 50, "Read"),
		"another plain line",
		makeResultLine("ok", 0.01, 100, 50, 1),
	}, "\n")
	t.Setenv("MOCK_STREAM_JSON", stream)

	result, err := Spawn(context.Background(), SpawnOptions{
		Prompt:      "test",
		CommandName: testBin,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code %d, want 0", result.ExitCode)
	}
	if result.Output != "ok" {
		t.Fatalf("output=%q, want %q", result.Output, "ok")
	}
	if len(result.TurnSummaries) != 1 {
		t.Fatalf("TurnSummaries=%d, want 1", len(result.TurnSummaries))
	}
}

func TestStreamNoResultMessage(t *testing.T) {
	// Emit init + assistant but no result, then exit
	stream := strings.Join([]string{
		streamInitLine,
		makeAssistantLine(100, 50, "Read"),
	}, "\n")
	t.Setenv("MOCK_STREAM_JSON", stream)
	t.Setenv("MOCK_EXIT_CODE", "1")

	result, err := Spawn(context.Background(), SpawnOptions{
		Prompt:      "test",
		CommandName: testBin,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Fatalf("exit code %d, want 1", result.ExitCode)
	}
	// Output should be concatenated raw lines
	if !strings.Contains(result.Output, "system") {
		t.Fatalf("expected raw output to contain init message, got: %q", result.Output)
	}
	if len(result.TurnSummaries) != 1 {
		t.Fatalf("TurnSummaries=%d, want 1", len(result.TurnSummaries))
	}
}

func TestStreamNoOutput(t *testing.T) {
	// Mockagent exits immediately without emitting anything
	// Don't set MOCK_STREAM_JSON or MOCK_OUTPUT
	t.Setenv("MOCK_EXIT_CODE", "1")

	result, err := Spawn(context.Background(), SpawnOptions{
		Prompt:      "test",
		CommandName: testBin,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Fatalf("exit code %d, want 1", result.ExitCode)
	}
	if result.Output != "" {
		t.Fatalf("output=%q, want empty", result.Output)
	}
	if len(result.TurnSummaries) != 0 {
		t.Fatalf("TurnSummaries=%d, want 0", len(result.TurnSummaries))
	}
}

func TestStreamNoVerboseForJSON(t *testing.T) {
	// When OutputFormat is "json", --verbose should NOT be added
	t.Setenv("MOCK_ECHO_ARGS", "1")

	result, err := Spawn(context.Background(), SpawnOptions{
		Prompt:       "test",
		CommandName:  testBin,
		OutputFormat: "json",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(result.Output, "--verbose") {
		t.Fatalf("--verbose should NOT be present for json format, got: %s", result.Output)
	}
}
