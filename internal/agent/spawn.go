package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
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
	WorkingDir   string           // if set, the subprocess runs in this directory
	OnTurn       func(arc.TurnEvent) // called after each agent turn with structured metadata
}

// SpawnResult is the outcome of a spawned agent subprocess.
type SpawnResult struct {
	Output         string
	Stderr         string        // captured stderr from the subprocess
	ExitCode       int
	TimedOut       bool
	InactivityKill bool          // true if watchdog killed the process
	Duration       time.Duration // wall-clock time from Start() to Wait() return
	Usage          arc.Usage
	TurnSummaries  []TurnSummary // per-turn metadata from stream
	PID            int           // subprocess PID (for diagnostics)
}

// inactivityTimeout is the duration after which a process with no stdout
// activity is killed. Tests override this for faster execution.
var inactivityTimeout = 5 * time.Minute

// watchdogInterval is how often the watchdog checks for inactivity.
var watchdogInterval = 30 * time.Second

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
		outputFormat = "stream-json"
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

	if outputFormat == "stream-json" {
		args = append(args, "--verbose")
	}

	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, cmdName, args...)
	cmd.Stdin = strings.NewReader(opts.Prompt)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if opts.WorkingDir != "" {
		cmd.Dir = opts.WorkingDir
	}

	// Clear Claude Code session env vars so subprocesses aren't blocked by
	// the nested-session check when arc is invoked from within Claude Code.
	env := filterEnv(os.Environ(), "CLAUDECODE")
	env = filterEnv(env, "CLAUDE_CODE_ENTRYPOINT")
	cmd.Env = env
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}

	// Use streaming for stream-json, buffered for everything else
	if outputFormat == "stream-json" {
		return spawnStreaming(ctx, timeoutCtx, cmd, opts.OnTurn)
	}
	return spawnBuffered(ctx, timeoutCtx, cmd)
}

// streamOutput collects parsed data from the stream reader goroutine.
type streamOutput struct {
	result    *streamResult
	summaries []TurnSummary
	rawLines  []string
	err       error
}

// spawnStreaming runs the subprocess with piped stdout, parsing stream-json
// lines and running an inactivity watchdog.
func spawnStreaming(ctx context.Context, timeoutCtx context.Context, cmd *exec.Cmd, onTurn func(arc.TurnEvent)) (*SpawnResult, error) {
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	pid := cmd.Process.Pid
	startTime := time.Now()
	slog.Info("agent started", "pid", pid)

	// Heartbeat for watchdog
	var lastActivity atomic.Int64
	lastActivity.Store(time.Now().UnixNano())

	// Stream reader goroutine
	outputCh := make(chan streamOutput, 1)
	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB line buffer
		var summaries []TurnSummary
		var finalResult *streamResult
		var rawLines []string
		turnNum := 0

		for scanner.Scan() {
			line := scanner.Text()
			rawLines = append(rawLines, line)

			// Update heartbeat
			lastActivity.Store(time.Now().UnixNano())

			// Parse message type
			var msg streamMessage
			if json.Unmarshal([]byte(line), &msg) != nil {
				continue
			}

			switch msg.Type {
			case "assistant":
				turnNum++
				if assistant, ok := parseStreamAssistant(line); ok {
					ts := parseTurnSummary(assistant, turnNum)
					summaries = append(summaries, ts)
					if onTurn != nil && len(ts.Tools) > 0 {
						ev := arc.TurnEvent{
							Timestamp: time.Now(),
							TurnNum:   turnNum,
							Tools:     ts.ToolUses,
							TokensIn:  ts.InputTokens,
							TokensOut: ts.OutputTokens,
						}
						onTurn(ev)
					}
				}
			case "result":
				if res, ok := parseStreamResult(line); ok {
					finalResult = res
				}
			}
		}
		outputCh <- streamOutput{result: finalResult, summaries: summaries, rawLines: rawLines, err: scanner.Err()}
	}()

	// Watchdog goroutine
	var watchdogFired atomic.Bool
	watchdogCtx, watchdogCancel := context.WithCancel(ctx)
	defer watchdogCancel()

	go func() {
		ticker := time.NewTicker(watchdogInterval)
		defer ticker.Stop()
		for {
			select {
			case <-watchdogCtx.Done():
				return
			case <-ticker.C:
				last := time.Unix(0, lastActivity.Load())
				if time.Since(last) > inactivityTimeout {
					slog.Warn("agent inactive, killing", "pid", cmd.Process.Pid, "inactive_for", time.Since(last))
					watchdogFired.Store(true)
					_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
					return
				}
			}
		}
	}()

	output := <-outputCh   // Wait for scanner to finish before cmd.Wait() closes pipe
	waitErr := cmd.Wait()
	watchdogCancel()
	duration := time.Since(startTime)

	exitCode := 0
	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}
	slog.Info("agent exited", "pid", pid, "exit_code", exitCode, "duration", duration, "stderr_len", stderr.Len())

	result, err2 := buildStreamResult(ctx, timeoutCtx, waitErr, output, watchdogFired.Load())
	if result != nil {
		result.Stderr = stderr.String()
		result.Duration = duration
		result.PID = pid
	}
	return result, err2
}

// buildStreamResult constructs a SpawnResult from stream parsing output.
func buildStreamResult(ctx context.Context, timeoutCtx context.Context, waitErr error, output streamOutput, watchdogKilled bool) (*SpawnResult, error) {
	if waitErr != nil {
		if timeoutCtx.Err() != nil && ctx.Err() == nil && !watchdogKilled {
			// Overall timeout fired
			return &SpawnResult{
				Output:        strings.Join(output.rawLines, "\n"),
				ExitCode:      -1,
				TimedOut:      true,
				TurnSummaries: output.summaries,
			}, nil
		}
		if ctx.Err() != nil && !watchdogKilled {
			return nil, ctx.Err()
		}

		exitCode := 1
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}

		if watchdogKilled {
			result := &SpawnResult{
				ExitCode:       exitCode,
				InactivityKill: true,
				TurnSummaries:  output.summaries,
			}
			if output.result != nil {
				result.Output = output.result.Result
				result.Usage = usageFromStreamResult(output.result)
			} else {
				result.Output = strings.Join(output.rawLines, "\n")
			}
			return result, nil
		}

		// Non-zero exit (e.g. SIGTERM, crash)
		result := &SpawnResult{
			ExitCode:      exitCode,
			TurnSummaries: output.summaries,
		}
		if output.result != nil {
			result.Output = output.result.Result
			result.Usage = usageFromStreamResult(output.result)
		} else {
			result.Output = strings.Join(output.rawLines, "\n")
		}
		return result, nil
	}

	// Check for scanner errors (e.g., token too long)
	if output.err != nil {
		slog.Warn("stream scanner error", "error", output.err)
		result := &SpawnResult{
			ExitCode:      1,
			TurnSummaries: output.summaries,
			Output:        strings.Join(output.rawLines, "\n"),
		}
		if output.result != nil {
			result.Output = output.result.Result
			result.Usage = usageFromStreamResult(output.result)
		}
		return result, nil
	}

	// Successful exit
	result := &SpawnResult{
		ExitCode:      0,
		TurnSummaries: output.summaries,
	}
	if output.result != nil {
		result.Output = output.result.Result
		result.Usage = usageFromStreamResult(output.result)
		if output.result.IsError {
			result.ExitCode = 1
		}
	} else {
		// No result message — fallback to raw lines
		raw := strings.Join(output.rawLines, "\n")
		if text, usage, ok := parseJSONOutput(raw); ok {
			result.Output = text
			result.Usage = usage
		} else {
			result.Output = raw
		}
	}
	return result, nil
}

// usageFromStreamResult converts a streamResult's usage into arc.Usage.
func usageFromStreamResult(sr *streamResult) arc.Usage {
	return arc.Usage{
		InputTokens:              sr.Usage.InputTokens,
		OutputTokens:             sr.Usage.OutputTokens,
		CacheCreationInputTokens: sr.Usage.CacheCreationInputTokens,
		CacheReadInputTokens:     sr.Usage.CacheReadInputTokens,
		CostUSD:                  sr.TotalCost,
	}
}

// spawnBuffered runs the subprocess with buffered stdout (legacy approach for
// --output-format json).
func spawnBuffered(ctx context.Context, timeoutCtx context.Context, cmd *exec.Cmd) (*SpawnResult, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	pid := cmd.Process.Pid
	startTime := time.Now()
	slog.Info("agent started", "pid", pid)

	err := cmd.Wait()
	duration := time.Since(startTime)

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}

	slog.Info("agent exited", "pid", pid, "exit_code", exitCode, "duration", duration, "stderr_len", stderr.Len())

	var result *SpawnResult
	if err != nil {
		if timeoutCtx.Err() != nil && ctx.Err() == nil {
			result = &SpawnResult{
				Output:   stdout.String(),
				Stderr:   stderr.String(),
				ExitCode: -1,
				TimedOut: true,
				Duration: duration,
				PID:      pid,
			}
		} else if ctx.Err() != nil {
			return nil, ctx.Err()
		} else {
			result = &SpawnResult{
				Output:   stdout.String(),
				Stderr:   stderr.String(),
				ExitCode: exitCode,
				TimedOut: false,
				Duration: duration,
				PID:      pid,
			}
		}
	} else {
		result = &SpawnResult{
			Output:   stdout.String(),
			Stderr:   stderr.String(),
			ExitCode: 0,
			TimedOut: false,
			Duration: duration,
			PID:      pid,
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
