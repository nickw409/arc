package arc

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// UnmarshalYAML handles polymorphic "next" field:
//   1. Try decoding as string (yaml.ScalarNode) -> Branches{"": nextState} (linear)
//   2. Try decoding as map[string]string (yaml.MappingNode) -> Branches{Verdict(k): v} (conditional)
//   3. If neither: return error with line number
func (t *Transition) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		// Handle null scalar
		if value.Tag == "!!null" {
			t.Branches = nil
			return nil
		}
		var s string
		if err := value.Decode(&s); err != nil {
			return fmt.Errorf("line %d: 'next' must be a string or verdict map", value.Line)
		}
		t.Branches = map[Verdict]string{"": s}
		return nil
	case yaml.MappingNode:
		var m map[string]string
		if err := value.Decode(&m); err != nil {
			return fmt.Errorf("line %d: 'next' must be a string or verdict map", value.Line)
		}
		t.Branches = make(map[Verdict]string, len(m))
		for k, v := range m {
			t.Branches[Verdict(k)] = v
		}
		return nil
	default:
		return fmt.Errorf("line %d: 'next' must be a string or verdict map", value.Line)
	}
}
