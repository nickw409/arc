package recipe

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nwiley/arc/internal/plan"
)

// writeRecipeFile writes YAML content to a temporary file and returns the path.
func writeRecipeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write recipe file: %v", err)
	}
	return path
}

// --- Load tests ---

func TestLoadRecipe(t *testing.T) {
	dir := t.TempDir()
	yaml := `name: api-endpoint
description: "Add a new API endpoint"
params:
  - name: endpoint_name
  - name: http_method
    default: GET
phases:
  - name: handler
    spec: "Create {{.http_method}} handler for /{{.endpoint_name}}"
    verify: go test ./internal/api/
    complexity: medium
`
	path := writeRecipeFile(t, dir, "api-endpoint.yaml", yaml)

	r, err := Load(path)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if r.Name != "api-endpoint" {
		t.Errorf("Name = %q, want %q", r.Name, "api-endpoint")
	}
	if r.Description != "Add a new API endpoint" {
		t.Errorf("Description = %q, want %q", r.Description, "Add a new API endpoint")
	}
	if len(r.Params) != 2 {
		t.Fatalf("len(Params) = %d, want 2", len(r.Params))
	}
	if r.Params[0].Name != "endpoint_name" {
		t.Errorf("Params[0].Name = %q, want %q", r.Params[0].Name, "endpoint_name")
	}
	if r.Params[1].Default != "GET" {
		t.Errorf("Params[1].Default = %q, want %q", r.Params[1].Default, "GET")
	}
	if len(r.Phases) != 1 {
		t.Fatalf("len(Phases) = %d, want 1", len(r.Phases))
	}
}

func TestLoadRecipeFallbackName(t *testing.T) {
	// When name is absent in YAML, it should fall back to the filename.
	dir := t.TempDir()
	yaml := `phases:
  - name: impl
    spec: do the thing
`
	path := writeRecipeFile(t, dir, "my-recipe.yaml", yaml)

	r, err := Load(path)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if r.Name != "my-recipe" {
		t.Errorf("Name = %q, want %q", r.Name, "my-recipe")
	}
}

func TestLoadRecipeNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/recipe.yaml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoadRecipeInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := writeRecipeFile(t, dir, "bad.yaml", "name: [invalid: yaml: {")

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

// --- LoadAll tests ---

func TestLoadAll(t *testing.T) {
	dir := t.TempDir()

	writeRecipeFile(t, dir, "recipe-a.yaml", `name: recipe-a
phases:
  - name: impl
    spec: do a
`)
	writeRecipeFile(t, dir, "recipe-b.yaml", `name: recipe-b
phases:
  - name: impl
    spec: do b
`)
	// Non-YAML file should be ignored.
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignore me"), 0644); err != nil {
		t.Fatalf("write notes.txt: %v", err)
	}

	recipes, err := LoadAll(dir)
	if err != nil {
		t.Fatalf("LoadAll error: %v", err)
	}
	if len(recipes) != 2 {
		t.Errorf("len(recipes) = %d, want 2", len(recipes))
	}
}

func TestLoadAllEmptyDir(t *testing.T) {
	dir := t.TempDir()

	recipes, err := LoadAll(dir)
	if err != nil {
		t.Fatalf("LoadAll error: %v", err)
	}
	if len(recipes) != 0 {
		t.Errorf("expected 0 recipes, got %d", len(recipes))
	}
}

func TestLoadAllDirNotExist(t *testing.T) {
	_, err := LoadAll("/nonexistent/recipes/dir")
	if err == nil {
		t.Fatal("expected error for missing dir, got nil")
	}
}

// --- Instantiate tests ---

func TestInstantiateBasic(t *testing.T) {
	r := &Recipe{
		Name: "api-endpoint",
		Params: []Param{
			{Name: "endpoint_name"},
			{Name: "http_method", Default: "GET"},
			{Name: "package", Default: "internal/api"},
		},
		Phases: []PhaseTemplate{
			{
				Name:       "handler",
				Spec:       "Create {{.http_method}} handler for /{{.endpoint_name}} in {{.package}}.",
				Verify:     "go test ./{{.package}}/ -count=1",
				Complexity: "medium",
				Files:      []string{"{{.package}}/{{.endpoint_name}}.go"},
			},
		},
	}

	inst, err := Instantiate(r, map[string]string{
		"endpoint_name": "users",
	})
	if err != nil {
		t.Fatalf("Instantiate error: %v", err)
	}

	if len(inst.Phases) != 1 {
		t.Fatalf("len(Phases) = %d, want 1", len(inst.Phases))
	}
	p := inst.Phases[0]
	if p.Name != "handler" {
		t.Errorf("Name = %q, want %q", p.Name, "handler")
	}
	if !strings.Contains(p.Spec.Spec, "GET handler for /users") {
		t.Errorf("Spec = %q, should contain 'GET handler for /users'", p.Spec.Spec)
	}
	if !strings.Contains(p.Spec.Verify, "go test ./internal/api/") {
		t.Errorf("Test = %q, should contain 'go test ./internal/api/'", p.Spec.Verify)
	}
	if len(p.Spec.Files) != 1 || p.Spec.Files[0] != "internal/api/users.go" {
		t.Errorf("Files = %v, want [internal/api/users.go]", p.Spec.Files)
	}
}

func TestInstantiateDefaults(t *testing.T) {
	r := &Recipe{
		Name: "simple",
		Params: []Param{
			{Name: "method", Default: "POST"},
		},
		Phases: []PhaseTemplate{
			{Name: "impl", Spec: "Use {{.method}} method"},
		},
	}

	// Provide no params — defaults should be used.
	inst, err := Instantiate(r, nil)
	if err != nil {
		t.Fatalf("Instantiate error: %v", err)
	}
	if !strings.Contains(inst.Phases[0].Spec.Spec, "POST method") {
		t.Errorf("Spec = %q, should contain 'POST method'", inst.Phases[0].Spec.Spec)
	}
}

func TestInstantiateMissingRequired(t *testing.T) {
	r := &Recipe{
		Name: "simple",
		Params: []Param{
			{Name: "required_param"},
		},
		Phases: []PhaseTemplate{
			{Name: "impl", Spec: "do {{.required_param}}"},
		},
	}

	_, err := Instantiate(r, nil)
	if err == nil {
		t.Fatal("expected error for missing required param, got nil")
	}
	if !strings.Contains(err.Error(), "required parameter") {
		t.Errorf("error = %q, should mention 'required parameter'", err.Error())
	}
	if !strings.Contains(err.Error(), "required_param") {
		t.Errorf("error = %q, should mention 'required_param'", err.Error())
	}
}

func TestInstantiateUnknownParamWarnsNotErrors(t *testing.T) {
	r := &Recipe{
		Name: "simple",
		Params: []Param{
			{Name: "known_param"},
		},
		Phases: []PhaseTemplate{
			{Name: "impl", Spec: "do {{.known_param}}"},
		},
	}

	// unknown_param is not declared in the recipe.
	inst, err := Instantiate(r, map[string]string{
		"known_param":   "something",
		"unknown_param": "extra",
	})
	if err != nil {
		t.Fatalf("unknown param should warn, not error; got: %v", err)
	}
	if inst == nil {
		t.Fatal("expected non-nil InstantiatedRecipe")
	}
}

func TestInstantiateGateAssertions(t *testing.T) {
	r := &Recipe{
		Name: "gated",
		Params: []Param{
			{Name: "fn_name"},
		},
		Phases: []PhaseTemplate{
			{
				Name: "impl",
				Spec: "implement {{.fn_name}}",
				Gate: GateTemplateSpec{
					Assertions: []GateAssertionTemplate{
						{
							Type:   "grep",
							Target: "func {{.fn_name}}",
							Grep:   "func {{.fn_name}}",
						},
					},
				},
			},
		},
	}

	inst, err := Instantiate(r, map[string]string{"fn_name": "MyFunc"})
	if err != nil {
		t.Fatalf("Instantiate error: %v", err)
	}

	gate := inst.Phases[0].Spec.Gate
	if len(gate.Assertions) != 1 {
		t.Fatalf("len(Assertions) = %d, want 1", len(gate.Assertions))
	}
	if gate.Assertions[0].Target != "func MyFunc" {
		t.Errorf("Target = %q, want %q", gate.Assertions[0].Target, "func MyFunc")
	}
	if gate.Assertions[0].Grep != "func MyFunc" {
		t.Errorf("Grep = %q, want %q", gate.Assertions[0].Grep, "func MyFunc")
	}
}

func TestInstantiateNoParams(t *testing.T) {
	r := &Recipe{
		Name:   "no-params",
		Phases: []PhaseTemplate{{Name: "impl", Spec: "just do it"}},
	}

	inst, err := Instantiate(r, nil)
	if err != nil {
		t.Fatalf("Instantiate error: %v", err)
	}
	if inst.Phases[0].Spec.Spec != "just do it" {
		t.Errorf("Spec = %q, want %q", inst.Phases[0].Spec.Spec, "just do it")
	}
}

func TestInstantiatePhaseNameSubstitution(t *testing.T) {
	r := &Recipe{
		Name: "dynamic-phase",
		Params: []Param{
			{Name: "module"},
		},
		Phases: []PhaseTemplate{
			{Name: "impl-{{.module}}", Spec: "implement {{.module}}"},
		},
	}

	inst, err := Instantiate(r, map[string]string{"module": "auth"})
	if err != nil {
		t.Fatalf("Instantiate error: %v", err)
	}
	if inst.Phases[0].Name != "impl-auth" {
		t.Errorf("phase Name = %q, want %q", inst.Phases[0].Name, "impl-auth")
	}
}

func TestInstantiateDeps(t *testing.T) {
	r := &Recipe{
		Name: "with-deps",
		Params: []Param{
			{Name: "pkg", Default: "internal/api"},
		},
		Phases: []PhaseTemplate{
			{Name: "handler", Spec: "create handler"},
			{Name: "integration", Spec: "add integration test", Deps: []string{"handler"}},
		},
	}

	inst, err := Instantiate(r, nil)
	if err != nil {
		t.Fatalf("Instantiate error: %v", err)
	}
	deps := inst.Phases[1].Spec.Deps
	if len(deps) != 1 || deps[0] != "handler" {
		t.Errorf("Deps = %v, want [handler]", deps)
	}
}

// --- ToPlan tests ---

func TestToPlanCreatesFiles(t *testing.T) {
	plansDir := t.TempDir()

	r := &Recipe{
		Name: "my-recipe",
		Params: []Param{
			{Name: "pkg", Default: "internal/api"},
		},
		Phases: []PhaseTemplate{
			{Name: "handler", Spec: "create handler", Verify: "go test ./internal/api/", Complexity: "medium"},
			{Name: "integration", Spec: "add integration test", Deps: []string{"handler"}},
		},
	}

	inst, err := Instantiate(r, nil)
	if err != nil {
		t.Fatalf("Instantiate error: %v", err)
	}

	if err := ToPlan(inst, plansDir, "my-plan"); err != nil {
		t.Fatalf("ToPlan error: %v", err)
	}

	// Verify plan directory structure.
	planDir := filepath.Join(plansDir, "my-plan")
	if _, err := os.Stat(planDir); os.IsNotExist(err) {
		t.Fatal("plan directory should exist")
	}
	if _, err := os.Stat(filepath.Join(planDir, "plan.json")); os.IsNotExist(err) {
		t.Fatal("plan.json should exist")
	}

	// Verify phases have spec.yaml written.
	for _, phaseName := range []string{"handler", "integration"} {
		specPath := filepath.Join(planDir, "phases", phaseName, "spec.yaml")
		if _, err := os.Stat(specPath); os.IsNotExist(err) {
			t.Fatalf("spec.yaml for phase %q should exist", phaseName)
		}
	}

	// Verify spec content for handler phase.
	spec, err := plan.ReadSpec(plansDir, "my-plan", "handler")
	if err != nil {
		t.Fatalf("ReadSpec handler error: %v", err)
	}
	if spec.Spec != "create handler" {
		t.Errorf("handler spec.Spec = %q, want %q", spec.Spec, "create handler")
	}
	if spec.Verify != "go test ./internal/api/" {
		t.Errorf("handler spec.Verify = %q, want %q", spec.Verify, "go test ./internal/api/")
	}
	if spec.Complexity != "medium" {
		t.Errorf("handler spec.Complexity = %q, want %q", spec.Complexity, "medium")
	}
}

func TestToPlanDefaultPlanName(t *testing.T) {
	plansDir := t.TempDir()

	r := &Recipe{
		Name:   "my-recipe",
		Phases: []PhaseTemplate{{Name: "impl", Spec: "do it"}},
	}
	inst, err := Instantiate(r, nil)
	if err != nil {
		t.Fatalf("Instantiate error: %v", err)
	}

	// Pass empty planName — should default to recipe name.
	if err := ToPlan(inst, plansDir, ""); err != nil {
		t.Fatalf("ToPlan error: %v", err)
	}

	planDir := filepath.Join(plansDir, "my-recipe")
	if _, err := os.Stat(planDir); os.IsNotExist(err) {
		t.Fatal("plan directory should be named after recipe when planName is empty")
	}
}

func TestToPlanEmptyRecipe(t *testing.T) {
	plansDir := t.TempDir()

	r := &Recipe{
		Name:   "empty-recipe",
		Phases: []PhaseTemplate{},
	}
	inst, err := Instantiate(r, nil)
	if err != nil {
		t.Fatalf("Instantiate error: %v", err)
	}

	// plan.Create requires at least one phase, so ToPlan should error.
	err = ToPlan(inst, plansDir, "empty-plan")
	if err == nil {
		t.Fatal("expected error for empty phases, got nil")
	}
}
