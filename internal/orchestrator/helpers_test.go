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


func TestTruncateShortString(t *testing.T) {
	got := truncate("hello", 10)
	if got != "hello" {
		t.Fatalf("truncate(%q, 10) = %q, want %q", "hello", got, "hello")
	}
}

func TestTruncateExactLength(t *testing.T) {
	got := truncate("hello", 5)
	if got != "hello" {
		t.Fatalf("truncate(%q, 5) = %q, want %q", "hello", got, "hello")
	}
}

func TestTruncateLongString(t *testing.T) {
	got := truncate("hello world", 5)
	if got != "hello" {
		t.Fatalf("truncate(%q, 5) = %q, want %q", "hello world", got, "hello")
	}
}

func TestTruncateEmptyString(t *testing.T) {
	got := truncate("", 5)
	if got != "" {
		t.Fatalf("truncate(%q, 5) = %q, want %q", "", got, "")
	}
}
