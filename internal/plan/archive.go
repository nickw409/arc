package plan

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nwiley/arc/internal/state"
	"github.com/nwiley/arc/internal/worktree"
)

// ArchiveOptions configures plan archiving.
type ArchiveOptions struct {
	PlansDir   string
	ArchiveDir string
	PlanName   string
	ProjectDir string // git project root for worktree cleanup; empty derives from PlansDir
	Force      bool
}

// Archive moves a plan from active to archive after validating all phases are terminal.
func Archive(opts ArchiveOptions) error {
	if err := validateName(opts.PlanName); err != nil {
		return fmt.Errorf("plan name: %w", err)
	}
	planDir := filepath.Join(opts.PlansDir, opts.PlanName)

	if _, err := os.Stat(planDir); os.IsNotExist(err) {
		return fmt.Errorf("plan %q not found", opts.PlanName)
	}

	meta, err := state.ReadPlan(planDir)
	if err != nil {
		return fmt.Errorf("reading plan: %w", err)
	}

	if !opts.Force {
		// Validate all phases are terminal
		for _, phase := range meta.Phases {
			statePath := filepath.Join(planDir, "phases", phase, "state.json")
			sf := state.NewStateFile(statePath)
			s, err := sf.Read()
			if err != nil {
				return fmt.Errorf("reading state for phase %s: %w", phase, err)
			}

			if !isTerminalStatus(s.PhaseStatus) {
				return fmt.Errorf("phase %q has non-terminal status %q (use --force to override)", phase, s.PhaseStatus)
			}
		}

		// Check for COMPLETION_REPORT.md
		completionReport := filepath.Join(planDir, "COMPLETION_REPORT.md")
		if _, err := os.Stat(completionReport); os.IsNotExist(err) {
			return fmt.Errorf("COMPLETION_REPORT.md not found (use --force to override)")
		}
	}

	// Update plan.json
	meta.Status = "archived"
	meta.ArchivedAt = time.Now().UTC().Format(time.RFC3339)
	if err := state.WritePlan(planDir, meta); err != nil {
		return fmt.Errorf("updating plan.json: %w", err)
	}

	// Remove orchestrator lock if present
	lockFile := filepath.Join(planDir, ".orchestrator.lock")
	os.Remove(lockFile) // ignore error if not present

	// Clean up any lingering worktrees for this plan
	projectDir := opts.ProjectDir
	if projectDir == "" {
		// PlansDir is typically <projectDir>/.plans/active
		projectDir = filepath.Dir(filepath.Dir(opts.PlansDir))
	}
	if n := worktree.CleanupPlan(projectDir, opts.PlanName); n > 0 {
		fmt.Printf("Cleaned up %d worktree(s) for plan %q\n", n, opts.PlanName)
	}

	// Move to archive
	if err := os.MkdirAll(opts.ArchiveDir, 0755); err != nil {
		return fmt.Errorf("creating archive dir: %w", err)
	}

	archivePath := filepath.Join(opts.ArchiveDir, opts.PlanName)
	if _, err := os.Stat(archivePath); err == nil {
		// Duplicate — add timestamp suffix
		archivePath = archivePath + "-" + time.Now().UTC().Format("20060102-150405")
	}

	if err := os.Rename(planDir, archivePath); err != nil {
		return fmt.Errorf("moving plan to archive: %w", err)
	}

	return nil
}

func isTerminalStatus(status string) bool {
	switch status {
	case "complete", "split", "deferred":
		return true
	default:
		return false
	}
}
