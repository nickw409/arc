package plan

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/state"
	"gopkg.in/yaml.v3"
)

// specPath returns the path to spec.yaml for a phase.
func specPath(plansDir, planName, phaseName string) string {
	return filepath.Join(plansDir, planName, "phases", phaseName, "spec.yaml")
}

// WriteSpec writes a PhaseSpec as spec.yaml to the phase directory.
func WriteSpec(plansDir, planName, phaseName string, spec *arc.PhaseSpec) error {
	data, err := yaml.Marshal(spec)
	if err != nil {
		return fmt.Errorf("marshal spec.yaml: %w", err)
	}
	path := specPath(plansDir, planName, phaseName)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write spec.yaml: %w", err)
	}
	return nil
}

// ReadSpec reads spec.yaml from the phase directory.
func ReadSpec(plansDir, planName, phaseName string) (*arc.PhaseSpec, error) {
	path := specPath(plansDir, planName, phaseName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read spec.yaml: %w", err)
	}
	var spec arc.PhaseSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("parse spec.yaml: %w", err)
	}
	return &spec, nil
}

// AddPhase adds a new phase to an existing plan with a spec.
// Creates the phase directory, writes spec.yaml, state.json, and updates plan.json.
// Also writes plan.md (from spec.Spec field) for backward compatibility with v1 tools.
func AddPhase(plansDir, planName, phaseName string, spec *arc.PhaseSpec) error {
	planDir := filepath.Join(plansDir, planName)

	meta, err := state.ReadPlan(planDir)
	if err != nil {
		return fmt.Errorf("reading plan: %w", err)
	}

	// Check the phase doesn't already exist
	for _, p := range meta.Phases {
		if p == phaseName {
			return fmt.Errorf("phase %q already exists in plan %q", phaseName, planName)
		}
	}

	// Create the phase directory
	phaseDir := filepath.Join(planDir, "phases", phaseName)
	if err := os.MkdirAll(phaseDir, 0755); err != nil {
		return fmt.Errorf("create phase directory: %w", err)
	}

	// Write spec.yaml
	if err := WriteSpec(plansDir, planName, phaseName, spec); err != nil {
		return err
	}

	// Write state.json
	phaseState := arc.NewPhaseState(planName, phaseName, meta.WorkflowType)
	stateData, err := json.MarshalIndent(phaseState, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state.json: %w", err)
	}
	if err := os.WriteFile(filepath.Join(phaseDir, "state.json"), stateData, 0644); err != nil {
		return fmt.Errorf("write state.json: %w", err)
	}

	// Write plan.md for backward compatibility with v1 tools
	planMDContent := spec.Spec
	if planMDContent == "" {
		planMDContent = "## Objective\n\n(no spec provided)\n"
	}
	if err := os.WriteFile(filepath.Join(phaseDir, "plan.md"), []byte(planMDContent), 0644); err != nil {
		return fmt.Errorf("write plan.md: %w", err)
	}

	// Update plan.json: add phase to Phases list, set PhaseOrder, add Dependencies
	meta.Phases = append(meta.Phases, phaseName)
	if meta.PhaseOrder == nil {
		meta.PhaseOrder = make(map[string]int)
	}
	meta.PhaseOrder[phaseName] = len(meta.Phases)
	if meta.Dependencies == nil {
		meta.Dependencies = make(map[string][]string)
	}
	if len(spec.Deps) > 0 {
		meta.Dependencies[phaseName] = append([]string{}, spec.Deps...)
	}

	return state.WritePlan(planDir, meta)
}

// RemovePhase removes a pending phase from a plan.
// Deletes the phase directory and updates plan.json.
// Returns error if the phase is not in "pending" status.
func RemovePhase(plansDir, planName, phaseName string) error {
	planDir := filepath.Join(plansDir, planName)

	meta, err := state.ReadPlan(planDir)
	if err != nil {
		return fmt.Errorf("reading plan: %w", err)
	}

	// Verify phase exists in plan
	found := false
	for _, p := range meta.Phases {
		if p == phaseName {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("phase %q not found in plan %q", phaseName, planName)
	}

	// Read state.json to verify phase is "pending"
	statePath := filepath.Join(planDir, "phases", phaseName, "state.json")
	sf := state.NewStateFile(statePath)
	phaseState, err := sf.Read()
	if err != nil {
		return fmt.Errorf("reading phase state: %w", err)
	}
	if phaseState.PhaseStatus != "pending" {
		return fmt.Errorf("phase %q has status %q; only pending phases can be removed", phaseName, phaseState.PhaseStatus)
	}

	// Remove from Phases list
	newPhases := make([]string, 0, len(meta.Phases)-1)
	for _, p := range meta.Phases {
		if p != phaseName {
			newPhases = append(newPhases, p)
		}
	}
	meta.Phases = newPhases

	// Remove from PhaseOrder and rebuild
	delete(meta.PhaseOrder, phaseName)
	meta.PhaseOrder = make(map[string]int, len(meta.Phases))
	for i, p := range meta.Phases {
		meta.PhaseOrder[p] = i + 1
	}

	// Remove from Dependencies (as a key and as a dependency of others)
	delete(meta.Dependencies, phaseName)
	for phase, deps := range meta.Dependencies {
		newDeps := make([]string, 0, len(deps))
		for _, dep := range deps {
			if dep != phaseName {
				newDeps = append(newDeps, dep)
			}
		}
		if len(newDeps) == 0 {
			delete(meta.Dependencies, phase)
		} else {
			meta.Dependencies[phase] = newDeps
		}
	}

	// Delete the phase directory
	phaseDir := filepath.Join(planDir, "phases", phaseName)
	if err := os.RemoveAll(phaseDir); err != nil {
		return fmt.Errorf("remove phase directory: %w", err)
	}

	return state.WritePlan(planDir, meta)
}

// UpdateSpec updates the spec.yaml for a phase.
// Only works on phases in "pending" or "blocked" status.
func UpdateSpec(plansDir, planName, phaseName string, spec *arc.PhaseSpec) error {
	planDir := filepath.Join(plansDir, planName)
	statePath := filepath.Join(planDir, "phases", phaseName, "state.json")
	sf := state.NewStateFile(statePath)
	phaseState, err := sf.Read()
	if err != nil {
		return fmt.Errorf("reading phase state: %w", err)
	}

	switch phaseState.PhaseStatus {
	case "pending", "blocked":
		// allowed
	default:
		return fmt.Errorf("phase %q has status %q; only pending or blocked phases can have their spec updated", phaseName, phaseState.PhaseStatus)
	}

	if err := WriteSpec(plansDir, planName, phaseName, spec); err != nil {
		return err
	}

	// Also update plan.md for backward compatibility
	planMDContent := spec.Spec
	if planMDContent == "" {
		planMDContent = "## Objective\n\n(no spec provided)\n"
	}
	phaseDir := filepath.Join(planDir, "phases", phaseName)
	return os.WriteFile(filepath.Join(phaseDir, "plan.md"), []byte(planMDContent), 0644)
}

// UpdateGate updates just the gate section of a phase's spec.
// Only works on phases in "pending" or "blocked" status.
func UpdateGate(plansDir, planName, phaseName string, gate arc.GateSpec) error {
	planDir := filepath.Join(plansDir, planName)
	statePath := filepath.Join(planDir, "phases", phaseName, "state.json")
	sf := state.NewStateFile(statePath)
	phaseState, err := sf.Read()
	if err != nil {
		return fmt.Errorf("reading phase state: %w", err)
	}
	switch phaseState.PhaseStatus {
	case "pending", "blocked":
		// allowed
	default:
		return fmt.Errorf("phase %q has status %q; only pending or blocked phases can have their gate updated", phaseName, phaseState.PhaseStatus)
	}

	spec, err := ReadSpec(plansDir, planName, phaseName)
	if err != nil {
		return err
	}
	spec.Gate = gate
	return WriteSpec(plansDir, planName, phaseName, spec)
}

// UpdateDeps updates the dependency edges for a phase in plan.json.
// Only works on phases in "pending" status.
func UpdateDeps(plansDir, planName, phaseName string, deps []string) error {
	planDir := filepath.Join(plansDir, planName)

	// Check the phase status before modifying plan.json.
	statePath := filepath.Join(planDir, "phases", phaseName, "state.json")
	sf := state.NewStateFile(statePath)
	phaseState, err := sf.Read()
	if err != nil {
		return fmt.Errorf("reading phase state: %w", err)
	}
	if phaseState.PhaseStatus != "pending" {
		return fmt.Errorf("phase %q has status %q; only pending phases can have their dependencies updated", phaseName, phaseState.PhaseStatus)
	}

	meta, err := state.ReadPlan(planDir)
	if err != nil {
		return fmt.Errorf("reading plan: %w", err)
	}

	// Verify phase exists
	found := false
	for _, p := range meta.Phases {
		if p == phaseName {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("phase %q not found in plan %q", phaseName, planName)
	}

	if meta.Dependencies == nil {
		meta.Dependencies = make(map[string][]string)
	}
	if len(deps) == 0 {
		delete(meta.Dependencies, phaseName)
	} else {
		meta.Dependencies[phaseName] = append([]string{}, deps...)
	}

	return state.WritePlan(planDir, meta)
}
