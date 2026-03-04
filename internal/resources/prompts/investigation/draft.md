# Draft Findings

Plan: {{plan}}
Phase: {{phase}}
Plan doc: {{plan_file}}
Research notes: {{phase_dir}}/research_notes.md

## Your Task

Write a findings document that answers the research questions from the plan.

## Steps

1. **Read your research notes** to review what you found
2. **Organize findings** into a coherent structure
3. **Write clear conclusions** that answer the research questions
4. **Include recommendations** if applicable

## Findings Document

Create `{{phase_dir}}/findings.md`:

```markdown
# Investigation Findings: {{phase}}

## Executive Summary
<1-2 sentence summary of key findings>

## Research Questions & Answers

### Question 1: <from plan>
**Answer**: <your finding>
**Evidence**: <code references, data>

### Question 2: ...

## Detailed Findings

### <Topic 1>
<detailed explanation with code references>

### <Topic 2>
...

## Recommendations
<if applicable, what actions should be taken based on findings>

## Appendix
- Key files examined
- Related documentation
- Further reading
```

## Rules
- Answer ALL research questions from the plan
- Support findings with evidence (file paths, code snippets)
- Be objective - report what IS, not what should be
- **Do NOT commit** - orchestrator handles commits

When done, write:

## Memory
[What was drafted, key conclusions, areas that need more depth. Future runs of this state will see this.]

## Verdict
done
