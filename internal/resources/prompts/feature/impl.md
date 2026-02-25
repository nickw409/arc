# Implementation - {{phase}}

You are implementing phase: **{{phase}}** of plan: **{{plan}}**.

## Context

{{#if plan_md}}
### Phase Specification

{{plan_md}}
{{/if}}

### Test Results

Tests passing: {{state.tests_passing | default: "0"}} / {{state.tests_total | default: "unknown"}}

{{#if previous_memory}}
## Previous Run Notes

{{previous_memory}}
{{/if}}

{{#if params.focus_area}}
### Focus Area

Focus your implementation on: **{{params.focus_area}}**
{{/if}}

## Instructions

{{#if state.last_verdict}}
The adversary found bugs and wrote failing tests to prove them. Your job is to fix the implementation.

1. Run the tests to see which adversary tests are failing
2. Read the failing tests to understand what bugs they expose
3. Fix the implementation to make all tests pass

Do NOT modify any test files — only fix the implementation code.

{{> common/test-commands.md}}

{{> common/do-not-rules.md}}
{{else}}
Write the implementation and a thorough test suite, then make all tests pass.

- Use `arc manage {{plan}} {{phase}} note <text>` to record intermediate progress
- Use `arc manage {{plan}} {{phase}} tests <passing> <total>` after each test run

{{> common/test-commands.md}}
{{/if}}

If genuinely blocked after multiple approaches, describe the blocker in the Memory section below and exit non-zero.

## Output Format

{{> common/reasoning-format.md}}

When done, also write:

## Memory
[What you explored, what worked, what failed, current state of the implementation. Future runs of this state will see this.]
