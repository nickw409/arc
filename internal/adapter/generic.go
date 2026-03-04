package adapter

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/nwiley/arc/internal/arc"
)

// defaultGenericTimeout is used when no timeout is specified in SessionConfig.
const defaultGenericTimeout = time.Hour

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

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := make([]string, len(a.Args))
	copy(args, a.Args)

	var tempFile string
	var promptFilePath string

	switch {
	case a.PromptFile != "":
		// Write prompt to workdir/PromptFile; no flag needed.
		promptFilePath = filepath.Join(workdir, a.PromptFile)
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
	var stdinData []byte
	if a.PromptFile == "" && a.PromptFlag == "" {
		stdinData = []byte(prompt)
	}
	cmd.Stdin = bytes.NewReader(stdinData)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	exitCode := 0
	timedOut := false

	if err != nil {
		if timeoutCtx.Err() != nil && ctx.Err() == nil {
			// Overall timeout fired, not a parent cancellation.
			timedOut = true
			exitCode = -1
		} else if ctx.Err() != nil {
			return nil, ctx.Err()
		} else if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, err
		}
	}

	return &arc.AgentResult{
		ExitCode: exitCode,
		Output:   stdout.String(),
		Stderr:   stderr.String(),
		TimedOut: timedOut,
		Duration: duration,
	}, nil
}

// Preflight checks that the command exists in PATH.
func (a *GenericAdapter) Preflight(ctx context.Context, workdir string) error {
	if _, err := exec.LookPath(a.Command); err != nil {
		return fmt.Errorf("command %q not found in PATH: %w", a.Command, err)
	}
	return nil
}
