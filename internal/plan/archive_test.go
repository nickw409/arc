package plan

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/state"
)

func setupArchiveTest(t *testing.T, phases []string, statuses map[string]string, completionReport bool) (string, string) {
	t.Helper()

	base := t.TempDir()
	plansDir := filepath.Join(base, "active")
	archiveDir := filepath.Join(base, "archive")
	planDir := filepath.Join(plansDir, "test-plan")

	meta := arc.NewPlanMeta("test-plan", "feature", phases)
	if err := os.MkdirAll(planDir, 0755); err != nil {
		t.Fatal(err)
	}
	metaData, _ := json.MarshalIndent(meta, "", "  ")
	if err := os.WriteFile(filepath.Join(planDir, "plan.json"), metaData, 0644); err != nil {
		t.Fatal(err)
	}

	for _, p := range phases {
		phaseDir := filepath.Join(planDir, "phases", p)
		if err := os.MkdirAll(phaseDir, 0755); err != nil {
			t.Fatal(err)
		}

		ps := arc.NewPhaseState("test-plan", p, "feature")
		if status, ok := statuses[p]; ok {
			ps.PhaseStatus = status
		}
		sd, _ := json.MarshalIndent(ps, "", "  ")
		if err := os.WriteFile(filepath.Join(phaseDir, "state.json"), sd, 0644); err != nil {
			t.Fatal(err)
		}
	}

	if completionReport {
		if err := os.WriteFile(filepath.Join(planDir, "COMPLETION_REPORT.md"), []byte("# Done"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	return plansDir, archiveDir
}

func TestArchiveHappyPath(t *testing.T) {
	plansDir, archiveDir := setupArchiveTest(t,
		[]string{"core", "api"},
		map[string]string{"core": "complete", "api": "complete"},
		true,
	)

	err := Archive(ArchiveOptions{
		PlansDir:   plansDir,
		ArchiveDir: archiveDir,
		PlanName:   "test-plan",
	})
	if err != nil {
		t.Fatalf("Archive error: %v", err)
	}

	// Plan should no longer be in active
	if _, err := os.Stat(filepath.Join(plansDir, "test-plan")); !os.IsNotExist(err) {
		t.Fatal("plan should have been moved from active")
	}

	// Plan should be in archive
	archivedPlan := filepath.Join(archiveDir, "test-plan")
	if _, err := os.Stat(archivedPlan); err != nil {
		t.Fatalf("plan should exist in archive: %v", err)
	}

	// plan.json should have archived status
	meta, err := state.ReadPlan(archivedPlan)
	if err != nil {
		t.Fatalf("reading archived plan: %v", err)
	}
	if meta.Status != "archived" {
		t.Fatalf("status = %q, want %q", meta.Status, "archived")
	}
	if meta.ArchivedAt == "" {
		t.Fatal("archived_at should not be empty")
	}
}

func TestArchiveWithMixedTerminal(t *testing.T) {
	plansDir, archiveDir := setupArchiveTest(t,
		[]string{"core", "api"},
		map[string]string{"core": "complete", "api": "deferred"},
		true,
	)

	err := Archive(ArchiveOptions{
		PlansDir:   plansDir,
		ArchiveDir: archiveDir,
		PlanName:   "test-plan",
	})
	if err != nil {
		t.Fatalf("Archive error: %v (split/deferred should be terminal)", err)
	}
}

func TestArchiveRejectsInProgress(t *testing.T) {
	plansDir, archiveDir := setupArchiveTest(t,
		[]string{"core", "api"},
		map[string]string{"core": "complete", "api": "implementing"},
		true,
	)

	err := Archive(ArchiveOptions{
		PlansDir:   plansDir,
		ArchiveDir: archiveDir,
		PlanName:   "test-plan",
	})
	if err == nil {
		t.Fatal("expected error for in-progress phase")
	}
}

func TestArchiveRejectsMissingCompletionReport(t *testing.T) {
	plansDir, archiveDir := setupArchiveTest(t,
		[]string{"core"},
		map[string]string{"core": "complete"},
		false, // no completion report
	)

	err := Archive(ArchiveOptions{
		PlansDir:   plansDir,
		ArchiveDir: archiveDir,
		PlanName:   "test-plan",
	})
	if err == nil {
		t.Fatal("expected error for missing COMPLETION_REPORT.md")
	}
}

func TestArchiveForce(t *testing.T) {
	plansDir, archiveDir := setupArchiveTest(t,
		[]string{"core", "api"},
		map[string]string{"core": "complete", "api": "implementing"},
		false, // no completion report either
	)

	err := Archive(ArchiveOptions{
		PlansDir:   plansDir,
		ArchiveDir: archiveDir,
		PlanName:   "test-plan",
		Force:      true,
	})
	if err != nil {
		t.Fatalf("Archive --force error: %v", err)
	}

	// Should be archived despite non-terminal phases
	archivedPlan := filepath.Join(archiveDir, "test-plan")
	if _, err := os.Stat(archivedPlan); err != nil {
		t.Fatalf("plan should exist in archive: %v", err)
	}
}

func TestArchiveNonexistent(t *testing.T) {
	base := t.TempDir()
	plansDir := filepath.Join(base, "active")
	archiveDir := filepath.Join(base, "archive")
	if err := os.MkdirAll(plansDir, 0755); err != nil {
		t.Fatal(err)
	}

	err := Archive(ArchiveOptions{
		PlansDir:   plansDir,
		ArchiveDir: archiveDir,
		PlanName:   "nonexistent",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent plan")
	}
}

func TestArchiveRemovesLock(t *testing.T) {
	plansDir, archiveDir := setupArchiveTest(t,
		[]string{"core"},
		map[string]string{"core": "complete"},
		true,
	)

	// Create lock file
	lockFile := filepath.Join(plansDir, "test-plan", ".orchestrator.lock")
	if err := os.WriteFile(lockFile, []byte("locked"), 0644); err != nil {
		t.Fatal(err)
	}

	err := Archive(ArchiveOptions{
		PlansDir:   plansDir,
		ArchiveDir: archiveDir,
		PlanName:   "test-plan",
	})
	if err != nil {
		t.Fatalf("Archive error: %v", err)
	}

	// Lock should not be in archive
	archivedLock := filepath.Join(archiveDir, "test-plan", ".orchestrator.lock")
	if _, err := os.Stat(archivedLock); !os.IsNotExist(err) {
		t.Fatal("lock file should have been removed before archiving")
	}
}

func TestArchiveNameCollision(t *testing.T) {
	// Archive a plan, then create another with the same name and archive it.
	// The second archive should get a timestamp suffix.
	plansDir, archiveDir := setupArchiveTest(t,
		[]string{"core"},
		map[string]string{"core": "complete"},
		true,
	)

	// First archive
	err := Archive(ArchiveOptions{
		PlansDir:   plansDir,
		ArchiveDir: archiveDir,
		PlanName:   "test-plan",
	})
	if err != nil {
		t.Fatalf("first Archive error: %v", err)
	}

	// Verify first archive exists
	if _, err := os.Stat(filepath.Join(archiveDir, "test-plan")); err != nil {
		t.Fatalf("first archived plan should exist: %v", err)
	}

	// Create another plan with the same name
	plansDir2, _ := setupArchiveTest(t,
		[]string{"core"},
		map[string]string{"core": "complete"},
		true,
	)

	// Archive the second plan to the same archive dir
	err = Archive(ArchiveOptions{
		PlansDir:   plansDir2,
		ArchiveDir: archiveDir,
		PlanName:   "test-plan",
	})
	if err != nil {
		t.Fatalf("second Archive error: %v", err)
	}

	// The second archived plan should exist with a timestamp suffix
	entries, err := os.ReadDir(archiveDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < 2 {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Fatalf("expected at least 2 archived plans, got %d: %v", len(entries), names)
	}
}
