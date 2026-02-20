package review

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
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

	result, err := RunAdversary(context.Background(), adv, planDir, phaseName, "# Test Phase")
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

	// Use a very short timeout context
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	result, err := RunAdversary(ctx, adv, planDir, phaseName, "# Test Phase")
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

	result, err := RunAdversary(context.Background(), adv, planDir, phaseName, "# Test Phase")
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
	result, err := RunAdversary(context.Background(), adv, planDir, phaseName, "# Test Phase")
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

	result, err := RunAdversary(context.Background(), adv, planDir, phaseName, "# Test Phase")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "error" {
		t.Fatalf("expected status 'error', got %q", result.Status)
	}
}
