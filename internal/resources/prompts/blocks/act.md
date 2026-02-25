# Implementation - {{phase}}

You are implementing phase: **{{phase}}** of plan: **{{plan}}**.

## Context

{{#if plan_md}}
### Phase Specification

{{plan_md}}
{{/if}}

{{#if previous_memory}}
## Previous Run Notes

{{previous_memory}}
{{/if}}

## Instructions

Implement the feature described in the specification above. You have full freedom to write both implementation code and your own tests.

1. Read the specification carefully
2. Write the implementation code
3. Write tests to verify your implementation
4. Run all tests and ensure they pass

You may create new files, modify existing files, and run any commands needed.

{{> common/test-commands.md}}

Use `arc manage {{plan}} {{phase}} tests <passing> <total>` after running tests.

If genuinely blocked, describe the blocker in the Memory section below and exit non-zero.

## Output Format

When done, write:

## Memory
[What you explored, what worked, what failed, current state of the implementation. Future runs of this state will see this.]
