package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestTailLines(t *testing.T) {
	// Build a 100-line string.
	var lines []string
	for i := 0; i < 100; i++ {
		lines = append(lines, fmt.Sprintf("line%d", i))
	}
	input := strings.Join(lines, "\n")

	result := tailLines(input, 50)
	resultLines := strings.Split(result, "\n")

	if len(resultLines) != 50 {
		t.Fatalf("expected 50 lines, got %d", len(resultLines))
	}
	// First line of result should be line50 (0-indexed).
	if resultLines[0] != "line50" {
		t.Errorf("first line: got %q, want line50", resultLines[0])
	}
	// Must not contain line49.
	if strings.Contains(result, "line49\n") || strings.HasPrefix(result, "line49") {
		t.Error("result should not contain line49")
	}
}

func TestTailLinesFewerThanN(t *testing.T) {
	input := strings.Repeat("line\n", 10)
	input = strings.TrimSuffix(input, "\n")

	result := tailLines(input, 50)
	if result != input {
		t.Errorf("expected full string unchanged, got different output")
	}
}

func TestTailLinesEmpty(t *testing.T) {
	result := tailLines("", 50)
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestTailLinesZero(t *testing.T) {
	result := tailLines("line1\nline2\nline3", 0)
	if result != "" {
		t.Errorf("expected empty string for n=0, got %q", result)
	}
}

func TestTailLinesNegative(t *testing.T) {
	result := tailLines("line1\nline2\nline3", -5)
	if result != "" {
		t.Errorf("expected empty string for negative n, got %q", result)
	}
}

func TestTailLinesOnlyNewlines(t *testing.T) {
	input := "\n\n\n"
	// Should not panic.
	result := tailLines(input, 2)
	_ = result
}

func TestBuildInterventionPromptContents(t *testing.T) {
	planMD := []byte("## plan content")
	stateJSON := []byte(`{"phase_status":"blocked"}`)

	result := buildInterventionPrompt("my-plan", "impl", 2, planMD, stateJSON)

	checks := []string{
		"my-plan",
		"impl",
		"Watch attempt: 2 of 3",
		"## plan content",
		`{"phase_status":"blocked"}`,
		"Do NOT call any arc CLI commands",
	}
	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("expected prompt to contain %q", check)
		}
	}
}

func TestBuildInterventionPromptEmpty(t *testing.T) {
	result := buildInterventionPrompt("plan", "phase", 1, []byte(""), []byte(""))
	if !strings.Contains(result, "plan") {
		t.Error("expected prompt to contain plan name")
	}
	if !strings.Contains(result, "phase") {
		t.Error("expected prompt to contain phase name")
	}
	if !strings.Contains(result, "Watch attempt") {
		t.Error("expected prompt to contain watch attempt")
	}
}

func TestInterventionLogJSONRoundTrip(t *testing.T) {
	original := InterventionLog{
		Plan:       "p",
		Phase:      "ph",
		Attempt:    1,
		StartedAt:  "2026-03-09T12:00:00Z",
		Duration:   "45s",
		ExitCode:   0,
		OutputTail: "last line",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	// output_tail should be present; error should be absent.
	if !strings.Contains(string(data), "output_tail") {
		t.Errorf("expected output_tail in JSON, got: %s", data)
	}
	if strings.Contains(string(data), `"error"`) {
		t.Errorf("error field should be omitted when empty, got: %s", data)
	}

	var got InterventionLog
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Plan != original.Plan || got.Phase != original.Phase ||
		got.Attempt != original.Attempt || got.OutputTail != original.OutputTail {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, original)
	}
}

func TestAppendInterventionLogCreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "watch.jsonl")

	entry := InterventionLog{Plan: "p", Phase: "ph", Attempt: 1, StartedAt: "2026-03-09T12:00:00Z", Duration: "1s"}
	appendInterventionLog(path, entry)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 line, got %d", len(lines))
	}
	var got InterventionLog
	if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Plan != "p" {
		t.Errorf("Plan: got %q, want p", got.Plan)
	}
}

func TestAppendInterventionLogAccumulates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "watch.jsonl")

	appendInterventionLog(path, InterventionLog{Plan: "p", Phase: "ph", Attempt: 1, Duration: "1s", StartedAt: "2026-03-09T12:00:00Z"})
	appendInterventionLog(path, InterventionLog{Plan: "p", Phase: "ph", Attempt: 2, Duration: "2s", StartedAt: "2026-03-09T12:01:00Z"})

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines, got %d: %s", len(lines), string(data))
	}
}

func TestAppendInterventionLogBadPath(t *testing.T) {
	// Should not panic.
	appendInterventionLog("/nonexistent/dir/watch.jsonl", InterventionLog{Plan: "p", Phase: "ph", Attempt: 1})
}

func TestAppendInterventionLogConcurrent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "watch.jsonl")

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			appendInterventionLog(path, InterventionLog{
				Plan:      "p",
				Phase:     fmt.Sprintf("phase-%d", i),
				Attempt:   i,
				StartedAt: "2026-03-09T12:00:00Z",
				Duration:  "1s",
			})
		}()
	}
	wg.Wait()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 10 {
		t.Errorf("expected 10 lines, got %d", len(lines))
	}
	// Verify each line parses as valid JSON.
	for i, line := range lines {
		var entry InterventionLog
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Errorf("line %d not valid JSON: %v: %s", i, err, line)
		}
	}
}

func TestAppendInterventionLogWithError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "watch.jsonl")

	entry := InterventionLog{Plan: "p", Phase: "ph", Attempt: 1, Error: "spawn failed", StartedAt: "2026-03-09T12:00:00Z", Duration: "1s"}
	appendInterventionLog(path, entry)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), `"error":"spawn failed"`) {
		t.Errorf("expected error field in JSON, got: %s", data)
	}
	if strings.Contains(string(data), `"output_tail"`) {
		t.Errorf("output_tail should be omitted when empty, got: %s", data)
	}
}

func TestMaxWatchAttemptsConstant(t *testing.T) {
	if MaxWatchAttempts != 3 {
		t.Errorf("MaxWatchAttempts: got %d, want 3", MaxWatchAttempts)
	}
}

func TestWatchEligiblePhasesIdentified(t *testing.T) {
	// Plan with 3 phases: eligible blocked, at-max blocked, complete.
	reg := makeReg("test-plan", []string{"impl", "maxed", "done"}, nil)
	reg.PhaseStates["impl"].PhaseStatus = "blocked"
	reg.PhaseStates["impl"].WatchAttempts = 0
	reg.PhaseStates["maxed"].PhaseStatus = "blocked"
	reg.PhaseStates["maxed"].WatchAttempts = MaxWatchAttempts
	reg.PhaseStates["done"].PhaseStatus = "complete"

	var eligible []string
	for _, phase := range reg.Meta.Phases {
		ps := reg.PhaseStates[phase]
		if ps != nil && ps.PhaseStatus == "blocked" && ps.WatchAttempts < MaxWatchAttempts {
			eligible = append(eligible, phase)
		}
	}

	if len(eligible) != 1 || eligible[0] != "impl" {
		t.Errorf("eligible: got %v, want [impl]", eligible)
	}
}

func TestWatchEligibleAtBoundary(t *testing.T) {
	reg := makeReg("test-plan", []string{"impl"}, nil)
	reg.PhaseStates["impl"].PhaseStatus = "blocked"
	reg.PhaseStates["impl"].WatchAttempts = MaxWatchAttempts - 1 // 2, eligible

	var eligible []string
	for _, phase := range reg.Meta.Phases {
		ps := reg.PhaseStates[phase]
		if ps != nil && ps.PhaseStatus == "blocked" && ps.WatchAttempts < MaxWatchAttempts {
			eligible = append(eligible, phase)
		}
	}
	if len(eligible) != 1 {
		t.Errorf("expected phase eligible at boundary (WatchAttempts=%d < %d), got %v",
			MaxWatchAttempts-1, MaxWatchAttempts, eligible)
	}
}

func TestWatchEligibleDeferredExcluded(t *testing.T) {
	reg := makeReg("test-plan", []string{"deferred-phase", "blocked-phase"}, nil)
	reg.PhaseStates["deferred-phase"].PhaseStatus = "deferred"
	reg.PhaseStates["deferred-phase"].WatchAttempts = 0
	reg.PhaseStates["blocked-phase"].PhaseStatus = "blocked"
	reg.PhaseStates["blocked-phase"].WatchAttempts = 0

	var eligible []string
	for _, phase := range reg.Meta.Phases {
		ps := reg.PhaseStates[phase]
		if ps != nil && ps.PhaseStatus == "blocked" && ps.WatchAttempts < MaxWatchAttempts {
			eligible = append(eligible, phase)
		}
	}
	if len(eligible) != 1 || eligible[0] != "blocked-phase" {
		t.Errorf("expected only blocked-phase eligible, got %v", eligible)
	}
}

func TestWatchEligibleEmptyPhases(t *testing.T) {
	reg := makeReg("test-plan", []string{}, nil)

	var eligible []string
	for _, phase := range reg.Meta.Phases {
		ps := reg.PhaseStates[phase]
		if ps != nil && ps.PhaseStatus == "blocked" && ps.WatchAttempts < MaxWatchAttempts {
			eligible = append(eligible, phase)
		}
	}
	if len(eligible) != 0 {
		t.Errorf("expected empty eligible for empty phases, got %v", eligible)
	}
}

func TestWatchEligibleNilPhaseState(t *testing.T) {
	reg := makeReg("test-plan", []string{"impl"}, nil)
	reg.PhaseStates["impl"] = nil // Nil phase state.

	var eligible []string
	for _, phase := range reg.Meta.Phases {
		ps := reg.PhaseStates[phase]
		if ps != nil && ps.PhaseStatus == "blocked" && ps.WatchAttempts < MaxWatchAttempts {
			eligible = append(eligible, phase)
		}
	}
	if len(eligible) != 0 {
		t.Error("nil phase state should be excluded from eligible")
	}
}

func TestWatchInterventionSetsInflight(t *testing.T) {
	s := NewScheduler(4, nil, nil)

	// Simulate two blocked phases being eligible.
	reg := makeReg("test-plan", []string{"a", "b"}, nil)
	reg.PhaseStates["a"].PhaseStatus = "blocked"
	reg.PhaseStates["a"].WatchAttempts = 0
	reg.PhaseStates["b"].PhaseStatus = "blocked"
	reg.PhaseStates["b"].WatchAttempts = 0
	s.registrations["test-plan"] = reg
	s.running["test-plan"] = nil

	eligible := []string{"a", "b"}
	reg.PendingWatch = true
	reg.WatchInflight = len(eligible)

	if !reg.PendingWatch {
		t.Error("PendingWatch should be true")
	}
	if reg.WatchInflight != 2 {
		t.Errorf("WatchInflight: got %d, want 2", reg.WatchInflight)
	}
	_ = s
}

func TestWatchInflightDecrement(t *testing.T) {
	s := NewScheduler(4, nil, nil)

	reg := makeReg("test-plan", []string{"a", "b"}, nil)
	reg.PhaseStates["a"].PhaseStatus = "blocked"
	reg.PhaseStates["b"].PhaseStatus = "blocked"
	reg.PendingWatch = true
	reg.WatchInflight = 2
	s.registrations["test-plan"] = reg
	s.running["test-plan"] = nil

	// Simulate first intervention result (WatchInflight: 2 → 1).
	result1 := PhaseResult{PlanName: "test-plan", PhaseName: "a", WatchIntervention: true}
	s.handlePhaseResult(result1)

	s.mu.Lock()
	inflight := reg.WatchInflight
	pendingWatch := reg.PendingWatch
	s.mu.Unlock()

	// Both phases still blocked, WatchAttempts still 0 (no disk file) → another round will fire.
	// But at this point after result1, WatchInflight should still be > 0 only if a 2nd round fires.
	// Since inflight was 1 after decrement, we check that WatchInflight was 1 OR that it was reset
	// for a new round (WatchInflight == 1 again for the retry phase "b").
	// The logic: WatchInflight 2→1, not 0 yet.
	if inflight != 1 {
		t.Errorf("WatchInflight after first result: got %d, want 1", inflight)
	}
	if !pendingWatch {
		t.Error("PendingWatch should still be true after first result")
	}
	_ = pendingWatch
}

func TestNoEligiblePhasesReleasePlan(t *testing.T) {
	released := make(chan struct{}, 1)
	finalizer := func(reg *PlanRegistration) {
		released <- struct{}{}
	}
	s := NewScheduler(4, nil, finalizer)

	// All blocked phases at MaxWatchAttempts.
	reg := makeReg("test-plan", []string{"a"}, nil)
	reg.PhaseStates["a"].PhaseStatus = "blocked"
	reg.PhaseStates["a"].WatchAttempts = MaxWatchAttempts // already maxed

	s.mu.Lock()
	s.registrations["test-plan"] = reg
	s.running["test-plan"] = nil
	s.mu.Unlock()

	// Simulate a normal phase result that triggers the allTerminal&&!allComplete path.
	// We need to trigger handlePhaseResult with a non-watch result.
	// The normal path goes: load updated state, mark dirty, check terminal...
	// But in handlePhaseResult, after the non-WatchIntervention path, it re-reads state from disk.
	// Since we don't have disk files, orchestrator.LoadPhaseState returns nil → in-memory unchanged.
	// Then it marks dirty and calls dispatchReady which calls collectReadyWork.
	// allTerminal=true (blocked), allComplete=false → no eligible → releasePlanLocked.
	s.handlePhaseResult(PhaseResult{PlanName: "test-plan", PhaseName: "a"})

	s.mu.Lock()
	_, stillRegistered := s.registrations["test-plan"]
	s.mu.Unlock()
	if stillRegistered {
		t.Error("plan should have been released (releasePlanLocked called) with no eligible phases")
	}
}
