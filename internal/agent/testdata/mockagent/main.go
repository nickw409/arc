// mockagent is a test binary that simulates the Claude CLI for spawn tests.
// Behavior is controlled via environment variables:
//   - MOCK_OUTPUT: string to print to stdout (default: reads stdin and echoes it)
//   - MOCK_EXIT_CODE: exit code (default: 0)
//   - MOCK_SLEEP_MS: milliseconds to sleep before output (default: 0)
//   - MOCK_STDERR: string to print to stderr (default: empty)
//   - MOCK_ECHO_STDIN: if "1", reads stdin and echoes it to stdout (overrides MOCK_OUTPUT)
//   - MOCK_ECHO_ARGS: if "1", prints os.Args[1:] joined by newlines to stdout (overrides MOCK_OUTPUT)
//   - MOCK_SCRIPT_DIR: directory containing call_0.txt, call_1.txt, etc. for sequential scripted
//     responses. A shared .call_count file tracks which response to serve next. Falls through to
//     MOCK_OUTPUT if the script file doesn't exist.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func main() {
	sleepMS, _ := strconv.Atoi(os.Getenv("MOCK_SLEEP_MS"))
	if sleepMS > 0 {
		time.Sleep(time.Duration(sleepMS) * time.Millisecond)
	}

	if stderr := os.Getenv("MOCK_STDERR"); stderr != "" {
		fmt.Fprint(os.Stderr, stderr)
	}

	if os.Getenv("MOCK_ECHO_ARGS") == "1" {
		fmt.Print(strings.Join(os.Args[1:], "\n"))
	} else if os.Getenv("MOCK_ECHO_STDIN") == "1" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading stdin: %v\n", err)
			os.Exit(1)
		}
		fmt.Print(string(data))
	} else if scriptDir := os.Getenv("MOCK_SCRIPT_DIR"); scriptDir != "" {
		printScriptedResponse(scriptDir)
	} else if output := os.Getenv("MOCK_OUTPUT"); output != "" {
		fmt.Print(output)
	}

	exitCode, _ := strconv.Atoi(os.Getenv("MOCK_EXIT_CODE"))
	os.Exit(exitCode)
}

func printScriptedResponse(scriptDir string) {
	counterPath := filepath.Join(scriptDir, ".call_count")

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
		if output := os.Getenv("MOCK_OUTPUT"); output != "" {
			fmt.Print(output)
		}
		return
	}
	fmt.Print(string(data))
}
