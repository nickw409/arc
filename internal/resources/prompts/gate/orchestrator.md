A phase has failed {{.AttemptCount}} gate attempt(s). Mechanical retries are exhausted.
Your job: read the context, make one strategic change, then exit.

## Phase
**{{.PhaseName}}**

Spec:
{{.SpecSummary}}

## Plan File
The phase spec lives at: `{{.PlanMDPath}}`

Read it now. The `## Spec` yaml block defines what was attempted and what the gate checks.

## Attempt History
{{range $i, $a := .Attempts}}
### Attempt {{add $i 1}}
Gate output:
```
{{$a.GateOutput}}
```
{{end}}

## Current Code State (git diff --stat)
```
{{.DiffSummary}}
```

## Decision

Choose ONE action and execute it:

**MODIFY_SPEC** — The spec is too ambitious, wrong approach, or missing context.
Simplify the spec field or change the approach in `{{.PlanMDPath}}`.
Preserve the `## Spec` yaml block structure — only edit the `spec:` text inside it.

**ADJUST_GATE** — A gate assertion is wrong, too strict, or checking the wrong thing.
Fix the `gate.assertions` list in `{{.PlanMDPath}}`.
Preserve the `## Spec` yaml block structure — only edit the assertions.

**FIX_CODE** — The spec and gate are correct but code needs a direct fix.
Make the code change now. Run tests to verify. Do NOT edit plan.md.

**GIVE_UP** — This phase cannot succeed in its current form and needs human review.
Do NOT edit any files. Just stop.

Pick the ONE action most likely to unblock this phase. Do not attempt multiple actions.
The gate will be re-evaluated automatically after you exit.
