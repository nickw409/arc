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

	"github.com/nwiley/arc/internal/agent"
	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/prompt"
	"github.com/nwiley/arc/internal/resources"
)

// agentCommandName is the binary name used for agent spawning.
// Tests override this to point to a mock binary.
var agentCommandName = "claude"

// SetAgentCommandNameForTest overrides the agent binary name for testing.
func SetAgentCommandNameForTest(name string) {
	agentCommandName = name
}

// Adversary defines a single adversarial reviewer.
type Adversary struct {
	Name        string
	PromptPath  string
	PassVerdict string
	FailVerdict string
	Required    bool
}

// DefaultAdversaries returns the standard adversarial reviewers.
// scope runs first as a pre-check; the other three run in parallel after scope passes.
func DefaultAdversaries() []Adversary {
	return []Adversary{
		{Name: "scope", PromptPath: "adversaries/scope.md", PassVerdict: "scope_appropriate", FailVerdict: "scope_too_large", Required: true},
		{Name: "spec-quality", PromptPath: "adversaries/spec-quality.md", PassVerdict: "spec_quality_sufficient", FailVerdict: "spec_quality_gaps", Required: true},
		{Name: "correctness", PromptPath: "adversaries/correctness.md", PassVerdict: "correctness_sufficient", FailVerdict: "correctness_gaps", Required: true},
		{Name: "gate", PromptPath: "adversaries/gate.md", PassVerdict: "gate_sufficient", FailVerdict: "gate_gaps", Required: true},
	}
}

// ScopeAdversary returns the scope pre-check adversary.
func ScopeAdversary() Adversary {
	return DefaultAdversaries()[0]
}

// RunAdversary spawns a single adversary agent and extracts its verdict.
func RunAdversary(ctx context.Context, adv Adversary, planDir string, phaseName string, planMD string, model string, projectContext string) (*AdversaryResult, error) {
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
	history := LoadHistory(histPath)

	if phaseHistory, ok := history.Phases[phaseName]; ok {
		if entry, ok := phaseHistory[adv.Name]; ok {
			if entry.Hash == hash {
				return &AdversaryResult{
					Name:         adv.Name,
					Status:       "cached",
					CachedStatus: entry.Status,
					Verdict:      entry.Verdict,
					Required:     adv.Required,
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

	// Load adversary prompt template
	promptBytes, err := resources.PromptBytes(adv.PromptPath)
	if err != nil {
		return &AdversaryResult{
			Name:     adv.Name,
			Status:   "error",
			Verdict:  "",
			Required: adv.Required,
			Output:   fmt.Sprintf("failed to load prompt %s: %v", adv.PromptPath, err),
		}, nil
	}

	fullPrompt := string(promptBytes)
	if projectContext != "" {
		fullPrompt += "\n\n## Project Context\n\n" + projectContext
	}
	fullPrompt += "\n\n## Plan Under Review\n\n" + planMD

	spawnResult, err := agent.Spawn(ctx, agent.SpawnOptions{
		Prompt:       fullPrompt,
		AllowedTools: []string{"Read"},
		CommandName:  agentCommandName,
		Timeout:      120 * time.Second,
		Model:        model,
	})
	if err != nil {
		return &AdversaryResult{
			Name:     adv.Name,
			Status:   "error",
			Verdict:  "",
			Required: adv.Required,
			Output:   fmt.Sprintf("agent spawn failed: %v", err),
		}, nil
	}

	if ctx.Err() != nil {
		return &AdversaryResult{
			Name:     adv.Name,
			Status:   "error",
			Verdict:  "",
			Required: adv.Required,
			Output:   ctx.Err().Error(),
		}, nil
	}

	validVerdicts := []arc.Verdict{arc.Verdict(adv.PassVerdict), arc.Verdict(adv.FailVerdict)}
	verdict, extractErr := prompt.ExtractVerdict(spawnResult.Output, validVerdicts)

	var status string
	if extractErr == nil && arc.Verdict(adv.PassVerdict) == verdict {
		status = "passed"
	} else {
		status = "failed"
	}

	return &AdversaryResult{
		Name:     adv.Name,
		Status:   status,
		Verdict:  string(verdict),
		Required: adv.Required,
		Output:   spawnResult.Output,
		Usage:    spawnResult.Usage,
	}, nil
}

func computePlanHash(path string) (string, error) {
	return ComputePlanHash(path)
}

// ComputePlanHash computes the SHA-256 hex digest of a file.
// Exported for use by callers that need to check plan.md hashes without running a review.
func ComputePlanHash(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), nil
}

// LoadHistory reads the adversary history file.
// It supports both the new wrapped format and the old flat map format for backward compat.
func LoadHistory(path string) *historyFile {
	data, err := os.ReadFile(path)
	if err != nil {
		return newHistoryFile()
	}

	// Try new format first
	var hf historyFile
	if err := json.Unmarshal(data, &hf); err == nil && hf.Phases != nil {
		if hf.Iterations == nil {
			hf.Iterations = make(map[string]int)
		}
		return &hf
	}

	// Fall back to old flat format: map[phase]map[adv]entry
	var flat map[string]map[string]historyEntry
	if err := json.Unmarshal(data, &flat); err != nil {
		return newHistoryFile()
	}
	return &historyFile{
		Phases:     flat,
		Iterations: make(map[string]int),
	}
}

// SaveHistory writes the adversary history file.
func SaveHistory(path string, history *historyFile) error {
	data, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling adversary history: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing adversary history %s: %w", path, err)
	}
	return nil
}

func newHistoryFile() *historyFile {
	return &historyFile{
		Phases:     make(map[string]map[string]historyEntry),
		Iterations: make(map[string]int),
	}
}

// historyEntry represents a single cached adversary result.
type historyEntry struct {
	Hash      string `json:"hash"`
	Verdict   string `json:"verdict"`
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
}

// historyFile is the on-disk format for adversary_history.json.
type historyFile struct {
	Phases     map[string]map[string]historyEntry `json:"phases"`
	Iterations map[string]int                     `json:"iterations"`
}
