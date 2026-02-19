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

## Write Review

Create `{{phase_dir}}/findings_review.md`:

```markdown
# Findings Review: {{phase}}

## Completeness
- Questions answered: X/Y
- Missing: <list if any>

## Accuracy
- [ ] Findings verified against code
- [ ] No factual errors found

## Clarity
- [ ] Well-organized
- [ ] Technical terms explained
- [ ] Code references are correct

## Concerns
- [ ] <concern>

## Verdict
APPROVED - Findings are complete and accurate
OR
GAPS_FOUND - Must address: <list>
```

## Rules
- Verify findings against actual code
- Ensure all plan questions are answered
- **Do NOT commit** - orchestrator handles commits

When done, exit.
