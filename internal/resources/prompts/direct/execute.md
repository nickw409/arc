# Direct Execution - {{phase}}

You are executing a task directly for phase: **{{phase}}** of plan: **{{plan}}**.

## Context

{{#if plan_md}}
### Task Specification

{{plan_md}}
{{/if}}

### Iteration
This is iteration {{iteration}}.

## Instructions

Execute the task described in the specification above. This is a simple task that should be completed in a single pass.

1. Read and understand the task specification
2. Explore the relevant files listed in the spec
3. Implement the changes
4. Run tests to verify your changes work

{{> common/test-commands.md}}

### Approach

1. **Read first** — understand the codebase context before making changes
2. **Make minimal changes** — only modify what is necessary
3. **Test your work** — run the project's tests after making changes
4. **Verify** — re-read your changes to confirm correctness

## Output Format

{{> common/reasoning-format.md}}
