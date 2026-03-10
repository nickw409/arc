package adapter

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/nwiley/arc/internal/arc"
)

// defaultCodexTimeout is the fallback timeout when none is set in SessionConfig.
const defaultCodexTimeout = 30 * time.Minute

// CodexAdapter implements arc.AgentAdapter for the OpenAI Codex CLI.
// It invokes: codex --quiet --approval-mode full-auto --prompt "<prompt>"
type CodexAdapter struct{}

// Name returns the adapter identifier.
func (a *CodexAdapter) Name() string { return "codex" }


// Spawn runs a Codex session with the given prompt and session config.
// Usage tracking is not available from codex and will be zero-valued.
// Turn limits are not available; timeout is the only termination mechanism.
func (a *CodexAdapter) Spawn(ctx context.Context, prompt string, workdir string, cfg arc.SessionConfig) (*arc.AgentResult, error) {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultCodexTimeout
	}

	args := []string{
		"--quiet",
		"--approval-mode", "full-auto",
		"--prompt", prompt,
	}
	if cfg.Model != "" {
		args = append(args, "--model", cfg.Model)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, "codex", args...)
	cmd.Dir = workdir

	// Use a process group so that SIGKILL reaches all child processes.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	start := time.Now()
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	var wg sync.WaitGroup
	var lastActivity atomic.Int64
	lastActivity.Store(time.Now().UnixNano())

	wg.Add(2)
	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, readErr := stdoutPipe.Read(buf)
			if n > 0 {
				stdoutBuf.Write(buf[:n])
				lastActivity.Store(time.Now().UnixNano())
			}
			if readErr != nil {
				break
			}
		}
	}()
	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, readErr := stderrPipe.Read(buf)
			if n > 0 {
				stderrBuf.Write(buf[:n])
				lastActivity.Store(time.Now().UnixNano())
			}
			if readErr != nil {
				break
			}
		}
	}()

	wg.Wait()
	waitErr := cmd.Wait()
	duration := time.Since(start)

	exitCode := 0
	timedOut := false

	if waitErr != nil {
		if timeoutCtx.Err() != nil && ctx.Err() == nil {
			// Our timeout fired, not a parent cancellation.
			timedOut = true
			exitCode = -1
		} else if ctx.Err() != nil {
			return nil, ctx.Err()
		} else if exitErr, ok := waitErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, waitErr
		}
	}

	stdout := stdoutBuf.String()
	stderr := stderrBuf.String()
	return &arc.AgentResult{
		ExitCode:  exitCode,
		Output:    stdout,
		Stderr:    stderr,
		TimedOut:  timedOut,
		RateLimit: isCodexRateLimit(stdout, stderr),
		Duration:  duration,
		// Usage is not reported by codex; left as zero.
	}, nil
}

// isCodexRateLimit returns true if stdout or stderr indicate a rate limit error.
func isCodexRateLimit(stdout, stderr string) bool {
	combined := stdout + stderr
	return strings.Contains(combined, "rate_limit_exceeded") ||
		strings.Contains(combined, "Rate limit reached") ||
		strings.Contains(combined, "Too Many Requests") ||
		strings.Contains(combined, "429")
}
