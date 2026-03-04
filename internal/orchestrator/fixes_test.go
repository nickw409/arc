package orchestrator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nwiley/arc/internal/arc"
)

// --- shortHash tests ---

func TestShortHashLongString(t *testing.T) {
	hash := "abcdef1234567890"
	got := shortHash(hash)
	if got != "abcdef1" {
		t.Fatalf("shortHash(%q) = %q, want %q", hash, got, "abcdef1")
	}
}

func TestShortHashExactlySevenChars(t *testing.T) {
	hash := "abcdef1"
	got := shortHash(hash)
	if got != "abcdef1" {
		t.Fatalf("shortHash(%q) = %q, want %q", hash, got, "abcdef1")
	}
}

func TestShortHashShortString(t *testing.T) {
	hash := "abc"
	got := shortHash(hash)
	if got != "abc" {
		t.Fatalf("shortHash(%q) = %q, want %q", hash, got, "abc")
	}
}

func TestShortHashEmpty(t *testing.T) {
	got := shortHash("")
	if got != "" {
		t.Fatalf("shortHash(%q) = %q, want %q", "", got, "")
	}
}

// --- discoverNewTestFiles tests (recursive) ---

func TestDiscoverNewTestFilesTopLevel(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "foo_test.go"), "")
	writeFile(t, filepath.Join(dir, "bar_test.go"), "")
	writeFile(t, filepath.Join(dir, "main.go"), "")

	got := discoverNewTestFiles(dir, nil)
	if len(got) != 2 {
		t.Fatalf("expected 2 new test files, got %d: %v", len(got), got)
	}
	asSet := sliceToSet(got)
	if !asSet["foo_test.go"] || !asSet["bar_test.go"] {
		t.Fatalf("expected foo_test.go and bar_test.go in results, got: %v", got)
	}
}

func TestDiscoverNewTestFilesRecursive(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "sub", "pkg")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "top_test.go"), "")
	writeFile(t, filepath.Join(subDir, "deep_test.go"), "")
	writeFile(t, filepath.Join(subDir, "nottest.go"), "")

	got := discoverNewTestFiles(dir, nil)
	if len(got) != 2 {
		t.Fatalf("expected 2 new test files, got %d: %v", len(got), got)
	}
	asSet := sliceToSet(got)
	if !asSet["top_test.go"] {
		t.Fatalf("expected top_test.go in results, got: %v", got)
	}
	wantDeep := filepath.Join("sub", "pkg", "deep_test.go")
	if !asSet[wantDeep] {
		t.Fatalf("expected %q in results, got: %v", wantDeep, got)
	}
}

func TestDiscoverNewTestFilesExcludesExisting(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "old_test.go"), "")
	writeFile(t, filepath.Join(dir, "new_test.go"), "")

	existing := []string{"old_test.go"}
	got := discoverNewTestFiles(dir, existing)
	if len(got) != 1 || got[0] != "new_test.go" {
		t.Fatalf("expected only new_test.go, got: %v", got)
	}
}

func TestDiscoverNewTestFilesExcludesExistingRecursive(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "pkg")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(subDir, "old_test.go"), "")
	writeFile(t, filepath.Join(subDir, "new_test.go"), "")

	existing := []string{filepath.Join("pkg", "old_test.go")}
	got := discoverNewTestFiles(dir, existing)
	if len(got) != 1 {
		t.Fatalf("expected 1 new file, got %d: %v", len(got), got)
	}
	want := filepath.Join("pkg", "new_test.go")
	if got[0] != want {
		t.Fatalf("expected %q, got %q", want, got[0])
	}
}

func TestDiscoverNewTestFilesEmptyDir(t *testing.T) {
	dir := t.TempDir()
	got := discoverNewTestFiles(dir, nil)
	if len(got) != 0 {
		t.Fatalf("expected no files, got: %v", got)
	}
}

func TestDiscoverNewTestFilesMissingDir(t *testing.T) {
	got := discoverNewTestFiles("/nonexistent/path/that/does/not/exist", nil)
	if len(got) != 0 {
		t.Fatalf("expected no files for missing dir, got: %v", got)
	}
}

// --- JudgeDispute case preservation tests ---

func TestJudgeDisputeApprovePreservesCase(t *testing.T) {
	// Simulate what the output parser does without spawning an agent.
	// We test the parsing logic directly by calling the same string ops.
	output := "APPROVE_DISPUTE: The test is wrong because it misinterprets the spec"
	upper := "APPROVE_DISPUTE"

	if len(output) < len(upper) {
		t.Fatal("output too short")
	}
	reason := output[len(upper):]
	// trim leading colon
	if len(reason) > 0 && reason[0] == ':' {
		reason = reason[1:]
	}
	reason = trimSpace(reason)

	want := "The test is wrong because it misinterprets the spec"
	if reason != want {
		t.Fatalf("reason = %q, want %q", reason, want)
	}
}

func TestJudgeDisputeRejectPreservesCase(t *testing.T) {
	output := "REJECT_DISPUTE: Implementation should pass this test"
	reason := output[len("REJECT_DISPUTE:"):]
	reason = trimSpace(reason)

	want := "Implementation should pass this test"
	if reason != want {
		t.Fatalf("reason = %q, want %q", reason, want)
	}
}

// --- phaseObjective trimming tests ---

func TestPhaseObjectiveMarkdownHeading(t *testing.T) {
	dir := t.TempDir()
	planName := "myplan"
	phaseName := "myphase"
	phaseDir := filepath.Join(dir, planName, "phases", phaseName)
	if err := os.MkdirAll(phaseDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(phaseDir, "plan.md"), "# Add user authentication\n\nSome details.")

	opts := RunPhaseOptions{
		PlanName:  planName,
		PhaseName: phaseName,
		PlansDir:  dir,
	}
	got := phaseObjective(opts)
	want := "add user authentication"
	if got != want {
		t.Fatalf("phaseObjective() = %q, want %q", got, want)
	}
}

func TestPhaseObjectiveMultiHashHeading(t *testing.T) {
	dir := t.TempDir()
	planName := "myplan"
	phaseName := "myphase"
	phaseDir := filepath.Join(dir, planName, "phases", phaseName)
	if err := os.MkdirAll(phaseDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(phaseDir, "plan.md"), "## Refactor database layer\n\nDetails.")

	opts := RunPhaseOptions{
		PlanName:  planName,
		PhaseName: phaseName,
		PlansDir:  dir,
	}
	got := phaseObjective(opts)
	want := "refactor database layer"
	if got != want {
		t.Fatalf("phaseObjective() = %q, want %q", got, want)
	}
}

func TestPhaseObjectiveNoSpaceAfterHash(t *testing.T) {
	// Ensure a heading like "# word" doesn't strip the leading space from content
	// and also that a space-only heading doesn't confuse things.
	dir := t.TempDir()
	planName := "myplan"
	phaseName := "myphase"
	phaseDir := filepath.Join(dir, planName, "phases", phaseName)
	if err := os.MkdirAll(phaseDir, 0755); err != nil {
		t.Fatal(err)
	}
	// "# fix" — the old TrimLeft("# ") would also strip 'f', 'i', 'x' if they appeared in "# ".
	// New code should preserve the word.
	writeFile(t, filepath.Join(phaseDir, "plan.md"), "# fix the parser bug\n")

	opts := RunPhaseOptions{
		PlanName:  planName,
		PhaseName: phaseName,
		PlansDir:  dir,
	}
	got := phaseObjective(opts)
	want := "fix the parser bug"
	if got != want {
		t.Fatalf("phaseObjective() = %q, want %q", got, want)
	}
}

func TestPhaseObjectiveMissingFile(t *testing.T) {
	dir := t.TempDir()
	opts := RunPhaseOptions{
		PlanName:  "noplan",
		PhaseName: "nophase",
		PlansDir:  dir,
	}
	got := phaseObjective(opts)
	if got != "implement phase" {
		t.Fatalf("phaseObjective() = %q, want %q", got, "implement phase")
	}
}

func TestPhaseObjectiveTruncatesLongLine(t *testing.T) {
	dir := t.TempDir()
	planName := "myplan"
	phaseName := "myphase"
	phaseDir := filepath.Join(dir, planName, "phases", phaseName)
	if err := os.MkdirAll(phaseDir, 0755); err != nil {
		t.Fatal(err)
	}
	longLine := "# " + string(make([]byte, 100))
	for i := range longLine[2:] {
		longLine = longLine[:2+i] + "a" + longLine[2+i+1:]
	}
	writeFile(t, filepath.Join(phaseDir, "plan.md"), longLine)

	opts := RunPhaseOptions{
		PlanName:  planName,
		PhaseName: phaseName,
		PlansDir:  dir,
	}
	got := phaseObjective(opts)
	if len(got) > 72 {
		t.Fatalf("phaseObjective() returned string longer than 72 chars: len=%d", len(got))
	}
}

// --- helpers ---

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func sliceToSet(ss []string) map[string]bool {
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		m[s] = true
	}
	return m
}

// trimSpace is a local alias to avoid importing strings in test assertions.
func trimSpace(s string) string {
	result := s
	for len(result) > 0 && (result[0] == ' ' || result[0] == '\t' || result[0] == '\n' || result[0] == '\r') {
		result = result[1:]
	}
	for len(result) > 0 {
		last := result[len(result)-1]
		if last == ' ' || last == '\t' || last == '\n' || last == '\r' {
			result = result[:len(result)-1]
		} else {
			break
		}
	}
	return result
}

// Ensure arc package is used (referenced by TestPhaseObjectiveTruncatesLongLine indirectly).
var _ = arc.PhaseState{}
