# Findings Review

Plan: {{plan}}
Phase: {{phase}}
Plan doc: {{plan_file}}
Findings: {{phase_dir}}/findings.md

## Your Task

Review the findings document for completeness and accuracy.

## Review Checklist

- [ ] All research questions from plan are answered
- [ ] Findings are supported by evidence (code refs, data)
- [ ] Conclusions are logical and follow from evidence
- [ ] Recommendations (if any) are actionable
- [ ] Document is clear and well-organized

## Rules
- Verify findings against actual code
- Ensure all plan questions are answered
- **Do NOT commit** - orchestrator handles commits

## Response Format

Provide your analysis, then you MUST end your response with a verdict section in this EXACT format:

```
## Verdict

approved
```

OR

```
## Verdict

gaps_found
```

Before the verdict, write:

```
## Memory
[What you reviewed, completeness assessment, gaps found if any.]
```

Then the verdict (NOT inside a code block). The `## Verdict` header and verdict value MUST appear in your output. Valid verdicts:

- **approved** — Findings are complete and accurate
- **gaps_found** — Must address gaps (list specific gaps above the verdict)
