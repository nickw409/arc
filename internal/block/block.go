package block

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/nwiley/arc/internal/arc"
	"gopkg.in/yaml.v3"
)

// Block is a reusable, parameterized group of workflow states.
type Block struct {
	Name        string              `yaml:"name"`
	Description string              `yaml:"description"`
	Params      map[string]ParamDef `yaml:"params"`
	Entry       string              `yaml:"entry"`
	Exits       []string            `yaml:"exits"`
	States      []BlockState        `yaml:"states"`
}

// ParamDef defines a block parameter with an optional default value.
type ParamDef struct {
	Default string `yaml:"default"`
}

// AgentConfigRaw holds agent config as strings for parameter substitution.
type AgentConfigRaw struct {
	MaxTurns     string   `yaml:"max_turns"`
	AllowedTools []string `yaml:"allowed_tools"`
	Timeout      string   `yaml:"timeout"`
	Model        string   `yaml:"model"`
}

// ToAgentConfig converts raw string values to a typed AgentConfig.
func (r *AgentConfigRaw) ToAgentConfig() *arc.AgentConfig {
	if r == nil {
		return nil
	}
	return &arc.AgentConfig{
		MaxTurns:     parseInt(r.MaxTurns),
		AllowedTools: r.AllowedTools,
		Timeout:      parseInt(r.Timeout),
		Model:        r.Model,
	}
}

// BlockState is a state definition within a block, using string-typed fields
// for parameter substitution before conversion to arc.StateConfig.
type BlockState struct {
	Name        string           `yaml:"name"`
	Description string           `yaml:"description"`
	Prompt      string           `yaml:"prompt"`
	Verdicts    []string         `yaml:"verdicts"`
	Next        map[string]string // parsed from YAML (string → linear, map → branching)
	Agent       *AgentConfigRaw  `yaml:"agent"`
	Constraints *ConstraintRaw   `yaml:"constraints"`
}

// rawBlockState is for YAML parsing, where "next" can be a string or a map.
type rawBlockState struct {
	Name        string          `yaml:"name"`
	Description string          `yaml:"description"`
	Prompt      string          `yaml:"prompt"`
	Verdicts    []string        `yaml:"verdicts"`
	Next        yaml.Node       `yaml:"next"`
	Agent       *AgentConfigRaw `yaml:"agent"`
	Constraints *ConstraintRaw  `yaml:"constraints"`
}

// rawBlock mirrors Block but uses rawBlockState for YAML parsing.
type rawBlock struct {
	Name        string              `yaml:"name"`
	Description string              `yaml:"description"`
	Params      map[string]ParamDef `yaml:"params"`
	Entry       string              `yaml:"entry"`
	Exits       []string            `yaml:"exits"`
	States      []rawBlockState     `yaml:"states"`
}

// ConstraintRaw holds constraint values as strings so they can contain ${param} references.
type ConstraintRaw struct {
	MaxIterations string `yaml:"max_iterations"`
}

// ResolvedBlock is a block with parameters applied and a namespace prefix.
type ResolvedBlock struct {
	Name   string // instance name (may differ from block name for parallel)
	Block  *Block
	Params map[string]string
}

// paramRe matches ${param_name} placeholders.
var paramRe = regexp.MustCompile(`\$\{(\w+)\}`)

// LoadBlock parses a block definition from YAML bytes.
func LoadBlock(data []byte) (*Block, error) {
	var raw rawBlock
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing block YAML: %w", err)
	}
	if raw.Name == "" {
		return nil, fmt.Errorf("block has no name")
	}
	if raw.Entry == "" {
		return nil, fmt.Errorf("block %q has no entry state", raw.Name)
	}

	b := &Block{
		Name:        raw.Name,
		Description: raw.Description,
		Params:      raw.Params,
		Entry:       raw.Entry,
		Exits:       raw.Exits,
	}

	b.States = make([]BlockState, len(raw.States))
	for i, rs := range raw.States {
		bs := BlockState{
			Name:        rs.Name,
			Description: rs.Description,
			Prompt:      rs.Prompt,
			Verdicts:    rs.Verdicts,
			Agent:       rs.Agent,
			Constraints: rs.Constraints,
		}

		// Parse "next" field: can be a string (linear) or map (branching)
		bs.Next = parseNextNode(&rs.Next)
		b.States[i] = bs
	}

	return b, nil
}

// parseNextNode converts a YAML node for "next" into a map[string]string.
// A scalar string "foo" becomes {"": "foo"} (linear transition).
// A mapping {"a": "b", "c": "d"} passes through directly.
func parseNextNode(node *yaml.Node) map[string]string {
	if node == nil || node.Kind == 0 {
		return nil
	}

	switch node.Kind {
	case yaml.ScalarNode:
		return map[string]string{"": node.Value}
	case yaml.MappingNode:
		m := make(map[string]string)
		for i := 0; i+1 < len(node.Content); i += 2 {
			m[node.Content[i].Value] = node.Content[i+1].Value
		}
		return m
	default:
		return nil
	}
}

// ResolveParams substitutes ${param} placeholders in a block's states
// using the provided params merged with defaults.
func ResolveParams(b *Block, params map[string]string) (*Block, error) {
	// Merge defaults with provided params (provided take priority)
	merged := make(map[string]string)
	for k, def := range b.Params {
		merged[k] = def.Default
	}
	for k, v := range params {
		merged[k] = v
	}

	// Deep copy the block to avoid mutating the original
	resolved := &Block{
		Name:        b.Name,
		Description: b.Description,
		Params:      b.Params,
		Entry:       b.Entry,
		Exits:       b.Exits,
	}

	resolved.States = make([]BlockState, len(b.States))
	for i, s := range b.States {
		rs := BlockState{
			Name:        s.Name,
			Description: substituteParams(s.Description, merged),
			Prompt:      s.Prompt,
			Verdicts:    s.Verdicts,
		}

		if s.Next != nil {
			rs.Next = make(map[string]string, len(s.Next))
			for k, v := range s.Next {
				rs.Next[k] = v
			}
		}

		// Resolve agent config
		if s.Agent != nil {
			rs.Agent = &AgentConfigRaw{
				AllowedTools: s.Agent.AllowedTools,
				Model:        substituteParams(s.Agent.Model, merged),
				MaxTurns:     substituteParams(s.Agent.MaxTurns, merged),
				Timeout:      substituteParams(s.Agent.Timeout, merged),
			}
		}

		// Resolve constraints
		if s.Constraints != nil {
			maxIter := substituteParams(s.Constraints.MaxIterations, merged)
			rs.Constraints = &ConstraintRaw{MaxIterations: maxIter}
		}

		resolved.States[i] = rs
	}

	return resolved, nil
}

// substituteParams replaces ${param} references in a string.
func substituteParams(s string, params map[string]string) string {
	return paramRe.ReplaceAllStringFunc(s, func(match string) string {
		key := paramRe.FindStringSubmatch(match)[1]
		if val, ok := params[key]; ok {
			return val
		}
		return match
	})
}

// parseInt converts a string to int, returning 0 on failure.
func parseInt(s string) int {
	var n int
	fmt.Sscanf(strings.TrimSpace(s), "%d", &n)
	return n
}
