package arc

import (
	"encoding/json"
	"testing"
)

func TestGateAssertionFields(t *testing.T) {
	tests := []struct {
		name string
		a    GateAssertion
	}{
		{"file_exists", GateAssertion{FileExists: "foo.go"}},
		{"grep", GateAssertion{Grep: "func New"}},
		{"test_exists", GateAssertion{TestExists: "TestFoo"}},
		{"legacy_type", GateAssertion{Type: "file_exists", Target: "bar.go"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.a)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var a2 GateAssertion
			if err := json.Unmarshal(data, &a2); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
		})
	}
}

func TestGateResultJSON(t *testing.T) {
	r := GateResult{
		Passed: true,
		Assertions: []AssertionResult{
			{Description: "file check", Passed: true},
			{Description: "grep check", Passed: false, Detail: "pattern not found"},
		},
		Checkpoints: []CheckpointStatus{
			{Name: "auth", Status: "pass"},
			{Name: "validate", Status: "fail", Output: "error output"},
		},
		ScopedTestPassed:  true,
		ScopedTestSkipped: false,
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
	if len(r2.Checkpoints) != 2 {
		t.Fatalf("len(Checkpoints) = %d, want 2", len(r2.Checkpoints))
	}
	if r2.Checkpoints[1].Output != "error output" {
		t.Errorf("Output = %q, want %q", r2.Checkpoints[1].Output, "error output")
	}
}

func TestGateStatusJSON(t *testing.T) {
	s := GateStatus{
		LastRun:     "2026-01-01T00:00:00Z",
		RunCount:    5,
		Passed:      true,
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
	if !s2.Passed {
		t.Error("Passed = false, want true")
	}
}

func TestNewGateStatus(t *testing.T) {
	result := &GateResult{
		Passed: true,
		Checkpoints: []CheckpointStatus{
			{Name: "a", Status: "pass"},
			{Name: "b", Status: "fail"},
		},
	}
	status := NewGateStatus(result)
	if !status.Passed {
		t.Error("Passed = false, want true")
	}
	if status.RunCount != 1 {
		t.Errorf("RunCount = %d, want 1", status.RunCount)
	}
	if status.Checkpoints["a"] != "pass" {
		t.Errorf("checkpoint a = %q, want pass", status.Checkpoints["a"])
	}
	if status.Checkpoints["b"] != "fail" {
		t.Errorf("checkpoint b = %q, want fail", status.Checkpoints["b"])
	}
}
