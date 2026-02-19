package project

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Detection holds the results of project auto-detection.
type Detection struct {
	Language       string // rust, go, typescript, python, unknown
	Runner         string // cargo-nextest, go-test, vitest, pytest, unknown
	DefaultPackage string // extracted from manifest
	BuildCommand   string // language-specific default
	TestCommand    string // language-specific default
}

// Detect inspects the project root for language indicators.
// Checks in priority order: Cargo.toml, go.mod, package.json, pyproject.toml/setup.py/requirements.txt.
func Detect(projectRoot string) *Detection {
	// 1. Cargo.toml -> rust
	if _, err := os.Stat(filepath.Join(projectRoot, "Cargo.toml")); err == nil {
		return &Detection{
			Language:       "rust",
			Runner:         "cargo-nextest",
			DefaultPackage: extractCargoPackageName(filepath.Join(projectRoot, "Cargo.toml")),
			BuildCommand:   "cargo build",
			TestCommand:    "cargo nextest run",
		}
	}

	// 2. go.mod -> go
	if _, err := os.Stat(filepath.Join(projectRoot, "go.mod")); err == nil {
		return &Detection{
			Language:       "go",
			Runner:         "go-test",
			DefaultPackage: extractGoModuleName(filepath.Join(projectRoot, "go.mod")),
			BuildCommand:   "go build ./...",
			TestCommand:    "go test ./...",
		}
	}

	// 3. package.json -> typescript
	if _, err := os.Stat(filepath.Join(projectRoot, "package.json")); err == nil {
		return &Detection{
			Language:       "typescript",
			Runner:         "vitest",
			DefaultPackage: extractPackageJsonName(filepath.Join(projectRoot, "package.json")),
			BuildCommand:   "npm run build",
			TestCommand:    "npx vitest run",
		}
	}

	// 4. pyproject.toml / setup.py / requirements.txt -> python
	for _, indicator := range []string{"pyproject.toml", "setup.py", "requirements.txt"} {
		if _, err := os.Stat(filepath.Join(projectRoot, indicator)); err == nil {
			return &Detection{
				Language:       "python",
				Runner:         "pytest",
				DefaultPackage: "",
				BuildCommand:   "",
				TestCommand:    "pytest",
			}
		}
	}

	return &Detection{
		Language:       "unknown",
		Runner:         "unknown",
		DefaultPackage: "",
		BuildCommand:   "",
		TestCommand:    "",
	}
}

func extractCargoPackageName(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	content := string(data)
	inPackage := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "[package]" {
			inPackage = true
			continue
		}
		if strings.HasPrefix(trimmed, "[") && trimmed != "[package]" {
			inPackage = false
			continue
		}
		if inPackage && strings.HasPrefix(trimmed, "name") {
			parts := strings.SplitN(trimmed, "=", 2)
			if len(parts) == 2 {
				name := strings.TrimSpace(parts[1])
				name = strings.Trim(name, "\"'")
				return name
			}
		}
	}
	return ""
}

func extractGoModuleName(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			modulePath := strings.TrimPrefix(line, "module ")
			modulePath = strings.TrimSpace(modulePath)
			parts := strings.Split(modulePath, "/")
			return parts[len(parts)-1]
		}
	}
	return ""
}

func extractPackageJsonName(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var pkg struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return ""
	}
	return pkg.Name
}
