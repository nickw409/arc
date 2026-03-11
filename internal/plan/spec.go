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

// SpecWarning is an issue detected during spec validation.
// Fatal warnings indicate structural errors that will cause runtime failures;
// non-fatal warnings are advisory.
type SpecWarning struct {
	Field   string
	Message string
	Fatal   bool
}

func (w SpecWarning) String() string {
	if w.Fatal {
		return fmt.Sprintf("%s: %s [fatal]", w.Field, w.Message)
	}
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

	// Gate assertions: each must have at least one recognized field set.
	// Assertions without a recognized field silently hit "unknown assertion type"
	// at runtime, causing the phase to block after exhausting retries.
	for i, a := range spec.Gate.Assertions {
		if isEmptyAssertion(a) {
			warnings = append(warnings, SpecWarning{
				Field:   "gate.assertions",
				Message: fmt.Sprintf("assertion %d has no recognized field — use 'grep:', 'file_exists:', 'test_exists:', 'build_passes:', or 'no_untracked:' (or legacy type+target)", i+1),
				Fatal:   true,
			})
		}
	}

	// Promises: each must have exactly one recognized field set.
	for i, p := range spec.Promises {
		fields := 0
		if strings.TrimSpace(p.FuncExists) != "" {
			fields++
		}
		if strings.TrimSpace(p.TestExists) != "" {
			fields++
		}
		if strings.TrimSpace(p.FileExists) != "" {
			fields++
		}
		if strings.TrimSpace(p.TestCovers) != "" {
			fields++
			if strings.TrimSpace(p.Test) == "" {
				warnings = append(warnings, SpecWarning{
					Field:   "promises",
					Message: fmt.Sprintf("promise %d has test_covers but no test — test field is required with test_covers", i+1),
					Fatal:   true,
				})
			}
		}
		if fields == 0 {
			warnings = append(warnings, SpecWarning{
				Field:   "promises",
				Message: fmt.Sprintf("promise %d has no recognized field — use func_exists, test_exists, file_exists, or test_covers", i+1),
				Fatal:   true,
			})
		} else if fields > 1 {
			warnings = append(warnings, SpecWarning{
				Field:   "promises",
				Message: fmt.Sprintf("promise %d has multiple promise types set — exactly one of func_exists, test_exists, file_exists, or test_covers must be set", i+1),
				Fatal:   true,
			})
		}
	}

	return warnings
}

// isEmptyAssertion returns true if a GateAssertion has no recognized field set
// and will therefore always produce "unknown assertion type or missing target".
func isEmptyAssertion(a arc.GateAssertion) bool {
	return a.Grep == "" &&
		a.FileExists == "" &&
		a.TestExists == "" &&
		a.BuildPasses == "" &&
		a.NoUntracked == "" &&
		a.FileAbsent == "" &&
		a.GrepNot == "" &&
		a.NoModified == "" &&
		a.FilesOnly == "" &&
		!(a.Type != "" && a.Target != "")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// ExtractSpecFromPlanMD parses the ```yaml block under the "## Spec" heading
// in plan.md and returns the PhaseSpec it contains.
// Returns false if no spec block is found, if parsing fails, or if the spec
// field is empty (indicating an unfilled template).
func ExtractSpecFromPlanMD(planMD string) (*arc.PhaseSpec, bool) {
	lines := strings.Split(planMD, "\n")

	inSpecSection := false
	inYAMLBlock := false
	var yamlLines []string

	for _, line := range lines {
		if strings.HasPrefix(line, "## Spec") {
			inSpecSection = true
			continue
		}
		// Another ## heading ends the spec section
		if inSpecSection && strings.HasPrefix(line, "## ") {
			break
		}
		if inSpecSection && !inYAMLBlock {
			if strings.TrimSpace(line) == "```yaml" {
				inYAMLBlock = true
				continue
			}
		}
		if inYAMLBlock {
			if strings.TrimSpace(line) == "```" {
				break
			}
			yamlLines = append(yamlLines, line)
		}
	}

	if len(yamlLines) == 0 {
		return nil, false
	}

	var spec arc.PhaseSpec
	if err := yaml.Unmarshal([]byte(strings.Join(yamlLines, "\n")), &spec); err != nil {
		return nil, false
	}
	if strings.TrimSpace(spec.Spec) == "" {
		return nil, false
	}
	return &spec, true
}

// SyncSpecFromPlanMD is a no-op stub retained for backward compatibility.
// Plan.md is now the authoritative spec source; no sync to spec.yaml is needed.
func SyncSpecFromPlanMD(_, _, _ string) (bool, error) {
	return false, nil
}

// ReplaceSpecInPlanMD finds the ```yaml block under the ## Spec heading in planMD
// and replaces it with the marshaled spec. Returns the updated planMD string.
// Returns an error if the ## Spec heading or a ```yaml block after it are not found.
func ReplaceSpecInPlanMD(planMD string, spec *arc.PhaseSpec) (string, error) {
	data, err := yaml.Marshal(spec)
	if err != nil {
		return "", fmt.Errorf("marshal spec: %w", err)
	}
	lines := strings.Split(planMD, "\n")

	specIdx := -1
	for i, l := range lines {
		if strings.HasPrefix(l, "## Spec") {
			specIdx = i
			break
		}
	}
	if specIdx < 0 {
		return "", fmt.Errorf("## Spec heading not found in plan.md")
	}

	openIdx := -1
	for i := specIdx + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") {
			break // another heading — stop looking
		}
		if strings.TrimSpace(lines[i]) == "```yaml" {
			openIdx = i
			break
		}
	}
	if openIdx < 0 {
		return "", fmt.Errorf("```yaml block not found after ## Spec in plan.md")
	}

	closeIdx := -1
	for i := openIdx + 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "```" {
			closeIdx = i
			break
		}
	}
	if closeIdx < 0 {
		return "", fmt.Errorf("closing ``` not found after yaml block in plan.md")
	}

	newBlock := []string{"```yaml"}
	newBlock = append(newBlock, strings.Split(strings.TrimRight(string(data), "\n"), "\n")...)
	newBlock = append(newBlock, "```")

	result := make([]string, 0, len(lines))
	result = append(result, lines[:openIdx]...)
	result = append(result, newBlock...)
	result = append(result, lines[closeIdx+1:]...)
	return strings.Join(result, "\n"), nil
}

// WriteSpec writes a PhaseSpec to the phase directory.
// It updates the ## Spec YAML block in plan.md (so ReadSpec can read it back),
// and also writes spec.yaml for backward compatibility with external callers.
// The plan.md update is skipped when the spec has no meaningful content
// (empty spec field) to avoid polluting template or custom plan.md files.
func WriteSpec(plansDir, planName, phaseName string, spec *arc.PhaseSpec) error {
	data, err := yaml.Marshal(spec)
	if err != nil {
		return fmt.Errorf("marshal spec: %w", err)
	}
	yamlContent := string(data)

	// Write spec.yaml for backward compatibility.
	specFilePath := filepath.Join(plansDir, planName, "phases", phaseName, "spec.yaml")
	if err := os.WriteFile(specFilePath, []byte(yamlContent), 0644); err != nil {
		return fmt.Errorf("write spec.yaml: %w", err)
	}

	// Only update plan.md when the spec has real content.
	if strings.TrimSpace(spec.Spec) == "" {
		return nil
	}

	// Update the ## Spec block in plan.md so ReadSpec can read from it.
	phaseDir := filepath.Join(plansDir, planName, "phases", phaseName)
	planMDPath := filepath.Join(phaseDir, "plan.md")
	planMDBytes, err := os.ReadFile(planMDPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read plan.md: %w", err)
	}
	updated := embedSpecInPlanMD(string(planMDBytes), yamlContent)
	if err := os.WriteFile(planMDPath, []byte(updated), 0644); err != nil {
		return fmt.Errorf("write plan.md: %w", err)
	}
	return nil
}

// embedSpecInPlanMD replaces the ```yaml block under "## Spec" in plan.md with
// the provided YAML content, or appends a new ## Spec section if none exists.
func embedSpecInPlanMD(planMD, yamlContent string) string {
	lines := strings.Split(planMD, "\n")

	inSpecSection := false
	inYAMLBlock := false
	specSectionStart := -1
	yamlStart := -1
	yamlEnd := -1

	for i, line := range lines {
		if strings.HasPrefix(line, "## Spec") {
			inSpecSection = true
			specSectionStart = i
			continue
		}
		if inSpecSection && strings.HasPrefix(line, "## ") {
			break
		}
		if inSpecSection && !inYAMLBlock && strings.TrimSpace(line) == "```yaml" {
			inYAMLBlock = true
			yamlStart = i
			continue
		}
		if inYAMLBlock && strings.TrimSpace(line) == "```" {
			yamlEnd = i
			break
		}
	}

	newBlock := "```yaml\n" + strings.TrimRight(yamlContent, "\n") + "\n```"

	if yamlStart >= 0 && yamlEnd >= 0 {
		// Replace existing block content.
		before := strings.Join(lines[:yamlStart], "\n")
		after := strings.Join(lines[yamlEnd+1:], "\n")
		if after != "" {
			return before + "\n" + newBlock + "\n" + after
		}
		return before + "\n" + newBlock
	}

	if specSectionStart >= 0 {
		// ## Spec section exists but no yaml block — insert one after the heading.
		before := strings.Join(lines[:specSectionStart+1], "\n")
		after := strings.Join(lines[specSectionStart+1:], "\n")
		result := before + "\n\n" + newBlock
		if strings.TrimSpace(after) != "" {
			result += "\n\n" + after
		}
		return result
	}

	// No ## Spec section — append one.
	trimmed := strings.TrimRight(planMD, "\n")
	return trimmed + "\n\n## Spec\n\n" + newBlock + "\n"
}

// ReadSpec reads the PhaseSpec for a phase from its plan.md file.
// It extracts the embedded ## Spec YAML block from plan.md.
// Returns an error if plan.md is missing or contains no parseable spec block.
func ReadSpec(plansDir, planName, phaseName string) (*arc.PhaseSpec, error) {
	phaseDir := filepath.Join(plansDir, planName, "phases", phaseName)
	data, err := os.ReadFile(filepath.Join(phaseDir, "plan.md"))
	if err != nil {
		return nil, fmt.Errorf("read plan.md for phase %s: %w", phaseName, err)
	}
	spec, ok := ExtractSpecFromPlanMD(string(data))
	if !ok {
		return nil, fmt.Errorf("no parseable spec block in plan.md for phase %s", phaseName)
	}
	return spec, nil
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

	// Write plan.md with embedded ## Spec YAML block so ReadSpec can find the spec.
	spec.Name = phaseName
	specData, err := yaml.Marshal(spec)
	if err != nil {
		return fmt.Errorf("marshal spec: %w", err)
	}
	planMDContent := "# Phase: " + phaseName + "\n\n## Objective\n\n" + strings.TrimSpace(spec.Spec) + "\n\n## Spec\n\n```yaml\n" + string(specData) + "```\n"
	if err := os.WriteFile(filepath.Join(phaseDir, "plan.md"), []byte(planMDContent), 0644); err != nil {
		return fmt.Errorf("write plan.md: %w", err)
	}

	// Write spec.yaml for backward compatibility with external tooling.
	if err := os.WriteFile(filepath.Join(phaseDir, "spec.yaml"), specData, 0644); err != nil {
		return fmt.Errorf("write spec.yaml: %w", err)
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

	// Read plan.md and extract existing spec (fallback to empty on error).
	phaseDir := filepath.Join(plansDir, planName, "phases", phaseName)
	planMDPath := filepath.Join(phaseDir, "plan.md")
	planMDBytes, err := os.ReadFile(planMDPath)
	if err != nil {
		return fmt.Errorf("read plan.md: %w", err)
	}
	existing, _ := ExtractSpecFromPlanMD(string(planMDBytes))
	if existing == nil {
		existing = &arc.PhaseSpec{}
	}

	// Merge non-zero fields from update into existing.
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

	// Write updated spec back into plan.md.
	updated, err := ReplaceSpecInPlanMD(string(planMDBytes), existing)
	if err != nil {
		// No ## Spec block yet — embed one.
		data, merr := yaml.Marshal(existing)
		if merr != nil {
			return fmt.Errorf("marshal spec: %w", merr)
		}
		updated = embedSpecInPlanMD(string(planMDBytes), string(data))
	}
	return os.WriteFile(planMDPath, []byte(updated), 0644)
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

	// Read plan.md and extract existing spec.
	phaseDir := filepath.Join(plansDir, planName, "phases", phaseName)
	planMDPath := filepath.Join(phaseDir, "plan.md")
	planMDBytes, err := os.ReadFile(planMDPath)
	if err != nil {
		return fmt.Errorf("read plan.md: %w", err)
	}
	spec, _ := ExtractSpecFromPlanMD(string(planMDBytes))
	if spec == nil {
		spec = &arc.PhaseSpec{}
	}
	spec.Gate = gate

	// Write updated spec back into plan.md.
	updated, err := ReplaceSpecInPlanMD(string(planMDBytes), spec)
	if err != nil {
		data, merr := yaml.Marshal(spec)
		if merr != nil {
			return fmt.Errorf("marshal spec: %w", merr)
		}
		updated = embedSpecInPlanMD(string(planMDBytes), string(data))
	}
	return os.WriteFile(planMDPath, []byte(updated), 0644)
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
