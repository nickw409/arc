package arc

// PhaseSpec is the structured specification for a v2 phase.
// Stored as spec.yaml in the phase directory.
type PhaseSpec struct {
	Name        string       `json:"name" yaml:"name"`
	Spec        string       `json:"spec" yaml:"spec"`
	Files       []string     `json:"files,omitempty" yaml:"files,omitempty"`
	Test        string       `json:"test,omitempty" yaml:"test,omitempty"`
	Deps        []string     `json:"deps,omitempty" yaml:"deps,omitempty"`
	Complexity  string       `json:"complexity,omitempty" yaml:"complexity,omitempty"` // simple, medium, complex
	Checkpoints []Checkpoint `json:"checkpoints,omitempty" yaml:"checkpoints,omitempty"`
	Gate        GateSpec     `json:"gate,omitempty" yaml:"gate,omitempty"`
}

// Checkpoint defines an ordered verification point within a phase.
type Checkpoint struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description" yaml:"description"`
	Test        string `json:"test" yaml:"test"` // command to verify this checkpoint
}

// GateSpec defines the verification gate for a phase.
type GateSpec struct {
	Assertions    []GateAssertion `json:"assertions,omitempty" yaml:"assertions,omitempty"`
	VerifierAgent bool            `json:"verifier_agent,omitempty" yaml:"verifier_agent,omitempty"`
}

// DefaultTurnBudget returns the default max turns for a given complexity tier.
func DefaultTurnBudget(complexity string) int {
	switch complexity {
	case "simple":
		return 50
	case "complex":
		return 200
	default: // medium
		return 100
	}
}
