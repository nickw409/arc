package workflow

import "github.com/nwiley/arc/internal/arc"

// ValidationError represents a specific validation failure.
type ValidationError struct {
	Field   string
	Message string
}

// Error formats as "field: message".
func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Message
}

// Validate checks a workflow for structural errors.
func Validate(w *arc.Workflow) []error {
	panic("not implemented")
}
