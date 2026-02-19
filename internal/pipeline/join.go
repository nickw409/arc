package pipeline

import "fmt"

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
