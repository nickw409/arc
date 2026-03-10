package review

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/nwiley/arc/internal/resources"
)

func TestSetAgentCommandNameForTest(t *testing.T) {
	original := agentCommandName
	defer func() { agentCommandName = original }()

	SetAgentCommandNameForTest("mock-agent")
	if agentCommandName != "mock-agent" {
		t.Fatalf("expected 'mock-agent', got %q", agentCommandName)
	}
}

func TestRunAdversaryBasic(t *testing.T) {
	planDir := t.TempDir()
	phaseName := "test-phase"

	// Create phase plan.md for hash computation
	phaseDir := filepath.Join(planDir, "phases", phaseName)
	if err := os.MkdirAll(phaseDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(phaseDir, "plan.md"), []byte("# Test Phase"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create reviews directory
	if err := os.MkdirAll(filepath.Join(planDir, "reviews"), 0755); err != nil {
		t.Fatal(err)
	}

	adv := Adversary{
		Name:        "coverage",
		PromptPath:  "adversaries/coverage.md",
		PassVerdict: "coverage_sufficient",
		FailVerdict: "coverage_gaps",
		Required:    true,
	}

	result, err := RunAdversary(context.Background(), adv, planDir, phaseName, "# Test Phase", "", 1, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Without a real agent binary, the agent spawn will fail, resulting in an error status
	if result.Name != "coverage" {
		t.Fatalf("expected name 'coverage', got %q", result.Name)
	}
	if result.Required != true {
		t.Fatal("expected Required=true")
	}
}

func TestRunAdversaryTimeout(t *testing.T) {
	planDir := t.TempDir()
	phaseName := "test-phase"

	phaseDir := filepath.Join(planDir, "phases", phaseName)
	if err := os.MkdirAll(phaseDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(phaseDir, "plan.md"), []byte("# Test Phase"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(planDir, "reviews"), 0755); err != nil {
		t.Fatal(err)
	}

	adv := Adversary{
		Name:        "coverage",
		PromptPath:  "adversaries/coverage.md",
		PassVerdict: "coverage_sufficient",
		FailVerdict: "coverage_gaps",
		Required:    true,
	}

	// Use a pre-cancelled context for deterministic timeout behavior
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := RunAdversary(ctx, adv, planDir, phaseName, "# Test Phase", "", 1, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "error" {
		t.Fatalf("expected status 'error', got %q", result.Status)
	}
}

func TestRunAdversaryCached(t *testing.T) {
	planDir := t.TempDir()
	phaseName := "test-phase"

	// Create phase plan.md
	phaseDir := filepath.Join(planDir, "phases", phaseName)
	if err := os.MkdirAll(phaseDir, 0755); err != nil {
		t.Fatal(err)
	}
	planContent := []byte("# Test Phase")
	if err := os.WriteFile(filepath.Join(phaseDir, "plan.md"), planContent, 0644); err != nil {
		t.Fatal(err)
	}

	// Compute hash of plan.md
	hashBytes := sha256.Sum256(planContent)
	hash := hex.EncodeToString(hashBytes[:])

	// Create adversary_history.json with matching hash and "passed" status
	reviewsDir := filepath.Join(planDir, "reviews")
	if err := os.MkdirAll(reviewsDir, 0755); err != nil {
		t.Fatal(err)
	}

	type HistoryEntry struct {
		Hash      string `json:"hash"`
		Verdict   string `json:"verdict"`
		Status    string `json:"status"`
		Timestamp string `json:"timestamp"`
	}

	history := map[string]map[string]HistoryEntry{
		phaseName: {
			"coverage": {
				Hash:      hash,
				Verdict:   "coverage_sufficient",
				Status:    "passed",
				Timestamp: time.Now().Format(time.RFC3339),
			},
		},
	}
	histData, _ := json.MarshalIndent(history, "", "  ")
	if err := os.WriteFile(filepath.Join(reviewsDir, "adversary_history.json"), histData, 0644); err != nil {
		t.Fatal(err)
	}

	adv := Adversary{
		Name:        "coverage",
		PromptPath:  "adversaries/coverage.md",
		PassVerdict: "coverage_sufficient",
		FailVerdict: "coverage_gaps",
		Required:    true,
	}

	result, err := RunAdversary(context.Background(), adv, planDir, phaseName, "# Test Phase", "", 1, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "cached" {
		t.Fatalf("expected status 'cached', got %q", result.Status)
	}
	if result.Verdict != "coverage_sufficient" {
		t.Fatalf("expected verdict 'coverage_sufficient', got %q", result.Verdict)
	}
}

func TestRunAdversaryCachedFailure(t *testing.T) {
	planDir := t.TempDir()
	phaseName := "test-phase"

	// Create phase plan.md
	phaseDir := filepath.Join(planDir, "phases", phaseName)
	if err := os.MkdirAll(phaseDir, 0755); err != nil {
		t.Fatal(err)
	}
	planContent := []byte("# Test Phase")
	if err := os.WriteFile(filepath.Join(phaseDir, "plan.md"), planContent, 0644); err != nil {
		t.Fatal(err)
	}

	// Compute hash of plan.md
	hashBytes := sha256.Sum256(planContent)
	hash := hex.EncodeToString(hashBytes[:])

	// Create adversary_history.json with a failed entry and matching hash
	reviewsDir := filepath.Join(planDir, "reviews")
	if err := os.MkdirAll(reviewsDir, 0755); err != nil {
		t.Fatal(err)
	}

	type HistoryEntry struct {
		Hash      string `json:"hash"`
		Verdict   string `json:"verdict"`
		Status    string `json:"status"`
		Timestamp string `json:"timestamp"`
	}

	history := map[string]map[string]HistoryEntry{
		phaseName: {
			"coverage": {
				Hash:      hash,
				Verdict:   "coverage_gaps",
				Status:    "failed",
				Timestamp: time.Now().Format(time.RFC3339),
			},
		},
	}
	histData, _ := json.MarshalIndent(history, "", "  ")
	if err := os.WriteFile(filepath.Join(reviewsDir, "adversary_history.json"), histData, 0644); err != nil {
		t.Fatal(err)
	}

	adv := Adversary{
		Name:        "coverage",
		PromptPath:  "adversaries/coverage.md",
		PassVerdict: "coverage_sufficient",
		FailVerdict: "coverage_gaps",
		Required:    true,
	}

	result, err := RunAdversary(context.Background(), adv, planDir, phaseName, "# Test Phase", "", 1, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "cached" {
		t.Fatalf("expected status 'cached', got %q", result.Status)
	}
	if result.Verdict != "coverage_gaps" {
		t.Fatalf("expected verdict 'coverage_gaps', got %q", result.Verdict)
	}
}

func TestRunAdversaryInvalidVerdict(t *testing.T) {
	planDir := t.TempDir()
	phaseName := "test-phase"

	phaseDir := filepath.Join(planDir, "phases", phaseName)
	if err := os.MkdirAll(phaseDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(phaseDir, "plan.md"), []byte("# Test Phase"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(planDir, "reviews"), 0755); err != nil {
		t.Fatal(err)
	}

	adv := Adversary{
		Name:        "coverage",
		PromptPath:  "adversaries/coverage.md",
		PassVerdict: "coverage_sufficient",
		FailVerdict: "coverage_gaps",
		Required:    true,
	}

	// Without a real agent binary, the spawn will fail resulting in error status
	result, err := RunAdversary(context.Background(), adv, planDir, phaseName, "# Test Phase", "", 1, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "coverage" {
		t.Fatalf("expected name 'coverage', got %q", result.Name)
	}
}

func TestRunAdversaryHashFailure(t *testing.T) {
	planDir := t.TempDir()
	phaseName := "test-phase"

	// Do NOT create plan.md — hash computation should fail gracefully
	phaseDir := filepath.Join(planDir, "phases", phaseName)
	if err := os.MkdirAll(phaseDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(planDir, "reviews"), 0755); err != nil {
		t.Fatal(err)
	}

	adv := Adversary{
		Name:        "coverage",
		PromptPath:  "adversaries/coverage.md",
		PassVerdict: "coverage_sufficient",
		FailVerdict: "coverage_gaps",
		Required:    true,
	}

	result, err := RunAdversary(context.Background(), adv, planDir, phaseName, "# Test Phase", "", 1, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "error" {
		t.Fatalf("expected status 'error', got %q", result.Status)
	}
}

func TestDefaultAdversaries_HasIntegration(t *testing.T) {
	advs := DefaultAdversaries()
	for _, a := range advs {
		if a.Name == "integration" {
			if a.PassVerdict != "integration_complete" {
				t.Errorf("integration PassVerdict = %q, want %q", a.PassVerdict, "integration_complete")
			}
			if a.FailVerdict != "integration_gaps" {
				t.Errorf("integration FailVerdict = %q, want %q", a.FailVerdict, "integration_gaps")
			}
			if !a.Required {
				t.Error("integration adversary must be Required=true")
			}
			return
		}
	}
	t.Error("no adversary named 'integration' found in DefaultAdversaries()")
}

func TestDefaultAdversaries_SixEntries(t *testing.T) {
	advs := DefaultAdversaries()
	if len(advs) != 6 {
		t.Errorf("len(DefaultAdversaries()) = %d, want 6", len(advs))
	}
}

func TestIntegrationAdversary_PromptExists(t *testing.T) {
	data, err := resources.PromptBytes("adversaries/integration.md")
	if err != nil {
		t.Fatalf("integration.md prompt not found: %v", err)
	}
	if len(data) == 0 {
		t.Error("integration.md prompt is empty")
	}
}

// runAdversary runs a named adversary against planContent and returns the verdict.
// Skips the test unless ARC_ADVERSARY_INTEGRATION_TEST=1 is set and claude is in PATH.
func runAdversary(t *testing.T, name, planContent string) string {
	t.Helper()

	if os.Getenv("ARC_ADVERSARY_INTEGRATION_TEST") != "1" {
		t.Skipf("skipping: set ARC_ADVERSARY_INTEGRATION_TEST=1 to run adversary integration tests")
	}
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skipf("skipping: claude binary not found in PATH (%v)", err)
	}

	var adv Adversary
	found := false
	for _, a := range DefaultAdversaries() {
		if a.Name == name {
			adv = a
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no adversary named %q found in DefaultAdversaries()", name)
	}

	planDir := t.TempDir()
	phaseName := "test-phase"
	phaseDir := filepath.Join(planDir, "phases", phaseName)
	if err := os.MkdirAll(phaseDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(phaseDir, "plan.md"), []byte(planContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(planDir, "reviews"), 0755); err != nil {
		t.Fatal(err)
	}

	result, err := RunAdversary(context.Background(), adv, planDir, phaseName, planContent, "", 1, "")
	if err != nil {
		t.Fatalf("RunAdversary error: %v", err)
	}
	return result.Verdict
}

func TestIntegrationAdversary_SilentPassNoIntegrations(t *testing.T) {
	planContent := `# Phase: new-feature
## Objective
Add a new package.
## Files
### Create
- internal/newpkg/newpkg.go
## Gate
assertions:
  - type: file_exists
    path: internal/newpkg/newpkg.go
`
	verdict := runAdversary(t, "integration", planContent)
	if verdict != "integration_complete" {
		t.Errorf("plan with no integrations got verdict %q, want integration_complete", verdict)
	}
}

func TestIntegrationAdversary_PassWithCoverage(t *testing.T) {
	planContent := `# Phase: wire-daemon
## Objective
Wire daemon into CLI.
## Files
### Modify
- internal/cli/run.go — add daemon.Connect() call
## Gate
assertions:
  - type: grep
    file: internal/cli/run.go
    pattern: "daemon\\.Connect"
`
	verdict := runAdversary(t, "integration", planContent)
	if verdict != "integration_complete" {
		t.Errorf("plan with covered integration got verdict %q, want integration_complete", verdict)
	}
}

func TestIntegrationAdversary_FailMissingGrepAssertion(t *testing.T) {
	planContent := `# Phase: wire-daemon
## Objective
Wire daemon into CLI.
## Files
### Modify
- internal/cli/run.go — add daemon.Connect() call
## Gate
assertions:
  - type: file_exists
    path: internal/daemon/daemon.go
`
	verdict := runAdversary(t, "integration", planContent)
	if verdict != "integration_gaps" {
		t.Errorf("plan with uncovered integration got verdict %q, want integration_gaps", verdict)
	}
}

func TestIntegrationAdversary_PassNoGateSection(t *testing.T) {
	planContent := `# Phase: new-lib
## Objective
Add utility library.
## Files
### Create
- internal/util/util.go
`
	verdict := runAdversary(t, "integration", planContent)
	if verdict != "integration_complete" {
		t.Errorf("plan with no gate section got verdict %q, want integration_complete (silent pass)", verdict)
	}
}
