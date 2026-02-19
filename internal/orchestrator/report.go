package orchestrator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nwiley/arc/internal/arc"
)

// generateCompletionReport creates COMPLETION_REPORT.md for a finished plan.
func generateCompletionReport(planDir, planName string, meta *arc.PlanMeta, phaseStates map[string]*arc.PhaseState) error {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("# Completion Report: %s\n\n", planName))
	b.WriteString(fmt.Sprintf("Generated: %s\n\n", time.Now().UTC().Format(time.RFC3339)))

	// Summary
	complete, blocked, total := 0, 0, len(meta.Phases)
	for _, phase := range meta.Phases {
		ps := phaseStates[phase]
		if ps == nil {
			continue
		}
		switch ps.PhaseStatus {
		case "complete", "split", "deferred":
			complete++
		case "blocked":
			blocked++
		}
	}

	b.WriteString("## Summary\n\n")
	b.WriteString(fmt.Sprintf("- **Phases**: %d/%d complete\n", complete, total))
	if blocked > 0 {
		b.WriteString(fmt.Sprintf("- **Blocked**: %d\n", blocked))
	}
	b.WriteString("\n")

	// Per-phase details
	b.WriteString("## Phases\n\n")
	for _, phase := range meta.Phases {
		ps := phaseStates[phase]
		if ps == nil {
			b.WriteString(fmt.Sprintf("### %s\n\nNo state found.\n\n", phase))
			continue
		}

		status := ps.PhaseStatus
		icon := "?"
		switch status {
		case "complete":
			icon = "x"
		case "blocked":
			icon = "X"
		case "split":
			icon = "/"
		case "deferred":
			icon = "-"
		}

		b.WriteString(fmt.Sprintf("### [%s] %s\n\n", icon, phase))
		b.WriteString(fmt.Sprintf("- **Status**: %s\n", status))
		b.WriteString(fmt.Sprintf("- **Iterations**: %d\n", ps.Iteration.Current))
		b.WriteString(fmt.Sprintf("- **Tests**: %d/%d\n", ps.TestsPassing, ps.TestsTotal))
		if ps.LastCommit != "" {
			b.WriteString(fmt.Sprintf("- **Last commit**: %s\n", ps.LastCommit))
		}
		if ps.RollbackCount > 0 {
			b.WriteString(fmt.Sprintf("- **Rollbacks**: %d\n", ps.RollbackCount))
		}
		b.WriteString("\n")
	}

	reportPath := filepath.Join(planDir, "COMPLETION_REPORT.md")
	if err := os.WriteFile(reportPath, []byte(b.String()), 0644); err != nil {
		return fmt.Errorf("writing completion report: %w", err)
	}

	fmt.Printf("\nCompletion report written to: %s\n", reportPath)
	return nil
}
