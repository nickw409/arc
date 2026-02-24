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

// --- PlanContent and CustomWorkflow tests ---

func TestCreateOptions_PlanContent(t *testing.T) {
	dir := t.TempDir()

	customContent := "# Custom Phase Content\n\nThis is custom plan.md content for phase1."
	_, err := Create(CreateOptions{
		PlansDir:     dir,
		Name:         "pc-test",
		Phases:       []string{"phase1"},
		WorkflowType: "feature",
		PlanContent:  map[string]string{"phase1": customContent},
	})
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}

	planMD, err := os.ReadFile(filepath.Join(dir, "pc-test", "phases", "phase1", "plan.md"))
	if err != nil {
		t.Fatalf("ReadFile plan.md error: %v", err)
	}
	if string(planMD) != customContent {
		t.Errorf("plan.md = %q, want %q", string(planMD), customContent)
	}
}

func TestCreateOptions_CustomWorkflow(t *testing.T) {
	dir := t.TempDir()

	workflowYAML := []byte(`name: custom
version: 1
description: Custom workflow

states:
  - name: impl
    description: Implement
    prompt: prompts/feature/impl.md
    next: impl_review

  - name: impl_review
    description: Review impl
    prompt: prompts/feature/impl-review.md
    verdicts:
      - approved
      - concerns
    next:
      approved: complete
      concerns: impl

  - name: complete
    description: Task completed
    prompt: prompts/common/complete.md

  - name: blocked
    description: Task blocked
    prompt: prompts/common/blocked.md

entry_state: impl
terminal_states: [complete, blocked]
`)

	// Note: "custom" is not a built-in workflow type, so we must use a
	// valid workflow type for the validation check. The CustomWorkflow
	// content overrides whatever built-in workflow would have been copied.
	// We use "direct" here since it's a valid type.
	meta, err := Create(CreateOptions{
		PlansDir:       dir,
		Name:           "cw-test",
		Phases:         []string{"impl"},
		WorkflowType:   "direct",
		CustomWorkflow: workflowYAML,
	})
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}

	// workflow.yaml should be written with custom content
	workflowPath := filepath.Join(dir, "cw-test", "workflow.yaml")
	data, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("workflow.yaml should exist: %v", err)
	}
	if !bytes.Equal(data, workflowYAML) {
		t.Errorf("workflow.yaml content mismatch")
	}

	// state.json should have the WorkflowType we set
	stateData, err := os.ReadFile(filepath.Join(dir, "cw-test", "phases", "impl", "state.json"))
	if err != nil {
		t.Fatalf("ReadFile state.json error: %v", err)
	}
	var state arc.PhaseState
	if err := json.Unmarshal(stateData, &state); err != nil {
		t.Fatalf("Unmarshal state.json error: %v", err)
	}
	if state.WorkflowType != "direct" {
		t.Errorf("state.WorkflowType = %q, want %q", state.WorkflowType, "direct")
	}
	_ = meta
}

func TestCreateOptions_PlanContentPartial(t *testing.T) {
	dir := t.TempDir()

	customContent := "# Custom for phase-a\n\nCustom content."
	_, err := Create(CreateOptions{
		PlansDir:     dir,
		Name:         "partial-test",
		Phases:       []string{"phase-a", "phase-b", "phase-c"},
		WorkflowType: "feature",
		PlanContent: map[string]string{
			"phase-a": customContent,
			"phase-b": "# Custom for phase-b",
			// phase-c has no entry — should use default template
		},
	})
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}

	// phase-a uses custom content
	mdA, err := os.ReadFile(filepath.Join(dir, "partial-test", "phases", "phase-a", "plan.md"))
	if err != nil {
		t.Fatalf("ReadFile plan.md for phase-a error: %v", err)
	}
	if string(mdA) != customContent {
		t.Errorf("phase-a plan.md = %q, want custom content", string(mdA))
	}

	// phase-b uses custom content
	mdB, err := os.ReadFile(filepath.Join(dir, "partial-test", "phases", "phase-b", "plan.md"))
	if err != nil {
		t.Fatalf("ReadFile plan.md for phase-b error: %v", err)
	}
	if string(mdB) != "# Custom for phase-b" {
		t.Errorf("phase-b plan.md = %q, want custom content", string(mdB))
	}

	// phase-c uses default template
	mdC, err := os.ReadFile(filepath.Join(dir, "partial-test", "phases", "phase-c", "plan.md"))
	if err != nil {
		t.Fatalf("ReadFile plan.md for phase-c error: %v", err)
	}
	if strings.Contains(string(mdC), "Custom") {
		t.Errorf("phase-c plan.md should use default template, got: %s", string(mdC))
	}
	// Should have default template content
	if !strings.Contains(string(mdC), "Phase:") {
		t.Errorf("phase-c plan.md should contain default template content")
	}
}

func TestCreateOptions_CustomWorkflowInvalidYAML(t *testing.T) {
	dir := t.TempDir()

	// Invalid YAML — plan.Create should write it without validation
	invalidYAML := []byte("invalid: yaml: syntax:")

	_, err := Create(CreateOptions{
		PlansDir:       dir,
		Name:           "invalid-wf",
		Phases:         []string{"impl"},
		WorkflowType:   "feature",
		CustomWorkflow: invalidYAML,
	})
	// Should succeed — validation happens later during workflow loading
	if err != nil {
		t.Fatalf("Create should not fail for invalid custom workflow YAML: %v", err)
	}

	// Verify the file was written as-is
	data, err := os.ReadFile(filepath.Join(dir, "invalid-wf", "workflow.yaml"))
	if err != nil {
		t.Fatalf("workflow.yaml should exist: %v", err)
	}
	if !bytes.Equal(data, invalidYAML) {
		t.Errorf("workflow.yaml content mismatch")
	}
}

func TestCreateOptions_PlanContentEmptyString(t *testing.T) {
	dir := t.TempDir()

	_, err := Create(CreateOptions{
		PlansDir:     dir,
		Name:         "empty-content",
		Phases:       []string{"phase1"},
		WorkflowType: "feature",
		PlanContent:  map[string]string{"phase1": ""},
	})
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}

	// Empty string means skip custom content — use default template
	planMD, err := os.ReadFile(filepath.Join(dir, "empty-content", "phases", "phase1", "plan.md"))
	if err != nil {
		t.Fatalf("ReadFile plan.md error: %v", err)
	}
	// Should have default template content (not empty)
	if len(planMD) == 0 {
		t.Error("plan.md should not be empty when PlanContent is empty string")
	}
	if !strings.Contains(string(planMD), "Phase:") {
		t.Errorf("plan.md should contain default template content, got: %s", string(planMD))
	}
}

func TestCreateOptions_PlanContentForNonexistentPhase(t *testing.T) {
	dir := t.TempDir()

	_, err := Create(CreateOptions{
		PlansDir:     dir,
		Name:         "nonexist-phase",
		Phases:       []string{"aa", "bb"},
		WorkflowType: "feature",
		PlanContent:  map[string]string{"cc": "content for nonexistent phase"},
	})
	// Should succeed — "cc" entry is just ignored
	if err != nil {
		t.Fatalf("Create should not fail for PlanContent with nonexistent phase: %v", err)
	}

	// Verify aa and bb exist with default templates
	for _, phase := range []string{"aa", "bb"} {
		planMD, err := os.ReadFile(filepath.Join(dir, "nonexist-phase", "phases", phase, "plan.md"))
		if err != nil {
			t.Fatalf("ReadFile plan.md for %s error: %v", phase, err)
		}
		if len(planMD) == 0 {
			t.Errorf("plan.md for %s should not be empty", phase)
		}
	}

	// Verify "cc" directory was NOT created
	_, err = os.Stat(filepath.Join(dir, "nonexist-phase", "phases", "cc"))
	if err == nil {
		t.Error("phase directory 'cc' should not exist")
	}
}
