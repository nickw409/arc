package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderGateImpl(t *testing.T) {
	data := ImplData{
		Spec:  "Implement a widget factory",
		Files: []string{"internal/widget/factory.go", "internal/widget/factory_test.go"},
		Checkpoints: []CheckpointData{
			{Name: "Factory creation", Description: "NewFactory returns a non-nil factory", Test: "go test ./internal/widget/ -run TestNewFactory"},
			{Name: "Widget creation", Description: "factory.Create returns a widget", Test: "go test ./internal/widget/ -run TestCreate"},
		},
		Plan:        "my-plan",
		Phase:       "impl",
		TestCommand: "go test ./internal/widget/",
	}

	result, err := RenderGatePrompt("impl", data)
	if err != nil {
		t.Fatalf("RenderGatePrompt impl: unexpected error: %v", err)
	}

	if !strings.Contains(result, "Implement a widget factory") {
		t.Error("expected output to contain spec text")
	}
	if !strings.Contains(result, "internal/widget/factory.go") {
		t.Error("expected output to contain file path")
	}
	if !strings.Contains(result, "Factory creation") {
		t.Error("expected output to contain first checkpoint name")
	}
	if !strings.Contains(result, "Widget creation") {
		t.Error("expected output to contain second checkpoint name")
	}
	if !strings.Contains(result, "arc gate my-plan impl") {
		t.Error("expected output to contain gate command")
	}
	if !strings.Contains(result, "go test ./internal/widget/") {
		t.Error("expected output to contain test command")
	}
	// Checkpoints should be 1-indexed
	if !strings.Contains(result, "1. **Factory creation**") {
		t.Error("expected 1-based checkpoint index")
	}
	if !strings.Contains(result, "2. **Widget creation**") {
		t.Error("expected 1-based checkpoint index for second checkpoint")
	}
}

func TestRenderGatePromptRetry(t *testing.T) {
	data := RetryData{
		Attempt:     2,
		MaxAttempts: 3,
		GateOutput:  "FAIL: assertion file_exists internal/widget/factory.go",
		DiffSummary: "+func NewFactory() *Factory {\n+\treturn nil\n+}",
	}

	result, err := RenderGatePrompt("retry", data)
	if err != nil {
		t.Fatalf("RenderGatePrompt retry: unexpected error: %v", err)
	}

	if !strings.Contains(result, "attempt 2 of 3") {
		t.Error("expected attempt count in output")
	}
	if !strings.Contains(result, "FAIL: assertion file_exists") {
		t.Error("expected gate output in retry prompt")
	}
	if !strings.Contains(result, "+func NewFactory()") {
		t.Error("expected diff summary in retry prompt")
	}
}

func TestRenderGatePromptPlanner(t *testing.T) {
	data := PlannerData{
		Description: "Add rate limiting to the HTTP server",
		PlanName:    "rate-limit",
	}

	result, err := RenderGatePrompt("planner", data)
	if err != nil {
		t.Fatalf("RenderGatePrompt planner: unexpected error: %v", err)
	}

	if !strings.Contains(result, "Add rate limiting to the HTTP server") {
		t.Error("expected description in planner output")
	}
	if !strings.Contains(result, "arc plan create rate-limit") {
		t.Error("expected plan create command in planner output")
	}
	if !strings.Contains(result, "arc plan add-phase rate-limit") {
		t.Error("expected add-phase command in planner output")
	}
}

func TestRenderGatePromptOrchestrator(t *testing.T) {
	data := OrchestratorData{
		AttemptCount: 3,
		PhaseName:    "impl",
		SpecSummary:  "Add widget factory",
		Attempts: []AttemptData{
			{GateOutput: "FAIL: test_exists TestNewFactory", CheckpointsPassed: 0, CheckpointsTotal: 2},
			{GateOutput: "FAIL: grep NewFactory returns nil", CheckpointsPassed: 1, CheckpointsTotal: 2},
			{GateOutput: "FAIL: grep NewFactory returns nil", CheckpointsPassed: 1, CheckpointsTotal: 2},
		},
		DiffSummary: "+func NewFactory() *Factory { return nil }",
	}

	result, err := RenderGatePrompt("orchestrator", data)
	if err != nil {
		t.Fatalf("RenderGatePrompt orchestrator: unexpected error: %v", err)
	}

	if !strings.Contains(result, "failed 3 attempts") {
		t.Error("expected attempt count in orchestrator output")
	}
	if !strings.Contains(result, "impl: Add widget factory") {
		t.Error("expected phase name and spec in orchestrator output")
	}
	if !strings.Contains(result, "### Attempt 1") {
		t.Error("expected attempt 1 header in orchestrator output")
	}
	if !strings.Contains(result, "### Attempt 3") {
		t.Error("expected attempt 3 header in orchestrator output")
	}
	if !strings.Contains(result, "FAIL: test_exists TestNewFactory") {
		t.Error("expected first attempt gate output")
	}
	if !strings.Contains(result, "Checkpoints passed: 0 / 2") {
		t.Error("expected checkpoint counts")
	}
	if !strings.Contains(result, "MODIFY_SPEC") {
		t.Error("expected MODIFY_SPEC option in decision section")
	}
	if !strings.Contains(result, "GIVE_UP") {
		t.Error("expected GIVE_UP option in decision section")
	}
}

func TestRenderGatePromptAdversary(t *testing.T) {
	data := AdversaryData{
		ChangedFiles: []string{
			"internal/widget/factory.go",
			"internal/widget/widget.go",
		},
		TestCommand: "go test ./internal/widget/",
	}

	result, err := RenderGatePrompt("adversary", data)
	if err != nil {
		t.Fatalf("RenderGatePrompt adversary: unexpected error: %v", err)
	}

	if !strings.Contains(result, "internal/widget/factory.go") {
		t.Error("expected changed file in adversary output")
	}
	if !strings.Contains(result, "internal/widget/widget.go") {
		t.Error("expected second changed file in adversary output")
	}
	if !strings.Contains(result, "go test ./internal/widget/") {
		t.Error("expected test command in adversary output")
	}
	if !strings.Contains(result, "Off-by-one errors") {
		t.Error("expected 'What to Look For' content in adversary output")
	}
}

func TestRenderGatePromptProjectContext(t *testing.T) {
	data := ImplData{
		Spec:        "Add a feature",
		Files:       []string{"internal/foo/foo.go"},
		Checkpoints: []CheckpointData{{Name: "cp1", Description: "first checkpoint", Test: "go test ./..."}},
		Plan:        "plan",
		Phase:       "phase",
		TestCommand: "go test ./...",
		ProjectContext: "This is a Go microservice using gRPC.\n" +
			"Do not use global variables.",
	}

	result, err := RenderGatePrompt("impl", data)
	if err != nil {
		t.Fatalf("RenderGatePrompt impl with context: unexpected error: %v", err)
	}

	if !strings.Contains(result, "Project Context") {
		t.Error("expected 'Project Context' section heading")
	}
	if !strings.Contains(result, "This is a Go microservice using gRPC.") {
		t.Error("expected project context content in output")
	}
	if !strings.Contains(result, "Do not use global variables.") {
		t.Error("expected second line of project context in output")
	}
}

func TestRenderGatePromptNoProjectContext(t *testing.T) {
	data := ImplData{
		Spec:           "Add a feature",
		Files:          []string{"internal/foo/foo.go"},
		Checkpoints:    []CheckpointData{{Name: "cp1", Description: "first checkpoint", Test: "go test ./..."}},
		Plan:           "plan",
		Phase:          "phase",
		TestCommand:    "go test ./...",
		ProjectContext: "",
	}

	result, err := RenderGatePrompt("impl", data)
	if err != nil {
		t.Fatalf("RenderGatePrompt impl without context: unexpected error: %v", err)
	}

	if strings.Contains(result, "Project Context") {
		t.Error("expected NO 'Project Context' section when ProjectContext is empty")
	}
}

func TestLoadProjectContext(t *testing.T) {
	t.Run("file exists", func(t *testing.T) {
		dir := t.TempDir()
		arcDir := filepath.Join(dir, ".arc")
		if err := os.MkdirAll(arcDir, 0o755); err != nil {
			t.Fatalf("failed to create .arc dir: %v", err)
		}
		content := "This project uses Go 1.24.\nModule: github.com/example/app"
		if err := os.WriteFile(filepath.Join(arcDir, "context.md"), []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write context.md: %v", err)
		}

		got := LoadProjectContext(dir)
		if got != content {
			t.Errorf("LoadProjectContext: got %q, want %q", got, content)
		}
	})

	t.Run("file missing", func(t *testing.T) {
		dir := t.TempDir()
		got := LoadProjectContext(dir)
		if got != "" {
			t.Errorf("LoadProjectContext: expected empty string for missing file, got %q", got)
		}
	})

	t.Run("arc dir missing", func(t *testing.T) {
		dir := t.TempDir()
		// No .arc directory at all
		got := LoadProjectContext(dir)
		if got != "" {
			t.Errorf("LoadProjectContext: expected empty string when .arc dir missing, got %q", got)
		}
	})
}

func TestAddFuncMap(t *testing.T) {
	fm := gateFuncMap()
	addFn, ok := fm["add"]
	if !ok {
		t.Fatal("expected 'add' function in gateFuncMap")
	}
	fn, ok := addFn.(func(int, int) int)
	if !ok {
		t.Fatal("expected add to be func(int, int) int")
	}
	if got := fn(1, 2); got != 3 {
		t.Errorf("add(1, 2) = %d, want 3", got)
	}
	if got := fn(0, 5); got != 5 {
		t.Errorf("add(0, 5) = %d, want 5", got)
	}
	if got := fn(10, -3); got != 7 {
		t.Errorf("add(10, -3) = %d, want 7", got)
	}
}
