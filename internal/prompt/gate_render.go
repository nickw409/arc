package prompt

import (
	"bytes"
	"text/template"

	"github.com/nwiley/arc/internal/resources"
)

// gateFuncMap returns the template.FuncMap used for gate-driven prompt templates.
func gateFuncMap() template.FuncMap {
	return template.FuncMap{
		"add": func(a, b int) int { return a + b },
	}
}

// RenderGatePrompt renders a gate-driven prompt template with the given data.
// templateName is the base name (without .md) of a file in prompts/gate/.
func RenderGatePrompt(templateName string, data interface{}) (string, error) {
	b, err := resources.PromptBytes("gate/" + templateName + ".md")
	if err != nil {
		return "", err
	}

	t, err := template.New(templateName).Funcs(gateFuncMap()).Parse(string(b))
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
