package arc

// ResultAction tells the orchestrator what to do next.
type ResultAction int

const (
	ActionContinue  ResultAction = iota // advance to next state
	ActionRetry                         // same state, next iteration
	ActionEscalate                      // trigger escalation ladder
	ActionIntervene                     // stop and wait for human
	ActionAbort                         // unrecoverable
)

var resultActionNames = [...]string{
	ActionContinue:  "continue",
	ActionRetry:     "retry",
	ActionEscalate:  "escalate",
	ActionIntervene: "intervene",
	ActionAbort:     "abort",
}

// String returns the human-readable name of the action.
func (a ResultAction) String() string {
	if int(a) < 0 || int(a) >= len(resultActionNames) {
		return "unknown"
	}
	return resultActionNames[a]
}

// IterationResult is the outcome of a single iteration.
type IterationResult struct {
	NextState string       // empty if no transition
	Verdict   Verdict      // the parsed verdict, if any
	Action    ResultAction // what the orchestrator should do
	Err       error        // underlying error, if any
}
