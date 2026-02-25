package dev

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// ShouldAskQuestions returns true when the task complexity is medium or complex
// and the discovery agent produced at least one question.
func ShouldAskQuestions(complexity TaskComplexity, questions []string) bool {
	if complexity == ComplexitySimple {
		return false
	}
	return len(questions) > 0
}

// FormatDiscoverySummary formats a discovery result as a readable summary block.
func FormatDiscoverySummary(d *DiscoveryResult) string {
	var b strings.Builder

	b.WriteString("[dev] Task Summary\n")
	b.WriteString("  ")
	b.WriteString(d.TaskSummary)
	b.WriteString("\n\n")

	b.WriteString("  Complexity:  ")
	b.WriteString(string(d.Complexity))
	if d.WorkflowType != "" {
		b.WriteString(" (")
		b.WriteString(d.WorkflowType)
		b.WriteString(" workflow)")
	}
	b.WriteString("\n")

	if d.Approach != "" {
		b.WriteString("  Approach:    ")
		b.WriteString(d.Approach)
		b.WriteString("\n")
	}

	if len(d.SuggestedPhases) > 0 {
		names := make([]string, len(d.SuggestedPhases))
		for i, p := range d.SuggestedPhases {
			names[i] = p.Name
		}
		b.WriteString("  Phases:      ")
		b.WriteString(strings.Join(names, ", "))
		b.WriteString("\n")
	}

	return b.String()
}

// AskQuestions presents each question to the user and reads one line of input
// per question. Empty answers are recorded as-is (user pressed enter to skip).
func AskQuestions(questions []string, r io.Reader, w io.Writer) ([]Clarification, error) {
	scanner := bufio.NewScanner(r)
	clarifications := make([]Clarification, 0, len(questions))

	for i, q := range questions {
		fmt.Fprintf(w, "  %d. %s\n  > ", i+1, q)
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return nil, fmt.Errorf("reading answer for question %d: %w", i+1, err)
			}
			// EOF — record remaining questions with empty answers
			clarifications = append(clarifications, Clarification{
				Question: q,
				Answer:   "",
			})
			for _, remaining := range questions[i+1:] {
				clarifications = append(clarifications, Clarification{
					Question: remaining,
					Answer:   "",
				})
			}
			return clarifications, nil
		}
		clarifications = append(clarifications, Clarification{
			Question: q,
			Answer:   strings.TrimSpace(scanner.Text()),
		})
	}

	return clarifications, nil
}
