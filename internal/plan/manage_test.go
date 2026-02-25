package plan

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/state"
)

func setupManageTest(t *testing.T) (string, ManageOptions) {
	t.Helper()

	plansDir := t.TempDir()
	planDir := filepath.Join(plansDir, "test-plan")
	phaseDir := filepath.Join(planDir, "phases", "core")
	if err := os.MkdirAll(phaseDir, 0755); err != nil {
		t.Fatal(err)
	}

	ps := arc.NewPhaseState("test-plan", "core", "feature")
	data, _ := json.MarshalIndent(ps, "", "  ")
	if err := os.WriteFile(filepath.Join(phaseDir, "state.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	// Write plan.json for Defer validation
	meta := arc.NewPlanMeta("test-plan", "feature", []string{"core"})
	metaData, _ := json.MarshalIndent(meta, "", "  ")
	if err := os.WriteFile(filepath.Join(planDir, "plan.json"), metaData, 0644); err != nil {
		t.Fatal(err)
	}

	opts := ManageOptions{
		PlansDir: plansDir,
		PlanName: "test-plan",
		Phase:    "core",
	}
	return plansDir, opts
}

func readTestState(t *testing.T, opts ManageOptions) *arc.PhaseState {
	t.Helper()
	sf := state.NewStateFile(filepath.Join(opts.PlansDir, opts.PlanName, "phases", opts.Phase, "state.json"))
	s, err := sf.Read()
	if err != nil {
		t.Fatalf("reading state: %v", err)
	}
	return s
}

func TestManageComplete(t *testing.T) {
	_, opts := setupManageTest(t)

	if err := ManageComplete(opts); err != nil {
		t.Fatalf("ManageComplete error: %v", err)
	}

	s := readTestState(t, opts)
	if s.PhaseStatus != "complete" {
		t.Fatalf("phase_status = %q, want %q", s.PhaseStatus, "complete")
	}
	if s.CompletedAt == "" {
		t.Fatal("completed_at should not be empty")
	}
}

func TestManagePendingClearsBlocked(t *testing.T) {
	_, opts := setupManageTest(t)

	// First block it, then reset to pending
	opts.Reason = "something"
	if err := ManageBlock(opts); err != nil {
		t.Fatal(err)
	}

	if err := ManagePending(opts); err != nil {
		t.Fatalf("ManagePending error: %v", err)
	}

	s := readTestState(t, opts)
	if s.PhaseStatus != "pending" {
		t.Fatalf("phase_status = %q, want %q", s.PhaseStatus, "pending")
	}
	if s.BlockedReason != "" {
		t.Fatalf("blocked_reason should be cleared, got %q", s.BlockedReason)
	}
	if s.BlockedAt != "" {
		t.Fatalf("blocked_at should be cleared, got %q", s.BlockedAt)
	}
	if s.Blocked.IsBlocked {
		t.Fatal("blocked.is_blocked should be false")
	}
}

func TestManagePendingClearsDeferred(t *testing.T) {
	_, opts := setupManageTest(t)

	// First defer it, then reset to pending
	opts.Reason = "not ready"
	if err := ManageDefer(opts); err != nil {
		t.Fatal(err)
	}

	// Verify it was deferred
	s := readTestState(t, opts)
	if s.PhaseStatus != "deferred" {
		t.Fatalf("setup: phase_status = %q, want %q", s.PhaseStatus, "deferred")
	}

	if err := ManagePending(opts); err != nil {
		t.Fatalf("ManagePending error: %v", err)
	}

	s = readTestState(t, opts)
	if s.PhaseStatus != "pending" {
		t.Fatalf("phase_status = %q, want %q", s.PhaseStatus, "pending")
	}
	if s.DeferredReason != "" {
		t.Fatalf("deferred_reason should be cleared, got %q", s.DeferredReason)
	}
	if s.DeferredAt != "" {
		t.Fatalf("deferred_at should be cleared, got %q", s.DeferredAt)
	}
}

func TestManageDefer(t *testing.T) {
	_, opts := setupManageTest(t)
	opts.Reason = "waiting on API"

	if err := ManageDefer(opts); err != nil {
		t.Fatalf("ManageDefer error: %v", err)
	}

	s := readTestState(t, opts)
	if s.PhaseStatus != "deferred" {
		t.Fatalf("phase_status = %q, want %q", s.PhaseStatus, "deferred")
	}
	if s.DeferredReason != "waiting on API" {
		t.Fatalf("deferred_reason = %q, want %q", s.DeferredReason, "waiting on API")
	}
	if s.DeferredAt == "" {
		t.Fatal("deferred_at should not be empty")
	}
}

func TestManageBlock(t *testing.T) {
	_, opts := setupManageTest(t)
	opts.Reason = "dependency missing"

	if err := ManageBlock(opts); err != nil {
		t.Fatalf("ManageBlock error: %v", err)
	}

	s := readTestState(t, opts)
	if s.PhaseStatus != "blocked" {
		t.Fatalf("phase_status = %q, want %q", s.PhaseStatus, "blocked")
	}
	if s.BlockedReason != "dependency missing" {
		t.Fatalf("blocked_reason = %q, want %q", s.BlockedReason, "dependency missing")
	}
	if s.BlockedAt == "" {
		t.Fatal("blocked_at should not be empty")
	}
	if !s.Blocked.IsBlocked {
		t.Fatal("blocked.is_blocked should be true")
	}
	if s.Blocked.Reason == nil || *s.Blocked.Reason != "dependency missing" {
		t.Fatalf("blocked.reason = %v, want %q", s.Blocked.Reason, "dependency missing")
	}
}

func TestManageTests(t *testing.T) {
	_, opts := setupManageTest(t)
	opts.Passing = 42
	opts.Total = 50

	if err := ManageTests(opts); err != nil {
		t.Fatalf("ManageTests error: %v", err)
	}

	s := readTestState(t, opts)
	if s.TestsPassing != 42 {
		t.Fatalf("tests_passing = %d, want %d", s.TestsPassing, 42)
	}
	if s.TestsTotal != 50 {
		t.Fatalf("tests_total = %d, want %d", s.TestsTotal, 50)
	}
}

func TestManageTestsNegativePassing(t *testing.T) {
	_, opts := setupManageTest(t)
	opts.Passing = -1
	opts.Total = 10
	if err := ManageTests(opts); err == nil {
		t.Fatal("expected error for negative passing count")
	}
}

func TestManageTestsNegativeTotal(t *testing.T) {
	_, opts := setupManageTest(t)
	opts.Passing = 0
	opts.Total = -1
	if err := ManageTests(opts); err == nil {
		t.Fatal("expected error for negative total count")
	}
}

func TestManageTestsPassingExceedsTotal(t *testing.T) {
	_, opts := setupManageTest(t)
	opts.Passing = 999
	opts.Total = 1
	if err := ManageTests(opts); err == nil {
		t.Fatal("expected error when passing exceeds total")
	}
}

func TestManageTestsAllPassing(t *testing.T) {
	_, opts := setupManageTest(t)
	opts.Passing = 5
	opts.Total = 5
	if err := ManageTests(opts); err != nil {
		t.Fatalf("passing == total should be valid, got error: %v", err)
	}
}

func TestManageTestsNoTests(t *testing.T) {
	_, opts := setupManageTest(t)
	opts.Passing = 0
	opts.Total = 0
	if err := ManageTests(opts); err != nil {
		t.Fatalf("0 passing and 0 total should be valid, got error: %v", err)
	}
}

func TestManagePackages(t *testing.T) {
	_, opts := setupManageTest(t)
	opts.Packages = []string{"internal/plan", "internal/pipeline"}

	if err := ManagePackages(opts); err != nil {
		t.Fatalf("ManagePackages error: %v", err)
	}

	s := readTestState(t, opts)
	if len(s.Packages) != 2 || s.Packages[0] != "internal/plan" {
		t.Fatalf("packages = %v, want [internal/plan internal/pipeline]", s.Packages)
	}
}

func TestManagePackagesNil(t *testing.T) {
	_, opts := setupManageTest(t)
	opts.Packages = nil

	if err := ManagePackages(opts); err != nil {
		t.Fatalf("ManagePackages error: %v", err)
	}

	s := readTestState(t, opts)
	if s.Packages == nil {
		t.Fatal("packages should not be nil (should be empty slice)")
	}
}

func TestManageNote(t *testing.T) {
	_, opts := setupManageTest(t)
	opts.Note = "needs attention from reviewer"

	if err := ManageNote(opts); err != nil {
		t.Fatalf("ManageNote error: %v", err)
	}

	s := readTestState(t, opts)
	if s.Notes != "needs attention from reviewer" {
		t.Fatalf("notes = %q, want %q", s.Notes, "needs attention from reviewer")
	}
}

func TestManageIteration(t *testing.T) {
	_, opts := setupManageTest(t)
	opts.Iteration = 5

	if err := ManageIteration(opts); err != nil {
		t.Fatalf("ManageIteration error: %v", err)
	}

	s := readTestState(t, opts)
	if s.Iteration.Current != 5 {
		t.Fatalf("iteration.current = %d, want %d", s.Iteration.Current, 5)
	}
}

func TestManageCopyFrom(t *testing.T) {
	plansDir, opts := setupManageTest(t)

	// Create a source phase with some data
	srcDir := filepath.Join(plansDir, "test-plan", "phases", "source")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}

	srcState := arc.NewPhaseState("test-plan", "source", "feature")
	srcState.TestsPassing = 99
	srcState.TestsTotal = 100
	srcState.PhaseStatus = "complete"
	srcData, _ := json.MarshalIndent(srcState, "", "  ")
	if err := os.WriteFile(filepath.Join(srcDir, "state.json"), srcData, 0644); err != nil {
		t.Fatal(err)
	}

	opts.SourcePhase = "source"
	if err := ManageCopyFrom(opts); err != nil {
		t.Fatalf("ManageCopyFrom error: %v", err)
	}

	s := readTestState(t, opts)
	// Should have copied test counts
	if s.TestsPassing != 99 || s.TestsTotal != 100 {
		t.Fatalf("tests = %d/%d, want 99/100", s.TestsPassing, s.TestsTotal)
	}
	// Should have copied phase status
	if s.PhaseStatus != "complete" {
		t.Fatalf("phase_status = %q, want %q (should copy from source)", s.PhaseStatus, "complete")
	}
	// Should preserve target identity
	if s.Phase != "core" {
		t.Fatalf("phase = %q, want %q (should preserve identity)", s.Phase, "core")
	}
	if s.Plan != "test-plan" {
		t.Fatalf("plan = %q, want %q (should preserve identity)", s.Plan, "test-plan")
	}
}

func TestManageShow(t *testing.T) {
	_, opts := setupManageTest(t)

	var buf bytes.Buffer
	if err := ManageShow(&buf, opts); err != nil {
		t.Fatalf("ManageShow error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, `"phase": "core"`) {
		t.Fatalf("output should contain phase name, got: %s", output)
	}
	if !strings.Contains(output, `"phase_status": "pending"`) {
		t.Fatalf("output should contain phase_status, got: %s", output)
	}

	// Verify it's valid JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &parsed); err != nil {
		t.Fatalf("output should be valid JSON: %v", err)
	}
}

func TestManageIterationZero(t *testing.T) {
	_, opts := setupManageTest(t)

	// Set iteration to 5 first
	opts.Iteration = 5
	if err := ManageIteration(opts); err != nil {
		t.Fatal(err)
	}

	// Reset to 0
	opts.Iteration = 0
	if err := ManageIteration(opts); err != nil {
		t.Fatalf("ManageIteration(0) error: %v", err)
	}

	s := readTestState(t, opts)
	if s.Iteration.Current != 0 {
		t.Fatalf("iteration.current = %d, want 0", s.Iteration.Current)
	}
}

func TestManageCompleteNonexistentPhase(t *testing.T) {
	plansDir := t.TempDir()
	opts := ManageOptions{
		PlansDir: plansDir,
		PlanName: "nonexistent-plan",
		Phase:    "nonexistent-phase",
	}

	err := ManageComplete(opts)
	if err == nil {
		t.Fatal("expected error for nonexistent phase path")
	}
}

func TestManageCopyFromNonexistentSource(t *testing.T) {
	_, opts := setupManageTest(t)
	opts.SourcePhase = "nonexistent-source"

	err := ManageCopyFrom(opts)
	if err == nil {
		t.Fatal("expected error for nonexistent source phase")
	}
	if !strings.Contains(err.Error(), "nonexistent-source") {
		t.Fatalf("error should mention source phase, got: %v", err)
	}
}

func TestManageShowNonexistentPhase(t *testing.T) {
	plansDir := t.TempDir()
	opts := ManageOptions{
		PlansDir: plansDir,
		PlanName: "nonexistent-plan",
		Phase:    "nonexistent-phase",
	}

	var buf bytes.Buffer
	err := ManageShow(&buf, opts)
	if err == nil {
		t.Fatal("expected error for nonexistent phase path")
	}
}
