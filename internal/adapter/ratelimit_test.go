package adapter

import (
	"context"
	"testing"
	"time"

	"github.com/nwiley/arc/internal/arc"
)

// --- isCodexRateLimit ---

func TestIsCodexRateLimitTrue(t *testing.T) {
	cases := []struct{ stdout, stderr string }{
		{"rate_limit_exceeded occurred", ""},
		{"", "Rate limit reached for this model"},
		{"Too Many Requests: slow down", ""},
		{"", "HTTP 429 error"},
		{"some output", "429: quota exceeded"},
	}
	for _, tc := range cases {
		if !isCodexRateLimit(tc.stdout, tc.stderr) {
			t.Errorf("isCodexRateLimit(%q, %q) = false, want true", tc.stdout, tc.stderr)
		}
	}
}

func TestIsCodexRateLimitFalse(t *testing.T) {
	cases := []struct{ stdout, stderr string }{
		{"", ""},
		{"task completed successfully", ""},
		{"rate", "limit"},    // partial words, not the expected phrases
		{"", "other error"},
		{"error 500 internal", ""},
	}
	for _, tc := range cases {
		if isCodexRateLimit(tc.stdout, tc.stderr) {
			t.Errorf("isCodexRateLimit(%q, %q) = true, want false", tc.stdout, tc.stderr)
		}
	}
}

// --- isGenericRateLimit ---

func TestIsGenericRateLimitTrue(t *testing.T) {
	cases := []struct{ stdout, stderr string }{
		{"rate limit hit", ""},
		{"", "RATE LIMIT EXCEEDED"},
		{"Rate Limit reached", ""},
		{"rate_limit encountered", ""},
		{"", "RATE_LIMIT_ERROR"},
		{"too many requests", ""},
		{"Too Many Requests from client", ""},
		{"HTTP 429 response", ""},
		{"", "error 429: slow down"},
	}
	for _, tc := range cases {
		if !isGenericRateLimit(tc.stdout, tc.stderr) {
			t.Errorf("isGenericRateLimit(%q, %q) = false, want true", tc.stdout, tc.stderr)
		}
	}
}

func TestIsGenericRateLimitFalse(t *testing.T) {
	cases := []struct{ stdout, stderr string }{
		{"", ""},
		{"build successful", ""},
		{"", "connection refused"},
		{"rate", ""},
		{"error 42", ""},
	}
	for _, tc := range cases {
		if isGenericRateLimit(tc.stdout, tc.stderr) {
			t.Errorf("isGenericRateLimit(%q, %q) = true, want false", tc.stdout, tc.stderr)
		}
	}
}

// --- ClaudeAdapter RateLimit propagation ---

func TestClaudeAdapterRateLimitPropagated(t *testing.T) {
	bin := buildMockAgent(t)

	// Emit a stream-json result with is_error=true and "rate limit" in the message.
	streamLines := `{"type":"system","subtype":"init","session_id":"test","tools":[],"mcp_servers":[]}` + "\n" +
		`{"type":"result","subtype":"error","is_error":true,"result":"API rate limit exceeded, please retry later","total_cost_usd":0.0,"num_turns":1,"usage":{"input_tokens":10,"output_tokens":5,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}`
	t.Setenv("MOCK_STREAM_JSON", streamLines)

	a := &ClaudeAdapter{CommandName: bin}
	res, err := a.Spawn(context.Background(), "prompt", t.TempDir(), arc.SessionConfig{
		Timeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil AgentResult")
	}
	if !res.RateLimit {
		t.Fatalf("RateLimit=false, want true when result contains 'rate limit' with is_error=true")
	}
	if res.ExitCode != 1 {
		t.Fatalf("ExitCode=%d, want 1 for error result", res.ExitCode)
	}
}

func TestClaudeAdapterRateLimitFalseOnSuccess(t *testing.T) {
	bin := buildMockAgent(t)

	streamLines := `{"type":"system","subtype":"init","session_id":"test","tools":[],"mcp_servers":[]}` + "\n" +
		`{"type":"result","subtype":"success","is_error":false,"result":"Done","total_cost_usd":0.0,"num_turns":1,"usage":{"input_tokens":10,"output_tokens":5,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}`
	t.Setenv("MOCK_STREAM_JSON", streamLines)

	a := &ClaudeAdapter{CommandName: bin}
	res, err := a.Spawn(context.Background(), "prompt", t.TempDir(), arc.SessionConfig{
		Timeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.RateLimit {
		t.Fatal("RateLimit=true, want false for successful result")
	}
}
