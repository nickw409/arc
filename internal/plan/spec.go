package plan

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/state"
	"gopkg.in/yaml.v3"
)

// numberedListRe matches lines that start with a number followed by a dot/paren,
// which indicates someone wrote a human-readable list instead of a shell command.
var numberedListRe = regexp.MustCompile(`(?m)^\s*\d+[\.\)]\s`)

// SpecWarning is a non-fatal issue detected during spec validation.
type SpecWarning struct {
	Field   string
	Message string
}

func (w SpecWarning) String() string {
	return fmt.Sprintf("%s: %s", w.Field, w.Message)
}

// ValidateSpec checks a PhaseSpec for common mistakes and returns warnings.
// It does not return errors — all issues are advisory so the caller can decide.
func ValidateSpec(spec *arc.PhaseSpec) []SpecWarning {
	var warnings []SpecWarning

	// Spec content: must have something for the agent and gate to work with.
	if strings.TrimSpace(spec.Spec) == "" && strings.TrimSpace(spec.Verify) == "" {
		warnings = append(warnings, SpecWarning{
			Field:   "spec",
			Message: "empty — phase has no spec or verify content; the gate will reject this as misconfigured",
		})
	}

	// Test field: should be a shell command, not a human-readable description.
	if spec.Verify != "" {
		if numberedListRe.MatchString(spec.Verify) {
			warnings = append(warnings, SpecWarning{
				Field:   "verify",
				Message: "looks like a numbered list, not a shell command — the gate executes this as 'sh -c <test>'",
			})
		}
		// Check for lines that are clearly not commands (contain ":" followed by prose)
		for _, line := range strings.Split(spec.Verify, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			// Lines starting with a capital letter and containing spaces but no path separators
			// are likely descriptions, not commands
			if len(line) > 0 && line[0] >= 'A' && line[0] <= 'Z' && !strings.ContainsAny(line, "/\\$|&;") {
				warnings = append(warnings, SpecWarning{
					Field:   "verify",
					Message: fmt.Sprintf("line %q looks like a description, not a shell command", truncate(line, 60)),
				})
				break // one warning is enough
			}
		}
	}

	// Role: if set, must be one of the allowed values.
	if spec.Role != "" {
		switch spec.Role {
		case "impl", "review", "investigate", "audit":
			// valid
		default:
			warnings = append(warnings, SpecWarning{
				Field:   "role",
				Message: fmt.Sprintf("invalid role %q — must be impl, review, investigate, or audit", spec.Role),
			})
		}
	}

	// Complexity: should be set.
	if spec.Complexity == "" {
		warnings = append(warnings, SpecWarning{
			Field:   "complexity",
			Message: "not set — verifier agent auto-detection requires complexity (simple/medium/complex)",
		})
	}

	// Files listed but no file_exists gate assertions.
	if len(spec.Files) > 0 && len(spec.Gate.Assertions) == 0 {
		warnings = append(warnings, SpecWarning{
			Field:   "gate",
			Message: fmt.Sprintf("%d files listed but no gate assertions — consider adding file_exists assertions", len(spec.Files)),
		})
	}

	return warnings
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

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

// UpdateSpec merges the non-zero fields of update into the existing spec.yaml for a phase.
// Only works on phases in "pending" or "blocked" status.
func UpdateSpec(plansDir, planName, phaseName string, update *arc.PhaseSpec) error {
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

	// Read existing spec and merge — only overwrite fields that are non-zero in update.
	existing, err := ReadSpec(plansDir, planName, phaseName)
	if err != nil {
		existing = &arc.PhaseSpec{}
	}
	if update.Spec != "" {
		existing.Spec = update.Spec
	}
	if update.Role != "" {
		existing.Role = update.Role
	}
	if update.Verify != "" {
		existing.Verify = update.Verify
	}
	if update.Complexity != "" {
		existing.Complexity = update.Complexity
	}
	if len(update.Files) > 0 {
		existing.Files = update.Files
	}
	if len(update.Deps) > 0 {
		existing.Deps = update.Deps
	}
	if len(update.Checkpoints) > 0 {
		existing.Checkpoints = update.Checkpoints
	}
	// Gate is only updated via UpdateGate — not merged here.

	if err := WriteSpec(plansDir, planName, phaseName, existing); err != nil {
		return err
	}

	// Also update plan.md for backward compatibility
	planMDContent := existing.Spec
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
