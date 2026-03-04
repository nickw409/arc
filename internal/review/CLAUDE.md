# Review

Adversarial plan review system. Five AI adversaries examine plan.md for issues, with auto-remediation via suggestion blocks.

**Start here:** `review.go` for the review loop, `adversary.go` for individual adversary execution.

## File Map

| File | Purpose |
|------|---------|
| `review.go` | `Run()` — main review loop. Runs up to 5 iterations: spawn adversaries, parse suggestions, apply fixes to plan.md, check convergence. |
| `adversary.go` | `RunAdversary()` — runs a single adversary agent. SHA-256 caching, progressive leniency at iterations 3+5. `DefaultAdversaries()` returns the canonical five. |
| `suggestion.go` | Parses `<<<ORIGINAL`/`<<<SUGGESTED` diff blocks, merges by priority, applies string replacements. |

## The Five Adversaries

| Name | Required | Priority | Checks |
|------|----------|----------|--------|
| executability | yes | 0 (highest) | Can the plan actually be executed? |
| consistency | yes | 1 | Are the steps internally consistent? |
| coverage | yes | 2 | Does the plan cover all requirements? |
| ambiguity | yes | 3 | Are instructions unambiguous? |
| scope | no | 4 (lowest) | Is scope appropriate? |

## Key Design Decisions

- **Review is advisory, not blocking**: `"needs_review"` is always downgraded to `"conditional"` after the loop. The system never prevents execution.
- **Convergence detection**: builds a sorted string of failing adversary names. If the same signature appears twice, stops (oscillation guard).
- **Progressive leniency**: at iteration >= 3 and >= 5, adversaries are told to raise their failure bar — prevents infinite nit-picking.
- **Suggestion priority**: executability > consistency > coverage > ambiguity > scope. Conflicts are resolved by greedy elimination (higher priority wins).
- **Confidence threshold**: only suggestions with confidence >= 80 are applied.
- **Debris stripping**: removes LLM analysis headings (`### Fix N:`, editorial comments) that agents inject inside `<<<SUGGESTED` blocks.
- **Caching**: SHA-256 of plan.md content. Same content = same cached result, skipping the agent call.
