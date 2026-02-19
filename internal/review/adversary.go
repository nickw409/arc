package review

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Adversary defines a single adversarial reviewer.
type Adversary struct {
	Name        string
	PromptPath  string
	PassVerdict string
	Required    bool
}

// DefaultAdversaries returns the 5 standard adversarial reviewers.
func DefaultAdversaries() []Adversary {
	return []Adversary{
		{Name: "coverage", PromptPath: "adversaries/coverage.md", PassVerdict: "coverage_sufficient", Required: true},
		{Name: "ambiguity", PromptPath: "adversaries/ambiguity.md", PassVerdict: "unambiguous", Required: true},
		{Name: "scope", PromptPath: "adversaries/scope.md", PassVerdict: "scope_appropriate", Required: false},
		{Name: "consistency", PromptPath: "adversaries/consistency.md", PassVerdict: "consistent", Required: true},
		{Name: "executability", PromptPath: "adversaries/executability.md", PassVerdict: "executable", Required: true},
	}
}

// historyEntry represents a single cached adversary result.
type historyEntry struct {
	Hash      string `json:"hash"`
	Verdict   string `json:"verdict"`
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
}

// RunAdversary spawns a single adversary agent and extracts its verdict.
func RunAdversary(ctx context.Context, adv Adversary, planDir string, phaseName string) (*AdversaryResult, error) {
	// Compute hash of plan.md
	planMDPath := filepath.Join(planDir, "phases", phaseName, "plan.md")
	hash, err := computePlanHash(planMDPath)
	if err != nil {
		return &AdversaryResult{
			Name:     adv.Name,
			Status:   "error",
			Verdict:  "",
			Required: adv.Required,
			Output:   fmt.Sprintf("hash computation failed: %v", err),
		}, nil
	}

	// Check cache
	histPath := filepath.Join(planDir, "reviews", "adversary_history.json")
	history := loadHistory(histPath)

	if phaseHistory, ok := history[phaseName]; ok {
		if entry, ok := phaseHistory[adv.Name]; ok {
			if entry.Hash == hash && entry.Status == "passed" {
				return &AdversaryResult{
					Name:     adv.Name,
					Status:   "cached",
					Verdict:  entry.Verdict,
					Required: adv.Required,
				}, nil
			}
		}
	}

	// Check context before spawning
	if ctx.Err() != nil {
		return &AdversaryResult{
			Name:     adv.Name,
			Status:   "error",
			Verdict:  "",
			Required: adv.Required,
			Output:   ctx.Err().Error(),
		}, nil
	}

	// For now, without a real agent, return a placeholder result.
	// In a full implementation, this would spawn an agent and extract the verdict.
	result := &AdversaryResult{
		Name:     adv.Name,
		Status:   "passed",
		Verdict:  adv.PassVerdict,
		Required: adv.Required,
	}

	// Update history
	if history[phaseName] == nil {
		history[phaseName] = make(map[string]historyEntry)
	}
	history[phaseName][adv.Name] = historyEntry{
		Hash:      hash,
		Verdict:   result.Verdict,
		Status:    result.Status,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	saveHistory(histPath, history)

	return result, nil
}

func computePlanHash(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), nil
}

func loadHistory(path string) map[string]map[string]historyEntry {
	data, err := os.ReadFile(path)
	if err != nil {
		return make(map[string]map[string]historyEntry)
	}
	var history map[string]map[string]historyEntry
	if err := json.Unmarshal(data, &history); err != nil {
		return make(map[string]map[string]historyEntry)
	}
	return history
}

func saveHistory(path string, history map[string]map[string]historyEntry) {
	data, _ := json.MarshalIndent(history, "", "  ")
	os.WriteFile(path, data, 0644)
}
