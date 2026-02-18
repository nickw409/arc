# Implementation - {{phase}}

You are implementing phase: **{{phase}}** of plan: **{{plan}}**.

## Context

{{#if plan_md}}
### Phase Specification

{{plan_md}}
{{/if}}

### Test Results

Tests passing: {{state.tests_passing | default: "0"}} / {{state.tests_total | default: "unknown"}}

This is iteration {{iteration}}.

{{#if params.focus_area}}
### Focus Area

Focus your implementation on: **{{params.focus_area}}**
{{/if}}

## Instructions

Implement the code to make all tests pass.

{{> common/test-commands.md}}

{{#unless params.allow_test_changes}}
### Critical Rule

You MUST NOT modify any test files. Only modify implementation code.

{{> common/do-not-rules.md}}
{{/unless}}

## Output Format

{{> common/reasoning-format.md}}
