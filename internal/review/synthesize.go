// synthesize.go implements the synthesizer agent for the review package.
// It is called by review.go when adversaries flag issues, and rewrites plan.md
// based on their critique output files.
package review

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/nwiley/arc/internal/agent"
	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/resources"
)

// SynthesisOptions configures the synthesizer agent.
type SynthesisOptions struct {
	PlanDir         string   // absolute path to plan dir, e.g. .plans/active/my-plan
	PhaseName       string
	FailedCritiques []string // adversary names that failed, e.g. ["coverage", "ambiguity"]
	Model           string
	CommandName     string // agent binary; empty defaults to "claude"
	ProjectContext  string
}

// RunSynthesizer spawns one agent to synthesize adversary critiques into an
// improved plan.md. The agent writes plan.md directly to disk.
// Returns (changed bool, usage arc.Usage, error).
// Non-blocking: caller logs on error and proceeds.
func RunSynthesizer(ctx context.Context, opts SynthesisOptions) (bool, arc.Usage, error) {
	commandName := opts.CommandName
	if commandName == "" {
		commandName = "claude"
	}

	// Build critique file paths.
	var critiquePaths []string
	for _, name := range opts.FailedCritiques {
		critiquePaths = append(critiquePaths, filepath.Join(opts.PlanDir, "reviews", opts.PhaseName+"_"+name+".md"))
	}
	critiqueFileList := strings.Join(critiquePaths, "\n")

	planMDPath := filepath.Join(opts.PlanDir, "phases", opts.PhaseName, "plan.md")
	hashBefore, err := ComputePlanHash(planMDPath)
	if err != nil {
		return false, arc.Usage{}, err
	}

	promptBytes, err := resources.PromptBytes("adversaries/synthesizer.md")
	if err != nil {
		return false, arc.Usage{}, err
	}

	fullPrompt := strings.ReplaceAll(string(promptBytes), "{PLAN_PATH}", planMDPath)
	fullPrompt = strings.ReplaceAll(fullPrompt, "{CRITIQUE_FILES}", critiqueFileList)

	spawnResult, err := agent.Spawn(ctx, agent.SpawnOptions{
		Prompt:       fullPrompt,
		AllowedTools: []string{"Read", "Write"},
		CommandName:  commandName,
		Timeout:      180 * time.Second,
		Model:        opts.Model,
	})
	if err != nil {
		return false, arc.Usage{}, err
	}

	hashAfter, _ := ComputePlanHash(planMDPath)
	return hashBefore != hashAfter, spawnResult.Usage, nil
}
