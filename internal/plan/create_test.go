package plan

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/nwiley/arc/internal/arc"
)

func TestCreatePlanStructure(t *testing.T) {
	dir := t.TempDir()

	meta, err := Create(CreateOptions{
		PlansDir:     dir,
		Name:         "my-plan",
		Phases:       []string{"qa", "impl"},
		WorkflowType: "feature",
	})
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}

	if meta == nil {
		t.Fatal("Create returned nil PlanMeta")
	}

	// Check directories exist
	dirs := []string{
		"my-plan/phases/qa",
		"my-plan/phases/impl",
	}
	for _, d := range dirs {
		full := filepath.Join(dir, d)
		if _, err := os.Stat(full); os.IsNotExist(err) {
			t.Fatalf("expected directory %s to exist", d)
		}
	}

	// Check files exist
	files := []string{
		"my-plan/plan.json",
		"my-plan/session_id",
		"my-plan/phases/qa/state.json",
		"my-plan/phases/qa/plan.md",
		"my-plan/phases/impl/state.json",
		"my-plan/phases/impl/plan.md",
	}
	for _, f := range files {
		full := filepath.Join(dir, f)
		if _, err := os.Stat(full); os.IsNotExist(err) {
			t.Fatalf("expected file %s to exist", f)
		}
	}
}

func TestCreatePlanDependencies(t *testing.T) {
	dir := t.TempDir()

	meta, err := Create(CreateOptions{
		PlansDir:     dir,
		Name:         "my-plan",
		Phases:       []string{"core", "api"},
		WorkflowType: "feature",
	})
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}

	// Dependencies["api"] == ["core"]
	apiDeps := meta.Dependencies["api"]
	if len(apiDeps) != 1 || apiDeps[0] != "core" {
		t.Fatalf("Dependencies[api] = %v, want [core]", apiDeps)
	}
}

func TestCreatePlanCopiesWorkflow(t *testing.T) {
	dir := t.TempDir()

	_, err := Create(CreateOptions{
		PlansDir:     dir,
		Name:         "my-plan",
		Phases:       []string{"fix"},
		WorkflowType: "bugfix",
	})
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}

	// For non-feature workflows, workflow.yaml should be copied to plan dir
	workflowPath := filepath.Join(dir, "my-plan", "workflow.yaml")
	data, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("workflow.yaml should exist for bugfix type: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("workflow.yaml should have content")
	}
}

func TestCreatePlanNoWorkflowCopyForFeature(t *testing.T) {
	dir := t.TempDir()

	_, err := Create(CreateOptions{
		PlansDir:     dir,
		Name:         "my-plan",
		Phases:       []string{"impl"},
		WorkflowType: "feature",
	})
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}

	// Feature is default, no workflow.yaml copied
	workflowPath := filepath.Join(dir, "my-plan", "workflow.yaml")
	if _, err := os.Stat(workflowPath); !os.IsNotExist(err) {
		t.Fatal("workflow.yaml should NOT exist for feature type")
	}
}

func TestCreatePlanDuplicateName(t *testing.T) {
	dir := t.TempDir()

	_, err := Create(CreateOptions{
		PlansDir:     dir,
		Name:         "my-plan",
		Phases:       []string{"impl"},
		WorkflowType: "feature",
	})
	if err != nil {
		t.Fatalf("first Create error: %v", err)
	}

	_, err = Create(CreateOptions{
		PlansDir:     dir,
		Name:         "my-plan",
		Phases:       []string{"impl"},
		WorkflowType: "feature",
	})
	if err == nil {
		t.Fatal("expected error for duplicate plan name, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("error = %q, want containing %q", err.Error(), "already exists")
	}
}

func TestCreatePlanInvalidNameUppercase(t *testing.T) {
	dir := t.TempDir()

	_, err := Create(CreateOptions{
		PlansDir:     dir,
		Name:         "My-Plan",
		Phases:       []string{"impl"},
		WorkflowType: "feature",
	})
	if err == nil {
		t.Fatal("expected error for uppercase plan name, got nil")
	}
	if !strings.Contains(err.Error(), "invalid plan name") {
		t.Fatalf("error = %q, want containing %q", err.Error(), "invalid plan name")
	}
}

func TestCreatePlanInvalidNameLeadingHyphen(t *testing.T) {
	dir := t.TempDir()

	_, err := Create(CreateOptions{
		PlansDir:     dir,
		Name:         "-my-plan",
		Phases:       []string{"impl"},
		WorkflowType: "feature",
	})
	if err == nil {
		t.Fatal("expected error for leading hyphen, got nil")
	}
	if !strings.Contains(err.Error(), "invalid plan name") {
		t.Fatalf("error = %q, want containing %q", err.Error(), "invalid plan name")
	}
}

func TestCreatePlanInvalidNameTrailingHyphen(t *testing.T) {
	dir := t.TempDir()

	_, err := Create(CreateOptions{
		PlansDir:     dir,
		Name:         "my-plan-",
		Phases:       []string{"impl"},
		WorkflowType: "feature",
	})
	if err == nil {
		t.Fatal("expected error for trailing hyphen, got nil")
	}
	if !strings.Contains(err.Error(), "invalid plan name") {
		t.Fatalf("error = %q, want containing %q", err.Error(), "invalid plan name")
	}
}

func TestCreatePlanInvalidNameSpecialChars(t *testing.T) {
	dir := t.TempDir()

	_, err := Create(CreateOptions{
		PlansDir:     dir,
		Name:         "my plan!",
		Phases:       []string{"impl"},
		WorkflowType: "feature",
	})
	if err == nil {
		t.Fatal("expected error for special chars in name, got nil")
	}
	if !strings.Contains(err.Error(), "invalid plan name") {
		t.Fatalf("error = %q, want containing %q", err.Error(), "invalid plan name")
	}
}

func TestCreatePlanNoPhases(t *testing.T) {
	dir := t.TempDir()

	_, err := Create(CreateOptions{
		PlansDir:     dir,
		Name:         "my-plan",
		Phases:       []string{},
		WorkflowType: "feature",
	})
	if err == nil {
		t.Fatal("expected error for empty phases, got nil")
	}
	if !strings.Contains(err.Error(), "no phases specified") {
		t.Fatalf("error = %q, want containing %q", err.Error(), "no phases specified")
	}
}

func TestCreatePlanSingleCharName(t *testing.T) {
	dir := t.TempDir()

	_, err := Create(CreateOptions{
		PlansDir:     dir,
		Name:         "a",
		Phases:       []string{"impl"},
		WorkflowType: "feature",
	})
	if err == nil {
		t.Fatal("expected error for single char name, got nil")
	}
	if !strings.Contains(err.Error(), "invalid plan name") {
		t.Fatalf("error = %q, want containing %q", err.Error(), "invalid plan name")
	}
}

func TestCreatePlanDuplicatePhases(t *testing.T) {
	dir := t.TempDir()

	_, err := Create(CreateOptions{
		PlansDir:     dir,
		Name:         "my-plan",
		Phases:       []string{"core", "core"},
		WorkflowType: "feature",
	})
	if err == nil {
		t.Fatal("expected error for duplicate phases, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate phase") {
		t.Fatalf("error = %q, want containing %q", err.Error(), "duplicate phase")
	}
}

func TestCreatePlanSessionIdFormat(t *testing.T) {
	dir := t.TempDir()

	_, err := Create(CreateOptions{
		PlansDir:     dir,
		Name:         "my-plan",
		Phases:       []string{"impl"},
		WorkflowType: "feature",
	})
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "my-plan", "session_id"))
	if err != nil {
		t.Fatalf("ReadFile session_id error: %v", err)
	}

	sessionID := strings.TrimSpace(string(data))
	uuidV4Re := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !uuidV4Re.MatchString(sessionID) {
		t.Fatalf("session_id = %q, does not match UUID v4 format", sessionID)
	}
}

func TestCreatePlanVeryLongName(t *testing.T) {
	dir := t.TempDir()

	// 256-character valid name (starts with letter, all lowercase alphanumeric)
	name := "a" + strings.Repeat("b", 254) + "c"

	_, err := Create(CreateOptions{
		PlansDir:     dir,
		Name:         name,
		Phases:       []string{"impl"},
		WorkflowType: "feature",
	})
	if err != nil {
		t.Fatalf("Create with long name should not error: %v", err)
	}
}

func TestCreatePlanInvalidWorkflowType(t *testing.T) {
	dir := t.TempDir()

	_, err := Create(CreateOptions{
		PlansDir:     dir,
		Name:         "my-plan",
		Phases:       []string{"impl"},
		WorkflowType: "nonexistent",
	})
	if err == nil {
		t.Fatal("expected error for invalid workflow type, got nil")
	}
}

func TestCreatePlanEmptyWorkflowType(t *testing.T) {
	dir := t.TempDir()

	meta, err := Create(CreateOptions{
		PlansDir:     dir,
		Name:         "my-plan",
		Phases:       []string{"impl"},
		WorkflowType: "",
	})
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}

	// Empty workflow type should default to "feature"
	if meta.WorkflowType != "feature" {
		t.Fatalf("WorkflowType = %q, want %q (default)", meta.WorkflowType, "feature")
	}

	// No workflow.yaml should be copied (feature is default)
	workflowPath := filepath.Join(dir, "my-plan", "workflow.yaml")
	if _, err := os.Stat(workflowPath); !os.IsNotExist(err) {
		t.Fatal("workflow.yaml should NOT exist for empty/feature type")
	}
}

func TestCreatePlanMinTwoChars(t *testing.T) {
	dir := t.TempDir()

	_, err := Create(CreateOptions{
		PlansDir:     dir,
		Name:         "ab",
		Phases:       []string{"impl"},
		WorkflowType: "feature",
	})
	if err != nil {
		t.Fatalf("Create with 2-char name should not error: %v", err)
	}
}

func TestCreatePlanInvestigationType(t *testing.T) {
	dir := t.TempDir()

	meta, err := Create(CreateOptions{
		PlansDir:     dir,
		Name:         "my-investigation",
		Phases:       []string{"analyze"},
		WorkflowType: "investigation",
	})
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}

	if meta.WorkflowType != "investigation" {
		t.Fatalf("WorkflowType = %q, want %q", meta.WorkflowType, "investigation")
	}

	// Workflow.yaml should exist for non-feature types
	workflowPath := filepath.Join(dir, "my-investigation", "workflow.yaml")
	data, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("workflow.yaml should exist for investigation type: %v", err)
	}
	if !strings.Contains(string(data), "investigation") {
		t.Fatal("workflow.yaml should contain investigation workflow content")
	}

	// State.json for "analyze" phase should have investigation workflow type
	stateData, err := os.ReadFile(filepath.Join(dir, "my-investigation", "phases", "analyze", "state.json"))
	if err != nil {
		t.Fatalf("ReadFile state.json error: %v", err)
	}
	var state arc.PhaseState
	if err := json.Unmarshal(stateData, &state); err != nil {
		t.Fatalf("Unmarshal state.json error: %v", err)
	}
	if state.WorkflowType != "investigation" {
		t.Fatalf("state.WorkflowType = %q, want %q", state.WorkflowType, "investigation")
	}
}

func TestCreatePlanStateJsonNonnullSlices(t *testing.T) {
	dir := t.TempDir()

	_, err := Create(CreateOptions{
		PlansDir:     dir,
		Name:         "my-plan",
		Phases:       []string{"impl"},
		WorkflowType: "feature",
	})
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}

	stateData, err := os.ReadFile(filepath.Join(dir, "my-plan", "phases", "impl", "state.json"))
	if err != nil {
		t.Fatalf("ReadFile state.json error: %v", err)
	}

	// Verify empty slices are [] not null
	if bytes.Contains(stateData, []byte(`"test_files":null`)) {
		t.Fatal("test_files should be [] not null")
	}
	if bytes.Contains(stateData, []byte(`"verdicts_history":null`)) {
		t.Fatal("verdicts_history should be [] not null")
	}
	if bytes.Contains(stateData, []byte(`"disputes":null`)) {
		t.Fatal("disputes should be [] not null")
	}
}
