package agent

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestBuildStreamResultIgnoresScannerError verifies that buildStreamResult
// propagates scanner errors. When the scanner encounters a fatal error
// (e.g., a line exceeds the 1MB buffer limit), output.err is set but
// buildStreamResult never checks it — the error is silently swallowed
// and the result appears as a successful exit with partial data.
func TestBuildStreamResultIgnoresScannerError(t *testing.T) {
	scannerErr := fmt.Errorf("bufio.Scanner: token too long")
	output := streamOutput{
		result:   nil,
		rawLines: []string{`{"type":"system","subtype":"init"}`},
		err:      scannerErr,
	}

	ctx := context.Background()
	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Process exited normally (exit 0) but scanner had a fatal error.
	// We pass nil for waitErr to simulate a clean process exit.
	result, err := buildStreamResult(ctx, timeoutCtx, nil, output, false)

	// With a scanner error and no result message, the function should
	// either return an error or indicate a problem via non-zero ExitCode.
	// The spec (TestScannerBufferOverflow) requires:
	//   "scanner.Err() is non-nil, error is logged, Spawn returns non-zero exit with error indication"
	if err == nil && result != nil && result.ExitCode == 0 {
		t.Fatalf("scanner error silently ignored: got ExitCode=0, err=nil; "+
			"expected error propagation when scanner.Err() = %v", scannerErr)
	}
}

// TestWatchdogKillLosesUsage verifies that the watchdog kill path preserves
// Usage data when a result message was already received before the kill.
// The non-watchdog error path calls usageFromStreamResult() but the watchdog
// path does not, losing cost and token data.
func TestWatchdogKillLosesUsage(t *testing.T) {
	sr := &streamResult{
		Type:      "result",
		Subtype:   "success",
		Result:    "task done",
		TotalCost: 0.05,
		NumTurns:  3,
	}
	sr.Usage.InputTokens = 500
	sr.Usage.OutputTokens = 200
	sr.Usage.CacheCreationInputTokens = 30
	sr.Usage.CacheReadInputTokens = 40

	output := streamOutput{
		result:    sr,
		summaries: []TurnSummary{{Turn: 1, InputTokens: 100, OutputTokens: 50}},
	}

	ctx := context.Background()
	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Simulate watchdog kill: process exited with error, watchdog fired.
	// The result message was already received before the kill.
	result, err := buildStreamResult(ctx, timeoutCtx, fmt.Errorf("signal: killed"), output, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify Output was extracted from result message
	if result.Output != "task done" {
		t.Fatalf("Output=%q, want %q", result.Output, "task done")
	}

	// Usage should be populated from the result message, same as the
	// non-watchdog error path which calls usageFromStreamResult().
	if result.Usage.InputTokens != 500 {
		t.Fatalf("Usage.InputTokens=%d, want 500 (usage lost on watchdog kill path)",
			result.Usage.InputTokens)
	}
	if result.Usage.OutputTokens != 200 {
		t.Fatalf("Usage.OutputTokens=%d, want 200", result.Usage.OutputTokens)
	}
	if result.Usage.CostUSD != 0.05 {
		t.Fatalf("Usage.CostUSD=%f, want 0.05", result.Usage.CostUSD)
	}
	if result.Usage.CacheCreationInputTokens != 30 {
		t.Fatalf("Usage.CacheCreationInputTokens=%d, want 30", result.Usage.CacheCreationInputTokens)
	}
	if result.Usage.CacheReadInputTokens != 40 {
		t.Fatalf("Usage.CacheReadInputTokens=%d, want 40", result.Usage.CacheReadInputTokens)
	}
}

// TestParseStreamResultAcceptsWrongType verifies that parseStreamResult
// validates the "type" field. Currently it unmarshals any valid JSON and
// returns true, even for assistant messages that are not result messages.
func TestParseStreamResultAcceptsWrongType(t *testing.T) {
	// This is a valid assistant message, NOT a result message
	assistantJSON := `{"type":"assistant","message":{"content":[],"usage":{"input_tokens":100,"output_tokens":50}}}`

	_, ok := parseStreamResult(assistantJSON)
	if ok {
		t.Fatal("parseStreamResult returned true for type='assistant'; should reject non-result JSON")
	}
}

// TestParseStreamAssistantAcceptsWrongType verifies that parseStreamAssistant
// validates the "type" field. Currently it unmarshals any valid JSON and
// returns true, even for result messages that are not assistant messages.
func TestParseStreamAssistantAcceptsWrongType(t *testing.T) {
	// This is a valid result message, NOT an assistant message
	resultJSON := `{"type":"result","subtype":"success","result":"done","total_cost_usd":0.01}`

	_, ok := parseStreamAssistant(resultJSON)
	if ok {
		t.Fatal("parseStreamAssistant returned true for type='result'; should reject non-assistant JSON")
	}
}
