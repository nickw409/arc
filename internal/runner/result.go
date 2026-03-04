package runner

import (
	"fmt"
	"strings"
	"time"
)

// TestResult is the JSON output from a test runner.
type TestResult struct {
	Total       int           `json:"total"`
	Passed      int           `json:"passed"`
	Failed      int           `json:"failed"`
	RawOutput   string        `json:"raw_output"`
	FailedNames []string      `json:"failed_names"`
	Duration    time.Duration `json:"duration"`
}

// Summary returns a one-line human-readable summary.
func (r *TestResult) Summary() string {
	dur := fmt.Sprintf("%.1fs", r.Duration.Seconds())
	if r.Total == 0 && r.Failed == 0 {
		return fmt.Sprintf("0 tests (%s)", dur)
	}
	total := r.Passed + r.Failed
	if r.Total > total {
		total = r.Total
	}
	if r.Failed == 0 {
		return fmt.Sprintf("%d/%d passed (%s)", r.Passed, total, dur)
	}
	names := ""
	if len(r.FailedNames) > 0 {
		show := r.FailedNames
		if len(show) > 10 {
			show = show[:10]
			names = ": " + strings.Join(show, ", ") + fmt.Sprintf(" (+%d more)", len(r.FailedNames)-10)
		} else {
			names = ": " + strings.Join(show, ", ")
		}
	}
	return fmt.Sprintf("%d/%d passed, %d failed (%s)%s", r.Passed, total, r.Failed, dur, names)
}
