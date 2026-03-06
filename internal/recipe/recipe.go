package recipe

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/plan"
	"github.com/nwiley/arc/internal/resources"
	"gopkg.in/yaml.v3"
)

// Param describes a single recipe parameter.
type Param struct {
	Name    string `yaml:"name"`
	Default string `yaml:"default,omitempty"`
}

// PhaseTemplate is a recipe phase before parameter substitution.
type PhaseTemplate struct {
	Name       string              `yaml:"name"`
	Spec       string              `yaml:"spec,omitempty"`
	Verify     string              `yaml:"verify,omitempty"`
	Complexity string              `yaml:"complexity,omitempty"`
	Files      []string            `yaml:"files,omitempty"`
	Deps       []string            `yaml:"deps,omitempty"`
	Gate       GateTemplateSpec    `yaml:"gate,omitempty"`
}

// GateTemplateSpec mirrors arc.GateSpec but uses string slices for YAML decoding.
type GateTemplateSpec struct {
	Assertions []GateAssertionTemplate `yaml:"assertions,omitempty"`
}

// GateAssertionTemplate mirrors arc.GateAssertion with raw template strings.
type GateAssertionTemplate struct {
	Type        string `yaml:"type"`
	Description string `yaml:"description,omitempty"`
	Target      string `yaml:"target,omitempty"`
	FileExists  string `yaml:"file_exists,omitempty"`
	Grep        string `yaml:"grep,omitempty"`
	TestExists  string `yaml:"test_exists,omitempty"`
}

// Recipe is a reusable plan template loaded from a YAML file.
type Recipe struct {
	Name        string          `yaml:"name"`
	Description string          `yaml:"description,omitempty"`
	Params      []Param         `yaml:"params,omitempty"`
	Phases      []PhaseTemplate `yaml:"phases"`
}

// InstantiatedPhase is a phase after parameter substitution.
type InstantiatedPhase struct {
	Name string
	Spec *arc.PhaseSpec
}

// InstantiatedRecipe is a recipe after parameter substitution.
type InstantiatedRecipe struct {
	Recipe *Recipe
	Params map[string]string
	Phases []InstantiatedPhase
}

// Load reads a single recipe from the given file path.
func Load(path string) (*Recipe, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read recipe %s: %w", path, err)
	}
	var r Recipe
	if err := yaml.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parse recipe %s: %w", path, err)
	}
	if r.Name == "" {
		// Fall back to the filename without extension.
		base := filepath.Base(path)
		r.Name = strings.TrimSuffix(base, filepath.Ext(base))
	}
	return &r, nil
}

// LoadAll reads all *.yaml files from dir and returns the parsed recipes.
// Files that fail to parse are returned as errors alongside successfully
// parsed recipes; the caller receives all errors joined.
func LoadAll(dir string) ([]*Recipe, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read recipes dir %s: %w", dir, err)
	}

	var recipes []*Recipe
	var errs []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		r, err := Load(filepath.Join(dir, name))
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		recipes = append(recipes, r)
	}

	if len(errs) > 0 {
		return recipes, fmt.Errorf("errors loading recipes: %s", strings.Join(errs, "; "))
	}
	return recipes, nil
}

// LoadBuiltIn reads a single built-in recipe by name from the embedded resources.
func LoadBuiltIn(name string) (*Recipe, error) {
	data, err := resources.RecipeBytes(name)
	if err != nil {
		return nil, fmt.Errorf("built-in recipe %q not found", name)
	}
	var r Recipe
	if err := yaml.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parse built-in recipe %s: %w", name, err)
	}
	if r.Name == "" {
		r.Name = name
	}
	return &r, nil
}

// LoadAllBuiltIn returns all built-in recipes embedded in the binary.
func LoadAllBuiltIn() ([]*Recipe, error) {
	names := resources.ListBuiltInRecipes()
	var recipes []*Recipe
	var errs []string
	for _, name := range names {
		r, err := LoadBuiltIn(name)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		recipes = append(recipes, r)
	}
	if len(errs) > 0 {
		return recipes, fmt.Errorf("errors loading built-in recipes: %s", strings.Join(errs, "; "))
	}
	return recipes, nil
}

// Instantiate resolves parameter defaults, validates provided params, and
// performs template substitution on all text fields in the recipe.
//
// Rules:
//   - Params with no default and not in provided are required → error.
//   - Keys in provided that do not correspond to a declared param → warning
//     printed to stderr, not an error.
func Instantiate(r *Recipe, provided map[string]string) (*InstantiatedRecipe, error) {
	// Build effective param map: start from defaults, override with provided.
	effective := make(map[string]string, len(r.Params))
	for _, p := range r.Params {
		if p.Default != "" {
			effective[p.Name] = p.Default
		}
	}
	for k, v := range provided {
		effective[k] = v
	}

	// Validate: check required params are satisfied.
	for _, p := range r.Params {
		if _, ok := effective[p.Name]; !ok {
			return nil, fmt.Errorf("required parameter %q not provided", p.Name)
		}
	}

	// Warn about unknown keys.
	declared := make(map[string]bool, len(r.Params))
	for _, p := range r.Params {
		declared[p.Name] = true
	}
	for k := range provided {
		if !declared[k] {
			fmt.Fprintf(os.Stderr, "warning: unknown recipe parameter %q (ignored)\n", k)
		}
	}

	// Substitute params in all phase text fields.
	phases := make([]InstantiatedPhase, 0, len(r.Phases))
	for _, pt := range r.Phases {
		phaseName, err := substituteString(pt.Name, effective)
		if err != nil {
			return nil, fmt.Errorf("phase name template error: %w", err)
		}

		spec, err := substitutePhase(pt, effective)
		if err != nil {
			return nil, fmt.Errorf("phase %q template error: %w", pt.Name, err)
		}

		phases = append(phases, InstantiatedPhase{
			Name: phaseName,
			Spec: spec,
		})
	}

	return &InstantiatedRecipe{
		Recipe: r,
		Params: effective,
		Phases: phases,
	}, nil
}

// ToPlan creates an Arc plan from an instantiated recipe. It calls plan.Create
// for the initial structure, then writes each phase's spec.
// planName defaults to the recipe name if empty. workflowType defaults to "feature".
func ToPlan(inst *InstantiatedRecipe, plansDir, planName string) error {
	return ToPlanWithWorkflow(inst, plansDir, planName, "feature")
}

// ToPlanWithWorkflow is like ToPlan but accepts a workflow type.
func ToPlanWithWorkflow(inst *InstantiatedRecipe, plansDir, planName, workflowType string) error {
	if planName == "" {
		planName = inst.Recipe.Name
	}

	// Collect phase names.
	phaseNames := make([]string, len(inst.Phases))
	for i, p := range inst.Phases {
		phaseNames[i] = p.Name
	}

	if workflowType == "" {
		workflowType = "feature"
	}
	_, err := plan.Create(plan.CreateOptions{
		PlansDir:     plansDir,
		Name:         planName,
		Phases:       phaseNames,
		WorkflowType: workflowType,
	})
	if err != nil {
		return fmt.Errorf("create plan: %w", err)
	}

	// Write specs for each phase. AddPhase would fail because the phases
	// already exist after Create, so we use WriteSpec + UpdateDeps directly.
	for _, p := range inst.Phases {
		if err := plan.WriteSpec(plansDir, planName, p.Name, p.Spec); err != nil {
			return fmt.Errorf("write spec for phase %q: %w", p.Name, err)
		}
		// Update deps if non-empty (Create wires sequential deps by default;
		// override with recipe-specified deps if the phase has explicit ones).
		if len(p.Spec.Deps) > 0 {
			if err := plan.UpdateDeps(plansDir, planName, p.Name, p.Spec.Deps); err != nil {
				return fmt.Errorf("update deps for phase %q: %w", p.Name, err)
			}
		}
	}

	return nil
}

// substituteString applies Go text/template to s with params as data.
// Template delimiters are {{ and }}.
func substituteString(s string, params map[string]string) (string, error) {
	if !strings.Contains(s, "{{") {
		return s, nil
	}
	tmpl, err := template.New("").Option("missingkey=error").Parse(s)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}
	return buf.String(), nil
}

// substituteSlice applies substituteString to each element of a string slice.
func substituteSlice(ss []string, params map[string]string) ([]string, error) {
	if len(ss) == 0 {
		return ss, nil
	}
	result := make([]string, len(ss))
	for i, s := range ss {
		v, err := substituteString(s, params)
		if err != nil {
			return nil, err
		}
		result[i] = v
	}
	return result, nil
}

// substitutePhase performs template substitution on all fields of a PhaseTemplate
// and returns an arc.PhaseSpec.
func substitutePhase(pt PhaseTemplate, params map[string]string) (*arc.PhaseSpec, error) {
	specText, err := substituteString(pt.Spec, params)
	if err != nil {
		return nil, fmt.Errorf("spec field: %w", err)
	}
	testCmd, err := substituteString(pt.Verify, params)
	if err != nil {
		return nil, fmt.Errorf("verify field: %w", err)
	}
	complexity, err := substituteString(pt.Complexity, params)
	if err != nil {
		return nil, fmt.Errorf("complexity field: %w", err)
	}
	files, err := substituteSlice(pt.Files, params)
	if err != nil {
		return nil, fmt.Errorf("files field: %w", err)
	}
	deps, err := substituteSlice(pt.Deps, params)
	if err != nil {
		return nil, fmt.Errorf("deps field: %w", err)
	}

	var assertions []arc.GateAssertion
	for _, at := range pt.Gate.Assertions {
		aType, err := substituteString(at.Type, params)
		if err != nil {
			return nil, fmt.Errorf("assertion type: %w", err)
		}
		aDesc, err := substituteString(at.Description, params)
		if err != nil {
			return nil, fmt.Errorf("assertion description: %w", err)
		}
		aTarget, err := substituteString(at.Target, params)
		if err != nil {
			return nil, fmt.Errorf("assertion target: %w", err)
		}
		aFileExists, err := substituteString(at.FileExists, params)
		if err != nil {
			return nil, fmt.Errorf("assertion file_exists: %w", err)
		}
		aGrep, err := substituteString(at.Grep, params)
		if err != nil {
			return nil, fmt.Errorf("assertion grep: %w", err)
		}
		aTestExists, err := substituteString(at.TestExists, params)
		if err != nil {
			return nil, fmt.Errorf("assertion test_exists: %w", err)
		}
		assertions = append(assertions, arc.GateAssertion{
			Type:        aType,
			Description: aDesc,
			Target:      aTarget,
			FileExists:  aFileExists,
			Grep:        aGrep,
			TestExists:  aTestExists,
		})
	}

	spec := &arc.PhaseSpec{
		Spec:       specText,
		Verify:     testCmd,
		Complexity: complexity,
		Files:      files,
		Deps:       deps,
	}
	if len(assertions) > 0 {
		spec.Gate = arc.GateSpec{Assertions: assertions}
	}

	return spec, nil
}
