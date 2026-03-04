package orchestrator

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Event type constants for PhaseEvent.
const (
	EventPhaseStarted   = "PhaseStarted"
	EventPhaseCompleted = "PhaseCompleted"
	EventPhaseFailed    = "PhaseFailed"
	EventGateRun        = "GateRun"
	EventAgentSpawned   = "AgentSpawned"
	EventAgentExited    = "AgentExited"
	EventRetryTriggered = "RetryTriggered"
)

// PhaseEvent is a structured log entry for orchestrator and phase activity.
type PhaseEvent struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Component string    `json:"component"`
	Phase     string    `json:"phase,omitempty"`
	Attempt   int       `json:"attempt,omitempty"`
	Event     string    `json:"event"`
	Detail    string    `json:"detail,omitempty"`
}

// PlanLogger writes structured JSON log entries to a plan's logs directory.
// It maintains two log streams:
//   - orchestrator.jsonl — plan-level events
//   - phase-<name>.jsonl — per-phase events
//
// All file writes are protected by a mutex so PlanLogger is safe for concurrent use.
type PlanLogger struct {
	logsDir string
	logger  *slog.Logger
	mu      sync.Mutex
	files   map[string]*os.File
}

// NewPlanLogger creates a PlanLogger that writes to <planDir>/logs/.
// The logs directory is created if it does not exist. If logger is nil,
// a discard logger is used as the fallback for warning output.
func NewPlanLogger(planDir string, logger *slog.Logger) *PlanLogger {
	if logger == nil {
		logger = slog.Default()
	}
	logsDir := filepath.Join(planDir, "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		logger.Warn("PlanLogger: failed to create logs directory", "dir", logsDir, "error", err)
	}
	return &PlanLogger{
		logsDir: logsDir,
		logger:  logger,
		files:   make(map[string]*os.File),
	}
}

// Close flushes and closes all open log file handles. Safe to call multiple times.
func (pl *PlanLogger) Close() {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	for name, f := range pl.files {
		f.Close()
		delete(pl.files, name)
	}
}

// LogOrchestrator writes an event to orchestrator.jsonl.
func (pl *PlanLogger) LogOrchestrator(event PhaseEvent) {
	pl.write("orchestrator.jsonl", event)
}

// LogPhase writes an event to phase-<phase>.jsonl and to orchestrator.jsonl.
func (pl *PlanLogger) LogPhase(event PhaseEvent) {
	filename := fmt.Sprintf("phase-%s.jsonl", event.Phase)
	pl.write(filename, event)
	pl.write("orchestrator.jsonl", event)
}

// PhaseStarted logs a PhaseStarted event.
func (pl *PlanLogger) PhaseStarted(phase string, attempt int) {
	pl.LogPhase(PhaseEvent{
		Timestamp: time.Now().UTC(),
		Level:     "INFO",
		Component: "orchestrator",
		Phase:     phase,
		Attempt:   attempt,
		Event:     EventPhaseStarted,
	})
}

// PhaseCompleted logs a PhaseCompleted event.
func (pl *PlanLogger) PhaseCompleted(phase string, attempt int, detail string) {
	pl.LogPhase(PhaseEvent{
		Timestamp: time.Now().UTC(),
		Level:     "INFO",
		Component: "orchestrator",
		Phase:     phase,
		Attempt:   attempt,
		Event:     EventPhaseCompleted,
		Detail:    detail,
	})
}

// PhaseFailed logs a PhaseFailed event.
func (pl *PlanLogger) PhaseFailed(phase string, attempt int, detail string) {
	pl.LogPhase(PhaseEvent{
		Timestamp: time.Now().UTC(),
		Level:     "ERROR",
		Component: "orchestrator",
		Phase:     phase,
		Attempt:   attempt,
		Event:     EventPhaseFailed,
		Detail:    detail,
	})
}

// GateRun logs a GateRun event with pass/fail and assertion details.
func (pl *PlanLogger) GateRun(phase string, attempt int, passed bool, detail string) {
	level := "INFO"
	if !passed {
		level = "WARN"
	}
	pl.LogPhase(PhaseEvent{
		Timestamp: time.Now().UTC(),
		Level:     level,
		Component: "gate",
		Phase:     phase,
		Attempt:   attempt,
		Event:     EventGateRun,
		Detail:    detail,
	})
}

// AgentSpawned logs an AgentSpawned event with adapter name and config summary.
func (pl *PlanLogger) AgentSpawned(phase string, attempt int, detail string) {
	pl.LogPhase(PhaseEvent{
		Timestamp: time.Now().UTC(),
		Level:     "DEBUG",
		Component: "agent",
		Phase:     phase,
		Attempt:   attempt,
		Event:     EventAgentSpawned,
		Detail:    detail,
	})
}

// AgentExited logs an AgentExited event with exit code, duration, and usage summary.
func (pl *PlanLogger) AgentExited(phase string, attempt int, detail string) {
	pl.LogPhase(PhaseEvent{
		Timestamp: time.Now().UTC(),
		Level:     "DEBUG",
		Component: "agent",
		Phase:     phase,
		Attempt:   attempt,
		Event:     EventAgentExited,
		Detail:    detail,
	})
}

// RetryTriggered logs a RetryTriggered event with tier classification and reason.
func (pl *PlanLogger) RetryTriggered(phase string, attempt int, detail string) {
	pl.LogPhase(PhaseEvent{
		Timestamp: time.Now().UTC(),
		Level:     "WARN",
		Component: "orchestrator",
		Phase:     phase,
		Attempt:   attempt,
		Event:     EventRetryTriggered,
		Detail:    detail,
	})
}

// write appends a JSON line to the named log file under logsDir.
// If any file operation fails, a warning is logged and the error is silently dropped.
func (pl *PlanLogger) write(filename string, event PhaseEvent) {
	line, err := json.Marshal(event)
	if err != nil {
		pl.logger.Warn("PlanLogger: failed to marshal event", "event", event.Event, "error", err)
		return
	}
	line = append(line, '\n')

	pl.mu.Lock()
	defer pl.mu.Unlock()

	f, err := pl.openLocked(filename)
	if err != nil {
		pl.logger.Warn("PlanLogger: failed to open log file", "file", filename, "error", err)
		return
	}

	if _, err := f.Write(line); err != nil {
		pl.logger.Warn("PlanLogger: failed to write log entry", "file", filename, "error", err)
	}
}

// openLocked returns (and caches) an open file handle for filename.
// Must be called with pl.mu held.
func (pl *PlanLogger) openLocked(filename string) (*os.File, error) {
	if f, ok := pl.files[filename]; ok {
		return f, nil
	}
	path := filepath.Join(pl.logsDir, filename)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	pl.files[filename] = f
	return f, nil
}
