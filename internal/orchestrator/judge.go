package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nwiley/arc/internal/agent"
	"github.com/nwiley/arc/internal/arc"
)

// judgeCommandName is the binary name used for judge/stuck-instruction agent spawning.
// Tests override this to point to a mock binary.
var judgeCommandName = "claude"

// DisputeResolution is the AI's judgment on a test dispute.
type DisputeResolution struct {
	Approve bool
	Reason  string
}

// JudgeDispute spawns a focused Claude call to determine if a test dispute is valid.
func JudgeDispute(ctx context.Context, phaseState *arc.PhaseState, phaseDir string) (*DisputeResolution, error) {
	if len(phaseState.Disputes) == 0 {
		return &DisputeResolution{Approve: false, Reason: "no disputes"}, nil
	}

	dispute := phaseState.Disputes[0]

	// Read plan.md for context
	planMD, _ := os.ReadFile(filepath.Join(phaseDir, "plan.md"))

	// Read last test output for context
	testOutput, _ := os.ReadFile(filepath.Join(phaseDir, "last_test_output.txt"))
	testOutputStr := string(testOutput)
	if len(testOutputStr) > 4000 {
		testOutputStr = testOutputStr[len(testOutputStr)-4000:]
	}

	prompt := fmt.Sprintf(`You are judging a test dispute during automated implementation.

Test name: %s
Dispute reason: %s

Plan specification (excerpt):
%s

Recent test output:
%s

Is the test wrong (the spec is being misinterpreted) or is the implementation wrong (it should be fixed to pass the test)?

Respond with exactly one line:
APPROVE_DISPUTE: <reason> — if the test is wrong and should be modified
REJECT_DISPUTE: <reason> — if the test is correct and the implementation should be fixed`,
		dispute.TestName,
		dispute.Reason,
		truncate(string(planMD), 6000),
		testOutputStr,
	)

	result, err := agent.Spawn(ctx, agent.SpawnOptions{
		Prompt:       prompt,
		CommandName:  judgeCommandName,
		MaxTurns:     1,
		Timeout:      60 * time.Second,
		AllowedTools: []string{},
	})
	if err != nil {
		return nil, fmt.Errorf("dispute judgment agent failed: %w", err)
	}

	output := strings.TrimSpace(result.Output)
	if strings.HasPrefix(strings.ToUpper(output), "APPROVE_DISPUTE") {
		reason := strings.TrimPrefix(output, "APPROVE_DISPUTE:")
		reason = strings.TrimPrefix(strings.ToUpper(output), "APPROVE_DISPUTE:")
		return &DisputeResolution{Approve: true, Reason: strings.TrimSpace(reason)}, nil
	}

	reason := strings.TrimPrefix(output, "REJECT_DISPUTE:")
	return &DisputeResolution{Approve: false, Reason: strings.TrimSpace(reason)}, nil
}

// generateStuckInstructions spawns a focused Claude call to generate instructions
// for the implementation agent when it's stuck.
func generateStuckInstructions(ctx context.Context, opts RunPhaseOptions, phaseState *arc.PhaseState) (string, error) {
	phaseDir := filepath.Join(opts.PlansDir, opts.PlanName, "phases", opts.PhaseName)

	// Read last test output
	testOutput, _ := os.ReadFile(filepath.Join(phaseDir, "last_test_output.txt"))
	testOutputStr := string(testOutput)
	if len(testOutputStr) > 4000 {
		testOutputStr = testOutputStr[len(testOutputStr)-4000:]
	}

	// Read impl reasoning if available
	reasoning, _ := os.ReadFile(filepath.Join(phaseDir, "impl_reasoning.md"))
	reasoningStr := truncate(string(reasoning), 2000)

	prompt := fmt.Sprintf(`Phase "%s" has been stuck for %d iterations. Tests: %d/%d passing.

Failing test output:
%s

Implementation reasoning so far:
%s

What specific, actionable instruction should be given to the implementation agent to help it make progress? Be concrete — reference specific test names, expected values, or code patterns. One paragraph max.`,
		opts.PhaseName,
		phaseState.StuckIterations,
		phaseState.TestsPassing,
		phaseState.TestsTotal,
		testOutputStr,
		reasoningStr,
	)

	result, err := agent.Spawn(ctx, agent.SpawnOptions{
		Prompt:       prompt,
		CommandName:  judgeCommandName,
		MaxTurns:     1,
		Timeout:      60 * time.Second,
		AllowedTools: []string{},
	})
	if err != nil {
		return "", fmt.Errorf("instruction generation failed: %w", err)
	}

	return strings.TrimSpace(result.Output), nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
