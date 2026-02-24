package arc

import (
	"fmt"
	"regexp"
	"strings"
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
	VerdictBugsFound          Verdict = "bugs_found"
	VerdictNoBugsFound        Verdict = "no_bugs_found"
	VerdictUnknown            Verdict = "unknown"
)

// identifierRe matches valid verdict identifiers: lowercase alpha followed by [a-z0-9_].
var identifierRe = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// ParseVerdict normalizes raw agent output into a known Verdict.
func ParseVerdict(raw string, validVerdicts []Verdict) (Verdict, error) {
	s := strings.TrimSpace(raw)
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "`", "")
	s = strings.TrimSpace(s)
	s = strings.ToLower(s)

	if s == "" {
		return VerdictUnknown, fmt.Errorf("empty verdict")
	}

	fields := strings.Fields(s)
	token := fields[0]

	if !identifierRe.MatchString(token) {
		return VerdictUnknown, fmt.Errorf("verdict %q is not a valid identifier", token)
	}

	candidate := Verdict(token)
	for _, v := range validVerdicts {
		if candidate == v {
			return candidate, nil
		}
	}

	names := make([]string, len(validVerdicts))
	for i, v := range validVerdicts {
		names[i] = string(v)
	}
	return VerdictUnknown, fmt.Errorf("verdict %q not in valid set [%s]", token, strings.Join(names, " "))
}

// IsValid returns true if v is not VerdictUnknown and not empty.
func (v Verdict) IsValid() bool {
	return v != VerdictUnknown && v != ""
}
