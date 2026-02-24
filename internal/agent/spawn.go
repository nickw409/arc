package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/nwiley/arc/internal/arc"
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
	Usage    arc.Usage
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
		outputFormat = "json"
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

	// Clear CLAUDECODE env var so subprocesses aren't blocked by the
	// nested-session check when arc is invoked from within Claude Code.
	cmd.Env = filterEnv(os.Environ(), "CLAUDECODE")
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

	var result *SpawnResult
	if err != nil {
		if timeoutCtx.Err() != nil && ctx.Err() == nil {
			result = &SpawnResult{
				Output:   stdout.String(),
				ExitCode: -1,
				TimedOut: true,
			}
		} else if ctx.Err() != nil {
			return nil, ctx.Err()
		} else {
			exitCode := 1
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			}
			result = &SpawnResult{
				Output:   stdout.String(),
				ExitCode: exitCode,
				TimedOut: false,
			}
		}
	} else {
		result = &SpawnResult{
			Output:   stdout.String(),
			ExitCode: 0,
			TimedOut: false,
		}
	}

	// Parse JSON envelope if present
	if text, usage, ok := parseJSONOutput(result.Output); ok {
		result.Output = text
		result.Usage = usage
	}

	return result, nil
}

// claudeJSONResult is the JSON envelope returned by `claude --output-format json`.
type claudeJSONResult struct {
	Result    string  `json:"result"`
	TotalCost float64 `json:"total_cost_usd"`
	Usage     struct {
		InputTokens              int `json:"input_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		OutputTokens             int `json:"output_tokens"`
	} `json:"usage"`
}

// parseJSONOutput attempts to parse the Claude CLI JSON envelope.
// Returns the extracted text, usage, and whether parsing succeeded.
func parseJSONOutput(raw string) (string, arc.Usage, bool) {
	var envelope claudeJSONResult
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		return "", arc.Usage{}, false
	}
	if envelope.Result == "" && envelope.Usage.InputTokens == 0 {
		return "", arc.Usage{}, false
	}
	usage := arc.Usage{
		InputTokens:              envelope.Usage.InputTokens,
		OutputTokens:             envelope.Usage.OutputTokens,
		CacheCreationInputTokens: envelope.Usage.CacheCreationInputTokens,
		CacheReadInputTokens:     envelope.Usage.CacheReadInputTokens,
		CostUSD:                  envelope.TotalCost,
	}
	return envelope.Result, usage, true
}

// filterEnv returns a copy of environ with the named variable removed.
func filterEnv(environ []string, name string) []string {
	prefix := name + "="
	filtered := make([]string, 0, len(environ))
	for _, e := range environ {
		if !strings.HasPrefix(e, prefix) {
			filtered = append(filtered, e)
		}
	}
	return filtered
}
