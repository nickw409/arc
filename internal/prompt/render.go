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

// Package-level compiled regexes for preprocessHandlebars.
var (
	partialRe      = regexp.MustCompile(`(?m)^\s*\{\{>\s+([^}]+?)\s*\}\}\s*$\n?`)
	pipeDefaultRe  = regexp.MustCompile(`\{\{(\w+)\.(\w+)\s*\|\s*default:\s*"([^"]*)"\}\}`)
	dotAccessRe    = regexp.MustCompile(`\{\{(\w+)\.(\w+)\}\}`)
	ifDotRe        = regexp.MustCompile(`\{\{#if\s+(\w+)\.(\w+)\}\}`)
	unlessDotRe    = regexp.MustCompile(`\{\{#unless\s+(\w+)\.(\w+)\}\}`)
	ifBareRe       = regexp.MustCompile(`\{\{#if\s+(\w+)\}\}`)
	unlessBareRe   = regexp.MustCompile(`\{\{#unless\s+(\w+)\}\}`)
	bareIdentRe    = regexp.MustCompile(`\{\{(\w+)\}\}`)
)

// TemplateContext contains all variables available during prompt rendering.
type TemplateContext struct {
	Phase          string
	Plan           string
	Iteration      int
	PlanMD         string
	State          map[string]string
	Params         map[string]string
	PlanFile       string
	PhaseDir       string
	StateFile      string
	ScriptsDir     string
	Mode           string
	DisputeCount   int
	DisputeList    string
	PreviousMemory string // notes saved by a previous run of the same state
	ScoutReport    string // scout agent output (edge cases to test)
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
func preprocessHandlebars(s string) (string, error) {
	// Inline partial includes {{> path/to/file.md}} by loading from embedded resources.
	var partialErr error
	s = partialRe.ReplaceAllStringFunc(s, func(match string) string {
		if partialErr != nil {
			return ""
		}
		parts := partialRe.FindStringSubmatch(match)
		if len(parts) < 2 {
			return ""
		}
		partialPath := parts[1]
		content, err := resources.PromptBytes(partialPath)
		if err != nil {
			partialErr = fmt.Errorf("partial include %q not found: %w", partialPath, err)
			return ""
		}
		return string(content)
	})
	if partialErr != nil {
		return "", partialErr
	}

	// Handle {{X.Y | default: "Z"}} pipe syntax with default values.
	s = pipeDefaultRe.ReplaceAllStringFunc(s, func(match string) string {
		parts := pipeDefaultRe.FindStringSubmatch(match)
		mapName := parts[1]
		key := parts[2]
		defaultVal := parts[3]
		capName := capitalize(mapName)
		return fmt.Sprintf(`{{defaultIndex .%s "%s" "%s"}}`, capName, key, defaultVal)
	})

	// Handle {{X.Y}} dot-access without pipes.
	s = dotAccessRe.ReplaceAllStringFunc(s, func(match string) string {
		parts := dotAccessRe.FindStringSubmatch(match)
		mapName := parts[1]
		key := parts[2]
		capName := capitalize(mapName)
		return fmt.Sprintf(`{{safeGet .%s "%s"}}`, capName, key)
	})

	// {{#if X.Y}} -> {{if hasKey .X "Y"}} for map field access in conditionals
	s = ifDotRe.ReplaceAllStringFunc(s, func(match string) string {
		parts := ifDotRe.FindStringSubmatch(match)
		mapName := parts[1]
		key := parts[2]
		capName := capitalize(mapName)
		return fmt.Sprintf(`{{if hasKey .%s "%s"}}`, capName, key)
	})

	// {{#unless X.Y}} -> {{if not (hasKey .X "Y")}}
	s = unlessDotRe.ReplaceAllStringFunc(s, func(match string) string {
		parts := unlessDotRe.FindStringSubmatch(match)
		mapName := parts[1]
		key := parts[2]
		capName := capitalize(mapName)
		return fmt.Sprintf(`{{if not (hasKey .%s "%s")}}`, capName, key)
	})

	// {{#if X}} -> {{if .X}} (where X is a bare identifier)
	s = ifBareRe.ReplaceAllString(s, "{{if .$1}}")

	// {{#unless X}} -> {{if not .X}}
	s = unlessBareRe.ReplaceAllString(s, "{{if not .$1}}")

	// {{/if}} and {{/unless}} -> {{end}}
	s = strings.ReplaceAll(s, "{{/if}}", "{{end}}")
	s = strings.ReplaceAll(s, "{{/unless}}", "{{end}}")

	// Bare {{identifier}} -> {{.identifier}} for all non-keywords
	goKeywords := map[string]bool{
		"if": true, "else": true, "end": true, "range": true,
		"with": true, "block": true, "define": true, "template": true,
		"nil": true, "not": true, "and": true, "or": true,
	}
	s = bareIdentRe.ReplaceAllStringFunc(s, func(match string) string {
		inner := bareIdentRe.FindStringSubmatch(match)[1]
		if goKeywords[inner] {
			return match
		}
		return fmt.Sprintf("{{.%s}}", inner)
	})

	return s, nil
}

// capitalize returns a string with its first letter uppercased.
func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// safeIndex is a custom index function that returns an error for nil maps
// and missing keys. Used by explicit {{index .X "Y"}} calls in Go templates.
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

// safeGet looks up a key in a map, returning empty string for nil maps or
// missing keys. Used by Handlebars-style {{params.X}} access after
// preprocessing converts it to {{safeGet .Params "X"}}.
func safeGet(item interface{}, key string) string {
	if item == nil {
		return ""
	}
	v := reflect.ValueOf(item)
	if !v.IsValid() || (v.Kind() == reflect.Map && v.IsNil()) {
		return ""
	}
	if v.Kind() != reflect.Map {
		return ""
	}
	result := v.MapIndex(reflect.ValueOf(key))
	if !result.IsValid() {
		return ""
	}
	return fmt.Sprintf("%v", result.Interface())
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
		"DisputeCount":   ctx.DisputeCount,
		"DisputeList":    ctx.DisputeList,
		"PreviousMemory": ctx.PreviousMemory,
		"ScoutReport":    ctx.ScoutReport,
		// Lowercase keys for Handlebars-style references
		"phase":           ctx.Phase,
		"plan":            ctx.Plan,
		"iteration":       ctx.Iteration,
		"plan_md":         ctx.PlanMD,
		"state":           ctx.State,
		"params":          ctx.Params,
		"plan_file":       ctx.PlanFile,
		"phase_dir":       ctx.PhaseDir,
		"state_file":      ctx.StateFile,
		"scripts_dir":     ctx.ScriptsDir,
		"mode":            ctx.Mode,
		"dispute_count":   ctx.DisputeCount,
		"dispute_list":    ctx.DisputeList,
		"previous_memory": ctx.PreviousMemory,
		"scout_report":    ctx.ScoutReport,
	}
}

// RenderString renders a prompt template from a raw string.
// If tmplStr looks like a resource path (e.g. "prompts/blocks/adversary.md"),
// the prompt content is loaded from embedded resources first.
func RenderString(tmplStr string, ctx TemplateContext) (string, error) {
	// Detect resource paths passed as template strings and resolve them.
	if strings.HasPrefix(tmplStr, "prompts/") && strings.HasSuffix(tmplStr, ".md") && !strings.Contains(tmplStr, "\n") {
		resourcePath := strings.TrimPrefix(tmplStr, "prompts/")
		if loaded, err := resources.PromptBytes(resourcePath); err == nil && len(loaded) > 0 {
			tmplStr = string(loaded)
		}
	}

	processed, err := preprocessHandlebars(tmplStr)
	if err != nil {
		return "", err
	}

	funcMap := template.FuncMap{
		"index":        safeIndex,
		"safeGet":      safeGet,
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
	processed, err := preprocessHandlebars(tmplStr)
	if err != nil {
		return err
	}
	t, err := template.New("validate").Funcs(template.FuncMap{
		"index":        safeIndex,
		"safeGet":      safeGet,
		"defaultIndex": defaultIndex,
		"hasKey":       hasKey,
	}).Parse(processed)
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
	m := map[string]string{
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
		"adversary_round":            strconv.Itoa(state.AdversaryRound),
	}

	// Serialize adversary test files for template access
	if len(state.AdversaryTests) > 0 {
		var files []string
		for _, roundFiles := range state.AdversaryTests {
			files = append(files, roundFiles...)
		}
		m["adversary_test_files"] = strings.Join(files, "\n")
	}

	return m
}
