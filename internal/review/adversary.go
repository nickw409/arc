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

// DefaultAdversaries returns the 5 standard adversarial reviewers.
func DefaultAdversaries() []Adversary {
	return []Adversary{
		{Name: "coverage", PromptPath: "adversaries/coverage.md", PassVerdict: "coverage_sufficient", FailVerdict: "coverage_gaps", Required: true},
		{Name: "ambiguity", PromptPath: "adversaries/ambiguity.md", PassVerdict: "unambiguous", FailVerdict: "ambiguous", Required: true},
		{Name: "scope", PromptPath: "adversaries/scope.md", PassVerdict: "scope_appropriate", FailVerdict: "scope_too_large", Required: false},
		{Name: "consistency", PromptPath: "adversaries/consistency.md", PassVerdict: "consistent", FailVerdict: "inconsistent", Required: true},
		{Name: "executability", PromptPath: "adversaries/executability.md", PassVerdict: "executable", FailVerdict: "blocked", Required: true},
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

// RunAdversary spawns a single adversary agent and extracts its verdict.
func RunAdversary(ctx context.Context, adv Adversary, planDir string, phaseName string, planMD string, model string, iteration int) (*AdversaryResult, error) {
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

	// Append plan content as context, with a post-plan reminder to emit suggestions.
	// The reminder must come AFTER the plan so the LLM sees it last, reinforcing
	// the requirement to produce <<<ORIGINAL/<<<SUGGESTED blocks on failure.
	postPlanReminder := "\n\n---\n\n" +
		"IMPORTANT: You have now read the plan. Produce your analysis and verdict. " +
		"If your verdict indicates failure, you MUST then include a ## Suggestions section " +
		"containing <<<ORIGINAL and <<<SUGGESTED fix blocks as described in your instructions above. " +
		"Omitting suggestions on a failing verdict makes your response invalid.\n"

	// Progressive leniency: on later iterations the plan has already been
	// improved, so adversaries should focus only on genuine blocking issues.
	// Thresholds are deliberately high so early iterations work at full rigor.
	if iteration >= 3 {
		postPlanReminder += fmt.Sprintf("\n"+
			"NOTE: This is review iteration %d. "+
			"The plan has already been improved based on previous review feedback. "+
			"Raise your bar for failure — only flag issues that would genuinely block implementation. "+
			"Minor stylistic concerns, theoretical edge cases, and nice-to-haves should PASS. "+
			"You MUST still use the exact verdict format and suggestion blocks described above.\n", iteration)
	}
	if iteration >= 5 {
		postPlanReminder += "\n" +
			"FINAL REVIEW: Only flag issues that will definitely cause incorrect behavior. " +
			"If the plan is good enough to implement successfully, approve it. " +
			"You MUST still output a valid verdict line.\n"
	}

	fullPrompt := string(promptBytes) + "\n\n## Plan Under Review\n\n" + planMD + postPlanReminder

	// Spawn agent with 120s timeout (review agents are read-only analysis,
	// but need enough time to produce analysis + verdict + suggestion blocks)
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

	// Check context after agent completes — a fast agent may finish before
	// the context deadline fires, but the caller still intended cancellation.
	if ctx.Err() != nil {
		return &AdversaryResult{
			Name:     adv.Name,
			Status:   "error",
			Verdict:  "",
			Required: adv.Required,
			Output:   ctx.Err().Error(),
		}, nil
	}

	// Extract verdict from agent output
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
