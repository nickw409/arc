// mockagent is a test binary that simulates the Claude CLI for spawn tests.
// Behavior is controlled via environment variables:
//   - MOCK_OUTPUT: string to print to stdout (default: reads stdin and echoes it)
//   - MOCK_EXIT_CODE: exit code (default: 0)
//   - MOCK_SLEEP_MS: milliseconds to sleep before output (default: 0)
//   - MOCK_STDERR: string to print to stderr (default: empty)
//   - MOCK_ECHO_STDIN: if "1", reads stdin and echoes it to stdout (overrides MOCK_OUTPUT)
//   - MOCK_ECHO_ARGS: if "1", prints os.Args[1:] joined by newlines to stdout (overrides MOCK_OUTPUT)
//   - MOCK_JSON_WRAP: if "1", wraps MOCK_OUTPUT in a Claude CLI JSON envelope with hardcoded usage
//   - MOCK_SCRIPT_DIR: directory containing call_0.txt, call_1.txt, etc. for sequential scripted
//     responses. A shared .call_count file tracks which response to serve next. Falls through to
//     MOCK_OUTPUT if the script file doesn't exist.
//   - MOCK_STREAM_JSON: multi-line value with stream-json messages to emit line by line.
//     Each line is emitted with a configurable delay (MOCK_STREAM_DELAY_MS, default 0).
//     If set, overrides MOCK_OUTPUT.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func flock(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
}

func funlock(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}

func main() {
	sleepMS, _ := strconv.Atoi(os.Getenv("MOCK_SLEEP_MS"))
	if sleepMS > 0 {
		time.Sleep(time.Duration(sleepMS) * time.Millisecond)
	}

	if stderr := os.Getenv("MOCK_STDERR"); stderr != "" {
		fmt.Fprint(os.Stderr, stderr)
	}

	// MOCK_STREAM_JSON: emit stream-json lines one at a time with optional delay
	if streamJSON := os.Getenv("MOCK_STREAM_JSON"); streamJSON != "" {
		delayMS, _ := strconv.Atoi(os.Getenv("MOCK_STREAM_DELAY_MS"))
		lines := strings.Split(streamJSON, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if delayMS > 0 {
				time.Sleep(time.Duration(delayMS) * time.Millisecond)
			}
			fmt.Println(line)
		}
		exitCode, _ := strconv.Atoi(os.Getenv("MOCK_EXIT_CODE"))
		os.Exit(exitCode)
	}

	var output string
	if os.Getenv("MOCK_ECHO_ARGS") == "1" {
		output = strings.Join(os.Args[1:], "\n")
	} else if os.Getenv("MOCK_ECHO_STDIN") == "1" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading stdin: %v\n", err)
			os.Exit(1)
		}
		output = string(data)
	} else if scriptDir := os.Getenv("MOCK_SCRIPT_DIR"); scriptDir != "" {
		output = getScriptedResponse(scriptDir)
	} else {
		output = os.Getenv("MOCK_OUTPUT")
	}

	if os.Getenv("MOCK_JSON_WRAP") == "1" && output != "" {
		envelope := map[string]interface{}{
			"result":         output,
			"total_cost_usd": 0.001,
			"usage": map[string]int{
				"input_tokens":                10,
				"output_tokens":               5,
				"cache_creation_input_tokens":  2,
				"cache_read_input_tokens":      3,
			},
		}
		data, _ := json.Marshal(envelope)
		fmt.Print(string(data))
	} else if output != "" {
		fmt.Print(output)
	}

	exitCode, _ := strconv.Atoi(os.Getenv("MOCK_EXIT_CODE"))
	os.Exit(exitCode)
}

func getScriptedResponse(scriptDir string) string {
	counterPath := filepath.Join(scriptDir, ".call_count")
	lockPath := counterPath + ".lock"

	// Acquire file lock for atomic counter increment.
	// This prevents races when parallel branches spawn concurrent mock agents.
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening lock file: %v\n", err)
		os.Exit(1)
	}
	defer lockFile.Close()
	if err := flock(lockFile); err != nil {
		fmt.Fprintf(os.Stderr, "error acquiring lock: %v\n", err)
		os.Exit(1)
	}
	defer funlock(lockFile)

	// Read current counter (default 0)
	count := 0
	if data, err := os.ReadFile(counterPath); err == nil {
		count, _ = strconv.Atoi(strings.TrimSpace(string(data)))
	}

	// Increment and write back
	if err := os.WriteFile(counterPath, []byte(strconv.Itoa(count+1)), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing call count: %v\n", err)
		os.Exit(1)
	}

	// Read scripted response file
	scriptFile := filepath.Join(scriptDir, fmt.Sprintf("call_%d.txt", count))
	data, err := os.ReadFile(scriptFile)
	if err != nil {
		// Fall through to MOCK_OUTPUT
		return os.Getenv("MOCK_OUTPUT")
	}
	return string(data)
}
