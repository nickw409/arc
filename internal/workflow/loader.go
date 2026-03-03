package workflow

import (
	"fmt"
	"os"

	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/block"
	"github.com/nwiley/arc/internal/resources"
	"gopkg.in/yaml.v3"
)

// rawState is a helper for YAML parsing that uses yaml.Node for the "next" field.
type rawState struct {
	Name        string                `yaml:"name"`
	Description string                `yaml:"description"`
	Prompt      string                `yaml:"prompt"`
	Verdicts    []string              `yaml:"verdicts"`
	Next        *arc.Transition       `yaml:"next"`
	Constraints *arc.ConstraintConfig `yaml:"constraints"`
	Escalation  []arc.EscalationRule  `yaml:"escalation"`
	After       []arc.HookConfig      `yaml:"after"`
	Parallel    *arc.ParallelConfig   `yaml:"parallel"`
	Agent       *arc.AgentConfig      `yaml:"agent"`
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
	Pipeline             []block.PipelineStep      `yaml:"pipeline"`
}

// Load reads a workflow YAML file and returns a validated, normalized Workflow.
func Load(path string) (*arc.Workflow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading workflow %s: %w", path, err)
	}
	return LoadBytes(data)
}

// LoadBytesWithBlockLoader loads a workflow from raw YAML bytes, using blockLoader to resolve blocks.
// If the YAML contains a "pipeline" key, uses block composition with the provided loader.
// Otherwise loads as a traditional state-machine workflow (blockLoader is unused).
func LoadBytesWithBlockLoader(data []byte, blockLoader func(string) ([]byte, error)) (*arc.Workflow, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty workflow data")
	}

	var raw rawWorkflow
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing workflow YAML: %w", err)
	}

	// Pipeline format: compose from blocks
	if len(raw.Pipeline) > 0 {
		return loadComposed(raw, blockLoader)
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
			Agent:       rs.Agent,
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

// LoadBytes loads a workflow from raw YAML bytes using the embedded block loader.
// Calls LoadBytesWithBlockLoader(data, resources.BlockBytes).
func LoadBytes(data []byte) (*arc.Workflow, error) {
	return LoadBytesWithBlockLoader(data, resources.BlockBytes)
}

// loadComposed resolves a pipeline-format workflow by loading blocks and composing them.
func loadComposed(raw rawWorkflow, blockLoader func(string) ([]byte, error)) (*arc.Workflow, error) {
	// Load all referenced blocks
	blockDefs := make(map[string]*block.Block)
	for _, step := range raw.Pipeline {
		if step.Block != "" {
			if _, ok := blockDefs[step.Block]; !ok {
				b, err := loadBlockDef(step.Block, blockLoader)
				if err != nil {
					return nil, fmt.Errorf("loading block %q: %w", step.Block, err)
				}
				blockDefs[step.Block] = b
			}
		}
		if step.Parallel != nil {
			for _, pbr := range step.Parallel.Blocks {
				if _, ok := blockDefs[pbr.Block]; !ok {
					b, err := loadBlockDef(pbr.Block, blockLoader)
					if err != nil {
						return nil, fmt.Errorf("loading parallel block %q: %w", pbr.Block, err)
					}
					blockDefs[pbr.Block] = b
				}
			}
		}
	}

	wf, parallelGroups, err := block.ComposePipeline(raw.Pipeline, blockDefs)
	if err != nil {
		return nil, fmt.Errorf("composing pipeline: %w", err)
	}

	wf.Name = raw.Name
	wf.Description = raw.Description
	wf.InterventionTriggers = raw.InterventionTriggers

	// Override terminal states if specified in the workflow
	if len(raw.TerminalStates) > 0 {
		wf.TerminalStates = raw.TerminalStates
	}

	// Store parallel groups for orchestrator runtime
	if len(parallelGroups) > 0 {
		wf.ParallelGroups = make([]arc.ParallelGroup, len(parallelGroups))
		for i, pg := range parallelGroups {
			wf.ParallelGroups[i] = arc.ParallelGroup{
				ForkState: pg.ForkState,
				JoinState: pg.JoinState,
				Strategy:  pg.Strategy,
			}
			for _, rb := range pg.Blocks {
				wf.ParallelGroups[i].Blocks = append(wf.ParallelGroups[i].Blocks, arc.ParallelBlockInstance{
					Name:   rb.Name,
					Params: rb.Params,
				})
			}
		}
	}

	// Validate the composed workflow
	errs := block.ValidateComposition(wf, nil)
	if len(errs) > 0 {
		return nil, errs[0]
	}

	return wf, nil
}

// loadBlockDef loads a block definition using the provided loader function.
func loadBlockDef(name string, blockLoader func(string) ([]byte, error)) (*block.Block, error) {
	if blockLoader == nil {
		return nil, fmt.Errorf("no block loader provided (cannot load block %q)", name)
	}
	data, err := blockLoader(name)
	if err != nil {
		return nil, err
	}
	return block.LoadBlock(data)
}
