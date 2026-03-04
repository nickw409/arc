package workflow

import (
	"strings"
	"testing"

	"github.com/nwiley/arc/internal/arc"
)

func TestValidateValidWorkflow(t *testing.T) {
	w, err := Load(testdataPath("valid-feature.yaml"))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	errs := Validate(w)
	if len(errs) != 0 {
		t.Fatalf("Validate returned %d errors, want 0: %v", len(errs), errs)
	}
}

func TestValidateBadNextReference(t *testing.T) {
	w := &arc.Workflow{
		Name:           "bad-next",
		EntryState:     "start",
		TerminalStates: []string{"done"},
		States: []arc.StateConfig{
			{
				Name:       "start",
				Transition: arc.Transition{Branches: map[arc.Verdict]string{"": "nonexistent"}},
			},
			{Name: "done"},
		},
	}
	errs := Validate(w)
	if len(errs) == 0 {
		t.Fatal("expected validation errors, got none")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "nonexistent") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected error referencing 'nonexistent', got: %v", errs)
	}
}

func TestValidateDuplicateState(t *testing.T) {
	w := &arc.Workflow{
		Name:           "dup",
		EntryState:     "qa",
		TerminalStates: []string{"done"},
		States: []arc.StateConfig{
			{
				Name:       "qa",
				Transition: arc.Transition{Branches: map[arc.Verdict]string{"": "done"}},
			},
			{
				Name:       "qa",
				Transition: arc.Transition{Branches: map[arc.Verdict]string{"": "done"}},
			},
			{Name: "done"},
		},
	}
	errs := Validate(w)
	if len(errs) == 0 {
		t.Fatal("expected validation errors, got none")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "duplicate") && strings.Contains(e.Error(), "qa") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected error about duplicate 'qa', got: %v", errs)
	}
}

func TestValidateUnreachableState(t *testing.T) {
	w := &arc.Workflow{
		Name:           "unreachable",
		EntryState:     "start",
		TerminalStates: []string{"done"},
		States: []arc.StateConfig{
			{
				Name:       "start",
				Transition: arc.Transition{Branches: map[arc.Verdict]string{"": "done"}},
			},
			{
				Name:       "orphan",
				Transition: arc.Transition{Branches: map[arc.Verdict]string{"": "done"}},
			},
			{Name: "done"},
		},
	}
	errs := Validate(w)
	if len(errs) == 0 {
		t.Fatal("expected validation errors, got none")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "unreachable") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected error about unreachable state, got: %v", errs)
	}
}

func TestValidateEntryIsTerminal(t *testing.T) {
	w := &arc.Workflow{
		Name:           "entry-terminal",
		EntryState:     "done",
		TerminalStates: []string{"done"},
		States: []arc.StateConfig{
			{Name: "done"},
		},
	}
	errs := Validate(w)
	if len(errs) == 0 {
		t.Fatal("expected validation errors, got none")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "entry") && strings.Contains(e.Error(), "terminal") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected error about entry state being terminal, got: %v", errs)
	}
}

func TestValidateEmptyStates(t *testing.T) {
	w := &arc.Workflow{
		Name:           "empty",
		EntryState:     "start",
		TerminalStates: []string{"done"},
		States:         []arc.StateConfig{},
	}
	errs := Validate(w)
	if len(errs) == 0 {
		t.Fatal("expected validation errors, got none")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "no states") || strings.Contains(e.Error(), "empty") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected error about empty states, got: %v", errs)
	}
}

func TestValidationErrorFormat(t *testing.T) {
	ve := &ValidationError{
		Field:   "states[2].next",
		Message: "references unknown state",
	}
	want := "states[2].next: references unknown state"
	if ve.Error() != want {
		t.Fatalf("Error() = %q, want %q", ve.Error(), want)
	}
}

func TestValidateVerdictsBranchesMismatch(t *testing.T) {
	// State has verdicts [approved, gaps_found] but only branches for approved
	w := &arc.Workflow{
		Name:           "mismatch",
		EntryState:     "review",
		TerminalStates: []string{"done"},
		States: []arc.StateConfig{
			{
				Name:     "review",
				Verdicts: []string{"approved", "gaps_found"},
				Transition: arc.Transition{Branches: map[arc.Verdict]string{
					arc.VerdictApproved: "done",
				}},
			},
			{Name: "done"},
		},
	}
	errs := Validate(w)
	if len(errs) == 0 {
		t.Fatal("expected validation errors for verdicts/branches mismatch, got none")
	}
	found := false
	for _, e := range errs {
		msg := e.Error()
		if (strings.Contains(msg, "verdict") || strings.Contains(msg, "branch")) &&
			strings.Contains(msg, "mismatch") {
			found = true
			break
		}
	}
	if !found {
		// Also check for missing branch error variant
		for _, e := range errs {
			if strings.Contains(e.Error(), "gaps_found") {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatalf("expected error about verdicts/branches mismatch, got: %v", errs)
	}
}

func TestValidateTerminalNonexistent(t *testing.T) {
	w := &arc.Workflow{
		Name:           "bad-terminal",
		EntryState:     "start",
		TerminalStates: []string{"nonexistent"},
		States: []arc.StateConfig{
			{
				Name:       "start",
				Transition: arc.Transition{Branches: map[arc.Verdict]string{"": "start"}},
			},
		},
	}
	errs := Validate(w)
	if len(errs) == 0 {
		t.Fatal("expected validation errors for nonexistent terminal state, got none")
	}
	found := false
	for _, e := range errs {
		msg := e.Error()
		if strings.Contains(msg, "terminal") && (strings.Contains(msg, "not found") || strings.Contains(msg, "does not exist")) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected error about terminal state not found, got: %v", errs)
	}
}

func TestValidateExtraBranchKey(t *testing.T) {
	// State has verdicts [approved] but branches has approved AND gaps_found
	w := &arc.Workflow{
		Name:           "extra-branch",
		EntryState:     "review",
		TerminalStates: []string{"done"},
		States: []arc.StateConfig{
			{
				Name:     "review",
				Verdicts: []string{"approved"},
				Transition: arc.Transition{Branches: map[arc.Verdict]string{
					arc.VerdictApproved:  "done",
					arc.VerdictGapsFound: "review",
				}},
			},
			{Name: "done"},
		},
	}
	errs := Validate(w)
	if len(errs) == 0 {
		t.Fatal("expected validation errors for extra branch key, got none")
	}
	found := false
	for _, e := range errs {
		msg := e.Error()
		if strings.Contains(msg, "branch") || strings.Contains(msg, "mismatch") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected error about extra branch key mismatch, got: %v", errs)
	}
}

func TestValidateNilWorkflow(t *testing.T) {
	// Validate(nil) should return an error and not panic.
	errs := Validate(nil)
	if len(errs) == 0 {
		t.Fatal("Validate(nil) should return at least one error")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "nil") || strings.Contains(e.Error(), "workflow") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Validate(nil) error should mention 'workflow' or 'nil', got: %v", errs)
	}
}

func TestValidateEntryNotFound(t *testing.T) {
	w := &arc.Workflow{
		Name:           "entry-missing",
		EntryState:     "nonexistent",
		TerminalStates: []string{"done"},
		States: []arc.StateConfig{
			{Name: "start", Transition: arc.Transition{Branches: map[arc.Verdict]string{"": "done"}}},
			{Name: "done"},
		},
	}
	errs := Validate(w)
	if len(errs) == 0 {
		t.Fatal("expected validation errors for missing entry state, got none")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "entry") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected error about entry state, got: %v", errs)
	}
}
