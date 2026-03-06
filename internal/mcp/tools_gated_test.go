package mcp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nwiley/arc/internal/plan"
)

// createPlanForGated is a helper that creates a plan with one phase for gated tool tests.
func createPlanForGated(t *testing.T, dir, planName string, phases []string) {
	t.Helper()
	plansDir := filepath.Join(dir, ".plans", "active")
	_, err := plan.Create(plan.CreateOptions{
		PlansDir:     plansDir,
		Name:         planName,
		Phases:       phases,
		WorkflowType: "feature",
	})
	if err != nil {
		t.Fatalf("creating plan: %v", err)
	}
}

// --- arc_plan_add_phase ---

func TestHandlePlanAddPhase(t *testing.T) {
	h, dir := newTestHandler(t)
	createPlanForGated(t, dir, "my-plan", []string{"impl"})

	result, err := callTool(context.Background(), h, h.handlePlanAddPhase, map[string]any{
		"plan_name":  "my-plan",
		"phase_name": "verify",
		"spec":       "Verify all tests pass",
		"verify":     "go test ./...",
		"complexity": "simple",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", resultText(t, result))
	}
	text := resultText(t, result)
	if !strings.Contains(text, "verify") {
		t.Fatalf("expected phase name in result, got: %s", text)
	}

	// Phase directory should exist
	phaseDir := filepath.Join(dir, ".plans", "active", "my-plan", "phases", "verify")
	if _, err := os.Stat(phaseDir); os.IsNotExist(err) {
		t.Fatal("phase directory should exist")
	}
	// spec.yaml should exist
	if _, err := os.Stat(filepath.Join(phaseDir, "spec.yaml")); os.IsNotExist(err) {
		t.Fatal("spec.yaml should exist")
	}
}

func TestHandlePlanAddPhaseDuplicate(t *testing.T) {
	h, dir := newTestHandler(t)
	createPlanForGated(t, dir, "my-plan", []string{"impl"})

	result, err := callTool(context.Background(), h, h.handlePlanAddPhase, map[string]any{
		"plan_name":  "my-plan",
		"phase_name": "impl",
		"spec":       "Implement the feature",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for duplicate phase")
	}
}

func TestHandlePlanAddPhaseMissingSpec(t *testing.T) {
	h, dir := newTestHandler(t)
	createPlanForGated(t, dir, "my-plan", []string{"impl"})

	result, err := callTool(context.Background(), h, h.handlePlanAddPhase, map[string]any{
		"plan_name":  "my-plan",
		"phase_name": "verify",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing spec")
	}
}

func TestHandlePlanAddPhaseMissingPlanName(t *testing.T) {
	h, _ := newTestHandler(t)

	result, err := callTool(context.Background(), h, h.handlePlanAddPhase, map[string]any{
		"phase_name": "verify",
		"spec":       "Verify",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing plan_name")
	}
}

func TestHandlePlanAddPhaseInvalidComplexity(t *testing.T) {
	h, dir := newTestHandler(t)
	createPlanForGated(t, dir, "my-plan", []string{"impl"})

	result, err := callTool(context.Background(), h, h.handlePlanAddPhase, map[string]any{
		"plan_name":  "my-plan",
		"phase_name": "verify",
		"spec":       "Verify",
		"complexity": "extreme",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid complexity")
	}
	text := resultText(t, result)
	if !strings.Contains(text, "complexity") {
		t.Fatalf("expected 'complexity' in error, got: %s", text)
	}
}

func TestHandlePlanAddPhaseWithDeps(t *testing.T) {
	h, dir := newTestHandler(t)
	createPlanForGated(t, dir, "my-plan", []string{"impl"})

	result, err := callTool(context.Background(), h, h.handlePlanAddPhase, map[string]any{
		"plan_name":  "my-plan",
		"phase_name": "verify",
		"spec":       "Verify",
		"deps":       []any{"impl"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", resultText(t, result))
	}
}

func TestHandlePlanAddPhaseWithRole(t *testing.T) {
	h, dir := newTestHandler(t)
	createPlanForGated(t, dir, "my-plan", []string{"impl"})

	result, err := callTool(context.Background(), h, h.handlePlanAddPhase, map[string]any{
		"plan_name":  "my-plan",
		"phase_name": "check",
		"spec":       "Review the code",
		"role":       "review",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", resultText(t, result))
	}

	// Verify role is persisted in spec.yaml
	spec, err := plan.ReadSpec(
		filepath.Join(dir, ".plans", "active"),
		"my-plan", "check",
	)
	if err != nil {
		t.Fatalf("ReadSpec: %v", err)
	}
	if spec.Role != "review" {
		t.Errorf("Role: got %q, want %q", spec.Role, "review")
	}
}

func TestHandlePlanAddPhaseInvalidRole(t *testing.T) {
	h, dir := newTestHandler(t)
	createPlanForGated(t, dir, "my-plan", []string{"impl"})

	result, err := callTool(context.Background(), h, h.handlePlanAddPhase, map[string]any{
		"plan_name":  "my-plan",
		"phase_name": "check",
		"spec":       "Do something",
		"role":       "unknown",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid role")
	}
	text := resultText(t, result)
	if !strings.Contains(text, "role") {
		t.Fatalf("expected 'role' in error, got: %s", text)
	}
}

func TestHandlePlanUpdatePhaseRole(t *testing.T) {
	h, dir := newTestHandler(t)
	createPlanForGated(t, dir, "my-plan", []string{"impl"})

	// Add phase first
	callTool(context.Background(), h, h.handlePlanAddPhase, map[string]any{
		"plan_name":  "my-plan",
		"phase_name": "check",
		"spec":       "Check code",
	})

	// Update just the role
	result, err := callTool(context.Background(), h, h.handlePlanUpdatePhase, map[string]any{
		"plan_name":  "my-plan",
		"phase_name": "check",
		"role":       "audit",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", resultText(t, result))
	}
	text := resultText(t, result)
	if !strings.Contains(text, "role") {
		t.Fatalf("expected 'role' in updated fields, got: %s", text)
	}

	// Verify persisted
	spec, err := plan.ReadSpec(
		filepath.Join(dir, ".plans", "active"),
		"my-plan", "check",
	)
	if err != nil {
		t.Fatalf("ReadSpec: %v", err)
	}
	if spec.Role != "audit" {
		t.Errorf("Role: got %q, want %q", spec.Role, "audit")
	}
}

// --- arc_plan_remove_phase ---

func TestHandlePlanRemovePhase(t *testing.T) {
	h, dir := newTestHandler(t)
	createPlanForGated(t, dir, "my-plan", []string{"impl", "verify"})

	result, err := callTool(context.Background(), h, h.handlePlanRemovePhase, map[string]any{
		"plan_name":  "my-plan",
		"phase_name": "verify",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", resultText(t, result))
	}
	text := resultText(t, result)
	if !strings.Contains(text, "verify") {
		t.Fatalf("expected phase name in result, got: %s", text)
	}

	// Phase directory should be gone
	phaseDir := filepath.Join(dir, ".plans", "active", "my-plan", "phases", "verify")
	if _, err := os.Stat(phaseDir); !os.IsNotExist(err) {
		t.Fatal("phase directory should be removed")
	}
}

func TestHandlePlanRemovePhaseNotFound(t *testing.T) {
	h, dir := newTestHandler(t)
	createPlanForGated(t, dir, "my-plan", []string{"impl"})

	result, err := callTool(context.Background(), h, h.handlePlanRemovePhase, map[string]any{
		"plan_name":  "my-plan",
		"phase_name": "nonexistent",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for nonexistent phase")
	}
}

func TestHandlePlanRemovePhaseMissingArgs(t *testing.T) {
	h, _ := newTestHandler(t)

	result, err := callTool(context.Background(), h, h.handlePlanRemovePhase, map[string]any{
		"plan_name": "my-plan",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing phase_name")
	}
}

// --- arc_plan_update_phase ---

func TestHandlePlanUpdatePhase(t *testing.T) {
	h, dir := newTestHandler(t)
	createPlanForGated(t, dir, "my-plan", []string{"impl"})

	// First add a phase with a spec
	callTool(context.Background(), h, h.handlePlanAddPhase, map[string]any{
		"plan_name":  "my-plan",
		"phase_name": "verify",
		"spec":       "Original spec",
	})

	result, err := callTool(context.Background(), h, h.handlePlanUpdatePhase, map[string]any{
		"plan_name":  "my-plan",
		"phase_name": "verify",
		"spec":       "Updated spec",
		"complexity": "medium",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", resultText(t, result))
	}
	text := resultText(t, result)
	if !strings.Contains(text, "spec") {
		t.Fatalf("expected 'spec' in result, got: %s", text)
	}
}

func TestHandlePlanUpdatePhaseNoFields(t *testing.T) {
	h, dir := newTestHandler(t)
	createPlanForGated(t, dir, "my-plan", []string{"impl"})

	result, err := callTool(context.Background(), h, h.handlePlanUpdatePhase, map[string]any{
		"plan_name":  "my-plan",
		"phase_name": "impl",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error when no fields provided")
	}
}

func TestHandlePlanUpdatePhaseMissingArgs(t *testing.T) {
	h, _ := newTestHandler(t)

	result, err := callTool(context.Background(), h, h.handlePlanUpdatePhase, map[string]any{
		"spec": "something",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing plan_name and phase_name")
	}
}

func TestHandlePlanUpdatePhaseInvalidComplexity(t *testing.T) {
	h, dir := newTestHandler(t)
	createPlanForGated(t, dir, "my-plan", []string{"impl"})

	result, err := callTool(context.Background(), h, h.handlePlanUpdatePhase, map[string]any{
		"plan_name":  "my-plan",
		"phase_name": "impl",
		"complexity": "huge",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid complexity")
	}
}

// --- arc_plan_update_gate ---

func TestHandlePlanUpdateGate(t *testing.T) {
	h, dir := newTestHandler(t)
	createPlanForGated(t, dir, "my-plan", []string{"impl"})

	// Add a phase first so spec.yaml exists
	callTool(context.Background(), h, h.handlePlanAddPhase, map[string]any{
		"plan_name":  "my-plan",
		"phase_name": "verify",
		"spec":       "Verify",
	})

	result, err := callTool(context.Background(), h, h.handlePlanUpdateGate, map[string]any{
		"plan_name":  "my-plan",
		"phase_name": "verify",
		"assertions": []any{"file_exists:cmd/main.go", "grep:func TestMain"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", resultText(t, result))
	}
	text := resultText(t, result)
	if !strings.Contains(text, "2 assertions") {
		t.Fatalf("expected '2 assertions' in result, got: %s", text)
	}
}

func TestHandlePlanUpdateGateWithVerifierAgent(t *testing.T) {
	h, dir := newTestHandler(t)
	createPlanForGated(t, dir, "my-plan", []string{"impl"})

	callTool(context.Background(), h, h.handlePlanAddPhase, map[string]any{
		"plan_name":  "my-plan",
		"phase_name": "verify",
		"spec":       "Verify",
	})

	result, err := callTool(context.Background(), h, h.handlePlanUpdateGate, map[string]any{
		"plan_name":      "my-plan",
		"phase_name":     "verify",
		"assertions":     []any{"test_exists:TestFoo"},
		"verifier_agent": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", resultText(t, result))
	}
}

func TestHandlePlanUpdateGateMissingAssertions(t *testing.T) {
	h, dir := newTestHandler(t)
	createPlanForGated(t, dir, "my-plan", []string{"impl"})

	result, err := callTool(context.Background(), h, h.handlePlanUpdateGate, map[string]any{
		"plan_name":  "my-plan",
		"phase_name": "impl",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing assertions")
	}
}

func TestHandlePlanUpdateGateInvalidFormat(t *testing.T) {
	h, dir := newTestHandler(t)
	createPlanForGated(t, dir, "my-plan", []string{"impl"})

	result, err := callTool(context.Background(), h, h.handlePlanUpdateGate, map[string]any{
		"plan_name":  "my-plan",
		"phase_name": "impl",
		"assertions": []any{"invalid-no-colon"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid assertion format")
	}
	text := resultText(t, result)
	if !strings.Contains(text, "type:target") {
		t.Fatalf("expected format hint in error, got: %s", text)
	}
}

func TestHandlePlanUpdateGateUnknownType(t *testing.T) {
	h, dir := newTestHandler(t)
	createPlanForGated(t, dir, "my-plan", []string{"impl"})

	result, err := callTool(context.Background(), h, h.handlePlanUpdateGate, map[string]any{
		"plan_name":  "my-plan",
		"phase_name": "impl",
		"assertions": []any{"unknown_type:some-target"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for unknown assertion type")
	}
	text := resultText(t, result)
	if !strings.Contains(text, "unknown assertion type") {
		t.Fatalf("expected 'unknown assertion type' in error, got: %s", text)
	}
}

// --- arc_plan_update_deps ---

func TestHandlePlanUpdateDeps(t *testing.T) {
	h, dir := newTestHandler(t)
	createPlanForGated(t, dir, "my-plan", []string{"impl", "verify", "qa"})

	result, err := callTool(context.Background(), h, h.handlePlanUpdateDeps, map[string]any{
		"plan_name":  "my-plan",
		"phase_name": "qa",
		"deps":       []any{"impl", "verify"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", resultText(t, result))
	}
	text := resultText(t, result)
	if !strings.Contains(text, "impl") || !strings.Contains(text, "verify") {
		t.Fatalf("expected deps in result, got: %s", text)
	}
}

func TestHandlePlanUpdateDepsEmpty(t *testing.T) {
	h, dir := newTestHandler(t)
	createPlanForGated(t, dir, "my-plan", []string{"impl", "verify"})

	result, err := callTool(context.Background(), h, h.handlePlanUpdateDeps, map[string]any{
		"plan_name":  "my-plan",
		"phase_name": "verify",
		"deps":       []any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", resultText(t, result))
	}
	text := resultText(t, result)
	if !strings.Contains(text, "Cleared") {
		t.Fatalf("expected 'Cleared' for empty deps, got: %s", text)
	}
}

func TestHandlePlanUpdateDepsMissingArgs(t *testing.T) {
	h, _ := newTestHandler(t)

	result, err := callTool(context.Background(), h, h.handlePlanUpdateDeps, map[string]any{
		"deps": []any{"impl"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing plan_name and phase_name")
	}
}

// --- arc_plan_show_spec ---

func TestHandlePlanShowSpec(t *testing.T) {
	h, dir := newTestHandler(t)
	createPlanForGated(t, dir, "my-plan", []string{"impl"})

	// Add phase with spec
	callTool(context.Background(), h, h.handlePlanAddPhase, map[string]any{
		"plan_name":  "my-plan",
		"phase_name": "verify",
		"spec":       "Verify all tests pass",
		"verify":     "go test ./...",
		"complexity": "simple",
	})

	result, err := callTool(context.Background(), h, h.handlePlanShowSpec, map[string]any{
		"plan_name":  "my-plan",
		"phase_name": "verify",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", resultText(t, result))
	}
	text := resultText(t, result)
	if !strings.Contains(text, "Verify all tests pass") {
		t.Fatalf("expected spec text in output, got: %s", text)
	}
	if !strings.Contains(text, "go test") {
		t.Fatalf("expected verify text in output, got: %s", text)
	}
}


func TestHandlePlanShowSpecMissingArgs(t *testing.T) {
	h, _ := newTestHandler(t)

	result, err := callTool(context.Background(), h, h.handlePlanShowSpec, map[string]any{
		"plan_name": "my-plan",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing phase_name")
	}
}

// --- round-trip: add phase, update gate, show spec ---

func TestGatedPhaseRoundTrip(t *testing.T) {
	h, dir := newTestHandler(t)
	createPlanForGated(t, dir, "my-plan", []string{"impl"})

	// Add phase
	r, err := callTool(context.Background(), h, h.handlePlanAddPhase, map[string]any{
		"plan_name":  "my-plan",
		"phase_name": "verify",
		"spec":       "Run the full test suite",
		"verify":     "go test ./...",
		"complexity": "simple",
		"files":      []any{"internal/runner/runner.go"},
		"deps":       []any{"impl"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.IsError {
		t.Fatalf("add phase: %s", resultText(t, r))
	}

	// Update gate
	r, err = callTool(context.Background(), h, h.handlePlanUpdateGate, map[string]any{
		"plan_name":  "my-plan",
		"phase_name": "verify",
		"assertions": []any{
			"file_exists:internal/runner/runner.go",
			"test_exists:TestRunner",
		},
		"verifier_agent": false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.IsError {
		t.Fatalf("update gate: %s", resultText(t, r))
	}

	// Show spec and verify round-trip
	r, err = callTool(context.Background(), h, h.handlePlanShowSpec, map[string]any{
		"plan_name":  "my-plan",
		"phase_name": "verify",
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.IsError {
		t.Fatalf("show spec: %s", resultText(t, r))
	}

	text := resultText(t, r)
	if !strings.Contains(text, "Run the full test suite") {
		t.Fatalf("expected spec text, got: %s", text)
	}
	if !strings.Contains(text, "file_exists") {
		t.Fatalf("expected gate assertion type, got: %s", text)
	}
	if !strings.Contains(text, "TestRunner") {
		t.Fatalf("expected gate assertion target, got: %s", text)
	}
}
