package arc

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestNewPhaseStateNoNilSlices(t *testing.T) {
	s := NewPhaseState("myplan", "01-foundation", "feature")

	if s.Disputes == nil {
		t.Fatal("Disputes should not be nil")
	}
	if len(s.Disputes) != 0 {
		t.Fatalf("Disputes should be empty, got %d", len(s.Disputes))
	}

	if s.VerdictsHistory == nil {
		t.Fatal("VerdictsHistory should not be nil")
	}
	if len(s.VerdictsHistory) != 0 {
		t.Fatalf("VerdictsHistory should be empty, got %d", len(s.VerdictsHistory))
	}

	if s.TestFiles == nil {
		t.Fatal("TestFiles should not be nil")
	}
	if len(s.TestFiles) != 0 {
		t.Fatalf("TestFiles should be empty, got %d", len(s.TestFiles))
	}

	if s.Packages == nil {
		t.Fatal("Packages should not be nil")
	}
	if len(s.Packages) != 0 {
		t.Fatalf("Packages should be empty, got %d", len(s.Packages))
	}

	if s.ExecutedEscalations == nil {
		t.Fatal("ExecutedEscalations should not be nil")
	}
	if len(s.ExecutedEscalations) != 0 {
		t.Fatalf("ExecutedEscalations should be empty, got %d", len(s.ExecutedEscalations))
	}

	if s.LastClearedDisputes == nil {
		t.Fatal("LastClearedDisputes should not be nil")
	}
	if len(s.LastClearedDisputes) != 0 {
		t.Fatalf("LastClearedDisputes should be empty, got %d", len(s.LastClearedDisputes))
	}

	if s.Chunks.Completed == nil {
		t.Fatal("Chunks.Completed should not be nil")
	}
	if len(s.Chunks.Completed) != 0 {
		t.Fatalf("Chunks.Completed should be empty, got %d", len(s.Chunks.Completed))
	}

	if s.Chunks.Remaining == nil {
		t.Fatal("Chunks.Remaining should not be nil")
	}
	if len(s.Chunks.Remaining) != 0 {
		t.Fatalf("Chunks.Remaining should be empty, got %d", len(s.Chunks.Remaining))
	}

	if s.ParallelExecution != nil {
		t.Fatal("ParallelExecution should be nil")
	}
	if s.InterventionRequest != nil {
		t.Fatal("InterventionRequest should be nil")
	}
	if s.Chunks.Current != nil {
		t.Fatal("Chunks.Current should be nil")
	}
	if s.Blocked.Reason != nil {
		t.Fatal("Blocked.Reason should be nil")
	}
}

func TestNewPhaseStateDefaults(t *testing.T) {
	s := NewPhaseState("myplan", "01-foundation", "feature")

	if s.PhaseStatus != "pending" {
		t.Fatalf("PhaseStatus = %q, want %q", s.PhaseStatus, "pending")
	}
	if s.Iteration.Max != 25 {
		t.Fatalf("Iteration.Max = %d, want 25", s.Iteration.Max)
	}
	if s.Iteration.Current != 0 {
		t.Fatalf("Iteration.Current = %d, want 0", s.Iteration.Current)
	}
	if s.Blocked.IsBlocked != false {
		t.Fatal("Blocked.IsBlocked should be false")
	}
}

func TestPhaseStateJSONRoundtrip(t *testing.T) {
	s := NewPhaseState("p", "ph", "feature")

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var s2 PhaseState
	if err := json.Unmarshal(data, &s2); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if !reflect.DeepEqual(*s, s2) {
		t.Fatal("roundtripped state does not match original")
	}

	// Verify empty slices marshal as [] not null
	raw := string(data)
	if strings.Contains(raw, `"disputes":null`) {
		t.Fatal("Disputes should marshal as [] not null")
	}
	if strings.Contains(raw, `"verdicts_history":null`) {
		t.Fatal("VerdictsHistory should marshal as [] not null")
	}
	if strings.Contains(raw, `"test_files":null`) {
		t.Fatal("TestFiles should marshal as [] not null")
	}
	if strings.Contains(raw, `"packages":null`) {
		t.Fatal("Packages should marshal as [] not null")
	}
	if strings.Contains(raw, `"executed_escalations":null`) {
		t.Fatal("ExecutedEscalations should marshal as [] not null")
	}
	if strings.Contains(raw, `"completed":null`) {
		t.Fatal("Chunks.Completed should marshal as [] not null")
	}
	if strings.Contains(raw, `"remaining":null`) {
		t.Fatal("Chunks.Remaining should marshal as [] not null")
	}

	// Chunks.Current should marshal as null
	if !strings.Contains(raw, `"current":null`) {
		t.Fatal("Chunks.Current should marshal as null")
	}

	// Blocked.Reason should marshal as null
	if !strings.Contains(raw, `"reason":null`) {
		t.Fatal("Blocked.Reason should marshal as null")
	}
}

func TestPhaseStateJSONUnknownFields(t *testing.T) {
	input := `{"phase":"p","plan":"pl","future_field":"value","phase_status":"pending","iteration":{"current":0,"max":25}}`

	var s PhaseState
	err := json.Unmarshal([]byte(input), &s)
	if err != nil {
		t.Fatalf("Unmarshal should not error on unknown fields, got: %v", err)
	}
	if s.Phase != "p" {
		t.Fatalf("Phase = %q, want %q", s.Phase, "p")
	}
}

func TestNewPlanMeta(t *testing.T) {
	m := NewPlanMeta("myplan", "feature", []string{"qa", "impl"})

	if !reflect.DeepEqual(m.Phases, []string{"qa", "impl"}) {
		t.Fatalf("Phases = %v, want [qa impl]", m.Phases)
	}

	wantOrder := map[string]int{"qa": 1, "impl": 2}
	if !reflect.DeepEqual(m.PhaseOrder, wantOrder) {
		t.Fatalf("PhaseOrder = %v, want %v", m.PhaseOrder, wantOrder)
	}

	wantDeps := map[string][]string{}
	if !reflect.DeepEqual(m.Dependencies, wantDeps) {
		t.Fatalf("Dependencies = %v, want %v", m.Dependencies, wantDeps)
	}

	if m.Status != "active" {
		t.Fatalf("Status = %q, want %q", m.Status, "active")
	}
	if m.ReviewStatus != "unreviewed" {
		t.Fatalf("ReviewStatus = %q, want %q", m.ReviewStatus, "unreviewed")
	}
	if m.WorkflowType != "feature" {
		t.Fatalf("WorkflowType = %q, want %q", m.WorkflowType, "feature")
	}
}

func TestNewPlanMetaSinglePhase(t *testing.T) {
	m := NewPlanMeta("myplan", "feature", []string{"core"})

	if !reflect.DeepEqual(m.Phases, []string{"core"}) {
		t.Fatalf("Phases = %v, want [core]", m.Phases)
	}

	wantDeps := map[string][]string{}
	if !reflect.DeepEqual(m.Dependencies, wantDeps) {
		t.Fatalf("Dependencies = %v, want %v", m.Dependencies, wantDeps)
	}

	wantOrder := map[string]int{"core": 1}
	if !reflect.DeepEqual(m.PhaseOrder, wantOrder) {
		t.Fatalf("PhaseOrder = %v, want %v", m.PhaseOrder, wantOrder)
	}
}

func TestNewPlanMetaEmptyPhases(t *testing.T) {
	m := NewPlanMeta("myplan", "feature", []string{})

	if m.Phases == nil {
		t.Fatal("Phases should not be nil")
	}
	if len(m.Phases) != 0 {
		t.Fatalf("Phases should be empty, got %d", len(m.Phases))
	}

	if m.Dependencies == nil {
		t.Fatal("Dependencies should not be nil")
	}
	if len(m.Dependencies) != 0 {
		t.Fatalf("Dependencies should be empty, got %d", len(m.Dependencies))
	}

	if m.PhaseOrder == nil {
		t.Fatal("PhaseOrder should not be nil")
	}
	if len(m.PhaseOrder) != 0 {
		t.Fatalf("PhaseOrder should be empty, got %d", len(m.PhaseOrder))
	}
}

func TestNewPlanMetaDuplicatePhases(t *testing.T) {
	m := NewPlanMeta("myplan", "feature", []string{"core", "core"})

	if !reflect.DeepEqual(m.Phases, []string{"core", "core"}) {
		t.Fatalf("Phases = %v, want [core core]", m.Phases)
	}

	// Last wins for PhaseOrder
	if m.PhaseOrder["core"] != 2 {
		t.Fatalf("PhaseOrder[core] = %d, want 2", m.PhaseOrder["core"])
	}

	// No auto-deps — phases are parallel by default
	wantDeps := map[string][]string{}
	if !reflect.DeepEqual(m.Dependencies, wantDeps) {
		t.Fatalf("Dependencies = %v, want %v", m.Dependencies, wantDeps)
	}
}

func TestNewPhaseStateInitialCurrentState(t *testing.T) {
	s := NewPhaseState("p", "ph", "feature")
	if s.CurrentState != "" {
		t.Fatalf("CurrentState = %q, want empty string", s.CurrentState)
	}
}

func TestNewPlanMetaEmptyName(t *testing.T) {
	m := NewPlanMeta("", "feature", []string{"a"})
	if m.Name != "" {
		t.Fatalf("Name = %q, want empty string", m.Name)
	}
}

func TestNewPlanMetaCreatedTimestamp(t *testing.T) {
	before := time.Now()
	m := NewPlanMeta("p", "feature", []string{"a"})
	after := time.Now()

	parsed, err := time.Parse(time.RFC3339, m.Created)
	if err != nil {
		t.Fatalf("Created is not valid RFC3339: %v", err)
	}

	if parsed.Before(before.Truncate(time.Second)) {
		t.Fatalf("Created timestamp %v is before test start %v", parsed, before)
	}
	if parsed.After(after.Add(time.Second)) {
		t.Fatalf("Created timestamp %v is after test end %v", parsed, after)
	}
}

func TestPhaseStateJSONRoundtripBlockedInfo(t *testing.T) {
	s := NewPhaseState("core", "my-plan", "feature")
	reason := "dependency not met"
	s.Blocked = BlockedInfo{IsBlocked: true, Reason: &reason}

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var s2 PhaseState
	if err := json.Unmarshal(data, &s2); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if !s2.Blocked.IsBlocked {
		t.Fatal("Blocked.IsBlocked should be true")
	}
	if s2.Blocked.Reason == nil {
		t.Fatal("Blocked.Reason should not be nil")
	}
	if *s2.Blocked.Reason != "dependency not met" {
		t.Fatalf("Blocked.Reason = %q, want %q", *s2.Blocked.Reason, "dependency not met")
	}
}

func TestPhaseStateChunksCompletedJSON(t *testing.T) {
	s := NewPhaseState("core", "my-plan", "feature")

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	raw := string(data)
	if strings.Contains(raw, `"completed":null`) {
		t.Fatal("Chunks.Completed should marshal as [] not null")
	}
	if !strings.Contains(raw, `"completed":[]`) {
		t.Fatal("Chunks.Completed should marshal as []")
	}
}
