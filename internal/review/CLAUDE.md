# Review

Adversarial plan review system. Four AI adversaries examine plan.md for issues, with auto-remediation via synthesizer.

**Start here:** `review.go` for the review loop, `adversary.go` for individual adversary execution.

## File Map

| File | Purpose |
|------|---------|
| `review.go` | `Run()` — main review loop. Scope pre-check, then 3 parallel adversaries (spec-quality, correctness, gate), synthesizer rewrites plan.md if any fail. |
| `adversary.go` | `RunAdversary()` — runs a single adversary agent. SHA-256 caching, progressive leniency at iterations 3+5. `DefaultAdversaries()` returns the canonical four. |
| `synthesize.go` | `RunSynthesizer()` — rewrites plan.md based on adversary feedback. |

## The Four Adversaries

| Name | When | Checks |
|------|------|--------|
| scope | Once at start (pre-check) | Is the phase small enough to execute reliably? |
| spec-quality | Parallel, every iteration | Is the spec concrete, unambiguous, and actionable? |
| correctness | Parallel, every iteration | Are types, names, and contracts consistent and correct? |
| gate | Parallel, every iteration | Do gate assertions cover every integration point? |

## Key Design Decisions

- **Review is advisory, not blocking**: `"needs_review"` is always downgraded to `"conditional"` after the loop. The system never prevents execution.
- **Convergence detection**: builds a sorted string of failing adversary names. If the same signature appears twice, stops (oscillation guard).
- **Progressive leniency**: at later iterations, adversaries are told to raise their failure bar — prevents infinite nit-picking.
- **Caching**: SHA-256 of plan.md content. Same content = same cached result, skipping the agent call.
