package daemon

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func testScheduler() *Scheduler {
	return NewScheduler(3, func(ctx context.Context, reg *PlanRegistration, phaseName string) error {
		return nil
	}, func(reg *PlanRegistration) {})
}

func tempDaemonConfig(t *testing.T) DaemonConfig {
	t.Helper()
	dir := t.TempDir()
	return DaemonConfig{
		SocketPath:  filepath.Join(dir, "daemon.sock"),
		PIDPath:     filepath.Join(dir, "daemon.pid"),
		LockPath:    filepath.Join(dir, "daemon.lock"),
		StatePath:   filepath.Join(dir, "daemon-state.json"),
		MaxParallel: 3,
	}
}

func TestNew(t *testing.T) {
	cfg := tempDaemonConfig(t)
	sched := testScheduler()
	d := New(cfg, sched)
	if d.cfg.SocketPath != cfg.SocketPath {
		t.Errorf("expected socket path %s, got %s", cfg.SocketPath, d.cfg.SocketPath)
	}
	if d.sched != sched {
		t.Error("scheduler not set")
	}
}

func TestStartAndShutdown(t *testing.T) {
	cfg := tempDaemonConfig(t)
	sched := testScheduler()
	d := New(cfg, sched)

	ctx, cancel := context.WithCancel(context.Background())

	connCh := make(chan net.Conn, 1)
	handler := func(conn net.Conn) {
		connCh <- conn
		conn.Close()
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- d.Start(ctx, handler)
	}()

	// Wait for socket to be ready
	waitForSocket(t, cfg.SocketPath, 3*time.Second)

	// Verify PID file
	pidData, err := os.ReadFile(cfg.PIDPath)
	if err != nil {
		t.Fatalf("reading PID file: %v", err)
	}
	pid, err := strconv.Atoi(string(pidData))
	if err != nil {
		t.Fatalf("parsing PID: %v", err)
	}
	if pid != os.Getpid() {
		t.Errorf("PID mismatch: got %d, want %d", pid, os.Getpid())
	}

	// Verify socket accepts connections
	conn, err := net.DialTimeout("unix", cfg.SocketPath, time.Second)
	if err != nil {
		t.Fatalf("connecting to daemon: %v", err)
	}
	conn.Close()

	// Wait for handler to receive connection
	select {
	case <-connCh:
	case <-time.After(time.Second):
		t.Error("handler did not receive connection")
	}

	// Cancel context to trigger shutdown
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Start returned error: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Start did not return after cancel")
	}

	// Verify cleanup
	if _, err := os.Stat(cfg.SocketPath); !os.IsNotExist(err) {
		t.Error("socket file not removed after shutdown")
	}
	if _, err := os.Stat(cfg.PIDPath); !os.IsNotExist(err) {
		t.Error("PID file not removed after shutdown")
	}
}

func TestStartAlreadyRunning(t *testing.T) {
	cfg := tempDaemonConfig(t)

	// Start a real listener on the socket to simulate an already-running daemon
	ln, err := net.Listen("unix", cfg.SocketPath)
	if err != nil {
		t.Fatalf("creating listener: %v", err)
	}
	defer ln.Close()

	d := New(cfg, testScheduler())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = d.Start(ctx, func(net.Conn) {})
	if err == nil {
		t.Fatal("expected error when daemon already running")
	}
	if !strings.Contains(err.Error(), "already running") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStartStaleSocket(t *testing.T) {
	cfg := tempDaemonConfig(t)

	// Create a stale socket file (listener closed but file remains)
	ln, err := net.Listen("unix", cfg.SocketPath)
	if err != nil {
		t.Fatal(err)
	}
	ln.Close()

	d := New(cfg, testScheduler())
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- d.Start(ctx, func(conn net.Conn) { conn.Close() })
	}()

	waitForSocket(t, cfg.SocketPath, 3*time.Second)

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Start returned error: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Start did not return")
	}
}

func TestFlockPreventsDoubleStart(t *testing.T) {
	cfg := tempDaemonConfig(t)

	d1 := New(cfg, testScheduler())
	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()

	errCh := make(chan error, 1)
	go func() {
		errCh <- d1.Start(ctx1, func(conn net.Conn) { conn.Close() })
	}()

	waitForSocket(t, cfg.SocketPath, 3*time.Second)

	// Second daemon should fail (either flock or socket probe)
	d2 := New(cfg, testScheduler())
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	err := d2.Start(ctx2, func(net.Conn) {})
	if err == nil {
		t.Fatal("expected error starting second daemon")
	}

	cancel1()
	<-errCh
}

func TestShutdownIdempotent(t *testing.T) {
	cfg := tempDaemonConfig(t)
	d := New(cfg, testScheduler())

	if err := d.Shutdown(); err != nil {
		t.Fatalf("first shutdown: %v", err)
	}
	if err := d.Shutdown(); err != nil {
		t.Fatalf("second shutdown: %v", err)
	}
}

func TestDefaultDaemonConfig(t *testing.T) {
	cfg := DefaultDaemonConfig()
	if cfg.MaxParallel != 3 {
		t.Errorf("expected MaxParallel 3, got %d", cfg.MaxParallel)
	}
	if cfg.SocketPath == "" {
		t.Error("SocketPath should not be empty")
	}
	if !filepath.IsAbs(cfg.SocketPath) {
		t.Error("SocketPath should be absolute")
	}
	if !strings.HasSuffix(cfg.SocketPath, "daemon.sock") {
		t.Errorf("SocketPath should end with daemon.sock, got %s", cfg.SocketPath)
	}
	if !strings.HasSuffix(cfg.PIDPath, "daemon.pid") {
		t.Errorf("PIDPath should end with daemon.pid, got %s", cfg.PIDPath)
	}
	if !strings.HasSuffix(cfg.LockPath, "daemon.lock") {
		t.Errorf("LockPath should end with daemon.lock, got %s", cfg.LockPath)
	}
	if !strings.HasSuffix(cfg.StatePath, "daemon-state.json") {
		t.Errorf("StatePath should end with daemon-state.json, got %s", cfg.StatePath)
	}
}

func TestLoadDaemonConfig_NoFile(t *testing.T) {
	cfg, err := LoadDaemonConfig()
	if err != nil {
		t.Fatalf("LoadDaemonConfig: %v", err)
	}
	if cfg.MaxParallel < 1 {
		t.Error("expected positive MaxParallel")
	}
	if cfg.SocketPath == "" {
		t.Error("expected non-empty SocketPath")
	}
}

func TestLoadDaemonConfig_YAMLParsing(t *testing.T) {
	// Test that DaemonConfig YAML tags work correctly by round-tripping
	content := []byte("socket_path: /tmp/test.sock\nmax_parallel: 8\n")

	var cfg DaemonConfig
	// Use yaml import from the daemon package (gopkg.in/yaml.v3)
	if err := yamlUnmarshalForTest(content, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if cfg.SocketPath != "/tmp/test.sock" {
		t.Errorf("expected /tmp/test.sock, got %s", cfg.SocketPath)
	}
	if cfg.MaxParallel != 8 {
		t.Errorf("expected 8, got %d", cfg.MaxParallel)
	}
	if cfg.PIDPath != "" {
		t.Errorf("expected empty PIDPath, got %s", cfg.PIDPath)
	}
}

func TestStartedAt(t *testing.T) {
	cfg := tempDaemonConfig(t)
	d := New(cfg, testScheduler())

	if !d.StartedAt().IsZero() {
		t.Error("StartedAt should be zero before Start")
	}

	ctx, cancel := context.WithCancel(context.Background())
	before := time.Now()

	go func() {
		d.Start(ctx, func(conn net.Conn) { conn.Close() })
	}()

	waitForSocket(t, cfg.SocketPath, 3*time.Second)

	after := time.Now()
	started := d.StartedAt()
	if started.Before(before) || started.After(after) {
		t.Errorf("StartedAt %v not between %v and %v", started, before, after)
	}

	cancel()
	time.Sleep(200 * time.Millisecond)
}

func TestMultipleConnections(t *testing.T) {
	cfg := tempDaemonConfig(t)
	d := New(cfg, testScheduler())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	count := make(chan struct{}, 10)
	go func() {
		d.Start(ctx, func(conn net.Conn) {
			count <- struct{}{}
			conn.Close()
		})
	}()

	waitForSocket(t, cfg.SocketPath, 3*time.Second)

	// Open multiple connections
	for i := 0; i < 5; i++ {
		conn, err := net.DialTimeout("unix", cfg.SocketPath, time.Second)
		if err != nil {
			t.Fatalf("connection %d: %v", i, err)
		}
		conn.Close()
	}

	// Wait for all handlers to fire
	deadline := time.After(3 * time.Second)
	received := 0
	for received < 5 {
		select {
		case <-count:
			received++
		case <-deadline:
			t.Fatalf("only received %d/5 connections", received)
		}
	}
}

func waitForSocket(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("unix", path, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("socket %s did not become ready within %v", path, timeout)
}

// yamlUnmarshalForTest uses the yaml package to unmarshal config for testing.
// This avoids importing yaml.v3 a second time in the test.
func yamlUnmarshalForTest(data []byte, v *DaemonConfig) error {
	// Write to temp file and use LoadDaemonConfig-style parsing
	// Instead, directly use the exported yaml tags by parsing manually
	// Since we're in the same package, we can call the yaml package.
	return unmarshalDaemonConfig(data, v)
}

func TestSaveDaemonConfig(t *testing.T) {
	// Override the home dir to use a temp dir.
	// SaveDaemonConfig uses defaultArcDir() which calls os.UserHomeDir().
	// We'll write to a known temp path by monkey-patching the environment.
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := &DaemonConfig{
		SocketPath:  "/tmp/test.sock",
		PIDPath:     "/tmp/test.pid",
		LockPath:    "/tmp/test.lock",
		StatePath:   "/tmp/test-state.json",
		MaxParallel: 7,
	}

	if err := SaveDaemonConfig(cfg); err != nil {
		t.Fatalf("SaveDaemonConfig: %v", err)
	}

	// Verify the file was written.
	configPath := filepath.Join(home, ".arc", "daemon.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("reading saved config: %v", err)
	}

	var loaded DaemonConfig
	if err := unmarshalDaemonConfig(data, &loaded); err != nil {
		t.Fatalf("unmarshaling saved config: %v", err)
	}

	if loaded.MaxParallel != 7 {
		t.Errorf("MaxParallel: got %d, want 7", loaded.MaxParallel)
	}
	if loaded.SocketPath != "/tmp/test.sock" {
		t.Errorf("SocketPath: got %s, want /tmp/test.sock", loaded.SocketPath)
	}
}

func TestSaveDaemonConfig_CreatesDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := &DaemonConfig{MaxParallel: 2}
	if err := SaveDaemonConfig(cfg); err != nil {
		t.Fatalf("SaveDaemonConfig: %v", err)
	}

	// Verify the ~/.arc directory was created.
	arcDir := filepath.Join(home, ".arc")
	if _, err := os.Stat(arcDir); err != nil {
		t.Errorf("~/.arc directory not created: %v", err)
	}
}
