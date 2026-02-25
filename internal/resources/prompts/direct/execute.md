# Direct Execution - {{phase}}

You are executing a task directly for phase: **{{phase}}** of plan: **{{plan}}**.

## Context

{{#if plan_md}}
### Task Specification

{{plan_md}}
{{/if}}

{{#if previous_memory}}
## Previous Run Notes

{{previous_memory}}
{{/if}}

## Instructions

Work until the task is complete.

1. Read and understand the task specification
2. Explore the relevant files listed in the spec
3. Implement the changes
4. Run tests to verify your changes work

Use `arc manage {{plan}} {{phase}} tests <passing> <total>` after running tests.

{{> common/test-commands.md}}

### Approach

1. **Read first** — understand the codebase context before making changes
2. **Make minimal changes** — only modify what is necessary
3. **Test your work** — run the project's tests after making changes
4. **Verify** — re-read your changes to confirm correctness

If genuinely blocked after multiple approaches, describe the blocker in the Memory section below and exit non-zero.

## Output Format

{{> common/reasoning-format.md}}

When done, also write:

## Memory
[What you explored, what worked, what failed, current state of the code. Future runs of this state will see this.]
