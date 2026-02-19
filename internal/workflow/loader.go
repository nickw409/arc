package workflow

import (
	"fmt"
	"os"

	"github.com/nwiley/arc/internal/arc"
	"gopkg.in/yaml.v3"
)

// rawState is a helper for YAML parsing that uses yaml.Node for the "next" field.
type rawState struct {
	Name        string              `yaml:"name"`
	Description string              `yaml:"description"`
	Prompt      string              `yaml:"prompt"`
	Verdicts    []string            `yaml:"verdicts"`
	Next        *arc.Transition      `yaml:"next"`
	Constraints *arc.ConstraintConfig `yaml:"constraints"`
	Escalation  []arc.EscalationRule  `yaml:"escalation"`
	After       []arc.HookConfig      `yaml:"after"`
	Parallel    *arc.ParallelConfig   `yaml:"parallel"`
}

// rawWorkflow is a helper for top-level YAML parsing.
type rawWorkflow struct {
	Name                 string                    `yaml:"name"`
	Version              int                       `yaml:"version"`
	Description          string                    `yaml:"description"`
	EntryState           string                    `yaml:"entry_state"`
	TerminalStates       []string                  `yaml:"terminal_states"`
	InterventionTriggers []arc.InterventionTrigger `yaml:"intervention_triggers"`
	States               []rawState                `yaml:"states"`
}

// Load reads a workflow YAML file and returns a validated, normalized Workflow.
func Load(path string) (*arc.Workflow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading workflow %s: %w", path, err)
	}
	return LoadBytes(data)
}

// LoadBytes loads a workflow from raw YAML bytes.
func LoadBytes(data []byte) (*arc.Workflow, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty workflow data")
	}

	var raw rawWorkflow
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing workflow YAML: %w", err)
	}

	w := &arc.Workflow{
		Name:                 raw.Name,
		Version:              raw.Version,
		Description:          raw.Description,
		EntryState:           raw.EntryState,
		TerminalStates:       raw.TerminalStates,
		InterventionTriggers: raw.InterventionTriggers,
	}

	states := make([]arc.StateConfig, len(raw.States))
	for i, rs := range raw.States {
		sc := arc.StateConfig{
			Name:        rs.Name,
			Description: rs.Description,
			Prompt:      rs.Prompt,
			Verdicts:    rs.Verdicts,
			Constraints: rs.Constraints,
			Escalation:  rs.Escalation,
			After:       rs.After,
			Parallel:    rs.Parallel,
		}

		if rs.Next != nil {
			sc.Transition = *rs.Next
		}
		// Terminal states (no next field) get Transition{Branches: nil}

		states[i] = sc
	}
	w.States = states

	errs := Validate(w)
	if len(errs) > 0 {
		return nil, errs[0]
	}

	return w, nil
}
