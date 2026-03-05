package orchestrator

import (
	"bufio"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// readJSONLines parses all JSON lines from a file into a slice of PhaseEvent.
func readJSONLines(t *testing.T, path string) []PhaseEvent {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("readJSONLines: %v", err)
	}
	var events []PhaseEvent
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e PhaseEvent
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("unmarshal line %q: %v", line, err)
		}
		events = append(events, e)
	}
	return events
}

func TestNewPlanLoggerCreatesLogsDir(t *testing.T) {
	planDir := t.TempDir()
	pl := NewPlanLogger(planDir, slog.Default())
	defer pl.Close()

	logsDir := filepath.Join(planDir, "logs")
	if _, err := os.Stat(logsDir); os.IsNotExist(err) {
		t.Fatal("expected logs directory to be created")
	}
}

func TestPhaseStartedWritesToPhaseAndOrchestratorFiles(t *testing.T) {
	planDir := t.TempDir()
	pl := NewPlanLogger(planDir, slog.Default())
	defer pl.Close()

	pl.PhaseStarted("impl", 1)

	// Give the OS time to flush, then close to flush handles.
	pl.Close()

	orchestratorPath := filepath.Join(planDir, "logs", "orchestrator.jsonl")
	phasePath := filepath.Join(planDir, "logs", "phase-impl.jsonl")

	for _, path := range []string{orchestratorPath, phasePath} {
		events := readJSONLines(t, path)
		if len(events) != 1 {
			t.Fatalf("%s: expected 1 event, got %d", path, len(events))
		}
		e := events[0]
		if e.Event != EventPhaseStarted {
			t.Errorf("%s: expected Event=%q, got %q", path, EventPhaseStarted, e.Event)
		}
		if e.Phase != "impl" {
			t.Errorf("%s: expected Phase=%q, got %q", path, "impl", e.Phase)
		}
		if e.Attempt != 1 {
			t.Errorf("%s: expected Attempt=1, got %d", path, e.Attempt)
		}
		if e.Level != "INFO" {
			t.Errorf("%s: expected Level=INFO, got %q", path, e.Level)
		}
		if e.Component != "orchestrator" {
			t.Errorf("%s: expected Component=orchestrator, got %q", path, e.Component)
		}
	}
}

func TestPhaseCompletedWritesEvent(t *testing.T) {
	planDir := t.TempDir()
	pl := NewPlanLogger(planDir, slog.Default())
	defer pl.Close()

	pl.PhaseCompleted("fix", 2, "all tests passing")
	pl.Close()

	events := readJSONLines(t, filepath.Join(planDir, "logs", "phase-fix.jsonl"))
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Event != EventPhaseCompleted {
		t.Errorf("expected EventPhaseCompleted, got %q", events[0].Event)
	}
	if events[0].Detail != "all tests passing" {
		t.Errorf("expected detail %q, got %q", "all tests passing", events[0].Detail)
	}
}

func TestPhaseFailedWritesErrorLevel(t *testing.T) {
	planDir := t.TempDir()
	pl := NewPlanLogger(planDir, slog.Default())
	defer pl.Close()

	pl.PhaseFailed("impl", 3, "max retries exceeded")
	pl.Close()

	events := readJSONLines(t, filepath.Join(planDir, "logs", "phase-impl.jsonl"))
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Event != EventPhaseFailed {
		t.Errorf("expected EventPhaseFailed, got %q", events[0].Event)
	}
	if events[0].Level != "ERROR" {
		t.Errorf("expected Level=ERROR, got %q", events[0].Level)
	}
}

func TestGateRunPassedWritesInfoLevel(t *testing.T) {
	planDir := t.TempDir()
	pl := NewPlanLogger(planDir, slog.Default())
	defer pl.Close()

	pl.GateRun("tests", 1, true, "5/5 assertions passed")
	pl.Close()

	events := readJSONLines(t, filepath.Join(planDir, "logs", "phase-tests.jsonl"))
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Event != EventGateRun {
		t.Errorf("expected EventGateRun, got %q", events[0].Event)
	}
	if events[0].Level != "INFO" {
		t.Errorf("expected Level=INFO for passed gate, got %q", events[0].Level)
	}
	if events[0].Component != "gate" {
		t.Errorf("expected Component=gate, got %q", events[0].Component)
	}
}

func TestGateRunFailedWritesWarnLevel(t *testing.T) {
	planDir := t.TempDir()
	pl := NewPlanLogger(planDir, slog.Default())
	defer pl.Close()

	pl.GateRun("tests", 1, false, "2/5 assertions passed")
	pl.Close()

	events := readJSONLines(t, filepath.Join(planDir, "logs", "phase-tests.jsonl"))
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Level != "WARN" {
		t.Errorf("expected Level=WARN for failed gate, got %q", events[0].Level)
	}
}

func TestAgentSpawnedAndExitedWriteDebugLevel(t *testing.T) {
	planDir := t.TempDir()
	pl := NewPlanLogger(planDir, slog.Default())
	defer pl.Close()

	pl.AgentSpawned("impl", 1, "adapter=claude model=sonnet")
	pl.AgentExited("impl", 1, "exit=0 duration=45s tokens=1200")
	pl.Close()

	events := readJSONLines(t, filepath.Join(planDir, "logs", "phase-impl.jsonl"))
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Event != EventAgentSpawned {
		t.Errorf("expected EventAgentSpawned, got %q", events[0].Event)
	}
	if events[0].Level != "DEBUG" {
		t.Errorf("expected Level=DEBUG, got %q", events[0].Level)
	}
	if events[1].Event != EventAgentExited {
		t.Errorf("expected EventAgentExited, got %q", events[1].Event)
	}
	if events[1].Level != "DEBUG" {
		t.Errorf("expected Level=DEBUG, got %q", events[1].Level)
	}
}

func TestRetryTriggeredWritesWarnLevel(t *testing.T) {
	planDir := t.TempDir()
	pl := NewPlanLogger(planDir, slog.Default())
	defer pl.Close()

	pl.RetryTriggered("impl", 2, "tier=session-crash reason=non-zero exit")
	pl.Close()

	events := readJSONLines(t, filepath.Join(planDir, "logs", "phase-impl.jsonl"))
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Event != EventRetryTriggered {
		t.Errorf("expected EventRetryTriggered, got %q", events[0].Event)
	}
	if events[0].Level != "WARN" {
		t.Errorf("expected Level=WARN, got %q", events[0].Level)
	}
}

func TestLogOrchestratorOnlyWritesToOrchestratorFile(t *testing.T) {
	planDir := t.TempDir()
	pl := NewPlanLogger(planDir, slog.Default())
	defer pl.Close()

	pl.LogOrchestrator(PhaseEvent{
		Timestamp: time.Now().UTC(),
		Level:     "INFO",
		Component: "orchestrator",
		Event:     EventPhaseStarted,
		Detail:    "plan-level event with no phase",
	})
	pl.Close()

	// orchestrator.jsonl should have one event.
	events := readJSONLines(t, filepath.Join(planDir, "logs", "orchestrator.jsonl"))
	if len(events) != 1 {
		t.Fatalf("expected 1 event in orchestrator.jsonl, got %d", len(events))
	}

	// No per-phase file should exist (phase field was empty).
	entries, err := os.ReadDir(filepath.Join(planDir, "logs"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "phase-") {
			t.Errorf("expected no per-phase file, found %q", entry.Name())
		}
	}
}

func TestEventsAppendToExistingFile(t *testing.T) {
	planDir := t.TempDir()

	// First logger writes one event.
	pl1 := NewPlanLogger(planDir, slog.Default())
	pl1.PhaseStarted("impl", 1)
	pl1.Close()

	// Second logger writes another event to the same file (append mode).
	pl2 := NewPlanLogger(planDir, slog.Default())
	pl2.PhaseCompleted("impl", 1, "done")
	pl2.Close()

	events := readJSONLines(t, filepath.Join(planDir, "logs", "phase-impl.jsonl"))
	if len(events) != 2 {
		t.Fatalf("expected 2 events after two logger instances, got %d", len(events))
	}
	if events[0].Event != EventPhaseStarted {
		t.Errorf("first event: expected EventPhaseStarted, got %q", events[0].Event)
	}
	if events[1].Event != EventPhaseCompleted {
		t.Errorf("second event: expected EventPhaseCompleted, got %q", events[1].Event)
	}
}

func TestTimestampIsSet(t *testing.T) {
	planDir := t.TempDir()
	before := time.Now().UTC()
	pl := NewPlanLogger(planDir, slog.Default())
	pl.PhaseStarted("impl", 1)
	pl.Close()
	after := time.Now().UTC()

	events := readJSONLines(t, filepath.Join(planDir, "logs", "phase-impl.jsonl"))
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ts := events[0].Timestamp
	if ts.Before(before) || ts.After(after) {
		t.Errorf("timestamp %v not between %v and %v", ts, before, after)
	}
}

func TestConcurrentWrites(t *testing.T) {
	planDir := t.TempDir()
	pl := NewPlanLogger(planDir, slog.Default())
	defer pl.Close()

	const goroutines = 20
	const eventsEach = 10

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < eventsEach; j++ {
				pl.PhaseStarted("impl", id*eventsEach+j)
			}
		}(i)
	}
	wg.Wait()
	pl.Close()

	// Both files should have goroutines*eventsEach events.
	expected := goroutines * eventsEach

	for _, filename := range []string{"orchestrator.jsonl", "phase-impl.jsonl"} {
		events := readJSONLines(t, filepath.Join(planDir, "logs", filename))
		if len(events) != expected {
			t.Errorf("%s: expected %d events, got %d", filename, expected, len(events))
		}
	}
}

func TestAdversaryStartedWritesToOrchestratorAndAdversaryFiles(t *testing.T) {
	planDir := t.TempDir()
	pl := NewPlanLogger(planDir, slog.Default())

	pl.AdversaryStarted(1, "files=5 adapter=claude")
	pl.Close()

	orchestratorPath := filepath.Join(planDir, "logs", "orchestrator.jsonl")
	adversaryPath := filepath.Join(planDir, "logs", "adversary.jsonl")

	for _, path := range []string{orchestratorPath, adversaryPath} {
		events := readJSONLines(t, path)
		if len(events) != 1 {
			t.Fatalf("%s: expected 1 event, got %d", path, len(events))
		}
		e := events[0]
		if e.Event != EventAdversaryStarted {
			t.Errorf("%s: expected Event=%q, got %q", path, EventAdversaryStarted, e.Event)
		}
		if e.Attempt != 1 {
			t.Errorf("%s: expected Attempt=1, got %d", path, e.Attempt)
		}
		if e.Component != "adversary" {
			t.Errorf("%s: expected Component=adversary, got %q", path, e.Component)
		}
		if e.Level != "INFO" {
			t.Errorf("%s: expected Level=INFO, got %q", path, e.Level)
		}
		if e.Detail != "files=5 adapter=claude" {
			t.Errorf("%s: expected Detail=%q, got %q", path, "files=5 adapter=claude", e.Detail)
		}
	}
}

func TestAdversaryCompletedNoBugsWritesInfo(t *testing.T) {
	planDir := t.TempDir()
	pl := NewPlanLogger(planDir, slog.Default())

	pl.AdversaryCompleted(1, 0, "no bugs found")
	pl.Close()

	for _, filename := range []string{"orchestrator.jsonl", "adversary.jsonl"} {
		events := readJSONLines(t, filepath.Join(planDir, "logs", filename))
		if len(events) != 1 {
			t.Fatalf("%s: expected 1 event, got %d", filename, len(events))
		}
		if events[0].Event != EventAdversaryCompleted {
			t.Errorf("%s: expected EventAdversaryCompleted, got %q", filename, events[0].Event)
		}
		if events[0].Level != "INFO" {
			t.Errorf("%s: expected Level=INFO for no bugs, got %q", filename, events[0].Level)
		}
	}
}

func TestAdversaryCompletedWithBugsWritesWarn(t *testing.T) {
	planDir := t.TempDir()
	pl := NewPlanLogger(planDir, slog.Default())

	pl.AdversaryCompleted(2, 3, "bugs_found=3")
	pl.Close()

	for _, filename := range []string{"orchestrator.jsonl", "adversary.jsonl"} {
		events := readJSONLines(t, filepath.Join(planDir, "logs", filename))
		if len(events) != 1 {
			t.Fatalf("%s: expected 1 event, got %d", filename, len(events))
		}
		if events[0].Level != "WARN" {
			t.Errorf("%s: expected Level=WARN when bugs found, got %q", filename, events[0].Level)
		}
		if events[0].Attempt != 2 {
			t.Errorf("%s: expected Attempt=2, got %d", filename, events[0].Attempt)
		}
	}
}

func TestAdversaryDoesNotWriteToPhaseFile(t *testing.T) {
	planDir := t.TempDir()
	pl := NewPlanLogger(planDir, slog.Default())

	pl.AdversaryStarted(1, "test")
	pl.AdversaryCompleted(1, 0, "done")
	pl.Close()

	// No phase-*.jsonl file should be created — adversary events use adversary.jsonl.
	entries, err := os.ReadDir(filepath.Join(planDir, "logs"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "phase-") {
			t.Errorf("adversary methods should not create phase files, found %q", entry.Name())
		}
	}
}

func TestNewPlanLoggerNilLoggerDoesNotPanic(t *testing.T) {
	planDir := t.TempDir()
	// nil logger should not panic
	pl := NewPlanLogger(planDir, nil)
	pl.PhaseStarted("impl", 1)
	pl.Close()

	events := readJSONLines(t, filepath.Join(planDir, "logs", "phase-impl.jsonl"))
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
}

func TestMultiplePhasesHaveSeparateFiles(t *testing.T) {
	planDir := t.TempDir()
	pl := NewPlanLogger(planDir, slog.Default())

	pl.PhaseStarted("impl", 1)
	pl.PhaseStarted("tests", 1)
	pl.PhaseCompleted("impl", 1, "done")
	pl.Close()

	implEvents := readJSONLines(t, filepath.Join(planDir, "logs", "phase-impl.jsonl"))
	if len(implEvents) != 2 {
		t.Errorf("phase-impl.jsonl: expected 2 events, got %d", len(implEvents))
	}

	testEvents := readJSONLines(t, filepath.Join(planDir, "logs", "phase-tests.jsonl"))
	if len(testEvents) != 1 {
		t.Errorf("phase-tests.jsonl: expected 1 event, got %d", len(testEvents))
	}

	// Orchestrator file gets all events.
	orchEvents := readJSONLines(t, filepath.Join(planDir, "logs", "orchestrator.jsonl"))
	if len(orchEvents) != 3 {
		t.Errorf("orchestrator.jsonl: expected 3 events, got %d", len(orchEvents))
	}
}
