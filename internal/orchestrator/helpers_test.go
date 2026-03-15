package orchestrator

import (
	"testing"
)

func TestStringOrDefaultWithValue(t *testing.T) {
	s := "hello"
	got := stringOrDefault(&s, "default")
	if got != "hello" {
		t.Fatalf("stringOrDefault(&%q, %q) = %q, want %q", s, "default", got, "hello")
	}
}

func TestStringOrDefaultNil(t *testing.T) {
	got := stringOrDefault(nil, "default")
	if got != "default" {
		t.Fatalf("stringOrDefault(nil, %q) = %q, want %q", "default", got, "default")
	}
}

func TestStringOrDefaultEmptyString(t *testing.T) {
	s := ""
	got := stringOrDefault(&s, "default")
	if got != "" {
		t.Fatalf("stringOrDefault(&%q, %q) = %q, want %q", s, "default", got, "")
	}
}


