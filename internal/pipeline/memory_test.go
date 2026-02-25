package pipeline

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractMemoryPresent(t *testing.T) {
	output := "Some analysis\n\n## Memory\nsome notes\n## Verdict\ndone"
	got := ExtractMemory(output)
	if got != "some notes" {
		t.Fatalf("got %q, want %q", got, "some notes")
	}
}

func TestExtractMemoryAbsent(t *testing.T) {
	output := "Some analysis\n\n## Verdict\ndone"
	got := ExtractMemory(output)
	if got != "" {
		t.Fatalf("got %q, want empty string", got)
	}
}

func TestExtractMemoryAtEnd(t *testing.T) {
	output := "## Memory\nsome notes"
	got := ExtractMemory(output)
	if got != "some notes" {
		t.Fatalf("got %q, want %q", got, "some notes")
	}
}

func TestExtractMemoryAdjacentToVerdict(t *testing.T) {
	output := "## Memory\nnotes\n\n## Verdict\napproved"
	got := ExtractMemory(output)
	if got != "notes" {
		t.Fatalf("got %q, want %q", got, "notes")
	}
}

func TestExtractMemoryMultiLine(t *testing.T) {
	output := "## Memory\nline one\nline two\nline three\n\n## Verdict\ndone"
	got := ExtractMemory(output)
	want := "line one\nline two\nline three"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestExtractMemoryEmpty(t *testing.T) {
	got := ExtractMemory("")
	if got != "" {
		t.Fatalf("got %q, want empty for empty output", got)
	}
}

func TestWriteReadMemoryRoundTrip(t *testing.T) {
	phaseDir := t.TempDir()
	content := "explored files A and B\nfailed approach: X\ncurrent state: Y"

	if err := WriteMemory(phaseDir, "impl", content); err != nil {
		t.Fatalf("WriteMemory failed: %v", err)
	}

	got, err := ReadMemory(phaseDir, "impl")
	if err != nil {
		t.Fatalf("ReadMemory failed: %v", err)
	}
	if got != content {
		t.Fatalf("got %q, want %q", got, content)
	}
}

func TestReadMemoryMissingFile(t *testing.T) {
	phaseDir := t.TempDir()

	got, err := ReadMemory(phaseDir, "nonexistent_state")
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty string for missing file, got %q", got)
	}
}

func TestWriteMemoryCreatesDirectory(t *testing.T) {
	phaseDir := t.TempDir()
	// memory/ subdir does not exist yet
	memDir := filepath.Join(phaseDir, "memory")
	if _, err := os.Stat(memDir); !os.IsNotExist(err) {
		t.Fatal("expected memory dir to not exist before write")
	}

	if err := WriteMemory(phaseDir, "qa", "notes"); err != nil {
		t.Fatalf("WriteMemory failed: %v", err)
	}

	if _, err := os.Stat(memDir); err != nil {
		t.Fatalf("expected memory dir to exist after write: %v", err)
	}
}

func TestWriteMemoryOverwrites(t *testing.T) {
	phaseDir := t.TempDir()

	if err := WriteMemory(phaseDir, "impl", "first"); err != nil {
		t.Fatalf("first write failed: %v", err)
	}
	if err := WriteMemory(phaseDir, "impl", "second"); err != nil {
		t.Fatalf("second write failed: %v", err)
	}

	got, err := ReadMemory(phaseDir, "impl")
	if err != nil {
		t.Fatalf("ReadMemory failed: %v", err)
	}
	if got != "second" {
		t.Fatalf("got %q, want %q (should be overwritten)", got, "second")
	}
}

func TestReadMemoryDifferentStates(t *testing.T) {
	phaseDir := t.TempDir()

	if err := WriteMemory(phaseDir, "qa", "qa notes"); err != nil {
		t.Fatal(err)
	}
	if err := WriteMemory(phaseDir, "impl", "impl notes"); err != nil {
		t.Fatal(err)
	}

	qa, err := ReadMemory(phaseDir, "qa")
	if err != nil {
		t.Fatal(err)
	}
	impl, err := ReadMemory(phaseDir, "impl")
	if err != nil {
		t.Fatal(err)
	}

	if qa != "qa notes" {
		t.Errorf("qa memory = %q, want %q", qa, "qa notes")
	}
	if impl != "impl notes" {
		t.Errorf("impl memory = %q, want %q", impl, "impl notes")
	}
}
