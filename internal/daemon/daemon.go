package daemon

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
)

// DaemonConfig holds configuration for the daemon process.
type DaemonConfig struct {
	SocketPath  string `yaml:"socket_path"`
	PIDPath     string `yaml:"pid_path"`
	LockPath    string `yaml:"lock_path"`
	StatePath   string `yaml:"state_path"`
	MaxParallel int    `yaml:"max_parallel"`
}

// Daemon manages the lifecycle of the arc daemon process.
type Daemon struct {
	cfg       DaemonConfig
	listener  net.Listener
	sched     *Scheduler
	lockFile  *os.File
	startedAt time.Time

	mu       sync.Mutex
	shutdown bool
}

// New creates a new Daemon with the given configuration and scheduler.
func New(cfg DaemonConfig, sched *Scheduler) *Daemon {
	return &Daemon{
		cfg:   cfg,
		sched: sched,
	}
}

// Start initializes the daemon: acquires the file lock, probes for an existing
// daemon, writes the PID file, starts the unix socket listener, and runs the
// accept loop until the context is cancelled.
func (d *Daemon) Start(ctx context.Context, handler func(net.Conn)) error {
	// 1. Acquire file lock
	if err := os.MkdirAll(filepath.Dir(d.cfg.LockPath), 0755); err != nil {
		return fmt.Errorf("creating lock directory: %w", err)
	}
	lockFile, err := os.OpenFile(d.cfg.LockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("opening lock file: %w", err)
	}
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lockFile.Close()
		return fmt.Errorf("acquiring lock (another daemon starting?): %w", err)
	}
	d.lockFile = lockFile

	// 2. Probe existing socket
	if err := d.probeSocket(); err != nil {
		d.releaseLock()
		return err
	}

	// 3. Write PID file
	if err := os.MkdirAll(filepath.Dir(d.cfg.PIDPath), 0755); err != nil {
		d.releaseLock()
		return fmt.Errorf("creating PID directory: %w", err)
	}
	pid := os.Getpid()
	if err := os.WriteFile(d.cfg.PIDPath, []byte(strconv.Itoa(pid)), 0644); err != nil {
		d.releaseLock()
		return fmt.Errorf("writing PID file: %w", err)
	}

	// 4. Listen on unix socket
	if err := os.MkdirAll(filepath.Dir(d.cfg.SocketPath), 0755); err != nil {
		d.removePIDFile()
		d.releaseLock()
		return fmt.Errorf("creating socket directory: %w", err)
	}
	ln, err := net.Listen("unix", d.cfg.SocketPath)
	if err != nil {
		d.removePIDFile()
		d.releaseLock()
		return fmt.Errorf("listening on socket: %w", err)
	}
	d.listener = ln
	d.startedAt = time.Now()

	// 5. Accept loop in goroutine
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				// Listener closed — expected during shutdown
				return
			}
			go handler(conn)
		}
	}()

	// 6. Wait for context cancellation
	<-ctx.Done()

	// 7. Shutdown
	return d.Shutdown()
}

// Shutdown performs graceful shutdown: closes the listener, waits for running
// phases, persists state, and cleans up socket/PID/lock files.
func (d *Daemon) Shutdown() error {
	d.mu.Lock()
	if d.shutdown {
		d.mu.Unlock()
		return nil
	}
	d.shutdown = true
	d.mu.Unlock()

	var errs []error

	// 1. Close listener — stops accept loop
	if d.listener != nil {
		if err := d.listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			errs = append(errs, fmt.Errorf("closing listener: %w", err))
		}
	}

	// 2. Wait up to 10s for running phases
	// The scheduler's context is cancelled by the caller; we just wait a bit
	// for in-flight work to drain.
	done := make(chan struct{})
	go func() {
		// Persist final state via scheduler (which also waits for drain)
		if d.sched != nil {
			if err := d.sched.PersistState(); err != nil {
				d.mu.Lock()
				errs = append(errs, fmt.Errorf("persisting state: %w", err))
				d.mu.Unlock()
			}
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		errs = append(errs, fmt.Errorf("timed out waiting for scheduler to persist"))
	}

	// 4. Remove socket file
	if d.cfg.SocketPath != "" {
		os.Remove(d.cfg.SocketPath)
	}

	// 5. Remove PID file
	d.removePIDFile()

	// 6. Release flock
	d.releaseLock()

	return errors.Join(errs...)
}

// StartedAt returns when the daemon was started.
func (d *Daemon) StartedAt() time.Time {
	return d.startedAt
}

// probeSocket checks if an existing socket is live or stale.
func (d *Daemon) probeSocket() error {
	_, err := os.Stat(d.cfg.SocketPath)
	if os.IsNotExist(err) {
		return nil // no socket — good to go
	}
	if err != nil {
		return fmt.Errorf("checking socket: %w", err)
	}

	// Socket file exists — try connecting
	conn, err := net.DialTimeout("unix", d.cfg.SocketPath, 2*time.Second)
	if err != nil {
		// Connection refused — stale socket, remove it
		os.Remove(d.cfg.SocketPath)
		return nil
	}
	conn.Close()
	return fmt.Errorf("daemon already running (socket %s accepts connections)", d.cfg.SocketPath)
}

func (d *Daemon) removePIDFile() {
	if d.cfg.PIDPath != "" {
		os.Remove(d.cfg.PIDPath)
	}
}

func (d *Daemon) releaseLock() {
	if d.lockFile != nil {
		d.lockFile.Close()
		d.lockFile = nil
	}
}

// defaultArcDir returns ~/.arc.
func defaultArcDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.TempDir()
	}
	return filepath.Join(home, ".arc")
}

// DefaultDaemonConfig returns a DaemonConfig with default paths.
func DefaultDaemonConfig() DaemonConfig {
	dir := defaultArcDir()
	return DaemonConfig{
		SocketPath:  filepath.Join(dir, "daemon.sock"),
		PIDPath:     filepath.Join(dir, "daemon.pid"),
		LockPath:    filepath.Join(dir, "daemon.lock"),
		StatePath:   filepath.Join(dir, "daemon-state.json"),
		MaxParallel: 3,
	}
}

// LoadDaemonConfig reads daemon configuration from ~/.arc/daemon.yaml,
// falling back to defaults for any unset fields.
func LoadDaemonConfig() (*DaemonConfig, error) {
	cfg := DefaultDaemonConfig()

	configPath := filepath.Join(defaultArcDir(), "daemon.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &cfg, nil
		}
		return nil, fmt.Errorf("reading daemon config: %w", err)
	}

	var fileCfg DaemonConfig
	if err := yaml.Unmarshal(data, &fileCfg); err != nil {
		return nil, fmt.Errorf("parsing daemon config: %w", err)
	}

	// Overlay non-zero file values onto defaults
	if fileCfg.SocketPath != "" {
		cfg.SocketPath = fileCfg.SocketPath
	}
	if fileCfg.PIDPath != "" {
		cfg.PIDPath = fileCfg.PIDPath
	}
	if fileCfg.LockPath != "" {
		cfg.LockPath = fileCfg.LockPath
	}
	if fileCfg.StatePath != "" {
		cfg.StatePath = fileCfg.StatePath
	}
	if fileCfg.MaxParallel > 0 {
		cfg.MaxParallel = fileCfg.MaxParallel
	}

	return &cfg, nil
}

// unmarshalDaemonConfig unmarshals YAML data into a DaemonConfig.
// Exported for use in tests within the same package.
func unmarshalDaemonConfig(data []byte, cfg *DaemonConfig) error {
	return yaml.Unmarshal(data, cfg)
}

// EnsureRunning checks if the daemon is running and auto-starts it if not.
func EnsureRunning(socketPath string) error {
	if IsRunning(socketPath) {
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding executable: %w", err)
	}

	logPath := filepath.Join(filepath.Dir(socketPath), "daemon.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("opening daemon log: %w", err)
	}

	cmd := exec.Command(exe, "daemon", "start")
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("starting daemon: %w", err)
	}
	logFile.Close()

	// Poll socket readiness: up to 5s, 100ms intervals
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if IsRunning(socketPath) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("daemon did not become ready within 5s")
}
