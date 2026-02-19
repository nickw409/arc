package project

import "github.com/nwiley/arc/internal/config"

// InitOptions configures project initialization.
type InitOptions struct {
	ProjectRoot string
	Force       bool
}

// Init initializes arc in a project directory.
func Init(opts InitOptions) (*config.Config, error) {
	panic("not implemented")
}
