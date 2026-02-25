# Adversarial Planning System

## Overview

Single-pass review is insufficient. The adversarial planning system uses multiple specialized adversaries to attack plans, then auto-remediates findings by applying structured suggestions directly to `plan.md`.

## Design Philosophy

- **Assume incompetence**: Sub-agents will misinterpret anything ambiguous
- **Assume shortcuts**: If something can be skipped, it will be
- **Assume isolation**: Sub-agents have NO context beyond the plan
- **Be harsh**: A plan that passes should be impossible to screw up
- **Fix, don't just report**: Adversaries write concrete fixes, not just complaints

## The Adversary Committee

Five specialized adversaries, each attacking a different aspect. Priority determines which adversary's suggestions win when two target overlapping text.

**Verdict Format:** All verdicts must be output in **lowercase** (e.g., `coverage_sufficient`, not `COVERAGE_SUFFICIENT`).

### 1. Executability Adversary (Priority 1 — highest)

**Focus**: Can a sub-agent actually do this?

**Attacks**:
- Does the sub-agent have access to everything it needs?
- Are there implicit dependencies on external systems?
- Can tests be run in isolation?
- Are there circular dependencies?
- Does this require knowledge from other phases?
- Are all referenced files/modules accessible?

**Verdict**: `executable` | `blocked`

### 2. Consistency Adversary (Priority 2)

**Focus**: Internal and cross-phase consistency

**Attacks**:
- Do types match across phases?
- Are error handling strategies consistent?
- Do integration points align?
- Are there contradictory requirements?
- Does phase N's output match phase N+1's expected input?
- Are naming conventions consistent?

**Verdict**: `consistent` | `inconsistent`

### 3. Coverage Adversary (Priority 3)

**Focus**: Test coverage and edge cases

**Attacks**:
- Is every function tested?
- Is every error variant tested?
- Are boundary conditions covered?
- What happens with empty/null/negative inputs?
- Are all code paths exercised?

**Verdict**: `coverage_sufficient` | `coverage_gaps`

### 4. Ambiguity Adversary (Priority 4)

**Focus**: Specification clarity

**Attacks**:
- Could a sub-agent misinterpret any requirement?
- Are all types fully specified (no pseudocode)?
- Are file paths explicit (not "somewhere in the package")?
- Does "should" mean "must" or "ideally"?
- What happens on error - panic, return Err, log and continue?
- Are defaults specified for optional fields?

**Verdict**: `unambiguous` | `ambiguous`

### 5. Scope Adversary (Priority 5 — lowest)

**Focus**: Phase size and complexity

**Attacks**:
- Can this be done in one session?
- Are there too many files to modify (>5)?
- Are there too many functions to implement (>12)?
- Should this be split into sub-phases?

**Metrics that trigger warnings**:

| Metric | Warning | Critical |
|--------|---------|----------|
| Files to create | >3 | >5 |
| Files to modify | >5 | >8 |
| Total files | >7 | >10 |
| Functions | >12 | >18 |
| Types (structs+enums) | >10 | >15 |
| Test cases | >40 | >60 |
| Packages affected | >2 | >3 |

**Verdict**: `scope_appropriate` | `scope_too_large`

## Priority Order Rationale

When two adversaries suggest conflicting changes to the same section of `plan.md`, the higher-priority adversary wins:

```
executability > consistency > coverage > ambiguity > scope
```

- If the plan can't execute, nothing else matters
- If it's inconsistent, coverage analysis is unreliable
- Missing coverage is worse than unclear wording
- Scope is optional and least critical

## The Review Loop

```
arc review <plan-name>
     │
     ▼
┌─────────────────────────────────────────────────────────┐
│  Iteration 1:                                           │
│    Run all 5 adversaries in parallel                    │
│    ┌─ coverage: FAIL (2 suggestions)                    │
│    ├─ ambiguity: FAIL (1 suggestion)                    │
│    ├─ scope: PASS                                       │
│    ├─ consistency: PASS                                 │
│    └─ executability: FAIL (1 suggestion)                │
│                                                         │
│    Parse suggestions from failures                      │
│    Filter by confidence (drop < 80)                     │
│    Merge by priority (executability > coverage > ...)   │
│    Drop conflicts (lower priority loses)                │
│    Apply remaining suggestions to plan.md               │
│                                                         │
│  Iteration 2:                                           │
│    plan.md changed → cache invalidated → re-run all     │
│    ┌─ coverage: PASS                                    │
│    ├─ ambiguity: PASS                                   │
│    ├─ scope: PASS                                       │
│    ├─ consistency: PASS                                 │
│    └─ executability: PASS                               │
│                                                         │
│    All passed → approved                                │
│                                                         │
│  Iteration 3:                                           │
│    plan.md unchanged → all cached → loop exits          │
└─────────────────────────────────────────────────────────┘
     │
     ▼
  plan.json updated: review_status = "approved"
```

### Exit Conditions

The loop stops when any of these are true:

1. **All adversaries pass** (status = `approved` or `conditional`)
2. **No suggestions applied** — adversaries failed but didn't provide actionable suggestions, so no progress can be made
3. **Iteration limit hit** (default: 5 per phase)

### Caching

Each adversary result is cached by the SHA256 hash of `plan.md`. When suggestions modify `plan.md`, the hash changes and all cached results are invalidated. When no suggestions are applied, the hash stays the same and cached results terminate the loop naturally.

## Suggestion Format

When an adversary fails, it emits structured find-and-replace blocks:

```markdown
## Suggestions

<<<ORIGINAL
exact text from plan.md to find
<<<SUGGESTED
replacement text with the issue fixed
>>>
```

### Confidence Annotations

Adversaries can annotate suggestions with a confidence score (0-100):

```markdown
<<<ORIGINAL (confidence: 85)
exact text from plan.md to find
<<<SUGGESTED
replacement text with the issue fixed
>>>
```

Suggestions below the confidence threshold (default: 80) are filtered out before merging. This reduces oscillation from speculative changes. If no confidence is specified, it defaults to 100 (always applied).

Rules enforced by the adversary prompts:
- ORIGINAL must be an exact substring of `plan.md` (character-for-character)
- Suggestions are minimal — only change what's needed
- Multiple suggestion blocks are allowed per adversary
- Passing adversaries omit the Suggestions section
- Both `>>>` and `<<<END` are accepted as block closers

### Conflict Resolution

When two suggestions target overlapping text (one contains the other, or they're identical):

1. Sort all suggestions by adversary priority (ascending = higher priority first)
2. Accept each suggestion in order
3. If a suggestion's ORIGINAL text overlaps with an already-accepted suggestion, drop it

This is deterministic — same inputs always produce same merge result.

## Implementation

### Key Types

```go
// internal/review/suggestion.go

type Suggestion struct {
    Adversary  string
    Priority   int
    Original   string
    Suggested  string
    Confidence int    // 0-100, default 100
}

const DefaultConfidenceThreshold = 80

// ParseSuggestions extracts <<<ORIGINAL/<<<SUGGESTED blocks from output.
// Parses optional (confidence: N) annotations on <<<ORIGINAL lines.
func ParseSuggestions(adversaryName string, output string) []Suggestion

// FilterByConfidence removes suggestions below the threshold.
func FilterByConfidence(suggestions []Suggestion, threshold int) []Suggestion

// MergeSuggestions sorts by priority and drops conflicts
func MergeSuggestions(suggestions []Suggestion) []Suggestion

// ApplySuggestions does mechanical find-and-replace on plan content
func ApplySuggestions(planMD string, suggestions []Suggestion) (string, int)
```

### Review Result

```go
// internal/review/review.go

type ReviewResult struct {
    Status             string                       // "approved", "needs_review", "conditional"
    Verdicts           map[string]AdversaryResult
    Iteration          int
    SuggestionsApplied int
    IterationDetails   []IterationDetail
}

type IterationDetail struct {
    Iteration          int
    Status             string
    SuggestionsFound   int
    SuggestionsApplied int
    Verdicts           map[string]string
}
```

### Adversary Definitions

Adversaries are defined in Go code (`internal/review/adversary.go`):

```go
func DefaultAdversaries() []Adversary {
    return []Adversary{
        {Name: "coverage",      PromptPath: "adversaries/coverage.md",      PassVerdict: "coverage_sufficient", FailVerdict: "coverage_gaps",    Required: true},
        {Name: "ambiguity",     PromptPath: "adversaries/ambiguity.md",     PassVerdict: "unambiguous",         FailVerdict: "ambiguous",        Required: true},
        {Name: "scope",         PromptPath: "adversaries/scope.md",         PassVerdict: "scope_appropriate",   FailVerdict: "scope_too_large",  Required: false},
        {Name: "consistency",   PromptPath: "adversaries/consistency.md",   PassVerdict: "consistent",          FailVerdict: "inconsistent",     Required: true},
        {Name: "executability", PromptPath: "adversaries/executability.md", PassVerdict: "executable",          FailVerdict: "blocked",          Required: true},
    }
}
```

Each adversary agent is spawned with:
- **Allowed tools**: `Read` only (read-only analysis)
- **Timeout**: 60 seconds
- **Model**: configurable via `--model` flag (default: `claude-sonnet-4-5-20250929`)

## Review Artifacts

```
.plans/active/<plan>/
├── phases/
│   └── <phase>/
│       └── plan.md                          # Modified by auto-remediation
└── reviews/
    ├── adversary_history.json               # Cache: hash → verdict per adversary
    ├── <phase>_coverage.md                  # Full adversary output
    ├── <phase>_ambiguity.md
    ├── <phase>_scope.md
    ├── <phase>_consistency.md
    └── <phase>_executability.md
```

### History File Format

```json
{
  "phases": {
    "phase-name": {
      "coverage": {
        "hash": "sha256-of-plan.md",
        "verdict": "coverage_sufficient",
        "status": "passed",
        "timestamp": "2026-02-23T00:00:00Z"
      }
    }
  },
  "iterations": {
    "phase-name": 2
  }
}
```

### Plan Metadata

After review completes, `plan.json` is updated:

```json
{
  "review_status": "approved",
  "reviewed_at": "2026-02-23T00:00:00Z",
  "review_iterations": 2,
  "review_results": {
    "coverage": "passed",
    "ambiguity": "passed",
    "scope": "passed",
    "consistency": "passed",
    "executability": "passed"
  }
}
```

## Status Determination

| Condition | Status |
|-----------|--------|
| All adversaries passed | `approved` |
| All **required** passed, optional failed | `conditional` |
| Any **required** failed | `needs_review` |

Only `approved` and `conditional` plans can proceed to `arc run`.

## CLI Usage

```bash
arc review <plan-name>                      # Review all phases (auto-remediates)
arc review <plan-name> --phase <phase>      # Review a single phase
arc review <plan-name> --model <model>      # Use a specific model
arc manage reset-review <plan> <phase>      # Clear cache and iteration counter
```

## Integration with Planning Process

1. **Plan Agent** generates initial plan with phase `plan.md` files
2. **`arc review`** runs adversary committee with auto-remediation loop
3. Loop until all required adversaries pass or max iterations
4. If max iterations reached, remaining findings are surfaced to the user
5. Once approved, `arc run` can begin execution
