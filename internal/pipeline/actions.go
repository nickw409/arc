package pipeline

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/config"
)

// ActionContext provides dependencies needed by actions.
type ActionContext struct {
	PhaseDir string
	PlanName string
	Phase    string
	Config   *config.Config
	State    *arc.PhaseState
	ArcHome  string
}

// RunAction executes a named action with the given parameters.
func RunAction(ctx context.Context, action string, params map[string]string, actx ActionContext) error {
	switch action {
	case "run_tests":
		return runTestsAction(ctx, params, actx)
	case "commit":
		return commitAction(ctx, params, actx)
	case "switch_model":
		return switchModelAction(params, actx)
	case "analyze_stuck":
		return analyzeStuckAction(actx)
	case "request_human":
		return requestHumanAction(params, actx)
	case "script":
		return scriptAction(ctx, params, actx)
	default:
		return fmt.Errorf("unknown action: %s", action)
	}
}

func runTestsAction(ctx context.Context, params map[string]string, actx ActionContext) error {
	if actx.State == nil || len(actx.State.TestFiles) == 0 {
		return nil
	}

	runner := ""
	if actx.Config != nil {
		runner = actx.Config.Runner
	}

	scriptPath := filepath.Join(actx.ArcHome, "scripts", "run-phase-tests.sh")
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		return fmt.Errorf("test runner script not found: %s", scriptPath)
	}

	args := []string{runner}
	if pattern, ok := params["pattern"]; ok && pattern != "" {
		args = append(args, pattern)
	}

	cmd := exec.CommandContext(ctx, scriptPath, args...)
	cmd.Dir = actx.PhaseDir
	output, err := cmd.CombinedOutput()

	if saveTo, ok := params["save_to"]; ok && saveTo != "" {
		_ = os.WriteFile(filepath.Join(actx.PhaseDir, saveTo), output, 0644)
	}

	return err
}

func commitAction(ctx context.Context, params map[string]string, actx ActionContext) error {
	message := params["message"]
	if message == "" {
		message = "chore: automated commit"
	}

	args := []string{"commit", "-m", message}
	if actx.Config != nil && actx.Config.Git.Sign {
		args = append(args, "-S")
	}

	cmd := exec.CommandContext(ctx, "git", append([]string{"add", "-A"}, []string{}...)...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git add failed: %w", err)
	}

	cmd = exec.CommandContext(ctx, "git", args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git commit failed: %w", err)
	}

	return nil
}

func switchModelAction(params map[string]string, actx ActionContext) error {
	model := params["model"]
	if model == "" {
		return fmt.Errorf("switch_model requires non-empty model parameter")
	}
	actx.State.ModelOverride = model
	return nil
}

func analyzeStuckAction(actx ActionContext) error {
	content := fmt.Sprintf("# Stuck Analysis\n\nGenerated at: %s\n\nPhase: %s\nPlan: %s\nIteration: %d\nStuck iterations: %d\n",
		time.Now().UTC().Format(time.RFC3339),
		actx.State.Phase,
		actx.State.Plan,
		actx.State.Iteration.Current,
		actx.State.StuckIterations,
	)
	return os.WriteFile(filepath.Join(actx.PhaseDir, "stuck_analysis.md"), []byte(content), 0644)
}

func requestHumanAction(params map[string]string, actx ActionContext) error {
	message := params["message"]
	if message == "" {
		message = "Human intervention requested"
	}

	content := fmt.Sprintf("# Intervention Request\n\n%s\n\nRequested at: %s\n",
		message,
		time.Now().UTC().Format(time.RFC3339),
	)
	if err := os.WriteFile(filepath.Join(actx.PhaseDir, "intervention_request.md"), []byte(content), 0644); err != nil {
		return err
	}

	actx.State.InterventionRequest = &arc.Intervention{
		Reason:      message,
		RequestedAt: time.Now().UTC().Format(time.RFC3339),
	}
	return nil
}

func scriptAction(ctx context.Context, params map[string]string, actx ActionContext) error {
	path := params["path"]
	if path == "" {
		return fmt.Errorf("script action requires path parameter")
	}

	if strings.Contains(path, "..") {
		return fmt.Errorf("script path contains '..': path traversal not allowed")
	}

	fullPath := filepath.Join(actx.ArcHome, path)

	args := []string{}
	if argsStr, ok := params["args"]; ok && argsStr != "" {
		args = strings.Fields(argsStr)
	}

	cmd := exec.CommandContext(ctx, fullPath, args...)
	cmd.Dir = actx.PhaseDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("script %s failed (exit non-zero): %s: %w", path, string(output), err)
	}
	return nil
}
