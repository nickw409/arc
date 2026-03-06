package arc

import (
	"testing"
	"time"
)

func TestSessionConfigDefaults(t *testing.T) {
	var cfg SessionConfig
	if cfg.MaxTurns != 0 {
		t.Errorf("zero value MaxTurns = %d, want 0", cfg.MaxTurns)
	}
	if cfg.Timeout != 0 {
		t.Errorf("zero value Timeout = %v, want 0", cfg.Timeout)
	}
}

func TestAgentResultFields(t *testing.T) {
	r := &AgentResult{
		ExitCode: 0,
		Output:   "done",
		Duration: 5 * time.Second,
		Usage:    Usage{InputTokens: 100, OutputTokens: 50, CostUSD: 0.01},
	}
	if r.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", r.ExitCode)
	}
	if r.Duration != 5*time.Second {
		t.Errorf("Duration = %v, want 5s", r.Duration)
	}
	if r.Usage.CostUSD != 0.01 {
		t.Errorf("CostUSD = %f, want 0.01", r.Usage.CostUSD)
	}
}
