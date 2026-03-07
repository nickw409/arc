package intelligence

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Store is the project intelligence database, stored at .arc/project.json.
type Store struct {
	mu   sync.Mutex
	path string
	data *Data
}

// Data is the serializable intelligence data.
type Data struct {
	TestCommands       map[string]string     `json:"test_commands,omitempty"`       // package → working test command
	FlakyTests         map[string]FlakyEntry `json:"flaky_tests,omitempty"`         // test name → flaky info
	FileCoupling       []CouplingEntry       `json:"file_coupling,omitempty"`       // frequently co-changed files
	CostHistory        []CostEntry           `json:"cost_history,omitempty"`        // historical cost per complexity
	FailurePatterns    []FailurePattern      `json:"failure_patterns,omitempty"`    // error → fix mappings
	ConventionPatterns []ConventionPattern   `json:"convention_patterns,omitempty"` // observed project conventions
	RateLimitHistory   []RateLimitEvent      `json:"rate_limit_history,omitempty"`  // rate limit events per adapter
	LastUpdated        time.Time             `json:"last_updated"`
}

// RateLimitEvent records a rate limit hit for a specific adapter at a given parallelism level.
type RateLimitEvent struct {
	Adapter   string    `json:"adapter"`
	Parallel  int       `json:"parallel"`
	Timestamp time.Time `json:"timestamp"`
}

// FailurePattern records a known error and its fix for future guidance.
type FailurePattern struct {
	Error    string `json:"error"`     // e.g. "undefined: NewFactory"
	Fix      string `json:"fix"`       // e.g. "add import for the factory package"
	Count    int    `json:"count"`     // how many times this pattern was seen
	LastSeen string `json:"last_seen"` // RFC3339 timestamp
}

// ConventionPattern records an observed project convention.
type ConventionPattern struct {
	Type         string `json:"type"`         // e.g. "test_naming", "file_structure", "import_style"
	Pattern      string `json:"pattern"`      // e.g. "Test files alongside source (*_test.go)"
	Confidence   int    `json:"confidence"`   // 0-100, increases with observations
	Observations int    `json:"observations"` // how many times observed
}

// FlakyEntry tracks pass/fail counts for a test to detect flakiness.
type FlakyEntry struct {
	FailCount int       `json:"fail_count"`
	PassCount int       `json:"pass_count"`
	LastSeen  time.Time `json:"last_seen"`
}

// CouplingEntry records files that are frequently changed together.
type CouplingEntry struct {
	Files []string `json:"files"`
	Count int      `json:"count"` // how many times changed together
}

// CostEntry records the cost of a single plan execution.
type CostEntry struct {
	PlanName   string    `json:"plan_name"`
	Complexity string    `json:"complexity"`
	CostUSD    float64   `json:"cost_usd"`
	Turns      int       `json:"turns"`
	Timestamp  time.Time `json:"timestamp"`
}

// Open loads or creates the intelligence store at the given project root.
// The store is located at <projectRoot>/.arc/project.json.
func Open(projectRoot string) (*Store, error) {
	path := filepath.Join(projectRoot, ".arc", "project.json")
	s := &Store{path: path}

	data, err := os.ReadFile(path)
	if err != nil {
		// File doesn't exist — start fresh.
		s.data = &Data{
			TestCommands: make(map[string]string),
			FlakyTests:   make(map[string]FlakyEntry),
		}
		return s, nil
	}

	var d Data
	if err := json.Unmarshal(data, &d); err != nil {
		// Corrupt file — start fresh.
		s.data = &Data{
			TestCommands: make(map[string]string),
			FlakyTests:   make(map[string]FlakyEntry),
		}
		return s, nil
	}
	if d.TestCommands == nil {
		d.TestCommands = make(map[string]string)
	}
	if d.FlakyTests == nil {
		d.FlakyTests = make(map[string]FlakyEntry)
	}
	s.data = &d
	return s, nil
}

// Save writes the intelligence data to disk atomically.
func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data.LastUpdated = time.Now().UTC()

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating .arc directory: %w", err)
	}

	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling intelligence data: %w", err)
	}
	return os.WriteFile(s.path, data, 0644)
}

// RecordTestCommand stores a working test command for a package.
func (s *Store) RecordTestCommand(pkg, cmd string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.TestCommands[pkg] = cmd
}

// TestCommandFor returns the known test command for a package, or empty string.
func (s *Store) TestCommandFor(pkg string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.TestCommands[pkg]
}

// RecordFlakyTest records a test result that may indicate flakiness.
func (s *Store) RecordFlakyTest(testName string, passed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.data.FlakyTests[testName]
	if passed {
		entry.PassCount++
	} else {
		entry.FailCount++
	}
	entry.LastSeen = time.Now().UTC()
	s.data.FlakyTests[testName] = entry
}

// IsFlaky returns true if a test has failed intermittently (both passes and failures recorded).
func (s *Store) IsFlaky(testName string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.data.FlakyTests[testName]
	if !ok {
		return false
	}
	return entry.FailCount > 0 && entry.PassCount > 0
}

// RecordCost records the cost of a plan execution for future estimation.
// History is capped at 100 entries.
func (s *Store) RecordCost(planName, complexity string, costUSD float64, turns int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.CostHistory = append(s.data.CostHistory, CostEntry{
		PlanName:   planName,
		Complexity: complexity,
		CostUSD:    costUSD,
		Turns:      turns,
		Timestamp:  time.Now().UTC(),
	})
	// Keep last 100 entries.
	if len(s.data.CostHistory) > 100 {
		s.data.CostHistory = s.data.CostHistory[len(s.data.CostHistory)-100:]
	}
}

// RecordFileCoupling records files that were changed together.
// Increments the count if an identical coupling already exists; otherwise appends a new entry.
// Requires at least 2 files.
func (s *Store) RecordFileCoupling(files []string) {
	if len(files) < 2 {
		return
	}
	// Normalize order for consistent comparison.
	normalized := make([]string, len(files))
	copy(normalized, files)
	sort.Strings(normalized)

	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.FileCoupling {
		if sameFiles(s.data.FileCoupling[i].Files, normalized) {
			s.data.FileCoupling[i].Count++
			return
		}
	}
	s.data.FileCoupling = append(s.data.FileCoupling, CouplingEntry{
		Files: normalized,
		Count: 1,
	})
}

// FilterFlakyTests returns only those test names from failing that are NOT known-flaky.
// Tests with both pass and fail history in the store are considered flaky and excluded.
func FilterFlakyTests(s *Store, failing []string) []string {
	if s == nil {
		return failing
	}
	out := make([]string, 0, len(failing))
	for _, name := range failing {
		if !s.IsFlaky(name) {
			out = append(out, name)
		}
	}
	return out
}

// RecordFailurePattern records an error pattern and its fix.
// If a pattern with the same error substring already exists, its count is incremented
// and last_seen is updated. Otherwise a new entry is appended.
// The list is capped at 50 entries; when over capacity the oldest by last_seen is evicted.
func (s *Store) RecordFailurePattern(errorPattern, fix string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)

	// Check for an existing entry whose Error substring matches errorPattern.
	for i := range s.data.FailurePatterns {
		if strings.Contains(s.data.FailurePatterns[i].Error, errorPattern) ||
			strings.Contains(errorPattern, s.data.FailurePatterns[i].Error) {
			s.data.FailurePatterns[i].Count++
			s.data.FailurePatterns[i].LastSeen = now
			return
		}
	}

	// Append new entry.
	s.data.FailurePatterns = append(s.data.FailurePatterns, FailurePattern{
		Error:    errorPattern,
		Fix:      fix,
		Count:    1,
		LastSeen: now,
	})

	// Cap at 50; evict oldest by last_seen.
	if len(s.data.FailurePatterns) > 50 {
		oldestIdx := 0
		for i := 1; i < len(s.data.FailurePatterns); i++ {
			if s.data.FailurePatterns[i].LastSeen < s.data.FailurePatterns[oldestIdx].LastSeen {
				oldestIdx = i
			}
		}
		s.data.FailurePatterns = append(
			s.data.FailurePatterns[:oldestIdx],
			s.data.FailurePatterns[oldestIdx+1:]...,
		)
	}
}

// FindFixForError scans known failure patterns and returns the fix for the first
// pattern whose Error field appears as a substring in errorOutput.
// Returns "" if no match is found.
func (s *Store) FindFixForError(errorOutput string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, fp := range s.data.FailurePatterns {
		if strings.Contains(errorOutput, fp.Error) {
			return fp.Fix
		}
	}
	return ""
}

// RecordConvention records an observed project convention of the given type and pattern.
// If the same type+pattern already exists, observations is incremented and confidence
// is raised by 10 (capped at 100). Otherwise a new entry is added with confidence=30
// and observations=1. The list is capped at 30 entries; when over capacity the entry
// with the lowest confidence is evicted.
func (s *Store) RecordConvention(patternType, pattern string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.data.ConventionPatterns {
		if s.data.ConventionPatterns[i].Type == patternType &&
			s.data.ConventionPatterns[i].Pattern == pattern {
			s.data.ConventionPatterns[i].Observations++
			if s.data.ConventionPatterns[i].Confidence < 100 {
				s.data.ConventionPatterns[i].Confidence += 10
				if s.data.ConventionPatterns[i].Confidence > 100 {
					s.data.ConventionPatterns[i].Confidence = 100
				}
			}
			return
		}
	}

	s.data.ConventionPatterns = append(s.data.ConventionPatterns, ConventionPattern{
		Type:         patternType,
		Pattern:      pattern,
		Confidence:   30,
		Observations: 1,
	})

	// Cap at 30; evict lowest confidence entry.
	if len(s.data.ConventionPatterns) > 30 {
		lowestIdx := 0
		for i := 1; i < len(s.data.ConventionPatterns); i++ {
			if s.data.ConventionPatterns[i].Confidence < s.data.ConventionPatterns[lowestIdx].Confidence {
				lowestIdx = i
			}
		}
		s.data.ConventionPatterns = append(
			s.data.ConventionPatterns[:lowestIdx],
			s.data.ConventionPatterns[lowestIdx+1:]...,
		)
	}
}

// GetConventions returns all conventions of the given type, sorted by confidence descending.
func (s *Store) GetConventions(patternType string) []ConventionPattern {
	s.mu.Lock()
	defer s.mu.Unlock()

	var result []ConventionPattern
	for _, cp := range s.data.ConventionPatterns {
		if cp.Type == patternType {
			result = append(result, cp)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Confidence > result[j].Confidence
	})
	return result
}

// GetAllConventions returns all conventions, sorted by confidence descending.
func (s *Store) GetAllConventions() []ConventionPattern {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]ConventionPattern, len(s.data.ConventionPatterns))
	copy(result, s.data.ConventionPatterns)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Confidence > result[j].Confidence
	})
	return result
}

// RecordRateLimit records a rate limit event for the given adapter at the given parallelism level.
// History is capped at 200 entries; the oldest entries are evicted when over capacity.
// Save is called outside the lock.
func (s *Store) RecordRateLimit(adapter string, parallel int) {
	s.mu.Lock()
	s.data.RateLimitHistory = append(s.data.RateLimitHistory, RateLimitEvent{
		Adapter:   adapter,
		Parallel:  parallel,
		Timestamp: time.Now().UTC(),
	})
	if len(s.data.RateLimitHistory) > 200 {
		s.data.RateLimitHistory = s.data.RateLimitHistory[len(s.data.RateLimitHistory)-200:]
	}
	s.mu.Unlock()

	_ = s.Save()
}

// SuggestMaxParallel returns the suggested max parallelism for an adapter based on rate limit history.
// Returns 0 if there is no history for the adapter.
// Otherwise returns max(1, minParallel-1) where minParallel is the minimum Parallel across all events.
func (s *Store) SuggestMaxParallel(adapter string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	minParallel := -1
	for _, e := range s.data.RateLimitHistory {
		if e.Adapter != adapter {
			continue
		}
		if minParallel < 0 || e.Parallel < minParallel {
			minParallel = e.Parallel
		}
	}

	if minParallel < 0 {
		return 0
	}
	suggested := minParallel - 1
	if suggested < 1 {
		suggested = 1
	}
	return suggested
}

// CountRateLimitEvents returns the number of rate limit events recorded for the given adapter.
func (s *Store) CountRateLimitEvents(adapter string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	count := 0
	for _, e := range s.data.RateLimitHistory {
		if e.Adapter == adapter {
			count++
		}
	}
	return count
}

// sameFiles returns true if two string slices contain exactly the same elements in the same order.
func sameFiles(a, b []string) bool {
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
