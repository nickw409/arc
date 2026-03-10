package adapter

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nwiley/arc/internal/arc"
)

// buildFakeCodex writes a shell script that behaves like a minimal codex CLI
// and returns the path to the script.  The script is placed in a temp dir
// that is prepended to PATH so that exec.LookPath("codex") finds it.
func buildFakeCodex(t *testing.T, script string) (binaryPath string, restorePATH func()) {
	t.Helper()
	dir := t.TempDir()
	binaryPath = filepath.Join(dir, "codex")
	if err := os.WriteFile(binaryPath, []byte(script), 0755); err != nil {
		t.Fatalf("writing fake codex: %v", err)
	}
	orig := os.Getenv("PATH")
	t.Setenv("PATH", dir+":"+orig)
	return binaryPath, func() { os.Setenv("PATH", orig) }
}

// TestCodexAdapterName verifies the adapter identifier.
func TestCodexAdapterName(t *testing.T) {
	a := &CodexAdapter{}
	if a.Name() != "codex" {
		t.Fatalf("Name()=%q, want %q", a.Name(), "codex")
	}
}


// TestCodexAdapterSpawnSuccess verifies a normal successful run.
func TestCodexAdapterSpawnSuccess(t *testing.T) {
	buildFakeCodex(t, "#!/bin/sh\necho 'codex output'\nexit 0\n")

	a := &CodexAdapter{}
	res, err := a.Spawn(context.Background(), "do something", t.TempDir(), arc.SessionConfig{
		Timeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil AgentResult")
	}
	if res.ExitCode != 0 {
		t.Fatalf("exit code %d, want 0", res.ExitCode)
	}
	if !strings.Contains(res.Output, "codex output") {
		t.Fatalf("output %q, want to contain %q", res.Output, "codex output")
	}
	if res.Duration <= 0 {
		t.Fatalf("duration %v, want > 0", res.Duration)
	}
}

// TestCodexAdapterSpawnNonZeroExit verifies that a non-zero exit code is
// propagated correctly.
func TestCodexAdapterSpawnNonZeroExit(t *testing.T) {
	buildFakeCodex(t, "#!/bin/sh\necho 'error'\nexit 3\n")

	a := &CodexAdapter{}
	res, err := a.Spawn(context.Background(), "fail please", t.TempDir(), arc.SessionConfig{
		Timeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ExitCode != 3 {
		t.Fatalf("exit code %d, want 3", res.ExitCode)
	}
}

// TestCodexAdapterSpawnUsageZero verifies that usage is always zero (codex
// does not report token counts).
func TestCodexAdapterSpawnUsageZero(t *testing.T) {
	buildFakeCodex(t, "#!/bin/sh\nexit 0\n")

	a := &CodexAdapter{}
	res, err := a.Spawn(context.Background(), "prompt", t.TempDir(), arc.SessionConfig{
		Timeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Usage.IsZero() {
		t.Fatalf("expected zero usage, got %+v", res.Usage)
	}
}

// TestCodexAdapterSpawnTimeout verifies that the adapter sets TimedOut when
// the process exceeds the configured timeout.
func TestCodexAdapterSpawnTimeout(t *testing.T) {
	buildFakeCodex(t, "#!/bin/sh\nsleep 60\n")

	a := &CodexAdapter{}
	res, err := a.Spawn(context.Background(), "prompt", t.TempDir(), arc.SessionConfig{
		Timeout: 150 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("unexpected error for timeout: %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil result for timeout")
	}
	if !res.TimedOut {
		t.Fatalf("expected TimedOut=true, got TimedOut=%v", res.TimedOut)
	}
	if res.ExitCode != -1 {
		t.Fatalf("exit code %d, want -1 for timeout", res.ExitCode)
	}
}

// TestCodexAdapterSpawnDefaultTimeout verifies that a zero Timeout falls back
// to the 30-minute default (we just check the run succeeds; we cannot wait
// 30 minutes in a test, so we only verify no error is returned for fast scripts).
func TestCodexAdapterSpawnDefaultTimeout(t *testing.T) {
	buildFakeCodex(t, "#!/bin/sh\necho ok\nexit 0\n")

	a := &CodexAdapter{}
	// Passing zero timeout — should use defaultCodexTimeout internally.
	res, err := a.Spawn(context.Background(), "prompt", t.TempDir(), arc.SessionConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("exit code %d, want 0", res.ExitCode)
	}
}

// TestCodexAdapterSpawnWithModel verifies that the --model flag is forwarded
// when cfg.Model is set.
func TestCodexAdapterSpawnWithModel(t *testing.T) {
	// Script prints its arguments so we can inspect them.
	buildFakeCodex(t, "#!/bin/sh\necho \"args: $@\"\nexit 0\n")

	a := &CodexAdapter{}
	res, err := a.Spawn(context.Background(), "prompt", t.TempDir(), arc.SessionConfig{
		Timeout: 10 * time.Second,
		Model:   "codex-mini",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(res.Output, "--model") {
		t.Fatalf("output %q should contain --model flag", res.Output)
	}
	if !strings.Contains(res.Output, "codex-mini") {
		t.Fatalf("output %q should contain model name", res.Output)
	}
}

// TestCodexAdapterSpawnModelOmittedWhenEmpty verifies that the --model flag
// is NOT added when cfg.Model is empty.
func TestCodexAdapterSpawnModelOmittedWhenEmpty(t *testing.T) {
	buildFakeCodex(t, "#!/bin/sh\necho \"args: $@\"\nexit 0\n")

	a := &CodexAdapter{}
	res, err := a.Spawn(context.Background(), "prompt", t.TempDir(), arc.SessionConfig{
		Timeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(res.Output, "--model") {
		t.Fatalf("output %q should NOT contain --model flag when Model is empty", res.Output)
	}
}

// TestCodexAdapterSpawnStderr verifies that stderr is captured.
func TestCodexAdapterSpawnStderr(t *testing.T) {
	buildFakeCodex(t, "#!/bin/sh\necho 'err output' >&2\nexit 0\n")

	a := &CodexAdapter{}
	res, err := a.Spawn(context.Background(), "prompt", t.TempDir(), arc.SessionConfig{
		Timeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(res.Stderr, "err output") {
		t.Fatalf("Stderr=%q, want to contain %q", res.Stderr, "err output")
	}
}

// TestCodexAdapterSpawnPromptPassedAsFlag verifies that the prompt is
// forwarded via the --prompt flag.
func TestCodexAdapterSpawnPromptPassedAsFlag(t *testing.T) {
	// Print args so we can confirm --prompt and its value appear.
	buildFakeCodex(t, "#!/bin/sh\necho \"args: $@\"\nexit 0\n")

	a := &CodexAdapter{}
	res, err := a.Spawn(context.Background(), "my task prompt", t.TempDir(), arc.SessionConfig{
		Timeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(res.Output, "--prompt") {
		t.Fatalf("output %q should contain --prompt flag", res.Output)
	}
	if !strings.Contains(res.Output, "my task prompt") {
		t.Fatalf("output %q should contain the prompt text", res.Output)
	}
}

// TestCodexAdapterSpawnRequiredFlags verifies that --quiet and
// --approval-mode full-auto are always passed.
func TestCodexAdapterSpawnRequiredFlags(t *testing.T) {
	buildFakeCodex(t, "#!/bin/sh\necho \"args: $@\"\nexit 0\n")

	a := &CodexAdapter{}
	res, err := a.Spawn(context.Background(), "prompt", t.TempDir(), arc.SessionConfig{
		Timeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(res.Output, "--quiet") {
		t.Fatalf("output %q should contain --quiet", res.Output)
	}
	if !strings.Contains(res.Output, "--approval-mode") {
		t.Fatalf("output %q should contain --approval-mode", res.Output)
	}
	if !strings.Contains(res.Output, "full-auto") {
		t.Fatalf("output %q should contain full-auto", res.Output)
	}
}

// TestCodexAdapterSpawnWorkdirSet verifies that the working directory is
// applied to the subprocess (script prints pwd to confirm).
func TestCodexAdapterSpawnWorkdirSet(t *testing.T) {
	buildFakeCodex(t, "#!/bin/sh\npwd\nexit 0\n")

	workdir := t.TempDir()
	a := &CodexAdapter{}
	res, err := a.Spawn(context.Background(), "prompt", workdir, arc.SessionConfig{
		Timeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	trimmed := strings.TrimSpace(res.Output)
	// On macOS /tmp may be a symlink to /private/tmp; use filepath.EvalSymlinks
	// on both sides if needed. For CI (Linux), direct comparison is sufficient.
	if !strings.HasSuffix(trimmed, filepath.Base(workdir)) {
		t.Fatalf("pwd output %q should end with workdir base %q", trimmed, filepath.Base(workdir))
	}
}

// TestCodexAdapterRegistered verifies that "codex" is available in the Registry.
func TestCodexAdapterRegistered(t *testing.T) {
	if _, ok := Registry["codex"]; !ok {
		t.Fatal("Registry should contain 'codex'")
	}
	a := Get("codex")
	if a == nil {
		t.Fatal("Get('codex') returned nil")
	}
	if _, ok := a.(*CodexAdapter); !ok {
		t.Fatalf("expected *CodexAdapter, got %T", a)
	}
	if a.Name() != "codex" {
		t.Fatalf("Name()=%q, want %q", a.Name(), "codex")
	}
}

// TestCodexAdapterRegistryMultipleCalls verifies that Get("codex") can be
// called multiple times without error and returns a usable adapter each time.
func TestCodexAdapterRegistryMultipleCalls(t *testing.T) {
	a1 := Get("codex")
	a2 := Get("codex")
	// Both must be non-nil and have the correct Name.
	if a1 == nil || a2 == nil {
		t.Fatal("Get should return non-nil adapter")
	}
	if a1.Name() != "codex" || a2.Name() != "codex" {
		t.Fatalf("Name()=%q/%q, both want %q", a1.Name(), a2.Name(), "codex")
	}
}
