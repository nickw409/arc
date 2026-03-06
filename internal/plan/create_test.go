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

	// No auto-deps — phases are parallel by default
	if len(meta.Dependencies) != 0 {
		t.Fatalf("Dependencies = %v, want empty (no auto-chaining)", meta.Dependencies)
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

// --- PlanContent tests ---

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
