package intelligence

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestOpen_MissingFile(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if s == nil {
		t.Fatal("expected non-nil store")
	}
	if s.data.TestCommands == nil {
		t.Error("TestCommands map should be initialized")
	}
	if s.data.FlakyTests == nil {
		t.Error("FlakyTests map should be initialized")
	}
}

func TestOpen_ExistingFile(t *testing.T) {
	dir := t.TempDir()
	arcDir := filepath.Join(dir, ".arc")
	if err := os.MkdirAll(arcDir, 0755); err != nil {
		t.Fatal(err)
	}

	d := &Data{
		TestCommands: map[string]string{"pkg/foo": "go test ./pkg/foo/"},
		FlakyTests: map[string]FlakyEntry{
			"TestFoo": {FailCount: 2, PassCount: 3, LastSeen: time.Now().UTC()},
		},
		CostHistory: []CostEntry{
			{PlanName: "my-plan", Complexity: "medium", CostUSD: 0.5, Turns: 10},
		},
	}
	data, _ := json.MarshalIndent(d, "", "  ")
	if err := os.WriteFile(filepath.Join(arcDir, "project.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if cmd := s.TestCommandFor("pkg/foo"); cmd != "go test ./pkg/foo/" {
		t.Errorf("TestCommandFor: got %q, want %q", cmd, "go test ./pkg/foo/")
	}
	if !s.IsFlaky("TestFoo") {
		t.Error("TestFoo should be flaky")
	}
	if len(s.data.CostHistory) != 1 {
		t.Errorf("cost history: got %d entries, want 1", len(s.data.CostHistory))
	}
}

func TestOpen_CorruptFile(t *testing.T) {
	dir := t.TempDir()
	arcDir := filepath.Join(dir, ".arc")
	if err := os.MkdirAll(arcDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(arcDir, "project.json"), []byte("not json{{{"), 0644); err != nil {
		t.Fatal(err)
	}

	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open with corrupt file: %v", err)
	}
	// Should start fresh.
	if s.data.TestCommands == nil {
		t.Error("TestCommands should be initialized after corrupt file")
	}
}

func TestRecordTestCommand_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	s.RecordTestCommand("internal/foo", "go test ./internal/foo/")
	s.RecordTestCommand("internal/bar", "go test -run TestBar ./internal/bar/")

	if got := s.TestCommandFor("internal/foo"); got != "go test ./internal/foo/" {
		t.Errorf("got %q, want %q", got, "go test ./internal/foo/")
	}
	if got := s.TestCommandFor("internal/bar"); got != "go test -run TestBar ./internal/bar/" {
		t.Errorf("got %q, want %q", got, "go test -run TestBar ./internal/bar/")
	}
	if got := s.TestCommandFor("nonexistent"); got != "" {
		t.Errorf("nonexistent package: got %q, want empty", got)
	}
}

func TestRecordTestCommand_Overwrite(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	s.RecordTestCommand("pkg", "old command")
	s.RecordTestCommand("pkg", "new command")

	if got := s.TestCommandFor("pkg"); got != "new command" {
		t.Errorf("got %q, want %q", got, "new command")
	}
}

func TestFlakyTest_NeitherFlaky(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	if s.IsFlaky("TestUnknown") {
		t.Error("unknown test should not be flaky")
	}
}

func TestFlakyTest_OnlyFailures(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	s.RecordFlakyTest("TestConsistentFail", false)
	s.RecordFlakyTest("TestConsistentFail", false)

	if s.IsFlaky("TestConsistentFail") {
		t.Error("test with only failures should not be flaky")
	}
}

func TestFlakyTest_OnlyPasses(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	s.RecordFlakyTest("TestAlwaysPass", true)
	s.RecordFlakyTest("TestAlwaysPass", true)

	if s.IsFlaky("TestAlwaysPass") {
		t.Error("test with only passes should not be flaky")
	}
}

func TestFlakyTest_MixedResults(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	s.RecordFlakyTest("TestFlaky", true)
	s.RecordFlakyTest("TestFlaky", false)

	if !s.IsFlaky("TestFlaky") {
		t.Error("test with both passes and failures should be flaky")
	}
}

func TestRecordCost_HistoryPruning(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	// Record 105 entries.
	for i := 0; i < 105; i++ {
		s.RecordCost("plan", "medium", float64(i)*0.01, i)
	}

	if len(s.data.CostHistory) != 100 {
		t.Errorf("cost history: got %d entries, want 100", len(s.data.CostHistory))
	}

	// The oldest 5 should have been pruned; last entry should be index 104.
	last := s.data.CostHistory[99]
	if last.Turns != 104 {
		t.Errorf("last entry turns: got %d, want 104", last.Turns)
	}
}

func TestRecordCost_Fields(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	s.RecordCost("my-plan", "complex", 1.23, 42)

	if len(s.data.CostHistory) != 1 {
		t.Fatalf("expected 1 cost entry, got %d", len(s.data.CostHistory))
	}
	e := s.data.CostHistory[0]
	if e.PlanName != "my-plan" {
		t.Errorf("PlanName: got %q, want %q", e.PlanName, "my-plan")
	}
	if e.Complexity != "complex" {
		t.Errorf("Complexity: got %q, want %q", e.Complexity, "complex")
	}
	if e.CostUSD != 1.23 {
		t.Errorf("CostUSD: got %f, want 1.23", e.CostUSD)
	}
	if e.Turns != 42 {
		t.Errorf("Turns: got %d, want 42", e.Turns)
	}
	if e.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
}

func TestRecordFileCoupling_NewEntry(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	s.RecordFileCoupling([]string{"a.go", "b.go"})

	if len(s.data.FileCoupling) != 1 {
		t.Fatalf("expected 1 coupling entry, got %d", len(s.data.FileCoupling))
	}
	e := s.data.FileCoupling[0]
	if e.Count != 1 {
		t.Errorf("count: got %d, want 1", e.Count)
	}
}

func TestRecordFileCoupling_Increments(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	s.RecordFileCoupling([]string{"a.go", "b.go"})
	s.RecordFileCoupling([]string{"b.go", "a.go"}) // same files, different order

	if len(s.data.FileCoupling) != 1 {
		t.Fatalf("expected 1 coupling entry (normalized), got %d", len(s.data.FileCoupling))
	}
	if s.data.FileCoupling[0].Count != 2 {
		t.Errorf("count: got %d, want 2", s.data.FileCoupling[0].Count)
	}
}

func TestRecordFileCoupling_TooFewFiles(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	s.RecordFileCoupling([]string{"a.go"}) // only 1 file — should be ignored
	s.RecordFileCoupling(nil)              // nil — should be ignored
	s.RecordFileCoupling([]string{})       // empty — should be ignored

	if len(s.data.FileCoupling) != 0 {
		t.Errorf("expected 0 coupling entries, got %d", len(s.data.FileCoupling))
	}
}

func TestRecordFileCoupling_MultipleSets(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	s.RecordFileCoupling([]string{"a.go", "b.go"})
	s.RecordFileCoupling([]string{"c.go", "d.go"})
	s.RecordFileCoupling([]string{"a.go", "b.go"})

	if len(s.data.FileCoupling) != 2 {
		t.Fatalf("expected 2 coupling entries, got %d", len(s.data.FileCoupling))
	}
}

func TestSave_Persistence(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	s.RecordTestCommand("pkg/baz", "go test ./pkg/baz/")
	s.RecordFlakyTest("TestRace", true)
	s.RecordFlakyTest("TestRace", false)
	s.RecordCost("plan-x", "simple", 0.1, 5)
	s.RecordFileCoupling([]string{"x.go", "y.go"})

	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Re-open and verify data persisted.
	s2, err := Open(dir)
	if err != nil {
		t.Fatalf("re-Open: %v", err)
	}

	if cmd := s2.TestCommandFor("pkg/baz"); cmd != "go test ./pkg/baz/" {
		t.Errorf("TestCommandFor after reload: got %q", cmd)
	}
	if !s2.IsFlaky("TestRace") {
		t.Error("TestRace should still be flaky after reload")
	}
	if len(s2.data.CostHistory) != 1 {
		t.Errorf("cost history after reload: got %d entries", len(s2.data.CostHistory))
	}
	if len(s2.data.FileCoupling) != 1 {
		t.Errorf("file coupling after reload: got %d entries", len(s2.data.FileCoupling))
	}
	if s2.data.LastUpdated.IsZero() {
		t.Error("LastUpdated should be set after Save")
	}
}

func TestSave_CreatesArcDirectory(t *testing.T) {
	dir := t.TempDir()
	// Do not pre-create .arc — Save should create it.
	s, _ := Open(dir)
	s.RecordTestCommand("pkg", "go test ./pkg/")

	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	path := filepath.Join(dir, ".arc", "project.json")
	if _, err := os.Stat(path); err != nil {
		t.Errorf("project.json not created: %v", err)
	}
}

func TestFilterFlakyTests_NilStore(t *testing.T) {
	// With a nil store, all tests should pass through unfiltered.
	failing := []string{"TestA", "TestB"}
	result := FilterFlakyTests(nil, failing)
	if len(result) != 2 {
		t.Errorf("nil store: expected 2 tests, got %d", len(result))
	}
}

func TestFilterFlakyTests_RemovesFlaky(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	// TestFlaky has both passes and failures — it's flaky.
	s.RecordFlakyTest("TestFlaky", true)
	s.RecordFlakyTest("TestFlaky", false)

	// TestReal only has failures — not flaky.
	s.RecordFlakyTest("TestReal", false)
	s.RecordFlakyTest("TestReal", false)

	failing := []string{"TestFlaky", "TestReal", "TestUnknown"}
	result := FilterFlakyTests(s, failing)

	// TestFlaky should be removed; TestReal and TestUnknown should remain.
	if len(result) != 2 {
		t.Errorf("expected 2 non-flaky tests, got %d: %v", len(result), result)
	}
	for _, name := range result {
		if name == "TestFlaky" {
			t.Error("TestFlaky should have been filtered out")
		}
	}
}

func TestFilterFlakyTests_EmptyInput(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	result := FilterFlakyTests(s, nil)
	if result == nil {
		// nil is acceptable for empty input — just check length is 0.
		return
	}
	if len(result) != 0 {
		t.Errorf("expected 0 tests, got %d", len(result))
	}
}

func TestFilterFlakyTests_AllKnownFlaky(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	s.RecordFlakyTest("TestA", true)
	s.RecordFlakyTest("TestA", false)
	s.RecordFlakyTest("TestB", true)
	s.RecordFlakyTest("TestB", false)

	result := FilterFlakyTests(s, []string{"TestA", "TestB"})
	if len(result) != 0 {
		t.Errorf("expected 0 non-flaky tests, got %d: %v", len(result), result)
	}
}

// ---- FailurePattern tests ----

func TestRecordFailurePattern_NewEntry(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	s.RecordFailurePattern("undefined: NewFactory", "add import for the factory package")

	if len(s.data.FailurePatterns) != 1 {
		t.Fatalf("expected 1 failure pattern, got %d", len(s.data.FailurePatterns))
	}
	fp := s.data.FailurePatterns[0]
	if fp.Error != "undefined: NewFactory" {
		t.Errorf("Error: got %q, want %q", fp.Error, "undefined: NewFactory")
	}
	if fp.Fix != "add import for the factory package" {
		t.Errorf("Fix: got %q, want %q", fp.Fix, "add import for the factory package")
	}
	if fp.Count != 1 {
		t.Errorf("Count: got %d, want 1", fp.Count)
	}
	if fp.LastSeen == "" {
		t.Error("LastSeen should not be empty")
	}
}

func TestRecordFailurePattern_IncrementExisting(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	s.RecordFailurePattern("undefined: NewFactory", "add import for the factory package")
	s.RecordFailurePattern("undefined: NewFactory", "add import for the factory package")

	if len(s.data.FailurePatterns) != 1 {
		t.Fatalf("expected 1 failure pattern (deduplicated), got %d", len(s.data.FailurePatterns))
	}
	if s.data.FailurePatterns[0].Count != 2 {
		t.Errorf("Count: got %d, want 2", s.data.FailurePatterns[0].Count)
	}
}

func TestRecordFailurePattern_SubstringMatch(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	// Record a short pattern.
	s.RecordFailurePattern("undefined: NewFactory", "add import for factory")

	// Record with a longer string that contains the existing error as a substring.
	s.RecordFailurePattern("build error: undefined: NewFactory at line 42", "add import for factory")

	// Should increment the existing entry rather than create a new one.
	if len(s.data.FailurePatterns) != 1 {
		t.Fatalf("expected 1 failure pattern (substring match), got %d", len(s.data.FailurePatterns))
	}
	if s.data.FailurePatterns[0].Count != 2 {
		t.Errorf("Count: got %d, want 2", s.data.FailurePatterns[0].Count)
	}
}

func TestRecordFailurePattern_MultipleDistinct(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	s.RecordFailurePattern("undefined: NewFactory", "add factory import")
	s.RecordFailurePattern("cannot use string as int", "fix type mismatch")
	s.RecordFailurePattern("imported and not used", "remove unused import")

	if len(s.data.FailurePatterns) != 3 {
		t.Errorf("expected 3 failure patterns, got %d", len(s.data.FailurePatterns))
	}
}

func TestRecordFailurePattern_Cap50(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	// Add 55 distinct patterns. Use UUIDs-like strings to avoid substring collisions.
	for i := 100; i < 155; i++ {
		s.RecordFailurePattern(fmt.Sprintf("unique_err_xyz_%d_abc", i), fmt.Sprintf("fix for %d", i))
	}

	if len(s.data.FailurePatterns) != 50 {
		t.Errorf("expected 50 failure patterns (capped), got %d", len(s.data.FailurePatterns))
	}
}

func TestFindFixForError_Match(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	s.RecordFailurePattern("undefined: NewFactory", "add import for the factory package")
	s.RecordFailurePattern("cannot use string as int", "fix type mismatch at call site")

	errorOutput := `./main.go:10:5: undefined: NewFactory
./main.go:10:5: too many errors`

	fix := s.FindFixForError(errorOutput)
	if fix != "add import for the factory package" {
		t.Errorf("FindFixForError: got %q, want %q", fix, "add import for the factory package")
	}
}

func TestFindFixForError_NoMatch(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	s.RecordFailurePattern("undefined: NewFactory", "add factory import")

	fix := s.FindFixForError("compilation failed: syntax error near }")
	if fix != "" {
		t.Errorf("FindFixForError: expected empty string, got %q", fix)
	}
}

func TestFindFixForError_EmptyStore(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	fix := s.FindFixForError("undefined: SomeFunc")
	if fix != "" {
		t.Errorf("FindFixForError on empty store: expected empty string, got %q", fix)
	}
}

func TestFailurePattern_LastSeenIsRFC3339(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	s.RecordFailurePattern("some error", "some fix")
	lastSeen := s.data.FailurePatterns[0].LastSeen

	if lastSeen == "" {
		t.Fatal("LastSeen should not be empty")
	}
	// Verify it is a valid RFC3339 timestamp.
	if _, err := time.Parse(time.RFC3339, lastSeen); err != nil {
		t.Errorf("LastSeen is not a valid RFC3339 timestamp: %q, error: %v", lastSeen, err)
	}
}

func TestFailurePattern_Persistence(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	s.RecordFailurePattern("undefined: Foo", "import pkg/foo")
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	s2, _ := Open(dir)
	if len(s2.data.FailurePatterns) != 1 {
		t.Fatalf("expected 1 failure pattern after reload, got %d", len(s2.data.FailurePatterns))
	}
	if s2.data.FailurePatterns[0].Error != "undefined: Foo" {
		t.Errorf("Error after reload: got %q", s2.data.FailurePatterns[0].Error)
	}
}

// ---- ConventionPattern tests ----

func TestRecordConvention_NewEntry(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	s.RecordConvention("file_structure", "Test files alongside source (*_test.go)")

	if len(s.data.ConventionPatterns) != 1 {
		t.Fatalf("expected 1 convention, got %d", len(s.data.ConventionPatterns))
	}
	cp := s.data.ConventionPatterns[0]
	if cp.Type != "file_structure" {
		t.Errorf("Type: got %q, want %q", cp.Type, "file_structure")
	}
	if cp.Pattern != "Test files alongside source (*_test.go)" {
		t.Errorf("Pattern: got %q", cp.Pattern)
	}
	if cp.Confidence != 30 {
		t.Errorf("Confidence: got %d, want 30", cp.Confidence)
	}
	if cp.Observations != 1 {
		t.Errorf("Observations: got %d, want 1", cp.Observations)
	}
}

func TestRecordConvention_IncrementExisting(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	s.RecordConvention("test_naming", "Test functions use TestXxx naming")
	s.RecordConvention("test_naming", "Test functions use TestXxx naming")
	s.RecordConvention("test_naming", "Test functions use TestXxx naming")

	if len(s.data.ConventionPatterns) != 1 {
		t.Fatalf("expected 1 convention (deduplicated), got %d", len(s.data.ConventionPatterns))
	}
	cp := s.data.ConventionPatterns[0]
	if cp.Observations != 3 {
		t.Errorf("Observations: got %d, want 3", cp.Observations)
	}
	// Confidence starts at 30 and increases by 10 per additional observation.
	// After 3 records: 30 + 10 + 10 = 50.
	if cp.Confidence != 50 {
		t.Errorf("Confidence: got %d, want 50", cp.Confidence)
	}
}

func TestRecordConvention_ConfidenceCappedAt100(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	// 8 additional observations after the first gives: 30 + 8*10 = 110, should cap at 100.
	for i := 0; i < 9; i++ {
		s.RecordConvention("file_structure", "Test files alongside source (*_test.go)")
	}

	if s.data.ConventionPatterns[0].Confidence != 100 {
		t.Errorf("Confidence: got %d, want 100 (capped)", s.data.ConventionPatterns[0].Confidence)
	}
}

func TestRecordConvention_DifferentTypeSamePattern(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	s.RecordConvention("file_structure", "same pattern")
	s.RecordConvention("test_naming", "same pattern")

	if len(s.data.ConventionPatterns) != 2 {
		t.Errorf("expected 2 conventions (different types), got %d", len(s.data.ConventionPatterns))
	}
}

func TestRecordConvention_SameTypeDifferentPattern(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	s.RecordConvention("file_structure", "pattern A")
	s.RecordConvention("file_structure", "pattern B")

	if len(s.data.ConventionPatterns) != 2 {
		t.Errorf("expected 2 conventions (different patterns), got %d", len(s.data.ConventionPatterns))
	}
}

func TestRecordConvention_Cap30(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	for i := 0; i < 35; i++ {
		s.RecordConvention("test_naming", fmt.Sprintf("convention pattern %d", i))
	}

	if len(s.data.ConventionPatterns) != 30 {
		t.Errorf("expected 30 conventions (capped), got %d", len(s.data.ConventionPatterns))
	}
}

func TestGetConventions_FiltersByType(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	s.RecordConvention("file_structure", "Test files alongside source")
	s.RecordConvention("test_naming", "TestXxx naming")
	s.RecordConvention("file_structure", "One package per directory")

	conventions := s.GetConventions("file_structure")
	if len(conventions) != 2 {
		t.Fatalf("expected 2 file_structure conventions, got %d", len(conventions))
	}
	for _, c := range conventions {
		if c.Type != "file_structure" {
			t.Errorf("GetConventions returned wrong type: %q", c.Type)
		}
	}
}

func TestGetConventions_SortedByConfidenceDesc(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	s.RecordConvention("file_structure", "pattern A")
	// Bump pattern A confidence to 50.
	s.RecordConvention("file_structure", "pattern A")
	s.RecordConvention("file_structure", "pattern A")

	s.RecordConvention("file_structure", "pattern B")
	// pattern B has confidence 30, pattern A has confidence 50.

	conventions := s.GetConventions("file_structure")
	if len(conventions) < 2 {
		t.Fatalf("expected at least 2 conventions, got %d", len(conventions))
	}
	if conventions[0].Confidence < conventions[1].Confidence {
		t.Errorf("not sorted descending: [0]=%d [1]=%d", conventions[0].Confidence, conventions[1].Confidence)
	}
}

func TestGetConventions_EmptyType(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	s.RecordConvention("file_structure", "some pattern")

	result := s.GetConventions("nonexistent_type")
	if len(result) != 0 {
		t.Errorf("expected 0 conventions for unknown type, got %d", len(result))
	}
}

func TestGetAllConventions_SortedByConfidenceDesc(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	s.RecordConvention("file_structure", "pattern A") // confidence 30
	s.RecordConvention("test_naming", "TestXxx")      // confidence 30
	// Bump test_naming to 40.
	s.RecordConvention("test_naming", "TestXxx")

	all := s.GetAllConventions()
	if len(all) != 2 {
		t.Fatalf("expected 2 total conventions, got %d", len(all))
	}
	if all[0].Confidence < all[1].Confidence {
		t.Errorf("not sorted descending: [0]=%d [1]=%d", all[0].Confidence, all[1].Confidence)
	}
	if all[0].Pattern != "TestXxx" {
		t.Errorf("expected highest-confidence pattern first, got %q", all[0].Pattern)
	}
}

func TestGetAllConventions_Empty(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	all := s.GetAllConventions()
	if len(all) != 0 {
		t.Errorf("expected 0 conventions, got %d", len(all))
	}
}

func TestConventionPattern_Persistence(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	s.RecordConvention("file_structure", "Test files alongside source (*_test.go)")
	s.RecordConvention("file_structure", "Test files alongside source (*_test.go)")

	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	s2, _ := Open(dir)
	conventions := s2.GetConventions("file_structure")
	if len(conventions) != 1 {
		t.Fatalf("expected 1 convention after reload, got %d", len(conventions))
	}
	if conventions[0].Observations != 2 {
		t.Errorf("Observations after reload: got %d, want 2", conventions[0].Observations)
	}
	if conventions[0].Confidence != 40 {
		t.Errorf("Confidence after reload: got %d, want 40", conventions[0].Confidence)
	}
}

func TestConcurrentAccess(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	var wg sync.WaitGroup
	n := 50

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s.RecordTestCommand("pkg", "go test .")
			s.TestCommandFor("pkg")
			s.RecordFlakyTest("TestConcurrent", i%2 == 0)
			s.IsFlaky("TestConcurrent")
			s.RecordCost("plan", "medium", 0.01, 1)
			s.RecordFileCoupling([]string{"a.go", "b.go"})
		}(i)
	}

	wg.Wait()

	// Basic sanity: cost history should have n entries (or 100 if pruned).
	if len(s.data.CostHistory) > 100 {
		t.Errorf("cost history exceeded max: %d", len(s.data.CostHistory))
	}
}

func TestRecordRateLimit_Basic(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	s.RecordRateLimit("claude", 5)
	s.RecordRateLimit("claude", 3)
	s.RecordRateLimit("codex", 2)

	if got := s.CountRateLimitEvents("claude"); got != 2 {
		t.Errorf("CountRateLimitEvents claude: got %d, want 2", got)
	}
	if got := s.CountRateLimitEvents("codex"); got != 1 {
		t.Errorf("CountRateLimitEvents codex: got %d, want 1", got)
	}
	if got := s.CountRateLimitEvents("generic"); got != 0 {
		t.Errorf("CountRateLimitEvents generic: got %d, want 0", got)
	}
}

func TestRecordRateLimit_Cap(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	for i := 0; i < 250; i++ {
		s.mu.Lock()
		s.data.RateLimitHistory = append(s.data.RateLimitHistory, RateLimitEvent{
			Adapter:   "claude",
			Parallel:  i,
			Timestamp: time.Now().UTC(),
		})
		s.mu.Unlock()
	}

	// Trigger cap via RecordRateLimit
	s.RecordRateLimit("claude", 10)

	if got := len(s.data.RateLimitHistory); got > 200 {
		t.Errorf("RateLimitHistory exceeded cap: %d entries", got)
	}
}

func TestSuggestMaxParallel_NoData(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	if got := s.SuggestMaxParallel("claude"); got != 0 {
		t.Errorf("SuggestMaxParallel with no data: got %d, want 0", got)
	}
}

func TestSuggestMaxParallel_MinusOne(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	s.RecordRateLimit("claude", 5)
	s.RecordRateLimit("claude", 3)
	s.RecordRateLimit("claude", 7)

	// min is 3, so suggestion is max(1, 3-1) = 2
	if got := s.SuggestMaxParallel("claude"); got != 2 {
		t.Errorf("SuggestMaxParallel: got %d, want 2", got)
	}
}

func TestSuggestMaxParallel_MinIsOne(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	s.RecordRateLimit("claude", 1)

	// min is 1, so suggestion is max(1, 0) = 1
	if got := s.SuggestMaxParallel("claude"); got != 1 {
		t.Errorf("SuggestMaxParallel with min=1: got %d, want 1", got)
	}
}

func TestSuggestMaxParallel_AdapterIsolation(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	s.RecordRateLimit("claude", 4)
	s.RecordRateLimit("codex", 10)

	// claude: min=4, suggest=3
	if got := s.SuggestMaxParallel("claude"); got != 3 {
		t.Errorf("SuggestMaxParallel claude: got %d, want 3", got)
	}
	// codex: min=10, suggest=9
	if got := s.SuggestMaxParallel("codex"); got != 9 {
		t.Errorf("SuggestMaxParallel codex: got %d, want 9", got)
	}
	// generic: no data
	if got := s.SuggestMaxParallel("generic"); got != 0 {
		t.Errorf("SuggestMaxParallel generic: got %d, want 0", got)
	}
}

func TestCountRateLimitEvents(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	if got := s.CountRateLimitEvents("claude"); got != 0 {
		t.Errorf("initial count: got %d, want 0", got)
	}

	s.RecordRateLimit("claude", 3)
	s.RecordRateLimit("claude", 4)
	s.RecordRateLimit("codex", 2)

	if got := s.CountRateLimitEvents("claude"); got != 2 {
		t.Errorf("after 2 claude events: got %d, want 2", got)
	}
}
