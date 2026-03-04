package gate

import (
	"fmt"
	"strings"

	"github.com/nwiley/arc/internal/arc"
)

const loopDetectionThreshold = 10

// Format returns a human-readable summary of the gate result for agent consumption.
//
// Example (all pass):
//
//	PASS
//	- [x] File internal/api/auth.go exists
//	- [x] func NewMiddleware found
//	- [x] TestTokenExpiry exists
//	- [x] Scoped tests: passing
//
// Example (failures):
//
//	FAIL
//	- [x] File internal/api/auth.go exists
//	- [ ] func NewMiddleware not found
//	- [ ] TestTokenExpiry not found in any _test.go file
//	- [x] Scoped tests: passing
//
//	Fix the items above, then run this command again.
func Format(result *arc.GateResult) string {
	return FormatWithRunCount(result, 0)
}

// FormatWithRunCount formats the gate result and optionally emits a loop
// detection warning when runCount exceeds the threshold.
func FormatWithRunCount(result *arc.GateResult, runCount int) string {
	var sb strings.Builder

	if result.Passed {
		sb.WriteString("PASS\n")
	} else {
		sb.WriteString("FAIL\n")
	}

	// Assertion items.
	for _, a := range result.Assertions {
		check := checkmark(a.Passed)
		label := a.Description
		if label == "" {
			label = a.Detail
		}
		if !a.Passed && a.Detail != "" {
			label = fmt.Sprintf("%s — %s", label, a.Detail)
		}
		sb.WriteString(fmt.Sprintf("- [%s] %s\n", check, label))
	}

	// Checkpoint items.
	for _, cp := range result.Checkpoints {
		if cp.Status == "not_run" {
			sb.WriteString(fmt.Sprintf("- [-] Checkpoint %q: no test command\n", cp.Name))
			continue
		}
		passed := cp.Status == "pass"
		check := checkmark(passed)
		sb.WriteString(fmt.Sprintf("- [%s] Checkpoint %q: %s\n", check, cp.Name, cp.Status))
	}

	// Scoped test item.
	if !result.ScopedTestSkipped {
		check := checkmark(result.ScopedTestPassed)
		if result.ScopedTestPassed {
			sb.WriteString(fmt.Sprintf("- [%s] Scoped tests: passing\n", check))
		} else {
			sb.WriteString(fmt.Sprintf("- [%s] Scoped tests: FAILED\n", check))
			if result.ScopedTestOutput != "" {
				sb.WriteString("\nTest output:\n")
				sb.WriteString(result.ScopedTestOutput)
				if !strings.HasSuffix(result.ScopedTestOutput, "\n") {
					sb.WriteString("\n")
				}
			}
		}
	}

	// Failure footer.
	if !result.Passed {
		sb.WriteString("\nFix the items above, then run this command again.\n")
	}

	// Loop detection warning.
	if runCount > loopDetectionThreshold {
		sb.WriteString(fmt.Sprintf(
			"\nWARNING: Gate has been run %d times this session with repeated failures.\n"+
				"The same assertions are failing. Stop and reconsider your approach.\n",
			runCount,
		))
	}

	return sb.String()
}

// checkmark returns "x" for pass and " " for fail, matching the bracket format.
func checkmark(passed bool) string {
	if passed {
		return "x"
	}
	return " "
}
