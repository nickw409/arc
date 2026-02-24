package plan

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nwiley/arc/internal/arc"
)

func TestGenerateSummary_AllComplete(t *testing.T) {
	dir := t.TempDir()
	planDir := filepath.Join(dir, "test-plan")
	os.MkdirAll(filepath.Join(planDir, "phases", "phase-a"), 0755)
	os.MkdirAll(filepath.Join(planDir, "phases", "phase-b"), 0755)

	os.WriteFile(filepath.Join(planDir, "phases", "phase-a", "plan.md"), []byte("## Objective\n\nFix the widget\n\n## Details\n\nMore info"), 0644)

	meta := &arc.PlanMeta{
		Name:   "test-plan",
		Phases: []string{"phase-a", "phase-b"},
	}
	states := map[string]*arc.PhaseState{
		"phase-a": {PhaseStatus: "complete", Iteration: arc.Iteration{Current: 5}, TestsPassing: 10, TestsTotal: 10, LastCommit: "abc1234567890", Usage: arc.Usage{CostUSD: 0.50}},
		"phase-b": {PhaseStatus: "complete", Iteration: arc.Iteration{Current: 3}, TestsPassing: 5, TestsTotal: 5, LastCommit: "def5678901234", Usage: arc.Usage{CostUSD: 0.30}},
	}

	content, err := GenerateSummary(SummaryOptions{
		PlanDir:     planDir,
		PlanName:    "test-plan",
		Meta:        meta,
		PhaseStates: states,
		ProjectDir:  dir,
	})
	if err != nil {
		t.Fatalf("GenerateSummary error: %v", err)
	}

	checks := []string{"2/2 complete", "abc1234", "def5678", "$0.8000", "Fix the widget"}
	for _, c := range checks {
		if !strings.Contains(content, c) {
			t.Errorf("summary missing %q", c)
		}
	}

	// Verify file was written
	data, err := os.ReadFile(filepath.Join(planDir, "SUMMARY.md"))
	if err != nil {
		t.Fatalf("SUMMARY.md not written: %v", err)
	}
	if string(data) != content {
		t.Error("file content doesn't match returned content")
	}
}

func TestGenerateSummary_WithBlocked(t *testing.T) {
	dir := t.TempDir()
	planDir := filepath.Join(dir, "test-plan")
	os.MkdirAll(filepath.Join(planDir, "phases", "phase-a"), 0755)
	os.MkdirAll(filepath.Join(planDir, "phases", "phase-b"), 0755)
	os.WriteFile(filepath.Join(planDir, "phases", "phase-a", "plan.md"), []byte("## Objective\n\nDo stuff"), 0644)

	meta := &arc.PlanMeta{
		Name:   "test-plan",
		Phases: []string{"phase-a", "phase-b"},
	}
	states := map[string]*arc.PhaseState{
		"phase-a": {PhaseStatus: "complete", Iteration: arc.Iteration{Current: 3}, Usage: arc.Usage{CostUSD: 0.10}},
		"phase-b": {PhaseStatus: "blocked", Blocked: arc.BlockedInfo{IsBlocked: true, Reason: strPtr("max rollbacks exhausted")}, Usage: arc.Usage{CostUSD: 0.20}},
	}

	content, err := GenerateSummary(SummaryOptions{
		PlanDir:     planDir,
		PlanName:    "test-plan",
		Meta:        meta,
		PhaseStates: states,
		ProjectDir:  dir,
	})
	if err != nil {
		t.Fatalf("GenerateSummary error: %v", err)
	}

	if !strings.Contains(content, "1/2 complete") {
		t.Error("missing '1/2 complete'")
	}
	if !strings.Contains(content, "**Blocked:** 1") {
		t.Error("missing '**Blocked:** 1'")
	}
	if !strings.Contains(content, "max rollbacks") {
		t.Error("missing blocked reason")
	}
}

func TestGenerateSummary_NoCommits(t *testing.T) {
	dir := t.TempDir()
	planDir := filepath.Join(dir, "test-plan")
	os.MkdirAll(filepath.Join(planDir, "phases", "phase-a"), 0755)
	os.WriteFile(filepath.Join(planDir, "phases", "phase-a", "plan.md"), []byte("## Objective\n\nTest"), 0644)

	meta := &arc.PlanMeta{
		Name:   "test-plan",
		Phases: []string{"phase-a"},
	}
	states := map[string]*arc.PhaseState{
		"phase-a": {PhaseStatus: "complete", Usage: arc.Usage{CostUSD: 0.10}},
	}

	content, err := GenerateSummary(SummaryOptions{
		PlanDir:     planDir,
		PlanName:    "test-plan",
		Meta:        meta,
		PhaseStates: states,
		ProjectDir:  dir,
	})
	if err != nil {
		t.Fatalf("GenerateSummary error: %v", err)
	}

	if !strings.Contains(content, "No commits recorded") {
		t.Error("missing 'No commits recorded'")
	}
}

func TestGenerateSummary_NilPhaseStates(t *testing.T) {
	_, err := GenerateSummary(SummaryOptions{
		PlanDir:  t.TempDir(),
		PlanName: "test",
		Meta:     &arc.PlanMeta{},
	})
	if err == nil {
		t.Fatal("expected error for nil phase states")
	}
}

func TestGenerateSummary_NilMeta(t *testing.T) {
	_, err := GenerateSummary(SummaryOptions{
		PlanDir:     t.TempDir(),
		PlanName:    "test",
		PhaseStates: map[string]*arc.PhaseState{},
	})
	if err == nil {
		t.Fatal("expected error for nil meta")
	}
}

func TestGenerateSummary_MissingPlanFile(t *testing.T) {
	dir := t.TempDir()
	planDir := filepath.Join(dir, "test-plan")
	os.MkdirAll(planDir, 0755)

	meta := &arc.PlanMeta{
		Name:   "test-plan",
		Phases: []string{"phase-a"},
	}
	states := map[string]*arc.PhaseState{
		"phase-a": {PhaseStatus: "complete", Usage: arc.Usage{CostUSD: 0.10}},
	}

	content, err := GenerateSummary(SummaryOptions{
		PlanDir:     planDir,
		PlanName:    "test-plan",
		Meta:        meta,
		PhaseStates: states,
		ProjectDir:  dir,
	})
	if err != nil {
		t.Fatalf("GenerateSummary error: %v", err)
	}

	if !strings.Contains(content, "(No objective found)") {
		t.Error("expected '(No objective found)' for missing plan.md")
	}
}

func TestGenerateSummary_TimestampFormat(t *testing.T) {
	dir := t.TempDir()
	planDir := filepath.Join(dir, "test-plan")
	os.MkdirAll(filepath.Join(planDir, "phases", "p"), 0755)
	os.WriteFile(filepath.Join(planDir, "phases", "p", "plan.md"), []byte("## Objective\n\nX"), 0644)

	content, err := GenerateSummary(SummaryOptions{
		PlanDir:     planDir,
		PlanName:    "test-plan",
		Meta:        &arc.PlanMeta{Name: "test-plan", Phases: []string{"p"}},
		PhaseStates: map[string]*arc.PhaseState{"p": {PhaseStatus: "complete"}},
		ProjectDir:  dir,
	})
	if err != nil {
		t.Fatalf("GenerateSummary error: %v", err)
	}

	// Extract timestamp from "Generated: <timestamp>"
	idx := strings.Index(content, "Generated: ")
	if idx == -1 {
		t.Fatal("missing 'Generated:' line")
	}
	after := content[idx+len("Generated: "):]
	eol := strings.Index(after, "\n")
	if eol == -1 {
		t.Fatal("no newline after timestamp")
	}
	ts := after[:eol]
	if _, err := time.Parse(time.RFC3339, ts); err != nil {
		t.Errorf("timestamp %q is not valid RFC3339: %v", ts, err)
	}
}

// --- CollectChangedFiles tests ---

func TestCollectChangedFiles_EmptyCommits(t *testing.T) {
	files, err := CollectChangedFiles(t.TempDir(), []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected empty, got %v", files)
	}
}

func TestCollectChangedFiles_InvalidCommit(t *testing.T) {
	// Create a git repo so git doesn't fail on "not a repo"
	dir := t.TempDir()
	exec.Command("git", "init", dir).Run()
	exec.Command("git", "-C", dir, "config", "user.email", "test@test.com").Run()
	exec.Command("git", "-C", dir, "config", "user.name", "test").Run()

	files, err := CollectChangedFiles(dir, []string{"nonexistent"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected empty for invalid commit, got %v", files)
	}
}

func TestCollectChangedFiles_ValidCommits(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		cmd.Run()
	}

	run("git", "init")
	run("git", "config", "user.email", "test@test.com")
	run("git", "config", "user.name", "test")

	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a"), 0644)
	run("git", "add", "a.go")
	run("git", "commit", "-m", "add a")

	// Get commit hash
	out, _ := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	hash1 := strings.TrimSpace(string(out))

	os.WriteFile(filepath.Join(dir, "b.go"), []byte("package b"), 0644)
	run("git", "add", "b.go")
	run("git", "commit", "-m", "add b")
	out, _ = exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	hash2 := strings.TrimSpace(string(out))

	files, err := CollectChangedFiles(dir, []string{hash1, hash2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %v", files)
	}
}

func TestCollectChangedFiles_Deduplication(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		cmd.Run()
	}

	run("git", "init")
	run("git", "config", "user.email", "test@test.com")
	run("git", "config", "user.name", "test")

	os.WriteFile(filepath.Join(dir, "shared.go"), []byte("v1"), 0644)
	run("git", "add", "shared.go")
	run("git", "commit", "-m", "v1")
	out, _ := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	hash1 := strings.TrimSpace(string(out))

	os.WriteFile(filepath.Join(dir, "shared.go"), []byte("v2"), 0644)
	run("git", "add", "shared.go")
	run("git", "commit", "-m", "v2")
	out, _ = exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	hash2 := strings.TrimSpace(string(out))

	files, err := CollectChangedFiles(dir, []string{hash1, hash2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(files) != 1 || files[0] != "shared.go" {
		t.Errorf("expected [shared.go], got %v", files)
	}
}

func TestCollectChangedFiles_NonGitDir(t *testing.T) {
	dir := t.TempDir() // not a git repo
	files, err := CollectChangedFiles(dir, []string{"abc123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected empty for non-git dir, got %v", files)
	}
}

func strPtr(s string) *string { return &s }
