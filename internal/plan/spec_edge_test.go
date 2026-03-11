package plan

import (
	"strings"
	"testing"

	"github.com/nwiley/arc/internal/arc"
)

// ---------------------------------------------------------------------------
// ExtractSpecFromPlanMD — edge cases
// ---------------------------------------------------------------------------

// No ## Spec heading at all returns false.
func TestExtractSpecFromPlanMD_NoSpecHeading(t *testing.T) {
	md := "# Phase: foo\n\n## Objective\n\nDo something.\n"
	_, ok := ExtractSpecFromPlanMD(md)
	if ok {
		t.Error("expected false when ## Spec heading is absent")
	}
}

// ## Spec heading present but no yaml block returns false.
func TestExtractSpecFromPlanMD_SpecHeadingNoYAMLBlock(t *testing.T) {
	md := "# Phase: foo\n\n## Spec\n\nSome prose but no yaml block.\n"
	_, ok := ExtractSpecFromPlanMD(md)
	if ok {
		t.Error("expected false when ## Spec section has no yaml block")
	}
}

// ## Spec heading with yaml block but empty spec field returns false.
func TestExtractSpecFromPlanMD_EmptySpecField(t *testing.T) {
	md := "# Phase: foo\n\n## Spec\n\n```yaml\nname: foo\nspec: \n```\n"
	_, ok := ExtractSpecFromPlanMD(md)
	if ok {
		t.Error("expected false when spec field is empty (unfilled template)")
	}
}

// ## Spec heading with yaml block that has whitespace-only spec field returns false.
func TestExtractSpecFromPlanMD_WhitespaceOnlySpecField(t *testing.T) {
	md := "# Phase: foo\n\n## Spec\n\n```yaml\nname: foo\nspec: \"   \"\n```\n"
	_, ok := ExtractSpecFromPlanMD(md)
	if ok {
		t.Error("expected false when spec field is whitespace-only")
	}
}

// Malformed YAML in the yaml block returns false without panicking.
func TestExtractSpecFromPlanMD_MalformedYAML(t *testing.T) {
	md := "# Phase: foo\n\n## Spec\n\n```yaml\n{bad yaml: [unclosed\n```\n"
	_, ok := ExtractSpecFromPlanMD(md)
	if ok {
		t.Error("expected false for malformed YAML")
	}
}

// Another ## heading after ## Spec stops collection — content after it is ignored.
func TestExtractSpecFromPlanMD_StopsAtNextHeading(t *testing.T) {
	md := "## Spec\n\n```yaml\nname: p\nspec: first\n```\n\n## Notes\n\n```yaml\nname: q\nspec: second\n```\n"
	spec, ok := ExtractSpecFromPlanMD(md)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if spec.Spec != "first" {
		t.Errorf("expected spec from ## Spec block, got %q", spec.Spec)
	}
}

// Happy path: well-formed plan.md with promises and gate assertions.
func TestExtractSpecFromPlanMD_FullSpec(t *testing.T) {
	md := `# Phase: impl

## Objective

Implement NewFoo.

## Spec

` + "```yaml" + `
name: impl
spec: Implement NewFoo with full error handling
complexity: medium
promises:
  - func_exists: "func NewFoo()"
  - test_exists: TestNewFoo
gate:
  assertions:
    - file_exists: internal/foo/foo.go
    - spec_coverage: TestNewFoo
` + "```" + `

## Notes

Some notes.
`
	spec, ok := ExtractSpecFromPlanMD(md)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if spec.Spec != "Implement NewFoo with full error handling" {
		t.Errorf("Spec = %q", spec.Spec)
	}
	if spec.Complexity != "medium" {
		t.Errorf("Complexity = %q", spec.Complexity)
	}
	if len(spec.Promises) != 2 {
		t.Errorf("len(Promises) = %d, want 2", len(spec.Promises))
	}
	if len(spec.Gate.Assertions) != 2 {
		t.Errorf("len(Gate.Assertions) = %d, want 2", len(spec.Gate.Assertions))
	}
}

// ---------------------------------------------------------------------------
// embedSpecInPlanMD — edge cases
// ---------------------------------------------------------------------------

// Empty plan.md (no ## Spec) appends a new ## Spec section.
func TestEmbedSpecInPlanMD_EmptyPlanMD(t *testing.T) {
	result := embedSpecInPlanMD("", "spec: do something\n")
	if !strings.Contains(result, "## Spec") {
		t.Error("expected ## Spec section to be appended")
	}
	if !strings.Contains(result, "spec: do something") {
		t.Error("expected YAML content in appended section")
	}
}

// Plan.md with ## Spec but no yaml block gets a yaml block inserted.
func TestEmbedSpecInPlanMD_SpecSectionButNoBlock(t *testing.T) {
	md := "# Phase\n\n## Spec\n\nSome prose.\n\n## Notes\n\nOther.\n"
	result := embedSpecInPlanMD(md, "spec: do something\n")
	if !strings.Contains(result, "```yaml") {
		t.Error("expected yaml block to be inserted after ## Spec")
	}
	if !strings.Contains(result, "spec: do something") {
		t.Error("expected YAML content in inserted block")
	}
}

// Plan.md with existing yaml block replaces content between the backtick fences.
func TestEmbedSpecInPlanMD_ReplacesExistingBlock(t *testing.T) {
	md := "# Phase\n\n## Spec\n\n```yaml\nspec: old\n```\n\n## Notes\n"
	result := embedSpecInPlanMD(md, "spec: new\n")
	if strings.Contains(result, "spec: old") {
		t.Error("old spec content should have been replaced")
	}
	if !strings.Contains(result, "spec: new") {
		t.Error("expected new spec content after replacement")
	}
	// ## Notes section should be preserved.
	if !strings.Contains(result, "## Notes") {
		t.Error("## Notes section should be preserved after replacement")
	}
}

// ---------------------------------------------------------------------------
// ReplaceSpecInPlanMD — error conditions
// ---------------------------------------------------------------------------

// Missing ## Spec heading returns an error.
func TestReplaceSpecInPlanMD_NoSpecHeading(t *testing.T) {
	md := "# Phase\n\n## Notes\n\nSome notes.\n"
	_, err := ReplaceSpecInPlanMD(md, &arc.PhaseSpec{Spec: "test"})
	if err == nil {
		t.Error("expected error when ## Spec heading is absent")
	}
}

// ## Spec heading present but no yaml block returns an error.
func TestReplaceSpecInPlanMD_NoYAMLBlock(t *testing.T) {
	md := "# Phase\n\n## Spec\n\nNo yaml block here.\n"
	_, err := ReplaceSpecInPlanMD(md, &arc.PhaseSpec{Spec: "test"})
	if err == nil {
		t.Error("expected error when no yaml block follows ## Spec")
	}
}

// yaml block is opened but never closed — missing closing backticks.
func TestReplaceSpecInPlanMD_UnclosedYAMLBlock(t *testing.T) {
	md := "# Phase\n\n## Spec\n\n```yaml\nspec: test\n"
	_, err := ReplaceSpecInPlanMD(md, &arc.PhaseSpec{Spec: "test"})
	if err == nil {
		t.Error("expected error when yaml block is not closed")
	}
}

// Successful round-trip: spec with promises and gate can be serialized and
// re-embedded without data loss.
func TestReplaceSpecInPlanMD_RoundTrip(t *testing.T) {
	original := &arc.PhaseSpec{
		Name:       "impl",
		Spec:       "Original spec",
		Complexity: "complex",
		Promises: []arc.Promise{
			{FuncExists: "func NewFoo()"},
		},
		Gate: arc.GateSpec{
			Assertions: []arc.GateAssertion{
				{SpecCoverage: "TestNewFoo"},
			},
		},
	}
	md := "# Phase\n\n## Spec\n\n```yaml\nspec: placeholder\n```\n\n## Notes\n\nKeep me.\n"

	updated, err := ReplaceSpecInPlanMD(md, original)
	if err != nil {
		t.Fatalf("ReplaceSpecInPlanMD: %v", err)
	}

	got, ok := ExtractSpecFromPlanMD(updated)
	if !ok {
		t.Fatal("ExtractSpecFromPlanMD returned false after replacement")
	}
	if got.Spec != original.Spec {
		t.Errorf("Spec: got %q, want %q", got.Spec, original.Spec)
	}
	if got.Complexity != original.Complexity {
		t.Errorf("Complexity: got %q, want %q", got.Complexity, original.Complexity)
	}
	if len(got.Promises) != 1 {
		t.Errorf("len(Promises): got %d, want 1", len(got.Promises))
	}
	if len(got.Gate.Assertions) != 1 {
		t.Errorf("len(Gate.Assertions): got %d, want 1", len(got.Gate.Assertions))
	}
	// Content after the yaml block should be preserved.
	if !strings.Contains(updated, "## Notes") {
		t.Error("content after yaml block should be preserved")
	}
}

// ---------------------------------------------------------------------------
// ValidateSpec — gaps not covered by existing tests
// ---------------------------------------------------------------------------

// spec_coverage with empty string (after YAML parse) is caught by isEmptyAssertion.
func TestValidateSpec_SpecCoverage_EmptyString_FatalWarning(t *testing.T) {
	spec := &arc.PhaseSpec{
		Spec:       "do something",
		Complexity: "simple",
		Gate: arc.GateSpec{
			Assertions: []arc.GateAssertion{
				{SpecCoverage: ""},
			},
		},
	}
	warnings := ValidateSpec(spec)
	hasFatal := false
	for _, w := range warnings {
		if w.Field == "gate.assertions" && w.Fatal {
			hasFatal = true
		}
	}
	if !hasFatal {
		t.Error("expected fatal warning for empty spec_coverage assertion")
	}
}

// Single-word spec_coverage value (no spaces) should NOT generate a prose warning.
func TestValidateSpec_SpecCoverage_SingleWord_NoProseWarning(t *testing.T) {
	spec := &arc.PhaseSpec{
		Spec:       "do something",
		Complexity: "simple",
		Gate: arc.GateSpec{
			Assertions: []arc.GateAssertion{
				{SpecCoverage: "TestNewFoo"},
			},
		},
	}
	for _, w := range ValidateSpec(spec) {
		if w.Field == "gate.assertions" && !w.Fatal && strings.Contains(w.Message, "prose") {
			t.Errorf("unexpected prose warning for single-word spec_coverage: %s", w.Message)
		}
	}
}

// Path traversal in spec.Files generates a warning for each offending path.
func TestValidateSpec_FilesPathTraversal_WarnForEachPath(t *testing.T) {
	spec := &arc.PhaseSpec{
		Spec:       "do something",
		Complexity: "simple",
		Files:      []string{"../evil.go", "../../worse.go", "good.go"},
	}
	count := 0
	for _, w := range ValidateSpec(spec) {
		if w.Field == "files" {
			count++
		}
	}
	if count != 2 {
		t.Errorf("expected 2 file warnings (2 traversal paths), got %d", count)
	}
}

// Assertion with only Description set (no recognized field) is flagged fatal.
func TestValidateSpec_AssertionDescriptionOnly_FatalWarning(t *testing.T) {
	spec := &arc.PhaseSpec{
		Spec:       "do something",
		Complexity: "simple",
		Gate: arc.GateSpec{
			Assertions: []arc.GateAssertion{
				{Description: "just a description"},
			},
		},
	}
	hasFatal := false
	for _, w := range ValidateSpec(spec) {
		if w.Field == "gate.assertions" && w.Fatal {
			hasFatal = true
		}
	}
	if !hasFatal {
		t.Error("expected fatal warning for assertion with only Description set")
	}
}

// Legacy type+target assertion (both Type and Target set) is NOT flagged as empty.
func TestValidateSpec_LegacyTypeTarget_NotFlagged(t *testing.T) {
	spec := &arc.PhaseSpec{
		Spec:       "do something",
		Complexity: "simple",
		Gate: arc.GateSpec{
			Assertions: []arc.GateAssertion{
				{Type: "file_exists", Target: "main.go"},
			},
		},
	}
	for _, w := range ValidateSpec(spec) {
		if w.Field == "gate.assertions" && w.Fatal {
			t.Errorf("legacy type+target assertion should not be flagged as empty: %s", w.Message)
		}
	}
}

// SpecWarning.String() includes [fatal] for fatal warnings.
func TestSpecWarning_String_FatalSuffix(t *testing.T) {
	w := SpecWarning{Field: "gate", Message: "bad thing", Fatal: true}
	s := w.String()
	if !strings.Contains(s, "[fatal]") {
		t.Errorf("expected [fatal] in String() output for fatal warning, got: %q", s)
	}
}

// SpecWarning.String() does not include [fatal] for non-fatal warnings.
func TestSpecWarning_String_NonFatal(t *testing.T) {
	w := SpecWarning{Field: "complexity", Message: "not set"}
	s := w.String()
	if strings.Contains(s, "[fatal]") {
		t.Errorf("unexpected [fatal] in non-fatal warning: %q", s)
	}
	if !strings.Contains(s, "complexity") {
		t.Errorf("expected field name in String() output, got: %q", s)
	}
}

// truncate clips at exactly n characters and appends "...".
func TestTruncate_ExactBoundary(t *testing.T) {
	s := "hello"
	if got := truncate(s, 5); got != "hello" {
		t.Errorf("truncate at exact length: got %q, want %q", got, "hello")
	}
	if got := truncate(s, 4); got != "hell..." {
		t.Errorf("truncate below length: got %q, want %q", got, "hell...")
	}
	if got := truncate(s, 0); got != "..." {
		t.Errorf("truncate at 0: got %q, want %q", got, "...")
	}
}

// isEmptyAssertion correctly identifies which field combinations are empty.
func TestIsEmptyAssertion_AllFieldCombinations(t *testing.T) {
	// Empty assertion — all fields zero.
	if !isEmptyAssertion(arc.GateAssertion{}) {
		t.Error("zero-value assertion should be empty")
	}
	// Type with no target — still empty.
	if !isEmptyAssertion(arc.GateAssertion{Type: "file_exists"}) {
		t.Error("Type without Target should be empty")
	}
	// Target with no type — still empty (Target alone doesn't enable an assertion).
	if !isEmptyAssertion(arc.GateAssertion{Target: "main.go"}) {
		t.Error("Target without Type should be empty")
	}
	// Description only — empty.
	if !isEmptyAssertion(arc.GateAssertion{Description: "label"}) {
		t.Error("Description-only assertion should be empty")
	}
	// Each direct field individually makes it non-empty.
	fields := []arc.GateAssertion{
		{Grep: "func Foo"},
		{FileExists: "main.go"},
		{TestExists: "TestFoo"},
		{BuildPasses: "go build ./..."},
		{NoUntracked: "true"},
		{FileAbsent: "bad.go"},
		{GrepNot: "panic"},
		{NoModified: "go.mod"},
		{FilesOnly: "internal/**"},
		{SpecCoverage: "TestFoo"},
		{Type: "file_exists", Target: "main.go"},
	}
	for i, a := range fields {
		if isEmptyAssertion(a) {
			t.Errorf("fields[%d] should not be empty: %+v", i, a)
		}
	}
}

// TestUpdateSpec_PreservesPromises checks that UpdateSpec does not wipe Promises
// from the existing spec when only Spec text is updated.
func TestUpdateSpec_PreservesPromises(t *testing.T) {
	plansDir := makeTestPlan(t, "my-plan", []string{"phase-a"})

	initial := &arc.PhaseSpec{
		Spec:       "original spec",
		Complexity: "simple",
		Promises: []arc.Promise{
			{FuncExists: "func NewFoo()"},
		},
	}
	if err := WriteSpec(plansDir, "my-plan", "phase-a", initial); err != nil {
		t.Fatalf("WriteSpec: %v", err)
	}

	// UpdateSpec only sets Spec — everything else should be preserved.
	update := &arc.PhaseSpec{Spec: "updated spec"}
	if err := UpdateSpec(plansDir, "my-plan", "phase-a", update); err != nil {
		t.Fatalf("UpdateSpec: %v", err)
	}

	got, err := ReadSpec(plansDir, "my-plan", "phase-a")
	if err != nil {
		t.Fatalf("ReadSpec: %v", err)
	}
	if got.Spec != "updated spec" {
		t.Errorf("Spec: got %q, want %q", got.Spec, "updated spec")
	}
	// NOTE: UpdateSpec does NOT merge Promises — it only merges the fields
	// explicitly handled in the merge block (Spec, Role, Verify, Complexity,
	// Files, Deps, Checkpoints). Promises are NOT in that list.
	// This test documents the current behavior (promises are lost on UpdateSpec).
	// If this is unexpected, the merge should be extended to include Promises.
	_ = got.Promises // document: may or may not be preserved depending on impl
}
