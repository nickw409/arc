package plan

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/resources"
	"github.com/nwiley/arc/internal/state"
)

// SplitOptions configures phase splitting.
type SplitOptions struct {
	PlansDir string
	PlanName string
	Phase    string
	SubNames []string
}

// InsertOptions configures phase insertion.
type InsertOptions struct {
	PlansDir string
	PlanName string
	RefPhase string
	NewNames []string
	Before   bool // true = insert before ref, false = insert after
}

// DeferOptions configures phase deferral.
type DeferOptions struct {
	PlansDir string
	PlanName string
	Phase    string
	Reason   string
}

// Split marks the original phase as split and creates sub-phases that replace it.
func Split(opts SplitOptions) error {
	if err := validateName(opts.PlanName); err != nil {
		return fmt.Errorf("plan name: %w", err)
	}
	if len(opts.SubNames) < 2 {
		return fmt.Errorf("split requires at least 2 sub-phase names")
	}

	// Validate each sub-name
	for _, sub := range opts.SubNames {
		if err := validateName(sub); err != nil {
			return fmt.Errorf("sub-phase name: %w", err)
		}
	}

	// Check for duplicates among sub-names
	subSeen := make(map[string]bool, len(opts.SubNames))
	for _, sub := range opts.SubNames {
		if subSeen[sub] {
			return fmt.Errorf("duplicate sub-phase name %q", sub)
		}
		subSeen[sub] = true
	}

	planDir := filepath.Join(opts.PlansDir, opts.PlanName)

	meta, err := state.ReadPlan(planDir)
	if err != nil {
		return fmt.Errorf("reading plan: %w", err)
	}

	// Check for conflicts with existing phases (excluding the phase being split)
	existingPhases := make(map[string]bool, len(meta.Phases))
	for _, p := range meta.Phases {
		if p != opts.Phase {
			existingPhases[p] = true
		}
	}
	for _, sub := range opts.SubNames {
		if existingPhases[sub] {
			return fmt.Errorf("sub-phase name %q conflicts with an existing phase", sub)
		}
	}

	// Validate original phase exists
	origIdx := -1
	for i, p := range meta.Phases {
		if p == opts.Phase {
			origIdx = i
			break
		}
	}
	if origIdx < 0 {
		return fmt.Errorf("phase %q not found in plan", opts.Phase)
	}

	// Read original phase state to get workflow type
	origPhaseDir := filepath.Join(planDir, "phases", opts.Phase)
	sf := state.NewStateFile(filepath.Join(origPhaseDir, "state.json"))
	origState, err := sf.Read()
	if err != nil {
		return fmt.Errorf("reading phase state: %w", err)
	}

	// Mark original as split
	if err := sf.Update(func(s *arc.PhaseState) error {
		s.PhaseStatus = "split"
		s.SplitInto = opts.SubNames
		return nil
	}); err != nil {
		return fmt.Errorf("marking phase as split: %w", err)
	}

	// Load plan template
	planTemplate, err := resources.TemplateBytes("plan-template.md")
	if err != nil {
		return fmt.Errorf("loading plan template: %w", err)
	}

	// Read original plan.md for copying
	origPlanMD, _ := os.ReadFile(filepath.Join(origPhaseDir, "plan.md"))

	// Create sub-phase directories
	for _, sub := range opts.SubNames {
		subDir := filepath.Join(planDir, "phases", sub)
		if err := os.MkdirAll(subDir, 0755); err != nil {
			return fmt.Errorf("creating sub-phase dir %s: %w", sub, err)
		}

		subState := arc.NewPhaseState(opts.PlanName, sub, origState.WorkflowType)
		subState.ParentPhase = opts.Phase
		stateData, err := json.MarshalIndent(subState, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling state for %s: %w", sub, err)
		}
		if err := os.WriteFile(filepath.Join(subDir, "state.json"), stateData, 0644); err != nil {
			return fmt.Errorf("writing state.json for %s: %w", sub, err)
		}

		if err := os.WriteFile(filepath.Join(subDir, "plan.md"), planTemplate, 0644); err != nil {
			return fmt.Errorf("writing plan.md for %s: %w", sub, err)
		}

		if len(origPlanMD) > 0 {
			if err := os.WriteFile(filepath.Join(subDir, "original_plan.md"), origPlanMD, 0644); err != nil {
				return fmt.Errorf("writing original_plan.md for %s: %w", sub, err)
			}
		}
	}

	// Replace original in phases list with sub-phases
	newPhases := make([]string, 0, len(meta.Phases)+len(opts.SubNames)-1)
	newPhases = append(newPhases, meta.Phases[:origIdx]...)
	newPhases = append(newPhases, opts.SubNames...)
	newPhases = append(newPhases, meta.Phases[origIdx+1:]...)
	meta.Phases = newPhases

	// Rewire dependencies: sub-phases chain sequentially
	for i, sub := range opts.SubNames {
		if i == 0 {
			// First sub-phase inherits original's dependencies
			if deps, ok := meta.Dependencies[opts.Phase]; ok {
				meta.Dependencies[sub] = append([]string{}, deps...)
			}
		} else {
			meta.Dependencies[sub] = []string{opts.SubNames[i-1]}
		}
	}

	// Anything that depended on original now depends on last sub-phase
	lastSub := opts.SubNames[len(opts.SubNames)-1]
	for phase, deps := range meta.Dependencies {
		for i, dep := range deps {
			if dep == opts.Phase {
				meta.Dependencies[phase][i] = lastSub
			}
		}
	}

	// Remove original's dependency entry
	delete(meta.Dependencies, opts.Phase)

	// Rebuild PhaseOrder
	meta.PhaseOrder = make(map[string]int, len(meta.Phases))
	for i, p := range meta.Phases {
		meta.PhaseOrder[p] = i + 1
	}

	// Track split in SplitPhases
	if meta.SplitPhases == nil {
		meta.SplitPhases = make(map[string][]string)
	}
	meta.SplitPhases[opts.Phase] = opts.SubNames

	return state.WritePlan(planDir, meta)
}

// Insert inserts new phases before or after a reference phase.
func Insert(opts InsertOptions) error {
	if err := validateName(opts.PlanName); err != nil {
		return fmt.Errorf("plan name: %w", err)
	}
	if len(opts.NewNames) == 0 {
		return fmt.Errorf("no new phase names specified")
	}

	// Validate each new name
	for _, name := range opts.NewNames {
		if err := validateName(name); err != nil {
			return fmt.Errorf("new phase name: %w", err)
		}
	}

	// Check for duplicates among new names
	newSeen := make(map[string]bool, len(opts.NewNames))
	for _, name := range opts.NewNames {
		if newSeen[name] {
			return fmt.Errorf("duplicate new phase name %q", name)
		}
		newSeen[name] = true
	}

	planDir := filepath.Join(opts.PlansDir, opts.PlanName)

	meta, err := state.ReadPlan(planDir)
	if err != nil {
		return fmt.Errorf("reading plan: %w", err)
	}

	// Check for conflicts with existing phases
	existingPhases := make(map[string]bool, len(meta.Phases))
	for _, p := range meta.Phases {
		existingPhases[p] = true
	}
	for _, name := range opts.NewNames {
		if existingPhases[name] {
			return fmt.Errorf("new phase name %q conflicts with an existing phase", name)
		}
	}

	// Validate ref phase exists
	refIdx := -1
	for i, p := range meta.Phases {
		if p == opts.RefPhase {
			refIdx = i
			break
		}
	}
	if refIdx < 0 {
		return fmt.Errorf("reference phase %q not found in plan", opts.RefPhase)
	}

	// Read ref phase state to get workflow type
	refPhaseDir := filepath.Join(planDir, "phases", opts.RefPhase)
	refSF := state.NewStateFile(filepath.Join(refPhaseDir, "state.json"))
	refState, err := refSF.Read()
	if err != nil {
		return fmt.Errorf("reading ref phase state: %w", err)
	}

	planTemplate, err := resources.TemplateBytes("plan-template.md")
	if err != nil {
		return fmt.Errorf("loading plan template: %w", err)
	}

	// Create new phase directories
	for _, name := range opts.NewNames {
		phaseDir := filepath.Join(planDir, "phases", name)
		if err := os.MkdirAll(phaseDir, 0755); err != nil {
			return fmt.Errorf("creating phase dir %s: %w", name, err)
		}

		ps := arc.NewPhaseState(opts.PlanName, name, refState.WorkflowType)
		stateData, err := json.MarshalIndent(ps, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling state for %s: %w", name, err)
		}
		if err := os.WriteFile(filepath.Join(phaseDir, "state.json"), stateData, 0644); err != nil {
			return fmt.Errorf("writing state.json for %s: %w", name, err)
		}
		if err := os.WriteFile(filepath.Join(phaseDir, "plan.md"), planTemplate, 0644); err != nil {
			return fmt.Errorf("writing plan.md for %s: %w", name, err)
		}
	}

	// Insert into phases list
	var insertIdx int
	if opts.Before {
		insertIdx = refIdx
	} else {
		insertIdx = refIdx + 1
	}

	newPhases := make([]string, 0, len(meta.Phases)+len(opts.NewNames))
	newPhases = append(newPhases, meta.Phases[:insertIdx]...)
	newPhases = append(newPhases, opts.NewNames...)
	newPhases = append(newPhases, meta.Phases[insertIdx:]...)
	meta.Phases = newPhases

	// Rewire dependencies
	if opts.Before {
		// First inserted inherits ref's deps
		if deps, ok := meta.Dependencies[opts.RefPhase]; ok {
			meta.Dependencies[opts.NewNames[0]] = append([]string{}, deps...)
		}
		// Chain inserted phases
		for i := 1; i < len(opts.NewNames); i++ {
			meta.Dependencies[opts.NewNames[i]] = []string{opts.NewNames[i-1]}
		}
		// Ref now depends on last inserted
		meta.Dependencies[opts.RefPhase] = []string{opts.NewNames[len(opts.NewNames)-1]}
	} else {
		// First inserted depends on ref
		meta.Dependencies[opts.NewNames[0]] = []string{opts.RefPhase}
		// Chain subsequent
		for i := 1; i < len(opts.NewNames); i++ {
			meta.Dependencies[opts.NewNames[i]] = []string{opts.NewNames[i-1]}
		}
		// Anything that depended on ref now depends on last inserted
		lastInserted := opts.NewNames[len(opts.NewNames)-1]
		for phase, deps := range meta.Dependencies {
			// Skip the newly inserted phases themselves
			isNew := false
			for _, n := range opts.NewNames {
				if phase == n {
					isNew = true
					break
				}
			}
			if isNew {
				continue
			}
			for i, dep := range deps {
				if dep == opts.RefPhase {
					meta.Dependencies[phase][i] = lastInserted
				}
			}
		}
		// Restore ref's original deps (they shouldn't change for after-insert)
	}

	// Rebuild PhaseOrder
	meta.PhaseOrder = make(map[string]int, len(meta.Phases))
	for i, p := range meta.Phases {
		meta.PhaseOrder[p] = i + 1
	}

	return state.WritePlan(planDir, meta)
}

// Defer marks a phase as deferred with a reason and timestamp.
func Defer(opts DeferOptions) error {
	planDir := filepath.Join(opts.PlansDir, opts.PlanName)

	// Validate phase exists in plan
	meta, err := state.ReadPlan(planDir)
	if err != nil {
		return fmt.Errorf("reading plan: %w", err)
	}

	found := false
	for _, p := range meta.Phases {
		if p == opts.Phase {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("phase %q not found in plan", opts.Phase)
	}

	phaseDir := filepath.Join(planDir, "phases", opts.Phase)
	sf := state.NewStateFile(filepath.Join(phaseDir, "state.json"))

	return sf.Update(func(s *arc.PhaseState) error {
		s.PhaseStatus = "deferred"
		s.DeferredReason = opts.Reason
		s.DeferredAt = time.Now().UTC().Format(time.RFC3339)
		return nil
	})
}
