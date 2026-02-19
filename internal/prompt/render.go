package prompt

import (
	"github.com/nwiley/arc/internal/arc"
)

// TemplateContext contains all variables available during prompt rendering.
type TemplateContext struct {
	Phase     string
	Plan      string
	Iteration int
	PlanMD    string
	State     map[string]string
	Params    map[string]string
}

// Render loads a prompt template from embedded resources and renders it.
func Render(promptPath string, ctx TemplateContext) (string, error) {
	panic("not implemented")
}

// RenderString renders a prompt template from a raw string.
func RenderString(tmplStr string, ctx TemplateContext) (string, error) {
	panic("not implemented")
}

// ValidateTemplate parses a template and executes against placeholder context.
func ValidateTemplate(tmplStr string) error {
	panic("not implemented")
}

// StateToTemplateMap converts a PhaseState into a flat map[string]string.
func StateToTemplateMap(state *arc.PhaseState) map[string]string {
	panic("not implemented")
}
