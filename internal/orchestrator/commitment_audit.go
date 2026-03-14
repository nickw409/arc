package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nwiley/arc/internal/adapter"
	"github.com/nwiley/arc/internal/agent"
	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/plan"
	"github.com/nwiley/arc/internal/resources"
)

// GapReport is the structured output from the audit agent.
type GapReport struct {
	Gaps []Gap `json:"gaps"`
}

// Gap describes a single missing integration.
type Gap struct {
	Phase      string `json:"phase"`
	Commitment string `json:"commitment"`
	File       string `json:"file"`
	Pattern    string `json:"pattern"`
}

// RunCommitmentAudit audits whether all integration commitments in plan.md files
// are implemented in the worktree. Skips for simple/direct plans.
// Runs up to 2 rounds: audit → fix → re-audit. Returns a non-nil error only
// when gaps remain after 2 rounds (hard block). Soft failures (spawn errors,
// parse failures) are logged and return nil.
func RunCommitmentAudit(
	ctx context.Context,
	opts LaunchOptions,
	meta *arc.PlanMeta,
	planDir string,
	workDir string,
	planLogger *PlanLogger,
) error {
	if !shouldRunCommitmentAudit(meta, opts) {
		return nil
	}

	fmt.Println("\nRunning commitment audit...")

	// Load audit prompt template
	auditPromptBytes, err := resources.PromptBytes("commitment-audit/audit.md")
	if err != nil {
		// Missing prompt is a configuration error — log and skip (don't block)
		opts.Logger.Warn("commitment audit prompt not found, skipping", "error", err)
		return nil
	}

	// Build plan context: append each phase's plan.md content
	var planContext strings.Builder
	for _, p := range meta.Phases {
		planMDPath := filepath.Join(planDir, "phases", p, "plan.md")
		content, readErr := os.ReadFile(planMDPath)
		if readErr != nil {
			continue
		}
		planContext.WriteString(fmt.Sprintf("\n\n## Phase: %s\n\n", p))
		planContext.Write(content)
	}

	fullAuditPrompt := string(auditPromptBytes) + "\n\n# Plan Files\n" + planContext.String()

	// Determine agent adapter and model
	adapterName := "claude"
	implModel := ""
	if opts.Config != nil {
		adapterName = opts.Config.AgentForRole("impl")
		implModel = opts.Config.ModelForRole("impl")
	}
	agentAdapter := adapter.Get(adapterName)

	const maxRounds = 2
	for round := 1; round <= maxRounds; round++ {
		fmt.Printf("\nCommitment audit round %d/%d...\n", round, maxRounds)

		// Spawn read-only audit agent
		auditResult, spawnErr := agent.Spawn(ctx, agent.SpawnOptions{
			Prompt:       fullAuditPrompt,
			AllowedTools: []string{"Read", "Grep", "Glob", "Bash"},
			MaxTurns:     30,
			Timeout:      10 * time.Minute,
			WorkingDir:   workDir,
			Model:        implModel,
		})
		if spawnErr != nil {
			opts.Logger.Warn("commitment audit spawn failed, skipping", "round", round, "error", spawnErr)
			return nil // soft fail
		}

		report := parseGapReport(auditResult.Output)
		if len(report.Gaps) == 0 {
			fmt.Println("Commitment audit: no gaps found")
			return nil
		}

		fmt.Printf("Commitment audit: %d gap(s) found\n", len(report.Gaps))
		for _, g := range report.Gaps {
			fmt.Printf("  [%s] %s (expected %q in %s)\n", g.Phase, g.Commitment, g.Pattern, g.File)
		}

		if round == maxRounds {
			// Hard block after final round
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("commitment audit: %d integration gap(s) remain after %d fix round(s):\n", len(report.Gaps), maxRounds-1))
			for _, g := range report.Gaps {
				sb.WriteString(fmt.Sprintf("  [%s] %s → %s\n", g.Phase, g.Commitment, g.File))
			}
			return fmt.Errorf("%s", sb.String())
		}

		// Load fix prompt and spawn fix agent
		fixPromptBytes, fixErr := resources.PromptBytes("commitment-audit/fix.md")
		if fixErr != nil {
			opts.Logger.Warn("commitment audit fix prompt not found, skipping fix", "error", fixErr)
			return nil
		}
		gapJSON, _ := json.MarshalIndent(report, "", "  ")
		fullFixPrompt := string(fixPromptBytes) + "\n\n## Gap Report\n\n```json\n" + string(gapJSON) + "\n```\n"

		fmt.Printf("Spawning fix agent for %d gap(s)...\n", len(report.Gaps))
		_, fixSpawnErr := agentAdapter.Spawn(ctx, fullFixPrompt, workDir, arc.SessionConfig{
			MaxTurns: 150,
			Timeout:  2 * time.Hour,
			Model:    implModel,
		})
		if fixSpawnErr != nil {
			opts.Logger.Warn("commitment audit fix agent failed", "round", round, "error", fixSpawnErr)
			return nil // soft fail
		}
	}

	return nil
}

// shouldRunCommitmentAudit returns true when the plan is complex enough
// to warrant a commitment audit. Returns false for "direct" workflow type
// or when every phase that has a spec reports complexity "simple".
func shouldRunCommitmentAudit(meta *arc.PlanMeta, opts LaunchOptions) bool {
	if meta.WorkflowType == "direct" {
		return false
	}
	for _, p := range meta.Phases {
		spec, err := plan.ReadSpec(opts.PlansDir, opts.PlanName, p)
		if err != nil || spec == nil {
			continue
		}
		if spec.Complexity != "" && spec.Complexity != "simple" {
			return true // at least one non-simple phase
		}
	}
	return false
}

// parseGapReport extracts GapReport JSON from audit agent output.
// Returns an empty GapReport (no gaps) on parse failure — safe fallback.
func parseGapReport(output string) GapReport {
	// Check for explicit no-gaps signal
	if strings.Contains(output, "NO_GAPS") {
		return GapReport{}
	}
	// Find JSON code fence
	const fence = "```json"
	if idx := strings.Index(output, fence); idx >= 0 {
		rest := output[idx+len(fence):]
		if end := strings.Index(rest, "```"); end >= 0 {
			var r GapReport
			if json.Unmarshal([]byte(strings.TrimSpace(rest[:end])), &r) == nil {
				return r
			}
		}
	}
	// Try raw JSON object
	if idx := strings.Index(output, "{"); idx >= 0 {
		var r GapReport
		if json.Unmarshal([]byte(output[idx:]), &r) == nil {
			return r
		}
	}
	return GapReport{}
}
