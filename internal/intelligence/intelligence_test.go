package intelligence

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// --- helpers ---

func tempDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

func mustLoad(t *testing.T, dir string) *ProjectData {
	t.Helper()
	d, err := Load(dir)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	return d
}

func mustSave(t *testing.T, dir string, d *ProjectData) {
	t.Helper()
	if err := Save(dir, d); err != nil {
		t.Fatalf("Save error: %v", err)
	}
}

// --- Load ---

func TestLoadNonExistentReturnsEmpty(t *testing.T) {
	dir := tempDir(t)
	d, err := Load(dir)
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if d == nil {
		t.Fatal("expected non-nil ProjectData")
	}
	if d.TestCommands == nil {
		t.Error("TestCommands map should be non-nil")
	}
	if d.FlakyTests == nil {
		t.Error("FlakyTests map should be non-nil")
	}
	if d.CostHistory == nil {
		t.Error("CostHistory map should be non-nil")
	}
	if len(d.FileCoupling) != 0 {
		t.Errorf("FileCoupling should be empty, got %d entries", len(d.FileCoupling))
	}
	if len(d.FailurePatterns) != 0 {
		t.Errorf("FailurePatterns should be empty, got %d entries", len(d.FailurePatterns))
	}
}

func TestLoadCorruptJSON(t *testing.T) {
	dir := tempDir(t)
	arcPath := filepath.Join(dir, arcDir)
	if err := os.MkdirAll(arcPath, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(arcPath, dataFile), []byte("{invalid"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for corrupt JSON, got nil")
	}
}

// --- Save / Load round-trip ---

func TestSaveLoadRoundtrip(t *testing.T) {
	dir := tempDir(t)
	d := mustLoad(t, dir)

	RecordTestCommand(d, "github.com/example/pkg", "go test ./internal/pkg/")
	RecordFlakyTest(d, "TestFoo", "github.com/example/pkg")
	RecordFileCoupling(d, []string{"a.go", "b.go"})
	RecordCost(d, "medium", 0.42)
	RecordFailurePattern(d, "undefined:", "add import")

	mustSave(t, dir, d)

	d2 := mustLoad(t, dir)

	if cmd := d2.TestCommands["github.com/example/pkg"]; cmd != "go test ./internal/pkg/" {
		t.Errorf("TestCommands round-trip: got %q, want %q", cmd, "go test ./internal/pkg/")
	}
	if rec, ok := d2.FlakyTests["TestFoo"]; !ok || rec.Occurrences != 1 {
		t.Errorf("FlakyTests round-trip: got %+v", rec)
	}
	if len(d2.FileCoupling) != 1 || d2.FileCoupling[0].CoChanges != 1 {
		t.Errorf("FileCoupling round-trip: got %+v", d2.FileCoupling)
	}
	if s := d2.CostHistory["medium"]; s.Count != 1 || s.AvgCost != 0.42 {
		t.Errorf("CostHistory round-trip: got %+v", s)
	}
	if len(d2.FailurePatterns) != 1 || d2.FailurePatterns[0].Pattern != "undefined:" {
		t.Errorf("FailurePatterns round-trip: got %+v", d2.FailurePatterns)
	}
}

func TestSaveCreatesArcDir(t *testing.T) {
	dir := tempDir(t)
	d := empty()
	mustSave(t, dir, d)

	if _, err := os.Stat(filepath.Join(dir, arcDir, dataFile)); err != nil {
		t.Fatalf("expected file to exist after Save: %v", err)
	}
}

func TestSaveAtomicNoTempLeftover(t *testing.T) {
	dir := tempDir(t)
	d := empty()
	mustSave(t, dir, d)

	entries, err := os.ReadDir(filepath.Join(dir, arcDir))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if e.Name() != dataFile {
			t.Errorf("unexpected file left in .arc/: %s", e.Name())
		}
	}
}

func TestSaveUpdatesVersion(t *testing.T) {
	dir := tempDir(t)
	d := empty()
	d.Version = 0
	mustSave(t, dir, d)

	d2 := mustLoad(t, dir)
	if d2.Version != version {
		t.Errorf("Version = %d, want %d", d2.Version, version)
	}
}

// --- TestCommands ---

func TestRecordAndQueryTestCommand(t *testing.T) {
	d := empty()
	RecordTestCommand(d, "github.com/foo/bar", "go test ./...")
	if got := SuggestedTestCommand(d, "github.com/foo/bar"); got != "go test ./..." {
		t.Errorf("SuggestedTestCommand = %q, want %q", got, "go test ./...")
	}
}

func TestSuggestedTestCommandUnknown(t *testing.T) {
	d := empty()
	if got := SuggestedTestCommand(d, "github.com/unknown"); got != "" {
		t.Errorf("SuggestedTestCommand for unknown pkg = %q, want empty", got)
	}
}

func TestRecordTestCommandOverwrites(t *testing.T) {
	d := empty()
	RecordTestCommand(d, "pkg", "old-cmd")
	RecordTestCommand(d, "pkg", "new-cmd")
	if got := SuggestedTestCommand(d, "pkg"); got != "new-cmd" {
		t.Errorf("SuggestedTestCommand = %q, want %q", got, "new-cmd")
	}
}

// --- FlakyTests ---

func TestRecordFlakyTestIncrements(t *testing.T) {
	d := empty()
	RecordFlakyTest(d, "TestBar", "pkg")
	RecordFlakyTest(d, "TestBar", "pkg")
	if rec := d.FlakyTests["TestBar"]; rec.Occurrences != 2 {
		t.Errorf("Occurrences = %d, want 2", rec.Occurrences)
	}
}

func TestIsFlakyBelowThreshold(t *testing.T) {
	d := empty()
	RecordFlakyTest(d, "TestFlaky", "pkg")
	RecordFlakyTest(d, "TestFlaky", "pkg")
	if IsFlaky(d, "TestFlaky") {
		t.Error("IsFlaky should be false below threshold (2 occurrences)")
	}
}

func TestIsFlakyAtThreshold(t *testing.T) {
	d := empty()
	for i := 0; i < FlakyThreshold; i++ {
		RecordFlakyTest(d, "TestFlaky", "pkg")
	}
	if !IsFlaky(d, "TestFlaky") {
		t.Errorf("IsFlaky should be true at threshold (%d occurrences)", FlakyThreshold)
	}
}

func TestIsFlakyUnknownTest(t *testing.T) {
	d := empty()
	if IsFlaky(d, "TestNeverSeen") {
		t.Error("IsFlaky should be false for unknown test")
	}
}

func TestRecordFlakyTestUpdatesPackage(t *testing.T) {
	d := empty()
	RecordFlakyTest(d, "TestBaz", "pkgA")
	if rec := d.FlakyTests["TestBaz"]; rec.Package != "pkgA" {
		t.Errorf("Package = %q, want %q", rec.Package, "pkgA")
	}
}

// --- FileCoupling ---

func TestRecordFileCouplingDeduplicates(t *testing.T) {
	d := empty()
	RecordFileCoupling(d, []string{"a.go", "b.go"})
	RecordFileCoupling(d, []string{"b.go", "a.go"}) // reversed order
	if len(d.FileCoupling) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(d.FileCoupling))
	}
	if d.FileCoupling[0].CoChanges != 2 {
		t.Errorf("CoChanges = %d, want 2", d.FileCoupling[0].CoChanges)
	}
}

func TestRecordFileCouplingDistinctSets(t *testing.T) {
	d := empty()
	RecordFileCoupling(d, []string{"a.go", "b.go"})
	RecordFileCoupling(d, []string{"c.go", "d.go"})
	if len(d.FileCoupling) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(d.FileCoupling))
	}
}

func TestRecordFileCouplingIgnoresSingleFile(t *testing.T) {
	d := empty()
	RecordFileCoupling(d, []string{"only.go"})
	if len(d.FileCoupling) != 0 {
		t.Errorf("expected no entries for single-file set, got %d", len(d.FileCoupling))
	}
}

func TestRecordFileCouplingIgnoresEmpty(t *testing.T) {
	d := empty()
	RecordFileCoupling(d, []string{})
	if len(d.FileCoupling) != 0 {
		t.Errorf("expected no entries for empty set, got %d", len(d.FileCoupling))
	}
}

func TestRecordFileCouplingThreeFiles(t *testing.T) {
	d := empty()
	RecordFileCoupling(d, []string{"z.go", "a.go", "m.go"})
	if len(d.FileCoupling) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(d.FileCoupling))
	}
	// Files should be stored sorted.
	entry := d.FileCoupling[0]
	if entry.Files[0] != "a.go" || entry.Files[1] != "m.go" || entry.Files[2] != "z.go" {
		t.Errorf("files not sorted: %v", entry.Files)
	}
}

// --- CostHistory ---

func TestRecordCostSingleEntry(t *testing.T) {
	d := empty()
	RecordCost(d, "high", 1.50)
	s := d.CostHistory["high"]
	if s.Count != 1 {
		t.Errorf("Count = %d, want 1", s.Count)
	}
	if s.TotalCost != 1.50 {
		t.Errorf("TotalCost = %f, want 1.50", s.TotalCost)
	}
	if s.AvgCost != 1.50 {
		t.Errorf("AvgCost = %f, want 1.50", s.AvgCost)
	}
	if s.MinCost != 1.50 || s.MaxCost != 1.50 {
		t.Errorf("Min/Max = %f/%f, want 1.50/1.50", s.MinCost, s.MaxCost)
	}
}

func TestRecordCostMultipleEntries(t *testing.T) {
	d := empty()
	RecordCost(d, "low", 0.10)
	RecordCost(d, "low", 0.30)
	RecordCost(d, "low", 0.20)
	s := d.CostHistory["low"]
	if s.Count != 3 {
		t.Errorf("Count = %d, want 3", s.Count)
	}
	if got := s.TotalCost; got < 0.599 || got > 0.601 {
		t.Errorf("TotalCost = %f, want ~0.60", got)
	}
	if got := s.AvgCost; got < 0.199 || got > 0.201 {
		t.Errorf("AvgCost = %f, want ~0.20", got)
	}
	if s.MinCost != 0.10 {
		t.Errorf("MinCost = %f, want 0.10", s.MinCost)
	}
	if s.MaxCost != 0.30 {
		t.Errorf("MaxCost = %f, want 0.30", s.MaxCost)
	}
}

func TestEstimateCostNoHistory(t *testing.T) {
	d := empty()
	if got := EstimateCost(d, "unknown"); got != 0 {
		t.Errorf("EstimateCost with no history = %f, want 0", got)
	}
}

func TestEstimateCostReturnsAvg(t *testing.T) {
	d := empty()
	RecordCost(d, "medium", 1.00)
	RecordCost(d, "medium", 3.00)
	if got := EstimateCost(d, "medium"); got != 2.00 {
		t.Errorf("EstimateCost = %f, want 2.00", got)
	}
}

// --- FailurePatterns ---

func TestRecordFailurePatternNew(t *testing.T) {
	d := empty()
	RecordFailurePattern(d, "undefined:", "add import")
	if len(d.FailurePatterns) != 1 {
		t.Fatalf("expected 1 pattern, got %d", len(d.FailurePatterns))
	}
	fp := d.FailurePatterns[0]
	if fp.Pattern != "undefined:" || fp.Fix != "add import" || fp.Occurrences != 1 {
		t.Errorf("unexpected pattern entry: %+v", fp)
	}
}

func TestRecordFailurePatternIncrement(t *testing.T) {
	d := empty()
	RecordFailurePattern(d, "undefined:", "add import")
	RecordFailurePattern(d, "undefined:", "add import")
	if len(d.FailurePatterns) != 1 {
		t.Fatalf("expected 1 pattern after dedup, got %d", len(d.FailurePatterns))
	}
	if d.FailurePatterns[0].Occurrences != 2 {
		t.Errorf("Occurrences = %d, want 2", d.FailurePatterns[0].Occurrences)
	}
}

func TestRecordFailurePatternDistinctFix(t *testing.T) {
	d := empty()
	RecordFailurePattern(d, "undefined:", "add import")
	RecordFailurePattern(d, "undefined:", "different fix")
	if len(d.FailurePatterns) != 2 {
		t.Errorf("expected 2 patterns for same error with different fix, got %d", len(d.FailurePatterns))
	}
}

func TestRecordFailurePatternDistinctPattern(t *testing.T) {
	d := empty()
	RecordFailurePattern(d, "error A", "fix A")
	RecordFailurePattern(d, "error B", "fix B")
	if len(d.FailurePatterns) != 2 {
		t.Errorf("expected 2 distinct patterns, got %d", len(d.FailurePatterns))
	}
}

// --- Prune ---

func TestPruneRemovesOldFlakyTests(t *testing.T) {
	d := empty()
	RecordFlakyTest(d, "TestOld", "pkg")
	// Backdate the entry.
	rec := d.FlakyTests["TestOld"]
	rec.LastSeen = time.Now().UTC().Add(-48 * time.Hour)
	d.FlakyTests["TestOld"] = rec
	RecordFlakyTest(d, "TestNew", "pkg")

	Prune(d, 24*time.Hour)

	if _, ok := d.FlakyTests["TestOld"]; ok {
		t.Error("old flaky test should have been pruned")
	}
	if _, ok := d.FlakyTests["TestNew"]; !ok {
		t.Error("new flaky test should not have been pruned")
	}
}

func TestPruneRemovesOldCoupling(t *testing.T) {
	d := empty()
	RecordFileCoupling(d, []string{"old1.go", "old2.go"})
	d.FileCoupling[0].LastSeen = time.Now().UTC().Add(-48 * time.Hour)
	RecordFileCoupling(d, []string{"new1.go", "new2.go"})

	Prune(d, 24*time.Hour)

	if len(d.FileCoupling) != 1 {
		t.Fatalf("expected 1 coupling entry after prune, got %d", len(d.FileCoupling))
	}
	if d.FileCoupling[0].Files[0] != "new1.go" {
		t.Errorf("wrong entry retained: %v", d.FileCoupling[0].Files)
	}
}

func TestPruneRemovesOldFailurePatterns(t *testing.T) {
	d := empty()
	RecordFailurePattern(d, "old error", "old fix")
	d.FailurePatterns[0].LastSeen = time.Now().UTC().Add(-48 * time.Hour)
	RecordFailurePattern(d, "new error", "new fix")

	Prune(d, 24*time.Hour)

	if len(d.FailurePatterns) != 1 {
		t.Fatalf("expected 1 failure pattern after prune, got %d", len(d.FailurePatterns))
	}
	if d.FailurePatterns[0].Pattern != "new error" {
		t.Errorf("wrong pattern retained: %s", d.FailurePatterns[0].Pattern)
	}
}

func TestPruneKeepsAllWhenNothingExpired(t *testing.T) {
	d := empty()
	RecordFlakyTest(d, "TestRecent", "pkg")
	RecordFileCoupling(d, []string{"a.go", "b.go"})
	RecordFailurePattern(d, "err", "fix")

	Prune(d, 24*time.Hour)

	if len(d.FlakyTests) != 1 {
		t.Errorf("FlakyTests: expected 1, got %d", len(d.FlakyTests))
	}
	if len(d.FileCoupling) != 1 {
		t.Errorf("FileCoupling: expected 1, got %d", len(d.FileCoupling))
	}
	if len(d.FailurePatterns) != 1 {
		t.Errorf("FailurePatterns: expected 1, got %d", len(d.FailurePatterns))
	}
}

func TestPruneDoesNotTouchTestCommandsOrCostHistory(t *testing.T) {
	d := empty()
	RecordTestCommand(d, "pkg", "go test ./...")
	RecordCost(d, "low", 0.10)

	Prune(d, 0) // zero duration prunes everything time-based

	if len(d.TestCommands) != 1 {
		t.Errorf("Prune should not touch TestCommands; got %d", len(d.TestCommands))
	}
	if len(d.CostHistory) != 1 {
		t.Errorf("Prune should not touch CostHistory; got %d", len(d.CostHistory))
	}
}

// --- Concurrent safety ---

func TestConcurrentRecordOperations(t *testing.T) {
	// The package does not promise goroutine-safety on a single *ProjectData
	// without external synchronisation; callers are expected to serialise
	// writes (e.g. load → mutate → save). This test verifies that concurrent
	// Save calls to different project directories don't interfere.
	const workers = 10
	var wg sync.WaitGroup
	errs := make(chan error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			dir := t.TempDir()
			d := empty()
			RecordTestCommand(d, "pkg", "go test ./...")
			RecordCost(d, "low", 0.10)
			if err := Save(dir, d); err != nil {
				errs <- err
				return
			}
			d2, err := Load(dir)
			if err != nil {
				errs <- err
				return
			}
			if SuggestedTestCommand(d2, "pkg") != "go test ./..." {
				errs <- fmt.Errorf("round-trip mismatch in goroutine")
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent error: %v", err)
	}
}

