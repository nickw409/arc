package review

import (
	"regexp"
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

// isBlockCloser returns true if the line is a block-closing delimiter.
// Accepts both ">>>" and "<<<END" which LLMs commonly produce.
func isBlockCloser(trimmed string) bool {
	return trimmed == ">>>" || trimmed == "<<<END"
}

// debrisPattern matches adversary analysis headings that LLMs inject into
// suggested replacement text. These are the adversary's own section headers
// (e.g. "### Fix 1: Clarify instructions") and editorial comments
// (e.g. "**(REMOVED — covered elsewhere)**") that should not appear in plan content.
var debrisPattern = regexp.MustCompile(
	`(?m)^###\s+(?:Fix|Issue|Gap|Suggestion)\s+\d+\s*:.*$|` +
		`(?m)^\*\*\((?:REMOVED|CHANGED|ADDED|NOTE)\s*[—–-].*\)\*\*$`,
)

// cleanSuggested strips adversary analysis debris from suggested replacement text.
// When adversaries produce <<<SUGGESTED blocks, they sometimes include their own
// section headings (### Fix 1: ...) or editorial comments (**(REMOVED — ...)** )
// that are not plan content and cause oscillation between adversaries.
func cleanSuggested(s string) string {
	cleaned := debrisPattern.ReplaceAllString(s, "")
	// Collapse runs of 3+ blank lines down to 2
	for strings.Contains(cleaned, "\n\n\n") {
		cleaned = strings.ReplaceAll(cleaned, "\n\n\n", "\n\n")
	}
	return strings.TrimRight(cleaned, "\n ")
}

// stripCodeFences removes leading/trailing code fence lines (```lang / ```)
// from a block of text. LLMs sometimes wrap suggestion content in code fences.
func stripCodeFences(s string) string {
	lines := strings.Split(s, "\n")
	if len(lines) < 2 {
		return s
	}
	first := strings.TrimSpace(lines[0])
	last := strings.TrimSpace(lines[len(lines)-1])
	if strings.HasPrefix(first, "```") && last == "```" {
		return strings.Join(lines[1:len(lines)-1], "\n")
	}
	return s
}

// ParseSuggestions extracts <<<ORIGINAL/<<<SUGGESTED blocks from adversary output.
// The parser is lenient about block closing: it accepts >>>, <<<END, and also
// treats <<<SUGGESTED as implicitly closing a preceding ORIGINAL block.
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

		// Collect ORIGINAL block — ends at >>>, <<<END, or <<<SUGGESTED
		i++
		var origLines []string
		hitSuggested := false
		for i < len(lines) {
			t := strings.TrimSpace(lines[i])
			if isBlockCloser(t) {
				i++
				break
			}
			if t == "<<<SUGGESTED" {
				// <<<SUGGESTED implicitly closes the ORIGINAL block
				hitSuggested = true
				break
			}
			origLines = append(origLines, lines[i])
			i++
		}

		// Find <<<SUGGESTED if we haven't already hit it
		if !hitSuggested {
			for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
				i++
			}
			if i >= len(lines) || strings.TrimSpace(lines[i]) != "<<<SUGGESTED" {
				continue
			}
		}

		// Collect SUGGESTED block — ends at >>>, <<<END, or <<<ORIGINAL (next pair)
		i++
		var sugLines []string
		for i < len(lines) {
			t := strings.TrimSpace(lines[i])
			if isBlockCloser(t) {
				i++
				break
			}
			if t == "<<<ORIGINAL" {
				// Next pair starts — implicitly closes this SUGGESTED block
				break
			}
			sugLines = append(sugLines, lines[i])
			i++
		}

		original := stripCodeFences(strings.Join(origLines, "\n"))
		suggested := stripCodeFences(strings.Join(sugLines, "\n"))

		// Trim trailing blank lines from both blocks
		original = strings.TrimRight(original, "\n ")
		suggested = strings.TrimRight(suggested, "\n ")

		// Strip adversary analysis debris from suggested text
		suggested = cleanSuggested(suggested)

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
