package arc

import (
	"regexp"
)

// Verdict represents a parsed agent verdict that drives state transitions.
type Verdict string

const (
	VerdictApproved           Verdict = "approved"
	VerdictGapsFound          Verdict = "gaps_found"
	VerdictConcerns           Verdict = "concerns"
	VerdictUnambiguous        Verdict = "unambiguous"
	VerdictAmbiguous          Verdict = "ambiguous"
	VerdictConsistent         Verdict = "consistent"
	VerdictInconsistent       Verdict = "inconsistent"
	VerdictCoverageSufficient Verdict = "coverage_sufficient"
	VerdictCoverageGaps       Verdict = "coverage_gaps"
	VerdictExecutable         Verdict = "executable"
	VerdictBlocked            Verdict = "blocked"
	VerdictScopeAppropriate   Verdict = "scope_appropriate"
	VerdictScopeTooLarge      Verdict = "scope_too_large"
	VerdictUnknown            Verdict = "unknown"
)

// identifierRe matches valid verdict identifiers: lowercase alpha followed by [a-z0-9_].
var identifierRe = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// ParseVerdict normalizes raw agent output into a known Verdict.
func ParseVerdict(raw string, validVerdicts []Verdict) (Verdict, error) {
	panic("not implemented")
}

// IsValid returns true if v is not VerdictUnknown and not empty.
func (v Verdict) IsValid() bool {
	panic("not implemented")
}
