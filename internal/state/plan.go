package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nwiley/arc/internal/arc"
)

// ReadPlan reads plan.json from the given plan directory.
func ReadPlan(planDir string) (*arc.PlanMeta, error) {
	path := filepath.Join(planDir, "plan.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading plan file %s: %w", path, err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("plan file %s is empty", path)
	}
	var meta arc.PlanMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("parsing plan file %s: %w", path, err)
	}
	return &meta, nil
}

// WritePlan atomically writes plan.json to the given plan directory.
func WritePlan(planDir string, meta *arc.PlanMeta) error {
	path := filepath.Join(planDir, "plan.json")
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling plan: %w", err)
	}
	tmp, err := os.CreateTemp(planDir, "plan.json.*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("closing temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming temp file: %w", err)
	}
	return nil
}

// NextPhase returns the next incomplete phase in dependency order, or "" if all complete.
func NextPhase(meta *arc.PlanMeta, phaseStates map[string]*arc.PhaseState) string {
	for _, phase := range meta.Phases {
		if isPhaseReady(phase, meta, phaseStates) {
			return phase
		}
	}
	return ""
}

// PhasesReady returns all phases whose dependencies are all complete.
func PhasesReady(meta *arc.PlanMeta, phaseStates map[string]*arc.PhaseState) []string {
	var ready []string
	for _, phase := range meta.Phases {
		if isPhaseReady(phase, meta, phaseStates) {
			ready = append(ready, phase)
		}
	}
	return ready
}

func isPhaseReady(phase string, meta *arc.PlanMeta, phaseStates map[string]*arc.PhaseState) bool {
	ps, ok := phaseStates[phase]
	if !ok || ps == nil {
		return false
	}
	if ps.PhaseStatus == "complete" || ps.PhaseStatus == "blocked" {
		return false
	}
	for _, dep := range meta.Dependencies[phase] {
		depState, ok := phaseStates[dep]
		if !ok || depState == nil || depState.PhaseStatus != "complete" {
			return false
		}
	}
	return true
}
