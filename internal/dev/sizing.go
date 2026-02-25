package dev

// ValidateComplexity checks that the agent's complexity assessment is
// reasonable given the discovery results. It may override the agent's
// assessment if the heuristics disagree strongly.
//
// Override rules are applied sequentially:
//   - If complexity is "simple" but SuggestedPhases has >1 entry, upgrade to "medium"
//   - If complexity is "simple" but RelevantFiles has >5 entries, upgrade to "medium"
//   - If complexity is "medium" but SuggestedPhases has >4 entries, upgrade to "complex"
//   - If complexity is "complex" but RelevantFiles has <=2 entries and
//     SuggestedPhases has <=1 entry, downgrade to "simple"
func ValidateComplexity(result *DiscoveryResult) TaskComplexity {
	complexity := result.Complexity

	if complexity == ComplexitySimple && len(result.SuggestedPhases) > 1 {
		complexity = ComplexityMedium
	}

	if complexity == ComplexitySimple && len(result.RelevantFiles) > 5 {
		complexity = ComplexityMedium
	}

	if complexity == ComplexityMedium && len(result.SuggestedPhases) > 4 {
		complexity = ComplexityComplex
	}

	if complexity == ComplexityComplex && len(result.RelevantFiles) <= 2 && len(result.SuggestedPhases) <= 1 {
		complexity = ComplexitySimple
	}

	return complexity
}

// ValidateWorkflowType checks that the suggested workflow type is valid.
// Returns the workflow type, or "feature" as default if the type is unrecognized.
func ValidateWorkflowType(workflowType string) string {
	switch workflowType {
	case "feature", "bugfix", "investigation", "refactor", "performance", "direct", "audit":
		return workflowType
	default:
		return "feature"
	}
}
