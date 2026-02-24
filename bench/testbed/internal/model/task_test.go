package model

import (
	"testing"
)

func TestParseStatus(t *testing.T) {
	tests := []struct {
		input string
		want  Status
		err   bool
	}{
		{"pending", StatusPending, false},
		{"active", StatusActive, false},
		{"completed", StatusCompleted, false},
		{"invalid", "", true},
	}

	for _, tt := range tests {
		got, err := ParseStatus(tt.input)
		if tt.err && err == nil {
			t.Errorf("ParseStatus(%q) expected error", tt.input)
		}
		if !tt.err && err != nil {
			t.Errorf("ParseStatus(%q) unexpected error: %v", tt.input, err)
		}
		if got != tt.want {
			t.Errorf("ParseStatus(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParsePriority(t *testing.T) {
	tests := []struct {
		input string
		want  Priority
		err   bool
	}{
		{"low", PriorityLow, false},
		{"medium", PriorityMedium, false},
		{"med", PriorityMedium, false},
		{"high", PriorityHigh, false},
		{"none", PriorityNone, false},
		{"", PriorityNone, false},
		{"critical", 0, true},
	}

	for _, tt := range tests {
		got, err := ParsePriority(tt.input)
		if tt.err && err == nil {
			t.Errorf("ParsePriority(%q) expected error", tt.input)
		}
		if !tt.err && err != nil {
			t.Errorf("ParsePriority(%q) unexpected error: %v", tt.input, err)
		}
		if got != tt.want {
			t.Errorf("ParsePriority(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestPriorityString(t *testing.T) {
	tests := []struct {
		p    Priority
		want string
	}{
		{PriorityLow, "low"},
		{PriorityMedium, "medium"},
		{PriorityHigh, "high"},
		{PriorityNone, "none"},
	}

	for _, tt := range tests {
		if got := tt.p.String(); got != tt.want {
			t.Errorf("Priority(%d).String() = %q, want %q", tt.p, got, tt.want)
		}
	}
}

func TestStatusValues(t *testing.T) {
	if StatusPending != "pending" {
		t.Error("StatusPending should be 'pending'")
	}
	if StatusActive != "active" {
		t.Error("StatusActive should be 'active'")
	}
	if StatusCompleted != "completed" {
		t.Error("StatusCompleted should be 'completed'")
	}
}
