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
		Name:        "spec-quality",
		PromptPath:  "adversaries/spec-quality.md",
		PassVerdict: "spec_quality_sufficient",
		FailVerdict: "spec_quality_gaps",
		Required:    true,
	}

	result, err := RunAdversary(context.Background(), adv, planDir, phaseName, "# Test Phase", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "spec-quality" {
		t.Fatalf("expected name 'spec-quality', got %q", result.Name)
	}
	if !result.Required {
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
		Name:        "spec-quality",
		PromptPath:  "adversaries/spec-quality.md",
		PassVerdict: "spec_quality_sufficient",
		FailVerdict: "spec_quality_gaps",
		Required:    true,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := RunAdversary(ctx, adv, planDir, phaseName, "# Test Phase", "", "")
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

	phaseDir := filepath.Join(planDir, "phases", phaseName)
	if err := os.MkdirAll(phaseDir, 0755); err != nil {
		t.Fatal(err)
	}
	planContent := []byte("# Test Phase")
	if err := os.WriteFile(filepath.Join(phaseDir, "plan.md"), planContent, 0644); err != nil {
		t.Fatal(err)
	}

	hashBytes := sha256.Sum256(planContent)
	hash := hex.EncodeToString(hashBytes[:])

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
			"spec-quality": {
				Hash:      hash,
				Verdict:   "spec_quality_sufficient",
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
		Name:        "spec-quality",
		PromptPath:  "adversaries/spec-quality.md",
		PassVerdict: "spec_quality_sufficient",
		FailVerdict: "spec_quality_gaps",
		Required:    true,
	}

	result, err := RunAdversary(context.Background(), adv, planDir, phaseName, "# Test Phase", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "cached" {
		t.Fatalf("expected status 'cached', got %q", result.Status)
	}
	if result.Verdict != "spec_quality_sufficient" {
		t.Fatalf("expected verdict 'spec_quality_sufficient', got %q", result.Verdict)
	}
}

func TestRunAdversaryCachedFailure(t *testing.T) {
	planDir := t.TempDir()
	phaseName := "test-phase"

	phaseDir := filepath.Join(planDir, "phases", phaseName)
	if err := os.MkdirAll(phaseDir, 0755); err != nil {
		t.Fatal(err)
	}
	planContent := []byte("# Test Phase")
	if err := os.WriteFile(filepath.Join(phaseDir, "plan.md"), planContent, 0644); err != nil {
		t.Fatal(err)
	}

	hashBytes := sha256.Sum256(planContent)
	hash := hex.EncodeToString(hashBytes[:])

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
			"spec-quality": {
				Hash:      hash,
				Verdict:   "spec_quality_gaps",
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
		Name:        "spec-quality",
		PromptPath:  "adversaries/spec-quality.md",
		PassVerdict: "spec_quality_sufficient",
		FailVerdict: "spec_quality_gaps",
		Required:    true,
	}

	result, err := RunAdversary(context.Background(), adv, planDir, phaseName, "# Test Phase", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "cached" {
		t.Fatalf("expected status 'cached', got %q", result.Status)
	}
	if result.Verdict != "spec_quality_gaps" {
		t.Fatalf("expected verdict 'spec_quality_gaps', got %q", result.Verdict)
	}
}

func TestRunAdversaryHashFailure(t *testing.T) {
	planDir := t.TempDir()
	phaseName := "test-phase"

	phaseDir := filepath.Join(planDir, "phases", phaseName)
	if err := os.MkdirAll(phaseDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(planDir, "reviews"), 0755); err != nil {
		t.Fatal(err)
	}

	adv := Adversary{
		Name:        "spec-quality",
		PromptPath:  "adversaries/spec-quality.md",
		PassVerdict: "spec_quality_sufficient",
		FailVerdict: "spec_quality_gaps",
		Required:    true,
	}

	result, err := RunAdversary(context.Background(), adv, planDir, phaseName, "# Test Phase", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "error" {
		t.Fatalf("expected status 'error', got %q", result.Status)
	}
}

func TestDefaultAdversaries_FourEntries(t *testing.T) {
	advs := DefaultAdversaries()
	if len(advs) != 4 {
		t.Errorf("len(DefaultAdversaries()) = %d, want 4", len(advs))
	}
}

func TestDefaultAdversaries_Names(t *testing.T) {
	advs := DefaultAdversaries()
	expected := []string{"scope", "spec-quality", "correctness", "gate"}
	for i, name := range expected {
		if advs[i].Name != name {
			t.Errorf("adversary[%d].Name = %q, want %q", i, advs[i].Name, name)
		}
	}
}

func TestDefaultAdversaries_AllRequired(t *testing.T) {
	for _, adv := range DefaultAdversaries() {
		if !adv.Required {
			t.Errorf("adversary %q: expected Required=true", adv.Name)
		}
	}
}

func TestScopeAdversary(t *testing.T) {
	adv := ScopeAdversary()
	if adv.Name != "scope" {
		t.Errorf("ScopeAdversary().Name = %q, want %q", adv.Name, "scope")
	}
	if adv.PassVerdict != "scope_appropriate" {
		t.Errorf("ScopeAdversary().PassVerdict = %q, want scope_appropriate", adv.PassVerdict)
	}
	if adv.FailVerdict != "scope_too_large" {
		t.Errorf("ScopeAdversary().FailVerdict = %q, want scope_too_large", adv.FailVerdict)
	}
}

func TestPromptsExist(t *testing.T) {
	paths := []string{
		"adversaries/scope.md",
		"adversaries/spec-quality.md",
		"adversaries/correctness.md",
		"adversaries/gate.md",
		"adversaries/synthesizer.md",
	}
	for _, path := range paths {
		data, err := resources.PromptBytes(path)
		if err != nil {
			t.Errorf("prompt %s not found: %v", path, err)
			continue
		}
		if len(data) == 0 {
			t.Errorf("prompt %s is empty", path)
		}
	}
}
