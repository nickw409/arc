package arc

import "fmt"

// PhaseError represents errors that can occur during phase execution.
type PhaseError struct {
	Phase   string
	State   string
	Kind    PhaseErrorKind
	Message string
	Cause   error
}

type PhaseErrorKind int

const (
	ErrIteration    PhaseErrorKind = iota // iteration-level failure
	ErrConstraint                         // constraint violation
	ErrEscalation                         // escalation trigger
	ErrIntervention                       // human intervention needed
	ErrSubprocess                         // subprocess crash/timeout
	ErrStateParse                         // state file corrupt
	ErrVerdictParse                       // verdict unparseable
	ErrWorkflow                           // workflow definition error
)

var phaseErrorKindNames = [...]string{
	ErrIteration:    "iteration",
	ErrConstraint:   "constraint",
	ErrEscalation:   "escalation",
	ErrIntervention: "intervention",
	ErrSubprocess:   "subprocess",
	ErrStateParse:   "state_parse",
	ErrVerdictParse: "verdict_parse",
	ErrWorkflow:     "workflow",
}

// Error formats as "[phase/state] kind: message" or "[phase/state] kind: message: cause".
func (e *PhaseError) Error() string {
	s := fmt.Sprintf("[%s/%s] %s: %s", e.Phase, e.State, phaseErrorKindNames[e.Kind], e.Message)
	if e.Cause != nil {
		s += ": " + e.Cause.Error()
	}
	return s
}

// Unwrap returns the underlying Cause error, or nil.
func (e *PhaseError) Unwrap() error {
	return e.Cause
}
