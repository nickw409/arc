package plan

import "io"

// StatusOptions configures status display.
type StatusOptions struct {
	PlansDir string
	PlanName string
}

// Status writes plan status to the given writer.
func Status(w io.Writer, opts StatusOptions) error {
	panic("not implemented")
}

// StatusIcon returns the display icon for a phase status string.
func StatusIcon(status string) string {
	panic("not implemented")
}
