package arc

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

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
			VerifierAgent: false,
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
	if !spec.Gate.VerifierAgent {
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
