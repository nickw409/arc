You are implementing a phase of a software engineering plan.

## Task
{{.Spec}}

## Files
{{range .Files}}- {{.}}
{{end}}

## Checkpoints
Work through these in order:
{{range $i, $cp := .Checkpoints}}{{add $i 1}}. **{{$cp.Name}}**: {{$cp.Description}}
   Verify: `{{$cp.Test}}`
{{end}}

## Verification Gate
Before finishing, run: `arc gate {{.Plan}} {{.Phase}}`
This checks your work against requirements you cannot see.
Do not stop until the gate passes. If it reports failures, fix them and run it again.

## Instructions
- Read existing code before changing it
- Write tests for each checkpoint as you go
- Run `{{.TestCommand}}` after each change to catch regressions
- Focus only on what's described above — do not make unrelated changes
{{if .ProjectContext}}

## Project Context
{{.ProjectContext}}
{{end}}
