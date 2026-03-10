package adapter

import (
	"context"

	"github.com/nwiley/arc/internal/agent"
	"github.com/nwiley/arc/internal/arc"
)

// ClaudeAdapter implements arc.AgentAdapter for the Claude Code CLI.
type ClaudeAdapter struct {
	// CommandName overrides the default "claude" binary name.
	CommandName string
}

// Name returns the adapter identifier.
func (a *ClaudeAdapter) Name() string { return "claude" }

// Spawn runs a Claude Code sub-agent with the given prompt and session config.
func (a *ClaudeAdapter) Spawn(ctx context.Context, prompt string, workdir string, config arc.SessionConfig) (*arc.AgentResult, error) {
	opts := agent.SpawnOptions{
		Prompt:       prompt,
		WorkingDir:   workdir,
		MaxTurns:     config.MaxTurns,
		Timeout:      config.Timeout,
		Model:        config.Model,
		AllowedTools: config.Tools,
		OutputFormat: "stream-json",
		CommandName:  a.commandName(),
		OnTurn:       config.OnTurn,
	}

	res, err := agent.Spawn(ctx, opts)
	if err != nil {
		return nil, err
	}

	return &arc.AgentResult{
		ExitCode:       res.ExitCode,
		Output:         res.Output,
		Stderr:         res.Stderr,
		Usage:          res.Usage,
		TimedOut:       res.TimedOut,
		InactivityKill: res.InactivityKill,
		RateLimit:      res.RateLimit,
		Duration:       res.Duration,
		PID:            res.PID,
	}, nil
}


// commandName returns the configured command name, falling back to "claude".
func (a *ClaudeAdapter) commandName() string {
	if a.CommandName != "" {
		return a.CommandName
	}
	return "claude"
}
