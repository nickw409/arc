// mockagent is a test binary that simulates the Claude CLI for spawn tests.
// Behavior is controlled via environment variables:
//   - MOCK_OUTPUT: string to print to stdout (default: reads stdin and echoes it)
//   - MOCK_EXIT_CODE: exit code (default: 0)
//   - MOCK_SLEEP_MS: milliseconds to sleep before output (default: 0)
//   - MOCK_STDERR: string to print to stderr (default: empty)
//   - MOCK_ECHO_STDIN: if "1", reads stdin and echoes it to stdout (overrides MOCK_OUTPUT)
//   - MOCK_ECHO_ARGS: if "1", prints os.Args[1:] joined by newlines to stdout (overrides MOCK_OUTPUT)
package main

import (
	"fmt"
	"io"
	"os"
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
	} else if output := os.Getenv("MOCK_OUTPUT"); output != "" {
		fmt.Print(output)
	}

	exitCode, _ := strconv.Atoi(os.Getenv("MOCK_EXIT_CODE"))
	os.Exit(exitCode)
}
