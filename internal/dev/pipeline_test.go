package dev

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestGeneratePlanName_Basic(t *testing.T) {
	dir := t.TempDir()
	got := GeneratePlanName("Add OAuth authentication to the API", dir)
	if got != "add-oauth-authentication-api" {
		t.Errorf("got %q, want %q", got, "add-oauth-authentication-api")
	}
}

func TestGeneratePlanName_LongDescription(t *testing.T) {
	dir := t.TempDir()
	got := GeneratePlanName("Implement a comprehensive user authentication system with OAuth support and session management", dir)
	if len(got) > 40 {
		t.Errorf("name too long: %d chars, want <= 40", len(got))
	}
	if !planNameValidRe.MatchString(got) {
		t.Errorf("name %q does not match plan name regex", got)
	}
}

func TestGeneratePlanName_ShortDescription(t *testing.T) {
	dir := t.TempDir()
	got := GeneratePlanName("Fix typo", dir)
	if got != "fix-typo" {
		t.Errorf("got %q, want %q", got, "fix-typo")
	}
}

func TestGeneratePlanName_SpecialChars(t *testing.T) {
	dir := t.TempDir()
	got := GeneratePlanName("Fix the bug in user's profile (issue #123)", dir)
	if !planNameValidRe.MatchString(got) {
		t.Errorf("name %q does not match plan name regex", got)
	}
}

func TestGeneratePlanName_Conflict(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "fix-typo"), 0755)
	got := GeneratePlanName("Fix typo", dir)
	if got != "fix-typo-2" {
		t.Errorf("got %q, want %q", got, "fix-typo-2")
	}
}

func TestGeneratePlanName_MultipleConflicts(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "fix-typo"), 0755)
	os.MkdirAll(filepath.Join(dir, "fix-typo-2"), 0755)
	got := GeneratePlanName("Fix typo", dir)
	if got != "fix-typo-3" {
		t.Errorf("got %q, want %q", got, "fix-typo-3")
	}
}

func TestGeneratePlanName_AllStopWords(t *testing.T) {
	dir := t.TempDir()
	got := GeneratePlanName("the a an to in for", dir)
	if len(got) < 4 || got[:4] != "dev-" {
		t.Errorf("expected fallback name starting with 'dev-', got %q", got)
	}
}

func TestGeneratePlanName_EmptyDescription(t *testing.T) {
	dir := t.TempDir()
	got := GeneratePlanName("", dir)
	if len(got) < 4 || got[:4] != "dev-" {
		t.Errorf("expected fallback name starting with 'dev-', got %q", got)
	}
}

func TestGeneratePlanName_NumbersOnly(t *testing.T) {
	dir := t.TempDir()
	got := GeneratePlanName("123 456 789", dir)
	if !planNameValidRe.MatchString(got) {
		t.Errorf("name %q does not match plan name regex", got)
	}
}

func TestGeneratePlanName_VeryLongDescription(t *testing.T) {
	dir := t.TempDir()
	desc := ""
	for i := 0; i < 200; i++ {
		desc += "word "
	}
	got := GeneratePlanName(desc, dir)
	if len(got) > 40 {
		t.Errorf("name too long: %d chars, want <= 40", len(got))
	}
	if !planNameValidRe.MatchString(got) {
		t.Errorf("name %q does not match plan name regex", got)
	}
}

func TestGeneratePlanName_DoubleDigitSuffix(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "fix-typo"), 0755)
	for i := 2; i <= 9; i++ {
		os.MkdirAll(filepath.Join(dir, fmt.Sprintf("fix-typo-%d", i)), 0755)
	}
	got := GeneratePlanName("Fix typo", dir)
	if got != "fix-typo-10" {
		t.Errorf("got %q, want %q", got, "fix-typo-10")
	}
}

func TestGeneratePlanName_UnicodeChars(t *testing.T) {
	dir := t.TempDir()
	got := GeneratePlanName("Fix café résumé bug", dir)
	if !planNameValidRe.MatchString(got) {
		t.Errorf("name %q does not match plan name regex", got)
	}
}

func TestGeneratePlanName_SingleWord(t *testing.T) {
	dir := t.TempDir()
	got := GeneratePlanName("refactor", dir)
	// Single word doesn't match regex (needs at least 2 chars with pattern [a-z][a-z0-9-]*[a-z0-9])
	// "refactor" has 8 chars, should match
	if !planNameValidRe.MatchString(got) {
		t.Errorf("name %q does not match plan name regex", got)
	}
}
