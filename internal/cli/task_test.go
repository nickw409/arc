package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// --- taskPlanName tests ---

func TestTaskPlanName_BasicSlugification(t *testing.T) {
	dir := t.TempDir()
	name := taskPlanName("Add JWT authentication to the API", dir)
	// Stop words ("to", "the") removed; first 4 significant words taken.
	// Expected: "add-jwt-authentication-api"
	if name != "add-jwt-authentication-api" {
		t.Errorf("got %q, want %q", name, "add-jwt-authentication-api")
	}
}

func TestTaskPlanName_NonAlphanumericStripped(t *testing.T) {
	dir := t.TempDir()
	name := taskPlanName("Fix: the broken test!", dir)
	// ":" and "!" stripped; "the" is a stop word.
	// significant: ["fix", "broken", "test"] → "fix-broken-test"
	if name != "fix-broken-test" {
		t.Errorf("got %q, want %q", name, "fix-broken-test")
	}
}

func TestTaskPlanName_MaxFourWords(t *testing.T) {
	dir := t.TempDir()
	name := taskPlanName("fix auth cache proxy router handler", dir)
	// significant (no stop words): ["fix", "auth", "cache", "proxy"] (capped at 4)
	want := "fix-auth-cache-proxy"
	if name != want {
		t.Errorf("got %q, want %q", name, want)
	}
}

func TestTaskPlanName_TruncatesTo30Chars(t *testing.T) {
	dir := t.TempDir()
	// "implementation" alone is 14 chars; "authentication" is 14; "middleware" is 10.
	// With stop words removed: "implementation", "authentication", "middleware", "layer"
	// Joined: "implementation-authentication-m" (truncated at 30) → trim trailing "-"
	name := taskPlanName("implementation authentication middleware layer", dir)
	if len(name) > 30 {
		t.Errorf("name %q exceeds 30 chars (%d)", name, len(name))
	}
}

func TestTaskPlanName_FallbackOnEmptyResult(t *testing.T) {
	dir := t.TempDir()
	// All stop words → empty significant list → falls back to "task-plan".
	name := taskPlanName("the a an", dir)
	if name != "task-plan" {
		t.Errorf("got %q, want %q", name, "task-plan")
	}
}

func TestTaskPlanName_ConflictSuffix(t *testing.T) {
	dir := t.TempDir()
	// Pre-create the directory that would be the first candidate.
	first := filepath.Join(dir, "fix-flaky-test")
	if err := os.MkdirAll(first, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	name := taskPlanName("Fix flaky test", dir)
	// First candidate "fix-flaky-test" is taken; should get "fix-flaky-test-2".
	if name != "fix-flaky-test-2" {
		t.Errorf("got %q, want %q", name, "fix-flaky-test-2")
	}
}

func TestTaskPlanName_ConflictSuffixChain(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"fix-flaky-test", "fix-flaky-test-2", "fix-flaky-test-3"} {
		if err := os.MkdirAll(filepath.Join(dir, n), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", n, err)
		}
	}
	name := taskPlanName("Fix flaky test", dir)
	if name != "fix-flaky-test-4" {
		t.Errorf("got %q, want %q", name, "fix-flaky-test-4")
	}
}

func TestTaskPlanName_StopWordsRemoved(t *testing.T) {
	dir := t.TempDir()
	// "add OAuth support" — "add", "oauth", "support" (no stop words here)
	name := taskPlanName("add OAuth support", dir)
	if name != "add-oauth-support" {
		t.Errorf("got %q, want %q", name, "add-oauth-support")
	}
}

func TestTaskPlanName_LowercasesInput(t *testing.T) {
	dir := t.TempDir()
	name := taskPlanName("ADD JWT AUTH", dir)
	if name != "add-jwt-auth" {
		t.Errorf("got %q, want %q", name, "add-jwt-auth")
	}
}

// --- isValidPlanName tests ---

func TestIsValidPlanName_Valid(t *testing.T) {
	cases := []string{
		"ab",
		"fix-bug",
		"add-jwt-auth",
		"task-plan",
		"abc123",
		"a1",
	}
	for _, c := range cases {
		if !isValidPlanName(c) {
			t.Errorf("isValidPlanName(%q) = false, want true", c)
		}
	}
}

func TestIsValidPlanName_Invalid(t *testing.T) {
	cases := []string{
		"",        // empty
		"a",       // too short
		"A-plan",  // uppercase first char
		"fix-bug-", // trailing hyphen
		"-fix",    // leading hyphen
		"fix bug", // space
		"1plan",   // starts with digit
	}
	for _, c := range cases {
		if isValidPlanName(c) {
			t.Errorf("isValidPlanName(%q) = true, want false", c)
		}
	}
}

// --- newTaskCmd flag tests ---

func TestNewTaskCmd_RequiresArg(t *testing.T) {
	cmd := newTaskCmd()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for no arguments, got nil")
	}
}

func TestNewTaskCmd_AcceptsDescription(t *testing.T) {
	cmd := newTaskCmd()
	if err := cmd.Args(cmd, []string{"add OAuth support"}); err != nil {
		t.Fatalf("expected args to be accepted: %v", err)
	}
}

func TestNewTaskCmd_MultipleArgs(t *testing.T) {
	cmd := newTaskCmd()
	if err := cmd.Args(cmd, []string{"fix", "the", "flaky", "TestWebSocket", "test"}); err != nil {
		t.Fatalf("expected multiple args to be accepted: %v", err)
	}
}

func TestNewTaskCmd_RunFlagDefault(t *testing.T) {
	cmd := newTaskCmd()
	val, err := cmd.Flags().GetBool("run")
	if err != nil {
		t.Fatalf("getting run flag: %v", err)
	}
	if !val {
		t.Error("expected --run default to be true")
	}
}

func TestNewTaskCmd_RunFlagFalse(t *testing.T) {
	cmd := newTaskCmd()
	if err := cmd.ParseFlags([]string{"--run=false"}); err != nil {
		t.Fatalf("unexpected flag parse error: %v", err)
	}
	val, err := cmd.Flags().GetBool("run")
	if err != nil {
		t.Fatalf("getting run flag: %v", err)
	}
	if val {
		t.Error("expected --run=false to set run to false")
	}
}

func TestNewTaskCmd_PlanNameFlag(t *testing.T) {
	cmd := newTaskCmd()
	if err := cmd.ParseFlags([]string{"--plan-name", "my-custom-plan"}); err != nil {
		t.Fatalf("unexpected flag parse error: %v", err)
	}
	val, err := cmd.Flags().GetString("plan-name")
	if err != nil {
		t.Fatalf("getting plan-name flag: %v", err)
	}
	if val != "my-custom-plan" {
		t.Errorf("plan-name = %q, want %q", val, "my-custom-plan")
	}
}

func TestNewTaskCmd_PlanNameFlagDefault(t *testing.T) {
	cmd := newTaskCmd()
	val, err := cmd.Flags().GetString("plan-name")
	if err != nil {
		t.Fatalf("getting plan-name flag: %v", err)
	}
	if val != "" {
		t.Errorf("plan-name default = %q, want empty string", val)
	}
}

func TestNewTaskCmd_TimeoutFlag(t *testing.T) {
	cmd := newTaskCmd()
	if err := cmd.ParseFlags([]string{"--timeout", "3600"}); err != nil {
		t.Fatalf("unexpected flag parse error: %v", err)
	}
	val, err := cmd.Flags().GetInt("timeout")
	if err != nil {
		t.Fatalf("getting timeout flag: %v", err)
	}
	if val != 3600 {
		t.Errorf("timeout = %d, want 3600", val)
	}
}

func TestNewTaskCmd_DefaultTimeout(t *testing.T) {
	cmd := newTaskCmd()
	val, err := cmd.Flags().GetInt("timeout")
	if err != nil {
		t.Fatalf("getting timeout flag: %v", err)
	}
	if val != 14400 {
		t.Errorf("default timeout = %d, want 14400", val)
	}
}

func TestNewTaskCmd_ModelFlag(t *testing.T) {
	cmd := newTaskCmd()
	if err := cmd.ParseFlags([]string{"--model", "opus"}); err != nil {
		t.Fatalf("unexpected flag parse error: %v", err)
	}
	val, err := cmd.Flags().GetString("model")
	if err != nil {
		t.Fatalf("getting model flag: %v", err)
	}
	if val != "opus" {
		t.Errorf("model = %q, want %q", val, "opus")
	}
}

func TestNewTaskCmd_SkipReviewFlag(t *testing.T) {
	cmd := newTaskCmd()
	if err := cmd.ParseFlags([]string{"--skip-review"}); err != nil {
		t.Fatalf("unexpected flag parse error: %v", err)
	}
	val, err := cmd.Flags().GetBool("skip-review")
	if err != nil {
		t.Fatalf("getting skip-review flag: %v", err)
	}
	if !val {
		t.Error("expected skip-review to be true")
	}
}

func TestNewTaskCmd_SkipReviewDefault(t *testing.T) {
	cmd := newTaskCmd()
	val, err := cmd.Flags().GetBool("skip-review")
	if err != nil {
		t.Fatalf("getting skip-review flag: %v", err)
	}
	if val {
		t.Error("expected skip-review default to be false")
	}
}
