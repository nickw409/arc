package workflow

import (
	"strings"
	"testing"

	"github.com/nwiley/arc/internal/arc"
)

// buildFeatureWorkflow constructs the standard feature workflow for tests.
func buildFeatureWorkflow() *arc.Workflow {
	return &arc.Workflow{
		Name:           "feature",
		Version:        5,
		Description:    "Standard feature development workflow",
		EntryState:     "qa",
		TerminalStates: []string{"complete", "blocked"},
		States: []arc.StateConfig{
			{
				Name:        "qa",
				Description: "Write tests",
				Prompt:      "feature/qa.md",
				Transition:  arc.Transition{Branches: map[arc.Verdict]string{"": "qa_review"}},
			},
			{
				Name:        "qa_review",
				Description: "Review tests",
				Prompt:      "feature/qa_review.md",
				Verdicts:    []string{"approved", "gaps_found"},
				Transition: arc.Transition{Branches: map[arc.Verdict]string{
					arc.VerdictApproved:  "impl",
					arc.VerdictGapsFound: "qa",
				}},
			},
			{
				Name:        "impl",
				Description: "Implement",
				Prompt:      "feature/impl.md",
				Transition:  arc.Transition{Branches: map[arc.Verdict]string{"": "impl_review"}},
			},
			{
				Name:        "impl_review",
				Description: "Review implementation",
				Prompt:      "feature/impl_review.md",
				Verdicts:    []string{"approved", "concerns"},
				Transition: arc.Transition{Branches: map[arc.Verdict]string{
					arc.VerdictApproved: "complete",
					arc.VerdictConcerns: "impl",
				}},
			},
			{
				Name:        "complete",
				Description: "Done",
			},
			{
				Name:        "blocked",
				Description: "Blocked",
			},
		},
	}
}

func TestNextStateLinear(t *testing.T) {
	m := NewMachine(buildFeatureWorkflow())
	next, err := m.NextState("qa", "")
	if err != nil {
		t.Fatalf("NextState error: %v", err)
	}
	if next != "qa_review" {
		t.Fatalf("NextState = %q, want %q", next, "qa_review")
	}
}

func TestNextStateBranchingApproved(t *testing.T) {
	m := NewMachine(buildFeatureWorkflow())
	next, err := m.NextState("qa_review", arc.VerdictApproved)
	if err != nil {
		t.Fatalf("NextState error: %v", err)
	}
	if next != "impl" {
		t.Fatalf("NextState = %q, want %q", next, "impl")
	}
}

func TestNextStateBranchingGapsFound(t *testing.T) {
	m := NewMachine(buildFeatureWorkflow())
	next, err := m.NextState("qa_review", arc.VerdictGapsFound)
	if err != nil {
		t.Fatalf("NextState error: %v", err)
	}
	if next != "qa" {
		t.Fatalf("NextState = %q, want %q", next, "qa")
	}
}

func TestNextStateBranchingNoVerdict(t *testing.T) {
	m := NewMachine(buildFeatureWorkflow())
	_, err := m.NextState("qa_review", "")
	if err == nil {
		t.Fatal("expected error for branching state with no verdict, got nil")
	}
	if !strings.Contains(err.Error(), "requires a verdict") {
		t.Fatalf("error = %q, want containing 'requires a verdict'", err.Error())
	}
}

func TestNextStateBranchingInvalidVerdict(t *testing.T) {
	m := NewMachine(buildFeatureWorkflow())
	_, err := m.NextState("qa_review", arc.VerdictConcerns)
	if err == nil {
		t.Fatal("expected error for invalid verdict on qa_review, got nil")
	}
	if !strings.Contains(err.Error(), `"concerns"`) {
		t.Fatalf("error = %q, want containing '\"concerns\"'", err.Error())
	}
	if !strings.Contains(err.Error(), `"qa_review"`) {
		t.Fatalf("error = %q, want containing '\"qa_review\"'", err.Error())
	}
}

func TestNextStateTerminal(t *testing.T) {
	m := NewMachine(buildFeatureWorkflow())
	next, err := m.NextState("complete", "")
	if err != nil {
		t.Fatalf("NextState error: %v", err)
	}
	if next != Terminal {
		t.Fatalf("NextState = %q, want %q", next, Terminal)
	}
}

func TestNextStateUnknownState(t *testing.T) {
	m := NewMachine(buildFeatureWorkflow())
	_, err := m.NextState("nonexistent", "")
	if err == nil {
		t.Fatal("expected error for unknown state, got nil")
	}
	if !strings.Contains(err.Error(), `"nonexistent"`) {
		t.Fatalf("error = %q, want containing '\"nonexistent\"'", err.Error())
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %q, want containing 'not found'", err.Error())
	}
}

func TestNextStateLinearWithVerdict(t *testing.T) {
	// Verdict should be ignored for linear states (no verdicts defined)
	m := NewMachine(buildFeatureWorkflow())
	next, err := m.NextState("qa", arc.VerdictApproved)
	if err != nil {
		t.Fatalf("NextState error: %v", err)
	}
	if next != "qa_review" {
		t.Fatalf("NextState = %q, want %q", next, "qa_review")
	}
}

func TestMachineIsTerminal(t *testing.T) {
	m := NewMachine(buildFeatureWorkflow())

	if !m.IsTerminal("complete") {
		t.Fatal("IsTerminal(\"complete\") = false, want true")
	}
	if !m.IsTerminal("blocked") {
		t.Fatal("IsTerminal(\"blocked\") = false, want true")
	}
	if m.IsTerminal("qa") {
		t.Fatal("IsTerminal(\"qa\") = true, want false")
	}
}

func TestMachineEntryState(t *testing.T) {
	m := NewMachine(buildFeatureWorkflow())
	if m.EntryState() != "qa" {
		t.Fatalf("EntryState() = %q, want %q", m.EntryState(), "qa")
	}
}

func TestMachineGetState(t *testing.T) {
	m := NewMachine(buildFeatureWorkflow())

	qa := m.GetState("qa")
	if qa == nil {
		t.Fatal("GetState(\"qa\") = nil, want non-nil")
	}
	if qa.Name != "qa" {
		t.Fatalf("GetState(\"qa\").Name = %q, want %q", qa.Name, "qa")
	}

	if m.GetState("nonexistent") != nil {
		t.Fatal("GetState(\"nonexistent\") should be nil")
	}
}

func TestMachineValidVerdicts(t *testing.T) {
	m := NewMachine(buildFeatureWorkflow())

	verdicts := m.ValidVerdicts("qa_review")
	if verdicts == nil {
		t.Fatal("ValidVerdicts(\"qa_review\") = nil, want non-nil")
	}
	// Should contain approved and gaps_found
	found := make(map[arc.Verdict]bool)
	for _, v := range verdicts {
		found[v] = true
	}
	if !found[arc.VerdictApproved] {
		t.Fatal("ValidVerdicts missing approved")
	}
	if !found[arc.VerdictGapsFound] {
		t.Fatal("ValidVerdicts missing gaps_found")
	}
	if len(verdicts) != 2 {
		t.Fatalf("len(ValidVerdicts) = %d, want 2", len(verdicts))
	}

	// Linear state: no verdicts
	if m.ValidVerdicts("qa") != nil {
		t.Fatal("ValidVerdicts(\"qa\") should be nil for linear state")
	}

	// Nonexistent state
	if m.ValidVerdicts("nonexistent") != nil {
		t.Fatal("ValidVerdicts(\"nonexistent\") should be nil")
	}
}

func TestNewMachineNilWorkflow(t *testing.T) {
	// Should not panic
	m := NewMachine(nil)
	if m == nil {
		t.Fatal("NewMachine(nil) should return non-nil Machine")
	}

	// Calling NextState on a nil-workflow machine should return error
	_, err := m.NextState("anything", "")
	if err == nil {
		t.Fatal("expected error from NextState on nil-workflow machine, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %q, want containing 'not found'", err.Error())
	}
}
