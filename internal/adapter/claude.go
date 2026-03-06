package adapter

import (
	"context"
	"fmt"
	"os/exec"
	"time"

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
		PID:            res.PID,
	}, nil
}

// Preflight checks that the claude binary exists, is executable, and is
// authenticated. It also verifies that workdir is accessible and writable.
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

	// Verify authentication by running a minimal prompt with a short timeout.
	authCtx, authCancel := context.WithTimeout(ctx, 10*time.Second)
	defer authCancel()

	authCmd := exec.CommandContext(authCtx, path, "--print", "test", "--max-turns", "1")
	if out, err := authCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("claude authentication check failed — ensure you are logged in (%w): %s", err, string(out))
	}

	// Verify the working directory is accessible and writable.
	if err := checkWorkdirWritable(workdir); err != nil {
		return fmt.Errorf("workdir %q is not accessible: %w", workdir, err)
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
