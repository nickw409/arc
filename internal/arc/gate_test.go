package arc

import (
	"encoding/json"
	"testing"
	"time"
)

func TestGateAssertionType(t *testing.T) {
	tests := []struct {
		name string
		a    GateAssertion
		want string
	}{
		{"file_exists", GateAssertion{FileExists: "foo.go"}, "file_exists"},
		{"grep", GateAssertion{Grep: "func New"}, "grep"},
		{"test_exists", GateAssertion{TestExists: "TestFoo"}, "test_exists"},
		{"unknown", GateAssertion{}, "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.a.Type(); got != tt.want {
				t.Errorf("Type() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGateAssertionTarget(t *testing.T) {
	tests := []struct {
		name string
		a    GateAssertion
		want string
	}{
		{"file_exists", GateAssertion{FileExists: "foo.go"}, "foo.go"},
		{"grep", GateAssertion{Grep: "func New"}, "func New"},
		{"test_exists", GateAssertion{TestExists: "TestFoo"}, "TestFoo"},
		{"empty", GateAssertion{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.a.Target(); got != tt.want {
				t.Errorf("Target() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGateResultJSON(t *testing.T) {
	r := GateResult{
		Passed: true,
		Assertions: []AssertionResult{
			{Type: "file_exists", Target: "foo.go", Passed: true},
			{Type: "grep", Target: "func New", Passed: false, Detail: "pattern not found"},
		},
		RunCount:  3,
		Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var r2 GateResult
	if err := json.Unmarshal(data, &r2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if r2.Passed != r.Passed {
		t.Errorf("Passed = %v, want %v", r2.Passed, r.Passed)
	}
	if len(r2.Assertions) != 2 {
		t.Fatalf("len(Assertions) = %d, want 2", len(r2.Assertions))
	}
	if r2.Assertions[1].Detail != "pattern not found" {
		t.Errorf("Detail = %q, want %q", r2.Assertions[1].Detail, "pattern not found")
	}
	if r2.RunCount != 3 {
		t.Errorf("RunCount = %d, want 3", r2.RunCount)
	}
}

func TestGateStatusJSON(t *testing.T) {
	s := GateStatus{
		LastRun:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		RunCount:    5,
		Checkpoints: map[string]string{"auth": "pass", "validate": "fail"},
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var s2 GateStatus
	if err := json.Unmarshal(data, &s2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if s2.Checkpoints["auth"] != "pass" {
		t.Errorf("checkpoint auth = %q, want %q", s2.Checkpoints["auth"], "pass")
	}
}
