package daemon

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nwiley/arc/internal/worktree"
)

func TestSaveAndLoadState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	now := time.Now().Truncate(time.Second)
	original := &DaemonState{
		Registrations: []PersistedRegistration{
			{
				PlanName:    "plan-a",
				ProjectDir:  "/tmp/project",
				PlansDir:    "/tmp/plans",
				ArcHome:     "/home/user/.arc",
				Timeout:     600,
				UseWorktree: true,
				ConfigPath:  "/tmp/.arc.yaml",
				SubmittedAt: now,
			},
			{
				PlanName:       "plan-b",
				ProjectDir:     "/tmp/project2",
				PlansDir:       "/tmp/plans2",
				ArcHome:        "/home/user/.arc",
				StopOnFailure:  true,
				SubmittedAt:    now,
				WorktreeDir:    "/tmp/wt",
				WorktreeBranch: "arc/plan-b",
			},
		},
	}

	if err := SaveState(path, original); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	loaded, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}

	if loaded.Schema != stateSchemaVersion {
		t.Errorf("Schema: got %d, want %d", loaded.Schema, stateSchemaVersion)
	}
	if len(loaded.Registrations) != 2 {
		t.Fatalf("Registrations: got %d, want 2", len(loaded.Registrations))
	}
	r0 := loaded.Registrations[0]
	if r0.PlanName != "plan-a" {
		t.Errorf("PlanName: got %q, want %q", r0.PlanName, "plan-a")
	}
	if r0.Timeout != 600 {
		t.Errorf("Timeout: got %d, want 600", r0.Timeout)
	}
	if !r0.UseWorktree {
		t.Error("UseWorktree: got false, want true")
	}

	r1 := loaded.Registrations[1]
	if r1.WorktreeDir != "/tmp/wt" {
		t.Errorf("WorktreeDir: got %q, want %q", r1.WorktreeDir, "/tmp/wt")
	}
}

func TestSaveStateAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	// Save once
	if err := SaveState(path, &DaemonState{}); err != nil {
		t.Fatalf("first SaveState: %v", err)
	}

	// Save again — should overwrite atomically
	if err := SaveState(path, &DaemonState{
		Registrations: []PersistedRegistration{
			{PlanName: "new-plan"},
		},
	}); err != nil {
		t.Fatalf("second SaveState: %v", err)
	}

	// Verify no .tmp file remains
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Error("expected .tmp file to not exist after atomic rename")
	}

	loaded, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if len(loaded.Registrations) != 1 || loaded.Registrations[0].PlanName != "new-plan" {
		t.Error("state was not properly overwritten")
	}
}

func TestLoadStateMissing(t *testing.T) {
	_, err := LoadState("/nonexistent/path.json")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestToPersisted(t *testing.T) {
	now := time.Now()
	reg := &PlanRegistration{
		PlanName:         "my-plan",
		ProjectDir:       "/project",
		PlansDir:         "/plans",
		ArcHome:          "/home/.arc",
		Timeout:          120,
		UseWorktree:      true,
		PerPhaseWorktree: false,
		StopOnFailure:    true,
		ChatMode:         false,
		ConfigPath:       "/config.yaml",
		SubmittedAt:      now,
		Worktree: &worktree.Worktree{
			Dir:    "/tmp/wt-dir",
			Branch: "arc/my-plan",
		},
	}

	p := reg.ToPersisted()

	if p.PlanName != "my-plan" {
		t.Errorf("PlanName: got %q, want %q", p.PlanName, "my-plan")
	}
	if p.WorktreeDir != "/tmp/wt-dir" {
		t.Errorf("WorktreeDir: got %q, want %q", p.WorktreeDir, "/tmp/wt-dir")
	}
	if p.WorktreeBranch != "arc/my-plan" {
		t.Errorf("WorktreeBranch: got %q, want %q", p.WorktreeBranch, "arc/my-plan")
	}
	if !p.StopOnFailure {
		t.Error("StopOnFailure: got false, want true")
	}
}

func TestToPersistedNilWorktree(t *testing.T) {
	reg := &PlanRegistration{
		PlanName: "no-wt",
	}
	p := reg.ToPersisted()
	if p.WorktreeDir != "" {
		t.Errorf("WorktreeDir: got %q, want empty", p.WorktreeDir)
	}
}

func TestToRegistration(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	p := PersistedRegistration{
		PlanName:      "restored-plan",
		ProjectDir:    "/proj",
		PlansDir:      "/plans",
		ArcHome:       "/home/.arc",
		Timeout:       300,
		UseWorktree:   true,
		StopOnFailure: true,
		ConfigPath:    "/cfg.yaml",
		SubmittedAt:   now,
	}

	reg := p.ToRegistration()

	if reg.PlanName != "restored-plan" {
		t.Errorf("PlanName: got %q, want %q", reg.PlanName, "restored-plan")
	}
	if reg.Timeout != 300 {
		t.Errorf("Timeout: got %d, want 300", reg.Timeout)
	}
	if !reg.UseWorktree {
		t.Error("UseWorktree: got false, want true")
	}
	if !reg.SubmittedAt.Equal(now) {
		t.Errorf("SubmittedAt: got %v, want %v", reg.SubmittedAt, now)
	}
	// Runtime fields should be nil/zero
	if reg.Config != nil {
		t.Error("Config should be nil")
	}
	if reg.Ctx != nil {
		t.Error("Ctx should be nil")
	}
}
