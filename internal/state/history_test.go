package state

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestAppendHistory(t *testing.T) {
	dir := t.TempDir()

	if err := AppendHistory(dir, "line 1"); err != nil {
		t.Fatalf("AppendHistory error: %v", err)
	}
	if err := AppendHistory(dir, "line 2"); err != nil {
		t.Fatalf("AppendHistory error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "history.md"))
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("len(lines) = %d, want 2", len(lines))
	}
	if lines[0] != "line 1" {
		t.Fatalf("lines[0] = %q, want %q", lines[0], "line 1")
	}
	if lines[1] != "line 2" {
		t.Fatalf("lines[1] = %q, want %q", lines[1], "line 2")
	}
}

func TestAppendHistoryCreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.md")

	// Verify file doesn't exist yet
	if _, err := os.Stat(path); err == nil {
		t.Fatal("history.md should not exist before AppendHistory")
	}

	if err := AppendHistory(dir, "first entry"); err != nil {
		t.Fatalf("AppendHistory error: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("history.md should exist after AppendHistory: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if strings.TrimSpace(string(data)) != "first entry" {
		t.Fatalf("content = %q, want %q", strings.TrimSpace(string(data)), "first entry")
	}
}

func TestAppendHistoryConcurrent(t *testing.T) {
	dir := t.TempDir()

	var wg sync.WaitGroup
	n := 10
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			entry := strings.Repeat("x", 50) // consistent-length entries
			if err := AppendHistory(dir, entry); err != nil {
				t.Errorf("AppendHistory goroutine %d error: %v", idx, err)
			}
		}(i)
	}
	wg.Wait()

	data, err := os.ReadFile(filepath.Join(dir, "history.md"))
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != n {
		t.Fatalf("len(lines) = %d, want %d", len(lines), n)
	}
	// Verify no interleaving — each line should be exactly the expected content
	expected := strings.Repeat("x", 50)
	for i, line := range lines {
		if line != expected {
			t.Fatalf("line %d = %q, want %q (possible interleaving)", i, line, expected)
		}
	}
}
