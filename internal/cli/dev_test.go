package cli

import (
	"testing"
)

func TestNewDevCmd_RequiresArg(t *testing.T) {
	cmd := newDispatchCmd()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for no arguments, got nil")
	}
}

func TestNewDevCmd_AcceptsDescription(t *testing.T) {
	cmd := newDispatchCmd()
	// We just test that argument parsing succeeds — execution will fail
	// because there's no project context, but that's fine.
	err := cmd.ParseFlags([]string{})
	if err != nil {
		t.Fatalf("unexpected flag parse error: %v", err)
	}
	if err := cmd.Args(cmd, []string{"add OAuth"}); err != nil {
		t.Fatalf("expected args to be accepted: %v", err)
	}
}

func TestNewDevCmd_MultipleArgs(t *testing.T) {
	cmd := newDispatchCmd()
	if err := cmd.Args(cmd, []string{"add", "OAuth", "support"}); err != nil {
		t.Fatalf("expected multiple args to be accepted: %v", err)
	}
}

func TestNewDevCmd_InteractiveFlag(t *testing.T) {
	cmd := newDispatchCmd()
	cmd.SetArgs([]string{"--interactive", "test task"})
	if err := cmd.ParseFlags([]string{"--interactive"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	val, err := cmd.Flags().GetBool("interactive")
	if err != nil {
		t.Fatalf("getting interactive flag: %v", err)
	}
	if !val {
		t.Error("expected interactive to be true")
	}
}

func TestNewDevCmd_SkipReviewFlag(t *testing.T) {
	cmd := newDispatchCmd()
	if err := cmd.ParseFlags([]string{"--skip-review"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	val, err := cmd.Flags().GetBool("skip-review")
	if err != nil {
		t.Fatalf("getting skip-review flag: %v", err)
	}
	if !val {
		t.Error("expected skip-review to be true")
	}
}

func TestNewDevCmd_TimeoutFlag(t *testing.T) {
	cmd := newDispatchCmd()
	if err := cmd.ParseFlags([]string{"--timeout", "3600"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	val, err := cmd.Flags().GetInt("timeout")
	if err != nil {
		t.Fatalf("getting timeout flag: %v", err)
	}
	if val != 3600 {
		t.Errorf("timeout = %d, want 3600", val)
	}
}

func TestNewDevCmd_ModelFlag(t *testing.T) {
	cmd := newDispatchCmd()
	if err := cmd.ParseFlags([]string{"--model", "sonnet"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	val, err := cmd.Flags().GetString("model")
	if err != nil {
		t.Fatalf("getting model flag: %v", err)
	}
	if val != "sonnet" {
		t.Errorf("model = %q, want %q", val, "sonnet")
	}
}

func TestNewDevCmd_DefaultTimeout(t *testing.T) {
	cmd := newDispatchCmd()
	val, err := cmd.Flags().GetInt("timeout")
	if err != nil {
		t.Fatalf("getting timeout flag: %v", err)
	}
	if val != 14400 {
		t.Errorf("default timeout = %d, want 14400", val)
	}
}
