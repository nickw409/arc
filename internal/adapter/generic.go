package adapter

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"strings"
	"syscall"
	"time"

	"github.com/nwiley/arc/internal/arc"
)

// defaultGenericTimeout is used when no timeout is specified in SessionConfig.
const defaultGenericTimeout = time.Hour

// genericWatchdogInterval is how often the generic watchdog checks for inactivity.
// Tests may override this for faster execution.
var genericWatchdogInterval = 30 * time.Second

// genericInactivityMin is the minimum inactivity timeout applied when the
// computed value (1/3 of total timeout) would be too small. Tests may lower
// this to avoid long waits.
var genericInactivityMin = 2 * time.Minute

// GenericAdapter implements arc.AgentAdapter for arbitrary CLI tools that
// accept a prompt file and run in a working directory.
type GenericAdapter struct {
	// Name_ is the adapter identifier returned by Name().
	Name_ string

	// Command is the executable to run (e.g., "aider").
	Command string

	// Args holds additional arguments passed to the command.
	Args []string

	// PromptFlag is the flag used to pass the prompt file path (e.g., "--message-file").
	// If set, the prompt is written to a temp file and passed as PromptFlag + path.
	PromptFlag string

	// PromptFile, if set, names a file written to workdir containing the prompt.
	// No flag is added to the command in this case.
	PromptFile string

	// Environment holds additional environment variables for the subprocess.
	Environment map[string]string
}

// Name returns the adapter identifier.
func (a *GenericAdapter) Name() string { return a.Name_ }

// Spawn runs the generic command with the provided prompt and config.
func (a *GenericAdapter) Spawn(ctx context.Context, prompt string, workdir string, config arc.SessionConfig) (*arc.AgentResult, error) {
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = defaultGenericTimeout
	}

	// Compute inactivity timeout: 1/3 of total timeout, min genericInactivityMin.
	inactivityLimit := timeout / 3
	if inactivityLimit < genericInactivityMin {
		inactivityLimit = genericInactivityMin
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := make([]string, len(a.Args))
	copy(args, a.Args)

	var tempFile string

	switch {
	case a.PromptFile != "":
		// Write prompt to workdir/PromptFile; no flag needed.
		promptFilePath := filepath.Join(workdir, a.PromptFile)
		if err := os.WriteFile(promptFilePath, []byte(prompt), 0600); err != nil {
			return nil, fmt.Errorf("writing prompt file: %w", err)
		}

	case a.PromptFlag != "":
		// Write prompt to a temp file and pass via flag.
		f, err := os.CreateTemp("", "arc-generic-prompt-*")
		if err != nil {
			return nil, fmt.Errorf("creating temp prompt file: %w", err)
		}
		tempFile = f.Name()
		if _, err := f.WriteString(prompt); err != nil {
			f.Close()
			os.Remove(tempFile)
			return nil, fmt.Errorf("writing temp prompt file: %w", err)
		}
		f.Close()
		args = append(args, a.PromptFlag, tempFile)
	}

	if tempFile != "" {
		defer os.Remove(tempFile)
	}

	cmd := exec.CommandContext(timeoutCtx, a.Command, args...)
	cmd.Dir = workdir

	// Merge parent environment with extra vars.
	env := os.Environ()
	for k, v := range a.Environment {
		env = append(env, k+"="+v)
	}
	cmd.Env = env

	// Pass prompt via stdin when no file mechanism is configured.
	if a.PromptFile == "" && a.PromptFlag == "" {
		cmd.Stdin = bytes.NewReader([]byte(prompt))
	}

	// Use process group so we can kill all children on watchdog fire.
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

	// Heartbeat for watchdog — updated on any stdout/stderr byte.
	var lastActivity atomic.Int64
	lastActivity.Store(time.Now().UnixNano())

	// Collect stdout and stderr concurrently, updating the heartbeat on activity.
	var stdoutBuf, stderrBuf bytes.Buffer
	var wg sync.WaitGroup

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

	// Watchdog goroutine: kills the process if it produces no output for inactivityLimit.
	var watchdogFired atomic.Bool
	watchdogCtx, watchdogCancel := context.WithCancel(ctx)
	defer watchdogCancel()

	go func() {
		ticker := time.NewTicker(genericWatchdogInterval)
		defer ticker.Stop()
		for {
			select {
			case <-watchdogCtx.Done():
				return
			case <-ticker.C:
				last := time.Unix(0, lastActivity.Load())
				if time.Since(last) > inactivityLimit {
					watchdogFired.Store(true)
					if cmd.Process != nil {
						_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
					}
					return
				}
			}
		}
	}()

	// Wait for readers to drain before calling cmd.Wait().
	wg.Wait()
	waitErr := cmd.Wait()
	watchdogCancel()
	duration := time.Since(start)

	exitCode := 0
	timedOut := false

	if waitErr != nil {
		if watchdogFired.Load() {
			exitCode = -1
			if exitErr, ok := waitErr.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			}
			return &arc.AgentResult{
				ExitCode:       exitCode,
				Output:         stdoutBuf.String(),
				Stderr:         stderrBuf.String(),
				InactivityKill: true,
				Duration:       duration,
			}, nil
		}
		if timeoutCtx.Err() != nil && ctx.Err() == nil {
			// Overall timeout fired, not a parent cancellation.
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
		RateLimit: isGenericRateLimit(stdout, stderr),
		Duration:  duration,
	}, nil
}

// isGenericRateLimit returns true if stdout or stderr indicate a rate limit error.
func isGenericRateLimit(stdout, stderr string) bool {
	combined := strings.ToLower(stdout + stderr)
	return strings.Contains(combined, "rate limit") ||
		strings.Contains(combined, "rate_limit") ||
		strings.Contains(combined, "too many requests") ||
		strings.Contains(combined, "429")
}

// Preflight checks that the command exists in PATH and that workdir is
// accessible and writable.
func (a *GenericAdapter) Preflight(ctx context.Context, workdir string) error {
	if _, err := exec.LookPath(a.Command); err != nil {
		return fmt.Errorf("command %q not found in PATH: %w", a.Command, err)
	}

	if err := checkWorkdirWritable(workdir); err != nil {
		return fmt.Errorf("workdir %q is not accessible: %w", workdir, err)
	}

	return nil
}

// checkWorkdirWritable verifies that dir exists and is writable by attempting
// to create and immediately remove a temporary file inside it.
func checkWorkdirWritable(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("stat failed: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory")
	}

	// Attempt to create a temporary file to confirm write access.
	tmp, err := os.CreateTemp(dir, ".arc-preflight-*")
	if err != nil {
		return fmt.Errorf("directory is not writable: %w", err)
	}
	tmp.Close()
	os.Remove(tmp.Name())
	return nil
}
