package review

import (
	"strings"
)

// adversaryPriority defines the fixed priority order for conflict resolution.
// Lower number = higher priority. Executability issues are most fundamental,
// scope issues are least critical.
var adversaryPriority = map[string]int{
	"executability": 0,
	"consistency":   1,
	"coverage":      2,
	"ambiguity":     3,
	"scope":         4,
}

// Suggestion represents a single find-and-replace suggestion from an adversary.
type Suggestion struct {
	Adversary string
	Priority  int
	Original  string
	Suggested string
}

// ParseSuggestions extracts <<<ORIGINAL/<<<SUGGESTED blocks from adversary output.
func ParseSuggestions(adversaryName string, output string) []Suggestion {
	priority := adversaryPriority[adversaryName]

	var suggestions []Suggestion
	lines := strings.Split(output, "\n")

	i := 0
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed != "<<<ORIGINAL" {
			i++
			continue
		}

		// Collect ORIGINAL block
		i++
		var origLines []string
		for i < len(lines) {
			if strings.TrimSpace(lines[i]) == ">>>" {
				i++
				break
			}
			origLines = append(origLines, lines[i])
			i++
		}

		// Expect <<<SUGGESTED
		for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
			i++
		}
		if i >= len(lines) || strings.TrimSpace(lines[i]) != "<<<SUGGESTED" {
			continue
		}

		// Collect SUGGESTED block
		i++
		var sugLines []string
		for i < len(lines) {
			if strings.TrimSpace(lines[i]) == ">>>" {
				i++
				break
			}
			sugLines = append(sugLines, lines[i])
			i++
		}

		original := strings.Join(origLines, "\n")
		suggested := strings.Join(sugLines, "\n")

		if original == "" || original == suggested {
			continue
		}

		suggestions = append(suggestions, Suggestion{
			Adversary: adversaryName,
			Priority:  priority,
			Original:  original,
			Suggested: suggested,
		})
	}

	return suggestions
}

// MergeSuggestions takes suggestions from multiple adversaries, sorts by priority,
// and drops lower-priority suggestions whose Original text overlaps with a
// higher-priority suggestion's Original text.
func MergeSuggestions(suggestions []Suggestion) []Suggestion {
	if len(suggestions) == 0 {
		return nil
	}

	// Sort by priority (stable to preserve order within same adversary)
	sorted := make([]Suggestion, len(suggestions))
	copy(sorted, suggestions)
	stableSortSuggestions(sorted)

	var merged []Suggestion
	for _, s := range sorted {
		conflict := false
		for _, accepted := range merged {
			if overlaps(accepted.Original, s.Original) {
				conflict = true
				break
			}
		}
		if !conflict {
			merged = append(merged, s)
		}
	}

	return merged
}

// ApplySuggestions applies merged suggestions to plan content via string replacement.
// Returns the modified content and the count of suggestions that were successfully applied.
func ApplySuggestions(planMD string, suggestions []Suggestion) (string, int) {
	applied := 0
	for _, s := range suggestions {
		if strings.Contains(planMD, s.Original) {
			planMD = strings.Replace(planMD, s.Original, s.Suggested, 1)
			applied++
		}
	}
	return planMD, applied
}

// overlaps returns true if the two strings share any common text that would
// cause a conflict when both are used as find targets in the same document.
// A conflict occurs when one original contains the other, or they are identical.
func overlaps(a, b string) bool {
	return a == b || strings.Contains(a, b) || strings.Contains(b, a)
}

// stableSortSuggestions sorts suggestions by priority (ascending) using insertion sort
// for stability.
func stableSortSuggestions(s []Suggestion) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j].Priority < s[j-1].Priority; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
