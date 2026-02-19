package runner

// TestResult is the JSON output from a test runner.
type TestResult struct {
	Total       int      `json:"total"`
	Passed      int      `json:"passed"`
	Failed      int      `json:"failed"`
	RawOutput   string   `json:"raw_output"`
	FailedNames []string `json:"failed_names"`
}
