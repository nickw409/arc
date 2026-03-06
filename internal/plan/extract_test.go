package plan

import (
	"testing"
)

func TestExtractSpecFromPlanMD(t *testing.T) {
	planMD := `# Phase: detect

## Objective

Some objective.

## Spec

` + "```yaml" + `
name: detect
complexity: medium
spec: |
  Do the thing.
checkpoints:
  - name: compiles
    description: builds
    test: go build ./...
gate:
  assertions:
    - type: grep
      file: internal/arc/adapter.go
      pattern: "RateLimit"
` + "```" + `

## Files

Some files.
`
	spec, ok := ExtractSpecFromPlanMD(planMD)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if spec.Name != "detect" {
		t.Errorf("name=%q want detect", spec.Name)
	}
	if spec.Complexity != "medium" {
		t.Errorf("complexity=%q want medium", spec.Complexity)
	}
	if len(spec.Checkpoints) != 1 {
		t.Errorf("checkpoints=%d want 1", len(spec.Checkpoints))
	}
	if len(spec.Gate.Assertions) != 1 {
		t.Errorf("assertions=%d want 1", len(spec.Gate.Assertions))
	}
}

func TestExtractSpecFromPlanMDNoBlock(t *testing.T) {
	planMD := "# Phase: foo\n\n## Objective\n\nNo spec block here.\n"
	_, ok := ExtractSpecFromPlanMD(planMD)
	if ok {
		t.Fatal("expected ok=false for plan.md with no ## Spec block")
	}
}

func TestExtractSpecFromPlanMDEmptySpec(t *testing.T) {
	planMD := "## Spec\n\n" + "```yaml\nname: x\ncomplexity: simple\nspec: \"\"\n```\n"
	_, ok := ExtractSpecFromPlanMD(planMD)
	if ok {
		t.Fatal("expected ok=false when spec field is empty")
	}
}
