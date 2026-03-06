package arc

import (
	"context"
	"time"
)

// AgentAdapter is the boundary between arc's orchestration and the agent runtime.
// Any agentic coder that can read a prompt, edit files, and run shell commands
// can be plugged in by implementing this interface.
type AgentAdapter interface {
	// Name returns the adapter identifier (e.g., "claude", "codex", "generic").
	Name() string

	// Spawn launches an agent session with the given prompt in the working directory.
	// It blocks until the agent exits and returns the result.
	Spawn(ctx context.Context, prompt string, workdir string, config SessionConfig) (*AgentResult, error)

	// Preflight validates that the agent is available and properly configured.
	// Returns nil if the adapter is ready to spawn agents.
	Preflight(ctx context.Context, workdir string) error
}

// SessionConfig configures an agent session spawned by an adapter.
type SessionConfig struct {
	MaxTurns int           `json:"max_turns,omitempty" yaml:"max_turns,omitempty"`
	Timeout  time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	Model    string        `json:"model,omitempty" yaml:"model,omitempty"`
	Tools    []string      `json:"tools,omitempty" yaml:"tools,omitempty"`
	// OnTurn is called after each agent turn with the tool names used that turn.
	// It is not serialized. Adapters that support streaming should call it.
	OnTurn func(tools []string) `json:"-" yaml:"-"`
}

// AgentResult is the outcome of a spawned agent session.
type AgentResult struct {
	ExitCode       int           `json:"exit_code"`
	Output         string        `json:"output"`
	Stderr         string        `json:"stderr,omitempty"`
	Usage          Usage         `json:"usage,omitempty"`
	TimedOut       bool          `json:"timed_out,omitempty"`
	InactivityKill bool          `json:"inactivity_kill,omitempty"`
	Duration       time.Duration `json:"duration"`
	PID            int           `json:"pid,omitempty"` // subprocess PID; 0 if not available
}
