package adapter

import (
	"context"
	"fmt"
	"os/exec"

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
		Duration:       res.Duration,
	}, nil
}

// Preflight checks that the claude binary exists and is executable.
func (a *ClaudeAdapter) Preflight(ctx context.Context, workdir string) error {
	name := a.commandName()

	path, err := exec.LookPath(name)
	if err != nil {
		return fmt.Errorf("claude binary %q not found in PATH: %w", name, err)
	}

	cmd := exec.CommandContext(ctx, path, "--version")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("claude binary %q failed preflight check (%w): %s", name, err, string(out))
	}

	return nil
}

// commandName returns the configured command name, falling back to "claude".
func (a *ClaudeAdapter) commandName() string {
	if a.CommandName != "" {
		return a.CommandName
	}
	return "claude"
}
