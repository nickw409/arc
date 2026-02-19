package validate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanGoProject(t *testing.T) {
	dir := t.TempDir()
	pkg := filepath.Join(dir, "mypkg")
	if err := os.MkdirAll(pkg, 0755); err != nil {
		t.Fatal(err)
	}

	writeFile(t, filepath.Join(pkg, "foo.go"), "package mypkg\nfunc Foo() {}\n")
	writeFile(t, filepath.Join(pkg, "foo_test.go"), "package mypkg\nfunc TestFoo(t *testing.T) {}\n")

	batches, err := Scan(ScanOptions{Paths: []string{dir}, Language: "go"})
	if err != nil {
		t.Fatalf("Scan() error: %v", err)
	}
	if len(batches) != 1 {
		t.Fatalf("len(batches) = %d, want 1", len(batches))
	}

	b := batches[0]
	if b.Package != pkg {
		t.Fatalf("Package = %q, want %q", b.Package, pkg)
	}
	if len(b.Files) != 2 {
		t.Fatalf("len(Files) = %d, want 2", len(b.Files))
	}

	var srcCount, testCount int
	for _, f := range b.Files {
		if f.IsTest {
			testCount++
		} else {
			srcCount++
		}
	}
	if srcCount != 1 {
		t.Fatalf("source count = %d, want 1", srcCount)
	}
	if testCount != 1 {
		t.Fatalf("test count = %d, want 1", testCount)
	}
}

func TestScanEmptyDir(t *testing.T) {
	dir := t.TempDir()
	batches, err := Scan(ScanOptions{Paths: []string{dir}, Language: "go"})
	if err != nil {
		t.Fatalf("Scan() error: %v", err)
	}
	if len(batches) != 0 {
		t.Fatalf("len(batches) = %d, want 0", len(batches))
	}
}

func TestScanSkipsVendor(t *testing.T) {
	dir := t.TempDir()
	vendor := filepath.Join(dir, "vendor", "lib")
	if err := os.MkdirAll(vendor, 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(vendor, "lib.go"), "package lib\n")

	pkg := filepath.Join(dir, "mypkg")
	if err := os.MkdirAll(pkg, 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(pkg, "main.go"), "package mypkg\n")

	batches, err := Scan(ScanOptions{Paths: []string{dir}, Language: "go"})
	if err != nil {
		t.Fatalf("Scan() error: %v", err)
	}

	for _, b := range batches {
		for _, f := range b.Files {
			if strings.Contains(f.Path, "vendor") {
				t.Fatalf("vendor file included: %s", f.Path)
			}
		}
	}
}

func TestSplitBatchUnderLimit(t *testing.T) {
	b := Batch{
		Package: "pkg",
		Files: []FileEntry{
			{Path: "a.go", Lines: 50},
			{Path: "a_test.go", Lines: 50, IsTest: true},
		},
		Lines: 100,
	}

	result := splitBatch(b, 3000)
	if len(result) != 1 {
		t.Fatalf("len(result) = %d, want 1", len(result))
	}
}

func TestSplitBatchOverLimit(t *testing.T) {
	b := Batch{
		Package: "pkg",
		Files: []FileEntry{
			{Path: "src.go", Content: "source", Lines: 100, IsTest: false},
			{Path: "a_test.go", Content: "test a", Lines: 200, IsTest: true},
			{Path: "b_test.go", Content: "test b", Lines: 200, IsTest: true},
			{Path: "c_test.go", Content: "test c", Lines: 200, IsTest: true},
			{Path: "d_test.go", Content: "test d", Lines: 200, IsTest: true},
		},
		Lines: 900,
	}

	result := splitBatch(b, 400)
	if len(result) < 2 {
		t.Fatalf("len(result) = %d, want >= 2", len(result))
	}

	// Source file should appear in every batch.
	for i, rb := range result {
		hasSrc := false
		for _, f := range rb.Files {
			if !f.IsTest {
				hasSrc = true
			}
		}
		if !hasSrc {
			t.Fatalf("batch %d missing source files", i)
		}
	}
}

func TestIsTestFileGo(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"foo_test.go", true},
		{"foo.go", false},
		{"test_foo.go", false},
	}
	for _, tt := range tests {
		if got := isTestFile(tt.name, "go"); got != tt.want {
			t.Errorf("isTestFile(%q, go) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestIsTestFilePython(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"test_foo.py", true},
		{"foo_test.py", true},
		{"foo.py", false},
		{"testfoo.py", false},
	}
	for _, tt := range tests {
		if got := isTestFile(tt.name, "python"); got != tt.want {
			t.Errorf("isTestFile(%q, python) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
