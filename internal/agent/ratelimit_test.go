package agent

import (
	"context"
	"testing"
)

// TestBuildStreamResultRateLimitDetected verifies that RateLimit is set when
// is_error=true and the result text contains "rate limit".
func TestBuildStreamResultRateLimitDetected(t *testing.T) {
	rateLimitResult := &streamResult{
		Type:    "result",
		Subtype: "error",
		IsError: true,
		Result:  "API rate limit exceeded. Please retry later.",
	}
	out := streamOutput{result: rateLimitResult}
	ctx := context.Background()
	result, err := buildStreamResult(ctx, ctx, nil, out, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil SpawnResult")
	}
	if !result.RateLimit {
		t.Fatalf("RateLimit=false, want true for rate limit error result")
	}
	if result.ExitCode != 1 {
		t.Fatalf("ExitCode=%d, want 1 for error result", result.ExitCode)
	}
}

// TestBuildStreamResultRateLimitCaseInsensitive verifies that rate limit
// detection is case-insensitive.
func TestBuildStreamResultRateLimitCaseInsensitive(t *testing.T) {
	cases := []string{
		"Rate Limit reached",
		"RATE LIMIT EXCEEDED",
		"rate limit: 429",
	}
	for _, msg := range cases {
		res := &streamResult{Type: "result", Subtype: "error", IsError: true, Result: msg}
		out := streamOutput{result: res}
		ctx := context.Background()
		result, err := buildStreamResult(ctx, ctx, nil, out, false)
		if err != nil {
			t.Fatalf("msg=%q: unexpected error: %v", msg, err)
		}
		if !result.RateLimit {
			t.Fatalf("msg=%q: RateLimit=false, want true", msg)
		}
	}
}

// TestBuildStreamResultNoRateLimitForOtherError verifies that RateLimit=false
// for error results that are not rate limit related.
func TestBuildStreamResultNoRateLimitForOtherError(t *testing.T) {
	errResult := &streamResult{
		Type:    "result",
		Subtype: "error",
		IsError: true,
		Result:  "Task cancelled by user",
	}
	out := streamOutput{result: errResult}
	ctx := context.Background()
	result, err := buildStreamResult(ctx, ctx, nil, out, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RateLimit {
		t.Fatalf("RateLimit=true, want false for non-rate-limit error")
	}
	if result.ExitCode != 1 {
		t.Fatalf("ExitCode=%d, want 1 for error result", result.ExitCode)
	}
}

// TestBuildStreamResultNoRateLimitOnSuccess verifies that RateLimit=false
// when the result is successful.
func TestBuildStreamResultNoRateLimitOnSuccess(t *testing.T) {
	successResult := &streamResult{
		Type:    "result",
		Subtype: "success",
		IsError: false,
		Result:  "Done successfully",
	}
	out := streamOutput{result: successResult}
	ctx := context.Background()
	result, err := buildStreamResult(ctx, ctx, nil, out, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RateLimit {
		t.Fatalf("RateLimit=true, want false for success result")
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode=%d, want 0 for success result", result.ExitCode)
	}
}
