package agent

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
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
	cmdName := opts.CommandName
	if cmdName == "" {
		cmdName = "claude"
	}

	maxTurns := opts.MaxTurns
	if maxTurns == 0 {
		maxTurns = 15
	}

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 600 * time.Second
	}

	outputFormat := opts.OutputFormat
	if outputFormat == "" {
		outputFormat = "text"
	}

	allowedTools := opts.AllowedTools
	if len(allowedTools) == 0 {
		allowedTools = []string{"View", "Edit", "Write", "Bash"}
	}

	args := []string{
		"--print",
		"--output-format", outputFormat,
		"--max-turns", strconv.Itoa(maxTurns),
		"--allowedTools", strings.Join(allowedTools, ","),
	}

	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, cmdName, args...)
	cmd.Stdin = strings.NewReader(opts.Prompt)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}

	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	err := cmd.Wait()
	if err != nil {
		if timeoutCtx.Err() != nil && ctx.Err() == nil {
			return &SpawnResult{
				Output:   stdout.String(),
				ExitCode: -1,
				TimedOut: true,
			}, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		exitCode := 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		return &SpawnResult{
			Output:   stdout.String(),
			ExitCode: exitCode,
			TimedOut: false,
		}, nil
	}

	return &SpawnResult{
		Output:   stdout.String(),
		ExitCode: 0,
		TimedOut: false,
	}, nil
}
