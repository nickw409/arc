package arc

// Workflow is the canonical internal representation of a workflow definition.
type Workflow struct {
	Name                 string                `yaml:"name"`
	Version              int                   `yaml:"version"`
	Description          string                `yaml:"description"`
	States               []StateConfig         `yaml:"-"`
	EntryState           string                `yaml:"entry_state"`
	TerminalStates       []string              `yaml:"terminal_states"`
	InterventionTriggers []InterventionTrigger `yaml:"intervention_triggers"`
	ParallelGroups       []ParallelGroup       `yaml:"-"` // populated by block composition
}

// ParallelGroup describes a set of blocks to run concurrently at a fork point.
type ParallelGroup struct {
	ForkState string                  // synthetic state triggering parallel execution
	JoinState string                  // synthetic state after parallel completes
	Strategy  string                  // "all" or "any"
	Blocks    []ParallelBlockInstance // blocks to run in parallel
}

// ParallelBlockInstance is a named block reference within a parallel group.
type ParallelBlockInstance struct {
	Name   string
	Params map[string]string
}

// AgentConfig configures per-state agent behavior. Nil means use defaults.
type AgentConfig struct {
	MaxTurns     int      `yaml:"max_turns"`
	AllowedTools []string `yaml:"allowed_tools"`
	Timeout      int      `yaml:"timeout"`
	Model        string   `yaml:"model"`
}

// StateConfig is the canonical internal representation of a workflow state.
type StateConfig struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	Prompt      string            `yaml:"prompt"`
	Verdicts    []string          `yaml:"verdicts"`
	Transition  Transition
	Constraints *ConstraintConfig `yaml:"constraints"`
	Escalation  []EscalationRule  `yaml:"escalation"`
	After       []HookConfig      `yaml:"after"`
	Parallel    *ParallelConfig   `yaml:"parallel"`
	Agent       *AgentConfig      `yaml:"agent"`
}

// Transition is the canonical transition format.
type Transition struct {
	Branches map[Verdict]string
}

type ConstraintConfig struct {
	MaxIterations       int      `yaml:"max_iterations"`
	RequireArtifactsIn  []string `yaml:"require_artifacts_in"`
	RequireArtifactsOut []string `yaml:"require_artifacts_out"`
}

type EscalationRule struct {
	AtIteration      *int              `yaml:"at_iteration"`
	AfterIteration   *int              `yaml:"after_iteration"`
	EveryNIterations *int              `yaml:"every_n_iterations"`
	Action           string            `yaml:"action"`
	Params           map[string]string `yaml:"params"`
}

type HookConfig struct {
	Action string            `yaml:"action"`
	When   string            `yaml:"when"`
	Params map[string]string `yaml:"params"`
}

type ParallelConfig struct {
	Branches []ParallelBranch `yaml:"branches"`
	Strategy string           `yaml:"strategy"`
	N        int              `yaml:"n"`
}

type ParallelBranch struct {
	Name   string `yaml:"name"`
	Prompt string `yaml:"prompt"`
}

type InterventionTrigger struct {
	Condition string `yaml:"condition"`
	Message   string `yaml:"message"`
}
