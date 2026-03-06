package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const stateSchemaVersion = 1

// DaemonState is the top-level persisted state for the daemon.
type DaemonState struct {
	Schema        int                     `json:"schema"`
	Registrations []PersistedRegistration `json:"registrations"`
}

// PersistedRegistration is the serializable subset of PlanRegistration.
type PersistedRegistration struct {
	PlanName         string    `json:"plan_name"`
	ProjectDir       string    `json:"project_dir"`
	PlansDir         string    `json:"plans_dir"`
	ArcHome          string    `json:"arc_home"`
	Timeout          int       `json:"timeout,omitempty"`
	UseWorktree      bool      `json:"use_worktree,omitempty"`
	PerPhaseWorktree bool      `json:"per_phase_worktree,omitempty"`
	StopOnFailure    bool      `json:"stop_on_failure,omitempty"`
	ChatMode         bool      `json:"chat_mode,omitempty"`
	ConfigPath       string    `json:"config_path,omitempty"`
	SubmittedAt      time.Time `json:"submitted_at"`
	WorktreeDir      string    `json:"worktree_dir,omitempty"`
	WorktreeBranch   string    `json:"worktree_branch,omitempty"`
}

// LoadState reads daemon state from the given path.
func LoadState(path string) (*DaemonState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s DaemonState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// SaveState writes daemon state atomically (write to .tmp, then rename).
func SaveState(path string, s *DaemonState) error {
	s.Schema = stateSchemaVersion
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := path + ".tmp"
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// ToPersisted converts a PlanRegistration to a PersistedRegistration.
func (reg *PlanRegistration) ToPersisted() PersistedRegistration {
	p := PersistedRegistration{
		PlanName:         reg.PlanName,
		ProjectDir:       reg.ProjectDir,
		PlansDir:         reg.PlansDir,
		ArcHome:          reg.ArcHome,
		Timeout:          reg.Timeout,
		UseWorktree:      reg.UseWorktree,
		PerPhaseWorktree: reg.PerPhaseWorktree,
		StopOnFailure:    reg.StopOnFailure,
		ChatMode:         reg.ChatMode,
		ConfigPath:       reg.ConfigPath,
		SubmittedAt:      reg.SubmittedAt,
	}
	if reg.Worktree != nil {
		p.WorktreeDir = reg.Worktree.Dir
		p.WorktreeBranch = reg.Worktree.Branch
	}
	return p
}

// ToRegistration converts a PersistedRegistration back to a PlanRegistration.
// Runtime fields (Config, Resolver, PlanLogger, Ctx, Cancel) are left nil/zero
// and must be populated by the caller.
func (p PersistedRegistration) ToRegistration() *PlanRegistration {
	return &PlanRegistration{
		PlanName:         p.PlanName,
		ProjectDir:       p.ProjectDir,
		PlansDir:         p.PlansDir,
		ArcHome:          p.ArcHome,
		Timeout:          p.Timeout,
		UseWorktree:      p.UseWorktree,
		PerPhaseWorktree: p.PerPhaseWorktree,
		StopOnFailure:    p.StopOnFailure,
		ChatMode:         p.ChatMode,
		ConfigPath:       p.ConfigPath,
		SubmittedAt:      p.SubmittedAt,
	}
}
