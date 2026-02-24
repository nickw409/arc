package prompt

import (
	"bytes"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"text/template"

	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/resources"
)

// TemplateContext contains all variables available during prompt rendering.
type TemplateContext struct {
	Phase        string
	Plan         string
	Iteration    int
	PlanMD       string
	State        map[string]string
	Params       map[string]string
	PlanFile     string
	PhaseDir     string
	StateFile    string
	ScriptsDir   string
	Mode         string
	DisputeCount int
	DisputeList  string
}

// Render loads a prompt template from embedded resources and renders it.
func Render(promptPath string, ctx TemplateContext) (string, error) {
	b, err := resources.PromptBytes(promptPath)
	if err != nil {
		return "", err
	}
	return RenderString(string(b), ctx)
}

// preprocessHandlebars converts Handlebars-style syntax to Go template syntax.
func preprocessHandlebars(s string) string {
	// Inline partial includes {{> path/to/file.md}} by loading from embedded resources.
	partialRe := regexp.MustCompile(`(?m)^\s*\{\{>\s+([^}]+?)\s*\}\}\s*$\n?`)
	s = partialRe.ReplaceAllStringFunc(s, func(match string) string {
		parts := partialRe.FindStringSubmatch(match)
		if len(parts) < 2 {
			return ""
		}
		partialPath := parts[1]
		content, err := resources.PromptBytes(partialPath)
		if err != nil {
			// If partial not found, remove the line silently (backwards compat)
			return ""
		}
		return string(content)
	})

	// Handle {{X.Y | default: "Z"}} pipe syntax with default values.
	// Convert to: {{index .X "Y"}}  (drop the default, index will handle missing keys)
	pipeDefaultRe := regexp.MustCompile(`\{\{(\w+)\.(\w+)\s*\|\s*default:\s*"([^"]*)"\}\}`)
	s = pipeDefaultRe.ReplaceAllStringFunc(s, func(match string) string {
		parts := pipeDefaultRe.FindStringSubmatch(match)
		mapName := parts[1]
		key := parts[2]
		defaultVal := parts[3]
		// Use a custom "defaultIndex" call: tries the map, falls back to default
		capName := capitalize(mapName)
		return fmt.Sprintf(`{{defaultIndex .%s "%s" "%s"}}`, capName, key, defaultVal)
	})

	// Handle {{X.Y}} dot-access without pipes.
	// Convert to: {{index .X "Y"}}
	dotAccessRe := regexp.MustCompile(`\{\{(\w+)\.(\w+)\}\}`)
	s = dotAccessRe.ReplaceAllStringFunc(s, func(match string) string {
		parts := dotAccessRe.FindStringSubmatch(match)
		mapName := parts[1]
		key := parts[2]
		capName := capitalize(mapName)
		return fmt.Sprintf(`{{index .%s "%s"}}`, capName, key)
	})

	// {{#if X.Y}} -> {{if hasKey .X "Y"}} for map field access in conditionals
	ifDotRe := regexp.MustCompile(`\{\{#if\s+(\w+)\.(\w+)\}\}`)
	s = ifDotRe.ReplaceAllStringFunc(s, func(match string) string {
		parts := ifDotRe.FindStringSubmatch(match)
		mapName := parts[1]
		key := parts[2]
		capName := capitalize(mapName)
		return fmt.Sprintf(`{{if hasKey .%s "%s"}}`, capName, key)
	})

	// {{#unless X.Y}} -> {{if not (hasKey .X "Y")}}
	unlessDotRe := regexp.MustCompile(`\{\{#unless\s+(\w+)\.(\w+)\}\}`)
	s = unlessDotRe.ReplaceAllStringFunc(s, func(match string) string {
		parts := unlessDotRe.FindStringSubmatch(match)
		mapName := parts[1]
		key := parts[2]
		capName := capitalize(mapName)
		return fmt.Sprintf(`{{if not (hasKey .%s "%s")}}`, capName, key)
	})

	// {{#if X}} -> {{if .X}} (where X is a bare identifier)
	s = regexp.MustCompile(`\{\{#if\s+(\w+)\}\}`).ReplaceAllString(s, "{{if .$1}}")

	// {{#unless X}} -> {{if not .X}}
	s = regexp.MustCompile(`\{\{#unless\s+(\w+)\}\}`).ReplaceAllString(s, "{{if not .$1}}")

	// {{/if}} and {{/unless}} -> {{end}}
	s = strings.ReplaceAll(s, "{{/if}}", "{{end}}")
	s = strings.ReplaceAll(s, "{{/unless}}", "{{end}}")

	// Bare {{identifier}} -> {{.identifier}} for all non-keywords
	bareRe := regexp.MustCompile(`\{\{(\w+)\}\}`)
	goKeywords := map[string]bool{
		"if": true, "else": true, "end": true, "range": true,
		"with": true, "block": true, "define": true, "template": true,
		"nil": true, "not": true, "and": true, "or": true,
	}
	s = bareRe.ReplaceAllStringFunc(s, func(match string) string {
		inner := bareRe.FindStringSubmatch(match)[1]
		if goKeywords[inner] {
			return match
		}
		return fmt.Sprintf("{{.%s}}", inner)
	})

	return s
}

// capitalize returns a string with its first letter uppercased.
func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// safeIndex is a custom index function that returns an error for nil maps.
func safeIndex(item interface{}, indices ...interface{}) (interface{}, error) {
	v := reflect.ValueOf(item)
	if !v.IsValid() || (v.Kind() == reflect.Map && v.IsNil()) {
		return "", fmt.Errorf("index of nil map")
	}
	for _, idx := range indices {
		k := reflect.ValueOf(idx)
		if v.Kind() != reflect.Map {
			return "", fmt.Errorf("index of non-map type %s", v.Type())
		}
		result := v.MapIndex(k)
		if !result.IsValid() {
			return "", fmt.Errorf("key %q not found in map", idx)
		}
		v = result
	}
	return v.Interface(), nil
}

// defaultIndex looks up a key in a map and returns a default value if the map
// is nil or the key is not found.
func defaultIndex(item interface{}, key string, defaultVal string) string {
	if item == nil {
		return defaultVal
	}
	v := reflect.ValueOf(item)
	if !v.IsValid() || (v.Kind() == reflect.Map && v.IsNil()) {
		return defaultVal
	}
	if v.Kind() != reflect.Map {
		return defaultVal
	}
	result := v.MapIndex(reflect.ValueOf(key))
	if !result.IsValid() {
		return defaultVal
	}
	s := fmt.Sprintf("%v", result.Interface())
	if s == "" {
		return defaultVal
	}
	return s
}

// hasKey checks if a map has a non-empty value for a given key.
func hasKey(item interface{}, key string) bool {
	if item == nil {
		return false
	}
	v := reflect.ValueOf(item)
	if !v.IsValid() || (v.Kind() == reflect.Map && v.IsNil()) {
		return false
	}
	if v.Kind() != reflect.Map {
		return false
	}
	result := v.MapIndex(reflect.ValueOf(key))
	if !result.IsValid() {
		return false
	}
	s := fmt.Sprintf("%v", result.Interface())
	return s != ""
}

// contextToMap converts a TemplateContext to a flat map[string]interface{}.
// Both capitalized (for direct .Field references in tests) and lowercase
// (for Handlebars-style references in prompts) keys are included.
func contextToMap(ctx TemplateContext) map[string]interface{} {
	return map[string]interface{}{
		// Capitalized keys for Go template direct references
		"Phase":        ctx.Phase,
		"Plan":         ctx.Plan,
		"Iteration":    ctx.Iteration,
		"PlanMD":       ctx.PlanMD,
		"State":        ctx.State,
		"Params":       ctx.Params,
		"PlanFile":     ctx.PlanFile,
		"PhaseDir":     ctx.PhaseDir,
		"StateFile":    ctx.StateFile,
		"ScriptsDir":   ctx.ScriptsDir,
		"Mode":         ctx.Mode,
		"DisputeCount": ctx.DisputeCount,
		"DisputeList":  ctx.DisputeList,
		// Lowercase keys for Handlebars-style references
		"phase":         ctx.Phase,
		"plan":          ctx.Plan,
		"iteration":     ctx.Iteration,
		"plan_md":       ctx.PlanMD,
		"state":         ctx.State,
		"params":        ctx.Params,
		"plan_file":     ctx.PlanFile,
		"phase_dir":     ctx.PhaseDir,
		"state_file":    ctx.StateFile,
		"scripts_dir":   ctx.ScriptsDir,
		"mode":          ctx.Mode,
		"dispute_count": ctx.DisputeCount,
		"dispute_list":  ctx.DisputeList,
	}
}

// RenderString renders a prompt template from a raw string.
func RenderString(tmplStr string, ctx TemplateContext) (string, error) {
	processed := preprocessHandlebars(tmplStr)

	funcMap := template.FuncMap{
		"index":        safeIndex,
		"defaultIndex": defaultIndex,
		"hasKey":       hasKey,
	}

	t, err := template.New("prompt").Funcs(funcMap).Option("missingkey=error").Parse(processed)
	if err != nil {
		return "", err
	}

	data := contextToMap(ctx)

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// ValidateTemplate parses a template and executes against placeholder context.
func ValidateTemplate(tmplStr string) error {
	t, err := template.New("validate").Parse(tmplStr)
	if err != nil {
		return err
	}
	ctx := TemplateContext{
		State:  map[string]string{},
		Params: map[string]string{},
	}
	data := contextToMap(ctx)
	var buf bytes.Buffer
	return t.Execute(&buf, data)
}

// FormatDisputeList formats a slice of Disputes into a markdown list for templates.
func FormatDisputeList(disputes []arc.Dispute) string {
	if len(disputes) == 0 {
		return "(none)"
	}
	var b strings.Builder
	for _, d := range disputes {
		fmt.Fprintf(&b, "- **%s**: %s\n", d.TestName, d.Reason)
	}
	return strings.TrimRight(b.String(), "\n")
}

// StateToTemplateMap converts a PhaseState into a flat map[string]string.
func StateToTemplateMap(state *arc.PhaseState) map[string]string {
	return map[string]string{
		"phase":                      state.Phase,
		"plan":                       state.Plan,
		"workflow_type":              state.WorkflowType,
		"phase_status":               state.PhaseStatus,
		"current_state":              state.CurrentState,
		"iteration":                  strconv.Itoa(state.Iteration.Current),
		"max_iterations":             strconv.Itoa(state.Iteration.Max),
		"tests_passing":              strconv.Itoa(state.TestsPassing),
		"tests_total":                strconv.Itoa(state.TestsTotal),
		"stuck_iterations":           strconv.Itoa(state.StuckIterations),
		"hang_count":                 strconv.Itoa(state.HangCount),
		"last_verdict":               state.LastVerdict,
		"last_reviewed_iteration":    strconv.Itoa(state.LastReviewedIter),
		"last_qa_reviewed_iteration": strconv.Itoa(state.LastQAReviewedIter),
		"rollback_count":             strconv.Itoa(state.RollbackCount),
		"global_iterations":          strconv.Itoa(state.GlobalIterations),
		"last_commit":                state.LastCommit,
		"model_override":             state.ModelOverride,
	}
}
