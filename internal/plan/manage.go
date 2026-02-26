package plan

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/state"
)

// ManageOptions configures phase management operations.
type ManageOptions struct {
	PlansDir    string
	PlanName    string
	Phase       string
	Reason      string
	Passing     int
	Total       int
	Packages    []string
	Note        string
	Iteration   int
	SourcePhase string
	Activity    string
}

func stateFileFor(opts ManageOptions) *state.StateFile {
	path := filepath.Join(opts.PlansDir, opts.PlanName, "phases", opts.Phase, "state.json")
	return state.NewStateFile(path)
}

// ManageComplete sets phase status to complete with a timestamp.
func ManageComplete(opts ManageOptions) error {
	return stateFileFor(opts).Update(func(s *arc.PhaseState) error {
		s.PhaseStatus = "complete"
		s.CompletedAt = time.Now().UTC().Format(time.RFC3339)
		return nil
	})
}

// ManagePending resets phase status to pending, clearing deferred and blocked fields.
func ManagePending(opts ManageOptions) error {
	return stateFileFor(opts).Update(func(s *arc.PhaseState) error {
		s.PhaseStatus = "pending"
		s.DeferredReason = ""
		s.DeferredAt = ""
		s.BlockedReason = ""
		s.BlockedAt = ""
		s.Blocked = arc.BlockedInfo{}
		return nil
	})
}

// ManageDefer delegates to the Defer function.
func ManageDefer(opts ManageOptions) error {
	return Defer(DeferOptions{
		PlansDir: opts.PlansDir,
		PlanName: opts.PlanName,
		Phase:    opts.Phase,
		Reason:   opts.Reason,
	})
}

// ManageBlock sets phase status to blocked with a reason.
func ManageBlock(opts ManageOptions) error {
	return stateFileFor(opts).Update(func(s *arc.PhaseState) error {
		s.PhaseStatus = "blocked"
		s.BlockedReason = opts.Reason
		s.BlockedAt = time.Now().UTC().Format(time.RFC3339)
		s.Blocked = arc.BlockedInfo{
			IsBlocked: true,
			Reason:    &opts.Reason,
		}
		return nil
	})
}

// ManageTests updates test passing/total counts.
func ManageTests(opts ManageOptions) error {
	if opts.Passing < 0 {
		return fmt.Errorf("passing count must be non-negative, got %d", opts.Passing)
	}
	if opts.Total < 0 {
		return fmt.Errorf("total count must be non-negative, got %d", opts.Total)
	}
	if opts.Passing > opts.Total {
		return fmt.Errorf("passing count (%d) cannot exceed total (%d)", opts.Passing, opts.Total)
	}
	return stateFileFor(opts).Update(func(s *arc.PhaseState) error {
		s.TestsPassing = opts.Passing
		s.TestsTotal = opts.Total
		return nil
	})
}

// ManagePackages sets the packages list.
func ManagePackages(opts ManageOptions) error {
	return stateFileFor(opts).Update(func(s *arc.PhaseState) error {
		s.Packages = opts.Packages
		if s.Packages == nil {
			s.Packages = []string{}
		}
		return nil
	})
}

// ManageNote sets the notes field.
func ManageNote(opts ManageOptions) error {
	return stateFileFor(opts).Update(func(s *arc.PhaseState) error {
		s.Notes = opts.Note
		return nil
	})
}

// ManageIteration sets the current iteration.
func ManageIteration(opts ManageOptions) error {
	return stateFileFor(opts).Update(func(s *arc.PhaseState) error {
		s.Iteration.Current = opts.Iteration
		return nil
	})
}

// ManageActivity sets or clears the agent activity message.
func ManageActivity(opts ManageOptions) error {
	return stateFileFor(opts).Update(func(s *arc.PhaseState) error {
		s.Activity = opts.Activity
		if opts.Activity == "" {
			s.ActivityUpdatedAt = ""
		} else {
			s.ActivityUpdatedAt = time.Now().UTC().Format(time.RFC3339)
		}
		return nil
	})
}

// ManageCopyFrom copies state from another phase.
func ManageCopyFrom(opts ManageOptions) error {
	srcPath := filepath.Join(opts.PlansDir, opts.PlanName, "phases", opts.SourcePhase, "state.json")
	srcSF := state.NewStateFile(srcPath)
	src, err := srcSF.Read()
	if err != nil {
		return fmt.Errorf("reading source phase %q: %w", opts.SourcePhase, err)
	}

	return stateFileFor(opts).Update(func(s *arc.PhaseState) error {
		// Preserve identity fields
		phase := s.Phase
		plan := s.Plan

		*s = *src
		s.Phase = phase
		s.Plan = plan
		return nil
	})
}

// ManageShow writes the phase state to the writer in formatted JSON.
func ManageShow(w io.Writer, opts ManageOptions) error {
	sf := stateFileFor(opts)
	s, err := sf.Read()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}

	_, err = fmt.Fprintln(w, string(data))
	return err
}

// ManageShowFromDir reads state.json from a phase directory and writes it.
// This is a convenience for when you already have the phase directory path.
func ManageShowFromDir(w io.Writer, phaseDir string) error {
	data, err := os.ReadFile(filepath.Join(phaseDir, "state.json"))
	if err != nil {
		return err
	}

	// Re-indent for consistent output
	var s arc.PhaseState
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	out, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(w, string(out))
	return err
}
