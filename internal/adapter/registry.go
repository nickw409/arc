package adapter

import "github.com/nwiley/arc/internal/arc"

// Registry maps adapter names to constructor functions.
var Registry = map[string]func() arc.AgentAdapter{
	"claude":  func() arc.AgentAdapter { return &ClaudeAdapter{} },
	"generic": func() arc.AgentAdapter { return &GenericAdapter{Name_: "generic"} },
	"codex":   func() arc.AgentAdapter { return &CodexAdapter{} },
}

// Get returns the adapter registered under the given name.
// If the name is not found, it returns a default ClaudeAdapter.
func Get(name string) arc.AgentAdapter {
	if fn, ok := Registry[name]; ok {
		return fn()
	}
	return &ClaudeAdapter{}
}
