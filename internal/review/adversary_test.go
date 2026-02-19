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
		Required:    true,
	}

	result, err := RunAdversary(context.Background(), adv, planDir, phaseName)
	// When implemented with mock agent producing "## Verdict\ncoverage_sufficient":
	// result.Status == "passed", result.Verdict == "coverage_sufficient"
	_ = result
	_ = err
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
		Required:    true,
	}

	// Use a very short timeout context
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	result, err := RunAdversary(ctx, adv, planDir, phaseName)
	// When implemented: result.Status == "error" — does not fail entire review
	_ = result
	_ = err
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
		Required:    true,
	}

	result, err := RunAdversary(context.Background(), adv, planDir, phaseName)
	// When implemented: result.Status == "cached" — agent not spawned
	_ = result
	_ = err
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
		Required:    true,
	}

	// When implemented with mock agent that outputs "## Verdict\ngibberish_not_valid":
	// result.Status == "failed", result.Verdict == "unknown"
	result, err := RunAdversary(context.Background(), adv, planDir, phaseName)
	_ = result
	_ = err
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
		Required:    true,
	}

	result, err := RunAdversary(context.Background(), adv, planDir, phaseName)
	// When implemented: result.Status == "error" — hash computation fails gracefully
	_ = result
	_ = err
}
