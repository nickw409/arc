package intelligence

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const (
	arcDir   = ".arc"
	dataFile = "project.json"
	version  = 1

	// FlakyThreshold is the minimum number of occurrences before a test is
	// considered flaky.
	FlakyThreshold = 3
)

// ProjectData holds accumulated project intelligence.
type ProjectData struct {
	Version         int                    `json:"version"`
	Updated         time.Time              `json:"updated"`
	TestCommands    map[string]string      `json:"test_commands"`    // package path → test command
	FlakyTests      map[string]FlakyRecord `json:"flaky_tests"`     // test name → flaky info
	FileCoupling    []CouplingEntry        `json:"file_coupling"`   // frequently co-changed files
	CostHistory     map[string]CostStats   `json:"cost_history"`    // complexity → cost stats
	FailurePatterns []FailurePattern       `json:"failure_patterns"` // error → fix mapping
}

// FlakyRecord tracks a test that intermittently fails.
type FlakyRecord struct {
	TestName    string    `json:"test_name"`
	Package     string    `json:"package"`
	Occurrences int       `json:"occurrences"`
	LastSeen    time.Time `json:"last_seen"`
}

// CouplingEntry records files that are frequently changed together.
type CouplingEntry struct {
	Files     []string  `json:"files"`
	CoChanges int       `json:"co_changes"`
	LastSeen  time.Time `json:"last_seen"`
}

// CostStats holds cost statistics for a complexity tier.
type CostStats struct {
	Count     int     `json:"count"`
	TotalCost float64 `json:"total_cost"`
	AvgCost   float64 `json:"avg_cost"`
	MinCost   float64 `json:"min_cost"`
	MaxCost   float64 `json:"max_cost"`
}

// FailurePattern records a known error string and the fix that resolved it.
type FailurePattern struct {
	Pattern     string    `json:"pattern"`     // error substring to match
	Fix         string    `json:"fix"`         // what fixed it
	Occurrences int       `json:"occurrences"`
	LastSeen    time.Time `json:"last_seen"`
}

// Load reads project intelligence from <projectDir>/.arc/project.json.
// Returns an empty, initialised ProjectData when the file does not exist.
func Load(projectDir string) (*ProjectData, error) {
	path := dataPath(projectDir)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return empty(), nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var d ProjectData
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	// Ensure maps are non-nil after unmarshal.
	if d.TestCommands == nil {
		d.TestCommands = make(map[string]string)
	}
	if d.FlakyTests == nil {
		d.FlakyTests = make(map[string]FlakyRecord)
	}
	if d.CostHistory == nil {
		d.CostHistory = make(map[string]CostStats)
	}
	return &d, nil
}

// Save atomically writes data to <projectDir>/.arc/project.json.
func Save(projectDir string, data *ProjectData) error {
	dir := filepath.Join(projectDir, arcDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	data.Updated = time.Now().UTC()
	data.Version = version

	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling project data: %w", err)
	}

	tmp, err := os.CreateTemp(dir, "project.json.*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("closing temp file: %w", err)
	}
	dest := dataPath(projectDir)
	if err := os.Rename(tmpPath, dest); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming temp file: %w", err)
	}
	return nil
}

// RecordTestCommand records that cmd is the working test command for pkg.
func RecordTestCommand(data *ProjectData, pkg, cmd string) {
	data.TestCommands[pkg] = cmd
}

// RecordFlakyTest increments the flaky occurrence counter for testName in pkg.
func RecordFlakyTest(data *ProjectData, testName, pkg string) {
	rec := data.FlakyTests[testName]
	rec.TestName = testName
	rec.Package = pkg
	rec.Occurrences++
	rec.LastSeen = time.Now().UTC()
	data.FlakyTests[testName] = rec
}

// RecordFileCoupling records that the given files were changed together.
// If an identical file set already exists, its counter is incremented.
// files must contain at least two entries; single-file sets are ignored.
func RecordFileCoupling(data *ProjectData, files []string) {
	if len(files) < 2 {
		return
	}
	// Normalise: sort so order doesn't matter for dedup.
	key := make([]string, len(files))
	copy(key, files)
	sort.Strings(key)

	for i, entry := range data.FileCoupling {
		if equalSorted(entry.Files, key) {
			data.FileCoupling[i].CoChanges++
			data.FileCoupling[i].LastSeen = time.Now().UTC()
			return
		}
	}
	data.FileCoupling = append(data.FileCoupling, CouplingEntry{
		Files:     key,
		CoChanges: 1,
		LastSeen:  time.Now().UTC(),
	})
}

// RecordCost records the actual cost for a given complexity tier and updates
// running statistics.
func RecordCost(data *ProjectData, complexity string, cost float64) {
	s := data.CostHistory[complexity]
	s.Count++
	s.TotalCost += cost
	s.AvgCost = s.TotalCost / float64(s.Count)
	if s.Count == 1 {
		s.MinCost = cost
		s.MaxCost = cost
	} else {
		if cost < s.MinCost {
			s.MinCost = cost
		}
		if cost > s.MaxCost {
			s.MaxCost = cost
		}
	}
	data.CostHistory[complexity] = s
}

// RecordFailurePattern records that pattern was observed and fix resolved it.
// If the same (pattern, fix) pair already exists, its counter is incremented.
func RecordFailurePattern(data *ProjectData, pattern, fix string) {
	for i, fp := range data.FailurePatterns {
		if fp.Pattern == pattern && fp.Fix == fix {
			data.FailurePatterns[i].Occurrences++
			data.FailurePatterns[i].LastSeen = time.Now().UTC()
			return
		}
	}
	data.FailurePatterns = append(data.FailurePatterns, FailurePattern{
		Pattern:     pattern,
		Fix:         fix,
		Occurrences: 1,
		LastSeen:    time.Now().UTC(),
	})
}

// IsFlaky reports whether testName is known-flaky (FlakyThreshold or more
// recorded occurrences).
func IsFlaky(data *ProjectData, testName string) bool {
	rec, ok := data.FlakyTests[testName]
	return ok && rec.Occurrences >= FlakyThreshold
}

// SuggestedTestCommand returns the known test command for pkg, or an empty
// string if none has been recorded.
func SuggestedTestCommand(data *ProjectData, pkg string) string {
	return data.TestCommands[pkg]
}

// EstimateCost returns the average historical cost for the given complexity
// tier, or 0 if no data is available.
func EstimateCost(data *ProjectData, complexity string) float64 {
	s, ok := data.CostHistory[complexity]
	if !ok || s.Count == 0 {
		return 0
	}
	return s.AvgCost
}

// Prune removes entries that have not been seen within maxAge.
func Prune(data *ProjectData, maxAge time.Duration) {
	cutoff := time.Now().UTC().Add(-maxAge)

	for name, rec := range data.FlakyTests {
		if rec.LastSeen.Before(cutoff) {
			delete(data.FlakyTests, name)
		}
	}

	kept := data.FileCoupling[:0]
	for _, entry := range data.FileCoupling {
		if !entry.LastSeen.Before(cutoff) {
			kept = append(kept, entry)
		}
	}
	data.FileCoupling = kept

	keptFP := data.FailurePatterns[:0]
	for _, fp := range data.FailurePatterns {
		if !fp.LastSeen.Before(cutoff) {
			keptFP = append(keptFP, fp)
		}
	}
	data.FailurePatterns = keptFP
}

// --- helpers ---

func dataPath(projectDir string) string {
	return filepath.Join(projectDir, arcDir, dataFile)
}

func empty() *ProjectData {
	return &ProjectData{
		Version:      version,
		TestCommands: make(map[string]string),
		FlakyTests:   make(map[string]FlakyRecord),
		CostHistory:  make(map[string]CostStats),
	}
}

// equalSorted compares two pre-sorted string slices for equality.
func equalSorted(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
