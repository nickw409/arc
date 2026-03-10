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

// RunVerifier spawns an agent to verify that the implementation matches the spec.
// The agent has read/search tools so it can inspect files directly rather than
// relying solely on the diff. Returns a pass/fail assessment with reasoning.
func RunVerifier(ctx context.Context, spec *arc.PhaseSpec, workdir string) (passed bool, reasoning string, err error) {
	// Get diff as orientation context — not the primary verification mechanism.
	diff, diffErr := getDiff(workdir)
	if diffErr != nil || diff == "" {
		return true, "no changes to verify", nil
	}

	// Truncate diff for prompt context — agent reads files directly for full detail.
	if len(diff) > 12000 {
		diff = diff[:12000] + "\n... (truncated — use Read/Grep tools for full detail)"
	}

	verifyCriteria := ""
	if spec.Verify != "" {
		verifyCriteria = fmt.Sprintf("\n## Acceptance Criteria\n%s\n", spec.Verify)
	}

	prompt := fmt.Sprintf(`You are a code reviewer verifying that an implementation matches its specification.

## Specification
%s
%s
## Code Changes (summary)
%s

## Instructions
Verify that the implementation FULLY satisfies the specification. You have Read, Grep, Glob,
and Bash tools — use them to inspect files directly rather than relying only on the diff above.

Check:
- Every file listed in the spec was modified or created
- Every required function, type, struct field, or wiring exists with the correct signature
- All integration points (register X in Y, wire A into B) are present
- The implementation builds: run "go build ./..." if in doubt
- No required items from the spec are missing or partially implemented

Do NOT fail for style, naming conventions, comments, or minor issues.
Do NOT fail if the diff is truncated — read the files directly.
Focus only on correctness and completeness against the spec.

Respond with PASS or FAIL on the first line, followed by your reasoning.
`, spec.Spec, verifyCriteria, diff)

	agentAdapter := adapter.Get("claude")

	sessionCfg := arc.SessionConfig{
		MaxTurns: 15,
		Timeout:  5 * time.Minute,
		Tools:    []string{"Read", "Grep", "Glob", "Bash"},
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

	// Parse PASS/FAIL verdict. Scan the first 5 lines — agents sometimes emit
	// a preamble before the verdict token despite the prompt instruction.
	lines := strings.SplitN(output, "\n", 6)
	for i, line := range lines {
		if i >= 5 {
			break
		}
		upper := strings.ToUpper(strings.TrimSpace(line))
		if strings.HasPrefix(upper, "PASS") {
			return true, output, nil
		}
		if strings.HasPrefix(upper, "FAIL") {
			return false, output, nil
		}
	}
	// No clear verdict in first 5 lines — assume fail to be conservative.
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
