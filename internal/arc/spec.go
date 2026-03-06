package arc

// GateSpec defines the verification gate for a phase.
type GateSpec struct {
	Assertions    []GateAssertion `json:"assertions,omitempty" yaml:"assertions,omitempty"`
	VerifierAgent bool            `json:"verifier_agent,omitempty" yaml:"verifier_agent,omitempty"`
}

// Checkpoint is a named milestone within a phase with an optional test command.
type Checkpoint struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description" yaml:"description"`
	Test        string `json:"test,omitempty" yaml:"test,omitempty"`
}

// PhaseSpec is the structured phase specification parsed from spec.yaml.
type PhaseSpec struct {
	Name        string       `json:"name" yaml:"name"`
	Spec        string       `json:"spec,omitempty" yaml:"spec,omitempty"`
	Description string       `json:"description,omitempty" yaml:"description,omitempty"`
	Role string `json:"role,omitempty" yaml:"role,omitempty"`
	Files       []string     `json:"files,omitempty" yaml:"files,omitempty"`
	Verify      string       `json:"verify,omitempty" yaml:"verify,omitempty"`
	Deps        []string     `json:"deps,omitempty" yaml:"deps,omitempty"`
	Complexity  string       `json:"complexity,omitempty" yaml:"complexity,omitempty"`
	Checkpoints []Checkpoint `json:"checkpoints,omitempty" yaml:"checkpoints,omitempty"`
	Gate        GateSpec     `json:"gate,omitempty" yaml:"gate,omitempty"`
}

// DefaultRole returns the given role if valid, or "impl" as the default.
func DefaultRole(role string) string {
	switch role {
	case "impl", "review", "investigate", "audit":
		return role
	default:
		return "impl"
	}
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
