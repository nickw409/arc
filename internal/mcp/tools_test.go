package mcp

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/orchestrator"
	"github.com/nwiley/arc/internal/plan"
)

func newTestHandler(t *testing.T) (*handlerContext, string) {
	t.Helper()
	dir := t.TempDir()
	return &handlerContext{
		projectDir: dir,
		arcHome:    dir,
		logger:     slog.New(slog.NewTextHandler(os.Stderr, nil)),
		jobs:       make(map[string]*runJob),
		jobsCtx:    context.Background(),
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

// --- RunStatus tests ---

func TestHandleRunStatusNoJobs(t *testing.T) {
	h, _ := newTestHandler(t)

	result, err := callTool(context.Background(), h, h.handleRunStatus, nil)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(t, result)
	if !strings.Contains(text, "No active runs") {
		t.Fatalf("expected 'No active runs', got: %s", text)
	}
}

func TestHandleRunStatusActiveJob(t *testing.T) {
	h, dir := newTestHandler(t)
	os.MkdirAll(filepath.Join(dir, ".plans", "active"), 0755)

	// Inject a fake running job.
	h.mu.Lock()
	h.jobs["test-plan"] = &runJob{
		PlanName:  "test-plan",
		Cancel:    func() {},
		Done:      make(chan struct{}),
		StartedAt: time.Now(),
	}
	h.mu.Unlock()

	result, err := callTool(context.Background(), h, h.handleRunStatus, map[string]any{
		"plan_name": "test-plan",
	})
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(t, result)
	if !strings.Contains(text, "in progress") {
		t.Fatalf("expected 'in progress', got: %s", text)
	}
}

func TestHandleRunStatusCompletedJob(t *testing.T) {
	h, _ := newTestHandler(t)

	done := make(chan struct{})
	close(done)
	h.mu.Lock()
	h.jobs["done-plan"] = &runJob{
		PlanName:  "done-plan",
		Cancel:    func() {},
		Done:      done,
		StartedAt: time.Now().Add(-5 * time.Second),
		Result: &orchestrator.LaunchResult{
			Status:       "complete",
			PhaseSummary: map[string]string{"impl": "complete"},
		},
	}
	h.mu.Unlock()

	result, err := callTool(context.Background(), h, h.handleRunStatus, map[string]any{
		"plan_name": "done-plan",
	})
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(t, result)
	if !strings.Contains(text, "finished") {
		t.Fatalf("expected 'finished', got: %s", text)
	}
	if !strings.Contains(text, "complete") {
		t.Fatalf("expected 'complete' status, got: %s", text)
	}

	// Job should be cleaned up.
	h.mu.Lock()
	_, exists := h.jobs["done-plan"]
	h.mu.Unlock()
	if exists {
		t.Fatal("expected completed job to be cleaned up")
	}
}

func TestHandleRunStatusFallthrough(t *testing.T) {
	h, dir := newTestHandler(t)
	plansDir := filepath.Join(dir, ".plans", "active")

	plan.Create(plan.CreateOptions{
		PlansDir:     plansDir,
		Name:         "test-plan",
		Phases:       []string{"impl"},
		WorkflowType: "feature",
	})

	// No job in registry — should fall through to arc_status.
	result, err := callTool(context.Background(), h, h.handleRunStatus, map[string]any{
		"plan_name": "test-plan",
	})
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(t, result)
	if !strings.Contains(text, "test-plan") {
		t.Fatalf("expected plan status output, got: %s", text)
	}
}

// --- RunCancel tests ---

func TestHandleRunCancelNoJob(t *testing.T) {
	h, _ := newTestHandler(t)

	result, err := callTool(context.Background(), h, h.handleRunCancel, map[string]any{
		"plan_name": "nonexistent",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for non-existent job")
	}
}

func TestHandleRunCancelRunningJob(t *testing.T) {
	h, _ := newTestHandler(t)

	cancelled := false
	done := make(chan struct{})
	h.mu.Lock()
	h.jobs["cancel-plan"] = &runJob{
		PlanName: "cancel-plan",
		Cancel: func() {
			cancelled = true
			close(done)
		},
		Done:      done,
		StartedAt: time.Now(),
	}
	h.mu.Unlock()

	result, err := callTool(context.Background(), h, h.handleRunCancel, map[string]any{
		"plan_name": "cancel-plan",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", resultText(t, result))
	}
	if !cancelled {
		t.Fatal("expected cancel to be called")
	}
}

func TestHandleRunCancelMissingName(t *testing.T) {
	h, _ := newTestHandler(t)

	result, err := callTool(context.Background(), h, h.handleRunCancel, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing plan_name")
	}
}

// --- validateName tests ---

func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		label   string
		wantErr bool
		errSub  string
	}{
		// valid
		{"my-plan", "plan_name", false, ""},
		{"ab", "plan_name", false, ""},
		{"plan1", "plan_name", false, ""},
		{"a1b2c3", "plan_name", false, ""},
		{"plan-alpha-beta", "plan_name", false, ""},
		// empty
		{"", "plan_name", true, "required"},
		// too long (65 chars)
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa1", "plan_name", true, "too long"},
		// path traversal
		{"../etc", "plan_name", true, "invalid"},
		{"../evil", "plan_name", true, "invalid"},
		// starts with digit
		{"1plan", "plan_name", true, "invalid"},
		// ends with hyphen
		{"plan-", "plan_name", true, "invalid"},
		// starts with hyphen
		{"-plan", "plan_name", true, "invalid"},
		// contains slash
		{"plan/phase", "plan_name", true, "invalid"},
		// contains dot
		{"plan.name", "plan_name", true, "invalid"},
		// single char (valid: single alphanumeric doesn't match pattern which requires len>=2)
		{"a", "plan_name", true, "invalid"},
	}

	for _, tc := range tests {
		t.Run(tc.name+"_"+tc.label, func(t *testing.T) {
			err := validateName(tc.name, tc.label)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("validateName(%q, %q) expected error, got nil", tc.name, tc.label)
				}
				if tc.errSub != "" && !strings.Contains(err.Error(), tc.errSub) {
					t.Fatalf("validateName(%q, %q) error = %q, want substring %q", tc.name, tc.label, err.Error(), tc.errSub)
				}
			} else {
				if err != nil {
					t.Fatalf("validateName(%q, %q) unexpected error: %v", tc.name, tc.label, err)
				}
			}
		})
	}
}

func TestValidateNamePathTraversal(t *testing.T) {
	dangerous := []string{
		"../etc/passwd",
		"../../secret",
		"plan/../other",
		"plan/../../etc",
	}
	for _, name := range dangerous {
		err := validateName(name, "plan_name")
		if err == nil {
			t.Errorf("validateName(%q) should reject path traversal, got nil", name)
		}
	}
}

// --- Manage numeric validation tests ---

func TestHandleManageTestsNonNumeric(t *testing.T) {
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
		"action":    "tests",
		"passing":   "not-a-number",
		"total":     "also-not-a-number",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for non-numeric tests args")
	}
}

func TestHandleManageTestsNegative(t *testing.T) {
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
		"action":    "tests",
		"passing":   float64(-1),
		"total":     float64(10),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for negative passing count")
	}
}

func TestHandleManageIterationNonNumeric(t *testing.T) {
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
		"action":    "iteration",
		"iteration": "not-a-number",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for non-numeric iteration arg")
	}
}

func TestHandleManageIterationNegative(t *testing.T) {
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
		"action":    "iteration",
		"iteration": float64(-5),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for negative iteration value")
	}
}

// --- Run async tests ---

func TestHandleRunAlreadyRunning(t *testing.T) {
	h, dir := newTestHandler(t)
	createApprovedPlan(t, dir, "running-plan")

	// Create .arc.yaml so config loading succeeds.
	os.WriteFile(filepath.Join(dir, ".arc.yaml"), []byte("{}"), 0644)

	// Inject a fake running job.
	h.mu.Lock()
	h.jobs["running-plan"] = &runJob{
		PlanName:  "running-plan",
		Cancel:    func() {},
		Done:      make(chan struct{}),
		StartedAt: time.Now(),
	}
	h.mu.Unlock()

	result, err := callTool(context.Background(), h, h.handleRun, map[string]any{
		"plan_name": "running-plan",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for already running plan")
	}
	text := resultText(t, result)
	if !strings.Contains(text, "already running") {
		t.Fatalf("expected 'already running', got: %s", text)
	}
}

func TestHandleRunStartsAsync(t *testing.T) {
	h, dir := newTestHandler(t)

	// Use a cancellable context for jobsCtx so the background job
	// is cleaned up when the test finishes.
	jobsCtx, jobsCancel := context.WithCancel(context.Background())
	t.Cleanup(jobsCancel)
	h.jobsCtx = jobsCtx

	createApprovedPlan(t, dir, "async-plan")

	// Create .arc.yaml so config loading succeeds.
	os.WriteFile(filepath.Join(dir, ".arc.yaml"), []byte("{}"), 0644)

	result, err := callTool(context.Background(), h, h.handleRun, map[string]any{
		"plan_name": "async-plan",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", resultText(t, result))
	}
	text := resultText(t, result)
	if !strings.Contains(text, "Started run") {
		t.Fatalf("expected 'Started run', got: %s", text)
	}

	// Verify job was registered.
	h.mu.Lock()
	job, exists := h.jobs["async-plan"]
	h.mu.Unlock()
	if !exists {
		t.Fatal("expected job to be registered")
	}

	// Cancel and wait for the background job to stop before TempDir cleanup.
	jobsCancel()
	<-job.Done
}

