// Package testcmd centralizes test command resolution and execution.
//
// All other packages should use this instead of hardcoding test commands
// or re-implementing resolution logic. The resolution priority is:
//
//  1. Explicit override (passed by caller)
//  2. Config (.arc.yaml test_command)
//  3. Intelligence store (per-package known commands)
//  4. Project detection (language-based default)
//  5. Fallback ("go test ./...")
package testcmd

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/nwiley/arc/internal/config"
	"github.com/nwiley/arc/internal/intelligence"
	"github.com/nwiley/arc/internal/project"
)

// Result holds the output and parsed metadata from a test run.
type Result struct {
	Passed      bool
	Output      string
	ExitCode    int
	FailedTests []string
	Duration    time.Duration
}

// Env holds the resolved testing environment for a project.
// Build one with NewEnv, then call methods on it.
type Env struct {
	// Command is the resolved full-project test command (e.g. "go test ./...").
	Command  string
	Language string
	Runner   string
	Dir      string
	intel    *intelligence.Store
}

// EnvOption configures how the Env is built.
type EnvOption func(*envOptions)

type envOptions struct {
	override   string
	config     *config.Config
	projectDir string
}

// WithOverride forces a specific test command, bypassing all resolution.
func WithOverride(cmd string) EnvOption {
	return func(o *envOptions) { o.override = cmd }
}

// WithConfig provides the project config for resolution.
func WithConfig(cfg *config.Config) EnvOption {
	return func(o *envOptions) { o.config = cfg }
}

// WithProjectDir sets the project root for detection and intelligence.
func WithProjectDir(dir string) EnvOption {
	return func(o *envOptions) { o.projectDir = dir }
}

// NewEnv resolves the testing environment. Resolution order:
// override > config.TestCommand > project detection > "go test ./..."
func NewEnv(opts ...EnvOption) *Env {
	var o envOptions
	for _, opt := range opts {
		opt(&o)
	}

	env := &Env{Dir: o.projectDir}

	// Try to open intelligence store (best-effort).
	if o.projectDir != "" {
		if s, err := intelligence.Open(o.projectDir); err == nil {
			env.intel = s
		}
	}

	// 1. Explicit override
	if o.override != "" {
		env.Command = o.override
		if o.config != nil {
			env.Language = o.config.Language
			env.Runner = o.config.Runner
		}
		return env
	}

	// 2. Config
	if o.config != nil {
		env.Language = o.config.Language
		env.Runner = o.config.Runner
		if o.config.TestCommand != "" {
			env.Command = o.config.TestCommand
			return env
		}
	}

	// 3. Project detection
	if o.projectDir != "" {
		det := project.Detect(o.projectDir)
		if env.Language == "" {
			env.Language = det.Language
		}
		if env.Runner == "" {
			env.Runner = det.Runner
		}
		if det.TestCommand != "" {
			env.Command = det.TestCommand
			return env
		}
	}

	// 4. Fallback
	env.Command = "go test ./..."
	return env
}

// RunAll runs the full project test suite.
func (e *Env) RunAll(ctx context.Context) (*Result, error) {
	return e.run(ctx, e.Command, e.Dir, 10*time.Minute)
}

// RunFile runs tests for a specific file. It derives the appropriate command
// from the file path and the environment's language/runner.
func (e *Env) RunFile(ctx context.Context, testFile string) (*Result, error) {
	cmd := e.commandForFile(testFile)
	return e.run(ctx, cmd, e.Dir, 5*time.Minute)
}

// RunPackage runs tests for a specific package path (e.g. "./internal/foo/").
func (e *Env) RunPackage(ctx context.Context, pkg string) (*Result, error) {
	cmd := e.commandForPackage(pkg)
	return e.run(ctx, cmd, e.Dir, 5*time.Minute)
}

// RunCommand runs an arbitrary test command string.
func (e *Env) RunCommand(ctx context.Context, cmd string) (*Result, error) {
	return e.run(ctx, cmd, e.Dir, 10*time.Minute)
}

// CommandForFile returns the test command that would be used for a specific file
// without executing it.
func (e *Env) CommandForFile(testFile string) string {
	return e.commandForFile(testFile)
}

// commandForFile derives a scoped test command from a file path.
func (e *Env) commandForFile(testFile string) string {
	// Check intelligence store for a known command for this file's package.
	if e.intel != nil {
		pkg := filepath.Dir(testFile)
		if cmd := e.intel.TestCommandFor(pkg); cmd != "" {
			return cmd
		}
	}

	lang := e.Language
	if lang == "" {
		lang = detectLanguageFromFile(testFile)
	}

	switch lang {
	case "go":
		return goTestForFile(testFile)
	case "rust":
		return fmt.Sprintf("cargo test --test %s", filepath.Base(strings.TrimSuffix(testFile, ".rs")))
	case "typescript":
		return fmt.Sprintf("npx vitest run %s", testFile)
	case "python":
		return fmt.Sprintf("pytest %s -v", testFile)
	default:
		// Fall back: run the full suite (better than nothing).
		return e.Command
	}
}

// commandForPackage derives a scoped test command from a package path.
func (e *Env) commandForPackage(pkg string) string {
	// Check intelligence store first.
	if e.intel != nil {
		if cmd := e.intel.TestCommandFor(pkg); cmd != "" {
			return cmd
		}
	}

	lang := e.Language
	switch lang {
	case "go":
		p := pkg
		if !strings.HasPrefix(p, "./") {
			p = "./" + p
		}
		if !strings.HasSuffix(p, "/") {
			p += "/"
		}
		return fmt.Sprintf("go test %s -count=1", p)
	case "rust":
		return fmt.Sprintf("cargo test -p %s", filepath.Base(pkg))
	case "typescript":
		return fmt.Sprintf("npx vitest run %s", pkg)
	case "python":
		return fmt.Sprintf("pytest %s -v", pkg)
	default:
		return e.Command
	}
}

func (e *Env) run(ctx context.Context, command, dir string, defaultTimeout time.Duration) (*Result, error) {
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultTimeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	if dir != "" {
		cmd.Dir = dir
	}

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	start := time.Now()
	runErr := cmd.Run()
	duration := time.Since(start)

	exitCode := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("executing test command %q: %w", command, runErr)
		}
	}

	output := buf.String()
	failedTests := ParseFailures(output)

	return &Result{
		Passed:      exitCode == 0,
		Output:      output,
		ExitCode:    exitCode,
		FailedTests: failedTests,
		Duration:    duration,
	}, nil
}

// ParseFailures extracts failing test names from test output.
// Currently supports Go test output ("--- FAIL: TestName").
func ParseFailures(output string) []string {
	var tests []string
	seen := make(map[string]bool)
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "--- FAIL: ") {
			rest := strings.TrimPrefix(line, "--- FAIL: ")
			name := rest
			if idx := strings.Index(rest, " "); idx >= 0 {
				name = rest[:idx]
			}
			if name != "" && !seen[name] {
				seen[name] = true
				tests = append(tests, name)
			}
		}
	}
	return deduplicateSubtests(tests)
}

// goTestForFile builds "go test ./path/to/pkg/ -v -count=1" from a test file path.
func goTestForFile(testFile string) string {
	dir := filepath.ToSlash(filepath.Dir(testFile))
	var pkg string
	switch {
	case filepath.IsAbs(testFile):
		pkg = dir + "/"
	case dir == ".":
		pkg = "./"
	default:
		pkg = "./" + dir + "/"
	}
	return fmt.Sprintf("go test %s -v -count=1", pkg)
}

func detectLanguageFromFile(path string) string {
	switch {
	case strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, ".go"):
		return "go"
	case strings.HasSuffix(path, ".rs"):
		return "rust"
	case strings.HasSuffix(path, ".test.ts") || strings.HasSuffix(path, ".test.js") ||
		strings.HasSuffix(path, ".spec.ts") || strings.HasSuffix(path, ".spec.js"):
		return "typescript"
	case strings.HasSuffix(path, "_test.py") || strings.HasSuffix(path, "test_") ||
		strings.HasPrefix(filepath.Base(path), "test_"):
		return "python"
	default:
		return ""
	}
}

var goFailRe = regexp.MustCompile(`--- FAIL: (\S+)`)

// deduplicateSubtests removes parent test names when a subtest is present.
func deduplicateSubtests(names []string) []string {
	if len(names) <= 1 {
		return names
	}
	var result []string
	for _, n := range names {
		isParent := false
		for _, other := range names {
			if other != n && strings.HasPrefix(other, n+"/") {
				isParent = true
				break
			}
		}
		if !isParent {
			result = append(result, n)
		}
	}
	return result
}
