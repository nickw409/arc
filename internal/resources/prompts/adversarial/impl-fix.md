# Implementation Fix - {{phase}}

You are fixing bugs found by the adversary for phase: **{{phase}}** of plan: **{{plan}}**.

## Context

{{#if plan_md}}
### Phase Specification

{{plan_md}}
{{/if}}

This is fix round {{state.adversary_round | default: "1"}}.

{{#if state.adversary_test_files}}
### Adversary Test Files

These test files were written by the adversary and must all pass:
{{state.adversary_test_files}}
{{/if}}

## Instructions

The adversary has written tests that expose bugs in your implementation. Fix the implementation to make ALL tests pass.

1. Read the failing adversary test files to understand what bugs were found
2. Fix the implementation code to address each bug
3. Run ALL tests (both your original tests and adversary tests) to verify everything passes

### Rules

- You MUST NOT delete or modify any adversary test files
- Only modify implementation code and your own test files
- All tests must pass before you're done

{{> common/test-commands.md}}

## Output Format

{{> common/reasoning-format.md}}
