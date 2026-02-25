package mcp

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/plan"
)

func newTestHandler(t *testing.T) (*handlerContext, string) {
	t.Helper()
	dir := t.TempDir()
	return &handlerContext{
		projectDir: dir,
		arcHome:    dir,
		logger:     slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}, dir
}

func callTool(ctx context.Context, h *handlerContext, handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error), args map[string]any) (*mcp.CallToolResult, error) {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = args
	return handler(ctx, req)
}

func resultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if result == nil {
		t.Fatal("result is nil")
	}
	if len(result.Content) == 0 {
		t.Fatal("result has no content")
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("result content is not TextContent: %T", result.Content[0])
	}
	return tc.Text
}

// --- Status tests ---

func TestHandleStatusNoPlans(t *testing.T) {
	h, dir := newTestHandler(t)
	// Create empty .plans/active directory
	os.MkdirAll(filepath.Join(dir, ".plans", "active"), 0755)

	result, err := callTool(context.Background(), h, h.handleStatus, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", resultText(t, result))
	}
}

func TestHandleStatusWithPlan(t *testing.T) {
	h, dir := newTestHandler(t)
	plansDir := filepath.Join(dir, ".plans", "active")

	_, err := plan.Create(plan.CreateOptions{
		PlansDir:     plansDir,
		Name:         "test-plan",
		Phases:       []string{"impl"},
		WorkflowType: "feature",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := callTool(context.Background(), h, h.handleStatus, map[string]any{
		"plan_name": "test-plan",
	})
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(t, result)
	if !strings.Contains(text, "test-plan") {
		t.Fatalf("status output should contain plan name, got: %s", text)
	}
}

// --- Plan tests ---

func TestHandlePlanCreate(t *testing.T) {
	h, dir := newTestHandler(t)
	os.MkdirAll(filepath.Join(dir, ".plans", "active"), 0755)

	result, err := callTool(context.Background(), h, h.handlePlan, map[string]any{
		"name":          "my-plan",
		"workflow_type": "feature",
		"phases":        []any{"qa", "impl"},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(t, result)
	if !strings.Contains(text, "my-plan") {
		t.Fatalf("expected plan name in result, got: %s", text)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", text)
	}

	// Verify plan was actually created
	planDir := filepath.Join(dir, ".plans", "active", "my-plan")
	if _, err := os.Stat(filepath.Join(planDir, "plan.json")); os.IsNotExist(err) {
		t.Fatal("plan.json should exist")
	}
}

func TestHandlePlanMissingName(t *testing.T) {
	h, _ := newTestHandler(t)

	result, err := callTool(context.Background(), h, h.handlePlan, map[string]any{
		"workflow_type": "feature",
		"phases":        []any{"impl"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing name")
	}
}

func TestHandlePlanMissingPhases(t *testing.T) {
	h, _ := newTestHandler(t)

	result, err := callTool(context.Background(), h, h.handlePlan, map[string]any{
		"name":          "my-plan",
		"workflow_type": "feature",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing phases")
	}
}

// --- Manage tests ---

func TestHandleManageComplete(t *testing.T) {
	h, dir := newTestHandler(t)
	plansDir := filepath.Join(dir, ".plans", "active")

	_, err := plan.Create(plan.CreateOptions{
		PlansDir:     plansDir,
		Name:         "test-plan",
		Phases:       []string{"impl"},
		WorkflowType: "feature",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := callTool(context.Background(), h, h.handleManage, map[string]any{
		"plan_name": "test-plan",
		"phase":     "impl",
		"action":    "complete",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", resultText(t, result))
	}
	text := resultText(t, result)
	if !strings.Contains(text, "complete") {
		t.Fatalf("expected 'complete' in result, got: %s", text)
	}
}

func TestHandleManageShow(t *testing.T) {
	h, dir := newTestHandler(t)
	plansDir := filepath.Join(dir, ".plans", "active")

	_, err := plan.Create(plan.CreateOptions{
		PlansDir:     plansDir,
		Name:         "test-plan",
		Phases:       []string{"impl"},
		WorkflowType: "feature",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := callTool(context.Background(), h, h.handleManage, map[string]any{
		"plan_name": "test-plan",
		"phase":     "impl",
		"action":    "show",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", resultText(t, result))
	}
	text := resultText(t, result)
	if !strings.Contains(text, "impl") {
		t.Fatalf("show output should contain phase name, got: %s", text)
	}
}

func TestHandleManageDeferNoReason(t *testing.T) {
	h, dir := newTestHandler(t)
	plansDir := filepath.Join(dir, ".plans", "active")

	plan.Create(plan.CreateOptions{
		PlansDir:     plansDir,
		Name:         "test-plan",
		Phases:       []string{"impl"},
		WorkflowType: "feature",
	})

	result, err := callTool(context.Background(), h, h.handleManage, map[string]any{
		"plan_name": "test-plan",
		"phase":     "impl",
		"action":    "defer",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for defer without reason")
	}
}

func TestHandleManageDeferWithReason(t *testing.T) {
	h, dir := newTestHandler(t)
	plansDir := filepath.Join(dir, ".plans", "active")

	plan.Create(plan.CreateOptions{
		PlansDir:     plansDir,
		Name:         "test-plan",
		Phases:       []string{"impl"},
		WorkflowType: "feature",
	})

	result, err := callTool(context.Background(), h, h.handleManage, map[string]any{
		"plan_name": "test-plan",
		"phase":     "impl",
		"action":    "defer",
		"reason":    "waiting on API design",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", resultText(t, result))
	}
}

func TestHandleManageInvalidAction(t *testing.T) {
	h, _ := newTestHandler(t)

	result, err := callTool(context.Background(), h, h.handleManage, map[string]any{
		"plan_name": "test-plan",
		"phase":     "impl",
		"action":    "explode",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid action")
	}
	text := resultText(t, result)
	if !strings.Contains(text, "unknown action") {
		t.Fatalf("expected 'unknown action' in error, got: %s", text)
	}
}

func TestHandleManageNote(t *testing.T) {
	h, dir := newTestHandler(t)
	plansDir := filepath.Join(dir, ".plans", "active")

	plan.Create(plan.CreateOptions{
		PlansDir:     plansDir,
		Name:         "test-plan",
		Phases:       []string{"impl"},
		WorkflowType: "feature",
	})

	result, err := callTool(context.Background(), h, h.handleManage, map[string]any{
		"plan_name": "test-plan",
		"phase":     "impl",
		"action":    "note",
		"note":      "needs attention",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", resultText(t, result))
	}
}

// --- Guide tests ---

func TestHandleGuideFullGuide(t *testing.T) {
	h, _ := newTestHandler(t)

	result, err := callTool(context.Background(), h, h.handleGuide, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", resultText(t, result))
	}
	text := resultText(t, result)
	if len(text) < 100 {
		t.Fatalf("guide output too short: %d chars", len(text))
	}
}

func TestHandleGuideSection(t *testing.T) {
	h, _ := newTestHandler(t)

	result, err := callTool(context.Background(), h, h.handleGuide, map[string]any{
		"section": "workflows",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", resultText(t, result))
	}
}

func TestHandleGuideInvalidSection(t *testing.T) {
	h, _ := newTestHandler(t)

	result, err := callTool(context.Background(), h, h.handleGuide, map[string]any{
		"section": "nonexistent",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid section")
	}
}

// --- ListPlans tests ---

func TestHandleListPlansEmpty(t *testing.T) {
	h, dir := newTestHandler(t)
	os.MkdirAll(filepath.Join(dir, ".plans", "active"), 0755)

	result, err := callTool(context.Background(), h, h.handleListPlans, nil)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(t, result)
	if !strings.Contains(text, "No active plans") {
		t.Fatalf("expected 'No active plans', got: %s", text)
	}
}

func TestHandleListPlansWithPlans(t *testing.T) {
	h, dir := newTestHandler(t)
	plansDir := filepath.Join(dir, ".plans", "active")

	plan.Create(plan.CreateOptions{
		PlansDir:     plansDir,
		Name:         "plan-alpha",
		Phases:       []string{"impl"},
		WorkflowType: "feature",
	})
	plan.Create(plan.CreateOptions{
		PlansDir:     plansDir,
		Name:         "plan-beta",
		Phases:       []string{"fix"},
		WorkflowType: "bugfix",
	})

	result, err := callTool(context.Background(), h, h.handleListPlans, nil)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(t, result)
	if !strings.Contains(text, "plan-alpha") {
		t.Fatalf("list should contain plan-alpha, got: %s", text)
	}
	if !strings.Contains(text, "plan-beta") {
		t.Fatalf("list should contain plan-beta, got: %s", text)
	}
}

func TestHandleListPlansNoDir(t *testing.T) {
	h, _ := newTestHandler(t)
	// Don't create .plans/active — directory doesn't exist

	result, err := callTool(context.Background(), h, h.handleListPlans, nil)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(t, result)
	if !strings.Contains(text, "No active plans") {
		t.Fatalf("expected 'No active plans' when dir missing, got: %s", text)
	}
}

// --- Archive tests ---

func TestHandleArchive(t *testing.T) {
	h, dir := newTestHandler(t)
	plansDir := filepath.Join(dir, ".plans", "active")
	archiveDir := filepath.Join(dir, ".plans", "archive")
	os.MkdirAll(archiveDir, 0755)

	plan.Create(plan.CreateOptions{
		PlansDir:     plansDir,
		Name:         "done-plan",
		Phases:       []string{"impl"},
		WorkflowType: "feature",
	})

	// Mark phase complete so archive succeeds
	plan.ManageComplete(plan.ManageOptions{
		PlansDir: plansDir,
		PlanName: "done-plan",
		Phase:    "impl",
	})

	result, err := callTool(context.Background(), h, h.handleArchive, map[string]any{
		"plan_name": "done-plan",
		"force":     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", resultText(t, result))
	}

	// Plan should no longer be in active
	if _, err := os.Stat(filepath.Join(plansDir, "done-plan")); !os.IsNotExist(err) {
		t.Fatal("plan should be removed from active directory")
	}
}

func TestHandleArchiveMissingName(t *testing.T) {
	h, _ := newTestHandler(t)

	result, err := callTool(context.Background(), h, h.handleArchive, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing plan_name")
	}
}

// --- Run tests ---

func TestHandleRunRequiresReview(t *testing.T) {
	h, dir := newTestHandler(t)
	plansDir := filepath.Join(dir, ".plans", "active")

	plan.Create(plan.CreateOptions{
		PlansDir:     plansDir,
		Name:         "test-plan",
		Phases:       []string{"impl"},
		WorkflowType: "feature",
	})

	result, err := callTool(context.Background(), h, h.handleRun, map[string]any{
		"plan_name": "test-plan",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for unreviewed plan")
	}
	text := resultText(t, result)
	if !strings.Contains(text, "review") {
		t.Fatalf("error should mention review, got: %s", text)
	}
}

func TestHandleRunMissingPlan(t *testing.T) {
	h, dir := newTestHandler(t)
	os.MkdirAll(filepath.Join(dir, ".plans", "active"), 0755)

	result, err := callTool(context.Background(), h, h.handleRun, map[string]any{
		"plan_name": "nonexistent",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing plan")
	}
}

// --- Iterate tests ---

func TestHandleIterateMissingPhase(t *testing.T) {
	h, dir := newTestHandler(t)
	plansDir := filepath.Join(dir, ".plans", "active")

	plan.Create(plan.CreateOptions{
		PlansDir:     plansDir,
		Name:         "test-plan",
		Phases:       []string{"impl"},
		WorkflowType: "feature",
	})

	result, err := callTool(context.Background(), h, h.handleIterate, map[string]any{
		"plan_name":  "test-plan",
		"phase_name": "nonexistent",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing phase")
	}
}

func TestHandleIterateMissingArgs(t *testing.T) {
	h, _ := newTestHandler(t)

	result, err := callTool(context.Background(), h, h.handleIterate, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing args")
	}
}

// --- Stdout isolation test ---

func TestHandlersWriteToBufferNotStdout(t *testing.T) {
	h, dir := newTestHandler(t)
	plansDir := filepath.Join(dir, ".plans", "active")

	plan.Create(plan.CreateOptions{
		PlansDir:     plansDir,
		Name:         "test-plan",
		Phases:       []string{"impl"},
		WorkflowType: "feature",
	})

	// Capture stdout — the handler should write to buffer, not stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	callTool(context.Background(), h, h.handleStatus, map[string]any{
		"plan_name": "test-plan",
	})

	w.Close()
	os.Stdout = oldStdout

	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	r.Close()

	if n > 0 {
		t.Fatalf("handler wrote %d bytes to stdout — should use buffer instead", n)
	}
}

// --- Manage missing required fields ---

func TestHandleManageMissingFields(t *testing.T) {
	h, _ := newTestHandler(t)

	result, err := callTool(context.Background(), h, h.handleManage, map[string]any{
		"plan_name": "test-plan",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing phase and action")
	}
}

// --- Helper to create a plan with approved review status ---

func createApprovedPlan(t *testing.T, dir, planName string) {
	t.Helper()
	plansDir := filepath.Join(dir, ".plans", "active")

	_, err := plan.Create(plan.CreateOptions{
		PlansDir:     plansDir,
		Name:         planName,
		Phases:       []string{"impl"},
		WorkflowType: "feature",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Set review status to approved
	planDir := filepath.Join(plansDir, planName)
	metaBytes, _ := os.ReadFile(filepath.Join(planDir, "plan.json"))
	var meta arc.PlanMeta
	json.Unmarshal(metaBytes, &meta)
	meta.ReviewStatus = "approved"
	data, _ := json.MarshalIndent(meta, "", "  ")
	os.WriteFile(filepath.Join(planDir, "plan.json"), data, 0644)
}
