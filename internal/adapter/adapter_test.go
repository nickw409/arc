package adapter

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nwiley/arc/internal/arc"
)

// buildMockAgent compiles the mockagent binary and returns its path.
func buildMockAgent(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "mockagent")
	cmd := exec.Command("go", "build", "-o", bin, "../agent/testdata/mockagent")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build mock: %s\n%s", err, out)
	}
	return bin
}

// --- ClaudeAdapter tests ---

func TestClaudeAdapterSpawn(t *testing.T) {
	bin := buildMockAgent(t)
	t.Setenv("MOCK_OUTPUT", "hello from claude")
	t.Setenv("MOCK_JSON_WRAP", "1")

	a := &ClaudeAdapter{CommandName: bin}
	res, err := a.Spawn(context.Background(), "test prompt", t.TempDir(), arc.SessionConfig{
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
	if res.Output != "hello from claude" {
		t.Fatalf("output %q, want %q", res.Output, "hello from claude")
	}
	if res.Duration <= 0 {
		t.Fatalf("duration %v, want > 0", res.Duration)
	}
	// Usage should be populated from the JSON envelope
	if res.Usage.InputTokens != 10 {
		t.Fatalf("InputTokens=%d, want 10", res.Usage.InputTokens)
	}
	if res.Usage.OutputTokens != 5 {
		t.Fatalf("OutputTokens=%d, want 5", res.Usage.OutputTokens)
	}
	if res.Usage.CostUSD != 0.001 {
		t.Fatalf("CostUSD=%f, want 0.001", res.Usage.CostUSD)
	}
}

func TestClaudeAdapterSpawnExitCode(t *testing.T) {
	bin := buildMockAgent(t)
	t.Setenv("MOCK_EXIT_CODE", "2")
	t.Setenv("MOCK_OUTPUT", "error output")

	a := &ClaudeAdapter{CommandName: bin}
	res, err := a.Spawn(context.Background(), "prompt", t.TempDir(), arc.SessionConfig{
		Timeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ExitCode != 2 {
		t.Fatalf("exit code %d, want 2", res.ExitCode)
	}
}

func TestClaudeAdapterSpawnTimeout(t *testing.T) {
	bin := buildMockAgent(t)
	t.Setenv("MOCK_SLEEP_MS", "5000")

	a := &ClaudeAdapter{CommandName: bin}
	res, err := a.Spawn(context.Background(), "prompt", t.TempDir(), arc.SessionConfig{
		Timeout: 150 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("unexpected error for timeout: %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil AgentResult for timeout")
	}
	if !res.TimedOut {
		t.Fatal("expected TimedOut=true")
	}
	if res.ExitCode != -1 {
		t.Fatalf("exit code %d, want -1 for timeout", res.ExitCode)
	}
}

func TestClaudeAdapterPreflightMissing(t *testing.T) {
	a := &ClaudeAdapter{CommandName: "nonexistent-binary-arc-test-12345"}
	err := a.Preflight(context.Background(), t.TempDir())
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' in error, got: %v", err)
	}
}

func TestClaudeAdapterPreflightExists(t *testing.T) {
	bin := buildMockAgent(t)
	// The mock agent accepts --version by printing nothing and exiting 0
	a := &ClaudeAdapter{CommandName: bin}
	err := a.Preflight(context.Background(), t.TempDir())
	// The mock agent doesn't handle --version specifically, but it exits 0 by default,
	// so preflight should succeed.
	if err != nil {
		t.Fatalf("expected no error for existing binary, got: %v", err)
	}
}

func TestClaudeAdapterPreflightWorkdirMissing(t *testing.T) {
	bin := buildMockAgent(t)
	a := &ClaudeAdapter{CommandName: bin}
	err := a.Preflight(context.Background(), "/nonexistent-arc-test-workdir-12345")
	if err == nil {
		t.Fatal("expected error for missing workdir")
	}
	if !strings.Contains(err.Error(), "not accessible") {
		t.Fatalf("expected 'not accessible' in error, got: %v", err)
	}
}

func TestClaudeAdapterPreflightWorkdirNotWritable(t *testing.T) {
	workdir := t.TempDir()
	// Make the directory read-only.
	if err := os.Chmod(workdir, 0o444); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	// Restore permissions after test so TempDir cleanup works.
	t.Cleanup(func() { os.Chmod(workdir, 0o755) })

	bin := buildMockAgent(t)
	a := &ClaudeAdapter{CommandName: bin}
	err := a.Preflight(context.Background(), workdir)
	if err == nil {
		t.Fatal("expected error for non-writable workdir")
	}
	if !strings.Contains(err.Error(), "not accessible") {
		t.Fatalf("expected 'not accessible' in error, got: %v", err)
	}
}

func TestClaudeAdapterPreflightAuthFailure(t *testing.T) {
	workdir := t.TempDir()
	// Write a script that succeeds for --version but fails for --print.
	scriptPath := filepath.Join(workdir, "fake-claude.sh")
	script := `#!/bin/sh
for arg in "$@"; do
  if [ "$arg" = "--print" ]; then
    echo "Not logged in" >&2
    exit 1
  fi
done
exit 0
`
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("writing script: %v", err)
	}

	a := &ClaudeAdapter{CommandName: scriptPath}
	err := a.Preflight(context.Background(), t.TempDir())
	if err == nil {
		t.Fatal("expected error for auth failure")
	}
	if !strings.Contains(err.Error(), "authentication") {
		t.Fatalf("expected 'authentication' in error, got: %v", err)
	}
}

func TestClaudeAdapterStderr(t *testing.T) {
	bin := buildMockAgent(t)
	t.Setenv("MOCK_OUTPUT", "ok")
	t.Setenv("MOCK_STDERR", "something on stderr")

	a := &ClaudeAdapter{CommandName: bin}
	res, err := a.Spawn(context.Background(), "prompt", t.TempDir(), arc.SessionConfig{
		Timeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Stderr != "something on stderr" {
		t.Fatalf("Stderr=%q, want %q", res.Stderr, "something on stderr")
	}
}

func TestClaudeAdapterNameDefault(t *testing.T) {
	a := &ClaudeAdapter{}
	if a.Name() != "claude" {
		t.Fatalf("Name()=%q, want %q", a.Name(), "claude")
	}
}

// --- GenericAdapter tests ---

func TestGenericAdapterSpawnPromptFile(t *testing.T) {
	// Write a shell script that reads the prompt file and echoes it.
	workdir := t.TempDir()
	scriptPath := filepath.Join(workdir, "myagent.sh")
	script := `#!/bin/sh
cat "$PROMPT_PATH"
`
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("writing script: %v", err)
	}

	// Use PromptFile to write prompt to workdir, then pass path via env in Args.
	// Simpler: just use PromptFlag with a script that echoes the file.
	scriptPath2 := filepath.Join(workdir, "readprompt.sh")
	script2 := `#!/bin/sh
cat "$1"
`
	if err := os.WriteFile(scriptPath2, []byte(script2), 0755); err != nil {
		t.Fatalf("writing script2: %v", err)
	}

	a := &GenericAdapter{
		Name_:      "test",
		Command:    scriptPath2,
		PromptFlag: "", // Will use PromptFile instead
		PromptFile: "prompt.txt",
	}

	// Use a separate workdir for the spawn so we can check the written file.
	spawnDir := t.TempDir()

	// Script reads first arg; since PromptFile is set, no flag is added.
	// We need the script to find the prompt file without a flag argument.
	// Rewrite script to read from fixed path.
	scriptPath3 := filepath.Join(workdir, "readfixed.sh")
	script3 := `#!/bin/sh
cat prompt.txt
`
	if err := os.WriteFile(scriptPath3, []byte(script3), 0755); err != nil {
		t.Fatalf("writing script3: %v", err)
	}

	a2 := &GenericAdapter{
		Name_:      "test",
		Command:    scriptPath3,
		PromptFile: "prompt.txt",
	}

	res, err := a2.Spawn(context.Background(), "hello from prompt file", spawnDir, arc.SessionConfig{
		Timeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("exit code %d, want 0 (stderr: %s)", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Output, "hello from prompt file") {
		t.Fatalf("output %q, want to contain %q", res.Output, "hello from prompt file")
	}

	// Verify the unused variable doesn't cause a compile error.
	_ = a
}

func TestGenericAdapterSpawnPromptFlag(t *testing.T) {
	workdir := t.TempDir()
	// Script: receives prompt file as first arg after flag, echoes its content.
	scriptPath := filepath.Join(workdir, "flagprompt.sh")
	script := `#!/bin/sh
# $1 is "--message-file", $2 is the file path
cat "$2"
`
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("writing script: %v", err)
	}

	a := &GenericAdapter{
		Name_:      "test",
		Command:    scriptPath,
		PromptFlag: "--message-file",
	}

	res, err := a.Spawn(context.Background(), "hello via flag", t.TempDir(), arc.SessionConfig{
		Timeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("exit code %d, want 0 (stderr: %s)", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Output, "hello via flag") {
		t.Fatalf("output %q, want to contain %q", res.Output, "hello via flag")
	}
}

func TestGenericAdapterSpawnStdin(t *testing.T) {
	workdir := t.TempDir()
	// Script that reads stdin and echoes it.
	scriptPath := filepath.Join(workdir, "stdin.sh")
	script := `#!/bin/sh
cat
`
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("writing script: %v", err)
	}

	a := &GenericAdapter{
		Name_:   "test",
		Command: scriptPath,
		// No PromptFile, no PromptFlag → use stdin
	}

	res, err := a.Spawn(context.Background(), "hello via stdin", t.TempDir(), arc.SessionConfig{
		Timeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(res.Output, "hello via stdin") {
		t.Fatalf("output %q, want to contain %q", res.Output, "hello via stdin")
	}
}

func TestGenericAdapterSpawnDuration(t *testing.T) {
	workdir := t.TempDir()
	scriptPath := filepath.Join(workdir, "sleep.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nsleep 0.1\n"), 0755); err != nil {
		t.Fatalf("writing script: %v", err)
	}

	a := &GenericAdapter{
		Name_:   "test",
		Command: scriptPath,
	}

	res, err := a.Spawn(context.Background(), "", t.TempDir(), arc.SessionConfig{
		Timeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Duration < 100*time.Millisecond {
		t.Fatalf("duration %v, want >= 100ms", res.Duration)
	}
}

func TestGenericAdapterSpawnUsageZero(t *testing.T) {
	workdir := t.TempDir()
	scriptPath := filepath.Join(workdir, "noop.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\necho done\n"), 0755); err != nil {
		t.Fatalf("writing script: %v", err)
	}

	a := &GenericAdapter{
		Name_:   "test",
		Command: scriptPath,
	}

	res, err := a.Spawn(context.Background(), "prompt", t.TempDir(), arc.SessionConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Usage.IsZero() {
		t.Fatalf("expected zero usage for generic adapter, got %+v", res.Usage)
	}
}

func TestGenericAdapterPreflightMissing(t *testing.T) {
	a := &GenericAdapter{
		Name_:   "test",
		Command: "nonexistent-binary-arc-test-99999",
	}
	err := a.Preflight(context.Background(), t.TempDir())
	if err == nil {
		t.Fatal("expected error for missing command")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' in error, got: %v", err)
	}
}

func TestGenericAdapterPreflightExists(t *testing.T) {
	a := &GenericAdapter{
		Name_:   "test",
		Command: "sh",
	}
	err := a.Preflight(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("expected no error for 'sh', got: %v", err)
	}
}

func TestGenericAdapterPreflightWorkdirMissing(t *testing.T) {
	a := &GenericAdapter{
		Name_:   "test",
		Command: "sh",
	}
	err := a.Preflight(context.Background(), "/nonexistent-arc-test-workdir-99999")
	if err == nil {
		t.Fatal("expected error for missing workdir")
	}
	if !strings.Contains(err.Error(), "not accessible") {
		t.Fatalf("expected 'not accessible' in error, got: %v", err)
	}
}

func TestGenericAdapterPreflightWorkdirNotWritable(t *testing.T) {
	workdir := t.TempDir()
	if err := os.Chmod(workdir, 0o444); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { os.Chmod(workdir, 0o755) })

	a := &GenericAdapter{
		Name_:   "test",
		Command: "sh",
	}
	err := a.Preflight(context.Background(), workdir)
	if err == nil {
		t.Fatal("expected error for non-writable workdir")
	}
	if !strings.Contains(err.Error(), "not accessible") {
		t.Fatalf("expected 'not accessible' in error, got: %v", err)
	}
}

func TestGenericAdapterPreflightWorkdirIsFile(t *testing.T) {
	// Pass a file path instead of a directory.
	f, err := os.CreateTemp("", "arc-test-*")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })

	a := &GenericAdapter{
		Name_:   "test",
		Command: "sh",
	}
	err = a.Preflight(context.Background(), f.Name())
	if err == nil {
		t.Fatal("expected error when workdir is a file, not a directory")
	}
	if !strings.Contains(err.Error(), "not accessible") {
		t.Fatalf("expected 'not accessible' in error, got: %v", err)
	}
}

func TestGenericAdapterEnvironment(t *testing.T) {
	workdir := t.TempDir()
	scriptPath := filepath.Join(workdir, "env.sh")
	script := `#!/bin/sh
echo "$MY_TEST_VAR"
`
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("writing script: %v", err)
	}

	a := &GenericAdapter{
		Name_:       "test",
		Command:     scriptPath,
		Environment: map[string]string{"MY_TEST_VAR": "custom_env_value"},
	}

	res, err := a.Spawn(context.Background(), "prompt", t.TempDir(), arc.SessionConfig{
		Timeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(res.Output, "custom_env_value") {
		t.Fatalf("output %q, want to contain %q", res.Output, "custom_env_value")
	}
}

// --- Registry tests ---

func TestRegistryGet(t *testing.T) {
	adapter := Get("claude")
	if adapter == nil {
		t.Fatal("expected non-nil adapter for 'claude'")
	}
	if adapter.Name() != "claude" {
		t.Fatalf("Name()=%q, want %q", adapter.Name(), "claude")
	}
	if _, ok := adapter.(*ClaudeAdapter); !ok {
		t.Fatalf("expected *ClaudeAdapter, got %T", adapter)
	}
}

func TestRegistryGetDefault(t *testing.T) {
	adapter := Get("unknown-adapter-xyz")
	if adapter == nil {
		t.Fatal("expected non-nil adapter for unknown name")
	}
	if _, ok := adapter.(*ClaudeAdapter); !ok {
		t.Fatalf("expected *ClaudeAdapter as default, got %T", adapter)
	}
}

func TestRegistryContainsClaude(t *testing.T) {
	if _, ok := Registry["claude"]; !ok {
		t.Fatal("Registry should contain 'claude'")
	}
}

func TestRegistryContainsGeneric(t *testing.T) {
	if _, ok := Registry["generic"]; !ok {
		t.Fatal("Registry should contain 'generic'")
	}
}

func TestRegistryGetGeneric(t *testing.T) {
	a := Get("generic")
	if a == nil {
		t.Fatal("expected non-nil adapter for 'generic'")
	}
	if _, ok := a.(*GenericAdapter); !ok {
		t.Fatalf("expected *GenericAdapter, got %T", a)
	}
	if a.Name() != "generic" {
		t.Fatalf("Name()=%q, want %q", a.Name(), "generic")
	}
}

func TestRegistryConstructorReturnsNewInstance(t *testing.T) {
	a1 := Get("claude")
	a2 := Get("claude")
	if a1 == a2 {
		t.Fatal("Get should return new instances each call")
	}
}

// --- GenericAdapter inactivity watchdog tests ---

func TestGenericAdapterInactivityKill(t *testing.T) {
	workdir := t.TempDir()
	// Script that sleeps long enough to trigger the watchdog.
	scriptPath := filepath.Join(workdir, "sleepy.sh")
	script := "#!/bin/sh\nsleep 60\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("writing script: %v", err)
	}

	// Override watchdog interval and minimum inactivity to tiny values for this test.
	origInterval := genericWatchdogInterval
	origMin := genericInactivityMin
	genericWatchdogInterval = 50 * time.Millisecond
	genericInactivityMin = 200 * time.Millisecond
	defer func() {
		genericWatchdogInterval = origInterval
		genericInactivityMin = origMin
	}()

	a := &GenericAdapter{
		Name_:   "test",
		Command: scriptPath,
	}

	// Total timeout is 10s; inactivity limit = 10s/3 ≈ 3.3s, but we force
	// a very short inactivity by using a tiny total timeout and very small
	// inactivity window. To make this fast in tests we set total timeout
	// equal to 6 minutes so that 1/3 = 2 minutes; that's too slow.
	// Instead we rely on the minimum (2 minutes) being way longer than the
	// script actually produces output — but that would take 2 minutes.
	//
	// Better approach: set total timeout to 3*300ms = 900ms so the
	// inactivity limit = 300ms, which is well below the 60s sleep.
	res, err := a.Spawn(context.Background(), "", t.TempDir(), arc.SessionConfig{
		Timeout: 900 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil result")
	}
	if !res.InactivityKill {
		t.Fatalf("expected InactivityKill=true, got TimedOut=%v InactivityKill=%v", res.TimedOut, res.InactivityKill)
	}
}

func TestGenericAdapterNoInactivityKillOnOutput(t *testing.T) {
	workdir := t.TempDir()
	// Script that produces output continuously, should NOT be inactivity-killed.
	scriptPath := filepath.Join(workdir, "chatty.sh")
	script := "#!/bin/sh\nfor i in 1 2 3 4 5; do echo line$i; done\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("writing script: %v", err)
	}

	origInterval := genericWatchdogInterval
	origMin := genericInactivityMin
	genericWatchdogInterval = 10 * time.Millisecond
	genericInactivityMin = 100 * time.Millisecond
	defer func() {
		genericWatchdogInterval = origInterval
		genericInactivityMin = origMin
	}()

	a := &GenericAdapter{
		Name_:   "test",
		Command: scriptPath,
	}

	res, err := a.Spawn(context.Background(), "", t.TempDir(), arc.SessionConfig{
		Timeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.InactivityKill {
		t.Fatal("expected InactivityKill=false for fast-completing process")
	}
	if res.TimedOut {
		t.Fatal("expected TimedOut=false")
	}
	if res.ExitCode != 0 {
		t.Fatalf("exit code %d, want 0", res.ExitCode)
	}
}
