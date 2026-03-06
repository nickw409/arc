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

// ToolUse represents a single tool invocation within an agent turn.
type ToolUse struct {
	Name string `json:"name"`           // tool name (Edit, Bash, Read, etc.)
	File string `json:"file,omitempty"` // file path for Edit/Read/Write/Glob/Grep
	Cmd  string `json:"cmd,omitempty"`  // command string for Bash
}

// TurnEvent is emitted after each agent turn with structured metadata.
type TurnEvent struct {
	Timestamp time.Time `json:"timestamp"`
	TurnNum   int       `json:"turn"`
	Tools     []ToolUse `json:"tools"`
	TokensIn  int       `json:"tokens_in"`
	TokensOut int       `json:"tokens_out"`
}

// ToolNames returns just the tool names from the event, for backward compat.
func (e TurnEvent) ToolNames() []string {
	names := make([]string, len(e.Tools))
	for i, t := range e.Tools {
		names[i] = t.Name
	}
	return names
}

// SessionConfig configures an agent session spawned by an adapter.
type SessionConfig struct {
	MaxTurns int           `json:"max_turns,omitempty" yaml:"max_turns,omitempty"`
	Timeout  time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	Model    string        `json:"model,omitempty" yaml:"model,omitempty"`
	Tools    []string      `json:"tools,omitempty" yaml:"tools,omitempty"`
	// OnTurn is called after each agent turn with structured turn metadata.
	// It is not serialized. Adapters that support streaming should call it.
	OnTurn func(TurnEvent) `json:"-" yaml:"-"`
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
