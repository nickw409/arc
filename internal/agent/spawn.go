package agent

import (
	"context"
	"time"
)

// SpawnOptions configures agent subprocess spawning.
type SpawnOptions struct {
	Prompt       string
	AllowedTools []string
	MaxTurns     int
	Timeout      time.Duration
	OutputFormat string
	Model        string
	CommandName  string
}

// SpawnResult is the outcome of a spawned agent subprocess.
type SpawnResult struct {
	Output   string
	ExitCode int
	TimedOut bool
}

// Spawn launches a Claude CLI sub-agent as a subprocess.
func Spawn(ctx context.Context, opts SpawnOptions) (*SpawnResult, error) {
	panic("not implemented")
}
