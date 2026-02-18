package arc

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

// Error formats as "[phase/state] kind: message" or "[phase/state] kind: message: cause".
func (e *PhaseError) Error() string {
	panic("not implemented")
}

// Unwrap returns the underlying Cause error, or nil.
func (e *PhaseError) Unwrap() error {
	panic("not implemented")
}
