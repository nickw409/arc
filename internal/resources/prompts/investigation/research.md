# Research Phase

Plan: {{plan}}
Phase: {{phase}}
Plan doc: {{plan_file}}

## Your Task

Research the topic described in the plan by examining the codebase and gathering information.

## Steps

1. **Read the plan** to understand what needs to be investigated
2. **Search the codebase** for relevant code, patterns, or data
3. **Take notes** on what you find
4. **Identify gaps** in your understanding

## Research Notes

Create `{{phase_dir}}/research_notes.md`:

```markdown
# Research Notes: {{phase}}

## Research Questions
<from the plan>

## Findings

### Finding 1: <topic>
- **Location**: <file:line or general area>
- **Description**: <what you found>
- **Relevance**: <how it answers the research questions>

### Finding 2: ...

## Code Patterns Found
<any patterns, conventions, or approaches discovered>

## Open Questions
<things still unclear that need more investigation>

## Key Files
- `<path>` - <why it's relevant>
```

## Tools Available
- Glob: Find files by pattern
- Grep: Search file contents
- Read: Read file contents

## Rules
- Do NOT modify any code
- Focus on gathering information, not solving problems
- Document everything you find
- **Do NOT commit** - orchestrator handles commits

When done, write:

## Memory
[Key findings, files examined, open questions. Future runs of this state will see this.]

## Verdict
done
