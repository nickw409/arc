# Adversarial Planning System

## Overview

Single-pass review is insufficient. The adversarial planning system uses multiple specialized adversaries to attack plans repeatedly until no flaws remain.

## Design Philosophy

- **Assume incompetence**: Sub-agents will misinterpret anything ambiguous
- **Assume shortcuts**: If something can be skipped, it will be
- **Assume isolation**: Sub-agents have NO context beyond the plan
- **Be harsh**: A plan that passes should be impossible to screw up

## The Adversary Committee

Five specialized adversaries, each attacking a different aspect.

**Verdict Format:** All verdicts must be output in **lowercase** (e.g., `coverage_sufficient`, not `COVERAGE_SUFFICIENT`). The system normalizes verdicts during extraction, but consistent formatting prevents errors.

### 1. Coverage Adversary

**Focus**: Test coverage and edge cases

**Attacks**:
- Is every function tested?
- Is every error variant tested?
- Are boundary conditions covered?
- What happens with empty/null/negative inputs?
- Are all code paths exercised?

**Verdict**: `coverage_sufficient` | `coverage_gaps`

### 2. Ambiguity Adversary

**Focus**: Specification clarity

**Attacks**:
- Could a sub-agent misinterpret any requirement?
- Are all types fully specified (no pseudocode)?
- Are file paths explicit (not "somewhere in the package")?
- Does "should" mean "must" or "ideally"?
- What happens on error - panic, return Err, log and continue?
- Are defaults specified for optional fields?

**Verdict**: `unambiguous` | `ambiguous`

### 3. Scope Adversary

**Focus**: Phase size and complexity

**Attacks**:
- Can this be done in one session?
- Are there too many files to modify (>5)?
- Are there too many functions to implement (>12)?
- Should this be split into sub-phases?
- Are dependencies between changes clear?
- Is the cognitive load manageable?

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

### 4. Consistency Adversary

**Focus**: Internal and cross-phase consistency

**Attacks**:
- Do types match across phases?
- Are error handling strategies consistent?
- Do integration points align?
- Are there contradictory requirements?
- Does phase N's output match phase N+1's expected input?
- Are naming conventions consistent?

**Verdict**: `consistent` | `inconsistent`

### 5. Executability Adversary

**Focus**: Can a sub-agent actually do this?

**Attacks**:
- Does the sub-agent have access to everything it needs?
- Are there implicit dependencies on external systems?
- Can tests be run in isolation?
- Are there circular dependencies?
- Does this require knowledge from other phases?
- Are all referenced files/modules accessible?

**Verdict**: `executable` | `blocked`

## Planning Loop

```
+---------------------------------------------------------------------+
|                      Planning Review Loop                            |
|                                                                      |
|  Iteration 1:                                                        |
|    Plan Agent --> Initial plan                                       |
|    Adversaries --> [coverage: gaps, ambiguity: ok, scope: ok,       |
|                     consistency: ok, executability: blocked]         |
|    Result: 2 adversaries failed                                      |
|                                                                      |
|  Iteration 2:                                                        |
|    Plan Agent --> Addresses coverage + executability                 |
|    Adversaries --> [coverage: ok, ambiguity: gaps, scope: ok,       |
|                     consistency: ok, executability: ok]              |
|    Result: 1 adversary failed                                        |
|                                                                      |
|  Iteration 3:                                                        |
|    Plan Agent --> Addresses ambiguity                                |
|    Adversaries --> [all ok]                                          |
|    Result: approved                                                  |
|                                                                      |
+---------------------------------------------------------------------+
```

## Implementation

### Adversary Definition File

```yaml
# $ARC_HOME/adversaries/planning-adversaries.yaml

max_iterations: 5  # Give up after this many rounds

adversaries:
  - name: coverage
    prompt: prompts/adversaries/coverage.md
    required: true  # Plan cannot proceed if this fails
    verdicts:
      pass: coverage_sufficient
      fail: coverage_gaps

  - name: ambiguity
    prompt: prompts/adversaries/ambiguity.md
    required: true
    verdicts:
      pass: unambiguous
      fail: ambiguous

  - name: scope
    prompt: prompts/adversaries/scope.md
    required: false  # Warning only, can proceed
    verdicts:
      pass: scope_appropriate
      fail: scope_too_large

  - name: consistency
    prompt: prompts/adversaries/consistency.md
    required: true
    verdicts:
      pass: consistent
      fail: inconsistent

  - name: executability
    prompt: prompts/adversaries/executability.md
    required: true
    verdicts:
      pass: executable
      fail: blocked
```

### Helper Functions

```bash
# Run an adversary agent and return its verdict
# Usage: run_adversary_agent <prompt_file> <plan_dir>
run_adversary_agent() {
    local prompt_file="$1"
    local plan_dir="$2"
    local output_file
    output_file=$(mktemp)

    # Build the prompt with plan context
    local full_prompt
    full_prompt=$(cat "$prompt_file")
    full_prompt+=$'\n\n## Plan Under Review\n\n'
    full_prompt+=$(cat "$plan_dir/phases/"*/plan.md 2>/dev/null || echo "No phases found")

    # Run Claude Code CLI with adversary prompt
    if ! claude --print --output-format text -p "$full_prompt" > "$output_file" 2>&1; then
        echo "error: agent failed"
        rm -f "$output_file"
        return 1
    fi

    # Return the output (verdict extraction happens in caller)
    cat "$output_file"
    rm -f "$output_file"
}

# Update plan status in the plan's metadata
update_plan_status() {
    local plan_dir="$1"
    local status="$2"
    local status_file="$plan_dir/status.json"

    jq --arg status "$status" \
       --arg updated "$(date -Iseconds)" \
       '. + {status: $status, updated_at: $updated}' \
       "$status_file" > "$status_file.tmp" && mv "$status_file.tmp" "$status_file"
}

# Collect failures from all adversary reviews for a given iteration
collect_failures() {
    local plan_dir="$1"
    local iteration="$2"

    echo "# Adversary Failures - Iteration $iteration"
    echo ""

    for review in "$plan_dir/reviews/iteration_${iteration}_"*.md; do
        [[ -f "$review" ]] || continue
        local adversary
        adversary=$(basename "$review" | sed "s/iteration_${iteration}_//" | sed 's/\.md$//')

        # Check if review contains a failure verdict
        if ! grep -q "^## Verdict" "$review" || \
           grep -A1 "^## Verdict" "$review" | grep -qE "(gaps|ambiguous|too_large|inconsistent|blocked)"; then
            echo "## $adversary"
            echo ""
            cat "$review"
            echo ""
        fi
    done
}

# Run plan agent to fix identified issues
run_plan_agent_fix() {
    local plan_dir="$1"
    local failures_file="$2"

    local fix_prompt="You are refining a plan based on adversary feedback.

## Current Plan
$(cat "$plan_dir/phases/"*/plan.md 2>/dev/null)

## Issues to Address
$(cat "$failures_file")

## Instructions
1. Read each adversary's concerns carefully
2. Modify the plan to address ALL issues
3. Output the complete updated plan.md content

Do NOT explain changes - just output the fixed plan."

    claude --print --output-format text -p "$fix_prompt"
}
```

### Review Loop Script

```bash
#!/usr/bin/env bash
# arc plan-review-loop

set -euo pipefail

PLAN_DIR="$1"
MAX_ITERATIONS=$(yq '.max_iterations' "$ADVERSARIES_FILE")

for ((iteration=1; iteration<=MAX_ITERATIONS; iteration++)); do
    echo "=== Planning Review Iteration $iteration ==="

    all_passed=true
    required_failed=false

    # Run each adversary
    for adversary in $(yq '.adversaries[].name' "$ADVERSARIES_FILE"); do
        echo "Running $adversary adversary..."

        prompt=$(yq ".adversaries[] | select(.name == \"$adversary\") | .prompt" "$ADVERSARIES_FILE")
        required=$(yq ".adversaries[] | select(.name == \"$adversary\") | .required" "$ADVERSARIES_FILE")
        pass_verdict=$(yq ".adversaries[] | select(.name == \"$adversary\") | .verdicts.pass" "$ADVERSARIES_FILE")

        # Run adversary agent
        verdict=$(run_adversary_agent "$prompt" "$PLAN_DIR")

        # Save review
        echo "$verdict" > "$PLAN_DIR/reviews/iteration_${iteration}_${adversary}.md"

        if [[ "$verdict" != *"$pass_verdict"* ]]; then
            all_passed=false
            echo "  FAIL $adversary: FAILED"

            if [[ "$required" == "true" ]]; then
                required_failed=true
            fi
        else
            echo "  PASS $adversary: passed"
        fi
    done

    # Check results
    if $all_passed; then
        echo ""
        echo "=== ALL ADVERSARIES SATISFIED ==="
        echo "Plan approved after $iteration iteration(s)"
        update_plan_status "$PLAN_DIR" "approved"
        exit 0
    fi

    if [[ $iteration -eq $MAX_ITERATIONS ]]; then
        echo ""
        echo "=== MAX ITERATIONS REACHED ==="
        echo "Plan requires human review"
        update_plan_status "$PLAN_DIR" "needs_human_review"
        exit 1
    fi

    # Collect all failures for plan agent to address
    echo ""
    echo "Collecting failures for plan agent..."
    collect_failures "$PLAN_DIR" "$iteration" > "$PLAN_DIR/reviews/round_${iteration}_failures.md"

    # Run plan agent to fix issues
    echo "Plan agent addressing failures..."
    run_plan_agent_fix "$PLAN_DIR" "$PLAN_DIR/reviews/round_${iteration}_failures.md"
done
```

## Adversary Prompts

### Coverage Adversary Prompt

```markdown
# Coverage Adversary

You are an adversarial reviewer focused on test coverage. Your job is to find gaps.

## Your Mindset
- Every untested function WILL have bugs
- Every untested edge case WILL cause production failures
- If it's not tested, it doesn't work

## Attack Checklist

### Function Coverage
For EVERY function in the plan's specification:
- [ ] Is there at least one test?
- [ ] Are error cases tested?
- [ ] Are boundary conditions tested?

### Type Coverage
For EVERY struct/enum:
- [ ] Is construction tested?
- [ ] Are all variants used in tests?
- [ ] Is serialization/deserialization tested (if applicable)?

### Edge Cases
For EVERY function:
- [ ] Empty input (vec![], "", None)
- [ ] Zero values
- [ ] Negative values (if numeric)
- [ ] Maximum values (u32::MAX, etc.)
- [ ] Invalid state combinations

### Error Handling
For EVERY Result-returning function:
- [ ] Is every error variant tested?
- [ ] Is error propagation tested?

## Output Format

```markdown
## Coverage Analysis

### Functions Without Tests
- [ ] `function_name` - no test found
- [ ] `another_function` - only happy path tested

### Missing Edge Case Coverage
- [ ] `function_name` - no test for empty input
- [ ] `function_name` - no test for negative values

### Untested Error Variants
- [ ] `ErrorType::Variant` - never triggered in tests

## Verdict
coverage_sufficient - All functions and edge cases covered
OR
coverage_gaps - X functions untested, Y edge cases missing
```

**Important:** Output verdicts in lowercase to match workflow configuration.

Be thorough. Missing coverage now means bugs later.
```

### Ambiguity Adversary Prompt

```markdown
# Ambiguity Adversary

You are an adversarial reviewer focused on specification clarity. Your job is to find anything a sub-agent could misinterpret.

## Your Mindset
- Sub-agents are competent but have ZERO context beyond the plan
- Any ambiguity WILL be misinterpreted
- "Obvious" things are not obvious to an isolated agent

## Attack Checklist

### Type Specifications
- [ ] Every field has an explicit type (`field: String`, not just `field`)
- [ ] Generic bounds are specified (`T: Serialize`, not just `T`)
- [ ] Return types are complete (`Result<Vec<u8>, MyError>`, not `Result`)
- [ ] Option/Result wrapping is explicit

### Behavioral Specifications
- [ ] "Should" vs "must" - which is it?
- [ ] Error behavior is explicit (panic? return Err? log and continue?)
- [ ] Default values are specified for optional fields
- [ ] Order of operations is clear when it matters

### File Locations
- [ ] Every file path is absolute from project root
- [ ] Module declarations are explicit (`pub mod X` in which file?)
- [ ] Test file locations are explicit

### Implicit Knowledge
- [ ] No references to "the usual way" without defining it
- [ ] No assumptions about existing code patterns
- [ ] No references to other phases without explicit context

### Terminology
- [ ] Domain terms are defined or unambiguous
- [ ] Variable names match between spec and tests
- [ ] No overloaded terms (same word meaning different things)

## Output Format

```markdown
## Ambiguity Analysis

### Critical (blocks execution)
- [ ] **Line X**: "returns error" - which error type?
- [ ] **Types section**: `Config` struct fields have no types

### Major (likely misinterpretation)
- [ ] **Test case 3**: "should handle edge case" - which edge case?
- [ ] **Line Y**: "appropriate value" - what makes it appropriate?

### Minor (could be clearer)
- [ ] **Line Z**: Consider specifying the exact error message format

## Verdict
unambiguous - Specification is clear and complete
OR
ambiguous - X critical, Y major issues must be resolved
```

**Important:** Output verdicts in lowercase to match workflow configuration.

A plan that passes your review should be impossible to misinterpret.
```

### Scope Adversary Prompt

```markdown
# Scope Adversary

You are an adversarial reviewer focused on phase scope. Your job is to identify phases that are too large to execute reliably.

## Your Mindset
- Large phases fail more often
- Cognitive overload causes mistakes
- If you can't hold it all in your head, it's too big

## Metrics to Evaluate

| Metric | Warning | Critical | This Phase |
|--------|---------|----------|------------|
| Files to create | >3 | >5 | ? |
| Files to modify | >5 | >8 | ? |
| Total files | >7 | >10 | ? |
| Functions | >12 | >18 | ? |
| Types (structs+enums) | >10 | >15 | ? |
| Test cases | >40 | >60 | ? |
| Packages affected | >2 | >3 | ? |

## Attack Checklist

### Cognitive Load
- [ ] Can a sub-agent understand this in one session?
- [ ] Are there too many moving parts?
- [ ] Are dependencies between changes clear?

### Session Viability
- [ ] Can this be completed in 15-25 iterations?
- [ ] Are there natural breakpoints for splitting?
- [ ] Would failure require re-doing significant work?

### Split Candidates
If scope is too large, identify split points:
- By file/module
- By feature
- By layer (types -> implementation -> tests)
- By dependency order

## Output Format

```markdown
## Scope Analysis

### Metrics
| Metric | Value | Status |
|--------|-------|--------|
| Files to create | X | OK/WARNING/CRITICAL |
| ... | ... | ... |

### Concerns
- [ ] Phase affects 4 packages - high coordination overhead
- [ ] 15 functions to implement - cognitive load concern

### Suggested Split (if needed)
1. Phase A: Types and core functions (files X, Y)
2. Phase B: Integration and edge cases (files Z, W)

## Verdict
scope_appropriate - Phase is manageable
OR
scope_too_large - Recommend splitting as described above
```

**Important:** Output verdicts in lowercase to match workflow configuration.

When in doubt, smaller is better.
```

### Consistency Adversary Prompt

```markdown
# Consistency Adversary

You are an adversarial reviewer focused on internal and cross-phase consistency. Your job is to find contradictions and misalignments.

## Your Mindset
- Inconsistent specs cause integration failures
- Type mismatches between phases break builds
- Naming inconsistencies cause confusion and bugs

## Attack Checklist

### Type Consistency
For EVERY type referenced across phases:
- [ ] Does the type name match exactly?
- [ ] Do field names and types match?
- [ ] Are derives/traits consistent?
- [ ] Are Option/Result wrappings consistent?

### Error Handling Consistency
- [ ] Are error types compatible across phases?
- [ ] Is error propagation strategy consistent?
- [ ] Do error messages follow same format?

### Integration Point Alignment
For EVERY integration point:
- [ ] Does Phase N's output match Phase N+1's expected input?
- [ ] Are function signatures compatible?
- [ ] Are serialization formats consistent?

### Naming Conventions
- [ ] Are variable names consistent (snake_case vs camelCase)?
- [ ] Are module names consistent?
- [ ] Are file naming patterns consistent?

### Cross-Phase Dependencies
- [ ] Are imports/use statements correct for dependent phases?
- [ ] Are version constraints consistent?
- [ ] Are feature flags referenced consistently?

## Output Format

```markdown
## Consistency Analysis

### Type Mismatches
- [ ] `TypeA` in Phase 1 has field `foo: String`, Phase 2 expects `foo: &str`
- [ ] `ErrorType` in Phase 1 missing variant used in Phase 2

### Integration Misalignments
- [ ] Phase 1 outputs `Vec<Item>`, Phase 2 expects `&[Item]`
- [ ] Phase 1 returns `Result<T, E1>`, Phase 2 expects `Result<T, E2>`

### Naming Inconsistencies
- [ ] Phase 1 uses `user_id`, Phase 2 uses `userId`
- [ ] Phase 1 module `data`, Phase 2 references `types`

### Contradictory Requirements
- [ ] Phase 1 says "must panic on error", Phase 2 says "return Err"

## Verdict
consistent - No contradictions or misalignments found
OR
inconsistent - X type mismatches, Y naming issues must be resolved
```

**Important:** Output verdicts in lowercase to match workflow configuration.

Assume nothing aligns. Verify everything explicitly.
```

### Executability Adversary Prompt

```markdown
# Executability Adversary

You are an adversarial reviewer focused on whether a sub-agent can actually execute this plan. Your job is to find blockers.

## Your Mindset
- Sub-agents work in isolation
- External dependencies WILL fail
- Implicit knowledge DOES NOT exist
- If a step can block, it will block

## Attack Checklist

### File System Access
For EVERY file referenced:
- [ ] Does the file exist (for reads)?
- [ ] Is the path correct and absolute?
- [ ] Does the sub-agent have write permission to parent directory?
- [ ] Are there any path assumptions (home dir, temp dir)?

### External Dependencies
- [ ] Does this require a running database?
- [ ] Does this require a running server?
- [ ] Does this require network access?
- [ ] Does this require GPU/CUDA?
- [ ] Are all dependencies specified with correct versions?

### Build Requirements
- [ ] Can the code compile with current dependencies?
- [ ] Are there circular dependencies?
- [ ] Are feature flags required?

### Test Requirements
- [ ] Can tests run in isolation?
- [ ] Do tests require setup/teardown?
- [ ] Do tests require specific environment variables?
- [ ] Do tests require test fixtures that don't exist yet?

### Cross-Phase Dependencies
- [ ] Does this phase require output from a previous phase?
- [ ] Is that output guaranteed to exist?
- [ ] Can this phase run if previous phase was skipped?

### Implicit Knowledge
- [ ] Does the plan reference "existing patterns" without defining them?
- [ ] Does the plan reference "similar to X" without specifying X?
- [ ] Does the plan assume knowledge of other parts of the codebase?

## Output Format

```markdown
## Executability Analysis

### Blocking Issues
- [ ] Requires server running on port 50051 - no setup step defined
- [ ] References `config.toml` that doesn't exist in repo
- [ ] Test `test_gpu_kernel` requires CUDA but no GPU check

### Missing Dependencies
- [ ] Uses `serde_yaml` but not in dependency list
- [ ] References `test_utils` module that doesn't exist

### Implicit Assumptions
- [ ] "Follow the existing pattern" - which pattern?
- [ ] "Similar to the other handlers" - which handlers?

### Environment Requirements
- [ ] Requires `DATABASE_URL` environment variable
- [ ] Requires `CUDA_HOME` to be set

## Verdict
executable - All requirements are met or explicitly handled
OR
blocked - Cannot execute due to: [list blockers]
```

**Important:** Output verdicts in lowercase to match workflow configuration.

If you can imagine a way it could fail, it will fail.
```

## Integration with Planning Process

1. **Plan Agent** generates initial plan
2. **arc init** creates structure and validates workflow
3. **arc plan-review-loop** runs adversary committee
4. Loop until all required adversaries pass or max iterations
5. If max iterations reached, escalate to human
6. Once approved, orchestrator can begin execution

## Adversary Output Storage

```
.plans/active/<plan>/
+-- phases/
|   +-- <phase>/
|       +-- plan.md
+-- reviews/
|   +-- iteration_1_coverage.md
|   +-- iteration_1_ambiguity.md
|   +-- iteration_1_scope.md
|   +-- iteration_1_consistency.md
|   +-- iteration_1_executability.md
|   +-- round_1_failures.md
|   +-- iteration_2_coverage.md
|   +-- ...
+-- workflow.yaml
```

## Human Override

If adversaries keep failing but human believes plan is acceptable:

```bash
# Skip adversarial review
SKIP_ADVERSARIES=1 arc init my-plan phase1 phase2

# Or approve despite failures
arc approve-plan my-plan --override "Adversary concerns addressed manually"
```

Override is logged for audit trail.
