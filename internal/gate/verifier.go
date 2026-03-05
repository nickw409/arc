package gate

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/nwiley/arc/internal/adapter"
	"github.com/nwiley/arc/internal/arc"
)

// RunVerifier spawns a lightweight agent to review the code diff against the phase spec.
// Returns a pass/fail assessment with reasoning.
func RunVerifier(ctx context.Context, spec *arc.PhaseSpec, workdir string) (passed bool, reasoning string, err error) {
	// Get diff of changes.
	diff, diffErr := getDiff(workdir)
	if diffErr != nil || diff == "" {
		return true, "no changes to verify", nil
	}

	// Truncate diff if too large.
	if len(diff) > 8000 {
		diff = diff[:8000] + "\n... (truncated)"
	}

	prompt := fmt.Sprintf(`You are a code reviewer verifying that an implementation matches its specification.

## Specification
%s

## Code Changes
%s

## Instructions
Review the changes and determine if they satisfy the specification.
Respond with either PASS or FAIL on the first line, followed by your reasoning.
Focus on:
- Does the implementation match what was asked for?
- Are there obvious bugs or missing pieces?
- Are the required files and functions present?

Do NOT nitpick style, naming, or minor issues. Focus on correctness and completeness.
`, spec.Spec, diff)

	agentAdapter := adapter.Get("claude")

	sessionCfg := arc.SessionConfig{
		MaxTurns: 3,
		Timeout:  2 * time.Minute,
	}

	result, spawnErr := agentAdapter.Spawn(ctx, prompt, workdir, sessionCfg)
	if spawnErr != nil {
		return false, fmt.Sprintf("verifier agent failed: %v", spawnErr), nil
	}

	output := ""
	if result != nil {
		output = result.Output
	}
	if output == "" {
		return true, "verifier produced no output (assuming pass)", nil
	}

	// Parse PASS/FAIL from first line.
	upper := strings.ToUpper(strings.TrimSpace(output))
	if strings.HasPrefix(upper, "PASS") {
		return true, output, nil
	}
	return false, output, nil
}

// getDiff returns the git diff for uncommitted changes in dir.
// Falls back to cached diff if HEAD diff fails (e.g. new repo with no commits).
func getDiff(dir string) (string, error) {
	cmd := exec.Command("git", "diff", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		// Try staged diff for new repos without a HEAD commit.
		cmd2 := exec.Command("git", "diff", "--cached")
		cmd2.Dir = dir
		out, err = cmd2.Output()
		if err != nil {
			return "", err
		}
	}
	return string(out), nil
}
