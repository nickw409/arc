package validate

import (
	"os"
	"path/filepath"
	"strings"
)

// FileEntry represents a single source or test file.
type FileEntry struct {
	Path    string
	Content string
	IsTest  bool
	Lines   int
}

// Batch groups files belonging to the same package directory.
type Batch struct {
	Package string
	Files   []FileEntry
	Lines   int
}

// ScanOptions configures file discovery.
type ScanOptions struct {
	Paths         []string
	Language      string
	MaxBatchLines int
}

const defaultMaxBatchLines = 3000

var skipDirs = map[string]bool{
	"vendor":       true,
	"node_modules": true,
	".git":         true,
	"testdata":     true,
}

// Scan walks the target paths, reads files, classifies them, and groups by directory.
func Scan(opts ScanOptions) ([]Batch, error) {
	maxLines := opts.MaxBatchLines
	if maxLines == 0 {
		maxLines = defaultMaxBatchLines
	}

	// Collect files grouped by directory.
	groups := make(map[string][]FileEntry)
	var dirOrder []string

	for _, root := range opts.Paths {
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // skip unreadable entries
			}
			if info.IsDir() {
				if skipDirs[info.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			if !isSourceOrTest(info.Name(), opts.Language) {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil // skip unreadable files
			}
			content := string(data)
			lines := strings.Count(content, "\n") + 1

			dir := filepath.Dir(path)
			if _, ok := groups[dir]; !ok {
				dirOrder = append(dirOrder, dir)
			}
			groups[dir] = append(groups[dir], FileEntry{
				Path:    path,
				Content: content,
				IsTest:  isTestFile(info.Name(), opts.Language),
				Lines:   lines,
			})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	var batches []Batch
	for _, dir := range dirOrder {
		files := groups[dir]
		totalLines := 0
		for _, f := range files {
			totalLines += f.Lines
		}
		b := Batch{
			Package: dir,
			Files:   files,
			Lines:   totalLines,
		}
		if totalLines > maxLines {
			batches = append(batches, splitBatch(b, maxLines)...)
		} else {
			batches = append(batches, b)
		}
	}

	return batches, nil
}

// isSourceOrTest returns true if the filename matches source or test patterns for the language.
func isSourceOrTest(name, language string) bool {
	switch language {
	case "go":
		return strings.HasSuffix(name, ".go")
	case "python":
		return strings.HasSuffix(name, ".py")
	case "typescript":
		return strings.HasSuffix(name, ".ts") || strings.HasSuffix(name, ".tsx")
	default:
		// unknown: accept all supported extensions
		return strings.HasSuffix(name, ".go") ||
			strings.HasSuffix(name, ".py") ||
			strings.HasSuffix(name, ".ts") || strings.HasSuffix(name, ".tsx")
	}
}

// isTestFile returns true if the filename matches test file patterns for the language.
func isTestFile(name, language string) bool {
	switch language {
	case "go":
		return strings.HasSuffix(name, "_test.go")
	case "python":
		return strings.HasPrefix(name, "test_") && strings.HasSuffix(name, ".py") ||
			strings.HasSuffix(name, "_test.py")
	case "typescript":
		return strings.HasSuffix(name, ".test.ts") || strings.HasSuffix(name, ".spec.ts") ||
			strings.HasSuffix(name, ".test.tsx") || strings.HasSuffix(name, ".spec.tsx")
	default:
		// unknown: check all patterns
		return strings.HasSuffix(name, "_test.go") ||
			(strings.HasPrefix(name, "test_") && strings.HasSuffix(name, ".py")) ||
			strings.HasSuffix(name, "_test.py") ||
			strings.HasSuffix(name, ".test.ts") || strings.HasSuffix(name, ".spec.ts") ||
			strings.HasSuffix(name, ".test.tsx") || strings.HasSuffix(name, ".spec.tsx")
	}
}

// splitBatch splits an oversized batch. Source files are duplicated into both halves;
// test files are split evenly.
func splitBatch(b Batch, maxLines int) []Batch {
	var sources, tests []FileEntry
	for _, f := range b.Files {
		if f.IsTest {
			tests = append(tests, f)
		} else {
			sources = append(sources, f)
		}
	}

	if len(tests) <= 1 {
		// Can't split further meaningfully.
		return []Batch{b}
	}

	mid := len(tests) / 2
	left := tests[:mid]
	right := tests[mid:]

	makeBatch := func(pkg string, srcs, tsts []FileEntry) Batch {
		files := make([]FileEntry, 0, len(srcs)+len(tsts))
		files = append(files, srcs...)
		files = append(files, tsts...)
		lines := 0
		for _, f := range files {
			lines += f.Lines
		}
		return Batch{Package: pkg, Files: files, Lines: lines}
	}

	b1 := makeBatch(b.Package, sources, left)
	b2 := makeBatch(b.Package, sources, right)

	var result []Batch
	if b1.Lines > maxLines {
		result = append(result, splitBatch(b1, maxLines)...)
	} else {
		result = append(result, b1)
	}
	if b2.Lines > maxLines {
		result = append(result, splitBatch(b2, maxLines)...)
	} else {
		result = append(result, b2)
	}
	return result
}
