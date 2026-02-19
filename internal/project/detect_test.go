package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectGoProject(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module github.com/example/myapp\n\ngo 1.22\n")

	d := Detect(dir)
	if d.Language != "go" {
		t.Fatalf("Language = %q, want %q", d.Language, "go")
	}
	if d.Runner != "go-test" {
		t.Fatalf("Runner = %q, want %q", d.Runner, "go-test")
	}
	if d.DefaultPackage != "myapp" {
		t.Fatalf("DefaultPackage = %q, want %q", d.DefaultPackage, "myapp")
	}
	if d.BuildCommand != "go build ./..." {
		t.Fatalf("BuildCommand = %q, want %q", d.BuildCommand, "go build ./...")
	}
	if d.TestCommand != "go test ./..." {
		t.Fatalf("TestCommand = %q, want %q", d.TestCommand, "go test ./...")
	}
}

func TestDetectRustProject(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "Cargo.toml"), "[package]\nname = \"myapp\"\nversion = \"0.1.0\"\n")

	d := Detect(dir)
	if d.Language != "rust" {
		t.Fatalf("Language = %q, want %q", d.Language, "rust")
	}
	if d.Runner != "cargo-nextest" {
		t.Fatalf("Runner = %q, want %q", d.Runner, "cargo-nextest")
	}
	if d.DefaultPackage != "myapp" {
		t.Fatalf("DefaultPackage = %q, want %q", d.DefaultPackage, "myapp")
	}
}

func TestDetectTypescriptProject(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"), `{"name": "myapp"}`)

	d := Detect(dir)
	if d.Language != "typescript" {
		t.Fatalf("Language = %q, want %q", d.Language, "typescript")
	}
	if d.Runner != "vitest" {
		t.Fatalf("Runner = %q, want %q", d.Runner, "vitest")
	}
	if d.DefaultPackage != "myapp" {
		t.Fatalf("DefaultPackage = %q, want %q", d.DefaultPackage, "myapp")
	}
}

func TestDetectPythonProject(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pyproject.toml"), "[build-system]\n")

	d := Detect(dir)
	if d.Language != "python" {
		t.Fatalf("Language = %q, want %q", d.Language, "python")
	}
	if d.Runner != "pytest" {
		t.Fatalf("Runner = %q, want %q", d.Runner, "pytest")
	}
}

func TestDetectPythonSetupPy(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "setup.py"), "from setuptools import setup\n")

	d := Detect(dir)
	if d.Language != "python" {
		t.Fatalf("Language = %q, want %q", d.Language, "python")
	}
	if d.Runner != "pytest" {
		t.Fatalf("Runner = %q, want %q", d.Runner, "pytest")
	}
}

func TestDetectPythonRequirementsTxt(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "requirements.txt"), "flask==2.0\n")

	d := Detect(dir)
	if d.Language != "python" {
		t.Fatalf("Language = %q, want %q", d.Language, "python")
	}
	if d.Runner != "pytest" {
		t.Fatalf("Runner = %q, want %q", d.Runner, "pytest")
	}
}

func TestDetectCargoNoPackageSection(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "Cargo.toml"), "[workspace]\nmembers = [\"crate-a\"]\n")

	d := Detect(dir)
	if d.Language != "rust" {
		t.Fatalf("Language = %q, want %q", d.Language, "rust")
	}
	if d.Runner != "cargo-nextest" {
		t.Fatalf("Runner = %q, want %q", d.Runner, "cargo-nextest")
	}
	if d.DefaultPackage != "" {
		t.Fatalf("DefaultPackage = %q, want empty string", d.DefaultPackage)
	}
}

func TestDetectGoModNoModule(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "go 1.22\n")

	d := Detect(dir)
	if d.Language != "go" {
		t.Fatalf("Language = %q, want %q", d.Language, "go")
	}
	if d.Runner != "go-test" {
		t.Fatalf("Runner = %q, want %q", d.Runner, "go-test")
	}
	if d.DefaultPackage != "" {
		t.Fatalf("DefaultPackage = %q, want empty string", d.DefaultPackage)
	}
}

func TestDetectUnknownProject(t *testing.T) {
	dir := t.TempDir()

	d := Detect(dir)
	if d.Language != "unknown" {
		t.Fatalf("Language = %q, want %q", d.Language, "unknown")
	}
	if d.Runner != "unknown" {
		t.Fatalf("Runner = %q, want %q", d.Runner, "unknown")
	}
	if d.DefaultPackage != "" {
		t.Fatalf("DefaultPackage = %q, want empty string", d.DefaultPackage)
	}
}

func TestDetectPriorityOrder(t *testing.T) {
	dir := t.TempDir()
	// Both go.mod and package.json exist; Go should win (checked before TS)
	writeFile(t, filepath.Join(dir, "go.mod"), "module github.com/example/myapp\n\ngo 1.22\n")
	writeFile(t, filepath.Join(dir, "package.json"), `{"name": "myapp"}`)

	d := Detect(dir)
	if d.Language != "go" {
		t.Fatalf("Language = %q, want %q (Go should win priority)", d.Language, "go")
	}
}

func TestDetectCargoTomlInvalid(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "Cargo.toml"), "[package\nbroken")

	d := Detect(dir)
	if d.Language != "rust" {
		t.Fatalf("Language = %q, want %q", d.Language, "rust")
	}
	if d.Runner != "cargo-nextest" {
		t.Fatalf("Runner = %q, want %q", d.Runner, "cargo-nextest")
	}
	if d.DefaultPackage != "" {
		t.Fatalf("DefaultPackage = %q, want empty string (invalid TOML)", d.DefaultPackage)
	}
}

func TestDetectGoModMultiSegment(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module github.com/org/repo/v2\n\ngo 1.22\n")

	d := Detect(dir)
	if d.Language != "go" {
		t.Fatalf("Language = %q, want %q", d.Language, "go")
	}
	if d.DefaultPackage != "v2" {
		t.Fatalf("DefaultPackage = %q, want %q (last path segment)", d.DefaultPackage, "v2")
	}
}

func TestDetectPackageJsonInvalid(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"), "{invalid json")

	d := Detect(dir)
	if d.Language != "typescript" {
		t.Fatalf("Language = %q, want %q", d.Language, "typescript")
	}
	if d.Runner != "vitest" {
		t.Fatalf("Runner = %q, want %q", d.Runner, "vitest")
	}
	if d.DefaultPackage != "" {
		t.Fatalf("DefaultPackage = %q, want empty string (invalid JSON)", d.DefaultPackage)
	}
}

func TestDetectCargoTomlEmpty(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "Cargo.toml"), "")

	d := Detect(dir)
	if d.Language != "rust" {
		t.Fatalf("Language = %q, want %q", d.Language, "rust")
	}
	if d.Runner != "cargo-nextest" {
		t.Fatalf("Runner = %q, want %q", d.Runner, "cargo-nextest")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writeFile(%s) error: %v", path, err)
	}
}
