package daemon

import (
	"testing"

	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/config"
	"github.com/nwiley/arc/internal/orchestrator"
	"github.com/nwiley/arc/internal/resources"
	"github.com/nwiley/arc/internal/worktree"
)

func testRegistration() *PlanRegistration {
	return &PlanRegistration{
		PlanName:   "test-plan",
		ProjectDir: "/tmp/project",
		PlansDir:   "/tmp/project/.plans/active",
		ArcHome:    "/tmp/project/.arc",
		Config:     &config.Config{},
		Resolver:   resources.NewResolver("", ""),
		PlanLogger: nil,
		ChatMode:   false,
		Meta: &arc.PlanMeta{
			Phases: []string{"phase1", "phase2"},
		},
		PhaseStates: map[string]*arc.PhaseState{
			"phase1": {PhaseStatus: "pending"},
			"phase2": {PhaseStatus: "pending"},
		},
	}
}

func TestBuildPhaseOptions_Basic(t *testing.T) {
	reg := testRegistration()
	opts := BuildPhaseOptions(reg, "phase1")

	if opts.PlanName != "test-plan" {
		t.Errorf("PlanName = %q, want %q", opts.PlanName, "test-plan")
	}
	if opts.PhaseName != "phase1" {
		t.Errorf("PhaseName = %q, want %q", opts.PhaseName, "phase1")
	}
	if opts.PlansDir != reg.PlansDir {
		t.Errorf("PlansDir = %q, want %q", opts.PlansDir, reg.PlansDir)
	}
	if opts.ArcHome != reg.ArcHome {
		t.Errorf("ArcHome = %q, want %q", opts.ArcHome, reg.ArcHome)
	}
	if opts.ProjectDir != reg.ProjectDir {
		t.Errorf("ProjectDir = %q, want %q", opts.ProjectDir, reg.ProjectDir)
	}
	if opts.Config != reg.Config {
		t.Error("Config mismatch")
	}
	if opts.Resolver != reg.Resolver {
		t.Error("Resolver mismatch")
	}
	if opts.UseWorktree {
		t.Error("UseWorktree should be false")
	}
	if opts.WorkingDir != "" {
		t.Errorf("WorkingDir should be empty, got %q", opts.WorkingDir)
	}
}

func TestBuildPhaseOptions_PerPhaseWorktree(t *testing.T) {
	reg := testRegistration()
	reg.PerPhaseWorktree = true

	opts := BuildPhaseOptions(reg, "phase1")
	if !opts.UseWorktree {
		t.Error("UseWorktree should be true for per-phase worktree")
	}
	if opts.WorkingDir != "" {
		t.Errorf("WorkingDir should be empty for per-phase worktree, got %q", opts.WorkingDir)
	}
}

func TestBuildPhaseOptions_SharedWorktree(t *testing.T) {
	reg := testRegistration()
	reg.Worktree = &worktree.Worktree{
		Dir:    "/tmp/worktree",
		Branch: "arc/test-plan",
	}

	opts := BuildPhaseOptions(reg, "phase1")
	if opts.UseWorktree {
		t.Error("UseWorktree should be false for shared worktree")
	}
	if opts.WorkingDir != "/tmp/worktree" {
		t.Errorf("WorkingDir = %q, want %q", opts.WorkingDir, "/tmp/worktree")
	}
}

func TestBuildPhaseOptions_ChatMode(t *testing.T) {
	reg := testRegistration()
	reg.ChatMode = true

	opts := BuildPhaseOptions(reg, "phase1")
	if !opts.ChatMode {
		t.Error("ChatMode should be true")
	}
}

func TestBuildLaunchOptions(t *testing.T) {
	reg := testRegistration()
	reg.ConfigPath = "/tmp/project/.arc.yaml"
	reg.Timeout = 300
	reg.UseWorktree = true
	reg.PerPhaseWorktree = true
	reg.StopOnFailure = true

	opts := BuildLaunchOptions(reg)
	if opts.PlanName != reg.PlanName {
		t.Errorf("PlanName = %q, want %q", opts.PlanName, reg.PlanName)
	}
	if opts.PlansDir != reg.PlansDir {
		t.Errorf("PlansDir = %q, want %q", opts.PlansDir, reg.PlansDir)
	}
	if opts.ConfigPath != reg.ConfigPath {
		t.Errorf("ConfigPath = %q, want %q", opts.ConfigPath, reg.ConfigPath)
	}
	if opts.Timeout != reg.Timeout {
		t.Errorf("Timeout = %d, want %d", opts.Timeout, reg.Timeout)
	}
	if !opts.UseWorktree {
		t.Error("UseWorktree should be true")
	}
	if !opts.PerPhaseWorktree {
		t.Error("PerPhaseWorktree should be true")
	}
	if !opts.StopOnFailure {
		t.Error("StopOnFailure should be true")
	}
	if opts.Config != reg.Config {
		t.Error("Config mismatch")
	}
	if opts.Resolver != reg.Resolver {
		t.Error("Resolver mismatch")
	}
}

func TestDefaultPhaseRunner_ReturnsFunction(t *testing.T) {
	runner := DefaultPhaseRunner(nil)
	if runner == nil {
		t.Fatal("DefaultPhaseRunner returned nil")
	}
}

func TestDefaultFinalizer_ReturnsFunction(t *testing.T) {
	finalizer := DefaultFinalizer()
	if finalizer == nil {
		t.Fatal("DefaultFinalizer returned nil")
	}
}

func TestBuildPhaseOptions_PlanLogger(t *testing.T) {
	reg := testRegistration()
	pl := orchestrator.NewPlanLogger(t.TempDir(), nil)
	defer pl.Close()
	reg.PlanLogger = pl

	opts := BuildPhaseOptions(reg, "phase1")
	if opts.PlanLogger != pl {
		t.Error("PlanLogger should be passed through")
	}
}
