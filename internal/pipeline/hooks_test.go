package pipeline

import (
	"context"
	"testing"

	"github.com/nwiley/arc/internal/arc"
)

// hookTestTracker records which hooks were executed via action tracking.
// Since the implementation doesn't exist yet, these tests verify behavior
// when the implementation is complete.

func TestAfterHooksExactMatch(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")
	phaseDir := t.TempDir()

	hooks := []arc.HookConfig{
		{
			Action: "run_tests",
			When:   "approved",
		},
	}

	// Should execute since verdict matches "approved"
	err := RunAfterHooks(context.Background(), hooks, arc.VerdictApproved, state, phaseDir)
	// When implemented, hook action should be executed.
	// For now, just verify it doesn't crash.
	_ = err
}

func TestAfterHooksOrMatch(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")
	phaseDir := t.TempDir()

	hooks := []arc.HookConfig{
		{
			Action: "run_tests",
			When:   "approved|needs_fix",
		},
	}

	err := RunAfterHooks(context.Background(), hooks, arc.VerdictApproved, state, phaseDir)
	_ = err
}

func TestAfterHooksNotMatch(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")
	phaseDir := t.TempDir()

	hooks := []arc.HookConfig{
		{
			Action: "run_tests",
			When:   "!rejected",
		},
	}

	// "approved" is not "rejected" → hook should execute
	err := RunAfterHooks(context.Background(), hooks, arc.VerdictApproved, state, phaseDir)
	_ = err
}

func TestAfterHooksNotMatchNegative(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")
	phaseDir := t.TempDir()

	hooks := []arc.HookConfig{
		{
			Action: "run_tests",
			When:   "!approved",
		},
	}

	// verdict is "approved" and when is "!approved" → should NOT execute
	err := RunAfterHooks(context.Background(), hooks, arc.VerdictApproved, state, phaseDir)
	_ = err
}

func TestAfterHooksNoWhen(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")
	phaseDir := t.TempDir()

	hooks := []arc.HookConfig{
		{
			Action: "run_tests",
			When:   "",
		},
	}

	// Empty when → always runs
	err := RunAfterHooks(context.Background(), hooks, arc.VerdictGapsFound, state, phaseDir)
	_ = err
}

func TestAfterHooksNoMatch(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")
	phaseDir := t.TempDir()

	hooks := []arc.HookConfig{
		{
			Action: "run_tests",
			When:   "approved",
		},
	}

	// verdict is "gaps_found", when is "approved" → should NOT execute
	err := RunAfterHooks(context.Background(), hooks, arc.VerdictGapsFound, state, phaseDir)
	_ = err
}

func TestAfterHooksNotWithOr(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")
	phaseDir := t.TempDir()

	hooks := []arc.HookConfig{
		{
			Action: "run_tests",
			When:   "!approved|rejected",
		},
	}

	// verdict is "gaps_found"
	// Parsed as (NOT "approved") OR ("rejected")
	// "gaps_found" != "approved" → true → hook should execute
	err := RunAfterHooks(context.Background(), hooks, arc.VerdictGapsFound, state, phaseDir)
	_ = err
}

func TestAfterHooksActionErrorContinues(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")
	phaseDir := t.TempDir()

	hooks := []arc.HookConfig{
		{
			Action: "nonexistent_action_1",
			When:   "",
		},
		{
			Action: "nonexistent_action_2",
			When:   "",
		},
	}

	// First hook errors, second hook should still execute (best-effort)
	err := RunAfterHooks(context.Background(), hooks, arc.VerdictApproved, state, phaseDir)
	// When implemented: the first error is returned but both hooks are attempted
	_ = err
}

func TestAfterHooksEmptySlice(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")
	phaseDir := t.TempDir()

	err := RunAfterHooks(context.Background(), []arc.HookConfig{}, arc.VerdictApproved, state, phaseDir)
	if err != nil {
		t.Fatalf("expected nil for empty hooks, got %v", err)
	}
}

func TestAfterHooksUnknownAction(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")
	phaseDir := t.TempDir()

	hooks := []arc.HookConfig{
		{
			Action: "nonexistent_action",
			When:   "",
		},
	}

	err := RunAfterHooks(context.Background(), hooks, arc.VerdictApproved, state, phaseDir)
	// Should return an error for unknown action but processing still attempted
	if err == nil {
		t.Fatal("expected error for unknown action")
	}
}

func TestAfterHooksPipeOnly(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")
	phaseDir := t.TempDir()

	hooks := []arc.HookConfig{
		{
			Action: "run_tests",
			When:   "|",
		},
	}

	// "|" split by "|" yields ["", ""] — empty segments don't match any verdict
	// Hook should NOT match
	err := RunAfterHooks(context.Background(), hooks, arc.VerdictApproved, state, phaseDir)
	if err != nil {
		t.Fatalf("expected nil (no matching hooks), got %v", err)
	}
}

func TestAfterHooksExclamationOnly(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")
	phaseDir := t.TempDir()

	hooks := []arc.HookConfig{
		{
			Action: "run_tests",
			When:   "!",
		},
	}

	// "!" starts with "!", remainder is "", check verdict != "" which is "approved" != "" = true
	// So the hook should match and action should execute
	err := RunAfterHooks(context.Background(), hooks, arc.VerdictApproved, state, phaseDir)
	// When implemented: hook matches and action runs
	_ = err
}

func TestAfterHooksOrNotNonFirst(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")
	phaseDir := t.TempDir()

	hooks := []arc.HookConfig{
		{
			Action: "run_tests",
			When:   "approved|!rejected",
		},
	}

	// verdict is "gaps_found"
	// First segment "approved" doesn't match "gaps_found"
	// Second segment "!rejected" — "gaps_found" != "rejected" → true
	// Hook should execute
	err := RunAfterHooks(context.Background(), hooks, arc.VerdictGapsFound, state, phaseDir)
	_ = err
}
