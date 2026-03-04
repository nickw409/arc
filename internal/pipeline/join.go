package pipeline

import (
	"fmt"
	"sort"

	"github.com/nwiley/arc/internal/arc"
)

// JoinParallel determines a verdict from parallel branch results based on strategy.
//
// Strategies:
//   - "all": all must exit 0 → "all_complete", else "any_failed"
//   - "any": any exit 0 → "first_complete", else "all_failed"
//   - "n_of_m": count(exit 0) >= n → "n_complete", else "insufficient"
func JoinParallel(strategy string, results map[string]int, n int) (string, error) {
	if len(results) == 0 {
		return "", fmt.Errorf("no results to join")
	}

	successes := 0
	for _, code := range results {
		if code == 0 {
			successes++
		}
	}

	switch strategy {
	case "all":
		if successes == len(results) {
			return "all_complete", nil
		}
		return "any_failed", nil
	case "any":
		if successes > 0 {
			return "first_complete", nil
		}
		return "all_failed", nil
	case "n_of_m":
		if n <= 0 {
			return "", fmt.Errorf("n_of_m strategy requires n > 0")
		}
		if successes >= n {
			return "n_complete", nil
		}
		return "insufficient", nil
	default:
		return "", fmt.Errorf("unknown strategy %q", strategy)
	}
}

// MergeVerdicts combines per-branch verdicts into a single merged verdict
// using the specified strategy. validVerdicts is the set of acceptable
// verdicts. All branch verdicts must be in this set.
//
// An optional positiveVerdict can be provided to specify which verdict
// represents the "clean" outcome. When provided:
//   - "all" strategy: positive verdict wins only if ALL branches agree;
//     otherwise the non-positive (negative) verdict wins.
//   - "any" strategy: positive verdict wins if ANY branch returned it.
//
// When positiveVerdict is not provided, falls back to alphabetical ordering
// (first alphabetically wins for "all", last for "any").
func MergeVerdicts(strategy string, branchVerdicts map[string]arc.Verdict, validVerdicts []arc.Verdict, positiveVerdict ...arc.Verdict) (arc.Verdict, error) {
	if len(branchVerdicts) == 0 {
		return "", fmt.Errorf("no branch verdicts to merge")
	}

	// Validate all branch verdicts are in validVerdicts set.
	validSet := make(map[arc.Verdict]bool, len(validVerdicts))
	for _, v := range validVerdicts {
		validSet[v] = true
	}
	for branch, v := range branchVerdicts {
		if !validSet[v] {
			return "", fmt.Errorf("branch %q returned invalid verdict %q", branch, v)
		}
	}

	// Validate strategy before processing.
	switch strategy {
	case "all", "any":
		// supported
	default:
		return "", fmt.Errorf("unsupported merge strategy %q", strategy)
	}

	// Count occurrences of each verdict.
	counts := make(map[arc.Verdict]int)
	for _, v := range branchVerdicts {
		counts[v]++
	}

	// If all branches agree, return that verdict regardless of strategy.
	if len(counts) == 1 {
		for v := range counts {
			return v, nil
		}
	}

	// Determine positive verdict (if provided).
	var posVerdict arc.Verdict
	hasPositive := len(positiveVerdict) > 0 && positiveVerdict[0] != ""
	if hasPositive {
		posVerdict = positiveVerdict[0]
	}

	switch strategy {
	case "all":
		// "all" strategy: positive wins only if ALL agree. If mixed,
		// the negative verdict wins (any dissent means failure).
		if hasPositive {
			// Return the first non-positive verdict found.
			for _, v := range validVerdicts {
				if v != posVerdict && counts[arc.Verdict(v)] > 0 {
					return arc.Verdict(v), nil
				}
			}
		}
		// Fallback: alphabetical (first present wins).
		var present []arc.Verdict
		for v := range counts {
			present = append(present, v)
		}
		sort.Slice(present, func(i, j int) bool {
			return string(present[i]) < string(present[j])
		})
		return present[0], nil

	case "any":
		// "any" strategy: positive wins if ANY branch returned it.
		if hasPositive && counts[posVerdict] > 0 {
			return posVerdict, nil
		}
		if hasPositive {
			// No branch returned positive; return the first non-positive.
			for _, v := range validVerdicts {
				if v != posVerdict && counts[arc.Verdict(v)] > 0 {
					return arc.Verdict(v), nil
				}
			}
		}
		// Fallback: alphabetical (last present wins).
		var present []arc.Verdict
		for v := range counts {
			present = append(present, v)
		}
		sort.Slice(present, func(i, j int) bool {
			return string(present[i]) < string(present[j])
		})
		return present[len(present)-1], nil
	}

	// Unreachable due to upfront strategy validation.
	return "", fmt.Errorf("unsupported merge strategy %q", strategy)
}
