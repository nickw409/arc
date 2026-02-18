# Prompt Template System

## Overview

Prompts are Markdown templates with variable substitution and conditional blocks. They define what sub-agents see and how they behave.

## Template Syntax

### Variable Substitution

```markdown
# Simple variables
Plan: {{plan}}
Phase: {{phase}}
Iteration: {{iteration}}/{{max_iterations}}

# Nested access
Packages: {{state.packages}}
Objective: {{plan.objective}}

# With defaults
Model: {{model | default: "sonnet"}}
```

### Conditional Blocks

```markdown
{{#if escalation_active}}
## ESCALATION ACTIVE

You have been stuck for {{stuck_iterations}} iterations.
{{/if}}

{{#unless has_tests}}
No tests exist yet. You need to write them.
{{/unless}}

{{#if verdict}}
Previous verdict: {{verdict}}
{{else}}
This is the first review.
{{/if}}
```

### Iteration Blocks

```markdown
{{#each previous_attempts}}
### Iteration {{this.iteration}}
- Approach: {{this.approach}}
- Result: {{this.result}}
{{/each}}

{{#each failing_tests}}
- {{this.name}}: {{this.error}}
{{/each}}
```

### Includes

```markdown
{{> common/test-commands}}

{{> common/do-not-rules}}

{{> common/reasoning-format}}
```

## Variable Sources

### Built-in Variables

| Variable | Type | Source | Description |
|----------|------|--------|-------------|
| `plan` | string | state.json | Plan name |
| `phase` | string | state.json | Phase name |
| `iteration` | number | state.json | Current iteration |
| `max_iterations` | number | workflow.yaml | Max iterations for state |
| `state_file` | string | computed | Path to state.json |
| `phase_dir` | string | computed | Path to phase directory |
| `plan_dir` | string | computed | Path to plan directory |
| `workflow_file` | string | computed | Path to workflow.yaml |
| `current_state` | string | state.json | Current workflow state |

### From Plan Frontmatter

```markdown
---
objective: Implement PCG RNG in WASM
packages:
  - my-wasm-package
---
```

| Variable | Type | Description |
|----------|------|-------------|
| `objective` | string | Phase objective |
| `packages` | array | Affected packages |

### From State

```json
{
  "iteration": 5,
  "stuck_iterations": 2,
  "tests_passing": 8,
  "tests_total": 12,
  "last_verdict": "some_failing"
}
```

| Variable | Type | Description |
|----------|------|-------------|
| `state.iteration` | number | Current iteration |
| `state.stuck_iterations` | number | Consecutive stuck iterations |
| `state.tests_passing` | number | Passing test count |
| `state.tests_total` | number | Total test count |
| `state.last_verdict` | string | Last review verdict |
| `state.previous_attempts` | array | History of approaches tried (from impl_reasoning.md parsing) |

**Note:** `previous_attempts` is populated by parsing `impl_reasoning.md` from previous iterations. Each entry contains:
```json
{
  "iteration": 3,
  "approach": "Changed RNG seed initialization",
  "result": "2 tests still failing",
  "hypothesis": "Seed not being reset between runs"
}
```

### From Workflow

```yaml
states:
  - name: fix
    params:
      allow_test_changes: false
      max_file_changes: 5
```

| Variable | Type | Description |
|----------|------|-------------|
| `params.*` | any | State-specific parameters |
| `max_iterations` | number | State max iterations |
| `timeout` | number | State timeout |

### Context Files

| Variable | Source File | Description |
|----------|-------------|-------------|
| `orchestrator_notes` | orchestrator_notes.md | Notes from orchestrator |
| `impl_reasoning` | impl_reasoning.md | Previous implementation reasoning |
| `qa_reasoning` | qa_reasoning.md | Test writing reasoning |
| `last_test_output` | last_test_output.txt | Recent test output |
| `findings` | findings.md | Investigation findings |

### Computed Variables

| Variable | Type | Description |
|----------|------|-------------|
| `escalation_active` | boolean | stuck_iterations >= 3 |
| `tests_all_passing` | boolean | tests_passing == tests_total |
| `has_disputes` | boolean | Active disputes exist |
| `cleared_disputes` | object | Recently cleared disputes |
| `remaining_iterations` | number | max_iterations - iteration |
| `previous_attempts` | array | Parsed from impl_reasoning.md history (see From State section) |

**Computed Variable Formulas:**
```bash
# In render_prompt()
remaining_iterations=$((max_iterations - iteration))
escalation_active=$([[ $stuck_iterations -ge 3 ]] && echo "true" || echo "false")
tests_all_passing=$([[ $tests_passing -eq $tests_total ]] && echo "true" || echo "false")
```

## Common Includes

### common/test-commands.md

```markdown
## Test Commands

Run the configured test command for your package with the phase-specific pattern:

```bash
$ARC_SCRIPTS_DIR/run-phase-tests.sh {{phase}}
```
```

### common/do-not-rules.md

```markdown
## DO NOT

- **Do NOT commit** -- orchestrator handles all commits
- **Do NOT modify test files** -- dispute if tests are wrong
- **Do NOT run tests after each change** -- batch fixes, run once
- **Do NOT blame "random variance"** -- verify with multiple runs
```

### common/reasoning-format.md

```markdown
## Required: Reasoning Document

Before exiting, you MUST write to `{{phase_dir}}/impl_reasoning.md`:

```markdown
# Impl Reasoning: {{phase}} (Iteration {{iteration}})

## Tests Status
- Passing: X/Y
- Failing: [list]

## Changes Made This Iteration

### Change 1: [file:line]
- **Hypothesis**: What I thought was wrong
- **Evidence**: Specific observation supporting this
- **Alternatives**: What else could explain this? Why ruled out?
- **Prediction**: What should happen after this fix

## If Tests Still Failing

For each remaining failure:
- **Test**: [name]
- **Error**: [exact error]
- **Consistent?**: Yes/No (ran N times)
- **Next hypothesis**: What to try next
```
```

## Full Prompt Examples

### Implementation Prompt

```markdown
# Implementation: {{phase}}

Plan: {{plan}}
Phase: {{phase}}
Iteration: {{iteration}}/{{max_iterations}}
Packages: {{#each packages}}{{this}}{{#unless @last}}, {{/unless}}{{/each}}

{{#if orchestrator_notes}}
## Notes from Orchestrator

{{orchestrator_notes}}

---
{{/if}}

{{#if escalation_active}}
## ESCALATION ACTIVE (Stuck {{stuck_iterations}} iterations)

Previous approaches have not worked. You MUST try something different.

### Recent Failures
```
{{recent_test_failures}}
```

### What's Been Tried
{{#each previous_attempts}}
- Iteration {{this.iteration}}: {{this.approach}} -> {{this.result}}
{{/each}}

### Required Actions
1. Do NOT repeat previous approaches
2. Read impl_reasoning.md to understand what was tried
3. Try a fundamentally different approach
4. If a test is genuinely wrong, file a dispute with evidence

---
{{/if}}

{{#if cleared_disputes}}
## Recent Dispute Resolution

{{cleared_disputes.count}} dispute(s) were resolved:
{{#each cleared_disputes.tests}}
- **{{this.name}}**: {{this.resolution}}
{{/each}}

These tests should now be passable.

---
{{/if}}

## Your Task

Make all tests pass. You have {{remaining_iterations}} iterations remaining.

### Step 1: Run ALL tests
```bash
$ARC_SCRIPTS_DIR/run-phase-tests.sh {{phase}}
```

### Step 2: Analyze ALL failures at once
Read the full output. List every failing test and why.

### Step 3: Fix ALL tests
- Identify root causes (one fix often resolves multiple tests)
- Make all edits without re-running tests between fixes
- Group related changes

### Step 4: Verify
Run tests once more to confirm.

{{> common/do-not-rules}}

{{> common/reasoning-format}}

When done, exit.
```

### Review Prompt

```markdown
# Review: {{phase}} ({{review_type}})

Plan: {{plan}}
Phase: {{phase}}
Iteration: {{iteration}}

## Your Task

Review the {{review_target}} for phase {{phase}}.

### Input Artifacts
{{#each required_artifacts_in}}
- `{{this}}` -- {{describe this}}
{{/each}}

### What to Check

{{#if is_qa_review}}
For EVERY item in the plan specification:
- [ ] Is there at least one test?
- [ ] Are edge cases covered?
- [ ] Are assertions meaningful (not just assert!(true))?
- [ ] Do tests import from implementation (not inline definitions)?
{{/if}}

{{#if is_impl_review}}
For each claim in the reasoning:
- [ ] Is there concrete evidence?
- [ ] Were alternatives ruled out with evidence?
- [ ] Does the prediction match what happened?
- [ ] Could there be a simpler explanation?
{{/if}}

### Red Flags

| Red Flag | Challenge |
|----------|-----------|
{{#if is_qa_review}}
| Empty test body | "This test asserts nothing" |
| `if condition { assert }` | "Conditional assertion may not run" |
| No edge cases | "What about empty/null/negative?" |
{{/if}}
{{#if is_impl_review}}
| "Probably X" | "What evidence?" |
| "Must be variance" | "Did you run multiple times?" |
| Complex fix for simple symptom | "Simpler explanation?" |
{{/if}}

## Output Format

Write to `{{output_artifact}}`:

```markdown
# {{review_type}} Review: {{phase}}

## Summary
[Brief summary of what was reviewed]

## Issues Found

### {{#if is_qa_review}}Coverage Gaps{{else}}Concerns{{/if}}

{{#if is_qa_review}}
- [ ] `function_name` -- no test found
- [ ] `test_name` -- missing edge case for empty input
{{else}}
- [ ] **Claim**: [what they said]
  **Challenge**: [why questionable]
  **Required**: [evidence needed]
{{/if}}

## Verdict

{{#if is_qa_review}}
approved -- Tests adequately cover the specification
OR
gaps_found -- Must address: [list critical gaps]
{{else}}
approved -- Reasoning is sound
OR
concerns -- Address before proceeding: [list]
{{/if}}
```

Be {{#if is_qa_review}}thorough{{else}}adversarial{{/if}}. Your job is to find problems.

When done, exit.
```

## Template Processing

### Processing Pipeline

```
1. Load template file
2. Load includes ({{> path}})
3. Resolve variables from sources
4. Evaluate conditionals
5. Expand iterations
6. Output final prompt
```

### Implementation

```bash
# In iterate.sh
render_prompt() {
    local template="$1"
    local state_file="$2"
    local workflow="$3"
    local plan_md="$4"

    # Build context object
    local context=$(jq -n \
        --slurpfile state "$state_file" \
        --slurpfile workflow "$workflow" \
        --arg phase "$PHASE" \
        --arg plan "$PLAN" \
        --arg phase_dir "$PHASE_DIR" \
        '{
            state: $state[0],
            workflow: $workflow[0],
            phase: $phase,
            plan: $plan,
            phase_dir: $phase_dir,
            iteration: $state[0].iteration,
            max_iterations: ($workflow[0].defaults.max_iterations // 10),
            escalation_active: ($state[0].stuck_iterations >= 3),
            stuck_iterations: ($state[0].stuck_iterations // 0)
        }')

    # Add orchestrator notes if present
    if [[ -f "$PHASE_DIR/orchestrator_notes.md" ]]; then
        local notes=$(cat "$PHASE_DIR/orchestrator_notes.md")
        context=$(echo "$context" | jq --arg notes "$notes" '. + {orchestrator_notes: $notes}')
    fi

    # Process template
    process_template "$template" "$context"
}
```

## Best Practices

### Prompt Design

1. **Front-load critical information** -- Put constraints and context first
2. **Use conditional sections** -- Don't show irrelevant info
3. **Be explicit about artifacts** -- Name files exactly
4. **Include examples** -- Show expected output format
5. **End with clear exit criteria** -- "When done, exit"

### Variable Naming

- Use `snake_case` for all variables
- Prefix computed variables with context (`state.`, `plan.`, etc.)
- Use descriptive names (`remaining_iterations` not `rem`)

### Conditional Logic

- Keep conditions simple (`{{#if x}}` not `{{#if (and x (not y))}}`)
- Nest sparingly (max 2 levels)
- Use `{{else}}` for binary states
- Use `{{#unless}}` for negative conditions

### Includes

- Put reusable content in `prompts/common/`
- Keep includes focused (one topic per file)
- Document what variables an include expects
