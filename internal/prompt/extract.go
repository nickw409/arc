package prompt

import (
	"fmt"
	"strings"

	"github.com/nwiley/arc/internal/arc"
)

// ExtractVerdict extracts a verdict from raw agent output text.
func ExtractVerdict(output string, validVerdicts []arc.Verdict) (arc.Verdict, error) {
	lines := strings.Split(output, "\n")
	inCodeBlock := false
	lastVerdictIdx := -1

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Only non-indented backtick fences toggle code block state.
		// Lines with 4+ leading spaces are not fences.
		if strings.HasPrefix(trimmed, "```") && !strings.HasPrefix(line, "    ") {
			inCodeBlock = !inCodeBlock
			continue
		}

		if inCodeBlock {
			continue
		}

		// Match ## Verdict or ### Verdict (case-insensitive).
		lower := strings.ToLower(trimmed)
		if lower == "## verdict" || lower == "### verdict" {
			lastVerdictIdx = i
		}
	}

	if lastVerdictIdx == -1 {
		return arc.VerdictUnknown, fmt.Errorf("no verdict section found")
	}

	// Extract the first non-empty line after the verdict header.
	for i := lastVerdictIdx + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			continue
		}
		return arc.ParseVerdict(trimmed, validVerdicts)
	}

	return arc.VerdictUnknown, fmt.Errorf("no verdict value after header")
}
