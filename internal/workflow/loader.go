package workflow

import "github.com/nwiley/arc/internal/arc"

// Load reads a workflow YAML file and returns a validated, normalized Workflow.
func Load(path string) (*arc.Workflow, error) {
	panic("not implemented")
}

// LoadBytes loads a workflow from raw YAML bytes.
func LoadBytes(data []byte) (*arc.Workflow, error) {
	panic("not implemented")
}
