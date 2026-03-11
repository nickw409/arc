package arc

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// Promise tests
// ---------------------------------------------------------------------------

func TestPromise_YAMLRoundTrip_FuncExists(t *testing.T) {
	p := Promise{FuncExists: "func NewFoo(x int) *Foo"}
	data, err := yaml.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Promise
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.FuncExists != p.FuncExists {
		t.Errorf("FuncExists = %q, want %q", got.FuncExists, p.FuncExists)
	}
}

func TestPromise_YAMLRoundTrip_TestExists(t *testing.T) {
	p := Promise{TestExists: "TestFooBar"}
	data, _ := yaml.Marshal(p)
	var got Promise
	yaml.Unmarshal(data, &got)
	if got.TestExists != p.TestExists {
		t.Errorf("TestExists = %q, want %q", got.TestExists, p.TestExists)
	}
}

func TestPromise_YAMLRoundTrip_FileExists(t *testing.T) {
	p := Promise{FileExists: "internal/foo/bar.go"}
	data, _ := yaml.Marshal(p)
	var got Promise
	yaml.Unmarshal(data, &got)
	if got.FileExists != p.FileExists {
		t.Errorf("FileExists = %q, want %q", got.FileExists, p.FileExists)
	}
}

func TestPromise_YAMLRoundTrip_TestCovers(t *testing.T) {
	p := Promise{TestCovers: "NewFoo", Test: "TestNewFoo"}
	data, _ := yaml.Marshal(p)
	var got Promise
	yaml.Unmarshal(data, &got)
	if got.TestCovers != p.TestCovers || got.Test != p.Test {
		t.Errorf("got %+v, want %+v", got, p)
	}
}

func TestPromise_YAMLRoundTrip_EmptyString(t *testing.T) {
	p := Promise{FuncExists: ""}
	data, _ := yaml.Marshal(p)
	var got Promise
	yaml.Unmarshal(data, &got)
	if got.FuncExists != "" {
		t.Errorf("FuncExists = %q, want empty", got.FuncExists)
	}
}

func TestPromise_JSONRoundTrip_FuncExists(t *testing.T) {
	p := Promise{FuncExists: "func Bar()"}
	data, _ := json.Marshal(p)
	var got Promise
	json.Unmarshal(data, &got)
	if got.FuncExists != p.FuncExists {
		t.Errorf("FuncExists = %q, want %q", got.FuncExists, p.FuncExists)
	}
}

func TestPromise_JSONRoundTrip_TestExists(t *testing.T) {
	p := Promise{TestExists: "TestBar"}
	data, _ := json.Marshal(p)
	var got Promise
	json.Unmarshal(data, &got)
	if got.TestExists != p.TestExists {
		t.Errorf("TestExists = %q, want %q", got.TestExists, p.TestExists)
	}
}

func TestPromise_JSONRoundTrip_FileExists(t *testing.T) {
	p := Promise{FileExists: "cmd/main.go"}
	data, _ := json.Marshal(p)
	var got Promise
	json.Unmarshal(data, &got)
	if got.FileExists != p.FileExists {
		t.Errorf("FileExists = %q, want %q", got.FileExists, p.FileExists)
	}
}

func TestPromise_JSONRoundTrip_TestCovers(t *testing.T) {
	p := Promise{TestCovers: "Bar", Test: "TestBar"}
	data, _ := json.Marshal(p)
	var got Promise
	json.Unmarshal(data, &got)
	if got.TestCovers != p.TestCovers || got.Test != p.Test {
		t.Errorf("got %+v, want %+v", got, p)
	}
}

func TestPhaseSpec_WithPromises_YAML(t *testing.T) {
	yamlData := `
name: my-phase
spec: Implement foo
promises:
  - func_exists: "func NewFoo()"
  - test_exists: TestNewFoo
  - file_exists: internal/foo/foo.go
`
	var spec PhaseSpec
	if err := yaml.Unmarshal([]byte(yamlData), &spec); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(spec.Promises) != 3 {
		t.Fatalf("len(Promises) = %d, want 3", len(spec.Promises))
	}
	if spec.Promises[0].FuncExists != "func NewFoo()" {
		t.Errorf("Promises[0].FuncExists = %q", spec.Promises[0].FuncExists)
	}
	if spec.Promises[1].TestExists != "TestNewFoo" {
		t.Errorf("Promises[1].TestExists = %q", spec.Promises[1].TestExists)
	}
	if spec.Promises[2].FileExists != "internal/foo/foo.go" {
		t.Errorf("Promises[2].FileExists = %q", spec.Promises[2].FileExists)
	}
}

func TestPhaseSpec_WithPromises_JSON(t *testing.T) {
	spec := PhaseSpec{
		Name: "my-phase",
		Spec: "Implement foo",
		Promises: []Promise{
			{FuncExists: "func NewFoo()"},
			{TestExists: "TestNewFoo"},
		},
	}
	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got PhaseSpec
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Promises) != 2 {
		t.Fatalf("len(Promises) = %d, want 2", len(got.Promises))
	}
	if got.Promises[0].FuncExists != "func NewFoo()" {
		t.Errorf("Promises[0].FuncExists = %q", got.Promises[0].FuncExists)
	}
}

func TestPhaseSpec_WithPromises_Nil(t *testing.T) {
	yamlData := `name: p\nspec: s\npromises: null\n`
	var spec PhaseSpec
	yaml.Unmarshal([]byte(yamlData), &spec)
	if spec.Promises != nil {
		t.Errorf("Promises should be nil, got %v", spec.Promises)
	}
}

func TestPhaseSpec_WithPromises_MultipleTypes(t *testing.T) {
	spec := PhaseSpec{
		Promises: []Promise{
			{FuncExists: "func A()"},
			{TestExists: "TestA"},
			{FileExists: "a.go"},
			{TestCovers: "A", Test: "TestA"},
		},
	}
	if len(spec.Promises) != 4 {
		t.Fatalf("want 4 promises, got %d", len(spec.Promises))
	}
}

func TestPromise_ZeroValue(t *testing.T) {
	var p Promise
	if p.FuncExists != "" || p.TestExists != "" || p.FileExists != "" || p.TestCovers != "" || p.Test != "" {
		t.Errorf("zero value Promise has non-empty fields: %+v", p)
	}
}

func TestGateResult_WithTestCoversQueue_YAML(t *testing.T) {
	gr := GateResult{
		Passed:          true,
		TestCoversQueue: []string{"NewFoo", "Bar"},
	}
	data, err := yaml.Marshal(gr)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got GateResult
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.TestCoversQueue) != 2 {
		t.Fatalf("len(TestCoversQueue) = %d, want 2", len(got.TestCoversQueue))
	}
}

func TestGateResult_WithTestCoversQueue_JSON(t *testing.T) {
	gr := GateResult{
		Passed:          true,
		TestCoversQueue: []string{"NewFoo"},
	}
	data, _ := json.Marshal(gr)
	var got GateResult
	json.Unmarshal(data, &got)
	if len(got.TestCoversQueue) != 1 || got.TestCoversQueue[0] != "NewFoo" {
		t.Errorf("TestCoversQueue = %v, want [NewFoo]", got.TestCoversQueue)
	}
}

func TestGateResult_TestCoversQueue_Nil(t *testing.T) {
	gr := GateResult{Passed: true}
	data, _ := json.Marshal(gr)
	if strings.Contains(string(data), "test_covers_queue") {
		t.Errorf("nil TestCoversQueue should be omitted from JSON")
	}
}

func TestGateResult_TestCoversQueue_Empty(t *testing.T) {
	gr := GateResult{Passed: true, TestCoversQueue: []string{}}
	data, _ := json.Marshal(gr)
	// empty slice serializes as [] not omitted
	_ = data
}


func TestPhaseSpecJSON(t *testing.T) {
	spec := PhaseSpec{
		Name:       "api-auth",
		Spec:       "Add JWT authentication middleware",
		Files:      []string{"internal/api/auth.go", "internal/api/middleware.go"},
		Verify:     "go test ./internal/api/ -count=1",
		Deps:       []string{"core-types"},
		Complexity: "medium",
		Checkpoints: []Checkpoint{
			{Name: "middleware-struct", Description: "Middleware type with constructor", Test: "go test ./internal/api/ -run TestMiddlewareNew -count=1"},
		},
		Gate: GateSpec{
			Assertions: []GateAssertion{
				{FileExists: "internal/api/auth.go"},
				{Grep: "func NewMiddleware"},
				{TestExists: "TestTokenExpiry"},
			},
			VerifierAgent: nil,
		},
	}

	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}
	var spec2 PhaseSpec
	if err := json.Unmarshal(data, &spec2); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	if spec2.Name != "api-auth" {
		t.Errorf("Name = %q, want %q", spec2.Name, "api-auth")
	}
	if len(spec2.Files) != 2 {
		t.Fatalf("len(Files) = %d, want 2", len(spec2.Files))
	}
	if len(spec2.Gate.Assertions) != 3 {
		t.Fatalf("len(Assertions) = %d, want 3", len(spec2.Gate.Assertions))
	}
}

func TestPhaseSpecYAML(t *testing.T) {
	yamlData := `
name: api-auth
spec: Add JWT authentication middleware
files:
  - internal/api/auth.go
verify: go test ./internal/api/ -count=1
complexity: simple
checkpoints:
  - name: handler
    description: Handler exists
    test: go test -run TestHandler
gate:
  assertions:
    - file_exists: internal/api/auth.go
    - grep: "func NewHandler"
  verifier_agent: true
`
	var spec PhaseSpec
	if err := yaml.Unmarshal([]byte(yamlData), &spec); err != nil {
		t.Fatalf("yaml unmarshal: %v", err)
	}
	if spec.Name != "api-auth" {
		t.Errorf("Name = %q, want %q", spec.Name, "api-auth")
	}
	if spec.Complexity != "simple" {
		t.Errorf("Complexity = %q, want %q", spec.Complexity, "simple")
	}
	if spec.Gate.VerifierAgent == nil || !*spec.Gate.VerifierAgent {
		t.Error("VerifierAgent = false, want true")
	}
	if len(spec.Gate.Assertions) != 2 {
		t.Fatalf("len(Assertions) = %d, want 2", len(spec.Gate.Assertions))
	}
}

func TestDefaultRole(t *testing.T) {
	tests := []struct {
		role string
		want string
	}{
		{"impl", "impl"},
		{"review", "review"},
		{"investigate", "investigate"},
		{"audit", "audit"},
		{"", "impl"},
		{"unknown", "impl"},
		{"IMPL", "impl"},
	}
	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			if got := DefaultRole(tt.role); got != tt.want {
				t.Errorf("DefaultRole(%q) = %q, want %q", tt.role, got, tt.want)
			}
		})
	}
}

func TestPhaseSpec_RoleYAMLRoundTrip(t *testing.T) {
	spec := PhaseSpec{
		Name: "check",
		Role: "review",
		Spec: "Review the code",
	}
	data, err := yaml.Marshal(spec)
	if err != nil {
		t.Fatalf("yaml marshal: %v", err)
	}
	var got PhaseSpec
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatalf("yaml unmarshal: %v", err)
	}
	if got.Role != "review" {
		t.Errorf("Role = %q, want %q", got.Role, "review")
	}

	// Empty role should not appear in YAML output
	spec2 := PhaseSpec{Name: "impl-phase", Spec: "Implement"}
	data2, err := yaml.Marshal(spec2)
	if err != nil {
		t.Fatalf("yaml marshal empty role: %v", err)
	}
	if strings.Contains(string(data2), "role") {
		t.Errorf("empty role should be omitted from YAML, got: %s", data2)
	}
}

func TestDefaultTurnBudget(t *testing.T) {
	tests := []struct {
		complexity string
		want       int
	}{
		{"simple", 50},
		{"medium", 100},
		{"complex", 200},
		{"", 100},
		{"unknown", 100},
	}
	for _, tt := range tests {
		t.Run(tt.complexity, func(t *testing.T) {
			if got := DefaultTurnBudget(tt.complexity); got != tt.want {
				t.Errorf("DefaultTurnBudget(%q) = %d, want %d", tt.complexity, got, tt.want)
			}
		})
	}
}
