package arc

import "time"

// GateResult is the outcome of running an arc gate check.
type GateResult struct {
	Passed     bool              `json:"passed"`
	Assertions []AssertionResult `json:"assertions"`
	TestOutput string            `json:"test_output,omitempty"`
	RunCount   int               `json:"run_count"`
	Timestamp  time.Time         `json:"timestamp"`
}

// AssertionResult is the result of a single gate assertion.
type AssertionResult struct {
	Type    string `json:"type"`    // file_exists, grep, test_exists, test_pass
	Target  string `json:"target"`  // what was checked (file path, pattern, test name)
	Passed  bool   `json:"passed"`
	Detail  string `json:"detail,omitempty"` // human-readable explanation on failure
}

// GateAssertion defines a single verification check in a phase gate.
type GateAssertion struct {
	FileExists string `json:"file_exists,omitempty" yaml:"file_exists,omitempty"`
	Grep       string `json:"grep,omitempty" yaml:"grep,omitempty"`
	TestExists string `json:"test_exists,omitempty" yaml:"test_exists,omitempty"`
}

// Type returns the assertion type string.
func (a GateAssertion) Type() string {
	switch {
	case a.FileExists != "":
		return "file_exists"
	case a.Grep != "":
		return "grep"
	case a.TestExists != "":
		return "test_exists"
	default:
		return "unknown"
	}
}

// Target returns the assertion target value.
func (a GateAssertion) Target() string {
	switch {
	case a.FileExists != "":
		return a.FileExists
	case a.Grep != "":
		return a.Grep
	case a.TestExists != "":
		return a.TestExists
	default:
		return ""
	}
}

// CheckpointStatus tracks the verification state of a single checkpoint.
type CheckpointStatus struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"` // pass, fail, not_found
	TestOutput  string `json:"test_output,omitempty"`
}

// GateStatus is the persistent gate state written to gate-status.json.
type GateStatus struct {
	LastRun     time.Time                  `json:"last_run"`
	RunCount    int                        `json:"run_count"`
	Checkpoints map[string]string          `json:"checkpoints"` // name → pass/fail/not_found
}
