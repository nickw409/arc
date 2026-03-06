A phase has failed {{.AttemptCount}} attempts. The orchestrator has exhausted
mechanical retries and needs a strategic decision.

## Phase
{{.PhaseName}}: {{.SpecSummary}}

## Attempt History
{{range $i, $a := .Attempts}}### Attempt {{add $i 1}}
Gate output:
```
{{$a.GateOutput}}
```
Checkpoints passed: {{$a.CheckpointsPassed}} / {{$a.CheckpointsTotal}}
{{end}}

## Current Code State
```
{{.DiffSummary}}
```

## Decision Required
Choose ONE action:
1. **MODIFY_SPEC** — Simplify the spec or change the approach. Provide the new spec.
2. **ADJUST_GATE** — Relax gate criteria that are too strict. Specify which assertions to change.
3. **SPLIT_PHASE** — Break into smaller phases. Provide the new phase definitions.
4. **GIVE_UP** — The task is not achievable in its current form. Explain why.

Respond with your decision and the specific changes to make.
